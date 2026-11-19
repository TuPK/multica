# Kế hoạch triển khai giao diện Astryx cho phân hệ `platform-management`

Tài liệu đặc tả: [`spec.md`](spec.md).
Ngày cập nhật: `2026-11-19` (sau khi rà soát lại mã nguồn tại commit `354a57584`).

Quy ước kiểm chứng từ [`AGENTS.md`](../../AGENTS.md):
- Trước khi chạy `vitest` hoặc `tsc` trực tiếp: chạy `pnpm catalog:build`.
- Kiểm tra nhanh trong lúc lặp (narrow checks):
  - Console: `pnpm --filter @multica/platform-console typecheck` và `pnpm --filter @multica/platform-console test`
  - Views: `pnpm --filter @multica/views typecheck` và `pnpm --filter @multica/views exec vitest run <tệp-test>`
- Kiểm tra tổng thể cuối mỗi đợt: `pnpm typecheck`, `pnpm lint`, `pnpm test`.
- Không sửa đổi bất kỳ cổng dịch vụ nào (`30002`, `30000`, `30001`, `8095`) và không chạm tới cơ sở dữ liệu hay backend Go.

---

## Trạng thái các lát cắt sau rà soát (2026-11-19)

| Slice | Tên lát cắt | Trạng thái | Ghi chú |
| --- | --- | --- | --- |
| **Slice 1** | Nền tảng: `StatusBadge`, `PageHeader`, `MetadataList`, `EmptyState`/`ErrorState`, `CATALOG_ACTIONS` | **Chưa làm** | `PageHeader.tsx`, `MetadataList.tsx` chưa tồn tại. `StatusBadge` vẫn dùng `emerald-*`. |
| **Slice 2** | Khung `AppShell` + `SideNav`, đồng bộ URL, `Login` | **Làm một phần** | Đã nối đủ route (`1a1293ddc`). Chưa có `SideNav`, chưa có `pushState`, `Login` chưa đổi. |
| **Slice 3** | `Overview`, `Tenants`, `Users`, `Audit` | **Chưa làm** | `Overview` còn `text-title-xl` và `emerald`. `Audit` còn `<select>` thô. |
| **Slice 4** | `TenantDetail`, `UserDetail`, schema/hook usage & hồ sơ thanh toán | **Làm một phần** | `useTenantUsage`, hook hồ sơ thanh toán đã có. `TenantDetail` còn 38 `Card`, `UserDetail` còn 10 `Card`. |
| **Slice 5** | `Orders`, `Staff` (+ `kind`, `Write off`) | **Chưa làm** | `Orders` chưa có `kind`/`Write off`; còn tab thô; `Staff` còn `<select>` thô. |
| **Slice 6** | `Plans` (checkbox, `audience`/`billing_model`) & lịch đổi giá | **Làm một phần** | Đã bỏ trường giá khỏi `PUT`, đã có `PriceSchedule.tsx`. Còn checkbox thô, chưa có cột `audience`/`billing_model`. |
| **Slice 7** | `Accounts` (token, Tabs, select, checkbox) | **Làm một phần** | Màn hình và hook đã có. Còn tab thô, 3 `<select>`, 1 checkbox, màu cứng. |
| **Slice 8** | Màn hình phía Tenant trong `packages/views/` | **Chưa làm** | `order-page.tsx` còn `text-title-md`, `bg-surface-muted`, 3 `Card`. `tokens-tab.tsx` còn "Card soup". |
| **Slice 9** | **[Mới]** `PaymentNotices`, `EinvoiceFailures`, `PriceSchedule` | **Chưa làm** | Ba màn hình này chưa có trong bản kế hoạch cũ. |

Thứ tự thực hiện không đổi: **1 → 2 → (3, 4, 5, 6, 7, 9 song song được) → 8**. Slice 8 không phụ thuộc Console.

---

## Chi tiết từng lát cắt (Slices)

### Slice 1: Nền tảng — `StatusBadge`, `PageHeader`, `MetadataList`, Empty/Error States

**Mục tiêu:** Có các thành phần cơ sở để các slice sau dùng chung, loại bỏ màu cứng khỏi `StatusBadge`.

