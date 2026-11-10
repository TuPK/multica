# Planning squad lead: reference

## Roster and ownership

| Member | Owns | Skills it runs |
| --- | --- | --- |
| Blueprint Researcher | Research reports | planning-research, bmad-deep-recon |
| Blueprint Analyst | Product brief, PRFAQ | planning-product, bmad-product-brief, bmad-prfaq, bmad-forge-idea |
| Blueprint PM | PRD, epics and stories, sprint change proposal | planning-product, bmad-prd, bmad-create-epics-and-stories, bmad-sprint-planning, bmad-correct-course |
| Blueprint UX Designer | UX design | planning-ux, bmad-ux, frontend-design |
| Blueprint Architect | Architecture, ADRs, project context | planning-architecture, bmad-architecture, bmad-project-context |
| Blueprint Reviewer | Readiness verdict | planning-readiness, bmad-review, bmad-advanced-elicitation |

Assign only to members on your Squad Roster, by the id in the member's mention link there. A piece that fits no member is split
across the members who are on the roster, or the request is set to blocked with the gap named.

## Issue description template

```
Goal: <one sentence>
Request: <identifier and title>
Mode: N | X
Impact map part: <the rows of docs/planning/impact-<id>.md this issue covers, or "all" in Mode N>
Inputs: <paths and links: the request, earlier reports, existing documents>
Outputs: <paths, and for Mode X which sections change>
Skill: <the skill the owner runs, for example bmad-prd in update mode>
Must not change: <documents, ids and delivered work that stay as they are>
Acceptance check: <what the next stage or the reviewer will verify>
This run is unattended. Do not wait for approval; take the recommended default and record it as an assumption.
```

## Impact map (Mode X)

Save as `docs/planning/impact-<request identifier>.md`.

```
# Impact map: <request identifier> <title>
Request in one sentence: <...>
Existing documents read: <paths>
Delivered work: <epics and stories already delivered, with evidence>

| Document | Touched? | Sections or ids affected | Why | Owner |
| --- | --- | --- | --- | --- |
| Research | yes/no | ... | ... | Researcher |
| Brief | ... | | | Analyst |
| PRD | ... | FR-12 changes; FR-40..FR-43 new | ... | PM |
| UX design | ... | Screen S-7 changes; S-21 new | ... | UX Designer |
| Architecture | ... | ADR-9 superseded by ADR-14 | ... | Architect |
| Epics | ... | Epic 6 gets stories 6.5, 6.6; Epic 11 new | ... | PM |

Decisions that still hold: <ADRs and requirements the request leaves alone>
Decisions that change: <and whether the change reverses an accepted decision or a built criterion: yes/no>
Stages skipped: <stage>: <reason>
Must not change: <list>
```

A "yes" on "reverses an accepted decision or a built criterion" means `scope-reversal`: see Step 2 of the skill.

## Planning handoff packet

Post as one comment on the request after a PASS verdict. Every field is required; write "none" where it does not apply.

```
## Planning handoff
Request: <identifier> <title>
Outcome: <one line>
Mode: N | X
Documents changed: <path> (<what changed, ids touched>) ...
New epics: <epic id, title, number of stories> ...
Changed epics: <epic id, stories added or changed> ...
Delivered and untouched: <epic ids>
Readiness verdict: PASS (review issue <identifier>)
Assumptions: <ids in docs/planning/assumptions.md>
Open items for implementation: <list or "none">
Build hint: <epics to build in order; known risks; checks that need an environment>
```

## Report templates

Plan comment (Step 2): mode, evidence paths, impact map link, stages chosen with their issues, stages skipped with reasons,
folder mode, assumptions.

Request for the requester (Step 6): one comment on the request that mentions them: the question, the options with what each
costs, your recommended default, and what you will do if they stay silent (never for money, a contract, or deleting data).
