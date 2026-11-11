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
- [ ] Re-check upstream licence/maturity of SeaweedFS and RustFS before Phase 4 ships.

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
- [ ] Handlers `GET /api/hub/items/{kind}/{id}` (not built; the UI reads the list). `GET
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
- [ ] Unified file API `GET/PUT/DELETE /api/hub/items/{kind}/{id}/files`, `POST .../files/move`.
      Not built: the Hub reads files through the existing skill and plugin-version routes
      (`packages/core/hub/files.ts`), so the optimistic lock below does not exist either.
- [ ] Optimistic lock: `updated_at`/ETag on file PUT, 409 on mismatch. Not built (see above).
- [x] `HubItemPage` exists (`hub-item-page.tsx`). Its tabs were not re-checked in this pass.
- [ ] Files tab: `hub-file-tree`, `hub-file-viewer` and `hub-item-files` exist. Not verified:
      virtualized tree, rename, move, new folder, unsaved guard, conflict dialog.
- [ ] Versions tab reusing `skill-versions-tab`; Usage tab. Not verified in this pass.
- [ ] Plugin items: manifest viewer and draft versions (`plugin-draft.ts` exists). Not verified
      in this pass.
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
- [ ] Private-repo credentials encrypted at rest. Not built: the Hub fetches public sources only.
- [x] UI: `AddToHubDialog` with the URL, registry and upload steps exists. Not verified in this
      pass: the blank step and the drag-and-drop folder.
- [x] E2E: `e2e/hub.spec.ts` exists. Not run in this pass.
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
- [ ] Deploy: `seaweedfs` service and bucket init in `docker-compose.selfhost.yml` (done, owned by
      the compose change). Helm values not verified in this pass.
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
- [ ] Metrics for sync duration and failures are not built. Logs are.
- Exit: update-available badge, diff, apply or fork all work end to end.

## Phase 6 — Hardening, permissions, docs, release (2–3 days)
- [x] Activity log: server `GET /api/hub/activity`; Activity tab on each item and a workspace Activity tab on the Hub.
- [ ] Permissions matrix tests (member view, editor edit, admin manage). Server tests exist in `hub_permissions_test.go`; matrix coverage was not reviewed in this pass.
- [ ] Quotas (storage bytes, item count) with clear errors. Limits are enforced server-side (`HUB_MAX_ITEMS_PER_WORKSPACE`, `HUB_MAX_STORAGE_BYTES_PER_WORKSPACE`) and the Hub shows a usage meter. Not done: the `quota_exceeded` error is not yet shown in the add, sync, or update flows.
- [x] Export bundle / export all: per-item "Download bundle" and admin-only "Export all" on the Hub.
- [ ] a11y pass on tree and editor (`accessibility-scan`), keyboard navigation. Done: the file tree is an ARIA tree with arrow, Home/End, and Enter keys, a roving tab stop, and labelled editor and preview regions. Not done: `accessibility-scan` was not run (no dev server in this pass), and dialog focus return was not checked.
- [x] Fumadocs pages (en + zh) for Hub and object-storage setup (`hub`, `hub-object-storage`); whats-new entry for the Hub (version 0.8.2 is unconfirmed; hidden until a server at that version connects).
- [-] Remove the `hub_v1` flag after soak. Dropped: the flag was never added.
- [ ] Full `make check` and the Playwright suite. Not run in this pass; the Go checks listed in the
      follow-up report were run instead.

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
