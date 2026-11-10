# Skill / Plugin Hub — Implementation Plan

Spec: [skill-plugin-hub.md](skill-plugin-hub.md). Each phase ships independently behind a feature
flag `hub_v1` (see `server/pkg/featureflag`), ends green on its checks, and is split into atomic
conventional commits. Migration numbers continue from 926.

Checks per phase (from repo root): `make sqlc` after SQL, `make test` (Go), `pnpm typecheck`,
`pnpm lint`, `pnpm test`; `pnpm exec playwright test` at phases 3 and 6.

## Phase 0 — Decisions and groundwork (0.5 day)
- [x] Resolve open questions Q1–Q4 (workspace-only; plugin edit in UI; SeaweedFS default in
      compose; user-provided registries only).
- [x] Spike done (2026-11-10): SeaweedFS 4.53 S3 gateway; the existing `S3Storage` did
      Upload / GetReader / PresignGet / DeleteObject against it unchanged (path-style, SigV4).
      Still unverified: list-prefix, copy, presigned PUT (needed in Phase 4).
- [x] `seaweedfs` service added to `docker-compose.selfhost.yml` (image pinned 4.53, credentials
      from `HUB_S3_*`, started and health-checked). It uses its own `HUB_S3_*` settings, NOT
      `S3_BUCKET`: setting the global S3 vars would redirect existing attachment reads.
- [ ] Re-check upstream licence/maturity of SeaweedFS and RustFS before Phase 4 ships.

## Phase 1 — Menu, routes, unified read model (read-only Hub) (2–3 days)
Scope change (implemented): Phase 1 is client-side only. The Hub merges the existing skill list
and plugin-package list in `packages/core/hub/items.ts`, deriving sources from
`skill.config.origin`. The `hub_source` / `hub_item` tables, backfill and `/api/hub/*` read
endpoints move to Phase 3, where writes (add/upload/sync) first need them. Reason: no new
schema or backfill is needed to ship a usable read-only Hub. The backend list below is therefore
deferred.
Backend (deferred to Phase 3)
- [ ] Migration 927 `hub_source`, 928 `hub_item` (+ each index in its own `CONCURRENTLY` file),
      down migrations, queries in `server/pkg/db/queries/hub.sql`, `make sqlc`.
- [ ] Idempotent backfill service: create `hub_item` for every `skill` (source from
      `config.origin`) and `plugin_package`; synthesize `hub_source` rows per origin type.
      Trigger on startup/maintenance job and on skill/plugin create paths.
- [ ] Handlers `GET /api/hub/items`, `GET /api/hub/items/{kind}/{id}`, `GET /api/hub/sources`
      with search, sort, filters, cursor. Go tests with `dbfx`.
Core / views / apps
- [ ] `packages/core/hub/`: types, zod schemas (`schema.ts` + malformed tests), queries, keys
      with `wsId`, view-mode/filter Zustand store (persisted prefs only).
- [ ] `packages/core/paths`: `hub()`, `hubSources()`, `hubItem(kind,id)`; update reserved slugs
      (`reserved_slugs.json` → `pnpm generate:reserved-slugs`), `tab-subject`, `tab-presentation`.
- [ ] `packages/views/hub/`: `HubPage` (list + catalog toggle, search, sort, filters, source
      badge), `HubSourcesPage` (read-only), empty states. Reuse `collection-page` /
      `list-column` layout primitives.
- [ ] Rail entry in `app-rail.tsx`, icon via `multica-icons` skill, compact-nav-sheet entry,
      web route under `apps/web/app/[workspaceSlug]/(dashboard)/hub`, desktop router entry.
- [ ] Redirect `/skills` → `/hub?kind=skill`; keep Team-panel row.
- [ ] i18n: `locales/*/hub.json` for all 7 locales + `resources-types.ts`; copy per conventions.
- Exit: Hub lists existing skills and plugins with correct sources; flag off hides the menu.

## Phase 2 — Detail view, file tree, preview, edit (3–4 days)
- [ ] Unified file API `GET/PUT/DELETE /api/hub/items/{kind}/{id}/files`, `POST .../files/move`,
      delegating to existing skill file service; plugin files read-only from `plugin_package_file`.
- [ ] Optimistic-lock: `updated_at`/ETag on file PUT, 409 on mismatch; tests.
- [ ] `HubItemPage`: header, tabs (Files, Overview, Versions, Usage, Source).
- [ ] Files tab: extract/reuse `file-tree`, `file-viewer`, `skill-files-editor`; add virtualized
      tree, rename/move/new folder, unsaved guard, conflict dialog, image/JSON/code preview
      (reuse the editor/markdown stack in `packages/views/editor`).
- [ ] Versions tab reusing `skill-versions-tab`; Usage tab from `agent_skill` + squad skills.
- [ ] Plugin items: manifest viewer; "edit" creates a draft version (per Q2) published via
      `PublishPluginPackage`; immutability preserved.
- Exit: browse, edit and save a skill file from Hub, version recorded; plugin files viewable.

