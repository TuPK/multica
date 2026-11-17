package subscription

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

const profileActor = "aaaaaaaa-0000-0000-0000-000000000004"

// fakeProfiles is the tables an account kind change touches, with transaction
// semantics: a failing fn leaves nothing behind.
type fakeProfiles struct {
	workspaces map[string]string
	plans      map[string]plan.Plan
	subs       map[string]Subscription
	profiles   map[string]BillingProfile
	audits     []audit.Entry
	txs        int
}

func newFakeProfiles(t *testing.T) *fakeProfiles {
	return &fakeProfiles{
		workspaces: map[string]string{wsID: wsName},
		plans:      catalogue(t),
		subs:       map[string]Subscription{},
		profiles:   map[string]BillingProfile{},
	}
}

func (s *fakeProfiles) InTx(_ context.Context, fn func(ProfileTx) error) error {
	s.txs++
	profiles := map[string]BillingProfile{}
	for k, v := range s.profiles {
		profiles[k] = v
	}
	audits := append([]audit.Entry(nil), s.audits...)
	if err := fn(&fakeProfileTx{s: s}); err != nil {
		s.profiles, s.audits = profiles, audits
		return err
	}
	return nil
}

// Profile makes fakeProfiles a KindReader as well.
func (s *fakeProfiles) Profile(_ context.Context, ws string) (BillingProfile, bool, error) {
	p, ok := s.profiles[ws]
	return p, ok, nil
}

type fakeProfileTx struct{ s *fakeProfiles }

func (t *fakeProfileTx) WorkspaceName(_ context.Context, id string) (string, bool, error) {
	n, ok := t.s.workspaces[id]
	return n, ok, nil
}

func (t *fakeProfileTx) Get(_ context.Context, ws string) (Subscription, bool, error) {
	v, ok := t.s.subs[ws]
	return v, ok, nil
}

func (t *fakeProfileTx) Plan(_ context.Context, key string) (plan.Plan, bool, error) {
	p, ok := t.s.plans[key]
	return p, ok, nil
}

func (t *fakeProfileTx) LockedProfile(ctx context.Context, ws string) (BillingProfile, bool, error) {
	return t.s.Profile(ctx, ws)
}

func (t *fakeProfileTx) SaveProfile(_ context.Context, p BillingProfile) error {
	p.UpdatedAt = t0
	t.s.profiles[p.WorkspaceID] = p
	return nil
}

func (t *fakeProfileTx) InsertAuditEvent(_ context.Context, e audit.Entry) error {
	t.s.audits = append(t.s.audits, e)
	return nil
}

func completeProfile() ProfileFields {
	return ProfileFields{LegalName: "Acme Co", TaxCode: "0123456789", Address: "1 Main St", InvoiceEmail: "ap@acme.test"}
}

func kindChange(kind string, fields *ProfileFields) KindChange {
	return KindChange{
		WorkspaceID: wsID, Kind: kind, Profile: fields,
		ActorType: audit.ActorMember, ActorID: profileActor, ActorLabel: "owner@example.test",
	}
}

func asError(t *testing.T, err error) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want *Error", err)
	}
	return e
}

func TestAccountKindWithoutARowIsIndividual(t *testing.T) {
	s := newFakeProfiles(t)
	kind, err := AccountKind(context.Background(), s, wsID)
	if err != nil || kind != AccountIndividual {
		t.Fatalf("AccountKind = %q, %v; want individual", kind, err)
	}
	p, err := ProfileOrDefault(context.Background(), s, wsID)
	if err != nil || p.AccountKind != AccountIndividual || p.WorkspaceID != wsID || p.VerifiedAt != nil {
		t.Fatalf("ProfileOrDefault = %+v, %v", p, err)
	}
}

func TestAccountKindReadsTheStoredRow(t *testing.T) {
	s := newFakeProfiles(t)
	s.profiles[wsID] = BillingProfile{WorkspaceID: wsID, AccountKind: AccountBusiness}
	if kind, err := AccountKind(context.Background(), s, wsID); err != nil || kind != AccountBusiness {
		t.Fatalf("AccountKind = %q, %v; want business", kind, err)
	}
}

