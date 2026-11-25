# P0: xác nhận giả định và ghi nền

> Ngày: 2026-11-25. Nhánh: `durable-platform`. HEAD khi kiểm: `bb391344d7`.
> Đặc tả: [spec.md](spec.md) mục 15. Kế hoạch: [plan.md](plan.md) P0.1 và P0.2.
> Phạm vi: chỉ đọc mã và chạy kiểm tra. Không đổi mã sản phẩm, spec hay plan.

## Tóm tắt

| Mục | Kết luận |
| --- | --- |
| Nền | Tất cả lệnh xanh ở HEAD. Lệnh Go đỏ lần đầu là do DB test lệch migration, không phải mã. |
| U1 | **Sai**: 24 điểm ghi `issue.status`, không có hàm chung. |
| U2 | **Một phần**: có `status_changed` kèm `from`/`to`, nhưng không đủ để backfill toàn bộ. |
| U3 | **Đúng**: mọi lời gọi tool connected app đi qua `internal/connectedapps`. |
| U4 | **Sai**: claim chặn task chạy trên runtime khác của agent. Cần agent bóng. |
| U5 | **Đúng**, có lưu ý: có thư mục tạm không claim worktree, và chế độ `in_place` không cách ly. |
| U6 | **Một phần**: daemon bỏ qua trường lạ, nhưng daemon cũ sẽ chạy shadow như việc thật. Cần capability. |
| U7 | **Đúng** (có thể thêm trường); hiện **chưa có** trường đánh dấu hệ quả. |
| U8 | **Sai**: không có sự kiện revert PR. Chỉ có `done → cancelled`. |
| U9 | **Sai**: runtime tuỳ chỉnh là một binary CLI, không có worker HTTP. Cần adapter. |
| U10 | **Đúng**: có mẫu khoá từ biến môi trường (secretbox). Không có Ed25519 nào. |

Đúng: U3, U5, U7, U10. Một phần: U2, U6. Sai: U1, U4, U8, U9.

Cần quyết định trước giai đoạn A: cách bắt chuyển trạng thái issue (U1, xem mục "Đề xuất sửa spec" số 1). Các giả định còn lại không chặn A.

---

## Nền

### Lệnh và kết quả

| # | Lệnh | Kết quả | Số liệu |
| --- | --- | --- | --- |
| 1 | `pnpm catalog:build` | Xanh | 302 template, 0 file ghi (đã có sẵn) |
| 2 | `pnpm typecheck` | Xanh | 15/15 task (4 cache), khoảng 32 s |
| 3 | `pnpm --filter @multica/core exec vitest run governance` | Xanh | 12 file, 70 test đạt |
| 4 | `pnpm --filter @multica/views exec vitest run governance` | Xanh | 11 file, 60 test đạt |
| 5 | `go test ./internal/service -run 'Autonomy\|Scorecard\|ApprovalPolicy\|Governance\|Probation\|Budget' -count=1` trên DB test mới | Xanh | 44 PASS, 0 FAIL, 1 SKIP |

Ghi chú lệnh 5:
- SKIP là `TestBudgetGateAdmissionSpikeP99`, cần `MULTICA_BUDGET_SPIKE=1`. Không chạy vì là bài đo.
- Lần đầu chạy `go test` thẳng vào DB `multica_engine_test` (xem bên dưới) có 12 FAIL. Đã kiểm tra: lỗi là `column "handoff_mode" does not exist` và `column "usage_squad_id" does not exist`. Đây là lỗi môi trường, không phải hồi quy. Không có dòng rác `duplicate key` nào.
- Lần chạy thẳng thứ hai (`go test` với `-count=1` và DB test mới) vượt 900 giây mà không in output. Nghi do tranh chấp CPU và build cache với các phiên khác trên cùng máy; chưa xác định chắc. Đã dựng binary bằng `go test -c` (mất khoảng 2 phút) rồi chạy binary với cùng `-run`. Chạy binary xong trong khoảng một phút.
- Không chạy `make test` hay `make check`.

### Môi trường DB

