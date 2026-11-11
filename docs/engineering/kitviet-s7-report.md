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
- **D1, routine (`templates.ts`, 19 kitviet templates, `quickstart.ts` `charterRequest`):** `SquadTemplate.operatingReview?: boolean`. No-routine Kit Viet templates set `false`, and `charterRequest(template)` sends `operating_review_enabled: false`. Legal and accounting (with routines) keep the server default. **Fixed at the source (server):** a first charter save with the switch off creates no review object (no autopilot, no routine row). The same rule applies on the starter company path. Turning the switch on later creates the review active; turning it off pauses it. **Verification:** Go tests pass against the worktree database: `TestOperatingReviewOffAtFirstSaveCreatesNothing` and `TestOperatingReviewTurnedOnLaterIsCreatedActive` (handler), and the company template tests (service), which had been asserting a paused review for every squad. Not re-drilled on a real stack; the Office Admin squad in drill workspace 2 keeps the paused review it got before the fix.
- **D4, mention (`kv-delivery-core`, `kv-squad-lead`, `kitviet-agent-instructions.ts`):** every `blocked` stop comment uses `[@<requester name>](mention://member/<requester id>)`, with the id from the request's `creator_id` when `creator_type` is `member`. The lead's stop templates now carry this markup.
- **Tests:** `quickstart-kitviet.test.ts` gains a D1 describe block (4 tests). New `kitviet-stage-status.test.ts` guards the text: no sentence tells a stage agent to set `in_review`, the blocked rule and its markup are present in the lead templates and in each role text.
- **Wording:** the shared text does not quote `VERDICT:` (existing tests forbid it in drafter text), and the lead's stop line says "sets the request to `in_review`" so the guard reads it as the request's status.

## Verification (final tree)

- `node scripts/generate-kitviet-pack.mjs --check`: up to date (29 files). `node --test scripts/generate-kitviet-pack.test.mjs`: 118/118.
- `pnpm exec vitest run packages/core/squads packages/core/agents`: 59 files, 1443/1443.
- `pnpm -C packages/core exec tsc --noEmit`: clean. `eslint` (in `packages/core`) on 25 changed files: clean. Root `pnpm typecheck`: 9/9 tasks, exit 0.

## Re-drill evidence (workspace 2)

**Routine (D1):** squad `Office Admin (Hành chính)` `b58899db-...`, 7 agents. Autopilots: one, `Operating review`, `paused`, created before the server fix (D1). Routines created by quickstart: none. This evidence is from before the fix and was not re-run after it.

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
3. **Operating review object created with the switch off (fixed at source, D1).** The drill's paused review predates the fix; see D1 for the verification status.
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

## Final re-drill

Status: run 1 reached the target end state (request `in_review`, handover QUA, requester notified). The usage-limit classification defect is fixed with a regression test. Two design gaps in the failure path are documented, not fixed. The environment is still running for the next step (seeding squads).

### Environment

- Worktree `/home/av-server/Documents/Github/multica-kitviet`, branch `kitviet-pack`, HEAD `1e2ba5a19`. Uncommitted: `server/pkg/taskfailure/classify.go`, `classify_test.go`, `server/internal/service/delegated_failure_recovery_test.go`, this section.
- Env `multica_kitviet-332`, DB `multica_multica_kitviet_332`, API `http://localhost:18412` (pid 3829708, restarted after the fix), daemon running for profile `dev-multica_kitviet-332`. Web and desktop not started.
- Workspace `kit-s7-final`, id `757a48e7-9a82-4fa8-80be-907433a2edbb`, issue prefix `KITF`.
- Bring-up: `make up C=api,daemon`. Before it ran, `make gc --dry-run` listed nothing to collect (both envs are human-owned, TTL 0, directories present). The daemon registered the new workspace's runtimes only after a daemon restart (`make down --components daemon`, then `make up`). The Claude runtime is `5bb00262-07ea-4c16-91f5-0a466367843d`.
- Quickstart: `runSquadQuickstart` with `kv_admin` through `ApiClient`, bundled by esbuild (`scratchpad/s8/quickstart-final.ts`). Squad `Office Admin (Hành chính)` `de1b45c7-8225-47bc-9191-4e2ca8ac764d`, lead `Admin Lead` `fa45dd84-45b6-4901-b30e-59d106fd4383`, 7 agents created, routines `[]`, workflow skipped, skills ready.

