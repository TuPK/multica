package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// A publish proposal's fields come from --payload-file, as a goal's do.
func TestProposalKindFromFlagsPublishReadsPayloadFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(file, []byte(`{"item_id":"post-1","version":1,"market":"vn","channels":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	addProposalCreateFlags(cmd)
	if err := cmd.Flags().Set("kind", "publish"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("payload-file", file); err != nil {
		t.Fatal(err)
	}

	kind, payload, err := proposalKindFromFlags(cmd)
	if err != nil {
		t.Fatalf("proposalKindFromFlags: %v", err)
	}
	if kind != "publish" || payload["item_id"] != "post-1" || payload["market"] != "vn" {
		t.Fatalf("kind=%q payload=%v", kind, payload)
	}
}

func TestProposalKindFromFlagsPublishNeedsPayloadFile(t *testing.T) {
	cmd := &cobra.Command{}
	addProposalCreateFlags(cmd)
	if err := cmd.Flags().Set("kind", "publish"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := proposalKindFromFlags(cmd); err == nil {
		t.Fatal("publish without --payload-file must be refused")
	}
}
