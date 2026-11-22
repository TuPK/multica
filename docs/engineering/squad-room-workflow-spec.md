# Spec: workflow theo phòng (bỏ workflow cấp dự án)

Trạng thái: bản nháp để duyệt. Chưa có code nào được đổi theo tài liệu này.
Ngày: 2026-11-22. Phạm vi: server Go, CLI, web/desktop, tài liệu agent và người dùng.

## 1. Vấn đề

Hiện một issue thuộc dự án có thể chịu hai lớp workflow: workflow của phòng (squad) đang hoạt động và workflow gắn ở cấp dự án (`project.workflow_id`). Hệ quả đã gặp trên dự án mua_ngon_ecommerce:

- Hai phòng khác nhau trong cùng dự án không chạy độc lập về workflow. Issue giao cho phòng BMAD Planning (phòng có workflow riêng `BMAD Planning Auto`) bị hủy ngay vì bước To Do của workflow cấp dự án (`SDLC Auto`) giao cho SDLC Developer. Run giao cho Blueprint Lead bị coi là lạc bước và bị hủy (`workflow_step_left`).
- Quy tắc chọn workflow nằm ở nhiều nơi và phải được kiểm tra lại mỗi lần giao việc, gây khó hiểu và dễ lệch giữa lúc tạo run và lúc claim.

Mục tiêu: một issue chỉ chịu luật của phòng nó thuộc về. Workflow cấp dự án không còn là nguồn luật.

## 2. Thuật ngữ

- **Phòng (room)** của issue:
  - giao trực tiếp cho squad: phòng là squad đó;
  - giao cho agent có `home_squad_id`: phòng là squad nhà của agent;
  - còn lại (người, hoặc agent không có squad nhà, hoặc chưa giao): không có phòng.
- **Yêu cầu của phòng (request)**: đúng định nghĩa `isSquadRequest` ([server/internal/handler/issue_workflow_entry.go](../../server/internal/handler/issue_workflow_entry.go)). Issue không có `parent_issue_id`, giao cho phòng hoặc cho leader của phòng đang hoạt động, và không phải do leader tự mở để tự điều phối. Đây là định nghĩa duy nhất; không dùng `parent_issue_id` làm tiêu chí riêng.
- **Task của engine (engine task)**: issue có `flow_task IS NOT NULL`. Engine tạo qua `createFlowTask` ([server/internal/handler/flow_engine.go](../../server/internal/handler/flow_engine.go)), luôn gán `flow_step`, `flow_task`, `flow_round` và `parent_issue_id`. Đây là tiêu chí duy nhất để nhận diện task do flow tạo; các truy vấn của engine đã dùng cùng cờ này (`ListFlowTasks`, `ClearIssueFlowTasks`, `InitIssueFlow`).
- **Phòng có flow**: phòng có workflow đang hoạt động có `flow_config`. Với phòng này, yêu cầu đi theo flow (`StartFlowIfEligible`).
- **Cấu hình workspace cho issue không phải yêu cầu** (mục 4).

## 3. Quy tắc chọn workflow (mới)

Hàm duy nhất: `ResolveWorkflow(issue)` trong `server/internal/issueworkflow`. Hàm này chỉ đọc, không bao giờ từ chối. Mọi nơi gọi `ForIssue` hiện nay chuyển sang hàm này.

Thứ tự:

1. **Engine task** (`flow_task` khác NULL): được quản lý bởi flow engine. Trạng thái đổi qua `applyFlowStatusRules`, chạy trước khi resolver được gọi. Resolver trả về Default chỉ để kiểm tra trạng thái (xem mục 9, R1).
2. **Issue có phòng và phòng có workflow đang hoạt động không phải flow**: trả về workflow của phòng. Áp cho cả yêu cầu lẫn không phải yêu cầu.
3. **Issue có phòng là phòng có flow**:
   - yêu cầu (`isSquadRequest`): đi theo flow như hiện nay;
   - không phải yêu cầu, không phải engine task: lấy cấu hình workspace (mục 4).
