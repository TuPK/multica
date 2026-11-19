// Package console is the staff console's API under /api/platform/: tenant and
// user lookup, workspace and account state changes, forced sign-out, the audit
// log, internal notes, staff management and the overview.
//
// The console never reads content tables (isolation rule I6). A scan test over
// this package and its queries enforces it.
package console

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
)

const maxBodyBytes = 64 << 10

// StatusSetter changes a workspace's state, with its audit row.
// *workspacestatus.Service is one.
type StatusSetter interface {
	Set(ctx context.Context, c workspacestatus.Change) (changed bool, err error)
}

// AccountCache drops the cached account state. *accountguard.Guard is one.
type AccountCache interface {
	Invalidate(ctx context.Context, userID string) error
}

// AccountNotifier tells the Multica server that an account changed.
// *accountcache.Publisher is one.
type AccountNotifier interface {
	Publish(ctx context.Context, userID, kind string) error
}

// PaymentApplier confirms a bank transfer against an order.
// *billing.Service is one.
type PaymentApplier interface {
	ApplyPayment(ctx context.Context, c billing.Confirmation) (billing.Applied, error)
	// WriteOffUsage cancels a usage invoice as a write-off (spec 7b.3, rule 14).
	WriteOffUsage(ctx context.Context, w billing.WriteOff) (billing.Order, error)
}

// EmailLookup returns a user's email. *staff.Store is one.
type EmailLookup interface {
	EmailByUserID(ctx context.Context, userID string) (string, error)
}

// Mailer tells a user their two-step sign-in was reset.
type Mailer interface {
	SendMFAReset(ctx context.Context, to string) error
}

// Deps are the Handler's dependencies. Now defaults to time.Now and Log to
// slog.Default.
type Deps struct {
	// RequireStaff is platformauth.Handler.RequirePlatformStaff.
	RequireStaff func(roles ...staff.Role) func(http.Handler) http.Handler
	// RequireOrigin is platformauth.Handler.RequireOrigin. It guards every
	// method but GET.
	RequireOrigin func(http.Handler) http.Handler

	Reader     Reader
	Runner     Runner
	Workspaces StatusSetter
	Cache      AccountCache
	Notifier   AccountNotifier
	Payments   PaymentApplier
	// Reconciler decides payment notices. *billing.Reconciler is one.
	Reconciler NoticeResolver
	Accounts   AccountRequests
	// Members serves the staff add-member routes (spec R6 section 4).
	Members MemberRequests
	// Deletions serves the immediate deletion routes (spec R6 section 6).
	Deletions DeletionRequests
	// Quota serves the quota requests of QT7 (spec R6 section 3c.5).
	Quota QuotaRequests
	// TenantStatuses locks and unlocks whole tenants (spec R6 section 3c.9).
	TenantStatuses TenantStatuses
	// LegalHolds places and releases legal holds (spec R6 section 3c.7).
	LegalHolds LegalHolds
	// Einvoices serves the retry route. *invoice.Service is one.
	Einvoices EinvoiceOps
	Logins    LoginLookup
	Emails    EmailLookup
	// Mailer may be nil when no email provider is configured; a reset then
	// sends no notice.
	Mailer Mailer

	Log *slog.Logger
	Now func() time.Time
}

// Handler serves the console routes.
type Handler struct {
	Deps
	mux *http.ServeMux
}

type handlerFunc func(*Handler, http.ResponseWriter, *http.Request)

// routeSpec is one path with the matrix action it needs (spec R6 section 3.6)
// and a handler for each method it answers. A method that is not listed is a
// 404, the same answer as an unknown route.
type routeSpec struct {
	pattern string
	action  string
	// methodActions overrides action for the methods it names.
	methodActions map[string]string
	methods       map[string]handlerFunc
}

// actionFor returns the matrix action of the route for method.
func (rt routeSpec) actionFor(method string) string {
	if a, ok := rt.methodActions[method]; ok {
		return a
	}
	return rt.action
}

// rolesFor returns the staff roles that may call the route with method: the
// roles the matrix gives access to every object, and those it gives access to
// their own tenant, because every handler of this package applies the Scope of
// the request (spec R6 section 3.5, rule 4).
func (rt routeSpec) rolesFor(method string) []staff.Role {
	return staff.RolesScoped(rt.actionFor(method))
}

