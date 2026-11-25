# Giao thức worker (ocf-worker/1)

Tài liệu này mô tả các thao tác một worker bên ngoài cần để làm việc trong một Delegora workspace, và ánh xạ từng thao tác sang endpoint hoặc lệnh `multica` đã có. Đây là đặc tả của F11 (spec 12.3) và quyết định D29 (spec mục 0): worker là **adapter CLI tham chiếu** chạy như một runtime tuỳ chỉnh, không phải một backend HTTP.

Giao thức không thêm endpoint nào cho v1. Nó là tài liệu hoá có phiên bản của những gì đã có, cộng các lệnh do chương trình durable thêm (`multica effect declare` của F3).

**Phiên bản:** `ocf-worker/1`. Thay đổi tương thích (thêm trường tuỳ chọn, thêm lệnh) giữ `/1`. Thay đổi làm hỏng adapter đang chạy tạo `/2`.

## 1. Mô hình

```
  Delegora server  <--HTTPS, mdt_ token-->  daemon  --spawn-->  adapter CLI  --> worker (tiến trình bên ngoài)
                   <--HTTPS, mat_ token---------------------------------------- multica CLI (trong adapter)
```

- **Daemon** là bên duy nhất nói chuyện với endpoint `/api/daemon/*` bằng token máy (`mdt_`). Daemon đăng ký runtime, claim task, nhận sự kiện từ tiến trình con, và báo kết quả.
- **Adapter CLI** (`multica-ocf-adapter`, mới, E2) là một tiến trình do daemon khởi chạy cho mỗi task. Nó bọc worker bên ngoài và nói theo giao thức của runtime tuỳ chỉnh.
- **Lệnh `multica`** mà adapter (hoặc worker) gọi dùng token task `mat_`, được cấp cho đúng một task và hết hạn cùng task.

Adapter không mở endpoint HTTP và không giữ token máy. Nó không cần biết HTTP.

### Điều kiện để chạy như runtime tuỳ chỉnh

Runtime tuỳ chỉnh không thêm giao thức mới. Hồ sơ runtime (`custom runtime profile`) phải chọn một `protocol_family` mà Delegora đã hỗ trợ, và lệnh phải tương thích với họ đó ([daemon-runtimes](../../../../apps/docs/content/docs/daemon-runtimes.mdx), mục "Custom runtime profiles"):

- Daemon chỉ chấp nhận `protocol_family` nằm trong `SupportedTypes` (`server/pkg/agent/agent.go`, dòng 447). Kiểm tra ở `server/internal/daemon/daemon.go`, quanh dòng 3201.
- Họ đó quyết định cách daemon gọi lệnh (tham số, stdin) và cách đọc đầu ra (luồng sự kiện). Adapter phải nói đúng cách đó.

Adapter chọn họ `pi`. Đã kiểm bằng `TestAdapterRunsUnderThePiBackend` (`server/cmd/multica-ocf-adapter/adapter_pi_test.go`): adapter được dựng và chạy qua chính backend `pi` của daemon, gồm lệnh (`-p --mode json --session <path>` cộng `custom_args`), prompt trên stdin, và luồng sự kiện (`agent_start`, `message_update` với `text_delta`, `error`, `agent_end` có `stopReason`). Daemon chấp nhận họ `pi` cho runtime tuỳ chỉnh (`agent.RuntimeProtocolFamily`).

Chi tiết cần biết khi cấu hình: `--worker <lệnh>` phải nằm trong `custom_args` của agent. Backend `pi` chỉ đưa `custom_args` vào argv (`server/pkg/agent/pi.go`, `buildPiArgs`), không đưa tham số cố định của hồ sơ runtime. Đặt worker bằng biến môi trường `MULTICA_OCF_WORKER` chưa được kiểm qua daemon.

## 2. Biến môi trường

Adapter và worker nhận các biến từ daemon. Năm biến được đánh dấu **integration contract** trong tài liệu runtime là phần có thể dựa vào:

