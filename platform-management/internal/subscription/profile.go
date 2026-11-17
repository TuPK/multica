package subscription

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

// Account kinds (spec 7b.1, rule 1).
const (
	AccountIndividual = "individual"
	AccountBusiness   = "business"
)

// Error codes of the account kind and audience checks (spec 7b.1).
const (
	codeAudienceMismatch  = "plan_audience_mismatch"
	codeProfileIncomplete = "billing_profile_incomplete"
	codeProfileInvalid    = "billing_profile_invalid"
	fieldAccountKind      = "account_kind"
	fieldLegalName        = "legal_name"
	fieldTaxCode          = "tax_code"
	fieldAddress          = "address"
	fieldInvoiceEmail     = "invoice_email"
	fieldVerifiedAt       = "verified_at"
	auditChanges          = "changes"
)

// BillingProfile is one row of billing_profile. A workspace with no row is an
// individual account with empty fields (ProfileOrDefault).
type BillingProfile struct {
	WorkspaceID  string
	AccountKind  string
	LegalName    string
	TaxCode      string
	Address      string
	InvoiceEmail string
	VerifiedAt   *time.Time
	VerifiedBy   string
	UpdatedBy    string
	UpdatedAt    time.Time
}

// ProfileFields are the four fields a business account must have.
type ProfileFields struct {
	LegalName    string
	TaxCode      string
	Address      string
	InvoiceEmail string
}

// KindReader reads a workspace's billing profile.
type KindReader interface {
	// Profile returns found == false for a workspace with no row.
	Profile(ctx context.Context, workspaceID string) (p BillingProfile, found bool, err error)
}

// ProfileOrDefault returns the stored profile, or the individual profile a
// workspace with no row has.
func ProfileOrDefault(ctx context.Context, r KindReader, workspaceID string) (BillingProfile, error) {
	p, found, err := r.Profile(ctx, workspaceID)
	if err != nil {
		return BillingProfile{}, err
	}
	if !found {
		return defaultProfile(workspaceID), nil
	}
	if !validKind(p.AccountKind) {
		return BillingProfile{}, fmt.Errorf("billing profile %s: unknown account kind %q", workspaceID, p.AccountKind)
	}
	return p, nil
}

// AccountKind returns the workspace's account kind: individual when it has no
// billing profile row.
func AccountKind(ctx context.Context, r KindReader, workspaceID string) (string, error) {
	p, err := ProfileOrDefault(ctx, r, workspaceID)
	return p.AccountKind, err
}

func defaultProfile(workspaceID string) BillingProfile {
	return BillingProfile{WorkspaceID: workspaceID, AccountKind: AccountIndividual}
}

func validKind(kind string) bool { return kind == AccountIndividual || kind == AccountBusiness }

// CheckAudience refuses a plan whose audience does not allow the account kind
// (spec 7b.1, rule 2). A plan with no audience is open to any kind.
func CheckAudience(p plan.Plan, kind string) *Error {
	if p.Audience == "" || p.Audience == plan.AudienceAny || p.Audience == kind {
		return nil
	}
	return &Error{
		Status: http.StatusConflict, Code: codeAudienceMismatch,
		Message: "the plan is not sold to this kind of account",
		Detail:  map[string]any{"plan_key": p.Key, "account_kind": kind},
	}
}

// CheckPlanForWorkspace is the audience check of every step that attaches a plan
// to a workspace: checkout and, later, a plan change. It reads the workspace's
// account kind and returns a *Error when p is not sold to it.
func CheckPlanForWorkspace(ctx context.Context, r KindReader, workspaceID string, p plan.Plan) error {
	kind, err := AccountKind(ctx, r, workspaceID)
	if err != nil {
		return err
	}
	if e := CheckAudience(p, kind); e != nil {
		return e
	}
	return nil
}

// A tax code is 10 digits, or 10 digits and a 3 digit branch with an optional
// hyphen between the two groups. Only the format is checked (spec 7b.1, rule 3).
var taxCodePattern = regexp.MustCompile(`^[0-9]{10}(-?[0-9]{3})?$`)

func incomplete(fields []string) *Error {
	return &Error{
		Status: http.StatusConflict, Code: codeProfileIncomplete,
		Message: "the billing profile lacks fields a business account needs",
		Detail:  map[string]any{"fields": fields},
	}
}

func invalid(msg string, fields ...string) *Error {
	e := badRequest(codeProfileInvalid, msg)
	e.Detail = map[string]any{"fields": fields}
	return e
}

// missingFields lists the required fields of f that are empty, in spec order.
func missingFields(f ProfileFields) []string {
	var out []string
	for _, c := range []struct{ name, value string }{
		{fieldLegalName, f.LegalName}, {fieldTaxCode, f.TaxCode},
		{fieldAddress, f.Address}, {fieldInvoiceEmail, f.InvoiceEmail},
	} {
		if c.value == "" {
			out = append(out, c.name)
		}
	}
	return out
}

