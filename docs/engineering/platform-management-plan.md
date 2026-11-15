# Kế hoạch triển khai: platform management cho SaaS nhiều tenant

Đặc tả (thiết kế, quyết định, mô hình dữ liệu, API, mã lỗi, tiêu chí chấp nhận): [platform-management-spec.md](platform-management-spec.md). Tài liệu này là các bước, có thứ tự, kiểm được. Cả hai là nguồn duy nhất cho việc triển khai.

Ngày viết 2026-11-14, đọc ở **HEAD `1c631cf2c`**. **Chưa có mã nào được viết.** Đường dẫn ghi "mới" chưa tồn tại; "đổi" đã tồn tại ở HEAD này. Phong cách theo `flow-engine-generalization-plan.md`.

## 0. Cách đọc và quy ước

- **Thứ tự bắt buộc:** pha A (R1, nền tảng an toàn) → pha B (R2, thương mại) → pha C (R3, doanh nghiệp) → pha D (R4, mỗi phần cần đặc tả riêng). Trong một pha, các bước theo đúng số thứ tự trừ khi ghi "độc lập".
- **Cổng:**
  - **G0** có cơ sở dữ liệu test (mục 0.2). Không có thì mọi bước [D] đứng yên.
  - **G-R1** sau P12. **G-R2** sau P30. **G-R3** sau P40.
  - **G-LIC** có giấy phép thương mại (spec mục 0). Đứng trước **mọi** lần mở dịch vụ cho người ngoài tổ chức. Không chặn việc viết mã.
- Mỗi bước là một commit nguyên tử theo conventional commits: `feat(platform): …`, `feat(audit): …`, `fix(auth): …`, `test(platform): …`, `docs(platform): …`. PR theo mẫu của repo; PR có giao diện đính kèm ảnh chụp hoặc video (AGENTS.md "Change and Delivery Rules").
- **Test viết trước** (đỏ rồi xanh), đặt cạnh mã. Mỗi bước ghi test nào phải đỏ trước khi sửa.
- Nhãn kiểm: **[N]** không cần DB. **[D]** cần DB test.
- Ước lượng ngày công ghi cuối mỗi bước là ước lượng của tác giả thiết kế, chưa đo.
- Mọi quyết định D1 đến D38 đã chốt (spec mục 1.4); không bước nào chờ người dùng trả lời. Các giả định nền U1 đến U9 đã được xác nhận bằng đọc mã (spec mục 15.1). Bước nào xử lý phần còn lại được ghi "Xác nhận U<n>".
- Việc nền của Multica là một `scheduler.JobSpec` đăng ký ở `server/cmd/server/main.go`; việc nền của platform chạy bằng `platform-management/internal/jobs` (spec mục 3.5, D26). Tên và nơi chạy của từng việc ở spec mục 3.5. Test của việc nền gọi thẳng `Handler` của nó hai lần liên tiếp để chứng minh lũy đẳng.

### 0.1 Lệnh chuẩn

| Mã | Lệnh | Ghi chú |
| --- | --- | --- |
| **[B]** | `pnpm catalog:build` | Sinh mã từ `catalog/`; chạy trước `vitest`, `tsc`, `go test` chạy trực tiếp. Không commit mã sinh |
| **[Q]** | `make sqlc` | Sau mọi sửa `server/pkg/db/queries/*.sql`; commit cả `server/pkg/db/generated/*` |
| **[GN]** | `( cd server && go test -count=1 <gói> -run '<mẫu>' )` | Nhãn [N]. Luôn kèm `-run` khi gói có test DB |
| **[D]** | `DATABASE_URL='<dsn-test>' bash scripts/require-test-database.sh && ( cd server && DATABASE_URL='<dsn-test>' go test -count=1 -p 1 <gói> -run '<mẫu>' )` | DSN do người dùng cấp, đặt tường minh trong chính lệnh |
| **[M]** | `DATABASE_URL='<dsn-test>' bash scripts/require-test-database.sh && ( cd server && DATABASE_URL='<dsn-test>' go run ./cmd/migrate up )` | Sau mỗi bước thêm migration |
| **[W]** | `pnpm --filter @multica/core exec vitest run <mẫu>`; `pnpm --filter @multica/views exec vitest run <mẫu>` | Nhãn [N]; chạy [B] trước |
| **[T]** | `pnpm typecheck`, `pnpm lint`, `pnpm test` | Gốc repo, không gồm mobile. Gồm console khi `platform-management/console` đã nằm trong `pnpm-workspace.yaml` (P17) |
| **[V]** | `( cd server && go build ./... && go vet ./internal/workspaceguard/... ./internal/auditstore/... ./internal/handler/... ./internal/middleware/... ./cmd/... )` | Sau mỗi bước Go ở `server/` |
| **[PG]** | `( cd platform-management && go build ./... && go vet ./... && go test -count=1 ./... -run '<mẫu>' )` | Nhãn [N]: test Go của module platform không cần DB. Luôn kèm `-run` khi gói có test DB |
| **[PD]** | `DATABASE_URL='<dsn-test>' bash scripts/require-test-database.sh && ( cd platform-management && DATABASE_URL='<dsn-test>' go test -count=1 -p 1 ./internal/<gói>/... -run '<mẫu>' )` | Nhãn [D] cho module platform; cùng DSN và cùng guard với [D]. Test cần Redis dùng Redis giả |
| **[PQ]** | `( cd platform-management && sqlc generate )` | Sau mọi sửa `platform-management/queries/*.sql`; commit mã sinh ở `platform-management/internal/db/`. `sqlc.yaml` của platform đọc schema từ `../server/migrations/` |
| **[PW]** | `pnpm --filter @multica/platform-console exec vitest run <mẫu>`; `pnpm --filter @multica/platform-console typecheck` | Test và kiểm kiểu của console; chạy [B] trước |
| **[PB]** | `pnpm --filter @multica/platform-console build` | Dựng bản tĩnh của console |
| **[PI]** | `docker build -f platform-management/Dockerfile -t multica-platform .` | Gốc repo. Chỉ build ảnh, không chạy container |
| **[E]** | `pnpm exec playwright test` | **Không chạy** trừ khi người dùng cho phép |

### 0.2 Cổng G0 và những điều không được làm

**G0.** Người dùng tạo một cơ sở dữ liệu test có tên **kết thúc `_test`** và **khác `multica`**, rồi cấp DSN. Mọi lệnh cần DB chạy sau guard `scripts/require-test-database.sh`. Trước G0, bước [D] chỉ viết test và PR ghi "chưa chạy".

Không được làm:

1. Không chạy `make test`, `make check`, `make migrate-up`, `make up`, `make down`, `make destroy`, `go test` trần hay `go run ./cmd/migrate up` trần. Không tạo cơ sở dữ liệu. Không ghi vào cơ sở dữ liệu `multica`.
2. Không khóa ngoại, không cascade. Bảng mới không có `PRIMARY KEY` hay `UNIQUE` nội tuyến. Mọi chỉ mục `CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS`, một chỉ mục một tệp một câu lệnh, có dòng dọn tương ứng trong `concurrentIndexCleanups` của `server/cmd/migrate/main.go`.
3. Không sửa `server/pkg/db/queries/*.sql` mà không chạy [Q] và commit mã sinh.
4. Không commit mã sinh của catalog.
5. Không sửa tệp ngoài danh sách điểm móc ở spec mục 3.3 mà không cập nhật danh sách đó trong cùng commit.
6. Không thêm đường dự phòng, ghi đôi hay bộ chuyển đổi tạm. `MULTICA_CLOUD_URL` là công tắc thương mại duy nhất (D36): không thêm chế độ hay công tắc thứ hai, và không chạy song song hai nguồn quyết định (spec I4).
7. Không nhân đôi quy tắc quyền giữa Go và TypeScript ngoài phần phản chiếu ở `packages/core/permissions/rules.ts`.
8. Không đụng `apps/mobile/` mà chưa đọc `apps/mobile/AGENTS.md`.
9. Mọi chuỗi giao diện mới đủ bảy ngôn ngữ; không mô tả thừa (`conventions.mdx` "UI descriptions").
10. Chỉ đẩy lên `origin` (`TuPK/multica`). Không mở PR lên `multica-ai/multica`; PR nếu có chỉ dùng `--repo TuPK/multica`.
11. Trước mỗi bước chạm một tệp ở spec mục 3.3, chạy `git status --short -- <tệp>` và `git log -3 --format='%h %s' -- <tệp>`. Nếu có sửa chưa commit của công việc khác thì chờ; không hoàn tác thay đổi của người khác. Lúc viết kế hoạch, cây làm việc có sửa chưa commit ở `packages/views/company/components/*` và `server/internal/issueworkflow/*`; không bước nào ở đây chạm các tệp đó.
12. Platform không chạy migration và không nhập gói `server/internal` của Multica (D33, D34). Chỗ phải lặp lại mã (bộ gửi email, bộ giải mã bí mật MFA) ghi ở spec 15.3(b); không thêm chỗ lặp thứ ba mà không cập nhật mục đó.
13. Bước nào thêm migration cho bảng mà platform đọc hay ghi thì thêm phiên bản đó vào danh sách của `platform-management/internal/schemacheck` (P13) trong cùng commit; test của P13 thất bại nếu thiếu.

### 0.3 Dải số migration

Hạng mục giữ dải **651 đến 699** (spec mục 8). Mỗi bước ghi số nó dùng. Phân bổ: R1 dùng 651 đến 662, R2 dùng 663 đến 688, R3 dùng 689 đến 699; nếu thiếu thì dùng dải dự phòng 953 đến 969. Mỗi lần thêm migration chạy `[GN] ./internal/migrations -run 'TestMigrationFilesHaveMatchingDirections|TestMigrationNumericPrefixesAreUnique'` và `[GN] ./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'`.

## 1. Pha 0: chuẩn bị

**P0. Kiểm lại nền trước khi viết mã** [N]

Các quyết định và giả định nền đã chốt trong spec, nên bước này chỉ kiểm rằng nền chưa trôi kể từ HEAD `1c631cf2c`:

1. Chạy lại script đối chiếu trích dẫn `tệp:dòng` của hai tài liệu trên HEAD hiện tại; sửa số dòng đã lệch trong cùng commit.
2. Kiểm dải 651 đến 699 còn trống: `ls server/migrations | sed 's/_.*//' | sort -n | uniq | awk '$1>=651 && $1<=699'` phải không in gì. Nếu công việc khác đã chiếm, chuyển phần còn lại sang dải 953 đến 969 và sửa mục 0.3.
3. Kiểm G0: có DSN của cơ sở dữ liệu test và guard chấp nhận nó.
4. Chạy [T] và ghi lại các test đang đỏ sẵn ở HEAD, để về sau không quy nhầm cho hạng mục này.

Kiểm: bốn mục trên có kết quả ghi trong PR đầu tiên. Ước lượng: 0,5 ngày.

## 2. Pha A: R1, nền tảng an toàn

**P1. Vá điểm lộ email lời mời** [D] (spec 4.7)

- Test đỏ trước: trong `server/internal/handler/invitation_test.go`, một thành viên thường gọi `GET /api/workspaces/{id}/invitations` qua router thật và mong 403. Test này phải đỏ ở HEAD.
- Đổi `server/cmd/server/router.go`: chuyển route sang nhóm `RequireWorkspaceRoleFromURL(…, "owner", "admin")`.
- Đổi `packages/core/workspace/queries.ts` (`invitationListOptions` nhận cờ `enabled`) và `packages/views/settings/components/members-tab.tsx`. Test views: thành viên thường không phát request.

Kiểm: [D] `./internal/handler -run 'Invitation'`, [W] `members-tab`, [V]. Ước lượng: 0,5 ngày.

**P2. Bảng `platform_account` và `auth.AccountGuard`** [D]

- Mới: migration 651 (bảng), 652 (chỉ mục duy nhất `user_id`), 653 (chỉ mục `status` có điều kiện); `server/pkg/db/queries/platform_account.sql`; `platform-management/accountguard/` (logic guard); `server/internal/auth/account_guard.go` (bộ nối) và test.
- Test [N] với Redis giả và DB giả: trượt cache đọc DB rồi ghi cache; kết quả "không có dòng" cũng được cache; Redis lỗi thì đọc DB; DB lỗi thì trả lỗi; guard `nil` cho qua; ghi trạng thái xóa cache.

Kiểm: [Q], [M], [GN] `./internal/auth -run 'AccountGuard'`, [V]. Ước lượng: 1,5 ngày.

**P3. Nối guard vào `Auth`, `DaemonAuth` và WebSocket** [D] (spec 4.1)

- Test đỏ trước: ma trận năm loại token × bốn trạng thái tài khoản (spec mục 13, bộ 1). Với `mdt_`, kiểm theo chủ runtime; token không có chủ được miễn. `pending_deletion` chỉ cho ba route trong danh sách.
- Đổi `server/internal/middleware/auth.go`, `daemon_auth.go`, `server/internal/realtime/hub.go`, `server/cmd/server/router.go` (truyền guard). Khi trạng thái chuyển sang khóa, ngắt WebSocket qua `Hub.DisconnectUser` (một lần gọi, mọi workspace; thêm mới ở P3, vì `Hub.DisconnectMember` chỉ ngắt theo một workspace và kiểm lại tư cách thành viên).
- Giữ nguyên `temporary_disabled_users.go`.

Kiểm: [D] `./internal/middleware ./internal/realtime -run 'AccountGuard|Suspended'`; toàn bộ test hiện có của hai gói xanh; [V]. Ước lượng: 2 ngày.

**P4. Buộc đăng xuất mọi thiết bị** [D] (spec 4.2)

- Test đỏ trước: (a) token cấp trước mốc bị 401 `session_revoked`; (b) token cũ đã gia hạn trước mốc vẫn bị loại; (c) token không có `auth_time` xét theo `iat`; (d) gia hạn chép `auth_time` nguyên vẹn; (e) phiên gọi lệnh nhận token mới dùng được.
- Đổi `server/internal/handler/auth.go` (`issueJWT`), `server/internal/auth/session.go` (`RenewSessionToken`), `server/internal/middleware/auth.go` (so mốc trước bước gia hạn). Mới: `POST /api/me/sessions/revoke-all` sau `RequireHumanActor`.
- Client: nút trong trang cài đặt tài khoản, web và desktop; xử lý `session_revoked` như hết phiên. Ghi việc cần làm cho mobile.

