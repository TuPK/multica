# Automation check before you attach a flow

The workflow runs unattended. A stop for a person is allowed only where the spec requires it, so before you attach
`flow.json` you check the file for stops nobody needs. The server cannot read the meaning of a task's instructions;
you can, so this check is yours. Run it on every change, not only the first.

## 1. Scan the text

Search every `instructions`, `messages` value, `stops[].message` and `steps[].notice` for these phrases, in any
language the file uses. Each one is a stop written into the text:

- "dừng 1", "điểm dừng", "stop 1", "stop 2", "stop 3", "human stop"
- "chờ người", "chờ xác nhận", "chờ người dùng", "người dùng xác nhận", "người xác nhận", "cần người xác nhận"
- "wait for", "wait until", "ask the", "ask a person", "ask the requester", "ask the owner"
- "đóng task bằng `blocked`", "set the task to blocked", "`--outcome blocked`" used for a source question
- "chưa được xác nhận" as a reason to stop

If a phrase is there, rewrite the sentence (section 3). Keep a stop only when section 2 allows it.

## 2. The only stops a person may cause

A flow may stop for a person in these cases, and no others. Name the case in your comment when you keep a stop.

| Case | Stop or manual step | Why it stays |
| --- | --- | --- |
| The work leaves the workspace or is sent outward | `leaves_workspace` terminal, reached through a person's check | The spec gives the outward action to a person. |
| An irreversible action: delete data, release, change production, spend money | A `manual` step with `exit_role` | It cannot be undone. |
| The final approval of a deliverable | A `manual` step at the end, or a `member` gate | The spec gives the last approval to a person. |
| A required input that no research, default or calculation can supply, after the collector searched the request, comments, linked issues and project knowledge | A stop of class `missing_grounds` | The product cannot be made at all. |
| A rework loop that is exhausted: the draft still fails after the last round | A stop of class `loop_exhausted` | Automatic retries did not converge. |

Anything else is not a person's decision. Source disagreements, unconfirmed facts, a choice the rules settle, and a
missing field that has a default are agent decisions: record the assumption and continue.

## 3. Rewrite the sentence

Use this shape for a gate. Keep the verdict line and the outcome names of your flow.

> Source disagreements, or sources not yet confirmed, are not a reason to stop. When a figure follows from the source
> lines by calculation, state that figure and mark it as an assumption in your comment. When it does not, state both
> readings, choose the one consistent with the source lines (or the more cautious one if neither is), and mark the
> choice as an assumption. Put every assumption in the handover list for the person who approves the result, and close
> with `approved` when the draft matches its sources and every assumption is written. Use `rejected` only for an error
> the drafter can fix. Use `blocked` only when an input is missing, cannot be found after searching the request,
> comments, linked issues and project knowledge, and cannot be defaulted.

For a collector or a work task that finds a missing input:

> Before you conclude that an input is missing, search the request's attachments and comments, the linked issues and
> the project knowledge. If it is still missing and cannot be defaulted, it is missing: name each item in your
> comment and set the task to `blocked`. Otherwise record the default as an assumption and continue.

## 4. Human gates and manual steps

A gate given to a `member`, `managers` or `requester` role, and a `manual` step, are person stops. Keep one only if the
procedure says a person must decide that point, and name the procedure's sentence in your comment. Otherwise make the
gate an agent gate with its own role, and give the work and the gate different roles.

## 5. Report what you changed

In the comment that attaches the file, list:

- every phrase you rewrote, by task key;
- every stop you kept, with the row of the table in section 2 it belongs to;
- every assumption the flow makes on the person's behalf.

A person reads this before publishing. It is the record that the flow was made to run unattended.
