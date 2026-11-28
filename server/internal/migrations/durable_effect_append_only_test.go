package migrations

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestEffectEventAppendOnly: effect_event is append-only except the bounded status moves
// (awaiting -> executed|blocked|failed and executed -> undoing -> executed|undone), the
// undo/result fields set during awaiting settlement, and the initial intent_id link while
// awaiting. No query may delete a row, and every UPDATE must enforce an exact allowed
// state transition in its WHERE clause.
//
// Spec deviation (spec 4.4): spec 4.4 lists only awaiting -> executed|blocked|failed and
// executed -> undone. The intermediate "undoing" claim state (executed -> undoing ->
// executed|undone) is kept deliberately so concurrent undo requests invoke the external
// provider at most once; spec 4.4 needs to be updated to include this claim transition.
func TestEffectEventAppendOnly(t *testing.T) {
	queries, err := filepath.Glob(filepath.Join("..", "..", "pkg", "db", "queries", "*.sql"))
	if err != nil || len(queries) == 0 {
		t.Fatalf("no query files found: %v", err)
	}
	deletes := regexp.MustCompile(`(?is)\bDELETE\s+FROM\s+effect_event\b`)
	// Match full UPDATE effect_event statements up to the terminating semicolon.
	updates := regexp.MustCompile(`(?is)\bUPDATE\s+effect_event\s+SET\s+(.*?)\s+WHERE\s+(.*?);`)
	assignments := regexp.MustCompile(`(?is)([a-z_]+)\s*=`)
	statusGuard := regexp.MustCompile(`(?is)\bstatus\s*=\s*'(awaiting|executed|undoing)'`)
	statusLit := regexp.MustCompile(`(?is)\bstatus\s*=\s*'([a-z_]+)'`)
	awaitingTargetGuard := regexp.MustCompile(`(?is)IN\s*\(\s*'executed'\s*,\s*'blocked'\s*,\s*'failed'\s*\)`)
	intentNullGuard := regexp.MustCompile(`(?is)\bintent_id\s+IS\s+NULL\b`)
	allowed := map[string]bool{"status": true, "undo": true, "undo_until": true, "intent_id": true, "result": true}
	sawUpdate := false
	for _, path := range queries {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		src := sqlComment.ReplaceAll(body, nil)
		if m := deletes.Find(src); m != nil {
			t.Errorf("%s deletes from effect_event (%q); the log is append-only (spec 4.4)", filepath.Base(path), m)
		}
		for _, m := range updates.FindAllSubmatch(src, -1) {
			sawUpdate = true
			setClause, whereClause := m[1], m[2]
			cols := map[string]bool{}
			for _, a := range assignments.FindAllSubmatch(setClause, -1) {
				col := string(a[1])
				cols[col] = true
				if !allowed[col] {
					t.Errorf("%s updates effect_event.%s; only status, undo, undo_until, intent_id and result may change (spec 4.4)", filepath.Base(path), col)
				}
			}
			guard := statusGuard.FindSubmatch(whereClause)
			if guard == nil {
				t.Errorf("%s updates effect_event without a status guard (awaiting|executed|undoing) in WHERE clause: %s", filepath.Base(path), string(whereClause))
				continue
			}
			fromState := string(guard[1])
			switch fromState {
			case "awaiting":
				if cols["intent_id"] && !intentNullGuard.Match(whereClause) {
					t.Errorf("%s updates effect_event.intent_id without intent_id IS NULL guard", filepath.Base(path))
				}
				if cols["status"] {
					if lit := statusLit.FindSubmatch(setClause); lit != nil {
						if dst := string(lit[1]); dst != "executed" && dst != "blocked" && dst != "failed" {
							t.Errorf("%s moves awaiting effect_event to disallowed status %q", filepath.Base(path), dst)
						}
					} else if !awaitingTargetGuard.Match(whereClause) {
						t.Errorf("%s sets parameterized status from awaiting without IN ('executed', 'blocked', 'failed') guard", filepath.Base(path))
					}
				}
			case "executed":
				if len(cols) != 1 || !cols["status"] {
					t.Errorf("%s updates non-status columns on executed effect_event: %v", filepath.Base(path), cols)
				}
				if lit := statusLit.FindSubmatch(setClause); lit == nil || string(lit[1]) != "undoing" {
					t.Errorf("%s moves executed effect_event to status other than undoing: %s", filepath.Base(path), string(setClause))
				}
			case "undoing":
				if len(cols) != 1 || !cols["status"] {
					t.Errorf("%s updates non-status columns on undoing effect_event: %v", filepath.Base(path), cols)
				}
				if lit := statusLit.FindSubmatch(setClause); lit == nil || (string(lit[1]) != "executed" && string(lit[1]) != "undone") {
					t.Errorf("%s moves undoing effect_event to status other than executed|undone: %s", filepath.Base(path), string(setClause))
				}
			}
		}
	}
	if !sawUpdate {
		t.Fatal("no UPDATE effect_event query found: the append-only check has nothing to guard")
	}
}
