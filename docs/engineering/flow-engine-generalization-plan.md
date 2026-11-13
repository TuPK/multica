# Kế hoạch triển khai: engine workflow tổng quát (schema 2) và trình soạn

Đặc tả (thiết kế, quyết định, mã lỗi, API, tiêu chí chấp nhận): [flow-engine-generalization-spec.md](flow-engine-generalization-spec.md). Tài liệu này là các bước, có thứ tự, kiểm được. Cả hai là nguồn duy nhất cho việc triển khai; chúng thay thế phần engine của [kitviet-workflow-spec.md](kitviet-workflow-spec.md) và [kitviet-workflow-plan.md](kitviet-workflow-plan.md) cho các đợt sau (hai tài liệu đó giữ vai lịch sử của đợt đầu, không sửa).

Ngày rà soát 2026-11-13, đọc ở **HEAD `d9da8d9fc`**. **Chưa có mã nào được viết.** Đường dẫn "mới" chưa tồn tại, "đổi" đã tồn tại và đã được kiểm bằng `ls` hoặc `grep` ở HEAD này. Quyết định D1 đến D10 do người dùng chốt; D11 đến D24 do đợt rà soát kiến trúc lần 2 chốt theo ủy quyền của người dùng (spec mục 1 và 15). Kế hoạch này đã được cập nhật theo đợt rà soát đó: các bước P6 đến P20 có thêm việc (đánh dấu **[R2]** = thêm bởi rà soát lần 2), và pha F (P23 đến P28: nhập bộ skill/agent, đề xuất bản nháp, chạy thử, số liệu) nay thuộc đợt này. Phong cách theo `kitviet-workflow-plan.md`.

## 0. Cách đọc và quy ước

- **Thứ tự bắt buộc** (yêu cầu của người dùng): **A** vá H1–H4 bằng test đỏ-trước → **B** đóng băng hành vi cũ và đưa 21 flow sang biểu diễn mới với bằng chứng không đổi quyết định runtime → **C** engine tổng quát và API → **D** bản nháp, xuất bản → **E** client và trình soạn → **F** nhập bộ skill/agent, đề xuất bản nháp, chạy thử, số liệu. Không bước E nào bắt đầu trước G-E2; không bước F nào bắt đầu trước G-C; giao diện chỉ hiển thị kết quả của máy chủ (I1 của spec).
- **Cổng:** G-A (sau P4), G-L (sau P7, quan trọng nhất: dừng hẳn nếu một trong 21 flow lệch), G-E1 (sau P11), G-E2 (sau P13), G-B (sau P15), G-C (sau P22), G-F (cuối, sau P28).
- Mỗi bước là một commit nguyên tử: `fix(workflow): …`, `test(workflow): …`, `feat(workflow): …`, `feat(squads): …`, `docs(squads): …`. PR theo mẫu (Problem, Goal, Proposal); PR giao diện đính kèm ảnh chụp hoặc video (AGENTS.md).
- **Test viết trước** (đỏ rồi xanh), cạnh mã. Test Go chạm DB dùng `server/internal/testutil` (`dbfx.Issue`, `dbfx.Task`, `dbfx.Insert`) và `testutil.Call(h, req).Want(status).JSON(&out)`; khẳng định trên dòng fixture của test, không trên tổng bảng. Tệp `.test.ts` không cần DOM bắt đầu bằng `// @vitest-environment node`. Test views không mock `next/*`, `react-router-dom`; mock API ở `@multica/core/api`.
- Test mặc định không gọi agent thật hay CLI agent của người dùng. "Agent giả" = test gọi API với tư cách agent (như `flow_chaos_test.go`, `flow_rehearsal_trade_test.go`).
- Nhãn kiểm: **[N]** không cần DB. **[D]** cần DB: chỉ chạy sau guard `scripts/require-test-database.sh`, chỉ khi G0 đạt.
- Ước lượng ngày công (ước lượng của tác giả thiết kế, chưa đo) ghi cuối mỗi bước.
- Điều chưa kiểm chứng được liệt kê ở [spec mục 14.2](flow-engine-generalization-spec.md). Trong kế hoạch này, H1 đến H4 là nghi vấn **chưa kiểm chứng** cho tới khi P1 chạy; nguồn phiên bản CLI của runtime (mục 8) và hành vi thông báo cho task giao thành viên (P12) cũng chưa kiểm chứng.

### 0.1 Lệnh chuẩn

| Mã | Lệnh | Ghi chú |
| --- | --- | --- |
| **[B]** | `node scripts/catalog/cli.mjs build` (hoặc `--target go`) | Ghi mã sinh (gitignore). Chạy trước `vitest`, `tsc`, `go test` chạy trực tiếp. Không `git add` chúng |
| **[C]** | `pnpm catalog:check`; `node --test scripts/catalog/*.test.mjs scripts/generate-kitviet-pack.test.mjs` | Chỉ đọc tệp |
| **[Q]** | `make sqlc` (`cd server && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`, `Makefile:377`) | Không DB. Sau mọi sửa `server/pkg/db/queries/*.sql`; commit cả `server/pkg/db/generated/*` |
| **[GN]** | `node scripts/catalog/cli.mjs build --target go && ( cd server && go test -count=1 ./internal/issueworkflow/... -run '<mẫu>' )`; thêm `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'`, `./internal/migrations -run 'TestMigrationFilesHaveMatchingDirections\|TestMigrationNumericPrefixesAreUnique'`, `./cmd/multica -run '<mẫu>'` | Nhãn [N]. Luôn kèm `-run` khi gói có test DB. `internal/issueworkflow` không import `testutil` hay đọc `DATABASE_URL` |
| **[D]** | `DATABASE_URL='<dsn-test>' bash scripts/require-test-database.sh && ( cd server && DATABASE_URL='<dsn-test>' go test -count=1 -p 1 <gói> -run '<mẫu>' )` | `<dsn-test>` = `postgres://<user>:<mật khẩu>@localhost:5432/<tên>_test?sslmode=disable`, do người dùng cấp. Guard từ chối tên DB không kết thúc `_test` hoặc bằng `multica` |
| **[M]** | `DATABASE_URL='<dsn-test>' bash scripts/require-test-database.sh && ( cd server && DATABASE_URL='<dsn-test>' go run ./cmd/migrate up )` | Sau P14 |
| **[W]** | `pnpm --filter @multica/core exec vitest run <mẫu>`; `pnpm --filter @multica/views exec vitest run <mẫu>`; `pnpm --filter @multica/core exec tsc --noEmit`; `pnpm --filter @multica/views exec tsc --noEmit` | Nhãn [N]; chạy [B] trước |
| **[T]** | `pnpm typecheck`, `pnpm lint`, `pnpm test` (gốc repo, không gồm mobile) | Nhãn [N] |
| **[V]** | `( cd server && go build ./... && go vet ./internal/issueworkflow/... ./internal/handler/... ./internal/service/... ./cmd/... )` | Sau mỗi bước Go |
| **[E]** | `pnpm exec playwright test` | **Không chạy** trừ khi người dùng cho phép |

**Cổng G0 (DB test):** người dùng tự tạo database test, tên **kết thúc `_test`** và **khác `multica`**, rồi cấp DSN; mọi lệnh cần DB đặt `DATABASE_URL` tường minh trong chính lệnh và chạy sau guard `scripts/require-test-database.sh` (guard chỉ đọc biến môi trường, từ chối tên không hợp lệ; test của guard: `bash scripts/require-test-database.test.sh`). Vai trong DSN cần quyền CREATEDB nếu test dùng `testutil.IsolatedDB`. Agent không chạy `CREATE DATABASE`/`createdb`. Trước G0, bước [D] chỉ viết test và PR ghi "chưa chạy". Không có DB test thì mọi bước [D] đứng yên và kế hoạch dừng ở đó.

### 0.2 Không được làm

1. **Không chạy** `make test`, `make check`, `make migrate-up`, `make up`, `make down`, `make destroy`, `scripts/ensure-postgres.sh`, `go test` trần hay `go run ./cmd/migrate up` trần. Không chạy lệnh tạo database. Không ghi vào DB không kết thúc `_test` hoặc bằng `multica`.
2. Không khóa ngoại, không `ON DELETE/UPDATE CASCADE`, không `CHECK` mới. Mọi chỉ mục `CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS`, một chỉ mục một tệp một câu lệnh, có dòng trong `concurrentIndexCleanups` (`server/cmd/migrate/main.go:142`).
3. Không sửa `server/pkg/db/queries/*.sql` mà không chạy [Q] và commit mã sinh.
4. Không commit mã sinh của catalog (`packages/core/catalog/generated/**`, `server/internal/catalog/generated/*`).
5. **Không sửa** 21 manifest `catalog/workflows/flow_kv_*`, không đổi `rev` của chúng, không sửa hàng `issue_workflow` nào đã có, không đổi `reworkReturnLimit = 3` (`issue_workflow_entry.go:160`), không đổi `applyIssueWorkflow` cho workflow không có `flow_config`. `TestWorkflowGolden_*` (`issue_workflow_golden_test.go`) phải xanh sau mọi bước.
6. Không thêm đường dự phòng, ghi đôi hay bộ chuyển đổi tạm ngoài `Lift`/`Adapt` đã được chấp thuận (AGENTS.md "Change and Delivery Rules"); không thêm chế độ bóng (shadow) chạy hai bộ diễn giải song song trong mã sản phẩm. Được phép và bắt buộc: điểm móc `flowDecideObserver` chỉ gán trong tệp `_test.go` (D24, spec 9.3 điều 7).
7. Không nhân đôi quy tắc kiểm giữa Go và TypeScript: client không có hàm "hợp lệ hay không". Đồ thị/bố cục là việc hiển thị, được phép ở client.
8. Không đặt mã phát sự kiện trạng thái ở `package service` (cần `handler.IssueResponse`).
9. Không đụng `apps/web/AGENTS.md` (đang có thay đổi chưa rõ nguồn).
10. Không `git commit/push/stash/reset/checkout/add` ngoài quy trình PR của người triển khai. Không hoàn tác thay đổi của người khác.
11. Mọi chuỗi giao diện mới đủ 7 locale (`en`, `zh-Hans`, `zh-Hant`, `fr`, `es`, `ko`, `ja`); không mô tả thừa (conventions.mdx "UI descriptions").

### 0.3 Xung đột với việc đang chạy

Hai công việc khác đang chạy song song. (1) Agent diễn tập web (đã commit `cf09459cf`, `c342c7cc2`, `ccb6892bf`) làm ở `packages/views/workflow/designer/*`, `packages/views/issues/components/issue-detail.tsx`, `packages/views/workflow/issue-flow-state-panel.tsx`, `server/internal/handler/flow_guards.go`, `server/internal/handler/issue.go`, và lúc rà soát còn sửa chưa commit `server/pkg/db/queries/flow.sql`, `server/pkg/db/generated/flow.sql.go`, `server/internal/handler/flow_runner_test.go`, `issue-flow-state-panel.tsx` (+ test) và `issues.json` của 7 locale. (2) Một agent khác sửa lỗi wakeup ở `server/internal/service/issue_wakeup*.go` và `server/internal/scheduler/jobs_issue_wakeup.go` (đã commit `d9da8d9fc`, có thể còn tiếp). Kế hoạch này **không tự đoán** nội dung thay đổi của họ.

