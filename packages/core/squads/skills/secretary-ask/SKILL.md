---
name: secretary-ask
description: "Use when the boss asks a question that needs the departments' judgment, such as progress against a plan, a risk, a cause, or whether a launch is on track. Chooses the leaders, files the request in one call, writes each ask, and tells the boss who was asked."
---

# Secretary ask

You do not know the work. The leaders do. Your job here is to choose who answers,
ask each one well, file the request in one call so the answers can come back to the
boss, and reply to the boss at once. You do not gather the facts yourself and you do
not analyse them. That happens in `secretary-analysis`, when the answers arrive.

A judgment question is never answered from the digest. Stop and use this skill.

## Steps

### 1. Pin the question

Write the question in one line: what the boss wants to know, the project, the period,
and the date the boss needs it by. If the project or the period is genuinely unclear,
reply to the boss with one short question (`--kind clarify`) and stop. Do not guess
and fan out.

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

### 3. File the request in one call

Write the boss's question to a file, and the addressees to a JSON file. Each addressee
names exactly one of `squad_id` or `agent_id`, and carries a `title` and the
`description` of its child issue: the ask, written for that addressee.

```bash
multica secretary ask --chat-session <session-id> \
  --question-file <question.md> --addressees-file <addressees.json> \
  --due-date <YYYY-MM-DD> --deadline <RFC3339>
```

The call files the request in the Secretary desk, one child issue per addressee, the
request's metadata (the chat, the boss, the deadline), and the deadline wakeup. If any
step fails, the call cancels what it had filed. Read the response: `request_id` and
`child_ids` are the ids you name to the boss.

The `chat-session` is the session of the boss's message you are answering. The call is
refused anywhere else.

### 4. Write each ask

Each child is written in your own words, for its addressee. Include:

- The boss's question and why it matters to the boss now.
- The period, and the depth that helps: a one-page status or a deep review.
- What you already know, so the leader does not repeat it.
- What you want back, in the answer format in the reference.
- When you need it.

Do not pre-digest the facts. The leader gathers and analyses its own part. You may
point to the issues that matter.

Send one question per child. Do not ask the same leader the same question twice.

### 5. Tell the boss

Reply in the boss's chat at once, with the reply command and `--kind ack`: who you
asked, about what, and when you expect the answers.

```bash
multica chat reply --session <session-id> --file <reply.md> --kind ack
```

The boss sees only what you write with `multica chat reply`. Your final message in this
turn is not shown to the boss, so do not put the answer there.

Then end the turn. Do not wait for the answers. They arrive in a later run, and the
`secretary-analysis` skill handles them.

## Rules

- Never ask a leader to fill in the digest's facts. The digest holds them.
- Never answer the judgment yourself from the digest or your own reading of issues.
- Never create work the boss did not ask for. A request needs the boss's question in
  chat.
- Name the request and its children in your reply, with links, so the boss can open
  them.
