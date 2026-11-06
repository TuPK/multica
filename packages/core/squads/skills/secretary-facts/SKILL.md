---
name: secretary-facts
description: "Use when the boss asks a plain fact about the company that the digest holds, such as a count, a status, or what shipped or is late. Answers at once from the digest and says where the figure came from."
---

# Secretary facts

A fact is something the server already counts or records: a number, a status, a
date, a list of what shipped or is late. The digest holds these. Answer them from
the digest, at once, with its source. Do not ask the leaders for a fact the digest
holds, and do not explain a fact with a judgment you did not get from a leader.

If the question asks why, whether it is on track, or what it means, it is not a
fact. Stop and use `secretary-ask`.

## Steps

1. Read the digest for the period the boss asked about:

   ```bash
   multica brief digest --since 24h --json
   ```

   Use the period the boss named (for example `--since 7d`). If the boss named no
   period, use the default and say which period you used.

2. Find the part of the digest that answers the question. Read the whole entry, not
   just its first field.

3. If the digest's `mode` is `floors`, it lists no squads. Read each entry of
   `floors`: its `brief` is the floor chief's report for the day, or `null` when
   none came in. Drill into a floor with `multica brief digest --floor <id> --json`
   only when its `flagged` is `true`.

4. If the digest is empty, fails, or lacks the figure, say so in the reply. Do not
   fill the gap from memory or from an earlier brief.

5. Reply with the figure, the period, and the source, in one or two short sentences.
   Name any figure you could not confirm.

## Reply shape

> 4 issues are in progress in the Mua Ngon project, from the digest of 06 Nov 2026,
> last 24 hours. The digest does not show their age; ask if you want that.

## Rules

- Quote the figure exactly. Do not round it or change its unit.
- A figure that the digest marks as stale or partial is reported with that caveat.
- Never answer a fact from the knowledge base if the digest has it. The digest is
  current; the knowledge base may be out of date.
