package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/litscout/cache"
)

type clock struct {
	slept []time.Duration
}

func (c *clock) sleep(_ context.Context, d time.Duration) error {
	c.slept = append(c.slept, d)
	return nil
}

func testClient(c *clock, dir string) *Client {
	cl := &Client{MaxRetries: 3, RetryMin: 5 * time.Second, Sleep: c.sleep, Rand: func() float64 { return 0 }}
	if dir != "" {
		cl.Cache = &cache.Cache{Dir: dir, TTL: time.Hour}
	}
	return cl
}

const openAlexPage = `{"results":[
 {"id":"https://openalex.org/W1","doi":"https://doi.org/10.1/ABC","title":" Graph RAG for law ","publication_date":"2025-03-31","type":"article",
  "abstract_inverted_index":{"law":[2],"Graph":[0],"RAG":[1],"for":[1]},
  "authorships":[{"author":{"display_name":"A One"}},{"author":null},{"author":{"display_name":"B Two"}}],
  "primary_location":{"landing_page_url":"https://example.org/p1"},"open_access":{"oa_url":"https://example.org/p1.pdf"},"referenced_works":["https://openalex.org/W9"]},
 {"id":"https://openalex.org/W2","title":"Front matter","is_paratext":true,"publication_date":"bad-date","authorships":[]},
 {"id":["not","a","string"]}
]}`

func TestRestoreAbstractOrdersByPositionThenWord(t *testing.T) {
	got := RestoreAbstract(map[string][]int{"b": {1}, "a": {1}, "z": {0}, "y": {2}})
	if got != "z a b y" {
		t.Fatalf("got %q", got)
	}
	if RestoreAbstract(nil) != "" || RestoreAbstract(map[string][]int{"x": {}}) != "" {
		t.Fatal("empty index must give empty abstract")
	}
}

func TestOpenAlexSearchNormalizes(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(openAlexPage))
	}))
	defer srv.Close()
	oa := &OpenAlex{Client: testClient(&clock{}, ""), BaseURL: srv.URL, APIKey: "KEY123"}
	got, err := oa.Search(context.Background(), Query{Text: "graph rag", FromYear: 2020, Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 papers (malformed work skipped), got %d", len(got))
	}
	p := got[0]
	if p.Title != "Graph RAG for law" || p.DOI != "10.1/abc" || p.OpenAlexID != "W1" || p.URL != "https://example.org/p1" {
		t.Fatalf("bad normalisation: %+v", p)
	}
	if p.Abstract != "Graph RAG for law" || p.AbstractSource != "OpenAlex" {
		t.Fatalf("abstract %q / %q", p.Abstract, p.AbstractSource)
	}
	if strings.Join(p.Authors, ",") != "A One,B Two" || p.PublicationDate != "2025-03-31" || p.OpenAccessURL == "" {
		t.Fatalf("bad fields: %+v", p)
	}
	if got[1].Type != "paratext" || got[1].PublicationDate != "" {
		t.Fatalf("paratext/bad date: %+v", got[1])
	}
	for _, want := range []string{"per_page=100", "api_key=KEY123", "from_publication_date%3A2020-01-01"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestCacheNeverStoresTheAPIKeyAndSavesRequests(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(openAlexPage))
	}))
	defer srv.Close()
	dir := t.TempDir()
	oa := &OpenAlex{Client: testClient(&clock{}, dir), BaseURL: srv.URL, APIKey: "SUPERSECRETKEY"}
	for i := 0; i < 2; i++ {
		if _, err := oa.Search(context.Background(), Query{Text: "q", Limit: 5}); err != nil {
			t.Fatal(err)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("second call should hit the cache, server saw %d", hits.Load())
	}
	entries, _ := filepath.Glob(filepath.Join(dir, "*"))
	for _, e := range entries {
		if strings.Contains(e, "SUPERSECRETKEY") {
			t.Fatal("key in cache file name")
		}
		b, _ := os.ReadFile(e)
		if strings.Contains(string(b), "SUPERSECRETKEY") {
			t.Fatal("key in cache content")
		}
	}
	// A different key must not change the cache key: the key is credentials, not a parameter.
	other := &OpenAlex{Client: oa.Client, BaseURL: srv.URL, APIKey: "OTHER"}
	if _, err := other.Search(context.Background(), Query{Text: "q", Limit: 5}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatal("same query with another key should reuse the cache")
	}
}

func TestRetryHonoursRetryAfterAndBackoff(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch n.Add(1) {
		case 1:
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusBadGateway)
		default:
			_, _ = w.Write([]byte(`{"results":[]}`))
		}
	}))
	defer srv.Close()
	ck := &clock{}
	oa := &OpenAlex{Client: testClient(ck, ""), BaseURL: srv.URL}
	if _, err := oa.Search(context.Background(), Query{Text: "q", Limit: 5}); err != nil {
		t.Fatal(err)
	}
	if len(ck.slept) != 2 || ck.slept[0] != 30*time.Second || ck.slept[1] != 10*time.Second {
		t.Fatalf("sleeps %v", ck.slept)
	}
}

