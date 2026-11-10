package salescollect

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeWeb answers requests from a map of URL to body, so tests never reach the network. A URL
// not in the map answers 404, and robots.txt answers 404 unless the map says otherwise.
type fakeWeb struct {
	pages map[string]fakePage
	hits  []string
}

type fakePage struct {
	status int
	body   []byte
	ctype  string
}

func (w *fakeWeb) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	u := req.URL.String()
	w.hits = append(w.hits, u)
	page, ok := w.pages[u]
	if !ok {
		page = fakePage{status: http.StatusNotFound, body: []byte("not found")}
	}
	if page.status == 0 {
		page.status = http.StatusOK
	}
	ctype := page.ctype
	if ctype == "" {
		ctype = "text/html; charset=utf-8"
	}
	return &http.Response{
		StatusCode: page.status,
		Status:     http.StatusText(page.status),
		Header:     http.Header{"Content-Type": []string{ctype}},
		Body:       io.NopCloser(bytes.NewReader(page.body)),
		Request:    req,
	}, nil
}

func (w *fakeWeb) client() *http.Client { return &http.Client{Transport: w, Timeout: 5 * time.Second} }

// options returns test options that read from the fake web with no rate wait.
func (w *fakeWeb) options() Options {
	return Options{HTTP: w.client(), RateInterval: time.Nanosecond, Limit: 20}
}

// fixture reads a saved response from testdata.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// mustRecipe parses a recipe JSON literal, failing the test on error.
func mustRecipe(t *testing.T, raw string) *Recipe {
	t.Helper()
	r, err := ParseRecipe([]byte(raw))
	if err != nil {
		t.Fatalf("ParseRecipe: %v", err)
	}
	return r
}

// pinNow fixes the clock for filter tests.
func pinNow(t *testing.T, at time.Time) {
	t.Helper()
	old := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = old })
}
