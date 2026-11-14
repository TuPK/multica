---
title: Workspaces and members
order: 3
---

A workspace is the top-level boundary. Members, issues, projects, agents, skills and run history belong to one workspace and are isolated from every other. One account can join several workspaces. Switching only changes what you are viewing; it moves no data.

### What a workspace contains

- **Work**: issues, projects, comments, attachments, run history.
- **Team**: members, roles, invitations.
- **Agents**: configuration, skills, runtimes, automations.
- **Shared settings**: workspace context, labels, custom properties, integrations.

Workspace context is given to every agent. Use it for team background, product conventions and long-lived requirements. A requirement for one piece of work belongs in that issue.

### Roles

- **Owner**: full control, including deleting the workspace. A workspace always keeps at least one owner.
- **Admin**: manages settings, members, issue statuses, MCP servers and shared options. An admin cannot use the role to run agents they were not granted.
- **Member**: works on issues, chats, and runs agents they have access to.

Roles do not decide who can run an agent. Each agent has its own access setting.

### Inviting members

Owners and admins open **Settings → Members**, click **Invite member**, enter an email and pick a role. Invitations last 7 days. The invitee joins by signing in with that email. For a link you can share yourself, create a **share link** with a role and an expiry; anyone with the link can join.

### Changing roles and removing members

Open the actions menu on a member's row and choose **Change role**. The change takes effect immediately. Admins can move people between admin and member; changes involving owners need an owner.

A removed member loses access at once. Their issues and comments stay. Runtimes they own are deactivated, agents bound to those runtimes are archived, and their unfinished runs are cancelled.

### Leaving and deleting

Members can leave a workspace, and need a new invitation to return. The last owner cannot leave; promote someone else first. Only an owner can delete a workspace. Deletion is permanent and removes all work, members, agent configuration and run history. If you only need a break, leave instead.

### Personal access tokens

A personal access token (starts with `mul_`) represents your account and reaches every workspace you can access, so treat it like a password. Create it under **Settings → API Token** with an expiry of 30 days, 90 days, one year or never. The full value is shown once. If it leaks, revoke it in the same page. Never put it in repositories, issues, comments, screenshots or logs. When you run the CLI, `delegora login` creates a token valid for 90 days and stores it in your local configuration.
