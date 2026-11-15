package plan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/db"
)

// Beginner starts a transaction. *pgxpool.Pool is one.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PgxRunner is the Runner over a Postgres pool.
type PgxRunner struct{ pool Beginner }

// NewPgxRunner returns a Runner that opens one transaction per call.
func NewPgxRunner(pool Beginner) *PgxRunner { return &PgxRunner{pool: pool} }

// InTx implements Runner. It returns fn's error as it is, so a caller can still
// recognise its own error types.
func (r *PgxRunner) InTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op once committed
	if err := fn(&pgxTx{q: db.New(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// PgxReader is the Reader over a Postgres pool.
type PgxReader struct{ q *db.Queries }

// NewPgxReader returns a Reader over pool.
func NewPgxReader(pool db.DBTX) *PgxReader { return &PgxReader{q: db.New(pool)} }

// Get implements Reader.
func (r *PgxReader) Get(ctx context.Context, key string) (Plan, bool, error) {
	row, err := r.q.GetPlan(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, false, nil
	}
	if err != nil {
		return Plan{}, false, err
	}
	p, err := FromRow(row)
	return p, err == nil, err
}

// List implements Reader.
func (r *PgxReader) List(ctx context.Context) ([]Plan, error) {
	rows, err := r.q.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Plan, len(rows))
	for i, row := range rows {
		if out[i], err = FromRow(row); err != nil {
			return nil, err
		}
	}
	return out, nil
}

type pgxTx struct{ q *db.Queries }

func (t *pgxTx) Count(ctx context.Context) (int64, error) { return t.q.CountPlans(ctx) }

func (t *pgxTx) InsertIfAbsent(ctx context.Context, p Plan) (bool, error) {
	params, err := toInsertParams(p)
	if err != nil {
		return false, err
	}
	n, err := t.q.InsertPlanIfAbsent(ctx, db.InsertPlanIfAbsentParams(params))
	return n > 0, err
}

func (t *pgxTx) Get(ctx context.Context, key string) (Plan, bool, error) {
	row, err := t.q.GetPlanForUpdate(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, false, nil
	}
	if err != nil {
		return Plan{}, false, err
	}
	p, err := FromRow(row)
	return p, err == nil, err
}

func (t *pgxTx) Upsert(ctx context.Context, p Plan) error {
	params, err := toInsertParams(p)
	if err != nil {
		return err
	}
	return t.q.UpsertPlan(ctx, db.UpsertPlanParams(params))
}

func (t *pgxTx) Delete(ctx context.Context, key string) (bool, error) {
	n, err := t.q.DeletePlan(ctx, key)
	return n > 0, err
}

func (t *pgxTx) CountSubscriptions(ctx context.Context, key string) (int64, error) {
	return t.q.CountSubscriptionsOnPlan(ctx, key)
}

// InsertAuditEvent implements audit.Writer inside the transaction.
func (t *pgxTx) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	var ws pgtype.UUID
	if e.WorkspaceID != "" {
		if err := ws.Scan(e.WorkspaceID); err != nil {
			return fmt.Errorf("invalid workspace id %q: %w", e.WorkspaceID, err)
		}
	}
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	return t.q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		WorkspaceID: ws,
		ActorType:   e.ActorType,
		ActorID:     pgtype.Text{String: e.ActorID, Valid: e.ActorID != ""},
		ActorLabel:  e.ActorLabel,
		Action:      e.Action,
		TargetType:  e.TargetType,
		TargetID:    e.TargetID,
		TargetLabel: e.TargetLabel,
		Detail:      raw,
		Ip:          e.IP,
		UserAgent:   e.UserAgent,
		RequestID:   e.RequestID,
	})
}

// FromRow converts a plan row. The two jsonb columns are objects by their
// defaults; a value that is not is a data error.
func FromRow(r db.Plan) (Plan, error) {
	p := Plan{
		Key: r.Key, Name: r.Name, Currency: r.Currency, TrialDays: int(r.TrialDays),
		IsPublic: r.IsPublic, Sort: int(r.Sort), Limits: Limits{}, Features: map[string]bool{},
	}
	if err := json.Unmarshal(r.Limits, &p.Limits); err != nil {
		return Plan{}, fmt.Errorf("plan %s: limits: %w", r.Key, err)
	}
	if err := json.Unmarshal(r.Features, &p.Features); err != nil {
		return Plan{}, fmt.Errorf("plan %s: features: %w", r.Key, err)
	}
	if p.Limits == nil {
		p.Limits = Limits{}
	}
	if p.Features == nil {
		p.Features = map[string]bool{}
	}
	if r.SeatPriceMonth.Valid {
		v := r.SeatPriceMonth.Int64
		p.SeatPriceMonth = &v
	}
	if r.SeatPriceYear.Valid {
		v := r.SeatPriceYear.Int64
		p.SeatPriceYear = &v
	}
	if r.SeatsFixed.Valid {
		v := int(r.SeatsFixed.Int32)
		p.SeatsFixed = &v
	}
	if r.MinSeats.Valid {
		v := int(r.MinSeats.Int32)
		p.MinSeats = &v
	}
	if r.ArchivedAt.Valid {
		v := r.ArchivedAt.Time
		p.ArchivedAt = &v
	}
	return p, nil
}

// insertParams has the field order of db.InsertPlanIfAbsentParams and
// db.UpsertPlanParams, so one conversion serves both.
type insertParams struct {
	Key            string             `json:"key"`
	Name           string             `json:"name"`
	Limits         []byte             `json:"limits"`
	Features       []byte             `json:"features"`
	Currency       string             `json:"currency"`
	SeatPriceMonth pgtype.Int8        `json:"seat_price_month"`
	SeatPriceYear  pgtype.Int8        `json:"seat_price_year"`
	SeatsFixed     pgtype.Int4        `json:"seats_fixed"`
	MinSeats       pgtype.Int4        `json:"min_seats"`
	TrialDays      int32              `json:"trial_days"`
	IsPublic       bool               `json:"is_public"`
	Sort           int32              `json:"sort"`
	ArchivedAt     pgtype.Timestamptz `json:"archived_at"`
}

func toInsertParams(p Plan) (insertParams, error) {
	limits, features := p.Limits, p.Features
	if limits == nil {
		limits = Limits{}
	}
	if features == nil {
		features = map[string]bool{}
	}
	limitsJSON, err := json.Marshal(limits)
	if err != nil {
		return insertParams{}, err
	}
	featuresJSON, err := json.Marshal(features)
	if err != nil {
		return insertParams{}, err
	}
	out := insertParams{
		Key: p.Key, Name: p.Name, Limits: limitsJSON, Features: featuresJSON, Currency: p.Currency,
		TrialDays: int32(p.TrialDays), IsPublic: p.IsPublic, Sort: int32(p.Sort),
	}
	if p.SeatPriceMonth != nil {
		out.SeatPriceMonth = pgtype.Int8{Int64: *p.SeatPriceMonth, Valid: true}
	}
	if p.SeatPriceYear != nil {
		out.SeatPriceYear = pgtype.Int8{Int64: *p.SeatPriceYear, Valid: true}
	}
	if p.SeatsFixed != nil {
		out.SeatsFixed = pgtype.Int4{Int32: int32(*p.SeatsFixed), Valid: true}
	}
	if p.MinSeats != nil {
		out.MinSeats = pgtype.Int4{Int32: int32(*p.MinSeats), Valid: true}
	}
	if p.ArchivedAt != nil {
		out.ArchivedAt = pgtype.Timestamptz{Time: *p.ArchivedAt, Valid: true}
	}
	return out, nil
}
