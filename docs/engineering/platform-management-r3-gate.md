# R3 gate (P40): record

Date: 2026-11-17. Checkout: `main`, commit `e25a379c0` before this record. Test database: `multica_platform_test`, reached through `scripts/require-test-database.sh` with the DSN built from `.env` inside each command. The shared `multica` database was not used. Every package ran with `-p 1` and `-count=1`. The test database is at migration head (953 files and 953 rows).

## 1. Commands and results

| Check | Result |
| --- | --- |
| `server ./internal/auth/...` | pass: 170 tests, 0 fail |
| `server ./internal/mfa/...` | pass: 8 tests, 0 fail |
| `server ./internal/sso/...` | pass: 19 tests, 0 fail |
| `server ./internal/workspacepolicy/...` | pass: 32 tests, 0 fail |
| `server ./internal/workspaceguard/...` | pass: 84 tests, 0 fail |
| `server ./internal/middleware/...` | pass: 357 tests, 0 fail |
| `server ./internal/handler` with `-run 'SSO\|Sso\|MFA\|Mfa\|Policy\|ShareLink\|Invitation\|PAT\|Pat\|Viewer\|Domain\|Account\|Token'` | pass: 491 tests, 0 fail |
| `go build ./...` and `go vet` on the touched server packages | exit 0 |
| `server ./internal/handler` (full package) | 6857 pass, 3 fail. See section 2 |
| `platform-management ./...` | pass: 1312 tests, 0 fail, 35 packages |
| `pnpm catalog:build`; console `vitest run` | pass: 16 files, 204 tests |
| console `tsc --noEmit` | exit 0 |

The full-package run used the same DSN, with `PLATFORM_TEST_DATABASE_URL` set to it for the platform module.

## 2. Full handler package: the three failures

**Stays red on purpose: two squad repro tests.** They are reproduction tests for a bug that is not yet fixed on main. They are not part of the R3 gate and are not regressions from R3.

- `TestReproSquadRequestMovedToInReviewByMemberGetsReviewerRun` (`server/internal/handler/issue_workflow_in_review_repro_test.go`). The test expects the in_review reviewer to be assigned and to get a run. Main does not do that. The test comes from commit `d0963d860`, which predates the R3 range.
- `TestReproSquadRequestMovedToInReviewByLeaderRunGetsReviewerRun`. The test expects a squad leader to assign an agent outside its squad. Main returns 403 for that, by the rule from `b4e53abe7`. The test is in conflict with that rule.

Neither test is affected by the R3 commits (`8301f8adb^..c06265ee4`, four commits). Both are red when run alone.

**Flaky under the full package: `TestClaimNextDueSeatCapacityIntentIsExclusiveAcrossReplicas`.** The full run failed with `claimed=2 empty=0, want 1/1`. The claim query `ClaimNextDueSeatCapacityIntent` is not scoped to a workspace, and it uses `FOR UPDATE SKIP LOCKED`. Two `release` rows left by other tests in the shared test database were already due, so both claimers correctly took one row each. The test asserted on totals, which is a test-design problem, not a claim-code problem.

Running the test alone three times, on `multica_platform_test`, `-p 1`:

- `scratchpad/gate/seat_alone_1.txt`: PASS
- `scratchpad/gate/seat_alone_2.txt`: PASS
- `scratchpad/gate/seat_alone_3.txt`: PASS

The fixture is changed in the working tree (`server/internal/handler/seat_capacity_test.go`, not committed with this record). Each claimer now drains every due row, and the test asserts that no row is claimed twice and that its own row is claimed once. The fixture change is outside the test files named by P40, so its owner decides whether it is committed.

## 3. Acceptance criteria AC15 to AC19

