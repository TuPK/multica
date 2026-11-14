# Platform management cho SaaS nhiều tenant: đặc tả

Trạng thái: **đặc tả đã chốt, sẵn sàng triển khai; chưa có mã nào được viết.** D1 đến D4 do người dùng chốt. Người dùng đã giao toàn quyền chọn phương án (2026-11-14), nên D5 đến D30 **đã chốt** theo ủy quyền đó; lý do từng cái ghi ngay trong bảng ở mục 1.4. Kế hoạch từng bước: [platform-management-plan.md](platform-management-plan.md). Hai tài liệu này là nguồn duy nhất cho việc triển khai hạng mục này.

Ngày viết: 2026-11-14. Đọc mã ở **HEAD `1c631cf2c`**. Trích dẫn dạng `tệp:dòng` đã được đối chiếu bằng script với HEAD này (tệp có thật, dòng chứa đúng ký hiệu được nêu). Các giả định nền đã được đọc mã để xác nhận trong đợt hoàn thiện; kết quả ở mục 15.1 và hai phụ lục. Điều còn chưa kiểm chứng và con số là lựa chọn cũng ghi ở mục 15.

Ngôn ngữ: tiếng Việt. Tên tệp, kiểu, lệnh, mã lỗi giữ tiếng Anh. Chú thích trong mã viết tiếng Anh.

Thuật ngữ: **Tenant** = một workspace cùng thuê bao của nó. **Nhà vận hành** = bên chạy hệ thống SaaS (bạn). **Staff** = nhân viên của nhà vận hành có quyền trên console. **Console** = giao diện quản trị của nhà vận hành. **Guard** = lớp kiểm trạng thái tài khoản và workspace trong đường xác thực. **Cổng (gate)** = một điểm cưỡng chế giới hạn theo gói. **Thông tin xác thực máy** = token `mat_` (agent đang chạy task) và `mcn_` (cloud node).

## 0. Điều kiện tiên quyết về giấy phép (G-LIC)

`LICENSE` Phần I điều 1a (`LICENSE:24`) cấm dùng mã nguồn Multica để cung cấp dịch vụ lưu trữ cho bên thứ ba, kể cả miễn phí, nếu chưa có **giấy phép thương mại** từ Index Labs (Hong Kong) Limited. Điều 1b (`LICENSE:49`) cấm gỡ hoặc sửa logo, tên sản phẩm Multica trên giao diện nếu chưa có văn bản miễn trừ thương hiệu.

Hệ quả cho hạng mục này:

- Viết mã, chạy thử nội bộ và dùng trong một tổ chức **không** cần giấy phép thương mại, nên việc triển khai theo tài liệu này không bị chặn.
- **Mở dịch vụ cho người ngoài tổ chức bị chặn** cho tới khi có giấy phép thương mại. Đây là cổng G-LIC trong kế hoạch, đứng trước mọi lần ra mắt công khai, kể cả bản beta miễn phí.
- Tài liệu này không phải tư vấn pháp lý. Người dùng cần đọc toàn văn `LICENSE` và làm việc với bên cấp phép.

## 1. Mục tiêu, phạm vi, bất biến, quyết định

### 1.1 Mục tiêu

Sau khi triển khai xong, nhà vận hành có đủ công cụ để chạy Multica như một SaaS nhiều tenant trên **một hệ thống chung**, phục vụ **cả nhóm nhỏ tự đăng ký lẫn doanh nghiệp**, bán tại **Việt Nam trước** và không phải làm lại khi ra quốc tế. Cụ thể là bảy năng lực:

1. **Kiểm soát tài khoản và phiên**: khóa, mở, buộc đăng xuất, xóa và xuất dữ liệu tài khoản, tất cả lưu trong cơ sở dữ liệu và có hiệu lực không cần triển khai lại.
2. **Nhật ký kiểm toán**: mọi hành động quản trị để lại vết bền vững, không sửa được, đọc được bởi chủ workspace và bởi staff.
3. **Console của nhà vận hành**: tra cứu tenant và người dùng, khóa hoặc mở, quản lý gói, xác nhận thanh toán, mà không đọc được nội dung của tenant.
4. **Gói, hạn mức và ghế**: mỗi workspace có một thuê bao; máy chủ cưỡng chế giới hạn theo gói.
5. **Thanh toán phù hợp Việt Nam**: đơn hàng, chuyển khoản có xác nhận, vòng đời dùng thử, quá hạn, ân hạn, hạ gói.
6. **Tính năng doanh nghiệp**: MFA, SSO qua OIDC có xác minh tên miền, chính sách theo workspace, token giới hạn quyền, vai trò chỉ xem.
7. **Vận hành theo tenant**: hạn mức API theo workspace, xuất dữ liệu workspace, số liệu sử dụng.

### 1.2 Bất biến thiết kế

| # | Bất biến | Hệ quả |
| --- | --- | --- |
| I1 | Máy chủ là nơi quyết định quyền. Client chỉ hiển thị | Mọi quy tắc mới có test ở máy chủ; `packages/core/permissions/rules.ts` chỉ phản chiếu |
| I2 | Chỉ thêm, không phá. Hành vi khi tắt mọi cấu hình mới bằng hành vi hiện tại | Bảng mới, gói mới `server/internal/platform`, `server/internal/audit`; mã lõi chỉ nhận các điểm móc liệt kê ở mục 3.3 |
| I3 | Không khóa ngoại, không cascade. Mọi chỉ mục tạo bằng `CREATE [UNIQUE] INDEX CONCURRENTLY`, mỗi chỉ mục một tệp một câu lệnh | Bảng mới không khai báo `PRIMARY KEY` hay `UNIQUE` nội tuyến, theo tiền lệ `server/migrations/950_hub_activity.up.sql` (AGENTS.md "Database and Migration Rules") |
| I4 | Có đúng một nguồn quyết định thương mại | `PLATFORM_MODE=local` và `MULTICA_CLOUD_URL` không được cùng bật; máy chủ từ chối khởi động nếu cả hai có giá trị |
| I5 | Thông tin xác thực máy không chạm được vào bảo mật tài khoản, thanh toán hay console | Mọi route mới thuộc ba nhóm đó đứng sau `RequireHumanActor` (`server/internal/handler/actor_guards.go:15`) |
| I6 | Staff không có quyền đọc nội dung tenant | Console chỉ trả siêu dữ liệu và số đếm. Không có mạo danh người dùng trong phạm vi này |
| I7 | Hành động đặc quyền và dòng kiểm toán của nó thành công hoặc thất bại cùng nhau | Ghi kiểm toán trong cùng giao dịch; ghi thất bại thì hành động thất bại (tiền lệ: `GetAgentEnv` ở `server/internal/handler/agent_env.go`) |
| I8 | Khóa và thu hồi là fail-closed ở lớp guard; lỗi Redis không được làm sập xác thực | Redis lỗi thì đọc thẳng DB; DB lỗi thì trả 503, không cho qua |
| I9 | Tương thích API: client cũ không vỡ khi gặp giá trị mới | Trường mới là tùy chọn; enum mới có fallback; JSON cho UI đi qua schema zod và `parseWithFallback` (AGENTS.md "API Compatibility") |
| I10 | Không mất dữ liệu của tenant vì lý do thương mại | Quá hạn, hạ gói, vượt hạn mức chỉ chặn tạo mới hoặc chuyển chỉ đọc. Xóa dữ liệu chỉ xảy ra theo yêu cầu của chủ workspace hoặc sau thời gian chờ đã thông báo |

### 1.3 Phạm vi theo bốn đợt phát hành

| Đợt | Nội dung | Mục |
| --- | --- | --- |
| **R1** Nền tảng an toàn | Guard tài khoản, buộc đăng xuất mọi thiết bị, nhật ký kiểm toán, ghi nhận đồng ý điều khoản, xuất và xóa tài khoản, vá điểm lộ email lời mời | 4 |
| **R2** Thương mại | Chế độ `local`, staff và console, trạng thái workspace, gói và thuê bao, ghế, cổng hạn mức mới, thanh toán và vòng đời, hạn mức API theo workspace, xuất dữ liệu workspace, số liệu sử dụng | 5 |
| **R3** Doanh nghiệp | MFA, SSO OIDC với xác minh tên miền, chính sách theo workspace, PAT giới hạn, vai trò chỉ xem | 6 |
| **R4** Mở rộng | Project hạn chế truy cập và khách mời, cổng thanh toán tự động, hóa đơn điện tử, thanh toán quốc tế. Mỗi phần cần đặc tả riêng trước khi viết mã | 7 |

R1 luôn bật, không phụ thuộc cấu hình thương mại, nên bản tự lưu trữ cũng được hưởng. R2 chỉ hoạt động khi `PLATFORM_MODE=local`. Ngoài phạm vi: mục 14.

### 1.4 Quyết định

| # | Nội dung | Trạng thái |
| --- | --- | --- |
| D1 | Phục vụ cả nhóm nhỏ tự đăng ký và doanh nghiệp, trong cùng một sản phẩm; khác nhau ở gói và chính sách theo workspace | Người dùng chốt |
| D2 | Mọi khách hàng chung một hệ thống (nhiều tenant, một cơ sở dữ liệu) | Người dùng chốt |
| D3 | Tái sử dụng tối đa cái đã có. Diễn giải: mã nguồn dịch vụ Multica Cloud không có trong repo, nên **không gọi dịch vụ đó**; thay vào đó cài đặt tại chỗ hai giao diện mà mã lõi đã dùng, `entitlement.Provider` (`server/internal/entitlement/types.go:82`) và `seatcapacity.Executor` (`server/internal/seatcapacity/client.go:61`), và phục vụ đúng hợp đồng JSON của `/api/cloud-subscriptions/*` để giao diện thanh toán hiện có dùng lại được | Người dùng chốt hướng; cách làm đã chốt |
| D4 | Việt Nam trước, quốc tế sau: tiền tệ và nhà cung cấp thanh toán là dữ liệu, không viết cứng | Người dùng chốt |
| D5 | Quyết định thương mại chạy **trong tiến trình máy chủ** (gói `platform`), không dựng dịch vụ riêng. Lý do: chung cơ sở dữ liệu nên giới hạn kiểm được trong cùng giao dịch, không cần giao thức bù trừ qua mạng; ít thành phần phải vận hành hơn | Đã chốt |
| D6 | Trạng thái tài khoản và workspace lưu ở **bảng riêng** (`platform_account`, `platform_workspace`), không thêm cột vào `user`, `workspace`. Không có dòng nghĩa là `active`. Lý do: giảm xung đột khi nhập mã từ upstream | Đã chốt |
| D7 | Buộc đăng xuất theo người dùng bằng mốc thời gian `sessions_valid_after`, không lưu bảng phiên. Danh sách thiết bị và thu hồi từng phiên để ngoài phạm vi | Đã chốt |
| D8 | Kiểm toán ghi bằng lời gọi tường minh trong handler, cùng giao dịch, không qua event bus. Lý do: I7 | Đã chốt |
| D9 | Staff là bảng riêng `platform_staff`, tách khỏi vai trò workspace. Email trong `PLATFORM_ADMIN_EMAILS` luôn là `admin`. Ba vai trò: `admin`, `support`, `billing` | Đã chốt |
| D10 | Không có mạo danh. Hỗ trợ khách hàng dựa trên siêu dữ liệu, nhật ký kiểm toán và việc khách mời staff vào workspace như một thành viên thường | Đã chốt |
| D11 | Console chỉ có trên web (`apps/web`, route `/platform`), không có trên desktop và mobile | Đã chốt |
| D12 | Bốn trạng thái workspace: `active`, `read_only`, `suspended`, `pending_deletion`. Agent ngừng nhận task mới khi workspace không `active` | Đã chốt |
| D13 | Thanh toán đợt đầu là **trả trước theo kỳ**, bằng chuyển khoản do staff xác nhận. Cổng thanh toán tự động là một adapter sau giao diện `PaymentProvider`, làm ở R4 | Đã chốt |
| D14 | Vòng đời khi hết kỳ: ân hạn 7 ngày đầy đủ chức năng, rồi chỉ đọc 30 ngày, rồi hạ về gói miễn phí, dữ liệu giữ nguyên | Đã chốt (số ngày là lựa chọn, đổi được bằng cấu hình) |
| D15 | Xóa workspace trong chế độ `local` có 14 ngày chờ; chủ workspace hủy được | Đã chốt (số ngày là lựa chọn, đổi được bằng cấu hình) |
| D16 | Xóa tài khoản có 14 ngày chờ, sau đó ẩn danh hóa bản ghi người dùng; nội dung đã viết trong workspace ở lại với workspace | Đã chốt; tư vấn pháp lý xác nhận trước G-LIC (mục 16) |
| D17 | MFA đợt đầu chỉ TOTP kèm mã khôi phục. Staff bắt buộc có MFA ngay khi R3 phát hành | Đã chốt |
| D18 | SSO đợt đầu chỉ OIDC. SAML và SCIM ngoài phạm vi | Đã chốt |
| D19 | Khi workspace bắt buộc SSO, chủ workspace vẫn đăng nhập được bằng mã email để tránh tự khóa | Đã chốt |
| D20 | PAT giới hạn theo hai chiều cưỡng chế tập trung: gắn vào một workspace, và chỉ đọc. Không làm bảng scope theo từng route | Đã chốt |
| D21 | Thêm đúng một vai trò `viewer` (chỉ xem), tính là một ghế. Vai trò kế toán và khách mời ngoài phạm vi đợt này | Đã chốt |
| D22 | Project hạn chế truy cập để ở R4 với đặc tả riêng. Lý do ở mục 7.1 | Đã chốt |
| D23 | Định giá **thuần theo ghế**: giá của một thuê bao bằng đơn giá ghế nhân số ghế, không có phí nền. Lý do: hợp đồng giá mà giao diện hiện có đọc chỉ mang một đơn giá theo kỳ (Phụ lục A), và mô hình này dễ giải thích với khách | Đã chốt |
| D24 | Danh mục gói khởi tạo theo bảng ở mục 5.5. Staff sửa được trong console mà không cần triển khai lại | Đã chốt (con số là lựa chọn) |
| D25 | Điểm dừng agent khi workspace không `active` là cổng nhận task đã có, `claimRefusalFor` (`server/internal/service/budget_gate.go:316`), không phải handler của daemon. Lý do: đây là điểm duy nhất mọi lần nhận task đi qua, và nó đã có sẵn cơ chế "từ chối thì task ở lại hàng đợi" | Đã chốt |
| D26 | Mọi việc nền của hạng mục đăng ký bằng `scheduler.JobSpec` (`server/internal/scheduler/spec.go:102`). Lý do: bộ lập lịch này đã có giành quyền chạy qua cơ sở dữ liệu, nhịp tim và thử lại, nên nhiều tiến trình máy chủ không chạy trùng một việc | Đã chốt |
| D27 | Thư viện: `github.com/pquerna/otp` cho TOTP; `github.com/coreos/go-oidc/v3` cùng `golang.org/x/oauth2` cho OIDC. Phiên bản ghim lúc triển khai, khai báo ở `server/go.mod` | Đã chốt |
| D28 | Chu kỳ của cổng `autopilot_runs` là tháng dương lịch theo UTC, không theo kỳ thanh toán. Lý do: gói miễn phí không có kỳ thanh toán, và một quy tắc cho mọi gói dễ hiểu hơn | Đã chốt |
| D29 | Trang thanh toán hiện có được dùng lại nguyên vẹn; thêm một bộ chọn gói và trang đơn hàng. Máy chủ báo `payment_provider` trong bản tóm tắt để client đổi nhãn và bỏ các bước riêng của Stripe | Đã chốt |
| D30 | Cổng thanh toán tự động ở R4 là adapter **đối soát chuyển khoản qua thông báo giao dịch ngân hàng**, khớp theo `transfer_code`. Lý do: giữ nguyên luồng chuyển khoản mà khách doanh nghiệp Việt Nam quen dùng, chỉ thay bước staff xác nhận bằng máy. Nhà cung cấp đầu tiên và hợp đồng API của họ được chọn và đọc tài liệu chính thức trong đặc tả R4 | Đã chốt hướng |

