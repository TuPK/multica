# Vận hành platform sau R1: hướng dẫn cho nhà vận hành

Trạng thái: **R1 đã viết xong, chưa qua cổng G-R1.** Tài liệu này mô tả các điều khiển có sau R1 và trước khi có console. Nguồn: [platform-management-spec.md](engineering/platform-management-spec.md) và [platform-management-plan.md](engineering/platform-management-plan.md) (bước P12). Trích dẫn dạng `tệp:dòng`, đường dẫn tính từ gốc repo.

Ngôn ngữ: tiếng Việt. Tên tệp, bảng, cột, lệnh, mã lỗi giữ tiếng Anh.

Mọi câu lệnh SQL dưới đây chạy bằng `psql` trên cơ sở dữ liệu của máy chủ Multica. `<uid>` là UUID người dùng (`"user".id`), `<ws>` là UUID workspace.

## 1. Khóa và mở tài khoản bằng SQL

Bảng `platform_account` (`server/migrations/651_platform_account.up.sql:1`) có tối đa một dòng cho mỗi người dùng (chỉ mục duy nhất `server/migrations/652_platform_account_user_idx.up.sql:1`). Người dùng không có dòng được coi là `active` (`platform-management/accountguard/accountguard.go:34`). Vì vậy mọi lệnh ghi dùng `INSERT ... ON CONFLICT`, giống `SetAccountState` và `RevokeSessionsForUser` (`server/pkg/db/queries/platform_account.sql:6`, `:18`).

Các lệnh SQL **không ghi `audit_event`**. Dấu vết duy nhất là `reason`, `changed_by`, `changed_at` của dòng `platform_account`. `changed_by` là UUID người vận hành, hoặc `NULL` nếu người đó không có tài khoản.

### 1.1 Khóa (suspend)

```sql
INSERT INTO platform_account (user_id, status, reason, changed_by, changed_at)
VALUES ('<uid>', 'suspended', '<lý do>', '<operator uid>', now())
ON CONFLICT (user_id) DO UPDATE SET
    status = 'suspended',
    reason = EXCLUDED.reason,
    changed_by = EXCLUDED.changed_by,
    changed_at = now()
WHERE platform_account.status IN ('active', 'suspended');
```

Câu `WHERE` giữ cho lệnh không đè lên tài khoản `pending_deletion` hoặc `deleted`. Kết quả phải là `INSERT 0 1`; `INSERT 0 0` nghĩa là tài khoản đang ở một trạng thái xóa, xem mục 3.

Người dùng thấy: mọi request, với mọi loại token, nhận **403 `account_suspended`** (`server/internal/middleware/auth.go:94`; áp cho mọi loại token: `:133`). Trạng thái lạ cũng bị xử như `suspended` (`:92`). Khóa không thu hồi phiên: khi mở lại, JWT cũ dùng được trở lại, trừ khi đặt thêm mốc ở mục 1.3.

### 1.2 Mở (reinstate)

```sql
UPDATE platform_account
SET status = 'active', reason = '<lý do>', changed_by = '<operator uid>', changed_at = now()
WHERE user_id = '<uid>' AND status = 'suspended';
```

Kết quả phải là `UPDATE 1`. Chỉ mở từ `suspended`: một dòng `pending_deletion` mở bằng tay sẽ để lại `deletion_scheduled_at` treo.

### 1.3 Buộc đăng xuất mọi nơi

```sql
INSERT INTO platform_account (user_id, status, changed_at, sessions_valid_after)
VALUES ('<uid>', 'active', now(), date_trunc('second', now()))
ON CONFLICT (user_id) DO UPDATE SET
    sessions_valid_after = date_trunc('second', now());
```

Chỉ cột `sessions_valid_after` đổi, như `RevokeSessionsForUser`. Phải cắt về giây: so sánh theo giây nguyên, token cấp đúng tại mốc vẫn sống (`server/internal/auth/session.go:223`). Handler API cũng cắt về giây (`server/internal/handler/me_sessions.go:60`).

Người dùng thấy: JWT cấp trước mốc nhận **401 `session_revoked`** và bị xóa cookie (`server/internal/middleware/auth.go:386`, `:101`). Token PAT không bị ảnh hưởng, vì không phải phiên (`server/internal/realtime/hub.go:823`). Lệnh này chỉ có tác dụng khi tài khoản `active`; tài khoản bị khóa nhận 403 trước.

