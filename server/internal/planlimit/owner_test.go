package planlimit

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestParseOwnerLimit(t *testing.T) {
	for raw, want := range map[string]int{"": 0, "  ": 0, "0": 0, "2": 2, " 3 ": 3, "100": 100} {
		got, err := ParseOwnerLimit(raw)
		if err != nil || got != want {
			t.Errorf("ParseOwnerLimit(%q) = %d, %v; want %d", raw, got, err, want)
		}
	}
	for _, raw := range []string{"-1", "two", "1.5", "2 3", "99999999999999999999"} {
		if got, err := ParseOwnerLimit(raw); err == nil {
			t.Errorf("ParseOwnerLimit(%q) = %d, nil; want an error", raw, got)
		}
	}
}

// A workspace the user owns counts while it is on the free plan: no
// subscription row (""), or a row whose plan is "free".
func TestOwnerLimitCountsFreeWorkspaces(t *testing.T) {
	cases := []struct {
		name    string
		limit   int
		owned   []string
		used    int
		reached bool
	}{
		{"free owner below the limit", 2, []string{"", "free"}[:1], 1, false},
		{"free owner at the limit", 2, []string{"", "free"}, 2, true},
		{"free owner above the limit", 2, []string{"", "", "free"}, 3, true},
		{"no workspaces", 2, nil, 0, false},
		{"a paid workspace is not counted", 2, []string{"", "pro"}, 1, false},
		{"paid workspaces never reach the limit", 1, []string{"pro", "business", "enterprise"}, 0, false},
		{"paid ones beside free ones at the limit", 2, []string{"free", "pro", "", "business"}, 2, true},
		{"limit 1 with one free", 1, []string{"free"}, 1, true},
		{"empty means no limit", 0, []string{"", "", "", "free"}, 4, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			used, reached := OwnerLimitReached(tc.limit, tc.owned)
			if used != tc.used || reached != tc.reached {
				t.Errorf("OwnerLimitReached(%d, %q) = %d, %v; want %d, %v", tc.limit, tc.owned, used, reached, tc.used, tc.reached)
			}
		})
	}
}

// A negative limit never reaches: ParseOwnerLimit refuses it, but the pure rule
// must not turn a zero-value misuse into "refuse everyone".
func TestOwnerLimitZeroAndNegativeMeanNoLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		if _, reached := OwnerLimitReached(limit, []string{"", ""}); reached {
			t.Errorf("limit %d reached, want no limit", limit)
		}
	}
}

// The lock is keyed on the user, so two creations by one user queue and
// creations by different users do not.
func TestOwnerLockKeyIsPerUser(t *testing.T) {
	alice, bob := uuid.New(), uuid.New()
	lockOf := func(user uuid.UUID) string {
		var events []string
		store := &fakeStore{events: &events}
		if _, err := CheckOwnerLimit(context.Background(), store, pg(user), 2); err != nil {
			t.Fatal(err)
		}
		if len(events) == 0 {
			t.Fatal("no lock taken")
		}
		return events[0]
	}
	a1, a2, b := lockOf(alice), lockOf(alice), lockOf(bob)
	if a1 != a2 {
		t.Errorf("one user got two keys: %q and %q", a1, a2)
	}
	if a1 == b {
		t.Errorf("two users share a key: %q", a1)
	}
	if want := "lock:" + alice.String() + "|" + OwnerGate; a1 != want {
		t.Errorf("key = %q, want %q", a1, want)
	}
}

// The lock comes first: the count must see what the previous holder committed.
func TestCheckOwnerLimitLocksBeforeCounting(t *testing.T) {
	var events []string
	user := uuid.New()
	store := &fakeStore{events: &events, keys: []string{"", "free"}}
	_, err := CheckOwnerLimit(context.Background(), store, pg(user), 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"lock:" + user.String() + "|" + OwnerGate, "list"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %q, want %q", events, want)
	}
}

// No limit: no lock and no query, so an install that sets nothing is unchanged.
func TestCheckOwnerLimitWithNoLimitTouchesNothing(t *testing.T) {
	var events []string
	store := &fakeStore{events: &events, keys: []string{"", "", ""}}
	for _, limit := range []int{0, -1} {
		if _, err := CheckOwnerLimit(context.Background(), store, pg(uuid.New()), limit); err != nil {
			t.Fatal(err)
		}
	}
	if len(events) != 0 {
		t.Fatalf("events = %q, want none", events)
	}
}

func TestCheckOwnerLimitRefusesOnlyFreeOwned(t *testing.T) {
	var events []string
	user := pg(uuid.New())
	refused := func(keys ...string) error {
		_, err := CheckOwnerLimit(context.Background(), &fakeStore{events: &events, keys: keys}, user, 2)
		return err
	}
	var limit *OwnerLimitError
	if err := refused("", "free"); !errors.As(err, &limit) || limit.Limit != 2 || limit.Used != 2 {
		t.Fatalf("two free workspaces: err = %v, want OwnerLimitError used 2 limit 2", err)
	}
	if err := refused("", "pro", "business"); err != nil {
		t.Fatalf("one free and two paid: err = %v, want nil", err)
	}
}

// An owner with an unpaid usage invoice is refused before any count, even
// below the limit.
func TestCheckOwnerLimitRefusesAnIndebtedOwner(t *testing.T) {
	var events []string
	store := &fakeStore{events: &events, indebted: true}
	_, err := CheckOwnerLimit(context.Background(), store, pg(uuid.New()), 5)
	var debt *OwnerDebtError
	if !errors.As(err, &debt) {
		t.Fatalf("err = %v, want *OwnerDebtError", err)
	}
}