**Tệp chạm tới:**
- `platform-management/console/src/styles.css` (không đổi nếu token đã đủ)
- `platform-management/console/src/components/types.ts` (sửa: `CATALOG_ACTIONS` tái xuất từ `AUDIT_ACTIONS`)
- `platform-management/console/src/components/StatusBadge.tsx` (sửa)
- `platform-management/console/src/components/EmptyState.tsx` (sửa: `icon`, `actions`, `isCompact`)
- `platform-management/console/src/components/ErrorState.tsx` (sửa)
- `platform-management/console/src/components/ConfirmReasonDialog.tsx` (sửa nhẹ)
- `platform-management/console/src/components/PageHeader.tsx` (**tạo mới**)
- `platform-management/console/src/components/MetadataList.tsx` (**tạo mới**)
- `platform-management/console/src/components/StatusBadge.test.tsx` (**tạo mới**)

**Thay đổi cụ thể:**
1. `StatusBadge.tsx`: thay `border-emerald-600/30 bg-emerald-50 text-emerald-700 ...` (`:9`) bằng `border-success/30 bg-success/10 text-success`. Thêm `StatusDot` (`size-1.5 rounded-full bg-current`, `aria-hidden`) cho trạng thái vận hành. Ánh xạ trạng thái đơn hàng, thuê bao, yêu cầu tài khoản sau khi đối chiếu enum trong `api/schemas.ts`.
2. `PageHeader.tsx`: `title`, `description?`, `backAction?`, `badge?`, `actions?`, vạch ngăn dưới `border-b border-surface-border pb-4`.
3. `MetadataList.tsx`: `MetadataList` (`columns?: 1|2|3|4`) và `MetadataListItem` (`label`, `children`, `mono?`, `tabular?`) dùng `<dl>`, `<dt className="text-label text-muted-foreground">`, `<dd className="mt-1 text-body font-medium text-foreground">`.
4. `EmptyState.tsx`: thêm `icon?`, `actions?`, `isCompact?`. `ErrorState.tsx`: `Alert` với `role="alert"`.
5. `types.ts`: `CATALOG_ACTIONS` lấy từ `AUDIT_ACTIONS`.

**Lệnh kiểm chứng:**
```bash
pnpm catalog:build
pnpm --filter @multica/platform-console typecheck
pnpm --filter @multica/platform-console test
```

---

### Slice 2: Khung `AppShell` + `SideNav`, đồng bộ URL, `Login`

**Mục tiêu:** Thay top-nav bằng `SideNav` phân nhóm, đưa điều hướng lên URL để F5 không mất màn hình, và chuẩn hóa `Login`.

**Đã xong:** Các route `orders`, `plans`, `accounts`, `payment-notices`, `einvoices` đã khai báo trong `App.tsx` và render với kiểm tra vai trò. Header đã sửa cho màn hình hẹp (`2f274a142`).

**Tệp chạm tới:**
- `platform-management/console/src/components/Layout.tsx` (sửa)
- `platform-management/console/src/components/Layout.test.tsx` (sửa)
- `platform-management/console/src/App.tsx` (sửa: `pushState`/`popstate`)
- `platform-management/console/src/App.test.tsx` (sửa)
- `platform-management/console/src/screens/Login.tsx` (sửa)
- `platform-management/console/src/screens/Login.test.tsx` (sửa nếu cần)

**Thay đổi cụ thể:**
1. `Layout.tsx`: khung `flex`, `aside` `aria-label="Main Navigation"` rộng `w-60` trên desktop; header thương hiệu; ba nhóm **Operations** (`Overview`, `Tenants`, `Users`), **Commercial** (`Orders`, `Payment notices`, `E-invoices` cho `billing`/`admin`; `Accounts`, `Plans` cho `admin`), **Administration** (`Staff` cho `admin`, `Audit Log`). Mục đang chọn `aria-current="page"`. Chân cột: email (`truncate` + `title`), `StatusBadge` vai trò, `Sign out`. Vùng `<main>` có `max-w-7xl p-6`.
2. `App.tsx`: thêm đồng bộ URL (`/`, `/tenants`, `/tenants/:id`, `/users`, `/users/:id`, `/orders`, `/payment-notices`, `/einvoices`, `/accounts`, `/plans`, `/staff`, `/audit`) với `history.pushState` và `popstate`. Không cài `react-router-dom`.
3. `Login.tsx`: mẫu `login-sso` (nền `bg-app-shell`, `Card` `max-w-sm shadow-none`, `Alert` cho thông báo, mã 6 số `font-mono tabular-nums tracking-widest`).

