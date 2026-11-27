# Delegora bền vững: kế hoạch triển khai

> **Cho agent thực thi:** dùng superpowers:subagent-driven-development (khuyến nghị) hoặc superpowers:executing-plans để làm từng task. Bước dùng ô `- [ ]` để theo dõi.

**Mục tiêu:** xây 12 tính năng F1 đến F12 của [spec.md](spec.md) theo năm giai đoạn A đến E, mỗi giai đoạn sau một cờ tính năng mặc định tắt.
**Kiến trúc:** bảng mới, service, handler, job và CLI ở file `*_fork.go`; hook một dòng vào file upstream; core ở `packages/core/durable/`, view ở `packages/views/durable/`, gắn vào trang Governance.
**Công nghệ:** Go (Chi, sqlc, pgx v5), PostgreSQL, TypeScript (TanStack Query, zod), React, Playwright.
**Đặc tả:** [spec.md](spec.md). Chiến lược: [strategy.md](strategy.md). Đọc spec trước; plan này không nhắc lại lý do. Mã `F`, `D`, `U` là của spec.

## Ràng buộc chung

1. Làm trực tiếp trên `main` của checkout chính; không worktree, không nhánh tính năng trừ khi cần build sạch. Commit chỉ khi người dùng yêu cầu. Không mở PR vào `multica-ai/multica`; PR (nếu có) dùng `--repo TuPK/multica`.
2. Mỗi task là một commit conventional nguyên tử, để cây xanh với kiểm tra của task đó.
3. Không khoá ngoại, không cascade. Mỗi chỉ mục là một file migration `CREATE [UNIQUE] INDEX CONCURRENTLY` riêng và được đăng ký trong `concurrentIndexCleanups` (up) và `concurrentDownIndexCleanups` (down) của `server/cmd/migrate/main.go`. Số migration: số lớn nhất hiện tại + 1, đánh lại ngay trước khi đẩy; chạy `cd server && go test ./cmd/migrate ./internal/migrations`.
4. Sau mọi thay đổi SQL: `make sqlc`.
5. Test Go có DB dùng chung DB test; **không bao giờ dùng DB `multica`**. Dựng DSN từ `.env` trong chính lệnh, trỏ tới DB test riêng (theo thói quen `multica_engine_test` của repo); trước khi coi một lỗi là hồi quy, kiểm dòng rác còn lại của lần chạy trước. Khẳng định trên dòng của fixture, không trên tổng.
6. Trước `vitest`, `tsc`, `go test` chạy ngoài turbo hoặc make: `pnpm catalog:build`.
7. Mọi chuỗi hiển thị mới có bản dịch ở cả 9 locale (`en`, `es`, `fr`, `ja`, `ko`, `ru`, `vi`, `zh-Hans`, `zh-Hant`); `packages/views/locales/parity.test.ts` phải xanh. Hiển thị là **Delegora**, định danh giữ `multica`.
8. Tính năng sau cờ mặc định tắt: không viết vào `apps/docs/content/docs/` hay `README*.md`.
9. Mỗi hook vào file upstream: đúng một dòng gọi sang file fork, và được thêm vào hợp đồng sync trong cùng commit (`pnpm sync:contract`, `pnpm sync:coverage` xanh).
10. Mọi JSON giao diện dùng đi qua zod + `parseWithFallback`, mỗi endpoint mới có test phản hồi sai dạng.
11. Báo đúng cái gì đã chạy và cái gì bỏ qua.

## Trọng tâm rà soát

Những trường hợp spec ngầm định mà dễ hỏng nhất với người dùng thật. Mỗi dòng có test ở task sở hữu (ghi trong ngoặc).

1. **Issue đổi trạng thái qua đường không ai nghĩ tới** (channel action, PR auto-complete, flow rollback, batch): phải để lại đúng một transition, nếu không uy tín sai âm thầm (A1: `TestEveryStatusWriterRecordsOneTransition`).
2. **Hai người duyệt cùng một ý định gần như cùng lúc**: lời gọi ra ngoài chỉ được chạy một lần (B4: `TestApproveIntentConcurrentlyExecutesOnce`).
3. **Agent đổi model giữa chừng một cửa sổ chốt**: kết quả giữ model lúc tạo, uy tín không trộn (D3: `TestOutcomeKeepsModelAtCreation`).
4. **Workspace có một squad và một agent duy nhất**: không có ai khác để kiểm hay làm juror; phải ra `unable` trung thực, không treo và không để người làm tự kiểm (A3: `TestNoEligibleVerifierYieldsUnable`; D1: `TestJuryWithOneAgentEscalates`).
5. **Người tắt cờ khi dữ liệu đang dở** (kết quả `pending`, ý định `pending`, shadow `running`): không có job nào tiếp tục ghi; bật lại thì tiếp tục từ chỗ dừng (A2, B4, D3: `Test*FlagOffStopsWrites`).

---

## Trạng thái thực tế (cập nhật 2026-11-27)

Bảng này ghi trạng thái theo mã nhánh trên `durable-platform` (tip `4ba65dade4`), đối chiếu với file, route và test có trong cây. Các ô `[ ]` trong từng task chưa được tick lại; mỗi task chỉ được tick khi phiên làm task đó xác nhận từng bước.

| Task | Trạng thái | Bằng chứng / còn thiếu |
|---|---|---|
| P0.1–P0.3 | Xong | `p0-findings.md`, `packages/core/durable/flags.ts` |
| A1–A9 | Đã merge | `packages/views/durable/outcomes`, `e2e/durable/outcomes.spec.ts`. Chưa có `TestEveryStatusWriterRecordsOneTransition` (trọng tâm rà soát 1) |
| B1–B4 | Đã merge | `durable_intent_fork.go`, `connected_apps_intent_fork_test.go` (test chốt một lần thực thi). Chưa có `durable_intent_fork_test.go` đúng tên plan |
| B5 | Chưa đủ | Có `durable_flow_effects_fork.go`; chưa xác nhận trường đánh dấu hệ quả trong manifest flow (U7), chưa xác nhận kiểm tra catalog validate |
| B6 | Đã merge | `packages/views/durable/effects` |
| B7 | Đã merge | `e2e/durable/effects.spec.ts` |
| C1 | Một phần | `actual_seconds` có trong attention; `attention.minutes_per_done` chỉ thấy trong digest, chưa thấy trong attention |
| C2–C4 | Đã merge | Principal, dự đoán, học, uỷ quyền, phỏng vấn (merge `35b8a186e4`) |
| C5 | Chưa làm | Không có UI principal, dự đoán, phỏng vấn |
| C6 | Chưa làm | Không có E2E giai đoạn C |
| D1 | Đã merge | `durable_jury_fork.go`, `TestJuryWithOneAgentEscalates` |
| D2 | Đã merge | Hồ sơ ghế (merge `235d48f272`) |
| D3 | Đã merge | Shadow run, merge `4ba65dade4` |
| D4 | Đã merge | `runtime_change` và tự hoàn tác (trong nhánh D3) |
| D5 | Một phần | `durable_necessity_fork.go` có; chưa thấy route hay job scaffolding sunset |
| D6 | Chưa làm | Không có UI hồ sơ ghế, shadow, jury |
| D7 | Chưa làm | Không có E2E giai đoạn D |
| E1 | Một phần | `durable_reach_fork.go` có; chưa thấy route và job scheduler reach |
| E2 | Một phần | Export đã có dossier lịch sử và principal theo đồng ý; reader import chưa nhập `history/dossiers.ndjson` (spec mục 6.6). |
| E3 | Một phần | `durable_attestation_fork.go` có; chưa thấy route `POST /api/attestations/verify`, well-known keys, lệnh CLI |
| E4 | Chưa làm | Không có offer ghế trên Hub |
| E5 | Chưa làm | Không có UI giai đoạn E |
| F1 | Chưa làm | Chạy sau mỗi giai đoạn |
| F2 | Chưa làm | Bật thử trên máy chủ dev, chỉ khi người dùng đồng ý |

