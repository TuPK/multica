# R4 currency inventory (P44.1)

Every place in the product code that names a currency: a currency code, a currency symbol,
a currency field or a money formatter. Spec: `platform-management-spec.md` section 7.4, K25.
The test `server/internal/currencyinventory/inventory_test.go` reads this file.

Status values:

- `todo`: not yet in the currency rule set. The only status in use at P44.1.
- `ruled`: reads the currency from the plan or subscription through the rule set and holds no hard-coded code.

How the test reads it: it scans the non-test source under `server`, `platform-management`
and `packages` (no database), builds one unit per (file, term), and fails when a unit has no
row below (`not listed: <file> <term>`), when a row matches no unit (`stale row: <file> <term>`),
when a Status is not `todo` or `ruled`, when Holds or Function or field is empty, or when two rows
share a key. Remove a row in the same commit that removes its last literal.

Scanned files: `.go`, `.sql`, `.ts`, `.tsx`, `.js`, `.mjs` and `.json`. Skipped: the directories
`node_modules`, `dist`, `generated`, `testdata`, `.git` and `__tests__`; the files `*_test.go`,
`*.test.ts(x)`, `*.spec.ts(x)` and `*.sql.go` (sqlc output; its source in `queries/*.sql` is
scanned); and `.json` files under a `contract/` path.

Terms (a line matches when any pattern of the term matches):

- `currency`: `(?i)(?:^|[^n])currenc[yi]`. The `[yi]` suffix excludes "concurrency", "occurrence" and "recurrence".
- `vnd`: the token `VND` or `vnd`, quoted or bare, and the symbol `₫`.
- `usd`: the token `USD`, and the quoted `"usd"`.
- `cad`: the token `CAD` or `cad`. No unit matches at P44.1.
- `money-format`: `formatStripeMinorAmount` or `formatMoney`.

Kind values: `field` (struct field, column, JSON key or TS property that holds a currency code),
`literal` (hard-coded currency code, symbol or currency constant), `formatter` (formats an amount),
`text` (comment, UI copy or a false match; no code depends on it).

## Scanned units

Count: 151 scanned units (file and term pairs), 151 rows.

### SQL (migrations and queries)

| File | Term | Function or field | Kind | Holds | Status | Note |
| --- | --- | --- | --- | --- | --- | --- |
| `platform-management/queries/billing.sql` | `currency` | `CreateBillingOrder` | field | code | todo | billing_order.currency column in the insert. |
| `platform-management/queries/einvoice.sql` | `currency` | `InsertEinvoiceIssuance`, `ListPaidOrdersNeedingEinvoice` | field | code | todo | einvoice_issuance.currency column in the insert. |
| `platform-management/queries/payment_notice.sql` | `currency` | `InsertPaymentNotice` | field | code | todo | payment_notice.currency column in the insert. |
| `platform-management/queries/plan.sql` | `currency` | `InsertPlanIfAbsent`, `UpsertPlan`, `PlanHasSubscriptionsOrOrders` | field | code | todo | plan.currency column in insert and upsert. |
| `platform-management/queries/usage_charge.sql` | `currency` | `GetUsageTerms` | field | code | todo | Reads plan.currency for usage terms. |
| `server/migrations/1007_payment_notice.up.sql` | `currency` | `payment_notice` | field | code | todo | payment_notice.currency column exists. |
| `server/migrations/1011_einvoice_issuance.up.sql` | `currency` | `einvoice_issuance` | field | code | todo | einvoice_issuance.currency column exists. |
| `server/migrations/213_task_usage_authoritative_cost.up.sql` | `usd` | `file-level comment` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/migrations/608_task_usage_credits.up.sql` | `usd` | `file-level comment` | text | comment | todo | Comment: credits have no USD rate; no change needed. |
| `server/migrations/670_plan.up.sql` | `currency` | `plan` | field | code | todo | plan.currency column exists. |
| `server/migrations/677_billing_order.up.sql` | `currency` | `billing_order` | field | code | todo | billing_order.currency column exists. |
| `server/pkg/db/queries/digest.sql` | `usd` | `DigestWorkspaceUsage` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/pkg/db/queries/metric.sql` | `usd` | `BuiltinRunStats` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/pkg/db/queries/scorecard.sql` | `usd` | `ScorecardRunCost` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/pkg/db/queries/task_usage.sql` | `usd` | `UpsertTaskUsage` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |

### Go

