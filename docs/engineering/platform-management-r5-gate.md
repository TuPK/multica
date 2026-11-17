# R5 gate (Q17): record

Date: 2026-11-17. Checkout: `main`, base commit `dce534e4a` (the last R5 commit before this record). Spec: `platform-management-spec.md` section 7b, acceptance criteria AC21 to AC48, section 13 (test bundle), section 11 (security). Plan: `platform-management-plan.md` step Q17.

Test databases: the platform suite ran against `multica_r5_test`; the server packages ran against `multica_platform_test`. Both were reached through `scripts/require-test-database.sh` with the DSN built from `.env` inside each command. The shared `multica` database was not used, and no database was created by hand. The shared PostgreSQL container (`multica-postgres-1`) had exited; it was started with `make db-up`, and its data volume was kept. Both test databases were at migration head: 953 to 969 are recorded in `schema_migrations`.

No dependency install was run; `node_modules` was present. `pnpm catalog:build` was run first, as the Makefile requires for Go tests.

## Verdict

See section 8 for the closing update: every acceptance criterion is now covered by a named test, and the checks are green except the two squad repro tests. The gate is closed for the criteria; the security rows and items in section 8.2 and 8.3 remain open.

The verdict below was written before the closing update.

The R5 gate was **not closed** at that point. Every check runs green except two reproduction tests that predate R5 (both red on main since the R3 gate). Thirteen more acceptance criteria are only partly covered by tests. The open list is in section 6. Nothing in this record fixes product code; each gap needs a decision from the owner.

### Update after the closing commits (2026-11-17)

Three features that this record marked as not built are now built, each with its tests (test-first, seen failing before the change):

| Gap | Closed by | Tests (named) | State |
| --- | --- | --- | --- |
| 6.1 item 1: staff verification of business profiles (AC25, rule 12) | `89ef83695` (platform route, audit, service); `f3ecb1507` (console verify control) | `TestVerifySetsVerifiedAtAndVerifiedBy`, `TestUnverifyClearsVerification`, `TestVerifyRouteAllowsBillingAndAdminOnly`, `TestDBVerifyPersistsAndUnverifyClears`; console `TenantDetail.test.tsx` | Closed, with one follow-up (below) |
| 6.1 item 3: `plan_currency_locked` (AC37) | `6156d9875` (platform); `f3ecb1507` (console message) | `TestPutPlanRefusesACurrencyChangeOnAPlanWithASubscription`, `TestPutPlanRefusesACurrencyChangeOnAPlanWithAnOrder`, `TestDBCurrencyLockedWithBillingOrder`; console `Plans.test.tsx` | Closed |
| 6.1 item 2: scheduled price change never reaches the customer (AC40) | `99ccba960` (platform summary and Multica plans response) | `TestSummaryShowsScheduledPriceChangeOnlyForThePlanInUseAfterNotice`, `TestDBSummaryScheduledPriceChangeReadsTheStore`, `TestListWorkspacePlansShowsScheduledPriceChangeAfterNoticeOnly` | Closed; the Multica banner needed no change |

Re-run of the Q17 checks after these commits:

| Check | Result |
| --- | --- |
| Platform suite on `multica_r5_test` (`go vet ./...`, `go test -p 1 ./...`) | pass: `go vet` exit 0; 35 packages `ok` |
| `server/internal/handler` on `multica_platform_test` (full package) | fails the same 2 pre-existing squad reproduction tests (6.1 item 6); all others pass |
| Migration checks (`TestMigrationFilesHaveMatchingDirections`, `TestMigrationNumericPrefixesAreUnique`, `TestEveryConcurrentUpBuildHasCleanup`, `TestUserColumnRegistry`) | pass |
| Console vitest (`platform-management/console`) | pass: 17 files, 234 tests |
| Console `tsc --noEmit` | exit 0 |
| `pnpm -C packages/core test` / `typecheck` | pass: 374 files, 5566 tests, 1 skipped / exit 0 |
| `pnpm -C packages/views test` / `typecheck` | pass: 733 files, 8876 tests / exit 0 |
| `pnpm lint` | exit 0, 7 of 7 tasks; 23 warnings, 0 errors (unused eslint-disable directives in core and views) |

Items still open after this update:
- **Follow-up from the console verify control:** `TenantDetail` shows "Unverified" when the page loads, because no console route returns a tenant's billing profile. The status is correct only after a verify or unverify click in the same session. The read route (`GET /tenants/{id}/billing-profile`, spec 7b.1) is not built.
- 6.1 items 0, 3a, 3b, 4, 5, 6 remain as listed below.
- The ACs marked "Partial" in section 3 are unchanged, except that AC25, AC37 and AC40 now have the tests named above; their remaining gaps still stand.
- The console has no lint script, so the console itself was not linted.

## 1. Commands and results

