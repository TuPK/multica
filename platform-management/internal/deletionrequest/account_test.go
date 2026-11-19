package deletionrequest

import (
	"errors"
	"testing"
)

// Rules of spec R6 section 7.3 that need no database.

func sampleRequired() []AccountWorkspace {
	return []AccountWorkspace{
		{ID: "ws-a", Slug: "alpha", Role: "owner", OwnerCount: 1, MemberCount: 3},
		{ID: "ws-b", Slug: "beta", Role: "owner", OwnerCount: 1, MemberCount: 2},
	}
}

func okSuccessor(_, userID string) bool { return userID == "good-user" }

func asError(t *testing.T, err error) *Error {
	t.Helper()
	var de *Error
	if !errors.As(err, &de) {
		t.Fatalf("error = %v, want *Error", err)
	}
	return de
}

func TestNeedsChoiceOnlyForSoleOwnerWithOtherMembers(t *testing.T) {
	cases := []struct {
		name string
		w    AccountWorkspace
		want bool
	}{
		{"sole owner with others", AccountWorkspace{Role: "owner", OwnerCount: 1, MemberCount: 2}, true},
		{"co-owner needs no choice", AccountWorkspace{Role: "owner", OwnerCount: 2, MemberCount: 3}, false},
		{"plain member needs no choice", AccountWorkspace{Role: "member", OwnerCount: 1, MemberCount: 3}, false},
		{"alone in the workspace is deleted with the account", AccountWorkspace{Role: "owner", OwnerCount: 1, MemberCount: 1}, false},
	}
	for _, c := range cases {
		if got := c.w.NeedChoice(); got != c.want {
			t.Errorf("%s: NeedsChoice = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestResolutionValidation(t *testing.T) {
	cases := []struct {
		name    string
		res     []Resolution
		status  int
		code    string
		wantWS  string
		noError bool
	}{
		{name: "transfer and delete cover both", noError: true, res: []Resolution{
			{WorkspaceID: "ws-a", Action: ActionTransfer, SuccessorUserID: "good-user"},
			{WorkspaceID: "ws-b", Action: ActionDelete},
		}},
		{name: "missing choice is 409 with the list", status: 409, code: CodeSoleOwnerUnresolved, res: []Resolution{
			{WorkspaceID: "ws-a", Action: ActionTransfer, SuccessorUserID: "good-user"},
		}},
		{name: "unknown workspace is 400 successor_invalid", status: 400, code: CodeSuccessorInvalid, wantWS: "ws-x", res: []Resolution{
			{WorkspaceID: "ws-x", Action: ActionDelete},
			{WorkspaceID: "ws-a", Action: ActionDelete},
			{WorkspaceID: "ws-b", Action: ActionDelete},
		}},
		{name: "duplicate choice is 400", status: 400, code: CodeSuccessorInvalid, wantWS: "ws-a", res: []Resolution{
			{WorkspaceID: "ws-a", Action: ActionDelete},
			{WorkspaceID: "ws-a", Action: ActionDelete},
			{WorkspaceID: "ws-b", Action: ActionDelete},
		}},
		{name: "successor that fails GĐ11 is 400 with the workspace", status: 400, code: CodeSuccessorInvalid, wantWS: "ws-a", res: []Resolution{
			{WorkspaceID: "ws-a", Action: ActionTransfer, SuccessorUserID: "viewer-user"},
			{WorkspaceID: "ws-b", Action: ActionDelete},
		}},
		{name: "transfer without successor is 400", status: 400, code: CodeSuccessorInvalid, wantWS: "ws-a", res: []Resolution{
			{WorkspaceID: "ws-a", Action: ActionTransfer},
			{WorkspaceID: "ws-b", Action: ActionDelete},
		}},
		{name: "unknown action is 400 resolution_invalid", status: 400, code: CodeResolutionInvalid, wantWS: "ws-a", res: []Resolution{
			{WorkspaceID: "ws-a", Action: "archive"},
			{WorkspaceID: "ws-b", Action: ActionDelete},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkResolutions(sampleRequired(), c.res, okSuccessor)
			if c.noError {
				if err != nil {
					t.Fatalf("checkResolutions: %v", err)
				}
				return
			}
			de := asError(t, err)
			if de.Status != c.status || de.Code != c.code {
				t.Fatalf("got %d %s, want %d %s", de.Status, de.Code, c.status, c.code)
			}
			if c.wantWS != "" && de.Extra["workspace_id"] != c.wantWS {
				t.Fatalf("workspace_id = %v, want %s", de.Extra["workspace_id"], c.wantWS)
			}
			if c.code == CodeSoleOwnerUnresolved {
				ids, _ := de.Extra["workspace_ids"].([]string)
				if len(ids) != 1 || ids[0] != "ws-b" {
					t.Fatalf("workspace_ids = %v, want [ws-b]", de.Extra["workspace_ids"])
				}
			}
		})
	}
}

func TestEmailConfirmIgnoresCase(t *testing.T) {
	if !EmailMatches("  Alice@Example.COM ", "alice@example.com") {
		t.Fatal("email confirmation must ignore case and surrounding spaces (spec GĐ10)")
	}
	if EmailMatches("alice@example.org", "alice@example.com") {
		t.Fatal("different email must not match")
	}
}
