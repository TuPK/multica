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

# Vận hành platform sau R2: hướng dẫn cho nhà vận hành (sections 6 to 12)

Status: **R2 is written. Gate G-R2 database tests pass (2026-11-16); items still open are in section 12.**

Language of sections 6 to 12: English, as set by the R2 task contract. Sections 1 to 5 stay in Vietnamese. File names, tables, columns, commands and error codes are the same in both parts.

Sources: [platform-management-spec.md](engineering/platform-management-spec.md) (5.1, 5.3, 5.8, 5.9, 11, 12), [platform-management-plan.md](engineering/platform-management-plan.md) (P30, G-R2) and the code in `platform-management/`. Where the code differs from the spec, this part follows the code and says so. Paths are from the repository root.

## 6. Startup checklist

Spec 5.1. The platform checks its own settings at start and exits with a message naming the variable. Two checks used to run in Multica and cannot run there now, so make them before each deploy. Values are never written in this file; keep them in the secret store.

**Multica server**

| Check | Expected | Note |
| --- | --- | --- |
| `REDIS_URL` | set | `docker-compose.selfhost.yml:73` defaults it to empty. |
| `MULTICA_DEV_VERIFICATION_CODE` | not set | `docker-compose.selfhost.yml:117` passes it through; leave it empty. |
| `APP_ENV` | `production` | The compose file defaults to `production` (`:116`). |

**Platform service** (`platform-management/internal/config/config.go`)

| Check | Expected | Failure |
| --- | --- | --- |
| `DATABASE_URL`, `REDIS_URL` | set; `REDIS_URL` is a `redis://`, `rediss://` or `unix://` URL | exit, variable named (`:137`, `:139`) |
| `PLATFORM_APP_URL` | origin of the Multica web app, such as `https://app.example.com`, no path | exit (`:217`) |
| `PLATFORM_SESSION_SECRET` | at least 32 bytes | exit (`:155`) |
| `PLATFORM_ADMIN_EMAILS` | one or more plain email addresses, comma-separated | exit (`:146`) |
| Email provider | Resend (`RESEND_API_KEY` and `RESEND_FROM_EMAIL`) or SMTP (`SMTP_HOST` and `SMTP_FROM_EMAIL`) | exit (`:319`) |
| `PLATFORM_DEV_VERIFICATION_CODE` | not set | exit when `APP_ENV=production` (`:246`) |
| Migrations | every version in `platform-management/internal/schemacheck` present | exit, missing versions listed (`cmd/platform-server/main.go:96`) |

- Each address in `PLATFORM_ADMIN_EMAILS` needs a Multica account. An address without one is logged and skipped (`main.go`, staff seeding). If none matches, nobody can sign in to the console.
- The email provider also carries the overdue notices. Without one, the `subscription_lifecycle` job is not registered and no subscription moves to the next overdue state (`main.go:239`). The platform logs `lifecycle notices are off: no email provider is configured`.
- Check after start: `GET /healthz` on the console port (30001) answers 200 when the database and Redis respond.

**Bank details (decision D49).** `BILLING_BANK_ACCOUNT_NAME`, `BILLING_BANK_ACCOUNT_NUMBER` and `BILLING_BANK_NAME` may be empty. The platform then starts and logs `billing: bank details are empty` (`main.go:61`). Before the first real payment:

1. Set all three. The order page reads them from the **Multica server** environment (`server/internal/handler/billing_orders.go:88`). The platform only uses them for the warning. `docker-compose.selfhost.yml` does not pass them to the server (see section 12).
2. Open an order page as a customer and check that all three fields show.

Until both are done, do not accept real orders. D49 is open (section 12).

### 6.2 Staff two-step sign-in (decision D51)

Every staff member signs in to the console with the email code and then a six-digit code from the authenticator app they enrolled in the Multica web app. The session records `mfa` in its `amr` claim, and the console refuses a session without it. A staff member with no confirmed second factor cannot sign in. The recovery codes of the Multica web app are not accepted at the console.

The platform reads `MULTICA_MFA_SECRET_KEY`, the same variable and value as the Multica server, because it opens the secrets that Multica sealed. Without it the platform exits at start. At start it also opens one confirmed `user_mfa` row; if the key does not open it, the platform exits with a message naming `MULTICA_MFA_SECRET_KEY`. Set the same value in both services.

**Rollout order** (do not start the new platform before step 3 is done for every staff member):

1. Set `MULTICA_MFA_SECRET_KEY` in the Multica server and in the platform to the same value, and deploy Multica.
2. Each staff member, including every address in `PLATFORM_ADMIN_EMAILS`, signs in to the Multica web app and turns on two-step sign-in. Check: `SELECT u.email FROM platform_staff s JOIN "user" u ON u.id = s.user_id WHERE s.revoked_at IS NULL AND NOT EXISTS (SELECT 1 FROM user_mfa m WHERE m.user_id = s.user_id AND m.confirmed_at IS NOT NULL);` returns no rows.
3. Deploy the platform with this change.

**Reset from the console.** A `support` or `admin` member opens the user in the console and chooses "Reset two-step sign-in". The `user_mfa` row is deleted, the audit event `user.mfa_reset` names the staff member and the user, and the user is emailed. The user is not signed out and signs in with the email code alone until they enrol again. A staff member cannot reset their own (`self_reset_refused`).

**Lockout recovery.**

- A staff member who lost their phone but has a recovery code: sign in to the Multica web app with the recovery code, turn two-step sign-in off with another recovery code, enrol again, then sign in to the console.
- A staff member with neither: another `support` or `admin` member resets them from the console, then they enrol again in the Multica web app.
- The first admin when no other staff member can sign in, for example because the rollout order was not followed: run this on the shared database, then enrol again in the Multica web app and sign in to the console. The change has no audit row, so write the reason in your change log.