| # | Command (run from the checkout, DSN from `.env`) | Result |
| --- | --- | --- |
| 1 | `scripts/require-test-database.sh` with `multica_r5_test`; `cd platform-management && go test -p 1 -count=1 ./...` | exit 0. 35 packages `ok`, 1 with no test files. Output: `scratchpad/gate/platform_suite_r5.txt` |
| 2 | `scripts/require-test-database.sh` with `multica_platform_test`; `cd server && go test -p 1 -count=1` on the 19 server packages that R5 touched with tests (list in section 2.4) | exit 1. 18 packages `ok`. `server/internal/handler` fails 2 tests: `TestReproSquadRequestMovedToInReviewByMemberGetsReviewerRun` and `TestReproSquadRequestMovedToInReviewByLeaderRunGetsReviewerRun`. Both are the reproduction tests recorded red in the R3 gate; they come from `d0963d860`, not from R5. All other handler tests pass. Output: `scratchpad/gate/server_suite_r5.txt` |
| 3 | `go test -p 1 -count=1 -v -run 'TestMigrationFilesHaveMatchingDirections\|TestMigrationNumericPrefixesAreUnique\|TestEveryConcurrentUpBuildHasCleanup\|TestUserColumnRegistry' ./internal/migrations/ ./cmd/migrate/` | pass: all 4 tests `PASS`, both packages `ok`, exit 0. Output: `scratchpad/gate/migration_checks.txt` |
| 4 | `pnpm -C platform-management/console test` | pass: 17 files, 225 tests |
| 5 | `pnpm -C platform-management/console typecheck` | exit 0 |
| 6 | `pnpm -C packages/core test` | pass: 374 files, 5566 tests, 1 skipped |
| 7 | `pnpm -C packages/core typecheck` | exit 0 |
| 8 | `pnpm -C packages/views test` | pass: 733 files, 8876 tests |
| 9 | `pnpm -C packages/views typecheck` | exit 0 |
| 10 | `pnpm lint` | exit 0. 7 of 7 tasks successful; turbo served it from cache (`FULL TURBO`), so this is the cached result of an earlier identical run |

Raw outputs are in `scratchpad/gate/` in the working session (`platform_suite_r5.txt`, `server_suite_r5.txt`, `migration_checks.txt`, `console_vitest.txt`, `console_tsc.txt`, `core_vitest.txt`, `core_typecheck.txt`, `views_vitest.txt`, `views_typecheck.txt`, `lint.txt`).

## 2. Schema, migrations and cleanup

### 2.1 `platform-management/internal/schemacheck`

The required list contains 953, 954, 955, and 961 to 968, as the plan requires. `RequiredListsThePlanAndSubscriptionMigrations`, `EveryR5PlatformTableMigrationIsRequired` and `R5ColumnMigrationIsRequired` pass. Two points for the owner:

- **969 is not in the list.** Migration 969 (`billing_order.expires_at` becomes nullable, for usage invoices that do not expire) is part of R5 and is applied by `migrate up`. The platform does not refuse to start when 969 is missing, so a database without it would start and then fail when it writes a usage invoice. Adding 969 to `Required` is a one-line change, and it is a product decision, so it is not made here.
- 956 to 960 (the account-group tables) are Multica-side tables that the platform does not read, so they are correctly not in the platform list.

### 2.2 Concurrent indexes and cleanup entries

Every R5 concurrent index has a cleanup entry in `server/cmd/migrate/main.go` (`concurrentIndexCleanups`): 955, 957, 958, 960, 962, 963, 965, 967, 968. `TestEveryConcurrentUpBuildHasCleanup` passes. Each is one statement in its own file, as AGENTS.md requires.

### 2.3 Migration files

`TestMigrationFilesHaveMatchingDirections` and `TestMigrationNumericPrefixesAreUnique` pass. `TestUserColumnRegistry` passes; that is the sổ đăng ký cột người dùng (section 4.6), so each new `user_id` column is classified.

### 2.4 Server packages run (check 2)

`cmd/migrate`, `cmd/server`, `internal/accountprovision`, `internal/auditaction`, `internal/auth`, `internal/entitlement`, `internal/handler` (full package), `internal/integrations/{dingtalk,lark,slack,telegram,wecom}`, `internal/membergroup`, `internal/middleware`, `internal/service`, `internal/sso`, `internal/workspaceguard`, `internal/workspacepolicy`, `internal/workspaceprovision`. The `authtest`, `ssotest` and `pkg/db/generated` packages have no test files.

## 3. Acceptance criteria AC21 to AC48

Mapping is by test name, checked against the test index of the 107 R5 test files. "Covered" means a test with that name asserts the behavior; bodies were not all re-read, so treat "covered" as "named and present", and the gaps as the real list.

