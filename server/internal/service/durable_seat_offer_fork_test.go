package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	proposalkinds "github.com/multica-ai/multica/server/internal/service/proposal_kinds"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// seatOfferEnv is an org with an owner, a squad and a runtime, and the seat offer service with
// durable_seats set by the test.
type seatOfferEnv struct {
	*orgEnv
	svc     *SeatOfferService
	runtime pgtype.UUID
	key     KeySet
	signed  []byte
}

func newSeatOfferEnv(t *testing.T, flagOn bool) seatOfferEnv {
	t.Helper()
	e := newOrgEnv(t)
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableSeats, featureflag.Rule{Default: flagOn})
	// seat_offer has no foreign keys, so the workspace teardown leaves its rows behind.
	t.Cleanup(func() {
		if _, err := sharedTestPool(t).Exec(context.Background(), `DELETE FROM seat_offer WHERE workspace_id = $1`, e.wsID); err != nil {
			t.Errorf("remove seat offers: %v", err)
		}
	})
	priv := testKey(t, testSeed)
	doc, err := Sign(testAttestation(), priv, "k1")
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	return seatOfferEnv{
		orgEnv:  e,
		svc:     &SeatOfferService{Queries: e.q, Flags: featureflag.NewService(provider), Proposals: e.props},
		runtime: mustUUID(t, e.fx.Runtime(t, "seat runtime")),
		key:     testKeySet("ocf.example.test", map[string]ed25519.PrivateKey{"k1": priv}),
		signed:  doc,
	}
}

func (e seatOfferEnv) post(t *testing.T) string {
	t.Helper()
	offer, err := e.svc.Post(context.Background(), e.ws, e.boss(), SeatOfferInput{
		Name: "Reviewer seat", Role: "reviewer", SquadID: e.squad, RuntimeID: e.runtime,
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

	e.fx.Exec(t, `UPDATE seat_offer SET status = 'closed' WHERE id = $1`, util.UUIDToString(offerID))
	if _, err := e.svc.Hire(ctx, e.ws, e.boss(), offerID, "critic", e.signed); err != ErrSeatOfferNotFound {
		t.Fatalf("hire from a closed offer = %v, want ErrSeatOfferNotFound", err)
	}
}

// TestSeatOffersOffWithFlag: with durable_seats off nothing is posted or hired.
func TestSeatOffersOffWithFlag(t *testing.T) {
	e := newSeatOfferEnv(t, false)
	ctx := context.Background()
	if _, err := e.svc.Post(ctx, e.ws, e.boss(), SeatOfferInput{
		Name: "Off", SquadID: e.squad, RuntimeID: e.runtime, Rationale: "r",
	}); err != ErrSeatOffersDisabled {
		t.Fatalf("post with the flag off = %v, want ErrSeatOffersDisabled", err)
	}
	if _, err := e.svc.Hire(ctx, e.ws, e.boss(), pgtype.UUID{Bytes: uuid.New(), Valid: true}, "critic", nil); err != ErrSeatOffersDisabled {
		t.Fatalf("hire with the flag off = %v, want ErrSeatOffersDisabled", err)
	}
}

// TestMalformedAttestationRefused: bytes that are not JSON are refused before anything is filed,
// unlike a bad signature, which is filed as unverified.
func TestMalformedAttestationRefused(t *testing.T) {
	e := newSeatOfferEnv(t, true)
	ctx := context.Background()
	offerID := mustUUID(t, e.post(t))
	if _, err := e.svc.Hire(ctx, e.ws, e.boss(), offerID, "critic", []byte("{not json")); err != ErrSeatAttestationMalformed {
		t.Fatalf("malformed attestation = %v, want ErrSeatAttestationMalformed", err)
	}
}
