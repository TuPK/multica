package paymentnotice

import "testing"

// TestNoticeStateTransitions pins the whole state matrix. A notice moves
// received to applied or needs_review, and needs_review to resolved or
// dismissed. Nothing moves backwards, and applied, resolved and dismissed are
// terminal.
func TestNoticeStateTransitions(t *testing.T) {
	allowed := map[[2]State]bool{
		{StateReceived, StateApplied}:      true,
		{StateReceived, StateNeedsReview}:  true,
		{StateNeedsReview, StateResolved}:  true,
		{StateNeedsReview, StateDismissed}: true,
	}
	all := []State{StateReceived, StateApplied, StateNeedsReview, StateResolved, StateDismissed}
	for _, from := range all {
		for _, to := range all {
			want := allowed[[2]State{from, to}]
			if got := from.CanMoveTo(to); got != want {
				t.Errorf("%s -> %s: CanMoveTo = %v, want %v", from, to, got, want)
			}
		}
	}
}