## P0. Nền

Mục đích: xác nhận các giả định U1 đến U10 của spec mục 15 và ghi nền kiểm tra trước khi xây. Không đổi mã sản phẩm.

**Trạng thái: xong.** Kết quả ở [p0-findings.md](p0-findings.md). Các đề xuất đã được áp dụng thành D26 đến D31 (spec mục 0 và 15).

### Task P0.1: Ghi nền kiểm tra

- [ ] Chạy và lưu kết quả vào scratchpad: `pnpm catalog:build`, `pnpm typecheck`, `pnpm --filter @multica/core exec vitest run governance`, `pnpm --filter @multica/views exec vitest run governance`, và Go: `go test ./internal/service -run 'Autonomy|Scorecard|ApprovalPolicy|Governance|Probation|Budget' -count=1` trên DB test.
- [ ] Mục nào đỏ ở HEAD: ghi là đỏ sẵn, không sửa trong P0.

### Task P0.2: Xác nhận giả định

Ghi kết quả từng mục vào `docs/specs/durable-platform/p0-findings.md` (mới) với bằng chứng (`file:line`, lệnh, đầu ra).

- [ ] **U1**: `rg -n "UpdateIssueStatus|SetIssueStatus|status_id" server/internal/service server/internal/handler server/internal/issueworkflow` và liệt kê mọi hàm ghi `issue.status_id`. Kết luận: một hàm chung hay nhiều.
- [ ] **U2**: đọc `server/migrations/001_init.up.sql` phần `activity_log` và các nơi ghi action trạng thái; kết luận backfill được hay không.
- [ ] **U3**: `rg -n "func .*Call|ExecuteTool" server/internal/connectedapps` và đường remote gateway; kết luận một điểm hay nhiều.
- [ ] **U4, U5, U6**: đọc `server/internal/handler/daemon.go` (`ClaimTasksByRuntime`, `buildClaimedTaskResponse`), payload claim ở `server/pkg/agent` và daemon; kết luận về runtime khác, thư mục tạm, trường lạ.
- [ ] **U7**: đọc manifest flow (`catalog/workflows/*/template.json`, `server/internal/issueworkflow/flowdoc_validate*.go`) cho trường hệ quả.
- [ ] **U8**: `rg -n "revert" server/internal/handler/vcs*.go server/internal/service`.
- [ ] **U9**: đọc runtime tuỳ chỉnh trong `server/pkg/agent/builtin_runtimes.go` và tài liệu runtime.
- [ ] **U10**: `rg -n "secret" server/internal/service/*secret*` cho kho bí mật máy chủ.
- [ ] Giả định nào sai: sửa spec (mục liên quan và mục 15) trong cùng commit, trước khi bắt đầu giai đoạn phụ thuộc.

### Task P0.3: Cờ tính năng và khung thư mục

**Files:**
- Create: `server/internal/featureflags/keys_fork.go`, `packages/core/durable/flags.ts`, `packages/core/durable/index.ts`, `packages/views/durable/README.md` (một dòng nói mục đích thư mục)
- Modify: `server/internal/featureflags/keys.go` (một dòng tham chiếu), `packages/core/feature-flags/*` (đăng ký khoá)
- Test: `server/internal/featureflags/keys_fork_test.go`, `packages/core/durable/flags.test.ts`

**Interfaces:**
- Produces: hằng `DurableOutcomes = "durable_outcomes"`, `DurableEffects`, `DurablePrincipal`, `DurableSeats`, `DurableOpen`; `func DurableDependencies() map[string][]string` (bảng spec 14.1); TS `DURABLE_FLAGS` cùng tên.

- [ ] Test đỏ: `TestDurableFlagsDefaultOff` (năm cờ tắt khi không cấu hình); `TestDurableFlagDependencies` (`durable_seats` phụ thuộc `durable_outcomes` và `durable_effects`; `durable_open` phụ thuộc `durable_effects`, `durable_outcomes`).
- [ ] Cài đặt; chạy `go test ./internal/featureflags`; xanh.
- [ ] Commit `feat(durable): feature flags for the durable platform program`.

---

## Giai đoạn A. Bằng chứng (F1, F2), cờ `durable_outcomes`

### Task A1: Lịch sử chuyển trạng thái issue

**Files:**
- Create: `server/migrations/<n>_durable_issue_status_transition.{up,down}.sql`, hai file chỉ mục `<n+1>`, `<n+2>`, file trigger `<n+3>` (`issue_status_transition_ai`, D26); `server/pkg/db/queries/durable_transition.sql`; `server/internal/service/durable_transition_fork.go`
- Modify: `server/cmd/migrate/main.go`. Không sửa từng đường ghi (D26).
- Test: `server/internal/service/durable_transition_fork_test.go`

**Interfaces:**
- Produces: `func WithActorSession(ctx context.Context, q *db.Queries, actor GovernanceActor, taskID pgtype.UUID) error` (đặt `SET LOCAL multica.actor_type|actor_id|task_id`); ; truy vấn sqlc `InsertIssueStatusTransition`, `ListIssueStatusTransitions(issue_id)`, `LatestDoneTransition(issue_id)`; sự kiện nội bộ `IssueEnteredDone{Transition}` và `IssueLeftDone{Transition}` phát sau commit.

- [ ] Test đỏ `TestEveryStatusWriterRecordsOneTransition`: bảng các đường ghi (API cập nhật, CLI, batch, flow advance, workflow transition, PR auto-complete, channel action), cùng `TestStatusTriggerCatchesRawSQL` (UPDATE thô, như sweeper, vẫn để lại đúng một dòng với `system`) và `TestEveryWriterSetsActorSession` (test tĩnh, 24 điểm ghi từ P0 U1); mỗi đường đổi một issue fixture và khẳng định đúng một dòng với khoá (`from_status_key`, `to_status_key`), nhóm đọc qua `ListIssueStatusTransitionRecords` (D32), và `actor_type` đúng.
- [ ] Test đỏ `TestTransitionNotRecordedWhenFlagOff`.
- [ ] Test đỏ `TestReopenDetected`: `done → started` phát `IssueLeftDone` với cờ `reopened = true`; `done → cancelled` có `reopened = false`.
- [ ] Viết migration, truy vấn, `make sqlc`, cài hook; chạy test; xanh.
- [ ] `go test ./cmd/migrate ./internal/migrations`; xanh.
- [ ] Commit `feat(durable): record issue status transitions`.

### Task A2: Kết quả và vòng đời

**Files:**
- Create: migration `work_outcome`, `outcome_verification`, `acceptance_criterion` và các chỉ mục của spec 2.3; `server/pkg/db/queries/durable_outcome.sql`; `server/internal/service/durable_outcome_fork.go`, `durable_work_class_fork.go`; `server/internal/scheduler/jobs_durable_outcome_fork.go`
- Modify: đăng ký job ở scheduler (một dòng), `server/cmd/migrate/main.go`
- Test: `server/internal/service/durable_outcome_fork_test.go`, `durable_work_class_fork_test.go`, `server/internal/migrations/durable_append_only_test.go`

