# Kế hoạch triển khai bổ sung: admin staff và vai trò system_admin (R6)

Đặc tả: [platform-management-spec-admin.md](platform-management-spec-admin.md) (gọi là "spec R6"; mục dẫn bằng số mục trần). Kế hoạch gốc: [platform-management-plan.md](platform-management-plan.md) ("kế hoạch gốc"). Tài liệu này là các slice có thứ tự, kiểm được, cho bốn quyết định đã chốt: thêm người vào workspace có sẵn, tenant nhiều workspace, xóa ngay workspace hoặc tenant, xóa ngay tài khoản. Không sửa kế hoạch gốc.

Ngày viết 2026-11-19, bản cập nhật lần hai (vai trò `system_admin`, `admin` theo tenant) và lần ba (nhóm quản trị QT1 đến QT19, slice S16 đến S38) cùng ngày; đọc mã ở **HEAD `7c9684daa`**. **Trạng thái (cập nhật 2026-11-20): slice S0 đến S37 đã có mã trên `main`; S38 là cổng G-R6b và là phần còn lại.** Đường dẫn ghi "mới" trong các slice đã làm đã tồn tại; "đổi" đã tồn tại ở HEAD này.

## 0. Cách đọc và quy ước

- Slice đánh số `S0` đến `S38`; **S0 là slice đầu**: vai trò staff, phạm vi tenant và migration vai trò cũ. S16 đến S25 là nhóm quản trị QT1 đến QT5; S26 đến S37 là QT6, QT7, QT10, QT11, QT18, QT19 (spec mục 3c); S38 là cổng cuối. Slice đánh dấu **[Frontend, chuyển cho agy]** chỉ gồm tệp `.tsx`, hook, schema và chuỗi giao diện, và chỉ bắt đầu khi slice backend mà nó phụ thuộc đã có phản hồi mẫu ở `platform-management/internal/console/testdata/` (hợp đồng JSON dùng chung cho test Go và test console); agy không đổi tệp Go hay SQL. Mỗi slice là một hoặc vài commit nguyên tử theo conventional commits (`feat(platform): …`, `feat(server): …`, `test(platform): …`). PR theo mẫu của repo; PR có giao diện đính kèm ảnh chụp hoặc video (AGENTS.md "Change and Delivery Rules").
- **Test viết trước.** Mỗi slice ghi "Test đỏ trước": test nào phải được viết và chạy ra **đỏ** trước khi sửa mã, rồi xanh sau. Tầng test theo AGENTS.md "Testing": test Go chạm DB dùng `server/internal/testutil` (Multica) hoặc `platform-management/internal/testdb` (platform), khẳng định trên dòng fixture của chính test, không trên tổng bảng; test view và console không mock `next/*` hay `react-router-dom`; mock API ở `@multica/core/api`; tệp `.test.ts` không cần DOM bắt đầu bằng `// @vitest-environment node`.
- Mọi quy ước và điều cấm của kế hoạch gốc mục 0.1 và 0.2 vẫn áp dụng, trừ chỗ mục 0.1 dưới đây nói khác. Không bước nào thêm khóa ngoại, cascade, đường dự phòng hay công tắc thương mại thứ hai.
- Mọi quyết định D60 đến D106 đã chốt ở spec R6 mục 1.4; không slice nào chờ người trả lời.
- Việc nền của Multica là một `scheduler.JobSpec` đăng ký ở `server/cmd/server/main.go`, theo khuôn `accountprovision.Job` (`server/internal/accountprovision/job.go:83`). Test của việc nền gọi thẳng hàm `RunOnce` hai lần liên tiếp và hai bản song song để chứng minh lũy đẳng.

### 0.1 Lệnh chuẩn và cơ sở dữ liệu test

**Cơ sở dữ liệu test là `multica_platform_test` và không có cơ sở nào khác.** DSN đọc từ `.env` **bên trong chính lệnh**, không in ra, không ghi vào tệp. Không bao giờ dùng cơ sở `multica`.

**[PRE]** đứng đầu mọi lệnh cần DB. Nó dựng `TESTDSN`, từ chối nếu DSN không trỏ đúng `multica_platform_test`, rồi chạy guard `scripts/require-test-database.sh` (guard chỉ in tên cơ sở, không in DSN):

```bash
cd /home/av-server/Documents/Github/multica
TESTDSN="$(grep '^DATABASE_URL=' .env | cut -d= -f2- | sed -E 's#/multica(\?|$)#/multica_platform_test\1#')"
case "$TESTDSN" in */multica_platform_test|*/multica_platform_test\?*) ;; *) echo "DSN không trỏ tới multica_platform_test" >&2; exit 1 ;; esac
DATABASE_URL="$TESTDSN" bash scripts/require-test-database.sh
```

| Mã | Lệnh (sau [PRE], cùng một shell) | Ghi chú |
| --- | --- | --- |
| **[B]** | `pnpm catalog:build` | Sinh mã từ `catalog/` (không commit). Chạy một lần mỗi checkout trước `go test`, `tsc`, `vitest` chạy trực tiếp. Không slice nào sửa `catalog/`, nên không cần `make catalog` hay `pnpm catalog:check` |
| **[Q]** | `make sqlc` | Sau mọi sửa `server/pkg/db/queries/*.sql`; commit cả `server/pkg/db/generated/*` |
| **[PQ]** | `( cd platform-management && sqlc generate )` | Sau mọi sửa `platform-management/queries/*.sql`; commit mã sinh ở `platform-management/internal/db/`. Lược đồ đọc từ `../server/migrations/` |
| **[GN]** | `( cd server && go test -count=1 <gói> -run '<mẫu>' )` | Không cần DB. Luôn kèm `-run` khi gói có test DB |
| **[D]** | `( cd server && DATABASE_URL="$TESTDSN" go test -count=1 -p 1 <gói> -run '<mẫu>' )` | Test DB của `server/` |
| **[M]** | `( cd server && DATABASE_URL="$TESTDSN" go run ./cmd/migrate up )` | Sau mỗi slice thêm migration, chỉ trên `multica_platform_test` |
| **[PG]** | `( cd platform-management && go build ./... && go vet ./... && go test -count=1 <gói> -run '<mẫu>' )` | Không cần DB |
| **[PD]** | `( cd platform-management && PLATFORM_TEST_DATABASE_URL="$TESTDSN" DATABASE_URL="$TESTDSN" go test -count=1 -p 1 <gói> -run '<mẫu>' )` | Test DB của module platform; `testdb.Open` (`platform-management/internal/testdb/testdb.go:37`) bỏ qua test nếu hai biến không bằng nhau hoặc tên không kết thúc `_test` |
| **[V]** | `( cd server && go build ./... && go vet ./internal/handler/... ./internal/planlimit/... ./cmd/... )` | Sau mỗi slice Go ở `server/` |
| **[W]** | `pnpm --filter @multica/core exec vitest run <mẫu>`; `pnpm --filter @multica/views exec vitest run <mẫu>` | Chạy [B] trước |
| **[PW]** | `pnpm --filter @multica/platform-console exec vitest run <mẫu>`; `pnpm --filter @multica/platform-console typecheck` | Test và kiểm kiểu của console |
| **[PB]** | `pnpm --filter @multica/platform-console build` | Dựng bản tĩnh của console |
| **[T]** | `pnpm typecheck`, `pnpm lint`, `pnpm test` | Gốc repo, không gồm mobile |
| **[PI]** | `docker build -f platform-management/Dockerfile -t multica-platform .` | Chỉ build ảnh, không chạy container |
| **[GA]** | `( cd server && DATABASE_URL="$TESTDSN" bash ../scripts/go-test-with-agent-cli-guard.sh -- go test -count=1 -p 1 $(go list ./... \| grep -v '/pkg/agent') )` rồi `( cd platform-management && PLATFORM_TEST_DATABASE_URL="$TESTDSN" DATABASE_URL="$TESTDSN" go test -count=1 -p 1 ./... )` | Toàn bộ test Go, chỉ ở S15. Dùng bao bọc guard để test mặc định không gọi CLI agent của người dùng (AGENTS.md "Testing"). `pkg/agent` chạy riêng theo `scripts/test-go.sh` nếu slice chạm gói đó; R6 không chạm |
| **[E]** | `pnpm exec playwright test` | **Không chạy** trừ khi chủ dự án cho phép |

**Vì sao không dùng `make test`, `make check`, `make migrate-up`.** Makefile `test` (`Makefile:390`) chuyển sang cơ sở `multica_test` và **tạo** nó nếu chưa có (`scripts/test-go.sh:19`), nghĩa là dùng một cơ sở khác `multica_platform_test` và tự tạo cơ sở dữ liệu, trái điều cấm của kế hoạch gốc 0.2 điều 1 và trái cơ sở test của đợt này. Mọi lệnh kiểm chứng ở đây là lệnh `go test` tường minh với `TESTDSN`; không slice nào chạy `make test`, `make check`, `make up`, `make down`, `make destroy`, và không slice nào tạo cơ sở dữ liệu.

**Điều cần kiểm trước mỗi slice có migration.** Số chưa bị chiếm:

```bash
ls server/migrations | sed 's/_.*//' | sort -n | uniq | awk '($1==938) || ($1>=941 && $1<=946) || ($1>=1016 && $1<=1039)'
```

Lệnh này in các số R6 đã có trên đĩa (938, 941 đến 946, 1016 đến 1029, 1031 đến 1039; 947 và 1030 không có). Trước khi thêm migration mới, số mới không được nằm trong danh sách này. Nếu một số đã bị chiếm, đổi số trong slice và trong spec R6 mục 8 trong cùng commit. Sau khi thêm migration: [GN] `./internal/migrations -run 'TestMigrationFilesHaveMatchingDirections|TestMigrationNumericPrefixesAreUnique'` (trong `server/`) và [GN] `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'`. Ghi chú về thứ tự áp dụng số bốn chữ số: spec R6 mục 5.4.

**Quy tắc schemacheck.** Slice nào thêm migration cho bảng mà platform đọc hay ghi thì thêm phiên bản đó vào `platform-management/internal/schemacheck/schemacheck.go` trong cùng commit (kế hoạch gốc 0.2 điều 13); test `TestEveryPlatformTableMigrationIsRequired` (`platform-management/internal/schemacheck/schemacheck_test.go:100`) đỏ nếu thiếu.

**Quy tắc điểm móc.** Trước mỗi slice sửa tệp lõi của Multica, chạy `git status --short -- <tệp>` và `git log -3 --format='%h %s' -- <tệp>` (kế hoạch gốc 0.2 điều 11). Nếu có sửa chưa commit của công việc khác thì chờ, không hoàn tác thay đổi của người khác. Lúc viết, cây làm việc không có thay đổi chưa commit nào ngoài hai tệp tài liệu R6.

### 0.2 Đồ thị phụ thuộc

Không có phụ thuộc vòng: mỗi slice chỉ phụ thuộc vào slice có số nhỏ hơn. S0 đứng đầu vì mọi slice sau cần vai trò mới và cột `platform_staff.tenant_id` để kiểm phân quyền; S1 phụ thuộc S0 vì test xóa workspace của S1 khẳng định việc xóa tenant không sửa dòng staff (cột `tenant_id` do S0 tạo).

| Slice | Phụ thuộc | Nhóm yêu cầu |
| --- | --- | --- |
| S0 | không | vai trò staff, phạm vi tenant, quản lý staff, migration vai trò cũ |
| S1 | S0 | tenant (dữ liệu) |
| S2 | S1 | tenant (gói, chính sách) |
| S3 | S1, S2 | tenant (tạo workspace, cổng) |
| S4 | S1 | tenant (backfill) |
| S5 | S0, S1, S2 | tenant (console, API, phạm vi `admin`) |
| S6 | S5 | tenant (console, giao diện) |
| S7 | S0, S5 | thêm người (platform) |
| S8 | S7 | thêm người (Multica) |
| S9 | S0, S5 | xóa ngay workspace, tenant (platform) |
| S10 | S9, S3 | xóa ngay workspace, tenant (Multica) |
| S11 | S0, S9 | xóa ngay tài khoản (platform) |
| S12 | S10, S11 | xóa ngay tài khoản (Multica) |
| S13 | S0, S6, S7, S9, S11 | giao diện console cho ba nhóm và cho quản lý staff |
| S14 | S3 | tenant (client web) |
| S15 | S0 đến S14 | cổng R6 |
| S16 | S0 | QT1 gói (platform) |
| S17 | S0, S5, S9 | QT2 danh sách workspace (platform) |
| S18 | S0, S5, S7 | QT3 người dùng theo tenant (platform) |
| S19 | S8, S18 | QT3 gỡ và đổi vai trò (Multica) |
| S20 | S0, S2, S5 | QT4 tài nguyên và hạn mức bổ sung (platform) |
| S21 | S0, S5, S9 | QT5 runtime (platform) |
| S22 | S10, S21 | QT5 chặn runtime và gỡ đăng ký (Multica) |
| S23 | S6, S13, S16, S17 | giao diện console QT1, QT2 (frontend, agy) |
| S24 | S13, S18 | giao diện console QT3 (frontend, agy) |
| S25 | S13, S20, S21 | giao diện console QT4, QT5 (frontend, agy) |
| S26 | S9, S11 | QT11 giữ hồ sơ pháp lý (platform) |
| S27 | S10, S12, S26 | QT11 chặn mọi đường xóa (Multica) |
| S28 | S9, S11, S26 | QT10 duyệt chéo (platform) |
| S29 | S0, S5 | QT6 chính sách tenant (platform) |
| S30 | S1, S29 | QT6 hợp chính sách (Multica) |
| S31 | S0, S5, S20 | QT7 cảnh báo và yêu cầu hạn mức (platform) |
| S32 | S31 | QT7 route yêu cầu cho owner (Multica) |
| S33 | S0, S5 | QT18 xuất kiểm toán (platform) |
| S34 | S5, S9, S11 | QT19 khóa tenant, báo owner, xuất trước (platform) |
| S35 | S3, S10, S27, S34 | QT19 (Multica) |
| S36 | S13, S26, S28, S29, S31, S33, S34 | giao diện console QT6, QT7, QT10, QT11, QT18, QT19 (frontend, agy) |
| S37 | S30, S32 | giao diện web QT6, QT7 (frontend, agy) |
| S38 | S16 đến S37 | cổng G-R6b |

S8 và S10 cùng sửa bước "delete platform state" của `deleteWorkspaceData` và `server/cmd/server/main.go`: làm S8 trước S10 để không xung đột tệp. S4, S6, S14 độc lập với nhau. Từ S16: S23 đến S25, S36, S37 là frontend và chạy song song với backend của nhóm khác sau khi hợp đồng JSON mẫu có; S26 đứng trước S28 và S27 vì duyệt chéo và chặn xóa đều cần kiểm giữ hồ sơ pháp lý. S7 đến S8, S9 đến S12 là hai chuỗi song song được sau S5.

## 1. Slice

### S0. Vai trò `system_admin`, phạm vi tenant của `admin`, quản lý staff, migration vai trò cũ

- **Phụ thuộc:** không.
- **Tiêu chí / quyết định:** AC91, AC92, AC98 (phần bảng quyền), AC99, AC100 (thân và vai trò; kiểm tenant tồn tại ở S5), AC101, AC102, AC103 (phần ghi `tenant_id`; đọc theo phạm vi ở S5), AC90 (phần schemacheck); D79, D81, D82, D83, D84.
- **File đổi:**
  - Mới: `server/migrations/942_platform_staff_tenant`, `943_platform_staff_tenant_idx`, `944_platform_staff_system_admin`, `945_audit_event_tenant`, `946_audit_event_tenant_created_idx` (mỗi tên có `.up.sql` và `.down.sql`; `down` của 944 thu hồi `admin` có tenant rồi đặt `system_admin` về `admin`, spec R6 mục 3.8); `platform-management/internal/staff/access.go` và `access_test.go` (kiểu `Access` với `None`, `Own`, `All`; hàm `Allowed(role, action)`; bảng quyền viết theo 3.6); `platform-management/internal/console/staff_manage.go`, `staff_manage_test.go`, `staff_manage_db_test.go` (`PUT /staff/{user_id}`, kiểm vai trò và tenant); `server/internal/migrations/staff_role_migration_test.go`; `server/internal/auditstore/tenant_test.go`.
  - Đổi: `platform-management/internal/staff/role.go` (thêm `RoleSystemAdmin`; bỏ `rank` và `routeRole` cộng dồn ở `:16` và `:19`, `Permits` đọc `Allowed`; `ParseRole` nhận bốn vai trò), `role_test.go`, `seed.go` (gieo `system_admin`, `:69` và `:82`), `seed_test.go`, `store.go`; `platform-management/queries/platform_staff.sql` (cột `tenant_id`; `InsertStaff`, `ReactivateStaff` nhận tenant; truy vấn đổi vai trò và đếm `system_admin` còn hoạt động; rồi [PQ]); `platform-management/internal/platformauth/middleware.go` (`Identity`, `:18`, thêm `TenantID`) và `platformauth.go` (`Directory.ActiveStaff`, `:55`, trả thêm tenant; `/me` trả `tenant_id`); `platform-management/internal/console/handler.go` (ba danh sách vai trò ở `:111` đến `:113` sinh từ `Allowed`; `actor.TenantID`; route `PUT /staff/{user_id}` cạnh `:138`), `write.go` (`addStaff` `:386` nhận `tenant_id`; `removeStaff` `:457` đếm `system_admin`, mã `last_system_admin`), `read.go` (`listStaff` `:314` thêm tenant), `validate.go` (`parseStaffRole`, thông điệp bốn vai trò), `orders.go` (`writeOffUsage`, `:323`: chỉ `system_admin`; mã lỗi `usage_cancel_admin_only` giữ nguyên để client không đổi); `platform-management/internal/subscription/console.go` (`:81`), `platform-management/internal/pricechange/handler.go` (`:65`), `platform-management/internal/billing/reconcile.go` (`allowed`, `:135`): thay `RoleAdmin` bằng `RoleSystemAdmin`; `platform-management/audit/audit.go` (`Entry`, `:328`, thêm `TenantID`; hằng `staff.role_changed`, `staff.tenant_assigned`) và `catalog_test.go`; `server/internal/auditstore/store.go` và `server/pkg/db/queries/audit_event.sql` (ghi `tenant_id`; rồi [Q]); `server/cmd/migrate/main.go` (`:142`, dòng dọn 943, 946); `platform-management/internal/schemacheck/schemacheck.go` và `schemacheck_test.go` (942 đến 946).
  - Migration: 942, 943, 944, 945, 946. S0 **không** đổi đường dẫn route (đổi tên ở S5) và **không** mở route nào cho `admin`: sau S0 và trước S5, `admin` mới nhận 404 ở mọi route trừ đăng nhập và `GET /me`, nên không có đường rò. Mọi `admin` cũ đã thành `system_admin` nên không ai mất quyền.
