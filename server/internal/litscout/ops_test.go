package litscout

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/litscout/source"
)

func fastClient() *source.Client {
	return &source.Client{MaxRetries: 1, RetryMin: time.Millisecond, Sleep: func(context.Context, time.Duration) error { return nil }}
}

// minimalPDF builds a one-page PDF whose text is text, with a correct xref table.
func minimalPDF(text string) []byte {
	var b strings.Builder
	offsets := make([]int, 0, 6)
	obj := func(s string) {
		offsets = append(offsets, b.Len())
		b.WriteString(s)
	}
	b.WriteString("%PDF-1.4\n")
	obj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	obj("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	obj("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n")
	stream := fmt.Sprintf("BT /F1 12 Tf 20 100 Td (%s) Tj ET", text)
	obj(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream))
	obj("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
	xref := b.Len()
	b.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return []byte(b.String())
}

func TestExtractPDFText(t *testing.T) {
	got, err := ExtractPDFText(context.Background(), minimalPDF("Open access full text"))
	if err != nil || !strings.Contains(got, "Open access full text") {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := ExtractPDFText(context.Background(), []byte("<html>not a pdf</html>")); err == nil {
		t.Fatal("html accepted as PDF")
	}
	if _, err := ExtractPDFText(context.Background(), []byte("%PDF-1.4\ngarbage")); err == nil {
		t.Fatal("garbage PDF accepted")
	}
}

func TestGoPDFReaderIsUsedWhenItKeepsSpacing(t *testing.T) {
	got, err := goPDFText(minimalPDF("Spaced words stay spaced"))
	if err != nil || !strings.Contains(got, "Spaced words stay spaced") {
		t.Fatalf("%q %v", got, err)
	}
}

func TestLostSpacingDetection(t *testing.T) {
	prose := strings.Repeat("Graph retrieval improves legal answers for statute questions. ", 8)
	glued := strings.ReplaceAll(prose, " ", "")
	if lostSpacing(prose) {
		t.Fatal("normal prose flagged")
	}
	if !lostSpacing(glued) {
		t.Fatal("text without word breaks not flagged")
	}
	if lostSpacing("short") {
		t.Fatal("a short text has too little evidence to flag")
	}
}

func TestPDFFetcherRefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(minimalPDF("x")) }))
	defer srv.Close()
	// httptest listens on loopback, which the guarded default client must refuse.
	if _, err := (&PDFFetcher{}).Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("loopback download was allowed")
	}
	got, err := (&PDFFetcher{HTTP: srv.Client()}).Fetch(context.Background(), srv.URL)
	if err != nil || !strings.Contains(got, "x") {
		t.Fatalf("%q %v", got, err)
	}
}

func fulltextRun(t *testing.T, allow bool) *State {
	s := selectedRun(t)
	s.Config.AllowFullText = allow
	s.Gate(false)
	return s
}

