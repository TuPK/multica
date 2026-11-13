# Diễn tập flow Kit Việt trên web

Trạng thái (đợt đầu, agent giả lập; đợt chạy agent thật nằm ở mục 8): **luồng chưa chạy được đến cuối từ web**. Các màn squad và request hiển thị đúng đến bước task Draft. Từ bước đó, người dùng không đi tiếp được nếu không có agent thật. Verdict của gate cũng không có nút trên UI (xem B5). Không chạy agent thật nào, không gọi dịch vụ ngoài.

## 1. Môi trường

- Backend: `APP_ENV=development`, cổng 18766, database `multica_kv_flow_test` (kiểm bằng `scripts/require-test-database.sh`). Không đặt `RESEND_API_KEY`, `SMTP_HOST`, `MULTICA_DEV_VERIFICATION_CODE`, nên mã đăng nhập được in vào log backend. `FF_FLOW_RUNNER` tắt (mặc định).
- Web: `next dev` của `apps/web`, cổng 3002, `REMOTE_API_URL` trỏ về cổng 18766. Đây là cách chạy tương đương `pnpm dev:web`, không tạo lệnh mới.
- Trình duyệt: Chromium của Playwright 1.58.2 có sẵn trong cache, lái bằng script trong `/home/av-server/.cache/claude-delegate/20261113-081623-kiro/drill/`. Ảnh chụp nằm trong `.../shots/`.
- Tài khoản mẫu: `kv.drill.owner@example.com` (owner của workspace). Workspace `kv-drill`, tiền tố issue `KVD`.
- Runtime mô phỏng (SIMULATED): quickstart squad yêu cầu một runtime online. Script `drill/sim-daemon.mjs` đăng ký một runtime `Drill sim claude` và gửi heartbeat, nhưng không bao giờ claim task và không khởi động CLI nào. Các lượt chạy của agent vì vậy nằm ở trạng thái Queued.
- Thao tác lead (plan, advance) và hủy lượt chạy queued được gọi qua API với phiên của owner. Các bước này được ghi là SIMULATED bên dưới.

## 2. Kết quả từng bước

| Bước | Kết quả | Bằng chứng và ghi chú |
| --- | --- | --- |
| 3a. Tạo squad từ template Kit Việt Trade | PASS | Quickstart "Import & Export Desk (Xuất nhập khẩu)" tạo squad, leader Trade Lead, 6 agent, workflow `flow_kv_trade@b10d7319`. Danh sách squad có cột Workflows: tên workflow, số `1`, badge `Enforced`. Ở 1280px tên bị cắt thành "Import ...". Ảnh `04b-squad-list.png`. |
| 3b. Mục Workflows trong squad detail | PASS | Thứ tự Instructions → Workflows → Recent work. Dòng hiện Active, `flow_kv_trade@b10d7319`, 9 bước, 7 task, Auto off (`05c-workflows-section.png`). |
| 3b. Graph sheet | PASS | Sheet liệt kê 9 bước, bước `in_review` có "Human only", gate và work, điều kiện `when` và `requires`, "Return up to 2 times" (`06-graph-sheet.png`). Chip `when customs_lookup` tràn qua mép thẻ bước customs (lỗi nhỏ). |
| 3c. Đặt workflow đang dùng | NOT RUN | Workflow đã là active sau quickstart. Không có workflow thứ hai để thử đổi active. |
| 3c. Bật/tắt auto | PASS | Bấm Auto off → Auto on, `PUT .../flow-auto` 200, giữ nguyên sau khi tải lại trang (`07b-auto-after-reload.png`). Trạng thái để lại là on (không ảnh hưởng vì runner tắt). |
| 3d. Tab Workflow: chip và ẩn sửa/nhân bản/xóa | PASS một phần | Chip hiện "Managed by Kit Việt" (locale en). Danh sách không có menu sửa, nhân bản, xóa. Trang designer vẫn có "Add step" và "Move up/Move down" trên mọi bước (B2). |
| 3e. Tạo request giao cho squad, thành request flow | PASS | KVD-1 có `managed: true`, bước `todo`, Round 0, ngay khi tạo. Panel Flow hiện workflow, bước, vòng, Resume và Detach (admin). Bảng task có cột Issue, Task, Round, Status, Outcome, nhưng cột Outcome bị cắt ở 1280px (B3). |
| 3f. Plan | SIMULATED | Lead gọi `POST /flow/plan` với route `shipment_dossier`, customs_lookup và export bật. Flow chuyển `collect` và mở KVD-2 (docs), KVD-3 (customs). |
| 3f. Đóng task Collect trên UI | PASS (sau khi sửa B1) | KVD-2 và KVD-3 đổi status sang Done trong menu trạng thái, PUT 200. Trước khi sửa B1, ô status không bấm được. |
| 3f. Advance sau khi task đóng | PASS sau khi hủy lượt chạy | Engine không chuyển bước vì hai lượt chạy vẫn Queued (B6). Hủy hai lượt bằng `POST /api/issues/{id}/tasks/{taskId}/cancel` (SIMULATED thay cho nút "Cancel run", vì nút này không bấm được bằng script). Advance sau đó: `routed` sang `draft`, mở KVD-5 (crosscheck). |
| 3f. Đóng task Draft khi chưa có đính kèm | PASS (kiểm phủ định) | PUT 422 `flow_attachment_required`. Thông điệp chỉ có lệnh CLI `multica issue comment add ... --attachment` (B4). |
| 3f. Đính kèm file và đóng Draft bằng người dùng | FAIL (thiết kế, B5) | Upload qua UI được (200). Comment có file đính kèm được tạo qua API với `attachment_ids`, file có `comment_id`. Guard vẫn từ chối vì `agentAttachmentCount` chỉ đếm file do agent được giao task tải lên. |
| 3f. Gate verdict từ UI | NOT RUN | Chưa tới bước gate. Menu trạng thái không có lựa chọn outcome (đã thấy trong menu), nên người dùng không có đường đưa verdict từ UI (B5). |
| 3f. Dừng 2, vòng sửa, dừng 3 | NOT RUN | Phụ thuộc bước gate và draft. |
| 3f. Resume và Detach cho admin | PASS (hiển thị) | Panel Flow của KVD-1 hiện cả hai nút với owner. |
| 3f. Resume và Detach ẩn với non-admin | NOT RUN | Không tạo người dùng thứ hai (cần invite và đăng nhập riêng). |
| 3g. Trạng thái rỗng: squad không có workflow hiện "-" | NOT RUN | Không tạo squad không có workflow. |
| 3g. Trạng thái lỗi tải, nút thử lại | NOT RUN | Không tạo lỗi có chủ đích. |
| 4. Phone 390px, squad detail | PASS một phần | Không cuộn ngang trang (scrollWidth 390 = viewport). Mục Workflows đọc được, badge Active và "1 open request" hiển thị đủ (`24-phone-squad-workflows.png`). Bảng thành viên bị chồng chữ (B7, không thuộc flow). Gutter quan sát khoảng 24px, chưa đo chính xác. |
| 4. Phone 390px, request | PASS một phần | Không cuộn ngang. Sub-issue và hoạt động hiển thị đủ (`25-phone-request-flow.png`). Panel Flow nằm trong Properties nên bị ẩn trên điện thoại, không kiểm được. |