- **Test đỏ trước:**
  - Tầng đơn vị, [PG]: `TestParseRoleAcceptsFourRoles` (`role_test.go`); `TestPermissionMatrixMatchesSpec` (bảng viết tay từ 3.6, hàng M1 đến M29 × bốn vai trò, kỳ vọng `None`, `Own`, `All`; không sinh từ bảng trong mã); `TestSystemAdminHasEveryAction`; `TestAdminHasNoSystemOnlyAction` (M1 đến M5, M12, M14 đến M17, M19, M24, M25); `TestSupportAndBillingAccessUnchangedFromSpec52` (so với ảnh chụp hành vi hiện nay của `support` và `billing`, gồm việc `billing` không khóa được); `TestEveryRouteInEveryRouteTableHasAMatrixRow` (duyệt năm bảng route của `console`, `subscription`, `pricechange`, `billing`, `orders`; mỗi route ánh xạ tới một hàng của 3.6); `TestValidateStaffTarget` (bảng ca: `admin` thiếu tenant, `admin` có tenant, vai trò khác kèm tenant, vai trò lạ); `TestUnknownRoleInDatabaseIsNotFound`; `TestIdentityCarriesTenantID`; `TestMeReturnsRoleAndTenantID` (`platformauth_test.go`); `TestAuditEntryTenantIDReachesTheWriter`, `TestAuditCatalogHasStaffRoleActions`; `TestRoleMatrix` (`platform-management/internal/console/handler_test.go:95`) viết lại với bốn người dùng: `system_admin` qua ở mọi hàng mà `admin` cũ qua, `admin` có tenant nhận 404 phẳng ở mọi route hiện có, `support` và `billing` không đổi; `TestBillingCannotWrite` (`:120`) và `TestSupportCannotReachStaff` (`:131`) vẫn xanh.
  - DB-backed, `platform-management/internal/testdb`, [PD]: `TestAddStaffAdminRequiresTenantID` (400 `tenant_required`); `TestAddStaffOtherRolesRejectTenantID` (400 `tenant_not_allowed`); `TestStaffRoutesAreSystemAdminOnly`; `TestPutStaffChangesRoleAndTenantAndAuditsInOneTransaction`; `TestPutStaffAuditFailureRollsBack` (I7); `TestStaffHandlersRefuseNonSystemAdminWhenCalledDirectly` (gọi thẳng bốn handler staff với vai trò `admin`: 404 và không ghi gì); `TestAdminCannotRaiseOwnOrOthersRole`; `TestRoleChangeTakesEffectOnNextRequest`; `TestLastSystemAdminCannotBeDemotedOrRevokedConcurrently` (hai goroutine); `TestSeedMakesEnvEmailsSystemAdmin`, `TestSeedReactivatesRevokedEnvEmailAsSystemAdmin`; `TestStaffAuditEntriesCarryTenantID`.
  - DB-backed Go, `server/internal/testutil`, [D]: `TestStaffRoleMigrationConvertsOldAdminsOnlyOnce` (bảng có `admin` không tenant, `admin` có tenant, `support`, `billing`, `system_admin`; chạy 944 hai lần); `TestStaffRoleMigrationLeavesScopedAdminsAndOtherRoles`; `TestStaffRoleMigrationDownRevokesScopedAdminsFirst`; `TestStaffTenantColumnIsNullableWithoutForeignKey` và `TestAuditEventTenantColumnIsNullableWithoutForeignKey` (truy vấn `pg_constraint`); `TestAuditStoreWritesTenantID`; hồi quy xanh: `platform-management/audit/queries_readonly_test.go`, `TestMigrationFilesHaveMatchingDirections`, `TestMigrationNumericPrefixesAreUnique`, `TestEveryConcurrentUpBuildHasCleanup`.
- **Lệnh kiểm chứng:**
  - [PRE]; kiểm số 942 đến 946 chưa bị chiếm; viết test, chạy thấy đỏ.
  - Sau khi sửa: [M], [Q], [PQ], [PG] `./internal/staff ./internal/platformauth ./internal/console ./internal/subscription ./internal/pricechange ./internal/billing ./audit ./internal/schemacheck -run 'Role|Permission|Matrix|Staff|Identity|Me|Audit|Required|Billing|Seed'`, [PD] `./internal/staff ./internal/console ./internal/platformauth -run 'Staff|Seed|Role|LastSystemAdmin'`, [D] `./internal/migrations ./internal/auditstore -run 'StaffRole|StaffTenant|AuditEventTenant|AuditStore|TestMigration'`, [GN] `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'` (trong `server/`), [V].

### S1. Lược đồ tenant, chép dữ liệu, bước xóa tenant, bản kê khai

- **Phụ thuộc:** S0.
- **Tiêu chí / quyết định:** AC68, AC72 (phần xóa), AC73 (phần migration 941), AC90 (phần schemacheck); D64, D68, D69, D70.
- **File đổi:**
  - Mới: `server/migrations/941_plan_max_workspaces.up.sql` và `.down.sql`; `1016_tenant`, `1017_tenant_id_uidx`, `1018_tenant_workspace`, `1019_tenant_workspace_ws_uidx`, `1020_tenant_workspace_tenant_idx`, `1021_tenant_backfill` (mỗi tên có `.up.sql` và `.down.sql`); `server/pkg/db/queries/tenant.sql` (rồi [Q]); `server/internal/tenant/tenant.go` và `tenant_test.go` (hằng `Gate = "max_workspaces"`, `Lock`, `Detach`); `server/internal/migrations/tenant_migration_test.go`; `server/internal/handler/workspace_delete_tenant_test.go`.
  - Đổi: `server/cmd/migrate/main.go` (`concurrentIndexCleanups`, `:142`, thêm `1017_tenant_id_uidx`, `1019_tenant_workspace_ws_uidx`, `1020_tenant_workspace_tenant_idx`); `server/internal/handler/workspace.go` (`deleteWorkspaceData`, `:1567`: thêm bước "detach tenant" ngay trước bước "delete workspace" quanh `:2037`); `server/internal/handler/workspace_delete_manifest_test.go` (phân loại `tenant`, `tenant_workspace`); `platform-management/internal/schemacheck/schemacheck.go` (thêm `941_plan_max_workspaces`, `1016` đến `1021`).
  - Migration: 941, 1016 đến 1021. Migration 1016 gồm luôn các cột `status` (mặc định `active`), `status_reason`, `status_changed_at`, `status_changed_by` của `tenant` (spec R6 mục 5.2, QT19), để slice S34 không cần migration.
- **Test đỏ trước:**
  - Tầng schema và migration, [GN]: `TestMigrationFilesHaveMatchingDirections`, `TestMigrationNumericPrefixesAreUnique` (`server/internal/migrations/migrations_lint_test.go:61`, `:93`) và `TestEveryConcurrentUpBuildHasCleanup` (`server/cmd/migrate/migrate_mul5999_index_retry_test.go:161`) đỏ khi tệp mới thiếu tệp `down` hoặc dòng dọn. Test tĩnh `TestFourDigitMigrationsOnlyTouchTablesCreatedEarlierInSortOrder` nằm ở `server/internal/migrations/migrations_order_test.go` (mới).
  - DB-backed Go, `server/internal/testutil`: `TestTenantBackfillIsIdempotentAndKeysTenantByWorkspace` (tạo ba workspace fixture, xóa dòng liên kết, chạy nội dung `1021_tenant_backfill.up.sql` hai lần liên tiếp rồi hai lần song song; mỗi workspace đúng một `tenant` có `id` bằng id workspace và đúng một `tenant_workspace`; dòng workspace đã có liên kết không đổi); `TestTenantTablesHaveNoForeignKeysOrInlineConstraints` (truy vấn `pg_constraint`: không có loại `f`, `p`, `u` cho hai bảng); `TestWorkspaceTableHasNoTenantColumn`; `TestFourDigitMigrationsOnlyTouchTablesCreatedEarlierInSortOrder` (tĩnh, không cần DB: đọc mọi tệp migration có tiền tố bốn chữ số, trích tên bảng sau `ALTER TABLE`, `INSERT INTO`, `FROM`, `UPDATE` và đòi bảng đó được tạo bởi một tệp đứng trước nó theo thứ tự chuỗi; đỏ nếu 1021 tham chiếu `workspace_subscription`); `TestPlanMaxWorkspacesMigrationIsIdempotentAndKeepsExistingKey` (hàng `free` chưa có khóa nhận 1, hàng `pro` đã có khóa 7 giữ 7, `enterprise` nhận JSON null tường minh; chạy hai lần không đổi; không tham chiếu cột `version`).
  - DB-backed Go, `server/internal/handler`: `TestDeleteWorkspaceDetachesTenant` (hai workspace một tenant: xóa một thì tenant và một liên kết còn; xóa nốt thì cả tenant lẫn liên kết mất); `TestDeleteWorkspaceTenantDetachWaitsForTenantLock` (giữ `LockPlanGate` của tenant ở giao dịch khác, xóa phải chờ); `TestWorkspaceDeletionManifestCoversPublicSchema` (`workspace_delete_manifest_test.go:257`) và `TestWorkspaceTeardownAppliesManifestDecisions` (`:343`) đỏ cho tới khi hai bảng mới được phân loại.
  - Platform, [PG]: `TestRequiredVersionsAreSortedUniqueAndExist` (`schemacheck_test.go:21`), `TestEveryPlatformTableMigrationIsRequired` (`:100`).
  - Phân quyền theo tenant, DB-backed Go, [D]: `TestDeleteWorkspaceLeavesPlatformStaffRowsUntouched` (một `admin` có `tenant_id` của tenant; xóa workspace cuối của tenant xóa dòng `tenant` nhưng không sửa dòng `platform_staff`, đúng spec R6 mục 3.5 quy tắc 5; AC104).
- **Lệnh kiểm chứng:**
  - [PRE]; kiểm số migration chưa bị chiếm; viết test, chạy thấy đỏ.
  - Sau khi sửa: [M], [Q], [D] `./internal/migrations ./internal/tenant ./internal/handler -run 'Tenant|Manifest|PlanMaxWorkspaces|DeleteWorkspaceDetaches'`, [GN] `./internal/migrations ./cmd/migrate -run 'TestMigration|TestFourDigit|TestEveryConcurrentUpBuildHasCleanup'`, [PG] `./internal/schemacheck`, [V].

### S2. `max_workspaces` ở gói và chính sách quyền lợi

- **Phụ thuộc:** S1.
- **Tiêu chí / quyết định:** AC71 (phần platform), AC73, AC89 (phần hợp đồng); D65, D66, D70.
- **File đổi:**
  - Mới: `platform-management/queries/tenant.sql` (đọc workspace của tenant, thuê bao, gói; rồi [PQ]); `platform-management/internal/plan/max_workspaces_test.go`; `platform-management/internal/entitlement/max_workspaces_test.go`; `platform-management/internal/entitlement/max_workspaces_db_test.go`; `server/internal/entitlement/max_workspaces_test.go`.
  - Đổi: `platform-management/internal/plan/plan.go` (hằng `GateMaxWorkspaces`, thêm vào `LimitGates` `:81` và `CountedGates`, kiểm giá trị không nhỏ hơn 1); `platform-management/internal/plan/catalogue.json` (sáu gói theo spec R6 mục 5.6); `platform-management/internal/entitlement/policy.go` (`planLimitGates`, `:134`; tính giá trị lớn nhất theo tenant), `loader.go`, `service.go`, `pgx.go` (nạp thuê bao, gói, ghi đè của các workspace anh em); `server/internal/entitlement/types.go` (`GateMaxWorkspaces`, `optionalGates` `:45`, `valid` `:52`), `server/internal/entitlement/client.go` (`normalizePolicy`).
  - Migration: không.
- **Test đỏ trước:**
  - Platform, [PG]: `TestCatalogueMaxWorkspacesDefaults` (free 1, pro 3, business 10, payg 5, enterprise và unlimited là null tường minh); `TestValidateRejectsMaxWorkspacesBelowOne` (0, âm bị từ chối, rỗng hợp lệ); `TestUnlimitedPlanStillLeavesEveryCountedGateEmpty` (khóa mới nằm trong `CountedGates`); `TestTenantMaxWorkspacesIsLargestEffectiveLimit` (bảng ca: mọi giá trị hữu hạn lấy lớn nhất; một giá trị rỗng thì không giới hạn; `limit_overrides` nâng hoặc nhả giới hạn; workspace không có thuê bao tính theo `free`; workspace không có dòng liên kết nhận cổng `off`).
  - Platform DB-backed, [PD], `platform-management/internal/testdb`: `TestPolicyCarriesTenantGateForEverySiblingWorkspace` (hai workspace một tenant gói khác nhau; cả hai nhận cùng `max_workspaces`; đổi gói một workspace đổi giá trị của cả hai sau khi xóa cache).
  - Hợp đồng ở Multica, [GN]: `TestNormalizePolicyAcceptsMaxWorkspacesGate`, `TestAbsentMaxWorkspacesGateIsOff` (chính sách của Multica Cloud hay platform cũ vẫn hợp lệ), `TestUnknownGateIsStillRejected`.
- **Lệnh kiểm chứng:** [PRE]; [PQ]; [PG] `./internal/plan ./internal/entitlement -run 'MaxWorkspaces|Catalogue|Unlimited|Validate'`; [PD] `./internal/entitlement -run 'TenantGate|Policy'`; [GN] `./internal/entitlement -run 'MaxWorkspaces|NormalizePolicy'` (trong `server/`); [V].

### S3. Tạo workspace vào tenant và cổng `max_workspaces`

- **Phụ thuộc:** S1, S2.
- **Tiêu chí / quyết định:** AC69, AC70, AC71 (phần Multica), AC72 (phần tạo), AC88, AC89 (phần phản hồi); D66, D67, D68.
- **File đổi:**
  - Mới: `server/internal/handler/workspace_tenant_test.go`; `server/internal/planlimit/tenant.go` (`CountTenantWorkspaces` và hằng phạm vi khóa) và `tenant_test.go`.
  - Đổi: `server/internal/workspaceprovision/provision.go` (`Params.TenantID`; `Create` `:40` tạo `tenant` và `tenant_workspace` hoặc chỉ liên kết) và `provision_test.go`; `server/internal/handler/workspace.go` (`CreateWorkspaceRequest` nhận `tenant_id`; `CreateWorkspace` `:328` đọc chính sách trước giao dịch, lấy khóa tenant, kiểm quyền, `Admit`, tạo; `workspaceToResponse` thêm `tenant_id`); `server/pkg/db/queries/tenant.sql` (do S1 tạo; thêm: tenant của người gọi, đếm, liệt kê workspace do người gọi sở hữu; rồi [Q]); `server/internal/accountprovision/job_test.go`.
  - Migration: không.
- **Test đỏ trước:**
  - DB-backed Go, `server/internal/testutil`, `server/internal/workspaceprovision`: `TestCreateWithoutTenantMakesTenantWithWorkspaceID`, `TestCreateWithTenantAddsLinkOnly`.
  - DB-backed Go, `server/internal/handler` (dùng nhà cung cấp quyền lợi giả như các test cổng hiện có): `TestCreateWorkspaceWithoutTenantIDKeepsResponseAndAddsTenantID`; `TestCreateWorkspaceIntoTenantAsOwner`; `TestCreateWorkspaceIntoForeignTenantIs404` (mã `tenant_not_found`); `TestCreateWorkspaceIntoTenantRefusedAtMaxWorkspaces` (402, `gate`, `limit`, `used`); `TestConcurrentCreateAtLastTenantSlotAdmitsExactlyOne` (hai goroutine); `TestCreateWorkspaceIntoTenantUnlimitedWhenGateOff`; `TestCreateWorkspaceTakesTenantLockWithTenantScope` (quan sát `pg_locks`); `TestDowngradeNeverDeletesWorkspaces`.
  - Gói `planlimit`, [GN]: `TestAdmitScopesLockByTenantID` (dùng `fakeStore`, `planlimit_test.go:31`).
  - Việc nền: `TestProvisionedWorkspaceHasTenant` (`server/internal/accountprovision/job_test.go`).
  - Hồi quy phải xanh trước và sau: `TestCreateWorkspaceUsesRequestedSlug`, `TestCreateWorkspaceSlugConflictReturnsConflict`, `TestCreateWorkspaceInvalidSlugReturnsBadRequest` (`server/internal/handler/handler_test.go:2117`, `:2138`, `:2159`) và toàn bộ test `planlimit` cho `FREE_PLAN_OWNED_WORKSPACE_LIMIT`.
  - Phân quyền theo tenant, [D]: `TestNewWorkspaceInTenantIsInAdminScopeWithoutStaffGrant` (sau `CreateWorkspace` có `tenant_id`, dòng `tenant_workspace` của workspace mới trỏ tới tenant, và số dòng `platform_staff` không đổi: phạm vi của `admin` suy ra từ liên kết, không có quyền cấp theo workspace; AC93); `TestCreateWorkspaceDoesNotGrantStaffAnyTenant` (không dòng `platform_staff` nào đổi `tenant_id`).
- **Lệnh kiểm chứng:** [PRE]; [Q]; [D] `./internal/workspaceprovision ./internal/planlimit ./internal/handler ./internal/accountprovision -run 'Tenant|CreateWorkspace|Provision|Admit'`; [V].

### S4. Lệnh `platform-backfill tenants`

- **Phụ thuộc:** S1.
- **Tiêu chí / quyết định:** AC68 (phần backfill); D69.
- **File đổi:**
  - Mới: `platform-management/internal/backfill/tenants.go`, `tenants_test.go`, `tenants_db_test.go`; `platform-management/queries/tenant_backfill.sql` (rồi [PQ]).
  - Đổi: `platform-management/cmd/platform-backfill/main.go` (lệnh con `tenants`) và `main_test.go`; `platform-management/internal/backfill/pgx.go`.
  - Migration: không.
- **Test đỏ trước:**
  - Platform DB-backed, [PD]: `TestBackfillTenantsCreatesOneTenantPerOrphanWorkspace` (workspace chưa có liên kết nhận tenant `id` bằng id workspace; workspace đã có liên kết không đổi); `TestBackfillTenantsIsIdempotentAndParallelSafe` (hai lần liên tiếp và hai tiến trình song song cho đúng một dòng mỗi workspace); `TestBackfillTenantsBatchesAreIndependent` (một lô hỏng không làm mất lô khác).
  - [PG]: `TestPlatformBackfillTenantsSubcommandParsesArguments` (lệnh con và cờ).
- **Lệnh kiểm chứng:** [PRE]; [PQ]; [PG] `./cmd/platform-backfill ./internal/backfill -run 'Tenants'`; [PD] `./internal/backfill -run 'Tenants'`.

### S5. Console: `/tenants` theo tenant, đổi tên route cấp workspace, phạm vi tenant của `admin` (phía API)

- **Phụ thuộc:** S0, S1, S2.
- **Tiêu chí / quyết định:** AC74, AC75, AC90 (phần cô lập nội dung), AC92, AC93, AC100 (kiểm tenant tồn tại), AC103, AC104, AC105 (phần xem), AC106, AC107; D77, D80, D83, D84.
- **File đổi:**
  - Mới: `platform-management/internal/console/tenants.go`, `tenants_db_test.go`; `platform-management/internal/console/scope.go`, `scope_test.go`, `scope_db_test.go` (phân giải phạm vi từ `platform_staff.tenant_id` và `tenant_workspace`; tham số phạm vi cho truy vấn).
  - Đổi: `platform-management/internal/console/handler.go` (`routeTable`, `:117`: `/tenants` và `/tenants/{id}` theo tenant; `/workspaces/{id}`, `/workspaces/{id}/status`, `/usage`, `/audit-events`, `/notes`, `/subscription`; xóa đường cũ `/tenants/{id}/…`); `read.go` (`listTenants` `:96`, `getTenant` `:121`, thêm `getWorkspace`), `write.go` (`setTenantStatus` `:31` đổi tên thành `setWorkspaceStatus`), `store.go`, `pgx.go`; `platform-management/queries/console.sql` và `tenant.sql` (rồi [PQ]; thêm biến thể truy vấn nhận tham số phạm vi, và truy vấn kiểm toán lọc theo `tenant_id` hoặc workspace của tenant); `platform-management/internal/subscription/console.go` (đổi `/tenants/{id}/subscription` và `/tenants/{id}/billing-profile*` thành `/workspaces/{id}/…`; `:83` đến `:105`); `platform-management/internal/console/write.go` (`addStaff`) và `staff_manage.go` (`updateStaff`): kiểm tenant tồn tại, mã `tenant_invalid`; `overview` thêm số workspace thiếu tenant; các test `handler_test.go`, `db_test.go`, `db_group_count_test.go`, `isolation_test.go`; `e2e/platform/api-refusals.spec.ts`, `e2e/platform/console-requests.spec.ts`, `e2e/platform/ui/screens.spec.ts` (đổi đường dẫn; chỉ chạy khi được phép [E]).
  - Migration: không.
