# Runbook soạn flow (flow authoring)

Tài liệu này dành cho người vận hành và người soạn flow. Đặc tả nằm ở [flow-engine-generalization-spec.md](flow-engine-generalization-spec.md). Runbook của các flow Kit Việt có sẵn nằm ở [kitviet-runbook.md](kitviet-runbook.md), mục 8f trỏ về đây. Báo cáo diễn tập trình duyệt nằm ở [flow-authoring-drill.md](flow-authoring-drill.md).

Phạm vi: soạn bản nháp, kiểm tra, mô phỏng, xuất bản, đặt phiên bản đang dùng, chạy thử một phiên bản, chọn workflow khi tạo request, quay lui. Số liệu theo phiên bản (spec 16.6) chưa được cài.

## 1. Cờ tính năng

| Cờ | Mặc định | Tác dụng khi tắt |
| --- | --- | --- |
| `FF_FLOW_ENGINE` | bật | Engine flow không chạy. Request đang chạy đứng yên. |
| `FF_FLOW_RUNNER` | tắt | Không tự chạy `flow_auto`. Lead hoặc người dùng tự gọi `plan` và `advance`. |
| `FF_FLOW_AUTHORING` | tắt | Các route bản nháp (`/flow-drafts`), `POST /api/flow/validate` và `GET /api/issue-workflows/{id}/flow-manifest` trả `404 flow_authoring_disabled`. |
| `FF_FLOW_GRAPH_V2` | tắt | Tạo nháp từ manifest và xuất bản (`POST .../publish`) trả `404 flow_graph_v2_disabled`. Các thao tác nháp còn lại (sửa, kiểm tra, mô phỏng) vẫn chạy. |

Cờ chỉ đặt qua biến môi trường của tiến trình server (không có UI). Đổi cờ cần khởi động lại server. Màn hình đọc `GET /api/flow/capabilities` để biết server hỗ trợ gì (giới hạn, loại bước, dữ kiện, tính năng) nên không phải đoán theo cờ.

Hai cờ cần bật cùng nhau để soạn đầy đủ: `FF_FLOW_AUTHORING=true` và `FF_FLOW_GRAPH_V2=true`.

## 2. Quyền

| Thao tác | Ai được làm | Từ chối |
| --- | --- | --- |
| Đọc, tạo, sửa, xóa, kiểm tra, mô phỏng, xuất bản nháp | Người quản lý squad (`canManageSquad`: owner, admin của workspace, hoặc người quản lý squad) | Người không quản lý: `403 flow_forbidden` (nháp), `403 flow_publish_forbidden` (xuất bản). Agent: `403 flow_authoring_member_only`. |
| `POST /api/flow/validate` | Thành viên và agent | Giới hạn 30 lần mỗi phút cho mỗi actor, đếm trong bộ nhớ của từng instance. Vượt giới hạn: `429 flow_validate_rate_limited`. |
| Đặt phiên bản đang dùng (`POST /api/squads/{id}/workflow`) | Như quản lý workflow của squad hiện có | Theo quyền hiện có. |
| Tick ack `unattended_release`, `allow_unattended_release`, `lead_may_work` | Admin (owner hoặc admin của workspace) | Người không phải admin xin ack này: `403 flow_ack_forbidden`. |
| Tick ack `no_gate` | Người quản lý squad | |
| Tạo trạng thái (status) mới khi xuất bản | Admin | `403 status_create_forbidden`. |
| Xuất bản flow có agent gắn vào vai | Người xuất bản phải được dùng agent đó | `403 flow_agent_forbidden`. |
| Thoát thủ công, `resume`, `detach` trên request | Người vận hành request (`authorizeFlowOperator`) | Theo quyền hiện có. |

`POST /api/flow/validate` nhận phong bì manifest `{"format":"multica.flow-manifest/v1","name":"...","doc":{...}}`, kiểm tra nghiêm ngặt. Phong bì sai: `400 flow_manifest_invalid`. Có thể thêm `squad_id` để kiểm tra danh sách vai với squad đó; squad không tồn tại: `404`.

## 3. Quy trình xuất bản

