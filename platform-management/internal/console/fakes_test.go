package console

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/usage"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

const (
	secret = "0123456789abcdef0123456789abcdef"
	origin = "https://console.example.com"

	adminID   = "aaaaaaaa-0000-0000-0000-000000000001"
	adminMail = "admin@example.com"
	suppID    = "aaaaaaaa-0000-0000-0000-000000000002"
	suppMail  = "support@example.com"
	billID    = "aaaaaaaa-0000-0000-0000-000000000003"
	billMail  = "billing@example.com"

	// tadminID is an admin of the tenant tenantID.
	tadminID      = "aaaaaaaa-0000-0000-0000-000000000004"
	tadminMail    = "tenantadmin@example.com"
	tenantID      = "eeeeeeee-0000-0000-0000-000000000001"
	otherTenantID = "eeeeeeee-0000-0000-0000-000000000002"

	wsID     = "bbbbbbbb-0000-0000-0000-000000000001"
	wsName   = "Acme"
	custID   = "cccccccc-0000-0000-0000-000000000001"
	custMail = "customer@example.com"
	noOneID  = "dddddddd-0000-0000-0000-000000000009"
)

var t0 = time.Date(2030, 5, 6, 7, 8, 9, 123456789, time.UTC)

// fakeDirectory is the staff table and the user table for the middleware and
// for the audit label.
type fakeDirectory struct {
	roles   map[string]staff.Role
	tenants map[string]string // user id -> tenant of an admin row
	emails  map[string]string
}

func (d *fakeDirectory) UserIDByEmail(_ context.Context, email string) (string, bool, error) {
	for id, e := range d.emails {
		if e == email {
			return id, true, nil
		}
	}
	return "", false, nil
}

func (d *fakeDirectory) ActiveStaff(_ context.Context, id string) (staff.Role, string, bool, error) {
	r, ok := d.roles[id]
	return r, d.tenants[id], ok, nil
}

func (d *fakeDirectory) EmailByUserID(_ context.Context, id string) (string, error) {
	e, ok := d.emails[id]
	if !ok {
		return "", errors.New("no such user")
	}
	return e, nil
}

type activeGuard struct{}

func (activeGuard) State(context.Context, string) (accountguard.Record, error) {
	return accountguard.Record{Status: accountguard.Active}, nil
}

// fakeReader serves canned rows and records what it was asked.
type fakeReader struct {
	details  map[string]WorkspaceDetail
	users    []UserRow
	userDets map[string]UserDetail
	notes    []Note
	staff    []StaffRow
	events   []AuditRow
	overview Overview
	err      error
	store    *fakeStore // the orders ListOrders, GetOrder and RecentOrders read

	invoiceStates map[string]string               // order id -> state of its latest e-invoice revision
	history       map[string][]EinvoiceHistoryRow // order id -> revisions, newest first
	historyErr    error

	tenantRows    []TenantRow
	tenantDetails map[string]TenantDetail
	wsTenant      map[string]string // workspace id -> tenant id
	userTenant    map[string]string // user id -> tenant id of one of its workspaces
	scopeChecks   []string          // the workspaces WorkspaceInScope was asked about

	lastTenants    TenantSearch
	workspaceList  []WorkspaceListRow
	lastWorkspaces WorkspaceSearch
	tenantUsers    []TenantUser
	runtimes       []RuntimeRow
	runtimeTasks   map[string][]RuntimeTask
	runtimeFailed  int64
	// memberRoles is keyed workspace id + "/" + user id; staffUsers holds active staff.
	memberRoles        map[string]string
	memberMail         map[string]string
	staffUsers         map[string]bool
	lastGetTenantScope Scope
	lastOrderScope     Scope
	lastUsers          UserSearch
	lastNotes          NoteQuery
	lastAudit          AuditQuery
	lastSince          time.Time

	lastOrderStatus billing.Status
	lastNewestFirst bool
	lastOrderLimit  int
	lastRecentWS    string
	lastRecentLimit int
}

func (r *fakeReader) ListOrders(_ context.Context, status billing.Status, newestFirst bool, limit int, scope Scope) ([]OrderRow, error) {
	r.lastOrderStatus, r.lastNewestFirst, r.lastOrderLimit, r.lastOrderScope = status, newestFirst, limit, scope
	var out []OrderRow
	for _, o := range r.store.orders {
		inScope := scope.All || (scope.TenantID != "" && r.wsTenant[o.WorkspaceID] == scope.TenantID)
		if o.Status == status && inScope {
			row := r.store.orderRow(o)
			row.InvoiceState = r.invoiceStates[o.ID]
			out = append(out, row)
		}
	}
	sortOrders(out, newestFirst)
	return out, r.err
}

