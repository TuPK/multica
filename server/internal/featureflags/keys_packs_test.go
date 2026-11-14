package featureflags

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// Importing a pack of skills and agents is off until an operator turns it on.
func TestFlowPacksDefaultsOff(t *testing.T) {
	ctx := context.Background()
	if FlowPacksEnabled(ctx, featureflag.NewService(featureflag.NewStaticProvider())) {
		t.Fatal("packs are on by default")
	}
	envOn := featureflag.NewService(featureflag.NewEnvProvider("FF_"))
	t.Setenv("FF_FLOW_PACKS", "on")
	if !FlowPacksEnabled(ctx, envOn) {
		t.Error("FF_FLOW_PACKS=on did not turn packs on")
	}
	t.Setenv("FF_FLOW_PACKS", "off")
	if FlowPacksEnabled(ctx, envOn) {
		t.Error("FF_FLOW_PACKS=off left packs on")
	}
	if FlowPacks != "flow_packs" {
		t.Errorf("key = %q; the FF_ name would not match", FlowPacks)
	}
}
