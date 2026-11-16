# Kế hoạch triển khai: platform management cho SaaS nhiều tenant

Đặc tả (thiết kế, quyết định, mô hình dữ liệu, API, mã lỗi, tiêu chí chấp nhận): [platform-management-spec.md](platform-management-spec.md). Tài liệu này là các bước, có thứ tự, kiểm được. Cả hai là nguồn duy nhất cho việc triển khai.

Ngày viết 2026-11-14, đọc ở **HEAD `1c631cf2c`**. **Chưa có mã nào được viết.** Đường dẫn ghi "mới" chưa tồn tại; "đổi" đã tồn tại ở HEAD này. Phong cách theo `flow-engine-generalization-plan.md`.

## 0. Cách đọc và quy ước

- **Thứ tự bắt buộc:** pha A (R1, nền tảng an toàn) → pha B (R2, thương mại) → pha C (R3, doanh nghiệp) → pha D (R4, mỗi phần cần đặc tả riêng). Đợt bổ sung (R5, năm nhóm tính năng, spec mục 7b) là các bước Q1 đến Q17 ở mục "Đợt bổ sung"; nó chạy sau G-R2, và riêng nhóm tài khoản (Q5 đến Q7) chạy sau G-R3. Trong một pha, các bước theo đúng số thứ tự trừ khi ghi "độc lập".
- **Cổng:**
  - **G0** có cơ sở dữ liệu test (mục 0.2). Không có thì mọi bước [D] đứng yên.
  - **G-R1** sau P12. **G-R2** sau P30. **G-R3** sau P40. **G-R5** sau Q17.
  - **G-LIC** có giấy phép thương mại (spec mục 0). Đứng trước **mọi** lần mở dịch vụ cho người ngoài tổ chức. Không chặn việc viết mã.
- Mỗi bước là một commit nguyên tử theo conventional commits: `feat(platform): …`, `feat(audit): …`, `fix(auth): …`, `test(platform): …`, `docs(platform): …`. PR theo mẫu của repo; PR có giao diện đính kèm ảnh chụp hoặc video (AGENTS.md "Change and Delivery Rules").
- **Test viết trước** (đỏ rồi xanh), đặt cạnh mã. Mỗi bước ghi test nào phải đỏ trước khi sửa.
- Nhãn kiểm: **[N]** không cần DB. **[D]** cần DB test.
- Ước lượng ngày công ghi cuối mỗi bước là ước lượng của tác giả thiết kế, chưa đo.
- Mọi quyết định D1 đến D38 đã chốt (spec mục 1.4); không bước nào chờ người dùng trả lời. Đợt bổ sung thêm D39 đến D48, trong đó D41, D43, D44, D46 thay một phần D21, D23, D13, D24 (các quyết định chốt theo ủy quyền, không phải do người dùng chốt); các giá trị chủ dự án phải đặt (spec 7b.7) đều có mặc định nên không bước nào chờ. Các giả định nền U1 đến U9 đã được xác nhận bằng đọc mã (spec mục 15.1). Bước nào xử lý phần còn lại được ghi "Xác nhận U<n>".
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

Hạng mục giữ dải **651 đến 699** (spec mục 8). Mỗi bước ghi số nó dùng. Phân bổ: R1 dùng 651 đến 662, R2 dùng 663 đến 688, R3 dùng 691 đến 699. Lệch khỏi phân bổ ban đầu: P27 dùng 685 và 687 (686 đã có), P28 dùng 689 và 690 (687 và 688 đã có), nên R3 bắt đầu từ 691. Nếu thiếu thì dùng dải dự phòng 953 đến 969. Đợt bổ sung R5 (mục "Đợt bổ sung") dùng 953 đến 968 và để 969 dự phòng; phân bổ từng số ở spec 7b.6 và ở từng bước Q. Mỗi lần thêm migration chạy `[GN] ./internal/migrations -run 'TestMigrationFilesHaveMatchingDirections|TestMigrationNumericPrefixesAreUnique'` và `[GN] ./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'`.

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

## Đợt bổ sung

Đợt R5: năm nhóm tính năng của spec mục 7b (quản lý thuê bao cá nhân và doanh nghiệp; nhóm tài khoản và quyền; gói không giới hạn và gói theo mức dùng; cấu hình giá từng gói; admin tạo tài khoản). Quyết định D39 đến D48 ở spec 1.4. Các bước đánh số `Q1` đến `Q17` để không trùng số P. Mọi quy ước ở mục 0 áp dụng: test đỏ trước, một bước một commit nguyên tử, nhãn kiểm ở mục 0.1, các điều cấm ở mục 0.2. Không bước Q nào sửa nội dung bước P; chỗ một bước Q đổi mã do bước P tạo thì ghi rõ tệp.

**Migration.** Đợt này dùng **953 đến 968**, và để 969 dự phòng. Lý do chọn dải này: dải 651 đến 699 đã phân bổ hết cho R1 đến R3 (R3 dùng đủ 689 đến 699) và một phần đã có tệp; ở cây làm việc lúc viết, các số 700 đến 704, 707 đến 709 và 720 đến 739 đã bị công việc khác chiếm (`ls server/migrations`), nên chỉ còn 705 và 706; dải 953 đến 969 là dải dự phòng đã khai ở mục 0.3, liền nhau, đủ 17 số, và đứng sau `plan` (670) và `workspace_subscription` (672) nên lệnh `ALTER TABLE` ở 953 chạy được trên cơ sở dữ liệu mới. Trước mỗi bước có migration, chạy lại `ls server/migrations | sed 's/_.*//' | sort -n | uniq` để chắc số chưa bị chiếm; nếu bị chiếm thì đổi số trong bước và trong spec 7b.6 cùng commit. Chín chỉ mục (955, 957, 958, 960, 962, 963, 965, 967, 968) đều có dòng dọn trong `concurrentIndexCleanups`.

**Điều kiện bắt đầu và thứ tự.**

| Bước | Điều kiện bắt đầu |
| --- | --- |
| Q1 | G-R2 (cần P18 và P24); P13 cho danh sách schemacheck |
| Q2, Q3 | Q1; P21 đến P23 |
| Q4 | Q3 |
| Q5 | G-R3 (cần P38 cho vai trò `viewer`); P20 cho cổng hạn mức; P11 cho sổ đăng ký cột người dùng |
| Q6 | Q5; Q2 và Q3 (Q6 đổi quyền của các route thanh toán do chúng tạo; nếu chúng chưa xong thì phần đó của Q6 làm sau khi chúng xong) |
| Q7 | Q6 |
| Q8 | Q1 |
| Q9 | Q2, Q8; P28 (`platform_usage_daily`) |
| Q10 | Q9; P21, P23 |
| Q11 | Q10 |
| Q12 | Q1; Q9 để nối tra giá theo ngày |
| Q13 | Q12 |
| Q14 | Q2; P14, P16 |
| Q15 | Q14 |
| Q16 | Q15; Q3 |
| Q17 | Q1 đến Q16 |

