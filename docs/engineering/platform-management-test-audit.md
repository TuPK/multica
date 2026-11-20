# R6 admin plan: test-name audit against the code

Audit of the test names in `docs/engineering/platform-management-plan-admin.md`, lines "Test đỏ trước" of slices S0 to S38. Each name is classified against the code at commit `5cf735a6b`. No test or implementation was changed; the database was not touched; no test command was run.

## 1. Method and classes

- Names: backticked Go `Test…` names and console/web test titles on the "Test đỏ trước" lines (S0 to S38). The "Lệnh kiểm chứng" lines hold `-run` regex filters, not test names; they are checked in section 5, not as rows. S15 and S38 add no names.
- Go presence: `func Test…(` searched in every `*.go` under `platform-management/` and `server/` (class a = same name).
- UI presence: the title passed to `it(` or `test(` in a `*.test.ts`/`*.test.tsx` file in the repository (class a = same title).
- Names missing by name were searched by keyword in the same package, then the candidate body was read where the name alone did not settle it. The implementation of each c and d item was located by grep; see section 7 for the ones not read in full.
- **a**: present under the same name.
- **b**: present under another name, and one or more existing tests assert every outcome the plan name claims. If any claimed outcome (an ordering, an atomicity claim, a role, a field) is not asserted by those tests, the row is c.
- **c**: the behaviour exists in code, but no test asserts the plan's claim in full.
- **d**: the behaviour is absent from code.

## 2. Counts per class

Rows: 435 (one per slice and name). Go rows: 198 a + 176 non-a (70 b, 102 c, 4 d). UI rows: 61 a.

| Class | Rows | Meaning |
|---|---|---|
| a | 259 | present under the same name |
| b | 70 | present under another name, same behaviour asserted |
| c | 102 | behaviour in code, no test for the plan's claim |
| d | 4 | behaviour missing from code |
| total | 435 | |

Excluded as not test names (SQL or assertion fragments in backticks): `ALTER TABLE` (S1), `INSERT INTO` (S1), `actor_type = staff` (S7), `DELETE FROM` (S9), `error_code = sole_owner_unresolved` (S12).

### Per slice

| Slice | a | b | c | d |
|---|---|---|---|---|
| S0 | 36 | 0 | 0 | 0 |
| S1 | 15 | 0 | 0 | 0 |
| S2 | 8 | 0 | 0 | 0 |
| S3 | 17 | 0 | 0 | 0 |
| S4 | 4 | 0 | 0 | 0 |
| S5 | 21 | 0 | 0 | 0 |
| S6 | 4 | 0 | 0 | 0 |
| S7 | 16 | 0 | 0 | 0 |
| S8 | 19 | 0 | 0 | 0 |
| S9 | 13 | 5 | 1 | 0 |
| S10 | 12 | 1 | 2 | 0 |
| S11 | 1 | 11 | 2 | 0 |
| S12 | 4 | 3 | 4 | 0 |
| S13 | 15 | 0 | 0 | 0 |
| S14 | 6 | 0 | 0 | 0 |
| S16 | 1 | 4 | 9 | 1 |
| S17 | 0 | 5 | 7 | 0 |
| S18 | 1 | 4 | 7 | 2 |
| S19 | 6 | 0 | 4 | 0 |
| S20 | 0 | 7 | 5 | 0 |
| S21 | 0 | 4 | 6 | 0 |
| S22 | 1 | 2 | 6 | 1 |
| S23 | 9 | 0 | 0 | 0 |
| S24 | 7 | 0 | 0 | 0 |
| S25 | 6 | 0 | 0 | 0 |
| S26 | 0 | 3 | 7 | 0 |
| S27 | 6 | 3 | 4 | 0 |
| S28 | 0 | 5 | 6 | 0 |
| S29 | 0 | 2 | 5 | 0 |
| S30 | 6 | 0 | 2 | 0 |
| S31 | 0 | 5 | 9 | 0 |
| S32 | 5 | 0 | 2 | 0 |
| S33 | 0 | 3 | 6 | 0 |
| S34 | 0 | 3 | 6 | 0 |
| S35 | 6 | 0 | 2 | 0 |
| S36 | 10 | 0 | 0 | 0 |
| S37 | 4 | 0 | 0 | 0 |

## 3. Class d: behaviour missing from code

| Slice | Name | Detail |
|---|---|---|
| S16 | `TestLimitLoweringNeedsConfirmWhenSubscribed` | none in platform-management Go (grep "confirm" in internal/subscription and internal/plan: no match) — the API has no confirm gate; the console UI implements it client-side (platform-management/console/src/screens/Plans.tsx:82 findLoweredLimits, :178 confirmLowerLimits; test Plans.test.tsx:380) |
| S18 | `TestAllocationRoutesRefuseAdmin` | no allocation route in platform-management Go (grep 'alloc' in platform-management: no match; the only 'allocation' is UI text in console/src/screens/UserDetail.tsx) — backend allocation route absent |
| S18 | `TestUserDetailAllocationShowsSeatsAndLimits` | no allocation field in platform-management console user detail (grep 'alloc' no match) — seats and limits per user not provided by backend |
| S22 | `TestDeleteRuntimeAndWorkspaceRemovePlatformRuntimeRows` | runtime half: no platform_runtime delete in server/pkg/db/queries (only workspace_delete.sql:784 for workspace); workspace half exists at server/pkg/db/queries/workspace_delete.sql:784 — runtime-delete removal missing; workspace removal has no test |

These four are for the later build session. The S16 item is partly present in the UI (client-side confirm), but the API has no gate for it.

## 4. M31 check (plan writes, system_admin only)

- Spec M31 (`docs/engineering/platform-management-spec-admin.md:596`): create, edit, enable, disable, delete plans (`POST`, `PUT`, `DELETE /plans…`, `/activate`, `/deactivate`); system_admin all, admin / support / billing none.
- `platform-management/internal/staff/access.go` has no row labelled M31 (the constants run M1 to M30, M32 to M52). `access_test.go` has no M31 row either.
- Routes: `platform-management/internal/subscription/console.go:95-113` map every plan write route (`/plans`, `/plans/{key}`, `/plans/{key}/activate`, `/plans/{key}/deactivate`) to `ActionPlansManage`. Reads use `ActionPlansRead` (M30).
- `ActionPlansManage` (M2) is declared at `access.go:18` and its matrix row is `access.go:78`, `{All, None, None, None}`: system_admin only.
- **Result: yes, the same roles.** M2 gives system_admin all, admin / support / billing none, which is the spec M31 row. The write routes are guarded by M2 with no separate M31 row, so M31 has no own test. Spec M2 (line 271) lists the write routes `POST`, `PUT`, `DELETE /plans…`; spec M31 (line 596) adds `/activate` and `/deactivate`, which the code also guards with M2 (`console.go:104-108`). Both spec rows give the same roles.

## 5. Verification filters that match no test

A `go test -run` filter that matches no test name passes without running anything. Checked against every `func Test…` name in `platform-management/` and `server/`:

| Slice | Command (from plan) | Result |
|---|---|---|
| S11 | `./internal/deletionrequest ./internal/console -run 'UserDeleteNow'` (the [PD] line) | matches 0 test names in either tree |
| S27 | `./audit -run 'Readonly'` (in `platform-management/`) | matches 0 test names in either tree; the test is named `TestPlatformAuditQueriesAreInsertAndSelectOnly` |
| S10 | `./internal/lifecycle -run 'NeverDeletes'` (in `platform-management/`) | matches 0 platform test names (two `server/` names match) |

The other filters are alternations of broad terms (for example `Tenant`, `Runtime`, `Plan`) that each match many names; they were not checked term by term. Those broad filters would pass even when the named tests are absent.

## 6. Rows

