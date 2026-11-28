package handler

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
)

type seatOfferHandlerFixture struct {
	fx      companyWorkspace
	wsfx    *testutil.Fixture
	squad   string
	runtime string
	agent   string
	priv    ed25519.PrivateKey
}

func newSeatOfferHandlerFixture(t *testing.T, flagOn bool) seatOfferHandlerFixture {
	t.Helper()
	fx := newCompanyWorkspace(t)
	wsfx := testutil.New(testPool, fx.ID, fx.Owner)
	runtime := wsfx.Runtime(t, "seat-offer-rt")
	leader := wsfx.Agent(t, "seat-offer-leader", runtime)
	squad := wsfx.Squad(t, "Seat Offer Squad", leader)
	wsfx.Cleanup(t, `DELETE FROM seat_offer WHERE workspace_id = $1`, fx.ID)
	withFeatureFlag(t, testHandler, featureflags.DurableOpen, flagOn)
	seed := []byte("delegora-handler-seat-seed-00001")
	return seatOfferHandlerFixture{
		fx:      fx,
		wsfx:    wsfx,
		squad:   squad,
		runtime: runtime,
		agent:   leader,
		priv:    ed25519.NewKeyFromSeed(seed),
	}
}

func (f seatOfferHandlerFixture) signAttestation(t *testing.T, host string) []byte {
	t.Helper()
	doc, err := service.Sign(map[string]any{
		"format":  "ocf-attestation/1",
		"subject": map[string]any{"template_ref": "critic", "seat_label": "Reviewer"},
		"issuer":  map[string]any{"host": host},
		"period":  map[string]any{"from": "2026-08-01", "to": "2026-11-01"},
		"work_classes": []map[string]any{{
			"class": "area:flow:pr", "n": 64, "pass": 60, "partial": 2, "fail": 2, "score_lower": 0.88,
			"verification": map[string]any{"full": 20, "sampled": 44, "jury": 4},
		}},
		"models":    []string{"anthropic/claude-sonnet-5-5"},
		"issued_at": "2026-11-02T10:00:00Z",
	}, f.priv, "k1")
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	return doc
}