Kiểm: [D] `./internal/handler ./internal/middleware ./internal/auth -run 'Session|Revoke'`, [W], [V]. Ước lượng: 2 ngày.

**P5. Bảng `audit_event` và gói `audit`** [D] (spec 4.3)

- Mới: migration 654 (bảng), 655 đến 657 (ba chỉ mục); `server/pkg/db/queries/audit_event.sql`; `platform-management/audit/` (`Record`, bộ lọc bí mật, kiểu `Entry`, hằng tên hành động).
- Test [N]: bộ lọc thay giá trị của khóa nhạy cảm ở mọi độ sâu; test đọc tệp `.sql` và thất bại nếu có `UPDATE` ngoài câu ẩn danh hóa.
- Test [D]: `Record` trong giao dịch bị hủy không để lại dòng.

Kiểm: [Q], [M], [GN] và [D] `./internal/auditstore`, [V]. Ước lượng: 1,5 ngày.

**P6. Ghi kiểm toán cho thành viên, lời mời, link chia sẻ, workspace** [D]

- Test đỏ trước, cho từng hành động trong ba nhóm đầu và nhóm workspace của bảng ở spec 4.3: đúng một dòng với `action`, `actor`, `target` đúng; và khi `audit.Record` trả lỗi (tiêm lỗi), thay đổi không được lưu.
- Đổi `server/internal/handler/workspace.go`, `invitation.go`, `share_link.go`: bọc giao dịch ở handler chưa có; gọi `audit.Record` trước khi commit.

Kiểm: [D] `./internal/handler -run 'Audit'`; toàn bộ test thành viên, lời mời, link chia sẻ hiện có xanh; [V]. Ước lượng: 3 ngày.

**P7. Ghi kiểm toán cấp tài khoản** [D]

- `auth.login` ở ba chỗ gọi `issueJWT` (`server/internal/handler/auth.go:459`, `:724`, `:774`), `auth.sessions_revoked`, `pat.created`, `pat.revoked`. Dòng có `workspace_id` rỗng.
- Test đỏ trước cho từng hành động.

Kiểm: [D] `./internal/handler -run 'Audit.*(Login|PAT|Session)'`, [V]. Ước lượng: 1 ngày.

**P8. API đọc nhật ký, xuất CSV, dọn theo hạn lưu, giao diện** [D]

- Mới: `GET /api/workspaces/{id}/audit-events` (owner, admin; phân trang theo khóa; lọc; `?format=csv`), `GET /api/me/security-events`; job dọn theo `AUDIT_RETENTION_DAYS`.
- Test [D]: thành viên thường 403; phân trang không trùng không sót khi nhiều dòng cùng thời điểm; workspace khác không đọc được; job chỉ xóa dòng quá hạn của fixture.
- Client: `packages/core/audit/` (query, schema zod, test phản hồi sai dạng); tab "Audit log" trong `packages/views/settings/`, nối cả web và desktop; mục sự kiện bảo mật trong cài đặt tài khoản.

Kiểm: [D] `./internal/handler -run 'AuditEvents'`, [W], [T], [V]. Ước lượng: 3,5 ngày.

**P9. Ghi nhận đồng ý điều khoản** [D] (spec 4.4)

- Mới: migration 658 (bảng), 659 (chỉ mục duy nhất); `GET /api/me` thêm `consent_required`; `POST /api/me/consents`; hộp thoại chặn ở client.
- Test: biến `LEGAL_*` rỗng thì trường vắng mặt; đổi phiên bản thì yêu cầu lại; chấp nhận hai lần cùng phiên bản không tạo hai dòng.

Kiểm: [Q], [M], [D] `./internal/handler -run 'Consent'`, [W], [V]. Ước lượng: 1,5 ngày.

**P10. Xuất dữ liệu tài khoản** [D] (spec 4.5)

- Mới: `GET /api/me/export` sau `RequireHumanActor`, có giới hạn một lần mỗi giờ.
- Test đỏ trước: token máy 403; bản xuất không chứa `token_hash`, giá trị token, bí mật kết nối (so chuỗi trên toàn bộ JSON); không chứa dữ liệu của người khác.

Kiểm: [D] `./internal/handler -run 'AccountExport'`, [V]. Ước lượng: 1,5 ngày.

**P11. Xóa tài khoản** [D] (spec 4.6)

1. Sổ đăng ký cột người dùng và test nội quan lược đồ. Test phải đỏ cho tới khi mọi cột được phân loại. Xác nhận U12.
2. `POST /api/me/deletion` (kèm `?preview=1`), `DELETE /api/me/deletion`; điều kiện `sole_owner`; đòi `auth_time` trong 10 phút.
3. Job thực hiện sau thời gian chờ, dùng `revokeAndRemoveMember` và đường xóa workspace hiện có; ẩn danh hóa bản ghi `user` và `actor_label` trong nhật ký.
4. Client: luồng xóa trong cài đặt tài khoản, có bước xem trước và nhập lại để xác nhận.

- Test đỏ trước: AC5 của spec; job chạy hai lần không lỗi; hủy trong thời gian chờ khôi phục đăng nhập; workspace có thành viên khác không bị ảnh hưởng ngoài việc mất một thành viên.

Kiểm: [D] `./internal/handler ./internal/accountdeletion -run 'AccountDeletion|UserColumnRegistry'`, [W], [V]. Ước lượng: 5 ngày.

**P12. Cổng G-R1**

- Chạy [T] và, trên DB test, toàn bộ `./internal/handler ./internal/middleware ./internal/auth ./internal/auditstore ./internal/realtime`, và [PG] cho `platform-management/audit` và `platform-management/accountguard`.
- Đối chiếu AC1 đến AC6.
- Đo độ trễ p95 của một route đọc đơn giản trước và sau khi có guard (spec 15.3); ghi số đo vào PR.
- Viết `docs/platform-operations.md`: khóa và mở tài khoản bằng SQL khi chưa có console, đọc nhật ký, xử lý yêu cầu xóa.

Ước lượng: 1,5 ngày. **Tổng R1: khoảng 25 ngày công.**

## 3. Pha B: R2, thương mại

**P13. Khung dịch vụ platform** [D] (spec 3.4, 5.1; D31, D32, D33)

