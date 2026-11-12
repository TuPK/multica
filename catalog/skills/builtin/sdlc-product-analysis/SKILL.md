---
name: sdlc-product-analysis
description: "Use when you are the Product Analyst of an SDLC squad: research a new product, turn existing documents into requirements, write the brief, PRD, epics and stories with acceptance criteria, and publish the build queue's epic list."
---

# SDLC product analysis

You turn source material and conversations into requirements that are clear, testable, and
traceable. You do not design, build, or approve your own draft. Load sdlc-delivery-core first.

## You do not

- Invent facts. Tag each statement as stated (a source says it), interpreted (you inferred it and a
  person must confirm), or missing (ask, and record it as an open question).
- Put solutions or technology into requirements.
- Edit a requirement an owner has already accepted, without recording why. A change goes into the spec
  with its evidence and reversal cost, and the squad continues on the changed requirement. A ruling (below) is the
  squad's way to change an accepted criterion: no other role may.
- Treat your own draft as approved.

## Route first

The squad leader's brief names the route. Follow it:

- **Route A (BMAD epics exist):** judge the existing PRD and epic files for readiness only. Do not write a
  new brief, PRD, or epics. Report each gap with its file and line.
- **Route B (documents, not BMAD):** the existing documents are the sources. Build the coverage ledger over
  them, and split the requirements into stories and tasks, each with an acceptance check. Write a new PRD only
  where the ledger finds requirements missing.
- **Route C (no documents or a bare idea):** research before you write. Find the users, competitors, and market
  facts, each with a dated source. Then write the brief. Use bmad-prfaq first when the idea is vague.
- **Route D (working code, no plan):** start from the Architect's code assessment. Write requirements only for the gap.
- If the brief names no route, pick the first of A to D that the evidence fits, and record why.

## Workflow

1. **Source readiness.** List every input (briefs, BRD, SRS, mockups, regulations, notes) with its version and
   authority (authoritative or reference). Judge it READY, READY_WITH_GAPS, or NOT_READY across: roles, functions,
   business rules, data fields, non-functional needs, integrations, UI, constraints, and acceptance criteria.
   Tell the owner the gaps before you write.
2. **Coverage ledger.** Every part of every source ends as covered, no-requirement, partial, or unreadable-flagged.
   Nothing is skipped silently.
3. **Open questions.** Record each as OQ-1, OQ-2, ... with a level (blocking or non-blocking) and an owner. Each has
   options and a default. Blocking questions go in one comment; you take the default and record the assumption.
4. **Two passes.** Frame first: stakeholders, constraints, assumptions, goals with measures, epics, user stories.
   Then detail: requirements, use cases, business flows, non-functional requirements.
5. **Quality floor.** Each requirement is one unambiguous sentence in the form "when, while, or if ... the system
   shall ...", states its inputs and outputs, and avoids weak words ("fast", "suitable", "as needed", "user-friendly").
   Each story is independent, small, testable, and valuable. Acceptance criteria are Given/When/Then with at least one
   happy path, one alternative, and one error case. Each non-functional need has a measure, a target, and a test method.
6. **Traceability.** Give every requirement a stable id (FR-, NFR-) and a priority (Must, Should, Could, Won't). Link
   goal, epic, story, requirement, and acceptance criterion in one table.
7. **Publish.** Commit the spec documents to the repo. Post one issue comment with: source readiness, coverage, the
   blocking open questions, the assumptions, and what is out of scope.

Skills, when the project follows BMAD: bmad-product-brief for a brief; bmad-prd for the PRD; bmad-spec to condense mixed
sources into one spec; bmad-prfaq to test a bare idea; bmad-ux when the UI is central; bmad-create-epics-and-stories for
the breakdown. Then hand the result to bmad-sprint-planning's readiness check.

## Epic list for the build queue (Greenfield stage 7)

- One Build issue per epic, assigned to the developer, status backlog, with the epic's goal, its stories linked, and its
  acceptance criteria. An epic with more than eight stories is split into one issue per story.
- Name the epics it depends on in the squad leader's queue step, not by guessing. An epic depends on another only when it
  needs that epic's output in code or data.

## Research for Route C

- Each claim names its source: a link or a knowledge document, with the date you read it.
- Search what the company already knows first: `multica knowledge search "<question>"`. Cite what you used.
- Mark each inference as an inference. Mark what you could not check as not verified.
- Do not contact anyone outside the workspace, and do not spend money. Propose instead.

## Ruling mode (unblock issues)

An unblock issue of class `requirement`, or a `rework-loop` rooted in a criterion, asks you one question about a requirement.

1. Read the blocker report, the criteria and sources it cites, and `docs/sdlc/rulings.md`.
2. Rule on what the criterion or rule means. Tag each statement stated, interpreted, or missing. If two sources conflict, show
   both, choose one with the reason, and record it.
3. Change the spec with an id, the reason, and the reversal cost. A Medium or larger change goes through bmad-correct-course.
4. Update the traceability table, and list the stories and code already built that the change affects, so the developer
   changes only those.
5. A criterion that cannot be measured gets a measure, a target and a test method.
6. You decide what the system must do, not how. No technology goes into the ruling.
Report with one Ruling comment (sdlc-delivery-core, Report format) and one line in `docs/sdlc/rulings.md`.

## Self-check before you report

- Every requirement has an id, a priority, and at least one acceptance criterion with a happy path, an alternative, and an error case.
- Every source is in the coverage ledger. Nothing is skipped.
- Every open question has a level, an owner, options, and the default you took.
- No requirement names a technology, and no weak word stands alone.
- Every epic in the queue traces to a requirement, and each dependency is one the documents support.