4. **Issue có phòng nhưng phòng chưa có workflow**: resolver trả về Default. Việc từ chối nằm ở bước ghi (mục 5), không nằm ở resolver. Issue cũ đã ở trạng thái này không bị đổi kết quả đột ngột.
5. **Issue không có phòng**: theo quyết định O-1 (mục 11). Đề xuất: Default. Workflow cấp dự án không còn được dùng.

Không còn bước "rơi về workflow của dự án" ở bất kỳ nhánh nào.

## 4. Cấu hình workspace cho issue không phải yêu cầu

- Phạm vi: **toàn workspace** (một cấu hình chung), theo xác nhận của người dùng.
- Lưu ở bảng cấu hình workspace có sẵn, hoặc một cột JSON trong thiết lập workspace (quyết định khi làm, phải có migration và bản down nếu thêm cột).
- Giá trị:
  - `no_action` (mặc định): chưa cấu hình, luôn hỏi;
  - `default`: dùng Default;
  - `workflow:<id>`: dùng một workflow đang có trong workspace.
- Khi người dùng đã chọn và lưu một giá trị khác `no_action`, các issue sau đó không còn hỏi lại. Người dùng đổi được trong phần cài đặt.
- Lựa chọn có hiệu lực cho issue không phải yêu cầu của phòng có flow. Không ghi đè cấu hình của phòng khác.

## 5. Chặn tại thời điểm ghi

Chỉ kiểm tra khi tạo issue, giao issue, hoặc chuyển issue vào một phòng. Không kiểm tra khi comment, claim, enqueue hay khi resolver chạy.

Các điểm ghi:
- `CreateIssue` ([server/internal/handler/issue.go:3551](../../server/internal/handler/issue.go:3551)) và `IssueService.Create` ([server/internal/service/issue.go](../../server/internal/service/issue.go)).
- Cập nhật giao việc (`UpdateIssue` và đường giao việc riêng nếu có).
- `issue create` của CLI và `QuickCreateIssue` (chạy qua agent).

Điều kiện từ chối (HTTP 422, mã lỗi cố định):
- `room_workflow_required`: phòng chưa có workflow đang hoạt động.
- `room_no_workflow_for_non_request`: issue không phải yêu cầu của phòng có flow, cấu hình workspace là `no_action`.

Bắt buộc:
- Lỗi được trả về **trước** khi ghi DB, trong cùng transaction với việc tạo, để không có issue mồ côi.
- Thông điệp lỗi nói rõ việc cần làm (gán workflow cho phòng, hoặc đặt cấu hình workspace), để agent và CLI báo lại được cho người dùng.
- Engine task được miễn kiểm tra (điều kiện: `flow_task IS NOT NULL`).

## 6. Giao diện

- **Popup "chưa có workflow"** (mục 4 của yêu cầu người dùng):
  - hiện khi người dùng tạo hoặc giao issue mà điều kiện mục 5 không thỏa, và cấu hình workspace là `no_action`;
  - hai lựa chọn: dùng Default (OK), hoặc chọn một workflow có sẵn từ dropdown; có liên kết mở tab Workflow để thiết kế workflow mới;
  - popup nói rõ: lựa chọn sau khi bấm OK và lưu sẽ được ghi vào cấu hình workspace, và có thể đổi sau trong cài đặt;
  - lưu xong thì giao việc được thực hiện lại.
- **Kiểm tra trước khi gửi**: để popup hiện trước khi người dùng gửi form, cần endpoint xem trước (ví dụ `GET` theo phòng và loại issue, trả về có cần workflow không). Không dựa vào việc bắt lỗi sau khi gửi.
- **Quick-create**: người tạo là người dùng trên modal, nên popup hiện trên modal trước khi gửi. Nếu run quick-create do agent chạy mà phòng thiếu workflow, server trả 422 và agent báo lại.
- **Cài đặt workspace**: thêm mục "Issue không thuộc yêu cầu của phòng" với giá trị mặc định `No Action`.
- **Bảng (board)**: xem O-3 (mục 11).

## 7. Bỏ workflow cấp dự án

Thực hiện sau khi các bước trên đã chạy ổn định và cờ tính năng đã bật (mục 8).