- `.env` trỏ `DATABASE_URL` tới DB `multica`. Không chạy test trên DB đó.
- `multica_engine_test` (DB có sẵn, dùng cho flow engine) lệch migration: `schema_migrations` có 1115 dòng, file `server/migrations/*.up.sql` có 1138. Thiếu các migration 732 đến 753 và nhiều hơn nữa, gồm `747_squad_handoff_mode` và `735_task_usage_hourly_squad_col`. Có 7 dòng trong `schema_migrations` không khớp file nào trên nhánh này (ví dụ `917_squad_handoff_rule` đến `921_squad_handoff_target_issue_idx`, `122_task_handoff_note`). Không sửa DB này, vì nó không thuộc phạm vi P0.
- Đã tạo DB mới `multica_p0_test` (DSN lấy từ `.env`, đổi tên DB trong lệnh), rồi chạy `go run ./cmd/migrate up`. Kết quả: 1138 migration. Lệnh 5 chạy trên DB này. Có thể xoá bằng `DROP DATABASE multica_p0_test;` khi không cần nữa.
- Rủi ro: `server/internal/testutil/db_test.go:29-32` rơi về DB `multica` khi không đặt `DATABASE_URL`. Mọi lệnh Go có DB phải đặt `DATABASE_URL` rõ ràng.
- Lần chạy đầu vào `multica_engine_test` không để lại dòng fixture nào (đã kiểm: 0 workspace `testutil%`, 0 user `testutil%`).

### Đỏ sẵn ở HEAD

Không có lệnh nào đỏ ở HEAD sau khi có DB đúng. Lỗi đỏ ở lần chạy đầu là do môi trường, đã ghi ở trên.

---

## U1. Mọi đường đổi trạng thái issue đi qua một hàm service

**Kết luận: Sai.** Không có hàm chung. Có 24 điểm gọi trực tiếp sqlc `UpdateIssue` hoặc `UpdateIssueStatus` (không tính test). Một điểm ghi bằng lệnh gọi trong `cmd/server`, ngoài service và handler.

**Bằng chứng:**
- Hai query ghi trạng thái: `pkg/db/queries/issue.sql:270` (`UPDATE issue AS i SET ... status = changed.next_status`, dùng cho `UpdateIssue`) và `issue.sql:306-313` (`UpdateIssueStatus`).
- Lệnh: `rg -n '\.(UpdateIssue|UpdateIssueStatus)\(' server/internal cmd --glob '!*_test.go'` trả 24 dòng, theo file:
  - `internal/service/proposal.go` 5 (680 đến 1166)
  - `internal/handler/flow_engine.go` 5 (290, 316, 404, 891, 1165)
  - `internal/service/initiative.go` 3 (508, 521, 893)
  - `internal/handler/issue_description_collab.go` 2 (454, 617)
  - mỗi file 1: `internal/service/task.go`, `internal/service/qoder_issue_lifecycle.go`, `internal/service/proposal_expiry.go`, `internal/service/issue_public.go`, `internal/handler/issue_move.go`, `internal/handler/issue.go`, `internal/handler/flow_engine_ops.go`, `internal/handler/flow_api_run.go`
  - **`cmd/server/runtime_sweeper.go:726`**: `queries.UpdateIssueStatus(...)` ghi thẳng, không qua service, không phát sự kiện, không ghi activity.
- Đường PR auto-complete: `internal/handler/pr_auto_complete.go:251-284` (`maybeAutoCompleteIssue`). Chưa lần theo hết luồng tới câu lệnh ghi.
- Đường channel action: `internal/handler/issue.go` là file có cả nhắc `channel` và lệnh ghi. Chưa lần theo hết.

**Tác động lên plan.md:**
- A1 ghi "một dòng `recordStatusTransition` mỗi đường". Vẫn đúng về cách làm, nhưng danh sách phải có 24 điểm, không phải "vài đường". Phải thêm `cmd/server/runtime_sweeper.go` vào danh sách file sửa của A1. Đường này không có sự kiện, nên hook theo sự kiện sẽ bỏ sót.
- Test `TestEveryStatusWriterRecordsOneTransition` phải có một ca cho sweeper (ca này sẽ đỏ nếu chỉ hook service).
- Cần quyết định cách làm (xem "Đề xuất sửa spec" số 1).

