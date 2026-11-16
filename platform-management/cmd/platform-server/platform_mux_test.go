package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The console is reached only for requests that pass the price refusal, so a
// refused PUT never reaches the plan write.
func TestPlanPutWithAPriceFieldIsRefusedBeforeTheConsole(t *testing.T) {
	var consoleBodies []string
	console := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		consoleBodies = append(consoleBodies, r.Method+" "+string(b))
		w.WriteHeader(http.StatusOK)
	})
	priceChanges := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("price-change route reached for %s %s", r.Method, r.URL.Path)
	})
	h := newPlatformMux(priceChanges, console)

	refused := httptest.NewRecorder()
	h.ServeHTTP(refused, httptest.NewRequest(http.MethodPut, "/api/platform/plans/team",
		strings.NewReader(`{"name":"Team","seat_price_month":1}`)))
	if refused.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", refused.Code, refused.Body)
	}
	var body struct {
		Code   string   `json:"code"`
		Fields []string `json:"fields"`
	}
	if err := json.Unmarshal(refused.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", refused.Body, err)
	}
	if body.Code != "price_field_via_schedule" || len(body.Fields) != 1 || body.Fields[0] != "seat_price_month" {
		t.Fatalf("body %s", refused.Body)
	}
	if len(consoleBodies) != 0 {
		t.Fatalf("console reached for a refused PUT: %v", consoleBodies)
	}

	// A PUT without a price field reaches the console with its body unchanged.
	const nameOnly = `{"name":"Team"}`
	ok := httptest.NewRecorder()
	h.ServeHTTP(ok, httptest.NewRequest(http.MethodPut, "/api/platform/plans/team", strings.NewReader(nameOnly)))
	if ok.Code != http.StatusOK || len(consoleBodies) != 1 || consoleBodies[0] != "PUT "+nameOnly {
		t.Fatalf("status %d, console saw %v", ok.Code, consoleBodies)
	}
}

func TestPlanGetAndPriceChangeRoutesAreNotRefused(t *testing.T) {
	var consoleCalls, priceCalls int
	console := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { consoleCalls++ })
	priceChanges := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { priceCalls++ })
	h := newPlatformMux(priceChanges, console)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/platform/plans/team", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/platform/plans/team/price-changes", strings.NewReader(`{"seat_price_month":1}`)))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/api/platform/plans/team/price-changes/scheduled", nil))
	if consoleCalls != 1 || priceCalls != 2 {
		t.Fatalf("console %d, price changes %d; want 1 and 2", consoleCalls, priceCalls)
	}
}