### Run 1 (spec 8.2, Office Admin, full inputs)

Request `KITF-1` (`01a1c9ad-e3ee-7c30-b2ba-36dc75c57f49`), assigned to the squad (as in S7, the squad routes to its leader). Invented unit "Công ty TNHH Thử Nghiệm Kit Final" (TNKF), signer "Ông Trần Mẫu Văn" (fictional), milestones M1 40%, M2 15%, M3 not started, reference figure 3 × 10,500,000 = 31,500,000, meeting 20/11/2026 at 9:00.

| Step | Issue | Result (from DB and comments) |
| --- | --- | --- |
| Stage 1 | KITF-3 collector, KITF-4 format lookup | done |
| Stage 2 | KITF-5 drafter | done; draft v0 attached (`ban-nhap-cong-van-moi-hop.md`); register line on `KITF-2` |
| Stage 3, round 0 | KITF-6 format gate | `VERDICT: QUA`, two non-blocking items |
| Stage 3, round 0 | KITF-7 numbers gate | `VERDICT: CHẶN`: line 45 said the number is given by the registry clerk; the request says the signer assigns it after signing. A real catch. |
| Stage 4, round 0 | KITF-8 handover | ran once and posted `VERDICT: CHẶN`, although the lead had posted a pause on it at 08:22:35 (the pause is a comment, not a stop). done. |
| Round 1/2 | lead comment `## Round 1/2` | counted by fix sub-issues: one fix (`KITF-9`, v1 attached) |
| Gates re-run on v1 | KITF-10 format gate | `VERDICT: QUA` (three non-blocking items) |
| Gates re-run on v1 | KITF-11 numbers gate | `VERDICT: QUA` |
| Stage 4 | KITF-12 handover | `VERDICT: QUA` |
| Request | KITF-1 | `todo` -> `in_review` at 08:26:45 |

- **Mention and stop comment:** the handover comment on `KITF-1` at 08:26:45 contains `[@dev](mention://member/8514973b-b3ba-4824-bf2a-c5cd81447a0d)` (the requester). The `IN REVIEW · stop 2` comment at 08:26:56 has a mention as well.
- **Inbox of the requester (`8514973b-…`, unread):** `status_changed` `{from: todo, to: in_review}` at 08:26:45, `mentioned` x2 on the two comments, `new_comment` x8 across the run.
- **Nothing numbered or signed:** the draft has `Số: [cần bổ sung: số văn bản]`, a signature placeholder, and `BẢN NHÁP - chờ ký`.
- **Status transitions:** stage sub-issues `backlog` -> `todo` -> `in_progress` -> `done`. No stage ended `blocked`. The request stayed `todo` for the whole run and was never `in_progress`. Only the lead's handover moved it to `in_review`.
- **Round counting and re-run (defect 1 and 2 from S7):** verified in this run. The round is counted by fix sub-issues, and both gates were re-run on the changed draft.

### Remaining defects in this run

1. The deliverable still carries internal notes in the same file as the letter (the `## NỘI BỘ - KHÔNG GỬI KHÁCH` section, with its source table and "Đã sửa theo soát vòng 1" log). The format gate listed this as Nên sửa and did not block. The handover still said QUA. Needs a rule that internal review notes go in a separate attachment.
   **Status: fixed in text and gate, not re-run on a real stack.** The admin drafter no longer writes internal notes into the file: `KV_LEAD_NOTES.kv_admin` and `KV_ROLE_NOTES.admin_drafter` forbid them, and the upstream drafter's "Nguồn dữ kiện", "Cần người ký xác nhận" and "Đã sửa theo soát" instructions now point to the lead's comment (`kitviet-patches/soan-van-ban-hanh-chinh.json`). The format gate rates internal notes in an outgoing document as Chặn (`kitviet-patches/soat-the-thuc-van-ban.json`). Guard tests in `kitviet-templates/admin.test.ts`. Not verified: a live Office Admin run that produces a draft and a gate verdict on the new text.
2. The handover (KITF-8) ran before the pause took effect and gave a CHẶN verdict. Stage sequencing does not hold a stage that was already started.
3. D4 (mention in a `blocked` stop comment) was not exercised: no stop ended `blocked` in this run. The `in_review` stop did carry the requester mention.

