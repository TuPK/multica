> Tài liệu này thay thế phần engine của [kitviet-workflow-spec.md](kitviet-workflow-spec.md) cho các đợt sau; `kitviet-workflow-spec.md` và `kitviet-workflow-plan.md` giữ vai tài liệu lịch sử của đợt đầu (21 squad Kit Việt).

# Engine workflow tổng quát (schema 2) và trình soạn điều khiển bằng dữ liệu: đặc tả

Trạng thái: **đặc tả thiết kế đã qua rà soát kiến trúc lần 2; đã triển khai đủ P0–P28 và gate G-F đã được duyệt.** Các quyết định D1 đến D10 do người dùng chốt (mục 1). Người dùng đã giao toàn quyền chọn giải pháp cho đợt rà soát lần 2 (2026-11-13), nên D11 đến D13 **đã chốt** và D14 đến D24 là quyết định mới của đợt rà soát; lý do và những gì đã đổi so với bản trước nằm ở mục 15. Phần "từ bộ skill/agent tới workflow" (trước là pha F để sau) nay thuộc phạm vi: mục 16. Đối chiếu với engine mã nguồn mở: Phụ lục F. Kế hoạch từng bước: [flow-engine-generalization-plan.md](flow-engine-generalization-plan.md). Hai tài liệu này là nguồn duy nhất cho việc triển khai.

Ngày rà soát: 2026-11-13. Đọc mã ở **HEAD `d9da8d9fc`** (nhánh hiện tại của máy rà soát). Mọi trích dẫn dạng `tệp:dòng` trong hai tài liệu đã được đối chiếu với HEAD bằng script (tệp có thật, dòng nằm trong tệp) và được đọc lại bằng mắt; chỗ lệch đã sửa. Điều không kiểm chứng được ghi ở mục 14; số liệu là lựa chọn hay ước lượng cũng ghi ở đó. Lúc rà soát cây làm việc có thay đổi của người khác chưa commit hoặc chưa theo dõi (`apps/web/AGENTS.md`, `docs/engineering/kitviet-flow-web-drill.md`, `packages/views/locales/*/issues.json`, `packages/views/workflow/issue-flow-state-panel.tsx` và `.test.tsx`, `server/internal/handler/flow_runner_test.go`, `server/pkg/db/queries/flow.sql` và `server/pkg/db/generated/flow.sql.go`); không đụng tới. Số dòng của các tệp mà công việc khác đang sửa (xem mục 0.3 của kế hoạch) có thể lệch sau khi họ commit.

Ngôn ngữ: tiếng Việt. Tên tệp, kiểu, lệnh, mã lỗi giữ tiếng Anh. Chú thích trong mã viết tiếng Anh.

Thuật ngữ: **FlowDoc** = tài liệu flow schema 2. **Bản nháp** = FlowDoc chưa gắn vai. **Phiên bản** = hàng `issue_workflow` đã xuất bản, bất biến, mang FlowDoc đã biên dịch. **Đồ thị** = dạng trong bộ nhớ mà engine diễn giải (từ FlowDoc schema 2 hoặc từ schema 1 qua `Lift`). **Token** = vị trí hiện tại của request (`issue.flow.step`).

## 1. Mục tiêu, bất biến thiết kế, phạm vi

Mục tiêu của người dùng: **đưa lên một bộ skill và agent bất kỳ (có hoặc không kèm quy trình viết sẵn bằng markdown cho agent, như Kit Việt), rồi thiết kế được workflow để bộ đó chạy ổn nhất.** Mục tiêu này cần ba thứ, và tài liệu phải đủ cả ba:

1. **Engine tổng quát**: một engine duy nhất, điều khiển bằng dữ liệu, chạy được Kit Việt, phòng nghiên cứu, nghiên cứu thị trường, rà soát pháp lý/nội dung và các loại sau này mà không sửa mã lõi; máy chủ cưỡng chế quy trình, không phải prompt (mục 2 đến 9).
2. **Đường vào cho bộ bất kỳ**: nhận bộ ở các định dạng đang phổ biến, dựng squad, và đưa quy trình markdown (nếu có) thành bản nháp có người duyệt (mục 16).
3. **Vòng cải tiến**: mô phỏng, chạy thử trên phiên bản chưa dùng chính thức, đọc lại đường đi và số liệu của các request đã chạy để sửa workflow (mục 16.5, 16.6).

| # | Bất biến thiết kế | Hệ quả |
| --- | --- | --- |
| I1 | Máy chủ là nguồn sự thật về hợp lệ và về mọi dữ kiện | Một bộ kiểm trong Go; client chỉ hiển thị `issues[]` và `fact_catalog` do máy chủ trả |
| I2 | Định nghĩa là dữ liệu có kiểu; engine là bộ diễn giải cố định | Loại workflow mới = FlowDoc mới, không phải mã mới |
| I3 | Phiên bản đã xuất bản bất biến; request ghim phiên bản | Hàng có `published_at` khóa ghi; request giữ `issue.flow.workflow_id` (mục 8) |
| I4 | Bản nháp dùng vai, phiên bản dùng UUID | Biên dịch ở xuất bản, trong Go |
| I5 | Văn bản của skill, comment, mô tả không là nguồn của luật hay của dữ kiện. Thứ duy nhất agent báo mà engine dùng là **giá trị có kiểu, đã khai báo, gửi qua API**: verdict của task và biến task được phép đặt (`Task.sets`) | Mục 6.3, 2.6, 4.12 |
| I6 | Chỉ thêm, không phá; API chỉ thêm | Mục 7.4 |
| I7 | Agent không ghi bản nháp, không xuất bản, không gắn vai. Agent **được đề xuất** một tệp manifest; chỉ người nhập và xuất bản nó | Route soạn từ chối actor agent; route kiểm không trạng thái mở cho agent (mục 16.4) |
| I8 | Một token, một bước hiện tại | Song song là trong bước; nối tiếp giữa bước (mục 4.4) |
| I9 | Mọi chuyển trạng thái là một giao dịch có khóa dòng và so sánh-đổi trên `seq`; chạy lại là no-op | Mục 4.1 |
| I10 | Không khóa ngoại, không cascade, mọi chỉ mục `CONCURRENTLY` trong tệp riêng | Mục 2.1 (AGENTS.md "Database and Migration Rules") |

Ngoài phạm vi: mục 13. Các số liệu là lựa chọn hay ước lượng: mục 14.1.

**Quyết định.** D1 đến D10 do người dùng chốt; D6 đã **đổi** so với thiết kế đầu (chấp nhận giới hạn Kit Việt cho đợt này): người dùng yêu cầu tổng quát hóa engine ngay trong đợt này. D11 đến D24 do đợt rà soát lần 2 chốt theo ủy quyền của người dùng (mục 15 nêu lý do từng cái); D7 đổi theo D14.

| # | Nội dung | Trạng thái |
| --- | --- | --- |
| D1 | Phương án (c) lai: lõi bản nháp và phiên bản bất biến lưu trong DB, kiểm bằng cùng bộ kiểm của máy chủ, cộng nhân bản flow Kit Việt và nhập/xuất tệp | Đã chốt |
| D2 | `canManageSquad` xuất bản flow; tạo trạng thái mới và bật `flow_auto` chỉ admin | Đã chốt |
| D3 | Flow không có bước người trước khi ra khỏi workspace: cảnh báo; chỉ admin xác nhận bỏ qua | Đã chốt |
| D4 | Chỉ `canManageSquad` đóng task thay agent (verdict và tệp), có nhật ký và hiện rõ trong bảng task | Đã chốt |
| D5 | Vá H1 đến H4 ngay, kể cả flow Kit Việt đang chạy, sau khi test đỏ-trước xác nhận từng lỗ hổng (mục 5) | Đã chốt |
| D6 | Tổng quát hóa engine ngay trong đợt này | Đã chốt (đổi so với thiết kế đầu) |
| D7 | Tải lên bó agent + skill làm ở pha F; thiết kế chỗ cắm ngay (mục 6.4, Phụ lục E) | Đã chốt, **mở rộng bởi D14**: pha F thuộc đợt này (phát hành R3) |
| D8 | Bản nhân bản từ Kit Việt tách hẳn, không nhận cập nhật Kit Việt | Đã chốt |
| D9 | Phiên bản mới không kế thừa `flow_auto` | Đã chốt |
| D10 | Có mô phỏng định tuyến trong đợt đầu | Đã chốt |
| D11 | Giữ trình đọc schema 1 (`Lift`) lâu dài như trình đọc một phiên bản định dạng. `AGENTS.md` ("Change and Delivery Rules") cấm bộ chuyển đổi tương thích nội bộ nếu người dùng không yêu cầu; khuyến nghị giữ vì 21 manifest và `rev` của chúng giữ nguyên, request đang chạy ghim hàng schema 1 (task mang `flow_round` kiểu cũ), và nhân bản Kit Việt cần chính hàm này. Phương án thay thế: chuyển dữ liệu một lần rồi bỏ bộ đọc (đổi `rev` 21 flow, sinh hàng workflow mới cho mọi squad, viết lại trạng thái `issue.flow` và `flow_round` của request đang mở, rollback gần như không làm được) | Đã chốt (giữ `Lift`) |
| D12 | Hai thứ để ngoài đợt này: task do máy thực hiện (`performer: system`) và nhiều token cùng lúc. Hệ quả: cổng quyết định của phòng nghiên cứu do một task báo verdict kèm tệp, không do máy chủ tính | Đã chốt, kèm D17 và D23 |
| D13 | Phát hành ba lần cùng đợt: R1 = P0 đến P13 (vá, `Lift`, engine tổng quát, API chạy, diễn tập), R2 = P14 đến P22 (bản nháp, xuất bản, trình soạn, 7 ngôn ngữ), R3 = P23 đến P28 (nhập bộ skill/agent, đề xuất bản nháp, chạy thử, số liệu) | Đã chốt |
| D14 | Nhập bộ skill/agent bất kỳ thuộc phạm vi đợt này. Máy chủ nhận các định dạng đang có sẵn ngoài cộng đồng (thư mục Agent Skills, thư mục agent kiểu Claude Code, agent pack của Multica, bó gốc), không bắt bộ phải viết lại theo định dạng riêng (mục 16.1, Phụ lục E) | Đã chốt |
| D15 | Quy trình markdown thành bản nháp bằng **đề xuất của agent qua tệp manifest**: người mở một issue cho agent soạn, agent tự kiểm bằng route kiểm không trạng thái, đính tệp; người nhập tệp đó thành bản nháp và xuất bản. Không thêm quyền ghi nào cho agent (mục 16.4) | Đã chốt |
| D16 | Luồng dữ liệu là phần của mô hình: `Task.inputs` tường minh, mặc định theo đường đã đi, phản hồi của vòng sửa theo cạnh quay lại, và `trail` trong trạng thái request (mục 4.11, 4.13) | Đã chốt |
| D17 | Task được đặt biến đã khai báo (`Task.sets`; `Var.set_by` nhận khóa vai). Đây là cách một agent lập kế hoạch sinh danh sách cho `instances` và cách một task quyết định định tuyến bằng số liệu, không cần `performer: system` (mục 4.12) | Đã chốt |
| D18 | Task nêu skill phải dùng (`Task.skills`); xuất bản kiểm agent gắn vào vai thật sự có skill đó (R09) | Đã chốt |
| D19 | Tách vai tính theo **đầu vào của gate**, không theo cả flow: gate không giao cho người đã làm task `work` mà nó phán. Bộ nhỏ hai agent duyệt chéo nhau hợp lệ (R07, E13) | Đã chốt |
| D20 | "Rời workspace" đòi không còn **công việc của agent chưa được người xem** (cờ `unreviewed`), thay cho "đã từng có một điểm kiểm" (4.8, Y01, E15) | Đã chốt |
| D21 | Task do engine hủy mang outcome dành riêng `superseded`, tách khỏi task do người hủy; biến đang được `instances` dùng bị đóng băng trong bước đó (4.2, 4.3) | Đã chốt |
| D22 | Chạy thử một phiên bản chưa dùng chính thức, chọn workflow khi tạo request, và số liệu vận hành theo phiên bản (mục 16.5, 16.6) | Đã chốt |
| D23 | Khi cần nhiều nhánh nhiều bước chạy cùng lúc, điểm mở rộng ưu tiên là **bước `subflow`** (request con chạy một flow đã xuất bản), không phải `fork`/`join` nhiều token trong một request. Chưa làm ở đợt này (B.10) | Đã chốt |
| D24 | Không có chế độ bóng lúc chạy thật. So sánh bộ quyết định cũ và mới ở **mọi lần `advance` của các bài diễn tập DB** bằng một điểm móc chỉ có trong test (mục 9.3 điều 7) | Đã chốt |

## 2. Mô hình dữ liệu

### 2.1 Migration (bắt đầu từ 988; kiểm lại `ls server/migrations/` ở bước P0, số cuối hiện là `987_issue_flow_open_idx`)

Sáu migration dưới đây cho R2; bốn migration 994 đến 997 của R3 (`squad_pack_install`) ở mục 16.3. Không có cột `authoring`: FlowDoc đã biên dịch trong `flow_config` có đủ thông tin, và xuất manifest chỉ là bỏ `bind`.

| Tệp | Nội dung |
| --- | --- |
| `988_issue_workflow_versions.{up,down}.sql` | `ALTER TABLE issue_workflow ADD COLUMN IF NOT EXISTS lineage_id UUID, ADD COLUMN IF NOT EXISTS version INTEGER, ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ, ADD COLUMN IF NOT EXISTS published_by UUID;` (siêu dữ liệu) |
| `989_issue_workflow_lineage_version_idx.{up,down}.sql` | `CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_workflow_lineage_version ON issue_workflow (lineage_id, version) WHERE lineage_id IS NOT NULL;` |
| `990_issue_workflow_draft.{up,down}.sql` | `CREATE TABLE IF NOT EXISTS issue_workflow_draft (...)` (2.2), không khóa chính |
| `991_issue_workflow_draft_pkey_idx.{up,down}.sql` | `CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS issue_workflow_draft_pkey_uidx ON issue_workflow_draft (id);` |
| `992_issue_workflow_draft_pkey.{up,down}.sql` | `ALTER TABLE issue_workflow_draft ADD CONSTRAINT issue_workflow_draft_pkey PRIMARY KEY USING INDEX issue_workflow_draft_pkey_uidx;` (như `574`/`575`) |
| `993_issue_workflow_draft_squad_idx.{up,down}.sql` | `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_workflow_draft_squad ON issue_workflow_draft (workspace_id, squad_id, updated_at DESC);` |

Đăng ký ba chỉ mục (`989`, `991`, `993`) vào `concurrentIndexCleanups` ở `server/cmd/migrate/main.go:142`; `TestEveryConcurrentUpBuildHasCleanup` (`server/cmd/migrate/migrate_mul5999_index_retry_test.go:163`) đỏ nếu thiếu. Mỗi tệp có `.down.sql`.

**Cột mới của `issue_workflow` (migration 988):**

| Cột | Kiểu | Ý nghĩa |
| --- | --- | --- |
| `lineage_id` | `UUID NULL` | Họ phiên bản. NULL với workflow cũ và workflow catalog |
| `version` | `INTEGER NULL` | 1, 2, 3… trong lineage; duy nhất theo `(lineage_id, version)` (migration 989) |
| `published_at` | `TIMESTAMPTZ NULL` | Khác NULL = khóa ghi (cùng tác dụng `template_ref`) |
| `published_by` | `UUID NULL` | Người xuất bản (`user.id`) |

Tên hàng của phiên bản: `<tên bản nháp> · v<N>`, cắt ở 64 ký tự (`MaxNameRunes`, `issueworkflow.go:49`), cùng cách đặt tên `flowWorkflowName` (`packages/core/squads/flow-workflow.ts:36`); tên bản nháp tối đa 56 ký tự để chừa chỗ cho hậu tố. Chỉ mục tên duy nhất theo workspace (`migrations/576`) vẫn áp; va chạm trả 409 `workflow_name_taken` (đã có ở `issue_workflow.go:889`). Khóa ghi dùng một hàm: `workflowLocked(row) bool = row.TemplateRef.Valid || row.PublishedAt.Valid`, dùng ở mọi nơi hiện kiểm `TemplateRef.Valid`: `UpdateIssueWorkflow` (`issue_workflow.go:1036`), `SetIssueWorkflowHandoff` (`:1587`), và các chỗ ẩn nút sửa phía client.

**Không cần migration** cho các phần tổng quát hóa, và đây là chủ ý:

- FlowDoc schema 2 nằm trong cột `issue_workflow.flow_config` (JSONB đã có, `migrations/985_issue_flow_columns.up.sql`), khác schema 1 ở `version: 2`. Cột `steps` của hàng schema 2 chứa **danh sách trạng thái** (mỗi phần tử chỉ có `status_key`, handler `none`) để mã chung dựa vào `steps` (`CheckStatus`, `Has`, hiển thị cột, ánh xạ trạng thái khi đổi workflow) tiếp tục đúng; đồ thị thật nằm trong `flow_config`.
- Trạng thái request nằm trong `issue.flow` (JSONB): thêm khóa, không đổi cột (2.4).
- Danh tính task: `issue.flow_step` (khóa bước), `issue.flow_task` (`key` hoặc `key~i`), `issue.flow_round` (số lần vào bước, `activation`): cột có sẵn (`985`), chỉ mục duy nhất `idx_issue_flow_task_once` (`986`) vẫn đúng vì `(parent, step, task~i, activation)` vẫn duy nhất.
- Verdict: `issue.outcome TEXT` (`985`) chứa khóa verdict tùy ý.
- Nhật ký đóng thay và đặt biến: `activity_log` qua `CreateActivity` (đã dùng ở `flow_engine_ops.go:197`, `:425`).

### 2.2 Bảng `issue_workflow_draft`

| Cột | Kiểu | Ghi chú |
| --- | --- | --- |
| `id` | `UUID NOT NULL DEFAULT gen_random_uuid()` | khóa chính (migration 992) |
| `workspace_id` | `UUID NOT NULL` | mọi truy vấn lọc theo cột này |
| `squad_id` | `UUID NOT NULL` | squad sở hữu bản nháp; ứng dụng kiểm squad cùng workspace |
| `lineage_id` | `UUID NULL` | NULL cho flow mới; khi tạo từ phiên bản thì kế thừa |
| `base_workflow_id` | `UUID NULL` | phiên bản bản nháp xuất phát; dùng phát hiện bản cũ (`flow_draft_stale`) |
| `name` | `TEXT NOT NULL` | 1 đến 56 ký tự |
| `doc` | `JSONB NOT NULL` | FlowDoc schema 2 chưa gắn vai (2.3), tối đa 512 KiB |
| `revision` | `INTEGER NOT NULL DEFAULT 1` | khóa lạc quan; mỗi lần ghi tăng 1 |
| `origin` | `TEXT NOT NULL DEFAULT 'manual'` | `manual`, `fork` (từ một phiên bản), `import` (tệp manifest), `pack` (từ `flow.json` của bộ), `proposal` (do agent đề xuất, 16.4). Kiểm trong mã, không `CHECK` |
| `source_ref` | `JSONB NULL` | Nguồn của bản nháp: `{ "issue_id": … }` cho `proposal`, `{ "pack_install_id": … }` cho `pack`. Không khóa ngoại |
| `created_by`, `updated_by` | `UUID NOT NULL` | `user.id` |
| `created_at`, `updated_at` | `TIMESTAMPTZ NOT NULL DEFAULT now()` | |

Trần ứng dụng: 20 bản nháp mỗi squad, 50 phiên bản mỗi lineage (`flow_limit_reached`; số liệu là lựa chọn, mục 14.1). Xóa squad: ứng dụng dọn bản nháp của squad trong cùng giao dịch (không cascade); nếu squad chỉ lưu trữ thì giữ bản nháp.

### 2.3 FlowDoc schema 2

Mọi khóa (`key`) khớp `^[a-z][a-z0-9_]{0,31}$`, duy nhất trong tập của nó. Khóa lạ ở mọi cấp là lỗi (`flow_doc_invalid`), vì máy chủ cũ không được đoán nghĩa của khóa mới.

```jsonc
{
  "version": 2,
  "start": "plan",
  "roles":     [ Role ],
  "vars":      [ Var ],
  "verdicts":  [ VerdictSet ],
  "loops":     [ Loop ],
  "stops":     [ StopKind ],
  "terminals": [ Terminal ],
  "steps":     [ Step ],
  "messages":  { "<msgKey>": "title\nbody with {placeholders}" },
  "policy":    Policy
}
```

**Role**

| Trường | Kiểu | Ghi chú |
| --- | --- | --- |
| `key`, `label` | chuỗi | `label` ≤ 80 ký tự |
| `kind` | `agent` \| `member` \| `lead` \| `requester` \| `managers` | `lead`, `requester`, `managers` là vai dựng sẵn: khóa cố định `lead`, `requester`, `managers`, không `bind` |
| `pool` | số nguyên 1..20 | số tối thiểu người/agent phải gắn; mặc định 1 |
| `bind` | `{ "agent_ids": [uuid] }` hoặc `{ "member_ids": [uuid] }` | chỉ có ở dạng biên dịch; `agent` dùng `agent_ids`, `member` dùng `member_ids` |
| `source` | `{ "bundle": id, "agent_key": key }` hoặc vắng | điểm cắm pha F; engine bỏ qua (mục 6.4) |

**Var**: `{ key, type: "bool"|"enum"|"int"|"string"|"list_string", values?: [string] (enum), min?, max? (int), max_len? (string, ≤ 200), max_items? (list_string, ≤ 20), default, set_by: [roleKey], set_at: [stepKey] | ["*"] }`. Trần 16 biến. `default` thuộc miền của kiểu. `set_by` nhận vai dựng sẵn (`lead`, `managers`, `requester`) và **vai khai báo**: một vai khai báo chỉ đặt được biến qua một task của vai đó có liệt kê biến trong `Task.sets`, khi task đang mở (D17, 4.12). Quản lý luôn đặt được mọi biến (có nhật ký).

**VerdictSet**: `{ key, verdicts: [ { key, class: "pass"|"fail"|"neutral", label } ] }`; 1..8 verdict; có ít nhất một `pass`; khóa verdict `^[a-z][a-z0-9_]{0,31}$`; `blocked` (class `block`) **luôn** hợp lệ và không được khai trong bộ. Bộ dựng sẵn (không khai): `work = {done: pass}`, `gate = {approved: pass, rejected: fail}`, `plan = {planned: pass}` (cộng `blocked`).

**Loop**: `{ key, header: stepKey, max: n | { var: varKey, cap: n }, reset: "on_entry"|"never" }`. `max` số 1..20; dạng `{var, cap}` đòi `var` là `int` với `max ≤ cap ≤ 20`. Trần 8 vòng.

**StopKind**: `{ code, class: "missing_grounds"|"loop_exhausted"|"custom", label, message: msgKey, status?: statusKey, resume_by: "managers", legacy_no?: 1|3 }`. `stalled` luôn có sẵn (không khai). `status` mặc định `blocked`; phải là trạng thái có sẵn trong workspace thuộc nhóm `started` hoặc là `blocked`.

**Terminal**: `{ key, status: statusKey, leaves_workspace: bool }`; trạng thái thuộc nhóm `done` hoặc `closed`. Ít nhất một terminal.

**Step**

| Trường | Kiểu | Ghi chú |
| --- | --- | --- |
| `key`, `title` | chuỗi | `title` ≤ 64 |
| `status` | `{key}` \| `{name, category}` | trạng thái hiển thị; nhiều bước được chung; tạo mới cần admin |
| `kind` | `tasks` \| `lead` \| `manual` | |
| `tasks` | `[Task]` | ≤ 12; không có với `lead` |
| `join` | `Join` | mặc định `{over:"gates", mode:"all", block:"immediate"}` |
| `lead` | `{ sets: [varKey], verdicts: setKey, requires?: Requires }` | chỉ với `kind: lead`; mặc định bộ `plan` |
| `exit_role` | roleKey | chỉ `manual`; mặc định `managers`; vai `member`/`requester`/`managers` |
| `notice` | msgKey | bình luận hệ thống khi vào bước |
| `edges` | `[Edge]` | 1..10; thứ tự quan trọng |
| `layout` | `{x,y}` | tùy chọn |

**Task**: `{ key, title (≤ 80), kind: "work"|"gate", role, instances?, after?: [taskKey], when?: Expr, verdicts?: setKey, requires?: { files?: [FileReq], verdict?: bool, vars?: [varKey] }, inputs?: [Input], sets?: [varKey], skills?: [skillName], deadline?, instructions (≤ 8000) }`.
- `instances`: `{ count: 1..20 }` hoặc `{ from_var: varKey, min?: 1..max (mặc định 1), max: 1..20 }`; thêm `distinct: bool` mặc định `true`. Danh sách ngắn hơn `min` lúc vào bước thì request đỗ `stalled` với lý do `instances_too_few`.
- `inputs` (≤ 12): `Input = { from: "<stepKey>.<taskKey>", slot?: slotKey, required?: bool (mặc định false) }`. Vắng thì engine dùng quy tắc mặc định theo đường đã đi (4.11). Engine chỉ đưa **liên kết** tới issue task nguồn và tên tệp của nó vào mô tả; không đọc nội dung.
- `sets` (≤ 8): biến mà người thực hiện task được đặt khi task đang mở (4.12). `requires.vars ⊆ sets`: task không đóng được khi các biến đó chưa được đặt ở lần vào bước này.
- `skills` (≤ 8, tên khớp `^[a-z0-9][a-z0-9._-]*$`): skill mà task phải dùng. Lời nhắc do máy chủ sinh nêu tên chúng; xuất bản kiểm mọi agent gắn vào vai có skill đó (R09).
- `verdicts` mặc định: `work` → bộ `work`; `gate` → bộ `gate`. Gate phải có bộ có ít nhất một `pass` và một `fail`.
- `requires.verdict` mặc định `true` cho `gate`, `false` cho `work` (như `flow_guards.go:284-300`).
- `FileReq`: `{ slot, min: 1..10, max?: ≥ min, ext?: [".md",…] (≤ 8, chữ thường), content_types?: [...] }`.
- `deadline`: `{ idle_minutes?, hard_cap_minutes?, max_attempts? }` cho vai `agent`; `{ remind_after_minutes?, stall_after_minutes? }` cho vai `member`/`managers`/`requester`. Mặc định theo `policy`.

**Join**: `{ over: "gates"|"all"|[taskKey], mode: "all"|"any"|"quorum", k?: 1..n, block: "immediate"|"at_close" }`. `quorum` đòi `k` và `over` trỏ tới tập có ≥ `k` thực thể **chắc chắn có** (với `instances.from_var` tính theo `min`; task có `when` không được tính). `block: at_close` nghĩa là `block` không cắt ngang: bước chờ đủ như thường rồi mới ra `result = block` nếu có task `block`.

**Edge**: `{ id, label (1..64), to?: stepKey|terminalKey, when?: Expr|null, loop?: loopKey, stop?: stopCode, trigger?: "auto"|"manual" }`.
- Đúng một trong `to` hoặc `stop` (cạnh `stop` không có `to`).
- `trigger: manual` chỉ trên bước `manual`, và bước `manual` chỉ có cạnh `manual` (mỗi cạnh là một nút người bấm); bước `tasks`/`lead` chỉ có cạnh `auto`.
- `loop` đòi `to == loop.header`.
- Với bước `auto`: cạnh cuối không có `when` (mặc định) và là cạnh duy nhất như thế.

**Expr**: cây `{all|any|not|field,op,value}` hiện có, `field` thuộc sổ dữ kiện (2.6), độ sâu ≤ 4 (`transitions.go:67`).

**Policy**: `{ task_idle_minutes: 120, task_hard_cap_minutes: 240, task_max_attempts: 2, human_remind_minutes: 1440, human_stall_minutes: 10080, max_activations: 100, max_task_issues: 300, lead_may_work: false, allow_unattended_release: false, acks: [ { kind, by, at } ] }`. Giá trị tối đa do trần máy chủ (2.7). `lead_may_work` và `allow_unattended_release` chỉ `true` khi có `acks` tương ứng của admin (kiểm lúc xuất bản, 3).

### 2.4 Trạng thái request (`issue.flow`)

```jsonc
{
  "workflow_id": "uuid", "version": 2,                // version = schema của định nghĩa đã ghim
  "step": "gate", "seq": 17,
  "activations": { "collect": 1, "draft": 2, "gate": 2 },
  "loops": { "fix": 1 },
  "vars": { "route": "shipment_dossier", "customs_lookup": true },
  "came_from": "draft", "entered_at": "RFC3339",
  "stop": { "code": "loop_exhausted", "from": "gate", "edge": "gate_default", "at": "RFC3339" },   // null khi không đỗ
  "blocked_from": "gate",
  "checkpoints": [ { "step": "release", "activation": 1, "by": "uuid", "at": "RFC3339" } ],
  "var_sets": { "plan_sources.list_sources": 1 },      // task nào đã đặt biến ở lần vào bước nào (4.12)
  "unreviewed": false,                                 // true từ lúc một task work của agent được mở tới điểm kiểm người kế tiếp (4.8)
  "trail": [ { "step": "plan", "activation": 1, "edge": "plan_ok", "at": "RFC3339" } ],   // mỗi lần rời một bước một phần tử; tối đa max_activations
  "trial": false,                                      // true cho request chạy thử (16.5)
  "plan_done": true, "closed_at": "RFC3339"            // closed_at vắng cho tới khi đóng (chỉ mục idx_issue_flow_open)
}
```

`trail` là nguồn của đầu vào mặc định (4.11), của màn hình "đường đã đi" và của số liệu theo phiên bản (16.6). Nó chỉ chứa khóa, số và thời điểm; dữ kiện lúc chọn cạnh nằm ở `activity_log` (`flow_transition`, 4.13). Cỡ tối đa: 100 phần tử, mỗi phần tử dưới 200 byte.

Tương thích: trạng thái schema 1 (`rework`, `round_base`, `switches`, `stop: 1|3|"stalled"`) vẫn đọc được; `Adapt` chuyển sang dạng trên trong bộ nhớ (9.3). Mọi lần ghi tăng `seq`. **Mỗi request có đúng một dạng trạng thái**: request ghim hàng schema 1 giữ dạng v1 (`Adapt` khi đọc, `Unadapt` khi ghi; không ghi đôi), request ghim hàng schema 2 dùng dạng trên. Nhờ vậy lùi nhị phân (revert mã) không làm hỏng bộ đếm của request schema 1. `stop` dạng cũ (`1`, `3`, `"stalled"`) khớp mã `grounds_missing` (legacy_no 1), `loop_exhausted` (3), `stalled`.

### 2.5 Danh tính task trong `issue`

- `flow_step`: khóa bước.
- `flow_task`: `key` hoặc `key~i` (i ≥ 1) cho thực thể thứ i.
- `flow_round`: `activation` của bước lúc mở (số lần request đã vào bước đó trước lần này). Với đồ thị `compat: "v1"`: giữ `RoundBase + Rework` (9.3).
- Cha: `parent_issue_id` = request. Tiêu đề: `<tên bước> · <tên task>[ · i/n] · v<activation+1>`.
- Mỗi lần thực thể được mở, `item` (phần tử của `list_string`) chỉ đi vào **mô tả** task như dữ liệu trích dẫn (khối mã, ≤ 200 ký tự), không đi vào bất kỳ dữ kiện nào. Thực thể thứ i nhận phần tử thứ i của danh sách; danh sách không đổi được khi request đang ở bước dùng nó (4.2).
- `outcome` dành riêng **`superseded`**: chỉ engine ghi, cùng lúc engine hủy một task (hợp `any`/`quorum` đã xong, request tới terminal, `resume` rời bước). Không ai chọn được nó qua API. Hợp bỏ qua task `superseded`; quy tắc mở lại của E7 không áp cho nó. Task bị hủy **không** có `superseded` là task do người hủy.
- **Chọn người trong hồ bơi** (vai có nhiều `bind`): thực thể thứ i nhận `bind[(i-1) mod n]`, task không có `instances` nhận `bind[0]`; cùng một thực thể giữ cùng người qua các lần vào bước (người sửa là người đã làm). Nếu người đó không dùng được (lưu trữ, rời squad) hoặc vi phạm tách vai (E13), engine lấy người kế tiếp trong `bind` chưa nhận thực thể nào của task ở lần vào bước này; hết người thì `stalled` với lý do `pool_too_small`.

### 2.6 Sổ dữ kiện (`FactRegistry`)

Bảng sau là toàn bộ nguồn mà biểu thức được phép đọc. Mọi mục có kiểu và toán tử. Bộ kiểm từ chối trường ngoài bảng.

