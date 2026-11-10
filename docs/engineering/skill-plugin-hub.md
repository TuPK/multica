# Skill / Plugin Hub — Spec

Status: draft for review. Companion plan: [skill-plugin-hub-plan.md](skill-plugin-hub-plan.md).

## 1. Problem

Skills and plugins already exist, but they are scattered and sources are invisible:

- Skills live under Team → Resources → Skills (`/{slug}/skills`). Import accepts only
  clawhub.ai, skills.sh and github.com (`detectImportSource`, `server/internal/handler/skill.go`).
  Where a skill came from is a loose `skill.config.origin` JSONB blob, not an entity.
- Plugins are managed from a Settings tab (`plugins-tab.tsx`). There is no catalog, no file
  browsing and no editing.
- Files are stored as TEXT in Postgres (`skill_file`) and BYTEA (`plugin_package_file`). Binary
  assets, large bundles and whole-package archives have no home.
- There is no single place to answer "what skills/plugins does this system have, from where,
  at which version, who uses them".

## 2. Goals

1. A top-level **Hub** menu (rail, next to Company / Chat / Inbox) that manages every skill and
   plugin of the workspace.
2. **Sources** are first-class: list, add, sync, remove; every item shows its source.
3. **Add** items from the internet (GitHub repo/subdir, zip URL, skills.sh, ClawHub, generic
   registry index) or **upload** a custom bundle (zip / folder).
4. **Browse** as list or catalog with search, sort and filters.
5. **Detail view**: metadata, file tree, file preview, in-place edit and save with versioning.
6. **Object storage** (S3-compatible, SeaweedFS as the reference) as the central blob store.
7. Supporting capabilities listed in §9.

Non-goals (this spec): a public marketplace for third parties, paid listings, plugin runtime
changes (hooks, surfaces, MCP), cross-instance federation.

## 3. Current state (verified in repo)

| Area | Fact |
| --- | --- |
| Skill API | `/api/skills` CRUD, `/import`, `/{id}/refresh`, `/files`, `/versions`, `/labels` |
| Skill tables | `skill`, `skill_file`, `skill_version`, `agent_skill`; `skill.plugin_installation_id` (mig 368) |
| Plugin API | `/plugins`, `/plugins/packages`, `/plugins/packages/local`, `/plugins/preview`, enable/disable |
| Plugin tables | `plugin_package`, `plugin_package_version` (immutable), `plugin_package_file` (BYTEA), `plugin_installation` |
| UI | `packages/views/skills/*` (list, detail, file-tree, file-viewer, files-editor, versions); `plugins-tab.tsx` |
| Storage | `server/internal/storage`: `Storage` interface with `S3Storage` and `LocalStorage`; `S3_BUCKET`, `AWS_ENDPOINT_URL`, `S3_USE_PATH_STYLE` already supported |
| Nav | `app-rail.tsx` (Company entry), `team-panel.tsx` (Skills row) |

Constraint found in migration 392: plugin package files are **deliberately in Postgres**, so an
installed plugin is never "half-available" (manifest and code commit together). The Hub must not
regress that (see §6).

## 4. Concepts

- **Hub item** — a unified view over a `skill` or a `plugin_package`. Not a new table for content;
  a read model plus a small link table.
- **Source** — where items come from: `github`, `skills_sh`, `clawhub`, `url_zip`, `registry`,
  `upload`, `runtime_local`, `builtin`. A source has config, status and last-sync data.
- **Bundle** — the immutable archive (zip) of an item at a version, with SHA-256.
- **Scope** — `workspace` (default). System-wide sharing is out of scope for v1 (open question Q1).

## 5. Functional requirements

### 5.1 Menu and routes
- Rail entry "Hub" (icon from `@multica/ui/icons`; draw a new one per `multica-icons` skill if
  needed) → `/{slug}/hub`.
- Sub-routes: `/hub` (catalog/list), `/hub/sources`, `/hub/{kind}/{id}` (detail; kind = `skill` |
  `plugin`), `/hub/add`.
- Existing `/skills` routes redirect to `/hub?kind=skill`. The Team-panel Skills row is kept as a
  shortcut. Add `hub` to `reserved_slugs.json` and run `pnpm generate:reserved-slugs`.
- Wired in both `apps/web` and the desktop router; `tab-subject.ts` and `tab-presentation.ts`
  gain `hub` cases.

### 5.2 Sources
- List with: name, type, URL, item count, last sync, status (ok / syncing / error + message).
- Actions: add, edit, sync now, enable/disable auto-sync (interval), remove.
- Removing a source never deletes items silently: choose *keep items (detach)* or *delete items
  not used by any agent*; items in use by agents block deletion with the agent list shown.
- Every item card/row and detail shows a **source badge** and a link to the origin.

