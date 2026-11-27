package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

func TestDurableFlagOffRefusesAttentionSetBudget(t *testing.T) {
	fx := newCompanyWorkspace(t)
	withFeatureFlag(t, testHandler, featureflags.DurablePrincipal, false)

	var before int32
	dbfx.QueryRow(t, `SELECT attention_minutes_per_week FROM workspace WHERE id = $1`, fx.ID).Scan(&before)

	req := companyReq(fx.Owner, fx.ID, http.MethodPut, "/api/company/attention", map[string]any{
		"minutes_per_week": 180,
		"split":            "goals",
	})
	testutil.Call(t, testHandler.SetAttentionBudget, req).Want(http.StatusServiceUnavailable)

	var after int32
	dbfx.QueryRow(t, `SELECT attention_minutes_per_week FROM workspace WHERE id = $1`, fx.ID).Scan(&after)
	if after != before {
		t.Errorf("attention_minutes_per_week changed from %d to %d with durable_principal off", before, after)
	}
}

func TestDurableFlagOffRefusesEnvelopeSetAndClear(t *testing.T) {
	o := newOutcomeFixture(t)
	withFeatureFlag(t, testHandler, featureflags.DurableOutcomes, false)
	squad := o.wsfx.Squad(t, "flag-off envelope squad", o.doer)
	o.wsfx.Cleanup(t, `DELETE FROM squad_autonomy_envelope WHERE squad_id = $1`, squad)

	body := map[string]any{"floor_level": 0, "ceiling_level": 2, "reason": "flag off test"}
	req := companyReq(o.fx.Owner, o.fx.ID, http.MethodPut, "/api/squads/"+squad+"/autonomy/envelope", body)
	testutil.Call(t, testHandler.SetSquadEnvelope, testutil.WithURLParams(req, "id", squad)).Want(http.StatusServiceUnavailable)
	if n := o.wsfx.Count(t, `SELECT count(*) FROM squad_autonomy_envelope WHERE squad_id = $1`, squad); n != 0 {
		t.Errorf("envelope rows after flag-off Set = %d, want 0", n)
	}

	o.wsfx.Exec(t, `INSERT INTO squad_autonomy_envelope (workspace_id, squad_id, floor_level, ceiling_level, set_by, reason)
		VALUES ($1, $2, 0, 2, $3, 'seeded')`, o.fx.ID, squad, o.fx.Owner)
	del := companyReq(o.fx.Owner, o.fx.ID, http.MethodDelete, "/api/squads/"+squad+"/autonomy/envelope", nil)
	testutil.Call(t, testHandler.ClearSquadEnvelope, testutil.WithURLParams(del, "id", squad)).Want(http.StatusServiceUnavailable)
	if n := o.wsfx.Count(t, `SELECT count(*) FROM squad_autonomy_envelope WHERE squad_id = $1`, squad); n != 1 {
		t.Errorf("envelope rows after flag-off Clear = %d, want 1", n)
	}
}

func TestDurableFlagOffRefusesEffectPolicySave(t *testing.T) {
	fx := newCompanyWorkspace(t)
	wsfx := testutil.New(testPool, fx.ID, fx.Owner)
	wsfx.Cleanup(t, `DELETE FROM effect_policy_override WHERE workspace_id = $1`, fx.ID)

	off := featureflag.NewStaticProvider()
	off.Set(featureflags.DurableEffects, featureflag.Rule{Default: false})
	flags := featureflag.NewService(off)
	h := *testHandler
	h.FeatureFlags = flags
	h.durable.EffectPolicy = &service.EffectPolicyStore{Queries: testHandler.Queries, TxStarter: testPool, Flags: flags}

	req := companyReq(fx.Owner, fx.ID, http.MethodPut, "/api/effects/policy", map[string]any{
		"autonomy_level": 1,
		"effect_class":   connectedapps.EffectWrite,
		"reversibility":  connectedapps.ReversibilityCompensable,
		"decision":       connectedapps.PolicyBlock,
	})
	testutil.Call(t, h.SaveEffectPolicy, req).Want(http.StatusServiceUnavailable)
	if n := wsfx.Count(t, `SELECT count(*) FROM effect_policy_override WHERE workspace_id = $1`, fx.ID); n != 0 {
		t.Errorf("effect_policy_override rows after flag-off Save = %d, want 0", n)
	}
}