```sql
DELETE FROM user_mfa
WHERE user_id = (SELECT id FROM "user" WHERE email = 'admin@example.com');
```

Until the user enrols again they cannot sign in to the console, because a staff member with no confirmed second factor is refused.

## 7. Bank transfer reconciliation

Who: staff with the `billing` or `admin` role. Other roles do not get the Orders screen.

The customer pays by bank transfer and writes the **transfer code** of the order in the transfer note. The code is `MUL` followed by eight characters from `A-Z` and `2-9` without `I` and `O` (`platform-management/internal/billing/transfer.go:10`). Each code belongs to one order.

1. Take the bank statement for the period.
2. In the console, open **Orders** and stay on the **Pending** tab. The list shows the oldest pending order first, at most 200 (`internal/console/orders.go:16`). There is no search box; compare the code by eye, and use the copy button next to the code.
3. For each incoming transfer, find the order with the same code. Check that the transferred amount equals **Total (incl. VAT)**, which is the net amount plus the VAT amount, in whole VND.
4. If they match, press **Confirm**. The dialog asks for a reason (required) and a bank reference (optional). Confirm does the following in one transaction: sets the order `paid`, updates the subscription, returns the workspace to `active` if it was `read_only` because of an overdue subscription, and writes the audit event `billing.order_paid` with the staff member's identity.
5. Confirming an order that is already paid changes nothing and reports `applied: false`.

The system does not store the reason or the bank reference of a confirmation (`internal/console/orders.go:176`; the audit row carries the order, amounts and transfer code). Keep the bank reference with the bank statement.

A pending order expires 7 days after it was created (`internal/billing/status.go:21`); an hourly job closes it. Confirming it after that fails with `order_expired` (409). Other refusals: `order_not_pending` (the order is canceled or expired), `order_not_applicable`, `order_conflict` (the subscription changed meanwhile; try again).

### 7.1 When the amount differs

Confirm has no amount field. It applies the whole order, and the system has no partial payment and no refund route. So the amount check in step 3 is the only check. Do not confirm an order whose transfer does not equal the total.

| Case | Action |
| --- | --- |
| Transfer is less than the total | Do not confirm. Leave the order pending, write a tenant note with the amounts and date, and contact the buyer at the invoice email shown in the list. When the transfers that carry this code add up to the total, confirm. If the buyer prefers to start again, cancel the order with a reason (**Cancel**, pending orders only, cannot be undone) and the buyer creates a new one. |
| Transfer is more than the total | Confirm: the order is paid in full. Return the excess outside the system and write a tenant note. |
| Transfer arrives after the order expired | Confirm is refused. Return the money, or ask the buyer to create a new order. Write a tenant note. |
| No order has the code, or the code is mistyped | Do not guess a match. Leave the transfer unmatched and contact the sender. |

How an underpayment is collected, whether any tolerance applies, and how refunds are made and recorded are not in the spec. They are open items for the project owner (section 12).

## 8. Overdue subscriptions

Spec 5.9. The `subscription_lifecycle` job runs in the platform every hour (`platform-management/internal/lifecycle/lifecycle.go:34`). A step happens at the first run at or after its boundary.

| State | Starts | Duration | Workspace |
| --- | --- | --- | --- |
| `active` | purchase or payment | to `current_period_end` | `active` |
| `past_due` | `current_period_end` passes without renewal | **7 days**: `grace_until` = `current_period_end` + 7 days | `active`, full function |
| `read_only` | `grace_until` | **30 days**, counted from `grace_until` (`edges.go:82`) | `read_only`, reason `billing` |
| `free` | 30 days after `grace_until` | until the next purchase | `active`, free-plan limits |

So the free plan starts 37 days after the period end. Data is kept; only creation above the free-plan limits is blocked. Other edges: a subscription with `cancel_at_period_end` goes to `free` at the period end with no grace period, and a `trialing` subscription goes to `free` at `trial_end`.

The owners and admins get an email at each change of state, and a reminder 14, 7 and 1 days before the period end. Each email is recorded in `billing_notice`, so none is sent twice.

**A payment ends the sequence at any step.** Confirming the order (section 7) makes the subscription `active`, sets the new period, clears the grace period, and returns a workspace that is `read_only` for `billing` to `active`.

### 8.1 Overriding a step

| Goal | How | Role |
| --- | --- | --- |
| Open or close the workspace | Tenants, the tenant, **Change status**, choose `active`, `read_only` or `suspended`, with a reason | `support`, `admin` |
| Move a boundary | `PUT /api/platform/tenants/{id}/subscription` with a later `grace_until` (delays `read_only`, and the `free` step with it, because it is counted from `grace_until`) or a later `current_period_end` | `billing`, `admin` |
| End the overdue state | Confirm the order | `billing`, `admin` |

Rules to know:

- The job leaves staff decisions alone. It does not make a `suspended` or `pending_deletion` workspace `read_only`, and it does not reopen a workspace that is `read_only` for a reason other than `billing` (`lifecycle.go:274`).
- Setting a workspace `active` by hand during an overdue period holds only until the next boundary. When grace ends, the job closes an `active` workspace again. To hold longer, move the date.
- Staff cannot set the subscription's own state (`past_due`, `read_only`, `free`). A step cannot be undone except by a payment.
- The `PUT` replaces every field of the subscription. A field left out of the body is cleared, including the period dates, `grace_until`, the cancel flag and the limit overrides (`internal/subscription/console.go:216`). A subscription with no `grace_until` is not moved by the job. The console's **Edit subscription** dialog sends only plan, seats, interval, trial end and reason (`console/src/api/hooks.ts:541`), so saving it on an overdue subscription clears its dates. Do not use the dialog on an overdue subscription. Open item in section 12.
- Each change writes `subscription.changed` or `workspace.status_changed` to the audit log.

