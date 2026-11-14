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
- Mọi quyết định D1 đến D30 đã chốt (spec mục 1.4); không bước nào chờ người dùng trả lời. Các giả định nền U1 đến U9 đã được xác nhận bằng đọc mã (spec mục 15.1). Bước nào xử lý phần còn lại được ghi "Xác nhận U<n>".
- Việc nền nào cũng là một `scheduler.JobSpec` đăng ký ở `server/cmd/server/main.go` với tên ở spec mục 3.5; test của việc nền gọi thẳng `Handler` của nó hai lần liên tiếp để chứng minh lũy đẳng.

### 0.1 Lệnh chuẩn

| Mã | Lệnh | Ghi chú |
| --- | --- | --- |
| **[B]** | `pnpm catalog:build` | Sinh mã từ `catalog/`; chạy trước `vitest`, `tsc`, `go test` chạy trực tiếp. Không commit mã sinh |
| **[Q]** | `make sqlc` | Sau mọi sửa `server/pkg/db/queries/*.sql`; commit cả `server/pkg/db/generated/*` |
| **[GN]** | `( cd server && go test -count=1 <gói> -run '<mẫu>' )` | Nhãn [N]. Luôn kèm `-run` khi gói có test DB |
| **[D]** | `DATABASE_URL='<dsn-test>' bash scripts/require-test-database.sh && ( cd server && DATABASE_URL='<dsn-test>' go test -count=1 -p 1 <gói> -run '<mẫu>' )` | DSN do người dùng cấp, đặt tường minh trong chính lệnh |
| **[M]** | `DATABASE_URL='<dsn-test>' bash scripts/require-test-database.sh && ( cd server && DATABASE_URL='<dsn-test>' go run ./cmd/migrate up )` | Sau mỗi bước thêm migration |
| **[W]** | `pnpm --filter @multica/core exec vitest run <mẫu>`; `pnpm --filter @multica/views exec vitest run <mẫu>` | Nhãn [N]; chạy [B] trước |
| **[T]** | `pnpm typecheck`, `pnpm lint`, `pnpm test` | Gốc repo, không gồm mobile |
| **[V]** | `( cd server && go build ./... && go vet ./internal/platform/... ./internal/audit/... ./internal/handler/... ./internal/middleware/... ./cmd/... )` | Sau mỗi bước Go |
| **[E]** | `pnpm exec playwright test` | **Không chạy** trừ khi người dùng cho phép |

### 0.2 Cổng G0 và những điều không được làm

**G0.** Người dùng tạo một cơ sở dữ liệu test có tên **kết thúc `_test`** và **khác `multica`**, rồi cấp DSN. Mọi lệnh cần DB chạy sau guard `scripts/require-test-database.sh`. Trước G0, bước [D] chỉ viết test và PR ghi "chưa chạy".

Không được làm:

