package service

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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	proposalkinds "github.com/multica-ai/multica/server/internal/service/proposal_kinds"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// seatOfferEnv is an org with an owner, a squad and a runtime, and the seat offer service with
// durable_open set by the test.
type seatOfferEnv struct {
	*orgEnv
	svc     *SeatOfferService
	flags   *featureflag.Service
	runtime pgtype.UUID
	key     KeySet
	signed  []byte
}

func newSeatOfferEnv(t *testing.T, flagOn bool) seatOfferEnv {
	t.Helper()
	e := newOrgEnv(t)
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableOpen, featureflag.Rule{Default: flagOn})
	provider.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: flagOn})
	flags := featureflag.NewService(provider)
	// seat_offer has no foreign keys, so the workspace teardown leaves its rows behind.
	t.Cleanup(func() {
		if _, err := sharedTestPool(t).Exec(context.Background(), `DELETE FROM seat_offer WHERE workspace_id = $1`, e.wsID); err != nil {
			t.Errorf("remove seat offers: %v", err)
		}
	})
	priv := testKey(t, testSeed)
	// The attestation certifies the template the seat hires: the instruction template "critic".
	att := testAttestation()
	att["subject"].(map[string]any)["template_ref"] = "critic"
	doc, err := Sign(att, priv, "k1")
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	return seatOfferEnv{
		orgEnv:  e,
		svc:     &SeatOfferService{Queries: e.q, Flags: flags, Proposals: e.props},
		flags:   flags,
		runtime: mustUUID(t, e.fx.Runtime(t, "seat runtime")),
		key:     testKeySet("ocf.example.test", map[string]ed25519.PrivateKey{"k1": priv}),
		signed:  doc,
	}
}

func (e seatOfferEnv) post(t *testing.T) string {
	t.Helper()
	offer, err := e.svc.Post(context.Background(), e.ws, e.boss(), SeatOfferInput{
		Name: "Reviewer seat", Role: "reviewer", WorkClass: "flow:flow_kv_pr:review",
		SquadID: e.squad, RuntimeID: e.runtime,
		OutcomeCriteria: "Reviews pass verification", Rationale: "The squad needs a second reviewer.",
	})
	if err != nil {
		t.Fatalf("post offer: %v", err)
	}
	return util.UUIDToString(offer.ID)
}

// TestHireFromOfferFilesHireProposalWithAttestation: a hire from an open offer files an org change
// hire into the offer's squad, with the candidate's attestation, and the attestation verifies.
func TestHireFromOfferFilesHireProposalWithAttestation(t *testing.T) {
	e := newSeatOfferEnv(t, true)
	ctx := context.Background()
	offerID := mustUUID(t, e.post(t))

	p, err := e.svc.Hire(ctx, e.ws, e.boss(), offerID, "critic", e.signed)
	if err != nil {
		t.Fatalf("hire: %v", err)
	}
	if p.Kind != proposalkinds.KeyOrgChange {
		t.Fatalf("kind = %q, want %q", p.Kind, proposalkinds.KeyOrgChange)
	}
	var filed hirePayload
	if err := json.Unmarshal(p.Payload, &filed); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if filed.Type != OrgChangeHire || filed.SquadID != util.UUIDToString(e.squad) || filed.Name != "Reviewer seat" || filed.Template != "critic" {
		t.Fatalf("hire payload = %+v, want a hire of the offered seat from the critic template", filed)
	}
	if len(filed.Attestation) == 0 {
		t.Fatal("the hire proposal carries no attestation")
	}
	ok, err := SeatAttestationVerified(p.Payload, e.key)
	if err != nil || !ok {
		t.Fatalf("SeatAttestationVerified = (%v, %v), want (true, nil)", ok, err)
	}
}

