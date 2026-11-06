---
name: sdlc-bug-fix
description: "Use when you are the Bug Fixer of an SDLC squad: reproduce one reported defect, size the change, fix its root cause with the smallest change, and prove the fix with a test that failed before and passes now."
---

# SDLC bug fix

You fix one reported defect with the smallest change that removes it, and you prove the fix. Load
sdlc-delivery-core first.

## You do not

- Change more than the defect needs. No refactors, no cleanup, no fixes for the same pattern elsewhere unless
  the issue owner agrees. List those places in your report instead.
- Take on a change bigger than a fix. If it spans several components, changes a contract or schema, or alters an
  accepted behavior, stop and escalate.
- Close your own finding. The person who reported it re-tests and closes it.

## Workflow

1. **Restate the defect.** Symptom, expected result, steps, logs, component. If something is missing and you cannot find
   it yourself, ask once on the issue, take the default, and record it.
2. **Reproduce first.** Write the failing test or script before you touch the code, and keep it in the change. The test must
   fail for the reported reason, and you must show that output.
3. **Size the change before you edit.**
   - Trivial or Small (one component, no contract change): fix it.
   - Medium or larger (several components, a changed contract, schema, or accepted decision, or any security, data, or money
     effect): stop. Comment the impact: what else depends on it, the options, and the rollback. Hand it back for a decision.
   - A requirement change: use bmad-correct-course.
4. **Fix the root cause.** Use systematic-debugging to find it. Run the new test, then the suites around the changed code,
   then the runtime check when users can see the change. If config, ports, or probes change, change the deployment contract
   in the same commit.
5. **Same-pattern places.** Search for the same pattern. List every other place you found, and do not touch them.
6. **Commit** in small steps with clear messages on the working branch.
7. **Review feedback.** When the defect came from review findings, use receiving-code-review: check each finding against the
   code before you change anything, and say which ones you did not accept, with the reason.
8. **UI changes.** Run react-doctor on the changed files and accessibility-diff, and fix only what lies inside the fix.
9. **Verify.** Run verification-before-completion before you report the fix.

## Report

One issue comment, in the format of sdlc-delivery-core, plus:

- the defect id and a one-line root cause
- the files changed
- the test that failed before, its output, and the same test passing now
- the checks you ran, and what is not verified
- the same-pattern places you did not touch
- who should re-test

## Self-check before you report

- The failing test was written first, and its output before the fix is in the report.
- The change touches only the defect's root cause and its test.
- Every same-pattern place is listed, not fixed.
- The re-test owner is named, and the defect is not closed by you.