## 9. Locking a tenant

A tenant is locked by setting its workspace state to `suspended`. Use it for a violation or a legal request (spec 5.3). It is not for non-payment; overdue subscriptions use `read_only` (section 8).

**Effect** (`server/internal/workspaceguard/decide.go`, spec 5.3):

- Members cannot read or write the workspace. Requests answer 403 `workspace_suspended`.
- Agents receive no new tasks. Queued tasks stay queued; tasks already running finish.
- Autopilot stops creating runs. It can take up to 5 minutes (spec 5.3).
- Still allowed: owners and admins use the subscription pages, owners export workspace data, and members can leave the workspace.
- If the state cannot be read, requests answer 503 `workspace_state_unavailable`.
- The console clears the cache key when it writes, so Multica sees the change at once. The cache lifetime of 60 seconds is the upper limit (`platform-management/workspacestate/workspacestate.go:23`).

**Lock:** console, **Tenants**, the tenant, **Change status**, **Suspended**, enter the reason, confirm. Roles `support` and `admin`. The change writes `workspace.status_changed` to the workspace audit log, which the tenant's owners can read.

**Restore:** the same dialog, choose **Active**, enter the reason. A restore changes only the workspace state. Check the **Subscription** panel afterwards: if the subscription is overdue, section 8 applies. Moving a workspace out of `pending_deletion` needs the `admin` role.

Locking one person is separate: Users, the user, status (R1 controls, section 1; the user then gets 403 `account_suspended`).

## 10. Order list for the accountant

The console has an order list and no export file.

- **Orders** has one tab per status: pending, paid, expired, canceled, refunded. The default is pending. One status shows at a time, at most 200 rows. Pending is oldest first; the other tabs are newest first (`internal/console/orders.go:112`). There is no date filter and no paging.
- For the accountant, use the **Paid** tab.
- The table shows: workspace name and id, transfer code, plan, interval and seats, total with net and VAT, buyer invoice email and tax code, status, expiry.
- The same list as JSON (`GET /api/platform/orders?status=paid`, `billing` and `admin` roles) also carries: order id, kind, currency, `created_at`, `paid_at`, buyer name and buyer address. Amounts are whole units of the currency; `amount` is before VAT and the total is `amount` + `vat_amount`.

Spec 5.8 says staff export the paid orders for e-invoicing and record the invoice number in `einvoice_ref`. Neither exists in this checkout: there is no export route or button, and no code writes `einvoice_ref`. The column is read into the order record (`platform-management/internal/billing/order.go:56`), but the console does not show it. Until they exist, the accountant works from the Paid tab or the JSON, and keeps the invoice numbers outside the system. Open item in section 12.

## 11. Rollout order

1. Back up the database.
2. Run the migrations first: `server/cmd/migrate up` against the shared database. The Multica image carries it as `migrate` (`Dockerfile:38`). Migrations only add, so the older Multica and platform keep working on the newer schema (spec 12).
3. Deploy Multica.
4. Deploy the platform. At start it compares `schema_migrations` with its required list (the last entry is `690_platform_usage_daily_uidx`) and **refuses to start when a version is missing**, naming the versions. Run it with `restart: unless-stopped` so that it starts by itself once the migration is done. Publish only the console port 30001; the API port 8095 stays on the internal network (spec 11, 12).
5. Point `MULTICA_CLOUD_URL` at `http://platform:8095` only after the rehearsal in section 12 and after every existing workspace has a transitional subscription set by staff (spec 12).

`docker-compose.selfhost.yml` has no `platform` service in this checkout (section 12).

## 12. Open gates

**G-R2 database tests: passed, 2026-11-16.** Test database: `multica_platform_test` (migrated to head), through `scripts/require-test-database.sh`. Every package below was run with `-p 1` and `-count=1`.

| Package and pattern | Result |
| --- | --- |
| server `./internal/handler`: `BillingOrder\|CloudSubscription\|Order\|Plans\|WorkspaceDeletion\|WorkspaceExport\|AuditR2\|SeatCapacity\|Invitation\|ShareLink\|WorkspaceDeletionManifest\|WorkspaceTeardown` | pass |
| server `./internal/handler`: `PlanLimit\|WorkspaceCreation\|Signup\|Audit`; `WorkspaceState\|ClaimRefusal\|Seat\|ShareLink` | pass |
| server `./internal/scheduler`, `./internal/usagesnapshot`, `./internal/workspaceexport`, `./internal/workspacedeletion`, `./internal/workspaceguard`, `./internal/middleware`, `./internal/seatcapacity`, `./internal/entitlement`, `./internal/auditaction` | pass |
| server `./internal/service` (full package, and `-run 'Entitlement\|IssueLimit\|AutopilotQuota\|WorkspaceState\|ClaimRefusal'`) | pass |
| server `./cmd/server` | pass |
| server `./internal/integrations/...` | pass, except the `lark` package (pre-existing, below) |
| platform `./...` (every package, with `PLATFORM_TEST_DATABASE_URL` set to the same DSN as `DATABASE_URL`) | pass |

Fixes made in R2 slices to reach this, on `main`: the deletion manifest and the teardown graph (`ba3ae01ff`); the connect audit test setup (`0eef0e2bc`); the usage snapshot database tests, which no longer create a database (`b663b6ea5`); the workspace state matrix, which matches only state refusals (`047a73c3e`); the WeCom and DingTalk install fixtures, which seed the installer (`acda220d3`); the platform notes visibility scan, which allows the three teardown files to delete the table (`b168b3a21`); the Mika plan-limit test context (`676198be9`); and the non-member audit read status, 404 (`385bf2843`).