| Khóa | Kiểu | Toán tử | Nguồn thật | Lúc nào hợp lệ |
| --- | --- | --- | --- | --- |
| `join.result` | enum `pass`,`fail`,`block`,`none` | eq, neq, in | Engine: kết quả `join` của bước vừa hoàn tất | Chỉ trong cạnh của bước `tasks`/`manual` |
| `join.pass`, `join.fail`, `join.block`, `join.neutral`, `join.open`, `join.total` | int | eq…lte | Engine: đếm class của các task hoạt động ở lần vào bước hiện tại | như trên |
| `task.<key>.verdict` | string (khóa verdict) | eq, neq, in, exists | Engine: `issue.outcome` của task (hoặc thực thể) đã đóng ở lần vào bước hiện tại; nhiều thực thể: giá trị khi chỉ có một thực thể, ngược lại dùng `task.<key>.<class>` | cạnh của bước chứa task |
| `task.<key>.pass`, `.fail`, `.block`, `.neutral` | int | eq…lte | Engine: đếm theo class trong các thực thể | như trên |
| `lead.verdict` | string | eq, neq, in | Engine: verdict lead báo khi hoàn tất bước `lead` (đã kiểm theo bộ verdict) | cạnh của bước `lead` |
| `loop.<key>.count` | int | eq…lte | Engine: `issue.flow.loops` | mọi cạnh |
| `step.<key>.activations` | int | eq…lte | Engine: `issue.flow.activations` | mọi cạnh |
| `flow.activations` | int | eq…lte | Engine: tổng activation | mọi cạnh |
| `request.priority` | enum | eq…, gt…lte theo thứ hạng | Hàng `issue` của request | mọi cạnh và `when` của task |
| `request.labels` | list_string | contains, exists | Nhãn của request | như trên |
| `request.creator_type` | enum `member`,`agent` | eq, neq, in | Hàng `issue` | như trên |
| `request.project_id` | string | eq, neq, in, exists | Hàng `issue` | như trên |
| `var.<key>` | theo `Var.type` | theo kiểu: bool eq/neq; enum eq/neq/in; int eq…lte, in; string eq/neq/in/exists; list contains/exists | Người hoặc task được phép đặt qua API (`set_by`, `set_at`, `Task.sets`); kiểm kiểu và miền; có nhật ký | mọi cạnh và `when` của task |

Quy tắc: (1) không có nguồn nào ngoài ba: `engine`, `request`, `var`; (2) dữ kiện `engine` do hàm thuần `BuildFacts(def, state, tasks, issue)` dựng từ trạng thái máy chủ và có test liệt kê từng dữ kiện với nguồn của nó; (3) `when` và `instances` của task chỉ được dùng dữ kiện **có sẵn lúc vào bước**: `var.*`, `request.*`, `loop.*`, `step.*`, `flow.*`, `task.<key>.*` của task ở **bước khác**, không dùng `join.*` hay `task.*` cùng bước (chưa tồn tại); biến mà một task của **chính bước đó** đặt (`Task.sets`) không dùng được cho `when`/`instances` của task cùng bước, trừ khi task dùng nó đứng sau task đặt nó qua `after`; `task.<key>.*` của bước khác đọc lần vào gần nhất của bước ấy, và là "không có" nếu bước ấy chưa từng chạy hoặc task ấy không mở ở lần vào đó: `exists` sai, mọi so sánh khác trên nó sai, các bộ đếm (`.pass`, `.fail`…) bằng 0. Đây là dữ kiện **đã khai nhưng chưa có giá trị**, khác trường ngoài sổ (lỗi lúc ghi, F01); (4) biểu thức ở cạnh khác được dùng mọi dữ kiện cho bước nguồn; (5) tên cũ của schema 1 (`tasks_outcome`, `rework_count`, `outcome`, `switch.*`) bị từ chối trong schema 2 (`legacy_field_in_v2`); `Lift` viết lại chúng (9).

### 2.7 Trần và danh mục khả năng (data-driven)

Các số do **máy chủ** đặt và trả qua `GET /api/flow/capabilities` (7.1) để client không gắn cứng. Giá trị khởi đầu (lựa chọn của tác giả thiết kế theo cỡ giao diện và số issue mỗi request, chưa đo):

| Trần | Giá trị | Ghi chú |
| --- | --- | --- |
| Bước mỗi flow | 40 | Schema 1: 12 trạng thái (giữ, `flow.go:40`) |
| Trạng thái bảng khác nhau mỗi flow | 12 | chống làm bảng của workspace rối; tạo trạng thái mới cần admin |
| Task mỗi bước | 12 | schema 1: 8 (`flow.go:41`) |
| Cạnh mỗi bước | 10 | `transitions.go:66` |
| Vai / biến / bộ verdict / vòng | 16 / 16 / 8 / 8 | |
| Thực thể mỗi task (`instances`) | 20 | |
| `loop.max`, `cap` | 20 | schema 1: 5 (`flow.go:44`), giữ cho schema 1 |
| Độ sâu biểu thức | 4 | `transitions.go:67` |
| Độ sâu `after` | 6 | |
| Số issue task trường hợp xấu nhất mỗi request | 300 (`max_task_issues`); cứng 500 | |
| Kích thước tài liệu | 512 KiB | |
| Thông điệp | 4000 ký tự | `flow.go:46` |

Danh mục còn có: các phiên bản schema hiểu được (`[1,2]`), `step_kinds`, `task_kinds`, `join_modes`, `role_kinds`, `verdict_classes`, `var_types`, `stop_classes`, `ops_by_type`, các dữ kiện dựng sẵn, và `features: { packs: false, proposals: false, system_performer: false, subflow: false, fork_join: false }` (`packs` và `proposals` thành `true` ở R3). Một khóa lạ mà client gặp trong tài liệu (từ máy chủ mới hơn) hiển thị là "unsupported" chứ không bị bỏ.

### 2.8 Ví dụ: Trade dưới dạng FlowDoc schema 2 (rút gọn)

```jsonc
{ "version": 2, "start": "plan",
  "roles": [ {"key":"docs_reader","kind":"agent"}, {"key":"customs_research","kind":"agent"},
             {"key":"drafter","kind":"agent"}, {"key":"docs_gate","kind":"agent"},
             {"key":"customs_gate","kind":"agent"}, {"key":"handover_gate","kind":"agent"} ],
  "vars": [ {"key":"route","type":"enum","values":["shipment_dossier"],"default":"shipment_dossier","set_by":["lead"],"set_at":["plan"]},
            {"key":"customs_lookup","type":"bool","default":false,"set_by":["lead"],"set_at":["plan"]},
            {"key":"export","type":"bool","default":false,"set_by":["lead"],"set_at":["plan"]} ],
  "loops": [ {"key":"fix","header":"draft","max":2} ],
  "stops": [ {"code":"grounds_missing","class":"missing_grounds","message":"stop1","legacy_no":1},
             {"code":"loop_exhausted","class":"loop_exhausted","message":"stop3","legacy_no":3} ],
  "terminals": [ {"key":"done","status":"done","leaves_workspace":true}, {"key":"cancelled","status":"cancelled","leaves_workspace":false} ],
  "steps": [
    {"key":"plan","kind":"lead","status":{"key":"todo"},"lead":{"sets":["route","customs_lookup","export"]},
     "edges":[ {"id":"plan_blocked","label":"Missing grounds","when":{"field":"lead.verdict","op":"eq","value":"blocked"},"stop":"grounds_missing"},
               {"id":"plan_ok","label":"Planned","to":"collect"} ]},
    {"key":"collect","kind":"tasks","status":{"name":"Collect","category":"started"},
     "tasks":[ {"key":"docs","kind":"work","role":"docs_reader","title":"…","instructions":"…"},
               {"key":"customs","kind":"work","role":"customs_research","when":{"field":"var.customs_lookup","op":"eq","value":true},"title":"…","instructions":"…"} ],
     "edges":[ {"id":"collect_blocked","label":"Missing grounds","when":{"field":"join.result","op":"eq","value":"block"},"stop":"grounds_missing"},
               {"id":"collect_ok","label":"Collected","to":"draft"} ]},
    {"key":"draft","kind":"tasks","status":{"name":"Draft","category":"started"},
     "tasks":[ {"key":"crosscheck","kind":"work","role":"drafter","requires":{"files":[{"slot":"draft","min":1}]},"title":"…","instructions":"…"} ],
     "edges":[ {"id":"draft_blocked","label":"Missing grounds","when":{"field":"join.result","op":"eq","value":"block"},"stop":"grounds_missing"},
               {"id":"draft_ok","label":"Drafted","to":"gate"} ]},
    {"key":"gate","kind":"tasks","status":{"name":"Gate","category":"started"},
     "tasks":[ {"key":"docs_gate","kind":"gate","role":"docs_gate","title":"…","instructions":"…"},
               {"key":"customs_gate","kind":"gate","role":"customs_gate","title":"…","instructions":"…"} ],
     "join":{"over":"gates","mode":"all","block":"immediate"},
     "edges":[ {"id":"gate_blocked","label":"Missing grounds","when":{"field":"join.result","op":"eq","value":"block"},"stop":"grounds_missing"},
               {"id":"gate_ok","label":"Approved","when":{"field":"join.result","op":"eq","value":"pass"},"to":"handover"},
               {"id":"gate_fix","label":"Fix and re-run all gates","when":{"all":[{"field":"join.result","op":"eq","value":"fail"},{"field":"loop.fix.count","op":"lt","value":2}]},"to":"draft","loop":"fix"},
               {"id":"gate_default","label":"Rework limit reached","stop":"loop_exhausted"} ]},
    {"key":"handover","kind":"tasks","status":{"name":"Handover","category":"started"},
     "tasks":[ {"key":"handover","kind":"gate","role":"handover_gate","title":"…","instructions":"…"} ],
     "edges":[ {"id":"handover_ok","label":"Approved","when":{"field":"join.result","op":"eq","value":"pass"},"to":"release"},
               {"id":"handover_stop1","label":"Missing grounds","when":{"field":"join.result","op":"eq","value":"block"},"stop":"grounds_missing"},
               {"id":"handover_stop","label":"Rejected","stop":"loop_exhausted"} ]},
    {"key":"release","kind":"manual","status":{"key":"in_review"},"notice":"stop2","exit_role":"managers",
     "tasks":[ {"key":"export","kind":"work","role":"drafter","when":{"field":"var.export","op":"eq","value":true},"title":"…","instructions":"…"} ],
     "edges":[ {"id":"release_done","label":"Release","trigger":"manual","to":"done"} ]}
  ] }
```

Hai cạnh `gate_blocked` và `handover_stop1` là kết quả của quy tắc tách ở mục 9.2 (trong schema 1 chỉ có một cạnh mặc định tới `blocked`, mã 1 hay 3 do dữ kiện lúc chạy quyết định).

### 2.9 Ví dụ rút gọn: vòng có giới hạn từ biến, `instances`, `quorum`

```jsonc
// Research-lab style: the lead sets how many search rounds are allowed, never more than 5.
"vars":  [ {"key":"max_rounds","type":"int","min":1,"max":5,"default":3,"set_by":["lead"],"set_at":["plan"]} ],
"loops": [ {"key":"rounds","header":"search","max":{"var":"max_rounds","cap":5}},
           {"key":"fixes","header":"synthesize","max":2} ],
// Market-research style: one collector task per source the lead names, from a pool of 3 agents.
{"key":"collect_source","kind":"work","role":"collector","instances":{"from_var":"sources","max":8},
 "requires":{"files":[{"slot":"report","min":1,"ext":[".md"]}]}}
// Legal-review style: five human reviewers, three approvals needed, abstentions allowed.
"verdicts":[ {"key":"review","verdicts":[{"key":"approve","class":"pass"},{"key":"reject","class":"fail"},{"key":"abstain","class":"neutral"}]} ],
{"key":"review","kind":"tasks","tasks":[{"key":"reviewer","kind":"gate","role":"reviewers","instances":{"count":5},"verdicts":"review"}],
 "join":{"over":"gates","mode":"quorum","k":3,"block":"immediate"},
 "edges":[ {"id":"ok","label":"Approved","when":{"field":"join.result","op":"eq","value":"pass"},"to":"signoff"},
           {"id":"cut","label":"Critical: back to intake","when":{"all":[{"field":"join.result","op":"eq","value":"fail"},{"field":"var.severity","op":"eq","value":"critical"},{"field":"loop.cutback.count","op":"lt","value":1}]},"to":"intake","loop":"cutback"},
           {"id":"fix","label":"Back to the clauses","when":{"all":[{"field":"join.result","op":"eq","value":"fail"},{"field":"loop.clauses.count","op":"lt","value":3}]},"to":"redraft_clauses","loop":"clauses"},
           {"id":"stop","label":"Review limit reached","stop":"loop_exhausted"} ]}
```

## 3. Bộ kiểm hợp lệ

Máy chủ là nguồn sự thật. Hàm thuần `issueworkflow.ValidateFlowDoc(doc, ctx) []Issue` với `ctx` chứa danh sách vai của squad (lead, thành viên agent), catalog trạng thái, danh sách thành viên workspace, trần. Ba nơi gọi: `validate` của bản nháp, `publish`, và `CreateIssueWorkflow` (đường quickstart catalog, để schema 2 do catalog cũng qua cùng bộ kiểm). Schema 1 tiếp tục qua `ValidateFlow` hiện có (`flow.go:493`) cộng bộ kiểm vai mới (nhóm R) vì ba quy tắc vai hiện chỉ có ở công cụ catalog (giả định A22, Phụ lục A).

**Dạng lỗi**: `{ "code", "severity": "error"|"warning"|"info", "path", "params", "message" }`. `path` dùng **khóa**, không dùng chỉ số, nên bền vững khi sắp xếp lại: `steps[gate].tasks[docs_gate].role`, `steps[gate].edges[gate_fix].when`, `roles[docs_gate]`. `message` là tiếng Anh cho nhật ký và CLI; giao diện dịch theo `code` + `params` ở 7 ngôn ngữ. Máy chủ gom **mọi** vấn đề một lần. Cảnh báo có `ack` nêu cần xác nhận khi xuất bản.

Cột "Nơi": **E** = đã có trong Go (kèm dòng); **N** = mới.

### 3.1 Cấu trúc và giới hạn (S)

| Mã | Quy tắc | Mức | Nơi |
| --- | --- | --- | --- |
| `S01` | `version` ∈ phiên bản hiểu được; khóa lạ ở mọi cấp bị từ chối (`flow_schema_unsupported` / `flow_doc_invalid`) | lỗi | N |
| `S02` | Kích thước và trần 2.7 | lỗi | N (schema 1: E `flow.go:508`, `:40-47`) |
| `S03` | Khóa đúng mẫu, duy nhất trong tập (vai, biến, bộ verdict, vòng, mã dừng, terminal, bước, task, cạnh trong bước) | lỗi | N (task: E `flow.go:56`) |
| `S04` | `start` tồn tại; bước đó có `kind` ∈ {`lead`, `tasks`} | lỗi | N (schema 1: E `flow.go:522-531`) |
| `S05` | Mỗi bước có `status`: khóa tồn tại, chưa lưu trữ, hoặc tên mới (chỉ admin: `status_create_forbidden`) | lỗi | E `issue_workflow.go:605-700` |
| `S06` | Số trạng thái bảng khác nhau ≤ 12 | lỗi | N |
| `S07` | Nhiều bước chung một trạng thái | cảnh báo `status_shared` (kéo thẻ thủ công vào trạng thái đó mơ hồ) | N |
| `S08` | Terminal có trạng thái nhóm `done`/`closed`; mọi `stop.status` hợp lệ | lỗi | N |
| `S09` | Tiêu đề, nhãn, hướng dẫn, thông điệp theo độ dài | lỗi | E `issueworkflow.go:49-51`, `flow.go:46` |

### 3.2 Đồ thị (G)

| Mã | Quy tắc | Mức | Nơi |
| --- | --- | --- | --- |
| `G01` | Mọi `edge.to` là bước hoặc terminal tồn tại; mọi `edge.stop` là mã đã khai báo hoặc `stalled` | lỗi | N (schema 1: E `transitions.go:693-731`) |
| `G02` | Mọi bước đến được từ `start` | lỗi `step_unreachable` | N |
| `G03` | Không bước chết: bước `tasks`/`lead` có cạnh mặc định hoặc cạnh bao hết miền; bước `manual` có ít nhất một cạnh `manual` | lỗi `dead_step` | N (schema 1: E `flow.go:652-662`) |
| `G04` | Bỏ hết cạnh quay lại (có `loop`) thì đồ thị không còn chu trình | lỗi `cycle_without_loop` | E cho schema 1 (`flow.go:719-818`, chu trình ở `:796-816`), N cho schema 2 |
| `G05` | Cạnh có `loop`: `loop` tồn tại, `to == loop.header`, cạnh nằm trong chu trình (đi từ `header` tới nguồn của nó) | lỗi `loop_edge_invalid` | N |
| `G06` | Mỗi vòng có ít nhất một cạnh quay lại và ít nhất một lối vào xuôi vào `header`; `header` không phải `start` của vòng lồng sai thứ tự (hai vòng cùng `header` cho phép) | lỗi `loop_invalid` | N |
| `G07` | Mỗi bước có cạnh quay lại thì có cạnh phía sau (mặc định hoặc `stop`) xử lý lúc vòng cạn | lỗi `loop_exit_missing` | N |
| `G08` | Tồn tại đường từ `start` tới ít nhất một terminal không phải qua `stop` (sống) | lỗi `no_live_path` | N |
| `G09` | Cạnh `manual` chỉ trên bước `manual`; bước `auto` không cạnh `manual` | lỗi | N |
| `G10` | Bước `lead` không có `tasks`; `lead.sets` ⊆ biến có `set_by` chứa `lead` và `set_at` chứa bước | lỗi | N (E một phần `flow.go:526-528`) |
| `G11` | Chi phí theo công thức ở 4.5 (thân vòng, không cần cây lồng): tổng `lần_vào_tối_đa(bước) × số_task_tối_đa(bước)` ≤ `max_task_issues` (300) và tổng `lần_vào_tối_đa` ≤ `max_activations` | lỗi `flow_too_expensive` kèm con số tính được | N |

### 3.3 Vai (R)

| Mã | Quy tắc | Mức | Nơi |
| --- | --- | --- | --- |
| `R01` | Mỗi `task.role`, `exit_role` có trong `roles` hoặc là vai dựng sẵn | lỗi `role_unknown` | N (mirror `build-workflows.mjs:199`) |
| `R02` | Mọi vai không dựng sẵn có `bind` đủ `pool` (lúc xuất bản) | lỗi `role_unbound` | N |
| `R03` | Agent gắn tồn tại, chưa lưu trữ, **là thành viên agent của squad**, **không phải lead**; người gắn là thành viên workspace | lỗi `role_not_member` / `role_is_lead` | N (mirror `build-workflows.mjs:200`) |
| `R04` | Mỗi vai không dựng sẵn được ít nhất một task dùng | lỗi `role_unused` | N (mirror `build-workflows.mjs:241`) |
| `R05` | Vai `lead` không dùng cho task `work`/`gate` trừ khi `policy.lead_may_work` có ack admin | lỗi `lead_cannot_work` | N |
| `R06` | `instances`: `distinct` đòi hồ bơi ≥ số thực thể (với `count`) hoặc ≥ `max` (với `from_var`); `from_var` là `list_string` | lỗi `pool_too_small` | N |
| `R07` | **Tách vai theo đầu vào (D19):** với mỗi task `gate` G, gọi W là các task `work` mà G phán: task `work` trong `inputs` của G (hoặc đầu vào mặc định theo 4.11) cộng task `work` cùng bước mà G đứng sau qua `after`. Phải còn ít nhất một người trong `bind` của vai G không thuộc `bind` của vai nào trong W (với `instances` có `distinct`: còn đủ số người). Vai G và vai W là **cùng một vai một người** thì lỗi | lỗi `separation_violation` | N (thay quy tắc theo vị trí `flow.go:633-643`; lúc chạy: E13) |
| `R08` | Vai `member`/`managers`/`requester` chỉ dùng loại `deadline` của người | lỗi | N |
| `R09` | **Skill của task (D18):** mỗi tên trong `Task.skills` là skill mà **mọi** agent trong `bind` của vai đang có (skill gắn trực tiếp hoặc qua squad, cùng truy vấn `ListEffectiveAgentSkills`, `squad_skill.sql:31`). Vai người không dùng `skills` | lỗi `skill_not_attached` (kèm `agent_id`, `skill`); chỉ kiểm lúc xuất bản | N |

### 3.4 Task, verdict, tệp, hợp (T, J)

| Mã | Quy tắc | Mức | Nơi |
| --- | --- | --- | --- |
| `T01` | `verdicts` tồn tại; gate có bộ với ≥ 1 `pass` và ≥ 1 `fail`; khóa verdict đúng mẫu | lỗi | N (E một phần `flow.go:616`) |
| `T02` | `requires.verdict` bắt buộc `true` cho gate | lỗi | E `flow.go:616` |
| `T03` | `after` chỉ trỏ task cùng bước, không chu trình, độ sâu ≤ 6; `after` không chứa task có `when` có thể sai khiến phụ thuộc không bao giờ đóng (coi task chưa mở là "bỏ qua", không chặn) | lỗi / cảnh báo | N |
| `T04` | `when` dùng dữ kiện có sẵn lúc vào bước (2.6 điều 3) | lỗi `fact_not_available` | N |
| `T05` | `FileReq`: `ext` đúng mẫu, `min ≥ 1`, `max ≥ min`; `slot` duy nhất trong task | lỗi | N |
| `T06` | `deadline` trong giới hạn | lỗi | N |
| `T07` | `inputs.from` trỏ task có thật ở **bước khác**, và có đường xuôi từ bước đó tới bước này; `slot` là khe đã khai ở task nguồn. Nguồn không nằm trên **mọi** đường tới bước này: cảnh báo `input_maybe_missing`, hoặc lỗi nếu `required: true` | lỗi `input_invalid` / cảnh báo | N |
| `T08` | `sets` trỏ biến có thật, và vai của task có trong `set_by` của biến, bước của task có trong `set_at`; `requires.vars ⊆ sets`; biến được dùng ở `when`/`instances` của bước nào thì phải có ít nhất một nơi đặt nó **trước** bước đó (task đặt ở bước đứng trước trên mọi đường, bước `lead` đứng trước, hoặc `default`) | lỗi `var_set_invalid` / cảnh báo `var_maybe_unset` | N |
| `J01` | `join.over` trỏ task có thật; `quorum` đòi `1 ≤ k ≤ n_chắc_chắn` (2.3 Join) | lỗi | N |
| `J02` | **Đủ miền:** với mỗi kết quả khả dĩ của `join.result` có ít nhất một cạnh khớp hoặc cạnh mặc định. Miền: {pass, fail, block} nếu `over` có gate; thêm `none` nếu `over` có thể rỗng lúc chạy (mọi task trong `over` có `when`) **hoặc** bộ verdict trong `over` có class `neutral`; {none, block} nếu không có gate | lỗi `join_result_unrouted` | N (tổng quát hóa E `flow.go:659-661` "cần cạnh `all_approved`") |
| `J03` | Bước `manual`: mọi cạnh `trigger: manual`, nhãn duy nhất | lỗi | N |

### 3.5 Dữ kiện và biến (F)

| Mã | Quy tắc | Mức | Nơi |
| --- | --- | --- | --- |
| `F01` | Mọi `field` thuộc sổ dữ kiện (2.6); trường ngoài sổ là lỗi, **không** `false` im lặng | lỗi `unknown_fact` | N (thay `transitions.go:235-247`) |
| `F02` | Toán tử hợp lệ cho kiểu; giá trị đúng kiểu và đúng miền (enum, khoảng int) | lỗi | E (cây `validateCondition`, `transitions.go:532-`) + N cho kiểu mới |
| `F03` | Biến: `default` thuộc miền, `set_at` là bước có thật, `set_by` ⊆ {lead, managers, requester} ∪ khóa vai đã khai; vai khai báo trong `set_by` phải có ít nhất một task liệt kê biến trong `sets` (T08) | lỗi | N (E một phần `flow.go:540-585`) |
| `F04` | Biến khai báo nhưng không dùng ở đâu | cảnh báo `var_unused` | N |
| `F05` | `loop.max.var` là `int` với `max ≤ cap` | lỗi | N |
| `F06` | Dữ kiện tên cũ schema 1 trong tài liệu schema 2 | lỗi `legacy_field_in_v2` | N |

### 3.6 Mã dừng, thông điệp, chính sách (P, Y)

| Mã | Quy tắc | Mức | Nơi |
| --- | --- | --- | --- |
| `P01` | Mọi `stop.message` và `step.notice` trỏ khóa trong `messages`; biến trong thông điệp thuộc `{requester, request, step, rework, tasks, missing, reason, loop, stop}` | lỗi / cảnh báo `message_placeholder` | N |
| `P02` | **Thông điệp không chứa liên kết `mention://`** (tác giả không được thông báo người tùy ý qua bình luận hệ thống) | lỗi `message_mention` | N |
| `Y01` | **Rời workspace (D3, D20):** gọi *cạnh điểm kiểm* là cạnh `manual` của bước `manual`, và cạnh của bước `tasks` mà điều kiện đòi `join.result = pass` khi `join.over` (mode `all`) có ít nhất một gate vai người không có `when`. Bỏ hết cạnh điểm kiểm khỏi đồ thị; nếu terminal `leaves_workspace` còn đến được từ `start` **hoặc từ bất kỳ bước nào có task `work` của vai agent** thì có đường mà việc agent làm ra ngoài không qua mắt người | cảnh báo `release_without_human` (kèm bước và đường mẫu), cần ack admin (`policy.allow_unattended_release`) | N |
| `Y02` | Không có gate nào: không có gì được kiểm độc lập | cảnh báo `no_gate`, cần ack | N |
| `Y03` | Vòng ngầm: hai vòng cùng `header` hoặc chồng lấn mà có `reset: never` | cảnh báo `loops_overlap` | N |
| `Y04` | `lead_may_work` hay `allow_unattended_release` `true` không có ack admin | lỗi `ack_required` | N |
| `Y05` | Bước `manual` không có `notice` | cảnh báo `manual_without_notice` | N |

## 4. Ngữ nghĩa thực thi

### 4.1 Khung: `Advance`, khóa dòng, chạy lại

`Advance(request)` giữ ba pha của bản hiện tại (`flow_engine.go:523-596`): (1) **quyết định** trong một giao dịch có `LockIssueForFlow`, ghi trạng thái bằng `SetIssueFlow` có điều kiện `step` và **`seq`** (đổi từ `step` và `rework`, `flow.sql` `SetIssueFlow`); (2) **mở task thiếu** ngoài giao dịch, mỗi task một `IssueService.Create` với `AllowDuplicate`, nhận `23505` của `idx_issue_flow_task_once` là "đã có"; (3) phát sự kiện. Chạy lại là no-op khi: đã đỗ, đã đóng, hoặc không còn gì thiếu/không có cạnh để đi (tiêu chí B1 hiện có). Mọi nơi gọi hiện có (lead; runner `flow_runner.go:32`; sự kiện sub-issue `handler/issue_child_event.go:17`; hoàn tất lượt chạy `flow_runner.go:72`; quét; bỏ qua `child_done` cho request tự chạy `service/issue_wakeup_system.go:866-869`) giữ nguyên chữ ký và hành vi.

### 4.2 Quyết định một request ở bước `S` (hàm thuần `Decide(graph, state, tasks, facts) Decision`)

1. `S.kind = lead` → `Wait` (lead hoàn tất bằng API 7.3).
2. `S.kind = tasks` hoặc `manual`:
   a. `expected` = task có `when` đúng (đánh giá lại **mỗi lần**, nên đổi biến mở thêm task ở lần `advance` kế, như hôm nay `flow.go:227-238`), mở rộng `instances` (đọc `list_string` ở `state.vars`, cắt ở `max`). Hai quy tắc giữ cho việc đánh giá lại không gây mơ hồ (D21): **(i)** task đã mở không bao giờ bị engine hủy vì một biến đổi giá trị; nó vẫn thuộc lần vào bước và vẫn được hợp chờ (đúng hành vi `currentTasks` hôm nay, `flow.go:441-450`); quản lý hủy tay nếu muốn. **(ii)** Biến đang được `instances.from_var` của bước hiện tại dùng không đổi được kể từ khi thực thể đầu tiên của task đó được mở ở lần vào bước này cho tới khi request rời bước: `PUT …/vars/{key}` trả 409 `flow_var_in_use`, nên thực thể thứ i luôn ứng với phần tử thứ i.
   b. `ready` = task trong `expected` chưa có hàng `(step, key~i, activation)` và mọi task trong `after` của nó đã đóng (không `block`).
   c. Nếu `ready` không rỗng → `Open(ready)` (đã qua kiểm vai, tách vai, trần). Vòng lặp `AdvanceFlow` (tối đa `flowAdvanceMaxRounds`, `flow_engine.go:476-492`) tiếp tục cho tới khi hết `ready`.
   d. Tính `JoinOutcome` từ các hàng của lần vào bước này (4.3). Chưa hoàn tất → `Wait`. Bước `manual` luôn `Wait` (cho tới khi người chọn cạnh).
3. Hoàn tất → `route`: dựng `FactSet`, duyệt `S.edges` **theo thứ tự**; cạnh đủ điều kiện khi `trigger = auto`, `when` đúng (hoặc vắng), và nếu có `loop` thì `loops[loop] < max(loop)` ; **cạnh đầu đủ điều kiện thắng**. Không có cạnh nào → `stalled` (như `flow.go:408-410`).
4. Áp dụng cạnh (4.5, 4.6).

### 4.3 Hợp (`join`)

Với các task hoạt động của lần vào bước (hàng có `flow_round = activation`):

- *Trạng thái đóng*: giữ nguyên định nghĩa hiện có (`flow_engine.go:102-143`): issue `done`/`cancelled` **và** không còn lượt chạy hoạt động.
- *Class của task đã đóng*: từ `issue.outcome` tra bộ verdict của task (`pass`/`fail`/`neutral`), `block` nếu outcome là `blocked` (hoặc trạng thái `blocked` hệ thống ghi không outcome: coi là `block`, như `flow_engine.go:116-120`). Task bị hủy: gate → `fail`; task `work` bị hủy: **không** góp class, và mở lại theo quy tắc H3 (mục 5, E7).
- `block` (từ **bất kỳ** task, không chỉ `over`) với `block: immediate` hoàn tất ngay với `result = block` kể cả còn task mở (giữ `flow.go:376-382`).
- Gate đã đóng mà **không có verdict** (đường cũ, hoặc do người hủy) tính `fail`, như hôm nay. Task `superseded` (2.5) bị bỏ qua hoàn toàn.
- `mode: all`: chờ mọi task đã mở đóng **và** không còn `ready`/chưa mở. `result` = `fail` nếu có `fail` trong `over`; `pass` nếu có **ít nhất một `pass`** trong `over` và không `fail`; `none` trong mọi trường hợp còn lại (`over` rỗng, hoặc mọi task trong `over` đều `neutral`). Với đồ thị nâng từ schema 1 không có `neutral` nên kết quả trùng `tasksOutcomeOf` (`flow.go:455-488`). Bản trước của tài liệu cho "toàn bộ bỏ phiếu trắng" ra `pass`; đó là lỗi và đã sửa: không ai đồng ý thì không phải là qua.
- `mode: any`: hoàn tất `pass` khi có task `pass` đầu tiên trong `over`, engine hủy các task còn mở (`superseded`); nếu mọi task đóng mà không có `pass` → `fail` nếu có `fail`, ngược lại `none`.
- `mode: quorum k`: `pass` khi `pass ≥ k`; `fail` khi `pass + open + chưa_mở < k`; thực thể `neutral` đóng nhưng không tính `pass`. Khi hoàn tất, task còn mở bị hủy (`superseded`). Lúc chạy mà số thực thể khả dĩ nhỏ hơn `k` ngay từ đầu (danh sách ngắn): `fail` ngay, không mở task nào; bộ kiểm J01 ngăn trường hợp tĩnh.
- Dữ kiện `join.*` và `task.<key>.*` dựng từ chính các hàng này.

### 4.4 Song song và nối tiếp

Chỉ có một token (I8). **Song song nằm trong bước**: nhiều task (và nhiều thực thể) cùng `activation`; bước kế tiếp chỉ bắt đầu khi `join` hoàn tất và một cạnh được chọn. **Nối tiếp nằm giữa các bước** hoặc trong bước qua `after`. Không có "nhánh" tách ra các bước khác nhau cùng lúc (điểm mở rộng `fork/join`, 13).

### 4.5 Vòng

