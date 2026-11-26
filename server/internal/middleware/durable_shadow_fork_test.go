package middleware

import (
	"net/http"
	"testing"
)

// TestShadowTaskCliMutation403: a shadow run's request that changes workspace data is refused
// with 403 shadow_task, and its reads are not (durable spec 9.4 (d)).
func TestShadowTaskCliMutation403(t *testing.T) {
	shadow := []byte(`{"durable_kind":"shadow","shadow_pair_id":"p"}`)
	plain := []byte(`{"wakeup_id":"w"}`)
	cases := []struct {
		method  string
		context []byte
		refused bool
	}{
		{http.MethodPost, shadow, true},
		{http.MethodPatch, shadow, true},
		{http.MethodDelete, shadow, true},
		{http.MethodGet, shadow, false},
		{http.MethodHead, shadow, false},
		{http.MethodPost, plain, false},
		{http.MethodPost, nil, false},
	}
	for _, c := range cases {
		if got := shadowMutationRefused(c.method, c.context); got != c.refused {
			t.Errorf("%s with context %s: refused = %v, want %v", c.method, c.context, got, c.refused)
		}
	}
}
