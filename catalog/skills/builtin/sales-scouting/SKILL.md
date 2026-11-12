---
name: sales-scouting
description: "Use when you are the Scout of a Sales Loop squad: run the collectors for a lane's sources with multica sales collect, turn the raw items into normalised signals with a family and an entity key, qualify and score the organisations, write or update their sales_account documents, read the content of shared posts, and record each source's health. Covers the adapters, the scoring formula, and when to use a connector or a paid provider."
---

# Sales scouting

You find organisations that may need the lane's offer now, and you write what you found into the knowledge base. You do
not write messages, quotes or posts; Writer and Deal Analyst do that from your documents. Load sales-delivery-core first.
[reference.md](reference.md) has the adapter table, the family table by goal, and the scoring rubric.

## Run the sources

- The sources of a lane are `sales_source` documents with `status: active`. List them:
  `multica knowledge list --kb sales --kind sales_source --output json`, then keep the ones whose `## Record` says
  `status: active` and whose `lane` matches (a source with no lane serves every lane it lists in `## What it gives`).
- Read a source's `## Recipe` section: the JSON in its fenced block. Save it to a file and run the collector:

  ```bash
  multica sales collect --recipe-file <file.json> --source-id <sales_source id> --output json
  ```

  The output has `status` (`ok`, `degraded`, or `error`), `http_status`, `items_seen`, `field_coverage`, `warnings`, and
  `signals`. Each signal already has `family`, `entity_name`, `entity_key`, `url`, `observed_at`, `summary`,
  `raw_ref`, `region` and `sector_hint`.
- Run with `--dry-run` to check a source without writing anything. The health run (every morning) is always a dry run.
- A `collect` run that the tool refuses (a cap, a missing recipe field, a blocked address) is not retried in a loop. Record
  the refusal in the source's health and move on.
- A source whose method is `provider`, `social_connector`, `human_share` or `mailbox` does not run in `collect`. Those
  are yours to run through the tools the squad has (see "Connectors and providers" below).

## Qualify before you write

For each signal, in this order:

1. **Hard stops** (no score, no account): the organisation is already `won`, `lost` or `disqualified`; its `entity_key`
   has `do_not_contact: true`; it is outside the lane's ICP exclusions (`sales_icp`, Disqualifiers); it has no contact path
   that the lane's channel allows (you still record it as `watch` with the gap, so the gap is visible); the signal has no
   source.
2. **Dedupe.** Look up the entity key in the lane's accounts. Same organisation, new signal: add the signal to the
   existing document (a new version with `--supersedes`), do not create a second account.
3. **Score** with the formula in [reference.md](reference.md):

   ```
   score = 100 × (0.35·fit + 0.30·signal + 0.15·timing + 0.10·reach + 0.10·proof)
   ```

   Each term is between 0 and 1 and has its reason written next to it in the account's `## Fit` section. The thresholds
   come from the lane: `min_score` (default 60) for `qualified`, 40 to 59 for `watch`, below 40 dropped. A dropped
   candidate is listed with its reason in the scan's `sales_report`, so it is not searched again.
4. **Write the account** with `multica knowledge add --kb sales --kind sales_account` and the idempotency key
   `sales:account:<lane>:<entity_key>`. Its `## Record` carries the stage, the score, the family, the signal URL and date,
   and the contact channel with its `contact_source`. Its `## Why now` says why this signal makes the timing right, in
   two sentences at most.

## Family by goal

Signals are the reason to contact now, so the families a lane watches come from its goal, not from a fixed list.

| Goal | Watch first |
| --- | --- |
| `direct_sale` | hiring (the function that buys the offer), funding, expansion, launch, regulatory, pain, competitor's customer |
| `local_business` | expansion, directory (local listings), pain (reviews), events |
| `channel` | directory (associations), event (exhibitor lists), competitor's customer, trade flow |
| `tender` | tender, regulatory |
| `export_buyer` | trade flow, event, directory, news |

Hiring counts for any function. A hiring of a store manager is a signal for an offer on store operations; a hiring of a
CNC operator is a signal for a machine supplier's service offer. Read the role's title and description and name the
function in the Record (`signal_function`).

## Timing windows

| Family | A signal counts as new for |
| --- | --- |
| hiring | 30 days |
| funding | 90 days |
| expansion, launch, leadership_change | 60 days |
| regulatory | until the effective date, then 30 days |
| tender | until the closing date |
| pain, inbound, share | 30 days |
| directory, registry | 180 days |

## Shared posts (route F)

- A `sales_share` document holds the shared content. Read it, and classify its intent: `seeking` (someone asks for a
  solution), `complaint`, `announcement`, `hiring`, or `irrelevant`. Irrelevant shares are closed with `state: rejected`
  and a reason.
- Identify the organisation from the post's own organisation, its public website or its public registration. A page in
  the post that is a private profile is not an organisation. If no organisation is named or findable, do not create an
  account; the share keeps its one-line summary of the need.
- Score it as any signal (`family: pain` or `inbound` in most cases). A reply suggestion is written by Writer as
  `public_reply`, which the person posts. Never post, comment or message on your own.

## Connectors and providers

- **Rung 1 (the person's own accounts):** use the connected tools the squad's agent has enabled through its Composio
  allowlist. Read only what the connection covers (the page's inbox, its lead forms, its own comments). A connector
  that is not connected is not a reason to try another route to the same data.
- **Rung 6 (paid providers):** use only when the source's recipe names a provider and the budget for the squad and the
  source still has room. Run with the lowest item cap the recipe allows (`max_items`). Write the cost to the source's
  `cost_30d` after every run. A run that would pass the cap is not started; say so in the source's health.
- Never use a person's logged-in session, cookies or a private account. Rung 6 data is public content only.

## Source health

After each run, update the source document's Record with a new version (`--supersedes`):

- `last_run`, `last_http`, `hits_7d`, `hits_30d` (signals written in those windows);
- `qualified_30d` (accounts that became `qualified` from this source) and `cost_30d`;
- `status`: `active` stays active while `last_http` is 200 and at least one item was read; two consecutive failed runs
  set `degraded`; a source that stays degraded for three more runs is set `blocked` with the reason in its
  `## Probe log`.

A source that reads zero items with HTTP 200 for a week is `degraded` too: the page may have changed. Hand it to
sales-source-builder for a repair (at most two repairs a week).

## What you do not do

- You do not decide that an organisation is ready to buy. You score the signal and say what it is.
- You do not write a contact address you did not read on a page the organisation publishes.
- You do not run a source whose status is `needs_connector`, `manual_only`, `blocked` or `disabled`.
- You do not raise a cap. Stop at the cap and report it.

## Self-check

- Every account you wrote has its family, its signal URL and date, its score with the reasons, and a `contact_source`
  when a contact is recorded.
- Every collector run has a health update on its source document.
- No account was created from a private person's post, and no duplicate account exists for one entity key.
- Nothing you did sends, posts, logs in, or spends money beyond a cap.