func (r *fakeReader) ListOrderEinvoices(_ context.Context, orderID string) ([]EinvoiceHistoryRow, error) {
	return r.history[orderID], r.historyErr
}

func (r *fakeReader) WorkspaceUsage(_ context.Context, ws string, _ time.Time) (usage.Month, error) {
	return usage.Month{WorkspaceID: ws}, nil
}

func (r *fakeReader) GetOrder(_ context.Context, id string) (OrderRow, bool, error) {
	o, ok := r.store.orders[id]
	if !ok {
		return OrderRow{}, false, r.err
	}
	return r.store.orderRow(o), true, r.err
}

func (r *fakeReader) RecentOrders(_ context.Context, workspaceID string, limit int) ([]OrderRow, error) {
	r.lastRecentWS, r.lastRecentLimit = workspaceID, limit
	var out []OrderRow
	for _, o := range r.store.orders {
		if o.WorkspaceID == workspaceID {
			out = append(out, r.store.orderRow(o))
		}
	}
	sortOrders(out, true)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, r.err
}

func sortOrders(rows []OrderRow, newestFirst bool) {
	sort.Slice(rows, func(i, j int) bool {
		if newestFirst {
			return rows[i].CreatedAt.After(rows[j].CreatedAt)
		}
		return rows[i].CreatedAt.Before(rows[j].CreatedAt)
	})
}

// SearchRuntimes returns the fake runtimes within the scope and status filter.
func (r *fakeReader) SearchRuntimes(_ context.Context, q RuntimeSearch) ([]RuntimeRow, error) {
	var out []RuntimeRow
	for _, x := range r.runtimes {
		if !runtimeVisible(q.Scope, x.TenantID) || (q.Status != "" && x.Status != q.Status) {
			continue
		}
		out = append(out, x)
	}
	return out, nil
}

// RuntimeDetail returns a fake runtime by id.
func (r *fakeReader) RuntimeDetail(_ context.Context, id string) (RuntimeRow, bool, error) {
	for _, x := range r.runtimes {
		if x.ID == id {
			return x, true, nil
		}
	}
	return RuntimeRow{}, false, nil
}

// RuntimeTasks returns the fake tasks and failure count of a runtime.
func (r *fakeReader) RuntimeTasks(_ context.Context, id string) ([]RuntimeTask, int64, error) {
	return r.runtimeTasks[id], r.runtimeFailed, nil
}

// SetRuntimeDisabled records the disabled state on the fake store and reports a change.
func (t *fakeTx) SetRuntimeDisabled(_ context.Context, id string, disabled bool, _, _ string) (bool, error) {
	if t.s.runtimeDisabled == nil {
		t.s.runtimeDisabled = map[string]bool{}
	}
	if t.s.runtimeDisabled[id] == disabled {
		return false, nil
	}
	t.s.runtimeDisabled[id] = disabled
	return true, nil
}

// TenantUsers returns the fake tenant users; the handler filters staff.
func (r *fakeReader) TenantUsers(_ context.Context, q TenantUserSearch) ([]TenantUser, error) {
	var out []TenantUser
	for _, u := range r.tenantUsers {
		if u.ID > q.CursorUserID {
			out = append(out, u)
		}
	}
	return out, nil
}

// TenantUser returns a fake tenant user whose memberships are in the tenant.
func (r *fakeReader) TenantUser(_ context.Context, tenantID, userID string) (TenantUser, bool, error) {
	for _, u := range r.tenantUsers {
		if u.ID == userID {
			return u, true, nil
		}
	}
	return TenantUser{}, false, nil
}

// WorkspaceMember returns the fake member row.
func (r *fakeReader) WorkspaceMember(_ context.Context, wsID, userID string) (string, string, bool, error) {
	role, ok := r.memberRoles[wsID+"/"+userID]
	return role, r.memberMail[userID], ok, nil
}

// IsActiveStaff reports the fake staff set.
func (r *fakeReader) IsActiveStaff(_ context.Context, userID string) (bool, error) {
	return r.staffUsers[userID], nil
}

