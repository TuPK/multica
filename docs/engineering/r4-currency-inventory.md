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
| `platform-management/queries/billing.sql` | `currency` | `CreateBillingOrder` | field | code | todo | Insert writes the order currency the caller passes; no literal here. P44.2 checks the caller's value against the rule set. |
| `platform-management/queries/einvoice.sql` | `currency` | `InsertEinvoiceIssuance`, `ListPaidOrdersNeedingEinvoice` | field | code | todo | Insert and paid-order read carry the order currency; no literal here. P44.2 checks the issuance path against the rule set. |
| `platform-management/queries/payment_notice.sql` | `currency` | `InsertPaymentNotice` | field | code | todo | Insert writes the provider's notice currency. P44.2 checks it against the rule set. |
| `platform-management/queries/plan.sql` | `currency` | `InsertPlanIfAbsent`, `UpsertPlan`, `PlanHasSubscriptionsOrOrders` | field | code | todo | Plan insert and upsert write plan.currency; the lock query blocks a change while subscriptions or orders exist. P44.2 checks the seed insert path against the rule set. |
| `platform-management/queries/usage_charge.sql` | `currency` | `GetUsageTerms` | field | code | ruled | GetUsageTerms reads plan.currency for the workspace's plan; no literal in the query. |
| `server/migrations/1007_payment_notice.up.sql` | `currency` | `payment_notice` | field | code | todo | Column with DEFAULT ''; nothing reads the default. P44.2 decides whether the empty default stays. |
| `server/migrations/1011_einvoice_issuance.up.sql` | `currency` | `einvoice_issuance` | field | code | todo | Column with DEFAULT ''; nothing reads the default. P44.2 decides whether the empty default stays. |
| `server/migrations/213_task_usage_authoritative_cost.up.sql` | `usd` | `file-level comment` | text | comment | exempt | Comment on provider-reported AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/migrations/608_task_usage_credits.up.sql` | `usd` | `file-level comment` | text | comment | exempt | Comment: provider AI credits have no USD rate; agent cost, not platform billing. |
| `server/migrations/670_plan.up.sql` | `currency` | `plan` | field | code | todo | NOT NULL column that holds plan.currency. P44.2 decides whether the database needs a check beyond the rule set. |
| `server/migrations/677_billing_order.up.sql` | `currency` | `billing_order` | field | code | todo | NOT NULL column that holds the order currency. P44.2 decides whether the database needs a check beyond the rule set. |
| `server/pkg/db/queries/digest.sql` | `usd` | `DigestWorkspaceUsage` | text | comment | exempt | Comment on provider-reported AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/pkg/db/queries/metric.sql` | `usd` | `BuiltinRunStats` | text | comment | exempt | Comment on provider-reported AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/pkg/db/queries/scorecard.sql` | `usd` | `ScorecardRunCost` | text | comment | exempt | Comment on provider-reported AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/pkg/db/queries/task_usage.sql` | `usd` | `UpsertTaskUsage` | text | comment | exempt | Comment on the provider's AI price in 1e-10 USD; agent cost, not platform billing. |

### Go