- Cạnh quay lại được chọn: `loops[L] += 1`; `activations[to] += 1`.
- Cạnh xuôi vào `header(L')` của vòng `L'`: `loops[L'] = 0` nếu `reset = on_entry` (mặc định). Cạnh xuôi vào bất kỳ bước: `activations[to] += 1`.
- Cạnh quay lại không đủ điều kiện (đã cạn) bị bỏ qua, xét cạnh kế.
- `max` dạng `{var, cap}` đọc `vars[var]` lúc đánh giá, kẹp ở `cap`.
- Chặn tổng: `flow.activations` > `policy.max_activations` hoặc tổng task issue > `policy.max_task_issues` → `stalled` với lý do `flow_cap_reached`; bộ kiểm G11 ngăn flow có thể vượt trần trong trường hợp xấu nhất.
- Đặt lại khi `resume` (4.7): tùy chọn `reset_loops`. `resume` vào một bước tính là một lần vào xuôi (tăng `activations`, đặt lại vòng có `header` là bước đó nếu `reset = on_entry`).
- Hai vòng cùng `header`: bộ đếm độc lập; cạnh quay lại của vòng này không đặt lại vòng kia (chỉ cạnh xuôi mới đặt lại).
- **Vì sao luôn dừng.** Mọi chu trình chứa ít nhất một cạnh quay lại (G04). Xét một request chạy mãi: trong các vòng có cạnh quay lại được đi vô hạn lần, lấy vòng L có `header` đứng sớm nhất theo thứ tự topo của đồ thị xuôi. Bộ đếm của L chỉ về 0 khi `header` được vào bằng cạnh xuôi, tức là request đã lùi về trước `header` bằng cạnh quay lại của một vòng có `header` sớm hơn; vòng đó chỉ được đi hữu hạn lần (theo cách chọn L). Vậy từ một lúc nào đó bộ đếm của L chỉ tăng và chạm `max`: mâu thuẫn. Vòng `reset: never` hiển nhiên có chặn.
- **Công thức chi phí (G11, `Analyze`).** *Thân* của vòng L = các bước nằm trên một đường xuôi từ `header(L)` tới nguồn của một cạnh quay lại của L. `lần_vào_tối_đa(bước) = ∏ (max(L) + 1)` trên mọi vòng L có thân chứa bước đó (vòng `max` theo biến tính bằng `cap`; vòng `reset: never` cũng nhân `max + 1`, là chặn trên an toàn). `số_task_tối_đa(bước)` = tổng số thực thể tối đa của các task (bỏ qua `when`). Đây là **chặn trên**, không phải số chính xác; trần lúc chạy ở trên mới là bất biến (E12), và test tính chất so chặn này với mô phỏng đối kháng.

### 4.6 Điểm dừng, thông báo, đích

- Cạnh `stop: C` → `state.stop = {code: C, from: S, edge, at}`; `blocked_from = S`; trạng thái request đặt thành `stops[C].status` (mặc định `blocked`); bình luận hệ thống từ `messages[stops[C].message]` (biến thay như `flow_engine.go:410-419`, thêm `{loop}`, `{stop}`); runner/quét bỏ qua request đang đỗ (như hôm nay `flow_engine.go:542`).
- Vào bước `manual` hoặc bước có `notice`: bình luận hệ thống từ `messages[notice]`. Đây thay "dừng 2" và "human": không đỗ, trạng thái request là `status` của bước.
- Cạnh `to: terminal T`: đặt `closed_at`; trạng thái = `T.status`; hủy task còn mở; với `T.leaves_workspace` kiểm điểm kiểm (4.8).
- `stalled` do engine: không cạnh, bước biến mất, quá hạn/quá lượt, trần, hồ bơi nhỏ, agent không dùng được (`flow_engine.go:724-794`); bình luận từ `messages.stalled` hoặc mặc định.

### 4.7 `resume`, bước `manual`, đóng thay

- **Resume** (`ResumeFlow`, `flow_engine_ops.go:224-`): chỉ `canManageSquad`. Mặc định về `blocked_from`; `to_step` tùy chọn (bước bất kỳ trừ terminal); `acknowledge_skip_checkpoint: true` bắt buộc khi request đang `stalled` mà `unreviewed = true` (công việc của agent chưa người nào xem, điển hình là `release_without_checkpoint`): thiếu cờ thì 422 `flow_checkpoint_ack_required` và request không đổi; có cờ thì resume tính là điểm kiểm, ghi `checkpoints += {step: đích, activation, by: người resume, at}` và xóa `unreviewed` (4.8). Giao diện chỉ gửi cờ sau khi người dùng tích ô xác nhận rõ ràng (CLI: `--acknowledge-skip-checkpoint`). Các resume khác (dừng 1, dừng 3, treo vì lý do khác) không cần cờ; `reset_loops: ["rounds"]|"all"`; `skip_cancelled: true` (H3). Task mở lại với `activation` mới nên không còn `RoundBase`. Không về được terminal (như `flow_engine_ops.go:241`).
- **Bước `manual`**: `POST …/flow/exits/{edgeId}` bởi người thuộc `exit_role` (hoặc quản lý). Ghi `checkpoints += {step, activation, by, at}`, kích hoạt cạnh như cạnh `auto` đã chọn. Kéo thẻ sang trạng thái khác (đường cũ) vẫn chạy qua `followManualMove`: ánh xạ trạng thái → bước **duy nhất** (nhiều bước chung trạng thái: 409 `flow_step_ambiguous`, dùng `resume --to-step`); sang terminal đóng request và ghi điểm kiểm.
- **Đóng thay** (D4): `POST /api/issues/{taskId}/flow/close` (7.3) hoặc `PUT` trạng thái trực tiếp bởi quản lý (luật 5, E1–E3). Luật verdict/tệp áp như cho người thực hiện; tệp do **người đóng thay** tải lên task được tính. Ghi `flow_manual_close`.

### 4.8 Điểm kiểm người và rời workspace

Điểm kiểm được ghi khi: người thuộc `exit_role` chọn cạnh `manual`; engine đi một *cạnh điểm kiểm* của bước `tasks` (định nghĩa ở Y01: `join.result = pass` với gate vai người trong `over`); quản lý kéo/đặt trạng thái hoặc `resume`.

Bản trước chỉ đòi "`checkpoints` không rỗng", quá yếu: người duyệt ở bước 2, agent viết lại ở bước 5, request vẫn ra ngoài được. Quy tắc mới (D20) theo dõi **công việc chưa được người xem**:

- `unreviewed = true` mỗi khi engine mở một task `work` giao cho vai `agent`.
- `unreviewed = false` mỗi khi một điểm kiểm được ghi.
- Vào terminal `leaves_workspace` khi `unreviewed = true` và `policy.allow_unattended_release` sai → `stalled` với lý do `release_without_checkpoint`. Đây là đai an toàn lúc chạy; bộ kiểm Y01 ngăn trường hợp tĩnh bằng cùng định nghĩa.

Task của agent nằm **trong** bước `manual` (như `export` của Kit Việt) được mở trước khi người chọn cạnh ra, nên điểm kiểm của cạnh ra phủ nó. Gate của agent không bật `unreviewed` (gate không làm ra sản phẩm). Đồ thị nâng từ schema 1 chạy với `allow_unattended_release = true` để không đổi hành vi của flow đang chạy (9.2 điều 12).

### 4.9 Hạn, thử lại, quét

Giữ `flow_sweep.go:35-243` và `DecideDeadline` (`flow_deadline.go:20-77`), đổi nguồn giới hạn: `deadline` của task nếu có, nếu không `policy`. Task do vai `agent`: như hôm nay (xếp lượt chạy, thử lại, hủy lượt quá hạn, `stalled`). Task do người: không xếp lượt chạy; quá `human_remind_minutes` đăng một bình luận nhắc người được giao (hoặc quản lý); quá `human_stall_minutes` → `stalled`. Quét vẫn đọc `idx_issue_flow_open`.

### 4.10 Mô phỏng

`Simulate(graph, vars, script, maxHops) Trace` dùng đúng `Decide` + `BuildFacts` với trạng thái giả (không DB). `script`: `{ "<stepKey>": [ { "<taskKey>[~i]": "<verdictKey>" } ... ] }` (một phần tử mỗi lần vào bước); mặc định task `work` → `done`, gate → verdict `pass` đầu tiên của bộ. Kết quả: danh sách bước (khóa bước, task mở, class, `join.result`, cạnh được chọn, bộ đếm vòng) và kết cục (`terminal`/`stop`/`stalled`), `truncated` khi quá `maxHops` (tối đa 120). Phân tích tĩnh đi kèm: `worst_case_task_issues`, `worst_case_activations`, `loops[]` (header, max), `stops[]`, `release_paths_without_human`.

### 4.11 Đầu vào của task và phản hồi của vòng sửa (D16)

Một workflow chạy ổn hay không phụ thuộc trước hết vào việc task sau nhận được đúng sản phẩm của task trước. Bản hiện tại lấy "bước gần nhất đứng trước theo chỉ số" (`flowTaskLinks`, `flow_engine.go:695-713`), không còn nghĩa khi bước là nút của đồ thị. Quy tắc mới, hàm thuần `TaskLinks(graph, state, tasks, task) (inputs, feedback)`:

- **Đầu vào tường minh.** Mỗi `Input{from, slot}` trỏ tới lần vào **gần nhất** của bước nguồn; với `instances`, mọi thực thể của task nguồn. Nguồn chưa từng chạy (nhánh không đi qua): bỏ qua, hoặc với `required: true` request đỗ `stalled` với lý do `input_missing` trước khi mở task.
- **Đầu vào mặc định** (task không khai `inputs`): đi ngược `trail` từ bước hiện tại, lấy bước gần nhất có ít nhất một task `work` đã đóng với class `pass`; đầu vào là các task `work` đó. Trong cùng bước, task có `after` nhận thêm các task nó đứng sau. Gate nhận như vậy nên mặc định phán sản phẩm của bước làm việc liền trước trên đường đã đi.
- **Phản hồi.** Khi lần vào bước hiện tại là do một cạnh quay lại (phần tử cuối của `trail` có cạnh mang `loop`), `feedback` = các task của bước nguồn cạnh đó, ở lần vào vừa kết thúc, có class `fail` (gate từ chối) hoặc `block`. Mô tả task ghi "đọc những gì các task này tìm ra và sửa", như mục Rework hôm nay (`flow.go:935-940`), nay đúng cho mọi vòng chứ không chỉ một vòng `rework`.
- **Hiển thị.** Mô tả và lời nhắc liệt kê từng nguồn: định danh issue, khóa task, tên khe (nếu có `slot`), lệnh tải tệp. Engine không đọc, không sao chép, không tóm tắt nội dung.
- Đồ thị `compat: "v1"` giữ `flowTaskLinks` theo vị trí (9.2 điều 9) để văn bản của flow đang chạy không đổi.

### 4.12 Biến do task đặt (D17)

Một agent lập kế hoạch thường là người biết danh sách nguồn, số vòng cần chạy, mức độ nghiêm trọng. Bản trước chỉ cho lead, quản lý, người yêu cầu đặt biến, nên mọi quyết định bằng dữ liệu phải dồn về lead. Quy tắc mới:

- Người thực hiện task (agent hoặc người được giao; quản lý đóng thay) gọi `PUT /api/issues/{taskId}/flow/vars/{key}` khi task **đang mở** và `key ∈ Task.sets`. Máy chủ kiểm kiểu, miền, `max_items`, `max_len` như mọi lần đặt biến; ghi `activity_log` `flow_var_set` kèm task và người đặt; tăng `seq`. Sai thì 403 `flow_var_forbidden` hoặc 422 `flow_var_invalid`.
- `requires.vars`: task không đóng được (`flow_vars_required`, kèm tên biến) nếu biến chưa được đặt bởi task này ở lần vào bước hiện tại. Engine ghi nhận việc đặt trong `issue.flow.var_sets["<step>.<task>"] = activation`.
- Giá trị là **dữ liệu có kiểu qua API**, cùng mức tin cậy với verdict (I5). Văn bản trong comment hay tệp không bao giờ thành biến. Phần tử của `list_string` vẫn chỉ đi vào mô tả task dưới dạng trích dẫn (2.5).
- Biến do task đặt không nới được bất biến nào: trần vòng vẫn kẹp ở `cap`, số thực thể kẹp ở `max`, điểm kiểm người không phụ thuộc biến (Y01 bỏ qua điều kiện trên `var.*` khi tính đường: coi mọi cạnh đều đi được).
- Lời nhắc do máy chủ sinh nêu lệnh: `multica issue flow set <id> <key> <value>` cho từng biến trong `sets`, kèm kiểu và miền.

### 4.13 Đường đã đi và nhật ký quyết định

Mỗi lần request rời một bước, engine (trong cùng giao dịch với `SetIssueFlow`) thêm một phần tử vào `trail` và ghi `activity_log` hành động `flow_transition` với `details = { from, activation, edge, to | stop, join, loops, facts }`, trong đó `facts` là các dữ kiện **được cạnh thắng và các cạnh đứng trước nó tham chiếu** (không phải toàn bộ sổ). Nhờ vậy câu hỏi "vì sao request đi đường này" trả lời được từ dữ liệu, và `GET /api/issues/{id}/flow` trả `trail` cho màn hình request. Không thêm bảng.

## 5. Bất biến engine luôn cưỡng chế

Bốn lỗ hổng cần vá (D5), tên gọi dùng trong tài liệu: **H1** agent đóng task của agent khác; **H2** agent đổi người nhận của task flow mà không đổi trạng thái; **H3** agent hủy task `work` có yêu cầu tệp để bước chuyển mà thiếu đầu ra; **H4** agent đổi verdict của task đã đóng trước khi engine chuyển bước; **H1m** biến thể của H1: nút Approve/Reject trong bảng task (commit `ccb6892bf`) hiện cho mọi người xem và máy chủ chấp nhận mọi thành viên bấm. Cả năm là **nghi vấn đọc từ mã, chưa chạy**: cơ sở là `checkFlowTaskStatus` không nhận actor (`flow_guards.go:248`), `checkFlowWrite` trả sớm khi trạng thái không đổi (`flow_guards.go:137-139`) và chỉ kiểm khi đích thuộc nhóm `done` (`flow_guards.go:274`). Bước P1 của kế hoạch viết test đỏ-trước cho từng cái; cái nào test cho thấy đã có luật khác chặn thì bỏ dòng tương ứng, không thêm mã.

Không phụ thuộc flow viết gì; mỗi cái có chỗ cưỡng chế và test tên. Cột "Trạng thái": đã có / mới (D5 cho E1–E7).

| # | Bất biến | Cưỡng chế | Mã lỗi | Trạng thái |
| --- | --- | --- | --- | --- |
| E1 | Agent chỉ đóng task **được giao cho nó**; người đóng task phải là người được giao hoặc `canManageSquad` (đóng thay, có nhật ký) | `checkFlowTaskStatus(…, actor)` ở `UpdateIssue` (`issue.go:4254`) và batch (`issue.go:5052`) | `flow_task_not_assignee` 403 | mới (H1) |
| E2 | Agent không đổi người nhận của task flow | `checkFlowWrite` không trả sớm khi chỉ đổi `assignee_*` (`flow_guards.go:136-139`) | `flow_task_assignee_locked` 409 | mới (H2) |
| E3 | Agent không hủy task flow; người hủy thì task `work` mở lại trừ khi `skip_cancelled` | `checkFlowTaskStatus` + `loadFlowTasks` | `flow_task_cancel_forbidden` 409 | mới (H3) |
| E4 | Verdict đã đóng trong lần vào bước hiện tại không đổi bởi agent; người mở lại bằng thao tác riêng có nhật ký | `checkFlowTaskStatus` | `flow_task_closed` 409 | mới (H4) |
| E5 | Verdict thuộc bộ verdict của task (hoặc `blocked`); không `blocked` kèm trạng thái `done`; gate chỉ đóng với verdict class `pass`/`fail` | `checkFlowTaskStatus` (`flow_guards.go:263-300`) tổng quát từ `approved`/`rejected` sang bộ verdict | `flow_outcome_required`, `flow_outcome_invalid` (giữ mã, `allowed_outcomes` là khóa verdict) | đổi |
| E6 | Task `requires.files` chỉ đóng khi đủ tệp theo từng `slot`: đúng phần mở rộng/`content_type`, đủ `min`, do người thực hiện (hoặc người đóng thay) tải lên **chính issue task** | `agentAttachmentCount` (`flow_guards.go:319`) → `countRequiredFiles` | `flow_attachment_required` (giữ mã) + `slot` | đổi |
| E7 | Task `work` bị hủy không làm bước qua mặt tệp: mở lại ở cùng `activation` (mã nhật ký `flow_task_reopened`) | `advanceOnce` | — | mới (H3) |
| E8 | Agent không đổi trạng thái request, không rời/vào bước `manual`, không tạo sub-issue dưới request | `checkFlowRequestMove` (`flow_guards.go:159-`), `checkFlowChildCreate` (`:214`) | `flow_managed_request`, `flow_human_only`, `flow_children_managed` | có |
| E9 | Chỉ engine ghi `issue.flow`, `flow_step`, `flow_task`, `flow_round`; API công khai bỏ qua | `TestGuard_PublicAPIIgnoresFlowColumns` | — | có |
| E10 | Mọi dữ kiện từ ba nguồn của 2.6; trường lạ bị từ chối lúc ghi, không `false` | `ValidateFlowDoc`, `BuildFacts` | `unknown_fact` | mới |
| E11 | Bộ đếm vòng tăng chỉ khi engine đi cạnh quay lại; cạnh quay lại cạn bị bỏ qua; không vượt `max` | `Decide` | — | mới |
| E12 | Không vượt `max_activations`, `max_task_issues` | `openFlowTasks`, `route` | `flow_cap_reached` (dừng `stalled`) | mới |
| E13 | Gate không giao cho người/agent đã thực hiện task `work` **mà nó phán** (các task trong đầu vào của nó theo 4.11, và task `work` cùng bước ở cùng lần vào). Hồ bơi còn người khác thì engine chọn người đó (2.5); không còn ai thì từ chối. Tĩnh ở R07 | `openFlowTasks` (mở rộng `flow_engine.go:608-620`) | `flow_gate_same_agent` | đổi (D19) |
| E14 | Task chỉ giao cho thành viên squad còn hoạt động, không phải lead (trừ `lead_may_work`) | `openFlowTasks` | `flow_agent_unavailable` | mới |
| E15 | Vào terminal `leaves_workspace` đòi `unreviewed = false`: không còn task `work` của agent mở sau điểm kiểm người gần nhất (trừ `allow_unattended_release` có ack) | `route` | `stalled` / `release_without_checkpoint` | mới (D20) |
| E16 | Request đang đỗ chỉ đi tiếp bằng `resume` của quản lý | `advanceOnce` (`flow_engine.go:542`) | — | có |
| E17 | Đặt biến chỉ bởi người thuộc `set_by`, ở `set_at` (quản lý bất kỳ lúc, ghi nhật ký); kiểm kiểu, miền | `SetFlowVar` | `flow_var_forbidden`, `flow_var_invalid` | đổi (`flow_engine_ops.go:169-223`) |
| E18 | Phiên bản đã xuất bản không sửa được; request ghim phiên bản | `workflowLocked` | `workflow_locked` | mới (mục 8) |
| E19 | Mọi thay đổi trạng thái request có khóa dòng và `seq` so-sánh-đổi; chạy lại no-op | `LockIssueForFlow`, `SetIssueFlow` | — | đổi |
| E20 | Agent không gọi API soạn/xuất bản | route soạn | `flow_authoring_member_only` | mới (mục 8) |
| E21 | Workflow không có `flow_config` (SDLC, BMAD, Default, dự án) không bị đụng | ranh giới `Definition.Flow != nil` (`flow_engine.go:61-83`, `service/flow_start.go:42-46`) | — | có |
| E22 | Task chỉ đặt biến có trong `Task.sets` của nó, khi đang mở, bởi người thực hiện; task có `requires.vars` không đóng khi chưa đặt | `SetFlowVar` (đường task), `checkFlowTaskStatus` | `flow_var_forbidden`, `flow_var_invalid`, `flow_vars_required` | mới (D17) |
| E23 | Chỉ engine ghi outcome `superseded`; API từ chối giá trị đó từ mọi actor | `issue.go` (kiểm `outcome`), `checkFlowTaskStatus` | `flow_outcome_invalid` | mới (D21) |
| E24 | Biến đang được `instances` của bước hiện tại dùng không đổi được sau khi thực thể đầu tiên đã mở | `SetFlowVar` | `flow_var_in_use` 409 | mới (D21) |
| E25 | Task có đầu vào `required` mà nguồn chưa chạy thì không mở | `openFlowTasks` | `stalled` / `input_missing` | mới (D16) |

Ghi chú: `FF_FLOW_ENGINE=off` vẫn tắt guard (`flow_guards.go:141`, `:147`, `:215`, `:237`); giữ là công tắc khẩn cấp, cần hiện cảnh báo quản trị khi tắt.

## 6. Hợp đồng vai và skill

### 6.1 Vai

Vai là khai báo của người có quyền (D2), được biên dịch và khóa (I3). Engine **chỉ tin** vai đã khai báo trong phiên bản đã ghim. Khóa vai không bao giờ lấy từ văn bản skill hay tên agent.

### 6.2 Skill bundle đóng góp gì

Chỉ **văn bản**: tên, mô tả, hướng dẫn agent, văn bản skill, và (tùy chọn) một tài liệu FlowDoc **đề xuất** không có `bind`. Mọi thứ qua bộ kiểm 3 như bản nháp của người dùng; không có đường tắt. Bộ không có FlowDoc (trường hợp thường gặp: quy trình nằm trong `SKILL.md` của agent điều phối, hoặc không có quy trình nào) vẫn nhập được; workflow được soạn sau, bằng tay hoặc từ đề xuất của agent (mục 16).

Điều engine lấy từ skill đúng một thứ: **tên skill**, để `Task.skills` trỏ tới và R09 kiểm agent có nó. Nội dung skill không được phân tích.

### 6.3 Engine không bao giờ tin

Dòng `VERDICT:` hay bất kỳ kết luận nào trong comment; lời khẳng định đã đính kèm tệp; danh sách bước, thứ tự, lệnh trong `SKILL.md`; tên vai, tên agent; mọi UUID trong bó; `skill.config`; lời trong skill bảo agent tạo sub-issue, đổi trạng thái request, gọi `multica issue flow` (máy chủ từ chối: E8). **Lời nhắc cho agent là do máy chủ sinh** từ định nghĩa (`FlowTaskBrief`, `FlowLeadBrief`, `TaskDescription` — `flow.go:920-1077`), nay sinh từ bộ verdict và `requires.files` của từng task, nên agent được nói đúng lệnh và đúng khóa verdict dù skill nói sai.

### 6.4 Điểm cắm cho pha F (thuộc đợt này, phát hành R3; chi tiết ở mục 16 và Phụ lục E)

Bảng dưới là trạng thái của từng điểm cắm sau R2 (cột giữa) và sau R3 (cột phải). Chỗ nào bảng này khác mục 16 thì mục 16 đúng.

| Điểm cắm | Sau R2 | Sau R3 (pha F) |
| --- | --- | --- |
| `Role.source {bundle, agent_key}` | có trường, engine bỏ qua, giao diện hiển thị | điền khi cài (`bundle` = `pack_install_id`) |
| Định dạng bó | không | bốn định dạng ở 16.1; bó gốc `multica.squad-bundle/v1` (Phụ lục E) là một trong số đó, với `flow.json` là FlowDoc schema 2 không `bind` |
| Bảng `squad_pack_install` | không tạo | tạo ở R3 (cột ở 16.3), chỉ mục `CONCURRENTLY` |
| Route | dành tên `POST /api/squads/packs/preview`, `/commit` (chưa có) | thêm |
| Nguồn bản nháp | `from.kind ∈ {blank, workflow, manifest}` | thêm `proposal` (16.4); bản nháp từ `flow.json` của bộ do `commit` tạo (16.3) |
| Cờ khả năng | `features.packs = false`, `features.proposals = false` | `true` (sau cờ `FF_FLOW_PACKS`) |
| Quét | — | `hub.Scan` (`server/internal/hub/scan.go`) và giới hạn `hub.DefaultLimits`; không dùng `hub_item` vì `kind` bị CHECK `('skill','plugin')` (`migrations/930_hub_item.up.sql:7`) |
| Quan hệ vai ↔ squad | vai `agent` phải là thành viên squad | cài bó thêm agent vào squad rồi ghép `bind` |

## 7. API

Mọi route cần `X-Workspace-ID`. Lỗi: `{ "error", "code", ...fields }` (`writeErrorCode`, `writeFlowRejection`). Mọi phản hồi mới qua zod + `parseWithFallback` (`packages/core/api/schema.ts:38`), có mặc định, enum lạ có nhánh dự phòng (AGENTS.md "API Compatibility"). Mọi route mới sau cờ `FF_FLOW_AUTHORING` (soạn) hoặc `FF_FLOW_ENGINE` (chạy; cờ có sẵn, `featureflags/keys.go:43`, mặc định bật).

### 7.1 Mới (chỉ thêm)

| Route | Quyền | Mô tả |
| --- | --- | --- |
| `GET /api/flow/capabilities` | thành viên | Trần, loại, dữ kiện dựng sẵn, `features` (2.7) |
| Nhóm `/api/squads/{id}/flow-drafts…` | `canManageSquad`, không agent | Xem 7.6. `POST` tạo bản nháp với `from.kind = workflow` nhận **mọi** hàng workflow flow, kể cả hàng schema 1 do catalog tạo (qua `Lift` sạch, 9.2) |
| `POST /api/issues/{id}/flow/steps/{step}/complete` | lead của squad hoặc `canManageSquad` | Hoàn tất bước `lead`: `{ "verdict": "planned", "vars": {…}, "blocked": {"missing": […], "reason": "…"} }` |
| `PUT /api/issues/{id}/flow/vars/{key}` | như `set_by` / quản lý | `{ "value": … }`; đồng nghĩa mở rộng của `…/switches/{key}` |
| `POST /api/issues/{id}/flow/exits/{edgeId}` | `exit_role` hoặc quản lý | Chọn cạnh `manual` |
| `POST /api/issues/{taskId}/flow/close` | `canManageSquad` | Đóng thay: `{ "verdict": "approved", "note": "…" }`; tệp đã tải lên task bởi chính người gọi |
| `GET /api/issues/{id}/flow` (mở rộng) | thành viên | Thêm: `schema`, `loops`, `activations`, `vars`, `facts`, `stop` dạng đối tượng (`code`, `legacy_no`), `checkpoints`, `unreviewed`, `trail`, `trial`, `viewer: { can_manage, closable_task_ids }`, mỗi task thêm `instance`, `class`, `closed_by { type, id, on_behalf }`, `slot_status`, `inputs`, `feedback`, `sets`, `verdicts: [{key, class}]` (bộ verdict của task: bộ nó khai báo, hoặc bộ dựng sẵn `work`/`gate` của 2.3; `blocked` luôn được phép và không nằm trong bộ). `stop` là đối tượng `{code, legacy_no, from, edge, at}` hoặc `null`; `stop_info` không tồn tại |
| `PUT /api/issues/{taskId}/flow/vars/{key}` | người thực hiện task (agent hoặc người), quản lý | Đặt biến từ một task (4.12): `{ "value": … }`. Trên issue **task**, không phải request |
| `POST /api/flow/validate` | thành viên **và agent** | Kiểm không trạng thái: `{ "manifest": {…}, "squad_id"?: "uuid" }` → `FlowValidation` (như 7.3). Không đọc, không ghi bản nháp; có `squad_id` thì kiểm vai theo danh sách của squad đó (người gọi phải thấy squad). Đây là route duy nhất của phần soạn mà agent gọi được (16.4). Giới hạn tần suất theo actor |
| `POST /api/issues/{id}/flow/start` (mở rộng) | như hiện có (`authorizeFlowOperator`) | Hôm nay route luôn lấy `squad.active_workflow_id` (`flow_api.go:289-303`). Thêm thân tùy chọn `{ "workflow_id"?: "uuid", "trial"?: bool }`: `workflow_id` phải là workflow flow của **chính squad được giao** (16.5): một phiên bản bất kỳ của mọi lineage của squad, hoặc bản đang dùng. Ngược lại `422 flow_workflow_invalid` (không phải `flow_cannot_start`, mã đó vẫn dành cho request không khởi động được). `trial: true` ghim phiên bản được chọn (hoặc bản đang dùng nếu không có `workflow_id`), đặt `trial = true` trong trạng thái, và chỉ dành cho **thành viên** quản lý squad (lead là agent nhận `403 flow_forbidden`). Workflow schema 1 không chạy thử được (`422 flow_workflow_invalid`) vì trạng thái schema 1 không có chỗ ghi `trial`. Không có thân: hành vi cũ |
| `POST /api/issues` (mở rộng) | thành viên | Thêm trường tùy chọn `flow_workflow_id`: `StartFlowIfEligibleOn` (`service/flow_start.go`) dùng nó thay cho phiên bản đang dùng; kiểm như route `flow/start` **trước khi ghi issue** (`422 flow_workflow_invalid`, không tạo gì). Thêm trường tùy chọn `flow_trial` (cùng route, để một lần tạo là một lần chạy thử như 16.5 mô tả): chỉ thành viên quản lý squad (`403 flow_forbidden`), không có `flow_workflow_id` thì chạy thử bản đang dùng. Actor agent gửi hai trường này bị bỏ qua |
| `GET /api/issue-workflows/{id}/flow-insights` | `canManageSquad` | Số liệu vận hành của một phiên bản (16.6) |
| `POST /api/squads/packs/preview`, `POST /api/squads/packs/commit` | admin workspace (tạo agent và skill) | Nhập bộ skill/agent (16.1 đến 16.3) |

### 7.2 Giữ nguyên, hành vi mở rộng

`plan` = `steps/{startStep}/complete` với verdict `planned`/`blocked`. `…/switches/{key}` = `vars/{key}`. `advance`, `resume` (thêm `reset_loops`, `skip_cancelled`, `acknowledge_skip_checkpoint`), `detach`, `start`, `PUT /api/issue-workflows/{id}/flow-auto` giữ nguyên (admin). `PATCH /api/issues/{id}` với `outcome`: chuỗi khớp `^[a-z][a-z0-9_]{0,31}$`; với task flow, kiểm theo bộ verdict (E5); với issue khác vẫn bốn giá trị (`issue.go:4247`, `:5033`).

### 7.3 Ví dụ

`validate` (rút gọn):

```json
{ "ok": false, "revision": 4,
  "issues": [ {"code":"role_is_lead","severity":"error","path":"roles[planner]","params":{"role":"planner"},"message":"…"},
              {"code":"release_without_human","severity":"warning","path":"terminals[done]","params":{},"message":"…","ack":"allow_unattended_release"} ],
  "analysis": { "worst_case_task_issues": 54, "worst_case_activations": 21,
                "loops": [ {"key":"fix","header":"draft","max":2} ],
                "stops": [ {"code":"grounds_missing","edge":"gate_blocked","step":"gate"} ],
                "release_paths_without_human": [] },
  "fact_catalog": { "edge:gate": [ {"key":"join.result","type":"enum","values":["pass","fail","block","none"],"ops":["eq","neq","in"]} ],
                    "task_when:collect": [ {"key":"var.customs_lookup","type":"bool","ops":["eq","neq"]} ] } }
```

`close` (đóng thay):

```json
POST /api/issues/{taskId}/flow/close   { "verdict": "approved", "note": "Agent offline; checked by hand" }
→ 200 { "task": {…, "closed_by": { "type":"member", "id":"…", "on_behalf": true }}, "state": {…} }
→ 403 flow_forbidden · 422 flow_outcome_invalid · 422 flow_attachment_required (slot "report")
```

### 7.4 Mã lỗi mới

`flow_schema_unsupported` 400, `flow_doc_invalid` 422, `flow_feature_unsupported` 422, `flow_task_not_assignee` 403, `flow_task_assignee_locked` 409, `flow_task_cancel_forbidden` 409, `flow_task_closed` 409, `flow_var_forbidden` 403, `flow_var_invalid` 422, `flow_exit_forbidden` 403, `flow_edge_unavailable` 409, `flow_step_ambiguous` 409, `flow_cap_reached` (dừng `stalled`, không phải HTTP), `flow_vars_required` 422, `flow_var_in_use` 409, `pack_invalid` 400, `pack_scan_blocked` 422, `pack_conflict` 409. Lý do `stalled` mới (không phải HTTP): `input_missing`, `instances_too_few`, `pool_too_small`, `release_without_checkpoint`. Mã của bản nháp và xuất bản ở 7.7. Mã cũ `flow_outcome_required`, `flow_outcome_invalid`, `flow_attachment_required` giữ nguyên tên. `flow_checkpoint_ack_required` 422 (resume một request treo vì công việc agent chưa ai xem, thiếu `acknowledge_skip_checkpoint`; kèm `step`). Mã của lớp soạn do vòng tích hợp thêm, ngoài bảng 7.7: `flow_graph_v2_disabled` 404, `flow_ack_forbidden` 403, `flow_ack_unknown` 400, `flow_validate_rate_limited` 429, `flow_workflow_invalid` 422, `flow_forbidden` 403, `flow_publish_forbidden` 403, `flow_draft_not_found` 404, `flow_draft_too_large` 422, `flow_draft_name_invalid` 400, `flow_draft_revision_required` 400, `flow_draft_from_unsupported` 400; ý nghĩa ở `flow-authoring-runbook.md`.