## 2. Hiện trạng và khoảng trống

### 2.1 Cái đã có và dùng lại

- **Danh tính**: `Auth` (`server/internal/middleware/auth.go:59`) nhận JWT, `mul_`, `mat_`, `mcn_`; `DaemonAuth` (`server/internal/middleware/daemon_auth.go:118`) nhận thêm `mdt_`. Cả hai xóa header danh tính do client gửi trước khi tự đóng dấu.
- **Vai trò workspace**: `owner`, `admin`, `member`, đọc từ DB mỗi request trong `buildMiddleware` (`server/internal/middleware/workspace.go:197`). Luôn còn ít nhất một owner (`server/internal/handler/workspace.go:680`, `:756`, `:818`).
- **Chuyển quyền sở hữu** đã làm được: owner nâng một thành viên lên owner rồi tự hạ mình (`UpdateMember`). Không cần endpoint mới.
- **Gỡ thành viên** thu hồi trọn vẹn trong một giao dịch: lưu trữ agent, hủy task, ngắt kết nối (`server/internal/handler/workspace_revoke.go:45`, `server/internal/realtime/membership.go:67`).
- **Hai giao diện thương mại** đã được mã lõi gọi: `entitlement.Provider` cho cổng `issue_count` và `autopilot_runs` (`server/internal/entitlement/types.go:15`), `seatcapacity.Executor` cho lời mời và link chia sẻ. Chúng được nối ở `server/cmd/server/router.go:502` và `:519`, hiện trỏ tới Multica Cloud.
- **Giao diện thanh toán theo workspace** đã có ở client, đứng sau cờ `billing_workspace_subscriptions` (`server/internal/featureflags/keys.go:14`), đọc `WorkspaceSubscriptionSummary` (`packages/core/types/billing.ts:225`).
- **Chặn thông tin xác thực máy**: `RequireHumanActor` đã bảo vệ PAT, CLI token, billing (`server/cmd/server/router.go:2133`).
- **Mã hóa bí mật**: `secretbox.LoadKey` đang dùng cho bí mật plugin (`server/cmd/server/router.go:1499`); dùng lại cho bí mật SSO và MFA.

### 2.2 Khoảng trống

| # | Khoảng trống | Bằng chứng | Giải ở |
| --- | --- | --- | --- |
| K1 | Khóa tài khoản là danh sách viết cứng trong mã | `server/internal/auth/temporary_disabled_users.go:14` | 4.1 |
| K2 | Không thu hồi được phiên; `Logout` chỉ xóa cookie | `server/internal/handler/auth.go:788` | 4.2 |
| K3 | Hành động quản trị không để lại vết; `activity_log` gắn theo issue | `server/migrations/001_init.up.sql:156` | 4.3 |
| K4 | Không xóa hay xuất được dữ liệu tài khoản | không có route nào trong `server/cmd/server/router.go` | 4.5, 4.6 |
| K5 | Mọi thành viên đọc được email người đang được mời | route ở nhóm thành viên `server/cmd/server/router.go:1902`; handler không kiểm vai trò `server/internal/handler/invitation.go:252` | 4.7 |
| K6 | Không có khái niệm staff, không có console | không có cột hay bảng vai trò toàn hệ thống | 5.2, 5.4 |
| K7 | Không khóa được workspace | không có trạng thái workspace | 5.3 |
| K8 | Quyết định gói và giới hạn nằm ngoài repo | `server/internal/entitlement/README.md` | 5.5 |
| K9 | Chỉ có hai cổng hạn mức | `server/internal/entitlement/types.go:15` | 5.7 |
| K10 | Thanh toán đi qua Stripe của bên khác | `server/internal/handler/cloud_billing.go` | 5.8 |
| K11 | Hạn mức đăng nhập tắt khi thiếu Redis; không có hạn mức theo workspace | `server/cmd/server/router.go:1688` | 5.1, 5.10 |
| K12 | Mã đăng nhập phát triển chỉ tắt khi `APP_ENV=production` | `server/internal/handler/auth.go:137` | 5.1 |
| K13 | Không có MFA, SSO | không có mã liên quan | 6.2, 6.3 |
| K14 | Chính sách đăng ký là cấu hình toàn hệ thống, không theo workspace | `server/internal/handler/auth.go:259` | 6.4 |
| K15 | PAT có toàn quyền của người dùng | `server/internal/handler/personal_access_token.go` | 6.5 |
| K16 | Chỉ ba vai trò; không có vai trò chỉ xem | `packages/core/types/workspace.ts:1` | 6.6 |
| K17 | Mọi thành viên thấy mọi project và issue | `loadIssueForUser` chỉ lọc theo workspace | 7.1 |

## 3. Kiến trúc

### 3.1 Thành phần

```
server/internal/audit/        ghi và đọc nhật ký kiểm toán (R1)
server/internal/platform/     guard, staff, gói, thuê bao, ghế, thanh toán, vòng đời (R1: guard; R2: phần còn lại)
server/internal/platform/sso  OIDC, xác minh tên miền (R3)
server/internal/platform/mfa  TOTP, mã khôi phục (R3)
apps/web/app/platform/        console của nhà vận hành (R2)
packages/core/platform-admin/ query và mutation của console (R2)
packages/views/settings/      tab nhật ký kiểm toán, bảo mật tài khoản, chính sách, SSO
packages/views/billing/       trang đơn hàng và hướng dẫn chuyển khoản
```

`packages/core/platform/` đã tồn tại với nghĩa khác (lớp nối nền tảng web và desktop), nên phần query của console dùng tên `platform-admin`.

### 3.2 Đường đi của một request sau khi triển khai

```
request
  → Auth / DaemonAuth        xác định người dùng từ token (không đổi)
  → AccountGuard             tài khoản bị khóa? phiên cấp trước mốc thu hồi?          [R1]
  → RequireHumanActor        (chỉ trên nhóm nhạy cảm; không đổi)
  → buildMiddleware          thành viên của workspace? vai trò?  (không đổi)
      → WorkspaceGuard       trạng thái workspace cho phép phương thức này?            [R2]
      → PolicyGuard          workspace đòi SSO, MFA, tuổi phiên?                        [R3]
      → ViewerGuard          vai trò chỉ xem gọi phương thức ghi?                       [R3]
      → TokenGuard           PAT bị gắn workspace khác, hoặc chỉ đọc mà gọi ghi?       [R3]
      → WorkspaceRateLimit   vượt hạn mức API của gói?                                  [R2]
  → handler                  quy tắc theo tài nguyên (không đổi) + cổng hạn mức + ghi kiểm toán
```

Bốn guard sau `buildMiddleware` dùng chung một hàm `platform.CheckWorkspaceAccess(r, member)` trả về mã lỗi hoặc cho qua. Hàm này cũng được gọi trong `requireWorkspaceMember` (`server/internal/handler/handler.go:1048`), vì một số route tự phân giải workspace mà không qua middleware (ví dụ `UploadFile` ở `server/internal/handler/file.go:374`).

### 3.3 Điểm móc vào mã lõi

Đây là toàn bộ chỗ mã hiện có bị sửa. Mỗi điểm là vài dòng gọi sang gói mới; logic nằm ở gói mới. Danh sách này là hợp đồng để việc nhập mã upstream sau này biết phải kiểm lại chỗ nào.

| Tệp | Sửa | Đợt |
| --- | --- | --- |
| `server/internal/middleware/auth.go`, `daemon_auth.go` | gọi `AccountGuard` sau khi có `userID` ở mỗi nhánh token | R1 |
| `server/internal/realtime/hub.go` (`HandleWebSocket`, `:798`) | gọi `AccountGuard` khi mở kết nối | R1 |
| `server/internal/auth/session.go` (`RenewSessionToken`), `server/internal/handler/auth.go` (`issueJWT`, `:166`) | thêm và chép tiếp claim `auth_time` (R1), `amr` (R3) | R1, R3 |
| `server/internal/handler/workspace.go`, `invitation.go`, `share_link.go`, `personal_access_token.go`, `auth.go` | bọc giao dịch và gọi `audit.Record` | R1 |
| `server/cmd/server/router.go` | chuyển route danh sách lời mời sang nhóm admin; đăng ký route mới; nối `Platform`; chọn Provider và Executor theo `PLATFORM_MODE` | R1, R2 |
| `server/internal/middleware/workspace.go` (`buildMiddleware`), `server/internal/handler/handler.go` (`requireWorkspaceMember`) | gọi `platform.CheckWorkspaceAccess` | R2, R3 |
| `server/internal/service/budget_gate.go` (`claimRefusalFor`, `:316`) | từ chối nhận task khi workspace không `active`, trước mọi phân loại lần chạy | R2 |
| `server/internal/handler/daemon.go` (`requireDaemonWorkspaceAccess`, `:56`) | từ chối vai trò chỉ xem | R3 |
| `server/cmd/server/main.go` | đăng ký các việc nền bằng `schedulerMgr.Register`, cạnh các việc đã có (`:941`) | R1, R2 |
| `server/internal/handler/cloud_billing.go` (7 handler `/api/cloud-subscriptions/*`) | rẽ sang cài đặt tại chỗ khi `h.Platform != nil` | R2 |
| `packages/core/api/schemas.ts` (`StripeHostedURLSchema`, `:3698`; `WorkspaceSubscriptionSummarySchema`, `:3756`), `packages/views/settings/components/billing-tab.tsx` | nhận thêm địa chỉ `http` của máy cục bộ khi phát triển; đọc trường tùy chọn `payment_provider`; bộ chọn gói | R2 |
| `server/internal/entitlement/types.go` | thêm tên cổng mới vào `GateName` và `valid()` | R2 |
| `server/internal/handler/agent.go` (`CreateAgent` `:1726`), `daemon.go` (`DaemonRegister` `:558`), `file.go` (`UploadFile`), `workspace.go` (`CreateWorkspace` `:255`, `DeleteWorkspace` `:1176`) | gọi cổng hạn mức; xóa workspace có thời gian chờ | R2 |
| `server/internal/handler/auth.go` (`VerifyCode`, `GoogleLogin`) | bước MFA; nhánh SSO | R3 |
| `server/internal/auth/pat_cache.go`, `server/internal/handler/personal_access_token.go` | lưu và trả giới hạn của PAT | R3 |
| `server/internal/handler/workspace.go` (`normalizeMemberRole`), `invitation.go`, `share_link.go` | nhận vai trò `viewer` | R3 |
| `packages/core/types/workspace.ts`, `packages/core/permissions/rules.ts`, `packages/core/api/schema.ts` | vai trò mới, fallback enum | R3 |