### 5.3 Add
Wizard with tabs:
1. **From URL** — GitHub repo / subdirectory / branch / tag, raw zip URL, skills.sh, ClawHub slug.
2. **From registry** — a URL serving a `hub-index.json` (schema in §7.4); lists items to pick.
3. **Upload** — zip or drag-and-drop folder (reuse `packages/core/skills/pack-archive.ts`).
   Server validates: contains `SKILL.md` (skill) or a valid plugin manifest (plugin).
4. **Create blank** — existing create-skill dialog.

All paths go through a **preview step** (name, description, file tree, detected type, scan
results, conflicts with an existing item) before commit.

### 5.4 List / catalog
- Toggle list (table) and catalog (cards); choice persisted client-side.
- Search over name, description, labels, file names; server-side with debounce.
- Sort: name, recently updated, recently added, most used, source.
- Filters: kind, source, label, status (update available, disabled, scan warning), in-use.
- Pagination or virtualization beyond 200 items.

### 5.5 Detail
- Header: name, kind, version, source badge, checksum, size, updated by/at, usage (agents).
- Tabs: **Files**, **Overview** (rendered README/SKILL.md, manifest), **Versions**, **Usage**,
  **Source** (provenance, update check).
- **Files tab**: resizable split — tree (left), preview/editor (right).
  - Preview: markdown, code (syntax highlight), JSON/YAML, images, binary fallback with download.
  - Edit: text files, Save / Discard, dirty indicator, unsaved-change guard, conflict detection
    (ETag/updated_at; 409 shows a merge/overwrite choice).
  - Tree operations: new file, new folder, rename, move, delete; `SKILL.md` cannot be deleted.
  - Saving creates a new item version (existing `skill_version` mechanism).
- Edit permissions follow `use-can-edit-skill`. Plugin **published versions are immutable**:
  editing a plugin creates a new draft version that is published explicitly.
- Items from a remote source warn that a re-sync will overwrite local edits, and offer
  *fork* (detach from source, keep local edits) or *keep tracking*.

### 5.6 Updates and sync
- Manual and scheduled sync; "update available" computed from remote commit/digest vs stored.
- Update flow shows a file-level diff before applying; local edits trigger fork-or-overwrite.

## 6. Storage architecture

Decision: **hybrid**, to keep the invariant from migration 392.

| Data | Where | Why |
| --- | --- | --- |
| Item metadata, manifest, text files in the active version | Postgres | Atomic with installs, queryable, existing code |
| Immutable bundle (zip) per version, upload staging, imported snapshots, binary assets, source caches | **S3-compatible object storage** | Size, dedupe, cheap history |
| Plugin files used at runtime | Postgres (unchanged) | No availability split |

- Object keys: `hub/{workspace_id}/{kind}/{item_id}/{version}/bundle.zip`;
  `hub/{workspace_id}/staging/{upload_id}`; assets under `.../assets/{sha256}`.
- Content-addressed by SHA-256; bundle digest stored in PG and verified on read.
- Reuse the `Storage` interface; add a `ReadWriteStorage`-style helper only if `Upload`/`GetReader`
  prove insufficient (list, copy). Presigned URLs for large download/upload.
- If `S3_BUCKET` is unset the Hub still works with `LocalStorage` (dev/self-host minimal).
- **Reference store: SeaweedFS** (Apache-2.0, S3 gateway, versioning, admin UI). RustFS
  (Apache-2.0, 1.0 only in Sept 2026) is a watch candidate; Garage (AGPLv3, no Object Lock) and
  Ceph (heavy) are not recommended. MinIO CE is archived and must not be a default. Selection
  is by config only — no code depends on the vendor. Re-verify release status at implementation.
- Deploy: add an optional `seaweedfs` service to `docker-compose.selfhost.yml` and the helm
  chart behind a profile; bucket bootstrap in `docker/entrypoint.sh` or an init job.

## 7. Data model (new migrations; no FKs, concurrent single-statement indexes)

### 7.1 `hub_source`
`id, workspace_id, name, type, url, config JSONB (branch, subdir, auth_ref, interval), enabled,
auto_sync_interval_sec, last_sync_at, last_sync_status, last_sync_error, item_count, created_by,
created_at, updated_at`. Unique `(workspace_id, name)`.

### 7.2 `hub_item`
Link/read-model row per skill or plugin package:
`id, workspace_id, kind ('skill'|'plugin'), ref_id (skill.id | plugin_package.id), source_id,
source_ref (path/commit/slug), source_digest, tracking ('tracking'|'forked'|'manual'),
latest_version, bundle_key, bundle_digest, size_bytes, file_count, scan_status, scan_report JSONB,
created_at, updated_at`. Unique `(workspace_id, kind, ref_id)`.
Backfill from `skill.config.origin`, `plugin_package` and runtime-local skills (a data migration
run in app code, idempotent).

