# R2 gate (G-R2, P30): record

Date: 2026-11-19. Checkout: `main`, commit `b64d9dec8` before this record. Test database: `multica_platform_test`, reached through `scripts/require-test-database.sh` with the DSN built from `.env` inside each command. The shared `multica` database was not used. Database packages ran with `-p 1 -count=1`.

Other sessions were editing `server/internal/handler/issue*.go`, the proposal files, `server/cmd/server/issue_unread_listeners.go` and `packages/views` during these runs. A kiro run for proposal visibility also used the test database during the G-R1 handler run. Results that touch those packages may include in-progress code.

## 1. Commands and results

| Check | Result |
| --- | --- |
| `server ./internal/service` (full package) | pass: 1797 tests, 0 fail, 20 skip |
| `server ./internal/entitlement` | pass: 65 tests, 0 fail |
| `server ./internal/seatcapacity` | pass: 37 tests, 0 fail |
| `server ./internal/workspaceguard` | pass: 110 tests, 0 fail |
| `server ./internal/usagesnapshot` | pass: 25 tests, 0 fail |
| `server ./internal/workspaceexport` | pass: 23 tests, 0 fail |
| `server ./internal/workspacedeletion` | pass: 8 tests, 0 fail |
| `server ./internal/scheduler` | pass: 81 tests, 0 fail |
| `server ./internal/auditaction` | pass: 6 tests, 0 fail |
| `server ./internal/middleware` | pass: 359 tests, 0 fail, 11 skip |
| `server ./cmd/server` | pass: 788 tests, 0 fail |
| `server ./internal/handler` (full package) | run in the G-R1 record: 7031 pass; the only red test is the pre-existing squad repro (see G-R1 section 4). The two R1 tests that were stale were fixed in `9fbd3b43f`. The full handler package was not repeated for this gate. |
| `platform-management ./...` (full module, with the test DSN) | pass: 1640 tests, 0 fail, 5 skip, 39 packages `ok` |
| `platform-management ./...` without a database | exit 0 (G-R1 record) |
| `pnpm typecheck`, `pnpm lint` | exit 0 (G-R1 record) |
| `pnpm test` | exit 1, same as the G-R1 record (section 4 there). The web `text-contrast` failure is the one real failure, from project-page work. Not rerun for this gate. |
| `pnpm --filter @multica/views exec vitest run billing-tab` | pass: 67 tests, 0 fail |
| `pnpm --filter @multica/platform-console` tests (from the G-R1 `pnpm test` run) | pass: 102 tests (2 files) |
| `go build` and `go vet` on the server packages named in the plan | exit 0 (G-R1 record) |
| `platform-management` `go build` and `go vet` | exit 0 (G-R1 record) |
| `pnpm catalog:build` | exit 0 |
| `docker build -f platform-management/Dockerfile` ([PI]) | **not run**: the Docker daemon did not respond. |
| `pnpm exec playwright test` | **not run**: not authorised for this gate. |

Tests added for this gate (each one was written first, then run):

- `server/internal/handler/invitation_test.go`: `TestSeatCapacityOutageLeavesWorkspaceReadsWorking` (AC20 read path), commit `b64d9dec8`. Passes.

## 2. Commercial enablement drill

The drill ran on `multica_platform_test` only. It is a rehearsal on a copy-like database with the spec's fixtures, not production data.

