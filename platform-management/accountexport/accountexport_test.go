package accountexport

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRetryAfterSeconds(t *testing.T) {
	cases := map[time.Duration]int{
		0:                       1,
		-time.Second:            1,
		time.Millisecond:        1,
		time.Second:             1,
		1500 * time.Millisecond: 2,
		time.Hour:               3600,
	}
	for in, want := range cases {
		if got := RetryAfterSeconds(in); got != want {
			t.Errorf("RetryAfterSeconds(%v) = %d, want %d", in, got, want)
		}
	}
}

func TestLimitIsOnePerHour(t *testing.T) {
	if Limit != 1 || Window != time.Hour {
		t.Fatalf("Limit=%d Window=%v", Limit, Window)
	}
}

func TestFindSecretsCleanExport(t *testing.T) {
	v := decode(t, `{"exported_at":"2025-01-01T00:00:00Z",
	 "personal_access_tokens":[{"id":"a","name":"ci","token_prefix":"mul_ab"}],
	 "security_events":[{"detail":{"method":"email_code","token":"[redacted]"}}]}`)
	if got := FindSecrets(v, nil); len(got) != 0 {
		t.Fatalf("unexpected findings: %v", got)
	}
}

func TestFindSecretsForbiddenKeysAtAnyDepth(t *testing.T) {
	v := decode(t, `{"personal_access_tokens":[{"token_hash":"abc"}],
	 "security_events":[{"detail":{"nested":{"Refresh_Token":"x"}}}],
	 "external_connections":[{"credentials":null}]}`)
	got := FindSecrets(v, nil)
	if len(got) != 3 {
		t.Fatalf("findings = %v, want 3", got)
	}
}

func TestFindSecretsForbiddenLiterals(t *testing.T) {
	v := decode(t, `{"security_events":[{"detail":{"note":"paste mul_supersecret here"}}]}`)
	got := FindSecrets(v, []string{"mul_supersecret"})
	if len(got) != 1 || !strings.Contains(got[0], "note") {
		t.Fatalf("findings = %v", got)
	}
	if got := FindSecrets(v, []string{"other_value_x"}); len(got) != 0 {
		t.Fatalf("unexpected: %v", got)
	}
	// Literals shorter than the minimum are ignored: they would match plain text.
	if got := FindSecrets(v, []string{"mul"}); len(got) != 0 {
		t.Fatalf("short literal matched: %v", got)
	}
}