// TestHiredSeatStartsFull: approving a hire proposal from a seat offer that carries a verified
// high-score attestation (and proposed budget/envelope) creates the new seat in 14-day probation,
// starting at verification mode "full" for every work class — external reputation informs the
// decision maker and does not grant verification shortcuts or alter squad envelopes (spec 13.3).
func TestHiredSeatStartsFull(t *testing.T) {
	e := newSeatOfferEnv(t, true)
	ctx := context.Background()
	budget := int64(50_000_000)
	offer, err := e.svc.Post(ctx, e.ws, e.boss(), SeatOfferInput{
		Name:                 "Verified reviewer",
		Role:                 "reviewer",
		WorkClass:            "flow:flow_kv_pr:review",
		SquadID:              e.squad,
		RuntimeID:            e.runtime,
		OutcomeCriteria:      "Reviews pass verification",
		ProposedBudgetMicros: &budget,
		ProposedBudgetPeriod: "month",
		ProposedEnvelope:     &Envelope{Floor: 1, Ceiling: 3},
		Rationale:            "Need an experienced reviewer.",
	})
	if err != nil {
		t.Fatalf("post offer: %v", err)
	}

	p, err := e.svc.Hire(ctx, e.ws, e.boss(), offer.ID, "critic", e.signed)
	if err != nil {
		t.Fatalf("hire: %v", err)
	}
	ok, err := SeatAttestationVerified(p.Payload, e.key)
	if err != nil || !ok {
		t.Fatalf("precondition: attestation must verify, got (%v, %v)", ok, err)
	}

	// Boss approves the hire proposal, executing org_change hire.
	e.approve(p.ID)

	app, err := e.q.GetOrgChangeApplicationByProposal(ctx, db.GetOrgChangeApplicationByProposalParams{
		WorkspaceID: e.ws, ProposalID: p.ID,
	})
	if err != nil {
		t.Fatalf("load org change application: %v", err)
	}
	if !app.ProbationEndsAt.Valid {
		t.Fatal("hired seat must enter 14-day probation")
	}
	newAgentID := app.SubjectID

	// The hired seat starts at verification mode "full" for every work class, including the
	// attested area, the offer's work_class and general.
	tracks := &TrackRecordService{Queries: e.q, TxStarter: sharedTestPool(t), Flags: e.flags}
	for _, wc := range []string{"flow:flow_kv_pr:review", "area:flow:flow_kv_pr", "general"} {
		mode, err := tracks.ModeFor(ctx, e.ws, newAgentID, wc)
		if err != nil {
			t.Fatalf("ModeFor(%q): %v", wc, err)
		}
		if mode != VerificationModeFull {
			t.Fatalf("ModeFor(%q) = %q, want %q", wc, mode, VerificationModeFull)
		}
	}
	if n := e.fx.Count(t, `SELECT COUNT(*) FROM track_record WHERE workspace_id = $1 AND agent_id = $2`,
		e.wsID, util.UUIDToString(newAgentID)); n != 0 {
		t.Fatalf("track_record rows for newly hired seat = %d, want 0 (external attestation does not mint local trust)", n)
	}
	// Proposed envelope on the offer is advisory only; hiring does not auto-set the squad envelope.
	if n := e.fx.Count(t, `SELECT COUNT(*) FROM squad_autonomy_envelope WHERE workspace_id = $1 AND squad_id = $2`,
		e.wsID, util.UUIDToString(e.squad)); n != 0 {
		t.Fatalf("squad_autonomy_envelope rows after hire = %d, want 0", n)
	}
}