1. Không chạy `make test`, `make check`, `make migrate-up`, `make up`, `make down`, `make destroy`, `go test` trần hay `go run ./cmd/migrate up` trần. Không tạo cơ sở dữ liệu. Không ghi vào cơ sở dữ liệu `multica`.
2. Không khóa ngoại, không cascade. Bảng mới không có `PRIMARY KEY` hay `UNIQUE` nội tuyến. Mọi chỉ mục `CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS`, một chỉ mục một tệp một câu lệnh, có dòng dọn tương ứng trong `concurrentIndexCleanups` của `server/cmd/migrate/main.go`.
3. Không sửa `server/pkg/db/queries/*.sql` mà không chạy [Q] và commit mã sinh.
4. Không commit mã sinh của catalog.
5. Không sửa tệp ngoài danh sách điểm móc ở spec mục 3.3 mà không cập nhật danh sách đó trong cùng commit.
6. Không thêm đường dự phòng, ghi đôi hay bộ chuyển đổi tạm. Chế độ `local` thay hẳn client Cloud; không chạy song song hai nguồn quyết định (spec I4).
7. Không nhân đôi quy tắc quyền giữa Go và TypeScript ngoài phần phản chiếu ở `packages/core/permissions/rules.ts`.
8. Không đụng `apps/mobile/` mà chưa đọc `apps/mobile/AGENTS.md`.
9. Mọi chuỗi giao diện mới đủ bảy ngôn ngữ; không mô tả thừa (`conventions.mdx` "UI descriptions").
10. Chỉ đẩy lên `origin` (`TuPK/multica`). Không mở PR lên `multica-ai/multica`; PR nếu có chỉ dùng `--repo TuPK/multica`.
11. Trước mỗi bước chạm một tệp ở spec mục 3.3, chạy `git status --short -- <tệp>` và `git log -3 --format='%h %s' -- <tệp>`. Nếu có sửa chưa commit của công việc khác thì chờ; không hoàn tác thay đổi của người khác. Lúc viết kế hoạch, cây làm việc có sửa chưa commit ở `packages/views/company/components/*` và `server/internal/issueworkflow/*`; không bước nào ở đây chạm các tệp đó.

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

- Mới: migration 651 (bảng), 652 (chỉ mục duy nhất `user_id`), 653 (chỉ mục `status` có điều kiện); `server/pkg/db/queries/platform_account.sql`; `server/internal/auth/account_guard.go` và test.
- Test [N] với Redis giả và DB giả: trượt cache đọc DB rồi ghi cache; kết quả "không có dòng" cũng được cache; Redis lỗi thì đọc DB; DB lỗi thì trả lỗi; guard `nil` cho qua; ghi trạng thái xóa cache.

Kiểm: [Q], [M], [GN] `./internal/auth -run 'AccountGuard'`, [V]. Ước lượng: 1,5 ngày.

**P3. Nối guard vào `Auth`, `DaemonAuth` và WebSocket** [D] (spec 4.1)

- Test đỏ trước: ma trận năm loại token × bốn trạng thái tài khoản (spec mục 13, bộ 1). Với `mdt_`, kiểm theo chủ runtime; token không có chủ được miễn. `pending_deletion` chỉ cho ba route trong danh sách.
- Đổi `server/internal/middleware/auth.go`, `daemon_auth.go`, `server/internal/realtime/hub.go`, `server/cmd/server/router.go` (truyền guard). Khi trạng thái chuyển sang khóa, ngắt WebSocket qua `Hub.DisconnectMember` cho từng workspace.
- Giữ nguyên `temporary_disabled_users.go`.

Kiểm: [D] `./internal/middleware ./internal/realtime -run 'AccountGuard|Suspended'`; toàn bộ test hiện có của hai gói xanh; [V]. Ước lượng: 2 ngày.

**P4. Buộc đăng xuất mọi thiết bị** [D] (spec 4.2)

- Test đỏ trước: (a) token cấp trước mốc bị 401 `session_revoked`; (b) token cũ đã gia hạn trước mốc vẫn bị loại; (c) token không có `auth_time` xét theo `iat`; (d) gia hạn chép `auth_time` nguyên vẹn; (e) phiên gọi lệnh nhận token mới dùng được.
- Đổi `server/internal/handler/auth.go` (`issueJWT`), `server/internal/auth/session.go` (`RenewSessionToken`), `server/internal/middleware/auth.go` (so mốc trước bước gia hạn). Mới: `POST /api/me/sessions/revoke-all` sau `RequireHumanActor`.
- Client: nút trong trang cài đặt tài khoản, web và desktop; xử lý `session_revoked` như hết phiên. Ghi việc cần làm cho mobile.

Kiểm: [D] `./internal/handler ./internal/middleware ./internal/auth -run 'Session|Revoke'`, [W], [V]. Ước lượng: 2 ngày.

**P5. Bảng `audit_event` và gói `audit`** [D] (spec 4.3)

