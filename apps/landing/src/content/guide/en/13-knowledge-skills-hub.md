---
title: Knowledge, skills and the Hub
order: 13
---

## Knowledge base

The knowledge base keeps the company's durable record: policies, research, product documents and decisions. Agents search it before non-trivial work and cite what they use. Only owners and admins can enable it. When the knowledge service is offline, agents keep working without company knowledge until it returns.

### Drafts and publishing

Writing and publishing are separate. Members and agents create **drafts**, which are saved but not searchable. An owner or admin presses **Publish** to make one searchable, or **Discard** to remove it. Proposal decisions and briefs are written to the decisions base automatically, so the history is always searchable.

Each knowledge base and each document kind can have a **publish policy**: a person publishes (the default), drafts publish after a review, or drafts publish at once. Squads below autonomy level 2 never publish automatically.

Documents have kinds, such as validation report, analysis, launch plan, retrospective or metrics report. Each kind lists the headings a draft must include; a draft missing one is refused.

### Uploading and importing

Upload a file or import a page from the Knowledge page, or click **Save to knowledge** on an issue to save its content as a draft. A new document is published as soon as it is added, if the policy allows.

Each base has its own access: limit it to specific squads, floors or agents. The Wiki is limited to three bases.

### Repository sync

A repository's `docs` folder can sync into the engineering base once a day. Each Markdown file becomes a reference document, and each file needs a **Summary** heading.

### Retrieval tests

Add a question and the document it should return. The system measures the hit rate and reports it in the brief. Search always returns its closest matches even when none is relevant, so check each result against your question.

## Skills

A skill is a reusable set of working methods in a workspace: a main `SKILL.md` file, optionally with scripts, templates and reference material. Attach it to an agent and the agent applies the guidance whenever the work is related, instead of copying the same method into every agent's instructions.

### Creating or importing

On the **Skills** page, click **New skill**. You can write one directly, import from GitHub, from a `.zip` or `.skill` archive, from skills.sh or ClawHub, or copy one from a runtime (a snapshot at that moment). If a skill with the same name exists, choose to overwrite, save under a new name, or skip.

A skill imported from a source can **update from source**. This keeps its identity and its agent assignments but replaces the name, description, `SKILL.md` and supporting files. Local edits are overwritten.

Attach skills from the skill page (**Add to agent**) or from the agent's Skills tab. When a skill is not needed for now, disable the assignment instead of deleting it. Changes apply from the next run.

Any member can create or import a skill. The creator and owners or admins can edit or delete it. Deletion is permanent and removes the skill from every agent.

## Hub

The Hub lists every skill and plugin in the workspace, with its source, version, and whether a newer version exists. Owners and admins manage the **sources** (GitHub, zip URLs, registry indexes). A token for a private repository is encrypted and never shown again.

Every import is scanned before anything is installed. The scan reports **No issues**, **Warnings** or **Blocked**. Blocked imports cannot be installed. Warnings must be acknowledged before the import is saved.

Published plugin versions do not change; edits form a draft until an owner or admin publishes a new version. When a source has a newer version, choose **Overwrite** (your local edits are lost) or **Fork** (keep your edits and stop receiving updates).

The default quota is 500 items and 1 GiB per workspace. An uploaded archive can be up to 16 MiB.