| Biến | Dùng cho |
| --- | --- |
| `MULTICA_TOKEN` | Token `mat_` của task. Mọi lệnh `multica` dùng nó. |
| `MULTICA_TASK_ID` | Task đang chạy. |
| `MULTICA_AGENT_ID` | Agent được giao. |
| `MULTICA_WORKSPACE_ID` | Workspace của task. |
| `MULTICA_SERVER_URL` | URL server do daemon chọn. |

Các biến khác (`MULTICA_TASK_CONFIG_ROOT`, `MULTICA_DAEMON_PORT`, `MULTICA_TASK_SLOT`, ...) là thông tin, có thể đổi. Không dựa vào chúng.

Cảnh báo: với runtime tuỳ chỉnh, mọi biến đi thẳng vào môi trường tiến trình, và tiến trình con kế thừa `MULTICA_TOKEN`. Adapter phải xoá token khỏi môi trường của tiến trình con không cần nó. Token không được đi vào prompt, log, file trong repo hay cấu hình lâu dài.

## 3. Thao tác và ánh xạ

| # | Thao tác | Ai thực hiện | Endpoint hoặc lệnh hiện có | Trạng thái |
| --- | --- | --- | --- | --- |
| 1 | Đăng ký runtime | daemon | `POST /api/daemon/register`: `workspace_id`, `daemon_id`, `device_name`, `cli_version`, `runtimes[]` | Có. Adapter không gọi. |
| 2 | Nhịp tim | daemon | `POST /api/daemon/heartbeat` | Có. |
| 3 | Nhận task (claim) | daemon | `POST /api/daemon/runtimes/{runtimeId}/tasks/claim`, hoặc batch `POST /api/daemon/tasks/claim` | Có. Adapter không claim. |
| 4 | Bắt đầu | daemon | `POST /api/daemon/tasks/{taskId}/start` | Có. |
| 5 | Nhận prompt | daemon → adapter | Theo họ đã chọn (mục 1): tham số hoặc stdin | Phụ thuộc họ. |
| 6 | Báo tiến độ, sự kiện | adapter → daemon → server | Adapter in sự kiện theo họ; daemon gửi `POST /api/daemon/tasks/{taskId}/messages` | Có. Adapter không gọi endpoint này trực tiếp. |
| 7 | Báo xong | daemon | `POST /api/daemon/tasks/{taskId}/complete`: `output`, `pr_url`, `session_id`, `work_dir`, `branch_name` | Có. Nội dung lấy từ sự kiện kết thúc của họ. |
| 8 | Báo lỗi | daemon | `POST /api/daemon/tasks/{taskId}/fail`: `error`, `failure_reason`, `session_id`, `work_dir`, `branch_name` (`server/pkg/protocol/messages.go`, dòng 241) | Có. |
| 9 | Báo usage | daemon | `POST /api/daemon/tasks/{taskId}/usage` | Có. Tuỳ chọn. |
| 10 | Thao tác trong workspace | adapter hoặc worker | `multica <lệnh>` với `MULTICA_TOKEN` | Có. Ví dụ comment, issue, criteria. |
| 11 | Khai báo hệ quả ngoài nền tảng (F3) | adapter hoặc worker | `multica effect declare --class <c> --reversibility <r> --target <t> --summary <s>` | **Chưa có** trên nhánh chính. F3 định nghĩa lệnh và endpoint. |
| 12 | Ghi kiểm chứng (F1) | worker trong task kiểm chứng | `multica outcome verify --outcome <id> [--criterion <id>] --result pass\|partial\|fail\|unable [--severity] [--note] [--evidence type:ref]` → `POST /api/outcomes/{id}/verifications` | Có (`server/cmd/multica/cmd_outcome_fork.go`, `server/cmd/server/router_fork.go` dòng 277). |

Ghi chú theo từng dòng:

- **Dòng 5 và 6.** Prompt và tiến độ không có endpoint riêng cho runtime tuỳ chỉnh. Họ đã chọn đưa prompt vào, và luồng sự kiện đưa tiến độ ra. Daemon đọc luồng đó và đăng qua `messages`. Vì vậy adapter không cần token máy.
- **Dòng 7 và 8.** Adapter không tự gọi `complete` hay `fail`; daemon làm việc đó. Khi adapter thoát mà không có sự kiện kết thúc, daemon báo `fail` (giả định, xem mục 4).
- **Dòng 10.** Lệnh `multica` của task đọc `MULTICA_TOKEN`, và server gắn hành động với agent và task của token. Adapter không cần truyền id agent hay task.
- **Dòng 11.** Lệnh là thoả thuận, không phải hàng rào (D11). Khai báo không ngăn được hành động; chính sách chỉ trả lời `allow`, `confirm`, `rehearse` hoặc `block`. Giao diện phải ghi rõ "đã khai báo", không phải "đã kiểm soát".
- **Dòng 12.** Đầu vào của `--outcome` và `--criterion` là id. Server lấy id task kiểm chứng từ token `mat_` (`server/internal/handler/durable_outcome_fork.go`, `TaskID: caller.task`), không từ thân request. Một worker không kiểm chứng được ngoài task kiểm chứng mà nó được giao. Thông điệp `--help` nói "platform sends the task id with the request": đó là cách hiểu chưa chính xác, nên sửa khi làm F1 xong.

## 4. Lệnh và mã thoát

- Lệnh `criteria` và `outcome` thoát khác 0 khi server trả 4xx hoặc 5xx (`cmd_outcome_fork.go`). Các lệnh khác chưa kiểm.
- `multica effect declare` (F3) thoát theo quyết định: `0` cho `allow`, `3` cho `confirm` hoặc `rehearse`, `4` cho `block`. Đầu ra là JSON `{ "event_id", "decision", "intent_id" }`. Đây là đặc tả của F3, chưa có mã.
- Adapter thoát `0` khi sự kiện kết thúc hợp lệ đã được in. Thoát khác 0 thì backend `pi` ghi kết quả `failed` và giữ thông báo lỗi. Đã kiểm bằng `TestAdapterRunsUnderThePiBackend` (worker thoát `3`). Chưa kiểm: việc daemon chuyển kết quả đó thành lệnh `fail` trên server (cần chạy daemon đầy đủ).

## 5. Vòng đời của một task

```
daemon: claim → start → spawn adapter (env + prompt)
adapter: chạy worker, in sự kiện theo họ
          ├─ sự kiện tiến độ  → daemon: messages
          ├─ lệnh multica      → server (với mat_)
          └─ sự kiện kết thúc  → daemon: complete (output, pr_url, session_id, ...)
          hoặc lỗi / thoát bất thường → daemon: fail
```

Hủy task: server có endpoint `POST /api/daemon/tasks/{taskId}/cancel-ack` (`AckTaskCancelled`). *Giả định, chưa kiểm:* daemon dừng tiến trình con khi task bị hủy, và adapter phải thoát khi nhận tín hiệu dừng. Cần đối chiếu với mã daemon khi làm E2.

## 6. Giới hạn của v1

- Không có backend HTTP worker. Adapter là tiến trình con của daemon, không phải dịch vụ.
- Lệnh `effect declare` và endpoint của nó thuộc F3. Trước khi F3 có mặt, worker không khai báo được hệ quả; kết quả đó phải nói rõ trong báo cáo.
- Adapter không tự đăng ký nhiều runtime. Một hồ sơ runtime tuỳ chỉnh là một cặp (lệnh, họ).
- Adapter không tiếp tục phiên. Tham số `--session` do backend `pi` truyền vào bị bỏ qua, nên mỗi task chạy worker mới, không nhớ task trước.

## 7. Chưa rõ

1. Hủy task (mục 5) chưa đối chiếu với mã daemon. Adapter dừng worker khi context bị huỷ, nhưng nếu daemon chỉ giết adapter bằng SIGKILL thì worker có thể còn chạy. Cần kiểm trên daemon thật.
2. Có cần `multica effect declare` trong v1 của worker, hay để F3 quyết định. Đặc tả hiện tại để F3 quyết.
3. Thông điệp `--help` của `multica outcome verify` về nguồn id task (mục 3, dòng 12).
