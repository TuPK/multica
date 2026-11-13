package handler

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The lead may write the stop comment itself before it reports the stop. The server still sets the
// status, and it posts its own comment only when the lead has not written one for that stop.

// stopCommentCount counts the comments on the request whose text starts with header.
func stopCommentCount(t *testing.T, request db.Issue, header string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1 AND content LIKE $2`, request.ID, header+"%")
}

// systemStopCommentCount counts the comments the server posted with that header.
func systemStopCommentCount(t *testing.T, request db.Issue, header string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1 AND author_type = 'system' AND content LIKE $2`, request.ID, header+"%")
}

// planBlockedByLead is the lead's report that grounds are missing, as `multica issue flow plan --blocked` sends it.
func planBlockedByLead(t *testing.T, request db.Issue) {
	t.Helper()
	_, err := testHandler.PlanFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID)), FlowPlanInput{
		Blocked: &FlowPlanBlocked{Missing: []string{"hóa đơn thương mại"}, Reason: "không có hóa đơn gốc"},
	})
	if err != nil {
		t.Fatalf("PlanFlow blocked: %v", err)
	}
}

func TestStopComment_LeadStop1IsNotPostedTwice(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	dbfx.Comment(t, uuidToString(request.ID), "BLOCKED · stop 1 · thiếu căn cứ\n\nThiếu hóa đơn gốc.",
		testutil.Cols{"author_type": "agent", "author_id": f.Agents.Docs})

	planBlockedByLead(t, request)

	if got := stopCommentCount(t, request, "BLOCKED · stop 1"); got != 1 {
		t.Fatalf("%d stop 1 comments, want the lead's one only", got)
	}
	if got := systemStopCommentCount(t, request, "BLOCKED · stop 1"); got != 0 {
		t.Fatalf("the server posted %d stop 1 comments on top of the lead's", got)
	}
	if st := advance(t, request).State; st.Step != "blocked" || stopOf(st) != issueworkflow.Stop1 || requestStatus(t, request) != "blocked" {
		t.Fatalf("step %q, stop %v, status %q; want blocked at stop 1 with the status set by the server", st.Step, st.Stop, requestStatus(t, request))
	}
}

// A stop comment from before this step was entered belongs to an earlier stop. It must not stop the
// server from posting the comment for the stop that happens now.
func TestStopComment_EarlierLeadCommentDoesNotCount(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	dbfx.Comment(t, uuidToString(request.ID), "BLOCKED · stop 1 · thiếu căn cứ\n\nLần dừng trước.",
		testutil.Cols{"author_type": "agent", "author_id": f.Agents.Docs, "created_at": testutil.Raw("now() - interval '1 hour'")})

	planBlockedByLead(t, request)

	if got := systemStopCommentCount(t, request, "BLOCKED · stop 1"); got != 1 {
		t.Fatalf("the server posted %d stop 1 comments for this stop, want 1", got)
	}
}

// Only the header of the stop that happens counts: a stop 3 comment does not stand for a stop 1.
func TestStopComment_OtherStopHeaderDoesNotCount(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	dbfx.Comment(t, uuidToString(request.ID), "BLOCKED · stop 3 · hết 2 vòng sửa\n\nKhông liên quan.",
		testutil.Cols{"author_type": "agent", "author_id": f.Agents.Docs})

	planBlockedByLead(t, request)

	if got := systemStopCommentCount(t, request, "BLOCKED · stop 1"); got != 1 {
		t.Fatalf("the server posted %d stop 1 comments, want 1", got)
	}
}

// A member's comment with the header does not stand for the lead's stop comment.
func TestStopComment_MemberCommentDoesNotCount(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	dbfx.Comment(t, uuidToString(request.ID), "BLOCKED · stop 1 · thiếu căn cứ\n\nNgười dùng ghi chú.")

	planBlockedByLead(t, request)

	if got := systemStopCommentCount(t, request, "BLOCKED · stop 1"); got != 1 {
		t.Fatalf("the server posted %d stop 1 comments, want 1", got)
	}
}

// The stop 2 comment is the one a person reads before the request leaves the workspace, so a lead
// that wrote it must not get a second copy.
func TestStopComment_LeadStop2IsNotPostedTwice(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	gates := f.driveToGate(t, request)
	finishTask(t, gates["docs_gate"], f.Agents.DocsGate, "approved")
	finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "approved")
	if st := advance(t, request).State; st.Step != "handover" {
		t.Fatalf("after both gates the request is at %q, want handover", st.Step)
	}
	dbfx.Comment(t, uuidToString(request.ID), "IN REVIEW · stop 2 · trước khi ra khỏi workspace\n\nSẵn sàng duyệt.",
		testutil.Cols{"author_type": "agent", "author_id": f.Agents.Handover})
	handover := tasksAt(t, request, "handover", 0)["handover"]
	finishTask(t, handover, f.Agents.Handover, "approved")

	if st := advance(t, request).State; st.Step != "in_review" || requestStatus(t, request) != "in_review" {
		t.Fatalf("step %q, status %q; want in_review", st.Step, requestStatus(t, request))
	}
	if got := stopCommentCount(t, request, "IN REVIEW · stop 2"); got != 1 {
		t.Fatalf("%d stop 2 comments, want the lead's one only", got)
	}
	if got := systemStopCommentCount(t, request, "IN REVIEW · stop 2"); got != 0 {
		t.Fatalf("the server posted %d stop 2 comments on top of the lead's", got)
	}
}

// The gate fails for the third time and the request stops at stop 3. A lead that already wrote the
// stop 3 comment gets no second copy.
func TestStopComment_LeadStop3IsNotPostedTwice(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	gates := f.driveToGate(t, request)
	for round := 0; round < 3; round++ {
		finishTask(t, gates["docs_gate"], f.Agents.DocsGate, "rejected")
		finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "rejected")
		if round == 2 {
			dbfx.Comment(t, uuidToString(request.ID), "BLOCKED · stop 3 · hết 2 vòng sửa\n\nVẫn bị từ chối.",
				testutil.Cols{"author_type": "agent", "author_id": f.Agents.DocsGate})
		}
		advance(t, request)
		if round < 2 {
			gates = f.finishDraft(t, request, round+1)
		}
	}
	if st := advance(t, request).State; st.Step != "blocked" || stopOf(st) != issueworkflow.Stop3 {
		t.Fatalf("step %q, stop %v; want blocked at stop 3", st.Step, st.Stop)
	}
	if got := stopCommentCount(t, request, "BLOCKED · stop 3"); got != 1 {
		t.Fatalf("%d stop 3 comments, want the lead's one only", got)
	}
	if got := systemStopCommentCount(t, request, "BLOCKED · stop 3"); got != 0 {
		t.Fatalf("the server posted %d stop 3 comments on top of the lead's", got)
	}
}