- `project.workflow_id`: bỏ khỏi resolver (đã làm ở mục 3), rồi bỏ khỏi:
  - truy vấn `project.sql` (dòng 50), `org_map.sql` (dòng 65, có `p.workflow_id`);
  - API `SetProjectWorkflow` ([server/cmd/server/router.go:2747](../../server/cmd/server/router.go:2747)) và các field trả về của project;
  - CLI `multica project workflow get/set` ([server/cmd/multica/cmd_workflow.go](../../server/cmd/multica/cmd_workflow.go));
  - giao diện: `project-workflow-sheet.tsx`, `project-workflow-row.tsx`, `create-project.tsx`, `issue-project-picker.tsx`, `company-assign-project-dialog.tsx`, `use-issue-surface-data.ts`, `workflow-columns.ts` (`useColumnWorkflow`);
  - schema zod ở `packages/core/api/schema.ts` và kiểu trong `packages/core/types/project.ts`.
- Migration: `DROP COLUMN` có bản down thêm lại cột (không khôi phục dữ liệu). **Thao tác này không hoàn tác được về dữ liệu, cần người dùng xác nhận riêng trước khi chạy trên DB live.** Số migration phải là số cao nhất + 1 sau khi rebase `origin/main` (hiện cao nhất là 999).
- Bảng `pipeline` (`workflow_id`, `project_id`, `pipeline.sql`): chưa xác định phụ thuộc vào `project.workflow_id`. Phải kiểm tra trước (mục 10).

## 8. Rollout

- Mọi thay đổi hành vi nằm sau cờ tính năng workspace mới `room_workflow` (đăng ký trong `server/internal/featureflags/keys.go`), **mặc định tắt**. Khi tắt, hành vi giữ nguyên như hiện nay.
- Trước khi bật:
  1. chạy truy vấn xem trước (mục 10, bước P0) và xử lý các issue bị ảnh hưởng;
  2. gán workflow cho mọi phòng đang có issue mà chưa có workflow;
  3. xác nhận không có run đang xếp hàng có `workflow_step` bị ảnh hưởng.
- Bật theo thứ tự: server (quy tắc ghi + resolver) → giao diện (popup, cài đặt) → xóa workflow cấp dự án.
- Tắt cờ là đường lui: resolver quay về hành vi cũ, không cần migration.

## 9. Rà soát rủi ro

Mỗi rủi ro có biện pháp và test kiểm chứng. Phần này cố ý đầy đủ, theo yêu cầu không bỏ sót rủi ro nào.

**R1. Engine task đổi workflow**
- Hiện: engine task (có `flow_task`) của phòng có flow rơi về workflow của dự án khi resolver chạy.
- Rủi ro: sau khi bỏ fallback, nếu resolver trả về Default hoặc workflow khác, các kiểm tra trạng thái của engine task đổi theo.
- Biện pháp: engine task luôn nhận diện bằng `flow_task`, không phụ thuộc phòng. `applyFlowStatusRules` chạy trước resolver. Cần test: mọi test trong `flow_engine*_test.go`, `flow_route_test.go`, `flow_guards` phải pass khi cờ bật.
- Chưa xác minh: liệu `applyFlowStatusRules` có phủ đủ mọi trạng thái của engine task. **Phải đọc và kiểm tra trước khi code.**

**R2. Sub-issue do flow tạo bị chặn khi cấu hình `no_action`**
- Biện pháp: kiểm tra mục 5 có điều kiện loại trừ `flow_task IS NOT NULL`. Test: flow mở task trong lúc cấu hình là `no_action` vẫn thành công.

**R3. Yêu cầu bị tách khỏi flow (`flow_detached`)**
- Hiện: request bị tách (`flow_detached`) không còn `flow`, vẫn không có parent và giao cho phòng, nên là yêu cầu; phòng là phòng có flow.
- Rủi ro: không có workflow nào để đi theo sau khi tách.
- Biện pháp: yêu cầu của phòng có flow mà không đang chạy flow thì đi theo Default, và không bị chặn. Đây là quyết định O-4. Test: tách rồi kiểm tra trạng thái.