**Pre-existing, not R2.** `server/internal/integrations/lark`: `TestLegacyReceiptBackfillIsolatedFromSharedDatabase`, `TestLegacyReceiptBackfillEvidenceAndIdempotence`, `TestLegacyReceiptBackfillBatchLimit`. They use `testutil.IsolatedDB`, which creates a database named `multica_test_<id>` that does not end in `_test`; the create timed out and a later run was killed. Not changed.

**Gate status.** G-R2 meets the database-test criterion of the contract. The items below that the tests do not cover remain open, and the R1 database tests of section 5 are still open. P30's full-suite and [T], [PG], [PW], [PB], [PI] checks and the AC7 to AC14 and AC20 walk-through (spec 13) are not recorded here.

**Open after the tests.** Buyer details of billing orders are not cleared (a retention worker must do this, per the tax and retention rules). Storage objects of workspace exports are not deleted by the teardown. Leftover `multica_test_*` databases from earlier `IsolatedDB` runs remain on the test server and need an owner decision to drop.

**G-R2 cutover rehearsal, 2026-11-16: closed.** The backfill command is `platform-management/cmd/platform-backfill` (commit `b12ccfa2c`). It gives each workspace with no `workspace_subscription` row the free plan, one batch of 500 per transaction, with a `subscription.backfilled` audit row in the same transaction. It is safe to run again.

The rehearsal ran on `multica_platform_test`, through `scripts/require-test-database.sh`. That database held no workspace rows, so the rehearsal used four fixture workspaces, `rehearsal-1` to `rehearsal-4`: three with no subscription row, and one with a paid row (plan `pro`, version 2). The fixtures and their audit rows were deleted after the checks.

The rehearsal ran twice. The first run failed the policy check: the test database's plan table held only `free`, because `plan.Seed` filled the table only when it was empty. The fix is `b8b0ab763`: Seed now inserts each catalogue plan whose key is missing, unless staff deleted it, and never changes an existing row. The second run, below, is the record. The first run's paid fixture used the plan key `team`, which is not in the catalogue; the catalogue's paid plans are `pro`, `business` and `enterprise`, so the fixture now uses `pro`.

Second run, with the fix, Redis on 127.0.0.1:16379 (a `redis:7-alpine` container started for the rehearsal and removed afterwards), and the platform on 8095:

| Count | Result |
| --- | --- |
| Workspaces with no subscription row, before | 3 of 4 |
| First backfill run: batches, inserted | 1 batch, 3 inserted |
| Workspaces with no subscription row, after | 0 |
| `subscription.backfilled` audit rows after the first run | 3 |
| Second backfill run: batches, inserted | 0 batches, 0 inserted |
| `subscription.backfilled` audit rows after the second run | 3 |
| Paid-row fixture after the runs | unchanged (plan `pro`, version 2) |

| Check | Result |
| --- | --- |
| `migrate up` on the test DSN (first run) | at head: 925 migration files and 925 rows before and after |
| Backfilled rows: `free`, `active`, 0 seats, `month`, version 1, no period, trial or grace date (first run) | pass |
| Platform on 8095, `/healthz` | pass: HTTP 200, `{"status":"ok"}` |
| Entitlement policy for each of the four fixtures | pass: rehearsal-1 to -4 all HTTP 200 |
| Platform refuses to start when a migration is missing (first run) | pass: exit 1, `missing required migrations: 690_platform_usage_daily_uidx`; the row was restored exactly |
| No workspace read-only, suspended or not active because of the cutover | pass: 0 `platform_workspace` rows among the fixtures |
| Command guard refuses a name without `_test` | pass: exit 1 before any connection |
| Port 8095 unreachable from outside; `mcn_` token refused | not run; outside the checks of this rehearsal |

After the platform started, the plan table of the test database held `business`, `enterprise`, `free` and `pro`. The platform suite passed with the test DSN and without it, after the Seed fix. One run of the console package failed `TestDBOverviewCountsWorkspacesByPlan` during a parallel run: the test compares global counts, and another package's test may create or delete a workspace in between. It passed in the two later isolated runs and in the next full run.

G-R2 is closed: the policy check passes for all four fixtures, and `/healthz` returns 200.

Open items:

- `docker-compose.selfhost.yml` has no `platform` service (section 11).
- Paid workspaces need a plan set by hand; the backfill never assigns one.
- The production run is the owner's: `platform-backfill --allow-production`, with the production `DATABASE_URL`.

Deviation from the contract: the audit detail is `{"plan": "free", "source": "cutover"}`, not `plan_key`. `audit.Record` redacts keys that contain `key`.

**Decision D49 open.** Bank details are empty by decision of the project owner. Real payments cannot start until `BILLING_BANK_*` are set and the order page shows them (section 6).

**Other items before launch** (plan P30): G-LIC (commercial license, spec 0), the items of spec 16, database backup, monitoring. Their state is not recorded in the sources I read.

**Gaps found in the tooling while writing this part:**

- No order export and no `einvoice_ref` write (section 10).
- The console's **Edit subscription** dialog clears dates on a subscription (section 8.1).
- `docker-compose.selfhost.yml` has no `platform` service, and does not pass `BILLING_BANK_*` to the Multica server (sections 6 and 11).
- Rules for underpayment, overpayment and refunds are not in the spec (section 7.1).

## 13. Bảng kiểm trước khi bật R5

Nguồn: mã trong `server/migrations/`, `platform-management/internal/schemacheck/`, `platform-management/internal/config/config.go` và `server/internal/handler/`. Đường dẫn tính từ gốc repo. Mục này chỉ ghi điều đã đọc trong mã; điều chưa kiểm được ghi rõ.

### 13.1 Migration

Migration R5 có số từ 953 đến 969 trong `server/migrations/`.

