package dispatch

import "testing"

// TestSquadLeaderSelfWakeSuppressed pins the leader's self-wake guard (D1). The
// request is a human stop or the leader's latest task on it decides; a comment
// the leader cannot tie to a task on this request never wakes the leader.
func TestSquadLeaderSelfWakeSuppressed(t *testing.T) {
	cases := []struct {
		name                  string
		atHumanStop           bool
		latestFound           bool
		latestIsSameSquadWork bool
		want                  bool
	}{
		{"leader task on a running request suppresses", false, true, false, true},
		{"generic task suppresses", false, true, false, true},
		// The dual-role leader reports a worker result while the request runs:
		// that wakes the leader, as before.
		{"same squad worker task wakes the leader while running", false, true, true, false},
		{"same squad worker task at a human stop suppresses", true, true, true, true},
		{"leader task at a human stop suppresses", true, true, false, true},
		{"no task on this request suppresses", false, false, false, true},
		{"no task on this request at a human stop suppresses", true, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SquadLeaderSelfWakeSuppressed(tc.atHumanStop, tc.latestFound, tc.latestIsSameSquadWork)
			if got != tc.want {
				t.Fatalf("SquadLeaderSelfWakeSuppressed(atHumanStop=%v, latestFound=%v, sameSquadWorker=%v) = %v, want %v",
					tc.atHumanStop, tc.latestFound, tc.latestIsSameSquadWork, got, tc.want)
			}
		})
	}
}