**Lệnh kiểm chứng:**
```bash
pnpm --filter @multica/platform-console typecheck
pnpm --filter @multica/platform-console exec vitest run src/components/Layout.test.tsx src/App.test.tsx src/screens/Login.test.tsx
```

---

### Slice 3: `Overview`, `Tenants`, `Users`, `Audit`

**Mục tiêu:** Áp dụng mẫu dashboard và table-filter; sửa lớp chữ không tồn tại; hiển thị dữ liệu mới của `Overview`.

**Tệp chạm tới:** `screens/Overview.tsx`, `Overview.test.tsx`, `Tenants.tsx`, `Tenants.test.tsx`, `Users.tsx`, `Users.test.tsx`, `Audit.tsx`, `Audit.test.tsx`.

**Thay đổi cụ thể:**
1. `Overview.tsx`: `PageHeader`. Thay `text-title-xl` (`:71,85,99,113,127,141`) bằng `text-display-sm font-semibold tabular-nums`. Thay `text-emerald-600` (`:82,85`) bằng `text-success`. Giữ KPI `Card` (`shadow-none border-surface-border`). Thêm thẻ `Pending Orders` khi `pending_orders` có dữ liệu. Thêm `Section` *Subscriptions by Plan* (`by_plan`) và *Platform Usage Snapshot* (`usage_*`, `MetadataList`, `formatBytes(usage_storage_bytes)`).
2. `Tenants.tsx`, `Users.tsx`: `PageHeader`; thanh lọc `HStack` với `Input` và icon `Search` từ `@multica/ui/icons`; bảng trong `rounded-lg border border-surface-border bg-surface`; `slug`/`id` dùng `font-mono text-caption`; `EmptyState` có `Clear search`.
3. `Audit.tsx`: thay `<select id="audit-action-select">` (`:68`) bằng `Select` của `@multica/ui`, giữ `aria-label="Action"`; thêm nút `Clear filters`.

**Lệnh kiểm chứng:**
```bash
pnpm --filter @multica/platform-console typecheck
pnpm --filter @multica/platform-console exec vitest run src/screens/Overview.test.tsx src/screens/Tenants.test.tsx src/screens/Users.test.tsx src/screens/Audit.test.tsx
```

---

### Slice 4: `TenantDetail` và `UserDetail`

**Mục tiêu:** Bỏ "Card soup" (38 `Card` trong `TenantDetail`, 10 trong `UserDetail`); dùng `PageHeader`, `Banner`, `Section`, `MetadataList`, `Table`; hiển thị mức dùng theo ngày và hồ sơ thanh toán.

**Đã có:** `api/schemas.ts` có `parseTenantUsage`; `api/hooks.ts` có `useTenantUsage` (`:289`), `useTenantBillingProfile` (`:298`), `useSetBillingProfileVerified` (`:865`), `useUpdateTenantBillingProfile` (`:909`).

**Tệp chạm tới:**
- `platform-management/console/src/api/schemas.ts` (kiểm tra lại; mở rộng nếu thiếu trường)
- `platform-management/console/src/api/schemas.test.ts`
- `platform-management/console/src/api/hooks.test.tsx`
- `platform-management/console/src/screens/TenantDetail.tsx` (sửa)
- `platform-management/console/src/screens/TenantDetail.test.tsx` (sửa)
- `platform-management/console/src/screens/UserDetail.tsx` (sửa)
- `platform-management/console/src/screens/UserDetail.test.tsx` (sửa)