1. Sao lưu cơ sở dữ liệu (mục 11, bước 1).
2. Chạy lệnh migrate của Multica **trước khi** khởi động lại Multica và platform:

```bash
cd server && go run ./cmd/migrate up
```

3. Lệnh này áp cả dải 953 đến 969. Hai nhóm không nằm trong danh sách kiểm của platform vẫn do lệnh này áp:
   - 956 đến 960 tạo bảng nhóm tài khoản (`member_group`, `member_group_member`) và chỉ mục của chúng. Chỉ máy chủ Multica đọc các bảng này.
   - **969** (`billing_order.expires_at` cho phép NULL) thuộc R5. Platform **không** kiểm migration này lúc khởi động (`schemacheck.go` không liệt kê nó), nên platform vẫn khởi động khi thiếu 969. Khi đó việc lập hóa đơn theo mức dùng không ghi được: cột còn `NOT NULL` (`677_billing_order.up.sql:23`), còn `usage.Invoicer` ghi đơn không có `expires_at` (`platform-management/internal/usage/invoicing.go:144`). Hãy kiểm `schema_migrations` có dòng `969_billing_order_usage_expiry` sau khi chạy lệnh.

### 13.2 Platform từ chối khởi động khi thiếu migration

Lúc khởi động, platform so `schema_migrations` với danh sách `Required` trong `platform-management/internal/schemacheck/schemacheck.go`. Thiếu một phiên bản thì nó thoát, với thông báo `database schema is missing required migrations:` kèm các phiên bản thiếu.

Các phiên bản R5 trong danh sách:

| Phiên bản | Bảng hoặc cột |
| --- | --- |
| `953_plan_subscription_r5` | cột R5 của `plan` và `workspace_subscription` |
| `954_billing_profile`, `955_billing_profile_uidx` | `billing_profile` |
| `961_usage_charge`, `962_usage_charge_uidx`, `963_usage_charge_order_idx` | `usage_charge` |
| `964_plan_price_change`, `965_plan_price_change_scheduled_uidx` | `plan_price_change` |
| `966_platform_account_request`, `967_platform_account_request_id_uidx`, `968_platform_account_request_email_uidx` | `platform_account_request` |

Tức là 953, 954, 955 và 961 đến 968. Phần trước đó của danh sách là các phiên bản R1 và R2 (651 đến 690).

### 13.3 Thứ tự khởi động lại

1. Chạy migrate (13.1).
2. Khởi động lại máy chủ Multica.
3. Khởi động lại dịch vụ platform.

### 13.4 Biến môi trường R5 của platform

Platform đọc các biến này trong `platform-management/internal/config/config.go`. Thư mục `config/` không có tệp `account.go` hay `usage.go`; các biến nằm cả trong `config.go`. Bỏ trống thì dùng mặc định. Giá trị sai (không phải số nguyên từ 1 trở lên, hoặc không phải `true` hay `false`) làm platform thoát với thông báo nêu tên biến.

| Biến | Mặc định | Ý nghĩa |
| --- | --- | --- |
| `PLATFORM_INDIVIDUAL_MAX_SEATS` | 5 | Số ghế tối đa của workspace `individual` khi đổi gói. |
| `PLATFORM_ACCOUNT_CREATE_PER_HOUR` | 30 | Số yêu cầu tạo tài khoản tối đa mỗi giờ cho mỗi staff. |
| `PLATFORM_USAGE_DUE_DAYS` | 7 | Số ngày hóa đơn theo mức dùng được chưa trả trước khi thuê bao sang `past_due`. |
| `PLATFORM_USAGE_MIN_INVOICE` | 10000 | Số tiền tối thiểu của hóa đơn tháng, tính theo đơn vị nhỏ nhất của tiền tệ. Dưới mức này, các dòng dồn sang tháng sau. |
| `BILLING_VAT_RATE` | 0.10 | Thuế suất của đơn hàng, gồm hóa đơn theo mức dùng. Biến này có từ R2. |
| `PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION` | `true` | Với `true`, giảm giá Business chỉ áp cho hồ sơ đã được staff xác minh. Chỉ nhận đúng `true` hoặc `false`. |
| `PLATFORM_API_RATE_CEILING` | 6000 | Trần số request API mỗi phút khi gói để trống hạn mức này (gói không giới hạn). |

Spec 7b.7 còn nêu `PLATFORM_PRICE_NOTICE_DAYS`, `PLATFORM_PRICE_CONFIRM_PCT` và `PLATFORM_PRICE_MAX`. Không có mã nào đọc ba biến này (`config.go` không có chúng, và `pricechange` chỉ nhắc tên trong chú thích). Đặt chúng không có tác dụng; lịch đổi giá dùng giá trị cố định 30 ngày, 50 phần trăm và 1.000.000.000 (`platform-management/internal/pricechange/pricechange.go`).

### 13.5 `MULTICA_CLOUD_URL` với bản tự lưu trữ

Với bản tự lưu trữ, để `MULTICA_CLOUD_URL` **trống**. Đã kiểm trong mã:

- Các route `/api/cloud-subscriptions/*` (gồm hồ sơ thanh toán, đổi gói, hủy, tiếp tục, mua ghế) không gọi platform. Máy chủ trả 403 `cloud_runtime_not_configured`, hoặc 403 `workspace_subscriptions_disabled` khi cờ `billing_workspace_subscriptions` tắt (`server/internal/handler/cloud_billing.go:113`, `:145`).
- Cổng `member_groups` tắt: không có provider quyền lợi nên `planlimit.Resolve` trả chính sách `off` (`server/internal/planlimit/planlimit.go:50`, `server/cmd/server/router.go:507`). Tạo nhóm tài khoản không bị giới hạn theo gói.

