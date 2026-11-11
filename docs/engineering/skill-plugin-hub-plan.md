# Skill / Plugin Hub — Implementation Plan

Spec: [skill-plugin-hub.md](skill-plugin-hub.md). Each phase ships independently behind a feature
flag `hub_v1` (see `server/pkg/featureflag`), ends green on its checks, and is split into atomic
conventional commits. Migration numbers continue from 926.

Status legend: `[x]` done, `[ ]` not done, `[-]` consciously dropped (the reason is given).
Items marked "not verified in this pass" are present in the tree but were not re-checked, so
they are not claimed as done beyond that.

Dropped from the plan (decided, not forgotten):
- The `/skills` → `/hub?kind=skill` redirect. `/skills` is still its own page, and the Team
  panel row still links to it.
- The `hub_v1` feature flag. It was never added to `server/pkg/featureflag`, so the Hub ships
  without a flag and there is nothing to remove after soak.
- The presigned PUT upload (`POST /api/hub/uploads` + `/complete`). Uploads go through the
  multipart preview route, `POST /api/hub/import/preview`, which is capped at 16 MiB. A presigned
  PUT would add a second write path and a client-side completion step for no gain at current
  sizes. The storage layer still implements `PresignPut`, but no handler uses it.

Checks per phase (from repo root): `make sqlc` after SQL, `make test` (Go), `pnpm typecheck`,
`pnpm lint`, `pnpm test`; `pnpm exec playwright test` at phases 3 and 6.

## Phase 0 — Decisions and groundwork (0.5 day)
- [x] Resolve open questions Q1–Q4 (workspace-only; plugin edit in UI; SeaweedFS default in
      compose; user-provided registries only).
- [x] Spike done (2026-11-10): SeaweedFS 4.53 S3 gateway; the existing `S3Storage` did
      Upload / GetReader / PresignGet / DeleteObject against it unchanged (path-style, SigV4).
      List-prefix and copy were added in Phase 4 and are unit-tested; the opt-in SeaweedFS
      integration test (`s3_integration_test.go`) was not run in the latest pass.
- [x] `seaweedfs` service added to `docker-compose.selfhost.yml` (image pinned 4.53, credentials
      from `HUB_S3_*`, started and health-checked). It uses its own `HUB_S3_*` settings, NOT
      `S3_BUCKET`: setting the global S3 vars would redirect existing attachment reads.
- [x] Re-checked licence/maturity (2026-11-11, from secondary sources, not the upstream release pages): the SeaweedFS core is Apache-2.0 (its enterprise edition is under a separate EULA; we pin the community image 4.53); RustFS 1.0 went GA on 2026-09-16 under Apache-2.0 and stays a watch candidate.

## Phase 1 — Menu, routes, unified read model (read-only Hub) (2–3 days)
Scope change (implemented): Phase 1 is client-side only. The Hub merges the existing skill list
and plugin-package list in `packages/core/hub/items.ts`, deriving sources from
`skill.config.origin`. The `hub_source` / `hub_item` tables, backfill and `/api/hub/*` read
endpoints move to Phase 3, where writes (add/upload/sync) first need them. Reason: no new
schema or backfill is needed to ship a usable read-only Hub. The backend list below is therefore
deferred.
Backend (deferred to Phase 3)
- [x] Migrations for `hub_source` (927, with its indexes in 928–929), `hub_item` (930, indexes
      in 931–933), `hub_item_version_blob` (934–937), update state (935), auto-sync index (940)
      and activity (950–952). The numbers differ from the plan: each index is its own
      `CONCURRENTLY` file, and each has a down migration. Queries are in `hub*.sql`, and sqlc is
      generated.
- [-] Idempotent backfill service and startup or maintenance job. Dropped in favour of lazy
      creation: `GET /api/hub/items` creates a row the first time it lists a skill or plugin,
      and the bundle route does the same when it is asked. Nothing is backfilled in bulk, so
      there is no maintenance job, and no create path triggers a row.