**Thay đổi cụ thể:**
1. `TenantDetail.tsx`: `PageHeader` (tên, `slug` • `id` mono, `StatusBadge`, nút `Change status`, `Edit subscription`); `Banner` khi có `state.reason` hoặc `deletion_scheduled_at`. Các khối *Workspace State*, *Subscription*, *Usage*, *Billing profile* dùng `Section` + `MetadataList`. Giữ `data-testid="usage-current"` và `usage-daily`. Khối *Recent Orders* và *Workspace Audit Log* dùng `Table` tràn viền (`padding={0}`). Thay 3 `<select>` (`:1106`, `:1168`, `:1245`) bằng `Select`.
2. Xác nhận trong mã xem bảng dòng phí theo ngày từ `useTenantUsage` đã được render hay chưa. Nếu chưa, thêm bảng `charges`, `subtotal`, `discount_amount`, `total`.
3. `UserDetail.tsx`: `PageHeader` với nhóm nút; `Alert` khi bị khóa; `MetadataList` cho *Account Record*; `Section` + `Table` cho *Workspace Memberships*.

**Lệnh kiểm chứng:**
```bash
pnpm --filter @multica/platform-console typecheck
pnpm --filter @multica/platform-console exec vitest run src/api/schemas.test.ts src/api/hooks.test.tsx src/screens/TenantDetail.test.tsx src/screens/UserDetail.test.tsx
```

---

### Slice 5: `Orders` và `Staff`

**Mục tiêu:** Dùng `Tabs` của `@multica/ui`; hiển thị `kind`; thêm `Write off` cho đơn `usage` của `admin`; thay `<select>` thô trong `Staff`.

**Tệp chạm tới:** `screens/Orders.tsx`, `Orders.test.tsx`, `Staff.tsx`, `Staff.test.tsx`.

**Thay đổi cụ thể:**
1. `Orders.tsx`:
   - `PageHeader`.
   - Thay `<button role="tab">` (`:200`) bằng `Tabs`, `TabsList`, `TabsTrigger` cho 5 trạng thái (`pending`, `paid`, `expired`, `canceled`, `refunded`).
   - Hiển thị `order.kind` (`subscribe`, `change_plan`, `add_seats`, `renew`, `usage`).
   - Đơn `usage` ở `pending`: nút `Write off` chỉ hiển thị khi `effectiveRole === "admin"` (đúng quy tắc `orders.go:296`).
   - Icon `Check` (`:271`): `text-success`.
2. `Staff.tsx`: `PageHeader` với `Add staff`; thay `<select id="staff-role-select">` (`:212`) bằng `Select`; `Alert` cho lỗi `last_admin`.

**Lệnh kiểm chứng:**
```bash
pnpm --filter @multica/platform-console typecheck
pnpm --filter @multica/platform-console exec vitest run src/screens/Orders.test.tsx src/screens/Staff.test.tsx
```

---

### Slice 6: `Plans` và lịch đổi giá

**Mục tiêu:** Hoàn thiện phần còn lại của `Plans` và `PriceSchedule` theo Astryx. Phần lỗi `RefusePriceFields` đã được sửa.

**Đã xong:**
- `handleSavePlan` không gửi `seat_price_month`/`seat_price_year` (`Plans.tsx:180-190`).
- `PriceSchedule.tsx` đã có dialog lập lịch, `dry_run`, `confirm_large_change`, hủy lịch chờ; hook `usePriceChanges`, `useSchedulePriceChange`, `useCancelScheduledPriceChange` đã có (`hooks.ts:737-797`).

**Lưu ý:** Giữ `currency` trong `PUT /plans/{key}` (`currency` không nằm trong `PriceFields`, `pricechange.go:47`; xem `ASSUMPTION-6` trong spec).

**Tệp chạm tới:**
- `platform-management/console/src/screens/Plans.tsx` (sửa)
- `platform-management/console/src/screens/Plans.test.tsx` (sửa)
- `platform-management/console/src/screens/PriceSchedule.tsx` (sửa)
- `platform-management/console/src/screens/PriceSchedule.test.tsx` (sửa)
- `platform-management/console/src/api/schemas.ts` (sửa nếu thiếu `audience`, `billing_model`)

