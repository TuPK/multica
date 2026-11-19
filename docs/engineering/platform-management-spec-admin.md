# Platform management: đặc tả bổ sung cho admin staff và vai trò system_admin (R6)

Trạng thái: **đặc tả đã chốt, chưa có mã nào của đợt này được viết.** Bốn quyết định ban đầu và quyết định về vai trò `system_admin` (mục 1.4: D60 đến D86 là chúng và hệ quả của chúng) do chủ dự án chốt; tài liệu này đưa thẳng vào, không còn câu hỏi mở. Bản cập nhật lần hai thêm mô hình vai trò staff `system_admin` và `admin` theo tenant (mục 3.5 đến 3.8, D79 đến D86, K30, AC91 đến AC107). Bản cập nhật lần ba thêm nhóm quản trị QT1 đến QT5 (mục 3b) và đối chiếu với AWS, Azure, Google Cloud cùng nhóm QT6 đến QT19 (mục 3c): D87 đến D106, K31 đến K41, AC108 đến AC143, GĐ19 đến GĐ31. Kế hoạch từng bước: [platform-management-plan-admin.md](platform-management-plan-admin.md).

Ngày viết: 2026-11-19. Đọc mã ở **HEAD `7c9684daa`**; cây làm việc không có thay đổi chưa commit nào ngoài hai tệp tài liệu R6. Mọi trích dẫn dạng `đường/dẫn:dòng` đã được đối chiếu bằng script với HEAD này (tệp có thật, dòng chứa đúng ký hiệu nêu cạnh nó).

Tài liệu này **bổ sung** cho [platform-management-spec.md](platform-management-spec.md) (gọi là "spec gốc", mục dẫn bằng "gốc X.Y"), không thay thế và không sửa tệp đó. Mục của tài liệu này dẫn bằng số mục trần ("mục 5.4"). Chỗ nào tài liệu này ghi đè spec gốc thì nằm trong bảng 1.2, có nêu rõ mục bị đổi. Ngoài bảng đó, spec gốc giữ nguyên hiệu lực. Chỉ viết phần thêm và phần đổi; không lặp lại nguyên văn spec gốc.

Ngôn ngữ: tiếng Việt. Tên tệp, kiểu, hàm, lệnh, bảng, cột, mã lỗi, route, khóa cấu hình giữ tiếng Anh. Thuật ngữ **tenant** được định nghĩa lại ở mục 5.1 (thay định nghĩa ở dòng 9 của spec gốc).

## 1. Mục tiêu, phạm vi, bất biến, quyết định

### 1.1 Mục tiêu và phạm vi

Chủ dự án chốt bốn thay đổi cho quyền của staff và cho mô hình tenant, rồi chốt thêm mô hình vai trò staff mới (`system_admin` toàn nền tảng, `admin` theo một tenant, mục 3.5). Đợt này gọi là **R6**, chạy sau G-R5 của spec gốc:

1. **Thêm người vào workspace có sẵn** (mục 4): staff `system_admin` (mọi workspace) hoặc `admin` (workspace trong tenant của mình), theo email, có kiểm ghế, chọn vai trò thành viên, ghi kiểm toán.
2. **Tenant có nhiều workspace** (mục 5): tenant thành thực thể bền; số workspace bị giới hạn theo gói bằng khóa `max_workspaces`.
3. **Xóa ngay workspace hoặc tenant** (mục 6): workspace do `system_admin` (mọi workspace) hoặc `admin` (trong tenant của mình); tenant chỉ `system_admin`; kèm lý do và gõ lại tên, ghi kiểm toán trước khi xóa.
4. **Xóa ngay tài khoản người dùng** (mục 7): chỉ staff `system_admin`, cùng pipeline với gốc 4.6, bắt buộc xử lý workspace mà người đó là owner duy nhất.
5. **Vai trò staff `system_admin` và `admin` theo tenant** (mục 3.5 đến 3.8): `system_admin` quản trị toàn nền tảng; `admin` chỉ quản trị trong một tenant được gán; chỉ `system_admin` cấp, gán tenant, đổi vai trò và thu hồi staff.

Ngoài phạm vi của đợt này: mục 14.

**Điểm chờ người.** Theo AGENTS.md ("Automation First"), mọi việc nền chạy không cần người. Chỉ có hai chỗ đòi một người quyết định, mỗi chỗ có điều khoản yêu cầu: (a) lý do và gõ lại tên ở xóa ngay (mục 6.2, 7.2), vì quyết định 3 và 4 của chủ dự án đòi, và hành động không hoàn tác được; (b) chọn (A) hoặc (B) cho workspace có owner duy nhất (mục 7.3), vì quyết định 4 đòi người xóa "phải chọn rõ". Staff có quyền tự khởi tạo mọi yêu cầu; không có bước nào chờ một người khác. Việc cấp và gán tenant cho staff (mục 3.7) là thao tác của `system_admin`, theo quyết định D81, không phải điểm chờ của việc nền.

### 1.2 Quan hệ với spec gốc: những chỗ bị ghi đè có chủ đích

| Mục gốc | Nội dung gốc | Đổi thành | Quyết định |
| --- | --- | --- | --- |
| Dòng 9 (Thuật ngữ) | Tenant = một workspace cùng thuê bao của nó | Tenant là thực thể bền, chứa một hoặc nhiều workspace (mục 5.1) | D64 |
| 1.2, I10 | Xóa dữ liệu chỉ theo yêu cầu của chủ workspace hoặc sau thời gian chờ đã thông báo | Thêm ngoại lệ có chủ đích: staff xóa ngay, theo phạm vi ở ma trận 3.6 (workspace: `system_admin` và `admin` trong tenant của mình; tenant và tài khoản: chỉ `system_admin`) (ghi chú ở 1.3, quyết định D71) | D71 |
| 4.6 | Xóa tài khoản chỉ do người dùng tự làm, chờ 14 ngày | Thêm đường staff `system_admin` xóa ngay (mục 7); đường tự phục vụ giữ nguyên | D74, D86 |
| 5.2 (bảng vai trò và `PLATFORM_ADMIN_EMAILS`) | Ba vai trò xếp bậc `support` < `billing` < `admin`; `admin` làm tất cả; email trong `PLATFORM_ADMIN_EMAILS` luôn là `admin` | Bốn vai trò: `system_admin` (toàn nền tảng, cao nhất), `admin` (một tenant), `support` và `billing` không đổi. Quyền không còn xếp bậc cộng dồn mà theo bảng tường minh ở 3.6. Email trong `PLATFORM_ADMIN_EMAILS` được gieo thành `system_admin`. `admin` cũ chuyển thành `system_admin` (3.8) | D79, D82, D83 |
| 5.4 (route `/staff`, `/plans`) | `GET/POST/DELETE /staff` và `GET/PUT /plans` là của `admin` | Của `system_admin`; thêm `PUT /staff/{user_id}`; `POST /staff` nhận `tenant_id` (3.7) | D81, D83 |
| 7b.4, 7b.5 (route giá và `/accounts`) | Của `admin` | Của `system_admin` (`billing` vẫn đọc lịch sử giá như gốc 7b.4) | D83 |
| 5.3 | Trạng thái workspace; staff lên lịch xóa với thời gian chờ | Tenant có `status` chỉ dùng cho hành động khóa cả tenant (3c.9); trạng thái cưỡng chế vẫn ở `platform_workspace`; `pending_deletion` có thể có `deletion_scheduled_at` bằng hiện tại (mục 5.5) | D71, D73, D106 |
| 5.4 | `/tenants` là một workspace | `/tenants` là tenant; route cấp workspace chuyển sang `/workspaces/{id}/…` (mục 5.8) | D77 |
| 5.5 | `limits` không có số workspace; thuê bao theo workspace | `limits` thêm `max_workspaces`; thuê bao vẫn ở workspace (mục 5.6) | D65, D66 |
| 5.7 | Hai giới hạn cấp tài khoản và các cổng đếm tài nguyên | Thêm cổng đếm `max_workspaces` ở giao dịch tạo workspace (mục 5.7) | D66 |
| 5.12 | Xóa workspace có thời gian chờ | Thêm đường xóa ngay (mục 6); đường có thời gian chờ không đổi | D71 |
| 7b.5, quy tắc 2 (`docs/engineering/platform-management-spec.md:1664`) | "không có thêm thành viên vào workspace có sẵn" | Quy tắc này không áp cho đường thêm người của staff ở mục 4 (`system_admin` hoặc `admin` trong tenant của mình); lời mời của owner và admin workspace giữ nguyên (mục 4.2) | D61 |

### 1.3 Ghi chú bổ sung cho bất biến (gốc 1.2)

Bảng này bổ sung ghi chú cho bảng bất biến ở `docs/engineering/platform-management-spec.md:35`; tệp gốc không bị sửa.

| # | Ghi chú R6 |
| --- | --- |
| **I10** (`docs/engineering/platform-management-spec.md:48`) | **Ngoại lệ có chủ đích, D71.** I10 cấm mất dữ liệu vì lý do thương mại: quá hạn, hạ gói, vượt hạn mức. Ba đường xóa mới (workspace, tenant, tài khoản) do staff khởi tạo theo yêu cầu của chủ dự án và theo ma trận 3.6; chúng không phải lý do thương mại và không được gọi từ vòng đời thuê bao, từ việc nền `subscription_lifecycle` hay từ cổng hạn mức. Vòng đời thuê bao, hạ gói và vượt `max_workspaces` vẫn chỉ chặn tạo mới (mục 5.7), không xóa workspace nào. Ngoại lệ có ba điều kiện bắt buộc: đúng vai trò và phạm vi (workspace: `system_admin` hoặc `admin` trong tenant của mình; tenant và tài khoản: chỉ `system_admin`); lý do không rỗng cùng gõ lại tên (D72); kiểm toán ghi trước khi xóa (D73). Một test (AC81) khẳng định `subscription_lifecycle` vẫn không xóa gì |
| **I6** | Staff không đọc nội dung tenant và không là thành viên (D10). Đường thêm người ở mục 4 không được biến thành đường vào của staff: email của staff đang hoạt động bị từ chối (D63) |
| **I7** | Áp cho mọi yêu cầu mới: kiểm toán của bước tạo yêu cầu nằm trong cùng giao dịch với dòng yêu cầu (mục 3.1) |
| **I3** | Mọi bảng và chỉ mục của R6 theo quy tắc không khóa ngoại, không cascade, chỉ mục `CONCURRENTLY` mỗi chỉ mục một tệp (mục 8) |

### 1.4 Quyết định

Cột "Trạng thái": **Chủ dự án chốt** là một trong bốn quyết định ban đầu; **Suy ra** là hệ quả trực tiếp của quyết định đó, có tên quyết định nguồn; **Giả định GĐn** là chỗ tôi chọn khi chủ dự án chưa nói, ghi ở mục 1.5 cùng hệ quả nếu sai.

| # | Nội dung | Trạng thái |
| --- | --- | --- |
| D60 | **Chỉ staff `system_admin` và `admin` được thêm người vào một workspace (tenant) có sẵn:** `system_admin` vào mọi workspace, `admin` chỉ vào workspace thuộc tenant của mình (D80). Thêm theo email: tài khoản đã có thì dùng, chưa có thì tạo mới. Staff `support` và `billing` không thêm được. Việc thêm là yêu cầu bất đồng bộ `platform_member_request` (cùng khuôn D47) | Chủ dự án chốt (quyết định 1; phạm vi theo D79) |
| D61 | **Đổi 7b.5 quy tắc 2 cho đường thêm người của staff.** Lý do: quy tắc 2 ra đời để `POST /accounts` không biến thành đường nhét người vào workspace của khách, nên cấm thêm thành viên vào workspace có sẵn trên route đó. Nay chủ dự án muốn nhà vận hành tự thêm người vào tenant có sẵn khi khách nhờ, qua một route riêng chỉ cho `system_admin` và `admin` (trong tenant của mình) có MFA, có kiểm ghế và kiểm toán mà chủ workspace đọc được. `POST /accounts` vẫn chỉ tạo người dùng và workspace mới. Lời mời của owner và admin workspace giữ nguyên quy tắc cũ (gốc 5.6) | Chủ dự án chốt (quyết định 1) |
| D62 | Thêm người dùng giao thức ghế gốc 5.6 và Phụ lục B với `token` bằng `id` của yêu cầu: `ClaimShareJoin(ws, token)` rồi tạo `member` rồi `Confirm(ws, token, memberID)`; thất bại thì `Release(ws, token)`. Dòng `platform_member_request` là bản ghi bền của ý định (không dùng `seat_capacity_outbox`), vì thao tác của platform lũy đẳng theo `token` nên chạy lại an toàn | Suy ra từ D60; Giả định GĐ7 |
| D63 | Vai trò **thành viên** được chọn khi thêm là `admin`, `member` hoặc `viewer`; không có `owner`. Email của staff đang hoạt động bị từ chối (409 `staff_cannot_be_added`), để giữ I6 và D10 | Suy ra từ D60; Giả định GĐ2 |
| D64 | **Tenant là thực thể bền**, bảng `tenant` và bảng liên kết `tenant_workspace`; không thêm cột vào `workspace` (giữ D6). Không khóa ngoại, không cascade; quan hệ kiểm trong code | Chủ dự án chốt (quyết định 2) |
| D65 | **Thuê bao vẫn gắn ở từng workspace** (phương án A, mục 5.6). Giới hạn `max_workspaces` của một tenant là giá trị lớn nhất trong các giới hạn hiệu lực của thuê bao các workspace thuộc tenant; giá trị không giới hạn thắng | Giả định GĐ1 (chủ dự án yêu cầu nêu phương án và khuyến nghị) |
| D66 | **`max_workspaces` là khóa trong `limits` của gói** (gốc 5.5). Cổng đếm chạy trong giao dịch tạo workspace vào một tenant có sẵn, giữ khóa tư vấn theo cặp (tenant, cổng). Vượt thì 402, mã `plan_limit_reached`, `gate = max_workspaces` | Chủ dự án chốt (quyết định 2) |
| D67 | `POST /api/workspaces` nhận thêm `tenant_id` tùy chọn. Không có thì tạo tenant mới cho workspace (hành vi hiện tại, I2). Có thì người gọi phải là `owner` của ít nhất một workspace thuộc tenant đó | Suy ra từ D64; Giả định GĐ4 |
| D68 | Tenant không còn workspace nào thì bị xóa, trong cùng giao dịch xóa workspace cuối cùng. Mọi thao tác thêm hoặc bớt workspace của một tenant đều giữ khóa tư vấn (tenant, `max_workspaces`) | Suy ra từ D64 |
| D69 | **Migrate dữ liệu hiện có:** mỗi workspace hiện có thành một tenant chỉ có một workspace, với `tenant.id` bằng `workspace.id`. Idempotent theo khóa tự nhiên; chạy lại bởi lệnh `platform-backfill` sau khi triển khai mã mới (mục 5.3) | Chủ dự án chốt (quyết định 2); Giả định GĐ5 |
| D70 | Giá trị mặc định của `max_workspaces`: `free` 1, `pro` 3, `business` 10, `payg` 5, `enterprise` và `unlimited` không giới hạn. Khóa vắng mặt trong `limits` nghĩa là không giới hạn (quy ước gốc 5.5) | Giả định GĐ6 |
| D71 | **Ngoại lệ có chủ đích của I10.** Chỉ staff `system_admin` (mọi workspace, mọi tenant) và `admin` (chỉ workspace trong tenant của mình; không xóa tenant, D85) được xóa ngay, không chờ 14 ngày. `support` và `billing` không xóa ngay; về khóa, mở và lên lịch xóa họ giữ đúng quyền của gốc 5.2 (`support` khóa và mở; lên lịch xóa chỉ `system_admin` và `admin`). Ngoại lệ này không mở rộng sang vòng đời thuê bao (ghi chú I10 ở 1.3) | Chủ dự án chốt (quyết định 3; phạm vi theo D79) |
| D72 | Xóa ngay đòi **lý do không rỗng** và **gõ lại**: slug cho workspace, tên cho tenant, email cho tài khoản. Máy chủ kiểm cả hai; client chỉ hiển thị | Chủ dự án chốt (quyết định 3, 4); Giả định GĐ10 |
| D73 | **Hai pha, kiểm toán trước.** Pha 1 là một giao dịch của platform ghi kiểm toán, dòng `platform_deletion_request` và đặt `pending_deletion` với `deletion_scheduled_at` bằng hiện tại. Pha 2 là việc nền `staff_deletion` của Multica gọi **đúng đường xóa hiện có** (`deleteWorkspaceData`, `DeleteAccount`), không có SQL xóa rời. Xóa lỗi thì dòng yêu cầu `failed`, kiểm toán của pha 1 còn nguyên | Chủ dự án chốt (quyết định 3, 4); Giả định GĐ8 |
| D74 | **Staff `system_admin` xóa ngay tài khoản người dùng**, không chờ 14 ngày, qua cùng pipeline ẩn danh hóa và gỡ thành viên của gốc 4.6. Staff `admin` không có quyền này (D86) | Chủ dự án chốt (quyết định 4; phạm vi theo D79 và GĐ15) |
| D75 | Người là owner duy nhất của một workspace còn thành viên khác: người tạo yêu cầu xóa (luôn là `system_admin`, D74) **phải** chọn cho từng workspace (A) chuyển quyền sở hữu cho một thành viên khác, hoặc (B) xóa workspace đó ngay (theo D71; `system_admin` có quyền xóa mọi workspace). Không chọn thì 409 `sole_owner_unresolved`. Chốt cuối là kiểm tra có sẵn trong `removeMemberForAccountDeletion` (`server/internal/handler/me_deletion.go:474`), nên workspace không bao giờ mồ côi | Chủ dự án chốt (quyết định 4); Giả định GĐ11 |
| D76 | Không xóa ngay tài khoản của staff đang hoạt động (mọi vai trò staff) và không cho `system_admin` xóa chính mình | Giả định GĐ9 |
| D77 | **Console `/tenants` theo tenant thật.** `GET /tenants` và `GET /tenants/{id}` trả tenant kèm danh sách workspace; với `admin` cùng route nhưng chỉ trả tenant của mình (D80). Các route cấp workspace (`status`, `usage`, `audit-events`, `notes`, `subscription`, `billing-profile`) chuyển thành `/workspaces/{id}/…`, không để bí danh (AGENTS.md: không thêm lớp tương thích tạm; console và platform triển khai cùng một image) | Suy ra từ D64 |
| D78 | Bốn khóa route mới trong bảng quyền của `platform-management/internal/staff/role.go:19` (`routeRole`, được thay bằng bảng quyền tường minh ở 3.6): `members.add` (`system_admin`, `admin` trong phạm vi), `workspaces.delete_now` (`system_admin`, `admin` trong phạm vi), `tenants.delete_now` (chỉ `system_admin`), `accounts.delete_now` (chỉ `system_admin`) | Suy ra từ D60, D71, D74, D79 |
| D79 | **Hai vai trò staff mới.** `system_admin`: phạm vi toàn nền tảng, mọi tenant, mọi người dùng, mọi thao tác ở ma trận 3.6; cao nhất. `admin`: phạm vi **một** tenant, chỉ thao tác trên đối tượng thuộc tenant mình được gán. Thứ bậc `system_admin` > `admin`: `system_admin` có mọi quyền của `admin` mà không bị giới hạn tenant. `support` và `billing` giữ nguyên như gốc 5.2. Tên `admin` ở đây là vai trò staff, không phải vai trò thành viên `admin` của một workspace (mục 3.5) | Chủ dự án chốt (yêu cầu vai trò `system_admin`) |
| D80 | **Phạm vi tenant của `admin`.** Staff `admin` mang đúng một `tenant_id`. Đối tượng "trong phạm vi" được định nghĩa ở 3.5; đối tượng ngoài phạm vi trả 404 giống hệt đối tượng không tồn tại; danh sách bị lọc im lặng. Phạm vi đọc từ cơ sở dữ liệu ở mỗi request (không nằm trong cookie), nên đổi tenant hay vai trò có hiệu lực ở request kế tiếp | Suy ra từ D79 |
| D81 | **Gán tenant và quản lý staff chỉ do `system_admin`.** Staff có vai trò `admin` phải gắn đúng một `tenant_id`; `system_admin`, `support`, `billing` có `tenant_id` rỗng. Chỉ `system_admin` tạo, gán tenant, đổi vai trò, thu hồi staff (`GET` và `POST /staff`, `PUT` và `DELETE /staff/{user_id}`). `admin` không nâng được vai trò của chính mình hay của ai. Mọi thay đổi ghi kiểm toán `actor_type = staff`. Không thu hồi hay hạ bậc `system_admin` cuối cùng (mục 3.7) | Chủ dự án chốt (quyết định gán tenant cho `admin`) |
| D82 | **Migration vai trò cũ.** Mọi tài khoản staff có vai trò `admin` hiện có chuyển thành `system_admin` (`tenant_id` rỗng); email trong `PLATFORM_ADMIN_EMAILS` được gieo thành `system_admin`. Thêm cột `platform_staff.tenant_id` (rỗng được, không khóa ngoại) và chỉ mục tạo `CONCURRENTLY`, mỗi chỉ mục một tệp, số migration mới (mục 3.8, 8) | Giả định GĐ16 |
| D83 | **Ma trận hành động × vai trò (3.6) là nguồn duy nhất của phân quyền staff.** Bảng quyền ở `platform-management/internal/staff/` và danh sách vai trò của từng bảng route (`console/handler.go`, `subscription/console.go`, `pricechange/handler.go`, `billing/reconcile.go`, `console/orders.go`) đều phải sinh ra từ cùng bảng, kèm một test bảng so với 3.6. `admin` không có quyền thanh toán, tạo tài khoản, thao tác cấp tài khoản, tổng quan toàn hệ thống (GĐ18) | Suy ra từ D79; Giả định GĐ18 |
| D84 | **Kiểm toán có phạm vi tenant.** Bảng `audit_event` thêm cột `tenant_id` (rỗng được, không khóa ngoại); các dòng kiểm toán R6 và của staff ghi `tenant_id`. `admin` đọc được dòng có `tenant_id` hoặc `workspace_id` thuộc tenant mình; `system_admin`, `support`, `billing` đọc toàn bộ như gốc 5.4 | Giả định GĐ17 |
| D85 | `admin` không xóa ngay được tenant, kể cả tenant của mình; chỉ `system_admin` | Giả định GĐ14 |
| D86 | `admin` không xóa ngay được tài khoản người dùng, kể cả người dùng chỉ thuộc tenant của mình; chỉ `system_admin` | Giả định GĐ15 |
| D87 | **Bật và tắt gói bằng route riêng.** `POST /plans/{key}/deactivate` và `/activate` đặt hoặc xóa `plan.archived_at`. Gói `inactive` ẩn với khách mới và không nhận đơn `subscribe` hay `change_plan` mới (409 `plan_inactive`); thuê bao đang dùng giữ nguyên gói, gia hạn và giới hạn. `PUT` với `archived` là bí danh | Suy ra từ yêu cầu quản trị gói (3b.1) |
| D88 | **Sửa gói có mô tả, lịch sử và xác nhận khi hạ giới hạn.** Thêm cột `plan.description`; `GET /plans/{key}/history` đọc từ kiểm toán; hạ giới hạn của gói đang có thuê bao đòi `confirm_affects_subscribers`. Đổi giá giữ quy tắc gốc 7b.4 (không đổi đơn đã tạo hay kỳ đã trả; thuê bao hiện hữu chịu giá mới ở kỳ gia hạn sau `effective_from`, có báo trước); muốn giữ giá cho thuê bao cũ thì tạo gói mới | Suy ra từ yêu cầu; Giả định GĐ28 |
| D89 | **Quyền quản trị gói.** Ghi (tạo, sửa, bật, tắt, xóa, lập giá) chỉ `system_admin`; `admin` chỉ đọc danh mục; `billing` đọc lịch sử. Xóa gói chỉ khi chưa có thuê bao hay đơn (kiểm trong code, không khóa ngoại); ngược lại chỉ tắt | Chủ dự án chốt (yêu cầu quản trị gói) |
| D90 | **Danh sách workspace toàn hệ thống** `GET /workspaces` với tìm kiếm, lọc theo trạng thái, gói, tenant và phân trang theo khóa; chi tiết thêm tenant, thuê bao, ghế, mức dùng và lịch sử trạng thái | Chủ dự án chốt (yêu cầu quản trị workspace) |
| D91 | **Hành động nhanh trên workspace dùng lại route đã có**, chỉ thêm yêu cầu xuất dữ liệu bởi staff (`POST /workspaces/{id}/export`). Trạng thái không cho phép trả 409 `workspace_state_conflict`. Mọi hành động ghi `workspace_id` và `tenant_id` | Suy ra từ D90 |
| D92 | **Người dùng thuộc tenant** khi là thành viên của ít nhất một workspace trong tenant; `GET /tenants/{id}/users` và chi tiết, không bảng "thành viên tenant" | Giả định GĐ19 |
| D93 | **Vô hiệu hóa người dùng có hai phạm vi.** Trong tenant (gỡ khỏi mọi workspace của tenant): `system_admin`, `admin`. Toàn cục (khóa tài khoản, gốc 4.1): `system_admin`, `support`. `admin` không khóa toàn cục | Giả định GĐ20 |
| D94 | **Gỡ thành viên và đổi vai trò thành viên bởi staff** là yêu cầu bất đồng bộ trên cùng bảng `platform_member_request` với cột `kind` (`add`, `remove`, `change_role`), việc nền `member_provisioning` xử lý cả ba; dùng `revokeAndRemoveMember` và `UpdateMemberRole`; owner bất biến qua đường này | Suy ra từ D60 |
| D95 | **Không có tài nguyên hay hạn mức riêng theo người dùng** trong mã; phân bổ thêm cho người dùng là thao tác ở workspace của người đó (ghế, `limit_overrides`) hoặc yêu cầu tăng hạn mức (QT7). `admin` không tự tăng | Giả định GĐ21 |
| D96 | **Tổng hợp tài nguyên** theo tenant (`GET /tenants/{id}/resources`) và toàn hệ thống (`GET /resources`, chỉ `system_admin`) đọc từ `platform_usage_daily` và bộ đếm hiện tại; không bảng mới; so với hạn mức hiệu lực | Chủ dự án chốt (yêu cầu quản trị tài nguyên) |
| D97 | **Hạn mức bổ sung cho tenant** (`POST /tenants/{id}/limit-overrides`) ghi `limit_overrides` của từng workspace trong một giao dịch, một dòng kiểm toán mỗi workspace; `system_admin` và `billing`; `admin` chỉ xem | Giả định GĐ23 |
| D98 | **Vô hiệu hóa runtime** bằng bảng `platform_runtime` mà Multica đọc ở `DaemonRegister`, `requireDaemonRuntimeAccess` và `claimRefusalFor`; gỡ đăng ký là yêu cầu `kind = runtime` qua `staff_deletion`. `system_admin` mọi runtime, `admin` trong tenant mình | Chủ dự án chốt (yêu cầu quản trị runtime) |
| D99 | **Không lưu lịch sử kết nối**; runtime chỉ có trạng thái hiện tại, `last_seen_at`, task gần nhất và kiểm toán staff; không lưu và không hiện IP | Giả định GĐ22 |
| D100 | **Phân loại QT6 đến QT19** theo mức ưu tiên (3c.3) và bảng đối chiếu hyperscaler (3c.2): bắt buộc QT11; nên có QT6, QT7, QT10, QT18, QT19; sau này QT8, QT9, QT12 đến QT17 | Giả định GĐ26 |
| D101 | **Chính sách tenant kế thừa xuống workspace** làm sàn; workspace chỉ siết thêm; hợp hai chính sách theo `workspacepolicy.Merge`; áp cho mọi workspace của tenant | Suy ra từ D64; Giả định GĐ24 |
| D102 | **Cảnh báo ngưỡng 80 và 100 phần trăm** hạn mức đếm theo workspace và **yêu cầu tăng hạn mức** do `system_admin` duyệt thủ công; không tự duyệt | Suy ra từ D97 và D95; Giả định GĐ31 |
| D103 | **Duyệt chéo** cho xóa ngay tenant và tài khoản khi `PLATFORM_APPROVAL_ACTIONS` có tên hành động; người duyệt là `system_admin` khác người yêu cầu; mặc định tắt | Giả định GĐ25 |
| D104 | **Giữ hồ sơ pháp lý** (`legal_hold`) trên tenant, workspace hoặc tài khoản chặn mọi đường xóa (staff, owner, việc nền) và việc xóa nhật ký theo thời hạn, đến khi `system_admin` gỡ. Là điều kiện đi kèm D71 và D74 | Giả định GĐ30 |
| D105 | **Xuất nhật ký kiểm toán cho staff** (`GET /audit-events/export`): `system_admin` toàn bộ, `admin` tenant mình; tối đa 31 ngày và 100.000 dòng mỗi trang; mỗi lần xuất ghi kiểm toán; không thêm truy vấn sửa hay xóa | Giả định GĐ29 |
| D106 | **Vòng đời tenant:** khóa cả tenant (`tenant.status`), chặn tạo workspace vào tenant không `active` (`tenant_not_active`), báo owner khi xóa ngay và tùy chọn xuất dữ liệu trước (`export_first`) | Giả định GĐ27 |

### 1.5 Giả định có nhãn

Mỗi giả định chỉ quyết định một giá trị hoặc một quy tắc đã tách riêng, nên đổi được mà không viết lại phần còn lại.