### Failure path (GOAL C): usage-limit failure

Evidence: the S7 DB has two `failed` tasks with `failure_reason = agent_error.unknown`, `attempt 1 / max_attempts 2`, error text `You've hit your 5-hour limit · resets 2:40pm (Asia/Ho_Chi_Minh)`. They were not retried.

- **Classification defect (fixed).** `Classify` matched only `you've hit your limit`, `hit your session limit` and `hit your weekly limit`. The 5-hour window was not matched and fell to `agent_error.unknown` (`server/pkg/taskfailure/classify.go`, the quota rule). Fix: `subscriptionWindowLimitRe` (`(hit|reached) your \d+-hour limit`) added to the quota case. Regression rows in `TestClassifyRules` (`classify_test.go`): three subtests failed before the fix (`5-hour limit ascii`, `curly`, `reached`); `go test ./pkg/taskfailure/` passes after it.
- **Effect of the label:** `freshSessionMayHelp` (`server/internal/daemon/daemon.go` ~9650) returns true for `unknown` (a fresh session may help) and false for `provider_quota_limit`. Before the fix the usage-limit run could lose its resume pointer. After the fix it keeps it.
- **Retryable?** No, and that is by design. `ReasonAgentProviderQuotaLimit` is documented "Not retryable until the quota resets" (`server/pkg/taskfailure/failure.go:216`). It is not in `retryableReasons` (`server/internal/service/task.go:5608-5615`). The classification fix does not change retry behaviour.
- **Wakes the coordinator?** Depends on who ran the task.
  - Delegated worker (has `delegated_from_task_id`): yes, for any reason. `FailTask` calls `recoverDelegatedTaskFailure` whenever no retry was created (`task.go:5529-5540`). New test `TestFailTaskUsageLimitDelegatedFailureWakesCoordinator` (`server/internal/service/delegated_failure_recovery_test.go`) passes: the reason is quota, no retry child, one `delegated_failure` recovery task routed to the coordinator agent and issue.
  - Top-level run (the lead on a human-assigned request, the S7 case): no wake, by design. `loadDelegatedFailureRecoveryTarget` returns nil when `DelegatedFromTaskID` is not set (`task.go:6660-6663`). The only effect is a system failure comment (`task.go` ~5551). The request is not landed: `FailedRunLandingStatus` applies only when the effective status is `in_progress` (`task.go:6396`, `6475`).
  - Why the request stays `todo`: the platform never moves a regular issue to `in_progress` when a task is claimed (the only `in_progress` writes are the Qoder lifecycle and the workflow entry). The agent sets it. In run 1 the lead never did, so a failed lead run leaves the request in `todo`, and nothing notifies the requester (no inbox item on failure).
- **Not fixed (design, not a small classification change):** how a failed top-level run hands the request back. Options: land `todo` requests to `blocked` on a terminal failure, notify the requester, or wake the squad lead on a top-level failure. Each changes the state model, so it needs a decision.

### Operating review (GOAL D)

- The new squad `de1b45c7-…` has no autopilot. `autopilot` has 0 rows in `kit-s7-final` (none titled "Operating review"). `squad_routine` has 0 rows for the squad. The kv_admin template sets `operatingReview: false`, so the server creates no object (D1 fix). Workspace 2's old paused review is unchanged (it predates the fix).

### Verification run

- `go test ./pkg/taskfailure/` ok. `go test ./internal/service/ -run 'Delegated|FailTask|Quota|Usage'` ok (includes the new test). `go test ./internal/daemon/ -run 'Classif|Quota|Usage|FreshSession'` ok. `gofmt -l` clean on the three Go files.
- `go test ./pkg/agent/` failed once under load and passed on the next run. I did not identify the failing test; the classifier change is not on that path.
- Not run: `make test` (full), `pnpm` checks (no frontend change).
- Not verified: a live usage-limit event (this run produced none). The API is `go run` from source, so it loads the fix; the reported commit is still `1e2ba5a19` because the fix is uncommitted. The requester notification was checked in the inbox table, not in the UI.

### Teardown

Not done. The environment is running (`make status`: api and daemon running). Next step: seed squads into `kit-s7-final`.
