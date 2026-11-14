---
title: Issues
order: 6
---

An issue is the basic unit for organising work: a feature, a bug, an investigation. Discussion, status changes and every run are recorded on the issue, so nobody has to dig through chat logs or terminal output for context.

### Parts of an issue

Title and description (goal, background, requirements, acceptance criteria); status and priority; the assignee (a member, an agent or a squad); dates, labels and custom properties; project and parent-child links; comments, activity and the execution log; and deliverables (files uploaded in comments).

### Creating an issue

Two modes: **create manually** (fill in the title and properties) and **create with agent** (describe the request only). In agent mode the agent writes the title and description and creates the issue; success or failure goes to your inbox. It does not add requirements you did not give. New issues start in `todo`, so an agent assignee starts working right away.

Each issue has a number such as `MUL-123`. If the workspace prefix changes, the numbers display with the new prefix.

### Statuses

Seven built-in statuses in four groups:

- **Unstarted**: `backlog` (assigning an agent here does not start a run) and `todo` (moving an issue here starts the assigned agent).
- **Started**: `in_progress`, `in_review` (waiting for review) and `blocked` (waiting on an outside dependency).
- **Done**: `done`.
- **Closed**: `cancelled` (no longer pursued, not a successful finish).

Agents move work to `in_progress` when they start and to `in_review` when they deliver. If a run fails and no other run or retry is pending, an `in_progress` issue moves to `blocked`. When every linked pull request is merged, the issue can move to `done` (see the projects and repositories chapter).

Admins can add custom statuses in one of the four groups. A custom status has no special behaviour: naming one "Waiting" does not pause an agent.

### Custom properties

Team-specific fields such as severity, customer or budget. A workspace can have up to 20 active properties. Types include text, number, select, multi-select, date, checkbox, URL, member and lists. Archiving a property hides it from pickers and filters but keeps its values.

### Views

The Issues page has five layouts: list, board, table, Gantt and swimlane, all filterable by status, assignee, project and properties. Save a combination of filters and display settings as a **view**. A private view is yours only; a workspace view can be edited by its creator and by owners and admins.

### Sub-issues

Split large work into sub-issues. The parent keeps the goal; each child progresses on its own. Sub-issues can be given a **stage** (1, 2, 3). When every sub-issue in the earliest unfinished stage is done or cancelled, the parent receives a notification and its agent assignee is woken to decide the next stage.

### Comments, mentions and deliverables

Comments support formatting, code blocks, links and attachments. Replies form threads. You can select text in a comment and add an annotation that becomes a numbered quote in your reply. Files uploaded in comments are collected under **Deliverables**; uploading the same name again creates a new version.

### Batch edits

Select several issues in a list or table to change status, priority or assignee, or to delete them together. Batch assignment to an agent asks for confirmation before any runs start.

### Duplicates

Mark an issue as a duplicate with **Mark as duplicate** and choose the original. The duplicate moves to `cancelled` and links to the original. **Not a duplicate** undoes it.

### Deleting

Workspace owners and admins can delete any issue. Members can delete issues they created or that an agent they own created. Deletion is permanent and cancels active runs. If the work is simply no longer pursued, set the status to `cancelled` instead.

### Keyboard shortcuts

`C` creates an issue; `Cmd/Ctrl+K` opens search; `Cmd/Ctrl+B` toggles the left sidebar; `Cmd/Ctrl+Enter` sends a comment. Single-key shortcuts do not fire while you type. You can rebind shortcuts under **Settings → Shortcuts**; they are saved on the current device.

### Inbox

The inbox tells you about new comments on issues you follow, changes to status, assignee, priority or dates, completed or failed issue creation by an agent, and autopilot activity you subscribe to. Your own actions do not notify you. Select a notification to mark it read, and archive it when you are done. Agents never read the inbox.