Quy tắc: trước **mỗi** bước chạm các tệp dưới đây, chạy `git status --short -- <tệp>` và `git log -3 --format='%h %s' -- <tệp>`. Nếu có sửa chưa commit của người khác: **phải chờ commit của công việc đang chạy**, rồi làm trên bản mới nhất; nếu không rõ ai đang sửa thì dừng và hỏi. Mỗi bước bị ảnh hưởng ghi dòng "Xung đột" riêng.

| Tệp hoặc nhóm | Bước chạm | Cách phối hợp |
| --- | --- | --- |
| `server/internal/handler/flow_guards.go` | P2, P3, P11, P12 | Phải chờ commit của công việc đang chạy; đây là tệp nóng nhất |
| `server/internal/handler/issue.go` (chỉ chỗ gọi `checkFlowWrite` `:4254`, `:5052` và kiểm outcome `:4247`, `:5033`) | P2, P12 | như trên |
| `server/internal/handler/flow_state.go` (chỉ thêm trường phản hồi) | P2, P11, P12 | như trên |
| `server/pkg/db/queries/flow.sql` và `server/pkg/db/generated/*` (mỗi lần [Q] sinh lại cả thư mục) | P4, P10, P12, P14 | Phải chờ commit của công việc đang chạy ở `flow.sql`/`flow.sql.go` trước khi sửa `.sql` hay chạy [Q]; nếu không, bản sinh lại sẽ ghi đè hoặc xung đột với thay đổi chưa commit của họ |
| `server/internal/handler/flow_runner.go`, `flow_sweep.go`, `issue_child_event.go` và `server/internal/service/issue_wakeup_system.go:866-869` (đọc `IsFlowRequestAuto`) | P11 (không đổi chữ ký `advanceFlowRequests`, `isAutoFlowRequest`, `IsFlowRequestAuto`) | Nếu P11 buộc phải đổi chữ ký hay ngữ nghĩa: phải chờ commit của công việc wakeup, rồi phối hợp |
| Nhóm `server/internal/service/issue_wakeup*.go`, `server/internal/scheduler/jobs_issue_wakeup.go` | Không bước nào sửa | Các bước chạy test `./internal/service ./internal/scheduler` (G-A, P13, G-E1, G-E2) chỉ có ý nghĩa khi công việc wakeup đã commit; trước đó, lỗi ở hai gói này không quy cho kế hoạch này |
| `packages/views/workflow/issue-flow-state-panel.tsx` (+ test) | P19 | Phải chờ commit của công việc đang chạy; thay đổi của ta chỉ thêm theo `viewer` và bộ verdict |
| `packages/views/workflow/designer/*` | P20 (chỉ `condition-builder.tsx`: nhận `facts?: FactSpec[]`, mặc định giữ hành vi) | Phải chờ commit của công việc đang chạy; **không** đụng `workflow-designer-page.tsx`, `designer-canvas.tsx`, `step-inspector.tsx` |
| `packages/views/issues/components/issue-detail.tsx` | Không bước nào | Panel nhận cùng props; `viewer` nằm trong dữ liệu của panel |
| `packages/views/issues/actions/use-issue-actions.ts`, `packages/views/issues/utils/issue-update-error.ts` | Không bước nào | Mã lỗi mới chỉ thêm khóa dịch `errors.<code>` ở khóa mới của `squads.json`, không sửa `issues.json` |
| `apps/web/AGENTS.md`, `docs/engineering/kitviet-flow-web-drill.md` | Không bước nào | Đang có thay đổi chưa rõ nguồn / đang được ghi bởi người khác |
| Trình soạn mới `packages/views/squads/flow-designer/*` | P17–P19 | Thư mục mới, không xung đột |
## 1. Pha A: vá H1–H4 (D4, D5)

### P0. Preflight (không sửa tệp) [N]

- `git rev-parse --short HEAD` ghi vào PR (kế hoạch viết ở `d9da8d9fc`).
- `ls server/migrations | tail -3` phải kết thúc ở `987_issue_flow_open_idx`; nếu đã có 988 trở lên thì dịch số của P14.
- `git status --short -- server packages scripts catalog docs` chỉ còn thay đổi của công việc khác đã nêu ở 0.3; ghi danh sách vào PR. Nếu có sửa chưa commit ở tệp mà bước sắp làm sẽ chạm thì chờ.
- [C] in `catalog ok`. [GN] `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'` xanh. `bash scripts/require-test-database.test.sh` xanh.
- **Gate:** D11 đến D24 đã chốt (spec mục 1), không còn gì chờ xác nhận. Còn lại G0: có DSN test kết thúc `_test` (người dùng cấp, hoặc DB do `testutil.IsolatedDB` tạo); không có thì các bước [D] chỉ viết test và ghi "chưa chạy".
- Ước lượng: 0,5.

### P1. Chứng minh hoặc bác bỏ H1–H4 bằng test [D]

- **Tệp (mới):** `server/internal/handler/flow_guards_actor_test.go` (dùng `flow_fixture_test.go`, `flow_guards_close_test.go`).
- **Test (viết trước, chưa đổi mã sản phẩm):**
  - `TestFlowGuard_AgentCannotCloseAnothersTask` (H1: agent B đóng gate giao cho A bằng `done --outcome approved`; kỳ vọng 403 `flow_task_not_assignee`).
  - `TestFlowGuard_PlainMemberCannotCloseTask` (H1 cho thành viên thường: gửi `status: done, outcome: approved` đúng như `handleVerdict` của `ccb6892bf`; kỳ vọng 403 `flow_task_not_assignee`).
  - `TestFlowGuard_ManagerClosesOnBehalfAndIsLogged` (D4: `canManageSquad` đóng được; có `activity_log` `flow_manual_close` kèm verdict, số tệp, người được giao).
  - `TestFlowGuard_ManagerOnBehalfStillNeedsTheFiles` (luật tệp vẫn áp; tệp do **chính quản lý** tải lên task được tính).
  - `TestFlowGuard_AgentCannotReassignFlowTask` (H2: đổi `assignee_id`, trạng thái giữ nguyên; 409 `flow_task_assignee_locked`).
  - `TestFlowGuard_AgentCannotCancelWorkTask` (H3: task `work` có `requires attachment`; 409 `flow_task_cancel_forbidden`; bước không chuyển).
  - `TestFlowGuard_ClosedVerdictFrozenForAgent` (H4: gate `rejected` rồi agent đóng lại `approved` trước `advance`; 409 `flow_task_closed`).
  - `TestFlowGuard_ManagerCanReopenClosedTask` (có nhật ký).
  - `TestFlowGuard_BatchPathSameRules` (cùng luật qua `issue.go:5052`).
- **Kiểm:** [D] `./internal/handler -run 'TestFlowGuard_'`. Ghi vào PR từng test đỏ (nghi vấn đúng) hay xanh (đã có luật khác chặn: xóa dòng tương ứng ở P2/P3/spec E1–E4, không thêm mã).
- **Gate:** bảng đỏ/xanh cho H1, H1m, H2, H3, H4. Ước lượng: 1,5.

### P2. Chặn H1, H1m, H2, H4 (E1, E2, E4) [D]

- **Xung đột:** **phải chờ commit của công việc đang chạy** ở `flow_guards.go`, `issue.go`, `flow_state.go` trước khi bắt đầu; không tự đoán nội dung thay đổi của họ (mục 0.3).
- **Tệp (đổi):** `server/internal/handler/flow_guards.go` (`checkFlowWrite` `:136` không trả sớm khi chỉ đổi `assignee_*` của task flow; `checkFlowTaskStatus` `:248` nhận `actor`, so với `task.AssigneeID` và `canManageFlowRequest` `:197`; từ chối đổi trạng thái/outcome của task đã đóng trong lần vào bước hiện tại với actor agent); `server/internal/handler/issue.go` (điểm gọi `:4254` và batch `:5052` truyền `actor`); `server/internal/handler/flow_state.go` (thêm `viewer {can_manage, closable_task_ids}` và `closed_by` vào phản hồi: **chỉ thêm trường**) và `packages/core/types/issue-flow.ts`, `packages/core/api/schemas.ts` (cùng trường, có mặc định) để P19 ẩn nút theo quyền.
- **Quy tắc:** agent chỉ đóng task giao cho nó; thành viên đóng task phải `canManageSquad` hoặc admin (đóng thay, ghi `flow_manual_close`); agent không đổi `assignee_*` của task flow; agent không đổi task đã đóng; thành viên mở lại task bằng thao tác riêng có nhật ký.
- **Test:** các test của P1 (phần đỏ) xanh; thêm `TestFlowState_ViewerFieldsOnlyForManagers` ([D] `flow_state_kind_test.go` hoặc tệp mới); schema zod: phản hồi thiếu `viewer` vẫn parse (mặc định `can_manage:false`, `closable_task_ids:[]`) ([W] `api/schemas.test.ts`).
- **Kiểm:** [Q] nếu có truy vấn mới; [V]; [D] `./internal/handler -run 'TestFlowGuard_|TestFlowTaskClose_|TestGuard_|Golden|TestPlainIssueUnaffected|TestFlowState'`; [W] `core exec vitest run api`.
- Ước lượng: 2.

### P3. Chặn H3: hủy task `work` (E3, E7) [D]

- **Xung đột:** **phải chờ commit của công việc đang chạy** ở `flow_guards.go` trước khi bắt đầu; không tự đoán nội dung thay đổi của họ (mục 0.3).
- **Tệp (đổi):** `flow_guards.go` (agent đặt `cancelled` cho task flow: 409 `flow_task_cancel_forbidden`), `flow_engine.go` (`loadFlowTasks` `:102` và `advanceOnce` `:523`: task `work` bị người hủy mà bước còn `requires` → mở lại ở cùng vòng, nhật ký `flow_task_reopened`, trừ khi `resume` chọn `skip_cancelled`), `flow_engine_ops.go` (`FlowResumeInput` thêm `SkipCancelled`; `ResumeFlow` `:224`).
- **Test (trước):** `TestAdvance_ManagerCancelledWorkTaskReopens`, `TestResume_SkipCancelledWorkTaskContinues`, `TestAdvance_CancelledGateStillRejects` (giữ `TestChaos_TaskCancelledByMember`, `flow_chaos_test.go:133`).
- **Kiểm:** [D] `./internal/handler -run 'Advance|Resume|Chaos|TestFlowGuard_'`. Ước lượng: 2.

### P4. Kiểm lại thành viên squad và lead lúc mở task (E14) [D]

- **Xung đột:** **phải chờ commit của công việc đang chạy** ở `server/pkg/db/queries/flow.sql` / `server/pkg/db/generated/*` ([Q]) và, nếu cần, `squad.sql` trước khi bắt đầu; không tự đoán nội dung thay đổi của họ (mục 0.3).
- **Tệp (đổi):** `flow_engine.go` (`openFlowTasks` `:598`, sau kiểm agent lưu trữ ở `:600-606`: thêm "còn là thành viên agent của squad của workflow và không phải lead"; lỗi dùng `stopForUnavailableAgent` `:724`); `server/pkg/db/queries/squad.sql` (`ListSquadMembers` `:140` đã có; thêm `IsSquadAgentMember` nếu cần) và [Q].
- **Test (trước):** `TestAdvance_AgentLeftSquadStalls`, `TestAdvance_AgentBecameLeadStalls`, `TestAdvance_CatalogFlowAgentsStillOpen`.
- **Kiểm:** [Q]; [D] `./internal/handler -run 'Advance|Rehearsal'`. Ước lượng: 1.
- **Gate G-A:** (kết quả `./internal/service`, `./internal/scheduler` chỉ tin được sau khi công việc wakeup đã commit, mục 0.3) [D] `go test -count=1 -p 1 ./internal/handler ./internal/service ./internal/scheduler -run 'Flow|Guard|Golden|Rehearsal|Chaos|Advance|Resume|Detach|Sweep|Runner'` xanh; PR ghi **danh sách hành vi đã đổi** của flow Kit Việt đang chạy (E1–E4, E14) để người dùng duyệt (D5).

