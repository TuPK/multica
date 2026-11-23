package seat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// The scenarios are the contract suite of Appendix B. They are data, in
// contract/testdata/seat_scenarios.json, so Delegora's seat client runs the same
// calls and checks the same expectations (server/internal/seatcapacity/
// contract_test.go). This file runs them on the HTTP handler over a fake store
// and records the responses in seat_transcript.json, which Delegora's test
// replays.
//
//	go test ./internal/seat -update
var update = flag.Bool("update", false, "rewrite contract/testdata/seat_transcript.json from the handler's output")

const (
	scenariosFile  = "../../contract/testdata/seat_scenarios.json"
	transcriptFile = "../../contract/testdata/seat_transcript.json"
	capacityPath   = "/api/v1/internal/subscriptions/"
)

type expect struct {
	Status    int     `json:"status"`
	Code      string  `json:"code"`
	Managed   *bool   `json:"managed"`
	Allowed   *bool   `json:"allowed"`
	Reason    *string `json:"reason"`
	State     *string `json:"state"`
	Purchased *int64  `json:"purchased"`
	Used      *int64  `json:"used"`
	Reserved  *int64  `json:"reserved"`
	Version   *int64  `json:"version"`
}

type worldSpec struct {
	Subscription bool  `json:"subscription"`
	Purchased    int   `json:"purchased"`
	Members      int64 `json:"members"`
	Version      int64 `json:"version"`
	Fail         bool  `json:"fail"`
}

type step struct {
	Name      string     `json:"name"`
	World     *worldSpec `json:"world"`
	Members   *int64     `json:"members"`
	Clock     *time.Time `json:"clock"`
	Call      string     `json:"call"`
	Token     string     `json:"token"`
	ExpiresAt *time.Time `json:"expires_at"`
	MemberID  string     `json:"member_id"`
	Expect    expect     `json:"expect"`
}

type scenario struct {
	Name  string `json:"name"`
	Rules []int  `json:"rules"`
	Steps []step `json:"steps"`
}

type scenarioFile struct {
	Workspace string     `json:"workspace"`
	MemberID  string     `json:"member_id"`
	Scenarios []scenario `json:"scenarios"`
}

type transcriptEntry struct {
	Scenario string          `json:"scenario"`
	Step     string          `json:"step"`
	Method   string          `json:"method"`
	Suffix   string          `json:"suffix"`
	Request  json.RawMessage `json:"request,omitempty"`
	Status   int             `json:"status"`
	Response json.RawMessage `json:"response"`
}

func loadScenarios(t *testing.T) scenarioFile {
	t.Helper()
	raw, err := os.ReadFile(scenariosFile)
	if err != nil {
		t.Fatal(err)
	}
	var f scenarioFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("%s: %v", scenariosFile, err)
	}
	return f
}

// call returns the request the Delegora client makes for a call step: the
// method, the path after /capacity/, and the body.
func (s step) request(memberID string) (method, suffix string, body map[string]any) {
	switch s.Call {
	case "reserve":
		return http.MethodPost, "reserve", map[string]any{
			"token": s.Token, "kind": KindInvitation, "subject_id": s.Token, "expires_at": s.ExpiresAt,
		}
	case "claim":
		return http.MethodPost, "claim", map[string]any{"token": s.Token, "kind": KindShareJoin, "subject_id": s.Token}
	case "consume", "release":
		return http.MethodPost, s.Call, map[string]any{"token": s.Token}
	case "confirm":
		return http.MethodPost, "confirm", map[string]any{"token": s.Token, "member_id": memberID}
	case "release-member":
		return http.MethodPost, "release-member", map[string]any{"member_id": memberID}
	case "operation":
		return http.MethodGet, "operations/" + s.Token, nil
	}
	panic("unknown call " + s.Call)
}

// world is the fake store of one workspace.
type world struct {
	spec worldSpec
	ops  map[string]Op
	now  time.Time
}

type fakeRunner struct{ w *world }

func (r fakeRunner) InTx(_ context.Context, fn func(Tx) error) error {
	if r.w.spec.Fail {
		return errors.New("store down: password=hunter2")
	}
	// A failed call leaves nothing behind, as a rolled-back transaction does.
	saved := make(map[string]Op, len(r.w.ops))
	for k, v := range r.w.ops {
		saved[k] = v
	}
	if err := fn(&fakeTx{w: r.w}); err != nil {
		r.w.ops = saved
		return err
	}
	return nil
}

type fakeTx struct {
	w      *world
	locked bool
}

func (t *fakeTx) Lock(context.Context, string) error { t.locked = true; return nil }

func (t *fakeTx) mustBeLocked() {
	if !t.locked {
		panic("a seat query ran before the workspace lock")
	}
}

func (t *fakeTx) Subscription(_ context.Context, ws string) (subscription.Subscription, bool, error) {
	t.mustBeLocked()
	if !t.w.spec.Subscription {
		return subscription.Subscription{}, false, nil
	}
	return subscription.Subscription{
		WorkspaceID: ws, PlanKey: "team", Status: "active", Seats: t.w.spec.Purchased, Version: t.w.spec.Version,
	}, true, nil
}

func (t *fakeTx) Plan(_ context.Context, key string) (plan.Plan, bool, error) {
	return plan.Plan{Key: key}, true, nil
}

func (t *fakeTx) Members(context.Context, string) (int64, error) {
	t.mustBeLocked()
	return t.w.spec.Members, nil
}

