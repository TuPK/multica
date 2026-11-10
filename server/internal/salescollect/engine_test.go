package salescollect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeEngine renders every page to one fixed HTML body and counts the calls.
type fakeEngine struct {
	html  string
	calls []string
}

func (e *fakeEngine) HTML(_ context.Context, rawURL string) (*Response, error) {
	e.calls = append(e.calls, rawURL)
	return &Response{Status: 200, ContentType: "text/html", Body: []byte(e.html), FinalURL: rawURL}, nil
}

func (e *fakeEngine) Status(context.Context) WebEngineStatus {
	return WebEngineStatus{Status: "running"}
}

func TestNewCrawl4AIRefusesAnAddressOtherMachinesCanReach(t *testing.T) {
	for _, base := range []string{"http://10.0.0.5:11235", "http://example.com:11235", "ftp://127.0.0.1"} {
		if _, err := NewCrawl4AI(base, "token"); err == nil {
			t.Errorf("NewCrawl4AI(%q) accepted an address that is not loopback", base)
		}
	}
	if _, err := NewCrawl4AI("http://127.0.0.1:11235", ""); err == nil {
		t.Error("an engine without a token was accepted")
	}
	if _, err := NewCrawl4AI("http://localhost:11235", "token"); err != nil {
		t.Errorf("localhost is loopback, got %v", err)
	}
}

func TestCrawl4AIHTMLSendsTheTokenAndReadsTheReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok","version":"0.9.5"}`))
			return
		}
		if r.URL.Path != "/crawl" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"results":[{"success":true,"html":"<ul><li>Mở chi nhánh</li></ul>","status_code":200,"error_message":""}]}`))
	}))
	defer srv.Close()
	engine, err := NewCrawl4AI(srv.URL, "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := engine.HTML(context.Background(), "https://example.vn/tin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp.Body), "Mở chi nhánh") {
		t.Fatalf("the rendered HTML did not come back: %q", resp.Body)
	}
	if got := engine.Status(context.Background()); got.Status != "running" || got.Version != "0.9.5" {
		t.Fatalf("a reachable engine must report running with its version, got %+v", got)
	}
}

func TestCrawl4AIRefusesAFailedRender(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"results":[{"success":false,"html":"","status_code":0,"error_message":"timeout"}]}`))
	}))
	defer srv.Close()
	engine, _ := NewCrawl4AI(srv.URL, "t")
	if _, err := engine.HTML(context.Background(), "https://example.vn/"); err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("a failed render must be an error with its reason, got %v", err)
	}
}

func TestPageNeverSendsAPrivateAddressToTheEngine(t *testing.T) {
	engine := &fakeEngine{html: "<p>x</p>"}
	f := newFetcher(Options{HTTP: (&fakeWeb{}).client(), RateInterval: time.Nanosecond, Engine: engine})
	if _, err := f.page(context.Background(), "http://127.0.0.1:8080/admin", true); err == nil {
		t.Fatal("a loopback page was sent to the engine")
	}
	if len(engine.calls) != 0 {
		t.Fatalf("the engine was asked for %v", engine.calls)
	}
}

func TestBrowserRecipeIsRefusedWithoutAnEngine(t *testing.T) {
	raw := `{"version":1,"rung":5,"method":"html_list","fetch":{"engine":"crawl4ai","url":"https://example.vn/ds","render":"browser"},"parse":{"items":"li","fields":{"entity":"a","url":"a@href","observed_at":"time","text":"p"}},"filter":{},"map":{"family":"directory","entity_from":"field"},"max_items":10}`
	res := Run(context.Background(), mustRecipe(t, raw), Options{HTTP: (&fakeWeb{}).client(), RateInterval: time.Nanosecond})
	if res.Status != "error" || !strings.Contains(res.Error, "none is running") {
		t.Fatalf("status %s, error %q: a browser recipe without an engine must stop with the reason", res.Status, res.Error)
	}
}

func TestBrowserRecipeReadsThroughTheEngine(t *testing.T) {
	engine := &fakeEngine{html: `<ul class="list"><li><a href="https://example.vn/a">Mở chi nhánh Hà Nội</a><time>2026-11-09</time><p>khai trương</p></li></ul>`}
	raw := `{"version":1,"rung":5,"method":"html_list","fetch":{"engine":"crawl4ai","url":"https://example.vn/ds","render":"browser"},"parse":{"items":"ul.list li","fields":{"entity":"a","url":"a@href","observed_at":"time","text":"p"}},"filter":{},"map":{"family":"expansion","entity_from":"field"},"max_items":10}`
	w := &fakeWeb{pages: map[string]fakePage{}}
	res := Run(context.Background(), mustRecipe(t, raw), Options{HTTP: w.client(), RateInterval: time.Nanosecond, Engine: engine, allowPrivate: false})
	if len(engine.calls) != 1 || engine.calls[0] != "https://example.vn/ds" {
		t.Fatalf("engine calls %v", engine.calls)
	}
	if res.Error != "" {
		t.Fatalf("unexpected error %q", res.Error)
	}
}
