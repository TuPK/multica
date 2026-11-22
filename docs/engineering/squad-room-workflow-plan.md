# Plan triển khai: workflow theo phòng, bỏ workflow cấp dự án

Căn cứ: [squad-room-workflow-spec.md](squad-room-workflow-spec.md). Mọi quyết định O-1 đến O-6 đang dùng giá trị đề xuất trong spec; nếu người dùng đổi, cập nhật spec trước khi làm bước liên quan.

Quy ước: mỗi bước có **Việc**, **File**, **Kiểm chứng**. Không chuyển bước khi kiểm chứng chưa pass. Không commit cho đến khi người dùng yêu cầu. Theo `AGENTS.md`: đổi giao diện phải có ảnh chụp; migration số = cao nhất + 1 sau khi rebase `origin/main`.

## Điểm dừng cần người dùng

- **S1 (trước P2):** xin quyền tạo DB test riêng (`*_test`). Không dùng DB `multica`.
- **S2 (trước P4):** xác nhận xóa cột `project.workflow_id` trên DB live. Thao tác không hoàn tác được về dữ liệu.
- **S3 (trước P5):** xác nhận bật cờ `room_workflow` trên workspace nào.

## P0. Kiểm tra dữ liệu và đọc mã (không đổi code)

Đã xác minh trong rà soát trước:
- `applyFlowStatusRules` ([server/internal/handler/flow_guards.go:654](../../server/internal/handler/flow_guards.go:654)) trả `handled=true` ngay khi `prev.FlowTask.Valid`. Vậy status của engine task không đi qua resolver khi thay đổi status.
- Có 8 file gọi `issueworkflow.ForIssue`.

Cần làm:
1. Liệt kê mọi lời gọi `ForIssue` (không tính các hàm có chữ `ForIssue` trong tên) và kết quả nó quyết định. Đã có danh sách sơ bộ trong spec, mục R6.
2. Đọc các đường đọc `issue_workflow.squad_id` (migration 973): `sdlcWorkflowFor`, `planningWorkflowFor`, `issue_workflow.go` (dòng 904), `initiative_build.go`, `squad_planning_workflow.go`. Ghi lại đường nào còn phụ thuộc.
3. Liệt kê đường tạo issue tự động: `isSecretaryFiling`, `squad_planning_workflow.go`, `initiative_build.go`, `autopilot.go` (`EnqueueTaskForIssue`), `service/issue.go` tạo sub-issue.
4. Kiểm tra `pipeline` (`pkg/db/queries/pipeline.sql`) và `org_map.sql:65` có phụ thuộc `project.workflow_id` không.
5. Truy vấn dữ liệu live (chỉ đọc, qua CLI hoặc API):
   - issue giao cho squad và cho agent có `home_squad_id`, nhóm theo phòng có/không workflow, có/không flow;
   - run `queued`/`dispatched` có `workflow_step`.
6. Đọc `applyIssueWorkflow` ([server/internal/handler/issue_workflow_entry.go:50](../../server/internal/handler/issue_workflow_entry.go:50)) và các test hiện có của nó.

**Kiểm chứng:** biên bản P0 ghi đủ 6 mục; không còn đường tạo issue nào chưa xác định. Nếu bước 5 cho thấy có issue hoặc run bị ảnh hưởng, dừng và báo người dùng trước khi làm tiếp.

## P1. Resolver và kiểm tra khi ghi, sau cờ `room_workflow`

### P1.1 Thêm cờ
- **File:** `server/internal/featureflags/keys.go` — thêm `RoomWorkflow = "room_workflow"`, mặc định tắt, theo mẫu `FlowAuthoring`.
- **Kiểm chứng:** test mặc định tắt (theo mẫu `TestFlowAuthoringDefaultsOff`).