func TestAccountKindRefusesAStoredKindItDoesNotKnow(t *testing.T) {
	s := newFakeProfiles(t)
	s.profiles[wsID] = BillingProfile{WorkspaceID: wsID, AccountKind: "enterprise"}
	if _, err := AccountKind(context.Background(), s, wsID); err == nil {
		t.Fatal("an unknown stored kind was accepted")
	}
}

func TestCheckAudienceMatrix(t *testing.T) {
	cases := []struct {
		audience, kind string
		ok             bool
	}{
		{plan.AudienceAny, AccountIndividual, true},
		{plan.AudienceAny, AccountBusiness, true},
		{plan.AudienceIndividual, AccountIndividual, true},
		{plan.AudienceIndividual, AccountBusiness, false},
		{plan.AudienceBusiness, AccountIndividual, false},
		{plan.AudienceBusiness, AccountBusiness, true},
		{"", AccountIndividual, true}, // a plan with no audience reads as any
	}
	for _, c := range cases {
		t.Run(c.audience+"/"+c.kind, func(t *testing.T) {
			err := CheckAudience(plan.Plan{Key: "p1", Audience: c.audience}, c.kind)
			if c.ok {
				if err != nil {
					t.Fatalf("CheckAudience = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("CheckAudience = nil, want a refusal")
			}
			if err.Status != http.StatusConflict || err.Code != "plan_audience_mismatch" {
				t.Fatalf("refusal = %d %s", err.Status, err.Code)
			}
			want := map[string]any{"plan_key": "p1", "account_kind": c.kind}
			if !reflect.DeepEqual(err.Detail, want) {
				t.Fatalf("detail = %v, want %v", err.Detail, want)
			}
		})
	}
}

func TestCheckPlanForWorkspaceRefusesABusinessPlanForAnIndividual(t *testing.T) {
	s := newFakeProfiles(t)
	ctx := context.Background()
	biz := s.plans["business"]
	if biz.Audience != plan.AudienceBusiness {
		t.Fatalf("catalogue business audience = %q", biz.Audience)
	}
	if e := asError(t, CheckPlanForWorkspace(ctx, s, wsID, biz)); e.Code != "plan_audience_mismatch" || e.Status != http.StatusConflict {
		t.Fatalf("refusal = %v", e)
	}
	if err := CheckPlanForWorkspace(ctx, s, wsID, s.plans["pro"]); err != nil {
		t.Fatalf("an individual buying an any plan: %v", err)
	}
	// A plan change that breaks the audience is refused the same way.
	s.profiles[wsID] = BillingProfile{WorkspaceID: wsID, AccountKind: AccountBusiness}
	if err := CheckPlanForWorkspace(ctx, s, wsID, biz); err != nil {
		t.Fatalf("a business buying a business plan: %v", err)
	}
	solo := s.plans["pro"]
	solo.Key, solo.Audience = "solo", plan.AudienceIndividual
	if e := asError(t, CheckPlanForWorkspace(ctx, s, wsID, solo)); e.Code != "plan_audience_mismatch" {
		t.Fatalf("a business buying an individual plan: %v", e)
	}
}

func TestChangeToBusinessNeedsACompleteProfile(t *testing.T) {
	s := newFakeProfiles(t)
	svc := NewProfileService(s)
	_, err := svc.ChangeAccountKind(context.Background(), kindChange(AccountBusiness, &ProfileFields{LegalName: "Acme Co", TaxCode: "0123456789"}))
	e := asError(t, err)
	if e.Status != http.StatusConflict || e.Code != "billing_profile_incomplete" {
		t.Fatalf("refusal = %v", e)
	}
	if want := []string{"address", "invoice_email"}; !reflect.DeepEqual(e.Detail["fields"], want) {
		t.Fatalf("fields = %v, want %v", e.Detail["fields"], want)
	}
	if len(s.profiles) != 0 || len(s.audits) != 0 {
		t.Fatalf("a refused change wrote %d profiles and %d audit rows", len(s.profiles), len(s.audits))
	}
	// No profile given and none stored: everything is missing, in spec order.
	_, err = svc.ChangeAccountKind(context.Background(), kindChange(AccountBusiness, nil))
	e = asError(t, err)
	if want := []string{"legal_name", "tax_code", "address", "invoice_email"}; e.Code != "billing_profile_incomplete" || !reflect.DeepEqual(e.Detail["fields"], want) {
		t.Fatalf("refusal = %v, fields %v", e, e.Detail["fields"])
	}
	// Spaces only is missing.
	f := completeProfile()
	f.Address = "   "
	_, err = svc.ChangeAccountKind(context.Background(), kindChange(AccountBusiness, &f))
	if e := asError(t, err); e.Code != "billing_profile_incomplete" || !reflect.DeepEqual(e.Detail["fields"], []string{"address"}) {
		t.Fatalf("refusal = %v, fields %v", e, e.Detail["fields"])
	}
}

func TestTaxCodeFormat(t *testing.T) {
	cases := []struct {
		code  string
		state string // "ok", "invalid" or "missing"
	}{
		{"0123456789", "ok"},
		{"0123456789012", "ok"},
		{"0123456789-012", "ok"},
		{" 0123456789 ", "ok"},
		{"012345678", "invalid"},
		{"01234567890", "invalid"},
		{"012345678901", "invalid"},
		{"01234567890123", "invalid"},
		{"01234567ab", "invalid"},
		{"0123-456789", "invalid"},
		{"0123456789--012", "invalid"},
		{"-0123456789", "invalid"},
		{"0123456789-", "invalid"},
		{"", "missing"},
		{"  ", "missing"},
	}
	for _, c := range cases {
		t.Run(c.code, func(t *testing.T) {
			s := newFakeProfiles(t)
			f := completeProfile()
			f.TaxCode = c.code
			_, err := NewProfileService(s).ChangeAccountKind(context.Background(), kindChange(AccountBusiness, &f))
			switch c.state {
			case "ok":
				if err != nil {
					t.Fatalf("err = %v", err)
				}
			case "invalid":
				e := asError(t, err)
				if e.Status != http.StatusBadRequest || e.Code != "billing_profile_invalid" || !reflect.DeepEqual(e.Detail["fields"], []string{"tax_code"}) {
					t.Fatalf("refusal = %v, fields %v", e, e.Detail["fields"])
				}
			case "missing":
				if e := asError(t, err); e.Code != "billing_profile_incomplete" {
					t.Fatalf("refusal = %v", e)
				}
			}
		})
	}
}

func TestChangeToBusinessWritesTheProfileAndAnAuditRow(t *testing.T) {
	s := newFakeProfiles(t)
	f := completeProfile()
	f.TaxCode = " 0123456789-012 "
	res, err := NewProfileService(s).ChangeAccountKind(context.Background(), kindChange(AccountBusiness, &f))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Profile.AccountKind != AccountBusiness || res.Profile.TaxCode != "0123456789-012" {
		t.Fatalf("result = %+v", res)
	}
	stored := s.profiles[wsID]
	if stored.AccountKind != AccountBusiness || stored.LegalName != "Acme Co" || stored.UpdatedBy != profileActor {
		t.Fatalf("stored = %+v", stored)
	}
	if len(s.audits) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(s.audits))
	}
	a := s.audits[0]
	if a.Action != "workspace.account_kind_changed" ||
		a.WorkspaceID != wsID || a.TargetType != "workspace" || a.TargetID != wsID || a.TargetLabel != wsName ||
		a.ActorType != audit.ActorMember || a.ActorID != profileActor {
		t.Fatalf("audit = %+v", a)
	}
	want := map[string]any{"account_kind": map[string]any{"from": AccountIndividual, "to": AccountBusiness}}
	if !reflect.DeepEqual(a.Detail["changes"], want) {
		t.Fatalf("detail changes = %v, want %v", a.Detail["changes"], want)
	}
}

