package console

import (
	"context"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/usage"
)

// The flows of spec R6 section 3b.4 that have no limit: task runs and issues
// created per day. Each states its newest day's count and a seven-day trend.
const (
	gateTasksRun      = "tasks_run"
	gateIssuesCreated = "issues_created"
)

// The seat types of spec R6 section 3b.4: the members of a workspace and its
// held seat operations, both against the seats it bought.
const (
	gateMembers   = "members"
	gateSeatsHeld = "seats_held"
)

// resourceGates are the resource types the console states, in order: the
// counted limits (usage.Current, the group count, the seats, the autopilot period
// and the API rate of the minute), then the flows.
var resourceGates = []string{
	plan.GateAgents, plan.GateRuntimes, plan.GateIssueCount, plan.GateStorageBytes, plan.GateMemberGroups,
	gateMembers, gateSeatsHeld, plan.GateAutopilotRuns, plan.GateAPIRatePerMinute, gateTasksRun, gateIssuesCreated,
}

// trendDays is the window of a flow's trend: the UTC days that end today.
const trendDays = 7

// warningPercent is where a resource turns to warning (spec 3b.4 rule 1).
const warningPercent = 80

// resourceJSON is one resource type. Limit is null when there is no limit, and
// then Percent is null and Status is "ok".
type resourceJSON struct {
	Type    string `json:"type"`
	Used    int64  `json:"used"`
	Limit   *int64 `json:"limit"`
	Percent *int64 `json:"percent"`
	Status  string `json:"status"`
	// Trend is the seven days of a flow, oldest first; other types have none.
	Trend []trendPointJSON `json:"trend,omitempty"`
}

// trendPointJSON is one day of a flow's trend.
type trendPointJSON struct {
	Day  string `json:"day"`
	Used int64  `json:"used"`
}

// trendSince is the first day of a flow's trend: the seven UTC days ending today.
func trendSince(now time.Time) time.Time {
	d := now.UTC()
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(trendDays - 1))
}

// flowValue is the count of one flow on the day of row f.
func flowValue(f usage.DayFlow, gate string) int64 {
	if gate == gateTasksRun {
		return f.TasksRun
	}
	return f.IssuesCreated
}

// flowUsed is the newest day's count of a flow, as the overview states it. The
// rows are the window's, oldest first; no row in the window is zero.
func flowUsed(flows []usage.DayFlow, gate string) int64 {
	if len(flows) == 0 {
		return 0
	}
	return flowValue(flows[len(flows)-1], gate)
}

// flowTrend gives the seven days of a flow from since, zero on a day with no row.
func flowTrend(flows []usage.DayFlow, gate string, now time.Time) []trendPointJSON {
	byDay := make(map[string]int64, len(flows))
	for _, f := range flows {
		byDay[f.Day.UTC().Format(time.DateOnly)] = flowValue(f, gate)
	}
	since := trendSince(now)
	out := make([]trendPointJSON, trendDays)
	for i := range out {
		d := since.AddDate(0, 0, i).Format(time.DateOnly)
		out[i] = trendPointJSON{Day: d, Used: byDay[d]}
	}
	return out
}

// newResource states a used count against its limit. Over is more than the
// limit; warning is from warningPercent of it. A zero limit is full at any use.
func newResource(gate string, used int64, limit *int64) resourceJSON {
	out := resourceJSON{Type: gate, Used: used, Limit: limit, Status: "ok"}
	if limit == nil {
		return out
	}
	pct := int64(100)
	if *limit > 0 {
		pct = used * 100 / *limit
	} else if used == 0 {
		pct = 0
	}
	out.Percent = &pct
	switch {
	case used > *limit:
		out.Status = "over"
	case pct >= warningPercent:
		out.Status = "warning"
	}
	return out
}

// workspaceUsed is the usage of one workspace: the used count of each resource
// gate, its effective limits, and the trend of each flow.
type workspaceUsed struct {
	used   map[string]int64
	limits map[string]*int64
	trend  map[string][]trendPointJSON
}

