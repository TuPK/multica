package salescollect

import "testing"

func TestNormalizeDateReadsTheFormatsSourcesUse(t *testing.T) {
	cases := map[string]string{
		"Tue, 10 Nov 2026 04:18:00 GMT": "2026-11-10",
		"2026-11-08T16:00:15+00:00":     "2026-11-08",
		"2026-11-08":                    "2026-11-08",
		"1794293724":                    "2026-11-10",
		"not a date":                    "",
		"":                              "",
	}
	for in, want := range cases {
		if got := normalizeDate(in); got != want {
			t.Errorf("normalizeDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEntityKeyPrefersDomainThenTaxCodeThenName(t *testing.T) {
	if got := EntityKey("Công ty A", "https://www.Example.vn/about", ""); got != "domain:example.vn" {
		t.Errorf("domain key = %q", got)
	}
	if got := EntityKey("Công ty A", "", "MST 0312345678 đăng ký"); got != "tax:0312345678" {
		t.Errorf("tax key = %q", got)
	}
	if got := EntityKey("Công  ty, A.", "", "no tax here"); got != "name:công ty a" {
		t.Errorf("name key = %q", got)
	}
}

func TestEntityKeyIsStableForTheSameOrganisation(t *testing.T) {
	a := EntityKey("ACME Co.", "acme.example", "")
	b := EntityKey("Acme Co", "https://acme.example/news", "a different summary")
	if a != b {
		t.Fatalf("the same organisation got two keys: %q and %q", a, b)
	}
}

func TestCleanTextKeepsOneShortLine(t *testing.T) {
	long := ""
	for i := 0; i < 400; i++ {
		long += "x"
	}
	if got := cleanText("  a\n\n b  "); got != "a b" {
		t.Errorf("cleanText = %q", got)
	}
	if got := cleanText(long); len([]rune(got)) != maxSummary {
		t.Errorf("summary length = %d, want %d", len([]rune(got)), maxSummary)
	}
}
