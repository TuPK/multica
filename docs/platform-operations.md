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

Status: **R2 is written. Gate G-R2 is not closed.** See section 12.

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

**G-R2 is not closed.** It stays open until every item below is done. G-R1 (section 5) is also not passed, and R2 follows it.

**Database tests not yet run.** Each needs a test database whose name ends in `_test`, supplied by the project owner (gate G0), and the guard `scripts/require-test-database.sh`. No run against such a database is recorded for these, so they count as not run. Platform packages use `PLATFORM_TEST_DATABASE_URL` and skip without it (`platform-management/internal/testdb/testdb.go`).

| Step | Package and pattern |
| --- | --- |
| P13 | platform `./internal/schemacheck` |
| P14 | platform `./internal/staff ./internal/session ./internal/mailer` |
| P15 | server `./internal/middleware ./internal/handler ./internal/service ./internal/workspaceguard -run 'WorkspaceState\|ClaimRefusal'`; platform `./internal/workspacestatus` |
| P16 | platform `./internal/console -run 'PlatformConsole\|PlatformIsolation'` |
| P18 | platform `./internal/plan ./internal/entitlement`; server `./internal/service ./internal/entitlement -run 'Entitlement\|IssueLimit\|AutopilotQuota'` |
| P19 | platform `./internal/seat`; server `./internal/handler ./internal/seatcapacity -run 'Seat\|Invitation\|ShareLink'` |
| P20 | server `./internal/handler -run 'PlanLimit\|WorkspaceCreation\|Signup'` |
| P21 | platform `./internal/billing` |
| P22 | platform `./internal/billingapi`; server `./internal/handler -run 'CloudSubscription\|Order\|Plans'` |
| P23 | platform `./internal/lifecycle -run 'Lifecycle'` |
| P24 | platform `./internal/console -run 'PlatformBilling'` |
| P25 | server `./internal/handler -run 'WorkspaceDeletion'` |
| P27 | server `./internal/workspaceexport -run 'WorkspaceExport'` |
| P28 | server `./internal/usagesnapshot -run 'Usage'`; platform `./internal/console -run 'Overview'` |
| P29 | server `./internal/handler -run 'Audit'` |

The R1 database tests listed in section 5 are also still open. P30 also asks for the full Go suite of every touched package on the test database, plus the checks [T], [PG], [PW], [PB] and [PI] of the plan, and a walk through AC7 to AC14 and AC20 (spec 13).

**Rehearsal not done.** P30 requires a rehearsal on a non-production copy of the data: build the stack with the `platform` service, run `server/cmd/migrate up`, give every existing workspace a transitional subscription, point `MULTICA_CLOUD_URL` at the platform, and confirm that no workspace is blocked (spec 12). Also check that the platform exits when a migration is missing and starts by itself after the migration; that port 8095 cannot be reached from outside the internal network; and that an `mcn_` token is refused. Write the record of the rehearsal into this file.

**Decision D49 open.** Bank details are empty by decision of the project owner. Real payments cannot start until `BILLING_BANK_*` are set and the order page shows them (section 6).

**Other items before launch** (plan P30): G-LIC (commercial license, spec 0), the items of spec 16, database backup, monitoring. Their state is not recorded in the sources I read.

**Gaps found in the tooling while writing this part:**

- No order export and no `einvoice_ref` write (section 10).
- The console's **Edit subscription** dialog clears dates on a subscription (section 8.1).
- `docker-compose.selfhost.yml` has no `platform` service, and does not pass `BILLING_BANK_*` to the Multica server (sections 6 and 11).
- Rules for underpayment, overpayment and refunds are not in the spec (section 7.1).