## 2. Pha B: đóng băng hành vi cũ và đưa 21 flow sang biểu diễn mới

### P5. Đóng băng tiêu chuẩn: tiêu chuẩn so sánh và bản chụp văn bản [N]

- **Tệp (mới):** `server/internal/issueworkflow/flow_v1_oracle_test.go` (sao chép **nguyên văn** `Decide`, `route`, `tasksOutcomeOf`, `RoutePlan`, `currentTasks`, `stalledRoute` từ `flow.go:352-488` đổi tên `oracle*`; đây là mã thuần, chỉ nằm trong tệp test), `flow_v1_texts_test.go`, `testdata/v1_texts/<flow-id>.golden` (21 tệp), `testdata/v1_states/*.json` (trạng thái `issue.flow` mẫu: đầu, giữa, `rework 1`, `round_base 1`, dừng 1/3/stalled, đã đóng).
- **Test (trước mọi thay đổi mã engine):**
  - `TestOracleMatchesProductionDecide`: oracle và `Decide` hiện tại cho cùng kết quả trên bảng `TestDecide` (`flow_test.go:406`) và `TestRoutePlan` (`:561`). Chứng minh bản chép trung thực.
  - `TestV1TextsGolden`: với mỗi flow catalog (dùng bộ nạp `catalogFlow` của `flow_catalog_test.go:13-`), vòng 0 và 1: `TaskDescription`, `FlowTaskBrief`, `FlowLeadBrief` (bước plan và sau plan), thông điệp dừng đã thay biến; so với `.golden`. Sinh bằng cờ `-update` lần đầu **trên mã chưa đổi** rồi commit.
  - `TestV1StatesDecode`: các `testdata/v1_states/*.json` giải mã vào `FlowState` hiện tại và mã hóa lại không mất khóa.
- **Kiểm:** [GN] `./internal/issueworkflow/... -run 'TestOracle|TestV1'`. Ước lượng: 2.
- **Gate:** xanh trên mã chưa đổi (nếu `.golden` sinh ra khác văn bản đang chạy thì dừng và hỏi).

### P6. Mô hình dữ liệu: FlowDoc, đồ thị, sổ dữ kiện [N]

- **Tệp (mới) trong `server/internal/issueworkflow/`:** `flowdoc.go` (cấu trúc 2.3, `ParseFlowDoc` nghiêm ngặt: khóa lạ là lỗi), `graph.go` (`Graph`, chỉ mục bước/cạnh/vòng, `compat`), `facts.go` (`FactRegistry`, `FactSpec`, `BuildFacts`), `state.go` (**[R2]** `State` schema 2 gồm `trail`, `unreviewed`, `var_sets`, `trial`; hàm thuần `AppendTrail`), `expr.go` (`EvaluateWith(c Condition, fs FactSet)` tái dùng cây `Condition` của `transitions.go:73-81`, thêm toán tử `contains` và kiểu `int/string/list_string`), tests cạnh tệp. **Tệp (đổi):** `transitions.go` chỉ để xuất hàm so sánh dùng chung; `Evaluate` cũ cho workflow không-flow **không đổi**.
- **Test (trước):** `TestParseFlowDocStrict` (khóa lạ ở mọi cấp, `version` cao, quá 512 KiB, khóa sai mẫu), `TestFactRegistry_EverySourceIsDeclared` (bảng liệt kê từng dữ kiện với nguồn: chỉ `engine|request|var`), `TestFactRegistry_RejectsLegacyNamesInV2`, `TestBuildFacts_FromServerStateOnly` (không có tham số văn bản), `TestExpr_TypedOperators` (bool/enum/int/string/list), `TestExpr_UnknownFactIsAnErrorNotFalse`, `TestExpr_NotOverMissingFactDoesNotLeak`, `TestGraph_IndexesAndBackEdges`. **[R2]** `TestParseFlowDoc_TaskInputsSetsSkills` (trường mới của 2.3; `requires.vars ⊄ sets` là lỗi phân tích), `TestExpr_DeclaredButAbsentFact` (dữ kiện đã khai chưa có giá trị: `exists` sai, so sánh sai, bộ đếm 0; khác trường ngoài sổ), `TestGraph_LoopBodies` (thân vòng theo spec 4.5, kể cả hai vòng không lồng nhau và hai vòng cùng `header`), `TestState_TrailCappedAtMaxActivations`.
- **Kiểm:** [GN] `./internal/issueworkflow/... -run 'FlowDoc|FactRegistry|BuildFacts|Expr|Graph|State_'`; [V]. Ước lượng: 5.

### P7. `Lift`, `Adapt`, `Unadapt` và bằng chứng vi phân [N]

- **Tệp (mới):** `lift.go` (`Lift(def Definition, opts LiftOpts) (Graph, error)`, quy tắc 9.2 của spec: vai, biến, bước, task, cạnh, tách cạnh tới `blocked`, vòng `rework`, mã dừng, `compat`), `adapt.go` (`Adapt`, `Unadapt` giữa `FlowState` v1 và `issue.flow` v2), `lift_test.go`, `lift_differential_test.go`.
- **Test (trước):**
  - `TestLift_DecideEquivalent_AllCatalogFlows`: với **mỗi** flow trong `flows.json`, mỗi bước, `rework ∈ [0, N+1]`, mọi tổ hợp công tắc, tập trạng thái task (mở, đóng×{không outcome, approved, rejected, blocked}, bị hủy): so oracle với `Decide(Lift(def), Adapt(state), tasks)` trên (kind, task thiếu, đích hoặc stop, cờ rework, mã dừng, HumanStop, id cạnh). Vượt 50.000 trạng thái mỗi bước thì lấy biên + 20.000 mẫu ngẫu nhiên hạt cố định (in hạt khi lỗi).
  - `TestLift_StopSplit` (bảng: cạnh mặc định tới `blocked` ở bước gate có/không dữ kiện `block`; ở bước handover; bước `todo`; cạnh đã có `any_blocked`).
  - `TestLift_ConditionRewrite` (`tasks_outcome`, `rework_count`, `outcome`, `switch.*`), `TestLift_ReworkLoopMaxIsLargestN`, `TestLift_SingleCounterWithTwoReworkEdges` (bộ kiểm hợp lệ schema 1 cho phép nhiều cạnh `rework`; lift giữ bộ đếm chung).
  - `TestLift_RunsRoundsLikeV1` (`compat`: `Round()` bằng `RoundBase+Rework`), `TestLift_InputsByPositionLikeV1`.
  - `TestLift_ManifestKeepsRoleKeys` và `TestLift_ForkDocIsCleanV2` (kết quả `opts.Runtime=false` không có `compat`, qua `ValidateFlowDoc` của P9 với danh sách vai suy ra; chạy thật sau P9).
  - `TestAdapt_RoundTrip` (mọi `testdata/v1_states/*.json` qua `Adapt` rồi `Unadapt` byte-trùng khóa), `TestAdapt_StopCodes` (1↔`grounds_missing`, 3↔`loop_exhausted`, `"stalled"`).
  - `TestV1TextsGolden` (P5) vẫn xanh khi nội dung được sinh từ đồ thị lifted.
  - **[R2]** `TestLift_RuntimeGraphIsExemptFromNewRules` (spec 9.2 điều 12: `allow_unattended_release = true`, tách vai "cùng vòng", đầu vào theo vị trí) và `TestLift_ForkDocGetsNoExemption` (bản nhân bản chịu Y01 và R07 mới; ghi vào PR flow catalog nào nhận cảnh báo gì).
- **Kiểm:** [GN] `./internal/issueworkflow/... -run 'Lift|Adapt|TestV1|TestOracle'`; [V]. Ước lượng: 5,5.
- **Gate G-L (dừng hẳn nếu lệch):** vi phân xanh cho cả 21 flow, `.golden` không đổi, `Adapt/Unadapt` khép kín. Một flow lệch dù một ca = không đi tiếp; điều tra, báo, không "sửa cho khớp" bằng cách đổi oracle.

### P8. `Decide` tổng quát và mô phỏng [N]

- **Tệp (mới):** `decide.go` (`Decide(g Graph, st State, tasks []TaskState, fs FactSet) Decision`: bước `lead`/`tasks`/`manual`, task sẵn sàng với `after`, `instances`, `join` all/any/quorum, `block: immediate`, chọn cạnh theo thứ tự, vòng, `activation`, cạnh `stop`, terminal, trần), `simulate.go` (`Simulate`, `Analyze`: `worst_case_task_issues`, `worst_case_activations`, `loops`, `stops`, `release_paths_without_human`), `links.go` (**[R2]** `TaskLinks`: đầu vào tường minh, đầu vào mặc định theo `trail`, phản hồi của cạnh quay lại; spec 4.11), `assign.go` (**[R2]** chọn người trong hồ bơi, spec 2.5), `testdata/flows/{trade,research,market,legal}.json` (bốn FlowDoc mẫu của analysis 3.1–3.4), **[R2]** `testdata/flows/{crossreview,planner_fanout}.json` (bộ hai agent duyệt chéo; agent lập kế hoạch đặt danh sách cho `instances`) và `testdata/flows/patterns/*.json` (mười hai mẫu "có" của spec Phụ lục F.2).
- **Test (trước):** `TestDecide_JoinAll_MatchesLiftedV1`, `TestDecide_JoinAnyCancelsTheRest`, `TestDecide_QuorumPasses`, `TestDecide_QuorumFailsEarly`, `TestDecide_NeutralIsNotCounted`, `TestDecide_BlockIsImmediate`, `TestDecide_AfterOpensInWaves`, `TestDecide_InstancesFromListVar`, `TestDecide_InstancesRespectMax`, `TestDecide_LoopCountsAndResetsOnForwardEntry`, `TestDecide_NestedLoopResets`, `TestDecide_LoopMaxFromVarIsCapped`, `TestDecide_ExhaustedBackEdgeFallsThrough`, `TestDecide_NoEdgeStalls`, `TestDecide_ManualStepWaitsForExit`, `TestDecide_StopEdgeParksWithCode`, `TestDecide_TerminalWithoutCheckpointStalls`, `TestDecide_ActivationCapStalls`.
  - `TestSimulate_{Trade,Research,Market,Legal}` (kịch bản trong `testdata/flows/*.json`: thành công, từ chối rồi qua, cạn vòng, dừng 1; research: `max_rounds` 2 và 5 (kẹp ở 5); legal: quorum 3/5 thoát sớm, quay lại bước cụ thể).
  - `TestSimulate_HasNoDatabaseDependency` (kiểm `go list -deps` của gói không có `pgxpool` ngoài kiểu `pgtype` sẵn có), `TestSimulate_Truncates`.
  - **[R2] Ngữ nghĩa sửa bởi rà soát lần 2:** `TestDecide_AllNeutralIsNoneNotPass`, `TestDecide_GateClosedWithoutVerdictFails`, `TestDecide_SupersededIgnoredByJoin`, `TestDecide_QuorumUnreachableFromStartFails`, `TestDecide_BlockAtCloseWaits`, `TestDecide_OpenTaskSurvivesVarChange` (spec 4.2a i), `TestDecide_InstancesTooFewStalls`, `TestDecide_UnreviewedBlocksRelease` và `TestDecide_CheckpointClearsUnreviewed` (spec 4.8), `TestDecide_ResumeCountsAsForwardEntry`, `TestDecide_SameHeaderLoopsAreIndependent`.
  - **[R2] Luồng dữ liệu:** `TestTaskLinks_ExplicitInputsLatestActivation`, `TestTaskLinks_DefaultFollowsTrailNotDeclarationOrder`, `TestTaskLinks_FeedbackAfterBackEdge`, `TestTaskLinks_RequiredInputMissingStalls`, `TestAssign_StickyAcrossActivations`, `TestAssign_SkipsWorkerForGate`, `TestAssign_PoolExhaustedStalls`.
  - **[R2] Mẫu luồng:** `TestPatterns_AllValidateAndSimulate` (mỗi tệp `patterns/*.json` qua bộ kiểm của P9 và cho kết cục ghi trong tệp; chạy thật sau P9; K23).
  - **Tính chất:** `TestAnalyze_BoundCoversAdversarialSimulation` (số lần vào bước và số task của mô phỏng đối kháng không vượt công thức 4.5), `TestDecide_AlwaysTerminatesWithinBound` (sinh đồ thị hợp lệ ngẫu nhiên hạt cố định, chạy mô phỏng với kịch bản đối kháng, khẳng định kết thúc trong `worst_case_activations`).
