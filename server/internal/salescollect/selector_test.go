package salescollect

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

const listPage = `<html><body>
<div class="jobs">
  <article class="job featured" id="j1"><h3 class="title"><a href="/jobs/1">Store manager</a></h3><span class="company">Phở Hà</span><time datetime="2026-11-07">7 Nov</time></article>
  <article class="job" id="j2"><h3 class="title"><a href="https://other.example/jobs/2">CNC operator</a></h3><span class="company">Máy Đông</span></article>
</div>
<p>not a job</p>
</body></html>`

func parse(t *testing.T, src string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestSelectorFindsItemsByTagClassIdAndAttribute(t *testing.T) {
	doc := parse(t, listPage)
	cases := map[string]int{
		"article":            2,
		".job":               2,
		"article.featured":   1,
		"#j2":                1,
		"div.jobs > article": 2,
		"body p":             1,
		"article[id]":        2,
		`article[id="j1"]`:   1,
		"h3 a[href]":         2,
		"section article":    0,
		"body > p":           1,
		"article > time":     1,
	}
	for src, want := range cases {
		sel, err := parseSelector(src)
		if err != nil {
			t.Fatalf("parseSelector(%q): %v", src, err)
		}
		if got := len(sel.findAll(doc)); got != want {
			t.Errorf("%q matched %d, want %d", src, got, want)
		}
	}
}

func TestSelectorRefusesWhatItCannotRead(t *testing.T) {
	for _, src := range []string{"a:first-child", "a + b", "", "> a", "a >", "[ ]", "a[href"} {
		if _, err := parseSelector(src); err == nil {
			t.Errorf("parseSelector(%q) was accepted", src)
		}
	}
}

func TestFieldSpecReadsTextAndAttributes(t *testing.T) {
	doc := parse(t, listPage)
	item := (selector{{comp: compound{tag: "article"}}}).findAll(doc)[0]

	text := func(raw string) string {
		spec, err := parseFieldSpec(raw)
		if err != nil {
			t.Fatalf("parseFieldSpec(%q): %v", raw, err)
		}
		return spec.read(item, "https://site.example/list")
	}
	if got := text(".company"); got != "Phở Hà" {
		t.Errorf("company text = %q", got)
	}
	if got := text("h3 a@href"); got != "https://site.example/jobs/1" {
		t.Errorf("relative link = %q", got)
	}
	if got := text("time@datetime"); got != "2026-11-07" {
		t.Errorf("time attribute = %q", got)
	}
	if got := text("h3 a"); got != "Store manager" {
		t.Errorf("link text = %q", got)
	}
	if got := text("span.missing"); got != "" {
		t.Errorf("a missing selector reads %q, want empty", got)
	}
}
