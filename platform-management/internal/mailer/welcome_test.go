package mailer

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

const (
	welcomeWorkspace = "Acme <Labs> & Co"
	welcomePlan      = "Business"
	welcomeAppURL    = "https://app.example.com"
)

func sentWelcome(t *testing.T) (html, subject string) {
	t.Helper()
	h := &fakeHTTP{}
	logger, _ := capture()
	m, err := New(resendConfig(), h, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SendWelcome(context.Background(), to, welcomeWorkspace, welcomePlan, welcomeAppURL); err != nil {
		t.Fatalf("SendWelcome: %v", err)
	}
	if h.req == nil {
		t.Fatal("SendWelcome sent nothing")
	}
	var body struct {
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		HTML    string   `json:"html"`
	}
	if err := json.Unmarshal(h.body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if len(body.To) != 1 || body.To[0] != to {
		t.Errorf("to = %v", body.To)
	}
	return body.HTML, body.Subject
}

// Spec 7b.5, rule 9: the workspace, the plan, the web app address and how to
// ask for a sign-in code; nothing that signs the person in.
func TestWelcomeTemplateHasNoSecrets(t *testing.T) {
	html, subject := sentWelcome(t)
	if subject == "" {
		t.Error("the welcome has no subject")
	}
	// The workspace name is the only free text and is escaped.
	for _, want := range []string{"Acme &lt;Labs&gt; &amp; Co", welcomePlan, welcomeAppURL} {
		if !strings.Contains(html, want) {
			t.Errorf("the welcome lacks %q: %s", want, html)
		}
	}
	if strings.Contains(html, "<Labs>") {
		t.Error("the workspace name is not escaped")
	}
	lower := strings.ToLower(html)
	for _, banned := range []string{"password", "token", "?", "verification"} {
		if strings.Contains(lower, banned) {
			t.Errorf("the welcome contains %q: %s", banned, html)
		}
	}
	if regexp.MustCompile(`\d{4,}`).MatchString(html) {
		t.Errorf("the welcome holds a run of digits that could be a code: %s", html)
	}
	links := regexp.MustCompile(`https?://[^"'\s<)]+`).FindAllString(html, -1)
	if len(links) == 0 {
		t.Fatal("the welcome has no link")
	}
	for _, l := range links {
		if l != welcomeAppURL {
			t.Errorf("link %q is not PLATFORM_APP_URL", l)
		}
	}
	if !strings.Contains(lower, "sign-in code") {
		t.Error("the welcome does not say how to get a sign-in code")
	}
}

func TestWelcomeRefusesABadAddressOrAppURL(t *testing.T) {
	logger, _ := capture()
	for name, c := range map[string]struct{ to, app string }{
		"display name": {"Ana <" + to + ">", welcomeAppURL},
		"empty app":    {to, ""},
		"not http":     {to, "javascript:alert(1)"},
		"no host":      {to, "https://"},
	} {
		t.Run(name, func(t *testing.T) {
			h := &fakeHTTP{}
			m, _ := New(resendConfig(), h, nil, logger)
			if err := m.SendWelcome(context.Background(), c.to, "W", "P", c.app); err == nil {
				t.Error("SendWelcome succeeded")
			}
			if h.req != nil {
				t.Error("a message went out")
			}
		})
	}
}