| File | Term | Function or field | Kind | Holds | Status | Note |
| --- | --- | --- | --- | --- | --- | --- |
| `platform-management/cmd/platform-server/main.go` | `currency` | `run` | literal | code | todo | Boot looks up the provider for the literal 'vnd' (line 218). P44.2: check one provider per enabled currency. |
| `platform-management/cmd/platform-server/main.go` | `vnd` | `run` | literal | code | todo | Literal 'vnd' provider lookup and its comment (lines 216 and 218). P44.2: use the rule set default or each enabled currency. |
| `platform-management/internal/accountrequest/finalize.go` | `currency` | `writeGrant`, `orderDetail` | field | code | ruled | Lower-cases the plan currency; an empty one falls back to money.Default (P44.2). |
| `platform-management/internal/billing/changeorder.go` | `currency` | `CreateChangeOrder`, `RenewalDraft` | field | code | ruled | Change order and renewal drafts take Currency from the target or current plan; no literal. |
| `platform-management/internal/billing/order.go` | `currency` | `Order.Currency` | field | code | todo | Order.Currency field, set from the plan at purchase and from the literal 'vnd' in usage.go. P44.2 removes the literal. |
| `platform-management/internal/billing/pgx.go` | `currency` | `orderFromRow`, `InsertOrder` | field | code | todo | Maps the stored order currency (line 103) and the provider notice currency (line 127). P44.2 checks the notice currency against the rule set. |
| `platform-management/internal/billing/pricing.go` | `vnd` | `daysCeil` | text | comment | todo | Comment: rounds to whole VND; reword when P44.2 lands. |
| `platform-management/internal/billing/provider.go` | `currency` | `Instructions.Currency`, `file-level comment`, `Notification.Currency`, `CreatePaymentRequest` | field | code | todo | Instructions, Notification and CreatePaymentRequest carry the currency the provider sends. P44.2 checks it against the rule set on receipt. |
| `platform-management/internal/billing/rate.go` | `vnd` | `file-level comment` | text | comment | todo | Comment: money is integer VND; reword when P44.2 lands. |
| `platform-management/internal/money/exponent.go` | `currency` | `Exponent` | field | code | ruled | The rule set itself: the exponent table of enabled currencies (P44.2). |
| `platform-management/internal/money/exponent.go` | `vnd` | `exponents` | literal | code | ruled | The rule set itself: the one place the vnd exponent (0) lives (P44.2). |
| `platform-management/internal/money/money.go` | `currency` | `Default` | field | code | ruled | The rule set itself: the package doc and the default currency constant (P44.2). |
| `platform-management/internal/money/money.go` | `vnd` | `Default` | literal | code | ruled | The rule set itself: Default = "vnd", the one place the default literal lives (P44.2). |
| `platform-management/internal/billing/reconcile.go` | `currency` | `noticeEntry` | field | code | todo | Audit detail copies the notice currency next to the order currency. Added at P44.2 (row was missing from the P44.1 table). |
| `platform-management/internal/billing/reconcile_match.go` | `currency` | `matchNotice` | field | code | todo | Compares notice and order currency (`currency_mismatch`). Added at P44.2 (row was missing from the P44.1 table). |
| `platform-management/internal/billing/registry.go` | `currency` | `file-level comment`, `ErrNoProvider`, `NewRegistry`, `ForCurrency` | field | code | todo | Registry maps currency to provider; ForCurrency is the lookup. P44.2 configures providers per enabled currency. |
| `platform-management/internal/billing/registry.go` | `vnd` | `file-level comment`, `NewRegistry` | literal | code | todo | Empty spec means "vnd=manual"; P44.2: configure per plan currency. |
| `platform-management/internal/billing/sepay.go` | `currency` | `sepayCurrency`, `ParseNotification` | literal | code | todo | sepayCurrency constant used for the notice currency; P44.2: read from the order. |
| `platform-management/internal/billing/sepay.go` | `vnd` | `sepayCurrency` | literal | code | todo | sepayCurrency = "VND"; P44.2: read from the order. |
| `platform-management/internal/billing/service.go` | `currency` | `currencyPattern`, `Draft.Currency`, `validate`, `orderDetail` | field | code | todo | Draft.Currency and the three-letter pattern check. P44.2 checks the code against the rule set, not only the pattern. |
| `platform-management/internal/billing/usage.go` | `currency` | `ActivateUsage` | literal | code | todo | Zero-amount usage order gets Currency "vnd"; P44.2: read from plan or subscription. |
| `platform-management/internal/billing/usage.go` | `vnd` | `file-level comment`, `ActivateUsage` | literal | code | todo | Literal "vnd" on the usage activation order; P44.2: read from plan or subscription. |
| `platform-management/internal/billing/vat.go` | `vnd` | `file-level comment` | text | comment | todo | Comment: VAT in whole VND; reword when P44.2 lands. |
| `platform-management/internal/billing/webhook.go` | `currency` | `serve` | field | code | todo | Copies notification Currency into the stored notice. P44.2 checks it against the rule set. |
| `platform-management/internal/billingapi/billingapi.go` | `currency` | `checkout`, `previewResponse.Currency`, `preview`, `purchaseRequest.Currency`, `purchaseResponse.Currency`, `purchase` | field | code | todo | Request and response Currency; the quote compares the request currency with the plan. P44.2 checks it against the rule set. |
| `platform-management/internal/billingapi/billingapi.go` | `vnd` | `refuseUnpaidUsage` | text | comment | todo | Comment: paid for 0 VND; reword when P44.2 lands. |
| `platform-management/internal/billingapi/planchange.go` | `currency` | `planChangePreviewResponse.Currency`, `planChangeOrderResponse.Currency`, `previewPlanChange`, `writeChangeOrder` | field | code | ruled | Plan-change preview and order responses take Currency from the target plan or the order; the paid-plan check compares with the current plan. |
| `platform-management/internal/config/config.go` | `currency` | `Error`, `EmptyBankVariables`, `loadBilling`, `loadPayments` | literal | code | todo | loadPayments checks a provider for money.Default only. P44.2 requires one provider per enabled currency (PLATFORM_CURRENCIES). |
| `platform-management/internal/config/config.go` | `vnd` | `loadBilling`, `loadPayments` | literal | code | todo | Boot check uses money.Default (P44.2); the comment above loadPayments still names vnd. Per-plan-currency check stays todo. |
| `platform-management/internal/console/einvoices.go` | `currency` | `ListEinvoices`, `toEinvoiceJSON` | field | code | todo | E-invoice list row and JSON currency from the issuance row. P44.2 checks the issuance path against the rule set. |
| `platform-management/internal/console/orders.go` | `currency` | `orderJSON.Currency`, `toOrderJSON`, `cancelOrder` | field | code | ruled | Order JSON and the cancel audit detail take Currency from the order row; no literal. |
| `platform-management/internal/console/payment_notices.go` | `currency` | `listEinvoices`, `noticeColumns` | field | code | todo | Notice list columns and the currency match in the notice lookup. P44.2 checks the notice currency against the rule set. |
| `platform-management/internal/console/pgx.go` | `currency` | `orderFromRow` | field | code | ruled | orderFromRow maps the stored order currency; no literal. |
| `platform-management/internal/db/models.go` | `currency` | `BillingOrder.Currency`, `EinvoiceIssuance.Currency`, `PaymentNotice.Currency`, `Plan.Currency` | field | code | todo | sqlc model fields for billing_order, einvoice_issuance, payment_notice and plan. Regenerated from the SQL; follows any P44.2 SQL change. |
| `platform-management/internal/db/models.go` | `usd` | `file-level comment` | text | comment | exempt | Comment on provider-reported AI cost in 1e-10 USD; agent cost, not platform billing. |
| `platform-management/internal/invoice/pgx.go` | `currency` | `insert`, `ReplaceIssued`, `fromRow` | field | code | todo | Insert and row mapping for e-invoice issuance. P44.2 checks the record currency against the rule set. |
| `platform-management/internal/invoice/provider.go` | `currency` | `IssueRequest.Currency` | field | code | todo | IssueRequest.Currency sent to the e-invoice provider. P44.2 checks the code against the rule set. |
| `platform-management/internal/invoice/record.go` | `currency` | `Record.Currency`, `NewRecord.Currency` | field | code | todo | Record and NewRecord fields, filled from the order at Enqueue. P44.2 checks the code against the rule set. |
| `platform-management/internal/invoice/service.go` | `currency` | `Enqueue`, `process` | field | code | ruled | Issuance record and request take Currency from the paid order and the record; no literal. |
| `platform-management/internal/invoice/store.go` | `currency` | `PaidOrder` | field | code | todo | PaidOrder.Currency, filled from the order row by ListPaidWithoutRecord (pgx.go). P44.2 checks the code against the rule set. |
| `platform-management/internal/lifecycle/expire.go` | `currency` | `Run` | field | code | ruled | Expiry audit detail takes the order currency; no literal. |
| `platform-management/internal/lifecycle/pgx.go` | `currency` | `orderFromRow` | field | code | ruled | orderFromRow maps the stored order currency; no literal. |
| `platform-management/internal/paymentnotice/notice.go` | `currency` | `Notice.Currency`, `NewNotice.Currency` | field | code | todo | Notice and NewNotice currency from the provider. P44.2 checks it against the rule set. |
| `platform-management/internal/paymentnotice/pgx.go` | `currency` | `Insert`, `fromRow` | field | code | todo | Insert and row mapping for payment notices. P44.2 checks the notice currency against the rule set. |
| `platform-management/internal/plan/pgx.go` | `currency` | `InsertIfAbsent`, `FromRow`, `insertParams.Currency`, `toInsertParams` | field | code | todo | Insert params and row mapping for plan. P44.2 checks the insert path against the rule set. |
| `platform-management/internal/plan/currency.go` | `currency` | `ValidateCurrency` | field | code | ruled | The enabled-set check on plan save: a currency outside PLATFORM_CURRENCIES is refused (P44.2b slice 3a). |
| `platform-management/internal/plan/plan.go` | `currency` | `IsUnlimited`, `Plan.Currency`, `Catalogue`, `Validate` | field | code | todo | Plan.Currency and the lower-case three-letter pattern check. P44.2 checks the code against the rule set as well. |
| `platform-management/internal/plan/plan.go` | `vnd` | `IsUnlimited` | text | comment | todo | Comment: VND has no decimals; reword when P44.2 lands. |
| `platform-management/internal/subscription/api.go` | `currency` | `priceJSON.Currency`, `listedPlanJSON.Currency`, `buildPrices` | field | code | ruled | Price and listed-plan JSON take Currency from the plan; no literal. |
| `platform-management/internal/subscription/console.go` | `currency` | `putPlanBody.Currency`, `putPlan` | field | code | ruled | Plan save keeps the stored currency when the body omits it, refuses a change while in use, and checks the code with plan.ValidateCurrency against PLATFORM_CURRENCIES. |
| `platform-management/internal/subscription/quote.go` | `vnd` | `conflict` | text | comment | todo | Comment on the quote rounding; reword when P44.2 lands. |
| `platform-management/internal/subscription/usage.go` | `currency` | `Usage.Currency` | field | code | todo | Usage JSON field, filled from the plan by usage/view.go. Declaration only; P44.2 checks the plan currency it carries. |
| `platform-management/internal/usage/invoicing.go` | `currency` | `invoice`, `invoiceDraft` | field | code | ruled | Usage invoice draft takes the plan currency from GetUsageTerms (P44.3). The "vnd" literal row is removed with its last literal. |
| `platform-management/internal/usage/notice.go` | `vnd` | `noticeText` | literal | amount with code in message text | todo | Notice text prints "VND"; P44.2: print the plan currency. |
| `platform-management/internal/usage/view.go` | `currency` | `UsageInput.Currency`, `BuildUsage`, `Usage` | field | code | ruled | Usage block takes Currency from the plan (p.Currency); no literal. |
| `server/cmd/multica/cmd_company_usage.go` | `usd` | `file-level comment`, `runCompanyUsage` | text | UI label | exempt | CLI heading 'COST (USD TICKS)' for provider-reported AI cost; not platform billing. |
| `server/internal/cli/update.go` | `vnd` | `fetchReleaseByTag`, `FetchLatestRelease` | text | media type | exempt | HTTP Accept media type application/vnd.github+json; not a currency. |
| `server/internal/daemon/types.go` | `usd` | `file-level comment` | text | comment | exempt | Comment on the provider's AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/internal/handler/agent.go` | `usd` | `attributionsOf` | text | comment | exempt | Comment on the provider's AI cost in 1e-10 USD; agent cost, not platform billing. |
| `server/internal/handler/billing_orders.go` | `currency` | `workspacePlanJSON.Currency`, `billingBankDetails`, `workspaceOrderJSON.Currency`, `toWorkspaceOrderJSON`, `ListWorkspacePlans` | field | code | ruled | Plan JSON and order JSON take Currency from the plan and the order; no literal. |
| `server/internal/handler/billing_orders.go` | `vnd` | `file-level comment`, `billingBankDetails` | text | comment | todo | Comments on the usage rate and VND having no decimals; reword when P44.2 lands. |
| `server/internal/handler/cloud_billing.go` | `currency` | `cloudSubscriptionSeatPurchaseRequest.Currency`, `GetCloudWorkspaceSubscriptionUsage`, `PurchaseCloudWorkspaceSubscriptionSeats`, `isASCIICurrency` | field | code | todo | Seat purchase request currency is lower-cased and shape-checked (isASCIICurrency), not checked against the rule set. P44.2 adds the check. |
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
| `packages/core/api/client.ts` | `currency` | `purchaseWorkspaceSeats` | field | code | todo | Sends the preview currency in the seat purchase body; the server checks it against the plan. P44.2 checks it against the rule set. |
| `packages/core/api/schemas.ts` | `currency` | `BillingTopupSchema`, `BillingCheckoutSessionStatusSchema`, `EMPTY_BILLING_CHECKOUT_SESSION_STATUS`, `WorkspaceUsagePeriodSchema`, `WorkspaceSubscriptionPriceSchema`, `WorkspaceSeatPurchasePreviewSchema`, `PurchaseWorkspaceSeatsResponseSchema`, `WorkspacePlanChangePreviewSchema`, `OrderPlanChangeResponseSchema` | field | code | todo | Schema field for the currency code. P44.2 validates the code against the rule set. |
| `packages/core/api/schemas.ts` | `usd` | `EMPTY_CLOUD_RUNTIME_NODE`, `BillingTopupSchema`, `BillingCheckoutSessionStatusSchema`, `EMPTY_BILLING_CHECKOUT_SESSION_STATUS` | literal | code | todo | P44.2: default comes from the plan or subscription, not "usd". |
| `packages/core/billing/workspace-orders-schemas.ts` | `currency` | `WorkspacePlanSchema`, `WorkspaceOrderSchema` | field | code | todo | Schema field for plan and order currency. P44.2 validates it against the rule set. |
| `packages/core/billing/workspace-orders-types.ts` | `currency` | `file-level comment`, `WorkspacePlan.currency`, `WorkspaceOrder.currency` | field | code | todo | Type field for plan and order currency. P44.2 owns any rule-set check. |
| `packages/core/billing/workspace-orders-types.ts` | `vnd` | `file-level comment` | text | comment | todo | Comment only: "VND has no decimals"; reword when P44.2 lands. |
| `packages/core/squads/company-template.ts` | `usd` | `DEFAULT_RUN_COST_USD`, `CompanyEstimate`, `estimateCompanyRuns` | literal | USD amount (0.5 per run) | exempt | Estimated AI run cost (DEFAULT_RUN_COST_USD, 0.5 per run) for the company wizard; not a platform charge. |
| `packages/core/squads/kitviet-previous-texts.ts` | `usd` | `KITVIET_PREVIOUS_AGENT_INSTRUCTIONS` | text | prompt text | exempt | Frozen agent prompt text that mentions USD; not code and not billing. |
| `packages/core/squads/sales-starter-sources.ts` | `usd` | `SALES_STARTER_SOURCES` | text | UI copy | exempt | UI copy price hints for third-party data sources; not platform billing. |
| `packages/core/types/agent.ts` | `usd` | `AgentTask`, `IssueUsageSummary` | text | comment | exempt | Comment on the provider's AI cost in 1e-10 USD; agent cost, not platform billing. |
| `packages/core/types/billing.ts` | `currency` | `BillingTopup.currency`, `BillingCheckoutSessionStatus.currency`, `WorkspaceUsagePeriod.currency`, `WorkspaceSubscriptionPrice.currency`, `WorkspaceSubscriptionPrice`, `WorkspaceSeatPurchasePreview.currency`, `WorkspaceSeatPurchasePreview`, `PurchaseWorkspaceSeatsRequest.currency`, `PurchaseWorkspaceSeatsResponse.currency`, `WorkspacePlanChangePreview.currency`, `CreateWorkspacePlanChangeResponse.currency` | field | code | todo | Type fields for the currency code; the amounts beside them are minor units. P44.2 checks the code against the rule set. |
| `packages/core/types/billing.ts` | `usd` | `file-level comment` | text | comment | todo | Comment on the credit unit (1 USD = 1000 credit). P44.2 confirms whether credits are a billed unit; no code reads the comment. |
| `packages/core/types/billing.ts` | `vnd` | `WorkspaceUsageMeter` | text | comment | todo | Comment on the usage meter price unit; reword when P44.2 lands. |
| `packages/ui/components/ui/number-flow.tsx` | `currency` | `CurrencyNumberFlowProps`, `CurrencyNumberFlow`, `CappedNumberFlow` | formatter | formatted amount | todo | CurrencyNumberFlow defaults the prefix to the literal '$' and takes no currency code. P44.2 confirms which currency it formats. |
| `packages/views/billing/billing-test-page.tsx` | `currency` | `CheckoutSessionStatusBanner`, `TopupRow`, `formatMoney` | formatter | formatted amount | todo | Local formatMoney via Intl.NumberFormat; P44.2: use the shared formatter. |
| `packages/views/billing/billing-test-page.tsx` | `money-format` | `CheckoutSessionStatusBanner`, `TierButton`, `TopupRow`, `formatMoney` | formatter | formatted amount | todo | Calls the local formatMoney; P44.2: use the shared formatter. |
| `packages/views/billing/billing-test-page.tsx` | `usd` | `TierButton` | literal | code | todo | Test page passes "usd" to formatMoney; P44.2: read from the tier. |
| `packages/views/billing/order-page.tsx` | `currency` | `BillingOrder.currency`, `normalizeOrder`, `BillingOrderPage` | field | code | todo | Order currency passed to the formatter; normalizeOrder falls back to 'vnd' (line 96). P44.2 removes the fallback. |
| `packages/views/billing/order-page.tsx` | `money-format` | `file-level comment`, `BillingOrderPage` | formatter | formatted amount | todo | Calls formatStripeMinorAmount. P44.2 replaces it with the shared minor-unit rule. |
| `packages/views/billing/order-page.tsx` | `vnd` | `normalizeOrder` | literal | code | todo | Falls back to "vnd" when the order has no currency; P44.2: no fallback, read from the order. |
| `packages/views/billing/plan-change-dialog.tsx` | `currency` | `PlanChangeDialog` | field | code | todo | Preview currency passed to the formatter. P44.2 uses the shared formatter. |
| `packages/views/billing/plan-change-dialog.tsx` | `money-format` | `file-level comment`, `PlanChangeDialog` | formatter | formatted amount | todo | Calls formatStripeMinorAmount. P44.2 replaces it with the shared minor-unit rule. |
| `packages/views/billing/scheduled-price-change-banner.tsx` | `currency` | `ScheduledPriceChangeBannerProps.currency`, `ScheduledPriceChangeBanner` | field | code | todo | Banner prop passed to formatUsageMoney. P44.2 uses the shared formatter. |
| `packages/views/billing/usage-card.tsx` | `currency` | `formatUsageMoney`, `UsageCard` | formatter | formatted amount | todo | formatUsageMoney appends the code in upper case; P44.2: use the shared formatter. |
| `packages/views/billing/usage-card.tsx` | `money-format` | `file-level comment`, `formatUsageMoney` | formatter | formatted amount | todo | Calls formatStripeMinorAmount. P44.2 replaces it with the shared minor-unit rule. |
| `packages/views/billing/usage-invoice-lines.tsx` | `currency` | `UsageInvoiceLinesProps.currency`, `UsageInvoiceLines` | field | code | todo | Prop passed to formatUsageMoney. P44.2 uses the shared formatter. |
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
| `packages/views/settings/components/billing-format.ts` | `currency` | `STRIPE_ZERO_DECIMAL_CURRENCIES`, `STRIPE_TWO_DECIMAL_COMPAT_CURRENCIES`, `STRIPE_THREE_DECIMAL_CURRENCIES`, `formatStripeMinorAmount` | formatter | formatted amount | todo | Zero-, two- and three-decimal currency sets and formatStripeMinorAmount; P44.2: replace with the shared minor-unit rule. |
| `packages/views/settings/components/billing-format.ts` | `money-format` | `formatStripeMinorAmount` | formatter | formatted amount | todo | formatStripeMinorAmount definition. P44.2 replaces it with the shared minor-unit rule. |
| `packages/views/settings/components/billing-format.ts` | `vnd` | `STRIPE_ZERO_DECIMAL_CURRENCIES` | literal | code | todo | "VND" in the zero-decimal set; P44.2: minor-unit exponent comes from the rule set. |
| `packages/views/settings/components/billing-tab.tsx` | `currency` | `BillingTabContent` | field | code | todo | Compares preview and request currency and reads the plan currency for prices. P44.2 checks these against the rule set. |
| `packages/views/settings/components/billing-tab.tsx` | `money-format` | `file-level comment`, `BillingTabContent` | formatter | formatted amount | todo | Calls formatStripeMinorAmount. P44.2 replaces it with the shared minor-unit rule. |
| `packages/views/settings/components/members-tab.tsx` | `currency` | `MembersTab` | field | code | todo | Preview currency passed to the formatter. P44.2 uses the shared formatter. |
| `packages/views/settings/components/members-tab.tsx` | `money-format` | `file-level comment`, `MembersTab` | formatter | formatted amount | todo | Calls formatStripeMinorAmount. P44.2 replaces it with the shared minor-unit rule. |
| `packages/views/settings/components/seat-invite-purchase.ts` | `currency` | `seatPurchaseMatchesPreview` | field | code | todo | Compares response and preview currency. P44.2 checks the code against the rule set. |
| `platform-management/console/src/api/hooks.ts` | `currency` | `PutPlanInput.currency`, `usePutPlan` | field | code | todo | PutPlan input field sent to the console API. P44.2 checks the code against the rule set in the form. |
| `platform-management/console/src/api/schemas.ts` | `currency` | `OrderSchema`, `PlanSchema` | field | code | todo | Order and plan schema field. P44.2 validates the code against the rule set. |
| `platform-management/console/src/screens/Orders.tsx` | `currency` | `Orders` | formatter | formatted amount | todo | formatCurrency prints amount and upper-case code; P44.2: use the shared formatter. |
| `platform-management/console/src/screens/PaymentNotices.tsx` | `currency` | `formatCurrency` | formatter | formatted amount | todo | Prints the notice and candidate amounts with the notice's own currency code in upper case. Belongs to another session's untracked work; the row exists so the test sees this unit. P44.2 uses the shared formatter once that work lands. |
| `platform-management/console/src/screens/Plans.tsx` | `currency` | `Plans` | literal | code | todo | Form state defaults to "vnd"; P44.2: no default code. |
| `platform-management/console/src/screens/Plans.tsx` | `vnd` | `Plans` | literal | code | todo | Default "vnd" and placeholder "e.g. vnd, usd"; P44.2: no default code. |
| `platform-management/console/src/screens/PriceSchedule.tsx` | `currency` | `formatSeatValue`, `formatPriceSnapshot`, `PriceSchedule` | formatter | formatted amount | todo | formatSeatValue and formatPriceSnapshot print the code in upper case. P44.2 uses the shared formatter. |
| `platform-management/console/src/screens/TenantDetail.tsx` | `currency` | `TenantDetail` | formatter | formatted amount | todo | Inline toLocaleString plus upper-case code for orders. P44.2 uses the shared formatter. |