## 3. Bug tìm được

**B1. Panel Flow làm ô thuộc tính bị bóp còn khoảng 8px (đã sửa).**
- Trang: request có flow và sub-issue của nó (KVD-1, KVD-2).
- Tái hiện: mở KVD-1 ở 1280px. Status hiện "T..", Assignee "I..", Project "N..". Ô status không bấm được.
- Nguyên nhân: nhánh flow trong `IssueWorkflowPropRow` bọc `IssueFlowCard` trong `<div className="py-2">`. Div này không có `col-span-2`, nên chiếm ô nhãn của lưới hai cột. Các PropRow khác đều dùng `col-span-2`.
- Mong đợi: panel trải qua cả hai cột, giá trị hiển thị đủ. Issue thường đo được 60–93px, issue flow sau sửa đo được 93px ("No project").
- Sửa: `packages/views/issues/components/issue-detail.tsx` (thêm `col-span-2` và `data-testid="issue-flow-prop"`). Test: `packages/views/issues/components/issue-detail.test.tsx`, thêm `getIssueFlow` vào mock mặc định và test "spans a flow request panel across both property columns so the values keep their width".

**B2. Trang designer của workflow do Kit Việt quản lý vẫn cho thêm và di chuyển bước (chưa sửa).**
- Trang: `/kv-drill/workflow/70eda70d-...` (workflow flow).
- Quan sát: danh sách và drawer đã ẩn sửa, nhân bản, xóa. Designer vẫn có "Add step" và "Move up/Move down" trên mọi bước. Nút Save và ô tên đã ẩn hoặc khóa.
- Cần quyết định: designer nên là chế độ chỉ đọc cho workflow có `template_ref`, hay chỉ ẩn các nút sửa. Chưa sửa vì đây là thay đổi tương tác của designer.

**B3. Bảng task trong panel Flow bị cắt ở mép phải (chưa sửa).**
- Quan sát ở 1280px: cột Status và Outcome nằm ngoài khung. Không có thanh cuộn.
- Chưa sửa: cần chọn cách hiển thị (cuộn ngang trong bảng, hay bỏ bớt cột ở độ rộng hẹp).

**B4. Thông điệp lỗi của guard chỉ có lệnh CLI (chưa sửa).**
- Ví dụ: `this task closes only with a file you attached to it: run multica issue comment add <id> --attachment <file> first`.
- Người dùng UI không chạy được lệnh này. Chưa xác nhận UI hiển thị thông điệp này thế nào (toast).
- Cần quyết định: viết lại thông điệp theo người dùng hay giữ cho agent.

**B5. Người dùng không hoàn thành được các bước cần file hoặc verdict (thiết kế, cần quyết định).**
- Task có `requires: ["attachment"]` chỉ đóng được khi file do agent được giao task tải lên (`agentAttachmentCount`, `flow_guards.go`). Nếu người dùng đính kèm, guard vẫn từ chối (đã kiểm: PUT 422 sau khi file có `comment_id`).
- Gate có `requires: ["outcome"]` cần `--outcome approved|rejected`. Menu trạng thái trên UI không có lựa chọn outcome. Người dùng không đưa được verdict từ UI. Đây là suy luận từ code và menu, chưa chạy được vì chưa tới bước gate.
- Hệ quả: web không đi hết được flow nếu không có agent thật. Cần quyết định: thêm lựa chọn outcome trên UI cho task gate, và cho phép người dùng (hoặc admin) đính kèm thay agent cho task cần file, hay để việc đó luôn cho agent.

**B6. Lượt chạy queued khiến task đã Done không được coi là đã đóng (thiết kế, cần quyết định).**
- `loadFlowTasks` coi task là chưa đóng nếu còn lượt chạy đang hoạt động (`ListActiveTasksByIssue`). Người dùng đặt Done trên UI, nhưng engine vẫn chờ.
- Với agent thật, lượt chạy kết thúc và vấn đề không xảy ra. Với lượt bị kẹt, người dùng phải hủy lượt riêng. Cần quyết định: Done trên UI có nên hủy lượt đang queued không.

**B7. Bảng thành viên squad bị chồng chữ ở 390px (không thuộc flow, chưa sửa).**
- Tên agent đè lên cột runtime, dòng cuối cùng đè lên nút "Trade Lead".

**Quan sát nhỏ, không phải lỗi sản phẩm.**
- Overlay Next dev báo 2 issue: `/api/me` trả 401 lúc chưa đăng nhập, `console.error` được ghi lại. Chỉ xuất hiện khi chạy dev.
- `next dev` ghi đè `apps/web/AGENTS.md` lúc khởi động (đổi `#` thành `##`). Đã khôi phục từ `HEAD`. Sẽ ghi lại lần nữa nếu chạy lại dev server.
- Test DB có 828 quy tắc wakeup cũ làm `issue_wakeup_dispatch` báo lỗi mỗi chu kỳ. Dữ liệu này có sẵn từ trước, không do drill.
- Tên workflow ở cột Workflows bị cắt ở 1280px.

## 4. Kiểm tra đã chạy

- Test đỏ trước khi sửa: `cd packages/views && pnpm exec vitest run issues/components/issue-detail.test.tsx -t "spans a flow request panel"` cho `Tests 1 failed | 111 skipped (112)`. Lỗi: wrapper không có `data-testid`.
- Sau khi sửa: `cd packages/views && pnpm exec vitest run issues/components/issue-detail.test.tsx` cho `Test Files 1 passed (1)` và `Tests 112 passed (112)`.
- Guard database: `bash scripts/require-test-database.sh` cho `test database: multica_kv_flow_test`.
- Không chạy `make test`, `make check`, `make up`, `make down`, `make migrate-up`, migration hay `go test`. Không thay đổi backend.
- Không chạy typecheck hay lint cho toàn `@multica/views` (thay đổi là một className và một data-testid).

## 5. Chưa kiểm và lý do

- Agent thật: không chạy (quy định của phiên). Mọi bước của agent (lead tạo task, drafter đính kèm, gate cho verdict) được giả lập hoặc không chạy được.
- Gate, dừng 2, vòng sửa, dừng 3, export: NOT RUN, vì phụ thuộc B5.
- Non-admin: NOT RUN, không có người dùng thứ hai.
- Trạng thái rỗng và lỗi: NOT RUN.
- Runner nền (tầng 2): không bật, không kiểm.
- Mobile app: không thuộc phạm vi.

## 6. Trạng thái để lại

