# R4 currency inventory (P44.1)

Every place in the product code that names a currency: a currency code, a currency symbol,
a currency field or a money formatter. Spec: `platform-management-spec.md` section 7.4, K25.
The test `server/internal/currencyinventory/inventory_test.go` reads this file.

Status values:

- `todo`: not yet in the currency rule set. The Note says what is still needed and which P44 step owns it.
- `ruled`: reads the currency from the plan or subscription through the rule set and holds no hard-coded code.
- `exempt`: the matched term is not a platform billing currency (for example AI token cost in USD, UI copy, locale strings, HTTP media types, or the substring "vnd"/"usd" in unrelated identifiers). A reason is required in the Note column.

How the test reads it: it scans the non-test source under `server`, `platform-management`
and `packages` (no database), builds one unit per (file, term), and fails when a unit has no
row below (`not listed: <file> <term>`), when a row matches no unit (`stale row: <file> <term>`),
when a Status is not `todo`, `ruled` or `exempt`, when Holds or Function or field is empty, or when two rows
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

### SQL (migrations and queries)

| File | Term | Function or field | Kind | Holds | Status | Note |
| --- | --- | --- | --- | --- | --- | --- |
| `platform-management/queries/billing.sql` | `currency` | `CreateBillingOrder` | field | code | ruled | CreateBillingOrder writes the draft currency the caller passes; callers take it from the plan (billingapi.go:483, billingapi.go:689) or the change order. No literal in the query. |
| `platform-management/queries/einvoice.sql` | `currency` | `InsertEinvoiceIssuance`, `ListPaidOrdersNeedingEinvoice` | field | code | ruled | InsertEinvoiceIssuance writes the order currency and ListPaidOrdersNeedingEinvoice reads o.currency from the order. No literal. |
| `platform-management/queries/payment_notice.sql` | `currency` | `InsertPaymentNotice` | field | code | todo | Stores the provider's notice currency as received; no check against PLATFORM_CURRENCIES on receipt. matchNotice (reconcile_match.go:29) compares it with the order only. P44.2b slice (notice check): validate it against the rule set when the notice arrives. |
| `platform-management/queries/plan.sql` | `currency` | `InsertPlanIfAbsent`, `UpsertPlan`, `PlanHasSubscriptionsOrOrders` | field | code | todo | Seed insert and upsert write plan.currency without a check against PLATFORM_CURRENCIES; the seeded "vnd" in catalogue.json is never validated. P44.2b slice (plan seed): validate the seeded currency against the enabled set before insert. The lock query stays. |
| `platform-management/queries/usage_charge.sql` | `currency` | `GetUsageTerms` | field | code | ruled | GetUsageTerms reads plan.currency for the workspace's plan; no literal in the query. |
| `server/migrations/1007_payment_notice.up.sql` | `currency` | `payment_notice` | field | code | todo | Stores the provider's notice currency as received; no check against PLATFORM_CURRENCIES on receipt. matchNotice (reconcile_match.go:29) compares it with the order only. P44.2b slice (notice check): validate it against the rule set when the notice arrives. |
| `server/migrations/1011_einvoice_issuance.up.sql` | `currency` | `einvoice_issuance` | field | code | ruled | Column holds the order currency copied at Enqueue; DEFAULT '' is empty, not a code, and InsertEinvoiceIssuance always passes a value. Applied migration, not edited. |
| `server/migrations/213_task_usage_authoritative_cost.up.sql` | `usd` | `file-level comment` | text | comment | exempt | Comment on provider-reported AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/migrations/608_task_usage_credits.up.sql` | `usd` | `file-level comment` | text | comment | exempt | Comment: provider AI credits have no USD rate; agent cost, not platform billing. |
| `server/migrations/670_plan.up.sql` | `currency` | `plan` | field | code | ruled | NOT NULL column that holds plan.currency; no default and no literal. The enabled-set check is plan.ValidateCurrency at save. |
| `server/migrations/677_billing_order.up.sql` | `currency` | `billing_order` | field | code | ruled | NOT NULL column that holds the order currency; no default and no literal. Orders take it from the plan. |
| `server/pkg/db/queries/digest.sql` | `usd` | `DigestWorkspaceUsage` | text | comment | exempt | Comment on provider-reported AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/pkg/db/queries/metric.sql` | `usd` | `BuiltinRunStats` | text | comment | exempt | Comment on provider-reported AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/pkg/db/queries/scorecard.sql` | `usd` | `ScorecardRunCost` | text | comment | exempt | Comment on provider-reported AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/pkg/db/queries/task_usage.sql` | `usd` | `UpsertTaskUsage` | text | comment | exempt | Comment on the provider's AI price in 1e-10 USD; agent cost, not platform billing. |

### Go

