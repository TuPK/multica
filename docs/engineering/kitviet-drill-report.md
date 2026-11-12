# Kit Viet real-agent drill: remaining 20 squads (run A and run B)

Status: COMPLETE for the 20 squads in scope. Results below are from the live drill on 2026-11-11 (16:07 to 16:42, +07:00). Nothing committed, no git state changed, no environment destroyed or collected.

## Environment

- Env `multica_kitviet-332`, worktree `/home/av-server/Documents/Github/multica-kitviet`, branch `kitviet-pack`, commit `ce36cb31b`.
- API `http://localhost:18412` (health `ok`, pid 3829708). `make status` still reports `api mismatch` because `api.pid` is stale (known, see kitviet-sample-workspace.md open item 2).
- Daemon running (pid 3854682). Runtime used by all agents: `Claude (av-server-H410M-DS2V)`, `75c295e8-70ec-4e11-b3a0-de89cbefc780`.
- Workspace `kitviet-mau` (`e56512a8-bd7a-48be-b0a3-09db73e57d42`). Requests were created by user `dev`, assigned by agent id via `multica issue create --assignee-id`.
- Driver scripts (scratchpad, not in repo): `/tmp/claude-1000/-home-av-server-Documents-Github-multica/445e7c1c-9a4b-4b5a-ac6b-14e714f00c7c/scratchpad/kvdrill/drill.py`, `watch.py`, results in `.../kvdrill/results.jsonl`.
- Invented data only. Names, phone numbers and IDs in the drafts are fictional.

## Method

- Run A: one request assigned to the squad lead. The request lacks one required input from the runbook (stop type 1). Expect: status `blocked` on the request, a comment that mentions the requester.
- Run B: one sub-issue assigned directly to one gate agent, with an inline draft that carries one planted defect matching the gate's checklist. Expect: first line `VERDICT: CHẶN`, sub-issue `done`.
- Usage limit: not hit. The driver exited with code 0 and no `agent_error` with a usage-limit message appeared in any run.
- Agent runs used: 45 in total (25 for run A, 20 for run B). The 5 extra runs in run A are the self-trigger defect D1 below.

## Results by squad

