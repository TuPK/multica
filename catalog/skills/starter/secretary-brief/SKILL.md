---
name: secretary-brief
description: "Use for the Secretary's daily brief or weekly review autopilot run. Reads the boss's feedback first, then the digest, and writes and publishes the brief in the agreed section order."
---

# Secretary brief

The autopilot starts this run. It is a brief run, not a chat turn, so the boss has not
asked anything in this message. Your job is to write one brief the boss reads in two
minutes, decisions first.

## Steps

### 1. Read the feedback first

Before you read the digest or write anything:

```bash
multica brief feedback --recent 14d --json
```

Read every entry: the rating, its tags, its note, and the brief's date.

- Record a standing preference in memory when a note states one, or when the same tag
  or rating recurs across several entries. For example, "always lead with decisions".
  One unexplained "not useful" is not a preference.
- Let the ratings shape this brief's emphasis: `too_long` means shorter; `too_vague`
  means name the decision and its options; `missing_something` means check what the
  boss last asked about; `wrong` means re-check every fact you cite; `not_for_me` means
  leave that topic out unless the boss raises it.
- An empty list means the boss has not rated a brief. Do not invent a preference from
  silence.

### 2. Read the digest

For a daily brief:

```bash
multica brief digest --since 24h --json
```

For a weekly review, use `--since 7d`. Use the `since` the autopilot gives you when it
differs.

The digest is the source of status facts. Open an issue only when the digest flags it
and you need its detail to explain the item. Never scan the whole board. If the digest
is empty or fails, say so in the brief. Do not fill the gap with guesses.

### 3. Write the brief in this order

1. **Headline**: at most three lines. On a quiet day, "Nothing needs you today."
2. **Needs your decision**: each item with the situation, the options, your
   recommendation, and a link. Pending proposals first, then any blocker only the boss
   can clear. At most five items; count the rest.
3. **Progress**: by floor, only squads with news. A quiet squad is one line.
4. **Pipeline**: only when the digest's `pipeline` lists active initiatives. One line
   per initiative, in stage order: INI-<seq>, title, stage, days in stage, owner squad.
   Name each missing required artifact. Mark a stage `stalled` when the digest flags it.
   Leave the section out when empty. Read initiative detail with the read-only commands
   `multica initiative list --output json` and `multica initiative show INI-<seq> --output json`.
   You do not run `multica initiative attest`, `advance`, or `shelve`. When one of those
   should happen, say so under Needs your decision, with the initiative and the person
   who runs it. The vocabulary is in the reference.
5. **Goals**: only when the digest's `goals` lists goals. Each goal that changed state,
   and each at `at_risk` or `off_track`, with its cause and recommendation. Say so when
   the owner overrode the confidence. Leave the section out when nothing to report.
6. **Budget**: only when `budget.scopes` or `budget.holds` has entries. Each scope at or
   above its soft line, and each hold still in force. Leave the section out when both
   are empty.
7. **Blockers and risks**: each with a proposed way out. Report each `system_risks` line
   as a risk with its fix. Name each `routines_not_on_time` routine and each squad whose
   `charter.silent` is `true`, with a fix. Name each `overdue_requests` item by identifier
   and title, with its squad and `hours_overdue`. When `knowledge.retrieval.dropped` is
   `true`, report it as a risk.
8. **Watch**: market and product notes from the Research floor since the last brief.
9. **People**: only when `people.count` is above zero. Each review published since the last
   brief, with its subject, rating and recommendation. Leave the section out when there
   are no reviews.
10. **FYI**: cost, runtime or failure anomalies, and company knowledge. Write "N drafts
    await publishing" when `pending_drafts` is above zero. List `knowledge.stale_published`
    by title and knowledge base, and ask the boss to re-verify them. List each
    `auto_approved` entry as "Auto-approved by policy: <title> (<identifier>,
    <policy_name>)". List each `knowledge.auto_published` item since the last brief, and
    say these were published without a person.

Decision first, concise, readable in two minutes. A quiet day gets a short brief, not a
padded one. Link squads and issues so the boss can drill in.

### 4. Ask a leader only when the digest flags something you cannot explain

```bash
multica brief ask --squad <squad-id> --issue <issue-id> "<one short, specific question>"
```

At most three inquiries per workspace per UTC day. A refused ask returns
`inquiry_limit`; put that question in Blockers and risks instead of asking again. The
answer appears in the next digest's `inquiries`, with `answered` and `answer_excerpt`.
Do not look for it on the issue.

### 5. Publish

```bash
multica brief publish --kind daily --title "Daily brief" --file <file>
```

For the weekly review, use `--kind weekly --title "Weekly review"`.

## Rules

- Language: the language of the autopilot's description. Keep names, identifiers and
  commands as they are.
- Every factual claim names its source and the date you read it.
- Never store company facts in memory. Memory holds the boss's preferences only.