func TestChangeToTheSameKindWritesNothing(t *testing.T) {
	s := newFakeProfiles(t)
	svc := NewProfileService(s)
	res, err := svc.ChangeAccountKind(context.Background(), kindChange(AccountIndividual, nil))
	if err != nil || res.Changed || res.Profile.AccountKind != AccountIndividual {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if len(s.profiles) != 0 || len(s.audits) != 0 {
		t.Fatalf("a no-op wrote %d profiles and %d audit rows", len(s.profiles), len(s.audits))
	}
}

func TestChangeBackToIndividualKeepsTheProfile(t *testing.T) {
	s := newFakeProfiles(t)
	svc := NewProfileService(s)
	ctx := context.Background()
	f := completeProfile()
	if _, err := svc.ChangeAccountKind(ctx, kindChange(AccountBusiness, &f)); err != nil {
		t.Fatal(err)
	}
	// No subscription row: the free plan, open to any kind.
	res, err := svc.ChangeAccountKind(ctx, kindChange(AccountIndividual, nil))
	if err != nil || !res.Changed {
		t.Fatalf("result = %+v, %v", res, err)
	}
	stored := s.profiles[wsID]
	if stored.AccountKind != AccountIndividual || stored.LegalName != "Acme Co" || stored.TaxCode != "0123456789" ||
		stored.Address != "1 Main St" || stored.InvoiceEmail != "ap@acme.test" {
		t.Fatalf("the profile was not kept: %+v", stored)
	}
	if len(s.audits) != 2 {
		t.Fatalf("audit rows = %d, want 2", len(s.audits))
	}
	want := map[string]any{"account_kind": map[string]any{"from": AccountBusiness, "to": AccountIndividual}}
	if !reflect.DeepEqual(s.audits[1].Detail["changes"], want) {
		t.Fatalf("second audit changes = %v, want %v", s.audits[1].Detail["changes"], want)
	}
	// Going back to business needs nothing new: the kept profile is complete.
	if res, err := svc.ChangeAccountKind(ctx, kindChange(AccountBusiness, nil)); err != nil || !res.Changed {
		t.Fatalf("back to business = %+v, %v", res, err)
	}
}

func TestChangeBackToIndividualFollowsTheCurrentPlanAudience(t *testing.T) {
	cases := []struct {
		planKey  string
		audience string
		ok       bool
	}{
		{"business", plan.AudienceBusiness, false},
		{"pro", plan.AudienceAny, true},
		{"solo", plan.AudienceIndividual, true},
	}
	for _, c := range cases {
		t.Run(c.audience, func(t *testing.T) {
			s := newFakeProfiles(t)
			p := s.plans["pro"]
			p.Key, p.Audience = c.planKey, c.audience
			s.plans[c.planKey] = p
			s.subs[wsID] = Subscription{WorkspaceID: wsID, PlanKey: c.planKey, Status: StatusActive}
			s.profiles[wsID] = BillingProfile{WorkspaceID: wsID, AccountKind: AccountBusiness, LegalName: "Acme Co"}
			_, err := NewProfileService(s).ChangeAccountKind(context.Background(), kindChange(AccountIndividual, nil))
			if c.ok {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			e := asError(t, err)
			if e.Status != http.StatusConflict || e.Code != "plan_audience_mismatch" {
				t.Fatalf("refusal = %v", e)
			}
			if s.profiles[wsID].AccountKind != AccountBusiness || len(s.audits) != 0 {
				t.Fatalf("a refused change wrote: %+v, %d audit rows", s.profiles[wsID], len(s.audits))
			}
		})
	}
}

func TestChangeKindRefusesUnknownKindAndWorkspace(t *testing.T) {
	s := newFakeProfiles(t)
	svc := NewProfileService(s)
	if e := asError(t, func() error {
		_, err := svc.ChangeAccountKind(context.Background(), kindChange("enterprise", nil))
		return err
	}()); e.Status != http.StatusBadRequest || e.Code != "billing_profile_invalid" {
		t.Fatalf("unknown kind: %v", e)
	}
	c := kindChange(AccountBusiness, nil)
	c.WorkspaceID = "bbbbbbbb-0000-0000-0000-0000000000ff"
	if e := asError(t, func() error { _, err := svc.ChangeAccountKind(context.Background(), c); return err }()); e.Status != http.StatusNotFound {
		t.Fatalf("unknown workspace: %v", e)
	}
	c.WorkspaceID = "nope"
	if e := asError(t, func() error { _, err := svc.ChangeAccountKind(context.Background(), c); return err }()); e.Status != http.StatusBadRequest {
		t.Fatalf("bad workspace id: %v", e)
	}
}

func TestChangingNameOrTaxCodeClearsTheVerification(t *testing.T) {
	verified := t0
	for name, mutate := range map[string]func(*ProfileFields){
		"legal name": func(f *ProfileFields) { f.LegalName = "Acme Holdings" },
		"tax code":   func(f *ProfileFields) { f.TaxCode = "9876543210" },
		"address":    func(f *ProfileFields) { f.Address = "2 Side St" },
	} {
		t.Run(name, func(t *testing.T) {
			s := newFakeProfiles(t)
			old := BillingProfile{
				WorkspaceID: wsID, AccountKind: AccountIndividual, LegalName: "Acme Co", TaxCode: "0123456789",
				Address: "1 Main St", InvoiceEmail: "ap@acme.test", VerifiedAt: &verified, VerifiedBy: staffID,
			}
			s.profiles[wsID] = old
			f := completeProfile()
			mutate(&f)
			if _, err := NewProfileService(s).ChangeAccountKind(context.Background(), kindChange(AccountBusiness, &f)); err != nil {
				t.Fatal(err)
			}
			got := s.profiles[wsID]
			wantCleared := name != "address"
			if wantCleared != (got.VerifiedAt == nil && got.VerifiedBy == "") {
				t.Fatalf("verified = %v by %q after changing the %s", got.VerifiedAt, got.VerifiedBy, name)
			}
		})
	}
}

func verifyChange(verified bool) VerifyChange {
	return VerifyChange{
		WorkspaceID: wsID, Verified: verified, At: t0,
		ActorType: audit.ActorStaff, ActorID: staffID, ActorLabel: staffEm,
	}
}

func businessProfile() BillingProfile {
	f := completeProfile()
	return BillingProfile{
		WorkspaceID: wsID, AccountKind: AccountBusiness, LegalName: f.LegalName, TaxCode: f.TaxCode,
		Address: f.Address, InvoiceEmail: f.InvoiceEmail,
	}
}

func TestVerifySetsVerifiedAtAndVerifiedBy(t *testing.T) {
	s := newFakeProfiles(t)
	s.profiles[wsID] = businessProfile()
	res, err := NewProfileService(s).Verify(context.Background(), verifyChange(true))
	if err != nil || !res.Changed {
		t.Fatalf("Verify = %+v, %v", res, err)
	}
	got := s.profiles[wsID]
	if got.VerifiedAt == nil || !got.VerifiedAt.Equal(t0) || got.VerifiedBy != staffID {
		t.Fatalf("stored verified_at = %v, verified_by = %q", got.VerifiedAt, got.VerifiedBy)
	}
	if res.Profile.VerifiedAt == nil || !res.Profile.VerifiedAt.Equal(t0) {
		t.Fatalf("result verified_at = %v", res.Profile.VerifiedAt)
	}
	if len(s.audits) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(s.audits))
	}
	e := s.audits[0]
	if e.Action != audit.ActionBillingProfileVerified || e.WorkspaceID != wsID || e.TargetType != "workspace" ||
		e.TargetID != wsID || e.TargetLabel != wsName || e.ActorType != audit.ActorStaff || e.ActorID != staffID || e.ActorLabel != staffEm {
		t.Fatalf("audit = %+v", e)
	}
	if strings.Contains(fmt.Sprint(e.Detail), completeProfile().TaxCode) {
		t.Fatalf("the tax code reached the audit detail: %v", e.Detail)
	}
}