// TestInvalidAttestationShownAsUnverified: a tampered attestation is filed, not refused, and reads
// as unverified. An attestation from a host the key set does not name reads as unverified too.
func TestInvalidAttestationShownAsUnverified(t *testing.T) {
	e := newSeatOfferEnv(t, true)
	ctx := context.Background()
	offerID := mustUUID(t, e.post(t))

	tampered := bytes.Replace(e.signed, []byte(`"n":64`), []byte(`"n":65`), 1)
	if bytes.Equal(tampered, e.signed) {
		t.Fatal("test setup: the attestation has no n:64 to tamper with")
	}
	p, err := e.svc.Hire(ctx, e.ws, e.boss(), offerID, "critic", tampered)
	if err != nil {
		t.Fatalf("hire with a tampered attestation: %v", err)
	}
	if ok, err := SeatAttestationVerified(p.Payload, e.key); err != nil || ok {
		t.Fatalf("tampered attestation = (%v, %v), want unverified", ok, err)
	}

	foreign := KeySet{Host: "other.example.test", Keys: e.key.Keys}
	if ok, err := SeatAttestationVerified(p.Payload, foreign); err != nil || ok {
		t.Fatalf("attestation against a foreign host = (%v, %v), want unverified", ok, err)
	}

	bare, err := e.svc.Hire(ctx, e.ws, e.boss(), offerID, "critic", nil)
	if err != nil {
		t.Fatalf("hire without an attestation: %v", err)
	}
	if ok, err := SeatAttestationVerified(bare.Payload, e.key); err != nil || ok {
		t.Fatalf("no attestation = (%v, %v), want unverified", ok, err)
	}
}

// TestOfferScopedToHubOrg: an offer is hired only in the workspace that posted it, and a closed
// offer is not hired at all.
func TestOfferScopedToHubOrg(t *testing.T) {
	e := newSeatOfferEnv(t, true)
	ctx := context.Background()
	offerID := mustUUID(t, e.post(t))

	otherOrg := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	if _, err := e.svc.Hire(ctx, otherOrg, e.boss(), offerID, "critic", e.signed); err != ErrSeatOfferNotFound {
		t.Fatalf("hire from another Hub org = %v, want ErrSeatOfferNotFound", err)
	}
	if _, err := e.svc.Get(ctx, otherOrg, offerID); err != ErrSeatOfferNotFound {
		t.Fatalf("get from another Hub org = %v, want ErrSeatOfferNotFound", err)
	}

	if _, err := e.svc.Close(ctx, e.ws, e.boss(), offerID); err != nil {
		t.Fatalf("close offer: %v", err)
	}
	if _, err := e.svc.Hire(ctx, e.ws, e.boss(), offerID, "critic", e.signed); err != ErrSeatOfferNotFound {
		t.Fatalf("hire from a closed offer = %v, want ErrSeatOfferNotFound", err)
	}
}

// TestSeatOffersOffWithFlag: with durable_open off nothing is posted, listed, closed or hired.
func TestSeatOffersOffWithFlag(t *testing.T) {
	e := newSeatOfferEnv(t, false)
	ctx := context.Background()
	if _, err := e.svc.Post(ctx, e.ws, e.boss(), SeatOfferInput{
		Name: "Off", SquadID: e.squad, RuntimeID: e.runtime, Rationale: "r",
	}); err != ErrSeatOffersDisabled {
		t.Fatalf("post with the flag off = %v, want ErrSeatOffersDisabled", err)
	}
	if _, err := e.svc.List(ctx, e.ws, "", 10); err != ErrSeatOffersDisabled {
		t.Fatalf("list with the flag off = %v, want ErrSeatOffersDisabled", err)
	}
	if _, err := e.svc.Hire(ctx, e.ws, e.boss(), pgtype.UUID{Bytes: uuid.New(), Valid: true}, "critic", nil); err != ErrSeatOffersDisabled {
		t.Fatalf("hire with the flag off = %v, want ErrSeatOffersDisabled", err)
	}
}

// TestMalformedAttestationRefused: bytes that are not a JSON object are refused before anything
// is filed, unlike a bad signature, which is filed as unverified.
func TestMalformedAttestationRefused(t *testing.T) {
	e := newSeatOfferEnv(t, true)
	ctx := context.Background()
	offerID := mustUUID(t, e.post(t))
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"invalid syntax", []byte("{not json")},
		{"null", []byte("null")},
		{"array", []byte("[1,2]")},
		{"string", []byte(`"attestation"`)},
		{"number", []byte("42")},
		{"whitespace", []byte("   ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.svc.Hire(ctx, e.ws, e.boss(), offerID, "critic", tc.raw); err != ErrSeatAttestationMalformed {
				t.Fatalf("malformed attestation (%s) = %v, want ErrSeatAttestationMalformed", tc.name, err)
			}
		})
	}
}

