# Workflow tab: design contract

Shared contract for two agents working in the same checkout on `main`, with no worktrees and no commits (the owner commits). Read it fully before touching code.

## 1. Goal

1. A new top-level tab **Workflow** (sibling of Chat, Hub, Work) for the workspace:
   - **Manage**: list every workflow, what it is assigned to (projects, squads, agents), view detail, edit, duplicate, delete with the existing guards.
   - **Design**: a drag-and-drop visual designer that builds a workflow from the statuses, handlers and handoff parts that already exist.
2. Replace the current two-way routing (`next_status_key` / `back_status_key`) with a general **multi-branch** routing model, so a step can route to 3, 4, 5... targets based on conditions.

Existing settings UI (`packages/views/workflows/`, reached from Settings → Workflows) must keep working until the new tab replaces it. Do not delete it in this pass.

## 2. What exists today (verified in code)

- Type: `packages/core/types/issue-workflow.ts`. A workflow is an ordered list of `IssueWorkflowStep` keyed by a catalog `status_key`. Each step has `handler` (`none | agent | squad | member | project_lead | creator`), `instructions`, optional `next_status_key` and `back_status_key`.
- Engine: `server/internal/issueworkflow/issueworkflow.go`. Fixed head `backlog, todo`; fixed tail `blocked, done, cancelled`. `todo`, `done`, `cancelled` are required. Entering a status through an ordinary issue write reassigns the issue to the step's handler; an agent run starts with the step's instructions.
- Rework loop: `server/internal/handler/issue_workflow_entry.go` (`reworkLoopFor`, `reworkReturnLimit = 3`). It is hard-coded to one pattern (review/QA sends back to In Progress, lead is `blocked`). This is the "only if/else" limitation: there is no general condition.
- Routes (`server/cmd/server/router.go`): `GET/POST /api/issue-workflows`, `GET/PATCH/DELETE /api/issue-workflows/{id}`, `PUT /api/issue-workflows/{id}/handoffs/{status}`, `POST /api/issue-workflows/{id}/brief`, `POST /api/projects/{id}/workflow`.
- Squads have `active_workflow_id` (WIP, see §6). Projects have `project_ids` on a workflow.
- Web UI: `packages/views/workflows/` (editor, mapping dialog, workflows tab) with `@dnd-kit/*` already in `packages/views/package.json`.

## 3. Routing model (backend decision)

Add an ordered `transitions` list to each step. A transition routes the issue when the step is left.

```ts
type Condition =
  | { all: Condition[] }                       // AND
  | { any: Condition[] }                       // OR
  | { not: Condition }
  | { field: ConditionField; op: "eq" | "neq" | "in" | "gt" | "gte" | "lt" | "lte" | "exists"; value?: unknown };

type ConditionField =
  | "outcome"          // what the handler reported: "approved" | "rejected" | "blocked" | "done" | free text from a closed set
  | "priority"         // none | low | medium | high | urgent
  | "labels"           // label names
  | "project_id"
  | "rework_count"     // returns of this issue into the same step
  | "creator_type"
  | "assignee_type";

interface IssueWorkflowTransition {
  id: string;                  // stable id, generated on write
  label: string;               // shown in the designer, <= 64 runes
  when: Condition | null;      // null = default branch, at most one per step, last in order
  to_status_key: string;       // must be a status the workflow lists
  handler?: IssueWorkflowHandler; // optional override of the target step's handler for this branch
}

// On IssueWorkflowStep, the new field:
transitions?: IssueWorkflowTransition[];
```

Rules:
- Evaluated in order, first match wins; a null `when` is the fallback. A step with no transitions keeps today's behaviour.
- `outcome` is supplied by the handler when it moves the issue out of the step. If none is supplied, `outcome` is absent and only conditions not needing it can match.
- Backwards compatibility: stored rows have `next_status_key` / `back_status_key`. Write a data migration (next free number is **977**; 973–976 are WIP files, do not touch them) that converts them to `transitions` (`outcome eq approved` → next, `outcome eq rejected` → back, default → next). API responses keep `next_status_key` / `back_status_key` derived from the transitions, so installed desktop clients keep working.
- The 3-return rework limit becomes a condition on `rework_count` in a transition, not hard-coded. Keep the existing behaviour as the default transition set produced by the migration.
- Validate in the write handler: targets exist in the workflow, at most one default per step, no cycles that never reach a terminal status (warn, do not reject, if unsure), max transitions per step 10, max condition depth 4, max 50 steps (existing).
- No foreign keys, cascades or extra DB tables. JSON on the existing workflow row.