func (f ProfileFields) trimmed() ProfileFields {
	return ProfileFields{
		LegalName: strings.TrimSpace(f.LegalName), TaxCode: strings.TrimSpace(f.TaxCode),
		Address: strings.TrimSpace(f.Address), InvoiceEmail: strings.TrimSpace(f.InvoiceEmail),
	}
}

func fieldsOf(p BillingProfile) ProfileFields {
	return ProfileFields{LegalName: p.LegalName, TaxCode: p.TaxCode, Address: p.Address, InvoiceEmail: p.InvoiceEmail}
}

// ProfileTx is the reads and writes of one account kind change, all inside one
// transaction with its audit row.
type ProfileTx interface {
	// WorkspaceName returns found == false for an unknown workspace.
	WorkspaceName(ctx context.Context, id string) (name string, found bool, err error)
	// Get reads the workspace's subscription row and locks it.
	Get(ctx context.Context, workspaceID string) (s Subscription, found bool, err error)
	// Plan reads a plan.
	Plan(ctx context.Context, key string) (p plan.Plan, found bool, err error)
	// LockedProfile reads the workspace's profile and holds off every other
	// writer of it until the transaction ends.
	LockedProfile(ctx context.Context, workspaceID string) (p BillingProfile, found bool, err error)
	// SaveProfile inserts the profile, or replaces the stored one, and sets
	// updated_at.
	SaveProfile(ctx context.Context, p BillingProfile) error

	audit.Writer
}

// ProfileRunner runs fn in one transaction: it commits when fn returns nil and
// rolls back otherwise.
type ProfileRunner interface {
	InTx(ctx context.Context, fn func(ProfileTx) error) error
}

// ProfileService changes a workspace's account kind.
type ProfileService struct{ runner ProfileRunner }

// NewProfileService returns a ProfileService.
func NewProfileService(r ProfileRunner) *ProfileService { return &ProfileService{runner: r} }

// KindChange asks for a workspace's account kind to become Kind. Profile, when
// set, replaces the four profile fields in the same write; nil keeps the stored
// ones.
type KindChange struct {
	WorkspaceID string
	Kind        string
	Profile     *ProfileFields

	ActorType  string
	ActorID    string
	ActorLabel string
}

// KindResult is the profile after a KindChange. Changed is false when the
// workspace already had the kind: nothing was written.
type KindResult struct {
	Profile BillingProfile
	Changed bool
}

// ChangeAccountKind moves a workspace to c.Kind (spec 7b.1, rule 3).
//
// To business, the profile (the stored one with c.Profile laid over it) must
// have legal_name, tax_code, address and invoice_email, and a well-formed
// tax_code. To individual, the workspace's current plan must be open to
// individuals; the profile is kept. A change of kind writes the profile and a
// workspace.account_kind_changed audit row in one transaction. Changing
// legal_name or tax_code clears the verification (rule 12).
func (s *ProfileService) ChangeAccountKind(ctx context.Context, c KindChange) (KindResult, error) {
	ws, ok := trust.CanonicalUUID(c.WorkspaceID)
	if !ok {
		return KindResult{}, badRequest(codeProfileInvalid, "workspace id is not a UUID")
	}
	c.WorkspaceID = ws
	if !validKind(c.Kind) {
		return KindResult{}, invalid("account_kind must be individual or business", fieldAccountKind)
	}
	if c.ActorID != "" {
		id, ok := trust.CanonicalUUID(c.ActorID)
		if !ok {
			return KindResult{}, badRequest(codeProfileInvalid, "actor id is not a UUID")
		}
		c.ActorID = id
	}

	var res KindResult
	err := s.runner.InTx(ctx, func(tx ProfileTx) error {
		res = KindResult{}
		name, found, err := tx.WorkspaceName(ctx, ws)
		if err != nil {
			return err
		}
		if !found {
			return NotFound()
		}
		cur, found, err := tx.LockedProfile(ctx, ws)
		if err != nil {
			return err
		}
		if !found {
			cur = defaultProfile(ws)
		}
		if !validKind(cur.AccountKind) {
			return fmt.Errorf("billing profile %s: unknown account kind %q", ws, cur.AccountKind)
		}
		if cur.AccountKind == c.Kind {
			res = KindResult{Profile: cur}
			return nil
		}

		next := cur
		next.AccountKind = c.Kind
		if c.Profile != nil {
			f := c.Profile.trimmed()
			next.LegalName, next.TaxCode, next.Address, next.InvoiceEmail = f.LegalName, f.TaxCode, f.Address, f.InvoiceEmail
		}
		if next.LegalName != cur.LegalName || next.TaxCode != cur.TaxCode {
			next.VerifiedAt, next.VerifiedBy = nil, ""
		}

		switch c.Kind {
		case AccountBusiness:
			if m := missingFields(fieldsOf(next)); len(m) > 0 {
				return incomplete(m)
			}
		case AccountIndividual:
			if err := s.currentPlanAllows(ctx, tx, ws, AccountIndividual); err != nil {
				return err
			}
		}
		if next.TaxCode != "" && !taxCodePattern.MatchString(next.TaxCode) {
			return invalid("tax_code must be 10 digits, or 13 digits with an optional hyphen after the tenth", fieldTaxCode)
		}

		next.UpdatedBy = c.ActorID
		if err := tx.SaveProfile(ctx, next); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			WorkspaceID: ws,
			ActorType:   c.ActorType,
			ActorID:     c.ActorID,
			ActorLabel:  c.ActorLabel,
			Action:      audit.ActionWorkspaceAccountKindChanged,
			TargetType:  targetWorkspace,
			TargetID:    ws,
			TargetLabel: name,
			Detail: map[string]any{
				auditChanges: map[string]any{
					fieldAccountKind: map[string]any{"from": cur.AccountKind, "to": next.AccountKind},
				},
			},
		}); err != nil {
			return err
		}
		res = KindResult{Profile: next, Changed: true}
		return nil
	})
	if err != nil {
		return KindResult{}, err
	}
	return res, nil
}