- Mới: migration 654 (bảng), 655 đến 657 (ba chỉ mục); `server/pkg/db/queries/audit_event.sql`; `server/internal/audit/` (`Record`, bộ lọc bí mật, kiểu `Entry`, hằng tên hành động).
- Test [N]: bộ lọc thay giá trị của khóa nhạy cảm ở mọi độ sâu; test đọc tệp `.sql` và thất bại nếu có `UPDATE` ngoài câu ẩn danh hóa.
- Test [D]: `Record` trong giao dịch bị hủy không để lại dòng.

Kiểm: [Q], [M], [GN] và [D] `./internal/audit`, [V]. Ước lượng: 1,5 ngày.

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

Kiểm: [D] `./internal/handler ./internal/platform -run 'AccountDeletion|UserColumnRegistry'`, [W], [V]. Ước lượng: 5 ngày.

**P12. Cổng G-R1**

- Chạy [T] và, trên DB test, toàn bộ `./internal/handler ./internal/middleware ./internal/auth ./internal/audit ./internal/realtime`.
- Đối chiếu AC1 đến AC6.
- Đo độ trễ p95 của một route đọc đơn giản trước và sau khi có guard (spec 15.3); ghi số đo vào PR.
- Viết `docs/platform-operations.md`: khóa và mở tài khoản bằng SQL khi chưa có console, đọc nhật ký, xử lý yêu cầu xóa.

Ước lượng: 1,5 ngày. **Tổng R1: khoảng 25 ngày công.**

## 3. Pha B: R2, thương mại

**P13. Chế độ `local`, khung gói `platform`, kiểm tra khi khởi động** [N] (spec 3.4, 5.1)

- Mới: `server/internal/platform/` (`Service`, `Config`); `Handler.Platform`. Đổi `server/cmd/server/router.go` và `main.go`.
- Test đỏ trước, mỗi cấu hình sai ở spec 3.4 một ca: máy chủ trả lỗi khởi động (AC7). Ca không đặt gì: hành vi như HEAD.

Kiểm: [GN] `./cmd/server -run 'PlatformMode'`, [V]. Ước lượng: 1,5 ngày.

**P14. Staff và `RequirePlatformStaff`** [D] (spec 5.2)

- Mới: migration 663, 664; `server/pkg/db/queries/platform_staff.sql`; middleware; `GET /api/platform/me`; `GET /api/me` thêm `platform_role`.
- Test đỏ trước (AC8): người thường 404; PAT của staff 404; token máy 404; `auth_time` cũ hơn 12 giờ 404; IP ngoài dải 404; email trong `PLATFORM_ADMIN_EMAILS` qua; staff đã thu hồi 404; ma trận vai trò × route.

Kiểm: [Q], [M], [D] `./internal/platform ./internal/handler -run 'Staff'`, [V]. Ước lượng: 2 ngày.

**P15. Trạng thái workspace** [D] (spec 5.3)

- Mới: migration 665 đến 667; query; `platform.CheckWorkspaceAccess`; cache.
- Đổi: `buildMiddleware`, `requireWorkspaceMember`, và `claimRefusalFor` ở `server/internal/service/budget_gate.go` (D25). `TaskService` nhận một giao diện `WorkspaceGate` có một phương thức; gán ở `router.go` khi `Platform` khác `nil`, để trống thì cổng cho qua như hiện tại. Kiểm tra trạng thái đứng **trước** bước phân loại lần chạy, vì lần chạy do người kích hoạt hiện được cho qua vô điều kiện.
- Test đỏ trước: ma trận trạng thái × phương thức × danh sách cho phép trên cả hai đường (bộ 2); với workspace không `active`, `ClaimTask` không trả task nào cho cả ba loại lần chạy (người kích hoạt, định kỳ, ủy thác) và task vẫn ở trạng thái xếp hàng; đưa workspace về `active` thì nhận được ngay; task đang chạy hoàn tất được; `WorkspaceGate` để trống thì hành vi như HEAD; AC9.