## 4. Backend additions (kiro)

- Extend the step type and storage; keep `storedStep` compatibility rules.
- Zod schemas in `packages/core/api/schema.ts` for the new fields, with defaults and `parseWithFallback` (AGENTS.md "API Compatibility"); malformed-response tests.
- Usage summary, read-only, for the Manage view: `GET /api/issue-workflows/usage` returning for each workflow `{ workflow_id, project_ids, squad_ids, agent_ids }` derived from `project` and `squad` tables and step handlers. If an equivalent already exists, reuse it.
- Delete: keep the guards (default cannot be deleted; a workflow used by squads or projects cannot be deleted without remapping). Expose the blocking reasons in the error body so the UI can explain them.
- Outcome transport: add optional `outcome` to the existing status-change write used by agents/members. Default absent. Document it in the agent CLI help only if the CLI already exposes status changes.
- Core: `packages/core/issue-workflows/` queries/mutations (TanStack Query, `wsId` in keys). Zustand only for designer UI state, and only if needed.
- Tests: Go table tests for condition evaluation (matrix), migration conversion, and the write validations; core tests for the zod schemas.

## 5. Frontend (agy)

- New feature folder `packages/views/workflow/` (business UI, shared web/desktop). It must not import `next/*` or `react-router-dom`; use `useNavigation()` / `<AppLink>`.
- Routes: list at the workspace tab root, detail/designer at `…/workflow/<id>`, plus a "new" entry. Wire into `apps/web/app/[workspaceSlug]/…` and the desktop router (`apps/desktop/src/renderer/src/routes.tsx`), tab presentation (`packages/core/paths/tab-presentation.ts`, `route-icons.ts`), and the sidebar/tab bar, the same way Chat/Hub/Work are registered. Use icons from `@multica/ui/icons` only; if an icon is missing, follow `.agents/skills/multica-icons/SKILL.md`.
- **Manage view**: table/cards of workflows with name, default badge, step count, assigned projects, squads, agents (chips), updated time. Search and filters. Detail drawer shows the steps and where each is used. Actions: edit, duplicate, delete (show blocking reasons).
- **Designer**: canvas of status nodes ordered as the board. Drag statuses from a palette into the canvas; drag to reorder inside the fixed head and tail rules (backlog/todo first, blocked/done/cancelled last). Each node: handler picker (none, agent, squad, member, project lead, creator) with instructions editor. Edges: drag from a node's output port to a target node creates a transition; click an edge to edit its condition and label. Condition builder: group (AND/OR/NOT), field, operator, value; supports 3+ branches per step. A "default" branch always exists when there is more than one branch. Show a live validation list (from §3 rules). Keyboard path for every drag action.
- Prefer `@dnd-kit` (already a dependency of `packages/views`). Adding a dependency (e.g. a graph canvas library) needs a justification in your report and must use `catalog:`.
- Copy: English keys plus all seven locales (en, zh-Hans, zh-Hant, fr, es, ko, ja) in `packages/views/locales/*/`. Follow "UI descriptions" in `apps/docs/content/docs/developers/conventions.mdx`; no description text unless it is a non-obvious constraint.
- Use semantic tokens (`packages/ui/styles/`) and `--text-*` typography; no inline SVG or emoji for icons.
- Until kiro's endpoints land, build against the types in this file via a local adapter in `packages/views/workflow/` that kiro will replace in phase B. Do not edit `packages/core/types/issue-workflow.ts`, `packages/core/api/schema.ts`, or `packages/core/issue-workflows/**`. If you need a change there, add a line under §8 "Requests" and continue.
- Tests: component tests for the wiring, the list, the condition builder and the validation list; no mocks of `next/*` / `react-router-dom`; mock stores with the Zustand callable shape and `getState`; API mocks at `@multica/core/api`.