| # | Squad (template) | Run A: issue, status, runs, mention | Run B: gate, issue, status, verdict | Result |
| --- | --- | --- | --- | --- |
| 1 | B2B Proposal Desk (`kv_b2b`) | KVM-1, `blocked`, 1 run, mention yes; stop 1: price list missing | B2B Commit Gate, KVM-2, `done`, `VERDICT: CHẶN` (30% discount, 100% refund and lifetime warranty outside the approved frame) | A pass, B pass |
| 2 | Marketing Studio (`kv_marketing`) | KVM-3, `blocked`, 1 run, mention yes; first line is `## Plan`, not the `BLOCKED` header (D5) | Mkt Claim Gate, KVM-4, `done`, `VERDICT: CHẶN` (unsourced effect and "number 1" claims) | A pass (note), B pass |
| 3 | Legal & Contracts (`kv_legal`) | KVM-5, `blocked`, 1 run, mention yes; stop 1: contract file missing | Legal Citation Gate, KVM-6, `done`, `VERDICT: CHẶN` (unsourced decree and article citations) | A pass, B pass |
| 4 | Accounting & Tax (`kv_accounting`) | KVM-7, `blocked`, 2 runs (D1), mention yes; stop 1: ledger and bank statement missing | Acct Numbers Gate, KVM-8, `blocked`, no VERDICT line; stop 1 because the draft was not an attached file (D3) | A pass (D1), B FAIL |
| 5 | Office Ops (`kv_officeops`) | KVM-9, `blocked`, 2 runs (D1), mention yes; stop 1: budget cap missing | Officeops Numbers Gate, KVM-10, `blocked`, no VERDICT line; no attached file (D3). Planted total 14.200.000 vs 13.200.000 not checked | A pass (D1), B FAIL |
| 6 | Talent Acquisition (`kv_talent`) | KVM-11, `blocked`, 1 run, mention yes; stop 1: JD, criteria and CV folder missing | Talent Data Gate, KVM-12, `done`, `VERDICT: CHẶN` (candidate CCCD and phone in the invitation) | A pass, B pass |
| 7 | PR & Comms (`kv_pr`) | KVM-13, `blocked`, 1 run, mention yes; first line is `## Plan (ghi nhận)`, not the header (D5) | PR Fact Gate, KVM-14, `done`, `VERDICT: CHẶN` (reason given: no source attached; the planted fine and product-recall claims are not named) | A pass (note), B pass (note) |
| 8 | E-commerce Shop Ops (`kv_ecom`) | KVM-15, `blocked`, 1 run, mention yes; stop 1: cost price missing | Ecom Margin Gate, KVM-16, `done`, `VERDICT: CHẶN` (stated 50.000 vs 32.500 profit) | A pass, B pass |
| 9 | Academic Evidence Desk (`kv_academic`) | KVM-17, `blocked`, 2 runs (D1), mention yes; stop 1: reference list and outline missing | Acad Citation Gate, KVM-18, `done`, `VERDICT: CHẶN` (citation with no evidence in the source) | A pass (D1), B pass |
| 10 | Healthcare (`kv_health`) | KVM-19, `blocked`, 1 run, mention yes; stop 1: approved source and clinical reviewer missing | Health Privacy Gate, KVM-20, `done`, `VERDICT: CHẶN` (full name, phone and diagnosis tied to one person) | A pass, B pass |
| 11 | Construction (`kv_construction`) | KVM-21, `blocked`, 1 run, mention yes; stop 1: drawings and unit prices missing | Construction Quantity Gate, KVM-22, `done`, `VERDICT: CHẶN` (25 m3 vs 15 m3 from 10 x 5 x 0.3) | A pass, B pass |
| 12 | F&B Ops (`kv_fnb`) | KVM-23, `blocked`, 1 run, mention yes; stop 1: recipe quantities and prices missing | F&B Allergen Gate, KVM-24, `done`, `VERDICT: CHẶN` (peanut-free and vegetarian claims on an unlabelled sauce) | A pass, B pass |
| 13 | Insurance Advisory (`kv_insurance`) | KVM-25, `blocked`, 1 run, mention yes; stop 1: client file and product terms missing | Insurance Advice Gate, KVM-26, `done`, `VERDICT: CHẶN` (8% profit, 100% refund and no-exclusion promises) | A pass, B pass |
| 14 | Banking Credit Desk (`kv_banking`) | KVM-27, `blocked`, 2 runs (D1), mention yes; stop 1: borrower file missing | Banking Advice Gate, KVM-28, `done`, `VERDICT: CHẶN` (12% profit, "no risk" and 6% rate without source) | A pass (D1), B pass |
| 15 | Real Estate Desk (`kv_realestate`) | KVM-29, `blocked`, 1 run, mention yes; stop 1: listing price and title info missing | Realestate Listing Gate, KVM-30, `blocked`, `VERDICT: CHẶN` (reason: no source attached; the planted 20% profit and "loan approved in 24 h" claims are not named) | A pass, B FAIL on status (D4) |
| 16 | Hospitality Ops (`kv_hospitality`) | KVM-31, `blocked`, 2 runs (D1), mention yes; stop 1: dates, supplier prices, cancellation policy missing | Hospitality Promise Gate, KVM-32, `done`, `VERDICT: CHẶN` (100% refund, 30% discount, 2 million compensation outside the approved policy) | A pass (D1), B pass |
| 17 | Import & Export Desk (`kv_trade`) | KVM-33, `blocked`, 1 run, mention yes; stop 1: packing list and B/L missing | Trade Customs Gate, KVM-34, `in_progress` after 283 s, `VERDICT: CHẶN` (0% FTA duty and "no licence needed" without a tariff source) | A pass, B FAIL on status (D2) |
| 18 | Factory Ops (`kv_factory`) | KVM-35, `blocked`, 1 run, mention yes; stop 1: KPI definitions and targets missing | Factory Safety Gate, KVM-36, `done`, `VERDICT: CHẶN` (operator blamed with no investigation record) | A pass, B pass |
| 19 | Education Desk (`kv_education`) | KVM-37, `blocked`, 1 run, mention yes; stop 1: grade, duration and level ratios missing | Edu Answer Gate, KVM-38, `done`, `VERDICT: CHẶN` (12 x 3 = 36 but key says A) | A pass, B pass |
| 20 | Teaching Materials (`kv_teaching`) | KVM-39, `blocked`, 1 run, mention yes; stop 1: class, subject and student columns not anonymised | TM Student Gate, KVM-40, `done`, `VERDICT: CHẶN` (student name, score and parent phone in a notice) | A pass, B pass |

Totals: run A 20 of 20 `blocked` with requester mention (5 with a second self-triggered run). Run B 16 of 20 `done` with `VERDICT: CHẶN` and the planted defect named. 4 run B failures: Accounting (no verdict), Officeops (no verdict), Real Estate (blocked with verdict), Trade (in_progress with verdict).

## Defects

