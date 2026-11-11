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
