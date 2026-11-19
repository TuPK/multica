# Platform console UI coverage matrix

Companion to [platform-management-flows.md](platform-management-flows.md) and [platform-management-e2e-gaps.md](platform-management-e2e-gaps.md). Generated against the console inventory (`discover-ui.mjs` over `platform-management/console/src`) and checked by `coverage-check.mjs` over `e2e/platform`.

Status: `covered` = a passing test exists; `fixme` = a test marks a product failure; `gap` = no UI test, with the reason in the last column.

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
| 23 | UserDetail.tsx | reset two-step sign-in opens its dialog | covered | staff opens the two-step sign-in reset |
| 23 | UserDetail.tsx | reset removes the MFA row and the notice email | gap | the reset dialog cannot be clicked (see fixme above); the notice email has no provider in the harness |
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
| 78 | EinvoiceFailures.tsx | retry, cancel or replace from the UI | gap | needs a live e-invoice provider; the provider is empty in the harness |
| 26 | Audit.tsx | empty state for an unknown actor | covered | the audit log filters by an actor id that has no events |
| 26 | Audit.tsx | columns and action filter | covered | the audit log shows its columns and the action filter narrows the list |
| 26 | Audit.tsx | date range filter | covered | flow 26: a From date after every event empties the audit list, and clearing it restores the list |
| 62 | Orders.tsx | orders screen | covered | flow 83: clicking Orders in the nav opens its own screen |
| 221 | Orders.tsx | expired order status label | gap | Orders is routed from the nav (flow 83); the Expired label only shows on that screen |
| 65, 66, 67 | Plans.tsx, PriceSchedule.tsx | plan and price schedule screens | covered | flow 83: the Plans screen opens from the nav and opens the price schedule |
| 83 | Layout.tsx | Orders and Plans nav fall back to Overview | covered | flow 83: clicking Orders and Plans in the nav opens their own screens |
| 146 | Layout.tsx | support nav hides restricted screens | covered | support sees only the screens its role allows |
| 146 | Layout.tsx | billing nav hides restricted screens | covered | billing sees the billing screens |
| 146 | App.tsx | admin opens Staff and Accounts | covered | an admin opens the Staff and Accounts screens |
| 146 | Layout.tsx | mobile layout without horizontal scroll (Tenants) | fixme | the Tenants screen fits a 375 by 667 phone |
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