// currentPlanAllows refuses when the workspace's current plan is not sold to
// kind. A workspace with no subscription row is on the free plan.
func (s *ProfileService) currentPlanAllows(ctx context.Context, tx ProfileTx, workspaceID, kind string) error {
	key := plan.FreeKey
	sub, found, err := tx.Get(ctx, workspaceID)
	if err != nil {
		return err
	}
	if found {
		key = sub.PlanKey
	}
	p, found, err := tx.Plan(ctx, key)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("workspace %s is on plan %q, which does not exist", workspaceID, key)
	}
	if e := CheckAudience(p, kind); e != nil {
		return e
	}
	return nil
}

// VerifyChange asks for a workspace's billing profile to be marked verified, or
// unverified, by a staff member who checked the tax code outside the system.
// At is the time stored as verified_at.
type VerifyChange struct {
	WorkspaceID string
	Verified    bool
	At          time.Time

	ActorType  string
	ActorID    string
	ActorLabel string
}

// VerifyResult is the profile after a VerifyChange. Changed is false when the
// profile already was in the asked state: nothing was written.
type VerifyResult struct {
	Profile BillingProfile
	Changed bool
}

// Verify sets or clears the verification of a workspace's billing profile (spec
// 7b.1, rule 12). The profile must be stored and have all four fields; its
// account kind is not checked. A change writes the profile and a
// billing_profile.verified or billing_profile.unverified audit row in one
// transaction.
func (s *ProfileService) Verify(ctx context.Context, c VerifyChange) (VerifyResult, error) {
	ws, ok := trust.CanonicalUUID(c.WorkspaceID)
	if !ok {
		return VerifyResult{}, badRequest(codeProfileInvalid, "workspace id is not a UUID")
	}
	if c.ActorID != "" {
		id, ok := trust.CanonicalUUID(c.ActorID)
		if !ok {
			return VerifyResult{}, badRequest(codeProfileInvalid, "actor id is not a UUID")
		}
		c.ActorID = id
	}

	var res VerifyResult
	err := s.runner.InTx(ctx, func(tx ProfileTx) error {
		res = VerifyResult{}
		name, found, err := tx.WorkspaceName(ctx, ws)
		if err != nil {
			return err
		}
		if !found {
			return NotFound()
		}
		cur, found, err := tx.LockedProfile(ctx, ws)
		if err != nil {
			return err
		}
		if !found {
			return incomplete(missingFields(ProfileFields{}))
		}
		if m := missingFields(fieldsOf(cur)); len(m) > 0 {
			return incomplete(m)
		}
		if (cur.VerifiedAt != nil) == c.Verified {
			res = VerifyResult{Profile: cur}
			return nil
		}

		next := cur
		action := audit.ActionBillingProfileUnverified
		var to any
		if c.Verified {
			at := c.At.UTC()
			next.VerifiedAt, next.VerifiedBy = &at, c.ActorID
			action, to = audit.ActionBillingProfileVerified, at.Format(time.RFC3339Nano)
		} else {
			next.VerifiedAt, next.VerifiedBy = nil, ""
		}
		var from any
		if cur.VerifiedAt != nil {
			from = cur.VerifiedAt.UTC().Format(time.RFC3339Nano)
		}
		if err := tx.SaveProfile(ctx, next); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			WorkspaceID: ws,
			ActorType:   c.ActorType,
			ActorID:     c.ActorID,
			ActorLabel:  c.ActorLabel,
			Action:      action,
			TargetType:  targetWorkspace,
			TargetID:    ws,
			TargetLabel: name,
			Detail: map[string]any{
				auditChanges: map[string]any{fieldVerifiedAt: map[string]any{"from": from, "to": to}},
			},
		}); err != nil {
			return err
		}
		res = VerifyResult{Profile: next, Changed: true}
		return nil
	})
	if err != nil {
		return VerifyResult{}, err
	}
	return res, nil
}