| # | Giả định | Hệ quả nếu sai |
| --- | --- | --- |
| GĐ1 | Thuê bao ở từng workspace; giới hạn tenant lấy lớn nhất giữa các workspace (D65, mục 5.6) | Nếu chủ dự án muốn trả tiền một lần cho cả tenant, phải chuyển sang phương án B (mục 5.6). Vì mọi tenant bắt đầu với một workspace, việc chuyển không mất dữ liệu, nhưng tenant đã có nhiều workspace với gói khác nhau cần một quyết định gộp thuê bao thủ công |
| GĐ2 | Thêm người (cả `system_admin` lẫn `admin`) không gán được vai trò thành viên `owner`, và không thêm được email của bất kỳ staff đang hoạt động nào (mọi vai trò staff) (D63) | Nếu cần staff làm thành viên để hỗ trợ, khách tự mời họ theo D10 (đường lời mời hiện có). Nếu cần thêm `owner`, thêm một giá trị vào danh sách vai trò thành viên hợp lệ của route, có thể chỉ cho `system_admin`; không đổi cấu trúc |
| GĐ3 | Cổng `max_workspaces` đếm mọi workspace còn tồn tại của tenant, kể cả `suspended`, `read_only`, `pending_deletion` | Nếu muốn không đếm workspace `pending_deletion`, owner sẽ tạo thêm được workspace trong lúc 14 ngày chờ; đổi truy vấn đếm, không đổi lược đồ |
| GĐ4 | Quyền thêm workspace vào tenant có sẵn là: người gọi là `owner` của ít nhất một workspace thuộc tenant (D67). Tenant không có cột chủ sở hữu | Nếu cần người "chủ tenant" riêng, thêm cột `owner_user_id` vào `tenant` và một quy tắc chuyển giao; xóa tài khoản (mục 7) sẽ phải xử lý thêm một loại quyền sở hữu |
| GĐ5 | Khi migrate, `tenant.id = workspace.id` và `tenant.name = workspace.name` tại thời điểm đó; đổi tên workspace sau đó không đổi tên tenant | Nếu muốn tên tenant theo sát tên workspace đầu tiên, thêm bước đồng bộ ở `UpdateWorkspace`; không ảnh hưởng dữ liệu |
| GĐ6 | Giá trị mặc định `max_workspaces`: 1, 3, 10, 5, không giới hạn, không giới hạn (D70) | Là con số thương mại chưa đo. Staff `system_admin` đổi bằng `PUT /plans`, `system_admin` hoặc `billing` đổi `limit_overrides`; không cần mã |
| GĐ7 | Dòng `platform_member_request` đủ làm bản ghi bền của ý định ghế, không cần dòng `seat_capacity_outbox` (D62) | Nếu tiến trình chết giữa `ClaimShareJoin` và `Confirm`, việc nền chạy lại cùng `token` và platform trả cùng kết quả; chỗ hở duy nhất là yêu cầu bị hủy giữa chừng, được xử lý bằng `Release` ở nhánh hủy (mục 4.2, quy tắc 8). Nếu sai, một chỗ ghế có thể giữ lâu hơn cần thiết cho đến khi staff hủy yêu cầu |
| GĐ8 | "Xóa ngay" nghĩa là truy cập bị chặn ngay khi pha 1 commit, còn dữ liệu bị gỡ khi việc nền `staff_deletion` chạy (nhịp một phút) và hoàn tất | Nếu cần dữ liệu biến mất đồng bộ trong request, phải cho platform gọi HTTP vào Multica, trái D35 và D47 |
| GĐ9 | Không xóa ngay tài khoản staff đang hoạt động (mọi vai trò staff) hay chính `system_admin` đang thao tác (D76) | Nếu cần, `system_admin` gỡ vai trò staff trước (`DELETE /staff/{user_id}`, mục 3.7) rồi xóa; không đổi mã |
| GĐ10 | So khớp xác nhận: slug và tên cắt khoảng trắng hai đầu rồi so khớp chính xác, phân biệt hoa thường; email so khớp không phân biệt hoa thường | Nếu quá chặt với tên có dấu, đổi hàm so khớp ở `platform-management/internal/console/validate.go`; không đổi API |
| GĐ11 | Người nhận quyền sở hữu ở lựa chọn (A) phải là thành viên hiện có của workspace, vai trò **thành viên** `admin` hoặc `member` (không `viewer`; đây là vai trò trong workspace, không phải vai trò staff), không phải khách, tài khoản đang `active` | Nếu muốn cho `viewer` nhận quyền, nới danh sách vai trò hợp lệ; việc nâng vai trò đi qua `UpdateMemberRole` như `UpdateMember` hiện nay (`server/internal/handler/workspace.go:923`) |
| GĐ12 | Người dùng mới tạo bởi mục 4 có tên bằng phần trước `@` của email, đúng như `findOrCreateUser` (`server/internal/handler/auth.go:243`); không có trường `display_name` ở API | Người dùng đổi tên được sau khi đăng nhập; nếu cần đặt tên khi thêm, thêm trường tùy chọn vào yêu cầu |
| GĐ13 | Không gửi email thông báo cho người được thêm; chủ workspace thấy qua kiểm toán và sự kiện `member:added` | Nếu pháp lý đòi báo cho người được thêm, thêm một thư theo khuôn thư chào ở gốc 7b.5 quy tắc 9 |
| GĐ14 (GĐ-mới-A) | `admin` không xóa ngay được tenant, kể cả tenant của mình; chỉ `system_admin` (D85). Lý do: xóa tenant xóa mọi workspace của nó và đổi số workspace tính cho `max_workspaces`, ảnh hưởng nhiều workspace cùng lúc | Nếu sai: thêm quyền `tenants.delete_now` cho `admin` kèm kiểm `tenant_id` của route bằng tenant của người gọi (một dòng trong ma trận 3.6 và một nhánh trong bộ kiểm phạm vi); `admin` vẫn xóa được từng workspace trong tenant mình theo D71 nên chỉ khác ở bước dọn dòng `tenant` |
| GĐ15 (GĐ-mới-B) | `admin` không xóa ngay được tài khoản người dùng, kể cả người chỉ thuộc tenant của mình; chỉ `system_admin` (D86). Lý do: một tài khoản có thể là thành viên của workspace ở nhiều tenant, và xóa tài khoản đổi cả dữ liệu ngoài tenant của `admin` | Nếu sai: phải định nghĩa quy tắc "người dùng chỉ thuộc tenant của `admin`" (mọi workspace mà người đó là thành viên đều trong tenant của `admin`, và không phải staff) và kiểm lại ở pha 1 và pha 2 trước khi mở quyền; sau đó thêm hàng cho `admin` ở ma trận |
| GĐ16 (GĐ-mới-C) | Mọi staff có vai trò `admin` hiện có chuyển thành `system_admin` (D82). Lý do: giữ nguyên quyền đang có, không để nền tảng mất người quản trị; vai trò `admin` mới bắt buộc có tenant nên không thể nhận hàng loạt từ dữ liệu cũ | Nếu sai: chủ dự án hạ từng tài khoản xuống `admin` bằng `PUT /staff/{user_id}` kèm `tenant_id`. Không có dữ liệu nào mất; chỉ có một khoảng thời gian tài khoản có quyền rộng hơn dự định |
| GĐ17 (GĐ-mới-D) | `admin` thấy kiểm toán của tenant mình qua `workspace_id` hoặc `tenant_id`. Các phương án: **(1)** thêm cột `audit_event.tenant_id`, chỉ dòng R6 và dòng staff ghi cột này, đọc bằng `tenant_id = T` hoặc `workspace_id` thuộc T; **(2)** chỉ suy từ `workspace_id` cộng `target_type = 'tenant'`; **(3)** đọc `detail->>'tenant_id'`. Khuyến nghị (1). Lý do: (2) làm dòng xóa workspace biến mất khỏi tầm nhìn của `admin` ngay khi workspace bị xóa vì `DetachWorkspaceAuditEvents` đặt `workspace_id` thành NULL (`server/pkg/db/queries/workspace_delete.sql:777`), trong khi `admin` cần thấy ai đã xóa workspace của tenant mình; (3) quét JSONB không có chỉ mục | Nếu sai: `admin` thấy quá ít (dòng cũ chưa có `tenant_id` chỉ hiện khi workspace còn) hoặc quá nhiều (dòng ghi sai `tenant_id`). Sửa bằng truy vấn đọc và test AC103; không đổi lược đồ ngoài một cột rỗng được |
| GĐ18 (GĐ-mới-E) | `admin` không có quyền thanh toán (xác nhận hay hủy đơn, đổi gói, ghế, gia hạn, xóa nợ, xác minh hồ sơ), không tạo tài khoản cho khách (`/accounts`, tạo tenant mới ngoài phạm vi), không khóa hay mở tài khoản người dùng, buộc đăng xuất, đặt lại MFA (đối tượng cấp tài khoản, xuyên tenant), không xem thông báo thanh toán và hóa đơn điện tử, không xem `/overview` toàn hệ thống. `admin` chỉ đọc đơn hàng của tenant mình | Nếu sai: thêm hàng cho `admin` ở ma trận 3.6 kèm phạm vi tenant; mỗi hàng một dòng bảng quyền và một nhánh test; không đổi lược đồ |
| GĐ19 | Người dùng thuộc tenant khi là thành viên của ít nhất một workspace trong tenant (D92). Người dùng là staff đang hoạt động không hiện với `admin` | Nếu cần người dùng "của tenant" độc lập với workspace (ví dụ người được mời nhưng chưa vào workspace nào), phải thêm bảng thành viên tenant và đổi định nghĩa phạm vi 3.5; người chưa là thành viên workspace nào sẽ không hiện ở danh sách tenant |
| GĐ20 | Vô hiệu hóa trong tenant là gỡ khỏi mọi workspace của tenant; vô hiệu hóa toàn cục là khóa tài khoản (D93). Khuyến nghị này thay cho một nút khóa toàn cục dùng chung | Nếu chủ dự án muốn `admin` khóa toàn cục, phải có quy tắc "người dùng chỉ thuộc tenant của `admin`" như GĐ15; nếu muốn vô hiệu hóa trong tenant không mất tư cách thành viên, phải thêm cờ chặn đọc ở `requireWorkspaceMember` (mọi route), thay đổi lớn |
| GĐ21 | Mã không có hạn mức hay tài nguyên theo người dùng; phân bổ thêm làm ở workspace (D95) | Nếu cần hạn mức theo người dùng (ví dụ số agent mỗi người), phải thêm bảng và kiểm ở mọi cổng đếm; đến lúc đó nút "phân bổ" trên dòng người dùng ghi vào bảng mới thay vì `limit_overrides` |
| GĐ22 | Không lưu lịch sử kết nối và IP của runtime; chi tiết dùng `last_seen_at`, task gần nhất và kiểm toán staff (D99). Phương án còn lại: một bảng `runtime_status_event` ghi ở mỗi lần đăng ký, hủy đăng ký và chuyển offline, giữ 30 ngày | Nếu cần lịch sử thật, thêm bảng sự kiện với một lần ghi ở `DaemonRegister`, `DaemonDeregister` và việc quét offline; chi phí là vài dòng mỗi chuyển trạng thái (không mỗi nhịp tim) và một điểm móc thêm ở lõi; IP cần thêm cột và có hệ quả riêng tư (thông tin cá nhân) |
| GĐ23 | Hạn mức bổ sung "cho tenant" là ghi hàng loạt `limit_overrides` của từng workspace; không có hàng "hạn mức tenant" riêng ngoài `max_workspaces`; tổng hợp không có bảng tổng hợp theo tenant (D96, D97) | Nếu truy vấn tổng hợp chậm hoặc cần hạn mức dùng chung của cả tenant, thêm bảng tổng hợp theo tenant do việc nền ghi và một hạn mức cấp tenant; đổi cổng đếm sang đếm theo tenant |
| GĐ24 | Chính sách tenant áp cho mọi workspace của tenant bất kể gói của workspace, và việc ghi đòi ít nhất một workspace có tính năng `workspace_policies` (D101) | Nếu chỉ áp cho workspace có tính năng, workspace gói thấp thoát sàn; nếu không đòi tính năng khi ghi, gói thấp có chính sách bảo mật miễn phí; sửa ở `EffectivePolicy` và ở kiểm tra ghi |
| GĐ25 | Duyệt chéo mặc định tắt (`PLATFORM_APPROVAL_ACTIONS` rỗng), chỉ áp xóa ngay tenant và tài khoản, và đòi từ hai `system_admin` (D103) | Nếu tắt, một `system_admin` bị chiếm vẫn xóa ngay được tenant và tài khoản; nếu bật khi chỉ có một `system_admin`, các hành động bị từ chối `approver_unavailable`; thêm `workspace.delete_now` vào danh sách nếu muốn chặt hơn |
| GĐ26 | Xếp QT8, QT9, QT12 đến QT17 vào "sau này" và QT11 vào "bắt buộc" (D100) | Nếu mạo danh (QT9) là cần ngay cho `support`, phải có quyết định riêng đảo I6 và D10 và làm QT10, QT11, QT18 trước; đưa QT9 lên "nên có" sẽ thêm khoảng một đợt ngang QT19 |
| GĐ27 | Hết hạn thanh toán không áp ở cấp tenant vì thuê bao ở từng workspace; vẫn theo vòng đời từng workspace (3c.9) | Nếu thuê bao chuyển lên tenant (phương án B của 5.3), thêm bước chuyển trạng thái tenant ở `subscription_lifecycle` |
| GĐ28 | Đổi giá giữ quy tắc gốc 7b.4: thuê bao hiện hữu chịu giá mới từ kỳ gia hạn sau `effective_from` kèm báo trước; giữ giá cho thuê bao cũ làm bằng gói mới (D88) | Nếu chủ dự án muốn mặc định thuê bao cũ giữ giá, phải thêm cột giá khóa trên thuê bao và cờ `existing_subscribers` ở `plan_price_change`, đổi quy tắc 9 của 7b.4 |
| GĐ29 | Xuất kiểm toán tối đa 31 ngày mỗi lần gọi và 100.000 dòng mỗi trang, truyền dòng (D105) | Giới hạn quá thấp thì cần nhiều trang; quá cao thì tải cơ sở dữ liệu; đổi hằng số, không đổi API |
| GĐ30 | `admin` chỉ biết đối tượng đang bị giữ qua lỗi `legal_hold`, không thấy lý do và mã vụ việc (D104) | Nếu `admin` cần thấy lý do, thêm trường vào lỗi và vào `GET /legal-holds` cho tenant mình; rủi ro lộ vụ việc pháp lý |
| GĐ31 | Cảnh báo hạn mức ở 80 và 100 phần trăm, nhịp mỗi giờ, gửi cho owner và admin workspace (D102) | Ngưỡng khác hay nhịp khác chỉ đổi hằng số; gửi cho người khác đòi thêm người nhận |

## 2. Hiện trạng và khoảng trống

### 2.1 Cái đã có và dùng lại

| Thành phần | Vị trí | Dùng cho |
| --- | --- | --- |
| Tạo workspace giao dịch một lần | `server/internal/workspaceprovision/provision.go:40` (`Create`), gọi ở `server/internal/handler/workspace.go:430` | Mục 5.9: thêm tạo tenant vào đúng hàm này, nên `CreateWorkspace` và việc nền `account_provisioning` cùng được hưởng |
| Cổng đếm có khóa tư vấn | `server/internal/planlimit/planlimit.go:101` (`Admit`), `server/pkg/db/queries/plan_limit.sql:1` (`LockPlanGate`, tham số `Scope` là chuỗi) | Mục 5.7: `Scope` là id tenant |
| Đường xóa workspace | `server/internal/handler/workspace.go:1567` (`deleteWorkspaceData`), `server/internal/handler/workspace_deletion.go:170` (`DeleteDueWorkspace`) | Mục 6: pha 2 |
| Đường xóa tài khoản | `server/internal/handler/me_deletion.go:365` (`DeleteAccount`), `:466` (`removeMemberForAccountDeletion`) | Mục 7: pha 2 |
| Chốt chống workspace mồ côi | `server/internal/handler/me_deletion.go:474` (`countOwners(members) <= 1`) | D75 |
| Giao thức ghế | `server/internal/seatcapacity/client.go:68` (`ClaimShareJoin`), `:70` (`Confirm`), `:71` (`Release`); mẫu dùng: `server/internal/handler/seat_capacity.go:153` (`beginShareJoinCapacity`) | Mục 4.2 |
| Khuôn yêu cầu bất đồng bộ | `server/internal/accountprovision/job.go:83` (`Job`, nhịp một phút), đăng ký ở `server/cmd/server/main.go:1055` | Hai việc nền mới |
| Kiểm toán staff | `platform-management/audit/audit.go:320` (`ActorStaff`) | Mọi hành động R6 |
| Giữ dòng kiểm toán sau khi xóa workspace | `server/pkg/db/queries/workspace_delete.sql:777` (`DetachWorkspaceAuditEvents`: đặt `workspace_id` thành NULL, dòng vẫn còn) | AC78 |
| Bảng route và vai trò của console | `platform-management/internal/console/handler.go:117` (`routeTable`), `platform-management/internal/staff/role.go:19` (`routeRole`) | Mục 9 |

### 2.2 Khoảng trống

| # | Khoảng trống | Bằng chứng | Giải ở |
| --- | --- | --- | --- |
| K26 | Tenant không phải thực thể: console coi một workspace là một tenant, `CreateWorkspace` không có khái niệm tenant, và không có giới hạn số workspace theo gói | `platform-management/internal/console/handler.go:120` (route `/tenants` trả workspace); `server/internal/handler/workspace.go:328` (`CreateWorkspace` không có tenant); `docs/engineering/platform-management-spec.md:9` | 5 |
| K27 | Không có đường nào để nhà vận hành thêm người vào workspace có sẵn; chỉ lời mời của owner và admin | `docs/engineering/platform-management-spec.md:1664` (7b.5 quy tắc 2); `platform-management/internal/staff/role.go:19` (không có khóa thêm thành viên) | 4 |
| K28 | Staff chỉ lên lịch xóa workspace với 14 ngày chờ; không có xóa ngay, không có khái niệm xóa tenant | `platform-management/internal/console/write.go:59` (`workspaces.schedule_deletion`); `server/internal/handler/workspace_deletion.go:34` | 6 |
| K29 | Xóa tài khoản chỉ do chính người dùng, chờ 14 ngày; staff không xóa được, và `Classify` chặn mọi owner còn thành viên khác | `server/internal/handler/me_deletion.go:129`; `platform-management/accountdeletion/rules.go:7` (`Wait`) | 7 |
| K30 | Vai trò staff chỉ có một cấp toàn nền tảng: `admin` làm tất cả và quyền xếp bậc cộng dồn; `platform_staff` không có cột tenant nên không thể giới hạn một staff vào một tenant | `platform-management/internal/staff/role.go:16` (`rank`); `server/migrations/663_platform_staff.up.sql:1` (bảng không có `tenant_id`) | 3.5 đến 3.8 |
| K31 | Quản trị gói thiếu route bật và tắt rõ ngữ nghĩa, mô tả, lịch sử đọc được và xác nhận khi hạ giới hạn | `platform-management/internal/subscription/console.go:461` (`putPlan` chỉ có cờ `archived`), `platform-management/internal/plan/plan.go:120` | 3b.1 |
| K32 | Không có danh sách workspace riêng, không lọc theo trạng thái, gói, tenant | `platform-management/queries/console.sql:11` (`SearchTenants` chỉ tìm theo tên, slug, email owner) | 3b.2 |
| K33 | Không xem người dùng theo tenant; staff không gỡ thành viên hay đổi vai trò thành viên được | `platform-management/internal/console/store.go:45` (`UserRow`); `server/internal/handler/workspace.go:988` (`DeleteMember` chỉ owner và admin workspace) | 3b.3 |
| K34 | Không tổng hợp tài nguyên theo tenant hay toàn hệ thống; hạn mức bổ sung chỉ đặt từng workspace | `platform-management/internal/usage/usage.go:39` (`Current` theo workspace) | 3b.4 |
| K35 | Không quản trị runtime: không danh sách, không vô hiệu hóa, không lịch sử kết nối | `server/internal/handler/daemon.go:571`, `:1514`; `server/migrations/004_agent_runtime_loop.up.sql:1` | 3b.5 |
| K36 | Chính sách bảo mật chỉ theo workspace; owner workspace có thể nới lỏng điều tenant cần giữ | `server/internal/workspaceguard/policy_access.go:143` | 3c.4 |
| K37 | Không cảnh báo trước khi chạm hạn mức đếm và không có đường xin tăng hạn mức | `server/internal/planlimit/planlimit.go:101` (chỉ từ chối ở giới hạn) | 3c.5 |
| K38 | Xóa tenant và tài khoản không hoàn tác chỉ cần một người và gõ lại tên | mục 6.2, 7.2 của tài liệu này (D72) | 3c.6 |
| K39 | Không giữ hồ sơ pháp lý: mọi đường xóa chạy bất kể vụ việc | `server/internal/handler/workspace.go:1567` (`deleteWorkspaceData` không kiểm gì ngoài trạng thái), `server/pkg/db/queries/audit_event.sql:38` | 3c.7 |
| K40 | Staff không xuất được nhật ký kiểm toán; việc nền xóa dòng cũ không biết vụ việc nào đang giữ | `server/internal/handler/audit_events.go:75`, `server/internal/scheduler/jobs_audit_retention.go:54` | 3c.8 |
| K41 | Không khóa cả tenant, không chặn tạo workspace vào tenant bị khóa, xóa ngay không báo owner và không cho xuất trước | `platform-management/internal/console/handler.go:121` (trạng thái chỉ theo workspace), `server/internal/handler/workspace.go:2007` | 3c.9 |

## 3. Kiến trúc

### 3.1 Mẫu hai pha dùng chung cho cả ba nhóm yêu cầu

Platform không nhập `server/internal` của Multica và không gọi HTTP vào Multica (D33, D35, D47). Cả thêm người, xóa ngay và xóa tài khoản đều đổi bảng của Multica (`user`, `member`, `workspace` và các bảng con). Vì vậy dùng đúng khuôn của D47:

1. **Pha 1, platform, một giao dịch.** Kiểm đầu vào ở máy chủ; ghi dòng yêu cầu (`platform_member_request` hoặc `platform_deletion_request`) ở trạng thái `pending`; ghi kiểm toán với `actor_type = staff`; với xóa ngay thì đặt trạng thái chặn truy cập. Kiểm toán và dòng yêu cầu cùng commit hoặc cùng hủy (I7). Sau commit, platform xóa các khóa cache liên quan.
2. **Pha 2, Multica, việc nền mỗi phút.** Lấy yêu cầu `pending` bằng `FOR UPDATE SKIP LOCKED`, thực hiện bằng mã hiện có của Multica, ghi các cột trạng thái, rồi ghi kiểm toán kết quả trong cùng giao dịch với thay đổi dữ liệu.
3. Mỗi việc nền lũy đẳng, chạy hai lần liên tiếp hay hai bản song song cho cùng kết quả, thử tối đa 3 lần cách nhau ít nhất một phút rồi đặt `failed`.

### 3.2 Điểm móc vào mã lõi (bổ sung cho gốc 3.3)

| Tệp | Sửa |
| --- | --- |
| `server/internal/workspaceprovision/provision.go` (`Create`, `:40`) | tạo dòng `tenant` và `tenant_workspace` cùng giao dịch; nhận `TenantID` tùy chọn |
| `server/internal/handler/workspace.go` (`CreateWorkspace`, `:328`) | nhận `tenant_id`; kiểm quyền, cổng `max_workspaces`, `tenant_deleting`; trả `tenant_id` trong phản hồi |
| `server/internal/handler/workspace.go` (`deleteWorkspaceData`, `:1567`) | thêm bước "detach tenant" trước bước "delete workspace" (D68) |
| `server/internal/handler/workspace_deletion.go` (`DeleteDueWorkspace`, `:170`), `me_deletion.go` (`DeleteAccount`, `:365`; `removeMemberForAccountDeletion`, `:466`; `deleteSoleMemberWorkspace`) | nhận `DeletionOrigin` (`self`, `period_ended`, `staff` kèm id yêu cầu, id và nhãn staff) để ghi kiểm toán đúng actor; hành vi xóa không đổi |
| `server/pkg/db/queries/platform_workspace.sql` (`ListDueWorkspaceDeletions`, `:35`), `account_deletion.sql` (`ListDueAccountDeletions`, `:46`) | bỏ qua workspace và tài khoản đang có yêu cầu xóa ngay chưa hoàn tất, để hai việc nền sẵn có không giành việc |
| `server/internal/entitlement/types.go` (`GateName`, `optionalGates`, `valid`), `client.go` (`normalizePolicy`) | thêm cổng `max_workspaces`; vắng mặt vẫn là `off` |
| `server/cmd/server/main.go` | đăng ký hai việc nền `member_provisioning` và `staff_deletion` |
| `server/internal/handler/workspace.go` (`workspaceToResponse`) | thêm trường tùy chọn `tenant_id` |
| `server/internal/handler/daemon.go` (`DaemonRegister` `:571`, `requireDaemonRuntimeAccess` `:183`), `server/internal/service/budget_gate.go` (`claimRefusalFor`, `:320`) | từ chối runtime bị vô hiệu hóa (`platform_runtime`): 403 `runtime_disabled` ở hai chỗ đầu, không giao task ở chỗ sau (QT5) |
| `server/internal/handler/runtime.go` (`DeleteAgentRuntime`, `:1337`) | xóa dòng `platform_runtime` khi runtime bị xóa; không đổi hành vi khác |
| `server/internal/workspaceguard/policy_access.go` (`EffectivePolicy`, `:143`), `server/internal/workspacepolicy/workspacepolicy.go` (`Merge`, mới), `server/internal/handler/workspace_policy.go` (`UpdateWorkspacePolicy`, `:142`) | hợp chính sách tenant và workspace; từ chối ghi dưới sàn (QT6) |
| `server/internal/handler/workspace.go` (`deleteWorkspaceData` `:1567`), `server/internal/handler/me_deletion.go` (`DeleteAccount` `:365`), `server/internal/handler/workspace.go` (`DeleteWorkspace` `:1436`) | kiểm giữ hồ sơ pháp lý ở đầu hàm; trả `legal_hold` (QT11) |
| `server/pkg/db/queries/audit_event.sql` (`DeleteAuditEventsBefore`, `:38`), `server/internal/scheduler/jobs_audit_retention.go` (`:54`) | không xóa dòng thuộc đối tượng bị giữ (QT11, QT18) |
| `server/internal/handler/workspace.go` (`CreateWorkspace` `:328`) | thêm kiểm `tenant_not_active` (QT19) |
| `server/internal/auditstore/store.go`, `server/pkg/db/queries/audit_event.sql` (`InsertAuditEvent`), `platform-management/audit/audit.go` (`Entry`, `:328`) | thêm cột và trường `tenant_id` tùy chọn của dòng kiểm toán (D84); không đổi chỗ gọi hiện có |
| `server/internal/handler/workspace_delete_manifest_test.go` | phân loại các bảng mới trong bản kê khai xóa workspace |
| `packages/core/types/workspace.ts`, `packages/core/api/client.ts` (`createWorkspace`, `:4416`), `packages/core/api/schemas.ts`, `packages/views/onboarding/steps/step-workspace.tsx` | `tenant_id` tùy chọn; ánh xạ lỗi `max_workspaces` |

### 3.3 Việc nền (bổ sung cho gốc 3.5)

| Tên | Chạy ở | Nhịp | Việc | Mục |
| --- | --- | --- | --- | --- |
| `member_provisioning` | Multica | phút | xử lý yêu cầu `platform_member_request` `pending` theo `kind`: `add` (tạo hoặc tìm `user`, giữ ghế, tạo `member`, xác nhận ghế), `remove` (gỡ thành viên, giải phóng ghế), `change_role` (đổi vai trò thành viên) | 4, 3b.3 |
| `staff_deletion` | Multica | phút | thực hiện yêu cầu `platform_deletion_request` `pending`: xóa workspace, xóa tenant, xóa tài khoản (kèm chuyển quyền), gỡ đăng ký runtime (QT5); chờ bản xuất khi `export_first` (QT19); kiểm giữ hồ sơ pháp lý (QT11) | 6, 7, 3b.5, 3c |
| `quota_alert` | platform | giờ | gửi cảnh báo 80 và 100 phần trăm hạn mức đếm theo workspace (QT7) | 3c.5 |
| `deletion_approval_expiry` | platform | phút | hủy yêu cầu xóa `awaiting_approval` quá 24 giờ (QT10) | 3c.6 |

### 3.4 Cấu hình

Đợt này thêm đúng một biến cấu hình, `PLATFORM_APPROVAL_ACTIONS`. Mọi giá trị khác là dữ liệu của gói (`max_workspaces`) hoặc hằng trong mã (nhịp việc nền, số lần thử). Một biến mới: `PLATFORM_APPROVAL_ACTIONS` (3c.6), danh sách tên hành động cần duyệt chéo, thuộc `tenant.delete_now` và `account.delete_now`, mặc định rỗng. Một biến đổi nghĩa: `PLATFORM_ADMIN_EMAILS` (gốc 5.2, D9) giữ tên, nhưng email trong đó được gieo thành `system_admin` thay cho `admin` (mục 3.8).

### 3.5 Vai trò staff và phạm vi tenant (D79, D80)

**Tên vai trò.** Spec R6 có hai tập tên vai trò, trùng chữ `admin` nhưng khác hẳn nhau. **Vai trò staff** nằm ở cột `platform_staff.role`: `system_admin`, `admin`, `support`, `billing`. **Vai trò thành viên** nằm ở cột `member.role` của một workspace: `owner`, `admin`, `member`, `viewer`. Khi có thể nhầm, tài liệu viết đủ "vai trò staff" hoặc "vai trò thành viên". Thêm người ở mục 4 chọn vai trò **thành viên**; ma trận 3.6 nói về vai trò **staff**.

| Vai trò staff | Phạm vi | `tenant_id` | Ghi chú |
| --- | --- | --- | --- |
| `system_admin` | toàn nền tảng: mọi tenant, mọi người dùng, mọi thao tác ở 3.6 | rỗng | cao nhất; có mọi quyền của `admin` không bị giới hạn tenant |
| `admin` | **một** tenant | bắt buộc, đúng một | chỉ thao tác trên đối tượng trong phạm vi bên dưới |
| `support` | toàn nền tảng, quyền như gốc 5.2 | rỗng | không đổi |
| `billing` | toàn nền tảng, quyền như gốc 5.2 | rỗng | không đổi |

**Thứ bậc.** `system_admin` > `admin`. Quan hệ xếp bậc cộng dồn của gốc 5.2 (`support` < `billing` < `admin`, mỗi vai trò gồm quyền vai trò thấp hơn, `platform-management/internal/staff/role.go:16`) **không còn**: `admin` mới không tự động có quyền của `billing` hay `support`. Quyền của từng vai trò là bảng tường minh ở 3.6. `support` và `billing` giữ đúng quyền của gốc 5.2, kể cả chỗ `billing` không khóa được workspace hay tài khoản (test `TestBillingCannotWrite`, `platform-management/internal/console/handler_test.go:120`).

**Đối tượng trong phạm vi của `admin` có `tenant_id = T`:**

| Đối tượng | Trong phạm vi khi |
| --- | --- |
| tenant | `tenant.id = T` |
| workspace | có dòng `tenant_workspace` `(workspace_id, T)` |
| người dùng (chỉ đọc) | là thành viên của ít nhất một workspace trong phạm vi |
| đơn hàng, ghi chú, số liệu sử dụng | thuộc workspace trong phạm vi |
| yêu cầu thêm người | `workspace_id` trong phạm vi |
| yêu cầu xóa | `kind = workspace` có đích trong phạm vi; `kind = tenant` có đích là T (chỉ đọc); `kind = account` không bao giờ trong phạm vi |
| dòng kiểm toán | `audit_event.tenant_id = T` hoặc `workspace_id` trong phạm vi (D84) |

**Quy tắc.**
1. Đối tượng ngoài phạm vi trả **404 có thân giống hệt đối tượng không tồn tại** (thân JSON `notFound()` của console, `platform-management/internal/console/handler.go:216`), không phân biệt "không có" với "không phải của bạn". Danh sách bị lọc im lặng, không báo số dòng bị ẩn.
2. Route mà vai trò **không bao giờ** dùng được (ô "không" ở 3.6) trả 404 phẳng của route không biết (`404 page not found`), như mẫu gốc 5.2.
3. Phạm vi đọc từ cơ sở dữ liệu ở mỗi request, từ `platform_staff.tenant_id` và `tenant_workspace`, không đặt trong cookie `mpa_session` (gốc 5.2 đã đòi kiểm lại dòng `platform_staff` ở mỗi request). Đổi vai trò hay tenant có hiệu lực ở request kế tiếp.
4. Ba lớp kiểm cùng đúng: (a) danh sách vai trò của route sinh từ bảng quyền; (b) mỗi handler gọi `staff.Access(role, action)` trả `None`, `Own` hoặc `All`, rồi lọc theo `tenant_id` khi `Own`; (c) truy vấn SQL nhận tham số phạm vi nên danh sách không bao giờ đọc dòng ngoài phạm vi. Test AC93 chạy cả handler gọi thẳng, bỏ qua middleware.
5. `admin` có `tenant_id` trỏ tới tenant không tồn tại hay đã bị xóa (mục 6) có phạm vi rỗng: mọi danh sách rỗng, mọi route theo id trả 404. Việc xóa tenant không sửa dòng staff; `system_admin` gán lại hoặc thu hồi (AC104).
6. `GET /me` (gốc 5.2, 5.4) trả thêm `tenant_id` (rỗng khi không có), là trường tùy chọn (I9); console dùng nó để ẩn mục menu và để lọc.

### 3.6 Ma trận hành động × vai trò staff (D83)

Cột `admin` luôn có nghĩa "trong tenant của mình". "mọi" là toàn nền tảng; "không" là 404 phẳng. Hàng ghi "như gốc" là hành vi của spec gốc, không đổi. Nguồn: gốc 5.2, 5.4, 5.5, 5.8, 5.12, 7b.3 quy tắc 14, 7b.4, 7b.5 và các mục 4 đến 7 của tài liệu này.