**R4. Agent và CLI bị chặn bất ngờ khi tạo sub-issue**
- Hiện: leader phòng plain (không flow) tạo sub-issue giao cho thành viên qua CLI. Phòng này có workflow nên không bị chặn.
- Rủi ro: leader hoặc planning job tạo issue vào phòng chưa có workflow, hoặc vào phòng flow có `no_action`, sẽ nhận 422 và run của agent có thể dừng hoặc treo.
- Biện pháp: lỗi 422 có mã và hướng dẫn; prompt của agent (builtin skill `multica-platform`) được cập nhật để báo lại thay vì thử lại vô hạn. Test: CLI in ra mã lỗi; agent không tự chuyển sang phòng khác.

**R5. Hủy run hàng loạt khi đổi resolver (`handoffRunStale`)**
- Hiện: `handoffRunStale` ([server/internal/handler/issue_workflow_entry.go:628](../../server/internal/handler/issue_workflow_entry.go:628)) hủy run có `workflow_step` khi kết quả resolver không khớp. Đây là nguồn gốc của lỗi `workflow_step_left` đã gặp.
- Rủi ro: khi đổi resolver, run đang xếp hàng sẽ bị hủy hàng loạt.
- Biện pháp: trước khi bật cờ, truy vấn các run `queued`, `dispatched` có `workflow_step` của issue thuộc phòng bị ảnh hưởng; phải bằng 0 hoặc được xử lý. Thông điệp hủy phải nói đúng lý do. Test: run đã xếp hàng trước khi đổi vẫn chạy khi cờ bật.

**R6. Comment, claim, enqueue**
- Hiện: `ForIssue` được gọi ở `comment.go` (một số chỗ), `task.go`, `issue_queue.go`, `issue_unmoved_run.go`, `pr_auto_complete.go`, `issue_trigger.go`, `autopilot.go`.
- Rủi ro: nếu resolver từ chối, mọi thao tác của issue cũ lỗi.
- Biện pháp: resolver không bao giờ từ chối (mục 5). Test: comment và claim trên issue đang ở trạng thái "phòng chưa có workflow" vẫn chạy.

**R7. Chặn quá muộn trong luồng tạo issue**
- Hiện: `issue_trigger.go:355` dùng workflow khi tạo để quyết định người nhận bàn giao.
- Rủi ro: kiểm tra mục 5 phải xảy ra trước khi trigger được ghi, nếu không sẽ có run đã enqueue cho issue bị từ chối.
- Biện pháp: kiểm tra trong cùng transaction, trước `appendTrigger`. Test: issue bị từ chối không có task nào được tạo.

**R8. Trạng thái khi run thất bại (`FailedRunLandingStatus`)**
- Hiện: `service/task.go:6731` đưa issue về `blocked` nếu workflow cho phép.
- Rủi ro: đổi workflow làm kết quả đổi (phòng không có workflow → Default → cho phép `blocked`).
- Biện pháp: kiểm tra test hiện có `FailedRunLandingStatus`; thêm ca cho phòng chưa có workflow.

**R9. Trạng thái merge PR (`pr_auto_complete.go:213`)**
- Hiện: merge chỉ ghi trạng thái nằm trong workflow của issue.
- Rủi ro: đổi workflow làm một số issue chuyển trạng thái khác.
- Biện pháp: test cho issue đã merge trước khi bật cờ; thông báo `prAutoCompleteNotInWorkflow` vẫn đúng.

**R10. Bảng (board) của dự án**
- Hiện: `useColumnWorkflow` lấy cột của dự án từ workflow dự án.
- Rủi ro: một bảng dự án có issue thuộc nhiều phòng, mỗi phòng một workflow. Không có một bộ cột duy nhất.
- Quyết định O-3 (mục 11). Đề xuất: bảng dự án hiển thị cột là hợp của các workflow đang dùng trong dự án, theo thứ tự xuất hiện; cột không thuộc workflow của issue nào thì không hiện.

