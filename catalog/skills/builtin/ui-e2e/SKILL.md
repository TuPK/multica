---
name: ui-e2e
description: Full workflow for testing a web product like an experienced tester before release, for any project - scan the codebase into a feature inventory, derive numbered user flows, build a coverage matrix with a completeness gate, write Playwright UI e2e tests that act as a real user, run exploratory tours, triage failures (test bug, environment, product), and report release readiness. Use when asked to test a product end to end, write or extend UI e2e tests, check whether coverage is complete, or prepare a release test pass.
---

# UI e2e: full workflow for any project

Act as an experienced tester who uses the product the way a real user does: opens the screens, reads what is on them, clicks, types, makes mistakes, and checks the results they can see. HTTP and SQL are for setup, cleanup and facts the screen cannot show.

The workflow has nine phases. Each phase has a template in `templates/` and an output file in the project's `docs/engineering/` (or the project's equivalent). Run them in order. Do not ask the user whether coverage is complete; run the gate and report its output.

| Phase | Output | Template or script |
| --- | --- | --- |
| 0. Bootstrap: find out how to run the product | run instructions, `e2e-up` script, setup doc | `templates/bootstrap-checklist.md`, `templates/e2e-up.sh` |
| 0b. Test setup | test database, runner, env guard | `templates/run-e2e.sh`, `templates/session.ts` |
| 1. Feature inventory (code scan) | `<product>-features.md` | `templates/features-template.md`, `templates/discover-ui.mjs` |
| 2. User flows | `<product>-flows.md` | `templates/flows-template.md` |
| 3. Coverage matrix | `<product>-ui-coverage.md` | `templates/matrix-template.md` |
| 4. Write tests (UI first) | `e2e/**/*.spec.ts` | `templates/spec.ts`, `templates/edge-cases.md` |
| 5. Exploratory tours | findings in the report | `templates/tours.md` |
| 6. Run and triage | run log, classified failures | `templates/run-e2e.sh` |
| 7. Completeness gate | gate output | `templates/coverage-check.mjs` |
| 8. Release report | `<product>-release-report.md` | `templates/release-report-template.md` |

## Phase 0: bootstrap (get the product running)

Goal: a repeatable command that starts the full stack (database, backend, frontend, workers, fake providers) in a test configuration, and proof that it works. Do this before any test is written. Follow `templates/bootstrap-checklist.md`.