| # | Hành động (route dưới `/api/platform`) | `system_admin` | `admin` (tenant mình) | `support` | `billing` | Nguồn |
| --- | --- | --- | --- | --- | --- | --- |
| M1 | Xem danh sách staff, cấp staff, đổi vai trò, gán tenant, thu hồi (`GET` và `POST /staff`, `PUT` và `DELETE /staff/{user_id}`) | mọi | không | không | không | gốc 5.2, 5.4; 3.7 |
| M2 | Danh mục gói, route ghi (`POST /plans`, `PUT`, `DELETE /plans/{key}`); xem gói ở M30 | mọi | không | không | không | gốc 5.4, 5.5 |
| M3 | Lập, xem trước, hủy thay đổi giá (`POST`, `DELETE` của `/plans/{key}/price-changes`) | mọi | không | không | không | gốc 7b.4 |
| M4 | Đọc lịch sử thay đổi giá (`GET /plans/{key}/price-changes`) | mọi | không | không | mọi (như gốc) | gốc 7b.4, AC36 |
| M5 | Yêu cầu tạo tài khoản cho khách (`/accounts*`) | mọi | không (GĐ18) | không | không | gốc 7b.5 |
| M6 | Danh sách tenant toàn nền tảng (`GET /tenants`) | mọi tenant | cùng route, lọc: chỉ tenant của mình | mọi | mọi | gốc 5.4; 5.8 |
| M7 | Chi tiết tenant (`GET /tenants/{id}`) | mọi | tenant mình | mọi | mọi | gốc 5.4 |
| M8 | Chi tiết, số liệu dùng, ghi chú của workspace (`GET /workspaces/{id}`, `/usage`, `/notes`) | mọi | workspace trong tenant mình | mọi | mọi | gốc 5.4 |
| M9 | Ghi chú nội bộ (`POST /workspaces/{id}/notes`) | mọi | workspace trong tenant mình | mọi | mọi | gốc 5.2, 5.4 |
| M10 | Khóa và mở workspace (`POST /workspaces/{id}/status` về `active`, `read_only`, `suspended`) | mọi | workspace trong tenant mình | mọi (như gốc) | không (như gốc) | gốc 5.2, 5.3 |
| M11 | Lên lịch xóa workspace có thời gian chờ và hủy lịch (trạng thái `pending_deletion`) | mọi | workspace trong tenant mình | không (như gốc) | không (như gốc) | gốc 5.2, 5.12 |
| M12 | Đổi gói, ghế, kỳ, gia hạn, ghi đè giới hạn; xác minh hồ sơ thanh toán (`PUT /workspaces/{id}/subscription`, `billing-profile*`) | mọi | không (GĐ18) | không | mọi (như gốc) | gốc 5.4, 7b.1 |
| M13 | Xem đơn hàng (`GET /orders`, `/orders/{id}/einvoices`) | mọi | đơn của workspace trong tenant mình, chỉ đọc (GĐ18) | không | mọi (như gốc) | gốc 5.4, 5.8 |
| M14 | Xác nhận, hủy đơn (`POST /orders/{id}/confirm`, `/cancel`) | mọi | không | không | mọi (như gốc) | gốc 5.4, 5.8 |
| M15 | Xóa nợ gói theo mức dùng (hủy hóa đơn `usage` như xóa nợ, `platform-management/internal/console/orders.go:323`) | mọi | không | không | không (như gốc: chỉ vai trò cao nhất) | gốc 7b.3 quy tắc 14 |
| M16 | Thông báo thanh toán, hóa đơn điện tử (`/payment-notices*`, `/einvoices*`) | mọi | không | không | mọi (như gốc) | gốc 7.2, 7.3 |
| M17 | Tổng quan (`GET /overview`) | mọi | không (GĐ18) | mọi | mọi | gốc 5.4 |
| M18 | Xem người dùng (`GET /users`, `GET /users/{id}`) | mọi | người dùng là thành viên của workspace trong tenant mình | mọi | mọi | gốc 5.4 |
| M19 | Khóa, mở tài khoản, buộc đăng xuất, đặt lại MFA (`POST /users/{id}/status`, `/revoke-sessions`, `/mfa-reset`) | mọi | không (GĐ18) | mọi (như gốc) | không (như gốc) | gốc 5.2, 5.4 |
| M20 | Đọc kiểm toán (`GET /audit-events`, `GET /workspaces/{id}/audit-events`) | toàn bộ | chỉ dòng có `tenant_id` hoặc `workspace_id` thuộc tenant mình (D84) | toàn bộ | toàn bộ | gốc 5.4 |
| M21 | Thêm người vào workspace (`POST /workspaces/{id}/members`, D60) | mọi workspace | workspace trong tenant mình | không | không | mục 4 |
| M22 | Xem, hủy yêu cầu thêm người (`GET /workspaces/{id}/member-requests`, `GET /member-requests/{id}`, `POST /member-requests/{id}/cancel`) | mọi | yêu cầu của workspace trong tenant mình | không | không | mục 4.4 |
| M23 | Xóa ngay workspace (`POST /workspaces/{id}/delete-now`, D71) | mọi workspace | workspace trong tenant mình | không (vẫn chỉ khóa, mở như M10) | không | mục 6 |
| M24 | Xóa ngay tenant (`POST /tenants/{id}/delete-now`, D71, D85) | mọi tenant | **không**, kể cả tenant của mình (GĐ14) | không | không | mục 6 |
| M25 | Xóa ngay tài khoản người dùng (`POST /users/{id}/delete-now`, `GET /users/{id}/delete-now-preview`, D74, D86) | mọi tài khoản | **không** (GĐ15) | không | không | mục 7 |
| M26 | Chọn (A) chuyển quyền owner hoặc (B) xóa workspace khi xóa tài khoản (D75) | mọi (chỉ `system_admin` tạo được yêu cầu xóa tài khoản; lựa chọn (B) dùng quyền M23, `system_admin` có ở mọi workspace) | không (không tạo được yêu cầu xóa tài khoản; quyền M23 chỉ áp cho yêu cầu xóa workspace) | không | không | mục 7.3 |
| M27 | Xem yêu cầu xóa (`GET /deletion-requests`, `GET /deletion-requests/{id}`) | mọi | `kind = workspace` có đích trong tenant mình và `kind = tenant` của tenant mình, chỉ đọc; không thấy `kind = account` | không | không | mục 6.5 |
| M28 | Hủy, thử lại yêu cầu xóa (`POST /deletion-requests/{id}/cancel`, `/retry`) | mọi yêu cầu | chỉ yêu cầu `kind = workspace` do chính mình tạo, đích trong tenant mình | không | không | mục 6.2 quy tắc 6 |
| M29 | Đăng nhập console, `GET /me` | có | có (trả `tenant_id`) | có | có | gốc 5.2 |

Hai điểm cần đọc kỹ. (1) Bảng gốc 5.2 ghi `billing` có "quyền xem của `support`" nhưng mã không cho `billing` khóa hay mở (hàng M10, M19): tài liệu này giữ hành vi của mã và của test. (2) Lên lịch xóa có thời gian chờ (M11) vẫn không thuộc `support` hay `billing`, như gốc 5.2.

### 3.7 Gán tenant cho tài khoản `admin` và quản lý staff (D81)

**Quy tắc dữ liệu.**
- Staff có vai trò `admin` phải có đúng một `tenant_id` trỏ tới một tenant tồn tại. `system_admin`, `support`, `billing` có `tenant_id` rỗng; gửi `tenant_id` cho các vai trò này là lỗi.
- Một `tenant_id` có thể có nhiều `admin`; một `admin` chỉ có một `tenant_id`. Dòng staff đang hoạt động duy nhất theo `user_id` (`server/migrations/664_platform_staff_user_idx.up.sql`) nên không có cách gán hai tenant cho một người.

**Route.** Tất cả chỉ `system_admin` dùng được; vai trò khác nhận 404 (M1). Các route `GET /staff`, `POST /staff`, `DELETE /staff/{user_id}` **đã có** ở gốc 5.4 (`platform-management/internal/console/handler.go:134` đến `:138`); mục này đổi vai trò được phép và nội dung. `PUT /staff/{user_id}` **mới**.

| Route | Trạng thái | Thân hoặc hành vi |
| --- | --- | --- |
| `GET /staff` | có ở gốc | thêm `tenant_id` và tên tenant vào mỗi dòng |
| `POST /staff` | có ở gốc, đổi | `{email, role, tenant_id?}`; `role` thuộc bốn vai trò; `admin` đòi `tenant_id`; tài khoản Multica phải có sẵn (`no_such_account` như hiện nay) |
| `PUT /staff/{user_id}` | **mới** | `{role, tenant_id?}`: đổi vai trò, gán lại tenant; đích phải là staff đang hoạt động |
| `DELETE /staff/{user_id}` | có ở gốc, đổi | thu hồi; chốt "không thu hồi người cuối cùng" chuyển từ `admin` sang `system_admin` |

**Ràng buộc.**
1. `role = admin` thiếu `tenant_id`: 400 `tenant_required`. `tenant_id` kèm vai trò khác `admin`: 400 `tenant_not_allowed`. `tenant_id` không tồn tại: 400 `tenant_invalid`. Vai trò ngoài bốn giá trị: 400 `invalid_role` (mã có sẵn, thông điệp đổi).
2. Đổi từ `admin` sang vai trò khác xóa `tenant_id`; đổi sang `admin` đòi `tenant_id`; đổi tenant của `admin` là một lần `PUT`.
3. **`admin` không tự nâng quyền hay nâng quyền cho ai:** `admin` không dùng được `/staff` (M1), và bốn handler staff tự kiểm `staff.Access(role, "staff.manage") = All`, nếu không thì trả 404 và không ghi gì, kể cả khi bị gọi thẳng bỏ qua middleware. Trong `PUT`, `system_admin` không hạ vai trò của chính mình nếu mình là `system_admin` cuối cùng.
4. Không thu hồi hay hạ bậc `system_admin` còn hoạt động cuối cùng: 400 `last_system_admin`. Việc đếm khóa các dòng `system_admin` trước khi đếm (như `removeStaff` hiện nay, `platform-management/internal/console/write.go:457`), nên hai yêu cầu đồng thời không cùng hạ hai người cuối.
5. Mọi thay đổi chạy trong một giao dịch cùng dòng kiểm toán `actor_type = staff`: `staff.added` (có sẵn), `staff.removed` (có sẵn), `staff.role_changed`, `staff.tenant_assigned` (mới); `detail` ghi vai trò và tenant cũ và mới; dòng có `tenant_id` của tenant liên quan (D84). Ghi kiểm toán lỗi thì thay đổi không lưu (I7).
6. Mọi staff vẫn qua MFA và `PLATFORM_ADMIN_CIDRS` như gốc 5.2.

### 3.8 Migration vai trò và dữ liệu cũ (D82, D84)

**Hiện trạng đã đọc.** `platform_staff.role` là cột `text` không có ràng buộc `CHECK` (`server/migrations/663_platform_staff.up.sql:1`). Tập vai trò hợp lệ do mã kiểm bằng `staff.ParseRole` (`platform-management/internal/staff/role.go`, đọc qua `rank`). Vì vậy việc "thêm `system_admin` vào ràng buộc vai trò" làm ở `ParseRole` và bảng quyền, không bằng một ràng buộc cơ sở dữ liệu; thêm `CHECK` sẽ khóa bảng và làm bản platform cũ khi hoàn nguyên đọc dòng lạ. Platform cũ coi vai trò lạ là "không tìm thấy" (`platform-management/internal/platformauth/platformauth.go:55`, 404), nên thứ tự triển khai ở mục 12 quan trọng.

**Thay đổi lược đồ** (số mới; mọi số chưa dùng ở `server/migrations/` lúc viết; platform không có chuỗi migration riêng, D33):

| Migration | Nội dung |
| --- | --- |
| 942 | `ALTER TABLE platform_staff ADD COLUMN IF NOT EXISTS tenant_id uuid` (rỗng được, không khóa ngoại) |
| 943 | `CREATE INDEX CONCURRENTLY IF NOT EXISTS platform_staff_tenant_idx ON platform_staff (tenant_id) WHERE revoked_at IS NULL AND tenant_id IS NOT NULL` |
| 944 | dữ liệu: `UPDATE platform_staff SET role = 'system_admin' WHERE role = 'admin' AND tenant_id IS NULL` |
| 945 | `ALTER TABLE audit_event ADD COLUMN IF NOT EXISTS tenant_id uuid` (rỗng được, không khóa ngoại) |
| 946 | `CREATE INDEX CONCURRENTLY IF NOT EXISTS audit_event_tenant_created_idx ON audit_event (tenant_id, created_at DESC, id) WHERE tenant_id IS NOT NULL` |

Số ba chữ số bảo đảm thứ tự đúng trên cơ sở dữ liệu mới: `platform_staff` tạo ở 663 và `audit_event` ở 654, cả hai đứng trước 942 đến 946 theo thứ tự chuỗi (mục 5.4 nói vì sao số bốn chữ số không dùng được ở đây). Hai chỉ mục tạo `CONCURRENTLY`, mỗi chỉ mục một tệp, có tệp `down` và dòng dọn trong `concurrentIndexCleanups` (`server/cmd/migrate/main.go:142`).

**Dữ liệu cũ (giả định GĐ16).** Migration 944 chuyển mọi dòng `role = 'admin'` chưa có tenant thành `system_admin`, gồm cả dòng đã thu hồi (dòng thu hồi chỉ là lịch sử; `ReactivateStaff` luôn ghi vai trò tường minh, `platform-management/queries/platform_staff.sql`). Điều kiện `tenant_id IS NULL` làm migration **lũy đẳng** và không đụng `admin` mới (luôn có tenant, mục 3.7): chạy lại không đổi gì. Hệ quả nếu sai: ghi ở GĐ16.

**Gieo staff từ cấu hình.** `PLATFORM_ADMIN_EMAILS` giữ tên; bộ gieo (`platform-management/internal/staff/seed.go`) mở lại hoặc tạo dòng với vai trò `system_admin`, không còn `admin`. Dòng đang hoạt động của một email trong danh sách được giữ nguyên vai trò, như hiện nay, nên `system_admin` hạ một người xuống `admin` thì lần gieo kế không nâng lại. Đó vẫn là "lối vào không thể tự khóa" của gốc 5.2: nếu không còn `system_admin` nào hoạt động, email trong danh sách mà dòng đã bị thu hồi được mở lại ở lần gieo kế.

**Hoàn nguyên.** `down` của 944 phải an toàn: nó thu hồi (`revoked_at = now()`) mọi `admin` có tenant đang hoạt động trước, rồi đặt `system_admin` về `admin`, vì bản platform cũ sẽ coi `admin` có tenant là quản trị toàn nền tảng. `down` của 942 và 945 bỏ cột; của 943 và 946 bỏ chỉ mục.

## 3b. Các nhóm quản trị QT1 đến QT5

Năm nhóm này bổ sung màn hình và hành động quản trị cho `system_admin`, `admin` (tenant của mình), `support` và `billing`. Chúng dùng đúng mô hình vai trò ở 3.5 đến 3.8 và thêm hàng M30 đến M52 vào ma trận (3c.10). Tên **QT1 đến QT19** là mã nhóm quản trị của đợt này; chúng không trùng "nhóm 1 đến 4" ở mục 4 đến 7 (bốn quyết định ban đầu). QT1 đến QT5 là các nhóm 1 đến 5 của yêu cầu quản trị, QT6 đến QT19 là các nhóm 6 đến 19 ở mục 3c.

**Quy ước chung của mọi nhóm QT.**
- Phạm vi theo 3.5: đối tượng ngoài tenant của `admin` trả 404 có thân giống đối tượng không tồn tại; danh sách lọc im lặng; route mà vai trò không bao giờ dùng được trả 404 phẳng.
- Phân trang theo khóa (`created_at`, `id`) như `GET /tenants` (`platform-management/internal/console/read.go:96`): tham số `limit` (mặc định 50, tối đa 200) và `cursor`; phản hồi có `next_cursor`.
- Hành động ghi chạy trong một giao dịch cùng dòng kiểm toán (I7). Mọi dòng kiểm toán của nhóm QT có `actor_type = staff`, `action` theo bảng của từng nhóm, `tenant_id` (D84), `workspace_id` khi hành động có workspace, và `user_id` đặt ở `target_type = 'user'`, `target_id` khi hành động có người dùng.
- Mọi trường phản hồi mới đi qua schema zod của console với giá trị rơi về cho enum lạ (I9).

### 3b.1 QT1: quản trị gói (K31, D87 đến D89)

**Hiện trạng đã đọc.** Phần lớn CRUD gói đã có: `GET /plans` (`platform-management/internal/subscription/console.go:380`), `POST /plans` tạo gói với khóa `key` duy nhất (409 `plan_exists`, `:593`), `PUT /plans/{key}` sửa tên, `limits`, `features`, tiền tệ, giá ghế, `audience`, `archived` kèm `expected_version` (`:461`), `DELETE /plans/{key}` từ chối khi có thuê bao hoặc đơn bằng 409 `plan_in_use` (`:664`), tiền tệ khóa khi đã dùng (`plan_currency_locked`), và mọi thay đổi ghi `plan.changed` có `changes` (giá trị cũ và mới, `:560`). Giá chỉ đổi qua lịch (`plan_price_change`, gốc 7b.4). `plan.ArchivedAt` (`platform-management/internal/plan/plan.go:120`) là cờ lưu trữ. Hành vi "tắt" cũng đã có: gói lưu trữ không hiện trong danh sách cho khách (`listed`, `platform-management/internal/subscription/api.go:539`, và truy vấn `ListPublicPlans`, `server/pkg/db/queries/billing_order.sql:5`) và không đặt được đơn mới trừ thuê bao của chính nó (`pickPlan`, `platform-management/internal/billingapi/billingapi.go:294`), nhưng trả "không tìm thấy" thay vì mã rõ.

**Còn thiếu và bổ sung ở đây:** (1) bật và tắt gói bằng route riêng, kèm mã lỗi `plan_inactive` rõ thay cho "không tìm thấy" (ngữ nghĩa ẩn với khách mới và giữ thuê bao cũ đã có); (2) trường `description`; (3) lịch sử thay đổi đọc được (tên, mô tả, `limits`, `features`, giá) của một gói; (4) xác nhận khi hạ giới hạn của gói đang có thuê bao; (5) quyền xem cho `admin`.

**Quy tắc.**
1. **Mã gói.** `key` (cột `plan.key`) là mã gói: duy nhất, 1 đến 32 ký tự, không đổi sau khi tạo (`PUT` không nhận `key` mới). Đây là "code" trong yêu cầu; không thêm cột `code`.
2. **Tạo.** `POST /plans` nhận `key`, `name`, `description` (tối đa 500 ký tự, tùy chọn), `currency`, giá, `limits` (gồm `max_workspaces`, D66, không nhỏ hơn 1), `features`, `audience`, `billing_model`, `is_public`, `sort`. Gói mới ở trạng thái `active`.
3. **Sửa.** Đổi `name`, `description`, `limits`, `features`, `is_public`, `sort` tại chỗ với `expected_version`. Hạ một giới hạn (giá trị mới nhỏ hơn giá trị cũ, kể cả từ "không giới hạn" xuống có giới hạn) của gói đang có ít nhất một thuê bao đòi `confirm_affects_subscribers = true`; thiếu thì 409 `plan_limit_change_confirm_required` kèm `affected_subscriptions`. Hạ giới hạn không xóa hay khóa dữ liệu (I10): chỉ chặn tạo mới.
4. **Giá.** Đổi giá chỉ qua `plan_price_change` (gốc 7b.4): không đổi đơn đã tạo hay kỳ đã trả; thuê bao đang chạy chịu giá mới từ kỳ gia hạn sau `effective_from`, có báo trước `PLATFORM_PRICE_NOTICE_DAYS` ngày khi tăng (quy tắc gốc, giữ nguyên, GĐ28). Muốn thuê bao hiện hữu **không** bị đổi giá, tạo một gói mới giá mới (`POST /plans`), tắt gói cũ cho khách mới và để thuê bao cũ ở lại gói cũ.
5. **Bật và tắt.** `POST /plans/{key}/deactivate` đặt `archived_at`; `POST /plans/{key}/activate` xóa nó. Gói `inactive` **không** xuất hiện trong `GET /api/workspaces/{id}/plans` cho khách chọn và không tạo được đơn `subscribe` hay `change_plan` mới tới gói đó (409 `plan_inactive`); thuê bao đang dùng giữ nguyên gói, gia hạn và mua ghế tiếp tục theo gói đó, và mọi giới hạn vẫn áp dụng. `PUT` với `archived` vẫn chạy nhưng là bí danh của hai route trên (cùng kiểm toán).
6. **Xóa.** `DELETE /plans/{key}` chỉ khi không có thuê bao và không có đơn nào (kiểm trong code bằng `PlanHasSubscriptionsOrOrders`, không khóa ngoại); ngược lại 409 `plan_in_use` và đề nghị `deactivate`. Gói `free` không xóa được (như hiện nay).
7. **Lịch sử.** `GET /plans/{key}/history` trả danh sách theo thời gian: mỗi mục có `at`, `actor` (email staff), `action` (`plan.changed`, `plan.price_scheduled`, `plan.price_applied`, `plan.price_canceled`, `plan.activated`, `plan.deactivated`), `changes[]` (`field`, `old`, `new`) và `reason` khi có. Đọc từ `audit_event` theo `target_type = plan` và `target_id = key`; không bảng mới.
8. **Quyền.** Ghi (tạo, sửa, bật, tắt, xóa, lập giá): chỉ `system_admin` (M31). Xem danh mục: `system_admin` và `admin` (chỉ đọc, M30). Lịch sử: `system_admin` và `billing` (M32).

**API.** `GET /plans`, `GET /plans/{key}` (mới, một gói), `POST /plans`, `PUT /plans/{key}`, `DELETE /plans/{key}`, `POST /plans/{key}/activate`, `POST /plans/{key}/deactivate`, `GET /plans/{key}/history`. **Mã lỗi:** `plan_exists` 409 (có), `plan_in_use` 409 (có), `plan_limit_change_confirm_required` 409 (`affected_subscriptions`), `plan_inactive` 409. **Kiểm toán:** `plan.activated`, `plan.deactivated` (mới); các hành động `plan.*` khác đã có. **Dữ liệu:** thêm cột `plan.description text` (migration 947).

### 3b.2 QT2: danh sách workspace toàn hệ thống và hành động nhanh (K32, D90, D91)

**Hiện trạng.** Danh sách hiện có là danh sách "tenant" cũ, thực chất là workspace, với tìm kiếm theo tên, slug, email owner và cursor (`platform-management/queries/console.sql:11`, `SearchTenants`). Sau D77, `/tenants` là tenant. Không có route danh sách workspace riêng, không lọc theo trạng thái, gói hay tenant.

**Quy tắc.**
1. `GET /workspaces` (mới): tìm `q` theo tên, slug, email owner, tên tenant; lọc `status` (`active`, `read_only`, `suspended`, `pending_deletion`, gốc 5.3), `plan_key` (gói hiệu lực, workspace không có thuê bao là `free`), `tenant_id`. Mỗi dòng: `id`, `name`, `slug`, `tenant_id`, `tenant_name`, `status`, `plan_key`, `member_count`, `created_at`, `last_activity_at`.
2. `GET /workspaces/{id}` (đã đổi tên ở 5.8) thêm: `tenant` (id, tên), `subscription` (gói, trạng thái, kỳ, ghế đã mua), `seats` (đã mua, đã dùng, đang giữ, theo `platform_seat_operation`), `usage_current` (`usage.Current`, gốc 5.13), `owner`, `created_at`, và `status_history[]` (đọc từ `audit_event`, `action = workspace.status_changed` kèm các hành động xóa ngay, 20 dòng mới nhất, có `cursor`).
3. Hành động nhanh (từ dòng danh sách hoặc trang chi tiết) **không có route mới**; mỗi nút gọi đúng route đã có và theo đúng ma trận: khóa, mở khóa (M10), lên lịch xóa và hủy lịch (M11), xóa ngay (M23), đổi gói và gia hạn (M12), mở nhật ký kiểm toán của workspace (M20). Thêm duy nhất `POST /workspaces/{id}/export` (M34): staff yêu cầu một bản xuất dữ liệu workspace (gốc 5.11) bằng cách chèn dòng `workspace_export` `queued` do platform sở hữu (`server/migrations/685_workspace_export.up.sql:1`) với `requested_by` là staff; việc nền `workspace_export` của Multica xử lý như cũ. Chỉ một bản đang chạy cho một workspace (chỉ mục `workspace_export_running_uidx`), vi phạm trả 409 `export_in_progress`.
4. **Trạng thái không cho phép:** mỗi hành động kiểm trạng thái hiện tại ở máy chủ; lệch thì 409 `workspace_state_conflict` kèm `status` hiện tại và `allowed[]`, không ghi gì. Ví dụ khóa workspace đã `suspended`, hủy lịch xóa workspace không ở `pending_deletion`, xuất dữ liệu khi workspace `pending_deletion` do xóa ngay.
5. Kiểm toán: các hành động đã có (`workspace.status_changed`, `workspace.immediate_deletion_requested`, `subscription.changed`, và mới `workspace.export_requested`) nay đều ghi `workspace_id` và `tenant_id`.
6. Quyền: M33 (danh sách), M8 (chi tiết), M34 (xuất). `admin` thấy và làm chỉ trong tenant mình.

**Mã lỗi:** `workspace_state_conflict` 409 (`status`, `allowed[]`), `export_in_progress` 409. **Kiểm toán mới:** `workspace.export_requested`. **Dữ liệu:** không bảng mới; `SearchWorkspaces` là truy vấn mới trên `workspace`, `tenant_workspace`, `platform_workspace`, `workspace_subscription` (có chỉ mục sẵn trên các khóa nối; không quét toàn bảng nhờ cursor và `limit`).

### 3b.3 QT3: quản trị người dùng theo tenant (K33, D92 đến D95)

**Định nghĩa (giả định GĐ19).** Người dùng **thuộc tenant T** khi là thành viên (`member`) của ít nhất một workspace trong T. Không có bảng "thành viên tenant". Hệ quả nếu sai: ở GĐ19.

**Hiện trạng.** `GET /users` và `GET /users/{id}` (gốc 5.4) trả người dùng toàn hệ thống kèm tư cách thành viên; `POST /users/{id}/status`, `/revoke-sessions`, `/mfa-reset` là thao tác cấp tài khoản (`platform-management/internal/console/handler.go:131` đến `:133`, M19). Gỡ thành viên và đổi vai trò thành viên chỉ có ở Multica cho owner và admin workspace (`server/internal/handler/workspace.go:923` `UpdateMemberRole`, `:988` `DeleteMember`, `server/internal/handler/workspace_revoke.go:49` `revokeAndRemoveMember`); platform không có đường làm thay.

**Quy tắc.**
1. `GET /tenants/{id}/users` (mới, M35): mỗi dòng gồm `user_id`, `name`, `email`, `account_status` (`active`, `suspended`, `pending_deletion`; giá trị `deactivated` trong yêu cầu ánh xạ sang `suspended`, D93), `workspaces[]` (`workspace_id`, `name`, `member_role`), `seats` (số workspace trong tenant mà người đó chiếm ghế), `last_login_at` (dòng `auth.login` mới nhất trong `audit_event`, rỗng nếu chưa từng), `is_staff` (chỉ `system_admin` thấy giá trị thật; với vai trò khác luôn `false`, và người dùng là staff không hiện với `admin`, GĐ19). Tìm `q` (tên, email), lọc `account_status`, `workspace_id`, `member_role`; phân trang.
2. `GET /tenants/{id}/users/{user_id}` (mới): chi tiết kèm 20 dòng kiểm toán gần nhất có `user_id` đó trong tenant, và `allocation` (D95).
3. **Vô hiệu hóa, hai phạm vi (D93, GĐ20).** (a) **Trong tenant** (`POST /tenants/{id}/users/{user_id}/deactivate`): gỡ người đó khỏi **mọi workspace của tenant** bằng yêu cầu gỡ (quy tắc 5), không đụng tài khoản hay workspace ngoài tenant. Quyền: `system_admin`, `admin` (M36). (b) **Toàn cục**: khóa tài khoản và thu hồi phiên bằng `POST /users/{id}/status` và `/revoke-sessions` (gốc 4.1, 4.2, M19): `system_admin`, `support`. **Khuyến nghị:** làm (a) là hành động mặc định của `admin` vì không ảnh hưởng tenant khác; (b) chỉ cho vai trò xuyên tenant. Phương án còn lại, dùng một nút "vô hiệu hóa" khóa toàn cục cho cả `admin`, bị loại vì một tài khoản có thể thuộc nhiều tenant (cùng lý do GĐ15). **Kích hoạt lại:** (a) là thêm lại bằng D60 (M21, vai trò thành viên chọn lại); (b) là mở khóa M19.
4. **Gỡ khỏi một workspace** (`POST /workspaces/{id}/members/{user_id}/remove`, M36) và **đổi vai trò thành viên** (`POST /workspaces/{id}/members/{user_id}/role`, thân `{role}` với `role` thuộc `admin`, `member`, `viewer`, M37) là yêu cầu bất đồng bộ trên **cùng** bảng `platform_member_request` (D94): thêm cột `kind` (`add`, `remove`, `change_role`), `target_user_id`, `previous_role`; việc nền `member_provisioning` xử lý cả ba loại. Gỡ dùng `revokeAndRemoveMember` và giải phóng ghế theo giao thức ghế (`server/internal/handler/seat_capacity.go:227` `enqueueMemberCapacityRelease`, Phụ lục B `ReleaseMember`); đổi vai trò dùng `UpdateMemberRole`; mỗi cái ghi kiểm toán `actor_type = staff` trong cùng giao dịch dữ liệu.
5. **Ràng buộc.** Vai trò thành viên `owner` bất biến qua đường này: không gỡ owner, không đổi vai trò từ hoặc sang `owner` (409 `owner_role_immutable`); gỡ hay hạ owner duy nhất đã bị chặn bởi quy tắc owner (`sole_owner`). `admin` không đổi vai trò thành viên của chính mình (409 `cannot_change_own_role`), không thao tác lên người dùng có dòng `platform_staff` đang hoạt động (404 với `admin`; `system_admin` nhận 409 `user_is_staff`). Người dùng ngoài tenant của `admin` trả 404. Gỡ làm tài khoản không còn thành viên nào vẫn **không** xóa tài khoản (D74 là đường duy nhất).
6. **Xóa người dùng:** chỉ `system_admin` bằng D74 (M25); `admin` nhận 404 (GĐ15).
7. Kiểm toán: `member.staff_remove_requested`, `member.staff_removed`, `member.staff_role_change_requested`, `member.staff_role_changed`, `member.staff_remove_failed`, `member.staff_role_change_failed`; mỗi dòng có `actor_type = staff`, `tenant_id`, `workspace_id`, `user_id` (target).

**Phân bổ tài nguyên theo người dùng (D95, GĐ21).** Mã hiện không có hạn mức hay tài nguyên gắn riêng cho một người dùng: ghế, agent, runtime, dung lượng và API đều tính theo **workspace** (gốc 5.5, 5.7, 5.10), và ghế là một dòng `member` trong một workspace. Vì vậy "nâng hạn mức và phân bổ thêm tài nguyên cho người dùng" được thực hiện ở cấp workspace, ngay trên dòng người dùng: nút "Thêm ghế" và "Nâng hạn mức" mở thao tác đã có trên workspace của người đó (`PUT /workspaces/{id}/subscription`, M12: `system_admin`, `billing`) hoặc gửi yêu cầu tăng hạn mức (QT7, M48: `system_admin`, `admin`). `GET …/users/{user_id}` trả `allocation`: ghế của người đó (`workspace_id`, vai trò, có chiếm ghế hay không) và, cho mỗi workspace, hạn mức hiệu lực cùng mức dùng (đọc từ QT4). Không bảng hạn mức theo người dùng. Hệ quả nếu sai: ở GĐ21.

**Mã lỗi:** `owner_role_immutable` 409, `cannot_change_own_role` 409, `member_role_invalid` (đã có) 400; `sole_owner` (đã có) 409.

### 3b.4 QT4: tài nguyên của tenant và toàn hệ thống (K34, D96, D97)

**Loại tài nguyên tìm thấy trong mã** (nguồn, đơn vị):

| Loại | Nguồn dữ liệu | Đơn vị | Có hạn mức theo gói |
| --- | --- | --- | --- |
| thành viên | `platform_usage_daily.members` và `usage.Current.Members` (`platform-management/internal/usage/usage.go:39`) | người | ghế (`seats`, `seats_fixed`) |
| ghế đang giữ | `platform_seat_operation` (gốc 5.6) | ghế | cùng ghế |
| agent | `platform_usage_daily.agents`, `usage.Current.Agents` (bảng `agent`, chưa lưu trữ, `kind = user`) | cái | `agents` |
| runtime | `platform_usage_daily.runtimes`, `usage.Current.Runtimes` (bảng `agent_runtime`, `server/migrations/004_agent_runtime_loop.up.sql:1`) | cái | `runtimes` |
| issue | `platform_usage_daily.issues_total`, `issues_created`, `usage.Current.IssuesTotal` | cái | `issue_count` |
| task đã chạy | `platform_usage_daily.tasks_run` | task mỗi ngày | không |
| dung lượng lưu trữ | `platform_usage_daily.storage_bytes`, `usage.Current.StorageBytes` (bảng `attachment`) | byte | `storage_bytes` |
| lượt autopilot trong chu kỳ | bảng `autopilot_quota_period` (`server/migrations/352_autopilot_quota_execution.up.sql:5`) | lượt mỗi tháng UTC | `autopilot_runs` |
| nhóm tài khoản | bảng `member_group` | nhóm | `member_groups` |
| workspace của tenant | `tenant_workspace` (D64) | cái | `max_workspaces` |
| lượt gọi API | bộ đếm Redis mỗi phút (`server/internal/middleware/workspace_rate_limit.go:57`); **không lưu lịch sử** | lượt mỗi phút | `api_rate_per_minute` |

