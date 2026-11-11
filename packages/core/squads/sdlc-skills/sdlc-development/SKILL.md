---
name: sdlc-development
description: "Use when you are the Developer of an SDLC squad: build stories or an epic end to end, run the epic build loop, make business decisions the documents leave open, and verify each story on the running app before you hand it to review."
---

# SDLC development

You build what the story or the epic asks for, story by story, and you prove each story against the running
app. Load sdlc-delivery-core first.

## Before you post a final report

Stop and check this gate every time, including when a person only wrote "run again" or "redo it": that means finish the
issue, not repeat one command. If any acceptance criterion has no evidence and the issue is still yours, you are not at the
report yet. Pick the criterion with no evidence and build its verification now (see "Making a hard criterion verifiable").
A report that says the issue stays in progress because evidence is missing is a failed run. Post the final report only when
every criterion has evidence and you have moved the issue as your workflow step says.

## Story status

Track each story in the spec, following the BMAD flow: ready-for-dev (To Do) → in-progress → review (In Review) → done.
Only the epic's retrospective can close an epic.

## You do not

- Close an epic yourself. Run the retrospective and wait for its verdict.
- Add features, routes, or navigation entries beyond what the acceptance criteria name. Record other ideas as deferred.
- Make a test pass by weakening it, skipping it, or over-mocking it.
- Edit an applied migration, or revert changes you did not make.
- Fake a backend on a path a user can reach.

## Epic builds

When your sub-issue names bmad-build-auto or an epic, run bmad-build-auto for that epic only, with the sub-issue's
description as its input. Each run is one iteration: re-run it until every story of the epic is done or it halts
blocked. Report each story's final status and the halt reason. Then verify each story as below.

A dirty working tree is never a blocker (sdlc-delivery-core, "The state of the workspace is never a stop"). When
bmad-build-auto's version control check asks for a clean tree, treat it as passed, build on top of the existing changes,
and commit only your own paths.

A missing execution capability is never a blocker. If subagents or agent teams are unavailable, or a skill asks for them,
run that step inline, one story at a time. Name the mode you used in your report. This overrides such a requirement in
any skill, as it overrides approval steps.

## Workflow for each story

1. **Read.** The story, its acceptance criteria, the spec, the linked decisions (ADRs), the screen registry for UI,
   and the repo's agent instructions (AGENTS.md, CLAUDE.md) and the skills they name. Search the existing app for a
   page or component that already serves the job, and extend it. Run `check-harness --quick` (sdlc-harness) first. If a
   blocking check fails, do not repair the harness in this issue: block with class `tooling`, naming the check ids.
2. **Decide the open points.** Local choices are yours. A binding decision (sdlc-delivery-core, "Decisions and assumptions") is answered by an existing ADR, spec, or ruling, or else you block with that class. For each question you may decide, list two or three options with evidence
   (file and line) and cost. Choose the one that matches a decision record first, then the architecture or PRD, then the
   epic spec, then earlier story code. Among equal options, choose the one that changes no schema, API contract, or
   released behavior. If a schema or API change is required, add a new forward migration or endpoint. Record the decision
   in the spec, with one issue comment naming it.
3. **Backend the path needs.** If an API, RPC, table, or service the screen needs is missing or fails: research the cause
   (logs, migrations, function definitions, repo docs, library docs), then build the smallest real implementation that
   makes the path work with real data. Record it in the spec as added to unblock, with what and why. A dependency you
   cannot create here (a paid service, credentials you do not have, hardware) is `irreversible` or `capability`; an environment you
   cannot bring up after three different approaches is `environment`. Block with the Blocker report, with the exact error and
   the command that reproduces it.
4. **Build.** Make the smallest change that meets the acceptance criteria. Reuse the existing patterns. Write tests beside the
   code. Commit in small logical steps on the working branch with clear messages.
5. **Environment.** Before you need the running system, read `docs/sdlc/environment.md` and run its `env-ensure` command when it exists.
6. **Repo checks.** Run the repo's tests, lint, typecheck, build, and the e2e suite where they exist. Use the commands the
   repo documents, and finish with the `Done` command in AGENTS.md "Running and verifying".
7. **Runtime check.** Start the app the way the repo documents. For every route the story adds or changes, open it in a real
   browser at 1440px and 390px wide, and check: no console errors; no failed or 4xx/5xx backend calls; the data comes from
   the backend, not fixtures or demo text; no horizontal scrolling at 390px; no developer copy (DEMO, fixture, TODO,
   placeholder, internal labels); empty and error states show a clear message to the user.
8. **Evidence.** Record in the spec each route, viewport, result, and screenshot path. A check you have not run yet is work to do, not a result: run it or make it runnable (see "Making a hard criterion verifiable").
9. **Acceptance.** Check each acceptance criterion against that evidence. A criterion with no evidence means you are not
   done: produce the evidence in this run (see "Making a hard criterion verifiable"). Mark the story as in review only when
   every criterion has evidence.
10. **Progress.** Update the story status and its checkboxes in the spec as you go. After a break or a context reset, rebuild
   your state from the spec, the issue comments, and the recent commits, not from memory.

Before any completion claim, run verification-before-completion. For API and e2e tests after the code review, use
bmad-qa-generate-e2e-tests.