func (t *fakeTx) Held(_ context.Context, _ string, now time.Time) (int64, error) {
	t.mustBeLocked()
	var n int64
	for _, op := range t.w.ops {
		if (op.State == StateReserved || op.State == StateConsuming) && (op.ExpiresAt == nil || op.ExpiresAt.After(now)) {
			n++
		}
	}
	return n, nil
}

func (t *fakeTx) Operation(_ context.Context, _, token string) (Op, bool, error) {
	t.mustBeLocked()
	op, ok := t.w.ops[token]
	return op, ok, nil
}

func (t *fakeTx) Insert(_ context.Context, op Op) (Op, bool, error) {
	t.mustBeLocked()
	if _, ok := t.w.ops[op.Token]; ok {
		return Op{}, false, nil
	}
	t.w.ops[op.Token] = op
	return op, true, nil
}

func (t *fakeTx) SetState(_ context.Context, _, token, state, reason string, expiresAt *time.Time) (Op, error) {
	t.mustBeLocked()
	op := t.w.ops[token]
	op.State, op.Reason, op.ExpiresAt = state, reason, expiresAt
	t.w.ops[token] = op
	return op, nil
}

func newHandler(w *world) http.Handler {
	svc := NewService(fakeRunner{w}, func() time.Time { return w.now })
	return NewHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Mux()
}

func check(t *testing.T, where string, e expect, status int, body []byte) {
	t.Helper()
	want := e.Status
	if want == 0 {
		want = http.StatusOK
	}
	if status == http.StatusTooManyRequests {
		t.Errorf("%s: status 429; the platform never answers it (rule 3)", where)
	}
	if status != want {
		t.Errorf("%s: status = %d, want %d: %s", where, status, want, body)
		return
	}
	if want != http.StatusOK {
		var errBody struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(body, &errBody)
		if errBody.Code != e.Code {
			t.Errorf("%s: code = %q, want %q", where, errBody.Code, e.Code)
		}
		return
	}
	var d Decision
	if err := json.Unmarshal(body, &d); err != nil {
		t.Errorf("%s: %v", where, err)
		return
	}
	checkDecision(t, where, e, d)
}

func checkDecision(t *testing.T, where string, e expect, d Decision) {
	t.Helper()
	if e.Managed != nil && d.Managed != *e.Managed {
		t.Errorf("%s: managed = %v, want %v", where, d.Managed, *e.Managed)
	}
	if e.Allowed != nil && d.Allowed != *e.Allowed {
		t.Errorf("%s: allowed = %v, want %v", where, d.Allowed, *e.Allowed)
	}
	if e.Reason != nil && d.Reason != *e.Reason {
		t.Errorf("%s: reason = %q, want %q", where, d.Reason, *e.Reason)
	}
	if e.State != nil && (d.Operation == nil || d.Operation.State != *e.State) {
		t.Errorf("%s: operation = %+v, want state %q", where, d.Operation, *e.State)
	}
	cap := func(name string, want *int64, get func(*Capacity) int64) {
		if want == nil {
			return
		}
		if d.Capacity == nil {
			t.Errorf("%s: no capacity, want %s = %d", where, name, *want)
		} else if got := get(d.Capacity); got != *want {
			t.Errorf("%s: %s = %d, want %d", where, name, got, *want)
		}
	}
	cap("purchased", e.Purchased, func(c *Capacity) int64 { return c.PurchasedSeats })
	cap("used", e.Used, func(c *Capacity) int64 { return c.UsedSeats })
	cap("reserved", e.Reserved, func(c *Capacity) int64 { return c.ReservedSeats })
	cap("version", e.Version, func(c *Capacity) int64 { return c.Version })
}

func compact(t *testing.T, raw []byte) json.RawMessage {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("not JSON: %s", raw)
	}
	return buf.Bytes()
}

// TestScenarios runs the contract suite on the HTTP handler and compares the
// handler's responses with seat_transcript.json (or rewrites it with -update).
func TestScenarios(t *testing.T) {
	f := loadScenarios(t)
	var transcript []transcriptEntry
	for _, sc := range f.Scenarios {
		w := &world{ops: map[string]Op{}}
		h := newHandler(w)
		for _, s := range sc.Steps {
			switch {
			case s.World != nil:
				w.spec, w.ops = *s.World, map[string]Op{}
			case s.Members != nil:
				w.spec.Members = *s.Members
			case s.Clock != nil:
				w.now = *s.Clock
			default:
				method, suffix, body := s.request(f.MemberID)
				var reqBody []byte
				if body != nil {
					reqBody, _ = json.Marshal(body)
				}
				req := httptest.NewRequest(method, capacityPath+f.Workspace+"/capacity/"+suffix, bytes.NewReader(reqBody))
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				check(t, sc.Name+" / "+s.Name, s.Expect, rec.Code, rec.Body.Bytes())
				transcript = append(transcript, transcriptEntry{
					Scenario: sc.Name, Step: s.Name, Method: method, Suffix: suffix,
					Request: reqBody, Status: rec.Code, Response: compact(t, rec.Body.Bytes()),
				})
			}
		}
	}
	got, err := json.MarshalIndent(transcript, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.WriteFile(filepath.Clean(transcriptFile), got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(transcriptFile)
	if err != nil {
		t.Fatalf("%v; run go test ./internal/seat -update", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("the handler's responses differ from %s; review the change and run go test ./internal/seat -update", transcriptFile)
	}
}
