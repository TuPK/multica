package featureflags

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// The flags of flow authoring are off until an operator turns them on, and they
// read the environment as the others do.
func TestFlowAuthoringDefaultsOff(t *testing.T) {
	ctx := context.Background()
	flags := featureflag.NewService(featureflag.NewStaticProvider())
	if FlowAuthoringEnabled(ctx, flags) || FlowGraphV2Enabled(ctx, flags) {
		t.Fatal("a flow authoring flag is on by default")
	}

	envOn := featureflag.NewService(featureflag.NewEnvProvider("FF_"))
	t.Setenv("FF_FLOW_AUTHORING", "on")
	t.Setenv("FF_FLOW_GRAPH_V2", "off")
	if !FlowAuthoringEnabled(ctx, envOn) {
		t.Error("FF_FLOW_AUTHORING=on did not turn authoring on")
	}
	if FlowGraphV2Enabled(ctx, envOn) {
		t.Error("FF_FLOW_GRAPH_V2=off left graph v2 on")
	}
	t.Setenv("FF_FLOW_GRAPH_V2", "true")
	if !FlowGraphV2Enabled(ctx, envOn) {
		t.Error("FF_FLOW_GRAPH_V2=true did not turn graph v2 on")
	}
	if FlowAuthoring != "flow_authoring" || FlowGraphV2 != "flow_graph_v2" {
		t.Errorf("keys = %q, %q; FF_ names would not match", FlowAuthoring, FlowGraphV2)
	}
}