- **Test đỏ trước:**
  - Platform, [PG]: `TestRoleMatrix` (`platform-management/internal/console/handler_test.go:95`) viết lại cho bảng route mới (vai trò giữ nguyên); `TestOldTenantSubroutesAreNotFound` (`/tenants/{id}/status`, `/tenants/{id}/notes`, `/tenants/{id}/subscription` trả 404 với mọi vai trò); `TestTenantListJSONHasWorkspaceCountAndMaxWorkspaces`.
  - Platform DB-backed, [PD]: `TestListTenantsSearchesTenantNameWorkspaceSlugAndOwnerEmail`; `TestGetTenantListsWorkspacesWithEffectiveMaxWorkspaces`; `TestTenantStatusSummaryCountsWorkspaceStatuses`; `TestLockingOneWorkspaceLeavesSiblingsUntouched`; `TestOverviewCountsWorkspacesWithoutTenant`.
  - Cô lập nội dung, [PG]: `TestConsoleFilesAreScannedAndNameNoContentTable` (`isolation_test.go:149`) phải xanh với tệp mới.
  - Phạm vi tenant của `admin`, [PD], `testdb` (bảng ca: `system_admin`, `admin` trong tenant, `admin` ngoài tenant, `admin` có tenant đã xóa, `support`, `billing`): `TestAdminSeesOnlyOwnTenantInList`; `TestAdminGetsSameNotFoundForForeignAndMissingObjects` (so từng byte thân 404 của id ngoài tenant với id không tồn tại, cho tenant, workspace, ghi chú, đơn, người dùng, yêu cầu); `TestAdminScopeFollowsTenantWorkspaceLinks` (thêm workspace vào tenant thì vào phạm vi, không cần cấp quyền); `TestAdminOfDeletedTenantHasEmptyScope` (AC104); `TestAdminLockUnlockAndScheduleDeletionInOwnTenantOnly` (AC106; `support` khóa và mở được, không lên lịch; `billing` không làm gì trong ba việc); `TestAdminOrdersAreReadOnlyAndScoped` và `TestAdminHasNoAccessToPaymentRoutesAccountsOverviewAndUserActions` (AC107, AC92); `TestUsersListScopedToTenantMembersForAdmin`; `TestAuditListScopedForAdminAndFullForOthers` (AC103; dòng xóa workspace còn hiện nhờ `tenant_id`); `TestStaffTenantMustExist` (AC100: `POST` và `PUT /staff` trả 400 `tenant_invalid`); `TestHandlersApplyScopeWhenCalledDirectlyWithoutMiddleware`.
  - Hồi quy xanh: `TestSupportBillingScopeUnchanged` (kết quả của `support` và `billing` giống trước S5 ở mọi route đã đổi tên).
- **Lệnh kiểm chứng:** [PRE]; [PQ]; [PG] `./internal/console -run 'RoleMatrix|OldTenant|TenantList|Isolation|ConsoleFiles'`; [PD] `./internal/console -run 'Tenant|Overview|Workspace'`.

### S6. Console: giao diện tenant và workspace

- **Phụ thuộc:** S5.
- **Tiêu chí / quyết định:** AC74, AC75; D77.
- **File đổi:**
  - Mới: `platform-management/console/src/screens/WorkspaceDetail.tsx` và `WorkspaceDetail.test.tsx` (nội dung chi tiết cũ của `TenantDetail.tsx`).
  - Đổi: `platform-management/console/src/screens/Tenants.tsx`, `Tenants.test.tsx` (một dòng một tenant, cột số workspace và `max_workspaces`, "không giới hạn" khi rỗng); `TenantDetail.tsx`, `TenantDetail.test.tsx` (tenant và danh sách workspace dẫn tới `WorkspaceDetail`); `platform-management/console/src/api/schemas.ts`, `schemas.test.ts`, `client.ts`, `client.test.ts`, `hooks.ts`, `hooks.test.tsx`; `App.tsx`, `App.test.tsx`.
  - Migration: không.
- **Test đỏ trước (tầng view của console, [PW], không mock `next/*`):** `Tenants.test.tsx` `renders one row per tenant with workspace count and max_workspaces`, `shows unlimited when max_workspaces is null`; `TenantDetail.test.tsx` `lists workspaces and links to the workspace detail`; `WorkspaceDetail.test.tsx` `status change posts to /workspaces/{id}/status`, `subscription form puts to /workspaces/{id}/subscription`; `schemas.test.ts` `tenant schema falls back for unknown fields and null max_workspaces` (I9); `client.test.ts` đường dẫn mới.
- **Lệnh kiểm chứng:** [B]; [PW] `Tenants|TenantDetail|WorkspaceDetail|schemas|client|App`; [PB]; [T] `pnpm typecheck`, `pnpm lint`. PR đính kèm ảnh chụp.

### S7. Staff thêm người vào workspace: phía platform

- **Phụ thuộc:** S0, S5.
- **Tiêu chí / quyết định:** AC62, AC63 (phần từ chối email staff), AC64, AC66 (pha 1), AC67 (phần platform), AC94, AC105 (phần yêu cầu thêm người); D60, D61, D62, D63, D78, D80, D83.
- **File đổi:**
  - Mới: `server/migrations/1022_platform_member_request`, `1023_platform_member_request_id_uidx`, `1024_platform_member_request_live_uidx` (mỗi tên có `.up.sql`, `.down.sql`); `platform-management/queries/member_request.sql` (rồi [PQ]); `platform-management/internal/memberrequest/` (`memberrequest.go`, `service.go`, `validate.go`, `pgx.go`, và các `_test.go`); `platform-management/internal/console/members.go`, `members_test.go`, `members_db_test.go`.
  - Đổi: `platform-management/internal/console/handler.go` (bốn route của spec R6 mục 4.4); `platform-management/internal/staff/access.go` (hàng M21, M22: `All` cho `system_admin`, `Own` cho `admin`) và `access_test.go`; `platform-management/audit/audit.go` (bốn hằng `member.staff_add_*`) và `catalog_test.go`; `platform-management/internal/schemacheck/schemacheck.go` (1022 đến 1024); `server/cmd/migrate/main.go` (dòng dọn 1023, 1024).
  - Migration: 1022, 1023, 1024. Migration 1022 gồm luôn các cột `kind` (mặc định `add`), `target_user_id`, `previous_role` (QT3); chỉ mục của 1024 là `(workspace_id, kind, email)` có điều kiện `state = 'pending'`.
- **Test đỏ trước:**
  - [PG]: `TestMemberRequestValidate` (bảng ca: email sai định dạng, quá 254 ký tự, chữ hoa chuẩn hóa; vai trò `owner` và giá trị lạ trả `member_role_invalid`); `TestRoleMatrix` mở rộng (`support`, `billing`, không phiên nhận 404 phẳng ở mọi route thêm người; `system_admin` qua; `admin` qua trong tenant mình, xem S7 phần phân quyền); `TestAuditCatalogHasMemberStaffActions`.
  - [PD], `testdb`: `TestMemberRequestStaffEmailIsRefused` (409 `staff_cannot_be_added`, không có dòng yêu cầu); `TestMemberRequestWorkspaceMustBeActive` (409 `workspace_not_active` cho ba trạng thái còn lại); `TestMemberRequestAlreadyMember` (409); `TestMemberRequestResendReturnsLiveRequest` (200 cùng `id`); `TestMemberRequestConcurrentSameEmailWritesOneRow`; `TestMemberRequestAuditAndRowShareOneTransaction` (làm hỏng ghi kiểm toán thì không có dòng yêu cầu, I7); `TestMemberRequestCancelReleasesSeatAndAudits` (409 `member_request_not_cancelable` khi không còn `pending`); `TestMemberRequestRoutesPassConsoleIsolationScan`.
  - Phân quyền theo tenant, [PD]: `TestMemberRequestAdminInOwnTenantSucceedsWithStaffAuditAndTenantID` (202, hai dòng kiểm toán `actor_type = staff` có `workspace_id` và `tenant_id`; AC94); `TestMemberRequestAdminOutsideTenantIsNotFoundAndWritesNothing` (POST, GET danh sách, GET theo id, cancel; thân 404 bằng thân id không tồn tại); `TestMemberRequestSystemAdminAnyWorkspace`; `TestMemberRequestAdminListIsFilteredToOwnTenant`; `TestMemberRequestRoutesMatchMatrixRows` (M21, M22 × bốn vai trò, kèm `support` và `billing` nhận 404 phẳng; AC62).
- **Lệnh kiểm chứng:** [PRE]; [M]; [PQ]; [PG] `./internal/memberrequest ./internal/staff ./audit ./internal/console ./internal/schemacheck -run 'MemberRequest|RoleMatrix|Audit|Required'`; [PD] `./internal/memberrequest ./internal/console -run 'MemberRequest'`; [GN] `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'` (trong `server/`).

### S8. Việc nền `member_provisioning` ở Multica

- **Phụ thuộc:** S7.
- **Tiêu chí / quyết định:** AC63, AC64, AC65, AC66 (pha 2), AC67 (hồi quy); D60, D62, D63.
- **File đổi:**
  - Mới: `server/internal/memberprovision/job.go` và `job_test.go` (khuôn `accountprovision`); `server/pkg/db/queries/platform_member_request.sql` (rồi [Q]).
  - Đổi: `server/cmd/server/main.go` (đăng ký bằng `schedulerMgr.Register`, cạnh `:1055`); `server/internal/handler/workspace.go` (bước "delete platform state", `:2007`, thêm xóa `platform_member_request` theo workspace); `server/pkg/db/queries/workspace_delete.sql`; `server/internal/handler/me_deletion.go` (`DeleteAccount`, `:365`: thêm bước ẩn danh hóa email trong `platform_member_request`) và `server/pkg/db/queries/account_deletion.sql`; `platform-management/accountdeletion/registry.go` (đăng ký `platform_member_request.user_id` và `tenant.created_by` là `keep`); `server/internal/handler/workspace_delete_manifest_test.go`.
  - Migration: không.
- **Test đỏ trước (DB-backed Go, `server/internal/testutil`; client ghế là bản giả ghi lại lời gọi theo bảng của Phụ lục B, vì Multica không nhập mã platform):**
  - `TestMemberProvisioningUsesExistingUser`; `TestMemberProvisioningCreatesOnboardedUserWithoutSessionPasswordOrToken`; `TestMemberProvisioningWorksWhenSignupIsDisabled`; `TestMemberProvisioningBlockedDomainFails`; `TestMemberProvisioningSuspendedOrPendingDeletionUserFails`; `TestMemberProvisioningRolesApplied` (bảng `admin`, `member`, `viewer`).
  - Ghế: `TestMemberProvisioningSeatFullReleasesAndCreatesNoMember`; `TestMemberProvisioningConfirmsSeatAfterCommit`; `TestMemberProvisioningUnmanagedWorkspaceSkipsSeat`; `TestMemberProvisioningTransportFailureRetriesThenFailsAndReleases`; `TestMemberProvisioningTwoRequestsForLastSeatOneWins` (bản giả cài các quy tắc có trạng thái của Phụ lục B).
  - Kiểm toán và sự kiện: `TestMemberProvisioningAuditActorIsStaffWithWorkspaceID`; `TestMemberProvisioningAuditFailureRollsBackMemberAndReleasesSeat`; `TestMemberProvisioningPublishesMemberAdded`.
  - Lũy đẳng: `TestMemberProvisioningTwiceAndParallelMakesOneMember`.
  - Xóa: `TestWorkspaceDeleteRemovesMemberRequests` (bản kê khai), `TestAccountDeletionAnonymizesMemberRequests`, `TestUserColumnRegistry` (`server/internal/migrations/user_columns_registry_test.go:38`).
  - Hồi quy xanh: toàn bộ `server/internal/handler/invitation_test.go`, `seat_capacity_test.go` và `server/internal/seatcapacity/contract_test.go`.
  - Platform DB-backed, [PD], `platform-management/internal/seat`: `TestStaffAddClaimIsIdempotentByRequestToken` (`Claim` hai lần cùng `token` giữ một chỗ, `Release` rồi giải phóng, theo `platform-management/internal/seat/seat.go:248`, `:336`).
- **Lệnh kiểm chứng:** [PRE]; [Q]; [D] `./internal/memberprovision ./internal/handler ./internal/seatcapacity ./internal/migrations -run 'MemberProvisioning|Manifest|Invitation|SeatCapacity|UserColumn|AccountDeletion'`; [PD] `./internal/seat -run 'StaffAdd'`; [V].

### S9. Xóa ngay workspace và tenant: phía platform

- **Phụ thuộc:** S0, S5.
- **Tiêu chí / quyết định:** AC76, AC77, AC78 (pha 1), AC80 (pha 1), AC81 (pha 1, hủy, thử lại), AC95 (pha 1), AC96, AC105 (phần xóa); D71, D72, D73, D78, D80, D83, D85.
- **File đổi:**
  - Mới: `server/migrations/1025_platform_deletion_request`, `1026_platform_deletion_request_id_uidx`, `1027_platform_deletion_request_live_uidx` (mỗi tên có `.up.sql`, `.down.sql`); `platform-management/queries/deletion_request.sql` (rồi [PQ]); `platform-management/internal/deletionrequest/` (`deletionrequest.go`, `service.go`, `pgx.go`, `validate.go`, và các `_test.go`); `platform-management/internal/console/deletions.go`, `deletions_test.go`, `deletions_db_test.go`.
  - Đổi: `platform-management/internal/workspacestatus/workspacestatus.go` (cho phép chạy `apply` trong giao dịch của người gọi, và kéo `deletion_scheduled_at` về hiện tại khi đã `pending_deletion`) và `pgx.go`; `platform-management/queries/platform_workspace.sql`; `platform-management/internal/console/handler.go` (sáu route của spec R6 mục 6.5), `validate.go` (hàm so khớp xác nhận, GĐ10); `platform-management/internal/staff/access.go` (hàng M23 đến M28: `workspaces.delete_now` là `Own` cho `admin`; `tenants.delete_now` chỉ `system_admin`); `platform-management/audit/audit.go` (`workspace.immediate_deletion_requested`, `tenant.immediate_deletion_requested`, `tenant.deleted`, `deletion.request_failed`, `deletion.request_canceled`, `deletion.request_retried`) và `catalog_test.go`; `platform-management/internal/schemacheck/schemacheck.go` (1025 đến 1027); `server/cmd/migrate/main.go` (dòng dọn 1026, 1027).
  - Migration: 1025, 1026, 1027. Migration 1025 gồm luôn trạng thái `awaiting_approval`, các cột `approved_by`, `approved_at`, `export_first`, `notice_sent_at` và `kind` nhận cả `runtime` (QT5, QT10, QT19); chỉ mục của 1027 có điều kiện `state IN ('awaiting_approval', 'pending', 'running', 'failed')`.
- **Test đỏ trước:**
  - [PG]: `TestRoleMatrix` mở rộng (`system_admin` qua ở mọi route xóa ngay; `admin` qua ở route workspace trong tenant mình và 404 phẳng ở route tenant; `support` và `billing` nhận 404 phẳng ở các route xóa ngay, trong khi `support` vẫn khóa và mở được còn `billing` không, như gốc 5.2); `TestConfirmMatching` (bảng ca GĐ10: cắt khoảng trắng, phân biệt hoa thường với slug và tên, email không phân biệt); `TestDeleteNowReasonRequired` (rỗng, chỉ khoảng trắng, quá 500 ký tự).
  - [PD], `testdb`: `TestDeleteNowConfirmMismatchWritesNothing`; `TestDeleteNowPhaseOneWritesAuditRequestAndStatusInOneTransaction` (làm hỏng ghi kiểm toán thì không có dòng yêu cầu, không đổi trạng thái); `TestDeleteNowSetsPendingDeletionDueNowAndClearsCache`; `TestDeleteNowExpeditesAlreadyScheduledDeletion`; `TestDeleteNowTenantListsEveryWorkspaceAndAuditsEach`; `TestDeleteNowSecondRequestReturnsLiveOne`; `TestCancelRestoresActiveAndAudits`; `TestRetryResetsAttempts`; `TestNoDeleteStatementsOnWorkspaceContentInPlatformCode` (quét `platform-management/queries/*.sql` và mã Go của gói `deletionrequest`: không có `DELETE FROM` trên bảng nội dung workspace, mở rộng khuôn của `isolation_test.go`).
  - Phân quyền theo tenant, [PD]: `TestDeleteWorkspaceNowAdminInOwnTenant` (202, kiểm toán pha 1 có `tenant_id`; AC95); `TestDeleteWorkspaceNowAdminOutsideTenantIsNotFound` (không ghi dòng yêu cầu, trạng thái hay kiểm toán); `TestDeleteTenantNowIsSystemAdminOnly` (`admin` nhận 404 phẳng cả với tenant của mình, AC96; `support` và `billing` cũng 404); `TestSystemAdminDeletesAnyWorkspaceAndTenant`; `TestDeletionRequestVisibilityForAdmin` (thấy yêu cầu `workspace` và `tenant` của tenant mình, không thấy `account`, không thấy tenant khác); `TestDeletionRequestCancelAndRetryOnlyOwnForAdmin` (AC105); `TestDeleteNowRoutesMatchMatrixRows` (M23, M24, M27, M28 × bốn vai trò).
- **Lệnh kiểm chứng:** [PRE]; [M]; [PQ]; [PG] `./internal/deletionrequest ./internal/console ./internal/staff ./audit ./internal/schemacheck -run 'DeleteNow|Confirm|RoleMatrix|Audit|Required|NoDelete'`; [PD] `./internal/deletionrequest ./internal/console ./internal/workspacestatus -run 'DeleteNow|Cancel|Retry|Expedite|NoDelete'`; [GN] `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'` (trong `server/`).

### S10. Việc nền `staff_deletion` cho workspace và tenant

- **Phụ thuộc:** S9, S3. Làm sau S8 (cùng sửa `workspace.go` và `main.go`).
- **Tiêu chí / quyết định:** AC78 (pha 2), AC79, AC80, AC81; D71, D73.
- **File đổi:**
  - Mới: `server/internal/handler/deletion_origin.go` (kiểu `DeletionOrigin`: `self`, `period_ended`, `staff` kèm `RequestID`, `StaffID`, `StaffLabel`); `server/internal/staffdeletion/job.go`, `workspace.go`, `job_test.go`; `server/pkg/db/queries/platform_deletion_request.sql` (rồi [Q]).
  - Đổi: `server/internal/handler/workspace_deletion.go` (`DeleteDueWorkspace` `:170` gọi lõi `deleteDueWorkspace(ctx, id, now, origin)` dùng chung; kiểm toán `workspace.deleted` theo `origin`); `server/internal/handler/workspace.go` (`CreateWorkspace`: kiểm `tenant_deleting` dưới khóa tenant); `server/pkg/db/queries/platform_workspace.sql` (`ListDueWorkspaceDeletions`, `:35`, loại trừ workspace có yêu cầu `pending`, `running`, `failed`); `server/cmd/server/main.go` (đăng ký `staff_deletion`).
  - Migration: không.
