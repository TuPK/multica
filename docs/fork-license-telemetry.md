# Fork: owner-reporting code removed

This fork does not report anything about its deployments to Index Labs (the
upstream owner). The self-hosted deployment snapshot that upstream added in
2026 is removed from the server, along with the collection that fed it.

This document lists what was removed, what was kept and why, and the rule for
future upstream merges.

## Removed

The self-host telemetry sent one daily deployment snapshot to the compiled-in
endpoint `https://telemetry.multica.ai/v1/telemetry/events`. The endpoint could
not be changed by configuration, and `DO_NOT_TRACK` only turned it off.

| Path | What it was |
| --- | --- |
| `server/internal/selfhosttelemetry/` (whole package) | Daily snapshot collector (SQL), HTTP sender to `telemetry.multica.ai`, leader lock on `instance_telemetry_state`, `DO_NOT_TRACK` parsing and tests. |
| `server/cmd/server/main.go` | Imports the package; reads `DO_NOT_TRACK`; builds and runs the telemetry worker; sets `TaskService.RecordMemberActivity`. |
| `server/internal/service/task.go` | `TaskService.RecordMemberActivity` flag, `RecordHumanDelegation` (writes `member_delegation_day`), and their call sites on enqueue, mention, quick-create, chat, and direct-chat paths. `CreatedAt` on the comment attribution facts existed only for that ledger. |
| `server/internal/service/issue.go` | `RecordHumanDelegation` calls on issue create (agent and squad assignment). |
| `server/internal/service/autopilot.go` | `RecordHumanDelegation` call on manual autopilot run. |
| `server/internal/handler/comment.go` | `RecordHumanDelegation` call on comment folded into a queued run. |
| `server/internal/attribution/attribution.go` | `Result.HumanAction` / `HumanActedAt`, `CommentFacts.CreatedAt`, and the lines that set them. Used only by the ledger. |
| `server/internal/attribution/attribution_test.go` | `TestHumanActionOnlyForTheActingMember`. |
| `server/internal/service/quick_create_enqueue_test.go` | `RecordMemberActivity` setup and the `member_delegation_day` assertion. The queued-task assertion stays. |
| `server/internal/service/selfhost_telemetry_activity_test.go` | Tests of the ledger. |
| `server/pkg/db/queries/member_delegation.sql`, `server/pkg/db/generated/member_delegation.sql.go` | The `RecordMemberDelegationDay` query. |
| `docker-compose.selfhost.yml`, `deploy/helm/multica/values.yaml`, `deploy/helm/multica/templates/configmap.yaml`, `.env.example` | `DO_NOT_TRACK` passthrough and settings. |
| `SELF_HOSTING.md`, `README.md`, `README.zh.md`, `README.ja.md` | "Anonymous Deployment Telemetry" sections. |
| `apps/docs/content/docs/environment-variables{,.zh,.ja,.ko,.fr,.zh-Hant}.mdx` | `DO_NOT_TRACK` row and telemetry paragraphs. |
| `apps/web/features/landing/i18n/{en,ja,ko,zh}.ts` | Privacy sentences and changelog bullets describing the snapshot. |
| `scripts/selfhost-config.test.sh` | One description string that mentioned `DO_NOT_TRACK`. |

### Kept as no-ops, so a new upstream call site compiles

`TaskService.RecordHumanDelegation` (`server/internal/service/telemetry_fork.go`) and `attribution.CommentFacts.CreatedAt` exist again but do nothing. When a merge brings a new upstream call, leave it: it reports nothing. Do not restore the ledger write, `RecordMemberActivity`, `Result.HumanAction` or `HumanActedAt`.

### Deliberately not removed: database objects

The migrations that created the telemetry tables stay, and so do the tables:

- `479`–`482`: `instance_telemetry_state` (deployment identity) and the
  `agent_task_queue` telemetry index.
- `589`–`591`: `member_delegation_day` (the member-activity ledger).

Removing a migration file breaks `schema_migrations` for every database that
already applied it. Dropping the tables destroys data. Both need a deliberate
decision. The tables now receive no writes.

The following still refer to `member_delegation_day` and are correct as long as
the table exists: `server/pkg/db/queries/account_deletion.sql` (deletes a user's
rows), `platform-management/accountdeletion/registry.go`, and
`server/internal/handler/workspace_delete_manifest_test.go`.