**Thay đổi cụ thể:**
1. `Plans.tsx:470,481`: thay hai `<input type="checkbox">` bằng `Checkbox` hoặc `Switch`.
2. Thêm cột `Audience` và `Billing Model` vào bảng. Nếu `PlanSchema` chưa có hai trường này, thêm vào với `parseWithFallback` và giá trị mặc định.
3. `PriceSchedule.tsx`: thay `amber-*` (`:510-513`) bằng `bg-warning/10 text-warning`; thay `emerald-*` (`:561`) bằng `text-success`; thay checkbox thô (`:521`) bằng `Checkbox`.
4. Kiểm tra quyền xem lịch sử giá cho `billing` theo `ASSUMPTION-4` trước khi thay đổi `App.tsx`.

**Lệnh kiểm chứng:**
```bash
pnpm --filter @multica/platform-console typecheck
pnpm --filter @multica/platform-console exec vitest run src/api/schemas.test.ts src/api/hooks.test.tsx src/screens/Plans.test.tsx src/screens/PriceSchedule.test.tsx
```

---

### Slice 7: `Accounts` — chuẩn hóa màn hình đã có

**Mục tiêu:** Đưa `Accounts.tsx` về cùng chuẩn với các màn hình khác (Tabs, Select, Checkbox, token ngữ nghĩa).

**Đã xong:** Màn hình, hook `useAccountRequests`, `useCreateAccountRequest`, `useCancelAccountRequest`, `useResendWelcome`; nối vào `App.tsx` cho `admin`.

**Tệp chạm tới:**
- `platform-management/console/src/screens/Accounts.tsx` (sửa)
- `platform-management/console/src/screens/Accounts.test.tsx` (sửa)

**Thay đổi cụ thể:**
1. Thay `role="tab"` tự chế (`:534`) bằng `Tabs`.
2. Thay 3 `<select>` (`:689`, `:830`, `:883`) và checkbox (`:908`) bằng `@multica/ui`. Giữ nhãn `aria-label` để test không đổi.
3. Thay `amber-*`/`emerald-*` (`:170`, `:185`, `:512`) bằng token ngữ nghĩa.
4. `PageHeader` với nút `Create account`. Giữ kiểm tra khớp email (`email_confirm`) và hiển thị lỗi theo trường `fields`.

**Lệnh kiểm chứng:**
```bash
pnpm --filter @multica/platform-console typecheck
pnpm --filter @multica/platform-console exec vitest run src/screens/Accounts.test.tsx src/App.test.tsx
pnpm --filter @multica/platform-console build
```

---

### Slice 8: Màn hình phía Tenant (`packages/views/billing/` & `packages/views/settings/components/`)

**Mục tiêu:** Đồng bộ với Astryx và `tokens.css`; xóa lớp chữ không tồn tại trong `order-page.tsx`; xóa "Card soup" trong `tokens-tab.tsx`.

> **Lưu ý trước khi thực hiện:** Trong lần rà soát này, `git status` của `packages/views` **không** còn thay đổi chưa commit (các file `billing-profile-form.tsx`, `plan-change-dialog.tsx`, v.v. đã được commit). Vẫn kiểm tra lại `git status --short packages/views` trước khi sửa và không ghi đè thay đổi chưa commit của người khác.

**Tệp chạm tới:**
- `packages/views/billing/order-page.tsx` (sửa, 567 dòng)
- `packages/views/billing/order-page.test.tsx` (sửa nếu cần)
- `packages/views/settings/components/tokens-tab.tsx` (sửa, 439 dòng)
- `packages/views/settings/components/audit-log-tab.tsx` (sửa)
- `packages/views/settings/components/account-tab.tsx` (sửa)
- `packages/views/settings/components/billing-tab.tsx` (sửa nhẹ: `Banner` cho `scheduled_price_change`, thẻ `usage`)