- **Test đỏ trước (DB-backed Go, `server/internal/testutil`):**
  - `TestStaffDeletionDeletesWorkspaceThroughExistingTeardown`; `TestStaffDeletionEquivalentToDeleteWorkspace` (hai fixture giống nhau, một xóa bằng `DeleteWorkspace`, một bằng việc nền; tập dòng còn lại theo danh sách bảng của bản kê khai bằng nhau); `TestStaffDeletionFailureMarksFailedAfterThreeAttemptsAndKeepsPhaseOneAudit` (tiêm lỗi vào một bước qua hook của test; dữ liệu workspace còn nguyên); `TestStaffDeletionRetryAfterFailureCompletes`; `TestStaffDeletionTenantDeletesEveryWorkspaceThenTenant`; `TestStaffDeletionTwiceAndParallelDeletesOnce`; `TestStaffDeletionAuditActorIsStaffWithOriginDetail`; `TestStaffDeletionAuditRowSurvivesWithNullWorkspaceID`.
  - `TestWorkspaceDeletionJobSkipsWorkspacesWithLiveOrFailedStaffRequest`; `TestCreateWorkspaceIntoDeletingTenantIs409`; `TestCreateRacingPhaseOneIsEitherListedOrRefused`; `TestSubscriptionLifecycleNeverDeletesAWorkspace` (I10, chạy mọi cạnh; ở `platform-management/internal/lifecycle`, [PD]).
  - Hồi quy xanh: `server/internal/handler/workspace_deletion_test.go` (`TestDeleteWorkspaceWithZeroDaysDeletesAtOnce`, `:175`; `TestDeleteWorkspaceWithDaysSchedulesInsteadOfDeleting`, `:190`), `workspace_delete_lock_test.go`, `workspace_delete_fence_test.go`, `scheduler/jobs_workspace_deletion_test.go`.
  - Phân quyền theo tenant, [D]: `TestStaffDeletionAuditRowKeepsTenantIDAfterWorkspaceIsGone` (sau khi workspace bị xóa, dòng kiểm toán pha 1 và dòng `workspace.deleted` có `workspace_id` rỗng nhưng `tenant_id` còn nguyên, để `admin` của tenant đọc được; AC95, AC103).
- **Lệnh kiểm chứng:** [PRE]; [Q]; [D] `./internal/staffdeletion ./internal/handler ./internal/scheduler -run 'StaffDeletion|DeleteWorkspace|WorkspaceDeletion|DeletingTenant|Racing'`; [PD] `./internal/lifecycle -run 'OrderExpiry'`; [V].

### S11. Xóa ngay tài khoản: phía platform

- **Phụ thuộc:** S0, S9.
- **Tiêu chí / quyết định:** AC82, AC84 (kiểm đầu vào), AC85 (phía platform), AC86 (pha 1), AC97; D74, D75, D76, D78, D86.
- **File đổi:**
  - Mới: `platform-management/internal/deletionrequest/account.go`, `account_test.go`, `account_db_test.go`; `platform-management/internal/console/user_deletion.go`, `user_deletion_test.go`, `user_deletion_db_test.go`.
  - Đổi: `platform-management/queries/deletion_request.sql` (do S9 tạo; thêm truy vấn workspace của người dùng kèm số owner và số thành viên; rồi [PQ]); `platform-management/internal/console/handler.go` (`POST /users/{id}/delete-now`, `GET /users/{id}/delete-now-preview`); `platform-management/internal/staff/access.go` (hàng M25, M26: `accounts.delete_now` chỉ `system_admin`); `platform-management/audit/audit.go` (`account.immediate_deletion_requested`, `account.deleted`) và `catalog_test.go`; `platform-management/internal/accountcache` (dùng sẵn để xóa cache và phát `mul:acct:changed`).
  - Migration: không.
- **Test đỏ trước:**
  - [PG]: `TestUserDeleteNowRequiresSystemAdmin` (ma trận vai trò); `TestResolutionValidation` (bảng ca: thiếu lựa chọn, thừa lựa chọn, người nhận là `viewer`, khách, không phải thành viên, chính người bị xóa, tài khoản `suspended`, workspace khác).
  - [PD], `testdb`: `TestUserDeleteNowSoleOwnerUnresolvedReturns409WithListAndWritesNothing`; `TestUserDeleteNowCoOwnerNeedsNoResolution` (dùng số owner thật, khác `Classify`); `TestUserDeleteNowRefusesStaffAndSelf`; `TestUserDeleteNowPhaseOneBlocksAccountAndInvalidatesGuardCache` (trạng thái `pending_deletion`, `sessions_valid_after`, khóa cache bị xóa, tin `mul:acct:changed` được phát); `TestUserDeleteNowCreatesChildRequestsForDeleteChoices`; `TestUserDeleteNowPreviewWritesNothing`; `TestUserDeleteNowAuditIsFirstAndHasNoEmailInDetail`; `TestUserDeleteNowExpeditesSelfServiceSchedule`.
  - Phân quyền theo tenant, [PD]: `TestUserDeleteNowIsSystemAdminOnly` (`admin` nhận 404 phẳng ở `POST /users/{id}/delete-now` và `GET /users/{id}/delete-now-preview`, kể cả với người dùng chỉ là thành viên của workspace trong tenant mình; AC97); `TestUserDeleteNowSystemAdminAnyUser`; `TestUserDeleteNowRefusesAnyStaffRoleAsTarget` (`user_is_staff` cho cả bốn vai trò staff); `TestUserDeleteNowRoutesMatchMatrixRows` (M25, M26 × bốn vai trò).
- **Lệnh kiểm chứng:** [PRE]; [PQ]; [PG] `./internal/deletionrequest ./internal/console ./audit -run 'UserDeleteNow|Resolution|Audit'`; [PD] `./internal/deletionrequest ./internal/console -run 'DeleteNow'`.

### S12. Việc nền `staff_deletion` cho tài khoản

- **Phụ thuộc:** S10, S11.
- **Tiêu chí / quyết định:** AC83, AC84, AC85, AC86, AC87; D74, D75.
- **File đổi:**
  - Mới: `server/internal/staffdeletion/account.go`, `account_test.go`.
  - Đổi: `server/internal/handler/me_deletion.go` (`DeleteAccount` `:365`, `removeMemberForAccountDeletion` `:466`, `deleteSoleMemberWorkspace` nhận `DeletionOrigin`; kiểm toán dùng actor và `detail.origin` theo nguồn; hành vi xóa không đổi; chốt `:474` giữ nguyên); `server/pkg/db/queries/account_deletion.sql` (`ListDueAccountDeletions`, `:46`, loại trừ tài khoản có yêu cầu `pending`, `running`, `failed`; truy vấn ghi `account.deleted` và ẩn danh hóa `target_label`); `server/internal/scheduler/jobs_account_deletion.go` (chỉ nếu giao diện `AccountDeleter` cần đổi; ưu tiên không đổi).
  - Migration: không.
- **Test đỏ trước (DB-backed Go, `server/internal/testutil`):**
  - `TestStaffAccountDeletionTransfersOwnershipThenDeletes` (lựa chọn A: người nhận thành `owner`, dòng `member.role_changed` actor `staff`); `TestStaffAccountDeletionDeletesWorkspaceThenAccount` (lựa chọn B); `TestStaffAccountDeletionCoOwnerCase`; `TestStaffAccountDeletionWaitsForChildRequestsAndFailsWhenChildFailed`; `TestStaffAccountDeletionSuccessorGoneBetweenPhasesFailsAndKeepsAccount` (`error_code = sole_owner_unresolved`, người dùng chưa bị xóa); `TestNoWorkspaceIsOrphanedAfterAnyResolutionCombination` (bảng tổ hợp: A, B, hỗn hợp, đồng owner, một thành viên; sau cùng mọi workspace còn lại có ít nhất một owner); `TestStaffAccountDeletionPipelineMatchesSelfServiceRows` (fixture đôi: tập dòng còn lại giống đường tự phục vụ); `TestStaffAccountDeletionAuditActorsAreStaffAndAnonymized`; `TestStaffAccountDeletionClearsTargetLabelEmail`; `TestAccountDeletionJobSkipsAccountsWithStaffRequest`.
  - Hồi quy xanh: `server/internal/handler/me_deletion_test.go` (`sole_owner`, 14 ngày), `server/internal/scheduler/jobs_account_deletion_test.go`, `TestUserColumnRegistry`.
- **Lệnh kiểm chứng:** [PRE]; [Q]; [D] `./internal/staffdeletion ./internal/handler ./internal/scheduler ./internal/migrations -run 'StaffAccountDeletion|NoWorkspaceIsOrphaned|AccountDeletion|MeDeletion|UserColumn'`; [V].

### S13. Console: thêm người, xóa ngay, danh sách yêu cầu, quản lý staff

- **Phụ thuộc:** S0, S6, S7, S9, S11.
- **Tiêu chí / quyết định:** AC62, AC76, AC77, AC82, AC84 (giao diện), AC91 (phần `GET /me`), AC100 (giao diện); D60, D72, D75, D78, D79, D81.
- **File đổi:**
  - Mới: `platform-management/console/src/components/TypeToConfirmDialog.tsx` và `.test.tsx` (lý do bắt buộc và gõ lại chuỗi, theo `ConfirmReasonDialog.tsx`); `platform-management/console/src/screens/DeletionRequests.tsx` và `.test.tsx`.
  - Đổi (đường dẫn tương đối `platform-management/console/src/`; `WorkspaceDetail.tsx` do S6 tạo): `screens/Staff.tsx` (chọn vai trò và, với `admin`, bắt buộc chọn tenant; đổi vai trò và tenant bằng `PUT /staff/{user_id}`; hiện tenant của từng staff), `screens/WorkspaceDetail.tsx` (nút "Thêm người", danh sách yêu cầu thêm người, nút "Xóa ngay"), `screens/TenantDetail.tsx` (nút "Xóa tenant"), `screens/UserDetail.tsx` (xóa ngay kèm biểu mẫu chọn A hoặc B cho từng workspace cần xử lý, dữ liệu từ `delete-now-preview`); `api/schemas.ts`, `client.ts`, `hooks.ts` và test; `App.tsx` (mục điều hướng chỉ hiện cho `admin`).
  - Migration: không.
- **Test đỏ trước ([PW]; chuỗi theo cơ chế sẵn có của console; dialog theo `packages/ui/docs/dialog.md`):** `TypeToConfirmDialog.test.tsx` `keeps submit disabled until reason is non-empty and typed text matches`; `WorkspaceDetail.test.tsx` `add member form offers the member roles admin, member and viewer but not owner`, `shows request state and error_code`; `UserDetail.test.tsx` `lists blocking workspaces and requires a choice for each before enabling submit`, `successor list excludes viewers`; `DeletionRequests.test.tsx` `retry and cancel call the right routes`; `schemas.test.ts` `unknown request state and error_code fall back` (I9); `App.test.tsx` `system-admin-only nav items are hidden for admin, support and billing`.
  - Phân quyền theo vai trò, [PW]: `App.test.tsx` `navigation items follow the role matrix for system_admin, admin, support and billing` (mục Staff, Plans, Accounts, Orders, Overview, Deletion requests hiện đúng theo 3.6); `Staff.test.tsx` `admin role requires a tenant and other roles hide the tenant field`, `shows the 400 tenant_required message`; `Tenants.test.tsx` `admin sees only the tenant returned by the API and no delete-tenant action`; `WorkspaceDetail.test.tsx` `admin sees add member and delete workspace for a workspace in scope`; `UserDetail.test.tsx` `delete account action is shown for system_admin only`; `schemas.test.ts` `me response accepts a missing or null tenant_id and an unknown role falls back`.
- **Lệnh kiểm chứng:** [B]; [PW] `TypeToConfirm|WorkspaceDetail|TenantDetail|UserDetail|DeletionRequests|schemas|App`; [PB]; [T]. PR đính kèm ảnh chụp.

### S14. Client web: `tenant_id` khi tạo workspace

- **Phụ thuộc:** S3.
- **Tiêu chí / quyết định:** AC69 (phía client), AC89; D67.
- **File đổi:**
  - Đổi: `packages/core/types/workspace.ts` (`tenant_id?: string`); `packages/core/api/client.ts` (`createWorkspace`, `:4416`, nhận `tenant_id`); `packages/core/api/schemas.ts` (schema `Workspace` có `tenant_id` tùy chọn kèm fallback, qua `parseWithFallback`); `packages/core/workspace/mutations.ts` (`useCreateWorkspace`, `:14`) và `mutations.test.tsx`; `packages/views/onboarding/steps/step-workspace.tsx` và `step-workspace.test.tsx` (công tắc "cùng tenant với {tên workspace}" chỉ hiện khi workspace `existing` có `tenant_id`; ánh xạ lỗi `plan_limit_reached` với `gate = max_workspaces` sang toast); bảy tệp `packages/views/locales/{en,es,fr,ja,ko,zh-Hans,zh-Hant}/onboarding.json`.
  - Không đổi `apps/mobile/` (mobile tạo workspace theo đường mặc định; trường mới bị bỏ qua).
  - Migration: không.
- **Test đỏ trước:** [W] core: `useCreateWorkspace sends tenant_id only when given` (`mutations.test.tsx`); `workspace schema accepts a missing tenant_id and ignores a malformed one` (I9, test phản hồi sai dạng). [W] views, không mock `next/*`, mock API ở `@multica/core/api`: `step-workspace.test.tsx` `shows the same-tenant switch only when the existing workspace has a tenant_id`, `sends tenant_id when the switch is on`, `maps plan_limit_reached for max_workspaces to the limit toast`, `404 tenant_not_found shows the generic create-failed toast`; `packages/views/locales/parity.test.ts` đỏ cho tới khi cả bảy ngôn ngữ có khóa mới.
- **Lệnh kiểm chứng:** [B]; [W] `workspace|step-workspace|parity`; [T] `pnpm typecheck`, `pnpm lint`, `pnpm test`. Không chạy [E] trừ khi được phép.

### S15. Cổng G-R6: hồi quy, bảo mật, tài liệu

- **Phụ thuộc:** S0 đến S14.
- **Tiêu chí / quyết định:** AC88, AC89, AC90; toàn bộ rà soát AC62 đến AC90.
- **File đổi:**
  - Đổi: `docs/platform-operations.md` (quy trình thêm người cho khách, xóa ngay, thứ tự triển khai và backfill của spec R6 mục 12, bảng kiểm trước khi bật R6); đầu hai tài liệu R6 (đổi "chưa có mã" thành trạng thái thật).
  - Không sửa spec và kế hoạch gốc.
- **Việc:**
  - [PRE]; [M] trên chính `multica_platform_test` đã có. Không tạo cơ sở dữ liệu trống (điều cấm); thứ tự áp dụng số bốn chữ số trên cơ sở trống được bảo đảm bằng test tĩnh `TestFourDigitMigrationsOnlyTouchTablesCreatedEarlierInSortOrder` của S1.
  - [GA] toàn bộ test Go; [T]; [PW]; [PB]; [PI].
  - Với mỗi dòng của bảng bảo mật spec R6 mục 11, ghi tên một test cụ thể trong PR (như P40 của kế hoạch gốc).
  - Kiểm: không bước nào chạy `make test`, `make check`, `make migrate-up` hay tạo cơ sở dữ liệu; mọi chuỗi giao diện đủ bảy ngôn ngữ; không mô tả thừa (AGENTS.md "UI Copy").
  - Kiểm ma trận 3.6: chạy `TestPermissionMatrixMatchesSpec`, `TestEveryRouteInEveryRouteTableHasAMatrixRow`, `TestRoleMatrix` và test phạm vi của S5 trên bảng route cuối cùng; mọi route mới thêm sau S0 phải có hàng ở 3.6.
  - Bảng kiểm triển khai thêm bước vai trò: chạy `server/cmd/migrate up` (941 đến 946, 1016 đến 1027), triển khai liền Multica và platform (spec R6 mục 12, cửa sổ vai trò), rồi `system_admin` hạ từng tài khoản cần thành `admin` kèm `tenant_id`; ghi vào `docs/platform-operations.md`.
  - Hồi quy với mọi cấu hình mới để trống và với `MULTICA_CLOUD_URL` rỗng hoặc trỏ tới Multica Cloud (AC88).
- **Test đỏ trước:** không có test mới; slice này chạy và đối chiếu toàn bộ. Nếu một AC chưa có test chạy được thì quay lại slice tương ứng.
- **Lệnh kiểm chứng:** [PRE]; [B]; [GA]; [T]; [PW]; [PB]; [PI]; `git diff --check`; `git status --short` không có tệp lạ.

### S16. QT1: quản trị gói, phía platform

- **Phụ thuộc:** S0.
- **Tiêu chí / quyết định:** AC108, AC109, AC110, AC111; D87, D88, D89.
- **File đổi:**
  - Mới: `server/migrations/947_plan_description.up.sql` và `.down.sql` (cột `description text NOT NULL DEFAULT ''`, không chỉ mục); `platform-management/internal/subscription/plan_lifecycle.go`, `plan_lifecycle_test.go`, `plan_lifecycle_db_test.go` (`activate`, `deactivate`, `history`, `getPlan`).
  - Đổi: `platform-management/internal/subscription/console.go` (bảng route `:79`; `putPlan` `:461` nhận `description` và `confirm_affects_subscribers`, `archived` thành bí danh; `postPlan` `:593`; `deletePlan` `:664` giữ), `platform-management/internal/plan/plan.go` (`Plan.Description`, kiểm độ dài), `platform-management/internal/plan/pgx.go`, `platform-management/queries/plan.sql` (rồi [PQ]), `platform-management/internal/billingapi/billingapi.go` (`pickPlan`, `:294`: gói không liệt kê được trả `plan_inactive` thay vì "không tìm thấy"), `platform-management/audit/audit.go` (`plan.activated`, `plan.deactivated`) và `catalog_test.go`, `platform-management/internal/staff/access.go` (M30 đến M32), `platform-management/internal/schemacheck/schemacheck.go` (947), `platform-management/internal/console/testdata/plans_*.json` (phản hồi mẫu dùng chung với console).
  - Migration: 947.
- **Test đỏ trước:**
  - [PG]: `TestPlanRoutesMatchMatrixRows` (M30 đến M32 × bốn vai trò, kèm người không phiên); `TestPlanKeyIsImmutable`; `TestPlanDescriptionMaxLength`; `TestLimitLoweringNeedsConfirmWhenSubscribed` (bảng ca: hạ, nâng, từ không giới hạn xuống có giới hạn, có giới hạn lên không giới hạn, gói chưa có thuê bao); `TestPutPlanArchivedIsAliasOfDeactivate` (cùng kiểm toán).
  - [PD], `testdb`: `TestDeactivateAndActivateAreIdempotent`; `TestInactivePlanIsHiddenFromWorkspacePlansAndRefusedAtCheckout` (mã `plan_inactive`); `TestInactivePlanKeepsExistingSubscriptionsRenewingAndBuyingSeats`; `TestPlanDeleteRefusedWithSubscriptionOrOrder` (`plan_in_use`, dữ liệu không đổi); `TestPlanDeleteAllowedWhenUnused`; `TestFreePlanCannotBeDeleted`; `TestPlanHistoryListsWhoWhenOldNew` (đổi tên, mô tả, `limits`, `features`, bật, tắt); `TestPlanHistoryIncludesPriceChanges`; `TestPriceChangeDoesNotTouchPaidPeriodsOrCreatedOrders` (dùng lại kịch bản AC39); `TestPlanDescriptionMigrationDefaultsToEmpty`.
