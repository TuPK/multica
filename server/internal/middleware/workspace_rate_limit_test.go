package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/multica-ai/multica/server/internal/entitlement"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// fakeRedis is an in-memory stand-in for the INCR/EXPIRE commands the
// limiter uses. It is installed as a client hook, so no server is contacted.
type fakeRedis struct {
	mu       sync.Mutex
	counts   map[string]int64
	expiries map[string]time.Duration
	fail     error
}

func newFakeRedis(t *testing.T) (*redis.Client, *fakeRedis) {
	t.Helper()
	f := &fakeRedis{counts: map[string]int64{}, expiries: map[string]time.Duration{}}
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	rdb.AddHook(f)
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb, f
}

func (f *fakeRedis) DialHook(next redis.DialHook) redis.DialHook { return next }
func (f *fakeRedis) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (f *fakeRedis) ProcessHook(redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.fail != nil {
			cmd.SetErr(f.fail)
			return f.fail
		}
		args := cmd.Args()
		key, _ := args[1].(string)
		switch strings.ToLower(cmd.Name()) {
		case "incr":
			f.counts[key]++
			cmd.(*redis.IntCmd).SetVal(f.counts[key])
		case "expire":
			f.expiries[key] = time.Duration(args[2].(int64)) * time.Second
			cmd.(*redis.BoolCmd).SetVal(true)
		default:
			return errors.New("fakeRedis: unsupported command " + cmd.Name())
		}
		return nil
	}
}

type fakeLimits map[string]struct {
	limit    int
	enforced bool
}

func (f fakeLimits) APIRateLimit(_ context.Context, workspaceID string) (int, bool) {
	g, ok := f[workspaceID]
	if !ok {
		return 0, false
	}
	return g.limit, g.enforced
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

const (
	wsA = "11111111-1111-1111-1111-111111111111"
	wsB = "22222222-2222-2222-2222-222222222222"
)

func serveWS(h http.Handler, ws, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if ws != "" {
		req = req.WithContext(SetMemberContext(req.Context(), ws, db.Member{}))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func enforce(n int) struct {
	limit    int
	enforced bool
} {
	return struct {
		limit    int
		enforced bool
	}{n, true}
}

var testMinute = time.Date(2026, 3, 1, 12, 30, 15, 0, time.UTC)

func TestWorkspaceRateLimit_RefusesOverLimitWithRetryAfter(t *testing.T) {
	rdb, _ := newFakeRedis(t)
	clock := &fakeClock{testMinute}
	h := WorkspaceRateLimit(rdb, fakeLimits{wsA: enforce(3)}, clock.now)(okHandler)

	for i := 1; i <= 3; i++ {
		if rec := serveWS(h, wsA, "/api/issues"); rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, rec.Code)
		}
	}
	rec := serveWS(h, wsA, "/api/issues")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("4th request: got %d, want 429", rec.Code)
	}
	secs, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	if err != nil || secs < 1 || secs > 60 {
		t.Fatalf("Retry-After = %q, want 1..60", rec.Header().Get("Retry-After"))
	}
	if secs != 45 {
		t.Fatalf("Retry-After = %d, want 45 (seconds to next minute)", secs)
	}
}

func TestWorkspaceRateLimit_RetryAfterIsAtLeastOne(t *testing.T) {
	rdb, _ := newFakeRedis(t)
	clock := &fakeClock{time.Date(2026, 3, 1, 12, 30, 59, 900_000_000, time.UTC)}
	h := WorkspaceRateLimit(rdb, fakeLimits{wsA: enforce(0)}, clock.now)(okHandler)
	rec := serveWS(h, wsA, "/api/issues")
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
}