// InsertMemberChange records the request on the fake store and returns its id.
func (t *fakeTx) InsertMemberChange(_ context.Context, c MemberChange, _, _ string) (string, bool, error) {
	t.s.memberChanges = append(t.s.memberChanges, c)
	return "change-" + c.Kind + "-" + c.TargetUserID, true, nil
}

// WorkspaceTenant returns the tenant of a fake workspace, from wsTenant.
func (r *fakeReader) WorkspaceTenant(_ context.Context, id string) (string, error) {
	return r.wsTenant[id], nil
}

// SearchWorkspaces applies the scope and the filters the way the query does.
func (r *fakeReader) SearchWorkspaces(_ context.Context, q WorkspaceSearch) ([]WorkspaceListRow, error) {
	r.lastWorkspaces = q
	var out []WorkspaceListRow
	for _, w := range r.workspaceList {
		if !q.Scope.All && w.TenantID != q.Scope.TenantID {
			continue
		}
		if q.Status != "" && w.Status != q.Status {
			continue
		}
		if q.TenantFilter != "" && w.TenantID != q.TenantFilter {
			continue
		}
		out = append(out, w)
	}
	return out, nil
}

func (r *fakeReader) SearchTenants(_ context.Context, q TenantSearch) ([]TenantRow, error) {
	r.lastTenants = q
	var out []TenantRow
	for _, t := range r.tenantRows {
		if q.Scope.All || (q.Scope.TenantID != "" && t.ID == q.Scope.TenantID) {
			out = append(out, t)
		}
	}
	return out, r.err
}

func (r *fakeReader) GetTenant(_ context.Context, id string, scope Scope) (TenantDetail, bool, error) {
	r.lastGetTenantScope = scope
	d, ok := r.tenantDetails[id]
	if !ok || !(scope.All || (scope.TenantID != "" && id == scope.TenantID)) {
		return TenantDetail{}, false, r.err
	}
	return d, true, r.err
}

// WorkspaceInScope is true when the workspace's tenant is the scope's.
func (r *fakeReader) WorkspaceInScope(_ context.Context, ws string, scope Scope) (bool, error) {
	r.scopeChecks = append(r.scopeChecks, ws)
	return scope.All || (scope.TenantID != "" && r.wsTenant[ws] == scope.TenantID), r.err
}

func (r *fakeReader) UserInScope(_ context.Context, user string, scope Scope) (bool, error) {
	return scope.All || (scope.TenantID != "" && r.userTenant[user] == scope.TenantID), r.err
}

func (r *fakeReader) GetWorkspace(_ context.Context, id string) (WorkspaceDetail, bool, error) {
	d, ok := r.details[id]
	return d, ok, r.err
}

func (r *fakeReader) SearchUsers(_ context.Context, q UserSearch) ([]UserRow, error) {
	r.lastUsers = q
	return r.users, r.err
}

func (r *fakeReader) GetUser(_ context.Context, id string) (UserDetail, bool, error) {
	d, ok := r.userDets[id]
	return d, ok, r.err
}

func (r *fakeReader) ListNotes(_ context.Context, q NoteQuery) ([]Note, error) {
	r.lastNotes = q
	return r.notes, r.err
}

func (r *fakeReader) ListStaff(context.Context) ([]StaffRow, error) { return r.staff, r.err }

func (r *fakeReader) ListAuditEvents(_ context.Context, q AuditQuery) ([]AuditRow, error) {
	r.lastAudit = q
	return r.events, r.err
}

func (r *fakeReader) Overview(_ context.Context, since time.Time) (Overview, error) {
	r.lastSince = since
	return r.overview, r.err
}

type fakeStaffRow struct {
	user    string
	role    string
	tenant  string
	revoked bool
}

// fakeStore is the writable tables in memory. InTx gives transaction
// semantics: a failing fn leaves nothing behind.
type fakeStore struct {
	mu         sync.Mutex
	workspaces map[string]string
	users      map[string]string // id -> email
	accounts   map[string]string // user id -> status
	sessions   map[string]time.Time
	staffRows  []fakeStaffRow
	notes      []Note
	audits     []audit.Entry
	failAudit  error
	txs        int
	orders     map[string]billing.Order
	subs       map[string]subscription.Subscription
	mfaRows    map[string]bool // user id -> has a user_mfa row
	// tenants are the tenants that exist; nil means every tenant id exists.
	tenants map[string]bool
	// exports are the export rows written; running marks workspaces with one running.
	exports []fakeExport
	running map[string]bool
	// memberChanges are the remove and change_role requests written.
	memberChanges []MemberChange
	// runtimeDisabled is the disabled state of each runtime id.
	runtimeDisabled map[string]bool
}