func TestFetchFullTextOnlyWhenAllowedAndOpenAccess(t *testing.T) {
	var pdfHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/10.1/a":
			_, _ = w.Write([]byte(`{"is_oa":true,"best_oa_location":{"url_for_pdf":"` + "http://" + r.Host + `/paper.pdf"}}`))
		case r.URL.Path == "/v2/10.1/b":
			_, _ = w.Write([]byte(`{"is_oa":false,"best_oa_location":null}`))
		case r.URL.Path == "/paper.pdf":
			pdfHits++
			_, _ = w.Write(minimalPDF("Extracted body text"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	svc := Services{
		Unpaywall: &source.Unpaywall{Client: fastClient(), BaseURL: srv.URL + "/v2", Email: "me@example.org"},
		PDF:       &PDFFetcher{HTTP: srv.Client()},
	}
	dir := t.TempDir()

	if _, err := fulltextRun(t, false).FetchFullText(context.Background(), svc, dir); err == nil {
		t.Fatal("full text must be refused unless the run allows it")
	}
	s := fulltextRun(t, true)
	rep, err := s.FetchFullText(context.Background(), svc, dir)
	if err != nil || rep.Fetched != 1 || rep.NotOpenAccess != 1 || pdfHits != 1 {
		t.Fatalf("%+v %v hits=%d", rep, err, pdfHits)
	}
	info := s.Fulltext[s.Scored[0].ID]
	if info.Status != "ok" || info.Source != "Unpaywall" {
		t.Fatalf("%+v", info)
	}
	b, err := os.ReadFile(filepath.Join(dir, info.Path))
	if err != nil || !strings.Contains(string(b), "Extracted body text") {
		t.Fatalf("%q %v", b, err)
	}
	if !hasPrefix(s, "FULLTEXT_LOW_CONFIDENCE") || s.Fulltext[s.Scored[1].ID].Status != "not_open_access" {
		t.Fatalf("%v %+v", s.Warnings, s.Fulltext)
	}
}

func TestCheckDOIs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/cr/works/") && strings.Contains(r.URL.Path, "10.1/a"):
			_, _ = w.Write([]byte(`{"message":{"DOI":"10.1/a","title":["T"]}}`))
		case strings.HasPrefix(r.URL.Path, "/handles/") && strings.Contains(r.URL.Path, "10.1/b"):
			// Registered by another agency: Crossref says 404, the DOI proxy knows it.
			_, _ = w.Write([]byte(`{"responseCode":1,"handle":"10.1/b"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	svc := Services{Crossref: &source.Crossref{Client: fastClient(), BaseURL: srv.URL + "/cr", HandleURL: srv.URL + "/handles"}}

	s := selectedRun(t)
	s.Unique[1].DOI = "10.1/b"
	rep, err := s.CheckDOIs(context.Background(), svc)
	if err != nil || rep.Checked != 2 || rep.OK != 2 || rep.Unresolved != 0 {
		t.Fatalf("%+v %v", rep, err)
	}
	if s.DOIStatus[s.Scored[0].ID] != "ok" || s.DOIStatus[s.Scored[1].ID] != "ok_other_registry" {
		t.Fatalf("%+v", s.DOIStatus)
	}

	// A DOI that neither Crossref nor the proxy knows is unresolved.
	s = selectedRun(t)
	s.Unique[1].DOI = "10.1/nowhere"
	rep, err = s.CheckDOIs(context.Background(), svc)
	if err != nil || rep.OK != 1 || rep.Unresolved != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	if s.DOIStatus[s.Scored[1].ID] != "unresolved" || !hasPrefix(s, "DOI_UNRESOLVED") {
		t.Fatalf("%+v %v", s.DOIStatus, s.Warnings)
	}
	s.Gate(false)
	if r := s.Verify(VerifyOptions{RequireDOICheck: true}); !r.OK {
		t.Fatalf("an unresolved DOI is a warning, not an error: %+v", r)
	}
	if _, err := s.CheckDOIs(context.Background(), Services{}); err == nil {
		t.Fatal("missing Crossref must be an error")
	}
}

func TestChaseSpendsARoundAndAddsCitingAndCitedWorks(t *testing.T) {
	var filters []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := r.URL.Query().Get("filter")
		filters = append(filters, f)
		switch {
		case strings.HasPrefix(f, "cites:"):
			_, _ = w.Write([]byte(`{"results":[{"id":"https://openalex.org/W50","title":"Citing paper","authorships":[{"author":{"display_name":"Z"}}],"doi":"10.9/c"}]}`))
		default:
			_, _ = w.Write([]byte(`{"results":[{"id":"https://openalex.org/W60","title":"Cited paper","authorships":[{"author":{"display_name":"Y"}}],"doi":"10.9/d"}]}`))
		}
	}))
	defer srv.Close()
	s := selectedRun(t)
	s.Config.MaxRounds = 3
	s.Unique[0].OpenAlexID = "W1"
	s.Unique[0].ReferencedWorks = []string{"https://openalex.org/W60"}
	svc := Services{OpenAlex: &source.OpenAlex{Client: fastClient(), BaseURL: srv.URL}}
	rep, err := s.Chase(context.Background(), svc, 1, 5)
	if err != nil || s.SearchRound != 1 || len(s.Raw) != 4 || rep.Results[0].Count != 2 {
		t.Fatalf("%+v %v raw=%d", rep, err, len(s.Raw))
	}
	if strings.Join(filters, " ") != "cites:W1 openalex:W60" {
		t.Fatalf("%v", filters)
	}
	// The second selected paper has no OpenAlex id; it is skipped with a warning when asked for.
	if _, err := s.Chase(context.Background(), svc, 2, 5); err != nil {
		t.Fatal(err)
	}
	if !hasPrefix(s, "CHASE_SKIPPED_NO_OPENALEX_ID") {
		t.Fatalf("%v", s.Warnings)
	}
	s.SearchRound = s.Config.MaxRounds
	if _, err := s.Chase(context.Background(), svc, 1, 5); err == nil {
		t.Fatal("chasing past max_rounds must fail")
	}
}
