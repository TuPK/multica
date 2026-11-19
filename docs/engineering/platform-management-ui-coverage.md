# Platform console UI coverage matrix

Companion to [platform-management-flows.md](platform-management-flows.md) and [platform-management-e2e-gaps.md](platform-management-e2e-gaps.md). Generated against the console inventory (`discover-ui.mjs` over `platform-management/console/src`) and checked by `coverage-check.mjs` over `e2e/platform`.

Status: `covered` = a passing test exists (UI or API in e2e/platform); `fixme` = a test marks a product failure; `gap` = no passing test in e2e/platform, with the reason in the last column (a Go test may cover the rule; the reason says so).

| id | screen | action | status | test |
| --- | --- | --- | --- | --- |
| 12 | Login.tsx | staff signs in through the form | covered | a staff member signs in, reloads the page, and signs out |
| 13 | Layout.tsx | staff signs out and the session ends | covered | a staff member signs in, reloads the page, and signs out |
| 13 | Login.tsx | cleared session returns to sign-in | covered | clearing the session cookie returns the console to the sign-in form |
| 142 | Login.tsx | send button disabled for empty or blank email | covered | the send-code button stays disabled for an empty or blank email |
| 142 | Login.tsx | malformed email stopped by the form | covered | a malformed email is stopped by the form |
| 142 | Login.tsx | unknown and known emails get the same message | covered | an unknown email and a known email get the same neutral message |
| 143 | Login.tsx | wrong email code | covered | a wrong email code shows the invalid-code message |
| 143 | Login.tsx | code with letters or too short | covered | a code with letters or a short code is refused |
| 143 | Login.tsx | Enter sends the code; back keeps the address | covered | Enter in the email field sends the code |
| 143 | Login.tsx | wrong authenticator code | covered | a wrong authenticator code after a correct email code |
| 26 | Overview.tsx | counters visible | covered | the overview shows the platform's counters |
| 26 | Overview.tsx | overview error state | covered | the overview shows the error state when its request fails |
| 26 | Tenants.tsx | search narrows, empties, restores | covered | search narrows the tenant list |
| 26 | Tenants.tsx | tenant list error state | covered | a failed tenant list shows the error state |
| 26 | TenantDetail.tsx | sections visible | covered | the tenant detail shows the workspace state |
| 26 | TenantDetail.tsx | internal note add, blank disabled, read back | covered | staff adds an internal note and reads it back |
| 264 | TenantDetail.tsx | note over 4000 refused | covered | a note one character over 4000 is refused |
| 264 | TenantDetail.tsx | note of 4000 accepted | covered | a note of exactly 4000 characters is accepted |
| 26 | TenantDetail.tsx | detail error state | covered | a tenant whose detail fails to load shows the error state |
| 27 | TenantDetail.tsx | status change with reason | covered | staff changes a workspace status with a reason |
| 27 | ConfirmReasonDialog.tsx | cancel leaves status unchanged | covered | Cancel in the status dialog leaves the workspace status unchanged |
| 260 | ConfirmReasonDialog.tsx | reason over limit refused in dialog | covered | a reason one character over the limit is refused |
| 27 | ConfirmReasonDialog.tsx | Escape closes without change | covered | the Escape key closes the status dialog without a change |
| 27 | Tenants (support) | support status change; no Pending deletion | covered | support changes a workspace status |
| 63 | TenantDetail.tsx | edit billing profile | covered | flow 63: staff edits the billing profile in a dialog and the saved legal name is shown |
| 63 | TenantDetail.tsx | tax code that is not 10 digits | covered | flow 63: a tax code that is not 10 digits is refused in the billing profile dialog |
| 64 | TenantDetail.tsx | override subscription | covered | flow 64: staff overrides the tenant subscription with a reason and the dialog closes |
| 64 | TenantDetail.tsx | unknown plan key | covered | flow 64: an unknown plan key is refused and the subscription dialog stays open |
| 64 | TenantDetail.tsx | usage and unpaid usage orders | gap | the usage write-off needs a seeded usage order (flow 225); the usage section is only checked for presence |
| 20 | Users.tsx | search with no match | covered | search finds no one |
| 20 | Users.tsx | user list error state | covered | the users list shows the error state |
| 20 | UserDetail.tsx | account record and empty memberships | covered | finds a user and reads the account record |
| 21 | UserDetail.tsx | suspend with reason, reinstate | covered | search finds no one, then the target is suspended |
| 21 | UserDetail.tsx | suspension in the audit log | covered | the suspension shows in the audit log |
| 252 | UserDetail.tsx | own account cannot be suspended | covered | an admin cannot suspend their own account |
| 22 | UserDetail.tsx | sign out everywhere needs reason | covered | Sign out everywhere asks for a reason |
| 23 | UserDetail.tsx | reset two-step sign-in opens its dialog | covered | flow 23: staff opens the two-step sign-in reset for another user |
| 23 | UserDetail.tsx | reset removes the MFA row and the notice email | covered | flow 23: the reset emails the user that two-step sign-in was turned off |
| 14 | Staff.tsx | list shows the signed-in admin with role | covered | the staff list shows the signed-in admin |
| 14 | Staff.tsx | add with unknown email refused | covered | an unknown email is refused in the add dialog |
| 256 | Staff.tsx | active staff cannot be added twice | covered | an active staff member cannot be added twice |
| 14 | Staff.tsx | add billing staff, see row, remove | covered | an admin adds a billing staff member |
| 257 | Staff.tsx | last active admin cannot be revoked | gap | needs a second admin seed and the remove dialog; the remove dialog cannot be clicked yet |
| 24 | Accounts.tsx | list columns or empty state | covered | the accounts list shows its columns |
| 24 | Accounts.tsx | create button offers the dialog | covered | the accounts screen opens and offers account creation |
| 244 | Accounts.tsx | invalid fields in create dialog | covered | flow 244: admin submits an invalid workspace slug and the dialog shows the field error |
| 245 | Accounts.tsx | paid seat without trial or comp reason | covered | flow 245: a paid plan without a trial or complimentary reason is refused in the dialog |
| 246 | Accounts.tsx | email already has a Multica user | covered | flow 246: an email that already has a Multica user is refused in the dialog |
| 247 | Accounts.tsx | cancel a request not pending | covered | flow 247: a request that is not pending has no Cancel action |
| 248 | Accounts.tsx | hourly create cap | gap | needs more than the cap of requests in an hour; API test only |
| 249 | Accounts.tsx | resend welcome on ineligible state | covered | flow 249: a pending request offers no Resend welcome action |
| 249 | Accounts.tsx | completed request with no account | covered | flow 249: a completed request with no account yet is refused a resend, and the reason is shown |
| 250 | Accounts.tsx | resend welcome daily cap | gap | needs three resends in a day against a seeded request |
| 251 | Accounts.tsx | resend welcome transport error | gap | needs the mail provider to fail; the harness has no mail provider |
| 60 | PaymentNotices.tsx | queue and view a notice | covered | the payment notice queue opens and each row can be viewed |
| 60 | PaymentNotices.tsx | match a notice to an order | covered | flow 60: billing matches a payment notice to a pending order in a dialog |
| 61 | PaymentNotices.tsx | dismiss an unmatched notice | covered | flow 61: staff dismisses a payment notice in a dialog and it leaves the review queue |
| 61 | PaymentNotices.tsx | queue empty state | covered | billing opens the payment notice queue |
| 268 | PaymentNotices.tsx | malformed id refused (API test, not UI) | covered | flow 268: a malformed payment notice id is refused with invalid_id |
| 72 | EinvoiceFailures.tsx | state tabs switch | covered | the e-invoice screen shows its state tabs |
| 78 | EinvoiceFailures.tsx | retry, cancel or replace from the UI | gap | DROPPED (user decision): the e-invoice replace and cancel flows were removed; no test is planned |
| 26 | Audit.tsx | empty state for an unknown actor | covered | the audit log filters by an actor id that has no events |
| 26 | Audit.tsx | columns and action filter | covered | the audit log shows its columns and the action filter narrows the list |
| 26 | Audit.tsx | date range filter | covered | flow 26: a From date after every event empties the audit list, and clearing it restores the list |
| 62 | Orders.tsx | orders screen | covered | flow 83: Orders and Plans in the nav open their own screens |
| 221 | Orders.tsx | expired order status label | gap | Orders is routed from the nav (flow 83); the Expired label only shows on that screen |
| 65, 66, 67 | Plans.tsx, PriceSchedule.tsx | plan and price schedule screens | covered | flow 83: Orders and Plans in the nav open their own screens |
| 83 | Layout.tsx | Orders and Plans nav fall back to Overview | covered | flow 83: Orders and Plans in the nav open their own screens |
| 146 | Layout.tsx | support nav hides restricted screens | covered | support sees only the screens its role allows |
| 146 | Layout.tsx | billing nav hides restricted screens | covered | billing sees the billing screens |
| 146 | App.tsx | admin opens Staff and Accounts | covered | an admin opens the Staff and Accounts screens |
| 146 | Layout.tsx | mobile layout without horizontal scroll (Tenants) | covered | edge: the Tenants screen fits a 375 by 667 phone without horizontal scrolling |
| 13 | Layout.tsx | reload keeps session | covered | a reload keeps the admin signed in |
| 0 | ErrorState.tsx | error state renders | covered | the users list shows the error state |
| 0 | ErrorState.tsx | Try again reloads after a failed request | covered | the error state's Try again button loads the overview |
| 12 | Login.tsx | Start over returns from the authenticator step | covered | Start over on the authenticator step returns to the email step |
| 26 | Overview.tsx | Active Workspaces counter | covered | the overview shows the platform's counters |
| 247 | Accounts.tsx | Cancel account request and its Confirm cancel dialog | covered | flow 247: staff keeps a pending request, then cancels it and the row shows Canceled |
| 244-246 | Accounts.tsx | create form fields: account-kind, account-email, account-confirm-email, account-display-name, account-workspace-name, account-workspace-slug, account-plan-key, account-seats, account-interval, account-comp-reason | covered | admin submits an invalid workspace slug |
| 63 | TenantDetail.tsx | billing profile fields: billing-legal-name, billing-tax-code, billing-address, billing-invoice-email, bp-account-kind, Legal name, Tax code, Invoice email | covered | flow 63: staff edits the billing profile in a dialog and the saved legal name is shown |
| 24 | Accounts.tsx | account status labels: Provisioned, Completed, Needs attention, Needs info | covered | flow 24: each request state shows its own label in the accounts list |
| 26 | Audit.tsx | audit-actor-input | covered | the audit log filters by an actor id that has no events |
| 26 | Audit.tsx | audit-from-input, audit-to-input date range | covered | flow 26: a To date before every event shows the empty state |
| 62 | Orders.tsx | Confirm payment, Cancel order, transfer-reference-input, Refunded | gap | Orders is routed from the nav; confirm and cancel need a pending order (flows 220-224) |
| 225 | TenantDetail.tsx | Write off usage order, Write off order | gap | needs a seeded unpaid usage order; the write-off is admin-only and sits in a dialog |
| 72 | EinvoiceFailures.tsx | E-invoice history | gap | needs a seeded e-invoice with revisions; the history opens from the row |
| 60 | PaymentNotices.tsx | Candidate orders, Match payment to order, Match payment | covered | flow 60: billing matches a payment notice to a pending order in a dialog |
| 61 | PaymentNotices.tsx | Dismiss payment notice, Dismiss notice | covered | flow 61: staff dismisses a payment notice in a dialog and it leaves the review queue |
| 65-67, 232-240 | Plans.tsx, PriceSchedule.tsx | Price Change History, Plans Catalogue, plan-name-input, plan-currency-input, trial-days-input, sort-order-input, min-seats-input, seats-fixed-input, Preview impact, Cancel scheduled change, Schedule price change, price-effective-from, price-seat-month, price-seat-year, price-reason | gap | Orders and Plans are routed from the nav; these Plans and PriceSchedule actions are not yet tested in the UI (API tests cover the refusals) |
| 14 | Staff.tsx | staff-email-input, staff-role-select | covered | an admin adds a billing staff member |
| 27 | TenantDetail.tsx | target-status-select | covered | staff changes a workspace status with a reason |
| 64 | TenantDetail.tsx | Edit subscription, Edit Subscription, sub-plan-key-input, sub-seats-input, sub-interval-select, sub-trial-end-input, sub-reason-input | covered | flow 64: staff overrides the tenant subscription with a reason and the dialog closes |
| 26 | TenantDetail.tsx | Daily charges, Missing days | covered | flow 26: the monthly usage section shows the daily charges table or its empty note |
| 12 | Console Login screen | Staff console sign-in (OTP + conditional TOTP MFA) and session check | covered | a staff member signs in, reloads the page, and signs out |
| 13 | Console Layout top nav | Staff console sign-out | covered | a staff member signs in, reloads the page, and signs out |
| 14 | Console Staff screen | Admin lists, adds, and revokes platform staff members | covered | an admin adds a billing staff member |
| 20 | Console Users -> UserDetail | Staff searches users and inspects account metadata | covered | finds a user and reads the account record |
| 21 | Console UserDetail | Staff suspends or reactivates user account with realtime WS disconnect | covered | search finds no one, then the target is suspended |
| 22 | Console UserDetail | Staff force-revokes all user sessions with realtime WS disconnect | covered | Sign out everywhere asks for a reason |
| 23 | Console UserDetail | Staff resets another user's MFA with email notification | covered | flow 23: the reset emails the user that two-step sign-in was turned off |
| 24 | Console Accounts screen | Admin creates provisioned account request, cancels pending request, or resends welcome email (and customer signs in with ALLOW_SIGNUP=false) | covered | flow 24: each request state shows its own label in the accounts list |
| 25 | Console Accounts screen | Admin welcome email resend records spec audit action account.welcome_resent | covered | flow 25: a welcome resend that is sent answers 200 and records account.welcome_resent in the audit log |
| 26 | Console Overview -> Tenants -> TenantDetail / Audit | Staff views platform overview KPIs, searches tenants, inspects tenant metadata, views audit logs, and adds internal notes | covered | flow 26: the audit log shows its columns and the action filter narrows the list |
| 27 | Console TenantDetail | Staff changes tenant workspace status (active, read_only, suspended, pending_deletion) | covered | staff changes a workspace status with a reason |
| 60 | Console PaymentNotices screen | Staff inspects payment notice queue and resolves notice onto candidate pending order | covered | flow 60: billing matches a payment notice to a pending order in a dialog |
| 61 | Console PaymentNotices screen | Staff dismisses unmatched payment notice from queue | covered | flow 61: staff dismisses a payment notice in a dialog and it leaves the review queue |
| 62 | Console Orders.tsx (unrouted) or API | Staff confirms or cancels pending order via Console Orders API / unrouted Orders.tsx | covered | flow 62: staff cancels a pending order and it is no longer pending |
| 63 | Console TenantDetail | Staff views, edits, verifies, and unverifies tenant billing profile | covered | flow 63: staff edits the billing profile in a dialog and the saved legal name is shown |
| 64 | Console TenantDetail | Staff inspects tenant usage, writes off unpaid usage order (admin), and overrides tenant subscription | covered | flow 64: staff overrides the tenant subscription with a reason and the dialog closes |
| 65 | Console Plans.tsx (unrouted) or API | Admin lists, updates non-price plan metadata, and deletes unused plan via Console API / unrouted Plans.tsx | covered | flow 65: staff edits a plan's name, and the catalogue shows the new name |
| 66 | Console Plans screen or API | Admin creates a new or private plan via POST /api/platform/plans | covered | flow 66: an admin creates a private plan with POST and it is stored under its key |
| 67 | Console PriceSchedule.tsx (unrouted) or API | Admin previews (?dry_run=1), schedules, views history, and cancels plan price change | covered | flow 67: staff previews a price change, and nothing is scheduled |
| 72 | Console EinvoiceFailures screen | Billing or admin staff lists e-invoices in Console EinvoiceFailures screen | covered | the e-invoice screen shows its state tabs and switches between them |
| 73 | Console EinvoiceFailures screen or API | Billing or admin staff inspects single order e-invoice revision history via GET /api/platform/orders/{orderId}/einvoices | covered | flow 73: the order's e-invoice history lists its revision with its state |
| 76 | Console API POST /api/platform/einvoices/{orderId}/retry | Billing or admin staff retries failed or needs_info e-invoice via POST /api/platform/einvoices/{orderId}/retry with live provider [NOT BUILT] | gap | DROPPED (user decision): the e-invoice retry flow was removed; no test is planned |
| 77 | Console API POST /api/platform/einvoices/{orderId}/replace | Admin replaces issued e-invoice via POST /api/platform/einvoices/{orderId}/replace with live provider [NOT BUILT] | gap | DROPPED (user decision): the e-invoice replace and cancel flows were removed; no test is planned |
| 78 | Console EinvoiceFailures screen | Billing or admin staff triggers Retry, Cancel, or Replace from Console EinvoiceFailures UI [NOT BUILT] | gap | DROPPED (user decision): the e-invoice replace and cancel flows were removed; no test is planned |
| 79 | Console API | Admin cancels issued e-invoice via POST /api/platform/einvoices/{orderId}/cancel [NOT BUILT] | gap | DROPPED (user decision): the e-invoice replace and cancel flows were removed; no test is planned |
| 83 | Console Layout top nav | Staff clicks Orders or Plans in Console top nav and falls back to Overview | covered | flow 83: Orders and Plans in the nav open their own screens |
| 142 | Console Login screen | Non-staff, locked account, or rate-limited caller requests Console OTP send (silent 204) | covered | an unknown email and a known email get the same neutral message |
| 143 | Console Login screen | Staff enters wrong OTP or TOTP on Console verify-code (invalid_code) | covered | a wrong email code shows the invalid-code message |
| 144 | Console SPA or HTTP client | Browser or caller sends Console auth, price-change, or e-invoice request with bad/missing Origin (bad_origin) | covered | flow 144: a caller with an origin that is not the console is refused with bad_origin |
| 145 | Console listener (:30001) | Caller hits Console proxy with dot segments, non-platform /api/* paths, or dead API upstream (404 / 502) | covered | flow 145: a path that is not under /api/platform/ is answered by the console proxy as not found |
| 146 | Console API /api/platform/* | Unauthenticated, expired (>12h), non-MFA, revoked, locked, outside-CIDR, wrong-method, or unauthorized-role caller hits /api/platform/* (not_found obfuscation) | covered | support sees only the screens its role allows |
| 188 | Console Accounts screen | Admin creates account request with mismatched kind and plan audience (plan_audience_mismatch) | covered | flow 188: a plan that is not sold for the account kind is refused with the field error |
| 190 | Console TenantDetail | Staff verifies incomplete tenant billing profile (billing_profile_incomplete) | covered | flow 190: verifying a tenant whose billing profile is incomplete is refused and names the missing fields |
| 216 | Console PaymentNotices | Staff resolves or dismisses payment notice no longer in needs_review (payment_notice_closed) | covered | flow 216: dismissing a notice that someone else already closed shows the closed message |
| 217 | Console PaymentNotices | Staff hits concurrent state change resolving or dismissing payment notice (payment_notice_conflict) | gap | race: two staff resolving the same notice at once; needs a forced interleaving, not reproduced here. Go test payment_notice_conflict |
| 220 | Console Orders / PaymentNotices / API | Staff confirms or cancels non-pending order (order_not_pending) | covered | flow 220: a canceled order cannot be canceled or confirmed again |
| 221 | Console Orders / PaymentNotices | Staff confirms order or resolves notice onto expired pending order (order_expired) | covered | flow 221: an order past its expiry cannot be confirmed |
| 222 | Console Orders / PaymentNotices | Staff confirms, cancels, or resolves nonexistent order ID (order_not_found) | covered | flow 222: confirming an order id that does not exist is refused with order_not_found |
| 223 | Console Orders / PaymentNotices | Staff confirms or resolves order incompatible with current subscription (order_not_applicable) | covered | flow 223: confirming an add-seats order when the workspace has no subscription is refused with order_not_applicable |
| 224 | Console Orders / PaymentNotices | Staff confirms or resolves order during concurrent subscription race (order_conflict) | gap | race: a subscription changed between the check and the apply; not reproducible on demand. Go tests only |
| 225 | Console Orders / TenantDetail / API | Billing staff attempts to cancel or write off usage invoice (usage_cancel_admin_only) | covered | flow 225: an admin writes off an unpaid usage order and it leaves the unpaid list |
| 228 | Console API POST /api/platform/einvoices/{orderId}/{retry,replace} | Staff calls e-invoice retry or replace while provider is unconfigured (einvoice_provider_not_configured) | covered | flow 228: an e-invoice retry while the provider is not configured is refused with einvoice_provider_not_configured |
| 229 | Console API POST /api/platform/einvoices/{orderId}/{retry,replace} | Staff calls e-invoice retry or replace on incomplete buyer fields (einvoice_buyer_incomplete) | covered | flow 229: retrying a failed invoice whose buyer lacks required fields is refused and names the fields |
| 230 | Console API POST /api/platform/einvoices/{orderId}/{retry,replace} | Staff calls e-invoice retry or replace on ineligible e-invoice state (einvoice_state_conflict) | covered | flow 230: retrying an invoice that is already issued is refused with einvoice_state_conflict |
| 231 | Console API POST /api/platform/einvoices/{orderId}/replace | Admin calls e-invoice replace with invalid buyer field (>500 chars or NUL) (invalid_buyer) | gap | NOT BUILT: the code has no invalid_buyer check (the e-invoice buyer checks return einvoice_buyer_incomplete only) |
| 232 | Console PriceSchedule / Plans | Admin submits invalid price, meter, currency, or non-00:00-UTC effective_from (price_invalid) | covered | flow 232: a negative seat price is refused in the price form before it is sent |
| 233 | Console Plans / API | Admin sends price fields directly to PUT /api/platform/plans/{key} (price_field_via_schedule) | covered | flow 233: a PUT to a plan with a price field is refused with price_field_via_schedule |
| 234 | Console PriceSchedule | Admin schedules price change when plan already has a scheduled change (price_change_pending) | covered | flow 234: a second price change while one is scheduled is refused with price_change_pending |
| 235 | Console PriceSchedule | Admin cancels scheduled price change when none is scheduled (price_change_not_found) | covered | flow 235: cancelling a scheduled price change when none is scheduled is refused with price_change_not_found |
| 236 | Console PriceSchedule | Admin schedules price delta above PLATFORM_PRICE_CONFIRM_PCT without confirmation (price_change_confirm_required) | covered | flow 236: a price change above the 50% threshold cannot be scheduled until it is confirmed |
| 237 | Console PriceSchedule | Admin schedules price increase shorter than PLATFORM_PRICE_NOTICE_DAYS for paid subscribers (price_notice_too_short) | covered | flow 237: a price increase on a plan with paid subscribers, effective before the notice period, is refused with price_notice_too_short |
| 238 | Console Plans / API | Admin changes currency on plan referenced by subscriptions or orders (plan_currency_locked) | covered | flow 238: a currency change on a plan that has orders is refused in the edit dialog |
| 239 | Console Plans / API | Admin updates plan with stale expected_version returning 409 plan_version_conflict | covered | flow 239: a plan update with a stale expected_version is refused with plan_version_conflict |
| 240 | Console Plans / API | Admin deletes plan still referenced by subscriptions (plan_in_use) | covered | flow 240: deleting a plan that a subscription still uses is refused with plan_in_use |
| 241 | Console TenantDetail / API | Staff assigns archived plan in tenant subscription override (plan_archived) | covered | flow 241: a tenant subscription override to an archived plan is refused with plan_archived |
| 242 | Console TenantDetail / API | Staff assigns unknown plan_key in tenant subscription override (unknown_plan) | covered | flow 64: an unknown plan key is refused and the subscription dialog stays open |
| 243 | Console TenantDetail / API | Staff hits concurrent update race overriding tenant subscription (concurrent_change) | gap | concurrent insert on a workspace with no subscription row needs a forced race; Go test only |
| 244 | Console Accounts | Admin submits invalid fields creating provisioned account request (account_request_invalid) | covered | flow 244: admin submits an invalid workspace slug and the dialog shows the field error |
| 245 | Console Accounts | Admin creates paid seat account request without trial or comp_reason (account_activation_required) | covered | flow 245: a paid plan without a trial or complimentary reason is refused in the dialog |
| 246 | Console Accounts | Admin creates account request for existing Multica user email (account_email_exists) | covered | flow 246: an email that already has a Multica user is refused in the dialog |
| 247 | Console Accounts | Admin cancels account request not in pending state (account_request_not_cancelable) | covered | flow 247: staff keeps a pending request, then cancels it and the row shows Canceled |
| 248 | Console Accounts | Admin exceeds hourly PLATFORM_ACCOUNT_CREATE_PER_HOUR cap (account_create_rate_limited) | covered | flow 248: a create past the hourly cap is refused in the create dialog |
| 249 | Console Accounts | Admin resends welcome email on ineligible account request state (account_request_not_resendable) | covered | flow 249: a completed request with no account yet is refused a resend, and the reason is shown |
| 250 | Console Accounts | Admin exceeds 3 welcome email resends per day (welcome_resend_rate_limited) | covered | flow 250: a fourth welcome resend in one day is refused by the daily limit |
| 251 | Console Accounts | Admin welcome email resend fails on email transport error (welcome_send_failed) | covered | flow 251: a welcome resend that the mail provider refuses shows the send-failed message |
| 252 | Console UserDetail / API | Staff targets own account status (self_target) | covered | an admin cannot suspend their own account |
| 253 | Console UserDetail / API | Staff attempts self MFA reset (self_reset_refused) | covered | flow 253: staff cannot reset their own two-step sign-in |
| 254 | Console UserDetail / API | Staff changes status of permanently deleted user account (account_locked) | covered | flow 254: an account that is pending deletion or deleted cannot be changed |
| 255 | Console Staff screen | Admin adds staff email that has no Multica account (no_such_account) | covered | an unknown email is refused in the add dialog |
| 256 | Console Staff screen | Admin adds user who is already active staff (already_staff) | covered | an active staff member cannot be added twice |
| 257 | Console Staff screen | Admin attempts to revoke final active admin (last_admin) | covered | flow 257: removing the last active admin is refused in the remove dialog |
| 258 | Console Staff screen / API | Admin adds staff with invalid role (invalid_role) | covered | flow 258: an unknown staff role is refused with invalid_role |
| 259 | Console Staff screen / API | Admin adds staff with malformed or >254-char email (invalid_email) | covered | flow 259: the add-staff form stops a malformed email before it is sent, and the dialog stays open |
| 260 | Console mutation dialogs / API | Staff submits console mutation with missing or blank reason (reason_required) | covered | a reason one character over the limit is refused |
| 261 | Console mutation dialogs / API | Staff submits console reason exceeding max length (reason_too_long) | covered | a reason one character over the limit is refused |
| 262 | Console Orders / API | Staff submits transfer_reference >100 chars confirming order (transfer_reference_too_long) | covered | flow 262: a transfer reference over 100 characters is refused with transfer_reference_too_long |
| 263 | Console TenantDetail / API | Staff submits empty internal tenant note (body_required) | covered | staff adds an internal note and reads it back |
| 264 | Console TenantDetail / API | Staff submits internal tenant note >4000 chars (body_too_long) | covered | a note one character over 4000 is refused |
| 265 | Console write endpoints | Staff submits malformed JSON, trailing data, or NUL byte to Console or pricechange (invalid_body) | covered | flow 265: a body that is not JSON is refused with invalid_body |
| 266 | Console list screens / API | Staff passes invalid query parameter to Console list endpoints (invalid_filter) | covered | flow 266: a limit outside 1 to the maximum is refused with invalid_filter |
| 267 | Console list screens / API | Staff passes invalid pagination cursor to Console list endpoints (invalid_cursor) | covered | flow 267: a malformed cursor is refused with invalid_cursor |
| 268 | Console PaymentNotices / API | Staff passes malformed UUID path parameter to payment notices endpoints (invalid_id) | covered | flow 268: a malformed payment notice id is refused with invalid_id |
| 269 | Console PaymentNotices / API | Staff passes non-UUID order_id resolving payment notice (invalid_order_id) | covered | flow 269: an order_id that is not a UUID is refused with invalid_order_id |
| 270 | Console Tenants / Users / Orders / API | Staff submits invalid status enum on tenant, user, or order endpoints (invalid_status) | covered | flow 270: an unknown tenant status is refused with invalid_status |
| 271 | Console PaymentNotices / EinvoiceFailures / API | Staff passes invalid state filter (including cancelled on e-invoices) to payment notices or e-invoices list (invalid_state) | covered | flow 271: an e-invoice list filter on a state that does not exist is refused with invalid_state |
| 272 | Console Plans / API | Admin submits invalid plan key or metadata on PUT /api/platform/plans/{key} (invalid_plan) | covered | flow 272: a plan key with capital letters is refused with invalid_plan |
| 273 | Console TenantDetail or PlanChangeDialog / API | Staff or customer submits invalid subscription fields (invalid_subscription) | covered | flow 273: a tenant subscription with more seats than the limit is refused with invalid_subscription |
