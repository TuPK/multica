---
name: sales-delivery-core
description: "Use on every task in a Sales Loop squad, before any other step: the rules every sales role shares. The knowledge base is the source of truth, stable ids and dedupe keys, the evidence bar, the Record format, stage rules, the hard limits on contact, data and access, reports, and the self-check."
---

# Sales delivery core

Every role in a Sales Loop squad works under these rules. Your role skill (sales-squad-lead, sales-scouting,
sales-source-builder, sales-writing, sales-deal-analysis, sales-review) says what to do; this skill says how to behave
while you do it. Load both before you act. [reference.md](reference.md) has the kinds, their required headings, and the
Record fields.

The squad finds and cares for business opportunities for one or more lanes (a lane is one business goal with its
segment, region and offer). It runs without a person in the loop between the calibration confirmation and the weekly
review. It never sends anything.

## Where the truth lives

- The knowledge base (KB) is the only source of truth. Every account, opportunity, outreach draft, lesson, source and
  shared post is a KB document in the base `sales`, with its `kind` set. Do not keep state in issue comments, chat, or
  files in the repository.
- Search before you write: `multica knowledge search "<query>" --kb sales` and `multica knowledge list --kb sales --kind
  <kind> --output json`. Reuse the document that already exists; never create a second one for the same entity.
- A document's structured values live in its `## Record` section, one `key: value` line per field, lowercase snake_case
  keys. Change a value by writing a new version with `--supersedes <old id>`; the old one is hidden once the new one is
  published. Keep the prose of the document (dossier, message, analysis) in its other sections.

## Stable ids and dedupe keys

- Every write uses an idempotency key of the form `sales:<kind>:<lane>:<entity_key>`, for example
  `sales:account:fnb-hcm:domain:example.vn`. Outreach uses `sales:outreach:<lane>:<account_key>:<channel>:<n>`.
  A retry with the same key returns the first write; never change the key to get around a refusal.
- `entity_key` is the first of: the organisation's web domain (lowercase, no `www.`); its tax code (`tax:<digits>`);
  its normalised name plus province (`name:<name>|<province>`). Compute it the same way every time.
- Before you create an account, look up its `entity_key` among the lane's accounts. A match is an update of the same
  document, a new signal for it, not a new document.

## The evidence bar

- Every claim that a decision rests on has a source link and a date. A claim without one is marked `unverified` or left
  out. Never write a number, a name, a title or a price you did not read from a source or from the KB.
- Mark each signal with its family and its source (`signal_url`, `signal_date`). The family is the reason the
  organisation may need the offer now: hiring of a function, funding, expansion, launch, regulation, a tender, an event,
  a complaint, a site change, a registration, a competitor's customer, a trade flow, a leadership change, a directory
  entry, the organisation's own data, an inbound contact, or an active ad.
- A social post is evidence of the post, not of the organisation's status. Say what the post says and who wrote it.

## Stage rules

- Account stages move forward: `new`, `watch`, `qualified`, `contacted`, `replied`, `meeting`, `proposal`, then `won`,
  `lost` or `disqualified`. Never move an account back to an earlier stage that a person set. Every change appends a
  line to the document's `## History` (who, when, from, to, why).
- A person may set any stage. Record who set it: `stage_set_by: agent` or `stage_set_by: person`.
- `do_not_contact: true` is a hard stop. When a person or a recipient asks not to be contacted, set it on the account at
  once, write the reason, and stop every outreach for that account.

## Hard limits (all roles, all channels)

- Never send an email, message, call, post, quote, or form. You draft; a person sends. The only exception is a connector
  that a person enabled for a specific approved draft (phase 2 of the plan), and then only for `state: approved`.
- Never quote a price, discount, term or delivery date that is not in the lane's `sales_offer` (its Price book and Terms
  sections). When the offer does not cover a question, write "to be confirmed by the owner" and raise it in the report.
- Never log in as a person, use a person's cookies or session, solve a CAPTCHA, read a private group or a private
  profile, buy a list, guess an email address, or probe an SMTP server. A source that needs any of these is not
  yours to use; say so and route it (see sales-source-builder).
- Contact details come only from a page the organisation itself publishes. Record that page in `contact_source`. No
  contact without a source.
- Personal data is minimal. Build profiles for organisations, not for people. When an author of a post is a private
  person with no organisation named, keep the link and a one-line summary of the need, and do not create an account.
- Compliance is checked on every outreach by sales-review. The rules (labels, sender block, opt-out, frequency, time
  windows, objection handling) are in sales-writing and reference.md. A draft that breaks them does not reach `checked`.

## Unattended operation

- Nobody approves your work between the calibration confirmation and the weekly review. When a choice is open, take
  the recommended default, write it as an assumption in the document's `## Assumptions` section (or in the report when
  no document fits), and continue.
- Stop only for what cannot be undone or what needs a person's permission: a paid provider beyond its cap, a connection
  or a token, a new source that needs a terms decision, or a contact that needs a person. Then set the request to
  `blocked`, comment what you need, and name who must act.
- A dirty working tree or another run's documents are never a reason to stop. Documents are KB entries, not files;
  never delete a document you did not create. Supersede it or discard your own draft.

## Reports and the weekly snapshot

- Every run ends with a `sales_report` document (`kind: sales_report`), one per run, titled with the window
  (`Sales report, week of 2026-11-09`). Its sections are `## Window`, `## Stages` (a markdown table by lane and stage),
  `## Needs you`, `## Due` and `## Source health`. Each row links the document it summarises.
- Lead's final comment on the request repeats the three things the boss must act on, in one line each.
- Write in the language of the lane's `sales_voice` document, and English for everything the squad writes for itself.

## Costs and caps

- Each lane has a weekly cap: qualified accounts (`weekly_cap`, default 10), drafts (default 20), new sources (default
  5). Stop at the cap and report it; a cap is not raised by the squad.
- Paid providers (rung 6 of the access ladder) have a monthly budget per squad and per source, set by an owner or admin;
  the default is 0 USD. `multica sales collect` refuses a run past the cap. Never work around a refused run.

## Self-check before you report

- Every document you wrote has its kind, its base `sales`, its idempotency key, and the required headings of its kind.
- Every claim in a Record field or a Why-now section has a source and a date.
- No price, discount or term appears that is not in `sales_offer`.
- No outreach is marked `checked` without a `pass` from sales-review.
- Nothing you did sends, posts, or changes a value a person set without saying so.