**Interfaces:**
- Consumes: `IssueEnteredDone`, `IssueLeftDone` (A1).
- Produces: `func WorkClassFor(issue db.Issue, labels []db.Label, squadID pgtype.UUID) string`; `type OutcomeService struct`; `func (s *OutcomeService) OnEnteredDone(ctx, tr db.IssueStatusTransition) (db.WorkOutcome, error)`; `func (s *OutcomeService) OnLeftDone(ctx, tr db.IssueStatusTransition) error`; `func (s *OutcomeService) Settle(ctx, now time.Time) (int, error)`; `func DecideVerdict(o db.WorkOutcome, results []CriterionResult, reopened bool) (verdict, severity, cause string)`; sự kiện `OutcomeSettled{Outcome}`, `OutcomeHighFail{Outcome}`.

- [ ] Test đỏ `TestWorkClassPriority`: bảng bốn trường hợp D2, nhãn `wc:` lấy theo tên đầu tiên, giới hạn 200 ký tự.
- [ ] Test đỏ `TestDecideVerdictTable`: đủ các hàng của bảng spec 2.6, gồm `sampled = false` → `unverified`, tiêu chí `unable` không tính.
- [ ] Test đỏ `TestOutcomeLifecycle`: vào `done` tạo một kết quả `pending` với `settle_at = now + 7d`; mở lại → `fail/reopened`; vào `done` lần nữa → bản mới và `superseded_by`; `wc:skip` không tạo.
- [ ] Test đỏ `TestSettleJobSettlesOnlyDue` và `TestOutcomeFlagOffStopsWrites`.
- [ ] Test đỏ `TestOutcomeVerificationAppendOnly`: không truy vấn nào trong `pkg/db/queries` có `UPDATE outcome_verification` hay `DELETE FROM outcome_verification` (theo mẫu test của `governance_event`).
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): work outcomes and their lifecycle`.

### Task A3: Tiêu chí, kiểm chứng và CLI

**Files:**
- Create: `server/internal/service/durable_verification_fork.go`, `server/internal/handler/durable_outcome_fork.go`, `server/cmd/multica/cmd_outcome_fork.go`
- Modify: `server/cmd/server/router_fork.go` (route spec 2.7), `RunClass` (thêm `verification`, `shadow`) qua `heldRunClasses()` trong file fork và một dòng trong `BudgetGate.Admit`; migration thêm hai class vào `budget.classes` có `routine`
- Test: `durable_verification_fork_test.go`, `server/internal/handler/durable_outcome_fork_test.go`, `server/cmd/multica/cmd_outcome_fork_test.go`

**Interfaces:**
- Consumes: `OutcomeService` (A2).
- Produces: `func (s *VerificationService) Schedule(ctx, o db.WorkOutcome) error` (tạo task `verification` cho `agent_judgment` và `command`, kiểm ngay `artifact`); `func (s *VerificationService) PickVerifier(ctx, ws, doerAgentID pgtype.UUID) (pgtype.UUID, bool)`; `func (s *VerificationService) Record(ctx, actor, outcomeID, input VerificationInput) (db.OutcomeVerification, error)`; job `metric` hằng giờ; handler cho mọi route spec 2.7; lệnh CLI spec 2.7.

- [ ] Test đỏ `TestVerifierNeverDoer` và `TestNoEligibleVerifierYieldsUnable` (workspace một agent).
- [ ] Test đỏ `TestImplicitCriterionWhenNone`: kết quả được chọn kiểm mà không có tiêu chí nhận một tiêu chí `agent_judgment` ngầm.
- [ ] Test đỏ `TestArtifactCriterionPassesOnPublish`, `TestMetricCriterionFailsAfterWindow`.
- [ ] Test đỏ `TestVerificationRunClassHeldByBudget`: budget `hard` giữ task `verification`, không giữ `human`.
- [ ] Test handler: quyền từng route (agent chỉ ghi cho task của mình → 403 khác task); `outcome_settled` khi sửa tiêu chí sau chốt; `override` ghi governance event `outcome_overridden`; phản hồi sai dạng.
- [ ] Test CLI: `multica outcome verify` gửi đúng thân; exit code khác 0 khi 4xx.
- [ ] Trường catalog tuỳ chọn `outcomeCriteria` theo vai của squad (spec 2.5): kiểm ở `scripts/catalog/lib/validate.mjs` (mảng `{role, text, check_kind}`, `role` là vai của squad), test ở `scripts/catalog/catalog.test.mjs`, áp khi squad nhận issue (`TestSquadTemplateCriteriaApplied`); `pnpm catalog:check` xanh.
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): acceptance criteria, verification tasks and outcome API`.

### Task A4: Briefing của người làm và người kiểm

**Files:**
- Modify (hook một dòng mỗi chỗ): nơi dựng briefing task trong daemon claim (`server/internal/handler/daemon*.go` hoặc service tương ứng tìm ở P0)
- Create: `server/internal/service/durable_briefing_fork.go`
- Test: `durable_briefing_fork_test.go`

**Interfaces:**
- Produces: `func DurableBriefingBlocks(ctx, task db.AgentTaskQueue) []BriefingBlock` trả khối "Tiêu chí nghiệm thu" cho task thường và khối kiểm chứng cho task `verification`.

- [ ] Test đỏ `TestDoerBriefingListsCriteria`; `TestVerifierBriefingHidesOtherVerdicts` (không có kết luận của người kiểm khác); `TestBriefingEmptyWhenFlagOff`.
- [ ] Test đỏ `TestSettledFailPostsSystemComment` (spec 2.8: kết quả `fail` hoặc `partial` đã chốt để lại một comment hệ thống nhắc người làm; không mở lại issue).
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): outcome briefing blocks`.

### Task A5: Scorecard và metric nội tại thật

**Files:**
- Modify: `server/internal/service/scorecard_metrics.go` qua một dòng gọi `durableScorecardMetrics(...)` trong `scorecard_metrics_fork.go` (mới); job metric nội tại
- Test: `scorecard_metrics_fork_test.go`

**Interfaces:**
- Produces: các khoá `verified_pass_rate`, `verified_coverage`, `rework_rate`, `cycle_time_days`, `history_partial` trong `scorecard.metrics`; metric nội tại `delivery.cycle_time_days`, `delivery.rework_rate`.

- [ ] Test đỏ `TestScorecardUsesDoneTransition` (thay proxy `updated_at`); `TestReworkRateFromTransitions`; `TestScorecardUnchangedWhenFlagOff` (so khớp đúng các khoá Phase 4).
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): real cycle time and rework in scorecards`.

### Task A6: Track record và chế độ kiểm chứng

**Files:**
- Create: migration `track_record` + chỉ mục; `server/pkg/db/queries/durable_track_record.sql`; `server/internal/service/durable_track_record_fork.go`, `durable_stats_fork.go`; `server/internal/scheduler/jobs_durable_track_record_fork.go`; handler route `/api/track-records*`
- Test: `durable_stats_fork_test.go`, `durable_track_record_fork_test.go`

**Interfaces:**
- Consumes: `OutcomeSettled`, `OutcomeHighFail` (A2).
- Produces: `func WilsonLower(pass, partial, fail int) float64` (z = 1.96, partial = 0.5); `func NextVerificationMode(cur string, r30, r90 TrackRow, lastChange time.Time, now time.Time) string`; `func Sampled(outcomeID uuid.UUID, mode string) bool` (băm sha256, ngưỡng 100/30/10%); `func (s *TrackRecordService) Recompute(ctx, ws, agentID pgtype.UUID, workClass string) error`; `func (s *TrackRecordService) ModeFor(ctx, ws, agentID, workClass) string`.