// routeTable is the route table of the contract, section 4.
func routeTable() []routeSpec {
	const base = "/api/platform"
	return []routeSpec{
		{base + "/tenants", staff.ActionTenantsList, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listTenants}},
		{base + "/tenants/{id}", staff.ActionTenantsRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).getTenant}},
		{base + "/workspaces/{id}", staff.ActionWorkspacesRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).getWorkspace}},
		{base + "/workspaces/{id}/status", staff.ActionWorkspacesLock, nil, map[string]handlerFunc{http.MethodPost: (*Handler).setWorkspaceStatus}},
		{base + "/workspaces/{id}/usage", staff.ActionWorkspacesRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).getWorkspaceUsage}},
		{base + "/workspaces/{id}/audit-events", staff.ActionAuditRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listWorkspaceAuditEvents}},
		{base + "/workspaces/{id}/notes", staff.ActionWorkspacesRead, map[string]string{http.MethodPost: staff.ActionNotesWrite}, map[string]handlerFunc{
			http.MethodGet:  (*Handler).listNotes,
			http.MethodPost: (*Handler).addNote,
		}},
		{base + "/users", staff.ActionUsersRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listUsers}},
		{base + "/users/{id}", staff.ActionUsersRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).getUser}},
		{base + "/users/{id}/status", staff.ActionAccountsControl, nil, map[string]handlerFunc{http.MethodPost: (*Handler).setUserStatus}},
		{base + "/users/{id}/revoke-sessions", staff.ActionAccountsControl, nil, map[string]handlerFunc{http.MethodPost: (*Handler).revokeSessions}},
		{base + "/users/{id}/mfa-reset", staff.ActionAccountsControl, nil, map[string]handlerFunc{http.MethodPost: (*Handler).resetUserMFA}},
		{base + "/staff", staff.ActionStaffManage, nil, map[string]handlerFunc{
			http.MethodGet:  (*Handler).listStaff,
			http.MethodPost: (*Handler).addStaff,
		}},
		{base + "/staff/{user_id}", staff.ActionStaffManage, nil, map[string]handlerFunc{
			http.MethodPut:    (*Handler).updateStaff,
			http.MethodDelete: (*Handler).removeStaff,
		}},
		{base + "/audit-events", staff.ActionAuditRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listAuditEvents}},
		{base + "/orders", staff.ActionOrdersRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listOrders}},
		{base + "/orders/{id}/einvoices", staff.ActionOrdersRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listOrderEinvoices}},
		{base + "/orders/{id}/confirm", staff.ActionOrdersDecide, nil, map[string]handlerFunc{http.MethodPost: (*Handler).confirmOrder}},
		{base + "/orders/{id}/cancel", staff.ActionOrdersDecide, nil, map[string]handlerFunc{http.MethodPost: (*Handler).cancelOrder}},
		{base + "/payment-notices", staff.ActionPaymentsManage, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listPaymentNotices}},
		{base + "/payment-notices/{id}", staff.ActionPaymentsManage, nil, map[string]handlerFunc{http.MethodGet: (*Handler).getPaymentNotice}},
		{base + "/payment-notices/{id}/resolve", staff.ActionPaymentsManage, nil, map[string]handlerFunc{http.MethodPost: (*Handler).resolvePaymentNotice}},
		{base + "/payment-notices/{id}/dismiss", staff.ActionPaymentsManage, nil, map[string]handlerFunc{http.MethodPost: (*Handler).dismissPaymentNotice}},
		{base + "/einvoices", staff.ActionPaymentsManage, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listEinvoices}},
		{base + "/einvoices/{orderId}/retry", staff.ActionPaymentsManage, nil, map[string]handlerFunc{http.MethodPost: (*Handler).retryEinvoice}},
		{base + "/accounts", staff.ActionAccountRequests, nil, map[string]handlerFunc{
			http.MethodGet:  (*Handler).listAccountRequests,
			http.MethodPost: (*Handler).createAccountRequest,
		}},
		{base + "/accounts/{id}", staff.ActionAccountRequests, nil, map[string]handlerFunc{http.MethodGet: (*Handler).getAccountRequest}},
		{base + "/accounts/{id}/cancel", staff.ActionAccountRequests, nil, map[string]handlerFunc{http.MethodPost: (*Handler).cancelAccountRequest}},
		{base + "/accounts/{id}/resend-welcome", staff.ActionAccountRequests, nil, map[string]handlerFunc{http.MethodPost: (*Handler).resendWelcome}},
		{base + "/workspaces/{id}/members", staff.ActionMembersAdd, nil, map[string]handlerFunc{http.MethodPost: (*Handler).addMember}},
		{base + "/workspaces/{id}/member-requests", staff.ActionMemberRequests, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listMemberRequests}},
		{base + "/member-requests/{id}", staff.ActionMemberRequests, nil, map[string]handlerFunc{http.MethodGet: (*Handler).getMemberRequest}},
		{base + "/member-requests/{id}/cancel", staff.ActionMemberRequests, nil, map[string]handlerFunc{http.MethodPost: (*Handler).cancelMemberRequest}},
		{base + "/workspaces/{id}/delete-now", staff.ActionWorkspaceDeleteNow, nil, map[string]handlerFunc{http.MethodPost: (*Handler).deleteWorkspaceNow}},
		{base + "/tenants/{id}/delete-now", staff.ActionTenantDeleteNow, nil, map[string]handlerFunc{http.MethodPost: (*Handler).deleteTenantNow}},
		{base + "/workspaces", staff.ActionWorkspacesList, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listWorkspaces}},
		{base + "/tenants/{id}/policy", staff.ActionTenantPolicyWrite, map[string]string{http.MethodGet: staff.ActionTenantPolicyRead}, map[string]handlerFunc{
			http.MethodGet: (*Handler).getTenantPolicy,
			http.MethodPut: (*Handler).putTenantPolicy,
		}},
		{base + "/quota-requests", staff.ActionQuotaRequestsRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listQuotaRequests}},
		{base + "/quota-requests/{id}", staff.ActionQuotaRequestsRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).getQuotaRequest}},
		{base + "/quota-requests/{id}/approve", staff.ActionQuotaRequestDecide, nil, map[string]handlerFunc{http.MethodPost: (*Handler).approveQuotaRequest}},
		{base + "/quota-requests/{id}/deny", staff.ActionQuotaRequestDecide, nil, map[string]handlerFunc{http.MethodPost: (*Handler).denyQuotaRequest}},
		{base + "/workspaces/{id}/quota-requests", staff.ActionQuotaRequestCreate, nil, map[string]handlerFunc{http.MethodPost: (*Handler).createQuotaRequest}},
		{base + "/audit-events/export", staff.ActionAuditExport, nil, map[string]handlerFunc{http.MethodGet: (*Handler).exportAudit}},
		{base + "/tenants/{id}/status", staff.ActionTenantStatus, nil, map[string]handlerFunc{http.MethodPost: (*Handler).setTenantStatus}},
		{base + "/approvals", staff.ActionApprovalsDecide, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listApprovals}},
		{base + "/approvals/{id}/approve", staff.ActionApprovalsDecide, nil, map[string]handlerFunc{http.MethodPost: (*Handler).approveApproval}},
		{base + "/approvals/{id}/reject", staff.ActionApprovalsDecide, nil, map[string]handlerFunc{http.MethodPost: (*Handler).rejectApproval}},
		{base + "/legal-holds", staff.ActionLegalHoldManage, map[string]string{}, map[string]handlerFunc{
			http.MethodGet:  (*Handler).listLegalHolds,
			http.MethodPost: (*Handler).placeLegalHold,
		}},
		{base + "/legal-holds/{id}/release", staff.ActionLegalHoldManage, nil, map[string]handlerFunc{http.MethodPost: (*Handler).releaseLegalHold}},
		{base + "/runtimes", staff.ActionRuntimesRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listRuntimes}},
		{base + "/runtimes/{id}", staff.ActionRuntimesRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).getRuntime}},
		{base + "/runtimes/{id}/status", staff.ActionRuntimeStatus, nil, map[string]handlerFunc{http.MethodPost: (*Handler).setRuntimeStatus}},
		{base + "/runtimes/{id}/unregister", staff.ActionRuntimeUnregister, nil, map[string]handlerFunc{http.MethodPost: (*Handler).unregisterRuntime}},
		{base + "/tenants/{id}/users", staff.ActionTenantUsersRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listTenantUsers}},
		{base + "/tenants/{id}/users/{user_id}", staff.ActionTenantUsersRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).getTenantUser}},
		{base + "/tenants/{id}/users/{user_id}/deactivate", staff.ActionMembersRemove, nil, map[string]handlerFunc{http.MethodPost: (*Handler).deactivateTenantUser}},
		{base + "/workspaces/{id}/members/{user_id}/remove", staff.ActionMembersRemove, nil, map[string]handlerFunc{http.MethodPost: (*Handler).removeWorkspaceMember}},
		{base + "/workspaces/{id}/members/{user_id}/role", staff.ActionMembersChangeRole, nil, map[string]handlerFunc{http.MethodPost: (*Handler).changeWorkspaceMemberRole}},
		{base + "/workspaces/{id}/export", staff.ActionWorkspaceExport, nil, map[string]handlerFunc{http.MethodPost: (*Handler).exportWorkspace}},
		{base + "/users/{id}/delete-now", staff.ActionAccountDeleteNow, nil, map[string]handlerFunc{http.MethodPost: (*Handler).deleteAccountNow}},
		{base + "/users/{id}/delete-now-preview", staff.ActionAccountDeleteNow, nil, map[string]handlerFunc{http.MethodGet: (*Handler).previewAccountDeleteNow}},
		{base + "/deletion-requests", staff.ActionDeletionRequestsRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).listDeletionRequests}},
		{base + "/deletion-requests/{id}", staff.ActionDeletionRequestsRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).getDeletionRequest}},
		{base + "/deletion-requests/{id}/cancel", staff.ActionDeletionRequestsAct, nil, map[string]handlerFunc{http.MethodPost: (*Handler).cancelDeletionRequest}},
		{base + "/deletion-requests/{id}/retry", staff.ActionDeletionRequestsAct, nil, map[string]handlerFunc{http.MethodPost: (*Handler).retryDeletionRequest}},
		{base + "/overview", staff.ActionOverviewRead, nil, map[string]handlerFunc{http.MethodGet: (*Handler).overview}},
	}
}

