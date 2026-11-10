package salescollect

import (
	"context"
	"testing"
	"time"
)

func TestRunDropsDuplicatesAndCapsAtTheRecipeLimit(t *testing.T) {
	feed := []byte(`<rss><channel>
<item><title>Phở Hà</title><link>https://a.example/1</link><pubDate>Tue, 10 Nov 2026 04:00:00 GMT</pubDate><description>opens</description></item>
<item><title>Phở Hà</title><link>https://a.example/1</link><pubDate>Tue, 10 Nov 2026 04:00:00 GMT</pubDate><description>opens again</description></item>
<item><title>Máy Đông</title><link>https://b.example/2</link><pubDate>Tue, 10 Nov 2026 04:00:00 GMT</pubDate><description>expands</description></item>
<item><title>Kế Toán</title><link>https://c.example/3</link><pubDate>Tue, 10 Nov 2026 04:00:00 GMT</pubDate><description>launches</description></item>
<item><title>Cơ Khí</title><link>https://d.example/4</link><pubDate>Tue, 10 Nov 2026 04:00:00 GMT</pubDate><description>opens</description></item>
</channel></rss>`)
	w := &fakeWeb{pages: map[string]fakePage{feedURL: {body: feed, ctype: "application/rss+xml"}}}
	raw := `{"version":1,"rung":2,"method":"rss","fetch":{"engine":"http","url":"` + feedURL + `","render":"none"},"parse":{"items":"","fields":{"entity":"title","url":"link","observed_at":"pubDate","text":"description"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"expansion","entity_from":"field"},"max_items":3}`
	res := Run(context.Background(), mustRecipe(t, raw), w.options())
	if res.ItemsSeen != 5 {
		t.Fatalf("items seen = %d", res.ItemsSeen)
	}
	if len(res.Signals) != 3 {
		t.Fatalf("signals = %d, want the recipe cap of 3 after dropping the duplicate", len(res.Signals))
	}
	// Five items, all with their four fields, meet the sample bar.
	if res.Status != "ok" || res.FieldCoverage != 1 {
		t.Fatalf("status %s, coverage %.2f: five complete items meet the bar", res.Status, res.FieldCoverage)
	}
}

func TestRunKeywordAndWindowFilters(t *testing.T) {
	pinNow(t, time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC))
	feed := []byte(`<rss><channel>
<item><title>Khai trương chi nhánh</title><link>https://a.example/1</link><pubDate>Mon, 09 Nov 2026 04:00:00 GMT</pubDate><description>mở mới</description></item>
<item><title>Giá cà phê tăng</title><link>https://b.example/2</link><pubDate>Mon, 09 Nov 2026 04:00:00 GMT</pubDate><description>thị trường</description></item>
<item><title>Khai trương cũ</title><link>https://c.example/3</link><pubDate>Mon, 01 Jan 2024 04:00:00 GMT</pubDate><description>cũ</description></item>
<item><title>Khai trương quảng cáo</title><link>https://d.example/4</link><pubDate>Mon, 09 Nov 2026 04:00:00 GMT</pubDate><description>quảng cáo</description></item>
<item><title>Khai trương nữa</title><link>https://e.example/5</link><pubDate>Mon, 09 Nov 2026 04:00:00 GMT</pubDate><description>nữa</description></item>
</channel></rss>`)
	w := &fakeWeb{pages: map[string]fakePage{feedURL: {body: feed, ctype: "application/rss+xml"}}}
	raw := `{"version":1,"rung":2,"method":"rss","fetch":{"engine":"http","url":"` + feedURL + `","render":"none"},"parse":{"items":"","fields":{"entity":"title","url":"link","observed_at":"pubDate","text":"description"}},"filter":{"keywords_any":["khai trương"],"keywords_none":["quảng cáo"],"language":"vi","since_days":30},"map":{"family":"expansion","entity_from":"field"},"max_items":50}`
	res := Run(context.Background(), mustRecipe(t, raw), w.options())
	if len(res.Signals) != 2 {
		t.Fatalf("kept %d signals, want 2: the two recent openings", len(res.Signals))
	}
	for _, s := range res.Signals {
		if s.ObservedAt != "2026-11-09" {
			t.Errorf("an old item was kept: %+v", s)
		}
	}
}

func TestRunReportsAnEmptyPageAsDegradedNotOk(t *testing.T) {
	w := &fakeWeb{pages: map[string]fakePage{feedURL: {body: []byte(`<rss><channel></channel></rss>`), ctype: "application/rss+xml"}}}
	res := Run(context.Background(), mustRecipe(t, rssRecipe(feedURL, "expansion")), w.options())
	if res.Status != "degraded" || res.ItemsSeen != 0 {
		t.Fatalf("an empty feed = %+v", res)
	}
}

func TestRunReportsAFailedFetchAsError(t *testing.T) {
	w := &fakeWeb{pages: map[string]fakePage{feedURL: {status: 503, body: []byte("busy")}}}
	res := Run(context.Background(), mustRecipe(t, rssRecipe(feedURL, "expansion")), w.options())
	if res.Status != "error" || res.HTTPStatus != 503 {
		t.Fatalf("a 503 = %+v", res)
	}
}

func TestRunStopsAtTheContextDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := &fakeWeb{pages: map[string]fakePage{feedURL: {body: fixture(t, "gnews_expansion.xml"), ctype: "application/rss+xml"}}}
	res := Run(ctx, mustRecipe(t, rssRecipe(feedURL, "expansion")), w.options())
	if res.Status != "error" {
		t.Fatalf("a cancelled run reported %s", res.Status)
	}
}
