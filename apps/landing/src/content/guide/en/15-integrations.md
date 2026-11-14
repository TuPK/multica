---
title: Integrations
order: 15
---

Integrations let the team ask an agent questions, mention it in a group chat, or create issues from a chat app, without opening Delegora.

## Chat apps

Slack, Telegram, Lark (Feishu), WeCom and DingTalk are supported. Each bot is bound to one agent; for several agents, create one bot per agent. An admin connects the bot with the credentials from the chat platform, then each member links their Delegora account through a one-time link that expires after 15 minutes. Only members of the workspace can use the bot.

- **Direct messages**: message the bot directly; no mention needed.
- **Groups and channels**: only messages that mention the bot are processed. Each thread has its own session, and follow-ups need a fresh mention.
- **Attachments**: images and files reach the agent, with per-platform limits (for example 20 MiB per file and 10 files per message on Slack).

Commands in chat:

- `/issue <description>`: creates an issue from the message and assigns it to the bot's agent.
- `/new`: starts a new conversation with the agent. `/new <message>` starts one and sends the message.
- `/clear`: keeps the full history but makes the agent forget the earlier context.
- `/brief` and `/decisions`: show the latest brief and the decisions waiting for you (see the Chat and Secretary chapter).

To disconnect, go to **Settings → Messaging**. This stops the bot receiving new messages; existing conversations are kept.

## GitHub

Connect GitHub to link pull requests to issues automatically, show their checks and mergeability, and move issues when their PRs merge. Details are in the Projects and repositories chapter.

## Knowledge service

The company knowledge base is connected and managed from the Knowledge page. Details are in the Knowledge, skills and Hub chapter.

## MCP

Agents can use the MCP servers you configure, either per agent or from the shared workspace library. Details are in the Agents chapter.

## Public API (v1)

The v1 API lets your own systems integrate with Delegora using a personal access token or an application token. An OpenAPI specification is available for generating clients. Never put a token in source code; load it from a secret manager.

## Webhooks

An autopilot can receive events from other systems and run the matching work. See the Workflows and Autopilot chapter.