### 3.4 Chế độ và cấu hình

| Biến | Ý nghĩa | Đợt |
| --- | --- | --- |
| `PLATFORM_MODE` | rỗng (mặc định, hành vi như hiện tại) hoặc `local` | R2 |
| `PLATFORM_ADMIN_EMAILS` | danh sách email luôn là staff `admin` | R2 |
| `PLATFORM_ADMIN_CIDRS` | tùy chọn: chỉ nhận request console từ các dải IP này | R2 |
| `PLATFORM_SECRET_KEY` | khóa `secretbox` cho bí mật SSO và MFA | R3 |
| `LEGAL_TERMS_VERSION`, `LEGAL_PRIVACY_VERSION` | phiên bản điều khoản và chính sách riêng tư đang hiệu lực | R1 |
| `SIGNUP_BLOCKED_EMAIL_DOMAINS` | tên miền email không được đăng ký mới | R2 |
| `AUDIT_RETENTION_DAYS` | số ngày giữ nhật ký khi gói không quy định; mặc định 365 | R1 |
| `BILLING_BANK_ACCOUNT_*`, `BILLING_VAT_RATE` | thông tin chuyển khoản hiển thị cho khách; thuế suất | R2 |

Các số ngày ở D14, D15, D16 và hạn đơn hàng đọc từ biến `PLATFORM_GRACE_DAYS` (7), `PLATFORM_READ_ONLY_DAYS` (30), `PLATFORM_WORKSPACE_DELETION_DAYS` (14), `PLATFORM_ACCOUNT_DELETION_DAYS` (14), `PLATFORM_ORDER_EXPIRY_DAYS` (7); giá trị trong ngoặc là mặc định.

Khi `PLATFORM_MODE=local`, máy chủ **từ chối khởi động** nếu: `MULTICA_CLOUD_URL` có giá trị (I4); thiếu `REDIS_URL` (K11); biến mã đăng nhập phát triển có giá trị (K12); `PLATFORM_ADMIN_EMAILS` rỗng; `JWT_SECRET` yếu (đã có kiểm tra ở `server/cmd/server/main.go`).

### 3.5 Việc nền

Mọi việc nền là một `scheduler.JobSpec` đăng ký ở `server/cmd/server/main.go` (D26). Mỗi việc lũy đẳng, có giới hạn số dòng mỗi lần chạy, và có tên cố định vì tên là khóa của lịch sử chạy.

| Tên | Nhịp | Việc | Đợt |
| --- | --- | --- | --- |
| `audit_retention` | ngày | xóa dòng kiểm toán quá hạn lưu | R1 |
| `account_deletion` | giờ | thực hiện xóa tài khoản đã hết thời gian chờ | R1 |
| `subscription_lifecycle` | giờ | chuyển trạng thái thuê bao; gửi nhắc | R2 |
| `order_expiry` | giờ | hết hạn đơn `pending` | R2 |
| `workspace_deletion` | giờ | xóa workspace đã hết thời gian chờ | R2 |
| `workspace_export` | phút | chạy việc xuất đang chờ | R2 |
| `platform_usage_daily` | ngày | chụp số liệu sử dụng | R2 |

## 4. R1: nền tảng an toàn

### 4.1 Guard tài khoản (K1)

**Dữ liệu.** `platform_account` mỗi người dùng tối đa một dòng: `status` (`active`, `suspended`, `pending_deletion`, `deleted`), `reason`, `changed_by`, `changed_at`, `sessions_valid_after`, `deletion_scheduled_at`. Không có dòng nghĩa là `active` và không có mốc thu hồi.

**Giao diện.** `auth.AccountGuard` có một phương thức `State(ctx, userID) (AccountState, error)`. Cài đặt đọc Redis khóa `mul:auth:acct:<userID>` (TTL 60 giây, lưu cả kết quả "không có dòng"), trượt thì đọc DB. Mọi lần ghi `platform_account` xóa khóa cache. Redis lỗi thì đọc DB. DB lỗi thì trả lỗi và middleware đáp 503 (I8). Guard `nil` cho qua, để test và bộ nhúng không phải dựng DB.

**Cưỡng chế.** Sau khi mỗi nhánh token xác định được người dùng:

| Trạng thái | Kết quả |
| --- | --- |
| `suspended`, `deleted` | 403, mã `account_suspended` |
| `pending_deletion` | 403, mã `account_pending_deletion`, trừ `GET /api/me`, `DELETE /api/me/deletion`, `POST /auth/logout` |
| JWT có thời điểm đăng nhập trước `sessions_valid_after` | 401, mã `session_revoked`; xóa cookie |

Áp cho cả năm loại token. Với `mat_`, người dùng bị kiểm là người mà token gắn vào. Với `mdt_`, là chủ runtime mà token gắn vào (`DaemonTokenOwnerIDFromContext`); token không có chủ (daemon do Qoder quản lý) không qua guard tài khoản. Kết nối WebSocket kiểm lúc mở. Khi một tài khoản bị khóa, máy chủ ngắt mọi kết nối đang mở của người đó bằng `Hub.DisconnectMember` cho từng workspace họ là thành viên.

**Độ trễ thu hồi.** Thông thường là tức thì nhờ xóa cache. Trường hợp xấu nhất, khi lệnh xóa cache thất bại, là 60 giây.

Danh sách viết cứng `temporary_disabled_users.go` giữ nguyên để không lệch upstream; nó trở nên thừa và tài liệu vận hành ghi rõ phải dùng console.

### 4.2 Buộc đăng xuất mọi thiết bị (K2)

JWT thêm claim `auth_time`: thời điểm đăng nhập gốc, đặt ở `issueJWT` và được `RenewSessionToken` chép tiếp nguyên vẹn. Token cũ chưa có `auth_time` được xét theo `iat`.

Lý do không dùng `iat` cho mọi token: gia hạn trượt đặt lại `iat`, nên một token bị đánh cắp và gia hạn sau thời điểm thu hồi sẽ lọt. Với `auth_time` chép tiếp, token đó vẫn mang thời điểm đăng nhập cũ và bị loại. Với token cũ, guard chạy **trước** bước gia hạn trong `Auth`, nên token có `iat` trước mốc bị loại trước khi kịp gia hạn.

`POST /api/me/sessions/revoke-all` (chỉ người thật) đặt `sessions_valid_after = now()`, xóa cache, ngắt WebSocket, rồi cấp một JWT mới cho chính request đó để người gọi không tự văng. Staff có lệnh tương đương cho bất kỳ người dùng nào (5.4). Khóa tài khoản không cần đặt mốc vì trạng thái đã chặn mọi token.

PAT không bị ảnh hưởng bởi lệnh này; người dùng thu hồi PAT ở trang token như hiện tại. Giao diện nói rõ điều đó.

### 4.3 Nhật ký kiểm toán (K3)

**Dữ liệu.** `audit_event`: `id`, `workspace_id` (rỗng với sự kiện cấp tài khoản hoặc cấp hệ thống), `actor_type` (`member`, `agent`, `system`, `staff`), `actor_id`, `actor_label` (email hoặc tên tại thời điểm ghi), `action`, `target_type`, `target_id`, `target_label`, `detail` (JSONB), `ip`, `user_agent`, `request_id`, `created_at`.

**Chỉ ghi thêm.** Tệp query của bảng này chỉ có `INSERT`, `SELECT` và một câu `DELETE` theo ngày cho việc dọn theo thời hạn lưu. Một test đọc tệp `.sql` và thất bại nếu xuất hiện `UPDATE`, trừ câu ẩn danh hóa `actor_label` khi xóa tài khoản.

**Ghi.** `audit.Record(ctx, q, Entry)` nhận `*db.Queries` của giao dịch đang mở. Handler chưa dùng giao dịch (ví dụ `UpdateMember`) được bọc giao dịch. Ghi lỗi thì trả lỗi lên, handler trả 500 và giao dịch hủy (I7). `detail` đi qua bộ lọc: khóa có tên chứa `secret`, `token`, `password`, `key`, `env` bị thay bằng `"[redacted]"`; có test.

**Danh mục hành động R1** (đợt R2 và R3 thêm ở các mục tương ứng):

| Nhóm | Hành động |
| --- | --- |
| Thành viên | `member.role_changed`, `member.removed`, `member.left` |
| Lời mời | `invitation.created`, `invitation.revoked`, `invitation.accepted`, `invitation.declined` |
| Link chia sẻ | `share_link.created`, `share_link.revoked`, `share_link.joined` |
| Workspace | `workspace.created`, `workspace.updated` (chỉ ghi tên trường đã đổi), `workspace.deleted` |
| Tài khoản | `auth.login`, `auth.sessions_revoked`, `pat.created`, `pat.revoked`, `account.export_requested`, `account.deletion_requested`, `account.deletion_canceled`, `consent.accepted` |

**Đọc.** `GET /api/workspaces/{id}/audit-events` cho owner và admin: phân trang theo khóa (`created_at`, `id`), lọc theo `action`, `actor_id`, khoảng thời gian; `?format=csv` xuất tệp. `GET /api/me/security-events` trả sự kiện cấp tài khoản của chính người gọi. Staff đọc xuyên workspace qua console.

**Thời hạn lưu.** Một job hằng ngày xóa dòng cũ hơn hạn của gói (R2) hoặc `AUDIT_RETENTION_DAYS`. Job là lũy đẳng và giới hạn số dòng mỗi lần chạy.

**Dữ liệu cá nhân.** Nhật ký chứa IP, user agent và có thể chứa email người được mời. Khi một tài khoản bị xóa, `actor_label` của người đó trong mọi dòng được thay bằng `deleted user`.

### 4.4 Ghi nhận đồng ý điều khoản

`user_consent`: `user_id`, `document` (`terms`, `privacy`), `version`, `accepted_at`, `ip`. `GET /api/me` thêm trường tùy chọn `consent_required` liệt kê tài liệu mà phiên bản đang hiệu lực chưa được người dùng chấp nhận. Client hiện hộp thoại chặn cho tới khi gọi `POST /api/me/consents`. Khi `LEGAL_*_VERSION` rỗng, tính năng tắt.

Máy chủ ghi nhận bằng chứng nhưng không chặn API khi thiếu đồng ý; việc chặn là ở client. Đây là lựa chọn có chủ ý để giữ đường xác thực đơn giản; nếu tư vấn pháp lý đòi chặn ở máy chủ thì thêm một trường vào `AccountState`.

### 4.5 Xuất dữ liệu tài khoản (K4)

`GET /api/me/export` (chỉ người thật, tối đa một lần mỗi giờ) trả một tệp JSON gồm: hồ sơ, tư cách thành viên và vai trò, siêu dữ liệu PAT (không có giá trị token), đồng ý đã ghi nhận, sự kiện bảo mật của tài khoản, kết nối bên ngoài gắn với người dùng. Nội dung đã viết trong workspace (issue, comment) thuộc về workspace và nằm trong bản xuất của workspace (5.11), không nằm ở đây.

### 4.6 Xóa tài khoản (K4)

`POST /api/me/deletion` (chỉ người thật, đòi `auth_time` trong vòng 10 phút, tức là vừa đăng nhập lại).

**Điều kiện.** Từ chối với mã `sole_owner` kèm danh sách nếu người dùng là owner duy nhất của một workspace còn thành viên khác. Họ phải chuyển quyền sở hữu hoặc xóa workspace trước. Workspace mà họ là thành viên duy nhất được liệt kê trong phản hồi xem trước (`?preview=1`) và sẽ bị xóa cùng tài khoản.

**Tiến trình.**

1. Đặt `status = pending_deletion`, `deletion_scheduled_at = now() + 14 ngày`, đặt mốc thu hồi phiên, ghi kiểm toán, gửi email xác nhận.
2. Trong 14 ngày, người dùng đăng nhập chỉ để hủy (`DELETE /api/me/deletion`).
3. Hết hạn, một job thực hiện trong các giao dịch theo từng workspace: gỡ tư cách thành viên bằng đúng đường `revokeAndRemoveMember` hiện có; xóa workspace một thành viên bằng đường xóa hiện có; thu hồi PAT; xóa kết nối và ràng buộc kênh chat của người dùng; rồi ẩn danh hóa bản ghi `user` (tên thành `Deleted user`, email thành một địa chỉ không gửi được và duy nhất, xóa ảnh đại diện và hồ sơ) và đặt `status = deleted`.

**Không bỏ sót bảng.** Một test nội quan lược đồ liệt kê mọi cột kiểu UUID có tên khớp `user_id`, `*_user_id`, `owner_id`, `creator_id`, `created_by`, `inviter_id` và đối chiếu với một sổ đăng ký trong mã ghi rõ cách xử lý từng cột khi xóa tài khoản (`delete`, `anonymize`, `keep`, `not_a_user`). Cột mới chưa đăng ký làm test thất bại. Cách này giữ cho việc xóa tài khoản đúng khi lược đồ lớn lên.