Kiểm: [D] `./internal/middleware ./internal/handler ./internal/service -run 'WorkspaceState|ClaimRefusal'`; toàn bộ test daemon và test cổng ngân sách hiện có xanh; [V]. Ước lượng: 4 ngày.

**P16. API console: tenant, người dùng, ghi chú, staff** [D] (spec 5.4)

- Mới: `server/internal/handler/platform_*.go`; migration 668, 669 (`platform_note`); route dưới `/api/platform`.
- Test đỏ trước: mỗi hành động ghi kiểm toán với `actor_type = staff` và, khi nhắm một workspace, có `workspace_id`; owner của workspace đọc được dòng đó; đổi trạng thái thiếu lý do bị 400; test duyệt handler cấm query bảng nội dung (bộ 6).

Kiểm: [D] `./internal/handler -run 'PlatformConsole|PlatformIsolation'`, [V]. Ước lượng: 4 ngày.

**P17. Giao diện console** [N]

- Thêm `platform` vào `server/internal/handler/reserved_slugs.json`; chạy `pnpm generate:reserved-slugs`; commit `packages/core/paths/reserved-slugs.ts`.
- Mới: `packages/core/platform-admin/` (query, mutation, schema zod, test phản hồi sai dạng); `apps/web/app/platform/` (danh sách và chi tiết tenant, người dùng, staff, nhật ký, tổng quan).
- Lối vào chỉ hiện khi `platform_role` có giá trị.

Kiểm: [W], [T]. PR đính kèm ảnh chụp. Ước lượng: 6 ngày.

**P18. Gói, thuê bao và Provider tại chỗ** [D] (spec 5.5)

- Mới: migration 670 đến 673 (`plan`, `workspace_subscription` và chỉ mục); tệp nhúng danh mục gói; `platform.EntitlementProvider`; nối ở `router.go` thay client Cloud khi `local`.
- Tệp nhúng mang đúng bảng gói ở spec 5.5 (D24); một test so tệp với bảng đó để hai nơi không lệch nhau.
- Test đỏ trước: workspace không có dòng là gói miễn phí; `limit_overrides` ghi đè; giá trị rỗng là không giới hạn và trả `Action = off`; cổng có giới hạn trả `Action = enforce` với `Limit` không âm; `autopilot_runs` luôn kèm `PeriodStart`, `PeriodEnd`, `ResetAt` của tháng dương lịch UTC, kể cả ở ranh giới tháng và năm (D28); `version` tăng và được trả làm `SubscriptionVersion`; cache hết hạn sau 30 giây và bị xóa khi đổi; workspace không `active` trả giới hạn 0 cho `autopilot_runs`. Với Provider tại chỗ, tạo lần chạy autopilot từ từng nguồn `schedule`, `manual`, `webhook`, `api` đều bị chặn khi hết hạn mức (xác nhận U5). Toàn bộ test hiện có của cổng `issue_count` và `autopilot_runs` chạy lại với Provider tại chỗ và xanh.

Kiểm: [Q], [M], [D] `./internal/platform ./internal/service -run 'Entitlement|IssueLimit|AutopilotQuota'`, [V]. Ước lượng: 4 ngày.

**P19. Executor ghế tại chỗ** [D] (spec 5.6)

1. Xác nhận U3: đọc nốt `server/internal/seatcapacity/worker.go` (worker gọi phương thức nào cho từng loại intent, có giữ `WorkspaceLocker` khi gọi không, dùng giá trị nào của `Operation.State`). Viết bộ test hợp đồng mã hóa bảng và bảy quy tắc của spec Phụ lục B, chạy trên bản giả đang dùng trong `server/internal/handler/invitation_test.go`. Bộ này phải xanh trước khi viết cài đặt mới; nếu một quy tắc không viết được cho xanh trên bản giả thì sửa Phụ lục B trong cùng commit rồi mới đi tiếp.
2. Mới: migration 674 đến 676; `platform.SeatExecutor`. Chạy cùng bộ hợp đồng trên nó.
3. Nối ở `router.go`. Chạy lại toàn bộ test lời mời và link chia sẻ với Executor tại chỗ.