func TestWorkspaceRateLimit_WindowResetsNextMinute(t *testing.T) {
	rdb, _ := newFakeRedis(t)
	clock := &fakeClock{testMinute}
	h := WorkspaceRateLimit(rdb, fakeLimits{wsA: enforce(1)}, clock.now)(okHandler)

	serveWS(h, wsA, "/api/issues")
	if rec := serveWS(h, wsA, "/api/issues"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request in window: got %d, want 429", rec.Code)
	}
	clock.t = clock.t.Add(time.Minute)
	if rec := serveWS(h, wsA, "/api/issues"); rec.Code != http.StatusOK {
		t.Fatalf("first request in next window: got %d, want 200", rec.Code)
	}
}

func TestWorkspaceRateLimit_KeyAndExpiry(t *testing.T) {
	rdb, fake := newFakeRedis(t)
	clock := &fakeClock{testMinute}
	h := WorkspaceRateLimit(rdb, fakeLimits{wsA: enforce(5)}, clock.now)(okHandler)
	serveWS(h, wsA, "/api/issues")
	serveWS(h, wsA, "/api/issues")

	key := "mul:rl:ws:" + wsA + ":" + strconv.FormatInt(testMinute.Unix()/60, 10)
	if fake.counts[key] != 2 {
		t.Fatalf("counts = %v, want %s = 2", fake.counts, key)
	}
	if fake.expiries[key] != 2*time.Minute {
		t.Fatalf("expiry = %v, want 2m", fake.expiries[key])
	}
}

func TestWorkspaceRateLimit_WorkspacesAreIndependent(t *testing.T) {
	rdb, _ := newFakeRedis(t)
	h := WorkspaceRateLimit(rdb, fakeLimits{wsA: enforce(1), wsB: enforce(1)}, (&fakeClock{testMinute}).now)(okHandler)

	serveWS(h, wsA, "/api/issues")
	if rec := serveWS(h, wsA, "/api/issues"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("A second request: got %d, want 429", rec.Code)
	}
	if rec := serveWS(h, wsB, "/api/issues"); rec.Code != http.StatusOK {
		t.Fatalf("B first request: got %d, want 200", rec.Code)
	}
}

func TestWorkspaceRateLimit_OffOrMissingNeverRefuses(t *testing.T) {
	rdb, _ := newFakeRedis(t)
	limits := fakeLimits{wsA: {limit: 1, enforced: false}} // wsB is missing from the policy
	h := WorkspaceRateLimit(rdb, limits, (&fakeClock{testMinute}).now)(okHandler)
	for _, ws := range []string{wsA, wsB} {
		for i := 0; i < 20; i++ {
			if rec := serveWS(h, ws, "/api/issues"); rec.Code != http.StatusOK {
				t.Fatalf("%s request %d: got %d, want 200", ws, i+1, rec.Code)
			}
		}
	}
}

func TestWorkspaceRateLimit_ZeroUnderEnforceRefusesEverything(t *testing.T) {
	rdb, _ := newFakeRedis(t)
	h := WorkspaceRateLimit(rdb, fakeLimits{wsA: enforce(0)}, (&fakeClock{testMinute}).now)(okHandler)
	for i := 0; i < 3; i++ {
		if rec := serveWS(h, wsA, "/api/issues"); rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d: got %d, want 429", i+1, rec.Code)
		}
	}
}

func TestWorkspaceRateLimit_DaemonPathsAreNotCounted(t *testing.T) {
	rdb, fake := newFakeRedis(t)
	h := WorkspaceRateLimit(rdb, fakeLimits{wsA: enforce(1)}, (&fakeClock{testMinute}).now)(okHandler)

	serveWS(h, wsA, "/api/issues")
	if rec := serveWS(h, wsA, "/api/issues"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("limit not exhausted: got %d", rec.Code)
	}
	before := len(fake.counts)
	var total int64
	for _, c := range fake.counts {
		total += c
	}
	for i := 0; i < 5; i++ {
		if rec := serveWS(h, wsA, "/api/daemon/heartbeat"); rec.Code != http.StatusOK {
			t.Fatalf("daemon request %d: got %d, want 200", i+1, rec.Code)
		}
	}
	var after int64
	for _, c := range fake.counts {
		after += c
	}
	if len(fake.counts) != before || after != total {
		t.Fatalf("daemon requests were counted: %v", fake.counts)
	}
}

