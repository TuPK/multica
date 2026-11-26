package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The outcome numbers of a company brief (durable spec 2.7, 3.3, 3.4). The Secretary writes
// the brief's prose; these facts are computed from the database and placed under its
// Progress section, so the numbers the boss reads are the stored ones and not a model's
// rendering of them.

// briefModeChangesMax bounds the verification mode changes a brief lists.
const briefModeChangesMax = 5

// BriefOutcomeFacts are the outcome facts of one brief period.
type BriefOutcomeFacts struct {
	Settled    int64
	Pass       int64
	Partial    int64
	Fail       int64
	Reopened   int64
	Unverified int64
	// ModeChanges are the pairs whose verification mode moved in the period, newest first.
	ModeChanges []BriefModeChange
	// EnvelopeMoves counts the squad levels the envelope moved on evidence in the period.
	EnvelopeMoves int
}

// BriefModeChange is one pair whose verification mode moved.
type BriefModeChange struct {
	AgentName string
	WorkClass string
	Mode      string
}

// BriefFactsFor reads the outcome facts of the period [start, end].
func BriefFactsFor(ctx context.Context, q *db.Queries, ws pgtype.UUID, start, end time.Time) (BriefOutcomeFacts, error) {
	period := pgtype.Timestamptz{Time: start, Valid: true}
	periodEnd := pgtype.Timestamptz{Time: end, Valid: true}

	counts, err := q.CountWorkOutcomesSettledBetween(ctx, db.CountWorkOutcomesSettledBetweenParams{
		WorkspaceID: ws, PeriodStart: period, PeriodEnd: periodEnd,
	})
	if err != nil {
		return BriefOutcomeFacts{}, fmt.Errorf("brief outcome counts: %w", err)
	}
	modes, err := q.ListTrackModeChangesBetween(ctx, db.ListTrackModeChangesBetweenParams{
		WorkspaceID: ws, PeriodStart: period, PeriodEnd: periodEnd, RowLimit: briefModeChangesMax,
	})
	if err != nil {
		return BriefOutcomeFacts{}, fmt.Errorf("brief mode changes: %w", err)
	}
	moves, err := q.ListAutoAutonomyChangesSince(ctx, db.ListAutoAutonomyChangesSinceParams{
		WorkspaceID: ws, Since: period, RowLimit: 1,
	})
	if err != nil {
		return BriefOutcomeFacts{}, fmt.Errorf("brief envelope moves: %w", err)
	}

	facts := BriefOutcomeFacts{
		Settled: counts.Settled, Pass: counts.NPass, Partial: counts.NPartial,
		Fail: counts.NFail, Reopened: counts.NReopened, Unverified: counts.NUnverified,
	}
	for _, m := range modes {
		facts.ModeChanges = append(facts.ModeChanges, BriefModeChange{
			AgentName: m.AgentName, WorkClass: m.WorkClass, Mode: m.VerificationMode,
		})
	}
	if len(moves) > 0 {
		facts.EnvelopeMoves = int(moves[0].TotalCount)
	}
	return facts, nil
}

// BriefFactsMarkdown renders the facts as bullet lines for the Progress section. The outcome
// line is always there, so a quiet period reads as zero rather than as missing.
func BriefFactsMarkdown(f BriefOutcomeFacts) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Outcomes this period:** %d settled: %d pass, %d partial, %d fail (%d reopened), %d unverified.\n",
		f.Settled, f.Pass, f.Partial, f.Fail, f.Reopened, f.Unverified)
	for _, m := range f.ModeChanges {
		fmt.Fprintf(&b, "- Verification mode of %s (%s) is now %s.\n", m.AgentName, m.WorkClass, m.Mode)
	}
	fmt.Fprintf(&b, "- Autonomy envelope: %d squad level change(s) on evidence.\n", f.EnvelopeMoves)
	return b.String()
}

// BriefWithFacts places facts at the end of the body's Progress section: before the next
// level-2 heading after "## Progress", ignoring headings inside code fences. A body with no
// Progress heading comes back unchanged.
func BriefWithFacts(body, facts string) string {
	if facts == "" {
		return body
	}
	lines := strings.Split(body, "\n")
	progress := -1
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if progress < 0 {
			if trimmed == "## Progress" {
				progress = i
			}
			continue
		}
		if strings.HasPrefix(line, "## ") {
			return joinAt(lines, i, facts)
		}
	}
	if progress < 0 {
		return body
	}
	return joinAt(lines, len(lines), facts)
}

// joinAt returns lines with facts inserted before index at, separated by a blank line.
func joinAt(lines []string, at int, facts string) string {
	out := make([]string, 0, len(lines)+4)
	out = append(out, lines[:at]...)
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	out = append(out, "", strings.TrimRight(facts, "\n"), "")
	out = append(out, lines[at:]...)
	return strings.Join(out, "\n")
}