- Test thêm: lũy đẳng theo `operation_token`; hai lời mời đồng thời ở ghế cuối chỉ một cái được; giữ chỗ hết hạn được giải phóng; gói không giới hạn trả `Managed = false`; vượt chỗ trả lỗi thỏa `IsCapacityOvercommitted`, không gỡ ai và không cho thêm (AC11); thành viên có từ trước khi bật chế độ `local` được tính vào số đã dùng; gọi Executor trong khi đang giữ khóa của `WorkspaceLocker` không treo (quy tắc 6 của Phụ lục B).

Kiểm: [D] `./internal/platform ./internal/handler -run 'Seat|Invitation|ShareLink'`, [V]. Ước lượng: 6 ngày. Đây là bước rủi ro cao nhất của pha B.

**P20. Cổng hạn mức mới và giới hạn cấp tài khoản** [D] (spec 5.7)

- Đổi `server/internal/entitlement/types.go`; `CreateAgent`, đường khôi phục agent, `DaemonRegister`, `UploadFile` và các đường tạo tệp đính kèm; `CreateWorkspace`; `checkSignupAllowed`.
- Test đỏ trước cho từng cổng: dưới giới hạn qua; tại giới hạn 402 `plan_limit_reached` với `gate`, `limit`, `used`; hai request đồng thời ở sát giới hạn chỉ một thành công (AC10); không có Provider thì không giới hạn. Giới hạn số workspace trên gói miễn phí; tên miền bị chặn.
- Client: mở rộng hộp thoại nâng cấp hiện có cho mã lỗi mới.

Kiểm: [D] `./internal/handler -run 'PlanLimit|WorkspaceCreation|Signup'`, [W], [V]. Ước lượng: 4 ngày.

**P21. Lõi thanh toán** [D] (spec 5.8)

- Mới: migration 677 đến 682 (`billing_order` và chỉ mục); `platform/billing` (định giá, tính theo tỷ lệ, `PaymentProvider`, cài đặt `manual`, `ApplyPayment`).
- Test [N]: bảng ca định giá và tính theo tỷ lệ, gồm biên đầu kỳ, cuối kỳ, năm nhuận, tiền tệ không có phần thập phân.
- Test [D] đỏ trước: xác nhận hai lần chỉ áp một lần (AC12); hai xác nhận đồng thời; đơn hết hạn không xác nhận được; khóa lũy đẳng trả lại đơn cũ; thanh toán đưa workspace `read_only` vì quá hạn về `active`.

Kiểm: [Q], [M], [GN] và [D] `./internal/platform/billing`, [V]. Ước lượng: 5 ngày.

**P22. API thanh toán tại chỗ và giao diện đơn hàng** [D]

- Đổi bảy handler trong `server/internal/handler/cloud_billing.go` theo bảng ánh xạ ở spec 5.8 và hợp đồng ở Phụ lục A. Mới: `GET /api/workspaces/{id}/plans`; route đơn hàng của workspace; trang đơn hàng trong `packages/views/billing/`, nối web và desktop.
- Đổi client (D29): `packages/core/api/schemas.ts` nới `StripeHostedURLSchema` để nhận `http` chỉ với máy chủ `localhost` hoặc `127.0.0.1`, và đọc trường tùy chọn `payment_provider`; `packages/views/settings/components/billing-tab.tsx` thêm bộ chọn gói, đổi nhãn nút và bỏ bước xử lý tham số quay về của Stripe khi `payment_provider` là `manual`. Khi trường vắng mặt, hành vi như hiện tại.
- Test đỏ trước: phản hồi của bảy handler đi qua schema zod của client (bộ 4; dùng tệp mẫu JSON chung giữa test Go và test TypeScript), gồm các ca biên của Phụ lục A: workspace miễn phí chưa có dòng thuê bao (`seat_capacity.version` phải dương), workspace không giới hạn ghế (`seat_capacity` là `null`), có đơn mua ghế đang chờ (`pending_quantity` và `active_purchase`); `currency` là chữ thường; schema từ chối `http` với máy chủ khác máy cục bộ và từ chối `javascript:`; thành viên thường đọc được tóm tắt nhưng không tạo được đơn; token máy 403; đổi gói qua `plan_key` tạo đúng loại đơn.