- Mới, trong `platform-management/`: `cmd/platform-server/` (một tiến trình, hai listener: API 8095 và console 30001); `internal/config` (đọc biến của platform ở spec 3.4); `internal/schemacheck` (danh sách phiên bản migration bắt buộc và hàm kiểm `schema_migrations`); `internal/jobs` (bộ chạy việc nền, giành quyền bằng `pg_try_advisory_lock` theo tên việc); `sqlc.yaml` và `queries/` (schema đọc từ `../server/migrations/`, mã sinh ở `internal/db/`); `internal/testdb` (fixture DB test của platform, vì không nhập được `server/internal/testutil`); `Dockerfile` (image `multica-platform`; ở bước này console còn là thư mục giữ chỗ, P17 thêm giai đoạn dựng console).
- Route: `GET /healthz` trên cả hai listener (API kèm kiểm DB và Redis). Listener console chuyển tiếp `/api/platform/` sang API và trả 404 cho mọi đường dẫn `/api/` khác (D32).
- Phía Multica không đổi gì ở bước này.
- Test đỏ trước, mỗi điều kiện sai ở spec 5.1 một ca (AC7): thiếu `DATABASE_URL`; `schema_migrations` thiếu một phiên bản bắt buộc (thông báo liệt kê phiên bản thiếu); thiếu `REDIS_URL`; `PLATFORM_ADMIN_EMAILS` rỗng; `PLATFORM_SESSION_SECRET` ngắn hơn 32 byte; thiếu cấu hình email. Ca đủ cấu hình: hai listener lên đúng cổng cấu hình (mặc định 8095 và 30001), `/healthz` trả 200, `/api/platform/…` ở cổng console tới được API, `/api/v1/internal/…` ở cổng console trả 404 còn ở cổng API thì tới được handler. Test đọc các tệp `server/migrations/*.up.sql` trong dải 651 đến 699 và thất bại nếu một migration tạo hay đổi bảng `platform_*`, `plan`, `workspace_subscription`, `billing_*`, `workspace_export`, `platform_usage_daily`, `platform_seat_operation` mà phiên bản của nó không có trong danh sách bắt buộc. Test khẳng định platform không có đường mã đăng nhập phát triển (K12). Hồi quy phía Multica: với `MULTICA_CLOUD_URL` rỗng, hành vi như HEAD.

Kiểm: [PG] `./cmd/... ./internal/config ./internal/schemacheck ./internal/jobs`, [PD] `./internal/schemacheck`, [PI], [V]. Ước lượng: 3,5 ngày (ước lượng lại: thêm Dockerfile, sqlc riêng, fixture DB test và bộ chạy việc nền của platform).

**P14. Staff: đăng nhập và phiên của console** [D] (spec 5.2; D34)

- Mới: migration 663, 664 (`platform_staff` và chỉ mục duy nhất); `platform-management/queries/platform_staff.sql`; `internal/staff` (gieo `PLATFORM_ADMIN_EMAILS` thành dòng `admin`); `internal/session` (cookie `mpa_session` ký bằng `PLATFORM_SESSION_SECRET`); `internal/mailer` (gửi mã bằng cùng biến email với Multica, ghi chỗ lặp ở spec 15.3(b)); route `/api/platform/auth/send-code`, `/auth/verify-code`, `/auth/logout`, `GET /api/platform/me`; middleware `RequirePlatformStaff`. `GET /api/me` của Multica không thêm trường vai trò staff.
- Test đỏ trước (AC8), với tên cookie và origin: không có cookie 404; cookie `multica_auth` (JWT Multica hợp lệ) 404; PAT trong `Authorization` 404; token máy 404; `mpa_session` hết hạn hoặc ký sai 404; `auth_time` cũ hơn 12 giờ 404; IP ngoài dải 404; email trong `PLATFORM_ADMIN_EMAILS` được gieo và qua; staff đã thu hồi 404 ngay ở request kế tiếp; tài khoản Multica bị khóa (`platform_account.status = suspended`) 404; ma trận vai trò × route.
- Cookie và mã: `mpa_session` có `HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/`, không có `Domain`; `send-code` luôn trả 204 kể cả với email lạ; mã hết hạn sau 10 phút; sai 5 lần thì mã bị hủy; mã lưu dạng băm; có hạn mức theo IP và theo email; không gửi mã khi tài khoản bị khóa; request ghi có `Origin` khác origin của console bị từ chối.
- Phía Multica ([N]): gửi `mpa_session` tới `Auth` của Multica không xác thực được ai.

Kiểm: [PQ], [M], [PD] `./internal/staff ./internal/session ./internal/mailer`, [PG], [GN] `./internal/middleware -run 'PlatformSessionIgnored'`, [V]. Ước lượng: 3,5 ngày (ước lượng lại: đăng nhập, phiên và bộ gửi email nằm trong platform).

**P15. Trạng thái workspace** [D] (spec 5.3; D25, D38)

- Mới: migration 665 đến 667 (`platform_workspace` và chỉ mục).
- Phía Multica (đọc): `server/pkg/db/queries/platform_workspace.sql`, `platform-management/workspacestate/` (thư viện dùng chung: đọc trạng thái, khóa cache `mul:plat:ws:<id>`), `server/internal/workspaceguard/` (`CheckWorkspaceAccess`, bộ nối DB và Redis, danh sách cho phép).
- Phía platform (ghi): `platform-management/internal/workspacestatus` ghi `platform_workspace`, ghi kiểm toán cùng giao dịch và xóa khóa cache. Route console gọi nó ở P16.
- Đổi: `buildMiddleware`, `requireWorkspaceMember`, và `claimRefusalFor` ở `server/internal/service/budget_gate.go` (D25, D38). `TaskService` nhận một giao diện `WorkspaceGate` có một phương thức; `router.go` luôn gán nó, vì không có dòng nghĩa là `active` nên hành vi như HEAD; để trống chỉ ở test. Kiểm tra trạng thái đứng **trước** bước phân loại lần chạy, vì lần chạy do người kích hoạt hiện được cho qua vô điều kiện.
- Test đỏ trước: ma trận trạng thái × phương thức × danh sách cho phép trên cả hai đường (bộ 2); với workspace không `active`, `ClaimTask` không trả task nào cho cả ba loại lần chạy (người kích hoạt, định kỳ, ủy thác) và task vẫn ở trạng thái xếp hàng; đưa workspace về `active` thì nhận được ngay; task đang chạy hoàn tất được; `WorkspaceGate` để trống thì hành vi như HEAD; AC9. Hai dịch vụ: trạng thái do platform ghi thì Multica thấy sau khi khóa cache bị xóa; xóa cache thất bại thì thấy trong 60 giây; Redis lỗi thì đọc DB; DB lỗi thì 503 (I8).

Kiểm: [Q], [M], [D] `./internal/middleware ./internal/handler ./internal/service ./internal/workspaceguard -run 'WorkspaceState|ClaimRefusal'`, [PQ], [PD] `./internal/workspacestatus`, [PG] `./workspacestate/...`; toàn bộ test daemon và test cổng ngân sách hiện có xanh; [V]. Ước lượng: 4,5 ngày (ước lượng lại: thêm phía ghi của platform).

**P16. API console: tenant, người dùng, ghi chú, staff** [D] (spec 5.4)

- Mới, trong platform: `platform-management/internal/console/` (handler nhóm `/api/platform`); migration 668, 669 (`platform_note`); route dưới `/api/platform`. Đổi trạng thái tài khoản và buộc đăng xuất xóa khóa cache Redis và phát tin lên kênh `mul:acct:changed`.
- Mới, ở Multica: `server/internal/realtime` nghe kênh `mul:acct:changed` và gọi `Hub.DisconnectUser` (điểm móc ở spec 3.3).
- Test đỏ trước: mỗi hành động ghi kiểm toán với `actor_type = staff` và, khi nhắm một workspace, có `workspace_id`; owner của workspace đọc được dòng đó; đổi trạng thái thiếu lý do bị 400; test duyệt handler của `platform-management/internal/console` cấm query bảng nội dung (bộ 6). Hai dịch vụ: khóa tài khoản từ console làm request kế tiếp của người đó ở Multica bị 403 và WebSocket đang mở bị ngắt (AC1, kiểm với Redis giả); mất tin trên kênh thì request HTTP vẫn bị chặn.

