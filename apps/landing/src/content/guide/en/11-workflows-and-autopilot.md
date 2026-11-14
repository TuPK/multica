---
title: Workflows and Autopilot
order: 11
---

## Workflows

A workflow sets the steps a project's issues go through, who handles each step, and what they should do. For example, a delivery workflow: **Development** goes to an engineering agent; **Review** goes to a review agent, which passes it to QA or sends it back; **QA** is a member who decides whether it is finished.

When an issue moves into Review, it is handed to the review agent. The description, discussion and earlier results stay on the issue, so the next person or agent picks up where the last one stopped.

### Creating a workflow

Owners and admins open **Settings → Workflows**, click **New workflow**, and start blank or from the **Delivery**, **Content** or **Community** template. Name it, choose its status columns and order them the way your team works.

For each column, choose what happens when an issue enters it: **No handoff**, **Hand to an agent**, or **Hand to a member** (you can also pick the project lead). For an agent step, write its instructions, set **When done, move to** (the next column if left empty) and **If rejected, send back to** for rework. Use **Blocked**, not send-back, when the step cannot continue for an outside reason.

Delegora checks the structure on save: Backlog comes first, Todo directly after Backlog, Blocked before Done, Done directly before Cancelled, Cancelled last, every step can reach an end state, and no step loops to itself. Errors are shown precisely so you can fix them.

### Using it in a project

Open a project and choose its **Workflow**. Changing the workflow does not hand off issues or start agents. A project without a choice uses **Default**, where every status is available and nothing is handed off automatically.

One workflow can serve several projects, and edits apply to all of them. To give one project its own process, duplicate the workflow and select the copy there.

A workflow still used by a project or squad must be switched to another workflow before it can be deleted.

## Autopilot

Recurring work, such as a daily progress summary, a periodic dependency check, or an agent triggered by an event from another system, belongs in an autopilot. Each autopilot stores a **runbook** (detailed instructions the agent reads on every run), an assignee, and one or more **triggers**.

There are six built-in templates: daily news, stale pull request review, bug triage, weekly progress report, dependency audit, documentation check.

### Triggers

- **Schedule**: a cron expression with an IANA timezone (for example `Europe/London`). The page previews the next run times so you can check before saving.
- **Webhook**: after saving, Delegora generates a URL. Another system sends a POST with JSON to start a run. Add an `Idempotency-Key` so retries do not run twice. You can filter by event type. If the URL leaks, click **Rotate URL**; the old URL stops working immediately.

### Run modes

- **Create issue**: each run creates an issue and runs like any other. If the runtime is offline, the issue is still created and the run waits.
- **Run only**: no issue is created. If the runtime is not available when the trigger fires, the run is skipped and leaves nothing waiting.

### Managing

**Run now** executes the whole flow once without waiting for the schedule. Use it to test before turning automation on. The delivery list shows every webhook received; you can replay only deliveries with a valid signature that were not rejected and have finished processing.

Owners and admins always have access; you can grant other members permission to edit and run an autopilot. The number of automatic runs per period is capped, and runs past the cap wait for the next period. An autopilot pauses after repeated failures, and you are notified in your inbox.