- Đã dừng backend (cổng 18766), web (cổng 3002) và daemon mô phỏng. `pgrep -fa` không còn tiến trình nào của phiên này. Hai `next-server` (16.3.6 và 16.4.3) thuộc phiên khác, không đụng.
- Test DB còn: workspace `kv-drill`, squad Import & Export Desk, workflow đã bật auto, request KVD-1 đến KVD-5, runtime `Drill sim claude`, các user `kv.drill.owner` và `kv.drill.dbg`. Đây là dữ liệu test.
- Repo: thay đổi của tôi chỉ là `issue-detail.tsx`, `issue-detail.test.tsx` và file báo cáo này. Chưa commit. `migrations_lint_test.go` và `knowledge_test.go` trước đó được báo là đã sửa, nay không còn trong `git status`. Tôi không chạm vào chúng.

## 7. Đợt sửa B2 đến B6 và diễn tập với agent thật (chưa chạy được)

### 7.1 Đã sửa (đợt này)

| Mục | Phương án chọn | Vì sao đủ tốt để đưa vào dùng | File và test |
| --- | --- | --- | --- |
| B2 | Khóa designer khi workflow có `template_ref` hoặc `flow_config`. Mọi điều khiển sửa (Add step, Move up/down, Add/Edit/Delete transition, handler, instructions, Save, tên, mô tả) đều `disabled`, canvas không kéo, nối hay xóa node. | Server vẫn là nguồn sự thật. Khóa ở UI chỉ chặn thao tác không được phép, không thay thế guard. Không dùng `fieldset`, vì test và trình duyệt kiểm được từng nút. | `workflow/designer/step-inspector.tsx`, `designer-canvas.tsx`, `workflow-designer-page.tsx`; test `locks every step and transition control on a template-managed workflow` trong `workflow-designer-page.test.tsx`. |
| B3 | Giữ wrapper `overflow-x-auto`, thêm `whitespace-nowrap` cho mã issue, round, status, outcome và giảm padding ngang từ `px-3` xuống `px-2`. | Dùng token có sẵn của bảng, không thêm component. Bảng vừa khung sidebar; nếu vẫn tràn thì người dùng vẫn cuộn được. | `workflow/issue-flow-state-panel.tsx`; test `keeps the identifier, status and outcome on one line...`. Jsdom không đo layout; kiểm tra thị giác cần làm trên trình duyệt. |
| B4 | Helper `issueUpdateErrorMessage` đổi mã lỗi guard thành copy localised (`flow.errors.*`). Khóa đã có nhưng chưa được dùng nên được nối vào. Copy `flow_attachment_required`, `flow_outcome_required`, `flow_outcome_invalid` được viết lại cho người dùng. Đã dịch cả 7 locale. | Thông điệp server chỉ có lệnh CLI, người dùng không chạy được. Mã không nằm trong danh sách thì vẫn hiện thông điệp của server, không ẩn lỗi. | `issues/utils/issue-update-error.ts` và test cùng tên; gọi từ `issues/actions/use-issue-actions.ts` và `issues/components/issue-detail.tsx`. Copy: `packages/views/locales/*/issues.json`. |
| B5 | Hai phần. (a) Verdict: panel Flow hiện nút Approve và Reject trên gate đang mở. Nút gửi `status: done` kèm `outcome`, guard của server quyết định. Kiểu và schema có thêm `kind` (gate hoặc work) lấy từ định nghĩa workflow. (b) Attachment: giữ thiết kế chỉ agent đính kèm. Guard đếm file do agent được giao task tải lên. Copy giải thích việc này khi bị từ chối. | Không đổi quy tắc server: thêm quyền cho người dùng đính kèm là đổi chính sách, không phải sửa UI. Verdict thì chỉ là cùng một thao tác status mà người dùng làm bằng menu, nên không có quy tắc mới. | `workflow/issue-flow-state-panel.tsx`; `packages/core/types/issue-flow.ts`; `packages/core/api/schemas.ts`; server `flow_state.go` (`Kind`, `flowTaskKind`) và `flow_state_kind_test.go`; test trong `issue-flow-state-panel.test.tsx`. |
| B6 | Khi task flow chuyển sang trạng thái đóng, server hủy các lượt chạy đang `queued` của task đó (cùng cơ chế "Cancel run" của người dùng, với `cancelled_by` là người đã đóng). Lượt đang `running` được để chạy xong, vì agent có thể còn đang ghi verdict. Gắn ở cả đường cập nhật đơn lẻ và batch. | Best-effort, cùng mẫu với `storeOutcomeAfterMove`: lỗi được ghi log và không làm hỏng việc đóng. Không thêm SQL mới, dùng lại `TaskService.CancelTaskByUser`. | `server/internal/handler/flow_guards.go` (`cancelQueuedRunsOfClosedFlowTask`), `issue.go` (hai chỗ gọi), test `flow_close_cancel_queued_test.go`. |
| B7 | Không sửa. Chỉ báo cáo: bảng thành viên squad bị chồng chữ ở 390px. | Ngoài phạm vi flow. | Không. |

### 7.2 Kiểm tra đã chạy

- `pnpm exec vitest run workflow/designer/` (trong `packages/views`): `Test Files 9 passed (9)`, `Tests 58 passed (58)` (trước đó đã đỏ đúng với test B2).
- `pnpm exec vitest run workflow/issue-flow-state-panel.test.tsx`: `Tests 8 passed (8)` (test verdict và test layout đều đã đỏ trước khi sửa).
- `pnpm exec vitest run issues/utils/issue-update-error.test.ts`: `Tests 12 passed (12)`.
- `pnpm exec vitest run issues/components/issue-detail.test.tsx`: `Tests 112 passed (112)`.
- `pnpm exec vitest run locales` (parity 7 locale): `Tests 407 passed` trên 2 file.
- `pnpm exec tsc --noEmit` trong `packages/core` và `packages/views`: không có lỗi.
- `pnpm exec vitest run api issue-workflows` trong `packages/core`: `Tests 703 passed (703)`.
- `pnpm exec vitest run workflow issues/utils issues/components/issue-detail.test.tsx locales` trong `packages/views`: `Tests 823 passed (823)`.
- Go: `go vet ./internal/handler/` sạch (bao gồm các test mới). `gofmt -l` sạch cho các file tôi đổi.
- Không chạy được: `TestFlowTaskKind` và `TestClosedFlowTaskCancelsItsQueuedRuns` (xem 7.3). `TestFlowTaskKind` không cần DB, nhưng `TestMain` của package handler yêu cầu DB, nên không chạy riêng được.

### 7.3 Diễn tập thật với agent kiro (NOT RUN ở đợt đó; đã chạy ở mục 8)

Không bắt đầu được. File `/home/av-server/.cache/claude-delegate/20261113-100000-feature/test-db.env` không còn tồn tại. Thư mục `20261113-100000-feature` cũng đã biến mất khỏi cache, và không còn file `test-db.env` nào ở các vị trí tìm được (`find` trong `.cache`). Không đoán thông tin đăng nhập database, không chạm vào database khác.

Hệ quả: không chạy được backend, nên không đăng ký daemon, không gắn runtime kiro với squad, không tạo request, và không có lượt chạy agent nào. Không có số lượt chạy agent và không có thời gian chạy.