Kiểm: [PQ], [M], [PD] `./internal/console -run 'PlatformConsole|PlatformIsolation'`, [GN] `./internal/realtime -run 'AccountChanged'`, [V]. Ước lượng: 4 ngày.

**P17. Console: ứng dụng một trang ở `platform-management/console/`, phục vụ trên cổng 30001** [N] (spec 5.4; D32, D37)

- Mới: `platform-management/console/` (package `@multica/platform-console`, bản build tĩnh) dựng bằng `@multica/ui` và `@multica/core`, không dùng `@multica/views`. Thêm `platform-management/console` vào `pnpm-workspace.yaml`; cập nhật `pnpm-lock.yaml`; kiểm `turbo.json` và `knip.jsonc` nhận package mới.
- Trong console: query, mutation và schema zod qua `parseWithFallback` của `@multica/core/api/schema`, kèm test phản hồi sai dạng; màn hình đăng nhập bằng mã email, danh sách và chi tiết tenant, người dùng, staff, nhật ký, tổng quan. Gọi `/api/platform/` cùng origin; không có bước nào phụ thuộc cookie `multica_auth`.
- Đổi `platform-management/Dockerfile`: thêm giai đoạn dựng console. Platform phục vụ bản build trên 30001, trả `index.html` cho đường dẫn của SPA và không cache `index.html`.
- Console không phải route của web app Multica, nên không có bước nào chạm danh sách slug dành riêng của web app.

Kiểm: [PW], [PB], [T], [PI]. PR đính kèm ảnh chụp. Ước lượng: 7,5 ngày (ước lượng lại: ứng dụng riêng, có đăng nhập, bản build và giai đoạn Docker).

**P18. Gói, thuê bao và hợp đồng quyền lợi của platform** [D] (spec 5.5; D35)

- Mới, trong platform: migration 670 đến 673 (`plan`, `workspace_subscription` và chỉ mục); tệp nhúng danh mục gói; `internal/plan`; `internal/entitlement` phục vụ `GET /api/v1/internal/entitlement-policies/{ws}` (cache 30 giây, xóa khi đổi dữ liệu) và các route `GET /api/v1/subscriptions/{ws}/summary` và `/prices` của hợp đồng Phụ lục A. Không có Provider trong tiến trình: phía Multica dùng client Cloud hiện có, chỉ đổi `MULTICA_CLOUD_URL`.
- Tệp mẫu hợp đồng: `platform-management/contract/testdata/*.json`, do test của platform sinh (cờ `-update`) và test của Multica và client đọc. Dùng lại ở P19 và P22.
- Tệp nhúng mang đúng bảng gói ở spec 5.5 (D24); một test so tệp với bảng đó để hai nơi không lệch nhau.
- Test đỏ trước: workspace không có dòng là gói miễn phí; `limit_overrides` ghi đè; giá trị rỗng là không giới hạn và trả `action = off`; cổng có giới hạn trả `action = enforce` với `limit` không âm; `autopilot_runs` luôn kèm `period_start`, `period_end`, `reset_at` của tháng dương lịch UTC, kể cả ở ranh giới tháng và năm (D28); `version` tăng và được trả làm `subscription_version`; cache hết hạn sau 30 giây và bị xóa khi đổi; workspace không `active` trả giới hạn 0 cho `autopilot_runs`; các cổng của spec 5.7 (cờ và số theo gói) có mặt đúng dạng. Với client hiện có trỏ vào handler của platform, tạo lần chạy autopilot từ từng nguồn `schedule`, `manual`, `webhook`, `api` đều bị chặn khi hết hạn mức (xác nhận U5). Toàn bộ test hiện có của cổng `issue_count` và `autopilot_runs` chạy lại với client trỏ vào platform và xanh.
- Test hợp đồng: phản hồi của handler platform khớp Phụ lục A cho `GET /prices` và đối tượng `entitlement` của `GET /summary` (tệp mẫu đi qua schema zod của client); phản hồi `entitlement-policies` đi qua `normalizePolicy` của client hiện có (bộ 10). `seat_capacity` của `summary` thêm ở P19, các route còn lại ở P22.

Kiểm: [PQ], [M], [PD] `./internal/plan ./internal/entitlement`, [D] `./internal/service ./internal/entitlement -run 'Entitlement|IssueLimit|AutopilotQuota'`, [V]. Ước lượng: 4,5 ngày (ước lượng lại: thêm hợp đồng HTTP và tệp mẫu).

**P19. Giao thức ghế của platform** [D] (spec 5.6)

1. Xác nhận U3: đọc nốt `server/internal/seatcapacity/worker.go` (worker gọi phương thức nào cho từng loại intent, có giữ `WorkspaceLocker` khi gọi không, dùng giá trị nào của `Operation.State`). Viết bộ test hợp đồng mã hóa bảng và bảy quy tắc của spec Phụ lục B, chạy trên bản giả đang dùng trong `server/internal/handler/invitation_test.go`. Bộ này phải xanh trước khi viết handler của platform; nếu một quy tắc không viết được cho xanh trên bản giả thì sửa Phụ lục B trong cùng commit rồi mới đi tiếp.
2. Mới, trong platform: migration 674 đến 676; `internal/seat` phục vụ `/api/v1/internal/subscriptions/{ws}/capacity/…` trên bảng `platform_seat_operation`. Chạy cùng bộ hợp đồng trên **handler HTTP** của nó; đồng thời sinh tệp mẫu phản hồi cho `platform-management/contract/testdata/`.
3. Phía Multica: `seatcapacity.Client` hiện có đọc các tệp mẫu đó qua một `httptest` server, và `invitation`, `share_link` chạy lại toàn bộ test với client này. Không có Executor trong tiến trình và không đổi `router.go`.

- Test thêm: lũy đẳng theo `operation_token`; hai lời mời đồng thời ở ghế cuối chỉ một cái được; giữ chỗ hết hạn được giải phóng; gói không giới hạn trả `Managed = false`; vượt chỗ trả HTTP 409 `capacity_overcommitted` để client dựng lỗi thỏa `IsCapacityOvercommitted`, không gỡ ai và không cho thêm (AC11); thành viên có từ trước khi bật platform được tính vào số đã dùng; platform không bao giờ trả 429; gọi platform trong khi handler đang giữ khóa của `WorkspaceLocker` không treo (quy tắc 6 của Phụ lục B). Khi platform ngừng, lời mời và link chia sẻ trả `seat_capacity_unavailable` 503 (AC20). Mở rộng test hợp đồng Phụ lục A với `seat_capacity` của `summary`.

Kiểm: [PQ], [M], [PD] `./internal/seat`, [D] `./internal/handler ./internal/seatcapacity -run 'Seat|Invitation|ShareLink'`, [V]. Ước lượng: 6,5 ngày (ước lượng lại: thêm tầng HTTP và tệp mẫu). Đây là bước rủi ro cao nhất của pha B.

**P20. Cổng hạn mức mới và giới hạn cấp tài khoản** [D] (spec 5.7)

