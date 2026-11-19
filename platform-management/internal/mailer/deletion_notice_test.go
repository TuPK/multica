package mailer

import (
	"strings"
	"testing"
	"time"
)

// Spec R6 section 3c.9 rule 4: the notice names the workspace, escapes it, and
// mentions the export only when one was requested.
func TestDeletionNoticeEscapesNameAndMentionsExportOnlyWhenAsked(t *testing.T) {
	at := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)
	body := deletionNoticeBody("<Acme & co>", at, false)
	if !strings.Contains(body, "&lt;Acme &amp; co&gt;") || strings.Contains(body, "<Acme") {
		t.Fatalf("the workspace name is not escaped: %s", body)
	}
	if !strings.Contains(body, "2026-11-22 09:00") {
		t.Fatalf("the start date is missing: %s", body)
	}
	if strings.Contains(body, "export of the workspace data") {
		t.Fatal("no export was requested, yet the notice mentions one")
	}
	if !strings.Contains(deletionNoticeBody("Acme", at, true), "export of the workspace data") {
		t.Fatal("an export was requested, yet the notice does not mention it")
	}
}