`platform_usage_daily` (`server/migrations/689_platform_usage_daily.up.sql:1`) đã giữ số liệu hằng ngày theo workspace; số hiện tại tính trực tiếp bằng truy vấn có giới hạn (gốc 5.13). Project (`project`) không có hạn mức và không có cột trong snapshot, nên không vào tổng hợp ở đợt này.

**Quy tắc.**
1. `GET /tenants/{id}/resources` (M38): với mỗi loại có hạn mức, trả `used` (hiện tại), `limit` (hiệu lực của gói cộng `limit_overrides`, rỗng là không giới hạn), `percent`, `status` (`ok`, `warning` từ 80 phần trăm, `over`), tổng của tenant và phân theo từng workspace. Loại không có hạn mức (task, issue theo ngày) trả số và xu hướng bảy ngày từ `platform_usage_daily`. `admin` xem tenant mình, `system_admin`, `support`, `billing` xem mọi tenant.
2. `GET /resources` (M39, chỉ `system_admin`): tổng hợp toàn hệ thống theo loại, theo gói, theo tenant; `top` tenant tiêu thụ nhiều nhất theo từng loại (`top=10` mặc định, tối đa 50); danh sách `over_limit[]` các (tenant, workspace, loại) đang `over` hoặc `warning`. Phân trang cho danh sách tenant; phản hồi có `complete` (boolean, `true` khi đã duyệt hết các trang), vì tổng chính xác đòi duyệt hết các trang.
3. **Chi phí.** Không bảng hay view mới. Tổng hợp đọc `platform_usage_daily` (ngày mới nhất mỗi workspace) và bộ đếm hiện tại có giới hạn; trang toàn hệ thống đọc tối đa một dòng mỗi workspace và chạy theo trang `limit`, nên tốn theo số workspace trong trang, không theo toàn bảng. Nếu đo thấy chậm, thêm một việc nền ghi bảng tổng hợp theo tenant là việc của đợt sau (GĐ23).
4. **Điều chỉnh hạn mức bổ sung (D97).** `POST /tenants/{id}/limit-overrides` thân `{gate, value, workspace_ids?, reason}`: `gate` thuộc `plan.LimitGates` (gồm `max_workspaces`), `value` là số nguyên không âm hoặc `null` (nhả giới hạn của gói). Mặc định áp cho **mọi workspace** của tenant; `workspace_ids` thu hẹp. Thực hiện bằng đúng cơ chế `limit_overrides` của thuê bao từng workspace (gốc 5.5), trong một giao dịch, tăng `version` từng thuê bao, xóa cache chính sách, và ghi **một dòng kiểm toán mỗi workspace** (`subscription.limit_override_changed`, giá trị cũ và mới, `reason` bắt buộc) kèm `tenant_id`. Workspace chưa có dòng thuê bao được tạo dòng `free` (cùng cách `PUT /workspaces/{id}/subscription`). Quyền M40: `system_admin`, `billing` (như M12). `admin` chỉ xem, không tự tăng (404 ở route ghi); `admin` gửi yêu cầu tăng qua QT7.
5. Không có hạn mức "cho cả tenant" như một đối tượng riêng: hạn mức đếm theo workspace (gốc 5.7) và `max_workspaces` là hạn mức duy nhất ở cấp tenant (D66). Hệ quả nếu sai: GĐ23.

**Mã lỗi:** `limit_gate_invalid` 400 (`gate`), `limit_value_invalid` 400. **Kiểm toán mới:** `subscription.limit_override_changed`.

### 3b.5 QT5: runtime và máy (daemon) đang kết nối (K35, D98, D99)

**Mô hình trong mã.** Bảng `agent_runtime` (`server/migrations/004_agent_runtime_loop.up.sql:1`): `id`, `workspace_id` (nên thuộc workspace và qua `tenant_workspace` thuộc tenant), `daemon_id`, `name`, `custom_name`, `runtime_mode` (`local`, `cloud`), `provider`, `status` (`online`, `offline`), `device_info` (văn bản hệ điều hành và máy), `metadata` (JSON, có `version` của agent CLI và `cli_version`, `server/internal/handler/daemon.go:796`), `last_seen_at`, `owner_id`, `max_concurrent_tasks`. Daemon đăng ký ở `DaemonRegister` (`server/internal/handler/daemon.go:571`), gửi nhịp tim ở `DaemonHeartbeat` (`:1514`), hủy đăng ký ở `DaemonDeregister` (`:1364`); mọi route của daemon đi qua `requireDaemonRuntimeAccess` (`:183`). Task chạy trên runtime nằm ở `agent_task_queue.runtime_id`. Token `mdt_` của daemon nằm ở `daemon_token` (`server/migrations/029_daemon_token.up.sql:1`).

**Khoảng trống.** Hệ thống **không lưu lịch sử kết nối**: chỉ có trạng thái hiện tại và `last_seen_at`. Không lưu địa chỉ IP của daemon trong `agent_runtime`. Không có cách vô hiệu hóa một runtime mà không xóa nó.

**Quy tắc.**
1. `GET /runtimes` (M41): danh sách toàn hệ thống (`system_admin`, `support`) hoặc trong tenant (`admin`); lọc `tenant_id`, `workspace_id`, `status`, `provider`, `disabled`; tìm `q` theo tên. Mỗi dòng: `id`, `name`, `workspace_id`, `tenant_id`, `status`, `last_seen_at`, `daemon_version` (`metadata.cli_version`), `agent_version` (`metadata.version`), `os` (rút từ `device_info`, không trả nguyên văn), `running_tasks` (số `agent_task_queue` `running` của runtime), `owner_id`, `disabled`.
2. **Riêng tư.** Không trả IP vì hệ thống không lưu; không trả `device_info` nguyên văn (có thể chứa tên máy cá nhân), chỉ hệ điều hành và kiến trúc; không trả `metadata` thô. Hệ quả nếu cần IP: GĐ22.
3. `GET /runtimes/{id}` (M41): thêm 20 task gần nhất trên runtime (`id`, `status`, thời điểm, mã lỗi nếu có, không nội dung), số task thất bại bảy ngày, và các dòng kiểm toán staff về runtime. "Lịch sử kết nối" là `last_seen_at` cộng các dòng đó; đây là phương án khuyến nghị (GĐ22).
4. **Vô hiệu hóa** (`POST /runtimes/{id}/status`, thân `{status: "disabled"|"enabled", reason}`, M42): ghi dòng `platform_runtime` (D98). Multica đọc bảng này qua cache Redis (cùng khuôn `workspacestate`) tại ba chỗ: `DaemonRegister` từ chối với 403 `runtime_disabled`; `requireDaemonRuntimeAccess` (nên cả heartbeat và mọi route daemon) từ chối cùng mã nên daemon dừng; và `claimRefusalFor` (`server/internal/service/budget_gate.go:320`) không giao task, task ở lại hàng đợi (cùng cơ chế D25). Platform xóa khóa cache và phát tin để ngắt kết nối đang mở. Task đang chạy hoàn tất theo cơ chế dừng đã có; không hủy task. **Kích hoạt lại** xóa trạng thái; runtime kết nối lại ở nhịp kế.
5. **Gỡ đăng ký** (`POST /runtimes/{id}/unregister`, thân `{reason, confirm}` với `confirm` là tên runtime, M43): yêu cầu `platform_deletion_request` `kind = runtime`; việc nền `staff_deletion` gọi đường xóa runtime đã có (`server/internal/handler/runtime.go:1337` `DeleteAgentRuntime`, không SQL xóa rời), cùng kiểm toán-trước như D73.
6. Quyền: `system_admin` mọi runtime; `admin` runtime thuộc workspace trong tenant mình; `support` chỉ đọc (M41); `billing` không. Runtime ngoài phạm vi trả 404.
7. Kiểm toán: `runtime.disabled`, `runtime.enabled`, `runtime.unregister_requested`, `runtime.unregistered`; mỗi dòng có `workspace_id`, `tenant_id`, `target_type = runtime`, `target_id`.

**Dữ liệu.** Bảng `platform_runtime` (`runtime_id`, `status`, `reason`, `changed_by`, `changed_at`), chỉ mục duy nhất `(runtime_id)`; không dòng nghĩa là cho phép. Platform ghi; Multica chỉ đọc. Khi runtime bị xóa, bước "delete runtimes and projects" của `deleteWorkspaceData` và `DeleteAgentRuntime` xóa dòng này (bản kê khai xóa workspace cập nhật cùng commit).

**Mã lỗi:** `runtime_disabled` 403 (trên API daemon của Multica), `runtime_not_found` (dùng 404 chuẩn). **Cấu hình:** không biến mới.

### 3b.6 Ma trận bổ sung (3c.10)

Các hàng M30 đến M52 nằm ở 3c.10 (vì gồm cả nhóm QT6 đến QT19). Quy tắc đọc giống 3.6.


## 3c. Nhóm QT6 đến QT19: đối chiếu với AWS, Azure, Google Cloud

### 3c.1 Cách đối chiếu và nguồn

Phần này đối chiếu mô hình tenant và workspace với cách AWS, Azure và Google Cloud tổ chức phân cấp, chính sách, hạn mức, chi phí và truy cập đặc quyền. Mỗi nhóm được chọn vì gắn với một rủi ro cụ thể của hệ thống này, không vì giống hyperscaler. Mức chắc chắn: **chính thức** là trang tài liệu đã mở lúc viết và ý được đọc trực tiếp trên trang; **thứ cấp** là ý chưa mở được trang chính thức, chỉ dùng làm gợi ý, không dùng làm căn cứ. Phần tương đương của hệ thống này là thiết kế của tài liệu, không phải tính năng của hyperscaler.

### 3c.2 Bảng đối chiếu với hyperscaler (D100)

| Tính năng hyperscaler | Tương đương trong hệ thống này | Trạng thái | Nguồn đã mở và mức chắc chắn |
| --- | --- | --- | --- |
| Phân cấp và chính sách kế thừa xuống cấp dưới mà chủ tài nguyên cấp dưới không đổi được (Azure management group, AWS SCP, GCP organization policy) | tenant là cấp trên, workspace là cấp dưới; chính sách tenant (QT6) kế thừa xuống workspace, workspace chỉ siết thêm | bổ sung (QT6) | Azure: `https://learn.microsoft.com/en-us/azure/governance/management-groups/overview` ("The resource or subscription owner can't alter this security policy"); AWS: `https://docs.aws.amazon.com/organizations/latest/userguide/orgs_manage_policies_scps.html` ("Any account has only those permissions permitted by every parent above it"); GCP: `https://docs.cloud.google.com/resource-manager/docs/cloud-platform-resource-hierarchy` ("effective policy ... results from policies applied directly to the resource and ... inherited from its ancestors"). Chính thức |
| Mỗi tài nguyên đúng một cha | mỗi workspace đúng một tenant (`tenant_workspace` duy nhất theo `workspace_id`, D64) | đã có | Azure (cùng trang trên: "Each management group and subscription can support only one parent"), GCP (cùng trang: "exactly one parent"). Chính thức |
| Guardrail không áp cho tài khoản quản lý gốc (SCP không ảnh hưởng management account) | chính sách tenant không giới hạn `system_admin` (staff không là thành viên) | đã có (hệ quả của D79) | AWS SCP (trang trên). Chính thức |
| Ngân sách và cảnh báo chi phí theo phạm vi; cảnh báo không dừng tài nguyên | cảnh báo ngưỡng 80 và 100 phần trăm của hạn mức theo workspace (QT7); hạn mức công nợ gói theo mức dùng đã có ở gốc 7b.3 | bổ sung (QT7) cho hạn mức tài nguyên; công nợ đã có | Azure: `https://learn.microsoft.com/en-us/azure/cost-management-billing/costs/tutorial-acm-create-budgets` ("Notifications are triggered when the budget thresholds are exceeded. Resources aren't affected"; đánh giá mỗi 24 giờ). Chính thức |
| Yêu cầu tăng hạn mức, nhỏ tự duyệt, lớn gửi người duyệt | tenant gửi yêu cầu, `system_admin` duyệt thủ công (QT7); không tự duyệt | bổ sung (QT7) | AWS: `https://docs.aws.amazon.com/servicequotas/latest/userguide/request-quota-increase.html` ("Smaller increases are usually automatically approved while larger requests are submitted to Support"); GCP: `https://docs.cloud.google.com/docs/quotas/view-manage` (yêu cầu điều chỉnh có xét duyệt; vai trò Quota Administrator). Chính thức |
| Truy cập đặc quyền theo thời hạn, cần phê duyệt, MFA, lý do, thông báo, rà soát quyền, không xóa quản trị viên cuối | duyệt chéo hai `system_admin` cho xóa ngay tenant và tài khoản (QT10); không xóa `system_admin` cuối đã có (D81); quyền có thời hạn và rà soát quyền thuộc nhóm sau này | bổ sung một phần (QT10); phần còn lại sau này (QT14) | Azure PIM: `https://learn.microsoft.com/en-us/entra/id-governance/privileged-identity-management/pim-configure` (time-bound, approval, MFA, justification, notifications, access reviews, "Prevent removal of the last active Global Administrator"). Chính thức |
| Giữ hồ sơ pháp lý, không hết hạn, độc lập với thời hạn lưu, gỡ tường minh | `legal_hold` trên tenant, workspace hoặc tài khoản chặn mọi đường xóa và việc xóa nhật ký theo thời hạn (QT11) | bổ sung (QT11) | AWS S3 Object Lock: `https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html` ("A legal hold ... has no expiration date ... remains in place until you explicitly remove it", "independent from retention periods"). Chính thức |
| Nhật ký hoạt động xuất ra nơi khác (diagnostic settings) | xuất kiểm toán theo khoảng thời gian bằng tệp hoặc API có phân trang (QT18); đẩy liên tục sang SIEM thuộc nhóm sau này | bổ sung (QT18) | Azure management group page ("enable diagnostic settings ... to send ... activity log entries to a Log Analytics workspace, Azure Storage, or Azure Event Hubs"). Chính thức |
| Nhãn (tag, label) để lọc và phân bổ chi phí | không có nhãn; tìm theo tên và slug | không áp dụng đợt này (QT8) | GCP hierarchy page ("labels that can be used for filtering projects"); Azure management group page (thẻ). Chính thức; áp dụng cho hệ này chưa có lý do cụ thể |
| Mạo danh có thời hạn và có nhật ký | không có (D10, I6); đường thay thế hiện hành là khách mời staff làm thành viên | không áp dụng đợt này (QT9) | Không mở được trang chính thức tương ứng. Thứ cấp, chưa xác nhận |
| Feature flag theo tenant và kill switch | cờ tính năng theo gói đã có (`features`); không cờ theo tenant | không áp dụng đợt này (QT12) | AWS Well-Architected SaaS lens (`https://wa.aws.amazon.com/saas.question.OPS_4.en.html`) chưa mở. Thứ cấp, chưa xác nhận |
| Cư trú dữ liệu theo vùng | một cơ sở dữ liệu chung (D2); D58 đã để ngoài R4 | không áp dụng đợt này (QT17) | Azure management group page chỉ nêu ví dụ chính sách giới hạn vùng tạo VM. Chính thức cho ví dụ, không cho khả năng chọn vùng của hệ này |
| Thông báo bảo trì và sự cố, lịch bảo trì | không có kênh; email vòng đời thuê bao đã có (gốc 5.9) | không áp dụng đợt này (QT13) | Không mở được trang chính thức. Thứ cấp, chưa xác nhận |
| Tín dụng, mã giảm giá, dùng thử theo tenant | dùng thử theo gói (`trial_days`) và `comp_reason` (gốc 7b.5) đã có | không áp dụng đợt này (QT15) | Không mở được trang chính thức. Thứ cấp, chưa xác nhận |
| Điểm sức khỏe tenant | không có | không áp dụng đợt này (QT16) | Không mở được trang chính thức. Thứ cấp, chưa xác nhận |
| Báo cáo tuân thủ chính sách (AWS Config, Azure Policy) | `GET /workspaces/{id}/policy` trả chính sách hiệu lực và nguồn của từng giá trị (QT6) thay cho báo cáo riêng | bổ sung một phần (QT6) | Azure Policy và AWS Config chưa mở. Thứ cấp, chưa xác nhận |
| Gắn vai trò ở cấp cao hơn thì kế thừa xuống (Azure RBAC, GCP IAM) | `system_admin` toàn nền tảng, `admin` theo tenant (D79, D80) | đã có | Azure management group page, GCP hierarchy page. Chính thức |

### 3c.3 Phân loại ưu tiên (D100)

| Nhóm | Mức | Lý do gắn với rủi ro của hệ thống này |
| --- | --- | --- |
| **QT11** giữ hồ sơ pháp lý | **bắt buộc** | D71 và D74 xóa không hoàn tác ngay; không có cách nào chặn việc xóa dữ liệu đang có yêu cầu pháp lý giữ lại, kể cả việc nền xóa sau 14 ngày và việc xóa nhật ký theo thời hạn. Thiếu thì rủi ro pháp lý |
| **QT6** chính sách tenant kế thừa xuống workspace | nên có | tenant có nhiều workspace (D64) nhưng K14 chỉ cho cấu hình theo từng workspace; owner của một workspace tắt MFA hay SSO thì tenant mất bảo đảm bảo mật đã chốt |
| **QT7** cảnh báo hạn mức và yêu cầu tăng hạn mức | nên có | `admin` không được tự tăng hạn mức (QT4) và chưa có đường chính thức để xin; khách chỉ biết bị chặn khi chạm 402 |
| **QT10** duyệt chéo thao tác không hoàn tác | nên có | một `system_admin` bị chiếm hay nhầm có thể xóa tenant hay tài khoản ngay với chỉ gõ lại tên (D72) |
| **QT18** xuất nhật ký kiểm toán | nên có | đòi hỏi pháp lý và điều tra sự cố cần xuất nhật ký; hiện chỉ có xuất CSV theo workspace cho owner (gốc 4.3); cũng cần để QT11 loại dòng bị giữ khỏi việc xóa theo thời hạn |
| **QT19** vòng đời tenant: khóa cả tenant, thông báo và xuất trước khi xóa | nên có | khóa một tenant nhiều workspace đòi nhiều thao tác và để hở workspace mới tạo; xóa ngay không báo owner và không cho họ lấy dữ liệu |
| QT8 nhãn | sau này | không rủi ro cụ thể; tìm theo tên và slug đủ với số tenant hiện nay |
| QT9 mạo danh cho support | sau này | đảo ngược I6 và D10 do chủ dự án chốt, cần quyết định riêng; đường thay thế (khách mời staff vào workspace) đang dùng được; phải có QT10, QT11, QT18 trước |
| QT12 cờ tính năng theo tenant | sau này | quyền theo gói đã ở `limits` và `features`; chặn một tenant đã có workspace `suspended`; chưa có nhu cầu rollout từng phần |
| QT13 thông báo và lịch bảo trì | sau này | cần hạ tầng trong ứng dụng chưa có; phần cần ngay (báo owner khi xóa ngay) nằm trong QT19 |
| QT14 rà soát quyền định kỳ | sau này | số staff nhỏ, MFA bắt buộc và quyền đọc lại ở mỗi request (gốc 5.2); cần QT18 để có dữ liệu rà soát |
| QT15 tín dụng, mã giảm giá | sau này | chưa thu tiền thật (D49); dùng thử và cấp miễn phí đã có |
| QT16 điểm sức khỏe tenant | sau này | chỉ suy ra từ dữ liệu QT4, QT7 sẵn có; chưa có yêu cầu hỗ trợ trong hệ thống để làm đầu vào |
| QT17 vùng dữ liệu | sau này | một cơ sở dữ liệu chung (D2) và D58 đã loại; cần hạ tầng nhiều vùng |

Việc xếp các nhóm QT8, QT9, QT12 đến QT17 vào "sau này" là phán đoán của tài liệu (GĐ26). Nhóm QT10 chỉ làm phần duyệt chéo; phần quyền có thời hạn kiểu PIM thuộc "sau này" (nhóm 10 trong yêu cầu gốc gồm cả hai, mục tiêu rủi ro là xóa nhầm, phần duyệt chéo đã giải quyết).

### 3c.4 QT6: chính sách tenant kế thừa xuống workspace (K36, D101)

**Hiện trạng.** Chính sách theo workspace (gốc 6.4, K14) gồm `AllowedInviteDomains`, `ShareLinksEnabled`, `RequireSSO`, `RequireMFA`, `SessionMaxAgeHours` (`server/internal/workspacepolicy/workspacepolicy.go:31`). Chính sách hiệu lực của một workspace tính ở `workspaceguard.EffectivePolicy` (`server/internal/workspaceguard/policy_access.go:143`); owner ghi bằng `PUT /api/workspaces/{id}/policy` (`server/internal/handler/workspace_policy.go:142`). Chưa có chính sách cấp tenant.

**Quy tắc.**
1. Bảng `tenant_policy` giữ **cùng năm trường** của chính sách workspace, làm "sàn": workspace không được lỏng hơn, chỉ được siết thêm.
2. **Hợp hai chính sách** (hiệu lực = chặt hơn): `AllowedInviteDomains` là giao của hai danh sách khi cả hai không rỗng, là danh sách không rỗng nếu chỉ một bên có; `ShareLinksEnabled` là `true` chỉ khi cả hai `true`; `RequireSSO` và `RequireMFA` là `true` nếu một trong hai `true`; `SessionMaxAgeHours` là giá trị nhỏ hơn khác 0 (0 là không giới hạn). Hàm thuần `workspacepolicy.Merge(tenant, workspace)`, một nơi duy nhất, gọi trong `EffectivePolicy`.
3. Ghi chính sách workspace lỏng hơn sàn: 400 `policy_below_tenant_floor` kèm `fields[]`. Chính sách workspace đã lưu từ trước mà lỏng hơn sàn mới vẫn lưu nguyên, nhưng bị `Merge` siết lúc cưỡng chế, nên không cần di dời dữ liệu; `GET /api/workspaces/{id}/policy` thêm trường tùy chọn `floor` (phần do tenant áp) để giao diện hiện lý do.
4. Tenant không có dòng `tenant_policy` thì không có sàn (I2). Chính sách tenant áp cho **mọi** workspace của tenant, không phụ thuộc gói của từng workspace, để một workspace gói thấp không là lỗ hổng của sàn. Việc **ghi** chính sách tenant đòi ít nhất một workspace của tenant có gói với tính năng `workspace_policies` (gốc 5.7), nếu không thì 402 `plan_feature_unavailable` với `feature = workspace_policies` (hệ quả ở GĐ24). Khi `MULTICA_CLOUD_URL` rỗng, tính năng coi như có, như mọi cổng tính năng.
5. `GET /tenants/{id}/policy` (M44) và `PUT /tenants/{id}/policy` (M45) cùng `expected_version`; ghi kiểm toán `tenant.policy_changed` (giá trị cũ và mới, `tenant_id`). Quyền: ghi `system_admin` và `admin` (tenant mình); đọc thêm `support`. Ví dụ: SSO bắt buộc ở sàn tenant thì owner workspace trong tenant vẫn đăng nhập bằng mã email để không tự khóa (D19, gốc 6.3) vì D19 ở tầng cưỡng chế.
6. Dữ liệu: bảng `tenant_policy` (`tenant_id`, `allowed_invite_domains`, `share_links_enabled`, `require_sso`, `require_mfa`, `session_max_age_hours`, `version`, `updated_by`, `updated_at`), chỉ mục duy nhất `(tenant_id)`; migration 1030, 1031. Multica ghi không; platform ghi, Multica đọc. Bước "detach tenant" xóa dòng khi tenant bị xóa.

**Mã lỗi:** `policy_below_tenant_floor` 400 (`fields[]`), `policy_version_conflict` 409. **Kiểm toán mới:** `tenant.policy_changed`.

### 3c.5 QT7: cảnh báo hạn mức và yêu cầu tăng hạn mức (K37, D102)

**Hiện trạng.** Hạn mức công nợ của gói theo mức dùng đã gửi email ở 80 phần trăm (gốc 7b.3, AC35). Với hạn mức đếm (agent, runtime, dung lượng, issue, lượt autopilot, nhóm), chỉ có 402 `plan_limit_reached` khi chạm giới hạn (gốc 5.7); không cảnh báo trước và không có đường xin tăng.

**Quy tắc.**
1. **Cảnh báo ngưỡng.** Việc nền `quota_alert` của platform (mỗi giờ) so mức dùng hiện tại của mỗi workspace với hạn mức hiệu lực ở bốn loại có số đếm hiện tại (agent, runtime, dung lượng, issue) và lượt autopilot trong chu kỳ; qua 80 phần trăm gửi email cảnh báo cho owner và admin của workspace, qua 100 phần trăm gửi một thư nữa (ngưỡng và nhịp cố định, GĐ31). Mỗi (workspace, loại, ngưỡng, chu kỳ) đúng một thư, dedupe bằng bảng `billing_notice` đã có (`server/migrations/684_billing_notice_key_uidx.up.sql`) với `kind = quota_80` hoặc `quota_100`. Cảnh báo **không** chặn hay đổi gì (theo mô hình ngân sách của Azure). Không gửi cho workspace `pending_deletion`.
2. **Yêu cầu tăng hạn mức.** `POST /api/workspaces/{id}/quota-requests` (Multica, owner hoặc người có `billing.manage`, người thật) và `POST /workspaces/{id}/quota-requests` (console: `system_admin`, `admin` trong tenant mình, M48) tạo yêu cầu với `gate`, `requested_value` (lớn hơn giá trị hiệu lực), `reason` (1 đến 500 ký tự). Mỗi (workspace, `gate`) có tối đa một yêu cầu `pending`.
3. **Duyệt.** `POST /quota-requests/{id}/approve` (thân `{value?, reason}`, có thể duyệt thấp hơn mức xin) và `/deny` (thân `{reason}`) chỉ `system_admin` (M47), không tự động. Duyệt ghi bằng cơ chế `limit_overrides` của QT4 cho đúng workspace, trong cùng giao dịch với việc đổi trạng thái yêu cầu và kiểm toán `quota.request_approved` (hoặc `quota.request_denied`). Người xin được email kết quả.
4. Xem danh sách: `GET /quota-requests`, `GET /quota-requests/{id}` (M46: `system_admin` mọi yêu cầu, `admin` yêu cầu của tenant mình, `billing` mọi yêu cầu, chỉ đọc).
5. Dữ liệu: bảng `quota_request` (`id`, `workspace_id`, `tenant_id`, `gate`, `current_value`, `requested_value`, `approved_value`, `state` (`pending`, `approved`, `denied`, `canceled`), `reason`, `decision_reason`, `requested_by`, `decided_by`, `created_at`, `decided_at`); chỉ mục duy nhất `(id)` và `(workspace_id, gate)` có điều kiện `state = 'pending'`; migration 1032 đến 1034. Platform sở hữu; Multica ghi dòng khi owner tạo yêu cầu (ngoại lệ về người ghi, bổ sung cho gốc 8).

**Mã lỗi:** `quota_request_invalid` 400 (`fields[]`), `quota_request_pending` 409, `quota_request_not_pending` 409 (`state`). **Kiểm toán mới:** `quota.request_created`, `quota.request_approved`, `quota.request_denied`.

### 3c.6 QT10: duyệt chéo cho thao tác không hoàn tác (K38, D103)

**Quy tắc.**
1. Cấu hình `PLATFORM_APPROVAL_ACTIONS`: danh sách hành động cần người thứ hai duyệt, thuộc `tenant.delete_now` và `account.delete_now`; **mặc định rỗng**, nên hành vi D71 và D74 không đổi (I2). Đây là điều khoản yêu cầu cho điểm chờ người theo AGENTS.md ("Automation First"): hành động không hoàn tác.
2. Khi hành động nằm trong danh sách, `POST /tenants/{id}/delete-now` và `POST /users/{id}/delete-now` chạy kiểm đầu vào như cũ (lý do, gõ lại, quyền, giữ hồ sơ pháp lý) rồi chỉ ghi dòng `platform_deletion_request` với `state = awaiting_approval` và kiểm toán `deletion.approval_requested`; **không** đặt trạng thái, không chặn truy cập, không xóa gì. Trả 202 `{request_id, state: "awaiting_approval"}`.
3. Một `system_admin` **khác** người yêu cầu duyệt bằng `POST /approvals/{id}/approve` hoặc từ chối bằng `/reject` (M49). Người duyệt trùng người yêu cầu: 409 `approver_must_differ`. Duyệt thì chạy pha 1 trong giao dịch duyệt (dùng `approved_by`, `approved_at`), rồi như D73. Từ chối đặt `canceled`.
4. Hết hạn sau 24 giờ không được duyệt: đặt `canceled` với lý do `approval_expired` (việc nền `deletion_approval_expiry` của platform, mỗi phút).
5. Chỉ có một `system_admin` hoạt động: yêu cầu bị từ chối ngay với 409 `approver_unavailable`, để không tạo yêu cầu không thể duyệt; khuyến nghị bật cấu hình khi có từ hai `system_admin` (GĐ25).
6. `admin` không tạo được hai hành động này (M24, M25) nên không có phần duyệt cho `admin`; xóa ngay **workspace** (M23) không qua duyệt chéo (hệ quả: GĐ25).
7. Dữ liệu: bảng `platform_deletion_request` (mục 8) thêm trạng thái `awaiting_approval` và các cột `approved_by`, `approved_at`; không bảng mới.

**Mã lỗi:** `approver_must_differ` 409, `approver_unavailable` 409, `approval_not_pending` 409 (`state`). **Kiểm toán mới:** `deletion.approval_requested`, `deletion.approved`, `deletion.rejected`, `deletion.approval_expired`.

### 3c.7 QT11: giữ hồ sơ pháp lý (K39, D104)

**Quy tắc.**
1. **Phạm vi giữ:** một tenant (giữ cả mọi workspace của nó, kể cả workspace thêm sau), một workspace, hoặc một tài khoản. Giữ **không có hạn**, đến khi gỡ tường minh (theo AWS Object Lock), và độc lập với mọi thời hạn khác (14 ngày chờ, hạn lưu nhật ký, hết kỳ thuê bao).
2. **Đặt và gỡ:** chỉ `system_admin`, `reason` và `case_ref` (mã vụ việc) bắt buộc, `POST /legal-holds` và `POST /legal-holds/{id}/release` (M50). Đặt hay gỡ ghi kiểm toán `legal_hold.placed` và `legal_hold.released` kèm `tenant_id`.
3. **Điều bị chặn** khi đối tượng đang bị giữ (kể cả khi giữ trên tenant của nó): (a) xóa ngay workspace, tenant, tài khoản của staff, ở pha 1 (409 `legal_hold`, không ghi gì) và ở pha 2; (b) xóa workspace do owner tự làm, cả xóa tức thì (`server/internal/handler/workspace.go:1436` `DeleteWorkspace`) lẫn việc nền `workspace_deletion`; (c) xóa tài khoản do việc nền `account_deletion`; (d) việc nền `audit_retention` không xóa dòng kiểm toán có `tenant_id` hoặc `workspace_id` thuộc đối tượng bị giữ (QT18). **Điểm chặn duy nhất** của (a) đến (c) nằm trong `deleteWorkspaceData` (`server/internal/handler/workspace.go:1567`) và `DeleteAccount` (`server/internal/handler/me_deletion.go:365`), đứng ở đầu hàm, để mọi đường hiện tại và tương lai đi qua cùng một kiểm tra; các kiểm tra ở pha 1 và ở handler chỉ để trả lỗi sớm.
4. **Không ảnh hưởng:** vận hành thường ngày (thành viên, dữ liệu, thuê bao, khóa, mở) vẫn chạy; việc giữ chỉ cấm xóa. Người dùng không bị khóa.
5. Workspace đang `pending_deletion` mà bị giữ: việc nền bỏ qua cho tới khi gỡ; không tự hủy lịch (gỡ giữ rồi việc nền chạy lại ở nhịp kế). Tài khoản một thành viên của workspace bị giữ: xóa tài khoản đòi xóa workspace, bị chặn, nên yêu cầu tài khoản `failed` với `error_code = legal_hold` và tài khoản giữ nguyên.
6. **Hiển thị:** `GET /legal-holds` (M50, chỉ `system_admin`) có `reason`, `case_ref`. `admin` chỉ thấy rằng đối tượng đang bị giữ qua lỗi 409 `legal_hold` (không có lý do, không có mã vụ việc) vì tính bảo mật của vụ việc (GĐ30). Trang chi tiết tenant và workspace của `system_admin` hiện dấu "đang giữ".
7. Dữ liệu: bảng `legal_hold` (`id`, `scope_kind` (`tenant`, `workspace`, `account`), `scope_id`, `reason`, `case_ref`, `placed_by`, `placed_at`, `released_by`, `released_at`); chỉ mục duy nhất `(id)` và `(scope_kind, scope_id)` có điều kiện `released_at IS NULL`; migration 1035 đến 1037. Platform ghi; Multica đọc. Nội dung `reason` và `case_ref` không vào `detail` kiểm toán (chỉ `legal_hold_id`).

