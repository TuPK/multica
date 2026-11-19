package console

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/usage"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func tsPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := ts(*t)
	return &s
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nextCursor returns the cursor of the last row kept, or nil when the page
// holds every remaining row. Readers return one row more than limit.
func nextCursor[T any](rows []T, limit int, key func(T) (time.Time, string)) ([]T, *string) {
	if len(rows) <= limit {
		return rows, nil
	}
	rows = rows[:limit]
	at, id := key(rows[len(rows)-1])
	c := encodeCursor(at, id)
	return rows, &c
}

type workspaceJSON struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Status         string `json:"status"`
	MemberCount    int64  `json:"member_count"`
	CreatedAt      string `json:"created_at"`
	LastActivityAt string `json:"last_activity_at"`
	// The newest daily usage row. A workspace with none has a null usage_day and zeros.
	usage.Snapshot
}

func toWorkspaceJSON(t WorkspaceRow) workspaceJSON {
	return workspaceJSON{
		ID: t.ID, Name: t.Name, Slug: t.Slug, Status: t.Status, MemberCount: t.MemberCount,
		CreatedAt: ts(t.CreatedAt), LastActivityAt: ts(t.LastActivityAt), Snapshot: t.Usage,
	}
}

type stateJSON struct {
	Status              string  `json:"status"`
	Reason              string  `json:"reason"`
	ChangedBy           *string `json:"changed_by"`
	ChangedAt           *string `json:"changed_at"`
	DeletionScheduledAt *string `json:"deletion_scheduled_at"`
}

func toStateJSON(d WorkspaceDetail) stateJSON {
	return stateJSON{
		Status:              d.Status,
		Reason:              d.StateReason,
		ChangedBy:           strPtr(d.StateChangedBy),
		ChangedAt:           tsPtr(d.StateChangedAt),
		DeletionScheduledAt: tsPtr(d.DeletionScheduledAt),
	}
}

type ownerJSON struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type workspaceDetailJSON struct {
	workspaceJSON
	State      stateJSON `json:"state"`
	NotesCount int64     `json:"notes_count"`
	// GroupCount is the number of account groups the workspace has (spec 7b.2).
	GroupCount int64      `json:"group_count"`
	Owner      *ownerJSON `json:"owner"`
	// RecentOrders are the workspace's newest billing orders, newest first.
	RecentOrders []orderJSON `json:"recent_orders"`
	// UsageCurrent counts the workspace now.
	UsageCurrent usage.Current `json:"usage_current"`
}

type tenantJSON struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Status         string           `json:"status"`
	WorkspaceCount int64            `json:"workspace_count"`
	MaxWorkspaces  *int64           `json:"max_workspaces"`
	BestPlanKey    string           `json:"best_plan_key"`
	StatusSummary  map[string]int64 `json:"status_summary"`
	CreatedAt      string           `json:"created_at"`
	LastActivityAt string           `json:"last_activity_at"`
}

func toTenantJSON(t TenantRow) tenantJSON {
	summary := t.StatusSummary
	if summary == nil {
		summary = map[string]int64{}
	}
	return tenantJSON{
		ID: t.ID, Name: t.Name, Status: t.Status, WorkspaceCount: t.WorkspaceCount, MaxWorkspaces: t.MaxWorkspaces,
		BestPlanKey: t.BestPlanKey, StatusSummary: summary, CreatedAt: ts(t.CreatedAt), LastActivityAt: ts(t.LastActivityAt),
	}
}

// tenantWorkspaceJSON is the summary of one workspace of a tenant. Counts and
// names only (isolation rule I6).
type tenantWorkspaceJSON struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Status      string     `json:"status"`
	PlanKey     string     `json:"plan_key"`
	MemberCount int64      `json:"member_count"`
	Owner       *ownerJSON `json:"owner"`
	CreatedAt   string     `json:"created_at"`
	usage.Snapshot
}

type tenantDetailJSON struct {
	tenantJSON
	CreatedBy  *string               `json:"created_by"`
	Workspaces []tenantWorkspaceJSON `json:"workspaces"`
}

