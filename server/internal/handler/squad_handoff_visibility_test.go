package handler

// The notes a squad handoff leaves on the source request show the target
// request's title only when every reader of the source can see the target
// (spec 7.1 rule 4). Otherwise they carry the wording a missing issue gets.

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

const missingIssueText = "issue not found"

type handoffNotes struct {
	source, target, targetTitle string
	handoffNote                 string
}

// handOff makes the handoff of a fresh request, with the build project's access
// set to access, and returns the target and the note it left on the source.
func (fx handoffFx) handOff(t *testing.T, access string) handoffNotes {
	t.Helper()
	dbfx.Exec(t, `UPDATE project SET visibility = $1 WHERE id = $2`, visibilityOf(access), fx.Project)
	if access == "restricted" {
		// The owner-side reader is a member of the private project; a role grants nothing.
		dbfx.Exec(t, `INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, fx.Project, testWorkspaceID, fx.AdminID)
	}
	fx.setRule(t, "auto").Want(http.StatusOK)
	source := fx.request(t, "Add feature X")
	fx.packet(t, source)
	var out handoffOut
	fx.leadAsks(t, source).Want(http.StatusOK).JSON(&out)
	// The response omits a target the caller cannot see, so read it back.
	n := handoffNotes{source: source}
	dbfx.QueryRow(t, `SELECT target_issue_id::text FROM squad_handoff WHERE source_issue_id = $1`, source).Scan(&n.target)
	dbfx.QueryRow(t, `SELECT title FROM issue WHERE id = $1`, n.target).Scan(&n.targetTitle)
	dbfx.QueryRow(t, `SELECT content FROM comment WHERE issue_id = $1 AND content LIKE 'Handed off to %'`, source).Scan(&n.handoffNote)
	return n
}

// closingNote finishes the target and returns the note CloseLoop left on the source.
func (fx handoffFx) closingNote(t *testing.T, n handoffNotes) string {
	t.Helper()
	dbfx.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, n.target)
	row, err := testHandler.Queries.GetIssue(context.Background(), parseUUID(n.target))
	if err != nil {
		t.Fatalf("load target: %v", err)
	}
	if err := testHandler.SquadHandoffService().CloseLoop(context.Background(), row); err != nil {
		t.Fatalf("CloseLoop: %v", err)
	}
	var content string
	dbfx.QueryRow(t, `SELECT content FROM comment WHERE issue_id = $1 AND type = 'system' ORDER BY created_at DESC LIMIT 1`, n.source).Scan(&content)
	return content
}

func assertNoTarget(t *testing.T, what, content string, n handoffNotes) {
	t.Helper()
	if strings.Contains(content, n.targetTitle) || strings.Contains(content, n.target) || strings.Contains(content, "mention://issue/") {
		t.Errorf("%s names the hidden target: %q", what, content)
	}
	if !strings.Contains(content, missingIssueText) {
		t.Errorf("%s = %q, want the wording of a missing issue (%q)", what, content, missingIssueText)
	}
}

// TestCloseLoopHidesRestrictedTargetTitle: the target sits in a restricted
// project the source's readers are not granted.
func TestCloseLoopHidesRestrictedTargetTitle(t *testing.T) {
	fx := newHandoffFx(t)
	n := fx.handOff(t, "restricted")
	assertNoTarget(t, "the closing note", fx.closingNote(t, n), n)
}

// TestHandoffNoteHidesRestrictedTargetTitle: the note left at hand-off time.
func TestHandoffNoteHidesRestrictedTargetTitle(t *testing.T) {
	fx := newHandoffFx(t)
	n := fx.handOff(t, "restricted")
	if n.handoffNote == "" {
		t.Fatal("no handoff note was left on the source")
	}
	assertNoTarget(t, "the handoff note", n.handoffNote, n)
}

// TestCloseLoopKeepsTitleForWorkspaceTarget: a target everyone can see keeps its
// title and link in both notes.
func TestCloseLoopKeepsTitleForWorkspaceTarget(t *testing.T) {
	fx := newHandoffFx(t)
	n := fx.handOff(t, "workspace")
	for what, content := range map[string]string{"handoff note": n.handoffNote, "closing note": fx.closingNote(t, n)} {
		if !strings.Contains(content, n.targetTitle) || !strings.Contains(content, "mention://issue/"+n.target) {
			t.Errorf("the %s lost the visible target's title or link: %q", what, content)
		}
	}
}
