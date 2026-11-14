---
title: Assignment and runs
order: 7
---

There are four ways to trigger an agent. Choose by need: give it ownership, ask it one thing, wake it on a condition, or run it on a schedule.

### Assigning an issue (ongoing ownership)

When work needs a clear owner, assign the issue to an agent or squad. The agent reads the description and discussion, does the work with its own configuration and updates the status. The confirmation dialog shows which agent will start. **Don't start yet** saves the assignee without creating a run; moving the issue out of `backlog` starts it.

Assigning to a **member** creates no run and only records who owns the follow-up. Assigning to a **squad** wakes the leader first; the leader decides who handles each step.

### Mentioning an agent in a comment (one request)

When you need one answer or one review and the owner should not change, mention the agent with `@` in a comment. Before you send, the trigger preview shows whether each agent is reached by a direct mention, by the assignee, or by a squad leader. Untick an agent to skip that one trigger.

A comment with no `@` is routed by context: a reply inside a discussion an agent already joined goes to that agent; a top-level comment goes to the assignee. Starting a comment with `/note` triggers no agent. `@all` notifies every member and turns off automatic triggering for the assignee.

Agents keep replying in their own threads even while their runtime is offline or busy. Delegora does not silently switch to another agent after a timeout.

### Steering a running agent

When an agent is heading the wrong way, do not stop and restart it. Send a correction as a comment; it joins the current run. The label under the comment box shows how it will be delivered, and you can change that. Only text joins a running run; messages with attachments are handled after it ends. Steering needs support from the AI tool.

Several consecutive comments for the same agent merge into one waiting run and are handled together.

### Wakeups (waiting on a condition)

A wakeup is a rule on an issue that says "when this happens, wake this agent". Use it when work has to wait: for a supplier's quote, a colleague's answer, sub-issues to finish, or a pull request to merge. The rule is stored on the issue, so nothing runs while it waits.

Conditions include a time, a reply from a person, a field or status change, sub-issues finishing, a linked pull request merging, or another issue changing. You can repeat a rule up to 50 times. At its deadline, the agent is woken to report that the condition did not happen, or the rule can end without a run. Agents can also schedule their own next check during a run.

A wakeup run acts on behalf of the person who created the rule, and Delegora rechecks that person's permission before it starts.

### Runs and the execution log

Every time an agent works, Delegora records a run: what triggered it, which agent, its progress and its result. A run is queued, running, completed, failed or cancelled. The execution log on the issue lists every run, lets you open any transcript, stop queued or running work, and retry.

Transient failures (a brief runtime disconnect, a daemon restart, a timeout, a network drop while the tool runs) can be retried automatically. Errors returned by the agent itself, such as an expired credential or an exhausted quota, usually are not. Manual retry is always available, and a rerun repeats the work with the same agent and context.

### Common failure reasons

- **Timeout**: split the request into smaller steps.
- **Model account out of credit**: top up or switch accounts.
- **Model account not signed in**: sign in again on the machine that runs the agent.
- **Runtime offline**: check the machine and the daemon.
- **Machine could not set up the run** (disk full, missing permission): the detailed error appears below the message.

The full list, with how to handle each, is in the Errors tab of Analytics.