**R11. Issue không có phòng**
- Hiện: fallback về workflow của dự án.
- Rủi ro: chưa có quy định rõ (O-1). Ba issue hiện tại chưa có phòng (TUPK-1155, TUPK-1159, TUPK-1207) đều đã done hoặc cancelled nên không bị ảnh hưởng.
- Biện pháp: quyết định O-1 trước khi code.

**R12. Phòng không có workflow đang có issue hoặc run**
- Hiện (live): 55 phòng chưa có workflow nhưng không có issue nào được giao cho các phòng này; không có autopilot phòng.
- Rủi ro: phòng bị bỏ sót khi gán workflow, hoặc có agent giao việc cho phòng mà không thấy trong truy vấn.
- Biện pháp: truy vấn P0 kiểm tra cả issue giao cho agent có `home_squad_id` thuộc phòng đó, không chỉ giao cho squad.

**R13. Phòng có flow hiện không có issue nào**
- Hiện (live): Marketing Studio, PR & Comms, E2E-FLOW Squad-test, Dynamic from scratch-test đều có workflow flow nhưng không có issue.
- Rủi ro: thấp về dữ liệu, nhưng là đường đi chính của các flow mới. Biện pháp: test đầy đủ đường flow.

**R14. Các dịch vụ dùng `issue_workflow.squad_id` (migration 973) và `squad.active_workflow_id` (974)**
- Hai cách liên kết workflow với phòng cùng tồn tại. `sdlcWorkflowFor` và `planningWorkflowFor` còn dùng `squad_id` của workflow.
- Rủi ro: resolver mới chỉ đọc `active_workflow_id`, nếu phần khác vẫn đọc `squad_id` thì hai nguồn lệch nhau.
- Biện pháp: kiểm tra toàn bộ nơi đọc `issue_workflow.squad_id` trước khi làm. Quyết định: `active_workflow_id` là nguồn duy nhất cho resolver; `squad_id` chỉ còn cho các chỗ đã phụ thuộc, hoặc thống nhất trong cùng đợt.

**R15. Issue được tạo bởi quy trình tự động của hệ thống**
- Ví dụ: `isSecretaryFiling` (Secretary tạo desk issue, giao cho chính nó), `squad_planning_workflow.go`, `initiative_build.go`, quy trình kế hoạch của công ty.
- Rủi ro: các đường này tạo issue vào phòng chưa có workflow hoặc không phải yêu cầu, và bị chặn.
- Biện pháp: liệt kê toàn bộ đường tạo issue tự động, kiểm tra từng đường trước khi bật cờ (mục 10, bước P1). Đường nào bị chặn phải được sửa, không được âm thầm bỏ qua.

**R16. Pipeline và org map**
- `org_map.sql:65` đọc `p.workflow_id`; `pipeline` có cột `workflow_id` riêng.
- Biện pháp: kiểm tra trước khi xóa cột (mục 7).

**R17. Tài liệu và skill của agent**
- Các trang: `apps/docs/content/docs/issues.mdx` (và bản `.zh`), `secretary-delegation.mdx`.
- Skill cho agent: `server/internal/service/builtin_skills/multica-platform/references/projects.md` mô tả workflow của dự án (dòng 89, 155, 214-223).
- Biện pháp: cập nhật cùng lúc với code, theo `AGENTS.md`, kèm các bản dịch.

**R18. Dữ liệu không hoàn tác được**
- Xóa cột `project.workflow_id` làm mất lựa chọn workflow cấp dự án của các dự án hiện có (ví dụ mua_ngon_ecommerce đang là SDLC Auto).
- Biện pháp: trước khi xóa, ghi lại danh sách dự án và workflow vào tài liệu này; người dùng xác nhận. Việc gán SDLC Auto cho SDLC Greenfield đã làm trước (0 issue bị ảnh hưởng) nên không mất thông tin.

**R19. Test không chạy được trong môi trường hiện tại**
- Test DB của handler cần `DATABASE_URL` kết thúc bằng `_test`. Hiện chưa có DB test nào được cấp. Không dùng DB `multica` cho test.
- Biện pháp: xin quyền tạo DB test riêng trước khi bắt đầu giai đoạn code có test DB.

