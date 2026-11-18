# Code reviewer eval

Measures how many known bugs the Code Reviewer (`sdlc-code-review`) catches, and how many findings it raises on
stable commits that have no bug. It follows the method in "How the Claude Code team thinks about verification":
recall is measured per tier, a tuning set is kept apart from the fixed set, and the checks are graded against bugs
that actually shipped.

## Files

- `cases.json`: the labelled cases. Each case names a commit range, the files and keywords that identify its bug,
  and a one-sentence answer key. `split` is `tuning` (used to change the reviewer), `fixed` (only used to report),
  or `control` (a stable commit with no bug).
- `score.mjs`: scores a results file against the cases. `--selftest` checks the scorer itself.

## Running it

1. For each case, run the reviewer on `base..head` from a worktree at `head`. Use the same effort level for every case.
2. Write `results.json` in this shape. Each tier is the reviewer's list at that step of its workflow:
   - `raw`: candidate findings before verification (`sdlc-code-review`, step 2)
   - `verified`: findings kept after verification
   - `final`: findings in the report

   ```json
   {
     "group-name-in-audit": {
       "raw": [{ "file": "server/internal/handler/member_group.go", "line": 115, "summary": "..." }],
       "verified": [],
       "final": []
     }
   }
   ```

   Set `"preExisting": true` on a finding that reproduces on the base branch. Those findings do not count as false
   positives on controls.
3. Score it:

   ```bash
   node scripts/sdlc-review-eval/score.mjs --cases scripts/sdlc-review-eval/cases.json --results results.json
   ```

## What a number means

- Recall is caught bugs over bug cases, per tier. A gap between `raw` and `final` is a bug the reviewer found and then
  lost, which points at the verification or report step.
- Tune the reviewer on `tuning` only. Report `fixed` once per change to the reviewer, not after each tweak. Stop tuning
  when the next gain would only fit this set.
- A result on fewer than about ten bug cases is an anecdote. The seed set has two bugs and one control, so no recall
  rate can be drawn from it yet.

## Growing the set

Add a bug case when a merged change is later reverted or fixed. Its base is the parent of the commit that introduced
the bug, its head is that commit, and its answer key is the fix's subject. Add a control when a merged commit of
similar size has stayed unchanged by fixes for a month. Confirm both with `git log --grep="^Revert"` and by listing fix
commits that touch the same files.