Việc tiếp theo: khôi phục file DSN (hoặc đặt biến `DATABASE_URL` trỏ tới `multica_kv_flow_test` trong môi trường). Khi có, cần chạy `go test -run 'TestFlowTaskKind|TestClosedFlowTaskCancelsItsQueuedRuns' ./internal/handler` qua guard để xác nhận hai test Go mới, rồi mới bắt đầu diễn tập.

Trạng thái môi trường: backend, web và daemon không được khởi động trong đợt này. Thư mục cache `20261113-081623-kiro/bin` có `go-test-kv.sh` (đã kiểm tra guard trước khi chạy test Go).

## 8. Diễn tập thật với agent kiro (squad Import & Export Desk, `flow_kv_trade`)

Kết luận ngắn: luồng chạy đúng từ đầu đến cuối trên web với agent thật cho đến khi hết vòng sửa (dừng 3), và dừng 1 + Resume cũng đúng. Chưa có lần nào đi hết đến Handover và dừng 2, vì bộ chứng từ mẫu có lỗi số liệu thật (mục 8.4) và vì hai điểm thiết kế ở mục 8.6. Tìm được 3 lỗi sản phẩm trong phạm vi flow, đã sửa bằng test đỏ trước (mục 8.5). Tổng số lượt chạy agent là 29, vượt mức 25 đã đặt (mục 8.3).

### 8.1 Môi trường

- Backend: `server/cmd/server` build từ working tree, `APP_ENV=development`, cổng 18770, database `multica_kv_flow_test` (qua `scripts/require-test-database.sh`, in ra `test database: multica_kv_flow_test`), `FF_FLOW_RUNNER=true`, `flow_auto` của workflow bật bằng nút Auto trên UI. Giả định: bật runner vì đây là chế độ tầng 2 mà người dùng sẽ bật; lead chỉ cần lập kế hoạch, server mở task và chuyển bước.
- Web: `pnpm dev` của `apps/web`, cổng 3005, `REMOTE_API_URL` trỏ về 18770. Playwright Chromium, 1280px.
- Daemon thật: `multica daemon start --foreground` (CLI build từ `server/cmd/multica`), profile riêng `kvdrill`, đăng nhập bằng token cá nhân tạo cho tài khoản thử. Để chỉ có kiro: `PATH` chỉ gồm thư mục riêng chứa liên kết tới `kiro-cli` cộng `/usr/local/bin:/usr/bin:/bin`, `MULTICA_CODEX_PATH=/nonexistent/codex` (trong `/usr/bin` có `codex`), `SHELL=/bin/false` để không dò bằng login shell. `daemon probe-runtimes` trả `{"runtime_count":1,"provider_summary":{"kiro":1}}`. Runtime đăng ký: `Kiro (kvdrill-host)`, online. Không CLI agent nào khác được chạy.
- Workspace mới `kv-real` (tiền tố KVR) để không đụng KVD-1 còn dở của đợt trước. Squad tạo bằng Quickstart trên UI; Quickstart tự gắn 7 agent vào runtime kiro duy nhất. Model của cả 7 agent đặt `claude-haiku-5.5` qua `PUT /api/agents/{id}`; trang squad hiện đủ 7 dòng "Kiro (kvdrill-host) · claude-haiku-5.5" (`shots/05-squad-detail.png`).
- Kịch bản: một request nhập khẩu LED (3 dòng hàng, 85 kiện), đính kèm 3 chứng từ (invoice, packing list, B/L) ngay trong hộp thoại tạo request. Invoice cố ý có lỗi số học: dòng 3 ghi 3.500 USD nhưng 250 x 12 = 3.000; tổng ghi 15.350 USD, tổng các dòng 15.750. Bật `customs_lookup` (lead chọn) để có hai task Collect song song. Tất cả file ở `.../20261113-100113-kiro-drill2/` (ảnh trong `shots/`, log trong `logs/`).

### 8.2 Kết quả từng bước (lần chạy 1, request KVR-1)

| Bước | Kết quả | Bằng chứng |
| --- | --- | --- |
| Đăng nhập bằng form, mã lấy từ log backend | PASS | `shots/01-signed-in.png` |
| Quickstart tạo squad + workflow, gắn runtime, đặt model | PASS | `02`, `03`, `04`, `05` |
| Bật Auto trên squad | PASS | `PUT .../flow-auto` 200, `07-workflows-section-auto.png` |
| Tạo request giao cho squad kèm 3 file | PASS | `POST /api/issues` 201, 3 `upload-file` 200, `09-request-created.png`; KVR-1 có `flow` ngay (step `todo`) |
| Lead chạy (1 lượt) và báo dừng 1 | PASS | Lead gọi `flow plan --blocked`; request `blocked`, `stop=1`; thiếu "loại hình" (đúng quy tắc dừng 1 của squad). Lead nêu cả lỗi invoice. `10-stop1-issue.png` |
| Comment trả lời + Resume trên UI | PASS | `POST .../comments` 201, `POST .../flow/resume` 200, step về `todo` (`11`, `12`) |
| Lead lập kế hoạch (`flow plan`, `customs_lookup=true`, `export=false`) | PASS | Kế hoạch đăng, step sang `collect` |
| Collect song song: KVR-2 (docs), KVR-3 (customs) | PASS | Hai lượt chạy bắt đầu cùng giây 03:20:40; KVR-2 xong 03:21:46 kèm `01-thu-thap.md`; KVR-3 xong 03:22:24. Bước chỉ chuyển khi cả hai đóng |
| Draft: KVR-4 đóng với tệp đính kèm | PASS | Guard `flow_attachment_required` thỏa: `02-ban-nhap.md`, `02-dong-hang.csv` |
| Gate song song: KVR-5, KVR-6 có verdict | PASS | Cả hai đóng `done` với outcome `rejected` (CHẶN) trong bảng task; mỗi gate chạy lại cùng lúc |
| Rework vòng 1 và 2 (draft mới, cả hai gate chạy lại) | PASS | `rework=1` rồi `rework=2`; task v2 (KVR-7..9), v3 (KVR-10..12) |
| Dừng 3 sau 2 vòng sửa | PASS | `step=blocked`, `stop=3`; hệ thống đăng comment "BLOCKED · stop 3 · hết 2 vòng sửa" kèm danh sách task. `13-stop3-issue.png` |
| Handover, dừng 2, export | NOT RUN | Không bao giờ tới (cả hai gate CHẶN) |
| Chặn dừng 1 do hệ thống đăng không trùng | FAIL (thiết kế, O1) | Có hai comment dừng 1 (một của lead, một của hệ thống) |
| Panel Flow: bảng task ở 1280px | FAIL (B3 chưa hết, mục 8.5) | Bảng rộng 365px trong khung 225px: Status, Outcome và nút Approve/Reject nằm sau thanh cuộn ngang |
| Nút Approve/Reject trên gate đang mở (B5) | NOT RUN trên trình duyệt | Agent đóng gate trước khi kịp chụp; chỉ có test đơn vị |
| B2 (designer khóa), B4 (thông điệp lỗi guard), B6 (hủy lượt queued) | NOT RUN | Không có thao tác tương ứng trong kịch bản; B6 không phát sinh vì agent tự đóng task |
| B7 (bảng thành viên 390px) | NOT RUN | Ngoài phạm vi; ở 1280px bảng thành viên vẫn bị chồng chữ ở dòng Leader (`05-squad-detail.png`) |