### P1.2 Resolver mới
- **File:** `server/internal/issueworkflow/issueworkflow.go`.
- **Việc:** thêm `ResolveWorkflow(ctx, q, issue-like input)` theo mục 3 của spec. Thứ tự: engine task → phòng có workflow không flow → phòng có flow (yêu cầu đi flow; không phải yêu cầu lấy cấu hình workspace) → phòng chưa có workflow trả Default → không có phòng trả O-1 (Default).
- Bỏ nhánh `ForProjectRow` khỏi resolver khi cờ bật. Khi cờ tắt, giữ hành vi cũ.
- **Test:** `server/internal/issueworkflow/flow_route_test.go` — `TestActiveWorkflowOfSkipsFlow` hiện khẳng định issue không phải yêu cầu của phòng flow rơi về workflow dự án. Đổi khẳng định theo spec (cờ bật → cấu hình workspace; cờ tắt → hành vi cũ). Thêm bảng test cho từng nhánh.
- **Kiểm chứng:** `cd server && go test ./internal/issueworkflow -count=1` pass (không cần DB).

### P1.3 Kiểm tra khi ghi
- **File:** `server/internal/handler/issue.go` (`CreateIssue`, dòng 3551; kiểm tra ở dòng 3683 đang gọi `ForIssue`), `server/internal/service/issue.go` (dòng 977, `IssueService.Create`), đường cập nhật giao việc trong `issue.go`, `server/internal/handler/issue_trigger.go` (dòng 355).
- **Việc:** trước khi ghi DB, trong cùng transaction, kiểm tra theo mục 5 của spec:
  - `room_workflow_required`: phòng không có workflow đang hoạt động;
  - `room_no_workflow_for_non_request`: không phải yêu cầu của phòng flow, cấu hình workspace là `no_action`;
  - miễn kiểm tra nếu `flow_task IS NOT NULL`.
- Kiểm tra đặt trước `appendTrigger` (R7).
- Đăng ký mã lỗi tại `writeError` theo mẫu các lỗi 422 khác của handler.
- **Test:**
  - tạo issue vào phòng không workflow → 422 đúng mã, không có row issue, không có task (dùng `dbfx`, `testutil.Call(...).Want(422)`);
  - engine task vẫn tạo được khi cấu hình `no_action`;
  - cờ tắt → không đổi hành vi.
- **Kiểm chứng:** `make test` hoặc `go test ./internal/handler -run 'RoomWorkflow|CreateIssue' -count=1` với DB test (cần S1).

### P1.4 Thay các lời gọi `ForIssue` còn lại
- **File:** `service/task.go:6731` (`FailedRunLandingStatus`), `handler/issue_queue.go:61`, `handler/issue_unmoved_run.go:42`, `handler/pr_auto_complete.go:213`, `handler/issue_workflow_entry.go` (dòng 58, 584, 635), `service/issue.go:977`.
- **Việc:** chuyển sang `ResolveWorkflow`. Với các nhánh đọc (không ghi), resolver không bao giờ trả lỗi chặn.
- **Test:** test hiện có của từng file vẫn pass khi cờ tắt; thêm ca cờ bật:
  - `FailedRunLandingStatus`: phòng không workflow → cho phép `blocked`;
  - `pr_auto_complete`: merge vào trạng thái không có trong workflow → `prAutoCompleteNotInWorkflow` đúng;
  - comment và claim của issue ở phòng không workflow vẫn chạy (R6).

### P1.5 Bảo vệ `handoffRunStale`
- **File:** `server/internal/handler/issue_workflow_entry.go:628`.
- **Việc:** khi cờ bật, không hủy run vì kết quả resolver đổi mà không do bước của run đã rời đi. Thông điệp hủy phải nói đúng lý do (R5).
- **Test:** run đã xếp hàng trước khi đổi resolver vẫn chạy; run thật sự lạc bước vẫn bị hủy với thông điệp đúng.

### P1.6 Flow
- **Việc:** chạy toàn bộ test engine với cờ bật: `flow_engine*_test.go`, `flow_route_test.go`, `flow_guards*`, `issue_workflow_flow_test.go`.
- **Kiểm chứng:** pass cả với cờ bật và tắt. Không có ca nào bị bỏ qua để pass.