### 4.7 Vá điểm lộ email lời mời (K5)

Chuyển `GET /api/workspaces/{id}/invitations` sang nhóm owner và admin. Ở client, `invitationListOptions` (`packages/core/workspace/queries.ts:218`) chỉ chạy khi người xem là owner hoặc admin; hiện `members-tab.tsx:423` gọi vô điều kiện. Viết test đỏ trước để xác nhận một thành viên thường hiện đọc được danh sách.

## 5. R2: thương mại

### 5.1 Chế độ `local` và kiểm tra khi khởi động

`PLATFORM_MODE=local` dựng `platform.Service` và gán vào `Handler.Platform`. Nó cung cấp `entitlement.Provider` và `seatcapacity.Executor` tại chỗ thay cho client Cloud ở `server/cmd/server/router.go:502` và `:519`. Các kiểm tra từ chối khởi động ở mục 3.4 đóng K11 và K12.

### 5.2 Staff (K6)

`platform_staff`: `user_id`, `role`, `created_by`, `created_at`, `revoked_at`. Email thuộc `PLATFORM_ADMIN_EMAILS` là `admin` mà không cần dòng; đó là cách khởi tạo và là lối vào không thể tự khóa.

Middleware `RequirePlatformStaff(roles...)` đòi tất cả điều sau: qua `Auth` và `AccountGuard`; là người thật; đăng nhập bằng JWT (PAT bị từ chối); `auth_time` trong vòng 12 giờ; IP thuộc `PLATFORM_ADMIN_CIDRS` nếu có cấu hình; và từ R3 trở đi, phiên có MFA. Không đạt thì trả **404**, để sự tồn tại của console không bị dò.

| Vai trò | Quyền |
| --- | --- |
| `support` | xem tenant và người dùng; khóa và mở tài khoản, workspace; buộc đăng xuất; ghi chú |
| `billing` | quyền xem của `support`; đơn hàng, xác nhận thanh toán, đổi gói, gia hạn |
| `admin` | tất cả; quản lý staff; sửa danh mục gói; lên lịch và hủy xóa workspace |

Mọi hành động của staff ghi kiểm toán với `actor_type = staff`. Hành động trên một workspace ghi kèm `workspace_id`, nên **chủ workspace thấy được** trong nhật ký của mình rằng nhà vận hành đã làm gì.

### 5.3 Trạng thái workspace (K7)

`platform_workspace`: `workspace_id`, `status`, `reason`, `changed_by`, `changed_at`, `deletion_scheduled_at`. Không có dòng nghĩa là `active`. Cache như guard tài khoản, khóa `mul:plat:ws:<id>`.

| Trạng thái | Đọc | Ghi | Agent nhận task mới | Vào bằng cách nào |
| --- | --- | --- | --- | --- |
| `active` | có | có | có | mặc định |
| `read_only` | có | không, trừ danh sách cho phép | không | quá hạn sau ân hạn; staff |
| `suspended` | không, trừ danh sách cho phép | không | không | staff (vi phạm, yêu cầu pháp lý) |
| `pending_deletion` | không, trừ danh sách cho phép | không | không | chủ workspace xóa; staff |

Danh sách cho phép khi `read_only` (để khách tự gỡ rối được): mọi route `/api/cloud-subscriptions/*`, gỡ thành viên, rời workspace, thu hồi lời mời và link chia sẻ, xuất dữ liệu, xóa workspace, xóa và lưu trữ tài nguyên. Danh sách cho phép khi `suspended` và `pending_deletion`: đọc bản ghi workspace, `/api/cloud-subscriptions/*` cho owner và admin, rời workspace, xuất dữ liệu cho owner, hủy xóa cho owner.

Mã lỗi: `workspace_read_only`, `workspace_suspended`, `workspace_pending_deletion` (403).

**Dừng agent** (D25). Mọi lần nhận task đi qua `TaskService.ClaimTask` (`server/internal/service/task.go:3639`), và trong giao dịch nhận, qua cổng `claimRefusalFor`. Cổng này hôm nay cho qua vô điều kiện các lần chạy do người kích hoạt; kiểm tra trạng thái workspace được đặt **trước** bước phân loại đó, nên áp cho mọi loại lần chạy. Bị từ chối thì giao dịch nhận hủy và task ở lại hàng đợi, đúng cơ chế đã dùng cho ngân sách và giờ yên lặng. `TaskService` nhận trạng thái qua một giao diện một phương thức để gói `service` không phụ thuộc gói `platform`.

Autopilot ngừng sinh lần chạy mới qua cổng `autopilot_runs`: Provider tại chỗ trả giới hạn 0 khi workspace không `active` (giới hạn 0 hợp lệ theo `quotaPolicy` ở `server/internal/service/autopilot_quota.go:80`). Task đang chạy được để chạy xong; kết quả vẫn ghi được vì daemon ghi qua đường riêng không qua `buildMiddleware`.

### 5.4 Console (K6)

**API** dưới `/api/platform/*`, tất cả sau `RequirePlatformStaff`:

| Route | Vai trò | Nội dung |
| --- | --- | --- |
| `GET /tenants`, `GET /tenants/{id}` | mọi staff | tìm theo tên, slug, email owner; gói, trạng thái, số thành viên, ngày tạo, hoạt động gần nhất; chi tiết có thuê bao, số đếm sử dụng, đơn hàng, ghi chú |
| `POST /tenants/{id}/status` | `support`, `admin` | đổi trạng thái kèm lý do bắt buộc |
| `GET /tenants/{id}/audit-events` | mọi staff | nhật ký của workspace |
| `GET/POST /tenants/{id}/notes` | mọi staff | ghi chú nội bộ |
| `PUT /tenants/{id}/subscription` | `billing`, `admin` | đổi gói, số ghế, kỳ, gia hạn dùng thử, ghi đè giới hạn |
| `GET /orders`, `POST /orders/{id}/confirm`, `POST /orders/{id}/cancel` | `billing`, `admin` | đơn hàng và xác nhận thanh toán |
| `GET /users`, `GET /users/{id}` | mọi staff | tìm theo email; tư cách thành viên, trạng thái |
| `POST /users/{id}/status`, `POST /users/{id}/revoke-sessions`, `POST /users/{id}/mfa-reset` | `support`, `admin` | khóa, mở, buộc đăng xuất, đặt lại MFA (R3) |
| `GET/POST/DELETE /staff` | `admin` | quản lý staff |
| `GET/PUT /plans` | `admin` | danh mục gói |
| `GET /audit-events` | mọi staff | nhật ký toàn hệ thống |
| `GET /overview` | mọi staff | số tenant theo gói và trạng thái, đăng ký mới, đơn chờ xác nhận |

Console **không** có route trả issue, comment, chat, tệp đính kèm, cấu hình agent hay bí mật (I6). Một test duyệt các handler của nhóm `/api/platform` và thất bại nếu có handler gọi các query của những bảng đó.

**Giao diện** ở `apps/web/app/platform/`. `platform` được thêm vào `server/internal/handler/reserved_slugs.json` và chạy `pnpm generate:reserved-slugs`. `GET /api/me` thêm trường tùy chọn `platform_role` để giao diện biết có hiện lối vào hay không.

### 5.5 Gói và thuê bao (K8)

**`plan`**: `key`, `name`, `limits` (JSONB), `features` (JSONB), `currency`, `seat_price_month`, `seat_price_year`, `seats_fixed` (số ghế cố định của gói không bán ghế; rỗng với gói bán theo ghế), `min_seats`, `trial_days`, `is_public`, `sort`, `archived_at`. Giá là số nguyên theo đơn vị nhỏ nhất của tiền tệ; VND không có phần thập phân nên giá ghi thẳng bằng đồng. Danh mục khởi tạo từ một tệp nhúng khi bảng rỗng; sau đó staff `admin` sửa trong console. Gói đã có thuê bao thì chỉ lưu trữ được, không xóa được.

**`workspace_subscription`**: `workspace_id`, `plan_key`, `status`, `seats`, `interval`, `current_period_start`, `current_period_end`, `trial_end`, `grace_until`, `cancel_at_period_end`, `limit_overrides` (JSONB), `version`, `updated_at`. Workspace không có dòng được coi là gói miễn phí. `version` tăng một ở mỗi lần đổi và là `SubscriptionVersion` mà Provider trả.

**Giới hạn hiệu lực** = `limits` của gói, ghi đè bởi `limit_overrides`. Giá trị rỗng nghĩa là không giới hạn.

**Provider tại chỗ** cài `entitlement.Provider`: đọc thuê bao và gói, cache trong tiến trình 30 giây theo workspace, xóa cache ngay trên tiến trình thực hiện thay đổi. Với nhiều tiến trình máy chủ, độ trễ tối đa giữa các tiến trình là 30 giây; chấp nhận được vì đổi gói không phải thao tác khẩn. Hai cổng đang có (`issue_count`, `autopilot_runs`) được cưỡng chế bởi mã lõi hiện tại mà không sửa gì.

Provider phải trả `Decision` đúng dạng mà hai nơi tiêu thụ đòi, nếu không chúng coi là chính sách hỏng và **cho qua**:

- Cổng có giới hạn: `Action = enforce`, `Limit` khác rỗng và không âm. Cổng không giới hạn: `Action = off`.
- Riêng `autopilot_runs` còn phải có `PeriodStart`, `PeriodEnd`, `ResetAt`, với `PeriodStart` trước `PeriodEnd` (`server/internal/service/autopilot_quota.go:80`). Chu kỳ là tháng dương lịch theo UTC (D28); `ResetAt` bằng `PeriodEnd`.
- `PolicyRevision` là hằng số của bản cài đặt; `SubscriptionVersion` là `version` của thuê bao (0 khi chưa có dòng); `Reason` là `refreshed` khi vừa đọc DB và `cache_fresh` khi lấy từ cache.

**Danh mục gói khởi tạo** (D24; con số là lựa chọn thương mại, sửa được trong console):

| | Free | Pro | Business | Enterprise |
| --- | --- | --- | --- | --- |
| Đối tượng | dùng thử lâu dài | nhóm nhỏ tự đăng ký | doanh nghiệp | hợp đồng riêng |
| Công khai | có | có | có | không |
| Đơn giá ghế mỗi tháng (VND) | 0 | 149.000 | 349.000 | theo hợp đồng |
| Đơn giá ghế mỗi năm (VND) | 0 | 1.490.000 | 3.490.000 | theo hợp đồng |
| Ghế | cố định 3 | mua, tối thiểu 1 | mua, tối thiểu 5 | theo hợp đồng |
| Dùng thử | không | 14 ngày | 14 ngày | không |
| `issue_count` | 200 | không giới hạn | không giới hạn | không giới hạn |
| `autopilot_runs` mỗi tháng | 50 | 2.000 | 10.000 | theo hợp đồng |
| `agents` | 3 | 30 | 200 | theo hợp đồng |
| `runtimes` | 2 | 20 | 100 | theo hợp đồng |
| `storage_bytes` | 1 GB | 20 GB | 200 GB | theo hợp đồng |
| `api_rate_per_minute` | 300 | 1.200 | 3.000 | theo hợp đồng |
| `audit_retention_days` | 30 | 180 | 365 | theo hợp đồng |
| `audit_export` | không | có | có | có |
| `sso`, `workspace_policies`, `viewer_role` | không | không | có | có |

Một người làm owner tối đa **2** workspace trên gói miễn phí. Gói Enterprise là gói Business cộng `limit_overrides` do staff đặt cho từng workspace.

### 5.6 Ghế

Executor tại chỗ cài `seatcapacity.Executor` trên bảng `platform_seat_operation` (`workspace_id`, `operation_token`, `kind`, `subject_id`, `state`, `member_id`, `expires_at`). Sức chứa là `seats` của thuê bao, hoặc `seats_fixed` của gói. Số đã dùng là số dòng `member` của workspace; số đang giữ là số thao tác còn sống chưa thành thành viên (lời mời đang giữ chỗ, lượt vào bằng link đang xử lý). Mọi phương thức lũy đẳng theo `operation_token`. Workspace không giới hạn ghế trả `Managed = false`, khi đó handler đi theo đường không quản lý ghế như hiện tại. Ngữ nghĩa từng phương thức, chuỗi lý do và kiểu lỗi mà handler dựa vào: Phụ lục B.

Vì bốn handler lời mời và link chia sẻ đã gọi Executor đúng chỗ (`server/internal/handler/invitation.go:59`, `:443`, `server/internal/handler/share_link.go:264`), không handler nào phải sửa. Tính đúng của Executor tại chỗ được bảo đảm bằng một bộ test hợp đồng chạy cùng một kịch bản trên cả bản giả đang dùng trong test hiện có và bản tại chỗ (mục 13).

Khi số ghế mua ít hơn số thành viên (hạ gói), workspace ở trạng thái vượt chỗ: không thêm được người, không ai bị gỡ tự động (I10).

### 5.7 Cổng hạn mức mới (K9)

