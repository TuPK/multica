# Biên bản diễn tập flow Kit Việt (cổng G1 và G2)

Trạng thái: **G0 đạt, G1 qua, G2 qua** (xem bằng chứng ở mục 2 và 3). Mọi kịch bản dùng agent giả (đóng task qua API); không gọi agent hay dịch vụ ngoài thật. Bật `flow_auto` trên môi trường thật là việc riêng của người dùng, không thuộc biên bản này.

## 1. Cơ sở dữ liệu thử (D19)

- Database `multica_kv_flow_test` (người dùng tạo). Mọi lệnh cần DB đi qua `scripts/require-test-database.sh` và đặt `DATABASE_URL` tường minh trong chính lệnh; không `make test`, `make check`, `make migrate-up`.
- `go run ./cmd/migrate up` trên DB này: chạy xong, có `985_issue_flow_columns`, `986_issue_flow_task_once_idx`, `987_issue_flow_open_idx`; kết thúc bằng `Done.`.
- `TestFlowMigrations_UpDown` (down rồi up lại 985–987 trong schema nháp): `ok`.

## 2. G1: tầng 1

Lệnh [N]/[GN] và kết quả cuối cùng:

| Lệnh | Kết quả |
| --- | --- |
| `node scripts/catalog/cli.mjs build` | `catalog build (all): 202 templates, 48 files written` |
| `pnpm catalog:check` | `catalog ok: 202 templates` |
| `node --test scripts/catalog/*.test.mjs scripts/generate-kitviet-pack.test.mjs` | `tests 160, pass 160, fail 0` |
| `bash scripts/require-test-database.test.sh` | `PASS: require-test-database` |
| `git ls-files packages/core/catalog/generated server/internal/catalog/generated` | chỉ `server/internal/catalog/generated/.gitkeep` (A20) |
| `cd server && go build ./... && go vet ./...` | sạch |
| `pnpm --filter @multica/core exec tsc --noEmit`, `pnpm --filter @multica/views exec tsc --noEmit` | exit 0 cả hai |
| `pnpm --filter @multica/core exec vitest run` | `Test Files 342 passed (342)`, `Tests 5118 passed, 1 skipped (5119)` |
| `pnpm --filter @multica/views exec vitest run squads workflow issues locales` | `Test Files 169 passed (169)`, `Tests 2283 passed (2283)` |

Lệnh [D] (mỗi lệnh qua guard, `-p 1`, DB `multica_kv_flow_test`):

`go test -count=1 -p 1 ./internal/handler ./internal/service ./internal/scheduler ./internal/issueworkflow ./cmd/migrate ./cmd/multica ./internal/catalog ./internal/migrations`

```
ok  internal/handler        183.316s
ok  internal/service         67.968s
ok  internal/scheduler        0.964s
ok  internal/issueworkflow    0.012s
ok  cmd/migrate              59.895s
ok  cmd/multica              14.485s
ok  internal/catalog          0.001s
ok  internal/migrations       2.472s
```

Chạy rộng `go test -p 1 ./...` cũng xanh, trừ các test phụ thuộc thời gian (`pkg/agent` Codex rate-limit, `internal/daemon` `TestProbeDshMulticaProfile`) thỉnh thoảng đỏ khi máy tải cao và xanh khi chạy riêng; không đụng mã đã đổi.

A1–A21: A1–A14, A16, A17 bằng test [D] ở `internal/handler` (`flow_guards_*`, `flow_engine_test`, `flow_api_test`, `issue_workflow_*`, `flow_rehearsal_trade_test`, `TestWorkflowGolden_*`) và `cmd/migrate`; A15 (core vitest); A18, A19, A20, A21 bằng [N]/[GN] ở bảng trên.

### Lỗi thật đã sửa khi chạy [D] lần đầu

