---
name: secretary-ask
description: "Use when the boss asks a question that needs the departments' judgment, such as progress against a plan, a risk, a cause, or whether a launch is on track. Chooses the leaders, files the request, writes each ask, and tells the boss who was asked."
---

# Secretary ask

You do not know the work. The leaders do. Your job here is to choose who answers,
ask each one well, file the request so the answers can come back to the boss, and
reply to the boss at once. You do not gather the facts yourself and you do not
analyse them. That happens in `secretary-analysis`, when the answers arrive.

A judgment question is never answered from the digest. Stop and use this skill.

## Steps

### 1. Pin the question

Write the question in one line: what the boss wants to know, the project, the period,
and the date the boss needs it by. If the project or the period is genuinely unclear,
ask the boss one short question and stop. Do not guess and fan out.

### 2. Choose the addressees

Read the org map. Choose from it; never guess an id.

```bash
multica company map --output json
multica company map --floor <floor-id> --output json   # to narrow to one floor
```

The map lists each floor with its chief, and each squad with its leader, project and
open work.

- A narrow question goes to the squad leader that owns the project.
- A floor-wide question goes to the floor chief when the floor has one.
- A company-wide question goes to the chiefs, and to the leaders of squads outside any
  chiefed floor.
- Ask a squad that the question depends on when the question touches the dependency.
- Leave out a squad whose answer you can already read. Say who you left out and why.

Platform caps: at most 20 addressees per request and 8 open requests per workspace.

### 3. File the parent request

Find the desk, then file the request assigned to you, with a due date and the chat
session in its metadata:

```bash
multica project list                       # find the "Secretary desk" project id
multica issue create --project <desk-id> --assignee-id <your-agent-id> \
  --title "<the boss's question, short>" --due-date <YYYY-MM-DD> \
  --description-file <file>
multica issue metadata set <request-id> --key chat_session_id --value '"<session-id>"'
multica issue metadata set <request-id> --key asked_by --value '"<boss-user-id>"'
```

The description file holds the question in full, the boss's words, and the deadline.

### 4. File one child issue per addressee

```bash
# a squad, which tags it as a request to that squad
multica issue create --assignee-squad <squad-id> --parent <request-id> \
  --title "<what you need from this squad>" --description-file <file>

# a chief or a leader agent
multica issue create --assignee-id <agent-id> --parent <request-id> \
  --title "<what you need>" --description-file <file>
```

Write each child in your own words, for its addressee. Include:

- The boss's question and why it matters to the boss now.
- The period, and the depth that helps: a one-page status or a deep review.
- What you already know, so the leader does not repeat it.
- What you want back, in the answer format in the reference.
- When you need it.

Do not pre-digest the facts. The leader gathers and analyses its own part. You may
point to the issues that matter.

Send one question per child. Do not ask the same leader the same question twice.

### 5. Set the deadline

Set a one-shot time wakeup on the request, so the reading starts even if a child never
closes:

```bash
multica issue wakeup create <request-id> --kind at --at <deadline-RFC3339> --mode once \
  --instruction "The deadline has passed: read the answers and assess what is in."
```

### 6. Tell the boss

Reply in the boss's chat at once: who you asked, about what, and when you expect the
answers. In this chat turn, your reply is your final message. In a later run, use
`multica chat reply --session <session-id> --file <file>`.

Then end the turn. Do not wait for the answers. They arrive in a later run, and the
`secretary-analysis` skill handles them.

## Rules

- Never ask a leader to fill in the digest's facts. The digest holds them.
- Never answer the judgment yourself from the digest or your own reading of issues.
- Never create work the boss did not ask for. A request needs the boss's question in
  chat.
- Name the request and its children in your reply, with links, so the boss can open
  them.