| Step | Result |
| --- | --- |
| Workspaces with no `workspace_subscription` row, before | 81 of 89 |
| `go run ./cmd/migrate up` on the test DSN | exit 0, already at head (every migration `skip ... already applied`) |
| `platform-backfill` first run | exit 0: 1 batch, 81 fetched, 81 inserted |
| `subscription.backfilled` audit rows after the first run | 81 |
| Workspaces with no subscription row, after the first run | 0 |
| `platform-backfill` second run | exit 0: inserted 0 |
| `subscription.backfilled` audit rows after the second run | 81 (no duplicates) |
| Backfilled rows that are not `free`, `active`, version 1 | 0 |
| `platform_workspace` rows that are not `active` | 0. No workspace is read-only or suspended because of the cutover. |
| Platform refuses to start when a migration is missing | covered by the schema check tests (`TestMissingErrorListsVersions`, `TestLoadMissingRequiredVariableNamesIt`), not run live |
| Platform starts and `/healthz` returns 200 | **not run**: no Redis in this environment (the platform requires `REDIS_URL`) and Docker did not respond |
| Entitlement policy for each backfilled workspace through a running platform | **not run**: depends on the platform start above. The DB state it reads was checked in the table above. |
| Compose stack with the `platform` service; `MULTICA_CLOUD_URL` pointed at the platform; no workspace blocked | **not run**: Docker did not respond |
| Port 8095 unreachable from outside; `mcn_` token refused | **not run** |

Backfill audit rows, and the backfill itself, are on the test database and can be removed by the owner if the test data is reset.

## 3. Acceptance criteria AC7 to AC14 and AC20

| AC | Result | Named tests (all PASS unless noted) | Gap |
| --- | --- | --- | --- |
| AC7 Platform refuses to start for each misconfiguration in 5.1, including a missing migration; Multica unchanged when `MULTICA_CLOUD_URL` is empty | Covered | Config: `TestLoadMissingRequiredVariableNamesIt`, `TestLoadBlankRequiredVariableCountsAsMissing`, `TestLoadSessionSecretLength`, `TestLoadMissingEmailConfigurationNamesIt`, `TestLoadReportsEveryMissingVariable`, `TestDevVerificationCodeIsRefusedInProduction`; schema: `TestMissingErrorListsVersions`; URL: `TestNewRefusesAnAppURLThatIsNotAnOrigin`; Multica off without URL: `TestPlanLimitGatesAreOffWithoutACloudURL` | None |
| AC8 No `mpa_session` means 404 on every `/api/platform/*` route; PAT, machine token and `multica_auth` are not accepted by the platform | Covered | `TestMiddlewareFailuresAreAll404`, `TestMiddlewareReadsOnlyTheSessionCookie`, `TestVerifyCodeSignsIn`, `TestRevocationTakesEffectOnTheNextRequest`, `TestCookieAttributes`; Multica ignores `mpa_session`: `TestAuthIgnoresPlatformSessionCookie` | None |
| AC9 Suspended workspace: members cannot read; the daemon gets no task; owner still sees billing and can export | Covered | Member refusal: `TestWorkspaceStateMatrixThroughRouter`, `TestWorkspaceStateUnknownStatusRefuses`, `TestRouterWiresTheWorkspaceStateIn`, `TestWorkspaceStateErrorIs503`; daemon: `TestClaimRefusalForRefusesNonActiveWorkspaceForEveryRunKind`; owner billing: `TestBillingRoutesStayOpenToOwnerAndAdmin`; export: `TestExportOfASuspendedWorkspaceCompletes` | None |
| AC10 Each quota gate returns 402 `plan_limit_reached`; two concurrent creations at the limit: one succeeds | Covered | `TestPlanLimitAgentsConcurrentCreationsAtOneSlot`, `TestPlanLimitRuntimesConcurrentRegistrationsAtOneSlot`, `TestIssueCountLimitSerializesConcurrentCreatesAndDeleteFreesCapacity`, `TestMemberGroupGateTwoConcurrentCreationsAtTheLimit`, `TestMemberGroupGateOverTheLimitIsPlanLimitReached`, `TestPlanLimitAgentsUnderAndAtTheLimit` | None |
| AC11 Invitations refused when seats are full; a confirmed seat purchase lets the invitation through at once | Covered | `TestCreateInvitation_BlocksWhenPurchasedCapacityIsFull`, `TestCreateInvitation_BlocksOvercommittedCapacityWithoutOfferingSingleSeatSemantics`, `TestCreateInvitation_PlatformSampleCapacityFull`, `TestDBConcurrentInvitationsForTheLastSeat`, `TestDBSeatDecreaseBelowTheMemberCountRemovesNobodyAndOvercommits`, `TestApplyPaymentAddSeats` | None |
| AC12 Confirming the same order twice applies it once | Covered | `TestApplyPaymentAppliesOnce`, `TestDBConfirmingTwiceAppliesOnce`, `TestDBConcurrentConfirmationsApplyOnce`, `TestDBOrdersTwoConcurrentConfirmsApplyOnce` | None |
| AC13 Overdue workspace follows the grace, read-only and downgrade chain on a fake clock; back to `active` on payment at any step; no rows deleted | Covered | `TestLifecycleChainKeepsTheFixtureRowCounts`, `TestLifecycleEdgesFireAtTheBoundary`, `TestLifecyclePaymentAtEachStepReturnsActive`, `TestLifecycleSecondRunChangesNothing`, `TestLifecycleParallelRunsApplyEachEdgeOnce`, `TestLifecycleCancelAtPeriodEndGoesStraightToFree`, `TestLifecycleStateEmailIsSentOncePerTransition` | None |
| AC14 Existing billing screen shows the right plan, seats and VND price when `MULTICA_CLOUD_URL` points at the platform | **Partly covered** | Web: `packages/views/settings/components/billing-tab.test.tsx`, 67 pass, including the plan selector, the `payment_provider = manual` path, and the usage card; the contract tests check the response fixtures (`TestBillingFixturesMatchThePlatformsResponses`). | **Gap:** no named test asserts VND price formatting. No walkthrough on a live stack. |
| AC20 With the platform stopped, reads still work; invitations and share links return 503 `seat_capacity_unavailable`; account and workspace status still enforced from the shared DB | Covered | 503: `TestCreateInvitation_PlatformUnreachableIsUnavailable`, `TestJoinShareLink_PlatformUnreachableIsUnavailable`, `TestCapacityConsumeFailsClosedWhenCloudIsUnavailable`, `TestWriteSeatCapacityError_DefaultBranchIsUnavailable`; reads during outage: `TestSeatCapacityOutageLeavesWorkspaceReadsWorking` (new, `b64d9dec8`); status from DB: `TestWorkspaceStateErrorIs503`, `TestAccountGuardSuspendedHasNoExemptRoute` | None for the listed clauses. The live outage was not run (see the drill). |

