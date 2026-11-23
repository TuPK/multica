package seatcapacity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/seatcapacity"
)

// The contract suite of the seat protocol: the table and the seven rules of
// spec Appendix B (docs/engineering/platform-management-spec.md).
//
// The scenarios and their expectations are data, shared with the platform:
//
//	platform-management/contract/testdata/seat_scenarios.json   hand-written
//	platform-management/contract/testdata/seat_transcript.json  the platform
//	                                                            handler's answers
//
// The platform's test (platform-management/internal/seat) runs every scenario
// on its HTTP handler, checks the expectations, and writes the transcript.
// This file runs the same calls with Delegora's own client against a server that
// replays the transcript, and checks the same expectations on what the client
// decodes. A change either side cannot read fails here or there.
//
// The stateful rules (1, 2, 4, 5, 7) need an implementation that keeps state,
// so they are proved on the platform's handler. Here they are proved as far as
// the client can see: what it sends, and what it decodes. The scripted
// stubSeatCapacity in the handler tests answers each call with one fixed
// decision and keeps no state, so it is not a subject of those rules.

const testdataDir = "../../../platform-management/contract/testdata/"

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

type step struct {
	Name      string          `json:"name"`
	World     json.RawMessage `json:"world"`
	Members   *int64          `json:"members"`
	Clock     *time.Time      `json:"clock"`
	Call      string          `json:"call"`
	Token     uuid.UUID       `json:"token"`
	ExpiresAt time.Time       `json:"expires_at"`
	Expect    expect          `json:"expect"`
}

type scenario struct {
	Name  string `json:"name"`
	Rules []int  `json:"rules"`
	Steps []step `json:"steps"`
}

type scenarioFile struct {
	Workspace uuid.UUID  `json:"workspace"`
	MemberID  uuid.UUID  `json:"member_id"`
	Scenarios []scenario `json:"scenarios"`
}

type transcriptEntry struct {
	Scenario string          `json:"scenario"`
	Step     string          `json:"step"`
	Method   string          `json:"method"`
	Suffix   string          `json:"suffix"`
	Request  json.RawMessage `json:"request"`
	Status   int             `json:"status"`
	Response json.RawMessage `json:"response"`
}