Nhóm 1 (Q1 đến Q4), nhóm 3 (Q8 đến Q11), nhóm 4 (Q12, Q13) và nhóm 5 (Q14 đến Q16) có thể chạy song song giữa các nhóm sau Q1 và Q2, vì chúng chỉ chung Q1 và Q2; nhóm 2 (Q5 đến Q7) độc lập với bốn nhóm kia, trừ việc đổi quyền route thanh toán ở Q6, và chạy sau G-R3. Trong một nhóm, đi theo thứ tự số.

**Q1. Nền mô hình gói và thuê bao mở rộng** [D] (spec 7b.1 dữ liệu, 7b.6)

- Test đỏ trước: (a) tệp nhúng bốn gói giữ đúng các giá trị ở spec 5.5 và nhận thêm `audience` (Free, Pro: `any`; Business, Enterprise: `business`), `billing_model = seat` và `member_groups` (Free 0, Pro 0, Business 50, Enterprise rỗng); (b) kiểm gói: `billing_model` lạ bị từ chối, gói `usage` có `seat_price_*` bị từ chối, `audience` lạ bị từ chối, `version` tăng một mỗi lần sửa; (c) `workspace_subscription` đọc và ghi `pending_change` và `unit_price`; hàng cũ không có hai cột này vẫn đọc được; (d) một test đọc tệp migration 953 và thất bại nếu phiên bản của nó không có trong danh sách của `internal/schemacheck` (mục 0.2, điều 13).
- Mới: `server/migrations/953_plan_subscription_r5.up.sql` và `.down.sql`: `ALTER TABLE plan ADD COLUMN IF NOT EXISTS audience, billing_model, usage_rates, usage_terms, version` và `ALTER TABLE workspace_subscription ADD COLUMN IF NOT EXISTS pending_change, unit_price`, giá trị mặc định theo spec 7b.1.
- Đổi: `platform-management/internal/plan/` (`plan.go`, `catalogue.json`, `store.go`, `pgx.go`), `platform-management/internal/subscription/` (`subscription.go`, `store.go`, `pgx.go`), `platform-management/queries/plan.sql` và `subscription.sql`, `platform-management/internal/schemacheck/`, mã sinh ở `platform-management/internal/db/`.
- Migration: 953.

Kiểm: [PQ], [M], [GN] `./internal/migrations -run 'TestMigrationFilesHaveMatchingDirections|TestMigrationNumericPrefixesAreUnique'`, [PG] `./internal/plan ./internal/subscription ./internal/schemacheck`, [PD] `./internal/plan ./internal/subscription -run 'Catalogue|Audience|Version|PendingChange'`. Ước lượng: 1,5 ngày (ước lượng). Tiêu chí: nền của AC21 và AC48.

**Q2. Hồ sơ thanh toán, loại tài khoản và kiểm `audience`** [D] (spec 7b.1 quy tắc 1 đến 4, 12, 13)

- Test đỏ trước: (a) workspace `individual` mua gói `business` bị 409 `plan_audience_mismatch`; chuyển sang `business` khi hồ sơ thiếu trường bị 409 `billing_profile_incomplete`; hồ sơ đủ thì mua được; chuyển ngược khi đang ở gói chỉ dành cho doanh nghiệp bị từ chối; (b) bảng ca mã số thuế: 10 chữ số đúng, 13 chữ số đúng, có dấu gạch, 9 chữ số, chữ cái, rỗng; (c) workspace `individual` đặt quá `PLATFORM_INDIVIDUAL_MAX_SEATS` ghế bị 409 `account_kind_required`, gói có `seats_fixed` không bị ảnh hưởng; (d) đổi `legal_name` hoặc `tax_code` xóa `verified_at`; (e) `support` không xác minh được, `billing` và `admin` xác minh được; (f) hồ sơ được chụp vào `billing_order.buyer` khi tạo đơn và sửa hồ sơ sau đó không đổi đơn cũ; (g) `GET /api/workspaces/{id}/plans` lọc theo `audience`.
- Mới: migration 954 (bảng `billing_profile`) và 955 (chỉ mục duy nhất `(workspace_id)`), kèm dòng dọn của 955 trong `concurrentIndexCleanups`; `platform-management/internal/billingprofile/` (kiểm, kho, xác minh); `platform-management/queries/billing_profile.sql`; route console `GET/PUT /tenants/{id}/billing-profile` và `POST .../verify`, `.../unverify` trong `platform-management/internal/console/`; thêm `account_kind` và `available_actions.edit_profile` vào `summary` ở `platform-management/internal/subscription/api.go`.
- Đổi ở Multica: `server/internal/handler/cloud_billing.go` (hai handler `GET` và `PUT /billing-profile` dùng `requireCloudSubscriptionWorkspace` và `proxyCloudSubscription`), `server/cmd/server/router.go` (hai route dưới `/api/cloud-subscriptions`), `GET /api/workspaces/{id}/plans` (lọc `audience`). Ở bước này quyền ghi là `owner` và `admin` như bảy route hiện có; Q6 đổi sang khóa quyền. Trước khi sửa `router.go`, chạy `git status --short -- server/cmd/server/router.go` (mục 0.2, điều 11).
- Migration: 954, 955. Thêm 954 và 955 vào danh sách của `internal/schemacheck`.

Kiểm: [PQ], [M], [GN] `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'`, [PG] `./internal/billingprofile`, [PD] `./internal/billingprofile ./internal/console -run 'BillingProfile|Verify'`, [D] `./internal/handler -run 'CloudSubscription.*Profile|WorkspacePlans'`, [V]. Ước lượng: 3 ngày (ước lượng). Tiêu chí: AC21; phần staff của AC25.

**Q3. Quản lý thuê bao tự phục vụ, phía platform và proxy của Multica** [D] (spec 7b.1 quy tắc 5 đến 11)

