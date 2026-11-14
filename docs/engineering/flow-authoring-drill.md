# Diễn tập trình duyệt: soạn flow (P21)

Báo cáo này ghi lần chạy trình duyệt thật cho trình soạn flow ở 1280px và 390px. Hướng dẫn vận hành nằm ở [flow-authoring-runbook.md](flow-authoring-runbook.md). Đặc tả: [flow-engine-generalization-spec.md](flow-engine-generalization-spec.md), mục 11.

Kết quả: spec `e2e/flow-designer-layout.spec.ts` **đạt** (`1 passed (44.2s)`) sau khi sửa 5 lỗi tìm thấy trong lúc diễn tập (mục 5). Lần chạy lại sau công việc chạy thử, chọn workflow và mẫu bước (mục 7) cũng đạt.

## 1. Môi trường

Chỉ dùng các cách ở [kitviet-flow-web-drill.md](kitviet-flow-web-drill.md) mục 1. Không dùng `make up`, `make dev`, `make test`.

| Phần | Cách chạy |
| --- | --- |
| Backend | Binary build từ `server/cmd/server`, chạy bằng `env -i` (môi trường sạch), `PORT=18780`, `APP_ENV=development`, `FF_FLOW_AUTHORING=true`, `FF_FLOW_GRAPH_V2=true` (chỉ qua biến môi trường), `JWT_SECRET` tạm, `LOCAL_UPLOAD_DIR` trong `/tmp`. `FF_FLOW_RUNNER` giữ mặc định (tắt). |
| Database | `multica_engine_test`, qua `DATABASE_URL` đặt tường minh. Không dùng database `multica`. |
| Web | `cd apps/web && pnpm dev` (`next dev --webpack`) với `FRONTEND_PORT=3010`, `REMOTE_API_URL=http://localhost:18780`, `NEXT_PUBLIC_API_URL=` và `NEXT_PUBLIC_WS_URL=` rỗng. |
| Trình duyệt | Chromium của Playwright, dự án `chromium`. |
| Runtime giả | `api.seedRuntime` đăng ký một runtime, kèm `setInterval` cập nhật `agent_runtime.last_seen_at` (chỉ heartbeat, không claim task, không chạy CLI agent nào). |
| Dữ liệu | `TestApiClient` tạo workspace, squad, lead và 7 agent vai. Không gọi dịch vụ ngoài, không chạy agent thật. |

### Xử lý DSN

Mật khẩu được đọc ngay trong lệnh, không ghi vào file hay báo cáo này, và đầu ra được lọc qua `sed` trước khi hiện:

```bash
PW="$(grep -m1 '^POSTGRES_PASSWORD=' .env | cut -d= -f2- | tr -d '"\r')" \
  && export DATABASE_URL="postgres://multica:${PW}@localhost:5432/multica_engine_test?sslmode=disable" \
  && bash scripts/require-test-database.sh    # in "test database: multica_engine_test"
```

Mọi lệnh có DB chạy `require-test-database.sh` trước, nối bằng `&&`. Không có CREATE hay DROP DATABASE.

### Lệnh chạy spec

Mọi biến đặt tường minh trong một script tạm (đã xóa sau khi chạy):

```bash
export NEXT_PUBLIC_API_URL=http://localhost:18780 PORT=18780
export PLAYWRIGHT_BASE_URL=http://localhost:3010 FRONTEND_ORIGIN=http://localhost:3010
export MULTICA_DEV_VERIFICATION_CODE=""      # xem lưu ý dưới
export FLOW_DESIGNER_SHOTS=<thư mục ảnh>
pnpm exec playwright test e2e/flow-designer-layout.spec.ts --project chromium
```

Lưu ý: `.env.worktree` đặt sẵn một mã xác minh cố định. Với mã đó `TestApiClient.login` hỏng, nên spec phải chạy với `MULTICA_DEV_VERIFICATION_CODE` rỗng. Hai tiến trình `next-server` của phiên khác (16.3.6 và 16.4.3) được để nguyên. Sau lần chạy, các cổng 18780 và 3010 đã đóng, binary và thư mục tạm đã xóa.

Đầu ra lần chạy cuối (sau mục 7):

```
Running 1 test using 1 worker
  ✓  1 [chromium] › e2e/flow-designer-layout.spec.ts:130:3 › Flow draft editor, laptop and phone › builds, checks, simulates, publishes and activates the research flow without sideways scroll (42.9s)
  1 passed (44.2s)
```

## 2. Kịch bản của spec

1. Tạo squad, lead và 7 agent vai bằng `TestApiClient`.
2. Ở 1280px: trang squad, New workflow, hộp thoại bắt đầu, Import a file với manifest 14 bước, vào trình soạn.
3. Gắn agent vào từng vai bằng ô chọn, bấm Check, Simulate (Run), Publish (tick ack theo yêu cầu của server), Make active. Trang squad hiện `v1` và `Active`.
4. Ở 390px: tải lại trang squad và trình soạn, mở một bước (Extra 3), duyệt các tab Graph, Roles, Variables, Loops, Stop reasons, Terminals.