- **Lệnh kiểm chứng:** [PRE]; [M]; [PQ]; [PG] `./internal/subscription ./internal/plan ./internal/billingapi ./internal/staff ./audit ./internal/schemacheck -run 'Plan|Deactivate|Activate|Matrix|Required|Audit'`; [PD] `./internal/subscription ./internal/billingapi -run 'Plan|Inactive|History|PriceChange'`.

### S17. QT2: danh sách workspace, chi tiết và hành động nhanh, phía platform

- **Phụ thuộc:** S0, S5, S9.
- **Tiêu chí / quyết định:** AC112, AC113, AC114, AC115; D90, D91.
- **File đổi:**
  - Mới: `platform-management/internal/console/workspaces.go`, `workspaces_test.go`, `workspaces_db_test.go` (`GET /workspaces`, chi tiết mở rộng, `workspace_state_conflict`); `platform-management/internal/console/workspace_export.go`, `workspace_export_test.go` (`POST /workspaces/{id}/export`); `platform-management/queries/workspace_list.sql` (`SearchWorkspaces`, `WorkspaceStatusHistory`, `WorkspaceSeatSummary`; rồi [PQ]); `platform-management/internal/console/testdata/workspaces_*.json`.
  - Đổi: `platform-management/internal/console/handler.go` (`routeTable`, `:117`), `store.go`, `pgx.go`, `platform-management/internal/staff/access.go` (M33, M34), `platform-management/audit/audit.go` (`workspace.export_requested`) và `catalog_test.go`; các handler hành động đã có (`setWorkspaceStatus`, xóa ngay) đọc trạng thái hiện tại và trả `workspace_state_conflict`.
  - Migration: không.
- **Test đỏ trước:**
  - [PG]: `TestWorkspaceListRoutesMatchMatrixRows` (M33); `TestStaffExportRoutesMatchMatrixRows` (M34); `TestQuickActionStateConflictReturns409WithAllowed` (bảng: mỗi hành động × từng trạng thái).
  - [PD]: `TestWorkspaceListFiltersAndPaginationAreStable` (không trùng và không sót dòng khi nối `cursor`); `TestWorkspaceListSearchMatchesNameSlugOwnerEmailAndTenant`; `TestWorkspaceListScopeForAdmin` (lọc `tenant_id` khác trả rỗng); `TestWorkspaceDetailHasSeatsUsageAndStatusHistory`; `TestWorkspaceDetailSeatsUsedEqualsMemberRows`; `TestWorkspaceDetailOutsideTenantIsNotFoundForAdmin`; `TestQuickActionAuditRowsCarryWorkspaceAndTenantID`; `TestStaffExportInsertsOneQueuedRowWithStaffRequester`; `TestStaffExportRefusedWhileRunning` (`export_in_progress`).
- **Lệnh kiểm chứng:** [PRE]; [PQ]; [PG] `./internal/console ./internal/staff ./audit -run 'Workspace|Export|Matrix|QuickAction|Audit'`; [PD] `./internal/console -run 'Workspace|Export|QuickAction'`.

### S18. QT3: người dùng theo tenant, gỡ thành viên và đổi vai trò, phía platform

- **Phụ thuộc:** S0, S5, S7.
- **Tiêu chí / quyết định:** AC116, AC117, AC118, AC119, AC121; D92, D93, D94, D95.
- **File đổi:**
  - Mới: `platform-management/internal/console/tenant_users.go`, `tenant_users_test.go`, `tenant_users_db_test.go`; `platform-management/queries/tenant_users.sql` (rồi [PQ]); `platform-management/internal/memberrequest/change.go`, `change_test.go` (kiểm đầu vào `remove` và `change_role`).
  - Đổi: `platform-management/internal/memberrequest/` (`kind`, `target_user_id`, `previous_role` đã có trong migration 1022 của S7), `platform-management/internal/console/handler.go`, `platform-management/internal/staff/access.go` (M35 đến M37), `platform-management/audit/audit.go` (sáu hành động `member.staff_remove_*`, `member.staff_role_change_*`) và `catalog_test.go`, `platform-management/internal/console/testdata/tenant_users_*.json`.
  - Migration: không (S7 đã tạo đủ cột).
- **Test đỏ trước:**
  - [PG]: `TestTenantUsersRoutesMatchMatrixRows` (M35); `TestRemoveAndRoleRoutesMatchMatrixRows` (M36, M37); `TestChangeRoleRequestValidation` (vai trò đích thuộc `admin`, `member`, `viewer`; `owner` hai phía bị `owner_role_immutable`; `admin` đổi chính mình bị `cannot_change_own_role`); `TestAllocationRoutesRefuseAdmin` (không có route phân bổ trực tiếp cho `admin`).
  - [PD]: `TestTenantUsersListDefinition` (thành viên của ít nhất một workspace trong tenant, không hơn); `TestTenantUsersListFieldsAndFilters` (trường, lọc, tìm, phân trang); `TestTenantUsersHidesStaffFromAdmin`; `TestTenantUsersScopeForAdmin`; `TestUserDetailAllocationShowsSeatsAndLimits`; `TestDeactivateInTenantCreatesRemoveRequestPerWorkspace` (không đụng workspace ngoài tenant, không đổi trạng thái tài khoản); `TestGlobalLockStillSystemAdminAndSupportOnly`; `TestRemoveRequestRefusesStaffUser` (404 với `admin`, 409 `user_is_staff` với `system_admin`); `TestMemberRequestKindsShareLiveUniqueIndex`; `TestRemoveAndRoleAuditCarryTenantWorkspaceAndUser`.
- **Lệnh kiểm chứng:** [PRE]; [PQ]; [PG] `./internal/console ./internal/memberrequest ./internal/staff ./audit -run 'TenantUsers|Remove|ChangeRole|Allocation|Matrix|Audit'`; [PD] `./internal/console ./internal/memberrequest -run 'TenantUsers|Deactivate|Remove|Role|Allocation'`.

### S19. QT3: việc nền `member_provisioning` xử lý `remove` và `change_role`, phía Multica

- **Phụ thuộc:** S8, S18.
- **Tiêu chí / quyết định:** AC118, AC119, AC120; D94.
- **File đổi:**
  - Mới: `server/internal/memberprovision/remove.go`, `change_role.go`, `remove_test.go`, `change_role_test.go`.
  - Đổi: `server/internal/memberprovision/job.go` (rẽ theo `kind`), `server/internal/handler/workspace_revoke.go` (`revokeAndRemoveMember`, `:49`: bọc thành một giao diện `MemberRemover` để việc nền gọi mà không đổi hành vi handler), `server/internal/handler/seat_capacity.go` (`enqueueMemberCapacityRelease`, `:227`, dùng lại), `server/cmd/server/main.go` (nối `MemberRemover`), `server/pkg/db/queries/platform_member_request.sql` (rồi [Q]).
  - Migration: không.
- **Test đỏ trước (DB-backed Go, `server/internal/testutil`):** `TestMemberRemoveUsesRevokeAndRemoveMember` (cùng dòng còn lại như `DeleteMember` trên fixture đôi); `TestMemberRemoveReleasesSeat` (bản giả ghế theo Phụ lục B: `ReleaseMember`, số đã dùng giảm một); `TestMemberRemoveRefusesSoleOwner` (`failed`, `sole_owner`); `TestMemberRemoveTwiceAndParallelRemovesOnce`; `TestMemberRemoveAuditActorStaffWithTenantWorkspaceUser`; `TestMemberRemovePublishesEventsAndInvalidatesCache`; `TestMemberChangeRoleUpdatesRoleAndAudits`; `TestMemberChangeRoleRefusesOwnerOnEitherSide`; `TestMemberChangeRoleStaleRoleFails` (`previous_role` không khớp thì `failed`, `role_changed_meanwhile`); `TestAddRemoveAddEndsWithOneMember`; hồi quy xanh: các test `DeleteMember`, `LeaveWorkspace` và `invitation_test.go` hiện có.
- **Lệnh kiểm chứng:** [PRE]; [Q]; [D] `./internal/memberprovision ./internal/handler -run 'MemberRemove|MemberChangeRole|AddRemoveAdd|Revoke|Invitation'`; [V].

### S20. QT4: tài nguyên của tenant và toàn hệ thống, hạn mức bổ sung, phía platform

- **Phụ thuộc:** S0, S2, S5.
- **Tiêu chí / quyết định:** AC122, AC123, AC124; D96, D97.
- **File đổi:**
  - Mới: `platform-management/internal/console/resources.go`, `resources_test.go`, `resources_db_test.go`; `platform-management/internal/console/limit_overrides.go`, `limit_overrides_test.go`, `limit_overrides_db_test.go`; `platform-management/queries/resources.sql` (rồi [PQ]); `platform-management/internal/console/testdata/resources_*.json`.
  - Đổi: `platform-management/internal/usage/usage.go` (`Current` dùng lại; thêm kiểu tổng hợp theo tenant), `platform-management/internal/subscription/` (hàm `SetLimitOverrides` dùng chung với `putSubscription`, `:245`), `platform-management/internal/console/handler.go`, `platform-management/internal/staff/access.go` (M38 đến M40), `platform-management/audit/audit.go` (`subscription.limit_override_changed`) và `catalog_test.go`.
  - Migration: không (không bảng hay view mới, D96).
- **Test đỏ trước:**
  - [PG]: `TestResourceRoutesMatchMatrixRows` (M38 đến M40); `TestResourceStatusThresholds` (`ok`, `warning` từ 80 phần trăm, `over`, không giới hạn); `TestLimitOverrideValidation` (`limit_gate_invalid`, `limit_value_invalid`, `null` nhả giới hạn).
  - [PD]: `TestTenantResourcesMatchSourceTablesOnFixture` (mọi loại ở bảng 3b.4 khớp bảng nguồn); `TestTenantResourcesScopeForAdmin`; `TestSystemResourcesTopAndOverLimit` (thứ tự, vượt, phân trang, trường `complete`); `TestSystemResourcesSystemAdminOnly`; `TestLimitOverridesWriteEveryWorkspaceInOneTransaction` (một workspace lỗi thì không workspace nào đổi); `TestLimitOverridesAuditRowPerWorkspaceWithOldNew`; `TestLimitOverridesBumpVersionAndClearCache`; `TestLimitOverridesCreateFreeRowForUnsubscribedWorkspace`; `TestLimitOverridesReachPolicyWithin30Seconds` (đồng hồ giả).
- **Lệnh kiểm chứng:** [PRE]; [PQ]; [PG] `./internal/console ./internal/usage ./internal/subscription ./internal/staff ./audit -run 'Resource|LimitOverride|Matrix|Audit'`; [PD] `./internal/console ./internal/subscription -run 'Resource|LimitOverride'`.

### S21. QT5: runtime, phía platform

- **Phụ thuộc:** S0, S5, S9.
- **Tiêu chí / quyết định:** AC125, AC126, AC127 (phía ghi), AC128 (tạo yêu cầu); D98, D99.
- **File đổi:**
  - Mới: `server/migrations/1028_platform_runtime`, `1029_platform_runtime_uidx` (mỗi tên có `.up.sql`, `.down.sql`); `platform-management/internal/runtimeadmin/` (`runtimeadmin.go`, `service.go`, `pgx.go` và `_test.go`); `platform-management/internal/console/runtimes.go`, `runtimes_test.go`, `runtimes_db_test.go`; `platform-management/queries/runtime.sql` (rồi [PQ]); `platform-management/internal/console/testdata/runtimes_*.json`.
  - Đổi: `platform-management/internal/console/handler.go`, `platform-management/internal/staff/access.go` (M41 đến M43), `platform-management/internal/deletionrequest/` (nhận `kind = runtime`, `confirm` là tên runtime), `platform-management/audit/audit.go` (`runtime.disabled`, `runtime.enabled`, `runtime.unregister_requested`, `runtime.unregistered`) và `catalog_test.go`, `platform-management/internal/schemacheck/schemacheck.go` (1028, 1029), `server/cmd/migrate/main.go` (`:142`, dòng dọn 1029), `platform-management/internal/console/isolation_test.go` (không chọn cột nội dung task).
  - Migration: 1028, 1029.
- **Test đỏ trước:** [PG]: `TestRuntimeRoutesMatchMatrixRows` (M41 đến M43); `TestRuntimeListOmitsIPAndRawDeviceInfo` (quét khóa JSON: không `ip`, `device_info`, `metadata`); `TestRuntimeUnregisterRequiresReasonAndConfirm`. [PD]: `TestRuntimeListScopeAndFilters`; `TestRuntimeDetailRecentTasksHaveNoContent`; `TestRuntimeOutsideTenantIsNotFoundForAdmin`; `TestRuntimeStatusWritesRowAuditAndClearsCache`; `TestRuntimeStatusIsIdempotent`; `TestRuntimeUnregisterCreatesRuntimeKindRequestWithAuditFirst`; `TestPlatformRuntimeTableHasNoForeignKey`.
- **Lệnh kiểm chứng:** [PRE]; [M]; [PQ]; [PG] `./internal/runtimeadmin ./internal/console ./internal/deletionrequest ./internal/staff ./audit ./internal/schemacheck -run 'Runtime|Matrix|Required|Audit|Isolation'`; [PD] `./internal/runtimeadmin ./internal/console -run 'Runtime'`; [GN] `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'` (trong `server/`).

### S22. QT5: chặn runtime bị vô hiệu hóa và gỡ đăng ký, phía Multica

- **Phụ thuộc:** S21, S10.
- **Tiêu chí / quyết định:** AC127, AC128; D98.
- **File đổi:**
  - Mới: `server/internal/runtimeguard/runtimeguard.go` và `runtimeguard_test.go` (đọc `platform_runtime` qua cache Redis, khuôn `platform-management/workspacestate`; DB lỗi trả 503, I8); `server/pkg/db/queries/platform_runtime.sql` (rồi [Q]); `server/internal/staffdeletion/runtime.go`, `runtime_test.go`.
  - Đổi: `server/internal/handler/daemon.go` (`requireDaemonRuntimeAccess`, `:183`, và `DaemonRegister`, `:571`: 403 `runtime_disabled`), `server/internal/service/budget_gate.go` (`claimRefusalFor`, `:320`: từ chối giao task), `server/internal/handler/runtime.go` (tách lõi xóa từ `DeleteAgentRuntime`, `:1337`, để việc nền dùng; xóa dòng `platform_runtime`), `server/internal/handler/workspace.go` (bước "delete runtimes and projects" của `deleteWorkspaceData`, xóa `platform_runtime` của workspace), `server/internal/handler/workspace_delete_manifest_test.go`, `server/cmd/server/main.go`.
  - Migration: không.
- **Test đỏ trước (DB-backed Go, `server/internal/testutil`):** `TestDisabledRuntimeCannotRegister`; `TestDisabledRuntimeRefusedOnEveryDaemonRoute` (bảng mọi route dùng `requireDaemonRuntimeAccess`, gồm nhịp tim); `TestDisabledRuntimeGetsNoTaskClaimAndTaskStaysQueued`; `TestRuntimeGuardCacheInvalidatedOnChange`; `TestRuntimeGuardDBErrorIs503`; `TestEnableRestoresRegisterHeartbeatRoutesAndClaim`; `TestStaffRuntimeUnregisterUsesDeleteAgentRuntimeCore`; `TestStaffRuntimeUnregisterEquivalentToOwnerDelete` (fixture đôi); `TestStaffRuntimeUnregisterAuditActorStaffWithTenantAndWorkspace`; `TestDeleteRuntimeAndWorkspaceRemovePlatformRuntimeRows` (bản kê khai); hồi quy xanh: test daemon hiện có (`daemon_claim_workspace_mcp_test.go`, `workspace_delete_manifest_test.go`).
- **Lệnh kiểm chứng:** [PRE]; [Q]; [D] `./internal/runtimeguard ./internal/staffdeletion ./internal/handler ./internal/service -run 'Runtime|Daemon|Manifest|Claim'`; [V].

### S23. Frontend console: gói, danh sách workspace **[Frontend, chuyển cho agy]**

- **Phụ thuộc:** S6, S13, S16, S17.
- **Tiêu chí / quyết định:** AC108 đến AC115 (giao diện); D87 đến D91.
- **File đổi (đường dẫn tương đối `platform-management/console/src/`):**
  - Mới: `screens/Workspaces.tsx`, `screens/Workspaces.test.tsx`, `screens/PlanHistory.tsx`, `screens/PlanHistory.test.tsx`.
  - Đổi: `screens/Plans.tsx` (mô tả, nút bật và tắt, xóa với thông báo `plan_in_use`, xác nhận khi hạ giới hạn, ngăn lịch sử; `admin` chỉ đọc), `screens/WorkspaceDetail.tsx` (ghế, mức dùng, lịch sử trạng thái, hàng nút hành động nhanh), `App.tsx` (menu theo ma trận), `api/schemas.ts`, `client.ts`, `hooks.ts` và test; hợp đồng JSON lấy từ `platform-management/internal/console/testdata/` của S16 và S17.
- **Test đỏ trước ([PW]):** `Workspaces.test.tsx` `filters by status plan and tenant and pages with the cursor`, `admin sees only the rows the API returns`; `Plans.test.tsx` `admin sees plans read only`, `deactivate and activate call the routes and show the inactive badge`, `delete shows plan_in_use message`, `lowering a limit asks for confirmation`; `PlanHistory.test.tsx` `renders who when old new`; `WorkspaceDetail.test.tsx` `quick actions follow allowed[] and show the 409 state conflict`; `schemas.test.ts` `unknown plan state and workspace status fall back`.
- **Lệnh kiểm chứng:** [B]; [PW] `Workspaces|Plans|PlanHistory|WorkspaceDetail|schemas|App`; [PB]; [T]. PR đính kèm ảnh chụp.

### S24. Frontend console: người dùng theo tenant **[Frontend, chuyển cho agy]**

- **Phụ thuộc:** S13, S18.
- **Tiêu chí / quyết định:** AC116 đến AC121 (giao diện); D92 đến D95.
- **File đổi (tương đối `platform-management/console/src/`):** Mới: `screens/TenantUsers.tsx` và `.test.tsx` (tab trong `TenantDetail`). Đổi: `screens/TenantDetail.tsx`, `screens/UserDetail.tsx` (hành động: vô hiệu hóa trong tenant, gỡ khỏi workspace, đổi vai trò thành viên; nút phân bổ mở thao tác ở workspace hoặc form yêu cầu tăng hạn mức; nút khóa toàn cục và xóa chỉ hiện đúng vai trò), `api/*`.
- **Test đỏ trước ([PW]):** `TenantUsers.test.tsx` `lists users with workspaces roles status seats and last login`, `search filter and paging`; `UserDetail.test.tsx` `tenant deactivate asks reason and lists the workspaces it removes the user from`, `change role offers admin member viewer only`, `global lock and delete hidden for admin`, `allocation opens the workspace action not a per-user quota`; `schemas.test.ts` `tenant user row falls back for unknown account_status`.
- **Lệnh kiểm chứng:** [B]; [PW] `TenantUsers|UserDetail|TenantDetail|schemas`; [PB]; [T].

### S25. Frontend console: tài nguyên và runtime **[Frontend, chuyển cho agy]**

