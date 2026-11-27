package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// The jury panels of an outcome (durable spec 8.3): jurors vote blind, so a juror's check is
// hidden while the panel is open and shown once the panel has left the open state.
func TestOutcomeJuryPanels_VotesHiddenWhileOpen(t *testing.T) {
	o := newOutcomeFixture(t)
	criterion := o.criterion(t, "The report has a header.")
	o.wsfx.Exec(t, `INSERT INTO jury_panel (workspace_id, outcome_id, criterion_id, juror_agent_ids, diversity, state)
		VALUES ($1, $2, $3, ARRAY[$4::uuid, $5::uuid], 'provider', 'open')`,
		o.fx.ID, o.outcome, criterion, o.doer, o.verifier)
	o.wsfx.Exec(t, `INSERT INTO outcome_verification (workspace_id, outcome_id, criterion_id, verifier_type, verifier_id, result, note)
		VALUES ($1, $2, $3, 'agent', $4, 'pass', 'the header is there')`,
		o.fx.ID, o.outcome, criterion, o.verifier)

	type panelsBody struct {
		Panels []JuryPanelResponse `json:"panels"`
	}
	read := func() panelsBody {
		t.Helper()
		var out panelsBody
		req := testutil.WithURLParams(companyReq(o.fx.Member, o.fx.ID, http.MethodGet, "/api/outcomes/"+o.outcome+"/jury", nil), "id", o.outcome)
		testutil.Call(t, testHandler.ListOutcomeJuryPanels, req).Want(http.StatusOK).JSON(&out)
		return out
	}

	open := read()
	if len(open.Panels) != 1 || open.Panels[0].State != "open" || len(open.Panels[0].JurorAgentIDs) != 2 {
		t.Fatalf("open panels = %+v, want one open panel with two jurors", open.Panels)
	}
	if len(open.Panels[0].Votes) != 0 {
		t.Fatalf("open panel shows %d votes, want none while the jurors vote blind", len(open.Panels[0].Votes))
	}

	o.wsfx.Exec(t, `UPDATE jury_panel SET state = 'split' WHERE outcome_id = $1`, o.outcome)
	split := read()
	if len(split.Panels) != 1 || len(split.Panels[0].Votes) != 1 {
		t.Fatalf("split panels = %+v, want the one juror vote shown", split.Panels)
	}
	if v := split.Panels[0].Votes[0]; v.Result != "pass" || v.Note != "the header is there" {
		t.Fatalf("vote = %+v, want pass with its note", v)
	}
}