Thêm vào `GateName`: `agents`, `runtimes`, `storage_bytes`. Mỗi cổng được kiểm trong giao dịch tạo tài nguyên, sau khi lấy một khóa tư vấn theo cặp (workspace, cổng), nên hai request đồng thời không cùng vượt giới hạn.

| Cổng | Điểm cưỡng chế | Cách đếm |
| --- | --- | --- |
| `agents` | `CreateAgent`, khôi phục agent đã lưu trữ | agent chưa lưu trữ, `kind = user` |
| `runtimes` | `DaemonRegister` khi tạo runtime mới | runtime của workspace |
| `storage_bytes` | `UploadFile` và các đường tạo tệp đính kèm | tổng kích thước tệp đính kèm |

Vượt giới hạn trả 402 với mã `plan_limit_reached` và trường `gate`, `limit`, `used`; client mở hộp thoại nâng cấp như đã làm với giới hạn issue.

Hai giới hạn cấp tài khoản, không qua Provider: số workspace một người được làm owner trên gói miễn phí, kiểm trong `CreateWorkspace` cạnh hai kiểm tra đã có (`server/internal/handler/workspace.go:266`); và `SIGNUP_BLOCKED_EMAIL_DOMAINS`, kiểm trong `checkSignupAllowed`.

`features` của gói là cờ bật tắt: `sso`, `workspace_policies`, `viewer_role`, `audit_export`, `audit_retention_days`, `api_rate_per_minute`. Tính năng R3 kiểm cờ này trước khi cho cấu hình.

### 5.8 Thanh toán (K10)

**Mô hình.** Trả trước theo kỳ tháng hoặc năm. Số tiền một kỳ bằng đơn giá ghế nhân số ghế (D23), cộng thuế theo `BILLING_VAT_RATE`. Mỗi lần mua là một **đơn hàng**. Thanh toán thành công áp đơn vào thuê bao trong cùng giao dịch.

**`billing_order`**: `id`, `workspace_id`, `kind` (`subscribe`, `renew`, `add_seats`, `change_plan`), `plan_key`, `interval`, `seats`, `amount`, `vat_amount`, `currency`, `status` (`pending`, `paid`, `expired`, `canceled`, `refunded`), `provider`, `provider_ref`, `transfer_code`, `idempotency_key`, `buyer` (JSONB: tên đơn vị, mã số thuế, địa chỉ, email nhận hóa đơn), `period_start`, `period_end`, `einvoice_ref`, `created_by`, `confirmed_by`, `paid_at`, `expires_at`.

**`PaymentProvider`** có ba phương thức: tạo yêu cầu thanh toán cho một đơn, xác thực và diễn giải thông báo từ nhà cung cấp, truy vấn trạng thái. Đợt này có một cài đặt, `manual`: trả về thông tin chuyển khoản và `transfer_code` duy nhất để khách ghi vào nội dung chuyển tiền; staff `billing` đối soát và bấm xác nhận trong console. Adapter cho cổng tự động ở R4 chỉ thay bước xác nhận; mô hình đơn hàng không đổi.

**Áp thanh toán** (`ApplyPayment`) là một giao dịch: khóa dòng đơn, kiểm đơn còn `pending`, đặt `paid`, cập nhật thuê bao và tăng `version`, đưa workspace về `active` nếu đang `read_only` vì quá hạn, ghi kiểm toán. Gọi lại trên đơn đã `paid` là không làm gì. Đơn `pending` hết hạn sau 7 ngày.

**Mua thêm ghế giữa kỳ** tính theo tỷ lệ số ngày còn lại của kỳ, làm tròn theo đơn vị nhỏ nhất của tiền tệ. VND không có phần thập phân.

**Tái sử dụng giao diện** (D29). Bảy handler `/api/cloud-subscriptions/*` (`server/cmd/server/router.go:2185`) rẽ sang cài đặt tại chỗ khi `h.Platform != nil` và trả **đúng hợp đồng JSON hiện có**, ghi đầy đủ ở Phụ lục A. Tóm tắt cách ánh xạ:

| Route | Cài đặt tại chỗ |
| --- | --- |
| `GET summary` | dựng từ thuê bao, gói và sổ ghế. `has_stripe_customer = false`; `available_actions.portal = false`; thêm trường tùy chọn `payment_provider = "manual"` |
| `GET prices` | đơn giá ghế theo tháng và năm của gói đang xét: gói hiện tại nếu trả phí, gói trả phí công khai rẻ nhất nếu đang miễn phí; nhận `?plan=<key>` |
| `POST checkout-sessions` | tạo đơn `subscribe` hoặc `change_plan`; trả `url` là địa chỉ tuyệt đối của trang đơn hàng. Nhận thêm trường tùy chọn `plan_key` và `seats` |
| `POST seats/purchase-preview` | tính tiền mua thêm ghế theo tỷ lệ; `purchase_version` là `version` của thuê bao |
| `POST seats/purchases` | tạo đơn `add_seats` sau khi kiểm `expected_current_seats`, `expected_purchase_version`, `accepted_proration_amount`; trả `status = "pending"` |
| `POST seats/reconcile` | không có gì để đối soát; trả `action = "noop"` |
| `POST portal-sessions` | 404; client không gọi vì `portal = false` |

Cờ `billing_workspace_subscriptions` phải bật. Client thêm ba thứ: bộ chọn gói đọc `GET /api/workspaces/{id}/plans`; trang đơn hàng hiển thị số tiền, thông tin chuyển khoản, `transfer_code`, trạng thái và biểu mẫu thông tin xuất hóa đơn; và nhánh theo `payment_provider` để đổi nhãn nút và bỏ bước xử lý tham số quay về của Stripe. Trường `url` hiện bị schema buộc là `https` (`packages/core/api/schemas.ts:3698`); schema được nới để nhận thêm `http` chỉ khi máy chủ là `localhost` hoặc `127.0.0.1`, cho việc phát triển. Client đã định dạng đúng tiền tệ không có phần thập phân: `VND` nằm trong danh sách ở `packages/views/settings/components/billing-format.ts:12`.

**Hóa đơn.** Đợt này hệ thống lưu thông tin người mua và cho staff xuất danh sách đơn đã thanh toán để kế toán lập hóa đơn điện tử bằng công cụ sẵn có, rồi ghi số hóa đơn vào `einvoice_ref`. Tích hợp tự động ở R4.

### 5.9 Vòng đời thuê bao

```
free ──mua──▶ active ──hết kỳ, chưa gia hạn──▶ past_due ──7 ngày──▶ (workspace read_only) ──30 ngày──▶ free
  │              ▲                                   │                        │
  └─dùng thử─▶ trialing ──hết dùng thử, chưa mua──▶ free        thanh toán ở bất kỳ bước nào ─▶ active
```

- `trialing`: đủ quyền của gói được thử cho tới `trial_end`. Mỗi workspace dùng thử một lần.
- `past_due`: vẫn đủ chức năng; hiện biểu ngữ cho owner và admin; `grace_until = current_period_end + 7 ngày`.
- Hết ân hạn: workspace chuyển `read_only` với `reason = billing`.
- Sau 30 ngày chỉ đọc: thuê bao về `free`, workspace về `active`, áp giới hạn của gói miễn phí. Dữ liệu vượt giới hạn giữ nguyên; chỉ việc tạo mới bị chặn.
- `cancel_at_period_end`: về `free` khi hết kỳ, không qua ân hạn.

Việc nền `subscription_lifecycle` (mục 3.5) chạy mỗi giờ thực hiện các chuyển trạng thái. Mỗi chuyển là một giao dịch có điều kiện trên trạng thái hiện tại và `version`, nên chạy lại hay chạy song song không gây chuyển hai lần. Email nhắc gửi trước khi hết kỳ 14, 7 và 1 ngày, và ở mỗi lần chuyển trạng thái; mỗi email ghi một dòng `billing_notice` để không gửi trùng. Staff ghi đè được mọi bước từ console.

### 5.10 Hạn mức API theo workspace (K11)

Middleware `WorkspaceRateLimit` đặt sau `buildMiddleware`, đếm theo cửa sổ một phút trong Redis với khóa theo workspace, giới hạn lấy từ `features.api_rate_per_minute` của gói. Vượt thì 429 kèm `Retry-After`. Route của daemon (`/api/daemon/*`) không tính vào hạn mức này vì nhịp heartbeat không phải do người dùng điều khiển.

### 5.11 Xuất dữ liệu workspace

`POST /api/workspaces/{id}/exports` (owner, người thật) tạo một việc xuất chạy nền, ghi một tệp nén gồm các tệp NDJSON: workspace, thành viên, project, issue, comment, nhãn, trạng thái, danh mục tệp đính kèm. Kết quả lưu ở kho tệp, tải qua liên kết có chữ ký hết hạn sau 7 ngày. Mỗi workspace một việc xuất đang chạy; tối đa một lần mỗi ngày. Bản xuất không chứa bí mật: biến môi trường của agent, cấu hình MCP, token.

Đây là công cụ để khách mang dữ liệu đi và là điều kiện để I10 có nghĩa khi workspace bị khóa hoặc sắp xóa.

### 5.12 Xóa workspace có thời gian chờ

Khi `PLATFORM_MODE=local`, `DeleteWorkspace` đặt `pending_deletion` với hạn 14 ngày thay vì xóa ngay. Owner hủy bằng `DELETE /api/workspaces/{id}/deletion`. Hết hạn, job gọi đúng đường xóa hiện có. Ngoài chế độ `local`, hành vi xóa ngay giữ nguyên.

### 5.13 Số liệu sử dụng

`platform_usage_daily`: `workspace_id`, `day`, `members`, `agents`, `runtimes`, `issues_total`, `issues_created`, `tasks_run`, `storage_bytes`. Một job hằng đêm tính từ các bảng đã có. Console dùng bảng này để sắp xếp danh sách tenant và vẽ xu hướng; trang chi tiết tenant tính thêm số đếm hiện thời bằng truy vấn có giới hạn.

### 5.14 Kiểm toán đợt R2

Thêm vào danh mục: tích hợp (`integration.connected`, `integration.disconnected` cho GitHub, VCS, Slack, Lark, Telegram, WeCom, DingTalk, Qoder), plugin (`plugin.installed`, `plugin.uninstalled`, `plugin.enabled`, `plugin.disabled`, `plugin.configured`, `plugin.token_rotated`), thư viện MCP, hồ sơ runtime, agent (`agent.created`, `agent.archived`, `agent.permission_changed`, `agent.env_updated`), runtime (`runtime.visibility_changed`, `runtime.deleted`), thanh toán (`billing.order_created`, `billing.order_paid`, `subscription.changed`), trạng thái (`workspace.status_changed`, `account.status_changed`), xuất dữ liệu, và mọi hành động của staff.

## 6. R3: doanh nghiệp

### 6.1 Claim của phiên

JWT thêm `amr`: danh sách cách xác thực đã dùng, gồm `otp` (mã email), `google`, `sso:<connection_id>`, và `mfa` khi đã qua yếu tố thứ hai. `RenewSessionToken` chép tiếp `amr` và `auth_time`. Token cũ không có `amr` được coi là `["otp"]`.

### 6.2 MFA (K13)

`user_mfa`: `user_id`, `secret_enc`, `confirmed_at`, `recovery_hashes`, `last_used_step`.

- **Đăng ký**: `POST /api/me/mfa/enroll` trả bí mật và URI cho ứng dụng xác thực; `POST /api/me/mfa/confirm` với một mã đúng thì kích hoạt và trả mười mã khôi phục, hiện một lần.
- **Đăng nhập**: sau yếu tố thứ nhất, nếu người dùng có MFA, máy chủ **không** cấp phiên mà trả một token chờ sống 5 phút, mang `aud = mfa_pending`. `Auth` từ chối mọi token có `aud` này. `POST /auth/mfa/verify` nhận token chờ và mã TOTP hoặc mã khôi phục, rồi mới cấp JWT có `mfa` trong `amr`.
- **Chống dò**: tối đa 5 lần sai cho mỗi token chờ; `last_used_step` chặn dùng lại một mã trong cùng cửa sổ thời gian.
- **Tắt hoặc đặt lại**: người dùng tắt bằng một mã hợp lệ; mất thiết bị thì dùng mã khôi phục hoặc nhờ staff đặt lại (ghi kiểm toán, gửi email báo cho người dùng).
- **Staff**: từ khi R3 phát hành, `RequirePlatformStaff` đòi `mfa` trong `amr`.

### 6.3 SSO OIDC (K13)

**Xác minh tên miền.** `sso_domain`: `workspace_id`, `domain`, `token`, `verified_at`. Owner thêm tên miền, hệ thống cấp một giá trị để đặt vào bản ghi TXT, rồi kiểm tra DNS. Một tên miền chỉ được xác minh bởi một workspace. Tên miền email công cộng bị từ chối theo một danh sách.

**Kết nối.** `sso_connection`: `id`, `workspace_id`, `issuer`, `client_id`, `client_secret_enc`, `default_role`, `status`, `created_by`. Mỗi workspace một kết nối.