| AC | Result | Tests | Gap |
| --- | --- | --- | --- |
| AC15 MFA gives no session before the second factor; pending token calls no other API | Covered, with one gap | `TestMFAPendingTokenIsNotASession`, `TestAuthRefusesMFAPendingToken` (middleware), `TestMFAVerifyCodeGivesAPendingTokenNotASession`, `TestMFAVerifyPendingTokenWorksOnce`, `TestMFAVerifySixthWrongCodeFailsTheToken`, `TestAMRMFAVerifyAddsMFA`, `TestMFAPendingTokenCarriesNoSessionIdentity` | **Gap:** no route-level test that a pending token gets 401 on every API route. The WebSocket path is covered only by the test that the token has no `sub` claim. |
| AC16 SSO callback with an email outside a verified domain is refused | Covered | `TestSSOLoginRefusesAnEmailOutsideTheVerifiedDomains`; `TestSSOCompleteRefusesWhatAC17Lists` in `server/internal/sso/oidc_test.go` (covers unverified email; the name is misleading) | None |
| AC17 `require_sso` blocks domain members in that workspace only; owner not blocked | Covered | `TestSSORequireSSORefusesOtherSignInMethods`, `TestSSORequiredDecision` | None |
| AC18 workspace-bound PAT refused in another workspace; read-only PAT refused on every write | Covered over HTTP. **Open over WebSocket** | `TestPATRestriction_BoundTokenWorksInItsWorkspaceAndIsRefusedInAnother`, `TestPATRestriction_ReadOnlyTokenReadsButDoesNotWrite`, `TestRequireWorkspaceMember_RefusesBoundPATOfOtherWorkspace`, `TestAuth_ReadOnlyPAT_SafeMethodsPass_WritesRefused`, `TestCheckWorkspaceAccess_TokenBinding` | **Open:** the WebSocket PAT resolver returns only a user ID (`server/internal/realtime/hub.go:38-39`). `HandleWebSocket` checks membership only, so PAT binding and read-only are not enforced there. |
| AC19 viewer refused on every workspace write, except the allow-list; cannot register a runtime | Partly covered | `TestViewerIsRefusedEveryWriteButTheAllowList` (7 routes), `TestCheckWorkspaceAccessRefusesAViewerWrite` (hand-written pattern list), `TestDaemonRegisterRefusesAViewer`, `TestUploadFileRefusesAViewer` | **Gap:** no test walks the real router's route table, which the spec requires. |

## 4. Security table (spec section 11)