Kiểm: [D] `./internal/handler -run 'CloudSubscription.*Local|Order|Plans'`, [W], [T], [V]. PR đính kèm ảnh chụp (AC14). Ước lượng: 7 ngày.

**P23. Vòng đời thuê bao và thông báo** [D] (spec 5.9)

- Mới: migration 683, 684 (`billing_notice`); job mỗi giờ; mẫu email nhắc và email chuyển trạng thái.
- Test đỏ trước với đồng hồ giả: mỗi cạnh của sơ đồ; chạy job hai lần liên tiếp và hai job song song không chuyển hai lần và không gửi trùng email (bộ 5); thanh toán ở từng bước đưa về `active` (AC13); số dòng dữ liệu của fixture không đổi qua cả chuỗi.

Kiểm: [Q], [M], [D] `./internal/platform -run 'Lifecycle'`, [V]. Ước lượng: 4 ngày.

**P24. Console: thuê bao, đơn hàng, danh mục gói** [D]

- Mới: các route `PUT /tenants/{id}/subscription`, `/orders*`, `/plans` và màn hình tương ứng.
- Test đỏ trước: vai trò `support` không xác nhận được đơn; đổi gói ghi kiểm toán có giá trị cũ và mới; gói đã có thuê bao không xóa được.

Kiểm: [D] `./internal/handler -run 'PlatformBilling'`, [W], [V]. Ước lượng: 4 ngày.

**P25. Xóa workspace có thời gian chờ** [D] (spec 5.12)

- Đổi `DeleteWorkspace`. Mới: `DELETE /api/workspaces/{id}/deletion`; job thực hiện.
- Test đỏ trước: ngoài chế độ `local` vẫn xóa ngay (hồi quy); trong `local` chuyển `pending_deletion`; hủy khôi phục; hết hạn gọi đường xóa hiện có.

Kiểm: [D] `./internal/handler -run 'WorkspaceDeletion'`, [V]. Ước lượng: 2 ngày.

**P26. Hạn mức API theo workspace** [D] (spec 5.10) — độc lập

- Mới: `middleware.WorkspaceRateLimit`.
- Test: vượt thì 429 kèm `Retry-After`; hai workspace không ảnh hưởng nhau; route daemon không bị tính; không có cờ của gói thì dùng mặc định.

Kiểm: [GN] `./internal/middleware -run 'WorkspaceRateLimit'` với Redis giả, [V]. Ước lượng: 1,5 ngày.

**P27. Xuất dữ liệu workspace** [D] (spec 5.11) — độc lập

- Mới: migration 685, 686; việc xuất chạy nền; route.
- Test đỏ trước: chỉ owner; bản xuất không chứa `custom_env`, cấu hình MCP, token (so chuỗi); không chứa dòng của workspace khác; một việc xuất mỗi lúc; workspace `suspended` vẫn xuất được.

Kiểm: [Q], [M], [D] `./internal/platform -run 'WorkspaceExport'`, [V]. Ước lượng: 4 ngày.

**P28. Số liệu sử dụng** [D] (spec 5.13) — độc lập

- Mới: migration 687, 688; job hằng đêm; `GET /api/platform/overview` và cột sử dụng ở danh sách tenant.
- Test: số đếm khớp fixture; chạy lại cùng ngày ghi đè, không cộng dồn.