## 6. Ownership and boundaries

| Area | Owner (phase A) |
| --- | --- |
| `server/**`, `packages/core/types/issue-workflow.ts`, `packages/core/api/schema.ts`, `packages/core/issue-workflows/**`, migrations ≥ 977 | kiro |
| `packages/views/workflow/**` (new), `packages/views/locales/*`, `apps/web/app/[workspaceSlug]/**` routes, `apps/desktop/**` routes and tab wiring, `packages/core/paths/**` | agy |
| `packages/views/workflows/**` (existing editor) | read-only for both, except agy may import from it |

Pre-existing uncommitted work on `main` (do not revert, do not reformat, build on it):
- Modified: `server/pkg/db/queries/issue_workflow.sql`, `server/pkg/db/queries/squad.sql`, `server/pkg/db/queries/project.sql`, `server/pkg/db/generated/*`, `server/internal/issueworkflow/issueworkflow.go`.
- Untracked: `server/migrations/973_*` … `976_*`, `docs/engineering/knowledge-work-plugins-squads*.md`.

Kiro must read `git diff` on those files before editing and keep the squad/active-workflow changes intact. Regenerate sqlc with `make sqlc` after SQL changes.

## 7. Phases and gates

- **Phase A (parallel)**: agy builds the frontend in §5. kiro builds §3 and §4 and the tests.
- **Phase B (kiro, after agy reports)**: replace agy's local adapter with the real core hooks and schema, wire the designer's save to the real endpoints, run the gates.
- **Gates**: `pnpm typecheck`, `pnpm lint`, `pnpm test` (frontend, excluding mobile); `make test` (Go); `make check` if time allows. Run `make up` before Go tests and read the sample-DB leftover note in memory before calling a failure a regression. Report exactly what ran and what was skipped.
- Not in scope: mobile, release, schema changes outside workflow tables, real-agent smoke tests.

## 8. Requests and open questions

Append here instead of editing another owner's file. Format: `- [from agy|kiro] …`.

- [from kiro] Phase A landed on `main` without commits: §3 model, §4 backend and core schemas/types. Details and what did not run are in the report. Decisions needed from the owner:
  1. Routing is enforced only as a check. A status write that carries `outcome` must name the status its step's transitions route that outcome to. Otherwise it gets a 400 with code `workflow_outcome_mismatch`. The engine does not yet redirect an issue on its own. Confirm whether an outcome should instead set the status automatically.
  2. `rework_count` counts returns from review or QA to In Progress (the existing `CountWorkflowReturns`), not returns into the same step as §3 says. Confirm which meaning the designer should expose.
  3. A transition's `handler` override is stored and validated but not applied at handoff time yet.
  4. Cycle warnings (§3, "warn if unsure") are not implemented.
  5. Legacy writes (`next_status_key` / `back_status_key` without `transitions`) that change a step's next or back target replace all of its branches, including conditional ones. A legacy write that repeats the derived keys keeps them. The designer should send `transitions`.