| Row | Result | Tests | Gap |
| --- | --- | --- | --- |
| Staff account compromised | Mostly covered | `TestVerifyCodeSignsIn` (cookie flags), `TestMiddlewareFailuresAreAll404` (12-hour token), `TestRevocationTakesEffectOnTheNextRequest`, `TestMiddlewareReadsOnlyTheSessionCookie` (no PAT), `TestMiddlewareAllowsAnIPInsideTheCIDRs`, `TestStaffWithoutConfirmedMFACannotSignIn` | **Gap:** no test that tenants can see staff actions. |
| Internal platform routes called from outside | **Gap** | Only `TestLoadPortDefaultsAndOverrides` (config default port). | No test that the console listener refuses `/api/v1/internal`, or that 8095 is not published. `platform-management/Dockerfile:62` has `EXPOSE 8095`, which does not publish it. |
| Console leaks tenant content | Covered | `TestScannerFindsContentTables`, `TestConsoleFilesAreScannedAndNameNoContentTable` | None |
| Audit log edited or deleted | Partly covered | Multica: `TestAuditEventQueriesOnlyAllowedUpdates`, `TestUpdateGuardRejectsOtherUpdates` | **Deviation:** the spec says no UPDATE query; `server/pkg/db/queries/audit_event.sql` has two UPDATE statements (label renames), and the test allows exactly those two. **Gap:** no platform-side `.sql` test. |
| Secret reaches the log | Covered | `TestRecordWritesRedactedRow` (server), `TestRedactNestedAndNoMutation`, `TestRecordRedactsAndWrites` (platform) | None |
| Locked user still runs agents | Covered by test names | `TestAccountGuardDecisionTable`, `TestDaemonAuthSessionRevokedCheck`, `TestClaimGateRefusalCodePerStatus` | Each `mat_` and `mdt_` row was not checked line by line. |
| Stolen token survives revocation through renewal | Covered | `TestAuthGuardRunsBeforeSessionRevokedCheck`, `TestRenewSessionTokenKeepsAMRAndAuthTime`, `TestRenewSessionTokenDoesNotAddAMRToLegacyToken` | None |
| Stale guard cache | **Gap** | `accountguard.TTL = 60s` (platform) has no test asserting the bound. | Add a TTL assertion. |
| SSO claims another person's email | Covered | `TestSSOLoginRefusesAnEmailOutsideTheVerifiedDomains`, `TestSSOCompleteRefusesWhatAC17Lists` | None |
| Owner locked out by `require_sso` | Covered | `TestSSORequireSSORefusesOtherSignInMethods`, `TestSSORequiredDecision` | None |
| MFA bypass | Covered | `TestAuthRefusesMFAPendingToken`, `TestMFAPendingParseRefusesSessionTokens`, `TestAMRMFAVerifyAddsMFA`, `TestRenewSessionTokenDoesNotAddAMRToLegacyToken` | None |
| TOTP brute force and reuse | Covered | `TestMFAVerifySixthWrongCodeFailsTheToken`, `TestAttemptLimiterFailsTheTokenOnTheSixthAttempt`, `TestMFAVerifyTOTPCodeCannotBeUsedTwiceInOneStep`, `TestATOTPStepCannotBeUsedTwice` (platform) | None |
| Fake or repeated payment confirmation | Covered for R2 and R3 | `TestApplyPaymentAppliesOnce`, `TestDBOrdersTwoConcurrentConfirmsApplyOnce`, `TestPlatformBillingConfirmTwiceAppliesOnce`, `TestPlatformBillingOrderRoutesAdmitBillingAndAdminOnly` | The webhook signature row applies to R4, which is not built. |
| Quota race | Covered | `TestPlanLimitAgentsConcurrentCreationsAtOneSlot`, `TestIssueCountLimitSerializesConcurrentCreatesAndDeleteFreesCapacity`, `TestMemberGroupGateTwoConcurrentCreationsAtTheLimit` | None |
| Export link leaked | Partly covered | `TestDownloadLinkIsAbsentAfterExpiry`, `TestNoLinkUnlessDone`, `TestExpiryIsSevenDaysAfterFinish` | **Gap:** the presigned URL's TTL is not tested (`server/internal/handler/workspace_export.go:207-211`), and export requests being audited is not verified. |
| Account deletion orphans a workspace | Covered | `TestDeletionSoleOwnerOfSharedWorkspaceIs409AndChangesNothing`, `TestAccountDeletionJobKeepsOtherMembersAndRefusesToOrphanAWorkspace` | None |
| Agent injects a call to a sensitive route | Partly covered | `TestRequireHumanActor_BlocksMachineCredentials`, `TestRequireHumanActor_AppliedViaChiRouterUse` | Route-level coverage is not verified. |

## 5. Open follow-ups from R3

- **Web and desktop screens for SSO, MFA and policies: open.** No such UI exists in `packages/views`.
- **Mobile SSO sign-in: open.** `apps/mobile` has no SSO code.
- **WebSocket PAT resolver (binding and read-only): open.** See AC18.
- **`sso_domain` cleanup on workspace deletion: closed by test.** `TestSSODeletingAWorkspaceRemovesItsSSORows` passes in the handler subset. Its comment still says "Open item from P35"; the comment is stale.
- **Route-table walk test for the viewer rule (AC19): open.**
- **Route-level test for the pending token (AC15): open.**
- **Console-listener and port-exposure test (security row): open.**
- **Account-guard cache TTL test (security row): open.**
- **Platform-side audit `.sql` test (security row): open.**
- **Export presign TTL test (security row): open.**

## 6. User documentation (apps/docs)

Pages added in English: `sso.mdx`, `two-factor-auth.mdx`, `workspace-policies.mdx`. `members-roles.mdx` now covers the `viewer` role, and `meta.json` lists the new pages. The pages state what is not built: no web or desktop screens for SSO, MFA or policies, and no mobile SSO sign-in. Other locales are not written.