- [ ] Test đơn vị đỏ `TestWilsonLowerKnownValues`: `(10,0,0) → 0.7225`, `(8,0,2) → 0.4902`, `(0,0,0) → 0`, `(45,0,5) → 0.7864` (4 chữ số).
- [ ] Test đỏ `TestModeTransitions`: bảng spec 3.3 (lên một bậc, không quá một lần mỗi 7 ngày, `high` về `full`, ghim giữ nguyên).
- [ ] Test đỏ `TestSampledDeterministic`: cùng id cùng kết quả; trên 10.000 id ngẫu nhiên tỷ lệ chọn của `sampled` trong 30% ± 2%.
- [ ] Test đỏ `TestHighFailRecomputesImmediately` (không đợi job); `TestOutcomeKeepsModelAtCreation`.
- [ ] Nối `ModeFor` và `Sampled` vào `OnEnteredDone` (A2): thay chế độ mặc định `full`.
- [ ] Route pin: agent 403; governance event `verification_mode_changed`.
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): track records and verification modes`.

### Task A7: Autonomy Envelope

**Files:**
- Create: migration `squad_autonomy_envelope` + chỉ mục; `server/pkg/db/queries/durable_envelope.sql`; `server/internal/service/durable_envelope_fork.go`; handler route envelope
- Modify: `server/internal/service/autonomy.go` (một dòng: đổi tay ra ngoài envelope thì nới envelope), danh sách `governanceActorTypes` (một dòng sang `durableActorTypes()`)
- Test: `durable_envelope_fork_test.go`

**Interfaces:**
- Consumes: `WilsonLower`, track record squad (A6), tỷ lệ chấp nhận proposal (`squad_attention`).
- Produces: `func NextSquadLevel(cur int, env Envelope, s SquadStats, lastAuto time.Time, now time.Time) int`; `func (s *EnvelopeService) OnHighFail(ctx, o db.WorkOutcome) error`; `func (s *EnvelopeService) Nightly(ctx, ws) error`.

- [ ] Test đỏ `TestNextSquadLevelTable`: các hàng spec 3.4, giới hạn 14 ngày, không vượt trần, không dưới sàn, `n < 20` không đổi.
- [ ] Test đỏ `TestNoEnvelopeNoAutoChange`; `TestHighFailDropsSquadNow`; `TestEnvelopeAgentForbidden` (403); `TestAutoChangeLogsOneEvent` và có trong digest.
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): evidence-driven autonomy within an envelope`.

### Task A8: Core và giao diện giai đoạn A

**Files:**
- Create: `packages/core/durable/outcomes/{schemas,queries,mutations}.ts`, `packages/core/durable/track-records/{schemas,queries,mutations}.ts`; `packages/views/durable/outcomes/issue-acceptance-panel.tsx`, `governance-outcomes-tab.tsx`, `agent-track-record-tab.tsx`, `envelope-dialog.tsx`; `packages/views/locales/<9 locale>/durable.ts`
- Modify: `packages/views/governance/governance-page.tsx` (một dòng thêm tab), `governance-autonomy-tab.tsx` (một dòng cột envelope), issue detail sidebar (một dòng slot), trang agent (một dòng tab); router desktop và route web cho tab mới
- Test: `*.test.ts(x)` cạnh từng file

**Interfaces:**
- Consumes: route A3, A6, A7.
- Produces: hook `useIssueOutcome(wsId, issueId)`, `useIssueCriteria`, `useAddCriterion`, `useOutcomes(wsId, filters)`, `useTrackRecords`, `usePinMode`, `useSquadEnvelope`, `useSetEnvelope`; khoá query có `wsId`.

- [ ] Test core (node) đỏ: schema chấp nhận phản hồi đúng, `verdict` lạ → `unverified`, phản hồi thiếu trường dùng mặc định (`parseWithFallback`).
- [ ] Test view đỏ: khối Nghiệm thu hiện tiêu chí và chip `pending` có thời gian còn lại; ẩn khi cờ tắt; tab Outcomes lọc; dialog envelope chặn sàn > trần; câu "chỉ kiểm soát những gì đi qua nền tảng" có mặt.
- [ ] Cài đặt; `pnpm typecheck`, `pnpm lint`, `pnpm --filter @multica/core exec vitest run durable`, `pnpm --filter @multica/views exec vitest run durable governance locales`; xanh.
- [ ] GIF và `scripts/motion/capture.mjs` cho khối Nghiệm thu (mở rộng/thu gọn).
- [ ] Commit `feat(durable): outcome and track record UI`.

### Task A9: E2E giai đoạn A

**Files:** Create `e2e/durable/outcomes.spec.ts`

- [ ] Kịch bản spec 17.2 mục 1 bằng `TestApiClient`, agent giả đóng task qua API (không gọi agent thật), job gọi qua route test (`MULTICA_TEST_ROUTES=1`) như mẫu retro của Phase 4.
- [ ] `pnpm exec playwright test e2e/durable/outcomes.spec.ts`; xanh. Commit `test(durable): outcome week e2e`.

---

## Giai đoạn B. Hệ quả (F3, F4), cờ `durable_effects`

### Task B1: Phân loại và chính sách

**Files:**
- Create: `server/internal/service/effect_policy_fork.go`, `server/internal/connectedapps/effect_fork.go`; migration `effect_policy_override` + chỉ mục
- Modify: `server/internal/connectedapps/policy.go` (`Stricter` nhận `rehearse` qua một dòng gọi `strictnessFork`), `catalog.go` (trường `Reversibility`, `Undo` tuỳ chọn trên tool curated)
- Test: `effect_policy_fork_test.go`, `server/internal/connectedapps/effect_fork_test.go`

**Interfaces:**
- Produces: hằng `EffectRead`, `EffectWriteInternal`, `EffectWrite`, `EffectSend`, `EffectShare`, `EffectDelete`, `EffectSpend`; `ReversibilityReversible|Compensable|Irreversible`; `func MapAppClass(class string) string`; `func DefaultReversibility(effect string) string`; `func DecideEffect(level int, effect, rev string, overrides []Override, runClass string, companyState string, sendCountToday int) string`; `func ValidateOverride(level int, effect, rev, decision string) error`.

- [ ] Test đỏ `TestDecideEffectFullTable`: 7 × 3 × 4 ô của spec 4.3 sinh từ một bảng mong đợi viết tay; `shadow` chỉ `read`; `verification` chỉ `read` và `write_internal`; `paused` chặn; trần `send` ở L3.
- [ ] Test đỏ `TestOverrideRules`: lỏng hơn ở L2 → lỗi; `spend`/`share` thành `allow` → lỗi ở mọi mức; chặt hơn luôn được.
- [ ] Test đỏ `TestStricterWithRehearse`.
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): effect classes, reversibility and policy`.

### Task B2: Nhật ký hệ quả và các nguồn

**Files:**
- Create: migration `effect_event` + chỉ mục; `server/pkg/db/queries/durable_effect.sql`; `server/internal/service/durable_effect_log_fork.go`
- Modify (một dòng mỗi nguồn, theo P0 U3): gateway connected apps (local và remote), Action API plugin, gửi kênh chat, `webhook_delivery_worker`, publish channel, các lệnh CLI ghi qua middleware handler
- Test: `durable_effect_log_fork_test.go`

**Interfaces:**
- Consumes: `DecideEffect` (B1), mức tự chủ squad (cache 60 giây).
- Produces: `func (s *EffectService) Gate(ctx, in EffectInput) (EffectDecision, error)` trả quyết định và đã ghi `effect_event`; `type EffectInput struct{ WS, TaskID, AgentID, MemberID pgtype.UUID; Source, Target, Class, Reversibility, Summary string; Undo *UndoSpec; AppToolCallID pgtype.UUID }`.

- [ ] Test đỏ `TestEachSourceLogsOneEvent`: bảng sáu nguồn, mỗi nguồn một hành động, kể cả bị chặn.
- [ ] Test đỏ `TestShadowTaskGatewayWriteBlocked`.
- [ ] Test đỏ `TestEffectEventNoDelete` (append-only, trừ hai chuyển trạng thái).
- [ ] Đo: test benchmark `BenchmarkGate` không thêm truy vấn đọc (đếm truy vấn bằng hook pgx của test).
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): effect log across platform sources`.