// TestSeatOfferBudgetEnvelopeAndWorkClass covers the spec 13.3 fields: proposed budget,
// proposed autonomy envelope, work class, and optional runtime_id.
func TestSeatOfferBudgetEnvelopeAndWorkClass(t *testing.T) {
	e := newSeatOfferEnv(t, true)
	ctx := context.Background()

	budget := int64(25_000_000)
	offer, err := e.svc.Post(ctx, e.ws, e.boss(), SeatOfferInput{
		Name:                 "Flexible seat",
		Role:                 "analyst",
		WorkClass:            "label:research",
		SquadID:              e.squad,
		OutcomeCriteria:      "Reports verified",
		ProposedBudgetMicros: &budget,
		ProposedBudgetPeriod: "week",
		ProposedEnvelope:     &Envelope{Floor: 0, Ceiling: 2},
		Rationale:            "Scale research capacity.",
	})
	if err != nil {
		t.Fatalf("post offer without fixed runtime: %v", err)
	}
	if offer.RuntimeID.Valid {
		t.Fatalf("expected optional runtime_id to be NULL")
	}
	if offer.WorkClass != "label:research" || !offer.ProposedBudgetMicros.Valid || offer.ProposedBudgetMicros.Int64 != budget || offer.ProposedBudgetPeriod != "week" {
		t.Fatalf("stored budget/work_class mismatch: %+v", offer)
	}
	if !offer.ProposedEnvelopeFloor.Valid || offer.ProposedEnvelopeFloor.Int32 != 0 || !offer.ProposedEnvelopeCeiling.Valid || offer.ProposedEnvelopeCeiling.Int32 != 2 {
		t.Fatalf("stored envelope mismatch: %+v", offer)
	}

	// Applying to an offer with no fixed runtime requires runtime_id on the application.
	if _, err := e.svc.Apply(ctx, e.ws, e.boss(), offer.ID, SeatOfferApplyInput{TemplateRef: "critic"}); err != ErrSeatOfferRuntimeRequired {
		t.Fatalf("apply without runtime on optional-runtime offer = %v, want ErrSeatOfferRuntimeRequired", err)
	}
	p, err := e.svc.Apply(ctx, e.ws, e.boss(), offer.ID, SeatOfferApplyInput{
		TemplateRef: "critic", RuntimeID: e.runtime, Attestation: e.signed,
	})
	if err != nil {
		t.Fatalf("apply with runtime: %v", err)
	}
	var applied hirePayload
	if err := json.Unmarshal(p.Payload, &applied); err != nil {
		t.Fatalf("unmarshal hire payload: %v", err)
	}
	if applied.Name != "Flexible seat" || applied.RuntimeID != util.UUIDToString(e.runtime) {
		t.Fatalf("applied payload = %+v, want Flexible seat with runtime %s", applied, util.UUIDToString(e.runtime))
	}

	// Validation errors for budget, envelope, and work_class length.
	neg := int64(-1)
	badCases := []struct {
		name string
		in   SeatOfferInput
		want error
	}{
		{"negative budget", SeatOfferInput{Name: "x", SquadID: e.squad, Rationale: "r", ProposedBudgetMicros: &neg, ProposedBudgetPeriod: "month"}, ErrSeatOfferBudgetInvalid},
		{"budget without period", SeatOfferInput{Name: "x", SquadID: e.squad, Rationale: "r", ProposedBudgetMicros: &budget}, ErrSeatOfferBudgetInvalid},
		{"period without budget", SeatOfferInput{Name: "x", SquadID: e.squad, Rationale: "r", ProposedBudgetPeriod: "month"}, ErrSeatOfferBudgetInvalid},
		{"invalid budget period", SeatOfferInput{Name: "x", SquadID: e.squad, Rationale: "r", ProposedBudgetMicros: &budget, ProposedBudgetPeriod: "year"}, ErrSeatOfferBudgetInvalid},
		{"envelope floor > ceiling", SeatOfferInput{Name: "x", SquadID: e.squad, Rationale: "r", ProposedEnvelope: &Envelope{Floor: 3, Ceiling: 1}}, ErrSeatOfferEnvelopeInvalid},
		{"envelope out of range", SeatOfferInput{Name: "x", SquadID: e.squad, Rationale: "r", ProposedEnvelope: &Envelope{Floor: 0, Ceiling: 4}}, ErrSeatOfferEnvelopeInvalid},
		{"work class too long", SeatOfferInput{Name: "x", SquadID: e.squad, Rationale: "r", WorkClass: strings.Repeat("a", 201)}, ErrSeatOfferInvalid},
	}
	for _, tc := range badCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.svc.Post(ctx, e.ws, e.boss(), tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("Post(%s) = %v, want %v", tc.name, err, tc.want)
			}
		})
	}
}