| AC | Status | Tests | Gap |
| --- | --- | --- | --- |
| AC21 individual/business purchase rules, profile errors, seat cap | Partial | `CheckoutRefusesABusinessPlanForAnIndividualWorkspace`, `CheckoutLetsABusinessWorkspaceBuyABusinessPlan`, `CheckoutRefusesAPlanOnlyForIndividualsToABusinessWorkspace`, `ChangeToBusinessNeedsACompleteProfile`, `TaxCodeFormat`, `ProfileAPIRefusesGoingBackOnABusinessOnlyPlan`, `DBChangeBackIsRefusedOnABusinessOnlyPlan`, `ScheduledSeatsAboveTheIndividualLimitAreRefused` | **Gap:** `PLATFORM_INDIVIDUAL_MAX_SEATS` is checked only on plan change (`billingapi/planchange.go:190`). Checkout and seat purchase do not check it, so an individual workspace can buy seats above the cap |
| AC22 upgrade order, idempotency, pending order, quote_changed | Covered | `ProrateChangeTable`, `PlanChangeRepeatedKeyReturnsTheSameOrder`, `ApplyingAChangeOrderTwiceChangesTheSubscriptionOnce`, `ChangeOrderRefusesAnotherPendingOrder`, `ChangeOrderRefusesAMismatchedAcceptedAmount`, `PlanChangeRefusesAnAcceptedAmountThatNoLongerMatches` | none |
| AC23 downgrade, seat decrease, pending_change, seat to usage at period end | Partial | `DowngradeIsStoredForTheNextRenewalAndNotCharged`, `RenewalIsPricedFromAndClearsThePendingChange`, `RenewalWithAMismatchedOrderIsRefused` | **Gap:** no test that a seat decrease below the member count removes nobody; **gap:** no test that a switch from a seat plan to a usage plan applies only at the end of the paid period |
| AC24 cancel and resume, past_due refusal, audit | Covered | `CancelAndResumeFollowTheStateTable`, `TrialingResumesBeforeTheTrialEnds`, `CancelRefusesAStatusOutsideTheTable`, `LifecycleCancelAtPeriodEndGoesStraightToFree`, `CancelThenResumeWritesOneAuditRowEach` | none |
| AC25 billing.manage / billing.view, machine 403, summary excludes profile, staff billing verifies, support does not | Partial | `CloudWorkspaceSubscriptionWritesRequireManagerRole`, `CloudWorkspaceSubscriptionsRejectMachineActorsAtHandler`, `ProfileAPIRefusesAMemberAndAViewer`, `ProfileAPIHidesTheWorkspaceFromANonMember` | **Gap (product):** a group holding `billing.manage` or `billing.view` is refused on the subscription routes, because the handlers in `server/internal/handler/cloud_billing.go` still require owner or admin (the comment at line 207 says the key is "not enforced on any route yet"); **gap (product, see 6.1):** staff verification of a business profile is not built: no console route, no `billing_profile.verified` audit action, and no code sets `verified_at` except SQL. **Gap:** no test that the profile is absent from `summary` and from the account export |
| AC26 members.invite group rules, invite role limit, removal takes effect at the next request | Partial | `InvitationMembersInviteCannotAssignAdmin`, `ShareLinkMembersInviteCannotCreateAdminLink`, `MemberGroupRemovalTakesEffectOnTheNextRequest`, `MemberGroupRoutesKeyByRouteMatrix` | **Gap:** no test that a member with `members.invite` cannot change a role or remove a member |
| AC27 per-key route set, owner/admin-only routes, suspended and pending_deletion | Partial | `MemberGroupRoutesKeyByRouteMatrix`, `GroupMemberRefusedWhileSuspendedOrPendingDeletion`, `MemberGroupRoutesAreForOwnerAndAdmin`, `PermissionRoutesAreWiredByKeyInTheRouter` | **Gap (product):** the router opens the `billing.*` routes to the keys, but the subscription handlers refuse key holders (see AC25), so the key does not do what the matrix says for those routes |
| AC28 viewer with six keys refused; removal and account deletion delete group rows; user-column registry | Covered (one note) | `ViewerGroupStillRefusedOnWrites`, `RevokeAndRemoveMemberDeletesGroupRows`, `RevokeAndRemoveMemberRollsBackGroupRows`, `MemberGroupDeleteRemovesTheMemberRows`, `TestUserColumnRegistry` | Note: account deletion removes memberships through `revokeAndRemoveMember` (`server/internal/handler/me_deletion.go`), so the group rows go with the same transaction; no test drives the deletion job itself to assert it |
| AC29 member_groups gate: 0 is 402, over the limit is 402, concurrent at the limit, lowering keeps groups, no limit when `MULTICA_CLOUD_URL` empty | Covered | `MemberGroupGateLimitZeroIsFeatureUnavailable`, `MemberGroupGateOverTheLimitIsPlanLimitReached`, `MemberGroupGateTwoConcurrentCreationsAtTheLimit`, `MemberGroupGateLoweringThePlanKeepsExistingGroups`, `MemberGroupGateOffWithoutAnEntitlementProvider`, `AddedGatesAreOffWhenDisabled`, `BuildMemberGroupsGateFollowsThePlan` | none |
| AC30 one audit row per change, rollback on audit failure, 404 for another workspace's group, 409 name, 400 permission key, console sees counts only | Covered | `MemberGroupAuditRowsOnePerChange`, `MemberGroupAuditFailureRollsTheChangeBack`, `MemberGroupOfAnotherWorkspaceIsNotFound`, `MemberGroupNamesAreUniquePerWorkspace`, `MemberGroupNormalizePermissionsRejectsAnUnknownKey`, `DBTenantDetailCountsAccountGroupsWithoutTheirNames` | none named for the old and new permission values in the audit row (not re-read) |
| AC31 unlimited plan: no counted limits, API rate ceiling, downgrade keeps data | Covered | `CatalogueUnlimitedPlanMatchesSpec7b3`, `BuildUnlimitedPlanIsOffOnEveryCountedGate`, `BuildUnlimitedPlanAPIRateIsOff`, `BuildUnlimitedPlanCapStaysOffWithACeiling`, `BuildAfterDowngradeFromUnlimitedUsesTheNewLimits`, `LoadAPIRateCeiling` | none |
| AC32 payg activation, profile required, seat fields null, prices never list usage plans | Partial | `UsageActivationNeedsTheProfileFieldsOfItsKind`, `UsagePlanSummaryHasNoSeatCapacity`, `PricesOfAPlanWithNoSeatPriceAre503`, `WorkspacePlansReturnsUsageRatesAndDiscount` | **Gap:** no test that `GET /prices` never returns a usage plan's price; **gap:** no test that invitations and share links are not blocked by seats on a usage plan |
| AC33 daily rating: three clocks, 28 to 31 day months, 100 TB, discount, one row per day and meter, zero rows for suspended | Partial | `ChargeKnownValues`, `ChargeTableAgreesWithTheExactReference`, `ChargeRoundsHalvesUpOnlyAtTheBoundary`, `DaysForIsOneForTasksAndTheUTCMonthForStockMeters`, `BusinessDiscountAppliesOnlyToBusinessAndWhenVerifiedWhenTheFlagIsOn`, `DBRunningTwiceOrInParallelWritesNoSecondRow`, `DBADayWithoutDataIsNotChargedAndALateRowIsChargedWhenItArrives`, `DBSuspendedAndPendingDeletionWriteZeroRowsAndReadOnlyIsCharged` | **Gap:** no test named for the 100 TB boundary (no overflow). The month-length and rounding tests cover the other parts |
| AC34 monthly invoice: VAT, minimum carried over, final invoice, one order under parallel runs, usage order does not expire | Covered | `UsageInvoiceMonthlyTotalsVAT`, `UsageMathVATRounding`, `UsageInvoiceMinimumCarriesOver`, `UsageInvoiceFinalWhenLeavingUsage`, `UsageInvoiceRunTwiceAndParallelCreatesOneOrder` | the "does not expire" rule is covered by migration 969 and the order code, not by a named test |
| AC35 credit limit, temporary invoice, 80 percent notice once, past_due to free, unpaid blocks a paid purchase, admin-only write-off with reason | Partial | `UsageInvoiceCreditLimitTempInvoiceAndEightyPercentNoticeOnce`, `UsageInvoiceCreditLimitHasNoMinimum`, `UsageInvoiceUnpaidBlocksPaidPurchase` (409 `usage_invoice_unpaid`), `LifecyclePaymentAtEachStepReturnsActive` | **Gap:** no named test that a write-off is refused for every role except `admin`, and needs a reason |
| AC36 admin-only price changes, billing read, support and no session get 404, PUT price field 400, one scheduled change under concurrency | Covered | `RoutesAdmitOnlyTheirRoles`, `WritesNeedTheConsoleOrigin`, `PriceErrorsCarryTheirFields`, `RefusePriceFieldsRefusesEachPriceField`, `PlanPutWithAPriceFieldIsRefusedBeforeTheConsole`, `DBConcurrentSchedulesLeaveOneScheduledChange`, `TestSecondScheduleIsRefusedWithPriceChangePending` | none |
| AC37 price_invalid with field, confirm_large_change, plan_currency_locked | Partial | `ScheduleValidation`, `LargeChangeNeedsConfirmation`, `PaidPricesAreWholeDong`, `PriceErrorsCarryTheirFields` | **Gap (product, see 6.1):** `plan_currency_locked` (409) is not in the code or in any test; **gap (product):** `PLATFORM_PRICE_CONFIRM_PCT` and `PLATFORM_PRICE_MAX` are not read from the environment (fixed defaults are used) |
| AC38 notice for increases, one email per owner, no mail to pending_deletion, decreases immediate | Partial | `TestIncreaseWithPaidSubscribersNeedsTheNoticePeriod`, `ADecreaseSendsNoNotice`, `ADecreaseTakesEffectInTheRequest`, `AnIncreaseForAPlanWithoutPaidSubscribersTakesEffectInTheRequest`, `NoticeIsSentOnceEvenWhenTheJobRunsAgain`, `AFailedSendIsRetriedByTheNextRunWithoutDuplicates` | **Gap:** no test that a `pending_deletion` workspace gets no price-change email |
| AC39 orders keep the old amount, quote_changed on seats or plan change, rating uses the 00:00 UTC price even when run late | Partial | `PurchaseRefusesAStaleQuote`, `CancelBeforeTheEffectiveDateKeepsThePrice`, `DBThePriceSnapshotStaysWhenThePlanPriceChangesLater`, `PlanChangeQuoteCarriesTheTargetUnitPrice` | **Gap:** no named test that the rating of day D uses the price in force at 00:00 UTC of D when the rating runs after a later change |
| AC40 plan_price_apply idempotent and parallel, cancel sends a correction, GET plans shows an upcoming change only after the notice, one audit row per step | **Gap (product, see 6.1)** | `DBParallelApplyAppliesOnceWithOneVersionBump`, `TestApplyJobTwiceInSequenceChangesNothing`, `TestApplyJobAppliesOnceAtTheEffectiveDate`, `DBNoticeThenCancelSendsOneCorrection`, `TestCancelAfterTheNoticeSendsOneCorrection`, `ScheduleApplyAndCancelEachWriteOneAuditRow` | **Gap:** no code emits `scheduled_price_change` on the plans response. Core parses the field and `scheduled-price-change-banner.tsx` renders it, but nothing on the server or platform side produces it, so the upcoming-price notice never reaches a customer |
| AC41 admin creates a personal account: user, active workspace, profile and plan; email code login with signup off; no onboarding; first login shows consent_required | Partial | `TestAccountEndToEnd` (asserts the user, owner, subscription, no sign-in codes, staff not a member, no state row, one welcome, the `account.provisioned` audit row) | **Gap:** `TestAccountEndToEnd` stops before the sign-in: no test that a provisioned user can sign in by email code with `ALLOW_SIGNUP` off, skips onboarding, or sees `consent_required` on first login |
| AC42 business account: verified profile, activation (trial, comp_reason, or usage), grant order, `account_activation_required`, `plan_audience_mismatch`, `account.provisioned` seen by the owner | Covered | `FinalizeWritesBillingProfileSubscriptionAndGrant`, `AccountProvisionHappyPath`, `TestAccountEndToEnd`, `TestCheckPlanTable` (case "paid plan without trial or comp_reason", `validate_test.go`) and `TestAccountRequestPlanChecksRunOnCreate`, `CheckAudienceMatrix`, `ProfileAPIChangesToBusinessAndWritesTheAuditRow` | none |
| AC43 existing email 409 with no change; blocked domain `email_domain_blocked`; resend returns the live request; two concurrent requests create one; a user who signs up first gives `email_exists` | Covered | `AccountRequestEmailOfAnExistingUserIsRefused`, `AccountRequestSecondRequestForALiveEmailReturnsTheFirst`, `AccountRequestDBParallelCreatesForOneEmailWriteOneRow`, `AccountProvisionBlockedDomainFails`, `AccountProvisionEmailExistsFails`, `AccountRequestInsertThatLosesTheRaceReturnsTheWinner` | none |
| AC44 admin-only routes, 429 per hour, cancel only when pending, one audit row per step | Covered | `AccountRequestRoutesAreAdminOnly`, `AccountCreateRateLimitRefusesTheRequestBeyondTheLimit`, `AccountCreateDBRateLimitAnswers429WithRetryAfter`, `AccountRequestCancelRefusesEveryStateButPending`, `AccountRequestCancelPendingWritesTheAuditRow`, `AccountRequestDBCreateStoresTheRowAndTheAuditRow` | none |
| AC45 provisioning and finalize idempotent and parallel; three tries on a transient error; `finalize_failed` keeps the user and workspace | Covered | `FinalizeIsIdempotentAndParallelSafe`, `FinalizeRetriesTemporaryErrorThreeTimes`, `FinalizePermanentErrorFailsAtOnce`, `FinalizeFailsAfterUserCreatedKeepsAccount`, `AccountProvisionParallelRunsCreateOnce`, `AccountProvisionSecondRunChangesNothing`, `AccountProvisionTransientFailureRetriesThenFails` | none |
| AC46 no session, password, code or token for the customer; staff not a member; console reads no workspace content; welcome sent once, no token, 3 resends a day; deletion anonymises `email` and `display_name` | Partial | `WelcomeSentExactlyOnce`, `WelcomeResendLimitPerDay`, `WelcomeRetriesSendErrorsThreeTimes`, `WelcomeTemplateHasNoSecrets`, `TestAccountEndToEnd`, `AccountDeletionJobAnonymisesAccountRequests` | **Gap:** no named test that the console has no route that reads a workspace's content |
| AC47 admin-created account is locked, logged out and deleted like any account; its workspace is under the R2 states and gates | Partial | `AccountGuardDecisionTable`, `AccountGuardSuspendedHasNoExemptRoute`, `AuthSessionRevokedCheck`, `AccountDeletionJobAnonymisesAndRemoves` | **Gap:** the cases are covered generically; no test starts from an admin-created account |
| AC48 regression: with the new settings empty and `MULTICA_CLOUD_URL` empty, new routes return the same errors as the existing routes; the client hides new features when fields are absent; no limit on groups | Partial | `CloudWorkspaceSubscriptionsDisabledByDefault` (summary and prices, flag off), `CloudBillingDisabledReturnsForbidden`, `AddedGatesAreOffInAPolicyThatLacksThem`, `MemberGroupsGateAbsentIsOff`, `AddedGatesAreOffWhenDisabled`, `CatalogueEntriesPassValidation` | **Gap:** no test compares the new R5 routes (plan change, usage, profile, price, group) with the existing routes' errors when `MULTICA_CLOUD_URL` is empty. The disabled-flag test covers only the two old read routes |