**Mã lỗi:** `legal_hold` 409 (lỗi của mọi đường xóa; `legal_hold_id` chỉ cho `system_admin`), `legal_hold_exists` 409, `legal_hold_not_found` (404 chuẩn). **Kiểm toán mới:** `legal_hold.placed`, `legal_hold.released`, `deletion.blocked_by_hold` (ghi khi một đường xóa bị chặn).

### 3c.8 QT18: xuất nhật ký kiểm toán cho staff (K40, D105)

**Hiện trạng.** `GET /api/workspaces/{id}/audit-events?format=csv` cho owner và admin workspace (`server/internal/handler/audit_events.go:75`, gốc 4.3); staff đọc ở console có phân trang. Việc nền `audit_retention` xóa dòng cũ (`server/internal/scheduler/jobs_audit_retention.go:54`, truy vấn `DeleteAuditEventsBefore` ở `server/pkg/db/queries/audit_event.sql:38`). Tệp query của bảng chỉ có INSERT, SELECT, một DELETE theo ngày và hai UPDATE ẩn danh hóa (gốc 4.3, `platform-management/audit/queries_readonly_test.go`).

**Quy tắc.**
1. `GET /audit-events/export` (M51) xuất theo `from`, `to` (UTC, tối đa 31 ngày mỗi lần gọi), lọc `tenant_id`, `workspace_id`, `action`, `actor_id`, `format` (`ndjson` mặc định, `csv`); phân trang bằng `cursor`, tối đa 100.000 dòng mỗi trang, truyền dòng (stream) không giữ cả tập trong bộ nhớ (GĐ29). `system_admin` xuất toàn hệ thống; `admin` xuất dòng thuộc tenant mình (quy tắc D84, `tenant_id` bắt buộc hoặc ngầm là tenant mình); `support`, `billing` và vai trò khác nhận 404.
2. Mỗi lần xuất ghi `audit.exported` (bộ lọc, số dòng, người xuất, `tenant_id`); xuất cũng là hành động có nhật ký. Dữ liệu cá nhân (IP, email) có trong dòng như đã có ở đọc console; không có trường mới.
3. **Không sửa, không xóa:** không thêm truy vấn UPDATE hay DELETE nào ngoài ba cái đã có; test `queries_readonly_test.go` giữ nguyên. **Giữ lại theo hồ sơ pháp lý (QT11):** `DeleteAuditEventsBefore` thêm điều kiện không xóa dòng thuộc đối tượng bị giữ (dòng có `tenant_id`, hoặc `workspace_id` của workspace bị giữ hay thuộc tenant bị giữ).
4. Đẩy liên tục sang hệ ngoài (SIEM) không thuộc đợt này (mục 14).

**Mã lỗi:** `export_range_too_large` 400, `export_range_invalid` 400. **Kiểm toán mới:** `audit.exported`.

### 3c.9 QT19: vòng đời tenant, khóa cả tenant, báo owner và xuất trước khi xóa (K41, D106)

**Đã có:** xuất dữ liệu workspace (gốc 5.11, giữ tệp ở kho sau khi workspace bị xóa, `server/internal/handler/workspace.go:2007` bước "delete platform state" xóa dòng `workspace_export` nhưng không xóa tệp), khóa từng workspace (gốc 5.3), xóa có thời gian chờ 14 ngày kèm thông báo (gốc 5.12). **Chưa có:** khóa cả tenant, ngăn tạo workspace mới trong tenant bị khóa, báo owner khi xóa ngay, và xuất dữ liệu trước khi xóa ngay.

**Quy tắc.**
1. **Trạng thái tenant** (cột mới `tenant.status`: `active`, `read_only`, `suspended`; `tenant.status_reason`, `status_changed_at`, `status_changed_by`). `POST /tenants/{id}/status` thân `{status, reason}` (M52; `system_admin`, `support`, `admin` trong tenant mình, đúng quyền khóa workspace M10): trong một giao dịch ghi `platform_workspace` cho **mọi workspace** của tenant với `reason = tenant_status` và kiểm toán một dòng cho tenant (`tenant.status_changed`, có `tenant_id`) cộng các dòng `workspace.status_changed` như thường lệ. Mở khóa (`active`) chỉ khôi phục workspace có `reason = tenant_status`; workspace đã bị khóa riêng (lý do khác) giữ nguyên. Khóa tenant không đặt `pending_deletion` (xóa dùng đường D71).
2. **Workspace mới trong tenant không `active`:** `CreateWorkspace` với `tenant_id` của tenant `read_only` hoặc `suspended` trả 409 `tenant_not_active` (đọc dưới khóa tenant ở 5.7). Tenant không có trạng thái riêng là phát biểu của 5.1 trước đây; từ nay tenant có `status` chỉ dùng cho hành động khóa cả tenant và cho kiểm tra trên, còn trạng thái cưỡng chế vẫn ở `platform_workspace` (guard không đổi).
3. **Hết hạn thanh toán không áp ở cấp tenant (GĐ27):** thuê bao ở từng workspace (GĐ1), nên chuyển `read_only` khi hết kỳ vẫn theo vòng đời từng workspace (gốc 5.9). Hệ quả nếu sai: phương án B của 5.3.
4. **Báo owner khi xóa ngay.** Pha 1 của xóa ngay workspace, tenant và tài khoản (mục 6, 7) ghi thêm một việc gửi email cho mọi owner của workspace bị xóa (người dùng bị xóa không nhận): nội dung nêu workspace, thời điểm dự kiến, và nếu có `export_first` thì cách tải dữ liệu. Gửi qua `internal/mailer` của platform (D34). Gửi lỗi không chặn việc xóa (ghi `deletion.notice_failed`).
5. **Xuất trước khi xóa.** Thân `delete-now` nhận `export_first` (mặc định `false`; áp cho workspace và tenant). Khi `true`: pha 1 chèn dòng `workspace_export` `queued` cho từng workspace; pha 2 chờ cả bản xuất `done` (tối đa 24 giờ) rồi gửi cho owner một email có liên kết tải có chữ ký, hạn 7 ngày (cùng cơ chế gốc 5.11; tệp ở kho sau khi workspace bị xóa), rồi mới xóa. Quá 24 giờ chưa xong: yêu cầu `failed` với `error_code = export_timeout`, staff thử lại hoặc hủy hoặc đặt lại không kèm xuất (một yêu cầu mới, có lý do). Giữ hồ sơ pháp lý chạy trước (QT11).
6. Dữ liệu: cột mới trên `tenant` (nằm trong migration 1016, chưa áp dụng, nên không migration thêm); cột `export_first` và `notice_sent_at` trên `platform_deletion_request` (migration 1025, chưa áp dụng).

**Mã lỗi:** `tenant_not_active` 409 (Multica, trên `POST /api/workspaces`), `export_timeout` là `error_code` của yêu cầu. **Kiểm toán mới:** `tenant.status_changed`, `deletion.notice_sent`, `deletion.notice_failed`, `deletion.export_ready`.

### 3c.10 Ma trận bổ sung M30 đến M52 (D83, mở rộng 3.6)

Quy ước như 3.6: cột `admin` là "trong tenant của mình".

| # | Hành động (route dưới `/api/platform` hoặc Multica nếu ghi) | `system_admin` | `admin` | `support` | `billing` | Nhóm |
| --- | --- | --- | --- | --- | --- | --- |
| M30 | Xem gói (`GET /plans`, `GET /plans/{key}`) | mọi | chỉ đọc (danh mục gói không thuộc tenant) | không | không | QT1 |
| M31 | Tạo, sửa, bật, tắt, xóa gói (`POST`, `PUT`, `DELETE /plans…`, `/activate`, `/deactivate`) | mọi | không | không | không | QT1 |
| M32 | Lịch sử thay đổi gói (`GET /plans/{key}/history`) | mọi | không | không | mọi | QT1 |
| M33 | Danh sách workspace (`GET /workspaces`) | mọi | workspace trong tenant mình | mọi | mọi | QT2 |
| M34 | Yêu cầu xuất dữ liệu workspace (`POST /workspaces/{id}/export`) | mọi | workspace trong tenant mình | không | không | QT2 |
| M35 | Người dùng của tenant (`GET /tenants/{id}/users`, `…/{user_id}`) | mọi | tenant mình | mọi | mọi | QT3 |
| M36 | Gỡ người dùng khỏi workspace hoặc khỏi cả tenant (`POST /workspaces/{id}/members/{user_id}/remove`, `POST /tenants/{id}/users/{user_id}/deactivate`) | mọi | tenant mình | không | không | QT3 |
| M37 | Đổi vai trò thành viên (`POST /workspaces/{id}/members/{user_id}/role`) | mọi | tenant mình (không đổi của chính mình, không đổi `owner`) | không | không | QT3 |
| M38 | Tài nguyên của tenant (`GET /tenants/{id}/resources`) | mọi | tenant mình | mọi | mọi | QT4 |
| M39 | Tài nguyên toàn hệ thống (`GET /resources`) | mọi | không | không | không | QT4 |
| M40 | Hạn mức bổ sung cho tenant (`POST /tenants/{id}/limit-overrides`) | mọi | không (xin tăng ở M48) | không | mọi (như M12) | QT4 |
| M41 | Xem runtime (`GET /runtimes`, `GET /runtimes/{id}`) | mọi | runtime trong tenant mình | mọi (chỉ đọc) | không | QT5 |
| M42 | Vô hiệu hóa, kích hoạt runtime (`POST /runtimes/{id}/status`) | mọi | tenant mình | không | không | QT5 |
| M43 | Gỡ đăng ký runtime (`POST /runtimes/{id}/unregister`) | mọi | tenant mình | không | không | QT5 |
| M44 | Xem chính sách tenant (`GET /tenants/{id}/policy`) | mọi | tenant mình | mọi | không | QT6 |
| M45 | Sửa chính sách tenant (`PUT /tenants/{id}/policy`) | mọi | tenant mình | không | không | QT6 |
| M46 | Xem yêu cầu tăng hạn mức (`GET /quota-requests`, `…/{id}`) | mọi | tenant mình | không | mọi | QT7 |
| M47 | Duyệt hoặc từ chối yêu cầu tăng hạn mức (`POST /quota-requests/{id}/approve`, `/deny`) | mọi | không | không | không | QT7 |
| M48 | Tạo yêu cầu tăng hạn mức thay khách (`POST /workspaces/{id}/quota-requests`) | mọi | tenant mình | không | không | QT7 |
| M49 | Duyệt chéo (`GET /approvals`, `POST /approvals/{id}/approve`, `/reject`) | mọi, người duyệt khác người yêu cầu | không | không | không | QT10 |
| M50 | Giữ hồ sơ pháp lý: đặt, gỡ, xem (`POST /legal-holds`, `…/release`, `GET /legal-holds`) | mọi | không (chỉ biết "đang giữ" qua lỗi 409) | không | không | QT11 |
| M51 | Xuất kiểm toán (`GET /audit-events/export`) | toàn bộ | chỉ dòng của tenant mình | không | không | QT18 |
| M52 | Khóa, mở cả tenant (`POST /tenants/{id}/status`) | mọi | tenant mình | mọi | không | QT19 |


## 4. Nhóm 1: thêm người vào workspace có sẵn (K27, D60 đến D63)

### 4.1 Mục tiêu và người dùng

- **Staff `system_admin`**: khi khách nhờ, thêm người của họ vào bất kỳ workspace nào mà không chờ lời mời được chấp nhận.
- **Staff `admin`**: làm việc tương tự nhưng chỉ cho workspace thuộc tenant mình được gán (3.5).
- **Chủ workspace**: thấy rõ ai đã thêm ai, vào lúc nào, qua nhật ký kiểm toán của mình (gốc 5.2: hành động của staff ghi kèm `workspace_id` nên chủ workspace thấy được).
- Staff không thành thành viên của workspace và không đọc được nội dung (I6, D10).

### 4.2 Quy tắc

1. Chỉ staff `system_admin` (mọi workspace) và `admin` (workspace trong tenant của mình) có MFA (gốc 6.2) gọi được route thêm người (ma trận 3.6, M21 và M22). `support`, `billing` và người không có phiên nhận 404 phẳng (gốc 5.2). Với `admin`, workspace ngoài tenant mình nhận 404 có thân giống workspace không tồn tại (3.5 quy tắc 1), kiểm **trước** mọi kiểm tra khác nên không lộ trạng thái hay sự tồn tại.
2. Đầu vào: `email` (chuẩn hóa chữ thường, tối đa 254 ký tự, đúng định dạng) và `role` là vai trò **thành viên** của workspace (`admin`, `member`, `viewer`; không phải vai trò staff). Sai thì 400 `member_request_invalid` kèm `fields[]`; vai trò ngoài danh sách (kể cả `owner`) thì 400 `member_role_invalid` (D63).
3. Workspace phải tồn tại và có trạng thái `active` (gốc 5.3); nếu không thì 409 `workspace_not_active` kèm `status`. Lý do: `read_only`, `suspended`, `pending_deletion` đều không nhận thành viên mới qua đường lời mời.
4. Email của staff đang hoạt động bị từ chối với 409 `staff_cannot_be_added` (D63). Email đã là thành viên của workspace thì 409 `already_member`.
5. Mỗi cặp (workspace, email) có tối đa một yêu cầu đang sống (`pending`). Gửi lại khi còn sống trả chính yêu cầu đó với 200.
6. **Tìm hoặc tạo người dùng (việc nền, pha 2).** Kiểm lại email chưa là thành viên và tài khoản không bị `auth.IsTemporarilyDisabledUserEmail` (cùng kiểm tra ở `server/internal/handler/auth.go:222`). Người dùng có sẵn mà `platform_account.status` là `suspended` hoặc `pending_deletion` thì yêu cầu `failed` với `error_code = account_not_active`. Người dùng chưa có thì tạo với tên theo GĐ12, kiểm miền thuộc `SIGNUP_BLOCKED_EMAIL_DOMAINS` (yêu cầu `failed`, `email_domain_blocked`) nhưng **không** kiểm `ALLOW_SIGNUP` (như gốc 7b.5 quy tắc 8), rồi đánh dấu đã onboard bằng `MarkUserOnboarded` như `AcceptInvitation` (`server/internal/handler/invitation.go:705`) để họ vào thẳng workspace. Không mật khẩu, không phiên, không mã đăng nhập tạo sẵn; người dùng đăng nhập bằng mã email như mọi người và vẫn qua `consent_required` (gốc 4.4).
7. **Ghế (D62).** Việc nền thực hiện đúng thứ tự: `ClaimShareJoin(ws, token = id yêu cầu)`; `Managed = false` thì bỏ qua ghế; `Allowed = false` thì yêu cầu `failed` với `error_code = capacity_full`; lỗi thỏa `IsCapacityOvercommitted` thì `failed` với `capacity_overcommitted`; lỗi truyền tải thì thử lại, hết 3 lần thì `failed` với `seat_capacity_unavailable`. Sau khi tạo `member` (cùng cách `CreateMember` ở `server/internal/handler/invitation.go:663`) và commit, gọi `Confirm(ws, token, memberID)`. Mọi nhánh `failed` sau `ClaimShareJoin` gọi `Release(ws, token)`.
8. Hủy: chỉ yêu cầu `pending` hủy được (409 `member_request_not_cancelable` kèm `state`). Việc hủy gọi `Release(ws, token)` ở dịch vụ ghế của chính platform; lệnh lũy đẳng và không làm gì khi chưa có thao tác ghế (Phụ lục B quy tắc 2).
9. Sau commit, việc nền xóa cache thành viên của người được thêm và phát sự kiện `member:added` (`server/pkg/protocol/events.go:62`) như `AcceptInvitation` (`server/internal/handler/invitation.go:748`), để owner và các thành viên thấy ngay.
10. **Lời mời giữ nguyên** (D61): owner và admin workspace vẫn mời bằng đường lời mời với giao thức ghế `ReserveInvitation`/`Consume`; không route lời mời nào đổi hành vi. Quy tắc 2 của gốc 7b.5 vẫn đúng cho `POST /api/platform/accounts`: route đó chỉ tạo người dùng và workspace mới.

### 4.3 Dữ liệu

`platform_member_request`: `id`, `workspace_id`, `kind` (`add` mặc định, `remove`, `change_role`; QT3, D94), `target_user_id` (có với `remove` và `change_role`), `previous_role` (có với `change_role`), `email`, `role`, `state` (`pending`, `completed`, `failed`, `canceled`), `error_code`, `attempts`, `user_id`, `member_id`, `user_created` (boolean: người dùng do yêu cầu này tạo), `requested_by`, `requested_by_label` (email staff tại lúc tạo), `created_at`, `updated_at`, `completed_at`. Chỉ mục duy nhất `(id)`; chỉ mục duy nhất `(workspace_id, kind, email)` có điều kiện `state = 'pending'` (với `remove` và `change_role`, `email` là email của người đích, lấy lúc tạo yêu cầu). Migration ở mục 8.

Ngoại lệ về người ghi (bổ sung cho gốc 8): Multica ghi các cột `state`, `error_code`, `attempts`, `user_id`, `member_id`, `user_created`, `updated_at`, `completed_at` ở pha 2; mọi cột khác do platform ghi. Cột `user_id` là cột người dùng, đăng ký `keep` trong sổ đăng ký xóa tài khoản (gốc 4.6); cột `email` được ẩn danh hóa khi người đó bị xóa, theo khuôn `AnonymizeAccountRequestsForUser`. Khi workspace bị xóa, các dòng yêu cầu của nó bị xóa trong bước "delete platform state" của `deleteWorkspaceData`.

### 4.4 API và mã lỗi

| Route (console, dưới `/api/platform`) | Vai trò | Nội dung |
| --- | --- | --- |
| `POST /workspaces/{id}/members` | `system_admin`; `admin` (tenant mình) | tạo yêu cầu; 202 `{id, state}`; 200 khi trả yêu cầu đang sống |
| `GET /workspaces/{id}/member-requests`, `GET /member-requests/{id}` | `system_admin`; `admin` (tenant mình) | danh sách và chi tiết, kèm `error_code` |
| `POST /member-requests/{id}/cancel` | `system_admin`; `admin` (tenant mình) | quy tắc 8 |

Mã lỗi: `member_request_invalid` 400 (`fields[]`), `member_role_invalid` 400, `workspace_not_active` 409 (`status`), `staff_cannot_be_added` 409, `already_member` 409, `member_request_not_cancelable` 409 (`state`). Giá trị `error_code` của yêu cầu `failed`: `already_member`, `account_not_active`, `email_domain_blocked`, `capacity_full`, `capacity_overcommitted`, `seat_capacity_unavailable`, `workspace_not_active`, `internal`.

### 4.5 Kiểm toán

| Hành động | Actor | `workspace_id` | Khi nào |
| --- | --- | --- | --- |
| `member.staff_add_requested` | `staff` | có | pha 1, cùng giao dịch với dòng yêu cầu; nhãn đích là email; ghi `tenant_id` của tenant chứa workspace (D84) |
| `member.staff_added` | `staff` (id và nhãn lấy từ `requested_by`, `requested_by_label`) | có | pha 2, cùng giao dịch với dòng `member`; `detail` ghi `role`, `request_id`, `user_created` |
| `member.staff_add_failed` | `system` | có | khi yêu cầu chuyển `failed`; `detail` ghi `error_code` |
| `member.staff_add_canceled` | `staff` | có | khi hủy |

Hành động `member.staff_added` đặt `actor_type = staff` đúng như yêu cầu của chủ dự án; actor là staff đã tạo yêu cầu (`system_admin` hoặc `admin`), không phải tiến trình nền. Cả bốn hành động ghi `tenant_id` để `admin` của tenant đọc được (D84).

### 4.6 Bảo mật

| Mối nguy | Biện pháp |
| --- | --- |
| Staff tự thêm mình hoặc đồng nghiệp để đọc dữ liệu khách (phá I6, D10) | Email của staff đang hoạt động bị từ chối (quy tắc 4); test AC63 |
| Thêm nhầm người lạ vào workspace khách | Chỉ `system_admin` và `admin` (trong tenant mình) có MFA; kiểm toán `member.staff_added` hiện trong nhật ký của chủ workspace; sự kiện `member:added` phát ngay; owner gỡ được bằng đường gỡ thành viên hiện có |
| Nhét `owner` mới để chiếm workspace | Vai trò `owner` không có trong danh sách (D63) |
| Vượt ghế bằng cách đua | `ClaimShareJoin` lũy đẳng và tuần tự hóa theo workspace ở platform (Phụ lục B quy tắc 6); chỉ mục duy nhất của yêu cầu đang sống |
| Dữ liệu cá nhân của người thứ ba | Dòng yêu cầu chứa email; ẩn danh hóa khi người đó bị xóa, xóa cùng workspace |
| `admin` thêm người vào workspace của tenant khác bằng cách đoán id | Phạm vi kiểm ở handler và ở truy vấn; id ngoài tenant trả 404 giống id không tồn tại (3.5); test AC93 |

## 5. Nhóm 2: tenant có nhiều workspace (K26, D64 đến D70, D77)

### 5.1 Định nghĩa tenant

**Tenant** = một thực thể bền chứa một hoặc nhiều workspace, bảng `tenant`. Mỗi workspace thuộc đúng một tenant, qua bảng liên kết `tenant_workspace`. Thuê bao, ghế, đơn hàng, trạng thái và số liệu sử dụng vẫn thuộc từng workspace (D65). Tenant mang ba thứ: định danh, tên, và là phạm vi của cổng `max_workspaces`. Trạng thái cưỡng chế luôn ở từng workspace (`platform_workspace`); trạng thái hiển thị của tenant là tổng hợp trạng thái các workspace của nó (mục 5.8). Tenant có thêm một cột `status` chỉ để ghi hành động khóa cả tenant và để chặn tạo workspace mới vào tenant bị khóa (3c.9, D106).

Không thêm cột vào `workspace` (giữ D6, giảm xung đột khi nhập mã từ upstream). Quan hệ `tenant_workspace` → `tenant` và → `workspace` không có khóa ngoại và không cascade; mã kiểm tra (mục 5.9, D68).

### 5.2 Dữ liệu

- `tenant`: `id`, `name`, `created_by` (có thể rỗng cho tenant tạo từ migration), `status` (`active` mặc định, `read_only`, `suspended`; chỉ do hành động khóa cả tenant ghi, 3c.9), `status_reason`, `status_changed_at`, `status_changed_by`, `created_at`, `updated_at`. Chỉ mục duy nhất `(id)`.
- `tenant_workspace`: `workspace_id`, `tenant_id`, `created_at`. Chỉ mục duy nhất `(workspace_id)` (mỗi workspace một tenant); chỉ mục `(tenant_id)`.

Hai bảng do Multica sở hữu (vòng đời workspace). Platform đọc chúng (console, chính sách quyền lợi) và chỉ ghi qua lệnh `platform-backfill` (mục 5.3). Vì platform đọc, các phiên bản migration của hai bảng vào danh sách của `platform-management/internal/schemacheck` (gốc 5.1).

### 5.3 Phương án gắn thuê bao

Chủ dự án yêu cầu nêu các phương án. Có ba:

| | A. Thuê bao ở từng workspace (khuyến nghị) | B. Thuê bao ở tenant | C. Thuê bao ở một workspace "neo" |
| --- | --- | --- | --- |
| Mô tả | Giữ `workspace_subscription` khóa theo `workspace_id`. Tenant lấy `max_workspaces` lớn nhất trong các workspace của nó | Thêm `tenant_subscription`; workspace thừa hưởng gói, ghế và trạng thái từ tenant | Thuê bao nằm ở workspace đầu tiên; các workspace khác thừa hưởng từ nó |
| Đơn vị trả tiền | mỗi workspace một thuê bao, một đơn | một thuê bao cho cả tenant | một thuê bao, nhưng gắn vào một workspace cụ thể |
| Ghế (Phụ lục B) | không đổi: sức chứa và số đã dùng tính theo workspace | phải định nghĩa lại: ghế dùng chung hay theo workspace; người ở hai workspace có tính hai ghế không | không đổi cho workspace neo, phải định nghĩa cho các workspace khác |
| Hợp đồng ngoài (Phụ lục A, B, chính sách quyền lợi, đều khóa theo workspace, D35) | không đổi | platform tự phân giải workspace sang tenant; hợp đồng vẫn khóa theo workspace nhưng mọi truy vấn của platform đổi | không đổi, nhưng phải phân giải ngầm |
| Phạm vi sửa mã | nhỏ: chỉ thêm cổng và bảng tenant | lớn: ghế, đơn hàng, vòng đời, số liệu dùng, gói theo mức dùng, hồ sơ thanh toán (D39, 7b.1) | trung bình, nhưng xóa workspace neo phải chuyển neo |
| Điểm yếu | khách muốn trả một lần cho cả tenant thì phải mua theo từng workspace | thay đổi lớn, đụng mọi thứ R2 và R5 đã xây | xóa hoặc khóa workspace neo làm cả tenant mất gói; mong manh |

**Khuyến nghị: A.** Lý do: (1) mọi hợp đồng ngoài và mọi bảng thương mại đã khóa theo workspace (D35, D39), nên A là phương án duy nhất không đổi chúng; (2) cổng `max_workspaces` chỉ cần một giá trị cho cả tenant, mà "giá trị lớn nhất" cho kết quả xác định và không phụ thuộc workspace nào là đầu tiên; (3) C có một điểm hỏng đã biết (xóa workspace neo) và B đụng quá nhiều thứ cho một đợt chỉ thêm quyền admin.

**Giả định GĐ1.** Hệ quả nếu sai: khách muốn một khoản thanh toán cho cả tenant. Khi đó chuyển sang B. Vì mọi tenant bắt đầu một workspace (mục 5.4), việc chuyển là cơ học cho tenant một workspace; với tenant đã có nhiều workspace gói khác nhau, cần staff chọn gói gộp trước. Không có dữ liệu nào mất.

**Cách A hoạt động.** Giới hạn hiệu lực của workspace là `limits` của gói ghi đè bởi `limit_overrides` (gốc 5.5), workspace không có dòng thuê bao thì dùng gói `free`. Giới hạn `max_workspaces` của tenant bằng giá trị lớn nhất trong các giá trị hiệu lực của mọi workspace thuộc tenant; nếu có một giá trị rỗng (không giới hạn) thì tenant không giới hạn. Platform trả giá trị này trong cổng `max_workspaces` của chính sách quyền lợi của **mọi** workspace thuộc tenant (mục 5.7), nên Multica không có client mới (D35).

### 5.4 Migrate dữ liệu hiện có (D69)

Mỗi workspace hiện có thành một tenant chỉ có một workspace.

**Thứ tự migration** (chi tiết số và tên tệp ở mục 8):

1. Tạo bảng `tenant` (1016) và chỉ mục duy nhất (1017); tạo bảng `tenant_workspace` (1018) và hai chỉ mục (1019, 1020). Chỉ mục tạo `CONCURRENTLY`, mỗi chỉ mục một tệp.
2. Migration 1021 chép dữ liệu: với mỗi workspace chưa có dòng trong `tenant_workspace`, chèn một dòng `tenant` có `id = workspace.id`, `name = workspace.name`, `created_at = workspace.created_at`, rồi chèn dòng `tenant_workspace` `(workspace.id, workspace.id)`.
3. Migration 941 thêm `max_workspaces` vào `limits` của các gói đang có (D70), chỉ khi khóa chưa có.
4. Triển khai mã mới của Multica và platform.
5. Chạy `platform-backfill tenants`: cùng điều kiện như bước 2, để vá workspace do máy chủ cũ tạo trong khoảng giữa bước 2 và bước 4. Console `/overview` hiển thị số workspace thiếu tenant (kỳ vọng 0).

**Điều kiện idempotent.** Bước 2 và bước 5 dùng cùng điều kiện `WHERE NOT EXISTS (SELECT 1 FROM tenant_workspace WHERE workspace_id = w.id)` và `ON CONFLICT DO NOTHING` trên hai chỉ mục duy nhất đã tạo ở bước 1; chạy lại không tạo dòng thứ hai. Dùng `tenant.id = workspace.id` để khóa tự nhiên trùng với khóa của workspace, nên hai lần chạy đồng thời cũng không tạo hai tenant cho một workspace (chỉ mục duy nhất `(workspace_id)` và `(id)` chặn). Migration 941 dùng `limits = jsonb_build_object('max_workspaces', <giá trị>) || limits` có điều kiện `NOT (limits ? 'max_workspaces')`, nên khóa đã có (do staff đặt) không bị ghi đè.

**Thứ tự áp dụng trên cơ sở dữ liệu mới.** Trình chạy sắp tệp theo chuỗi (`server/internal/migrations/migrations.go:69`), nên số bốn chữ số đứng trước mọi số ba chữ số từ `101` trở lên. Hệ quả: các migration 1016 đến 1037 chỉ được tham chiếu bảng tạo ở tệp số nhỏ hơn 100 (`workspace` ở `001_init`); migration 941 chạy trước 953 trên cơ sở dữ liệu mới, nên chỉ được dùng cột của `plan` có từ migration 670 (`limits`, `key`), không dùng `version` (thêm ở 953). Trên cơ sở dữ liệu mới, bảng `plan` rỗng lúc migration chạy, nên giá trị ban đầu đến từ `catalogue.json` (mục 5.6).

**Hoàn nguyên.** Bảng chỉ thêm; mã cũ bỏ qua chúng. Mỗi migration có tệp `down`.

### 5.5 Thay đổi 5.3 (trạng thái workspace)

- Tenant không có dòng `platform_workspace` riêng; trạng thái cưỡng chế luôn thuộc workspace (hành động khóa cả tenant ghi từng workspace, 3c.9). Staff khóa, mở, lên lịch xóa từng workspace như cũ (gốc 5.3, 5.4).
- Cột `deletion_scheduled_at` của `pending_deletion` có thể bằng hoặc trước hiện tại khi staff xóa ngay (mục 6.3). `reason` ghi `staff_immediate_deletion`. Với mọi guard và danh sách cho phép của gốc 5.3 nó vẫn là `pending_deletion`, nên truy cập bị chặn như thường.
- Workspace đang `pending_deletion` do chủ workspace đặt (14 ngày) mà staff có quyền xóa ngay (mục 6.2 quy tắc 1) xóa ngay: pha 1 kéo `deletion_scheduled_at` về hiện tại thay vì bỏ qua vì trạng thái không đổi.

### 5.6 Thay đổi 5.5 (gói và thuê bao)

- `limits` của gói nhận thêm khóa `max_workspaces`: số nguyên không nhỏ hơn 1, hoặc rỗng (không giới hạn). Giá trị 0 hoặc âm bị `PUT /plans` từ chối bằng cùng lỗi 400 hiện có cho giới hạn không hợp lệ. Thêm vào `plan.LimitGates` (`platform-management/internal/plan/plan.go:81`) và `plan.CountedGates`, để gói `unlimited` vẫn không giới hạn mọi hạn mức đếm (D42).
- `limit_overrides` của workspace ghi đè được `max_workspaces`.
- Danh mục khởi tạo nhận thêm hàng sau (D70, GĐ6), ghi vào `platform-management/internal/plan/catalogue.json` và vào migration 941:

| | Free | Pro | Business | Enterprise | Unlimited | Payg |
| --- | --- | --- | --- | --- | --- | --- |
| `max_workspaces` | 1 | 3 | 10 | không giới hạn | không giới hạn | 5 |

