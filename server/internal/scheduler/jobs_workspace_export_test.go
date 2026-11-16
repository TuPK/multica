package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Pure: no database. The queue logic is tested in internal/workspaceexport;
// this covers the job's registration contract and how it reports a run.

type fakeExportRunner struct {
	n   int
	err error
	at  int
}

func (f *fakeExportRunner) RunOnce(context.Context) (int, error) {
	f.at++
	return f.n, f.err
}

func TestWorkspaceExportJobRunsEveryMinuteUnderItsStableName(t *testing.T) {
	spec := WorkspaceExportJob(&fakeExportRunner{})
	if spec.Name != "workspace_export" {
		t.Fatalf("name = %q, want workspace_export", spec.Name)
	}
	if spec.Cadence != time.Minute {
		t.Fatalf("cadence = %v, want 1m", spec.Cadence)
	}
	if spec.Handler == nil {
		t.Fatal("no handler")
	}
	if err := spec.validate(); err != nil {
		t.Fatalf("the spec is not valid: %v", err)
	}
}

func TestWorkspaceExportJobReportsTheRowsItProcessed(t *testing.T) {
	runner := &fakeExportRunner{n: 3}
	res, err := WorkspaceExportJob(runner).Handler(context.Background(), HandlerInput{})
	if err != nil || res.RowsAffected != 3 || runner.at != 1 {
		t.Fatalf("result = %+v, err = %v, runs = %d", res, err, runner.at)
	}
}

func TestWorkspaceExportJobReturnsARunnerError(t *testing.T) {
	boom := errors.New("boom")
	if _, err := WorkspaceExportJob(&fakeExportRunner{err: boom}).Handler(context.Background(), HandlerInput{}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}