**Luồng đăng nhập.** Trang đăng nhập gửi email tới `POST /auth/sso/discover`. Nếu tên miền của email đã được xác minh và workspace có kết nối đang bật, client chuyển hướng sang nhà cung cấp danh tính bằng luồng authorization code có PKCE, `state` và `nonce`. Ở callback, máy chủ kiểm chữ ký ID token, `iss`, `aud`, `nonce`, hạn; đòi `email_verified`; và **đòi tên miền của email trong token thuộc các tên miền đã xác minh của chính workspace đó**. Điều kiện cuối là điểm an toàn cốt lõi: nếu thiếu, một workspace có thể cấu hình nhà cung cấp danh tính của mình để khẳng định bất kỳ email nào và chiếm tài khoản người khác.

**Cấp tài khoản tức thời.** Người dùng chưa tồn tại thì được tạo. Chưa là thành viên thì được thêm với `default_role`, đi qua Executor ghế như một lượt vào bằng link chia sẻ; hết ghế thì từ chối với thông báo rõ ràng.

**Bắt buộc SSO.** Khi chính sách `require_sso` bật, thành viên có email thuộc tên miền đã xác minh phải có `sso:<kết nối của workspace>` trong `amr` mới vào được workspace đó; thiếu thì 403 mã `sso_required` kèm địa chỉ bắt đầu luồng SSO. Owner được miễn (D19). Thành viên ngoài tên miền (nhà thầu, khách) theo chính sách lời mời ở 6.4.

Cả hai tính năng đòi cờ `sso` của gói.

### 6.4 Chính sách theo workspace (K14)

`workspace_policy`: `workspace_id`, `allowed_invite_domains`, `share_links_enabled`, `require_sso`, `require_mfa`, `session_max_age_hours`, `updated_by`, `updated_at`. Owner sửa; mọi thay đổi ghi kiểm toán (`policy.updated`, ghi giá trị cũ và mới).

| Chính sách | Điểm cưỡng chế | Mã lỗi |
| --- | --- | --- |
| `allowed_invite_domains` | `CreateInvitation`; `JoinByShareLink` theo email người vào | `invite_domain_not_allowed` |
| `share_links_enabled = false` | `CreateShareLink`, `JoinByShareLink`; tắt chính sách thì vô hiệu hóa link đang hoạt động | `share_links_disabled` |
| `require_mfa` | `platform.CheckWorkspaceAccess`: `amr` phải có `mfa` | `mfa_required` |
| `require_sso` | như 6.3 | `sso_required` |
| `session_max_age_hours` | `platform.CheckWorkspaceAccess`: `now - auth_time` | `reauth_required` |

Ba mã cuối chỉ chặn workspace đó; người dùng vẫn vào được các workspace khác của họ. Thông tin xác thực máy được miễn ba kiểm tra phiên này vì chúng không phải phiên của người: agent chạy theo quyền của người đã kích hoạt nó, và người đó đã qua kiểm tra khi kích hoạt.

### 6.5 PAT giới hạn (K15)

`personal_access_token` thêm hai cột tùy chọn: `workspace_id` và `read_only`. PAT hiện có giữ nguyên toàn quyền.

- PAT gắn workspace: `platform.CheckWorkspaceAccess` từ chối khi workspace của request khác workspace của token (403, `token_workspace_mismatch`); route không có ngữ cảnh workspace chỉ cho `GET /api/me`.
- PAT chỉ đọc: `Auth` từ chối phương thức không an toàn (403, `token_read_only`).

`PATCache` hiện chỉ lưu `userID`. Giá trị cache đổi thành cấu trúc có giới hạn, dưới một tiền tố khóa mới để các tiến trình chạy bản cũ và bản mới trong lúc triển khai cuốn chiếu không đọc nhầm của nhau.

### 6.6 Vai trò chỉ xem (K16)

Thêm `viewer` vào ràng buộc `CHECK` của `member`, `workspace_invitation`, `workspace_share_link`. Viewer đọc được những gì một thành viên đọc được.

`platform.CheckWorkspaceAccess` từ chối mọi phương thức không an toàn của viewer (403, `viewer_read_only`), trừ danh sách cho phép gồm các thao tác trên trạng thái cá nhân: đánh dấu đã đọc hộp thư, tùy chọn thông báo, ghim, rời workspace. `requireDaemonWorkspaceAccess` từ chối viewer, và cache tư cách thành viên của đường daemon không ghi cho viewer (cache hiện chỉ lưu "là thành viên", `server/internal/auth/membership_cache.go`).

Quy tắc ai được gán vai trò giữ nguyên: owner và admin gán được `viewer`, `member`, `admin`; chỉ owner gán được `owner`.

Client: `MemberRole` thêm `viewer`; `rules.ts` từ chối mọi quyền ghi cho viewer; vai trò máy chủ trả mà client không biết được coi là `viewer`, tức quyền thấp nhất (I9). Mobile có mã riêng và phải cập nhật cùng đợt. Client desktop bản cũ có thể hiện nút ghi cho viewer nhưng máy chủ từ chối; đây là suy giảm trải nghiệm chấp nhận được, không phải lỗ hổng.

Đòi cờ `viewer_role` của gói.

## 7. R4: mở rộng (mỗi phần cần đặc tả riêng)

### 7.1 Project hạn chế truy cập và khách mời (K17)

**Yêu cầu.** Một project có thể đặt là hạn chế; chỉ người được cấp mới thấy project và issue trong đó. Khách mời là thành viên chỉ thấy các project được cấp.

**Vì sao không gộp vào đợt này.** Đo ở HEAD: bảng `issue` được đọc ở khoảng 100 câu truy vấn trong 29 tệp query; sự kiện issue được phát cho toàn bộ workspace qua `ScopeWorkspace`; client đồng bộ chỉ mục tìm kiếm cục bộ của cả workspace; agent đọc được mọi issue; bảng điều khiển và các bản tổng hợp cộng dồn qua mọi issue. Một lỗi bỏ sót ở bất kỳ đường nào là rò rỉ dữ liệu giữa những người trong cùng workspace, tức là loại lỗi mà chính tính năng này bán lời hứa ngăn chặn.

**Hướng thiết kế để đặc tả riêng triển khai.** Cột `project.access` (`workspace`, `restricted`) và bảng `project_member`; một vị từ nhìn thấy duy nhất dùng chung cho mọi đường đọc; sự kiện của issue hạn chế phát theo người thay vì theo workspace; chỉ mục tìm kiếm lọc theo người; agent thừa hưởng tầm nhìn của người kích hoạt. Bước đầu tiên của đặc tả đó là bản kiểm kê đầy đủ các đường đọc, có test tự động phát hiện đường mới.

### 7.2 Cổng thanh toán tự động

Một adapter `PaymentProvider` đối soát chuyển khoản tự động (D30): nhà cung cấp gửi thông báo mỗi khi tài khoản nhận tiền; adapter kiểm chữ ký, tìm đơn theo `transfer_code` trong nội dung chuyển tiền, so số tiền, rồi gọi `ApplyPayment`. Giao dịch không khớp đơn nào hoặc lệch số tiền được đưa vào hàng chờ cho staff xử lý tay. Hợp đồng API của nhà cung cấp chưa được kiểm chứng trong tài liệu này; đặc tả R4 phải đọc tài liệu chính thức của họ trước khi thiết kế adapter.

### 7.3 Hóa đơn điện tử

Tích hợp một nhà cung cấp hóa đơn điện tử: phát hành khi đơn chuyển `paid`, lưu số và liên kết tra cứu vào `einvoice_ref`.

### 7.4 Quốc tế

Adapter thanh toán quốc tế, nhiều tiền tệ trong danh mục gói, lựa chọn vùng lưu dữ liệu. Mô hình đơn hàng và thuê bao ở mục 5 không cần đổi.

## 8. Mô hình dữ liệu

Tất cả là bảng mới, trừ ba thay đổi trên bảng đã có ở R3. Không khóa ngoại. Không `PRIMARY KEY` hay `UNIQUE` nội tuyến; tính duy nhất do chỉ mục `UNIQUE` tạo riêng bảo đảm (I3).

| Bảng | Đợt | Chỉ mục duy nhất | Chỉ mục khác |
| --- | --- | --- | --- |
| `platform_account` | R1 | `(user_id)` | `(status)` có điều kiện `status <> 'active'` |
| `audit_event` | R1 | `(id)` | `(workspace_id, created_at DESC, id)`; `(actor_id, created_at DESC)` |
| `user_consent` | R1 | `(user_id, document, version)` | |
| `platform_staff` | R2 | `(user_id)` | |
| `platform_workspace` | R2 | `(workspace_id)` | `(status)` có điều kiện `status <> 'active'` |
| `platform_note` | R2 | `(id)` | `(workspace_id, created_at DESC)` |
| `plan` | R2 | `(key)` | |
| `workspace_subscription` | R2 | `(workspace_id)` | `(status, current_period_end)` |
| `platform_seat_operation` | R2 | `(workspace_id, operation_token)` | `(workspace_id, state)` |
| `billing_order` | R2 | `(id)`; `(workspace_id, idempotency_key)`; `(transfer_code)` | `(status, created_at)`; `(workspace_id, created_at DESC)` |
| `billing_notice` | R2 | `(workspace_id, kind, period_end)` | |
| `workspace_export` | R2 | `(id)` | `(workspace_id, created_at DESC)` |
| `platform_usage_daily` | R2 | `(workspace_id, day)` | |
| `user_mfa` | R3 | `(user_id)` | |
| `sso_domain` | R3 | `(domain)` có điều kiện `verified_at IS NOT NULL` | `(workspace_id)` |
| `sso_connection` | R3 | `(workspace_id)` | |
| `workspace_policy` | R3 | `(workspace_id)` | |

Thay đổi trên bảng đã có (R3): `personal_access_token` thêm `workspace_id`, `read_only`; ràng buộc vai trò của `member`, `workspace_invitation`, `workspace_share_link` nhận thêm `viewer`.

**Đánh số migration.** Trình chạy sắp tệp theo thứ tự chuỗi (`server/internal/migrations/migrations.go:69`) và tiền tố ba chữ số đã dùng tới 997, nên không thể nối tiếp bằng số bốn chữ số. Dải **651 đến 699** đang trống ở HEAD này và đứng sau mọi bảng mà hạng mục này phụ thuộc; hạng mục giữ dải này. Dải 953 đến 969 là dự phòng. Trên cơ sở dữ liệu đang chạy, trình chạy vẫn áp dụng một tệp có số nhỏ hơn các tệp đã áp dụng, vì nó kiểm từng phiên bản một trong `schema_migrations` (U1, mục 15.1).

## 9. API

Chỉ liệt kê route mới và route đổi hành vi. Mọi phản hồi cho giao diện có schema zod ở client và test phản hồi sai dạng.

| Route | Quyền | Đợt |
| --- | --- | --- |
| `GET /api/workspaces/{id}/invitations` | **đổi**: owner, admin | R1 |
| `POST /api/me/sessions/revoke-all` | người thật | R1 |
| `GET /api/me/security-events` | người thật | R1 |
| `GET /api/workspaces/{id}/audit-events` | owner, admin | R1 |
| `POST /api/me/consents` | người thật | R1 |
| `GET /api/me/export` | người thật | R1 |
| `POST /api/me/deletion`, `DELETE /api/me/deletion` | người thật, vừa đăng nhập lại | R1 |
| `GET /api/me` | **thêm trường tùy chọn** `consent_required`, `platform_role`, `mfa_enabled` | R1, R2, R3 |
| `/api/platform/*` (mục 5.4) | staff | R2 |
| `/api/cloud-subscriptions/*` | **đổi cài đặt** khi `local`; hợp đồng giữ nguyên | R2 |
| `GET /api/workspaces/{id}/plans` | thành viên | R2 |
| `GET /api/workspaces/{id}/orders`, `GET /api/workspaces/{id}/orders/{orderId}`, `PUT .../buyer` | owner, admin, người thật | R2 |
| `POST /api/workspaces/{id}/exports`, `GET .../exports` | owner, người thật | R2 |
| `DELETE /api/workspaces/{id}/deletion` | owner | R2 |
| `POST /api/me/mfa/enroll`, `/confirm`, `DELETE /api/me/mfa` | người thật | R3 |
| `POST /auth/mfa/verify` | token chờ | R3 |
| `POST /auth/sso/discover`, `GET /auth/sso/start`, `GET /auth/sso/callback` | công khai, có hạn mức theo IP | R3 |
| `GET/PUT /api/workspaces/{id}/policy` | đọc: owner, admin; ghi: owner | R3 |
| `GET/POST/DELETE /api/workspaces/{id}/sso/domains`, `POST .../verify`, `GET/PUT/DELETE /api/workspaces/{id}/sso/connection` | owner | R3 |
| `POST /api/tokens` | **thêm trường tùy chọn** `workspace_id`, `read_only` | R3 |

## 10. Mã lỗi

Mọi lỗi mới trả JSON `{"error": "<thông điệp>", "code": "<mã>"}`; trường bổ sung ghi ở cột cuối. Client dịch theo `code`, không theo thông điệp.

