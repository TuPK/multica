package console

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Audit export for staff (spec R6 section 3c.8, QT18, D105). A range of at most
// 31 days is read a page at a time, newest first, and streamed as NDJSON or CSV.
// Each export writes audit.exported first, so the log records who read what.
// Nothing is changed or deleted: the export reads the same rows as the console.

const (
	exportMaxDays     = 31
	exportDefaultPage = 1000
	exportMaxPage     = 1000
)

// exportFields are the columns of a CSV export, in order (spec rule 2: the rows
// carry what the console read already shows; no new fields).
var exportFields = []string{"id", "created_at", "actor_type", "actor_id", "actor_label", "action", "target_type", "target_id", "target_label", "workspace_id", "tenant_id", "ip", "request_id", "detail"}

// auditRowJSON is one exported row as a JSON object with the exportFields keys.
func auditRowJSON(x AuditRow) map[string]any {
	var detail any = map[string]any{}
	if len(x.Detail) > 0 {
		_ = json.Unmarshal(x.Detail, &detail)
	}
	return map[string]any{
		"id": x.ID, "created_at": ts(x.CreatedAt), "actor_type": x.ActorType, "actor_id": x.ActorID,
		"actor_label": x.ActorLabel, "action": x.Action, "target_type": x.TargetType, "target_id": x.TargetID,
		"target_label": x.TargetLabel, "workspace_id": x.WorkspaceID, "tenant_id": "", "ip": x.IP,
		"request_id": x.RequestID, "detail": detail,
	}
}

// parseExportRange reads from and to (RFC 3339, UTC) and checks the range.
func parseExportRange(q map[string][]string) (time.Time, time.Time, *apiError) {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	from, err1 := time.Parse(time.RFC3339, get("from"))
	to, err2 := time.Parse(time.RFC3339, get("to"))
	if err1 != nil || err2 != nil || !from.Before(to) {
		return time.Time{}, time.Time{}, &apiError{Status: http.StatusBadRequest, Code: "export_range_invalid", Message: "from and to must be RFC 3339 times, with from before to"}
	}
	if to.Sub(from) > exportMaxDays*24*time.Hour {
		return time.Time{}, time.Time{}, &apiError{Status: http.StatusBadRequest, Code: "export_range_too_large", Message: "a range can be at most 31 days"}
	}
	return from.UTC(), to.UTC(), nil
}

// exportAudit streams one page of the audit log as NDJSON or CSV (M51).
func (h *Handler) exportAudit(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionAuditExport)
	if !ok {
		return
	}
	q := r.URL.Query()
	from, to, perr := parseExportRange(q)
	if perr != nil {
		writeError(w, perr)
		return
	}
	format := q.Get("format")
	if format == "" {
		format = "ndjson"
	}
	if format != "ndjson" && format != "csv" {
		writeError(w, badRequest("export_format_invalid", "format must be ndjson or csv"))
		return
	}
	page := exportDefaultPage
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > exportMaxPage {
			writeError(w, badRequest("invalid_limit", "limit must be 1 to 1000"))
			return
		}
		page = n
	}
	// parseListParams reads the cursor; the page size is this export's own.
	lp := url.Values{}
	if c := q.Get("cursor"); c != "" {
		lp.Set("cursor", c)
	}
	cur, perr := parseListParams(lp)
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	query := AuditQuery{Scope: scope, Cursor: cur.cursor, Limit: page + 1, From: &from, To: &to}
	if v := q.Get("workspace_id"); v != "" {
		query.WorkspaceID = v
	}
	if v := q.Get("action"); v != "" {
		query.Action = &v
	}
	if v := q.Get("actor_id"); v != "" {
		query.ActorID = &v
	}
	rows, err := h.Reader.ListAuditEvents(r.Context(), query)
	if err != nil {
		h.fail(w, r, "export audit events", err)
		return
	}
	rows, next := nextCursor(rows, page, func(x AuditRow) (time.Time, string) { return x.CreatedAt, x.ID })
	tenant := scope.TenantID
	err = h.Runner.InTx(r.Context(), func(tx Tx) error {
		return audit.Record(r.Context(), tx, a.staffEntry(audit.Entry{
			TenantID: tenant, Action: audit.ActionAuditExported, TargetType: "audit_event",
			Detail: map[string]any{
				"from": ts(from), "to": ts(to), "format": format, "rows": len(rows),
				"workspace_id": query.WorkspaceID, "action": q.Get("action"), "actor_id": q.Get("actor_id"),
			},
		}))
	})
	if err != nil {
		h.fail(w, r, "record audit export", err)
		return
	}
	if next != nil {
		w.Header().Set("X-Next-Cursor", *next)
	}
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		cw := csv.NewWriter(w)
		_ = cw.Write(exportFields)
		for _, x := range rows {
			_ = cw.Write(csvRow(auditRowJSON(x)))
		}
		cw.Flush()
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	enc := json.NewEncoder(w)
	for _, x := range rows {
		_ = enc.Encode(auditRowJSON(x))
	}
}

// csvRow renders one row in exportFields order; detail is its JSON text.
func csvRow(row map[string]any) []string {
	out := make([]string, len(exportFields))
	for i, f := range exportFields {
		if f == "detail" {
			raw, _ := json.Marshal(row[f])
			out[i] = string(raw)
			continue
		}
		out[i] = strings.TrimSpace(toString(row[f]))
	}
	return out
}

func toString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return ""
	}
}