func TestUnverifyClearsVerification(t *testing.T) {
	s := newFakeProfiles(t)
	p := businessProfile()
	at := t0.Add(-time.Hour)
	p.VerifiedAt, p.VerifiedBy = &at, staffID
	s.profiles[wsID] = p
	res, err := NewProfileService(s).Verify(context.Background(), verifyChange(false))
	if err != nil || !res.Changed {
		t.Fatalf("Verify = %+v, %v", res, err)
	}
	if got := s.profiles[wsID]; got.VerifiedAt != nil || got.VerifiedBy != "" {
		t.Fatalf("stored verified_at = %v, verified_by = %q", got.VerifiedAt, got.VerifiedBy)
	}
	if len(s.audits) != 1 || s.audits[0].Action != audit.ActionBillingProfileUnverified || s.audits[0].WorkspaceID != wsID {
		t.Fatalf("audits = %+v", s.audits)
	}
}

func TestVerifyTwiceWritesNothingTheSecondTime(t *testing.T) {
	s := newFakeProfiles(t)
	s.profiles[wsID] = businessProfile()
	svc := NewProfileService(s)
	if _, err := svc.Verify(context.Background(), verifyChange(true)); err != nil {
		t.Fatal(err)
	}
	later := verifyChange(true)
	later.At = t0.Add(time.Hour)
	res, err := svc.Verify(context.Background(), later)
	if err != nil || res.Changed {
		t.Fatalf("second Verify = %+v, %v; want Changed false", res, err)
	}
	if got := s.profiles[wsID]; !got.VerifiedAt.Equal(t0) {
		t.Fatalf("verified_at moved to %v", got.VerifiedAt)
	}
	if len(s.audits) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(s.audits))
	}
	// Unverifying a profile that is not verified writes nothing either.
	s2 := newFakeProfiles(t)
	s2.profiles[wsID] = businessProfile()
	if res, err := NewProfileService(s2).Verify(context.Background(), verifyChange(false)); err != nil || res.Changed || len(s2.audits) != 0 {
		t.Fatalf("unverify of an unverified profile = %+v, %v, %d audit rows", res, err, len(s2.audits))
	}
}

