package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// freshSquadLeader makes an agent in the fixture's workspace to lead a squad of its own. An agent leads one live
// squad at a time (squad_live_leader_uidx), so every squad after the first needs a leader that no other squad has.
func freshSquadLeader(t *testing.T, f *testutil.Fixture, name string) string {
	t.Helper()
	return f.Agent(t, name+" leader "+uuid.NewString()[:8], "")
}