Summary: 14 AC are covered by name, 13 are partial, and 1 (AC40) has a product gap that no test can close until code is added. The gaps are listed in section 6.

## 4. Security tables (spec section 7b, "Bảo mật và riêng tư")

Thirty-five rows in five groups. The "test" column names the test that covers the row's measure; "gap" means no test name covers it.

### 4.1 Group 1 (subscriptions, 7b.1)

| Threat | Test | Result |
| --- | --- | --- |
| Self-declared business to get the discount | `DBBusinessDiscountFollowsVerificationWhenTheFlagIsOn`, `BusinessDiscountAppliesOnlyToBusinessAndWhenVerifiedWhenTheFlagIsOn`, `ChangingNameOrTaxCodeClearsTheVerification` | covered, but staff cannot set `verified_at` yet (6.1) |
| Tax code, address, invoice email leak | `ProfileAPIRefusesAMemberAndAViewer` (billing.view); not in summary or account export: none | **gap** |
| Member creates a paid order | `CloudWorkspaceSubscriptionWritesRequireManagerRole`, `CloudWorkspaceSubscriptionsRejectMachineActorsAtHandler`, `CloudSubscriptionPlanChangeRoleChecks` | covered |
| Price changes between quote and purchase | `PurchaseRefusesAStaleQuote`, `PlanChangeRefusesAnAcceptedAmountThatNoLongerMatches` | covered |
| Two tabs change the plan | `ChangeOrderRefusesAStaleVersion`, `ChangeOrderRefusesAnotherPendingOrder`, `ScheduleRefusesAStaleVersion` | covered |
| Seats reduced and the service kept as before | none | **gap** |

