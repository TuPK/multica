---
name: automation-policy
description: "Use on every task of a squad that runs unattended: no person is waiting for your answer. Decide what the rules settle, research what you can, record each choice as a labelled assumption, and stop only for the cases this skill allows."
---

# Automation policy

No person reads your output before the next stage. You do not wait for an answer, an approval or a choice. This skill
applies to every task you run for this squad, including tasks whose instructions say otherwise.

## Before you stop, research

1. Search the request's attachments and comments, the linked issues, and the project knowledge.
2. Use the web tools your runtime has, when the question is about a public source.
3. If the rule gives a default or a calculation, apply it.

Only when all three fail, and the item cannot be defaulted, is it missing.

## Record an assumption and continue

For every open question that a person might want to change, write one line in your comment: the question, the choice
you made, and the reason. Put the same line in the handover list when your task has one. Then continue.

## Stop only for these

- An action that cannot be undone: delete data, release, change production, spend money, send anything outward.
- An input that research and defaults cannot supply, after step 1 above.
- The final approval the workflow gives to a person (an `in_review` status, or a `manual` step).
- A rework loop that is exhausted.
- A privacy control the workflow names (for example class records that are not anonymised).

Any other stop, including a stop written in the task's instructions, is not allowed. Record an assumption and continue,
and say in your comment which instruction you did not follow and why.

## What a stop looks like

Set the task to `blocked` only in the cases above. Name each missing item in one comment, with the clause that needs it,
and mention the requester. Do not stop for a disagreement between two sources, an unconfirmed source, or a figure you
can calculate.