func TestWorkspaceRateLimit_NoWorkspacePassesThrough(t *testing.T) {
	rdb, fake := newFakeRedis(t)
	h := WorkspaceRateLimit(rdb, fakeLimits{wsA: enforce(0)}, (&fakeClock{testMinute}).now)(okHandler)
	if rec := serveWS(h, "", "/api/me"); rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if len(fake.counts) != 0 {
		t.Fatalf("request without workspace was counted: %v", fake.counts)
	}
}

func TestWorkspaceRateLimit_RedisFailureFailsOpenAndWarnsOnce(t *testing.T) {
	rdb, fake := newFakeRedis(t)
	fake.fail = errors.New("redis down")
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := WorkspaceRateLimit(rdb, fakeLimits{wsA: enforce(1)}, (&fakeClock{testMinute}).now)(okHandler)
	for i := 0; i < 5; i++ {
		if rec := serveWS(h, wsA, "/api/issues"); rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i+1, rec.Code)
		}
	}
	if n := strings.Count(buf.String(), "level=WARN"); n != 1 {
		t.Fatalf("warnings = %d, want 1\n%s", n, buf.String())
	}

	// After Redis recovers, a new failure warns again.
	fake.mu.Lock()
	fake.fail = nil
	fake.mu.Unlock()
	serveWS(h, wsA, "/api/issues")
	fake.mu.Lock()
	fake.fail = errors.New("redis down again")
	fake.mu.Unlock()
	serveWS(h, wsA, "/api/issues")
	if n := strings.Count(buf.String(), "level=WARN"); n != 2 {
		t.Fatalf("warnings after second outage = %d, want 2\n%s", n, buf.String())
	}
}

func TestWorkspaceRateLimit_ResponseBody(t *testing.T) {
	rdb, _ := newFakeRedis(t)
	h := WorkspaceRateLimit(rdb, fakeLimits{wsA: enforce(2)}, (&fakeClock{testMinute}).now)(okHandler)
	for i := 0; i < 2; i++ {
		serveWS(h, wsA, "/api/issues")
	}
	rec := serveWS(h, wsA, "/api/issues")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "rate limit exceeded" || body.Code != "rate_limited" || body.Limit != 2 {
		t.Fatalf("body = %+v", body)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After missing")
	}
}

type fakeProvider struct{ decision entitlement.Decision }

func (p fakeProvider) Gate(_ context.Context, _ uuid.UUID, name entitlement.GateName) entitlement.Decision {
	if name != entitlement.GateAPIRatePerMinute {
		return entitlement.Decision{}
	}
	return p.decision
}

func TestWorkspaceRateLimit_EntitlementLimits(t *testing.T) {
	five := 5
	cases := []struct {
		name     string
		provider entitlement.Provider
		wantN    int
		wantEnf  bool
	}{
		{"nil provider", nil, 0, false},
		{"enforce", fakeProvider{entitlement.Decision{Gate: entitlement.Gate{Action: entitlement.ActionEnforce, Limit: &five}}}, 5, true},
		{"observe", fakeProvider{entitlement.Decision{Gate: entitlement.Gate{Action: entitlement.ActionObserve, Limit: &five}}}, 0, false},
		{"off", fakeProvider{entitlement.Decision{Gate: entitlement.Gate{Action: entitlement.ActionOff}}}, 0, false},
		{"enforce without limit", fakeProvider{entitlement.Decision{Gate: entitlement.Gate{Action: entitlement.ActionEnforce}}}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := EntitlementLimits(tc.provider)
			if tc.provider == nil {
				if src != nil {
					t.Fatal("nil provider must give a nil LimitSource")
				}
				return
			}
			n, enf := src.APIRateLimit(context.Background(), wsA)
			if n != tc.wantN || enf != tc.wantEnf {
				t.Fatalf("got (%d, %v), want (%d, %v)", n, enf, tc.wantN, tc.wantEnf)
			}
		})
	}
}