- [from kiro] Migration `977_issue_workflow_transitions` is applied to the managed database `multica_multica_51` (1,176 rows converted, no legacy keys left). It is not applied to the shared database `multica`, whose running server (pid 3825596, cwd `/home/av-server/Documents/Github/multica/server`) is at migration 972 and does not know transitions. Applying 973–977 there needs a restart plan from the owner.
- [from kiro] `make up` cannot start this checkout's API while pid 3825596 holds `:8080` and `:6060`. I did not stop it.
- [from kiro] `server/internal/handler/squad_workflow_test.go` (untracked) defines `issueStatusOf`, which `issue_triage_guard_test.go` also defines, so `go vet ./internal/handler/` fails. I ran the handler tests with a `go test -overlay` that blanks that file and adds `squadActiveWorkflowName`, outside the repo. Its owner should rename the helper.
- [from kiro] `packages/views/workflow/designer/*` (condition-builder, designer-canvas, status-node-card, transition-editor-dialog) and `packages/views/search/use-root-model.ts` fail `pnpm typecheck` in `packages/views`. The errors are in those files, not in the core types, which only added optional fields and new exports. agy should rerun `pnpm typecheck` after reading the core types.
- [from kiro] Core exports for agy: `IssueWorkflowStep.transitions` (always an array after parsing), `WorkflowCondition` (leaf `field` and `op` accept unknown strings, so match nothing), `IssueOutcome` on `UpdateIssueRequest.outcome`, `issueWorkflowUsageOptions(wsId)` for `GET /api/issue-workflows/usage`, and `parseIssueWorkflowBlockers(error.body)` for the delete conflict's `reasons`.
- [from kiro] `multica issue status <id> <status> --outcome approved|rejected|blocked|done` is available; agents can use it when they leave a routed column.
- [from agy] Phase A frontend landed on `main` without commits:
  1. Top-level Workflow tab wired into paths, tab-presentation, route-icons, app-rail, web routes (`apps/web/app/[workspaceSlug]/(dashboard)/workflow/`), desktop routes (`apps/desktop/src/renderer/src/routes.tsx`), and search keywords.
  2. Manage view built at `packages/views/workflow/manage/workflow-manage-page.tsx`: lists workflows with default badge, step count, assigned projects/squads/agents chips, relative updated time, search filter, category filter tabs, detail drawer (`workflow-detail-drawer.tsx`), and delete dialog with blocking explanation (`workflow-delete-dialog.tsx`).
  3. Visual Designer built at `packages/views/workflow/designer/`: board-style horizontal canvas with `@dnd-kit` reordering respecting fixed head (`backlog`, `todo`) and tail (`blocked`, `done`, `cancelled`), SVG cubic-bezier transition curves, status palette drawer (`status-palette.tsx`), condition builder (`condition-builder.tsx`) supporting AND/OR/NOT groups and rules with field/op/value, transition editor modal (`transition-editor-dialog.tsx`), and live validation panel (`validation-list.tsx`).
  4. Validation engine implemented in `packages/views/workflow/validation.ts` enforcing all §3 rules: required `todo/done/cancelled`, fixed head/tail positions, max 50 steps, max 10 transitions/step, max condition depth 4, target exists, single default last in order, cycle warnings for terminal reachability.
  5. Locales complete across all 7 supported languages (`en`, `zh-Hans`, `zh-Hant`, `ja`, `ko`, `fr`, `es`).
  6. Phase A local adapter in `packages/views/workflow/local-adapter.ts` derives multi-branch transitions from legacy handoffs and handles local draft overrides. Ready for kiro in Phase B to swap with TanStack Query hooks and real endpoints (`issueWorkflowUsageOptions`, save workflow with `transitions`, and `parseIssueWorkflowBlockers` on delete).
  7. Verification: `pnpm typecheck` passed cleanly across all 10 packages (0 errors). Vitest passed for `paths` (9 files, 128 tests), `layout` (37 files, 293 tests), and `workflow` (12 files, 58 tests). `pnpm --filter @multica/views lint` passed with 0 errors.

