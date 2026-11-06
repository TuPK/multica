package service

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/util"
)

// A blocked SDLC issue must be handed to the squad's lead, which sends it back
// to In Progress for the developer; with no handler it sits unowned.
func TestSDLCStepsHandBlockedIssueToLeadAndBackToInProgress(t *testing.T) {
	agents := map[string]pgtype.UUID{}
	for i, role := range []string{"lead", "developer", "reviewer", "qa"} {
		agents[role] = pgtype.UUID{Bytes: [16]byte{byte(i + 1)}, Valid: true}
	}
	var blocked issueworkflow.Step
	found := false
	for _, s := range sdlcSteps(agents) {
		if s.StatusKey == issuestatus.Blocked {
			blocked, found = s, true
		}
	}
	if !found {
		t.Fatal("SDLC workflow has no blocked step")
	}
	if blocked.Handler.Type != issueworkflow.HandlerAgent || blocked.Handler.ID != util.UUIDToString(agents["lead"]) {
		t.Fatalf("blocked handler = %+v, want agent lead", blocked.Handler)
	}
	if blocked.BackStatusKey != sdlcInProgressStatusKey {
		t.Fatalf("blocked back status = %q, want %q", blocked.BackStatusKey, sdlcInProgressStatusKey)
	}
	if blocked.Instructions == "" {
		t.Fatal("blocked step has no instructions for the lead")
	}
}