### 4.2 Group 2 (groups and permissions, 7b.2)

| Threat | Test | Result |
| --- | --- | --- |
| Escalation by inviting an admin | `InvitationMembersInviteCannotAssignAdmin`, `ShareLinkMembersInviteCannotCreateAdminLink` | covered |
| Self-escalation or escalation of others through groups | `MemberGroupRoutesAreForOwnerAndAdmin`, `PermissionRoutesAreWiredByKeyInTheRouter` | covered |
| Group id from another workspace | `MemberGroupOfAnotherWorkspaceIsNotFound`, `MemberGroupServiceGroupsOfAnotherWorkspaceAreNotFound` | covered |
| Rights kept after removal (no cache; rows deleted in the transaction) | `MemberGroupRemovalTakesEffectOnTheNextRequest`, `RevokeAndRemoveMemberDeletesGroupRows`, `TestUserColumnRegistry` | covered |
| Viewer writes through a group | `ViewerGroupStillRefusedOnWrites`, `MemberGroupRoutesKeyByRouteMatrix` | covered |
| Org structure leaked | `DBTenantDetailCountsAccountGroupsWithoutTheirNames`, `MemberGroupRoutesAreForOwnerAndAdmin`; group names kept out of the audit log: none | covered in part (the audit-log part has no test) |
| A new key opens a wrong route later | `MemberGroupRoutesKeyByRouteMatrix` (one test per key, route set by key) | covered |