- [x] Handlers `GET /api/hub/items/{kind}/{id}` (built in the completion pass). `GET
      /api/hub/items` and `GET /api/hub/sources` exist, but neither takes search, sort, filter
      or cursor parameters. Go tests with `dbfx` exist for the list and the sources.
Core / views / apps
- [x] `packages/core/hub/`: types, queries, keys and the client exist, with tests. Not verified
      in this pass: the zod schemas and the view-mode store (G3's area).
- [x] `packages/core/paths`: `hub()`, `hubSources()`, `hubItem(kind,id)` exist. Reserved slugs,
      `tab-subject` and `tab-presentation` were not re-checked in this pass.
- [x] `packages/views/hub/`: `HubPage`, `HubSourcesManager`, `HubItemPage` and the add, update,
      fork and export dialogs exist, with tests. Not verified in this pass: search, sort and the
      source badge.
- [x] Rail entry in `app-rail.tsx` and the web route under `(dashboard)/hub`. Desktop router entry
      and compact-nav-sheet entry not re-checked.
- [-] Redirect `/skills` → `/hub?kind=skill`. Dropped (see the note at the top); `/skills` keeps
      its own page.
- [x] i18n: `locales/{en,es,fr,ja,ko,zh-Hans,zh-Hant}/hub.json` exist. Copy was not re-reviewed.
- Exit: Hub lists existing skills and plugins with correct sources; flag off hides the menu.

## Phase 2 — Detail view, file tree, preview, edit (3–4 days)
- [x] Unified file save `PUT /api/hub/items/{kind}/{id}/files` with an etag check. Reads, deletes
      and moves stay on the existing skill and plugin-version routes (`packages/core/hub/files.ts`);
      a second server path for them would add nothing, so GET/DELETE/move under `/api/hub` were dropped.
- [x] Optimistic lock: the etag is the row's RFC3339Nano `updated_at`; a stale or missing etag on an existing file returns 409 `file_conflict` with the current etag (checked under the skill row lock).
- [x] `HubItemPage` exists (`hub-item-page.tsx`). Its tabs were not re-checked in this pass.
- [x] Files tab: `hub-file-tree`, `hub-file-viewer` and `hub-item-files` with virtualized tree, rename,
      move, new folder, unsaved guard and conflict dialog, covered by component tests and the e2e spec.
      Not exercised by hand in a browser.
- [x] Versions tab reusing `skill-versions-tab`; Usage tab (covered by `hub-item-page.test.tsx`).
- [x] Plugin items: manifest viewer and draft versions (`plugin-draft.ts`), covered by core and view tests.
- Exit: browse, edit and save a skill file from Hub, version recorded; plugin files viewable.

## Phase 3 — Add flows: URL, registry, upload, preview (4–5 days)
- [x] `server/internal/hub/` package: `Importer` interface; `github` (codeload zip, with the
      base URL injectable for tests, and HEAD → main → master fallback), `url_zip`, `skills_sh`,
      `clawhub` and `registry`. Deviation: the ClawHub and skills.sh fetchers were not moved out
      of `handler/skill.go`. The hub reaches them through adapters that pass them the
      SafeFetcher's client.
- [x] Safe fetcher (`hub/fetch.go`): loopback and private addresses refused at dial time, redirect
      cap, scheme check, 30s deadline, streaming size cap. Archives refuse symlinks, traversal,
      bombs (compression ratio) and duplicate paths (`hub/archive.go`).
- [x] Scanner: rules are in `server/internal/hub/scan.go`, not a `scan/` subpackage, with table
      tests in `scan_test.go`.
- [x] `POST /api/hub/import/preview` + `/commit`. Commit is idempotent: a version row is unique on
      (item, version, digest). Conflict strategies are rename, replace and skip.
- [-] Upload through `POST /api/hub/uploads` and `/complete`, and through a presigned PUT. Dropped
      (see the note at the top). A zip is uploaded through the preview route, validated on the
      server, and capped at 16 MiB.
- [x] Source CRUD endpoints (`hub_source.go`).
- [x] Private-repo credentials encrypted at rest (AES-256-GCM with `MULTICA_HUB_SECRET_KEY`; unset -> 409 `secrets_not_configured`). The token goes only to the source's own host, is never returned or logged, and is dropped on cross-host redirects. `make dev-full` generates the key.
- [x] UI: `AddToHubDialog` with the URL, registry and upload steps exists. Not verified in this
      pass: the blank step and the drag-and-drop folder.
- [x] E2E: `e2e/hub.spec.ts` exists and was run in the frontend completion pass: 3 passed (import and
      edit, filter and sort with a reload, and a skill-file conflict that is resolved by overwrite).
- Exit: skills and plugins can be added from all source types with scan results.

## Phase 4 — Central object storage (3 days)
- [x] Storage gained list prefix (`Lister`), copy (`Copier`) and presign PUT (`PresignPutter`) on
      `S3Storage` and `LocalStorage`, with tests. The opt-in SeaweedFS integration test was not run
      in this pass. Presign PUT is implemented but unused (see the note at the top).
- [x] `hub_item_version_blob` (migrations 934–937). Every commit, apply and rebuild writes an
      immutable bundle and a version row. Deviation: the table is 934, not 929.
- [x] Digest verified on read: a download re-reads the stored zip and refuses to serve it unless
      its content digest matches the recorded digest. A mismatch is a 500, with the digests in the
      log and no content. A presigned link is not verified, because the bytes go straight to the
      client. Bundles are kept current: an edited skill or a newer plugin version gets a new bundle
      before it is served.
- [-] Maintenance job that backfills bundles for existing items. Dropped: a bundle is built on
      the first request for an item that has none, so there is nothing to backfill.
- [x] Staging garbage collection: `hub_staging_gc` runs hourly on the scheduler (global scope)
      and deletes `staging/` objects older than 60 minutes (the 30 minute preview TTL plus a
      margin). It complements the bucket lifecycle rule on `staging/` and does not replace it.
      Local disk has no lifecycle rule, so the job is what sweeps it there.
- [x] Deploy: `seaweedfs` service and bucket init in `docker-compose.selfhost.yml`, and in `make dev-full`.
      Helm: `deploy/helm` does not model the Hub or WeKnora, so it is unchanged (not applicable).
- [x] Runbook `docs/engineering/runbooks/hub-object-storage.md` exists. Not re-reviewed in this
      pass.
- Exit: bundles in SeaweedFS, digest-verified; switching endpoint requires config only.

## Phase 5 — Sources management, sync, updates (3–4 days)
- [x] Source manager UI (`hub-sources-manager.tsx`): add, edit, remove, sync now, auto-sync
      interval and delete with a choice for items. Not verified in this pass: the in-use guard.
- [x] Sync worker on the existing `scheduler` (`jobs_hub_sync.go`): fetch, compare the digest,
      record the check. Forked and locally edited items are flagged, not overwritten.
- [x] `check-update` / `apply-update` with the file diff, `fork`, and tracking states. Plugin apply
      is covered end to end: overwrite, `version_conflict`, `key_changed`, and fork.
- [x] Metrics: `multica_hub_sync_*`, `multica_hub_import_commits_total`, `multica_hub_bundle_builds_total`, `multica_hub_storage_errors_total`.
- Exit: update-available badge, diff, apply or fork all work end to end.

## Phase 6 — Hardening, permissions, docs, release (2–3 days)
- [x] Activity log: server `GET /api/hub/activity`; Activity tab on each item and a workspace Activity tab on the Hub.
- [x] Permissions matrix tests (member view, editor edit, admin manage) in `hub_permissions_test.go`; routes added later (item detail, file save, source credentials) have their own permission cases in their tests.
- [x] Quotas (storage bytes, item count) with clear errors. Limits are enforced server-side (`HUB_MAX_ITEMS_PER_WORKSPACE`, `HUB_MAX_STORAGE_BYTES_PER_WORKSPACE`), the Hub shows a usage meter, and `quota_exceeded` is shown in the add, sync, and update flows (`hub-source-sync-button.tsx`, `hub-update-dialog.tsx`, `hub-import-preview.tsx`).
- [x] Export bundle / export all: per-item "Download bundle" and admin-only "Export all" on the Hub.
- [ ] a11y pass on tree and editor (`accessibility-scan`), keyboard navigation. Done: the file tree is an ARIA tree with arrow, Home/End, and Enter keys, a roving tab stop, and labelled editor and preview regions. Dialog focus return is covered by tests for the Add dialog and the update review (Escape and Cancel return focus to the trigger). Not done: `accessibility-scan` has not been run.
- [x] Fumadocs pages (en + zh) for Hub and object-storage setup (`hub`, `hub-object-storage`); whats-new entry for the Hub (version 0.8.2 is unconfirmed; hidden until a server at that version connects).
- [-] Remove the `hub_v1` flag after soak. Dropped: the flag was never added.
- [ ] Full `make check`. Not run as one pipeline; its parts were run separately: `pnpm typecheck`, core and views vitest, the Go suites for handler, service, daemon, hub, storage, scheduler, cmd, and `e2e/hub.spec.ts` (3 cases) against a live stack wired to SeaweedFS.

## Follow-up hardening (G4 pass)
- Legacy ClawHub and skills.sh fetches go through the SafeFetcher's client, so a redirect to a
  loopback or private address is refused at dial time. Test: `hub/fetch_test.go`,
  `handler/hub_legacy_test.go`.
- The GitHub codeload base URL is a package variable (`hub.GitHubCodeloadBase`) so the handler
  tests can point it at an httptest origin. Tests: `handler/hub_github_test.go`.
- A stored plugin manifest is canonicalized again before a bundle or update check reads it. Its
  stored form is JSONB, which does not keep the publish's byte layout, so without this every
  imported plugin read as locally modified. Test: `handler/hub_update_test.go`.
- A bundle imported from a plugin records the raw manifest digest, but the local copy hashes the
  canonical one. The first bundle request for an imported plugin therefore rebuilds once. After
  that the digests agree.
- Deleting a skill or a plugin package removes its hub row, version rows and bundle objects,
  after the delete commits. Best effort. Test: `handler/hub_delete_test.go`.

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

## Frontend completion pass
- List and catalog: sort by name, updated, created, most used, or source, with a direction control.
  Filters for kind, source, label, status (update available, scan warning, tracking, forked,
  manual), and in-use (agents for skills, installations for plugins). Dimensions are multi-select
  and combine with AND. The count reads "N of M", and "Clear filters" resets filters and search.
  Rows past 200 are virtualized (`HUB_VIRTUALIZE_AFTER`). Logic is in `packages/core/hub/list.ts`.
- View mode, sort, and filters persist per workspace in `useHubViewStore`
  (`packages/core/hub/stores/view-store.ts`). The search text is session-only.
- Search matches name, description, labels, and the source type. File-name search is not
  implemented: `GET /api/hub/items` takes no query parameter, so there is no cheap server route to
  reuse.
- Saving a skill file uses the server's etag lock (`PUT /api/hub/items/skill/{id}/files`). A 409
  becomes `HubFileConflictError` with the server's current etag, and overwrite uses that etag
  without another read. `getHubItem` and `hubItemOptions` are exposed. `useHubItemMeta` still
  derives from the list, which is one request for every item.
- Private sources: the Add dialog's From URL tab (GitHub and zip) takes an optional token. A token
  is saved as a source first, and the preview uses that source's id. The source form shows
  "Token saved" with Replace and Remove, and never shows the token.
- Origins: uploaded skills (`origin.type = upload`) and url_zip or registry origins now map to
  their own source. Before, they were shown as "Created here".