### 7.5 Tương thích client cũ (desktop cài sẵn)

- Phản hồi workflow của schema 2 giữ `steps[]` kiểu cũ: mỗi trạng thái một phần tử, **kèm chiếu** `tasks`/`human_only` của các bước ánh xạ vào trạng thái đó (hàm thuần `ProjectLegacySteps`), để `workflow-graph-sheet.tsx` cũ hiển thị được (suy giảm, không lỗi). `flow_config.version = 2` là tín hiệu cho client mới.
- `stop` trong `issue.flow` / `IssueFlowRef` vẫn có thể là `1`, `3`, `"stalled"`; mã mới là chuỗi; client cũ đã rơi về `"stalled"` cho giá trị lạ (`flow.go:262-277` phía Go; `packages/core/api/schemas.ts:1663-1668` phía client).
- CLI: `multica issue status --outcome` hiện kiểm cứng bốn giá trị ở máy khách (`cmd_issue.go:2199-2200`); phải đổi sang chuyển nguyên chuỗi và để máy chủ kiểm. **Daemon cài sẵn có CLI cũ sẽ từ chối verdict mới ngay ở máy khách.** Danh mục khả năng trả `min_cli_version` cho verdict tùy chỉnh; xuất bản flow dùng verdict tùy chỉnh hiện cảnh báo `cli_version` nếu có runtime của workspace báo phiên bản cũ hơn. Nguồn phiên bản: daemon báo `cli_version` lúc đăng ký (`handler/daemon.go:779`), máy chủ lưu ở `metadata.cli_version` của runtime và đọc bằng `readRuntimeCLIVersion` (`handler/issue.go:3083`); tiền lệ kiểm phiên bản tối thiểu là `checkQuickCreateDaemonVersion` (`handler/issue.go:3011`). Giá trị tối thiểu cụ thể chưa xác định (mục 14.2).

### 7.6 Route bản nháp và xuất bản (dưới `/api/squads/{id}`, cạnh `router.go:2477-2479`)

Mọi route soạn: actor phải là thành viên (`resolveActor` khác agent) và `canManageSquad` (`squad.go:155`), trừ khi nêu khác. Cờ `FF_FLOW_AUTHORING` tắt thì mọi route mới trả 404 `flow_authoring_disabled`.

| Method và đường dẫn | Body | Phản hồi |
| --- | --- | --- |
| `GET /flow-drafts` | | `200 {"drafts": [FlowDraftSummary], "total": n}` |
| `POST /flow-drafts` | `{"name": "...", "from": {"kind": "blank"} \| {"kind": "workflow", "workflow_id": "uuid"} \| {"kind": "manifest", "manifest": {…}} \| {"kind": "proposal", "agent_id": "uuid", "references": […], "note": "…"}}` (`proposal` ở R3, 16.4) | `201 FlowDraft` |
| `POST /flow-drafts/{draftId}/import-proposal` | `{"revision": 1}` | `200 FlowDraft` (đọc tệp `flow.json` mới nhất do agent soạn đính ở issue đề xuất; kiểm như nhập manifest); `404 flow_proposal_missing` khi chưa có tệp |
| `GET /flow-drafts/{draftId}` | | `200 FlowDraft` |
| `PATCH /flow-drafts/{draftId}` | `{"revision": 3, "name"?: "...", "doc"?: {…}}` | `200 FlowDraft` (revision + 1); `409 flow_draft_conflict` |
| `DELETE /flow-drafts/{draftId}` | | `204` |
| `POST /flow-drafts/{draftId}/validate` | `{}` hoặc `{"doc": {…}}` (kiểm tài liệu chưa lưu) | `200 FlowValidation` (luôn 200, kể cả có lỗi; ví dụ ở 7.3) |
| `POST /flow-drafts/{draftId}/simulate` | `{"doc"?: {…}, "vars": {…}, "script": {…}, "max_hops": 60}` | `200 FlowTrace` |
| `POST /flow-drafts/{draftId}/publish` | `{"revision": 3, "ack": ["allow_unattended_release"], "make_active": false}` | `201 IssueWorkflowResponse` |
| `GET /flow-drafts/{draftId}/export` | | `200 FlowManifest` (tải về) |
| `GET /api/issue-workflows/{id}/flow-manifest` | | `200 FlowManifest` (quản lý squad của hàng đó) |

`FlowDraft`: `{"id","squad_id","name","lineage_id","base_workflow_id","revision","doc","origin","source_ref","created_by","updated_by","created_at","updated_at"}`. `FlowDraftSummary`: bỏ `doc`, thêm `"summary": {"steps","tasks","gates"}`. `FlowManifest`: `{"format": "multica.flow-manifest/v1", "name": "...", "doc": {…FlowDoc schema 2, roles không có bind…}}`. Nhập: `from.kind = manifest` kiểm đúng như bản nháp (mục 3), từ chối khóa lạ, `doc.version` hoặc `format` cao hơn máy chủ biết, và tài liệu quá 512 KiB (`flow_manifest_invalid`).

`FlowTrace` (mô phỏng chỉ qua `Decide` và `BuildFacts`, **không** ghi DB, không chạy agent), rút gọn:

```json
{
  "hops": [
    {"n": 0, "step": "plan", "action": "complete", "verdict": "planned", "edge": "plan_ok", "to": "collect"},
    {"n": 1, "step": "collect", "opened": ["docs"], "closed": {"docs": "done"}, "join": "none", "edge": "collect_ok", "to": "draft"},
    {"n": 2, "step": "gate", "activation": 1, "closed": {"docs_gate": "rejected"}, "join": "fail", "edge": "gate_fix", "to": "draft", "loops": {"fix": 1}}
  ],
  "end": {"kind": "stop", "code": "loop_exhausted", "from": "gate"},
  "truncated": false
}
```

`end.kind` là `terminal`, `stop` hoặc `stalled`. `script` và `max_hops` như 4.10. Mô phỏng chứng minh định tuyến, giới hạn vòng và điểm dừng; **không** chứng minh agent làm đúng việc (giao diện ghi rõ điều này).

### 7.7 Mã lỗi của bản nháp và xuất bản

| Mã | HTTP | Khi nào |
| --- | --- | --- |
| `flow_authoring_disabled` | 404 | `FF_FLOW_AUTHORING` tắt |
| `flow_authoring_member_only` | 403 | Actor là agent |
| `flow_publish_forbidden` | 403 | Không có `canManageSquad` |
| `flow_agent_forbidden` | 403 | Người xuất bản không được phép gắn agent (`memberCanWireAgent`) |
| `flow_draft_conflict` | 409 | `revision` lệch |
| `flow_draft_stale` | 409 | Phiên bản nền không còn mới nhất (kèm `latest_workflow_id`) |
| `flow_draft_invalid` | 422 | Có lỗi kiểm tra; kèm `issues[]` |
| `flow_ack_required` | 422 | Cảnh báo cần xác nhận mà thiếu `ack` (kèm `required`) |
| `flow_manifest_invalid` | 400 | Manifest sai định dạng, khóa lạ, quá giới hạn |
| `flow_limit_reached` | 409 | Quá 20 bản nháp mỗi squad hoặc 50 phiên bản mỗi lineage |
| `workflow_locked` | 409 | Ghi vào hàng đã xuất bản (`PATCH`, `PUT …/handoffs/{status}`) |

Mã có sẵn dùng lại: `workflow_name_taken`, `status_create_forbidden`, `invalid_workflow`, `workflow_has_flow_requests`, `workflow_is_squad_active`, `flow_agent_unavailable`.

## 8. Phiên bản, xuất bản, cờ, đường lui

### 8.1 Xuất bản: các bước trong một giao dịch

1. Cờ `FF_FLOW_AUTHORING` bật; actor là thành viên (không phải agent) có `canManageSquad`, nếu không 403 `flow_publish_forbidden`.
2. Mở giao dịch; `LockIssueStatusCatalog` (exclusive, như `CreateIssueWorkflow`, `issue_workflow.go:834`) rồi `LockSquadForWorkflowChange` (`squad.sql:32`).
3. Đọc bản nháp theo `(id, workspace_id, squad_id)`. `revision` lệch thì 409 `flow_draft_conflict`. `base_workflow_id` khác phiên bản mới nhất của lineage thì 409 `flow_draft_stale` kèm `latest_workflow_id`.
4. Đọc danh sách vai của squad: lead (`squad.leader`) và thành viên agent (`ListSquadMembers`, `squad.sql:140`).
5. `ValidateFlowDoc` với toàn bộ mục 3; gom **mọi** vấn đề, không dừng ở vấn đề đầu. Có lỗi thì 422 `flow_draft_invalid` kèm `issues[]`. Thiếu xác nhận cho cảnh báo cần `ack` thì 422 `flow_ack_required` kèm `required`.
6. Với từng agent đã gắn: `memberCanWireAgent` (`squad.go:171`) cho người xuất bản, nếu không 403 `flow_agent_forbidden` kèm `agent_id`.
7. Phân giải trạng thái bằng `resolveStepInputs` (`issue_workflow.go:612`) với `allowCreate = isWorkspaceAdmin(member)`; tạo trạng thái mới mà không phải admin thì 403 `status_create_forbidden` (có sẵn).
8. Biên dịch (`bind` điền từ vai) và ghi hàng mới qua hàm nội bộ dùng chung với `CreateIssueWorkflow`: `flow_config` (version 2), `steps` (danh sách trạng thái), `lineage_id` (mới nếu NULL), `version = max + 1`, `published_at = now()`, `published_by`, `squad_id`, `owner_id`, `flow_auto = false` (D9), `template_ref = NULL`.
9. Commit; sau commit phát `issue_workflow:changed` (action `published`) và `flow_draft:updated`.
10. Bản nháp **được giữ** (với `base_workflow_id` = phiên bản vừa tạo) để sửa tiếp. `make_active: true` chỉ là tiện ích client: sau 201, client gọi `POST /api/squads/{id}/workflow` (`squad_workflow.go:180`) để tái dùng nguyên vẹn luật di chuyển trạng thái của lệnh đó.

Phiên bản mới không gắn vào request nào cho tới khi request mới khởi tạo: `service/flow_start.go:23` lấy `squad.active_workflow_id`.

### 8.2 Phiên bản và nhân bản

- **Lineage.** Bản nháp xuất bản lần đầu sinh `lineage_id` mới và `version = 1`; mỗi lần sau `version + 1`. Bản nháp tạo từ một phiên bản kế thừa `lineage_id`.
- **Bất biến.** Hàng có `published_at` khóa ghi (I2). Không có API sửa tại chỗ; muốn đổi, tạo bản nháp từ phiên bản (`from.kind = workflow`).
- **Đang dùng và request.** `squad.active_workflow_id` trỏ tới một phiên bản. Request mới ghim phiên bản đang dùng lúc khởi tạo. Request đang chạy giữ phiên bản đã ghim cho đến khi kết thúc hoặc được `detach`. Phiên bản chỉ xóa được khi không phải bản đang dùng và `open_flow_requests = 0` (chặn có sẵn, `issue_workflow.go:1202-1260`).
- **Đặt phiên bản mới làm đang dùng.** Dùng `POST /api/squads/{id}/workflow` có sẵn. Workflow flow không dời trạng thái issue thường (kế hoạch rỗng khi `flow_config` khác NULL, `squad_workflow.go:312-318`; test `TestSetSquadActiveWorkflow_FlowWorkflowDoesNotRemap`, `flow_squad_workflow_test.go:40`).
- **Quay lui.** Đặt lại phiên bản cũ làm đang dùng; không cần migration. Request đã chạy trên phiên bản lỗi tiếp tục trên phiên bản đó; người quản lý dùng `detach` hoặc `resume` có sẵn.
- **`flow_auto`.** Không kế thừa sang phiên bản mới (D9; như workflow catalog, `kitviet-flow-rehearsal.md` mục 5); admin bật lại sau khi xem phiên bản. Hộp thoại xuất bản nhắc điều này khi phiên bản đang dùng đang `auto`.
- **Nhân bản (D8).** `from.kind = workflow` nhận mọi hàng: hàng schema 2 sao chép FlowDoc (bỏ `bind` nếu người dùng chọn gắn lại); hàng schema 1 (kể cả hàng Kit Việt do catalog tạo) đi qua `Lift` với `opts.Runtime = false` (9.2) cho tài liệu schema 2 sạch, vai `a_<8 ký tự đầu của uuid>` đã gắn sẵn agent và nhãn lấy tên agent, người dùng đổi tên khóa được (đổi khóa cập nhật mọi tham chiếu). Bản sao **tách khỏi catalog**: không `template_ref`, nên nâng cấp Kit Việt không động tới nó: `upgrade.ts:251-252` chỉ nâng cấp workflow đang dùng có `template_ref` bắt đầu bằng `<flow.id>@`, workflow khác được giữ. Giao diện ghi rõ điều này khi nhân bản.
- **Nhập/xuất manifest.** Xuất: `FlowManifest` không có `bind`. Nhập: tạo bản nháp với mọi vai chưa gắn; kiểm như mục 3; từ chối khóa lạ.
- **Phiên bản định dạng.** `doc.version` và `format` có số phiên bản; máy chủ từ chối giá trị cao hơn mình biết (`flow_manifest_invalid`, `flow_schema_unsupported`) vì máy chủ cũ không được đoán nghĩa của bản mới.

### 8.3 Cờ và đường lui

- Cờ mới (chưa có trong `featureflags/keys.go`; `FF_FLOW_ENGINE` và `FF_FLOW_RUNNER` đã có ở `:43`, `:47`): `FF_FLOW_AUTHORING` (route soạn/xuất bản, mặc định tắt); **`FF_FLOW_GRAPH_V2`** (chỉ cho phép *ghi* tài liệu schema 2: nhập, xuất bản, tạo qua API; mặc định tắt). Cờ này không điều khiển bộ diễn giải: bộ diễn giải mới là **đường chạy duy nhất** cho cả schema 1 (qua `Lift`) và schema 2, nên không có "đường cũ" để rơi về (AGENTS.md cấm đường dự phòng). An toàn của bước chuyển nằm ở bài kiểm vi phân và bộ diễn tập (mục 9.3 và cổng G-L của kế hoạch), không ở cờ. `FF_FLOW_ENGINE` giữ vai kill switch khẩn cấp.
- Cờ của R3: **`FF_FLOW_PACKS`** (route `packs/preview`, `packs/commit`, `from.kind = proposal`, `import-proposal`; mặc định tắt). `POST /api/flow/validate`, chạy thử, `flow_workflow_id` và `flow-insights` đi sau `FF_FLOW_AUTHORING`. Tắt `FF_FLOW_PACKS` không ảnh hưởng squad, agent, skill đã nhập.
- Đường lui: tắt `FF_FLOW_GRAPH_V2` và `FF_FLOW_AUTHORING` dừng việc tạo flow mới; phiên bản schema 2 đã xuất bản và request đang chạy tiếp tục (bộ diễn giải đọc được); lui hẳn sang mã cũ chỉ bằng revert thay đổi mã (không có dữ liệu schema 1 bị sửa nên revert an toàn với flow cũ); revert mã khi đã có request schema 2 đang chạy cần `detach` chúng trước. Migration: chạy down theo thứ tự 993, 992, 991, 990, 989, 988 (xóa bản nháp và cột phiên bản; các hàng `issue_workflow` đã xuất bản vẫn là workflow flow dùng được, chỉ mất nhãn lineage).

### 8.4 Quan sát và vận hành

- Sự kiện thời gian thực mới `flow_draft:updated` `{squad_id, draft_id}` (`server/pkg/protocol/events.go`, cạnh `:148`; `packages/core/types/events.ts`); client chỉ vô hiệu hóa khóa truy vấn bản nháp (AGENTS.md "State Rules"). Sự kiện `issue_workflow:changed` có sẵn nhận thêm action `published`.
- Nhật ký hoạt động (`activity_log` qua `CreateActivity`, dùng ở `flow_engine_ops.go:197`, `:425`) cho xuất bản và đóng thay: tên người, phiên bản, số cảnh báo đã xác nhận.
- Cờ `FF_FLOW_AUTHORING` và `FF_FLOW_GRAPH_V2` (`server/internal/featureflags/keys.go`, cạnh `FlowEngineEnabled` ở `:98`), mặc định **tắt** khi giao, bật sau diễn tập.
- Tương thích: client desktop cài sẵn thấy phiên bản mới là workflow có `flow_config` (có từ tầng 1); trường mới `lineage_id`, `version`, `published_at`, `published_by` đi qua `.loose()` và có mặc định. Client đọc workflow phải coi "khóa" là `template_ref || published_at`; client cũ chỉ biết `template_ref` vẫn gọi `PATCH` và nhận 409 `workflow_locked` với thông báo đọc được.

## 9. Chuyển schema 1 sang bộ diễn giải mới

### 9.1 Nguyên tắc

Không sửa hàng nào, không sửa manifest nào (giữ `rev` của 21 flow, nên không phát sinh hàng workflow mới và không chạm request đang chạy). Hàng schema 1 được nạp qua hàm thuần `Lift`. Đây là **trình đọc một phiên bản định dạng**, cùng loại với `schemaVersion` của catalog; có tài liệu và test vĩnh viễn trong chừng nào còn hàng/manifest schema 1 (quyết định D11).

### 9.2 `Lift(def v1, opts) → Graph`

Đầu vào: `Definition` hiện có (`Steps`, `Flow`). Quy tắc:

1. **Vai:** mỗi UUID agent khác nhau trong `task.Handler.ID` → một vai `agent` khóa `a_<8 ký tự đầu của uuid>`, `bind.agent_ids = [uuid]`. (Với manifest catalog, `Lift` dùng khóa vai `task.role` sẵn có.)
2. **Biến:** mỗi `Switch` → `Var` cùng khóa/kiểu/giá trị/mặc định; `set_by [lead]`, `set_at [start]`; thành viên quản lý đặt mọi lúc (như hôm nay).
3. **Bước:** `todo` → bước `lead` (`lead.sets` = mọi biến; bộ verdict `plan`); `blocked` → biến mất (thành `stop`); `done`, `cancelled` → terminal (`done.leaves_workspace = true`, `cancelled = false`); bước `HumanOnly` → `kind: manual`, `exit_role: managers`, `notice` = `stop2` nếu khóa trạng thái là `in_review`, ngược lại `human`; bước khác → `kind: tasks`.
4. **Task:** `Kind`, `Instructions`, `Title`, `When` giữ; `Handler` → vai ở (1); `requires:[outcome]` → `requires.verdict = true` với bộ `gate`; `requires:[attachment]` → `files:[{slot:"output", min:1}]` (người tải = người thực hiện); `join.over = gates`, `mode = all`, `block = immediate`.
5. **Cạnh:** `Transitions` giữ thứ tự. Điều kiện viết lại: `tasks_outcome` → `join.result` với ánh xạ `all_approved→pass`, `any_rejected→fail`, `any_blocked→block`, `none→none`; `rework_count` → `loop.rework.count`; `outcome` (bước `todo`) → `lead.verdict`; `switch.k` → `var.k`.
6. **Cạnh tới `blocked`:** tại bước `todo` → `stop: grounds_missing`. Tại bước khác: với cạnh `e` có điều kiện `C`, thêm **trước** `e` một cạnh `e1` điều kiện `C ∧ join.result = block` (nếu `C` đã suy ra `block` thì `e1 = e`) mang `stop: grounds_missing`; `e` mang `stop: loop_exhausted`. Đây là cách tĩnh hóa quy tắc động của `route` (`flow.go:424-435`: mã 1 nếu dữ kiện lúc chạy có `any_blocked`, ngược lại mã 3).
7. **Cạnh `rework`:** thành cạnh quay lại gắn vòng `rework` (một vòng duy nhất vì bộ đếm của schema 1 là chung); `max` = giá trị N lớn nhất trong các điều kiện `rework_count lt N` (≤ 5); `header` = đích chung của các cạnh `rework` nếu giống nhau, rỗng nếu khác (khi đó vòng `reset: never`). Điều kiện cũ `rework_count lt N` được giữ nguyên trong `when` của từng cạnh, nên giới hạn thật của từng cạnh không đổi.
8. **Mã dừng:** `grounds_missing` (`legacy_no 1`), `loop_exhausted` (`3`); `stalled` ngầm. `messages` ← `flow_config.messages` (`stop1`, `stop2`, `stop3`, `stalled`, `human`).
9. **Cờ `compat: "v1"`** (chỉ với chạy thật, `opts.Runtime`): vòng của task = `RoundBase + Rework`; đầu vào task theo vị trí (`flowTaskLinks`); văn bản mô tả/ bình luận dùng mẫu cũ. Với `opts.Runtime = false` (nhân bản sang bản nháp): không `compat`, `activation`, đầu vào theo cạnh đã đi.
10. Đồ thị lifted **không** chịu bộ kiểm G-R mới (đã hợp lệ theo `ValidateFlow` lúc ghi); chỉ kiểm bất biến nội tại (tham chiếu đủ).
11. `Adapt(stateV1) → StateV2`: `rework → loops.rework`, `switches → vars`, `stop 1|3|"stalled" → code`, `step`, `round_base` giữ cho `compat`.
12. **Các luật mới không áp cho đồ thị `compat: "v1"`**, để flow đang chạy không đổi hành vi ngoài danh sách D5: `policy.allow_unattended_release = true` (E15 không đỗ request nào của schema 1); tách vai giữ luật "cùng vòng" hiện có (`flow_engine.go:608-620`), không dùng luật theo đầu vào của E13; không `trail`-dựa đầu vào (điều 9); không `Task.sets`, `Task.skills`, `inputs` (schema 1 không có chúng). Bản nhân bản sang bản nháp (`opts.Runtime = false`) **không** mang các miễn trừ này: nó là tài liệu schema 2 sạch và chịu toàn bộ mục 3, nên người nhân bản một flow Kit Việt có thể gặp `release_without_human` hay `separation_violation` nếu flow gốc vi phạm; đó là thông tin đúng, không phải hồi quy.
13. `trail` vẫn được ghi cho request schema 1 (chỉ thêm khóa vào `issue.flow`; `Unadapt` giữ nó) để màn hình "đường đã đi" và số liệu dùng chung một nguồn.

### 9.3 Chứng minh "quyết định runtime không đổi"

1. **Tiêu chuẩn đóng băng:** sao chép nguyên văn `Decide`, `route`, `tasksOutcomeOf`, `RoutePlan` hiện tại vào tệp test `server/internal/issueworkflow/flow_v1_oracle_test.go` (đoạn mã thuần, không đổi).
2. **Vi phân (`TestLift_DecideEquivalent_*`):** với **mỗi** flow của catalog (đọc từ `flows.json` sinh bởi `node scripts/catalog/cli.mjs build --target go`), mỗi bước, `rework ∈ [0, N+1]`, mọi tổ hợp giá trị công tắc, và tập trạng thái task của bước (mỗi task ∈ {mở, đóng×{không outcome, approved, rejected, blocked}, bị hủy}): so `DecideV1` với `Decide(Lift(def), Adapt(state), tasks)` trên (kind, task thiếu theo khóa, đích hoặc `stop`, cờ rework, mã dừng, cờ HumanStop, id cạnh). Không gian vượt 50.000 trạng thái mỗi bước thì lấy toàn bộ trường hợp biên cộng 20.000 mẫu ngẫu nhiên với hạt cố định (ghi hạt trong test).
3. **Trạng thái:** `TestAdapt_RoundTrip` mọi `issue.flow` mẫu (từ các test diễn tập) giải mã/chuyển/ghi lại không mất trường.
4. **Văn bản:** `TestLift_RenderedTextUnchanged`: mô tả task, lời nhắc task/lead và bình luận dừng của 21 flow, mọi vòng 0 và 1, trùng từng byte với bản đang chạy (chụp bằng test trước khi sửa mã).
5. **Diễn tập DB:** bộ `flow_rehearsal_trade_test.go`, `flow_rehearsal_squads_test.go` (21 hàm `TestRehearsal_<Squad>`), `flow_rehearsal_trade_auto_test.go`, `flow_chaos_test.go`, `flow_engine_test.go`, `flow_guards_*_test.go`, `flow_runner_test.go`, `flow_rollback_test.go` chạy lại **không sửa** (ngoại trừ các test mô tả hành vi mà D5 cố ý đổi: nêu từng cái trong PR) trên bộ diễn giải mới.
6. **Ranh giới:** `TestWorkflowGolden_*` (SDLC, Planning) xanh (E21).
7. **So sánh ở mọi lần `advance` của diễn tập DB (D24).** Bài vi phân ở điều 2 duyệt không gian trạng thái tổng hợp; nó không chứng minh các trạng thái **thật** mà DB sinh ra (task đóng nhưng còn lượt chạy, trạng thái `blocked` do hệ thống ghi, `round_base` sau `resume`) được chuyển đúng. Vì vậy `advanceOnce` có một điểm móc `flowDecideObserver func(def, stateV1, tasks, decision)` mặc định `nil`; gói test đặt nó thành hàm gọi oracle của điều 1 trên cùng đầu vào và làm test đỏ khi lệch. Điểm móc chỉ được gán trong tệp `_test.go`; mã sản phẩm không có đường chạy thứ hai, nên đây không phải chế độ bóng lúc chạy thật.

## 10. Quyền

| Hành động | Ai | Ghi chú |
| --- | --- | --- |
| Xem phiên bản, đồ thị, trạng thái request | thành viên workspace | |
| Soạn bản nháp, validate, simulate, nhập/xuất manifest, nhân bản Kit Việt | `canManageSquad` (không phải agent) | |
| Xuất bản | `canManageSquad` và từng agent gắn qua `memberCanWireAgent`; **tạo trạng thái mới chỉ admin** | D2. Đây là route mới. Route có sẵn `POST /api/issue-workflows` vẫn chỉ cho admin tạo workflow chạy bằng flow: điều kiện `(req.FlowConfig != nil \|\| templateRef != "" \|\| stepInputsCarryFlowFields(inputs)) && !isWorkspaceAdmin(member)` trả 403 `flow_workflow_forbidden` (`issue_workflow.go:812-816`). Route mới không đi qua điều kiện đó và không nới nó |
| Xác nhận cảnh báo `release_without_human` (`allow_unattended_release`), `lead_may_work` | **admin** | D3 |
| Xác nhận cảnh báo `no_gate` | `canManageSquad` | |
| Bật `flow_auto` | admin | D2, D9 |
| Đặt phiên bản làm đang dùng | `canManageSquad` | có sẵn |
| Đóng task thay (verdict, tệp), mở lại task đã đóng | `canManageSquad` | D4; nhật ký bắt buộc |
| Chọn cạnh thoát của bước `manual` | thành viên thuộc `exit_role` hoặc quản lý | |
| Đặt biến | theo `set_by`/`set_at`; quản lý bất kỳ lúc (nhật ký); người thực hiện task cho biến trong `Task.sets` | D17 |
| Kiểm một manifest (`POST /api/flow/validate`) | thành viên và agent | Không ghi gì; D15 |
| Nhập bộ skill/agent (preview, commit) | người có quyền tạo agent và skill trong workspace (cùng phép kiểm với route tạo agent và nhập skill hiện có), không phải agent | D14 |
| Chạy thử một phiên bản; chọn workflow khi tạo request | `canManageSquad` (chạy thử); thành viên (chọn trong các phiên bản flow của squad) | D22 |
| Xem số liệu của phiên bản | `canManageSquad` | D22 |
| Resume, detach | `canManageSquad` | `skip_checkpoint` cần xác nhận rõ |
| Xóa phiên bản | chủ sở hữu/admin, không đang dùng, không request mở | có sẵn |
| Bật/tắt `FF_FLOW_ENGINE` | vận hành | ghi nhật ký quản trị |

## 11. Giao diện

Tuân theo AGENTS.md: logic dùng chung ở `packages/core` và `packages/views`; không `next/*`, `react-router-dom` trong `views`; TanStack Query giữ dữ liệu máy chủ, Zustand chỉ trạng thái UI (không cần); icon từ `@multica/ui/icons`; token typography từ `packages/ui/styles/tokens.css`; mô tả phụ bị bỏ mặc định ("UI descriptions" của `conventions.mdx`).

### 11.1 Cấu trúc

Trang soạn `FlowDraftEditorPage` trong `packages/views/squads/flow-designer/` (thư mục mới, không xung đột với việc đang chạy). Route web `apps/web/app/[workspaceSlug]/(dashboard)/squads/[id]/workflows/[draftId]/page.tsx`, desktop `squads/:id/workflows/:draftId` cạnh `apps/desktop/src/renderer/src/routes.tsx:287`. Mọi bộ chọn (loại bước, loại vai, kiểu biến, mode hợp, class verdict, toán tử) và mọi giới hạn lấy từ `capabilities`; giá trị lạ hiển thị chip "unsupported" nguyên văn. Trang chi tiết squad, mục Workflows, liệt kê **bản nháp** của squad (chỉ người `canManageSquad` thấy, mỗi dòng dẫn tới trang soạn); không có danh sách này thì một bản nháp không tới lại được sau khi đóng trang soạn.

### 11.2 Màn hình và điều khiển

| Khu | Nội dung |
| --- | --- |
| Thanh trên | Tên bản nháp (sửa tại chỗ), trạng thái lưu, Check, Simulate, Export, Publish (Publish khóa khi còn lỗi) |
| **Outline** (trái) | Danh sách bước theo thứ tự topo của đồ thị (không theo vị trí bảng), mỗi hàng: tên, loại (Tasks/Lead/Manual), số task, chip vòng `↺ rounds 0/3`, chip lỗi. Tìm bước theo tên; gập/mở nhóm theo vòng; kéo thả chỉ đổi `layout`, không đổi luồng |
| **Inspector** (giữa) | Bước: tên, trạng thái hiển thị (chung được, cảnh báo), loại, `join` (All / Any / Quorum k), `notice`, danh sách task, danh sách cạnh có thứ tự (kéo để đổi thứ tự). Task: tiêu đề, vai, loại Work/Gate, `instances`, `after`, "Only when", bộ verdict, tệp yêu cầu, hạn, hướng dẫn. Cạnh: nhãn, điều kiện (bộ xây điều kiện), đích hoặc mã dừng, vòng |
| **Mẫu bước** | Khi thêm bước: Work, Gate, Quorum review, Human approval, Lead decision. Mỗi mẫu sinh cạnh chuẩn (ví dụ Gate: block → stop, pass → next, fail ∧ vòng chưa cạn → quay lại, mặc định → stop) và người dùng sửa được từng cạnh; cạnh không còn khớp mẫu hiện "Custom routing". **Cài đặt (menu "Add step" ở Outline):** `addStepFromTemplate` (`step-templates.ts`) thêm bước ở cuối tài liệu, mọi cạnh ra trỏ tới terminal đầu tiên để người dùng nối lại; chưa có cạnh nào dẫn vào bước, nên kiểm tra báo `step_unreachable` cho tới khi người dùng nối. Gate: block → `stalled`, pass → terminal, fail ∧ `loop.<k>.count < max` → quay lại header của vòng (chỉ khi tài liệu đã có vòng với `max` là số; không thì bỏ cạnh này), mặc định → `stalled`; vai chấm là vai agent chưa làm task `work` nào, không có thì thêm vai `reviewer`. Quorum review: ba người chấm, qua khi hai người duyệt, vai `reviewers` có `pool` 3. Human approval: bước `manual`, `exit_role: managers`, một cạnh `manual`, kèm `notice`. Chưa cài: nhãn "Custom routing" |
| **Vai** | Bảng: khóa, nhãn, loại (Agent / Person / Squad managers / Requester), hồ bơi, người/agent gắn (bộ chọn giới hạn thành viên squad; "Add agent to squad"). Lead cố định, không gắn |
| **Biến** | Khóa, kiểu, mặc định, "Set by", "Set at"; giá trị `list` có giới hạn mục |
| **Bộ verdict** | Bảng verdict + class (Pass / Fail / Neutral); dựng sẵn hiện chỉ đọc |
| **Vòng** | Bảng vòng: header, cạnh quay lại, `max` hoặc "from variable (cap n)"; phân tích chi phí xấu nhất "Up to N task issues" |
| **Dừng và đích** | Mã dừng khai báo + soạn thông điệp (biến, xem trước đã điền); bảng terminal với "Leaves the workspace"; cảnh báo `release_without_human` và hộp xác nhận (chỉ admin tick) |
| **Problems** | `issues[]` nhóm theo bước; bấm chọn đúng `path`; cảnh báo cần `ack` |
| **Simulate** | Chọn biến, kết quả task theo lần vào bước (bảng nhỏ), Run, kết quả danh sách bước kèm bộ đếm vòng; dòng cố định "Simulation checks the routing. It does not check what agents do." |