### 4.3 Group 3 (usage billing, 7b.3)

| Threat | Test | Result |
| --- | --- | --- |
| Surprise invoice | `UsageInvoiceCreditLimitTempInvoiceAndEightyPercentNoticeOnce` | covered |
| Charged twice or missed | `DBRunningTwiceOrInParallelWritesNoSecondRow`, `RateDayChargesAMeterTheRatesDoNotNameAtZero` | covered |
| Rounding error or overflow | `ChargeRoundsHalvesUpOnlyAtTheBoundary`, `DaysForIsOneForTasksAndTheUTCMonthForStockMeters`; 100 TB boundary: none | **gap** (100 TB) |
| Business discount from self-declared tax code | `DBBusinessDiscountFollowsVerificationWhenTheFlagIsOn` | covered |
| Dispute over the numbers (stored snapshot) | `DBThePriceSnapshotStaysWhenThePlanPriceChangesLater`, `RateDayKeepsTheSnapshotOfEveryRow` | covered |
| "Unlimited" used to load the service | `BuildUnlimitedPlanCapStaysOffWithACeiling`, `BuildAPIRateCeilingDefaultsTo6000` | covered |
| Abandon a debt and open a new workspace | none | **gap** |
| Usage data reveals content | `TestQueriesOnContentTablesOnlyCountAndSum`, `TestEveryLatestQueryFiltersByWorkspace` | covered |

### 4.4 Group 4 (plan prices, 7b.4)

| Threat | Test | Result |
| --- | --- | --- |
| Mistyped price | `RefusePriceFieldsRefusesEachPriceField`, `ScheduleValidation`, `DryRunOverHTTPSavesNothing` | covered |
| Two admins at once | `DBConcurrentSchedulesLeaveOneScheduledChange`, `DBVersionIncreasesByOnePerEdit` | covered |
| Silent or backdated increase | `TestIncreaseWithPaidSubscribersNeedsTheNoticePeriod`, `TestCancelBeforeTheEffectiveDateKeepsThePrice` | covered |
| Admin account taken over or abused (MFA, reasons in audit) | `RoutesAdmitOnlyTheirRoles`, `DBEachWriteAuditsOnceWithTheStaffActor`; MFA on the price routes: none | **gap** (MFA) |
| Quote mismatch | `ChangeOrderRefusesAMismatchedAcceptedAmount`, `PlanChangeRefusesAnAcceptedAmountThatNoLongerMatches` | covered |
| Clock skew moves the price earlier or later | `TestUsageChangeAtMidnightIsAccepted`, `DBApplyWritesTheRowTheVersionAndTheAuditRow` | covered in part (the 00:00 UTC rule has no named test) |

### 4.5 Group 5 (account creation, 7b.5)

| Threat | Test | Result |
| --- | --- | --- |
| Staff keeps access to the customer's account | `TestAccountEndToEnd` (no sign-in codes, staff not a member) | covered |
| Mistyped email gives a stranger a workspace | `AccountCreateMapsServiceErrorsToTheSpecCodes`, `TestCheckPlanTable` | covered |
| Mass creation for free plans | `AccountCreateRateLimitRefusesTheRequestBeyondTheLimit`, `AccountCreateDBRateLimitAnswers429WithRetryAfter` | covered; MFA on the route: none (**gap**) |
| Taking over an existing account | `AccountRequestEmailOfAnExistingUserIsRefused`, `AccountProvisionEmailExistsFails` | covered |
| Two jobs create one user | `AccountProvisionParallelRunsCreateOnce`, `FinalizeIsIdempotentAndParallelSafe` | covered |
| Real user signs up before the job runs | `AccountProvisionEmailExistsFails` (`email_exists`) | covered |
| Third-party personal data kept | `AccountDeletionJobAnonymisesAccountRequests`; the profile copy removed after completion: none | **gap** (the copy removal) |
| Welcome email used for phishing | `WelcomeTemplateHasNoSecrets` | covered in part (no test on the link host) |
| Accounts that nobody uses | "never signed in" indicator in the console: none | **gap** |

Of the 35 rows, 5 have no test that covers the threat (seat reduction, debt bypass, MFA on the price routes, copy removal, unused-account indicator). 6 are covered only in part (tax-code leak to summary and export, the audit part of group privacy, the 100 TB boundary, the clock-skew rule, MFA on the account-creation route, and the welcome link host).

## 5. Copy, section 3.3, and the regression rule

### 5.1 Copy (seven locales)

- The R5 keys added to `packages/views/locales/{en,es,fr,ja,ko,zh-Hans,zh-Hant}/{agents,billing,settings}.json` have the same key set in all seven locales. The only differences are the `_one` plural forms that ja, ko and the two Chinese locales do not use; `parity.test.ts` passes.
- A heuristic scan of the R5 view components found no hardcoded English user text. One thrown error, `"Order not found"` in `packages/views/billing/order-page.tsx`, is a query error and is not shown as copy; it is recorded here, not changed.
- The console is English only, as the spec says. The console's R5 screens have their own tests (`Accounts.test.tsx`, `PriceSchedule.test.tsx`, `Plans.test.tsx`).
- Redundant descriptions: not reviewed screen by screen in this gate. The documentation pass does not add UI descriptions.

### 5.2 Section 3.3 (core files touched by R5)

The table in section 3.3 names most touched core files, but not these R5 edits (found from `git log a535887cf^..HEAD`):

