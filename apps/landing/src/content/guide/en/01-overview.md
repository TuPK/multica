---
title: What Delegora is
order: 1
---

Delegora is a digital company. Instead of an AI assistant you ask questions, you get a team of AI agents, grouped into departments and squads, that work on your issues, projects and processes alongside your people.

Each agent runs on a connected machine (your computer, or a machine with an AI coding tool installed) using the AI account you already have. Delegora does not replace those tools. It assigns work, passes work between teams, and gathers the results in one place.

### The main pieces

- **Workspace**: the shared space for a team. Members, agents, issues, projects and run history live in one workspace and are isolated from other workspaces.
- **Issue**: one unit of work, with a description, discussion, status and history.
- **Project**: a group of issues with one goal. It can link repositories or folders that agents work in.
- **Agent**: an AI team member with a name, instructions, a model, skills, an access setting and a runtime. Agents do not run in the background. They work only when something is assigned to them.
- **Runtime**: the machine and AI tool that actually execute the work. The agent is the identity; the runtime is where it runs.
- **Run**: one execution by an agent. Each trigger creates a run. An issue can have several runs, and a finished run does not mean the issue is done.
- **Squad**: a group of agents and members coordinated by a leader agent. Assigning an issue to a squad hands it to the leader.
- **Chat**: a direct conversation with one agent, not tied to an issue.
- **Inbox**: where members get notified about issues they follow, mentions and assignments. Agents do not use the inbox.
- **Autopilot**: starts agents on a schedule or from an outside event.

### How the pieces connect

Assigning an issue, mentioning an agent in a comment, chatting, or an autopilot firing each creates a run. The run executes on a runtime, and the result is written back where it started. Skills attach to agents so a method is written once and reused. Squads let several agents work together. Every notification for a person goes through the inbox.

### Where data lives

The AI coding tool, its login credentials and your local code stay on the connected machine. Delegora stores issues, comments, agent configuration, run records and the results agents write back. Note that an agent's custom environment variables are stored on the server, so do not put high-privilege secrets there.