Lượt chạy lần 1: lead 3 (dừng 1, lập kế hoạch, và một lượt thừa là lỗi D1 bên dưới), docs 1, customs 1, drafter 3, gate 6 = 14.

### 8.3 Số lượt chạy agent

- Lần 1 (KVR-1): 14.
- Ngoài kịch bản: 2 lượt lead chạy trên KVR-13 và KVR-14, là các issue "Q4 2026 planning" và "Q1 2027 planning" do tính năng lập kế hoạch công ty tự tạo, giao cho squad, rồi bị flow nhận làm request (mục 8.6, O3). Cùng lúc đó workspace `kv-drill` cũng nhận hai issue tương tự (KVD-6, KVD-7), nhưng agent của nó gắn runtime mô phỏng nên không chạy.
- Lần chạy lại (KVR-16): 13, trong đó 2 bị hủy. Tôi dự định Detach ngay khi gate vòng 0 xong, nhưng vòng lặp theo dõi so khớp sai chuỗi nên bỏ lỡ thời điểm đó và chạy hết vòng 1 (thêm 6 lượt); sau Detach lỗi D3 thêm 2 lượt (KVR-26 và một lượt lead), tôi hủy cả hai.
- Tổng 29, hơn mức 25. Lỗi theo dõi là của script drill (loại hạ tầng), không phải của sản phẩm.

### 8.4 Gate chặn có đúng không

Các gate chặn đúng. Cả hai nêu đúng chỗ: dòng 3 lệch 500 USD, tổng lệch 400 USD, và bản nháp dù đã đánh dấu `[cần xác nhận]` vẫn không đủ để QUA vì người bán chưa xác nhận. Drafter không sửa được chứng từ gốc, nên mỗi vòng sửa chỉ lặp lại cùng kết luận. Hai điểm khiến gate hải quan chặn thêm nằm ngoài lỗi số liệu: task KVR-3 không đính kèm tệp (O2) và knowledge của workspace đang `disabled` nên không có kho mã HS (môi trường thử).

### 8.5 Lỗi sản phẩm tìm được và đã sửa

| Mã | Lỗi | Nguyên nhân | Sửa | Test (đỏ trước, xanh sau) |
| --- | --- | --- | --- | --- |
| D1 | Lead bị gọi thêm một lượt thừa ngay sau khi người dùng trả lời và bấm Resume | `FlowRunStats` chỉ đếm lượt chạy tạo sau `entered_at`. Comment của người dùng đánh thức lead trước, Resume đặt `entered_at` sau, nên lượt đang chạy "biến mất" và sweep (30 giây một lần) thấy `attempts=0` rồi đưa thêm một lượt lead (log: `flow sweep: gave the task a run step=todo`, 21 giây sau Resume) | `server/pkg/db/queries/flow.sql` (+ `flow.sql.go` do `sqlc generate`): lượt đang hoạt động và lượt chạy lâu nhất được đếm bất kể thời điểm tạo; `attempts` và `last_activity` vẫn tính từ `entered_at` | `TestFlowSweep_LeadRunStartedBeforeResumeIsNotDoubled` trong `flow_runner_test.go`: đỏ `runs = 2, want 1`, xanh sau sửa |
| D2 | Panel Flow ở 1280px không hiện Status, Outcome và nút Approve/Reject (B3 "đã sửa" nhưng chỉ chưa đo trên trình duyệt). Task đóng không có outcome hiện "Pending" | Bảng 5 cột rộng 365px trong khung 225px | `packages/views/workflow/issue-flow-state-panel.tsx`: mỗi task là một khối (mã, tên, vòng ở dòng trên; status và outcome hoặc Approve/Reject ở dòng dưới, tự xuống dòng). Task đã đóng không có outcome hiện "Done". Bỏ 5 khóa `flow.table_*` không còn dùng ở 7 locale | `issue-flow-state-panel.test.tsx`: hai test mới (`stacks each task in its own block...`, `marks a closed work task without an outcome as done, not pending`), đỏ trước sửa. Ảnh sau sửa: `shots/14-rerun-gate-open.png` |
| D3 | Detach bị hoàn tác: với runner bật và Auto bật, sweep nhận request vừa Detach làm "request chưa có flow" và đưa lại lên flow trong vòng 30 giây, mở lại task và chạy thêm agent | `ListUnstartedFlowRequests` chọn mọi request của squad flow không có `flow`; Detach xóa `flow` nên khớp | `flow_engine_ops.go` (`DetachFlow` ghi mục `flow_detached`), `flow.sql` (truy vấn bỏ request có mục đó) | `TestFlowSweep_DoesNotRestartADetachedRequest`: đỏ `the sweep put the detached request back on its flow`, xanh sau sửa |

Phát hiện D3 ở lần chạy lại nên chưa kiểm lại được trên runtime thật (chỉ được chạy lại một lần và đã vượt số lượt chạy). D1 và D2 đã kiểm lại ở lần chạy lại (8.7).

### 8.6 Điểm thiết kế đang mở (không sửa)

- **O1. Hai comment dừng 1.** Luật riêng của squad và skill `kv--squad-lead` bảo lead tự đăng comment dừng ("BLOCKED · stop 1 · ...") và đặt trạng thái, trong khi phần chế độ flow nói server đăng comment dừng. Lead làm theo cả hai nên trùng. Cần quyết định: sửa luật squad cho chế độ flow, hoặc để server bỏ qua khi lead đã đăng.
- **O2. Task Collect không bắt buộc tệp.** `docs` và `customs` có hướng dẫn "đính kèm tệp" nhưng không có `requires: ["attachment"]` (đúng spec). Vai `trade_customs_research` lại không có công cụ ghi tệp, nên agent đăng kết quả trong comment (suy nghĩ của nó nêu rõ xung đột này). Gate hải quan không nhận được đầu vào nên luôn chặn, và vòng sửa chỉ làm lại bản nháp, không làm lại Collect. Cần quyết định: cho vai này khả năng tạo tệp, hay đổi hướng dẫn và gate sang đọc comment, kèm `requires` tương ứng.
- **O3. Issue lập kế hoạch công ty bị nhận làm request flow.** Issue do agent tạo, giao cho squad có flow, sẽ được sweep đưa lên flow và đánh thức lead trên việc không phải lô hàng. Cần quyết định: chỉ nhận request do thành viên tạo, hoặc loại issue có nguồn lập kế hoạch.
- **O4. Ghi chú thử lại của lead.** `flowRetryNote` nói "close it with the commands in its description", đúng cho task nhưng sai cho lead (lead không có việc "đóng"). Lượt thừa D1 đã nhận ghi chú này và nói thẳng là nó không có lệnh nào như vậy.
- **O5. Vòng sửa khi lỗi nằm ở chứng từ gốc.** Hai vòng sửa tốn 3 bản nháp và 6 lượt gate để ra cùng một kết luận "cần người bán xác nhận". Cần quyết định: gate nên cho kết luận "cần người quyết" dẫn tới dừng 1 thay vì sửa lại.
- **O6. Detach với `cancel_tasks=true` vẫn đánh thức lead** ("Wakeup: sub-issues closed") vì các sub-issue bị hủy. Một lượt lead chạy trên request đã rời flow (tôi đã hủy).
- **O7. Dọn dẹp của test Go.** `cleanupFlowRows` xóa mọi dòng `agent_task_queue` tạo trong giây vừa qua, không giới hạn workspace; chạy test Go trên cùng database khi đang diễn tập có thể xóa lượt chạy thật. Tôi chỉ chạy test giữa hai lần chạy agent.