| Mã | HTTP | Trường thêm |
| --- | --- | --- |
| `account_suspended`, `account_pending_deletion` | 403 | |
| `session_revoked` | 401 | |
| `sole_owner` | 409 | `workspaces[]` |
| `workspace_read_only`, `workspace_suspended`, `workspace_pending_deletion` | 403 | `reason` |
| `plan_limit_reached` | 402 | `gate`, `limit`, `used` |
| `plan_feature_unavailable` | 402 | `feature` |
| `workspace_rate_limited` | 429 | header `Retry-After` |
| `order_not_pending` | 409 | `status` |
| `mfa_required`, `sso_required`, `reauth_required` | 403 | `sso_start_url` với `sso_required` |
| `mfa_invalid_code`, `mfa_attempts_exceeded` | 400, 429 | |
| `sso_domain_unverified`, `sso_email_domain_mismatch` | 403 | |
| `invite_domain_not_allowed`, `share_links_disabled` | 403 | |
| `token_workspace_mismatch`, `token_read_only` | 403 | |
| `viewer_read_only` | 403 | |

## 11. Bảo mật

| Mối nguy | Biện pháp |
| --- | --- |
| Tài khoản staff bị chiếm | JWT mới đăng nhập trong 12 giờ, không nhận PAT hay token máy, giới hạn IP tùy chọn, MFA bắt buộc từ R3; mọi hành động ghi kiểm toán và tenant thấy được |
| Console rò rỉ nội dung tenant | Không có route trả nội dung; test duyệt handler (5.4); không mạo danh |
| Sửa hoặc xóa nhật ký | Không có query `UPDATE`; test trên tệp `.sql`; xóa chỉ theo thời hạn lưu |
| Bí mật lọt vào nhật ký | Bộ lọc khóa trên `detail`; test |
| Người bị khóa vẫn chạy agent | Guard áp cho `mat_` và `mdt_` theo người mà token gắn vào; workspace không `active` không giao task |
| Token bị đánh cắp sống sót sau thu hồi nhờ gia hạn | `auth_time` chép tiếp; guard chạy trước bước gia hạn |
| Cache guard cũ | Xóa cache khi ghi; TTL 60 giây là giới hạn trên |
| Workspace khẳng định email của người khác qua SSO | Chỉ nhận email thuộc tên miền workspace đã chứng minh bằng DNS; đòi `email_verified` |
| Tự khóa khi bắt buộc SSO | Owner vẫn đăng nhập bằng mã email |
| Bỏ qua MFA | Token chờ có `aud` riêng mà `Auth` từ chối; `amr` chỉ có `mfa` sau khi xác minh; gia hạn chép `amr`, không tự thêm |
| Dò mã TOTP | Giới hạn số lần theo token chờ; chặn dùng lại theo `last_used_step` |
| Xác nhận thanh toán giả hoặc lặp | `ApplyPayment` lũy đẳng, khóa dòng; chỉ staff `billing` xác nhận; ghi kiểm toán. Thông báo từ cổng tự động (R4) phải kiểm chữ ký |
| Vượt hạn mức do đua | Khóa tư vấn theo (workspace, cổng) trong giao dịch tạo |
| Liên kết bản xuất bị lộ | Liên kết có chữ ký, hạn 7 ngày; bản xuất không chứa bí mật; yêu cầu xuất ghi kiểm toán |
| Xóa tài khoản làm workspace mồ côi | Từ chối khi là owner duy nhất của workspace còn người khác |
| Agent bị chèn lệnh gọi route nhạy cảm | Mọi route bảo mật tài khoản, thanh toán, console sau `RequireHumanActor` |

## 12. Tương thích và triển khai

- **Mặc định tắt.** Không đặt biến nào thì chỉ có R1 hoạt động, và R1 không đổi hành vi cho đến khi có dòng trong `platform_account`.
- **Client cũ.** Trường mới của `GET /api/me` là tùy chọn. Mã lỗi mới đi kèm mã HTTP mà client cũ đã xử lý (401 về trang đăng nhập; 403 và 402 hiện lỗi chung). Vai trò `viewer` trên client cũ: mục 6.6.
- **Mobile.** Mã riêng (`apps/mobile/`). R1 cần mobile xử lý `session_revoked` và `account_suspended`; R3 cần luồng MFA, SSO và vai trò `viewer`. Đọc `apps/mobile/AGENTS.md` trước khi sửa.
- **Bật thương mại cho hệ thống đang chạy.** Bật `PLATFORM_MODE=local` khi đã có workspace: mọi workspace chưa có thuê bao là gói miễn phí. Để không ai bị chặn đột ngột, bước triển khai gán cho workspace hiện hữu một thuê bao chuyển tiếp không giới hạn có hạn, do staff đặt trước khi bật cưỡng chế. Trình tự này là một bước riêng trong kế hoạch.
- **Triển khai cuốn chiếu.** Claim JWT mới chỉ được thêm; bản cũ bỏ qua claim lạ. Khóa cache PAT đổi tiền tố (6.5).
- **Chuỗi giao diện** đủ bảy ngôn ngữ của web và desktop, cùng bản dịch riêng của mobile; không viết mô tả thừa (AGENTS.md "UI Copy").

## 13. Kiểm thử và tiêu chí chấp nhận

**Nguyên tắc.** Test viết trước, đỏ rồi xanh, đặt cạnh mã. Test Go chạm DB dùng `server/internal/testutil` và khẳng định trên dòng fixture của chính test, không trên tổng bảng (AGENTS.md "Testing"). Test mặc định không gọi agent thật.

**Bộ test bắt buộc ngoài test từng bước:**

1. **Ma trận token × trạng thái tài khoản**: năm loại token, bốn trạng thái, trên cả `Auth`, `DaemonAuth` và WebSocket.
2. **Ma trận trạng thái workspace × phương thức × danh sách cho phép**, trên cả đường middleware và đường `requireWorkspaceMember`.
3. **Hợp đồng Executor ghế**: một kịch bản chung chạy trên bản giả hiện có và bản tại chỗ; toàn bộ test lời mời và link chia sẻ hiện có chạy lại với bản tại chỗ.
4. **Hợp đồng JSON thanh toán**: phản hồi của bảy handler tại chỗ đi qua đúng schema zod mà client đang dùng.
5. **Vòng đời thuê bao**: mỗi cạnh của sơ đồ 5.9 với đồng hồ giả; chạy job hai lần liên tiếp không đổi kết quả.
6. **Cô lập tenant của console**: test duyệt handler ở 5.4.
7. **Sổ đăng ký cột người dùng** ở 4.6.
8. **Nhật ký chỉ ghi thêm và lọc bí mật** ở 4.3.
9. **Hồi quy**: toàn bộ test hiện có xanh sau mỗi bước, với mọi cấu hình mới để trống.

**Tiêu chí chấp nhận theo đợt.**

R1:
- AC1. Staff hoặc lệnh SQL đặt một tài khoản `suspended`; trong vòng 60 giây mọi request bằng JWT, PAT, token task và token daemon của người đó bị từ chối và WebSocket bị ngắt, không cần triển khai lại.
- AC2. Sau "đăng xuất mọi thiết bị", một token cũ không dùng được và không gia hạn được; phiên vừa gọi vẫn dùng được.
- AC3. Mỗi hành động trong danh mục R1 sinh đúng một dòng kiểm toán; làm hỏng việc ghi kiểm toán khiến hành động thất bại và không để lại thay đổi.
- AC4. Thành viên thường nhận 403 khi đọc danh sách lời mời.
- AC5. Xóa tài khoản: bị từ chối khi là owner duy nhất; sau thời gian chờ, bản ghi được ẩn danh hóa, không còn tư cách thành viên, PAT hay kết nối; email cũ đăng ký lại được như người mới.
- AC6. Bản xuất tài khoản không chứa giá trị token hay bí mật.

R2:
- AC7. Máy chủ từ chối khởi động với từng cấu hình sai ở mục 3.4.
- AC8. Người không phải staff nhận 404 trên mọi route `/api/platform/*`; PAT và token máy của staff cũng nhận 404.
- AC9. Workspace `suspended`: thành viên không đọc được dữ liệu, daemon không nhận được task, owner vẫn xem được thanh toán và xuất được dữ liệu.
- AC10. Vượt từng cổng hạn mức trả 402 `plan_limit_reached`; hai request đồng thời ở sát giới hạn chỉ một cái thành công.
- AC11. Mời người khi hết ghế bị từ chối; xác nhận một đơn mua ghế thì mời được ngay.
- AC12. Xác nhận cùng một đơn hai lần chỉ áp một lần.
- AC13. Workspace quá hạn đi đúng chuỗi ân hạn, chỉ đọc, hạ gói theo đồng hồ giả, và trở lại `active` khi thanh toán ở bất kỳ bước nào; không dòng dữ liệu nào bị xóa.
- AC14. Giao diện thanh toán hiện có hiển thị đúng gói, ghế, giá bằng VND với cài đặt tại chỗ.

R3:
- AC15. Tài khoản có MFA không nhận được phiên nếu chưa qua bước thứ hai; token chờ không gọi được API nào khác.
- AC16. Callback SSO với email ngoài tên miền đã xác minh bị từ chối.
- AC17. Workspace bật `require_sso`: thành viên thuộc tên miền đăng nhập bằng mã email bị chặn ở workspace đó nhưng vẫn vào được workspace khác; owner không bị chặn.
- AC18. PAT gắn workspace A bị từ chối ở workspace B; PAT chỉ đọc bị từ chối ở mọi phương thức ghi.
- AC19. Viewer bị từ chối ở mọi route ghi của workspace (test duyệt bảng route) trừ danh sách cho phép, và không đăng ký được runtime.

## 14. Ngoài phạm vi

- Mạo danh người dùng bởi staff (D10).
- Danh sách thiết bị và thu hồi từng phiên (D7).
- SAML, SCIM (D18). MFA bằng khóa bảo mật hoặc SMS (D17).
- Bảng scope chi tiết cho PAT; tài khoản dịch vụ cấp workspace (D20).
- Vai trò kế toán, vai trò tùy biến, khách mời (D21, 7.1).
- Tự động trừ tiền định kỳ; hoàn tiền tự động; mã giảm giá.
- Sao lưu và khôi phục theo từng tenant ở mức cơ sở dữ liệu. Công cụ cấp tenant là bản xuất (5.11); khôi phục toàn hệ thống là việc của hạ tầng.
- Trang trạng thái dịch vụ và thông báo trong ứng dụng từ nhà vận hành.
- Tín dụng trả trước và runtime đám mây do nhà vận hành cung cấp (`/api/cloud-billing/*`, `/api/cloud-runtime/*`): đây là sản phẩm của Multica Cloud, không dựng lại.
- Mỗi khách một hệ thống riêng (D2).

## 15. Chưa kiểm chứng và rủi ro

### 15.1 Kết quả xác nhận các giả định nền

Mười hai điểm dưới đây từng là giả định. Tám điểm đã được đọc mã để xác nhận trong đợt hoàn thiện; bốn điểm còn lại được xử lý bằng một bước cụ thể trong kế hoạch.

| # | Giả định | Kết quả | Bằng chứng |
| --- | --- | --- | --- |
| U1 | Trình chạy migration áp dụng được tệp có số nhỏ hơn các tệp đã áp dụng | **Đã xác nhận.** Trình chạy hỏi từng phiên bản đã có trong `schema_migrations` chưa rồi mới chạy, không dựa vào phiên bản lớn nhất; `AllVersions` ghi rõ có tính tới migration không theo thứ tự | `server/cmd/migrate/main.go:1116`, `server/internal/migrations/migrations.go:75` |
| U2 | Hợp đồng JSON của `/api/cloud-subscriptions/*` | **Đã xác nhận** từ schema zod của client; ghi ở Phụ lục A. Chưa thấy phản hồi thật của Multica Cloud, nhưng client chỉ chấp nhận những gì qua schema này nên schema là hợp đồng đủ | `packages/core/api/schemas.ts:3756` |
| U3 | Ngữ nghĩa giao thức ghế | **Đã xác nhận phần handler dựa vào**; ghi ở Phụ lục B. Chưa đọc hết worker phục hồi và giá trị của `Operation.State`; bước P19 viết bộ test hợp đồng trên bản giả đang có trước khi viết cài đặt | `server/internal/handler/seat_capacity.go:58` |
| U4 | Mọi lần nhận task đi qua một điểm | **Đã xác nhận.** `TaskService.ClaimTask` là nơi duy nhất gọi `ClaimAgentTask`, và cổng `claimRefusalFor` chạy trong giao dịch đó. Thiết kế đổi theo (D25) | `server/internal/service/task.go:3639`, `server/internal/service/budget_gate.go:316` |
| U5 | Autopilot đi qua cổng `autopilot_runs` | **Đã xác nhận đường tạo lần chạy có hạn mức** (`quotaPolicy`, `createAutopilotRunWithQuota`). Chưa liệt kê mọi nơi gọi; bước P18 có test cho từng nguồn kích hoạt (`schedule`, `manual`, `webhook`, `api`) | `server/internal/service/autopilot_quota.go:80` |
| U6 | Hạ tầng việc nền tránh chạy trùng | **Đã xác nhận.** `scheduler.JobSpec` có giành quyền chạy qua cơ sở dữ liệu, nhịp tim, thử lại. Thiết kế dùng nó (D26) | `server/internal/scheduler/spec.go:102`, `server/internal/scheduler/db_ops.go:60` |
| U7 | Client định dạng đúng VND | **Đã xác nhận.** `VND` có trong danh sách tiền tệ không có phần thập phân | `packages/views/settings/components/billing-format.ts:12` |
| U8 | Client có giả định về Stripe | **Đã xác nhận có ba chỗ**: schema buộc `url` là `https`; trang thanh toán xử lý tham số quay về của Stripe; nhãn nút nhắc Stripe. Cách xử lý ở mục 5.8 (D29) | `packages/core/api/schemas.ts:3698`, `packages/views/settings/components/billing-tab.tsx:262` |
| U9 | Vai trò thành viên có qua enum zod không | **Đã xác nhận là không**: vai trò đi qua như chuỗi, nên giá trị `viewer` không làm hỏng việc phân tích ở client cũ; kiểu `MemberRole` chỉ là kiểu TypeScript | `packages/core/types/workspace.ts:1` |
| U10 | Thư viện TOTP và OIDC | Đã chọn (D27). Phiên bản ghim và kiểm giấy phép thư viện ở bước P32 và P36 | |
| U11 | Mức ảnh hưởng tới mobile | Chưa đọc mã mobile. Bước P39 đọc `apps/mobile/AGENTS.md` và liệt kê trước khi sửa | |
| U12 | Danh sách bảng tham chiếu người dùng | Không liệt kê tay; test nội quan lược đồ ở 4.6 bắt buộc phân loại từng cột (bước P11) | |

