package handler

// The chat paths act for the member the sender is bound to, so they see the
// proposals that member sees (spec 7.1 rule 5), no more and no fewer.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/service"
)

func TestChannelDecisionsListShowsProposalsInVisibleProjects(t *testing.T) {
	chatCommandAppURL(t)
	f := newRestrictedFixture(t)

	// Each proposal comes from its own squad, as a squad may hold three pending.
	// The backing issue is filed without a project; moving it places it.
	file := func(name, projectID string) (title, proposalID string) {
		fx := newProposalTestFixture(t)
		p := fx.fileProposal(t, name, nil)
		if projectID != "" {
			dbfx.Exec(t, `UPDATE issue SET project_id = $2 WHERE id = $1`, p.IssueID, projectID)
		}
		time.Sleep(2 * time.Millisecond)
		return fx.title(name), p.ID
	}
	restrictedTitle, restrictedID := file("scope restricted", f.restrictedProjectID)
	workspaceTitle, workspaceID := file("scope workspace project", f.workspaceProjectID)
	looseTitle, looseID := file("scope no project", "")

	decisions := func(userID string) string {
		installs := uuid.NewString()
		sender := bindChatSender(t, installs, testWorkspaceID, userID)
		msg, handled := chatCommand(t, installs, sender, channel.CommandDecisions)
		if !handled {
			t.Fatal("/decisions was not answered for a member")
		}
		return msg.Text
	}

	granted := decisions(f.granteeID)
	for name, want := range map[string]struct{ title, id string }{
		"restricted project": {restrictedTitle, restrictedID},
		"workspace project":  {workspaceTitle, workspaceID},
		"no project":         {looseTitle, looseID},
	} {
		if !strings.Contains(granted, want.title) || !strings.Contains(granted, "decision="+want.id) {
			t.Errorf("/decisions for the granted member omits the %s proposal:\n%s", name, granted)
		}
	}

	ungranted := decisions(f.outsiderID)
	if strings.Contains(ungranted, restrictedTitle) || strings.Contains(ungranted, "decision="+restrictedID) {
		t.Errorf("/decisions for a member without the grant names the restricted proposal:\n%s", ungranted)
	}
	for name, want := range map[string]struct{ title, id string }{
		"workspace project": {workspaceTitle, workspaceID},
		"no project":        {looseTitle, looseID},
	} {
		if !strings.Contains(ungranted, want.title) || !strings.Contains(ungranted, "decision="+want.id) {
			t.Errorf("/decisions for a member without the grant omits the %s proposal:\n%s", name, ungranted)
		}
	}
}

func TestChannelSnoozeOfHiddenProposalAnswersMissing(t *testing.T) {
	env := newChatActionEnv(t)
	f := newRestrictedFixture(t)
	dbfx.Exec(t, `UPDATE issue SET project_id = $2 WHERE id = $1`, env.Proposal.IssueID, f.restrictedProjectID)
	raw := env.token(t, env.Proposal.ID, service.ProposalDecisionSnooze)
	signed, err := env.Signer.Verify(raw)
	if err != nil {
		t.Fatal(err)
	}

	snooze := func(userID string) channel.PressResult {
		sender := bindChatSender(t, env.Installs, env.WorkspaceID, userID)
		binding, ok, err := env.Actions.binding(context.Background(), channel.Sender{
			ChannelType: "telegram", InstallationID: chatUUID(t, env.Installs), ChannelUserID: sender,
		})
		if err != nil || !ok {
			t.Fatalf("binding: ok=%v err=%v", ok, err)
		}
		return env.Actions.apply(context.Background(), binding, signed)
	}

	// A proposal of a project the member cannot see answers like a missing one,
	// and nothing changes.
	res := snooze(f.outsiderID)
	if res.Toast != chatToastNotFound || !strings.Contains(res.Reply, chatMsgNotFound) {
		t.Fatalf("snooze by a member without the grant = %+v, want the not-found answer", res)
	}
	if until := proposalSnoozedUntil(t, env.Proposal.ID); until != nil {
		t.Fatalf("a hidden proposal was snoozed until %v", *until)
	}

	res = snooze(f.granteeID)
	if !strings.Contains(res.Reply, "Snoozed until") {
		t.Fatalf("snooze by the granted member = %+v, want snoozed", res)
	}
	if until := proposalSnoozedUntil(t, env.Proposal.ID); until == nil {
		t.Fatal("the granted member's snooze did not set snoozed_until")
	}
}