- **Phụ thuộc:** S13, S20, S21.
- **Tiêu chí / quyết định:** AC122 đến AC128 (giao diện); D96 đến D99.
- **File đổi (tương đối `platform-management/console/src/`):** Mới: `screens/Resources.tsx` (toàn hệ thống, `system_admin`), `screens/Runtimes.tsx`, `screens/RuntimeDetail.tsx` và test tương ứng; Đổi: `screens/TenantDetail.tsx` (tab Tài nguyên với thanh mức dùng so hạn mức, nút "Đặt hạn mức bổ sung" chỉ cho `system_admin` và `billing`), `App.tsx`, `api/*`.
- **Test đỏ trước ([PW]):** `Resources.test.tsx` `shows top tenants and over limit list for system_admin only`; `TenantDetail.test.tsx` `resource bars show used limit percent and warning over states`, `limit override form hidden for admin`; `Runtimes.test.tsx` `lists runtimes with status last seen versions and no ip column`; `RuntimeDetail.test.tsx` `disable asks reason and shows disabled badge`, `unregister requires typing the runtime name`.
- **Lệnh kiểm chứng:** [B]; [PW] `Resources|Runtimes|RuntimeDetail|TenantDetail|App`; [PB]; [T].
### S26. QT11: giữ hồ sơ pháp lý, phía platform

- **Phụ thuộc:** S9, S11.
- **Tiêu chí / quyết định:** AC136, AC137 (pha 1); D104.
- **File đổi:**
  - Mới: `server/migrations/1035_legal_hold`, `1036_legal_hold_id_uidx`, `1037_legal_hold_live_uidx` (mỗi tên có `.up.sql`, `.down.sql`); `platform-management/internal/legalhold/` (`legalhold.go`, `service.go`, `pgx.go` và `_test.go`); `platform-management/internal/console/legal_holds.go`, `legal_holds_test.go`, `legal_holds_db_test.go`; `platform-management/queries/legal_hold.sql` (rồi [PQ]).
  - Đổi: `platform-management/internal/deletionrequest/` (pha 1 của xóa ngay workspace, tenant, tài khoản kiểm giữ trước mọi ghi, trả `legal_hold`), `platform-management/internal/console/handler.go`, `platform-management/internal/staff/access.go` (M50), `platform-management/audit/audit.go` (`legal_hold.placed`, `legal_hold.released`, `deletion.blocked_by_hold`) và `catalog_test.go`, `platform-management/internal/schemacheck/schemacheck.go` (1035 đến 1037), `server/cmd/migrate/main.go` (dòng dọn 1036, 1037).
  - Migration: 1035, 1036, 1037.
- **Test đỏ trước:** [PG]: `TestLegalHoldRoutesMatchMatrixRows` (M50: chỉ `system_admin`; `admin` 404 phẳng); `TestLegalHoldRequiresReasonAndCaseRef`; `TestLegalHoldAuditDetailHasOnlyHoldID`. [PD]: `TestSecondHoldOnSameScopeIs409`; `TestReleaseKeepsRowAsHistory`; `TestHoldOnTenantCoversWorkspaceAndAccountChecks` (hỏi theo workspace tìm được giữ ở tenant); `TestDeleteNowRefusedWhileHeldWritesNothing` (workspace, tenant, tài khoản; không dòng yêu cầu, không đổi trạng thái); `TestAdminGetsLegalHoldErrorWithoutReasonOrCaseRef`; `TestSystemAdminGetsHoldIDInError`; `TestLegalHoldTablesHaveNoForeignKey`.
- **Lệnh kiểm chứng:** [PRE]; [M]; [PQ]; [PG] `./internal/legalhold ./internal/console ./internal/deletionrequest ./internal/staff ./audit ./internal/schemacheck -run 'LegalHold|Matrix|Required|Audit|DeleteNow'`; [PD] `./internal/legalhold ./internal/deletionrequest -run 'LegalHold|Hold'`; [GN] `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'` (trong `server/`).

### S27. QT11: giữ hồ sơ pháp lý chặn mọi đường xóa, phía Multica

- **Phụ thuộc:** S26, S10, S12.
- **Tiêu chí / quyết định:** AC137, AC138, AC140 (điều kiện giữ ở truy vấn xóa nhật ký); D104, D105.
- **File đổi:**
  - Mới: `server/internal/legalhold/legalhold.go` và `legalhold_test.go` (đọc giữ theo workspace, tenant của workspace, tài khoản); `server/pkg/db/queries/legal_hold.sql` (rồi [Q]).
  - Đổi: `server/internal/handler/workspace.go` (`deleteWorkspaceData`, `:1567`: kiểm giữ ở đầu hàm, `ErrLegalHold`; `DeleteWorkspace`, `:1436`: ánh xạ 409 `legal_hold`), `server/internal/handler/me_deletion.go` (`DeleteAccount`, `:365`: kiểm ở đầu), `server/internal/handler/workspace_deletion.go` (`DeleteDueWorkspace`, `:170`: coi `ErrLegalHold` là bỏ qua), `server/internal/scheduler/jobs_workspace_deletion.go`, `server/internal/scheduler/jobs_account_deletion.go` (ghi log và bỏ qua), `server/internal/staffdeletion/` (yêu cầu dừng `failed` với `error_code = legal_hold`, dữ liệu nguyên), `server/pkg/db/queries/audit_event.sql` (`DeleteAuditEventsBefore`, `:38`: thêm điều kiện loại dòng thuộc đối tượng bị giữ) và `server/internal/scheduler/jobs_audit_retention.go` (`:54`), `server/internal/handler/workspace_delete_manifest_test.go` (`legal_hold` là `keep`).
  - Migration: không.
- **Test đỏ trước (DB-backed Go, `server/internal/testutil`):** `TestDeleteWorkspaceDataRefusesHeldWorkspace` (giữ trên workspace, trên tenant, trên tenant rồi tạo workspace mới); `TestDeleteAccountRefusesHeldAccount`; `TestDeleteAccountOfSoleMemberOfHeldWorkspaceIsRefused`; `TestOwnerDeleteWorkspaceReturnsLegalHold409`; `TestWorkspaceDeletionJobSkipsHeldWorkspace`; `TestAccountDeletionJobSkipsHeldAccount`; `TestStaffDeletionFailsWithLegalHoldAndKeepsData`; `TestAuditRetentionSkipsHeldRows`; `TestReleaseResumesDeletionAndRetentionOnNextRun`; `TestHeldObjectOrdinaryOperationsUnaffected` (thêm thành viên, ghi issue, đổi gói); `TestDeletionBlockedByHoldWritesAuditRow`; `TestEveryWorkspaceContentDeleteStatementIsInsideDeleteWorkspaceData` (quét tĩnh `server/internal/handler/*.go` và gói việc nền: truy vấn `DeleteWorkspace*` chỉ được gọi từ `deleteWorkspaceData`); `TestAuditQueriesReadonlyStillPasses` (`platform-management/audit/queries_readonly_test.go`).
- **Lệnh kiểm chứng:** [PRE]; [Q]; [D] `./internal/legalhold ./internal/handler ./internal/scheduler ./internal/staffdeletion -run 'LegalHold|Held|Retention|Manifest|DeleteWorkspace|AccountDeletion|StaffDeletion'`; [PG] `./audit -run 'AuditQueriesAreInsertAndSelectOnly'` (trong `platform-management/`); [V].

### S28. QT10: duyệt chéo xóa ngay tenant và tài khoản

- **Phụ thuộc:** S9, S11, S26.
- **Tiêu chí / quyết định:** AC134, AC135; D103.
- **File đổi:**
  - Mới: `platform-management/internal/deletionrequest/approval.go`, `approval_test.go`, `approval_db_test.go`; `platform-management/internal/console/approvals.go`, `approvals_test.go`; `platform-management/internal/jobs/approval_expiry.go`, `approval_expiry_test.go`.
  - Đổi: `platform-management/internal/config/config.go` (`Load`, `:215`: đọc `PLATFORM_APPROVAL_ACTIONS`, chỉ nhận `tenant.delete_now` và `account.delete_now`, tên lạ làm khởi động thất bại), `config_test.go`; `platform-management/internal/deletionrequest/` (dừng ở `awaiting_approval` khi hành động nằm trong danh sách; đếm `system_admin` hoạt động), `platform-management/internal/console/handler.go`, `platform-management/internal/staff/access.go` (M49), `platform-management/audit/audit.go` (`deletion.approval_requested`, `deletion.approved`, `deletion.rejected`, `deletion.approval_expired`) và `catalog_test.go`, `platform-management/cmd/platform-server` (đăng ký việc nền `deletion_approval_expiry`).
  - Migration: không (trạng thái và cột đã có trong migration 1025 của S9).
- **Test đỏ trước:** [PG]: `TestApprovalConfigParsesKnownActionsOnly`; `TestApprovalRoutesMatchMatrixRows` (M49). [PD]: `TestAwaitingApprovalWritesNoStateChangeOrAccessBlock`; `TestApproveRunsPhaseOneInOneTransaction`; `TestApproverMustDiffer` (409); `TestSingleSystemAdminCannotRequireApproval` (409 `approver_unavailable`, không dòng yêu cầu); `TestRejectCancelsRequest`; `TestApprovalExpiresAfter24HoursIdempotent` (đồng hồ giả, việc nền hai lần); `TestDefaultEmptyConfigKeepsDirectDeleteNow` (AC96, AC97 vẫn xanh); `TestAdminCannotSeeOrActOnApprovals`; `TestLegalHoldCheckedBeforeApprovalIsRequested`.
- **Lệnh kiểm chứng:** [PRE]; [PG] `./internal/config ./internal/deletionrequest ./internal/console ./internal/jobs ./internal/staff ./audit -run 'Approval|Matrix|Config|Audit'`; [PD] `./internal/deletionrequest ./internal/jobs -run 'Approval|DeleteNow'`; [PG] `./cmd/platform-server -run 'Config|Jobs'`.

### S29. QT6: chính sách tenant, phía platform

- **Phụ thuộc:** S0, S5.
- **Tiêu chí / quyết định:** AC129; D101.
- **File đổi:**
  - Mới: `server/migrations/1030_tenant_policy`, `1031_tenant_policy_uidx` (mỗi tên có `.up.sql`, `.down.sql`); `platform-management/internal/tenantpolicy/` (`tenantpolicy.go`, `validate.go`, `pgx.go` và `_test.go`); `platform-management/internal/console/tenant_policy.go`, `tenant_policy_test.go`, `tenant_policy_db_test.go`; `platform-management/queries/tenant_policy.sql` (rồi [PQ]).
  - Đổi: `platform-management/internal/console/handler.go`, `platform-management/internal/staff/access.go` (M44, M45), `platform-management/audit/audit.go` (`tenant.policy_changed`) và `catalog_test.go`, `platform-management/internal/schemacheck/schemacheck.go` (1030, 1031), `server/cmd/migrate/main.go` (dòng dọn 1031).
  - Migration: 1030, 1031.
- **Test đỏ trước:** [PG]: `TestTenantPolicyRoutesMatchMatrixRows` (M44, M45); `TestTenantPolicyValidationMatchesWorkspacePolicyBounds` (cùng giới hạn với `workspacepolicy`). [PD]: `TestTenantPolicyWriteNeedsWorkspacePoliciesFeatureSomewhereInTenant` (402 `plan_feature_unavailable`); `TestTenantPolicyVersionConflict`; `TestTenantPolicyAuditHasOldAndNewAndTenantID`; `TestTenantPolicyScopeForAdmin`; `TestTenantPolicyTableHasNoForeignKey`.
- **Lệnh kiểm chứng:** [PRE]; [M]; [PQ]; [PG] `./internal/tenantpolicy ./internal/console ./internal/staff ./audit ./internal/schemacheck -run 'TenantPolicy|Matrix|Required|Audit'`; [PD] `./internal/tenantpolicy ./internal/console -run 'TenantPolicy'`.

### S30. QT6: hợp chính sách và sàn tenant, phía Multica

- **Phụ thuộc:** S29, S1.
- **Tiêu chí / quyết định:** AC129, AC130; D101.
- **File đổi:**
  - Mới: `server/internal/workspacepolicy/merge.go` và `merge_test.go` (`Merge(tenant, workspace)`); `server/pkg/db/queries/tenant_policy.sql` (rồi [Q]).
  - Đổi: `server/internal/workspaceguard/policy_access.go` (`EffectivePolicy`, `:143`: đọc dòng tenant của workspace và gọi `Merge`), `server/internal/handler/workspace_policy.go` (`UpdateWorkspacePolicy`, `:142`: 400 `policy_below_tenant_floor`; `GetWorkspacePolicy` trả `floor`), `server/internal/tenant/tenant.go` (`Detach` xóa dòng `tenant_policy`), `server/internal/handler/workspace_delete_manifest_test.go`.
  - Migration: không.
- **Test đỏ trước:** [GN]: `TestMergeTable` (giao tên miền, hội `RequireSSO` và `RequireMFA`, AND của `ShareLinksEnabled`, nhỏ hơn khác 0 của `SessionMaxAgeHours`, danh sách rỗng một phía, hai phía rỗng). [D]: `TestEffectivePolicyAppliesTenantFloorToEveryWorkspaceRegardlessOfPlan`; `TestWorkspacePolicyWriteBelowFloorIs400WithFields`; `TestStoredLooseWorkspacePolicyIsTightenedAtEnforcement`; `TestNoTenantPolicyRowKeepsBehaviour` (hồi quy `workspace_policy` hiện có xanh); `TestGetWorkspacePolicyReturnsFloor`; `TestDetachTenantDeletesPolicyRow`; `TestOwnerStillSignsInWithEmailCodeUnderTenantSSOFloor` (D19).
- **Lệnh kiểm chứng:** [PRE]; [Q]; [GN] `./internal/workspacepolicy -run 'Merge'`; [D] `./internal/workspaceguard ./internal/handler ./internal/tenant -run 'Policy|Floor|Detach|Manifest'`; [V].

### S31. QT7: cảnh báo hạn mức và yêu cầu tăng hạn mức, phía platform

- **Phụ thuộc:** S0, S5, S20.
- **Tiêu chí / quyết định:** AC131, AC132, AC133; D102.
- **File đổi:**
  - Mới: `server/migrations/1032_quota_request`, `1033_quota_request_id_uidx`, `1034_quota_request_live_uidx` (mỗi tên có `.up.sql`, `.down.sql`); `platform-management/internal/quota/` (`alert.go`, `request.go`, `pgx.go` và `_test.go`); `platform-management/internal/jobs/quota_alert.go`, `quota_alert_test.go`; `platform-management/internal/console/quota_requests.go`, `quota_requests_test.go`, `quota_requests_db_test.go`; `platform-management/queries/quota.sql` (rồi [PQ]); mẫu thư ở `platform-management/internal/lifecycle/templates/` (`quota_80.txt.tmpl`, `quota_100.txt.tmpl`, `quota_result.txt.tmpl`).
  - Đổi: `platform-management/internal/console/handler.go`, `platform-management/internal/staff/access.go` (M46 đến M48), `platform-management/audit/audit.go` (`quota.request_created`, `quota.request_approved`, `quota.request_denied`) và `catalog_test.go`, `platform-management/internal/subscription/` (dùng `SetLimitOverrides` của S20 khi duyệt), `platform-management/internal/schemacheck/schemacheck.go` (1032 đến 1034), `server/cmd/migrate/main.go` (dòng dọn 1033, 1034), `platform-management/cmd/platform-server` (đăng ký `quota_alert`).
  - Migration: 1032, 1033, 1034.
- **Test đỏ trước:** [PG]: `TestQuotaRequestRoutesMatchMatrixRows` (M46 đến M48); `TestQuotaRequestValidation` (`requested_value` phải lớn hơn hiệu lực, độ dài lý do, `gate` thuộc `plan.LimitGates`). [PD]: `TestQuotaAlertSendsOnceAt80AndOnceAt100` (bảng loại tài nguyên); `TestQuotaAlertTwiceAndParallelSendsOne`; `TestQuotaAlertSkipsPendingDeletionWorkspace`; `TestQuotaAlertDoesNotChangeEnforcement`; `TestQuotaRequestOnePendingPerWorkspaceGate`; `TestApproveWritesOverrideAndAuditInOneTransaction`; `TestApproveLowerThanRequested`; `TestDenyRequiresReason`; `TestApproveAndDenyOnlySystemAdmin`; `TestQuotaRequestScopeForAdminAndBilling`; `TestApprovedOverrideReachesPolicyWithin30Seconds`; `TestResultEmailGoesToRequester`.
- **Lệnh kiểm chứng:** [PRE]; [M]; [PQ]; [PG] `./internal/quota ./internal/jobs ./internal/console ./internal/staff ./audit ./internal/schemacheck -run 'Quota|Matrix|Required|Audit'`; [PD] `./internal/quota ./internal/jobs ./internal/console -run 'Quota'`; [GN] `./cmd/migrate -run 'TestEveryConcurrentUpBuildHasCleanup'` (trong `server/`).

### S32. QT7: route yêu cầu tăng hạn mức cho owner, phía Multica

- **Phụ thuộc:** S31.
- **Tiêu chí / quyết định:** AC132; D102.
- **File đổi:**
  - Mới: `server/internal/handler/quota_request.go`, `quota_request_test.go`; `server/pkg/db/queries/quota_request.sql` (rồi [Q]).
  - Đổi: `server/cmd/server/router.go` (nhóm `/api/workspaces/{id}`, sau `RequireHumanActor`, quyền `billing.manage`), `server/internal/handler/workspace.go` (bước "delete platform state", `:2007`: xóa `quota_request` theo workspace), `server/internal/handler/workspace_delete_manifest_test.go`.
  - Migration: không.
- **Test đỏ trước (DB-backed Go):** `TestCreateQuotaRequestByOwner`; `TestCreateQuotaRequestByPlainMemberIsPermissionDenied`; `TestQuotaRequestMachineTokenIs403`; `TestQuotaRequestValueMustExceedEffectiveLimit` (nhà cung cấp quyền lợi giả); `TestDuplicatePendingIs409`; `TestListQuotaRequestsScopedToWorkspace`; `TestDeleteWorkspaceRemovesQuotaRequests`.
- **Lệnh kiểm chứng:** [PRE]; [Q]; [D] `./internal/handler -run 'QuotaRequest|Manifest'`; [V].

### S33. QT18: xuất nhật ký kiểm toán cho staff

- **Phụ thuộc:** S0, S5.
- **Tiêu chí / quyết định:** AC139, AC140; D105.
- **File đổi:**
  - Mới: `platform-management/internal/console/audit_export.go`, `audit_export_test.go`, `audit_export_db_test.go`; `platform-management/queries/audit_export.sql` (truy vấn khóa `(created_at, id)`; rồi [PQ]).
  - Đổi: `platform-management/internal/console/handler.go`, `platform-management/internal/staff/access.go` (M51), `platform-management/audit/audit.go` (`audit.exported`) và `catalog_test.go`, `platform-management/audit/queries_readonly_test.go` (giữ nguyên quy tắc; thêm đường dẫn truy vấn mới vào phạm vi quét).
  - Migration: không (chỉ mục `audit_event_created_idx` ở `server/migrations/660_audit_event_created_idx.up.sql` và chỉ mục tenant ở 946 đủ).
