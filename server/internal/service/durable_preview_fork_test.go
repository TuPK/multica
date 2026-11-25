package service

import (
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/connectedapps"
)

// TestBuildPreviewKinds: each kind of call shows what it will do: an email names its
// recipients and subject, a post shows its text, a record shows its fields, and any
// other call lists its arguments (durable spec 5.2).
func TestBuildPreviewKinds(t *testing.T) {
	cases := []struct {
		name     string
		toolkit  string
		tool     string
		args     map[string]any
		wantKind string
	}{
		{
			name: "email to one recipient", toolkit: "gmail", tool: "GMAIL_SEND_EMAIL", wantKind: "email",
			args: map[string]any{"to": "a@example.com", "subject": "Hi", "body": "Text"},
		},
		{
			name: "post", toolkit: "x", tool: "X_CREATE_POST", wantKind: "post",
			args: map[string]any{"text": "Launch day"},
		},
		{
			name: "record update", toolkit: "hubspot", tool: "HUBSPOT_UPDATE_CONTACT", wantKind: "record",
			args: map[string]any{"contact_id": "c-1", "properties": map[string]any{"lifecycle": "customer"}},
		},
		{
			name: "unknown tool", toolkit: "acme", tool: "ACME_DO_THING", wantKind: "generic",
			args: map[string]any{"amount": 12, "note": "x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildPreview(tc.toolkit, tc.tool, tc.args)
			if got.Kind != tc.wantKind {
				t.Fatalf("kind = %q, want %q (%+v)", got.Kind, tc.wantKind, got)
			}
		})
	}
}

// TestBuildPreviewEmailNamesRecipientsAndSubject: the email preview shows the people
// the mail reaches and its subject, which is what the person approves.
func TestBuildPreviewEmailNamesRecipientsAndSubject(t *testing.T) {
	p := BuildPreview("gmail", "GMAIL_SEND_EMAIL", map[string]any{
		"to":      []any{"a@example.com", "b@example.com"},
		"subject": "Quote",
		"body":    "Here it is.",
	})
	if len(p.Recipients) != 2 || p.Recipients[1] != "b@example.com" {
		t.Fatalf("recipients = %v", p.Recipients)
	}
	if p.Title != "Quote" || p.BodyMarkdown != "Here it is." {
		t.Fatalf("title %q body %q", p.Title, p.BodyMarkdown)
	}
}

// TestPreviewNeverShowsPlatformParameters: a key with an underscore is the platform's
// own, the rationale among them, and no preview carries it.
func TestPreviewNeverShowsPlatformParameters(t *testing.T) {
	args := map[string]any{"to": "a@example.com", "subject": "S", "body": "B", "_rationale": "because the boss said so"}
	for _, p := range []Preview{
		BuildPreview("gmail", "GMAIL_SEND_EMAIL", args),
		BuildPreview("acme", "ACME_DO_THING", args),
	} {
		raw := strings.Join(previewStrings(p), " ")
		if strings.Contains(raw, "because the boss said so") || strings.Contains(raw, "_rationale") {
			t.Fatalf("preview shows the rationale: %s", raw)
		}
	}
}

func previewStrings(p Preview) []string {
	out := []string{p.Title, p.BodyMarkdown}
	for _, f := range p.Fields {
		out = append(out, f.Label, f.Before, f.After)
	}
	return append(out, p.Recipients...)
}

// TestPreviewTruncatesLongValues: a value longer than the preview's limit is cut.
func TestPreviewTruncatesLongValues(t *testing.T) {
	long := strings.Repeat("x", previewValueMax+50)
	p := BuildPreview("acme", "ACME_DO_THING", map[string]any{"blob": long})
	if len([]rune(p.Fields[0].After)) > previewValueMax+1 {
		t.Fatalf("value not truncated: %d runes", len([]rune(p.Fields[0].After)))
	}
}

// TestCallSHA256Stable: the same call hashes the same whatever order its keys were
// written in, and a changed argument changes the hash.
func TestCallSHA256Stable(t *testing.T) {
	a := connectedapps.CanonicalCall{Tool: "T", AccountID: "ca_1", Args: map[string]any{"x": 1, "y": "two"}}
	b := connectedapps.CanonicalCall{Tool: "T", AccountID: "ca_1", Args: map[string]any{"y": "two", "x": 1}}
	c := connectedapps.CanonicalCall{Tool: "T", AccountID: "ca_1", Args: map[string]any{"x": 2, "y": "two"}}
	if CallSHA256(a) != CallSHA256(b) {
		t.Fatal("the same call hashes differently")
	}
	if CallSHA256(a) == CallSHA256(c) {
		t.Fatal("a changed argument did not change the hash")
	}
	if len(CallSHA256(a)) != 64 {
		t.Fatalf("hash length %d, want 64 hex digits", len(CallSHA256(a)))
	}
}