Điều này **không** có nghĩa mọi route R5 giữ nguyên như trước. Các route nhóm tài khoản (`/api/workspaces/{id}/groups`) và việc kiểm khóa quyền (`members.invite`, `audit.read` và các khóa khác) chỉ đọc cơ sở dữ liệu, không phụ thuộc `MULTICA_CLOUD_URL`. Chúng chạy sau khi triển khai bản R5 dù biến để trống. Tôi chưa kiểm các hành vi R5 khác với biến trống.

Chỉ trỏ `MULTICA_CLOUD_URL` về platform theo mục 11, bước 5.

### 13.6 Hoàn tác

- Dừng dịch vụ platform thì các route R5 phía platform ngừng trả lời: hồ sơ thanh toán, đổi gói, hủy và tiếp tục, mua ghế, đọc mức dùng, và các việc nền của platform (chấm tiền theo mức dùng, lập hóa đơn, áp đổi giá). Máy chủ Multica vẫn chạy.
- Phần nằm trong máy chủ Multica không dừng theo: các route nhóm tài khoản và việc kiểm khóa quyền đọc cơ sở dữ liệu (13.5).
- Migration **không** tự hoàn tác. Chạy lại bản Multica cũ không rút migration nào. Mỗi migration có tệp `.down.sql`, và `cmd/migrate` nhận lệnh `down`, nhưng không có bước nào gọi lệnh này tự động. Chỉ chạy `down` khi có quyết định riêng và đã có bản sao lưu.

## 14. Hóa đơn điện tử (R4): hướng dẫn cho nhà vận hành

Nguồn: spec 7.3 ([platform-management-spec.md](engineering/platform-management-spec.md)) và bước P43.7 ([platform-management-plan.md](engineering/platform-management-plan.md)). Mục này chỉ ghi điều đã đọc trong mã và spec. Chưa chạy bước nào trên SePay, nên không có kết luận nào ở đây dựa trên một lần gọi thật.

### 14.1 Cấu hình

Platform đọc các biến này trong `loadEinvoice` (`platform-management/internal/config/config.go:437`). Thông báo lỗi chỉ nêu tên biến, không in giá trị. Thiếu hoặc sai một biến bắt buộc thì platform thoát lúc khởi động.

| Biến | Giá trị | Ghi chú |
| --- | --- | --- |
| `PLATFORM_EINVOICE_PROVIDER` | rỗng hoặc `sepay` | Rỗng là tắt. Giá trị khác thì thoát (`config.go:445`). |
| `PLATFORM_EINVOICE_SEPAY_API_KEY` | `client_id:client_secret` | Bắt buộc khi provider là `sepay`; thiếu một trong hai phần thì thoát (`config.go:452`). Không in ra. |
| `PLATFORM_EINVOICE_SEPAY_BASE_URL` | `https://einvoice-api.sepay.vn` (production) hoặc `https://einvoice-api-sandbox.sepay.vn` (sandbox) | Mã kiểm scheme là `https` và host là một trong hai host trên; đường dẫn và cổng không được kiểm riêng (`config.go:455`). |
| `PLATFORM_EINVOICE_SEPAY_PROVIDER_ACCOUNT_ID` | UUID | Bắt buộc khi provider là `sepay` (`config.go:463`). |
| `PLATFORM_EINVOICE_START_AT` | RFC 3339, ví dụ `2026-02-01T00:00:00+07:00` | Bắt buộc khi provider là `sepay`. Đơn trả trước mốc này không có hóa đơn (`config.go:467`). |
| `PLATFORM_EINVOICE_MAX_ATTEMPTS` | số nguyên | Mặc định 8 khi không đặt (`config.go:439`). |
| `BILLING_VAT_RATE` | tỷ lệ, mặc định 0.10 | Thuế suất cho đơn `vnd`. Khi provider là `sepay`, `newEinvoiceService` yêu cầu tỷ lệ này là số phần trăm nguyên (0.10 hợp lệ, 0.085 không); sai thì thoát với lỗi nêu `BILLING_VAT_RATE` (`platform-management/cmd/platform-server/main.go:454`). |

Khi `PLATFORM_EINVOICE_PROVIDER` rỗng, dịch vụ tắt và không tạo bản ghi nào (`main.go:449`, spec 7.3 quy tắc 9). Các biến SePay không được đọc trong trường hợp này (`config.go:437`).

### 14.2 Sandbox và production

Sandbox không gửi hóa đơn lên cơ quan thuế, và token chỉ dùng được trên môi trường đã cấp ra nó (spec 7.3, phần "Xác thực" và "Hợp đồng đã đọc lại"). Vì vậy `PLATFORM_EINVOICE_SEPAY_BASE_URL` và `PLATFORM_EINVOICE_SEPAY_API_KEY` phải cùng một môi trường. Không ghép khóa sandbox với host production, và ngược lại. Host production chỉ dùng sau khi kế toán xác nhận (14.3).

### 14.3 Thứ tự bật ở production

Kế toán xác nhận bộ trường người mua bắt buộc và cách xử lý hóa đơn sai (spec mục 16, điểm 4) **trước khi** đặt `PLATFORM_EINVOICE_PROVIDER` và `PLATFORM_EINVOICE_START_AT` ở production (spec 7.3 và plan P43.7). Mã và test không chờ xác nhận này. Chỉ lần bật ở production chờ. Xác nhận của kế toán hiện **còn mở**.

### 14.4 Hai việc nền

Cả hai chạy mỗi phút (`platform-management/internal/jobs/einvoice.go:20`, `:25`; đăng ký ở `platform-management/cmd/platform-server/payment_jobs.go:26`, `:32`).

