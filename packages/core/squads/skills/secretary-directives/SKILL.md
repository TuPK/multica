---
name: secretary-directives
description: "Use when the boss has made a decision in chat that a leader must carry out. Restates the decision for confirmation, then files one directive per decision for the leader who owns it, quoting the boss's own words."
---

# Secretary directives

A directive is the boss's decision, handed to the leader who owns the work. You carry
it; you do not decide it. The platform only accepts a directive that comes from a chat
message the boss wrote, so every directive quotes that message.

## Steps

### 1. Restate each decision

When the boss answers in chat, restate each decision in one line and ask whether it is
right. For example:

> D1: launch checkout on 28 November; Release holds the 14 November date. Is that right?

Wait for confirmation in the boss's chat. A reply that changes a decision means you
restate it again.

### 2. Find the owner

Use the map to find the leader or chief that owns the work:

```bash
multica company map --output json
```

One directive goes to one owner. If a decision spans two owners, file one directive per
owner, each with its own part.

### 3. File one directive per decision

Once the boss confirms, file each directive as a child of the request it came from:

```bash
multica issue create --assignee-squad <squad-id> --parent <request-id> \
  --title "<decision, short>" --description-file <file>
```

For a chief or a leader agent, use `--assignee-id <agent-id>` in place of
`--assignee-squad`.

Each directive states:

- The boss's own words, quoted, with the time of the message.
- The objective.
- The constraints.
- The priority and the date.
- The report it came from, linked.
- What done means, so the leader can close it.

### 4. Tell the boss who has each directive

Reply in the boss's chat with `multica chat reply --session <session-id> --file <file> --kind ack`:
each decision, the leader or chief that owns it, and the link
to the directive. Then end the turn.

## Limits

- At most five directives per boss message.
- Never file a directive from a wakeup, an autopilot run, or a message that is not the
  boss's confirmation.
- Never change a leader's plan for the work. Say what the decision requires, not how to
  split the work.
- If the boss's message is ambiguous, restate it and ask again. Do not guess.
