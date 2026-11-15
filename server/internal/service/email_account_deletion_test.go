package service

import (
	"strings"
	"testing"
)

func TestAccountDeletionScheduledEmail(t *testing.T) {
	p := buildAccountDeletionScheduledParams("from@x.test", "u@x.test", "2030-01-15T10:00:00Z", "https://app.test")
	if p.To[0] != "u@x.test" || p.From != "from@x.test" {
		t.Fatalf("addresses: %+v", p)
	}
	for _, want := range []string{"2030-01-15T10:00:00Z", "Cancel deletion", "https://app.test"} {
		if !strings.Contains(p.Html, want) {
			t.Errorf("body lacks %q", want)
		}
	}
	for _, banned := range []string{"token", "code=", "?key="} {
		if strings.Contains(strings.ToLower(p.Html), banned) {
			t.Errorf("body must not carry %q", banned)
		}
	}
}

func TestAccountDeletionScheduledEmailEscapesInput(t *testing.T) {
	p := buildAccountDeletionScheduledParams("f@x.test", "u@x.test", "<script>x</script>", "https://a.test")
	if strings.Contains(p.Html, "<script>") {
		t.Fatal("scheduledAt was not escaped")
	}
}