- **Test đỏ trước:** [PG]: `TestAuditExportRoutesMatchMatrixRows` (M51); `TestExportRangeLimits` (31 ngày, `from` sau `to`); `TestAuditQueriesReadonlyStillPasses`. [PD]: `TestExportFormatsNdjsonAndCsv`; `TestExportCursorNoDuplicateNoMissAcrossPages`; `TestExportScopeAdminOwnTenantIncludingDeletedWorkspaceRows`; `TestExportStreamsWithBoundedMemory` (móc đếm dòng đang giữ); `TestExportWritesAuditRowWithFilterAndCount`; `TestExportAddsNoFieldsBeyondConsoleRead`.
- **Lệnh kiểm chứng:** [PRE]; [PQ]; [PG] `./internal/console ./internal/staff ./audit -run 'AuditExport|Export|Matrix|Readonly|Audit'`; [PD] `./internal/console -run 'AuditExport|Export'`.

### S34. QT19: khóa cả tenant, báo owner và xuất trước khi xóa, phía platform

- **Phụ thuộc:** S5, S9, S11.
- **Tiêu chí / quyết định:** AC141, AC142 (pha 1); D106.
- **File đổi:**
  - Mới: `platform-management/internal/tenantstatus/` (`tenantstatus.go`, `pgx.go` và `_test.go`); `platform-management/internal/deletionrequest/notice.go`, `notice_test.go` (email owner ở pha 1; chèn `workspace_export` khi `export_first`); mẫu thư `platform-management/internal/mailer/templates/deletion_notice.txt.tmpl` (nếu gói `mailer` chưa có thư mục mẫu thì đặt cạnh mẫu hiện có của gói đó).
  - Đổi: `platform-management/internal/console/handler.go` (`POST /tenants/{id}/status`), `tenants.go`, `platform-management/internal/staff/access.go` (M52), `platform-management/internal/deletionrequest/` (nhận `export_first`; gọi `notice`), `platform-management/audit/audit.go` (`tenant.status_changed`, `deletion.notice_sent`, `deletion.notice_failed`, `deletion.export_ready`) và `catalog_test.go`.
  - Migration: không (cột `tenant.status` nằm trong 1016 của S1; `export_first`, `notice_sent_at` nằm trong 1025 của S9).
- **Test đỏ trước:** [PG]: `TestTenantStatusRoutesMatchMatrixRows` (M52). [PD]: `TestTenantStatusWritesAllWorkspacesInOneTransaction`; `TestTenantUnlockRestoresOnlyTenantStatusRows`; `TestTenantStatusAuditRows`; `TestTenantStatusFailureWritesNothing`; `TestDeleteNowSendsNoticeToOwnersNotTheDeletedUser`; `TestNoticeFailureDoesNotBlockDeletionAndIsAudited`; `TestExportFirstQueuesExportForEveryWorkspace`; `TestExportFirstRefusedWhenExportAlreadyRunningReusesIt`.
- **Lệnh kiểm chứng:** [PRE]; [PG] `./internal/tenantstatus ./internal/deletionrequest ./internal/console ./internal/staff ./audit -run 'TenantStatus|Notice|ExportFirst|Matrix|Audit'`; [PD] `./internal/tenantstatus ./internal/deletionrequest -run 'TenantStatus|Notice|ExportFirst'`.

### S35. QT19: chặn tạo workspace vào tenant bị khóa và chờ bản xuất trước khi xóa, phía Multica

- **Phụ thuộc:** S34, S10, S3, S27.
- **Tiêu chí / quyết định:** AC141, AC142; D106.
- **File đổi:**
  - Đổi: `server/internal/handler/workspace.go` (`CreateWorkspace`, `:328`: đọc `tenant.status` dưới khóa tenant, 409 `tenant_not_active`), `server/pkg/db/queries/tenant.sql` (đọc trạng thái; rồi [Q]), `server/internal/staffdeletion/job.go` và `workspace.go` (khi `export_first`: chờ bản xuất `done` tối đa 24 giờ, gửi owner liên kết tải có chữ ký hạn 7 ngày bằng cơ chế của `server/internal/workspaceexport`, rồi xóa; hết hạn thì `failed` `export_timeout`), `server/internal/service/email.go` (thư liên kết tải).
  - Migration: không.
- **Test đỏ trước (DB-backed Go):** `TestCreateWorkspaceIntoSuspendedTenantIs409`; `TestCreateWorkspaceIntoReadOnlyTenantIs409`; `TestCreateWorkspaceIntoActiveTenantUnchanged`; `TestStaffDeletionWaitsForExportThenMailsSignedLink`; `TestStaffDeletionExportTimeoutFailsAndKeepsData`; `TestStaffDeletionWithoutExportFirstDoesNotWait`; `TestExportArchiveSurvivesWorkspaceDeletion`; `TestLegalHoldCheckedBeforeExportWait`.
- **Lệnh kiểm chứng:** [PRE]; [Q]; [D] `./internal/handler ./internal/staffdeletion ./internal/workspaceexport -run 'TenantNotActive|SuspendedTenant|ReadOnlyTenant|ExportFirst|ExportTimeout|ExportArchive|LegalHold'`; [V].

### S36. Frontend console: chính sách tenant, yêu cầu hạn mức, duyệt, giữ hồ sơ, xuất kiểm toán, khóa tenant **[Frontend, chuyển cho agy]**

- **Phụ thuộc:** S13, S26, S28, S29, S31, S33, S34.
- **Tiêu chí / quyết định:** AC129 đến AC142 (giao diện); D101 đến D106.
- **File đổi (tương đối `platform-management/console/src/`):** Mới: `screens/TenantPolicy.tsx`, `screens/QuotaRequests.tsx`, `screens/Approvals.tsx`, `screens/LegalHolds.tsx` và test tương ứng. Đổi: `screens/Audit.tsx` (biểu mẫu xuất: khoảng ngày, định dạng, giới hạn 31 ngày), `screens/TenantDetail.tsx` (tab Chính sách; nút khóa và mở cả tenant; dấu "đang giữ" cho `system_admin`), `screens/WorkspaceDetail.tsx` (nút "Yêu cầu tăng hạn mức"), `components/TypeToConfirmDialog.tsx` (công tắc `export_first`, thông báo owner), `screens/DeletionRequests.tsx` (trạng thái `awaiting_approval`, `legal_hold`, `export_timeout`), `App.tsx`, `api/*`.
- **Test đỏ trước ([PW]):** `TenantPolicy.test.tsx` `shows merged effective policy and which value comes from the tenant`, `admin edits own tenant policy only`; `QuotaRequests.test.tsx` `system_admin approves with a lower value and gives a reason`, `admin cannot approve`; `Approvals.test.tsx` `approver must differ message and refuses own request`; `LegalHolds.test.tsx` `place requires reason and case ref`, `admin has no menu entry`; `Audit.test.tsx` `export form rejects ranges over 31 days`; `TypeToConfirmDialog.test.tsx` `export first toggle is sent and owners notice is stated`; `DeletionRequests.test.tsx` `awaiting_approval legal_hold and export_timeout states render`.
- **Lệnh kiểm chứng:** [B]; [PW] `TenantPolicy|QuotaRequests|Approvals|LegalHolds|Audit|TypeToConfirm|DeletionRequests|App`; [PB]; [T].

### S37. Frontend web: yêu cầu tăng hạn mức và hiện sàn chính sách **[Frontend, chuyển cho agy]**

- **Phụ thuộc:** S30, S32.
- **Tiêu chí / quyết định:** AC130, AC132 (giao diện); D101, D102.
- **File đổi:** Đổi: `packages/views/settings/components/billing-tab.tsx` (khi `plan_limit_reached`, nút "Yêu cầu tăng hạn mức" và danh sách yêu cầu của workspace) và `billing-tab.test.tsx`; `packages/views/settings/components/policy-tab.tsx` (hiện giá trị do tenant áp từ `floor` và khóa trường không được nới) và `policy-tab.test.tsx`; `packages/core/api/schemas.ts` và `client.ts` (`QuotaRequest`, trường `floor` tùy chọn, `parseWithFallback`); `packages/core/types/` (kiểu mới); bảy tệp `packages/views/locales/{en,es,fr,ja,ko,zh-Hans,zh-Hant}/settings.json` (chuỗi mới, không mô tả thừa). Không đụng `apps/mobile/`.
- **Test đỏ trước ([W], không mock `next/*`, mock API ở `@multica/core/api`):** `billing-tab.test.tsx` `shows request increase after plan_limit_reached and lists my requests`, `member without billing.manage does not see the button`; `policy-tab.test.tsx` `locks fields set by the tenant floor and shows the reason`; core: `schemas` `quota request and floor accept missing fields and unknown states`; `packages/views/locales/parity.test.ts` đỏ cho tới khi cả bảy ngôn ngữ có khóa mới.
- **Lệnh kiểm chứng:** [B]; [W] `billing-tab|policy-tab|schemas|parity`; [T] `pnpm typecheck`, `pnpm lint`, `pnpm test`.

### S38. Cổng G-R6b: hồi quy, bảo mật và tài liệu cho S16 đến S37

- **Phụ thuộc:** S16 đến S37.
- **Tiêu chí / quyết định:** AC108 đến AC143, đặc biệt AC143; D87 đến D106.
- **File đổi:** `docs/platform-operations.md` (quy trình giữ hồ sơ pháp lý, duyệt chéo và cách bật `PLATFORM_APPROVAL_ACTIONS`, vô hiệu hóa runtime, yêu cầu tăng hạn mức, xuất kiểm toán, khóa tenant); đầu hai tài liệu R6 (trạng thái). Không sửa spec và kế hoạch gốc.
- **Việc:**
  - [PRE]; [B]; [M] trên chính `multica_platform_test`; [GA]; [T]; [PW]; [PB]; [PI].
  - Chạy `TestPermissionMatrixMatchesSpec` mở rộng tới M30 đến M52 và `TestEveryRouteInEveryRouteTableHasAMatrixRow` trên bảng route cuối cùng; mọi route mới sau S0 phải có hàng ở 3.6 hoặc 3c.10.
  - Rà theo bảng bảo mật spec R6 mục 11: mỗi dòng nêu tên một test cụ thể trong PR.
  - Kiểm: không bước nào chạy `make test`, `make check`, `make migrate-up` hay tạo cơ sở dữ liệu; chuỗi giao diện đủ bảy ngôn ngữ; với mọi cấu hình mới để trống toàn bộ test hiện có xanh (AC143).
  - Bảng kiểm triển khai: chạy `server/cmd/migrate up` cho các migration R6 có trên đĩa: 938, 941 đến 946, 1016 đến 1029 và 1031 đến 1039. Đã kiểm bằng `ls server/migrations` ngày 2026-11-20. Lưu ý: 947 không có, vì cột `plan.description` nằm ở `938_plan_description` (sửa trong `e189735c5`, để chạy sau bảng `plan` ở 670); 1030 không có; chính sách tenant là 1033 và 1034, quota 1035 đến 1037, giữ hồ sơ pháp lý 1038 và 1039; 1031 và 1032 là `platform_runtime` (S21), không phải chỗ trống. Spec R6 mục 8 và mục 12 còn ghi số cũ (947, 1030, 1031 cho các bảng này), cần đồng bộ với danh sách trên; bật `PLATFORM_APPROVAL_ACTIONS` khi có từ hai `system_admin`; đặt giữ hồ sơ pháp lý trước khi xóa ngay một đối tượng đang có vụ việc.
- **Test đỏ trước:** không có test mới; slice này chạy và đối chiếu toàn bộ.
- **Lệnh kiểm chứng:** [PRE]; [B]; [GA]; [T]; [PW]; [PB]; [PI]; `git diff --check`; `git status --short` không có tệp lạ.

## 2. Phân bổ tiêu chí cho slice

| AC | Slice |
| --- | --- |
| AC62 | S7, S13 |
| AC63 | S7, S8 |
| AC64 | S7, S8 |
| AC65 | S8 |
| AC66 | S7, S8 |
| AC67 | S7, S8 |
| AC68 | S1, S4 |
| AC69 | S3, S14 |
| AC70 | S3 |
| AC71 | S2, S3 |
| AC72 | S1, S3 |
| AC73 | S1, S2 |
| AC74 | S5, S6 |
| AC75 | S5, S6 |
| AC76 | S9, S13 |
| AC77 | S9, S13 |
| AC78 | S9, S10 |
| AC79 | S9, S10 |
| AC80 | S9, S10 |
| AC81 | S9, S10 |
| AC82 | S11, S13 |
| AC83 | S12 |
| AC84 | S11, S12, S13 |
| AC85 | S11, S12 |
| AC86 | S11, S12 |
| AC87 | S12 |
| AC88 | S3, S15 |
| AC89 | S2, S3, S14, S15 |
| AC90 | S0, S1, S5, S15 |
| AC91 | S0, S13 |
| AC92 | S0, S5 |
| AC93 | S3, S5, S7 |
| AC94 | S7, S8 |
| AC95 | S9, S10 |
| AC96 | S9 |
| AC97 | S11 |
| AC98 | S0, S5 |
| AC99 | S0 |
| AC100 | S0, S5, S13 |
| AC101 | S0 |
| AC102 | S0 |
| AC103 | S0, S5, S10 |
| AC104 | S1, S5 |
| AC105 | S5, S7, S9 |
| AC106 | S5 |
| AC107 | S5 |
| AC108 | S16, S23 |
| AC109 | S16, S23 |
| AC110 | S16, S23 |
| AC111 | S16, S23 |
| AC112 | S17, S23 |
| AC113 | S17, S23 |
| AC114 | S17, S23 |
| AC115 | S17 |
| AC116 | S18, S24 |
| AC117 | S18, S24 |
| AC118 | S18, S19, S24 |
| AC119 | S18, S19, S24 |
| AC120 | S19 |
| AC121 | S18, S24 |
| AC122 | S20, S25 |
| AC123 | S20, S25 |
| AC124 | S20, S25 |
| AC125 | S21, S25 |
| AC126 | S21, S25 |
| AC127 | S21, S22, S25 |
| AC128 | S21, S22, S25 |
| AC129 | S29, S30, S36 |
| AC130 | S30, S37 |
| AC131 | S31 |
| AC132 | S31, S32, S37 |
| AC133 | S31, S36 |
| AC134 | S28 |
| AC135 | S28, S36 |
| AC136 | S26, S36 |
| AC137 | S26, S27 |
| AC138 | S27 |
| AC139 | S33, S36 |
| AC140 | S27, S33 |
| AC141 | S34, S35, S36 |
| AC142 | S34, S35, S36 |
| AC143 | S38 |

## 3. Bảng truy vết: yêu cầu, tiêu chí, slice, test

Cột "Yêu cầu" đánh số từng ý trong bốn quyết định đã chốt của chủ dự án (YC1.x là quyết định 1, YC2.x là quyết định 2, YC3.x là quyết định 3, YC4.x là quyết định 4, YC5.x là yêu cầu chung của tài liệu). Cột "Test" nêu ít nhất một test cụ thể của slice tương ứng.

