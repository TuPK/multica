---
title: Agents
order: 4
---

An agent is a team member in the workspace: you assign it issues, mention it in comments, or chat with it directly. It does not run in the background. It keeps an identity and configuration, and produces a run only when work arrives.

### Creating an agent

A runtime must be online first. The runtime decides which machine and AI tool executes the work. There are two ways to create one:

- **Let AI write it**: the builder asks for the key details and fills the configuration; you can edit the draft beside the conversation.
- **Fill it in yourself**: only a name (unique in the workspace) and a runtime are required.

The description is a short introduction for the team and is never sent into the prompt. The **instructions** are what the agent reads on every run. Say what it is responsible for and what it is not, what to check first, what it may change, how to deliver results, and when to ask a person.

### Configuration

- **Model and thinking level**: leave blank to use the tool's default. Each tool has its own model list; after switching tools, pick the model again.
- **Skills**: attach reusable methods. Put requirements that belong only to this agent in the instructions.
- **Concurrency limit**: how many runs it can run at once, default 6. Extra runs wait in the queue.
- **Environment variables**: injected when the tool starts. Use only narrow, low-privilege credentials, such as a read-only API key. Do not use production database admin passwords.
- **Custom arguments**: appended to the tool's launch command. Do not put secrets here; they can be visible to other local processes.
- **MCP servers**: configure them for this agent, or pick from the workspace's shared MCP library.
- **Integrations**: connect external services the agent may use.

Changes do not affect runs already in progress. Later runs use the saved configuration.

### Access

Every agent has an owner and an access setting:

- **Only me** (default): only the owner can run it.
- **Entire workspace**: every member can run it.
- **Specific people**: the owner and the members you choose.

Only the owner changes access. Owners and admins manage the rest of the configuration, but cannot use their role to run an agent they were not granted.

### MCP servers and secrets

Configuration that may contain tokens is stored on the server and is not shown again after saving. To change it, click **Replace configuration** and enter the complete new version. Deleting a server from the shared library removes it from every agent it was assigned to. To stop one agent using it, switch it off on that agent's MCP tab.

### Duplicating

Duplicating keeps most of the configuration (instructions, skills, access, environment variables, MCP servers) and changes only the name. Assignments of shared MCP servers are not copied and must be set again.

### Archive and restore

Archiving stops new assignments and mentions, and cancels unfinished runs. History is kept, and you can restore the agent later.

### Status

The agent list shows two things side by side: **availability** (online, offline or unstable, from the runtime) and **workload** (working, queued or idle, from runs). An offline agent is not deleted; queued runs wait until the runtime returns.

### Conversation starters

Add up to three starters, each with a short label and a full prompt, so people know what the agent is good at. Selecting one fills the input without sending.

### Creating with the CLI

```bash
delegora agent create --name "Frontend reviewer" --runtime-id <runtime-id> \
  --description "Reviews frontend pull requests" \
  --instructions "Read the diff and tests first; post conclusions only as issue comments."
```

Use `delegora agent copy <agent-id>` to duplicate an existing agent.