### 11.3 Đồ thị đọc được khi có nhiều bước

- Bố cục phân tầng (tầng = chiều dài đường dài nhất từ `start` sau khi bỏ cạnh quay lại): hàm thuần `layoutFlowGraph` ở `packages/core/squads/flow-layout.ts` (không thêm phụ thuộc; chỉ dùng `@xyflow/react` đã có ở `packages/views/package.json`). Cạnh quay lại vẽ cong, nét đứt, nhãn `↺ n/max`; cạnh `stop` tới các chấm đỗ gom theo mã, không vẽ về từng nút.
- Nút bước thu gọn mặc định (tên, loại, số task, chip join); mở ra khi chọn. Có minimap, zoom, "Fit", "Find a step", "Focus path" (làm mờ mọi thứ không nằm trên đường từ bước chọn tới terminal), "Show loops only".
- Đồ thị chỉ để xem và chọn; **Outline là bề mặt chính** (bàn phím, đọc màn hình, điện thoại). 40 bước không vẽ chồng nhờ bố cục tầng và thu gọn.
- Trạng thái rỗng, lỗi, xung đột: bảng ở 11.8.

### 11.4 Điện thoại (dưới 640px)

Ngưỡng "điện thoại" của trang soạn là `useIsMobile` của ứng dụng (dưới 768px), không phải 640px: một ngưỡng chung cho cả ứng dụng, để Sheet của Inspector và các màn khác đổi cùng lúc. Danh sách đồ thị dọc vẫn đổi ở 640px (`sm:`).

Một cột, không cuộn ngang. Thanh trên gọn (tên + "⋯"). Outline dạng thẻ gập; chọn bước/task/cạnh mở Inspector thành Sheet cao đầy. Đồ thị mở bằng "View graph" thành danh sách dọc phân tầng (không canvas). Các khu Vai/Biến/Verdict/Vòng/Dừng/Problems là mục gập. Thanh hành động dính đáy: Check, Publish. Vùng chạm ≥ 44px. Bộ xây điều kiện dùng danh sách dọc (mỗi điều kiện một thẻ).

### 11.5 Bộ xây điều kiện

Dùng lại cây AND/OR/NOT của `packages/views/workflow/designer/condition-builder.tsx` và các hàm thuần của `packages/views/workflow/condition-spec.ts` (`ruleWithField`, `ruleWithOperator`, …), nhưng **tham số hóa theo sổ dữ kiện**: thay hằng `CONDITION_FIELDS`, `FIELD_OPERATORS`, `FIELD_ENUM_VALUES` (`condition-spec.ts:20-50`) bằng một đối số `FactSpec[]` nhận từ `fact_catalog` của máy chủ; mặc định bằng bộ hiện tại để trình thiết kế cũ không đổi. Mở rộng cho kiểu `int`, `string`, `list_string`. Máy chủ vẫn là bên kiểm cuối.

### 11.6 Màn hình request (chạy)

`IssueFlowStatePanel` (`packages/views/workflow/issue-flow-state-panel.tsx`): hiển thị vòng `↺ n/max`, bước đang ở, mỗi thực thể một hàng, class và verdict, huy hiệu "đóng thay", nút chọn verdict theo bộ verdict của task (thay hai nút Approve/Reject hiện tại, chỉ hiện khi `viewer.closable_task_ids` chứa task; bản hiện tại hiện cho mọi người, xem E1), nút cạnh `manual` cho `exit_role`, nút "Close on behalf" cho quản lý.

### 11.7 Nội dung 7 ngôn ngữ

Khóa nằm ở `packages/views/locales/<locale>/squads.json` dưới `flow_designer`. Hai bảng: 11.7.1 (khóa nền của trình soạn) và 11.7.2 (khóa của mô hình tổng quát). Nhãn dừng theo số (`stop.stop1` đến `stop.stop3`) chỉ dùng cho mã dừng có `legacy_no`; mã dừng khai báo mới dùng nhãn do tác giả flow đặt (một ngôn ngữ, không dịch).

#### 11.7.1 Khóa nền

| Khóa | en | zh-Hans | zh-Hant | fr | es | ko | ja |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `title` | Edit workflow | 编辑工作流 | 編輯工作流程 | Modifier le workflow | Editar flujo de trabajo | 워크플로 편집 | ワークフローを編集 |
| `new` | New workflow | 新建工作流 | 建立工作流程 | Nouveau workflow | Nuevo flujo de trabajo | 새 워크플로 | 新規ワークフロー |
| `start.blank` | Blank | 空白 | 空白 | Vide | En blanco | 빈 워크플로 | 空白 |
| `start.kitviet` | Copy a Kit Việt workflow | 复制 Kit Việt 工作流 | 複製 Kit Việt 工作流程 | Copier un workflow Kit Việt | Copiar un flujo de Kit Việt | Kit Việt 워크플로 복사 | Kit Việt のワークフローをコピー |
| `start.import` | Import a file | 导入文件 | 匯入檔案 | Importer un fichier | Importar un archivo | 파일 가져오기 | ファイルをインポート |
| `export` | Export | 导出 | 匯出 | Exporter | Exportar | 내보내기 | エクスポート |
| `save.saved` | Saved | 已保存 | 已儲存 | Enregistré | Guardado | 저장됨 | 保存済み |
| `save.unsaved` | Unsaved changes | 有未保存的更改 | 有未儲存的變更 | Modifications non enregistrées | Cambios sin guardar | 저장되지 않은 변경 사항 | 未保存の変更があります |
| `conflict.title` | Someone saved this draft first | 其他人已先保存了此草稿 | 其他人已先儲存此草稿 | Quelqu'un a déjà enregistré ce brouillon | Alguien guardó este borrador antes | 다른 사람이 이 초안을 먼저 저장했습니다 | 他のユーザーが先にこの下書きを保存しました |
| `conflict.reload` | Reload | 重新加载 | 重新載入 | Recharger | Recargar | 새로 고침 | 再読み込み |
| `conflict.keep_copy` | Keep mine as a copy | 将我的修改保存为副本 | 將我的修改儲存為副本 | Garder ma version en copie | Guardar la mía como copia | 내 변경 사항을 사본으로 보관 | 自分の変更をコピーとして保存 |
| `roles.title` | Roles | 角色 | 角色 | Rôles | Roles | 역할 | ロール |
| `roles.leader` | The leader plans and takes no tasks | 队长只负责规划，不接任务 | 隊長只負責規劃，不接任務 | Le chef planifie et ne prend aucune tâche | El líder planifica y no toma tareas | 리더는 계획만 하고 작업은 맡지 않습니다 | リーダーは計画のみで、タスクは担当しません |
| `roles.pick_agent` | Choose an agent | 选择智能体 | 選擇 AI 代理 | Choisir un agent | Elegir un agente | 에이전트 선택 | エージェントを選択 |
| `roles.unused` | Not used by any task | 没有任务使用 | 沒有任務使用 | Aucune tâche ne l'utilise | Ninguna tarea lo usa | 어떤 작업에서도 쓰이지 않음 | どのタスクでも使われていません |
| `step.work` | Work step | 工作步骤 | 工作步驟 | Étape de travail | Paso de trabajo | 작업 단계 | 作業ステップ |
| `step.gate` | Gate step | 门禁步骤 | 關卡步驟 | Étape porte | Paso de puerta | 게이트 단계 | ゲートステップ |
| `step.human` | Human only | 仅限人工 | 僅限人工 | Humain uniquement | Solo humano | 사람 전용 | 人間のみ |
| `step.add` | Add step | 添加步骤 | 新增步驟 | Ajouter une étape | Añadir paso | 단계 추가 | ステップを追加 |
| `task.add` | Add task | 添加任务 | 新增任務 | Ajouter une tâche | Añadir tarea | 작업 추가 | タスクを追加 |
| `task.attachment` | Needs an attachment | 需要附件 | 需要附件 | Pièce jointe requise | Requiere archivo adjunto | 첨부 파일 필요 | 添付ファイルが必要 |
| `task.verdict` | Needs a verdict | 需要裁决 | 需要裁決 | Verdict requis | Requiere veredicto | 판정 필요 | 判定が必要 |
| `task.only_when` | Only when | 仅当 | 僅當 | Seulement si | Solo cuando | 다음일 때만 | 次の場合のみ |
| `switches.title` | Switches | 开关 | 開關 | Commutateurs | Interruptores | 스위치 | スイッチ |
| `rework.label` | If rejected, go back up to {{count}} times | 被驳回时最多返回 {{count}} 次 | 被駁回時最多返回 {{count}} 次 | En cas de rejet, retour max. {{count}} fois | Si se rechaza, volver como máximo {{count}} veces | 반려되면 최대 {{count}}회 되돌아감 | 却下された場合は最大 {{count}} 回差し戻し |
| `stop.stop1` | Stop 1 · missing grounds | 停止 1 · 缺少依据 | 停止 1 · 缺少依據 | Arrêt 1 · éléments manquants | Parada 1 · faltan datos | 중단 1 · 근거 부족 | 停止 1 · 根拠不足 |
| `stop.stop2` | Stop 2 · human review before leaving the workspace | 停止 2 · 离开工作区前需人工审核 | 停止 2 · 離開工作區前需人工審核 | Arrêt 2 · relecture humaine avant de quitter l'espace de travail | Parada 2 · revisión humana antes de salir del espacio de trabajo | 중단 2 · 워크스페이스를 벗어나기 전 사람 검토 | 停止 2 · ワークスペースを出る前の人間によるレビュー |
| `stop.stop3` | Stop 3 · rework limit reached | 停止 3 · 已达返工上限 | 停止 3 · 已達返工上限 | Arrêt 3 · limite de reprises atteinte | Parada 3 · límite de repeticiones alcanzado | 중단 3 · 재작업 한도 도달 | 停止 3 · 差し戻しの上限に到達 |
| `stop.stalled` | Stalled | 已停滞 | 已停滯 | Sans progrès | Estancado | 진행 정체 | 停滞 |
| `stop.waiting` | Waiting for approval | 等待批准 | 等待核准 | En attente d'approbation | Esperando aprobación | 승인 대기 중 | 承認待ち |
| `actions.check` | Check | 检查 | 檢查 | Vérifier | Comprobar | 검사 | チェック |
| `actions.simulate` | Simulate | 模拟 | 模擬 | Simuler | Simular | 시뮬레이션 | シミュレーション |
| `actions.publish` | Publish | 发布 | 發布 | Publier | Publicar | 게시 | 公開 |
| `publish.note` | Publishing creates a locked version. Running requests keep theirs. | 发布会生成一个锁定的版本。运行中的请求保留原版本。 | 發布會產生一個鎖定的版本。執行中的請求保留原版本。 | La publication crée une version verrouillée. Les demandes en cours gardent la leur. | Publicar crea una versión bloqueada. Las solicitudes en curso conservan la suya. | 게시하면 잠긴 버전이 만들어집니다. 진행 중인 요청은 기존 버전을 유지합니다. | 公開するとロックされたバージョンが作成されます。実行中のリクエストは現在のバージョンのままです。 |
| `publish.make_active` | Make this the active workflow | 设为当前使用的工作流 | 設為目前使用的工作流程 | Définir comme workflow actif | Usar como flujo de trabajo activo | 활성 워크플로로 지정 | アクティブなワークフローにする |
| `simulate.note` | Simulation checks the routing. It does not check what agents do. | 模拟只检查流转，不检查智能体的实际工作。 | 模擬只檢查流轉，不檢查 AI 代理的實際工作。 | La simulation vérifie le routage, pas le travail des agents. | La simulación comprueba el enrutamiento, no el trabajo de los agentes. | 시뮬레이션은 경로만 확인하며 에이전트의 작업은 확인하지 않습니다. | シミュレーションはルーティングのみを確認し、エージェントの作業は確認しません。 |
| `ack.no_human_exit` | I understand this workflow can finish without a person's review | 我了解此工作流无需人工审核即可完成 | 我了解此工作流程無需人工審核即可完成 | Je comprends que ce workflow peut se terminer sans relecture humaine | Entiendo que este flujo puede terminar sin revisión humana | 이 워크플로가 사람 검토 없이 완료될 수 있음을 이해합니다 | このワークフローは人間のレビューなしで完了し得ることを理解しました |
| `ack.no_gate` | I understand no task is checked by a gate | 我了解没有任务经过门禁检查 | 我了解沒有任務經過關卡檢查 | Je comprends qu'aucune tâche n'est contrôlée par une porte | Entiendo que ninguna tarea pasa por una puerta | 어떤 작업도 게이트로 검증되지 않음을 이해합니다 | どのタスクもゲートで検査されないことを理解しました |
| `problems.none` | No problems found | 未发现问题 | 未發現問題 | Aucun problème | Sin problemas | 문제가 없습니다 | 問題は見つかりませんでした |
| `empty.title` | No workflow yet | 还没有工作流 | 尚無工作流程 | Aucun workflow | Aún no hay flujos de trabajo | 아직 워크플로가 없습니다 | ワークフローはまだありません |
| `load_failed` | Could not load the workflow | 无法加载工作流 | 無法載入工作流程 | Impossible de charger le workflow | No se pudo cargar el flujo de trabajo | 워크플로를 불러오지 못했습니다 | ワークフローを読み込めませんでした |
| `retry` | Retry | 重试 | 重試 | Réessayer | Reintentar | 다시 시도 | 再試行 |
| `version` | Version {{n}} | 版本 {{n}} | 版本 {{n}} | Version {{n}} | Versión {{n}} | 버전 {{n}} | バージョン {{n}} |
| `locked` | Published versions are locked | 已发布的版本已锁定 | 已發布的版本已鎖定 | Les versions publiées sont verrouillées | Las versiones publicadas están bloqueadas | 게시된 버전은 잠겨 있습니다 | 公開済みのバージョンはロックされています |
| `edit_as_draft` | Edit as draft | 作为草稿编辑 | 以草稿編輯 | Modifier comme brouillon | Editar como borrador | 초안으로 편집 | 下書きとして編集 |

#### 11.7.2 Khóa của mô hình tổng quát

Đây là **khóa mới** của bản tổng quát (cùng quy tắc thuật ngữ: gate: 门禁/關卡/Porte/Puerta/게이트/ゲート; verdict: 裁决/裁決/Verdict/Veredicto/판정/判定; agent: 智能体/AI 代理/Agent/Agente/에이전트/エージェント; Human only: 仅限人工/僅限人工/Humain uniquement/Solo humano/사람 전용/人間のみ — các từ này lấy từ `squads.json`, `agents.json`, `workflow.json` hiện có). Bản dịch là đề xuất, chưa được người bản ngữ đọc; đã đối chiếu với `apps/docs/content/docs/developers/conventions.mdx` mục 2 ở các điểm sau: Agent là 智能体 (zh-Hans) và AI 代理 (zh-Hant), Member là 成员/成員/Membre/Miembro/멤버/メンバー, Workspace là 工作区. Chưa đối chiếu: glossary đặt `issue` là 任务/タスク/태스크/Tâche/Tarea và "task" của API là "Run", trong khi `squads.json` hiện dùng "作业/작업" cho loại task `work`; cần chốt cách gọi task của flow (sub-issue) trước khi nhận bản dịch (mục 14.2).

| Khóa | en | zh-Hans | zh-Hant | fr | es | ko | ja |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `step.kind.tasks` | Tasks | 任务 | 任務 | Tâches | Tareas | 작업 | タスク |
| `step.kind.lead` | Lead step | 队长步骤 | 隊長步驟 | Étape du chef | Paso del líder | 리더 단계 | リーダーのステップ |
| `step.kind.manual` | Manual exit | 手动放行 | 手動放行 | Sortie manuelle | Salida manual | 수동 종료 | 手動で進める |
| `join.title` | Wait for | 等待条件 | 等待條件 | Attendre | Esperar | 대기 조건 | 待機条件 |
| `join.all` | All tasks | 全部任务 | 全部任務 | Toutes les tâches | Todas las tareas | 모든 작업 | すべてのタスク |
| `join.any` | Any task | 任一任务 | 任一任務 | Une tâche | Cualquier tarea | 작업 하나 | いずれかのタスク |
| `join.quorum` | At least {{count}} approve | 至少 {{count}} 个通过 | 至少 {{count}} 個通過 | Au moins {{count}} approbations | Al menos {{count}} aprueban | 최소 {{count}}명 승인 | {{count}} 件以上が承認 |
| `loop.title` | Loops | 循环 | 循環 | Boucles | Bucles | 반복 | ループ |
| `loop.max` | Up to {{count}} times | 最多 {{count}} 次 | 最多 {{count}} 次 | {{count}} fois maximum | Hasta {{count}} veces | 최대 {{count}}회 | 最大 {{count}} 回 |
| `loop.back_to` | Go back to {{step}} | 返回到 {{step}} | 返回到 {{step}} | Retour à {{step}} | Volver a {{step}} | {{step}}(으)로 돌아가기 | {{step}} に戻る |
| `verdicts.title` | Verdicts | 裁决选项 | 裁決選項 | Verdicts | Veredictos | 판정 옵션 | 判定の選択肢 |
| `verdict.pass` | Counts as pass | 视为通过 | 視為通過 | Compte comme réussite | Cuenta como aprobado | 통과로 간주 | 合格として扱う |
| `verdict.fail` | Counts as fail | 视为未通过 | 視為未通過 | Compte comme échec | Cuenta como rechazo | 실패로 간주 | 不合格として扱う |
| `verdict.neutral` | Neutral | 中立 | 中立 | Neutre | Neutral | 중립 | 中立 |
| `instances.count` | Run {{count}} in parallel | 并行运行 {{count}} 个 | 並行執行 {{count}} 個 | Lancer {{count}} en parallèle | Ejecutar {{count}} en paralelo | {{count}}개 병렬 실행 | {{count}} 件を並列実行 |
| `instances.per_item` | One per item of {{var}} | 按 {{var}} 的每一项各一个 | 按 {{var}} 的每一項各一個 | Une par élément de {{var}} | Una por elemento de {{var}} | {{var}}의 항목마다 하나씩 | {{var}} の項目ごとに 1 件 |
| `task.after` | Starts after | 在其后开始 | 在其後開始 | Démarre après | Empieza después de | 다음 이후 시작 | 次の後に開始 |
| `files.title` | Required files | 必需文件 | 必要檔案 | Fichiers requis | Archivos requeridos | 필수 파일 | 必須ファイル |
| `files.min` | At least {{count}} | 至少 {{count}} 个 | 至少 {{count}} 個 | Au moins {{count}} | Al menos {{count}} | 최소 {{count}}개 | {{count}} 件以上 |
| `roles.kind.agent` | Agent | 智能体 | AI 代理 | Agent | Agente | 에이전트 | エージェント |
| `roles.kind.member` | Member | 成员 | 成員 | Membre | Miembro | 멤버 | メンバー |
| `roles.kind.managers` | Squad managers | 小队管理者 | 小隊管理者 | Gestionnaires du squad | Gestores del squad | 스쿼드 관리자 | スクワッド管理者 |
| `vars.title` | Variables | 变量 | 變數 | Variables | Variables | 변수 | 変数 |
| `stops.declared` | Stop reasons | 停止原因 | 停止原因 | Motifs d'arrêt | Motivos de parada | 중단 사유 | 停止の理由 |
| `terminal.leaves` | Leaves the workspace | 离开工作区 | 離開工作區 | Quitte l'espace de travail | Sale del espacio de trabajo | 워크스페이스를 벗어남 | ワークスペースを出る |
| `release.warning` | This workflow can leave the workspace without a person's review | 此工作流可在无人审核的情况下离开工作区 | 此工作流程可在無人審核的情況下離開工作區 | Ce workflow peut quitter l'espace de travail sans relecture humaine | Este flujo puede salir del espacio de trabajo sin revisión humana | 이 워크플로는 사람 검토 없이 워크스페이스를 벗어날 수 있습니다 | このワークフローは人間のレビューなしでワークスペースを出る可能性があります |
| `close_on_behalf` | Close on behalf | 代为关闭 | 代為關閉 | Clôturer à la place | Cerrar en su nombre | 대신 닫기 | 代理でクローズ |
| `on_behalf_badge` | Closed by {{name}} on behalf of the agent | {{name}} 代智能体关闭 | {{name}} 代 AI 代理關閉 | Clôturé par {{name}} à la place de l'agent | Cerrado por {{name}} en nombre del agente | {{name}}님이 에이전트 대신 닫음 | {{name}} がエージェントの代わりにクローズ |
| `graph.collapse` | Collapse | 折叠 | 摺疊 | Réduire | Contraer | 접기 | 折りたたむ |
| `graph.find` | Find a step | 查找步骤 | 尋找步驟 | Trouver une étape | Buscar un paso | 단계 찾기 | ステップを検索 |
| `graph.worst_case` | Up to {{count}} task issues in the worst case | 最坏情况下最多 {{count}} 个任务 | 最壞情況下最多 {{count}} 個任務 | Jusqu'à {{count}} tâches dans le pire cas | Hasta {{count}} tareas en el peor caso | 최악의 경우 최대 {{count}}개 작업 | 最悪の場合は最大 {{count}} 件のタスク |
| `unsupported` | Not supported by this app version | 此应用版本不支持 | 此應用程式版本不支援 | Non pris en charge par cette version | No compatible con esta versión | 이 앱 버전에서 지원되지 않음 | このアプリのバージョンでは未対応 |

Mỗi mã vấn đề ở mục 3 và mỗi mã lỗi ở 7.4 cần khóa `problems.<code>` / `errors.<code>` đủ 7 locale; `packages/views/locales/parity.test.ts` chặn thiếu. Thông điệp dừng/`notice` do tác giả flow viết bằng một ngôn ngữ và không dịch (như nội dung Kit Việt).

### 11.8 Trạng thái rỗng, lỗi, xung đột

| Tình huống | Hiển thị |
| --- | --- |
| Squad chưa có workflow | Dòng "No workflow yet" và hai nút: Blank, Import a file. **Lệch so với bản đầu của mục này:** "Copy a Kit Việt workflow" chỉ hiện khi squad đã có một hàng workflow tạo từ catalog (`template_ref`), vì nhân bản (`from.kind = workflow`) cần một hàng có sẵn và tạo hàng catalog là việc của admin (`flow_workflow_forbidden`). Hộp thoại New workflow hiện nút này khi có hàng như vậy |
| Squad chưa có agent thành viên nào ngoài lead | Trang soạn mở được, khối Roles hiện "Add agent to squad" và Publish khóa kèm lý do |
| Tải bản nháp lỗi | Dòng lỗi "Could not load the workflow" và nút **Retry**; 404 về chi tiết squad |
| `flow_draft_conflict` khi lưu | Hộp thoại "Someone saved this draft first": **Reload** (bỏ thay đổi cục bộ) hoặc **Keep mine as a copy** (tạo bản nháp mới từ bộ đệm cục bộ) |
| `flow_draft_stale` khi xuất bản | Nêu phiên bản mới hơn; nút **Rebase**: tạo bản nháp mới từ phiên bản mới nhất, giữ bản nháp cũ |
| Hàng đã xuất bản mở trong trình thiết kế chung | Trình cũ giữ chế độ chỉ đọc (`workflow-designer-page.tsx:132`) và thêm dòng "Edit as draft" dẫn tới trang soạn mới |
| `flow_authoring_disabled` | Ẩn nút **New workflow** và **Edit as draft**; danh sách phiên bản vẫn xem được |
| Mất mạng khi tự lưu | Trạng thái "Unsaved changes" kèm nút **Retry**; không mất bộ đệm cục bộ |

### 11.9 Bề mặt mới của R3 và của các quyết định D16 đến D22

| Khu | Nội dung |
| --- | --- |
| **Nhập bộ** (trang Squads, nút "Import a pack") | Ba bước: (1) tải zip hoặc dán URL; (2) xem `PackPreview`: bảng agent (tạo mới / dùng agent đang có / bỏ qua, chọn runtime, skill gắn), bảng skill (xung đột tên), chọn lead (một agent của bộ hoặc lead chung), mục "Ignored settings", phát hiện quét; (3) kết quả và lối đi tiếp: "Design the workflow". Bộ chỉ có skill hiện bảng agent rỗng với "Add agent" và "One agent per skill" |
| **Hộp thoại tạo workflow** | Thêm lối thứ tư "Ask an agent to draft" (16.4): chọn agent soạn, tick tài liệu tham khảo, ô ghi chú |
| **Ô tham khảo** (trình soạn, cột phải, gập được) | Đọc hướng dẫn của agent và tệp skill của squad (API skill file và agent có sẵn) cạnh Inspector, để người soạn đối chiếu quy trình gốc khi dựng bước. Chỉ đọc |
| **Bản nháp `origin: proposal`** | Dải trạng thái: "Waiting for the agent's proposal" kèm liên kết issue; khi có tệp: nút "Import proposal"; sau khi nhập: nhãn "Proposed by {{agent}}" cho tới khi xuất bản |
| **Inspector của task** | Thêm: "Inputs" (chọn task nguồn theo bước, khe tệp, bắt buộc), "Skills" (chọn trong skill mà vai đang có; skill thiếu hiện lỗi R09 tại chỗ), "Sets variables" và "Must set before closing" |
| **Trang phiên bản** | Nút "Trial run"; thẻ số liệu (16.6) trên từng bước và cạnh của đồ thị; bật/tắt "Include trial runs" |
| **Form tạo issue giao cho squad** | Bộ chọn workflow khi squad có hơn một lineage flow đã xuất bản |
| **Màn hình request** | Mục "Path": danh sách `trail` (bước, cạnh, thời điểm); mở một dòng hiện dữ kiện của lần chọn cạnh đó (`flow_transition`); huy hiệu "Trial"; với task: "Inputs", "Feedback", biến đã đặt |

Khóa dịch mới nằm cùng khối `flow_designer` và một khối `packs` mới trong `squads.json`; bản `en` dưới đây, sáu ngôn ngữ còn lại viết ở bước kế hoạch tương ứng (P20b, P27) và `parity.test.ts` chặn thiếu.

| Khóa | en |
| --- | --- |
| `packs.import` | Import a pack |
| `packs.agents` | Agents |
| `packs.skills` | Skills |
| `packs.leader` | Leader |
| `packs.leader_generic` | Use a general leader |
| `packs.one_agent_per_skill` | One agent per skill |
| `packs.ignored` | Ignored settings |
| `packs.design_workflow` | Design the workflow |
| `start.proposal` | Ask an agent to draft |
| `proposal.waiting` | Waiting for the agent's proposal |
| `proposal.import` | Import proposal |
| `proposal.by` | Proposed by {{agent}} |
| `ack.proposal_reviewed` | I have read this workflow |
| `task.inputs` | Inputs |
| `task.skills` | Skills |
| `task.sets` | Sets variables |
| `task.requires_vars` | Must set before closing |
| `trial.run` | Trial run |
| `trial.badge` | Trial |
| `insights.include_trial` | Include trial runs |
| `request.path` | Path |
| `request.feedback` | Feedback |
| `reference.title` | Reference |

## 12. Tiêu chí chấp nhận

### 12.1 Engine tổng quát (K)

| # | Tiêu chí |
| --- | --- |
| K1 | `TestLift_DecideEquivalent_*` xanh cho cả 21 flow (9.3) |
| K2 | `TestLift_RenderedTextUnchanged` và `TestAdapt_RoundTrip` xanh |
| K3 | Toàn bộ test diễn tập, hỗn loạn, runner, guard của tầng 1–2 chạy không sửa trên bộ diễn giải mới (ngoại lệ có danh sách do D5) |
| K4 | `TestWorkflowGolden_*` (SDLC, Planning) xanh; `reworkReturnLimit` và `applyIssueWorkflow` không đổi |
| K5 | E1–E7: mỗi hành vi bị chặn với mã nêu, trên flow catalog và flow người dùng (test đỏ-trước) |
| K6 | Bốn kịch bản mẫu (Phụ lục C.2 đến C.4, cộng Trade) chạy hết bằng agent giả: Kit Việt-mẫu mới (`schema 2`), nghiên cứu (vòng lồng, `max` từ biến), thị trường (`instances`, `after`), pháp lý (`quorum` 3/5, quay lại bước cụ thể, thoát sớm) |
| K7 | Bộ kiểm: mỗi mã ở mục 3 có ít nhất một test dương và một test âm; `validate` trả đủ mọi vấn đề một lượt |
| K8 | `analysis.worst_case_task_issues` khớp số issue thật tạo ra bởi kịch bản xấu nhất của test |
| K9 | Mô phỏng: kết cục khớp chạy thật cho cùng kịch bản (so hai chiều); không ghi DB |
| K10 | Trần: flow có thể vượt `max_task_issues` bị từ chối lúc ghi; ép vượt lúc chạy dừng `stalled` `flow_cap_reached` |
| K11 | Client cũ: `GET /api/issue-workflows/{id}` cho hàng schema 2 qua zod cũ không lỗi; `workflow-graph-sheet` cũ hiển thị chiếu |
| K12 | Mọi phản hồi mới có test phản hồi sai (zod): khóa thiếu, enum lạ, `null` cho mảng, kiểu sai |
| K13 | Parity 7 ngôn ngữ xanh; ảnh chụp 390px và 1280px không cuộn ngang; đồ thị 40 bước dựng được và Outline dùng được bằng bàn phím |
| K14 | `TestEveryConcurrentUpBuildHasCleanup`, `TestMigrationFilesHaveMatchingDirections`, `TestMigrationNumericPrefixesAreUnique` xanh |
| K15 | `git ls-files packages/core/catalog/generated server/internal/catalog/generated` chỉ trả `.gitkeep` |
| K16 | Các tiêu chí C1 đến C20 ở 12.2 (xuất bản, quyền, bản nháp) xanh |
| K17 | Đầu vào (4.11): gate ở nhánh A nhận sản phẩm của bước làm việc trên **đường đã đi**, không phải bước đứng trước theo thứ tự khai báo; sau cạnh quay lại, task nhận `feedback` là các gate đã từ chối của lần vào trước; đầu vào `required` thiếu thì `stalled` `input_missing` |
| K18 | Biến do task đặt (4.12): task đặt được biến trong `sets`, không đặt được biến khác, không đặt được sau khi đóng; `requires.vars` chặn đóng; danh sách do task đặt sinh đúng số thực thể ở bước sau và bị kẹp ở `max`; biến bị đóng băng khi fan-out đang chạy |
| K19 | Tách vai theo đầu vào: bộ hai agent duyệt chéo (A viết, B duyệt; B viết, A duyệt) xuất bản được và chạy hết; một agent vừa viết vừa duyệt sản phẩm của chính nó bị từ chối lúc xuất bản và lúc mở task |
| K20 | Điểm kiểm: request có người duyệt sớm rồi agent viết lại sau đó **không** ra khỏi workspace (`release_without_checkpoint`); cùng flow có bước người cuối thì ra được |
| K21 | Hợp: mọi gate `neutral` cho `join.result = none`; task `superseded` không tính vào hợp và không bị mở lại; `quorum` với danh sách ngắn hơn `k` cho `fail` ngay |
| K22 | `trail` và `flow_transition`: mỗi lần rời bước có đúng một phần tử và một bản ghi nhật ký; `GET …/flow` trả `trail`; số phần tử không vượt `max_activations` |
| K23 | Mười hai mẫu luồng ở Phụ lục F.2 đánh dấu "có" đều có một FlowDoc trong `testdata/flows/patterns/` qua bộ kiểm và chạy đúng trong mô phỏng |
| K24 | Các tiêu chí N1 đến N10 ở 12.3 (nhập bộ, đề xuất, chạy thử, số liệu) xanh |

### 12.2 Bản nháp, xuất bản, quyền (C)