### JSON

| File | Term | Function or field | Kind | Holds | Status | Note |
| --- | --- | --- | --- | --- | --- | --- |
| `packages/views/locales/en/runtimes.json` | `usd` | `unit_hint` | text | UI copy | exempt | UI copy: unit hint for AI token rates; not platform billing. |
| `packages/views/locales/es/runtimes.json` | `usd` | `unit_hint` | text | UI copy | exempt | Locale string: unit hint for AI token rates; not platform billing. |
| `packages/views/locales/fr/runtimes.json` | `usd` | `unit_hint` | text | UI copy | exempt | Locale string: unit hint for AI token rates; not platform billing. |
| `packages/views/locales/ja/runtimes.json` | `usd` | `unit_hint` | text | UI copy | exempt | Locale string: unit hint for AI token rates; not platform billing. |
| `packages/views/locales/ko/runtimes.json` | `usd` | `unit_hint` | text | UI copy | exempt | Locale string: unit hint for AI token rates; not platform billing. |
| `platform-management/internal/plan/catalogue.json` | `currency` | `currency` | literal | code | todo | Catalogue seeds each plan with "vnd"; stays per-plan data, P44.2: validate against the rule set. |
| `platform-management/internal/plan/catalogue.json` | `vnd` | `currency` | literal | code | todo | Seed value "vnd" for the catalogue plans; stays per-plan data, P44.2: validate against the rule set. |

