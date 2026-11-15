package consent

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestFromEnvEmptyIsOff(t *testing.T) {
	c := FromEnv(env(nil))
	if len(c.Documents()) != 0 {
		t.Fatalf("expected feature off, got %+v", c.Documents())
	}
	if got := c.Pending(nil); len(got) != 0 {
		t.Fatalf("expected nothing pending, got %+v", got)
	}
}

func TestFromEnvSkipsDocumentWithoutVersion(t *testing.T) {
	c := FromEnv(env(map[string]string{
		"LEGAL_TERMS_VERSION": " 2025-01 ", "LEGAL_TERMS_URL": " https://x/t ",
		"LEGAL_PRIVACY_URL": "https://x/p",
	}))
	docs := c.Documents()
	if len(docs) != 1 || docs[0] != (Document{Name: "terms", Version: "2025-01", URL: "https://x/t"}) {
		t.Fatalf("got %+v", docs)
	}
}

func TestPendingListsUnacceptedAtCurrentVersion(t *testing.T) {
	c := FromEnv(env(map[string]string{
		"LEGAL_TERMS_VERSION": "2", "LEGAL_TERMS_URL": "https://x/t",
		"LEGAL_PRIVACY_VERSION": "1", "LEGAL_PRIVACY_URL": "https://x/p",
	}))
	accepted := []Acceptance{{Document: "terms", Version: "1"}, {Document: "privacy", Version: "1"}}
	got := c.Pending(accepted)
	if len(got) != 1 || got[0].Document != "terms" || got[0].Version != "2" || got[0].URL != "https://x/t" {
		t.Fatalf("a version change must make terms required again, got %+v", got)
	}
	accepted = append(accepted, Acceptance{Document: "terms", Version: "2"})
	if got := c.Pending(accepted); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestValidateInvalidDocument(t *testing.T) {
	c := FromEnv(env(map[string]string{"LEGAL_TERMS_VERSION": "1"}))
	err := c.Validate([]Item{{Document: "terms", Version: "1"}, {Document: "cookies", Version: "1"}})
	if err != ErrInvalidDocument {
		t.Fatalf("got %v", err)
	}
}

func TestValidateVersionMismatch(t *testing.T) {
	c := FromEnv(env(map[string]string{"LEGAL_TERMS_VERSION": "2"}))
	if err := c.Validate([]Item{{Document: "terms", Version: "1"}}); err != ErrVersionMismatch {
		t.Fatalf("got %v", err)
	}
	// A known document that is not configured has no current version.
	if err := c.Validate([]Item{{Document: "privacy", Version: "1"}}); err != ErrVersionMismatch {
		t.Fatalf("got %v", err)
	}
	if err := c.Validate([]Item{{Document: "terms", Version: "2"}}); err != nil {
		t.Fatalf("got %v", err)
	}
}

func TestValidateInvalidDocumentWinsOverMismatch(t *testing.T) {
	c := FromEnv(env(map[string]string{"LEGAL_TERMS_VERSION": "2"}))
	err := c.Validate([]Item{{Document: "terms", Version: "1"}, {Document: "x", Version: "1"}})
	if err != ErrInvalidDocument {
		t.Fatalf("got %v", err)
	}
}