- Chính sách quyền lợi (`platform-management/internal/entitlement/policy.go:134`, `planLimitGates`) thêm `max_workspaces` vào danh sách cổng giới hạn; platform tính giá trị theo tenant (mục 5.3, "Cách A hoạt động") và đưa vào chính sách của mọi workspace của tenant. Workspace chưa có dòng `tenant_workspace` (chưa backfill) nhận cổng `off`.
- Độ trễ: chính sách bị cache tối đa 30 giây ở platform và `valid_for_seconds` (tối đa 300 giây) ở client (gốc 5.5). Đổi gói của một workspace khiến giới hạn của các workspace anh em trễ tối đa khoảng 5,5 phút.

### 5.7 Thay đổi 5.7 (cổng hạn mức)

Thêm một hàng vào bảng cổng đếm của gốc 5.7:

| Cổng | Điểm cưỡng chế | Cách đếm |
| --- | --- | --- |
| `max_workspaces` | `CreateWorkspace` khi có `tenant_id` | số dòng `tenant_workspace` của tenant |

**Cách chạy** (D66, D67). Trong giao dịch tạo workspace của `CreateWorkspace` (`server/internal/handler/workspace.go:405` mở giao dịch):

1. Phân giải `tenant_id`: người gọi phải là `owner` của ít nhất một workspace thuộc tenant, nếu không thì 404 `tenant_not_found` (không phân biệt "không có" với "không phải của bạn").
2. Lấy khóa tư vấn bằng `LockPlanGate` với `Scope` là id tenant và `Gate = max_workspaces`. Khóa này giữ cho cả cổng đếm lẫn mọi thao tác thêm hoặc bớt workspace của tenant (D68), kể cả khi cổng không bị cưỡng chế.
3. Nếu tenant có yêu cầu xóa ngay chưa xong thì 409 `tenant_deleting` (mục 6.4).
4. Chính sách được đọc **trước** giao dịch (là lời gọi mạng) bằng `h.resolvePlanGate` trên một workspace của tenant mà người gọi làm owner (cổng nằm trong chính sách của mọi workspace của tenant, nên workspace nào cũng được; chọn cái cũ nhất để xác định). Rồi `Policy.Admit` (`server/internal/planlimit/planlimit.go:101`) với `Scope` là id tenant và `count` là số dòng `tenant_workspace` của tenant. Tham số `workspaceID` của `Admit` chỉ dùng làm `Scope`, nên truyền id tenant.
5. Vượt thì 402, mã `plan_limit_reached`, `gate = max_workspaces`, `limit`, `used`. Giới hạn 0 không xảy ra vì mục 5.6 cấm.
6. Tạo workspace bằng `workspaceprovision.Create` có `TenantID`, cùng giao dịch.

Hai request đồng thời ở chỗ cuối cùng xếp hàng trên khóa; request thứ hai đếm cả workspace mà request thứ nhất đã commit, nên chỉ một cái thành công (AC70).

**Không có `tenant_id`.** Hành vi hiện tại giữ nguyên: `Create` tạo tenant mới có `id` bằng id workspace và một dòng `tenant_workspace`. Không có cổng `max_workspaces` vì một tenant mới có đúng một workspace. `FREE_PLAN_OWNED_WORKSPACE_LIMIT` (gốc 5.7, `server/internal/planlimit/owner.go:17`) giữ nguyên và chạy độc lập với cổng này, trước nó: kiểm tra đó đếm workspace gói miễn phí theo người sở hữu, còn `max_workspaces` đếm theo tenant.

**Khi không có nguồn thương mại** (`MULTICA_CLOUD_URL` rỗng hoặc trỏ tới Multica Cloud): cổng `max_workspaces` vắng mặt trong chính sách nghĩa là `off`; tenant vẫn tạo được nhiều workspace không giới hạn (I2, I9).

**Hạ gói và quá hạn.** Chỉ chặn tạo workspace mới; không workspace nào bị khóa hay xóa vì tenant vượt giới hạn (I10).

### 5.8 Thay đổi console `/tenants` (D77)

`GET /api/platform/tenants` và `GET /api/platform/tenants/{id}` (`platform-management/internal/console/handler.go:120`, `:121`) trả tenant thật.

- **Phạm vi theo vai trò (D80):** `system_admin`, `support`, `billing` thấy mọi tenant. `admin` dùng cùng route nhưng chỉ thấy tenant của mình: `GET /tenants` trả đúng một dòng (hoặc không dòng nếu tenant đã bị xóa), `GET /tenants/{id}` với tenant khác trả 404 như tenant không tồn tại. Tìm kiếm của `admin` không vượt ra ngoài tenant mình.
- **Danh sách:** tìm theo tên tenant, tên hoặc slug workspace, email owner. Mỗi dòng: `id`, `name`, `workspace_count`, `max_workspaces` (hiệu lực, rỗng là không giới hạn), `best_plan_key`, `status_summary` (đếm workspace theo trạng thái), `created_at`, hoạt động gần nhất.
- **Chi tiết:** tenant kèm `workspaces[]`, mỗi phần tử là tóm tắt hiện có của một workspace (gói, trạng thái, số thành viên, owner). Không có route trả issue, comment, chat, tệp, cấu hình agent hay bí mật (I6; test `TestConsoleFilesAreScannedAndNameNoContentTable`, `platform-management/internal/console/isolation_test.go:149`).
- **Route cấp workspace chuyển sang `/workspaces/{id}/…`:** `GET /workspaces/{id}` (chi tiết cũ của `/tenants/{id}`), `POST /workspaces/{id}/status`, `GET /workspaces/{id}/usage`, `GET /workspaces/{id}/audit-events`, `GET` và `POST /workspaces/{id}/notes`, `PUT /workspaces/{id}/subscription`, và nhóm `billing-profile` (`GET`, `PUT /workspaces/{id}/billing-profile`, `POST …/verify`, `POST …/unverify`, hiện ở `platform-management/internal/subscription/console.go`). Vai trò theo ma trận 3.6 (với `support` và `billing` giữ nguyên như gốc; `admin` chỉ ở các dòng M8 đến M11). Đường cũ `/tenants/{id}/status` và các đường cùng nhóm trả 404; không có bí danh (D77).
- Bảng route ở gốc 5.4 và 9 được đọc theo bảng đổi tên này.

### 5.9 Tạo workspace và tenant cùng nhau

`workspaceprovision.Create` nhận `TenantID` tùy chọn. Rỗng thì tạo dòng `tenant` (`id` bằng id workspace mới, `name` bằng tên workspace, `created_by` bằng owner) và dòng `tenant_workspace`; có thì chỉ chèn dòng `tenant_workspace` sau khi `CreateWorkspace` đã kiểm. Việc nền `account_provisioning` (gốc 7b.5, bước 2) gọi cùng hàm nên mỗi tài khoản do admin tạo cũng có tenant. Bước "detach tenant" của `deleteWorkspaceData` (D68): lấy cùng khóa tư vấn, xóa dòng `tenant_workspace` của workspace, nếu tenant không còn dòng nào thì xóa dòng `tenant`; không có khóa ngoại nên chỉ làm bằng mã này. Phản hồi của `POST /api/workspaces` và `GET /api/workspaces/{id}` thêm trường tùy chọn `tenant_id` (I9).

## 6. Nhóm 3: staff xóa ngay workspace hoặc tenant (K28, D71 đến D73, D85)

### 6.1 Mục tiêu và người dùng

Staff xóa ngay một workspace hoặc cả tenant khi có yêu cầu của khách hoặc yêu cầu pháp lý, không chờ 14 ngày. `system_admin` xóa ngay mọi workspace và mọi tenant; `admin` xóa ngay workspace trong tenant mình, không xóa tenant (D85, GĐ14). `support` và `billing` không xóa ngay; quyền khóa, mở và lên lịch xóa của họ giữ như gốc 5.2 (`support` khóa và mở; lên lịch xóa chỉ `system_admin` và `admin`).

### 6.2 Quy tắc

1. Xóa ngay workspace: `system_admin` (mọi workspace) hoặc `admin` (workspace trong tenant mình), có MFA. Xóa ngay tenant: chỉ `system_admin`; `admin` nhận 404 phẳng kể cả với tenant của mình. `support`, `billing` nhận 404 phẳng. Workspace ngoài tenant của `admin` nhận 404 giống workspace không tồn tại (3.5).
2. Thân yêu cầu có `reason` (không rỗng sau khi cắt khoảng trắng, tối đa 500 ký tự; rỗng thì 400 `reason_required`) và `confirm`. `confirm` phải bằng slug của workspace, hoặc tên của tenant (D72, GĐ10); sai thì 400 `confirmation_mismatch`. Máy chủ kiểm cả hai trước mọi ghi.
3. Chỉ có đúng một yêu cầu đang sống cho một đích (`pending`, `running` hoặc `failed`). Gửi lại trả yêu cầu đó với 200.
4. **Đường xóa hiện có (D73).** Pha 2 gọi `deleteWorkspaceData` (`server/internal/handler/workspace.go:1567`) qua `DeleteDueWorkspace` (`server/internal/handler/workspace_deletion.go:170`), cùng bước dọn Qoder, cùng bản kê khai bảng. Không có truy vấn `DELETE` mới cho dữ liệu workspace ở platform hay ở việc nền mới. Test AC79 khẳng định.
5. **Xóa tenant** là xóa tuần tự từng workspace của tenant bằng chính đường ở quy tắc 4, mỗi workspace một giao dịch; dòng `tenant` biến mất khi workspace cuối bị xóa (D68). Platform liệt kê workspace sẽ bị xóa ở pha 1 (kèm số thành viên) và ghi chúng vào `detail` của kiểm toán.
6. Hủy: `system_admin` hủy hay thử lại mọi yêu cầu; `admin` chỉ hủy hay thử lại yêu cầu `kind = workspace` do chính mình tạo và có đích trong tenant mình (M28), nên `admin` không hủy được một yêu cầu do `system_admin` tạo trên workspace của tenant mình. Yêu cầu `pending` hoặc `failed` hủy được, khi đó workspace trở về `active` (qua `workspacestatus` với kiểm toán `workspace.status_changed`) và dòng yêu cầu `canceled`. Yêu cầu đang `running` hay `completed` thì 409 `deletion_request_not_cancelable`. Thử lại yêu cầu `failed` đặt nó về `pending` với `attempts = 0`.

### 6.3 Hai pha

**Pha 1 (platform, một giao dịch)**, theo thứ tự:

1. Lấy khóa tư vấn (tenant, `max_workspaces`) nếu đích là tenant; liệt kê workspace.
2. Chèn dòng `platform_deletion_request` (`kind = workspace` hoặc `tenant`, `target_id`, `target_label` là slug hoặc tên, `reason`, `state = pending`, `requested_by`, `requested_by_label`).
3. **Ghi kiểm toán** `workspace.immediate_deletion_requested` (cho mỗi workspace, có `workspace_id`) và, với tenant, `tenant.immediate_deletion_requested` (không có `workspace_id`; `detail` liệt kê workspace). `actor_type = staff`, `detail` có `reason` và `request_id`.
4. Đặt mỗi workspace `pending_deletion` với `reason = staff_immediate_deletion` và `deletion_scheduled_at = now()`, kéo về hiện tại nếu đã có hẹn.

Commit. Sau commit, xóa khóa cache `mul:plat:ws:<id>` của từng workspace (gốc 5.3). Từ đây truy cập vào workspace trả 403 `workspace_pending_deletion` và agent ngừng nhận task, tức là xóa "ngay" theo nghĩa chặn truy cập (GĐ8).

**Pha 2 (Multica, việc nền `staff_deletion`).** Lấy yêu cầu `pending` đổi thành `running`; với mỗi workspace gọi đường xóa hiện có với `DeletionOrigin = staff` (kiểm toán `workspace.deleted` có `actor_type = staff`, `detail` có `request_id` và `origin = staff`); với tenant xong thì ghi `tenant.deleted`; đặt `completed`. Nếu một workspace lỗi: giao dịch của workspace đó hủy (đường xóa là một giao dịch), yêu cầu thử lại tối đa 3 lần, rồi `failed` với `error_code`; kiểm toán của pha 1 và các workspace đã xóa xong giữ nguyên. Việc nền sẵn có `workspace_deletion` bỏ qua workspace có yêu cầu `pending`, `running` hoặc `failed` (mục 3.2), nên staff quyết định hủy hay thử lại.

**Kiểm toán sau khi workspace bị xóa.** `DetachWorkspaceAuditEvents` (`server/pkg/db/queries/workspace_delete.sql:777`) đặt `workspace_id` của các dòng thành NULL; dòng vẫn còn với `target_id`, `target_label` và `detail`, đọc được ở `GET /api/platform/audit-events` toàn hệ thống. Chủ workspace không còn đọc được vì workspace không còn.

### 6.4 Tạo workspace vào tenant đang bị xóa

`CreateWorkspace` với `tenant_id` mà tenant có yêu cầu xóa ngay chưa xong (`pending`, `running`, `failed`) trả 409 `tenant_deleting`, đọc dưới khóa tư vấn của mục 5.7. Pha 1 giữ cùng khóa khi liệt kê workspace, nên một workspace mới hoặc nằm trong danh sách xóa hoặc bị từ chối, không có chỗ hở.

### 6.5 API và mã lỗi

| Route (console, dưới `/api/platform`) | Vai trò | Nội dung |
| --- | --- | --- |
| `POST /workspaces/{id}/delete-now` | `system_admin`; `admin` (tenant mình) | thân `{reason, confirm}`; 202 `{request_id, state}` |
| `POST /tenants/{id}/delete-now` | `system_admin` | như trên; `confirm` là tên tenant |
| `GET /deletion-requests`, `GET /deletion-requests/{id}` | `system_admin`; `admin` (M27) | lọc theo `kind`, `state`; kèm `error_code`, `attempts`; `admin` chỉ thấy yêu cầu `workspace` và `tenant` của tenant mình |
| `POST /deletion-requests/{id}/cancel`, `POST /deletion-requests/{id}/retry` | `system_admin`; `admin` (M28) | quy tắc 6 |

Mã lỗi: `reason_required` 400, `confirmation_mismatch` 400, `deletion_request_not_cancelable` 409 (`state`), `deletion_request_not_retryable` 409 (`state`); ở Multica: `tenant_deleting` 409.

### 6.6 Kiểm toán

`workspace.immediate_deletion_requested`, `tenant.immediate_deletion_requested` (pha 1, `staff`; mọi dòng ghi `tenant_id` của tenant chứa đích, D84, để `admin` của tenant vẫn đọc được sau khi workspace bị xóa); `workspace.deleted` (đã có, `staff`, `detail.origin = staff`), `tenant.deleted` (pha 2); `deletion.request_failed` (`system`), `deletion.request_canceled`, `deletion.request_retried` (`staff`). Hằng tên thêm vào `platform-management/audit/audit.go` và danh mục của `platform-management/audit/catalog_test.go`.

## 7. Nhóm 4: `system_admin` xóa ngay tài khoản người dùng (K29, D74 đến D76, D86)

### 7.1 Mục tiêu và người dùng

Staff `system_admin` xóa ngay tài khoản khi có yêu cầu pháp lý hoặc theo đề nghị của chủ tài khoản đã xác minh ngoài kênh, không chờ 14 ngày. `admin` không có quyền này (D86, GĐ15). Dùng cùng pipeline ẩn danh hóa và gỡ thành viên của gốc 4.6: gỡ tư cách thành viên bằng `revokeAndRemoveMember`, xóa workspace một thành viên bằng đường xóa workspace, thu hồi PAT và token, xóa kết nối và ràng buộc kênh, ẩn danh hóa bản ghi `user`, đặt `status = deleted`.

### 7.2 Quy tắc

1. Chỉ `system_admin` (có MFA). `admin`, `support`, `billing` và người không có phiên nhận 404 phẳng, kể cả khi người dùng chỉ thuộc tenant của `admin` (M25).
2. `reason` và `confirm` như mục 6.2; với tài khoản, `confirm` là email của người dùng (GĐ10).
3. Từ chối 409 `user_is_staff` nếu người dùng có dòng `platform_staff` đang hoạt động, và 409 `cannot_delete_self` nếu là chính `system_admin` đang thao tác (D76, GĐ9). Quy tắc này áp cho mọi vai trò staff của người bị xóa (`system_admin`, `admin`, `support`, `billing`).
4. Tài khoản đã `deleted` thì 404; đã `pending_deletion` do tự phục vụ thì pha 1 kéo `deletion_scheduled_at` về hiện tại.

### 7.3 Owner duy nhất của workspace còn thành viên khác (D75)

Platform tính, bằng số owner thật của từng workspace, tập **cần xử lý**: các workspace mà người dùng là `owner` duy nhất và còn thành viên khác. Workspace có đồng owner không cần xử lý (người dùng chỉ bị gỡ). Workspace chỉ có người dùng là thành viên thì bị xóa cùng tài khoản như gốc 4.6, không cần chọn.

Với mỗi workspace cần xử lý, thân yêu cầu phải có một phần tử trong `resolutions[]`:

- **(A) chuyển quyền:** `{workspace_id, action: "transfer", successor_user_id}`. Người nhận phải thỏa GĐ11; sai thì 400 `successor_invalid` kèm `workspace_id`.
- **(B) xóa workspace ngay:** `{workspace_id, action: "delete"}`; đi theo đúng đường của mục 6 (D71).

Thiếu bất kỳ workspace cần xử lý nào thì 409 `sole_owner_unresolved` kèm `workspaces[]` (cùng dạng `sole_owner` ở gốc 10) và không ghi gì. Phần tử thừa (workspace không thuộc tập cần xử lý) thì 400 `successor_invalid`.

**Vì sao không dùng lại `Classify`.** `accountdeletion.Classify` (`platform-management/accountdeletion/rules.go`) chặn mọi owner của workspace còn thành viên khác, kể cả khi có đồng owner, chặt hơn gốc 4.6. Đường staff dùng số owner thật; đường tự phục vụ không đổi (mục 15.3).

### 7.4 Hai pha

**Pha 1 (platform, một giao dịch).** Kiểm đầu vào và quy tắc 7.3; chèn dòng `platform_deletion_request` `kind = account` (`target_id` là id người dùng, `target_label` là email, `resolutions` là JSONB); với mỗi lựa chọn (B) chèn một dòng yêu cầu con `kind = workspace` có `parent_id` và làm pha 1 của mục 6.3 cho workspace đó; ghi kiểm toán `account.immediate_deletion_requested` (`staff`, đích là email, `detail` có `reason`, `request_id`, các lựa chọn); đặt `platform_account.status = pending_deletion` với `deletion_scheduled_at = now()` và `sessions_valid_after = now()`. Commit, rồi xóa cache guard và phát tin `mul:acct:changed` (gốc 4.1); từ đây mọi token của người đó bị chặn và kết nối bị ngắt.

**Pha 2 (Multica, việc nền `staff_deletion`).**

1. Yêu cầu con (B) chạy trước theo mục 6.3. Yêu cầu tài khoản chỉ xử lý khi mọi yêu cầu con `completed`; có con `failed` thì đặt `failed` với `error_code = child_failed`.
2. Với mỗi lựa chọn (A), trong một giao dịch: kiểm lại người nhận vẫn là thành viên thỏa GĐ11, đặt vai trò `owner` bằng `UpdateMemberRole` (cùng truy vấn như `server/internal/handler/workspace.go:923`) và ghi `member.role_changed` với `actor_type = staff`, `detail` có `request_id`. Không còn thỏa thì dừng với `error_code = sole_owner_unresolved`.
3. Kiểm lại: không còn workspace nào mà người dùng là owner duy nhất và còn thành viên khác. Còn thì `failed` với `sole_owner_unresolved`; **tài khoản không bị xóa** và chốt `server/internal/handler/me_deletion.go:474` vẫn đứng sau như lớp bảo vệ cuối.
4. Gọi `DeleteAccount` (`server/internal/handler/me_deletion.go:365`) với `DeletionOrigin = staff`: giữ nguyên toàn bộ pipeline; chỉ khác là điều kiện "đã đến hạn" thỏa vì `deletion_scheduled_at <= now()`. Các dòng kiểm toán `member.left` và `workspace.deleted` sinh ra có `actor_type = staff` và `detail.origin = staff`.
5. Ghi `account.deleted` (`staff`), đặt `target_label` của yêu cầu thành `deleted user` (không giữ email), đặt `completed`.

Việc nền sẵn có `account_deletion` bỏ qua tài khoản có yêu cầu `pending`, `running` hoặc `failed` (mục 3.2). Cùng cách hủy và thử lại như mục 6.2 quy tắc 6; hủy một yêu cầu tài khoản đặt tài khoản về `active` và hủy các yêu cầu con còn `pending`.

### 7.5 API và mã lỗi

| Route (console, dưới `/api/platform`) | Vai trò | Nội dung |
| --- | --- | --- |
| `POST /users/{id}/delete-now` | `system_admin` | thân `{reason, confirm, resolutions[]}`; 202 `{request_id, state}` |
| `GET /users/{id}/delete-now-preview` | `system_admin` | trả tập workspace cần xử lý, workspace sẽ bị xóa cùng, và số thành viên; không ghi gì. Console dùng để dựng biểu mẫu |

Cùng nhóm `GET /deletion-requests`, `…/cancel`, `…/retry` của mục 6.5. Mã lỗi: `sole_owner_unresolved` 409 (`workspaces[]`), `successor_invalid` 400 (`workspace_id`), `user_is_staff` 409, `cannot_delete_self` 409, và các mã của mục 6.5.

### 7.6 Kiểm toán

`account.immediate_deletion_requested` (pha 1, `staff`, trước mọi xóa); `member.role_changed` (chuyển quyền, `staff`); `workspace.immediate_deletion_requested` và `workspace.deleted` cho lựa chọn (B); `member.left` và `workspace.deleted` do `DeleteAccount` sinh với `actor_type = staff`; `account.deleted` (`staff`). Sau khi xóa, các bước "anonymize audit actor" và "anonymize audit target" của `DeleteAccount` (`server/internal/handler/me_deletion.go:438`) thay nhãn email trong các dòng của người đó bằng `deleted user`; `detail` không chứa email.

## 8. Mô hình dữ liệu và migration của R6

Quy tắc chung như gốc 8 và I3: không khóa ngoại, không cascade, không `PRIMARY KEY` hay `UNIQUE` nội tuyến; tính duy nhất do chỉ mục `UNIQUE` tạo riêng; mọi chỉ mục tạo bằng `CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS`, mỗi chỉ mục một tệp một câu lệnh, có tệp `down` và một dòng trong `concurrentIndexCleanups` (`server/cmd/migrate/main.go:142`). Theo gốc 8, Postgres, Redis và chuỗi migration duy nhất ở `server/migrations/`.

| Bảng | Chủ sở hữu | Chỉ mục duy nhất | Chỉ mục khác |
| --- | --- | --- | --- |
| `tenant` | Multica | `(id)` | |
| `tenant_workspace` | Multica | `(workspace_id)` | `(tenant_id)` |
| `platform_member_request` | platform (Multica ghi một số cột, mục 4.3) | `(id)`; `(workspace_id, kind, email)` có điều kiện `state = 'pending'` | |
| `platform_deletion_request` | platform (Multica ghi một số cột) | `(id)`; `(kind, target_id)` có điều kiện `state IN ('awaiting_approval', 'pending', 'running', 'failed')` | |
| `platform_runtime` | platform (Multica đọc) | `(runtime_id)` | |
| `tenant_policy` | platform (Multica đọc) | `(tenant_id)` | |
| `quota_request` | platform (Multica ghi dòng khi owner tạo yêu cầu) | `(id)`; `(workspace_id, gate)` có điều kiện `state = 'pending'` | |
| `legal_hold` | platform (Multica đọc) | `(id)`; `(scope_kind, scope_id)` có điều kiện `released_at IS NULL` | |

**Cột của `platform_deletion_request`:** `id`, `kind` (`workspace`, `tenant`, `account`, `runtime`), `target_id`, `target_label`, `reason`, `state` (`awaiting_approval`, `pending`, `running`, `completed`, `failed`, `canceled`), `error_code`, `attempts`, `approved_by`, `approved_at`, `export_first` (boolean, QT19), `notice_sent_at` (QT19), `resolutions` (JSONB, rỗng cho `workspace` và `tenant`), `parent_id` (yêu cầu cha, rỗng nếu không phải con), `requested_by`, `requested_by_label`, `created_at`, `updated_at`, `completed_at`. Multica ghi `state`, `error_code`, `attempts`, `target_label` (ẩn danh hóa email khi xong), `updated_at`, `completed_at` ở pha 2; mọi cột khác do platform ghi. Dòng của workspace hoặc tenant đã xóa giữ lại làm lịch sử (nhãn là slug hoặc tên); dòng `account` mất email khi hoàn tất.

**Sổ đăng ký cột người dùng (gốc 4.6).** Cột UUID khớp mẫu của test sổ đăng ký trong các bảng mới: `tenant.created_by` (`keep`), `platform_member_request.user_id` (`keep`, `email` ẩn danh hóa). Các cột `requested_by` và `target_id` không khớp mẫu của test nhưng được ghi chú ở sổ cùng chỗ để người đọc sau không bỏ sót.

**Thay đổi cột trên bảng đã có (R6):** `platform_staff` thêm `tenant_id uuid` rỗng được (mục 3.7); `audit_event` thêm `tenant_id uuid` rỗng được (D84). Cả hai không khóa ngoại, không cascade. `platform_staff.tenant_id` không phải cột người dùng nên không vào sổ đăng ký cột người dùng.

**Đánh số migration.** Số bốn chữ số `1014` và `1015` đã dành cho R4 (`docs/engineering/platform-management-spec.md:1154`); R6 dùng **1016 đến 1037** cho bảng mới, và các số ba chữ số **941 đến 947** cho thay đổi bảng đã có ở tệp số ba chữ số (`plan` ở 670, `platform_staff` ở 663, `audit_event` ở 654).

| Migration | Nội dung | Slice |
| --- | --- | --- |
| 941 | thêm `max_workspaces` vào `plan.limits` theo D70, có điều kiện khóa chưa có | S1 |
| 942 | thêm cột `platform_staff.tenant_id` | S0 |
| 943 | chỉ mục `platform_staff (tenant_id)` có điều kiện `revoked_at IS NULL AND tenant_id IS NOT NULL` | S0 |
| 944 | dữ liệu: `admin` cũ (không có tenant) thành `system_admin`, lũy đẳng | S0 |
| 945 | thêm cột `audit_event.tenant_id` | S0 |
| 946 | chỉ mục `audit_event (tenant_id, created_at DESC, id)` có điều kiện `tenant_id IS NOT NULL` | S0 |
| 947 | thêm cột `plan.description` (QT1) | S16 |
| 1016 | bảng `tenant` | S1 |
| 1017 | chỉ mục duy nhất `tenant (id)` | S1 |
| 1018 | bảng `tenant_workspace` | S1 |
| 1019 | chỉ mục duy nhất `tenant_workspace (workspace_id)` | S1 |
| 1020 | chỉ mục `tenant_workspace (tenant_id)` | S1 |
| 1021 | chép dữ liệu: mỗi workspace chưa có liên kết thành một tenant và một dòng `tenant_workspace` | S1 |
| 1022 | bảng `platform_member_request` | S7 |
| 1023 | chỉ mục duy nhất `platform_member_request (id)` | S7 |
| 1024 | chỉ mục duy nhất `platform_member_request (workspace_id, email)` có điều kiện `pending` | S7 |
| 1025 | bảng `platform_deletion_request` | S9 |
| 1026 | chỉ mục duy nhất `platform_deletion_request (id)` | S9 |
| 1027 | chỉ mục duy nhất `platform_deletion_request (kind, target_id)` có điều kiện đang sống | S9 |
| 1028 | bảng `platform_runtime` (QT5) | S21 |
| 1029 | chỉ mục duy nhất `platform_runtime (runtime_id)` | S21 |
| 1030 | bảng `tenant_policy` (QT6) | S29 |
| 1031 | chỉ mục duy nhất `tenant_policy (tenant_id)` | S29 |
| 1032 | bảng `quota_request` (QT7) | S31 |
| 1033 | chỉ mục duy nhất `quota_request (id)` | S31 |
| 1034 | chỉ mục duy nhất `quota_request (workspace_id, gate)` có điều kiện `pending` | S31 |
| 1035 | bảng `legal_hold` (QT11) | S26 |
| 1036 | chỉ mục duy nhất `legal_hold (id)` | S26 |
| 1037 | chỉ mục duy nhất `legal_hold (scope_kind, scope_id)` có điều kiện chưa gỡ | S26 |

Dòng dọn trong `concurrentIndexCleanups` (khóa là phiên bản, giá trị là tên chỉ mục): `943_platform_staff_tenant_idx`, `946_audit_event_tenant_created_idx`, `1029_platform_runtime_uidx`, `1031_tenant_policy_uidx`, `1033_quota_request_id_uidx`, `1034_quota_request_live_uidx`, `1036_legal_hold_id_uidx`, `1037_legal_hold_live_uidx`, `1017_tenant_id_uidx`, `1019_tenant_workspace_ws_uidx`, `1020_tenant_workspace_tenant_idx`, `1023_platform_member_request_id_uidx`, `1024_platform_member_request_live_uidx`, `1026_platform_deletion_request_id_uidx`, `1027_platform_deletion_request_live_uidx`.

**Danh sách `internal/schemacheck`.** Platform đọc hoặc ghi cả bốn bảng, nên thêm vào danh sách (platform đọc và ghi cả `platform_staff` lẫn `audit_event`, nên cả `942` đến `946` cũng vào; danh sách hiện có `969_billing_order_usage_expiry` ở `platform-management/internal/schemacheck/schemacheck.go:76` cho R5 và `1013_einvoice_issuance_due_idx` ở `:26` cho R4): `941_plan_max_workspaces`, `947_plan_description`, `942_platform_staff_tenant`, `943_platform_staff_tenant_idx`, `944_platform_staff_system_admin`, `945_audit_event_tenant`, `946_audit_event_tenant_created_idx`, `1016_tenant`, `1017_tenant_id_uidx`, `1018_tenant_workspace`, `1019_tenant_workspace_ws_uidx`, `1020_tenant_workspace_tenant_idx`, `1021_tenant_backfill`, `1022_platform_member_request`, `1023_platform_member_request_id_uidx`, `1024_platform_member_request_live_uidx`, `1025_platform_deletion_request`, `1026_platform_deletion_request_id_uidx`, `1027_platform_deletion_request_live_uidx`, `1028_platform_runtime`, `1029_platform_runtime_uidx`, `1030_tenant_policy`, `1031_tenant_policy_uidx`, `1032_quota_request`, `1033_quota_request_id_uidx`, `1034_quota_request_live_uidx`, `1035_legal_hold`, `1036_legal_hold_id_uidx`, `1037_legal_hold_live_uidx`. Mỗi slice thêm phiên bản của nó trong cùng commit (kế hoạch bổ sung, quy ước chung).

**Bản kê khai xóa workspace.** Bước "detach tenant" xử lý `tenant` và `tenant_workspace`; bước "delete platform state" thêm `platform_member_request` theo `workspace_id`; `platform_deletion_request` và `legal_hold` được `keep`; `platform_runtime` xóa theo runtime và `tenant_policy` xóa ở bước "detach tenant"; `quota_request` xóa theo `workspace_id` ở bước "delete platform state". Cập nhật `server/internal/handler/workspace_delete_manifest_test.go` trong cùng commit.

## 9. API (bổ sung cho gốc 9)

Mọi route ở bảng này của platform nằm dưới `/api/platform`, sau `RequirePlatformStaff` (gốc 5.2): không đạt thì 404. Cột quyền là tóm tắt; nguồn duy nhất là ma trận 3.6 (D83). "`admin`" ở cột này là `admin` trong tenant của mình.