- **D1. Lead re-runs on its own stop comment (5 cases: KVM-7, KVM-9, KVM-17, KVM-27, KVM-31).** After the lead posts the stop-1 comment, the request is woken again. The second run posts only a "no reply this turn" comment. This doubles agent cost for those requests. Suspected file: the comment-triggered wakeup path in `server/internal/service/` (task enqueue on agent-authored comments, and the issue wakeup logic), not verified. The lead's own comment should not wake the lead.
- **D2. Gate sub-issue left `in_progress` after a CHẶN verdict (KVM-34, Trade).** The verdict line was posted, but the sub-issue never moved to `done` (283 s, then the run ended). Suspected: gate role instructions in `packages/core/squads/kitviet-skills-data/` or `kitviet-agent-instructions.ts` (the `done` write for a CHẶN verdict), or the run ending before the status write. Not verified.
- **D3. Gate gives no verdict when the draft is inline text, not an attached file (KVM-8 Accounting, KVM-10 Officeops; also KVM-14 and KVM-30 cite a missing attachment as the reason).** KVM-8 and KVM-10 set `blocked` with no `VERDICT:` line. Runbook section 5 says a missing VERDICT counts as CHẶN, so this is a contract gap. Suspected: the shared numbers-gate skill (`soat-so-lieu`) and the "Cần giấy tờ" attachment rule in `packages/core/squads/kitviet-skills-data/common.ts`. Part of this is my drill design: I put drafts in the issue description, not as attachments. The gate should still emit a VERDICT line (CHẶN) rather than an unlabelled stop. Decide whether gates accept inline drafts.
- **D4. Real Estate listing gate (KVM-30): CHẶN with status `blocked`, and planted promises not named.** The gate posted `VERDICT: CHẶN` but set the sub-issue to `blocked` instead of `done`, and its reason stops at the missing source, so the 20% profit and 24-hour loan claims are not reported. Suspected: same area as D2 plus gate check ordering. Not verified.
- **D5. Lead stop comment does not start with the `BLOCKED · stop 1` header in two cases (KVM-3 Marketing, KVM-13 PR).** Both start with a plan, then the stop. Status `blocked` and the mention are correct. Suspected: lead stop template in `packages/core/squads/kitviet-agent-instructions.ts` or the lead notes in `kitviet-templates/*.ts`. Minor.

## Not verified

- Gate checklists were not read line by line. Each planted defect was chosen from the runbook description of the gate.
- Run B used one sub-issue per squad with no parent. Stage barrier wakeups were not exercised.
- No full end-to-end run for any squad (out of scope for this pass).
- Accounting F19 test (two 200-row tables) and the Go test suite were not run.
- The Monitor watch on drill.log was left armed; it expires on its own.

## Follow-up status (D1 to D5)

Recorded after the follow-up change. Nothing below was run again on a live stack, and no live agent was called. The text changes are checked by guard tests that read wording only. The Go test for the unmatched-task path was written but did not run, because no dedicated test database was available. See the runbook, section 9, for the same table in Vietnamese.

| Defect | Status | What changed | Not verified |
| --- | --- | --- | --- |
| D1. Lead re-runs on its own stop comment | Fixed for the squad-assigned route (`89ca0c07e`, on main). Cause of the five live cases (KVM-7, 9, 17, 27, 31): the lead's own stop comment carried a `mention://agent/<lead id>` link, which the delivery core told the lead to write. Each such mention queued a new run of the lead. Text fixed. | `CreateComment` does not wake the assigned squad leader for the leader's own stop while the request is `blocked` or `in_review` (`89ca0c07e`). `kv--delivery-core` no longer shows the lead how to write its own mention and forbids a mention of the lead in its own stop comment. Guard test `kitviet-lead-self-mention.test.ts`, red before the change. The unmatched `X-Task-ID` branch was removed: it is not reachable over HTTP. | No live re-drill, so it is not known whether the lead still writes its own mention. The server still allows self-mention on purpose; see the runbook, section 9. |
| D2. Gate sub-issue left `in_progress` after CHẶN | Text fix (`4ddd14f7f`): a gate ends `done` after its VERDICT, CHẶN included, and the status change is its last command. | No server change. | No live re-drill. The last-command rule is text only. A run that ends before that command still leaves the stage open. |
| D3. No VERDICT line for an inline draft | Text fix (`4ddd14f7f`), plus the description case in the delivery core: text in a comment or a description is not a draft. With no attached draft, the gate posts `VERDICT: CHẶN - không có bản nháp đính kèm` and ends `done`. | Decided: gates do not accept inline drafts (runbook, section 9). | Guard test `kitviet-verdict-line.test.ts`, which covers the no-draft line and the inline rule. No live re-drill. |
| D4. Real-estate gate set `blocked` and missed promises | Text and patch fix (`4ddd14f7f`): the gate ends `done`, and each guarantee or profit promise is its own finding (`soat-tin-dang-bds.json` and related patches). | No server change. | No live re-drill. |
| D5. Lead stop comment starts with a plan | Text fix: a stop is its own comment, and its first line is the header. The rule is in `kv-squad-lead/SKILL.md`, the lead kind text in `kitviet-agent-instructions.ts`, and the Marketing and PR lead notes. | Guard test `kitviet-stop-header.test.ts`. | No live re-drill. |

Also changed: the gate kind text now states the one-line VERDICT rule in the same words as `kv-delivery-core` (guard test `kitviet-verdict-line.test.ts`). The guard reads the wording only. No gate output is tested.