- Test đỏ trước: (a) bảng ca tiền của nâng gói (đầu kỳ, giữa kỳ, ngày cuối của kỳ, kỳ năm, ghế được nâng lên `min_seats`, `delta` bằng 0 và âm đi nhánh hạ gói, `free` và `trialing` đi nhánh `subscribe`), chạy trên hàm thuần; (b) lặp cùng `Idempotency-Key` trả cùng đơn, áp thanh toán hai lần chỉ đổi thuê bao một lần; (c) một đơn `pending` khác thì 409 `order_pending`; (d) số tiền chấp nhận lệch thì 409 `quote_changed`; (e) hạ gói, giảm ghế, đổi chu kỳ chỉ ghi `pending_change`, đơn `renew` kế tiếp tính theo nó và áp rồi xóa nó; giảm ghế dưới số thành viên không gỡ ai (I10); (f) hủy và tiếp tục theo bảng trạng thái, 409 `subscription_not_cancelable` ngoài `active` và `trialing`, hết kỳ về `free` không qua ân hạn; (g) `ApplyPayment` đặt `unit_price`; (h) mỗi thao tác một dòng kiểm toán; (i) tệp mẫu hợp đồng JSON của các route mới đi qua schema zod ở Q4.
- Mới, trong platform: `platform-management/internal/subscription/` (`quote.go`, `planchange.go`, `cancel.go`), `platform-management/internal/billingapi/` (route `plan-changes/preview`, `plan-changes`, `plan-changes/scheduled`, `cancel`, `resume` dưới `/api/v1/subscriptions/{ws}/`), tệp mẫu ở `platform-management/contract/testdata/`.
- Đổi, do P21 và P22 tạo: `platform-management/internal/billing/` (`ApplyPayment` đặt `unit_price` và áp `pending_change` khi đơn `renew` được thanh toán); `summary` thêm `pending_change` và `available_actions.change_plan`, `.cancel`, `.resume`.
- Đổi ở Multica: `server/internal/handler/cloud_billing.go` (năm handler mới), `server/cmd/server/router.go` (năm route dưới `/api/cloud-subscriptions`; quyền `owner` và `admin` ở bước này).
- Kiểm toán: thêm hằng `subscription.change_scheduled`, `subscription.change_canceled`, `subscription.cancel_requested`, `subscription.cancel_withdrawn` vào `platform-management/audit/audit.go` và `catalog_test.go`.
- Migration: không.

Kiểm: [PG] `./internal/subscription -run 'Proration|Quote'`, [PD] `./internal/subscription ./internal/billing ./internal/billingapi -run 'PlanChange|Cancel|Resume|ApplyPayment'`, [D] `./internal/handler -run 'CloudSubscription.*(PlanChange|Cancel|Resume)'`, [V]. Ước lượng: 6 ngày (ước lượng). Tiêu chí: AC22, AC23, AC24.

**Q4. Giao diện quản lý thuê bao** [N] (spec 7b.1 màn hình)

- Test đỏ trước: schema zod của các phản hồi mới có test phản hồi sai dạng (`parseWithFallback`, AGENTS.md "API Compatibility") và test phản hồi **không** có trường mới (Multica Cloud) vẫn hợp lệ; test views: bộ chọn gói lọc theo `audience`, hộp thoại xem trước hiện số tiền hay ngày hiệu lực theo `mode`, nút hủy và tiếp tục theo `available_actions`, biểu mẫu hồ sơ với công tắc cá nhân và doanh nghiệp, biểu ngữ `pending_change`, mọi điều khiển ghi ẩn khi `available_actions` vắng mặt.
- Đổi: `packages/core/api/schemas.ts` và `client.ts`; hook và mutation trong `packages/core/` (cạnh code thanh toán hiện có); `packages/views/settings/components/billing-tab.tsx`; mới: `packages/views/billing/` (hộp thoại đổi gói, biểu mẫu hồ sơ thanh toán); chuỗi giao diện bảy ngôn ngữ, không mô tả thừa (mục 0.2, điều 9). Nối cả web và desktop. Mobile không có trang thanh toán nên không đổi; ghi vào PR.

Kiểm: [B], [W], [T]. PR đính kèm ảnh chụp hoặc video. Ước lượng: 5 ngày (ước lượng). Tiêu chí: giao diện của AC21 đến AC24; AC48 (client ẩn chức năng khi trường mới vắng mặt).

**Q5. Nhóm tài khoản: bảng, kho dữ liệu, API quản lý, danh mục khóa và cổng đếm** [D] (spec 7b.2 quy tắc 1 đến 6, 12 đến 14)

- Test đỏ trước: (a) quy tắc tên (độc nhất không phân biệt hoa thường, độ dài), thành viên bắt buộc là thành viên workspace (`not_a_member`), giới hạn 1.000 thành viên, khóa lạ bị 400 `permission_unknown` và khóa lạ đã lưu bị bỏ qua khi tính quyền; (b) quyền hiệu lực là hợp của vai trò và các nhóm, `billing.manage` kéo theo `billing.view`; (c) cổng `member_groups`: giới hạn 0 trả 402 `plan_feature_unavailable`, vượt trả 402 `plan_limit_reached`, hai request đồng thời ở sát giới hạn chỉ một thành công, hạ gói giữ nguyên nhóm, `MULTICA_CLOUD_URL` rỗng thì không giới hạn; (d) mỗi thay đổi đúng một dòng kiểm toán (quyền có giá trị cũ và mới), và khi `audit.Record` lỗi thì thay đổi không lưu; (e) id nhóm của workspace khác trả 404 `group_not_found`; (f) test sổ đăng ký cột người dùng (P11) thất bại cho tới khi ba cột mới được phân loại; (g) `permissions/me`.
- Mới: migration 956 (`member_group`), 957 (chỉ mục duy nhất `(workspace_id, id)`), 958 (chỉ mục duy nhất `(workspace_id, lower(name))`), 959 (`member_group_member`), 960 (chỉ mục duy nhất `(workspace_id, user_id, group_id)`), kèm dòng dọn cho 957, 958, 960; `server/pkg/db/queries/member_group.sql` rồi [Q]; `server/internal/membergroup/` (danh mục khóa là hằng, kho, dịch vụ); `server/internal/handler/member_group.go`; các route ở spec 7b.2 trong `server/cmd/server/router.go`, đều sau `RequireHumanActor`.
- Đổi: `server/internal/entitlement/types.go` và `client.go` (thêm `GateMemberGroups`; cổng vắng mặt vẫn `off`); `platform-management/internal/plan/catalogue.json` và `platform-management/internal/entitlement/` (trả cổng `member_groups` theo bảng gói); `platform-management/audit/audit.go` và `catalog_test.go` (sáu hành động `group.*`); sổ đăng ký cột người dùng `platform-management/accountdeletion/registry.go` (ba cột mới, kèm lý do).
- Migration: 956 đến 960. Không thêm vào schemacheck của platform (các bảng này thuộc Multica); nhưng `platform-management/internal/console` đọc số đếm nhóm ở Q7, nên đọc có kiểm lỗi khi bảng chưa tồn tại.

