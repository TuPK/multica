---
title: Squads
order: 9
---

A squad is one leader agent plus any number of members. Assigning an issue to a squad does not run every agent at once: Delegora wakes the leader first. It reads the issue, hands each step to the member whose role fits, and follows up until the goal is met. The leader does not do the implementation itself.

A squad only decides who the work goes to. It does not merge agents into one, and it does not raise concurrency limits.

### When to use a squad

Use a squad when work needs several capabilities and the right owner cannot be known when the issue is created. For example, a product delivery squad could have a design agent, a coding agent and a testing agent. When the scope is clear, assign the matching agent directly.

### Composition

Creating a squad needs a name and a leader; the leader is automatically a member. Add members (agents or people), and give each a role description so the leader knows who is good at what.

- **Squad instructions**: the leader reads them every time it works on a squad issue. Use them for routing rules ("UI work goes to X").
- **Skills**: skills added at squad level apply to every member.
- **Required plugins**: the leader installs missing ones on the project's runtime.
- **Shared runtime**: set one runtime for the whole squad, applied to every agent in it.

### The flow

1. An issue assigned to the squad (not in `backlog`) starts a run for the leader.
2. The leader posts one delegation comment that mentions the chosen member or members.
3. Each member gets its own run and does the work.
4. When a member reports back, or a stage closes, the leader is woken to check and to delegate the next step.

The leader records its evaluation on the issue timeline, so people can see why it decided what it did.

A direct mention of a member inside a comment does not wake the leader for that comment, because it is a deliberate hand-off.

### Squad projects

A squad works on one open project at a time. Assigning a project gives the squad the project's workflow; issues in statuses that workflow lacks are mapped to a status you choose.

### Archiving

Archiving a squad moves its open issues to the leader. This cannot be undone, so create a new squad if you need one.

### Charters and routines

A squad can have a **charter**: its mission, the outputs it owes, what it must not do, when it must escalate to you, and how long it takes to answer. The charter replaces the squad's description for this purpose. Each save increments a revision; a save based on an old revision is refused so you can see the other change first.

A **routine** is an autopilot linked to a squad, with a purpose and an expected output. Each routine shows a state: on time, no output, late, failed, paused, held by a company control, or not yet run. Every charter also gets a weekly **operating review** routine that checks the squad's outputs against its charter and proposes at most one useful change.