// Patterns returns the path patterns the Handler serves, for the API mux.
func Patterns() []string {
	var out []string
	for _, r := range routeTable() {
		out = append(out, r.pattern)
	}
	return out
}

// New builds the Handler. Every method of every route is wrapped by the staff
// middleware for the roles of its action, so a refused request and a wrong
// method both get the 404 of an unknown route.
func New(d Deps) *Handler {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	h := &Handler{Deps: d, mux: http.NewServeMux()}
	for _, rt := range routeTable() {
		h.mux.Handle(rt.pattern, h.methodSwitch(rt))
	}
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }

func (h *Handler) methodSwitch(rt routeSpec) http.Handler {
	calls := map[string]http.Handler{}
	for method, fn := range rt.methods {
		roles := rt.rolesFor(method)
		if len(roles) == 0 {
			// An empty list would admit every staff role.
			panic("console: " + method + " " + rt.pattern + " lists no staff role")
		}
		call := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fn(h, w, r) })
		var inner http.Handler = call
		if method != http.MethodGet {
			inner = h.RequireOrigin(call)
		}
		calls[method] = h.RequireStaff(roles...)(inner)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call, ok := calls[r.Method]
		if !ok {
			http.NotFound(w, r)
			return
		}
		call.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, e *apiError) {
	writeJSON(w, e.Status, map[string]string{"error": e.Message, "code": e.Code})
}

