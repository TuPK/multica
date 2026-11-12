package main

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/realtime"
)

// The Research Lab settings service is built by the router, and its three routes exist. Without the
// service every research endpoint would answer 503, and nothing else in this package would notice.
func TestRouterWiresTheResearchSettings(t *testing.T) {
	t.Setenv("JWT_SECRET", "router-test-secret")
	t.Setenv("MULTICA_RESEARCH_SECRET_KEY", "")
	router, h := NewRouterWithOptions(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil, RouterOptions{})

	if h.Research == nil {
		t.Fatal("the router did not build the research settings service")
	}
	if h.Research.Box == nil {
		t.Fatal("with JWT_SECRET set the service must be able to seal keys")
	}

	want := map[string]bool{
		http.MethodGet + " /api/research/settings":    false,
		http.MethodPut + " /api/research/settings":    false,
		http.MethodGet + " /api/research/credentials": false,
	}
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if _, ok := want[method+" "+route]; ok {
			want[method+" "+route] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for route, found := range want {
		if !found {
			t.Errorf("route %s is not registered", route)
		}
	}
}