// workspaceUsage reads the usage of a workspace. A workspace the platform does
// not know counts zero for the counts it holds.
func (h *Handler) workspaceUsage(ctx context.Context, workspaceID string, now time.Time) (workspaceUsed, error) {
	out := workspaceUsed{used: map[string]int64{}, trend: map[string][]trendPointJSON{}}
	d, found, err := h.Reader.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return workspaceUsed{}, err
	}
	if found {
		out.used[plan.GateAgents] = d.Current.Agents
		out.used[plan.GateRuntimes] = d.Current.Runtimes
		out.used[plan.GateIssueCount] = d.Current.IssuesTotal
		out.used[plan.GateStorageBytes] = d.Current.StorageBytes
		out.used[plan.GateMemberGroups] = d.GroupCount
		out.used[gateMembers] = d.Current.Members
	}
	if out.used[gateSeatsHeld], err = h.Reader.HeldSeats(ctx, workspaceID, now); err != nil {
		return workspaceUsed{}, err
	}
	if out.used[plan.GateAutopilotRuns], err = h.Reader.AutopilotRuns(ctx, workspaceID, now); err != nil {
		return workspaceUsed{}, err
	}
	if out.used[plan.GateAPIRatePerMinute], err = h.RateCounter.RateCurrent(ctx, workspaceID, now); err != nil {
		return workspaceUsed{}, err
	}
	flows, err := h.Reader.Flows(ctx, workspaceID, trendSince(now))
	if err != nil {
		return workspaceUsed{}, err
	}
	for _, g := range []string{gateTasksRun, gateIssuesCreated} {
		out.used[g] = flowUsed(flows, g)
		out.trend[g] = flowTrend(flows, g, now)
	}
	out.limits, err = h.Reader.WorkspaceLimits(ctx, workspaceID, now)
	if err != nil {
		return workspaceUsed{}, err
	}
	seats, err := h.Reader.SeatLimit(ctx, workspaceID)
	if err != nil {
		return workspaceUsed{}, err
	}
	if out.limits == nil {
		out.limits = map[string]*int64{}
	}
	out.limits[gateMembers], out.limits[gateSeatsHeld] = seats, seats
	return out, nil
}

type workspaceResourcesJSON struct {
	WorkspaceID string         `json:"workspace_id"`
	Slug        string         `json:"slug"`
	PlanKey     string         `json:"plan_key"`
	Resources   []resourceJSON `json:"resources"`
}

// tenantResources is a tenant's resources: the totals of its workspaces, the
// summed trend of each flow, the tenant's own workspace count against
// max_workspaces, and each workspace.
type tenantResources struct {
	totals     map[string]int64
	trends     map[string][]trendPointJSON
	workspaces []workspaceResourcesJSON
}

// tenantUsage reads every workspace of a tenant. Totals have no limit: the
// limits are per workspace (spec 3b.4 rule 5).
func (h *Handler) tenantUsage(ctx context.Context, workspaces []TenantWorkspace, now time.Time) (tenantResources, error) {
	out := tenantResources{totals: map[string]int64{}, trends: map[string][]trendPointJSON{}, workspaces: make([]workspaceResourcesJSON, 0, len(workspaces))}
	for _, ws := range workspaces {
		u, err := h.workspaceUsage(ctx, ws.ID, now)
		if err != nil {
			return tenantResources{}, err
		}
		entry := workspaceResourcesJSON{WorkspaceID: ws.ID, Slug: ws.Slug, PlanKey: ws.PlanKey, Resources: make([]resourceJSON, 0, len(resourceGates))}
		for _, g := range resourceGates {
			res := newResource(g, u.used[g], u.limits[g])
			res.Trend = u.trend[g]
			entry.Resources = append(entry.Resources, res)
			out.totals[g] += u.used[g]
			if pts := u.trend[g]; pts != nil {
				out.trends[g] = addTrend(out.trends[g], pts)
			}
		}
		out.workspaces = append(out.workspaces, entry)
	}
	return out, nil
}

// addTrend adds the days of pts to the trend acc, day by day. acc is a copy, so
// the workspace's own trend is not changed.
func addTrend(acc, pts []trendPointJSON) []trendPointJSON {
	if acc == nil {
		return slices.Clone(pts)
	}
	for i := range acc {
		acc[i].Used += pts[i].Used
	}
	return acc
}

// totalResources lists the tenant totals in gate order, then max_workspaces,
// the only limit that belongs to the tenant (spec 3b.4 rule 5).
func totalResources(t tenantResources, workspaceCount int64, maxWorkspaces *int64) []resourceJSON {
	out := make([]resourceJSON, 0, len(resourceGates)+1)
	for _, g := range resourceGates {
		res := newResource(g, t.totals[g], nil)
		res.Trend = t.trends[g]
		out = append(out, res)
	}
	return append(out, newResource(plan.GateMaxWorkspaces, workspaceCount, maxWorkspaces))
}

type tenantResourcesJSON struct {
	TenantID   string                   `json:"tenant_id"`
	Resources  []resourceJSON           `json:"resources"`
	Workspaces []workspaceResourcesJSON `json:"workspaces"`
}

// getTenantResources answers M38: the tenant's resources against its limits. An
// admin sees its own tenant; a tenant outside the scope answers 404.
func (h *Handler) getTenantResources(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionTenantResourcesRead)
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
	usage, err := h.tenantUsage(r.Context(), d.Workspaces, h.Now())
	if err != nil {
		h.fail(w, r, "read tenant resources", err)
		return
	}
	writeJSON(w, http.StatusOK, tenantResourcesJSON{
		TenantID:   id,
		Resources:  totalResources(usage, int64(len(d.Workspaces)), d.MaxWorkspaces),
		Workspaces: usage.workspaces,
	})
}