---

## U2. activity_log không có lịch sử trạng thái đáng tin; nếu có thì backfill được

**Kết luận: Một phần.** Có dữ liệu trạng thái, nhưng có khoảng trống và chưa kiểm được dữ liệu thật.

**Bằng chứng:**
- Schema: `migrations/001_init.up.sql:156-165`. Bảng có `action TEXT`, `details JSONB`, không có cột `from`/`to` riêng.
- Listener ghi trạng thái: `cmd/server/activity_listeners.go:90-91` dựng `details = {"from": prev_status, "to": issue.Status}`; dòng 111 ghi `Action: "status_changed"`. Đây là đường đáng tin cho các thay đổi đi qua bus sự kiện.
- Ghi thủ công cùng action: `internal/service/qoder_issue_lifecycle.go:70` (`Action: "status_changed"`).
- Khoảng trống: writer ghi thẳng SQL (`cmd/server/runtime_sweeper.go:726`) không phát sự kiện nên không có dòng activity. `prev_status` có thể rỗng (`from` là chuỗi rỗng).
- Không có chính sách xoá theo thời gian. Chỉ `workspace_delete.sql:375` xoá activity theo workspace.
- **Chưa kiểm được**: số dòng và ngày cũ nhất trong dữ liệu thật. Quy tắc không cho đọc DB `multica`. Truy vấn để người dùng chạy (chỉ đọc): `SELECT count(*), min(created_at) FROM activity_log WHERE action = 'status_changed' AND details ? 'from' AND details->>'from' <> '';`

**Tác động lên plan.md:**
- Backfill chỉ là ước tính: chỉ phủ các chuyển đã có sự kiện. Đánh dấu dòng backfill bằng cột nguồn (ví dụ `source = 'activity_backfill'`) và đặt khoá `history_partial` (đã có trong A5) cho issue thiếu lịch sử.
- A1 cần migration backfill riêng, có test trên dòng fixture, không khẳng định tổng.

---

## U3. Gateway internal/connectedapps là điểm duy nhất của lời gọi tool connected app

**Kết luận: Đúng.** Mọi lời gọi thực thi tool connected app đi qua `internal/connectedapps`.

**Bằng chứng:**
- Composio: `internal/connectedapps/gateway.go:512` và `internal/connectedapps/requests.go:463` gọi `s.composio.ExecuteTool`. Đây là hai nơi gọi duy nhất. Định nghĩa là `internal/integrations/composio/team.go:379`, không có lời gọi nào ngoài `connectedapps`.
- Remote MCP: `internal/connectedapps/remote.go:112` (`remotemcpUpstream.CallTool`), gọi từ `remote_gateway.go` (`remoteRun`).
- Điểm vào HTTP: `internal/handler/connected_apps.go:1500-1506` (`AppsGateway` → `ServeHTTP`) và `internal/handler/connected_apps_remote.go:28` (`ServeRemoteApp`).

**Lưu ý, không ảnh hưởng U3:**
- Plugin hook gọi ra ngoài qua `internal/service/plugin_hook.go:197` (`InvokeHook`). Spec mục 4.5 đã liệt kê nguồn `plugin_action` tách khỏi `connected_app`, nên không phải lỗ của F3 ở nguồn `connected_app`.

**Tác động lên plan.md:** không cần đổi. Test cho `connected_app` nên khẳng định cả hai đường (Composio và remote) cùng đi qua một hook.

---

## U4. Daemon chạy được một task trên runtime khác runtime của agent (cho shadow) mà không cần agent thứ hai

**Kết luận: Sai.** Claim chỉ trả task cho runtime của chính agent. Shadow cần agent bóng, đúng như spec đã ghi làm phương án dự phòng.

**Bằng chứng:**
- `pkg/db/queries/agent.sql:893` (`ClaimAgentTask`): điều kiện `a.runtime_id = atq.runtime_id`, kèm chú thích "A task's persisted runtime is not authority after an agent rebind". Task chỉ được nhận khi runtime của task đúng là runtime hiện tại của agent. Đồng thời yêu cầu runtime `online`.
- Enqueue gán `RuntimeID: agent.RuntimeID`: `internal/service/task.go:1387`, `1564`, `1770`, `2215`.
- Xác thực phía claim: `internal/handler/daemon.go:3067` (`claimResponseAgentIdentityMatches`): chỉ kiểm `resp.Agent.ID == resp.AgentID`.