### 7.3 `hub_item_version_blob`
`id, item_id, version, bundle_key, digest, size_bytes, created_by, created_at`. Immutable.

### 7.4 Registry index (`hub-index.json`)
```json
{ "schema": 1, "name": "Acme Hub", "items": [
  { "kind": "skill", "name": "…", "description": "…", "version": "1.2.0",
    "url": "https://…/x.zip", "sha256": "…", "tags": ["…"] } ] }
```

## 8. API (all workspace-scoped; zod schema + `parseWithFallback` on the client)

- `GET /api/hub/items?q=&kind=&source=&label=&sort=&cursor=` — unified list
- `GET /api/hub/items/{kind}/{id}` — detail incl. source, usage, scan
- `GET/PUT/DELETE /api/hub/items/{kind}/{id}/files[...]`, `POST .../files/move`
- `GET /api/hub/items/{kind}/{id}/bundle` — presigned download
- `GET/POST/PATCH/DELETE /api/hub/sources`, `POST /api/hub/sources/{id}/sync`
- `POST /api/hub/import/preview`, `POST /api/hub/import/commit`
- `POST /api/hub/uploads` (initiate, presigned PUT) and `POST /api/hub/uploads/{id}/complete`
- `POST /api/hub/items/{kind}/{id}/fork`, `.../check-update`, `.../apply-update`

Rules: handlers use `loadSkillForUser`-style loaders, `parseUUIDOrBadRequest`, and filter by
`workspace_id`. Existing `/api/skills` and `/api/plugins` stay; hub handlers delegate to the same
services. Unknown enums get fallbacks for older desktop clients.

## 9. Supporting capabilities

- **Security scan on import/upload**: path traversal and zip-slip, symlinks, size/file-count/zip
  bomb limits, executable bit, secret patterns, suspicious shell (`curl | sh`), network hosts in
  scripts. Result `clean | warn | blocked`; `blocked` cannot be installed; `warn` needs confirm.
- **SSRF-safe fetching**: allowlist or resolve-and-block private/loopback ranges; redirect caps;
  per-fetch deadline below proxy timeout (existing 45s pattern); size cap while streaming.
- **Private repos**: per-source credential stored encrypted (reuse the secret mechanism used by
  plugin secrets); never returned to the client.
- **Integrity**: SHA-256 per file and per bundle; version pinning for plugins.
- **Permissions**: view (member), edit (existing skill rules), manage sources/delete (admin).
- **Audit/activity log**: add, sync, edit, fork, delete, with actor.
- **Usage**: which agents/squads use an item; "in use" guard on delete.
- **Labels** reuse the existing skill label system for all kinds.
- **Export/backup**: download bundle; export all items as one archive; bucket lifecycle docs.
- **Quotas**: per-workspace storage and item caps (configurable).
- **Observability**: metrics for sync duration/failures, storage bytes; background sync via the
  existing `scheduler` (see `docs/maintenance-jobs.md`).
- **i18n**: all copy in every supported locale (en, es, fr, ja, ko, zh-Hans, zh-Hant); follow
  `conventions.mdx` (UI descriptions optional, no restating titles).
- **Docs**: Fumadocs page for the Hub and for configuring object storage.

## 10. Non-functional

- Large trees (1k files) render with virtualization; file preview capped at 1 MB text, larger
  files offer download.
- Upload limit default 50 MB per bundle, configurable.
- No backwards-compat shims; API changes keep response compatibility (AGENTS.md).
- State: TanStack Query for server data; Zustand only for view mode, filters, editor drafts
  (stores in `packages/core`, not `views`).

## 11. Testing strategy

- Go: handler tests with `testutil`/`dbfx`; importer unit tests per source with httptest servers;
  scanner table tests (zip-slip, bombs, secrets); storage tests against `LocalStorage`, and an
  opt-in S3 test against a SeaweedFS container.
- Core: query/mutation tests, zod malformed-response tests.
- Views: component tests for list/catalog, source manager, add wizard, file editor (dirty/conflict).
- E2E (Playwright): add source → sync → open item → edit file → save → version appears.
- Default tests must not run user-installed agent CLIs (repo rule).

## 12. Decisions (resolved)

- Q1: Hub is **workspace-scoped** only. No instance-level tier in v1.
- Q2: Plugins **are editable in the UI** via draft versions; published versions stay immutable.
- Q3: Self-host compose **runs SeaweedFS by default**; `LocalStorage` remains the fallback when
  no `S3_BUCKET` is configured (dev, minimal installs).
- Q4: No hosted public registry in v1; only user-provided registry URLs (`hub-index.json`).
  Revisit after v1 if demand appears.