func TestVerifyNeedsCompleteProfile(t *testing.T) {
	s := newFakeProfiles(t)
	svc := NewProfileService(s)
	_, err := svc.Verify(context.Background(), verifyChange(true))
	e := asError(t, err)
	if e.Status != http.StatusConflict || e.Code != "billing_profile_incomplete" ||
		!reflect.DeepEqual(e.Detail["fields"], []string{"legal_name", "tax_code", "address", "invoice_email"}) {
		t.Fatalf("no row: %v, fields %v", e, e.Detail["fields"])
	}

	p := businessProfile()
	p.TaxCode = ""
	s.profiles[wsID] = p
	_, err = svc.Verify(context.Background(), verifyChange(true))
	e = asError(t, err)
	if e.Status != http.StatusConflict || e.Code != "billing_profile_incomplete" || !reflect.DeepEqual(e.Detail["fields"], []string{"tax_code"}) {
		t.Fatalf("missing tax code: %v, fields %v", e, e.Detail["fields"])
	}
	if got := s.profiles[wsID]; got.VerifiedAt != nil || len(s.audits) != 0 {
		t.Fatalf("a refused verify wrote verified_at %v and %d audit rows", got.VerifiedAt, len(s.audits))
	}
}

func TestVerifyUnknownWorkspaceIsNotFound(t *testing.T) {
	s := newFakeProfiles(t)
	c := verifyChange(true)
	c.WorkspaceID = "bbbbbbbb-0000-0000-0000-0000000000ff"
	if e := asError(t, func() error { _, err := NewProfileService(s).Verify(context.Background(), c); return err }()); e.Status != http.StatusNotFound {
		t.Fatalf("unknown workspace: %v", e)
	}
	if len(s.audits) != 0 {
		t.Fatalf("audit rows = %d", len(s.audits))
	}
}

func TestAuditActionAccountKindChangedIsInTheCatalogue(t *testing.T) {
	found := false
	for _, a := range audit.Catalog {
		found = found || a == "workspace.account_kind_changed"
	}
	if !found {
		t.Fatal("audit.Catalog lacks workspace.account_kind_changed")
	}
}

func TestErrorDetailIsPartOfTheBody(t *testing.T) {
	e := &Error{Status: http.StatusConflict, Code: "x", Message: "m", Detail: map[string]any{"fields": []string{"a"}}}
	body := errorBody(e)
	if body["code"] != "x" || body["error"] != "m" || !reflect.DeepEqual(body["fields"], []string{"a"}) {
		t.Fatalf("body = %v", body)
	}
	if plain := errorBody(badRequest("c", "m")); len(plain) != 2 {
		t.Fatalf("a refusal with no detail has body %v", plain)
	}
}