### 8.7 Chạy lại một lần (request KVR-16, cùng kịch bản)

- Dừng 1 lần nữa, rồi comment trả lời + Resume trên UI. Lead chạy đúng 2 lượt (so với 3 ở KVR-1): sau Resume, log chỉ có `flow/resume` 11:05:04 và `flow/plan` 11:05:33, không có dòng `flow sweep: gave the task a run`. D1 đã hết.
- Collect (KVR-17, 18) → Draft (KVR-19) → Gate (KVR-20, 21 CHẶN) → vòng 1 (KVR-22, 23, 24) → vòng 2 (KVR-25 bắt đầu). Cùng kết quả như KVR-1 (nên không phải ngẫu nhiên).
- Panel Flow sau sửa D2: cả 9 task hiện đủ mã, vòng, status và outcome trong khung 225px (`shots/14-rerun-gate-open.png`).
- Detach trên UI (`POST .../flow/detach` 200 `{"detached":true}`) lộ ra D3 (mục 8.5).

### 8.8 Kiểm tra đã chạy

- Đỏ trước sửa: `--- FAIL: TestFlowSweep_LeadRunStartedBeforeResumeIsNotDoubled`, `flow_runner_test.go:296: runs = 2, want 1`; `--- FAIL: TestFlowSweep_DoesNotRestartADetachedRequest`; `Tests 2 failed | 7 passed (9)` cho `issue-flow-state-panel.test.tsx`.
- `go test -count=1 -run 'TestFlowSweep|TestFlowRunner' ./internal/handler` sau sửa D1: `ok  github.com/multica-ai/multica/server/internal/handler  9.559s`.
- `go test -count=1 -run 'Flow|Rehearsal|Golden' ./internal/handler` (DATABASE_URL tường minh, qua guard): `ok  github.com/multica-ai/multica/server/internal/handler  30.600s`.
- `go test -count=1 -run 'TestDetach|TestChaos|TestChild|TestAdvance|TestFlowTaskKind|TestClosedFlowTask|TestResume|TestOutcome' ./internal/handler`: `ok ... 13.226s`; `go test -count=1 ./internal/issueworkflow ./internal/scheduler`: `ok` cả hai. `go vet ./internal/handler/` sạch, `gofmt -l` sạch cho các file đã đổi.
- `pnpm exec vitest run workflow/issue-flow-state-panel.test.tsx` (trong `packages/views`): `Tests 9 passed (9)`. `pnpm exec vitest run workflow/designer issues/utils issues/components/issue-detail.test.tsx locales`: `Test Files 20 passed (20)`, `Tests 738 passed (738)`.
- Một lần chạy rộng hơn (`workflow issues/components/issue-detail.test.tsx issues/utils locales`) có 3 test hết thời gian chờ ở 42 giây (`handoff-form`, `workflow-pill`, `workflow-graph-sheet`) khi tải máy là 35; chạy riêng ba file đó: `Tests 13 passed (13)`.
- `pnpm exec tsc --noEmit` trong `packages/views` và `packages/core`: không lỗi. `eslint` trên hai file panel: exit 0. `git diff --check`: sạch.
- Không chạy: `make test`, `make check`, `pnpm test`, Playwright e2e, `pnpm lint` toàn repo.

### 8.9 Trạng thái để lại và việc còn mở

- Đã dừng backend (18770), web (3005), daemon (profile `kvdrill`); đã xóa profile và token của daemon. `ps` chỉ còn hai `next-server` của phiên khác. `apps/web/AGENTS.md` bị `next dev` ghi đè lại, đã khôi phục như HEAD.
- Test DB còn workspace `kv-real` (KVR-1 đến KVR-26, KVR-1 dừng 3, KVR-16 đã detach), runtime kiro offline và dữ liệu `kv-drill` cũ.
- Người dùng cần quyết định O1 đến O6, và có muốn một lần chạy thật thứ ba để kiểm D3 và đi hết đến Handover (cần chứng từ không có lỗi số liệu, và xử lý O2 trước).

## 9. Lần 3: giải quyết O1 đến O7 và diễn tập thật

Kết luận ngắn: bảy mục mở đã được quyết định và sửa (mục 9.1). Lần diễn tập thứ ba với kiro thật chạy đúng đến hết vòng sửa và đến tận bước Handover, nhưng không tới được dừng 2: hai gate (agent thật) luôn CHẶN vì không có nguồn bậc 1 trong môi trường thử, và Handover (agent thật) cũng CHẶN vì hai gate không có dòng `VERDICT`. Lần này tìm thêm một lỗi (comment dừng 1 trống, đã sửa) và hai điểm thiết kế (O8, O9). 20 lượt chạy agent.

### 9.1 O1 đến O7

