package accountdeletion

import (
	"sort"
	"testing"
)

func TestRegistryEntriesAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Columns {
		key := c.Table + "." + c.Column
		if c.Table == "" || c.Column == "" {
			t.Errorf("entry %+v has an empty table or column", c)
		}
		if seen[key] {
			t.Errorf("%s is listed twice", key)
		}
		seen[key] = true
		switch c.Action {
		case ActionDelete:
		case ActionAnonymize, ActionKeep, ActionNotAUser:
			if c.Reason == "" {
				t.Errorf("%s: action %q needs a reason", key, c.Action)
			}
		default:
			t.Errorf("%s: unknown action %q", key, c.Action)
		}
	}
}

func TestRegistryIsSorted(t *testing.T) {
	keys := make([]string, len(Columns))
	for i, c := range Columns {
		keys[i] = c.Table + "." + c.Column
	}
	if !sort.StringsAreSorted(keys) {
		t.Error("registry entries must stay sorted by table, then column")
	}
}

func TestDeletedColumnsList(t *testing.T) {
	got := DeletedColumns()
	if len(got) == 0 {
		t.Fatal("no delete entries")
	}
	for _, c := range got {
		if c.Action != ActionDelete {
			t.Errorf("%s.%s is %q", c.Table, c.Column, c.Action)
		}
	}
}