Kiểm: [D] `./internal/platform -run 'Usage'`, [V]. Ước lượng: 2,5 ngày.

**P29. Ghi kiểm toán đợt R2** [D] (spec 5.14)

- Thêm `audit.Record` cho tích hợp, plugin, thư viện MCP, hồ sơ runtime, agent, runtime, thanh toán, trạng thái, xuất dữ liệu. Cập nhật spec 3.3 với các tệp handler chạm thêm.
- Test đỏ trước cho từng hành động.

Kiểm: [D] `./internal/handler -run 'Audit'`, [V]. Ước lượng: 4 ngày.

**P30. Cổng G-R2 và diễn tập bật thương mại**

- [T]; toàn bộ test Go của các gói đã chạm trên DB test; đối chiếu AC7 đến AC14.
- Diễn tập trên một bản sao dữ liệu không phải sản xuất: gán thuê bao chuyển tiếp cho mọi workspace hiện hữu, bật `PLATFORM_MODE=local`, xác nhận không workspace nào bị chặn (spec mục 12). Ghi biên bản vào `docs/platform-operations.md`.
- Bổ sung tài liệu vận hành: quy trình đối soát chuyển khoản, xử lý quá hạn, khóa tenant, xuất danh sách đơn cho kế toán.
- Danh sách ra mắt: G-LIC, các mục ở spec 16, sao lưu cơ sở dữ liệu, giám sát.

Ước lượng: 3 ngày. **Tổng R2: khoảng 68,5 ngày công.**

## 4. Pha C: R3, doanh nghiệp

**P31. Claim `amr`** [D] (spec 6.1)

- Đổi `issueJWT` (nhận cách xác thực), `RenewSessionToken` (chép `amr`).
- Test đỏ trước: mỗi đường đăng nhập đặt đúng `amr`; gia hạn giữ nguyên; token cũ không có `amr` được coi là `["otp"]`.

Kiểm: [D] `./internal/handler ./internal/auth -run 'AMR'`, [V]. Ước lượng: 1 ngày.

**P32. MFA** [D] (spec 6.2)

- Xác nhận U10: thêm `github.com/pquerna/otp` vào `server/go.mod` (D27), ghim phiên bản và kiểm giấy phép của thư viện. Mới: migration 689, 690; `server/internal/platform/mfa/`; route đăng ký, xác nhận, tắt, `POST /auth/mfa/verify`; đổi `VerifyCode`, `GoogleLogin`.
- Test đỏ trước: AC15; token chờ bị `Auth` từ chối trên mọi route; quá số lần sai; mã dùng lại trong cùng cửa sổ; mã khôi phục dùng một lần; bí mật lưu ở dạng mã hóa; thiếu `PLATFORM_SECRET_KEY` thì tính năng tắt chứ không lưu bí mật trần.
- Client: màn hình đăng ký, bước nhập mã khi đăng nhập, web và desktop. Ghi việc cho mobile.

Kiểm: [Q], [M], [D] `./internal/platform/mfa ./internal/handler -run 'MFA'`, [W], [V]. Ước lượng: 6 ngày.

**P33. Bắt buộc MFA cho staff; đặt lại MFA từ console** [D]

- Đổi `RequirePlatformStaff`; mới `POST /api/platform/users/{id}/mfa-reset`.
- Test đỏ trước: staff không có `mfa` trong `amr` nhận 404; đặt lại ghi kiểm toán và gửi email cho người dùng.
- Trước khi phát hành: mọi staff đăng ký MFA; ghi trình tự vào tài liệu vận hành để không tự khóa console.

Kiểm: [D] `./internal/handler -run 'Staff.*MFA'`, [V]. Ước lượng: 1 ngày.

**P34. Chính sách theo workspace** [D] (spec 6.4)

