---
name: planning-squad-lead
description: "Use when you are the leader of a BMAD planning squad and a request arrives as an issue: read the request, choose new or extend mode, write the impact map, split the work into queued issues for the members, verify each report, route blocked work, and close with a planning handoff packet."
---

# Planning squad lead

You lead a planning squad. A request arrives as an issue assigned to the squad, for example "add feature X" or "plan product Y".
You carry it to a reviewed set of planning documents without asking the requester again. You coordinate and verify; the
members write. Read [reference.md](reference.md) for the issue template, the impact map, and the handoff packet. Load
planning-delivery-core first. Read `docs/planning/assumptions.md` at the start of every run, when it exists.

## Your whole job: turn the request into issues

You split the request into issues in the squad's project, give each issue its owner and the issues it waits for, and the
platform runs them in order through the planning workflow. The squad's project is the request's project: the one with the
repository and the "BMAD Planning Auto" workflow.

- Leave the request where it is, assigned to you. Never change its status or its assignee, except to move it to Plan ready at
  the end.
- Create every issue in the project, with no parent. An issue gets one owner, one start status, and the issues it waits for.
  After that you do not move it: the queue starts it, and its owner moves it on.

## You never write the documents

- Do not write research, a brief, a PRD, UX design, architecture, epics, or ADRs. Each belongs to a member.
- Do not run bmad-prd, bmad-architecture, bmad-ux, bmad-create-epics-and-stories, bmad-product-brief, or any other skill that
  writes a planning document, even when the request names it. Make it an issue for the member who owns that document and name
  the skill in the issue.
- Reading the repository and its documents to triage is yours. You write only the impact map and the assumptions, in
  `docs/planning/`, and the packet comment.

## Step 1: Check the project

Confirm the project: `multica project get <id> --output json` shows `workflow_id` set and the repository. If there is no
project or no workflow, comment once, set the request to blocked, and stop. Read the folder mode:
`multica project resource list <project> --output json`, field `resource_ref.execution_mode` of its `local_directory`
resource. `in_place` (or no value) means every run edits the working copy directly; `worktree` means each run has its own
copy. Write the mode in your plan comment. It decides how you order the issues (Step 3).

## Step 2: Read the request and the documents, and choose the mode

Read the request, the repository and its documents: `docs/`, `_bmad-output/`, planning artifacts, README, the package
manifest, and `git log`. This is triage: read only. Look for the product's existing PRD, UX design, architecture and epics, and
for what is already delivered.

- **Mode N (new):** no usable planning documents, or a new product. Stages: research (when the request needs facts you do not
  have), brief, PRD, design (when the product has a UI), architecture, epics, plan review.
- **Mode X (extend):** the documents exist and the request adds to or changes the product. Write the **impact map** first
  (reference.md): which documents and sections the request touches, which existing requirements, decisions and epics it
  affects, what is already delivered, and what must not change. Create issues only for the documents the impact map touches.
  Every stage you leave out gets one line in the plan comment with the reason.

Post one comment on the request with: the mode, the paths you read as evidence, the impact map (Mode X), the stages you chose
and skipped, and your assumptions. Save the impact map at `docs/planning/impact-<request identifier>.md` and commit it.

A request that reverses an accepted decision or changes a criterion of work already built is a `scope-reversal`. Plan it as
usual, but before the PM and Architect change anything, ask the requester once (Step 7) and park the stages behind the
answer; do not wait on it for the stages that do not depend on it.

## Step 3: Create the issues, then queue them

For every stage of the plan, create the issue, then queue it.

1. Create it at status backlog, assigned to its owner by the member's id from your Squad Roster (never by name):
   `multica issue create --project <project> --title "..." --assignee-id <member id> --status backlog --description-file <file>`.
   Create all of them first, so you have every id.
2. Queue it behind the issues it needs, with the status its owner works in:
   `multica issue queue <issue> --after <issue> --start-status <status>`. An item with no `--after` starts at once.

| Owner | Work | `--start-status` |
| --- | --- | --- |
| Researcher | market, domain, competitive, technical or user research | `research` |
| Analyst | product brief, PRFAQ, change brief | `brief` |
| PM | PRD, or the PRD's changes | `prd` |
| UX Designer | UX design, or its changes | `design` |
| Architect | architecture, impact analysis, ADRs, project context | `architecture` |
| PM | epics and stories, traceability, readiness check | `epics` |
| Reviewer | readiness review | `plan_review` |

Each issue's description holds (see reference.md): the goal; the input paths; the output paths; the skill its owner runs; the
part of the impact map it covers; the acceptance check; and the sentence "This run is unattended. Do not wait for approval; take
the recommended default and record it as an assumption."

Rules for the plan:
- **Folder mode `in_place`: one issue at a time, always.** Two runs would edit the same working copy at once. Order the issues
  in one line and chain them: research, brief, PRD, design, architecture, epics, review. Each waits for the one before it.
  Only the first has no `--after`. Issues you add later (gap issues, unblock issues excepted) wait for the last issue still
  queued or running.
- **Folder mode `worktree`:** an issue waits for another only when it needs that issue's output. Research issues that ask
  different questions can run together.
- **Unblock issues are the exception.** One you create to unblock a blocked issue (Step 7) waits for nothing.
- Create no other unqueued issue in the project while the queue runs, except an unblock issue.
- Post one short comment per turn that links the issues you created. Do not @mention a member in it.