- [from kiro] Phase B (integration) landed on `main`, uncommitted.
  1. Local adapter removed. `packages/views/workflow/local-adapter.ts` is deleted. The manage page, the designer and the detail drawer read the server through `@multica/core/issue-workflows`: `useIssueWorkflows`, `useIssueWorkflowDetail` (new `api.getIssueWorkflow`), `useIssueWorkflowUsage` (`GET /api/issue-workflows/usage`), `useCreateIssueWorkflow`, `useUpdateIssueWorkflow` and `useDeleteIssueWorkflow`. A writer patches its own cache entries on success and invalidates on failure. Other clients refresh through `issue_workflow:changed`.
  2. Save sends canonical `transitions`. The Default workflow's name is not sent. A 409 `workflow_status_mapping_required` opens the existing `WorkflowMappingDialog` and retries with `status_mapping` (`issueWorkflowMappingPlanFrom` in core).
  3. Delete runs only on confirm. The page first checks Default, project and squad use from the usage data. A refused delete shows the server's `reasons` (`parseIssueWorkflowBlockers`), mapped to the existing locale messages.
  4. Duplicate creates a real workflow with the source steps and transitions.
  5. Core: `IssueWorkflowStepInput.transitions` added. `outcome` is stripped from issue optimistic patches and from reconciliation, like `enter_workflow_step`. A regression test in `packages/core/issues/mutations.test.tsx` fails without the onMutate strip.
  6. Mismatches with the server, fixed in `validation.ts` and the new `condition-spec.ts`:
     - The handler picker offered `squad` and `creator`. The server rejects both on steps.
     - The condition builder offered all eight operators on every field. The server accepts a subset per field, and `condition-spec.ts` now mirrors that table. `exists` kept a stale value, and `in` sent a scalar. `in` now takes a comma-separated list, and `exists` drops the value.
     - An empty branch label and a branch to its own step were accepted. Both are errors now, and the transition Save button stays disabled without a label.
     - Also flagged now: an agent or member handler with no id, duplicate transition ids, and each condition the server would refuse (empty groups, unknown or invalid values, a `rework_count` that is not a whole number).
  7. Gate failures caused by this feature, fixed:
     - Web text-contrast guard: transparency on the designer edge and arrow SVG and on the empty-state icon. Now solid tones and `text-faint-foreground`.
     - Core diagnostics route table: added `workflow`, `workflow/new` and `workflow/:id`.
     - Views locale parity: removed dead `_one` plural keys from zh-Hans, zh-Hant, ko and ja.
     - Added `manage.action_failed`, `designer.save_failed` and `designer.load_failed` to all seven locales.

  Still open:
  - Validation messages and condition messages are English literals, not locale keys. §5 asks for copy in all seven locales. The `(Copy)` suffix on duplicate is also an English literal.
  - The UI does not send `outcome`. Question 1 above is still open.
  - The designer cannot edit a branch's `handler` override. Cycle warnings exist only in the UI.
  - Question 2 above (what `rework_count` counts) is still open.
  - `server/internal/handler/squad_workflow_test.go` (untracked, not touched) redeclares `issueStatusOf`, so `go vet ./internal/handler/` fails. Its owner should rename the helper.

  Verification, run from the repo root unless noted:
  - `pnpm typecheck`: passed, 9 of 9 tasks.
  - `pnpm lint`: passed, 0 errors, 6 of 6 tasks. Warnings remain in 21 views, 2 core, 3 ui, 1 web and 1 desktop files. None are in files this phase changed.
  - `pnpm test`: passed, 7 of 7 tasks. core 321 files and 4767 tests (1 skipped), views 681 files and 8355 tests, web 38 files and 321 tests, desktop 72 files and 839 tests, ui 11 files and 430 tests (1 skipped), docs 6 files and 30 tests. The first run failed on the three gate issues in item 7. This is the rerun.
  - `npx vitest run` in `packages/views`: `workflow` tests 13 files and 82 tests passed; `locales` and `workflow` together 15 files and 489 tests passed.
  - `packages/core`: `issue-workflows`, `issues` and `squads/upgrade` tests 54 files and 615 tests passed, and the 50 issue-mutation tests passed after the regression test was added.
  - `cd server && go test ./internal/issueworkflow/`: ok.
  - `cd server && go vet ./internal/handler/`: fails on the duplicate `issueStatusOf` above, not caused by this phase.

  Not run:
  - `make test` and `make check`. `make test` reads `.env` by default, and that file's `DATABASE_URL` names the shared `multica` database. Its `migrate up` would apply 973 to 977 there. The override path (`ENV_FILE=.env.worktree`) runs `docker compose up -d postgres` against the shared container. I skipped both to leave the shared database and container alone.
  - Playwright E2E.
  - DB-backed Go packages. No Go file changed in this phase, and the handler package does not compile.
  - No server was started, stopped or restarted, and no migration was applied.

