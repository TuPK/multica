package pricechange

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// AC38: the price-change notice goes once to the owner of each paid workspace
// of the plan, and never to a workspace that is pending deletion.
func TestNoticeIsNotSentForAPendingDeletionWorkspace(t *testing.T) {
	st := newFakeStore()
	mail := &fakeMail{}
	s := newService(t, st, mail, nil)
	ctx := context.Background()
	if _, err := s.Schedule(ctx, admin, "pro", seatReq(in31Days, "120000", "1200000"), false, t0); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if err := s.Run(ctx, t0.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	var to []string
	for _, m := range mail.sent {
		to = append(to, m.To...)
	}
	slices.Sort(to)
	if want := []string{"owner1@example.com", "owner3@example.com"}; !slices.Equal(to, want) {
		t.Fatalf("notice recipients = %v, want exactly %v and none for the pending_deletion workspace", to, want)
	}
	for k := range st.notices {
		if strings.HasPrefix(k, ws2) {
			t.Errorf("a notice row was recorded for the pending_deletion workspace: %s", k)
		}
	}
}
