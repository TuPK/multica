package salescollect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestBlockedAddressesCoverPrivateLoopbackLinkLocalAndMetadata(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.9", "192.168.1.1", "169.254.169.254",
		"0.0.0.0", "100.64.0.5", "224.0.0.1", "::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1",
	}
	for _, s := range blocked {
		if !blockedAddr(netip.MustParseAddr(s)) {
			t.Errorf("%s was not blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "203.0.113.7", "2606:4700:4700::1111"} {
		if blockedAddr(netip.MustParseAddr(s)) {
			t.Errorf("public address %s was blocked", s)
		}
	}
}

// R11: a request to a loopback address is refused by the default client, even when the host is
// reachable. The test server listens on 127.0.0.1, so the refusal is the guard's, not a failure to connect.
func TestRunRefusesAnInternalURLWithTheDefaultClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write(fixture(t, "techinasia.xml"))
	}))
	defer srv.Close()
	r := mustRecipe(t, `{"version":1,"rung":2,"method":"rss","fetch":{"engine":"http","url":"`+srv.URL+`/feed","render":"none"},"parse":{"items":"","fields":{"entity":"title","url":"link","observed_at":"pubDate","text":"description"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"funding","entity_from":"field"},"max_items":10}`)

	res := Run(context.Background(), r, Options{RateInterval: time.Nanosecond})
	if res.Status != "error" || !strings.Contains(res.Error, "refusing to connect") {
		t.Fatalf("an internal address was read: status=%s error=%q", res.Status, res.Error)
	}
	if res.ItemsSeen != 0 || len(res.Signals) != 0 {
		t.Fatalf("items came back from a refused address: %+v", res)
	}
}

func TestRobotsGroupsAndLongestMatch(t *testing.T) {
	body := `
User-agent: BadBot
Disallow: /

User-agent: *
Disallow: /private
Allow: /private/open

User-agent: MulticaSalesCollect
Disallow: /jobs/closed
`
	rules := parseRobots(body)
	// Our own group applies, so the * group's rules are ignored: /private is open to us.
	cases := map[string]bool{
		"/":              true,
		"/private/x":     true,
		"/jobs/closed/1": false,
		"/jobs/open":     true,
	}
	for path, want := range cases {
		if got := rules.allows(path); got != want {
			t.Errorf("allows(%q) = %v, want %v", path, got, want)
		}
	}
	// With no group for us, the * group applies.
	wild := parseRobots("User-agent: *\nDisallow: /x\n")
	if wild.allows("/x/1") {
		t.Error("the * group was not applied")
	}
	if !parseRobots("User-agent: *\nDisallow: /x\n").allows("/y") {
		t.Error("a path the * group does not name was refused")
	}
}

func TestRobotsDisallowStopsTheFetch(t *testing.T) {
	w := &fakeWeb{pages: map[string]fakePage{
		"https://site.example/robots.txt": {body: []byte("User-agent: *\nDisallow: /feed\n")},
		"https://site.example/feed":       {body: fixture(t, "techinasia.xml"), ctype: "application/rss+xml"},
	}}
	r := mustRecipe(t, `{"version":1,"rung":2,"method":"rss","fetch":{"engine":"http","url":"https://site.example/feed","render":"none"},"parse":{"items":"","fields":{"entity":"title","url":"link","observed_at":"pubDate","text":"description"}},"filter":{"keywords_any":[],"keywords_none":[],"language":"","since_days":0},"map":{"family":"funding","entity_from":"field"},"max_items":10}`)

	res := Run(context.Background(), r, w.options())
	if res.Status != "error" || !strings.Contains(res.Error, "disallows") {
		t.Fatalf("a disallowed path was fetched: %+v", res)
	}
	for _, hit := range w.hits {
		if strings.HasSuffix(hit, "/feed") {
			t.Fatalf("the feed was requested despite robots.txt: %v", w.hits)
		}
	}
}