- **Kiểm:** [GN] `./internal/issueworkflow/... -run 'TestDecide_|TestSimulate_|TestTaskLinks_|TestAssign_|TestAnalyze_|TestPatterns_'`. Ước lượng: 8.

### P9. Bộ kiểm hợp lệ có tên [N]

- **Tệp (mới):** `flowdoc_validate.go` (`ValidateFlowDoc(doc, ctx) []Issue`, mục 3 của spec: S, G, R, T, J, F, P, Y; `ValidateSchema1ForSquad(def, roster) []Issue` cho các quy tắc R của schema 1), `flowdoc_validate_test.go`. **Tệp (đổi):** `flow_catalog_test.go` (`TestCatalogFlowsPassValidateFlow` `:100` thêm gọi `ValidateSchema1ForSquad` với danh sách vai suy ra từ manifest squad `catalog/squads/kv_*/template.json`).
- **Test (trước):** mỗi mã một cặp dương/âm: `TestValidate_S01…S09`, `G01…G11`, `R01…R08`, `T01…T06`, `J01…J03`, `F01…F06`, `P01…P02`, `Y01…Y05` (bảng chạy `t.Run(code)`); `TestValidate_ReturnsAllIssuesAtOnce`; `TestValidate_PathsUseKeysNotIndexes` (sắp xếp lại bước không đổi `path`); `TestValidate_G11WorstCaseNumbers` (số tính được khớp công thức, so với `Analyze`); `TestValidate_R07SeparationIsAcrossTheWholeFlow`; `TestValidate_Y01ReleasePaths` (bao gồm bước `manual`, task người `pass`, và đường vòng qua cạnh `stop`); `TestValidateSchema1ForSquad_AllCatalogFlowsPass`. **[R2]** `R09` (`skill_not_attached`: skill gắn trực tiếp, skill qua squad, vai người), `T07` (`input_invalid`, `input_maybe_missing`, `required` trên nhánh không chắc), `T08` (`var_set_invalid`, `var_maybe_unset`), `TestValidate_R07SeparationByInputs` (thay `…AcrossTheWholeFlow`: duyệt chéo hai agent hợp lệ; tự duyệt bị từ chối; hồ bơi đủ người hợp lệ), `TestValidate_Y01WorkAfterHumanReview` (người duyệt sớm rồi agent viết lại: cảnh báo; task agent trong bước `manual`: không cảnh báo), `TestValidate_J02NeutralNeedsNoneRoute`, `TestValidate_J01QuorumCountsOnlyCertainInstances`.
- **Kiểm:** [GN] `./internal/issueworkflow/... -run 'TestValidate|TestCatalogFlows|TestPatterns_'`. Ước lượng: 6,5.
- **Gate:** 21 flow catalog qua `ValidateSchema1ForSquad` **mà không sửa manifest**. Một flow rớt thì dừng và hỏi (có thể là lỗi thật của manifest, không sửa im lặng).

## 3. Pha C: tích hợp engine và API chạy

### P10. Ghi trạng thái so-sánh-đổi trên `seq` [D]

- **Xung đột:** **phải chờ commit của công việc đang chạy** ở `server/pkg/db/queries/flow.sql`, `server/pkg/db/generated/*` ([Q]) trước khi bắt đầu; không tự đoán nội dung thay đổi của họ (mục 0.3).
- **Tệp (đổi):** `server/pkg/db/queries/flow.sql` (`SetIssueFlow`: điều kiện `flow->>'step'` và `COALESCE((flow->>'seq')::int,0) = expected_seq`, thay `rework`), `server/internal/handler/flow_engine.go` (`saveFlowState` `:230`). [Q].
- **Test (trước):** `TestSetIssueFlow_ConcurrentWritersLoseOnSeq`, `TestSetIssueFlow_StateWithoutSeqCountsAsZero`, giữ `TestAdvance_ConcurrentWritersLoseOnFlowCondition`, `TestChaos_SweepAndEventConcurrent`.
- **Kiểm:** [Q]; [V]; [D] `./internal/handler -run 'SetIssueFlow|Concurrent|Chaos'`. Ước lượng: 1.

### P11. Chuyển bộ diễn giải: engine đọc `Graph` (G-E1) [D]

- **Xung đột:** **phải chờ commit của công việc đang chạy** ở `flow_guards.go`, `flow_state.go`, `flow_runner.go`, `flow_sweep.go` (và không đổi chữ ký đọc bởi `issue_child_event.go`, `service/issue_wakeup_system.go:866-869`) trước khi bắt đầu; không tự đoán nội dung thay đổi của họ (mục 0.3).
- **Mục tiêu:** một đường chạy duy nhất cho cả schema 1 (qua `Lift`) và schema 2. **Không đổi hành vi của schema 1.**
- **Tệp (đổi):** `server/internal/issueworkflow/issueworkflow.go` (`Definition` thêm `Graph`; `Decode` `:229` dựng `Graph` từ `flow_config.version` 1 qua `Lift` hoặc 2 qua `ParseFlowDoc`; `HasFlow`), `server/internal/handler/flow_engine.go` (`loadFlowRun` `:61`, `loadFlowTasks`, `advanceOnce` `:523`, `routeFlowRequest` `:306`, `openFlowTasks` `:598`, `postStopComment`/`stopKindOf` `:356-440` → mã dừng khai báo, `stallFlow` `:739`), `flow_engine_ops.go` (`PlanFlow` `:98` → hoàn tất bước `lead`; `SetFlowSwitch` `:169` → `SetFlowVar`; `ResumeFlow` `:224`; `DetachFlow`; `followManualMove` `:368`), `flow_guards.go` (`flowDefinition` `:66`, `flowTaskSpec` `:86`, `claimFlowBrief` `:417`), `flow_state.go`, `flow_sweep.go`, `flow_runner.go`, `server/internal/service/flow_start.go` (trạng thái khởi tạo từ `Graph`), `server/internal/handler/squad_workflow.go` (`:312-318` `target.Flow != nil` → `target.HasFlow()`).
- **Quy tắc:** hàm cũ `Decide/route/tasksOutcomeOf/RoutePlan` bị xóa khỏi mã sản phẩm (đã nằm trong oracle test). Các điều kiện `def.Flow == nil` đổi thành `HasFlow()`.
- **[R2] Ba lát, mỗi lát một commit, bộ test DB cũ xanh sau từng lát:** (a) chỉ đường đọc: `Decode` dựng `Graph`, `flowDefinition`, `flow_state.go` đọc qua `Graph`; bộ quyết định cũ vẫn chạy. (b) `advanceOnce` gọi `Decide` mới; thêm điểm móc `flowDecideObserver` (spec 9.3 điều 7) và `flow_observer_test.go` gán nó thành oracle cho **mọi** test DB của gói handler (`TestMain`), đỏ khi lệch. (c) xóa bộ quyết định cũ, chuyển `PlanFlow`/`SetFlowSwitch`/`ResumeFlow`/quét sang `Graph`; ghi `trail` và `activity_log` `flow_transition` (spec 4.13) trong cùng giao dịch với `SetIssueFlow`.
- **Test (trước):** `TestEngine_LoadsV1RowThroughLift` (hàng catalog đọc ra `Graph` có `compat`), `TestEngine_V1RowIsNeverRewritten` (hàng `issue_workflow` trước/sau một lần chạy Trade trùng từng byte), `TestEngine_V1StateRoundTripInDB` (`issue.flow` trước/sau mỗi bước chỉ khác `seq`/`step`/`entered_at`/`trail`), **[R2]** `TestEngine_ObserverSeesEveryAdvance` (điểm móc được gọi ở mỗi quyết định; mặc định `nil` ngoài test), `TestEngine_TrailAndTransitionLogPerHop` (K22), **toàn bộ test hiện có chạy không sửa:** `flow_engine_test.go`, `flow_api_test.go`, `flow_chaos_test.go`, `flow_guards_*_test.go`, `flow_runner_test.go`, `flow_rollback_test.go`, `flow_squad_workflow_test.go`, `flow_close_cancel_queued_test.go`, `flow_state_kind_test.go`, `flow_rehearsal_trade_test.go`, `flow_rehearsal_trade_auto_test.go`, `flow_rehearsal_squads_test.go`, `issue_workflow_golden_test.go`.
- **Kiểm:** [Q] nếu có truy vấn mới; [V]; [D] `go test -count=1 -p 1 ./internal/handler ./internal/service ./internal/scheduler ./internal/issueworkflow -run 'Flow|Guard|Golden|Rehearsal|Chaos|Advance|Resume|Detach|Sweep|Runner|Engine'`.
- **Gate G-E1:** toàn bộ bộ trên xanh **với điểm móc oracle bật**, test bị đổi chỉ gồm danh sách có chủ ý của D5 (nêu từng cái trong PR). Ước lượng: 9,5.

### P12. Luật đóng task tổng quát và API chạy [D]