// TestSeatOfferOwnerOrAdminRequired verifies that posting and closing seat offers require an
// owner or admin member; regular members and agents are refused with ErrGovernanceActorNotBoss.
func TestSeatOfferOwnerOrAdminRequired(t *testing.T) {
	e := newSeatOfferEnv(t, true)
	ctx := context.Background()

	adminID := e.fx.User(t, "Admin User", "seat-offer-admin@example.test")
	e.fx.Member(t, e.wsID, adminID, "admin")
	memberID := e.fx.User(t, "Plain Member", "seat-offer-member@example.test")
	e.fx.Member(t, e.wsID, memberID, "member")

	adminActor := GovernanceActor{Type: GovernanceActorMember, ID: mustUUID(t, adminID)}
	memberActor := GovernanceActor{Type: GovernanceActorMember, ID: mustUUID(t, memberID)}
	agentActor := GovernanceActor{Type: GovernanceActorAgent, ID: e.subject}

	in := SeatOfferInput{Name: "Admin seat", SquadID: e.squad, RuntimeID: e.runtime, Rationale: "Need help."}
	if _, err := e.svc.Post(ctx, e.ws, memberActor, in); !errors.Is(err, ErrGovernanceActorNotBoss) {
		t.Fatalf("member Post = %v, want ErrGovernanceActorNotBoss", err)
	}
	if _, err := e.svc.Post(ctx, e.ws, agentActor, in); !errors.Is(err, ErrGovernanceActorNotBoss) {
		t.Fatalf("agent Post = %v, want ErrGovernanceActorNotBoss", err)
	}

	offer, err := e.svc.Post(ctx, e.ws, adminActor, in)
	if err != nil {
		t.Fatalf("admin Post failed: %v", err)
	}

	if _, err := e.svc.Close(ctx, e.ws, memberActor, offer.ID); !errors.Is(err, ErrGovernanceActorNotBoss) {
		t.Fatalf("member Close = %v, want ErrGovernanceActorNotBoss", err)
	}
	if _, err := e.svc.Close(ctx, e.ws, agentActor, offer.ID); !errors.Is(err, ErrGovernanceActorNotBoss) {
		t.Fatalf("agent Close = %v, want ErrGovernanceActorNotBoss", err)
	}
	if _, err := e.svc.Close(ctx, e.ws, adminActor, offer.ID); err != nil {
		t.Fatalf("admin Close failed: %v", err)
	}
}