Test results for this change:

- `node --test scripts/generate-kitviet-pack.test.mjs`: 121 pass, 0 fail.
- `node scripts/generate-kitviet-pack.mjs --check`: up to date (29 files).
- `pnpm exec vitest run packages/core/squads packages/core/agents`: 61 files, 1522 tests pass. The `company-template-data` test failed once after the instruction text changed. `pnpm generate:company-template` regenerated `server/internal/service/company_template_data.json`, which changed only the lead and gate instruction texts. The rerun passed.
- `pnpm -C packages/core exec tsc --noEmit`: exit 0.
- `go build ./...` (server): exit 0. `go vet ./internal/handler/`: exit 0. `gofmt -l` on the three changed Go files: clean.
- `go test -run TestSquadLeaderSelfWakeSuppressed ./internal/dispatch/`: pass (7 subtests).
- `go test -run TestCreateComment_SquadLeaderStopCommentDoesNotSelfWake` and `…UnmatchedTaskDoesNotSelfWake` in `./internal/handler/`: not run. The handler `TestMain` uses `DATABASE_URL`, and falls back to the dev database when it is unset. Running it there would write fixtures to a shared database, and `make test` migrates its target database first. A dedicated test database is needed.

## Second follow-up: D1 live path and runbook section 9

Still nothing run on a live stack, and no real agent was called. The read-only SQL below ran against the drill database `multica_multica_kitviet_332`.

What the drill database shows for the five D1 cases (KVM-7, 9, 17, 27, 31):

- Each lead run that followed a stop was created by the trigger comment, `trigger_evidence_kind = 'comment'`, and the trigger comment was the lead's own stop comment (`author_type = 'agent'`, `author_id` equal to the run's agent).
- Each stop comment contained `mention://agent/<lead id>`, the lead's own mention, copied from the resume rule in `kv--delivery-core`.
- The unmatched `X-Task-ID` path was removed from the code. Over HTTP it is not reachable: `Auth` and `DaemonAuth` strip client `X-Agent-ID` and `X-Task-ID`, and the `mat_` branch sets them with `X-Actor-Source=task_token`, which `resolveActor` trusts first.
- The committed fix `89ca0c07e` covers the squad-assigned fallback. The drill assigned each lead request directly to the lead agent, so that fallback was not the route. The self-mention route is `resolveMentionedAgentCommentTriggers`, which allows self-mention on purpose. The text was changed, not the server.

Changes in this step:

- `server/internal/handler/squad.go`, `comment.go`, `squad_leader_stop_comment_test.go`: the unmatched-task helper, its wiring and its test were removed. These three files now match `89ca0c07e`.
- `packages/core/squads/kitviet-skills/kv-delivery-core/SKILL.md`: the resume rule no longer gives the lead's mention as text to copy. A stop comment never mentions its own agent. Inline text in a comment or a description is not a draft (D3).
- `packages/core/squads/kitviet-skills-data/common.ts`: regenerated with `node scripts/generate-kitviet-pack.mjs`.
- `packages/core/squads/kitviet-lead-self-mention.test.ts` (new) and `kitviet-verdict-line.test.ts` (D3 guards). The self-mention test was run before the wording change and failed on all three assertions.
- `docs/engineering/kitviet-runbook.md`, section 9: each open item is fixed or decided, with the reason.

Checks run for this step, with the results:

| Command | Result |
| --- | --- |
| `node scripts/generate-kitviet-pack.mjs --check` | `kitviet pack data is up to date (29 files)`, exit 0 |
| `node --test scripts/generate-kitviet-pack.test.mjs` | 121 pass, 0 fail |
| `pnpm exec vitest run packages/core/squads packages/core/agents` | 63 files, 1569 tests pass |
| `pnpm -C packages/core exec tsc --noEmit` | exit 0 |
| `cd packages/core && pnpm exec eslint squads/kitviet-lead-self-mention.test.ts squads/kitviet-verdict-line.test.ts` | exit 0 |
| `cd server && go build ./...` | exit 0 |
| `cd server && go vet ./internal/handler/` | exit 0 |
| `gofmt -l` on `squad.go`, `comment.go`, `squad_leader_stop_comment_test.go` | no output |
| `go test -run TestSquadLeaderSelfWakeSuppressed ./internal/dispatch/ -count=1` | pass, 7 subtests |
| `DATABASE_URL=<multica_multica_51> go test -run 'TestCreateComment_SquadLeaderStopCommentDoesNotSelfWake$' ./internal/handler/ -count=1` | pass at HEAD |
| The same test on a copy of `89ca0c07e^` in `/tmp` | fail: `queued 1 leader tasks, want 0` |
| `DATABASE_URL=<multica_multica_51> go test -run 'TestNotification_TaskFailed' ./cmd/server/ -count=1` | 3 pass |
| `git diff --check` | exit 0 |