**Kiểm chứng P1 tổng:** `cd server && make test` (hoặc các package đã nêu) pass; `make check` cho phần web/backend không đổi ngoài dự kiến.

## P2. Cấu hình workspace, popup, CLI

### P2.1 Lưu cấu hình
- **Việc:** lưu `non_request_workflow` (`no_action` | `default` | `workflow:<id>`) cho workspace. Chọn một trong hai:
  - cột JSON trong bảng workspace có sẵn (một migration `ALTER TABLE ... ADD COLUMN` có down); hoặc
  - bảng `workspace_setting` nếu đã có cơ chế tương tự (kiểm tra trước).
- Ghi qua một endpoint chỉ dành cho quản trị workspace. Đọc qua endpoint thành viên thường.
- Migration: số = cao nhất + 1 (hiện 999 → 1000, kiểm tra lại sau rebase). Không thêm foreign key. Không cần `CREATE INDEX CONCURRENTLY` nếu không tạo index.
- **Test:** handler test cho quyền (thành viên bị từ chối ghi), giá trị không hợp lệ bị từ chối, mặc định là `no_action`.
- **Kiểm chứng:** sqlc sau khi sửa SQL (`make sqlc`); `go test ./cmd/migrate ./internal/migrations` pass.

### P2.2 Endpoint xem trước
- **Việc:** `GET` nhận phòng và loại issue (yêu cầu hay không, có flow hay không) và trả về `needs_workflow` cùng lý do. Giao diện gọi trước khi gửi form (R4, mục 6 của spec).
- **Test:** ca phòng có workflow, phòng không workflow, phòng flow có/không cấu hình.

### P2.3 Schema và kiểu
- **File:** `packages/core/api/schema.ts` (dùng `parseWithFallback`, có giá trị mặc định cho field mới), `packages/core/types/`.
- **Test:** test schema cho response thiếu field (theo `AGENTS.md`, mục API Compatibility).

### P2.4 Giao diện popup
- **File:** `packages/views/modals/create-issue.tsx`, `packages/views/modals/create-project.tsx` (chỉ phần liên quan), chỗ giao việc trong issue detail (picker), modal quick-create.
- **Việc:** popup theo mục 6 của spec: hai lựa chọn (OK dùng Default; hoặc dropdown workflow có sẵn), liên kết mở tab Workflow để thiết kế mới, thông báo rõ lựa chọn sẽ được lưu vào cấu hình và đổi được sau. Lưu xong thì thực hiện lại thao tác giao việc.
- Chuỗi giao diện: viết theo `packages/views/locales/en/*.json` và đủ các ngôn ngữ đang hỗ trợ (theo `apps/docs/.../conventions.mdx`, mục UI descriptions). Không mock `next/*` hay `react-router-dom` trong test của views.
- **Test:** component test: popup hiện đúng điều kiện; OK lưu cấu hình; không hiện lại khi đã cấu hình; test mock API ở `@multica/core/api`.
- **Kiểm chứng:** `pnpm typecheck`, `pnpm lint`, `pnpm test` (frontend). Ảnh chụp popup và trang cài đặt cho PR (theo `AGENTS.md`).

### P2.5 Trang cài đặt workspace
- **Việc:** mục "Issue không thuộc yêu cầu của phòng" với giá trị mặc định `No Action`, chọn `Default` hoặc workflow có sẵn. Chỉ quản trị workspace sửa được.
- **File:** khu vực cài đặt workspace trong `packages/views/settings/`.
- **Test:** component test cho quyền và lưu giá trị.

### P2.6 CLI
- **File:** `server/cmd/multica/cmd_workflow.go` (thêm `multica workflow config get/set`), và thông điệp lỗi 422 trong `multica issue create` (in mã và hướng dẫn, R4).
- **Test:** `cmd_workflow_test.go`, `cmd_issue_flow_test.go` pass; thêm ca in thông điệp cho mã lỗi mới.