| Route | Quyền | Mục |
| --- | --- | --- |
| `GET /tenants`, `GET /tenants/{id}` | mọi staff; `admin` chỉ thấy tenant của mình | 5.8 (đổi nghĩa) |
| `GET /staff`, `POST /staff`, `PUT /staff/{user_id}`, `DELETE /staff/{user_id}` | `system_admin` (`PUT` mới; ba route kia đã có ở gốc 5.4) | 3.7 |
| `GET /me` | mọi staff; thêm trường `tenant_id` | 3.5 |
| `GET /plans/{key}`, `POST /plans/{key}/activate`, `POST /plans/{key}/deactivate`, `GET /plans/{key}/history` | đọc: `system_admin`, `admin` (một gói), `history`: `system_admin`, `billing`; ghi: `system_admin` (M30 đến M32) | 3b.1 |
| `GET /workspaces`, `POST /workspaces/{id}/export` | M33, M34 | 3b.2 |
| `GET /tenants/{id}/users`, `GET /tenants/{id}/users/{user_id}`, `POST /tenants/{id}/users/{user_id}/deactivate`, `POST /workspaces/{id}/members/{user_id}/remove`, `POST /workspaces/{id}/members/{user_id}/role` | M35 đến M37 | 3b.3 |
| `GET /tenants/{id}/resources`, `GET /resources`, `POST /tenants/{id}/limit-overrides` | M38 đến M40 | 3b.4 |
| `GET /runtimes`, `GET /runtimes/{id}`, `POST /runtimes/{id}/status`, `POST /runtimes/{id}/unregister` | M41 đến M43 | 3b.5 |
| `GET /tenants/{id}/policy`, `PUT /tenants/{id}/policy` | M44, M45 | 3c.4 |
| `GET /quota-requests`, `GET /quota-requests/{id}`, `POST /quota-requests/{id}/approve`, `POST /quota-requests/{id}/deny`, `POST /workspaces/{id}/quota-requests` | M46 đến M48 | 3c.5 |
| `GET /approvals`, `POST /approvals/{id}/approve`, `POST /approvals/{id}/reject` | M49 | 3c.6 |
| `GET /legal-holds`, `POST /legal-holds`, `POST /legal-holds/{id}/release` | M50 | 3c.7 |
| `GET /audit-events/export` | M51 | 3c.8 |
| `POST /tenants/{id}/status` | M52 | 3c.9 |
| `GET /workspaces/{id}`, `POST /workspaces/{id}/status`, `GET /workspaces/{id}/usage`, `GET /workspaces/{id}/audit-events`, `GET` và `POST /workspaces/{id}/notes` | như route cũ tương ứng, thêm `admin` (workspace trong tenant mình) theo M8 đến M11 | 5.8 (đổi tên) |
| `PUT /workspaces/{id}/subscription` | `billing`, `system_admin` | 5.8 (đổi tên) |
| `POST /workspaces/{id}/members`, `GET /workspaces/{id}/member-requests`, `GET /member-requests/{id}`, `POST /member-requests/{id}/cancel` | `system_admin`, `admin` | 4.4 |
| `POST /workspaces/{id}/delete-now` | `system_admin`, `admin` | 6.5 |
| `POST /tenants/{id}/delete-now` | `system_admin` | 6.5 |
| `POST /users/{id}/delete-now`, `GET /users/{id}/delete-now-preview` | `system_admin` | 7.5 |
| `GET /deletion-requests`, `GET /deletion-requests/{id}`, `POST /deletion-requests/{id}/cancel`, `POST /deletion-requests/{id}/retry` | `system_admin`, `admin` (phạm vi hẹp ở M27, M28) | 6.5 |

Route của máy chủ Multica mới của các nhóm QT: `POST /api/workspaces/{id}/quota-requests` và `GET /api/workspaces/{id}/quota-requests` (3c.5, chỉ owner và người có `billing.manage`, người thật); `GET /api/workspaces/{id}/policy` thêm trường tùy chọn `floor` (3c.4). Route của máy chủ Multica: `POST /api/workspaces` (`server/cmd/server/router.go:1928`) nhận thêm `tenant_id` tùy chọn; `POST /api/workspaces` và `GET /api/workspaces/{id}` trả thêm `tenant_id` tùy chọn. Không có route Multica mới.

## 10. Mã lỗi (bổ sung cho gốc 10)

Cùng quy ước: JSON `{"error": "<thông điệp>", "code": "<mã>"}`; client dịch theo `code`.

| Mã | HTTP | Trường thêm | Nơi |
| --- | --- | --- | --- |
| `plan_limit_reached` (đã có) | 402 | `gate = max_workspaces`, `limit`, `used` | Multica, mục 5.7 |
| `tenant_not_found` | 404 | | Multica, 5.7 |
| `tenant_deleting` | 409 | | Multica, 6.4 |
| `member_request_invalid` | 400 | `fields[]` | platform, 4.2 |
| `member_role_invalid` | 400 | | platform, 4.2 |
| `workspace_not_active` | 409 | `status` | platform, 4.2 |
| `staff_cannot_be_added` | 409 | | platform, 4.2 |
| `already_member` | 409 | | platform, 4.2 |
| `member_request_not_cancelable` | 409 | `state` | platform, 4.2 |
| `reason_required` | 400 | | platform, 6.2 |
| `confirmation_mismatch` | 400 | | platform, 6.2 |
| `deletion_request_not_cancelable`, `deletion_request_not_retryable` | 409 | `state` | platform, 6.2 |
| `sole_owner_unresolved` | 409 | `workspaces[]` | platform, 7.3 |
| `successor_invalid` | 400 | `workspace_id` | platform, 7.3 |
| `user_is_staff`, `cannot_delete_self` | 409 | | platform, 7.2 |
| `tenant_required` | 400 | | platform, 3.7 |
| `tenant_not_allowed` | 400 | | platform, 3.7 |
| `tenant_invalid` | 400 | | platform, 3.7 |
| `last_system_admin` | 400 | | platform, 3.7 (thay mã `last_admin` của gốc 5.4, không còn dùng) |
| `invalid_role` (đã có) | 400 | | platform, 3.7: thông điệp nêu bốn vai trò |
| `plan_limit_change_confirm_required` | 409 | `affected_subscriptions` | platform, 3b.1 |
| `plan_inactive` | 409 | | platform, 3b.1 |
| `workspace_state_conflict` | 409 | `status`, `allowed[]` | platform, 3b.2 |
| `export_in_progress` | 409 | | platform, 3b.2 |
| `owner_role_immutable` | 409 | | platform, 3b.3 |
| `cannot_change_own_role` | 409 | | platform, 3b.3 |
| `limit_gate_invalid`, `limit_value_invalid` | 400 | `gate` | platform, 3b.4 |
| `runtime_disabled` | 403 | | Multica (API daemon), 3b.5 |
| `policy_below_tenant_floor` | 400 | `fields[]` | Multica, 3c.4 |
| `policy_version_conflict` | 409 | `version` | platform, 3c.4 |
| `quota_request_invalid` | 400 | `fields[]` | platform và Multica, 3c.5 |
| `quota_request_pending` | 409 | | platform và Multica, 3c.5 |
| `quota_request_not_pending` | 409 | `state` | platform, 3c.5 |
| `approver_must_differ`, `approver_unavailable` | 409 | | platform, 3c.6 |
| `approval_not_pending` | 409 | `state` | platform, 3c.6 |
| `legal_hold` | 409 | `legal_hold_id` (chỉ `system_admin`) | platform và Multica, 3c.7 |
| `legal_hold_exists` | 409 | | platform, 3c.7 |
| `export_range_too_large`, `export_range_invalid` | 400 | | platform, 3c.8 |
| `tenant_not_active` | 409 | `status` | Multica, 3c.9 |

## 11. Bảo mật (bổ sung cho gốc 11)

| Mối nguy | Biện pháp |
| --- | --- |
| Xóa nhầm workspace, tenant hoặc tài khoản | Chỉ đúng vai trò và phạm vi ở ma trận 3.6, có MFA; lý do bắt buộc; gõ lại slug, tên hoặc email do máy chủ kiểm (D72); console hiển thị số workspace và số thành viên sẽ bị xóa; hủy được cho tới khi việc nền bắt đầu chạy (cửa sổ tối đa khoảng một phút, GĐ8) |
| Mất dấu vết khi xóa | Kiểm toán ghi ở pha 1, trước mọi xóa, cùng giao dịch với dòng yêu cầu; xóa lỗi không làm mất nó (D73); dòng giữ lại sau khi workspace bị xóa nhờ `DetachWorkspaceAuditEvents` |
| Xóa tài khoản làm workspace mồ côi | Bắt buộc chọn (A) hoặc (B) (D75); việc nền kiểm lại; chốt cuối ở `server/internal/handler/me_deletion.go:474` |
| Đua giữa tạo workspace vào tenant và xóa tenant | Cùng khóa tư vấn (tenant, `max_workspaces`) cho tạo, bớt và xóa tenant; `tenant_deleting` (mục 6.4) |
| Hai yêu cầu xóa cùng đích | Chỉ mục duy nhất của yêu cầu đang sống; gửi lại trả yêu cầu cũ |
| Việc nền sẵn có (`workspace_deletion`, `account_deletion`) xóa trước hoặc sai actor | Hai truy vấn "đến hạn" loại trừ đích có yêu cầu xóa ngay chưa xong |
| Staff dùng đường thêm người để vào workspace khách | Mục 4.6: từ chối email của staff, không `owner`, kiểm toán chủ workspace đọc được. Rủi ro còn lại ở 15.3 |
| Rò email người thứ ba qua bảng yêu cầu | Xóa dòng yêu cầu cùng workspace; ẩn danh hóa email khi người dùng bị xóa; `target_label` của yêu cầu tài khoản đổi thành `deleted user` khi xong |
| Console rò nội dung tenant | Route mới chỉ trả siêu dữ liệu; test duyệt handler của console (gốc 5.4) áp cho cả route mới (AC90) |
| `admin` đọc hoặc sửa dữ liệu của tenant khác (đoán id, danh sách, kiểm toán) | Phạm vi ở ba lớp (3.5 quy tắc 4); id ngoài phạm vi trả 404 giống id không tồn tại; truy vấn nhận tham số phạm vi; kiểm toán lọc theo `tenant_id` hoặc `workspace_id` (D84); test AC93, AC103 |
| Dữ liệu vụ việc pháp lý bị xóa khi `system_admin` xóa ngay hoặc việc nền chạy | Giữ hồ sơ pháp lý có điểm chặn duy nhất ở `deleteWorkspaceData` và `DeleteAccount`, cộng loại trừ ở việc nền xóa nhật ký (3c.7); test AC137, AC138 |
| `system_admin` bị chiếm hay nhầm xóa tenant hoặc tài khoản | Duyệt chéo khi bật `PLATFORM_APPROVAL_ACTIONS` (3c.6); gõ lại tên và lý do (D72); kiểm toán trước khi xóa (D73) |
| Runtime bị chiếm vẫn nhận task | Vô hiệu hóa chặn ở đăng ký, mọi route daemon và giao task (3b.5); gỡ đăng ký qua đường xóa có sẵn |
| Chính sách bảo mật workspace bị nới lỏng dưới sàn tenant | `Merge` ở một nơi cưỡng chế, cộng từ chối ghi dưới sàn (3c.4) |
| Staff xuất kiểm toán rộng hơn quyền | `admin` chỉ tenant mình; `support`, `billing` không xuất; mỗi lần xuất ghi kiểm toán (3c.8) |
| IP hay tên máy cá nhân rò qua danh sách runtime | Không lưu IP; chỉ trả hệ điều hành và kiến trúc rút từ `device_info` (3b.5) |
| `admin` tự nâng quyền hoặc nâng quyền người khác | `/staff` chỉ `system_admin` (M1); bốn handler staff tự kiểm vai trò và trả 404 khi bị gọi thẳng; vai trò đọc lại từ cơ sở dữ liệu ở mỗi request; test AC101 |
| Mất toàn bộ `system_admin` | Không thu hồi hay hạ bậc `system_admin` cuối cùng (3.7 quy tắc 4); email trong `PLATFORM_ADMIN_EMAILS` được gieo lại thành `system_admin` (3.8) |
| `admin` giữ quyền sau khi tenant bị xóa | Phạm vi rỗng (3.5 quy tắc 5): mọi route theo id trả 404; `system_admin` gán lại hoặc thu hồi; test AC104 |
| Tenant lộ cho người không phải owner | `tenant_id` không thuộc người gọi trả 404 `tenant_not_found`, không phân biệt với tenant không tồn tại |

## 12. Tương thích và triển khai

- **Mặc định giữ hành vi cũ (I2).** Không đặt gì thì `CreateWorkspace` không có `tenant_id` tạo tenant một workspace; `max_workspaces` vắng mặt nên cổng `off`; hai việc nền mới chỉ đọc các bảng rỗng.
- **Thứ tự triển khai:** (1) `server/cmd/migrate up` cho 941 đến 947 và 1016 đến 1037; (2) Multica và platform mới; (3) `platform-backfill tenants`; (4) kiểm `/overview` báo 0 workspace thiếu tenant; (5) `system_admin` hạ từng tài khoản cần thành `admin` bằng `PUT /staff/{user_id}` kèm `tenant_id` (nếu có). Platform từ chối khởi động nếu thiếu một phiên bản (gốc 5.1, D33).
- **Cửa sổ vai trò.** Migration 944 đổi `admin` thành `system_admin` trước khi platform mới chạy, mà platform cũ coi vai trò lạ là không tìm thấy (`platform-management/internal/platformauth/platformauth.go:55`). Giữa bước (1) và (2) staff không đăng nhập được console (404, fail-closed); người dùng Multica không bị ảnh hưởng. Chạy (1) và (2) liền nhau. Hoàn nguyên bằng `down` của 944 thu hồi `admin` có tenant (3.8).
- **Client cũ (I9).** `tenant_id` là trường phản hồi tùy chọn; client cũ bỏ qua. Mã lỗi mới đi kèm mã HTTP mà client đã xử lý (402, 404, 409). Schema zod của `Workspace` thêm `tenant_id` tùy chọn kèm fallback (mục 13, AC89).
- **Mobile.** Không đổi. `apps/mobile/` tạo workspace theo đường mặc định không có `tenant_id`; trường phản hồi mới được bỏ qua. Không sửa tệp trong `apps/mobile/` nên không cần đọc `apps/mobile/AGENTS.md` cho đợt này.
- **Console** là một image với platform nên route cấp workspace đổi tên cùng lúc với giao diện (D77).
- **Chuỗi giao diện:** Multica web đủ bảy ngôn ngữ của `packages/views/locales/` (`en`, `es`, `fr`, `ja`, `ko`, `zh-Hans`, `zh-Hant`); không viết mô tả thừa (AGENTS.md "UI Copy").

## 13. Kiểm thử và tiêu chí chấp nhận

**Nguyên tắc.** Như gốc 13: test viết trước, đỏ rồi xanh, đặt cạnh mã; test Go chạm cơ sở dữ liệu dùng `server/internal/testutil` (Multica) hoặc `platform-management/internal/testdb` (platform) và khẳng định trên dòng fixture của chính test, không trên tổng bảng; test mặc định không gọi agent thật. Mỗi tiêu chí là một test chạy được.

**Bộ test bắt buộc của R6:**

15. **Ma trận vai trò staff × route**: `system_admin`, `admin` (trong tenant mình), `admin` (ngoài tenant), `support`, `billing`, người không phiên và khách hàng, trên mọi route ở mục 9; kết quả từng ô bằng đúng ma trận 3.6 (qua, 404 phẳng, 404 của đối tượng ngoài phạm vi). Bảng kỳ vọng của test được viết tay từ 3.6, không sinh từ bảng quyền trong mã.
16. **Hợp đồng cổng `max_workspaces`**: chính sách của platform qua `normalizePolicy` của client hiện có, kể cả khi vắng mặt.
17. **Xóa tương đương**: hai fixture giống nhau, một xóa bằng `DeleteWorkspace`, một bằng đường staff; tập dòng còn lại bằng nhau.
18. **Chạy lại và song song** cho `member_provisioning` và `staff_deletion`.
19. **Không-mồ-côi**: với mọi tổ hợp lựa chọn (A), (B), hỗn hợp, đồng owner, một thành viên; sau khi xong mọi workspace còn lại có ít nhất một owner.
20. **Phạm vi tenant của `admin`**: với mỗi route có ô "tenant mình" ở 3.6, ba ca: đối tượng trong tenant, đối tượng ngoài tenant (thân 404 bằng từng byte thân của đối tượng không tồn tại), và `admin` có tenant đã xóa; danh sách chỉ chứa dòng trong phạm vi; chạy cả khi handler được gọi thẳng bỏ qua middleware.
21. **Quản lý staff và migration vai trò**: tạo, đổi, thu hồi staff theo 3.7; migration 944 trên bảng có đủ bốn vai trò, chạy hai lần.
22. **Ma trận mở rộng**: mỗi hàng M30 đến M52 × `system_admin`, `admin` (trong và ngoài tenant), `support`, `billing`, người không phiên; kết quả bằng đúng ma trận 3c.10; bảng kỳ vọng viết tay.
23. **Phạm vi của route nhóm QT**: với mỗi route có ô "tenant mình" ở 3c.10, ba ca (trong tenant, ngoài tenant với thân 404 giống đối tượng không tồn tại, tenant đã xóa) và gọi thẳng handler bỏ qua middleware.
24. **Giữ hồ sơ pháp lý chặn mọi đường xóa**: bảng đường xóa (xóa ngay workspace, tenant, tài khoản; `DeleteWorkspace` tức thì; việc nền `workspace_deletion`; việc nền `account_deletion`; gỡ runtime không phải đường xóa dữ liệu workspace nên không thuộc bảng; `audit_retention`) × đối tượng bị giữ ở ba phạm vi; mỗi ô kỳ vọng không còn dòng nào bị xóa và dòng kiểm toán `deletion.blocked_by_hold`.
25. **Vô hiệu hóa runtime chặn ở mọi điểm**: đăng ký, nhịp tim, các route daemon khác dùng `requireDaemonRuntimeAccess`, giao task; kích hoạt lại khôi phục cả bốn.

**Tiêu chí chấp nhận R6.** Cột "Mục" là mục của tài liệu này; "QĐ" là quyết định ở mục 1.4. Mã bắt đầu từ AC62 vì spec gốc dùng đến AC61.

| AC | Nội dung kiểm | Mục | QĐ |
| --- | --- | --- | --- |
| AC62 | `POST /workspaces/{id}/members`, `GET /workspaces/{id}/member-requests`, `GET /member-requests/{id}`, `POST /member-requests/{id}/cancel`: `system_admin` gọi được trên mọi workspace, `admin` gọi được trên workspace trong tenant mình; `support`, `billing` và người không có phiên nhận 404 phẳng | 3.6 (M21, M22), 4.2, 4.4 | D60, D78, D79 |
| AC63 | Email có sẵn: dùng người dùng đó, không đổi tên hay dữ liệu. Email mới: tạo `user` với tên theo GĐ12, đã onboard, không mật khẩu, phiên, token hay mã tạo sẵn; lần đăng nhập đầu vẫn có `consent_required`. Miền bị chặn: yêu cầu `failed` `email_domain_blocked`. Tài khoản `suspended` hoặc `pending_deletion`: `failed` `account_not_active`. Email của staff đang hoạt động: 409 `staff_cannot_be_added` và không có dòng yêu cầu. Hoạt động được cả khi `ALLOW_SIGNUP` tắt | 4.2 | D60, D63 |
| AC64 | Vai trò **thành viên** được chọn (`admin`, `member`, `viewer`) là vai trò của dòng `member` tạo ra; `owner` hoặc giá trị lạ trả 400 `member_role_invalid` và không ghi gì | 4.2 | D60, D63 |
| AC65 | Ghế: hết ghế thì yêu cầu `failed` `capacity_full`, không có dòng `member`, thao tác ghế ở trạng thái `released`; còn ghế thì `ClaimShareJoin`, tạo `member`, `Confirm`, số đã dùng tăng một và số giữ về cũ; workspace không quản lý ghế bỏ qua; lỗi truyền tải thử 3 lần rồi `failed` `seat_capacity_unavailable` và giải phóng; hai yêu cầu đồng thời ở ghế cuối chỉ một cái thành công; chạy việc nền hai lần chỉ có một `member` | 4.2 | D62 |
| AC66 | Mỗi lần thêm sinh `member.staff_add_requested` (pha 1) và `member.staff_added` (pha 2) với `actor_type = staff`, `workspace_id` và `tenant_id`; chủ workspace đọc được hai dòng ở `GET /api/workspaces/{id}/audit-events`; làm hỏng ghi kiểm toán ở pha 1 thì không có dòng yêu cầu, ở pha 2 thì không có `member` và ghế được giải phóng; sự kiện `member:added` được phát | 4.5 | D60 |
| AC67 | Đổi 7b.5 quy tắc 2 đúng phạm vi: `POST /accounts` vẫn không thêm thành viên vào workspace có sẵn; mọi test lời mời và link chia sẻ hiện có xanh với client ghế thật; workspace không `active` bị 409 `workspace_not_active`; người đã là thành viên bị 409 `already_member`; hủy yêu cầu `pending` giải phóng ghế đã giữ | 4.2 | D61 |
| AC68 | Migration và backfill: mọi workspace có sẵn có đúng một tenant (`id` bằng id workspace) và đúng một dòng `tenant_workspace`; chạy migration 1021 và `platform-backfill tenants` hai lần, kể cả song song, không tạo dòng thứ hai; bảng `workspace` không thêm cột; các bảng mới không có khóa ngoại, `PRIMARY KEY` hay `UNIQUE` nội tuyến; chỉ mục tạo `CONCURRENTLY`, mỗi chỉ mục một tệp, đủ dòng dọn (`TestEveryConcurrentUpBuildHasCleanup`, `TestMigrationFilesHaveMatchingDirections`) | 5.4, 8 | D64, D69 |
| AC69 | `POST /api/workspaces` không có `tenant_id` tạo tenant mới và phản hồi cũ vẫn hợp lệ (thêm `tenant_id` tùy chọn); có `tenant_id` thì workspace vào đúng tenant khi người gọi là owner của một workspace trong đó, ngược lại 404 `tenant_not_found`; việc nền `account_provisioning` cũng tạo tenant | 5.7, 5.9 | D67 |
| AC70 | Tenant ở giới hạn `max_workspaces` bị 402 `plan_limit_reached` với `gate = max_workspaces`, `limit`, `used`; hai request đồng thời ở chỗ cuối chỉ một cái thành công; khóa tư vấn có `Scope` là id tenant; hạ gói không xóa hay khóa workspace nào, chỉ chặn tạo mới | 5.7 | D66 |
| AC71 | Giới hạn của tenant là lớn nhất giữa các workspace (kể cả `limit_overrides`), giá trị rỗng thắng, workspace không có thuê bao tính theo `free`; mọi workspace của tenant nhận cùng giá trị trong chính sách; khi `MULTICA_CLOUD_URL` rỗng hoặc platform không phản hồi thì không chặn | 5.3, 5.6, 5.7 | D65, D66 |
| AC72 | Xóa workspace thường (`DeleteWorkspace` và việc nền) xóa dòng liên kết và, khi là workspace cuối, xóa dòng `tenant`; xóa một trong nhiều workspace giữ tenant; bản kê khai xóa workspace phân loại các bảng mới; tạo và xóa đồng thời không để dòng liên kết trỏ vào tenant đã xóa | 5.9 | D68 |
| AC73 | Danh mục có đủ giá trị mặc định ở mục 5.6; migration 941 lũy đẳng và không ghi đè khóa đã có; `max_workspaces` nhỏ hơn 1 bị `PUT /plans` từ chối; gói `unlimited` vẫn không giới hạn mọi hạn mức đếm kể cả khóa mới | 5.6 | D66, D70 |
| AC74 | `GET /tenants` và `GET /tenants/{id}` trả tenant với danh sách workspace và `max_workspaces`; route cấp workspace ở `/workspaces/{id}/…` hoạt động với vai trò cũ; đường cũ `/tenants/{id}/status` trả 404; test duyệt handler của console vẫn xanh | 5.8 | D77 |
| AC75 | Trạng thái cưỡng chế chỉ ở workspace (khóa cả tenant ghi từng workspace, AC141): `status_summary` đếm đúng trạng thái workspace; khóa, mở và lên lịch xóa một workspace không đổi workspace khác của tenant; `support` và `billing` không thêm quyền nào so với gốc 5.2 | 5.5, 5.8 | D71, D78 |
| AC76 | `system_admin` gọi được `POST /workspaces/{id}/delete-now` và `POST /tenants/{id}/delete-now`; `admin` gọi được route workspace trên workspace trong tenant mình và nhận 404 phẳng ở route tenant; `support` và `billing` nhận 404 phẳng ở cả hai, `support` vẫn khóa và mở được còn `billing` không (như gốc 5.2), và không ai trong hai người lên lịch xóa được như trước | 3.6 (M10, M11, M23, M24), 6.2, 6.5 | D71, D78, D85 |
| AC77 | Lý do rỗng hoặc chỉ khoảng trắng: 400 `reason_required`; `confirm` sai slug, sai tên: 400 `confirmation_mismatch`; cả hai kiểm ở máy chủ và không ghi dòng yêu cầu, trạng thái hay kiểm toán nào | 6.2 | D72 |
| AC78 | Sau commit pha 1, dòng kiểm toán `workspace.immediate_deletion_requested` (actor `staff`, có `reason`) đã tồn tại; gây lỗi giữa xóa (tiêm lỗi vào một bước) thì dữ liệu workspace còn nguyên (một giao dịch), dòng yêu cầu `failed` sau 3 lần thử, dòng kiểm toán pha 1 còn; thử lại hoàn tất; sau khi workspace bị xóa, dòng kiểm toán còn với `workspace_id` rỗng và `target_id` đúng | 6.3 | D73 |
| AC79 | Xóa theo đường hiện có: tập dòng còn lại sau xóa staff bằng tập dòng còn lại sau `DeleteWorkspace` trên fixture tương đương; không có truy vấn `DELETE` mới trên bảng dữ liệu workspace trong `platform-management/queries/` hay trong gói việc nền mới (test quét tệp `.sql` và `.go`) | 6.2 | D73 |
| AC80 | Xóa tenant xóa mọi workspace và dòng `tenant`; `CreateWorkspace` vào tenant đang xóa trả 409 `tenant_deleting`; tạo đồng thời với pha 1 thì workspace mới hoặc nằm trong danh sách xóa hoặc bị từ chối; gửi lại trả cùng yêu cầu; việc nền chạy hai lần và song song không xóa hai lần | 6.3, 6.4 | D71, D73 |
| AC81 | Ngay sau pha 1, request vào workspace trả 403 `workspace_pending_deletion` và agent không nhận task; `deletion_scheduled_at` nhỏ hơn hoặc bằng hiện tại; việc nền sẵn có `workspace_deletion` bỏ qua workspace có yêu cầu đang sống hay `failed`; hủy yêu cầu `pending` đưa workspace về `active` kèm kiểm toán; `subscription_lifecycle` chạy mọi cạnh vẫn không xóa workspace nào | 5.5, 6.3 | D71, D73 |
| AC82 | Chỉ `system_admin` gọi được route tạo yêu cầu xóa tài khoản (`admin`, `support`, `billing` nhận 404 phẳng); `confirm` phải là email; tài khoản của staff đang hoạt động bị 409 `user_is_staff`, chính staff đang thao tác bị 409 `cannot_delete_self` | 7.2 | D74, D76 |
| AC83 | Cùng pipeline gốc 4.6, không chờ 14 ngày: sau pha 1 mọi token của người đó bị chặn (`account_pending_deletion`) và kết nối bị ngắt; sau pha 2 bản ghi `user` bị ẩn danh hóa (`Deleted user`, email không gửi được), PAT và token bị xóa, tư cách thành viên bị gỡ qua `revokeAndRemoveMember`, workspace một thành viên bị xóa, nhãn kiểm toán thành `deleted user`; test sổ đăng ký cột người dùng xanh với các cột mới | 7.1, 7.4 | D74 |
| AC84 | Owner duy nhất của workspace còn người khác mà thiếu lựa chọn: 409 `sole_owner_unresolved` kèm danh sách và không ghi gì. (A): người nhận thành `owner` với `member.role_changed` actor `staff`, rồi người dùng bị gỡ. (B): workspace bị xóa theo mục 6, rồi tài khoản bị xóa. Người nhận sai: 400 `successor_invalid`. Có đồng owner: không cần lựa chọn | 7.3, 7.4 | D75 |
| AC85 | Không workspace nào mồ côi sau mọi tổ hợp (bộ test 19); nếu người nhận bị gỡ giữa hai pha thì yêu cầu `failed` `sole_owner_unresolved` và tài khoản chưa bị xóa; chốt `countOwners` của `removeMemberForAccountDeletion` còn nguyên | 7.4 | D75 |
| AC86 | Kiểm toán xóa tài khoản: `account.immediate_deletion_requested` (actor `staff`) tồn tại trước mọi dòng xóa; các dòng `member.role_changed`, `workspace.deleted`, `member.left` sinh trong pipeline có `actor_type = staff`; `account.deleted` ghi cuối; `detail` không chứa email; sau khi xong nhãn email trong mọi dòng và trong `target_label` của yêu cầu đã bị thay | 7.6 | D74 |
| AC87 | Hồi quy đường tự phục vụ: `POST /api/me/deletion` vẫn chờ 14 ngày, trả `sole_owner` như cũ, việc nền `account_deletion` vẫn xóa tài khoản đến hạn; tài khoản có yêu cầu xóa ngay bị việc nền đó bỏ qua | 3.2, 7.4 | D74 |
| AC88 | Cấu hình rỗng: khi `MULTICA_CLOUD_URL` rỗng hoặc trỏ tới Multica Cloud, `max_workspaces` là `off` và `CreateWorkspace` hành xử như trước R6; toàn bộ test hiện có của `server/`, `platform-management/` và `packages/` xanh | 5.7, 12 | D66 |
| AC89 | Hợp đồng JSON mở rộng: phản hồi workspace có và không có `tenant_id` đều qua schema zod của client; mã lỗi mới có ánh xạ chuỗi đủ bảy ngôn ngữ (kiểm bởi `packages/views/locales/parity.test.ts`); `plan_limit_reached` với `gate = max_workspaces` hiện thông báo đúng | 12 | D66, D67 |
| AC90 | Platform từ chối khởi động khi thiếu một phiên bản migration của R6, gồm 941 đến 947 và 1016 đến 1037 (danh sách `schemacheck`); test duyệt handler của console khẳng định route mới không chạm bảng nội dung tenant; bộ test 15 (ma trận vai trò × route) xanh | 8, 9, 11 | D78, D83 |

Vai trò `system_admin`, phạm vi tenant của `admin`, quản lý staff (mục 3.5 đến 3.8):