- Đổi `server/internal/entitlement/types.go` và `client.go` (`normalizePolicy` đọc các cổng mới của spec 5.7; cổng vắng mặt là `off`); `CreateAgent`, đường khôi phục agent, `DaemonRegister`, `UploadFile` và các đường tạo tệp đính kèm; `CreateWorkspace`; `checkSignupAllowed`.
- Test đỏ trước cho từng cổng: dưới giới hạn qua; tại giới hạn 402 `plan_limit_reached` với `gate`, `limit`, `used`; hai request đồng thời ở sát giới hạn chỉ một thành công (AC10); `MULTICA_CLOUD_URL` rỗng thì mọi cổng là `off`. Giới hạn số workspace trên gói miễn phí đọc từ `FREE_PLAN_OWNED_WORKSPACE_LIMIT` (rỗng hoặc 0 là không giới hạn; workspace trả phí không tính); tên miền bị chặn.
- Client: mở rộng hộp thoại nâng cấp hiện có cho mã lỗi mới.

Kiểm: [D] `./internal/handler -run 'PlanLimit|WorkspaceCreation|Signup'`, [W], [V]. Ước lượng: 4 ngày.

**P21. Lõi thanh toán** [D] (spec 5.8)

- Mới: migration 677 đến 682 (`billing_order` và chỉ mục); `platform-management/internal/billing` (định giá, tính theo tỷ lệ, `PaymentProvider`, cài đặt `manual`, `ApplyPayment`).
- Test [N]: bảng ca định giá và tính theo tỷ lệ, gồm biên đầu kỳ, cuối kỳ, năm nhuận, tiền tệ không có phần thập phân.
- Test [D] đỏ trước: xác nhận hai lần chỉ áp một lần (AC12); hai xác nhận đồng thời; đơn hết hạn không xác nhận được; khóa lũy đẳng trả lại đơn cũ; thanh toán đưa workspace `read_only` vì quá hạn về `active`.

Kiểm: [PQ], [M], [PG] và [PD] `./internal/billing`, [V]. Ước lượng: 5 ngày.

**P22. API thanh toán của platform và giao diện đơn hàng** [D]

- Trong platform: phục vụ các đường dẫn `/api/v1/subscriptions/…` mà `proxyCloudSubscription` gọi (`summary`, `prices`, `checkout-sessions`, `seats/purchase-preview`, `seats/purchases`, `seats/reconcile`, `portal-sessions`) theo bảng ánh xạ ở spec 5.8 và hợp đồng ở Phụ lục A (`platform-management/internal/billingapi`).
- Đổi `server/internal/handler/cloud_billing.go` (sửa nhỏ, spec 3.3): `CreateCloudWorkspaceSubscriptionCheckout` chuyển thêm `plan_key` và `seats` tùy chọn. Sáu handler còn lại không đổi.
- Mới, ở Multica: `GET /api/workspaces/{id}/plans`; route đơn hàng của workspace, đọc `plan` và `billing_order` từ DB chung, `PUT .../buyer` ghi cột `buyer` của đơn `pending` (spec 5.8, mục 9), kèm query trong `server/pkg/db/queries/`; trang đơn hàng trong `packages/views/billing/`, nối web và desktop.
- Đổi client (D29): `packages/core/api/schemas.ts` nới `StripeHostedURLSchema` để nhận `http` chỉ với máy chủ `localhost` hoặc `127.0.0.1`, và đọc trường tùy chọn `payment_provider`; `packages/views/settings/components/billing-tab.tsx` thêm bộ chọn gói, đổi nhãn nút và bỏ bước xử lý tham số quay về của Stripe khi `payment_provider` là `manual`. Khi trường vắng mặt, hành vi như hiện tại.
- Test đỏ trước: phản hồi của bảy route **của platform** đi qua schema zod của client (bộ 4; dùng tệp mẫu JSON ở `platform-management/contract/testdata/`, chung giữa test Go và test TypeScript), gồm các ca biên của Phụ lục A: workspace miễn phí chưa có dòng thuê bao (`seat_capacity.version` phải dương), workspace không giới hạn ghế (`seat_capacity` là `null`), có đơn mua ghế đang chờ (`pending_quantity` và `active_purchase`); `currency` là chữ thường; schema từ chối `http` với máy chủ khác máy cục bộ và từ chối `javascript:`; đổi gói qua `plan_key` tạo đúng loại đơn và `url` dựng từ `PLATFORM_APP_URL`. Ở Multica: handler `checkout-sessions` chuyển `plan_key` và `seats` nguyên vẹn; thành viên thường đọc được tóm tắt nhưng không tạo được đơn; token máy 403.

Kiểm: [PQ], [Q], [PD] `./internal/billingapi`, [D] `./internal/handler -run 'CloudSubscription|Order|Plans'`, [W], [T], [V]. PR đính kèm ảnh chụp (AC14). Ước lượng: 7 ngày.

**P23. Vòng đời thuê bao và thông báo** [D] (spec 5.9)

- Mới, trong platform: migration 683, 684 (`billing_notice`); job `subscription_lifecycle` mỗi giờ ở `platform-management/internal/lifecycle`, gửi email bằng bộ gửi của platform; mẫu email nhắc và email chuyển trạng thái.
- Test đỏ trước với đồng hồ giả: mỗi cạnh của sơ đồ; chạy job hai lần liên tiếp và hai job song song không chuyển hai lần và không gửi trùng email (bộ 5); thanh toán ở từng bước đưa về `active` (AC13); số dòng dữ liệu của fixture không đổi qua cả chuỗi.

Kiểm: [PQ], [M], [PD] `./internal/lifecycle -run 'Lifecycle'`, [V]. Ước lượng: 4 ngày.

**P24. Console: thuê bao, đơn hàng, danh mục gói** [D]

- Mới, trong platform và console: các route `PUT /tenants/{id}/subscription`, `/orders*`, `/plans` và màn hình tương ứng.
- Test đỏ trước: vai trò `support` không xác nhận được đơn; đổi gói ghi kiểm toán có giá trị cũ và mới; gói đã có thuê bao không xóa được.

Kiểm: [PD] `./internal/console -run 'PlatformBilling'`, [PW], [V]. Ước lượng: 4 ngày.

**P25. Xóa workspace có thời gian chờ** [D] (spec 5.12)

- Đổi `DeleteWorkspace` ở Multica. Mới: `DELETE /api/workspaces/{id}/deletion`; việc nền `workspace_deletion` của Multica thực hiện.
- Test đỏ trước: khi `PLATFORM_WORKSPACE_DELETION_DAYS` bằng 0 hoặc không đặt thì xóa ngay (hồi quy); khi lớn hơn 0 thì ghi `pending_deletion` vào `platform_workspace` và xóa khóa cache; hủy khôi phục; hết hạn gọi đường xóa hiện có.

Kiểm: [D] `./internal/handler -run 'WorkspaceDeletion'`, [V]. Ước lượng: 2 ngày.

**P26. Hạn mức API theo workspace** [D] (spec 5.10) — độc lập

- Mới: `middleware.WorkspaceRateLimit`, đọc giới hạn từ cổng `api_rate_per_minute` của client quyền lợi.
- Test: vượt thì 429 kèm `Retry-After`; hai workspace không ảnh hưởng nhau; route daemon không bị tính; cổng vắng mặt hoặc `off` thì không giới hạn.