### Task B3: Khai báo và hoàn tác

**Files:**
- Create: `server/cmd/multica/cmd_effect_fork.go`, `server/internal/handler/durable_effect_fork.go`; khối hướng dẫn briefing trong `durable_briefing_fork.go`
- Test: `cmd_effect_fork_test.go`, `durable_effect_fork_test.go`

**Interfaces:**
- Produces: `POST /api/effects/declare`, `GET /api/effects`, `POST /api/effects/{id}/undo`; CLI `multica effect declare|list|undo` với exit code 0/3/4.

- [ ] Test đỏ `TestDeclareExitCodes`; `TestUndoCallsInverseTool` (gateway giả, `GMAIL_CREATE_DRAFT` → `GMAIL_DELETE_DRAFT` với `draft_id`); `TestUndoAfterWindow409`; `TestUndoInternalRevisionKinds` (ba loại v1, loại khác 422 "không hoàn tác được").
- [ ] Test briefing: khối khai báo có mặt khi cờ bật.
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): declared effects and undo`.

### Task B4: Ý định hành động

**Files:**
- Create: migration `effect_intent` + chỉ mục; `server/internal/service/durable_intent_fork.go`, `durable_preview_fork.go`; handler route ý định; job hết hạn
- Modify: gateway (một dòng: quyết định `rehearse` → `IntentService.Create`)
- Test: `durable_intent_fork_test.go`, `durable_preview_fork_test.go`

**Interfaces:**
- Consumes: `EffectService.Gate` (B2).
- Produces: `func (s *IntentService) Create(ctx, in EffectInput, call CanonicalCall, rationale string) (db.EffectIntent, error)`; `func (s *IntentService) Approve(ctx, actor, id pgtype.UUID, editedCall *CanonicalCall, note string) error` (khoá `FOR UPDATE`, so băm, thực thi qua gateway, đánh thức agent); `Reject`, `Expire`; `func BuildPreview(toolkit, tool string, args map[string]any) Preview`; `func CallSHA256(c CanonicalCall) string` (JSON chuẩn hoá).

- [ ] Test đỏ `TestRehearseDoesNotCallGateway`; `TestApproveExecutesExactCall`; `TestApproveIntentConcurrentlyExecutesOnce` (hai goroutine, một lời gọi gateway, một 409 `intent_not_pending`); `TestEditedApproveRehashesAndComments`; `TestExpireWakesAgentOnce`; `TestBatchApproveFive`; `TestIntentFlagOffStopsWrites`.
- [ ] Test preview: email, bài đăng, bản ghi CRM, tool lạ (bảng tham số rút gọn); preview không bao giờ lấy chữ từ `_rationale`.
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): rehearsed intents with bound execution`.

### Task B5: Flow có hệ quả ngoài

Chỉ làm nếu P0 U7 xác nhận được trường manifest; nếu không, ghi lại vào `p0-findings.md` và bỏ task (spec 5.4 thu hẹp).

**Files:** Modify `server/internal/issueworkflow/flowdoc_validate*.go` (một dòng), `scripts/catalog/lib/validate.mjs` (trường `effects`); Create `server/internal/service/durable_flow_effects_fork.go`; Test tương ứng và `scripts/catalog/catalog.test.mjs`.

- [ ] Test đỏ: bước `effects: external` tạo ý định `declared` trước khi chạy và chỉ chạy sau khi duyệt; cổng Launch không đổi.
- [ ] Cài đặt; `pnpm catalog:check`; chạy; xanh. Commit `feat(durable): rehearse external flow steps`.

### Task B6: Giao diện giai đoạn B

**Files:**
- Create: `packages/core/durable/effects/*`, `packages/core/durable/intents/*`; `packages/views/durable/effects/governance-effects-tab.tsx`, `effect-policy-grid.tsx`, `intent-card.tsx`, `intent-batch-card.tsx`; bản dịch
- Modify: Inbox (một dòng loại thẻ), Governance page và Controls tab (một dòng mỗi chỗ), run detail (một dòng)
- Test: cạnh từng file; `packages/views/durable/effects/intent-card.test.tsx` dùng mutation thật và API giả, unmount trước khi request xong (mẫu `use-inbox-snooze.test.tsx`)

- [ ] Test đỏ: thẻ ý định hiện preview và dự đoán (nếu có); Sửa rồi duyệt gửi `editedCall`; toast vẫn hiện sau unmount; lưới chính sách khoá ô không được lỏng và hiện lý do; tab Effects tách "đã khai báo".
- [ ] Wire web và desktop; `pnpm typecheck`, `pnpm lint`, vitest `durable governance inbox locales`; xanh. GIF cho thẻ ý định.
- [ ] Commit `feat(durable): effects and intents UI`.

### Task B7: E2E giai đoạn B

**Files:** Create `e2e/durable/effects.spec.ts`

- [ ] Phần F3, F4 của kịch bản spec 17.2 mục 2 với gateway giả của test (không gọi dịch vụ ngoài thật).
- [ ] Chạy; xanh. Commit `test(durable): rehearsed email e2e`.

---

## Giai đoạn C. Con người (F5, F6), cờ `durable_principal`

### Task C1: Attention ledger

**Files:**
- Create: migration `attention_ledger` + chỉ mục; cài đặt workspace `attention_minutes_per_week`, `attention_split`; `server/internal/service/durable_attention_fork.go`; metric nội tại `attention.minutes_per_done`
- Modify (một dòng mỗi chỗ tạo câu hỏi): proposal create, `app_action_request` create, ý định (B4), kiểm chứng `manual` (A3), inquiry của Secretary; digest của brief
- Test: `durable_attention_fork_test.go`

**Interfaces:**
- Produces: `func EstimateMinutes(kind string, words int, irreversible bool, predictedConf float64, batchSize int) float64`; `func (s *AttentionService) Admit(ctx, ask AskInput) (state string, err error)` (`delivered` | `queued`); `func (s *AttentionService) Answered(ctx, subjectType string, subjectID pgtype.UUID, actualSeconds *int) error`; khối digest `QueuedAsks`.