### 1.4 Độ trễ và xóa cache

Guard đọc trạng thái qua cache Redis với TTL 60 giây (`platform-management/accountguard/accountguard.go:23`). Cả "không có dòng" cũng được cache (`:74`, `:88`), nên khóa một người chưa có dòng cũng chờ tối đa 60 giây. Handler API tự xóa cache sau khi ghi (`server/internal/handler/me_sessions.go:92`); **SQL thì không**. Muốn có hiệu lực ngay, xóa khóa sau khi chạy SQL:

```bash
redis-cli DEL mul:auth:acct:<uid>
```

Thêm tùy chọn kết nối (`-h`, `-p`, `-a` hoặc `-u`) theo Redis của hệ thống. Tên khóa lấy từ `platform-management/accountguard/accountguard.go:25` và `:63`. Nếu máy chủ chạy không có Redis thì không có cache và mọi thay đổi có hiệu lực ngay (`server/internal/auth/account_guard.go:67`). Việc xóa phải làm **sau** khi SQL đã commit.

### 1.5 WebSocket

SQL **không ngắt** WebSocket đang mở. Máy chủ chỉ kiểm tài khoản lúc kết nối hoặc lúc nhận thông điệp xác thực đầu tiên (`server/internal/realtime/hub.go:880`, `:926`). Kết nối đang mở vẫn sống cho đến khi tự rớt; khi nối lại, nó bị từ chối bằng 403 `account_suspended` hoặc 401 `session_revoked` (`:826`). Ngắt kết nối (`Hub.DisconnectUser`, `server/internal/realtime/hub.go:606`) chỉ được gọi từ ba đường API: đăng xuất mọi thiết bị (`server/internal/handler/me_sessions.go:98`), yêu cầu xóa (`server/internal/handler/me_deletion.go:244`) và job xóa (`:457`). Muốn ngắt ngay ở R1: khởi động lại máy chủ Multica (hub nằm trong bộ nhớ, ngắt mọi người dùng), và chấp nhận ảnh hưởng đó.

## 2. Đọc nhật ký kiểm toán bằng SQL

Bảng `audit_event` (`server/migrations/654_audit_event.up.sql:1`). Các cột: `id`, `workspace_id` (NULL với sự kiện cấp tài khoản), `actor_type` (`member`, `agent`, `system`, `staff`), `actor_id`, `actor_label`, `action`, `target_type`, `target_id`, `target_label`, `detail` (jsonb), `ip`, `user_agent`, `request_id`, `created_at`. Tên `action` ở `platform-management/audit/audit.go:14`.

Sự kiện của một người dùng trong một workspace, một hành động:

```sql
SELECT created_at, action, actor_type, actor_id, actor_label,
       target_type, target_id, target_label, detail, ip, request_id
FROM audit_event
WHERE workspace_id = '<ws>'
  AND action = '<ví dụ: member.removed>'
  AND (actor_id = '<uid>'
       OR (target_type IN ('user', 'member') AND target_id = '<uid>'))
ORDER BY created_at, id;
```

Người đó là tác nhân khi `actor_id = '<uid>'`, là đối tượng khi `target_id = '<uid>'` (hàng `member.*` lưu UUID người dùng, `server/internal/handler/workspace.go:939`). Chỉ mục: `(workspace_id, created_at DESC, id)` và `(actor_id, created_at DESC)` (migration 656, 657). Bỏ điều kiện `workspace_id` để xem cả sự kiện cấp tài khoản.

`actor_label` và `target_label` là **email tại lúc ghi**, không đổi theo hồ sơ sau này. Khi tài khoản bị xóa, job đổi nhãn thành `deleted user` cho hàng có `actor_type = 'member'` và `actor_id` là người đó, và hàng có `target_type = 'user'` và `target_id` là người đó (`server/pkg/db/queries/audit_event.sql:45`, `:51`; gọi ở `server/internal/handler/me_deletion.go:436`). Hàng job tự ghi lúc xóa dùng sẵn nhãn `deleted user` (`:27`). Không sửa `audit_event` bằng tay. Job `audit_retention` xóa hàng quá `AUDIT_RETENTION_DAYS` (mặc định 365, tối thiểu 30; `server/internal/scheduler/jobs_audit_retention.go:20`).