| Mục | Phương án chọn và lý do | File | Test (đỏ rồi xanh) |
| --- | --- | --- | --- |
| O7 | Sửa trước tiên. `cleanupFlowRows` chỉ xóa lượt chạy của agent thuộc workspace test (`agent_id IN (SELECT id FROM agent WHERE workspace_id = $2)`), thay vì mọi dòng `agent_task_queue` tạo trong giây vừa qua. Lý do: cùng database với diễn tập thì nó xóa được lượt chạy thật. | `server/internal/handler/flow_fixture_test.go` | `TestCleanupFlowRows_LeavesOtherWorkspacesAlone`: đỏ `the run of another workspace was removed (found 0)`, xanh `ok ... 0.826s` |
| O1 | Nguồn duy nhất là lời nhắc do server sinh cho mỗi lượt chạy của lead trên request (`FlowLeadBrief`): server đăng comment dừng và đặt trạng thái, lead không đăng ba tiêu đề `BLOCKED · stop 1`, `BLOCKED · stop 3`, `IN REVIEW · stop 2`, và lời nhắc này thắng luật squad khi workflow đang chạy request. Không sửa skill hay luật squad: chúng đúng cho chế độ cũ (`managed: false`), dùng chung 21 squad, và agent đã tạo từ bản cũ vẫn nhận được lời nhắc mới. Tôi không chọn "server bỏ qua khi lead đã đăng" vì nó phải đọc văn bản comment. | `server/internal/issueworkflow/flow.go` | `TestFlowBrief_LeadLeavesTheStopToTheServer`: đỏ `plan brief is missing "The server posts the stop comment and sets the status"`, xanh. Trên runtime thật: lần 3 có đúng một comment dừng 1 (của hệ thống), lead không đăng comment dừng |
| O2 | Đầu ra của Collect là tệp. Engine giao cho bước sau tệp đính kèm của bước trước nên `docs` và `customs` có `requires: ["attachment"]`; hướng dẫn task nói rõ rằng dù mô tả vai ghi không có công cụ ghi, ở task này vẫn tạo tệp bằng shell. Chỉ đổi manifest Trade. Tôi không chọn "gate đọc comment" vì mô tả task cho gate và drafter nói rõ chỉ xét tệp đính kèm (chống thiên kiến), nên đổi sang comment là đổi hợp đồng của engine. | `catalog/workflows/flow_kv_trade/template.json` | `TestRehearsal_Trade_CollectTasksNeedAFile`: đỏ `expected 422, got 200` (với manifest cũ), xanh. `node scripts/catalog/cli.mjs build` ghi 202 mẫu, `pnpm catalog:check` cho `catalog ok: 202 templates`. Trên runtime thật: `KRT-2` và `KRT-3` đều đóng kèm tệp |
| O3 | Job quét chỉ đưa lên flow request do thành viên tạo (`creator_type = 'member'`). Agent giao việc cho squad qua đường tự khởi tạo (bàn giao giữa squad gọi `StartFlowIfEligible`), còn job lập kế hoạch công ty tạo issue dưới tên lead nên không phải request. Không đổi SDLC/BMAD (chỉ truy vấn quét của flow). | `server/pkg/db/queries/flow.sql`, `flow.sql.go` | `TestFlowSweep_LeavesAgentFiledIssuesOffTheFlow`: đỏ `the sweep put an agent-filed issue on the flow`, xanh. Trên runtime thật: không thấy issue lập kế hoạch nào trong lần 3 (job không chạy trong 1 giờ), nên chỉ kiểm bằng test |
| O4 | Hai lời nhắc: task giữ câu "close it with the commands in its description"; lead nhận `flowRetryLeadNote` đòi kế hoạch và `flow plan` (hoặc `--blocked --missing --reason`). | `server/internal/handler/flow_sweep.go` | `TestFlowSweep_RetryBriefNamesTheJobOfTheRun`: đỏ (lời nhắc của lead còn "close it"), xanh |
| O5 | Giữ rework, không đổi engine. Chuyển tiếp `any_blocked -> blocked` (dừng 1) đã có; chỉ dạy hai gate trong manifest dùng đúng: lỗi nằm ở chính chứng từ gốc và bản nháp đã nêu đúng, drafter không sửa được thì đóng `blocked --outcome blocked` (người xác nhận rồi `resume`), `rejected` chỉ khi drafter sửa được. Không đổi giới hạn 2 vòng. Lý do: giữ hành vi engine như spec, chỉ chọn đường đã có; thay đổi nằm trong văn bản gate của Trade. | `catalog/workflows/flow_kv_trade/template.json`, spec mục 3.6 | `TestRehearsal_Trade_GateInstructionsSeparateRejectedFromBlocked`: đỏ, xanh; `TestRehearsal_Trade_GateBlockedOnTheSourcesStopsAtOne` xanh (đường engine đã có). Trên runtime thật: gate hải quan tự đóng `blocked` (xem 9.3) |
| O6 | Kiểm tra, không cần sửa mã. Detach với `cancel_tasks=true` hủy task mở và lượt chạy của chúng và không đánh thức lead. Lượt lead thừa ở lần chạy lại là `child_done` của chế độ cũ: ở lần đó `detach` trước đó dùng `cancel_tasks=false`, nên task chạy tiếp như sub-issue thường rồi tự đóng, và đó đúng thiết kế (spec, phần detach). Đã ghi vào spec. Chặn tái phát bằng test hồi quy. | spec (API detach) | `TestDetachCancelTasks_DoesNotWakeTheLead` xanh ngay khi viết (không đỏ vì hành vi đã đúng) |

Thêm: lỗi mới ở 9.3 (D4).

### 9.2 Môi trường lần 3

- Backend `server/cmd/server` build từ working tree (chứa `65ae949d5` và các sửa trên), cổng 18770, database `multica_kv_flow_test` (guard in `test database: multica_kv_flow_test`), `FF_FLOW_RUNNER=true`; web `pnpm dev` cổng 3005. Workspace mới `kv-real-3` (tiền tố KRT). Backend được khởi động lại một lần giữa chừng để nhận sửa D4.
- Daemon thật `multica daemon start --foreground`, profile `kvdrill3`, `PATH` chỉ có liên kết tới `kiro-cli` cộng `/usr/local/bin:/usr/bin:/bin`, `MULTICA_CODEX_PATH=/nonexistent/codex`, `SHELL=/bin/false`. `daemon probe-runtimes`: `{"probe_result":"success","runtime_count":1,"provider_summary":{"kiro":1}}`. Runtime `Kiro (kvdrill3-host)`.
- Squad tạo bằng Quickstart trên UI. API xác nhận cả 7 agent: model `claude-haiku-5.5` và đúng runtime `Kiro (kvdrill3-host)`. Workflow `flow_kv_trade@84e11b9a` có `blocked --outcome blocked` trong gate và `requires: attachment` trên Collect. Auto bật trên UI.
- Chứng từ lần 3 (sửa lỗi lần trước): invoice đã hết lỗi (dòng 3 = 3.000, tổng 15.250), loại hình A11 nằm trong mô tả request, thêm C/O Form E và một "hồ sơ đơn vị" tự soạn làm kho mã HS và tham số đã xác nhận. Dữ liệu tổng hợp, do tôi viết cho diễn tập.

### 9.3 Kết quả từng bước (KRT-1)