1. Tạo bản nháp: `POST /api/squads/{id}/flow-drafts` (rỗng, hoặc với `from`: `{kind:"fork"}` từ phiên bản có sẵn, `{kind:"manifest"}` từ manifest; `{kind:"proposal"}` chưa hỗ trợ). Mỗi squad tối đa 20 bản nháp (`409 flow_limit_reached`). Fork từ workflow schema 1 dùng `Lift`: vai có khóa `a_<8 hex>`, không giữ `template_ref`, thông điệp dừng và thông báo mặc định được điền.
2. Sửa: `PATCH .../flow-drafts/{draftId}` kèm `revision`. Thiếu: `400 flow_draft_revision_required`. Lệch: `409 flow_draft_conflict`.
3. Kiểm tra: `POST .../validate`. Kết quả trong phần 5.
4. Mô phỏng: `POST .../simulate` (phần 6).
5. Xuất bản: `POST .../publish` với thân `{revision, ack: [...], make_active}`. Máy chủ xét theo đúng thứ tự này và dừng ở lỗi đầu tiên:
   1. `revision` không khớp: `409 flow_draft_conflict`.
   2. Bản gốc đã có phiên bản mới hơn: `409 flow_draft_stale` kèm `latest_workflow_id`. Fork lại từ phiên bản mới.
   3. Không qua kiểm tra: `422 flow_draft_invalid`, trả toàn bộ `issues`.
   4. Thiếu ack: `422 flow_ack_required` kèm `required` (danh sách ack cần tick).
   5. Quyền, agent, trạng thái mới, tên trùng (`409 workflow_name_taken`), giới hạn 50 phiên bản mỗi dòng (`409 flow_limit_reached`).
6. Thành công: `201` với phiên bản mới (khóa). **Server không đặt phiên bản đang dùng**, kể cả khi thân có `make_active` (cờ này chỉ là gợi ý cho client). Client gọi tiếp `POST /api/squads/{id}/workflow` với `workflow_id` của phiên bản. UI làm hai bước này liền nhau (Publish rồi Make active).

`flow_auto` không được kế thừa sang phiên bản mới. Bật lại bằng `PUT /api/issue-workflows/{id}/flow-auto` nếu cần.

Phiên bản đã xuất bản bị khóa: sửa trả `409 workflow_locked`; workflow do catalog quản lý trả `409 workflow_managed_by_template`. Muốn đổi thì fork thành bản nháp mới và xuất bản.

### Ack (xác nhận có chủ đích)

Một số cấu hình chỉ xuất bản được khi người xuất bản xác nhận rõ ràng. Server báo ack cần thiết bằng `flow_ack_required` và trường `required`; trong `issues[]`, lỗi `code: "ack_required"` mang `params.ack`.

Tên gửi trong `ack`: `unattended_release` hoặc `allow_unattended_release` (cùng một ack), `lead_may_work`, `no_gate`.

| Ack | Ý nghĩa | Ai tick |
| --- | --- | --- |
| `unattended_release` | Có đường thoát khỏi workspace không qua người. | Admin |
| `allow_unattended_release` | Cho phép đường đó chạy không người giám sát. | Admin |
| `lead_may_work` | Lead được nhận task làm việc. | Admin |
| `no_gate` | Flow không có cổng kiểm duyệt. | Người quản lý squad |

Tên ack lạ: `400 flow_ack_unknown`. Client chỉ gửi ack sau khi người dùng tick, không tick sẵn.

### Ack khi bỏ qua checkpoint lúc resume (spec 4.8)

`POST /api/issues/{id}/flow/resume` với request schema 2 đang dừng mã `stalled` và còn công việc chưa được xem (`unreviewed`) cần `acknowledge_skip_checkpoint: true`. Thiếu cờ: `422 flow_checkpoint_ack_required`, trường `field` là `step`, request không đổi. Có cờ: server ghi một checkpoint (`step`, `activation`, `by`, `at`) và xóa `unreviewed`. Các lần resume khác (dừng mã 1 hoặc 3, schema 1) không cần cờ. CLI: `multica issue flow resume --acknowledge-skip-checkpoint`. UI hiện hộp thoại với một ô tick.

## 4. Quay lui

| Tình huống | Việc cần làm |
| --- | --- |
| Phiên bản mới có lỗi nhưng chưa có request nào chạy | `POST /api/squads/{id}/workflow` với phiên bản trước. Request mới dùng phiên bản đó. |
| Có request đang chạy trên phiên bản lỗi | Request ghim phiên bản lúc bắt đầu và chạy tiếp theo phiên bản đó. Đổi phiên bản đang dùng không đụng đến chúng. Request cần thoát thì dùng thoát thủ công (`POST /flow/exits/{edgeId}`) hoặc `detach`. |
| Ngừng soạn mới ngay | Tắt `FF_FLOW_AUTHORING` và khởi động lại. Phiên bản đã xuất bản và request đang chạy vẫn chạy. |
| Ngừng xuất bản và nhập manifest | Tắt `FF_FLOW_GRAPH_V2`. Phiên bản schema 2 đã xuất bản vẫn chạy; `CreateIssueWorkflow` luôn từ chối `flow_config.version >= 2` bằng `400 flow_schema_unsupported`. |
| Gỡ hẳn lược đồ | Migration 988 đến 993: chạy `down` theo thứ tự ngược (993 trước). Sao lưu trước. Không có khóa ngoại hay cascade; dữ liệu phụ thuộc tự dọn trong code. |