type fakeExport struct {
	ws string
	by string
}

// RunningExport and InsertExport are the export writes of the fake store.
func (t *fakeTx) RunningExport(_ context.Context, ws string) (bool, error) {
	return t.s.running[ws], nil
}

func (t *fakeTx) InsertExport(_ context.Context, ws, by string) (string, error) {
	t.s.exports = append(t.s.exports, fakeExport{ws: ws, by: by})
	return "export-" + ws, nil
}

// defaultStaffRows is the number of staff rows newFakeStore starts with.
const defaultStaffRows = 4

func newFakeStore() *fakeStore {
	return &fakeStore{
		running:    map[string]bool{},
		workspaces: map[string]string{wsID: wsName},
		users: map[string]string{
			adminID: adminMail, suppID: suppMail, billID: billMail, custID: custMail, tadminID: tadminMail,
		},
		accounts: map[string]string{},
		sessions: map[string]time.Time{},
		orders:   map[string]billing.Order{},
		subs:     map[string]subscription.Subscription{},
		mfaRows:  map[string]bool{custID: true, suppID: true},
		staffRows: []fakeStaffRow{
			{user: adminID, role: "system_admin"},
			{user: suppID, role: "support"},
			{user: billID, role: "billing"},
			{user: tadminID, role: "admin", tenant: tenantID},
		},
	}
}

type snapshot struct {
	exports       []fakeExport
	memberChanges []MemberChange
	running       map[string]bool
	accounts      map[string]string
	sessions      map[string]time.Time
	staffRows     []fakeStaffRow
	notes         []Note
	audits        []audit.Entry
	orders        map[string]billing.Order
	subs          map[string]subscription.Subscription
	mfaRows       map[string]bool
}

func (s *fakeStore) snap() snapshot {
	c := snapshot{accounts: map[string]string{}, sessions: map[string]time.Time{}}
	for k, v := range s.accounts {
		c.accounts[k] = v
	}
	for k, v := range s.sessions {
		c.sessions[k] = v
	}
	c.staffRows = append(c.staffRows, s.staffRows...)
	c.notes = append(c.notes, s.notes...)
	c.audits = append(c.audits, s.audits...)
	c.orders, c.subs = maps.Clone(s.orders), maps.Clone(s.subs)
	c.mfaRows = maps.Clone(s.mfaRows)
	c.exports = append([]fakeExport(nil), s.exports...)
	c.memberChanges = append([]MemberChange(nil), s.memberChanges...)
	c.running = maps.Clone(s.running)
	return c
}

func (s *fakeStore) restore(c snapshot) {
	s.accounts, s.sessions, s.staffRows, s.notes, s.audits = c.accounts, c.sessions, c.staffRows, c.notes, c.audits
	s.orders, s.subs = c.orders, c.subs
	s.mfaRows = c.mfaRows
	s.exports, s.running = c.exports, c.running
	s.memberChanges = c.memberChanges
}

func (s *fakeStore) orderRow(o billing.Order) OrderRow {
	return OrderRow{Order: o, WorkspaceName: s.workspaces[o.WorkspaceID]}
}