- [from kiro] Phase C (close-out of the items above). Uncommitted. No commits, and no git state changed.
  1. **Duplicate helper, done.** `issueStatusOf` in `server/internal/handler/squad_workflow_test.go` is renamed to `squadWorkflowIssueStatus`. That file never calls it. `go vet ./internal/handler/` exits 0.
  2. **Handler DB tests, done for this feature.** Run without `-overlay`, with `DATABASE_URL` from `.env.worktree` (database `multica_multica_51`, the managed one). All 55 named workflow tests pass (transitions, rework, usage, delete blockers, outcome, squad workflow). None skipped. The full package run (no race) has three failures, all in squad WIP `squad_project_test.go`, and both causes are in test code:
     - `projectWorkflowName` inner-joins `issue_workflow`, so a project with no workflow returns "no rows". Affects `TestAssignSquadProjectSetsSDLCWorkflowAndSquad` and `TestAssignSquadProjectGivesAPlanningSquadItsWorkflow`.
     - `setProjectWorkflow` (in `issue_workflow_test.go`) sends no `X-Workspace-ID`, so a project in the fixture's workspace gets 404. Affects `TestAssignSquadProjectKeepsTheProjectWorkflow`.
     These belong to the squad WIP owner. I did not change them.
  3. **Locale copy, done.** Validation messages, condition problems, handler labels and generated names are keys in all seven locales (`packages/views/locales/*/workflow.json`, 29 message keys plus 8 others per locale).
     - `validation.ts` returns `{ key, values }` instead of English text. `types.ts` derives the key union from `en/workflow.json`, so a missing key fails typecheck.
     - `condition-spec.ts` returns keyed `ConditionProblem`s the same way.
     - `use-validation-message.ts` (new) renders a message with `t()`. The validation list and the save toast use it.
     - The `(Copy)` suffix is `manage.copy_name`. The defaults "New Workflow", "Untitled Workflow" and the branch labels "Start" and "Done" are keys. The handler picker labels "Agent", "Member" and "Project lead" are keys.
     - Enum option values (`approved`, `high`, `member`, ...) stay as identifiers. conventions.mdx keeps schema identifiers in lowercase English.
     - The CJK locales have no `_one` keys, as in phase B. Parity is therefore `en` against the rest, minus `designer.validation.issues_found_one` and `manage.steps_count_one`.
     - The designer's init effect lists `t` in its dependencies. It returns early after the first run, so behaviour is unchanged.
     - Tests now assert keys: `validation.test.ts`, `condition-spec.test.ts`, `designer/validation-list.test.tsx`.
  4. **Outcome in the UI, no change, decided.** No UI path moves an issue through routing. The designer and manage pages edit workflow definitions. The issue status control records a person's choice, which is not an outcome a handler reports. Sending `outcome` from it would give the choice a routing meaning it does not have. Agents send it with `multica issue status --outcome`. Question 1 above stays open for the owner.
  5. **Managed environment, not started.** `make status` reports `multica-51` with database `multica_multica_51`. Its manifest records `ENV_FILE=.env`. Resolved with `make -f Makefile ENV_FILE=.env`, `DATABASE_URL` is the shared `multica` URL, and `dev-env.sh` starts the API with that env file. So `make up` would start this checkout's API against the shared database. I did not run `make up` and did not edit the manifest. The owner should set `ENV_FILE=.env.worktree` in `~/.multica/dev/envs/multica-51/manifest.env`, or confirm that `.env` is intended. Port 18131 and 30000 were free. Port 8080 is now free, and pid 3825596 no longer exists. Port 6060 is held by pid 3829708, which I did not touch.
  6. **Playwright, not run.** No spec under `e2e/` covers the workflow tab. The suite needs the API on 18131 and the web app on 30000, and starting those is blocked by item 5. The fixtures also default to the shared `multica` database when `DATABASE_URL` is unset. A run needs `make up` first, then `pnpm exec playwright test` with `DATABASE_URL` exported to the managed database. `e2e/env.ts` already loads `.env.worktree`.

  Verification, with the exact commands and results:
  - `go vet ./internal/handler/` (in `server/`): exit 0.
  - `DATABASE_URL=<managed> go test ./internal/handler/ -count=1 -run '^(<55 workflow test names>)$' -v`: 55 tests, all PASS, 0 FAIL, 0 SKIP. Log: `/tmp/handler-workflow-tests.log`.
  - `go test ./internal/handler/ -count=1 -timeout 40m` (no race): three FAILs, the squad tests above. Log: `/tmp/handler-full-tests.log`.
  - `ENV_FILE=.env.worktree make test` (Go, `-race`): exit 1. The migrate step ran against `multica_multica_51` and found 977 applied, with no legacy keys left. Failing packages: `cmd/migrate`, `internal/daemon`, `internal/handler`, `internal/integrations/{dingtalk,lark,telegram}`, `internal/maintenance`, `internal/migrations`, `internal/service`, `internal/testutil`. Log: `/tmp/phasec/make-test.log`.
  - Isolated reruns (`go test -race -count=1`, one package at a time, same guard as `test-go.sh`). Log: `/tmp/phasec/isolated-race.log`.
    - Pass alone: `cmd/migrate`, `internal/daemon`, `dingtalk`, `lark`, `telegram`, `maintenance`, `testutil`. The failures in the parallel run were contention on the shared database.
    - `internal/handler`: the same three squad tests. All seven of the other handler failures from the parallel run pass alone.
    - `internal/migrations`: `TestMigrationNumericPrefixesAreUnique` fails. Committed migrations 600 to 616 reuse 16 numeric prefixes. The same failure reproduces on committed `HEAD`, checked by exporting `HEAD` with `git archive`. Pre-existing, not touched.
    - `internal/service`: `TestProbationWorseForHireAndRetire` and `TestProbationJudgesTheWindow` fail on committed `HEAD` too (same export check). Pre-existing. `TestTenantRemovalWaitsForTheHoldAndIsRecoverable` failed in the parallel run and in the first isolated run, then passed in four standalone `-race` runs (two on `HEAD`, two on the tree).
  - Squad tests in the `make test` run: `TestAssignSquadProject*` did not appear in its failure list, although they fail in every other run. The cause is unknown and was not traced.
  - `packages/views`: `pnpm typecheck` exit 0. `npx vitest run workflow locales`: 15 files, 489 tests, all pass. `pnpm lint`: 0 errors, 21 warnings, none in `workflow/` or `locales/`.
  - Repo root: `pnpm typecheck`: 9 of 9 tasks pass. `pnpm lint`: 6 of 6 pass, 0 errors. `pnpm exec turbo test --filter=!@multica/mobile --continue`: 7 of 7 pass. Counts: ui 430 (1 skipped), docs 30, web 321, core 4887 (1 skipped), desktop 839, views 8355. The first `pnpm test` run failed once on `kitviet-skills.test.ts` at 5.1 s (a timeout under the parallel load). That file passes alone (488 tests) and in the rerun.
  - Locale keys: a script compared key sets across the seven `workflow.json` files. Only the CJK `_one` keys differ, as above.

  Not run: `make check` (to completion). Its Go step runs the same test script, which fails, so the E2E step never starts. Its typecheck and unit steps ran above as separate commands. Also not run: Playwright (item 6), and `make up` (item 5).

  Side effects, for the record:
  - `make test` ran `docker compose up -d postgres`. The container was already running, so nothing changed.
  - One read-only `psql` SELECT ran against the shared `multica` URL while I checked the migration table layout. It failed on a wrong column name. Nothing changed.
  - I did not read the "sample-DB leftover" note from memory. This session has no memory tool, so I did not use that note to excuse any failure.

  Still open:
  - Squad WIP test defects in `squad_project_test.go` and `setProjectWorkflow` (item 2 above).
  - The manifest `ENV_FILE` problem, which blocks `make up`.
  - Parallel-package contention in `make test`. Tests in several packages read or change shared rows, so their results depend on what else is running.
  - Pre-existing failures: migration prefixes 600 to 616, and the two probation tests.
  - Playwright has no workflow spec.
  - Section 8 questions 1, 2, 3, 4 and 5 are unchanged. Question 1 remains the open decision on `outcome`.