- [ ] Test đỏ `TestEstimateMinutesTable` (bảng spec 7.2, hệ số 0.5, lô).
- [ ] Test đỏ `TestSquadShareExhaustedQueues` (không thông báo Inbox, không chat, có trong digest); `TestUrgentNotQueued`; `TestDelegatedCostsNothing`; `TestAttentionFlagOffNoLedger`.
- [ ] Test đỏ `TestAgentAskRequiresReason` (thiếu `reason` hoặc `why_not_decide` → 422 `attention_reason_required`; `missing_info` thiếu `searched` → 422; người không bắt buộc).
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): attention budget and ask ledger`.

### Task C2: Principal profile, đồng ý và phỏng vấn

**Files:**
- Create: migration `principal_profile`, `principal_trait` + chỉ mục; `server/internal/service/durable_principal_fork.go`; handler route profile (spec 6.6); CLI `multica principal trait add|retire`, `multica principal show`
- Modify: khởi động phỏng vấn qua chat của Secretary (một dòng vào luồng chat Secretary có sẵn)
- Test: `durable_principal_fork_test.go`

**Interfaces:**
- Produces: `func (s *PrincipalService) Enable(ctx, member, consentText string, share bool) error`; `AddTrait(ctx, actor, in TraitInput) error` (kiểm bằng chứng thuộc đúng người, tối đa 200 active); `Delete(ctx, member) error`; `CanRead(actor, member) bool`.

- [ ] Test đỏ `TestTraitRequiresOwnEvidence` (422); `TestEditedTraitNotRetiredByJob`; `TestProfileReadAccess` (người đó, Secretary trong task dự đoán/cập nhật, owner khi share; agent khác 403); `TestDeleteProfile` (xoá cứng ba bảng, làm sạch `why` và `trait_ids`); `TestReviewJobsNeverReadProfile` (truy vấn của scorecard, review, org change không chạm bảng principal; kiểm bằng danh sách truy vấn sqlc mà các job đó gọi).
- [ ] Test phỏng vấn: 8 câu cố định, mỗi trả lời thành một trait `interview` với bằng chứng là tin nhắn.
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): principal profile with consent`.

### Task C3: Học và dự đoán

**Files:**
- Create: migration `decision_prediction` + chỉ mục; `server/internal/service/durable_prediction_fork.go`; job 6 giờ cập nhật profile; CLI `multica principal predict`
- Modify: điểm tạo câu hỏi (C1) gọi `PredictionService.Request` (cùng dòng hook của C1, mở rộng)
- Test: `durable_prediction_fork_test.go`

**Interfaces:**
- Produces: `func (s *PredictionService) Request(ctx, ask AskInput) error` (gộp vào task Secretary); `Record(ctx, task, in PredictionInput) error`; `Resolve(ctx, subjectType, subjectID, actual string) error`; `Accuracy(ctx, member, kind string) (matched, n int, lower float64)`.

- [ ] Test đỏ `TestNoProfileNoPrediction`; `TestPredictionResolvedOnDecision`; `TestAccuracyLast50`; `TestNoPredictionAboutSelf`; `TestProfileUpdateJobBatches` (một task cho nhiều quyết định trong 6 giờ).
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): decision predictions and profile learning`.

### Task C4: Uỷ quyền bằng dự đoán

**Files:**
- Create: migration `principal_delegation` + chỉ mục; `server/internal/service/durable_delegation_fork.go`
- Modify: các đường áp quyết định (proposal decide, ý định approve, kiểm chứng manual, inquiry) nhận actor `principal` qua hàm fork
- Test: `durable_delegation_fork_test.go`

**Interfaces:**
- Produces: `func ForbiddenDelegationKind(kind string, ask AskInput) bool`; `func (s *DelegationService) Enable(ctx, member, kind string, minConf float64) error`; `func (s *DelegationService) MaybeApply(ctx, pred db.DecisionPrediction, ask AskInput) (bool, error)`; `func (s *DelegationService) OnReverted(ctx, predID pgtype.UUID) error`.

- [ ] Test đỏ `TestEnableRequires20And85`; `TestForbiddenKindsNeverDelegated` (bảng mọi loại cấm spec 6.5); `TestDelegatedDecisionLogged` (governance event, FYI, nút đảo 72 giờ); `TestAutoDisableOnTwoReverts`; `TestAutoDisableBelow80`; `TestOnlyProfileOwnerEnables` (owner khác 403).
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): delegation by prediction`.

### Task C5: Giao diện giai đoạn C

**Files:**
- Create: `packages/core/durable/attention/*`, `packages/core/durable/principal/*`; `packages/views/durable/principal/principal-profile-page.tsx`, `consent-dialog.tsx`, `trait-list.tsx`, `delegation-panel.tsx`; `packages/views/durable/attention/attention-meter.tsx`, `queued-asks.tsx`; bản dịch
- Modify: trang Tài khoản (một dòng mục profile), Office header/pulse (một dòng đồng hồ phút), Controls (một dòng ô ngân sách), thẻ proposal và thẻ ý định (một dòng hiện dự đoán)
- Test: cạnh từng file

- [ ] Test đỏ: hộp đồng ý hiện đủ bốn ý của spec 6.6 và không bật khi chưa tích; trait sửa được, xoá được; bảng uỷ quyền khoá loại cấm và loại chưa đủ điều kiện, hiện độ chính xác; đồng hồ phút; dự đoán hiện cạnh câu hỏi.
- [ ] Wire web và desktop; typecheck, lint, vitest; xanh. GIF cho bảng uỷ quyền.
- [ ] Commit `feat(durable): principal profile and attention UI`.

### Task C6: E2E giai đoạn C

**Files:** Modify `e2e/durable/effects.spec.ts` (thêm phần F5, F6 của kịch bản 17.2 mục 2)

- [ ] Dự đoán do task giả ghi qua API; ledger 0.5 phút sau duyệt. Chạy; xanh. Commit `test(durable): prediction and attention e2e`.

---

## Giai đoạn D. Chống lỗi thời (F7, F8, F9), cờ `durable_seats`

### Task D1: Jury

**Files:**
- Create: migration `jury_panel` + chỉ mục; cài đặt `jury_pool`, `jury_work_classes`; `server/internal/service/durable_jury_fork.go`
- Modify: `VerificationService.Schedule` (A3) chọn Jury khi đủ điều kiện (trong file fork, không chạm upstream)
- Test: `durable_jury_fork_test.go`

**Interfaces:**
- Produces: `func PickJurors(pool []JurorCandidate, doer JurorCandidate, size int) ([]JurorCandidate, string)` (trả `diversity`); `func (s *JuryService) Open(ctx, o db.WorkOutcome, c db.AcceptanceCriterion) error`; `OnVote`, `Timeout`; metric `jury.split_rate`.

- [ ] Test đơn vị đỏ `TestPickJurorsDiversity` (ba provider → không trùng người làm; hai provider → `model_only` khi cần; một agent → rỗng).
- [ ] Test đỏ `TestJuryUnanimous`; `TestJurySplitCreatesOneAsk` (ledger `reason = verification`); `TestJurorUnableReplacedOnce`; `TestJuryTimeout24h`; `TestJuryWithOneAgentEscalates`; `TestJurorTrackRecordOnOverrule` (`work_class = verify:<gốc>`).
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): cross-model jury`.

### Task D2: Hồ sơ ghế

**Files:**
- Create: migration `seat_dossier_entry` + chỉ mục; `server/internal/service/durable_dossier_fork.go`; CLI `multica seat lesson add|list|retire`; job tháng dọn; event `seat_model_changed`
- Modify: hook cập nhật agent (một dòng ghi event khi đổi runtime hoặc model); `DurableBriefingBlocks` (A4) thêm khối hồ sơ ghế
- Test: `durable_dossier_fork_test.go`

**Interfaces:**
- Produces: `func SelectDossier(entries []db.SeatDossierEntry, workClass string, limit int) []db.SeatDossierEntry` (giới hạn 4.000 ký tự, ưu tiên spec 9.2); `func (s *DossierService) OnOutcomeSettled(ctx, o db.WorkOutcome) error` (task ghi bài học cho `fail`/`partial`).

- [ ] Hook một dòng ở `server/internal/service/retro.go`: retrospective đã xong cũng giao task ghi bài học (`TestLessonTaskAfterRetro`).
- [ ] Test đỏ `TestSelectDossierLimitAndOrder`; `TestLessonTaskOnFailOnly`; `TestMax100Active`; `TestMemberEntryOnlyMemberRetires`; `TestModelChangeLogsEvent`.
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): seat dossier`.

