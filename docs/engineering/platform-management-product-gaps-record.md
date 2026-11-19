# Platform management product gaps: record

Date: 2026-11-19. Checkout: `main`. Test database: `multica_platform_test` only, DSN built from `.env` with the database name changed inside each command. The shared `multica` database was not used.

Flow rows in `platform-management-flows.md` are not edited here: that file has uncommitted changes from another session. This record is the place for this batch.

## Results

| Item | Result | Commit | Named test |
| --- | --- | --- | --- |
| Flow 276: malformed X-User-ID on the internal entitlement route | Built: 401 `unauthorized`, policy not loaded. Missing header still 200 (client sends none). | `022079bb9` | `TestHandlerRefusesAMalformedUserHeader` (PASS) |
| Tenants at 375px: page overflow 31px | Cause is the header account group in `console/src/components/Layout.tsx` (`shrink-0`, about 382px), not the Tenants screen. Changed to `min-w-0 flex-wrap`. Table scrolls in its own container. | `65acc0c03` | `edge: the Tenants screen fits a 375 by 667 phone...` in `e2e/platform/ui/admin.spec.ts`. **Real-stack run NOT passed**: sign-in TOTP refused on this host, before the assertion ran. Browser measurement with stubbed APIs: overflow 31 to 0. Console unit tests pass (13/13). |
| Flow 25: welcome resend audit | Built: a resend records `account.welcome_resent`; first welcome unchanged. | `d5f0d205f` | `TestResendWelcomeRecordsWelcomeResent` (PASS); `TestWelcomeResendLimitPerDay` updated to 1 sent + 3 resent |
| E-invoice service not passed to console (retry 503 with provider configured) | Fixed: `Einvoices` set in `console.Deps`; service built before the console. | `1693019dd` | `TestRetryEinvoiceReachesTheProviderWhenConfigured` (PASS; FAIL on the previous `main.go`) |
| Flow 239: stale `expected_version` on PUT plan | Built: 409 `plan_version_conflict`, check inside the locked transaction. Absent field: unchanged. | `5d82fd559` | `TestPutPlanRefusesAStaleExpectedVersion` (PASS) |
| Flow 66: POST creates a plan | Built: 201, 409 `plan_exists` on a duplicate key, validation as PUT, audited as `plan.changed` with `created: true`. No console form. | `5f4a9b732` | `TestPostPlanCreatesAPlan` (PASS) |
| Flow 73: one order's e-invoice history at `GET /api/platform/einvoices/{orderId}` | **Not built.** The current spec (P43.6) moved this history to `GET /api/platform/orders/{id}/einvoices`; that route exists. Adding the old path back would contradict the spec. | none | Existing `TestDBOrderEinvoiceHistoryReadsTheRevisionSnapshot` covers the spec path (PASS). |
| Flow 231: `invalid_buyer` on e-invoice replace | **Not built.** Replace was removed on 2026-11-18 (spec: the route returns 404), and no `invalid_buyer` code remains in platform-management. The live buyer input is Multica's `PUT /api/workspaces/{id}/orders/{orderId}/invoice-buyer` in `server/`, outside this scope. | none | none |

Flows 76, 77, 78 and 79 were not built, as instructed. Retry (flow 76 and 78 wording) now reaches the provider when it is configured, through the wiring fix above; replace and cancel stay out.

## Test commands

```bash
cd /home/av-server/Documents/Github/multica
TESTDSN="$(grep '^DATABASE_URL=' .env | cut -d= -f2- | sed 's#/multica?#/multica_platform_test?#')"
DATABASE_URL="$TESTDSN" bash scripts/require-test-database.sh
cd platform-management
PLATFORM_TEST_DATABASE_URL="$TESTDSN" DATABASE_URL="$TESTDSN" go test ./internal/entitlement/ ./internal/trust/ ./internal/accountrequest/ ./audit/ ./internal/subscription/ ./internal/plan/ ./cmd/platform-server/ -count=1 -p 1
```

Result of the final run of the packages touched by this batch: see the session log; each named test above was run and passed at its commit.

## Open

- The Tenants layout e2e test has not run to a pass. The failure is sign-in TOTP refused in `beforeAll` (`e2e/platform/ui/session.ts`), and `e2e/platform/harness.ts` has uncommitted edits from another session.
- Flow 231 and the flow 73 path decision need a product decision (see the rows above).
