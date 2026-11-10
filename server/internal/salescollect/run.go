package salescollect

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// CoverageBar is the share of items that must carry all four fields to pass a sample (spec 4.11).
const CoverageBar = 0.8

// SampleBar is the fewest items a sample needs before it can pass.
const SampleBar = 5

// Result is what one collector run reports. Status is "ok" when the sample bar is met,
// "degraded" when the source answered but the items are missing or thin, and "error" when it
// did not answer or the recipe cannot run here.
type Result struct {
	SourceID      string   `json:"source_id,omitempty"`
	Method        string   `json:"method"`
	Rung          int      `json:"rung"`
	Status        string   `json:"status"`
	HTTPStatus    int      `json:"http_status"`
	ItemsSeen     int      `json:"items_seen"`
	FieldCoverage float64  `json:"field_coverage"`
	Signals       []Signal `json:"signals"`
	Warnings      []string `json:"warnings,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// Run executes a recipe: it reads the source with its adapter, filters and normalises the items,
// removes duplicates by entity key and URL, and reports coverage. A refusal (a method that runs
// elsewhere, a Web engine the build lacks) is a Result with status "error", not a Go error.
func Run(ctx context.Context, r *Recipe, opts Options) *Result {
	res := &Result{SourceID: opts.SourceID, Method: r.Method, Rung: r.Rung, Status: "error"}
	if err := r.Collectable(); err != nil {
		res.Error = err.Error()
		return res
	}
	run := adapterFor(r.Method)
	if run == nil {
		res.Error = fmt.Sprintf("no collector for method %s", r.Method)
		return res
	}
	got, err := run(ctx, newFetcher(opts), r, opts)
	res.HTTPStatus = got.status
	res.Warnings = got.warnings
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.ItemsSeen = len(got.records)

	complete := 0
	seen := map[string]bool{}
	limit := r.MaxItemsOrDefault()
	if opts.Limit > 0 && opts.Limit < limit {
		limit = opts.Limit
	}
	res.Signals = []Signal{}
	for _, rec := range got.records {
		if rec.fieldsComplete() {
			complete++
		}
		if !keep(rec, r.Filter, now()) {
			continue
		}
		sig := toSignal(rec, r, opts.SourceID, opts)
		dedupe := sig.EntityKey + "|" + sig.URL
		if seen[dedupe] {
			continue
		}
		seen[dedupe] = true
		if len(res.Signals) >= limit {
			continue
		}
		res.Signals = append(res.Signals, sig)
	}
	if res.ItemsSeen > 0 {
		res.FieldCoverage = math.Round(float64(complete)/float64(res.ItemsSeen)*100) / 100
	}
	if res.ItemsSeen >= SampleBar && res.FieldCoverage >= CoverageBar {
		res.Status = "ok"
	} else {
		res.Status = "degraded"
	}
	return res
}

// keep applies the recipe's filter to one item: a keyword list that must match, a list that must
// not, and a window in days. An item with no date is kept, since the window cannot judge it.
func keep(rec record, f Filter, at time.Time) bool {
	text := strings.ToLower(rec.Entity + " " + rec.Text)
	if len(f.KeywordsAny) > 0 {
		hit := false
		for _, k := range f.KeywordsAny {
			if strings.Contains(text, strings.ToLower(k)) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	for _, k := range f.KeywordsNone {
		if k != "" && strings.Contains(text, strings.ToLower(k)) {
			return false
		}
	}
	if f.SinceDays > 0 {
		if d := normalizeDate(rec.ObservedAt); d != "" {
			when, err := time.Parse("2006-01-02", d)
			if err == nil && when.Before(at.AddDate(0, 0, -f.SinceDays)) {
				return false
			}
		}
	}
	return true
}
