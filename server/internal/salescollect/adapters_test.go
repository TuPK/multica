package salescollect

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const feedURL = "https://news.example/feed"

// rssRecipe reads a feed whose items carry the four standard tags.
func rssRecipe(url string, family string) string {
	return `{"version":1,"rung":2,"method":"rss","fetch":{"engine":"http","url":"` + url + `","render":"none"},"parse":{"items":"","fields":{"entity":"title","url":"link","observed_at":"pubDate","text":"description"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"` + family + `","entity_from":"field"},"max_items":50}`
}

func TestRSSAdapterReadsGoogleNewsItemsWithPublisherAndDate(t *testing.T) {
	w := &fakeWeb{pages: map[string]fakePage{
		feedURL: {body: fixture(t, "gnews_expansion.xml"), ctype: "application/rss+xml"},
	}}
	res := Run(context.Background(), mustRecipe(t, rssRecipe(feedURL, "expansion")), w.options())
	if res.Status != "ok" {
		t.Fatalf("status = %s (%s), coverage %.2f over %d items", res.Status, res.Error, res.FieldCoverage, res.ItemsSeen)
	}
	if res.ItemsSeen != 5 || res.FieldCoverage != 1 {
		t.Fatalf("items = %d, coverage = %v", res.ItemsSeen, res.FieldCoverage)
	}
	first := res.Signals[0]
	if first.Family != "expansion" || first.URL == "" || first.ObservedAt != "2026-11-10" || first.EntityName == "" {
		t.Fatalf("first signal = %+v", first)
	}
}

func TestRSSAdapterReadsAtomEntriesWithHrefLinks(t *testing.T) {
	w := &fakeWeb{pages: map[string]fakePage{
		feedURL: {body: fixture(t, "producthunt.xml"), ctype: "application/atom+xml"},
	}}
	raw := `{"version":1,"rung":2,"method":"rss","fetch":{"engine":"http","url":"` + feedURL + `","render":"none"},"parse":{"items":"","fields":{"entity":"title","url":"link","observed_at":"published","text":"content"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"launch","entity_from":"field"},"max_items":50}`
	res := Run(context.Background(), mustRecipe(t, raw), w.options())
	if res.Status != "ok" || res.ItemsSeen != 5 {
		t.Fatalf("atom feed: status %s, items %d (%s)", res.Status, res.ItemsSeen, res.Error)
	}
	if !strings.HasPrefix(res.Signals[0].URL, "https://") {
		t.Fatalf("atom link = %q", res.Signals[0].URL)
	}
}

func TestJSONAPIReadsRemotiveJobsAtADotPath(t *testing.T) {
	url := "https://remote.example/api/remote-jobs"
	w := &fakeWeb{pages: map[string]fakePage{url: {body: fixture(t, "remotive_jobs.json"), ctype: "application/json"}}}
	raw := `{"version":1,"rung":3,"method":"json_api","fetch":{"engine":"http","url":"` + url + `","render":"none"},"parse":{"items":"jobs","fields":{"entity":"company_name","url":"url","observed_at":"publication_date","text":"title"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"hiring","entity_from":"field"},"max_items":50}`
	res := Run(context.Background(), mustRecipe(t, raw), w.options())
	if res.Status != "ok" || res.ItemsSeen != 5 {
		t.Fatalf("remotive: status %s, items %d, coverage %.2f (%s)", res.Status, res.ItemsSeen, res.FieldCoverage, res.Error)
	}
}

func TestJSONAPIReadsTopLevelArrayAndSkipsTheLegalNotice(t *testing.T) {
	url := "https://ok.example/api"
	w := &fakeWeb{pages: map[string]fakePage{url: {body: fixture(t, "remoteok_jobs.json"), ctype: "application/json"}}}
	raw := `{"version":1,"rung":3,"method":"json_api","fetch":{"engine":"http","url":"` + url + `","render":"none"},"parse":{"items":"","fields":{"entity":"company","url":"url","observed_at":"date","text":"position"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"hiring","entity_from":"field"},"max_items":50}`
	res := Run(context.Background(), mustRecipe(t, raw), w.options())
	if res.Status != "ok" {
		t.Fatalf("status %s (%s)", res.Status, res.Error)
	}
	if res.ItemsSeen != 5 {
		t.Fatalf("items = %d; the legal notice must not count as an item", res.ItemsSeen)
	}
}

func TestJSONAPIReadsUnixTimesAsDates(t *testing.T) {
	url := "https://ok.example/arbeit"
	w := &fakeWeb{pages: map[string]fakePage{url: {body: fixture(t, "arbeitnow_jobs.json"), ctype: "application/json"}}}
	raw := `{"version":1,"rung":3,"method":"json_api","fetch":{"engine":"http","url":"` + url + `","render":"none"},"parse":{"items":"data","fields":{"entity":"company_name","url":"url","observed_at":"created_at","text":"title"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"hiring","entity_from":"field"},"max_items":50}`
	res := Run(context.Background(), mustRecipe(t, raw), w.options())
	if res.Status != "ok" || res.Signals[0].ObservedAt == "" {
		t.Fatalf("arbeitnow: status %s, first date %q (%s)", res.Status, res.Signals[0].ObservedAt, res.Error)
	}
}