### Task D3: Shadow run

**Files:**
- Create: migration `shadow_config`, `shadow_pair` + chỉ mục, và cột `agent.shadow_of_agent_id` (D27); `server/internal/service/durable_shadow_fork.go`; handler route shadow; job so sánh
- Modify: claim trong `server/internal/handler/daemon.go` (`ClaimTasksByRuntime`) thêm điều kiện capability `DaemonCapabilityShadowV1` qua hàm trong file fork (một dòng hook; D30); payload claim (`shadow`, `omit_skills`) chỉ gửi cho daemon có capability; daemon chạy task bóng trong thư mục tạm; middleware lệnh `multica` (một dòng 403 `shadow_task`). Test thêm: `TestShadowAgentHidden`, `TestShadowInPlaceRefused`, `TestShadowClaimRequiresCapability`.
- Test: `durable_shadow_fork_test.go`, test daemon tương ứng

**Interfaces:**
- Consumes: `EffectService.Gate` (B2) với run class `shadow`; `PickJurors` (D1).
- Produces: `func ShadowSelected(issueID, configID uuid.UUID, rate float64) bool`; `func (s *ShadowService) OnPrimaryClaimed(ctx, task db.AgentTaskQueue) error`; `Compare(ctx, pair db.ShadowPair) error` (A/B ngẫu nhiên); `Report(ctx, configID) ShadowReport`.

- [ ] Test đỏ `TestShadowSelectionDeterministic`; `TestShadowTaskCliMutation403`; `TestShadowGatewayWriteBlocked`; `TestShadowNoWorktreeClaim`; `TestShadowNoWriteMcpOrAppTokens`; `TestComparisonBlindAB` (briefing của juror không có tên model, nhãn đổi ngẫu nhiên giữa các cặp); `TestShadowHeldByBudget`; `TestShadowFlagOffStopsWrites`; `TestShadowCostEstimateWarns`.
- [ ] Test daemon: client không khai báo `DaemonCapabilityShadowV1` không bao giờ nhận task bóng (`TestShadowClaimRequiresCapability`, D30).
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): shadow runs with isolation`.

### Task D4: Vào ghế (`runtime_change`)

**Files:**
- Modify: bảng kiểu `org_change` và `org_change_probation_types` (một dòng sang danh sách fork); áp thay đổi org change (một dòng)
- Create: `server/internal/service/durable_runtime_change_fork.go`
- Test: `durable_runtime_change_fork_test.go`

**Interfaces:**
- Produces: `func PromotionReady(r ShadowReport) bool` (cận dưới "không thua" ≥ 0.5 và thắng ≥ thua); `func (s *RuntimeChangeService) OnShadowCompleted(ctx, cfg db.ShadowConfig, r ShadowReport) error` (proposal hoặc áp khi `auto_promote`); revert khi probation `worse`.

- [ ] Test đỏ `TestPromotionReadyTable`; `TestCompletedCreatesProposal`; `TestAutoPromoteAppliesAndProbation`; `TestProbationWorseReverts`; `TestAutoPromoteOwnerOnly`.
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): runtime change after shadow trial`.

### Task D5: Scaffolding Sunset

**Files:**
- Create: migration `necessity_test`; `server/internal/service/durable_necessity_fork.go`, `durable_newcombe_fork.go`; job tuần chọn bài kiểm tra; job tháng thống kê bước flow
- Test: `durable_necessity_fork_test.go`, `durable_newcombe_fork_test.go`

**Interfaces:**
- Consumes: `ShadowService` (D3) với `purpose = necessity_test` và `omit_skills`/cắt khối chỉ dẫn.
- Produces: `func NewcombeDiffCI(lossA, winA, n int) (lo, hi float64)`; `func NecessityVerdict(pairs []Comparison, margin float64, minPairs int) string`; `func (s *NecessityService) WeeklySchedule(ctx, ws) error` (tối đa 2 mỗi squad); `FlowStageCandidates(ctx, ws) []StageStat`.