func (h *Handler) listTenants(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionTenantsList)
	if !ok {
		return
	}
	q := r.URL.Query()
	p, perr := parseListParams(q)
	if perr != nil {
		writeError(w, perr)
		return
	}
	pattern, perr := likePattern(q.Get("q"))
	if perr != nil {
		writeError(w, perr)
		return
	}
	rows, err := h.Reader.SearchTenants(r.Context(), TenantSearch{Pattern: pattern, Cursor: p.cursor, Limit: p.limit, Scope: scope})
	if err != nil {
		h.fail(w, r, "search tenants", err)
		return
	}
	rows, next := nextCursor(rows, p.limit, func(t TenantRow) (time.Time, string) { return t.CreatedAt, t.ID })
	out := make([]tenantJSON, len(rows))
	for i, t := range rows {
		out[i] = toTenantJSON(t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": out, "next_cursor": next})
}

func (h *Handler) getTenant(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionTenantsRead)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	d, found, err := h.Reader.GetTenant(r.Context(), id, scope)
	if err != nil {
		h.fail(w, r, "get tenant", err)
		return
	}
	if !found {
		writeError(w, notFound())
		return
	}
	out := tenantDetailJSON{tenantJSON: toTenantJSON(d.TenantRow), CreatedBy: strPtr(d.CreatedBy), Workspaces: make([]tenantWorkspaceJSON, len(d.Workspaces))}
	for i, ws := range d.Workspaces {
		out.Workspaces[i] = tenantWorkspaceJSON{
			ID: ws.ID, Name: ws.Name, Slug: ws.Slug, Status: ws.Status, PlanKey: ws.PlanKey, MemberCount: ws.MemberCount,
			CreatedAt: ts(ws.CreatedAt), Snapshot: ws.Usage,
		}
		if ws.OwnerID != "" {
			out.Workspaces[i].Owner = &ownerJSON{ID: ws.OwnerID, Email: ws.OwnerEmail}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) getWorkspace(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionWorkspacesRead)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if visible, err := h.workspaceVisible(r.Context(), scope, id); err != nil {
		h.fail(w, r, "check workspace scope", err)
		return
	} else if !visible {
		writeError(w, notFound())
		return
	}
	d, found, err := h.Reader.GetWorkspace(r.Context(), id)
	if err != nil {
		h.fail(w, r, "get tenant", err)
		return
	}
	if !found {
		writeError(w, notFound())
		return
	}
	orders, err := h.Reader.RecentOrders(r.Context(), id, recentOrdersLimit)
	if err != nil {
		h.fail(w, r, "recent orders", err)
		return
	}
	out := workspaceDetailJSON{
		workspaceJSON: toWorkspaceJSON(d.WorkspaceRow), State: toStateJSON(d), NotesCount: d.NotesCount,
		GroupCount: d.GroupCount, RecentOrders: toOrdersJSON(orders), UsageCurrent: d.Current,
	}
	if d.OwnerID != "" {
		out.Owner = &ownerJSON{ID: d.OwnerID, Email: d.OwnerEmail}
	}
	writeJSON(w, http.StatusOK, out)
}

// getWorkspaceUsage is the usage tab of a workspace: this month's rows, the days
// with no usage numbers, and the unpaid usage invoices. It carries counts and
// money only (isolation rule I6).
func (h *Handler) getWorkspaceUsage(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionWorkspacesRead)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if visible, err := h.workspaceVisible(r.Context(), scope, id); err != nil {
		h.fail(w, r, "check workspace scope", err)
		return
	} else if !visible {
		writeError(w, notFound())
		return
	}
	if _, found, err := h.Reader.GetWorkspace(r.Context(), id); err != nil {
		h.fail(w, r, "get tenant", err)
		return
	} else if !found {
		writeError(w, notFound())
		return
	}
	m, err := h.Reader.WorkspaceUsage(r.Context(), id, h.Now())
	if err != nil {
		h.fail(w, r, "tenant usage", err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

type userJSON struct {
	ID               string `json:"id"`
	Email            string `json:"email"`
	Name             string `json:"name"`
	Status           string `json:"status"`
	MembershipsCount int64  `json:"memberships_count"`
	CreatedAt        string `json:"created_at"`
}

func toUserJSON(u UserRow) userJSON {
	return userJSON{
		ID: u.ID, Email: u.Email, Name: u.Name, Status: u.Status,
		MembershipsCount: u.MembershipsCount, CreatedAt: ts(u.CreatedAt),
	}
}

type membershipJSON struct {
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
}

type accountJSON struct {
	Status             string  `json:"status"`
	Reason             string  `json:"reason"`
	ChangedAt          *string `json:"changed_at"`
	SessionsValidAfter *string `json:"sessions_valid_after"`
}

type userDetailJSON struct {
	userJSON
	Memberships []membershipJSON `json:"memberships"`
	Account     accountJSON      `json:"account"`
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionUsersRead)
	if !ok {
		return
	}
	q := r.URL.Query()
	p, perr := parseListParams(q)
	if perr != nil {
		writeError(w, perr)
		return
	}
	pattern, perr := likePattern(q.Get("q"))
	if perr != nil {
		writeError(w, perr)
		return
	}
	rows, err := h.Reader.SearchUsers(r.Context(), UserSearch{Pattern: pattern, Cursor: p.cursor, Limit: p.limit, Scope: scope})
	if err != nil {
		h.fail(w, r, "search users", err)
		return
	}
	rows, next := nextCursor(rows, p.limit, func(u UserRow) (time.Time, string) { return u.CreatedAt, u.ID })
	out := make([]userJSON, len(rows))
	for i, u := range rows {
		out[i] = toUserJSON(u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out, "next_cursor": next})
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionUsersRead)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if !scope.All {
		if visible, err := h.Reader.UserInScope(r.Context(), id, scope); err != nil {
			h.fail(w, r, "check user scope", err)
			return
		} else if !visible {
			writeError(w, notFound())
			return
		}
	}
	d, found, err := h.Reader.GetUser(r.Context(), id)
	if err != nil {
		h.fail(w, r, "get user", err)
		return
	}
	if !found {
		writeError(w, notFound())
		return
	}
	out := userDetailJSON{
		userJSON:    toUserJSON(d.UserRow),
		Memberships: make([]membershipJSON, len(d.Memberships)),
		Account: accountJSON{
			Status:             d.Status,
			Reason:             d.AccountReason,
			ChangedAt:          tsPtr(d.AccountChangedAt),
			SessionsValidAfter: tsPtr(d.SessionsValidAfter),
		},
	}
	for i, m := range d.Memberships {
		out.Memberships[i] = membershipJSON{WorkspaceID: m.WorkspaceID, Name: m.Name, Role: m.Role}
	}
	writeJSON(w, http.StatusOK, out)
}

type noteJSON struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	AuthorID    string  `json:"author_id"`
	AuthorEmail *string `json:"author_email"`
	Body        string  `json:"body"`
	CreatedAt   string  `json:"created_at"`
}