| Bước | Kết quả | Bằng chứng |
| --- | --- | --- |
| Quickstart, gắn runtime, đặt model, bật Auto, tạo request kèm 5 file | PASS | `shots/` và API ở 9.2; `KRT-1` có flow ngay |
| Lead lập kế hoạch (một lượt, không dừng 1 vì đủ hồ sơ) | PASS (agent thật) | 1 lượt lead; step sang `collect`. Không có lượt lead thừa |
| Collect song song, đóng kèm tệp | PASS (agent thật) | KRT-2 và KRT-3 bắt đầu cách nhau 1 giây, đều đóng với tệp (O2 chạy được) |
| Draft kèm tệp, gate song song | PASS (agent thật) | KRT-4; KRT-5 `rejected`, KRT-6 tự đóng `blocked` (lỗi nằm ở nguồn tham số) |
| Gate `blocked` dẫn tới dừng 1 | PASS | `stop=1` ngay khi KRT-6 `blocked`, không tốn vòng sửa (O5). Một comment dừng (O1) |
| Nút Approve/Reject trên gate đang mở trong panel Flow | PASS (UI) | `shots/20-gate-open.png`: dòng KRT-6 hiện Approve và Reject. `shots/40-final-stop3.png`: 18 dòng task trong khung 225px, `scrollWidth = clientWidth = 225`, không cuộn ngang (D2 đã sửa) |
| Comment dừng 1 nêu cái thiếu | FAIL trước sửa, PASS sau sửa (D4) | Lần đầu comment có "Thiếu:" trống và "Lý do:" trống; sau khi khởi động lại backend có `- KRT-6 · customs_gate: VERDICT: CHẶN - ...` và "Lý do: The task customs_gate reported it cannot go on" |
| Trả lời kèm tệp rồi Resume trên UI | PASS một phần | Tệp đính kèm lên được và `flow/resume` 200, nhưng comment văn bản không đăng được (script dùng Ctrl+Enter sau khi chọn tệp; lỗi script, không phải sản phẩm). Resume chạy lại đúng gate đã `blocked` (KRT-6 lượt 2) trên bản nháp cũ và cho cùng kết luận: 1 lượt lãng phí (O9) |
| Resume với `to_step=draft` (API) | PASS | `step=draft`, task soạn mở ở vòng mới, `rework` giữ 0; sau đó hai gate chạy lại |
| Rework vòng 1 và 2, mỗi vòng hai gate | PASS (agent thật) | `rework=1` rồi `rework=2`, mỗi vòng drafter + 2 gate |
| Gate QUA | NOT RUN với agent | Mọi vòng gate của agent (5 vòng) đều CHẶN, đều với lý do thiếu nguồn bậc 1 (thuế suất, tỷ giá, mã 8 số). Môi trường không có mạng tới biểu thuế, knowledge của workspace tắt, và hồ sơ đơn vị tự soạn không được gate coi là nguồn bậc 1 |
| Thành viên ghi verdict trên hai gate bằng nút Approve (UI) | PASS | `shots/30-gates-open-before-approve.png`, `31-gates-approved.png`: bốn `PUT` 200. Script cũng bấm nhầm hai gate `blocked` của vòng cũ vì panel vẫn hiện nút trên chúng (O8). Hai lượt chạy gate đang chạy bị tôi hủy (`CANCEL ... 200`) |
| Handover chạy (agent thật) và đưa ra kết luận | PASS (agent thật) | KRT-19; chạy sau khi hai gate `approved`; kết luận `rejected` vì hai gate "không có dòng `VERDICT`" (chúng do người ghi bằng nút, không có comment). Đúng quy tắc của handover |
| Dừng 3 sau Handover không QUA | PASS | `stop=3`; comment hệ thống nêu KRT-19 `done · rejected` |
| Dừng 2, Excel export | NOT RUN | Không tới được |

Số lượt chạy agent: 20 (lead 1, docs 1, customs 1, drafter 5, gate 11 gồm hai lượt bị hủy, handover 1). Thời gian: tạo request 12:19, dừng cuối 13:07 (48 phút). Mức 25 không bị vượt.

### 9.4 Lỗi mới và điểm thiết kế mới

- **D4 (lỗi, đã sửa).** Comment dừng 1 do task `blocked` (gate) có "Thiếu:" và "Lý do:" trống, vì chỉ `plan --blocked` của lead điền hai trường này. Từ khi gate được dạy dùng `blocked` (O5) đây là đường chính. Sửa: `blockedTaskNote` trong `flow_engine.go` (điền từ dòng đầu của comment mới nhất của agent trên task `blocked`), truy vấn `LastAgentCommentOnIssue` trong `flow.sql`. Test `TestRehearsal_Trade_StopOneFromABlockedTaskSaysWhatIsMissing`: đỏ (comment trống), xanh. Đã kiểm lại trên runtime thật (bảng 9.3).
- **O8 (mở).** Panel Flow hiện nút Approve/Reject trên mọi gate chưa đóng, kể cả gate `blocked` của vòng cũ. Một lần bấm nhầm ghi verdict lên lịch sử. Gợi ý: chỉ hiện trên gate `todo` hoặc `in_progress` của vòng hiện tại.
- **O9 (mở).** Resume dừng 1 do gate `blocked` chạy lại đúng gate đó trên bản nháp cũ, đọc được tệp mới của request nhưng không thấy bản nháp đổi; nếu nguồn mới phải vào bản nháp thì phải `resume` với `to_step` là bước soạn (API và CLI có, nút Resume trên UI chưa có ô chọn bước). Chưa đổi engine vì cả hai lựa chọn đều đúng tùy cái người dùng bổ sung; cần quyết định: thêm ô chọn bước vào nút Resume, hay đặt mặc định quay về bước soạn khi task `blocked` là gate.
- **Nhận xét môi trường, không phải lỗi sản phẩm.** Gate hải quan và gate chứng từ yêu cầu nguồn bậc 1 có thể truy cập (biểu thuế, thông báo tỷ giá). Trong sandbox không có mạng và knowledge tắt, nên không có cách trung thực để gate QUA. Muốn đi hết đến dừng 2 với agent thật cần môi trường có nguồn thật (bật knowledge với hồ sơ đơn vị thật, hoặc cho runtime ra mạng). WARN trong log backend chỉ có hai dòng 404 `GET /api/attachments/...` do gate cố tải tệp 05 mà tôi đã xóa khỏi request (lỗi của script drill).

### 9.5 Kiểm tra đã chạy

- `go test -count=1 -run 'Flow|Rehearsal|Golden' ./internal/handler`: `ok  github.com/multica-ai/multica/server/internal/handler  34.206s`.
- `go test -count=1 -run 'Golden|IssueWorkflow' ./internal/handler`: `ok ... 0.888s`; `go test -count=1 ./internal/issueworkflow ./internal/scheduler ./internal/catalog ./internal/service`: `ok` cả bốn (service `46.551s`).
- `go test -count=1 -run 'TestDetach|TestChaos|TestChild|TestAdvance|TestCleanup|TestClosedFlowTask|TestResume|TestOutcome' ./internal/handler`: `ok ... 6.148s`.
- `go test -count=1 -p 1 ./internal/handler` (cả package): `ok ... 161.126s`.
- `go vet ./internal/handler ./internal/issueworkflow`: sạch. `gofmt -l` sạch cho các file đã đổi. `git diff --check`: sạch.
- `pnpm --filter @multica/core exec vitest run catalog issue-workflows`: `Test Files 30 passed (30)`, `Tests 539 passed (539)`. `node --test scripts/catalog/*.test.mjs`: `tests 37, pass 37, fail 0`. `pnpm catalog:check`: `catalog ok: 202 templates`.
- Không đổi `packages/views` trong lần này nên không chạy lại vitest của views (kết quả mục 8.8 còn đúng).

### 9.6 Trạng thái để lại

- Đã dừng backend, web, daemon `kvdrill3`; xóa profile và token. `apps/web/AGENTS.md` khôi phục như HEAD. Hai `next-server` và daemon `3399571` của phiên khác còn nguyên.
- Test DB còn workspace `kv-real-3` (KRT-1 đến KRT-19, KRT-1 ở dừng 3), runtime kiro offline.
- Việc người dùng cần quyết định: O8, O9, và có muốn bật nguồn thật (knowledge hoặc mạng) để chạy lại đến dừng 2. 20 manifest flow còn lại chưa nhận thay đổi O2/O5 của Trade.