The DB tests ran on the isolated database of this checkout (`multica_multica_51`, from `.env.worktree`). They did not run on the dev database `multica`.

Not verified: a live re-drill of D1 to D5, the D1 self-mention path under a real agent, and the handler test for the squad-assigned unmatched path, which no longer exists.

## Third follow-up: items 1 to 7 (2026-11-12)

Live steps ran on the drill stack. Four real agent runs were made, the number the user authorised: two gate re-runs and two lead runs. Each request had one run and no retry.

### Environment

- API: pid 2671233, binary `/tmp/kv-drill/bin/server-fix`, log `/tmp/kv-drill/api-fix.log`. Started detached with `setsid nohup`. `/health` reports `commit f27661e8a+kv-drill-fixes`.
- Build: `git archive f27661e8a` into `/tmp/kv-drill/export-fix`, then the changed hunks only (below). The working tree was not used as the base. Not built from `fa07c0ca3`: HEAD adds migrations 973 to 977 (commits `2e4fafc3d`, `d56d8f516`), and the drill DB is at 972. Those migrations were not applied to the test DB, so the API is not HEAD.
- `api.env` was read to load the environment and deleted after the start. No secret was printed.
- Daemon: pid 3854682 still running (cwd `multica-kitviet`, profile `dev-multica_kitviet-332`), checked after the test runs.
- Workspace `kitviet-mau` (`e56512a8-bd7a-48be-b0a3-09db73e57d42`). The CLI profile points to another workspace, so every command set `MULTICA_WORKSPACE_ID`.

### Item 1: gates with the draft attached

| Old | New | Gate | Status | First line | Planted promises named |
|---|---|---|---|---|---|
| KVM-34 | KVM-74 | Trade Customs Gate `e035ef53` | `done` | `VERDICT: CHẶN - thiếu biểu thuế; thuế FTA 0% và "không cần giấy phép" chưa có căn cứ` | yes: FTA 0% and no-licence |
| KVM-30 | KVM-75 | Realestate Listing Gate `25a36a88` | `done` | `VERDICT: CHẶN - có cam kết lợi nhuận 20%/năm và thiếu nguồn sổ` | yes: 20% profit and loan in 24 hours |

Each new sub-issue has the draft as an attached file (`gate-drafts/KVM-34-draft.txt`, `gate-drafts/KVM-30-draft.txt`). Its description holds no draft text. One run each, trigger `issue_assignment`. KVM-75 repeats `VERDICT: CHẶN` in its conclusion. The first line is correct, and the rule is one VERDICT line, so this is a residual wording issue, not fixed here.

### Item 2: no plan comment before a stop (D5)

- Cause: `kv-squad-lead/SKILL.md` (Templates) said "A plan from step 3 is a separate comment, posted before the stop", and the lead kind text said the same. The lead followed this text.
- Text change: step 3 now gives way to a stop. The Templates section says no `## Plan` and no summary is posted before a stop. Same rule in the lead kind text (`kitviet-agent-instructions.ts`) and in the Marketing and PR notes.
- Guard test (red first, 4 assertions failed on the old text): `kitviet-stop-header.test.ts`.
- Live check: KVM-76 (Office Ops lead `2a90df7c`) and KVM-77 (Marketing lead `dbadf4b2`). Each has one agent comment, first line `BLOCKED · stop 1 · thiếu căn cứ`, no `## Plan` comment before it, no self mention, one member mention. Each request has one run.
- Lead texts in the workspace: `upgradeKitvietSquad` (dry run, then apply) updated 21 lead agents and 21 squads, and 21 squad skills are `ready`. A second dry run found 187 of 187 current.

### Item 3: orphan KVM-41

- Cancelled in `kitviet-mau` with a comment that gives the reason (client timeout after the DB write, enqueue lost to `context canceled`, KVM-52 re-runs the scenario). Not deleted.
- Commands: `multica issue comment add <id> --content ...`, then `multica issue status <id> cancelled`. Result: `KVM-41 cancelled`.

### Item 4: enqueue after an issue create uses the request context

- Test first: `server/internal/service/issue_create_enqueue_test.go`, `TestCreateAgentIssueQueuesRunWhenRequestCancelsAfterCommit`. The request context is cancelled right after the commit, through the existing `afterCommitTxStarter`. Red before the fix: `the assigned run was not queued after the request was cancelled`.
- Fix: `IssueService.Create` runs the post-commit enqueue steps (`maybeEnqueueOnAssign`, the deferred-channel squad path, and the overlay hydrate) on `context.WithTimeout(context.WithoutCancel(ctx), issueEnqueueTimeout)`, with `issueEnqueueTimeout = 30s`.
- Green: working tree and the clean export both pass the test.
- Other enqueues in the handler: `handler/issue.go` calls only `IssueService.Create` on this path. `EnqueueQuickCreateTask` (quick create, line ~2961) uses `r.Context()` and has the same pattern, but it is another endpoint. Not changed.