// overLimitJSON is one workspace resource that is warning or over.
type overLimitJSON struct {
	TenantID    string `json:"tenant_id"`
	WorkspaceID string `json:"workspace_id"`
	Type        string `json:"type"`
	Used        int64  `json:"used"`
	Limit       *int64 `json:"limit"`
	Status      string `json:"status"`
}

// tenantUsageSummary is one tenant of the system page, kept for the top lists.
type tenantUsageSummary struct {
	id    string
	usage tenantResources
}

// The defaults and bounds of the system page (spec 3b.4 rule 2).
const (
	defaultTop = 10
	maxTop     = 50
)

// getSystemResources answers M39 for system_admin: the resources of one page of
// tenants, their totals by type and by plan, the top tenants of each type and
// the workspaces that are warning or over. complete is true on the last page.
func (h *Handler) getSystemResources(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionSystemResourcesRead)
	if !ok {
		return
	}
	q := r.URL.Query()
	p, perr := parseListParams(q)
	if perr != nil {
		writeError(w, perr)
		return
	}
	top := defaultTop
	if raw := q.Get("top"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, &apiError{Status: http.StatusBadRequest, Code: "top_invalid", Message: "top must be a positive integer"})
			return
		}
		top = min(n, maxTop)
	}
	rows, err := h.Reader.SearchTenants(r.Context(), TenantSearch{Cursor: p.cursor, Limit: p.limit, Scope: scope})
	if err != nil {
		h.fail(w, r, "search tenants", err)
		return
	}
	rows, next := nextCursor(rows, p.limit, func(t TenantRow) (time.Time, string) { return t.CreatedAt, t.ID })

	now := h.Now()
	totals := map[string]int64{}
	byPlan := map[string]map[string]int64{}
	var summaries []tenantUsageSummary
	var over []overLimitJSON
	for _, row := range rows {
		d, found, err := h.Reader.GetTenant(r.Context(), row.ID, scope)
		if err != nil {
			h.fail(w, r, "get tenant", err)
			return
		}
		if !found {
			continue
		}
		usage, err := h.tenantUsage(r.Context(), d.Workspaces, now)
		if err != nil {
			h.fail(w, r, "read tenant resources", err)
			return
		}
		summaries = append(summaries, tenantUsageSummary{id: row.ID, usage: usage})
		for g, v := range usage.totals {
			totals[g] += v
		}
		for _, ws := range usage.workspaces {
			if byPlan[ws.PlanKey] == nil {
				byPlan[ws.PlanKey] = map[string]int64{}
			}
			for _, res := range ws.Resources {
				byPlan[ws.PlanKey][res.Type] += res.Used
				if res.Status == "warning" || res.Status == "over" {
					over = append(over, overLimitJSON{TenantID: row.ID, WorkspaceID: ws.WorkspaceID, Type: res.Type, Used: res.Used, Limit: res.Limit, Status: res.Status})
				}
			}
		}
	}

	topLists := map[string][]topTenantJSON{}
	for _, g := range resourceGates {
		sort.SliceStable(summaries, func(i, j int) bool {
			if summaries[i].usage.totals[g] != summaries[j].usage.totals[g] {
				return summaries[i].usage.totals[g] > summaries[j].usage.totals[g]
			}
			return summaries[i].id < summaries[j].id
		})
		list := []topTenantJSON{}
		for i := 0; i < len(summaries) && i < top; i++ {
			list = append(list, topTenantJSON{TenantID: summaries[i].id, Used: summaries[i].usage.totals[g]})
		}
		topLists[g] = list
	}

	totalsOut := make([]resourceJSON, 0, len(resourceGates))
	for _, g := range resourceGates {
		totalsOut = append(totalsOut, newResource(g, totals[g], nil))
	}
	byPlanOut := map[string][]resourceJSON{}
	for key, m := range byPlan {
		list := make([]resourceJSON, 0, len(resourceGates))
		for _, g := range resourceGates {
			list = append(list, newResource(g, m[g], nil))
		}
		byPlanOut[key] = list
	}
	if over == nil {
		over = []overLimitJSON{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenants":     len(summaries),
		"totals":      totalsOut,
		"by_plan":     byPlanOut,
		"top":         topLists,
		"over_limit":  over,
		"next_cursor": next,
		"complete":    next == nil,
	})
}

type topTenantJSON struct {
	TenantID string `json:"tenant_id"`
	Used     int64  `json:"used"`
}