- **Xung đột:** **phải chờ commit của công việc đang chạy** ở `flow_guards.go`, `issue.go`, `flow_state.go`, `flow.sql` + `server/pkg/db/generated/*` trước khi bắt đầu; không tự đoán nội dung thay đổi của họ (mục 0.3).
- **Tệp (đổi):** `flow_guards.go` (E5 theo bộ verdict, E6 theo `slot`/phần mở rộng/`min`, khóa task có hậu tố `~i`, giao task cho người, mã lỗi giữ `flow_outcome_required|invalid`, `flow_attachment_required`), `issue.go` (`:4247`, `:5033`: `outcome` khớp `^[a-z][a-z0-9_]{0,31}$`; với task flow kiểm theo bộ verdict, issue khác vẫn bốn giá trị), `flow_api.go` (route mới: `…/flow/steps/{step}/complete`, `…/flow/vars/{key}`, `…/flow/exits/{edgeId}`, `POST /api/issues/{taskId}/flow/close`), `server/cmd/server/router.go` (cạnh `:2363-2369`), `server/pkg/db/queries/flow.sql` (đếm tệp theo người tải/phần mở rộng, thay `CountAgentAttachmentsOnIssue`), `server/internal/issueworkflow/flow.go` (lời nhắc sinh từ bộ verdict/tệp, **[R2]** cộng mục Inputs/Feedback từ `TaskLinks`, tên skill của `Task.skills`, lệnh đặt biến cho `Task.sets`), **[R2]** `flow_api.go` thêm `PUT /api/issues/{taskId}/flow/vars/{key}` (đường task, spec 4.12) và `flow_engine.go` (`openFlowTasks`: chọn người theo `assign.go`, tách vai theo đầu vào E13, đầu vào `required` E25, bật `unreviewed`; engine hủy task thì ghi outcome `superseded`), `server/cmd/multica/cmd_issue.go` (`:732`, `:2198-2200`: bỏ kiểm cứng bốn giá trị ở máy khách), `cmd_issue_flow.go` (lệnh `vars`, `exits`, `complete`, **[R2]** `set <task-id> <key> <value>`; giữ `plan`, `switch`). [Q].
- **Test (trước):** `TestVerdictSet_CustomAcceptedOthersRejected`, `TestVerdict_BlockedAlwaysAllowed`, `TestVerdict_ClassDrivesJoinNotTheName`, `TestFiles_SlotExtensionAndMinEnforced`, `TestFiles_OnBehalfCountsTheManagersUploads`, `TestMemberTask_AssignedToMemberAndClosedByThem` (vùng chưa kiểm, xem 14 của spec), `TestMemberTask_ManagersRoleAnyoneWithRightCloses`, `TestLeadStep_CompleteSetsVarsAtomically`, `TestVars_SetByAndSetAtEnforced`, `TestVars_ManagerAnytimeIsLogged`, `TestExits_ManualEdgeByExitRole`, `TestExits_ForbiddenForOthers`, `TestClose_OnBehalfLogsAndShowsInState`, `TestStatePayload_AddsFieldsOnly` (so hình dạng phản hồi cũ). **[R2]** `TestTaskVar_AssigneeSetsDeclaredVarWhileOpen`, `TestTaskVar_OtherVarOrClosedTaskForbidden`, `TestTaskVar_RequiredBeforeClose` (`flow_vars_required`), `TestTaskVar_FrozenWhileFanOutRuns` (`flow_var_in_use`), `TestOutcome_SupersededIsEngineOnly` (E23), `TestOpen_GateSkipsWorkerInPool`, `TestOpen_RequiredInputMissingStalls`, `TestBrief_ListsInputsFeedbackSkillsAndVarCommands`, `TestRelease_UnreviewedStallsThenManualExitReleases` (K20). CLI [N] ở `cmd/multica`: `TestIssueStatusPassesAnyOutcomeKey`, `TestIssueFlowVarsExitsCompleteCommandsCallEndpoints`, `TestIssueFlowHelp` (golden cập nhật).
- **Kiểm:** [Q]; [V]; [GN] `./cmd/multica -run 'IssueStatus|IssueFlow'`; [D] `./internal/handler -run 'Verdict|Files|MemberTask|LeadStep|Vars|Exits|Close|StatePayload|TaskVar|Outcome_|TestOpen_|TestBrief_|TestRelease_|TestFlow'`. Ước lượng: 10.

### P13. Diễn tập các loại workflow mới [D]

- **Xung đột:** **phải chờ commit của công việc đang chạy** ở các gói `./internal/service`, `./internal/scheduler` (công việc wakeup) trước khi bắt đầu; không tự đoán nội dung thay đổi của họ (mục 0.3).
- **Tệp (mới):** `server/internal/handler/flow_rehearsal_graph_test.go` (dùng `testdata/flows/*.json` của P8, nhập qua đường tạo workflow schema 2 bằng fixture; chưa cần API bản nháp).
- **Test (trước):** `TestRehearsalGraph_TradeV2MatchesTradeV1Counts` (cùng số sub-issue như `flow_rehearsal_trade_test.go`), `TestRehearsalGraph_ResearchNestedLoopsAndVarMax`, `TestRehearsalGraph_MarketInstancesAndAfter`, `TestRehearsalGraph_LegalQuorumEarlyFailAndSpecificBackStep`, `TestRehearsalGraph_MisbehavingAgentMatrix` (E1–E8 trên cả bốn loại), `TestRehearsalGraph_CapReachedStalls`, `TestRehearsalGraph_PoolTooSmallStalls`, `TestRehearsalGraph_ReleaseWithoutCheckpointStalls`, `TestRehearsalGraph_RunningRequestKeepsPinnedVersion`, `TestRehearsalGraph_SweepRemindsHumanTaskThenStalls`. **[R2]** `TestRehearsalGraph_CrossReviewTwoAgents` (K19: A viết, B duyệt; B viết, A duyệt; chạy hết), `TestRehearsalGraph_PlannerTaskSetsListThenFanOut` (K18: task của agent đặt `sources`, bước sau mở đúng số thực thể, kẹp ở `max`), `TestRehearsalGraph_ReworkOnlyTheFailedBranch` (mẫu 12 của F.2), `TestRehearsalGraph_InputsFollowThePathTaken` (K17).
- **Kiểm:** [D] `./internal/handler -run 'TestRehearsalGraph_'`.
- **Gate G-E2:** [D] `go test -count=1 -p 1 ./internal/handler ./internal/service ./internal/scheduler ./internal/issueworkflow ./cmd/migrate ./cmd/multica ./internal/catalog ./internal/migrations ./internal/featureflags` xanh; [C] xanh; `git ls-files packages/core/catalog/generated server/internal/catalog/generated` chỉ `.gitkeep`. Người dùng xem biên bản diễn tập (như `kitviet-flow-rehearsal.md`) trước khi sang pha D. Ước lượng: 6,5.

## 4. Pha D: bản nháp và xuất bản

### P14. Migration 988–993, bản nháp, cờ, khả năng [D]

- **Xung đột:** **phải chờ commit của công việc đang chạy** ở `server/pkg/db/generated/*` ([Q]) và `flow.sql` trước khi bắt đầu; không tự đoán nội dung thay đổi của họ (mục 0.3).
- **Tệp (mới):** `server/migrations/988_issue_workflow_versions.{up,down}.sql`, `989_issue_workflow_lineage_version_idx.{up,down}.sql`, `990_issue_workflow_draft.{up,down}.sql`, `991_issue_workflow_draft_pkey_idx.{up,down}.sql`, `992_issue_workflow_draft_pkey.{up,down}.sql`, `993_issue_workflow_draft_squad_idx.{up,down}.sql` (nội dung ở spec 2.1; bảng bản nháp có thêm cột `origin`, `source_ref` của spec 2.2); `server/pkg/db/queries/flow_draft.sql`; `server/internal/handler/flow_draft.go`, `flow_draft_test.go`, `flow_capabilities.go`, `flow_capabilities_test.go`. **(đổi):** `server/cmd/migrate/main.go` (ba dòng vào `concurrentIndexCleanups`: `989…`, `991…`, `993…`), `server/pkg/db/queries/issue_workflow.sql` (`CreateIssueWorkflow` nhận `lineage_id`, `version`, `published_at`, `published_by`), `server/cmd/server/router.go` (nhóm `/flow-drafts` dưới `/api/squads/{id}` cạnh `:2477-2479`, `GET /api/flow/capabilities`), `server/internal/featureflags/keys.go` (`FlowAuthoring`, `FlowGraphV2`, mặc định tắt, cạnh `:98`), `server/pkg/protocol/events.go` (`flow_draft:updated`, cạnh `:148`). **(mới)** `server/internal/featureflags/keys_test.go`.
- **Test (trước):** `TestEveryConcurrentUpBuildHasCleanup` ([N], đỏ tới khi thêm ba dòng), `TestMigrationFilesHaveMatchingDirections`, `TestMigrationNumericPrefixesAreUnique`, `TestFlowDraftMigrations_UpDown` (mới, `server/cmd/migrate/migrate_flow_draft_test.go`: cột đúng kiểu, bảng, ba chỉ mục hợp lệ, khóa chính gắn vào `issue_workflow_draft_pkey_uidx`, down sạch, up hai lần); `TestFlowDraft_CRUD`, `TestFlowDraft_RevisionConflict409`, `TestFlowDraft_AgentActor403`, `TestFlowDraft_NotManager403`, `TestFlowDraft_FlagOff404`, `TestFlowDraft_DocTooLarge`, `TestFlowDraft_LimitPerSquad`, `TestFlowDraft_ImportManifestStrict`, `TestCapabilities_ReportsLimitsKindsAndFeatures`, `TestFlowAuthoringDefaultsOff` ([N]).
- **Kiểm:** [Q]; [V]; [GN] hai test tĩnh và `./internal/featureflags -run FlowAuthoring`; [D] `./cmd/migrate -run 'TestFlowDraftMigrations_UpDown'`, `./internal/handler -run 'TestFlowDraft_|TestCapabilities_'`; [M]. Ước lượng: 4.

### P15. validate, simulate, publish, khóa ghi, nhân bản [D]

- **Tệp (đổi):** `flow_draft.go` (validate trả `issues`, `analysis`, `fact_catalog`; simulate; publish theo spec mục 8.1, biên dịch vai→UUID, ghi `flow_config` version 2 và `steps` = danh sách trạng thái; `from.kind ∈ {blank, workflow, manifest}` với `workflow` hỗ trợ **mọi hàng** (schema 1 qua `Lift` với `opts.Runtime=false`, role `a_<8hex>`, nhãn lấy tên agent)), `issue_workflow.go` (hàm dùng chung `createFlowWorkflowRow`; `workflowLocked(row)` ở `:1036`, `:1587`; `CreateIssueWorkflow` `:782` gọi `ValidateFlowDoc`/`ValidateSchema1ForSquad` khi `flow_config` khác NULL; `IssueWorkflowResponse` `:98-105` thêm `lineage_id`, `version`, `published_at`, `published_by`; `ProjectLegacySteps`), `squad_workflow.go` (`ListSquadWorkflows` trả cột mới).
- **Test (trước):** `TestPublish_CreatesLockedVersion`, `TestPublish_SecondVersionLeavesFirstUntouched`, `TestPublish_RunningRequestKeepsPinnedVersion`, `TestPublish_ReturnsAllIssues`, `TestPublish_Forbidden` (ba ca D2), `TestPublish_NewStatusNeedsAdmin`, `TestPublish_ReleaseWarningNeedsAdminAck` (D3), `TestPublish_NoGateWarningNeedsAck`, `TestPublish_StaleBase409`, `TestPublish_NameTaken409`, `TestPublish_DoesNotInheritFlowAuto` (D9), `TestPublish_MakeActiveThenSetSquadActive`, `TestUpdateIssueWorkflow_PublishedLocked409`, `TestSetHandoff_PublishedLocked409`, `TestCreateIssueWorkflow_SchemaOneWithLeaderTask422`, `TestFork_FromSchemaOneRowGivesCleanV2Draft`, `TestFork_ForkNeverReceivesKitVietUpgrade` (D8; `upgrade.ts:251-252`), `TestLegacyProjection_OldGraphSheetShapeUnchanged` (K11), `TestWorkflowGolden_StillGreen`. **[R2]** route `POST /api/flow/validate` (kiểm không trạng thái; `flow_draft.go`, `router.go`): `TestStatelessValidate_MemberAndAgentAllowed`, `TestStatelessValidate_WritesNothing`, `TestStatelessValidate_SquadRosterOnlyWhenVisible`, `TestStatelessValidate_RateLimited`. **[R2]** chọn workflow và chạy thử (spec 16.5; `flow_api.go` `StartIssueFlow` nhận `workflow_id`, `trial`; `issue.go` tạo issue nhận `flow_workflow_id`; `service/flow_start.go`): `TestStart_ExplicitVersionOfSameSquad`, `TestStart_OtherSquadOrNonFlowRejected`, `TestCreateIssue_FlowWorkflowIDIgnoredForAgentActor`, `TestTrial_PinsVersionAndForbidsUnattendedRelease` (N8, N9).
- **Kiểm:** [D] `./internal/handler -run 'TestPublish_|TestUpdateIssueWorkflow_|TestSetHandoff_|TestCreateIssueWorkflow_|TestFork_|TestLegacyProjection_|Golden'`; [W] `core exec vitest run squads/upgrade-flow`.
- **Gate G-B:** toàn bộ [D] của G-E2 xanh lại, cộng P14–P15; người dùng xem biên bản. Ước lượng: 7.