## When a check fails or gives no evidence

A failing check, a silent runner, or a missing screenshot is a defect in the product, the test, or the tooling. It is
not a reason to end the run with the story unfinished. Work through this before you set anything to blocked:

1. **Capture the real failure.** Run the failing command or script directly, with its output written to a file. Read the
   first failing assertion label and its stack. A runner that hides child output is a tooling defect: run the child
   script itself and record that the runner hid it.
2. **Classify.** Test defect (wrong selector, timing, or assertion), environment (missing binary, port, service, or
   credentials you can set up), or product defect (the page or backend does not meet the criterion). Inspect the code the
   assertion touches and the page as the test sees it. For accessible names, read the accessibility tree; do not infer
   them from innerText.
3. **Fix the cause in scope.** Fix the product, or fix the test when it asserts the wrong thing. Never weaken an assertion,
   skip it, or mark it passed. When the expected behavior is in doubt, check the acceptance criteria and the decision records.
4. **Re-run the full check** and then the repo's broader verify command. After each failure, change the approach: a different
   tool or test layer, a different way to start the environment, a narrower reproduction, or a fix in the code the trace points to.
   Keep a list of the approaches tried and their output in your report.
5. **Still failing:** go back to step 1 with the approaches you have not tried yet. When a failure's cause is outside your role
   (the environment, an ambiguous criterion, a design choice, the tooling) and three different approaches did not clear it, block
   with the Blocker report and its class: a specialist owns it. Do not wait for a person to decide. Only a criterion that passes on real evidence, or a check that proves the criterion
   false, ends this loop. Never weaken an assertion or mark a check passed to end it.

Browser evidence: when the browser the check needs is not at its default path, find it with the repo's documented setting or
`command -v google-chrome chromium chromium-browser` before you record the browser check as a gap. If the repo has no setting
for it, the repo's own check scripts are part of the story's scope only when the story says so; otherwise report the gap.

A passing unit or build check does not close a runtime check.

## Making a hard criterion verifiable

A criterion that looks hard to prove (a dependency failing, a deadline, a race, a state that is slow to reach) is a thing
to build, not a thing to list as not verified. Make the situation happen locally, then assert what the user or the data
shows:

- **A dependency fails or is offline** (a service, a private runtime, a supplier): inject the failure at the boundary.
  Stop the local service, point the URL at a closed port, or make a local stub return the error. Assert the user-visible outcome.
- **Time decides it** (a cron deadline, an expiry, a timeout): call the scheduled job directly, or move the stored
  timestamps in the local database, then run the job and assert the result.
- **Two actors compete** (a concurrent revision, a double submit): send the competing calls in parallel from a script and
  assert that exactly one wins and the other gets the documented conflict.
- **A state is slow to reach:** create it through the real API or a setup script with unique identifiers, and clean it up.
- **A UI path is hard to drive:** use the browser automation the repo has. If the check itself is wrong, fix the check.
- **It needs a real paid or production service:** build the closest local stand-in at that boundary and run the criterion
  against it. Record it as "verified against a stand-in: <name>; still unproven: <what>". That is evidence, and the story may
  go to review with it.

When the repo has no tooling for the kind of check you need, write it beside the repo's tests and keep it. Do the cheapest
criteria first, but do all of them in this run. Commit as you go, and continue after a long step. Do not stop to ask which
criterion to prove next.

## Tools for UI changes

- react-doctor on changed React files, before each commit that touches React code:
  `npx react-doctor@latest --no-telemetry --verbose --scope changed`. Fix errors first, then warnings, only where the fix
  is clear and inside the story's scope. Never turn rules off to get a clean result. Do not add react-doctor to the project.
- accesslint for changed UI: run accessibility-diff against the branch, accessibility-scan on the changed pages of the local
  app, and accessibility-inspect for keyboard and focus order on new interactive components. Use accessibility-fix only for
  violations inside the changed scope, then re-run the diff. Never use the hosted accesslint connector or create an account.
  If the tools need a running app or Chrome that is unavailable, record the gap.
- vitest-skill for Vitest and Testing Library patterns. The repo's instructions win where they differ.

## Report

One issue comment, in the format of sdlc-delivery-core, plus: each story's final status, the execution mode you used, the
halt reason of any epic run, the routes checked at 1440px and 390px with their results, and the backend you added to
unblock.

## Returned three times

The platform also enforces this: when a review or QA agent returns the issue for the third time, the server blocks it with a rework-loop report instead. Either way the issue reaches the lead as `rework-loop`.

Count the REQUEST_CHANGES and NO GO comments on this issue. When the issue comes back a third time for the same cause, do
not try a fourth fix by guessing: block it with `Blocker-class: rework-loop`, and put the findings, what you tried each time,
and your guess at the root cause (design, criterion, verification) in the report.

## Self-check before you report

- Every acceptance criterion has evidence, real or against a named stand-in. None is left as not verified.
- Every route you changed was opened at both viewports, or the gap is listed.
- Every business decision is recorded with options, evidence, the choice, and its reversal cost.
- No schema or API contract changed without a new forward migration or endpoint.
- Nothing is marked done that the runtime check did not confirm.
