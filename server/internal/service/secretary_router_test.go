package service

import (
	"context"
	"errors"
	"testing"
)

type fakeJSONModel struct {
	enabled bool
	reply   string
	err     error
}

func (f fakeJSONModel) Enabled() bool { return f.enabled }
func (f fakeJSONModel) GenerateJSON(_ context.Context, _, _, _ string, _ float64, _ int64) (string, error) {
	return f.reply, f.err
}

// The router takes only a route it knows. Anything else, or no model, is an
// unrouted turn: the Secretary classifies the message itself.
func TestChatRouteFor(t *testing.T) {
	cases := map[string]struct {
		model JSONLLM
		want  string
	}{
		"a known route is kept":         {fakeJSONModel{enabled: true, reply: `{"route":"judgment"}`}, ChatRouteJudgment},
		"an unknown route is dropped":   {fakeJSONModel{enabled: true, reply: `{"route":"gossip"}`}, ""},
		"malformed JSON is dropped":     {fakeJSONModel{enabled: true, reply: `route: fact`}, ""},
		"a failed call is dropped":      {fakeJSONModel{enabled: true, err: errors.New("upstream down")}, ""},
		"a disabled model is not asked": {fakeJSONModel{enabled: false, reply: `{"route":"fact"}`}, ""},
		"no model is not asked":         {nil, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ChatRouteFor(context.Background(), tc.model, "Is the launch on track?"); got != tc.want {
				t.Fatalf("ChatRouteFor() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestChatRouteForEmptyMessageIsUnrouted(t *testing.T) {
	if got := ChatRouteFor(context.Background(), fakeJSONModel{enabled: true, reply: `{"route":"fact"}`}, "   "); got != "" {
		t.Fatalf("empty message routed as %q", got)
	}
}