**R20. Quyền của người dùng khi đổi cấu hình workspace**
- Biện pháp: đổi cấu hình workspace chỉ dành cho quản trị workspace (như các cài đặt workspace khác). Thành viên thường chỉ thấy popup và chọn trong popup nếu có quyền gán workflow cho phòng.

## 10. Kế hoạch triển khai

Mỗi bước có tiêu chí kiểm chứng. Bước không qua được tiêu chí thì không chuyển bước sau.

**P0. Kiểm tra dữ liệu và đọc mã (không đổi code)**
- Đọc `applyFlowStatusRules` và xác nhận phủ engine task (R1).
- Liệt kê mọi đường tạo issue tự động (R15) và đường đọc `issue_workflow.squad_id` (R14).
- Truy vấn: issue giao cho phòng, và issue giao cho agent có `home_squad_id` thuộc phòng, theo trạng thái phòng (có/không workflow, có/không flow).
- Truy vấn: run `queued`/`dispatched` có `workflow_step`.
- Kiểm tra `pipeline` và `org_map` (R16).
- Tiêu chí: danh sách đầy đủ, không còn đường tạo issue nào chưa xác định.

**P1. Resolver và quy tắc ghi, sau cờ `room_workflow` (tắt mặc định)**
- Viết `ResolveWorkflow` (mục 3). Thay toàn bộ lời gọi `ForIssue` (khoảng 11 chỗ thực sự, không phải toàn bộ các hàm có chữ `ForIssue` trong tên).
- Thêm kiểm tra ghi (mục 5) vào `CreateIssue`, `IssueService.Create`, cập nhật giao việc, và đường CLI/quick-create.
- Thêm endpoint xem trước (mục 6).
- Test:
  - bảng test resolver: mọi nhánh của mục 3, kể cả engine task, yêu cầu, không phải yêu cầu, phòng không workflow, không phòng;
  - test ghi: 422 đúng mã, không có DB row, không có trigger;
  - test flow: `flow_engine*_test.go`, `flow_route_test.go` pass;
  - test không đổi hành vi khi cờ tắt;
  - test comment, claim, `handoffRunStale`, `FailedRunLandingStatus`, `pr_auto_complete`.
- Tiêu chí: test pass với cờ bật và tắt.

**P2. Cấu hình workspace và giao diện**
- Lưu cấu hình (mục 4), API đọc/ghi (chỉ quản trị workspace), schema zod.
- Popup (mục 6): tạo issue, giao việc, quick-create modal; OK để dùng Default hoặc chọn workflow; lưu cấu hình; thông báo rõ có thể đổi sau.
- Trang cài đặt workspace.
- CLI: đọc và đặt cấu hình; lỗi 422 có hướng dẫn.
- Test: component test popup và cài đặt; test không mock `next/*` hay `react-router-dom` trong views; test mock API ở `@multica/core/api`.
- Tiêu chí: popup hiện đúng điều kiện, không hiện khi đã cấu hình.

**P3. Bảng dự án và các chỗ đọc workflow cấp dự án**
- Quyết định O-3 áp dụng cho `useColumnWorkflow` và `workflow-columns.ts`.
- Test: bảng hiển thị đúng cột khi dự án có nhiều phòng.

**P4. Gỡ bỏ workflow cấp dự án**
- Bỏ API, CLI, giao diện và field như mục 7.
- Migration: bỏ cột `project.workflow_id` (up), thêm lại cột nullable (down). **Chờ xác nhận riêng trước khi chạy trên DB live.**
- Cập nhật `org_map.sql`, chạy `make sqlc`.
- Cập nhật tài liệu (R17), kèm bản `.zh`.
- Tiêu chí: `go test ./cmd/migrate ./internal/migrations` pass; không còn tham chiếu nào đến `project.workflow_id` (kiểm tra bằng `rg`).

**P5. Bật và kiểm chứng**
- Chạy lại P0 trên dữ liệu thật; xử lý các mục còn lại.
- Bật cờ trên môi trường dev; quan sát run và popup.
- Tiêu chí: không run nào bị hủy ngoài ý muốn, không issue nào bị chặn không có lý do, đường flow chạy hết một vòng.

