# R1 gate (G-R1, P12): record

Date: 2026-11-19. Checkout: `main`, commits `9fbd3b43f` and `74319a406` before this record (test-only fixes; no product code changed for R1). Test database: `multica_platform_test`, reached through `scripts/require-test-database.sh` with the DSN built from `.env` inside the command. The shared `multica` database was not used. Database packages ran with `-p 1 -count=1`.

Other sessions were editing `server/internal/handler/issue.go`, `issue_workflow_entry.go`, `server/cmd/server/issue_unread_listeners.go` and the proposal files while the handler run was in progress. A kiro run for proposal visibility also used the test database during this gate. Results for `./internal/handler` therefore include some in-progress code. The full handler run was done before the R1 test fixes and was not repeated.

## 1. Commands and results

| Check | Result |
| --- | --- |
| `server ./internal/middleware` | pass: 359 tests, 0 fail |
| `server ./internal/auth` | pass: 170 tests, 0 fail |
| `server ./internal/auditstore` | pass: 6 tests, 0 fail |
| `server ./internal/realtime` | pass: 138 tests, 0 fail |
| `server ./cmd/server` | pass: 778 tests, 0 fail |
| `server ./internal/handler` (full package, before the R1 test fixes) | 7031 pass, 4 `--- FAIL` lines: `TestAuditInvitationEvents` (parent, `created`, `accepted`) and `TestReproSquadRequestMovedToInReviewByMemberGetsReviewerRun`. The first three were fixed in `9fbd3b43f`; `TestAuditInvitationEvents` now passes with all four subtests. The squad repro test is a pre-existing reproduction test, not an R1 regression (see section 4). |
| `platform-management ./audit/... ./accountguard/... ./accountdeletion/... ./accountexport/...` | pass: 48 tests, 0 fail |
| `platform-management ./...` without a database (`go test` with no DSN) | exit 0, includes `TestCacheTTLIsAtMostSixtySeconds` (`74319a406`) |
| `go build ./...`, `go vet` on the packages listed in the plan | exit 0 |
| `platform-management` `go build ./...`, `go vet ./...` | exit 0 |
| `pnpm catalog:build` | exit 0 |
| `pnpm --filter @multica/views exec vitest run members-tab` (P1) | exit 0 |
| `pnpm typecheck` | exit 0 |
| `pnpm lint` | exit 0 |
| `pnpm test` | **exit 1**. See section 4. |
| Docker image build `docker build -f platform-management/Dockerfile` ([PI]) | **not run**: the Docker daemon did not respond in this environment. |
| p95 latency of a read route before and after the guard (spec 15.3) | **not measured**. |

New tests written for gaps in this gate (each one was written first, then run):

- `server/internal/handler/me_deletion_test.go`: `TestAccountDeletionFreesTheEmailForANewAccount` (AC5). Passes; the product already meets it.
- `platform-management/accountguard/accountguard_test.go`: `TestCacheTTLIsAtMostSixtySeconds` (AC1 bound). Passes.
- `server/internal/handler/audit_events_test.go`: stale `is_guest` expectation in `TestAuditInvitationEvents` (`9fbd3b43f`). Test-only.

## 2. Acceptance criteria AC1 to AC6

