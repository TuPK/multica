package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/realtime"
)

var updateRouteTable = flag.Bool("update-route-table", false, "rewrite testdata/route_table.golden")

// The route table is a contract: every method + pattern with the number of middlewares in
// front of it and the handler that serves it. Moving route registrations between files (the
// fork keeps its own in router_fork*.go so upstream edits to router.go do not conflict) must
// leave it unchanged. Regenerate with `go test ./cmd/server -run TestRouteTable -update-route-table`
// only when a route is meant to change, and review the diff.
func TestRouteTableMatchesGolden(t *testing.T) {
	t.Setenv("JWT_SECRET", "router-test-secret")
	router, _ := NewRouterWithOptions(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil, RouterOptions{})

	var lines []string
	err := chi.Walk(router, func(method, route string, h http.Handler, mws ...func(http.Handler) http.Handler) error {
		lines = append(lines, fmt.Sprintf("%s %s mw=%d %s", method, route, len(mws), handlerName(h)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	got := strings.Join(lines, "\n") + "\n"

	const golden = "testdata/route_table.golden"
	if *updateRouteTable {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read %s: %v (run with -update-route-table to create it)", golden, err)
	}
	if string(want) != got {
		wantSet := map[string]bool{}
		for _, l := range strings.Split(string(want), "\n") {
			wantSet[l] = true
		}
		var added, removed []string
		for _, l := range lines {
			if !wantSet[l] {
				added = append(added, l)
			}
			delete(wantSet, l)
		}
		for l := range wantSet {
			if l != "" {
				removed = append(removed, l)
			}
		}
		sort.Strings(removed)
		t.Fatalf("route table changed.\nadded (%d):\n  %s\nremoved (%d):\n  %s", len(added), strings.Join(first(added, 15), "\n  "), len(removed), strings.Join(first(removed, 15), "\n  "))
	}
}

func first(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// handlerName names the function behind a handler, so a route that silently points at a
// different handler shows up as a diff.
func handlerName(h http.Handler) string {
	v := reflect.ValueOf(h)
	if v.Kind() == reflect.Func {
		return runtime.FuncForPC(v.Pointer()).Name()
	}
	return reflect.TypeOf(h).String()
}