func TestSeatOfferHandler_PostListGetCloseAndPermissions(t *testing.T) {
	f := newSeatOfferHandlerFixture(t, true)
	task := f.wsfx.Task(t, f.agent, testutil.Cols{"status": "running", "runtime_id": f.runtime})

	body := map[string]any{
		"name":             "Reviewer seat",
		"role":             "reviewer",
		"work_class":       "flow:pr:review",
		"squad_id":         f.squad,
		"runtime_id":       f.runtime,
		"outcome_criteria": "Reviews pass verification",
		"proposed_budget":  map[string]any{"amount_micros": 12000000, "period": "month"},
		"proposed_envelope": map[string]any{
			"floor_level":   1,
			"ceiling_level": 2,
		},
		"rationale": "Need a dedicated code reviewer.",
	}

	// Regular member and agent are forbidden from posting a seat offer.
	memberReq := companyReq(f.fx.Member, f.fx.ID, http.MethodPost, "/api/hub/seat-offers", body)
	testutil.Call(t, testHandler.CreateSeatOffer, memberReq).Want(http.StatusForbidden)

	agReq := agentReq(t, f.fx.ID, f.fx.Owner, f.agent, task, http.MethodPost, "/api/hub/seat-offers", body)
	testutil.Call(t, testHandler.CreateSeatOffer, agReq).Want(http.StatusForbidden)

	// Admin can post an offer.
	adminReq := companyReq(f.fx.Admin, f.fx.ID, http.MethodPost, "/api/hub/seat-offers", body)
	var created SeatOfferResponse
	testutil.Call(t, testHandler.CreateSeatOffer, adminReq).Want(http.StatusCreated).JSON(&created)
	if created.ID == "" || created.WorkClass != "flow:pr:review" || created.Status != "open" {
		t.Fatalf("created offer = %+v", created)
	}
	if created.ProposedBudget == nil || created.ProposedBudget.AmountMicros == nil || *created.ProposedBudget.AmountMicros != 12000000 || created.ProposedBudget.Period != "month" {
		t.Fatalf("proposed_budget = %+v", created.ProposedBudget)
	}
	if created.ProposedEnvelope == nil || *created.ProposedEnvelope.FloorLevel != 1 || *created.ProposedEnvelope.CeilingLevel != 2 {
		t.Fatalf("proposed_envelope = %+v", created.ProposedEnvelope)
	}

	// Regular member can list and get offers in the workspace.
	listReq := companyReq(f.fx.Member, f.fx.ID, http.MethodGet, "/api/hub/seat-offers?status=open", nil)
	var listed SeatOfferListResponse
	testutil.Call(t, testHandler.ListSeatOffers, listReq).Want(http.StatusOK).JSON(&listed)
	if len(listed.Offers) != 1 || listed.Offers[0].ID != created.ID {
		t.Fatalf("listed = %+v, want [%s]", listed.Offers, created.ID)
	}

	getReq := testutil.WithURLParams(companyReq(f.fx.Member, f.fx.ID, http.MethodGet, "/api/hub/seat-offers/"+created.ID, nil), "id", created.ID)
	var got SeatOfferResponse
	testutil.Call(t, testHandler.GetSeatOffer, getReq).Want(http.StatusOK).JSON(&got)
	if got.ID != created.ID {
		t.Fatalf("got = %+v, want %s", got, created.ID)
	}

	// Another workspace cannot see or close the offer (Hub org scoping).
	other := newCompanyWorkspace(t)
	otherGet := testutil.WithURLParams(companyReq(other.Owner, other.ID, http.MethodGet, "/api/hub/seat-offers/"+created.ID, nil), "id", created.ID)
	testutil.Call(t, testHandler.GetSeatOffer, otherGet).Want(http.StatusNotFound)

	// Regular member and agent are forbidden from closing; owner succeeds.
	memberClose := testutil.WithURLParams(companyReq(f.fx.Member, f.fx.ID, http.MethodPost, "/api/hub/seat-offers/"+created.ID+"/close", nil), "id", created.ID)
	testutil.Call(t, testHandler.CloseSeatOffer, memberClose).Want(http.StatusForbidden)

	agentClose := testutil.WithURLParams(agentReq(t, f.fx.ID, f.fx.Owner, f.agent, task, http.MethodPost, "/api/hub/seat-offers/"+created.ID+"/close", nil), "id", created.ID)
	testutil.Call(t, testHandler.CloseSeatOffer, agentClose).Want(http.StatusForbidden)

	ownerClose := testutil.WithURLParams(companyReq(f.fx.Owner, f.fx.ID, http.MethodPost, "/api/hub/seat-offers/"+created.ID+"/close", nil), "id", created.ID)
	var closed SeatOfferResponse
	testutil.Call(t, testHandler.CloseSeatOffer, ownerClose).Want(http.StatusOK).JSON(&closed)
	if closed.Status != "closed" {
		t.Fatalf("closed status = %q, want closed", closed.Status)
	}
}