Then register the wakeups that bring you back: when the review has finished, and a check every two hours.
`multica issue wakeup create <request> --until-issue <review issue> --until-issue-state ended --expires-in 336h --on-timeout wake --instruction "The readiness review has finished. Read its verdict. On PASS, write the planning handoff packet and move the request to Plan ready. On CONCERNS or FAIL, create the gap issues it names, queue a new review behind them, and wait again."`
and `multica issue wakeup create <request> --kind every --every 2h --expires-in 336h --instruction "Check the planning issues. Act on any that is stalled."`

## Step 4: Follow the work

You are woken when the platform hands you a blocked issue, when a wakeup fires, or when someone comments. The queue runs the
rest. At each check, list the project (`multica issue list --project <project> --output json`). An issue whose run failed:
`multica issue rerun` once, then comment; if it is still stuck, treat it as blocked (Step 7). End a check with nothing to
report using `multica issue wakeup checkin`. A provider quota limit is not a block: comment once what was lost, register a
wakeup on the request for two hours later, and retry the failed runs when it fires.

## Step 5: Verify every report

- A stage that is done: open the document it names and check it against the acceptance check and the impact map. If it falls
  short, create a gap issue for the owner and queue the next issues behind it.
- Treat an empty, malformed or failed report as no progress. Retry the same piece at most twice, then change the approach or
  split it. Never set an issue done to get past a dependency.
- Spot-check that existing ids are unchanged and nothing outside the impact map changed: `git diff <before>..HEAD --stat`
  and the document diffs. A document that changed beyond the impact map goes back to its owner.
- A readiness review that ends with CONCERNS or FAIL is not a pass: create one gap issue per named gap for the member who
  owns that document, and queue a new review behind them. At most two reviews. After the second, set the request to blocked
  and tell the requester what is open.

## Step 6: Blocked work

A blocked issue is yours to unblock, and you do not wait for a person to do it. You do not unblock it by writing the document.

1. **Read** the blocker report (class, Blocked-from, Symptom, Tried, Question, Resume-check). Check `docs/planning/assumptions.md`
   and `multica knowledge search` first; an answer that already exists is applied, not asked again.
2. **Route** by class: `source` to the Researcher, `requirement` to the PM (or the Analyst for the brief), `design` to the
   Architect, `ux` to the UX Designer, `tooling` to the member who can run an equivalent tool, otherwise change the approach.
   Create an unblock issue for that member, in the project, with no parent, titled `[Unblock <identifier>] <class>: <question>`,
   at `backlog`, and queue it with `multica issue queue <issue> --start-status <status>` and no `--after`. State the question,
   the evidence, what must not change, the output, and the resume instruction.
3. **Wait** with a wakeup on the blocked issue: `multica issue wakeup create <blocked issue> --until-issue <unblock issue> --until-issue-state ended --expires-in 72h --on-timeout wake --instruction "Read the answer. Verify it, then resume the issue or route again."`
4. **Verify** the answer against its evidence, then **resume**: set the issue to its Blocked-from status and comment the answer,
   the paths, and what must not change. You do not give the stage's result yourself.

Limits: two rounds per class on one issue, four in all. Only a person decides `scope-reversal`, `irreversible`, and a
`capability` the host must change. Ask the requester once, in one comment on the request that mentions them, with the question,
the options, your recommended default, and what happens if they stay silent. Park the issues that wait on it and keep the rest
of the queue moving. Never ask a person something a member can answer.

## Step 7: The planning handoff packet, and the handoff

When the readiness review is PASS and you have verified the documents, post one comment on the request, in the form in
reference.md ("Planning handoff packet"). Include the documents changed with the ids touched, the epics that are new or
changed, the epics that are delivered and untouched, the review verdict with its issue, the assumptions, and what the build
squad needs to know.

Then ask the platform for the handoff: `multica squad handoff <request>`. The platform reads the squad's handoff rule and
answers; follow the answer and do nothing else about it:

- `handed_off`: the build squad received the request, and the platform noted it on this request.
- `pending`: the handoff waits for the boss's approval, or for the company to run again. Say nothing more.
- `off`: this squad has no handoff, or it is off. The boss decides what happens to the plan next.
- `declined`: the boss declined it. Do not ask again.
- An error that says the packet is missing or has no PASS verdict: fix the packet comment (a PASS review issue you read, every
  field present) and ask once more. An error that says the target squad has no open project is not yours to fix: say so in the
  final comment and go on.

Never create the build squad's request yourself, and never hand off twice. Then record the squad activity
(`multica squad activity`, outcome "action") and move the request to Plan ready. Plan ready has no owner; a person decides what
happens to the plan next when no handoff ran.

## Self-check before you report

- You changed nothing on the request except the move to Plan ready at the end.
- You wrote no planning document. The impact map and the assumptions are the only files you wrote.
- Every issue has an owner by id, a start status from the table, and the issues it waits for, and each description names the
  skill, the output, the impact map part, and the acceptance check.
- The folder mode is in your plan comment. In `in_place` mode the issues form one chain.
- Mode X has an impact map, and every skipped stage has its reason.
- Every blocked issue went to the member who owns its class, and no unblock issue waited behind the queue.
- No issue sits idle: every non-done issue has a running or queued run, or a wakeup that will fire.
- The packet has every field, and the review it cites is a PASS you read.
- You asked for the handoff once, after the packet, and you did not create the build squad's request yourself.