**Tác động lên plan.md:**
- F8 (giai đoạn D) phải tạo agent bóng: một agent ẩn, cùng workspace, trỏ tới runtime ứng viên. Không đổi `ClaimAgentTask` vì đó là đường claim nhạy cảm về race (xem Row Lock Order trong AGENTS.md).
- Thêm vào D-stage: thứ tự khoá (agent trước, rồi `lock_task_owner_rows`) theo quy tắc enqueue.

---

## U5. Daemon có thể chạy task trong thư mục tạm không claim worktree

**Kết luận: Đúng**, với lưu ý: chế độ `in_place` không cách ly.

**Bằng chứng:**
- Thư mục làm việc bắt đầu rỗng khi không có repo: `internal/daemon/execenv/execenv.go:451` ("The workdir starts empty (no repo checkouts)").
- Mỗi task có thư mục tạm riêng: `internal/daemon/daemon.go:9093` (`ensureTaskTempDir`), định nghĩa tại `daemon.go:11520-11548`.
- Local directory có hai chế độ: `in_place` (mặc định) và `worktree`. Chỉ `worktree` mới được cách ly: `internal/daemon/local_directory.go:27-33` (hằng), `:58` (`UsesWorktree()` chỉ đúng khi `execution_mode == worktree`).
- `in_place` chạy thẳng trong thư mục người dùng, có mutex theo đường dẫn (`internal/daemon/daemon.go:6526`), không tách bản sao.

**Tác động lên plan.md:**
- Shadow chạy trong thư mục tạm, không cần worktree. Đúng như spec, nhưng cách ly yếu.
- Thêm vào D-stage: shadow không được gắn vào local directory `in_place`; runtime có quyền push bị từ chối cho shadow. Test: `TestShadowRejectsInPlaceDirectory`.

---

## U6. Payload claim có chỗ cho cờ `shadow` và danh sách skill bị bỏ mà không đổi giao thức cũ

**Kết luận: Một phần.** Daemon cũ bỏ qua trường lạ, nhưng bỏ qua không có nghĩa là an toàn. Daemon cũ sẽ chạy task shadow như task thật.

**Bằng chứng:**
- Daemon giải mã bằng `json.NewDecoder(...).Decode(...)` không có `DisallowUnknownFields`: `internal/daemon/client.go:1516`, `1550`, `1584`. Không có `DisallowUnknownFields` trong `internal/daemon`. Trường lạ bị bỏ qua.
- Đã có tiền lệ cho hai dạng claim skill: `internal/handler/daemon_skill_load_failure_test.go:97-112` (claim skill-refs gọn và claim inline cũ).
- Đã có cơ chế capability: hằng `DaemonCapability*V1` trong `pkg/protocol/messages.go:6-77`; server đọc header `X-Client-Capabilities` (`internal/handler/daemon.go:2148-2150`, `requestHasClientCapability`).
- Đã có cổng theo phiên bản cho worktree: `internal/handler/daemon.go:4583` trả `error_worktree_daemon_version` khi daemon thiếu `local-worktree-v1`.

**Tác động lên plan.md:**
- Với F8 và F9, không dựa vào cờ `shadow` trong payload. Thêm hằng `DaemonCapabilityShadowV1` (tên đề xuất, `shadow-v1`), và server từ chối dispatch task shadow tới daemon không quảng bá capability đó. Theo đúng mẫu `error_worktree_daemon_version`.
- Thêm test: daemon không có capability không nhận shadow (không có trường `shadow` nào được gửi tới daemon đó).

---

## U7. Manifest flow có hoặc thêm được trường đánh dấu bước có hệ quả ngoài

**Kết luận: Đúng về khả năng thêm. Hiện chưa có trường đánh dấu.**