## 3. Xử lý yêu cầu xóa tài khoản

**Chờ 14 ngày** (`platform-management/accountdeletion/rules.go:7`, không cấu hình được). Khi người dùng gửi yêu cầu (`POST /api/me/deletion`, cần đăng nhập trong 10 phút), tài khoản thành `status = 'pending_deletion'` với `deletion_scheduled_at = now() + 14 ngày`, mọi phiên bị thu hồi, WebSocket bị ngắt, có email xác nhận (`server/internal/handler/me_deletion.go:203`, `:244`). Yêu cầu bị từ chối với 409 `sole_owner` nếu người đó là owner duy nhất của workspace còn thành viên khác (`:165`).

**Hủy.** Trong lúc chờ, người dùng đăng nhập; tài khoản chỉ gọi được `GET /api/me`, `DELETE /api/me/deletion` và `POST /auth/logout` (`server/internal/middleware/auth.go:57`). `DELETE /api/me/deletion` đặt lại `active`, `reason = 'deletion_canceled'`, `deletion_scheduled_at = NULL` và giữ mốc thu hồi phiên (`server/pkg/db/queries/account_deletion.sql:35`).

**Xem yêu cầu đang chờ:**

```sql
SELECT pa.user_id, u.email, pa.changed_at AS requested_at, pa.deletion_scheduled_at,
       pa.deletion_scheduled_at <= now() AS due
FROM platform_account pa
JOIN "user" u ON u.id = pa.user_id
WHERE pa.status = 'pending_deletion'
ORDER BY pa.deletion_scheduled_at;
```

Dòng có `due = true` mà vẫn còn là yêu cầu đã quá hạn và job chưa xử lý xong.

**Job `account_deletion`** chạy mỗi 24 giờ (`server/internal/scheduler/jobs_account_deletion.go:13`, `:33`; đăng ký ở `server/cmd/server/main.go:883`). Mỗi lần xử lý tối đa 50 tài khoản đến hạn (`platform-management/accountdeletion/rules.go:16`), mỗi tài khoản trong giao dịch riêng, từ cũ nhất (`server/pkg/db/queries/account_deletion.sql:46`). Với mỗi tài khoản, job gỡ tư cách thành viên, xóa workspace mà người đó là thành viên duy nhất, xóa dữ liệu cá nhân, ẩn danh hóa bản ghi `user` và nhãn kiểm toán, rồi đặt `deleted` (`server/internal/handler/me_deletion.go:394`, `:414`). Chạy lại là an toàn.

**Chủ workspace còn thành viên khác.** Nếu sau khi yêu cầu, người đó vẫn là owner duy nhất của workspace còn thành viên khác, job báo lỗi cho tài khoản đó và để nó ở `pending_deletion` (`server/internal/handler/me_deletion.go:472`). Job thử lại mỗi ngày. Nó chỉ xong khi workspace có thêm owner khác hoặc không còn thành viên khác. Người dùng đang chờ không dùng được ứng dụng để chuyển quyền, nên họ hủy yêu cầu, chuyển quyền rồi gửi lại. Workspace đã xử lý trước đó của cùng tài khoản đã commit riêng và không hoàn lại (`:359`).

**Yêu cầu job không làm xong.** Không sửa dòng bằng tay. Đọc log của máy chủ theo tên:

- `account deletion` mức Info, kèm `due`, `deleted`, `failed` (`server/internal/scheduler/jobs_account_deletion.go:75`).
- `account deletion failed; the account stays pending and the next run retries`, mức Error, kèm `user_id` và `error` (`:70`).

Một tài khoản lỗi không làm job báo lỗi: `runAccountDeletions` trả `nil` (`:76`), nên chỉ log mới cho thấy. Lọc log theo `account_deletion` để thấy cả lần chạy của scheduler. Chỉ sửa nguyên nhân được nêu trong `error`, rồi đợi lần chạy sau.

## 4. Giới hạn đã biết trong R1

Nguồn: commit `ffd327db0` ("Known gaps"). Commit nói các giới hạn này đã ghi trong spec, nhưng tôi không tìm thấy chúng trong `platform-management-spec.md`.

