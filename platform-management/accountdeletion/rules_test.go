package accountdeletion

import (
	"testing"
	"time"
)

var now = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

func TestWaitIsFourteenDays(t *testing.T) {
	if Wait != 14*24*time.Hour {
		t.Fatalf("Wait = %v", Wait)
	}
	if got := ScheduledAt(now); !got.Equal(now.Add(14 * 24 * time.Hour)) {
		t.Fatalf("ScheduledAt = %v", got)
	}
}

func TestRecentLogin(t *testing.T) {
	at := func(d time.Duration) (int64, bool) { return now.Add(d).Unix(), true }
	cases := []struct {
		name string
		auth func() (int64, bool)
		want bool
	}{
		{"just now", func() (int64, bool) { return at(0) }, true},
		{"nine minutes", func() (int64, bool) { return at(-9 * time.Minute) }, true},
		{"exactly ten minutes", func() (int64, bool) { return at(-10 * time.Minute) }, true},
		{"ten minutes and a second", func() (int64, bool) { return at(-10*time.Minute - time.Second) }, false},
		{"an hour", func() (int64, bool) { return at(-time.Hour) }, false},
		{"small clock skew ahead", func() (int64, bool) { return at(30 * time.Second) }, true},
		{"far in the future", func() (int64, bool) { return at(time.Hour) }, false},
		{"no auth_time", func() (int64, bool) { return 0, false }, false},
	}
	for _, c := range cases {
		auth, ok := c.auth()
		if got := RecentLogin(auth, ok, now); got != c.want {
			t.Errorf("%s: RecentLogin = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestClassifyMemberships(t *testing.T) {
	ms := []Membership{
		{WorkspaceID: "a", Name: "Solo", Role: "owner", MemberCount: 1},
		{WorkspaceID: "b", Name: "Team", Role: "owner", MemberCount: 3},
		{WorkspaceID: "c", Name: "Other", Role: "member", MemberCount: 1},
		{WorkspaceID: "d", Name: "Joined", Role: "admin", MemberCount: 4},
		{WorkspaceID: "e", Name: "Zero", Role: "owner", MemberCount: 0},
	}
	c := Classify(ms)
	if len(c.Sole) != 3 || c.Sole[0].WorkspaceID != "a" || c.Sole[1].WorkspaceID != "c" || c.Sole[2].WorkspaceID != "e" {
		t.Errorf("Sole = %+v", c.Sole)
	}
	if len(c.Blocking) != 1 || c.Blocking[0].WorkspaceID != "b" || c.Blocking[0].MemberCount != 3 {
		t.Errorf("Blocking = %+v", c.Blocking)
	}
}

func TestClassifyEmpty(t *testing.T) {
	c := Classify(nil)
	if c.Sole == nil || c.Blocking == nil {
		t.Fatal("results must be non-nil so they encode as JSON arrays")
	}
}

func TestDue(t *testing.T) {
	past := now.Add(-time.Second)
	future := now.Add(time.Second)
	cases := []struct {
		name   string
		status string
		at     *time.Time
		want   bool
	}{
		{"pending and due", StatusPending, &past, true},
		{"pending and due exactly now", StatusPending, &now, true},
		{"pending not yet due", StatusPending, &future, false},
		{"pending without a date", StatusPending, nil, false},
		{"canceled", "active", &past, false},
		{"already deleted", "deleted", &past, false},
		{"suspended", "suspended", &past, false},
	}
	for _, c := range cases {
		if got := Due(c.status, c.at, now); got != c.want {
			t.Errorf("%s: Due = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBatchSizeIsFifty(t *testing.T) {
	if BatchSize != 50 {
		t.Fatalf("BatchSize = %d", BatchSize)
	}
}

func TestDeletedEmail(t *testing.T) {
	if got := DeletedEmail("u-1"); got != "deleted+u-1@deleted.invalid" {
		t.Fatalf("DeletedEmail = %q", got)
	}
}