### Item 5: pool exhaustion (not fixed)

- Evidence from `/tmp/kv-drill/api-f276.log`: `db pool pressure` at 05:47:03 to 05:48:33 showed `acquired_conns=25 idle_conns=0`, `empty_acquire_delta` in the hundreds, and `avg_acquire_ms` up to 2086. Before that, at 05:46:48, `acquired_conns=22` after a burst of 1825 acquires. The 500s on `supplements/claim`, `messages` and `daemon/ws` were waits for a connection (`context canceled` after 5 to 18 s).
- Code review found no path that holds a pooled connection across the network on the drill path. Verified:
  - `selfhosttelemetry/store.go`: one connection kept for the leader session (by design).
  - `service/knowledge.go` `lock()`: one connection held for a whole `Enable`, which calls WeKnora over HTTP. A clear hold across the network, but knowledge is disabled in this stack (`KNOWLEDGE_URL not set`), so it is not on this path.
  - `service/budget_gate.go`: each refused claim candidate is a rolled-back transaction, up to 16 per claim. Short, not a hold.
  - `service/issue_wakeup_system.go` `dispatchSystem`: the transaction uses `SET LOCAL lock_timeout = '50ms'` and only transaction-bound queries. Its 14.5 s pass is not explained by this read.
- Not found: the holder of the 25 connections for about 90 seconds. The claim transaction (`claim task: timeout` after 3.7 s with `claim_agent_ms=328`) is not explained either.
- Decision: no fix, no pool-size change. Next step: a `pg_stat_activity` snapshot (`state`, `wait_event`, `query` grouped by `application_name`) during a load of 11 or more runs, and a read of `ClaimAgentTask` lock order. The knowledge lock is a separate, clear case (a pool connection held across HTTP); fixing it needs a decision.

### Item 6: upgrade registry filled

- `packages/core/squads/kitviet-previous-texts.ts`: 70 role keys with 142 earlier agent texts, and 44 earlier squad texts. Sources: the dumps of `1e2ba5a19`, `1cae0e31e`, `ac8c06003` (`/tmp/kv-drill/texts-*.json`), and the `f27661e8a` texts of the 21 lead roles and 21 squads that item 2 changed. Generated by a one-off script from those dumps; no entry was edited.
- Tests (`packages/core/squads/upgrade.test.ts`, describe `upgradeKitvietSquad`, block "with the texts recorded in kitviet-previous-texts.ts"): a marketing agent and squad that hold a recorded text are upgraded; the same with a hand-added line are kept; every recorded key is a Kit Viet role, and no entry equals the current text.
- Red first: with the registry empty, the two behaviour tests fail (`expected undefined to be defined`, then `upgraded` expected). Green with the registry filled: 21 of 21 tests pass.
- Limit: texts shipped before `1e2ba5a19` are not in the registry. A workspace still on one of those texts is kept, not upgraded.
- `kitviet-skills-data/common.ts` regenerated with `node scripts/generate-kitviet-pack.mjs`. `server/internal/service/company_template_data.json` regenerated with `pnpm generate:company-template`: one line changed (the lead kind text).

### Item 7: verification

See the list in the runbook section 9. Summary:

- `pnpm lint`: exit 0.
- `pnpm typecheck`: exit 1. All 14 errors are in `packages/views/workflow/` and its locales, untracked work from another session. None in changed files. `pnpm -C packages/core exec tsc --noEmit`: exit 0.
- `pnpm test`: exit 1 in both runs. Run 1: three timing failures under parallel Go load (`text-contrast`, `image-sequence.performance`, `sdlc-harness-check`), and in run 2 `kitviet-skills.test.ts` "matches what scripts/generate-kitviet-pack.mjs produces now" (5 s timeout). Each passed when run alone. Desktop: 3 files fail to resolve imports to `workflows/workflows-tab` and `workflows/handoff-confirm-dialog`, files from another session.
- Go: `internal/handler` passes (`ok`, 489 s). `cmd/multica` daemon-stop subset passes. `internal/service` fails 3 tests: `TestProbationWorseForHireAndRetire`, `TestProbationJudgesTheWindow` (fail on the pristine `f27661e8a` tree too), `TestWakeupConditionPullRequestCoversEveryVCSProvider` (passes on the clean export, fails in the dirty tree). `cmd/server` `TestNotification_CurrentMembership` fails on the pristine tree too: duplicate `user_email` in the shared DB.
- Expert-review flag: `packages/views/modals/squad-quickstart.test.tsx` 15 of 15 pass. No browser check was run; it needs a running web app on the drill stack.

### Files changed in this follow-up

Kit Việt: `packages/core/squads/kitviet-previous-texts.ts`, `upgrade.test.ts`, `kitviet-stop-header.test.ts`, `kitviet-agent-instructions.ts`, `kitviet-skills/kv-squad-lead/SKILL.md`, `kitviet-skills-data/common.ts` (generated), `docs/engineering/kitviet-drill-report.md`, `kitviet-runbook.md`.