func TestRetryAfterAsHTTPDate(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	got := ParseRetryAfter(now.Add(45*time.Second).UTC().Format(http.TimeFormat), now)
	if got != 45*time.Second {
		t.Fatalf("got %v", got)
	}
	if ParseRetryAfter("", now) != 0 || ParseRetryAfter("soon", now) != 0 {
		t.Fatal("invalid values must give 0")
	}
}

func TestRateLimitedAfterAllRetries(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	oa := &OpenAlex{Client: testClient(&clock{}, ""), BaseURL: srv.URL}
	_, err := oa.Search(context.Background(), Query{Text: "q", Limit: 1})
	if !errors.Is(err, ErrRateLimited) || n.Load() != 4 {
		t.Fatalf("err=%v tries=%d", err, n.Load())
	}
}

func TestOpenAlexQuotaIsNotRetried(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"Insufficient budget"}`))
	}))
	defer srv.Close()
	oa := &OpenAlex{Client: testClient(&clock{}, ""), BaseURL: srv.URL, APIKey: "k"}
	_, err := oa.Search(context.Background(), Query{Text: "q", Limit: 1})
	if !errors.Is(err, ErrQuotaExhausted) || n.Load() != 1 {
		t.Fatalf("err=%v tries=%d", err, n.Load())
	}
	if strings.Contains(err.Error(), "api_key") {
		t.Fatal("error leaks the request")
	}
}

func TestClientErrorsAreNotRetriedAndHideTheURL(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	oa := &OpenAlex{Client: testClient(&clock{}, ""), BaseURL: srv.URL, APIKey: "TOPSECRET"}
	_, err := oa.Search(context.Background(), Query{Text: "q", Limit: 1})
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 400 || n.Load() != 1 || strings.Contains(err.Error(), "TOPSECRET") {
		t.Fatalf("err=%v tries=%d", err, n.Load())
	}
}

func TestOpenAlexCitedByAndByIDs(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("filter"))
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	oa := &OpenAlex{Client: testClient(&clock{}, ""), BaseURL: srv.URL}
	_, _ = oa.CitedBy(context.Background(), "https://openalex.org/W5", 10)
	_, _ = oa.ByIDs(context.Background(), []string{"https://openalex.org/W1", "W2"})
	if len(queries) != 2 || queries[0] != "cites:W5" || queries[1] != "openalex:W1|W2" {
		t.Fatalf("filters %v", queries)
	}
}

func TestCrossrefSearchAndResolve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/works"):
			_, _ = w.Write([]byte(`{"message":{"items":[{"DOI":"10.1/X","URL":"https://doi.org/10.1/x","type":"journal-article","title":["T"],
			 "abstract":"<jats:p>An <i>abstract</i>.</jats:p>","author":[{"given":"G","family":"F"},{"name":"Org"}],"issued":{"date-parts":[[2024,5]]}}]}}`))
		case strings.Contains(r.URL.Path, "10.1%2Fmissing") || strings.Contains(r.URL.Path, "10.1/missing"):
			w.WriteHeader(http.StatusNotFound)
		default:
			_, _ = w.Write([]byte(`{"message":{"DOI":"10.1/X","title":["T"]}}`))
		}
	}))
	defer srv.Close()
	c := &Crossref{Client: testClient(&clock{}, ""), BaseURL: srv.URL, Mailto: "me@example.org"}
	got, err := c.Search(context.Background(), Query{Text: "q", FromYear: 2020, Limit: 5})
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	p := got[0]
	if p.DOI != "10.1/x" || p.Abstract != "An abstract ." || p.PublicationDate != "2024-05" || strings.Join(p.Authors, "|") != "G F|Org" || p.MetadataSource != "Crossref" {
		t.Fatalf("%+v", p)
	}
	if _, found, err := c.Resolve(context.Background(), "10.1/X"); err != nil || !found {
		t.Fatalf("resolve existing: %v %v", found, err)
	}
	if _, found, err := c.Resolve(context.Background(), "10.1/missing"); err != nil || found {
		t.Fatalf("resolve missing: %v %v", found, err)
	}
}

func TestArxivParsesAtomAndFiltersYear(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("search_query"), "all:graph AND all:rag") {
			t.Errorf("search_query %q", r.URL.Query().Get("search_query"))
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom">
		<entry><id>http://arxiv.org/abs/2501.01234v2</id><title>  Graph
		 RAG </title><summary> Sum  mary </summary><published>2025-01-03T00:00:00Z</published>
		 <author><name>A</name></author><link title="pdf" href="http://arxiv.org/pdf/2501.01234v2" type="application/pdf"/></entry>
		<entry><id>http://arxiv.org/abs/1801.00001v1</id><title>Old</title><summary>x</summary><published>2018-01-01T00:00:00Z</published></entry>
		</feed>`))
	}))
	defer srv.Close()
	a := &Arxiv{Client: testClient(&clock{}, ""), BaseURL: srv.URL}
	got, err := a.Search(context.Background(), Query{Text: "graph rag", FromYear: 2020, Limit: 5})
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	p := got[0]
	if p.Title != "Graph RAG" || p.ArxivID != "2501.01234" || p.Abstract != "Sum mary" || p.OpenAccessURL == "" || p.Type != "preprint" {
		t.Fatalf("%+v", p)
	}
	if ArxivID("http://arxiv.org/abs/hep-th/9901001v3") != "hep-th/9901001" {
		t.Fatal("old-style id")
	}
}

