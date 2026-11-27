package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// TestPrincipalProfileRoutes covers the principal profile over HTTP (durable spec 6.6, 14.4): the
// routes answer 404 while durable_principal is off, except the person's own delete; consent is
// required; and a trait that cites nothing the person wrote is refused with 422.
func TestPrincipalProfileRoutes(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	previous := testHandler.FeatureFlags
	t.Cleanup(func() {
		testHandler.FeatureFlags = previous
		testPool.Exec(context.Background(), `DELETE FROM principal_trait WHERE workspace_id = $1`, testWorkspaceID)
		testPool.Exec(context.Background(), `DELETE FROM principal_profile WHERE workspace_id = $1`, testWorkspaceID)
		testPool.Exec(context.Background(), `DELETE FROM governance_event WHERE workspace_id = $1 AND kind LIKE 'principal_%'`, testWorkspaceID)
	})
	flags := func(on bool) *featureflag.Service {
		p := featureflag.NewStaticProvider()
		p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: on})
		return featureflag.NewService(p)
	}
	profilePath := "/api/principal/profile"

	t.Run("off: reads and writes answer 404", func(t *testing.T) {
		testHandler.FeatureFlags = flags(false)
		testutil.Call(t, testHandler.GetPrincipalProfile, newRequest("GET", profilePath, nil)).Want(http.StatusNotFound)
		testutil.Call(t, testHandler.EnablePrincipalProfile, newRequest("PUT", profilePath,
			map[string]any{"consent_text": "Tôi đồng ý"})).Want(http.StatusNotFound)
		testutil.Call(t, testHandler.AddPrincipalTrait, newRequest("POST", profilePath+"/traits", map[string]any{})).Want(http.StatusNotFound)
	})

	t.Run("on: consent is required to turn the profile on", func(t *testing.T) {
		testHandler.FeatureFlags = flags(true)
		wantOutcomeCode(t, testutil.Call(t, testHandler.EnablePrincipalProfile,
			newRequest("PUT", profilePath, map[string]any{"consent_text": "  "})),
			http.StatusUnprocessableEntity, "profile_consent_required")
		testutil.Call(t, testHandler.EnablePrincipalProfile,
			newRequest("PUT", profilePath, map[string]any{"consent_text": "Tôi đồng ý", "share_with_owners": false})).
			Want(http.StatusOK)
	})

	t.Run("on: a trait citing nothing of the person's is refused with 422", func(t *testing.T) {
		body := map[string]any{
			"decision_kind": "general",
			"statement":     "Chọn nhanh khi việc nhỏ",
			"origin":        "edited",
			"evidence":      []map[string]any{{"type": "comment", "ref": "00000000-0000-0000-0000-000000000001"}},
		}
		wantOutcomeCode(t, testutil.Call(t, testHandler.AddPrincipalTrait, newRequest("POST", profilePath+"/traits", body)),
			http.StatusUnprocessableEntity, "principal_evidence_invalid")
	})

	t.Run("on: the profile reads back with no traits yet", func(t *testing.T) {
		var got struct {
			Status string           `json:"status"`
			Traits []map[string]any `json:"traits"`
		}
		testutil.Call(t, testHandler.GetPrincipalProfile, newRequest("GET", profilePath, nil)).Want(http.StatusOK).JSON(&got)
		if got.Status != "active" || len(got.Traits) != 0 {
			t.Fatalf("profile = %q with %d traits, want active with none", got.Status, len(got.Traits))
		}
	})

	t.Run("delete works with the flag off", func(t *testing.T) {
		testHandler.FeatureFlags = flags(false)
		testutil.Call(t, testHandler.DeletePrincipalProfile, newRequest("DELETE", profilePath, nil)).Want(http.StatusNoContent)
		testHandler.FeatureFlags = flags(true)
		testutil.Call(t, testHandler.GetPrincipalProfile, newRequest("GET", profilePath, nil)).Want(http.StatusNotFound)
	})
}