| Slice | Name | Class | Location |
|---|---|---|---|
| S0 | `TestParseRoleAcceptsFourRoles` | a | platform-management/internal/staff/role_test.go:5 |
| S0 | `TestPermissionMatrixMatchesSpec` | a | platform-management/internal/staff/access_test.go:73 |
| S0 | `TestSystemAdminHasEveryAction` | a | platform-management/internal/staff/access_test.go:102 |
| S0 | `TestAdminHasNoSystemOnlyAction` | a | platform-management/internal/staff/access_test.go:113 |
| S0 | `TestSupportAndBillingAccessUnchangedFromSpec52` | a | platform-management/internal/staff/access_test.go:131 |
| S0 | `TestEveryRouteInEveryRouteTableHasAMatrixRow` | a | platform-management/internal/subscription/console_test.go:1165 |
| S0 | `TestValidateStaffTarget` | a | platform-management/internal/console/staff_manage_test.go:14 |
| S0 | `TestUnknownRoleInDatabaseIsNotFound` | a | platform-management/internal/staff/seed_test.go:309 |
| S0 | `TestIdentityCarriesTenantID` | a | platform-management/internal/platformauth/platformauth_test.go:536 |
| S0 | `TestMeReturnsRoleAndTenantID` | a | platform-management/internal/platformauth/platformauth_test.go:500 |
| S0 | `TestAuditEntryTenantIDReachesTheWriter` | a | platform-management/audit/catalog_test.go:102 |
| S0 | `TestAuditCatalogHasStaffRoleActions` | a | platform-management/audit/catalog_test.go:17 |
| S0 | `TestRoleMatrix` | a | platform-management/internal/console/handler_test.go:115 |
| S0 | `TestBillingCannotWrite` | a | platform-management/internal/console/handler_test.go:143 |
| S0 | `TestSupportCannotReachStaff` | a | platform-management/internal/console/handler_test.go:154 |
| S0 | `TestAddStaffAdminRequiresTenantID` | a | platform-management/internal/console/staff_manage_db_test.go:50 |
| S0 | `TestAddStaffOtherRolesRejectTenantID` | a | platform-management/internal/console/staff_manage_db_test.go:73 |
| S0 | `TestStaffRoutesAreSystemAdminOnly` | a | platform-management/internal/console/staff_manage_db_test.go:91 |
| S0 | `TestPutStaffChangesRoleAndTenantAndAuditsInOneTransaction` | a | platform-management/internal/console/staff_manage_db_test.go:120 |
| S0 | `TestPutStaffAuditFailureRollsBack` | a | platform-management/internal/console/staff_manage_db_test.go:175 |
| S0 | `TestStaffHandlersRefuseNonSystemAdminWhenCalledDirectly` | a | platform-management/internal/console/staff_manage_db_test.go:206 |
| S0 | `TestAdminCannotRaiseOwnOrOthersRole` | a | platform-management/internal/console/staff_manage_db_test.go:246 |
| S0 | `TestRoleChangeTakesEffectOnNextRequest` | a | platform-management/internal/console/staff_manage_db_test.go:266 |
| S0 | `TestLastSystemAdminCannotBeDemotedOrRevokedConcurrently` | a | platform-management/internal/console/staff_manage_db_test.go:289 |
| S0 | `TestSeedMakesEnvEmailsSystemAdmin` | a | platform-management/internal/staff/seed_test.go:131 |
| S0 | `TestSeedReactivatesRevokedEnvEmailAsSystemAdmin` | a | platform-management/internal/staff/seed_test.go:143 |
| S0 | `TestStaffAuditEntriesCarryTenantID` | a | platform-management/internal/console/staff_manage_db_test.go:344 |
| S0 | `TestStaffRoleMigrationConvertsOldAdminsOnlyOnce` | a | server/internal/migrations/staff_role_migration_test.go:104 |
| S0 | `TestStaffRoleMigrationLeavesScopedAdminsAndOtherRoles` | a | server/internal/migrations/staff_role_migration_test.go:135 |
| S0 | `TestStaffRoleMigrationDownRevokesScopedAdminsFirst` | a | server/internal/migrations/staff_role_migration_test.go:157 |
| S0 | `TestStaffTenantColumnIsNullableWithoutForeignKey` | a | server/internal/migrations/staff_role_migration_test.go:181 |
| S0 | `TestAuditEventTenantColumnIsNullableWithoutForeignKey` | a | server/internal/migrations/staff_role_migration_test.go:197 |
| S0 | `TestAuditStoreWritesTenantID` | a | server/internal/auditstore/tenant_test.go:14 |
| S0 | `TestMigrationFilesHaveMatchingDirections` | a | server/internal/migrations/migrations_lint_test.go:93 |
| S0 | `TestMigrationNumericPrefixesAreUnique` | a | server/internal/migrations/migrations_lint_test.go:61 |
| S0 | `TestEveryConcurrentUpBuildHasCleanup` | a | server/cmd/migrate/migrate_mul5999_index_retry_test.go:161 |
| S1 | `TestMigrationFilesHaveMatchingDirections` | a | server/internal/migrations/migrations_lint_test.go:93 |
| S1 | `TestMigrationNumericPrefixesAreUnique` | a | server/internal/migrations/migrations_lint_test.go:61 |
| S1 | `TestEveryConcurrentUpBuildHasCleanup` | a | server/cmd/migrate/migrate_mul5999_index_retry_test.go:161 |
| S1 | `TestFourDigitMigrationsOnlyTouchTablesCreatedEarlierInSortOrder` | a | server/internal/migrations/migrations_order_test.go:61 |
| S1 | `TestTenantBackfillIsIdempotentAndKeysTenantByWorkspace` | a | server/internal/migrations/tenant_migration_test.go:88 |
| S1 | `TestTenantTablesHaveNoForeignKeysOrInlineConstraints` | a | server/internal/migrations/tenant_migration_test.go:164 |
| S1 | `TestWorkspaceTableHasNoTenantColumn` | a | server/internal/migrations/tenant_migration_test.go:204 |
| S1 | `TestPlanMaxWorkspacesMigrationIsIdempotentAndKeepsExistingKey` | a | server/internal/migrations/tenant_migration_test.go:237 |
| S1 | `TestDeleteWorkspaceDetachesTenant` | a | server/internal/handler/workspace_delete_tenant_test.go:44 |
| S1 | `TestDeleteWorkspaceTenantDetachWaitsForTenantLock` | a | server/internal/handler/workspace_delete_tenant_test.go:74 |
| S1 | `TestWorkspaceDeletionManifestCoversPublicSchema` | a | server/internal/handler/workspace_delete_manifest_test.go:265 |
| S1 | `TestWorkspaceTeardownAppliesManifestDecisions` | a | server/internal/handler/workspace_delete_manifest_test.go:351 |
| S1 | `TestRequiredVersionsAreSortedUniqueAndExist` | a | platform-management/internal/schemacheck/schemacheck_test.go:21 |
| S1 | `TestEveryPlatformTableMigrationIsRequired` | a | platform-management/internal/schemacheck/schemacheck_test.go:100 |
| S1 | `TestDeleteWorkspaceLeavesPlatformStaffRowsUntouched` | a | server/internal/handler/workspace_delete_tenant_test.go:117 |
| S2 | `TestCatalogueMaxWorkspacesDefaults` | a | platform-management/internal/plan/max_workspaces_test.go:10 |
| S2 | `TestValidateRejectsMaxWorkspacesBelowOne` | a | platform-management/internal/plan/max_workspaces_test.go:46 |
| S2 | `TestUnlimitedPlanStillLeavesEveryCountedGateEmpty` | a | platform-management/internal/plan/max_workspaces_test.go:78 |
| S2 | `TestTenantMaxWorkspacesIsLargestEffectiveLimit` | a | platform-management/internal/entitlement/max_workspaces_test.go:19 |
| S2 | `TestPolicyCarriesTenantGateForEverySiblingWorkspace` | a | platform-management/internal/entitlement/max_workspaces_db_test.go:49 |
| S2 | `TestNormalizePolicyAcceptsMaxWorkspacesGate` | a | server/internal/entitlement/max_workspaces_test.go:8 |
| S2 | `TestAbsentMaxWorkspacesGateIsOff` | a | server/internal/entitlement/max_workspaces_test.go:30 |
| S2 | `TestUnknownGateIsStillRejected` | a | server/internal/entitlement/max_workspaces_test.go:46 |
| S3 | `TestCreateWithoutTenantMakesTenantWithWorkspaceID` | a | server/internal/workspaceprovision/provision_test.go:280 |
| S3 | `TestCreateWithTenantAddsLinkOnly` | a | server/internal/workspaceprovision/provision_test.go:299 |
| S3 | `TestCreateWorkspaceWithoutTenantIDKeepsResponseAndAddsTenantID` | a | server/internal/handler/workspace_tenant_test.go:83 |
| S3 | `TestCreateWorkspaceIntoTenantAsOwner` | a | server/internal/handler/workspace_tenant_test.go:109 |
| S3 | `TestCreateWorkspaceIntoForeignTenantIs404` | a | server/internal/handler/workspace_tenant_test.go:136 |
| S3 | `TestCreateWorkspaceIntoTenantRefusedAtMaxWorkspaces` | a | server/internal/handler/workspace_tenant_test.go:176 |
| S3 | `TestConcurrentCreateAtLastTenantSlotAdmitsExactlyOne` | a | server/internal/handler/workspace_tenant_test.go:199 |
| S3 | `TestCreateWorkspaceIntoTenantUnlimitedWhenGateOff` | a | server/internal/handler/workspace_tenant_test.go:241 |
| S3 | `TestCreateWorkspaceTakesTenantLockWithTenantScope` | a | server/internal/handler/workspace_tenant_test.go:259 |
| S3 | `TestDowngradeNeverDeletesWorkspaces` | a | server/internal/handler/workspace_tenant_test.go:318 |
| S3 | `TestAdmitScopesLockByTenantID` | a | server/internal/planlimit/tenant_test.go:27 |
| S3 | `TestProvisionedWorkspaceHasTenant` | a | server/internal/accountprovision/job_test.go:878 |
| S3 | `TestCreateWorkspaceUsesRequestedSlug` | a | server/internal/handler/handler_test.go:2117 |
| S3 | `TestCreateWorkspaceSlugConflictReturnsConflict` | a | server/internal/handler/handler_test.go:2138 |
| S3 | `TestCreateWorkspaceInvalidSlugReturnsBadRequest` | a | server/internal/handler/handler_test.go:2159 |
| S3 | `TestNewWorkspaceInTenantIsInAdminScopeWithoutStaffGrant` | a | server/internal/handler/workspace_tenant_test.go:338 |
| S3 | `TestCreateWorkspaceDoesNotGrantStaffAnyTenant` | a | server/internal/handler/workspace_tenant_test.go:359 |
| S4 | `TestBackfillTenantsCreatesOneTenantPerOrphanWorkspace` | a | platform-management/internal/backfill/tenants_db_test.go:99 |
| S4 | `TestBackfillTenantsIsIdempotentAndParallelSafe` | a | platform-management/internal/backfill/tenants_db_test.go:135 |
| S4 | `TestBackfillTenantsBatchesAreIndependent` | a | platform-management/internal/backfill/tenants_db_test.go:184 |
| S4 | `TestPlatformBackfillTenantsSubcommandParsesArguments` | a | platform-management/cmd/platform-backfill/main_test.go:67 |
| S5 | `TestRoleMatrix` | a | platform-management/internal/console/handler_test.go:115 |
| S5 | `TestOldTenantSubroutesAreNotFound` | a | platform-management/internal/console/tenants_test.go:15 |
| S5 | `TestTenantListJSONHasWorkspaceCountAndMaxWorkspaces` | a | platform-management/internal/console/tenants_test.go:36 |
| S5 | `TestListTenantsSearchesTenantNameWorkspaceSlugAndOwnerEmail` | a | platform-management/internal/console/tenants_db_test.go:63 |
| S5 | `TestGetTenantListsWorkspacesWithEffectiveMaxWorkspaces` | a | platform-management/internal/console/tenants_db_test.go:104 |
| S5 | `TestTenantStatusSummaryCountsWorkspaceStatuses` | a | platform-management/internal/console/tenants_db_test.go:153 |
| S5 | `TestLockingOneWorkspaceLeavesSiblingsUntouched` | a | platform-management/internal/console/tenants_db_test.go:180 |
| S5 | `TestOverviewCountsWorkspacesWithoutTenant` | a | platform-management/internal/console/tenants_db_test.go:206 |
| S5 | `TestConsoleFilesAreScannedAndNameNoContentTable` | a | platform-management/internal/console/isolation_test.go:162 |
| S5 | `TestAdminSeesOnlyOwnTenantInList` | a | platform-management/internal/console/scope_db_test.go:61 |
| S5 | `TestAdminGetsSameNotFoundForForeignAndMissingObjects` | a | platform-management/internal/console/scope_db_test.go:82 |
| S5 | `TestAdminScopeFollowsTenantWorkspaceLinks` | a | platform-management/internal/console/scope_db_test.go:119 |
| S5 | `TestAdminOfDeletedTenantHasEmptyScope` | a | platform-management/internal/console/scope_db_test.go:140 |
| S5 | `TestAdminLockUnlockAndScheduleDeletionInOwnTenantOnly` | a | platform-management/internal/console/scope_db_test.go:159 |
| S5 | `TestAdminOrdersAreReadOnlyAndScoped` | a | platform-management/internal/console/scope_db_test.go:196 |
| S5 | `TestAdminHasNoAccessToPaymentRoutesAccountsOverviewAndUserActions` | a | platform-management/internal/console/scope_db_test.go:223 |
| S5 | `TestUsersListScopedToTenantMembersForAdmin` | a | platform-management/internal/console/scope_db_test.go:244 |
| S5 | `TestAuditListScopedForAdminAndFullForOthers` | a | platform-management/internal/console/scope_db_test.go:262 |
| S5 | `TestStaffTenantMustExist` | a | platform-management/internal/console/scope_db_test.go:297 |
| S5 | `TestHandlersApplyScopeWhenCalledDirectlyWithoutMiddleware` | a | platform-management/internal/console/scope_db_test.go:317 |
| S5 | `TestSupportBillingScopeUnchanged` | a | platform-management/internal/console/scope_db_test.go:360 |
| S6 | `renders one row per tenant with workspace count and max_workspaces` | a | platform-management/console/src/screens/Tenants.test.tsx (test title) |
| S6 | `shows unlimited when max_workspaces is null` | a | platform-management/console/src/screens/Tenants.test.tsx (test title) |
| S6 | `lists workspaces and links to the workspace detail` | a | platform-management/console/src/screens/TenantDetail.test.tsx (test title) |
| S6 | `tenant schema falls back for unknown fields and null max_workspaces` | a | platform-management/console/src/api/schemas.test.ts (test title) |
| S7 | `TestMemberRequestValidate` | a | platform-management/internal/memberrequest/validate_test.go:9 |
| S7 | `TestRoleMatrix` | a | platform-management/internal/console/handler_test.go:115 |
| S7 | `TestAuditCatalogHasMemberStaffActions` | a | platform-management/audit/catalog_test.go:36 |
| S7 | `TestMemberRequestStaffEmailIsRefused` | a | platform-management/internal/memberrequest/db_test.go:171 |
| S7 | `TestMemberRequestWorkspaceMustBeActive` | a | platform-management/internal/memberrequest/db_test.go:202 |
| S7 | `TestMemberRequestAlreadyMember` | a | platform-management/internal/memberrequest/db_test.go:235 |
| S7 | `TestMemberRequestResendReturnsLiveRequest` | a | platform-management/internal/memberrequest/db_test.go:247 |
| S7 | `TestMemberRequestConcurrentSameEmailWritesOneRow` | a | platform-management/internal/memberrequest/db_test.go:278 |
| S7 | `TestMemberRequestAuditAndRowShareOneTransaction` | a | platform-management/internal/memberrequest/db_test.go:335 |
| S7 | `TestMemberRequestCancelReleasesSeatAndAudits` | a | platform-management/internal/memberrequest/db_test.go:362 |
| S7 | `TestMemberRequestRoutesPassConsoleIsolationScan` | a | platform-management/internal/console/members_test.go:257 |
| S7 | `TestMemberRequestAdminInOwnTenantSucceedsWithStaffAuditAndTenantID` | a | platform-management/internal/console/members_db_test.go:49 |
| S7 | `TestMemberRequestAdminOutsideTenantIsNotFoundAndWritesNothing` | a | platform-management/internal/console/members_db_test.go:78 |
| S7 | `TestMemberRequestSystemAdminAnyWorkspace` | a | platform-management/internal/console/members_db_test.go:109 |
| S7 | `TestMemberRequestAdminListIsFilteredToOwnTenant` | a | platform-management/internal/console/members_db_test.go:122 |
| S7 | `TestMemberRequestRoutesMatchMatrixRows` | a | platform-management/internal/console/members_db_test.go:140 |
| S8 | `TestMemberProvisioningUsesExistingUser` | a | server/internal/memberprovision/job_test.go:345 |
| S8 | `TestMemberProvisioningCreatesOnboardedUserWithoutSessionPasswordOrToken` | a | server/internal/memberprovision/job_test.go:366 |
| S8 | `TestMemberProvisioningWorksWhenSignupIsDisabled` | a | server/internal/memberprovision/job_test.go:391 |
| S8 | `TestMemberProvisioningBlockedDomainFails` | a | server/internal/memberprovision/job_test.go:403 |
| S8 | `TestMemberProvisioningSuspendedOrPendingDeletionUserFails` | a | server/internal/memberprovision/job_test.go:423 |
| S8 | `TestMemberProvisioningRolesApplied` | a | server/internal/memberprovision/job_test.go:475 |
| S8 | `TestMemberProvisioningSeatFullReleasesAndCreatesNoMember` | a | server/internal/memberprovision/job_test.go:502 |
| S8 | `TestMemberProvisioningConfirmsSeatAfterCommit` | a | server/internal/memberprovision/job_test.go:532 |
| S8 | `TestMemberProvisioningUnmanagedWorkspaceSkipsSeat` | a | server/internal/memberprovision/job_test.go:556 |
| S8 | `TestMemberProvisioningTransportFailureRetriesThenFailsAndReleases` | a | server/internal/memberprovision/job_test.go:578 |
| S8 | `TestMemberProvisioningTwoRequestsForLastSeatOneWins` | a | server/internal/memberprovision/job_test.go:610 |
| S8 | `TestMemberProvisioningAuditActorIsStaffWithWorkspaceID` | a | server/internal/memberprovision/job_test.go:632 |
| S8 | `TestMemberProvisioningAuditFailureRollsBackMemberAndReleasesSeat` | a | server/internal/memberprovision/job_test.go:672 |
| S8 | `TestMemberProvisioningPublishesMemberAdded` | a | server/internal/memberprovision/job_test.go:695 |
| S8 | `TestMemberProvisioningTwiceAndParallelMakesOneMember` | a | server/internal/memberprovision/job_test.go:733 |
| S8 | `TestWorkspaceDeleteRemovesMemberRequests` | a | server/internal/handler/workspace_delete_member_request_test.go:13 |
| S8 | `TestAccountDeletionAnonymizesMemberRequests` | a | server/internal/handler/me_deletion_test.go:543 |
| S8 | `TestUserColumnRegistry` | a | server/internal/migrations/user_columns_registry_test.go:38 |
| S8 | `TestStaffAddClaimIsIdempotentByRequestToken` | a | platform-management/internal/seat/staff_add_db_test.go:10 |
| S9 | `TestRoleMatrix` | a | platform-management/internal/console/handler_test.go:115 |
| S9 | `TestConfirmMatching` | a | platform-management/internal/deletionrequest/validate_test.go:9 |
| S9 | `TestDeleteNowReasonRequired` | a | platform-management/internal/deletionrequest/validate_test.go:33 |
| S9 | `TestDeleteNowConfirmMismatchWritesNothing` | a | platform-management/internal/deletionrequest/db_test.go:144 |
| S9 | `TestDeleteNowPhaseOneWritesAuditRequestAndStatusInOneTransaction` | a | platform-management/internal/deletionrequest/db_test.go:159 |
| S9 | `TestDeleteNowSetsPendingDeletionDueNowAndClearsCache` | a | platform-management/internal/deletionrequest/db_test.go:184 |
| S9 | `TestDeleteNowExpeditesAlreadyScheduledDeletion` | a | platform-management/internal/deletionrequest/db_test.go:199 |
| S9 | `TestDeleteNowTenantListsEveryWorkspaceAndAuditsEach` | a | platform-management/internal/deletionrequest/db_test.go:231 |
| S9 | `TestDeleteNowSecondRequestReturnsLiveOne` | a | platform-management/internal/deletionrequest/db_test.go:212 |
| S9 | `TestCancelRestoresActiveAndAudits` | a | platform-management/internal/deletionrequest/db_test.go:264 |
| S9 | `TestRetryResetsAttempts` | a | platform-management/internal/deletionrequest/db_test.go:289 |
| S9 | `TestNoDeleteStatementsOnWorkspaceContentInPlatformCode` | a | platform-management/internal/deletionrequest/no_delete_test.go:13 |
| S9 | `TestDeleteWorkspaceNowAdminInOwnTenant` | b | platform-management/internal/console/deletions_test.go:187 TestDeleteNowAdminInOwnTenantIsAccepted — 202 + service receives tadmin, confirm |
| S9 | `TestDeleteWorkspaceNowAdminOutsideTenantIsNotFound` | b | platform-management/internal/console/deletions_test.go:200 TestDeleteNowAdminOutsideTenantIsNotFound — 404 not_found, service not reached |
| S9 | `TestDeleteTenantNowIsSystemAdminOnly` | b | platform-management/internal/console/deletions_test.go:208 TestDeleteNowTenantIsSystemAdminOnly — admin/support/billing 404; system_admin 202 |
| S9 | `TestSystemAdminDeletesAnyWorkspaceAndTenant` | c | (code: platform-management/internal/console/deletions.go:100 deleteWorkspaceNow, scopeFor All) — system_admin workspace delete outside tenant not asserted; only wsID via TestRoleMatrix and tenant via TestDeleteNowTenantIsSystemAdminOnly |
| S9 | `TestDeletionRequestVisibilityForAdmin` | a | platform-management/internal/console/deletions_test.go:240 |
| S9 | `TestDeletionRequestCancelAndRetryOnlyOwnForAdmin` | b | platform-management/internal/console/deletions_test.go:262 TestDeletionCancelAndRetryOnlyOwnForAdmin — read: admin 404 on foreign and system-made rows; own cancel 200 |
| S9 | `TestDeleteNowRoutesMatchMatrixRows` | b | platform-management/internal/console/handler_test.go:115 TestRoleMatrix (rows delete-now workspace and tenant); validate_test.go:372 TestEveryRouteInEveryRouteTableHasAMatrixRow — account delete-now routes not in TestRoleMatrix; covered by deletions_test.go:276 TestDeleteAccountNowIsSystemAdminOnly |
| S10 | `TestStaffDeletionDeletesWorkspaceThroughExistingTeardown` | a | server/internal/handler/staff_deletion_test.go:76 |
| S10 | `TestStaffDeletionEquivalentToDeleteWorkspace` | a | server/internal/handler/staff_deletion_test.go:174 |
| S10 | `TestStaffDeletionFailureMarksFailedAfterThreeAttemptsAndKeepsPhaseOneAudit` | a | server/internal/staffdeletion/job_test.go:241 |
| S10 | `TestStaffDeletionRetryAfterFailureCompletes` | a | server/internal/staffdeletion/job_test.go:206 |
| S10 | `TestStaffDeletionTenantDeletesEveryWorkspaceThenTenant` | a | server/internal/staffdeletion/job_test.go:270 |
| S10 | `TestStaffDeletionTwiceAndParallelDeletesOnce` | a | server/internal/staffdeletion/job_test.go:176 |
| S10 | `TestStaffDeletionAuditActorIsStaffWithOriginDetail` | a | server/internal/handler/staff_deletion_test.go:132 |
| S10 | `TestStaffDeletionAuditRowSurvivesWithNullWorkspaceID` | b | server/internal/handler/staff_deletion_test.go:153 TestStaffDeletionAuditRowKeepsTenantIDAfterWorkspaceIsGone — asserts workspace.deleted row with workspace_id IS NULL and tenant_id |
| S10 | `TestWorkspaceDeletionJobSkipsWorkspacesWithLiveOrFailedStaffRequest` | a | server/internal/staffdeletion/job_test.go:308 |
| S10 | `TestCreateWorkspaceIntoDeletingTenantIs409` | a | server/internal/handler/staff_deletion_test.go:204 |
| S10 | `TestCreateRacingPhaseOneIsEitherListedOrRefused` | c | server/internal/handler/workspace.go:494-501 (tenant_deleting refusal); test server/internal/handler/staff_deletion_test.go:204 TestCreateWorkspaceIntoDeletingTenantIs409 — refusal covered; racing with phase one not tested |
| S10 | `TestSubscriptionLifecycleNeverDeletesAWorkspace` | c | platform-management/internal/lifecycle/lifecycle.go (no workspace delete call) — no NeverDeletes test in lifecycle package |
| S10 | `TestDeleteWorkspaceWithZeroDaysDeletesAtOnce` | a | server/internal/handler/workspace_deletion_test.go:175 |
| S10 | `TestDeleteWorkspaceWithDaysSchedulesInsteadOfDeleting` | a | server/internal/handler/workspace_deletion_test.go:190 |
| S10 | `TestStaffDeletionAuditRowKeepsTenantIDAfterWorkspaceIsGone` | a | server/internal/handler/staff_deletion_test.go:153 |
| S11 | `TestUserDeleteNowRequiresSystemAdmin` | b | platform-management/internal/console/deletions_test.go:276 TestDeleteAccountNowIsSystemAdminOnly — admin/support/billing 404, service not reached |
| S11 | `TestResolutionValidation` | a | platform-management/internal/deletionrequest/account_test.go:46 |
| S11 | `TestUserDeleteNowSoleOwnerUnresolvedReturns409WithListAndWritesNothing` | b | platform-management/internal/deletionrequest/account_db_test.go:72 TestAccountSoleOwnerUnresolvedWritesNothing + platform-management/internal/console/deletions_test.go:303 TestDeleteAccountNowSoleOwnerUnresolvedIs409WithList — split: service 409 and no rows (db), list of workspace_ids (console) |
| S11 | `TestUserDeleteNowCoOwnerNeedsNoResolution` | b | platform-management/internal/deletionrequest/account_db_test.go:93 TestAccountCoOwnerNeedsNoChoice |
| S11 | `TestUserDeleteNowRefusesStaffAndSelf` | b | platform-management/internal/deletionrequest/account_db_test.go:110 TestAccountRefusesStaffAndSelf |
| S11 | `TestUserDeleteNowPhaseOneBlocksAccountAndInvalidatesGuardCache` | b | platform-management/internal/deletionrequest/account_db_test.go:126 TestAccountPhaseOneWritesRequestChildAndStatus — asserts pending_deletion row, cache and publish |
| S11 | `TestUserDeleteNowCreatesChildRequestsForDeleteChoices` | b | platform-management/internal/deletionrequest/account_db_test.go:126 TestAccountPhaseOneWritesRequestChildAndStatus — child workspace request count |
| S11 | `TestUserDeleteNowPreviewWritesNothing` | b | platform-management/internal/deletionrequest/account_db_test.go:176 TestAccountPreviewWritesNothing |
| S11 | `TestUserDeleteNowAuditIsFirstAndHasNoEmailInDetail` | c | platform-management/internal/deletionrequest/account_db_test.go:126 TestAccountPhaseOneWritesRequestChildAndStatus — asserts audit row and no email in detail; audit ordering before other writes not asserted |
| S11 | `TestUserDeleteNowExpeditesSelfServiceSchedule` | c | platform-management/internal/deletionrequest/account.go:314 and pgx.go:360 ScheduleAccount — no test of an account already scheduled by self-service becoming due now |
| S11 | `TestUserDeleteNowIsSystemAdminOnly` | b | platform-management/internal/console/deletions_test.go:276 TestDeleteAccountNowIsSystemAdminOnly — duplicate of TestUserDeleteNowRequiresSystemAdmin |
| S11 | `TestUserDeleteNowSystemAdminAnyUser` | b | platform-management/internal/console/deletions_test.go:287 TestDeleteAccountNowPassesResolutionsAndAnswers202 — system_admin accepted on one user; not across several users |
| S11 | `TestUserDeleteNowRefusesAnyStaffRoleAsTarget` | b | platform-management/internal/deletionrequest/account_db_test.go:110 TestAccountRefusesStaffAndSelf — tests role admin only |
| S11 | `TestUserDeleteNowRoutesMatchMatrixRows` | b | platform-management/internal/console/deletions_test.go:276 TestDeleteAccountNowIsSystemAdminOnly; validate_test.go:372 TestEveryRouteInEveryRouteTableHasAMatrixRow |
| S12 | `TestStaffAccountDeletionTransfersOwnershipThenDeletes` | a | server/internal/staffdeletion/account_test.go:84 |
| S12 | `TestStaffAccountDeletionDeletesWorkspaceThenAccount` | a | server/internal/staffdeletion/account_test.go:120 |
| S12 | `TestStaffAccountDeletionCoOwnerCase` | b | server/internal/staffdeletion/account_test.go:230 TestStaffAccountDeletionCoOwnerNeedsNoResolution |
| S12 | `TestStaffAccountDeletionWaitsForChildRequestsAndFailsWhenChildFailed` | b | server/internal/staffdeletion/account_test.go:160 TestStaffAccountDeletionWaitsForChildRequests + :182 TestStaffAccountDeletionFailsWhenChildFailed — split across two tests |
| S12 | `TestStaffAccountDeletionSuccessorGoneBetweenPhasesFailsAndKeepsAccount` | b | server/internal/staffdeletion/account_test.go:206 TestStaffAccountDeletionSuccessorGoneFailsAndKeepsAccount |
| S12 | `TestNoWorkspaceIsOrphanedAfterAnyResolutionCombination` | c | server/internal/handler/me_deletion_test.go:683 TestAccountDeletionJobKeepsOtherMembersAndRefusesToOrphanAWorkspace (self-service, one combination); code deletionrequest/account.go checkResolutions — no combinatorial test on the staff path |
| S12 | `TestStaffAccountDeletionPipelineMatchesSelfServiceRows` | c | server/internal/staffdeletion/account.go — no test comparing staff account pipeline to self-service |
| S12 | `TestStaffAccountDeletionAuditActorsAreStaffAndAnonymized` | c | server/internal/staffdeletion/account.go:100-102, 155-157 (ActorStaff, deletedUserLabel) — no account audit-actor assertion in account_test.go |
| S12 | `TestStaffAccountDeletionClearsTargetLabelEmail` | c | server/internal/staffdeletion/account.go:106 (TargetLabel deletedUserLabel) — no assertion on the cleared label/email |
| S12 | `TestAccountDeletionJobSkipsAccountsWithStaffRequest` | a | server/internal/staffdeletion/account_test.go:266 |
| S12 | `TestUserColumnRegistry` | a | server/internal/migrations/user_columns_registry_test.go:38 |
| S13 | `keeps submit disabled until reason is non-empty and typed text matches` | a | platform-management/console/src/components/TypeToConfirmDialog.test.tsx (test title) |
| S13 | `add member form offers the member roles admin, member and viewer but not owner` | a | platform-management/console/src/screens/WorkspaceDetail.test.tsx (test title) |
| S13 | `shows request state and error_code` | a | platform-management/console/src/screens/WorkspaceDetail.test.tsx (test title) |
| S13 | `lists blocking workspaces and requires a choice for each before enabling submit` | a | platform-management/console/src/screens/UserAccountMenu.test.tsx (test title) |
| S13 | `successor list excludes viewers` | a | platform-management/console/src/screens/UserAccountMenu.test.tsx (test title) |
| S13 | `retry and cancel call the right routes` | a | platform-management/console/src/screens/DeletionRequests.test.tsx (test title) |
| S13 | `unknown request state and error_code fall back` | a | platform-management/console/src/api/schemas.test.ts (test title) |
| S13 | `system-admin-only nav items are hidden for admin, support and billing` | a | platform-management/console/src/App.test.tsx (test title) |
| S13 | `navigation items follow the role matrix for system_admin, admin, support and billing` | a | platform-management/console/src/App.test.tsx (test title) |
| S13 | `admin role requires a tenant and other roles hide the tenant field` | a | platform-management/console/src/screens/Staff.test.tsx (test title) |
| S13 | `shows the 400 tenant_required message` | a | platform-management/console/src/screens/Staff.test.tsx (test title) |
| S13 | `admin sees only the tenant returned by the API and no delete-tenant action` | a | platform-management/console/src/screens/Tenants.test.tsx (test title) |
| S13 | `admin sees add member and delete workspace for a workspace in scope` | a | platform-management/console/src/screens/WorkspaceDetail.test.tsx (test title) |
| S13 | `delete account action is shown for system_admin only` | a | platform-management/console/src/screens/UserAccountMenu.test.tsx (test title) |
| S13 | `me response accepts a missing or null tenant_id and an unknown role falls back` | a | platform-management/console/src/api/schemas.test.ts (test title) |
| S14 | `useCreateWorkspace sends tenant_id only when given` | a | packages/core/workspace/mutations.test.tsx (test title) |
| S14 | `workspace schema accepts a missing tenant_id and ignores a malformed one` | a | packages/core/api/schemas.test.ts (test title) |
| S14 | `shows the same-tenant switch only when the existing workspace has a tenant_id` | a | packages/views/onboarding/steps/step-workspace.test.tsx (test title) |
| S14 | `sends tenant_id when the switch is on` | a | packages/views/onboarding/steps/step-workspace.test.tsx (test title) |
| S14 | `maps plan_limit_reached for max_workspaces to the limit toast` | a | packages/views/onboarding/steps/step-workspace.test.tsx (test title) |
| S14 | `404 tenant_not_found shows the generic create-failed toast` | a | packages/views/onboarding/steps/step-workspace.test.tsx (test title) |
| S16 | `TestPlanRoutesMatchMatrixRows` | b | platform-management/internal/subscription/console_test.go:1165 TestEveryRouteInEveryRouteTableHasAMatrixRow; :1184 TestRouteRolesFollowTheMatrix — generic route-matrix tests in subscription package; plan routes M2/M30 checked through them (basis: name + route table, body of :1184 not read) |
| S16 | `TestPlanKeyIsImmutable` | c | platform-management/internal/subscription/console.go:493-535 (key taken from path in putPlan) — no test that a PUT cannot change the key |
| S16 | `TestPlanDescriptionMaxLength` | a | platform-management/internal/plan/description_test.go:10 |
| S16 | `TestLimitLoweringNeedsConfirmWhenSubscribed` | d | none in platform-management Go (grep "confirm" in internal/subscription and internal/plan: no match) — the API has no confirm gate; the console UI implements it client-side (platform-management/console/src/screens/Plans.tsx:82 findLoweredLimits, :178 confirmLowerLimits; test Plans.test.tsx:380) |
| S16 | `TestPutPlanArchivedIsAliasOfDeactivate` | c | platform-management/internal/subscription/console_test.go:564 TestArchivingAPlanKeepsItsTimestampAndRestoringClearsIt (PUT archived=true); deactivate route subscription/plan_lifecycle.go:38 — no assertion that PUT archived and POST deactivate produce the same state |
| S16 | `TestDeactivateAndActivateAreIdempotent` | c | platform-management/internal/subscription/plan_lifecycle.go:34-44 (activatePlan, deactivatePlan, setPlanArchived); console.go:104-108 routes — no idempotence test for the activate/deactivate routes |
| S16 | `TestInactivePlanIsHiddenFromWorkspacePlansAndRefusedAtCheckout` | c | platform-management/internal/billingapi/plan_inactive_test.go:12 TestDeactivatedPlanIsPlanInactive (checkout); subscription/change_test.go:375 TestUnknownAndArchivedPlans (plan_archived refusal) — checkout refusal covered; hidden from workspace plan list not asserted |
| S16 | `TestInactivePlanKeepsExistingSubscriptionsRenewingAndBuyingSeats` | c | subscription/change_test.go:375 covers only new assignment refusal — renewal and seat purchase on an inactive plan not tested |
| S16 | `TestPlanDeleteRefusedWithSubscriptionOrOrder` | c | platform-management/internal/subscription/console_test.go:617 TestDeleteOfAPlanWithSubscriptionsIs409PlanInUse (subscriptions); fake counts orders at console_test.go:127 — order case never set in any test |
| S16 | `TestPlanDeleteAllowedWhenUnused` | b | platform-management/internal/subscription/console_test.go:652 TestDeleteOfAnUnusedPlan |
| S16 | `TestFreePlanCannotBeDeleted` | b | platform-management/internal/subscription/console_test.go:644 TestTheFreePlanCannotBeDeleted |
| S16 | `TestPlanHistoryListsWhoWhenOldNew` | b | platform-management/internal/subscription/plan_lifecycle_test.go:12 TestPlanHistoryListsWhoWhenFieldsOldAndNew |
| S16 | `TestPlanHistoryIncludesPriceChanges` | c | platform-management/internal/subscription/plan_lifecycle.go:141 planHistory — no test that price-change rows appear in plan history |
| S16 | `TestPriceChangeDoesNotTouchPaidPeriodsOrCreatedOrders` | c | platform-management/internal/pricechange (service.go, scheduled apply); usage/job_test.go:305 TestDBThePriceSnapshotStaysWhenThePlanPriceChangesLater (partial) — no test that paid periods and existing orders keep their price |
| S16 | `TestPlanDescriptionMigrationDefaultsToEmpty` | c | server/migrations/938_plan_description.up.sql (DEFAULT '') — no test reads the default |
| S17 | `TestWorkspaceListRoutesMatchMatrixRows` | c | platform-management/internal/console/workspaces.go (list route); read.go nextCursor — only admin list test; support/billing list roles not asserted |
| S17 | `TestStaffExportRoutesMatchMatrixRows` | b | platform-management/internal/console/workspaces_list_test.go:48 TestWorkspaceExportIsQueuedOnceAndRefusedWhenRunningOrDeleting — support and billing 404, own-tenant admin 202; system_admin not asserted in this test |
| S17 | `TestQuickActionStateConflictReturns409WithAllowed` | c | platform-management/internal/console/workspaces.go:88-93 (409 workspace_state_conflict with allowed list) — test asserts the 409 code only, not the allowed field |
| S17 | `TestWorkspaceListFiltersAndPaginationAreStable` | c | platform-management/internal/console/read.go:30-38 nextCursor; workspace list handler — test covers the status filter only (workspaces_list_test.go:13); cursor stability not tested |
| S17 | `TestWorkspaceListSearchMatchesNameSlugOwnerEmailAndTenant` | c | workspace list handler and reader in platform-management/internal/console (SearchWorkspaces path) — no workspace-list search test; tenant search test tenants_db_test.go:63 is a different endpoint |
| S17 | `TestWorkspaceListScopeForAdmin` | b | platform-management/internal/console/workspaces_list_test.go:13 TestWorkspaceListIsScopedForAdminAndFiltered; :36 TestWorkspaceListOtherTenantFilterIsEmptyForAdmin |
| S17 | `TestWorkspaceDetailHasSeatsUsageAndStatusHistory` | c | platform-management/internal/console/handler.go workspace detail; usage_test.go:12; handler_test.go:578 — no test for seats or status history on the detail |
| S17 | `TestWorkspaceDetailSeatsUsedEqualsMemberRows` | c | platform-management/internal/console (workspace detail, MemberCount) — no test ties seats used to member rows |
| S17 | `TestWorkspaceDetailOutsideTenantIsNotFoundForAdmin` | b | platform-management/internal/console/scope_db_test.go:82 TestAdminGetsSameNotFoundForForeignAndMissingObjects; tenants_test.go:159 TestAdminOutOfScopeObjectsAnswerTheSame404AsMissingOnes — covers workspace detail among other objects |
| S17 | `TestQuickActionAuditRowsCarryWorkspaceAndTenantID` | c | platform-management/internal/console/write.go:31 setWorkspaceStatus (audit entry) — no test that the quick-action audit row carries tenant_id |
| S17 | `TestStaffExportInsertsOneQueuedRowWithStaffRequester` | b | platform-management/internal/console/workspaces_list_test.go:48 TestWorkspaceExportIsQueuedOnceAndRefusedWhenRunningOrDeleting — one row, requester tadmin, one audit row |
| S17 | `TestStaffExportRefusedWhileRunning` | b | platform-management/internal/console/workspaces_list_test.go:48 TestWorkspaceExportIsQueuedOnceAndRefusedWhenRunningOrDeleting — export_in_progress 409 asserted |
| S18 | `TestTenantUsersRoutesMatchMatrixRows` | c | platform-management/internal/console/tenant_users_test.go:71 (admin/system list only); handler routes in console/handler.go — support and billing role per route not asserted |
| S18 | `TestRemoveAndRoleRoutesMatchMatrixRows` | c | platform-management/internal/console/tenant_users_test.go:86 TestRemoveRefusesOwnerAndStaffTarget; :96 TestChangeRoleWritesRequestAndAudit — support/billing and per-route role rows not asserted together |
| S18 | `TestChangeRoleRequestValidation` | b | platform-management/internal/console/tenant_users_test.go:40 TestRoleChangeRules (checkRoleChange: owner, own role, unknown role 400) — validates the rules, not the HTTP body parse |
| S18 | `TestAllocationRoutesRefuseAdmin` | d | no allocation route in platform-management Go (grep 'alloc' in platform-management: no match; the only 'allocation' is UI text in console/src/screens/UserDetail.tsx) — backend allocation route absent |
| S18 | `TestTenantUsersListDefinition` | c | platform-management/internal/console tenant users list handler (tenant_users read path) — no test for the list definition |
| S18 | `TestTenantUsersListFieldsAndFilters` | c | platform-management/internal/console tenant users list handler — fields and filters not asserted by any test (tenant_users_test.go:71 checks count only) |
| S18 | `TestTenantUsersHidesStaffFromAdmin` | a | platform-management/internal/console/tenant_users_test.go:71 |
| S18 | `TestTenantUsersScopeForAdmin` | b | platform-management/internal/console/tenant_users_test.go:71 TestTenantUsersHidesStaffFromAdmin; :125 TestAdminDeactivatesMemberOfOwnTenantOnly (other tenant 404) |
| S18 | `TestUserDetailAllocationShowsSeatsAndLimits` | d | no allocation field in platform-management console user detail (grep 'alloc' no match) — seats and limits per user not provided by backend |
| S18 | `TestDeactivateInTenantCreatesRemoveRequestPerWorkspace` | c | platform-management/internal/console/tenant_users_test.go:125 TestAdminDeactivatesMemberOfOwnTenantOnly (one workspace in fixture) — the per-workspace fan-out with several workspaces is not asserted |
| S18 | `TestGlobalLockStillSystemAdminAndSupportOnly` | b | platform-management/internal/console/handler_test.go:115 TestRoleMatrix (POST /users/{id}/status: system_admin and support only) |
| S18 | `TestRemoveRequestRefusesStaffUser` | b | platform-management/internal/console/tenant_users_test.go:86 TestRemoveRefusesOwnerAndStaffTarget (409 user_is_staff) |
| S18 | `TestMemberRequestKindsShareLiveUniqueIndex` | c | server/migrations/1024_platform_member_request_live_uidx.up.sql (partial unique index on (workspace_id, kind, email) WHERE pending) — no test asserts the index behaviour across kinds |
| S18 | `TestRemoveAndRoleAuditCarryTenantWorkspaceAndUser` | c | platform-management/internal/console/tenant_users_test.go:96 TestChangeRoleWritesRequestAndAudit (tenant id only) — workspace and user on the remove/role audit row not asserted |
| S19 | `TestMemberRemoveUsesRevokeAndRemoveMember` | a | server/internal/memberprovision/staffchange_test.go:102 |
| S19 | `TestMemberRemoveReleasesSeat` | c | server/internal/handler/member_staff.go:91 settleMemberCapacityRelease (called from RemoveMemberForStaff) — no test that a staff remove decrements the seat count |
| S19 | `TestMemberRemoveRefusesSoleOwner` | a | server/internal/memberprovision/staffchange_test.go:122 |
| S19 | `TestMemberRemoveTwiceAndParallelRemovesOnce` | a | server/internal/memberprovision/staffchange_test.go:134 |
| S19 | `TestMemberRemoveAuditActorStaffWithTenantWorkspaceUser` | c | server/internal/memberprovision/staffchange_test.go:158 TestMemberRemoveAuditCarriesTenantAndStaffActor (tenant and actor asserted; workspace not) — workspace_id not asserted |
| S19 | `TestMemberRemovePublishesEventsAndInvalidatesCache` | c | server/internal/handler/member_staff.go:57-60 (MembershipCache.Invalidate, publish EventMemberRemoved) — no staff-path test; daemon_test.go:4131 covers DeleteMember only |
| S19 | `TestMemberChangeRoleUpdatesRoleAndAudits` | a | server/internal/memberprovision/staffchange_test.go:180 |
| S19 | `TestMemberChangeRoleRefusesOwnerOnEitherSide` | a | server/internal/memberprovision/staffchange_test.go:196 |
| S19 | `TestMemberChangeRoleStaleRoleFails` | a | server/internal/memberprovision/staffchange_test.go:211 |
| S19 | `TestAddRemoveAddEndsWithOneMember` | c | server/internal/memberprovision/job.go (provision) and member_staff.go (remove) — no add-remove-add sequence test |
| S20 | `TestResourceRoutesMatchMatrixRows` | c | platform-management/internal/console/handler.go:206-207 (tenants/{id}/resources, resources) — route roles for support and billing rows not asserted; scope and system-only tests exist separately |
| S20 | `TestResourceStatusThresholds` | b | platform-management/internal/console/resources_test.go:55 TestTenantResourcesAddUpAndStateEachLimit (90 percent warning, over, ok) |
| S20 | `TestLimitOverrideValidation` | b | platform-management/internal/subscription/limits_test.go:77 TestLimitOverrideValidationAndRelease |
| S20 | `TestTenantResourcesMatchSourceTablesOnFixture` | c | platform-management/internal/console/resources.go (tenant resource read); resources_test.go:55 uses a fake fixture — no test that the figures match the source tables on a database fixture |
| S20 | `TestTenantResourcesScopeForAdmin` | b | platform-management/internal/console/resources_test.go:90 TestTenantResourcesScopeAnswersOutsideTenantAsNotFound |
| S20 | `TestSystemResourcesTopAndOverLimit` | b | platform-management/internal/console/resources_test.go:217 TestSystemResourcesTotalsTopAndOverLimit |
| S20 | `TestSystemResourcesSystemAdminOnly` | b | platform-management/internal/console/resources_test.go:256 TestSystemResourcesIsSystemAdminOnly |
| S20 | `TestLimitOverridesWriteEveryWorkspaceInOneTransaction` | b | platform-management/internal/subscription/limits_test.go:21 TestLimitOverrideSetsEveryWorkspaceWithOneAuditEach; :56 TestLimitOverrideIsAllOrNothing |
| S20 | `TestLimitOverridesAuditRowPerWorkspaceWithOldNew` | c | platform-management/internal/subscription/limits_test.go:21 (one audit row per workspace, actor, tenant; old and new values not asserted); limits.go:84 SetLimitOverride — no old/new assertion |
| S20 | `TestLimitOverridesBumpVersionAndClearCache` | b | platform-management/internal/subscription/limits_test.go:21 (version 4 and 1; cache dropped for both) |
| S20 | `TestLimitOverridesCreateFreeRowForUnsubscribedWorkspace` | c | platform-management/internal/subscription/limits_test.go:21 (new row version 1 for an unsubscribed workspace); limits.go:108 SetLimitOverrideIn — plan key 'free' on the new row not asserted |
| S20 | `TestLimitOverridesReachPolicyWithin30Seconds` | c | platform-management/internal/entitlement (loader and policy cache; the 30-second bound is not a constant asserted in tests) — no test of propagation time |
| S21 | `TestRuntimeRoutesMatchMatrixRows` | c | platform-management/internal/console/handler.go:227-228 region (runtime routes M41-M43) — runtime routes not in TestRoleMatrix; per-role rows not asserted |
| S21 | `TestRuntimeListOmitsIPAndRawDeviceInfo` | b | platform-management/internal/console/runtimes_test.go:30 TestRuntimeListIsScopedAndHidesDeviceDetails (device_info, metadata, ip absent) |
| S21 | `TestRuntimeUnregisterRequiresReasonAndConfirm` | c | platform-management/internal/console/runtimes_test.go:104 TestRuntimeUnregisterGoesThroughDeletionRequest (success path, confirm passed) — refusal without reason or confirm not asserted for runtimes |
| S21 | `TestRuntimeListScopeAndFilters` | c | platform-management/internal/console/runtimes_test.go:30 (scope asserted; filters not) |
| S21 | `TestRuntimeDetailRecentTasksHaveNoContent` | b | platform-management/internal/console/runtimes_test.go:54 TestRuntimeDetailHasRecentTasksWithoutContent |
| S21 | `TestRuntimeOutsideTenantIsNotFoundForAdmin` | b | platform-management/internal/console/runtimes_test.go:54 (rtOther detail 404) and :96 TestRuntimeStatusOutsideTenantIsNotFound |
| S21 | `TestRuntimeStatusWritesRowAuditAndClearsCache` | c | platform-management/internal/console/runtimes_test.go:69 TestRuntimeDisableIsIdempotentAndAuditedOnce (audit asserted; cache clear not asserted) |
| S21 | `TestRuntimeStatusIsIdempotent` | b | platform-management/internal/console/runtimes_test.go:69 TestRuntimeDisableIsIdempotentAndAuditedOnce |
| S21 | `TestRuntimeUnregisterCreatesRuntimeKindRequestWithAuditFirst` | c | platform-management/internal/console/runtimes_test.go:104 (request kind and target asserted; audit-first order not) |
| S21 | `TestPlatformRuntimeTableHasNoForeignKey` | c | platform-management/migrations server/migrations/1031_platform_runtime.up.sql (no REFERENCES clause) — no schema test |
| S22 | `TestDisabledRuntimeCannotRegister` | a | server/internal/handler/runtime_disabled_test.go:74 |
| S22 | `TestDisabledRuntimeRefusedOnEveryDaemonRoute` | c | server/internal/handler/runtime_disabled_test.go:39 TestDisabledRuntimeRefusedOnDaemonRoutes (heartbeat, claim); :122 TestDisabledRuntimeRefusedOnWSHeartbeat; daemon.go:184 requireDaemonRuntimeAccess — no table over every route using the guard |
| S22 | `TestDisabledRuntimeGetsNoTaskClaimAndTaskStaysQueued` | b | server/internal/service/runtime_disabled_claim_test.go:12 TestClaimRefusesTaskOnDisabledRuntime (refusal runtime_disabled; task stays queued) |
| S22 | `TestRuntimeGuardCacheInvalidatedOnChange` | b | server/internal/runtimeguard/runtimeguard_test.go:99 TestDisableTakesEffectAfterInvalidation |
| S22 | `TestRuntimeGuardDBErrorIs503` | c | server/internal/runtimeguard/runtimeguard_test.go:66 TestDisabledReturnsReadErrorInsteadOfGuessing (error returned); HTTP 503 mapping daemon.go:218-230 — no test asserts the 503 status |
| S22 | `TestEnableRestoresRegisterHeartbeatRoutesAndClaim` | c | server/internal/handler/runtime_disabled_test.go:60 TestEnableRestoresDaemonRoutes (heartbeat only) — register and claim after enable not asserted |
| S22 | `TestStaffRuntimeUnregisterUsesDeleteAgentRuntimeCore` | c | server/internal/handler/runtime_staff_test.go:27 TestStaffRuntimeUnregisterRemovesRuntimeWithStaffAudit (removal); DeleteRuntimeForStaff — reuse of the core not asserted |
| S22 | `TestStaffRuntimeUnregisterEquivalentToOwnerDelete` | c | server/internal/handler/runtime_staff_test.go:27 (staff only) — no comparison with owner delete |
| S22 | `TestStaffRuntimeUnregisterAuditActorStaffWithTenantAndWorkspace` | c | server/internal/handler/runtime_staff_test.go:27 (actor staff and request_id asserted; tenant and workspace not) |
| S22 | `TestDeleteRuntimeAndWorkspaceRemovePlatformRuntimeRows` | d | runtime half: no platform_runtime delete in server/pkg/db/queries (only workspace_delete.sql:784 for workspace); workspace half exists at server/pkg/db/queries/workspace_delete.sql:784 — runtime-delete removal missing; workspace removal has no test |
| S23 | `filters by status plan and tenant and pages with the cursor` | a | platform-management/console/src/screens/Workspaces.test.tsx (test title) |
| S23 | `admin sees only the rows the API returns` | a | platform-management/console/src/screens/Workspaces.test.tsx (test title) |
| S23 | `admin sees plans read only` | a | platform-management/console/src/screens/Plans.test.tsx (test title) |
| S23 | `deactivate and activate call the routes and show the inactive badge` | a | platform-management/console/src/screens/Plans.test.tsx (test title) |
| S23 | `delete shows plan_in_use message` | a | platform-management/console/src/screens/Plans.test.tsx (test title) |
| S23 | `lowering a limit asks for confirmation` | a | platform-management/console/src/screens/Plans.test.tsx (test title) |
| S23 | `renders who when old new` | a | platform-management/console/src/screens/PlanHistory.test.tsx (test title) |
| S23 | `quick actions follow allowed[] and show the 409 state conflict` | a | platform-management/console/src/screens/WorkspaceDetail.test.tsx (test title) |
| S23 | `unknown plan state and workspace status fall back` | a | platform-management/console/src/api/schemas.test.ts (test title) |
| S24 | `lists users with workspaces roles status seats and last login` | a | platform-management/console/src/screens/TenantUsers.test.tsx (test title) |
| S24 | `search filter and paging` | a | platform-management/console/src/screens/TenantUsers.test.tsx (test title) |
| S24 | `tenant deactivate asks reason and lists the workspaces it removes the user from` | a | platform-management/console/src/screens/UserDetail.test.tsx (test title) |
| S24 | `change role offers admin member viewer only` | a | platform-management/console/src/screens/UserDetail.test.tsx (test title) |
| S24 | `global lock and delete hidden for admin` | a | platform-management/console/src/screens/UserDetail.test.tsx (test title) |
| S24 | `allocation opens the workspace action not a per-user quota` | a | platform-management/console/src/screens/UserDetail.test.tsx (test title) |
| S24 | `tenant user row falls back for unknown account_status` | a | platform-management/console/src/api/schemas.test.ts (test title) |
| S25 | `shows top tenants and over limit list for system_admin only` | a | platform-management/console/src/screens/Resources.test.tsx (test title) |
| S25 | `resource bars show used limit percent and warning over states` | a | platform-management/console/src/screens/TenantDetail.test.tsx (test title) |
| S25 | `limit override form hidden for admin` | a | platform-management/console/src/screens/TenantDetail.test.tsx (test title) |
| S25 | `lists runtimes with status last seen versions and no ip column` | a | platform-management/console/src/screens/Runtimes.test.tsx (test title) |
| S25 | `disable asks reason and shows disabled badge` | a | platform-management/console/src/screens/RuntimeDetail.test.tsx (test title) |
| S25 | `unregister requires typing the runtime name` | a | platform-management/console/src/screens/RuntimeDetail.test.tsx (test title) |
| S26 | `TestLegalHoldRoutesMatchMatrixRows` | b | platform-management/internal/console/legal_holds_test.go:33 TestLegalHoldRoutesAreSystemAdminOnly (admin/support/billing 404, system_admin 201) |
| S26 | `TestLegalHoldRequiresReasonAndCaseRef` | b | platform-management/internal/legalhold/legalhold_test.go:6 TestCheckInputRequiresKindReasonAndCaseRef; legalhold/legalhold.go:61 CheckInput |
| S26 | `TestLegalHoldAuditDetailHasOnlyHoldID` | c | platform-management/internal/legalhold/service.go:36 Place (audit entry) — no test on the audit detail content |
| S26 | `TestSecondHoldOnSameScopeIs409` | c | platform-management/internal/legalhold/service.go:34-55 Place (CodeExists legal_hold_exists) — no test in any package references legal_hold_exists (grep) |
| S26 | `TestReleaseKeepsRowAsHistory` | c | platform-management/internal/legalhold/service.go:72 Release (row kept with released_at) — no release test |
| S26 | `TestHoldOnTenantCoversWorkspaceAndAccountChecks` | c | platform-management/internal/deletionrequest/hold.go:19 checkWorkspaceHold (tenant hold check); hold_db_test.go:13 covers workspace only — tenant-hold coverage of a workspace and account not asserted in platform tests |
| S26 | `TestDeleteNowRefusedWhileHeldWritesNothing` | c | platform-management/internal/deletionrequest/hold.go:19; hold_db_test.go:13 TestLegalHoldBlocksPhaseOneWithoutWriting — covers workspace only; tenant and account refusals not asserted |
| S26 | `TestAdminGetsLegalHoldErrorWithoutReasonOrCaseRef` | c | platform-management/internal/console/legal_holds_test.go:46 TestHoldRefusalShowsItsIDOnlyToSystemAdmin (hold id hidden from admin) — reason and case_ref absence in the admin error not asserted |
| S26 | `TestSystemAdminGetsHoldIDInError` | b | platform-management/internal/console/legal_holds_test.go:46 TestHoldRefusalShowsItsIDOnlyToSystemAdmin |
| S26 | `TestLegalHoldTablesHaveNoForeignKey` | c | server/migrations/1038_legal_hold.up.sql and 1039 (no REFERENCES, grep) — no schema test |
| S27 | `TestDeleteWorkspaceDataRefusesHeldWorkspace` | b | server/internal/handler/legal_hold_test.go:49 TestWorkspaceHoldRefusesStaffDeletion; :68 TestTenantHoldCoversWorkspaceLinkedLater; :88 TestOwnerDeleteWorkspaceReturnsLegalHold409 — workspace, tenant, and tenant-then-new-workspace cases covered across these |
| S27 | `TestDeleteAccountRefusesHeldAccount` | b | server/internal/handler/legal_hold_test.go:124 TestAccountHoldRefusesAccountDeletion |
| S27 | `TestDeleteAccountOfSoleMemberOfHeldWorkspaceIsRefused` | a | server/internal/handler/legal_hold_test.go:141 |
| S27 | `TestOwnerDeleteWorkspaceReturnsLegalHold409` | a | server/internal/handler/legal_hold_test.go:88 |
| S27 | `TestWorkspaceDeletionJobSkipsHeldWorkspace` | a | server/internal/scheduler/legal_hold_test.go:28 |
| S27 | `TestAccountDeletionJobSkipsHeldAccount` | c | server/internal/handler/me_deletion.go (account deletion path; not read) — no test of the account job skipping a held account (scheduler/legal_hold_test.go:28 is workspace only) |
| S27 | `TestStaffDeletionFailsWithLegalHoldAndKeepsData` | a | server/internal/staffdeletion/legal_hold_test.go:10 |
| S27 | `TestAuditRetentionSkipsHeldRows` | a | server/internal/handler/legal_hold_test.go:161 |
| S27 | `TestReleaseResumesDeletionAndRetentionOnNextRun` | c | platform-management/internal/legalhold/service.go:72 Release; scheduler jobs — no test of resume after release |
| S27 | `TestHeldObjectOrdinaryOperationsUnaffected` | c | no hold check on ordinary writes (held-object path in server/internal/handler; not tested) — no test |
| S27 | `TestDeletionBlockedByHoldWritesAuditRow` | a | server/internal/handler/legal_hold_test.go:107 |
| S27 | `TestEveryWorkspaceContentDeleteStatementIsInsideDeleteWorkspaceData` | c | server/internal/handler/workspace.go:1684 deleteWorkspaceData (teardown closures from :1812); server/internal/handler/workspace_mcp_api.go:347 calls DeleteWorkspaceMcpServer outside it (a single-server delete, would fail a literal scan) — no static scan test; literal scan would flag workspace_mcp_api.go:347 |
| S27 | `TestAuditQueriesReadonlyStillPasses` | b | platform-management/audit/queries_readonly_test.go:31 TestPlatformAuditQueriesAreInsertAndSelectOnly (body read) |
| S28 | `TestApprovalConfigParsesKnownActionsOnly` | b | platform-management/internal/config/approval_test.go:10 TestApprovalActionsDefaultEmptyAndKnownOnly |
| S28 | `TestApprovalRoutesMatchMatrixRows` | c | platform-management/internal/console/handler.go:209-211 (approvals routes, M49) — no route-role test for approvals in matrix tests |
| S28 | `TestAwaitingApprovalWritesNoStateChangeOrAccessBlock` | b | platform-management/internal/deletionrequest/approval_db_test.go:52 TestTenantDeletionWaitsForASecondAdminAndApprovalRunsPhaseOne (workspace stays active while awaiting) |
| S28 | `TestApproveRunsPhaseOneInOneTransaction` | c | platform-management/internal/deletionrequest/approval.go:107 Approve; approval_db_test.go:52 (phase-one result asserted) — the single-transaction claim is not asserted |
| S28 | `TestApproverMustDiffer` | b | platform-management/internal/deletionrequest/approval_db_test.go:52 (CodeApproverMustDiffer) |
| S28 | `TestSingleSystemAdminCannotRequireApproval` | c | platform-management/internal/deletionrequest/approval_db_test.go:42 TestApprovalNeedsTwoAdminsAndWritesNothingWithOne (requireSecondAdmin code only) — the 409 at the console and absence of a request row not asserted |
| S28 | `TestRejectCancelsRequest` | b | platform-management/internal/deletionrequest/approval_db_test.go:85 TestRejectCancelsAndAwaitingRequestsExpireAfterADay |
| S28 | `TestApprovalExpiresAfter24HoursIdempotent` | c | platform-management/internal/deletionrequest/approval_db_test.go:85 (expiry once; second run not asserted) — idempotence not asserted |
| S28 | `TestDefaultEmptyConfigKeepsDirectDeleteNow` | b | platform-management/internal/config/approval_test.go:10 (default empty); direct path deletionrequest/db_test.go:159 TestDeleteNowPhaseOneWritesAuditRequestAndStatusInOneTransaction — the direct-delete-now result with empty config is not asserted in one test |
| S28 | `TestAdminCannotSeeOrActOnApprovals` | c | platform-management/internal/console/handler.go:209-211 (ActionApprovalsDecide: system_admin only) — no test; matrix M49 admin = None but no test of the admin 404 |
| S28 | `TestLegalHoldCheckedBeforeApprovalIsRequested` | c | platform-management/internal/deletionrequest/hold.go:19 checkWorkspaceHold; approval.go:72 holdForApproval — no test ordering hold check before approval |
| S29 | `TestTenantPolicyRoutesMatchMatrixRows` | c | platform-management/internal/console/handler.go (tenant policy routes M44, M45); no matrix test covers them |
| S29 | `TestTenantPolicyValidationMatchesWorkspacePolicyBounds` | c | platform-management/internal/console/tenant_policy_test.go:60 TestTenantPolicyRejectsBadDomainsAndAges (own bounds; no comparison with workspacepolicy) — no test ties the bounds to workspacepolicy |
| S29 | `TestTenantPolicyWriteNeedsWorkspacePoliciesFeatureSomewhereInTenant` | c | platform-management/internal/console/tenant_policy_test.go:22 TestTenantPolicyNeedsTheWorkspacePoliciesFeature (feature on one workspace only) — "somewhere in the tenant" case not asserted |
| S29 | `TestTenantPolicyVersionConflict` | b | platform-management/internal/console/tenant_policy_test.go:32 TestTenantPolicyVersionConflictWritesNothing |
| S29 | `TestTenantPolicyAuditHasOldAndNewAndTenantID` | c | platform-management/internal/console/tenant_policy_test.go:41 TestTenantPolicyWriteIsAuditedWithOldAndNew (old and new asserted; tenant_id not) |
| S29 | `TestTenantPolicyScopeForAdmin` | b | platform-management/internal/console/tenant_policy_test.go:66 TestTenantPolicyOutsideTenantIsNotFound; :72 TestTenantPolicyReadDefaultsWithoutRow |
| S29 | `TestTenantPolicyTableHasNoForeignKey` | c | schema migration for tenant policy (grep for REFERENCES in its migration not run here) — no schema test |
| S30 | `TestMergeTable` | a | server/internal/workspacepolicy/merge_test.go:10 |
| S30 | `TestEffectivePolicyAppliesTenantFloorToEveryWorkspaceRegardlessOfPlan` | a | server/internal/handler/workspace_policy_floor_test.go:26 |
| S30 | `TestWorkspacePolicyWriteBelowFloorIs400WithFields` | a | server/internal/handler/workspace_policy_floor_test.go:43 |
| S30 | `TestStoredLooseWorkspacePolicyIsTightenedAtEnforcement` | a | server/internal/handler/workspace_policy_floor_test.go:64 |
| S30 | `TestNoTenantPolicyRowKeepsBehaviour` | c | server/internal/workspacepolicy/merge.go (merge with no tenant row); workspace_policy_floor_test.go has no such case — no test with no tenant row |
| S30 | `TestGetWorkspacePolicyReturnsFloor` | a | server/internal/handler/workspace_policy_floor_test.go:81 |
| S30 | `TestDetachTenantDeletesPolicyRow` | a | server/internal/handler/workspace_policy_floor_test.go:99 |
| S30 | `TestOwnerStillSignsInWithEmailCodeUnderTenantSSOFloor` | c | server/internal/workspaceguard/policy_access.go:80 (require_sso does not bind IsOwner); workspacepolicy/merge.go:18 (tenant OR workspace) — no test of the owner's email-code sign-in under the tenant floor |
| S31 | `TestQuotaRequestRoutesMatchMatrixRows` | c | platform-management/internal/console/quota_requests_test.go:71 TestQuotaApproveAndDenyAreSystemAdminOnly (approve/deny only); M46-M48 routes — read routes and create not in matrix tests |
| S31 | `TestQuotaRequestValidation` | b | platform-management/internal/quota/quota_test.go:18 TestRequestedMustBeAboveCurrentAndGateMustHaveALimit; :34 TestGateAndReasonChecks |
| S31 | `TestQuotaAlertSendsOnceAt80AndOnceAt100` | b | platform-management/internal/quotaalert/quotaalert_test.go:79 TestWarningAtEightyThenOneHundredOncePerMonth |
| S31 | `TestQuotaAlertTwiceAndParallelSendsOne` | c | platform-management/internal/quotaalert (Job.Run with Notices store); quotaalert_test.go:79 runs twice sequentially only — no parallel run |
| S31 | `TestQuotaAlertSkipsPendingDeletionWorkspace` | c | platform-management/internal/quotaalert (source of workspaces; not read in detail) — no test |
| S31 | `TestQuotaAlertDoesNotChangeEnforcement` | c | platform-management/internal/quotaalert (alert sends only) — no test that the policy is unchanged |
| S31 | `TestQuotaRequestOnePendingPerWorkspaceGate` | c | platform-management/internal/console/quota_requests_test.go:65 TestQuotaCreateReportsAPendingRequestAsConflict; platform-management/internal/quota/service.go (pending check) — conflict per gate not asserted |
| S31 | `TestApproveWritesOverrideAndAuditInOneTransaction` | c | platform-management/internal/quota/service.go:151-153 (SetLimitOverrideIn in the approval transaction) — no test of the single transaction or the override row |
| S31 | `TestApproveLowerThanRequested` | b | platform-management/internal/quota/quota_test.go:26 TestApprovalMayBeLowerThanRequestedButNotHigher |
| S31 | `TestDenyRequiresReason` | c | platform-management/internal/quota (deny path; not read in detail) — no test asserts deny refuses an empty reason |
| S31 | `TestApproveAndDenyOnlySystemAdmin` | b | platform-management/internal/console/quota_requests_test.go:71 TestQuotaApproveAndDenyAreSystemAdminOnly |
| S31 | `TestQuotaRequestScopeForAdminAndBilling` | c | platform-management/internal/console/quota_requests_test.go:85 TestQuotaListIsScopedByTenant (admin only; billing not asserted) |
| S31 | `TestApprovedOverrideReachesPolicyWithin30Seconds` | c | platform-management/internal/entitlement (cache); quota/service.go:151 — no timing test |
| S31 | `TestResultEmailGoesToRequester` | b | platform-management/internal/console/quota_requests_test.go:115 TestQuotaDecisionEmailsTheRequester |
| S32 | `TestCreateQuotaRequestByOwner` | a | server/internal/handler/quota_request_test.go:45 |
| S32 | `TestCreateQuotaRequestByPlainMemberIsPermissionDenied` | c | server/internal/handler/quota_request.go:55 CreateQuotaRequest (role gate in route, not read) — no test for a plain member |
| S32 | `TestQuotaRequestMachineTokenIs403` | c | server/internal/handler/quota_request.go:55 CreateQuotaRequest (machine guard, not read) — no test for a machine token |
| S32 | `TestQuotaRequestValueMustExceedEffectiveLimit` | a | server/internal/handler/quota_request_test.go:60 |
| S32 | `TestDuplicatePendingIs409` | a | server/internal/handler/quota_request_test.go:78 |
| S32 | `TestListQuotaRequestsScopedToWorkspace` | a | server/internal/handler/quota_request_test.go:95 |
| S32 | `TestDeleteWorkspaceRemovesQuotaRequests` | a | server/internal/handler/quota_request_test.go:120 |
| S33 | `TestAuditExportRoutesMatchMatrixRows` | c | platform-management/internal/console/audit_export_test.go:62 TestAuditExportIsStaffOnly (staff only, not per role) — M51 roles not asserted per row |
| S33 | `TestExportRangeLimits` | b | platform-management/internal/console/audit_export_test.go:27 TestAuditExportRangeLimits |
| S33 | `TestAuditQueriesReadonlyStillPasses` | b | platform-management/audit/queries_readonly_test.go:31 TestPlatformAuditQueriesAreInsertAndSelectOnly |
| S33 | `TestExportFormatsNdjsonAndCsv` | b | platform-management/internal/console/audit_export_test.go:34 TestAuditExportNDJSONIsAuditedWithCount; :53 TestAuditExportCSVHasHeader |
| S33 | `TestExportCursorNoDuplicateNoMissAcrossPages` | c | platform-management/internal/console audit export (cursor not in tests) — no test |
| S33 | `TestExportScopeAdminOwnTenantIncludingDeletedWorkspaceRows` | c | platform-management/internal/console audit export scope (not read) — no test |
| S33 | `TestExportStreamsWithBoundedMemory` | c | platform-management/internal/console audit export writer (streaming not in tests) — no memory test |
| S33 | `TestExportWritesAuditRowWithFilterAndCount` | c | platform-management/internal/console/audit_export_test.go:34 (count asserted; filter not) |
| S33 | `TestExportAddsNoFieldsBeyondConsoleRead` | c | platform-management/internal/console audit export field set (not read) — no test |
| S34 | `TestTenantStatusRoutesMatchMatrixRows` | c | platform-management/internal/console/handler.go (tenant status routes, M52); handler_test.go:389 TestTenantStatusCallsWorkspaceStatusSet — no matrix route test for M52 |
| S34 | `TestTenantStatusWritesAllWorkspacesInOneTransaction` | c | platform-management/internal/tenantstatus/tenantstatus_test.go:11 TestLockWritesEveryWorkspaceNotAlreadyInThatState (targets only) — transaction not asserted |
| S34 | `TestTenantUnlockRestoresOnlyTenantStatusRows` | b | platform-management/internal/tenantstatus/tenantstatus_test.go:23 TestUnlockRestoresOnlyTenantLockedWorkspaces |
| S34 | `TestTenantStatusAuditRows` | c | platform-management/internal/console/handler_test.go:389 TestTenantStatusCallsWorkspaceStatusSet (not read for audit) — audit rows not asserted (not verified) |
| S34 | `TestTenantStatusFailureWritesNothing` | c | platform-management/internal/console/handler_test.go:418 TestTenantStatusRefusals (refusals; write-nothing not read) — not verified |
| S34 | `TestDeleteNowSendsNoticeToOwnersNotTheDeletedUser` | c | platform-management/internal/deletionrequest/notice_db_test.go:41 TestDeleteNowNoticesEveryOwnerAndAuditsEach (owners notified; deleted user not asserted) |
| S34 | `TestNoticeFailureDoesNotBlockDeletionAndIsAudited` | b | platform-management/internal/deletionrequest/notice_db_test.go:57 TestFailedNoticeDoesNotStopTheDeletion |
| S34 | `TestExportFirstQueuesExportForEveryWorkspace` | b | platform-management/internal/deletionrequest/notice_db_test.go:70 TestExportFirstQueuesOneExportPerWorkspace |
| S34 | `TestExportFirstRefusedWhenExportAlreadyRunningReusesIt` | c | platform-management/internal/deletionrequest/pgx.go:392 InsertExport (returns bool: reuses running) — no test of reuse |
| S35 | `TestCreateWorkspaceIntoSuspendedTenantIs409` | a | server/internal/handler/tenant_status_gate_test.go:18 |
| S35 | `TestCreateWorkspaceIntoReadOnlyTenantIs409` | a | server/internal/handler/tenant_status_gate_test.go:31 |
| S35 | `TestCreateWorkspaceIntoActiveTenantUnchanged` | a | server/internal/handler/tenant_status_gate_test.go:45 |
| S35 | `TestStaffDeletionWaitsForExportThenMailsSignedLink` | a | server/internal/staffdeletion/export_first_test.go:30 |
| S35 | `TestStaffDeletionExportTimeoutFailsAndKeepsData` | a | server/internal/staffdeletion/export_first_test.go:56 |
| S35 | `TestStaffDeletionWithoutExportFirstDoesNotWait` | a | server/internal/staffdeletion/export_first_test.go:73 |
| S35 | `TestExportArchiveSurvivesWorkspaceDeletion` | c | server/internal/workspaceexport (archive; not read) — no test |
| S35 | `TestLegalHoldCheckedBeforeExportWait` | c | server/internal/staffdeletion/export_first.go (not read); legal_hold_test.go:10 covers hold refusal only — no test of ordering |
| S36 | `shows merged effective policy and which value comes from the tenant` | a | platform-management/console/src/screens/TenantPolicy.test.tsx (test title) |
| S36 | `admin edits own tenant policy only` | a | platform-management/console/src/screens/TenantPolicy.test.tsx (test title) |
| S36 | `system_admin approves with a lower value and gives a reason` | a | platform-management/console/src/screens/QuotaRequests.test.tsx (test title) |
| S36 | `admin cannot approve` | a | platform-management/console/src/screens/QuotaRequests.test.tsx (test title) |
| S36 | `approver must differ message and refuses own request` | a | platform-management/console/src/screens/Approvals.test.tsx (test title) |
| S36 | `place requires reason and case ref` | a | platform-management/console/src/screens/LegalHolds.test.tsx (test title) |
| S36 | `admin has no menu entry` | a | platform-management/console/src/screens/LegalHolds.test.tsx (test title) |
| S36 | `export form rejects ranges over 31 days` | a | platform-management/console/src/screens/Audit.test.tsx (test title) |
| S36 | `export first toggle is sent and owners notice is stated` | a | platform-management/console/src/components/TypeToConfirmDialog.test.tsx (test title) |
| S36 | `awaiting_approval legal_hold and export_timeout states render` | a | platform-management/console/src/screens/DeletionRequests.test.tsx (test title) |
| S37 | `shows request increase after plan_limit_reached and lists my requests` | a | packages/views/settings/components/billing-tab.test.tsx (test title) |
| S37 | `member without billing.manage does not see the button` | a | packages/views/settings/components/billing-tab.test.tsx (test title) |
| S37 | `locks fields set by the tenant floor and shows the reason` | a | packages/views/settings/components/policy-tab.test.tsx (test title) |
| S37 | `quota request and floor accept missing fields and unknown states` | a | packages/core/api/schemas.test.ts (test title) |

