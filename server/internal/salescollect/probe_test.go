package salescollect

import (
	"context"
	"testing"
)

func TestDetectPlatformNamesTheNetworksOfTheSpec(t *testing.T) {
	cases := map[string]string{
		"https://www.facebook.com/somepage":     "facebook",
		"https://instagram.com/brand":           "instagram",
		"https://www.linkedin.com/company/acme": "linkedin",
		"https://www.tiktok.com/@brand":         "tiktok",
		"https://youtu.be/abc":                  "youtube",
		"https://zalo.me/12345":                 "zalo",
		"https://x.com/brand":                   "x",
		"https://t.me/s/durov":                  "telegram",
		"https://www.google.com/maps/place/x":   "maps",
		"https://voz.vn/forums/chuyen.17/":      "forum",
		"https://www.vnexpress.net/rss":         "web",
	}
	for in, want := range cases {
		if got := detectPlatform(in); got != want {
			t.Errorf("detectPlatform(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProbeFindsTheFeedLinkOfAPageAndPassesIt(t *testing.T) {
	page := `<html><head><link rel="alternate" type="application/rss+xml" href="/feed.xml"></head><body>news</body></html>`
	w := &fakeWeb{pages: map[string]fakePage{
		"https://news.example/":         {body: []byte(page)},
		"https://news.example/feed.xml": {body: fixture(t, "techinasia.xml"), ctype: "application/rss+xml"},
	}}
	rep := Probe(context.Background(), "https://news.example/", w.options())
	if rep.Verdict != "active" {
		t.Fatalf("verdict = %s (%s); candidates %+v", rep.Verdict, rep.Reason, rep.Candidates)
	}
	var passed *ProbeCandidate
	for i := range rep.Candidates {
		if rep.Candidates[i].Status == "ok" {
			passed = &rep.Candidates[i]
		}
	}
	if passed == nil || passed.Recipe == nil || passed.Recipe.VerifiedAt == "" || passed.Recipe.SampleCount < 5 {
		t.Fatalf("the passing candidate lacks its recipe or sample: %+v", passed)
	}
	if rep.WebEngine.Status != "not_running" {
		t.Fatalf("web engine = %s", rep.WebEngine.Status)
	}
}

func TestProbeReadsAJSONAPIWithGuessedFields(t *testing.T) {
	w := &fakeWeb{pages: map[string]fakePage{
		"https://jobs.example/api": {body: fixture(t, "remotive_jobs.json"), ctype: "application/json"},
	}}
	rep := Probe(context.Background(), "https://jobs.example/api", w.options())
	if rep.Verdict != "active" {
		t.Fatalf("verdict = %s (%s)", rep.Verdict, rep.Reason)
	}
	if rep.Candidates[0].Recipe == nil || rep.Candidates[0].Recipe.Parse.Items != "jobs" {
		t.Fatalf("the guessed items path = %+v", rep.Candidates[0].Recipe)
	}
}

func TestProbeSaysNeedsReviewForAPageWithNoOpenFormat(t *testing.T) {
	w := &fakeWeb{pages: map[string]fakePage{
		"https://dir.example/members": {body: []byte("<html><body><div class='m'>Hội viên</div></body></html>")},
	}}
	rep := Probe(context.Background(), "https://dir.example/members", w.options())
	if rep.Verdict != "needs_review" {
		t.Fatalf("verdict = %s", rep.Verdict)
	}
}

func TestProbeReportsABlockedPage(t *testing.T) {
	w := &fakeWeb{pages: map[string]fakePage{
		"https://blocked.example/": {status: 403, body: []byte("forbidden")},
	}}
	rep := Probe(context.Background(), "https://blocked.example/", w.options())
	if rep.Verdict != "blocked" {
		t.Fatalf("verdict = %s", rep.Verdict)
	}
}

func TestProbeSendsASocialPlatformToItsConnectorWithTheUnlockSentence(t *testing.T) {
	w := &fakeWeb{pages: map[string]fakePage{
		"https://www.facebook.com/somepage": {body: []byte("<html>shell</html>")},
	}}
	rep := Probe(context.Background(), "https://www.facebook.com/somepage", w.options())
	if rep.Verdict != "needs_connector" || rep.Reason == "" {
		t.Fatalf("social verdict = %+v", rep)
	}
	for _, c := range rep.Candidates {
		if c.Status == "ok" {
			t.Fatal("a social page passed the sample without a connection")
		}
	}
}

func TestProbeNeverUsesAProviderOrALogin(t *testing.T) {
	w := &fakeWeb{pages: map[string]fakePage{"https://news.example/": {body: []byte("<html></html>")}}}
	_ = Probe(context.Background(), "https://news.example/", w.options())
	for _, hit := range w.hits {
		if hit != "https://news.example/" && hit != "https://news.example/robots.txt" {
			t.Fatalf("probe requested %s", hit)
		}
	}
}