func toNoteJSON(n Note) noteJSON {
	return noteJSON{
		ID: n.ID, WorkspaceID: n.WorkspaceID, AuthorID: n.AuthorID, AuthorEmail: strPtr(n.AuthorEmail),
		Body: n.Body, CreatedAt: ts(n.CreatedAt),
	}
}

func (h *Handler) listNotes(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionWorkspacesRead)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if visible, err := h.workspaceVisible(r.Context(), scope, id); err != nil {
		h.fail(w, r, "check workspace scope", err)
		return
	} else if !visible {
		writeError(w, notFound())
		return
	}
	p, perr := parseListParams(r.URL.Query())
	if perr != nil {
		writeError(w, perr)
		return
	}
	rows, err := h.Reader.ListNotes(r.Context(), NoteQuery{WorkspaceID: id, Cursor: p.cursor, Limit: p.limit})
	if err != nil {
		h.fail(w, r, "list notes", err)
		return
	}
	rows, next := nextCursor(rows, p.limit, func(n Note) (time.Time, string) { return n.CreatedAt, n.ID })
	out := make([]noteJSON, len(rows))
	for i, n := range rows {
		out[i] = toNoteJSON(n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": out, "next_cursor": next})
}

type staffJSON struct {
	UserID    string  `json:"user_id"`
	Email     string  `json:"email"`
	Role      string  `json:"role"`
	TenantID  *string `json:"tenant_id"`
	CreatedBy *string `json:"created_by"`
	CreatedAt string  `json:"created_at"`
}

func (h *Handler) listStaff(w http.ResponseWriter, r *http.Request) {
	if !h.requireStaffManager(w, r) {
		return
	}
	rows, err := h.Reader.ListStaff(r.Context())
	if err != nil {
		h.fail(w, r, "list staff", err)
		return
	}
	out := make([]staffJSON, len(rows))
	for i, s := range rows {
		out[i] = staffJSON{UserID: s.UserID, Email: s.Email, Role: s.Role, TenantID: strPtr(s.TenantID), CreatedBy: strPtr(s.CreatedBy), CreatedAt: ts(s.CreatedAt)}
	}
	writeJSON(w, http.StatusOK, map[string]any{"staff": out})
}

// auditEventJSON is the wire shape of the workspace log in Multica.
type auditEventJSON struct {
	ID          string          `json:"id"`
	WorkspaceID *string         `json:"workspace_id"`
	ActorType   string          `json:"actor_type"`
	ActorID     *string         `json:"actor_id"`
	ActorLabel  string          `json:"actor_label"`
	Action      string          `json:"action"`
	TargetType  string          `json:"target_type"`
	TargetID    string          `json:"target_id"`
	TargetLabel string          `json:"target_label"`
	Detail      json.RawMessage `json:"detail"`
	IP          string          `json:"ip"`
	UserAgent   string          `json:"user_agent"`
	RequestID   string          `json:"request_id"`
	CreatedAt   string          `json:"created_at"`
}

func toAuditEventJSON(e AuditRow) auditEventJSON {
	detail := e.Detail
	if len(detail) == 0 {
		detail = []byte("{}")
	}
	return auditEventJSON{
		ID: e.ID, WorkspaceID: strPtr(e.WorkspaceID), ActorType: e.ActorType, ActorID: strPtr(e.ActorID),
		ActorLabel: e.ActorLabel, Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID,
		TargetLabel: e.TargetLabel, Detail: detail, IP: e.IP, UserAgent: e.UserAgent,
		RequestID: e.RequestID, CreatedAt: ts(e.CreatedAt),
	}
}

// auditPage serves an audit list for workspaceID, or for every row when it is
// empty.
func (h *Handler) auditPage(w http.ResponseWriter, r *http.Request, workspaceID string, scope Scope) {
	p, perr := parseAuditParams(r.URL.Query())
	if perr != nil {
		writeError(w, perr)
		return
	}
	rows, err := h.Reader.ListAuditEvents(r.Context(), AuditQuery{
		Scope: scope, WorkspaceID: workspaceID, Action: p.action, ActorID: p.actorID, From: p.from, To: p.to,
		Cursor: p.cursor, Limit: p.limit,
	})
	if err != nil {
		h.fail(w, r, "list audit events", err)
		return
	}
	rows, next := nextCursor(rows, p.limit, func(e AuditRow) (time.Time, string) { return e.CreatedAt, e.ID })
	events := make([]auditEventJSON, len(rows))
	for i, e := range rows {
		events[i] = toAuditEventJSON(e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "next_cursor": next})
}

// listWorkspaceAuditEvents does not check that the workspace exists: the rows of a
// deleted workspace stay readable.
func (h *Handler) listWorkspaceAuditEvents(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionAuditRead)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if visible, err := h.workspaceVisible(r.Context(), scope, id); err != nil {
		h.fail(w, r, "check workspace scope", err)
		return
	} else if !visible {
		writeError(w, notFound())
		return
	}
	h.auditPage(w, r, id, scope)
}

