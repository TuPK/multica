package console

import (
	"strconv"
	"testing"
	"time"
)

// TestRateKeyIsTheAPIMinuteCounter pins the key the Delegora API increments once
// per workspace and minute (server/internal/middleware/workspace_rate_limit.go).
func TestRateKeyIsTheAPIMinuteCounter(t *testing.T) {
	now := time.Date(2030, 5, 6, 7, 8, 59, 0, time.UTC)
	want := "mul:rl:ws:" + secondWsID + ":" + strconv.FormatInt(now.Unix()/60, 10)
	if got := rateKey(secondWsID, now); got != want {
		t.Fatalf("rateKey = %q, want %q", got, want)
	}
	if rateKey(secondWsID, now) == rateKey(secondWsID, now.Add(time.Minute)) {
		t.Fatal("the next minute must have its own counter")
	}
}