**Thay đổi cụ thể:**
1. `order-page.tsx`: `text-title-md` (`:411`, `:449`, `:477`) → `text-title-sm`; `bg-surface-muted` (`:444`) → `bg-muted`. Thay 3 `Card` bằng `Section` + `SettingsCard` phẳng. Giữ `id`, `name`, `role="alert"`, `aria-label`.
2. `tokens-tab.tsx`: một `SettingsCard` chứa hàng `divide-y divide-surface-border` thay cho `Card` từng token (`:289`); form tạo token vào `SettingsCard` phẳng; thay `<details>` (`:180`) bằng văn bản gọn.
3. `audit-log-tab.tsx`: bọc `Table` trong `rounded-lg border border-surface-border bg-surface`; thêm `Clear filters` khi không có kết quả.
4. `account-tab.tsx`: `securityEvents` (`:485-499`) dùng `font-mono text-caption` cho mã hành động và `tabular-nums` cho thời gian.
5. `billing-tab.tsx`: hiển thị `scheduled_price_change` bằng `Banner`; hiển thị `usage` bằng `SettingsCard` khi dữ liệu có mặt.
6. Mọi chuỗi mới phải thêm đủ 7 ngôn ngữ trong `packages/views/locales/`.

**Lệnh kiểm chứng:**
```bash
pnpm catalog:build
pnpm --filter @multica/views typecheck
pnpm --filter @multica/views exec vitest run billing/order-page.test.tsx settings/components/audit-log-tab.test.tsx settings/components/billing-tab.test.tsx settings/components/tokens-tab.test.tsx settings/components/account-tab.test.tsx
```

---

### Slice 9: **[Mới]** `PaymentNotices`, `EinvoiceFailures` và `PriceSchedule` theo Astryx

**Mục tiêu:** Ba màn hình đã được thêm sau bản kế hoạch đầu tiên nhưng chưa được đối chiếu với Astryx.

**Tệp chạm tới:**
- `platform-management/console/src/screens/PaymentNotices.tsx` (sửa)
- `platform-management/console/src/screens/PaymentNotices.test.tsx` (sửa)
- `platform-management/console/src/screens/EinvoiceFailures.tsx` (sửa)
- `platform-management/console/src/screens/EinvoiceFailures.test.tsx` (sửa)
- `PriceSchedule.tsx` được xử lý ở Slice 6.

**Thay đổi cụ thể:**
1. `PaymentNotices.tsx`: `PageHeader`; `Table` tràn viền; `StatusBadge` cho trạng thái thông báo; `EmptyState` có `actions`; hành động resolve/dismiss qua `ConfirmReasonDialog`. Bỏ `Card` bao toàn trang nếu có.
2. `EinvoiceFailures.tsx`: thay tab thô (`:94`) bằng `Tabs`; `PageHeader`; `StatusBadge` cho trạng thái hóa đơn; không dùng màu cứng.

**Lệnh kiểm chứng:**
```bash
pnpm --filter @multica/platform-console typecheck
pnpm --filter @multica/platform-console exec vitest run src/screens/PaymentNotices.test.tsx src/screens/EinvoiceFailures.test.tsx
```

---

## Kiểm chứng tổng thể sau khi hoàn tất các Slices

Chạy bộ lệnh chuẩn của repo từ thư mục gốc:

```bash
pnpm catalog:build
pnpm typecheck
pnpm lint
pnpm test
pnpm --filter @multica/platform-console build
git diff --check
```

Và kiểm tra thủ công trên cổng `30001`: đổi màn hình, F5 (sau Slice 2 phải giữ được màn hình), kiểm tra vai trò `billing`/`support`/`admin` chỉ thấy đúng mục điều hướng.

## Kiểm tra màu cứng và lớp không tồn tại (đặt làm bước chốt)

Sau mỗi slice thuộc Console hoặc `packages/views`, chạy để đảm bảo không còn màu Tailwind cứng hay lớp chữ ngoài `tokens.css`:

```bash
grep -rn "emerald-\|amber-\|red-[0-9]\|text-title-xl\|text-title-md\|bg-surface-muted\|role=\"tab\"\|<select\|type=\"checkbox\"" platform-management/console/src packages/views/billing packages/views/settings/components --include=*.tsx | grep -v "\.test\."
```

Kết quả mong đợi: không có dòng nào (trừ khi có ghi chú lý do trong spec).
