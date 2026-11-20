# Đặc tả áp dụng ngôn ngữ thiết kế Astryx cho phân hệ `platform-management`

Ngày viết: `2026-11-17`
Ngày rà soát lại hiện trạng mã nguồn: `2026-11-19` (commit `354a57584` trên nhánh `main`)
Nguồn tham chiếu thiết kế: [`https://github.com/meta/astryx`](https://github.com/meta/astryx) (đọc từ bản clone tại `$HOME/.cache/claude-delegate/astryx-src`, phiên bản `@astryxdesign/core@0.7.17` / `@astryxdesign/theme-neutral@0.7.17`, đã kiểm tra lại khớp `packages/core/package.json`).
Nguồn đặc tả nghiệp vụ nền: [`docs/engineering/platform-management-spec.md`](../engineering/platform-management-spec.md) và [`docs/engineering/platform-management-plan.md`](../engineering/platform-management-plan.md).
Kế hoạch triển khai đi kèm: [`plan.md`](plan.md).

> **Ghi chú rà soát 2026-11-19:** Mục 1 và 3–4 đã được đối chiếu với mã nguồn hiện tại. Các mục đã được sửa so với bản `2026-11-17` được đánh dấu **[Đã xong]**, **[Còn thiếu]** hoặc **[Mới]**. Mục 2 (ngôn ngữ thiết kế Astryx) không đổi vì tham chiếu vẫn ở cùng phiên bản.

---

## 1. Hiện trạng phân hệ `platform-management`

### 1.1 Kiến trúc triển khai và cổng dịch vụ (bất biến)

Phân hệ `platform-management` gồm dịch vụ độc lập ở `platform-management/` (Go module riêng + SPA console) và các mặt giao diện phía tenant trong `packages/views/`:

| Thành phần | Cổng cố định | Vị trí mã nguồn | Vai trò |
| --- | --- | --- | --- |
| Landing | `30002` | `Dockerfile.landing`, `docker-compose.landing.yml` | Trang giới thiệu |
| Multica Web | `30000` | `apps/web/` + `packages/views/` | Ứng dụng web cho người dùng cuối & chủ workspace |
| Platform Console | `30001` | `platform-management/console/` | SPA quản trị cho staff (`admin`, `billing`, `support`), phục vụ tĩnh bởi `platform-server` và proxy `/api/platform/*` về cổng API |
| Platform API | `8095` | `platform-management/cmd/platform-server/`, `platform-management/internal/` | HTTP API nội bộ cho Multica server (`/api/v1/internal/*`, `/api/v1/subscriptions/*`) và cho Console (`/api/platform/*`) |

> **Ràng buộc cố định:** Không thay đổi bất kỳ cổng nào trong bốn cổng trên (`30002`, `30000`, `30001`, `8095`) hoặc cơ chế proxy `/api/platform/` tại `platform-management/internal/server/server.go:116-134`.

### 1.2 Phụ thuộc gói và ranh giới kiến trúc

| Gói / Thư mục | Phụ thuộc trực tiếp (`package.json`) | Ranh giới từ `AGENTS.md` và `D37` |
| --- | --- | --- |
| `platform-management/console` (`@multica/platform-console`) | `@multica/core`, `@multica/ui`, `@tanstack/react-query`, `zod`, `shadcn`, `tw-animate-css` | Chỉ dùng `@multica/core` và `@multica/ui`. **Không** phụ thuộc `@multica/views`, `next/*`, `react-router-dom`. |
| `packages/views/billing/` & `packages/views/settings/components/` | `@multica/core`, `@multica/ui`, `react-i18next`, `sonner` | Dùng cho cả `apps/web` và `apps/desktop`. Không định nghĩa store Zustand, không import `next/*` hay `react-router-dom` (dùng `useNavigation()`, `<AppLink>`). |
| `packages/ui/` (`@multica/ui`) | Primitives Base UI / shadcn, `@multica/ui/icons`, `packages/ui/styles/tokens.css` | Không chứa logic nghiệp vụ, không import `@multica/core`. Mọi icon lấy từ `@multica/ui/icons`. |

**[Đã xong] Quét CSS dùng chung:** `platform-management/console/src/styles.css` đã thêm `@source "../../../packages/ui"` (commit `6d7d1d646`), nên các class của `Dialog`, `Table`, `Button` trong `packages/ui` được sinh đầy đủ.

### 1.3 Kiểm kê màn hình, thành phần và route hiện có

#### A. Platform Console (`platform-management/console/src/`) — Cổng `30001`

| Màn hình / Thành phần | Tệp hiện tại | Route API (`/api/platform/...`) | Trạng thái hiện tại (rà soát 2026-11-19) |
| --- | --- | --- | --- |
| Shell & điều hướng | `App.tsx`, `components/Layout.tsx` | `GET /me`, `POST /auth/logout` | **[Còn thiếu]** Vẫn là top header ngang (`header` `sticky`, `h-14`), không phải `AppShell` + `SideNav`. **[Còn thiếu]** Điều hướng bằng state `currentScreen` trong bộ nhớ, `App.tsx` không dùng `pushState`/`popstate` nên F5 mất màn hình. **[Đã xong]** Lỗi nối dây cũ đã sửa (commit `1a1293ddc`): `App.tsx` khai báo và render `orders`, `plans`, `accounts`, `payment-notices`, `einvoices`, `staff`, `audit` kèm kiểm tra vai trò trong `handleNavigate` và `renderScreen()`. |
| Đăng nhập Staff | `screens/Login.tsx` | `POST /auth/send-code`, `POST /auth/verify-code` | Ba bước (email → mã 6 số → TOTP MFA). Vẫn bọc trong `Card`. **[Còn thiếu]** Chưa áp dụng mẫu `login-sso`. |
| Tổng quan (Overview) | `screens/Overview.tsx` | `GET /overview` | **[Còn thiếu]** Vẫn dùng `text-title-xl` (không có trong `tokens.css`) ở `Overview.tsx:71,85,99,113,127,141` và màu cứng `text-emerald-600` (`:82`, `:85`). Chưa hiển thị `pending_orders`, `by_plan`, `usage_*` dù `OverviewSchema` đã có. Có 24 chỗ dùng `<Card`. |
| Danh sách Tenant | `screens/Tenants.tsx` | `GET /tenants?q=&cursor=` | Ô tìm kiếm + `Table` + `Load more`. Chưa đối chiếu chi tiết các thay đổi từ lần rà soát trước. |
| Chi tiết Tenant | `screens/TenantDetail.tsx` | `GET /tenants/{id}`, `POST /tenants/{id}/status`, `GET/POST /tenants/{id}/notes`, `PUT /tenants/{id}/subscription`, `GET /tenants/{id}/audit-events`, `GET /tenants/{id}/usage`, `GET/PUT /tenants/{id}/billing-profile`, `POST .../verify`, `POST .../unverify` | **[Mới]** Đã có hook `useTenantUsage`, `useTenantBillingProfile`, `useUpdateTenantBillingProfile`, `useSetBillingProfileVerified`, và `TenantDetail.tsx` đã gọi các hook này. Cần kiểm tra lại xem bảng dòng phí theo ngày có được render hay chưa. **[Còn thiếu]** Vẫn có 38 chỗ `<Card` (layout "Card soup"), 3 `<select>` HTML thô (`TenantDetail.tsx:1106`, `:1168`, `:1245`), và badge màu cứng `emerald-*` (`:890`). |
| Danh sách Người dùng | `screens/Users.tsx` | `GET /users?q=&cursor=` | Ô tìm kiếm + `Table` + `Load more`. Có `EmptyState`. |
| Chi tiết Người dùng | `screens/UserDetail.tsx` | `GET /users/{id}`, `POST /users/{id}/status`, `POST /users/{id}/revoke-sessions`, `POST /users/{id}/mfa-reset` | **[Còn thiếu]** Vẫn có 10 chỗ `<Card`, chưa dùng `PageHeader`/`MetadataList`. |
| Đơn hàng (Orders) | `screens/Orders.tsx` | `GET /orders?status=`, `POST /orders/{id}/confirm`, `POST /orders/{id}/cancel` | **[Đã xong]** Đã nối vào `App.tsx` cho `billing` và `admin`. **[Còn thiếu]** Vẫn dùng `<button role="tab">` tự chế (`Orders.tsx:200`). Chưa hiển thị `order.kind` (`subscribe`, `change_plan`, `add_seats`, `renew`, `usage`) và chưa có nhãn `Write off` cho đơn `usage`. Icon `Check` vẫn là `text-emerald-500` (`:271`). |
| Danh mục Gói (Plans) | `screens/Plans.tsx` | `GET /plans`, `PUT /plans/{key}`, `DELETE /plans/{key}`, `GET/POST /plans/{key}/price-changes`, `DELETE /plans/{key}/price-changes/scheduled` | **[Đã xong]** Đã nối vào `App.tsx` cho `admin`. **[Đã xong]** `handleSavePlan` không còn gửi `seat_price_month`/`seat_price_year` (`Plans.tsx:180-190` chỉ gửi `currency`, `name`, `trial_days`, `limits`, `features`, `seats_fixed`, `min_seats`, `is_public`, `sort`, `archived`). **[Đã xong]** Đã có `PriceSchedule.tsx` (được import vào `Plans.tsx:27`) với `dry_run` và `confirm_large_change`, và `Plans.tsx` có nút `CalendarClock`. **[Còn thiếu]** Hai `<input type="checkbox">` thô (`Plans.tsx:470`, `:481`). Chưa hiển thị cột `audience` và `billing_model`. |
| Lịch đổi giá (Price schedule) | `screens/PriceSchedule.tsx` | `GET/POST /plans/{key}/price-changes`, `DELETE .../scheduled` | **[Mới]** Có dialog lập lịch, xem trước tác động, cảnh báo đổi lớn (`confirm_large_change`, nền `amber`). **[Còn thiếu]** Dùng `amber-*`/`emerald-*` cứng (`:510-513`, `:561`), `<input type="checkbox">` thô (`:521`). |
| Tài khoản (Accounts) | `screens/Accounts.tsx` | `GET/POST /accounts`, `GET /accounts/{id}`, `POST /accounts/{id}/cancel`, `POST /accounts/{id}/resend-welcome` | **[Mới]** Đã có màn hình, hook `useAccountRequests`, `useCreateAccountRequest`, `useCancelAccountRequest`, `useResendWelcome`, và nối vào `App.tsx` cho `admin`. **[Còn thiếu]** Bộ lọc trạng thái dùng `role="tab"` tự chế (`Accounts.tsx:534`), có 3 `<select>` thô (`:689`, `:830`, `:883`), 1 checkbox thô (`:908`), và badge/cảnh báo màu cứng `amber-*`/`emerald-*` (`:170`, `:185`, `:512`). |
| Thông báo thanh toán (Payment notices) | `screens/PaymentNotices.tsx` | `GET /payment-notices`, `GET /payment-notices/{id}`, `POST .../resolve`, `POST .../dismiss` (theo hook `usePaymentNotices`, `usePaymentNotice`, `useResolvePaymentNotice`, `useDismissPaymentNotice`) | **[Mới]** Đã nối vào `App.tsx` cho `billing` và `admin`, dùng `Table`. Chưa được đối chiếu với Astryx. |
| Hóa đơn điện tử lỗi (E-invoices) | `screens/EinvoiceFailures.tsx` | `GET /einvoices`, `GET /orders/{id}/einvoices`, route retry | **[Mới]** Đã nối vào `App.tsx` cho `billing` và `admin`. Có tab bằng `role="tab"` tự chế (`EinvoiceFailures.tsx:94`). |
| Nhân sự (Staff) | `screens/Staff.tsx` | `GET /staff`, `POST /staff`, `DELETE /staff/{user_id}` | **[Đã xong]** Đã nối vào `App.tsx` cho `admin`. **[Còn thiếu]** `<select>` HTML thô (`Staff.tsx:212`). |
| Nhật ký kiểm toán (Audit) | `screens/Audit.tsx` | `GET /audit-events?action=&actor_id=&from=&to=&cursor=` | **[Đã xong]** Có `EmptyState`. **[Còn thiếu]** `<select>` HTML thô (`Audit.tsx:68`). `CATALOG_ACTIONS` (`components/types.ts:104`) vẫn là danh sách riêng, chưa tái xuất từ `AUDIT_ACTIONS` (`api/schemas.ts`). |
| Thành phần dùng chung Console | `components/StatusBadge.tsx`, `ConfirmReasonDialog.tsx`, `EmptyState.tsx`, `ErrorState.tsx`, `LoadingState.tsx`, `Layout.tsx` | — | **[Còn thiếu]** `StatusBadge` vẫn dùng `border-emerald-600/30 bg-emerald-50 text-emerald-700` (`:9`), và chưa có ánh xạ cho trạng thái đơn hàng, thuê bao, yêu cầu tài khoản. **[Còn thiếu]** `EmptyState` chỉ có `title`, `description` (chưa có `icon`, `actions`, `isCompact`). **[Còn thiếu]** Chưa có `PageHeader.tsx` và `MetadataList.tsx`. |

#### B. Tính năng Backend Console đã có nhưng trạng thái trên Console còn khác nhau

| Nhóm tính năng | Backend API đã có trong `platform-management/internal/` | Trạng thái trên Console UI (rà soát 2026-11-19) |
| --- | --- | --- |
| Lịch thay đổi giá gói (Q12/Q13) | `GET/POST /api/platform/plans/{key}/price-changes` (hỗ trợ `?dry_run=1`), `DELETE /api/platform/plans/{key}/price-changes/scheduled` (`internal/pricechange/handler.go`); `PUT /api/platform/plans/{key}` chặn các trường `PriceFields` (`pricechange.go:47`: `seat_price_month`, `seat_price_year`, `usage_rates`, `usage_terms`) bằng `RefusePriceFields` (`cmd/platform-server/main.go:379`) | **[Đã xong]** Đã có `PriceSchedule.tsx`. **[Đã xong]** `PUT` không còn gửi trường giá. **[Lưu ý]** `currency` **không** nằm trong `PriceFields`, nên việc gửi `currency` trong `PUT` là hợp lệ và có chủ đích (kèm lỗi `plan_currency_locked` đã được xử lý trong `Plans.tsx`). Bản `2026-11-17` nêu sai là phải bỏ `currency`. **[Còn thiếu]** Chưa thấy nhãn lịch sử giá (`Price history`) có quyền `billing` (xem `ASSUMPTION-4`). |
| Yêu cầu tạo tài khoản bởi Admin (Q14–Q16) | `GET/POST /api/platform/accounts`, `GET /api/platform/accounts/{id}`, `POST .../cancel`, `POST .../resend-welcome` (`internal/console/handler.go:138-143`) | **[Đã xong]** Đã có `Accounts.tsx`, hook và nút gửi lại thư chào (`useResendWelcome`). |
| Mức dùng & xóa nợ Usage của Tenant (Q10/Q11) | `GET /api/platform/tenants/{id}/usage` (`internal/console/handler.go`), `POST /api/platform/orders/{id}/cancel` (nhánh `writeOffUsage` chỉ cho `admin`, `orders.go:252-255`) | **[Một phần]** Đã có `useTenantUsage` (`hooks.ts:289`). **[Còn thiếu]** `Orders.tsx` chưa có nhãn `Write off`. |
| Hồ sơ thanh toán & xác minh doanh nghiệp (Q2) | `GET/PUT /api/platform/tenants/{id}/billing-profile`, `POST .../verify`, `POST .../unverify` | **[Đã xong]** `TenantDetail.tsx` đã dùng các hook hồ sơ thanh toán và xác minh. Cần rà soát trực quan theo Astryx. |
| Hóa đơn điện tử (P43) | `GET /api/platform/einvoices`, `GET /api/platform/orders/{id}/einvoices`, retry | **[Đã xong]** Có `EinvoiceFailures.tsx`. |
| Thông báo thanh toán (P42) | Hàng đợi `payment_notice` và route xử lý | **[Đã xong]** Có `PaymentNotices.tsx`. |

#### C. Giao diện phía Tenant thuộc phân hệ `platform-management` (`packages/views/`) — Cổng `30000` (Web) & Desktop

| Màn hình / Thành phần | Tệp hiện tại | Trạng thái hiện tại (rà soát 2026-11-19) |
| --- | --- | --- |
| Cài đặt Thanh toán & Thuê bao | `packages/views/settings/components/billing-tab.tsx` | **[Đã xong]** Dùng `SettingsTab`, `SettingsSection`, `SettingsCard`, `SettingsRow`. Đã tích hợp `BillingProfileForm` (`billing-profile-form.tsx`), `PendingChangeBanner`, `PlanChangeDialog`, `SubscriptionLifecycleSection`, `plan-audience.ts`. **[Còn thiếu]** Chưa thấy thẻ mức dùng `usage` và biểu ngữ `scheduled_price_change` (Q11, Q13) trong `billing-tab.tsx`. |
| Trang Đơn hàng chuyển khoản | `packages/views/billing/order-page.tsx` (567 dòng) | **[Còn thiếu]** Vẫn dùng 3 `Card` (`:409`, `:475` và một khối thứ ba). Vẫn có `text-title-md` (`:411`, `:449`, `:477`) và `bg-surface-muted` (`:444`), là các lớp không có trong `tokens.css`. |
| Nhật ký kiểm toán Workspace | `packages/views/settings/components/audit-log-tab.tsx` | Dùng `SettingsTab`, bộ lọc `Select` + 2 `Input` ngày, `Table`, xuất CSV. Chưa có khung `rounded-lg border` bọc bảng và chưa có nút `Clear filters` khi bộ lọc không có kết quả. |
| Bảo mật tài khoản, Phiên, Xuất & Xóa tài khoản | `packages/views/settings/components/account-tab.tsx`, `packages/views/account/deletion-gate.tsx`, `packages/views/legal/consent-gate.tsx` | Đã dùng `SettingsTab`/`SettingsCard`/`SettingsRow` và `Dialog`. Danh sách `securityEvents` vẫn là `<ul>/<li>` thủ công. |
| Token cá nhân (PAT) | `packages/views/settings/components/tokens-tab.tsx` (439 dòng) | **[Còn thiếu]** Vẫn "Card soup": mỗi token là một `Card` (`:289`), và các khối `Card` ở `:186`, `:263`, `:275`. Vẫn có `<details>/<summary>` thô (`:180`). |

---

## 2. Ngôn ngữ thiết kế Astryx trích xuất từ repo tham chiếu

*(Không thay đổi so với bản `2026-11-17`; đã kiểm tra lại phiên bản `0.7.17`.)*

Từ mã nguồn `@astryxdesign/core` và `@astryxdesign/theme-neutral` tại `$HOME/.cache/claude-delegate/astryx-src`:

### 2.1 Triết lý và 8 quy tắc cốt lõi (`packages/cli/assets/docs/principles.doc.mjs`)

1. **Components over primitives:** Dùng component chuẩn cho mọi mẫu tương tác/hiển thị trước khi dùng thẻ HTML thô (`<select>`, `<input type="checkbox">`, `<button role="tab">`, `<div>` flex tự chế).
2. **Outside-in, frame-first layout (`docs/tree/scaffold.doc.mjs`):** Chốt khung trang (`AppShell` → `SideNav` / `TopNav` → `Layout` với `header`, `start`, `content`, `end`, `footer`) và ngân sách chiều rộng (width budget) trước khi đặt nội dung:
   - `SideNav`: `240px–280px` (mặc định `256px`/`260px`, hỗ trợ thu gọn `collapsible` thành rail `64px–72px`).
   - `LayoutPanel` (side panel chi tiết master-detail ở khe `end`): `340px–420px` (khuyến nghị `380px`).
   - Vùng nội dung (`LayoutContent`): `fill` (100% chiều rộng) cho bảng dữ liệu, biểu đồ, bảng điều khiển; `capped` (`contentWidth={640}` cho biểu mẫu/cài đặt tập trung văn bản, `contentWidth={960}` hoặc `1280` cho trang hỗn hợp). Dividers luôn chạy tràn lề (`full-bleed`).
3. **Thang bậc vùng chứa từ yếu đến mạnh (`docs/tree/structure.doc.mjs`):**
   - Bậc 1 — `spacing` & `gap`: nhóm các phần tử liên quan (tight gap `4px–8px` bên trong nhóm, generous gap `16px–24px` giữa các nhóm).
   - Bậc 2 — `Divider` (`Separator`): phân tách các mục đồng cấp trong danh sách/thanh công cụ hoặc ngăn cách header/footer cố định với thân cuộn.
   - Bậc 3 — `Section`: đơn vị cấu trúc trang mặc định dưới một tiêu đề (`Heading` + `Stack`), **không viền**.
   - Bậc 4 — `Card`: **chỉ** dùng cho widget độc lập (ô chỉ số KPI, biểu đồ, thẻ tác vụ có thể tách rời). **Tuyệt đối tránh "Card soup"** (bọc từng bản ghi hoặc từng phần của trang trong một `Card` riêng).
4. **Dữ liệu dày đặc hiển thị dạng hàng (`Table`, `List`/`Item`):** Tập bản ghi hiển thị bằng `Table` (nhiều cột) hoặc `List` (một dòng), tràn cạnh vùng chứa với đường kẻ ngang (`dividers="rows"`), chiều cao hàng `32px–40px`.
5. **Một đường căn lề nội dung duy nhất mỗi vùng (`docs/tree/layout-spacing.doc.mjs`):**
   - Công thức: `container_inset = content_line - component_intrinsic_inset`.
   - `Heading` và `Text` không có inset nội tại nên nằm trong vùng có padding; `Table` và `List` đã có padding ô (`12px–16px`), nên khi đặt trong một vùng hoặc khung có viền thì vùng chứa phải đặt `padding={0}` để chữ trong cột đầu tiên thẳng hàng dọc với tiêu đề vùng phía trên.
6. **Mật độ (`density`) đi đôi với kích thước điều khiển (`size`):**
   - `compact`: bảng nhật ký, giám sát, tập dữ liệu lớn → đi cùng control `sm` (`28px`).
   - `balanced`: bảng quản trị và danh sách thông thường → đi cùng control `sm` hoặc `md` (`32px`).
   - `spacious`: cài đặt tần suất thấp hoặc quyết định quan trọng → đi cùng control `md` hoặc `lg` (`36px`).
   - Mọi điều khiển trên cùng một hàng ngang phải có cùng chiều cao.
7. **Thang bậc chữ 4 mức (`docs/tree/structure.doc.mjs`):**
   - Mỗi vùng chỉ có **một** tiêu đề dẫn (`Lead`: `Heading` theo đúng cấp độ outline `h1→h2→h3` hoặc `Text` đậm).
   - Phân cấp bằng **độ đậm (`weight`) và màu sắc (`primary` vs `secondary`)**, không lạm dụng việc thu nhỏ cỡ chữ. Chỉ dùng 2 mức màu chữ cho nội dung đọc: `primary` và `secondary` (không dùng màu `disabled` cho văn bản).
   - Số liệu thống kê, tiền tệ, bộ đếm thời gian luôn bật `tabular-nums` (`hasTabularNumbers`). Mã định danh dài (`id`, `slug`, `transfer_code`) cắt 1 dòng kèm tooltip.
8. **Sử dụng `Badge` và `StatusDot` có kỷ luật (`Badge.doc.mjs`, `StatusDot.doc.mjs`):**
   - Không gắn `Badge` màu xanh lá rực ("Active") cho mọi dòng bình thường trong bảng vì làm chìm các dòng cần chú ý (`Suspended`, `Pending Deletion`, `Expired`).
   - Trạng thái vận hành bình thường hoặc độ ưu tiên dùng `StatusDot` + nhãn văn bản (hoặc badge sắc độ dịu `neutral`/`green` tint); dành `Badge` nổi bật cho trạng thái ngoại lệ cần hành động hoặc phân loại danh mục.
   - Không dùng `Badge` cho siêu dữ liệu như số lượng ghế, ngày tháng, thời lượng (dùng `Text` phụ).

### 2.2 Hệ thống Token của Astryx và đối chiếu với `@multica/ui`

#### A. Màu sắc & Bề mặt (Surface Hierarchy)

Astryx định nghĩa phân tầng bề mặt `body → surface → card → popover` thích ứng sáng/tối. Đối chiếu với `packages/ui/styles/tokens.css`:

| Vai trò trong Astryx (`tokens.stylex.ts`) | Giá trị mẫu Astryx (`neutral` / default) | Token tương ứng trong `@multica/ui` (`tokens.css`) | Lớp Tailwind sử dụng trong repo |
| --- | --- | --- | --- |
| `--color-background-body` | `#F1F4F7` (light) / `#111112` (dark) | `--app-shell` / `--page-canvas` (`--background`) | `bg-app-shell`, `bg-page-canvas`, `bg-background` |
| `--color-background-surface` | `#FFFFFF` / `#1F1F22` | `--surface` | `bg-surface` |
| `--color-background-card` | `#FFFFFF` / `#1F1F22` | `--card` (`var(--surface)`) | `bg-card`, `bg-surface` |
| `--color-background-popover` | `#FFFFFF` / `#28292C` | `--popover` (`var(--surface-raised)`) | `bg-popover`, `bg-surface-raised` |
| `--color-background-muted` | `#0536590C` / `#1111127F` | `--muted` | `bg-muted` |
| `--color-overlay-hover` | `#0536590C` / `#FFFFFF0C` | `--surface-hover` | `hover:bg-surface-hover` |
| `--color-accent-muted` (selected) | `#0082FB33` / neutral tint | `--surface-selected` | `bg-surface-selected` |
| `--color-text-primary` | `#0A1317` / `#DFE2E5` | `--foreground` / `--surface-foreground` | `text-foreground` |
| `--color-text-secondary` | `#4E606F` / `#AAAFB5` (≥ 4.5:1 AA) | `--muted-foreground` (pinned ≥ 4.88:1 AA) | `text-muted-foreground` |
| `--color-icon-secondary` / non-text mark | `#4E606F` / `#AAAFB5` | `--faint-foreground` (≥ 3:1 non-text) | `text-faint-foreground` (chỉ dùng cho icon/dấu gạch, không dùng cho chữ) |
| `--color-accent` | `#0064E0` / `#0171E3` (hoặc neutral ink) | `--primary` / `--brand` | `bg-primary`, `text-brand` |
| `--color-border` | `#05365919` / `#F2F4F619` | `--border` / `--surface-border` | `border-border`, `border-surface-border` |
| `--color-success`, `--color-warning`, `--color-error`, `--color-info` | Semantic status tokens | `--success`, `--warning`, `--destructive`, `--info` | `text-success`, `text-warning`, `text-destructive`, `text-info` (kèm tint `/10`, `/15` cho nền badge/banner) |

> **Quy tắc bắt buộc từ `AGENTS.md`:** Giữ nguyên các biến ngữ nghĩa của `packages/ui/styles/tokens.css` để tương thích cả 3 theme (`light`, `dark`, `blue-white`). Tuyệt đối không viết màu Tailwind cứng (`emerald-*`, `amber-*`, `red-*`, `#hex`) trong các màn hình `platform-management`.

**[Còn thiếu] Các vị trí màu cứng hiện còn** (cần thay bằng token ngữ nghĩa): `StatusBadge.tsx:9`, `Overview.tsx:82,85`, `Orders.tsx:271`, `TenantDetail.tsx:890`, `PriceSchedule.tsx:510-513,561`, `Accounts.tsx:170,185,512`.

#### B. Thang chữ (Typography Scale)

Astryx dùng thang nhân học `base = 14px, ratio = 1.2` (`tokens.stylex.ts:362-477`). Repo Multica quy định **chỉ dùng thang `--text-*` theo vai trò** trong `packages/ui/styles/tokens.css:94-128`. Bảng ánh xạ chuẩn:

| Vai trò trong Astryx (`Text` / `Heading`) | Cỡ / Dòng trong Astryx | Token bắt buộc trong Multica (`tokens.css`) | Lớp Tailwind hợp lệ | Cách dùng trong `platform-management` |
| --- | --- | --- | --- | --- |
| `Heading level={1}` (Page title) | `24px / 32px`, `semibold` | `--text-display-sm` (`24px / 32px`) hoặc `--text-title-lg` (`20px / 28px`) | `text-display-sm font-semibold` (trang độc lập) hoặc `text-title-lg font-semibold` (header thanh gọn) | Tiêu đề chính của màn hình (`PageHeader`) |
| `Heading level={2}` (Card / Dialog / Major section) | `20px / 28px`, `semibold` | `--text-title-lg` (`20px / 28px`) hoặc `--text-title` (`18px / 28px`) | `text-title-lg font-semibold` / `text-title font-semibold` | Số liệu KPI trong thẻ Overview, tiêu đề Dialog |
| `Heading level={3}` (Section heading) | `17px / 24px`, `semibold`/`bold` | `--text-title-sm` (`16px / 24px`) | `text-title-sm font-semibold` | Tiêu đề mỗi `Section` (VD: *Subscription*, *Usage*, *Internal Notes*) |
| `Heading level={4}` (Subsection / Card title) | `14px / 20px`, `semibold` | `--text-body` (`14px / 20px`) | `text-body font-semibold` | Tiêu đề nhóm nhỏ, nhãn thẻ KPI |
| `Text type="body"` (Mặc định) | `14px / 20px`, `normal` (`400`) | `--text-body` (`14px / 20px`) | `text-body` | Văn bản nội dung, ô bảng, input |
| `Text type="large"` (Roomy prose) | `17px / 24px` | `--text-body-lg` (`15px / 22px`) | `text-body-lg` | Mô tả mở đầu trong Dialog / Onboarding |
| `Text type="label"` (Field/Row label, Table header) | `14px / 20px`, `medium` (`500`) | `--text-label` (`13px / 18px`) | `text-label font-medium` | Nhãn trường biểu mẫu, tiêu đề cột `TableHead`, hàng thuộc tính `MetadataList`, tab item |
| `Text type="supporting"` (Helper, metadata, ID) | `12px / 20px`, `secondary` | `--text-caption` (`12px / 16px`) | `text-caption text-muted-foreground` | Chú thích dưới trường, slug, UUID, mô tả phụ |
| `Badge` / Counter / Overline | `10px–11px / 15px–16px` | `--text-micro` (`11px / 15px`) | `text-micro` | Nhãn badge nhỏ, bộ đếm, dấu thời gian nhỏ |
| `Text type="code"` | `14px / 20px` mono | `--font-mono` + `--text-caption` / `--text-label` | `font-mono text-caption` / `font-mono text-label` | `transfer_code`, `action` kiểm toán, `slug`, `workspace_id`, `token_prefix` |

> **[Còn thiếu] Lớp không tồn tại trong `tokens.css` hiện còn trong mã:** `text-title-xl` (`Overview.tsx:71,85,99,113,127,141`), `text-title-md` (`order-page.tsx:411,449,477`), `bg-surface-muted` (`order-page.tsx:444`). Phải thay bằng `text-display-sm` / `text-title-lg` / `text-title-sm` / `bg-muted`.

#### C. Khoảng cách, Bo góc, Đổ bóng và Kích thước phần tử

| Nhóm Token | Quy ước Astryx (`tokens.stylex.ts`) | Quy ước tương ứng trong `@multica/ui` (`tokens.css`) |
| --- | --- | --- |
| **Spacing** | Lưới `4px` (`0.25rem`): `--spacing-1` (`4px`), `--spacing-2` (`8px`), `--spacing-3` (`12px`), `--spacing-4` (`16px`), `--spacing-6` (`24px`), `--spacing-8` (`32px`). | Dùng thang Tailwind chuẩn (`gap-1`, `gap-2`, `gap-3`, `gap-4`, `gap-6`, `p-4`, `p-6`). Không dùng giá trị lẻ. |
| **Element Sizes** | `sm = 28px`, `md = 32px`, `lg = 36px` (`--size-element-*`). | Khớp với Button/Input của Multica: `--button-height-sm: 1.75rem (28px)`, `--button-height-default: 2rem (32px)`, `--button-height-lg: 2.25rem (36px)`. |
| **Border Radius** | `inner = 4px/6px`, `element = 8px/10px`, `container = 12px`, `full = 9999px`. Công thức đồng tâm: `inner_radius = max(0px, outer_radius - padding)`. | `--radius-sm` (`4px`), `--radius-md` (`6px`), `--radius-lg` (`8px` - controls), `--radius-xl` (`12px` - cards/dialogs), `rounded-full` (status dots/pills). |
| **Elevation / Shadow** | 4 mức: `none` (mặc định cho mọi Card/Banner trong luồng trang), `low` (nổi nhẹ), `med` (Popover/Menu), `high` (Modal Dialog). | Mặc định Card/Section trong trang dùng `shadow-none` + `border-surface-border`; Menu dùng `--menu-shadow`; Dialog dùng `--floating-shadow`. |

### 2.3 Danh mục thành phần Astryx và ánh xạ sang mã nguồn hiện tại

*(Cập nhật `2026-11-20` theo mã nguồn sau commit `26525d7c5`. Bản trước mô tả quy ước "dựng bằng `@multica/ui`", nhưng Console hiện import trực tiếp nhiều thành phần của `@astryxdesign/core`; bảng dưới ghi đúng những gì đang dùng.)*

Phạm vi kiểm chứng: `platform-management/console/src/`, không tính các tệp `.test.`. `packages/ui/` và `packages/views/` hiện không import `@astryxdesign/core`. Các thành phần của `@multica/ui` (Button, Select, Input, Checkbox, Tabs, Dialog, Alert, icon từ `@multica/ui/icons`) vẫn được dùng song song với Astryx.

| Thành phần Astryx (`@astryxdesign/core`) | Vai trò trong Astryx | Hiện trạng trong Console |
| --- | --- | --- |
| `AppShell` + `SideNav` (`SideNavSection`, `SideNavItem`, `SideNavCollapseButton`) | Khung ứng dụng ngoài cùng và thanh điều hướng trái, thu gọn được | `components/Layout.tsx` dùng `AppShell` (`variant="section"`, `contentPadding={0}`) và `SideNav` có `collapsible` với `SideNavCollapseButton`. Theme `blue-white` tô thanh điều hướng bằng xanh Multica (`--sidebar: #0054a6`, chữ trắng) trong `styles.css`. `SidebarProvider` / `Sidebar` của `@multica/ui/components/ui/sidebar` có sẵn nhưng Console chưa dùng. |
| `Layout` (`LayoutHeader`, `LayoutContent`) | Bố cục theo vùng cho mỗi màn hình | Dùng trong 9 tệp màn hình (`screens/*.tsx`). |
| `Heading`, `Text`, `Divider` (dựng `PageHeader`) | Tiêu đề cấp 1, văn bản phụ, vạch ngăn | `components/PageHeader.tsx` ghép ba thành phần này. Hàng đầu gồm nút quay lại, tiêu đề, `badge`, `meta`; bên phải là `actions` và `HeaderControls`. `Divider` đóng header một lần duy nhất; `description` nằm dưới vạch ngăn, để chiều cao header không đổi giữa các trang. |
| Điều khiển ngôn ngữ và theme (không có trong Astryx) | — | `components/PreferenceControls.tsx`: `LanguageSwitch` (EN/VI) và `ThemeSwitch` (`blue-white`, `light`, `dark`) là nhóm nút phân đoạn (`role="group"`, mỗi nút có `aria-pressed`), không dùng `<select>`. `components/header-controls.tsx` đưa các điều khiển này, cùng email, vai trò và nút đăng xuất, lên cùng hàng với tiêu đề trang. |
| `Section` | Đơn vị phân vùng mặc định trên trang, không viền | Dùng trực tiếp `@astryxdesign/core/Section` ở 16 tệp. |
| `Card` | Khối có viền cho KPI, hộp đăng nhập, widget độc lập | Dùng `@astryxdesign/core/Card` ở 19 tệp. Console không import `@multica/ui/components/ui/card`. Việc giảm "Card soup" vẫn là mục còn mở ở mục 3.1. |
| `MetadataList` + `MetadataListItem` | Danh sách cặp khóa–giá trị | `components/MetadataList.tsx` bọc `MetadataList` và `MetadataListItem` của Astryx, nhận `columns` từ 1 đến 4. |
| `Table` | Bảng dữ liệu tràn viền vùng chứa | Dùng `@astryxdesign/core/Table` (`Table`, `pixel`, `proportional`) ở 24 tệp. Không tệp nào dùng `@multica/ui/components/ui/table`. Thanh lọc dùng `Input` và `Select` của `@multica/ui` (lần lượt 17 và 15 tệp). |
| `Badge` & `StatusDot` | `StatusDot` và nhãn chữ cho trạng thái thường; `Badge` ngữ nghĩa cho trạng thái cần chú ý và phân loại | `components/StatusBadge.tsx` dùng `Badge` và `StatusDot` của Astryx, theo bảng `STATUS_CONFIG`: trạng thái bình thường hoặc không hoạt động là `kind: "quiet"` (chấm + chữ); trạng thái ngoại lệ hoặc chuyển tiếp là `kind: "badge"` với `badgeVariant`; vai trò là `Badge` không có chấm. Trong `StatusBadge.tsx` không còn màu `emerald-*`, `amber-*` hay mã hex. |
| `Banner` | Thông báo bền vững (`info`, `warning`, `error`, `success`) | Dùng `@astryxdesign/core/Banner` ở 5 tệp. Song song, `Alert` của `@multica/ui/components/ui/alert` được dùng ở 14 tệp. |
| `Dialog`, `Field` | Hộp thoại và trường biểu mẫu | `@astryxdesign/core/Dialog` ở 2 tệp và `Field` ở 3 tệp. Phần lớn hộp thoại vẫn dùng `@multica/ui/components/ui/dialog` (9 tệp). |
| `EmptyState` | Khung trống có icon, tiêu đề, mô tả và hành động | `components/EmptyState.tsx` bọc `Card` và `EmptyState` của Astryx, nhận `title`, `description`, `icon`, `actions`, `isCompact`. |
| `TabList` (`Tab`) | Thanh chuyển tab | `@multica/ui/components/ui/tabs` (`Tabs`, `TabsList`, `TabsTrigger`) ở 4 tệp, ví dụ `screens/Orders.tsx`. Không còn `role="tab"` tự dựng trong `platform-management/console/src/` (đã kiểm tra bằng grep). |

---

## 3. Phân tích khoảng trống (Gap Analysis)

### 3.1 Những điểm trong UI hiện tại xung đột với Astryx và quy ước repo

**Trạng thái:** trong lần rà soát này, một số khoảng trống của bản `2026-11-17` đã được đóng. Các mục dưới đây chỉ liệt kê những gì **còn mở** và ghi rõ những gì **đã đóng**.

1. **[Còn mở] Khung ứng dụng Console dùng top bar thay vì `AppShell` + `SideNav`:**
   - Hiện có 10 mục điều hướng (`Overview`, `Tenants`, `Users`, `Orders`, `Payment notices`, `E-invoices`, `Accounts`, `Plans`, `Staff`, `Audit Log`) trong một thanh ngang (`Layout.tsx:31-70`) có `overflow-x-auto`. Theo `scaffold.doc.mjs`, bộ điều hướng từ 5 đích trở lên phải dùng `AppShell` với `SideNav` (`240px–260px`), phân nhóm.
   - **[Đã đóng] Thiếu route:** `Orders`, `Plans`, `Accounts`, `Payment notices`, `E-invoices` đã được khai báo và render trong `App.tsx`.
   - **[Còn mở] Không đồng bộ URL:** `App.tsx` không dùng `pushState`/`popstate`, nên F5 trên cổng `30001` (dù `staticHandler` trả `index.html`) luôn về `Overview`. Màn hình chi tiết (`tenant-detail`, `user-detail`) cũng không có URL.
2. **[Còn mở] Lạm dụng `Card` làm khung bố cục trang ("Card soup"):**
   - `TenantDetail.tsx`: 38 chỗ `<Card`.
   - `Overview.tsx`: 24 chỗ `<Card` (cần giữ `Card` cho ô KPI; các khối còn lại chuyển sang `Section`).
   - `UserDetail.tsx`: 10 chỗ `<Card`.
   - `tokens-tab.tsx:289`: mỗi PAT một `Card`.
   - `order-page.tsx`: ba khối `Card` rời rạc (`:409`, `:475`, và khối thứ ba).
   - `PaymentNotices.tsx`, `PriceSchedule.tsx`, `Login.tsx` cũng có `Card`; cần rà soát lại theo "Card hay hàng".
3. **[Còn mở] Phần tử HTML thô thay vì component hệ thống (`principles.doc.mjs` Rule 1):**
   - `<select>` thô: `TenantDetail.tsx:1106,1168,1245`, `Staff.tsx:212`, `Audit.tsx:68`, `Accounts.tsx:689,830,883`.
   - `<input type="checkbox">` thô: `Plans.tsx:470,481`, `PriceSchedule.tsx:521`, `Accounts.tsx:908`.
   - `<button role="tab">` tự viết: `Orders.tsx:200`, `Accounts.tsx:534`, `EinvoiceFailures.tsx:94`.
   - `<details>/<summary>` thô: `tokens-tab.tsx:180`.
   - `<ul>/<li>` thủ công cho danh sách sự kiện bảo mật: `account-tab.tsx:485-499`.
4. **[Còn mở] Vi phạm hệ thống token Typography và Color:**
   - Lớp chữ không tồn tại: `text-title-xl` (`Overview.tsx`), `text-title-md` và `bg-surface-muted` (`order-page.tsx`).
   - Màu cứng `emerald-*`, `amber-*` trong `StatusBadge.tsx:9`, `TenantDetail.tsx:890`, `Accounts.tsx:170,185,512`, `Orders.tsx:271`, `Overview.tsx:82,85`, `PriceSchedule.tsx:510-513,561`. (Danh sách đầy đủ ở mục 2.2.A.)
5. **[Còn mở] Badge và trạng thái chưa theo `Badge.doc.mjs`:**
   - `StatusBadge.tsx` chưa có ánh xạ cho trạng thái đơn hàng (`pending`, `paid`, `expired`, `canceled`, `refunded`), trạng thái thuê bao (`trialing`, `past_due`), và trạng thái yêu cầu tài khoản (`pending`, `provisioned`, `completed`, `failed`, `canceled`) như bản trước nêu. Cần xác nhận lại danh sách enum trong `api/schemas.ts` trước khi ánh xạ.
   - Chưa có `StatusDot`; mọi trạng thái (kể cả `active`) đang dùng badge có viền màu.
6. **[Đã đóng một phần] Khoảng trống chức năng so với backend:**
   - **[Đã đóng] Lỗi `RefusePriceFields`:** `Plans.tsx` không còn gửi `seat_price_month` / `seat_price_year`.
   - **[Đã đóng] Lịch đổi giá (Q13):** có `PriceSchedule.tsx`, có xem trước `dry_run`, cảnh báo `confirm_large_change`, nút hủy lịch chờ.
   - **[Đã đóng] Màn hình Accounts (Q14–Q16):** có `Accounts.tsx`, có `useResendWelcome`.
   - **[Đã đóng] Hồ sơ thanh toán (Q2) trên Tenant:** `TenantDetail.tsx` đã dùng hook hồ sơ và xác minh.
   - **[Còn mở] Overview:** chưa hiển thị `pending_orders`, `by_plan`, `usage_*`.
   - **[Còn mở] Mức dùng theo ngày trong Tenant:** có `useTenantUsage`, cần xác nhận bảng dòng phí đã được render.
   - **[Còn mở] Xóa nợ usage (`Write off`):** `Orders.tsx` chưa có nhãn và điều kiện hiển thị theo vai trò `admin`.
   - **[Còn mở] Hiển thị `kind` của đơn hàng và `audience` / `billing_model` của gói:** chưa có trong `Orders.tsx` và `Plans.tsx`.
   - **[Còn mở] Lịch sử giá cho `billing`:** `pricechange/handler.go:66-69` cho phép `billing` xem lịch sử, nhưng `App.tsx` chỉ mở `plans` cho `admin`. Xem `ASSUMPTION-4`.

### 3.2 Những phần có thể tái sử dụng nguyên trạng (Reuse as-is)

- **Tầng API & Schema của Console (`platform-management/console/src/api/client.ts`, `schemas.ts`, `hooks.ts`):** Giữ nguyên `platformFetch`, `PlatformApiError`, `parseWithFallback`, hệ thống khóa `platformKeys`, và toàn bộ hook hiện có (gồm cả các hook mới cho `price-changes`, `accounts`, `tenantUsage`, `billingProfile`, `einvoices`, `paymentNotices`).
- **Tầng Primitives trong `@multica/ui` (`packages/ui/components/ui/`):** Đã có đầy đủ `sidebar`, `tabs`, `table`, `select`, `checkbox`, `switch`, `badge`, `button`, `dialog`, `alert-dialog`, `alert`, `empty`, `field`, `skeleton`, `spinner`, `tooltip`, `separator`.
- **Hệ thống bố cục Settings phía Tenant (`packages/views/settings/components/settings-layout.tsx`):** `SettingsTab`, `SettingsSection`, `SettingsCard`, `SettingsRow`, `SettingsViewTabs`, `SettingsReadOnlyNotice` đã tuân thủ rất sát mẫu `settings` của Astryx.
- **Logic đồng bộ thanh toán trong `packages/views/settings/components/billing-tab.tsx`:** Giữ nguyên state machine idempotency, polling ghế, preview đổi gói và kiểm tra quyền.
- **Tách logic thuần `packages/views/billing/plan-audience.ts`:** Đã là hàm thuần, có test; dùng lại được cho hiển thị `audience` trên Console nếu cần (cần kiểm tra ranh giới: Console không được import `@multica/views`, nên phải tách sang `@multica/core` nếu muốn chia sẻ).

---

## 4. Bảng ánh xạ chi tiết (Mapping Table)

### 4.1 Thành phần nền tảng & Bố cục dùng chung trong Platform Console (`platform-management/console/src/`)

| Thành phần hiện tại | Mẫu Astryx tương ứng | Thay đổi cụ thể cần thực hiện |
| --- | --- | --- |
| `App.tsx` + `components/Layout.tsx` | `AppShell` + `SideNav` (`256px`, nhóm `SideNavSection`) + `Layout` (`LayoutHeader` + `LayoutContent`) | 1. Chuyển từ top-nav sang `SideNav` bên trái, nhóm theo vai trò: **Operations** [`Overview`, `Tenants`, `Users`], **Commercial** [`Orders` (billing/admin), `Payment notices` (billing/admin), `E-invoices` (billing/admin), `Accounts` (admin), `Plans` (admin)], **Administration** [`Staff` (admin), `Audit Log`]. Chân sidebar hiển thị email staff, `StatusBadge` vai trò, nút `Sign out`.<br>2. Giữ nguyên kiểm tra quyền trong `handleNavigate` và `renderScreen()` (đã có).<br>3. Đồng bộ state với URL (`/`, `/tenants`, `/tenants/:id`, `/users`, `/users/:id`, `/orders`, `/payment-notices`, `/einvoices`, `/accounts`, `/plans`, `/staff`, `/audit`) bằng `history.pushState` + `popstate`. |
| `components/PageHeader.tsx` *(**chưa có**)* | `LayoutHeader` (`hasDivider`) | Tạo component chuẩn đầu màn hình: `title`, `description?`, `backAction?`, `badge?`, `actions?`, vạch ngăn dưới. Dùng cho `TenantDetail`, `UserDetail`, và các màn hình danh sách. |
| `components/MetadataList.tsx` *(**chưa có**)* | `MetadataList` + `MetadataListItem` | `<dl>/<dt>/<dd>` với `text-label text-muted-foreground` cho nhãn và `text-body font-medium` cho giá trị (có `tabular-nums` / `font-mono`), lưới 1–4 cột. |
| `components/StatusBadge.tsx` | `StatusDot` + `Badge` (semantic variants) | 1. Thay màu cứng `emerald-*` (`:9`) bằng token ngữ nghĩa (`border-success/30 bg-success/10 text-success`, v.v.).<br>2. Trạng thái hàng bảng (`active`, `read_only`, `suspended`, `pending_deletion`, `deleted`): chấm `StatusDot` + nhãn chữ.<br>3. Bổ sung ánh xạ đơn hàng, thuê bao, yêu cầu tài khoản sau khi đối chiếu enum trong `schemas.ts`. |
| `components/EmptyState.tsx`, `ErrorState.tsx`, `LoadingState.tsx` | `EmptyState` (`icon`, `title`, `description`, `actions`, `isCompact`), `Banner` (`status="error"`), `Skeleton` / `Spinner` | 1. Mở rộng `EmptyState` nhận `icon`, `actions`, `isCompact`. Hiện `EmptyState` chỉ có `title`, `description`.<br>2. `ErrorState` dùng `Alert` có `role="alert"`. |
| `components/ConfirmReasonDialog.tsx` | `Dialog` (`purpose="form"`) + `FormLayout` + `Field` | Giữ nguyên hợp đồng props; chuẩn hóa nhãn bằng `Field` / `Label`, lỗi bằng `Alert variant="destructive"`. |
| `components/types.ts` | — | `CATALOG_ACTIONS` (`:104`) tái xuất từ `AUDIT_ACTIONS` trong `api/schemas.ts` để không lệch danh sách mã hành động. |

### 4.2 Từng màn hình của Platform Console (`platform-management/console/src/screens/`)

| Màn hình | Trạng thái (2026-11-19) | Mẫu Astryx mục tiêu | Thay đổi cụ thể cần thực hiện |
| --- | --- | --- | --- |
| `Login.tsx` | Còn `Card`, chưa theo `login-sso` | Template `login-sso` (`Center` + `Card maxWidth={400}` + `VStack gap={4}`) | Nền ngoài `bg-app-shell`, `Card` `max-w-sm shadow-none border-surface-border`, thông báo bằng `Alert`, ô mã 6 số và TOTP dùng `font-mono tabular-nums tracking-widest`, nút `Button` có `disabled` + `aria-busy` + `Spinner`. |
| `Overview.tsx` | Còn `text-title-xl`, `emerald`, 24 `Card`, thiếu `pending_orders` / `by_plan` / `usage_*` | Template `dashboard` (`Grid` KPI Cards + `Section` + `Separator`) | 1. Thay `text-title-xl` bằng `text-display-sm font-semibold tabular-nums`; `emerald-*` bằng `text-success`.<br>2. Ba `Section` phân tách bởi `Separator`: KPI (`Total Workspaces`, `Active`, `Sign-ups (7d)`, `Pending Orders`, `Read-Only`, `Suspended`, `Pending Deletion`); phân bổ theo gói (`by_plan`); số liệu sử dụng toàn nền tảng (`usage_*`, dùng `MetadataList`, `formatBytes` cho `usage_storage_bytes`). |
| `Tenants.tsx` | Có `EmptyState`, `Table` | Template `table-filter` | Đặt `PageHeader`, thanh tìm kiếm có nút xóa nhanh, bảng phẳng `shadow-none`, cột `Status` dùng `StatusBadge` có chấm, cột số `tabular-nums`. |
| `TenantDetail.tsx` | 38 `Card`, 3 `<select>` thô, `emerald` cứng | Template Detail Page (`PageHeader` + `Banner` + `Section` + `MetadataList` + `Table`) | 1. Thay 38 `Card` bằng `Section`/`MetadataList`/`Table`.<br>2. `PageHeader` có tên tenant, `slug` & `id` (mono), `StatusBadge`, nút `Change status` và `Edit subscription`.<br>3. `Banner` (`Alert`) khi có `state.reason` hoặc `deletion_scheduled_at`.<br>4. Khối mức dùng: `MetadataList` + bảng dòng phí theo ngày từ `useTenantUsage` (**xác nhận lại trong mã**).<br>5. Khối hồ sơ thanh toán: `MetadataList` + trạng thái xác minh (`useTenantBillingProfile`).<br>6. Bảng `Recent Orders`, `Workspace Audit Log` tràn viền (`padding={0}`).<br>7. Thay 3 `<select>` thô (`:1106`, `:1168`, `:1245`) bằng `Select` của `@multica/ui`. |
| `UserDetail.tsx` | 10 `Card` | Template Detail Page | `PageHeader` có nhóm nút (`Suspend`/`Reinstate`, `Sign out everywhere`, `Reset two-step sign-in`), `Alert` khi `account.reason` hoặc suspended, `MetadataList` cho `Account Record`, `Section` + `Table` cho `Workspace Memberships`. |
| `Users.tsx` | Có `EmptyState` | Template `table-filter` | Đồng bộ với `Tenants.tsx`. |
| `Orders.tsx` | Đã nối `App.tsx`; còn `role="tab"` tự chế; chưa có `kind` / `Write off` | Template `table-filter` + `TabList` | 1. `PageHeader`.<br>2. Thay `<button role="tab">` (`:200`) bằng `Tabs` / `TabsList` / `TabsTrigger` từ `@multica/ui`.<br>3. Hiển thị `order.kind` (`subscribe`, `change_plan`, `add_seats`, `renew`, `usage`).<br>4. Đơn `usage` ở `pending`: nút `Write off` chỉ cho `admin` (đúng quy tắc `orders.go:296`).<br>5. Icon `Check` (`:271`) dùng `text-success`. |
| `Plans.tsx` | Đã nối; đã bỏ trường giá; còn 2 checkbox thô; chưa có `audience` / `billing_model` | Template `table` + `Dialog` (`purpose="form"`) | 1. Thay `<input type="checkbox">` (`:470`, `:481`) bằng `Checkbox` hoặc `Switch`.<br>2. Thêm cột `Audience` (`any`, `individual`, `business`) và `Billing Model` (`seat`, `usage`) vào bảng gói (cần thêm vào `PlanSchema` nếu chưa có).<br>3. Giữ `currency` trong `PUT` (đã được `plan_currency_locked` xử lý). |
| `PriceSchedule.tsx` | **[Mới]** Có dialog lập lịch, `dry_run`, cảnh báo đổi lớn | Template `Dialog` (`purpose="form"`) + `Banner` | Thay `amber-*` (`:510-513`) và `emerald-*` (`:561`) bằng `bg-warning/10 text-warning` và `text-success`; thay checkbox thô (`:521`); xác nhận lại `min_effective_from` và bảng lịch sử giá. |
| `Accounts.tsx` | **[Mới]** Có tạo tài khoản, lọc trạng thái, hủy, gửi lại thư chào | Template `table` + `Dialog` (`purpose="form"`) | 1. Thay `role="tab"` (`:534`) bằng `Tabs`.<br>2. Thay 3 `<select>` (`:689`, `:830`, `:883`) và checkbox (`:908`) bằng `@multica/ui`.<br>3. Thay `amber-*`/`emerald-*` (`:170`, `:185`, `:512`) bằng token.<br>4. `PageHeader` với nút `Create account`. |
| `PaymentNotices.tsx` | **[Mới]** Có `Table` | Template `table-filter` + `Dialog` | `PageHeader`, `StatusBadge` cho trạng thái thông báo, `EmptyState`, xác nhận hành động qua `ConfirmReasonDialog`. Cần đối chiếu sau. |
| `EinvoiceFailures.tsx` | **[Mới]** Có tab tự chế (`:94`) | Template `table-filter` + `TabList` | Thay tab thô bằng `Tabs`; `PageHeader`; `StatusBadge` cho trạng thái hóa đơn. |
| `Staff.tsx` | Còn `<select>` thô | Template `table` + `Dialog` | Thay `<select>` (`:212`) bằng `Select`; `Alert` cho lỗi `last_admin`; `PageHeader` với nút `Add staff`. |
| `Audit.tsx` | Có `EmptyState`; còn `<select>` thô | Template `table-filter` | Thay `<select>` (`:68`) bằng `Select`; `CATALOG_ACTIONS` đầy đủ; nút `Clear filters`. |

### 4.3 Các mặt giao diện phía Tenant (`packages/views/`)

| Thành phần hiện tại | Trạng thái (2026-11-19) | Mẫu Astryx tương ứng | Thay đổi cụ thể cần thực hiện |
| --- | --- | --- | --- |
| `packages/views/billing/order-page.tsx` | Còn 3 `Card`; còn `text-title-md`, `bg-surface-muted` | Template Detail / Checkout (`Layout` `contentWidth={960}` + `Section` + `SettingsCard` + `MetadataList`) | 1. `text-title-md` → `text-title-sm` (`:411`, `:449`, `:477`); `bg-surface-muted` → `bg-muted` (`:444`).<br>2. Bỏ 3 `Card`, dùng `Section` + `SettingsCard` phẳng; khối mã chuyển khoản nổi bật (`font-mono tabular-nums`), thông tin ngân hàng, bảng tổng tiền (`tabular-nums`), biểu mẫu hóa đơn. Giữ nguyên `id`, `name`, `role="alert"`, `aria-label` để test không đổi. |
| `packages/views/settings/components/billing-tab.tsx` | Đã theo `SettingsTab` | Template `settings` | Chuẩn hóa thông báo `scheduled_price_change` và thẻ `usage` bằng `Banner` / `SettingsCard`, khi dữ liệu có mặt. |
| `packages/views/settings/components/audit-log-tab.tsx` | Chưa có khung bảng đồng bộ | Template `table-filter` trong `SettingsTab` | Nút `Clear filters` khi không có kết quả; bọc `Table` trong `rounded-lg border border-surface-border bg-surface`. |
| `packages/views/settings/components/account-tab.tsx` | `<ul>/<li>` cho `securityEvents` | Template `settings` | `font-mono text-caption` cho mã hành động, `tabular-nums` cho thời gian. |
| `packages/views/settings/components/tokens-tab.tsx` | "Card soup" (`:289`), `<details>` (`:180`) | Template `settings` + Rows | 1. Một `SettingsCard` chứa hàng `divide-y divide-surface-border` cho mỗi token (tên, `token_prefix` mono, ngày tạo, lần dùng cuối, hết hạn, nút thu hồi).<br>2. Thay `<details>` bằng văn bản gọn hoặc `Collapsible`. |

---

## 5. Mục tiêu ngoài phạm vi (Non-goals) và Ràng buộc kỹ thuật

### 5.1 Ràng buộc bắt buộc

1. **Giữ nguyên cổng và kiến trúc dịch vụ:** Không thay đổi các cổng `30002` (landing), `30000` (Multica web), `30001` (platform console), `8095` (platform API).
2. **Không sửa đổi Backend Go hay Database Schema:** Mọi API cần thiết đã có sẵn trong `platform-management/internal/`. Việc áp dụng giao diện Astryx chỉ diễn ra ở tầng frontend (`platform-management/console/`, `packages/views/`, và mở rộng schema/hook tương ứng nếu cần).
3. **Tuân thủ tuyệt đối ranh giới gói (`AGENTS.md`):**
   - `platform-management/console` chỉ import từ `@multica/core` và `@multica/ui`, không import `@multica/views`, `next/*`, hay `react-router-dom`.
   - `packages/views/` không import `next/*` hay `react-router-dom`, không khởi tạo store Zustand.
   - Mọi icon phải được import từ `@multica/ui/icons` (nếu thiếu icon mới thì vẽ theo `.agents/skills/multica-icons/SKILL.md`, không cài `lucide-react` hay `@heroicons/react` vào repo, không dùng SVG nội tuyến hay emoji).
4. **Tuân thủ hệ thống Token và Component của `@multica/ui`:**
   - Chỉ sử dụng các bước chữ `--text-*` trong `packages/ui/styles/tokens.css`.
   - Tuân thủ `packages/ui/docs/button.md` (không có prop `loading` hay `variant="primary"`; dùng `variant="default"`, `disabled` + `aria-busy` + `<Spinner />`) và `packages/ui/docs/dialog.md` (chỉ đóng dialog sau khi mutation thành công; giữ dữ liệu đã nhập và hiện lỗi khi thất bại).
5. **Tương thích API (`AGENTS.md` "API Compatibility"):** Mọi dữ liệu JSON đọc từ API đều đi qua schema `zod` và `parseWithFallback`, có giá trị mặc định cho trường tùy chọn và fallback `"unknown"` cho enum lạ.
6. **Quy tắc UI Copy & Đa ngôn ngữ (`AGENTS.md` "UI Copy"):**
   - Không viết mô tả thừa lặp lại tiêu đề hay nhãn nút.
   - Mọi chuỗi giao diện mới trong `packages/views/locales/` phải được cập nhật đủ 7 ngôn ngữ (`en`, `es`, `fr`, `ja`, `ko`, `zh-Hans`, `zh-Hant`). Riêng `platform-management/console` là công cụ nội bộ cho staff nên giữ tiếng Anh như thiết kế hiện tại.

### 5.2 Ngoài phạm vi (Non-goals)

- Phạm vi cài đặt `@astryxdesign/core`, `@astryxdesign/theme-neutral`, `@astryxdesign/charts` và `@stylexjs/stylex` hiện giới hạn trong `platform-management/console/package.json` (dùng `catalog:`). `packages/ui/`, `packages/views/` không khai báo hay import Astryx; không đưa Astryx vào các gói này. Trong Console, các thành phần Astryx được dùng trực tiếp như mục 2.3 mô tả. **Ngôn ngữ thiết kế, quy tắc bố cục, phân cấp bề mặt, mật độ và mẫu trang của Astryx** được áp dụng qua `@multica/ui` và `tokens.css` khi có thể.
- Không thay đổi giao diện của `apps/mobile/` hay `apps/docs/`.
- Không thay đổi các màn hình sản phẩm ngoài phạm vi `platform-management` (như Issues, Chat, Autopilot, Knowledge).

---

## 6. Giả định và Câu hỏi mở (Assumptions)

Theo quy tắc **Automation First** trong `AGENTS.md`, mọi điểm chưa nêu tường minh được giải quyết bằng việc đối chiếu mã nguồn và ghi nhận thành giả định có gắn nhãn (`[ASSUMPTION-n]`):

- **`[ASSUMPTION-1]` Phương thức áp dụng Astryx (Design Language & Patterns over Package Swap):** Áp dụng Astryx nghĩa là chuyển đổi bố cục, cấu trúc vùng (`AppShell` + `SideNav` + `Layout` + `Section` + `MetadataList` + `Table` + `TabList`), mật độ, thang chữ và màu sắc theo chuẩn Astryx (`neutralTheme` + `layout` guides), nhưng dựng bằng primitive `@multica/ui` và token `packages/ui/styles/tokens.css`.
- **`[ASSUMPTION-2]` Phạm vi "toàn bộ phân hệ platform-management":** Bao gồm (1) Platform Console tại `platform-management/console/` (cổng `30001`) gồm cả các màn hình mới `Accounts`, `PaymentNotices`, `EinvoiceFailures`, `PriceSchedule`; và (2) các màn hình/tab phía Tenant trong `packages/views/billing/` và `packages/views/settings/components/` (`billing-tab.tsx`, `order-page.tsx`, `audit-log-tab.tsx`, `account-tab.tsx`, `tokens-tab.tsx`, `consent-gate.tsx`, `deletion-gate.tsx`).
- **`[ASSUMPTION-3]` Điều hướng URL trên Platform Console (`30001`):** `staticHandler` trong `platform-management/internal/server/server.go:166-203` trả `index.html` cho mọi đường dẫn SPA ngoài `/api/` và `/assets/`, nên Console dùng History API ngay trong `App.tsx` (không cài `react-router-dom`). Các unit test hiện có render `<App />` tại `/` cần được giữ tương thích.
- **`[ASSUMPTION-4]` Quyền truy cập lịch sử giá trên Console:** Backend cho phép `billing` đọc `GET /api/platform/plans/{key}/price-changes` (`pricechange/handler.go:66-69`), còn `GET/PUT/DELETE /api/platform/plans*` (`subscription/console.go`) giới hạn cho `admin`. `App.tsx` hiện chỉ mở `plans` cho `admin`. Giả định: giữ nguyên hành vi này cho đến khi có yêu cầu mở rộng; nếu mở cho `billing` thì chỉ hiển thị chế độ xem lịch sử.
- **`[ASSUMPTION-5]` Gửi lại thư chào (đã giải quyết):** Hook `useResendWelcome` đã có và `Accounts.tsx` đã dùng; không còn rủi ro route chưa sẵn sàng. Cần kiểm tra lại đúng route `POST /api/platform/accounts/{id}/resend-welcome` trong `internal/console/handler.go` khi thực hiện Slice 7.
- **`[ASSUMPTION-6]` Trường `currency` khi sửa gói:** `currency` không nằm trong `PriceFields` (`pricechange.go:47`), nên `PUT /plans/{key}` được phép gửi `currency`. Giả định: đây là thay đổi có chủ đích (kèm `plan_currency_locked`) và không cần bỏ.
- **`[ASSUMPTION-7]` Phạm vi `PaymentNotices` và `EinvoiceFailures`:** Hai màn hình này nằm trong phân hệ (P42, P43) và được đưa vào phạm vi áp dụng Astryx giống các màn hình khác. Không cần thay đổi nghiệp vụ.