Kiểm: [GN] `./internal/middleware -run 'WorkspaceRateLimit'` với Redis giả, [V]. Ước lượng: 1,5 ngày.

**P27. Xuất dữ liệu workspace** [D] (spec 5.11) — độc lập

- Mới: migration 685, 686 (`workspace_export`, bảng thuộc platform); việc nền `workspace_export` và route ở Multica (`server/internal/workspaceexport/`).
- Test đỏ trước: chỉ owner; bản xuất không chứa `custom_env`, cấu hình MCP, token (so chuỗi); không chứa dòng của workspace khác; một việc xuất mỗi lúc; workspace `suspended` vẫn xuất được.

Kiểm: [PQ], [M], [D] `./internal/workspaceexport -run 'WorkspaceExport'`, [V]. Ước lượng: 4 ngày.

**P28. Số liệu sử dụng** [D] (spec 5.13) — độc lập

- Mới: migration 687, 688 (`platform_usage_daily`, bảng thuộc platform); việc nền hằng đêm ở Multica (`server/internal/usagesnapshot/`); `GET /api/platform/overview` và cột sử dụng ở danh sách tenant trong platform.
- Test: số đếm khớp fixture; chạy lại cùng ngày ghi đè, không cộng dồn.

Kiểm: [M], [D] `./internal/usagesnapshot -run 'Usage'`, [PD] `./internal/console -run 'Overview'`, [V]. Ước lượng: 2,5 ngày.

**P29. Ghi kiểm toán đợt R2** [D] (spec 5.14)

- Thêm `audit.Record` cho tích hợp, plugin, thư viện MCP, hồ sơ runtime, agent, runtime, xuất dữ liệu ở Multica; và cho thanh toán, trạng thái (`billing.*`, `subscription.changed`, `workspace.status_changed`, `account.status_changed`) ở platform, trong giao dịch của platform. Cập nhật spec 3.3 với các tệp handler chạm thêm.
- Test đỏ trước cho từng hành động.

Kiểm: [D] `./internal/handler -run 'Audit'`, [V]. Ước lượng: 4 ngày.

**P30. Cổng G-R2 và diễn tập bật thương mại**

- [T], [PG], [PW], [PB], [PI]; toàn bộ test Go của các gói đã chạm trên DB test, gồm [PD] cho module platform; đối chiếu AC7 đến AC14 và AC20.
- Diễn tập trên một bản sao dữ liệu không phải sản xuất: dựng stack với service `platform` (compose), chạy `server/cmd/migrate up`, gán thuê bao chuyển tiếp cho mọi workspace hiện hữu, rồi trỏ `MULTICA_CLOUD_URL` về platform, xác nhận không workspace nào bị chặn (spec mục 12). Kiểm thêm: platform thoát khi thiếu migration và tự khởi động lại được sau khi migrate; cổng 8095 không truy cập được từ ngoài mạng nội bộ; token `mcn_` bị từ chối. Ghi biên bản vào `docs/platform-operations.md`.
- Bảng kiểm triển khai cho hai kiểm tra không còn ở khởi động của Multica (spec 5.1): Multica có `REDIS_URL`; Multica không đặt biến mã đăng nhập phát triển và `APP_ENV=production`.
- Bổ sung tài liệu vận hành: quy trình đối soát chuyển khoản, xử lý quá hạn, khóa tenant, xuất danh sách đơn cho kế toán, thứ tự triển khai migrate rồi Multica và platform.
- Danh sách ra mắt: G-LIC, các mục ở spec 16, sao lưu cơ sở dữ liệu, giám sát.

Ước lượng: 3,5 ngày. **Tổng R2: khoảng 75,5 ngày công (ước lượng lại; trước đây 68,5).**

## 4. Pha C: R3, doanh nghiệp

**P31. Claim `amr`** [D] (spec 6.1)

- Đổi `issueJWT` (nhận cách xác thực), `RenewSessionToken` (chép `amr`).
- Test đỏ trước: mỗi đường đăng nhập đặt đúng `amr`; gia hạn giữ nguyên; token cũ không có `amr` được coi là `["otp"]`.

Kiểm: [D] `./internal/handler ./internal/auth -run 'AMR'`, [V]. Ước lượng: 1 ngày.

**P32. MFA** [D] (spec 6.2)

- Xác nhận U10: thêm `github.com/pquerna/otp` vào `server/go.mod` (D27), ghim phiên bản và kiểm giấy phép của thư viện. Mới: migration 689, 690; `server/internal/mfa/`; route đăng ký, xác nhận, tắt, `POST /auth/mfa/verify`; đổi `VerifyCode`, `GoogleLogin`.
- Test đỏ trước: AC15; token chờ bị `Auth` từ chối trên mọi route; quá số lần sai; mã dùng lại trong cùng cửa sổ; mã khôi phục dùng một lần; bí mật lưu ở dạng mã hóa; thiếu `PLATFORM_SECRET_KEY` thì tính năng tắt chứ không lưu bí mật trần.
- Client: màn hình đăng ký, bước nhập mã khi đăng nhập, web và desktop. Ghi việc cho mobile.

Kiểm: [Q], [M], [D] `./internal/mfa ./internal/handler -run 'MFA'`, [W], [V]. Ước lượng: 6 ngày.

**P33. Bắt buộc MFA cho staff; đặt lại MFA từ console** [D]

- Trong platform: đổi `RequirePlatformStaff`, thêm bước TOTP sau mã email (bí mật ở `user_mfa`, giải mã bằng `PLATFORM_SECRET_KEY`; spec 6.2); mới `POST /api/platform/users/{id}/mfa-reset`.
- Test đỏ trước: staff không có `mfa` trong `amr` nhận 404; đặt lại ghi kiểm toán và gửi email cho người dùng.
- Trước khi phát hành: mọi staff đăng ký MFA; ghi trình tự vào tài liệu vận hành để không tự khóa console.

Kiểm: [PD] `./internal/staff ./internal/console -run 'Staff.*MFA'`, [V]. Ước lượng: 1 ngày.

**P34. Chính sách theo workspace** [D] (spec 6.4)

- Mới: migration 691, 692; route `GET/PUT /api/workspaces/{id}/policy`; nhánh chính sách trong `workspaceguard.CheckWorkspaceAccess`; đổi `CreateInvitation`, `CreateShareLink`, `JoinByShareLink`.
- Test đỏ trước cho từng dòng của bảng ở spec 6.4, gồm: tắt link chia sẻ vô hiệu hóa link đang hoạt động; ba kiểm tra phiên chỉ chặn workspace đó; token máy được miễn; gói không có cờ `workspace_policies` nhận 402 `plan_feature_unavailable`.
- Client: tab chính sách trong cài đặt workspace; xử lý `mfa_required`, `reauth_required`.

Kiểm: [Q], [M], [D] `./internal/handler ./internal/middleware -run 'Policy'`, [W], [V]. Ước lượng: 4 ngày.

**P35. Xác minh tên miền** [D] (spec 6.3)

- Mới: migration 693 đến 695; route tên miền; bộ phân giải DNS tiêm được để test.
- Test đỏ trước: tên miền đã được workspace khác xác minh bị từ chối; tên miền email công cộng bị từ chối; bản ghi TXT sai không xác minh.

