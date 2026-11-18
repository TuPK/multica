package einvoice

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEinvoiceFixturesParse(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, f := range files {
		var v map[string]any
		if err := json.Unmarshal([]byte(fixture(t, filepath.Base(f))), &v); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}

	var req struct {
		IsDraft bool `json:"is_draft"`
	}
	if err := json.Unmarshal([]byte(fixture(t, "create_request.json")), &req); err != nil || !req.IsDraft {
		t.Errorf("create_request.json is_draft = %v, %v; want true", req.IsDraft, err)
	}

	var pending struct {
		Data struct {
			Retry *struct {
				MaxRetries int `json:"max_retries"`
			} `json:"retry"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(fixture(t, "create_check_pending.json")), &pending); err != nil || pending.Data.Retry == nil {
		t.Errorf("create_check_pending.json has no retry block: %v", err)
	}

	var done struct {
		Data struct {
			Invoice struct {
				Status string `json:"status"`
			} `json:"invoice"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(fixture(t, "issue_check_success.json")), &done); err != nil || done.Data.Invoice.Status != "issued" {
		t.Errorf("issue_check_success.json invoice status = %q, %v; want issued", done.Data.Invoice.Status, err)
	}
}
