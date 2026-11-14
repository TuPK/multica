---
title: Governance, budgets and costs
order: 14
---

## Autonomy levels

Each squad has an autonomy level from 0 to 3 (default 1). Only owners and admins change it, and every change is recorded in the governance log.

- **Suggest** (0): routine runs are held. A squad runs only when someone assigns it work or mentions it. It can still file proposals.
- **Work** (1): routine runs go ahead. The squad researches, analyses, writes knowledge drafts and files proposals. Every decision is yours.
- **Trusted** (2): as Work, and knowledge drafts can be published after review. Standing approvals can approve the squad's proposals.
- **Autonomous** (3): as Trusted, and standing approvals can create initiatives from the squad's own opportunities at the first stage, within limits on count and budget.

At every level, launching a project or changing the organisation remains your decision, and spending is always bounded by budgets.

Delegora cannot see what an agent does inside a run. An autonomy level therefore cannot stop an agent from calling an external API that its credentials allow.

### Standing approvals

A standing approval automatically approves a narrow class of proposals you have pre-authorised: the proposal kind, the squads or floors it covers, a maximum cost, and a weekly maximum. You can review each approval in the brief and revoke it at any time. If a review reverses one of its approvals, it is flagged. Agents cannot create or change these policies.

## Company controls

The company is in one of three states:

- **Running**: work runs normally.
- **Paused**: routine and delegated runs are held; running work is cancelled when you choose "stop running work". Chat and the Secretary keep working.
- **Quiet hours**: routine runs wait until the window ends (default 22:00 to 07:00). Runs a person starts are not held.

Only owners and admins can pause or resume. Members can read the state.

## Budgets

A budget sets a spending limit for one scope and one period, counted from the runs in that scope. As spend approaches the limit, the budget warns you. In **hold** mode, a run that would exceed the limit stays queued and is not failed. In **warn** mode, it only alerts. A new budget should start in warn mode until you have tested it.

You can extend the current period; every extension is recorded. A squad that needs more can file a budget proposal instead.

## The governance log

The log is an append-only record of every change to how the company is governed: standing-approval policies, budgets, autonomy changes, organisation changes applied or reverted, automatic knowledge publishing, forced initiative moves, pauses and resumes, activated goals and published reviews. You can filter by kind, actor or subject and export CSV, up to 50,000 rows. Nothing in the application can edit or delete an entry.

## Analytics

The **Analytics** page rolls up agent usage across the workspace: cost, tokens, run time, run count and failed runs. Every member can open it.

- **Usage** shows a trend (tokens, cost, time or runs) and a leaderboard of agents.
- **Errors** shows failed runs, the failure rate, a breakdown by failure class, and the agents with the most failures. Each failure code has a documented meaning and fix.

Costs are **estimates** in USD from published model rates, not your provider's bill. Models missing from the rate table count tokens but cost $0 until you set a custom price.

## Security boundary

Each run is a child process of the daemon and runs with the full permissions of the operating-system user that runs the daemon. Delegora does not sandbox the filesystem for you. To limit what agents can reach, run the daemon as a dedicated user, in a container, or in a virtual machine, with only the repositories and secrets the agents need. Treat every credential reachable from that environment as one the agent may use: scope tokens narrowly and prefer deploy keys for each purpose.

Run tokens (`mat_`) are created by the server for each run, bound to the user, workspace, agent and run, and expire on their own.