**Kiểm chứng P2:** chạy thử trên môi trường dev với cờ bật, tạo issue ở từng trường hợp, chụp ảnh popup. Ghi lại issue đã tạo để dọn dẹp (theo quy tắc dev e2e trong memory).

## P3. Bảng dự án nhiều phòng

- **File:** `packages/views/issues/utils/workflow-columns.ts` (`useColumnWorkflow`), `packages/views/issues/surface/use-issue-surface-data.ts` (dòng 423-454), `use-issue-surface-controller.ts`.
- **Việc (O-3):** khi cờ bật, cột của bảng dự án là hợp các workflow đang dùng trong dự án, theo thứ tự xuất hiện. Cột của workflow không thuộc issue nào thì không hiện. Bảng không có dự án (toàn workspace) dùng Default như cũ.
- **Test:** test hàm cột: một phòng, hai phòng khác workflow, phòng không workflow (không đóng góp cột nào), filter trạng thái vẫn hoạt động.
- **Kiểm chứng:** ảnh chụp bảng dự án mua_ngon_ecommerce trên dev.

## P4. Gỡ workflow cấp dự án

Chỉ bắt đầu sau khi P1–P3 pass và cờ đã bật ổn định. Cần S2.

### P4.1 Backend
- Bỏ `SetProjectWorkflow` (`server/cmd/server/router.go:2747`) và handler tương ứng; bỏ field workflow khỏi response project (`server/internal/handler/project.go`).
- `server/pkg/db/queries/project.sql:50` và `org_map.sql:65`: bỏ `workflow_id`, chạy `make sqlc`.
- Xử lý `pipeline` theo kết quả P0 bước 4.
- **Test:** `rg -n "workflow_id" server/pkg/db/queries/project.sql` không còn; test project create/update/get pass.

### P4.2 Migration
- Migration `DROP COLUMN project.workflow_id` (up) và thêm lại cột nullable (down). Số = cao nhất + 1 sau rebase.
- **Kiểm chứng:** `go test ./cmd/migrate ./internal/migrations`; chạy up/down trên DB test (S1). **Chỉ chạy trên DB live sau khi có S2.**

### P4.3 CLI và giao diện
- CLI: bỏ `multica project workflow get/set` (`server/cmd/multica/cmd_workflow.go`).
- Giao diện: bỏ `project-workflow-sheet.tsx`, `project-workflow-row.tsx`, phần chọn workflow trong `create-project.tsx`, `issue-project-picker.tsx`, `company-assign-project-dialog.tsx`; `use-issue-surface-data.ts` không đọc workflow dự án; `packages/core/types/project.ts` và `schema.ts` bỏ field.
- **Kiểm chứng:** `rg -n "projectWorkflow|useProjectWorkflow|project.*workflow_id"` chỉ còn trong code đã xóa rồi; `pnpm typecheck`, `pnpm test`.

### P4.4 Tài liệu và skill của agent
- `server/internal/service/builtin_skills/multica-platform/references/projects.md` (các dòng 89, 155, 214-223 mô tả workflow dự án): cập nhật.
- `apps/docs/content/docs/issues.mdx` và `.zh` bản dịch; `secretary-delegation.mdx`: cập nhật theo `conventions.mdx`.
- `docs/engineering/*`: không sửa tài liệu lịch sử; ghi thêm ghi chú trong spec nếu cần.
- **Kiểm chứng:** `git diff --check`; kiểm tra liên kết nội bộ.

## P5. Bật và kiểm chứng

Cần S3.
1. Chạy lại truy vấn P0 bước 5 trên dữ liệu thật ngay trước khi bật.
2. Gán workflow cho mọi phòng đang có issue mà chưa có workflow (dữ liệu hiện tại: không có issue nào ở các phòng này; cần kiểm tra lại).
3. Bật `room_workflow` trên dev. Quan sát:
   - không run nào bị hủy với thông điệp `workflow_step_left` ngoài ý muốn;
   - không issue nào bị chặn mà không có popup/thông điệp rõ;
   - một vòng flow hoàn chỉnh (yêu cầu → task engine → hoàn tất) chạy đúng;
   - leader phòng plain tạo sub-issue vẫn thành công.