Kiểm: [Q], [M], [D] `./internal/membergroup ./internal/handler -run 'MemberGroup|PermissionsMe'`, [GN] `./internal/migrations -run 'TestUserColumnRegistry'`, [GN] `./internal/entitlement -run 'MemberGroups'`, [GN] `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'`, [PG] `./audit`, [PD] `./internal/entitlement -run 'Gate'`, [V]. Ước lượng: 5 ngày (ước lượng). Tiêu chí: AC29, AC30; phần sổ đăng ký của AC28.

**Q6. Cưỡng chế quyền theo nhóm** [D] (spec 7b.2 quy tắc 7 đến 11 và danh mục khóa)

- Test đỏ trước: (a) ma trận khóa × route (bộ 11, spec mục 13): với mỗi khóa trong sáu khóa, đúng tập route đã ghi mở cho người có khóa và không route nào khác mở; route ngoài danh mục (đổi vai trò, gỡ thành viên, cập nhật và xóa workspace, plugin, thư viện MCP, quản lý nhóm) vẫn chỉ owner và admin; (b) viewer thuộc nhóm có đủ sáu khóa vẫn bị chặn ở mọi phương thức ghi và đọc được theo khóa; (c) người chỉ có quyền nhóm bị chặn khi workspace `suspended` và `pending_deletion`, owner và admin vẫn dùng được route cho phép; (d) người có `members.invite` mời vai trò `admin` bị 403 `invite_role_not_assignable`, mời `member` và `viewer` được, và không đổi vai trò hay gỡ được ai; (e) bớt khỏi nhóm thì request kế tiếp bị 403 `permission_denied`; (f) gỡ thành viên, rời workspace, xóa tài khoản xóa dòng nhóm trong cùng giao dịch, và khi giao dịch gỡ lỗi thì dòng nhóm còn nguyên; (g) token máy và agent: không đổi so với hôm nay, và route quản lý nhóm 403 với token máy; (h) route mới thuộc nhóm owner và admin sau này mặc định vẫn đóng.
- Đổi: `server/internal/middleware/workspace.go` (thêm `RequireWorkspacePermissionFromURL` và `RequireWorkspacePermission`: owner và admin qua ngay, vai trò khác tra `membergroup` một truy vấn); `server/cmd/server/router.go` (tách các nhóm route owner và admin theo khóa, spec 7b.2; đổi quyền các route `/api/cloud-subscriptions` và route đơn hàng của Q2, Q3 sang `billing.view` và `billing.manage`); `server/internal/handler/invitation.go` (`CreateInvitation`, kiểm vai trò ở dòng `normalizeMemberRole`) và `share_link.go` (`CreateShareLink`); `server/internal/handler/workspace_revoke.go` (`revokeAndRemoveMember` xóa dòng nhóm); `server/internal/handler/handler.go` chỉ nếu cần hàm trợ giúp. Cập nhật spec 3.3 nếu chạm thêm tệp. Trước khi sửa `router.go`, `invitation.go`, `workspace_revoke.go`, chạy `git status --short -- <tệp>` và `git log -3 --format='%h %s' -- <tệp>` (mục 0.2, điều 11).
- Migration: không.

Kiểm: [D] `./internal/middleware ./internal/handler -run 'Permission|MemberGroupRoutes|Invitation|ShareLink|Revoke|ViewerGroup'`; toàn bộ test hiện có của thành viên, lời mời, link chia sẻ và tích hợp xanh; [GN] `./internal/workspaceguard -run 'RouterWires'`; [V]. Ước lượng: 5 ngày (ước lượng). Tiêu chí: AC25, AC26, AC27, AC28. Đây là bước rủi ro cao nhất của nhóm 2, vì nó tách các nhóm route hiện có.

**Q7. Giao diện nhóm tài khoản** [N] (spec 7b.2 màn hình)

- Test đỏ trước: schema zod và test phản hồi sai dạng cho `groups`, `permission-catalogue`, `permissions/me`; `rules.ts`: `can(key)` theo `permissions/me`, và quyền không rõ là không có; views: tab "Groups" ẩn với `member`; thành viên có `members.invite` thấy điều khiển mời nhưng không thấy chọn vai trò `admin`; thành viên có `audit.read` thấy tab nhật ký; viewer không thấy điều khiển ghi dù thuộc nhóm.
- Mới: `packages/core/groups/` (query, mutation, schema), `packages/views/settings/components/groups-tab.tsx` và hộp thoại tạo, sửa; đổi: `packages/core/permissions/rules.ts`, `packages/views/settings/components/members-tab.tsx` (nhóm của từng người, điều khiển theo khóa), các tab nhật ký, thanh toán, tích hợp, hồ sơ runtime (kiểm khóa thay vì vai trò); `platform-management/internal/console/` (`GET /tenants/{id}` thêm `group_count`, chỉ số đếm) và console hiện số nhóm; chuỗi giao diện bảy ngôn ngữ. Mobile: ghi vào PR rằng mobile chỉ cần hiểu `permission_denied`; không đổi mobile ở R5.

Kiểm: [B], [W], [PW], [T]. PR đính kèm ảnh chụp. Ước lượng: 4 ngày (ước lượng). Tiêu chí: giao diện của AC26 và AC30; console không lộ tên nhóm (AC30).

**Q8. Gói không giới hạn, trần tốc độ API và chính sách quyền lợi** [D] (spec 7b.3 quy tắc 1 đến 4, 15)

- Test đỏ trước: (a) tệp nhúng có thêm hai gói `unlimited` và `payg` đúng bảng ở spec 7b.3 (so từng ô); (b) với gói `unlimited`, chính sách quyền lợi trả `action = off` cho mọi cổng đếm và `summary.limits` là `unlimited`; (c) `api_rate_per_minute` rỗng ở gói và ở `limit_overrides` được trả thành `PLATFORM_API_RATE_CEILING` với `action = enforce`, và giá trị tường minh cao hơn trần được dùng nguyên; (d) tạo issue, agent, runtime, tệp, nhóm ở mức vượt mọi gói khác không bị chặn trên gói `unlimited` (client quyền lợi hiện có trỏ vào handler platform); (e) hạ từ `unlimited` xuống gói có giới hạn không xóa dòng nào; (f) `GET /prices` của workspace đang ở gói `usage` trả giá ghế của gói theo ghế công khai rẻ nhất, không bao giờ trả giá của gói `usage`.
- Đổi: `platform-management/internal/plan/catalogue.json` (hai gói mới), `platform-management/internal/plan/plan.go` (`EffectiveLimit` nhận trần), `platform-management/internal/entitlement/`, `platform-management/internal/subscription/api.go` (`prices`), `platform-management/internal/config/` (`PLATFORM_API_RATE_CEILING`).
- Migration: không.

