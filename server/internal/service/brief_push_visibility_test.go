package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// A brief push names pending decisions with the recipient's project scope: a
// proposal whose issue sits in a restricted project the member is not granted is
// left out (spec 7.1 rule 5). The owner's push still names it.
func TestBriefPushVisibility_HiddenDecisionIsNotNamed(t *testing.T) {
	f := newBriefPushFixture(t)
	hiddenProj := f.fx.Project(t, "Hidden brief project", testutil.Cols{"visibility": "private"})
	f.fx.Exec(t, `INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, hiddenProj, f.fx.WorkspaceID, f.owner)

	proposalIn := func(title, project string) string {
		cols := testutil.Cols{}
		if project != "" {
			cols["project_id"] = project
		}
		issue := f.fx.Issue(t, title, cols)
		return f.fx.Insert(t, "proposal", testutil.Cols{
			"workspace_id": f.ws, "squad_id": uuid.NewString(), "issue_id": issue,
			"created_by_type": "member", "created_by_id": f.owner, "state": "pending",
		})
	}
	hidden := proposalIn("HIDDEN-DECISION-TITLE", hiddenProj)
	open := proposalIn("OPEN-DECISION-TITLE", "")

	member, memberBinding := f.member(nil)
	f.preference(member, true, memberBinding, nil, nil)
	ownerBinding := f.binding(f.owner, fmt.Sprintf("%d", 9000+f.n))
	f.preference(f.owner, true, ownerBinding, nil, nil)

	briefID := f.brief("Two decisions.", hidden, open)
	f.enqueue(briefID)
	f.deliverDue()

	var memberText, ownerText string
	for _, s := range f.notifier.sent {
		text := s.msg.Text
		for _, b := range s.msg.Buttons {
			text += " " + b.Label + " " + b.CallbackData + " " + b.LinkURL
		}
		switch uuidString(s.binding.MulticaUserID) {
		case member:
			memberText = text
		case f.owner:
			ownerText = text
		}
	}
	if memberText == "" || ownerText == "" {
		t.Fatalf("pushes: member %q owner %q; want both sent", memberText, ownerText)
	}
	if strings.Contains(memberText, "HIDDEN-DECISION-TITLE") || strings.Contains(memberText, hidden) {
		t.Errorf("member push names the hidden decision: %s", memberText)
	}
	if !strings.Contains(memberText, "OPEN-DECISION-TITLE") || !strings.Contains(memberText, "1 decision needs you") {
		t.Errorf("member push should name only the open decision: %s", memberText)
	}
	if !strings.Contains(ownerText, "HIDDEN-DECISION-TITLE") || !strings.Contains(ownerText, "2 decisions need you") {
		t.Errorf("owner push should name both decisions: %s", ownerText)
	}
}