| File | Term | Function or field | Kind | Holds | Status | Note |
| --- | --- | --- | --- | --- | --- | --- |
| `platform-management/cmd/platform-server/main.go` | `currency` | `run` | field | code | ruled | The usage provider lookup and the e-invoice VAT rate take money.Default (ForCurrency, VATRateOf); no literal. |
| `platform-management/internal/accountrequest/finalize.go` | `currency` | `writeGrant`, `orderDetail` | field | code | ruled | Lower-cases the plan currency; an empty one falls back to money.Default (P44.2). |
| `platform-management/internal/billing/changeorder.go` | `currency` | `CreateChangeOrder`, `RenewalDraft` | field | code | ruled | Change order and renewal drafts take Currency from the target or current plan; no literal. |
| `platform-management/internal/billing/order.go` | `currency` | `Order.Currency` | field | code | ruled | Order.Currency declaration only; orders take the currency from the plan (billingapi.go:483) or the change order. The usage.go literal is its own row. |
| `platform-management/internal/billing/pgx.go` | `currency` | `orderFromRow`, `InsertOrder` | field | code | todo | Stores the provider's notice currency as received; no check against PLATFORM_CURRENCIES on receipt. matchNotice (reconcile_match.go:29) compares it with the order only. P44.2b slice (notice check): validate it against the rule set when the notice arrives. |
| `platform-management/internal/billing/pricing.go` | `currency` | `daysCeil` | text | comment | ruled | Comment says amounts round to whole minor units of the plan currency. |
| `platform-management/internal/billing/provider.go` | `currency` | `Instructions.Currency`, `file-level comment`, `Notification.Currency`, `CreatePaymentRequest` | field | code | todo | Stores the provider's notice currency as received; no check against PLATFORM_CURRENCIES on receipt. matchNotice (reconcile_match.go:29) compares it with the order only. P44.2b slice (notice check): validate it against the rule set when the notice arrives. |
| `platform-management/internal/billing/rate.go` | `currency` | `file-level comment` | text | comment | ruled | Package comment says money is integer minor units of the plan currency. |
| `platform-management/internal/money/exponent.go` | `currency` | `Exponent` | field | code | ruled | The rule set itself: the exponent table of enabled currencies (P44.2). |
| `platform-management/internal/money/exponent.go` | `vnd` | `exponents` | literal | code | ruled | The rule set itself: the one place the vnd exponent (0) lives (P44.2). |
| `platform-management/internal/money/money.go` | `currency` | `Default` | field | code | ruled | The rule set itself: the package doc and the default currency constant (P44.2). |
| `platform-management/internal/money/money.go` | `vnd` | `Default` | literal | code | ruled | The rule set itself: Default = "vnd", the one place the default literal lives (P44.2). |
| `platform-management/internal/billing/reconcile.go` | `currency` | `noticeEntry` | field | code | todo | Stores the provider's notice currency as received; no check against PLATFORM_CURRENCIES on receipt. matchNotice (reconcile_match.go:29) compares it with the order only. P44.2b slice (notice check): validate it against the rule set when the notice arrives. |
| `platform-management/internal/billing/reconcile_match.go` | `currency` | `matchNotice` | field | code | todo | Stores the provider's notice currency as received; no check against PLATFORM_CURRENCIES on receipt. matchNotice (reconcile_match.go:29) compares it with the order only. P44.2b slice (notice check): validate it against the rule set when the notice arrives. |
| `platform-management/internal/billing/registry.go` | `currency` | `file-level comment`, `ErrNoProvider`, `NewRegistry`, `ForCurrency` | field | code | ruled | Registry maps a currency to a provider; CurrencyGate looks up the draft currency (line 84) and refuses with ErrNoProvider. No literal on these lines. |
| `platform-management/internal/billing/sepay.go` | `currency` | `SePay`, `ParseNotification` | field | code | ruled | SePay.currency is set by the registry from the currency mapped to the provider (default money.Default); ParseNotification copies it into every notice. No literal. |
| `platform-management/internal/billing/service.go` | `currency` | `currencyPattern`, `Draft.Currency`, `validate`, `orderDetail` | field | code | ruled | Draft.Currency comes from the plan or order; validate checks the three-letter shape (line 206) and holds no code. Enabled set: plan.ValidateCurrency at plan save, CurrencyGate at order creation. |
| `platform-management/internal/billing/usage.go` | `currency` | `ActivateUsage` | field | code | ruled | ActivateUsage writes UsageActivation.Currency, which checkoutUsage takes from the plan; an empty one falls back to money.Default. No literal. |
| `platform-management/internal/billing/vat.go` | `currency` | `file-level comment` | text | comment | ruled | Comment says VAT is in whole minor units of the order currency. |
| `platform-management/internal/billing/webhook.go` | `currency` | `serve` | field | code | todo | Stores the provider's notice currency as received; no check against PLATFORM_CURRENCIES on receipt. matchNotice (reconcile_match.go:29) compares it with the order only. P44.2b slice (notice check): validate it against the rule set when the notice arrives. |
| `platform-management/internal/billingapi/billingapi.go` | `currency` | `checkout`, `previewResponse.Currency`, `preview`, `purchaseRequest.Currency`, `purchaseResponse.Currency`, `purchase` | field | code | ruled | Order and response currency come from the plan (lines 483, 622, 669); the purchase request currency is compared with the plan currency (line 670). No literal on these lines. |
| `platform-management/internal/billingapi/planchange.go` | `currency` | `planChangePreviewResponse.Currency`, `planChangeOrderResponse.Currency`, `previewPlanChange`, `writeChangeOrder` | field | code | ruled | Plan-change preview and order responses take Currency from the target plan or the order; the paid-plan check compares with the current plan. |
| `platform-management/internal/config/config.go` | `currency` | `Error`, `EmptyBankVariables`, `loadBilling`, `loadPayments` | field | code | ruled | Uses money.Default and money.Exponent; the comments say minor units of the plan currency. No literal. |
| `platform-management/internal/console/einvoices.go` | `currency` | `ListEinvoices`, `toEinvoiceJSON` | field | code | ruled | List row and JSON currency come from the issuance row (lines 78, 103, 232); no literal. File has uncommitted edits from another session; classified as in the working tree. |
| `platform-management/internal/console/orders.go` | `currency` | `orderJSON.Currency`, `toOrderJSON`, `cancelOrder` | field | code | ruled | Order JSON and the cancel audit detail take Currency from the order row; no literal. |
| `platform-management/internal/console/payment_notices.go` | `currency` | `listEinvoices`, `noticeColumns` | field | code | ruled | List columns and the candidate lookup use the stored notice and order currency (lines 59, 73, 133); no literal. |
| `platform-management/internal/console/pgx.go` | `currency` | `orderFromRow` | field | code | ruled | orderFromRow maps the stored order currency; no literal. |
| `platform-management/internal/console/read.go` | `currency` | `overview` | field | code | ruled | Overview JSON groups the pending orders by their own order currency (pending_by_currency); no literal and no cross-currency sum. |
| `platform-management/internal/console/store.go` | `currency` | `Overview.PendingByCurrency`, `CurrencyTotal` | field | code | ruled | Overview and CurrencyTotal hold the pending order sums keyed by order currency; no literal. |
| `platform-management/internal/db/models.go` | `currency` | `BillingOrder.Currency`, `EinvoiceIssuance.Currency`, `PaymentNotice.Currency`, `Plan.Currency` | field | code | ruled | sqlc model fields generated from the SQL (lines 394, 935, 1986, 2036); no literal. |
| `platform-management/internal/db/models.go` | `usd` | `file-level comment` | text | comment | exempt | Comment on provider-reported AI cost in 1e-10 USD; agent cost, not platform billing. |
| `platform-management/internal/invoice/pgx.go` | `currency` | `insert`, `ReplaceIssued`, `fromRow` | field | code | ruled | Insert and row mapping copy the order currency (lines 137, 318, 380); no literal. |
| `platform-management/internal/invoice/provider.go` | `currency` | `IssueRequest.Currency` | field | code | ruled | IssueRequest.Currency is filled from the record, which is filled from the paid order; no literal. |
| `platform-management/internal/einvoice/sepay.go` | `currency` | `createBody` | field | code | ruled | Copies the issuance request currency, which comes from the paid order (invoice/service.go:57), into the SePay body in upper case; no literal. |
| `platform-management/internal/invoice/record.go` | `currency` | `Record.Currency`, `NewRecord.Currency` | field | code | ruled | Record and NewRecord fields filled from the order at Enqueue; no literal. |
| `platform-management/internal/invoice/service.go` | `currency` | `Enqueue`, `process` | field | code | ruled | Issuance record and request take Currency from the paid order and the record; no literal. |
| `platform-management/internal/invoice/store.go` | `currency` | `PaidOrder` | field | code | ruled | PaidOrder.Currency is read from the order row (einvoice.sql ListPaidOrdersNeedingEinvoice); no literal. |
| `platform-management/internal/lifecycle/expire.go` | `currency` | `Run` | field | code | ruled | Expiry audit detail takes the order currency; no literal. |
| `platform-management/internal/lifecycle/pgx.go` | `currency` | `orderFromRow` | field | code | ruled | orderFromRow maps the stored order currency; no literal. |
| `platform-management/internal/paymentnotice/notice.go` | `currency` | `Notice.Currency`, `NewNotice.Currency` | field | code | todo | Stores the provider's notice currency as received; no check against PLATFORM_CURRENCIES on receipt. matchNotice (reconcile_match.go:29) compares it with the order only. P44.2b slice (notice check): validate it against the rule set when the notice arrives. |
| `platform-management/internal/paymentnotice/pgx.go` | `currency` | `Insert`, `fromRow` | field | code | todo | Stores the provider's notice currency as received; no check against PLATFORM_CURRENCIES on receipt. matchNotice (reconcile_match.go:29) compares it with the order only. P44.2b slice (notice check): validate it against the rule set when the notice arrives. |
| `platform-management/internal/plan/pgx.go` | `currency` | `InsertIfAbsent`, `FromRow`, `insertParams.Currency`, `toInsertParams` | field | code | todo | Seed insert path (InsertIfAbsent) writes the catalogue currency without a check against PLATFORM_CURRENCIES. P44.2b slice (plan seed): validate it before insert. |
| `platform-management/internal/plan/currency.go` | `currency` | `ValidateCurrency` | field | code | ruled | The enabled-set check on plan save: a currency outside PLATFORM_CURRENCIES is refused (P44.2b slice 3a). |
| `platform-management/internal/plan/plan.go` | `currency` | `IsUnlimited`, `Plan.Currency`, `Catalogue`, `Validate` | field | code | todo | Plan.Currency is checked by pattern only; Catalogue seeds "vnd" unchecked. Save-time check is plan.ValidateCurrency (subscription/console.go:529). P44.2b slice (plan seed): validate the catalogue currency against the enabled set. |
| `platform-management/internal/subscription/api.go` | `currency` | `priceJSON.Currency`, `listedPlanJSON.Currency`, `buildPrices` | field | code | ruled | Price and listed-plan JSON take Currency from the plan; no literal. |
| `platform-management/internal/subscription/console.go` | `currency` | `putPlanBody.Currency`, `putPlan` | field | code | ruled | Plan save keeps the stored currency when the body omits it, refuses a change while in use, and checks the code with plan.ValidateCurrency against PLATFORM_CURRENCIES. |
| `platform-management/internal/subscription/quote.go` | `currency` | `conflict` | text | comment | ruled | Comment says proration rounds to whole minor units of the order currency. |
| `platform-management/internal/subscription/usage.go` | `currency` | `Usage.Currency` | field | code | ruled | Usage.Currency declaration; usage/view.go fills it from the plan. No literal. |
| `platform-management/internal/usage/invoicing.go` | `currency` | `invoice`, `invoiceDraft` | field | code | ruled | Usage invoice draft takes the plan currency from GetUsageTerms (P44.3). The "vnd" literal row is removed with its last literal. |
| `platform-management/internal/usage/notice.go` | `currency` | `noticeEighty`, `noticeText` | field | amount with code in message text | ruled | noticeText prints the plan currency (from GetUsageTerms) beside both amounts; no literal. |
| `platform-management/internal/usage/view.go` | `currency` | `UsageInput.Currency`, `BuildUsage`, `Usage` | field | code | ruled | Usage block takes Currency from the plan (p.Currency); no literal. |
| `server/cmd/multica/cmd_company_usage.go` | `usd` | `file-level comment`, `runCompanyUsage` | text | UI label | exempt | CLI heading 'COST (USD TICKS)' for provider-reported AI cost; not platform billing. |
| `server/internal/cli/update.go` | `vnd` | `fetchReleaseByTag`, `FetchLatestRelease` | text | media type | exempt | HTTP Accept media type application/vnd.github+json; not a currency. |
| `server/internal/daemon/types.go` | `usd` | `file-level comment` | text | comment | exempt | Comment on the provider's AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/internal/handler/agent.go` | `usd` | `attributionsOf` | text | comment | exempt | Comment on the provider's AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/internal/handler/billing_orders.go` | `currency` | `workspacePlanJSON.Currency`, `billingBankDetails`, `workspaceOrderJSON.Currency`, `toWorkspaceOrderJSON`, `ListWorkspacePlans` | field | code | ruled | Plan JSON and order JSON take Currency from the plan and the order; no literal. |
| `server/internal/handler/billing_orders.go` | `vnd` | `file-level comment`, `billingBankDetails` | text | comment | todo | Comments at lines 72 and 144 name "VND" as the usage price unit and say VND has no decimals. P44.2b slice (comment reword). |
| `server/internal/handler/cloud_billing.go` | `currency` | `cloudSubscriptionSeatPurchaseRequest.Currency`, `GetCloudWorkspaceSubscriptionUsage`, `PurchaseCloudWorkspaceSubscriptionSeats`, `isASCIICurrency` | field | code | todo | Seat purchase request currency is lower-cased and shape-checked only (isASCIICurrency). Not checked against the plan or PLATFORM_CURRENCIES. P44.2b slice (seat purchase check): compare it with the plan currency. |
| `server/internal/handler/daemon.go` | `usd` | `formatLegacyCommentEntry` | text | comment | exempt | Comment on the provider's AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/internal/handler/dashboard.go` | `usd` | `parseProjectIDParam`, `listDashboardUsageDaily` | text | comment | exempt | Comment on the AI cost aggregate in 1e-10 USD; agent cost, not platform billing. |
| `server/internal/handler/github.go` | `vnd` | `fetchInstallationAccount`, `setGitHubAPIHeaders` | text | media type | exempt | HTTP Accept media type application/vnd.github+json; not a currency. |
| `server/internal/handler/proposal.go` | `currency` | `relatedToResponse` | text | comment | exempt | Comment on a proposal's cost estimate in micros, an agent spend limit (spec 4.4); not platform billing. |
| `server/internal/handler/runtime.go` | `usd` | `runtimeToResponse`, `GetRuntimeTaskActivity`, `GetRuntimeUsageByAgent` | text | comment | exempt | Comment on the AI cost aggregate in 1e-10 USD; agent cost, not platform billing. |
| `server/internal/handler/skill.go` | `vnd` | `githubRefExists` | text | media type | exempt | HTTP Accept media type application/vnd.github.v3.sha; not a currency. |
| `server/internal/integrations/ghsnapshot/client.go` | `vnd` | `mintInstallationToken`, `restGet` | text | media type | exempt | HTTP Accept media type application/vnd.github+json; not a currency. |
| `server/internal/integrations/lark/outbound_media.go` | `vnd` | `isImageAttachment` | text | media type | exempt | MIME type image/vnd.microsoft.icon; not a currency. |
| `server/internal/metrics/business.go` | `usd` | `NewBusinessMetrics`, `ObserveChatClaimRolloutMissingQuery` | text | metric help text and comment | exempt | Help text and comment for the AI token cost metric; agent cost, not platform billing. |
| `server/internal/metrics/pricing.go` | `usd` | `file-level comment` | text | comment | exempt | Comments on the AI token price sheet; agent cost, not platform billing. |
| `server/internal/salescollect/probe.go` | `usd` | `file-level comment` | text | UI copy | exempt | UI price hint for a third-party data source; not platform billing. |
| `server/internal/salescollect/recipe.go` | `usd` | `Collectable` | text | comment and error text | exempt | Error text: a paid third-party provider's monthly budget (spec 4.12) is 0 USD until an owner sets it; agent budget, not billing. |
| `server/internal/service/budget_accounting.go` | `usd` | `file-level comment` | text | comment | exempt | Comment on the provider's AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/internal/service/digest.go` | `usd` | `computeWorkspace` | text | comment | exempt | Comment on the provider's AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/internal/service/knowledge_sales_kinds.go` | `currency` | `urlField` | field | code (free text) | exempt | Free-text currency field of the sales knowledge kind; user text, not billed money. |
| `server/internal/service/knowledge_sales_kinds.go` | `usd` | `urlField` | text | UI label | exempt | Field label 'Cost, 30 days (USD)' for a sales data source; not platform billing. |
| `server/internal/service/metrics.go` | `usd` | `file-level comment` | literal | unit label "USD" | exempt | Metric catalogue unit USD for provider-reported AI run cost; not platform billing. |
| `server/internal/service/metrics_builtin.go` | `usd` | `file-level comment` | text | comment | exempt | Comment on the AI cost tick conversion; agent cost, not platform billing. |
| `server/internal/service/scorecard_metrics.go` | `usd` | `file-level comment` | text | comment | exempt | Comment on the provider's AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/internal/service/task.go` | `usd` | `captureTaskCancelled` | text | comment | exempt | Comment on the provider's AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/pkg/agent/agent.go` | `usd` | `runContext` | text | comment | exempt | Comment on the provider's AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/pkg/agent/reasonix.go` | `currency` | `observe`, `isReasonixUSDCurrency` | field | code | exempt | CLI-reported AI turn currency; only USD is parsed. Not platform billing. |
| `server/pkg/agent/reasonix.go` | `usd` | `isReasonixUSDCurrency` | literal | code | exempt | Accepts USD, $ and US$ for the CLI's AI turn cost; not platform billing. |

### TypeScript and TSX

| File | Term | Function or field | Kind | Holds | Status | Note |
| --- | --- | --- | --- | --- | --- | --- |
| `packages/core/api/client.ts` | `currency` | `purchaseWorkspaceSeats` | field | code | ruled | purchaseWorkspaceSeats sends the preview currency (line 3445); no literal. |
| `packages/core/api/schemas.ts` | `currency` | `BillingTopupSchema`, `BillingCheckoutSessionStatusSchema`, `EMPTY_BILLING_CHECKOUT_SESSION_STATUS`, `WorkspaceUsagePeriodSchema`, `WorkspaceSubscriptionPriceSchema`, `WorkspaceSeatPurchasePreviewSchema`, `PurchaseWorkspaceSeatsResponseSchema`, `WorkspacePlanChangePreviewSchema`, `OrderPlanChangeResponseSchema` | field | code | todo | The "usd" defaults are removed (empty-string default, no code). Field rows that carry a code from the plan stay todo until the rule set validates them (round C). |
| `packages/core/api/schemas.ts` | `usd` | `file-level comment` | text | comment | exempt | Comment on the provider-reported AI cost in 1e-10 USD (`cost_usd_ticks`); agent cost, not platform billing. The "usd" currency defaults are removed. |
| `packages/core/billing/money.ts` | `cad` | `CURRENCY_EXPONENTS` | literal | code | ruled | Exponent table entry (cad=2). The table is the one place that names the enabled currencies; it holds exponents, not a default currency. |
| `packages/core/billing/money.ts` | `currency` | `formatMinorAmount` | formatter | formatted amount | ruled | formatMinorAmount formats minor units with the table digits; an unknown code uses 2 digits and prints the code. |
| `packages/core/billing/money.ts` | `vnd` | `CURRENCY_EXPONENTS` | literal | code | ruled | Exponent table entry (vnd=0). |
| `packages/core/billing/workspace-orders-schemas.ts` | `currency` | `WorkspacePlanSchema`, `WorkspaceOrderSchema` | field | code | ruled | Plan and order schema fields use z.string().min(1) (lines 39, 132) and copy the value; no literal. |
| `packages/core/billing/workspace-orders-types.ts` | `currency` | `file-level comment`, `WorkspacePlan.currency`, `WorkspaceOrder.currency` | field | code | ruled | Type fields are clean and the file comment now says amounts are minor units of the order currency. |
| `packages/core/squads/company-template.ts` | `usd` | `DEFAULT_RUN_COST_USD`, `CompanyEstimate`, `estimateCompanyRuns` | literal | USD amount (0.5 per run) | exempt | Estimated AI run cost (DEFAULT_RUN_COST_USD, 0.5 per run) for the company wizard; not a platform charge. |
| `packages/core/squads/kitviet-previous-texts.ts` | `usd` | `KITVIET_PREVIOUS_AGENT_INSTRUCTIONS` | text | prompt text | exempt | Frozen agent prompt text that mentions USD; not code and not billing. |
| `packages/core/squads/sales-starter-sources.ts` | `usd` | `SALES_STARTER_SOURCES` | text | UI copy | exempt | UI copy price hints for third-party data sources; not platform billing. |
| `packages/core/types/agent.ts` | `usd` | `AgentTask`, `IssueUsageSummary` | text | comment | exempt | Comment on the provider's AI cost in 1e-10 USD; agent cost, not platform billing. |
| `packages/core/types/billing.ts` | `currency` | `BillingTopup.currency`, `BillingCheckoutSessionStatus.currency`, `WorkspaceUsagePeriod.currency`, `WorkspaceSubscriptionPrice.currency`, `WorkspaceSubscriptionPrice`, `WorkspaceSeatPurchasePreview.currency`, `WorkspaceSeatPurchasePreview`, `PurchaseWorkspaceSeatsRequest.currency`, `PurchaseWorkspaceSeatsResponse.currency`, `WorkspacePlanChangePreview.currency`, `CreateWorkspacePlanChangeResponse.currency` | field | code | todo | Type fields are clean, but the comments at lines 324, 360 and 362 give "$20.00" and "Stripe's minor currency unit" as the unit. P44.2b slice (comment reword). |
| `packages/core/types/billing.ts` | `usd` | `file-level comment` | text | comment | exempt | Line 9 comment defines the AI credit display unit (1 USD = 1000 credit); credits are agent spend, not the platform billing currency, and no code reads the comment. |
| `packages/ui/components/ui/number-flow.tsx` | `currency` | `CurrencyNumberFlowProps`, `CurrencyNumberFlow`, `CappedNumberFlow` | formatter | formatted amount | exempt | CurrencyNumberFlow defaults the prefix to "$" (line 39). Its only users are dashboard-page, runtime-list and usage-section, which show AI token cost in USD; no billing view imports it. |
| `packages/views/billing/billing-test-page.tsx` | `currency` | `CheckoutSessionStatusBanner`, `TopupRow`, `formatMoney` | formatter | formatted amount | ruled | formatMoney delegates to formatMinorAmount; the currency comes from the checkout status, top-up row or tier. |
| `packages/views/billing/billing-test-page.tsx` | `money-format` | `CheckoutSessionStatusBanner`, `TierButton`, `TopupRow`, `formatMoney` | formatter | formatted amount | ruled | formatMoney calls pass the currency from the data (tier.currency for tiers); no literal code. |
| `packages/views/billing/order-page.tsx` | `currency` | `BillingOrder.currency`, `normalizeOrder`, `BillingOrderPage` | field | code | ruled | normalizeOrder reads the currency from the order with no code fallback; totals use Formats through formatStripeMinorAmount with the currency read from the data; digits come from the shared exponent table (packages/core/billing/money.ts). |
| `packages/views/billing/order-page.tsx` | `money-format` | `file-level comment`, `BillingOrderPage` | formatter | formatted amount | ruled | Formats through formatStripeMinorAmount with the currency read from the data; digits come from the shared exponent table (packages/core/billing/money.ts). |
| `packages/views/billing/plan-change-dialog.tsx` | `currency` | `PlanChangeDialog` | field | code | ruled | Passes preview.currency. Formats through formatStripeMinorAmount with the currency read from the data; digits come from the shared exponent table (packages/core/billing/money.ts). |
| `packages/views/billing/plan-change-dialog.tsx` | `money-format` | `file-level comment`, `PlanChangeDialog` | formatter | formatted amount | ruled | Formats through formatStripeMinorAmount with the currency read from the data; digits come from the shared exponent table (packages/core/billing/money.ts). |
| `packages/views/billing/scheduled-price-change-banner.tsx` | `currency` | `ScheduledPriceChangeBannerProps.currency`, `ScheduledPriceChangeBanner` | field | code | ruled | Passes the currency prop to formatUsageMoney. Formats through formatStripeMinorAmount with the currency read from the data; digits come from the shared exponent table (packages/core/billing/money.ts). |
| `packages/views/billing/usage-card.tsx` | `currency` | `formatUsageMoney`, `UsageCard` | formatter | formatted amount | ruled | formatUsageMoney wraps formatStripeMinorAmount; the fallback prints the bare amount with no upper-cased code. |
| `packages/views/billing/usage-card.tsx` | `money-format` | `file-level comment`, `formatUsageMoney` | formatter | formatted amount | ruled | Formats through formatStripeMinorAmount with the currency read from the data; digits come from the shared exponent table (packages/core/billing/money.ts). |
| `packages/views/billing/usage-invoice-lines.tsx` | `currency` | `UsageInvoiceLinesProps.currency`, `UsageInvoiceLines` | field | code | ruled | Passes the currency prop to formatUsageMoney. Formats through formatStripeMinorAmount with the currency read from the data; digits come from the shared exponent table (packages/core/billing/money.ts). |
| `packages/views/company/components/company-insights-parts.tsx` | `currency` | `usd`, `usdWhole` | formatter | formatted amount | exempt | Intl formatters fixed to USD for company AI run cost (cost_usd_ticks); not platform billing. |
| `packages/views/company/components/company-insights-parts.tsx` | `usd` | `costChartConfig`, `usd`, `usdWhole` | literal | code | exempt | Hard-coded USD in the Intl formatters and chart label for AI run cost; not platform billing. |
| `packages/views/company/components/company-starter-wizard.tsx` | `usd` | `file-level comment`, `CompanyStarterWizard` | formatter | formatted amount | exempt | Shows the default AI run-cost estimate with formatUsd; not a platform charge. |
| `packages/views/company/lib/company-insights.ts` | `usd` | `MICROS_PER_USD`, `microsToUsd` | literal | conversion constant | exempt | Micros-to-dollars divisor for AI run cost; a conversion constant, not a currency code. |
| `packages/views/dashboard/components/dashboard-page.tsx` | `currency` | `file-level comment`, `DashboardPage` | formatter | formatted amount | exempt | CurrencyNumberFlow shows the AI cost KPI (totals.cost, from the usage aggregate); not platform billing. |
| `packages/views/editor/mermaid-viewer.tsx` | `vnd` | `MermaidViewerContent` | text | media type | exempt | MIME type text/vnd.mermaid; not a currency. |
| `packages/views/governance/governance-budgets-tab.tsx` | `currency` | `file-level comment`, `formatBudgetUsd` | formatter | formatted amount | exempt | Budget display in USD: agent spend budgets, out of scope for the billing currency rule (Migration check). |
| `packages/views/governance/governance-budgets-tab.tsx` | `usd` | `formatBudgetUsd` | literal | code | exempt | Hard-coded USD for agent spend budgets, out of scope for the billing currency rule (Migration check). |
| `packages/views/runtimes/components/runtime-list.tsx` | `currency` | `file-level comment`, `CostCell` | formatter | formatted amount | exempt | CurrencyNumberFlow shows the AI token cost per runtime; not platform billing. |
| `packages/views/runtimes/components/update-section.tsx` | `vnd` | `fetchLatestVersion` | text | media type | exempt | HTTP Accept media type application/vnd.github+json; not a currency. |
| `packages/views/runtimes/components/usage-section.tsx` | `currency` | `file-level comment`, `UsageSection` | formatter | formatted amount | exempt | CurrencyNumberFlow shows the AI token cost KPI; not platform billing. |
| `packages/views/runtimes/utils.ts` | `usd` | `formatCredits`, `hasTokenFigure`, `MODEL_PRICING`, `Priceable`, `COST_USD_TICKS_PER_USD`, `estimateCost`, `estimateCostBreakdown` | literal | USD rate and amount | exempt | AI token price table and tick divisor in USD; agent cost, not platform billing. |
| `packages/views/settings/components/billing-format.ts` | `currency` | `STRIPE_ZERO_DECIMAL_CURRENCIES`, `STRIPE_TWO_DECIMAL_COMPAT_CURRENCIES`, `STRIPE_THREE_DECIMAL_CURRENCIES`, `formatStripeMinorAmount` | formatter | formatted amount | ruled | formatStripeMinorAmount delegates to formatMinorAmount; the three Stripe currency sets are gone. |
| `packages/views/settings/components/billing-format.ts` | `money-format` | `formatStripeMinorAmount` | formatter | formatted amount | ruled | formatStripeMinorAmount keeps its name and delegates to the shared formatter. |
| `packages/views/settings/components/billing-tab.tsx` | `currency` | `BillingTabContent` | field | code | ruled | Prices use the plan or preview currency. Formats through formatStripeMinorAmount with the currency read from the data; digits come from the shared exponent table (packages/core/billing/money.ts). |
| `packages/views/settings/components/billing-tab.tsx` | `money-format` | `file-level comment`, `BillingTabContent` | formatter | formatted amount | ruled | Formats through formatStripeMinorAmount with the currency read from the data; digits come from the shared exponent table (packages/core/billing/money.ts). |
| `packages/views/settings/components/members-tab.tsx` | `currency` | `MembersTab` | field | code | ruled | Passes the preview currency. Formats through formatStripeMinorAmount with the currency read from the data; digits come from the shared exponent table (packages/core/billing/money.ts). |
| `packages/views/settings/components/members-tab.tsx` | `money-format` | `file-level comment`, `MembersTab` | formatter | formatted amount | ruled | Formats through formatStripeMinorAmount with the currency read from the data; digits come from the shared exponent table (packages/core/billing/money.ts). |
| `packages/views/settings/components/seat-invite-purchase.ts` | `currency` | `seatPurchaseMatchesPreview` | field | code | ruled | seatPurchaseMatchesPreview compares the response currency with the preview currency (line 57); no literal or formatter. |
| `platform-management/console/src/api/hooks.ts` | `currency` | `PutPlanInput.currency`, `usePutPlan` | field | code | ruled | PutPlanInput.currency and usePutPlan pass the form value to the console API (lines 656-689); the server checks it with plan.ValidateCurrency. No literal. The form default is the Plans.tsx row. |
| `platform-management/console/src/api/schemas.ts` | `currency` | `OrderSchema`, `PlanSchema` | field | code | ruled | Order and plan schema fields typed as text (lines 218-917); no literal. |
| `platform-management/console/src/money.ts` | `cad` | `CURRENCY_EXPONENTS` | literal | code | ruled | Console copy of the exponent table (cad=2); the console is a separate app. |
| `platform-management/console/src/money.ts` | `currency` | `formatMinorAmount` | formatter | formatted amount | ruled | formatMinorAmount prints the amount with the table digits, then the upper-case code; the amount alone when the code is empty. |
| `platform-management/console/src/money.ts` | `vnd` | `CURRENCY_EXPONENTS` | literal | code | ruled | Console copy of the exponent table (vnd=0). |
| `platform-management/console/src/screens/EinvoiceFailures.tsx` | `currency` | `formatCurrency` | formatter | formatted amount | ruled | Formats through formatMinorAmount (console/src/money.ts) with the currency read from the data; no toLocaleString plus upper-case code. |
| `platform-management/console/src/screens/Orders.tsx` | `currency` | `Orders` | formatter | formatted amount | ruled | Formats through formatMinorAmount (console/src/money.ts) with the currency read from the data; no toLocaleString plus upper-case code. |
| `platform-management/console/src/screens/PaymentNotices.tsx` | `currency` | `formatCurrency` | formatter | formatted amount | ruled | Formats through formatMinorAmount (console/src/money.ts) with the currency read from the data; no toLocaleString plus upper-case code. |
| `platform-management/console/src/screens/Plans.tsx` | `currency` | `Plans` | field | code | ruled | Form starts with an empty code and the list uses formatMinorAmount; the placeholder is the neutral "ISO 4217 code". |
| `platform-management/console/src/screens/PriceSchedule.tsx` | `currency` | `formatSeatValue`, `formatPriceSnapshot`, `PriceSchedule` | formatter | formatted amount | ruled | Formats through formatMinorAmount (console/src/money.ts) with the currency read from the data; no toLocaleString plus upper-case code. |
| `platform-management/console/src/screens/TenantDetail.tsx` | `currency` | `TenantDetail` | formatter | formatted amount | ruled | Formats through formatMinorAmount (console/src/money.ts) with the currency read from the data; no toLocaleString plus upper-case code. |

### JSON

| File | Term | Function or field | Kind | Holds | Status | Note |
| --- | --- | --- | --- | --- | --- | --- |
| `packages/views/locales/en/runtimes.json` | `usd` | `unit_hint` | text | UI copy | exempt | UI copy: unit hint for AI token rates; not platform billing. |
| `packages/views/locales/es/runtimes.json` | `usd` | `unit_hint` | text | UI copy | exempt | Locale string: unit hint for AI token rates; not platform billing. |
| `packages/views/locales/fr/runtimes.json` | `usd` | `unit_hint` | text | UI copy | exempt | Locale string: unit hint for AI token rates; not platform billing. |
| `packages/views/locales/ja/runtimes.json` | `usd` | `unit_hint` | text | UI copy | exempt | Locale string: unit hint for AI token rates; not platform billing. |
| `packages/views/locales/ko/runtimes.json` | `usd` | `unit_hint` | text | UI copy | exempt | Locale string: unit hint for AI token rates; not platform billing. |
| `platform-management/internal/plan/catalogue.json` | `currency` | `currency` | literal | code | todo | Each catalogue plan seeds currency "vnd" (lines 17-142), and Seed does not call plan.ValidateCurrency. P44.2b slice (plan seed): validate the seeded currency against the enabled set. |
| `platform-management/internal/plan/catalogue.json` | `vnd` | `currency` | literal | code | todo | Seed value "vnd" on six plans (lines 17-142). It is per-plan data; P44.2b slice (plan seed): validate it against the enabled set at Seed. |

## Count

- Scanned units: 158. Rows: 158.
- Rows by Kind: field 72, literal 15, formatter 24, text 47.
- Rows by Status: todo 18, ruled 82, exempt 58.

## Not yet in the currency rule set

The rule set (`platform-management/internal/money`) exists. These 2 rows of Kind `literal` or `formatter` still hold a
hard-coded code or format an amount, so P44.2 has to move them onto the rule set:

- `platform-management/internal/plan/catalogue.json` `currency` (literal)
- `platform-management/internal/plan/catalogue.json` `vnd` (literal)

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