| File | Term | Function or field | Kind | Holds | Status | Note |
| --- | --- | --- | --- | --- | --- | --- |
| `platform-management/cmd/platform-server/main.go` | `currency` | `run` | literal | code | todo | Looks up the provider for "vnd"; P44.2: look up by order currency. |
| `platform-management/cmd/platform-server/main.go` | `vnd` | `run` | literal | code | todo | Hard-coded "vnd" provider lookup and comment; P44.2: read from plan or subscription. |
| `platform-management/internal/accountrequest/finalize.go` | `currency` | `writeGrant`, `orderDetail` | field | code | ruled | Lower-cases the plan currency; an empty one falls back to money.Default (P44.2). |
| `platform-management/internal/billing/changeorder.go` | `currency` | `CreateChangeOrder`, `RenewalDraft` | field | code | todo | Change order copies Currency from the target plan. |
| `platform-management/internal/billing/order.go` | `currency` | `Order.Currency` | field | code | todo | Order.Currency. |
| `platform-management/internal/billing/pgx.go` | `currency` | `orderFromRow`, `InsertOrder` | field | code | todo | Row mapping for orders. |
| `platform-management/internal/billing/pricing.go` | `vnd` | `daysCeil` | text | comment | todo | Comment: rounds to whole VND; reword when P44.2 lands. |
| `platform-management/internal/billing/provider.go` | `currency` | `Instructions.Currency`, `file-level comment`, `Notification.Currency`, `CreatePaymentRequest` | field | code | todo | Instructions, Notification and CreatePaymentRequest carry Currency. |
| `platform-management/internal/billing/rate.go` | `vnd` | `file-level comment` | text | comment | todo | Comment: money is integer VND; reword when P44.2 lands. |
| `platform-management/internal/money/exponent.go` | `currency` | `Exponent` | field | code | ruled | The rule set itself: the exponent table of enabled currencies (P44.2). |
| `platform-management/internal/money/exponent.go` | `vnd` | `exponents` | literal | code | ruled | The rule set itself: the one place the vnd exponent (0) lives (P44.2). |
| `platform-management/internal/money/money.go` | `currency` | `Default` | field | code | ruled | The rule set itself: the package doc and the default currency constant (P44.2). |
| `platform-management/internal/money/money.go` | `vnd` | `Default` | literal | code | ruled | The rule set itself: Default = "vnd", the one place the default literal lives (P44.2). |
| `platform-management/internal/billing/reconcile.go` | `currency` | `noticeEntry` | field | code | todo | Audit detail copies the notice currency next to the order currency. Added at P44.2 (row was missing from the P44.1 table). |
| `platform-management/internal/billing/reconcile_match.go` | `currency` | `matchNotice` | field | code | todo | Compares notice and order currency (`currency_mismatch`). Added at P44.2 (row was missing from the P44.1 table). |
| `platform-management/internal/billing/registry.go` | `currency` | `file-level comment`, `ErrNoProvider`, `NewRegistry`, `ForCurrency` | field | code | todo | Registry maps currency code to provider; ForCurrency is the lookup. |
| `platform-management/internal/billing/registry.go` | `vnd` | `file-level comment`, `NewRegistry` | literal | code | todo | Empty spec means "vnd=manual"; P44.2: configure per plan currency. |
| `platform-management/internal/billing/sepay.go` | `currency` | `sepayCurrency`, `ParseNotification` | literal | code | todo | sepayCurrency constant used for the notice currency; P44.2: read from the order. |
| `platform-management/internal/billing/sepay.go` | `vnd` | `sepayCurrency` | literal | code | todo | sepayCurrency = "VND"; P44.2: read from the order. |
| `platform-management/internal/billing/service.go` | `currency` | `currencyPattern`, `Draft.Currency`, `validate`, `orderDetail` | field | code | todo | Draft.Currency and the three-letter pattern check. |
| `platform-management/internal/billing/usage.go` | `currency` | `ActivateUsage` | literal | code | todo | Zero-amount usage order gets Currency "vnd"; P44.2: read from plan or subscription. |
| `platform-management/internal/billing/usage.go` | `vnd` | `file-level comment`, `ActivateUsage` | literal | code | todo | Literal "vnd" on the usage activation order; P44.2: read from plan or subscription. |
| `platform-management/internal/billing/vat.go` | `vnd` | `file-level comment` | text | comment | todo | Comment: VAT in whole VND; reword when P44.2 lands. |
| `platform-management/internal/billing/webhook.go` | `currency` | `serve` | field | code | todo | Copies notification Currency into the stored notice. |
| `platform-management/internal/billingapi/billingapi.go` | `currency` | `checkout`, `previewResponse.Currency`, `preview`, `purchaseRequest.Currency`, `purchaseResponse.Currency`, `purchase` | field | code | todo | Request and response Currency; quote lower-cases plan currency. |
| `platform-management/internal/billingapi/billingapi.go` | `vnd` | `refuseUnpaidUsage` | text | comment | todo | Comment: paid for 0 VND; reword when P44.2 lands. |
| `platform-management/internal/billingapi/planchange.go` | `currency` | `planChangePreviewResponse.Currency`, `planChangeOrderResponse.Currency`, `previewPlanChange`, `writeChangeOrder` | field | code | todo | Plan-change preview and order response Currency. |
| `platform-management/internal/config/config.go` | `currency` | `Error`, `EmptyBankVariables`, `loadBilling`, `loadPayments` | literal | code | todo | loadPayments requires a provider for "vnd"; P44.2: require one per plan currency. |
| `platform-management/internal/config/config.go` | `vnd` | `loadBilling`, `loadPayments` | literal | code | todo | Boot check uses money.Default (P44.2); the comment above loadPayments still names vnd. Per-plan-currency check stays todo. |
| `platform-management/internal/console/orders.go` | `currency` | `orderJSON.Currency`, `toOrderJSON`, `cancelOrder` | field | code | todo | Order JSON Currency. |
| `platform-management/internal/console/pgx.go` | `currency` | `orderFromRow` | field | code | todo | Row mapping for orders. |
| `platform-management/internal/db/models.go` | `currency` | `BillingOrder.Currency`, `EinvoiceIssuance.Currency`, `PaymentNotice.Currency`, `Plan.Currency` | field | code | todo | sqlc model fields for billing_order, einvoice_issuance, payment_notice and plan. |
| `platform-management/internal/db/models.go` | `usd` | `file-level comment` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `platform-management/internal/invoice/pgx.go` | `currency` | `insert`, `ReplaceIssued`, `fromRow` | field | code | todo | Insert and row mapping for e-invoice issuance. |
| `platform-management/internal/invoice/provider.go` | `currency` | `IssueRequest.Currency` | field | code | todo | IssueRequest.Currency. |
| `platform-management/internal/invoice/record.go` | `currency` | `Record.Currency`, `NewRecord.Currency` | field | code | todo | Record and NewRecord Currency. |
| `platform-management/internal/invoice/service.go` | `currency` | `Enqueue`, `process` | field | code | todo | Copies the order and record currency into the issuance request. Added at P44.2 (row was missing from the P44.1 table). |
| `platform-management/internal/invoice/store.go` | `currency` | `PaidOrder` | field | code | todo | PaidOrder.Currency. Added at P44.2 (row was missing from the P44.1 table). |
| `platform-management/internal/lifecycle/expire.go` | `currency` | `Run` | field | code | todo | Audit detail copies the order currency. |
| `platform-management/internal/lifecycle/pgx.go` | `currency` | `orderFromRow` | field | code | todo | Row mapping for orders. |
| `platform-management/internal/paymentnotice/notice.go` | `currency` | `Notice.Currency`, `NewNotice.Currency` | field | code | todo | Notice and NewNotice Currency. |
| `platform-management/internal/paymentnotice/pgx.go` | `currency` | `Insert`, `fromRow` | field | code | todo | Insert and row mapping for payment notices. |
| `platform-management/internal/plan/pgx.go` | `currency` | `InsertIfAbsent`, `FromRow`, `insertParams.Currency`, `toInsertParams` | field | code | todo | Insert params and row mapping for plan. |
| `platform-management/internal/plan/plan.go` | `currency` | `IsUnlimited`, `Plan.Currency`, `Catalogue`, `Validate` | field | code | todo | Plan.Currency and the lower-case three-letter pattern. |
| `platform-management/internal/plan/plan.go` | `vnd` | `IsUnlimited` | text | comment | todo | Comment: VND has no decimals; reword when P44.2 lands. |
| `platform-management/internal/subscription/api.go` | `currency` | `priceJSON.Currency`, `listedPlanJSON.Currency`, `buildPrices` | field | code | todo | priceJSON and listedPlanJSON Currency from the plan. |
| `platform-management/internal/subscription/console.go` | `currency` | `putPlanBody.Currency`, `putPlan` | field | code | todo | PutPlan body currency and the plan_currency_locked check. |
| `platform-management/internal/subscription/quote.go` | `vnd` | `conflict` | text | comment | todo | Comment on the quote rounding; reword when P44.2 lands. |
| `platform-management/internal/subscription/usage.go` | `currency` | `Usage.Currency` | field | code | todo | Usage.Currency. |
| `platform-management/internal/usage/invoicing.go` | `currency` | `invoice`, `invoiceDraft` | literal | code | todo | Usage invoice draft uses "vnd"; P44.2: read from plan or subscription. |
| `platform-management/internal/usage/invoicing.go` | `vnd` | `invoiceDraft` | literal | code | todo | Literal "vnd" on the usage invoice draft; P44.2: read from plan or subscription. |
| `platform-management/internal/usage/notice.go` | `vnd` | `noticeText` | literal | amount with code in message text | todo | Notice text prints "VND"; P44.2: print the plan currency. |
| `platform-management/internal/usage/view.go` | `currency` | `UsageInput.Currency`, `BuildUsage`, `Usage` | field | code | todo | UsageInput.Currency and Usage built from plan currency. |
| `server/cmd/multica/cmd_company_usage.go` | `usd` | `file-level comment`, `runCompanyUsage` | text | UI label | todo | CLI heading "COST (USD TICKS)"; agent cost, no change needed. |
| `server/internal/cli/update.go` | `vnd` | `fetchReleaseByTag`, `FetchLatestRelease` | text | media type | todo | False match: media type "vnd.*", not a currency; no change needed. |
| `server/internal/daemon/types.go` | `usd` | `file-level comment` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/internal/handler/agent.go` | `usd` | `attributionsOf` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/internal/handler/billing_orders.go` | `currency` | `workspacePlanJSON.Currency`, `billingBankDetails`, `workspaceOrderJSON.Currency`, `toWorkspaceOrderJSON`, `ListWorkspacePlans` | field | code | todo | Plan and order JSON Currency. |
| `server/internal/handler/billing_orders.go` | `vnd` | `file-level comment`, `billingBankDetails` | text | comment | todo | Comments on the usage rate and VND having no decimals; reword when P44.2 lands. |
| `server/internal/handler/cloud_billing.go` | `currency` | `cloudSubscriptionSeatPurchaseRequest.Currency`, `GetCloudWorkspaceSubscriptionUsage`, `PurchaseCloudWorkspaceSubscriptionSeats`, `isASCIICurrency` | field | code | todo | Seat purchase request Currency, lower-cased, and the isASCIICurrency check. |
| `server/internal/handler/daemon.go` | `usd` | `formatLegacyCommentEntry` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/internal/handler/dashboard.go` | `usd` | `parseProjectIDParam`, `listDashboardUsageDaily` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/internal/handler/github.go` | `vnd` | `fetchInstallationAccount`, `setGitHubAPIHeaders` | text | media type | todo | False match: media type "vnd.*", not a currency; no change needed. |
| `server/internal/handler/proposal.go` | `currency` | `relatedToResponse` | text | comment | todo | Comment mentions the workspace currency; no change needed. |
| `server/internal/handler/runtime.go` | `usd` | `runtimeToResponse`, `GetRuntimeTaskActivity`, `GetRuntimeUsageByAgent` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/internal/handler/skill.go` | `vnd` | `githubRefExists` | text | media type | todo | False match: media type "vnd.*", not a currency; no change needed. |
| `server/internal/integrations/ghsnapshot/client.go` | `vnd` | `mintInstallationToken`, `restGet` | text | media type | todo | False match: media type "vnd.*", not a currency; no change needed. |
| `server/internal/integrations/lark/outbound_media.go` | `vnd` | `isImageAttachment` | text | media type | todo | False match: media type "vnd.*", not a currency; no change needed. |
| `server/internal/metrics/business.go` | `usd` | `NewBusinessMetrics`, `ObserveChatClaimRolloutMissingQuery` | text | metric help text and comment | todo | Help text for the cost metric; agent cost, no change needed. |
| `server/internal/metrics/pricing.go` | `usd` | `file-level comment` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/internal/salescollect/probe.go` | `usd` | `file-level comment` | text | UI copy | todo | Price hint for a data source; no change needed. |
| `server/internal/salescollect/recipe.go` | `usd` | `Collectable` | text | comment and error text | todo | Budget wording for paid providers; no change needed. |
| `server/internal/service/budget_accounting.go` | `usd` | `file-level comment` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/internal/service/digest.go` | `usd` | `computeWorkspace` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/internal/service/knowledge_sales_kinds.go` | `currency` | `urlField` | field | code (free text) | todo | Sales knowledge kind has a free-text "currency" field; not billed money. |
| `server/internal/service/knowledge_sales_kinds.go` | `usd` | `urlField` | text | UI label | todo | Field label "Cost, 30 days (USD)"; no change needed. |
| `server/internal/service/metrics.go` | `usd` | `file-level comment` | literal | unit label "USD" | todo | Metric catalogue unit for run cost (agent cost, not billing). |
| `server/internal/service/metrics_builtin.go` | `usd` | `file-level comment` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/internal/service/scorecard_metrics.go` | `usd` | `file-level comment` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/internal/service/task.go` | `usd` | `captureTaskCancelled` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/pkg/agent/agent.go` | `usd` | `runContext` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `server/pkg/agent/reasonix.go` | `currency` | `observe`, `isReasonixUSDCurrency` | field | code | todo | Parses the CLI turn currency; only USD is priced. |
| `server/pkg/agent/reasonix.go` | `usd` | `isReasonixUSDCurrency` | literal | code | todo | Accepts "USD", "$", "US$" for agent cost; not billed money, P44.2 decides whether it stays. |

### TypeScript and TSX

| File | Term | Function or field | Kind | Holds | Status | Note |
| --- | --- | --- | --- | --- | --- | --- |
| `packages/core/api/client.ts` | `currency` | `hubImportPreviewFromResponse` | field | code | todo | Copies currency from the response. |
| `packages/core/api/schemas.ts` | `currency` | `BillingTopupSchema`, `BillingCheckoutSessionStatusSchema`, `EMPTY_BILLING_CHECKOUT_SESSION_STATUS`, `WorkspaceUsagePeriodSchema`, `WorkspaceSubscriptionPriceSchema`, `WorkspaceSeatPurchasePreviewSchema`, `PurchaseWorkspaceSeatsResponseSchema`, `WorkspacePlanChangePreviewSchema`, `OrderPlanChangeResponseSchema` | field | code | todo | Schema field for the currency code. |
| `packages/core/api/schemas.ts` | `usd` | `EMPTY_CLOUD_RUNTIME_NODE`, `BillingTopupSchema`, `BillingCheckoutSessionStatusSchema`, `EMPTY_BILLING_CHECKOUT_SESSION_STATUS` | literal | code | todo | P44.2: default comes from the plan or subscription, not "usd". |
| `packages/core/billing/workspace-orders-schemas.ts` | `currency` | `WorkspacePlanSchema`, `WorkspaceOrderSchema` | field | code | todo | Schema field for plan and order currency. |
| `packages/core/billing/workspace-orders-types.ts` | `currency` | `file-level comment`, `WorkspacePlan.currency`, `WorkspaceOrder.currency` | field | code | todo | Type field for plan and order currency. |
| `packages/core/billing/workspace-orders-types.ts` | `vnd` | `file-level comment` | text | comment | todo | Comment only: "VND has no decimals"; reword when P44.2 lands. |
| `packages/core/squads/company-template.ts` | `usd` | `DEFAULT_RUN_COST_USD`, `CompanyEstimate`, `estimateCompanyRuns` | literal | USD amount (0.5 per run) | todo | Run-cost estimate for the company wizard, not billed money; decide in P44.2 whether it stays USD. |
| `packages/core/squads/kitviet-previous-texts.ts` | `usd` | `KITVIET_PREVIOUS_AGENT_INSTRUCTIONS` | text | prompt text | todo | Frozen agent prompt text; no change needed. |
| `packages/core/squads/sales-starter-sources.ts` | `usd` | `SALES_STARTER_SOURCES` | text | UI copy | todo | Price hint for third-party data sources; no change needed. |
| `packages/core/types/agent.ts` | `usd` | `AgentTask`, `IssueUsageSummary` | text | comment | todo | Comment about provider cost ticks in USD (agent cost accounting, not billing); no change needed. |
| `packages/core/types/billing.ts` | `currency` | `BillingTopup.currency`, `BillingCheckoutSessionStatus.currency`, `WorkspaceUsagePeriod.currency`, `WorkspaceSubscriptionPrice.currency`, `WorkspaceSubscriptionPrice`, `WorkspaceSeatPurchasePreview.currency`, `WorkspaceSeatPurchasePreview`, `PurchaseWorkspaceSeatsRequest.currency`, `PurchaseWorkspaceSeatsResponse.currency`, `WorkspacePlanChangePreview.currency`, `CreateWorkspacePlanChangeResponse.currency` | field | code | todo | Type fields for the currency code; the amounts beside them are minor units. |
| `packages/core/types/billing.ts` | `usd` | `file-level comment` | text | comment | todo | Comment on the credit unit (1 USD = 1000 credit); no change needed. |
| `packages/core/types/billing.ts` | `vnd` | `WorkspaceUsageMeter` | text | comment | todo | Comment on the usage meter price unit; reword when P44.2 lands. |
| `packages/ui/components/ui/number-flow.tsx` | `currency` | `CurrencyNumberFlowProps`, `CurrencyNumberFlow`, `CappedNumberFlow` | formatter | formatted amount | todo | Currency number component; P44.2: confirm which code it formats. |
| `packages/views/billing/billing-test-page.tsx` | `currency` | `CheckoutSessionStatusBanner`, `TopupRow`, `formatMoney` | formatter | formatted amount | todo | Local formatMoney via Intl.NumberFormat; P44.2: use the shared formatter. |
| `packages/views/billing/billing-test-page.tsx` | `money-format` | `CheckoutSessionStatusBanner`, `TierButton`, `TopupRow`, `formatMoney` | formatter | formatted amount | todo | Calls the local formatMoney; P44.2: use the shared formatter. |
| `packages/views/billing/billing-test-page.tsx` | `usd` | `TierButton` | literal | code | todo | Test page passes "usd" to formatMoney; P44.2: read from the tier. |
| `packages/views/billing/order-page.tsx` | `currency` | `BillingOrder.currency`, `normalizeOrder`, `BillingOrderPage` | field | code | todo | Order currency passed to the formatter. |
| `packages/views/billing/order-page.tsx` | `money-format` | `file-level comment`, `BillingOrderPage` | formatter | formatted amount | todo | Calls formatStripeMinorAmount. |
| `packages/views/billing/order-page.tsx` | `vnd` | `normalizeOrder` | literal | code | todo | Falls back to "vnd" when the order has no currency; P44.2: no fallback, read from the order. |
| `packages/views/billing/plan-change-dialog.tsx` | `currency` | `PlanChangeDialog` | field | code | todo | Preview currency passed to the formatter. |
| `packages/views/billing/plan-change-dialog.tsx` | `money-format` | `file-level comment`, `PlanChangeDialog` | formatter | formatted amount | todo | Calls formatStripeMinorAmount. |
| `packages/views/billing/scheduled-price-change-banner.tsx` | `currency` | `ScheduledPriceChangeBannerProps.currency`, `ScheduledPriceChangeBanner` | field | code | todo | Banner prop passed to formatUsageMoney. |
| `packages/views/billing/usage-card.tsx` | `currency` | `formatUsageMoney`, `UsageCard` | formatter | formatted amount | todo | formatUsageMoney appends the code in upper case; P44.2: use the shared formatter. |
| `packages/views/billing/usage-card.tsx` | `money-format` | `file-level comment`, `formatUsageMoney` | formatter | formatted amount | todo | Calls formatStripeMinorAmount. |
| `packages/views/billing/usage-invoice-lines.tsx` | `currency` | `UsageInvoiceLinesProps.currency`, `UsageInvoiceLines` | field | code | todo | Prop passed to formatUsageMoney. |
| `packages/views/company/components/company-insights-parts.tsx` | `currency` | `usd`, `usdWhole` | formatter | formatted amount | todo | Intl formatters fixed to USD for company run cost; P44.2: decide with the cost-display rule. |
| `packages/views/company/components/company-insights-parts.tsx` | `usd` | `costChartConfig`, `usd`, `usdWhole` | literal | code | todo | Hard-coded "USD" in Intl formatters and chart label. |
| `packages/views/company/components/company-starter-wizard.tsx` | `usd` | `file-level comment`, `CompanyStarterWizard` | formatter | formatted amount | todo | Shows the default run cost with formatUsd. |
| `packages/views/company/lib/company-insights.ts` | `usd` | `MICROS_PER_USD`, `microsToUsd` | literal | conversion constant | todo | Micros-to-dollars divisor for run cost; not a currency code. |
| `packages/views/dashboard/components/dashboard-page.tsx` | `currency` | `file-level comment`, `DashboardPage` | formatter | formatted amount | todo | Uses CurrencyNumberFlow for cost values. |
| `packages/views/editor/mermaid-viewer.tsx` | `vnd` | `MermaidViewerContent` | text | media type | todo | False match: media type "vnd.*", not a currency; no change needed. |
| `packages/views/governance/governance-budgets-tab.tsx` | `currency` | `file-level comment`, `formatBudgetUsd` | formatter | formatted amount | todo | formatBudgetUsd formats budgets with a fixed USD code. |
| `packages/views/governance/governance-budgets-tab.tsx` | `usd` | `formatBudgetUsd` | literal | code | todo | Hard-coded "USD" in the Intl formatter. |
| `packages/views/runtimes/components/runtime-list.tsx` | `currency` | `file-level comment`, `CostCell` | formatter | formatted amount | todo | Uses CurrencyNumberFlow for cost values. |
| `packages/views/runtimes/components/update-section.tsx` | `vnd` | `fetchLatestVersion` | text | media type | todo | False match: media type "vnd.*", not a currency; no change needed. |
| `packages/views/runtimes/components/usage-section.tsx` | `currency` | `file-level comment`, `UsageSection` | formatter | formatted amount | todo | Uses CurrencyNumberFlow for cost values. |
| `packages/views/runtimes/utils.ts` | `usd` | `formatCredits`, `hasTokenFigure`, `MODEL_PRICING`, `Priceable`, `COST_USD_TICKS_PER_USD`, `estimateCost`, `estimateCostBreakdown` | literal | USD rate and amount | todo | Token price table and tick divisor are USD; agent cost, not billed money; decide in P44.2 whether it stays USD. |
| `packages/views/settings/components/billing-format.ts` | `currency` | `STRIPE_ZERO_DECIMAL_CURRENCIES`, `STRIPE_TWO_DECIMAL_COMPAT_CURRENCIES`, `STRIPE_THREE_DECIMAL_CURRENCIES`, `formatStripeMinorAmount` | formatter | formatted amount | todo | Zero-, two- and three-decimal currency sets and formatStripeMinorAmount; P44.2: replace with the shared minor-unit rule. |
| `packages/views/settings/components/billing-format.ts` | `money-format` | `formatStripeMinorAmount` | formatter | formatted amount | todo | formatStripeMinorAmount definition. |
| `packages/views/settings/components/billing-format.ts` | `vnd` | `STRIPE_ZERO_DECIMAL_CURRENCIES` | literal | code | todo | "VND" in the zero-decimal set; P44.2: minor-unit exponent comes from the rule set. |
| `packages/views/settings/components/billing-tab.tsx` | `currency` | `BillingTabContent` | field | code | todo | Compares preview and request currency. |
| `packages/views/settings/components/billing-tab.tsx` | `money-format` | `file-level comment`, `BillingTabContent` | formatter | formatted amount | todo | Calls formatStripeMinorAmount. |
| `packages/views/settings/components/members-tab.tsx` | `currency` | `MembersTab` | field | code | todo | Preview currency passed to the formatter. |
| `packages/views/settings/components/members-tab.tsx` | `money-format` | `file-level comment`, `MembersTab` | formatter | formatted amount | todo | Calls formatStripeMinorAmount. |
| `packages/views/settings/components/seat-invite-purchase.ts` | `currency` | `seatPurchaseMatchesPreview` | field | code | todo | Compares response and preview currency. |
| `platform-management/console/src/api/hooks.ts` | `currency` | `PutPlanInput.currency`, `usePutPlan` | field | code | todo | PutPlan input field. |
| `platform-management/console/src/api/schemas.ts` | `currency` | `OrderSchema`, `PlanSchema` | field | code | todo | Order and plan schema field. |
| `platform-management/console/src/screens/Orders.tsx` | `currency` | `Orders` | formatter | formatted amount | todo | formatCurrency prints amount and upper-case code; P44.2: use the shared formatter. |
| `platform-management/console/src/screens/Plans.tsx` | `currency` | `Plans` | literal | code | todo | Form state defaults to "vnd"; P44.2: no default code. |
| `platform-management/console/src/screens/Plans.tsx` | `vnd` | `Plans` | literal | code | todo | Default "vnd" and placeholder "e.g. vnd, usd"; P44.2: no default code. |
| `platform-management/console/src/screens/PriceSchedule.tsx` | `currency` | `formatSeatValue`, `formatPriceSnapshot`, `PriceSchedule` | formatter | formatted amount | todo | formatSeatValue and formatPriceSnapshot print the code in upper case. |
| `platform-management/console/src/screens/TenantDetail.tsx` | `currency` | `TenantDetail` | formatter | formatted amount | todo | Inline toLocaleString plus upper-case code for orders. |

### JSON

| File | Term | Function or field | Kind | Holds | Status | Note |
| --- | --- | --- | --- | --- | --- | --- |
| `packages/views/locales/en/runtimes.json` | `usd` | `unit_hint` | text | UI copy | todo | Unit hint for token rates; no change needed. |
| `packages/views/locales/es/runtimes.json` | `usd` | `unit_hint` | text | UI copy | todo | Unit hint for token rates; no change needed. |
| `packages/views/locales/fr/runtimes.json` | `usd` | `unit_hint` | text | UI copy | todo | Unit hint for token rates; no change needed. |
| `packages/views/locales/ja/runtimes.json` | `usd` | `unit_hint` | text | UI copy | todo | Unit hint for token rates; no change needed. |
| `packages/views/locales/ko/runtimes.json` | `usd` | `unit_hint` | text | UI copy | todo | Unit hint for token rates; no change needed. |
| `platform-management/internal/plan/catalogue.json` | `currency` | `currency` | literal | code | todo | Catalogue seeds each plan with "vnd"; stays per-plan data, P44.2: validate against the rule set. |
| `platform-management/internal/plan/catalogue.json` | `vnd` | `currency` | literal | code | todo | Seed value "vnd" for the catalogue plans; stays per-plan data, P44.2: validate against the rule set. |

## Count

- Scanned units: 151. Rows: 151.
- Rows by Kind: field 53, literal 28, formatter 20, text 50.
- Rows by Status: todo 151, ruled 0.

Produced by running the test against an empty table and counting the failure lines:
`cd server && go test -count=1 ./internal/currencyinventory -run TestEveryCurrencyUseIsListed 2>&1 | grep -c 'not listed'`.
The count was 151 at P44.1 before the tables above existed. After the tables the same command prints 0.

## Not yet in the currency rule set

The rule set (`platform-management/internal/money`, P44.2) does not exist yet. These 48 rows of Kind
`literal` or `formatter` hold a hard-coded code or format an amount, so P44.2 has to move them onto the rule set:

- `packages/core/api/schemas.ts` `usd` (literal)
- `packages/core/squads/company-template.ts` `usd` (literal)
- `packages/ui/components/ui/number-flow.tsx` `currency` (formatter)
- `packages/views/billing/billing-test-page.tsx` `currency` (formatter)
- `packages/views/billing/billing-test-page.tsx` `money-format` (formatter)
- `packages/views/billing/billing-test-page.tsx` `usd` (literal)
- `packages/views/billing/order-page.tsx` `money-format` (formatter)
- `packages/views/billing/order-page.tsx` `vnd` (literal)
- `packages/views/billing/plan-change-dialog.tsx` `money-format` (formatter)
- `packages/views/billing/usage-card.tsx` `currency` (formatter)
- `packages/views/billing/usage-card.tsx` `money-format` (formatter)
- `packages/views/company/components/company-insights-parts.tsx` `currency` (formatter)
- `packages/views/company/components/company-insights-parts.tsx` `usd` (literal)
- `packages/views/company/components/company-starter-wizard.tsx` `usd` (formatter)
- `packages/views/company/lib/company-insights.ts` `usd` (literal)
- `packages/views/dashboard/components/dashboard-page.tsx` `currency` (formatter)
- `packages/views/governance/governance-budgets-tab.tsx` `currency` (formatter)
- `packages/views/governance/governance-budgets-tab.tsx` `usd` (literal)
- `packages/views/runtimes/components/runtime-list.tsx` `currency` (formatter)
- `packages/views/runtimes/components/usage-section.tsx` `currency` (formatter)
- `packages/views/runtimes/utils.ts` `usd` (literal)
- `packages/views/settings/components/billing-format.ts` `currency` (formatter)
- `packages/views/settings/components/billing-format.ts` `money-format` (formatter)
- `packages/views/settings/components/billing-format.ts` `vnd` (literal)
- `packages/views/settings/components/billing-tab.tsx` `money-format` (formatter)
- `packages/views/settings/components/members-tab.tsx` `money-format` (formatter)
- `platform-management/cmd/platform-server/main.go` `currency` (literal)
- `platform-management/cmd/platform-server/main.go` `vnd` (literal)
- `platform-management/console/src/screens/Orders.tsx` `currency` (formatter)
- `platform-management/console/src/screens/Plans.tsx` `currency` (literal)
- `platform-management/console/src/screens/Plans.tsx` `vnd` (literal)
- `platform-management/console/src/screens/PriceSchedule.tsx` `currency` (formatter)
- `platform-management/console/src/screens/TenantDetail.tsx` `currency` (formatter)
- `platform-management/internal/billing/registry.go` `vnd` (literal)
- `platform-management/internal/billing/sepay.go` `currency` (literal)
- `platform-management/internal/billing/sepay.go` `vnd` (literal)
- `platform-management/internal/billing/usage.go` `currency` (literal)
- `platform-management/internal/billing/usage.go` `vnd` (literal)
- `platform-management/internal/config/config.go` `currency` (literal)
- `platform-management/internal/config/config.go` `vnd` (literal)
- `platform-management/internal/plan/catalogue.json` `currency` (literal)
- `platform-management/internal/plan/catalogue.json` `vnd` (literal)
- `platform-management/internal/usage/invoicing.go` `currency` (literal)
- `platform-management/internal/usage/invoicing.go` `vnd` (literal)
- `platform-management/internal/usage/notice.go` `vnd` (literal)
- `server/internal/service/metrics.go` `usd` (literal)
- `server/pkg/agent/reasonix.go` `usd` (literal)

Rows of Kind `field` carry a code from the plan, order or provider and stay `todo` until the rule set
validates them. Rows of Kind `text` need no change; the ones that state "VND" in a comment should be reworded when P44.2 lands.

## Migration check

No migration needed at P44. Every table that holds a billing amount either has a `currency` column or reads
the currency from the plan:

- `plan` (`670_plan.up.sql`), `billing_order` (`677_billing_order.up.sql`), `payment_notice`
  (`1007_payment_notice.up.sql`) and `einvoice_issuance` (`1011_einvoice_issuance.up.sql`) have a `currency` column.
- `usage_charge` (`unit_price`, `net_amount`), `workspace_subscription` (`unit_price`) and `plan_price_change` (`old` and `new` price snapshots)
  have no currency column. They are keyed by `plan_key`, and the currency is `plan.currency` (`GetUsageTerms` in
  `platform-management/queries/usage_charge.sql` reads it that way). The console refuses to change a plan currency once
  subscriptions or orders exist (`plan_currency_locked`), so the join stays correct.
- The budget tables (`budget`, `budget_period`, `budget_hold`, `knowledge_link.amount_micros`) hold agent spend in
  micros, not billed money, and are out of scope for the billing currency rule.
