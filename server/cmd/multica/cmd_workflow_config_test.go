package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/cli"
)

func TestRoomWorkflowRefusalNamesTheNextStep(t *testing.T) {
	refused := &cli.HTTPError{Method: "POST", Path: "/api/issues", StatusCode: 422,
		Body: `{"code":"room_no_workflow_for_non_request","error":"not a request"}`}
	got := roomWorkflowRefusal(refused).Error()
	if !strings.Contains(got, "multica workflow config set default") {
		t.Fatalf("message = %q, want the config command", got)
	}

	squad := &cli.HTTPError{Method: "POST", Path: "/api/issues", StatusCode: 422,
		Body: `{"code":"room_workflow_required","error":"no workflow"}`}
	if got := roomWorkflowRefusal(squad).Error(); !strings.Contains(got, "Workflow tab") {
		t.Fatalf("message = %q, want the Workflow tab", got)
	}
}

func TestRoomWorkflowRefusalLeavesOtherErrorsAlone(t *testing.T) {
	plain := errors.New("boom")
	if got := roomWorkflowRefusal(plain); got != plain {
		t.Fatalf("error = %v, want it unchanged", got)
	}
	other := &cli.HTTPError{StatusCode: 422, Body: `{"code":"invalid_workflow"}`}
	if got := roomWorkflowRefusal(other); got != other {
		t.Fatalf("a different 422 code was rewritten: %v", got)
	}
}
