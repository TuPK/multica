package execenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// prepareAutoMerge builds a worktree task on repo with auto-merge set, the way
// the daemon does for a local_directory resource that opted in.
func prepareAutoMerge(t *testing.T, repo string, autoMerge bool) *LocalWorktree {
	t.Helper()
	wt, err := PrepareLocalWorktree(LocalWorktreeParams{
		LocalPath: repo,
		EnvRoot:   t.TempDir(),
		AgentName: "J",
		TaskID:    "11112222-3333-4444-5555-666677778888",
		AutoMerge: autoMerge,
	}, worktreeTestLogger())
	if err != nil {
		t.Fatalf("PrepareLocalWorktree: %v", err)
	}
	return wt
}

func TestFinalizeAutoMergesDeliveredBranchIntoCheckedOutBranch(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	wt := prepareAutoMerge(t, repo, true)
	writeFile(t, filepath.Join(wt.WorkDir, "real-change.txt"), "actual work\n")

	branch := wt.Branch
	outcome := finalizeOK(t, wt)
	if outcome.BranchMergeStatus != BranchMergeMerged {
		t.Fatalf("BranchMergeStatus = %q (%s), want %q", outcome.BranchMergeStatus, outcome.BranchMergeError, BranchMergeMerged)
	}
	if outcome.Branch != "" {
		t.Errorf("Branch = %q after a merge, want it cleared", outcome.Branch)
	}
	if _, err := gitTry(t, repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		t.Errorf("branch %s still exists after a successful merge", branch)
	}
	if refs := gitRun(t, repo, "for-each-ref", "--format=%(refname)", "refs/multica"); refs != "" {
		t.Errorf("snapshot refs left behind after a merge:\n%s", refs)
	}

	if got := readFile(t, filepath.Join(repo, "real-change.txt")); got != "actual work\n" {
		t.Errorf("agent change not present on the checked-out branch: %q", got)
	}
	if status := gitRun(t, repo, "status", "--porcelain"); status != "" {
		t.Errorf("repository not clean after auto-merge:\n%s", status)
	}
	if subject := gitRun(t, repo, "log", "-1", "--format=%s"); !strings.HasPrefix(subject, "Merge agent branch ") {
		t.Errorf("HEAD subject = %q, want a merge of the agent branch", subject)
	}
}

func TestFinalizeWithoutAutoMergeLeavesCheckedOutBranchAlone(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	headBefore := gitRun(t, repo, "rev-parse", "HEAD")
	wt := prepareAutoMerge(t, repo, false)
	writeFile(t, filepath.Join(wt.WorkDir, "real-change.txt"), "actual work\n")

	outcome := finalizeOK(t, wt)

	if outcome.BranchMergeStatus != "" {
		t.Errorf("BranchMergeStatus = %q without auto-merge, want empty", outcome.BranchMergeStatus)
	}
	if got := gitRun(t, repo, "rev-parse", "HEAD"); got != headBefore {
		t.Errorf("HEAD moved without auto-merge: %s -> %s", headBefore, got)
	}
	if _, err := os.Stat(filepath.Join(repo, "real-change.txt")); !os.IsNotExist(err) {
		t.Errorf("agent change written into the working copy without auto-merge: %v", err)
	}
	gitRun(t, repo, "rev-parse", "--verify", outcome.Branch)
}

func TestFinalizeAutoMergeSkipsDirtyRepository(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	wt := prepareAutoMerge(t, repo, true)
	writeFile(t, filepath.Join(wt.WorkDir, "real-change.txt"), "actual work\n")
	headBefore := gitRun(t, repo, "rev-parse", "HEAD")
	// The user's own uncommitted edit to a tracked file must survive untouched.
	writeFile(t, filepath.Join(repo, "tracked.txt"), "user is editing\n")

	outcome := finalizeOK(t, wt)

	if outcome.BranchMergeStatus != BranchMergeSkipped || outcome.BranchMergeError == "" {
		t.Errorf("BranchMergeStatus = %q (%q), want %q with a reason", outcome.BranchMergeStatus, outcome.BranchMergeError, BranchMergeSkipped)
	}
	if got := gitRun(t, repo, "rev-parse", "HEAD"); got != headBefore {
		t.Errorf("HEAD moved despite uncommitted edits: %s -> %s", headBefore, got)
	}
	if got := readFile(t, filepath.Join(repo, "tracked.txt")); got != "user is editing\n" {
		t.Errorf("user's uncommitted edit was overwritten: %q", got)
	}
	gitRun(t, repo, "rev-parse", "--verify", outcome.Branch)
}

func TestFinalizeAutoMergeAbortsConflictAndKeepsBranch(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	wt := prepareAutoMerge(t, repo, true)
	writeFile(t, filepath.Join(wt.WorkDir, "tracked.txt"), "agent version\n")
	// The user commits a different change to the same line after the task
	// started, so the agent's merge cannot be applied cleanly.
	writeFile(t, filepath.Join(repo, "tracked.txt"), "user version\n")
	gitRun(t, repo, "commit", "-am", "user change")
	userHead := gitRun(t, repo, "rev-parse", "HEAD")

	outcome := finalizeOK(t, wt)

	if outcome.BranchMergeStatus != BranchMergeFailed || outcome.BranchMergeError == "" {
		t.Errorf("BranchMergeStatus = %q (%q), want %q with a reason", outcome.BranchMergeStatus, outcome.BranchMergeError, BranchMergeFailed)
	}
	if got := gitRun(t, repo, "rev-parse", "HEAD"); got != userHead {
		t.Errorf("HEAD moved on a conflicting merge: %s -> %s", userHead, got)
	}
	if _, err := gitTry(t, repo, "rev-parse", "-q", "--verify", "MERGE_HEAD"); err == nil {
		t.Error("MERGE_HEAD left behind; the conflicting merge was not aborted")
	}
	if got := readFile(t, filepath.Join(repo, "tracked.txt")); got != "user version\n" {
		t.Errorf("user's committed change was altered: %q", got)
	}
	gitRun(t, repo, "rev-parse", "--verify", outcome.Branch)
}
