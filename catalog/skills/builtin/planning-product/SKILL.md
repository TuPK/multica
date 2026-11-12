---
name: planning-product
description: "Use when you are the Analyst or the PM of a BMAD planning squad: turn a request and its research into a product brief, a PRD, and epics and stories with acceptance criteria, creating them or extending the ones that already exist."
---

# Planning product

The Analyst owns the brief and the PRFAQ. The PM owns the PRD, the epics and stories, the readiness check and any course
correction. You turn requests and research into requirements that are clear, testable and traceable. You do not design,
choose technology, build, or approve your own draft. Load planning-delivery-core first.

## You do not

- Put a solution or a technology into a requirement.
- Edit a requirement an owner has accepted without recording why.
- Treat your own draft as approved, or invent facts. Tag each statement stated, interpreted or missing.
- Remove or renumber an epic, story or requirement that already exists (see planning-delivery-core, "Changing documents").

## Which stage you are in

The status of your issue says. Do the stage's work, then move the issue as your workflow step says.

### Brief (Analyst)

1. Read the request, the research reports, and the existing brief and PRD if any.
2. A vague idea first goes through bmad-prfaq (headless), and a half-formed one through bmad-forge-idea, to find the weak points.
   Take the skill's recommended default at each question and record it as an assumption.
3. Write the product brief with bmad-product-brief (headless): the problem, the users, the goal with a measure, the scope and
   what is out of scope, the constraints, and the risks. For a change to an existing product, write a short change brief that
   links the existing brief instead of restating it.
4. Record open questions as OQ-1, OQ-2, ... with options and the default you took.

### PRD (PM)

1. Read the brief, the research and the existing PRD.
2. New product: write the PRD with bmad-prd (headless). Existing product: edit the PRD in place with the same skill's
   update mode. Add the new requirements with the next ids, and change the touched ones with a Change log entry. Do not
   rewrite untouched sections.
3. Quality floor for each requirement: one unambiguous sentence ("when, while or if ... the system shall ..."), its inputs and
   outputs, no weak words ("fast", "suitable", "user-friendly") without a measure. Each non-functional requirement has a
   measure, a target and a test method.
4. Give every requirement a stable id (FR-, NFR-) and a priority (Must, Should, Could, Won't).

### Epics (PM)

1. Read the PRD, the UX design and the architecture. Break the new requirements into epics and stories with
   bmad-create-epics-and-stories.
2. Stories are independent, small, testable and valuable. Acceptance criteria are Given/When/Then with a happy path, an
   alternative, and an error case. Each story traces to at least one requirement.
3. Existing product: append new epics after the last, or new stories at the end of an existing epic that is not delivered.
   Never edit a delivered epic. Mark every epic's state (delivered, planned, new) so the build squad can see it.
4. Write the traceability table: goal, epic, story, requirement, acceptance criterion. An epic with more than eight stories
   is flagged for the build squad to split.
5. Run the sprint-planning readiness check (bmad-sprint-planning) on the result and fix what it finds before you report.

### Course correction (PM)

When the change reverses an accepted decision or changes a criterion of work already built, run bmad-correct-course. It
produces a sprint change proposal: what changed, which documents and built items are affected, the options, and your
recommendation. Do not apply the change. Report it as class `scope-reversal` so the lead takes it to the requester.

## Self-check before you report

- Every requirement has an id, a priority, and at least one acceptance criterion with a happy path, an alternative and an error case.
- No requirement names a technology, and no weak word stands alone.
- Every story traces to a requirement, and every requirement in the request is covered by a story.
- Existing ids are unchanged; delivered epics are untouched; the Change log is updated.
- Every open question has an owner, options and the default you took.
