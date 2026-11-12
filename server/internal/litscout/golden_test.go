package litscout

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
	"github.com/multica-ai/multica/server/internal/litscout/source"
)

// These tests compare the Go port with LitScout's own Python code on the same inputs. The expected
// values in testdata/python_golden/golden.json were produced by generate.py from the original
// services (dedup, prefilter, heuristic score, abstract restoration, DOI normalisation) and from
// rapidfuzz. The one deliberate difference is how accents are treated; see TestNormalizeAccentsDifferOnPurpose.

type goldenPaper struct {
	Title      string   `json:"title"`
	Authors    []string `json:"authors"`
	DOI        string   `json:"doi"`
	OpenAlexID string   `json:"openalex_id"`
	Abstract   string   `json:"abstract"`
}

func (g goldenPaper) metadata() paper.Metadata {
	m := paper.Metadata{Title: g.Title, Authors: g.Authors, DOI: g.DOI, OpenAlexID: g.OpenAlexID, Abstract: g.Abstract, MetadataSource: "OpenAlex"}
	if g.Abstract != "" {
		m.AbstractSource = "OpenAlex"
	}
	return m
}

type golden struct {
	Source          string                     `json:"source"`
	NormalizeASCII  []struct{ In, Out string } `json:"normalize_ascii"`
	NormalizeAccent []struct {
		In     string `json:"in"`
		Python string `json:"python"`
	} `json:"normalize_accent"`
	Ratio []struct {
		A     string  `json:"a"`
		B     string  `json:"b"`
		Ratio float64 `json:"ratio"`
	} `json:"ratio"`
	Same []struct {
		A    goldenPaper `json:"a"`
		B    goldenPaper `json:"b"`
		Same bool        `json:"same"`
	} `json:"same"`
	Dedup struct {
		Input  []goldenPaper `json:"input"`
		Unique []goldenPaper `json:"unique"`
	} `json:"dedup"`
	Prefilter []struct {
		Query  string   `json:"query"`
		Limit  int      `json:"limit"`
		Titles []string `json:"titles"`
	} `json:"prefilter"`
	PrefilterPapers []goldenPaper `json:"prefilter_papers"`
	Heuristic       []struct {
		Query    string  `json:"query"`
		Title    string  `json:"title"`
		Abstract string  `json:"abstract"`
		Score    float64 `json:"score"`
	} `json:"heuristic"`
	Abstracts []struct {
		Index map[string][]int `json:"index"`
		Text  string           `json:"text"`
	} `json:"abstracts"`
	DOIs []struct{ In, Out string } `json:"dois"`
}

func loadGolden(t *testing.T) golden {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "python_golden", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(g.Source, "LitScout") {
		t.Fatalf("unexpected golden source %q", g.Source)
	}
	return g
}

func TestGoldenNormalizeMatchesPythonForASCII(t *testing.T) {
	for _, c := range loadGolden(t).NormalizeASCII {
		if got := Normalize(c.In); got != c.Out {
			t.Errorf("Normalize(%q) = %q, Python gives %q", c.In, got, c.Out)
		}
	}
}

// Python splits an accented word where the accent was (NFKD, then the combining mark is replaced by a
// space), so "Phân" becomes "pha n". The Go port removes the accent instead, which is what the dedup
// wants: "Phân" and "Phan" are one word. Both sides stay self-consistent, so a duplicate is found
// either way; only the normalised text differs.
func TestNormalizeAccentsDifferOnPurpose(t *testing.T) {
	want := map[string]string{
		"Phân tích Pháp luật Việt Nam": "phan tich phap luat viet nam",
		"Café—Déjà vu":                 "cafe deja vu",
		"Straße":                       "strasse",
		"Ångström":                     "angstrom",
		"naïve résumé":                 "naive resume",
		"Đại học Bách khoa":            "đai hoc bach khoa",
	}
	cases := loadGolden(t).NormalizeAccent
	if len(cases) != len(want) {
		t.Fatalf("golden has %d accent cases, test has %d", len(cases), len(want))
	}
	for _, c := range cases {
		if got := Normalize(c.In); got != want[c.In] {
			t.Errorf("Normalize(%q) = %q, want %q", c.In, got, want[c.In])
		}
		if c.Python == want[c.In] && c.In != "Straße" {
			t.Errorf("%q: Python and Go agree, so this is no longer a difference", c.In)
		}
	}
}

