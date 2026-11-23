package seat

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	ws    = "11111111-aaaa-4aaa-8aaa-111111111111"
	tok   = "00000000-0000-4000-8000-000000000001"
	other = "00000000-0000-4000-8000-000000000002"
)

func newWorld(purchased int, members int64) *world {
	return &world{
		spec: worldSpec{Subscription: true, Purchased: purchased, Members: members, Version: 3},
		ops:  map[string]Op{},
		now:  time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC),
	}
}

func do(h http.Handler, method, suffix, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, capacityPath+ws+"/capacity/"+suffix, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The route set is exactly the suffixes the client builds.
func TestRoutes(t *testing.T) {
	h := newHandler(newWorld(3, 1))
	body := `{"token":"` + tok + `","kind":"invitation","subject_id":"` + tok + `","expires_at":"2026-08-15T11:00:00Z","member_id":"` + other + `"}`
	for _, c := range []struct{ method, suffix string }{
		{"POST", "reserve"}, {"POST", "claim"}, {"POST", "consume"}, {"POST", "confirm"},
		{"POST", "release"}, {"POST", "release-member"}, {"GET", "operations/" + tok},
	} {
		rec := do(h, c.method, c.suffix, body)
		if rec.Code == http.StatusNotFound && !strings.Contains(rec.Body.String(), CodeNotFound) || rec.Code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d: %s", c.method, c.suffix, rec.Code, rec.Body)
		}
	}
	if rec := do(h, "GET", "reserve", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET reserve = %d, want 405", rec.Code)
	}
	if len(Patterns) != 7 {
		t.Errorf("Patterns = %d, want 7", len(Patterns))
	}
}

func TestInvalidRequestsAreRejectedBeforeTheStore(t *testing.T) {
	w := newWorld(3, 1)
	w.spec.Fail = true // any store access would be a 500
	h := newHandler(w)
	for name, c := range map[string]struct{ suffix, body string }{
		"not json":            {"reserve", `nope`},
		"token missing":       {"consume", `{}`},
		"token not a uuid":    {"release", `{"token":"x"}`},
		"reserve no expiry":   {"reserve", `{"token":"` + tok + `","kind":"invitation","subject_id":"` + tok + `"}`},
		"reserve wrong kind":  {"reserve", `{"token":"` + tok + `","kind":"share_join","subject_id":"` + tok + `","expires_at":"2026-08-15T11:00:00Z"}`},
		"claim wrong kind":    {"claim", `{"token":"` + tok + `","kind":"invitation","subject_id":"` + tok + `"}`},
		"confirm no member":   {"confirm", `{"token":"` + tok + `"}`},
		"release-member none": {"release-member", `{}`},
	} {
		rec := do(h, "POST", c.suffix, c.body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), CodeInvalid) {
			t.Errorf("%s: %d %s, want 400 %s", name, rec.Code, rec.Body, CodeInvalid)
		}
	}
	if rec := do(h, "GET", "operations/zzz", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad token in the path = %d, want 400", rec.Code)
	}
	req := httptest.NewRequest("POST", capacityPath+"not-a-uuid/capacity/release", strings.NewReader(`{"token":"`+tok+`"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad workspace = %d, want 400 (a 404 would read as a released seat)", rec.Code)
	}
}

// A store failure is a 500 with a stable code, and the body does not carry the
// cause.
func TestStoreFailureIsA500WithAStableCode(t *testing.T) {
	w := newWorld(3, 1)
	w.spec.Fail = true
	rec := do(newHandler(w), "POST", "release", `{"token":"`+tok+`"}`)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"code":"internal"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Errorf("the body leaks the cause: %s", rec.Body)
	}
}

// Nothing is read or written before the workspace lock (rule 6): the fake
// store panics on a query made before Lock.
func TestEveryCallTakesTheLockFirst(t *testing.T) {
	h := newHandler(newWorld(3, 1))
	body := `{"token":"` + tok + `","kind":"invitation","subject_id":"` + tok + `","expires_at":"2026-08-15T11:00:00Z","member_id":"` + other + `"}`
	for _, c := range []struct{ method, suffix string }{
		{"POST", "reserve"}, {"POST", "consume"}, {"POST", "confirm"}, {"POST", "release"},
		{"POST", "release-member"}, {"GET", "operations/" + tok},
	} {
		do(h, c.method, c.suffix, body) // a panic fails the test
	}
}

// A reservation refused as full writes no row.
func TestARefusedReservationHoldsNothing(t *testing.T) {
	w := newWorld(1, 1)
	h := newHandler(w)
	rec := do(h, "POST", "reserve", `{"token":"`+tok+`","kind":"invitation","subject_id":"`+tok+`","expires_at":"2026-08-15T11:00:00Z"}`)
	if !strings.Contains(rec.Body.String(), `"reason":"capacity_full"`) || len(w.ops) != 0 {
		t.Fatalf("%s, %d operations", rec.Body, len(w.ops))
	}
}

// Rule 6: the platform's lock key space differs from Delegora's WorkspaceLocker.
// The test reads both: Delegora's from its source, which cannot be imported from
// this module, and the platform's from LockKey and its query.
func TestLockKeySpaceDiffersFromWorkspaceLocker(t *testing.T) {
	src, err := os.ReadFile("../../../server/internal/seatcapacity/workspace_locker.go")
	if err != nil {
		t.Fatal(err)
	}
	prefix := regexp.MustCompile(`append\(\[\]byte\("([^"]+)"\), workspaceID\[:\]\.\.\.\)`).FindSubmatch(src)
	if prefix == nil {
		t.Fatal("Delegora's lock key derivation was not found in workspace_locker.go; update this test with it")
	}
	if !bytes.Contains(src, []byte("pg_try_advisory_lock($1)")) {
		t.Fatal("Delegora's lock is no longer the one-key session lock; recheck rule 6")
	}
	query, err := os.ReadFile("../../queries/seat.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(query), "pg_advisory_xact_lock($1::int, $2::int)") {
		t.Error("the platform's lock is not the two-key transaction lock")
	}
	if LockNamespace == 0 {
		t.Error("LockNamespace is 0")
	}
	if lockKeyPrefix == string(prefix[1]) {
		t.Errorf("the hash prefix equals Delegora's %q", prefix[1])
	}
	for _, id := range []string{ws, tok, other} {
		raw := uuidBytes(id)
		sum := sha256.Sum256(append(append([]byte{}, prefix[1]...), raw...))
		multica := int64(binary.BigEndian.Uint64(sum[:8]))
		ns, key := LockKey(id)
		if packed := int64(ns)<<32 | int64(uint32(key)); packed == multica {
			t.Errorf("the platform's lock for %s equals Delegora's %d", id, multica)
		}
		if ns2, key2 := LockKey(id); ns2 != ns || key2 != key {
			t.Error("LockKey is not deterministic")
		}
	}
}

// uuidBytes returns the 16 bytes of a UUID in its dashed text form.
func uuidBytes(id string) []byte {
	hex := strings.NewReplacer("-", "").Replace(id)
	raw := make([]byte, 0, 16)
	for i := 0; i < len(hex); i += 2 {
		var b byte
		for _, c := range hex[i : i+2] {
			b = b<<4 | byte(strings.IndexRune("0123456789abcdef", c))
		}
		raw = append(raw, b)
	}
	return raw
}