**Bằng chứng:**
- Schema flow: `internal/issueworkflow/flowdoc.go:284-297` (`DocStep`: `Key`, `Title`, `Status`, `Kind`, `Tasks`, `Join`, `Lead`, `ExitRole`, `Notice`, `Edges`, `Layout`). Không có trường hệ quả.
- `flowdoc.go:318-332` (`DocTask`): `Key`, `Title`, `Kind`, `Role`, `Instances`, `After`, `When`, `Verdicts`, `Requires`, `Inputs`, `Sets`, `Skills`, `Deadline`, `Instructions`. Không có trường hệ quả.
- Kiểm tra manifest: `internal/issueworkflow/flowdoc_validate.go` và `flowdoc_validate_rules.go`. **Không có** `server/internal/handler/flow_manifest_rules.go` như plan.md viết.
- Catalog: `catalog/workflows/<tên>/template.json`, khối `designer.steps[]` có `statusKey` và `transitions` (đã kiểm bước đầu tiên). Không có trường hệ quả.

**Tác động lên plan.md:**
- F4 mục 5.4 chỉ còn khai báo thủ công, đúng như spec đã ghi làm dự phòng. Thêm trường vào `DocTask` là khả thi (decoder không chặt, và validate là chỗ thêm quy tắc).
- Sửa đường dẫn trong plan.md P0.2 (xem "Đề xuất sửa spec" số 7).

---

## U8. Có sự kiện VCS cho revert PR

**Kết luận: Sai.** Không có sự kiện nào cho revert PR. `fail_cause = reverted` chỉ suy ra được từ `done → cancelled`.

**Bằng chứng:**
- `rg -n -i revert` trong `internal/handler/github.go`, `vcs.go`, `vcs_webhook.go`, `internal/integrations`, `internal/vcs`: không có kết quả nào về revert PR.
- `internal/handler/github.go:1519-1561` (`handlePullRequestEvent`): chỉ xử lý `closed` với `Merged` (dòng 1561). Dòng 1994 xử lý `opened`, `synchronize`, `reopened`.
- `internal/handler/pr_auto_complete.go:230` (`case "closed"`).

**Tác động lên plan.md:**
- A2 không đổi. Ánh xạ `fail_cause` giữ như spec.
- Nếu muốn nhận diện revert, chỉ có thể suy đoán từ tiêu đề PR có dạng `Revert "..."`. Đây là heuristic, không phải sự kiện. Cần ghi nhãn "suy đoán" trong spec 2.5 nếu làm.

---

## U9. Runtime tuỳ chỉnh hiện có đủ cho worker HTTP bên ngoài

**Kết luận: Sai.** Runtime tuỳ chỉnh bọc một binary CLI nói một giao thức họ có sẵn. Không có backend HTTP.

**Bằng chứng:**
- `pkg/agent/agent.go:447-480` (`SupportedTypes`): danh sách CLI (claude, codex, pi, kiro, ...). Không có họ HTTP.
- `pkg/agent/agent.go:538-596` (`New`): mỗi case dựng backend cho một CLI. Không có case HTTP.
- `pkg/agent/builtin_runtimes.go:18-32`: `BuiltinRuntime` có `ProtocolFamily` và `DefaultCommand` (lệnh CLI trên PATH).
- `pkg/agent/pi.go:1313`: "Custom runtime profiles only promise Pi's JSON protocol". `pkg/agent/opencode_v2.go:50`: runtime tuỳ chỉnh "wraps a binary".

**Tác động lên plan.md:**
- F11 cần một adapter tham chiếu: một binary nhỏ nói giao thức JSON của một họ có sẵn (ví dụ pi), và gọi worker HTTP bên ngoài. Plan F11 phải có task viết adapter này, không chỉ cấu hình runtime.

---

## U10. Có chỗ lưu bí mật máy chủ cho khoá Ed25519

**Kết luận: Đúng**, với một điểm cần quyết định.

**Bằng chứng:**
- Không có Ed25519 nào trong code server (`rg -i ed25519` ngoài test: 0 kết quả).
- Đã có mẫu khoá từ biến môi trường với `secretbox`: `MULTICA_PLUGIN_SECRET_KEY` (`internal/service/plugin.go:34-46`, `plugin_hook.go:455-458`, `plugin_mcp_transport.go:258`); `internal/research/service.go:44-58` (`NewBox`); `MULTICA_APPS_SECRET_KEY` (`cmd/server/router.go:1534`).
- Mẫu hai kiểu khi thiếu khoá: plugin **tắt tính năng** (`plugin_hook.go:455`), còn JWT **không cho khởi động** (`cmd/server/main.go:338`, `jwtSecretBootError`).

