package console

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// The resource types of spec R6 section 3b.4 that have a plan limit and are
// counted now (usage.Current and the group count). The API rate lives in Redis
// and has no history, and the seven-day trend needs platform_usage_daily; both
// are outside this slice.
var resourceGates = []string{plan.GateAgents, plan.GateRuntimes, plan.GateIssueCount, plan.GateStorageBytes, plan.GateMemberGroups}

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

// workspaceUsage returns the used count of each resource gate of a workspace
// and its effective limits. A workspace the platform does not know counts zero.
func (h *Handler) workspaceUsage(ctx context.Context, workspaceID string, now time.Time) (map[string]int64, map[string]*int64, error) {
	used := map[string]int64{}
	d, found, err := h.Reader.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	if found {
		used[plan.GateAgents] = d.Current.Agents
		used[plan.GateRuntimes] = d.Current.Runtimes
		used[plan.GateIssueCount] = d.Current.IssuesTotal
		used[plan.GateStorageBytes] = d.Current.StorageBytes
		used[plan.GateMemberGroups] = d.GroupCount
	}
	limits, err := h.Reader.WorkspaceLimits(ctx, workspaceID, now)
	if err != nil {
		return nil, nil, err
	}
	return used, limits, nil
}

type workspaceResourcesJSON struct {
	WorkspaceID string         `json:"workspace_id"`
	Slug        string         `json:"slug"`
	PlanKey     string         `json:"plan_key"`
	Resources   []resourceJSON `json:"resources"`
}

// tenantResources is a tenant's resources: the totals of its workspaces, the
// tenant's own workspace count against max_workspaces, and each workspace.
type tenantResources struct {
	totals     map[string]int64
	workspaces []workspaceResourcesJSON
}

// tenantUsage reads every workspace of a tenant. Totals have no limit: the
// limits are per workspace (spec 3b.4 rule 5).
func (h *Handler) tenantUsage(ctx context.Context, workspaces []TenantWorkspace, now time.Time) (tenantResources, error) {
	out := tenantResources{totals: map[string]int64{}, workspaces: make([]workspaceResourcesJSON, 0, len(workspaces))}
	for _, ws := range workspaces {
		used, limits, err := h.workspaceUsage(ctx, ws.ID, now)
		if err != nil {
			return tenantResources{}, err
		}
		entry := workspaceResourcesJSON{WorkspaceID: ws.ID, Slug: ws.Slug, PlanKey: ws.PlanKey, Resources: make([]resourceJSON, 0, len(resourceGates))}
		for _, g := range resourceGates {
			entry.Resources = append(entry.Resources, newResource(g, used[g], limits[g]))
			out.totals[g] += used[g]
		}
		out.workspaces = append(out.workspaces, entry)
	}
	return out, nil
}

// totalResources lists the tenant totals in gate order, then max_workspaces,
// the only limit that belongs to the tenant (spec 3b.4 rule 5).
func totalResources(t tenantResources, workspaceCount int64, maxWorkspaces *int64) []resourceJSON {
	out := make([]resourceJSON, 0, len(resourceGates)+1)
	for _, g := range resourceGates {
		out = append(out, newResource(g, t.totals[g], nil))
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