| # | Tiêu chí |
| --- | --- |
| C1 | `PATCH` hoặc `PUT …/handoffs/{status}` lên hàng đã xuất bản thì 409 `workflow_locked`; hàng catalog giữ 409 `workflow_managed_by_template` |
| C2 | Xuất bản hai lần từ cùng một bản nháp tạo `version` 1 rồi 2, hàng 1 không đổi một byte |
| C3 | Request khởi tạo trước khi xuất bản v2 chạy hết trên v1 (test: đặt v2 làm đang dùng giữa chừng) |
| C4 | Xuất bản bị từ chối đủ các mã R01 đến R04, R07, F01, P02 với `issues[]` liệt kê **tất cả** vấn đề một lượt |
| C5 | Bộ kiểm vai chạy trên mọi flow catalog và xanh (`TestCatalogFlowsPassValidateFlow` mở rộng); đường `CreateIssueWorkflow` từ chối workflow flow có agent là lead |
| C6 | Actor agent gọi mọi route soạn thì 403 `flow_authoring_member_only` |
| C7 | Thành viên không `canManageSquad` thì 403 `flow_publish_forbidden`; người không được gắn agent thì 403 `flow_agent_forbidden`; không phải admin mà tạo trạng thái mới thì 403 `status_create_forbidden` |
| C8 | `revision` lệch thì 409 `flow_draft_conflict`; phiên bản nền cũ thì 409 `flow_draft_stale` |
| C9 | `simulate` trên bản nháp kiểu Trade với gate bị `rejected` hai lần cho `end` là mã dừng `loop_exhausted` và `loops.fix = 2`; không ghi DB |
| C10 | `validate` trả `analysis.stops[]` khớp với điểm dừng thật mà `Decide` sinh ra cho cùng định nghĩa (test so hai chiều) |
| C11 | H1 đến H4 và H1m: mỗi hành vi ở mục 5 bị chặn với mã nêu, trên flow do người dùng xuất bản **và** trên flow catalog |
| C12 | Chạy lại kịch bản diễn tập Trade (`flow_rehearsal_trade_test.go`) trên flow do người dùng xuất bản từ bản sao Trade: xanh, cùng số sub-issue |
| C13 | Phản hồi sai (thiếu `doc`, `issues` null, `severity` lạ, `stop` lạ, `lineage_id` sai kiểu) qua zod không làm vỡ giao diện (test `schemas.test.ts`) |
| C14 | Parity 7 ngôn ngữ xanh; không có chữ Việt (ngoài nội dung tác giả flow) hay khóa thiếu |
| C15 | Điện thoại 390px: không cuộn ngang, Inspector mở bằng Sheet, hành động Check và Publish bấm được (kiểm bằng trình duyệt; ảnh chụp đính kèm PR) |
| C16 | `FF_FLOW_AUTHORING` tắt thì route soạn 404, nút ẩn, phiên bản đã xuất bản và request đang chạy không đổi |
| C17 | Mọi migration có down; `TestEveryConcurrentUpBuildHasCleanup` xanh với ba dòng mới; `TestMigrationFilesHaveMatchingDirections` xanh |
| C18 | Workflow SDLC, Planning và workflow dự án không đổi hành vi (test vàng `issue_workflow_golden_test.go` xanh) |
| C19 | Manifest nhập quá 512 KiB, khóa lạ, `version` cao hơn thì 400 `flow_manifest_invalid`; không tạo bản nháp |
| C20 | Thông điệp chứa `mention://` bị từ chối (P02) |

### 12.3 Từ bộ skill/agent tới workflow (N)

| # | Tiêu chí |
| --- | --- |
| N1 | Bốn bộ mẫu trong `server/internal/pack/testdata/` nhập được qua `preview` rồi `commit`: (a) chỉ có thư mục skill theo chuẩn Agent Skills, không agent; (b) thư mục agent kiểu Claude Code (`agents/*.md`) kèm `skills/`; (c) agent pack của Multica (`pack.json`); (d) bó gốc có `flow.json`. Mỗi bộ cho đúng số agent, skill và liên kết agent↔skill đã nêu trong test |
| N2 | `preview` không ghi gì vào DB (so số hàng trước/sau của fixture); `commit` là một giao dịch: lỗi giữa chừng không để lại agent hay skill nào |
| N3 | Bộ hỏng bị từ chối với mã đúng: vượt giới hạn `hub.DefaultLimits`, đường dẫn thoát thư mục, hai agent trùng khóa, agent nhắc skill không có trong bộ, `SKILL.md` thiếu `name`/`description`, tệp bị `hub.Scan` chặn (`pack_scan_blocked`) |
| N4 | Nhập lại cùng bộ (cùng `digest`) là no-op; bộ cùng `id` khác `digest` trả `pack_conflict` kèm lựa chọn (giữ, tạo bản mới) và không ghi đè agent hay skill đang có |
| N5 | Sau `commit` của bộ (a) (không agent): người dùng tạo agent trong giao diện có sẵn, gắn skill, và trình soạn chọn được skill đó cho `Task.skills` |
| N6 | Bộ (d): bản nháp sinh từ `flow.json` có mọi vai chưa gắn; ghép vai theo **lựa chọn của người nhập**, không theo tên; xuất bản được sau khi ghép |
| N7 | Đề xuất (16.4): agent gọi `POST /api/flow/validate` được 200; cùng agent gọi mọi route `/flow-drafts` vẫn 403 `flow_authoring_member_only`; tệp manifest đính ở issue nhập được thành bản nháp bởi người quản lý; bản nháp mang `origin: "proposal"` và xuất bản đòi ack `proposal_reviewed` |
| N8 | Chạy thử (16.5): request chạy thử ghim đúng phiên bản được chọn dù phiên bản đang dùng là bản khác; `trial = true` trong trạng thái; request chạy thử không vào terminal `leaves_workspace` nếu chưa có người chọn cạnh ra (không nhận `allow_unattended_release`) |
| N9 | Chọn workflow khi tạo request: `flow_workflow_id` của squad khác, của workflow không phải flow, hoặc gửi bởi agent đều không có hiệu lực (422 hoặc bỏ qua như đã nêu) |
| N10 | Số liệu (16.6): với một tập request fixture đã chạy, `flow-insights` trả đúng số request theo kết cục, số lần đi từng cạnh quay lại, số lần đỗ theo mã, số lần đóng thay; request của phiên bản khác không lẫn vào |

## 13. Ngoài phạm vi đợt này

- Task do máy thực hiện (`performer: system`, hàm có tên như `research.gate`); nhiều token trong một request (`fork`/`join`); bước `subflow` (request con chạy flow khác, D23); chờ sự kiện bên ngoài và hẹn giờ. Khóa tương ứng bị từ chối `flow_feature_unsupported`. (D12)
- Chuyển manifest catalog 21 flow sang schema 2; chế độ `inline` cho SDLC/BMAD.
- Chuyển request đang chạy sang phiên bản mới; sao chép flow giữa workspace qua sổ đăng ký.
- Máy chủ tự gọi mô hình để sinh flow. Đề xuất bản nháp đi qua một agent của workspace và một tệp manifest do người nhập (16.4); máy chủ không gọi mô hình nào.
- Suy luật từ văn bản của bộ: máy chủ không đọc `SKILL.md` hay hướng dẫn agent để đoán bước, vai, hay thứ tự (I5). Việc đọc hiểu là của agent đề xuất và của người duyệt.
- Giữ một phiên làm việc của agent xuyên nhiều task (task sau tiếp tục ngữ cảnh của task trước). Mỗi task là một issue riêng với lượt chạy riêng; ngữ cảnh đi qua tệp và liên kết đầu vào (4.11).
- Tự cập nhật bộ đã nhập khi nguồn đổi (đồng bộ nguồn là việc của Hub, `docs/engineering/skill-plugin-hub.md`).
- Trang soạn trên `apps/mobile` (mobile sở hữu UI riêng, `apps/mobile/AGENTS.md`).
- Bản dịch thông điệp dừng của tác giả flow.
- Xuất flow thành mẫu catalog; chia sẻ flow giữa workspace; soạn trực tiếp chuyển tiếp thô cho mọi trường hợp (chỉ có chế độ Advanced cho flow nhập).
- Kiểm tra mobile chịu được trạng thái lạ và trường mới (chưa kiểm).

## 14. Giả định và điều chưa kiểm chứng

### 14.1 Số liệu là lựa chọn hay ước lượng (không đo)

- Các trần ở 2.7: 40 bước, 12 trạng thái bảng khác nhau, 12 task mỗi bước, 16 vai và 16 biến, 8 bộ verdict, 8 vòng, 20 thực thể, vòng tối đa 20, 300 issue task mỗi request (cứng 500), tài liệu 512 KiB. Chọn theo cỡ giao diện và chi phí; do máy chủ đặt và đổi được không cần phát hành client mới.
- 20 bản nháp mỗi squad và 50 phiên bản mỗi lineage (2.2): lựa chọn, chưa có dữ liệu sử dụng.
- `policy` mặc định: `max_activations` 100, `human_remind_minutes` 1440, `human_stall_minutes` 10080 (2.3): lựa chọn.
- Chi phí ngày công trong kế hoạch: ước lượng.
- Bản dịch 7 ngôn ngữ: đề xuất (11.7).

### 14.2 Chưa kiểm chứng

- H1 đến H4 và H1m: đọc từ mã, chưa có test chạy (mục 5; kế hoạch P1).
- Thông báo hộp thư khi task flow giao cho thành viên: `IssueService.Create` nhận `AssigneeType` bất kỳ (`service/issue.go:403`, `:430`) và chỉ xếp lượt chạy cho agent (`service/issue.go:1003`); bộ nghe sự kiện tạo issue ở `cmd/server/notification_listeners.go:687` chưa đọc nên chưa biết có thông báo cho người được giao hay không.
- Hành vi của `Evaluate` với trường lạ (`transitions.go:235-247`) suy từ đọc mã, chưa chạy.
- Giá trị phiên bản CLI tối thiểu cho verdict tùy chỉnh: chưa xác định (tiền lệ `MinQuickCreateCLIVersion`, `handler/issue.go:3011`).
- Mức sửa bộ xây điều kiện: đã đọc `condition-builder.tsx` (348 dòng); builder dùng hằng của `condition-spec.ts` ở bốn chỗ (`:216`, `:217`, `:229`, `:234`) và `condition-spec.ts` có 15 tham chiếu tới các hằng và hàm đó; việc tham số hóa theo `FactSpec[]` là sửa vừa, chưa thử.
- Nơi daemon dùng `skill.config`: chưa tìm thấy chỗ máy chủ đọc `config` để điều khiển quy trình; chưa xác nhận có nơi dùng khác.
- Quy tắc ánh xạ trạng thái khi nhiều bước chung một trạng thái (`flow_step_ambiguous`, 4.7) là thiết kế mới, chưa có mã đối chứng.
- Mobile (`apps/mobile`) chịu được trạng thái lạ và trường mới: chưa kiểm.
- Số dòng ở các tệp mà công việc khác đang sửa (`flow_guards.go`, `issue.go`, `flow_state.go`, `issue-flow-state-panel.tsx`, `designer/*`, nhóm `issue_wakeup*`) có thể lệch sau khi họ commit.
- Phép kiểm quyền chính xác của route tạo agent và route nhập skill (mục 10 dòng "Nhập bộ") chưa đọc tới cùng; P23 đọc và dùng lại đúng hàm đó, không viết luật quyền mới.
- Định dạng tệp agent kiểu Claude Code (`agents/*.md`, frontmatter `name`, `description`, tùy chọn `tools`, `model`) đã đối chiếu với bộ Kit Việt upstream đang nằm trong repo (`catalog/_sources/kitviet/upstream/<ngành>/agents/*.md` và `skills/*/SKILL.md`), chưa đối chiếu với một bản đặc tả có phiên bản; khóa `skills:` trong frontmatter của agent chưa thấy ở bộ đó. Bộ đọc chỉ dùng `name`, `description` và thân tệp, bỏ qua khóa lạ, nên sai lệch ở các khóa khác không gây hỏng. Chuẩn Agent Skills đã đối chiếu (Phụ lục F.3).
- Truy vấn số liệu 16.6 quét `issue.flow` của request đã đóng theo `workflow_id`; chưa đo trên dữ liệu lớn. Có giới hạn 500 request gần nhất; nếu chậm thì thêm chỉ mục biểu thức ở một migration riêng (`CONCURRENTLY`), chưa đưa vào kế hoạch.
- Chất lượng bản nháp do agent đề xuất phụ thuộc mô hình và skill `flow-authoring`; chưa thử. Bộ kiểm chặn tài liệu sai cấu trúc, không chặn tài liệu đúng cấu trúc nhưng quy trình dở; vì vậy có ack `proposal_reviewed` và chạy thử.

## 15. Rà soát kiến trúc lần 2: kết luận và những gì đã đổi

### 15.1 Kết luận

**Giữ lõi.** Phương án (ii) của Phụ lục D (đồ thị khai báo, bộ diễn giải cố định, một nguồn sự thật ở Postgres, một token, song song trong bước) là đúng cho bài toán này. Nó cùng họ với Amazon States Language và Conductor (định nghĩa là JSON, engine diễn giải), khác LangGraph, CrewAI và Temporal (định nghĩa là mã). Với sản phẩm mà người dùng tự soạn quy trình trong giao diện và máy chủ phải kiểm trước khi chạy, họ "dữ liệu" là họ duy nhất kiểm tĩnh và mô phỏng được. Ba lựa chọn của bản trước mà đợt rà soát xác nhận lại: `Lift` làm trình đọc schema 1 lâu dài (D11), biểu thức là cây JSON có sổ dữ kiện chứ không phải ngôn ngữ nhúng (Conductor dùng JavaScript trong `loopCondition` và tài liệu của chính họ phải cảnh báo vòng không chặn; ta không có lớp lỗi đó), và mọi chu trình phải đi qua cạnh quay lại có đếm (LangGraph chỉ có một trần `recursion_limit` cho cả lần chạy và báo lỗi khi chạm; ta có trần theo từng vòng, kiểm tĩnh, và một cạnh khai báo cho lúc cạn).

**Nhưng bản trước chưa đạt mục tiêu của người dùng**, vì hai lý do. Thứ nhất, mục tiêu bắt đầu bằng "đưa lên một bộ skill, agent bất kỳ", còn bản trước để việc đó ra ngoài đợt và đòi một định dạng bó riêng mà không bộ nào ngoài kia có. Thứ hai, mô hình engine có sáu lỗ làm một workflow soạn cho bộ bất kỳ **chạy không ổn** dù qua được bộ kiểm. Bảng dưới liệt kê từng lỗ và chỗ sửa.

### 15.2 Lỗ hổng và chỗ sửa

| # | Vấn đề ở bản trước | Hệ quả nếu để nguyên | Sửa | Quyết định |
| --- | --- | --- | --- | --- |
| V1 | Nhập bộ ở ngoài phạm vi; bó phải theo `multica.squad-bundle/v1` | Người dùng không đưa được bộ có sẵn lên; mục tiêu không đạt dù engine xong | Mục 16.1 đến 16.3, Phụ lục E: nhận bốn định dạng có sẵn | D14 |
| V2 | Không có đường từ quy trình markdown tới bản nháp ("sinh flow bằng AI" bị loại) | Bộ kiểu Kit Việt phải chép tay từng bước | 16.4: agent đề xuất tệp manifest, người nhập; route kiểm không trạng thái | D15 |
| V3 | Đầu vào của task chỉ có một câu ("theo cạnh đã đi"); `inputs?` có ở Phụ lục B nhưng không có ở 2.3 và 4 | Task sau không biết đọc sản phẩm nào; vòng sửa không biết ai chê gì ngoài vòng `rework` | 2.3 `Task.inputs`, 4.11, `trail` | D16 |
| V4 | Chỉ lead, quản lý, người yêu cầu đặt biến | Bộ có agent lập kế hoạch riêng không sinh được danh sách cho `instances`; quyết định theo số liệu dồn hết về lead; chỗ "không map sạch" của phòng nghiên cứu | 2.3 `Task.sets`, 4.12, E22 | D17 |
| V5 | Task không biết skill | Workflow soạn cho bộ skill mà không nói được task nào dùng skill nào, và không kiểm được agent có skill đó | 2.3 `Task.skills`, R09 | D18 |
| V6 | R07 cấm gate chung người với **bất kỳ** task `work` nào trong flow | Bộ nhỏ (2 đến 3 agent) không xuất bản được; ví dụ duyệt chéo của chính tài liệu (C.2) vi phạm | R07, E13 theo đầu vào | D19 |
| V7 | Rời workspace chỉ đòi `checkpoints` không rỗng | Người duyệt sớm, agent viết lại sau, sản phẩm vẫn ra ngoài | 4.8, Y01, E15 theo `unreviewed` | D20 |
| V8 | `mode: all` cho `pass` khi không có `fail`, kể cả mọi phiếu `neutral` | Không ai đồng ý vẫn qua cổng | 4.3, J02 | — |
| V9 | Task engine hủy (`any`, `quorum`) và task người hủy không phân biệt được, trong khi E7 mở lại task `work` bị hủy | Engine hủy rồi lại tự mở lại; vòng vô ích | 2.5 `superseded`, E23 | D21 |
| V10 | `when`/`instances` đánh giá lại mỗi lần mà không nói task đã mở thì sao, và danh sách đổi giữa chừng thì thực thể nào ứng phần tử nào | Hành vi không xác định khi đổi biến giữa bước | 4.2a, E24 | D21 |
| V11 | G11 tính "tích theo cây lồng", nhưng đồ thị cho phép vòng không lồng nhau; không có lập luận dừng | Phân tích chi phí không định nghĩa được cho đồ thị hợp lệ | 4.5: thân vòng, công thức, lập luận dừng | — |
| V12 | `quorum` với `instances.from_var`: `k` có thể lớn hơn số thực thể lúc chạy; `block: at_close` không có nghĩa | Bước treo | 2.3 Join, `instances.min`, 4.3 | — |
| V13 | Chọn người trong hồ bơi không được định nghĩa | Hai lần chạy cùng dữ liệu giao việc khác nhau; người sửa không phải người viết | 2.5 | — |
| V14 | Không có nhật ký quyết định và không có số liệu theo phiên bản | Không biết vì sao request đi đường đó; không có dữ liệu để sửa workflow | 4.13, 16.6 | D22 |
| V15 | Muốn thử một phiên bản phải đặt nó làm bản đang dùng của squad; một squad chỉ chạy một flow | Thử nghiệm ảnh hưởng mọi request mới; bộ có hai quy trình phải nhồi vào một flow | 16.5 | D22 |
| V16 | Bài vi phân chỉ duyệt trạng thái tổng hợp; P11 đổi bộ diễn giải một lần | Trạng thái thật do DB sinh ra có thể lệch mà test không thấy | 9.3 điều 7 | D24 |
| V17 | Cổng P0 chờ người dùng xác nhận D11 đến D13 và cấp DSN | Kế hoạch đứng yên | Mục 1; kế hoạch P0 | — |

### 15.3 Đã cân nhắc và không đổi

- **Hai bộ diễn giải (giữ engine schema 1, viết engine schema 2 bên cạnh).** Ít rủi ro cho P11 hơn, nhưng mọi tệp phía handler (`flow_engine.go`, `flow_engine_ops.go`, `flow_guards.go`, `flow_sweep.go`, `flow_state.go`) phải rẽ nhánh mãi mãi, và mỗi luật mới phải viết hai lần. `Lift` cộng bài vi phân cộng D24 cho cùng mức an toàn mà chỉ còn một đường chạy.
- **Nhiều token trong một request.** Xem B.10 (D23).
- **Máy chủ gọi mô hình để sinh flow.** Thêm khóa API, chi phí, và một bề mặt tấn công ở máy chủ; nền tảng đã có agent chạy trên runtime của người dùng. 16.4 dùng đúng thứ đó.
- **Đầu ra có kiểu riêng cho task (`task.<key>.out.<field>`), tách khỏi biến.** Thêm một họ dữ kiện thứ tư và một bộ kiểm thứ hai cho cùng một việc; `Task.sets` dùng lại sổ biến, kiểu, miền, nhật ký đã có.
- **Chụp kế hoạch task lúc vào bước (không đánh giá lại `when`).** Xác định hơn, nhưng đổi hành vi hiện có của Kit Việt (quản lý bật công tắc giữa bước thì task mở thêm) và buộc `Lift` giữ thêm một ngoại lệ. Hai quy tắc ở 4.2a cho cùng tính xác định mà không đổi hành vi.
- **Bỏ `compat: "v1"`, chấp nhận văn bản và số vòng của request đang chạy đổi.** Rẻ hơn khoảng 3 ngày công, nhưng D5 chỉ cho phép đổi hành vi theo danh sách H1 đến H4.

## 16. Từ bộ skill/agent tới workflow (R3)

Đây là đường đi trọn vẹn của mục tiêu: **đưa bộ lên → có squad → có bản nháp → kiểm, mô phỏng, chạy thử → xuất bản → đọc số liệu → sửa.** Mọi bước sau "có squad" dùng lại R1 và R2.

### 16.1 Định dạng nhận (D14)

Máy chủ nhận tệp zip hoặc nguồn mà Hub đã biết lấy (GitHub repo/thư mục, URL zip; `server/internal/hub/importers.go`), giải nén và giới hạn bằng `hub.DefaultLimits` (`server/internal/hub/types.go:65`), rồi nhận dạng theo **thứ tự cố định**. Gói mới `server/internal/pack` chứa phần thuần (nhận dạng, chuẩn hóa, kiểm); không sửa `hub.Detect` (nó chỉ biết một skill hoặc một plugin).

| Thứ tự | Dấu hiệu | Hiểu là | Agent | Skill |
| --- | --- | --- | --- | --- |
| 1 | `squad-bundle.json` ở gốc | Bó gốc (Phụ lục E.1) | theo `agents[]` | theo `skills/` |
| 2 | `pack.json` ở gốc | Agent pack của Multica (`packages/core/agent-packs/schema.ts`, `catalog/packs/<id>/`) | một agent | theo `skills[]` |
| 3 | Thư mục `agents/` có tệp `*.md` với frontmatter `name` và `description` (kiểu Claude Code; có hoặc không có `.claude-plugin/plugin.json`) | Bộ agent | mỗi tệp một agent: `name`, `description`, thân tệp là hướng dẫn | mọi `skills/*/SKILL.md`; agent có khóa `skills:` trong frontmatter thì chỉ gắn các skill đó, không có thì để người nhập chọn |
| 4 | Một hoặc nhiều thư mục có `SKILL.md` (sâu tối đa 3 cấp) theo chuẩn Agent Skills | Bộ chỉ có skill | không có | mỗi thư mục một skill |
| — | Không khớp, hoặc khớp hai dấu hiệu ở gốc | `pack_invalid` | | |

Quy tắc đọc: khóa frontmatter lạ bị bỏ qua (chuẩn Agent Skills quy định như vậy cho tương thích xuôi); `SKILL.md` thiếu `name` hoặc `description` là lỗi; tên skill theo `^[a-z0-9][a-z0-9._-]*$`; tên agent chuyển thành khóa `^[a-z][a-z0-9_]{1,31}$` (chữ thường, ký tự khác thành `_`), trùng khóa là lỗi. `tools`, `model`, `allowed-tools`, biến môi trường và cấu hình MCP trong bộ **không** được áp: `preview` liệt kê chúng dưới mục "bị bỏ qua" để người nhập tự cấu hình agent. Máy chủ không đọc thân `SKILL.md` hay hướng dẫn agent để đoán vai, bước, lead hay thứ tự.

**Tài liệu tham khảo.** `preview` liệt kê các tệp markdown có thể chứa quy trình: hướng dẫn của từng agent, `SKILL.md` của từng skill, và tệp `.md` khác trong bộ (`README.md`, `workflows/**`, `docs/**`; tối đa 20 tệp loại tài liệu, mỗi tệp 200 KiB. Hướng dẫn agent và `SKILL.md` không bao giờ bị cắt; số tài liệu bị bỏ nằm ở `references_omitted`). Đây chỉ là danh sách để người nhập chọn đưa cho agent đề xuất (16.4) và mở trong ô tham khảo của trình soạn (11.9); máy chủ không phân tích chúng.

### 16.2 `preview`

`POST /api/squads/packs/preview` (multipart tệp zip, hoặc `{ "source": { "kind": "github"|"url_zip", "url": … } }`) → `200 PackPreview`. Không ghi DB; bó đã giải nén giữ trong bộ nhớ đệm phía máy chủ theo `digest` trong 30 phút (cùng cách hub giữ bó giữa preview và commit).

```jsonc
{ "digest": "sha256:…", "format": "claude_agents", "id": "…", "name": "…", "version": "…",
  "agents": [ { "key": "researcher", "name": "Researcher", "description": "…", "instructions_bytes": 3120,
                "skills": ["web-research"], "existing_agent_id": null } ],
  "skills": [ { "name": "web-research", "files": 4, "existing_skill_id": "uuid|null", "same_content": false } ],
  "leader": "researcher|null",                       // chỉ bó gốc khai; các định dạng khác là null
  "flow": { "present": true, "validation": { …FlowValidation… } } ,
  "references": [ { "path": "README.md", "bytes": 5120, "kind": "doc" } ],
  "references_omitted": 0,
  "ignored": [ { "agent": "researcher", "key": "model" } ],
  "scan": [ { "path": "skills/x/scripts/run.sh", "severity": "warn|block", "rule": "…" } ],
  "issues": [ { "code": "pack_skill_missing", "path": "agents[researcher].skills[foo]" } ] }
```

Có phát hiện `block` của `hub.Scan` (`server/internal/hub/scan.go`) thì `commit` bị từ chối `pack_scan_blocked`; phát hiện `warn` hiển thị và cần người nhập xác nhận.

### 16.3 `commit`

`POST /api/squads/packs/commit` với lựa chọn của người nhập:

```jsonc
{ "digest": "sha256:…",
  "squad": { "name": "…" },
  "agents": [ { "key": "researcher", "action": "create|use_existing|skip", "existing_agent_id": "uuid?",
                "runtime_id": "uuid?", "skills": ["web-research"] } ],
  "extra_agents": [ { "key": "editor", "name": "Editor", "instructions": "…", "runtime_id": "uuid", "skills": [] } ],
  "leader": { "agent": "researcher" } | { "generic": true, "runtime_id": "uuid" },
  "skills": { "web-research": "keep_existing|import_as_new" },
  "create_draft_from_flow": true, "ack_scan_warnings": true }
```

- **Một giao dịch**: nhập skill (đường `createHubSkill`, `hub_import.go:925`), tạo agent (cần `runtime_id`, như `CreateAgentRequest`, `agent.go:1405`), gắn skill, tạo squad với lead (`CreateSquad` đòi `leader_id`, `squad.go:291-310`), thêm thành viên (`squad_member.role` = nhãn vai), ghi `squad_pack_install`. Lỗi ở bất kỳ bước nào thì không còn gì.
- **Lead.** Bó gốc khai `leader`. Định dạng khác: người nhập chọn một agent của bộ, hoặc chọn **lead chung**: agent tạo từ mẫu catalog mới `catalog/agents/flow_lead/` (hướng dẫn: làm theo lời nhắc của workflow, không tự chia việc). Lead chung là lựa chọn mặc định cho bộ chỉ có skill.
- **Bộ chỉ có skill.** `extra_agents` cho người nhập khai agent ngay trong trình nhập (tên, skill, runtime); có nút "một agent cho mỗi skill" (thao tác cơ học, không suy đoán). Không khai agent nào thì `commit` chỉ nhập skill và không tạo squad.
- **Không ghi đè.** Skill trùng tên: giữ bản đang có hoặc nhập tên mới (`<name>-2`); agent trùng tên: dùng bản đang có hoặc tạo mới. Không sửa agent hay skill đang có.
- **Chạy lại.** Cùng `digest` đã cài cho cùng squad: 200 với kết quả cũ. Cùng `id` khác `digest`: 409 `pack_conflict` kèm bản cài trước; người nhập chọn tạo squad mới.
- **Bản nháp.** Có `flow.json` hợp lệ và `create_draft_from_flow`: tạo bản nháp `origin: pack`, mọi vai chưa gắn trừ vai có khóa trùng khóa agent vừa tạo (ghép theo **khóa do bó khai**, không theo tên hiển thị; người nhập sửa được trước khi xuất bản).

**Bảng `squad_pack_install`** (migration 994 đến 997, cùng mẫu 990 đến 993: bảng, chỉ mục khóa chính `CONCURRENTLY`, gắn khóa chính, chỉ mục `(workspace_id, squad_id, installed_at DESC)`): `id`, `workspace_id`, `squad_id`, `pack_id TEXT`, `pack_version TEXT`, `format TEXT`, `digest TEXT`, `source JSONB`, `roster JSONB` (khóa agent → `agent_id`, tên skill → `skill_id`), `installed_by`, `installed_at`. Không khóa ngoại. Không dùng `hub_item` (`kind` bị `CHECK ('skill','plugin')`, `migrations/930_hub_item.up.sql:7`).

### 16.4 Từ quy trình markdown tới bản nháp: đề xuất của agent (D15)

Nguyên tắc: **agent đọc và đề xuất; máy chủ kiểm; người quyết.** Không có quyền ghi mới nào cho agent.

1. Trong hộp thoại tạo workflow, người quản lý chọn "Ask an agent to draft": chọn agent soạn (một agent bất kỳ của workspace mà họ được giao việc; mẫu catalog `catalog/agents/flow_designer/` kèm skill `flow-authoring` là lựa chọn gợi ý) và chọn tài liệu tham khảo (hướng dẫn agent, skill của squad, tệp tải lên).
2. `POST /api/squads/{id}/flow-drafts` với `from: { "kind": "proposal", "agent_id": …, "references": […], "note": "…" }` (route soạn, chỉ thành viên): máy chủ tạo **bản nháp rỗng** `origin: proposal` và **một issue thường** giao cho agent soạn, `source_ref.issue_id` trỏ tới nó. Mô tả issue do máy chủ sinh: danh sách thành viên squad (khóa gợi ý, tên, skill đang có, ai là lead), tài liệu tham khảo đính kèm, và yêu cầu đầu ra: một tệp `flow.json` (`multica.flow-manifest/v1`) đính vào issue, đã qua `multica flow validate --file flow.json --squad <id>` không còn lỗi. Issue này không chạy trên flow nào.
3. Agent làm việc như với mọi issue. Nó gọi được đúng một route của phần soạn: `POST /api/flow/validate` (7.1), không ghi gì. Skill `flow-authoring` (`catalog/skills/builtin/flow-authoring/`) chứa: tham chiếu schema 2, bảng mẫu luồng (Phụ lục F.2) kèm FlowDoc mẫu, cách đọc một quy trình văn xuôi thành bước/vai/cổng/vòng, và danh sách điều máy chủ từ chối.
4. Khi issue có tệp đính kèm tên kết thúc `flow.json` do agent soạn tải lên, trang bản nháp hiện "Import proposal". Người quản lý bấm: `POST /flow-drafts/{draftId}/import-proposal` đọc tệp mới nhất, kiểm như nhập manifest (7.6), thay `doc` của bản nháp (`revision + 1`). Từ đây là bản nháp bình thường: sửa, Check, Simulate.
5. Xuất bản bản nháp `origin: proposal` đòi ack `proposal_reviewed` (`canManageSquad`): "tôi đã đọc quy trình này". Ack ghi vào `policy.acks` của phiên bản.

Vì sao an toàn: tài liệu trong bộ có thể chứa lời dụ agent soạn một quy trình xấu, nhưng sản phẩm của agent chỉ là một tệp; nó không thành luật cho tới khi qua bộ kiểm (không vai lạ, không thông báo người tùy ý, Y01 về rời workspace), qua mắt người, và người đó xuất bản. Agent không gắn được vai (`bind` bị bỏ khi nhập), không bật được `flow_auto`, không đặt được phiên bản làm bản đang dùng.

Bộ **không có** quy trình viết sẵn đi cùng đường này: người quản lý viết vài dòng ở `note` mô tả quy trình mong muốn, hoặc bắt đầu từ bản nháp trắng và các mẫu bước (11.2).

### 16.5 Chạy thử và chọn workflow theo request (D22)

- **Chạy thử.** Trang một phiên bản đã xuất bản có nút "Trial run": tạo một request (issue giao cho squad, tiêu đề do người dùng nhập) với `flow_workflow_id` là phiên bản đó và `trial: true`. Phiên bản không cần là bản đang dùng của squad, nên thử nghiệm không ảnh hưởng request khác. Request chạy thử chạy bằng agent thật trên đúng engine; khác biệt duy nhất: `trial = true` hiện huy hiệu, bị loại khỏi số liệu mặc định (16.6), và `allow_unattended_release` không có hiệu lực (phải có người chọn cạnh ra). Cài đặt: `decisionFor` coi `st.Trial` như chính sách không cho phép; công việc chưa ai xem (`unreviewed`) vào terminal `leaves_workspace` thì dừng `stalled` (`release_without_checkpoint`) cho tới khi có checkpoint (cổng người, bước thủ công) hoặc resume có `acknowledge_skip_checkpoint`. Request chưa có công việc nào của agent (không `unreviewed`) vẫn ra được. Nút "Trial run" nằm ở từng dòng phiên bản đã xuất bản (schema 2) trên trang squad, mở hộp thoại hỏi tiêu đề request mẫu và tạo issue bằng `flow_workflow_id` + `flow_trial`; trang request hiện huy hiệu "Trial" cạnh tên workflow.
- **Chọn workflow khi tạo request.** Squad có nhiều phiên bản flow đã xuất bản thuộc nhiều lineage: form tạo issue giao cho squad hiện bộ chọn workflow (mặc định: bản đang dùng). Máy chủ nhận `flow_workflow_id` chỉ khi nó là workflow flow của chính squad đó (`issue_workflow.squad_id`, hoặc chính bản đang dùng của squad), ngược lại `422 flow_workflow_invalid`. Bộ có hai quy trình (viết bài, viết bản tin) là hai lineage của một squad, không phải một flow nhồi biến `route`.
- Request do routine hay agent tạo luôn dùng bản đang dùng.