func (s *fakeStore) InTx(_ context.Context, fn func(Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.txs++
	before := s.snap()
	if err := fn(&fakeTx{s: s}); err != nil {
		s.restore(before)
		return err
	}
	return nil
}

func (s *fakeStore) activeRole(user string) (string, bool) {
	for _, r := range s.staffRows {
		if r.user == user && !r.revoked {
			return r.role, true
		}
	}
	return "", false
}

func (s *fakeStore) activeTenant(user string) string {
	for _, r := range s.staffRows {
		if r.user == user && !r.revoked {
			return r.tenant
		}
	}
	return ""
}

type fakeTx struct{ s *fakeStore }

func (t *fakeTx) WorkspaceName(_ context.Context, id string) (string, bool, error) {
	n, ok := t.s.workspaces[id]
	return n, ok, nil
}

func (t *fakeTx) InsertNote(_ context.Context, ws, author, body string) (string, time.Time, error) {
	id := fmt.Sprintf("eeeeeeee-0000-0000-0000-%012d", len(t.s.notes)+1)
	at := t0.Add(time.Duration(len(t.s.notes)) * time.Second)
	t.s.notes = append(t.s.notes, Note{ID: id, WorkspaceID: ws, AuthorID: author, Body: body, CreatedAt: at})
	return id, at, nil
}

func (t *fakeTx) UserIDByEmail(_ context.Context, email string) (string, bool, error) {
	for id, e := range t.s.users {
		if e == email {
			return id, true, nil
		}
	}
	return "", false, nil
}

func (t *fakeTx) UserEmail(_ context.Context, id string) (string, bool, error) {
	e, ok := t.s.users[id]
	return e, ok, nil
}

func (t *fakeTx) Account(_ context.Context, id string) (Account, bool, error) {
	st, ok := t.s.accounts[id]
	return Account{Status: st}, ok, nil
}

func (t *fakeTx) SetAccountStatus(_ context.Context, id, status, _, _ string) error {
	t.s.accounts[id] = status
	return nil
}

func (t *fakeTx) SetSessionsValidAfter(_ context.Context, id, _ string, at time.Time) error {
	t.s.sessions[id] = at
	if _, ok := t.s.accounts[id]; !ok {
		t.s.accounts[id] = "active"
	}
	return nil
}

func (t *fakeTx) DeleteUserMFA(_ context.Context, id string) (bool, error) {
	had := t.s.mfaRows[id]
	delete(t.s.mfaRows, id)
	return had, nil
}

func (t *fakeTx) TenantExists(_ context.Context, id string) (bool, error) {
	return t.s.tenants == nil || t.s.tenants[id], nil
}

func (t *fakeTx) ActiveSystemAdmins(context.Context) ([]string, error) {
	var out []string
	for _, r := range t.s.staffRows {
		if r.role == "system_admin" && !r.revoked {
			out = append(out, r.user)
		}
	}
	return out, nil
}

func (t *fakeTx) ActiveStaffRole(_ context.Context, id string) (string, string, bool, error) {
	r, ok := t.s.activeRole(id)
	return r, t.s.activeTenant(id), ok, nil
}

func (t *fakeTx) ReactivateStaff(_ context.Context, id, role, tenant string) (bool, error) {
	for i := len(t.s.staffRows) - 1; i >= 0; i-- {
		if r := &t.s.staffRows[i]; r.user == id && r.revoked {
			r.revoked, r.role, r.tenant = false, role, tenant
			return true, nil
		}
	}
	return false, nil
}

func (t *fakeTx) InsertStaff(_ context.Context, id, role, tenant, _ string) (bool, error) {
	if _, ok := t.s.activeRole(id); ok {
		return false, nil
	}
	t.s.staffRows = append(t.s.staffRows, fakeStaffRow{user: id, role: role, tenant: tenant})
	return true, nil
}

func (t *fakeTx) UpdateStaff(_ context.Context, id, role, tenant string) (bool, error) {
	for i := range t.s.staffRows {
		if r := &t.s.staffRows[i]; r.user == id && !r.revoked {
			r.role, r.tenant = role, tenant
			return true, nil
		}
	}
	return false, nil
}

func (t *fakeTx) RevokeStaff(_ context.Context, id string) (bool, error) {
	for i := range t.s.staffRows {
		if r := &t.s.staffRows[i]; r.user == id && !r.revoked {
			r.revoked = true
			return true, nil
		}
	}
	return false, nil
}

func (t *fakeTx) LockOrder(_ context.Context, id string) (billing.Order, bool, error) {
	o, ok := t.s.orders[id]
	return o, ok, nil
}

func (t *fakeTx) CancelOrder(_ context.Context, id string) (billing.Order, error) {
	o := t.s.orders[id]
	o.Status = billing.StatusCanceled
	t.s.orders[id] = o
	return o, nil
}

func (t *fakeTx) InsertAuditEvent(_ context.Context, e audit.Entry) error {
	if t.s.failAudit != nil {
		return t.s.failAudit
	}
	t.s.audits = append(t.s.audits, e)
	return nil
}

// fakeBillingRunner runs billing.Service transactions on the same store as the
// console's, so a confirm and a cancel see one set of orders and audit rows.
type fakeBillingRunner struct{ s *fakeStore }

func (r fakeBillingRunner) InTx(_ context.Context, fn func(billing.Tx) error) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	before := r.s.snap()
	if err := fn(&fakeBillingTx{fakeTx{s: r.s}}); err != nil {
		r.s.restore(before)
		return err
	}
	return nil
}