func TestDurableFlagOffRefusesTrackRecordPin(t *testing.T) {
	o := newOutcomeFixture(t)
	withFeatureFlag(t, testHandler, featureflags.DurableOutcomes, false)
	o.wsfx.Cleanup(t, `DELETE FROM track_record WHERE agent_id = $1`, o.doer)

	req := companyReq(o.fx.Owner, o.fx.ID, http.MethodPut, "/api/track-records/pin", map[string]any{
		"agent_id":   o.doer,
		"work_class": "general",
		"mode":       "spot",
	})
	testutil.Call(t, testHandler.PinTrackRecord, req).Want(http.StatusServiceUnavailable)
	if n := o.wsfx.Count(t, `SELECT count(*) FROM track_record WHERE agent_id = $1`, o.doer); n != 0 {
		t.Errorf("track_record rows after flag-off Pin = %d, want 0", n)
	}
}

func TestDurableFlagOffRefusesCriteriaAndVerificationWrites(t *testing.T) {
	o := newOutcomeFixture(t)
	// Seed one criterion directly in DB so we can test UpdateCriterion with flag off too.
	seeded := o.wsfx.Insert(t, "acceptance_criterion", testutil.Cols{
		"workspace_id": o.fx.ID, "issue_id": o.issue, "position": 0,
		"text": "Original criterion", "check_kind": "manual", "check_spec": `{}`,
		"source": "member", "created_by_type": "member", "created_by_id": o.fx.Owner,
	})
	withFeatureFlag(t, testHandler, featureflags.DurableOutcomes, false)

	createReq := testutil.WithURLParams(companyReq(o.fx.Member, o.fx.ID, http.MethodPost, "/api/issues/"+o.issue+"/criteria",
		map[string]any{"text": "New criterion", "check_kind": "manual", "check_spec": map[string]any{}}), "id", o.issue)
	testutil.Call(t, testHandler.CreateIssueCriterion, createReq).Want(http.StatusServiceUnavailable)
	if n := o.wsfx.Count(t, `SELECT count(*) FROM acceptance_criterion WHERE issue_id = $1`, o.issue); n != 1 {
		t.Errorf("acceptance_criterion rows after flag-off Create = %d, want 1", n)
	}

	patchReq := testutil.WithURLParams(companyReq(o.fx.Owner, o.fx.ID, http.MethodPatch, "/api/issues/"+o.issue+"/criteria/"+seeded,
		map[string]any{"text": "Edited criterion"}), "id", o.issue, "cid", seeded)
	testutil.Call(t, testHandler.UpdateIssueCriterion, patchReq).Want(http.StatusServiceUnavailable)
	var text string
	o.wsfx.QueryRow(t, `SELECT text FROM acceptance_criterion WHERE id = $1`, seeded).Scan(&text)
	if text != "Original criterion" {
		t.Errorf("criterion text changed to %q with flag off", text)
	}

	verifyReq := testutil.WithURLParams(companyReq(o.fx.Member, o.fx.ID, http.MethodPost, "/api/outcomes/"+o.outcome+"/verifications",
		map[string]any{"result": "pass", "note": "looks good"}), "id", o.outcome)
	testutil.Call(t, testHandler.RecordOutcomeVerification, verifyReq).Want(http.StatusServiceUnavailable)
	if n := o.wsfx.Count(t, `SELECT count(*) FROM outcome_verification WHERE outcome_id = $1`, o.outcome); n != 0 {
		t.Errorf("outcome_verification rows after flag-off Record = %d, want 0", n)
	}

	// Settle the outcome directly, then test Override with flag off.
	o.wsfx.Exec(t, `UPDATE work_outcome SET settle_at = now() - interval '1 minute' WHERE id = $1`, o.outcome)
	svc := &service.OutcomeService{Queries: testHandler.Queries, TxStarter: testPool, Flags: featureflag.NewService(onFlag())}
	if _, err := svc.Settle(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	overrideReq := testutil.WithURLParams(companyReq(o.fx.Admin, o.fx.ID, http.MethodPost, "/api/outcomes/"+o.outcome+"/override",
		map[string]any{"result": "fail", "note": "bad"}), "id", o.outcome)
	testutil.Call(t, testHandler.OverrideOutcome, overrideReq).Want(http.StatusServiceUnavailable)
	var verdict string
	o.wsfx.QueryRow(t, `SELECT verdict FROM work_outcome WHERE id = $1`, o.outcome).Scan(&verdict)
	if verdict == "fail" {
		t.Errorf("settled outcome was overridden to fail with flag off")
	}
}
