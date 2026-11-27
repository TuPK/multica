package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestTrackRecordPinRefusedForAgent: an agent acting through its own task may not pin a
// verification mode, even though it can read the track record. The refusal is 403 and
// changes nothing.
func TestTrackRecordPinRefusedForAgent(t *testing.T) {
	o := newOutcomeFixture(t)
	task := o.wsfx.Task(t, o.doer, testutil.Cols{"issue_id": o.issue, "status": "running", "runtime_id": o.runtime})
	body := map[string]any{"agent_id": o.doer, "work_class": "general", "mode": "spot"}
	req := agentReq(t, o.fx.ID, o.fx.Owner, o.doer, task, http.MethodPut, "/api/track-records/pin", body)
	testutil.Call(t, testHandler.PinTrackRecord, req).Want(http.StatusForbidden)

	if n := o.wsfx.Count(t, `SELECT count(*) FROM track_record WHERE agent_id = $1 AND mode_pinned`, o.doer); n != 0 {
		t.Fatalf("pinned rows after an agent's refused pin = %d, want none", n)
	}
}

// TestTrackRecordPinByOwnerWritesOneEvent: an owner pins a mode; the list shows the pin,
// and the change is one verification_mode_changed event by that member.
func TestTrackRecordPinByOwnerWritesOneEvent(t *testing.T) {
	o := newOutcomeFixture(t)
	o.wsfx.Cleanup(t, `DELETE FROM track_record WHERE agent_id = $1`, o.doer)
	o.wsfx.Cleanup(t, `DELETE FROM governance_event WHERE workspace_id = $1 AND kind = 'verification_mode_changed'`, o.fx.ID)

	body := map[string]any{"agent_id": o.doer, "work_class": "general", "mode": "spot"}
	req := companyReq(o.fx.Owner, o.fx.ID, http.MethodPut, "/api/track-records/pin", body)
	var got TrackRecordResponse
	testutil.Call(t, testHandler.PinTrackRecord, req).Want(http.StatusOK).JSON(&got)
	if got.VerificationMode != "spot" || !got.ModePinned {
		t.Fatalf("pinned row = %+v, want pinned spot", got)
	}

	if n := o.wsfx.Count(t, `SELECT count(*) FROM governance_event WHERE workspace_id = $1
		AND kind = 'verification_mode_changed' AND actor_type = 'member' AND subject_id = $2`, o.fx.ID, o.doer); n != 1 {
		t.Fatalf("verification_mode_changed events = %d, want one by the member", n)
	}

	list := companyReq(o.fx.Member, o.fx.ID, http.MethodGet, "/api/track-records?agent_id="+o.doer, nil)
	var page TrackRecordList
	testutil.Call(t, testHandler.ListTrackRecords, list).Want(http.StatusOK).JSON(&page)
	if len(page.TrackRecords) != 1 || page.TrackRecords[0].VerificationMode != "spot" {
		t.Fatalf("list = %+v, want the pinned spot row", page)
	}
}

// TestTrackRecordPinRejectsUnknownMode: a mode the table does not know is a 400.
func TestTrackRecordPinRejectsUnknownMode(t *testing.T) {
	o := newOutcomeFixture(t)
	body := map[string]any{"agent_id": o.doer, "work_class": "general", "mode": "always"}
	req := companyReq(o.fx.Owner, o.fx.ID, http.MethodPut, "/api/track-records/pin", body)
	wantOutcomeCode(t, testutil.Call(t, testHandler.PinTrackRecord, req), http.StatusBadRequest, "verification_mode_invalid")
}

func TestListTrackRecordsFiltersBySquad(t *testing.T) {
	o := newOutcomeFixture(t)
	squad := o.wsfx.Squad(t, "track-record squad", o.doer)
	o.wsfx.SquadMember(t, squad, "agent", o.doer)
	o.wsfx.Cleanup(t, `DELETE FROM track_record WHERE workspace_id = $1`, o.fx.ID)

	for _, agent := range []string{o.doer, o.verifier} {
		o.wsfx.Exec(t, `INSERT INTO track_record (workspace_id, agent_id, work_class, window_days, model, verification_mode)
			VALUES ($1, $2, 'general', 30, '', 'full')`, o.fx.ID, agent)
	}

	// (a) Filtering by squad_id returns only agents belonging to that squad.
	var filtered TrackRecordList
	req := companyReq(o.fx.Member, o.fx.ID, http.MethodGet, "/api/track-records?squad_id="+squad, nil)
	testutil.Call(t, testHandler.ListTrackRecords, req).Want(http.StatusOK).JSON(&filtered)
	if len(filtered.TrackRecords) != 1 || filtered.TrackRecords[0].AgentID != o.doer {
		t.Errorf("filtered by squad_id = %+v, want only doer %s", filtered.TrackRecords, o.doer)
	}

	// (b) A squad belonging to another workspace is rejected with 404.
	other := newCompanyWorkspace(t)
	otherFx := testutil.New(testPool, other.ID, other.Owner)
	otherRuntime := otherFx.Runtime(t, "other rt")
	otherAgent := otherFx.Agent(t, "other agent", otherRuntime)
	otherSquad := otherFx.Squad(t, "other ws squad", otherAgent)
	crossReq := companyReq(o.fx.Member, o.fx.ID, http.MethodGet, "/api/track-records?squad_id="+otherSquad, nil)
	testutil.Call(t, testHandler.ListTrackRecords, crossReq).Want(http.StatusNotFound)

	// (c) Without squad_id, both agents in the workspace are returned.
	var all TrackRecordList
	allReq := companyReq(o.fx.Member, o.fx.ID, http.MethodGet, "/api/track-records", nil)
	testutil.Call(t, testHandler.ListTrackRecords, allReq).Want(http.StatusOK).JSON(&all)
	if len(all.TrackRecords) != 2 {
		t.Errorf("unfiltered track records = %d, want 2", len(all.TrackRecords))
	}
}