Sau mỗi màn, spec đo `document.documentElement.scrollWidth` và `body.scrollWidth` so với `clientWidth` và fail nếu lớn hơn.

## 3. Kết quả theo màn hình

| Kiểm tra | 1280px | 390px |
| --- | --- | --- |
| Trang squad, danh sách workflow | Đạt, không cuộn ngang | Đạt, không cuộn ngang |
| Trình soạn với 14 bước | Đạt | Đạt |
| Inspector của bước | Cột cố định bên phải | Mở thành Sheet dạng dialog, ô `Step title` = `Extra 3` |
| Tab Graph, Roles, Variables, Loops, Stop reasons, Terminals | Đạt (Graph, Roles) | Đạt cả sáu tab, không cuộn ngang |
| Hộp thoại Simulate, Publish | Đạt | Không kiểm |
| Check, Simulate, Publish, Make active gọi API thật | Đạt | Không kiểm |

Khung đồ thị (canvas) có cuộn nội bộ khi sơ đồ rộng. Đó là cuộn bên trong khung, không phải cuộn trang, và spec chỉ đo cuộn trang.

### Gọi mạng được kiểm

| Gọi | Kiểm tra |
| --- | --- |
| `POST /api/squads/{id}/flow-drafts` | Được gọi khi Import (spec assert) |
| `GET /api/squads/{id}/flow-drafts/{draftId}` | Được gọi khi mở trình soạn |
| `GET /api/flow/capabilities` | Được gọi |
| `PATCH /api/squads/{id}/flow-drafts/{draftId}` | Được gọi khi sửa vai (autosave) |
| `POST .../flow-drafts/{draftId}/validate` | Chờ phản hồi; không có issue `error` nào |
| `POST .../flow-drafts/{draftId}/simulate` | Chờ phản hồi; `hops` hơn 5; hộp thoại hiện đúng kết thúc của trace |
| `POST .../flow-drafts/{draftId}/publish` | Mã 201, `version` = 1 |
| `POST /api/squads/{id}/workflow` | Mã 200 |

## 4. Ảnh chụp

Thư mục: `/home/av-server/.cache/claude-delegate/20261113-140138-feature/p21-screens/` (19 ảnh).

| Ảnh | Nội dung |
| --- | --- |
| `01-squad-workflows-1280.png` | Trang squad, mục workflow |
| `02-start-dialog-1280.png` | Hộp thoại bắt đầu: Blank, Import |
| `03-import-dialog-1280.png` | Hộp thoại Import |
| `04-editor-14-steps-1280.png` | Trình soạn với 14 bước |
| `05-graph-1280.png` | Tab Graph |
| `06-roles-bound-1280.png` | Roles đã gắn agent |
| `07-check-1280.png` | Sau Check |
| `08-simulate-1280.png` | Kết quả Simulate |
| `09-publish-dialog-1280.png` | Hộp thoại Publish |
| `10-published-active-1280.png` | `v1` và Active trên trang squad |
| `11-squad-workflows-390.png` | Trang squad ở 390px |
| `12-editor-390.png` | Trình soạn ở 390px |
| `13-inspector-sheet-390.png` | Inspector dạng Sheet (đã chờ hết animation, nền đục) |
| `14-graph-390.png` | Graph ở 390px |
| `15-roles-390.png` | Roles |
| `16-vars-390.png` | Variables |
| `17-loops-390.png` | Loops |
| `18-stops-390.png` | Stop reasons |
| `19-terminals-390.png` | Terminals |

## 5. Lỗi tìm thấy và cách sửa

Spec không bị sửa để cho qua; mỗi lỗi được sửa trong code rồi chạy lại.

| # | Lỗi | Cách sửa |
| --- | --- | --- |
| 1 | Next báo lỗi build: "importing a module that depends on useState into a React Server Component". 14 file của trình soạn thiếu `"use client"` (12 panel cộng `draft-status.tsx` và `use-draft-buffer.ts`). Test jsdom không bắt được. | Thêm `"use client";` ở dòng đầu mỗi file. |
| 2 | Ở 390px inspector nằm chen trong trang, chật. Spec 11.4 yêu cầu Sheet. | `flow-draft-editor-page.tsx` dùng `useIsMobile` và `Sheet`; `Outline` thêm `fullWidth`. |
| 3 | Không gắn được agent vào vai (không có ô chọn), nên không xuất bản được. | `roles-panel.tsx` thêm ô chọn agent và thành viên, pool sửa được, cột `roles.col_assigned`. Trang nạp lựa chọn từ `companySquadMembersOptions`, `agentListOptions`, `memberListOptions`, bỏ squad leader. |
| 4 | Một mã dừng thêm trên UI không có thông điệp, nên P01 chặn xuất bản. | `stops-panel.tsx` thêm ô sửa thông điệp và prop `onChangeMessages`; thêm mã dừng sẽ gán sẵn một thông điệp mặc định. |
| 5 | Lần dựng 14 bước đầu vi phạm S06 (14 trạng thái, tối đa 12). | Sửa ở dữ liệu thử: sáu bước "Extra" dùng chung trạng thái `in_progress`. Quy tắc S06 đúng, không đổi. |