Kiểm: [Q], [M], [D] `./internal/sso -run 'Domain'`, [V]. Ước lượng: 2,5 ngày.

**P36. Kết nối OIDC, luồng đăng nhập, cấp tài khoản tức thời, bắt buộc SSO** [D] (spec 6.3)

- Xác nhận U10: thêm `github.com/coreos/go-oidc/v3` và `golang.org/x/oauth2` vào `server/go.mod` (D27), ghim phiên bản và kiểm giấy phép. Mới: migration 696, 697; `server/internal/sso/`; ba route `/auth/sso/*`; route kết nối. Test dùng một nhà cung cấp danh tính giả chạy trong tiến trình.
- Test đỏ trước: AC16, AC17; `state` hoặc `nonce` sai bị từ chối; `email_verified` sai bị từ chối; chữ ký sai bị từ chối; người mới được tạo và thêm vào workspace với `default_role`; hết ghế thì từ chối; owner không bị chặn khi `require_sso`; `client_secret` không bao giờ xuất hiện trong phản hồi hay nhật ký.
- Client: trang đăng nhập hỏi email trước; cấu hình SSO trong cài đặt workspace.

Kiểm: [Q], [M], [D] `./internal/sso ./internal/handler -run 'SSO'`, [W], [V]. Ước lượng: 9 ngày.

**P37. PAT giới hạn** [D] (spec 6.5)

- Mới: migration 698 (hai cột). Đổi `server/internal/auth/pat_cache.go` (cấu trúc và tiền tố khóa mới), `Auth`, `DaemonAuth`, `workspaceguard.CheckWorkspaceAccess`, `CreatePersonalAccessToken`.
- Test đỏ trước: AC18; PAT cũ giữ toàn quyền; mục cache theo tiền tố cũ bị bỏ qua.
- Client: hai tùy chọn khi tạo token.

Kiểm: [Q], [M], [D] `./internal/auth ./internal/middleware ./internal/handler -run 'PAT'`, [W], [V]. Ước lượng: 3 ngày.

**P38. Vai trò chỉ xem** [D] (spec 6.6)

- Mới: migration 699 (ba ràng buộc vai trò). Đổi `normalizeMemberRole`, lời mời, link chia sẻ, `workspaceguard.CheckWorkspaceAccess`, `requireDaemonWorkspaceAccess`; `packages/core/types/workspace.ts`, `rules.ts`, schema zod với fallback về `viewer`; giao diện thành viên.
- Test đỏ trước: AC19 bằng test duyệt bảng route (mọi route ghi của workspace, trừ danh sách cho phép, trả 403 cho viewer); viewer không đăng ký được runtime và không được ghi vào cache tư cách thành viên; quy tắc ai gán được vai trò nào; vai trò lạ ở client thành `viewer`.

Kiểm: [M], [D] `./internal/handler ./internal/middleware -run 'Viewer'`, [W], [T], [V]. Ước lượng: 5 ngày.

**P39. Mobile** [N]

- Đọc `apps/mobile/AGENTS.md`. Xác nhận U11. Cập nhật mobile cho: `session_revoked`, `account_suspended`, trạng thái workspace, bước MFA, đăng nhập SSO, vai trò `viewer`, mã lỗi hạn mức.

Kiểm: các lệnh ở `apps/mobile/AGENTS.md`. Ước lượng: 5 ngày.

**P40. Cổng G-R3**

- [T]; toàn bộ test Go của các gói đã chạm; đối chiếu AC15 đến AC19.
- Rà soát bảo mật theo bảng ở spec mục 11: mỗi dòng trỏ tới một test tên cụ thể.
- Cập nhật tài liệu người dùng trong `apps/docs/` cho SSO, MFA, chính sách, vai trò.

Ước lượng: 3 ngày. **Tổng R3: khoảng 39,5 ngày công.**

## 5. Pha D: R4

Không bước nào ở pha này bắt đầu viết mã trước khi đặc tả riêng của nó được viết, đối chiếu với mã và commit vào `docs/engineering/`. Hướng thiết kế của từng phần đã chốt ở spec mục 7.

| Bước | Việc | Điều kiện bắt đầu |
| --- | --- | --- |
| P41 | Đặc tả project hạn chế truy cập và khách mời (spec 7.1). Bước đầu là bản kiểm kê mọi đường đọc `issue` và `project`, kèm test phát hiện đường mới | G-R3 |
| P42 | Adapter đối soát chuyển khoản tự động (spec 7.2, D30). Bước đầu của đặc tả là đọc tài liệu API chính thức của nhà cung cấp và ghi hợp đồng thông báo, chữ ký, thử lại | G-R2 |
| P43 | Tích hợp hóa đơn điện tử (spec 7.3) | G-R2; xác nhận của kế toán về thông tin bắt buộc trên hóa đơn |
| P44 | Thanh toán quốc tế và nhiều tiền tệ (spec 7.4) | P42 |

## 6. Bảng truy vết yêu cầu

| Khoảng trống (spec 2.2) | Bước | Tiêu chí chấp nhận |
| --- | --- | --- |
| K1 khóa tài khoản | P2, P3 | AC1 |
| K2 thu hồi phiên | P4 | AC2 |
| K3 nhật ký kiểm toán | P5 đến P8, P29 | AC3 |
| K4 xóa và xuất tài khoản | P10, P11 | AC5, AC6 |
| K5 lộ email lời mời | P1 | AC4 |
| K6 staff và console | P14, P16, P17, P24 | AC8 |
| K7 khóa workspace | P15, P25 | AC9 |
| K8 gói và thuê bao | P18 | AC14 |
| K9 cổng hạn mức | P19, P20 | AC10, AC11, AC20 |
| K10 thanh toán | P21 đến P24 | AC12, AC13, AC14 |
| K11 hạn mức API, Redis | P13, P26 | AC7 |
| K12 mã đăng nhập phát triển | P13 | AC7 |
| K13 MFA, SSO | P32, P33, P35, P36 | AC15, AC16, AC17 |
| K14 chính sách theo workspace | P34 | AC17 |
| K15 PAT giới hạn | P37 | AC18 |
| K16 vai trò chỉ xem | P38 | AC19 |
| K17 project hạn chế | P41 | đặc tả riêng |

## 7. Tổng hợp ước lượng

| Pha | Ngày công (ước lượng) |
| --- | --- |
| P0 | 0,5 |
| R1 | 25 |
| R2 | 75,5 (ước lượng lại; trước đây 68,5) |
| R3 | 39,5 |
| R4 | chưa ước lượng; phụ thuộc đặc tả riêng |
| Tổng P0 đến R3 | 140,5 (ước lượng lại; trước đây 133,5) |

Số của R2 là ước lượng lại sau D31 đến D38. Các bước đổi: P13 từ 1,5 lên 3,5; P14 từ 2 lên 3,5; P15 từ 4 lên 4,5; P17 từ 6 lên 7,5; P18 từ 4 lên 4,5; P19 từ 6 lên 6,5; P30 từ 3 lên 3,5. Các bước khác của R2 giữ nguyên. P13 phải xong trước P14.

Các bước ghi "độc lập" (P26, P27, P28) có thể làm song song với chuỗi chính của pha B. P17 có thể làm song song với P18 đến P21 sau khi P16 xong.
