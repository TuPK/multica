package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/issueposition"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// EffectRef là phần của effect_event mà Reach Map đọc. effect_event (B2) chưa có,
// nên đây là kiểu thuần; khi có bảng, SurfaceOf nhận db.EffectEvent qua một hàm chuyển đổi nhỏ.
type EffectRef struct {
	Source string
	Target string
}

// SurfaceOf trả bề mặt (spec 11.2) của một hệ quả. Giả định phân tích chuỗi:
//   - connected_app: Target dạng TOOLKIT_ACTION thì toolkit là phần trước dấu "_" đầu tiên
//     (không đổi hoa/thường); không tách được (không có "_", hoặc "_" ở đầu) thì dùng cả Target.
//   - channel: Target là loại kênh (ví dụ "slack").
//   - declared: Target là URL (có "://") thì lấy hostname, bỏ cổng; "smtp:<host>[:port]" và
//     "host:<host>[:port]" lấy phần host; còn lại, hoặc rỗng, là "unknown".
//   - nguồn khác: work_class cắt tới dấu ":" thứ hai ("flow:abc:stage" → "flow:abc");
//     không có hai dấu ":" thì giữ nguyên.
func SurfaceOf(e EffectRef) string {
	switch e.Source {
	case "connected_app":
		return "app:" + toolkitOf(e.Target)
	case "channel":
		return "channel:" + e.Target
	case "declared":
		return "external:" + hostOf(e.Target)
	default:
		return "area:" + workClassPrefix(e.Target)
	}
}

func toolkitOf(target string) string {
	if i := strings.Index(target, "_"); i > 0 {
		return target[:i]
	}
	return target
}

func hostOf(target string) string {
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil || u.Hostname() == "" {
			return "unknown"
		}
		return u.Hostname()
	}
	for _, prefix := range []string{"smtp:", "host:"} {
		if rest, ok := strings.CutPrefix(target, prefix); ok {
			host, _, _ := strings.Cut(rest, ":")
			if host != "" {
				return host
			}
			return "unknown"
		}
	}
	return "unknown"
}

func workClassPrefix(workClass string) string {
	parts := strings.SplitN(workClass, ":", 3)
	if len(parts) < 3 {
		return workClass
	}
	return parts[0] + ":" + parts[1]
}

// SurfaceStats là dữ liệu 30 ngày của một bề mặt, đã gom sẵn. ReachState chỉ đọc các trường này.
type SurfaceStats struct {
	EffectsAgent30d    int  // hệ quả do agent làm
	Effects30d         int  // mọi hệ quả trên bề mặt (người và agent)
	DeclaredEffects30d int  // hệ quả nguồn declared
	AgentConfirm30d    int  // hệ quả của agent đi qua confirm hoặc rehearse
	AllowedAgent       bool // có hệ quả của agent được allow trong 30 ngày
	GapAsks30d         int  // câu hỏi missing_access | missing_info
	MissingAccess      bool // có câu hỏi missing_access cho bề mặt này
}

// ReachState trả trạng thái của bề mặt theo thứ tự ưu tiên của bảng 11.2.
// "declared_only" đòi mọi hệ quả là declared; "human_only" đòi không có hệ quả của agent.
func ReachState(s SurfaceStats) string {
	switch {
	case s.AllowedAgent && s.GapAsks30d == 0:
		return "agent_operable"
	case s.EffectsAgent30d > 0 && 2*s.AgentConfirm30d > s.EffectsAgent30d:
		return "agent_with_confirm"
	case s.EffectsAgent30d == 0 && s.DeclaredEffects30d > 0 && s.DeclaredEffects30d == s.Effects30d:
		return "declared_only"
	case s.MissingAccess && s.EffectsAgent30d == 0:
		return "human_only"
	default:
		return "unknown"
	}
}

// reachWindowDays is the window of every count on the map (spec 11.2).
const reachWindowDays = 30

