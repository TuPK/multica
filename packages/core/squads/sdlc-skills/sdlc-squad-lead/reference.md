# Plans and report templates

Read this after SKILL.md. It shows what a plan looks like as issues, and the templates for your comments.

## Folder mode decides the order

Read it first (`multica project resource list <build> --output json`, `resource_ref.execution_mode`).

- **`in_place`: one issue at a time.** The plan is one chain. Every issue waits for the one before it. The examples below
  show the dependencies each item needs; in this mode you also chain the items that could otherwise run together.
- **`worktree`:** the dependencies shown are all there is, and items without a dependency between them may run together.

An unblock issue (SKILL.md Step 7) is outside the chain: it waits for nothing, because the issue it unblocks is parked.

In `in_place` mode Plan A looks like this:

```
E2  --start-status todo                       (no --after)
E3  --start-status todo              --after E2
E4  --start-status todo              --after E3
QA  --start-status qa                --after E4
SEC --start-status security_review   --after QA
OPS --start-status release_prep      --after SEC
```

## Plan A: BMAD epics exist

One issue per undelivered epic, assigned to the Developer, start status `todo`. An epic waits for the epics its epic file
names as dependencies and for nothing else. Then the final verification.

```
E1  Epic 1 (delivered)                       created as done, evidence linked
E2  Epic 2   Developer  --start-status todo   --after E1 (done)    -> starts at once
E3  Epic 3   Developer  --start-status todo   --after E2
E4  Epic 4   Developer  --start-status todo   --after E2           (worktree mode only: runs with E3 when they touch different code)
QA  Final QA        QA Tester          --start-status qa               --after E3 --after E4
SEC Security review Security Reviewer  --start-status security_review  --after QA
OPS Release notes   DevOps             --start-status release_prep     --after QA
```

With an environment bring-up, it is the first issue: `ENV  --start-status environment` (no `--after`), and E2 waits for it.

## Plans B, C and D: planning first

The planning issues, queued behind each other. When the readiness review has passed, create the epic issues as in Plan A.

```
P1  Research and brief          Product Analyst  --start-status analysis      (Plan C; Plan B writes a requirements ledger,
                                                                               Plan D starts with the Architect's code assessment)
P2  PRD and requirements        Product Analyst  --start-status analysis      --after P1
P3  UX design                   UX Designer      --start-status design        --after P2   (only if the product has a UI)
P4  Architecture                Architect        --start-status architecture  --after P2   (in_place mode: --after P3)
P5  Epics and stories           Product Analyst  --start-status analysis      --after P3 --after P4
P6  Planning readiness review   Code Reviewer    --start-status plan_review   --after P5
```

Register the wakeup on the request for P6 (SKILL.md Step 4). On PASS create the epic issues. On CONCERNS or FAIL create one gap
issue per gap, queue a new review behind them, at most twice.

## Maintenance routes

Triage first, as one comment on the request: the type (bug, feature, optimization, upgrade, upkeep), the risk tier
(Lightweight, Standard, High-risk), the route, and the assumptions. High-risk means auth, money, data migration or deletion, a
public contract, a security control, or a production release. Then create the issues of the route:

- **A. Hotfix, or a bug that is Trivial or Small:** Bug Fixer (`todo`), then Code Reviewer, then QA Tester.
- **B. A bug with an unclear cause or a wide effect:** Architect (root cause and impact; add Security Reviewer when auth or data
  is involved), then one implementation issue per component for the Bug Fixer or Developer, then review, QA, and security.
- **C. Feature or enhancement:** Product Analyst (requirements and acceptance criteria; add UX Designer when the UI changes),
  Architect (design delta; skip when there is none), then one implementation issue per story for the Developer in dependency
  order, then review, QA, and security.
- **D. Optimization:** Architect (measure the baseline and set a target), Developer, then QA compares against the baseline.
- **E. Upgrade or upkeep:** Architect (risks and rollback plan), Developer or DevOps, then QA runs the regression.

Rules for every route:
- Issues that touch different files may run together; give them no dependency on each other.
- Every change gets an independent Code Reviewer issue and a QA Tester issue after it. High-risk changes also get a Security
  Reviewer issue.
- A Medium-sized or larger bug fix, or a changed requirement, goes through bmad-correct-course before anyone builds it.
- A blocking review or QA verdict sends the work back: a fix issue for the owner, then the check again. At most twice, then the
  request is blocked and the requester is mentioned.

## Report templates

Plan comment (Step 2):

```
Plan: <A | B | C | D | Maintenance route and risk tier>
Folder mode: <in_place | worktree>
Evidence read: <paths and what each showed>
Issues created: <identifier, owner, start status, waits for>
Readiness: <environment | architecture | requirements | UX: created, or skipped and why>
Assumptions: <id, decision, why>
```

Final report on the request:

```
Outcome: <one line>
Built or changed: <what, and where, with paths and commit ids>
Evidence: <each verification with its result and link; anything not verified>
Decisions and assumptions: <id, decision, why, reversal cost>
Open risks and follow-ups: <risk, owner, and what is needed>
How to run and roll back: <commands and steps>
```
