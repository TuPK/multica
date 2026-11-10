package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func fieldCommand(values ...string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().StringArray("field", nil, "")
	for _, v := range values {
		_ = cmd.Flags().Set("field", v)
	}
	return cmd
}

func TestFieldFlagsReadKeyValuePairs(t *testing.T) {
	props, err := fieldFlags(fieldCommand("stage=qualified", "fit_score=72", "note=a=b"))
	if err != nil {
		t.Fatal(err)
	}
	if props["stage"] != "qualified" || props["fit_score"] != "72" || props["note"] != "a=b" {
		t.Fatalf("props = %+v", props)
	}
}

func TestFieldFlagsRefuseAPairWithoutAKey(t *testing.T) {
	for _, bad := range []string{"stage", "=x"} {
		if _, err := fieldFlags(fieldCommand(bad)); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestPropertyLineSortsKeys(t *testing.T) {
	got := propertyLine(map[string]any{"stage": "watch", "fit_score": 72.0, "owner": "maria"})
	if got != "fit_score=72 owner=maria stage=watch" {
		t.Fatalf("propertyLine = %q", got)
	}
}