func notFound() *apiError {
	return &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "not found"}
}

// fail answers with err when it is an *apiError and with a 500 otherwise. The
// cause of a 500 goes to the log only.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, op string, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		writeError(w, ae)
		return
	}
	h.Log.Error("console: "+op+" failed", "path", r.URL.Path, "error", err)
	writeError(w, &apiError{Status: http.StatusInternalServerError, Code: "internal", Message: "internal error"})
}

// decodeBody reads one JSON object into v. Unknown fields and trailing data
// are refused.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) *apiError {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return badRequest("invalid_body", "the request body is not a valid JSON object for this route")
	}
	if dec.More() {
		return badRequest("invalid_body", "the request body has more than one JSON value")
	}
	return nil
}

// canonicalUUID returns the lower-case text form of a UUID.
func canonicalUUID(s string) (string, bool) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil || !u.Valid {
		return "", false
	}
	b := u.Bytes
	return fmtUUID(b), true
}

func fmtUUID(b [16]byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for i, c := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out = append(out, '-')
		}
		out = append(out, hexDigits[c>>4], hexDigits[c&0x0f])
	}
	return string(out)
}

// pathID returns the canonical UUID in the path wildcard name. A value that is
// not a UUID names nothing, so it is a 404.
func pathID(r *http.Request, name string) (string, *apiError) {
	id, ok := canonicalUUID(r.PathValue(name))
	if !ok {
		return "", notFound()
	}
	return id, nil
}

func forbidden() *apiError {
	return &apiError{Status: http.StatusForbidden, Code: "forbidden", Message: "your role cannot do this"}
}

// actor is the signed-in staff member as the audit log records them.
type actor struct {
	ID    string
	Email string
	Role  staff.Role
}

// currentActor returns the staff member of the request, with their email for
// the audit label.
func (h *Handler) currentActor(r *http.Request) (actor, error) {
	id, ok := platformauth.IdentityFrom(r.Context())
	if !ok {
		return actor{}, errors.New("no staff identity in the request context")
	}
	email, err := h.Emails.EmailByUserID(r.Context(), id.UserID)
	if err != nil {
		return actor{}, err
	}
	return actor{ID: id.UserID, Email: email, Role: id.Role}, nil
}

// staffEntry fills the actor fields of an audit entry.
func (a actor) staffEntry(e audit.Entry) audit.Entry {
	e.ActorType = audit.ActorStaff
	e.ActorID = a.ID
	e.ActorLabel = a.Email
	return e
}
