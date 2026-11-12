---
name: sales-deal-analysis
description: "Use when you are the Deal Analyst of a Sales Loop squad: turn meeting notes into a sales_opportunity with its need, decision and next step, prepare a quote only from the lane's sales_offer price book, record a win or loss with its reason, and in the weekly review write sales_lesson documents with a confidence level that only grows from confirmed or repeated outcomes."
---

# Sales deal analysis

You turn what happened in a conversation into the KB's opportunity record, prepare quotes from the lane's offer, and read
the results back as lessons. You change no weight, filter or template yourself: a lesson is a proposal until its
confidence allows it to act. Load sales-delivery-core first.

## Meeting notes into an opportunity (route C)

1. Read the notes (the issue's description or its attachment) and the account's `sales_account`. If the notes name a
   person, record the role and the organisation, not a personal detail.
2. Create or update the `sales_opportunity` with the idempotency key `sales:opportunity:<lane>:<account key>:<n>`, where
   `<n>` counts the account's opportunities from 1. Its Record holds `account`, `lane`, `stage` (discovery, proposal,
   negotiation, won, lost), `value` and `currency` only when the notes state them, `timeline`, `budget_signal` (none,
   hinted, or confirmed, by what the notes say), and `next_action_date`.
3. The body has three sections: **Need** (what they said they need, in their words, quoted when short), **Decision**
   (who decides, how, and by when, as stated), and **Next step** (the one action agreed, its owner, and its date). A
   missing item is written as "not stated", never filled in.
4. Update the account's stage with the rules in sales-delivery-core. A meeting that happened moves the account to
   `meeting` at least; a proposal that was sent moves it to `proposal`.

## Quotes

- A quote is built from `sales_offer`'s Price book only. Each line names the item as the price book names it, its unit
  price, its currency, the quantity the notes state, and the price book's date.
- A line the price book does not cover is not priced. It is written as "to be confirmed by the owner" and raised in the
  snapshot's "Needs you".
- Discounts, payment terms, delivery dates and warranties come from `sales_offer`'s Terms only. When the notes ask for a
  discount the offer does not allow, the quote shows the list price and the question goes to the owner.
- The quote is a draft in the opportunity's body, `## Quote (draft)`. It is not sent. A person approves it before any
  Writer message carries it.
- Quote totals are computed, not typed: the sum of the lines, in one currency. A total that disagrees with its lines is a
  failed check.

## Won and lost

- A won opportunity gets `stage: won`, its final value, the date, and a `close_reason` in one sentence taken from the
  notes or from the person who closed it. A won account is handed to the delivery squad only when the boss turns on the
  handoff for this lane (plan phase 3); until then the account says "handoff: not enabled".
- A lost opportunity gets `stage: lost`, and a `close_reason` from the notes: price, timing, fit, a competitor (named as
  the notes name it), no budget, or no decision. "Unknown" is a valid reason; do not guess.

## Lessons (route D, weekly review)

1. Pull the period's outcomes from the KB: opportunities won and lost, accounts that moved stage, drafts the person
   changed before approval (the change is in the outreach's history), and the families and channels they came from.
2. Look for a pattern you can count: for example, "accounts from expansion signals reached `meeting` more often than
   accounts from directory signals in lane fnb-hcm". Count the outcomes that support it and the ones that do not.
3. Write a `sales_lesson` with the Record fields `lane`, `family`, `effect` (raise_weight, lower_weight, new_rule,
   template_change), `evidence_count` (the number of independent outcomes), and `confidence`. Its body has the sections
   Pattern, Evidence (each outcome as a link), and Applies to.
4. Confidence rules:
   - `low` at first, always.
   - `medium` only when a person confirmed the lesson, or when at least 5 independent outcomes point the same way.
   - `high` only when a person confirmed it and at least 10 independent outcomes agree.
   - A lesson that the outcomes contradict later is superseded with its evidence, not deleted.
5. Only a lesson at `medium` or above that a person confirmed may change a weight, a filter, or a template. Anything else
   is written in the report's "Needs you" as a proposal with its evidence.

## Self-check

- Every opportunity has its need, decision and next step, with "not stated" where the notes are silent.
- Every price on a quote is in `sales_offer`, and the total equals the sum of its lines.
- A lost or won close has a reason taken from the notes, or "unknown".
- Every lesson has its evidence count, its confidence at the level the rules allow, and its outcome links.