func TestSeatOfferHandler_ApplyWithRemoteWellKnownAndMalformedAttestation(t *testing.T) {
	f := newSeatOfferHandlerFixture(t, true)

	// Spin up a remote well-known host publishing our Ed25519 public key.
	var keysDoc []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != service.AttestationKeysPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(keysDoc)
	}))
	defer srv.Close()

	remoteHost := strings.TrimPrefix(srv.URL, "http://")
	pub := f.priv.Public().(ed25519.PublicKey)
	var err error
	keysDoc, err = service.PublicKeysDocument(service.KeySet{
		Host: remoteHost,
		Keys: map[string]ed25519.PublicKey{"k1": pub},
	})
	if err != nil {
		t.Fatalf("PublicKeysDocument: %v", err)
	}

	prevFetcher := seatOfferKeyFetcherFor
	t.Cleanup(func() { seatOfferKeyFetcherFor = prevFetcher })
	seatOfferKeyFetcherFor = func(h *Handler) service.AttestationKeyResolver {
		return &service.WellKnownKeyFetcher{
			Scheme: "http",
			Get: func(ctx context.Context, targetURL string, maxBytes int64) ([]byte, error) {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
				if err != nil {
					return nil, err
				}
				resp, err := srv.Client().Do(req)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					return nil, errors.New(resp.Status)
				}
				return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
			},
		}
	}

	// Owner posts an offer.
	createReq := companyReq(f.fx.Owner, f.fx.ID, http.MethodPost, "/api/hub/seat-offers", map[string]any{
		"name":       "Remote candidate seat",
		"squad_id":   f.squad,
		"runtime_id": f.runtime,
		"rationale":  "Hire from remote attestation.",
	})
	var offer SeatOfferResponse
	testutil.Call(t, testHandler.CreateSeatOffer, createReq).Want(http.StatusCreated).JSON(&offer)

	// Valid attestation signed by remoteHost verifies via remote well-known keys.
	signed := f.signAttestation(t, remoteHost)
	applyReq := testutil.WithURLParams(companyReq(f.fx.Member, f.fx.ID, http.MethodPost, "/api/hub/seat-offers/"+offer.ID+"/apply", map[string]any{
		"template_ref": "critic",
		"attestation":  json.RawMessage(signed),
	}), "id", offer.ID)
	var applied ApplySeatOfferResponse
	testutil.Call(t, testHandler.ApplySeatOffer, applyReq).Want(http.StatusCreated).JSON(&applied)
	if applied.ProposalID == "" || !applied.AttestationVerified {
		t.Fatalf("valid remote attestation response = %+v, want verified=true", applied)
	}

	// Tampered attestation is filed as unverified (not refused).
	tampered := bytes.Replace(signed, []byte(`"n":64`), []byte(`"n":65`), 1)
	tamperedReq := testutil.WithURLParams(companyReq(f.fx.Member, f.fx.ID, http.MethodPost, "/api/hub/seat-offers/"+offer.ID+"/apply", map[string]any{
		"template_ref": "critic",
		"attestation":  json.RawMessage(tampered),
	}), "id", offer.ID)
	var unverified ApplySeatOfferResponse
	testutil.Call(t, testHandler.ApplySeatOffer, tamperedReq).Want(http.StatusCreated).JSON(&unverified)
	if unverified.ProposalID == "" || unverified.AttestationVerified {
		t.Fatalf("tampered attestation response = %+v, want verified=false", unverified)
	}

	// A valid signature for another template is not verified.
	mismatchReq := testutil.WithURLParams(companyReq(f.fx.Member, f.fx.ID, http.MethodPost, "/api/hub/seat-offers/"+offer.ID+"/apply", map[string]any{
		"template_ref": "code_reviewer", "attestation": json.RawMessage(signed),
	}), "id", offer.ID)
	var mismatch ApplySeatOfferResponse
	testutil.Call(t, testHandler.ApplySeatOffer, mismatchReq).Want(http.StatusCreated).JSON(&mismatch)
	if mismatch.AttestationVerified {
		t.Fatalf("mismatched template verified: %+v", mismatch)
	}

	// Malformed attestation payloads (non-JSON body, JSON array, string, null) are refused 400.
	malformedBodies := []struct {
		name string
		raw  string
	}{
		{"broken json body", `{"template_ref":"critic","attestation":{bad}`},
		{"attestation array", `{"template_ref":"critic","attestation":[1,2]}`},
		{"attestation string", `{"template_ref":"critic","attestation":"not-an-object"}`},
		{"attestation null", `{"template_ref":"critic","attestation":null}`},
		{"attestation number", `{"template_ref":"critic","attestation":42}`},
	}
	for _, tc := range malformedBodies {
		t.Run(tc.name, func(t *testing.T) {
			req := companyReq(f.fx.Member, f.fx.ID, http.MethodPost, "/api/hub/seat-offers/"+offer.ID+"/apply", nil)
			req.Body = io.NopCloser(strings.NewReader(tc.raw))
			req = testutil.WithURLParams(req, "id", offer.ID)
			testutil.Call(t, testHandler.ApplySeatOffer, req).Want(http.StatusBadRequest)
		})
	}
}

func TestSeatOfferHandler_FlagOffReturns404(t *testing.T) {
	f := newSeatOfferHandlerFixture(t, false)
	createReq := companyReq(f.fx.Owner, f.fx.ID, http.MethodPost, "/api/hub/seat-offers", map[string]any{
		"name": "Off", "squad_id": f.squad, "runtime_id": f.runtime, "rationale": "r",
	})
	testutil.Call(t, testHandler.CreateSeatOffer, createReq).Want(http.StatusNotFound)

	listReq := companyReq(f.fx.Owner, f.fx.ID, http.MethodGet, "/api/hub/seat-offers", nil)
	testutil.Call(t, testHandler.ListSeatOffers, listReq).Want(http.StatusNotFound)

	if n := f.wsfx.Count(t, `SELECT COUNT(*) FROM seat_offer WHERE workspace_id = $1`, f.fx.ID); n != 0 {
		t.Fatalf("seat_offer rows with durable_open off = %d, want 0", n)
	}
}
