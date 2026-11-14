---
title: Getting started
order: 2
---

You need three things: a Delegora account, a machine with at least one AI coding tool installed and signed in (for example Claude Code or Codex), and about ten minutes.

### 1. Sign in and open a workspace

Sign in on the web or in the desktop app with an email verification code or Google. If someone invited you, accept the invitation from the email. Otherwise create a workspace: choose a name, a URL (lowercase letters, digits and hyphens; it cannot be changed later) and an issue prefix (for example `MUL` gives `MUL-1`, `MUL-2`).

### 2. Connect a machine that runs agents

Open **Runtimes** in the sidebar.

- With the desktop app: this computer is registered automatically and detected AI tools appear. After installing a new tool, click **Refresh**.
- With another machine: click **Add**, choose **Other machine or server**, and run the two commands shown there. When the daemon is online, the dialog detects the machine.

An online runtime is what lets an agent run.

### 3. Create your first agent

Open **Agents**, click **New agent**, choose an AI tool on the machine you connected, enter a name and click **Create agent**. If you would rather not write the configuration, click **Let AI write it**: the builder asks a few questions and fills the configuration in. The agent is ready when it appears in the list and shows as online.

### 4. Hand it your first issue

Click **New Issue** (or press `C`). In agent mode, describe the work in a sentence or two, choose the agent under **Created by**, and submit. Delegora creates the issue, assigns it to the agent, and the agent starts right away.

### 5. Check the result

Open the issue. The execution log shows the run's status and the agent's replies appear in the timeline. **Completed** means that run ended, not that the issue is finished. Review the result, then comment to ask for changes or change the status.

### Next steps

- Read [Agents](#agent) to configure them in depth.
- Read [Assignment and runs](#assignment-and-runs) for every way to trigger an agent.
- If an agent does not move, see [Troubleshooting](#troubleshooting).