## 7. Limits of this audit

- Name-only matches (the candidate body was not read): S17 `TestWorkspaceDetailOutsideTenantIsNotFoundForAdmin`; S26 `TestLegalHoldRequiresReasonAndCaseRef` (the `CheckInput` function was read, not the test body); S28 `TestApprovalConfigParsesKnownActionsOnly` and `TestDefaultEmptyConfigKeepsDirectDeleteNow` (matched on the config test name and title only); S29 `TestTenantPolicyVersionConflict` and `TestTenantPolicyScopeForAdmin`; S31 `TestApproveAndDenyOnlySystemAdmin` and `TestResultEmailGoesToRequester`.
- Rows whose implementation was not read in full (the note says "not read"): S16 `TestPlanRoutesMatchMatrixRows`; S27 `TestAccountDeletionJobSkipsHeldAccount`; S31 `TestQuotaAlertSkipsPendingDeletionWorkspace`; S31 `TestDenyRequiresReason`; S32 `TestCreateQuotaRequestByPlainMemberIsPermissionDenied`; S32 `TestQuotaRequestMachineTokenIs403`; S33 `TestExportScopeAdminOwnTenantIncludingDeletedWorkspaceRows`; S33 `TestExportAddsNoFieldsBeyondConsoleRead`; S34 `TestTenantStatusAuditRows`; S34 `TestTenantStatusFailureWritesNothing`; S35 `TestExportArchiveSurvivesWorkspaceDeletion`; S35 `TestLegalHoldCheckedBeforeExportWait`.
- Class a UI rows are present by title, not by file: the file that holds a title can differ from the one the plan names (for example S13 "lists blocking workspaces…" is in `UserAccountMenu.test.tsx`, not `UserDetail.test.tsx`).
- Presence of a title or name does not mean the test passes. No test was run.
