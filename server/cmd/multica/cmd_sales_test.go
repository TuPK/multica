package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/salescollect"
)

// resetSalesFlags puts the collect flags back to their defaults after a test sets them.
func resetSalesFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		for _, name := range []string{"recipe-file", "csv", "source-id", "region", "sector", "limit", "dry-run"} {
			f := salesCollectCmd.Flags().Lookup(name)
			_ = salesCollectCmd.Flags().Set(name, f.DefValue)
		}
	})
}

func TestSalesCollectReadsACSVRecipeOfflineAndPrintsSignals(t *testing.T) {
	resetSalesFlags(t)
	dir := t.TempDir()
	recipe := filepath.Join(dir, "recipe.json")
	csvPath := filepath.Join(dir, "list.csv")
	must(t, os.WriteFile(recipe, []byte(`{"version":1,"rung":7,"method":"csv_drop","fetch":{"engine":"http","url":"","render":"none"},"parse":{"items":"","fields":{"entity":"name","url":"website","observed_at":"date","text":"note","website":"website"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"own_data","entity_from":"field"},"max_items":10}`), 0o600))
	must(t, os.WriteFile(csvPath, []byte("name,website,date,note\nPhở Hà,pho-ha.example,2026-11-05,customer\nMáy Đông,may-dong.example,2026-11-06,lead\n"), 0o600))

	must(t, salesCollectCmd.Flags().Set("recipe-file", recipe))
	must(t, salesCollectCmd.Flags().Set("csv", csvPath))
	must(t, salesCollectCmd.Flags().Set("source-id", "src-1"))
	var out bytes.Buffer
	salesCollectCmd.SetOut(&out)
	t.Cleanup(func() { salesCollectCmd.SetOut(nil) })

	err := runSalesCollect(salesCollectCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "could not be read") {
		// Two complete rows is below the sample bar: degraded, not an error, so no error here.
		if err != nil {
			t.Fatalf("collect returned %v", err)
		}
	}
	var res salescollect.Result
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if res.ItemsSeen != 2 || res.SourceID != "src-1" || res.Signals[0].SourceID != "src-1" {
		t.Fatalf("result = %+v", res)
	}
	if res.Status != "degraded" {
		t.Fatalf("two items do not pass the five-item sample: status %s", res.Status)
	}
}

func TestSalesCollectRefusesAProviderRecipeWithItsReason(t *testing.T) {
	resetSalesFlags(t)
	dir := t.TempDir()
	recipe := filepath.Join(dir, "provider.json")
	must(t, os.WriteFile(recipe, []byte(`{"version":1,"rung":6,"method":"provider","provider":{"toolkit":"APIFY","actor":"x","input":{},"cost_cap_usd":0},"fetch":{"engine":"http","url":"https://www.facebook.com/x","render":"none"},"parse":{"items":"","fields":{"entity":"a","url":"b","observed_at":"c","text":"d"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":null,"entity_from":"field"},"max_items":10}`), 0o600))
	must(t, salesCollectCmd.Flags().Set("recipe-file", recipe))
	var out bytes.Buffer
	salesCollectCmd.SetOut(&out)
	t.Cleanup(func() { salesCollectCmd.SetOut(nil) })

	err := runSalesCollect(salesCollectCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "agent task") {
		t.Fatalf("a provider recipe must fail with its reason here, got %v", err)
	}
}

func TestSalesProbeOfAnInternalAddressIsBlockedAndOffline(t *testing.T) {
	var out bytes.Buffer
	salesProbeCmd.SetOut(&out)
	t.Cleanup(func() { salesProbeCmd.SetOut(nil) })
	if err := runSalesProbe(salesProbeCmd, []string{"http://127.0.0.1:9/feed"}); err != nil {
		t.Fatal(err)
	}
	var rep salescollect.ProbeReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if rep.Verdict != "blocked" || !strings.Contains(rep.Reason, "refusing to connect") {
		t.Fatalf("an internal address was probed: %+v", rep)
	}
}

func TestSalesCollectNeedsARecipeFile(t *testing.T) {
	resetSalesFlags(t)
	if err := runSalesCollect(salesCollectCmd, nil); err == nil || !strings.Contains(err.Error(), "--recipe-file is required") {
		t.Fatalf("collect without a recipe = %v", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