## 5. Pha E: client và giao diện

### P16. Kiểu, zod, client, truy vấn, bố cục [N]

- **Tệp (mới):** `packages/core/types/flow-draft.ts`, `packages/core/flow-drafts/{queries.ts,mutations.ts,index.ts}` (+ test), `packages/core/squads/flow-layout.ts` (+ `flow-layout.test.ts`: `layoutFlowGraph` thuần). **(đổi):** `packages/core/types/index.ts`, `packages/core/types/issue-workflow.ts`, `packages/core/types/issue-flow.ts`, `packages/core/types/events.ts`, `packages/core/api/schemas.ts`, `packages/core/api/client.ts`, `packages/core/api/schemas.test.ts`. Mọi phản hồi qua `parseWithFallback`, có mặc định; enum lạ có nhánh dự phòng. **[R2]** kiểu và zod cho trường mới của trạng thái request (`trail`, `unreviewed`, `trial`, và `inputs`/`feedback`/`sets` của task), của bản nháp (`origin`, `source_ref`), và client cho `POST /api/flow/validate`, `PUT /api/issues/{taskId}/flow/vars/{key}`, `flow_workflow_id` khi tạo issue.
- **Test (trước):** phản hồi sai (K12): thiếu `doc`, `issues` null, `severity` lạ → `"warning"`, `stop` kiểu đối tượng lẫn số, `capabilities` thiếu khóa, loại bước lạ giữ chuỗi; mutation chờ máy chủ rồi vô hiệu hóa (không cập nhật lạc quan); khóa truy vấn có `wsId`; `layoutFlowGraph`: tầng theo đường dài nhất sau khi bỏ cạnh quay lại, cạnh quay lại không đổi tầng, 40 bước không chồng nút, ổn định khi đổi thứ tự khai báo.
- **Kiểm:** [B]; [W] `core exec vitest run flow-drafts squads/flow-layout api/schemas`; [W] `core exec tsc --noEmit`. Ước lượng: 4.

### P17. Khung trang soạn, tự lưu, xung đột [N]

- **Tệp (mới):** `packages/views/squads/flow-designer/{flow-draft-editor-page.tsx,use-draft-buffer.ts,outline.tsx,draft-status.tsx,index.ts}` (+ test), `apps/web/app/[workspaceSlug]/(dashboard)/squads/[id]/workflows/[draftId]/page.tsx`, `apps/desktop/src/renderer/src/pages/flow-draft-editor-page.tsx`, một dòng route `squads/:id/workflows/:draftId` cạnh `apps/desktop/src/renderer/src/routes.tsx:287`. **(đổi):** `packages/views/squads/components/index.ts` hoặc `packages/views/squads/index.ts` (xuất trang), `packages/views/package.json` nếu cần xuất mới.
- **Test (trước):** `flow-draft-editor-page.test.tsx`: lỗi tải có Retry; 404 về squad; tự lưu 1,5 giây kèm `revision`; xung đột hiện Reload / Keep mine as a copy; mất mạng giữ bộ đệm; không điều hướng khi chưa lưu mà không hỏi; cờ tắt ẩn trang; giá trị lạ từ `capabilities` hiện "unsupported".
- **Kiểm:** [W] `views exec vitest run squads/flow-designer`; `views exec tsc --noEmit`. Ước lượng: 3.

### P18. Vai, biến, verdict, vòng, dừng, loại bước, task, cạnh [N]

- **Tệp (mới):** `packages/views/squads/flow-designer/{roles-panel,vars-panel,verdicts-panel,loops-panel,stops-panel,terminals-panel,step-inspector,task-card,edge-list,step-templates}.tsx` + test.
- **Test (trước):** mẫu bước Gate sinh đúng bốn cạnh chuẩn và đổi N hợp lệ; cạnh quay lại chỉ chọn `loop` có `header` bằng đích; thêm vòng với `max` từ biến chỉ chọn biến `int` có `max`; mẫu Quorum review sinh `join` quorum và bộ verdict `review`; mẫu Human approval sinh bước `manual` và `notice`; vai chỉ liệt kê thành viên squad; vai không dùng hiện nút xóa; đạt trần (40 bước, 12 task, 16 vai…) thì nút thêm vô hiệu có lý do, số lấy từ `capabilities`; kéo đổi thứ tự cạnh đổi thứ tự lưu; cạnh không khớp mẫu hiện "Custom routing"; hiển thị cảnh báo `release_without_human` và chỉ admin tick được ô xác nhận. **[R2]** Inspector của task (spec 11.9): "Inputs" chỉ liệt kê task ở bước khác có đường xuôi tới bước này, chọn được khe tệp; "Skills" chỉ liệt kê skill mà vai đang có và hiện lỗi R09 tại chỗ; "Sets variables" chỉ liệt kê biến mà vai được đặt; tick "Must set before closing" ghi `requires.vars`; bảng biến cho chọn vai khai báo ở "Set by".
- **Kiểm:** [W] `views exec vitest run squads/flow-designer`. Ước lượng: 10.

### P19. Đồ thị, Problems, Simulate, Publish, màn hình request [N]

- **Xung đột:** **phải chờ commit của công việc đang chạy** ở `packages/views/workflow/issue-flow-state-panel.tsx` (+ test) trước khi bắt đầu; không tự đoán nội dung thay đổi của họ (mục 0.3).
- **Tệp (mới):** `packages/views/squads/flow-designer/{flow-graph.tsx,problems-panel.tsx,simulate-dialog.tsx,publish-dialog.tsx}` + test. **(đổi, chung tệp nóng):** `packages/views/workflow/issue-flow-state-panel.tsx` (+ `.test.tsx`): nút verdict theo bộ verdict của task và chỉ khi `viewer.closable_task_ids` chứa task; nút cạnh `manual`; "Close on behalf"; huy hiệu "đóng thay"; vòng `↺ n/max`; mỗi thực thể một hàng; chỉ thêm theo trường mới, giữ test hiện có.
- **Test (trước):** Problems nhóm theo bước, bấm chọn đúng `path`; lỗi chặn Publish; cảnh báo cần `ack`; Simulate gửi đúng `script` và hiện bộ đếm vòng; Publish chỉ điều hướng khi 201; `flow_draft_stale` hiện Rebase; đồ thị 40 bước dựng được, cạnh quay lại nét đứt có nhãn, nút "Find a step" chọn đúng nút, dưới 640px thay bằng danh sách dọc; panel request: người không có quyền không thấy nút verdict (so với hành vi hiện tại hiện cho mọi người), người có quyền thấy bộ verdict đúng của task. **[R2]** panel request có mục "Path" (`trail`; mở một dòng tải dữ kiện của `flow_transition`), mỗi task hiện Inputs, Feedback và biến đã đặt; `trail` thiếu hoặc sai kiểu trong phản hồi không làm vỡ panel.
- **Kiểm:** [W] `views exec vitest run squads/flow-designer workflow/issue-flow-state-panel`. Ước lượng: 8,5.

### P20. Điểm vào, bộ xây điều kiện theo sổ dữ kiện, 7 ngôn ngữ [N]

- **Xung đột:** **phải chờ commit của công việc đang chạy** ở `packages/views/workflow/designer/condition-builder.tsx` trước khi bắt đầu; không tự đoán nội dung thay đổi của họ (mục 0.3).
- **Tệp (đổi):** `packages/views/squads/components/squad-workflows-section.tsx` (nút New workflow, nhóm theo lineage, Edit as draft, Export, trạng thái rỗng), `workflow-graph-sheet.tsx` (hiện `version`, nhãn khóa; đọc `steps[]` chiếu), `packages/views/workflow/manage/workflow-detail-drawer.tsx` ("Edit as draft" cho hàng đã xuất bản), `packages/views/workflow/condition-spec.ts` (hàm nhận `FactSpec[]` tùy chọn; mặc định giữ hằng hiện tại, `:20-50`), **(tệp nóng)** `packages/views/workflow/designer/condition-builder.tsx` (nhận `facts?: FactSpec[]`; không đổi hành vi mặc định), `packages/views/locales/{en,zh-Hans,zh-Hant,fr,es,ko,ja}/squads.json` (khối `flow_designer` theo spec 11.7.1 và 11.7.2, cộng `problems.<code>` và `errors.<code>`). **(mới)** `packages/views/squads/flow-designer/{start-dialog.tsx,import-dialog.tsx}`.
- **Test (trước):** `condition-spec.test.ts` hiện có xanh không sửa, thêm ca có `FactSpec[]` (int, list, string); `condition-builder.test.tsx` hiện có xanh; `squad-workflows-section.test.tsx` thêm ca (nút ẩn khi không quản lý hoặc cờ tắt; nhóm lineage; rỗng có ba lối vào; nhập tệp sai hiện `flow_manifest_invalid`; nhân bản hàng schema 1 gọi `from.kind=workflow`); `parity.test.ts` (đã có) xanh; không chuỗi nào chứa tiếng Việt ngoài nội dung tác giả. **[R2] (P20b)** khóa của spec 11.9 thuộc R2 (`task.inputs`, `task.skills`, `task.sets`, `task.requires_vars`, `request.path`, `request.feedback`, `trial.*`) đủ 7 locale; form tạo issue giao cho squad có bộ chọn workflow khi squad có hơn một lineage flow (`packages/views/issues/`: kiểm `git status` của tệp trước khi sửa, mục 0.3); trang phiên bản có nút "Trial run".
- **Kiểm:** [W] `views exec vitest run squads workflow locales issues`; [T] `pnpm lint`. Ước lượng: 5.

### P21. Kiểm tra điện thoại và trình duyệt [thủ công]