## 4. Open items after G-R2

1. **AC14:** VND formatting has no named assertion, and there is no walkthrough on a live stack.
2. **Commercial drill, live part:** the platform start, `/healthz`, entitlement check through a running platform, compose stack, port 8095 and `mcn_` refusal were not run. Docker did not respond and there is no Redis here. These need an environment where Docker works.
3. **[PI] Docker image build** was not run.
4. **`pnpm test` red** (see G-R1 section 4): one real web contrast failure from project-page work, and load-related timeouts elsewhere. Not rerun for this gate.
5. **Buyer details on billing orders are not cleared** and **export storage objects are not deleted** by workspace teardown. These are listed in `docs/platform-operations.md` section 12 as open, and the R2 tests do not cover them.
6. **Leftover `multica_test_*` databases** from earlier `IsolatedDB` runs remain on the test server. This needs an owner decision to drop them. It is not part of this gate.
7. **Pre-existing, not R2:** `server/internal/integrations/lark` tests (`TestLegacyReceiptBackfill*`) use `IsolatedDB`, which creates a database whose name does not end in `_test`. The `./internal/integrations/...` package was not run for this gate.
8. **Decision D49:** bank details are empty by the owner's decision. Real payments cannot start until `BILLING_BANK_*` are set (`docs/platform-operations.md` section 6).

## 5. Commits in this gate

- `b64d9dec8` test(platform): pin workspace reads during a seat-capacity outage (AC20)
- This record: committed separately.