// TestWellKnownKeyFetcherRemoteAndMalformed checks remote well-known public key resolution for
// seat offer attestations, local Own short-circuiting, and safe rejection of malformed or foreign
// key documents.
func TestWellKnownKeyFetcherRemoteAndMalformed(t *testing.T) {
	ctx := context.Background()
	priv := testKey(t, testSeed)
	var servedBody []byte
	var servedStatus = http.StatusOK
	var requestedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(servedStatus)
		_, _ = w.Write(servedBody)
	}))
	defer srv.Close()

	remoteHost := strings.TrimPrefix(srv.URL, "http://")
	att := testAttestation()
	att["issuer"] = map[string]any{"host": remoteHost}
	signed, err := Sign(att, priv, "k1")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	keysDoc, err := PublicKeysDocument(testKeySet(remoteHost, map[string]ed25519.PrivateKey{"k1": priv}))
	if err != nil {
		t.Fatalf("PublicKeysDocument: %v", err)
	}
	servedBody = keysDoc

	fetcher := &WellKnownKeyFetcher{
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

	payload, _ := json.Marshal(hirePayload{Type: OrgChangeHire, Template: "hub:flow-reviewer@1.2.0", Attestation: signed})
	ok, err := VerifySeatAttestation(ctx, payload, fetcher)
	if err != nil || !ok {
		t.Fatalf("VerifySeatAttestation(remote valid) = (%v, %v), want (true, nil)", ok, err)
	}
	if requestedPath != AttestationKeysPath {
		t.Fatalf("fetched path = %q, want %q", requestedPath, AttestationKeysPath)
	}

	// Malformed well-known JSON document reads as unverified.
	servedBody = []byte("{not json")
	if ok, err := VerifySeatAttestation(ctx, payload, fetcher); err != nil || ok {
		t.Fatalf("VerifySeatAttestation(malformed key doc) = (%v, %v), want (false, nil)", ok, err)
	}

	// Foreign host claimed inside well-known document reads as unverified.
	foreignDoc, _ := PublicKeysDocument(testKeySet("other.example.test", map[string]ed25519.PrivateKey{"k1": priv}))
	servedBody = foreignDoc
	if ok, err := VerifySeatAttestation(ctx, payload, fetcher); err != nil || ok {
		t.Fatalf("VerifySeatAttestation(mismatched host in key doc) = (%v, %v), want (false, nil)", ok, err)
	}

	// HTTP error from remote host reads as unverified.
	servedStatus = http.StatusNotFound
	servedBody = keysDoc
	if ok, err := VerifySeatAttestation(ctx, payload, fetcher); err != nil || ok {
		t.Fatalf("VerifySeatAttestation(remote 404) = (%v, %v), want (false, nil)", ok, err)
	}

	// Invalid host names with path/userinfo injection are refused directly.
	for _, badHost := range []string{"", "evil.test/path", "user@evil.test", "bad host"} {
		if _, err := fetcher.KeysForHost(ctx, badHost); err == nil {
			t.Fatalf("KeysForHost(%q) succeeded, want error", badHost)
		}
	}

	// Local Own key set resolves without calling Get.
	own := testKeySet("local.example.test", map[string]ed25519.PrivateKey{"k1": priv})
	localFetcher := &WellKnownKeyFetcher{
		Own: &own,
		Get: func(ctx context.Context, targetURL string, maxBytes int64) ([]byte, error) {
			t.Fatalf("unexpected network fetch for own host: %s", targetURL)
			return nil, nil
		},
	}
	localAtt := testAttestation()
	localAtt["issuer"] = map[string]any{"host": "local.example.test"}
	localSigned, _ := Sign(localAtt, priv, "k1")
	localPayload, _ := json.Marshal(hirePayload{Type: OrgChangeHire, Template: "hub:flow-reviewer@1.2.0", Attestation: localSigned})
	if ok, err := VerifySeatAttestation(ctx, localPayload, localFetcher); err != nil || !ok {
		t.Fatalf("VerifySeatAttestation(own host) = (%v, %v), want (true, nil)", ok, err)
	}
}