type fakeBillingTx struct{ fakeTx }

func (t *fakeBillingTx) InsertOrder(context.Context, billing.NewOrder) (billing.Order, bool, error) {
	return billing.Order{}, false, errors.New("not used")
}

// Plan serves the catalogue to the fake transaction, as the database does.
func (t *fakeBillingTx) Plan(_ context.Context, key string) (plan.Plan, bool, error) {
	all, err := plan.Catalogue()
	if err != nil {
		return plan.Plan{}, false, err
	}
	for _, p := range all {
		if p.Key == key {
			return p, true, nil
		}
	}
	return plan.Plan{}, false, nil
}

func (t *fakeBillingTx) PendingOrder(context.Context, string) (billing.Order, bool, error) {
	return billing.Order{}, false, nil
}

func (t *fakeBillingTx) UnpaidUsageOrder(context.Context, string) (billing.Order, bool, error) {
	return billing.Order{}, false, nil
}
func (t *fakeBillingTx) CancelOrder(context.Context, string) (billing.Order, error) {
	return billing.Order{}, nil
}
func (t *fakeBillingTx) UsageLines(context.Context, string) (int64, error) { return 0, nil }

func (t *fakeBillingTx) OrderByIdempotencyKey(context.Context, string, string) (billing.Order, bool, error) {
	return billing.Order{}, false, errors.New("not used")
}

func (t *fakeBillingTx) MarkPaid(_ context.Context, id, by string, at time.Time) (billing.Order, error) {
	o := t.s.orders[id]
	o.Status, o.ConfirmedBy, o.PaidAt = billing.StatusPaid, by, &at
	t.s.orders[id] = o
	return o, nil
}

func (t *fakeBillingTx) LockSubscription(_ context.Context, ws string) (subscription.Subscription, bool, error) {
	s, ok := t.s.subs[ws]
	return s, ok, nil
}

func (t *fakeBillingTx) InsertSubscription(_ context.Context, s subscription.Subscription) (subscription.Subscription, bool, error) {
	s.Version = 1
	t.s.subs[s.WorkspaceID] = s
	return s, true, nil
}

func (t *fakeBillingTx) UpdateSubscription(_ context.Context, s subscription.Subscription) (subscription.Subscription, error) {
	s.Version = t.s.subs[s.WorkspaceID].Version + 1
	t.s.subs[s.WorkspaceID] = s
	return s, nil
}

func (t *fakeBillingTx) ReadOnlyForBilling(context.Context, string) (bool, error) { return false, nil }

func (t *fakeBillingTx) Current(context.Context, string) (workspacestate.Record, bool, error) {
	return workspacestate.Record{}, false, nil
}

func (t *fakeBillingTx) Write(context.Context, workspacestatus.Change) (bool, error) {
	return false, errors.New("not used")
}

type noopInvalidator struct{}

func (noopInvalidator) Invalidate(context.Context, string) error { return nil }

type noopEntitlements struct{}

func (noopEntitlements) Invalidate(string) {}
func (noopEntitlements) InvalidateAll()    {}

// fakeStatusSetter records the changes passed to workspacestatus.Set.
type fakeStatusSetter struct {
	calls   []workspacestatus.Change
	changed bool
	err     error
}

func (f *fakeStatusSetter) Set(_ context.Context, c workspacestatus.Change) (bool, error) {
	f.calls = append(f.calls, c)
	return f.changed, f.err
}

type fakeCache struct {
	dropped []string
	err     error
}

func (c *fakeCache) Invalidate(_ context.Context, id string) error {
	c.dropped = append(c.dropped, id)
	return c.err
}

type published struct{ user, kind string }

type fakeNotifier struct {
	sent []published
	err  error
}

func (n *fakeNotifier) Publish(_ context.Context, user, kind string) error {
	n.sent = append(n.sent, published{user, kind})
	return n.err
}

// fakeMailer records the reset notices.
type fakeMailer struct {
	sent []string
	err  error
}

func (m *fakeMailer) SendMFAReset(_ context.Context, to string) error {
	m.sent = append(m.sent, to)
	return m.err
}