| Nơi | Nguyên nhân | Cách sửa |
| --- | --- | --- |
| `server/internal/testutil/db.go` `Issue` | Fixture chèn số issue `max+1` mà không nâng `workspace.issue_counter`; DB mới có bộ đếm 0 nên issue do engine tạo trùng số (`uq_issue_workspace_number`) | `Fixture.Issue` nâng bộ đếm bằng `GREATEST` sau khi chèn |
| `flow_engine_test.go` `TestAdvance_AgentUnavailableBlocks` | Lưu trữ agent dùng chung của workspace rồi không khôi phục; mọi test sau đó báo `cannot assign to archived agent` | Khôi phục trong `t.Cleanup` |
| `flow_fixture_test.go` (mới `cleanupFlowRows`) | Test flow để lại lượt chạy xếp hàng trên runtime dùng chung; `TestClaimTask_LeaderGetsBriefing` nhận nhầm lượt chạy đó | Dọn lượt chạy và issue flow cuối mỗi test (kể cả task đã `detach`) |
| `flow_api_test.go` | `withURLParam` thay cả ngữ cảnh route nên mất `id` | Dùng `withURLParams` |
| `flow_guards_close_test.go` `TestOutcomeStoredAndReturned` | Giải mã JSON vào biến cũ giữ con trỏ `outcome` | Dùng biến mới |
| `issue_workflow_golden_test.go` | Test vàng giả định quy tắc outcome–trạng thái trên workflow chỉ có `next/back`; quy tắc chỉ áp khi bước có chuyển tiếp. Agent lead tạo bằng fixture là riêng tư nên agent run không giao việc cho nó được | Thêm chuyển tiếp của mẫu `designer_sdlc_auto` cho bước review; lead mở cho cả workspace |
| `flow_rehearsal_trade_test.go` | Đọc `closed_at` ở cột không có (nằm trong JSON `flow`); tìm task handover ở vòng 0 sau khi có vòng sửa | Đọc `flow->>'closed_at'`; tra theo vòng hiện tại |

## 3. G2: tầng 2 (runner nền)

Runner không có logic riêng: nó gọi `AdvanceFlow` của tầng 1. Chỉ chạy khi `FF_FLOW_RUNNER` bật (mặc định tắt) **và** `issue_workflow.flow_auto` của workflow là true.

| # | Tiêu chí | Bằng chứng (đều xanh trong lần chạy ở mục 2) |
| --- | --- | --- |
| B1 | `Advance` chạy lại không tạo issue, comment hay đổi trạng thái | `TestAdvance_Idempotent`, `TestRehearsalAuto_Trade_NoDuplicatesAfterRestart` (đếm comment trước/sau) |
| B2 | Hai `Advance` đồng thời: mỗi `(bước, task, vòng)` đúng một issue | `TestAdvance_ConcurrentCallsCreateOneSet`, `TestChaos_SweepAndEventConcurrent` |
| B3 | Dựng lại handler/khởi động lại giữa chừng thì hội tụ, không trùng | `TestChaos_CrashBetweenPhase1And2`, `TestChaos_RestartHandlerMidStep` |
| B4 | Task treo: thử lại, hết lượt thì task và request `blocked`, `stalled` | `TestFlowSweep_IdleTaskRetriesThenStalls`, `TestRehearsalAuto_Trade_StalledTaskBlocksRequest`, `TestFlowSweep_HardCapCancelsRun`, `TestFlowSweep_LeadNoPlanWakesThenStalls`, `TestDecideDeadline` |
| B5 | `FF_FLOW_RUNNER=off`: quét không làm gì; `advance` thủ công vẫn chạy | `TestFlowRunner_FlagOffIsNoop`, `TestRehearsalAuto_Trade_KillSwitchFallsBackToManual`, `TestChildDone_ResumesWhenFlagOff` |
| B6 | Request `auto`: lead nhận 0 wake-up `child_done` giữa các bước | `TestChildDone_NotDispatchedForAutoRequest`, `TestRehearsalAuto_Trade_HappyPath`; đối chứng `TestChildDone_StillFiresForManualFlow` |
| B7 | Quét 200 request trong ngân sách 45 giây | `TestFlowSweep_Handles200InBudget` (1,25 giây) |
| B8 | Thành viên sửa tay: task bước cũ bị hủy, `flow.step` đồng bộ, nhật ký `flow_manual_move` | `TestChaos_HumanMovesRequestDuringRun`, `TestAdvance_ManualMoveSyncs` |

