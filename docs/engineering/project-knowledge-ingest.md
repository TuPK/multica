# Project data into the knowledge base: findings and proposed mechanism

Status: research and proposal. Step 2 of section 4 (repo docs through the daemon, option A) is built; the rest of section 3 is not.
Reviewed against the project `mua_ngon_ecommerce` in workspace `tupk`, 2026-11-09.

## 1. What a project owns today

| Source | `mua_ngon_ecommerce` | Where it lives | In the KB today |
|---|---|---|---|
| Documents in the repository | 51 curated Markdown files (PRD, architecture, epics, UX, 10 epic specs, retros, rulings), about 5 MB under `docs/`. Another 156 files under `.agents/skills` and 25 under `_bmad` are tooling, not project knowledge. | A local directory, `/home/av-server/Documents/Github/mua_ngon_ecommerce`, attached as a `local_directory` project resource (`execution_mode: in_place`, bound to one daemon). The GitHub remote `TuPK/mua_ngon_ecommerce` is private. | No |
| Issues | 61 issues across 7 statuses, each with a goal and acceptance text | Multica DB | No |
| Issue comments | 385 comments: agent reports, QA verdicts, rulings, retros | Multica DB | No |
| Attachments | 0 | Multica DB | No |
| Project resources | 1 (the local directory) | `project_resource` | No |

The project is the richest knowledge source in the company and none of it reaches the KB.

## 2. Why the current mechanisms do not cover it

1. **Repo-docs sync is workspace-wide, not per project.** One repository, one base (`engineering`), one docs path per workspace (`knowledge_repo_sync`).
2. **It needs a public https URL.** The project's repository is private and its working copy is a local path. The sync clones with plain `git` and has no credentials.
3. **It would reject every file.** It writes kind `reference`, which requires a `Summary` heading. 0 of the 51 project docs have one. They would all be listed as failed.
4. **Issues and comments are never read.** Agents may call `multica knowledge add`, but only by choosing to. That is the manual path you want to remove.
5. **Before this change there was no per-project place to put anything.** All documents of all projects shared `engineering` and `product`.

## 3. Proposed mechanism: one ingest pipeline, several sources

Principle: a project's base is a *projection* of the project. Nobody uploads. Each source is registered once per project, and a worker keeps the base in step with it. Writes use the existing document path (`knowledge_link` with an idempotency key, content hash, replace on change, archive on removal), so retries and re-runs are safe.

### 3.1 Sources

| Source | Trigger | Document produced | Kind |
|---|---|---|---|
| **Repo docs** of the project | Daily, plus a webhook or "sync now" | One document per Markdown file under the configured paths (default `docs/`, `README.md`, `AGENTS.md`). Excludes `.agents`, `_bmad`, `node_modules`, `dist`. | `reference`, or a mapped kind (for example `prd.md` to `product_spec`) |
| **Issue digest** | When an issue reaches a terminal status (done, cancelled, stage_done) | One document per issue: title, goal, final status, and the comments that carry a verdict or decision. Bot chatter is dropped. | new kind `issue_record` |
| **Decisions and rulings** | When a comment is marked or recognised as a ruling, or an initiative gate is decided | One document per decision | existing `decision` kinds |
| **Project resources** | When a resource is attached | Files from the resource, same as repo docs | `reference` |
| **Attachments** | On upload to an issue of the project | The file, through the upload path | by file type |

### 3.2 Where the files are read from (the part that decides feasibility)

The server cannot read the local directory, because the path belongs to the daemon's machine. Three options:

- **A. Daemon reads, server stores (recommended).** The daemon already runs on the machine that holds the project directory and already receives the project's `local_directory` resource. Add a daemon task "collect docs": it walks the configured paths, applies the filters, and posts `{path, sha256, content}` batches to a new server endpoint. The server compares hashes and writes only changes. This works for private repos and for any project without a remote.
- **B. Server clones with a credential.** Store a deploy token per project. Works without a daemon but needs secret storage and does not cover local-only projects.
- **C. Agents push.** Rely on agents running `multica knowledge add`. Already possible, but it is the manual route and is not reliable.

Recommendation: A for files, with B as an optional second source for projects that have a remote and no daemon.

### 3.3 Fixing the `Summary` problem

Do not force project authors to add headings. For project bases, the ingest derives the `Summary` from the first paragraph or, when a chat model is configured, a one-line generated summary, and records the source path in the document's metadata. Kind `reference` stays strict for hand-written documents.

### 3.4 Isolation and access

- Each project has its own base (`project-<12 hex of id>`, built in this change). Search can be scoped to it, and the automatic issue context should search the issue's project base first, then the company bases.
- A project base inherits the project's visibility. Private projects get a restricted base.
- Archiving or cancelling a project archives its base; deleting a project queues its documents for removal.

### 3.5 Making it automatic

- When knowledge is ready and a project has a `local_directory` or `github_repo` resource, the project gets a default repo-docs source with the default paths. No setting is needed.
- A job reuses the hourly auto-enable pass: for each ready workspace, for each project, ensure the base and run due sources.
- The Knowledge page shows, per project: sources, last run, document count, and failures.

## 4. Suggested order of work

1. **Done in this change:** knowledge on by default for new workspaces, retry of failed ones, one base per project, higher WeKnora quotas.
2. Per-project repo-docs source through the daemon (option A) with derived `Summary`. This alone brings the 51 curated docs of `mua_ngon_ecommerce` into `project-25cbad1aa0ee`.
3. Issue digest for terminal issues (the 24 `stage_done` and 27 `done` issues hold most of the decisions).
4. Automatic issue context scoped to the project base.
5. Project status panel on the Knowledge page.

## 5. Open questions for the owner

- Should private projects get restricted bases by default?
- Is option A acceptable, meaning the daemon reads project files and uploads them to the server?
- Which comments count as knowledge: all comments by agents with a QA or architect role, or only ones tagged by a person?

## 6. Built: project docs through the daemon (step 2)

- **Daemon** (`server/internal/daemon/project_docs.go`): 90 seconds after start, then every 30 minutes, it reads each `local_directory` resource pinned to it. It takes `README.md`, `AGENTS.md`, `CLAUDE.md` and every `.md` under `docs/`. It skips symlinks, hidden and tooling directories (`node_modules`, `dist`, `build`, `vendor`, `target`, `coverage`), empty files and files over 1 MiB, with a cap of 500 files. A scan that finds nothing is an error, never an empty manifest. Set `MULTICA_PROJECT_DOCS_SYNC=false` to turn it off on a machine.
- **Protocol** (`/api/daemon/workspaces/{ws}`): `GET /knowledge/project-sources`, `POST /projects/{id}/knowledge/manifest` (path and hash of every file; the server removes documents of files that left and answers with the paths it needs), `POST /projects/{id}/knowledge/docs` (the needed files).
- **Server** (`knowledge_project_docs.go`): writes untyped, published documents into the project's base, so no `Summary` heading is needed. A changed file replaces its document. Tracking is in `knowledge_project_file` (migrations 914 and 915), cleaned up when a workspace is deleted.
- **Measured on `mua_ngon_ecommerce`:** the scan finds 66 files, 0.8 MB.
- **Not built:** issue digests, decisions, attachments, a Knowledge page panel per project, and project-scoped automatic issue context.