func TestGoldenIndelRatioMatchesRapidfuzz(t *testing.T) {
	g := loadGolden(t)
	if len(g.Ratio) < 50 {
		t.Fatalf("only %d ratio cases", len(g.Ratio))
	}
	for _, c := range g.Ratio {
		if got := IndelRatio(c.A, c.B); math.Abs(got-c.Ratio) > 1e-6 {
			t.Errorf("IndelRatio(%q, %q) = %.6f, rapidfuzz gives %.6f", c.A, c.B, got, c.Ratio)
		}
	}
}

func TestGoldenSameMatchesPython(t *testing.T) {
	for i, c := range loadGolden(t).Same {
		// Python merges two untitled records that share a first author, because rapidfuzz scores two empty
		// strings as 100. The port does not: with no title there is no evidence. Untitled records are also
		// dropped before dedup, so this cannot be reached from a run.
		if Normalize(c.A.Title) == "" && Normalize(c.B.Title) == "" {
			if Same(c.A.metadata(), c.B.metadata()) {
				t.Errorf("case %d: two untitled records must not be merged", i)
			}
			continue
		}
		if got := Same(c.A.metadata(), c.B.metadata()); got != c.Same {
			t.Errorf("case %d (%q vs %q): Same = %v, Python gives %v", i, c.A.Title, c.B.Title, got, c.Same)
		}
	}
}

func TestGoldenDedupMatchesPython(t *testing.T) {
	g := loadGolden(t)
	var in []paper.Metadata
	for _, p := range g.Dedup.Input {
		in = append(in, p.metadata())
	}
	got := Deduplicate(nil, in)
	if len(got) != len(g.Dedup.Unique) {
		t.Fatalf("Go keeps %d papers, Python keeps %d", len(got), len(g.Dedup.Unique))
	}
	for i, want := range g.Dedup.Unique {
		m := got[i]
		if m.Title != want.Title || m.DOI != want.DOI || m.OpenAlexID != want.OpenAlexID || m.Abstract != want.Abstract {
			t.Errorf("paper %d: Go {%q %q %q %q}, Python {%q %q %q %q}", i,
				m.Title, m.DOI, m.OpenAlexID, m.Abstract, want.Title, want.DOI, want.OpenAlexID, want.Abstract)
		}
	}
}

func TestGoldenPrefilterMatchesPython(t *testing.T) {
	g := loadGolden(t)
	var papers []paper.Metadata
	for _, p := range g.PrefilterPapers {
		papers = append(papers, p.metadata())
	}
	for _, c := range g.Prefilter {
		var titles []string
		for _, p := range Prefilter(c.Query, papers, c.Limit) {
			titles = append(titles, p.Title)
		}
		if strings.Join(titles, "|") != strings.Join(c.Titles, "|") {
			t.Errorf("Prefilter(%q, limit %d):\n Go     %v\n Python %v", c.Query, c.Limit, titles, c.Titles)
		}
	}
}

func TestGoldenHeuristicScoreMatchesPython(t *testing.T) {
	for _, c := range loadGolden(t).Heuristic {
		got, _ := HeuristicScore(c.Query, paper.Metadata{Title: c.Title, Abstract: c.Abstract})
		if math.Abs(got-c.Score) > 1e-9 {
			t.Errorf("HeuristicScore(%q, %q) = %v, Python gives %v", c.Query, c.Title, got, c.Score)
		}
	}
}

func TestGoldenAbstractRestoreMatchesPython(t *testing.T) {
	for _, c := range loadGolden(t).Abstracts {
		if got := source.RestoreAbstract(c.Index); got != c.Text {
			t.Errorf("RestoreAbstract(%v) = %q, Python gives %q", c.Index, got, c.Text)
		}
	}
}

func TestGoldenDOINormalisationMatchesPython(t *testing.T) {
	for _, c := range loadGolden(t).DOIs {
		if got := paper.NormalizeDOI(c.In); got != c.Out {
			t.Errorf("NormalizeDOI(%q) = %q, Python gives %q", c.In, got, c.Out)
		}
	}
}