### Quan sát, chưa sửa

- Nút "Close" ở chân hộp thoại trùng với nút X.
- Với flow nghiên cứu, validate chỉ có cảnh báo dùng chung trạng thái và không cần ack nào, nên đường xin ack khi xuất bản được phủ bởi test đơn vị chứ không phải bởi lần diễn tập này.
- Lần đầu chạy sau khi sửa code, trang có thể trắng trong lúc `next dev` biên dịch nguội. Chạy lại thì qua.

## 6. Sai khác so với đặc tả

- `research.json` có 8 bước, không phải 14. Spec ghép thêm sáu bước pass-through "Extra N" (trạng thái `{key:"in_progress"}`) để có 14 bước, vì S06 giới hạn 12 trạng thái khác nhau. Manifest được nhập qua hộp thoại Import của UI.
- Ngưỡng Sheet là 768px (`useIsMobile` của app), không phải 640px như spec 11.4 (đã ghi vào spec). Danh sách đồ thị dọc vẫn đổi ở 640px (`sm:`).
- Lựa chọn Kit Việt chỉ hiện khi có dòng workflow catalog (spec 11.8 đã ghi sai khác).
- `step-templates.ts` ở vòng đầu chưa được nối vào đâu. Vòng sau spec 11.x được đối chiếu (nó liệt kê năm mẫu bước), nên mẫu được viết lại cho khớp bộ kiểm thật và nối vào menu "Add step" (mục 7).

## 7. Chạy thử, chọn workflow và mẫu bước (vòng sau)

Cùng môi trường, cùng spec. Sau khi bản v1 đã xuất bản và đang dùng, spec làm thêm:

| Bước | Kết quả |
| --- | --- |
| Xuất bản flow thứ hai ("Newsletter flow", một lineage riêng) bằng API: tạo nháp từ manifest, gắn agent vào vai bằng `PATCH`, `POST publish` | 201 |
| Ở 390px, form tạo issue: giao cho squad, ô chọn "Workflow" hiện vì squad có hai flow; menu liệt kê "Research flow · v1 (active)" và "Newsletter flow · v1" | Đạt, không cuộn ngang. Ảnh `20-create-form-workflow-chooser-390.png` |
| Chọn Newsletter, tạo issue: thân `POST /api/issues` có `flow_workflow_id`; `GET /api/issues/{id}/flow` trả `workflow_id` đúng và `trial = false` | Đạt |
| Trang squad ở 390px có hai nút "Trial run", không cuộn ngang; bấm một nút, nhập tiêu đề, Start: thân `POST /api/issues` có `flow_trial: true` và `assignee_id` của squad; trang chuyển sang request; `GET /flow` trả `trial = true` | Đạt |
| Mở thanh bên phải của request (trên điện thoại thuộc tính nằm sau nút này): panel Flow có huy hiệu "Trial" cạnh tên workflow | Đạt. Ảnh `21-trial-badge-390.png` |
| Ở 1280px, nháp mới: Add step → Gate, rồi Check bằng validator thật | Bước `gate_1` chỉ có một lỗi `step_unreachable` (chưa nối cạnh nào vào), và cảnh báo `status_shared` (S07: dùng chung trạng thái `in_review`). Ảnh `22-gate-template-checked-1280.png` |

Ghi chú khi viết bước này:

- Phím tắt `c` không mở form tạo issue ở 390px, và hàm `preferManualCreateMode` của các spec khác chờ chữ "Issues" của sidebar mà điện thoại không hiện. Spec đặt `multica_create_mode` trong `localStorage` rồi tải lại trang.
- Mở thanh bên phải khi request còn đang tải thì nó đóng lại. Spec bấm nút cho tới khi panel ở lại.
- Ảnh đầu tiên của ô chọn workflow chụp lúc danh sách người nhận việc còn mở; spec đợi nó đóng rồi mới chụp.
- Manifest không mang theo `bind`, nên flow thứ hai phải gắn agent bằng `PATCH` trước khi xuất bản (nếu không: 422 `flow_draft_invalid`, `role_unbound`).

Ảnh thêm (22 ảnh tất cả, cùng thư mục):

| Ảnh | Nội dung |
| --- | --- |
| `20-create-form-workflow-chooser-390.png` | Form tạo issue, ô chọn workflow đang mở |
| `21-trial-badge-390.png` | Panel Flow của request chạy thử, huy hiệu Trial |
| `22-gate-template-checked-1280.png` | Bước Gate từ mẫu, sau Check |