Server, shared with other sessions:

- `server/internal/service/issue.go`. Hunks added by this change: (1) the `issueEnqueueTimeout` constant above the `Create` doc comment; (2) the `enqueueCtx` block after the commit; (3) four call sites changed from `ctx` to `enqueueCtx` (`hydrateDeferredChannelIssueTaskOverlay`, `shouldEnqueueSquadLeaderOnAssign`, `enqueueSquadLeaderTask`, `maybeEnqueueOnAssign`). The file already had another session's changes; they are not part of this change. The export `/tmp/kv-drill/export-fix` contains only these hunks on top of `f27661e8a`.
- `server/internal/service/company_template_data.json` (generated, one line).
- `server/internal/service/issue_create_enqueue_test.go` (new).

Not in this change: `server/internal/handler/issue*.go`, `squad_workflow*.go`, migrations 973 to 977, and other files of the dirty tree.

## Fourth follow-up: remaining items (2026-11-12)

This section supersedes three statements in the third follow-up: item 4 (quick create "not changed"), item 5 (pool, "no fix"), and the registry counts in item 6.

### Item 1: quick create enqueue uses the request context (fixed)

- Premise check: quick create has no commit-then-enqueue step. Its one write is the task insert. The work after the insert used the request context: `RecordHumanDelegation` (the member delegation ledger) and `NotifyTaskEnqueued` (queued analytics). A client that disconnected after the insert left the task without its ledger row, and no error was returned.
- Test first: `server/internal/service/quick_create_enqueue_test.go`, `TestEnqueueQuickCreateTaskRecordsDelegationWhenRequestCancelsAfterInsert`. A DBTX wrapper cancels the request context right after the task insert. Red before the fix: `member_delegation_day rows = 0, want 1` and the log line `record member delegation day failed error="context canceled"`.
- Fix: `enqueueQuickCreateTask` in `server/internal/service/task.go` derives `enqueueCtx` with `context.WithTimeout(context.WithoutCancel(ctx), issueEnqueueTimeout)` after the task is durable, and passes it to the two follow-up calls. The checks before the insert keep the request context. The source-context path (`EnqueueQuickCreateTaskWithSourceContext`) shares the function, and its commit happens before the detached context starts.
- Green after the fix. The handler is unchanged.

### Item 2: knowledge lock holds a connection during HTTP (not fixed)

- Measured with a temporary probe, then deleted. Wrapping the knowledge client and reading `pool.Stat().AcquiredConns()` at the first WeKnora call (`EnsureTenant`) gave 1, with a baseline of 0. `lock()` in `server/internal/service/knowledge.go` holds one dedicated connection for the whole `Enable`.
- Why no change: the lock has to cover every HTTP step, and a Postgres session advisory lock lives on its connection. Releasing the connection during HTTP would drop mutual exclusion. The smaller options fail: an in-process mutex does not protect a deployment with several server instances. A compare-and-set on `workspace_knowledge.state` needs a stale-lease rule, and that table has no `updated_at`, so the lease needs a migration. Migrations were not applied in this run.
- Exposure: at most one held connection per workspace that is enabling. A second `Enable` for the same workspace gets `ErrKnowledgeBusy` and releases its connection. `RunAutoEnable` loops over workspaces one at a time. Knowledge is disabled on this stack.
- Decision needed: a lease in the database with a migration, or keep the current hold.

### Item 3: connection pool size (decided: unchanged)

- The pool stays at 25. Recorded evidence: in the window 05:47:03 to 05:48:33 of the drill, the pool showed `acquired_conns=25 idle_conns=0` (third follow-up, item 5). The report records 45 agent runs in total: 25 in run A and 20 in run B (first section).
- Not measured: a load test with `pg_stat_activity` (`state`, `wait_event`, `application_name`) is still needed before any pool change.
- Known holder not fixed: the knowledge lock in item 2 can hold one connection during HTTP when knowledge is enabled. It is not confirmed as a cause of the saturation, because knowledge is disabled on the drill stack.

### Item 4: upgrade registry (extended)

- Git history: the first Kit Việt commit on HEAD's history is `aa996c9bb` (2026-11-10). `1e2ba5a19` (2026-11-11) and `1cae0e31e` are not ancestors of HEAD. No release tag contains any Kit Việt commit: `git tag --contains aa996c9bb` is empty, and the newest tag, `v0.8.1`, is dated 2026-11-06. "Shipped" in the registry therefore means "on main".
- Added 138 texts for 76 role keys to `packages/core/squads/kitviet-previous-texts.ts`. Each is the full composed instruction of a role that a later commit replaced, and no entry existed for it:
  - `f27661e8a`, gate roles: 48. Replaced by item 5 in this change.
  - `1c604d8ad`, gate roles: 48. Replaced at `2375d7ba1`.
  - `e787f2217`: 42 (14 gate, 5 lead, 23 collector, drafter and handover). Replaced at `1c604d8ad`.