## 11. Quyết định cần người dùng xác nhận

- **O-1. Issue không có phòng** (agent không có squad nhà, hoặc người giao): đề xuất **Default**, không dùng workflow dự án. Không có popup vì không có phòng.
- **O-2. Agent và CLI tạo issue khi chưa có cấu hình** (`no_action`): đề xuất **từ chối kèm mã lỗi và hướng dẫn**, không âm thầm dùng Default, để không che giấu việc chưa cấu hình. Đây là giả định đang áp dụng.
- **O-3. Bảng dự án có nhiều phòng**: đề xuất **hợp của các workflow đang dùng**, mỗi cột giữ thứ tự xuất hiện trong workflow.
- **O-4. Yêu cầu bị tách khỏi flow** (R3): đề xuất đi theo Default, không chặn.
- **O-5. Bật cờ**: đề xuất cờ `room_workflow` mặc định tắt, bật từng workspace sau khi P0 xong.
- **O-6. Xóa cột `project.workflow_id`** (P4): cần xác nhận riêng vì không hoàn tác được về dữ liệu.

## 12. Kiểm chứng và rà soát lần hai

Kết quả rà soát lần này:
- Số lời gọi thực sự của `ForIssue` là khoảng 11, không phải 40. Kết quả 40 trước đó do đếm cả các hàm có tên chứa `ForIssue` (ví dụ `ListCommentsForIssue`).
- Phát hiện mới so với đề xuất trước: R10 (bảng dự án nhiều phòng), R14 (hai cách liên kết workflow với phòng), R15 (đường tạo issue tự động), R16 (pipeline và org map), R18 (dữ liệu không hoàn tác được), R19 (không có test DB).
- Đã xác minh: dữ liệu live hiện không có issue nào ở phòng chưa có workflow hay ở phòng flow; sub-issue do engine có cờ `flow_task`; `ListFlowTasks` và các truy vấn khác của engine dùng cờ này.
- Chưa xác minh (phải làm ở P0): `applyFlowStatusRules` phủ engine task; danh sách đầy đủ đường tạo issue tự động; các đường đọc `issue_workflow.squad_id`.


## 13. Quyết định bổ sung (người dùng, 2026-11-22)

**Đường hệ thống tự tạo issue cho phòng chưa có workflow: chặn ngay, không dùng Default.**

- Áp dụng cho mọi đường tạo issue giao cho phòng: chuyển giao giữa phòng (`service/squad_handoff.go`), đề xuất (`service/proposal.go`), kiểm toán kiến thức (`service/knowledge_audit.go`), nguồn nhập (`handler/squad_sources.go`), và các đường tương tự.
- Việc chặn xảy ra ở đúng thời điểm giao issue cho phòng. Với người dùng, popup hiện ngay khi giao: thông báo phòng chưa có workflow, cho chọn workflow có sẵn, hoặc điều hướng tới tab Workflow để tạo workflow mới.
- Với đường hệ thống, lỗi 422 `room_workflow_required` được trả về ngay, không tạo issue, và đường gọi phải ghi nhận lỗi (không âm thầm bỏ qua).
- Hệ quả: một phòng chưa có workflow không nhận được việc tự động cho đến khi người dùng gán workflow. Đây là chủ đích.

Cập nhật các mục liên quan:
- O-2 (agent/CLI khi `no_action`): từ chối kèm mã lỗi, nhất quán với quyết định trên.
- R4 và R15: không còn biện pháp "dùng Default" cho đường hệ thống.
- Mục 5: thêm điều kiện `room_workflow_required` áp cho đường hệ thống, không có ngoại lệ.

**Ghi chú triển khai:** resolver cần biết issue có phải yêu cầu của phòng hay không (dựa trên `parent_issue_id` và người tạo), nên không thể chỉ truyền `project_id`, `assignee_type`, `assignee_id`. Chữ ký `ForIssue` phải đổi để nhận issue đầy đủ hoặc các trường tương đương; khoảng 11 lời gọi bị ảnh hưởng.