func (h *Handler) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionAuditRead)
	if !ok {
		return
	}
	h.auditPage(w, r, "", scope)
}

const signupWindow = 7 * 24 * time.Hour

// overview returns the workspace counts by state and by plan, the last week's
// sign-ups, and the pending orders' amounts grouped by currency.
// Active is the rest: the total less the workspaces in another state.
func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	o, err := h.Reader.Overview(r.Context(), h.Now().Add(-signupWindow))
	if err != nil {
		h.fail(w, r, "overview", err)
		return
	}
	byStatus := map[string]int64{
		string(workspacestate.ReadOnly):        o.NonActive[string(workspacestate.ReadOnly)],
		string(workspacestate.Suspended):       o.NonActive[string(workspacestate.Suspended)],
		string(workspacestate.PendingDeletion): o.NonActive[string(workspacestate.PendingDeletion)],
	}
	active := o.WorkspacesTotal
	for _, n := range byStatus {
		active -= n
	}
	byStatus[string(workspacestate.Active)] = max(active, 0)
	byPlan := make(map[string]int64, len(o.ByPlan))
	for key, n := range o.ByPlan {
		byPlan[key] = n
	}
	pendingByCurrency := make(map[string]any, len(o.PendingByCurrency))
	for currency, t := range o.PendingByCurrency {
		pendingByCurrency[currency] = map[string]int64{"orders": t.Orders, "amount": t.Amount, "vat_amount": t.VATAmount}
	}
	body := map[string]any{
		"workspaces_without_tenant": o.WorkspacesWithoutTenant,
		"workspaces_by_status":      byStatus, "signups_7d": o.Signups, "by_plan": byPlan, "pending_orders": o.PendingOrders,
		"pending_by_currency": pendingByCurrency,
	}
	for k, v := range o.Usage.Map() {
		body[k] = v
	}
	writeJSON(w, http.StatusOK, body)
}