- `packages/core/api/client.ts`, `packages/core/permissions/index.ts` (account groups, `e9b7e6961`)
- `packages/core/billing/workspace-orders-queries.ts`, `workspace-subscription-mutations.ts`, `packages/core/types/api.ts`, `billing.ts`, `index.ts` (usage reads and subscription management, `829960416`, `8aad3da56`, `1b2b30252`, `32a236e85`)
- `packages/views/billing/order-page.tsx` (usage card and usage invoices, `72aa7ad49`)
- `packages/views/runtimes/components/runtime-list.tsx`, `runtime-detail.tsx`, `runtime-detail-page.tsx` (account-group key gates, `d6d81536c`)
- `server/internal/handler/billing_orders.go` (usage invoices, `260437570`)
- `server/internal/handler/me_deletion.go` (account requests anonymised, `ab40ad0cc`)
- `server/internal/handler/plugin_action.go` (viewer refusal, `18a12da4d`)
- `server/internal/integrations/{slack,telegram,dingtalk,lark,wecom}/binding*.go` (viewer refusal at the binding redeem, `34f090c28`)

The section names the `packages/views/settings/` folder and `server/internal/workspaceguard/`, so those are covered. The `server/internal/service/company_*` and `packages/core/squads/` edits come from catalog commits, not R5, and are not listed. The owner can add the rows above to 3.3; this gate does not edit the spec.

### 5.3 Regression rule (AC48)

See the AC48 row in section 3. The regression check is partial: the old routes return the same errors with the flag off, and the client gates are off when the policy omits them, but the new routes have no comparison test.

## 6. Open items

### 6.1 Product gaps found by the gate (not fixed here)