Kiểm: [PG] `./internal/plan ./internal/config`, [PD] `./internal/plan ./internal/entitlement ./internal/subscription -run 'Unlimited|Ceiling|Prices|Catalogue'`, [D] `./internal/service ./internal/entitlement -run 'Entitlement|IssueLimit|AutopilotQuota'` với client trỏ vào platform, [V]. Ước lượng: 3 ngày (ước lượng). Tiêu chí: AC31; phần `prices` của AC32.

**Q9. Đo và chấm tiền theo mức dùng, kích hoạt gói theo mức dùng** [D] (spec 7b.3 quy tắc 5 đến 10)

- Bước đầu: định nghĩa "task chạy" của `platform_usage_daily.tasks_run` ở P28 (đề xuất: task hoàn tất theo ngày UTC của `completed_at`). Nếu spec 5.13 chưa ghi, sửa spec 5.13 trong cùng commit và thêm test đối chiếu `tasks_run`, `storage_bytes`, `members` với bảng nguồn trên fixture. Đây là rủi ro 15.3(d).
- Test đỏ trước: (a) bảng ca tiền của hàm thuần (spec 7b.3 quy tắc 8): ba đồng hồ, tháng 28, 29, 30, 31 ngày, giảm giá 0, 20 và 100 phần trăm, số lượng lên tới 100 TB không tràn số, làm tròn nửa lên đúng ở biên; (b) giảm giá Business chỉ áp khi `account_kind = business` và hồ sơ đã xác minh khi cờ bật, áp cả khi chưa xác minh nếu cờ tắt; (c) mỗi (workspace, ngày, đồng hồ) đúng một dòng; chạy `usage_rating` hai lần và song song không sinh dòng thứ hai; (d) ngày chưa có số liệu không bị chấm, dòng đến muộn được chấm khi đến; (e) ngày workspace `suspended` hoặc `pending_deletion` ghi dòng 0 đồng với `note`, ngày `read_only` vẫn tính; (f) kích hoạt gói `payg`: hồ sơ đủ thì tạo thuê bao `active`, `seats = 0` và đơn `subscribe` 0 đồng `paid`, `seat_capacity` là `null`, lời mời không bị chặn vì ghế; hồ sơ thiếu thì 409 `billing_profile_incomplete`; (g) dòng giữ ảnh chụp giá và tỷ lệ.
- Mới: migration 961 (`usage_charge`), 962 (chỉ mục duy nhất `(workspace_id, day, meter)`), 963 (chỉ mục `(order_id)` có điều kiện), kèm dòng dọn cho 962 và 963; `platform-management/internal/usage/` (`rating.go` hàm thuần dùng `math/big`, `job.go`, kho); `platform-management/queries/usage_charge.sql`; đăng ký việc nền `usage_rating` ở `platform-management/internal/jobs`; thêm 961 đến 963 vào `internal/schemacheck`.
- Đổi: `platform-management/internal/billingapi/` (nhánh `checkout-sessions` cho gói `usage`), `platform-management/internal/subscription/` (kích hoạt kỳ tháng dương lịch UTC).
- Migration: 961, 962, 963.

Kiểm: [PQ], [M], [PG] `./internal/usage -run 'Rating|Math'`, [PD] `./internal/usage ./internal/billingapi -run 'UsageRating|PaygActivation'`, [V]. Ước lượng: 6 ngày (ước lượng). Tiêu chí: AC32, AC33.

**Q10. Hóa đơn theo mức dùng, hạn mức công nợ và nợ** [D] (spec 7b.3 quy tắc 11 đến 14, 16)

- Bước đầu: đọc migration `billing_order` do P21 tạo. Nếu có `CHECK` trên `kind`, thêm migration 969 nới ràng buộc để nhận `usage`; nếu không thì để 969 dự phòng.
- Test đỏ trước: (a) hóa đơn tháng gồm mọi dòng chưa lập hóa đơn của ngày trước đầu tháng, số tiền bằng tổng, VAT đúng `BILLING_VAT_RATE`, `expires_at` rỗng, các dòng được gắn `order_id` trong cùng giao dịch; (b) tổng dưới `PLATFORM_USAGE_MIN_INVOICE` dồn sang kỳ sau; (c) hóa đơn cuối được lập, bỏ mức tối thiểu, khi thuê bao về `free` hay sang gói theo ghế; (d) chạy `usage_invoicing` hai lần và song song chỉ lập một đơn; (e) đạt hạn mức công nợ thì lập hóa đơn tạm; email 80 phần trăm gửi một lần mỗi tháng; (f) đơn quá hạn đưa thuê bao đi `past_due`, `read_only`, `free` theo đồng hồ giả và dữ liệu không mất; thanh toán đưa về `active`; (g) khi còn nợ, mua gói trả phí bị 409 `usage_invoice_unpaid`, gói `free` vẫn dùng được; (h) chỉ `admin` kèm lý do hủy được đơn `usage`, `billing` bị từ chối, dòng của đơn bị hủy không vào hóa đơn khác; (i) tệp mẫu hợp đồng JSON của `usage` trong `summary` và của `GET /usage`; (j) `GET /api/workspaces/{id}/plans` trả `usage_rates` và `effective_discount_pct` (0 khi chưa xác minh).
- Mới: `platform-management/internal/usage/` (`invoicing.go`), kind `usage` và `billing_notice` kind `usage_80` ở `platform-management/internal/billing/`; route platform `GET /api/v1/subscriptions/{ws}/usage`.
- Đổi: `platform-management/internal/lifecycle/` (nhánh đơn `usage` chưa trả), `platform-management/internal/console/` (hủy đơn `usage` chỉ `admin` kèm lý do; `GET /tenants/{id}/usage`), `platform-management/internal/subscription/api.go` (`billing_model`, `usage`), `platform-management/audit/audit.go` (ba hành động `billing.usage_*`); ở Multica `server/internal/handler/cloud_billing.go` (`GET /usage`), route đơn hàng `GET /api/workspaces/{id}/orders/{orderId}/usage`, `GET /api/workspaces/{id}/plans` (đọc `billing_profile`), `server/cmd/server/router.go`; quyền `billing.view` (đã có từ Q6).
- Migration: không, hoặc 969 như bước đầu.