func TestSemanticScholarSearchSendsKeyAsHeader(t *testing.T) {
	var key, rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, rawQuery = r.Header.Get("x-api-key"), r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[{"paperId":"abc","title":"T","abstract":null,"url":"https://s2/abc","year":2023,"authors":[{"name":"A"}],
		 "externalIds":{"DOI":"10.9/Z","ArXiv":"2301.1"},"publicationTypes":["JournalArticle"],"openAccessPdf":{"url":"https://x/pdf"}}]}`))
	}))
	defer srv.Close()
	s := &SemanticScholar{Client: testClient(&clock{}, ""), BaseURL: srv.URL, APIKey: "S2KEY"}
	got, err := s.Search(context.Background(), Query{Text: "q", FromYear: 2020, Limit: 5})
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	if key != "S2KEY" || strings.Contains(rawQuery, "S2KEY") {
		t.Fatalf("key=%q query=%q", key, rawQuery)
	}
	p := got[0]
	if p.Abstract != "" || p.AbstractSource != "" || p.DOI != "10.9/z" || p.ArxivID != "2301.1" || p.PublicationDate != "2023" || p.S2ID != "abc" {
		t.Fatalf("%+v", p)
	}
}

func TestUnpaywall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "10.1/oa"):
			_, _ = w.Write([]byte(`{"is_oa":true,"best_oa_location":{"url_for_pdf":"https://x/p.pdf"}}`))
		case strings.Contains(r.URL.Path, "10.1/closed"):
			_, _ = w.Write([]byte(`{"is_oa":false,"best_oa_location":null}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	u := &Unpaywall{Client: testClient(&clock{}, ""), BaseURL: srv.URL, Email: "me@example.org"}
	if got, ok, err := u.OALocation(context.Background(), "10.1/OA"); err != nil || !ok || got != "https://x/p.pdf" {
		t.Fatalf("%q %v %v", got, ok, err)
	}
	if _, ok, err := u.OALocation(context.Background(), "10.1/closed"); err != nil || ok {
		t.Fatalf("closed: %v %v", ok, err)
	}
	if _, ok, err := u.OALocation(context.Background(), "10.1/none"); err != nil || ok {
		t.Fatalf("404: %v %v", ok, err)
	}
	if _, _, err := (&Unpaywall{Client: u.Client}).OALocation(context.Background(), "10.1/oa"); err == nil {
		t.Fatal("missing email must be an error")
	}
}

func TestMinIntervalSpacesRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"results":[]}`)) }))
	defer srv.Close()
	ck := &clock{}
	now := time.Unix(100, 0)
	cl := testClient(ck, "")
	cl.MinInterval = 3 * time.Second
	cl.Now = func() time.Time { return now }
	oa := &OpenAlex{Client: cl, BaseURL: srv.URL}
	for i := 0; i < 2; i++ {
		_, _ = oa.Search(context.Background(), Query{Text: "q" + string(rune('a'+i)), Limit: 1})
	}
	if len(ck.slept) != 1 || ck.slept[0] != 3*time.Second {
		t.Fatalf("sleeps %v", ck.slept)
	}
}

func TestCrossrefRegisteredUsesTheDOIProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "10.48550/arxiv.2409.13699") {
			_, _ = w.Write([]byte(`{"responseCode":1,"handle":"10.48550/arxiv.2409.13699"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := &Crossref{Client: testClient(&clock{}, ""), HandleURL: srv.URL}
	if ok, err := c.Registered(context.Background(), "https://doi.org/10.48550/arXiv.2409.13699"); err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if ok, err := c.Registered(context.Background(), "10.9999/none"); err != nil || ok {
		t.Fatalf("%v %v", ok, err)
	}
	if ok, err := c.Registered(context.Background(), ""); err != nil || ok {
		t.Fatalf("%v %v", ok, err)
	}
}
