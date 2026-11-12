# Team playbook

Read this after SKILL.md when you route a problem. It says who owns what, which member answers which kind of question,
and the templates for an unblock issue and for a ruling. The class list is the one in sdlc-delivery-core.

## Who owns what

| Member | Owns | Decides | Answers questions about | Start status |
| --- | --- | --- | --- | --- |
| Product Analyst | requirements, acceptance criteria, epics and stories, traceability | what a requirement means, scope, business rules | `requirement` | `analysis` |
| UX Designer | screen specs, states, copy, accessibility design | flows, states, user-facing wording | `ux` | `design` |
| Architect | architecture, ADRs, contracts, data model, component split | technology and structure, trade-offs | `design`, root cause of a `rework-loop` | `architecture` |
| Developer | code for an epic or story | local implementation choices | nothing: it builds | `todo` |
| Bug Fixer | one defect, its root cause and fix | the smallest fix | `defect` | `todo` |
| Code Reviewer | independent review, plan readiness | a verdict | `review-capacity` | `plan_review` (plan or second opinion), `in_review` (a build) |
| QA Tester | test design and execution, final verification | GO, CONDITIONAL GO, NO GO | `verification` | `qa` |
| Security Reviewer | threat models, access control, secrets, dependencies | what controls are needed; never a waiver | `security` | `security_review` |
| DevOps | environment, tooling, packaging, release | how to build, run and check the environment | `environment`, `tooling`, `capability` probe | `environment` (environment, tooling), `release_prep` (release) |

## Route a blocker

| Class | Member | `--start-status` | What a good ruling contains |
| --- | --- | --- | --- |
| `environment` | DevOps | `environment` | Running environment, `docs/sdlc/environment.md`, an idempotent `env-ensure` command, health output |
| `tooling` | DevOps | `environment` | The tool installed or replaced, or an equivalent way of working, shown running |
| `requirement` | Product Analyst | `analysis` | The criterion or rule clarified in the spec with an id, reason, reversal cost, and the built parts it affects |
| `design` | Architect | `architecture` | An ADR Accepted by the squad, frozen contracts, the smallest change plan for the developer |
| `ux` | UX Designer | `design` | Updated screen specs, states and copy |
| `security` | Security Reviewer | `security_review` | The controls needed, the evidence, an owner for each fix; no waiver |
| `verification` | QA Tester | `qa` | An executable check plan, or a finding that the criterion cannot be measured, sent to the Product Analyst |
| `defect` | Bug Fixer, or the Architect when the reach is wide | `todo`, or `architecture` | Root cause and the smallest fix, or an impact analysis |
| `rework-loop` | The owner of the root cause: Architect, Product Analyst or QA Tester | by member | The cause, a change of approach, and a test or criterion that ends the loop |
| `review-capacity` | Code Reviewer (a new run that does not see the first verdict) | `plan_review` | An independent verdict on the same commit range |
| `capability` | DevOps probes first | `environment` | The probe result and the exact change a person must make to the runtime or host |
| `irreversible` | A person | proposal | The decision |

An owner decision in a member's comment beats the table: record `Owner waiver O-<n>` (comment id, covered, still in
force, expiry), drop the controls it makes unnecessary, and resume without a new round.

A `tooling` blocker that names failed `check-harness` ids (a `gate` check: H1, H3 or H4) belongs to the Architect, start status `harness`
(sdlc-harness, Mode 3), not to DevOps. Only a missing Ensure command goes to DevOps.

Order when there are several causes: `requirement`, then `design` and `ux`, then `environment` and `tooling`, then `verification`.

## Substitutes when the roster lacks the member

The roster is the only source of assignees. When no member exists for a class:

- `environment`, `tooling`: the Developer, with the issue stating "environment mode" and the DevOps rules of
  `docs/sdlc/environment.md`. Say in your comment that you used a substitute.
- `design`: the Code Reviewer does a design review, and the Developer records the result as a Proposed ADR.
- `requirement`, `ux`, `security`, `capability`: no safe substitute. File a proposal with a recommended default.

## Unblock issue template

```
Title: [Unblock <identifier>] <class>: <the question, short>

Unblock ruling for <mention://issue/<id> of the blocked issue> (<class>, round <n> of 2)
Question: <from the blocker report>
Evidence: <the blocker report, file and line, command output>
Do not change: <acceptance criteria, accepted decisions, applied migrations, ...>
Output: <the ruling artifacts the class table names> and one Ruling comment (sdlc-delivery-core, Report format)
Resume: put the blocked issue in <status> and run <Resume-check>
Load sdlc-delivery-core and the skill of your role, and follow its Ruling mode.
This run is unattended. Do not wait for approval; take the recommended default and record it as an assumption.
```

## Resume comment template

```
Ruling R-<n> (<class>) applied: <one line>.
Read: <artifact paths>
Run first: <command>
Do not change: <criteria, decisions>
```

## Files the lead keeps

| File | Content |
| --- | --- |
| `docs/sdlc/capabilities.md` | Capability preflight: need, epic that uses it, minimum evidence level, who can provide it |
| `docs/sdlc/release-conditions.md` | `RC-<n>`, condition, level needed, why it has not run, who can open it, source |
| `docs/sdlc/parked.md` | Parked issue, reason, question, default, deadline |

Request for the boss, one comment that you edit in place:

```
Waiting for you (edited <time>)
| Item | What is needed | My default if you stay silent | Default applies at |
| ---- | -------------- | ----------------------------- | ------------------ |
```