### 16.6 Số liệu vận hành theo phiên bản (D22)

`GET /api/issue-workflows/{id}/flow-insights?include_trial=false` trả, cho tối đa 500 request gần nhất ghim phiên bản đó:

```jsonc
{ "requests": { "total": 42, "open": 5, "by_end": { "terminal:done": 30, "stop:loop_exhausted": 4, "stop:grounds_missing": 2, "stalled": 1 } },
  "steps": [ { "key": "gate", "entered": 51, "median_minutes": 38, "p90_minutes": 190 } ],
  "edges": [ { "step": "gate", "edge": "gate_fix", "taken": 14 } ],
  "loops": [ { "key": "fix", "max": 2, "histogram": [28, 10, 4] } ],
  "stalls": [ { "reason": "flow_agent_unavailable", "count": 1 } ],
  "manual_closes": 3, "truncated": false }
```

Nguồn: `issue.flow.trail`, `issue.flow.stop`, `activity_log` (`flow_manual_close`). Hàm thuần `Insights(states []State) Report` ở `internal/issueworkflow`, truy vấn chỉ đọc. Trình soạn hiện các số này cạnh từng bước và cạnh khi mở một phiên bản ("14 trong 51 lần qua cổng này bị trả về"), là dữ liệu để quyết định sửa hướng dẫn task, đổi giới hạn vòng, hay thêm cổng. Không có khuyến nghị tự động.

### 16.7 Quyết định ngoài đặc tả

Chi tiết ở [flow-pack-drill.md](flow-pack-drill.md) mục 7.

- `commit` nhận `new_install: true`: bỏ qua kiểm bản cài trước, tạo squad mới. Đây là cách gửi lựa chọn "tạo squad mới" sau `409 pack_conflict` (16.3); cùng `digest` thì không có đường nào khác để cài lần hai.
- `409 pack_name_conflict` kèm `agents` và `skills` là các tên trùng, khi người nhập chưa chọn cách xử lý tên trùng (16.3, "Không ghi đè"). Tách khỏi `pack_conflict` để giao diện nêu đúng tên.
- `issue.flow.stop.reason` (`omitempty`, chỉ khi `code` là `stalled`) ghi vì sao engine treo request; `stalls[]` của 16.6 đọc từ đây.
- Hạn mức 20 tài liệu tham khảo chỉ áp cho loại `doc`; `preview` trả thêm `references_omitted` (16.1, 16.2).
- `hub.ExtractZip` không bóc thư mục duy nhất tên `agents`, `skills`, `docs`, `workflows` hay `references`: đó là nội dung của bộ, không phải vỏ bọc.

## Phụ lục A. Danh mục giả định cứng của engine hiện tại

Ba loại: **(i) cấu trúc**, phải đổi mô hình hoặc mã lõi. **(ii) chính sách**, nên là dữ liệu cấu hình được (có trần do máy chủ đặt). **(iii) bất biến engine**, luôn cưỡng chế bất kể flow viết gì, hoặc phải thêm. Cột cuối: "có sẵn" là thứ tái dùng được; "mới" là thứ phải viết.

### A.1 Hình dạng đồ thị

| # | Giả định | Bằng chứng | Loại | Có sẵn / mới |
| --- | --- | --- | --- | --- |
| A1 | Mỗi bước là một trạng thái workspace, **1-1**: một trạng thái không xuất hiện hai lần, trạng thái phải có trong danh mục | `issueworkflow.go:499-512` (`status %q appears more than once`, `does not exist`) | (i) | Mới: tách `step.key` khỏi `step.status` |
| A2 | Đang ở bước nào được suy từ `issue.status`; thành viên kéo thẻ sang trạng thái khác thì engine "đi theo" | `flow_engine.go:550` (`run.issue.Status != run.state.Step`), `flow_engine_ops.go:368-` (`followManualMove`) | (i) | Mới: bước theo khóa trong `issue.flow`; trạng thái chỉ là hiển thị |
| A3 | Tối đa 12 trạng thái, trong đó 4 trạng thái cố định (`todo`, `blocked`, `done`, `cancelled`) còn lại 8; flow Kit Việt dùng một trong 8 cho `in_review` (bước chỉ-người) nên còn **7 bước làm việc**; workflow thường tối đa 50 | `flow.go:40`, `flow.go:508`; `issueworkflow.go:48`; `fixedHead/fixedTail` `issueworkflow.go:61-62` | (ii) sau khi làm A1 | Số liệu thành trần máy chủ (xem 2.7) |
| A4 | Thứ tự **vị trí** là thứ tự bảng: `Arrange` đặt trạng thái cố định đầu/cuối, thứ tự giữa là thứ tự cột | `issueworkflow.go:103-128` (`Arrange`) | (i) | Mới: đồ thị không phụ thuộc thứ tự cột |
| A5 | "Bước trước" tính theo vị trí: gate không trùng agent với task `work` của bước **liền trước theo chỉ số** | `flow.go:633-643` | (i) | Thay bằng ràng buộc tách vai (iii, R07 và E13) |
| A6 | Chuyển tiếp `rework` chỉ trỏ về bước **đứng trước theo chỉ số** | `flow.go:779` (`target >= i`) | (i) | Mới: cạnh quay lại khai báo `loop`, đích theo khóa |
| A7 | Đầu vào của task = task của bước **gần nhất đứng trước theo chỉ số** đã chạy trong vòng đó | `flow_engine.go:695-713` (`flowTaskLinks`) | (i) | Mới: đầu vào theo cạnh đã đi (`came_from`) hoặc khai báo |
| A8 | `stage` của sub-issue = chỉ số bước + 1 | `flow_engine.go:626-631` | (i) | Mới: `stage` theo thứ tự topo hoặc bỏ |
| A9 | Bước bắt đầu là trạng thái chữ `todo`, và là bước plan của lead | `issueworkflow.go:56` (`StartStatus`), `flow.go:359`, `:522-531`, `flow_engine_ops.go:104`, `:175`, `:238`, `service/flow_start.go:66` | (i) | Mới: `flow.start` theo khóa; bước loại `lead` |
| A10 | Đích kết thúc là trạng thái nhóm `done`/`closed`; request tự đóng khi route vào đó | `flow_engine.go:319-322` | (ii) | Có sẵn nhóm trạng thái; mới: `terminals[]` khai báo |
| A11 | Đúng **một** token: `issue.flow.step` là một chuỗi | `flow.go:279-281` | (iii) giữ | Giữ, ghi rõ (xem B.9, D12) |
| A12 | Chỉ một flow chạy mỗi squad (`squad.active_workflow_id`); route khác nhau dùng công tắc enum `route` | `service/flow_start.go:34-37` | (ii) | Giữ; `route` thành `var` kiểu enum |

### A.2 Vai, task, hợp

| # | Giả định | Bằng chứng | Loại | Có sẵn / mới |
| --- | --- | --- | --- | --- |
| A13 | Task chỉ giao cho **agent**; handler là UUID cố định trong định nghĩa | `flow.go:619`; `flow_engine.go:643` (`AssigneeType: "agent"`); `flow.go:60-70` | (i) | Mới: vai (`Role`) có loại agent/người/lead, biên dịch ở xuất bản |
| A14 | Vai chỉ là khóa trong manifest catalog; DB không lưu khóa vai (`squad_member.role` là văn bản tự do) | `catalog/squads/kv_trade/template.json` (`squad.roles[].key`); `migrations/084_squad.up.sql:22` | (i) | Mới: `roles[]` trong tài liệu flow |
| A15 | Mỗi `(bước, task, vòng)` đúng một issue; không có nhiều thực thể cùng khóa task | `migrations/986_issue_flow_task_once_idx.up.sql`; `flow_engine.go:678` | (i) | Mới: hậu tố thực thể `key~n` trong `flow_task` (cột tự do, không migration) |
| A16 | Mọi task của bước mở **cùng lúc** khi vào bước; chỉ điều kiện `when` trên công tắc | `flow.go:227-238` (`ExpectedTasks`), `Decide` `flow.go:352-394` | (i) | Mới: `after` (phụ thuộc trong bước), `instances` |
| A17 | Gộp (join) cố định: chờ **mọi** task đóng; `blocked` thắng và thoát sớm; gate không `approved` (kể cả bị hủy) tính `rejected`; task `work` không góp verdict; không gate = `none` | `flow.go:455-488` (`tasksOutcomeOf`), `:380-382` | (i) | Mới: `join` {all, any, quorum} |
| A18 | Task là **lượt chạy agent**: quét tự xếp lượt chạy, thử lại, hủy; hết lượt thì `stalled` | `flow_sweep.go:132-188`, `flow_deadline.go:20-77` | (i) | Mới: task người dùng nhắc hạn, không xếp lượt chạy |
| A19 | Bước chỉ-người chỉ nhận task có khóa `export` và có `when` | `flow.go:645-650` | (i) | Mới: `step.exit = manual` + cạnh `trigger: manual` |
| A20 | Hết thời gian là cấu hình của **cả flow**, không theo task | `flow.go:120-170`, `flow_deadline.go:33-40` | (ii) | Mới: ghi đè theo task |
| A21 | Lead là người lập kế hoạch và không nhận task: quy tắc ở công cụ catalog, không ở Go; lead của squad hoặc người `canManageSquad` gọi `plan` | `scripts/catalog/lib/build-workflows.mjs:200`; `flow_api.go:75-105`, `:157-166` | (ii) + (iii) | Mới: vào Go |
| A22 | Quy tắc "vai thuộc squad / lead không làm task / mọi vai được dùng" chỉ ở `.mjs` | `build-workflows.mjs:199`, `:200`, `:241`; không có trong `flow.go:493-` | (iii) | Mới: vào Go (`ValidateFlowForSquad`) |
| A23 | Gate không giao cho agent đã làm việc cùng "vòng"; kiểm lúc mở task và lúc ghi | `flow_engine.go:608-620`, `flow.go:633-643` | (iii) | Có sẵn dạng cứng; mới: khai báo `separation` |

### A.3 Verdict, tệp, đóng task

| # | Giả định | Bằng chứng | Loại | Có sẵn / mới |
| --- | --- | --- | --- | --- |
| A24 | Bốn verdict cố định `approved`, `rejected`, `blocked`, `done` | `transitions.go:19-24`, `IsOutcome` `:87`; `issue.go:4247`, `:5033`; `cmd/multica/cmd_issue.go:2199`; `flow_guards.go:50` | (i) | Mới: `verdicts[]` theo task; cột `issue.outcome` là `TEXT` nên không migration |
| A25 | Gate chỉ đóng bằng `approved`/`rejected` | `flow_guards.go:50`, `:284-300` | (i) | Như trên |
| A26 | "Có tệp" là một boolean; đếm tệp do **agent được giao** tải lên; không loại, không số tối thiểu | `flow.go:29`, `flow_guards.go:319-330`, `flow.sql` `CountAgentAttachmentsOnIssue` | (i) | Mới: `requires.files[]` {slot, min, max, ext} và người tải hợp lệ |
| A27 | Không kiểm **ai** đóng task (H1) | `flow_guards.go:248` (`checkFlowTaskStatus` không nhận actor) | (iii) | Mới (D5) |
| A28 | Đổi người nhận / hủy task / đổi verdict đã đóng không bị chặn (H2–H4) | `flow_guards.go:136-139` (trả sớm khi trạng thái không đổi), `:274` (chỉ kiểm nhóm `done`), `:333-350` | (iii) | Mới (D5) |
| A29 | Lời nhắc cho agent, CLI và mô tả task đều gắn cứng `approved|rejected` | `flow.go:920-962`, `:985-1018`; `cmd_issue.go:732`, `:2199` | (ii) | Mới: sinh từ `verdicts` của task |
| A30 | Verdict của task bị hủy không tồn tại; gate bị hủy = `rejected`; task `work` bị hủy qua mặt tệp (nghi vấn H3) | `flow.go:455-488`, `flow_guards.go:274` | (iii) | Mới (D5) |

### A.4 Vòng sửa, điểm dừng, ra khỏi workspace

| # | Giả định | Bằng chứng | Loại | Có sẵn / mới |
| --- | --- | --- | --- | --- |
| A31 | **Một** bộ đếm vòng cho cả request: `FlowState.Rework`; vòng của task = `RoundBase + Rework` | `flow.go:279-301`, `flow.sql` `SetIssueFlow` (điều kiện `rework`), `flow_engine.go:306-355` | (i) | Mới: bộ đếm theo `loop`, số lần vào bước theo `activation` |
| A32 | Giới hạn vòng đọc từ điều kiện `rework_count lt N`, 1 ≤ N ≤ 5; N cố định lúc soạn | `flow.go:44` (`MaxReworkLimit`), `:779-790`, `:820-` | (ii) | Mới: `loop.max` số hoặc `{var, cap}` (phòng nghiên cứu lấy giới hạn từ dữ liệu) |
| A33 | `RoundBase` là mẹo để `resume` mở task mới không đụng chỉ mục duy nhất | `flow.go:283-301`, `flow_engine_ops.go:224-300` (`:267`), `:411` | (i) | Biến mất khi vòng = `activation` |
| A34 | Mã dừng là ba giá trị cố định `1`, `3`, `stalled`; suy từ dữ kiện lúc route, không khai báo trên cạnh | `flow.go:240-277`, `:424-435` (`route`) | (i) | Mới: `stops[]` khai báo; cạnh mang `stop` |
| A35 | Stop 2 = đích là **đúng** `in_review`; các bước chỉ-người khác dùng thông điệp `human` | `flow_engine.go:356-375` | (i) | Mới: `step.notice` + `terminals.leaves_workspace` |
| A36 | Thông điệp dừng có đúng năm khóa; biến cố định `{requester} {request} {step} {rework} {tasks} {missing} {reason}` | `flow.go:109-118`, `flow_engine.go:377-413` | (ii) | Mới: `messages{}` theo khóa tự do |
| A37 | Đỗ lại luôn ở trạng thái chữ `blocked`; `resume` không về được `blocked`/`done`/`cancelled` | `flow_engine.go:316`, `:768-781`; `flow_engine_ops.go:241`, `:395`; `flow.go:409` | (ii) | Mới: `stop.status` |
| A38 | Không có khái niệm "rời workspace": engine không biết đường nào đi ra ngoài; luật "phải có người" là quy ước của catalog | không có mã; `kitviet-workflow-spec.md` mục 4.1 | (iii) | Mới: `terminals[].leaves_workspace` + điểm kiểm (checkpoint) |
| A39 | Người dùng kéo thẻ sang `done` đóng request không cần bước người nào (ngoài tay người) | `flow_engine_ops.go:376-390` | (iii) | Giữ: người là điểm kiểm; ghi nhật ký |

### A.5 Biến, biểu thức, sự kiện

| # | Giả định | Bằng chứng | Loại | Có sẵn / mới |
| --- | --- | --- | --- | --- |
| A40 | Công tắc chỉ đặt ở bước plan; lead đặt, thành viên quản lý đặt bất kỳ lúc nào (có nhật ký) | `flow_engine_ops.go:98-155` (`PlanFlow`), `:169-223` (`SetFlowSwitch`) | (ii) | Mới: `var.set_by`, `var.set_at` |
| A41 | Công tắc kiểu `bool`/`enum`; ≤ 8; enum ≤ 16 giá trị | `flow.go:99-107`, `:203-223`, `:42`, `:45` | (ii) | Mới: thêm `int`, `string`, `list<string>` |
| A42 | Bảng dữ kiện của biểu thức cố định: `outcome`, `priority`, `labels`, `project_id`, `rework_count`, `creator_type`, `assignee_type`, `tasks_outcome` | `transitions.go:97-109`, `:489-513` | (i) | Mới: **sổ dữ kiện** (`FactRegistry`) theo định nghĩa |
| A43 | Engine chỉ cung cấp 3 dữ kiện cho flow; trường khác so với giá trị rỗng, và `not` bọc quanh luôn đúng | `flow.go:227-238`, `:387-391`; `transitions.go:208-231` | (iii) | Mới: validator từ chối trường không có (xem spec V-F01) |
| A44 | ≤ 10 cạnh mỗi bước, độ sâu điều kiện ≤ 4, nhãn ≤ 64 ký tự | `transitions.go:66-68` | (ii) | Giữ làm trần |
| A45 | Trường lạ trong điều kiện: `evaluateField` trả `false` im lặng | `transitions.go:235-247` | (iii) | Thay bằng lỗi lúc ghi |

### A.6 Thực thi, trạng thái, vòng đời

| # | Giả định | Bằng chứng | Loại | Có sẵn / mới |
| --- | --- | --- | --- | --- |
| A46 | Chống ghi đồng thời bằng điều kiện `flow->>'step'` và `rework` | `flow.sql` (`SetIssueFlow`), `flow_engine.go:230-252` | (i) | Mới: `seq` (số thứ tự ghi) trong `issue.flow`; có sẵn `LockIssueForFlow` |
| A47 | Định nghĩa đọc **sống** từ hàng workflow, sửa tại chỗ đổi request đang chạy | `flow_engine.go:61-83`, `flow_guards.go:66-84` | (iii) | Mới: phiên bản bất biến (mục 8) |
| A48 | Engine chỉ chạy cho workflow mà `Definition.Flow` khác nil, tức cột `flow_config` khác NULL và không phải `null` (`ParseFlowConfig` `flow.go:131-134`, `Decode` `issueworkflow.go:229-239`, `HasFlow` `:202`) | `flow_engine.go:61-83` (`def.Flow == nil` là `errFlowNotManaged`), `service/flow_start.go:42-46`, `squad_workflow.go:312-318` | (iii) | Giữ làm ranh giới với SDLC/BMAD |
| A49 | Không có trần số issue task cho một request; với `instances` và vòng lồng nhau cần trần | không có mã | (iii) | Mới: `policy.max_task_issues` |
| A50 | Cờ `FF_FLOW_ENGINE` tắt thì mọi guard nhường chỗ | `flow_guards.go:141`, `:147`, `:215`, `:237` | (iii) | Giữ làm công tắc khẩn cấp, ghi nhật ký quản trị |
| A51 | SDLC/BMAD chạy ở mức **issue**, không dùng engine này; giới hạn trả về là hằng `reworkReturnLimit = 3` | `issue_workflow_entry.go:156-200` (hằng ở `:160`) | (iii) | **Không đụng** |
| A52 | Catalog: manifest `flow` là schema 1; `rev` = băm của khối `flow`; đổi định dạng đổi `rev` và sinh hàng workflow mới cho mọi squad | `scripts/catalog/lib/build-workflows.mjs`; `kitviet-workflow-spec.md` mục 8 | (ii) | Giữ manifest schema 1 (xem B.12, D11) |

### A.7 Giao diện, CLI, tương thích

| # | Giả định | Bằng chứng | Loại | Có sẵn / mới |
| --- | --- | --- | --- | --- |
| A53 | Trình thiết kế lưu làm mất `tasks`/`human_only`; chỉ đọc với flow | `packages/views/workflow/workflow-data.ts:14-37`, `designer/workflow-designer-page.tsx:129-132` | (i) | Trình soạn mới ở thư mục mới |
| A54 | Bộ xây điều kiện dùng danh sách trường cứng | `packages/views/workflow/condition-spec.ts:20-28`, `:30-50` | (i) | Có sẵn cây AND/OR/NOT; mới: nhận sổ dữ kiện làm tham số |
| A55 | Client web biết `stop` chỉ là `1|3|"stalled"` (giá trị lạ → `stalled`) | `flow.go:262-277` (Go), zod của `IssueFlowState` | (ii) | Mới: `stop` là mã chuỗi + `legacy_no` |
| A56 | Phản hồi workflow cho client cũ mô tả bước = trạng thái | `issue_workflow.go:98-105` | (iii) | Mới: chiếu schema 2 xuống `steps[]` kiểu cũ |

## Phụ lục B. Mô hình tổng quát: khái niệm và lý do

Một nguyên tắc dẫn đường: **định nghĩa chỉ là dữ liệu có kiểu; engine là một bộ diễn giải nhỏ, cố định, của dữ liệu đó; mọi thứ engine tin đến từ trạng thái máy chủ.** Kit Việt chỉ là một tài liệu trong mô hình, không là hình dạng của mô hình.

Phụ lục này giữ lập luận của bản đầu. Ba chỗ đã được rà soát lần 2 sửa, và ở đó các mục 2 đến 5 mới là bản đúng: tách vai (R07, E13 thay cho "không chung với bất kỳ task `work` nào"), hợp `all` khi mọi phiếu `neutral` (4.3), và điều kiện rời workspace (4.8 thay cho "`checkpoints` không rỗng" ở B.7). Các trường `inputs`, `sets`, `skills` của task được định nghĩa ở 2.3, 4.11, 4.12.

Tên chung: **FlowDoc** (schema 2). Hai dạng: *tài liệu soạn* (bản nháp, vai chưa gắn) và *tài liệu biên dịch* (phiên bản đã xuất bản, vai đã gắn). Cùng một cấu trúc; khác nhau chỗ `roles[].bind`.

### B.1 Vai (`Role`)

```
Role { key, label, kind: "agent" | "member" | "lead" | "requester" | "managers",
       bind?: { agent_ids: [uuid] } | { member_ids: [uuid] },       // chỉ có ở dạng biên dịch
       pool?: n }                                                    // số người/agent tối thiểu yêu cầu
```

- **Khóa ổn định** `^[a-z][a-z0-9_]{0,31}$`; không phải UUID. Biên dịch lúc xuất bản: kiểm UUID tồn tại, chưa lưu trữ, thuộc workspace, **agent là thành viên của squad và không phải lead**, người là thành viên workspace; sau đó phiên bản bất biến.
- **Số lượng**: một hoặc nhiều. `bind.agent_ids` có thể có nhiều phần tử (hồ bơi). Task có `instances` lấy các thực thể phân biệt từ hồ bơi (xem B.5). `pool` là số tối thiểu cần để xuất bản.
- **Loại người đảm nhiệm**: `agent` (UUID agent, chạy bằng lượt chạy), `member` (UUID người, task giao cho người), `lead` (dựng sẵn, lấy **lúc chạy** từ `squad.leader_id`, nên đổi lead không cần phiên bản mới), `requester` (dựng sẵn, người tạo request, nếu là người), `managers` (dựng sẵn, bất kỳ ai thỏa `canManageSquad`; task không gán ai, ai có quyền đều đóng được).
- **Quan hệ với squad**: `agent` phải là thành viên agent của squad (kiểm lúc xuất bản và lại lúc mở task, như bước U4 của plan trước). `lead` không nhận task `work`/`gate` trừ khi `policy.lead_may_work` (mặc định tắt; bật cần admin xác nhận).

### B.2 Bước và cạnh (`Step`, `Edge`)

```
Step { key, title, status: {key}|{name,category}, kind: "tasks"|"lead"|"manual",
       tasks[], join, edges[], notice?: messageKey, exit: "auto"|"manual", layout? }
Edge { id, label, when?: Expr, to: stepKey|terminalKey, loop?: loopKey,
       stop?: stopCode, trigger: "auto"|"manual" }
```

- **Biểu diễn**: *đồ thị có hướng*, được kiểm theo quy tắc **DAG cộng cạnh quay lại có giới hạn**. Cạnh có `loop` là cạnh quay lại; bỏ hết cạnh quay lại thì đồ thị phải không còn chu trình. Cho phép chu trình tùy ý, nhưng mọi chu trình đều đi qua ít nhất một cạnh quay lại, nên mọi chu trình đều bị chặn bởi `loop.max`. Không cần "cấu trúc khối lồng nhau" (như BPMN có cấu trúc) nên vẫn quay về **bước cụ thể** (rà soát pháp lý), và vòng lồng nhau vẫn được (phòng nghiên cứu).
- **Khóa bước**, không đánh số theo vị trí. `status` chỉ là chỗ hiển thị trên bảng: nhiều bước được phép dùng chung một trạng thái; khi đó thao tác kéo thẻ thủ công vào trạng thái đó mơ hồ và engine yêu cầu chọn bước (`resume --to-step`), không đoán.
- **`kind`**: `tasks` (mở sub-issue), `lead` (lead hành động trên chính request, như bước plan hôm nay), `manual` (đỗ lại, người chọn cạnh ra).
- **Cạnh**: có thứ tự, **cạnh đầu khớp thắng** (giữ ngữ nghĩa hiện tại, `transitions.go` mục "first match wins"), cạnh cuối bước `auto` không có `when` (mặc định). Cạnh có `stop` không có `to`: đi theo nó là đỗ request với mã dừng đó và request vẫn "ở bước nguồn" để `resume` biết quay lại đâu. Cạnh quay lại chỉ đủ điều kiện khi `loop.count < loop.max` (bộ bảo vệ ngầm), nếu không thì engine bỏ qua và xét cạnh kế tiếp, thường là cạnh `stop`.
- **Kiểm đồ thị** (chi tiết spec 6): mọi đích tồn tại; mọi bước đến được từ `start`; không bước chết (bước `auto` không cạnh mặc định, hoặc không có đường tới đích kết thúc/đỗ); bỏ cạnh quay lại thì không chu trình; mỗi cạnh quay lại trỏ đúng `loop.header`; mỗi vòng có ít nhất một lối vào xuôi; hệ quả chặn của tích các `max` được tính và so với trần chi phí.

### B.3 Task (`Task`)

```
Task { key, title, kind: "work"|"gate", role, instances?, after?: [key], when?: Expr,
       verdicts?: setKey, requires?: { files: [FileReq], verdict: bool }, inputs?, deadline?, instructions }
```

- **`kind`** chỉ có hai: `work` (làm ra sản phẩm) và `gate` (phán, chịu luật độc lập). "**Người**" không phải loại task thứ ba: là **vai có `kind: member`** hoặc `managers`; một task người có thể là `work` (nhập liệu, ký) hay `gate` (người duyệt). "**Tùy chọn**" không phải loại: là thuộc tính `when` trên bất kỳ task nào (công tắc bật task). Cách chia này giữ trực giao: *ai làm* (vai) tách khỏi *làm gì* (kind) và *khi nào* (`when`).
- **Loại nào ở bước nào**: bước `tasks`: mọi task. Bước `manual`: task tùy ý nhưng bước **không tự rời** khi task xong (chỉ cạnh `trigger: manual`). Bước `lead`: không có task (lead hành động trên request). Task `gate` không dùng vai `lead` (đã có quy tắc).
- `after`: phụ thuộc **trong bước** (DAG con), `instances`: nhiều thực thể (B.5).

### B.4 Cổng và hợp đồng đóng (`Verdict`, `Close`)

```
VerdictSet { key, verdicts: [ { key, class: "pass"|"fail"|"neutral", label } ] }   // "blocked" luôn có, class "block"
Task.requires.files = [ { slot, min, max?, ext?: [".md"], content_types?: [] } ]
```

- **Bộ verdict theo từng gate**, không cố định bốn. Hai bộ dựng sẵn giữ nguyên hành vi: `work = {done:pass}` và `gate = {approved:pass, rejected:fail}`. Ví dụ tùy chỉnh: `audit = {pass:pass, pass_with_conditions:pass, request_changes:fail}`, `review = {approve:pass, reject:fail, abstain:neutral}`. `blocked` (class `block`, "không làm tiếp được") luôn hợp lệ cho mọi task, như hôm nay (`flow_guards.go:263-273`).
- **Class** là thứ engine hiểu: `pass`, `fail`, `neutral`, `block`. Nhãn verdict chỉ để người đọc và để đặt điều kiện cạnh cụ thể (`task.audit.verdict eq pass_with_conditions`).
- **Tệp yêu cầu**: loại theo phần mở rộng và/hoặc `content_type` (cột `attachment.filename`, `content_type` có sẵn, `models.go:231-233`), số tối thiểu/tối đa, và nhãn khe (`slot`) để task sau tham chiếu làm đầu vào. Engine **không đọc nội dung tệp**.
- **Ai đóng**: *người thực hiện* (agent được giao hoặc người được giao hoặc, với vai `managers`, bất kỳ quản lý), và **luôn** thêm `canManageSquad` đóng thay (D4) với nhật ký bắt buộc. Không còn đường khác.
- **Quan hệ verdict ↔ cạnh**: verdict **không** nhảy thẳng vào cạnh. Verdict của các task trong bước đi qua `join` thành dữ kiện (`join.result`, đếm theo class, `task.<key>.verdict`), cạnh đặt điều kiện trên dữ kiện. Một cổng người duyệt có ba lối ra (xuất bản, sửa bản nháp, tìm lại nguồn) chỉ là ba cạnh với `task.approval.verdict eq …`.

### B.5 Hợp song song (`join`) và thực thể (`instances`)

```
join { over: "gates"|"all"|[taskKey], mode: "all"|"any"|"quorum", k?: n, block: "immediate"|"at_close" }
instances { count: n } | { from_var: key, max: n }   // + distinct: true (mặc định)
```

- **all** (mặc định, đúng như hôm nay): chờ mọi task đã mở đóng; có `block` thì thoát sớm với `join.result = block`.
- **any**: task đầu tiên có class `pass` hoàn tất bước; engine hủy các task còn mở.
- **quorum k trong n**: hoàn tất `pass` khi `pass ≥ k`; hoàn tất `fail` khi `pass + open + chưa_mở < k` (không thể đạt nữa); `neutral` (bỏ phiếu trắng) đóng nhưng không tính vào `pass`.
- **Bước sau chờ bước song song** như thế nào: không cần cơ chế riêng. Có đúng một token; "bước sau" chỉ chạy khi `join` của bước hiện tại hoàn tất và một cạnh được chọn. Song song là **trong bước**; nối tiếp là **giữa bước**.
- **`instances`**: `count` cố định hoặc một task mỗi phần tử của biến `list<string>` (do lead đặt, trần `max`). Thực thể i có định danh `key~i`; mỗi thực thể nhận một phần tử riêng (`{{item}}` trong mô tả, được trích dẫn là dữ liệu). Với vai có hồ bơi, các thực thể lấy người/agent khác nhau (`distinct`); hồ bơi nhỏ hơn số thực thể là lỗi lúc xuất bản (với `count`) hoặc lúc mở (với `from_var`, khi đó request đỗ `stalled` với lý do `pool_too_small`).

### B.6 Vòng sửa (`Loop`)

```
Loop { key, header: stepKey, max: n | { var, cap }, reset: "on_entry"(mặc định) | "never" }
```

- Bộ đếm **theo vòng** (`issue.flow.loops[key]`), không theo request. Tăng khi engine đi theo cạnh quay lại gắn vòng đó. Đặt lại về 0 khi request đi vào `header` bằng cạnh xuôi (lối vào mới của vòng), nên vòng trong được đặt lại mỗi lần vòng ngoài lặp.
- **Giới hạn**: số cố định, hoặc lấy từ biến có **trần** (`cap` ≤ trần máy chủ): phòng nghiên cứu cho lead đặt `max_rounds` nhưng tuyệt đối ≤ 5.
- **Khi chạm giới hạn**: cạnh quay lại bị bỏ qua; cạnh sau nó (thường mang `stop: loop_exhausted`) được chọn. Không đường nào lặp vô hạn: cạnh quay lại là cạnh duy nhất tạo chu trình và nó có đếm.
- `activation`: mỗi lần vào một bước, `issue.flow.activations[step]` tăng 1; số này (không phải bộ đếm vòng) là `flow_round` của task và làm khóa duy nhất, nên `resume` không cần mẹo `RoundBase`.

### B.7 Điểm dừng, thông báo, đích và "rời workspace"

```
StopKind { code, class: "missing_grounds"|"loop_exhausted"|"stalled"|"custom", label, message: msgKey,
           status?: statusKey (mặc định "blocked"), resume_by: "managers" }
Terminal { key, status: statusKey (nhóm done|closed), leaves_workspace: bool }
Step.notice: msgKey      // bình luận hệ thống khi vào bước (hôm nay: dừng 2 và `human`)
```