Kiểm: [PG] `./internal/usage -run 'Invoice|Math'`, [PD] `./internal/usage ./internal/lifecycle ./internal/billing ./internal/console -run 'UsageInvoice|UsageLifecycle|WriteOff'`, [D] `./internal/handler -run 'CloudSubscriptionUsage|OrderUsage|WorkspacePlans'`, [M] chỉ khi có 969, [V]. Ước lượng: 6 ngày (ước lượng). Tiêu chí: AC34, AC35.

**Q11. Giao diện mức dùng: khách và console** [N] (spec 7b.3 màn hình)

- Test đỏ trước: schema zod và test phản hồi sai dạng cho `usage` của `summary`, `GET /usage` và `plans` mở rộng; views: thẻ mức dùng ở các trạng thái (chưa có dữ liệu, dưới và gần hạn mức công nợ), dòng gói theo mức dùng chỉ hiện phần giảm Business khi `effective_discount_pct` dương, danh sách hóa đơn có đơn `usage` với chi tiết dòng theo ngày, biểu ngữ hóa đơn chưa trả; console: tab mức dùng, ngày thiếu số liệu, nút xóa nợ chỉ hiện với `admin` và đòi lý do.
- Đổi: `packages/core/api/schemas.ts`, `packages/views/settings/components/billing-tab.tsx`, `packages/views/billing/` (thẻ mức dùng, chi tiết hóa đơn); `platform-management/console/` (tab mức dùng ở chi tiết tenant); chuỗi bảy ngôn ngữ.

Kiểm: [B], [W], [PW], [PB], [T]. PR đính kèm ảnh chụp. Ước lượng: 4 ngày (ước lượng). Tiêu chí: giao diện của AC33 và AC35.

**Q12. Lịch thay đổi giá của gói** [D] (spec 7b.4)

- Test đỏ trước: (a) chỉ `admin` lập và hủy, `billing` đọc lịch sử, `support` và người không có phiên nhận 404; (b) `PUT /plans/{key}` có trường giá bị 400 `price_field_via_schedule`; (c) hai yêu cầu lập đồng thời cho một gói chỉ một cái thành công, cái kia 409 `price_change_pending` (cả bằng khóa dòng lẫn bằng chỉ mục duy nhất có điều kiện); (d) bảng ca `price_invalid`: âm, thập phân, giá năm quá 12 lần giá tháng, vượt `PLATFORM_PRICE_MAX`, gói `free` khác 0, gói `usage` có giá ghế, khóa đồng hồ lạ, `per` bằng 0, `discount_pct` ngoài 0 đến 100, `effective_from` của giá theo mức dùng không đúng 00:00 UTC; (e) đổi quá `PLATFORM_PRICE_CONFIRM_PCT` đòi `confirm_large_change`; (f) tiền tệ bị khóa khi đã có thuê bao hoặc đơn; (g) tăng giá cho gói có thuê bao trả phí đòi `effective_from` không sớm hơn `PLATFORM_PRICE_NOTICE_DAYS` ngày, giảm giá và gói chưa có thuê bao thì có hiệu lực ngay trong request; (h) `plan_price_apply` lũy đẳng, song song áp một lần, `version` tăng một lần; (i) đơn đã tạo giữ số tiền, kỳ đã trả không đổi, đơn tạo từ hiệu lực dùng giá mới, báo giá cũ bị 409 `quote_changed`; (j) email báo gửi đúng một lần cho mỗi owner, không gửi cho workspace `pending_deletion`, thư đính chính khi hủy sau khi đã báo; (k) tra giá của một ngày đã qua theo spec 7b.4 quy tắc 9 và chấm muộn sau khi giá đổi vẫn dùng giá của ngày đó; (l) `GET /api/workspaces/{id}/plans` chỉ trả `scheduled_price_change` cho gói đang dùng và chỉ sau khi thư báo đã gửi; (m) `?dry_run=1` không lưu gì và trả số thuê bao bị ảnh hưởng.
- Mới: migration 964 (`plan_price_change`) và 965 (chỉ mục duy nhất `(plan_key)` có điều kiện `state = 'scheduled'`, kèm dòng dọn); `platform-management/internal/planprice/` (`validate.go` hàm thuần, kho, lập lịch, áp, thư báo); `platform-management/queries/plan_price_change.sql`; việc nền `plan_price_apply`; thêm 964, 965 vào `internal/schemacheck`.
- Đổi: `platform-management/internal/plan/` (từ chối trường giá ở `PUT`, `POST /plans` cho gói mới), `platform-management/internal/console/` (route `/plans*` ở spec 7b.4), `platform-management/internal/usage/` (tra giá theo ngày dùng lịch sử), `platform-management/internal/billing/` (so báo giá), `GET /api/workspaces/{id}/plans` của Multica, `platform-management/audit/audit.go` (ba hành động `plan.price_*`).
- Migration: 964, 965.

Kiểm: [PQ], [M], [PG] `./internal/planprice -run 'Validate|Notice|RatesOn'`, [PD] `./internal/planprice ./internal/console ./internal/usage -run 'PriceChange|PlanPrice|RatesOn'`, [D] `./internal/handler -run 'WorkspacePlans'`, [V]. Ước lượng: 6 ngày (ước lượng). Tiêu chí: AC36, AC37, AC38, AC39, AC40.

**Q13. Console cấu hình gói và giá; hiển thị giá sắp đổi cho khách** [N] (spec 7b.4 màn hình)

- Test đỏ trước: schema zod và test phản hồi sai dạng cho `price-changes` và `scheduled_price_change`; console: màn hình Gói chỉ hiện với `admin`, biểu mẫu giá có xem trước tác động bằng `dry_run`, cảnh báo khi đổi quá ngưỡng, hiện `min_effective_from` khi 409, `billing` chỉ thấy lịch sử; Multica: biểu ngữ giá sắp đổi cho owner và `billing.view`.
- Đổi: `platform-management/console/` (màn hình Gói, lập lịch giá, lịch sử), `packages/core/api/schemas.ts`, `packages/views/settings/components/billing-tab.tsx`; chuỗi bảy ngôn ngữ cho phần của Multica.

Kiểm: [PW], [PB], [B], [W], [T]. PR đính kèm ảnh chụp. Ước lượng: 4 ngày (ước lượng). Tiêu chí: giao diện của AC36, AC38, AC40.

**Q14. Yêu cầu tạo tài khoản: bảng, API console, kiểm đầu vào, hạn mức** [D] (spec 7b.5 quy tắc 1 đến 6, 11, 12)