- Cần máy chủ và DB; **chỉ chạy khi người dùng cho phép** (không `make up/dev`): backend với `DATABASE_URL` tường minh sau guard, `next dev` cổng riêng, Chromium của Playwright, cách đã dùng ở diễn tập web `docs/engineering/kitviet-flow-web-drill.md` mục 1 (tệp chưa theo dõi lúc rà soát; nếu chưa có, lập lại: backend cổng riêng với DB `_test`, `next dev` cổng riêng, runtime giả chỉ gửi heartbeat). Mở trang soạn ở 1280px và 390px (K13): không cuộn ngang; Inspector bằng Sheet; dựng flow nghiên cứu mẫu 14 bước; Check, Simulate, Publish, Make active; chụp ảnh đính kèm PR. Không agent thật. Biên bản `docs/engineering/flow-authoring-drill.md` (mới). Ước lượng: 1,5.

### P22. Tài liệu [N]

- `docs/engineering/flow-authoring-runbook.md` (mới: quyền, cờ, quy trình xuất bản, quay lui, đọc `issues[]`). **Không sửa** `kitviet-workflow-spec.md` và `kitviet-workflow-plan.md` (tài liệu lịch sử; spec mới chỉ có một dòng trỏ tới chúng). Cập nhật `kitviet-runbook.md` mục 8f để trỏ tới runbook mới. `AGENTS.md` chỉ nếu đổi ranh giới (không đổi). `git diff --check`; báo "chưa chạy test mã" cho commit chỉ-tài liệu. Ước lượng: 1,5.

**Gate G-C:** [T] `pnpm typecheck`, `pnpm lint`, `pnpm test` xanh; [C] xanh; toàn bộ [D] của G-B xanh lại; parity xanh; ảnh chụp 390px/1280px; `git ls-files` không có mã sinh. Người dùng duyệt trước khi bật `FF_FLOW_AUTHORING` và `FF_FLOW_GRAPH_V2` trên workspace thật. G-C là điều kiện bắt đầu pha F. Bật `flow_auto` cho flow người dùng là việc riêng của admin, sau một lượt diễn tập bằng agent giả trên workspace đó.

## 6. Pha F: từ bộ skill/agent tới workflow (R3; spec mục 16, D14, D15, D22)

Mục tiêu của pha: người dùng đưa một bộ skill/agent **nguyên trạng** lên, có squad, có bản nháp (tự soạn hoặc do agent đề xuất từ quy trình markdown), chạy thử, xuất bản, và đọc số liệu để sửa. Mọi route của pha sau cờ `FF_FLOW_PACKS` (mặc định tắt) trừ khi nêu khác.

### P23. Gói `pack` thuần: nhận dạng, chuẩn hóa, kiểm [N]

- **Tệp (mới):** `server/internal/pack/{pack.go,detect.go,normalize.go,validate.go}` (kiểu `Pack{ID, Name, Version, Format, Agents, Skills, Leader, Flow, References, Ignored}`; `Detect` theo thứ tự cố định của spec 16.1; `Normalize` cho từng định dạng; `Validate` trả `[]Issue` cùng dạng với bộ kiểm flow), `*_test.go`, `testdata/{skills_only,claude_agents,multica_pack,native_bundle}/` và `testdata/broken/*`. Đầu vào là `*hub.Bundle` đã giải nén (`server/internal/hub/archive.go`); gói không import handler hay DB. **Không sửa** `hub.Detect`.
- **Dữ liệu thật cho test:** `claude_agents` sao từ một ngành của `catalog/_sources/kitviet/upstream/` (ví dụ `ban-hang-online/`: `agents/*.md` có frontmatter `name`, `description`, `tools`, `model`; `skills/*/SKILL.md`), giữ nguyên tệp và giấy phép (`upstream/LICENSE`, `NOTICE.md`). Đây là bộ ngoài đời đúng định dạng 3 và có quy trình viết bằng markdown (skill `day-chuyen-*`).
- **Test (trước):** `TestDetect_OrderAndAmbiguity` (bốn định dạng; hai dấu hiệu ở gốc là `pack_invalid`), `TestNormalize_ClaudeAgents` (mỗi tệp một agent; thân tệp là hướng dẫn; `tools`/`model` vào `Ignored`; có `skills:` thì chỉ gắn các skill đó), `TestNormalize_SkillsOnlyHasNoAgents`, `TestNormalize_MulticaPack`, `TestNormalize_NativeBundleKeepsLeaderAndFlow`, `TestNormalize_AgentKeyFromName` (chuẩn hóa khóa; trùng khóa là lỗi), `TestValidate_BrokenPacks` (bảng: thiếu `name`/`description`, skill được nhắc không có, đường dẫn thoát thư mục, vượt `hub.DefaultLimits`, `leader` trỏ agent không có), `TestReferences_ListedNotParsed` (tài liệu tham khảo chỉ là đường dẫn và cỡ; gói không đọc nội dung để suy vai hay bước: kiểm bằng cách đổi thân tệp và so `Pack` không đổi ngoài `instructions`), `TestUnknownFrontmatterKeysIgnored`.
- **Kiểm:** [GN] với gói `./internal/pack/...`; [V]. Ước lượng: 4.

### P24. Migration 994–997, `preview`, `commit` [D]

- **Tệp (mới):** `server/migrations/994_squad_pack_install.{up,down}.sql`, `995_squad_pack_install_pkey_idx.{up,down}.sql`, `996_squad_pack_install_pkey.{up,down}.sql`, `997_squad_pack_install_squad_idx.{up,down}.sql` (spec 16.3; kiểm lại số cuối bằng `ls server/migrations | tail -3` trước khi đặt tên); `server/pkg/db/queries/squad_pack.sql`; `server/internal/handler/squad_pack.go`, `squad_pack_test.go`; `catalog/agents/flow_lead/` (mẫu hướng dẫn của lead chung, đủ i18n theo `catalog/README.md`). **(đổi):** `server/cmd/migrate/main.go` (hai dòng `995…`, `997…` vào `concurrentIndexCleanups`), `server/cmd/server/router.go` (`POST /api/squads/packs/preview`, `/commit`), `server/internal/featureflags/keys.go` (`FlowPacks`, mặc định tắt), `flow_capabilities.go` (`features.packs`).
- **Quy tắc:** `preview` không ghi DB. `commit` là một giao dịch, dùng lại hàm nội bộ của route tạo skill (`createHubSkill`, `hub_import.go:925`), tạo agent, tạo squad; **đọc phép kiểm quyền của ba route đó trước khi viết và gọi đúng hàm ấy** (spec 14.2); nếu chúng chưa có dạng hàm nội bộ nhận `qtx` thì tách ra trong một commit refactor riêng, test cũ của chúng xanh không sửa. Không ghi đè agent hay skill đang có. Bí mật, `tools`, `model`, MCP của bộ không được áp.
- **Test (trước):** `TestPackPreview_FourFormats` (N1), `TestPackPreview_WritesNothing` (N2), `TestPackCommit_CreatesSquadAgentsSkillsAtomically` (N1, N2: ép lỗi ở agent thứ hai, không còn hàng nào của fixture), `TestPackCommit_BrokenPacksRejected` (N3), `TestPackCommit_ScanBlockRejected_WarnNeedsAck`, `TestPackCommit_SameDigestIsNoop_OtherDigestConflicts` (N4), `TestPackCommit_SkillsOnly_NoAgentsNoSquad` và `…_ExtraAgentsAndGenericLead` (N5), `TestPackCommit_NativeFlowBecomesUnboundDraft` (N6: ghép theo khóa bó khai, không theo tên), `TestPackCommit_UseExistingAgentNeedsWirePermission` (`memberCanWireAgent`), `TestPack_AgentActor403`, `TestPack_FlagOff404`, `TestSquadPackMigrations_UpDown`, `TestEveryConcurrentUpBuildHasCleanup`.
- **Kiểm:** [Q]; [V]; [C]; [D] `./cmd/migrate -run 'SquadPackMigrations|TestEveryConcurrentUpBuildHasCleanup'`, `./internal/handler -run 'TestPack'`; [M]. Ước lượng: 6.

### P25. Đề xuất bản nháp từ quy trình markdown [N] + [D]

- **Tệp (mới):** `catalog/skills/builtin/flow-authoring/` (`SKILL.md`: tham chiếu schema 2, cách đọc quy trình văn xuôi thành bước/vai/cổng/vòng, danh sách điều máy chủ từ chối, lệnh kiểm; `references/patterns/*.json` sao từ `testdata/flows/patterns/` của P8; `template.json`), `catalog/agents/flow_designer/` (mẫu agent soạn), `server/cmd/multica/cmd_flow.go` (`multica flow validate --file <path> [--squad <id>]`, `multica flow capabilities`; in `issues[]` theo `path`, mã thoát khác 0 khi có lỗi) + test. **(đổi):** `flow_draft.go` (`from.kind = proposal`: tạo bản nháp rỗng `origin: proposal` và issue thường giao cho agent soạn với mô tả do máy chủ sinh; `POST /flow-drafts/{draftId}/import-proposal`; xuất bản đòi ack `proposal_reviewed`), `router.go`, `flow_capabilities.go` (`features.proposals`).
- **Quy tắc:** agent chỉ gọi được `POST /api/flow/validate` (P15). Mô tả issue đề xuất không chứa UUID agent ngoài squad và không chứa bí mật. Tệp nhập là tệp đính kèm mới nhất có tên kết thúc `flow.json` **do agent soạn tải lên chính issue đó**; `bind` trong tệp bị bỏ.
- **Test (trước):** `TestProposal_CreatesDraftAndPlainIssue` (issue không có `flow`), `TestProposal_AgentCanValidateButNotWriteDrafts` (N7), `TestProposal_ImportReadsLatestAgentAttachment`, `TestProposal_ImportIgnoresFilesFromOthers`, `TestProposal_ImportStripsBind`, `TestProposal_InvalidManifestKeepsDraftUntouched`, `TestProposal_MissingFile404`, `TestPublish_ProposalNeedsReviewedAck`, `TestProposalBrief_ListsRosterSkillsAndOutputContract`; CLI [N]: `TestFlowValidateCommand_PrintsIssuesAndExitCode`; catalog: [C] xanh, `TestFlowAuthoringSkill_PatternFilesValidate` (mọi tệp mẫu trong skill qua `ValidateFlowDoc`, để skill không dạy tài liệu sai).
- **Kiểm:** [C]; [GN] `./cmd/multica -run 'FlowValidate'`, `./internal/issueworkflow/... -run 'FlowAuthoringSkill'`; [D] `./internal/handler -run 'TestProposal|TestPublish_Proposal'`. Ước lượng: 5.

### P26. Số liệu vận hành theo phiên bản [N] + [D]

- **Tệp (mới):** `server/internal/issueworkflow/insights.go` (`Insights(states []State, closes []ManualClose) Report`, thuần) + test; `server/internal/handler/flow_insights.go` + test; truy vấn chỉ đọc trong `flow.sql` (500 request gần nhất theo `flow->>'workflow_id'`, lọc `trial`). **Xung đột:** `flow.sql` và `server/pkg/db/generated/*` theo mục 0.3.
- **Test (trước):** `TestInsights_EndsEdgesLoopsDurations` (bảng từ `trail` dựng tay), `TestInsights_IgnoresMalformedTrail`, [D] `TestFlowInsights_OnlyThisVersion_TrialExcludedByDefault` (N10), `TestFlowInsights_NotManager403`, `TestFlowInsights_Truncated`.
- **Kiểm:** [Q]; [GN] `./internal/issueworkflow/... -run 'Insights'`; [D] `./internal/handler -run 'TestFlowInsights'`. Ước lượng: 3.