- [ ] Test đơn vị đỏ `TestNewcombeKnownValues` (so với giá trị tính sẵn trong bảng test); `TestNecessityVerdictTable` (80 hoà ở biên 0.05 → `not_needed`; 40 hoà ở biên 0.05 → `inconclusive`; bóng thua 30% → `needed`; giữa → `inconclusive` sau `2 × minPairs`).
- [ ] Test đỏ `TestShadowOmitsSkillUnderTest`; `TestWeeklyMaxTwoPerSquad`; `TestNotNeededFilesOneProposal`; `TestNoSkillRemovalWithoutApproval`; `TestFlowStageCandidateThreshold` (≥ 95% trên ≥ 30 lượt).
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): necessity tests for scaffolding`.

### Task D6: Giao diện giai đoạn D

**Files:**
- Create: `packages/core/durable/{jury,dossier,shadow,necessity}/*`; `packages/views/durable/seats/seat-dossier-tab.tsx`, `shadow-trial-dialog.tsx`, `shadow-report.tsx`, `necessity-tests.tsx`, `jury-split-card.tsx`; bản dịch
- Modify: trang agent (một dòng mỗi tab), Inbox (một dòng thẻ split), Controls (một dòng cài đặt jury)
- Test: cạnh từng file

- [ ] Test đỏ: dialog shadow hiện ước tính chi phí và câu cảnh báo khoá truy cập (spec 9.4); báo cáo hiện thắng/hoà/thua và cận dưới; thẻ split hiện các lý do song song; hồ sơ ghế sửa được.
- [ ] Wire web và desktop; typecheck, lint, vitest; xanh. GIF cho báo cáo shadow.
- [ ] Commit `feat(durable): seats, jury and shadow UI`.

### Task D7: E2E giai đoạn D

**Files:** Create `e2e/durable/shadow.spec.ts`

- [ ] Kịch bản spec 17.2 mục 3 với runtime giả; khẳng định không có `effect_event` nào khác `read` từ task bóng.
- [ ] Chạy; xanh. Commit `test(durable): shadow trial e2e`.

---

## Giai đoạn E. Mở ra ngoài (F10, F11, F12), cờ `durable_open`

### Task E1: Reach Map

**Files:**
- Create: migration `reach_snapshot` + chỉ mục; `server/internal/service/durable_reach_fork.go`; `server/internal/scheduler/jobs_durable_reach_fork.go`; handler route reach và "lấp khoảng trống"
- Test: `durable_reach_fork_test.go`

**Interfaces:**
- Consumes: `effect_event` (B2), `attention_ledger` (C1), catalog connected apps.
- Produces: `func SurfaceOf(e db.EffectEvent) string`; `func ReachState(s SurfaceStats) string`; `func (s *ReachService) Snapshot(ctx, ws, date) error`; `CloseGap(ctx, actor, surface string, squadID pgtype.UUID) (db.Issue, error)`.

- [ ] Test đỏ `TestReachStateTable` (năm trạng thái); `TestSnapshotIdempotentPerDay`; `TestCloseGapCreatesIssueWithMetricCriterion`; `TestCloseGapTwiceOpensExisting`; `TestReachRetention400Days`.
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): reach map`.

### Task E2: Open Company Format

**Files:**
- Create: `docs/specs/durable-platform/ocf/v1.schema.json`, `ocf/README.md`, `ocf/worker-protocol.md`; `server/internal/service/durable_ocf_fork.go`; tệp schema nhúng cho Go (`//go:embed`), bản sao được kiểm khớp bằng test
- Modify: `company_export` và `company import` (một dòng mỗi chỗ: `--history`, đọc `manifest.json`); CLI `multica company export --history`
- Test: `durable_ocf_fork_test.go`; test CI xác thực export mẫu bằng schema

**Interfaces:**
- Produces: `func ExportOCF(ctx, ws, opts ExportOptions) (io.Reader, error)`; `func ImportOCF(ctx, ws, r io.ReaderAt) (ImportReport, error)`; `func ValidateOCF(manifest, parts) []ValidationError`.

- [ ] Test đỏ `TestExportHistoryValidatesAgainstSchema`; `TestImportMarksHistoryImported`; `TestImportedTrackRecordsGiveNoMode` (mọi chế độ `full`); `TestImportLegacyExport`; `TestUnknownFieldRejectedWithPath`; `TestPrincipalsExportedOnlyWithConsent`; `TestEmbeddedSchemaMatchesDocs`.
- [ ] Adapter CLI tham chiếu (D29): `server/cmd/multica-ocf-adapter/` (mới) nói giao thức `worker-protocol.md`, chạy được như runtime tuỳ chỉnh hiện có; test end-to-end với một worker giả.
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): open company format v1`.

### Task E3: Attestation

**Files:**
- Create: `server/internal/service/durable_attestation_fork.go`, `durable_jcs_fork.go` (RFC 8785); route `/.well-known/multica-attestation-keys.json` (công khai) và `POST /api/attestations/verify` (công khai, giới hạn tốc độ) trong `router_fork.go`; route phát hành (owner, admin); CLI `multica attestation issue|verify`
- Test: `durable_attestation_fork_test.go`, `durable_jcs_fork_test.go`

**Interfaces:**
- Produces: `func Canonicalize(v any) ([]byte, error)`; `func (s *AttestationService) Issue(ctx, actor, agentID pgtype.UUID, period Period, showWorkspace bool) (Attestation, error)`; `func Verify(a Attestation, keys KeySet) error`; quản lý khoá với xoay vòng.

- [ ] Test đơn vị đỏ `TestCanonicalizeRFC8785Vectors` (bộ vector của RFC); `TestSignVerifyRoundTrip`; `TestTamperFails` (sửa một chữ số); `TestRotatedKeyStillVerifies`; `TestForeignKeyRejected`.
- [ ] Test đỏ `TestIssueRequires30`; `TestWorkClassCoarsenedToArea`; `TestWorkspaceHiddenByDefault`; `TestVerifyRateLimited`.
- [ ] Test `TestIssueDisabledWithoutKey`: không có `MULTICA_ATTESTATION_KEY` thì API phát hành trả 503 `attestation_disabled`, còn xác minh và khoá công khai vẫn chạy (D28).
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): signed track record attestations`.

### Task E4: Seat offer trên Hub

**Files:**
- Modify: kiểu item Hub (một dòng sang danh sách fork), gói template (thư mục `attestations/`)
- Create: `server/internal/service/durable_seat_offer_fork.go`
- Test: `durable_seat_offer_fork_test.go`

**Interfaces:**
- Consumes: `Verify` (E3), `org_change hire` có sẵn.
- Produces: `func (s *SeatOfferService) Hire(ctx, actor, offerID, templateRef pgtype.UUID) (db.Proposal, error)`.

- [ ] Test đỏ `TestHireFromOfferFilesHireProposalWithAttestation`; `TestHiredSeatStartsFull`; `TestInvalidAttestationShownAsUnverified`; `TestOfferScopedToHubOrg`.
- [ ] Cài đặt; chạy; xanh. Commit `feat(durable): seat offers on the hub`.

### Task E5: Giao diện giai đoạn E

**Files:**
- Create: `packages/core/durable/{reach,ocf,attestations}/*`; `packages/views/durable/reach/governance-reach-tab.tsx`, `reach-surface-detail.tsx`; `packages/views/durable/open/export-history-dialog.tsx`, `attestation-badge.tsx`; bản dịch
- Modify: Governance page (một dòng tab Reach), Hub item (một dòng huy hiệu), dialog export (một dòng tuỳ chọn lịch sử)
- Test: cạnh từng file

- [ ] Test đỏ: lưới bề mặt tô theo trạng thái và kích thước theo phút; "Lấp khoảng trống" tạo issue; huy hiệu attestation hiện "đã xác minh" hoặc "chưa xác minh"; dialog export có tuỳ chọn profile chỉ khi người đó đồng ý.
- [ ] Wire web và desktop; typecheck, lint, vitest; xanh. GIF cho lưới Reach.
- [ ] Commit `feat(durable): reach map, export and attestation UI`.

---

## Giai đoạn F. Kiểm tra rộng và phát hành từng cờ

### Task F1: Kiểm tra rộng sau mỗi giai đoạn

- [ ] `make check`; `pnpm exec playwright test e2e/durable`; `pnpm sync:contract`; `pnpm sync:coverage`; `pnpm test:scripts` nếu có thay đổi ở `scripts/`.
- [ ] `/security-review` trên thay đổi của giai đoạn; sửa phát hiện thật trong commit riêng.
- [ ] Ghi kết quả (cái gì chạy, cái gì bỏ qua, đỏ sẵn) vào `docs/specs/durable-platform/rollout.md` (mới).

### Task F2: Bật thử trên máy chủ dev

Cần sự đồng ý của người dùng cho từng lần (đây là thay đổi môi trường thật). Chỉ workspace `tupk`; ghi mọi đối tượng được tạo vào `rollout.md`.

- [ ] Bật cờ của giai đoạn cho workspace thử; chạy 7 ngày; đo các thước đo của [strategy.md mục 8](strategy.md#8-thước-đo-thành-công); ghi vào `rollout.md`.
- [ ] Thước đo đạt: đề xuất PR bật mặc định, cùng PR viết tài liệu `apps/docs/content/docs/` ở mọi locale (theo AGENTS.md). Không đạt: ghi nguyên nhân và việc cần sửa.

## Thứ tự và cách chia việc

```
P0 ──► A1 ──► A2 ──► A3 ──► A4
              │       └───► A5
              └──► A6 ──► A7 ──► A8 ──► A9
P0 ──► B1 ──► B2 ──► B3
              └──► B4 ──► B5 ──► B6 ──► B7          (B chạy song song với A sau P0; B2 không chạm file của A)
A2 + B4 ──► C1 ──► C2 ──► C3 ──► C4 ──► C5 ──► C6
A3 + B2 ──► D1 ──► D2 ──► D3 ──► D4 ──► D5 ──► D6 ──► D7
B2 + C1 ──► E1 ;  A6 ──► E2 ──► E3 ──► E4 ──► E5
mỗi giai đoạn kết thúc bằng F1
```

- Mỗi giai đoạn có thể giao cho một phiên riêng (một phiên một giai đoạn, không để hai phiên ghi cùng file). A và B song song được sau P0; điểm chạm chung duy nhất là `durable_briefing_fork.go` (A4 tạo, B3 thêm khối): B3 đợi A4.
- Theo thói quen của repo: backend và core do phiên chính làm; giao diện (A8, B6, C5, D6, E5) có thể giao cho agent giao diện khi người dùng yêu cầu.
- Dừng sau giai đoạn nào thì phần đã làm vẫn dùng được độc lập (spec 14.1).