- Test đỏ trước: (a) chỉ `admin` gọi được các route `/accounts`, `support` và `billing` nhận 404; (b) bảng ca đầu vào: email sai định dạng, quá dài, trùng hoa thường, thiếu hồ sơ khi `business`; (c) email đã thuộc một `user` bị 409 `account_email_exists` và không sửa gì; (d) gửi lại cùng email khi yêu cầu còn sống trả chính yêu cầu đó với 200; hai yêu cầu đồng thời cùng email chỉ một cái được ghi (chỉ mục duy nhất có điều kiện); (e) gói sai `audience` bị 409 `plan_audience_mismatch`; gói trả phí thiếu cả `trial` lẫn `comp_reason` bị 400 `account_activation_required`; `trial` với gói không có `trial_days` bị từ chối; `individual` quá `PLATFORM_INDIVIDUAL_MAX_SEATS` bị từ chối; (f) hạn mức `PLATFORM_ACCOUNT_CREATE_PER_HOUR` theo staff với Redis giả, 429 `account_create_rate_limited`; (g) hủy chỉ khi `pending`; (h) mỗi bước một dòng kiểm toán; thất bại ghi kiểm toán thì yêu cầu không được tạo (I7); (i) xóa tài khoản (P11) ẩn danh hóa `email` và `display_name` của yêu cầu có `user_id` đó.
- Mới: migration 966 (`platform_account_request`), 967 (chỉ mục duy nhất `(id)`), 968 (chỉ mục duy nhất `(email)` có điều kiện `state IN ('pending', 'provisioned')`), kèm dòng dọn cho 967 và 968; `platform-management/internal/accountrequest/` (kiểm, kho, API); `platform-management/queries/account_request.sql`; route `/api/platform/accounts*` ở `internal/console/`; hằng kiểm toán `account.*` ở `platform-management/audit/audit.go`; thêm 966 đến 968 vào `internal/schemacheck`.
- Đổi: bước xóa tài khoản của P11 (job ở `platform-management/accountdeletion/` hoặc nơi P11 đặt nó) thêm ẩn danh hóa `platform_account_request`; sổ đăng ký cột người dùng (`platform-management/accountdeletion/registry.go`) phân loại `created_by` và `user_id` là `keep`.
- Migration: 966, 967, 968.

Kiểm: [PQ], [M], [PG] `./internal/accountrequest -run 'Validate'`, [PD] `./internal/accountrequest ./internal/console -run 'AccountRequest|AccountCreate'`, [D] hoặc [PD] `-run 'AccountRequest'` trên gói của job xóa tài khoản (theo nơi P11 đặt nó), [V]. Ước lượng: 4 ngày (ước lượng). Tiêu chí: AC43 (phần kiểm đầu vào), AC44.

**Q15. Cấp phát phía Multica: tách `workspaceprovision`, việc nền `account_provisioning`** [D] (spec 7b.5 quy tắc 7 bước 2, 8)

- Bước đầu: đọc `server/internal/handler/workspace.go` (`CreateWorkspace`, tác dụng phụ sau commit: analytics, `h.Knowledge.AutoEnable`, `notifyDaemonWorkspacesChanged`) và mã tri thức để xác nhận lượt chạy hằng giờ tự bật cho workspace chưa gọi `AutoEnable`; ghi kết quả vào spec 15.3(f) và sửa phần "tác dụng phụ" của bước này theo đó.
- Test đỏ trước: (a) hồi quy: toàn bộ test `CreateWorkspace` hiện có xanh trước và sau khi tách; (b) việc nền tạo `user` (tên theo `display_name`), workspace, owner, trạng thái issue, đánh dấu onboarded, ghi `user_id`, `workspace_id`, đặt `provisioned`; (c) slug: dùng `workspace_slug` nếu có, trùng hay dành riêng thì `failed` `slug_unavailable`; suy từ tên (bỏ dấu), trùng thì thêm hậu tố ngẫu nhiên, tối đa 5 lần; (d) email đã có hay miền bị chặn thì `failed` `email_exists`, `email_domain_blocked`, không tạo gì; (e) hai việc nền song song không tạo hai người dùng hay hai workspace (`SKIP LOCKED`, ràng buộc `user.email`); chạy hai lần liên tiếp không đổi kết quả; (f) lỗi tạm thời thử tối đa 3 lần rồi `failed`; (g) người dùng tạo ra đăng nhập được bằng mã email khi `ALLOW_SIGNUP` tắt, bằng đúng đường `findOrCreateUser` (trả người dùng có sẵn); (h) không có dòng `member` hay phiên nào của staff; (i) kiểm toán `workspace.created` và `account.provisioned` (actor `system`, có `workspace_id`).
- Mới: `server/internal/workspaceprovision/` (hàm giao dịch dùng chung), `server/internal/accountprovision/` (việc nền), `server/pkg/db/queries/platform_account_request.sql` rồi [Q]; đăng ký ở `server/cmd/server/main.go` bằng `schedulerMgr.Register`.
- Đổi: `server/internal/handler/workspace.go` (`CreateWorkspace` gọi `workspaceprovision.Create`, hành vi giữ nguyên); cập nhật spec 3.3 nếu chạm thêm tệp. Trước khi sửa `workspace.go` và `main.go`, chạy `git status --short -- <tệp>` và `git log -3 --format='%h %s' -- <tệp>` (mục 0.2, điều 11).
- Migration: không.

Kiểm: [Q], [D] `./internal/workspaceprovision ./internal/accountprovision ./internal/handler -run 'Workspace|AccountProvision'`, [V]. Ước lượng: 5 ngày (ước lượng). Tiêu chí: AC41, AC43, AC45.

**Q16. Hoàn thiện phía platform, thư chào và console tạo tài khoản** [D] (spec 7b.5 quy tắc 7 bước 3, 9, 10, 13)