func readJSON(t *testing.T, name string, v any) {
	t.Helper()
	raw, err := os.ReadFile(testdataDir + name)
	if err != nil {
		t.Fatalf("%v (the platform writes it: cd platform-management && go test ./internal/seat -update)", err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// replay serves the recorded answers of one scenario in order, and checks that
// each request is the one the platform recorded.
type replay struct {
	t         *testing.T
	workspace uuid.UUID
	entries   []transcriptEntry
	next      int
}

func (r *replay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.t.Helper()
	if r.next >= len(r.entries) {
		r.t.Errorf("unexpected request %s %s: the transcript has no more entries", req.Method, req.URL.Path)
		http.Error(w, "no entry", http.StatusTeapot)
		return
	}
	e := r.entries[r.next]
	r.next++
	if want := "/api/v1/internal/subscriptions/" + r.workspace.String() + "/capacity/" + e.Suffix; req.Method != e.Method || req.URL.Path != want {
		r.t.Errorf("%s: client sent %s %s, want %s %s", e.Step, req.Method, req.URL.Path, e.Method, want)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		r.t.Errorf("%s: Authorization = %q, want none", e.Step, got)
	}
	body, _ := io.ReadAll(req.Body)
	if len(e.Request) == 0 {
		if len(body) != 0 {
			r.t.Errorf("%s: client sent a body %s, want none", e.Step, body)
		}
	} else {
		var got, want any
		if err := json.Unmarshal(body, &got); err != nil {
			r.t.Errorf("%s: client body %q: %v", e.Step, body, err)
		}
		_ = json.Unmarshal(e.Request, &want)
		if !reflect.DeepEqual(got, want) {
			r.t.Errorf("%s: client body = %s, want %s", e.Step, body, e.Request)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	_, _ = w.Write(e.Response)
}

func (r *replay) done() {
	r.t.Helper()
	if r.next != len(r.entries) {
		r.t.Errorf("the client made %d calls, the transcript has %d", r.next, len(r.entries))
	}
}

// invoke makes the client call a step names.
func invoke(ctx context.Context, c seatcapacity.Executor, ws, member uuid.UUID, s step) (seatcapacity.Decision, error) {
	switch s.Call {
	case "reserve":
		return c.ReserveInvitation(ctx, ws, s.Token, s.ExpiresAt)
	case "claim":
		return c.ClaimShareJoin(ctx, ws, s.Token)
	case "consume":
		return c.Consume(ctx, ws, s.Token)
	case "confirm":
		return c.Confirm(ctx, ws, s.Token, member)
	case "release":
		return c.Release(ctx, ws, s.Token)
	case "release-member":
		return c.ReleaseMember(ctx, ws, member)
	case "operation":
		return c.GetOperation(ctx, ws, s.Token)
	}
	panic("unknown call " + s.Call)
}

// The states the recovery worker handles (worker.go: recoverReserve and
// recoverConsuming). An operation in any other state fails the worker.
var workerStates = []string{"reserved", "consuming", "used", "released"}

func TestContractOnTheClient(t *testing.T) {
	var f scenarioFile
	var transcript []transcriptEntry
	readJSON(t, "seat_scenarios.json", &f)
	readJSON(t, "seat_transcript.json", &transcript)
	if len(f.Scenarios) == 0 {
		t.Fatal("no scenarios")
	}
	ctx := context.Background()
	for _, sc := range f.Scenarios {
		t.Run(sc.Name, func(t *testing.T) {
			var entries []transcriptEntry
			for _, e := range transcript {
				if e.Scenario == sc.Name {
					entries = append(entries, e)
				}
			}
			rep := &replay{t: t, workspace: f.Workspace, entries: entries}
			srv := httptest.NewServer(rep)
			defer srv.Close()
			client, err := seatcapacity.New(seatcapacity.Config{BaseURL: srv.URL})
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range sc.Steps {
				if s.Call == "" {
					continue
				}
				d, err := invoke(ctx, client, f.Workspace, f.MemberID, s)
				where := s.Name
				checkStep(t, where, s, d, err)
				checkTable(t, where, s, d, err)
			}
			rep.done()
		})
	}
}

// checkStep checks a step's expectation on what the client decoded.
func checkStep(t *testing.T, where string, s step, d seatcapacity.Decision, err error) {
	t.Helper()
	var remote *seatcapacity.HTTPError
	if seatcapacity.IsRateLimited(err) {
		t.Errorf("%s: the platform answered 429 (rule 3)", where)
	}
	e := s.Expect
	if e.Status != 0 && e.Status != http.StatusOK {
		if !errors.As(err, &remote) || remote.StatusCode != e.Status || remote.Code != e.Code {
			t.Errorf("%s: error = %v, want HTTP %d %q", where, err, e.Status, e.Code)
			return
		}
		if got := seatcapacity.IsCapacityOvercommitted(err); got != (e.Status == http.StatusConflict && e.Code == "capacity_overcommitted") {
			t.Errorf("%s: IsCapacityOvercommitted = %v", where, got) // rule 3, AC11
		}
		if got := seatcapacity.IsNotFound(err); got != (e.Status == http.StatusNotFound) {
			t.Errorf("%s: IsNotFound = %v", where, got)
		}
		return
	}
	if err != nil {
		t.Errorf("%s: %v", where, err)
		return
	}
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
	capacity := func(name string, want *int64, get func(*seatcapacity.Capacity) int64) {
		if want == nil {
			return
		}
		if d.Capacity == nil {
			t.Errorf("%s: no capacity, want %s = %d", where, name, *want)
		} else if got := get(d.Capacity); got != *want {
			t.Errorf("%s: %s = %d, want %d", where, name, got, *want)
		}
	}
	capacity("purchased", e.Purchased, func(c *seatcapacity.Capacity) int64 { return int64(c.PurchasedSeats) })
	capacity("used", e.Used, func(c *seatcapacity.Capacity) int64 { return int64(c.UsedSeats) })
	capacity("reserved", e.Reserved, func(c *seatcapacity.Capacity) int64 { return int64(c.ReservedSeats) })
	capacity("version", e.Version, func(c *seatcapacity.Capacity) int64 { return c.Version })
}

// checkTable checks what the handler and the worker rely on, per method (the
// table of Appendix B), whatever the step's own expectation says.
func checkTable(t *testing.T, where string, s step, d seatcapacity.Decision, err error) {
	t.Helper()
	if err != nil {
		return // the handler maps every other error to seat_capacity_unavailable
	}
	if d.Operation != nil && !slices.Contains(workerStates, d.Operation.State) {
		t.Errorf("%s: operation state %q is not one the recovery worker handles", where, d.Operation.State)
	}
	if !d.Managed && (d.Allowed || d.Operation != nil || d.Capacity != nil) {
		t.Errorf("%s: an unmanaged answer must be bare: %+v", where, d)
	}
	switch s.Call {
	case "reserve", "claim":
		// Any refusal other than capacity_full reads as "service unavailable".
		if d.Managed && !d.Allowed && d.Reason != "capacity_full" && d.Reason != "released" {
			t.Errorf("%s: refusal reason %q", where, d.Reason)
		}
		if d.Managed && !d.Allowed && d.Reason == "released" && d.Operation == nil {
			t.Errorf("%s: a refusal of a known token carries its operation", where)
		}
	case "consume":
		// Allowed = false makes the handler expire the invitation, so it must
		// mean the held seat is gone.
		if d.Managed && !d.Allowed && d.Reason != "released" {
			t.Errorf("%s: consume refused with %q; only a lost hold may refuse", where, d.Reason)
		}
	case "confirm":
		if d.Managed && !d.Allowed {
			t.Errorf("%s: confirm must succeed once the member exists: %+v", where, d)
		}
	case "release", "release-member":
		if !(!d.Managed || d.Allowed || d.Reason == "released") {
			t.Errorf("%s: release is not a success the handler accepts: %+v", where, d)
		}
	}
}

// Every scenario step has a recorded answer, in order, and no answer is left
// over: a scenario edited without regenerating the transcript fails here.
func TestTranscriptMatchesTheScenarios(t *testing.T) {
	var f scenarioFile
	var transcript []transcriptEntry
	readJSON(t, "seat_scenarios.json", &f)
	readJSON(t, "seat_transcript.json", &transcript)
	var want []string
	for _, sc := range f.Scenarios {
		for _, s := range sc.Steps {
			if s.Call != "" {
				want = append(want, sc.Name+"/"+s.Name)
			}
		}
	}
	var got []string
	for _, e := range transcript {
		got = append(got, e.Scenario+"/"+e.Step)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("transcript steps differ from the scenarios; run go test ./internal/seat -update in platform-management\n got: %v\nwant: %v", got, want)
	}
}

// Every rule of Appendix B is exercised by a scenario, and every route of the
// client is called by one.
func TestScenariosCoverTheRulesAndTheRoutes(t *testing.T) {
	var f scenarioFile
	readJSON(t, "seat_scenarios.json", &f)
	rules := map[int]bool{}
	calls := map[string]bool{}
	for _, sc := range f.Scenarios {
		for _, r := range sc.Rules {
			rules[r] = true
		}
		for _, s := range sc.Steps {
			calls[s.Call] = true
		}
	}
	// Rules 6 (the lock) is not visible on the wire; it is tested in the
	// platform's package and, with a database, in its db_test.go.
	for _, r := range []int{1, 2, 3, 4, 5, 7} {
		if !rules[r] {
			t.Errorf("no scenario covers rule %d", r)
		}
	}
	for _, c := range []string{"reserve", "claim", "consume", "confirm", "release", "release-member", "operation"} {
		if !calls[c] {
			t.Errorf("no scenario calls %s", c)
		}
	}
}

// The platform is unreachable: the client returns an error that is neither a
// capacity refusal, a rate limit nor a missing operation, so the handlers fall
// to their default branch (seat_capacity_unavailable, HTTP 503; spec 5.6, AC20).
func TestUnreachablePlatformIsAnUnavailableError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	client, err := seatcapacity.New(seatcapacity.Config{BaseURL: url})
	if err != nil {
		t.Fatal(err)
	}
	ws, token := uuid.New(), uuid.New()
	calls := map[string]func() (seatcapacity.Decision, error){
		"reserve": func() (seatcapacity.Decision, error) {
			return client.ReserveInvitation(context.Background(), ws, token, time.Now().Add(time.Hour))
		},
		"claim":   func() (seatcapacity.Decision, error) { return client.ClaimShareJoin(context.Background(), ws, token) },
		"consume": func() (seatcapacity.Decision, error) { return client.Consume(context.Background(), ws, token) },
	}
	for name, call := range calls {
		_, err := call()
		if err == nil {
			t.Fatalf("%s: no error from an unreachable platform", name)
		}
		if seatcapacity.IsNotFound(err) || seatcapacity.IsRateLimited(err) || seatcapacity.IsCapacityOvercommitted(err) {
			t.Errorf("%s: %v reads as a protocol answer", name, err)
		}
	}
}

// A platform 5xx is the same: neither a protocol answer nor a rate limit.
func TestPlatformServerErrorIsAnUnavailableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.Copy(w, bytes.NewBufferString(`{"error":"internal error","code":"internal"}`))
	}))
	defer srv.Close()
	client, err := seatcapacity.New(seatcapacity.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Consume(context.Background(), uuid.New(), uuid.New())
	var remote *seatcapacity.HTTPError
	if !errors.As(err, &remote) || remote.Code != "internal" || seatcapacity.IsRateLimited(err) || seatcapacity.IsNotFound(err) {
		t.Fatalf("error = %v", err)
	}
}