| AC | Nội dung kiểm | Mục | QĐ |
| --- | --- | --- | --- |
| AC91 | Mô hình vai trò: `staff.ParseRole` nhận đúng bốn vai trò `system_admin`, `admin`, `support`, `billing` và từ chối giá trị khác; vai trò lạ trong cơ sở dữ liệu thì người đó "không tìm thấy" ở `RequirePlatformStaff` (404); `GET /me` trả `role` và `tenant_id` (rỗng cho ba vai trò không có tenant, có giá trị cho `admin`); thứ bậc không cộng dồn: `admin` không dùng được route của `billing` hay `support` | 3.5 | D79 |
| AC92 | `admin` gọi mọi route dành riêng cho `system_admin` (hàng M1 đến M5 trừ `GET /plans` ở M30, M12, M14 đến M17, M19, M24, M25; `GET /overview` thuộc M17) nhận 404 phẳng như route không biết; không ghi dòng nào và không mở giao dịch | 3.6 | D79, D83 |
| AC93 | `admin` gọi route theo id trên đối tượng ngoài tenant mình (tenant, workspace và nhóm con, yêu cầu thêm người, yêu cầu xóa, đơn hàng, người dùng không là thành viên workspace nào trong tenant) nhận 404 có thân bằng từng byte thân của đối tượng không tồn tại; danh sách của `admin` chỉ chứa dòng trong phạm vi; kiểm cả khi handler bị gọi thẳng bỏ qua middleware | 3.5 | D80 |
| AC94 | `admin` thêm người vào workspace trong tenant mình: 202, yêu cầu được việc nền xử lý thành `member` đúng vai trò thành viên, có `member.staff_add_requested` và `member.staff_added` với `actor_type = staff`, `workspace_id` và `tenant_id` của tenant mình; cùng yêu cầu trên workspace ngoài tenant trả 404 và không ghi gì; `system_admin` làm được trên workspace của bất kỳ tenant nào | 3.6 (M21), 4.2, 4.5 | D60, D80 |
| AC95 | `admin` xóa ngay workspace trong tenant mình (đủ lý do và gõ lại slug): thành công theo mục 6 và dòng kiểm toán pha 1 có `tenant_id`; sau khi workspace bị xóa, `admin` của tenant vẫn đọc được dòng đó; workspace ngoài tenant trả 404; `system_admin` xóa được workspace của bất kỳ tenant nào | 3.6 (M23), 6.2, 6.3 | D71, D80, D84 |
| AC96 | `admin` gọi `POST /tenants/{id}/delete-now` nhận 404 phẳng, kể cả với tenant của chính mình, và không ghi dòng nào; `system_admin` xóa được tenant | 3.6 (M24), 6.2 | D71, D85 |
| AC97 | `admin` gọi `POST /users/{id}/delete-now` và `GET /users/{id}/delete-now-preview` nhận 404 phẳng, kể cả với người dùng chỉ là thành viên của workspace trong tenant mình, và không ghi dòng nào; `system_admin` xóa được tài khoản | 3.6 (M25), 7.2 | D74, D86 |
| AC98 | `system_admin` làm được mọi hành động của ma trận 3.6 trên đối tượng của bất kỳ tenant nào (test bảng M1 đến M29, mỗi hàng một ca thành công); mọi hàng cho `support` và `billing` khớp bảng gốc 5.2 và kết quả hiện có của `TestRoleMatrix` | 3.6 | D79, D83 |
| AC99 | Migration vai trò: chạy 942 đến 944 trên bảng có `admin` không tenant, `admin` có tenant, `support`, `billing`, `system_admin`: `admin` không tenant thành `system_admin`, các dòng khác không đổi; chạy lần hai không đổi gì; cột `tenant_id` rỗng được và không có khóa ngoại; hai chỉ mục tạo `CONCURRENTLY`, mỗi chỉ mục một tệp, có dòng dọn; `down` của 944 thu hồi `admin` có tenant trước khi đặt `system_admin` về `admin`; bộ gieo từ `PLATFORM_ADMIN_EMAILS` tạo hoặc mở lại dòng `system_admin` | 3.8, 8 | D82 |
| AC100 | Chỉ `system_admin` tạo staff: `POST /staff` với `role = admin` thiếu `tenant_id` trả 400 `tenant_required`, `tenant_id` không tồn tại trả 400 `tenant_invalid`, `tenant_id` kèm `system_admin`, `support` hoặc `billing` trả 400 `tenant_not_allowed`; `role = admin` kèm tenant hợp lệ thành công và `GET /staff` trả tenant; `PUT /staff/{user_id}` áp cùng ba kiểm tra và xóa `tenant_id` khi đổi `admin` sang vai trò khác | 3.7 | D81 |
| AC101 | `admin` gọi `PUT`, `POST`, `DELETE /staff` cho chính mình hoặc người khác nhận 404 và không ghi gì, kể cả khi handler bị gọi thẳng bỏ qua middleware; sau khi `system_admin` đổi vai trò hoặc tenant của một staff, request kế tiếp của staff đó đã dùng quyền mới; không thu hồi hay hạ bậc `system_admin` cuối cùng (400 `last_system_admin`), kể cả hai yêu cầu đồng thời | 3.7 | D81 |
| AC102 | Mỗi lần thêm, đổi vai trò, gán tenant, thu hồi staff sinh đúng một dòng kiểm toán (`staff.added`, `staff.role_changed`, `staff.tenant_assigned`, `staff.removed`) với `actor_type = staff`, `detail` có giá trị cũ và mới, và `tenant_id` của tenant liên quan; làm hỏng ghi kiểm toán thì thay đổi không lưu | 3.7 | D81, D84 |
| AC103 | Phạm vi kiểm toán: `admin` đọc `GET /audit-events` và `GET /workspaces/{id}/audit-events` chỉ thấy dòng có `tenant_id` hoặc `workspace_id` thuộc tenant mình; `system_admin`, `support`, `billing` thấy toàn bộ; dòng xóa workspace (đã mất `workspace_id`) vẫn hiện cho `admin` của tenant nhờ `tenant_id`; `audit.Record` ghi `tenant_id` khi `Entry.TenantID` có giá trị và không đổi hành vi cũ khi để trống | 3.6 (M20), 8 | D84 |
| AC104 | `admin` có `tenant_id` trỏ tới tenant đã bị xóa có phạm vi rỗng: mọi danh sách rỗng, mọi route theo id trả 404; dòng staff không bị sửa khi xóa tenant; `system_admin` gán lại tenant thì request kế tiếp có phạm vi mới | 3.5 quy tắc 5, 6.3 | D80, D81 |
| AC105 | Yêu cầu xóa và yêu cầu thêm người: `admin` chỉ thấy yêu cầu `workspace` và `tenant` của tenant mình và không thấy yêu cầu `account`; `admin` hủy hoặc thử lại được yêu cầu `workspace` do chính mình tạo, nhận 404 khi hủy yêu cầu do `system_admin` tạo; `system_admin` hủy và thử lại được mọi yêu cầu | 3.6 (M22, M27, M28), 6.2 | D80, D83 |
| AC106 | Khóa, mở, lên lịch xóa: `system_admin` làm được trên mọi workspace; `admin` làm được trên workspace trong tenant mình và nhận 404 ngoài tenant; `support` khóa và mở được nhưng không lên lịch xóa; `billing` không khóa, mở hay lên lịch xóa (đúng bảng gốc 5.2 và `TestBillingCannotWrite`) | 3.6 (M10, M11) | D83 |
| AC107 | Quyền của `admin` ở phần thanh toán: đọc được đơn hàng của workspace trong tenant mình; `confirm`, `cancel`, đổi gói, ghế, kỳ, xác minh hồ sơ, xóa nợ, `payment-notices`, `einvoices` và `/accounts` nhận 404 phẳng; `billing` giữ quyền của gốc; chỉ `system_admin` xóa nợ gói theo mức dùng | 3.6 (M5, M12 đến M16) | D83 |

Nhóm quản trị QT1 đến QT19 (mục 3b, 3c). Tiêu chí của QT6 đến QT19 chỉ viết cho nhóm bắt buộc và nên có.

| AC | Nội dung kiểm | Mục | QĐ |
| --- | --- | --- | --- |
| AC108 | Tạo gói: `POST /plans` nhận `key` duy nhất không đổi sau khi tạo (`PUT` không đổi được `key`), `description` tối đa 500 ký tự, `limits` có `max_workspaces` không nhỏ hơn 1; gói mới `active`; `key` trùng trả 409 `plan_exists`; chỉ `system_admin` ghi, `admin` đọc được `GET /plans` và `GET /plans/{key}` nhưng nhận 404 phẳng ở route ghi; `support` nhận 404 phẳng ở cả hai | 3b.1, 3c.10 (M30, M31) | D87, D89 |
| AC109 | Bật và tắt: `deactivate` đặt `archived_at` và ghi `plan.deactivated`; gói tắt không có trong `GET /api/workspaces/{id}/plans`, và đơn `subscribe` hay `change_plan` tới gói đó trả 409 `plan_inactive`; thuê bao đang dùng vẫn gia hạn, mua ghế và giữ giới hạn; `activate` khôi phục; hai lần liên tiếp không đổi thêm và không ghi thêm dòng | 3b.1 | D87 |
| AC110 | Xóa gói: có thuê bao hoặc đơn thì 409 `plan_in_use` và dữ liệu không đổi; không có thì xóa được và gói `free` không xóa được; kiểm trong code, schema không có khóa ngoại | 3b.1 | D89 |
| AC111 | Lịch sử và sửa: `GET /plans/{key}/history` trả đủ ai, lúc nào, trường, giá trị cũ và mới cho đổi tên, mô tả, `limits`, `features`, giá, bật, tắt; hạ giới hạn gói có thuê bao thiếu `confirm_affects_subscribers` trả 409 `plan_limit_change_confirm_required` kèm số thuê bao và không lưu; có xác nhận thì lưu và không xóa dữ liệu nào; đổi giá không đổi đơn đã tạo và kỳ đã trả (tái dùng AC39) | 3b.1 | D88 |
| AC112 | `GET /workspaces`: tìm theo tên, slug, email owner, tên tenant; lọc theo trạng thái, gói, tenant; phân trang theo khóa không trùng và không sót dòng; `system_admin`, `support`, `billing` thấy mọi workspace; `admin` chỉ workspace trong tenant mình, lọc `tenant_id` của tenant khác trả danh sách rỗng | 3b.2, 3c.10 (M33) | D90 |
| AC113 | Chi tiết workspace có tenant, thuê bao, ghế (đã mua, đã dùng, đang giữ), `usage_current`, owner, ngày tạo và lịch sử trạng thái; số ghế đã dùng bằng số dòng `member`; workspace ngoài tenant của `admin` trả 404 giống workspace không tồn tại | 3b.2 | D90 |
| AC114 | Hành động nhanh: khóa, mở khóa, lên lịch xóa, hủy lịch, xóa ngay, đổi gói, gia hạn, mở kiểm toán đều dùng route đã có theo đúng ma trận; hành động lệch trạng thái trả 409 `workspace_state_conflict` kèm `status` và `allowed[]` và không ghi gì; mọi dòng kiểm toán có `workspace_id` và `tenant_id` | 3b.2 | D91 |
| AC115 | `POST /workspaces/{id}/export` do `system_admin` hoặc `admin` (workspace trong tenant mình) tạo đúng một bản xuất `queued` có `requested_by` là staff; việc nền `workspace_export` xử lý như cũ; lần thứ hai khi đang chạy trả 409 `export_in_progress`; ghi `workspace.export_requested`; `support`, `billing` nhận 404 phẳng | 3b.2 | D91 |
| AC116 | `GET /tenants/{id}/users` liệt kê người dùng là thành viên của ít nhất một workspace trong tenant, đủ trường (tên, email, trạng thái tài khoản, workspace và vai trò thành viên, số ghế, lần đăng nhập gần nhất), tìm, lọc, phân trang; không hiện người chỉ thuộc tenant khác; người dùng là staff không hiện với `admin` | 3b.3, 3c.10 (M35) | D92 |
| AC117 | Chi tiết người dùng trong tenant có kiểm toán gần nhất trong tenant và `allocation` (ghế, hạn mức hiệu lực và mức dùng theo workspace); người dùng ngoài tenant của `admin` trả 404; `admin` gọi `PUT` hay `POST` phân bổ trực tiếp nhận 404 phẳng | 3b.3 | D92, D95 |
| AC118 | Vô hiệu hóa trong tenant: `POST /tenants/{id}/users/{user_id}/deactivate` tạo một yêu cầu `remove` cho mỗi workspace của tenant mà người đó là thành viên, việc nền gỡ qua `revokeAndRemoveMember` và giải phóng ghế; workspace ngoài tenant và tài khoản không đổi; vô hiệu hóa toàn cục (`/users/{id}/status`) vẫn chỉ `system_admin` và `support`, `admin` nhận 404 phẳng | 3b.3 | D93, D94 |
| AC119 | Đổi vai trò thành viên và gỡ qua yêu cầu `change_role` và `remove`: vai trò đích thuộc `admin`, `member`, `viewer`; `owner` làm nguồn hay đích trả 409 `owner_role_immutable`; `admin` đổi vai trò của chính mình trả 409 `cannot_change_own_role`; người dùng là staff trả 404 cho `admin` và 409 `user_is_staff` cho `system_admin`; gỡ owner duy nhất bị chặn bởi `sole_owner`; mỗi bước ghi kiểm toán `actor_type = staff` có `tenant_id`, `workspace_id` và người dùng đích | 3b.3 | D94 |
| AC120 | Gỡ thành viên giải phóng ghế theo giao thức ghế (số đã dùng giảm một, thao tác ghế `released`), chạy hai lần và song song cho kết quả không đổi, và đổi vai trò không đổi số ghế; chuỗi `add`, `remove`, `add` lại cho cùng người ra một dòng `member` cuối cùng | 3b.3 | D94 |
| AC121 | Ranh giới phân bổ và xóa: `admin` không xóa được tài khoản (AC97), không đặt `limit_overrides`, không tăng ghế, chỉ gửi yêu cầu tăng hạn mức (AC132); `system_admin` và `billing` đổi ghế và `limit_overrides` như M12; không có bảng hạn mức theo người dùng | 3b.3 | D95 |
| AC122 | `GET /tenants/{id}/resources` trả đủ loại tài nguyên ở bảng 3b.4 với `used`, `limit`, `percent`, `status` (`ok`, `warning` từ 80 phần trăm, `over`), tổng tenant và từng workspace, khớp bảng nguồn trên dữ liệu mẫu; `admin` chỉ tenant mình; `limit` rỗng khi không giới hạn | 3b.4, 3c.10 (M38) | D96 |
| AC123 | `GET /resources` chỉ `system_admin`: tổng theo loại, gói, tenant; `top` đúng thứ tự; `over_limit[]` liệt kê đúng (tenant, workspace, loại) vượt hoặc sát; phân trang; `admin`, `support`, `billing` nhận 404 phẳng; không tạo bảng mới | 3b.4, 3c.10 (M39) | D96 |
| AC124 | `POST /tenants/{id}/limit-overrides` ghi `limit_overrides` cho mọi workspace của tenant (hoặc tập con) trong một giao dịch, tăng `version`, xóa cache chính sách, một dòng kiểm toán `subscription.limit_override_changed` mỗi workspace có giá trị cũ, mới, `reason`, `tenant_id`; `gate` lạ trả 400 `limit_gate_invalid`; một workspace lỗi thì không workspace nào đổi; `admin` và `support` nhận 404 phẳng; hạn mức hiệu lực trong chính sách đổi trong 30 giây | 3b.4, 3c.10 (M40) | D97 |
| AC125 | `GET /runtimes`: `system_admin` và `support` thấy mọi runtime, `admin` chỉ runtime của workspace trong tenant mình; lọc, tìm, phân trang; có `running_tasks`, phiên bản daemon và agent, `os`; **không** có IP, `device_info` nguyên văn hay `metadata` thô; `billing` nhận 404 phẳng | 3b.5, 3c.10 (M41) | D98, D99 |
| AC126 | Chi tiết runtime có 20 task gần nhất (không nội dung), số task lỗi bảy ngày, dòng kiểm toán staff; runtime ngoài tenant của `admin` trả 404 giống runtime không tồn tại | 3b.5 | D99 |
| AC127 | Vô hiệu hóa runtime: sau khi ghi `platform_runtime`, `DaemonRegister` và mọi route qua `requireDaemonRuntimeAccess` trả 403 `runtime_disabled`, `claimRefusalFor` không giao task (task ở lại hàng đợi); kích hoạt lại khôi phục; tác dụng trong 60 giây kể cả khi cache hỏng; ghi `runtime.disabled` và `runtime.enabled` có `workspace_id` và `tenant_id`; chỉ `system_admin` và `admin` trong tenant mình | 3b.5, 3c.10 (M42) | D98 |
| AC128 | Gỡ đăng ký runtime: cần `reason` và `confirm` bằng tên runtime; kiểm toán trước khi xóa; xóa bằng `DeleteAgentRuntime` có sẵn (so với xóa qua route của owner trên dữ liệu tương đương, tập dòng còn lại bằng nhau); không SQL xóa rời; xóa dòng `platform_runtime`; `support` và `billing` nhận 404 phẳng | 3b.5, 3c.10 (M43) | D98 |
| AC129 | Chính sách tenant: `GET` và `PUT /tenants/{id}/policy` kèm `expected_version`; ghi bị 402 `plan_feature_unavailable` nếu không workspace nào của tenant có `workspace_policies`; `workspacepolicy.Merge` đúng bảng ca (giao tên miền, hội `RequireSSO` và `RequireMFA`, `ShareLinksEnabled` bằng AND, nhỏ hơn khác 0 của `SessionMaxAgeHours`); chính sách tenant áp cho mọi workspace bất kể gói; `system_admin` và `admin` (tenant mình) ghi, `support` đọc | 3c.4, 3c.10 (M44, M45) | D101 |
| AC130 | Sàn: `PUT /api/workspaces/{id}/policy` lỏng hơn sàn trả 400 `policy_below_tenant_floor` kèm `fields[]` và không lưu; chính sách workspace đã lưu lỏng hơn sàn mới vẫn còn nhưng `EffectivePolicy` trả giá trị chặt hơn; `GET .../policy` trả `floor`; tenant không có dòng `tenant_policy` thì hành vi y như trước (hồi quy `workspace_policy` hiện có xanh) | 3c.4 | D101 |
| AC131 | Cảnh báo ngưỡng: `quota_alert` chạy hai lần và song song gửi đúng một thư mỗi (workspace, loại, ngưỡng, chu kỳ) cho owner và admin ở 80 phần trăm, một thư nữa ở 100 phần trăm; dedupe bằng `billing_notice`; không gửi cho workspace `pending_deletion`; không chặn hay đổi gì khác | 3c.5 | D102 |
| AC132 | Yêu cầu tăng hạn mức: owner hoặc người có `billing.manage` (Multica) và `system_admin` hay `admin` (console, tenant mình) tạo được; `requested_value` không lớn hơn giá trị hiệu lực bị 400; mỗi (workspace, `gate`) một yêu cầu `pending` (409 `quota_request_pending`); token máy bị 403; `admin` và `billing` đọc được phạm vi của mình, `support` nhận 404 phẳng | 3c.5, 3c.10 (M46, M48) | D102 |
| AC133 | Duyệt và từ chối chỉ `system_admin`, không tự động; duyệt ghi `limit_overrides` đúng workspace cùng kiểm toán `quota.request_approved` trong một giao dịch (có thể duyệt thấp hơn mức xin); từ chối có lý do; yêu cầu đã đóng trả 409 `quota_request_not_pending`; người xin nhận email kết quả | 3c.5, 3c.10 (M47) | D102 |
| AC134 | Duyệt chéo bật (`PLATFORM_APPROVAL_ACTIONS` có `tenant.delete_now` và `account.delete_now`): yêu cầu xóa ngay tenant và tài khoản chạy mọi kiểm đầu vào rồi dừng ở `awaiting_approval`, chưa có trạng thái `pending_deletion`, chưa chặn truy cập, chưa xóa; chỉ có một `system_admin` thì 409 `approver_unavailable` và không ghi dòng yêu cầu | 3c.6 | D103 |
| AC135 | Duyệt: `system_admin` khác người yêu cầu duyệt thì pha 1 chạy như D73; trùng người trả 409 `approver_must_differ`; từ chối hoặc quá 24 giờ đặt `canceled`; `admin` nhận 404 phẳng; với cấu hình rỗng, AC96 và AC97 giữ nguyên kết quả (hồi quy) | 3c.6, 3c.10 (M49) | D103 |
| AC136 | Giữ hồ sơ pháp lý: chỉ `system_admin` đặt và gỡ, đòi `reason` và `case_ref`; hai giữ cùng đối tượng trả 409 `legal_hold_exists`; `admin` không có route (404 phẳng) và chỉ gặp lỗi `legal_hold` không kèm lý do hay mã vụ việc; `detail` kiểm toán chỉ chứa `legal_hold_id`; tenant bị giữ phủ cả workspace tạo sau đó | 3c.7, 3c.10 (M50) | D104 |
| AC137 | Giữ chặn mọi đường xóa: xóa ngay workspace, tenant, tài khoản ở pha 1 (409 `legal_hold`, không ghi gì) và ở pha 2 (yêu cầu dừng, dữ liệu còn nguyên); `DeleteWorkspace` tức thì của owner; việc nền `workspace_deletion` và `account_deletion`; xóa tài khoản của người là thành viên duy nhất của workspace bị giữ; mỗi lần chặn ghi `deletion.blocked_by_hold`; điểm chặn nằm ở `deleteWorkspaceData` và `DeleteAccount` nên một đường mới cũng bị chặn | 3c.7 | D104 |
| AC138 | Việc nền `audit_retention` không xóa dòng thuộc đối tượng bị giữ (theo `tenant_id` hoặc `workspace_id` của tenant hay workspace bị giữ); gỡ giữ rồi việc nền xóa dòng đến hạn ở lần chạy kế; vận hành thường ngày của đối tượng bị giữ không đổi; dữ liệu không mất ở mọi ca | 3c.7, 3c.8 | D104, D105 |
| AC139 | Xuất kiểm toán: `GET /audit-events/export` theo `from`, `to` (tối đa 31 ngày, quá thì 400 `export_range_too_large`), `ndjson` và `csv`, phân trang bằng `cursor` tối đa 100.000 dòng, truyền dòng; `system_admin` toàn bộ, `admin` chỉ dòng của tenant mình, `support`, `billing` nhận 404 phẳng; mỗi lần xuất ghi `audit.exported` có bộ lọc và số dòng | 3c.8, 3c.10 (M51) | D105 |
| AC140 | Nhật ký không sửa, không xóa: sau đợt này `queries_readonly_test` vẫn xanh (không UPDATE ngoài hai ẩn danh hóa, không DELETE ngoài xóa theo thời hạn); `DeleteAuditEventsBefore` có điều kiện loại dòng bị giữ; không route nào của staff sửa hay xóa dòng kiểm toán | 3c.8 | D105 |
| AC141 | Khóa cả tenant: `POST /tenants/{id}/status` ghi `platform_workspace` cho mọi workspace của tenant với `reason = tenant_status` trong một giao dịch, một dòng `tenant.status_changed`; mở khóa chỉ khôi phục workspace có `reason = tenant_status` và giữ workspace bị khóa riêng; `CreateWorkspace` vào tenant `read_only` hay `suspended` trả 409 `tenant_not_active`; `system_admin`, `support`, `admin` (tenant mình) làm được, `billing` nhận 404 phẳng | 3c.9, 3c.10 (M52) | D106 |
| AC142 | Báo owner và xuất trước: xóa ngay workspace, tenant, tài khoản gửi email cho mọi owner ở pha 1 (người bị xóa không nhận), gửi lỗi không chặn xóa và ghi `deletion.notice_failed`; với `export_first = true` pha 2 chờ bản xuất `done`, gửi owner liên kết tải có chữ ký hạn 7 ngày, rồi mới xóa; quá 24 giờ chưa xong thì yêu cầu `failed` với `export_timeout` và dữ liệu còn nguyên; giữ hồ sơ pháp lý chạy trước | 3c.9 | D106 |
| AC143 | Ma trận mở rộng: mỗi hàng M30 đến M52 cho đúng kết quả với `system_admin`, `admin` trong và ngoài tenant, `support`, `billing`, không phiên; `system_admin` làm được mọi hàng; mọi route nhóm QT có hàng ở ma trận; với mọi cấu hình mới để trống (`PLATFORM_APPROVAL_ACTIONS` rỗng, không dòng `tenant_policy`, `platform_runtime`, `legal_hold`, `quota_request`) toàn bộ test hiện có xanh | 3c.10 | D83, D100 |

## 14. Ngoài phạm vi

- Thuê bao ở cấp tenant (phương án B của mục 5.3) và thanh toán một lần cho cả tenant.
- Vai trò hoặc cột "chủ tenant"; chuyển workspace giữa các tenant; gộp hay tách tenant.
- Hoàn tác xóa ngay (không có thùng rác); khôi phục từ bản xuất là việc của gốc 5.11.
- Thêm người hàng loạt, thêm `owner` bằng staff, gửi email thông báo cho người được thêm (GĐ13).
- Xóa hàng loạt và xóa theo lịch của đường xóa ngay.
- Màn hình Multica để người dùng liệt kê hay quản lý tenant; đợt này chỉ có trường `tenant_id` và lựa chọn "cùng tenant" ở bước tạo workspace (kế hoạch bổ sung, S14).
- Sửa `accountdeletion.Classify` của đường tự phục vụ (mục 15.3).
- **Nhóm QT8, QT9, QT12 đến QT17 (mức "sau này", D100, GĐ26), mỗi nhóm kèm lý do:** QT8 nhãn cho tenant và workspace (không rủi ro cụ thể; tìm theo tên và slug đủ); QT9 mạo danh có kiểm soát cho `support` (đảo I6 và D10, cần quyết định riêng, và cần QT10, QT11, QT18 trước); QT12 cờ tính năng theo tenant và kill switch (quyền theo gói đã ở `limits` và `features`); QT13 thông báo và lịch bảo trì (cần hạ tầng trong ứng dụng; phần báo owner khi xóa ngay nằm ở QT19); QT14 rà soát quyền định kỳ (số staff nhỏ, cần QT18); QT15 tín dụng, mã giảm giá (chưa thu tiền thật, D49); QT16 điểm sức khỏe tenant (chưa có nguồn yêu cầu hỗ trợ); QT17 vùng dữ liệu (một cơ sở dữ liệu chung, D2, D58); phần quyền có thời hạn kiểu PIM của QT10.
- Đẩy nhật ký kiểm toán liên tục sang hệ ngoài (SIEM) và lịch xuất định kỳ (QT18 chỉ xuất theo yêu cầu).
- Hạn mức hay tài nguyên riêng theo người dùng (GĐ21), lịch sử kết nối và IP của runtime (GĐ22), bảng tổng hợp tài nguyên theo tenant (GĐ23).
- `admin` gắn nhiều tenant, `admin` tạo tenant mới hay tạo staff khác, và vai trò staff tùy biến ngoài bốn vai trò ở 3.5.
- Mở quyền thanh toán, tạo tài khoản, thao tác cấp tài khoản cho `admin` (GĐ18) và quyền xóa tenant hay tài khoản cho `admin` (GĐ14, GĐ15).

## 15. Chưa kiểm chứng và rủi ro

### 15.1 Đã xác nhận bằng đọc mã

- `workspaceprovision.Create` đã tồn tại và là nơi chung của `CreateWorkspace` và việc nền `account_provisioning` (`server/internal/workspaceprovision/provision.go:40`).
- `LockPlanGate` nhận `Scope` là chuỗi tùy ý, nên dùng được cho id tenant (`server/pkg/db/queries/plan_limit.sql:1`).
- `DetachWorkspaceAuditEvents` giữ dòng kiểm toán sau khi workspace bị xóa (`server/pkg/db/queries/workspace_delete.sql:777`).
- `platform_staff.role` là cột `text` không có `CHECK` (`server/migrations/663_platform_staff.up.sql:1`); tập vai trò do mã kiểm bằng `ParseRole`, và bản platform cũ coi vai trò lạ là không tìm thấy (`platform-management/internal/platformauth/platformauth.go:55`). Hai điều này quyết định cách làm ở 3.8 và thứ tự triển khai ở mục 12.
- Gói đã có CRUD, lưu trữ, xóa có chặn `plan_in_use` và kiểm toán `plan.changed` có `changes` (`platform-management/internal/subscription/console.go:380`, `:461`, `:593`, `:664`), nên QT1 chỉ bổ sung phần thiếu. Model runtime không có lịch sử kết nối (`server/migrations/004_agent_runtime_loop.up.sql:1`: chỉ `status`, `last_seen_at`). Mã không có hạn mức theo người dùng (mọi cổng đếm theo workspace, gốc 5.7).
- Năm bảng route có danh sách vai trò viết tay chứa `RoleAdmin` và phải đổi cùng nhau ở slice S0: `platform-management/internal/console/handler.go:111`, `platform-management/internal/subscription/console.go:81`, `platform-management/internal/pricechange/handler.go:65`, `platform-management/internal/billing/reconcile.go:135`, `platform-management/internal/console/orders.go:323`.
- Giao thức ghế của platform lũy đẳng theo `token` (gốc Phụ lục B quy tắc 1), nên GĐ7 đứng được.

### 15.2 Con số là lựa chọn, chưa đo

Giá trị `max_workspaces` ở mục 5.6, nhịp một phút của hai việc nền, giới hạn 500 ký tự của `reason`, và số lần thử 3. Không có số liệu đo.

### 15.3 Rủi ro

| # | Rủi ro | Xử lý |
| --- | --- | --- |
| a | **Đường thêm người là lối vào dữ liệu khách.** Một staff `system_admin` hoặc `admin` (trong tenant mình) bị chiếm hoặc xấu có thể thêm email của kẻ tấn công vào workspace của khách; `system_admin` bị chiếm làm được trên mọi tenant. Biện pháp ở 4.6 giảm thiệt hại (MFA, kiểm toán chủ workspace thấy, không `owner`) nhưng không ngăn hoàn toàn | Chấp nhận theo quyết định 1 của chủ dự án. Nếu cần chặt hơn: đòi hai staff `system_admin` duyệt, hoặc đòi chủ workspace xác nhận qua email; mỗi cái là một đợt riêng |
| b | `accountdeletion.Classify` chặn mọi owner của workspace còn người khác, kể cả khi có đồng owner, chặt hơn gốc 4.6. Đường staff không dùng nó (7.3) nên hai đường có thể khác kết quả với cùng một người | Không sửa đường tự phục vụ ở đợt này (ngoài phạm vi); ghi lại để chủ dự án quyết định |
| c | "Xóa ngay" không đồng bộ: giữa pha 1 và pha 2 (tối đa khoảng một phút cộng thời gian xóa) dữ liệu còn trong cơ sở dữ liệu dù không truy cập được | GĐ8; nếu pháp lý đòi đồng bộ, đổi D35 và D47 |
| d | Xóa không hoàn tác được. Hủy chỉ có tác dụng trước khi việc nền bắt đầu chạy | Gõ lại tên, lý do, danh sách đích trên console; ngoại lệ I10 ghi ở 1.3 |
| e | Việc nền `account_deletion` ở mã chạy hằng ngày (`server/internal/scheduler/jobs_account_deletion.go:33`) còn bảng 3.5 của spec gốc ghi "giờ"; không ảnh hưởng đợt này vì đường staff có việc nền riêng | Ghi nhận; sửa chữ trong spec gốc không thuộc đợt này |
| f | Độ trễ giới hạn: đổi gói của một workspace làm giới hạn các workspace anh em trễ khoảng 5,5 phút (5.6) | Chấp nhận như gốc 5.5 cho mọi chính sách |
| g | Thứ tự migration bốn chữ số (5.4): nếu sau này có migration của R6 cần bảng tạo ở tệp số từ `101` trở lên, nó sẽ lỗi trên cơ sở dữ liệu mới | Quy tắc ở 5.4; `TestMigrationNumericPrefixesAreUnique` không bắt lỗi này, nên slice S1 thêm test tĩnh `TestFourDigitMigrationsOnlyTouchTablesCreatedEarlierInSortOrder` (không cần cơ sở dữ liệu) |
| h | Tài liệu này chưa chạy một test nào; đây là đặc tả. Trích dẫn mã đã kiểm bằng script (tệp tồn tại, dòng chứa ký hiệu), không phải bằng chạy | Mỗi AC là một test viết ở kế hoạch bổ sung |
| i | **Quyền `admin` đổi nghĩa.** Mọi `admin` hiện có thành `system_admin` (GĐ16). Nếu chủ dự án muốn một số người chỉ quản trị một tenant, phải hạ họ bằng `PUT /staff/{user_id}`; giữa lúc migration và lúc hạ, họ có quyền toàn nền tảng | Ghi trong bảng kiểm triển khai (mục 12, bước 5) |
| j | Phạm vi `admin` là kiểm tra ở ba lớp mới viết tay (3.5 quy tắc 4); một route mới quên lọc sẽ rò dữ liệu giữa tenant | Test 20 chạy trên mọi route có ô "tenant mình"; test duyệt bảng route đòi mỗi route mới có dòng ở 3.6 |
| k | Cột `audit_event.tenant_id` chỉ có ở dòng R6 và dòng staff; dòng kiểm toán cũ của workspace đã bị xóa không có cả `workspace_id` lẫn `tenant_id` nên `admin` không thấy chúng (GĐ17) | Chấp nhận; không backfill |
| l | Quan hệ xếp bậc cộng dồn bị bỏ: một route cũ chỉ dùng danh sách vai trò có `RoleAdmin` sẽ không tự có `system_admin` nếu quên đổi | Slice S0 đổi cả năm bảng route và `TestPermissionMatrix` so mỗi route với 3.6 |
| m | Giữ hồ sơ pháp lý chỉ chặn đường xóa đã biết; một đường xóa dữ liệu mới thêm sau mà không qua `deleteWorkspaceData` hay `DeleteAccount` sẽ không bị chặn | Test 24 liệt kê mọi đường xóa; test duyệt mã đòi mọi `DELETE` trên bảng nội dung workspace nằm trong `deleteWorkspaceData` (AC137) |
| n | Vô hiệu hóa runtime không dừng task đang chạy; một task dài tiếp tục đến khi xong | Chấp nhận như cơ chế dừng của D25; gỡ đăng ký qua `DeleteAgentRuntime` hủy task như đường xóa runtime hiện có |
| o | Duyệt chéo mặc định tắt nên bảo vệ thực tế chỉ có khi vận hành bật cấu hình | Ghi trong bảng kiểm triển khai (kế hoạch S38); GĐ25 |
| p | Tổng hợp tài nguyên toàn hệ thống đọc theo trang nên tổng chính xác đòi duyệt hết các trang; số đầu trang là tạm | Phản hồi trả `complete` (đã hết trang hay chưa); chấp nhận ở quy mô hiện nay (GĐ23) |