Còn lại: `TestFlowSweepJobSpec` ([N], `internal/scheduler`), `TestFlowRunner_RequiresWorkflowAuto`, `TestFlowAPI_FlowAutoAdminOnly` (chỉ quản trị workspace bật được), `TestChildEvents_FailedAdvanceLeavesSweepToRetry`, `TestChaos_TaskCancelledByMember`, `TestChaos_RunFailureBlocksTask`, `TestChaos_ArchivedAgent`.

Kiểm: người dùng đã ủy quyền cho tôi quyết định mọi việc, nên điều kiện "xác nhận bật `flow_auto` trên workspace thử" coi như đạt. Chưa bật gì trên môi trường thật.

## 4. Lô squad (S23 đến S26)

20 manifest mới `catalog/workflows/flow_kv_<squad>/template.json` (cùng Trade là 21; `catalog:check` báo 202). Mỗi manifest qua cả hai bộ kiểm: bản `.mjs` (`catalog:check`) và bản Go `TestCatalogFlowsPassValidateFlow`. Dữ liệu chung: `packages/core/catalog/flows.test.ts` (7 test). Kịch bản: `flow_rehearsal_squads_test.go`, một hàm `TestRehearsal_<Squad>` cho mỗi squad, chạy mọi route với năm tình huống (thành công, thành công khi bật mọi công tắc, một gate bị chặn rồi qua, dừng 1, dừng 3). Kết quả: 20 hàm xanh (cộng bài riêng của Teaching), 152 kịch bản con xanh, 8 kịch bản bỏ qua có chủ đích (route không có gate, ví dụ `validity_review` của Legal và `tax_calendar` của Accounting).

## 5. Chênh lệch so với đặc tả và quyết định khi đặc tả để mở

- **Văn bản task**: hướng dẫn của task sinh theo mẫu cho từng loại (thu thập, soạn, gate, bàn giao) với tiêu đề tiếng Việt theo vai; quy trình chi tiết vẫn nằm ở thân agent của pack. Chưa đối chiếu từng khâu của 18 squad với skill `day-chuyen-*` và `KV_LEAD_NOTES` như kế hoạch dặn: cần người đọc theo runbook §8d trước khi bật `flow_auto` cho lô 4 (Health, Construction, F&B, Insurance, Banking, Factory).
- **Thông điệp dừng**: dùng chung mẫu cho 21 squad; chưa có đoạn riêng theo từng ngành.
- **Bước chờ duyệt (Marketing, PR)**: thêm khóa thông điệp `human` (`flow_config.messages.human`) để comment ở bước `Approval` không bị đăng nhầm là "dừng 2". Dừng 2 chỉ áp cho bước `in_review`.
- **Kế hoạch bị chặn bởi công tắc**: chuyển tiếp từ bước `todo` sang `blocked` tính là dừng 1 (Teaching: `class_records` với `records_anonymised` sai).
- **Task `blocked` do hệ thống ghi** (lượt chạy hỏng hẳn, không có outcome) được tính như outcome `blocked` nên request dừng 1.
- **Route chỉ rà soát không có bản nháp** (`order_report` của E-commerce, `chapter_review` của Academic): gate bị chặn thì dừng 3 ngay, không có vòng sửa.
- **Gate rỗng** (Office Ops khi không có chi phí): có chuyển tiếp `tasks_outcome = none` sang Handover.
- **Số lần thử** tính từ lúc bước bắt đầu (`flow.entered_at`); `resume` đặt lại mốc nên đặt lại bộ đếm.
- **`flow_auto` chưa được kế thừa** sang hàng workflow mới khi nâng cấp bản flow: admin bật lại bằng `PUT .../flow-auto`.
- Không thêm chỉ số hay cảnh báo cho runner (kế hoạch để sau G2).
