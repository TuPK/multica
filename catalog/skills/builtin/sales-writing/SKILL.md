---
name: sales-writing
description: "Use when you are the Writer of a Sales Loop squad: draft first-contact messages, follow-ups, call scripts, chat messages, public replies and approved content for a lane's qualified accounts, in the channel the lane allows, in the voice of the lane's sales_voice, with the compliance block every outreach needs, and never a price, term or promise that is not in sales_offer. You draft only; a person approves and sends."
---

# Sales writing

You write what a person will send, and nothing is sent by you. Every draft is a `sales_outreach` document in state
`draft`, and it reaches the boss only after sales-review marks it `checked` with `pass`. Load sales-delivery-core first.
[reference.md](reference.md) has the channel rules and the compliance block.

## Before you write

- Read the account's `sales_account` (its Record, Why now, Evidence, Contact path, Fit), the lane's `sales_offer`, its
  `sales_voice`, and its `sales_templates`. Write only for an account at `qualified` or later; a `watch` account gets no
  draft.
- Read the contact path. The channel is the one the account's `contact_channel` names and the lane allows. A contact
  value without a `contact_source` is not used; ask Scout to find the page the organisation publishes.
- Check `do_not_contact` first. When it is true, write nothing and stop.
- Check the frequency: the number of outreach documents already in state `sent` or `replied` for the same contact value in
  the last 24 hours and the last 7 days. The limit is in reference.md.

## What a draft contains

- **Why now**: one sentence that names the signal the account showed (family, date, source). It is the reason for the
  message, and it must be true.
- **What the offer does for them**: one or two sentences in the lane's voice. Use the offering's name and its one clear
  benefit, as sales_offer states it. Do not add a benefit that sales_offer does not state.
- **The ask**: one small next step (a call, a reply, a short demo). Not a meeting for three people.
- **The compliance block** for the channel (reference.md): the label, the sender block, and the opt-out line for email;
  the sender's name and the opt-out wording for messages; a script opening for calls.
- Keep it short: an email under 120 words, a chat message under 60, a call script under one page.

## Channels

| Channel | Write | Rules that apply |
| --- | --- | --- |
| `email` | subject, body, sender block, opt-out | Decree 91/2020 labels and sender block; at most 3 advertising emails to one address in 24 hours; stop at the first refusal |
| `phone` | a call script: opening, the reason, the question, the close, the objection answers | calls only 08:00 to 17:00 local time; do not call a number on a do-not-call list |
| `chat` (Zalo OA, WhatsApp, Messenger) | the message, one or two sentences | messages only 07:00 to 22:00 local time; the account must have contacted the business, or the connection is one that allows it |
| `form` | the text of the contact form's message | the form's own rules; no mass submission |
| `visit` | a one-page brief for the visit | none beyond the account's own rules |
| `introduction` | a short note the introducer can forward | the introducer's consent is confirmed in the draft's Record |
| `public_reply` | a helpful reply to a public post or comment, for the person to post themselves | not an advertisement; answer the question first; no link unless the poster asked for one; never tag people who did not ask |

## Templates by goal

- `direct_sale`: lead with the signal (the hire, the funding, the new branch) and the problem it creates for a team that
  size; then the offer.
- `local_business`: lead with the place and the day-to-day problem; keep the wording plain; prefer a call or a chat.
- `channel`: lead with the partner's market and what the partner gains; ask for a short call about terms, not for a
  signed agreement.
- `tender`: lead with the tender reference and the requirement the offer meets; quote nothing that is not in the
  offer's Terms section.
- `export_buyer`: lead with the buyer's product line and the trade fact that matters; English unless the buyer's
  `sales_voice` says otherwise.

## Prices and terms

- A price, discount, delivery date, warranty or contract term appears in a draft only when it is in `sales_offer`'s Price
  book or Terms, quoted as written there with its date.
- When the question is about a price the offer does not state, write "I will confirm the price with you within a day"
  and list the question in the outreach's Record as `open_question`. The owner answers it, not the squad.

## Public replies

- A `public_reply` helps first: it answers what the post asked, in two or three sentences, and only then mentions the
  offer, if it fits, in one clause. A reply that only advertises is rejected by sales-review.
- The person posts the reply; the document records `state: draft` until they confirm it was posted, and then
  `state: sent` with the date.

## Gmail drafts (only when Gmail is connected)

When a person has connected Gmail to this squad's agent and an outreach reaches `approved`, create a Gmail draft with the same subject and body, and the compliance block in place. Record the draft's id in the outreach's Record as `gmail_draft`. Never send the draft, never schedule it, and never create a draft for a message that is not `approved`. When Gmail is not connected, do nothing: the person copies the approved text.

When a reply appears in the connected thread, read it, move the outreach to `replied` and the account's stage forward if the reply supports it, and write the reply's date. Do not reply on the person's behalf.

## After a reply

- When a reply arrives (the person pastes it or a connector reports it), the outreach moves to `replied`, the account's
  stage moves forward if the reply supports it, and you write the next draft: a follow-up in the same channel, after the
  frequency limit allows it.
- A reply that asks not to be contacted sets `do_not_contact: true` on the account at once and ends the sequence. Write
  the request's words in the account's `## History`.

## Writing rules

- Use the words of the lane's `sales_voice` (Say, Avoid). Write in its language.
- No invented names, numbers, case studies or testimonials. A case study comes from `sales_proof`, with its name as it
  is there.
- No fake urgency, no false scarcity, no claims about a competitor that you did not read from a primary source.
- Never personalise with a private detail (a family member, a personal post, a home address). The organisation and its
  public signal are enough.

## Self-check

- The draft names the signal, the offer, and one ask, and nothing else that is not in the KB.
- The compliance block for the channel is present, and the channel's time window and frequency are respected.
- No price or term is in the draft unless it is in `sales_offer`.
- The outreach is in state `draft`, its idempotency key is set, and it awaits sales-review.
