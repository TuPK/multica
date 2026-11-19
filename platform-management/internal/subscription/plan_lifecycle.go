package subscription

import (
	"net/http"
	"sort"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// Plan lifecycle routes of spec R6 section 3b.1: one plan, activate and
// deactivate, and the history read from the audit table.

func (c *Console) getPlan(w http.ResponseWriter, r *http.Request) {
	key, ok := planKey(w, r, func() *Error {
		return badRequest("invalid_plan", "key must be 1 to 32 lower-case letters, digits or underscores, starting with a letter")
	})
	if !ok {
		return
	}
	p, found, err := c.Plans.Get(r.Context(), key)
	if err != nil {
		c.fail(w, r, "get plan", err)
		return
	}
	if !found {
		writeFromError(w, &Error{Status: http.StatusNotFound, Code: "plan_not_found", Message: "plan not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": p})
}

func (c *Console) activatePlan(w http.ResponseWriter, r *http.Request) {
	c.setPlanArchived(w, r, false)
}

func (c *Console) deactivatePlan(w http.ResponseWriter, r *http.Request) {
	c.setPlanArchived(w, r, true)
}

// setPlanArchived sets or clears archived_at. A plan already in that state is a
// no-op: nothing is written and no audit row is added (spec R6 section 3b.1, rule 5).
func (c *Console) setPlanArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	key, ok := planKey(w, r, func() *Error {
		return badRequest("invalid_plan", "key must be 1 to 32 lower-case letters, digits or underscores, starting with a letter")
	})
	if !ok {
		return
	}
	a, err := c.currentActor(r)
	if err != nil {
		c.fail(w, r, "resolve staff actor", err)
		return
	}
	var stored plan.Plan
	changed := false
	err = c.PlanRunner.InTx(r.Context(), func(tx plan.Tx) error {
		changed = false
		old, exists, err := tx.Get(r.Context(), key)
		if err != nil {
			return err
		}
		if !exists {
			return &Error{Status: http.StatusNotFound, Code: "plan_not_found", Message: "plan not found"}
		}
		next, moved := plan.WithArchived(old, archived, c.Now().UTC().Truncate(time.Microsecond))
		stored = next
		if !moved {
			return nil
		}
		diff, err := planChanges(&old, next)
		if err != nil {
			return err
		}
		if err := tx.Upsert(r.Context(), next); err != nil {
			return err
		}
		changed = true
		action := audit.ActionPlanActivated
		if archived {
			action = audit.ActionPlanDeactivated
		}
		return audit.Record(r.Context(), tx, a.entry(audit.Entry{
			Action: action, TargetType: targetPlan, TargetID: key, TargetLabel: next.Name,
			Detail: map[string]any{"changes": diff},
		}))
	})
	if err != nil {
		c.fail(w, r, "set plan archived", err)
		return
	}
	if changed {
		c.Cache.InvalidateAll()
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed, "plan": stored})
}

// historyEntry is one row of GET /plans/{key}/history.
type historyEntry struct {
	At      time.Time       `json:"at"`
	Actor   string          `json:"actor"`
	Action  string          `json:"action"`
	Changes []historyChange `json:"changes"`
	Reason  string          `json:"reason,omitempty"`
}

// historyChange is one field that a plan audit row changed.
type historyChange struct {
	Field string `json:"field"`
	Old   any    `json:"old"`
	New   any    `json:"new"`
}

// historyChanges turns the audit detail's changes object, field to {from, to},
// into a list sorted by field. Rows without changes give an empty list.
func historyChanges(detail map[string]any) []historyChange {
	raw, _ := detail["changes"].(map[string]any)
	out := make([]historyChange, 0, len(raw))
	for field, v := range raw {
		m, _ := v.(map[string]any)
		out = append(out, historyChange{Field: field, Old: m["from"], New: m["to"]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// planHistoryEntries maps the audit rows of a plan to the response.
func planHistoryEntries(rows []plan.HistoryRow) []historyEntry {
	out := make([]historyEntry, 0, len(rows))
	for _, row := range rows {
		reason, _ := row.Detail["reason"].(string)
		out = append(out, historyEntry{
			At: row.At.UTC(), Actor: row.ActorLabel, Action: row.Action,
			Changes: historyChanges(row.Detail), Reason: reason,
		})
	}
	return out
}

func (c *Console) planHistory(w http.ResponseWriter, r *http.Request) {
	key, ok := planKey(w, r, func() *Error {
		return badRequest("invalid_plan", "key must be 1 to 32 lower-case letters, digits or underscores, starting with a letter")
	})
	if !ok {
		return
	}
	_, found, err := c.Plans.Get(r.Context(), key)
	if err != nil {
		c.fail(w, r, "get plan", err)
		return
	}
	if !found {
		writeFromError(w, &Error{Status: http.StatusNotFound, Code: "plan_not_found", Message: "plan not found"})
		return
	}
	rows, err := c.PlanHistory.History(r.Context(), key)
	if err != nil {
		c.fail(w, r, "plan history", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": planHistoryEntries(rows)})
}