- `einvoice_enqueue` tạo bản ghi `pending` cho mỗi đơn `paid` chưa có bản ghi (`platform-management/internal/invoice/service.go:43`). Điều kiện nằm trong `ListPaidOrdersNeedingEinvoice` (`platform-management/queries/einvoice.sql:46`): `amount > 0`, `provider <> 'admin_grant'`, `paid_at >= PLATFORM_EINVOICE_START_AT`. Đơn 0 đồng, đơn `admin_grant` và đơn trả trước mốc không có bản ghi.
- `einvoice_issue` nhận các dòng `pending` đã đến hạn bằng `FOR UPDATE SKIP LOCKED` và gọi nhà cung cấp (`platform-management/queries/einvoice.sql:57`, `:71`; `service.go:77`). Lần nhận tăng `attempts` trước khi gọi.

Đơn luôn giữ `paid`, bất kể hóa đơn ra sao (spec 7.3 quy tắc 5). Thử lại lỗi tạm thời cách nhau theo cấp số nhân từ 1 phút, trần 6 giờ (`platform-management/internal/invoice/retry.go:13`); hết `PLATFORM_EINVOICE_MAX_ATTEMPTS` thì `failed` (`retry.go:33`).

### 14.5 Trạng thái và việc cần làm

Tên trạng thái ở `platform-management/internal/invoice/record.go:10`.

| Trạng thái | Ý nghĩa với nhà vận hành |
| --- | --- |
| `pending` | Chờ phát hành hoặc chờ lần thử lại tiếp theo. |
| `issued` | Đã có số hóa đơn và liên kết tra cứu. Platform không xóa và không sửa. |
| `needs_info` | Thiếu trường người mua bắt buộc. **Chưa gọi** nhà cung cấp (`service.go:94`). Bản ghi lưu tên trường thiếu, không lưu giá trị. |
| `failed` | Hết số lần thử với lỗi tạm thời (mạng, hết thời gian, 429, 5xx), hoặc nhà cung cấp từ chối dữ liệu vĩnh viễn (`retry.go:33`). Đọc mã lỗi trong hàng `einvoice.failed` (14.8). |
| `cancelled` | Giá trị cũ. Không có mã nào đặt trạng thái này (spec 7.3 quy tắc 2). |

Với `needs_info` và `failed` do dữ liệu người mua, người sửa là khách. Khách sửa ở trang đơn qua `PUT /api/workspaces/{id}/orders/{orderId}/invoice-buyer` (`server/cmd/server/router.go:2071`, handler `server/internal/handler/billing_orders.go:578`). Route yêu cầu quyền `billing.manage` và người thật; spec 7.3 viết "owner và admin" cho cùng route. Sửa xong, bản ghi về `pending`; bản ghi `issued` trả 409 `einvoice_already_issued`. Console không có thao tác sửa người mua. Sau khi khách sửa, bản ghi `failed` vẫn cần staff thử lại (14.6).

Bộ trường người mua bắt buộc theo mã: `name`, `tax_code`, `address`, `email` (`platform-management/internal/einvoice/sepay.go:92`). Một trường rỗng tính là thiếu (`platform-management/internal/invoice/provider.go:119`). Đây là bộ **tạm**: SePay không công bố bộ trường bắt buộc, và mã ghi rõ là giả định (spec 7.3, mục "Trường người mua bắt buộc (chưa xác nhận)"). Bộ này chờ kế toán xác nhận (spec mục 16, điểm 4).

### 14.6 Thử lại

`POST /api/platform/einvoices/{orderId}/retry`, vai trò `billing` hoặc `admin` (`platform-management/internal/console/handler.go:149`). Mọi phương thức ghi đi qua `RequireOrigin` (`handler.go:201`). Middleware so header `Origin` với `PLATFORM_CONSOLE_ORIGIN` (`platform-management/internal/platformauth/platformauth.go:177`, `config.go:296`); `Origin` sai, hoặc không có `Origin` mà có `Cookie`, bị 403 `bad_origin`.

Bản ghi `failed` hoặc `needs_info` về `pending`, `attempts` về 0 (`platform-management/queries/einvoice.sql:88`). Handler ở `platform-management/internal/console/einvoices.go:341`; lỗi ở `einvoices.go:298`:

| Mã | Khi nào |
| --- | --- |
| 409 `einvoice_buyer_incomplete` (kèm `fields[]`) | Người mua vẫn thiếu trường bắt buộc. Bản ghi giữ nguyên. |
| 409 `einvoice_state_conflict` | Bản ghi không ở `failed` hay `needs_info`. |
| 404 | Đơn không có bản ghi. |
| 503 `einvoice_provider_not_configured` | Provider tắt. |

Màn hình `EinvoiceFailures` (`platform-management/console/src/screens/EinvoiceFailures.tsx`) **không có nút thử lại**. Hiện chỉ thử lại được bằng lời gọi API. Đây là việc còn mở (spec 7.6; CSN-27 trong `docs/engineering/platform-management-features.md`). Mục này không mô tả giao diện nào chưa có.

Route `.../replace` đã bỏ: `POST` trả 404 (`TestReplaceRouteIsGone`, `platform-management/internal/console/einvoices_routes_test.go:328`). Không có route hủy. Hóa đơn sai do `admin` sửa trực tiếp trên SePay (spec 7.3 quy tắc 6).

### 14.7 Kết quả không rõ sau khi hết thời gian

Mã tham chiếu của mỗi lần phát hành là `order_id-revision` (`platform-management/internal/invoice/provider.go:26`). Khi một lần thử trước có thể đã tới nhà cung cấp (`attempts > 1` lúc xử lý, `service.go:116`), việc nền tra cứu theo mã này trước khi phát hành lại (`provider.go:137`; `Lookup` ở `platform-management/internal/einvoice/sepay.go:257`). Một hóa đơn nháp chưa tính là đã phát hành, và lần chạy sau tiếp tục từ bản nháp.

