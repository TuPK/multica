package paper

import "testing"

func TestNormalizeDOI(t *testing.T) {
	cases := map[string]string{
		"https://doi.org/10.59232/DST-V5I1P101": "10.59232/dst-v5i1p101",
		"http://dx.doi.org/10.1/ABC":            "10.1/abc",
		"doi:10.1/x":                            "10.1/x",
		"  ":                                    "",
		"10.1/Plain":                            "10.1/plain",
	}
	for in, want := range cases {
		if got := NormalizeDOI(in); got != want {
			t.Errorf("NormalizeDOI(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNormalizeOpenAlexIDAndID(t *testing.T) {
	if got := NormalizeOpenAlexID("https://openalex.org/W123"); got != "W123" {
		t.Fatal(got)
	}
	if NewID("a") == NewID("b") || NewID("a") != NewID("a") {
		t.Fatal("NewID must be deterministic and distinct")
	}
	if (Metadata{PublicationDate: "2025-03-31"}).Year() != "2025" || (Metadata{}).Year() != "" {
		t.Fatal("Year")
	}
}
