package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The issueworkflow text goldens render a flow with no stop template from
// testdata/v1_stop_messages.json, because that package cannot import this one.
// This test keeps the file and defaultStopMessages the same text.
func TestDefaultStopMessagesMatchIssueworkflowTestdata(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "issueworkflow", "testdata", "v1_stop_messages.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fromFile map[string]string
	if err := json.Unmarshal(raw, &fromFile); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromFile, defaultStopMessages) {
		t.Fatalf("testdata/v1_stop_messages.json differs from defaultStopMessages\n file: %q\n code: %q", fromFile, defaultStopMessages)
	}
}