// reachRetentionDays is how long snapshots are kept (spec 11.2).
const reachRetentionDays = 400

// reachTopAsks bounds the sample of asks a surface's row keeps.
const reachTopAsks = 5

var (
	// ErrReachOff: a write was asked while durable_open is off.
	ErrReachOff = errors.New("reach map is off")
	// ErrReachSurfaceInvalid: a surface is not app:, channel:, area: or external: followed by a name.
	ErrReachSurfaceInvalid = errors.New("reach surface must be app:, channel:, area: or external: with a name")
	// ErrReachSurfaceUnknown: the newest snapshot has no row for the surface.
	ErrReachSurfaceUnknown = errors.New("reach surface is not in the latest snapshot")
	// ErrReachSquadNotFound: the squad chosen for the gap issue is not in this workspace.
	ErrReachSquadNotFound = errors.New("squad not found in this workspace")
	// ErrReachNoSquad: no squad has asked about the surface, so the gap issue has no one to go to.
	ErrReachNoSquad = errors.New("no squad has asked about this surface")
)

// ReachService computes the Reach Map (durable spec 11, F10) nightly and opens the issues that
// close its gaps. It writes nothing while durable_open is off (spec 14.1).
type ReachService struct {
	Queries      *db.Queries
	Issues       *IssueService
	Verification *VerificationService
	Flags        *featureflag.Service
	// Now is the clock for the snapshot day. Nil means time.Now.
	Now func() time.Time
}

func (s *ReachService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *ReachService) on(ctx context.Context) bool {
	return featureflags.DurableOpenEnabled(ctx, s.Flags)
}

// ValidReachSurface reports whether a surface names one of the four kinds of spec 11.2.
func ValidReachSurface(surface string) bool {
	for _, prefix := range []string{"app:", "channel:", "area:", "external:"} {
		if rest, ok := strings.CutPrefix(surface, prefix); ok && rest != "" {
			return true
		}
	}
	return false
}

// reachDay is the UTC date a snapshot is filed under.
func reachDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// reachAcc gathers one surface's counts before its state is read.
type reachAcc struct {
	stats   SurfaceStats
	minutes float64
	asks    []pgtype.UUID
}

