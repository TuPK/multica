# Compliance block, limits and channel rules

These rules come from the squad's spec (section 10). They are the minimum the squad enforces. Whether a given use is
lawful is for a lawyer to confirm before the squad contacts Vietnamese recipients (gate G9); until then, sales-review
treats every rule here as a hard requirement.

## Email (advertising)

- Subject line starts with `[QC]` or `[AD]`, in the first position.
- Sender block before the opt-out line: the sender's name, phone, email, postal address, website, and social profile when
  there is one.
- Opt-out line at the end: how to refuse (a reply with the word "stop", a link, or a phone call), and the sentence that
  the refusal is honoured.
- A confirmation is sent once. Stop at the first refusal; write it to the account at once and set `do_not_contact`.
- Limit: at most 3 advertising emails to one address in any 24 hours.

## Messages (chat, SMS, Zalo, WhatsApp, Messenger)

- Time window: 07:00 to 22:00 local time.
- Identify the sender by name and organisation in the first message.
- Opt-out wording in the first message: "Reply STOP to stop receiving these messages."
- Stop at the first refusal, as for email.

## Calls

- Time window: 08:00 to 17:00 local time.
- Opening: name, organisation, the reason in one sentence, and "is this a good time?". Stop if the answer is no.
- Do not call a number on a do-not-call list. Lists are checked by the person before a call; a draft that does not confirm
  the check is `concerns`.

## Frequency and addresses

| Limit | Value |
| --- | --- |
| advertising emails to one address | at most 3 in 24 hours |
| messages to one contact per day | at most 1 |
| follow-ups in a sequence | at most 3, then stop |
| contacts with `do_not_contact: true` | 0 |

## Objections

- "Not interested": thank them, stop, record it. No further contact for 90 days.
- "Send information": send the offer's description from `sales_offer` only, and set a next action date for one week.
- "Who gave you my contact?": answer with the public page the contact came from (`contact_source`) and offer to stop.
- A request to delete or to stop: honour it at once, the same day, and write the record of the request.

## Compliance block template (email)

```
[AD] <subject>

<body>

---
Sender: <name>, <organisation>
Phone: <phone> | Email: <email>
Address: <postal address>
Website: <url>
To stop receiving these emails, reply "stop" or use this link: <opt-out link>. Your request is honoured at once.
```