// env is a console Handler behind the real staff middleware, on fakes.
type env struct {
	t      *testing.T
	h      http.Handler
	dir    *fakeDirectory
	reader *fakeReader
	store  *fakeStore
	ws     *fakeStatusSetter
	cache  *fakeCache
	pub    *fakeNotifier
	mail   *fakeMailer
	logs   *bytes.Buffer
	member *fakeMembers
	delete *fakeDeletions
}

func newEnv(t *testing.T) *env { return newEnvMail(t, true) }

// newEnvMail builds the environment with a mailer, or with none when withMail
// is false, as when no email provider is configured.
func newEnvMail(t *testing.T, withMail bool) *env {
	t.Helper()
	e := &env{
		t: t,
		dir: &fakeDirectory{
			roles:   map[string]staff.Role{adminID: staff.RoleSystemAdmin, suppID: staff.RoleSupport, billID: staff.RoleBilling, tadminID: staff.RoleAdmin},
			tenants: map[string]string{tadminID: tenantID},
			emails:  map[string]string{adminID: adminMail, suppID: suppMail, billID: billMail, custID: custMail, tadminID: tadminMail},
		},
		reader: &fakeReader{
			wsTenant:   map[string]string{wsID: tenantID},
			userTenant: map[string]string{custID: tenantID},
			tenantDetails: map[string]TenantDetail{tenantID: {TenantRow: TenantRow{
				ID: tenantID, Name: "Acme tenant", Status: "active", WorkspaceCount: 1, BestPlanKey: "free",
				StatusSummary: map[string]int64{"active": 1}, CreatedAt: t0, LastActivityAt: t0,
			}, Workspaces: []TenantWorkspace{{ID: wsID, Name: wsName, Slug: "acme", Status: "active", PlanKey: "free", MemberCount: 2, CreatedAt: t0}}}},
			details:  map[string]WorkspaceDetail{wsID: {WorkspaceRow: WorkspaceRow{ID: wsID, Name: wsName, Slug: "acme", Status: "active", CreatedAt: t0, LastActivityAt: t0}}},
			userDets: map[string]UserDetail{custID: {UserRow: UserRow{ID: custID, Email: custMail, Name: "Cu", Status: "active", CreatedAt: t0}}},
		},
		store:  newFakeStore(),
		ws:     &fakeStatusSetter{changed: true},
		cache:  &fakeCache{},
		pub:    &fakeNotifier{},
		mail:   &fakeMailer{},
		logs:   &bytes.Buffer{},
		member: newFakeMembers(),
		delete: newFakeDeletions(),
	}
	e.reader.store = e.store
	payments, err := billing.New(billing.Deps{
		Runner: fakeBillingRunner{e.store}, Provider: billing.ProviderManual,
		Workspaces: noopInvalidator{}, Entitlements: noopEntitlements{},
		Now: func() time.Time { return t0 },
	})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(e.logs, nil))
	auth := platformauth.New(platformauth.Deps{
		Config:    platformauth.Config{SessionSecret: secret, ConsoleOrigin: origin},
		Directory: e.dir,
		Guard:     activeGuard{},
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:       func() time.Time { return t0 },
	})
	var mail Mailer
	if withMail {
		mail = e.mail
	}
	e.h = New(Deps{
		Mailer:        mail,
		RequireStaff:  auth.RequirePlatformStaff,
		RequireOrigin: auth.RequireOrigin,
		Reader:        e.reader,
		Runner:        e.store,
		Workspaces:    e.ws,
		Cache:         e.cache,
		Notifier:      e.pub,
		Payments:      payments,
		Emails:        e.dir,
		Members:       e.member,
		Deletions:     e.delete,
		Log:           log,
		Now:           func() time.Time { return t0 },
	})
	return e
}

// do sends a request as user ("" sends no cookie). Write methods carry the
// console's Origin.
func (e *env) do(user, method, path, body string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.doWith(user, method, path, body, origin)
}

func (e *env) doWith(user, method, path, body, originHeader string) *httptest.ResponseRecorder {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if user != "" {
		tok, err := platformauth.SignMFASession(secret, user, t0.Add(-time.Minute))
		if err != nil {
			e.t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: session.CookieName, Value: tok})
	}
	if method != http.MethodGet && originHeader != "" {
		req.Header.Set("Origin", originHeader)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}