**Tác động lên plan.md:**
- F12 dùng biến môi trường bắt buộc. Cần chọn: fail-fast như JWT, hay tắt tính năng như plugin. Vì F12 sau cờ mặc định tắt, đề xuất tắt tính năng khi thiếu khoá (xem "Đề xuất sửa spec" số 6).

---

## Đề xuất sửa spec

Không có sửa nào được áp dụng trong P0. Mỗi đề xuất cần người dùng xác nhận trước khi sửa spec hoặc plan.

1. **U1 (cần quyết định trước giai đoạn A).** Chọn một trong hai cách ghi chuyển trạng thái:
   - (a) Hook tường minh tại 24 điểm, qua một hàm `recordStatusTransition`. Dễ đọc, nhưng dễ bỏ sót điểm mới.
   - (b) Trigger PostgreSQL `AFTER UPDATE OF status ON issue` ghi `issue_status_transition`. Bắt được cả đường SQL thô như sweeper. Actor lấy từ biến session (`SET LOCAL`) do ứng dụng đặt.
   - Đề xuất: (b) làm lớp bảo đảm, kèm test tĩnh liệt kê các điểm ghi. Cần xác nhận vì trigger là thay đổi kiến trúc.
2. **U2.** Spec mục 2.2: ghi rõ backfill chỉ là một phần, dùng `source` và `history_partial`.
3. **U4.** Spec mục 9.4: agent bóng là bắt buộc, không còn là dự phòng. Ghi tên và cờ ẩn của agent bóng.
4. **U5.** Spec mục 9.4: shadow không chạy trên `in_place`; runtime có quyền push bị từ chối (đã có ý này, cần ghi rõ).
5. **U6.** Spec mục 14.4: shadow và danh sách skill được gate bằng capability `X-Client-Capabilities` (đề xuất `DaemonCapabilityShadowV1`). Không dựa vào trường lạ trong payload.
6. **U10.** Spec mục 13.2: chọn tắt tính năng khi thiếu khoá (đề xuất), hoặc fail-fast như JWT. Tên biến đề xuất `MULTICA_DELEGORA_ATTEST_KEY` (cần xác nhận; không phải tên đã có).
7. **plan.md P0.2, đường dẫn sai.** `server/internal/handler/daemon_start_claim.go` và `worktree_claim_gate.go` không tồn tại. Claim nằm trong `server/internal/handler/daemon.go` (`ClaimTasksByRuntime` ở dòng 2281, `buildClaimedTaskResponse` ở 3100). `server/internal/handler/flow_manifest_rules.go` không tồn tại; dùng `server/internal/issueworkflow/flowdoc_validate*.go`. Các đường dẫn `server/pkg/agent/...` còn lại trong plan.md đúng.
8. **U8.** Spec mục 2.5 (hoặc chỗ ánh xạ `fail_cause`): ghi rõ không có sự kiện revert; chỉ `done → cancelled`.
9. **U9.** Spec mục 12.3: F11 cần adapter CLI tham chiếu, không phải worker HTTP trực tiếp.
10. **U7.** Spec mục 4.5 và 5.4: khai báo thủ công là đường duy nhất vì chưa có trường đánh dấu.

---

## Chưa kiểm được

- Dữ liệu thật của `activity_log` (U2). Cần người dùng chạy truy vấn chỉ đọc ở mục U2.
- Luồng đầy đủ của PR auto-complete (`pr_auto_complete.go`) và channel action. Đã xác định file, chưa lần theo đến câu lệnh ghi cuối. Việc này thuộc A1.
- Chạy `make test`, `make check`, `pnpm lint`, Playwright: không chạy trong P0 theo yêu cầu.
- Không kiểm cách daemon cũ xử lý trường `shadow` thực tế (U6). Mới suy ra từ việc giải mã không chặt và từ mẫu `error_worktree_daemon_version`.