- Test đỏ trước: (a) `account_request_finalize` ghi `billing_profile` (với `business`: `verified_at` và `verified_by`), thuê bao theo từng kiểu kích hoạt (gói `free` không có thuê bao; `trial`; `comp_reason` một kỳ `interval`; gói `usage`) kèm đơn 0 đồng `admin_grant`, tăng `version`, xóa cache, xóa bản sao hồ sơ trong yêu cầu, đặt `completed`; (b) lũy đẳng và song song; lỗi tạm thời thử tối đa 3 lần; lỗi sau bước 2 để lại `finalize_failed` và giữ người dùng, workspace; (c) thư chào gửi đúng một lần, thử lại tối đa 3 lần khi lỗi gửi, gửi lại tối đa 3 lần mỗi ngày mỗi yêu cầu; nội dung không chứa mã, mật khẩu hay liên kết có token và chỉ có địa chỉ `PLATFORM_APP_URL`; (d) kịch bản đầu cuối trên cơ sở dữ liệu test với đồng hồ giả: tạo yêu cầu, chạy việc nền của Multica (Q15) rồi việc nền của platform, người dùng đăng nhập bằng mã email, thấy workspace và gói (bộ 14, spec mục 13); (e) tài khoản tạo ra bị khóa, buộc đăng xuất và xóa như mọi tài khoản; (f) owner đọc được `account.provisioned` trong nhật ký workspace; console không có route đọc nội dung workspace đó; (g) UI console: biểu mẫu nhập email hai lần, danh sách yêu cầu, lỗi, nút hủy và gửi lại.
- Mới: `platform-management/internal/accountrequest/` (`finalize.go`, `welcome.go`), mẫu thư chào ở `platform-management/internal/mailer/`, đăng ký việc nền `account_request_finalize` ở `platform-management/internal/jobs`, route `POST /accounts/{id}/resend-welcome`; `platform-management/console/` (màn hình Tạo tài khoản và danh sách yêu cầu).
- Migration: không.

Kiểm: [PG], [PD] `./internal/accountrequest ./internal/mailer ./internal/console -run 'Finalize|Welcome|AccountEndToEnd'`, [D] `./internal/accountprovision -run 'EndToEnd'`, [PW], [PB], [T], [V]. PR đính kèm ảnh chụp. Ước lượng: 5 ngày (ước lượng). Tiêu chí: AC41, AC42, AC45, AC46, AC47.

**Q17. Cổng G-R5: tích hợp, hồi quy, bảo mật và tài liệu**

- [T], [PG], [PW], [PB], [PI]; toàn bộ test Go của các gói đã chạm trên cơ sở dữ liệu test, gồm [PD] cho module platform; đối chiếu AC21 đến AC48.
- Hồi quy với mọi cấu hình mới để trống và với `MULTICA_CLOUD_URL` rỗng hoặc trỏ tới Multica Cloud (AC48): route mới trả đúng lỗi của bảy route hiện có, client ẩn chức năng khi trường mới vắng mặt, nhóm tài khoản chạy không giới hạn số nhóm.
- Rà soát bảo mật theo các bảng của spec 7b: mỗi dòng trỏ tới một test tên cụ thể (như P40).
- Kiểm tra 3.3 khớp với mọi tệp lõi đã chạm trong R5; kiểm tra danh sách schemacheck có đủ 953, 954, 955, 961 đến 968; chuỗi giao diện đủ bảy ngôn ngữ; không mô tả thừa.
- Tài liệu: `apps/docs/` cho thuê bao, nhóm tài khoản và gói theo mức dùng; thêm vào `docs/platform-operations.md` quy trình tạo tài khoản cho khách, xóa nợ, đổi giá và báo trước, và bảng kiểm trước khi bật R5 (chạy `server/cmd/migrate up` cho 953 đến 968 trước khi khởi động lại Multica và platform).
- Cập nhật spec 15.1 hoặc 15.3 nếu Q9 và Q15 đã xác nhận các điều chưa kiểm chứng.

Ước lượng: 3 ngày (ước lượng). **Tổng R5: khoảng 75,5 ngày công (ước lượng).** Theo nhóm: nhóm 1 (Q1 đến Q4) 15,5; nhóm 2 (Q5 đến Q7) 14; nhóm 3 (Q8 đến Q11) 19; nhóm 4 (Q12, Q13) 10; nhóm 5 (Q14 đến Q16) 14; cổng (Q17) 3.

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
| K17 | project hạn chế | P41 | đặc tả riêng |
| K18 quản lý thuê bao cá nhân và doanh nghiệp | Q1, Q2, Q3, Q4 | AC21, AC22, AC23, AC24, AC25 |
| K19 nhóm tài khoản và quyền | Q5, Q6, Q7 | AC25, AC26, AC27, AC28, AC29, AC30 |
| K20 gói không giới hạn, gói theo mức dùng | Q8, Q9, Q10, Q11 | AC31, AC32, AC33, AC34, AC35 |
| K21 cấu hình giá từng gói | Q12, Q13 | AC36, AC37, AC38, AC39, AC40 |
| K22 tạo tài khoản bởi admin | Q14, Q15, Q16 | AC41, AC42, AC43, AC44, AC45, AC46, AC47 |
| hồi quy R5 (mọi cấu hình mới để trống; nguồn thương mại khác) | Q1, Q4, Q17 | AC48 |

## 7. Tổng hợp ước lượng

| Pha | Ngày công (ước lượng) |
| --- | --- |
| P0 | 0,5 |
| R1 | 25 |
| R2 | 75,5 (ước lượng lại; trước đây 68,5) |
| R3 | 39,5 |
| R4 | chưa ước lượng; phụ thuộc đặc tả riêng |
| R5 (đợt bổ sung, Q1 đến Q17) | 75,5 (ước lượng) |
| Tổng P0 đến R3, chưa có đợt bổ sung (số cũ) | 140,5 (ước lượng lại; trước đây 133,5) |
| **Tổng P0 đến R3 cộng R5 (số mới)** | **216** (140,5 + 75,5; tăng 75,5) |

Số của R2 là ước lượng lại sau D31 đến D38. Các bước đổi: P13 từ 1,5 lên 3,5; P14 từ 2 lên 3,5; P15 từ 4 lên 4,5; P17 từ 6 lên 7,5; P18 từ 4 lên 4,5; P19 từ 6 lên 6,5; P30 từ 3 lên 3,5. Các bước khác của R2 giữ nguyên. P13 phải xong trước P14.

Các bước ghi "độc lập" (P26, P27, P28) có thể làm song song với chuỗi chính của pha B. P17 có thể làm song song với P18 đến P21 sau khi P16 xong.

Số của R5 là ước lượng mới, không có số cũ để so: Q1 1,5; Q2 3; Q3 6; Q4 5; Q5 5; Q6 5; Q7 4; Q8 3; Q9 6; Q10 6; Q11 4; Q12 6; Q13 4; Q14 4; Q15 5; Q16 5; Q17 3. Tổng 75,5 ngày công. Tổng của các pha trước giữ nguyên; tổng chung tăng từ 140,5 lên 216. Phần giao diện (Q4, Q7, Q11, Q13) chiếm 17 ngày và có thể làm song song với phần máy chủ của nhóm khác khi API của nhóm đó đã có tệp mẫu hợp đồng.
