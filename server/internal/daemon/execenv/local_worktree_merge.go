package execenv

import (
	"log/slog"
	"strings"
)

// Outcomes of merging a delivered task branch, as reported on the task.
const (
	// BranchMergeMerged: the branch landed in the checked-out branch and was
	// deleted, together with its recorded snapshot.
	BranchMergeMerged = "merged"
	// BranchMergeSkipped: a precondition refused the merge before git ran. The
	// branch is kept for a manual merge, and the reason says why.
	BranchMergeSkipped = "skipped"
	// BranchMergeFailed: git refused the merge (typically a conflict). The
	// repository is restored and the branch is kept.
	BranchMergeFailed = "failed"
)

// mergeDeliveredBranch merges a delivered task branch into the branch checked
// out in the user's repository. It is the opt-in counterpart to the rule that
// Multica never writes to the user's working copy, so every refusal below
// leaves the branch for the user to merge by hand.
//
// It returns the outcome and, for anything but a merge, the reason. It never
// returns an error: the work is already delivered when this runs, and a merge
// that does not land must not fail the task. The caller holds the git root lock.
func mergeDeliveredBranch(gitRoot, branch string, logger *slog.Logger) (status, reason string) {
	skip := func(why string, err error) (string, string) {
		if logger != nil {
			logger.Warn("execenv: auto-merge did not land; the branch is left for a manual merge",
				"git_root", gitRoot, "branch", branch, "reason", why, "error", err)
		}
		return BranchMergeSkipped, why
	}

	if _, err := runGitTrimmed(gitRoot, "symbolic-ref", "--quiet", "HEAD"); err != nil {
		return skip("the repository is on a detached HEAD", err)
	}
	// Tracked edits only: a merge that would overwrite an untracked file fails
	// on its own and is aborted below, while untracked files are common in a
	// working copy and must not block the merge.
	porcelain, err := runGitStdout(gitRoot, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return skip("could not read the repository status", err)
	}
	if strings.TrimSpace(porcelain) != "" {
		return skip("the repository has uncommitted changes to tracked files", nil)
	}

	subjects, err := runGitStdout(gitRoot, "log", "--format=%s", "HEAD.."+branch)
	if err != nil {
		return skip("could not list the branch commits", err)
	}
	for _, subject := range strings.Split(strings.TrimSpace(subjects), "\n") {
		if subject == baselineUserStateSubject || subject == continuedUserStateSubject {
			return skip("the branch carries your uncommitted edits as commits", nil)
		}
	}

	// --no-verify for the same reason commitEverything uses it: the user's hooks
	// must not decide whether the agent's work lands.
	if _, err := runGit(gitRoot, "merge", "--no-ff", "--no-edit", "--no-verify",
		"-m", "Merge agent branch "+branch, branch); err != nil {
		if _, headErr := runGit(gitRoot, "rev-parse", "-q", "--verify", "MERGE_HEAD"); headErr == nil {
			if out, abortErr := runGit(gitRoot, "merge", "--abort"); abortErr != nil {
				return BranchMergeFailed, "merge failed and could not be aborted: " + strings.TrimSpace(out)
			}
		}
		mergeErr := withGitStderr(err)
		if logger != nil {
			logger.Warn("execenv: auto-merge failed; the repository was restored and the branch is left for a manual merge",
				"git_root", gitRoot, "branch", branch, "error", mergeErr)
		}
		return BranchMergeFailed, mergeErr.Error()
	}

	// The branch is now part of the checked-out history, so nothing is lost by
	// deleting it; dropBranch also removes its recorded snapshot.
	dropBranch(gitRoot, branch, logger)
	if logger != nil {
		logger.Info("execenv: merged the delivered task branch into the checked-out branch and deleted it",
			"git_root", gitRoot, "branch", branch)
	}
	return BranchMergeMerged, ""
}
