# Route playbooks and report templates

Read this after SKILL.md. It holds the detail you need when you triage a request.

## Greenfield routes, and the stages each one creates

Planning stages (create the ones your route names, all up front; stage 2 and later as backlog):

1. Product Analyst: product research (users, competitors, market facts, each with a dated source), then the product brief
   (bmad-product-brief; add bmad-prfaq when the idea is vague).
2. Product Analyst: PRD (bmad-prd).
3. UX Designer: UX design (bmad-ux). Skip when the product has no UI.
4. Architect: architecture (bmad-architecture), then project context (bmad-project-context).
5. Product Analyst: epics and stories (bmad-create-epics-and-stories).
6. Code Reviewer: planning readiness check (the readiness gate of bmad-sprint-planning). On CONCERNS or FAIL, send each
   named gap to the member who owns that document, then run stage 6 again. At most twice.
7. Product Analyst: publish the build queue (see SKILL.md step 6).

| Route | Stages created | Notes |
| --- | --- | --- |
| A. BMAD epics exist | 6 and 7 | Stage 6 checks the existing PRD, architecture, and epic files. A failing check falls back to route B. |
| B. Documents, not BMAD | 1 (ledger), 2 only if requirements are missing, 3 only if no UI design, 4 (code state against the documents), 5, 6, 7 | Stage 1 is the coverage ledger of the existing documents; it writes no new brief. |
| D. Working code, no plan | 1 (Architect code assessment), 2 (requirements for the gap), 5, 6, 7 | Stage 1 finds what runs and what is missing, with file and line. |
| C. No documents or bare idea | 1 to 7 | Stage 1 starts with product research. |

Checking order: A, B, D, C. A route that is not chosen creates no sub-issues.

## Maintenance routes

Triage first. Then take the route:

- **A. Hotfix, or a bug that is Trivial or Small:** stage 1 Bug Fixer, stage 2 Code Reviewer, stage 3 QA Tester.
- **B. A bug with an unclear cause or a wide effect:** stage 1 Architect (root cause and impact; add Security Reviewer when
  auth or data is involved), stage 2 one implementation sub-issue per component for the Bug Fixer or Developer, then
  review, QA, and security stages.
- **C. Feature or enhancement:** stage 1 Product Analyst (requirements and acceptance criteria; add UX Designer when the UI
  changes), stage 2 Architect (design delta; skip when there is none), stage 3 one implementation sub-issue per story for the
  Developer, in dependency order, then review, QA, and security stages.
- **D. Optimization:** stage 1 Architect (measure the baseline and set a target), stage 2 Developer, then QA compares
  against the baseline.
- **E. Upgrade or upkeep:** stage 1 Architect (risks and rollback plan), stage 2 Developer or DevOps, then QA runs the regression.

Rules for every Maintenance route:
- Run sub-issues of a stage in parallel only when they touch different files.
- Every change gets an independent Code Reviewer sub-issue and a QA Tester sub-issue after it. High-risk changes also get a
  Security Reviewer sub-issue.
- A Medium-sized or larger bug fix, or a changed requirement, goes through bmad-correct-course before anyone builds it.
- A blocking review or QA verdict sends the work back: a fix sub-issue for the owner, then the check again. At most twice,
  then the request is blocked and the requester is mentioned.

## Market research (not an SDLC squad)

Use only for the research squad: the scope comes from the issue or the knowledge base, research members report with dated
links, a critic challenges the evidence, and proposals are filed with `multica proposal create --squad "Market Research"`.

## Report templates

Final report on the request (Greenfield or Maintenance):

```
Outcome: <one line>
Built or changed: <what, and where, with paths and commit ids>
Evidence: <each verification with its result and link; anything not verified>
Decisions and assumptions: <id, decision, why, reversal cost>
Open risks and follow-ups: <risk, owner, and what is needed>
How to run and roll back: <commands and steps>
```

Route comment (step 2):

```
Route: <A | B | C | D | Maintenance route and risk tier>
Evidence read: <paths and what each showed>
Stages created: <numbers and owners>
Assumptions: <id, decision, why>
```