## 5. Đọc `issues[]` từ validate

`POST .../validate` và `POST /api/flow/validate` trả `{issues: [...], analysis: {...}}`.

Mỗi phần tử trong `issues`:

| Trường | Nội dung |
| --- | --- |
| `code` | Mã lỗi ổn định, ví dụ `ack_required`. Dùng để xử lý trong code. |
| `severity` | `error` chặn xuất bản. `warning` và `info` không chặn. |
| `path` | Vị trí trong tài liệu, ví dụ một bước, một cạnh, một vai. UI dùng để nhảy tới đúng chỗ. |
| `params` | Tham số của lỗi. Luôn có `rule` (ví dụ `R03`, `S06`, `F01`, `P01`: nhóm quy tắc trong spec 6). Lỗi ack có `params.ack`. |
| `message` | Câu mô tả cho người đọc. |
| `ack` | Có khi lỗi được giải bằng một ack. |

Ví dụ thường gặp: `S06` là quá 12 trạng thái khác nhau (giải bằng cách dùng chung trạng thái giữa các bước); `P01` là bước dừng thiếu thông điệp.

`analysis` mô tả tình huống xấu nhất: số bước và vòng lặp tối đa, danh sách vòng, các mã dừng, và `release_paths_without_human` (đường thoát khỏi workspace không qua người). Số này quyết định ack nào cần tick.

Quy trình đọc: sửa mọi `error` trước, rồi xem `warning`, rồi xem `analysis`. Xuất bản chỉ xét `error` và ack.

## 6. Mô phỏng và route liên quan

- `POST /api/squads/{id}/flow-drafts/{draftId}/simulate`, thân `{"doc"?: {...}, "vars": {...}, "script": {...}, "max_hops": 60}`. `max_hops` mặc định 60, tối đa 120 (lớn hơn bị cắt). Trả `FlowTrace` (200), không ghi DB. Mô phỏng kiểm tra việc chọn cạnh, không kiểm tra công việc agent làm.
- `GET .../flow-drafts/{draftId}/export`: xuất manifest. `GET /api/issue-workflows/{id}/flow-manifest`: xuất manifest của phiên bản đã có.
- `POST /api/issues/{id}/flow/start` nhận thân tùy chọn `{"workflow_id"?: uuid, "trial"?: bool}`. Không có thân: bản đang dùng của squad. `workflow_id` phải là workflow flow của chính squad được giao (bất kỳ phiên bản nào, hoặc bản đang dùng); khác squad, không phải flow hay không tồn tại: `422 flow_workflow_invalid`, request không đổi. `trial: true` chỉ dành cho thành viên quản lý squad (lead là agent nhận `403 flow_forbidden`) và chỉ với workflow schema 2; workflow schema 1 trả `422 flow_workflow_invalid`.
- `POST /api/issues` nhận `flow_workflow_id` (mọi thành viên) và `flow_trial` (thành viên quản lý squad), kiểm như trên trước khi ghi issue. Actor agent gửi hai trường này bị bỏ qua, request chạy trên bản đang dùng.
- **Cờ.** Với `FF_FLOW_AUTHORING` tắt, `flow/start` có `workflow_id` hoặc `trial: true` và `POST /api/issues` có `flow_workflow_id` hoặc `flow_trial` trả `404 flow_authoring_disabled`, trước khi ghi gì (actor agent vẫn bị bỏ qua trường này như trước). Không có thân, `{}`, hay tạo issue không có các trường đó: chạy như cũ. UI ẩn ô chọn workflow và nút "Trial run" khi `GET /api/flow/capabilities` trả `features.authoring = false`.
- **Chạy thử.** Request chạy thử ghim đúng phiên bản được chọn dù bản khác đang dùng, có `trial = true` trong trạng thái và huy hiệu "Trial" ở trang request. Chính sách `allow_unattended_release` không có hiệu lực: công việc của agent chưa ai xem không ra khỏi workspace; request dừng `stalled` (`release_without_checkpoint`) tới khi một người chọn cạnh thoát, qua cổng người, hoặc resume với `acknowledge_skip_checkpoint`. Trên web: nút "Trial run" ở dòng phiên bản trên trang squad. Chạy thử dùng agent thật.
- Trạng thái request: `GET /api/issues/{id}/flow`. Mỗi task có `verdicts: [{key, class}]` (bộ verdict của task, hoặc bộ dựng sẵn `work`/`gate`). Đóng hộ task: `POST /api/issues/{taskId}/flow/close`; người quản lý đóng hộ được ghi `flow_manual_close`.

### Trường `stop` và `issue.flow`