## Count

- Scanned units: 161. Rows: 161.
- Rows by Kind: field 63, literal 27, formatter 21, text 50.
- Rows by Status: todo 87, ruled 19, exempt 55.

## Not yet in the currency rule set

The rule set (`platform-management/internal/money`) exists. These 33 rows of Kind `literal` or `formatter` still hold a
hard-coded code or format an amount, so P44.2 has to move them onto the rule set:

- `platform-management/cmd/platform-server/main.go` `currency` (literal)
- `platform-management/cmd/platform-server/main.go` `vnd` (literal)
- `platform-management/internal/billing/registry.go` `vnd` (literal)
- `platform-management/internal/billing/sepay.go` `currency` (literal)
- `platform-management/internal/billing/sepay.go` `vnd` (literal)
- `platform-management/internal/billing/usage.go` `currency` (literal)
- `platform-management/internal/billing/usage.go` `vnd` (literal)
- `platform-management/internal/config/config.go` `currency` (literal)
- `platform-management/internal/config/config.go` `vnd` (literal)
- `platform-management/internal/usage/notice.go` `vnd` (literal)
- `packages/core/api/schemas.ts` `usd` (literal)
- `packages/ui/components/ui/number-flow.tsx` `currency` (formatter)
- `packages/views/billing/billing-test-page.tsx` `currency` (formatter)
- `packages/views/billing/billing-test-page.tsx` `money-format` (formatter)
- `packages/views/billing/billing-test-page.tsx` `usd` (literal)
- `packages/views/billing/order-page.tsx` `money-format` (formatter)
- `packages/views/billing/order-page.tsx` `vnd` (literal)
- `packages/views/billing/plan-change-dialog.tsx` `money-format` (formatter)
- `packages/views/billing/usage-card.tsx` `currency` (formatter)
- `packages/views/billing/usage-card.tsx` `money-format` (formatter)
- `packages/views/settings/components/billing-format.ts` `currency` (formatter)
- `packages/views/settings/components/billing-format.ts` `money-format` (formatter)
- `packages/views/settings/components/billing-format.ts` `vnd` (literal)
- `packages/views/settings/components/billing-tab.tsx` `money-format` (formatter)
- `packages/views/settings/components/members-tab.tsx` `money-format` (formatter)
- `platform-management/console/src/screens/Orders.tsx` `currency` (formatter)
- `platform-management/console/src/screens/PaymentNotices.tsx` `currency` (formatter)
- `platform-management/console/src/screens/Plans.tsx` `currency` (literal)
- `platform-management/console/src/screens/Plans.tsx` `vnd` (literal)
- `platform-management/console/src/screens/PriceSchedule.tsx` `currency` (formatter)
- `platform-management/console/src/screens/TenantDetail.tsx` `currency` (formatter)
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