// TestPrincipalReadRoutes covers the reads the principal pages make (durable spec 6.5, 6.6, 7.5) and
// the interview answer over HTTP: every route answers 404 while durable_principal is off; a person
// reads only their own predictions, delegations and decisions; an answer is written into the chat and
// cited by its trait.
func TestPrincipalReadRoutes(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	previous := testHandler.FeatureFlags
	t.Cleanup(func() {
		testHandler.FeatureFlags = previous
		for _, q := range []string{
			`DELETE FROM principal_trait WHERE workspace_id = $1`,
			`DELETE FROM principal_profile WHERE workspace_id = $1`,
			`DELETE FROM principal_delegation WHERE workspace_id = $1`,
			`DELETE FROM decision_prediction WHERE workspace_id = $1`,
			`DELETE FROM governance_event WHERE workspace_id = $1 AND kind LIKE 'principal_%'`,
		} {
			testPool.Exec(context.Background(), q, testWorkspaceID)
		}
	})
	flags := func(on bool) *featureflag.Service {
		p := featureflag.NewStaticProvider()
		p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: on})
		return featureflag.NewService(p)
	}

	t.Run("off: every read answers 404", func(t *testing.T) {
		testHandler.FeatureFlags = flags(false)
		subject := uuid.NewString()
		testutil.Call(t, testHandler.GetPrincipalDelegations, newRequest("GET", "/api/principal/delegations", nil)).Want(http.StatusNotFound)
		testutil.Call(t, testHandler.GetPrincipalPrediction, newRequest("GET",
			"/api/principal/predictions?subject_type=proposal&subject_id="+subject, nil)).Want(http.StatusNotFound)
		testutil.Call(t, testHandler.ListPrincipalEvidenceCandidates, newRequest("GET", "/api/principal/evidence-candidates", nil)).Want(http.StatusNotFound)
		testutil.Call(t, testHandler.ListQueuedAttention, newRequest("GET", "/api/company/attention/queued", nil)).Want(http.StatusNotFound)
	})

	testHandler.FeatureFlags = flags(true)
	testutil.Call(t, testHandler.EnablePrincipalProfile,
		newRequest("PUT", "/api/principal/profile", map[string]any{"consent_text": "Tôi đồng ý"})).Want(http.StatusOK)

	t.Run("prediction: a made prediction reads back; an unknown question is 404; a bad subject type is 400", func(t *testing.T) {
		subject := uuid.NewString()
		dbfx.Exec(t, `INSERT INTO decision_prediction (workspace_id, member_id, subject_type, subject_id, decision_kind, predicted, confidence, why)
			VALUES ($1, $2, 'proposal', $3, 'general', 'approve', 0.92, 'Bạn duyệt nhanh việc nhỏ')`, testWorkspaceID, testUserID, subject)
		var got struct {
			Predicted  string  `json:"predicted"`
			Confidence float64 `json:"confidence"`
			Why        string  `json:"why"`
			Actual     *string `json:"actual"`
			Matched    *bool   `json:"matched"`
		}
		testutil.Call(t, testHandler.GetPrincipalPrediction, newRequest("GET",
			"/api/principal/predictions?subject_type=proposal&subject_id="+subject, nil)).Want(http.StatusOK).JSON(&got)
		if got.Predicted != "approve" || got.Confidence != 0.92 || got.Why == "" || got.Actual != nil || got.Matched != nil {
			t.Fatalf("prediction = %+v, want approve at 0.92 with no outcome yet", got)
		}
		testutil.Call(t, testHandler.GetPrincipalPrediction, newRequest("GET",
			"/api/principal/predictions?subject_type=proposal&subject_id="+uuid.NewString(), nil)).Want(http.StatusNotFound)
		testutil.Call(t, testHandler.GetPrincipalPrediction, newRequest("GET",
			"/api/principal/predictions?subject_type=budget&subject_id="+subject, nil)).Want(http.StatusBadRequest)
	})

	t.Run("delegations: twenty matched predictions make a kind eligible, and a delegation reads back", func(t *testing.T) {
		dbfx.Exec(t, `INSERT INTO decision_prediction (workspace_id, member_id, subject_type, subject_id, decision_kind,
				predicted, confidence, actual, matched, resolved_at)
			SELECT $1, $2, 'proposal', gen_random_uuid(), 'general', 'approve', 0.9, 'approve', true, now()
			FROM generate_series(1, 20)`, testWorkspaceID, testUserID)
		var got struct {
			Delegations []struct {
				DecisionKind  string  `json:"decision_kind"`
				Enabled       bool    `json:"enabled"`
				MinConfidence float64 `json:"min_confidence"`
			} `json:"delegations"`
			Kinds []principalKindAccuracy `json:"kinds"`
		}
		testutil.Call(t, testHandler.GetPrincipalDelegations, newRequest("GET", "/api/principal/delegations", nil)).Want(http.StatusOK).JSON(&got)
		general, ok := findKind(got.Kinds, "general")
		if !ok || general.Matched != 20 || general.N != 20 || !general.Eligible {
			t.Fatalf("general kind = %+v (found %v), want 20 of 20 matched and eligible", general, ok)
		}
		if len(got.Delegations) != 0 {
			t.Fatalf("%d delegations before any was turned on, want none", len(got.Delegations))
		}
		testutil.Call(t, testHandler.EnablePrincipalDelegation, newRequest("PUT", "/api/principal/delegations",
			map[string]any{"decision_kind": "general", "min_confidence": 0.9})).Want(http.StatusOK)
		testutil.Call(t, testHandler.GetPrincipalDelegations, newRequest("GET", "/api/principal/delegations", nil)).Want(http.StatusOK).JSON(&got)
		if len(got.Delegations) != 1 || !got.Delegations[0].Enabled || got.Delegations[0].DecisionKind != "general" {
			t.Fatalf("delegations = %+v, want general on", got.Delegations)
		}
	})

	t.Run("evidence: the person's recent decisions are the candidates", func(t *testing.T) {
		issue := dbfx.Issue(t, "evidence candidate "+uuid.NewString()[:8])
		proposal := dbfx.Insert(t, "proposal", testutil.Cols{
			"workspace_id": testWorkspaceID, "squad_id": uuid.NewString(), "issue_id": issue,
			"state": "approved", "created_by_type": "member", "created_by_id": testUserID,
			"decided_by": testUserID, "decided_at": time.Now(),
		})
		var got struct {
			Decisions []struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Decision string `json:"decision"`
			} `json:"decisions"`
		}
		testutil.Call(t, testHandler.ListPrincipalEvidenceCandidates, newRequest("GET", "/api/principal/evidence-candidates", nil)).Want(http.StatusOK).JSON(&got)
		found := false
		for _, d := range got.Decisions {
			if d.ID == proposal {
				found = d.Decision == "approved" && d.Title != ""
			}
		}
		if !found {
			t.Fatalf("decision %s missing from the candidates: %+v", proposal, got.Decisions)
		}
	})

	t.Run("interview: an empty answer is 422; no Secretary is 409; an answer is written to the chat and cited", func(t *testing.T) {
		dbfx.Exec(t, `UPDATE workspace SET secretary_agent_id = NULL WHERE id = $1`, testWorkspaceID)
		answer := newRequest("POST", "/api/principal/interview/answers", map[string]any{"question": 0, "answer": "Một câu trả lời"})
		wantOutcomeCode(t, testutil.Call(t, testHandler.AnswerPrincipalInterview, answer), http.StatusConflict, "principal_no_secretary")

		runtime := handlerTestRuntimeID(t)
		secretary := dbfx.Agent(t, "Secretary "+uuid.NewString(), runtime, testutil.Cols{"workspace_id": testWorkspaceID})
		dbfx.Exec(t, `UPDATE workspace SET secretary_agent_id = $1 WHERE id = $2`, secretary, testWorkspaceID)
		t.Cleanup(func() {
			dbfx.Exec(t, `UPDATE workspace SET secretary_agent_id = NULL WHERE id = $1`, testWorkspaceID)
		})
		dbfx.Cleanup(t, `DELETE FROM chat_session WHERE workspace_id = $1 AND creator_id = $2`, testWorkspaceID, testUserID)
		dbfx.Cleanup(t, `DELETE FROM chat_message WHERE chat_session_id IN (SELECT id FROM chat_session WHERE workspace_id = $1 AND creator_id = $2)`, testWorkspaceID, testUserID)

		wantOutcomeCode(t, testutil.Call(t, testHandler.AnswerPrincipalInterview,
			newRequest("POST", "/api/principal/interview/answers", map[string]any{"question": 0, "answer": "   "})),
			http.StatusUnprocessableEntity, "principal_trait_invalid")

		var trait struct {
			Origin   string `json:"origin"`
			Evidence []struct {
				Type string `json:"type"`
				Ref  string `json:"ref"`
			} `json:"evidence"`
		}
		testutil.Call(t, testHandler.AnswerPrincipalInterview,
			newRequest("POST", "/api/principal/interview/answers", map[string]any{"question": 0, "answer": "Chậm mà chắc"})).Want(http.StatusCreated).JSON(&trait)
		if trait.Origin != "interview" || len(trait.Evidence) != 1 || trait.Evidence[0].Type != "interview" {
			t.Fatalf("trait = %+v, want an interview trait citing one interview message", trait)
		}
		if n := dbfx.Count(t, `SELECT COUNT(*) FROM chat_message m JOIN chat_session s ON s.id = m.chat_session_id
			WHERE s.workspace_id = $1 AND s.creator_id = $2 AND m.role = 'user' AND m.content = $3 AND m.id = $4`,
			testWorkspaceID, testUserID, "Chậm mà chắc", trait.Evidence[0].Ref); n != 1 {
			t.Fatalf("%d answer messages match the cited one, want 1", n)
		}
	})

	t.Run("queued: the person sees the questions addressed to them; an owner also sees the unaddressed ones", func(t *testing.T) {
		var role string
		dbfx.QueryRow(t, `SELECT role FROM member WHERE workspace_id = $1 AND user_id = $2`, testWorkspaceID, testUserID).Scan(&role)
		ownAsk, unaddressed, otherAsk := uuid.NewString(), uuid.NewString(), uuid.NewString()
		week := time.Now().UTC().Truncate(24 * time.Hour)
		for _, row := range []struct {
			subject string
			member  any
		}{{ownAsk, testUserID}, {unaddressed, nil}, {otherAsk, uuid.NewString()}} {
			dbfx.Insert(t, "attention_ledger", testutil.Cols{
				"workspace_id": testWorkspaceID, "member_id": row.member, "subject_type": "proposal",
				"subject_id": row.subject, "reason": "judgment", "estimated_minutes": 2.0,
				"state": "queued", "week_start": week,
			})
		}
		var got struct {
			Asks []struct {
				SubjectID string `json:"subject_id"`
				Reason    string `json:"reason"`
			} `json:"asks"`
		}
		testutil.Call(t, testHandler.ListQueuedAttention, newRequest("GET", "/api/company/attention/queued", nil)).Want(http.StatusOK).JSON(&got)
		seen := map[string]bool{}
		for _, a := range got.Asks {
			seen[a.SubjectID] = true
		}
		if !seen[ownAsk] || seen[otherAsk] {
			t.Fatalf("queued asks = %+v, want the own ask and not another person's", got.Asks)
		}
		owner := role == "owner" || role == "admin"
		if seen[unaddressed] != owner {
			t.Fatalf("unaddressed ask visible = %v for role %q, want %v", seen[unaddressed], role, owner)
		}
	})
}

// principalKindAccuracy is one kind of the delegation panel's accuracy list, as the route writes it.
type principalKindAccuracy struct {
	DecisionKind string  `json:"decision_kind"`
	Matched      int     `json:"matched"`
	N            int     `json:"n"`
	Lower        float64 `json:"lower"`
	Eligible     bool    `json:"eligible"`
}

func findKind(kinds []principalKindAccuracy, name string) (principalKindAccuracy, bool) {
	for _, k := range kinds {
		if k.DecisionKind == name {
			return k, true
		}
	}
	return principalKindAccuracy{}, false
}
