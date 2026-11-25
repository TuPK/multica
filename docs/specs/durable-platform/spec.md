# Delegora bền vững: đặc tả

Trạng thái: đặc tả, chưa triển khai. Chiến lược: [strategy.md](strategy.md). Kế hoạch: [plan.md](plan.md).
Ngày rà soát: 2026-11-25, trên HEAD `21cbcdd90`, cây làm việc sạch.
Ngôn ngữ: tiếng Việt. Định danh, đường dẫn, lệnh, tên bảng và tên trường giữ nguyên tiếng Anh.
Quy ước: đường dẫn kèm "(mới)" là đề xuất, chưa tồn tại. Đường dẫn còn lại đã được đọc hoặc `ls` lúc rà soát. Số migration là chỗ giữ: số lớn nhất lúc viết là `1040`; đánh số thật theo quy tắc của [AGENTS.md](../../../AGENTS.md#database-and-migration-rules) ngay trước khi đẩy. Mục 15 liệt kê những gì chưa kiểm chứng.
Nền: [digital-company](../../engineering/digital-company.md). Đặc tả này dùng lại nguyên các khái niệm ở đó: squad, Secretary, proposal, brief, run class (`human`, `routine`, `delegated`), budget gate, autonomy L0 đến L3, standing approval, governance log, scorecard, review, probation, retrospective, company export. Khi nói "Phase 4" là nói [mục Phase 4](../../engineering/digital-company.md#phase-4-governance) của tài liệu đó.

## Mục lục

0. Quyết định đã chốt
1. Khái niệm chung
2. F1 Outcome Record
3. F2 Track Record và Autonomy Envelope
4. F3 Effect Registry
5. F4 Rehearse & Preview
6. F5 Principal Profile
7. F6 Attention Budget
8. F7 Disagreement Jury
9. F8 Seat Dossier và Shadow Run
10. F9 Scaffolding Sunset
11. F10 Reach Map
12. F11 Open Company Format
13. F12 Agent Labor Market
14. Yêu cầu chung (quyền, API, giao diện, bảo mật, hiệu năng, tài liệu)
15. Chưa kiểm chứng
16. Ngoài phạm vi
17. Kiểm thử và nghiệm thu tổng

## 0. Quyết định đã chốt

Người dùng giao toàn quyền thiết kế. Các quyết định dưới đây do kỹ sư thiết kế chọn, lý do ở mục được dẫn. Theo Automation First, mỗi chỗ mặc định là một giả định có nhãn, sửa được bằng cài đặt mà không đổi code.

| # | Quyết định | Mục |
| --- | --- | --- |
| D1 | Chương trình nằm sau năm cờ tính năng mặc định tắt, mỗi giai đoạn một cờ: `durable_outcomes` (F1, F2), `durable_effects` (F3, F4), `durable_principal` (F5, F6), `durable_seats` (F7, F8, F9), `durable_open` (F10, F11, F12). Tắt cờ thì hành vi Phase 4 giữ nguyên từng byte | 14.1 |
| D2 | "Loại việc" (`work_class`) là một chuỗi suy ra được, không phải danh mục do người nhập: `flow:<flow_id>:<stage_key>`, rồi `label:<label_name>` nếu issue có nhãn bắt đầu bằng `wc:`, rồi `squad:<squad_id>`, rồi `general`. Thứ tự này là thứ tự ưu tiên | 1.2 |
| D3 | Lịch sử chuyển trạng thái issue được ghi thật (bảng `issue_status_transition`). Đây là sửa lỗi nền của Phase 4: không có thời điểm xong, không có lịch sử mở lại | 2.2 |
| D4 | Một việc xong có đúng một `work_outcome` hiện hành. Làm lại thì tạo bản mới và bản cũ trỏ tới bản mới (`superseded_by`). Bản ghi không bao giờ bị sửa nội dung kết luận sau khi đã chốt (`settled`) | 2.3 |
| D5 | Kết luận có năm giá trị: `pass`, `partial`, `fail`, `unverified`, `pending`. `unverified` là trung thực, không phải lỗi: việc không kiểm được thì nói là không kiểm được | 2.3 |
| D6 | Cửa sổ chốt mặc định 7 ngày. Mở lại trong cửa sổ thì kết luận thành `fail` với nguyên nhân `reopened`, dù trước đó là `pass` | 2.4 |
| D7 | Uy tín dùng cận dưới Wilson (z = 1.96) trên tỷ lệ `pass`, không dùng tỷ lệ thô, để mẫu nhỏ không được thưởng | 3.2 |
| D8 | Ba chế độ kiểm chứng theo uy tín: `full` (kiểm 100%), `sampled` (30%), `spot` (10%, tối thiểu một lần mỗi tuần). Hạ về `full` là tức thì khi có `fail` mức `high` | 3.3 |
| D9 | Autonomy Envelope: người chủ đặt sàn và trần cho mức tự chủ của squad. Máy di chuyển trong khoảng đó theo bằng chứng. Đặt khoảng là quyết định của người; di chuyển trong khoảng không cần hỏi lại. Không có envelope thì hành vi là của Phase 4 (chỉ người đổi) | 3.4 |
| D10 | Effect Registry mở rộng lớp hành động của `connectedapps` (`read`, `write`, `send`, `delete`, `share`) thêm `write_internal` và `spend`, và thêm trục thứ hai là khả năng đảo ngược (`reversible`, `compensable`, `irreversible`). Lớp cũ giữ nguyên tên và ý nghĩa | 4.2 |
| D11 | Hành động mà runtime làm bên ngoài nền tảng (shell, git push, gọi API bằng khoá riêng) không cưỡng chế được. Agent khai báo trước bằng `multica effect declare`. Giao diện nói rõ đây là khai báo, không phải kiểm soát | 4.5 |
| D12 | Rehearse: với hành động được duyệt qua bản nháp, nền tảng tự thực thi đúng lệnh đã duyệt (ràng buộc bằng băm), không trả lại cho agent tự gọi | 5.3 |
| D13 | Principal Profile mặc định tắt cho từng người, bật bằng sự đồng ý của chính người đó. Chỉ người đó, Secretary của workspace và (khi người đó cho phép) owner khác đọc được. Không bao giờ dùng trong scorecard hay review | 6.6 |
| D14 | Uỷ quyền bằng dự đoán chỉ bật theo từng loại quyết định, khi độ chính xác ≥ 85% trên ≥ 20 quyết định gần nhất, và người chủ bật. Không bao giờ áp cho `org_change`, cổng Launch, hành động `irreversible` hay `spend` | 6.5 |
| D15 | Đơn vị của Attention Budget là phút. Mặc định 60 phút mỗi người quyết định mỗi tuần, chia đều cho các squad đang hoạt động. Hết ngân sách thì câu hỏi chờ brief kế tiếp, không bị bỏ. Câu hỏi khẩn vẫn giữ luật một lần mỗi tuần của Phase 1 | 7.3 |
| D16 | Jury chọn người kiểm từ các runtime khác `provider` với người làm khi có thể. Không đủ provider khác thì dùng model khác cùng provider, và ghi `diversity = model_only` | 8.2 |
| D17 | Ghế (seat) là agent hiện có. Không có bảng ghế riêng. Đổi runtime hoặc model của agent là "đổi người ngồi ghế", lịch sử uy tín ở lại với agent và được chia đoạn theo model | 9.1 |
| D18 | Shadow run chỉ được hành động `read`. Mọi hành động khác đi qua nền tảng bị chặn cứng. Hành động bên trong runtime chạy trong thư mục làm việc tạm, và task bóng không được claim worktree hay đẩy mã | 9.4 |
| D19 | Đổi model của ghế sau shadow run là `org_change` kiểu mới `runtime_change`. Mặc định là proposal cho người chủ. Người chủ có thể bật "tự nâng cấp" cho một ghế: khi đó thắng có ý nghĩa thống kê thì tự đổi và vào probation 14 ngày, `worse` thì tự đổi lại | 9.6 |
| D20 | Scaffolding Sunset dùng thử nghiệm không kém hơn (non-inferiority) với biên 5 điểm phần trăm và tối thiểu 20 cặp. Kết quả là proposal `skill_change` hoặc ghi chú cho người viết flow; không tự gỡ gì | 10.3 |
| D21 | Reach Map tính lại hằng đêm từ dữ liệu có sẵn (Effect Registry, Attention ledger, connected apps). Không có bước người nhập bản đồ | 11.2 |
| D22 | Open Company Format là JSON Schema có version (`ocf/1`), đặt trong repo này và xuất bản như tài liệu mở. Export hiện tại được nâng lên thành `ocf/1`, import đọc cả bản cũ | 12.2 |
| D23 | Attestation của track record được ký bằng khoá Ed25519 của máy chủ. Khoá công khai xuất bản ở `/.well-known/multica-attestation-keys.json`. V1 không có thanh toán hay ký quỹ giữa các tổ chức: mọi việc tiêu tiền ra ngoài là quyết định của người | 13.4 |
| D24 | Thêm hai run class: `verification` (task kiểm chứng, task của Jury) và `shadow` (task bóng). Cả hai bị budget giữ như `routine`, và bị dừng khi công ty `paused` | 1.4 |
| D25 | Mọi mã Go mới nằm trong file `*_fork.go`; route ở `server/cmd/server/router_fork.go`; mã TypeScript mới ở thư mục mới `packages/core/durable/` và `packages/views/durable/`. File upstream chỉ nhận hook một dòng, mỗi hook được ghi vào `pnpm sync:contract` | 14.2 |
| D26 | Ghi chuyển trạng thái issue bằng **trigger PostgreSQL** `AFTER UPDATE OF status` (chỉ khi giá trị đổi; cột `issue.status` là khoá TEXT, không có `status_id`), không hook từng đường. Actor đến từ biến phiên `multica.actor_type`, `multica.actor_id`, `multica.task_id` do ứng dụng đặt bằng `SET LOCAL` trong cùng transaction (`WithActorSession`); không đặt thì `system`. Test tĩnh đối chiếu 24 điểm ghi với việc đã đặt biến phiên (P0 U1) | 2.2, 15 |
| D27 | Shadow dùng **agent bóng**: agent ẩn (`agent.shadow_of_agent_id`) cùng vai và hồ sơ ghế, còn runtime và model là ứng viên (P0 U4). Shadow không chạy ở chế độ `in_place`, và runtime có quyền push bị từ chối (P0 U5) | 9.4, 15 |
| D28 | Khoá attestation đọc từ `MULTICA_ATTESTATION_KEY`. Thiếu khoá thì **tắt tính năng phát hành** (503 `attestation_disabled`); xác minh và khoá công khai vẫn chạy. Không sinh khoá tự động, không lưu khoá trong DB. Tên biến không mang tên thương hiệu, theo quy tắc nhãn hiệu của repo (P0 U10 đề xuất `MULTICA_DELEGORA_ATTEST_KEY`, bị bỏ vì lý do này) | 13.2, 15 |
| D29 | Worker của F11 là **adapter CLI tham chiếu** chạy như runtime tuỳ chỉnh hiện có. Không xây backend HTTP worker trong v1 (P0 U9) | 12.3, 15 |
| D30 | Shadow và danh sách skill bị bỏ được gate bằng capability `DaemonCapabilityShadowV1` trong header `X-Client-Capabilities`. Daemon không khai báo capability thì không nhận task bóng (P0 U6) | 9.4, 14.4, 15 |
| D31 | Không có sự kiện revert PR: `fail_cause = reverted` chỉ đến từ `done → cancelled` (P0 U8). Backfill từ `activity_log` chỉ best-effort (P0 U2), đánh dấu cột `source = 'activity_backfill'`; dữ liệu trước khi bật vẫn `history_partial = true` | 2.2, 2.4, 15 |
| D32 | Trigger ghi chuyển trạng thái chỉ đọc bảng của chính nó và các cột của `issue`. Trigger không tham chiếu bảng tạo sau (`issue_status`), vì migration số 4 chữ số sort trước migration tạo bảng đó. Bảng lưu khoá (`from_status_key`, `to_status_key`), không lưu nhóm trạng thái. Nhóm (`unstarted`, `started`, `done`, `closed`) suy ra khi đọc: join `issue_status`, và hàm Go `LifecycleCategoryOf` cho khoá built-in chưa có dòng catalog | 2.2, 15 |

## 1. Khái niệm chung

### 1.1 Từ vựng

| Từ | Nghĩa |
| --- | --- |
| **Việc** | Một issue đi tới nhóm trạng thái `done`. Đơn vị kết quả của chương trình |
| **Người làm** | Agent được gán issue lúc nó vào `done`, hoặc agent của task hoàn tất gần nhất trên issue nếu issue gán cho squad |
| **Kết quả** (`work_outcome`) | Bản ghi kết luận về một việc: đạt hay không, ai kiểm, bằng chứng gì |
| **Tiêu chí** (`acceptance_criterion`) | Một điều kiện kiểm được gắn với issue |
| **Loại việc** (`work_class`) | Chuỗi phân nhóm việc để tính uy tín (D2) |
| **Uy tín** (`track_record`) | Thống kê kết quả theo cặp (agent, loại việc) trong một cửa sổ thời gian |
| **Hệ quả** (`effect`) | Một hành động có tác động, đi qua nền tảng hoặc được khai báo |
| **Người quyết định** (principal) | Thành viên có vai owner hoặc admin, là "the boss" của Phase 4 |
| **Câu hỏi** (ask) | Bất cứ thứ gì làm người quyết định phải dừng lại: proposal, xác nhận ghi, bản nháp hành động, inquiry, kiểm chứng thủ công |

### 1.2 Loại việc

`work_class` được tính một lần khi kết quả được tạo và lưu lại; đổi nhãn sau đó không đổi lịch sử.

```
nếu issue có flow_id và flow_stage_key       → "flow:<flow_id>:<stage_key>"
nếu issue có nhãn tên bắt đầu bằng "wc:"     → "label:<phần sau wc:>"   (nhãn đầu tiên theo tên)
nếu issue gán cho squad hoặc người làm có home squad → "squad:<squad_id>"
còn lại                                       → "general"
```

Giới hạn: `char_length(work_class) <= 200`. Nhãn `wc:` là cách duy nhất người dùng đặt loại việc bằng tay; không có màn hình quản lý danh mục loại việc. Lý do: danh mục do người nhập sẽ không ai giữ, còn chuỗi suy ra thì luôn đúng với dữ liệu.

### 1.3 Diễn viên hệ thống mới

Theo mẫu `policy:<id>` của Phase 4, governance log nhận các `actor_type` mới:

| `actor_type` | Khi nào |
| --- | --- |
| `track_record` | Máy đổi chế độ kiểm chứng hoặc mức tự chủ trong envelope |
| `principal` | Quyết định được áp bằng uỷ quyền dự đoán (`actor_id` là thành viên có profile) |
| `jury` | Kết luận do Jury đưa ra |
| `shadow` | Tự nâng cấp model của ghế |

Thêm các giá trị này vào `governanceActorTypes` trong `server/internal/handler/governance.go` bằng một hook một dòng gọi danh sách của file fork.

### 1.4 Run class mới

| Class | Tạo bởi | Bị budget giữ | Bị `paused` giữ | Ghi chú |
| --- | --- | --- | --- | --- |
| `verification` | F1 kiểm chứng bằng agent, F7 Jury | có | có | Không bao giờ được kiểm chứng tiếp (không đệ quy) |
| `shadow` | F8, F9 | có | có | Chỉ `read` (D18); không tạo `work_outcome` |

`RunClass` trong `server/internal/service/budget*.go` nhận hai giá trị mới. `BudgetGate.Admit` hiện chỉ xét `routine` và `delegated`; sửa thành tập hợp lấy từ một hàm fork `heldRunClasses()` để hai class mới cũng bị giữ. Mặc định `budget.classes` của bản ghi mới thành `{routine,delegated,verification,shadow}`; bản ghi cũ được migration thêm hai giá trị nếu có `routine`.

## 2. F1 Outcome Record

### 2.1 Mục tiêu

Mỗi việc xong có một kết luận đã kiểm chứng (hoặc trung thực là chưa kiểm được), có bằng chứng, có người kiểm khác người làm. Đây là dữ liệu gốc cho F2, F7, F8, F9 và cho scorecard.

**Phép thử 10x:** mô hình giỏi hơn thì làm được việc lớn hơn, và việc lớn hơn càng cần được kiểm. Giá trị tăng.

### 2.2 Lịch sử chuyển trạng thái

```sql
CREATE TABLE issue_status_transition (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    issue_id UUID NOT NULL,
    from_status_key TEXT,               -- khoá lúc trước (issue.status), NULL khi tạo issue
    to_status_key TEXT NOT NULL,        -- khoá mới (issue.status)
    actor_type TEXT NOT NULL,           -- member | agent | system
    actor_id UUID,
    task_id UUID,                       -- task đang chạy của agent nếu agent đổi
    source TEXT NOT NULL DEFAULT 'live' CHECK (source IN ('live', 'activity_backfill')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- index (workspace_id, issue_id, created_at)        CONCURRENTLY, migration 1042
-- index (workspace_id, to_status_key, created_at)   CONCURRENTLY, migration 1043
-- Không có cột nhóm: nhóm được suy ra khi đọc (D32).
```

- Ghi bằng **trigger PostgreSQL** `AFTER UPDATE OF status ON issue` (migration 1044, khi giá trị đổi, D26), nên mọi đường ghi, kể cả SQL trực tiếp của sweeper, để lại đúng một dòng. Trigger chỉ đọc `OLD.status`, `NEW.status` và biến phiên (D32). Actor lấy từ biến phiên đặt bằng `WithActorSession` ở đầu mỗi transaction ghi issue của ứng dụng; không đặt thì `actor_type = system`. Hai bài test: `TestStatusTriggerCatchesRawSQL` (UPDATE thô vẫn có dòng) và `TestEveryWriterSetsActorSession` (test tĩnh: mỗi trong 24 điểm ghi, theo P0 U1, đã gọi `WithActorSession`).
- Nhóm trạng thái (`unstarted`, `started`, `done`, `closed`) của mỗi khoá: dòng catalog trong `issue_status` quyết định; không có dòng thì khoá built-in map theo bảng migration 469 (`LifecycleCategoryOf`). Mọi đọc lịch sử đi qua hàm này, không đọc khoá thô.
- **Mở lại** là chuyển từ nhóm `done` sang nhóm khác `done` và khác `closed` (`cancelled`).
- Backfill từ `activity_log` chỉ best-effort, vì dữ liệu `status_changed` có khoảng trống (P0 U2, D31): dòng backfill có `source = 'activity_backfill'`. Dữ liệu trước khi bật vẫn được scorecard đánh dấu `history_partial = true` trong `metrics`.
- Sửa hai dòng lệch của Phase 4: `delivery.cycle_time_days` (từ lần đầu vào `started` tới lần vào `done`) và `delivery.rework_rate` (số lần mở lại / số lần vào `done`) giờ tính được và được thêm vào job metric nội tại. Proxy `issue.updated_at` trong scorecard được thay bằng thời điểm vào `done`.

### 2.3 Tiêu chí và kết quả

```sql
CREATE TABLE acceptance_criterion (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    issue_id UUID NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    text TEXT NOT NULL CHECK (char_length(text) BETWEEN 1 AND 500),
    check_kind TEXT NOT NULL CHECK (check_kind IN
        ('manual', 'agent_judgment', 'command', 'artifact', 'metric')),
    check_spec JSONB NOT NULL DEFAULT '{}',
    source TEXT NOT NULL CHECK (source IN ('member', 'agent', 'flow', 'template')),
    created_by_type TEXT NOT NULL,
    created_by_id UUID,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);   -- index (issue_id, position) CONCURRENTLY

CREATE TABLE work_outcome (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    issue_id UUID NOT NULL,
    done_transition_id UUID NOT NULL,   -- issue_status_transition đã tạo kết quả này
    agent_id UUID,                      -- người làm; NULL khi người làm là thành viên
    member_id UUID,
    squad_id UUID,
    runtime_provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    work_class TEXT NOT NULL CHECK (char_length(work_class) <= 200),
    verification_mode TEXT NOT NULL CHECK (verification_mode IN ('full', 'sampled', 'spot')),
    sampled BOOLEAN NOT NULL,           -- false: không được chọn để kiểm, kết luận theo cửa sổ
    verdict TEXT NOT NULL DEFAULT 'pending'
        CHECK (verdict IN ('pending', 'pass', 'partial', 'fail', 'unverified')),
    severity TEXT NOT NULL DEFAULT 'none' CHECK (severity IN ('none', 'low', 'medium', 'high')),
    fail_cause TEXT NOT NULL DEFAULT ''
        CHECK (fail_cause IN ('', 'criteria', 'reopened', 'reverted', 'metric', 'jury', 'member')),
    criteria_results JSONB NOT NULL DEFAULT '[]',   -- [{criterion_id, result, verifier_ref, note}]
    evidence JSONB NOT NULL DEFAULT '[]',           -- [{type, ref, note}] type: run|comment|document|metric|pr|attachment|url
    cost_micros BIGINT NOT NULL DEFAULT 0,          -- chi phí các task của người làm trên issue
    verification_cost_micros BIGINT NOT NULL DEFAULT 0,
    settle_at TIMESTAMPTZ NOT NULL,
    settled_at TIMESTAMPTZ,
    superseded_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- unique (done_transition_id)                                CONCURRENTLY
-- index (workspace_id, agent_id, work_class, settled_at)     CONCURRENTLY
-- index (workspace_id, issue_id) WHERE superseded_by IS NULL CONCURRENTLY
-- index (workspace_id, verdict, settle_at) WHERE settled_at IS NULL CONCURRENTLY

CREATE TABLE outcome_verification (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    outcome_id UUID NOT NULL,
    criterion_id UUID,                  -- NULL: kết luận chung
    verifier_type TEXT NOT NULL CHECK (verifier_type IN ('system', 'agent', 'member', 'metric', 'jury')),
    verifier_id UUID,
    task_id UUID,                       -- task verification nếu verifier là agent
    result TEXT NOT NULL CHECK (result IN ('pass', 'partial', 'fail', 'unable')),
    severity TEXT NOT NULL DEFAULT 'none' CHECK (severity IN ('none', 'low', 'medium', 'high')),
    note TEXT NOT NULL DEFAULT '' CHECK (char_length(note) <= 4000),
    evidence JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);   -- index (outcome_id) CONCURRENTLY
```

Không có khoá ngoại; mọi quan hệ kiểm trong service. `outcome_verification` là append-only: không có câu `UPDATE` hay `DELETE` trong `pkg/db/queries`, một test DB khẳng định điều này như với `governance_event`.

### 2.4 Vòng đời của một kết quả

```
issue vào done ──► tạo work_outcome (pending, settle_at = now + settle_days)
        │                 │
        │                 ├─ sampled = true  → lên lịch kiểm từng tiêu chí (2.5)
        │                 └─ sampled = false → không kiểm chủ động
        │
        ├─ mở lại trong cửa sổ ─► verdict = fail, fail_cause = reopened; khi issue vào done lần nữa
        │                         thì tạo bản mới và bản cũ có superseded_by
        └─ tới settle_at ─► chốt: verdict theo bảng 2.6, settled_at = now
```

- `settle_days` là cài đặt workspace, mặc định 7, khoảng 1 đến 60.
- Issue vào `cancelled` từ `done`: kết quả hiện hành thành `fail` với `fail_cause = reverted`.
- Một issue cha có issue con: mỗi issue có kết quả riêng. Không gộp.
- Issue do thành viên làm (không agent): vẫn tạo kết quả với `member_id`, nhưng không tính vào uy tín agent. Lý do: rework rate và cycle time của cả công ty cần cả hai.
- Issue có nhãn `wc:skip` hoặc thuộc dự án được đánh dấu `outcomes_disabled`: không tạo kết quả. Đây là lối thoát cho việc nhiễu (nhắc việc, ghi chú).

### 2.5 Cách kiểm từng loại tiêu chí

| `check_kind` | `check_spec` | Ai kiểm | Khi nào |
| --- | --- | --- | --- |
| `manual` | `{}` | Thành viên (người tạo issue, hoặc người quyết định nếu người tạo là agent) | Câu hỏi trong Inbox, tốn ngân sách chú ý (F6) |
| `agent_judgment` | `{ "rubric": "..." }` (≤ 2000 ký tự) | Agent kiểm chứng khác người làm; với chế độ `full` và loại việc được đánh dấu quan trọng thì là Jury (F7) | Task `verification` ngay khi vào `done` |
| `command` | `{ "run": "pnpm test", "expect_exit": 0 }` | Agent kiểm chứng chạy lệnh trong worktree của PR và báo kết quả | Task `verification`; nền tảng không tự chạy lệnh |
| `artifact` | `{ "kind": "<knowledge kind>" }` | Hệ thống | Ngay: có tài liệu đã publish đúng kind gắn với issue chưa |
| `metric` | `{ "metric_key", "scope", "op": ">=", "value", "within_days" }` | Hệ thống | Hằng giờ cho tới `within_days`; quá hạn mà chưa đạt là `fail` với `fail_cause = metric` |

Tiêu chí đến từ đâu:

- **Người hoặc agent viết** trên issue (giao diện, `multica issue criteria add`). Agent viết thì `source = agent` và người làm không được kiểm chính tiêu chí của mình.
- **Flow**: mỗi `required artifact` của stage sinh một tiêu chí `artifact`. Đây là chỗ nối với `initiative_artifact` có sẵn.
- **Template**: catalog squad có thể khai báo `outcomeCriteria` mặc định theo vai (`catalog/squads/<id>/template.json`, trường mới, tuỳ chọn); áp khi squad nhận issue.
- **Không có tiêu chí nào**: khi được chọn kiểm, tạo một tiêu chí ngầm `agent_judgment` với rubric chung "Việc có giải quyết đúng yêu cầu trong mô tả và các comment của người không?". Không bao giờ để việc được chọn kiểm mà không có cách kiểm.

Agent kiểm chứng: cài đặt workspace `outcome_verifier_agent_id`; mặc định là agent reviewer của Phase 4 (`people_reviewer`) nếu có; nếu không có thì là Secretary. Người kiểm không bao giờ trùng agent với người làm; trùng thì chọn agent thứ hai trong danh sách `verifier_pool` (cài đặt), và không còn ai thì tiêu chí là `unable`.

### 2.6 Chốt kết luận

Tại `settle_at`, nếu chưa bị đánh `fail`:

| Tình trạng các tiêu chí (tiêu chí `unable` không tính) | Kết luận |
| --- | --- |
| `sampled = false` và không bị mở lại | `unverified` |
| không có tiêu chí nào có kết quả | `unverified` |
| mọi tiêu chí `pass` | `pass` |
| có `fail` ở tiêu chí bắt buộc (mọi tiêu chí là bắt buộc trong v1) | `fail`, `fail_cause = criteria` |
| còn lại (có `partial`) | `partial` |

`severity` của kết quả là mức cao nhất trong các lần kiểm. Người quyết định đổi được kết luận đã chốt trong 30 ngày, bằng một lần kiểm mới `verifier_type = member`; lần đổi đó là một governance event `outcome_overridden`. Đây là cách duy nhất đổi kết luận đã chốt (D4).

Tại sao `sampled = false` là `unverified` mà không phải `pass`: việc không bị mở lại chưa chắc đã đúng. Uy tín (F2) chỉ tính `pass`, `partial` và `fail`; `unverified` chỉ được đếm để biết độ phủ.

### 2.7 API

| Phương thức và đường dẫn | Ai | Việc |
| --- | --- | --- |
| `GET /api/outcomes?agent_id&squad_id&work_class&verdict&since&cursor` | thành viên | Danh sách, phân trang con trỏ, tối đa 100 |
| `GET /api/outcomes/{id}` | thành viên | Chi tiết kèm các lần kiểm |
| `GET /api/issues/{id}/outcome` | thành viên | Kết quả hiện hành của issue, 404 nếu chưa có |
| `GET /api/issues/{id}/criteria` | thành viên | Tiêu chí |
| `POST /api/issues/{id}/criteria` | thành viên, agent | Thêm tiêu chí; tối đa 20 tiêu chí mở mỗi issue |
| `PATCH /api/issues/{id}/criteria/{cid}` | người tạo, owner, admin | Sửa chữ hoặc archive; không sửa khi kết quả đang `pending` có lần kiểm |
| `POST /api/outcomes/{id}/verifications` | agent kiểm chứng được giao task, thành viên | Ghi một lần kiểm; agent chỉ ghi được cho task của chính nó |
| `POST /api/outcomes/{id}/override` | owner, admin | Đổi kết luận đã chốt (2.6) |
| `GET /api/outcomes/summary?group_by=agent|squad|work_class&period=week|month` | thành viên | Tổng hợp cho giao diện |

CLI (`server/cmd/multica/cmd_outcome_fork.go` (mới)): `multica issue criteria list|add|archive`, `multica outcome show <issue>`, `multica outcome verify --outcome <id> [--criterion <id>] --result pass|partial|fail|unable [--severity] [--note] [--evidence type:ref]`.

### 2.8 Phía agent

- Briefing của task `verification` có: mô tả issue, các comment của người, tiêu chí cần kiểm, các run của người làm (đường dẫn, không chép nội dung), và câu lệnh `multica outcome verify` phải gọi. Agent kiểm chứng không thấy kết luận của người kiểm khác (mù, để F7 dùng được).
- Briefing của task thường có thêm một khối "Tiêu chí nghiệm thu của issue này" khi issue có tiêu chí, để người làm biết mình sẽ bị kiểm theo gì.
- Kết quả `fail` và `partial` đã chốt có một comment tự động trên issue (do hệ thống), nhắc người làm, để agent học từ lỗi qua đường mention có sẵn. Không mở lại issue tự động.

### 2.9 Giao diện

- **Issue detail**: khối "Nghiệm thu" ở cột phải: danh sách tiêu chí với kết quả từng tiêu chí, chip kết luận (`pending` hiện thời gian còn lại tới khi chốt), nút "Thêm tiêu chí". Khi tắt cờ: không hiện.
- **Governance → Outcomes** (tab mới): bảng lọc theo agent, squad, loại việc, kết luận; biểu đồ tỷ lệ theo tuần; ô "độ phủ kiểm chứng" (`pass+partial+fail` / tổng).
- **Scorecard**: thêm `verified_pass_rate`, `verified_coverage`, `rework_rate`, `cycle_time_days`.

### 2.10 Nghiệm thu F1

1. Đổi trạng thái issue bằng mọi đường gọi (API, CLI, batch, flow, PR auto-complete) để lại đúng một `issue_status_transition`.
2. Issue vào `done` tạo đúng một `work_outcome` với `work_class` đúng thứ tự D2; issue nhãn `wc:skip` không tạo.
3. Mở lại trong cửa sổ cho `fail/reopened`; vào `done` lần nữa tạo bản mới, bản cũ có `superseded_by`.
4. Tiêu chí `artifact` qua khi tài liệu đúng kind được publish; `metric` qua khi reading đạt ngưỡng trong hạn, `fail` khi quá hạn.
5. Người làm không bao giờ được giao task kiểm chứng việc của chính mình (test thử cả đường cài đặt verifier trùng).
6. Kết luận đã chốt không đổi được qua API nào ngoài `override`; `override` để lại governance event.
7. Tắt `durable_outcomes`: không ghi gì vào bốn bảng mới, scorecard trả đúng các trường của Phase 4.

## 3. F2 Track Record và Autonomy Envelope

### 3.1 Mục tiêu

Biến kết quả đã kiểm chứng thành uy tín theo cặp (agent, loại việc), và dùng uy tín đó để (a) giảm việc kiểm chứng cho người làm tốt, (b) dời mức tự chủ của squad trong khoảng người chủ cho phép.

**Phép thử 10x:** mô hình giỏi hơn thì uy tín lên nhanh hơn, quyền nới nhanh hơn. Nền tảng hấp thụ tiến bộ thay vì bị nó thay thế. Giá trị tăng.

### 3.2 Bảng uy tín

```sql
CREATE TABLE track_record (
    workspace_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    work_class TEXT NOT NULL,
    window_days INTEGER NOT NULL CHECK (window_days IN (30, 90)),
    model TEXT NOT NULL DEFAULT '',     -- '' là gộp mọi model; còn lại là đoạn theo model (F8)
    n_pass INTEGER NOT NULL DEFAULT 0,
    n_partial INTEGER NOT NULL DEFAULT 0,
    n_fail INTEGER NOT NULL DEFAULT 0,
    n_unverified INTEGER NOT NULL DEFAULT 0,
    n_high_fail INTEGER NOT NULL DEFAULT 0,
    score_lower NUMERIC NOT NULL DEFAULT 0,   -- cận dưới Wilson của pass, partial tính 0.5
    last_high_fail_at TIMESTAMPTZ,
    verification_mode TEXT NOT NULL DEFAULT 'full' CHECK (verification_mode IN ('full', 'sampled', 'spot')),
    mode_pinned BOOLEAN NOT NULL DEFAULT false,  -- người chủ ghim chế độ
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);   -- unique (workspace_id, agent_id, work_class, window_days, model) CONCURRENTLY
```

Công thức, với `n = n_pass + n_partial + n_fail` và `p = (n_pass + 0.5·n_partial) / n`, `z = 1.96`:

```
score_lower = ( p + z²/(2n) − z·sqrt( p(1−p)/n + z²/(4n²) ) ) / ( 1 + z²/n )      (n = 0 thì 0)
```

Tính lại: job hằng đêm (`jobs_track_record_fork.go` (mới)) cho mọi cặp có kết quả chốt trong 90 ngày, và tính lại ngay cặp bị ảnh hưởng khi một kết quả chốt hoặc có `fail` mức `high`. Chỉ kết quả đã chốt được tính, trừ `fail` mức `high` được tính ngay khi có.

### 3.3 Chế độ kiểm chứng

Áp cho kết quả mới của cặp (agent, loại việc), đọc từ dòng `window_days = 30, model = ''`:

| Chế độ | Điều kiện để lên | Tỷ lệ chọn kiểm (`sampled = true`) |
| --- | --- | --- |
| `full` | mặc định | 100% |
| `sampled` | `n ≥ 10`, `score_lower ≥ 0.80`, không có `fail` mức `high` trong 30 ngày | 30% |
| `spot` | đang `sampled`, dòng 90 ngày có `n ≥ 30` và `score_lower ≥ 0.90`, không có `fail` mức `high` trong 90 ngày | 10%, và ít nhất một kết quả mỗi 7 ngày của cặp |

Xuống:

- Có `fail` mức `high`: về `full` ngay.
- `score_lower` dưới ngưỡng của chế độ hiện tại: xuống một bậc ở lần tính kế tiếp.
- Lên chỉ một bậc mỗi lần tính, và không quá một lần mỗi 7 ngày.

Chọn mẫu: tất định theo băm `sha256(outcome_id)`, để test lặp lại được và để agent không đoán trước được (agent không biết `outcome_id` trước khi việc xong).

Người chủ ghim được chế độ cho một cặp (`mode_pinned`), ví dụ ghim `full` cho việc pháp lý. Mọi lần đổi chế độ là governance event `verification_mode_changed` với `actor_type = track_record` hoặc `member`.

Loại việc mới của một agent luôn bắt đầu ở `full`. Agent đổi model (F8) giữ chế độ, vì uy tín thuộc về ghế, nhưng một `fail` mức `high` trong 14 ngày đầu sau khi đổi thì về `full` như mọi lần khác.

### 3.4 Autonomy Envelope

```sql
CREATE TABLE squad_autonomy_envelope (
    workspace_id UUID NOT NULL,
    squad_id UUID NOT NULL,
    floor_level INTEGER NOT NULL CHECK (floor_level BETWEEN 0 AND 3),
    ceiling_level INTEGER NOT NULL CHECK (ceiling_level BETWEEN 0 AND 3),
    set_by UUID NOT NULL,
    reason TEXT NOT NULL DEFAULT '' CHECK (char_length(reason) <= 500),
    last_auto_change_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (floor_level <= ceiling_level)
);   -- unique (workspace_id, squad_id) CONCURRENTLY
```

Uy tín của squad: gộp mọi kết quả đã chốt của các agent có home squad là squad đó, cửa sổ 90 ngày, cùng công thức Wilson. Squad cần `n ≥ 20` mới được xét.

| Đi từ | Tới | Điều kiện (mọi điều kiện) |
| --- | --- | --- |
| L0 | L1 | `score_lower ≥ 0.60` |
| L1 | L2 | `score_lower ≥ 0.80`, không có `fail` mức `high` trong 30 ngày, tỷ lệ proposal được chấp nhận (2.6 Phase 2) ≥ 60% |
| L2 | L3 | `score_lower ≥ 0.90`, không có `fail` mức `high` trong 90 ngày, ít nhất 60 ngày ở L2 |

- Lên: tối đa một bậc mỗi 14 ngày, chỉ tới `ceiling_level`.
- Xuống: một `fail` mức `high` của bất kỳ agent nào trong squad thì squad xuống một bậc ngay (không thấp hơn `floor_level`). `score_lower` dưới điều kiện của mức hiện tại thì xuống một bậc ở lần tính kế tiếp.
- Mỗi lần đổi ghi governance event `autonomy_auto_changed` với `details = {from, to, score_lower, n, window_days, trigger_outcome_id?}` và đi vào mục FYI của brief kế tiếp.
- Người chủ đổi tay vẫn được như Phase 4; đổi tay ra ngoài envelope thì envelope được nới theo, với lý do bắt buộc.
- **Điều khoản của spec cho chỗ dừng chờ người** (theo Automation First): đặt hoặc nới `ceiling_level` là quyết định của người vì nó trao quyền mới, cùng loại với "any change to autonomy" của Phase 4 4.4. Mọi di chuyển trong khoảng không chờ ai.
- Hai điều Phase 4 nói "never depend on level" vẫn giữ: Launch gate và `org_change` luôn là của người.
- Standing approval vẫn không được đổi autonomy (Phase 4 4.4). Envelope không phải standing approval: nó là cài đặt riêng, chỉ owner và admin viết, và agent không bao giờ viết.

### 3.5 API và giao diện

| Phương thức và đường dẫn | Ai | Việc |
| --- | --- | --- |
| `GET /api/track-records?agent_id&squad_id&work_class&window_days` | thành viên | Danh sách |
| `PUT /api/track-records/pin` | owner, admin | Body `{agent_id, work_class, mode | null}`; `null` bỏ ghim |
| `GET /api/squads/{id}/autonomy/envelope` | thành viên | Envelope và uy tín squad |
| `PUT /api/squads/{id}/autonomy/envelope` | owner, admin, người (không agent) | Đặt; `DELETE` để bỏ |

Giao diện: Governance → Autonomy thêm cột "Khoảng" (sàn–trần) và "Uy tín (90 ngày)" cho mỗi squad, với nút "Đặt khoảng". Trang agent thêm tab "Thành tích": bảng theo loại việc, chế độ kiểm chứng, nút ghim. Câu mô tả bắt buộc ở tab Autonomy: "Delegora chỉ kiểm soát những gì đi qua nền tảng" (giữ nguyên ý của Phase 4 4.3).

### 3.6 Nghiệm thu F2

1. Với dữ liệu giả 10 `pass` liên tiếp ở `full`, lần tính kế tiếp lên `sampled` (với `score_lower` đúng tới 4 chữ số), và không lên `spot` trước 7 ngày.
2. Một `fail` mức `high` đưa cặp về `full` và squad xuống một bậc trong cùng giao dịch với lần kiểm, không đợi job đêm.
3. Squad có envelope `[1, 2]` không bao giờ lên L3 và không xuống L0, dù dữ liệu cho phép.
4. Squad không có envelope không bao giờ đổi mức tự động.
5. Agent actor gọi `PUT .../envelope` nhận 403.
6. Mỗi lần đổi tự động có đúng một governance event và xuất hiện trong digest của brief kế tiếp.

## 4. F3 Effect Registry

### 4.1 Mục tiêu

Mọi hành động có tác động mà nền tảng nhìn thấy đều có nhãn hệ quả, một quyết định chính sách theo nhãn, một dòng nhật ký, và nếu có thể là một cách hoàn tác. Automation First (các điểm dừng "không đảo ngược được, tốn tiền, gửi ra ngoài") trở thành luật được máy cưỡng chế ở mọi kênh, không chỉ ở connected apps.

**Phép thử 10x:** kênh hành động sẽ đổi (CLI, MCP, computer-use, thứ chưa có tên), nhưng câu hỏi "hành động này có lấy lại được không, có tốn tiền không, có ra khỏi công ty không" thì không. Giá trị tăng.

### 4.2 Phân loại

Hai trục. Trục một mở rộng `connectedapps` (`server/internal/connectedapps/classify.go`):

| `effect_class` | Nghĩa | Ánh xạ từ lớp `connectedapps` |
| --- | --- | --- |
| `read` | Không đổi gì | `read` |
| `write_internal` | Đổi dữ liệu trong workspace Delegora (issue, comment, tài liệu, agent) | (mới) |
| `write` | Đổi dữ liệu ở hệ thống ngoài, không ai bên ngoài thấy ngay | `write` |
| `send` | Gửi tới người hoặc hệ thống ngoài (email, tin nhắn, bài đăng, webhook ra) | `send` |
| `share` | Mở quyền truy cập cho bên ngoài | `share` |
| `delete` | Xoá | `delete` |
| `spend` | Tốn tiền thật (mua, thanh toán, nạp, đặt quảng cáo) | (mới; chỉ từ danh sách curated hoặc khai báo) |

Trục hai: `reversibility`:

| Giá trị | Nghĩa | Mặc định cho |
| --- | --- | --- |
| `reversible` | Có thao tác ngược rõ ràng, nền tảng biết cách gọi | `write_internal`; `write` khi có cặp ngược trong bảng curated |
| `compensable` | Không lấy lại được nguyên trạng, nhưng có thao tác bù (gửi đính chính, hoàn tiền) | `write` mặc định |
| `irreversible` | Không lấy lại được | `send`, `share`, `delete`, `spend` |

Bảng curated (`server/internal/connectedapps/catalog.go`) thêm hai trường tuỳ chọn cho từng tool: `reversibility` và `undo` (tên tool ngược và cách ánh xạ tham số, ví dụ `GMAIL_CREATE_DRAFT` → `GMAIL_DELETE_DRAFT` với `{draft_id: result.id}`). Tool không curated dùng mặc định theo bảng trên.

### 4.3 Chính sách

Quyết định cho một hành động là một trong `allow`, `confirm`, `rehearse` (F4), `block`. Bảng mặc định trong code (`server/internal/service/effect_policy_fork.go` (mới)), theo mức tự chủ của squad của agent (agent không có squad dùng L1):

| `effect_class` / `reversibility` | L0 | L1 | L2 | L3 |
| --- | --- | --- | --- | --- |
| `read` | allow | allow | allow | allow |
| `write_internal` | allow | allow | allow | allow |
| `write` / reversible | confirm | allow | allow | allow |
| `write` / compensable | confirm | confirm | allow | allow |
| `send` | block | rehearse | rehearse | allow trong hạn mức, ngoài hạn mức rehearse |
| `share` | block | rehearse | rehearse | rehearse |
| `delete` | block | confirm | rehearse | rehearse |
| `spend` | block | rehearse | rehearse | rehearse |

- Quy tắc ghép với `connectedapps.Decide`: quyết định cuối là cái **chặt hơn** giữa bảng này, quy tắc của tài khoản (`app_action_rule`) và mức của agent cho app (`read`, `confirm_writes`, `write`). Thứ tự chặt dần: `allow` < `confirm` < `rehearse` < `block`. Hàm `Stricter` có sẵn được mở rộng cho `rehearse`.
- Workspace ghi đè được từng ô theo hướng **chặt hơn** ở bất kỳ mức nào; theo hướng **lỏng hơn** chỉ ở L3 và không bao giờ cho `spend` hay `share` thành `allow`. Lỏng hơn là quyết định của người (điều khoản: trao quyền mới), lưu ở `effect_policy_override` và ghi governance event.
- "Hạn mức" của `send` ở L3: `effect_send_daily_cap` (mặc định 20 hành động mỗi agent mỗi ngày).
- Task class `shadow`: mọi thứ trừ `read` là `block` (D18). Task class `verification`: mọi thứ trừ `read` và `write_internal` lên bình luận/kiểm chứng là `block`.
- Công ty `paused`: mọi thứ trừ `read` của task không phải `human` là `block`.

```sql
CREATE TABLE effect_policy_override (
    workspace_id UUID NOT NULL,
    autonomy_level INTEGER NOT NULL CHECK (autonomy_level BETWEEN 0 AND 3),
    effect_class TEXT NOT NULL,
    reversibility TEXT NOT NULL,
    decision TEXT NOT NULL CHECK (decision IN ('allow', 'confirm', 'rehearse', 'block')),
    set_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);   -- unique (workspace_id, autonomy_level, effect_class, reversibility) CONCURRENTLY
```

### 4.4 Nhật ký hệ quả

```sql
CREATE TABLE effect_event (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    task_id UUID,
    agent_id UUID,
    member_id UUID,
    source TEXT NOT NULL CHECK (source IN
        ('connected_app', 'plugin_action', 'cli', 'channel', 'webhook', 'publish_channel', 'declared')),
    target TEXT NOT NULL CHECK (char_length(target) <= 300),    -- toolkit/tool, kênh, URL host
    effect_class TEXT NOT NULL,
    reversibility TEXT NOT NULL,
    decision TEXT NOT NULL CHECK (decision IN ('allow', 'confirm', 'rehearse', 'block')),
    status TEXT NOT NULL CHECK (status IN ('executed', 'blocked', 'awaiting', 'failed', 'undone', 'declared')),
    summary TEXT NOT NULL DEFAULT '' CHECK (char_length(summary) <= 500),
    undo JSONB,                         -- {tool, args} khi reversible và biết cách ngược
    undo_until TIMESTAMPTZ,
    intent_id UUID,                     -- F4
    app_tool_call_id UUID,              -- liên kết với app_tool_call có sẵn
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- index (workspace_id, created_at DESC)            CONCURRENTLY
-- index (workspace_id, task_id)                    CONCURRENTLY
-- index (workspace_id, effect_class, created_at)   CONCURRENTLY
```

Append-only trừ hai chuyển `status`: `awaiting → executed|blocked|failed` và `executed → undone`. Không có `DELETE`. Không lưu tham số đầy đủ của lời gọi (có thể chứa dữ liệu nhạy cảm); `summary` là chuỗi do bộ phân loại tạo, tham số đầy đủ ở `app_tool_call` có sẵn theo chính sách lưu giữ của nó.

### 4.5 Nguồn hệ quả

| Nguồn | Đi qua đâu | Cưỡng chế được |
| --- | --- | --- |
| `connected_app` | gateway `internal/connectedapps` | có |
| `plugin_action` | Action API (`handler/plugin_action.go`) | có |
| `cli` | các lệnh `multica` thay đổi dữ liệu workspace | có (`write_internal`, luôn `allow` trừ khi `paused` hoặc `shadow`) |
| `channel` | gửi ra Lark, Slack, WeCom, DingTalk, Telegram, Weixin | có |
| `webhook` | webhook gửi ra (`webhook_delivery`) | có |
| `publish_channel` | kênh xuất bản (`packages/core/publish-channels`) | có |
| `declared` | agent tự khai báo trước khi làm việc ngoài nền tảng | **không**: chỉ ghi nhận và trả lời chính sách |

Khai báo:

```
multica effect declare --class send --reversibility irreversible \
    --target "smtp:customer@example.com" --summary "Gửi báo giá cho khách"
→ { "event_id": "...", "decision": "rehearse", "intent_id": "..." }   (exit 0 khi allow, 3 khi confirm/rehearse, 4 khi block)
```

Khối hướng dẫn trong briefing của mọi agent (khi bật cờ) nói: trước mọi hành động ngoài nền tảng thuộc các lớp `send`, `share`, `delete`, `spend` hoặc `irreversible`, gọi `multica effect declare` và làm theo quyết định. Đây là thoả thuận, không phải hàng rào (D11). Giao diện ghi rõ "đã khai báo" khác "đã kiểm soát".

### 4.6 Hoàn tác

- `POST /api/effects/{id}/undo` (owner, admin, người đã duyệt hành động): khi `undo` có và `now < undo_until` (mặc định 24 giờ), nền tảng gọi tool ngược qua gateway với chính tài khoản đó, ghi một `effect_event` mới (`source = connected_app`, `summary = "Hoàn tác <id>"`) và chuyển dòng gốc thành `undone`.
- `write_internal` của agent (sửa issue, comment, tài liệu) hoàn tác bằng cơ chế revision có sẵn của từng loại; v1 chỉ hỗ trợ: mô tả issue (revision có sẵn), chỉ dẫn agent (`agent_instruction_revision`), tài liệu knowledge (bản nháp). Loại khác hiện "không hoàn tác được".

### 4.7 Giao diện

- **Governance → Effects** (tab mới): dòng thời gian lọc theo lớp, nguồn, agent, quyết định; nút Hoàn tác khi còn hạn; ô "đã khai báo" tách riêng.
- **Governance → Controls**: lưới chính sách 7×3×4 với ô ghi đè; ô không được lỏng hơn bị khoá kèm lý do.
- **Run detail**: các hệ quả của run.

### 4.8 Nghiệm thu F3

1. Mỗi nguồn trong 4.5 (trừ `declared`) tạo đúng một `effect_event` cho mỗi hành động, với lớp đúng, kể cả khi bị chặn.
2. Bảng 4.3 được test đủ 7×3×4 ô; ghi đè lỏng hơn ở L2 bị từ chối 422; `spend → allow` bị từ chối ở mọi mức.
3. Task `shadow` gọi tool `write` qua gateway nhận `block` dù agent có mức `write`.
4. Hoàn tác một `GMAIL_CREATE_DRAFT` gọi đúng tool ngược và chuyển dòng thành `undone`; quá `undo_until` thì 409.
5. `multica effect declare` trả đúng exit code cho từng quyết định.

## 5. F4 Rehearse & Preview

### 5.1 Mục tiêu

Người duyệt **kết quả sẽ xảy ra** (nội dung email, bài đăng, bản ghi sẽ đổi, số tiền), không duyệt kế hoạch bằng lời. Duyệt xong thì đúng thứ đó được thực thi.

**Phép thử 10x:** agent giỏi hơn thì làm nhiều hành động ra ngoài hơn; một chỗ duyệt nhanh, chính xác, gộp được cho hành động không đảo ngược càng có giá trị. Giá trị tăng.

### 5.2 Ý định hành động

```sql
CREATE TABLE effect_intent (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    task_id UUID,
    agent_id UUID,
    issue_id UUID,
    source TEXT NOT NULL,               -- như effect_event.source
    target TEXT NOT NULL,
    effect_class TEXT NOT NULL,
    reversibility TEXT NOT NULL,
    call JSONB NOT NULL,                -- lời gọi chuẩn hoá: {tool, account_id, args}; NULL-args cho declared
    call_sha256 TEXT NOT NULL,
    preview JSONB NOT NULL,             -- {kind, title, body_markdown, fields:[{label, before, after}], amount_micros?, recipients?[]}
    rationale TEXT NOT NULL DEFAULT '' CHECK (char_length(rationale) <= 2000),
    predicted JSONB,                    -- F5: {verdict, confidence, why}
    state TEXT NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'approved', 'rejected', 'expired', 'executed', 'failed', 'withdrawn')),
    decided_by UUID,
    decided_at TIMESTAMPTZ,
    decision_note TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,    -- mặc định now + 72h
    batch_key TEXT NOT NULL DEFAULT '', -- gộp: agent_id + target + effect_class + ngày
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- index (workspace_id, state, created_at)  CONCURRENTLY
-- index (workspace_id, batch_key) WHERE state = 'pending'  CONCURRENTLY
```

### 5.3 Luồng

```
agent gọi tool ─► chính sách = rehearse ─► gateway không gọi; tạo effect_intent + preview
                                           trả cho agent: {status: "awaiting_approval", intent_id}
                                           effect_event status = awaiting
người duyệt ─► approve ─► nền tảng tự gọi đúng `call` (so call_sha256) qua gateway
                        ├─ thành công: intent executed, effect_event executed, comment hệ thống trên issue,
                        │              đánh thức agent qua đường wakeup có sẵn với kết quả lời gọi
                        └─ lỗi:        intent failed, đánh thức agent với lỗi
            ─► reject (ghi chú bắt buộc) ─► đánh thức agent với ghi chú; agent có thể tạo ý định mới
            ─► hết hạn ─► expired, đánh thức agent
```

- Preview do bộ tạo preview theo toolkit sinh ra từ `call` (không phải agent viết), để preview không nói khác lời gọi. Toolkit curated có bộ tạo riêng (email: người nhận, tiêu đề, thân; bài đăng: kênh, nội dung, ảnh; bản ghi CRM: trường trước và sau). Tool khác: bảng tham số đã làm gọn.
- Agent kèm `rationale` (lý do) qua tham số đặc biệt `_rationale` của gateway hoặc `--rationale` của `multica effect declare`.
- Người duyệt sửa được nội dung trước khi duyệt cho các preview `kind` có trường sửa được (thân email, nội dung bài đăng). Sửa thì `call` mới được băm lại, và comment hệ thống ghi "đã sửa trước khi gửi".
- `declared` (ngoài nền tảng): duyệt chỉ trả quyết định về cho agent (đánh thức với "đã duyệt"); nền tảng không thực thi được.
- Gộp: các ý định `pending` cùng `batch_key` hiện thành một thẻ với "Duyệt tất cả" và duyệt từng cái.
- `confirm` của connected apps hiện có (`app_action_request`) giữ nguyên. `rehearse` là bậc chặt hơn, dùng bảng mới; không chuyển dữ liệu cũ.

### 5.4 Flow có hệ quả ra ngoài

Bước của flow có `effects: external` (trường manifest có sẵn hoặc mới, xem 15) tạo `effect_intent` với `source = declared` cho bước đó trước khi bước chạy, và chỉ chạy khi được duyệt. Cổng Launch của pipeline giữ nguyên là quyết định của người; F4 không thay nó.

### 5.5 Giao diện

- **Inbox**: thẻ "Chờ duyệt hành động" với preview đầy đủ, nút Duyệt, Sửa rồi duyệt, Từ chối (bắt ghi chú), dự đoán của F5 nếu có ("Dự đoán: bạn sẽ duyệt, 92%").
- **Brief**: mục "Hành động chờ duyệt" liệt kê theo lô.
- Ứng dụng chat (Phase 2 2.7): nút duyệt và từ chối có chữ ký và hạn, như nút proposal có sẵn.

### 5.6 Nghiệm thu F4

1. Một tool `send` ở L1 không được gọi; ý định có preview khớp tham số; duyệt thì gateway được gọi đúng một lần với đúng tham số đã băm.
2. Sửa thân email rồi duyệt gửi đúng bản đã sửa, và comment ghi việc sửa.
3. Gọi lại API duyệt cho ý định đã `executed` trả 409, không gọi lần hai.
4. Hết 72 giờ thì `expired` và agent được đánh thức đúng một lần.
5. Năm ý định cùng lô duyệt bằng một lần bấm thành năm lời gọi và năm sự kiện.

## 6. F5 Principal Profile

### 6.1 Mục tiêu

Học phán đoán của người quyết định từ các quyết định thật, để (a) điền sẵn câu trả lời dự đoán cho mọi câu hỏi, (b) với loại quyết định đã dự đoán đúng ổn định, người chủ có thể uỷ quyền cho dự đoán.

**Phép thử 10x:** mô hình giỏi hơn không làm nó biết người chủ của *bạn* muốn gì. Dự đoán càng chính xác, người càng ít bị hỏi. Giá trị tăng.

### 6.2 Dữ liệu

```sql
CREATE TABLE principal_profile (
    workspace_id UUID NOT NULL,
    member_id UUID NOT NULL,            -- user id của thành viên
    status TEXT NOT NULL CHECK (status IN ('active', 'paused')),
    consent_at TIMESTAMPTZ NOT NULL,
    share_with_owners BOOLEAN NOT NULL DEFAULT false,
    version INTEGER NOT NULL DEFAULT 1,
    summary TEXT NOT NULL DEFAULT '' CHECK (char_length(summary) <= 4000),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);   -- unique (workspace_id, member_id) CONCURRENTLY

CREATE TABLE principal_trait (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    member_id UUID NOT NULL,
    decision_kind TEXT NOT NULL,        -- 'proposal:<kind>' | 'effect:<class>' | 'outcome' | 'inquiry' | 'general'
    statement TEXT NOT NULL CHECK (char_length(statement) BETWEEN 1 AND 500),
    evidence JSONB NOT NULL,            -- [{type: decision|comment|interview, ref}] tối thiểu một phần tử
    origin TEXT NOT NULL CHECK (origin IN ('learned', 'interview', 'edited')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    retired_at TIMESTAMPTZ
);   -- index (workspace_id, member_id, decision_kind) CONCURRENTLY

CREATE TABLE decision_prediction (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    member_id UUID NOT NULL,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('proposal', 'effect_intent', 'outcome_check', 'inquiry')),
    subject_id UUID NOT NULL,
    decision_kind TEXT NOT NULL,
    predicted TEXT NOT NULL,            -- approve | reject | pass | fail | <lựa chọn của inquiry>
    confidence NUMERIC NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    why TEXT NOT NULL DEFAULT '' CHECK (char_length(why) <= 1000),
    trait_ids UUID[] NOT NULL DEFAULT '{}',
    profile_version INTEGER NOT NULL,
    predictor_task_id UUID,
    actual TEXT,
    matched BOOLEAN,
    applied_by_delegation BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);   -- unique (subject_type, subject_id, member_id) CONCURRENTLY
     -- index (workspace_id, member_id, decision_kind, resolved_at) CONCURRENTLY

CREATE TABLE principal_delegation (
    workspace_id UUID NOT NULL,
    member_id UUID NOT NULL,
    decision_kind TEXT NOT NULL,
    min_confidence NUMERIC NOT NULL DEFAULT 0.90,
    enabled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    disabled_at TIMESTAMPTZ,
    disabled_reason TEXT NOT NULL DEFAULT ''
);   -- unique (workspace_id, member_id, decision_kind) WHERE disabled_at IS NULL CONCURRENTLY
```

### 6.3 Học

- **Phỏng vấn khởi đầu** (theo cách của Distilly): khi người bật profile, Secretary mở một cuộc chat khoảng 5 phút với 8 câu hỏi cố định (ưu tiên giữa tốc độ và chất lượng, ngưỡng chi phí tự quyết, điều không bao giờ được làm, giọng văn với khách, khi nào muốn được hỏi…). Mỗi câu trả lời thành một trait `origin = interview` có bằng chứng là tin nhắn đó.
- **Sau mỗi quyết định** của người có profile (proposal, ý định hành động, kiểm chứng thủ công, inquiry): một job gom các quyết định mới mỗi 6 giờ và giao cho Secretary một task `routine` "cập nhật profile": đọc quyết định, ghi chú, các trait hiện có; đề xuất thêm, sửa (bằng cách retire và tạo mới) hoặc retire trait. Mỗi trait mới phải dẫn ít nhất một quyết định; server từ chối trait không có bằng chứng hợp lệ (ref tồn tại, thuộc đúng người).
- **Người sửa trực tiếp** trong trang profile: sửa câu (thành `edited`), xoá (retire), thêm. Sửa của người không bao giờ bị job ghi đè: trait `edited` chỉ được retire bởi chính người đó.
- Mỗi lần thay đổi tăng `version`. Lịch sử giữ bằng trait đã retire.
- Giới hạn: tối đa 200 trait `active` mỗi người; vượt thì job phải retire trước khi thêm.

### 6.4 Dự đoán

- Khi một câu hỏi được tạo cho người quyết định có profile `active`, server giao cho Secretary một task nhẹ "dự đoán" (run class `routine`, có thể gộp nhiều câu trong một task). Đầu vào: câu hỏi, các trait cùng `decision_kind` và `general`, 10 quyết định gần nhất cùng loại kèm kết quả. Đầu ra qua `multica principal predict --subject ... --predicted ... --confidence ... --why ... --traits ...`.
- Dự đoán hiện cạnh câu hỏi. Người quyết định xong thì `actual` và `matched` được ghi.
- **Độ chính xác** theo `decision_kind`: tỷ lệ `matched` trên 50 dự đoán đã resolve gần nhất, cùng cận dưới Wilson.
- Dự đoán không bao giờ được tạo cho câu hỏi về chính profile hay về chính người được hỏi (ví dụ org change về quyền của họ).

### 6.5 Uỷ quyền bằng dự đoán

- Bật theo `decision_kind`, chỉ bởi chính người có profile, chỉ khi có ít nhất 20 dự đoán resolve và tỷ lệ khớp ≥ 85%. Server kiểm điều kiện lúc bật.
- Khi bật: câu hỏi mới cùng loại có dự đoán với `confidence ≥ min_confidence` được áp như quyết định của người đó với `actor_type = principal`, `applied_by_delegation = true`. Ghi governance event `principal_delegated_decision`. Hiện trong FYI của brief với nút "Đảo lại" trong 72 giờ (dùng đường reject/undo có sẵn của loại đó; với ý định hành động `reversible` là undo của F3).
- **Cấm bằng code**: `proposal:org_change`, cổng Launch và `stage_gate`, `effect:spend`, `effect:share`, mọi ý định `irreversible`, mọi câu hỏi `urgent`, mọi thứ liên quan tới budget, autonomy, envelope, policy, profile.
- Tự tắt: nếu trong 20 quyết định uỷ quyền gần nhất có 2 lần bị người đảo lại, hoặc độ chính xác cuộn rơi dưới 80%. Ghi `disabled_reason` và governance event; người bật lại được khi đủ điều kiện.
- **Điều khoản của spec cho chỗ dừng chờ người**: bật uỷ quyền là quyết định của chính người có profile (trao quyền mới).

### 6.6 Quyền riêng tư và đồng ý

- Bật profile cần thao tác của chính người đó trong trang Tài khoản, với văn bản đồng ý nói rõ: dữ liệu nào được đọc (quyết định và ghi chú trong workspace này, câu trả lời phỏng vấn), ai đọc được, không dùng để đánh giá người, xoá được bất cứ lúc nào.
- Đọc: chính người đó; Secretary của workspace trong task dự đoán và cập nhật; owner khác chỉ khi `share_with_owners`. Agent khác: 403. Không có trong `company export` mặc định (F11 có cờ riêng, cần chính người đó đồng ý).
- Xoá profile: xoá cứng `principal_profile`, `principal_trait`, `principal_delegation`; `decision_prediction` giữ số liệu khớp nhưng xoá `why` và `trait_ids`. Ghi governance event không có nội dung.
- Không bao giờ đọc profile trong scorecard, review hay org change (test khẳng định truy vấn của các job đó không chạm bảng này).
- Không mô hình hoá thành viên không phải owner hoặc admin trong v1.

### 6.7 Nghiệm thu F5

1. Không có profile thì không có dự đoán và không có task dự đoán.
2. Trait không có bằng chứng, hoặc bằng chứng của người khác, bị từ chối 422.
3. Uỷ quyền không bật được khi dưới 20 mẫu hoặc dưới 85%; bật được khi đủ; áp đúng cho câu hỏi đủ `confidence`; không bao giờ áp cho các loại cấm (test thử từng loại).
4. Hai lần đảo lại trong 20 lần uỷ quyền gần nhất tự tắt uỷ quyền.
5. Agent không phải Secretary đọc profile nhận 403; xoá profile xoá đúng các bảng.

## 7. F6 Attention Budget

### 7.1 Mục tiêu

Thời gian của người quyết định là một ngân sách có đơn vị (phút), có chủ (squad hỏi), có kế toán (đã tiêu bao nhiêu, cho việc gì), và có luật: câu hỏi phải gộp, có lý do, có dự đoán.

**Phép thử 10x:** agent càng nhiều và càng nhanh thì thời gian của người càng là thứ khan hiếm nhất. Giá trị tăng.

### 7.2 Chi phí ước tính

Mỗi câu hỏi có `attention_cost_minutes` ước tính lúc tạo:

| Loại câu hỏi | Phút cơ bản | Cộng thêm |
| --- | --- | --- |
| proposal | 3 | +1 cho mỗi 400 từ thân |
| `org_change` | 6 | |
| ý định hành động (F4) | 1 | +1 nếu `irreversible` |
| kiểm chứng `manual` (F1) | 4 | |
| xác nhận ghi connected apps | 1 | |
| inquiry của Secretary | 2 | |

Có dự đoán của F5 với `confidence ≥ 0.8`: nhân 0.5. Lô gộp: phút của cả lô = phút lớn nhất trong lô + 0.5 cho mỗi câu thêm.

Thời gian thật được đo khi có thể: từ lúc người mở câu hỏi trên giao diện tới lúc quyết định, cắt ở 30 phút. Số đo thật dùng để hiệu chỉnh bảng hằng tháng (job tính trung vị theo loại và hiện cạnh bảng mặc định; không tự đổi bảng).

### 7.3 Ngân sách

```sql
CREATE TABLE attention_ledger (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    member_id UUID,                     -- người quyết định nhận câu hỏi; NULL là bất kỳ owner/admin nào
    squad_id UUID,                      -- squad hỏi; NULL là hệ thống
    subject_type TEXT NOT NULL,         -- proposal | effect_intent | outcome_check | app_action_request | inquiry
    subject_id UUID NOT NULL,
    reason TEXT NOT NULL CHECK (reason IN
        ('judgment', 'irreversible', 'missing_access', 'missing_info', 'policy', 'verification')),
    estimated_minutes NUMERIC NOT NULL,
    actual_seconds INTEGER,
    state TEXT NOT NULL CHECK (state IN ('queued', 'delivered', 'answered', 'expired', 'delegated')),
    week_start DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    answered_at TIMESTAMPTZ
);   -- unique (subject_type, subject_id) CONCURRENTLY
     -- index (workspace_id, week_start, squad_id) CONCURRENTLY
```

- Cài đặt workspace: `attention_minutes_per_week` (mặc định 60), `attention_split` (`equal` mặc định, hoặc `goals`: chia theo số goal đang active của squad).
- Phần của squad = ngân sách × tỷ lệ của squad. Squad chưa dùng hết không chuyển sang tuần sau.
- Khi tạo câu hỏi: nếu phần của squad còn đủ thì `delivered` ngay (như hôm nay). Nếu không: `queued`, câu hỏi vẫn tồn tại nhưng không đến Inbox và không đến ứng dụng chat; nó xuất hiện trong brief kế tiếp ở mục "Đang xếp hàng vì hết thời gian của bạn" theo thứ tự ưu tiên. Người quyết định vẫn mở và trả lời được bất cứ lúc nào.
- Câu hỏi `urgent` không bị xếp hàng nhưng vẫn bị luật "một câu khẩn mỗi squad mỗi 7 ngày" của Phase 1.
- Giới hạn số lượng của Phase 1 và 2.6 (3/3/10 và thích nghi) giữ nguyên và áp trước; F6 là lớp thứ hai.
- Quyết định áp bằng uỷ quyền (F5) có `state = delegated` và không tiêu phút.

### 7.4 Luật cho câu hỏi

Mọi câu hỏi tạo bởi agent khi bật cờ phải có:

1. `reason` (bảng ở 7.3). Thiếu thì 422.
2. `why_not_decide` (≤ 500 ký tự): vì sao agent không tự quyết. Thiếu thì 422.
3. Khi `reason = missing_info`: danh sách đã tìm ở đâu (`searched: [knowledge|issues|comments|web|attachments]`), đúng bước "tìm trước khi hỏi" của Automation First.

`reason` là dữ liệu đầu vào của Reach Map (F10): `missing_access` và `missing_info` là khoảng trống có thể lấp; `judgment` và `irreversible` là chỗ dừng đúng.

### 7.5 Giao diện và chỉ số

- **Governance → Controls**: ô ngân sách tuần, cách chia.
- **Office header / pulse**: "Thời gian của bạn tuần này: 23 / 60 phút".
- **Brief**: mục "Thời gian của bạn" (đã tiêu theo squad, câu hỏi đang xếp hàng, chỉ số phút người trên mỗi việc xong).
- Chỉ số `attention.minutes_per_done` (metric nội tại mới) = phút đã tiêu trong kỳ / issue vào `done` trong kỳ.

### 7.6 Nghiệm thu F6

1. Mỗi loại câu hỏi tạo đúng một dòng ledger với phút ước tính đúng bảng, kể cả hệ số dự đoán và lô.
2. Squad hết phần thì câu hỏi tiếp theo là `queued`, không có thông báo Inbox hay chat, và có trong digest của brief kế tiếp.
3. Câu hỏi khẩn không bị xếp hàng.
4. Câu hỏi thiếu `reason` hoặc `why_not_decide` từ agent bị 422; từ người thì không bắt buộc.
5. Tắt cờ: không ghi ledger, hành vi đúng Phase 4.

## 8. F7 Disagreement Jury

### 8.1 Mục tiêu

Với việc quan trọng, hai hoặc ba người kiểm từ các mô hình khác nhau kiểm độc lập. Đồng ý thì tự đi tiếp. Bất đồng là tín hiệu chính xác về chỗ cần người.

**Phép thử 10x:** mô hình thông minh nhất vẫn có điểm mù chung trong cùng họ mô hình. Sự đa dạng giữa các họ mô hình là thứ bền, và Delegora vốn chạy nhiều runtime. Giá trị tăng.

### 8.2 Khi nào gọi Jury

Một tiêu chí `agent_judgment` được kiểm bằng Jury khi cả hai đúng:

1. kết quả có `verification_mode = full`, hoặc tiêu chí có `check_spec.jury = true`;
2. loại việc được đánh dấu quan trọng: cài đặt workspace `jury_work_classes` (danh sách tiền tố, ví dụ `flow:flow_kv_pr:`), hoặc issue có độ ưu tiên `urgent`/`high`.

Còn lại dùng một agent kiểm chứng như F1.

Hội đồng: cài đặt `jury_pool` (danh sách agent, tối thiểu 2). Chọn 2 người (3 khi issue `urgent`): khác người làm, ưu tiên `runtime_provider` khác người làm và khác nhau, rồi `model` khác nhau. Ghi `diversity` (`provider`, `model_only`, `none`) vào lần kiểm. `none` (cùng model) vẫn chạy nhưng giao diện cảnh báo.

### 8.3 Phán quyết

```sql
CREATE TABLE jury_panel (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    outcome_id UUID NOT NULL,
    criterion_id UUID,
    juror_agent_ids UUID[] NOT NULL,
    diversity TEXT NOT NULL CHECK (diversity IN ('provider', 'model_only', 'none')),
    state TEXT NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'unanimous', 'split', 'escalated', 'resolved', 'timed_out')),
    verdict TEXT,
    resolved_by_type TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at TIMESTAMPTZ
);   -- index (outcome_id) CONCURRENTLY
```

- Mỗi juror chạy một task `verification` mù (không thấy juror khác), ghi `outcome_verification` với `verifier_type = agent`.
- Đủ phiếu: giống nhau → `unanimous`, ghi một lần kiểm `verifier_type = jury` với kết quả đó. Khác nhau → `split` → tạo câu hỏi `outcome_check` cho người quyết định, kèm các lý do song song (tốn ngân sách chú ý, `reason = verification`) → người trả lời → `resolved`.
- `unable` của một juror: thay bằng juror kế tiếp một lần; vẫn thiếu thì `escalated` như `split`.
- Quá 24 giờ chưa đủ phiếu: `timed_out`, tiêu chí `unable`.
- Ghi nhận **tỷ lệ bất đồng** theo loại việc (`jury.split_rate`, metric nội tại). Tỷ lệ cao là dấu hiệu tiêu chí mơ hồ: brief gợi ý viết lại tiêu chí của loại việc đó.
- Juror nào bị người quyết định bác (người chọn kết luận ngược với phiếu của juror) trong một `split` được ghi vào uy tín của juror với `work_class = "verify:" + work_class gốc`. Người kiểm cũng có track record.

### 8.4 Nghiệm thu F7

1. Với pool có ba provider, hội đồng hai người không có ai trùng provider với người làm.
2. Juror không thấy phiếu của juror khác trong briefing (test nội dung briefing).
3. Hai phiếu giống nhau cho kết luận không cần người; khác nhau tạo đúng một câu hỏi và ledger.
4. Một juror `unable` được thay đúng một lần.
5. Chi phí của task juror cộng vào `verification_cost_micros` và bị budget giữ.

## 9. F8 Seat Dossier và Shadow Run

### 9.1 Mục tiêu

Kinh nghiệm, uy tín và mối quan hệ thuộc về **ghế** (agent trong tổ chức), không thuộc về mô hình đang ngồi ghế. Mô hình mới được thử trên việc thật, ở chế độ không gây hệ quả, trước khi vào ghế.

**Phép thử 10x:** mỗi lần có mô hình tốt hơn, công ty muốn đổi ngay mà không mất những gì ghế đã học. Đây là điểm mà nền tảng của một hãng mô hình không làm được. Giá trị tăng.

### 9.2 Hồ sơ ghế

```sql
CREATE TABLE seat_dossier_entry (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('lesson', 'pitfall', 'preference', 'context', 'contact')),
    text TEXT NOT NULL CHECK (char_length(text) BETWEEN 1 AND 800),
    work_class TEXT NOT NULL DEFAULT '',   -- '' áp mọi việc
    evidence JSONB NOT NULL,               -- [{type: outcome|retro|comment|document, ref}] tối thiểu một
    origin TEXT NOT NULL CHECK (origin IN ('outcome', 'retro', 'agent', 'member')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    uses INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    retired_at TIMESTAMPTZ
);   -- index (workspace_id, agent_id, status) CONCURRENTLY
```

- **Nguồn**: sau mỗi kết quả `fail` hoặc `partial` đã chốt, và sau mỗi retrospective của Phase 4 4.7, agent đó nhận một task `routine` ngắn "ghi bài học" và gọi `multica seat lesson add --kind pitfall --text ... --evidence outcome:<id>` (tối đa 3 mục mỗi lần). Thành viên thêm và sửa được trong trang agent.
- **Đọc**: briefing của mọi task của agent có khối "Hồ sơ ghế" gồm tối đa 4.000 ký tự: các mục `pitfall` và `lesson` khớp `work_class` của issue trước, rồi mục chung, mới trước. Khối này là văn bản trung lập, không phụ thuộc runtime.
- **Dọn**: tối đa 100 mục `active` mỗi agent; job tháng giao cho agent retire mục trùng hoặc lỗi thời (dẫn lý do). Mục `member` chỉ người retire.
- Đây không thay `agent_instruction_revision`: chỉ dẫn là vai trò; hồ sơ là kinh nghiệm.

### 9.3 Đổi người ngồi ghế

- Mọi thay đổi `agent.runtime_id` hoặc `agent.model` ghi governance event `seat_model_changed` với `{from_provider, from_model, to_provider, to_model}`.
- `work_outcome` lưu `runtime_provider` và `model` lúc tạo, nên `track_record` có dòng theo từng model (cột `model`). Giao diện thành tích có bộ lọc model.
- Chế độ kiểm chứng giữ nguyên qua lần đổi (3.3).

### 9.4 Shadow run

```sql
CREATE TABLE shadow_config (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    agent_id UUID NOT NULL,             -- ghế
    candidate_runtime_id UUID NOT NULL,
    candidate_model TEXT NOT NULL DEFAULT '',
    purpose TEXT NOT NULL CHECK (purpose IN ('model_trial', 'necessity_test')),
    necessity_test_id UUID,             -- F9
    work_class_prefix TEXT NOT NULL DEFAULT '',
    sample_rate NUMERIC NOT NULL DEFAULT 0.3 CHECK (sample_rate > 0 AND sample_rate <= 1),
    target_pairs INTEGER NOT NULL DEFAULT 20 CHECK (target_pairs BETWEEN 5 AND 200),
    auto_promote BOOLEAN NOT NULL DEFAULT false,
    state TEXT NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'completed', 'stopped')),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE shadow_pair (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    config_id UUID NOT NULL,
    issue_id UUID NOT NULL,
    primary_task_id UUID NOT NULL,
    shadow_task_id UUID,
    comparison TEXT CHECK (comparison IN ('primary', 'shadow', 'tie', 'unable')),
    comparison_reason TEXT NOT NULL DEFAULT '',
    primary_cost_micros BIGINT NOT NULL DEFAULT 0,
    shadow_cost_micros BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    compared_at TIMESTAMPTZ
);   -- index (config_id) CONCURRENTLY
```

- Khi task chính của ghế được claim và issue khớp `work_class_prefix`, chọn mẫu tất định theo `sha256(issue_id || config_id) < sample_rate`. Được chọn thì enqueue một task `shadow` cho **agent bóng** (D27): agent ẩn có cùng vai và cùng hồ sơ ghế, còn runtime và model là ứng viên; briefing giống hệt (cùng issue, cùng hồ sơ ghế). Claim chỉ trao task bóng cho daemon có capability `DaemonCapabilityShadowV1` (D30).
- **Cách ly**: (a) Effect Registry chặn mọi thứ trừ `read` (D18); (b) task bóng không nhận MCP overlay có quyền ghi và không nhận token connected apps có mức ghi; (c) daemon chạy task bóng trong thư mục làm việc tạm, không claim worktree, không chạy ở chế độ `in_place`, và runtime có quyền push bị từ chối (D27); không có biến môi trường bí mật của agent trừ khoá runtime; (d) mọi lệnh `multica` có tác động gọi từ task bóng bị server từ chối 403 `shadow_task`. Đầu ra của task bóng là văn bản cuối và các tệp trong thư mục tạm, được lưu làm tệp đính kèm riêng tư của `shadow_pair`.
- **Giới hạn trung thực**: runtime ứng viên vẫn có thể gọi mạng bằng khoá của chính nó. Giao diện bật shadow nói rõ: "Chỉ dùng runtime không có khoá truy cập hệ thống thật ngoài khoá mô hình".
- **So sánh**: khi cả hai task xong, một task `verification` mù (juror ngẫu nhiên từ `jury_pool`, khác cả hai model nếu có thể) nhận hai đầu ra được gán nhãn A/B ngẫu nhiên và trả `A`, `B`, `tie` hoặc `unable` kèm lý do. Khi kết quả chính đã chốt, nếu kết quả chính là `fail` thì cặp được tính `shadow` thắng chỉ khi juror chọn bóng; không đổi gì khác.
- **Kết thúc**: đủ `target_pairs` cặp có so sánh. Báo cáo: thắng, hoà, thua, cận dưới Wilson của tỷ lệ bóng không thua (`(shadow + tie)/n`), chi phí trung bình mỗi bên.

### 9.5 Chi phí

Task bóng là run class `shadow`, bị budget giữ. Bật shadow hiển thị ước tính chi phí = chi phí trung bình 30 ngày của task chính khớp × `sample_rate` × `target_pairs`. Ước tính vượt phần còn lại của budget squad thì phải xác nhận.

### 9.6 Vào ghế

- Hoàn tất với cận dưới Wilson của "bóng không thua" ≥ 0.5 và tỷ lệ thắng ≥ tỷ lệ thua: tạo proposal `org_change` kiểu mới `runtime_change` (payload `{agent_id, to_runtime_id, to_model, shadow_config_id, report}`), người chủ quyết.
- `auto_promote = true` (chỉ owner, admin bật, có lý do; điều khoản: trao quyền mới): thay vì proposal, áp đổi luôn với `actor_type = shadow`, governance event `seat_model_changed`, vào probation 14 ngày của Phase 4 4.6. Probation `worse` thì tự đổi lại runtime và model cũ (`runtime_change` có thể đảo ngược nên khác `hire`).
- Thêm `runtime_change` vào bảng `org_change` của Phase 4 và vào `org_change_probation_types`.

### 9.7 Nghiệm thu F8

1. Đổi model của agent ghi event; kết quả sau đó mang model mới; track record có dòng riêng theo model.
2. Hồ sơ ghế trong briefing không quá 4.000 ký tự và ưu tiên đúng loại việc.
3. Task bóng gọi lệnh `multica issue update` nhận 403; gọi tool ghi qua gateway nhận `block`; không claim worktree.
4. Mẫu tất định: cùng `issue_id` và `config_id` luôn cho cùng kết quả chọn.
5. Juror so sánh nhận nhãn A/B ngẫu nhiên và không thấy model nào là A.
6. `auto_promote` đổi runtime khi đạt ngưỡng và đổi lại khi probation `worse`.

## 10. F9 Scaffolding Sunset

### 10.1 Mục tiêu

Mỗi mảnh giàn giáo (skill, khối chỉ dẫn, bước flow) có một bài kiểm tra cần thiết: chạy cùng loại việc **không có** mảnh đó, so kết quả. Không kém hơn thì đề xuất gỡ. Nền tảng chủ động dọn phần lỗi thời của chính nó.

**Phép thử 10x:** mô hình giỏi hơn làm nhiều giàn giáo thành thừa; một cơ chế phát hiện cái thừa càng có giá trị. Đây là cách duy nhất để lõi không bị giàn giáo kéo xuống.

### 10.2 Bài kiểm tra

```sql
CREATE TABLE necessity_test (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('skill', 'instruction_block', 'flow_stage')),
    subject_ref TEXT NOT NULL,          -- tên skill | agent_id#block_heading | flow_id:stage_key
    agent_id UUID NOT NULL,
    shadow_config_id UUID,              -- F8, purpose = necessity_test
    margin NUMERIC NOT NULL DEFAULT 0.05,
    min_pairs INTEGER NOT NULL DEFAULT 20,
    state TEXT NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'not_needed', 'needed', 'inconclusive', 'stopped')),
    result JSONB NOT NULL DEFAULT '{}',
    proposal_id UUID,
    created_by_type TEXT NOT NULL,
    created_by_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);
```

- Dùng hạ tầng shadow của F8 với `candidate_runtime_id` = runtime hiện tại, cùng model, và **briefing của task bóng bỏ mảnh đang kiểm** (skill không được nạp; khối chỉ dẫn có tiêu đề đó bị cắt; bước flow: v1 không chạy bóng bước flow, chỉ thống kê, xem 10.4).
- Kết luận (không kém hơn, biên `margin`): gọi `d` là tỷ lệ bóng thua trừ tỷ lệ bóng thắng, với khoảng tin cậy 95% bằng phương pháp Newcombe. Cận trên của `d` < `margin` → `not_needed`. Cận dưới của `d` > 0 → `needed`. Còn lại sau `min_pairs × 2` cặp → `inconclusive`.
- `not_needed` → proposal `org_change` `skill_change` (gỡ skill khỏi agent) hoặc `instruction_change` (gỡ khối) kèm báo cáo. Không bao giờ tự gỡ (D20). Lý do: gỡ một mảnh thay đổi hành vi của ghế, và Phase 4 giữ thay đổi tổ chức là của người.

### 10.3 Ai khởi tạo

- **Lịch**: mỗi skill được gán cho agent, và mỗi khối chỉ dẫn có tiêu đề H2, được kiểm ít nhất một lần mỗi 180 ngày nếu agent có đủ việc (≥ 40 task trong 30 ngày). Job tuần chọn tối đa 2 bài kiểm tra mỗi squad đang chạy cùng lúc, ưu tiên mảnh lâu chưa kiểm nhất.
- **Khi đổi model** (F8): sau khi ghế đổi model, các mảnh của ghế được xếp lên đầu hàng đợi kiểm.
- **Người** khởi tạo từ trang agent.

### 10.4 Flow

Bước flow không chạy bóng trong v1 vì bước có cổng và hệ quả. Thay vào đó, job tháng tính cho mỗi `flow_stage`: tỷ lệ bước được duyệt ngay lần đầu, số lần bị gửi lại, phút người tiêu ở cổng (F6). Bước có tỷ lệ duyệt lần đầu ≥ 95% trên ≥ 30 lượt trong 90 ngày được ghi vào brief như "ứng viên gộp hoặc bỏ cổng", kèm số liệu, cho người viết flow theo [flow-authoring-runbook](../../engineering/flow-authoring-runbook.md).

### 10.5 Nghiệm thu F9

1. Task bóng của bài kiểm tra skill không có skill đó trong danh sách nạp; task chính có.
2. Dữ liệu giả 80 cặp hoà ở biên 0.05 cho `not_needed` và tạo đúng một proposal; 40 cặp bóng thua 30% cho `needed` và không tạo proposal. (Đã sửa từ 40 cặp: với n = 40 và biên 0.05, cận trên 95% của d là 0.0876, nên không thể ra `not_needed`; 40 cặp hoà ở biên 0.05 cho `inconclusive`. Xem `durable_necessity_fork_test.go`.)
3. Job tuần không chạy quá 2 bài kiểm tra mỗi squad.
4. Không có đường nào gỡ skill mà không qua proposal được duyệt.

## 11. F10 Reach Map

### 11.1 Mục tiêu

Một bản đồ cho người chủ thấy agent đã chạm được những phần nào của công việc, phần nào còn phải nhờ người, phần nào tốn thời gian người nhất, và tạo việc để lấp khoảng trống. Đây là `refine` của CLI-Anything áp cho cả công ty.

**Phép thử 10x:** mô hình giỏi hơn thì khoảng trống còn lại chủ yếu là quyền truy cập và giao diện, đúng thứ bản đồ này đo. Giá trị tăng.

### 11.2 Bề mặt công việc

Một "bề mặt" (`surface`) là một trong:

- `app:<toolkit>`: một toolkit connected apps (có sẵn trong catalog workspace hoặc đã từng được nhắc trong câu hỏi `missing_access`);
- `channel:<type>`: một kênh chat;
- `area:<work_class prefix>`: một nhóm loại việc (cắt `work_class` tới dấu `:` thứ hai);
- `external:<host>`: host trong `target` của hệ quả `declared`.

```sql
CREATE TABLE reach_snapshot (
    workspace_id UUID NOT NULL,
    snapshot_date DATE NOT NULL,
    surface TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('agent_operable', 'agent_with_confirm', 'declared_only', 'human_only', 'unknown')),
    effects_30d INTEGER NOT NULL DEFAULT 0,
    agent_effects_30d INTEGER NOT NULL DEFAULT 0,
    human_minutes_30d NUMERIC NOT NULL DEFAULT 0,
    gap_asks_30d INTEGER NOT NULL DEFAULT 0,     -- câu hỏi reason missing_access | missing_info
    top_ask_ids UUID[] NOT NULL DEFAULT '{}',
    gap_issue_id UUID,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);   -- unique (workspace_id, snapshot_date, surface) CONCURRENTLY
```

Trạng thái (theo thứ tự):

| Trạng thái | Khi |
| --- | --- |
| `agent_operable` | có hệ quả của agent được `allow` trong 30 ngày, và `gap_asks_30d = 0` |
| `agent_with_confirm` | hệ quả của agent chủ yếu qua `confirm` hoặc `rehearse` |
| `declared_only` | chỉ có hệ quả `declared` |
| `human_only` | có câu hỏi `missing_access` cho bề mặt này mà không có hệ quả của agent |
| `unknown` | còn lại |

Job đêm `jobs_reach_fork.go` (mới) tính snapshot; giữ 400 ngày.

### 11.3 Lấp khoảng trống

- Danh sách khoảng trống xếp theo `human_minutes_30d` giảm dần.
- Nút "Lấp khoảng trống" (owner, admin) tạo issue gán cho squad được chọn (mặc định squad có nhiều câu hỏi nhất cho bề mặt đó), mô tả sinh từ dữ liệu: bề mặt, các câu hỏi tiêu biểu, gợi ý theo loại (`app` chưa kết nối → kết nối tài khoản; `external` → xây công cụ, ví dụ bằng CLI-Anything, hoặc connected app; `missing_info` → tài liệu knowledge). Issue có tiêu chí `metric` mặc định: `reach.gap_asks_30d` của bề mặt giảm 50% trong 30 ngày.
- Secretary nhắc khoảng trống lớn nhất trong brief tuần một lần.

### 11.4 Giao diện

Governance → Reach (tab mới): lưới bề mặt tô theo trạng thái, kích thước theo phút người; chi tiết bề mặt với chuỗi thời gian và các câu hỏi tiêu biểu; con số tổng "Phần trăm hệ quả do agent làm (30 ngày)".

### 11.5 Nghiệm thu F10

1. Dữ liệu giả cho mỗi trạng thái cho đúng trạng thái.
2. "Lấp khoảng trống" tạo đúng một issue với tiêu chí `metric`; bấm lần hai mở issue đã có.
3. Snapshot hai lần trong cùng ngày ghi đè, không nhân đôi.

## 12. F11 Open Company Format

### 12.1 Mục tiêu

Một chuẩn mở, có version, mô tả cấu trúc **và lịch sử** của một công ty agent. Mọi framework, hôm nay hay sau này, chỉ cần adapter để vào làm. Dữ liệu của khách luôn mang đi được. Nghịch lý có chủ đích: nền tảng bền nhất là nền tảng định nghĩa chuẩn mà nó không sở hữu riêng.

**Phép thử 10x:** mô hình giỏi hơn sinh ra framework mới; chuẩn trung lập làm Delegora là nơi chúng đến, không phải đối thủ của chúng. Giá trị tăng.

### 12.2 Định dạng

- Schema: `docs/specs/durable-platform/ocf/v1.schema.json` (mới), JSON Schema 2020-12, và bản đọc cho người `docs/specs/durable-platform/ocf/README.md` (mới). Giấy phép của riêng thư mục này: CC-BY-4.0 cho tài liệu và MIT cho schema (ghi trong README của thư mục; không đổi giấy phép của repo).
- Gói xuất (`.ocf.zip`):

```
manifest.json         {"format": "ocf/1", "generator": "delegora/<version>", "exported_at", "workspace": {name, slug}, "parts": [...]}
structure.json        nội dung company export hiện có (floors, seats, squads, charters, routines, agents, pipeline,
                      kinds, policies, goals, budgets, autonomy) + envelopes, effect policy overrides
history/outcomes.ndjson            mỗi dòng một work_outcome đã chốt (không có nội dung issue)
history/track_records.ndjson
history/governance.ndjson          governance_event
history/dossiers.ndjson            seat_dossier_entry active
history/principals.ndjson          chỉ khi người có profile đồng ý xuất (cờ riêng)
attestations/*.json                F12
```

- `history/*` là tuỳ chọn khi xuất (`multica company export --history`). Không có bí mật, runtime, nội dung tài liệu, nội dung issue hay comment (giữ đúng loại trừ của Phase 4 4.9).
- Import: đọc `ocf/1` và định dạng export cũ (không có `manifest.json`). Lịch sử nhập vào workspace mới được đánh dấu `imported = true` và **không** tạo uy tín: track record nhập chỉ để xem, không dùng cho chế độ kiểm chứng hay envelope (chống nhập uy tín giả). Uy tín mang sang đáng tin chỉ qua attestation có chữ ký (F12).
- Kiểm tra: server kiểm gói theo schema trước khi import; CI kiểm mọi export mẫu trong test theo schema.

### 12.3 Giao thức worker

Đặc tả `docs/specs/durable-platform/ocf/worker-protocol.md` (mới): mô tả bằng ngôn ngữ trung lập các thao tác một worker bên ngoài cần, ánh xạ 1-1 sang API daemon hiện có: đăng ký, nhận task (claim), báo tiến độ (task message), báo xong, khai báo hệ quả (F3), ghi kiểm chứng (F1). Không thêm endpoint mới cho v1; giao thức là tài liệu hoá có version của những gì đã có, cộng các lệnh mới của chương trình. Adapter tham chiếu (D29): một **adapter CLI** (`multica-ocf-adapter`, mới) đóng gói worker theo giao thức này dưới dạng binary CLI, để chạy được như runtime tuỳ chỉnh hiện có. Không xây backend HTTP worker trong v1.

### 12.4 Nghiệm thu F11

1. Export `--history` của một workspace có dữ liệu của mọi tính năng hợp lệ theo schema.
2. Import vào workspace trống tạo lại cấu trúc; lịch sử có `imported = true`; không có chế độ kiểm chứng nào khác `full`.
3. Import gói export cũ vẫn chạy.
4. Gói thiếu `manifest.json` nhưng có trường lạ bị từ chối với lỗi chỉ ra trường.

## 13. F12 Agent Labor Market

### 13.1 Mục tiêu

Hub tiến hoá từ chợ skill và template thành chợ lao động có bảo chứng: một ghế được đăng tuyển với hợp đồng kết quả, một agent template nộp đơn mang theo track record có chữ ký. Uy tín trở thành tài sản mang đi được, và Delegora là bên bảo chứng.

**Phép thử 10x:** khi agent của mọi tổ chức đều giỏi, câu hỏi còn lại là "tin agent nào". Bên giữ lịch sử đã kiểm chứng trả lời được câu đó. Giá trị tăng.

### 13.2 Attestation

```json
{
  "format": "ocf-attestation/1",
  "subject": { "template_ref": "hub:<item>@<version>", "seat_label": "<tên hiển thị>" },
  "issuer": { "host": "<domain máy chủ>", "key_id": "<id khoá>" },
  "period": { "from": "2026-08-01", "to": "2026-11-01" },
  "work_classes": [
    { "class": "area:flow:flow_kv_pr", "n": 64, "pass": 58, "partial": 3, "fail": 3, "score_lower": 0.86,
      "verification": { "full": 20, "sampled": 44, "jury": 12 } }
  ],
  "models": ["<provider>/<model>"],
  "issued_at": "...",
  "signature": "<ed25519, base64, trên JSON chuẩn hoá RFC 8785 không có trường signature>"
}
```

- Chỉ owner, admin phát hành, cho một agent có `n ≥ 30` kết quả đã chốt trong kỳ. `work_class` được làm thô tới mức `area:` để không lộ id nội bộ; không có tên workspace (trừ khi người phát hành chọn hiện).
- Khoá: Ed25519, seed 32 byte đọc từ `MULTICA_ATTESTATION_KEY` (D28). Thiếu biến thì tính năng phát hành tắt: API phát hành trả 503 `attestation_disabled`; xác minh và khoá công khai `GET /.well-known/multica-attestation-keys.json` vẫn chạy. Không sinh khoá tự động, không lưu khoá trong DB. Xoay khoá giữ khoá cũ ở danh sách để xác minh.
- `POST /api/attestations/verify` (công khai, giới hạn tốc độ) và `multica attestation verify <file>` xác minh chữ ký với khoá của host trong `issuer` (tải qua HTTPS).

### 13.3 Đăng tuyển ghế và nộp đơn

- Loại item Hub mới `seat_offer`: vai, loại việc, tiêu chí kết quả mặc định, budget đề xuất, envelope đề xuất, runtime yêu cầu (nếu có). Trong v1, chỉ trong phạm vi một Hub org (không công khai liên tổ chức).
- Item agent template trên Hub được đính kèm attestation (tệp `attestations/*.json` trong gói).
- "Tuyển" = `org_change` kiểu `hire` có sẵn, với template từ Hub, kèm attestation; ghế mới bắt đầu ở `full` cho mọi loại việc (uy tín bên ngoài là thông tin cho người quyết định, không phải quyền ở đây), và vào probation 14 ngày.

### 13.4 Ngoài v1

Thanh toán, ký quỹ, hợp đồng pháp lý giữa tổ chức, bảo hiểm dựa trên attestation. Đây là các bước tiêu tiền và ra ngoài; chúng cần một spec riêng có điều khoản dừng chờ người.

### 13.5 Nghiệm thu F12

1. Attestation phát hành, sửa một chữ số, xác minh thất bại; nguyên bản xác minh thành công qua API và CLI.
2. Không phát hành được khi `n < 30`.
3. Tuyển từ `seat_offer` tạo proposal `hire` có attestation; ghế mới ở `full`.

## 14. Yêu cầu chung

### 14.1 Cờ tính năng

Thêm năm khoá vào `server/internal/featureflags/keys.go` bằng hook một dòng tới `keys_fork.go` (mới), mặc định tắt, và phía client qua `packages/core/feature-flags`. Mọi đường ghi mới kiểm cờ ở service, mọi phần giao diện mới kiểm cờ ở view. Theo [AGENTS.md](../../../AGENTS.md#change-and-delivery-rules), tính năng sau cờ mặc định tắt không được viết vào `apps/docs/content/docs/` hay `README*.md`; PR ghi lý do. PR bật cờ mặc định thì thêm tài liệu ở mọi locale.

| Cờ | Tính năng | Phụ thuộc cờ |
| --- | --- | --- |
| `durable_outcomes` | F1, F2 | |
| `durable_effects` | F3, F4 | |
| `durable_principal` | F5, F6 | F6 dùng được không cần F5; dự đoán chỉ khi có F5 |
| `durable_seats` | F7, F8, F9 | `durable_outcomes`, `durable_effects` (cách ly shadow cần F3) |
| `durable_open` | F10, F11, F12 | F10 cần `durable_effects`; F12 cần `durable_outcomes` |

Server từ chối bật cờ khi cờ phụ thuộc đang tắt.

### 14.2 Vị trí mã

| Lớp | Vị trí |
| --- | --- |
| Migration | `server/migrations/<n>_durable_<name>.up.sql` / `.down.sql`; mỗi chỉ mục một file `CONCURRENTLY`, đăng ký trong `server/cmd/migrate/main.go` |
| Truy vấn | `server/pkg/db/queries/durable_<name>.sql`, `make sqlc` |
| Service | `server/internal/service/durable_<name>_fork.go` |
| Handler | `server/internal/handler/durable_<name>_fork.go` |
| Route | `server/cmd/server/router_fork.go` (`registerForkWorkspaceRoutes`) |
| Job | `server/internal/scheduler/jobs_durable_<name>_fork.go` |
| CLI | `server/cmd/multica/cmd_<name>_fork.go` |
| Core | `packages/core/durable/<name>/` (schema zod, queries, mutations) |
| Views | `packages/views/durable/<name>/`, gắn vào `packages/views/governance/governance-page.tsx` bằng một dòng |
| Bản dịch | `packages/views/locales/<locale>/durable.ts` (mới) cho mọi locale có trong `packages/views/locales/` (en, es, fr, ja, ko, ru, vi, zh-Hans, zh-Hant) |

Mỗi hook vào file upstream được thêm vào hợp đồng `pnpm sync:contract` / `sync:coverage` trong cùng PR.

### 14.3 Quyền

- Mọi truy vấn lọc `workspace_id`. Membership chặn truy cập; `X-Workspace-ID` chọn workspace.
- Agent actor không bao giờ viết: envelope, ghim chế độ, chính sách hệ quả, uỷ quyền, profile của người, `auto_promote`, phát hành attestation. Mỗi điều có test 403.
- UUID theo [Backend UUID Rules](../../../AGENTS.md#backend-uuid-rules): tham số tài nguyên qua loader; UUID thuần qua `parseUUIDOrBadRequest`.
- Thứ tự khoá dòng: đường enqueue task `verification` và `shadow` dùng đúng đường enqueue có sẵn (agent rồi `lock_task_owner_rows`), không tự khoá theo thứ tự khác ([Row Lock Order](../../../AGENTS.md#row-lock-order)).

### 14.4 API và tương thích

- Mọi JSON giao diện dùng đi qua schema zod và `parseWithFallback` (`packages/core/api/schema.ts` hoặc schema cục bộ trong `packages/core/durable/`); enum lạ có giá trị dự phòng (`verdict` lạ → `unverified`, `effect_class` lạ → `write`, `decision` lạ → `confirm`).
- Mỗi endpoint mới có test phản hồi sai dạng.
- Capability của daemon: `DaemonCapabilityShadowV1` gửi qua header `X-Client-Capabilities`. Task bóng và danh sách skill bị bỏ (`omit_skills`) chỉ gửi cho daemon có capability này (D30). Không dựa vào trường lạ trong payload.
- Danh sách phân trang con trỏ, tối đa 100. Thân yêu cầu tối đa 16 KiB (32 KiB cho preview của ý định).
- Lỗi có `code` máy đọc được: `outcome_settled`, `verifier_is_doer`, `envelope_violation`, `effect_blocked`, `intent_not_pending`, `profile_consent_required`, `delegation_forbidden_kind`, `attention_reason_required`, `shadow_task`, `attestation_sample_too_small`.

### 14.5 Giao diện

- Dùng primitive có sẵn (`packages/ui`), icon từ `@multica/ui/icons`, token ngữ nghĩa, thang `--text-*`. Tương tác đổi bố cục theo `packages/ui/docs/motion.md`; PR giao diện đính GIF và kết quả `scripts/motion/capture.mjs`.
- Mô tả mặc định bỏ; chỉ viết khi có ràng buộc hoặc hệ quả không hiển nhiên (ví dụ câu "chỉ kiểm soát những gì đi qua nền tảng", câu đồng ý của profile).
- Wire cả web (`apps/web`) và desktop (router hoặc overlay) cho mọi tab mới.
- Server data ở TanStack Query với khoá có `wsId`; không đưa payload server vào Zustand. Sự kiện WebSocket mới (`outcome:*`, `effect_intent:*`, `attention:*`, `track_record:*`) chỉ invalidate.
- Duyệt và từ chối ý định, quyết định uỷ quyền: gắn toast vào promise của request (`mutateAsync().then`) vì thẻ Inbox có thể unmount.

### 14.6 Bảo mật

- Preview và lý do của agent hiện như Markdown trơ. Nội dung lấy từ bên ngoài trong briefing (hồ sơ ghế, trait) đánh dấu là dữ liệu, không phải chỉ dẫn.
- Gateway thực thi ý định đã duyệt kiểm `call_sha256` và trạng thái trong cùng giao dịch khoá dòng ý định (`FOR UPDATE`), để hai lần duyệt đồng thời không gọi hai lần.
- Attestation: chuẩn hoá RFC 8785 trước khi ký; từ chối khoá không thuộc host của `issuer`.
- Rà soát bảo mật trước khi bật mỗi cờ mặc định (`/security-review` trên nhánh).

### 14.7 Hiệu năng

- Hook ghi `issue_status_transition` là một `INSERT` trong giao dịch có sẵn.
- Chính sách hệ quả là hàm thuần trên dữ liệu đã cache (mức tự chủ squad cache 60 giây như budget gate); không truy vấn thêm ở đường nóng của gateway ngoài một `INSERT` vào `effect_event`.
- Job đêm phân trang theo workspace, không quét chéo workspace trong một giao dịch.

## 15. Giả định đã kiểm ở P0

P0 kiểm mười giả định khi viết (bằng chứng ở [p0-findings.md](p0-findings.md)). Mỗi kết luận được áp dụng bằng một quyết định trong mục 0.

| # | Kết luận P0 | Quyết định | Mục bị ảnh hưởng |
| --- | --- | --- | --- |
| U1 | Sai: 24 điểm ghi `issue.status`, không có hàm chung | D26: trigger PostgreSQL | 2.2 |
| U2 | Một phần: có `status_changed` nhưng không đủ để backfill đầy đủ | D31: backfill best-effort, cột `source` | 2.2 |
| U3 | Đúng: `internal/connectedapps` là điểm vào của tool connected apps | — | F3 |
| U4 | Sai: claim chỉ trả task cho runtime của chính agent | D27: agent bóng | 9.4 |
| U5 | Đúng: task tạm không claim worktree; runtime có quyền push phải bị từ chối | D27: không chạy `in_place` | 9.4 |
| U6 | Một phần: daemon cũ bỏ qua trường lạ nhưng vẫn chạy task bóng như việc thật | D30: capability | 9.4, 14.4 |
| U7 | Đúng: manifest flow thêm được trường đánh dấu hệ quả | — | B5 |
| U8 | Sai: không có sự kiện revert PR | D31 | 2.4 |
| U9 | Sai: runtime tuỳ chỉnh là binary CLI, không có backend HTTP | D29: adapter CLI | 12.3 |
| U10 | Đúng: có chỗ lưu bí mật, nhưng không có khoá sẵn cho attestation | D28: tắt khi thiếu khoá | 13.2 |

Còn chưa kiểm: dữ liệu thật của `activity_log` (U2, cần truy vấn chỉ đọc trên DB thật), luồng đầy đủ của PR auto-complete và channel action đến câu lệnh ghi cuối (thuộc A1), và hành vi thực tế của daemon cũ với trường `shadow` (U6, cần kiểm trên daemon thật).

## 16. Ngoài phạm vi

- Thanh toán, ký quỹ, hợp đồng liên tổ chức (F12 v1 dừng ở attestation).
- Mô hình hoá thành viên không phải owner hoặc admin; mô hình hoá người ngoài workspace (khách hàng, người nổi tiếng).
- Tự chạy lệnh kiểm chứng trên máy chủ (nền tảng không chạy mã của khách).
- Kiểm soát bên trong một lần chạy của runtime (giới hạn đã nêu ở Phase 4 4.3 vẫn đúng).
- Thay đổi ứng dụng mobile (`apps/mobile`): mobile tự quyết khi nào đọc các API mới; v1 chỉ bảo đảm API không phá client mobile.
- Gỡ hoặc viết lại skill, flow, harness có sẵn.

## 17. Kiểm thử và nghiệm thu tổng

### 17.1 Tầng test

| Tầng | Sở hữu |
| --- | --- |
| Go đơn vị (không DB) | Công thức Wilson, Newcombe, bảng chính sách 7×3×4, chọn mẫu tất định, ước tính phút, chuẩn hoá và ký attestation, chọn juror |
| Go có DB (`testutil`, `dbfx`) | Vòng đời kết quả, chuyển chế độ, envelope, ý định và thực thi, uỷ quyền, ledger, shadow cách ly, append-only. Khẳng định trên dòng của fixture, không trên tổng ([Testing](../../../AGENTS.md#testing)) |
| Core (vitest, node) | Schema zod và dự phòng enum, khoá query, helper định dạng |
| Views (vitest, jsdom) | Wiring tab, khối Nghiệm thu, thẻ ý định (unmount trước khi request xong), trang profile và đồng ý |
| E2E (Playwright) | Ba kịch bản ở 17.2 |

### 17.2 Kịch bản nghiệm thu tổng

1. **Một tuần của một squad (giai đoạn A):** squad L1 có envelope `[1, 2]` làm 12 việc có tiêu chí; 11 `pass`, 1 mở lại; chế độ lên `sampled`; squad chưa lên L2 vì thiếu mẫu; brief hiện đúng các con số.
2. **Một email ra khách (giai đoạn B và C):** agent L1 gửi email qua connected app; chính sách `rehearse`; ý định có preview; dự đoán "duyệt 90%"; người sửa một câu rồi duyệt; email gửi đúng bản sửa; ledger ghi 0.5 phút; hoàn tác không có (email `irreversible`) và giao diện nói vậy.
3. **Một model mới (giai đoạn D):** bật shadow cho một ghế với model ứng viên; 20 cặp; task bóng không tạo hệ quả nào ngoài `read`; báo cáo đủ số; proposal `runtime_change`; duyệt; probation bắt đầu.

### 17.3 Lệnh kiểm tra

| Phạm vi | Lệnh |
| --- | --- |
| Go | `make test` (hoặc `cd server && go test ./internal/service ./internal/handler ./internal/scheduler ./cmd/multica ./cmd/migrate ./internal/migrations` với DB test riêng) |
| Migration | `cd server && go test ./cmd/migrate ./internal/migrations`; `node scripts/check-migration-numbers.mjs --head <PR head>` trước khi merge |
| Frontend | `pnpm typecheck`, `pnpm lint`, `pnpm test` |
| E2E | `pnpm exec playwright test e2e/durable` |
| Tổng | `make check` |
| Sync | `pnpm sync:contract`, `pnpm sync:coverage` |
