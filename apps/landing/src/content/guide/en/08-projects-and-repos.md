---
title: Projects and repositories
order: 8
---

A project groups issues that share one goal, tracks progress together, and links resources (repositories or folders) so agents know which code to work in. Work that a single issue can describe does not need a project.

### Progress and status

A project's progress is the share of its issues in the done or closed groups. Project status and issue status are independent: finishing every issue does not mark the project complete.

The project description is shared with every run inside the project. Put there what every run needs to know.

The lead can be a member or an agent. Making someone the lead does not assign issues or start work. A member who is the lead can also change the project's workflow.

### Resources

- **GitHub repository**: a repo the runtime can check out, with an optional branch, tag or commit.
- **Local directory**: lets a runtime on one machine work in an existing folder, with the agent editing your files directly. Use it only when nothing else fits.

Any Git URL the runtime can reach works as a repository, not only GitHub.

For repositories, each run gets its own **worktree**, so several runs can work in parallel without colliding. Each run hands back an `agent/…` branch for you to review and merge.

For local directories there are two modes: **Direct** (one run at a time; changes appear in your folder as the agent works) and **Parallel** (each run gets its own worktree; the folder must be a git repository with at least one commit).

**Auto-merge** (optional): when a run finishes, its branch is merged into the checked-out branch and then deleted. It is skipped if the repository has uncommitted changes or is detached.

### Connecting GitHub

An owner or admin opens **Settings → Code**, clicks **Connect GitHub**, chooses the account or organisation, and grants access to all repositories or selected ones. The GitHub app is read-only: it never pushes commits, comments or sets status checks.

The switches:

- **PR sidebar**: show linked pull requests in issue details.
- **Co-authored-by**: add a co-author line to commits made by agents.
- **Auto-link PRs**: link a PR to the issue whose identifier appears in its branch name or title, or follows a closing keyword in its body.
- **After PRs merge, move the issue to**: `Done` (default), another status, or leave it unchanged.
- **PR card → CI and mergeability**: show checks and conflicts on each PR.

### Linking a PR to an issue

Put the identifier in the branch name or PR title, such as `mul-123-fix-login-redirect` or `MUL-123 Fix the redirect after login`. If it appears only in the body, put it right after `Closes`, `Fixes` or `Resolves`. A plain mention such as `Related to MUL-123` does not link. You can also link by hand with **+** in the Pull requests block.

### When merged PRs move an issue

An issue moves to the chosen status only when **every** linked PR is merged, the issue is not already `done` or `cancelled`, and it is not set to keep its status. A PR closed without merging keeps the issue waiting until you remove it. For an issue whose PR delivers only part of the work, or where a release step follows, use **Keep status when PRs merge** in the Pull requests menu.

### Several workspaces

One GitHub app installation can connect to several workspaces. Each workspace matches only its own issue prefix. If an identifier matches an issue in more than one connected workspace, nothing is linked automatically, so an unrelated issue is never closed by mistake.

### Disconnecting

Disconnecting from **Settings → Code** only removes the link between this workspace and the installation. To revoke repository access on the GitHub side, remove the app from your GitHub installations page.
