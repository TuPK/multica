# Kit Việt S7 real-agent drill (Office Admin, kv_admin)

Status: PARTIAL. Code fixes D1-D4 are in and their tests pass. The re-drill of run 1 ran the full chain, but the chain did NOT reach handover QUA. The request is still `todo`. It never reached `blocked` or `in_review`, so the requester got no stop notification.

Branch: kitviet-pack (worktree multica-kitviet, base commit 4866d01ba). Nothing committed or pushed.

## Environment

- `make up C=api,daemon` in the worktree. Env `multica_kitviet-332`, DB `multica_multica_kitviet_332`, API `http://localhost:18412`, profile `dev-multica_kitviet-332`.
- Drill workspace 1: `kit-s7-drill-0511` (`ac302d7d-...`), first run. Drill workspace 2: `kit-s7-drill-0511b` (`964091a6-df80-4d10-b221-6219812ad62b`), the re-drill. Runtime: `claude` CLI 2.1.312. The daemon registers runtimes for a new workspace only after a restart (`make down` / `make up`).
- Driver: `packages/core/squads/quickstart.ts` `runSquadQuickstart` with `kv_admin`, bundled by esbuild from the scratchpad. Script: `/tmp/claude-1000/-home-av-server-Documents-Github-multica/445e7c1c-9a4b-4b5a-ac6b-14e714f00c7c/scratchpad/s7/quickstart-drill.ts`.
- Invented unit data only ("Công ty TNHH Thử Nghiệm Diễn Tập S7", signer "Ông Trần Mẫu Văn", fictional).
- **Side effect:** `make up` ran its automatic `gc --auto` and destroyed another session's `multica_hub-49` env (DB dropped, profile removed, daemon workspaces removed). Its checkout directory `/home/av-server/Documents/Github/multica-hub` no longer exists. I did not target it, but my `make up` triggered the collection. Not recoverable from here. The main checkout env `multica-51` is listed and untouched.

## Code fixes (D1-D4)