4. Ghi kết quả vào spec (mục kiểm chứng).

**Đường lui:** tắt cờ `room_workflow`. Resolver quay về hành vi cũ, không cần migration. Chỉ khi P4 đã xong thì đường lui là khôi phục từ bản down, và không có dữ liệu cũ quay lại.

## Thứ tự và phụ thuộc

P0 → P1 → P2 → P3 → P4 → P5. P2.4 và P3 có thể làm song song với nhau sau khi P1 xong. P4 không làm trước khi P5 hoàn tất.

## Ước lượng và rủi ro còn lại

- P1 và P2 là phần lớn công việc; P4 là phần có rủi ro dữ liệu cao nhất.
- Không kiểm chứng được đầy đủ nếu chưa có S1 (DB test). Đây là điều kiện bắt buộc trước P1.3.
- Các mục chưa xác minh ở P0 có thể làm thay đổi thứ tự hoặc phạm vi P1.4 và P2.

## Trạng thái thực hiện

Xác nhận của người dùng: S1, S2, S3 đều đồng ý. Cờ `room_workflow` bật mặc định cho mọi workspace; `FF_ROOM_WORKFLOW=off` là công tắc tắt khẩn cấp.

### Đã làm
- **P1.1** Cờ `room_workflow`, mặc định bật, có công tắc env. Thêm `RoomWorkflowEnabled` và test (`server/internal/featureflags`, pass).

### P0 — kết quả đọc mã (bổ sung)
- Các đường tạo issue tự động giao việc cho **phòng** (squad), không chỉ cho agent. Đây là rủi ro mới, chưa có trong spec:
  - `service/squad_handoff.go:480` — chuyển giao giữa các phòng;
  - `service/proposal.go:797` — đề xuất giao cho phòng;
  - `service/knowledge_audit.go:312` — kiểm toán kiến thức giao cho phòng;
  - `handler/squad_sources.go:123` — nguồn nhập giao cho phòng.
  Nếu phòng đích chưa có workflow, hoặc là phòng flow với cấu hình `no_action`, các đường này sẽ nhận 422 và luồng tự động bị hỏng. **Cần quyết định**: với đường hệ thống (không phải người dùng và không phải agent của phòng), chặn hay tự dùng Default. Đề xuất: không chặn đường hệ thống; dùng Default và ghi nhật ký rõ. Phải làm rõ trước P1.3.
- Các đường agent/hệ thống khác cần kiểm tra cùng lúc: `service/retro.go:364`, `service/review.go:514`, `service/initiative_build.go:110`, `integrations/channel/engine/router.go:1209`, `handler/flow_proposal.go:139` (giao cho agent, nên đi theo phòng của agent).
- `issue_workflow.squad_id` vẫn được đọc ở `service/initiative_build.go:277` (`wf.SquadID`) và `planningWorkflowFor` (đọc theo tên workflow). Cần thống nhất trước khi bỏ resolver cũ.
- `applyFlowStatusRules` (`handler/flow_guards.go:654`) trả `handled=true` khi `prev.FlowTask.Valid`, nên engine task không đi qua resolver khi đổi trạng thái. Đã xác minh.

### Còn lại
- P0 bước 5: truy vấn dữ liệu live theo phòng (đã làm một phần: 30 issue, không có issue nào ở phòng chưa workflow hay phòng flow; còn các run `queued`/`dispatched` có `workflow_step`).
- P1.2 đến P1.6, P2 đến P5: chưa làm. Resolver cần nhận cờ, nên phải đổi chữ ký ở ~11 lời gọi `ForIssue`; việc này cần làm kèm test DB (S1 đã đồng ý, cần tạo DB `*_test` riêng, không dùng `multica`).


