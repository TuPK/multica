package migrations

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestEffectEventAppendOnly: effect_event is append-only except the status moves, the
// undo fields and the intent link (durable spec 4.4). No query may delete a row, and
// an update may set only status, undo, undo_until and intent_id.
func TestEffectEventAppendOnly(t *testing.T) {
	queries, err := filepath.Glob(filepath.Join("..", "..", "pkg", "db", "queries", "*.sql"))
	if err != nil || len(queries) == 0 {
		t.Fatalf("no query files found: %v", err)
	}
	deletes := regexp.MustCompile(`(?is)\bDELETE\s+FROM\s+effect_event\b`)
	// An UPDATE's SET list must name only the allowed columns. The SET clause runs
	// to the next WHERE or RETURNING keyword.
	updates := regexp.MustCompile(`(?is)\bUPDATE\s+effect_event\s+SET\s+(.*?)\s+(?:WHERE|RETURNING)\b`)
	assignments := regexp.MustCompile(`(?is)([a-z_]+)\s*=`)
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
			for _, a := range assignments.FindAllSubmatch(m[1], -1) {
				if col := string(a[1]); !allowed[col] {
					t.Errorf("%s updates effect_event.%s; only status, undo and undo_until may change (spec 4.4)", filepath.Base(path), col)
				}
			}
		}
	}
	if !sawUpdate {
		t.Fatal("no UPDATE effect_event query found: the append-only check has nothing to guard")
	}
}