func TestJSONAPIPagesUntilAPageIsEmpty(t *testing.T) {
	base := "https://pages.example/api"
	page1 := `{"items":[{"name":"A Co","url":"https://a.example","date":"2026-11-01","text":"opens a branch"}]}`
	page2 := `{"items":[{"name":"B Co","url":"https://b.example","date":"2026-11-02","text":"opens a branch"}]}`
	w := &fakeWeb{pages: map[string]fakePage{
		base + "?page=1": {body: []byte(page1), ctype: "application/json"},
		base + "?page=2": {body: []byte(page2), ctype: "application/json"},
		base + "?page=3": {body: []byte(`{"items":[]}`), ctype: "application/json"},
	}}
	raw := `{"version":1,"rung":3,"method":"json_api","fetch":{"engine":"http","url":"` + base + `","render":"none","pagination":{"type":"page","max_pages":5,"param":"page"}},"parse":{"items":"items","fields":{"entity":"name","url":"url","observed_at":"date","text":"text"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"expansion","entity_from":"field"},"max_items":50}`
	res := Run(context.Background(), mustRecipe(t, raw), w.options())
	if res.ItemsSeen != 2 {
		t.Fatalf("pages gave %d items, want 2 (stopped at the empty page)", res.ItemsSeen)
	}
}

func TestJSONLDAdapterFindsTheTypedObjectsOfAPage(t *testing.T) {
	url := "https://itviec.example/it-jobs"
	w := &fakeWeb{pages: map[string]fakePage{url: {body: fixture(t, "itviec_list.html")}}}
	raw := `{"version":1,"rung":4,"method":"html_jsonld","fetch":{"engine":"http","url":"` + url + `","render":"none"},"parse":{"items":"ListItem","fields":{"entity":"name","url":"url","observed_at":"datePosted","text":"name"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"hiring","entity_from":"field"},"max_items":50}`
	res := Run(context.Background(), mustRecipe(t, raw), w.options())
	if res.ItemsSeen == 0 {
		t.Fatalf("no ListItem read from the page (%s)", res.Error)
	}
	// ITviec's JSON-LD carries links only, with no name or date: the sample must not pass.
	if res.Status != "degraded" || res.FieldCoverage != 0 {
		t.Fatalf("a list with links only passed the sample: status %s, coverage %.2f", res.Status, res.FieldCoverage)
	}
}

func TestHTMLListAdapterReadsItemsWithSelectors(t *testing.T) {
	page := `<html><body><ul>`
	for i, name := range []string{"Phở Hà", "Máy Đông", "Kế Toán Nam", "Cơ Khí Bắc", "Tư Vấn Trung"} {
		page += `<li class="job"><a class="name" href="/co/` + string(rune('a'+i)) + `">` + name + `</a><span class="date" data-d="2026-11-0` + string(rune('1'+i)) + `">Nov</span><p class="desc">Mở chi nhánh mới</p></li>`
	}
	page += `</ul></body></html>`
	url := "https://directory.example/members"
	w := &fakeWeb{pages: map[string]fakePage{url: {body: []byte(page)}}}
	raw := `{"version":1,"rung":4,"method":"html_list","fetch":{"engine":"http","url":"` + url + `","render":"none"},"parse":{"items":"li.job","fields":{"entity":"a.name","url":"a.name@href","observed_at":"span.date@data-d","text":"p.desc"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"directory","entity_from":"field"},"max_items":50}`
	res := Run(context.Background(), mustRecipe(t, raw), w.options())
	if res.Status != "ok" || res.ItemsSeen != 5 {
		t.Fatalf("html_list: status %s, items %d, coverage %.2f (%s)", res.Status, res.ItemsSeen, res.FieldCoverage, res.Error)
	}
	if res.Signals[0].URL != "https://directory.example/co/a" || res.Signals[0].ObservedAt != "2026-11-01" {
		t.Fatalf("first signal = %+v", res.Signals[0])
	}
}

func TestHTMLListNeedsItsItemSelector(t *testing.T) {
	raw := `{"version":1,"rung":4,"method":"html_list","fetch":{"engine":"http","url":"https://x.example","render":"none"},"parse":{"items":"","fields":{"entity":"a","url":"a@href","observed_at":"time","text":"p"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"directory","entity_from":"field"},"max_items":5}`
	res := Run(context.Background(), mustRecipe(t, raw), (&fakeWeb{}).options())
	if res.Status != "error" || !strings.Contains(res.Error, "parse.items") {
		t.Fatalf("an empty item selector was accepted: %+v", res)
	}
}

func TestCSVAdapterReadsADroppedFileByHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "customers.csv")
	csv := "name,website,date,note\nPhở Hà,pho-ha.example,2026-11-05,long-time customer\n,,,\nMáy Đông,,2026-11-06,\n"
	if err := os.WriteFile(path, []byte(csv), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := `{"version":1,"rung":7,"method":"csv_drop","fetch":{"engine":"http","url":"","render":"none"},"parse":{"items":"","fields":{"entity":"name","url":"website","observed_at":"date","text":"note","website":"website"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"own_data","entity_from":"field"},"max_items":50}`
	r := mustRecipe(t, raw)
	opts := Options{CSVPath: path}
	res := Run(context.Background(), r, opts)
	if res.ItemsSeen != 2 {
		t.Fatalf("csv items = %d, want 2 (the empty row is not an item)", res.ItemsSeen)
	}
	if res.Signals[0].EntityKey != "domain:pho-ha.example" {
		t.Fatalf("the website must give the key: %q", res.Signals[0].EntityKey)
	}
	if got := Run(context.Background(), r, Options{}); got.Status != "error" || !strings.Contains(got.Error, "--csv") {
		t.Fatalf("a csv run without a file must say so: %+v", got)
	}
}