- Hàng kiểm toán có đối tượng là thành viên (`member.*`) hoặc lời mời (`invitation.*`) vẫn giữ email trong `target_label`, kể cả sau khi xóa tài khoản (`server/internal/handler/workspace.go:939`, `server/internal/handler/invitation.go:193`). Job chỉ đổi nhãn của hàng `target_type = 'user'`.
- Phiên chat trong workspace dùng chung vẫn còn (commit `ffd327db0`).
- Tài khoản bên ngoài (dịch vụ ngoài mà người dùng kết nối) không bị gỡ khỏi dịch vụ của chúng (commit `ffd327db0`).
- Chưa có console. Khóa, mở, đăng xuất, đọc nhật ký làm bằng SQL, và các lệnh SQL không ghi `audit_event`.
- SQL không ngắt WebSocket đang mở (mục 1.5), nên AC1 chưa đạt trọn bằng SQL.

## 5. Cổng G-R1: trạng thái

Nguồn tiêu chí: spec mục 13. Cột "Đã chạy" chỉ ghi kết quả đã thấy. Test cần cơ sở dữ liệu bỏ qua (skip) khi không có `PLATFORM_TEST_DATABASE_URL`, nên mục đó **không** được tính là đã chạy.

| AC | Test hoặc kiểm tra | Đã chạy |
| --- | --- | --- |
| AC1 | Bảng quyết định guard: `server/internal/middleware/account_guard_test.go:173`, `:220`. WebSocket từ chối khi nối: `server/internal/realtime/hub_test.go:777`, `:829`. Cache: `platform-management/accountguard/accountguard_test.go:54`. Đọc `platform_account`: `server/internal/auth/account_guard_test.go:16` | Phần không cần DB: đã chạy, pass (middleware, realtime, accountguard). `auth/account_guard_test.go`: not run: no `_test` database provided (G0). Phần "WebSocket bị ngắt" của AC1 chưa đạt bằng SQL (mục 1.5) |
| AC2 | `server/internal/middleware/account_guard_test.go:308`, `:347`, `:407`. WebSocket: `server/internal/realtime/hub_test.go:894`, `:936`. Phiên vừa gọi vẫn dùng được: `server/internal/handler/me_sessions_test.go:92` | Middleware và realtime: đã chạy, pass. `me_sessions_test.go`: not run: no `_test` database provided (G0) |
| AC3 | Danh mục hành động: `platform-management/audit/catalog_test.go:6`. Một hàng mỗi hành động: `server/internal/handler/audit_events_test.go:174` đến `:420`. Lỗi ghi kiểm toán hoàn tác thay đổi: `:469`. Ghi trong giao dịch: `server/internal/auditstore/store_test.go` | `catalog_test.go`: đã chạy, pass. Các test còn lại: not run: no `_test` database provided (G0) |
| AC4 | `server/cmd/server/invitations_route_test.go:15` (qua router thật) | not run: no `_test` database provided (G0) |
| AC5 | 409 `sole_owner`: `server/internal/handler/me_deletion_test.go:113`. Job ẩn danh hóa và gỡ: `:357`. Không để workspace mất owner: `:493`. Quy tắc thuần: `platform-management/accountdeletion/rules_test.go:43`. Sổ đăng ký cột: `server/internal/migrations/user_columns_registry_test.go:37` | Quy tắc thuần và sổ đăng ký cột: đã chạy, pass. `me_deletion_test.go`: not run: no `_test` database provided (G0). Việc đăng ký lại bằng email cũ không có test riêng; `:377` chỉ kiểm email bị đổi thành địa chỉ không gửi được |
| AC6 | Quét bí mật: `platform-management/accountexport/accountexport_test.go:41`, `:50`, `:60`. Nội dung bản xuất: `server/internal/handler/me_export_test.go:66` | `accountexport_test.go`: đã chạy, pass. `me_export_test.go`: not run: no `_test` database provided (G0) |

Chưa chạy trong lần viết tài liệu này: toàn bộ test của `./internal/handler`, `./internal/auditstore`, `./internal/auth`, `./cmd/server`; `pnpm typecheck`, `lint`, `test`; `go vet`; phép đo p95 của guard (spec 15.3). Cổng G-R1 chưa qua cho đến khi các mục "not run" được chạy trên cơ sở dữ liệu test.