// Snapshot recomputes the map of one workspace for a day from the 30 days that end with it, and
// writes one row per surface. A second run of the same day overwrites its rows (spec 11.5, 3).
// It writes nothing while durable_open is off.
func (s *ReachService) Snapshot(ctx context.Context, ws pgtype.UUID, day time.Time) error {
	if !s.on(ctx) {
		return nil
	}
	day = reachDay(day)
	end := day.AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -reachWindowDays)
	effects, err := s.Queries.ReachEffectCounts(ctx, db.ReachEffectCountsParams{
		WorkspaceID: ws, WindowStart: pgtype.Timestamptz{Time: start, Valid: true}, WindowEnd: pgtype.Timestamptz{Time: end, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("count effects: %w", err)
	}
	asks, err := s.reachAsks(ctx, ws, start, end)
	if err != nil {
		return err
	}

	surfaces := map[string]*reachAcc{}
	acc := func(surface string) *reachAcc {
		a, ok := surfaces[surface]
		if !ok {
			a = &reachAcc{}
			surfaces[surface] = a
		}
		return a
	}
	for _, e := range effects {
		a := acc(SurfaceOf(EffectRef{Source: e.Source, Target: e.Target}))
		a.stats.Effects30d += int(e.N)
		if e.Source == "declared" {
			// Self-declared effects are not platform-controlled agent effects (spec 4.5, 11.2).
			a.stats.DeclaredEffects30d += int(e.N)
		} else if e.ByAgent {
			a.stats.EffectsAgent30d += int(e.N)
			if e.Decision == "allow" {
				a.stats.AllowedAgent = true
			}
			if e.Decision == "confirm" || e.Decision == "rehearse" {
				a.stats.AgentConfirm30d += int(e.N)
			}
		}
	}
	for _, k := range asks {
		a := acc(SurfaceOf(EffectRef{Source: k.Source, Target: k.Target}))
		a.minutes += k.Minutes
		if k.Reason == "missing_access" || k.Reason == "missing_info" {
			a.stats.GapAsks30d += int(k.N)
		}
		if k.Reason == "missing_access" {
			a.stats.MissingAccess = true
		}
		for _, id := range k.TopAskIds {
			if len(a.asks) < reachTopAsks {
				a.asks = append(a.asks, id)
			}
		}
	}

	names := make([]string, 0, len(surfaces))
	for name := range surfaces {
		names = append(names, name)
	}
	sort.Strings(names)
	computed := pgtype.Timestamptz{Time: s.now(), Valid: true}
	for _, name := range names {
		a := surfaces[name]
		var minutes pgtype.Numeric
		if err := minutes.Scan(strconv.FormatFloat(a.minutes, 'f', 2, 64)); err != nil {
			return fmt.Errorf("human minutes: %w", err)
		}
		topAsks := a.asks
		if topAsks == nil {
			topAsks = []pgtype.UUID{}
		}
		if err := s.Queries.UpsertReachSnapshotRow(ctx, db.UpsertReachSnapshotRowParams{
			WorkspaceID: ws, SnapshotDate: pgtype.Date{Time: day, Valid: true}, Surface: name,
			State: ReachState(a.stats), Effects30d: int32(a.stats.Effects30d), AgentEffects30d: int32(a.stats.EffectsAgent30d),
			HumanMinutes30d: minutes, GapAsks30d: int32(a.stats.GapAsks30d), TopAskIds: topAsks, ComputedAt: computed,
		}); err != nil {
			return fmt.Errorf("write surface %s: %w", name, err)
		}
	}
	return nil
}

// reachAsks reads the window's asks, which the map groups by surface.
func (s *ReachService) reachAsks(ctx context.Context, ws pgtype.UUID, start, end time.Time) ([]db.ReachAskCountsRow, error) {
	rows, err := s.Queries.ReachAskCounts(ctx, db.ReachAskCountsParams{
		WorkspaceID: ws, WindowStart: pgtype.Timestamptz{Time: start, Valid: true}, WindowEnd: pgtype.Timestamptz{Time: end, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("count asks: %w", err)
	}
	return rows, nil
}

// Nightly writes today's map of every workspace and drops snapshots older than 400 days. It
// returns the number of workspaces written, and writes nothing while durable_open is off.
func (s *ReachService) Nightly(ctx context.Context) (int, error) {
	if !s.on(ctx) {
		return 0, nil
	}
	ids, err := s.Queries.ListAllWorkspaceIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list workspaces: %w", err)
	}
	today := reachDay(s.now())
	n := 0
	for _, ws := range ids {
		if err := s.Snapshot(ctx, ws, today); err != nil {
			return n, err
		}
		n++
	}
	cutoff := today.AddDate(0, 0, -reachRetentionDays)
	if _, err := s.Queries.DeleteReachSnapshotsBefore(ctx, pgtype.Date{Time: cutoff, Valid: true}); err != nil {
		return n, fmt.Errorf("drop old snapshots: %w", err)
	}
	return n, nil
}

// Latest returns the newest snapshot day of a workspace and its rows, the largest human time
// first. ok is false when no snapshot exists yet.
func (s *ReachService) Latest(ctx context.Context, ws pgtype.UUID) (day time.Time, rows []db.ReachSnapshot, ok bool, err error) {
	d, err := s.Queries.LatestReachSnapshotDate(ctx, ws)
	if err != nil {
		return time.Time{}, nil, false, fmt.Errorf("latest snapshot: %w", err)
	}
	if !d.Valid {
		return time.Time{}, nil, false, nil
	}
	rows, err = s.Queries.ListReachSnapshotDay(ctx, db.ListReachSnapshotDayParams{WorkspaceID: ws, SnapshotDate: d})
	if err != nil {
		return time.Time{}, nil, false, fmt.Errorf("list snapshot: %w", err)
	}
	return d.Time, rows, true, nil
}

// CloseGap opens the issue that asks a squad to close a surface's gap (spec 11.3), assigned to
// squadID, or to the squad that has asked most about the surface when squadID is not set. A second
// call returns the issue already open for the surface. The issue, its metric criterion, and the
// snapshot's gap_issue_id link commit together in one transaction under a row lock on the surface's
// snapshots so concurrent calls open at most one issue and a failure leaves no orphan issue.
func (s *ReachService) CloseGap(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, surface string, squadID pgtype.UUID) (db.Issue, error) {
	if !s.on(ctx) {
		return db.Issue{}, ErrReachOff
	}
	if !ValidReachSurface(surface) {
		return db.Issue{}, ErrReachSurfaceInvalid
	}
	if s.Verification == nil || !s.Verification.outcomesEnabled(ctx) {
		return db.Issue{}, ErrOutcomesOff
	}
	issueCountPolicy := ResolveIssueCountPolicy(ctx, s.Issues.Entitlements, ws)

	tx, err := s.Issues.TxStarter.Begin(ctx)
	if err != nil {
		return db.Issue{}, fmt.Errorf("begin close gap tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	lockedRows, err := qtx.LockReachSnapshotRowsForSurface(ctx, db.LockReachSnapshotRowsForSurfaceParams{
		WorkspaceID: ws, Surface: surface,
	})
	if err != nil {
		return db.Issue{}, fmt.Errorf("lock reach snapshot: %w", err)
	}
	if len(lockedRows) == 0 {
		return db.Issue{}, ErrReachSurfaceUnknown
	}
	for _, r := range lockedRows {
		if r.GapIssueID.Valid {
			if issue, err := qtx.GetIssue(ctx, r.GapIssueID); err == nil {
				return issue, nil
			}
		}
	}

	day, err := qtx.LatestReachSnapshotDate(ctx, ws)
	if err != nil {
		return db.Issue{}, fmt.Errorf("latest snapshot: %w", err)
	}
	if !day.Valid || lockedRows[0].SnapshotDate != day {
		return db.Issue{}, ErrReachSurfaceUnknown
	}
	row := lockedRows[0]

	if squadID.Valid {
		// A squad chosen by the caller must belong to this workspace before the issue is
		// assigned to it: the schema has no foreign key to check it.
		if _, err := qtx.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: squadID, WorkspaceID: ws}); err != nil {
			return db.Issue{}, mapNoRows(err, ErrReachSquadNotFound, "load squad")
		}
	}
	if !squadID.Valid {
		squadID, err = s.topGapSquadWith(ctx, qtx, ws, surface)
		if err != nil {
			return db.Issue{}, err
		}
	}

	issueNumber, err := AllocateIssueNumber(ctx, qtx, ws, issueCountPolicy)
	if err != nil {
		return db.Issue{}, fmt.Errorf("allocate gap issue number: %w", err)
	}
	const status = "todo"
	pos, err := issueposition.NextTopPosition(ctx, tx, ws, status)
	if err != nil {
		return db.Issue{}, fmt.Errorf("gap issue position: %w", err)
	}
	issue, err := qtx.CreateIssue(ctx, db.CreateIssueParams{
		ID:           dbid.NewV7(),
		WorkspaceID:  ws,
		Title:        "Lấp khoảng trống: " + surface,
		Description:  pgtype.Text{String: reachGapBody(surface, row.GapAsks30d), Valid: true},
		Status:       status,
		Priority:     "none",
		AssigneeType: pgtype.Text{String: "squad", Valid: true},
		AssigneeID:   squadID,
		CreatorType:  actor.Type,
		CreatorID:    actor.ID,
		Position:     pos,
		Number:       issueNumber,
		Properties:   []byte("{}"),
	})
	if err != nil {
		return db.Issue{}, fmt.Errorf("create gap issue: %w", err)
	}
	source := criterionSourceMember
	if actor.Type == GovernanceActorAgent {
		source = criterionSourceAgent
	}
	if _, err := s.Verification.insertCriterion(ctx, qtx, ws, issue.ID, actor, source, CriterionInput{
		Text:      fmt.Sprintf("Số câu hỏi thiếu quyền hoặc thông tin của %s giảm còn một nửa (từ %d) trong 30 ngày.", surface, row.GapAsks30d),
		CheckKind: CheckKindMetric,
		CheckSpec: map[string]any{
			"metric_key":  "reach.gap_asks_30d",
			"scope":       "company",
			"op":          "<=",
			"value":       float64(row.GapAsks30d) / 2,
			"within_days": reachWindowDays,
			"surface":     surface,
			"baseline":    row.GapAsks30d,
		},
	}); err != nil {
		return db.Issue{}, fmt.Errorf("add gap criterion: %w", err)
	}
	if err := qtx.SetReachGapIssue(ctx, db.SetReachGapIssueParams{
		GapIssueID: issue.ID, WorkspaceID: ws, Surface: surface,
	}); err != nil {
		return db.Issue{}, fmt.Errorf("record gap issue: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Issue{}, fmt.Errorf("commit gap issue: %w", err)
	}
	return issue, nil
}

// topGapSquadWith returns the squad whose gap asks about the surface are most numerous in the 30 days
// to now. Ties go to the larger squad id, so the choice is the same on every call.
func (s *ReachService) topGapSquadWith(ctx context.Context, q *db.Queries, ws pgtype.UUID, surface string) (pgtype.UUID, error) {
	end := reachDay(s.now()).AddDate(0, 0, 1)
	rows, err := q.ReachAskCounts(ctx, db.ReachAskCountsParams{
		WorkspaceID: ws,
		WindowStart: pgtype.Timestamptz{Time: end.AddDate(0, 0, -reachWindowDays), Valid: true},
		WindowEnd:   pgtype.Timestamptz{Time: end, Valid: true},
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("count asks: %w", err)
	}
	counts := map[pgtype.UUID]int64{}
	for _, k := range rows {
		if !k.SquadID.Valid || SurfaceOf(EffectRef{Source: k.Source, Target: k.Target}) != surface {
			continue
		}
		if k.Reason == "missing_access" || k.Reason == "missing_info" {
			counts[k.SquadID] += k.N
		}
	}
	var best pgtype.UUID
	var bestN int64
	for id, n := range counts {
		if n > bestN || (n == bestN && string(id.Bytes[:]) > string(best.Bytes[:])) {
			best, bestN = id, n
		}
	}
	if bestN == 0 {
		return pgtype.UUID{}, ErrReachNoSquad
	}
	return best, nil
}

// reachGapBody is the issue's description: the surface, its size, and what usually closes it.
func reachGapBody(surface string, gapAsks int32) string {
	var hint string
	switch {
	case strings.HasPrefix(surface, "app:"):
		hint = "Kết nối tài khoản của công cụ này cho công ty, rồi cho agent quyền dùng nó."
	case strings.HasPrefix(surface, "external:"):
		hint = "Xây công cụ cho hệ thống này (ví dụ bằng CLI-Anything) hoặc dùng một connected app."
	case strings.HasPrefix(surface, "channel:"):
		hint = "Cấp cho agent quyền dùng kênh này."
	default:
		hint = "Bổ sung tài liệu knowledge cho loại việc này."
	}
	return fmt.Sprintf("Bề mặt: %s. Trong 30 ngày có %d câu hỏi thiếu quyền hoặc thông tin.\n\n%s\n\nĐóng issue khi số câu hỏi giảm còn một nửa.", surface, gapAsks, hint)
}