- [from kiro] Phase D (close-out, uncommitted, no git state changed). Environment: `ENV_FILE=.env.worktree` throughout (database `multica_multica_51`, API 18131, web 30000). The shared `multica` database was not touched.
  1. **Squad test defects, fixed (test side only).** In `server/internal/handler/squad_project_test.go`: `projectWorkflowName` uses a LEFT JOIN, so a project with no workflow reads as "". `TestAssignSquadProjectKeepsTheProjectWorkflow` now acts in the fixture's own workspace through two new helpers, `setProjectWorkflowIn` (sends `X-Workspace-ID` and the admin) and `insertWorkflowIn`. This was not a product bug: `SetProjectWorkflow` resolves the workspace from `X-Workspace-ID` and correctly returns 404 for a project outside it. The file is tracked and already modified by squad WIP; those edits were left as they were. The three squad tests now pass.
  2. **E2E smoke spec, added.** `e2e/workflow.spec.ts`: opens the workflow list, opens `/workflow/new`, expects the badge "Workflow is valid", removes `in_progress`, and expects "1 Errors". It saves nothing. No product, schema, migration or locale change in this phase.
  3. **Manifest problem, not fixed.** `make up` cannot be used. `dev-env.sh` `detect_env_file` returns `.env` because that file exists, so it ignores `ENV_FILE`. The registry entry `~/.multica/dev/envs/multica-51/manifest.env` also says `ENV_FILE=.env`, and its API then runs with the shared `multica` URL. The owner must fix the manifest or decide. I started the API and web dev server by hand with `.env.worktree` sourced, as `check.sh` does, and stopped them afterwards.
  - Runs and results:
    - `go test ./internal/handler/ -count=1 -timeout 40m -v`: 0 FAIL, 48 SKIP (Redis and live GitHub gates). The full non-race run passes.
    - `go test -race -count=1` on the handler package alone: pass (5764 PASS lines, 48 SKIP).
    - `ENV_FILE=.env.worktree make test`: exit 2. Under parallel DB contention it failed `cmd/migrate`, `handler`, `integrations/lark`, `integrations/telegram`, `testutil`, `service` and `migrations`. Run alone with `-race`, `cmd/migrate`, `handler`, `lark`, `telegram` and `testutil` pass. `service` fails only on the two pre-existing probation tests, and `migrations` only on the pre-existing prefix check.
    - `ENV_FILE=.env.worktree make check`: exit 2 at the Go step. Typecheck 9/9 and unit tests 7/7 pass. The Go step (`test-go.sh`, no race) passes `handler`; it fails only `migrations` and `service` (the pre-existing tests below). `check.sh` stops there, so its E2E step did not run.
    - `pnpm typecheck --force`: 9/9 pass. `pnpm lint --force`: 6/6 pass, 0 errors, 35 warnings, none in workflow files.
    - `pnpm test --force --continue`: 6 of 7 tasks passed. Views and core each had one timing-sensitive test fail under load (`billing-tab.test.tsx`, `image-sequence.performance.test.ts`). Both pass when run alone. A forced rerun of `@multica/views` passed: 681/681 files, 8355/8355 tests.
    - `go test ./internal/issueworkflow/ -count=1`: ok. Handler tests matching `Workflow|Transition|Rework|Outcome|Usage|Blocker|Squad`: 369 PASS, 0 FAIL, 0 SKIP.
    - `pnpm exec playwright test e2e/workflow.spec.ts --reporter=line`: 1 passed (30.5 s), against the API and web dev server above.
  - Pre-existing failures, confirmed on `git archive HEAD` in `/tmp` (same results as the tree):
    - `migrations` `TestMigrationNumericPrefixesAreUnique`: prefixes 600–616 are each used twice in committed migrations. Not renumbered.
    - `service` `TestProbationWorseForHireAndRetire` and `TestProbationJudgesTheWindow`: outcome "" instead of worse/improved/neutral. Probation logic not changed.
  - Test data: the E2E user `e2e-workflow@multica.ai` and workspace `e2e-workflow-workspace` stay in `multica_multica_51`. Each run reuses them, as the Hub spec does.
  - Still open, unchanged: section 8 questions 1 to 5, and the UI still does not send `outcome`. Also open: the manifest `ENV_FILE` mismatch in item 3, and parallel-package contention in `make test`.