- Method: `git archive` of each commit, bundled with esbuild. `loadKitvietRoleInstructions` gave the full text of every role, compared per role key. No existing entry was edited.
- Evidence of use: 53 agent rows in the drill database `multica_multica_kitviet_332` (read only) have instructions equal to the added texts: 48 `f27661e8a` gate, 2 `1c604d8ad` gate, 3 `e787f2217` collector, drafter or handover.
- Still not in the registry: (a) texts from before `e787f2217`, the generation that has no `composeKvAgentInstructions` (`aa996c9bb` to `7a93d1136`); not reproduced. (b) 4 agent rows in the drill database match neither the current text nor any registry text. Their origin is not determined.
- Size: the file grows from 1.49 MB to 2.62 MB. `upgrade.ts` has no production caller yet, so no app bundle includes it now. Storing a hash of each text would cut the size. That changes the structure, so it is not done here.
- Tests: `packages/core/squads/kitviet-previous-texts.test.ts`, 3 tests. Red before the entries were added: `admin_format_gate: expected false to be true`. Green after.

### Item 5: duplicate VERDICT line in the gate conclusion (fixed)

- Cause (KVM-75): the gate rule required one VERDICT line but did not forbid repeating it in the conclusion.
- Text changes: the `kv_gate` kind text in `kitviet-agent-instructions.ts` and the gate rules in `kitviet-skills/kv-delivery-core/SKILL.md` now say the VERDICT line appears once, as the first line, and the conclusion does not repeat it. `kitviet-skills-data/common.ts` and `server/internal/service/company_template_data.json` are regenerated.
- Test: `packages/core/squads/kitviet-verdict-line.test.ts`, describe block "the gate's conclusion does not repeat the VERDICT line". Red before: 2 tests failed. Green after.
- Not verified on a live gate run.

### Item 6: expert-review UI in a browser (not done)

- Not started. The web dev server imports `packages/views`, which contains another session's uncommitted workflow files (typecheck errors in `packages/views/workflow/`). Checking the expert-review UI needs a squad created on the test API, which is a write to a test workspace this run did not authorise.

### Verification (this run)

| Check | Command | Result |
|---|---|---|
| Kit Việt pack is up to date | `node scripts/generate-kitviet-pack.mjs --check` | exit 0, "up to date (29 files)" |
| Generator tests | `node --test scripts/generate-kitviet-pack.test.mjs` | 121 pass, 0 fail |
| Squads and agents | `pnpm exec vitest run packages/core/squads packages/core/agents` | 64 files, 1581 tests, all pass |
| Core types | `pnpm -C packages/core exec tsc --noEmit` | exit 0 |
| Lint, changed TS files | `pnpm exec eslint` from `packages/core` on the 5 changed TS files | exit 0 |
| Go build | `go build ./...` in `server/` | exit 0 |
| Go vet | `go vet ./internal/service/ ./internal/handler/` | exit 0 |
| gofmt | `gofmt -l` on the changed Go files | `task.go` listed. The one hunk (line 8185) is in code that is not changed here, and the file was not formatted at HEAD either. |
| Targeted service tests | `go test ./internal/service/ -run 'Enqueue\|QuickCreate\|Knowledge\|CreateAgentIssueQueues' -count=1`, `DATABASE_URL` = `multica_multica_51` | 58 pass, 0 fail |
| Quick-create handler tests | `go test ./internal/handler/ -run QuickCreate -count=1`, same DB | 12 pass, 0 fail |
| Full service package | `go test ./internal/service/ -count=1`, same DB | 878 pass, 2 fail, 20 skip. The 2 failures are `TestProbationWorseForHireAndRetire` and `TestProbationJudgesTheWindow`, listed as failing on the clean export. |

The test database for Go was `multica_multica_51` (schema at migration 977). The dev database `multica` was not used. `make test` and `make check` were not run.

### Files changed in this follow-up

- Go: `server/internal/service/task.go` (new hunks: `enqueueCtx` and the two follow-up calls), `server/internal/service/quick_create_enqueue_test.go` (new), `server/internal/service/issue.go` (the issue-create hunks from the earlier change, plus the constant's comment, which now also names the quick-create writes), `server/internal/service/company_template_data.json` (generated, one more line changed).
- Kit Việt: `packages/core/squads/kitviet-agent-instructions.ts`, `kitviet-skills/kv-delivery-core/SKILL.md`, `kitviet-skills-data/common.ts` (generated), `kitviet-verdict-line.test.ts`, `kitviet-previous-texts.ts`, `kitviet-previous-texts.test.ts` (new).
- Docs: `docs/engineering/kitviet-drill-report.md`, `docs/engineering/kitviet-runbook.md`.