- **Mã dừng khai báo** (`code` chuỗi); số `1`, `2`, `3` chỉ còn là nhãn cũ (`legacy_no`). `stalled` luôn có sẵn và do engine đặt (không chọn được cạnh, bước biến mất, quá hạn, quá trần). Mã dừng gắn vào **cạnh** (`edge.stop`), không suy từ dữ kiện lúc chạy. "Cần người duyệt trước khi rời workspace" là `Step.notice` trên bước `manual`, không phải trạng thái đỗ.
- **Engine biết một đường rời workspace** vì `Terminal.leaves_workspace = true` (mặc định `true` cho `done`). Một đường hợp lệ là đường đi từ `start` tới terminal đó có đi qua **điểm kiểm người**: bước `manual` (người chọn cạnh ra), hoặc task có vai `member`/`managers` và verdict class `pass` được yêu cầu. Máy chủ tính điều này bằng phân tích đồ thị (bỏ các bước điểm kiểm; nếu terminal còn đến được thì vi phạm). Lúc chạy, vào một terminal `leaves_workspace` yêu cầu `issue.flow.checkpoints` không rỗng cho chính đường này (đai an toàn khi người thao tác thủ công).

### B.8 Biến, sổ dữ kiện và biểu thức

**Chọn: tái dùng biểu thức JSON hiện có (cây `all/any/not/field-op-value`), mở rộng bằng sổ dữ kiện có kiểu; không viết ngôn ngữ mới.**

Lý do: đã có bộ đánh giá và bộ kiểm (`transitions.go:208-731`), có dữ liệu thật trong 21 flow, có bộ xây điều kiện phía client (`designer/condition-builder.tsx`, `condition-spec.ts`), không có phép lặp/gọi hàm nên không thể chạy vô hạn hay đọc dữ liệu ngoài, độ sâu và số cạnh đã bị chặn (`transitions.go:66-68`), và nó là AST nên xuất/nhập/mô phỏng/ánh xạ giao diện đều đơn giản. Một DSL mới buộc phải có trình phân tích, thông báo lỗi 7 ngôn ngữ, bộ soạn mới, mà không thêm sức biểu đạt cần cho các ví dụ. Mở rộng cần có: sổ dữ kiện động (thay `fieldSpecs` cố định, `transitions.go:489-513`), kiểu `int`/`string`/`list`, toán tử `contains`.

**Sổ dữ kiện**: mỗi mục `{key, type, source, scope}`. Nguồn **chỉ** được là:

| Nguồn | Ví dụ | Ai ghi |
| --- | --- | --- |
| `engine` | `join.result`, `join.pass`, `join.fail`, `join.block`, `join.open`, `task.<key>.verdict`, `task.<key>.pass`, `loop.<key>.count`, `step.<key>.activations`, `flow.activations`, `lead.verdict` | Engine, từ trạng thái máy chủ (task, verdict, bộ đếm) |
| `request` | `request.priority`, `request.labels`, `request.creator_type`, `request.project_id` | Hàng `issue` của request |
| `var` | `var.route`, `var.max_rounds`, `var.sources` | Biến khai báo, ghi qua API bởi người được phép (`set_by`, `set_at`), có nhật ký, kiểm kiểu |

Quy tắc: không có nguồn `comment`, `description`, `agent_output`. Verdict của task là cột `issue.outcome` do máy chủ ghi sau khi `checkFlowTaskStatus` chấp nhận; văn bản `VERDICT:` trong comment không bao giờ là dữ kiện. Dữ kiện không có trong sổ là **lỗi lúc ghi** (thay cho `false` im lặng ở A45).

### B.9 Một token, hợp song song nhiều bước

Mô hình giữ **một token**. Hai thứ ngoài mô hình đợt này, ghi thẳng: (a) song song giữa nhiều **bước** (nhánh tách rồi nối lại, mỗi nhánh ở bước khác nhau cùng lúc), (b) task do **máy** thực hiện (kiểm tra bằng hàm máy chủ có tên). Cả hai có chỗ để mở rộng (B.10) nhưng không làm bây giờ; ví dụ cần chúng đã được xác định ở Phụ lục C và D12.

### B.10 Điểm mở rộng (có tên, bị từ chối bởi bộ kiểm hôm nay)

`performer.kind = "system"` (hàm máy chủ có tên làm task, cho phòng nghiên cứu: `research.gate`), `Step.kind = "subflow"`, `Step.kind = "fork"|"join"` (đa token), `Task.kind = "check"`. Schema 2 đặt chỗ nhưng `ValidateFlow` trả `flow_feature_unsupported`; một máy chủ cũ gặp khóa lạ cũng từ chối (nguyên tắc: không đoán nghĩa).

**Thứ tự ưu tiên khi cần nhiều nhánh nhiều bước chạy cùng lúc (D23): `subflow` trước, `fork`/`join` sau.** Một bước `subflow` mở một (hoặc, với `instances`, nhiều) request con, mỗi request con chạy một flow đã xuất bản với trạng thái `issue.flow` riêng, và bước cha hợp kết cục của chúng như hợp verdict. Lý do chọn hướng này: (1) đơn vị thực thi của engine đã là "một issue mang một token", nên request con dùng lại nguyên bộ quyết định, khóa dòng, quét hạn và màn hình hiện có; (2) mỗi nhánh có vòng sửa và điểm dừng riêng mà không cần nhiều token trong một trạng thái, thứ sẽ buộc viết lại so-sánh-đổi trên `seq`, hợp, và `resume`; (3) đây là cách các engine điều khiển bằng dữ liệu làm (Map của ASL chạy một máy trạng thái con cho mỗi phần tử; `SUB_WORKFLOW` của Conductor; Phụ lục F). Việc cần làm khi đến lúc: nới E8 cho engine tạo request con, thêm `terminals[].result: pass|fail` để cha đọc kết cục, và trần độ sâu lồng. Trong lúc chưa có, phần lớn nhu cầu "làm lại riêng nhánh hỏng" viết được bằng `when` trên `task.<key>.verdict` của bước gate (ví dụ ở Phụ lục F.2, mẫu 12).

### B.11 Người đóng thay và nhật ký

Chỉ `canManageSquad` (hoặc admin) đóng thay agent hay người được giao. Yêu cầu: dùng đúng API đóng task (kể cả `POST …/flow/close` mới), cùng luật verdict/tệp (tệp do **chính người đóng thay** tải lên task đó được tính cho khe yêu cầu), và ghi `activity_log` `flow_manual_close` kèm `{task, activation, verdict, files, on_behalf_of}`. Bảng task hiển thị huy hiệu "đóng thay" cùng người đóng. Không có đường đóng thay cho agent khác.

### B.12 Phiên bản schema và chuyển 21 flow

`flow_config.version`: 1 (hiện tại) hoặc 2. Hai nguyên tắc:

- **Trình đọc schema 1 là phần vĩnh viễn của hệ thống**, giống `schemaVersion` của catalog: hàm thuần `Lift(v1) → Graph`. Nó chạy lúc nạp hàng `issue_workflow` (không sửa hàng), cho mọi request đã ghim phiên bản cũ, cho manifest catalog schema 1, và cho tính năng **nhân bản Kit Việt** (D8) sinh tài liệu schema 2 sạch.
- **Bằng chứng không đổi quyết định runtime**: bài kiểm vi phân (so `Decide` cũ, giữ nguyên trong tệp test làm tiêu chuẩn, với `Decide(Lift(def))` trên toàn bộ không gian trạng thái của cả 21 flow) và các bài diễn tập DB cũ chạy lại không sửa. Chi tiết spec 9.

Cờ tương thích `compat: "v1"` của đồ thị nâng chỉ giữ ba thứ: vòng của task = `RoundBase + Rework`, đầu vào task theo vị trí, thông điệp dừng cũ. Chúng tồn tại vì có request đang chạy chứa hàng `flow_round` cũ; chúng mất khi hết request ghim hàng schema 1.

## Phụ lục C. Mô hình bao phủ các loại workflow

Ký hiệu rút gọn: `R:` vai, `V:` biến, `S:` bước (`tasks`, `lead`, `manual`), `→` cạnh, `↺L` cạnh quay lại vòng L, `⊘code` cạnh đỗ.

### C.1 Kit Việt (Trade): **map sạch, một-một**

`R:` lead, docs_reader, customs_research, crosscheck_drafter, docs_gate, customs_gate, handover_gate (agent). `V:` route enum, customs_lookup bool, export bool (set_by lead, set_at plan). `Loop fix: header draft, max 2`.

`S:plan(lead; sets V; verdict planned|blocked)` → collect[docs; customs? when var.customs_lookup] → draft[crosscheck; files attachment≥1] → gate[docs_gate, customs_gate; join all/gates] → handover[handover gate] → release(manual, notice stop2, task export?) → done(leaves_workspace).
Cạnh gate: `join.result=block ⊘grounds_missing`; `pass → handover`; `fail ∧ loop.fix.count<2 ↺fix → draft`; mặc định `⊘loop_exhausted`. Đúng từng cạnh của `flow_kv_trade` (đã đọc `catalog/workflows/flow_kv_trade/template.json`). Cả 21 flow có đúng 1 cạnh rework (đếm bằng script đọc 21 manifest, mỗi flow `rework = 1`), nên một vòng duy nhất thay cho bộ đếm chung là tương đương; 2 flow (Marketing, PR) có thêm bước chỉ-người `approval` giữa Collect và Draft = bước `manual` thứ hai với `notice: human`.

### C.2 Phòng nghiên cứu (Research Lab): **map được, hai chỗ cần tính năng mới**

Từ `catalog/skills/builtin/research-lab-squad-lead/SKILL.md`: mỗi vòng có Scout rồi Screener; cổng quyết định `refine_query|summarize|summarize_available|stop_no_new_queries`; `max_rounds ≤ 5` do lead đặt lúc `init`; sau đó Synthesizer, Auditor; Auditor `REQUEST CHANGES` quay lại Synthesizer **tối đa 2 lần**, hết thì dừng và không export.
`R:` lead, scout, screener, synthesizer, auditor, reviewer_a, reviewer_b (agent), requester. `V:` max_rounds int(1..5), `Loop rounds: header search, max {var max_rounds, cap 5}`, `Loop fixes: header synthesize, max 2`.
`S:plan(lead sets max_rounds,…)` → search[scout] → screen[screener] → decide(lead; verdicts refine_query:fail|summarize:pass|summarize_available:pass|stop_no_new_queries:pass) → (`task.decide.verdict=refine_query` ↺rounds → search) → synthesize → audit[auditor; verdicts pass|pass_with_conditions|request_changes] → (`request_changes` ↺fixes → synthesize; hết ⊘loop_exhausted) → review[reviewer_b + reviewer_a chéo: hai gate khác vai, join all] → approve(manual, requester) → done.
- Vòng đánh giá chéo giữa hai reviewer: hai gate hai vai trong một bước, hai chiều là hai bước nối bằng cạnh quay lại (reviewer_a duyệt sản phẩm của b rồi ngược lại), mỗi chiều một vòng đếm riêng.
- Vòng trong (`fixes`) nằm trong vòng ngoài (`rounds`)? Ở đây tách rời; nếu lồng thì quy tắc đặt lại theo lối vào xuôi xử lý.
- **Cần mới #1:** `max` lấy từ biến có trần: có trong mô hình (B.6).
- **Cần mới #2 (điểm mở rộng):** cổng quyết định do **công cụ** tính trên tệp trạng thái (`litscout` `state.go`; `kitviet-workflow-spec.md` mục 12.3). Trong mô hình hôm nay, bước `decide` do **lead** báo verdict (kèm tệp đầu ra bắt buộc); lead không thể nhảy ngoài các cạnh đã khai báo và bị chặn bởi `rounds.max`, nhưng verdict không do máy chủ tính. Đưa việc tính ấy vào máy chủ cần `performer: system` (B.10). **Không map sạch ở điểm này, đã nêu trong D12.** Sau rà soát lần 2 (D17) bước `decide` không cần là bước của lead: nó là một task thường của một vai agent, báo verdict trong bộ bốn giá trị trên, bắt buộc kèm tệp trạng thái, và có thể đặt biến có kiểu (ví dụ số nguồn mới tìm được) để cạnh định tuyến theo số liệu. Verdict vẫn do agent báo chứ không do máy chủ tính; phần đó vẫn cần `performer: system`.

### C.3 Nghiên cứu thị trường (Market Research): **map sạch với `instances`**

Từ `catalog/squads/market_research/instructions.md` và `template.json`: hai vai song song (scout thị trường, competitor), rồi critic, rồi 0–3 proposal; request mở từ routine hàng tuần.
`R:` lead, collector (hồ bơi ≥ 3 agent), verifier, critic, synthesizer (agent), editor (member managers). `V:` sources list<string> (≤ 8), depth enum. `Loop recheck: header collect, max 2`.
`S:plan(sets sources)` → collect[`collect_source`: instances from_var sources, files report≥1, role collector distinct] → verify[`source_check`: instances (after collect), gate, verdicts verified:pass|unverified:fail, bắt buộc: join all] → (`join.result=fail ∧ recheck<2 ↺recheck → collect`) → synthesize[work; files report] → critique[critic gate] → edit(manual, role editor, notice human_review) → done(leaves_workspace).
"Kiểm chứng nguồn **bắt buộc**" = không có cạnh nào từ `verify` tới `synthesize` trừ `join.result = pass`. Số proposal (0–3) là việc ngoài engine (proposal là thực thể khác); request kết thúc ở bước `edit`. Map sạch, cần `instances`, `after`, hồ bơi vai.

### C.4 Rà soát pháp lý / nội dung: **map sạch với `quorum` và cạnh quay lại tới bước cụ thể**

`R:` lead, drafter (agent), reviewers (member, hồ bơi 5 luật sư, hoặc agent), counsel (member managers). `V:` severity enum(minor, major, critical).
`S:plan` → draft[drafter] → review[`reviewer`: instances count 5 distinct; verdicts approve:pass|reject:fail|abstain:neutral; join quorum k=3] → cạnh: `join.result=pass → counsel_signoff`; `join.result=fail ∧ var.severity=critical ↺cut_back → intake`; `join.result=fail ↺rework → redraft_clauses` (bước cụ thể, **không về đầu**; hai vòng khác nhau, mỗi vòng có max); mặc định ⊘loop_exhausted → counsel_signoff(manual, leaves_workspace) → done.
Thoát sớm: khi `reject` đủ nhiều để `pass + open + chưa_mở < 3`, bước hoàn tất `fail` ngay, hủy các thực thể còn mở. Map sạch; cần `quorum`, `instances`, vai `member`, nhiều vòng.

### C.5 SDLC và BMAD: **khớp về hình, cố ý không chuyển**

SDLC Auto/BMAD Planning là workflow **cấp issue**: mỗi trạng thái là một bước giao *chính issue đó* cho một handler, chuyển theo `outcome`, giới hạn trả về `reworkReturnLimit = 3` khi review/QA trả về `in_progress` (`issue_workflow_entry.go:156-200` (hằng ở `:160`)), và dùng `squad.workflow` (`catalog/workflows/sdlc_auto/template.json`). Viết dưới mô hình: mỗi trạng thái = một bước một task "inline" của handler, review→in_progress = cạnh quay lại vòng `rework` max 3, quá giới hạn ⊘blocked giao lead. **Khớp về hình.** Nhưng chế độ thực thi khác (giao lại cùng issue, không mở sub-issue; lượt chạy của handler dùng chính issue; thành viên kéo thẻ là đường chính) nên chúng **không** chuyển sang engine request-flow: ranh giới là `Definition.Flow != nil` (A48), `reworkReturnLimit` và `applyIssueWorkflow` không đổi, test vàng `issue_workflow_golden_test.go` là cổng. Một chế độ `inline` có thể thêm sau như `Step.exec = "inline"`; không có trong đợt này và không cần cho bất kỳ ví dụ nào ngoài chính SDLC.

### C.6 Bảng tổng

| Ví dụ | Kết quả | Tính năng mô hình cần | Còn thiếu |
| --- | --- | --- | --- |
| Kit Việt (21 flow) | Sạch, một-một | vai, join all, vòng, stop khai báo, bước manual | không |
| Phòng nghiên cứu | Map được, một điểm không sạch | `loop.max` theo biến, nhiều verdict, cổng lead | **hàm máy tính quyết định** (`system`), D12 |
| Nghiên cứu thị trường | Sạch | `instances` từ danh sách, `after`, hồ bơi vai | không |
| Rà soát pháp lý/nội dung | Sạch | `quorum`, vai `member`, cạnh quay lại theo bước, nhiều vòng | không |
| SDLC, BMAD | Khớp hình, không chuyển có chủ ý | — | chế độ `inline` (không cần) |

**Không map được:** nhánh tách/nối nhiều token cùng lúc ở nhiều bước (BPMN parallel gateway), chờ sự kiện bên ngoài (webhook), request con sinh động. Không ví dụ nào trong danh sách cần chúng. Nếu cần, thêm `fork/join` (B.10); không đổi lõi hiện tại vì phần còn lại của mô hình không giả định một token ngoài `issue.flow.step`.

## Phụ lục D. So sánh các phương án cho lõi

| | (i) Giữ máy trạng thái cố định + thêm tham số | (ii) Đồ thị khai báo + kiểm bằng quy tắc + bộ đếm theo vòng | (iii) Ngôn ngữ luồng nhỏ (DSL / BPMN con) | (iv) Engine ngoài (Temporal) |
| --- | --- | --- | --- | --- |
| Ý tưởng | Nới `MaxFlowSteps`, thêm tham số vào `FlowConfig`, thêm trường | Bước/cạnh/vòng/vai/verdict là dữ liệu; một bộ diễn giải | Văn bản chương trình luồng, có phân tích cú pháp | Quy trình là mã Go/TS chạy trên cụm Temporal |
| Ưu | Rẻ nhất, ít đụng 21 flow | Dữ liệu có kiểu, kiểm tĩnh được, mô phỏng được, soạn bằng giao diện, tái dùng AST điều kiện, lõi nhỏ | Biểu đạt mạnh, đọc/ghi bằng văn bản | Hẹn giờ, thử lại, lịch sử, bền vững có sẵn |
| Nhược | Không giải A1, A2, A4–A7, A16–A19, A31: bước = trạng thái, một bộ đếm, tất cả task mở cùng lúc đều là *cấu trúc*; thêm tham số chỉ nới số | Lõi phải viết lại (ước lượng 1.500 đến 1.800 dòng Go quyết định và thực thi: `flow.go` quyết định/kiểm ~600 dòng, `flow_engine.go` 794, `flow_engine_ops.go` 432), cần bộ kiểm đồ thị | Phải xây trình phân tích, lỗi 7 ngôn ngữ, bộ soạn cho văn bản, bảo vệ chống chương trình xấu; UI soạn khó hơn dữ liệu | Hai nguồn sự thật (Postgres issue và Temporal), mọi ghi trạng thái phải đồng bộ với `issue` và bảng/hoạt động; gánh nặng tự lưu trữ |
| Rủi ro | Mỗi loại workflow mới lại nới thêm, nợ tăng; vẫn nhiều chỗ phải sửa tay | Rủi ro hồi quy 21 flow (giảm bằng bài kiểm vi phân và bằng diễn tập cũ) | Bề mặt tấn công và lỗi cú pháp, chi phí dịch | Vận hành: cụm riêng cho mỗi cài đặt tự lưu trữ (`docker-compose.selfhost.yml` hiện chỉ có Postgres, backend, frontend, SeaweedFS và nhóm knowledge, dòng 36, 55, 269, 306, 331-498); chuẩn hóa tất định, quản lý phiên bản mã; không có `temporal` trong `server/go.mod` hay `package.json` (đã `grep`) |
| Chi phí (ước lượng, ngày công) | 10–15 | 75–95 cho lõi và trình soạn theo bản trước; sau rà soát lần 2 kế hoạch cộng 134 gồm cả pha F (xem kế hoạch mục 7). Phân rã của bản trước (cộng 88: vá 7; đóng băng, mô hình, `Lift`, `Decide`, bộ kiểm 22; tích hợp và API 21; bản nháp và xuất bản 9; client và giao diện 26; còn lại 3) | 110–150 | 120–180 + hạ tầng |
| Ảnh hưởng 21 flow | Gần như không, nhưng không giải được vấn đề | Qua `Lift`, hành vi giữ nguyên, chứng minh bằng vi phân | Phải chuyển lại cả 21 sang cú pháp mới | Phải viết lại cả 21 thành mã workflow |

**Khuyến nghị: (ii).** Lý do đủ chuẩn sản xuất:

1. *Kiểm tĩnh và mô phỏng được.* Dữ liệu có kiểu cho phép kiểm đồ thị (đạt được, không bước chết, mọi chu trình có chặn, tích các `max` ≤ trần), tính chi phí trường hợp xấu nhất và chạy mô phỏng bằng chính bộ quyết định thuần (`Decide` đã thuần, `flow.go:352`).
2. *Giữ một nguồn sự thật.* Mọi thay đổi trạng thái vẫn là giao dịch Postgres có khóa dòng (`LockIssueForFlow`, `flow.sql`) cộng so sánh-và-đổi trên `seq`; không thêm hạ tầng; kill switch và runner hiện tại dùng lại.
3. *Không quá rộng.* Mô hình chỉ có: vai, bước ba loại, cạnh có điều kiện, task hai loại, ba kiểu hợp, vòng có đếm, mã dừng và đích khai báo, dữ kiện từ ba nguồn. Không có phép lặp, hàm, hay đọc ngoài. Mỗi phần tử xuất hiện vì ít nhất một ví dụ ở Phụ lục C cần nó (bảng C.6).
4. *Không quá hẹp.* Chỗ mở rộng có tên (B.10) và khóa lạ bị từ chối chứ không đoán. Thêm kiểu verdict, kiểu biến, tập vai không đổi lõi. Loại workflow mới là **dữ liệu**, không phải mã.
5. *Chuyển được an toàn.* `Lift` là hàm thuần; bằng chứng bằng so sánh với bản cũ giữ trong test; request đang chạy ghim hàng cũ không bị đụng.

Lý do bác từng phương án khác: (i) chỉ hoãn vấn đề: bước = trạng thái, một bộ đếm, mọi task mở cùng lúc là cấu trúc, thêm tham số không đổi chúng; (iii) tốn chi phí cho cú pháp mà không thêm sức biểu đạt cần cho các ví dụ, và biểu diễn dữ liệu của (ii) có thể xuất thành văn bản sau này nếu cần; (iv) gánh hạ tầng cho mọi cài đặt tự lưu trữ và tạo hai nguồn sự thật cho một thứ mà toàn bộ giao diện (bảng, bình luận, nhật ký, quyền) đã nằm ở Postgres. Chỉ nên xem lại (iv) nếu sau này cần hẹn giờ dài hạn xuyên hệ thống, hoặc hàng nghìn request đồng thời cần sổ lịch sử bền ngoài `issue`.

## Phụ lục E. Bó gốc `multica.squad-bundle/v1` (một trong bốn định dạng nhận ở 16.1)

Bó gốc là định dạng **đầy đủ nhất** (khai lead và kèm được `flow.json`), dành cho bộ được đóng gói cho Multica hoặc xuất ra từ một squad. Nó không phải điều kiện để nhập: bộ theo ba định dạng còn lại ở 16.1 nhập được nguyên trạng (D14). Đường cài, bảng và route: mục 16.2, 16.3.

### E.1 Một bó phải chứa gì (`multica.squad-bundle/v1`, zip)

```text
squad-bundle.json            {"format":"multica.squad-bundle/v1","id":"…","version":"1.0.0","name":"…",
                              "leader":"planner",
                              "agents":[{"key":"planner","name":"…","role_label":"…","description":"…",
                                         "instructions":"agents/planner/instructions.md",
                                         "skills":["skill-a","skill-b"],"max_concurrent_tasks":2}, …],
                              "flow":"flow.json"}                       # tùy chọn
agents/<key>/instructions.md
skills/<skill-name>/SKILL.md (+ tệp tham chiếu)
flow.json                    # FlowManifest: FlowDoc schema 2 với roles theo khóa agent, KHÔNG có bind
```

Ràng buộc: khóa agent `^[a-z][a-z0-9_]{1,31}$`; `leader` là khóa của đúng một agent; có ít nhất một agent khác lead; mọi tên skill trong `agents[].skills` tồn tại trong `skills/`; tên skill theo `^[a-z0-9][a-z0-9._-]*$` (cùng luật `packages/core/agent-packs/schema.ts:4`); `flow.json` chỉ nhắc các khóa agent không phải lead; giới hạn kích thước theo `hub.DefaultLimits` (512 tệp, 1 MiB mỗi tệp, 8 MiB tổng; `server/internal/hub/types.go:55-70`). Quét bí mật và hành vi nguy hiểm bằng `hub.Scan` (`server/internal/hub/scan.go:60-80`).

### E.2 Chọn vai

- Mỗi agent không phải lead trở thành **một vai**, khóa vai bằng khóa agent. Nhãn vai (`role_label`) ghi vào `squad_member.role` (cột văn bản tự do, `migrations/084_squad.up.sql:22`; quickstart làm tương tự ở `packages/core/squads/quickstart.ts:852`).
- Lead là `leader`; bó không có `leader` bị từ chối.
- `doc.roles` của bản nháp sinh từ bó lúc cài; `bind` điền sau khi các agent được tạo hoặc ghép với agent có sẵn do **người cài** chọn, không bao giờ dựa vào tên. Ghép lại với agent khác về sau: sửa `bind` trong bản nháp rồi xuất bản phiên bản mới.
- Đường cài là phía máy chủ: `POST /api/squads/packs/preview` và `/commit` (16.2, 16.3), dùng giải nén và `hub.Scan` của hub và hàm tạo skill `createHubSkill` (`server/internal/handler/hub_import.go:925`); bảng `squad_pack_install` (16.3).
- Bó có `leader` trống hoặc trỏ agent không có: với bó gốc là lỗi `pack_invalid`. (Ba định dạng còn lại không khai lead; người nhập chọn, 16.3.)

## Phụ lục F. Đối chiếu với engine mã nguồn mở và độ phủ mẫu luồng

### F.1 Các engine đã đối chiếu

| Engine | Định nghĩa là | Vòng lặp và chặn | Fan-out động | Người trong vòng | Lấy gì, không lấy gì |
| --- | --- | --- | --- | --- | --- |
| Amazon States Language (Step Functions) | JSON; engine diễn giải | Chu trình tự do qua `Next`; chặn bằng `MaxAttempts` của `Retry` và thời hạn | `Map`: một máy trạng thái con cho mỗi phần tử của một mảng trong dữ liệu; `MaxConcurrency` | Task chờ token gọi lại | **Lấy:** `Choice` là cây `And/Or/Not` trên so sánh có kiểu, có nhánh `Default` (đúng hình cây điều kiện của ta, và quy tắc "cạnh cuối là mặc định"); `Catch` là cạnh khai báo cho lúc hỏng (cạnh `stop` của ta). **Không lấy:** JSONPath/JSONata trên dữ liệu tự do (ta dùng sổ dữ kiện đóng) |
| Conductor (conductor-oss) | JSON; engine diễn giải | `DO_WHILE` với `loopCondition` là JavaScript; tài liệu yêu cầu luôn tự thêm trần lặp | `FORK_JOIN_DYNAMIC` lấy danh sách nhánh từ đầu ra của task trước; tài liệu khuyên kiểm và giới hạn khi danh sách do agent sinh | Task `HUMAN` | **Lấy:** fan-out từ đầu ra của một task (D17: `Task.sets` + `instances.from_var`, có `max`); `SUB_WORKFLOW` làm hướng mở rộng (D23). **Không lấy:** biểu thức là mã nhúng |
| LangGraph | Mã Python/JS; đồ thị nút và cạnh có điều kiện | Chu trình tự do; một trần `recursion_limit` cho cả lần chạy, chạm thì ném lỗi | `Send`: chạy một nút mỗi phần tử, biết số lượng lúc chạy | `interrupt()` + checkpointer; tiếp tục bằng `Command(resume=…)` | **Lấy:** ghi trạng thái ở biên từng bước và yêu cầu chạy lại phải an toàn (đã có: `seq`, chỉ mục duy nhất của task, I9); lịch sử từng bước để gỡ lỗi (`trail`, `flow_transition`). **Không lấy:** định nghĩa là mã (không kiểm tĩnh, không soạn bằng giao diện) |
| BPMN 2.0 / Camunda | XML; engine diễn giải | Chu trình qua cổng; không bắt chặn | Multi-instance (số lượng hoặc tập hợp), điều kiện hoàn tất sớm | User task, phân vai | **Lấy:** multi-instance có điều kiện hoàn tất (`quorum`, `any`), tách "ai làm" khỏi "làm gì". **Không lấy:** cổng song song nhiều token, sự kiện, bù trừ |
| Temporal | Mã Go/TS, phát lại tất định | Tự do trong mã | Trong mã | Signal | Không lấy (Phụ lục D, phương án iv) |
| Chuẩn Agent Skills | Thư mục có `SKILL.md` (frontmatter `name`, `description` bắt buộc; `license`, `compatibility`, `metadata`, `allowed-tools` tùy chọn; khóa lạ bỏ qua) | — | — | — | **Lấy:** làm định dạng nhận số 4 và quy tắc đọc ở 16.1. Chuẩn chỉ định nghĩa một skill; nó không có khái niệm agent, vai hay quy trình, nên phần đó là của ta |

Ba điểm mô hình này chặt hơn cả năm engine trên, và là chủ ý: (1) mọi chu trình có bộ đếm khai báo và được kiểm tĩnh trước khi chạy; (2) biểu thức chỉ đọc sổ dữ kiện đóng, có kiểu, từ ba nguồn; (3) người thực hiện không tự đi sang bước khác: nó chỉ báo verdict và biến đã khai, máy chủ chọn cạnh.

### F.2 Độ phủ theo danh mục mẫu luồng (Workflow Patterns của van der Aalst và cộng sự)

"Có" nghĩa là viết được bằng FlowDoc schema 2 mà không sửa mã; mỗi dòng "có" có một tệp trong `server/internal/issueworkflow/testdata/flows/patterns/` (K23).

| # | Mẫu | Có? | Viết bằng |
| --- | --- | --- | --- |
| 1 | Tuần tự | Có | bước nối bước; `after` trong bước |
| 2 | Tách song song và đồng bộ (trong một bước) | Có | nhiều task một bước, `join all` |
| 3 | Chọn loại trừ và gộp đơn | Có | cạnh có điều kiện, cạnh mặc định; nhiều cạnh cùng đích |
| 4 | Chọn nhiều nhánh | Có | `when` trên từng task |
| 5 | Nhiều thực thể, biết số lúc soạn | Có | `instances.count` |
| 6 | Nhiều thực thể, biết số lúc chạy | Có | `instances.from_var`, danh sách do lead hoặc một task đặt |
| 7 | Bộ phân biệt (một cái xong là đi) | Có | `join any` |
| 8 | N trong M | Có | `join quorum` |
| 9 | Chu trình tùy ý có chặn, quay về bước cụ thể, vòng lồng | Có | cạnh quay lại + `loops` |
| 10 | Chọn trì hoãn (người chọn lối ra) | Có | bước `manual` với nhiều cạnh `manual` |
| 11 | Hủy một việc, hủy cả ca | Có | quản lý hủy task; terminal `cancelled`, `detach` |
| 12 | Làm lại riêng nhánh hỏng | Có | bước làm việc có hai task `fe`, `be`; bước cổng có `fe_review`, `be_review`; cạnh quay lại về bước làm việc; `when` của `fe`: `not (task.fe_review.verdict eq approved)`, tương tự cho `be` (lần đầu dữ kiện chưa có nên `not` đúng và cả hai đều mở); `when` của `fe_review`: `task.fe.pass gte 1` để cổng chỉ phán nhánh vừa làm lại |
| 13 | Nhiều nhánh nhiều bước chạy cùng lúc, mỗi nhánh có vòng riêng | **Không** | cần `subflow` (D23) |
| 14 | Chờ sự kiện ngoài, hẹn giờ | **Không** | ngoài phạm vi (13) |
| 15 | Thêm thực thể khi bước đang chạy | Một phần | task có `when` mở thêm khi biến đổi (4.2a); danh sách `instances` thì bị đóng băng (E24) |
| 16 | Quy trình con dùng lại | **Không** | cần `subflow` (D23) |

### F.3 Nguồn đã đọc (2026-11-13)

- Amazon States Language: `https://states-language.net/spec.html` (Choice, Map, Retry, Catch).
- Conductor: `https://conductor-oss.github.io/conductor/documentation/configuration/workflowdef/operators/do-while-task.html`, `https://conductor-oss.github.io/conductor/architecture/json-native.html` (DO_WHILE, FORK_JOIN_DYNAMIC, HUMAN, khuyến cáo giới hạn nhánh do agent sinh).
- LangGraph: `https://docs.langchain.com/oss/python/langgraph/graph-api` và `…/graph-api-control-flow`, `…/checkpointers` (cạnh có điều kiện, `Send`, `recursion_limit`, checkpoint theo bước, yêu cầu idempotent).
- Agent Skills: `https://agentskills.io/specification` (frontmatter, cấu trúc thư mục, khóa lạ).
- BPMN, Temporal và danh mục Workflow Patterns: từ hiểu biết chung, không tra lại ở đợt này.