| Yêu cầu | AC | Slice | Test |
| --- | --- | --- | --- |
| **YC1.1** Chỉ `system_admin` (mọi workspace) và `admin` (workspace trong tenant mình) thêm người; `support` và `billing` không | AC62, AC94 | S7, S13 | `TestMemberRequestRoutesMatchMatrixRows`; `TestMemberRequestAdminInOwnTenantSucceedsWithStaffAuditAndTenantID`; `App.test.tsx` `system-admin-only nav items are hidden for admin, support and billing` |
| **YC1.2** Thêm theo email: có thì dùng, chưa có thì tạo | AC63 | S7, S8 | `TestMemberProvisioningUsesExistingUser`; `TestMemberProvisioningCreatesOnboardedUserWithoutSessionPasswordOrToken` |
| **YC1.3** Phải còn ghế, dùng giao thức ghế 5.6 và Phụ lục B | AC65 | S8 | `TestMemberProvisioningSeatFullReleasesAndCreatesNoMember`; `TestMemberProvisioningTwoRequestsForLastSeatOneWins`; `TestStaffAddClaimIsIdempotentByRequestToken` |
| **YC1.4** Vai trò chọn khi thêm | AC64 | S7, S8 | `TestMemberRequestValidate`; `TestMemberProvisioningRolesApplied` |
| **YC1.5** Kiểm toán `actor_type = staff` và `workspace_id` | AC66 | S7, S8 | `TestMemberRequestAuditAndRowShareOneTransaction`; `TestMemberProvisioningAuditActorIsStaffWithWorkspaceID` |
| **YC1.6** Nêu vì sao 7b.5 quy tắc 2 đổi cho đường này; lời mời giữ nguyên | AC67 | S7, S8 | `TestMemberRequestWorkspaceMustBeActive`; hồi quy `invitation_test.go` và `seat_capacity_test.go`; `TestMemberRequestStaffEmailIsRefused` (đường này không thành đường vào của staff) |
| **YC2.1** Tenant nhiều workspace; `max_workspaces` trong `limits` (gốc 5.5) | AC71, AC73 | S1, S2 | `TestCatalogueMaxWorkspacesDefaults`; `TestPlanMaxWorkspacesMigrationIsIdempotentAndKeepsExistingKey`; `TestTenantMaxWorkspacesIsLargestEffectiveLimit` |
| **YC2.2** Cổng đếm trong giao dịch tạo, khóa tư vấn (tenant, cổng); 402 `plan_limit_reached`, gate `max_workspaces` | AC70 | S3 | `TestCreateWorkspaceIntoTenantRefusedAtMaxWorkspaces`; `TestConcurrentCreateAtLastTenantSlotAdmitsExactlyOne`; `TestAdmitScopesLockByTenantID` |
| **YC2.3** Tenant là thực thể bền; nêu phương án gắn thuê bao, khuyến nghị, giả định và hệ quả | AC68, AC69, AC71 | S1, S2, S3 | `TestTenantBackfillIsIdempotentAndKeysTenantByWorkspace`; `TestCreateWithoutTenantMakesTenantWithWorkspaceID`; `TestPolicyCarriesTenantGateForEverySiblingWorkspace` (phương án A, spec R6 mục 5.3, GĐ1) |
| **YC2.4** Migrate dữ liệu hiện có; thứ tự và điều kiện idempotent | AC68 | S1, S4 | `TestTenantBackfillIsIdempotentAndKeysTenantByWorkspace`; `TestBackfillTenantsIsIdempotentAndParallelSafe` |
| **YC2.5** Thay đổi 5.3, 5.5, 5.7, 5.12 và console `/tenants` | AC72, AC73, AC74, AC75 | S1, S2, S3, S5, S6 | `TestDeleteWorkspaceDetachesTenant` (5.12); `TestCatalogueMaxWorkspacesDefaults` (5.5); `TestCreateWorkspaceIntoTenantRefusedAtMaxWorkspaces` (5.7); `TestTenantStatusSummaryCountsWorkspaceStatuses` (5.3); `TestOldTenantSubroutesAreNotFound` và `Tenants.test.tsx` (console) |
| **YC2.6** Không khóa ngoại, cascade; quan hệ kiểm trong code | AC68, AC72 | S1 | `TestTenantTablesHaveNoForeignKeysOrInlineConstraints`; `TestWorkspaceTableHasNoTenantColumn` |
| **YC2.7** Chỉ mục `CONCURRENTLY`, mỗi chỉ mục một tệp | AC68 | S1, S7, S9 | `TestEveryConcurrentUpBuildHasCleanup`; `TestMigrationFilesHaveMatchingDirections` |
| **YC3.1** Xóa ngay workspace: `system_admin` và `admin` trong tenant mình; xóa ngay tenant: chỉ `system_admin`; `support` và `billing` như cũ | AC75, AC76, AC95, AC96 | S9, S13 | `TestRoleMatrix` (mở rộng cho route xóa ngay, nhánh `support` khóa và mở được còn `billing` không); `TestDeleteTenantNowIsSystemAdminOnly`; `TestDeleteWorkspaceNowAdminInOwnTenant` |
| **YC3.2** Lý do bắt buộc; xác nhận gõ lại tên hoặc slug | AC77 | S9, S13 | `TestDeleteNowReasonRequired`; `TestDeleteNowConfirmMismatchWritesNothing`; `TypeToConfirmDialog.test.tsx` |
| **YC3.3** Kiểm toán trước khi xóa, không mất khi xóa lỗi | AC78 | S9, S10 | `TestDeleteNowPhaseOneWritesAuditRequestAndStatusInOneTransaction`; `TestStaffDeletionFailureMarksFailedAfterThreeAttemptsAndKeepsPhaseOneAudit`; `TestStaffDeletionAuditRowSurvivesWithNullWorkspaceID` |
| **YC3.4** Xóa theo đường hiện có, không SQL xóa rời | AC79, AC80 | S9, S10 | `TestStaffDeletionEquivalentToDeleteWorkspace`; `TestNoDeleteStatementsOnWorkspaceContentInPlatformCode`; `TestStaffDeletionTenantDeletesEveryWorkspaceThenTenant` |
| **YC3.5** Ngoại lệ I10 nêu trong bảng quyết định (D71, không dùng D49) | AC81 | S9, S10 | `TestSubscriptionLifecycleNeverDeletesAWorkspace`; `TestDeleteNowSetsPendingDeletionDueNowAndClearsCache`; `TestWorkspaceDeletionJobSkipsWorkspacesWithLiveOrFailedStaffRequest` |
| **YC4.1** Chỉ `system_admin` xóa ngay tài khoản, không chờ 14 ngày, cùng pipeline gốc 4.6 | AC82, AC83, AC87, AC97 | S11, S12, S13 | `TestUserDeleteNowRequiresSystemAdmin`; `TestUserDeleteNowIsSystemAdminOnly`; `TestStaffAccountDeletionPipelineMatchesSelfServiceRows`; `TestAccountDeletionJobSkipsAccountsWithStaffRequest` |
| **YC4.2** Owner duy nhất của workspace còn người khác: chọn (A) chuyển quyền hoặc (B) xóa workspace ngay | AC84 | S11, S12, S13 | `TestUserDeleteNowSoleOwnerUnresolvedReturns409WithListAndWritesNothing`; `TestStaffAccountDeletionTransfersOwnershipThenDeletes`; `TestStaffAccountDeletionDeletesWorkspaceThenAccount`; `UserDetail.test.tsx` `requires a choice for each` |
| **YC4.3** Không xóa để workspace mồ côi | AC85 | S11, S12 | `TestNoWorkspaceIsOrphanedAfterAnyResolutionCombination`; `TestStaffAccountDeletionSuccessorGoneBetweenPhasesFailsAndKeepsAccount` |
| **YC4.4** Ghi kiểm toán | AC86 | S11, S12 | `TestUserDeleteNowAuditIsFirstAndHasNoEmailInDetail`; `TestStaffAccountDeletionAuditActorsAreStaffAndAnonymized` |
| **YC5.1** Mã mới đúng dải (K26, D60, AC62 trở đi); mỗi AC truy được tới mục spec và quyết định | AC62 đến AC107 | S0 đến S15 | spec R6 mục 13 (cột Mục và QĐ cho mọi AC); bảng phân bổ ở mục 2 của tài liệu này |
| **YC5.2** Hồi quy: cấu hình mới để trống; client cũ không vỡ; bảo mật và schemacheck | AC88, AC89, AC90 | S2, S3, S14, S15 | `TestAbsentMaxWorkspacesGateIsOff`; `workspace schema accepts a missing tenant_id and ignores a malformed one`; `TestEveryPlatformTableMigrationIsRequired`; `TestConsoleFilesAreScannedAndNameNoContentTable` |
| **YC6.1** Hai vai trò staff: `system_admin` (toàn nền tảng, cao nhất) và `admin` (một tenant); thứ bậc `system_admin` > `admin`; `support`, `billing` không đổi | AC91, AC98 | S0, S13 | `TestParseRoleAcceptsFourRoles`; `TestPermissionMatrixMatchesSpec`; `TestSystemAdminHasEveryAction`; `TestSupportAndBillingAccessUnchangedFromSpec52` |
| **YC6.2** `admin` chỉ thao tác trên đối tượng trong tenant của mình; đối tượng ngoài phạm vi trả 404 không lộ tồn tại | AC93, AC104, AC105 | S3, S5, S7, S9 | `TestAdminGetsSameNotFoundForForeignAndMissingObjects`; `TestAdminOfDeletedTenantHasEmptyScope`; `TestAdminScopeFollowsTenantWorkspaceLinks`; `TestMemberRequestAdminOutsideTenantIsNotFoundAndWritesNothing`; `TestDeletionRequestCancelAndRetryOnlyOwnForAdmin` |
| **YC6.3** Ma trận hành động × vai trò cho mọi hành động R6 và các hành động quản trị gốc (gồm khóa, mở, lên lịch xóa, gói và giá, kiểm toán, danh sách tenant) | AC92, AC98, AC106, AC107 | S0, S5 | `TestPermissionMatrixMatchesSpec`; `TestEveryRouteInEveryRouteTableHasAMatrixRow`; `TestAdminHasNoAccessToPaymentRoutesAccountsOverviewAndUserActions`; `TestAdminLockUnlockAndScheduleDeletionInOwnTenantOnly` |
| **YC6.4** `admin` thêm người vào workspace trong tenant mình: thành công, có kiểm toán; ngoài tenant bị từ chối | AC94 | S7, S8 | `TestMemberRequestAdminInOwnTenantSucceedsWithStaffAuditAndTenantID`; `TestMemberProvisioningAuditActorIsStaffWithWorkspaceID` |
| **YC6.5** `admin` xóa ngay workspace trong tenant mình, không xóa được tenant hay tài khoản; `system_admin` làm được mọi hành động | AC95, AC96, AC97, AC98 | S9, S10, S11 | `TestDeleteWorkspaceNowAdminInOwnTenant`; `TestDeleteTenantNowIsSystemAdminOnly`; `TestUserDeleteNowIsSystemAdminOnly`; `TestSystemAdminDeletesAnyWorkspaceAndTenant` |
| **YC6.6** Gán tenant và quản lý staff chỉ `system_admin`; `admin` phải có `tenant_id`; `POST`, `PUT`, `DELETE /staff`; kiểm toán `actor_type = staff`; `admin` không tự nâng vai trò | AC100, AC101, AC102 | S0, S5, S13 | `TestAddStaffAdminRequiresTenantID`; `TestAddStaffOtherRolesRejectTenantID`; `TestStaffTenantMustExist`; `TestAdminCannotRaiseOwnOrOthersRole`; `TestStaffHandlersRefuseNonSystemAdminWhenCalledDirectly`; `TestLastSystemAdminCannotBeDemotedOrRevokedConcurrently`; `TestStaffAuditEntriesCarryTenantID` |
| **YC6.7** Migration dữ liệu cũ: `admin` thành `system_admin` lũy đẳng; cột `tenant_id` rỗng được, không khóa ngoại; chỉ mục `CONCURRENTLY` mỗi chỉ mục một tệp, số migration mới | AC99 | S0 | `TestStaffRoleMigrationConvertsOldAdminsOnlyOnce`; `TestStaffTenantColumnIsNullableWithoutForeignKey`; `TestEveryConcurrentUpBuildHasCleanup` |
| **YC6.8** Kiểm toán có phạm vi tenant: `admin` thấy dòng thuộc tenant mình | AC103 | S0, S5, S10 | `TestAuditStoreWritesTenantID`; `TestAuditListScopedForAdminAndFullForOthers`; `TestStaffDeletionAuditRowKeepsTenantIDAfterWorkspaceIsGone` |
| **YC6.9** Không còn "staff `admin`" nghĩa toàn nền tảng ở chỗ cũ (thêm người, xóa ngay, xóa tài khoản, route, ma trận vai trò) | AC62, AC76, AC82, AC90 | S7, S9, S11, S15 | `TestMemberRequestRoutesMatchMatrixRows`; `TestDeleteNowRoutesMatchMatrixRows`; `TestUserDeleteNowRoutesMatchMatrixRows`; `TestEveryRouteInEveryRouteTableHasAMatrixRow` |
| **YC7.1** QT1: tạo gói với `key` duy nhất không đổi, mô tả, `limits` (gồm `max_workspaces`), trạng thái `active` (mục 3b.1, D87) | AC108 | S16, S23 | `TestPlanKeyIsImmutable`; `TestPlanDescriptionMaxLength`; `TestPlanDescriptionMigrationDefaultsToEmpty` |
| **YC7.2** QT1: sửa gói; đổi giá không đổi thuê bao đang chạy trừ khi có thay đổi giá có lịch rõ ràng; hạ giới hạn cần xác nhận (mục 3b.1, D88) | AC111 | S16, S23 | `TestLimitLoweringNeedsConfirmWhenSubscribed`; `TestPriceChangeDoesNotTouchPaidPeriodsOrCreatedOrders` |
| **YC7.3** QT1: bật và tắt gói; gói tắt ẩn với khách mới, thuê bao cũ giữ nguyên (mục 3b.1, D87) | AC109 | S16, S23 | `TestInactivePlanIsHiddenFromWorkspacePlansAndRefusedAtCheckout`; `TestInactivePlanKeepsExistingSubscriptionsRenewingAndBuyingSeats`; `TestDeactivateAndActivateAreIdempotent` |
| **YC7.4** QT1: xóa gói chỉ khi chưa có thuê bao, kiểm trong code (mục 3b.1, D89) | AC110 | S16, S23 | `TestPlanDeleteRefusedWithSubscriptionOrOrder`; `TestPlanDeleteAllowedWhenUnused` |
| **YC7.5** QT1: lịch sử thay đổi giá và `limits` (ai, lúc nào, cũ, mới) và kiểm toán (mục 3b.1, D88) | AC111 | S16, S23 | `TestPlanHistoryListsWhoWhenOldNew`; `TestPlanHistoryIncludesPriceChanges` |
| **YC7.6** QT1: chỉ `system_admin` ghi, `admin` chỉ xem, có test phân quyền (mục 3c.10 M30 đến M32, D89) | AC108 | S16, S23 | `TestPlanRoutesMatchMatrixRows` |
| **YC8.1** QT2: danh sách workspace tìm theo tên, slug, email owner, tenant; lọc trạng thái, gói, tenant; phân trang (mục 3b.2, D90) | AC112 | S17, S23 | `TestWorkspaceListFiltersAndPaginationAreStable`; `TestWorkspaceListSearchMatchesNameSlugOwnerEmailAndTenant` |
| **YC8.2** QT2: chi tiết workspace (tenant, gói, thuê bao, thành viên, ghế, usage, lịch sử trạng thái) (mục 3b.2, D90) | AC113 | S17, S23 | `TestWorkspaceDetailHasSeatsUsageAndStatusHistory`; `TestWorkspaceDetailSeatsUsedEqualsMemberRows` |
| **YC8.3** QT2: hành động nhanh, 409 khi trạng thái không cho phép, kiểm toán có `workspace_id` và `tenant_id`, xuất dữ liệu (mục 3b.2, D91) | AC114, AC115 | S17, S23 | `TestQuickActionStateConflictReturns409WithAllowed`; `TestQuickActionAuditRowsCarryWorkspaceAndTenantID`; `TestStaffExportInsertsOneQueuedRowWithStaffRequester` |
| **YC8.4** QT2: phân quyền `system_admin` toàn bộ, `admin` tenant mình, `support` và `billing` theo gốc (mục 3c.10 M33, M34, D90) | AC112, AC113 | S17 | `TestWorkspaceListScopeForAdmin`; `TestWorkspaceListRoutesMatchMatrixRows`; `TestWorkspaceDetailOutsideTenantIsNotFoundForAdmin` |
| **YC9.1** QT3: danh sách người dùng của tenant (định nghĩa thành viên của ít nhất một workspace), tìm, lọc, phân trang (mục 3b.3, D92) | AC116 | S18, S24 | `TestTenantUsersListDefinition`; `TestTenantUsersListFieldsAndFilters` |
| **YC9.2** QT3: chi tiết người dùng và phân bổ tài nguyên (mục 3b.3, D95) | AC117 | S18, S24 | `TestUserDetailAllocationShowsSeatsAndLimits` |
| **YC9.3** QT3: vô hiệu hóa hai phạm vi (trong tenant, toàn cục) (mục 3b.3, D93) | AC118 | S18, S19, S24 | `TestDeactivateInTenantCreatesRemoveRequestPerWorkspace`; `TestGlobalLockStillSystemAdminAndSupportOnly` |
| **YC9.4** QT3: gỡ khỏi workspace và đổi vai trò theo giao thức ghế (mục 3b.3, D94) | AC119, AC120 | S18, S19 | `TestMemberRemoveUsesRevokeAndRemoveMember`; `TestMemberRemoveReleasesSeat`; `TestMemberChangeRoleRefusesOwnerOnEitherSide` |
| **YC9.5** QT3: xóa người dùng ngay chỉ `system_admin` (mục 7, D74, D86) | AC97, AC121 | S11, S18 | `TestUserDeleteNowIsSystemAdminOnly` |
| **YC9.6** QT3: nâng hạn mức và phân bổ thêm; `admin` không tự tăng (mục 3b.3, D95) | AC121 | S18, S24 | `TestAllocationRoutesRefuseAdmin` |
| **YC9.7** QT3: `admin` không tự nâng quyền, không đổi vai trò `system_admin`, không thao tác người dùng là staff (mục 3b.3, D94) | AC119 | S18 | `TestChangeRoleRequestValidation`; `TestRemoveRequestRefusesStaffUser`; `TestTenantUsersHidesStaffFromAdmin` |
| **YC9.8** QT3: kiểm toán `actor_type = staff` với `user_id`, `tenant_id`, `workspace_id` (mục 3b.3, D94) | AC119 | S18, S19 | `TestRemoveAndRoleAuditCarryTenantWorkspaceAndUser`; `TestMemberRemoveAuditActorStaffWithTenantWorkspaceUser` |
| **YC10.1** QT4: tài nguyên của tenant theo loại, theo workspace, so với hạn mức; loại tài nguyên liệt kê từ mã (mục 3b.4, D96) | AC122 | S20, S25 | `TestTenantResourcesMatchSourceTablesOnFixture`; `TestTenantResourcesScopeForAdmin`; `TestResourceStatusThresholds` |
| **YC10.2** QT4: tài nguyên toàn hệ thống theo loại, tenant, gói; top; cảnh báo vượt (mục 3b.4, D96) | AC123 | S20, S25 | `TestSystemResourcesTopAndOverLimit`; `TestSystemResourcesSystemAdminOnly` |
| **YC10.3** QT4: hạn mức bổ sung cho tenant, `admin` chỉ xem, kiểm toán (mục 3b.4, D97) | AC124 | S20, S25 | `TestLimitOverridesWriteEveryWorkspaceInOneTransaction`; `TestLimitOverridesAuditRowPerWorkspaceWithOldNew`; `TestResourceRoutesMatchMatrixRows` |
| **YC11.1** QT5: danh sách runtime toàn hệ thống và trong tenant, riêng tư (không IP) (mục 3b.5, D98) | AC125 | S21, S25 | `TestRuntimeListScopeAndFilters`; `TestRuntimeListOmitsIPAndRawDeviceInfo` |
| **YC11.2** QT5: chi tiết runtime và khoảng trống lịch sử kết nối (mục 3b.5, D99) | AC126 | S21, S25 | `TestRuntimeDetailRecentTasksHaveNoContent`; `TestRuntimeOutsideTenantIsNotFoundForAdmin` |
| **YC11.3** QT5: vô hiệu hóa và kích hoạt lại runtime, buộc ngắt và từ chối kết nối (mục 3b.5, D98) | AC127 | S21, S22, S25 | `TestDisabledRuntimeCannotRegister`; `TestDisabledRuntimeRefusedOnEveryDaemonRoute`; `TestDisabledRuntimeGetsNoTaskClaimAndTaskStaysQueued`; `TestRuntimeStatusWritesRowAuditAndClearsCache` |
| **YC11.4** QT5: gỡ đăng ký runtime qua đường xóa có sẵn, có kiểm toán (mục 3b.5, D98) | AC128 | S21, S22, S25 | `TestStaffRuntimeUnregisterEquivalentToOwnerDelete`; `TestRuntimeUnregisterRequiresReasonAndConfirm` |
| **YC12.1** QT11 (bắt buộc): giữ hồ sơ pháp lý chặn mọi đường xóa và việc xóa nhật ký (mục 3c.7, D104) | AC136, AC137, AC138 | S26, S27 | `TestDeleteWorkspaceDataRefusesHeldWorkspace`; `TestDeleteNowRefusedWhileHeldWritesNothing`; `TestEveryWorkspaceContentDeleteStatementIsInsideDeleteWorkspaceData`; `TestAuditRetentionSkipsHeldRows` |
| **YC12.2** QT10 (nên có): duyệt chéo xóa ngay tenant và tài khoản (mục 3c.6, D103) | AC134, AC135 | S28 | `TestApproverMustDiffer`; `TestAwaitingApprovalWritesNoStateChangeOrAccessBlock`; `TestDefaultEmptyConfigKeepsDirectDeleteNow` |
| **YC12.3** QT6 (nên có): chính sách tenant kế thừa xuống workspace, workspace chỉ siết thêm (mục 3c.4, D101) | AC129, AC130 | S29, S30, S37 | `TestMergeTable`; `TestWorkspacePolicyWriteBelowFloorIs400WithFields`; `TestEffectivePolicyAppliesTenantFloorToEveryWorkspaceRegardlessOfPlan` |
| **YC12.4** QT7 (nên có): cảnh báo ngưỡng và yêu cầu tăng hạn mức do `system_admin` duyệt (mục 3c.5, D102) | AC131, AC132, AC133 | S31, S32, S37 | `TestQuotaAlertSendsOnceAt80AndOnceAt100`; `TestApproveWritesOverrideAndAuditInOneTransaction`; `TestCreateQuotaRequestByOwner` |
| **YC12.5** QT18 (nên có): xuất nhật ký kiểm toán cho staff, nhật ký không sửa và xóa (mục 3c.8, D105) | AC139, AC140 | S33, S27 | `TestExportCursorNoDuplicateNoMissAcrossPages`; `TestExportScopeAdminOwnTenantIncludingDeletedWorkspaceRows`; `TestAuditQueriesReadonlyStillPasses` |
| **YC12.6** QT19 (nên có): khóa cả tenant, chặn tạo workspace mới, báo owner, xuất trước khi xóa (mục 3c.9, D106) | AC141, AC142 | S34, S35 | `TestTenantStatusWritesAllWorkspacesInOneTransaction`; `TestCreateWorkspaceIntoSuspendedTenantIs409`; `TestDeleteNowSendsNoticeToOwnersNotTheDeletedUser`; `TestStaffDeletionWaitsForExportThenMailsSignedLink` |
| **YC12.7** Bảng đối chiếu hyperscaler, phân loại ưu tiên QT6 đến QT19, giả định có nhãn, mục ngoài phạm vi (mục 3c.2, 3c.3, 1.5, 14, D100) | AC143 | S38 | `TestPermissionMatrixMatchesSpec`; `TestEveryRouteInEveryRouteTableHasAMatrixRow` |