0. **Group billing keys do not work on subscription routes** (AC25, AC27). `billing.view` and `billing.manage` are opened in the router, but the handlers in `cloud_billing.go` (including the usage read) still require owner or admin. A group member holding `billing.manage` is refused.
1. **[Closed, see the update above]** Staff verification of business profiles was not built. Spec 7b.1 rule 12 and the console's billing block need a verify action that sets `verified_at` and `verified_by`, with the audit actions `billing_profile.verified` and `billing_profile.unverified`. None exists in code: no route, no audit action, no console button. Until this exists, the business discount (D45) cannot be turned on from the product.
2. **[Closed, see the update above]** The upcoming price change never reached the customer. `scheduled_price_change` is parsed in `packages/core` and rendered in `packages/views/billing/scheduled-price-change-banner.tsx`, but no server or platform code emits it. AC40 names the response of `GET /api/workspaces/{id}/plans`.
3. **[Closed, see the update above]** `plan_currency_locked` was not built. AC37 requires a 409 when the currency changes on a plan that has subscriptions. Neither the code nor the tests contain it. `plan_version_conflict` (in the spec's error table) is also absent.
3a. **Price settings are not read from the environment.** `PLATFORM_PRICE_NOTICE_DAYS`, `PLATFORM_PRICE_CONFIRM_PCT` and `PLATFORM_PRICE_MAX` (spec 3.4, AC37, AC38) are not read by `config.go`; the code uses fixed defaults (30 days, 50 percent). `PLATFORM_INDIVIDUAL_MAX_SEATS` is read but enforced only on plan change (AC21).
3b. **No pay-as-you-go plan is seeded.** The seed has the four original plans and the unlimited plan. Staff must create a `payg` plan before any account can activate it. This is a seed gap for AC32 and the operator checklist, not a code defect.
4. **Migration 969 is missing from the platform schema check** (section 2.1).
5. **AC48 comparison test is missing** for the new routes with `MULTICA_CLOUD_URL` empty.
6. **Two squad reproduction tests are red on main**: `TestReproSquadRequestMovedToInReviewByMemberGetsReviewerRun` and `TestReproSquadRequestMovedToInReviewByLeaderRunGetsReviewerRun`. They predate R5 and were recorded red in the R3 gate.

### 6.2 Named gaps from the test gaps above

Each row in sections 3 and 4 marked "gap" is a missing test. They are listed there and are not repeated here.

### 6.3 Open items carried from earlier steps (not closed by this gate)

- The removed-member gap recorded in Q9 (spec 15.3 (d)).
- Mobile SSO sign-in is not built.
- Web and desktop follow-ups (as named in the Q17 brief; their scope was not re-verified in this gate).
- The `platform_account_request` teardown gap (as named in the Q17 brief; commit `2bb56a630` records it in the deletion manifest).
- The `sso_domain` cleanup (as named in the Q17 brief).

## 7. Documentation (Q17 step 7)

R5 user pages were written under `apps/docs/content/docs/` (subscriptions, account groups, usage billing). The operator checklist is section 13 of `docs/platform-operations.md`. Both were written from the code, not from the spec's plans; the in-app upcoming-price notice and the removed-member cleanup are left out because they are not built. At review, one claim was corrected: the subscription page said staff can mark a profile verified in the product, which is not built (6.1 item 1); the page now says verification is recorded outside the console. The operations checklist section 13 also notes that migration 969 is applied by `migrate up` but not checked by the platform, and that the `PLATFORM_PRICE_*` settings have no effect. Section 11 of the same file still names migration 690 as the last required one; that stale text was not edited.

## 8. Closing update: partial acceptance criteria closed (2026-11-17)

Each partial criterion in section 3 got its test first, from the criterion's wording, and a fix only where the test showed the behaviour missing. Backend work was done by kiro and frontend by agy, reviewed by hand; each slice was committed with explicit paths and pushed to origin main.

| Slice | Commit | Criteria | What changed |
| --- | --- | --- | --- |
| A | `255bb1192` | AC25, AC26, AC27 | Group `billing.view` and `billing.manage` keys now open the subscription and order routes; viewers still refused on writes. AC26 tests passed on first run. |
| B | `d4c4eaef0` | AC21, AC37, AC38, gate item 3a and 4 | `PLATFORM_PRICE_NOTICE_DAYS`, `PLATFORM_PRICE_CONFIRM_PCT`, `PLATFORM_PRICE_MAX` read from the environment; individual seat cap on checkout and seat purchase; migration 969 in the required list. |
| C | `94a301c1b` | AC23(i), AC33, AC38, AC39 | Renewal clears `pending_change` and stores `unit_price`; daily rating uses the price in force at 00:00 UTC of the day. AC32, AC33 (100 TB), AC38 (`pending_deletion`) tests passed on first run. |
| D | `2440ee68b` | AC35, AC41, AC46, AC47, AC48 | Tests for the write-off rule, admin-created sign-in with signup off, console route isolation, admin-created account lifecycle and the cloud-URL regression. The contract fixture for the paid workspace is now a business account. |
| E | `9db96de12` | AC25 | Console `GET /api/platform/tenants/{id}/billing-profile`; customer `GET /api/cloud-subscriptions/billing-profile` under `billing.view`. |
| F | `ae478aca5` | AC25 (console follow-up) | Tenant detail reads the verification on page load; verify and unverify update that cache. |
| G | `69f398aca` | AC23(ii) | A seat-to-usage plan change is stored as `pending_change` with no charge and applies at the end of the paid period through a `usage_switch` lifecycle edge, with no renewal order. |
| H | `8db23ff2b` | AC48 (client) | Tests that the billing UI and schemas hide features when the optional fields are absent. Passed on first run. |
| payg | `798500681` | AC32, gate item 3b | The `payg` plan is in the seed catalogue with the spec's rates; `GET /prices` no longer lists usage plans. |
| J | `662b69636` | AC25, AC28, AC34 | Tests that the billing profile stays out of the summary and export, that a usage order never expires, and that account deletion removes group rows. Passed on first run. |
| K | `564467dca` | (gate check) | A stale-worker seat test depended on other tests' due rows in the shared database; it now claims until its own row comes back, as in `9b2a6ac3a`. |

### 8.1 Status of each criterion after the closing slices

Covered (by named tests, including the slices above): AC21, AC22, AC23 (both halves), AC24, AC25, AC26, AC27, AC28, AC29, AC30, AC31, AC32, AC33, AC34, AC35, AC36, AC37, AC38, AC39, AC40, AC41, AC42, AC43, AC44, AC45, AC46, AC47, AC48.

Caveats the tests do not remove:
- AC28: group rows on account deletion are removed by the shared `revokeAndRemoveMember` transaction; the deletion job test checks that outcome.
- AC32: "invitations and share links are not blocked by seats on a usage plan" is tested in the platform seat package (`TestDBInvitationsAndShareLinksAreNotBlockedBySeatsOnAUsagePlan`), not through the Multica invitation handler.
- AC23(ii): the switch is stored and applied at period end. Storing a second pending change is not refused by a new rule; the existing flow replaces the pending change unless an order is pending. The spec's AC23 wording does not name that refusal.
- AC39 and AC23(ii): a usage period starts at the instant the paid seat period ends, so the transition UTC day is rated wholly at usage rates. The gate accepts this as under one day of overlap; the owner should confirm it.
- AC25 (summary and export): tested by `TestSummaryNeverCarriesTheBillingProfile` and `TestExportHasNoBillingProfile`; the Multica proxy forwards the platform body unchanged and has no separate test.

### 8.2 Security rows still without a test (section 4)

These are spec 7b security rows, not acceptance criteria, so they do not block closing the criteria. They remain open:
- MFA on the console price routes (7b.4 "admin bị chiếm").
- MFA on the account-creation route (7b.5 "tạo hàng loạt").
- Debt bypass: an owner who abandons a usage debt and opens a new free workspace (7b.3).
- Removal of the profile copy in the account request after completion (7b.5).
- Unused-account indicator in the console (7b.5): not built.
- Welcome email link host (7b.5): only the no-secret check is tested.
- Group audit privacy (7b.2): group names kept out of the audit log is not tested.

### 8.3 Other open items

- The console `PUT /tenants/{id}/billing-profile` in spec 7b.1 is not built (not an acceptance criterion).
- The two squad repro tests stay red, as instructed.
- The items carried from earlier: the removed-member gap in Q9, mobile SSO sign-in, web and desktop follow-ups, the `platform_account_request` teardown gap, and the `sso_domain` cleanup.
- Operations doc section 11 still names migration 690 as the last required one (stale, not edited here).

### 8.4 Final checks (after K)

| Check | Result |
| --- | --- |
| Platform module on `multica_r5_test`: `go vet ./...`; `go test -p 1 -count=1 ./...` | vet exit 0; 35 packages `ok`, exit 0 |
| Server migration checks (four named tests) on `multica_platform_test` | pass, exit 0 |
| Server R5 packages on `multica_platform_test`, `-p 1` (19 packages, excluding the handler full run) | 18 `ok`, exit 0 in the same lane run before K; the handler package is run separately below |
| `server/internal/handler` full package on `multica_platform_test` (after K) | fails only the two squad repro tests; all other tests pass |
| Console `vitest` | pass: 17 files, 242 tests |
| Console `tsc --noEmit` | exit 0 |
| `packages/core` vitest / typecheck | pass: 374 files, 5569 tests, 1 skipped / exit 0 |
| `packages/views` vitest / typecheck | pass: 733 files, 8881 tests / exit 0 |
| `pnpm lint` | exit 0, 7 of 7 tasks (turbo may have served cached results) |

Gate verdict: every acceptance criterion in section 3 is now covered by a named test, and every check above is green except the two squad repro tests. The security rows in 8.2 and the items in 8.3 remain open and are listed with their reasons.