- Không phát hành lại bằng tay. Không tạo hóa đơn thứ hai trên SePay cho cùng một đơn.
- Thấy trên SePay một hóa đơn mà platform không hiện, thì tra theo mã tham chiếu `order_id-revision` trước.
- **Khoảng trống đã biết:** sau lần thử lại của staff, `attempts` về 0 (`platform-management/internal/invoice/service.go:185`), nên lần nhận đầu tiên có `attempts = 1` và **không** tra cứu trước khi gọi (`service.go:116`). Với bản ghi `failed` do hết thời gian, lần thử lại này có thể phát hành lại khi hóa đơn đã tồn tại trên SePay. Trước khi thử lại, kiểm trên SePay xem hóa đơn theo mã `order_id-revision` đã có chưa. Đây là khác biệt so với quy tắc 5 của spec 7.3 và AC58; chưa sửa trong bước đóng P43.

### 14.8 Xem lịch sử

- Danh sách: `GET /api/platform/einvoices`. Mặc định lọc `needs_info` và `failed`. Tham số `state` nhận `pending`, `issued`, `needs_info`, `failed`; `cancelled` và giá trị khác trả 400 `invalid_state` (`platform-management/internal/console/einvoices.go:240`, `:258`). Màn hình `EinvoiceFailures` dùng danh sách này.
- Một đơn: `GET /api/platform/orders/{id}/einvoices`, mọi `revision`, mới nhất trước; đơn không có bản ghi trả 200 với danh sách rỗng (`handler.go:141`, `einvoices.go:177`). Spec 7.3 ghi `/api/platform/einvoices/{orderId}`; đường dẫn đã xây là đường dẫn theo đơn.
- Màn hình console: hook `useOrderEinvoices` (`platform-management/console/src/api/hooks.ts:1136`) chỉ được gọi ở màn hình Orders, trong hộp thoại "E-invoice history" mở bằng nút History trên từng đơn (`platform-management/console/src/screens/Orders.tsx:96`, `:330`).
- Khách: trang đơn hiện trạng thái, số hóa đơn, liên kết tra cứu (chỉ `https`) và tên trường thiếu, qua `EinvoiceStatusSummary` trong `packages/views/billing/einvoice-section.tsx:119`. Mã không có thành phần tên `EinvoiceSection`; tên đó chỉ là tên tệp.

### 14.9 Kiểm toán

Sáu sự kiện: `einvoice.requested`, `einvoice.issued`, `einvoice.needs_info`, `einvoice.failed`, `einvoice.retried` (hằng ở `platform-management/audit/audit.go:234`) và `einvoice.buyer_updated`. Sự kiện cuối là chuỗi cố định trong `server/internal/handler/billing_orders.go:550`, không có trong danh mục `audit.go`. `audit.go` còn hằng `einvoice.replaced`; không còn mã nào ghi nó.

`detail` chứa tên trường, mã lỗi, số lần thử, số hóa đơn và mã hóa đơn của nhà cung cấp, không chứa giá trị người mua (`platform-management/internal/invoice/pgx.go:218`, `:264`, `:451`; `billing_orders.go:669`). `target_type` là `einvoice`, `target_id` là UUID đơn (`pgx.go:58`).

```sql
SELECT created_at, action, actor_type, actor_label, target_label, detail
FROM audit_event
WHERE target_type = 'einvoice'
  AND target_id = '<order id>'
ORDER BY created_at, id;
```

Giữ nguyên hạn chế ở mục 2: không sửa `audit_event` bằng tay.

### 14.10 Giới hạn

- Không dùng webhook cho hóa đơn: spec 7.3 liệt webhook vào mục "Chưa xác minh". Trạng thái phát hành được lấy bằng thăm dò.
- `TestSepaySandboxLive` (`platform-management/internal/einvoice/sandbox_live_test.go:17`) đọc hai biến riêng của test, `SEPAY_EINVOICE_API_KEY` và `SEPAY_EINVOICE_PROVIDER_ACCOUNT_ID`, không đọc tên `PLATFORM_EINVOICE_*`; thiếu thì bị bỏ qua. Test này **chưa chạy** khi viết mục này.
- Bộ trường người mua bắt buộc và cách xử lý hóa đơn sai chờ kế toán (spec mục 16, điểm 4). Còn mở.
- Spec còn các điểm chưa xác minh: tỷ giá ngoại tệ, giới hạn tốc độ, 429, Nghị định 254/2026, lookup bản nháp trên sandbox.

### 14.11 Bảng kiểm trước khi bật ở production

Các mục dưới là việc cần làm. Mục này không ghi mục nào đã xong.

1. Kế toán đã xác nhận bộ trường người mua bắt buộc và cách xử lý hóa đơn sai; xác nhận được lưu ở nơi nhóm tra cứu được.
2. `PLATFORM_EINVOICE_SEPAY_BASE_URL` là host production, và `PLATFORM_EINVOICE_SEPAY_API_KEY` cùng `PLATFORM_EINVOICE_SEPAY_PROVIDER_ACCOUNT_ID` được cấp từ môi trường production.
3. `PLATFORM_EINVOICE_START_AT` đặt đúng giờ bật, kèm múi giờ.
4. `BILLING_VAT_RATE` là số phần trăm nguyên và khớp với thuế suất kế toán xác nhận.
5. `PLATFORM_CONSOLE_ORIGIN` khớp với origin console dùng để gọi API thử lại.
6. Luồng trên sandbox đã chạy: một đơn phát hành được, một đơn `needs_info` được khách sửa, và API thử lại được gọi thành công một lần.
7. `TestSepaySandboxLive` đã chạy với khóa sandbox của nhóm.