- **D2/D3, stage status (`kv-delivery-core` SKILL.md, `kv-squad-lead` SKILL.md, `kitviet-agent-instructions.ts`):** stage sub-issues end `done` (including a gate's CHẶN verdict) or `blocked` (own stop), never `in_review`. `in_review` is the request's status only, after handover QUA. The output template is `Kết quả: <stage> <done | blocked>`. The barrier counts only `done`/`closed` (`issue_wakeup_condition.go` ~625-665), so a blocked stage holds until a person resumes it with a mention of the lead. The delivery skill states this resume path.
- **D1, routine (`templates.ts`, 19 kitviet templates, `quickstart.ts` `charterRequest`):** `SquadTemplate.operatingReview?: boolean`. No-routine Kit Viet templates set `false`, and `charterRequest(template)` sends `operating_review_enabled: false`. Legal and accounting (with routines) keep the server default. **Verified on the server:** the charter reads `operating_review_enabled: false`. The review object is still created, but `paused`, not `active`. This is the server's design (`CreateOperatingReview` creates it paused when the switch is off). Decision for you: a paused object still exists, while the runbook says `routine: null`. Removing it would need a server change.
- **D4, mention (`kv-delivery-core`, `kv-squad-lead`, `kitviet-agent-instructions.ts`):** every `blocked` stop comment uses `[@<requester name>](mention://member/<requester id>)`, with the id from the request's `creator_id` when `creator_type` is `member`. The lead's stop templates now carry this markup.
- **Tests:** `quickstart-kitviet.test.ts` gains a D1 describe block (4 tests). New `kitviet-stage-status.test.ts` guards the text: no sentence tells a stage agent to set `in_review`, the blocked rule and its markup are present in the lead templates and in each role text.
- **Wording:** the shared text does not quote `VERDICT:` (existing tests forbid it in drafter text), and the lead's stop line says "sets the request to `in_review`" so the guard reads it as the request's status.

## Verification (final tree)

- `node scripts/generate-kitviet-pack.mjs --check`: up to date (29 files). `node --test scripts/generate-kitviet-pack.test.mjs`: 118/118.
- `pnpm exec vitest run packages/core/squads packages/core/agents`: 59 files, 1443/1443.
- `pnpm -C packages/core exec tsc --noEmit`: clean. `eslint` (in `packages/core`) on 25 changed files: clean. Root `pnpm typecheck`: 9/9 tasks, exit 0.

## Re-drill evidence (workspace 2)

**Routine (D1):** squad `Office Admin (Hành chính)` `b58899db-...`, 7 agents. Autopilots: one, `Operating review`, `paused`. Routines created by quickstart: none. Expected-with-caveat: see decision above.

**Gate check (cổng cố tình sai), KITS-3:** planted draft figure (35,000,000 for 3 printers at 10,500,000; correct is 31,500,000). Sub-issue assigned directly to Admin Numbers Gate. Result: comment begins `VERDICT: CHẶN`, names the draft line, recomputes 3 × 10,500,000 = 31,500,000 with a script, gap 3,500,000. Sub-issue `done`. **PASS.**

**Run 1 (KITS-1, full inputs):**
- Stage 1 collector (KITS-5): `done`.
- Stage 2 drafter (KITS-6): `done`. Its workdir holds 27 skill dirs: 24 squad skills plus its three role skills (`kv-soan-don-tu`, `kv-so-sanh-bao-gia-mua-sam`, `kv-chuan-bi-hop-hdqt`). No lead-only skills. **Union PASS.**
- Stage 3: numbers gate (KITS-8) `VERDICT: QUA`, `done`. Format gate (KITS-7) `VERDICT: CHẶN`: basis "Căn cứ tiến độ thực hiện dự án Mẫu" has no source. `done`.
- Stage 4 handover (KITS-9) round 1: `VERDICT: CHẶN` (same format finding). `done`.
- Round 2: fix sub-issue KITS-10 (drafter) `done`. Format gate KITS-11 `VERDICT: CHẶN` (missing "TNDT" symbol in the number line). Format gate KITS-12 `VERDICT: CHẶN`, noting the second CHẶN: stop 3 should apply here, and that the numbers gate QUA was on v1.
- Then the lead ran further rounds: KITS-13 (drafter fix, `done`), KITS-14 (format gate, `VERDICT: QUA`, `done`), KITS-15 (handover, `VERDICT: CHẶN`, `done`). Handover reason: format QUA only at round 3, past the two-round limit; the numbers gate was not re-run on the final draft.
- KITS-1 stayed `todo`. No `blocked` stop comment, no `in_review`, no handover QUA.

**Requester notifications (inbox, dev user):** `status_changed` only for sub-issues; `new_comment` on KITS-1 (the resume mention). No `blocked` or `in_review` item on the request. Notification on `blocked`/`in_review` was verified in the first drill (KITS-2 and KITS-3 `blocked`), not in this run. `in_review` not reached.

## Defects

1. **Round counting was ambiguous (fixed in text).** The lead opened two fix sub-issues (KITS-10 and KITS-13), which is within the limit of two fix rounds; the gate and the handover comments, however, counted check rounds ("the second CHẶN: stop 3", "past the two-round limit") and contradicted the lead. `kv-squad-lead` step 6 now defines a round as one fix sub-issue the lead opened, says the third check after the second fix decides stop 3, and tells the lead to ignore round or stop wording in a gate comment; every gate role text now says to give a verdict only. A K8-style test guards the wording. Not re-run on a real stack.
2. **Numbers gate not re-run after a drafter change.** The drafter revised the document in rounds 1-2 and the numbers gate (QUA on v1) was not re-run; KITS-15 noted this. The lead rule says to re-run only the gate that blocked. Should be: re-run every gate on any changed draft, or the handover must refuse.
3. **Operating review object stays (decision, see above).** Created, but paused. Spec says `routine: null`.
4. **Run 3 (earlier drill) mention gap (D4, low severity, one sample).** KITS-3 in the first drill had a `blocked` comment without the mention markup. Not re-tested in this run; the fix is in the text, not verified by a stop-1 run.
5. **Stalled first run (fixed).** D2/D3: a collector set `in_review` on its sub-issue and the barrier never released. Re-drill: all stage sub-issues reached `done`. Verified.
6. **Runtime quota (environmental).** Claude 5-hour limit made tasks fail with `agent_error.unknown`; the request stayed `todo` and nothing retried. Suggest: an agent-error on the lead should move the request to `blocked`, not leave it `todo`.

## Not verified

- Run 1 end state: handover QUA and request `in_review`. Not reached.
- Requester notification on `in_review` and on the lead's stop-3 `blocked`.
- D4 mention markup in a live `blocked` stop comment (the re-drill never reached a stop comment; the first drill's KITS-2 shows the mention form works).
- Legal/accounting routine behaviour (out of scope).
- Go tests (`make test`): not run. `company_template_data.json` changed (regenerated); the server embeds it.

## Teardown

`make down` (DB and profile kept). Watchers stopped. No `make destroy` run. Drill workspaces remain in the DB for inspection.