Ngoài ra còn chưa kiểm chứng: hợp đồng API của nhà cung cấp đối soát chuyển khoản và của nhà cung cấp hóa đơn điện tử (R4); và việc `WorkspaceLocker` có được worker phục hồi giữ trong lúc gọi Executor hay không (Phụ lục B, bước P19).

### 15.2 Con số là lựa chọn, chưa đo

TTL cache guard 60 giây; cache gói 30 giây; ân hạn 7 ngày; chỉ đọc 30 ngày; chờ xóa 14 ngày; hạn đơn hàng 7 ngày; hạn liên kết xuất 7 ngày; cửa sổ đăng nhập lại 10 phút và 12 giờ; số lần thử MFA; thời hạn lưu nhật ký mặc định 365 ngày; mọi giới hạn và giá của gói. Ước lượng công ở kế hoạch là ước lượng của tác giả thiết kế.

### 15.3 Rủi ro

| Rủi ro | Mức | Giảm thiểu |
| --- | --- | --- |
| Chưa có giấy phép thương mại khi muốn ra mắt | cao | G-LIC; bắt đầu liên hệ bên cấp phép song song với R1 |
| Executor ghế tại chỗ lệch ngữ nghĩa với handler | cao | Bộ test hợp đồng trước khi nối; chạy lại toàn bộ test lời mời và link chia sẻ |
| Xung đột khi nhập mã upstream vào các tệp ở 3.3 | trung bình | Điểm móc nhỏ, logic ở gói riêng; danh sách 3.3 là danh sách kiểm sau mỗi lần nhập |
| Thêm một lần đọc cache vào mọi request | trung bình | Một lần `GET` Redis; đo độ trễ p95 trước và sau ở G-R1 |
| Bật thương mại làm khách hiện hữu bị chặn | trung bình | Thuê bao chuyển tiếp (mục 12) |
| Guard trả 503 khi DB chậm | thấp | Cache che phần lớn; đây là hành vi đúng theo I8 |
| Hết dải số migration 651 đến 699 | thấp | Kế hoạch đã phân bổ đủ 46 tệp trong dải; dự phòng 953 đến 969 |

## 16. Việc pháp lý và thuế cần người có chuyên môn xác nhận

Tài liệu này không khẳng định yêu cầu pháp lý nào. Các điểm sau ảnh hưởng tới thiết kế và cần xác nhận trước G-LIC:

1. Giấy phép thương mại và miễn trừ thương hiệu của Multica (mục 0).
2. Nghĩa vụ theo quy định bảo vệ dữ liệu cá nhân của Việt Nam: nội dung đồng ý (4.4), thời hạn xử lý yêu cầu xóa và việc giữ lại nội dung đã viết trong workspace (D16), thời hạn lưu nhật ký có chứa dữ liệu cá nhân (4.3).
3. Yêu cầu về nơi lưu dữ liệu đối với từng nhóm khách hàng.
4. Nghĩa vụ xuất hóa đơn điện tử, thuế suất, và thông tin người mua bắt buộc trên đơn hàng (5.8).
5. Điều khoản dịch vụ về khóa tài khoản, chỉ đọc khi quá hạn, và xóa dữ liệu sau thời gian chờ (5.3, 5.9, 5.12).

## Phụ lục A. Hợp đồng JSON của `/api/cloud-subscriptions/*`

Nguồn: schema zod ở `packages/core/api/schemas.ts` (từ `:3698`) và lời gọi ở `packages/core/api/client.ts` (từ `:2807`). Mọi đối tượng là "loose": trường lạ được bỏ qua, nên cài đặt tại chỗ thêm trường tùy chọn được. Phản hồi không khớp schema bị client coi là "không khả dụng", không bao giờ coi là gói miễn phí; vì vậy mỗi ràng buộc dưới đây phải có test.

**`GET /summary`**

```json
{
  "entitlement": {
    "workspace_id": "<uuid>",
    "plan": "pro",
    "status": "active",
    "seats": 5,
    "limits": {
      "issue_count":    { "mode": "unlimited" },
      "autopilot_runs": { "mode": "limited", "limit": 2000 }
    },
    "current_period_end": "<rfc3339> | null",
    "snapshot_expires_at": "<rfc3339> | null",
    "version": 7
  },
  "billing_interval": "month | year | null",
  "human_members": 4,
  "seat_capacity": {
    "purchased": 5, "used": 4, "reserved": 0, "available": 1,
    "overcommitted": false, "version": 7,
    "pending_quantity": null
  },
  "cancel_at_period_end": false,
  "grace_until": "<rfc3339> | null",
  "has_stripe_customer": false,
  "available_actions": { "checkout": true, "portal": false, "purchase_seats": true },
  "payment_provider": "manual"
}
```

Ràng buộc: `limit` là số nguyên dương khi `mode = "limited"` (giới hạn 0 không biểu diễn được ở đây; workspace không `active` vẫn hiển thị giới hạn của gói). `seats`, `human_members`, `used`, `reserved`, `available`, `entitlement.version` không âm. `seat_capacity.purchased` và `seat_capacity.version` **dương**; workspace không giới hạn ghế trả `seat_capacity: null`. Vì `version` của thuê bao là 0 khi chưa có dòng, `seat_capacity.version` trả `version + 1`. `pending_quantity` là số dương hoặc `null`; khi có đơn `add_seats` đang chờ, trả số ghế đang chờ và `active_purchase` gồm `request_id` (mã đơn), `target_seats`, `status = "pending"`, `expires_at`. `payment_provider` là trường thêm của cài đặt tại chỗ.

**`GET /prices`**: `{"month": {"currency": "vnd", "unit_amount": 149000, "interval": "month", "interval_count": 1}, "year": {…, "interval": "year", "interval_count": 1}}`. `unit_amount` dương; `interval_count` đúng bằng 1.

**`POST /checkout-sessions`**: yêu cầu `{"interval": "month|year", "idempotency_key": "…"}` cùng header `Idempotency-Key`; cài đặt tại chỗ nhận thêm `plan_key`, `seats`. Phản hồi `{"request_id": "…", "session_id": "…", "url": "https://…"}`; cả `request_id` và `session_id` là mã đơn hàng.

**`POST /seats/purchase-preview`**: yêu cầu `{"additional_seats": n}`. Phản hồi `{"current_seats", "additional_seats", "resulting_seats", "purchase_version", "currency", "proration_amount", "next_invoice_amount", "quoted_at"}`. Ba số ghế và `purchase_version` dương; `currency` là ba chữ cái **thường**; hai số tiền không âm.

**`POST /seats/purchases`**: yêu cầu `{"additional_seats", "expected_current_seats", "expected_purchase_version", "accepted_proration_amount", "currency", "idempotency_key"}`. Phản hồi `{"request_id", "current_seats", "additional_seats", "resulting_seats", "currency", "proration_amount", "next_invoice_amount", "status": "pending|submitted|confirmed"}`.

**`POST /seats/reconcile`**: phản hồi `{"workspace_id", "action", "version"}`.

**`POST /portal-sessions`**: phản hồi `{"url": "https://…"}`; cài đặt tại chỗ không phục vụ.

Lỗi ghế mà giao diện đã biết: `seat_capacity_full` (409), `seat_capacity_overcommitted` (409), `seat_capacity_rate_limited` (429), `seat_capacity_unavailable` (503), sinh ở `writeSeatCapacityError` (`server/internal/handler/seat_capacity.go:352`). Lỗi giới hạn issue đã có: 402 với `code = "issue_limit_reached"` (`server/internal/handler/issue_limit.go:37`); `plan_limit_reached` ở mục 10 theo đúng mẫu này.

## Phụ lục B. Giao thức ghế mà Executor tại chỗ phải giữ

Nguồn: `server/internal/seatcapacity/client.go` và `server/internal/handler/seat_capacity.go`. Phía sản phẩm đã có sẵn và **giữ nguyên**: bảng intent bền, worker phục hồi, và khóa theo workspace (`WorkspaceLocker`, một khóa tư vấn gắn phiên). Executor tại chỗ đóng vai mà trước đây Multica Cloud đóng.

**Kiểu trả về.** `Decision{Managed, Allowed, Reason, Operation, Capacity}`; `Capacity{PurchasedSeats, UsedSeats, ReservedSeats, Version}`; `Operation{Token, WorkspaceID, Kind, SubjectID, State, ExpiresAt}`.

| Phương thức | Handler gọi khi | Điều handler dựa vào |
| --- | --- | --- |
| `ReserveInvitation(ws, invitationID, expiresAt)` | tạo lời mời | `Managed = false`: bỏ qua quản lý ghế. `Allowed = false` với `Reason = "capacity_full"`: trả lỗi hết ghế; lý do khác bị coi là dịch vụ không khả dụng. Lỗi thỏa `IsCapacityOvercommitted`: trả lỗi vượt chỗ |
| `ClaimShareJoin(ws, token)` | vào bằng link chia sẻ | như trên; `Allowed = false` là hết ghế |
| `Consume(ws, token)` | chấp nhận lời mời | `Allowed = false`: handler **cho lời mời hết hạn** và trả hết ghế. Vì vậy chỉ trả `false` khi chỗ đã giữ thật sự không còn |
| `Confirm(ws, token, memberID)` | sau khi tạo thành viên | thành công khi `!Managed` hoặc `Allowed`; còn lại được ghi để worker thử lại |
| `Release(ws, token)` | thu hồi, từ chối, hết hạn, hoặc bù trừ | thành công khi `!Managed`, hoặc `Allowed`, hoặc `Reason = "released"`, hoặc lỗi thỏa `IsNotFound` |
| `ReleaseMember(ws, memberID)` | gỡ thành viên, rời workspace | như `Release` |
| `GetOperation(ws, token)` | worker phục hồi | trả trạng thái hiện tại của một thao tác |
| `RecoveryAvailable()` | lúc khởi động | trả `true` để worker phục hồi chạy |

**Quy tắc của cài đặt tại chỗ.**

1. Mọi phương thức lũy đẳng theo `token`: gọi lại trả cùng kết quả, không giữ thêm chỗ. Worker và handler có thể gọi lại sau lỗi.
2. `Release` và `ReleaseMember` trên thao tác không tồn tại hoặc đã giải phóng trả `Allowed = false`, `Reason = "released"`, không trả lỗi.
3. Vượt chỗ được báo bằng `*seatcapacity.HTTPError{StatusCode: 409, Code: "capacity_overcommitted"}`, vì handler nhận biết qua `IsCapacityOvercommitted` (`server/internal/seatcapacity/client.go:260`). Không bao giờ trả lỗi thỏa `IsRateLimited`.
4. Còn chỗ khi `PurchasedSeats - UsedSeats - ReservedSeats > 0`. `UsedSeats` đếm trực tiếp bảng `member`, nên thành viên có từ trước khi bật chế độ `local` được tính đúng mà không cần chuyển dữ liệu. `Confirm` chuyển thao tác khỏi nhóm đang giữ vì thành viên vừa tạo đã nằm trong `UsedSeats`.
5. Chỗ giữ cho lời mời hết hiệu lực tại `expiresAt`; việc đếm bỏ qua thao tác đã quá hạn.
6. Executor tự tuần tự hóa theo workspace bằng `pg_advisory_xact_lock` với một không gian khóa **khác** của `WorkspaceLocker`. Handler đang giữ khóa của `WorkspaceLocker` trên một kết nối khác khi gọi Executor; dùng chung khóa sẽ tự khóa chết.
7. `Capacity.Version` là `version` của thuê bao.

Bộ test hợp đồng (kế hoạch P19) mã hóa bảng và bảy quy tắc trên, chạy trên bản giả đang dùng trong `server/internal/handler/invitation_test.go` trước, rồi trên cài đặt tại chỗ.