## Phase 3 — Add flows: URL, registry, upload, preview (4–5 days)
- [ ] `server/internal/hub/` package: `Importer` interface; implementations `github`
      (repo/subdir/ref via codeload zip), `zipurl`, `skills_sh`, `clawhub` (move existing fetchers
      out of `skill.go`, keep behaviour), `registry` (parses `hub-index.json`).
- [ ] Safe fetcher: SSRF guard (block private/loopback after DNS resolve, cap redirects),
      streaming size cap, deadline < proxy timeout, zip-slip/symlink/bomb protection.
- [ ] Scanner (`server/internal/hub/scan`): rules per spec §9; table tests.
- [ ] `POST /api/hub/import/preview` + `/commit` (commit is idempotent on digest), conflict
      handling (same name → rename/replace/skip).
- [ ] Upload: `POST /api/hub/uploads` + `/complete` (S3 presigned PUT, local fallback);
      validate zip server-side; produce preview the same way.
- [ ] Source CRUD endpoints, private-repo credentials encrypted at rest.
- [ ] UI: `AddToHubDialog` wizard (URL / Registry / Upload / Blank), preview step with tree and
      scan result, progress and error states; drag-and-drop folder via `pack-archive.ts`.
- [ ] E2E: add from a local fixture registry and by zip upload.
- Exit: skills and plugins can be added from all source types with scan results.

## Phase 4 — Central object storage (3 days)
- [ ] Extend `storage` with only what is needed (list prefix, copy, presign PUT) on `S3Storage`
      and `LocalStorage`; tests incl. opt-in SeaweedFS integration test.
- [ ] Migration 929 `hub_item_version_blob`; write immutable bundle on every version/commit;
      verify digest on read; `GET .../bundle` presigned download.
- [ ] Backfill bundles for existing items (maintenance job, resumable, idempotent).
- [ ] Deploy: optional `seaweedfs` service + bucket init in `docker-compose.selfhost.yml`, helm
      values, `.env.example` docs; no vendor-specific code; default stays LocalStorage unless
      configured (per Q3).
- [ ] Runbook in `docs/engineering/runbooks/` (backup, bucket lifecycle, restore).
- Exit: bundles in SeaweedFS, digest-verified; switching endpoint requires config only.

## Phase 5 — Sources management, sync, updates (3–4 days)
- [ ] Source manager UI: add/edit/remove, status, item count, sync now, auto-sync interval,
      delete-with-items choices and in-use guard.
- [ ] Sync worker on the existing `scheduler`: fetch, compare `source_digest`, record
      `last_sync_*`, never overwrite forked/locally-edited items silently.
- [ ] `check-update` / `apply-update` with file diff view; `fork` action; tracking states.
- [ ] Metrics and logs for sync duration/failures.
- Exit: update-available badge, diff, apply or fork all work end to end.

## Phase 6 — Hardening, permissions, docs, release (2–3 days)
- [x] Activity log: server `GET /api/hub/activity`; Activity tab on each item and a workspace Activity tab on the Hub.
- [ ] Permissions matrix tests (member view, editor edit, admin manage). Server tests exist in `hub_permissions_test.go`; matrix coverage was not reviewed in this pass.
- [ ] Quotas (storage bytes, item count) with clear errors. Limits are enforced server-side (`HUB_MAX_ITEMS_PER_WORKSPACE`, `HUB_MAX_STORAGE_BYTES_PER_WORKSPACE`) and the Hub shows a usage meter. Not done: the `quota_exceeded` error is not yet shown in the add, sync, or update flows.
- [x] Export bundle / export all: per-item "Download bundle" and admin-only "Export all" on the Hub.
- [ ] a11y pass on tree and editor (`accessibility-scan`), keyboard navigation. Done: the file tree is an ARIA tree with arrow, Home/End, and Enter keys, a roving tab stop, and labelled editor and preview regions. Not done: `accessibility-scan` was not run (no dev server in this pass), and dialog focus return was not checked.
- [x] Fumadocs pages (en + zh) for Hub and object-storage setup (`hub`, `hub-object-storage`); whats-new entry for the Hub (version 0.8.2 is unconfirmed; hidden until a server at that version connects).
- [ ] Full `make check`, Playwright suite, remove `hub_v1` flag after soak.

## Risks and mitigations
| Risk | Mitigation |
| --- | --- |
| Object store vendor churn (MinIO precedent) | Vendor-neutral S3 config; no vendor code; documented swap |
| Splitting data across PG and S3 breaks plugin availability | Hybrid rule: runtime plugin files stay in PG; S3 holds immutable bundles only |
| Malicious or oversized imports | Scanner, SSRF guard, caps, `blocked` status, confirm on `warn` |
| Local edits lost on sync | Tracking/forked model, diff before apply, never silent overwrite |
| Scope creep | Phases ship independently; Q1/Q4 deferred items stay out until decided |
| Go tests share one DB | Assert on own fixture rows (see memory: go-test-db-leftovers) |

## Rough estimate
~18–24 working days for one engineer; Phase 1–2 alone gives a usable Hub.