## Reviewed and kept

These were checked for outbound reporting. None of them sends deployment data
to the upstream owner by default.

| Component | Why it stays |
| --- | --- |
| PostHog analytics (`server/internal/analytics/`, `packages/core/analytics/`, `packages/core/ai-traffic/`, `handler/config.go`) | Product analytics that runs only when `POSTHOG_API_KEY` is set. Self-hosted builds ship it empty, so the no-op client is used. If an operator sets the key, the events go to that operator's PostHog project. `ANALYTICS_DISABLED=true` turns it off. **Do not copy the upstream cloud `.env` into the fork.** `handler/config.go` defaults `POSTHOG_HOST` to `us.i.posthog.com`; that default is applied only when a key is set. |
| `server/internal/entitlement/`, `planlimit/`, `workspaceguard/policy_access.go`, `auth/cloud_pat.go`, `handler/plan_limit.go` | Cloud plan enforcement and cloud PAT verification. They reach out only when `MULTICA_CLOUD_URL` is set. With it empty, the code takes the self-host path with no request. The request carries a workspace ID to that URL, so **leave `MULTICA_CLOUD_URL` unset**. |
| `server/internal/handler/feedback.go`, `handler/contact_sales.go` | User-initiated feedback and sales inquiries. They are analytics events (PostHog, same gate as above) and, for contact sales, a request the user submits on purpose. |
| `server/internal/usagesnapshot/`, `scheduler/jobs_usage_snapshot.go`, table `platform_usage_daily` | Writes usage rows to the local database only. Nothing reads them from outside this deployment. The platform console is the upstream's back office and is not started by the server. |
| `platform-management/` | Upstream back office (billing, lifecycle email via Resend, e-invoice provider). Separate Go module, not started by the self-host server. |
| `server/internal/salescollect/`, `cmd/multica/cmd_sales.go` | Crawl engine for the `multica sales` command. Talks to a crawl service the operator configures. |
| `server/internal/cli/update.go` | Reads release metadata from the GitHub API for update checks. It sends no deployment data. |
| `server/internal/daemon/`, `server/internal/qoderruntime/`, `server/internal/integrations/*` | Daemon and integration clients connect where the operator configures them (for example GitHub, Lark, Slack). They are not owner reporting. |
| `server/internal/handler/auth.go` (Google userinfo) | Login with Google. It is not owner reporting. |

## Rule for future upstream merges

1. **Do not bring these back.** When merging `upstream/main`, drop any
   reintroduced file or hunk in the list above. Keep the table and migration
   history as described.
2. If a merge re-adds `server/internal/selfhosttelemetry/`, the `DO_NOT_TRACK`
   wiring, `RecordHumanDelegation`, `RecordMemberActivity`, or
   `HumanAction`/`HumanActedAt`, delete them again in the same merge commit,
   then rerun the checks below.
3. Any new code that sends data to a fixed Multica host (`multica.ai`,
   `telemetry.`, `ghcr.io/multica-ai` beyond image pulls) needs a decision in
   this document before it lands.

Check after each merge (expect no output apart from the migration and
account-deletion references listed above):

```bash
rg -n "selfhosttelemetry|RecordHumanDelegation|RecordMemberActivity|HumanActedAt|telemetry\.multica\.ai|DO_NOT_TRACK" \
  --glob '!server/migrations/**' --glob '!docs/fork-license-telemetry.md' .
```

Then run:

```bash
cd server && go vet ./internal/attribution/ ./internal/service/ ./internal/handler/ ./cmd/server/
pnpm typecheck
```

## Verification of this change

- `go build ./...` in `server/`: passes.
- `go vet` on `internal/attribution`, `internal/service`, `internal/handler`,
  `cmd/server`, `internal/usagesnapshot`: passes.
- `go test ./internal/attribution ./internal/usagesnapshot`: passes.
- `pnpm typecheck`: passes.
- `bash scripts/selfhost-config.test.sh`: fails identically on `HEAD` (it
  reports undocumented `KNOWLEDGE_*` and `MULTICA_RESEARCH_SECRET_KEY` keys).
  This change does not add to that list.
- DB-backed tests were not run against a dedicated test database. The
  telemetry ledger tests were deleted with their code.
