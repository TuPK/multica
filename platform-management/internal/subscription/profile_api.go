package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"slices"
	"unicode/utf8"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

// ProfilePattern is the route Multica's billing proxy forwards
// PUT /api/cloud-subscriptions/billing-profile to. Like the other Cloud routes
// it carries X-User-ID and no credential.
const ProfilePattern = "PUT /api/v1/subscriptions/{ws}/billing-profile"

const (
	maxProfileBody  = 16 << 10
	maxNameLen      = 255
	maxTaxCodeLen   = 32
	maxAddressLen   = 1024
	maxEmailLen     = 254
	roleOwner       = "owner"
	roleAdmin       = "admin"
	codeForbidden   = "forbidden"
	codeInternalErr = "internal"
)

// KindChanger changes a workspace's account kind. *ProfileService is one.
type KindChanger interface {
	ChangeAccountKind(ctx context.Context, c KindChange) (KindResult, error)
}

// ProfileAPIDeps are the profile route's dependencies.
type ProfileAPIDeps struct {
	Service KindChanger
	Members MemberReader
	Log     *slog.Logger
}

// ProfileAPI serves ProfilePattern.
type ProfileAPI struct{ ProfileAPIDeps }

// NewProfileAPI returns a ProfileAPI.
func NewProfileAPI(d ProfileAPIDeps) *ProfileAPI {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &ProfileAPI{d}
}

// Handler serves ProfilePattern. requireUser is trust.Peers.RequireUser.
func (a *ProfileAPI) Handler(requireUser func(http.Handler) http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(ProfilePattern, requireUser(http.HandlerFunc(a.put)))
	return mux
}

// profileRequest is the body of the route. The four profile fields replace the
// stored ones when at least one is present.
type profileRequest struct {
	AccountKind  string  `json:"account_kind"`
	LegalName    *string `json:"legal_name"`
	TaxCode      *string `json:"tax_code"`
	Address      *string `json:"address"`
	InvoiceEmail *string `json:"invoice_email"`
}

// profileResponse omits verified_by: the owner sees whether the profile is
// verified, not who verified it (spec 7b.1, API table).
type profileResponse struct {
	AccountKind  string `json:"account_kind"`
	LegalName    string `json:"legal_name"`
	TaxCode      string `json:"tax_code"`
	Address      string `json:"address"`
	InvoiceEmail string `json:"invoice_email"`
	Verified     bool   `json:"verified"`
	Changed      bool   `json:"changed"`
}

func (a *ProfileAPI) put(w http.ResponseWriter, r *http.Request) {
	ws, ok := trust.CanonicalUUID(r.PathValue("ws"))
	if !ok {
		writeAPIError(w, NotFound())
		return
	}
	user, ok := trust.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "X-User-ID is required")
		return
	}
	role, found, err := a.Members.Role(r.Context(), ws, user)
	if err != nil {
		a.internalError(w, r, "read member role", err)
		return
	}
	if !found {
		writeAPIError(w, NotFound())
		return
	}
	if role != roleOwner && role != roleAdmin {
		writeError(w, http.StatusForbidden, codeForbidden, "owner or admin role required")
		return
	}

	var in profileRequest
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxProfileBody))
	if err == nil {
		err = json.Unmarshal(raw, &in)
	}
	if err != nil {
		writeAPIError(w, invalid("body must be a JSON object of strings"))
		return
	}
	change := KindChange{
		WorkspaceID: ws, Kind: in.AccountKind,
		ActorType: audit.ActorMember, ActorID: user,
	}
	if in.LegalName != nil || in.TaxCode != nil || in.Address != nil || in.InvoiceEmail != nil {
		f := ProfileFields{LegalName: deref(in.LegalName), TaxCode: deref(in.TaxCode), Address: deref(in.Address), InvoiceEmail: deref(in.InvoiceEmail)}.trimmed()
		if e := checkProfileFormat(f); e != nil {
			writeAPIError(w, e)
			return
		}
		change.Profile = &f
	}

	res, err := a.Service.ChangeAccountKind(r.Context(), change)
	if err != nil {
		var refusal *Error
		if errors.As(err, &refusal) {
			writeAPIError(w, refusal)
			return
		}
		a.internalError(w, r, "change the account kind", err)
		return
	}
	p := res.Profile
	writeJSON(w, http.StatusOK, profileResponse{
		AccountKind: p.AccountKind, LegalName: p.LegalName, TaxCode: p.TaxCode, Address: p.Address,
		InvoiceEmail: p.InvoiceEmail, Verified: p.VerifiedAt != nil, Changed: res.Changed,
	})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// checkProfileFormat bounds the free-text fields and checks the invoice email
// is one bare address. The tax code format is the service's check.
func checkProfileFormat(f ProfileFields) *Error {
	var bad []string
	for _, c := range []struct {
		field, value string
		max          int
	}{
		{fieldLegalName, f.LegalName, maxNameLen},
		{fieldTaxCode, f.TaxCode, maxTaxCodeLen},
		{fieldAddress, f.Address, maxAddressLen},
		{fieldInvoiceEmail, f.InvoiceEmail, maxEmailLen},
	} {
		if utf8.RuneCountInString(c.value) > c.max {
			bad = append(bad, c.field)
		}
	}
	if f.InvoiceEmail != "" && !bareEmail(f.InvoiceEmail) && !slices.Contains(bad, fieldInvoiceEmail) {
		bad = append(bad, fieldInvoiceEmail)
	}
	if len(bad) == 0 {
		return nil
	}
	return invalid("a profile field is too long or malformed", bad...)
}

func bareEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Name == "" && a.Address == s
}

func (a *ProfileAPI) internalError(w http.ResponseWriter, r *http.Request, op string, err error) {
	// The cause may name the database, so it goes to the log only.
	a.Log.Error("billing profile api: "+op+" failed", "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, codeInternalErr, "internal error")
}
