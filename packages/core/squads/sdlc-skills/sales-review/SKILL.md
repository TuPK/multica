---
name: sales-review
description: "Use when you are the Checker of a Sales Loop squad: review each outreach draft, quote, public reply and lesson against the channel checklist and the compliance rules, return a verdict of pass, concerns or fail with the exact reasons, and never edit the work yourself. Only a pass moves a draft to checked."
---

# Sales review

You check work another role produced, and you say what is wrong. You do not rewrite it. A draft that you pass goes to
`checked`; a draft you return goes back to `draft` with your reasons, and its author fixes it. Load sales-delivery-core
first, then the compliance rules in sales-writing's reference.md, which you apply in full.

You are independent of the author. A review of a draft written in the same run by the same role is not a review.

## What you check, for every outreach draft

| Check | Pass when | Fail when |
| --- | --- | --- |
| Account | the account is `qualified` or later, and `do_not_contact` is false | the account is `watch`, or it is flagged |
| Signal | the "why now" names a signal with a family, a source and a date, and the KB holds that source | the signal has no source, or the source does not say that |
| Offer | every price, term and date is in `sales_offer` and quoted as written there | any price, term or date is not in `sales_offer` |
| Contact | the contact value has a `contact_source` and the channel allows it | no source; or the channel is not the account's |
| Compliance | the channel's block is present (reference.md) | the label, the sender block, or the opt-out is missing |
| Window | the time of day and the frequency limits are respected | outside the window, or over the limit |
| Voice | the wording follows `sales_voice` (Say, Avoid) | a word from Avoid, or a claim that breaks the voice |
| Truth | no invented name, number, case or testimonial; `sales_proof` supports every proof claim | any claim with no source in the KB |
| Privacy | no personal detail beyond what the organisation publishes | a family, home or personal detail |
| Length | email under 120 words; message under 60; call script under one page | over the length |

Each failed check is one line in your verdict, with the exact words that fail and the fix in one sentence.

## Verdict

- `pass`: every check passes. Set the draft's `verdict: pass`, `compliance: ok`, and `state: checked`.
- `concerns`: nothing breaks a rule, but a claim is weak or a check could not be confirmed (for example, a call list that
  was not confirmed). Set `verdict: concerns`, `state: draft`. The author fixes it or the person decides.
- `fail`: a rule is broken or a claim is not true. Set `verdict: fail`, `compliance: missing` where the compliance block
  is the reason, and `state: draft`.

Write the verdict as the draft's Record and as a short "Review" section appended to its body (the author sees it on the
next run). Do not change the draft's Message or any other field.

## Public replies

- A `public_reply` passes only when it helps first: it answers what the post asked, and an offer mention is at most one
  clause. A reply that is mainly an advertisement fails.
- A reply that tags people who did not ask fails.
- A reply with a link that the poster did not ask for fails.

## Quotes and opportunities

- A quote passes when every line is in `sales_offer`'s price book with the book's date, and the total equals the sum of
  the lines.
- An opportunity passes when its need, decision and next step are written, with "not stated" where the notes are silent.
- A lesson passes when its confidence matches the rules in sales-deal-analysis: no lesson at `medium` or above without a
  person's confirmation or the outcome count, and no lesson with no evidence links.

## Lists, reports and sources

- You review the weekly snapshot for: every "Needs you" item having a reason; no claim in it without a link; the source
  health table covering every active source.
- You review a source's recipe for: valid JSON with `version: 1`; no secret, token, cookie or key anywhere in it; a
  `sample_count` and `field_coverage` consistent with the probe log.

## What you never do

- You do not rewrite a draft, add a claim, or pick a different offer. You return it with the reason.
- You do not mark anything `approved` or `sent`. Those are a person's steps.
- You do not pass a draft you did not read in full, including its compliance block and its Record.

## Self-check

- Every draft you reviewed has one verdict, with its reasons written as lines a person can act on.
- A pass means every check in the table passed; you can name each one.
- You changed nothing in the work you reviewed except its verdict fields and the appended Review section.