1. **Read the project's own instructions first.** README, CONTRIBUTING, AGENTS.md, CLAUDE.md, docs/, Makefile, package.json scripts, CI workflows (`.github/workflows`). Note any rule about environments, databases, secrets, ports, or worktrees; obey it.
2. **Identify the stack:** language and version for each part (`package.json`, `pnpm-workspace.yaml`, `go.mod`, `pyproject.toml`, `Cargo.toml`, `Gemfile`, `pom.xml`), the database engine and migration tool, cache, queue, object storage, mail, and any external API.
3. **Check what is already there:** toolchains installed, dependencies installed (`node_modules`, module cache), a `.env` or `.env.example`, a running database, Docker available and responsive, ports free.
4. **Choose a path:**
   - **A. The project already has a dev or test start command** (for example `make dev`, `docker compose up`, `pnpm dev`, a documented script). Use it. Only add what it lacks, such as a test database and seed data.
   - **B. The pieces exist but nothing wires them together.** Write `e2e-up` and `e2e-down` scripts (`templates/e2e-up.sh` is the skeleton). Start only the dependencies in containers (`docker-compose.e2e.yml`, or `docker run` with fixed ports), run migrations and a seed, start the backend and the frontend with an env file generated from `.env.example` with random secrets, and wait on health endpoints.
   - **C. Nothing is set up.** Install the toolchain the project declares, write a Dockerfile or compose file only where the project has none, and keep everything the scripts need in the repository (or the project's own scripts folder). Prefer native processes when the toolchain is present; use containers for stateful services.
5. **Use fakes for external providers** (mail, payments, e-invoices, LLM). Read the project's configuration for a dev or fake mode first; if none exists, add the smallest fake that the tests need, behind a config flag, and record it.
6. **Isolate the test environment:** its own database name (ending in `_test`), its own ports, its own data directory. Never point the test run at the development database. Never copy another checkout's secrets into it.
7. **Verify:** health endpoints answer, the first page loads in a browser, a seeded user can sign in through the UI, and the server logs show no errors on startup. Only then continue.
8. **Record** the working commands, env variables (names only), ports, seed data, and known issues in the project's setup doc (for example `docs/engineering/<product>-e2e-setup.md`).

Ask the user only when the setup needs something that cannot be generated or found: a credential for a real external service, or access the machine does not have. Use a fake instead when that is possible.

## Phase 0b: test setup

- Use a dedicated test database (name ending in `_test`) and guard it in the setup code. Never print a connection string, token or secret.
- Run with one worker when tests share fixed ports or one database. Log to a scratch folder with `templates/run-e2e.sh`.
- Check machine load before a long run. A stalled `beforeAll` is usually environment, not product (see phase 6).

## Phase 1: feature inventory from the code

Goal: a list of every feature with an ID, the rule it enforces, where it lives, and whether it is built.

1. Run the UI scan: `node <skill>/templates/discover-ui.mjs <srcDir> > <scratch>/ui-inventory.md`. It lists screens, buttons, fields, dialogs, states, and navigation labels.
2. Read the backend and jobs too: routes or handlers, validation rules and error codes, state transitions, audit actions, scheduled jobs, config flags, and the product's own spec or PRD if one exists.
3. Write `<product>-features.md` with the template. One row per feature, rule, or endpoint. Mark `built` only after you found it in code.
4. Verify every row: each cited file exists and the route, label or error code string appears in it. Rows that fail are either wrong or not built. Record a spec-versus-code disagreement as its own row.

An inventory written from documents alone is not evidence. A spec can list a route the router does not have. Phase 1 ends only after the verification step.

## Phase 2: user flows

Goal: a numbered list of journeys a real user takes, from the inventory.

- One flow per user journey, including the happy path, the common alternative path, and the error path for each rule that a user can meet.
- Use this line format: `N. <flow name> | <actor> | <start screen> | <steps, arrow-separated> | <expected end state> | <inventory IDs>`.
- Add a role tour for each role (what it sees, what it is refused), and a return tour (sign out, sign in again, reload).
- Mark flows that are not reachable from the UI (for example a route that is not registered) so the matrix can show them as API-only.

## Phase 3: coverage matrix

Goal: one row per flow, per screen action, and per UI label, each with a status.

- Columns: `| id | screen | action | status | test |`. Status is `covered` (a passing test exists), `fixme` (a product failure, marked `test.fixme`), or `gap` (no test; the reason goes in the test column).
- Every screen file in the inventory must be named in at least one row. Every simple button or field label in the inventory must appear in a row or a spec.

## Phase 4: write the tests, UI first

- Act only through the UI for the action under test: sign in, search, submit, confirm, click a row, sign out. Setup may use SQL or an API helper.
- Use the labels the code renders. Read the component before writing a selector. A row is often not clickable: click its button.
- Wait with `expect(...).toBeVisible()`, not a sleep. Read lists only after their container is visible.
- Each test seeds what it needs and does not depend on another test. Accept data that other tests may have left behind; do not assert an empty table in a shared database.
- Apply `templates/edge-cases.md` to every form, dialog and list the test touches: empty, whitespace, the limit and one over, invalid format, wrong credential, unknown versus known target, self-target, duplicate, role gating, dialog cancel and escape, double submit, search with no match, refresh, network failure, mobile width, keyboard submit.
- When a request is refused by the server, check the message the user sees, not only the status.
- A failure that matches the spec is a product failure: mark the test `test.fixme` with the observed behaviour and the likely cause in a comment, and record it in the matrix. Do not change product code from a test pass. A test bug is fixed in the test.
- API-only tests are allowed for flows that the UI cannot reach. Label those rows as API in the action column.

## Phase 5: exploratory tours

Write scripted tests first, then spend time as a tester who looks for what the scripts miss. Use `templates/tours.md`:

- First-time user: empty states, first-run hints, what the screen says when there is no data.
- Returning user: reload, back button, a bookmark to an inner page, a session that expired in another tab.
- Two tabs or two users: the same record edited in both; a stale view after a change.
- Errors and slowness: network failure, a slow response, a request that succeeds but returns an error body.
- Money or permissions: every path that changes billing or access; check each refusal.
- Boundaries and input: longest text, emoji and right-to-left text, pasted text with newlines, a number at its limit.
- Narrow screens and keyboard: 375 pixels wide, tab order, Enter and Escape.
- Each finding becomes a `test.fixme` with evidence (a measured value, the screen, the steps) or a new flow.

## Phase 6: run and triage

- Run the full suite with `templates/run-e2e.sh`. Read the summary and each failure's error text.
- Classify every failure before you act:
  - **Test bug:** fix the test. Re-run that file.
  - **Environment:** a `beforeAll` timeout, a server that never answers its health check, a database row the app refuses to load (for example a secret sealed with another key), docker not responding, or machine load. Fix the cause and re-run; do not mark it fixme.
  - **Product:** the spec says one thing and the product does another. Mark `test.fixme` with evidence.
- Measure the dialog and layout problems before you blame a test: check where the element is with `getBoundingClientRect()` at the test's viewport. A dialog whose top equals the viewport height is off-screen. Check the built CSS for the utility classes the component uses (for example `top-1/2`).
- Repeat a test that passed only once three times before calling it stable.
- Report counts: passed, failed, fixme and skipped, and each failure with its class.

## Phase 7: completeness gate

Run it instead of answering from memory:

```bash
node <skill>/templates/discover-ui.mjs <srcDir> > <scratch>/ui-inventory.md
node <skill>/templates/coverage-check.mjs <scratch>/ui-inventory.md <product>-ui-coverage.md <specDir> [ignore.txt]
```

It must print `coverage OK`. It checks that every row has a valid status, that each covered row's test title exists, that each fixme row has `test.fixme`, that each gap has a reason, that every screen file is named, and that every inventory label is in a row or a spec. A passing gate means the matrix is complete; it does not mean the tests pass. Phase 6 gives that.

## Phase 8: release report

Use `templates/release-report-template.md`:
- The gate output and the last full run's counts.
- Product findings (fixme), with evidence and the likely cause.
- Gaps with reasons, and which of them block a release.
- A release recommendation: ship, ship with known issues, or hold. Say which evidence it rests on.

## Phase 9: keep it current

- A new screen or route: re-run phase 1's scan, add rows, run the gate.
- A changed flow: update its row and test.
- Splitting phases 1-3 across helper agents is allowed when the project and the session support it. Whoever writes the output, phase 1 step 4 (verification against code) still applies to every row.

## Hygiene and done

- Test database only; no secrets in logs; rows created by tests are removed in `afterAll`, except rows another table references (leave the user, remove what the test owns).
- Done means: the gate passes, the full suite has run and its counts are reported, every product failure is a fixme with evidence, and the release report is written.