- Mới: migration 691, 692; route `GET/PUT /api/workspaces/{id}/policy`; nhánh chính sách trong `platform.CheckWorkspaceAccess`; đổi `CreateInvitation`, `CreateShareLink`, `JoinByShareLink`.
- Test đỏ trước cho từng dòng của bảng ở spec 6.4, gồm: tắt link chia sẻ vô hiệu hóa link đang hoạt động; ba kiểm tra phiên chỉ chặn workspace đó; token máy được miễn; gói không có cờ `workspace_policies` nhận 402 `plan_feature_unavailable`.
- Client: tab chính sách trong cài đặt workspace; xử lý `mfa_required`, `reauth_required`.

Kiểm: [Q], [M], [D] `./internal/handler ./internal/middleware -run 'Policy'`, [W], [V]. Ước lượng: 4 ngày.

**P35. Xác minh tên miền** [D] (spec 6.3)

- Mới: migration 693 đến 695; route tên miền; bộ phân giải DNS tiêm được để test.
- Test đỏ trước: tên miền đã được workspace khác xác minh bị từ chối; tên miền email công cộng bị từ chối; bản ghi TXT sai không xác minh.

Kiểm: [Q], [M], [D] `./internal/platform/sso -run 'Domain'`, [V]. Ước lượng: 2,5 ngày.

**P36. Kết nối OIDC, luồng đăng nhập, cấp tài khoản tức thời, bắt buộc SSO** [D] (spec 6.3)

- Xác nhận U10: thêm `github.com/coreos/go-oidc/v3` và `golang.org/x/oauth2` vào `server/go.mod` (D27), ghim phiên bản và kiểm giấy phép. Mới: migration 696, 697; `server/internal/platform/sso/`; ba route `/auth/sso/*`; route kết nối. Test dùng một nhà cung cấp danh tính giả chạy trong tiến trình.
- Test đỏ trước: AC16, AC17; `state` hoặc `nonce` sai bị từ chối; `email_verified` sai bị từ chối; chữ ký sai bị từ chối; người mới được tạo và thêm vào workspace với `default_role`; hết ghế thì từ chối; owner không bị chặn khi `require_sso`; `client_secret` không bao giờ xuất hiện trong phản hồi hay nhật ký.
- Client: trang đăng nhập hỏi email trước; cấu hình SSO trong cài đặt workspace.

Kiểm: [Q], [M], [D] `./internal/platform/sso ./internal/handler -run 'SSO'`, [W], [V]. Ước lượng: 9 ngày.

**P37. PAT giới hạn** [D] (spec 6.5)

- Mới: migration 698 (hai cột). Đổi `server/internal/auth/pat_cache.go` (cấu trúc và tiền tố khóa mới), `Auth`, `DaemonAuth`, `platform.CheckWorkspaceAccess`, `CreatePersonalAccessToken`.
- Test đỏ trước: AC18; PAT cũ giữ toàn quyền; mục cache theo tiền tố cũ bị bỏ qua.
- Client: hai tùy chọn khi tạo token.

Kiểm: [Q], [M], [D] `./internal/auth ./internal/middleware ./internal/handler -run 'PAT'`, [W], [V]. Ước lượng: 3 ngày.

**P38. Vai trò chỉ xem** [D] (spec 6.6)

- Mới: migration 699 (ba ràng buộc vai trò). Đổi `normalizeMemberRole`, lời mời, link chia sẻ, `platform.CheckWorkspaceAccess`, `requireDaemonWorkspaceAccess`; `packages/core/types/workspace.ts`, `rules.ts`, schema zod với fallback về `viewer`; giao diện thành viên.
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
| K9 cổng hạn mức | P19, P20 | AC10, AC11 |
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
| R2 | 68,5 |
| R3 | 39,5 |
| R4 | chưa ước lượng; phụ thuộc đặc tả riêng |

Các bước ghi "độc lập" (P26, P27, P28) có thể làm song song với chuỗi chính của pha B. P17 có thể làm song song với P18 đến P21 sau khi P16 xong.