### P27. Giao diện R3: nhập bộ, đề xuất, ô tham khảo, số liệu [N]

- **Tệp (mới):** `packages/core/packs/{queries.ts,mutations.ts,index.ts}` + test, `packages/core/types/pack.ts`, `packages/views/squads/pack-import/{pack-import-dialog,pack-preview-agents,pack-preview-skills,pack-leader-picker,pack-result}.tsx` + test, `packages/views/squads/flow-designer/{proposal-banner,reference-panel,insights-badges}.tsx` + test. **(đổi):** `packages/core/api/{client.ts,schemas.ts,schemas.test.ts}`, `packages/views/squads/flow-designer/start-dialog.tsx` (lối "Ask an agent to draft"), `publish-dialog.tsx` (ack `proposal_reviewed`), `flow-graph.tsx` (thẻ số liệu), trang danh sách squad (nút "Import a pack"; kiểm `git status` trước khi sửa), `packages/views/locales/*/squads.json` (khối `packs` và các khóa R3 của spec 11.9, đủ 7 locale). Nối vào cả web và desktop như P17.
- **Test (trước):** preview hiện đúng bảng agent/skill và mục "Ignored settings"; bộ chỉ có skill hiện "Add agent" và "One agent per skill"; không chọn lead thì nút cài khóa có lý do; phát hiện `block` khóa cài, `warn` cần tick; `pack_conflict` hiện lựa chọn; cài xong có lối "Design the workflow"; cờ tắt ẩn nút; bản nháp `origin: proposal` hiện dải chờ, rồi "Import proposal" khi có tệp; ô tham khảo chỉ đọc và không nhận nội dung làm HTML; thẻ số liệu ẩn khi không có dữ liệu; phản hồi sai của `PackPreview` và `flow-insights` không làm vỡ giao diện (zod); parity 7 locale.
- **Kiểm:** [B]; [W] `core exec vitest run packs api/schemas`; [W] `views exec vitest run squads locales`; [T] `pnpm lint`, `pnpm typecheck`. PR đính kèm ảnh chụp 1280px và 390px. Ước lượng: 7.

### P28. Diễn tập đầu-cuối và tài liệu [D] + [thủ công]

- **Tệp (mới):** `server/internal/handler/flow_rehearsal_pack_test.go`, `docs/engineering/flow-pack-drill.md` (biên bản). **(đổi):** `docs/engineering/flow-authoring-runbook.md` (mục "Từ một bộ skill/agent tới workflow đang chạy": nhập, chọn lead, soạn hoặc đề xuất, kiểm, mô phỏng, chạy thử, xuất bản, đọc số liệu, sửa), `apps/docs/content/docs/workflows*.mdx` nếu trang tài liệu người dùng mô tả workflow của squad (đủ các ngôn ngữ của trang đó).
- **Test (trước), mỗi cái đi trọn đường bằng agent giả:**
  - `TestRehearsalPack_KitVietUpstreamRaw`: nhập thư mục ngành của Kit Việt upstream nguyên trạng (định dạng 3) → squad với lead chung → agent giả đính một `flow.json` soạn sẵn (dịch tay từ skill `day-chuyen-*` của bộ) → `import-proposal` → `validate` không lỗi → `simulate` → xuất bản với ack → chạy thử → request kết thúc ở terminal → `flow-insights` có một request.
  - `TestRehearsalPack_SkillsOnly`: bộ chỉ có skill → ba agent khai trong lúc nhập → bản nháp trắng dựng bằng API (hai bước làm việc, một cổng duyệt chéo, một bước người) dùng `Task.skills` → xuất bản → chạy hết.
  - `TestRehearsalPack_NativeBundleWithFlow`: bó gốc → bản nháp từ `flow.json` → ghép vai → xuất bản → chạy với một lần cổng từ chối rồi qua.
  - `TestRehearsalPack_TwoWorkflowsOneSquad`: hai lineage của một squad; request tạo với `flow_workflow_id` chạy đúng flow đã chọn.
- **Thủ công (chỉ khi người dùng cho phép, như P21):** mở trình nhập và trình soạn trên trình duyệt với bộ Kit Việt upstream; chụp ảnh. **Thử đề xuất bằng agent thật** chỉ khi người dùng cho phép rõ ràng (AGENTS.md "real-agent smoke tests"); kết quả ghi vào biên bản là quan sát, không phải tiêu chí chấp nhận.
- **Kiểm:** [D] `./internal/handler -run 'TestRehearsalPack_'`; `git diff --check`. Ước lượng: 3,5.

**Gate G-F:** toàn bộ [D] của G-C xanh lại cộng P23–P28; [T] và [C] xanh; N1 đến N10 của spec 12.3 có test tên tương ứng xanh; biên bản `flow-pack-drill.md` nêu với từng bộ: số agent, số skill, số bước của flow, kết cục chạy thử. Người dùng duyệt trước khi bật `FF_FLOW_PACKS` trên workspace thật.

## 7. Thứ tự và phụ thuộc

```text
P0 ─► P1 ─► P2 ─► P3 ─► P4 ─► [G-A]
P5 ─► P6 ─► P7 ─► [G-L] ─► P8 ─► P9          (P5–P9 độc lập với P1–P4, chạy song song được; P11 cần cả G-A và G-L)
[G-A]+[G-L]+P9 ─► P10 ─► P11 ─► [G-E1] ─► P12 ─► P13 ─► [G-E2]
[G-E2] ─► P14 ─► P15 ─► [G-B] ─► P16 ─► P17 ─► P18 ─► P19 ─► P20 ─► P21 ─► P22 ─► [G-C]
[G-C] ─► P23 ─► P24 ─► P25 ─► P26 ─► P27 ─► P28 ─► [G-F]   (P23 và P26 chỉ cần G-E2 và P15 nên làm song song với pha E được; P24, P25 cần P15; P27 cần P20)
```

Tổng ước lượng: P0 0,5 + P1 1,5 + P2 2 + P3 2 + P4 1 + P5 2 + P6 5 + P7 5,5 + P8 8 + P9 6,5 + P10 1 + P11 9,5 + P12 10 + P13 6,5 + P14 4 + P15 7 + P16 4 + P17 3 + P18 10 + P19 8,5 + P20 5 + P21 1,5 + P22 1,5 + P23 4 + P24 6 + P25 5 + P26 3 + P27 7 + P28 3,5 = **134 ngày công** (ước lượng, chưa đo). Bản trước là 88; phần thêm gồm 17,5 ngày cho các sửa của rà soát lần 2 trong P6 đến P20 và 28,5 ngày cho pha F. Ba phát hành (D13): R1 = P0–P13 (61 ngày: engine tổng quát, vá lỗ hổng, bằng chứng, chưa có giao diện soạn); R2 = P14–P22 (44,5 ngày: bản nháp, xuất bản, trình soạn); R3 = P23–P28 (28,5 ngày: nhập bộ, đề xuất, chạy thử, số liệu). P5–P9 song song với P1–P4, và P23, P26 song song với pha E, rút thời gian thực tế nếu có hai người.

## 8. Rủi ro khi thực hiện

| Rủi ro | Bước | Cách giảm |
| --- | --- | --- |
| Một trong 21 flow lệch ở bài vi phân | P7 | G-L dừng hẳn; không sửa oracle; điều tra nguyên nhân (`Lift` hay oracle chép sai: P5 đã có `TestOracleMatchesProductionDecide`) |
| Văn bản/bình luận/mô tả của flow đang chạy đổi | P7, P11 | `.golden` chụp trước ở P5 |
| Thay bộ diễn giải gây hồi quy chỉ lộ ở DB | P11 | Chạy nguyên bộ test DB cũ không sửa; hàng workflow và `issue.flow` không bị ghi lại sai (`TestEngine_V1RowIsNeverRewritten`) |
| Vá H1–H4 đổi hành vi flow Kit Việt đang chạy | P2–P4 | P1 xác nhận trước; G-A liệt kê thay đổi; D5 |
| CLI cũ trên daemon từ chối verdict mới ở máy khách | P12 | Đổi CLI; cảnh báo `cli_version` khi xuất bản; cập nhật daemon (`/api/runtimes/{id}/update`); nguồn phiên bản (`cli_version` trong metadata runtime, `handler/daemon.go:779`) đã đọc nhưng giá trị tối thiểu chưa kiểm chứng |
| Va chạm với agent diễn tập web ở `flow_guards.go`, `issue.go`, `flow_state.go`, panel | P2, P3, P11, P12, P19 | 0.3: `git status`/`git log` ngay trước, hỏi khi có sửa chưa commit |
| Giao task cho người chưa được kiểm | P12 | Test `TestMemberTask_*` chạy sớm; đọc nhánh `AssigneeType: member` của `IssueService.Create` trước khi viết |
| Trần chi phí (300 issue/request) quá chặt hay quá lỏng | P9, P13 | Số do máy chủ trả qua `capabilities`; đổi không cần phiên bản mới của client |
| Kích thước thay đổi lớn | cả kế hoạch | R1/R2/R3; mỗi cổng có biên bản; không bước E trước G-E2, không bước F trước G-C |
| Bộ diễn giải mới đúng trên trạng thái tổng hợp nhưng lệch trên trạng thái thật của DB | P11 | Ba lát; điểm móc oracle bật cho mọi test DB của gói handler (D24) |
| Luật mới (Y01 theo `unreviewed`, R07 theo đầu vào) làm flow Kit Việt đang chạy đổi hành vi | P7, P11 | Đồ thị `compat: "v1"` miễn các luật mới (spec 9.2 điều 12), có test; chỉ bản nhân bản mới chịu |
| Biến do task đặt bị dùng để lái định tuyến sai | P12 | Chỉ biến đã khai, có kiểu và miền, trong `Task.sets`, khi task mở; trần vòng và số thực thể vẫn kẹp ở `cap`/`max`; điểm kiểm người không phụ thuộc biến; mọi lần đặt có nhật ký |
| Bộ nhập chứa lời dụ agent soạn quy trình xấu, hoặc tệp nguy hiểm | P24, P25 | `hub.Scan` và `hub.DefaultLimits`; máy chủ không phân tích văn bản của bộ; sản phẩm của agent chỉ là tệp, phải qua bộ kiểm, ack `proposal_reviewed` và người xuất bản; agent không ghi được bản nháp |
| Bản nháp do agent đề xuất đúng cấu trúc nhưng quy trình dở | P25, P28 | Skill `flow-authoring` có mẫu đã kiểm; mô phỏng và chạy thử trước khi dùng chính thức; số liệu theo phiên bản để sửa |
| `commit` của bộ cần hàm nội bộ mà route tạo agent/skill/squad chưa tách | P24 | Refactor tách hàm trong commit riêng, test cũ của các route đó xanh không sửa; nếu tách quá rộng thì dừng và báo |
| Truy vấn số liệu chậm trên workspace nhiều request | P26 | Giới hạn 500 request; nếu đo thấy chậm thì thêm chỉ mục biểu thức ở migration riêng (`CONCURRENTLY`), chưa đưa vào kế hoạch |