| AC | Result | Named tests | Gap |
| --- | --- | --- | --- |
| AC1 Suspended account: every JWT, PAT, task and daemon request refused and WebSocket cut within 60 seconds, without redeploy | **Partly covered, open** | Decision table: `TestAccountGuardDecisionTable`; daemon exemption: `TestAccountGuardSkipsDaemonTokenWithoutOwner`; WebSocket refused on connect: `TestHandleWebSocketRefusesNonActiveAccountOnCookieUpgrade`, `TestHandleWebSocketRefusesNonActiveAccountOnFirstMessageAuth`; cache: `TestMissReadsStoreThenHitsCache`; DB read: `TestAccountGuardReadsPlatformAccount`; TTL bound: `TestCacheTTLIsAtMostSixtySeconds` | **Open:** a SQL change to `suspended` does not disconnect an open WebSocket. The Redis channel path (console) does. The spec's wording covers SQL too. This needs a design decision (periodic WebSocket re-check, or a DB notify), so it is not closed here. |
| AC2 After "sign out everywhere", an old token is refused and cannot renew; the current session keeps working | Covered | `TestRevokeAllSessionsWritesMarkAndReturnsASessionThatSurvivesIt`, `TestRevokeAllSessionsDisconnectsTheUsersWebSocket`, `TestRevokeAllSessionsRefusesMachineCredentials`, `TestAuthSessionRevokedCheck`, `TestAuthSessionRevokedClearsCookieAndDoesNotRenew`, `TestAuthRenewalKeepsAuthTime`, `TestHandleWebSocketRefusesRevokedSessionOnCookieUpgrade`, `TestHandleWebSocketRefusesRevokedSessionOnFirstMessageAuth` | None |
| AC3 Each R1 action writes exactly one audit row; a failing audit write rolls the change back | Covered | `TestAuditMemberEvents` (left, removed, role changed), `TestAuditInvitationEvents` (created, revoked, accepted, declined), `TestAuditAccountEvents` (PAT created and revoked, sessions revoked), `TestAuditLoginAtEachCallSite`, `TestAuditFailureRollsBackMemberAndWorkspaceChanges` (six subtests), `TestRecordWritesRedactedRow` (auditstore) | None for the listed actions. The R1 catalog is checked by `TestR1CatalogActionsAreDefined` (platform). |
| AC4 A plain member gets 403 when listing invitations | Covered, through the real router | `TestListInvitationsThroughRouterRequiresOwnerOrAdmin` (`server/cmd/server`); web: `members-tab` test (exit 0) | None |
| AC5 Account deletion: refused for a sole owner; after the grace period the record is anonymised, memberships, PATs and connections are gone, and the old email can register as a new person | Covered (the last clause was added in `74319a406`) | Sole owner 409: `TestDeletionSoleOwnerOfSharedWorkspaceIs409AndChangesNothing`; job: `TestAccountDeletionJobAnonymisesAndRemoves`, `TestAccountDeletionJobKeepsOtherMembersAndRefusesToOrphanAWorkspace`, `TestAccountDeletionJobRemovesTheUsersGroupRows`, `TestAccountDeletionJobLeavesAccountsThatAreNotDue`, `TestAccountDeletionJobAnonymisesAccountRequests`; re-registration: `TestAccountDeletionFreesTheEmailForANewAccount`; rules: `TestClassifyMemberships`; column registry: `isUserColumnName` (test file `server/internal/migrations/user_columns_registry_test.go`) | None. The job-runs-twice property is not separately named; see the note below. |
| AC6 The export has no token value or secret, and no other person's rows | Covered | `TestExportHasExactKeysNoSecretsAndNoOtherUsersRows`, `TestExportHasNoBillingProfile`; platform secret scan: `TestFindSecretsCleanExport`, `TestFindSecretsForbiddenKeysAtAnyDepth`, `TestFindSecretsForbiddenLiterals` | None |

Note on the job running twice (plan P11, "job chạy hai lần không lỗi"): `TestAccountDeletionJobLeavesAccountsThatAreNotDue` and `TestAccountDeletionJobKeepsOtherMembersAndRefusesToOrphanAWorkspace` cover the idempotent case in part. No test runs the job twice and compares the result. This is open.

Plan P11 test "hủy trong thời gian chờ khôi phục đăng nhập": covered by `TestDeletionCancelRestoresActiveAndKeepsTheRevocation`.

## 3. Documentation and steps that are not gates

- `docs/platform-operations.md` section 5 (R1 status) is an older record. It says the G-R1 database tests were "not run". This record supersedes that status. The document has not been edited here.
- Other sessions' work in `packages/views` and `server/internal/handler/issue*` is not in this gate.

## 4. Failures outside R1, and open items

**`pnpm test`, exit 1.** Results by package:

- `@multica/web`: one real failure. `app/text-contrast.test.ts` reports `packages/views/projects/components/project-detail.tsx:249` uses `opacity-60`, which the test forbids. This comes from recent project-page work, not from R1. Not fixed here.
- `@multica/core`: several tests fail with 10 to 80 second timeouts, in the SDLC harness, skill catalog and image-timeline suites. These look like load timeouts while several sessions run heavy jobs. They were not rerun in isolation, so they are unverified.
- `@multica/desktop`: `daemon-manager-quit.test.ts` ("leaves an already prevented quit to the Windows updater") fails. Not investigated.
- `@multica/ui`: vitest reported `Failed to start forks worker` for four test files. The summary still shows 60 passed, 1 skipped. Environment, not product.
- `@multica/views`: `never throws, whatever it is handed` timed out at 80 seconds. Not investigated.
- `@multica/docs`, `@multica/platform-console`: pass.

These are not R1 failures, but the gate is not green until they are rerun or triaged. The R1 criteria themselves are covered by the Go and platform tests above.

**Open R1 items:**

1. AC1, SQL-path suspension does not disconnect an open WebSocket. Needs a design decision.
2. The p95 latency measurement (spec 15.3) was not done.
3. Docker image build (`[PI]`) and the compose-based checks were not run; Docker did not respond.
4. Repo-wide `pnpm test` failures listed above.
5. `TestReproSquadRequestMovedToInReviewByMemberGetsReviewerRun` (`server/internal/handler/issue_workflow_in_review_repro_test.go`) is a reproduction test for an unfixed bug, red on main before R1, and not part of R1.

## 5. Commits in this gate

- `9fbd3b43f` test(audit): expect is_guest in invitation audit details
- `74319a406` test(platform): pin the account-email reuse and the 60s guard cache bound
- This record: committed separately.