### Cập nhật quyết định (2026-11-22)
- Đường hệ thống tự tạo issue cho phòng chưa có workflow: chặn ngay (422 `room_workflow_required`), không dùng Default. Popup người dùng hiện ngay khi giao phòng, cho chọn workflow có sẵn hoặc mở tab Workflow để tạo mới. Xem spec mục 13.
- P1.2 cần đổi chữ ký resolver để nhận đủ thông tin issue (parent, người tạo) nhằm phân biệt yêu cầu và không phải yêu cầu. Việc này đụng ~11 lời gọi `ForIssue` và phải làm kèm test DB `*_test`.

### Tiến độ P1 (cập nhật)
- Đã làm: resolver theo phòng và kiểm tra khi ghi (`server/internal/issueworkflow/room.go`, có unit test, pass); nối vào `IssueService.createWorkflow`, `applyIssueWorkflow`, handoff, proposal; lỗi trả 422 `room_workflow_required` / `room_no_workflow_for_non_request` ở create, update, handoff và proposal.
- Test DB-backed (cờ bật, mặc định): `service` pass; `handler` còn 36 test lỗi.
  - Nhóm lỗi đúng theo luật mới, cần viết lại test: test giả định workflow dự án được áp cho issue không có phòng, và fixture squad không có workflow (ví dụ `TestWorkflowGolden_*`, `TestStatusChangeHandsOffOnDefaultProject`, `TestGuard_*`, `TestSquadHandoff*`, `TestSecretary*`). Cần gán `squad.active_workflow_id` trong fixture hoặc đổi kỳ vọng.
  - `TestHandlerTestMainRejectsConfiguredDatabaseFailure` và `TestSearchIndexCopyFromRoleBasedAccessIsRebuilt`: cần kiểm tra có liên quan không.
- Lưu ý: phép so sánh với `FF_ROOM_WORKFLOW=off` không hợp lệ, vì harness test dùng static provider và bỏ qua biến môi trường.
- Còn lại: P2 (lưu cấu hình workspace bằng `workspace.settings`, endpoint, popup, trang cài đặt, CLI), P3, P4, P5.

### Rà soát sau triển khai (2026-11-22)
- Cột `project.workflow_id` được giữ lại (O-6 chưa xác nhận). Không có khóa ngoại, không có index, và không còn code nào đọc hoặc ghi. Xóa cột cần đổi `sqlc.yaml` (sqlc đọc migration theo thứ tự chữ cái, nên migration xóa 1040 đọc trước migration thêm 577).
- Cờ `room_workflow` đã bị xóa, không còn đường tắt `FF_ROOM_WORKFLOW=off`. Luật phòng luôn bật.
- Đường hệ thống tự giao issue vào phòng đều kiểm tra theo spec mục 13: tự động hóa (autopilot), initiative (tạo và chuyển giao), build (issue khởi động), và đề xuất (duyệt giao cho phòng). Các đường này dùng `issueworkflow.CheckRoomPlacement`, trả 422 và không bỏ qua lỗi. Các đường còn lại đi qua `IssueService.Create` hoặc handler cập nhật issue, đã có kiểm tra.
- Ghi cài đặt workspace không còn có thể ghi đè lựa chọn cho issue không phải yêu cầu (kể cả từ form cũ). Lựa chọn chỉ được ghi qua endpoint riêng.
- Không xóa được workflow đang là lựa chọn cho issue không phải yêu cầu (409 `workflow_is_non_request_choice`).
- Còn lại: tên khối `[Project workflow]` trong prompt của agent được giữ, vì daemon và skill phải khớp nhau khi phiên bản daemon chưa đồng bộ. Luật phòng chỉ kiểm tra issue thuộc một dự án, vì issue ngoài dự án không có workflow nào để theo; issue ngoài dự án không bị từ chối. Đây là điểm cần người dùng xác nhận. P5 (bật trên dev) chờ xác nhận S3.