Trong payload trạng thái, `stop` là đối tượng `{code, legacy_no, from, edge, at}` hoặc `null`. `stop_info` không còn. `code` là mã dừng của schema 2 (ví dụ `stalled`, `loop_exhausted` hoặc mã do tác giả đặt). `legacy_no` là số cũ (1, 2 hoặc 3).

`issue.flow` và `IssueFlowRef` trong payload issue giữ nguyên giá trị cũ: `1`, `3`, `"stalled"` (spec 7.5). Client cũ vì thế không vỡ. Client mới đọc `stop` từ trạng thái, không suy ra từ `issue.flow`.

## 7. Mã lỗi

Mọi lỗi có dạng `{error, code}`; một số có thêm trường (`required`, `latest_workflow_id`, `field`, `issues`).

| Mã | HTTP | Khi nào | Xử lý |
| --- | --- | --- | --- |
| `flow_authoring_disabled` | 404 | `FF_FLOW_AUTHORING` tắt | Bật cờ, khởi động lại. |
| `flow_graph_v2_disabled` | 404 | `FF_FLOW_GRAPH_V2` tắt khi tạo nháp từ manifest hoặc xuất bản | Bật cờ, khởi động lại. |
| `flow_authoring_member_only` | 403 | Agent gọi route bản nháp | Dùng tài khoản thành viên. |
| `flow_forbidden` | 403 | Không quản lý squad (nháp) | Cần quyền quản lý squad. |
| `flow_publish_forbidden` | 403 | Không quản lý squad (xuất bản) | Như trên. |
| `flow_ack_forbidden` | 403 | Người không phải admin xin ack dành cho admin | Nhờ admin xuất bản. |
| `flow_ack_unknown` | 400 | Tên ack không có | Sửa tên ack. |
| `flow_ack_required` | 422 | Xuất bản thiếu ack; `required` liệt kê | Tick các ack đó sau khi đọc kỹ. |
| `flow_validate_rate_limited` | 429 | Quá 30 lần mỗi phút mỗi actor | Chờ rồi gọi lại. |
| `flow_manifest_invalid` | 400 | Phong bì manifest sai (cũng khi tạo nháp từ manifest) | Sửa `format`, `name`, `doc`. |
| `flow_workflow_invalid` | 422 | `CreateIssueWorkflow` kèm `squad_id` mà vai không khớp roster; hoặc `flow/start` / `POST /api/issues` chọn workflow không thuộc squad của request, không phải flow, không có, hoặc chạy thử workflow schema 1 | Sửa vai hoặc roster; chọn workflow của squad. |
| `flow_schema_unsupported` | 400 | `CreateIssueWorkflow` nhận `flow_config.version >= 2` | Dùng route bản nháp và xuất bản. |
| `flow_checkpoint_ack_required` | 422 | Resume request schema 2 đang `stalled` còn `unreviewed` mà thiếu `acknowledge_skip_checkpoint` | Xem công việc rồi gửi lại với cờ. |
| `flow_draft_not_found` | 404 | Không có bản nháp | |
| `flow_draft_conflict` | 409 | `revision` lệch | Tải lại bản nháp. |
| `flow_draft_stale` | 409 | Bản gốc có phiên bản mới hơn (`latest_workflow_id`) | Fork lại. |
| `flow_draft_invalid` | 422 | Không qua kiểm tra khi xuất bản | Đọc `issues[]`. |
| `flow_draft_doc_invalid` | 400 | Tài liệu nháp sai cấu trúc | Sửa tài liệu. |
| `flow_draft_too_large` | 422 | Tài liệu quá lớn | Rút gọn. |
| `flow_draft_name_invalid` | 400 | Tên nháp không hợp lệ | Đổi tên. |
| `flow_draft_revision_required` | 400 | Thiếu `revision` | Gửi `revision`. |
| `flow_draft_from_invalid` | 422 | Phiên bản nguồn của `from` không dùng được | Chọn phiên bản flow hợp lệ. |
| `flow_draft_from_unsupported` | 400 | Loại `from` chưa hỗ trợ (`proposal`) | Dùng `fork` hoặc `manifest`. |
| `flow_agent_forbidden` | 403 | Không được dùng agent đã gắn vào vai | Đổi agent hoặc nhờ người có quyền. |
| `flow_limit_reached` | 409 | 20 nháp mỗi squad hoặc 50 phiên bản mỗi dòng | Xóa nháp cũ hoặc tạo dòng mới. |
| `workflow_name_taken` | 409 | Tên workflow trùng | Đổi tên. |
| `workflow_locked` | 409 | Sửa phiên bản đã khóa | Fork thành nháp. |
| `workflow_managed_by_template` | 409 | Workflow do catalog quản lý | Sao chép rồi sửa bản sao. |
| `status_create_forbidden` | 403 | Cần tạo trạng thái mới mà không phải admin | Nhờ admin hoặc dùng trạng thái có sẵn. |