## 6. Closing run for the open items (2026-11-19)

This section supersedes the "open" entries in sections 2 and 4 for the items below. Other sections are unchanged.

Database: `multica_platform_test` only, through `scripts/require-test-database.sh`, with `DATABASE_URL` and `PLATFORM_TEST_DATABASE_URL` built from `.env` inside the command. The database `multica` was not used. Code was implemented by kiro (claude-sonnet-5.5, medium) and each change was checked by hand.

### AC1: WebSocket cut when an account is suspended by SQL (closed)

**Choice.** A SQL change publishes nothing, so the Redis notice cannot reach the server. Each open socket re-reads its account status every 20 seconds through the existing account guard. The guard cache TTL drops from 60s to 30s. Worst case from a SQL change to a cut socket: TTL + interval = 50s, inside the 60s bound. A Postgres trigger with NOTIFY was rejected: it needs a migration and a DB-side change for a path that the 20s recheck already covers.

**Scope.** The watcher disconnects only on HTTP 403 from `accountRefusal`: suspended, deleted, pending deletion, or an unknown status. A store error (503) does not cut the socket. Session revocation is not rechecked here; AC2 is covered by the existing revocation paths.

**Files.** `server/internal/realtime/hub.go` (`Client.done`, `watchAccount`, `accountStatusRecheckInterval`), `server/internal/realtime/account_watch_test.go`, `platform-management/accountguard/accountguard.go` (TTL 30s), `platform-management/accountguard/accountguard_test.go`, `platform-management/accountguard/ttl_bound_test.go`, `platform-management/internal/console/write.go` (comment only).

| Test | Result | Red first |
| --- | --- | --- |
| `TestAccountWatchClosesSocketWhenSuspendedBySQL` | PASS | Yes: with the watcher disabled, the socket stayed open. |
| `TestAccountWatchStopsWhenConnectionCloses` | PASS | Weak: it fails with the watcher disabled, but does not isolate the exit path. |
| `TestHandleWebSocketRefusesNonActiveAccountOnCookieUpgrade`, `...OnFirstMessageAuth` | PASS | Existing tests |
| `go test ./accountguard/...` (platform) | PASS | Pinned `TTL <= 30s` and `TTL + 20s <= 60s`. |
| `go test ./internal/realtime -count=1` | PASS | Full package. |

Not verified: the real Redis cache and Postgres path for the watcher. The tests use in-memory fakes.

### Account deletion job runs twice (closed)

`TestAccountDeletionJobRunsTwiceWithoutError` (`server/internal/handler/me_deletion_test.go`) calls `DeleteAccount` for one account twice. The first call returns `(true, nil)`. The second returns `(false, nil)`, and a snapshot of the account, user, membership, workspace and audit rows is unchanged. No production change was needed; `LockDueAccountDeletion` already finds no row on the second run.

An earlier version of the test called `DueAccountDeletions` over the whole shared database. With that version, about 1,500 foreign-key errors broke unrelated handler tests. The mechanism was not identified. The version without the global listing gave no such errors in the full run.

Result: `TestAccountDeletionJobRunsTwiceWithoutError` PASS, and the scheduler tests `TestRunAccountDeletions*` PASS.

### Docker build of the platform service (done)

`docker build -f platform-management/Dockerfile -t multica-platform-r1gate .` exited 0 on the second attempt. The first attempt failed with `DeadlineExceeded` while pulling `golang:1.27.3-alpine`, which points to the network, not the Dockerfile. The image was built locally; it was not run and the compose checks were not repeated.

### Handler package (one test still failing, not from this slice)

Full run: `go test ./internal/handler -count=1 -p 1 -timeout 60m -skip 'TestReproSquad' -v`. The `-timeout 60m` flag was needed; the default 10 minutes hit a timeout in `TestDigest_OrdersByWhatNeedsAttentionFirst`. The squad in_review repro tests were excluded, as owned by `handler-test-triage`.

- Without the deletion test, the run was green: 3915 PASS, 0 FAIL (before the test fix below).
- With the fixed deletion test: 7079 PASS lines, 1 FAIL: `TestClaimTasksByRuntime_DeadlineRecoveryOverWebSocket` (`daemon_batch_claim_recovery_test.go:124`, read i/o timeout). It fails alone too. It uses `daemonws.Hub`, not `realtime.Hub`, so the watcher in this slice does not touch that path. The checkout has uncommitted edits from other sessions in the daemon and service code; the cause was not investigated, and the file was not touched because it lies outside this slice.

Gate status for this item: **not green**. Needs an owner for `TestClaimTasksByRuntime_DeadlineRecoveryOverWebSocket`.

### Commits

- `f85ec5fad` fix(platform): cap account guard cache TTL at 30s
- `8a2469175` fix(realtime): close sockets of accounts suspended by SQL
- `4b5483de3` test(handler): account deletion can run twice without error
- This record: committed separately.
