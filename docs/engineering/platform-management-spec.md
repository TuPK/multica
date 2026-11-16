# Platform management cho SaaS nhiều tenant: đặc tả

Trạng thái: **đặc tả đã chốt, sẵn sàng triển khai; chưa có mã nào được viết.** D1 đến D4 do người dùng chốt. Người dùng đã giao toàn quyền chọn phương án (2026-11-14), nên D5 đến D30 **đã chốt** theo ủy quyền đó. D31 đến D38 do người dùng chốt sau đó, khi đổi hình thái triển khai: platform management là một dịch vụ riêng, không nằm trong máy chủ Multica. Lý do từng cái ghi ngay trong bảng ở mục 1.4. D39 đến D48 thuộc đợt bổ sung năm nhóm tính năng (mục 7b, đợt R5), chốt trong đợt bổ sung theo yêu cầu của chủ dự án; bốn quyết định cũ bị thay hoặc thay một phần (D13, D21, D23, D24) đều là quyết định chốt theo ủy quyền, không phải do người dùng chốt, và được đánh dấu ngay ở bảng. Kế hoạch từng bước: [platform-management-plan.md](platform-management-plan.md). Hai tài liệu này là nguồn duy nhất cho việc triển khai hạng mục này.

Ngày viết: 2026-11-14. Đọc mã ở **HEAD `1c631cf2c`**. Trích dẫn dạng `tệp:dòng` đã được đối chiếu bằng script với HEAD này (tệp có thật, dòng chứa đúng ký hiệu được nêu). Các giả định nền đã được đọc mã để xác nhận trong đợt hoàn thiện; kết quả ở mục 15.1 và hai phụ lục. Điều còn chưa kiểm chứng và con số là lựa chọn cũng ghi ở mục 15. Các đoạn sửa theo D31 đến D38 trích thêm `platform-management/`, `server/internal/entitlement/`, `server/internal/seatcapacity/` và `server/internal/service/email.go` ở cây làm việc hiện tại; các dòng trích mới đã kiểm bằng `sed -n`. Mục 7b và các dòng K18 đến K22, D39 đến D48 được viết ngày 2026-11-15, đọc mã ở cây làm việc tại **HEAD `6979a4bb8`**; các trích dẫn mới của đợt này đã kiểm bằng `grep -n` ở cây làm việc đó, trong đó `server/migrations/670` đến `673` và `platform-management/internal/plan/` là tệp chưa commit lúc viết.

Ngôn ngữ: tiếng Việt. Tên tệp, kiểu, lệnh, mã lỗi giữ tiếng Anh. Chú thích trong mã viết tiếng Anh.

Thuật ngữ: **Tenant** = một workspace cùng thuê bao của nó. **Nhà vận hành** = bên chạy hệ thống SaaS (bạn). **Staff** = nhân viên của nhà vận hành có quyền trên console. **Platform** = dịch vụ riêng của nhà vận hành (thư mục `platform-management/`), gồm API Go và console. **Console** = ứng dụng một trang để quản trị, do platform phục vụ. **Guard** = lớp kiểm trạng thái tài khoản và workspace trong đường xác thực. **Cổng (gate)** = một điểm cưỡng chế giới hạn theo gói. **Thông tin xác thực máy** = token `mat_` (agent đang chạy task) và `mcn_` (cloud node).

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
| I2 | Chỉ thêm, không phá. Hành vi khi tắt mọi cấu hình mới bằng hành vi hiện tại | Bảng mới, dịch vụ platform riêng và các gói dùng chung `platform-management/audit`, `accountguard`, `workspacestate`; mã lõi chỉ nhận các điểm móc liệt kê ở mục 3.3 |
| I3 | Không khóa ngoại, không cascade. Mọi chỉ mục tạo bằng `CREATE [UNIQUE] INDEX CONCURRENTLY`, mỗi chỉ mục một tệp một câu lệnh | Bảng mới không khai báo `PRIMARY KEY` hay `UNIQUE` nội tuyến, theo tiền lệ `server/migrations/950_hub_activity.up.sql` (AGENTS.md "Database and Migration Rules") |
| I4 | Có đúng một nguồn quyết định thương mại | Đúng một nguồn thương mại được cấu hình, và nó là giá trị của `MULTICA_CLOUD_URL`: trỏ tới Multica Cloud, hoặc tới API platform, hoặc rỗng (không có nguồn nào, hành vi như hiện tại). Không có công tắc thứ hai (D36) |
| I5 | Thông tin xác thực máy không chạm được vào bảo mật tài khoản, thanh toán hay console | Mọi route mới thuộc ba nhóm đó ở Multica đứng sau `RequireHumanActor` (`server/internal/handler/actor_guards.go:15`). Route của console thuộc platform và chỉ nhận cookie `mpa_session` (D34), không nhận thông tin xác thực máy |
| I6 | Staff không có quyền đọc nội dung tenant | Console chỉ trả siêu dữ liệu và số đếm. Không có mạo danh người dùng trong phạm vi này |
| I7 | Hành động đặc quyền và dòng kiểm toán của nó thành công hoặc thất bại cùng nhau | Ghi kiểm toán trong cùng giao dịch; ghi thất bại thì hành động thất bại (tiền lệ: `GetAgentEnv` ở `server/internal/handler/agent_env.go`) |
| I8 | Khóa và thu hồi là fail-closed ở lớp guard; lỗi Redis không được làm sập xác thực | Redis lỗi thì đọc thẳng DB; DB lỗi thì trả 503, không cho qua |
| I9 | Tương thích API: client cũ không vỡ khi gặp giá trị mới | Trường mới là tùy chọn; enum mới có fallback; JSON cho UI đi qua schema zod và `parseWithFallback` (AGENTS.md "API Compatibility") |
| I10 | Không mất dữ liệu của tenant vì lý do thương mại | Quá hạn, hạ gói, vượt hạn mức chỉ chặn tạo mới hoặc chuyển chỉ đọc. Xóa dữ liệu chỉ xảy ra theo yêu cầu của chủ workspace hoặc sau thời gian chờ đã thông báo |

### 1.3 Phạm vi theo năm đợt phát hành

| Đợt | Nội dung | Mục |
| --- | --- | --- |
| **R1** Nền tảng an toàn | Guard tài khoản, buộc đăng xuất mọi thiết bị, nhật ký kiểm toán, ghi nhận đồng ý điều khoản, xuất và xóa tài khoản, vá điểm lộ email lời mời | 4 |
| **R2** Thương mại | Dịch vụ platform (khung, kiểm tra schema), staff và console, trạng thái workspace, gói và thuê bao, ghế, cổng hạn mức mới, thanh toán và vòng đời, hạn mức API theo workspace, xuất dữ liệu workspace, số liệu sử dụng | 5 |
| **R3** Doanh nghiệp | MFA, SSO OIDC với xác minh tên miền, chính sách theo workspace, PAT giới hạn, vai trò chỉ xem | 6 |
| **R4** Mở rộng | Project hạn chế truy cập và khách mời, cổng thanh toán tự động, hóa đơn điện tử, thanh toán quốc tế. Mỗi phần cần đặc tả riêng trước khi viết mã | 7 |
| **R5** Bổ sung | Năm nhóm tính năng: quản lý thuê bao cá nhân và doanh nghiệp; nhóm tài khoản và quyền trong workspace doanh nghiệp; gói không giới hạn và gói theo mức dùng có giảm giá cho Business; cấu hình giá từng gói bởi admin nền tảng; admin nền tảng tạo tài khoản cho người dùng. Đặc tả đầy đủ trong tài liệu này, không cần đặc tả riêng | 7b |

R5 là đợt riêng vì ba lý do: nó đổi mô hình giá của D23 và mô hình trả trước của D13 cho một loại gói mới; nó thêm một cơ chế phân quyền mà D21 từng để ngoài phạm vi; và nó phụ thuộc vào thuê bao, đơn hàng, số liệu sử dụng của R2. Nhóm tài khoản (7b.2) còn phụ thuộc vai trò `viewer` của R3, nên R5 chạy sau G-R2, và riêng nhóm 2 chạy sau G-R3.

R1 luôn bật, không phụ thuộc cấu hình thương mại, nên bản tự lưu trữ cũng được hưởng. R2 chỉ có tác dụng khi dịch vụ platform chạy và `MULTICA_CLOUD_URL` trỏ về API của nó. Khi `MULTICA_CLOUD_URL` rỗng hoặc trỏ tới Multica Cloud, Multica không gọi platform; bảng trạng thái vẫn được đọc nhưng không có dòng nghĩa là `active`. Ngoài phạm vi: mục 14.

### 1.4 Quyết định

| # | Nội dung | Trạng thái |
| --- | --- | --- |
| D1 | Phục vụ cả nhóm nhỏ tự đăng ký và doanh nghiệp, trong cùng một sản phẩm; khác nhau ở gói và chính sách theo workspace | Người dùng chốt |
| D2 | Mọi khách hàng chung một hệ thống (nhiều tenant, một cơ sở dữ liệu) | Người dùng chốt |
| D3 | Tái sử dụng tối đa cái đã có. Diễn giải: mã nguồn dịch vụ Multica Cloud không có trong repo, nên **không gọi dịch vụ đó**; thay vào đó cài đặt tại chỗ hai giao diện mà mã lõi đã dùng, `entitlement.Provider` (`server/internal/entitlement/types.go:82`) và `seatcapacity.Executor` (`server/internal/seatcapacity/client.go:61`), và phục vụ đúng hợp đồng JSON của `/api/cloud-subscriptions/*` để giao diện thanh toán hiện có dùng lại được | Người dùng chốt hướng; phần cài đặt trong tiến trình máy chủ **bị thay bởi D35 và D36** (platform phục vụ cùng hợp đồng qua HTTP) |
| D4 | Việt Nam trước, quốc tế sau: tiền tệ và nhà cung cấp thanh toán là dữ liệu, không viết cứng | Người dùng chốt |
| D5 | Quyết định thương mại chạy **trong tiến trình máy chủ** (gói `platform`), không dựng dịch vụ riêng. Lý do: chung cơ sở dữ liệu nên giới hạn kiểm được trong cùng giao dịch, không cần giao thức bù trừ qua mạng; ít thành phần phải vận hành hơn | **Bị thay bởi D31 và D36** |
| D6 | Trạng thái tài khoản và workspace lưu ở **bảng riêng** (`platform_account`, `platform_workspace`), không thêm cột vào `user`, `workspace`. Không có dòng nghĩa là `active`. Lý do: giảm xung đột khi nhập mã từ upstream | Đã chốt |
| D7 | Buộc đăng xuất theo người dùng bằng mốc thời gian `sessions_valid_after`, không lưu bảng phiên. Danh sách thiết bị và thu hồi từng phiên để ngoài phạm vi | Đã chốt |
| D8 | Kiểm toán ghi bằng lời gọi tường minh trong handler, cùng giao dịch, không qua event bus. Lý do: I7 | Đã chốt |
| D9 | Staff là bảng riêng `platform_staff`, tách khỏi vai trò workspace. Email trong `PLATFORM_ADMIN_EMAILS` luôn là `admin`. Ba vai trò: `admin`, `support`, `billing` | Đã chốt |
| D10 | Không có mạo danh. Hỗ trợ khách hàng dựa trên siêu dữ liệu, nhật ký kiểm toán và việc khách mời staff vào workspace như một thành viên thường | Đã chốt |
| D11 | Console chỉ có trên web (`apps/web`, route `/platform`), không có trên desktop và mobile | **Bị thay bởi D31 và D37** (vẫn chỉ trên web; vị trí và cách dựng đổi) |
| D12 | Bốn trạng thái workspace: `active`, `read_only`, `suspended`, `pending_deletion`. Agent ngừng nhận task mới khi workspace không `active` | Đã chốt |
| D13 | Thanh toán đợt đầu là **trả trước theo kỳ**, bằng chuyển khoản do staff xác nhận. Cổng thanh toán tự động là một adapter sau giao diện `PaymentProvider`, làm ở R4 | Đã chốt; **bị thay một phần bởi D44** (gói theo mức dùng trả sau; gói theo ghế vẫn trả trước) |
| D14 | Vòng đời khi hết kỳ: ân hạn 7 ngày đầy đủ chức năng, rồi chỉ đọc 30 ngày, rồi hạ về gói miễn phí, dữ liệu giữ nguyên | Đã chốt (số ngày là lựa chọn, đổi được bằng cấu hình) |
| D15 | Xóa workspace trong chế độ `local` có 14 ngày chờ; chủ workspace hủy được | Đã chốt (số ngày là lựa chọn, đổi được bằng cấu hình); điều kiện "chế độ `local`" **bị thay bởi D36**: áp dụng khi `PLATFORM_WORKSPACE_DELETION_DAYS` trên Multica lớn hơn 0 (mục 5.12) |
| D16 | Xóa tài khoản có 14 ngày chờ, sau đó ẩn danh hóa bản ghi người dùng; nội dung đã viết trong workspace ở lại với workspace | Đã chốt; tư vấn pháp lý xác nhận trước G-LIC (mục 16) |
| D17 | MFA đợt đầu chỉ TOTP kèm mã khôi phục. Staff bắt buộc có MFA ngay khi R3 phát hành | Đã chốt |
| D18 | SSO đợt đầu chỉ OIDC. SAML và SCIM ngoài phạm vi | Đã chốt |
| D19 | Khi workspace bắt buộc SSO, chủ workspace vẫn đăng nhập được bằng mã email để tránh tự khóa | Đã chốt |
| D20 | PAT giới hạn theo hai chiều cưỡng chế tập trung: gắn vào một workspace, và chỉ đọc. Không làm bảng scope theo từng route | Đã chốt |
| D21 | Thêm đúng một vai trò `viewer` (chỉ xem), tính là một ghế. Vai trò kế toán và khách mời ngoài phạm vi đợt này | Đã chốt; vế "vai trò tùy biến ngoài phạm vi" của mục 14 **bị thay một phần bởi D41** (nhóm tài khoản với quyền cố định, không phải vai trò mới; vẫn đúng một vai trò mới là `viewer`) |
| D22 | Project hạn chế truy cập để ở R4 với đặc tả riêng. Lý do ở mục 7.1 | Đã chốt |
| D23 | Định giá **thuần theo ghế**: giá của một thuê bao bằng đơn giá ghế nhân số ghế, không có phí nền. Lý do: hợp đồng giá mà giao diện hiện có đọc chỉ mang một đơn giá theo kỳ (Phụ lục A), và mô hình này dễ giải thích với khách | Đã chốt; **bị thay một phần bởi D43** cho gói theo mức dùng (gói theo ghế vẫn thuần theo ghế, không phí nền) |
| D24 | Danh mục gói khởi tạo theo bảng ở mục 5.5. Staff sửa được trong console mà không cần triển khai lại | Đã chốt (con số là lựa chọn); **cách sửa giá bị thay một phần bởi D46** (giá đi qua lịch thay đổi giá bất biến, không sửa tại chỗ; các thuộc tính khác của gói vẫn sửa tại chỗ) |
| D25 | Điểm dừng agent khi workspace không `active` là cổng nhận task đã có, `claimRefusalFor` (`server/internal/service/budget_gate.go:316`), không phải handler của daemon. Lý do: đây là điểm duy nhất mọi lần nhận task đi qua, và nó đã có sẵn cơ chế "từ chối thì task ở lại hàng đợi" | Đã chốt |
| D26 | Mọi việc nền của hạng mục đăng ký bằng `scheduler.JobSpec` (`server/internal/scheduler/spec.go:102`). Lý do: bộ lập lịch này đã có giành quyền chạy qua cơ sở dữ liệu, nhịp tim và thử lại, nên nhiều tiến trình máy chủ không chạy trùng một việc | Đã chốt cho việc nền chạy ở Multica; việc nền của platform **bị thay một phần bởi D31** (bộ chạy riêng, mục 3.5) |
| D27 | Thư viện: `github.com/pquerna/otp` cho TOTP; `github.com/coreos/go-oidc/v3` cùng `golang.org/x/oauth2` cho OIDC. Phiên bản ghim lúc triển khai, khai báo ở `server/go.mod` | Đã chốt |
| D28 | Chu kỳ của cổng `autopilot_runs` là tháng dương lịch theo UTC, không theo kỳ thanh toán. Lý do: gói miễn phí không có kỳ thanh toán, và một quy tắc cho mọi gói dễ hiểu hơn | Đã chốt |
| D29 | Trang thanh toán hiện có được dùng lại nguyên vẹn; thêm một bộ chọn gói và trang đơn hàng. Máy chủ báo `payment_provider` trong bản tóm tắt để client đổi nhãn và bỏ các bước riêng của Stripe | Đã chốt |
| D30 | Cổng thanh toán tự động ở R4 là adapter **đối soát chuyển khoản qua thông báo giao dịch ngân hàng**, khớp theo `transfer_code`. Lý do: giữ nguyên luồng chuyển khoản mà khách doanh nghiệp Việt Nam quen dùng, chỉ thay bước staff xác nhận bằng máy. Nhà cung cấp đầu tiên và hợp đồng API của họ được chọn và đọc tài liệu chính thức trong đặc tả R4 | Đã chốt hướng |
| D31 | Platform management là **một dịch vụ riêng**, nằm ở `platform-management/` ở gốc repo, là Go module riêng (`github.com/multica-ai/multica/platform-management`). Dịch vụ gồm API Go (`cmd/platform-server`) và ứng dụng một trang cho console (`platform-management/console/`). Build thành image Docker riêng `multica-platform` từ `platform-management/Dockerfile`. Chạy độc lập với máy chủ Multica, web app Multica và landing | Người dùng chốt |
| D32 | Cổng: landing 30002 (có sẵn); web app Multica 30000; console (front end của platform) 30001; API platform (back end) 8095, nghe riêng cho bên gọi nội bộ, kể cả máy chủ Multica; backend Multica 8080 (có sẵn); dịch vụ tri thức giữ 8085 và 8090. Tiến trình Go của platform phục vụ bản build của console trên 30001 và chuyển tiếp `/api/platform/` trên cổng đó sang API platform, nên console và API cùng origin và cookie không băng qua origin khác. Chỉ chuyển tiếp `/api/platform/`, không chuyển cả `/api/`, vì các route nội bộ cho Multica (mục 5.5, 5.6) không có xác thực người dùng và chỉ được mở trên 8095 | Người dùng chốt; chuyển tiếp thu hẹp xuống `/api/platform/` là điều chỉnh khi viết (lý do bảo mật, mục 11) |
| D33 | **Một cơ sở dữ liệu.** Bảng của platform nằm chung Postgres với Multica. Chỉ có một chuỗi migration, là `server/migrations/`, do `server/cmd/migrate` hiện có chạy. Platform không chạy migration; lúc khởi động nó kiểm mọi phiên bản migration nó cần đã có trong `schema_migrations` và từ chối khởi động nếu thiếu một phiên bản | Người dùng chốt |
| D34 | **Console có đăng nhập riêng.** Console không dùng cookie `multica_auth`, vì cookie đó thuộc origin của Multica. Staff đăng nhập bằng mã gửi qua email; platform gửi bằng cùng cấu hình SMTP hoặc Resend mà máy chủ Multica đọc, qua một bộ gửi nhỏ của riêng platform vì nó không nhập được gói `server/internal` của Multica. Chỗ trùng lặp này là mục cần xem lại (15.3). Cookie phiên là `mpa_session`, đặt trên origin của console. Chỉ người có dòng `platform_staff` đang hoạt động mới giữ được phiên | Người dùng chốt |
| D35 | **Cách Multica và platform nói chuyện.** (1) Trạng thái tài khoản và workspace: máy chủ Multica đọc trực tiếp từ cơ sở dữ liệu chung, qua guard tài khoản và cache ở mục 3; không có lời gọi HTTP cho việc này. (2) Quyền lợi (entitlement) và ghế: máy chủ Multica lấy qua HTTP từ API platform bằng các client Cloud hiện có (`entitlement`, `seatcapacity`, và `cloudruntime` cho thanh toán), theo hợp đồng JSON ở Phụ lục A cùng giao thức ghế ở Phụ lục B; Multica không có client mới, chỉ đổi địa chỉ gốc. (3) Xác minh PAT Cloud (`mcn_`) thuộc Multica Cloud, không thuộc platform: khi `MULTICA_CLOUD_URL` trỏ về platform, token `mcn_` bị từ chối. (4) Khi platform không phản hồi: dùng quyết định quyền lợi đã lưu tối đa 15 phút; sau đó hành động tính phí mới bị chặn, hành động đọc vẫn chạy | Người dùng chốt; ý (4) không viết được nguyên văn với client hiện có, xem 5.5 và 15.3(c) |
| D36 | **Một công tắc, không hai.** `PLATFORM_MODE=local` và các bản cài đặt trong tiến trình `platform.Service`, `entitlement.Provider`, `seatcapacity.Executor` bị bỏ khỏi thiết kế. Công tắc duy nhất là `MULTICA_CLOUD_URL`: trỏ tới Multica Cloud, hoặc tới API platform, hoặc rỗng. `PLATFORM_ADMIN_EMAILS` và các biến chỉ của platform đặt trên dịch vụ platform, không đặt trên Multica. Thay D5 | Người dùng chốt |
| D37 | **Console không thuộc `apps/web`.** Console là `platform-management/console/`, dựng bằng `packages/ui` và `packages/core` dùng chung, và được thêm vào `pnpm-workspace.yaml`. Không cần thêm `platform` vào `reserved_slugs.json`, vì console không phải route của web app Multica. Thay D11 | Người dùng chốt |
| D38 | Các trạng thái workspace ở mục 5.3 giữ nguyên. Máy chủ Multica đọc chúng từ `platform_workspace` trong cơ sở dữ liệu chung, qua cache Redis. Cổng nhận task `claimRefusalFor` (D25) giữ nguyên chỗ và hành vi trong Multica | Người dùng chốt |
| D39 | Đơn vị thuê bao vẫn là workspace. "Cá nhân" và "doanh nghiệp" là thuộc tính `account_kind` (`individual`, `business`) của workspace, lưu ở `billing_profile` (không có dòng nghĩa là `individual`). Mỗi gói có `audience` (`individual`, `business`, `any`) giới hạn loại workspace được mua. Không có thuê bao ở cấp người dùng. Lý do: client quyền lợi, client ghế và hợp đồng Phụ lục A đều khóa theo workspace (D35); thuê bao cấp người dùng buộc đổi cả ba | Đã chốt trong đợt bổ sung (7b.1) |
| D40 | Nâng gói áp ngay và tính theo tỷ lệ số ngày còn lại, cùng công thức mua thêm ghế (5.8). Hạ gói, giảm ghế, đổi chu kỳ và chuyển từ gói theo ghế sang gói theo mức dùng lưu thành `pending_change` và áp ở kỳ kế. Không hoàn tiền. Lý do: I10; hoàn tiền tự động đang ngoài phạm vi (mục 14) | Đã chốt trong đợt bổ sung (7b.1) |
| D41 | **Supersedes D21 một phần.** Nhóm tài khoản trong workspace (`member_group`) cấp **thêm** quyền từ một danh mục sáu khóa cố định trong mã (7b.2), cộng lên vai trò nền. Không thêm vai trò, không có quyền "cấm", viewer không bao giờ được ghi nhờ nhóm, và chỉ owner và admin quản lý nhóm nên không ai nâng quyền cho chính mình. Lý do: chủ dự án yêu cầu gom tài khoản và gán quyền cho nhóm; một vai trò tùy biến đầy đủ buộc sửa mọi route (cùng loại rủi ro với 7.1), còn danh mục khóa cố định chỉ chạm các nhóm route owner và admin đã có | Đã chốt trong đợt bổ sung (7b.2); thay một phần D21 |
| D42 | "Không giới hạn" nghĩa là mọi hạn mức đếm của gói để trống (đúng quy ước ở 5.5), không phải vô thời hạn. Riêng `api_rate_per_minute` rỗng được platform trả thành trần chống lạm dụng `PLATFORM_API_RATE_CEILING`; chỉ `limit_overrides` tường minh của staff mới vượt được trần. Lý do: hạn mức tốc độ bảo vệ hệ thống dùng chung, không phải hạn mức thương mại | Đã chốt trong đợt bổ sung (7b.3) |
| D43 | **Supersedes D23 một phần.** Gói có `billing_model = usage` tính tiền theo mức dùng, đo bằng ba đồng hồ lấy từ `platform_usage_daily` (`tasks_run`, `storage_bytes`, `members`); không có đơn giá ghế và không có ghế mua. Gói `seat` vẫn thuần theo ghế, không phí nền. Lý do: hợp đồng giá của trang thanh toán chỉ đọc một đơn giá ghế (Phụ lục A), nên gói theo mức dùng đi qua đường riêng (đơn `usage`, trường tùy chọn `usage` trong `summary`) và `GET /prices` không bao giờ trả giá của gói theo mức dùng | Đã chốt trong đợt bổ sung (7b.3); thay một phần D23 |
| D44 | **Supersedes D13 một phần.** Gói theo mức dùng trả **sau** theo tháng dương lịch UTC (cùng quy ước D28); hóa đơn là đơn `usage` bằng chuyển khoản như mọi đơn; có hạn mức công nợ theo loại tài khoản; hóa đơn chưa trả đi theo vòng đời D14. Gói theo ghế vẫn trả trước. Lý do: số tiền chỉ biết sau khi dùng; phương án số dư trả trước cần sổ cái và việc trừ tiền, gấp đôi phạm vi, trong khi D14 đã xử lý "chưa trả" | Đã chốt trong đợt bổ sung (7b.3); thay một phần D13 |
| D45 | Giảm giá Business là `usage_terms.discount_pct.business`, phần trăm trên giá theo mức dùng, chỉ áp khi `account_kind = business` và, mặc định, hồ sơ đã được staff xác minh. Làm tròn một lần trên từng dòng ngày; dòng giữ ảnh chụp giá và tỷ lệ. Lý do: mã số thuế tự khai không chứng minh được doanh nghiệp, mà ưu đãi giá là chỗ có động cơ gian lận | Đã chốt trong đợt bổ sung (7b.3) |
| D46 | **Supersedes D24 một phần.** Giá của gói (đơn giá ghế, bảng giá theo mức dùng, điều khoản) chỉ đổi qua `plan_price_change` có `effective_from`, không sửa tại chỗ; mỗi gói tối đa một thay đổi chờ; tăng giá cho gói đã có thuê bao trả phí cần báo trước `PLATFORM_PRICE_NOTICE_DAYS` ngày. Giá mới áp cho đơn tạo từ lúc hiệu lực, không đổi đơn đã tạo hay kỳ đã trả. Tên, giới hạn, tính năng vẫn sửa tại chỗ như D24. Giá thương lượng riêng cho một khách là một gói riêng `is_public = false`. Lý do: sửa tại chỗ không để lại lịch sử, không có báo trước, và đổi giá của kỳ gia hạn khách chưa biết | Đã chốt trong đợt bổ sung (7b.4); thay một phần D24 |
| D47 | Tạo tài khoản bởi admin là yêu cầu bất đồng bộ `platform_account_request`: platform ghi yêu cầu; việc nền của Multica tạo `user` và workspace; việc nền của platform tạo hồ sơ thanh toán và thuê bao rồi gửi thư chào. Không mật khẩu, không phiên, staff không là thành viên (giữ D10). Platform không gọi HTTP vào Multica (giữ D35). Lý do: tạo workspace chạm nhiều bảng của Multica (trạng thái issue, tri thức) mà platform không được nhập mã | Đã chốt trong đợt bổ sung (7b.5) |
| D48 | R5 chạy sau G-R2; nhóm 2 chạy sau G-R3 vì phụ thuộc vai trò `viewer`. Mọi giá trị chủ dự án phải đặt (giá, tỷ lệ giảm, hạn mức công nợ, trần) là dữ liệu trong gói hoặc cấu hình, có mặc định, liệt kê ở 7b.7 | Đã chốt trong đợt bổ sung (mục 7b) |
| D49 | **Sai lệch so với P21 (thông tin chuyển khoản).** Ba biến `BILLING_BANK_*` không còn bắt buộc ở mọi môi trường. Khi trống, platform vẫn khởi động và ghi một cảnh báo khi khởi động; phương thức `manual` trả các trường trống. Lý do: chủ dự án chốt rằng giai đoạn này chưa thu tiền thật, nên thông tin ngân hàng chưa cần. Trước khi nhận đơn thật, phải đặt lại các biến này; nếu không khách không biết chuyển tiền đi đâu | Chủ dự án chốt; **điều kiện để mở thu tiền thật**: đặt lại `BILLING_BANK_*` và kiểm tra trang đơn hàng hiển thị đủ |
| D50 | **Đường đăng nhập dev cho staff platform (không phải đăng nhập bằng mật khẩu).** Khi `PLATFORM_DEV_VERIFICATION_CODE` có sáu chữ số, mã đó được chấp nhận ở bước xác minh mã, thêm vào mã email; kiểm tra quyền staff vẫn áp dụng, và mã sai vẫn bị từ chối như trước. Khi có mã này và không có nhà cung cấp email, platform không gửi thư và chỉ ghi log không chứa địa chỉ hay mã. Platform từ chối khởi động khi `APP_ENV=production` và mã này được đặt. Khi có mã dev, nhà cung cấp email không còn bắt buộc. `make dev-full` lấy mã này từ `MULTICA_DEV_VERIFICATION_CODE` và tạo tài khoản `system@admin.com` làm admin. Lý do: môi trường phát triển không có nhà cung cấp email, và chủ dự án muốn đăng nhập bằng cùng mã với Multica. Mục 5.2 vẫn là đường đăng nhập chính; không thêm mật khẩu | Chủ dự án chốt cho môi trường dev; **không dùng cho production**. Commit `90d7eb4f0` và `0f0ae1d87` |

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
| K18 | Thuê bao không phân biệt cá nhân và doanh nghiệp; chủ workspace không tự đổi gói, hạ ghế hay hủy được (bảy route chỉ có thanh toán, ghế và cổng quản lý của Stripe) | `server/cmd/server/router.go:2208` | 7b.1 |
| K19 | Quyền theo workspace chỉ có ba vai trò; mọi thao tác quản trị là của cả nhóm `owner`, `admin`, không giao riêng được từng thao tác cho một nhóm người | `server/cmd/server/router.go:1957`, `server/migrations/001_init.up.sql:30` | 7b.2 |
| K20 | Chỉ có giá theo ghế và hai cổng hạn mức; không có gói không giới hạn rõ ràng, không có tính tiền theo mức dùng | `server/internal/entitlement/types.go:15`, D23 | 7b.3 |
| K21 | Giá của gói là cột sửa tại chỗ, không có lịch hiệu lực, lịch sử, báo trước hay kiểm tra nhập sai | mục 5.5 (`plan.seat_price_month`), D24 | 7b.4 |
| K22 | Người dùng chỉ được tạo khi tự đăng nhập; nhà vận hành không có đường tạo tài khoản và workspace cho khách | `server/internal/handler/auth.go:205`, `:270` | 7b.5 |

## 3. Kiến trúc

### 3.1 Thành phần

Hai dịch vụ chạy độc lập, dùng chung một Postgres và một Redis (D31 đến D33):

```
trình duyệt, daemon ──▶ máy chủ Multica (8080)       web app Multica (30000)
                          │ đọc trực tiếp: platform_account, platform_workspace (cache Redis)
                          │ HTTP: chính sách quyền lợi, ghế, thanh toán   (MULTICA_CLOUD_URL)
                          ▼
                       API platform (8095) ──▶ Postgres chung, Redis chung ◀── máy chủ Multica
                          ▲
nhân viên ──▶ console (30001) ──/api/platform/──┘
```

Mã nằm ở các chỗ sau:

```
platform-management/go.mod                 Go module riêng (D31)
platform-management/audit/                 ghi và đọc nhật ký kiểm toán (R1); thư viện dùng chung với server
platform-management/accountguard/          guard trạng thái tài khoản (R1); thư viện dùng chung với server
platform-management/workspacestate/        đọc trạng thái workspace và khóa cache (R2); thư viện dùng chung với server
platform-management/cmd/platform-server/   tiến trình platform: API 8095, console 30001 (R2)
platform-management/internal/              staff, phiên, bộ gửi email, gói, thuê bao, ghế, thanh toán, vòng đời, API console, kiểm tra schema, việc nền (R2)
platform-management/console/               ứng dụng một trang của console (R2)
platform-management/Dockerfile             image multica-platform (R2)
server/internal/auth/account_guard.go      bộ nối DB và Redis cho guard, phía Multica (R1)
server/internal/workspaceguard/            CheckWorkspaceAccess và đọc trạng thái workspace, phía Multica (R2, R3)
server/internal/sso/, server/internal/mfa/ OIDC, xác minh tên miền; TOTP, mã khôi phục (R3)
packages/views/settings/                   tab nhật ký kiểm toán, bảo mật tài khoản, chính sách, SSO
packages/views/billing/                    trang đơn hàng và hướng dẫn chuyển khoản
```

Máy chủ Multica nhập `audit`, `accountguard` và `workspacestate` qua dòng `replace` đã có ở `server/go.mod:85`. Ba gói này chỉ dùng thư viện chuẩn và nhận cơ sở dữ liệu qua giao diện, nên cả hai dịch vụ dùng được. Platform không nhập được gói `server/internal` của Multica; chỗ phải lặp lại mã ghi ở 15.3. Console dùng `@multica/core` (client API, `parseWithFallback`) và `@multica/ui`, không dùng `@multica/views`.

### 3.2 Đường đi của một request sau khi triển khai

Request vào máy chủ Multica:

```
request
  → Auth / DaemonAuth        xác định người dùng từ token (không đổi)
  → AccountGuard             tài khoản bị khóa? phiên cấp trước mốc thu hồi?          [R1]
                             (đọc platform_account qua cache Redis, rồi DB chung)
  → RequireHumanActor        (chỉ trên nhóm nhạy cảm; không đổi)
  → buildMiddleware          thành viên của workspace? vai trò?  (không đổi)
      → WorkspaceGuard       trạng thái workspace cho phép phương thức này?            [R2]
                             (đọc platform_workspace qua cache Redis, rồi DB chung)
      → PolicyGuard          workspace đòi SSO, MFA, tuổi phiên?                        [R3]
      → ViewerGuard          vai trò chỉ xem gọi phương thức ghi?                       [R3]
      → TokenGuard           PAT bị gắn workspace khác, hoặc chỉ đọc mà gọi ghi?       [R3]
      → WorkspaceRateLimit   vượt hạn mức API của gói?                                  [R2]
  → handler                  quy tắc theo tài nguyên (không đổi) + cổng hạn mức + ghi kiểm toán
                             (cổng hạn mức và ghế: HTTP tới MULTICA_CLOUD_URL, cache trong tiến trình)
```

Bốn guard sau `buildMiddleware` dùng chung một hàm `workspaceguard.CheckWorkspaceAccess(r, member)` trả về mã lỗi hoặc cho qua. Hàm này cũng được gọi trong `requireWorkspaceMember` (`server/internal/handler/handler.go:1048`), vì một số route tự phân giải workspace mà không qua middleware (ví dụ `UploadFile` ở `server/internal/handler/file.go:374`).

Một thay đổi do staff thực hiện:

```
console (30001) → /api/platform/ → API platform (8095)
  → RequirePlatformStaff (mpa_session, dòng platform_staff, tài khoản không bị khóa)
  → giao dịch trên DB chung: ghi bảng platform_* + audit.Record (cùng giao dịch, I7)
  → xóa khóa cache Redis; phát tin lên kênh Redis mul:acct:changed khi đổi tài khoản
  → máy chủ Multica thấy thay đổi ở request kế tiếp (tức thì nhờ xóa cache; tối đa 60 giây nếu xóa cache thất bại)
```

Quyền lợi và ghế của một workspace:

```
handler Multica → client Cloud hiện có → GET /api/v1/internal/entitlement-policies/{ws}      (5.5)
                                       → POST /api/v1/internal/subscriptions/{ws}/capacity/…  (5.6)
                                       → /api/v1/subscriptions/…  (proxyCloudSubscription)    (5.8, Phụ lục A)
                  ⇒ API platform (8095), đọc DB chung
```

### 3.3 Điểm móc vào mã lõi

Đây là toàn bộ chỗ mã hiện có bị sửa. Mỗi điểm là vài dòng gọi sang gói dùng chung (`platform-management/...`) hoặc gói mới của Multica (`server/internal/workspaceguard`, `sso`, `mfa`); logic nằm ở đó. Mã của dịch vụ platform nằm hoàn toàn ngoài danh sách này, vì `platform-management/` là mã mới. Danh sách này là hợp đồng để việc nhập mã upstream sau này biết phải kiểm lại chỗ nào.

| Tệp | Sửa | Đợt |
| --- | --- | --- |
| `server/internal/middleware/auth.go`, `daemon_auth.go` | gọi `AccountGuard` sau khi có `userID` ở mỗi nhánh token | R1 |
| `server/internal/realtime/hub.go` (`HandleWebSocket`, `:798`) | gọi `AccountGuard` khi mở kết nối; nghe kênh Redis `mul:acct:changed` và gọi `Hub.DisconnectUser` | R1 |
| `server/internal/auth/session.go` (`RenewSessionToken`), `server/internal/handler/auth.go` (`issueJWT`, `:166`) | thêm và chép tiếp claim `auth_time` (R1), `amr` (R3) | R1, R3 |
| `server/internal/handler/workspace.go`, `invitation.go`, `share_link.go`, `personal_access_token.go`, `auth.go` | bọc giao dịch và gọi `audit.Record` | R1 |
| `server/cmd/server/router.go` | chuyển route danh sách lời mời sang nhóm admin; đăng ký route mới; nối `AccountGuard` và `WorkspaceGate`. Việc chọn nguồn thương mại vẫn chỉ là `MULTICA_CLOUD_URL` (`:460`), không đổi | R1, R2 |
| `server/internal/middleware/workspace.go` (`buildMiddleware`), `server/internal/handler/handler.go` (`requireWorkspaceMember`) | gọi `workspaceguard.CheckWorkspaceAccess` | R2, R3 |
| `server/internal/service/budget_gate.go` (`claimRefusalFor`, `:316`) | từ chối nhận task khi workspace không `active`, trước mọi phân loại lần chạy | R2 |
| `server/internal/handler/daemon.go` (`requireDaemonWorkspaceAccess`, `:56`) | từ chối vai trò chỉ xem | R3 |
| `server/cmd/server/main.go` | đăng ký các việc nền của Multica (mục 3.5) bằng `schedulerMgr.Register`, cạnh các việc đã có (`:941`) | R1, R2 |
| `server/internal/handler/cloud_billing.go` (7 handler `/api/cloud-subscriptions/*`) | **sửa nhỏ**: `CreateCloudWorkspaceSubscriptionCheckout` (`:197`) dựng lại thân yêu cầu gửi đi nên hiện bỏ mọi trường lạ; thêm chuyển tiếp hai trường tùy chọn `plan_key` và `seats`. Bảy handler vẫn chuyển yêu cầu tới `MULTICA_CLOUD_URL` qua `proxyCloudSubscription` (`:116`), tới đường dẫn `/api/v1/subscriptions/…`. Multica thêm route kế hoạch và đơn hàng của workspace (mục 9), đọc `plan` và `billing_order` từ DB chung | R2 |
| `packages/core/api/schemas.ts` (`StripeHostedURLSchema`, `:3698`; `WorkspaceSubscriptionSummarySchema`, `:3756`), `packages/views/settings/components/billing-tab.tsx` | nhận thêm địa chỉ `http` của máy cục bộ khi phát triển; đọc trường tùy chọn `payment_provider`; bộ chọn gói | R2 |
| `server/internal/entitlement/types.go`, `client.go` (`normalizePolicy`, `:278`) | thêm tên cổng mới vào `GateName` và `valid()`; `normalizePolicy` đọc thêm các cổng mới và coi cổng vắng mặt là `off` (mục 5.7). Đây là sửa bộ phân tích của client hiện có, không phải client mới | R2 |
| `server/internal/handler/agent.go` (`CreateAgent` `:1726`), `daemon.go` (`DaemonRegister` `:558`), `file.go` (`UploadFile`), `workspace.go` (`CreateWorkspace` `:255`, `DeleteWorkspace` `:1176`) | gọi cổng hạn mức; xóa workspace có thời gian chờ | R2 |
| `server/internal/handler/audit_tx.go` (mới), `workspace_deletion.go`, `workspace_export.go`, `github.go`, `vcs.go`, `slack.go`, `lark.go`, `telegram.go`, `wecom_web.go`, `dingtalk.go`, `qoder_connection.go`, `qoder_disconnect.go`, `plugin.go`, `plugin_hook.go`, `workspace_mcp_api.go`, `runtime_profile.go`, `agent.go`, `agent_env.go`, `squad_pack.go`, `runtime.go`, `workspace.go` (`recordAudit`); `server/internal/service/plugin.go`, `plugin_token.go`; `server/internal/integrations/slack/install.go`, `telegram/install.go`, `dingtalk/install.go`, `lark/registration_service.go`, `wecom/installation.go` (và gói mới `integrations/connectaudit`); `server/internal/workspaceexport/dbstore.go`, `job.go` và `server/cmd/server/main.go` (tham số giao dịch của `NewRunner`); `platform-management/internal/lifecycle/expire.go` (job `billing_order_expiry`, đăng ký ở `platform-management/cmd/platform-server/main.go`) | ghi sự kiện kiểm toán của R2 (5.14) trong cùng giao dịch với thay đổi, qua `recordAuditWith` và `auditTx`; các thay đổi chưa có giao dịch được bọc giao dịch. `PluginService` nhận thêm tham số `PluginTxHook` để handler ghi dòng kiểm toán trước khi giao dịch của service commit; thêm hằng tên vào `platform-management/audit/audit.go` và `audit.Catalog`; `server/internal/auditaction/` và bộ lọc của console đọc chung danh sách đó | R2 |
| `server/internal/handler/auth.go` (`VerifyCode`, `GoogleLogin`) | bước MFA; nhánh SSO | R3 |
| `server/internal/auth/pat_cache.go`, `server/internal/handler/personal_access_token.go` | lưu và trả giới hạn của PAT | R3 |
| `server/internal/handler/workspace.go` (`normalizeMemberRole`), `invitation.go`, `share_link.go` | nhận vai trò `viewer` | R3 |
| `packages/core/types/workspace.ts`, `packages/core/permissions/rules.ts`, `packages/core/api/schema.ts` | vai trò mới, fallback enum | R3 |
| `server/internal/handler/workspace.go` (`CreateWorkspace`, `:308`), `server/internal/handler/workspace_revoke.go` (`revokeAndRemoveMember`, `:49`) | tách phần giao dịch của `CreateWorkspace` (tạo workspace, thêm owner, `issuestatus.Ensure`, kiểm toán) thành `workspaceprovision.Create`, handler gọi lại hàm đó với hành vi giữ nguyên; `revokeAndRemoveMember` xóa dòng `member_group_member` của người bị gỡ trong cùng giao dịch | R5 |
| `server/cmd/server/router.go` (nhóm owner và admin, `:1957`; `/api/cloud-subscriptions`, `:2208`), `server/internal/middleware/workspace.go` | tách nhóm route owner và admin theo sáu khóa quyền của 7b.2, thay `RequireWorkspaceRoleFromURL(..., "owner", "admin")` bằng `RequireWorkspacePermissionFromURL` ở đúng các route được ánh xạ; đăng ký route mới của nhóm tài khoản và route quản lý thuê bao dưới `/api/cloud-subscriptions` | R5 |
| `server/internal/handler/invitation.go` (`CreateInvitation`, `:60`; vai trò, `:79`), `share_link.go` (`CreateShareLink`, `:70`) | người chỉ có quyền `members.invite` qua nhóm chỉ mời được vai trò `member` hoặc `viewer` | R5 |
| `server/internal/entitlement/types.go`, `client.go` (`normalizePolicy`, `:278`) | thêm cổng đếm `member_groups` vào `GateName` và `valid()`; cổng vắng mặt vẫn là `off` | R5 |
| `server/internal/handler/cloud_billing.go` | thêm handler mới cho 7b.1 và 7b.3 dùng lại `requireCloudSubscriptionWorkspace` và `proxyCloudSubscription`; không thêm client | R5 |
| `server/cmd/server/main.go` | đăng ký việc nền `account_provisioning` của Multica (mục 3.5) | R5 |
| `packages/core/api/schemas.ts`, `packages/core/permissions/rules.ts`, `packages/views/settings/` | schema và giao diện mới của 7b; `rules.ts` phản chiếu quyền nhóm từ `GET /api/workspaces/{id}/permissions/me` | R5 |

### 3.4 Cấu hình

Công tắc thương mại duy nhất là `MULTICA_CLOUD_URL` (I4, D36). Mỗi dịch vụ đọc biến của riêng nó.

**Máy chủ Multica**

| Biến | Ý nghĩa | Đợt |
| --- | --- | --- |
| `MULTICA_CLOUD_URL` | nguồn thương mại: Multica Cloud, hoặc API platform (ví dụ `http://platform:8095`), hoặc rỗng (mặc định, hành vi như hiện tại) | R2 |
| `LEGAL_TERMS_VERSION`, `LEGAL_PRIVACY_VERSION` | phiên bản điều khoản và chính sách riêng tư đang hiệu lực | R1 |
| `AUDIT_RETENTION_DAYS` | số ngày giữ nhật ký khi gói không quy định; mặc định 365 | R1 |
| `PLATFORM_ACCOUNT_DELETION_DAYS` | thời gian chờ xóa tài khoản; mặc định 14 | R1 |
| `SIGNUP_BLOCKED_EMAIL_DOMAINS` | tên miền email không được đăng ký mới | R2 |
| `PLATFORM_WORKSPACE_DELETION_DAYS` | thời gian chờ xóa workspace; mặc định 0 (xóa ngay như hiện tại, I2); triển khai SaaS đặt 14 (D15) | R2 |
| `FREE_PLAN_OWNED_WORKSPACE_LIMIT` | số workspace tối đa một người làm owner trên gói miễn phí (mục 5.7); rỗng hoặc 0 là không giới hạn | R2 |
| `BILLING_BANK_ACCOUNT_*` | thông tin chuyển khoản hiển thị cho khách ở trang đơn hàng | R2 |
| `PLATFORM_SECRET_KEY` | khóa `secretbox` cho bí mật SSO và MFA; cùng giá trị với biến cùng tên của platform | R3 |

Các biến hiện có của Multica (`DATABASE_URL`, `REDIS_URL`, `JWT_SECRET`, cấu hình email và các biến khác) không đổi.

**Dịch vụ platform**

| Biến | Ý nghĩa | Đợt |
| --- | --- | --- |
| `PLATFORM_API_PORT` | cổng API cho bên gọi nội bộ; mặc định 8095 | R2 |
| `PLATFORM_CONSOLE_PORT` | cổng phục vụ console và chuyển tiếp `/api/platform/`; mặc định 30001 | R2 |
| `DATABASE_URL` | cùng cơ sở dữ liệu với Multica (D33) | R2 |
| `REDIS_URL` | cùng Redis với Multica; để xóa cache guard và giới hạn đăng nhập của console | R2 |
| `RESEND_API_KEY`, `RESEND_FROM_EMAIL`, `SMTP_*` | cấu hình email, cùng tên và ý nghĩa với biến Multica đọc (`server/internal/service/email.go:115`, `:178`) | R2 |
| `PLATFORM_ADMIN_EMAILS` | danh sách email luôn là staff `admin` | R2 |
| `PLATFORM_ADMIN_CIDRS` | tùy chọn: chỉ nhận request console từ các dải IP này | R2 |
| `PLATFORM_SESSION_SECRET` | khóa ký cookie `mpa_session`; ít nhất 32 byte | R2 |
| `PLATFORM_APP_URL` | origin của web app Multica, để dựng địa chỉ trang đơn hàng trả cho `checkout-sessions` | R2 |
| `BILLING_VAT_RATE` | thuế suất của đơn hàng | R2 |
| `PLATFORM_SECRET_KEY` | như bảng trên; platform dùng để giải mã bí mật MFA khi xác minh TOTP của staff | R3 |
| `PLATFORM_INDIVIDUAL_MAX_SEATS` | số ghế tối đa của workspace `individual`; mặc định 5 (7b.1) | R5 |
| `PLATFORM_USAGE_DUE_DAYS`, `PLATFORM_USAGE_MIN_INVOICE` | hạn trả hóa đơn theo mức dùng, mặc định 7 ngày; số tiền tối thiểu để lập hóa đơn, mặc định 10.000 (đơn vị nhỏ nhất của tiền tệ), dưới mức đó dồn sang kỳ sau (7b.3) | R5 |
| `PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION` | mặc định `true`: giảm giá Business chỉ áp cho hồ sơ đã được staff xác minh (D45) | R5 |
| `PLATFORM_API_RATE_CEILING` | trần `api_rate_per_minute` khi gói để trống; mặc định 6.000 (D42) | R5 |
| `PLATFORM_PRICE_NOTICE_DAYS`, `PLATFORM_PRICE_CONFIRM_PCT`, `PLATFORM_PRICE_MAX` | báo trước khi tăng giá, mặc định 30 ngày; mức đổi giá phải xác nhận lại, mặc định 50 phần trăm; giá tối đa một đơn vị, mặc định 1.000.000.000 (7b.4) | R5 |
| `PLATFORM_ACCOUNT_CREATE_PER_HOUR` | số yêu cầu tạo tài khoản tối đa mỗi giờ cho mỗi staff; mặc định 30 (7b.5) | R5 |

Các số ngày ở D14 và hạn đơn hàng đọc từ biến của platform `PLATFORM_GRACE_DAYS` (7), `PLATFORM_READ_ONLY_DAYS` (30), `PLATFORM_ORDER_EXPIRY_DAYS` (7); giá trị trong ngoặc là mặc định. Các số ngày ở D15 và D16 đọc ở Multica (bảng trên), vì việc xóa chạy ở Multica (3.5).

Kiểm tra khi khởi động của platform nằm ở mục 5.1. Máy chủ Multica không có kiểm tra khởi động mới.

### 3.5 Việc nền

Việc nền của Multica là `scheduler.JobSpec` đăng ký ở `server/cmd/server/main.go` (D26). Platform không nhập được `server/internal/scheduler`, nên việc nền của platform chạy trong tiến trình platform bằng một bộ chạy nhỏ `platform-management/internal/jobs`: mỗi việc giành quyền chạy bằng `pg_try_advisory_lock` theo tên việc, nên nhiều bản platform không chạy trùng một việc.

Quy tắc chia: việc chạm bảng riêng của platform chạy ở platform; việc chạm dữ liệu workspace chạy ở Multica và đọc trạng thái của platform từ cơ sở dữ liệu chung. Mỗi việc lũy đẳng, có giới hạn số dòng mỗi lần chạy, và có tên cố định vì tên là khóa của lịch sử chạy.

| Tên | Chạy ở | Nhịp | Việc | Đợt |
| --- | --- | --- | --- | --- |
| `audit_retention` | Multica | ngày | xóa dòng kiểm toán quá hạn lưu | R1 |
| `account_deletion` | Multica | giờ | thực hiện xóa tài khoản đã hết thời gian chờ | R1 |
| `subscription_lifecycle` | platform | giờ | chuyển trạng thái thuê bao và `platform_workspace`; gửi nhắc | R2 |
| `order_expiry` | platform | giờ | hết hạn đơn `pending` | R2 |
| `workspace_deletion` | Multica | giờ | xóa workspace đã hết thời gian chờ | R2 |
| `workspace_export` | Multica | phút | chạy việc xuất đang chờ | R2 |
| `platform_usage_daily` | Multica | ngày | chụp số liệu sử dụng vào bảng `platform_usage_daily` | R2 |
| `usage_rating` | platform | giờ | chấm tiền các ngày đã đóng của gói theo mức dùng vào `usage_charge`; phát hóa đơn tạm khi vượt hạn mức công nợ (7b.3) | R5 |
| `usage_invoicing` | platform | giờ | lập hóa đơn tháng và hóa đơn cuối cho gói theo mức dùng (7b.3) | R5 |
| `plan_price_apply` | platform | giờ | áp các thay đổi giá đã đến `effective_from`; gửi báo trước tăng giá (7b.4) | R5 |
| `account_provisioning` | Multica | phút | tạo `user` và workspace cho yêu cầu tạo tài khoản `pending` (7b.5) | R5 |
| `account_request_finalize` | platform | phút | tạo hồ sơ thanh toán, thuê bao và gửi thư chào cho yêu cầu `provisioned` (7b.5) | R5 |

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

Áp cho cả năm loại token. Với `mat_`, người dùng bị kiểm là người mà token gắn vào. Với `mdt_`, là chủ runtime mà token gắn vào (`DaemonTokenOwnerIDFromContext`); token không có chủ (daemon do Qoder quản lý) không qua guard tài khoản. Kết nối WebSocket kiểm lúc mở. Khi một tài khoản bị khóa, máy chủ ngắt mọi kết nối đang mở của người đó bằng `Hub.DisconnectUser`, một lần gọi cho mọi workspace. Khi platform là bên khóa (console, vòng đời), nó xóa khóa cache và phát tin lên kênh Redis `mul:acct:changed`; mỗi tiến trình Multica nghe kênh này và gọi `Hub.DisconnectUser`. Tin có thể mất; khi đó kết nối đang mở sống tới lần mở lại kế tiếp, còn mọi request HTTP của người đó đã bị chặn.

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

### 5.1 Dịch vụ platform và kiểm tra khi khởi động

Dịch vụ platform (`platform-management/cmd/platform-server`) mở hai listener: API trên `PLATFORM_API_PORT` (8095) và console trên `PLATFORM_CONSOLE_PORT` (30001), nơi nó phục vụ bản build của console và chuyển tiếp `/api/platform/` sang API (D32). Platform không chạy migration (D33).

Lúc khởi động platform từ chối chạy, thoát với mã khác 0 và in điều kiện sai, nếu:

- thiếu `DATABASE_URL`, hoặc bảng `schema_migrations` thiếu một phiên bản trong danh sách bắt buộc của `platform-management/internal/schemacheck`. Danh sách gồm mọi phiên bản tạo hoặc đổi bảng mà platform đọc hay ghi. Thông báo liệt kê phiên bản thiếu và nhắc chạy `server/cmd/migrate` (D33);
- thiếu `REDIS_URL` (K11: cache guard và hạn mức đăng nhập của console cần Redis);
- `PLATFORM_ADMIN_EMAILS` rỗng;
- `PLATFORM_SESSION_SECRET` ngắn hơn 32 byte;
- không có cấu hình email (Resend hoặc SMTP), vì không có email thì không ai đăng nhập được console.

Platform không có đường mã đăng nhập phát triển (K12): mã gửi qua email là cách duy nhất, và một test khẳng định điều đó. Hai kiểm tra từng nằm ở khởi động của Multica khi bật chế độ thương mại (thiếu Redis; biến mã đăng nhập phát triển của Multica, `server/internal/handler/auth.go:137`) không còn kiểm được ở Multica, vì Multica không biết nó chạy cùng platform. Chúng chuyển vào bảng kiểm triển khai ở P30.

Route sức khỏe: `GET /healthz` trên API (kèm kiểm DB và Redis) và trên cổng console.

### 5.2 Staff (K6)

`platform_staff`: `user_id`, `role`, `created_by`, `created_at`, `revoked_at`. Email thuộc `PLATFORM_ADMIN_EMAILS` được platform gieo thành một dòng `admin` đang hoạt động khi người đó đã có tài khoản Multica (lúc khởi động và lúc đăng nhập); dòng bị thu hồi của email này được mở lại ở lần gieo kế tiếp. Đó là cách khởi tạo và là lối vào không thể tự khóa. Người trong danh sách phải đã đăng ký một lần ở Multica.

**Đăng nhập (D34).** Console không dùng cookie `multica_auth` của Multica.

1. `POST /api/platform/auth/send-code` nhận email, luôn trả 204. Platform chỉ gửi mã khi email thuộc người có dòng `platform_staff` đang hoạt động, tài khoản Multica của họ không bị khóa (qua `accountguard`) và chưa vượt hạn mức theo IP và theo email (Redis).
2. Mã gồm 6 chữ số, sống 10 phút, lưu dạng băm trong Redis, tối đa 5 lần thử sai. Bộ gửi email nằm trong platform (`platform-management/internal/mailer`), đọc cấu hình giống Multica; chỗ trùng lặp này ghi ở 15.3.
3. `POST /api/platform/auth/verify-code` với mã đúng đặt cookie `mpa_session` (`HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/`, không có thuộc tính `Domain`, nên chỉ thuộc origin của console). Giá trị cookie là token có chữ ký bằng `PLATFORM_SESSION_SECRET`, mang `user_id`, `auth_time`, `amr` và hạn 12 giờ. Phiên không lưu ở bảng riêng; mọi request đều kiểm lại dòng `platform_staff` nên thu hồi staff có hiệu lực ngay. Mã sai trả 400, không phân biệt lý do.
4. `POST /api/platform/auth/logout` xóa cookie. Mốc `sessions_valid_after` của người dùng (4.2) cũng làm phiên console mất hiệu lực. Request ghi phải có header `Origin` trùng origin của console.

Middleware `RequirePlatformStaff(roles...)` của platform đòi tất cả điều sau: cookie `mpa_session` hợp lệ; dòng `platform_staff` đang hoạt động và đúng vai trò; tài khoản Multica của người đó qua `accountguard` (không bị khóa, phiên chưa bị thu hồi); `auth_time` trong vòng 12 giờ; IP thuộc `PLATFORM_ADMIN_CIDRS` nếu có cấu hình; và từ R3 trở đi, `amr` có `mfa` (6.2). Platform không đọc PAT, token máy hay cookie `multica_auth`. Không đạt thì trả **404**, để sự tồn tại của console không bị dò; trang đăng nhập của console hiện khi `GET /api/platform/me` trả 404.

| Vai trò | Quyền |
| --- | --- |
| `support` | xem tenant và người dùng; khóa và mở tài khoản, workspace; buộc đăng xuất; ghi chú |
| `billing` | quyền xem của `support`; đơn hàng, xác nhận thanh toán, đổi gói, gia hạn |
| `admin` | tất cả; quản lý staff; sửa danh mục gói; lên lịch và hủy xóa workspace |

Mọi hành động của staff ghi kiểm toán với `actor_type = staff`. Hành động trên một workspace ghi kèm `workspace_id`, nên **chủ workspace thấy được** trong nhật ký của mình rằng nhà vận hành đã làm gì.

### 5.3 Trạng thái workspace (K7)

`platform_workspace`: `workspace_id`, `status`, `reason`, `changed_by`, `changed_at`, `deletion_scheduled_at`. Không có dòng nghĩa là `active`. Cache như guard tài khoản, khóa `mul:plat:ws:<id>` (D38). Platform ghi bảng này (staff, vòng đời thuê bao) và xóa khóa cache; máy chủ Multica đọc qua `platform-management/workspacestate`. Máy chủ Multica chỉ ghi bảng này ở hai thao tác của chủ workspace, đặt `pending_deletion` và hủy xóa (5.12), và cũng xóa khóa cache.

| Trạng thái | Đọc | Ghi | Agent nhận task mới | Vào bằng cách nào |
| --- | --- | --- | --- | --- |
| `active` | có | có | có | mặc định |
| `read_only` | có | không, trừ danh sách cho phép | không | quá hạn sau ân hạn; staff |
| `suspended` | không, trừ danh sách cho phép | không | không | staff (vi phạm, yêu cầu pháp lý) |
| `pending_deletion` | không, trừ danh sách cho phép | không | không | chủ workspace xóa; staff |

Danh sách cho phép khi `read_only` (để khách tự gỡ rối được): mọi route `/api/cloud-subscriptions/*`, gỡ thành viên, rời workspace, thu hồi lời mời và link chia sẻ, xuất dữ liệu, xóa workspace, xóa và lưu trữ tài nguyên. Danh sách cho phép khi `suspended` và `pending_deletion`: đọc bản ghi workspace, `/api/cloud-subscriptions/*` cho owner và admin, rời workspace, xuất dữ liệu cho owner, hủy xóa cho owner.

Mã lỗi: `workspace_read_only`, `workspace_suspended`, `workspace_pending_deletion` (403).

**Dừng agent** (D25). Mọi lần nhận task đi qua `TaskService.ClaimTask` (`server/internal/service/task.go:3639`), và trong giao dịch nhận, qua cổng `claimRefusalFor`. Cổng này hôm nay cho qua vô điều kiện các lần chạy do người kích hoạt; kiểm tra trạng thái workspace được đặt **trước** bước phân loại đó, nên áp cho mọi loại lần chạy. Bị từ chối thì giao dịch nhận hủy và task ở lại hàng đợi, đúng cơ chế đã dùng cho ngân sách và giờ yên lặng. `TaskService` nhận trạng thái qua một giao diện một phương thức để gói `service` không phụ thuộc gói `platform`.

Autopilot ngừng sinh lần chạy mới qua cổng `autopilot_runs`: platform trả giới hạn 0 trong chính sách quyền lợi khi workspace không `active` (giới hạn 0 hợp lệ theo `quotaPolicy` ở `server/internal/service/autopilot_quota.go:80`). Client cache chính sách tối đa 5 phút (`maxPolicyTTL`, `server/internal/entitlement/client.go:24`), nên autopilot dừng trễ tới 5 phút; việc dừng nhận task là tức thì vì `claimRefusalFor` đọc trạng thái qua cache guard. Task đang chạy được để chạy xong; kết quả vẫn ghi được vì daemon ghi qua đường riêng không qua `buildMiddleware`.

### 5.4 Console (K6)

**API** của platform dưới `/api/platform/*`. Bốn route đăng nhập (5.2) không đòi phiên; mọi route còn lại đứng sau `RequirePlatformStaff`:

| Route | Vai trò | Nội dung |
| --- | --- | --- |
| `POST /auth/send-code`, `POST /auth/verify-code`, `POST /auth/logout`, `GET /me` | không đòi phiên; `GET /me` trả vai trò hoặc 404 | đăng nhập bằng mã email (5.2); `GET /me` cho giao diện biết hiện màn hình nào |
| `GET /tenants`, `GET /tenants/{id}` | mọi staff | tìm theo tên, slug, email owner; gói, trạng thái, số thành viên, ngày tạo, hoạt động gần nhất; chi tiết có thuê bao, số đếm sử dụng, đơn hàng, ghi chú |
| `POST /tenants/{id}/status` | `support`, `admin` | đổi trạng thái kèm lý do bắt buộc |
| `GET /tenants/{id}/audit-events` | mọi staff | nhật ký của workspace |
| `GET/POST /tenants/{id}/notes` | mọi staff | ghi chú nội bộ |
| `PUT /tenants/{id}/subscription` | `billing`, `admin` | đổi gói, số ghế, kỳ, gia hạn dùng thử, ghi đè giới hạn |
| `GET /orders`, `POST /orders/{id}/confirm`, `POST /orders/{id}/cancel` | `billing`, `admin` | đơn hàng và xác nhận thanh toán |
| `GET /users`, `GET /users/{id}` | mọi staff | tìm theo email; tư cách thành viên, trạng thái |
| `POST /users/{id}/status`, `POST /users/{id}/revoke-sessions`, `POST /users/{id}/mfa-reset` | `support`, `admin` | khóa, mở, buộc đăng xuất, đặt lại MFA (R3) |
| `GET/POST/DELETE /staff` | `admin` | quản lý staff |
| `GET/PUT /plans` | `admin` | danh mục gói; từ R5 giá không sửa tại chỗ mà đổi qua lịch thay đổi giá (7b.4, D46) |
| `GET /audit-events` | mọi staff | nhật ký toàn hệ thống |
| `GET /overview` | mọi staff | số tenant theo gói và trạng thái, đăng ký mới, đơn chờ xác nhận |

Console **không** có route trả issue, comment, chat, tệp đính kèm, cấu hình agent hay bí mật (I6). Một test duyệt các handler của nhóm `/api/platform` và thất bại nếu có handler gọi các query của những bảng đó.

**Giao diện** là ứng dụng một trang ở `platform-management/console/`, dựng bằng `@multica/ui` và `@multica/core` dùng chung, thêm vào `pnpm-workspace.yaml` (D37). Platform phục vụ bản build trên cổng 30001 và chuyển tiếp `/api/platform/` sang API, nên console và API cùng origin và cookie `mpa_session` không băng qua origin nào khác (D32). Console không phải route của `apps/web`, nên không đụng tới danh sách slug dành riêng của web app. Console chỉ có trên web, không có trên desktop và mobile. `GET /api/me` của Multica không có trường vai trò staff; console tự hỏi `GET /api/platform/me`.

### 5.5 Gói và thuê bao (K8)

**`plan`**: `key`, `name`, `limits` (JSONB), `features` (JSONB), `currency`, `seat_price_month`, `seat_price_year`, `seats_fixed` (số ghế cố định của gói không bán ghế; rỗng với gói bán theo ghế), `min_seats`, `trial_days`, `is_public`, `sort`, `archived_at`. Giá là số nguyên theo đơn vị nhỏ nhất của tiền tệ; VND không có phần thập phân nên giá ghi thẳng bằng đồng. Danh mục khởi tạo từ một tệp nhúng khi bảng rỗng; sau đó staff `admin` sửa trong console. Gói đã có thuê bao thì chỉ lưu trữ được, không xóa được.

**`workspace_subscription`**: `workspace_id`, `plan_key`, `status`, `seats`, `interval`, `current_period_start`, `current_period_end`, `trial_end`, `grace_until`, `cancel_at_period_end`, `limit_overrides` (JSONB), `version`, `updated_at`. Workspace không có dòng được coi là gói miễn phí. `version` tăng một ở mỗi lần đổi và là `subscription_version` mà platform trả trong chính sách quyền lợi.

**Giới hạn hiệu lực** = `limits` của gói, ghi đè bởi `limit_overrides`. Giá trị rỗng nghĩa là không giới hạn.

**Chính sách quyền lợi qua HTTP.** Platform phục vụ `GET /api/v1/internal/entitlement-policies/{workspaceId}` trên API (8095), đúng đường dẫn mà client hiện có gọi (`server/internal/entitlement/client.go:241`). Máy chủ Multica không có client mới: khi `MULTICA_CLOUD_URL` trỏ về platform, client hiện có gọi nó và cache của client giữ chính sách theo `valid_for_seconds`. Platform đọc thuê bao, gói và `platform_workspace`, cache trong tiến trình 30 giây theo workspace và xóa cache ngay trên tiến trình thực hiện thay đổi. Với nhiều bản platform, độ trễ tối đa giữa các bản là 30 giây; chấp nhận được vì đổi gói không phải thao tác khẩn. Hai cổng đang có (`issue_count`, `autopilot_runs`) được cưỡng chế bởi mã lõi hiện tại mà không sửa gì.

Phụ lục A là hợp đồng của giao diện thanh toán (`/api/cloud-subscriptions/*`), không phải của endpoint chính sách này. Hợp đồng chính sách là những gì client phân tích (`wirePolicy`, `server/internal/entitlement/client.go:212`; `normalizePolicy`, `:278`). Phản hồi phải đúng dạng đó, nếu không client coi là chính sách hỏng và **cho qua**:

- `schema_version = 1`; `policy_revision` dương (hằng số của bản cài đặt); `subscription_version` là `version` của thuê bao (0 khi chưa có dòng); `valid_for_seconds` từ 1 đến 300; `valid_until` khác rỗng (chỉ để chẩn đoán).
- Cổng có giới hạn: `action = enforce`, `limit` khác rỗng và không âm. Cổng không giới hạn: `action = off`.
- Riêng `autopilot_runs` còn phải có `period_start`, `period_end`, `reset_at`, với `period_start` trước `period_end` và trước `reset_at`. Chu kỳ là tháng dương lịch theo UTC (D28); `reset_at` bằng `period_end`.

**Khi platform không phản hồi** (D35 ý 4). Client giữ chính sách đã nhận: tươi trong `valid_for_seconds` (tối đa 5 phút), rồi thêm cửa sổ chịu lỗi 15 phút (`defaultStaleGrace`, `server/internal/entitlement/client.go:22`) trong đó `enforce` hạ thành `observe`, nghĩa là không chặn. Hết cửa sổ, kết quả là `off`. D35 đòi chặn hành động tính phí mới sau 15 phút; client hiện có không làm vậy, nó mở cho qua (README của gói `server/internal/entitlement`). Đổi nó thành đóng nằm ngoài D35, vì sẽ đổi cả hành vi với Multica Cloud. Nên thiết kế này ghi đúng hành vi hiện có: đọc luôn chạy; cổng hạn mức không chặn khi platform ngừng; lời mời và link chia sẻ bị chặn với `seat_capacity_unavailable` (503), vì lỗi truyền tải của client ghế rơi vào nhánh mặc định của `writeSeatCapacityError` (`server/internal/handler/seat_capacity.go:352`); trạng thái tài khoản và workspace vẫn được cưỡng chế từ DB chung. Rủi ro ở 15.3(c).

**Token `mcn_`.** Xác minh PAT Cloud (`mcn_`) thuộc Multica Cloud (`server/internal/middleware/auth.go:257`, `daemon_auth.go:243`). Platform không phục vụ route xác minh đó, nên khi `MULTICA_CLOUD_URL` trỏ về platform, mọi token `mcn_` bị từ chối. Các route `/api/cloud-billing/*` và `/api/cloud-runtime/*` (tín dụng trả trước, runtime đám mây) cũng thuộc Multica Cloud và platform không phục vụ (mục 14).

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

Platform phục vụ giao thức ghế qua HTTP trên API (8095), tại `/api/v1/internal/subscriptions/{workspaceId}/capacity/…`, đúng đường dẫn mà client ghế hiện có gọi (`server/internal/seatcapacity/client.go:191`), trên bảng `platform_seat_operation` (`workspace_id`, `operation_token`, `kind`, `subject_id`, `state`, `member_id`, `expires_at`). Máy chủ Multica dùng client ghế hiện có; chỉ địa chỉ gốc đổi (D35, D36). Sức chứa là `seats` của thuê bao, hoặc `seats_fixed` của gói. Số đã dùng là số dòng `member` của workspace, platform đọc trực tiếp từ DB chung; số đang giữ là số thao tác còn sống chưa thành thành viên (lời mời đang giữ chỗ, lượt vào bằng link đang xử lý). Mọi thao tác lũy đẳng theo `operation_token`. Workspace không giới hạn ghế trả `Managed = false`, khi đó handler đi theo đường không quản lý ghế như hiện tại. Ngữ nghĩa từng phương thức, chuỗi lý do và kiểu lỗi mà handler dựa vào: Phụ lục B.

Vì bốn handler lời mời và link chia sẻ đã gọi client ghế đúng chỗ (`server/internal/handler/invitation.go:59`, `:443`, `server/internal/handler/share_link.go:264`), không handler nào phải sửa. Tính đúng của platform được bảo đảm bằng một bộ test hợp đồng chạy cùng một kịch bản trên bản giả đang dùng trong test hiện có và trên handler HTTP của platform (mục 13).

Khi số ghế mua ít hơn số thành viên (hạ gói), workspace ở trạng thái vượt chỗ: không thêm được người, không ai bị gỡ tự động (I10).

### 5.7 Cổng hạn mức mới (K9)

Thêm vào `GateName`: `agents`, `runtimes`, `storage_bytes` (cổng đếm tài nguyên) và `api_rate_per_minute`, `audit_retention_days`, `sso`, `workspace_policies`, `viewer_role`, `audit_export` (cổng mang giá trị của gói, xem cuối mục). Mỗi cổng đếm được kiểm trong giao dịch tạo tài nguyên, sau khi lấy một khóa tư vấn theo cặp (workspace, cổng), nên hai request đồng thời không cùng vượt giới hạn.

| Cổng | Điểm cưỡng chế | Cách đếm |
| --- | --- | --- |
| `agents` | `CreateAgent`, khôi phục agent đã lưu trữ | agent chưa lưu trữ, `kind = user` |
| `runtimes` | `DaemonRegister` khi tạo runtime mới | runtime của workspace |
| `storage_bytes` | `UploadFile` và các đường tạo tệp đính kèm | tổng kích thước tệp đính kèm |

Vượt giới hạn trả 402 với mã `plan_limit_reached` và trường `gate`, `limit`, `used`; client mở hộp thoại nâng cấp như đã làm với giới hạn issue.

Hai giới hạn cấp tài khoản, không qua chính sách quyền lợi: số workspace một người được làm owner trên gói miễn phí, kiểm trong `CreateWorkspace` cạnh hai kiểm tra đã có (`server/internal/handler/workspace.go:266`); và `SIGNUP_BLOCKED_EMAIL_DOMAINS`, kiểm trong `checkSignupAllowed`. Giới hạn thứ nhất lấy từ `FREE_PLAN_OWNED_WORKSPACE_LIMIT` của Multica (rỗng hoặc 0 là không giới hạn), và "gói miễn phí" của một workspace được đọc từ `workspace_subscription` trong DB chung (không có dòng, hoặc `plan_key = free`).

Giá trị theo gói (`features`) đến Multica qua cùng chính sách quyền lợi, dưới dạng cổng cùng tên: cờ bật là `action = off`, cờ tắt là `enforce` với `limit` 0; `api_rate_per_minute` và `audit_retention_days` là `enforce` với `limit` là giá trị của gói. Cổng vắng mặt trong phản hồi nghĩa là `off`, nên chính sách của Multica Cloud hay của một bản platform cũ vẫn qua `normalizePolicy` (mục 3.3). Khi `MULTICA_CLOUD_URL` rỗng mọi cổng là `off`, nên bản tự lưu trữ không bị khóa tính năng. Tính năng R3 kiểm cổng này trước khi cho cấu hình.

### 5.8 Thanh toán (K10)

**Mô hình.** Trả trước theo kỳ tháng hoặc năm. Số tiền một kỳ bằng đơn giá ghế nhân số ghế (D23), cộng thuế theo `BILLING_VAT_RATE`. Mỗi lần mua là một **đơn hàng**. Thanh toán thành công áp đơn vào thuê bao trong cùng giao dịch.

**`billing_order`**: `id`, `workspace_id`, `kind` (`subscribe`, `renew`, `add_seats`, `change_plan`), `plan_key`, `interval`, `seats`, `amount`, `vat_amount`, `currency`, `status` (`pending`, `paid`, `expired`, `canceled`, `refunded`), `provider`, `provider_ref`, `transfer_code`, `idempotency_key`, `buyer` (JSONB: tên đơn vị, mã số thuế, địa chỉ, email nhận hóa đơn), `period_start`, `period_end`, `einvoice_ref`, `created_by`, `confirmed_by`, `paid_at`, `expires_at`.

**`PaymentProvider`** có ba phương thức: tạo yêu cầu thanh toán cho một đơn, xác thực và diễn giải thông báo từ nhà cung cấp, truy vấn trạng thái. Đợt này có một cài đặt, `manual`: trả về thông tin chuyển khoản và `transfer_code` duy nhất để khách ghi vào nội dung chuyển tiền; staff `billing` đối soát và bấm xác nhận trong console. Adapter cho cổng tự động ở R4 chỉ thay bước xác nhận; mô hình đơn hàng không đổi.

**Áp thanh toán** (`ApplyPayment`) là một giao dịch: khóa dòng đơn, kiểm đơn còn `pending`, đặt `paid`, cập nhật thuê bao và tăng `version`, đưa workspace về `active` nếu đang `read_only` vì quá hạn, ghi kiểm toán. Gọi lại trên đơn đã `paid` là không làm gì. Đơn `pending` hết hạn sau 7 ngày.

**Mua thêm ghế giữa kỳ** tính theo tỷ lệ số ngày còn lại của kỳ, làm tròn theo đơn vị nhỏ nhất của tiền tệ. VND không có phần thập phân.

**Tái sử dụng giao diện** (D29). Bảy handler `/api/cloud-subscriptions/*` (`server/cmd/server/router.go:2185`) chuyển yêu cầu tới `MULTICA_CLOUD_URL` bằng client hiện có (`proxyCloudSubscription`, `server/internal/handler/cloud_billing.go:116`), tới đường dẫn `/api/v1/subscriptions/…`, kèm header `X-User-ID` và `X-Request-ID` và không có xác thực nào khác. Khi URL trỏ về platform, platform phục vụ các đường dẫn `/api/v1/subscriptions/…` đó trên API (8095) và trả **đúng hợp đồng JSON hiện có**, ghi đầy đủ ở Phụ lục A; handler của Multica chuyển nguyên phản hồi cho trình duyệt. Tóm tắt cách ánh xạ:

| Route | Platform làm |
| --- | --- |
| `GET summary` | dựng từ thuê bao, gói và sổ ghế. `has_stripe_customer = false`; `available_actions.portal = false`; thêm trường tùy chọn `payment_provider = "manual"` |
| `GET prices` | đơn giá ghế theo tháng và năm của gói đang xét: gói hiện tại nếu trả phí, gói trả phí công khai rẻ nhất nếu đang miễn phí; nhận `?plan=<key>` |
| `POST checkout-sessions` | tạo đơn `subscribe` hoặc `change_plan`; trả `url` là địa chỉ tuyệt đối của trang đơn hàng, dựng từ `PLATFORM_APP_URL`. Nhận thêm trường tùy chọn `plan_key` và `seats` |
| `POST seats/purchase-preview` | tính tiền mua thêm ghế theo tỷ lệ; `purchase_version` là `version` của thuê bao |
| `POST seats/purchases` | tạo đơn `add_seats` sau khi kiểm `expected_current_seats`, `expected_purchase_version`, `accepted_proration_amount`; trả `status = "pending"` |
| `POST seats/reconcile` | không có gì để đối soát; trả `action = "noop"` |
| `POST portal-sessions` | 404; client không gọi vì `portal = false` |

Cờ `billing_workspace_subscriptions` phải bật. `GET /api/workspaces/{id}/plans` và các route đơn hàng của workspace là route của Multica, đọc `plan` và `billing_order` từ DB chung. Riêng `PUT .../buyer` ghi cột `buyer` của đơn đang `pending`; đó là chỗ duy nhất Multica ghi vào bảng của platform ngoài `platform_workspace`. Client thêm ba thứ: bộ chọn gói đọc `GET /api/workspaces/{id}/plans`; trang đơn hàng hiển thị số tiền, thông tin chuyển khoản, `transfer_code`, trạng thái và biểu mẫu thông tin xuất hóa đơn; và nhánh theo `payment_provider` để đổi nhãn nút và bỏ bước xử lý tham số quay về của Stripe. Trường `url` hiện bị schema buộc là `https` (`packages/core/api/schemas.ts:3698`); schema được nới để nhận thêm `http` chỉ khi máy chủ là `localhost` hoặc `127.0.0.1`, cho việc phát triển. Client đã định dạng đúng tiền tệ không có phần thập phân: `VND` nằm trong danh sách ở `packages/views/settings/components/billing-format.ts:12`.

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

Việc nền `subscription_lifecycle` (mục 3.5) chạy mỗi giờ trong platform, thực hiện các chuyển trạng thái và ghi `platform_workspace`; Multica thấy kết quả qua DB chung và cache. Mỗi chuyển là một giao dịch có điều kiện trên trạng thái hiện tại và `version`, nên chạy lại hay chạy song song không gây chuyển hai lần. Email nhắc gửi trước khi hết kỳ 14, 7 và 1 ngày, và ở mỗi lần chuyển trạng thái; mỗi email ghi một dòng `billing_notice` để không gửi trùng. Staff ghi đè được mọi bước từ console.

### 5.10 Hạn mức API theo workspace (K11)

Middleware `WorkspaceRateLimit` đặt sau `buildMiddleware`, đếm theo cửa sổ một phút trong Redis với khóa theo workspace, giới hạn lấy từ cổng `api_rate_per_minute` của chính sách quyền lợi (5.7). Vượt thì 429 kèm `Retry-After`. Route của daemon (`/api/daemon/*`) không tính vào hạn mức này vì nhịp heartbeat không phải do người dùng điều khiển.

### 5.11 Xuất dữ liệu workspace

`POST /api/workspaces/{id}/exports` (owner, người thật) tạo một việc xuất chạy nền, ghi một tệp nén gồm các tệp NDJSON: workspace, thành viên, project, issue, comment, nhãn, trạng thái, danh mục tệp đính kèm. Kết quả lưu ở kho tệp, tải qua liên kết có chữ ký hết hạn sau 7 ngày. Mỗi workspace một việc xuất đang chạy; tối đa một lần mỗi ngày. Việc nền chạy ở Multica (3.5), vì đọc dữ liệu workspace; bảng `workspace_export` thuộc platform (mục 8). Bản xuất không chứa bí mật: biến môi trường của agent, cấu hình MCP, token.

Đây là công cụ để khách mang dữ liệu đi và là điều kiện để I10 có nghĩa khi workspace bị khóa hoặc sắp xóa.

### 5.12 Xóa workspace có thời gian chờ

Khi `PLATFORM_WORKSPACE_DELETION_DAYS` trên Multica lớn hơn 0, `DeleteWorkspace` ghi `pending_deletion` vào `platform_workspace` với hạn theo số ngày đó (D15: 14) thay vì xóa ngay. Owner hủy bằng `DELETE /api/workspaces/{id}/deletion`. Hết hạn, việc nền `workspace_deletion` của Multica gọi đúng đường xóa hiện có. Khi biến bằng 0 hoặc không đặt, hành vi xóa ngay giữ nguyên.

### 5.13 Số liệu sử dụng

`platform_usage_daily`: `workspace_id`, `day`, `members`, `agents`, `runtimes`, `issues_total`, `issues_created`, `tasks_run`, `storage_bytes`. Một việc nền hằng đêm của Multica (3.5) tính từ các bảng đã có và ghi vào bảng này, là bảng thuộc platform. Console dùng bảng này để sắp xếp danh sách tenant và vẽ xu hướng; trang chi tiết tenant tính thêm số đếm hiện thời bằng truy vấn có giới hạn.

### 5.14 Kiểm toán đợt R2

Thêm vào danh mục: tích hợp (`integration.connected`, `integration.disconnected` cho GitHub, VCS, Slack, Lark, Telegram, WeCom, DingTalk, Qoder), plugin (`plugin.installed`, `plugin.uninstalled`, `plugin.enabled`, `plugin.disabled`, `plugin.configured`, `plugin.token_rotated`), thư viện MCP, hồ sơ runtime, agent (`agent.created`, `agent.archived`, `agent.permission_changed`, `agent.env_updated`), runtime (`runtime.visibility_changed`, `runtime.deleted`), thanh toán (`billing.order_created`, `billing.order_paid`, `subscription.changed`), trạng thái (`workspace.status_changed`, `account.status_changed`), xuất dữ liệu, và mọi hành động của staff.

Hành động của đợt R5 thuộc cùng danh mục và ghi ở bảng kiểm toán của từng nhóm trong mục 7b: `billing_profile.*`, `workspace.account_kind_changed`, `subscription.change_*`, `subscription.cancel_*` (7b.1); `group.*` (7b.2); `billing.usage_*` (7b.3); `plan.price_*` (7b.4); `account.provision_*`, `account.provisioned`, `account.welcome_*` (7b.5). Hằng tên được thêm vào `platform-management/audit/audit.go` và `catalog_test.go` ở bước Q tương ứng.

## 6. R3: doanh nghiệp

### 6.1 Claim của phiên

JWT thêm `amr`: danh sách cách xác thực đã dùng, gồm `otp` (mã email), `google`, `sso:<connection_id>`, và `mfa` khi đã qua yếu tố thứ hai. `RenewSessionToken` chép tiếp `amr` và `auth_time`. Token cũ không có `amr` được coi là `["otp"]`.

### 6.2 MFA (K13)

`user_mfa`: `user_id`, `secret_enc`, `confirmed_at`, `recovery_hashes`, `last_used_step`.

- **Đăng ký**: `POST /api/me/mfa/enroll` trả bí mật và URI cho ứng dụng xác thực; `POST /api/me/mfa/confirm` với một mã đúng thì kích hoạt và trả mười mã khôi phục, hiện một lần.
- **Đăng nhập**: sau yếu tố thứ nhất, nếu người dùng có MFA, máy chủ **không** cấp phiên mà trả một token chờ sống 5 phút, mang `aud = mfa_pending`. `Auth` từ chối mọi token có `aud` này. `POST /auth/mfa/verify` nhận token chờ và mã TOTP hoặc mã khôi phục, rồi mới cấp JWT có `mfa` trong `amr`.
- **Chống dò**: tối đa 5 lần sai cho mỗi token chờ; `last_used_step` chặn dùng lại một mã trong cùng cửa sổ thời gian.
- **Tắt hoặc đặt lại**: người dùng tắt bằng một mã hợp lệ; mất thiết bị thì dùng mã khôi phục hoặc nhờ staff đặt lại (ghi kiểm toán, gửi email báo cho người dùng).
- **Staff**: từ khi R3 phát hành, sau mã email, console đòi thêm mã TOTP của staff (bí mật ở `user_mfa`, đăng ký qua web app Multica) và đặt `mfa` vào `amr` của `mpa_session`; `RequirePlatformStaff` đòi `mfa` trong `amr` đó. Platform giải mã bí mật bằng `PLATFORM_SECRET_KEY` (15.3(b)).

### 6.3 SSO OIDC (K13)

**Xác minh tên miền.** `sso_domain`: `workspace_id`, `domain`, `token`, `verified_at`. Owner thêm tên miền, hệ thống cấp một giá trị để đặt vào bản ghi TXT, rồi kiểm tra DNS. Một tên miền chỉ được xác minh bởi một workspace. Tên miền email công cộng bị từ chối theo một danh sách.

**Kết nối.** `sso_connection`: `id`, `workspace_id`, `issuer`, `client_id`, `client_secret_enc`, `default_role`, `status`, `created_by`. Mỗi workspace một kết nối.

**Luồng đăng nhập.** Trang đăng nhập gửi email tới `POST /auth/sso/discover`. Nếu tên miền của email đã được xác minh và workspace có kết nối đang bật, client chuyển hướng sang nhà cung cấp danh tính bằng luồng authorization code có PKCE, `state` và `nonce`. Ở callback, máy chủ kiểm chữ ký ID token, `iss`, `aud`, `nonce`, hạn; đòi `email_verified`; và **đòi tên miền của email trong token thuộc các tên miền đã xác minh của chính workspace đó**. Điều kiện cuối là điểm an toàn cốt lõi: nếu thiếu, một workspace có thể cấu hình nhà cung cấp danh tính của mình để khẳng định bất kỳ email nào và chiếm tài khoản người khác.

**Cấp tài khoản tức thời.** Người dùng chưa tồn tại thì được tạo. Chưa là thành viên thì được thêm với `default_role`, đi qua đường giữ ghế (client ghế) như một lượt vào bằng link chia sẻ; hết ghế thì từ chối với thông báo rõ ràng.

**Bắt buộc SSO.** Khi chính sách `require_sso` bật, thành viên có email thuộc tên miền đã xác minh phải có `sso:<kết nối của workspace>` trong `amr` mới vào được workspace đó; thiếu thì 403 mã `sso_required` kèm địa chỉ bắt đầu luồng SSO. Owner được miễn (D19). Thành viên ngoài tên miền (nhà thầu, khách) theo chính sách lời mời ở 6.4.

Cả hai tính năng đòi cờ `sso` của gói.

### 6.4 Chính sách theo workspace (K14)

`workspace_policy`: `workspace_id`, `allowed_invite_domains`, `share_links_enabled`, `require_sso`, `require_mfa`, `session_max_age_hours`, `updated_by`, `updated_at`. Owner sửa; mọi thay đổi ghi kiểm toán (`policy.updated`, ghi giá trị cũ và mới).

| Chính sách | Điểm cưỡng chế | Mã lỗi |
| --- | --- | --- |
| `allowed_invite_domains` | `CreateInvitation`; `JoinByShareLink` theo email người vào | `invite_domain_not_allowed` |
| `share_links_enabled = false` | `CreateShareLink`, `JoinByShareLink`; tắt chính sách thì vô hiệu hóa link đang hoạt động | `share_links_disabled` |
| `require_mfa` | `workspaceguard.CheckWorkspaceAccess`: `amr` phải có `mfa` | `mfa_required` |
| `require_sso` | như 6.3 | `sso_required` |
| `session_max_age_hours` | `workspaceguard.CheckWorkspaceAccess`: `now - auth_time` | `reauth_required` |

Ba mã cuối chỉ chặn workspace đó; người dùng vẫn vào được các workspace khác của họ. Thông tin xác thực máy được miễn ba kiểm tra phiên này vì chúng không phải phiên của người: agent chạy theo quyền của người đã kích hoạt nó, và người đó đã qua kiểm tra khi kích hoạt.

### 6.5 PAT giới hạn (K15)

`personal_access_token` thêm hai cột tùy chọn: `workspace_id` và `read_only`. PAT hiện có giữ nguyên toàn quyền.

- PAT gắn workspace: `workspaceguard.CheckWorkspaceAccess` từ chối khi workspace của request khác workspace của token (403, `token_workspace_mismatch`); route không có ngữ cảnh workspace chỉ cho `GET /api/me`.
- PAT chỉ đọc: `Auth` từ chối phương thức không an toàn (403, `token_read_only`).

`PATCache` hiện chỉ lưu `userID`. Giá trị cache đổi thành cấu trúc có giới hạn, dưới một tiền tố khóa mới để các tiến trình chạy bản cũ và bản mới trong lúc triển khai cuốn chiếu không đọc nhầm của nhau.

### 6.6 Vai trò chỉ xem (K16)

Thêm `viewer` vào ràng buộc `CHECK` của `member`, `workspace_invitation`, `workspace_share_link`. Viewer đọc được những gì một thành viên đọc được.

`workspaceguard.CheckWorkspaceAccess` từ chối mọi phương thức không an toàn của viewer (403, `viewer_read_only`), trừ danh sách cho phép gồm các thao tác trên trạng thái cá nhân: đánh dấu đã đọc hộp thư, tùy chọn thông báo, ghim, rời workspace. `requireDaemonWorkspaceAccess` từ chối viewer, và cache tư cách thành viên của đường daemon không ghi cho viewer (cache hiện chỉ lưu "là thành viên", `server/internal/auth/membership_cache.go`).

Quy tắc ai được gán vai trò giữ nguyên: owner và admin gán được `viewer`, `member`, `admin`; chỉ owner gán được `owner`.

Client: `MemberRole` thêm `viewer`; `rules.ts` từ chối mọi quyền ghi cho viewer; vai trò máy chủ trả mà client không biết được coi là `viewer`, tức quyền thấp nhất (I9). Mobile có mã riêng và phải cập nhật cùng đợt. Client desktop bản cũ có thể hiện nút ghi cho viewer nhưng máy chủ từ chối; đây là suy giảm trải nghiệm chấp nhận được, không phải lỗ hổng.

Đòi cờ `viewer_role` của gói.

## 7. R4: mở rộng (mỗi phần cần đặc tả riêng)

### 7.1 Project hạn chế truy cập và khách mời (K17)

**Yêu cầu.** Một project có thể đặt là hạn chế; chỉ người được cấp mới thấy project và issue trong đó. Khách mời là thành viên chỉ thấy các project được cấp.

**Vì sao không gộp vào đợt này.** Đo ở HEAD: bảng `issue` được đọc ở khoảng 100 câu truy vấn trong 29 tệp query; sự kiện issue được phát cho toàn bộ workspace qua `ScopeWorkspace`; client đồng bộ chỉ mục tìm kiếm cục bộ của cả workspace; agent đọc được mọi issue; bảng điều khiển và các bản tổng hợp cộng dồn qua mọi issue. Một lỗi bỏ sót ở bất kỳ đường nào là rò rỉ dữ liệu giữa những người trong cùng workspace, tức là loại lỗi mà chính tính năng này bán lời hứa ngăn chặn.

**Hướng thiết kế để đặc tả riêng triển khai.** Cột `project.access` (`workspace`, `restricted`) và bảng `project_member`; một vị từ nhìn thấy duy nhất dùng chung cho mọi đường đọc; sự kiện của issue hạn chế phát theo người thay vì theo workspace; chỉ mục tìm kiếm lọc theo người; agent thừa hưởng tầm nhìn của người kích hoạt. Bước đầu tiên của đặc tả đó là bản kiểm kê đầy đủ các đường đọc, có test tự động phát hiện đường mới.

### 7.2 Cổng thanh toán tự động

Một adapter `PaymentProvider` ở platform, đối soát chuyển khoản tự động (D30): nhà cung cấp gửi thông báo mỗi khi tài khoản nhận tiền; adapter kiểm chữ ký, tìm đơn theo `transfer_code` trong nội dung chuyển tiền, so số tiền, rồi gọi `ApplyPayment`. Giao dịch không khớp đơn nào hoặc lệch số tiền được đưa vào hàng chờ cho staff xử lý tay. Hợp đồng API của nhà cung cấp chưa được kiểm chứng trong tài liệu này; đặc tả R4 phải đọc tài liệu chính thức của họ trước khi thiết kế adapter.

### 7.3 Hóa đơn điện tử

Tích hợp một nhà cung cấp hóa đơn điện tử: phát hành khi đơn chuyển `paid`, lưu số và liên kết tra cứu vào `einvoice_ref`.

### 7.4 Quốc tế

Adapter thanh toán quốc tế, nhiều tiền tệ trong danh mục gói, lựa chọn vùng lưu dữ liệu. Mô hình đơn hàng và thuê bao ở mục 5 không cần đổi.

## 7b. Đợt bổ sung: năm nhóm tính năng

Chủ dự án yêu cầu năm nhóm tính năng: (1) quản lý thuê bao cho cá nhân và doanh nghiệp; (2) nhóm tài khoản và quyền trong một doanh nghiệp; (3) gói không giới hạn và gói theo mức dùng có giảm giá cho Business; (4) admin nền tảng đặt giá từng gói; (5) admin nền tảng tạo tài khoản cho người dùng. Mục này là đặc tả đầy đủ của cả năm nhóm, thuộc đợt R5 (mục 1.3), và không cần đặc tả riêng như R4. Mỗi nhóm có đúng các tiểu mục: mục tiêu và người dùng; quy tắc; dữ liệu; API, vai trò, mã lỗi; màn hình; kiểm toán; tương tác với cổng hạn mức, cổng nhận task và trạng thái workspace; bảo mật và riêng tư; tiêu chí chấp nhận (nằm ở mục 13, đợt R5).

### 7b.0 Diễn giải, quy ước chung và thứ tự

Yêu cầu gốc ngắn và có chỗ mơ hồ. Các diễn giải dưới đây là giả định có nhãn; chủ dự án đổi được mà không phải viết lại phần còn lại, vì mỗi giả định chỉ quyết định một giá trị hoặc một quy tắc đã tách riêng.

| # | Giả định | Hệ quả nếu sai |
| --- | --- | --- |
| A1 | "Cá nhân" và "doanh nghiệp" là loại của **workspace**, không phải loại của người dùng (D39). Một người có thể làm owner của một workspace cá nhân và một workspace doanh nghiệp | Nếu chủ dự án muốn thuê bao cấp người dùng (một gói áp cho mọi workspace của người đó), phải đổi client quyền lợi và client ghế: ngoài phạm vi D35 |
| A2 | "Nhóm tài khoản" là nhóm **thành viên của một workspace**, không phải nhóm các workspace. "Quyền của nhóm" là tập khóa quyền cố định (D41) | Nếu muốn vai trò tùy biến tự do, cần đặc tả riêng giống 7.1 |
| A3 | "Không giới hạn" là không giới hạn **hạn mức đếm**, không phải vô thời hạn (D42) | Gói vô thời hạn (mua một lần) cần thêm kiểu kỳ `lifetime` và đổi mục 5.9 |
| A4 | "Giảm giá cho Business" là phần trăm giảm trên **giá theo mức dùng** của workspace `business` (D45) | Nếu muốn giảm cả giá ghế theo năm hay theo số lượng, thêm khóa trong `usage_terms` hoặc một gói riêng |
| A5 | "Tạo tài khoản" là tạo người dùng cùng workspace đầu tiên và thuê bao ban đầu của họ; người dùng đăng nhập bằng mã email như mọi người, không có mật khẩu (D47, D10) | Mật khẩu do staff đặt bị cấm bởi D10; nếu cần, đặc tả riêng |

**Quy ước chung của mục 7b.**

- **Quyền P.** Với một khóa quyền P của 7b.2, "quyền P" nghĩa là: vai trò `owner` hoặc `admin`, hoặc thành viên (`member`, `viewer`) thuộc một nhóm có khóa P. Với phương thức ghi, viewer không bao giờ đạt quyền P (7b.2, quy tắc 7). Ở 7b.1 và 7b.3, mọi route cho khách nằm dưới `/api/cloud-subscriptions`, chỉ người thật (`RequireHumanActor`), đòi cờ `billing_workspace_subscriptions` như bảy route hiện có, và Multica chuyển yêu cầu tới `/api/v1/subscriptions/{workspaceId}/…` của platform bằng `proxyCloudSubscription` (`server/internal/handler/cloud_billing.go:116`) kèm `X-User-ID`. Multica không có client mới (D35).
- **Tiền.** Số nguyên theo đơn vị nhỏ nhất của tiền tệ; làm tròn nửa lên; mọi phép nhân chia trên số tiền dùng số nguyên lớn (`math/big`) hoặc thứ tự phép tính không tràn `int64`, có test biên.
- **Khi `MULTICA_CLOUD_URL` rỗng hoặc trỏ tới Multica Cloud.** Route mới ở `/api/cloud-subscriptions` trả đúng lỗi mà bảy route hiện có trả (`cloud_runtime_not_configured` hoặc 404 của Cloud); client ẩn chức năng khi trường `available_actions` mới vắng mặt (I9). Nhóm tài khoản (7b.2) không phụ thuộc platform: khi không có nguồn thương mại, cổng `member_groups` là `off` nên không giới hạn số nhóm (I2).
- **Loại đơn mới.** `billing_order.kind` có thêm `usage` (7b.3). `kind` là cột văn bản của đơn; nếu migration của P21 đặt `CHECK` trên cột này thì Q10 nới ràng buộc (kế hoạch, Q10).
- **Thứ tự.** Dữ liệu nền dùng chung của nhóm 1, 3, 4 (cột mới của `plan` và `workspace_subscription`) đặt ở 7b.1. Nhóm 3 cần `platform_usage_daily` của P28. Nhóm 4 đứng sau nhóm 3 ở phần giá theo mức dùng. Nhóm 5 cần nhóm 1 (hồ sơ thanh toán) và dùng `audience` của gói.

### 7b.1 Nhóm 1: quản lý thuê bao cho cá nhân và doanh nghiệp (K18)

**Mục tiêu và người dùng.**

- **Cá nhân** (workspace `individual`): một người hoặc nhóm rất nhỏ tự đăng ký, thanh toán bằng tên mình, không cần hóa đơn công ty. Tự xem gói, nâng gói, hạ ghế, hủy và tiếp tục mà không nhờ staff.
- **Doanh nghiệp** (workspace `business`): cần hồ sơ pháp nhân và mã số thuế để xuất hóa đơn, mua được gói chỉ bán cho doanh nghiệp, và có thể giao việc thanh toán cho một nhóm người (7b.2) thay vì owner.
- **Admin nền tảng và staff** `billing`: xem hồ sơ thanh toán của tenant, xác minh doanh nghiệp, sửa hồ sơ khi khách nhờ.

**Quy tắc.**

1. Mỗi workspace có đúng một `account_kind` thuộc {`individual`, `business`}. Không có dòng `billing_profile` nghĩa là `individual`. Workspace tạo mới là `individual`, trừ workspace do admin tạo cho doanh nghiệp (7b.5).
2. Mỗi gói có `audience` thuộc {`individual`, `business`, `any`}; mặc định `any`. Workspace `individual` chỉ chọn được gói `individual` hoặc `any`; workspace `business` chỉ chọn được gói `business` hoặc `any`. Vi phạm trả 409 `plan_audience_mismatch`. Danh mục khởi tạo đặt `audience = business` cho Business và Enterprise, `any` cho Free, Pro và hai gói mới ở 7b.3.
3. Chuyển `individual` thành `business` đòi hồ sơ đủ: `legal_name`, `tax_code`, `address`, `invoice_email`. `tax_code` là chuỗi 10 chữ số, hoặc 13 chữ số (10 chữ số cộng 3 chữ số chi nhánh, chấp nhận dấu gạch giữa); hệ thống chỉ kiểm định dạng, không đối chiếu với cơ quan thuế. Thiếu trường trả 409 `billing_profile_incomplete`; sai định dạng trả 400 `billing_profile_invalid`. Chuyển `business` thành `individual` chỉ khi gói hiện tại có `audience` là `individual` hoặc `any`; không thì 409 `plan_audience_mismatch`. Hồ sơ cũ được giữ khi chuyển ngược.
4. Workspace `individual` có tối đa `PLATFORM_INDIVIDUAL_MAX_SEATS` ghế (mặc định 5). Mua hoặc đặt số ghế lớn hơn trả 409 `account_kind_required` kèm `max_seats`; client mời chuyển sang doanh nghiệp. Gói có `seats_fixed` không bị ảnh hưởng.
5. **Nâng gói** (đổi sang gói đắt hơn trên cùng `billing_model = seat`, cùng `interval`) áp ngay: tạo đơn `change_plan` trạng thái `pending`; quyền lợi mới có hiệu lực khi đơn được áp (`ApplyPayment`, 5.8). Số ghế mới là lớn hơn giữa số ghế hiện tại và `min_seats` của gói mới. Gọi P là số ngày của kỳ hiện tại và R là số ngày còn lại tính từ lúc tạo đơn, cả hai làm tròn lên theo ngày UTC, cùng hàm với mua thêm ghế ở 5.8. `delta = đơn giá ghế mới theo interval × ghế mới − unit_price × ghế hiện tại`, trong đó `unit_price` là đơn giá ghế đã trả của kỳ hiện tại, lưu trên thuê bao lúc áp thanh toán (7b.1, dữ liệu). Số tiền = làm tròn nửa lên của `delta × R / P`. `delta ≤ 0` không phải nâng gói và đi theo quy tắc 6. Workspace `free` hoặc `trialing` không có `unit_price`: đổi gói là đơn `subscribe` với giá đầy đủ của một kỳ bắt đầu từ lúc thanh toán.
6. **Hạ gói, giảm ghế, đổi chu kỳ** (tháng sang năm hoặc ngược lại) không đổi quyền lợi hiện tại. Chúng được lưu vào `workspace_subscription.pending_change` và áp ở **kỳ kế**: đơn gia hạn (`renew`) kế tiếp lấy gói, ghế, chu kỳ từ `pending_change` để tính tiền và áp thuê bao khi thanh toán xong, rồi xóa `pending_change`. Giảm ghế xuống dưới số thành viên hiện có là hợp lệ, không ai bị gỡ, workspace ở trạng thái vượt chỗ (5.6, I10). Số ghế mới không thấp hơn `min_seats` của gói đích. Không có hoàn tiền (D40).
7. **Chuyển giữa hai mô hình tính tiền.** Từ gói theo mức dùng sang gói theo ghế: áp ngay như `subscribe` giá đầy đủ một kỳ mới, và hóa đơn cuối của gói theo mức dùng được lập (7b.3 quy tắc 11). Từ gói theo ghế sang gói theo mức dùng: lưu thành `pending_change` và áp ở cuối kỳ đã trả; không hoàn phần chưa dùng.
8. **Hủy và tiếp tục.** Hủy đặt `cancel_at_period_end = true` khi `status` thuộc {`active`, `trialing`}; ngoài hai trạng thái đó trả 409 `subscription_not_cancelable`. Tiếp tục đặt `false` khi `now` còn trước `current_period_end` (hoặc `trial_end` với `trialing`). Hết kỳ mà cờ còn bật: về `free` không qua ân hạn (5.9). Hủy giữ nguyên `pending_change`, nhưng `pending_change` không còn tác dụng cho tới khi tiếp tục.
9. **Một đơn chờ.** Khi workspace đã có một đơn `pending` loại `subscribe`, `renew`, `add_seats` hoặc `change_plan`, mọi thao tác tạo đơn hoặc ghi `pending_change` trả 409 `order_pending` kèm `order_id`. Đơn loại `usage` (nợ) không chặn quy tắc này; nó có quy tắc riêng ở 7b.3.
10. **Quyền.** Đọc hồ sơ và `pending_change` đòi quyền `billing.view`; mọi route ghi đòi quyền `billing.manage` (7b.2). Token máy luôn bị từ chối. Hai route đọc hiện có (`summary`, `prices`) vẫn cho mọi thành viên.
11. **Lũy đẳng.** Mọi route ghi mới đòi `Idempotency-Key` (header hoặc trường), tối đa 255 byte như `requireCloudSubscriptionIdempotencyKey`; lặp lại cùng khóa trả cùng kết quả, không tạo đơn thứ hai. Đơn `change_plan` áp tối đa một lần.
12. **Xác minh doanh nghiệp.** `verified_at` do staff `billing` hoặc `admin` đặt sau khi đối chiếu mã số thuế ngoài hệ thống. Đổi `legal_name` hoặc `tax_code` xóa `verified_at`. Xác minh chỉ ảnh hưởng ưu đãi giá theo mức dùng (D45); nó không chặn việc mua gói theo ghế.
13. **Bản chụp trên đơn.** Hồ sơ hiện tại được chụp vào `billing_order.buyer` khi tạo đơn. Sửa hồ sơ sau đó không đổi đơn đã tạo; sửa đơn `pending` đi qua `PUT .../buyer` hiện có (5.8).

**Dữ liệu.** Tất cả migration ở 7b dùng dải 953 đến 969 (mục 8, đánh số migration cho R5).

`billing_profile` (platform ghi): `workspace_id`, `account_kind`, `legal_name`, `tax_code`, `address`, `invoice_email`, `verified_at`, `verified_by`, `updated_by`, `updated_at`. Chỉ mục duy nhất `(workspace_id)`. Migration 954 và 955.

Cột mới (migration 953, dùng chung cho nhóm 1, 3 và 4):

| Bảng | Cột mới | Ý nghĩa |
| --- | --- | --- |
| `plan` | `audience text NOT NULL DEFAULT 'any'` | loại workspace được mua (quy tắc 2) |
| `plan` | `billing_model text NOT NULL DEFAULT 'seat'` | `seat` hoặc `usage` (7b.3) |
| `plan` | `usage_rates jsonb NOT NULL DEFAULT '{}'`, `usage_terms jsonb NOT NULL DEFAULT '{}'` | bảng giá và điều khoản theo mức dùng (7b.3) |
| `plan` | `version bigint NOT NULL DEFAULT 0` | tăng một ở mỗi lần đổi gói; dùng cho `expected_version` của console (7b.4) |
| `workspace_subscription` | `pending_change jsonb` | `{plan_key, seats, interval, requested_by, requested_at}` hoặc rỗng |
| `workspace_subscription` | `unit_price bigint` | đơn giá ghế đã trả của kỳ hiện tại; rỗng với `free`, `trialing`, gói theo mức dùng |

Bảng gói trong tệp nhúng (5.5) thêm `audience`, `billing_model`, `usage_rates`, `usage_terms`; một test so tệp với bảng ở 7b.3 và 7b.4.

**API, vai trò, mã lỗi.**

| Route (khách, dưới `/api/cloud-subscriptions`) | Quyền | Ghi chú |
| --- | --- | --- |
| `GET /billing-profile` | `billing.view` | trả `account_kind`, hồ sơ, `verified`; không trả `verified_by` |
| `PUT /billing-profile` | `billing.manage` | đổi hồ sơ và `account_kind` (quy tắc 3) |
| `POST /plan-changes/preview` | `billing.manage` | nhận `plan_key`, `seats`, `interval`; trả `mode` (`immediate` hoặc `scheduled`), `amount`, `currency`, `effective_at`, `purchase_version` |
| `POST /plan-changes` | `billing.manage` | `immediate`: tạo đơn `change_plan` và trả `url` của trang đơn như `checkout-sessions`; `scheduled`: ghi `pending_change`. Nhận `accepted_amount` và `expected_purchase_version` như mua ghế |
| `DELETE /plan-changes/scheduled` | `billing.manage` | xóa `pending_change` |
| `POST /cancel`, `POST /resume` | `billing.manage` | quy tắc 8 |

Trường tùy chọn mới của `GET /summary`: `account_kind`, `billing_model`, `pending_change`, `available_actions.change_plan`, `.cancel`, `.resume`, `.edit_profile`. `GET /api/workspaces/{id}/plans` (5.8) lọc theo `audience` và trả `billing_model`.

| Route (console, dưới `/api/platform`) | Vai trò | Nội dung |
| --- | --- | --- |
| `GET /tenants/{id}/billing-profile` | mọi staff | đọc hồ sơ |
| `PUT /tenants/{id}/billing-profile` | `billing`, `admin` | sửa hồ sơ và `account_kind` |
| `POST /tenants/{id}/billing-profile/verify`, `.../unverify` | `billing`, `admin` | quy tắc 12 |

| Mã | HTTP | Trường thêm |
| --- | --- | --- |
| `plan_audience_mismatch` | 409 | `plan_key`, `account_kind` |
| `billing_profile_invalid` | 400 | `fields[]` |
| `billing_profile_incomplete` | 409 | `fields[]` |
| `account_kind_required` | 409 | `max_seats` |
| `order_pending` | 409 | `order_id` |
| `plan_change_unavailable` | 409 | `reason` (`same_plan`, `plan_archived`, `not_public`) |
| `subscription_not_cancelable` | 409 | `status` |
| `quote_changed` | 409 | `amount` (số tiền mới) |

**Màn hình.**

- **Multica** (`packages/views/settings/components/billing-tab.tsx` và `packages/views/billing/`, web và desktop): bộ chọn gói lọc theo `audience`; hộp thoại xem trước đổi gói (nâng ngay có số tiền; hạ có ngày hiệu lực); mục hủy và tiếp tục; biểu mẫu hồ sơ thanh toán với công tắc cá nhân và doanh nghiệp; biểu ngữ "thay đổi sẽ áp ngày …" khi có `pending_change`. Hiện với quyền `billing.view`; nút ghi hiện với quyền `billing.manage`. Mobile không có trang thanh toán và không đổi.
- **Console**: chi tiết tenant thêm khối hồ sơ thanh toán với nút xác minh; `billing` và `admin` sửa được, các vai trò khác chỉ xem.

**Kiểm toán.**

| Nhóm | Hành động |
| --- | --- |
| Hồ sơ | `billing_profile.updated` (ghi tên trường đã đổi, không ghi giá trị mã số thuế), `billing_profile.verified`, `billing_profile.unverified`, `workspace.account_kind_changed` (giá trị cũ và mới) |
| Thuê bao | `subscription.change_scheduled`, `subscription.change_canceled`, `subscription.cancel_requested`, `subscription.cancel_withdrawn`; đơn `change_plan` dùng `billing.order_created` và `billing.order_paid` đã có (5.14) |

Mọi dòng có `workspace_id`, nên chủ workspace thấy trong nhật ký của mình, kể cả khi staff là bên thực hiện.

**Tương tác với cổng hạn mức, cổng nhận task và trạng thái workspace.**

- Cổng hạn mức: đổi gói đổi chính sách quyền lợi qua `version` của thuê bao; client quyền lợi lấy chính sách mới trong tối đa 5 phút (5.5). Giảm xuống gói thấp hơn chỉ chặn tạo mới (I10).
- Trạng thái workspace: các route mới nằm dưới `/api/cloud-subscriptions/*` nên đã thuộc danh sách cho phép của 5.3: owner và admin dùng được khi `read_only`, `suspended` và `pending_deletion`. Người chỉ có quyền nhóm không dùng được khi `suspended` và `pending_deletion` (7b.2, quy tắc 8). Đơn `change_plan` thanh toán xong đưa workspace `read_only` vì quá hạn về `active` như mọi đơn (5.8).
- Cổng nhận task: không đổi (D25).

**Bảo mật và riêng tư.**

| Mối nguy | Biện pháp |
| --- | --- |
| Tự khai doanh nghiệp để lấy ưu đãi | Ưu đãi giá theo mức dùng đòi `verified_at` (D45, mặc định bật); mã số thuế chỉ kiểm định dạng nên không dùng làm bằng chứng |
| Lộ mã số thuế, địa chỉ, email hóa đơn | Chỉ đọc được với quyền `billing.view`; không có trong `summary`; nhật ký chỉ ghi tên trường; hồ sơ thuộc workspace nên vào bản xuất workspace của owner (5.11) và không vào bản xuất tài khoản |
| Thành viên thường tạo đơn tốn tiền | Route ghi đòi quyền `billing.manage`; token máy 403; mọi đơn ghi `created_by` và kiểm toán |
| Đổi giá giữa lúc xem trước và lúc mua | `accepted_amount` và `expected_purchase_version`; lệch trả 409 `quote_changed` |
| Hai tab cùng đổi gói | Quy tắc 9 và khóa dòng thuê bao trong giao dịch tạo đơn |
| Hạ ghế rồi dùng dịch vụ như cũ | I10: không gỡ ai; vượt chỗ chỉ chặn thêm người (5.6) |

**Tiêu chí chấp nhận.** Mục 13, đợt R5, nhóm 1.

### 7b.2 Nhóm 2: nhóm tài khoản và quyền trong một doanh nghiệp (K19)

**Mục tiêu và người dùng.**

- **Owner và admin của doanh nghiệp**: gom nhân viên thành nhóm (ví dụ kế toán, trưởng nhóm, nhân sự) và trao cho nhóm đúng vài quyền quản trị cụ thể, không phải nâng từng người lên `admin`.
- **Thành viên trong nhóm**: làm được những thao tác nhóm được trao, không hơn.
- **Staff**: không đọc được tên nhóm hay danh sách thành viên nhóm (I6); console chỉ thấy số nhóm của tenant.

**Quy tắc.**

1. Nhóm thuộc đúng một workspace. Tên dài 1 đến 64 ký tự và duy nhất không phân biệt hoa thường trong workspace (409 `group_name_taken`); mô tả tùy chọn, tối đa 280 ký tự.
2. Chỉ thành viên của workspace vào được nhóm (409 `not_a_member`). Một thành viên ở được nhiều nhóm. Một nhóm có tối đa 1.000 thành viên (hằng số trong mã; vượt trả 409 `group_member_limit`).
3. Quyền của nhóm là một tập con của danh mục sáu khóa dưới đây. Khóa không thuộc danh mục trả 400 `permission_unknown`. Khóa lạ đã lưu (do bản mã khác ghi) bị bỏ qua lúc kiểm, tức không cấp quyền nào.
4. Quyền hiệu lực của một người = quyền ngầm của vai trò hợp với quyền của mọi nhóm họ thuộc. `owner` và `admin` có mọi khóa. `billing.manage` kéo theo `billing.view`.
5. Nhóm chỉ **cộng** quyền. Không có quyền "cấm" và không có thứ tự ưu tiên giữa các nhóm.
6. Quản lý nhóm (tạo, sửa, xóa, đặt quyền, thêm và bớt thành viên) chỉ dành cho `owner` và `admin` là người thật; không khóa nào trong danh mục cho phép việc này, nên không ai nâng quyền cho chính mình hay cho người khác qua nhóm.
7. **Viewer không bao giờ ghi nhờ nhóm.** Với phương thức không an toàn, `ViewerGuard` (6.6) chặn trước khi kiểm khóa. Với phương thức an toàn (đọc), viewer dùng khóa như thành viên.
8. Khi workspace `suspended` hoặc `pending_deletion`, các route dùng khóa chỉ mở cho `owner` và `admin` (danh sách cho phép ở 5.3), không mở cho người chỉ có quyền nhóm. Khi `read_only`, route ghi vẫn bị `CheckWorkspaceAccess` chặn như hiện nay, trừ danh sách cho phép.
9. Người có khóa `members.invite` mà không phải `owner` hay `admin` chỉ tạo được lời mời và link chia sẻ với vai trò `member` hoặc `viewer`; vai trò `admin` trả 403 `invite_role_not_assignable`. Khóa này không gồm đổi vai trò hay gỡ thành viên.
10. Gỡ thành viên, rời workspace và xóa tài khoản xóa mọi dòng nhóm của người đó trong **cùng giao dịch** gỡ (`revokeAndRemoveMember`, `server/internal/handler/workspace_revoke.go:49`). Xóa nhóm xóa các dòng thành viên của nhóm trong cùng giao dịch (I3).
11. Kiểm quyền đọc cơ sở dữ liệu mỗi request tại route được bảo vệ, chỉ khi vai trò không phải `owner` hay `admin` (một truy vấn theo `(workspace_id, user_id)`). Không cache, nên bớt một người khỏi nhóm có hiệu lực ở request kế tiếp.
12. Cổng đếm `member_groups` theo 5.7: giới hạn 0 trả 402 `plan_feature_unavailable` với `feature = member_groups`; vượt giới hạn trả 402 `plan_limit_reached`; kiểm trong giao dịch tạo nhóm sau khóa tư vấn theo (workspace, cổng). Hạ gói giữ nguyên nhóm và quyền đang có (I10); chỉ chặn tạo nhóm mới. Khi không có nguồn thương mại, cổng là `off`.
13. Mọi thay đổi nhóm, thành viên nhóm và quyền ghi kiểm toán; thay đổi quyền ghi cả giá trị cũ và mới.
14. `GET /api/workspaces/{id}/permissions/me` trả quyền hiệu lực của người gọi. Client dùng nó để hiện hay ẩn điều khiển; máy chủ vẫn là nơi quyết định (I1).

**Danh mục khóa quyền (sáu khóa, cố định trong mã).** Cột "route" liệt kê các route hiện đang là của owner và admin mà khóa mở thêm cho thành viên có khóa. Thêm khóa mới là thêm một dòng ở đây, một hằng trong mã, ánh xạ route và test.

| Khóa | Route mở thêm (tiền tố `/api/workspaces/{id}` trừ khi ghi khác) | Ghi chú |
| --- | --- | --- |
| `members.invite` | `POST /members` (tạo lời mời), `GET /invitations`, `DELETE /invitations/{invitationId}`, `GET /share-links`, `POST /share-links`, `DELETE /share-links/{linkId}` | vai trò mời bị giới hạn (quy tắc 9) |
| `audit.read` | `GET /audit-events`, kể cả `?format=csv` khi gói có `audit_export` | |
| `billing.view` | `GET /orders`, `GET /orders/{orderId}`, `GET /api/cloud-subscriptions/billing-profile`, `GET /api/cloud-subscriptions/usage` | |
| `billing.manage` | các route ghi của `/api/cloud-subscriptions` đang dành cho owner và admin (`checkout-sessions`, `seats/*`, `portal-sessions`) và route mới của 7b.1, `PUT /orders/{orderId}/buyer` | |
| `integrations.manage` | kết nối và gỡ GitHub, VCS, Qoder, Slack, WeCom, Telegram (các route trong nhóm owner và admin ở `server/cmd/server/router.go:1957` và các nhóm cùng loại ngay sau) | |
| `runtime_profiles.manage` | `POST /runtime-profiles`, `PATCH`, `PUT`, `DELETE /runtime-profiles/{profileId}` | |

Không thuộc danh mục và vẫn chỉ dành cho owner và admin: cập nhật workspace, đổi vai trò và gỡ thành viên, thư viện MCP, mọi thao tác plugin, xuất dữ liệu, xóa workspace (owner), chính sách và SSO (R3), quản trị tri thức, và chính các route nhóm. Lý do: các thao tác này chạy mã hoặc giữ bí mật của workspace, hoặc là cách để nâng quyền.

**Dữ liệu.** Hai bảng mới, thuộc Multica (workspace sở hữu), không khóa ngoại.

`member_group`: `id`, `workspace_id`, `name`, `description`, `permissions` (JSONB, mảng chuỗi khóa), `created_by`, `created_at`, `updated_at`. Chỉ mục duy nhất `(workspace_id, id)` và `(workspace_id, lower(name))`. Migration 956 (bảng), 957 và 958 (hai chỉ mục).

`member_group_member`: `workspace_id`, `group_id`, `user_id`, `added_by`, `added_at`. Chỉ mục duy nhất `(workspace_id, user_id, group_id)`: tiền tố `(workspace_id, user_id)` phục vụ truy vấn quyền của một người; truy vấn theo nhóm lọc `workspace_id` rồi `group_id`, số dòng của một workspace bị chặn bởi số thành viên nhân số nhóm. Migration 959 (bảng), 960 (chỉ mục).

Sổ đăng ký cột người dùng (4.6): `member_group_member.user_id` là `delete`; `member_group_member.added_by` và `member_group.created_by` là `keep`.

**API, vai trò, mã lỗi.** Mọi route nhóm đứng sau `RequireHumanActor`.

| Route | Quyền |
| --- | --- |
| `GET /api/workspaces/{id}/permission-catalogue` | owner, admin |
| `GET /api/workspaces/{id}/permissions/me` | mọi thành viên |
| `GET /api/workspaces/{id}/groups`, `GET .../groups/{groupId}` | owner, admin |
| `POST /api/workspaces/{id}/groups` | owner, admin; qua cổng `member_groups` |
| `PATCH`, `DELETE /api/workspaces/{id}/groups/{groupId}` | owner, admin |
| `PUT /api/workspaces/{id}/groups/{groupId}/permissions` | owner, admin |
| `POST /api/workspaces/{id}/groups/{groupId}/members` (tối đa 100 người mỗi lần), `DELETE .../members/{userId}` | owner, admin |

| Mã | HTTP | Trường thêm |
| --- | --- | --- |
| `permission_denied` | 403 | `permission` |
| `invite_role_not_assignable` | 403 | `role` |
| `group_name_taken` | 409 | |
| `group_not_found` | 404 | |
| `not_a_member` | 409 | |
| `group_member_limit` | 409 | `limit` |
| `permission_unknown` | 400 | `permission` |

`plan_feature_unavailable` và `plan_limit_reached` (mục 10) dùng cho quy tắc 12. `permission_denied` thay thông điệp 403 chung ở các route thuộc danh mục; client cũ vẫn xử lý được vì mã HTTP không đổi.

**Màn hình.**

- **Multica** (`packages/views/settings/`, web và desktop): tab "Groups" cho owner và admin (danh sách nhóm, hộp thoại tạo và sửa với hộp chọn khóa, bộ chọn thành viên); danh sách thành viên hiện nhóm của từng người; `packages/core/permissions/rules.ts` thêm `can(key)` đọc từ `permissions/me`; `members-tab.tsx` và các tab thanh toán, nhật ký, tích hợp đổi từ kiểm vai trò sang kiểm khóa. Mobile không có các màn hình này; mobile chỉ cần hiểu mã `permission_denied` (R5 không đổi mobile).
- **Console**: chi tiết tenant thêm số nhóm; không có tên nhóm, thành viên nhóm hay quyền (I6).

**Kiểm toán.**

| Nhóm | Hành động |
| --- | --- |
| Nhóm | `group.created`, `group.updated` (chỉ ghi tên trường đã đổi), `group.deleted` |
| Thành viên nhóm | `group.member_added`, `group.member_removed` |
| Quyền | `group.permissions_changed` (giá trị cũ và mới của mảng khóa) |

**Tương tác với cổng hạn mức, cổng nhận task và trạng thái workspace.**

- Cổng hạn mức: thêm cổng đếm `member_groups` vào `GateName` (7b.0, hook ở 3.3). Giá trị khởi tạo ở bảng gói (7b.3): Free và Pro là 0, Business là 50, các gói còn lại không giới hạn.
- Cổng nhận task (D25): không đổi. Quyền nhóm không ảnh hưởng việc agent nhận task; agent chạy dưới quyền của người đã kích hoạt như hiện nay, kể cả quyền nhóm của người đó.
- Trạng thái workspace: quy tắc 8. Route nhóm là route ghi nên bị chặn khi `read_only`, `suspended` và `pending_deletion`.
- R3: ViewerGuard (6.6) chạy trước kiểm khóa; chính sách `require_mfa`, `require_sso`, `reauth_required` (6.4) áp trước khi tới kiểm khóa, vì chúng nằm trong `CheckWorkspaceAccess`.

**Bảo mật và riêng tư.**

| Mối nguy | Biện pháp |
| --- | --- |
| Nâng quyền qua mời một `admin` | Quy tắc 9; test mọi route tạo lời mời và link chia sẻ |
| Tự nâng quyền hoặc nâng quyền cho người khác qua nhóm | Quản lý nhóm không nằm trong danh mục (quy tắc 6) |
| Dùng id nhóm của workspace khác | Mọi truy vấn lọc `workspace_id`; id lạ trả 404 `group_not_found`; test chéo workspace |
| Quyền còn lại sau khi bị bỏ khỏi nhóm hay bị gỡ | Không cache (quy tắc 11); dọn trong giao dịch gỡ (quy tắc 10); test sổ đăng ký cột người dùng |
| Viewer ghi qua nhóm | Quy tắc 7; test duyệt bảng route với viewer thuộc nhóm có đủ sáu khóa |
| Lộ cơ cấu tổ chức | Danh sách nhóm chỉ owner và admin; console chỉ có số đếm; tên nhóm không vào nhật ký của tenant khác |
| Khóa mới thêm sau này mở nhầm route | Mỗi khóa có test duyệt: đúng tập route của khóa mở cho người có khóa, và không route nào ngoài tập đó |

**Tiêu chí chấp nhận.** Mục 13, đợt R5, nhóm 2.

### 7b.3 Nhóm 3: gói không giới hạn và gói theo mức dùng, có giảm giá cho Business (K20)

**Mục tiêu và người dùng.**

- **Cá nhân và nhóm nhỏ dùng nhẹ**: gói theo mức dùng (pay as you go) trả đúng phần đã dùng, không cam kết số ghế.
- **Doanh nghiệp**: gói theo mức dùng với tỷ lệ giảm riêng cho Business, hoặc gói không giới hạn để không phải theo dõi hạn mức.
- **Admin nền tảng và staff** `billing`: theo dõi dòng dùng theo ngày, hóa đơn và nợ của từng tenant; `admin` xóa nợ có lý do.

**Quy tắc.** Quy tắc 1 đến 4 là về gói không giới hạn; từ quy tắc 5 là gói theo mức dùng.

1. Gói không giới hạn (`unlimited` trong danh mục khởi tạo) là gói có **mọi hạn mức đếm để trống**: `issue_count`, `autopilot_runs`, `agents`, `runtimes`, `storage_bytes`, `member_groups`. Quy ước "rỗng nghĩa là không giới hạn" của 5.5 vẫn áp cho từng cổng của mọi gói; gói không giới hạn chỉ là gói đặt rỗng cho tất cả.
2. `api_rate_per_minute` rỗng, ở gói hoặc ở `limit_overrides`, được platform trả thành `PLATFORM_API_RATE_CEILING` (mặc định 6.000) với `action = enforce` (D42). Giá trị tường minh do staff đặt cao hơn trần thì được dùng nguyên.
3. Cổng đếm để trống trả `action = off` (5.5). Không giới hạn không miễn trừ trạng thái workspace (5.3) hay cổng nhận task.
4. Chuyển từ gói không giới hạn xuống gói có giới hạn không xóa dữ liệu: phần vượt giữ nguyên, chỉ chặn tạo mới (I10).
5. Gói `billing_model = usage` không có `seat_price_month`, `seat_price_year`, `seats_fixed`, `min_seats`, `trial_days`. Thuê bao của gói này có `seats = 0`; `seat_capacity` của `summary` là `null` và ghế không được quản lý (`Managed = false`, Phụ lục B), nên lời mời và link chia sẻ không bị chặn vì ghế.
6. **Kích hoạt.** `POST checkout-sessions` với `plan_key` của gói theo mức dùng không tạo đơn chờ thanh toán. Nó kích hoạt thuê bao `active` ngay, kỳ là tháng dương lịch UTC hiện tại (D28), kèm một đơn `subscribe` 0 đồng trạng thái `paid` để vết đầy đủ, và trả `url` của trang đơn. Điều kiện: hồ sơ thanh toán đủ (với `business`: 7b.1 quy tắc 3; với `individual`: `legal_name` là tên người trả tiền và `invoice_email`), và workspace không có đơn `usage` chưa trả (quy tắc 13). Không có hồ sơ thì 409 `billing_profile_incomplete`.
7. **Ba đồng hồ**, cố định trong mã, lấy từ `platform_usage_daily` (5.13): `tasks_run` (số task chạy trong ngày), `storage_bytes` (dung lượng cuối ngày), `members` (số thành viên cuối ngày). `tasks_run` là đồng hồ dòng chảy; hai đồng hồ còn lại là đồng hồ tồn, tính theo tỷ lệ ngày của tháng. Thêm đồng hồ là thêm một hằng trong mã, một cột nguồn và test.
8. `usage_rates` là map đồng hồ sang `{per, price}`: `price` là số tiền cho `per` đơn vị (task; byte-tháng; thành viên-tháng). Đồng hồ vắng mặt tính 0 đồng. Công thức của **một dòng ngày**, một số nguyên, một lần làm tròn nửa lên:

   `net = round_half_up( quantity × price × (100 − discount_pct) / (per × days × 100) )`

   với `days = 1` cho `tasks_run`, và `days` là số ngày của tháng UTC chứa ngày đó cho hai đồng hồ còn lại.
9. **Giảm giá Business.** `discount_pct` là `usage_terms.discount_pct.business` khi `account_kind = business` và, nếu `PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION` là `true`, `verified_at` có giá trị lúc chấm; ngược lại là 0. Tỷ lệ là số nguyên từ 0 đến 100 (D45).
10. **Chấm tiền theo ngày.** Việc nền `usage_rating` chấm các ngày đã đóng (`day` trước ngày UTC hiện tại) có dòng `platform_usage_daily`: mỗi (workspace, ngày, đồng hồ) đúng một dòng `usage_charge`, kể cả dòng 0 đồng. Dòng giữ ảnh chụp `unit_price`, `per`, `discount_pct`, `plan_key`. Giá của ngày D là giá có hiệu lực lúc 00:00 UTC của D (7b.4 quy tắc 9), nên đổi giá không chấm lại ngày đã chấm. Ngày chưa có số liệu không bị chấm; dòng đến muộn được chấm khi đến và đi vào hóa đơn kế tiếp. Ngày workspace đang `suspended` hoặc `pending_deletion` được ghi dòng 0 đồng với `note = 'workspace_suspended'`; ngày `read_only` vẫn tính.
11. **Hóa đơn tháng.** Việc nền `usage_invoicing` lập, sau đầu tháng UTC, một đơn `usage` cho mỗi thuê bao theo mức dùng có dòng chưa lập hóa đơn với `day` trước đầu tháng hiện tại. Số tiền bằng tổng `net_amount`; `vat_amount` theo `BILLING_VAT_RATE`; `status = pending`; `expires_at` rỗng (hóa đơn không tự hết hạn); `period_start` và `period_end` là tháng; `buyer` chụp từ hồ sơ. Trong cùng giao dịch, các dòng được gắn `order_id` bằng câu lệnh có điều kiện `order_id IS NULL`. Tổng nhỏ hơn `PLATFORM_USAGE_MIN_INVOICE` thì không lập đơn và dồn sang kỳ sau. **Hóa đơn cuối** được lập, bỏ mức tối thiểu, khi thuê bao rời gói theo mức dùng (về `free`, hoặc đổi sang gói theo ghế).
12. **Hạn mức công nợ.** `usage_terms.credit_limit` có giá trị theo `account_kind`. Khi tổng `net_amount` chưa lập hóa đơn của một workspace đạt hạn mức, `usage_rating` lập ngay một hóa đơn tạm cho các dòng đã chấm (như quy tắc 11, bỏ mức tối thiểu). Khi đạt 80 phần trăm hạn mức, gửi một email cho owner (một lần mỗi tháng; `billing_notice` kind `usage_80`). Hạn mức công nợ không chặn việc dùng.
13. **Chưa trả.** Đơn `usage` còn `pending` quá `PLATFORM_USAGE_DUE_DAYS` (mặc định 7) từ `created_at` đưa thuê bao sang `past_due` với `grace_until = created_at + PLATFORM_USAGE_DUE_DAYS + PLATFORM_GRACE_DAYS`; qua `grace_until` workspace sang `read_only` với `reason = billing`; sau `PLATFORM_READ_ONLY_DAYS` thuê bao về `free` và workspace về `active` (D14, 5.9). Đơn vẫn `pending`, là nợ. Khi còn nợ, `checkout-sessions` và `plan-changes` sang gói trả phí trả 409 `usage_invoice_unpaid`; gói `free` vẫn dùng được (I10). Thanh toán đơn `usage` đưa workspace `read_only` vì lý do này về `active`.
14. **Xóa nợ.** Chỉ staff `admin`, kèm lý do bắt buộc, hủy được đơn `usage` `pending`. Các dòng đã gắn với đơn đó không bao giờ vào hóa đơn khác. Staff `billing` không xóa được nợ.
15. `GET /prices` không bao giờ trả giá của gói theo mức dùng (D43). Với workspace đang ở gói theo mức dùng, nó trả giá ghế của gói theo ghế công khai rẻ nhất, như với workspace `free` (5.8).
16. Khách xem được mức dùng của mình: kỳ hiện tại (tạm tính tới ngày đã chấm), từng đồng hồ với số lượng, số tiền và phần giảm; chi tiết mỗi hóa đơn `usage` có đủ dòng theo ngày.

**Gói khởi tạo bổ sung** (D24; con số là lựa chọn chưa đo, chủ dự án đặt ở 7b.7). Bốn gói hiện có giữ nguyên các giá trị ở 5.5 và nhận thêm hai cột: `audience` (Free và Pro: `any`; Business và Enterprise: `business`) và `member_groups` (Free và Pro: 0; Business: 50; Enterprise: không giới hạn).

| | Unlimited (`unlimited`) | Pay as you go (`payg`) |
| --- | --- | --- |
| `audience` | `any` | `any` |
| `billing_model` | `seat` | `usage` |
| Công khai | có | có |
| Đơn giá ghế mỗi tháng và mỗi năm (VND) | 249.000 và 2.490.000 | không có |
| Ghế | mua, tối thiểu 1 | không quản lý ghế |
| Dùng thử | 14 ngày | không |
| `issue_count`, `autopilot_runs`, `agents`, `runtimes`, `storage_bytes`, `member_groups` | không giới hạn | không giới hạn |
| `api_rate_per_minute` | 3.000 | 1.200 |
| `audit_retention_days` | 180 | 180 |
| `audit_export` | có | có |
| `sso`, `workspace_policies`, `viewer_role` | không | không |
| `usage_rates.tasks_run` | không có | 30.000 cho 1.000 task |
| `usage_rates.storage_bytes` | không có | 3.000 cho 1 GiB-tháng (`per` = 1.073.741.824) |
| `usage_rates.members` | không có | 49.000 cho 1 thành viên-tháng |
| `usage_terms.discount_pct.business` | không có | 20 |
| `usage_terms.credit_limit` | không có | `individual` 2.000.000; `business` 20.000.000 |

**Dữ liệu.** Cột mới của `plan` ở 7b.1 (migration 953). Bảng mới `usage_charge` (platform ghi): `workspace_id`, `day` (date), `meter`, `quantity` (bigint), `plan_key`, `unit_price`, `per`, `discount_pct`, `net_amount`, `note`, `order_id` (rỗng tới khi lập hóa đơn), `rated_at`. Chỉ mục duy nhất `(workspace_id, day, meter)`; chỉ mục `(order_id)` có điều kiện `order_id IS NOT NULL`. Migration 961 (bảng), 962 và 963 (chỉ mục). Kinds mới của `billing_notice`: `usage_80`. Kind mới của `billing_order`: `usage`.

**API, vai trò, mã lỗi.**

| Route | Quyền | Ghi chú |
| --- | --- | --- |
| `GET /api/cloud-subscriptions/usage` (khách) | `billing.view` | kỳ hiện tại, từng đồng hồ, hạn mức công nợ |
| `GET /api/workspaces/{id}/orders/{orderId}/usage` (khách) | `billing.view` | các dòng theo ngày của một hóa đơn `usage` |
| `POST /api/cloud-subscriptions/checkout-sessions` (khách) | `billing.manage` | quy tắc 6 cho gói theo mức dùng |
| `GET /api/platform/tenants/{id}/usage` (console) | mọi staff | dòng theo ngày; chỉ số đếm và tiền, không nội dung (I6) |
| `POST /api/platform/orders/{id}/cancel` (console) | `billing`, `admin`; riêng đơn `usage` chỉ `admin`, kèm `reason` | quy tắc 14 |

Trường tùy chọn mới của `GET /summary`: `billing_model` và `usage` gồm `period_start`, `period_end`, `currency`, `accrued_amount`, `credit_limit`, `meters[]` (`key`, `quantity`, `amount`). Mã lỗi mới: `usage_invoice_unpaid` (409, `order_id`). Dùng lại `billing_profile_incomplete`, `plan_audience_mismatch`.

**Màn hình.**

- **Multica** (`billing-tab.tsx`, `packages/views/billing/`, web và desktop): thẻ "mức dùng kỳ này" (từng đồng hồ, số tiền tạm tính, thanh so với hạn mức công nợ); dòng gói theo mức dùng trong bộ chọn gói hiển thị bảng giá và, chỉ khi hồ sơ đã xác minh, phần giảm Business; danh sách hóa đơn có đơn `usage`; biểu ngữ khi có hóa đơn chưa trả.
- **Console**: tab mức dùng ở chi tiết tenant (dòng theo ngày, ngày thiếu số liệu, đơn `usage` chưa trả); nút xóa nợ cho `admin`.

**Kiểm toán.**

| Nhóm | Hành động |
| --- | --- |
| Mức dùng | `billing.usage_invoiced` (mã đơn, số tiền, số dòng), `billing.usage_credit_limit_reached`, `billing.usage_written_off` (lý do) |
| Kích hoạt | `subscription.changed` (đã có) với `detail.billing_model = usage` |

**Tương tác với cổng hạn mức, cổng nhận task và trạng thái workspace.**

- Cổng hạn mức: gói theo mức dùng và gói không giới hạn trả `off` cho các cổng đếm để trống; `api_rate_per_minute` luôn `enforce` (quy tắc 2). Đồng hồ `tasks_run` đo, không chặn; không có cổng mới.
- Cổng nhận task (D25): không đổi. Workspace `read_only` vì nợ không nhận task (5.3), và đó là hệ quả của quy tắc 13, không phải của việc đo.
- Trạng thái workspace: quy tắc 10 (ngày khóa do staff không tính) và quy tắc 13 (nợ đi theo D14).
- Ghế: `Managed = false` (quy tắc 5).

**Bảo mật và riêng tư.**

| Mối nguy | Biện pháp |
| --- | --- |
| Hóa đơn bất ngờ | Hạn mức công nợ với hóa đơn tạm; email ở 80 phần trăm; khách xem mức dùng theo ngày |
| Tính tiền hai lần hoặc sót | Chỉ mục duy nhất `(workspace_id, day, meter)`; gắn `order_id` có điều kiện trong cùng giao dịch lập đơn; job lũy đẳng, chạy song song không lập hai đơn |
| Sai số làm tròn, tràn số | Một lần làm tròn mỗi dòng; số nguyên lớn; test biên 100 TB và tháng 28 đến 31 ngày |
| Lấy ưu đãi giá bằng mã số thuế tự khai | D45: đòi `verified_at` |
| Tranh chấp số liệu | Dòng giữ ảnh chụp giá, tỷ lệ và số lượng; khách và staff xem cùng dòng; chỉ có xóa nợ theo cả đơn, không có điều chỉnh từng dòng (rủi ro ở 15.3) |
| Dùng "không giới hạn" để gây tải | Trần tốc độ API (quy tắc 2); điều khoản dùng hợp lý ghi ở mục 16; staff sắp xếp danh sách tenant theo dung lượng (5.13) và đặt `limit_overrides` |
| Bỏ nợ rồi mở workspace mới | Nợ gắn với workspace; số workspace miễn phí của một owner bị chặn bởi `FREE_PLAN_OWNED_WORKSPACE_LIMIT` (5.7); phần còn lại là rủi ro chấp nhận (15.3) |
| Số liệu dùng lộ nội dung | `usage_charge` chỉ có số đếm, dung lượng và tiền |

**Tiêu chí chấp nhận.** Mục 13, đợt R5, nhóm 3.

### 7b.4 Nhóm 4: admin nền tảng cấu hình giá từng gói (K21)

**Mục tiêu và người dùng.**

- **Admin nền tảng** (vai trò `admin`): đặt giá tùy ý cho từng gói (đơn giá ghế, bảng giá và điều khoản theo mức dùng), tạo gói riêng cho khách có giá thương lượng, và biết trước tác động của mỗi lần đổi giá.
- **Staff `billing`**: đọc lịch sử giá để trả lời khách.
- **Khách (owner và người có quyền `billing.view`)**: biết trước giá sắp đổi, không bị đổi giá của kỳ đã trả hay của đơn đã tạo.

**Quy tắc.**

1. Chỉ `admin` lập và hủy thay đổi giá. `billing` đọc được lịch sử; `support` và vai trò khác nhận 404 như mọi route console (5.2).
2. **Trường giá** là `seat_price_month`, `seat_price_year`, `usage_rates` và `usage_terms`. Chúng chỉ đổi qua `plan_price_change` (D46). `PUT /plans/{key}` có chứa một trường giá trả 400 `price_field_via_schedule`. Tên, `limits`, `features`, `audience`, `min_seats`, `trial_days`, `is_public`, `sort` vẫn sửa tại chỗ, kèm `expected_version` (409 `plan_version_conflict` khi lệch). `POST /plans` tạo gói mới cho phép đặt giá ban đầu trực tiếp; từ đó trở đi mọi đổi giá đi qua lịch.
3. Một thay đổi giá gồm `effective_from` (UTC), các giá trị mới của một hay nhiều trường giá, và `reason` bắt buộc (tối đa 500 ký tự). Hệ thống lưu `old` và `new` đầy đủ của các trường bị đổi. Mỗi gói có tối đa một thay đổi `scheduled` (409 `price_change_pending`); giao dịch lập khóa dòng `plan`, và chỉ mục duy nhất có điều kiện chặn cả khi hai admin gửi cùng lúc.
4. **Kiểm giá hợp lệ** (400 `price_invalid` kèm `field`): giá là số nguyên không âm, không có phần thập phân (VND); gói `billing_model = seat` trả phí có giá tháng và giá năm đều dương, và giá năm không vượt 12 lần giá tháng; mọi giá không vượt `PLATFORM_PRICE_MAX`; gói `free` luôn 0; gói `usage` không có giá ghế; mọi khóa của `usage_rates` thuộc ba đồng hồ của 7b.3, với `per` dương và `price` không âm; `discount_pct` và `credit_limit` hợp lệ theo 7b.3.
5. **Xác nhận lại mức đổi lớn.** Một giá đổi quá `PLATFORM_PRICE_CONFIRM_PCT` phần trăm (mặc định 50) so với giá hiện hành, khi giá hiện hành khác 0, đòi `confirm_large_change = true`; thiếu thì 409 `price_change_confirm_required` kèm `change_pct`. Mục đích là bắt lỗi gõ thừa hay thiếu một chữ số.
6. Tiền tệ của gói không đổi khi gói đã có thuê bao hoặc đơn (409 `plan_currency_locked`).
7. **Báo trước khi tăng.** Một thay đổi **tăng** giá (bất kỳ giá nào tăng, hoặc `discount_pct` giảm) cho gói đang có ít nhất một thuê bao trả phí hiệu lực (`active`, `past_due`) đòi `effective_from` không sớm hơn `now + PLATFORM_PRICE_NOTICE_DAYS` (mặc định 30 ngày); vi phạm trả 409 `price_notice_too_short` kèm `min_effective_from`. Giảm giá, gói mới và gói chưa có thuê bao trả phí có hiệu lực ngay: `effective_from` bằng hiện tại và được áp trong chính request.
8. **Áp dụng.** Việc nền `plan_price_apply` (hoặc chính request khi `effective_from` đã tới) áp một thay đổi trong một giao dịch: khóa dòng `plan`, ghi giá mới, tăng `version`, đặt `state = applied` và `applied_at`, ghi kiểm toán, xóa cache chính sách của tiến trình (5.5). Chạy lại là lũy đẳng. Các bản platform khác thấy giá mới trong tối đa 30 giây (cache 5.5).
9. **Phạm vi áp dụng.** (a) Đơn đã tạo giữ số tiền (`billing_order.amount` đã cố định). (b) Kỳ đã trả không đổi. (c) Mọi đơn `subscribe`, `renew`, `add_seats`, `change_plan` tạo từ `effective_from` dùng giá mới; gia hạn sớm trước hiệu lực vẫn được giá cũ, và đây là hành vi chấp nhận. (d) Giá theo mức dùng: ngày D dùng giá có hiệu lực lúc 00:00 UTC của D; vì vậy thay đổi `usage_rates` hoặc `usage_terms` phải có `effective_from` đúng 00:00 UTC, không thì 400 `price_invalid` với `field = effective_from`. Tra giá của một ngày đã qua: lấy `old` của thay đổi `applied` sớm nhất có `effective_from` sau 00:00 UTC của ngày đó; nếu không có thì lấy giá hiện hành của `plan`.
10. **Báo giá.** `purchase-preview` và `plan-changes/preview` trả `purchase_version`. Khi mua mà số tiền tính lại khác `accepted_proration_amount` hay `accepted_amount` (vì giá vừa đổi), trả 409 `quote_changed`. `GET /prices` luôn đọc giá hiện hành.
11. **Báo khách.** Khi lập thay đổi tăng giá, `plan_price_apply` gửi email cho owner của mỗi workspace có thuê bao trả phí hiệu lực của gói đó, đúng một lần (`billing_notice` kind `price_change`, `period_end = effective_from`), trừ workspace `pending_deletion`. `GET /api/workspaces/{id}/plans` chỉ trả `scheduled_price_change` (hiệu lực và giá mới) cho gói mà workspace đang dùng, và chỉ sau khi thư báo đã gửi.
12. **Hủy.** Admin hủy thay đổi còn `scheduled` bằng `DELETE /plans/{key}/price-changes/scheduled`; không có thì 404 `price_change_not_found`. Nếu thư báo đã gửi, hệ thống gửi một thư đính chính (`billing_notice` kind `price_change_canceled`).
13. **Giá thương lượng riêng** là một gói `is_public = false` với key riêng (`POST /plans`); staff `billing` hoặc `admin` gán thuê bao vào gói đó bằng `PUT /tenants/{id}/subscription` (5.4). Không có ghi đè giá theo workspace.
14. Gói đã có thuê bao chỉ lưu trữ được, không xóa (5.5). Lưu trữ không đổi thuê bao hiện có.
15. Mọi lần lập, áp và hủy ghi kiểm toán với giá cũ và mới.

**Dữ liệu.** `plan_price_change` (platform ghi): `id`, `plan_key`, `state` (`scheduled`, `applied`, `canceled`), `effective_from`, `old` (JSONB), `new` (JSONB), `reason`, `notice_sent_at`, `created_by`, `created_at`, `applied_at`, `canceled_by`, `canceled_at`. Chỉ mục duy nhất `(plan_key)` có điều kiện `state = 'scheduled'`. Bảng nhỏ (số dòng bằng số lần đổi giá), nên tra theo `plan_key` không cần chỉ mục thứ hai. Migration 964 (bảng), 965 (chỉ mục). Sổ đăng ký cột người dùng: `created_by` và `canceled_by` là `keep`. Cột `plan.version` ở 7b.1.

**API, vai trò, mã lỗi.**

| Route (console, dưới `/api/platform`) | Vai trò | Nội dung |
| --- | --- | --- |
| `GET /plans` | `admin` | danh mục gói (đã có ở 5.4) |
| `POST /plans` | `admin` | tạo gói, kể cả gói riêng |
| `PUT /plans/{key}` | `admin` | trường không phải giá; `expected_version` |
| `GET /plans/{key}/price-changes` | `billing`, `admin` | lịch sử và thay đổi đang chờ |
| `POST /plans/{key}/price-changes` | `admin` | `effective_from`, `new`, `reason`, `confirm_large_change`; có `?dry_run=1` trả số thuê bao bị ảnh hưởng và ngày gửi thư báo mà không lưu |
| `DELETE /plans/{key}/price-changes/scheduled` | `admin` | quy tắc 12 |

Route của Multica đổi: `GET /api/workspaces/{id}/plans` thêm `scheduled_price_change` (quy tắc 11).

| Mã | HTTP | Trường thêm |
| --- | --- | --- |
| `price_invalid` | 400 | `field` |
| `price_field_via_schedule` | 400 | `fields[]` |
| `price_change_pending` | 409 | |
| `price_change_not_found` | 404 | |
| `price_change_confirm_required` | 409 | `change_pct` |
| `price_notice_too_short` | 409 | `min_effective_from` |
| `plan_currency_locked` | 409 | |
| `plan_version_conflict` | 409 | `version` |

**Màn hình.**

- **Console**: màn hình "Gói" cho `admin`: bảng gói, biểu mẫu sửa thuộc tính, biểu mẫu lập lịch giá với bước xem trước tác động (số thuê bao, ngày gửi thư báo), lịch sử đổi giá; `billing` thấy lịch sử.
- **Multica**: bộ chọn gói và trang thuê bao hiện giá sắp đổi cho owner và người có `billing.view` khi có `scheduled_price_change`.

**Kiểm toán.**

| Nhóm | Hành động |
| --- | --- |
| Giá | `plan.price_scheduled`, `plan.price_applied`, `plan.price_canceled` (cùng ghi `plan_key`, `effective_from`, giá cũ, giá mới, lý do) |
| Thuộc tính | `plan.changed` (đã có) cho sửa tại chỗ và tạo, lưu trữ gói |

**Tương tác với cổng hạn mức, cổng nhận task và trạng thái workspace.**

- Cổng hạn mức: đổi giá không đổi giới hạn. Cache chính sách 30 giây của platform và 5 phút của client quyền lợi (5.5) không ảnh hưởng tiền, vì tiền tính từ `plan` lúc tạo đơn, không từ chính sách quyền lợi.
- Cổng nhận task (D25) và trạng thái workspace: không đổi. Thư báo gửi cho workspace ở mọi trạng thái trừ `pending_deletion`.

**Bảo mật và riêng tư.**

| Mối nguy | Biện pháp |
| --- | --- |
| Gõ nhầm giá (thừa hay thiếu chữ số) | Quy tắc 4, 5; `?dry_run=1` hiện tác động trước khi lưu |
| Hai admin sửa cùng lúc | Khóa dòng `plan` và chỉ mục duy nhất có điều kiện; `expected_version` cho thuộc tính |
| Đổi giá ngầm hay ngược dòng | Báo trước khi tăng (quy tắc 7); không đổi đơn đã tạo hay kỳ đã trả (quy tắc 9); không sửa dòng đã `applied` |
| Admin bị chiếm hay lạm quyền | Chỉ `admin`, có MFA từ R3 (6.2), mọi bước ghi kiểm toán với lý do; giá thương lượng riêng đi qua gói riêng nên mỗi lần gán thuê bao có `subscription.changed` |
| Khách thấy một giá, bị tính giá khác | Số tiền tính ở máy chủ lúc tạo đơn; báo giá hết hiệu lực trả 409 `quote_changed` thay vì tính theo giá cũ |
| Đồng hồ máy lệch làm giá áp sớm hay muộn | Dùng giờ cơ sở dữ liệu (UTC) cho `effective_from` và áp dụng; ngày dùng theo UTC |

**Tiêu chí chấp nhận.** Mục 13, đợt R5, nhóm 4.

### 7b.5 Nhóm 5: admin nền tảng tạo tài khoản cho người dùng (K22)

**Mục tiêu và người dùng.**

- **Admin nền tảng** (vai trò `admin`): tạo tài khoản cho khách mua ngoài kênh tự đăng ký (hợp đồng, bán trực tiếp), cá nhân hoặc doanh nghiệp, kèm workspace và thuê bao ban đầu.
- **Khách**: nhận một thư chào, đăng nhập bằng mã gửi qua email như mọi người dùng, và vào thẳng workspace đã sẵn sàng.
- Staff không đăng nhập thay khách và không giữ thông tin xác thực của khách (D10).

**Quy tắc.**

1. Chỉ staff `admin` (có MFA từ R3) tạo yêu cầu, hủy yêu cầu và gửi lại thư chào. Mỗi staff tạo tối đa `PLATFORM_ACCOUNT_CREATE_PER_HOUR` yêu cầu mỗi giờ (đếm ở Redis); vượt thì 429 `account_create_rate_limited`.
2. Mỗi yêu cầu tạo đúng một người dùng và một workspace do người đó làm owner. Loại `individual`: workspace cá nhân. Loại `business`: workspace doanh nghiệp kèm hồ sơ thanh toán. Không có tạo hàng loạt và không có thêm thành viên vào workspace có sẵn; owner mời thành viên bằng đường lời mời hiện có (ghế, 5.6).
3. Đầu vào: `kind`, `email` (chuẩn hóa chữ thường, tối đa 254 ký tự, đúng định dạng), `display_name` (1 đến 100 ký tự), `workspace_name` (1 đến 100), `workspace_slug` (tùy chọn), `plan_key`, `seats`, `interval`, `trial` hoặc `comp_reason`, `billing_profile` (bắt buộc với `business`, đủ trường như 7b.1 quy tắc 3). Sai thì 400 `account_request_invalid` kèm `fields[]`.
4. **Email.** Email đã thuộc một `user` thì 409 `account_email_exists`; hệ thống không bao giờ sửa hay gắn vào tài khoản có sẵn. Email thuộc `SIGNUP_BLOCKED_EMAIL_DOMAINS` bị từ chối ở bước 2 (cấu hình nằm ở Multica): yêu cầu chuyển `failed` với `error_code = email_domain_blocked`.
5. Mỗi email có tối đa một yêu cầu đang sống (`pending` hoặc `provisioned`). Gửi lại cùng email khi yêu cầu đang sống trả lại chính yêu cầu đó với 200, không tạo yêu cầu thứ hai.
6. **Gói và kích hoạt.** `plan_key` mặc định là `free` cho `individual` và `business` cho `business`. Gói tồn tại, chưa lưu trữ, và `audience` hợp `kind` (409 `plan_audience_mismatch`). `seats` không dưới `min_seats` và, với `individual`, không quá `PLATFORM_INDIVIDUAL_MAX_SEATS`. Gói `free` không tạo thuê bao. Gói `seat` trả phí đòi một trong hai: `trial = true` (gói phải có `trial_days` dương; thuê bao `trialing`) hoặc `comp_reason` (cấp miễn phí đúng một kỳ `interval`; thuê bao `active`); thiếu cả hai thì 400 `account_activation_required`. Gói `usage` kích hoạt ngay như 7b.3 quy tắc 6, không `trial` và không `comp_reason`.
7. **Cấp phát qua ba bước** (D47). Trạng thái yêu cầu: `pending`, `provisioned`, `completed`, `failed`, `canceled`.
   - *Bước 1, console.* Platform kiểm đầu vào, ghi yêu cầu `pending` và trả 202 cùng `id`.
   - *Bước 2, Multica, việc nền `account_provisioning` mỗi phút.* Lấy tối đa 20 yêu cầu `pending` bằng `FOR UPDATE SKIP LOCKED`. Mỗi yêu cầu một giao dịch: kiểm lại email chưa có và miền không bị chặn; tạo `user` với tên là `display_name`; đánh dấu đã onboard (`MarkUserOnboarded`) để người dùng vào thẳng workspace; tạo workspace bằng `workspaceprovision.Create` (cùng mã với `CreateWorkspace`: thêm owner, `issuestatus.Ensure`, kiểm toán `workspace.created`); ghi `user_id`, `workspace_id` và đặt `provisioned`. `workspace_slug` do staff đặt mà trùng hay thuộc danh sách dành riêng thì `failed` với `error_code = slug_unavailable`; nếu không có `workspace_slug`, slug suy từ tên (bỏ dấu, chữ thường, gạch nối), trùng hay dành riêng thì thêm `-` và 4 ký tự ngẫu nhiên, thử tối đa 5 lần rồi `failed`.
   - *Bước 3, platform, việc nền `account_request_finalize` mỗi phút.* Mỗi yêu cầu `provisioned` một giao dịch: ghi `billing_profile` (với `business`, đặt `verified_at` là hiện tại và `verified_by` là staff tạo yêu cầu); tạo thuê bao theo quy tắc 6, kèm một đơn 0 đồng `provider = admin_grant`, `confirmed_by` là staff, trạng thái `paid`; tăng `version`; xóa cache; kiểm toán; xóa bản sao `billing_profile` trong yêu cầu; đặt `completed`. Sau commit, gửi thư chào (quy tắc 9).
   - Lỗi tạm thời được thử lại tối đa 3 lần (`attempts`), cách nhau ít nhất một phút, rồi `failed` với `error_code = internal`. Lỗi không thử lại được thì `failed` ngay. Nếu bước 3 `failed` sau khi bước 2 đã tạo người dùng, `error_code = finalize_failed`: người dùng và workspace được giữ nguyên, staff hoàn thiện thuê bao bằng chức năng thuê bao thường (5.4).
8. **Không có thông tin xác thực.** Không mật khẩu, không phiên, không mã đăng nhập tạo sẵn. Staff không là thành viên của workspace và không đọc được nội dung (D10, I6). Khách đăng nhập bằng mã gửi qua email như mọi người; vì `user` đã tồn tại, lần đăng nhập đầu không đi qua `checkSignupAllowed` (`server/internal/handler/auth.go:270`), nên dùng được cả khi `ALLOW_SIGNUP` tắt.
9. **Thư chào.** Platform gửi bằng bộ gửi email của mình (`internal/mailer`, D34) tới `email`. Nội dung: tên workspace, tên gói, địa chỉ web app (`PLATFORM_APP_URL`) và chỉ dẫn nhập email để nhận mã. Không chứa mã, mật khẩu hay liên kết có token. Gửi đúng một lần (`welcome_sent_at`); gửi lỗi thì thử lại ở nhịp kế tối đa 3 lần (`welcome_attempts`). Staff gửi lại bằng route riêng, tối đa 3 lần mỗi ngày mỗi yêu cầu.
10. Tạo tài khoản không thay cho sự đồng ý của người dùng: lần đăng nhập đầu vẫn qua `consent_required` (4.4).
11. Đây là tài khoản thường: guard tài khoản (4.1), khóa, xóa và xuất dữ liệu hoạt động như mọi tài khoản. Xóa tài khoản (4.6) ẩn danh hóa `email` và `display_name` của mọi `platform_account_request` có `user_id` đó.
12. Chỉ hủy được yêu cầu `pending` (409 `account_request_not_cancelable`).
13. Owner của workspace thấy dòng `account.provisioned` (actor là `staff`) trong nhật ký của mình, nên nhà vận hành không tạo tài khoản mà khách không biết (5.2).

**Dữ liệu.** `platform_account_request`: `id`, `kind`, `email`, `display_name`, `workspace_name`, `workspace_slug`, `plan_key`, `seats`, `interval`, `trial`, `comp_reason`, `billing_profile` (JSONB), `state`, `error_code`, `attempts`, `user_id`, `workspace_id`, `created_by`, `created_at`, `updated_at`, `completed_at`, `welcome_sent_at`, `welcome_attempts`. Chỉ mục duy nhất `(id)` và `(email)` có điều kiện `state IN ('pending', 'provisioned')`. Bảng nhỏ do con người tạo; việc nền lọc theo `state` trên bảng này không cần chỉ mục thứ ba. Migration 966 (bảng), 967 và 968 (hai chỉ mục). Sổ đăng ký cột người dùng: `created_by` và `user_id` là `keep`; `email` và `display_name` được ẩn danh hóa bởi bước xóa tài khoản (quy tắc 11).

Ngoại lệ về người ghi (bổ sung cho mục 8): Multica ghi các cột `state`, `user_id`, `workspace_id`, `error_code`, `attempts`, `updated_at` của `platform_account_request` ở bước 2; mọi cột khác do platform ghi.

**API, vai trò, mã lỗi.**

| Route (console, dưới `/api/platform`) | Vai trò | Nội dung |
| --- | --- | --- |
| `GET /accounts`, `GET /accounts/{id}` | `admin` | danh sách và trạng thái yêu cầu, kèm `error_code`; `user_id` và `workspace_id` khi đã có |
| `POST /accounts` | `admin` | tạo yêu cầu; 202; có `Idempotency-Key` |
| `POST /accounts/{id}/cancel` | `admin` | quy tắc 12 |
| `POST /accounts/{id}/resend-welcome` | `admin` | quy tắc 9 |

| Mã | HTTP | Trường thêm |
| --- | --- | --- |
| `account_request_invalid` | 400 | `fields[]` |
| `account_activation_required` | 400 | |
| `account_email_exists` | 409 | |
| `account_request_not_cancelable` | 409 | `state` |
| `account_create_rate_limited` | 429 | header `Retry-After` |

Giá trị `error_code` của yêu cầu `failed`: `email_exists` (người dùng đăng nhập tạo tài khoản trước khi việc nền chạy), `email_domain_blocked`, `slug_unavailable`, `finalize_failed`, `internal`.

**Màn hình.**

- **Console**: màn hình "Tạo tài khoản" cho `admin`: biểu mẫu (loại, email nhập hai lần để tránh gõ nhầm, tên, workspace, gói, ghế, kích hoạt, hồ sơ doanh nghiệp) và danh sách yêu cầu với trạng thái, lỗi, nút hủy và gửi lại thư chào.
- **Multica**: không có màn hình mới. Khách thấy workspace đã có sẵn ở lần đăng nhập đầu.

**Kiểm toán.**

| Nhóm | Hành động |
| --- | --- |
| Yêu cầu | `account.provision_requested` (actor `staff`, không có `workspace_id`, nhãn đích là email), `account.provision_canceled`, `account.provision_failed` |
| Kết quả | `account.provisioned` (actor `system`, có `workspace_id`; `detail` ghi staff tạo yêu cầu, `kind`, `plan_key`) |
| Thư chào | `account.welcome_sent`, `account.welcome_resent` |

Bước 3 còn ghi `subscription.changed`, `billing.order_created`, `billing.order_paid` (đã có) và `billing_profile.verified` (7b.1).

**Tương tác với cổng hạn mức, cổng nhận task và trạng thái workspace.**

- Cổng hạn mức: workspace mới theo giới hạn của gói đã chọn. `FREE_PLAN_OWNED_WORKSPACE_LIMIT` (5.7) không cản vì đây là workspace đầu của người dùng mới. Ghế: owner là thành viên đầu tiên, trong `min_seats`.
- Cổng nhận task (D25) và trạng thái workspace: workspace tạo ra ở trạng thái `active` (không có dòng `platform_workspace`).
- Guard tài khoản: không có dòng `platform_account` nghĩa là `active` (4.1).

**Bảo mật và riêng tư.**

| Mối nguy | Biện pháp |
| --- | --- |
| Staff giữ quyền vào tài khoản của khách | Không mật khẩu, không phiên, không mã sẵn; staff không là thành viên (quy tắc 8); test khẳng định không có dòng `member` nào cho staff và không có phiên nào được cấp |
| Gõ nhầm email nên người lạ nhận workspace | Nhập email hai lần; workspace trống; thiệt hại tối đa là một gói miễn phí hoặc dùng thử; `comp_reason` bắt buộc cho gói cấp miễn phí |
| Tạo hàng loạt tài khoản rác để lấy gói miễn phí | Chỉ `admin`, có MFA, hạn mức theo giờ (quy tắc 1), mỗi bước ghi kiểm toán |
| Chiếm hay sửa tài khoản đã có | Quy tắc 4: không bao giờ động vào `user` có sẵn |
| Hai việc nền cùng tạo một người dùng | `FOR UPDATE SKIP LOCKED`; ràng buộc duy nhất của `user.email`; chỉ mục duy nhất của yêu cầu đang sống; giao dịch một yêu cầu |
| Người thật đăng nhập trước khi việc nền chạy | Yêu cầu chuyển `failed` với `email_exists`; không có dữ liệu nào bị hỏng |
| Dữ liệu cá nhân của người thứ ba | Lưu tối thiểu; xóa bản sao hồ sơ sau khi hoàn tất; ẩn danh hóa khi xóa tài khoản; thư chào nêu rõ ai tạo tài khoản và vì sao |
| Thư chào bị lợi dụng để lừa đảo | Không có liên kết ngoài địa chỉ web app đã cấu hình; không đòi nhập thông tin; gửi từ cấu hình email của nhà vận hành |
| Tài khoản tạo ra mà không ai dùng | Hiển thị "chưa đăng nhập lần nào" (có hay không dòng `auth.login`) ở console; chưa có dọn tự động (rủi ro ở 15.3) |

**Tiêu chí chấp nhận.** Mục 13, đợt R5, nhóm 5.

### 7b.6 Việc nền, cấu hình, điểm móc và migration của R5

Việc nền mới đã thêm vào bảng ở 3.5: `usage_rating`, `usage_invoicing`, `plan_price_apply`, `account_request_finalize` (platform), `account_provisioning` (Multica). `subscription_lifecycle` (5.9) được mở rộng ở nhóm 3 cho đơn `usage` chưa trả. Biến cấu hình mới đã thêm vào bảng platform ở 3.4; Multica không có biến mới. Điểm móc mới vào mã lõi đã thêm vào bảng ở 3.3.

Platform lúc khởi động (5.1) kiểm thêm các phiên bản migration sau, và danh sách của `platform-management/internal/schemacheck` được thêm đúng các phiên bản này: 953 (cột mới của `plan` và `workspace_subscription`), 954 và 955 (`billing_profile`), 961 đến 963 (`usage_charge`), 964 và 965 (`plan_price_change`), 966 đến 968 (`platform_account_request`). Các bảng `member_group` và `member_group_member` thuộc Multica nên không nằm trong danh sách này.

| Migration | Nội dung | Nhóm |
| --- | --- | --- |
| 953 | thêm `audience`, `billing_model`, `usage_rates`, `usage_terms`, `version` vào `plan`; thêm `pending_change`, `unit_price` vào `workspace_subscription` | 1, 3, 4 |
| 954, 955 | bảng `billing_profile`; chỉ mục duy nhất `(workspace_id)` | 1 |
| 956, 957, 958 | bảng `member_group`; chỉ mục duy nhất `(workspace_id, id)`; chỉ mục duy nhất `(workspace_id, lower(name))` | 2 |
| 959, 960 | bảng `member_group_member`; chỉ mục duy nhất `(workspace_id, user_id, group_id)` | 2 |
| 961, 962, 963 | bảng `usage_charge`; chỉ mục duy nhất `(workspace_id, day, meter)`; chỉ mục `(order_id)` có điều kiện | 3 |
| 964, 965 | bảng `plan_price_change`; chỉ mục duy nhất `(plan_key)` có điều kiện `state = 'scheduled'` | 4 |
| 966, 967, 968 | bảng `platform_account_request`; chỉ mục duy nhất `(id)`; chỉ mục duy nhất `(email)` có điều kiện | 5 |
| 969 | dự phòng: chỉ dùng nếu migration của P21 đặt `CHECK` trên `billing_order.kind` (Q10) | 3 |

Mọi chỉ mục tạo bằng `CREATE [UNIQUE] INDEX CONCURRENTLY IF NOT EXISTS`, mỗi chỉ mục một tệp một câu lệnh, có dòng dọn trong `concurrentIndexCleanups` (I3, kế hoạch 0.2). Migration 953 chỉ có `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` với giá trị mặc định không làm khóa bảng lâu.

### 7b.7 Giá trị cần chủ dự án đặt

Các giá trị dưới đây là quyết định thương mại hoặc quyết định chính sách của chủ dự án. Mỗi giá trị có mặc định hợp lý để triển khai và thử nghiệm, nằm trong dữ liệu gói hoặc biến cấu hình (D48), nên đổi không cần sửa mã. Tất cả là lựa chọn chưa đo (mục 15.2).

| Giá trị | Mặc định | Nằm ở | Vì sao chủ dự án phải đặt |
| --- | --- | --- | --- |
| Đơn giá ghế gói Unlimited | 249.000 mỗi tháng; 2.490.000 mỗi năm (VND) | tệp gói nhúng, sau đó `plan_price_change` | giá bán |
| Bảng giá gói `payg` | 30.000 cho 1.000 task; 3.000 cho 1 GiB-tháng; 49.000 cho 1 thành viên-tháng | `usage_rates` | giá bán và chi phí vận hành thật của nhà vận hành |
| Tỷ lệ giảm Business | 20 phần trăm | `usage_terms.discount_pct.business` | quyết định thương mại |
| Hạn mức công nợ | `individual` 2.000.000; `business` 20.000.000 (VND) | `usage_terms.credit_limit` | mức rủi ro tín dụng nhà vận hành chấp nhận |
| Bắt xác minh mới được giảm giá | `true` | `PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION` | cân bằng giữa chống gian lận và công sức xác minh |
| Hạn trả hóa đơn, hóa đơn tối thiểu | 7 ngày; 10.000 VND | `PLATFORM_USAGE_DUE_DAYS`, `PLATFORM_USAGE_MIN_INVOICE` | chính sách công nợ |
| Trần tốc độ API | 6.000 mỗi phút | `PLATFORM_API_RATE_CEILING` | năng lực hạ tầng |
| Tốc độ API của Unlimited và `payg` | 3.000 và 1.200 mỗi phút | `limits` của gói | chính sách gói |
| Số ghế tối đa của workspace cá nhân | 5 | `PLATFORM_INDIVIDUAL_MAX_SEATS` | ranh giới giữa cá nhân và doanh nghiệp |
| Số nhóm tối đa mỗi gói | Free 0, Pro 0, Business 50, Enterprise không giới hạn | `limits.member_groups` của gói | chính sách gói |
| Báo trước tăng giá; ngưỡng xác nhận; giá tối đa | 30 ngày; 50 phần trăm; 1.000.000.000 | `PLATFORM_PRICE_NOTICE_DAYS`, `PLATFORM_PRICE_CONFIRM_PCT`, `PLATFORM_PRICE_MAX` | nghĩa vụ báo trước với khách (mục 16) |
| Hạn mức tạo tài khoản | 30 yêu cầu mỗi giờ mỗi staff | `PLATFORM_ACCOUNT_CREATE_PER_HOUR` | mức tin cậy với staff |
| Gói mặc định khi tạo tài khoản | `free` cho cá nhân; `business` dùng thử cho doanh nghiệp | quy tắc 6 của 7b.5 | chính sách bán hàng |
| Năm diễn giải A1 đến A5 | như bảng 7b.0 | mục 7b.0 | chỉ chủ dự án biết ý định gốc |

## 8. Mô hình dữ liệu

Tất cả là bảng mới, trừ ba thay đổi trên bảng đã có ở R3. Không khóa ngoại. Không `PRIMARY KEY` hay `UNIQUE` nội tuyến; tính duy nhất do chỉ mục `UNIQUE` tạo riêng bảo đảm (I3).

Mọi bảng nằm trong cơ sở dữ liệu chung của Multica và platform (D33), theo một chuỗi migration duy nhất ở `server/migrations/`. Dịch vụ platform sở hữu các bảng `platform_*`, `plan`, `workspace_subscription`, `billing_*`, `workspace_export`, `platform_usage_daily` và `platform_seat_operation`: schema và truy vấn của console thuộc về nó, và nó kiểm các phiên bản migration của chúng lúc khởi động (5.1). Ngoại lệ về người ghi: Multica ghi `platform_account` ở thao tác của chính người dùng (đăng xuất mọi thiết bị, xóa tài khoản; R1), ghi `platform_workspace` ở xóa có thời gian chờ (5.3), ghi cột `buyer` của `billing_order` (5.8), và chạy việc nền `workspace_export` và `platform_usage_daily` vì chúng đọc dữ liệu workspace (3.5). Các bảng còn lại (`audit_event`, `user_consent`, `user_mfa`, `sso_*`, `workspace_policy`) do Multica sở hữu; platform ghi thêm vào `audit_event` qua gói dùng chung `audit`.

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

**Bảng và cột của R5** nằm ở 7b.1 đến 7b.5, theo đúng các quy tắc trên (không khóa ngoại, chỉ mục duy nhất tạo riêng). Đợt R5 giữ dải dự phòng **953 đến 969** (17 số), vì ba lý do: dải 651 đến 699 đã được phân bổ hết cho R1 đến R3 và một phần đã dùng (686 và 688 đã có tệp); các số 700 đến 709 và 720 đến 739 đã bị công việc khác chiếm ở cây làm việc hiện tại, nên không còn trống; và dải 953 đến 969 là dải liền duy nhất đủ 17 số, đứng sau mọi bảng mà R5 đổi (`plan` ở 670, `workspace_subscription` ở 672). Phân bổ chi tiết ở bảng 7b.6; 16 số dùng, số 969 dự phòng. Ngoại lệ về người ghi của R5: Multica ghi một số cột của `platform_account_request` ở bước 2 của 7b.5.

## 9. API

Chỉ liệt kê route mới và route đổi hành vi. Mọi phản hồi cho giao diện có schema zod ở client và test phản hồi sai dạng. Route ở bảng là của máy chủ Multica, trừ ba dòng ghi rõ platform.

| Route | Quyền | Đợt |
| --- | --- | --- |
| `GET /api/workspaces/{id}/invitations` | **đổi**: owner, admin | R1 |
| `POST /api/me/sessions/revoke-all` | người thật | R1 |
| `GET /api/me/security-events` | người thật | R1 |
| `GET /api/workspaces/{id}/audit-events` | owner, admin | R1 |
| `POST /api/me/consents` | người thật | R1 |
| `GET /api/me/export` | người thật | R1 |
| `POST /api/me/deletion`, `DELETE /api/me/deletion` | người thật, vừa đăng nhập lại | R1 |
| `GET /api/me` | **thêm trường tùy chọn** `consent_required`, `mfa_enabled` | R1, R3 |
| `/api/platform/*` (mục 5.4), trên API platform | staff, cookie `mpa_session` | R2 |
| `/api/cloud-subscriptions/*` | Multica: giữ nguyên, chuyển tới `MULTICA_CLOUD_URL` (sửa nhỏ ở mục 3.3). Phiên bản của platform: phục vụ `/api/v1/subscriptions/…` với phản hồi theo Phụ lục A | R2 |
| `GET /api/v1/internal/entitlement-policies/{id}`, `/api/v1/internal/subscriptions/{id}/capacity/*`, `/api/v1/subscriptions/*`, trên API platform | không xác thực người dùng (tin `X-User-ID` do Multica đặt); chỉ mạng nội bộ (mục 11) | R2 |
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

Route của R5 (mục 7b) không lặp lại ở bảng này; mỗi nhóm có bảng route riêng: 7b.1 (`/api/cloud-subscriptions/billing-profile`, `plan-changes`, `cancel`, `resume`), 7b.2 (`/api/workspaces/{id}/groups`, `permissions/me`), 7b.3 (`/api/cloud-subscriptions/usage`, `.../orders/{orderId}/usage`), 7b.4 (`/api/platform/plans/{key}/price-changes`), 7b.5 (`/api/platform/accounts`). Các route nội bộ mới của platform dưới `/api/v1/subscriptions/{id}/…` theo cùng quy tắc mạng nội bộ như route đã có (mục 11).

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

Mã lỗi của R5 ghi cùng route ở bảng của từng nhóm: 7b.1 (`plan_audience_mismatch`, `billing_profile_invalid`, `billing_profile_incomplete`, `account_kind_required`, `order_pending`, `plan_change_unavailable`, `subscription_not_cancelable`, `quote_changed`), 7b.2 (`permission_denied`, `invite_role_not_assignable`, `group_name_taken`, `group_not_found`, `not_a_member`, `group_member_limit`, `permission_unknown`), 7b.3 (`usage_invoice_unpaid`), 7b.4 (`price_invalid`, `price_field_via_schedule`, `price_change_pending`, `price_change_not_found`, `price_change_confirm_required`, `price_notice_too_short`, `plan_currency_locked`, `plan_version_conflict`), 7b.5 (`account_request_invalid`, `account_activation_required`, `account_email_exists`, `account_request_not_cancelable`, `account_create_rate_limited`). Cùng quy ước: client dịch theo `code`.

## 11. Bảo mật

| Mối nguy | Biện pháp |
| --- | --- |
| Tài khoản staff bị chiếm | Phiên `mpa_session` đăng nhập trong 12 giờ, cookie `HttpOnly`, `Secure`, `SameSite=Strict`, chỉ trên origin console; mỗi request kiểm lại dòng `platform_staff` và trạng thái tài khoản; không nhận PAT hay token máy; giới hạn IP tùy chọn; MFA bắt buộc từ R3; mọi hành động ghi kiểm toán và tenant thấy được |
| Route nội bộ của platform bị gọi từ ngoài | API 8095 không publish ra ngoài mạng nội bộ; console chỉ chuyển tiếp `/api/platform/`; compose không map 8095 ra máy chủ (mục 12). Các route nội bộ tin `X-User-ID` và đường dẫn workspace do Multica gửi, nên lộ chúng ra ngoài là mất cưỡng chế quyền |
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

- **Mặc định tắt.** Không đặt biến nào thì chỉ có R1 hoạt động, và R1 không đổi hành vi cho đến khi có dòng trong `platform_account`. Không chạy platform và để `MULTICA_CLOUD_URL` rỗng là cấu hình tự lưu trữ hiện tại.
- **Image platform.** `multica-platform`, build từ `platform-management/Dockerfile` (nhiều giai đoạn: dựng console bằng pnpm trong workspace, dựng `cmd/platform-server`, ảnh cuối chạy một tiến trình). Cổng: API 8095 (chỉ mạng nội bộ; Multica gọi `http://platform:8095`) và console 30001 (mở cho staff). Web app Multica 30000 và landing 30002 không đổi.
- **Kiểm tra khi khởi động (D33).** Platform không chạy migration. Nó kiểm `schema_migrations` và thoát nếu thiếu một phiên bản bắt buộc (5.1). Thứ tự triển khai: `server/cmd/migrate up`, rồi Multica và platform. Migration chỉ thêm (I2), nên bản platform cũ chạy được với schema mới hơn.
- **Compose.** Thêm service `platform` vào compose triển khai SaaS (`docker-compose.selfhost.yml`), cạnh `landing`: image `multica-platform`, phụ thuộc Postgres và Redis, đặt `restart: unless-stopped` để nó tự thử lại cho tới khi migrate xong; chỉ publish cổng 30001. Multica đặt `MULTICA_CLOUD_URL=http://platform:8095`.
- **Client cũ.** Trường mới của `GET /api/me` là tùy chọn. Mã lỗi mới đi kèm mã HTTP mà client cũ đã xử lý (401 về trang đăng nhập; 403 và 402 hiện lỗi chung). Vai trò `viewer` trên client cũ: mục 6.6.
- **Mobile.** Mã riêng (`apps/mobile/`). R1 cần mobile xử lý `session_revoked` và `account_suspended`; R3 cần luồng MFA, SSO và vai trò `viewer`. Đọc `apps/mobile/AGENTS.md` trước khi sửa.
- **Bật thương mại cho hệ thống đang chạy.** Trỏ `MULTICA_CLOUD_URL` về API platform khi đã có workspace: mọi workspace chưa có thuê bao là gói miễn phí. Để không ai bị chặn đột ngột, bước triển khai gán cho workspace hiện hữu một thuê bao chuyển tiếp không giới hạn có hạn, do staff đặt trước khi đổi `MULTICA_CLOUD_URL`. Trình tự này là một bước riêng trong kế hoạch.
- **Triển khai cuốn chiếu.** Claim JWT mới chỉ được thêm; bản cũ bỏ qua claim lạ. Khóa cache PAT đổi tiền tố (6.5).
- **Chuỗi giao diện** đủ bảy ngôn ngữ của web và desktop, cùng bản dịch riêng của mobile; không viết mô tả thừa (AGENTS.md "UI Copy").

## 13. Kiểm thử và tiêu chí chấp nhận

**Nguyên tắc.** Test viết trước, đỏ rồi xanh, đặt cạnh mã. Test Go chạm DB dùng `server/internal/testutil` và khẳng định trên dòng fixture của chính test, không trên tổng bảng (AGENTS.md "Testing"). Test mặc định không gọi agent thật.

**Bộ test bắt buộc ngoài test từng bước:**

1. **Ma trận token × trạng thái tài khoản**: năm loại token, bốn trạng thái, trên cả `Auth`, `DaemonAuth` và WebSocket.
2. **Ma trận trạng thái workspace × phương thức × danh sách cho phép**, trên cả đường middleware và đường `requireWorkspaceMember`.
3. **Hợp đồng giao thức ghế**: một kịch bản chung chạy trên bản giả hiện có và trên handler HTTP của platform (qua client ghế hiện có); toàn bộ test lời mời và link chia sẻ hiện có chạy lại với client ghế trỏ về platform.
4. **Hợp đồng JSON thanh toán**: phản hồi của bảy route `/api/cloud-subscriptions/*` của platform đi qua đúng schema zod mà client đang dùng, và khớp Phụ lục A.
5. **Vòng đời thuê bao**: mỗi cạnh của sơ đồ 5.9 với đồng hồ giả; chạy job hai lần liên tiếp không đổi kết quả.
6. **Cô lập tenant của console**: test duyệt handler ở 5.4.
7. **Sổ đăng ký cột người dùng** ở 4.6.
8. **Nhật ký chỉ ghi thêm và lọc bí mật** ở 4.3.
9. **Hồi quy**: toàn bộ test hiện có xanh sau mỗi bước, với mọi cấu hình mới để trống.
10. **Hợp đồng chính sách quyền lợi và kiểm tra schema**: phản hồi của `entitlement-policies` đi qua `normalizePolicy` của client hiện có; platform từ chối khởi động khi thiếu một phiên bản migration bắt buộc.
11. **Ma trận khóa quyền × route** (7b.2): với mỗi khóa trong danh mục, đúng tập route của khóa mở cho người có khóa; không route nào ngoài tập đó mở; viewer trong nhóm đủ sáu khóa vẫn bị chặn ở mọi phương thức ghi; route ngoài danh mục vẫn chỉ owner và admin.
12. **Bảng ca tiền** (7b.1, 7b.3, 7b.4): tỷ lệ nâng gói, chấm tiền theo ngày (ba đồng hồ, tháng 28 đến 31 ngày, giảm giá, số rất lớn), hóa đơn tháng và hóa đơn cuối, tra giá của một ngày đã qua; chạy trên hàm thuần không cần cơ sở dữ liệu.
13. **Hợp đồng JSON mở rộng**: phản hồi các route mới và `summary` có trường tùy chọn mới đi qua schema zod của client; phản hồi **không** có trường mới (Multica Cloud, bản platform cũ) vẫn hợp lệ và client ẩn đúng chức năng.
14. **Cấp phát tài khoản đầu cuối** (7b.5): yêu cầu đi qua ba bước trên cơ sở dữ liệu test với đồng hồ giả; mỗi việc nền chạy hai lần liên tiếp và hai bản song song, kết quả không đổi; không có phiên, mật khẩu hay dòng `member` của staff nào được tạo.

**Tiêu chí chấp nhận theo đợt.**

R1:
- AC1. Staff hoặc lệnh SQL đặt một tài khoản `suspended`; trong vòng 60 giây mọi request bằng JWT, PAT, token task và token daemon của người đó bị từ chối và WebSocket bị ngắt, không cần triển khai lại.
- AC2. Sau "đăng xuất mọi thiết bị", một token cũ không dùng được và không gia hạn được; phiên vừa gọi vẫn dùng được.
- AC3. Mỗi hành động trong danh mục R1 sinh đúng một dòng kiểm toán; làm hỏng việc ghi kiểm toán khiến hành động thất bại và không để lại thay đổi.
- AC4. Thành viên thường nhận 403 khi đọc danh sách lời mời.
- AC5. Xóa tài khoản: bị từ chối khi là owner duy nhất; sau thời gian chờ, bản ghi được ẩn danh hóa, không còn tư cách thành viên, PAT hay kết nối; email cũ đăng ký lại được như người mới.
- AC6. Bản xuất tài khoản không chứa giá trị token hay bí mật.

R2:
- AC7. Dịch vụ platform từ chối khởi động với từng cấu hình sai ở mục 5.1, kể cả khi thiếu một phiên bản migration bắt buộc; máy chủ Multica không đổi hành vi khi `MULTICA_CLOUD_URL` rỗng.
- AC8. Người không có cookie `mpa_session` hợp lệ, hoặc có dòng `platform_staff` đã thu hồi, nhận 404 trên mọi route `/api/platform/*` (trừ hai route nhận mã); PAT, token máy và cookie `multica_auth` không được platform nhận.
- AC9. Workspace `suspended`: thành viên không đọc được dữ liệu, daemon không nhận được task, owner vẫn xem được thanh toán và xuất được dữ liệu.
- AC10. Vượt từng cổng hạn mức trả 402 `plan_limit_reached`; hai request đồng thời ở sát giới hạn chỉ một cái thành công.
- AC11. Mời người khi hết ghế bị từ chối; xác nhận một đơn mua ghế thì mời được ngay.
- AC12. Xác nhận cùng một đơn hai lần chỉ áp một lần.
- AC13. Workspace quá hạn đi đúng chuỗi ân hạn, chỉ đọc, hạ gói theo đồng hồ giả, và trở lại `active` khi thanh toán ở bất kỳ bước nào; không dòng dữ liệu nào bị xóa.
- AC14. Giao diện thanh toán hiện có hiển thị đúng gói, ghế, giá bằng VND khi `MULTICA_CLOUD_URL` trỏ về platform.
- AC20. Khi platform ngừng, đọc dữ liệu workspace vẫn chạy; lời mời và link chia sẻ trả `seat_capacity_unavailable` (503); trạng thái tài khoản và workspace vẫn được cưỡng chế từ DB chung.

R3:
- AC15. Tài khoản có MFA không nhận được phiên nếu chưa qua bước thứ hai; token chờ không gọi được API nào khác.
- AC16. Callback SSO với email ngoài tên miền đã xác minh bị từ chối.
- AC17. Workspace bật `require_sso`: thành viên thuộc tên miền đăng nhập bằng mã email bị chặn ở workspace đó nhưng vẫn vào được workspace khác; owner không bị chặn.
- AC18. PAT gắn workspace A bị từ chối ở workspace B; PAT chỉ đọc bị từ chối ở mọi phương thức ghi.
- AC19. Viewer bị từ chối ở mọi route ghi của workspace (test duyệt bảng route) trừ danh sách cho phép, và không đăng ký được runtime.

R5 (mục 7b). Mỗi tiêu chí là một test chạy được; tiêu chí của nhóm 2 chạy sau G-R3.

Nhóm 1, quản lý thuê bao cá nhân và doanh nghiệp (7b.1):
- AC21. Workspace `individual` mua gói `business` bị 409 `plan_audience_mismatch`; sau khi hồ sơ doanh nghiệp đủ và chuyển sang `business` thì mua được; hồ sơ thiếu trường bị 409 `billing_profile_incomplete`, mã số thuế sai định dạng bị 400 `billing_profile_invalid`; chuyển `business` về `individual` bị từ chối khi đang ở gói chỉ dành cho doanh nghiệp; workspace `individual` đặt quá `PLATFORM_INDIVIDUAL_MAX_SEATS` ghế bị 409 `account_kind_required`.
- AC22. Nâng gói giữa kỳ tạo đúng một đơn `change_plan` với số tiền theo công thức ở 7b.1 quy tắc 5 (đầu kỳ, giữa kỳ, ngày cuối, kỳ năm, ghế nâng lên `min_seats`); lặp cùng `Idempotency-Key` trả cùng đơn; áp thanh toán hai lần chỉ đổi thuê bao một lần; khi đang có một đơn `pending` khác thì 409 `order_pending`; lệch số tiền đã chấp nhận thì 409 `quote_changed`.
- AC23. Hạ gói, giảm ghế và đổi chu kỳ không đổi quyền lợi hiện tại; đơn `renew` kế tiếp tính tiền theo `pending_change` và áp nó khi thanh toán rồi xóa `pending_change`; giảm ghế xuống dưới số thành viên không gỡ ai và workspace ở trạng thái vượt chỗ; chuyển từ gói theo ghế sang gói theo mức dùng chỉ áp ở cuối kỳ đã trả.
- AC24. Hủy rồi tiếp tục: trạng thái đúng ở từng bước; hết kỳ khi cờ hủy còn bật thì về `free` không qua ân hạn; hủy khi `past_due` bị 409 `subscription_not_cancelable`; mỗi thao tác ghi đúng một dòng kiểm toán.
- AC25. Người không có quyền `billing.manage` (không phải owner, admin, thành viên nhóm có khóa) nhận 403 `permission_denied` ở mọi route ghi mới; người không có `billing.view` không đọc được hồ sơ thanh toán; token máy bị 403 ở mọi route mới; hồ sơ không xuất hiện trong `summary` và không trong bản xuất tài khoản; staff `billing` xác minh được doanh nghiệp, `support` không.

Nhóm 2, nhóm tài khoản và quyền (7b.2):
- AC26. Thành viên `member` thuộc nhóm có `members.invite` tạo được lời mời vai trò `member` và `viewer`, bị 403 `invite_role_not_assignable` khi mời `admin`, và không đổi được vai trò hay gỡ được ai; bớt người đó khỏi nhóm thì request kế tiếp đã bị 403 `permission_denied`.
- AC27. Với mỗi khóa trong sáu khóa, đúng tập route đã ghi ở danh mục mở cho người có khóa và không route nào khác mở; route ngoài danh mục (đổi vai trò, gỡ thành viên, cập nhật và xóa workspace, plugin, thư viện MCP, quản lý nhóm) vẫn chỉ owner và admin; người chỉ có quyền nhóm bị chặn ở workspace `suspended` và `pending_deletion` trong khi owner và admin vẫn dùng được route cho phép.
- AC28. Viewer thuộc nhóm có đủ sáu khóa vẫn bị chặn ở mọi phương thức ghi và đọc được những gì khóa đọc cho phép; gỡ thành viên, rời workspace và xóa tài khoản xóa mọi dòng nhóm của người đó trong cùng giao dịch; xóa nhóm xóa dòng thành viên nhóm; test sổ đăng ký cột người dùng thất bại nếu cột `user_id` mới chưa được phân loại.
- AC29. Cổng `member_groups`: giới hạn 0 trả 402 `plan_feature_unavailable`; vượt giới hạn trả 402 `plan_limit_reached`; hai request đồng thời ở sát giới hạn chỉ một cái thành công; hạ gói giữ nguyên nhóm và quyền đang hiệu lực, chỉ chặn tạo nhóm mới; khi `MULTICA_CLOUD_URL` rỗng thì không giới hạn số nhóm.
- AC30. Mỗi thay đổi nhóm, thành viên nhóm và quyền sinh đúng một dòng kiểm toán (quyền có giá trị cũ và mới) mà owner đọc được; làm hỏng việc ghi kiểm toán khiến thay đổi thất bại; dùng id nhóm của workspace khác trả 404 `group_not_found`; tên nhóm trùng không phân biệt hoa thường trả 409 `group_name_taken`; khóa quyền lạ trả 400 `permission_unknown`; staff không đọc được tên nhóm hay thành viên nhóm qua console.

Nhóm 3, gói không giới hạn và gói theo mức dùng (7b.3):
- AC31. Gói `unlimited` không chặn tạo issue, agent, runtime, tệp hay nhóm ở mọi mức thử và trả `action = off` cho các cổng đếm; `api_rate_per_minute` vẫn bị chặn ở `PLATFORM_API_RATE_CEILING` khi gói để trống và ở giá trị tường minh khi staff đặt cao hơn; hạ xuống gói có giới hạn không xóa dữ liệu nào.
- AC32. Kích hoạt gói `payg` cần hồ sơ thanh toán đủ (409 `billing_profile_incomplete` nếu thiếu), tạo thuê bao `active` cùng đơn 0 đồng `paid`, `seats = 0`, `seat_capacity` là `null`; lời mời và link chia sẻ không bị chặn vì ghế; `GET /prices` không bao giờ trả giá của gói theo mức dùng.
- AC33. Chấm tiền theo ngày đúng công thức ở 7b.3 quy tắc 8 cho ba đồng hồ, tháng 28, 29, 30, 31 ngày và số lượng lớn tới 100 TB không tràn số; giảm giá Business chỉ áp khi `account_kind = business` và hồ sơ đã xác minh (khi cờ bật); mỗi (workspace, ngày, đồng hồ) đúng một dòng, chạy `usage_rating` hai lần và song song không sinh dòng thứ hai; ngày thiếu số liệu không bị chấm; ngày workspace `suspended` ghi dòng 0 đồng.
- AC34. Hóa đơn tháng gồm mọi dòng chưa lập hóa đơn của ngày trước đầu tháng; số tiền bằng tổng các dòng, VAT đúng `BILLING_VAT_RATE`; dưới `PLATFORM_USAGE_MIN_INVOICE` thì dồn sang kỳ sau; hóa đơn cuối được lập khi rời gói theo mức dùng; chạy `usage_invoicing` hai lần và song song chỉ lập một đơn; đơn `usage` không tự hết hạn.
- AC35. Đạt hạn mức công nợ thì lập hóa đơn tạm và gửi email báo một lần ở 80 phần trăm; đơn quá hạn đưa thuê bao đi `past_due`, `read_only`, rồi về `free` theo đồng hồ giả như D14 và không dòng dữ liệu nào bị xóa; thanh toán đưa workspace về `active`; khi còn nợ, mua gói trả phí bị 409 `usage_invoice_unpaid` còn gói `free` vẫn dùng được; chỉ staff `admin` kèm lý do xóa được nợ.

Nhóm 4, cấu hình giá từng gói (7b.4):
- AC36. Chỉ `admin` lập, xem trước và hủy thay đổi giá; `billing` đọc được lịch sử; `support` và người không có phiên nhận 404; sửa trường giá bằng `PUT /plans/{key}` bị 400 `price_field_via_schedule`; hai yêu cầu lập đồng thời cho một gói chỉ một cái thành công và cái kia nhận 409 `price_change_pending`.
- AC37. Giá không hợp lệ bị 400 `price_invalid` với `field` đúng (âm, thập phân, giá năm quá 12 lần giá tháng, vượt `PLATFORM_PRICE_MAX`, gói `free` khác 0, khóa đồng hồ lạ, `effective_from` của giá theo mức dùng không đúng 00:00 UTC); đổi quá ngưỡng đòi `confirm_large_change` (409 `price_change_confirm_required`); đổi tiền tệ khi đã có thuê bao bị 409 `plan_currency_locked`.
- AC38. Tăng giá gói đang có thuê bao trả phí đòi `effective_from` không sớm hơn `PLATFORM_PRICE_NOTICE_DAYS` ngày (409 `price_notice_too_short` kèm `min_effective_from`); email báo gửi đúng một lần cho mỗi owner và không gửi cho workspace `pending_deletion`; giảm giá và gói chưa có thuê bao trả phí có hiệu lực ngay trong chính request.
- AC39. Đơn đã tạo giữ số tiền cũ, kỳ đã trả không đổi, đơn tạo từ `effective_from` dùng giá mới; mua ghế hay đổi gói bằng báo giá cũ bị 409 `quote_changed`; chấm tiền của ngày D dùng giá có hiệu lực lúc 00:00 UTC của D, kể cả khi chấm muộn sau khi giá đã đổi.
- AC40. Việc nền `plan_price_apply` lũy đẳng: chạy hai lần và song song áp một lần, tăng `version` một lần; hủy trước hiệu lực giữ nguyên giá và gửi thư đính chính nếu thư báo đã đi; `GET /api/workspaces/{id}/plans` chỉ trả giá sắp đổi cho gói đang dùng và chỉ sau khi thư báo đã gửi; mỗi lần lập, áp, hủy sinh đúng một dòng kiểm toán có giá cũ và mới.

Nhóm 5, admin nền tảng tạo tài khoản (7b.5):
- AC41. Admin tạo tài khoản cá nhân: trong vài nhịp việc nền, người dùng có `user`, một workspace `active` do họ làm owner, `billing_profile` loại `individual` và gói đã chọn; họ đăng nhập bằng mã email (kể cả khi `ALLOW_SIGNUP` tắt) và vào thẳng workspace, không qua onboarding; lần đăng nhập đầu vẫn hiện `consent_required`.
- AC42. Tài khoản doanh nghiệp có `billing_profile` loại `business` đã xác minh, thuê bao theo kích hoạt (dùng thử, cấp miễn phí một kỳ có `comp_reason`, hoặc gói theo mức dùng) cùng đơn 0 đồng `admin_grant`; thiếu cả `trial` lẫn `comp_reason` cho gói trả phí bị 400 `account_activation_required`; gói sai `audience` bị 409 `plan_audience_mismatch`; owner thấy dòng `account.provisioned` với actor `staff` trong nhật ký của workspace.
- AC43. Email đã có người dùng bị 409 `account_email_exists` và tài khoản cũ không bị sửa; email thuộc miền bị chặn làm yêu cầu `failed` với `email_domain_blocked`; gửi lại cùng email khi yêu cầu còn sống trả chính yêu cầu đó; hai yêu cầu đồng thời cùng email chỉ một cái thành công; người dùng đăng nhập tạo tài khoản trước việc nền thì yêu cầu `failed` với `email_exists` mà không hỏng dữ liệu.
- AC44. Chỉ `admin` gọi được các route `/accounts`; vượt `PLATFORM_ACCOUNT_CREATE_PER_HOUR` bị 429 `account_create_rate_limited`; hủy chỉ khi `pending`; mỗi bước sinh dòng kiểm toán đúng loại.
- AC45. `account_provisioning` và `account_request_finalize` lũy đẳng, chạy hai lần và song song không tạo hai người dùng, hai workspace hay hai thuê bao; lỗi tạm thời thử tối đa 3 lần rồi `failed`; bước 3 lỗi sau bước 2 để lại `finalize_failed` mà người dùng và workspace vẫn nguyên.
- AC46. Không có phiên, mật khẩu, mã đăng nhập hay token nào được tạo cho khách; staff không có dòng `member` trong workspace mới và console không có route đọc nội dung workspace đó; thư chào đúng một lần, không chứa mã hay liên kết có token, gửi lại tối đa 3 lần mỗi ngày; xóa tài khoản ẩn danh hóa `email` và `display_name` của yêu cầu.
- AC47. Tài khoản do admin tạo bị khóa, buộc đăng xuất và xóa như mọi tài khoản (4.1, 4.6); workspace của nó chịu mọi cổng và trạng thái của 5.3 và 5.7.

Hồi quy R5:
- AC48. Khi `MULTICA_CLOUD_URL` rỗng hoặc trỏ tới Multica Cloud, mọi route mới ở `/api/cloud-subscriptions` trả đúng lỗi của bảy route hiện có và client ẩn chức năng khi trường tùy chọn mới vắng mặt; nhóm tài khoản vẫn chạy không giới hạn số nhóm; toàn bộ test hiện có xanh với mọi cấu hình mới để trống.

## 14. Ngoài phạm vi

- Mạo danh người dùng bởi staff (D10).
- Danh sách thiết bị và thu hồi từng phiên (D7).
- SAML, SCIM (D18). MFA bằng khóa bảo mật hoặc SMS (D17).
- Bảng scope chi tiết cho PAT; tài khoản dịch vụ cấp workspace (D20).
- Vai trò kế toán, vai trò tùy biến, khách mời (D21, 7.1). Nhóm tài khoản với quyền cố định là ngoại lệ có chủ ý của vế "tùy biến" (D41, 7b.2); vai trò kế toán và khách mời vẫn ngoài phạm vi.
- Tự động trừ tiền định kỳ; hoàn tiền tự động; mã giảm giá. Gói theo mức dùng (7b.3) lập hóa đơn trả bằng chuyển khoản, không trừ tiền tự động; giảm giá Business là tỷ lệ trong điều khoản của gói, không phải mã giảm giá.
- Sao lưu và khôi phục theo từng tenant ở mức cơ sở dữ liệu. Công cụ cấp tenant là bản xuất (5.11); khôi phục toàn hệ thống là việc của hạ tầng.
- Trang trạng thái dịch vụ và thông báo trong ứng dụng từ nhà vận hành.
- Tín dụng trả trước và runtime đám mây do nhà vận hành cung cấp (`/api/cloud-billing/*`, `/api/cloud-runtime/*`): đây là sản phẩm của Multica Cloud, không dựng lại.
- Mỗi khách một hệ thống riêng (D2).
- Thuộc đợt bổ sung R5 và để ngoài có chủ ý: thuê bao ở cấp người dùng (A1); vai trò tùy biến tự do và quyền "cấm" (D41); thêm đồng hồ tính tiền ngoài ba đồng hồ của 7b.3; điều chỉnh từng dòng mức dùng (chỉ có xóa nợ theo cả đơn); giá ghi đè theo từng workspace (dùng gói riêng, D46); tạo tài khoản hàng loạt và thêm thành viên vào workspace có sẵn bằng console (7b.5); mật khẩu và mạo danh (D10); gói vô thời hạn mua một lần (A3); số dư trả trước (D44).

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

Ngoài ra còn chưa kiểm chứng: hợp đồng API của nhà cung cấp đối soát chuyển khoản và của nhà cung cấp hóa đơn điện tử (R4); và việc `WorkspaceLocker` có được worker phục hồi giữ trong lúc gọi client ghế hay không (Phụ lục B, bước P19).

### 15.2 Con số là lựa chọn, chưa đo

TTL cache guard 60 giây; cache chính sách ở platform 30 giây; cửa sổ chịu lỗi platform 15 phút (có sẵn ở client); ân hạn 7 ngày; chỉ đọc 30 ngày; chờ xóa 14 ngày; hạn đơn hàng 7 ngày; hạn liên kết xuất 7 ngày; cửa sổ đăng nhập lại 10 phút và 12 giờ; số lần thử MFA; thời hạn lưu nhật ký mặc định 365 ngày; mọi giới hạn và giá của gói. Ước lượng công ở kế hoạch là ước lượng của tác giả thiết kế. Thêm ở R5: mức tối đa 1.000 thành viên mỗi nhóm; tối đa 100 người mỗi lần thêm vào nhóm; số ghế tối đa của workspace cá nhân (5); hạn trả hóa đơn theo mức dùng 7 ngày và mức tối thiểu 10.000; 80 phần trăm hạn mức công nợ để báo; ngưỡng xác nhận đổi giá 50 phần trăm; báo trước tăng giá 30 ngày; trần tốc độ API 6.000; 20 yêu cầu mỗi lô và nhịp một phút của việc cấp phát tài khoản; 3 lần thử lại; 30 yêu cầu tạo tài khoản mỗi giờ.

### 15.3 Rủi ro

| Rủi ro | Mức | Giảm thiểu |
| --- | --- | --- |
| Chưa có giấy phép thương mại khi muốn ra mắt | cao | G-LIC; bắt đầu liên hệ bên cấp phép song song với R1 |
| Platform phục vụ ghế lệch ngữ nghĩa với handler | cao | Bộ test hợp đồng trước khi nối; chạy lại toàn bộ test lời mời và link chia sẻ |
| (a) Cơ sở dữ liệu chung buộc phiên bản schema của hai dịch vụ vào nhau | trung bình | Một chuỗi migration duy nhất; platform kiểm `schema_migrations` lúc khởi động và từ chối chạy nếu thiếu (D33); migration chỉ thêm nên bản platform cũ chạy được với schema mới hơn; thứ tự triển khai ở mục 12 |
| (b) Bộ gửi email của platform lặp lại bộ gửi của Multica (`server/internal/service/email.go:115`, `:178`), và bộ giải mã bí mật MFA lặp lại `secretbox` | thấp | Cùng biến cấu hình; cả hai có test với máy chủ SMTP giả; ghi là mục cần xem lại: khi có một gói dùng chung nằm ngoài `server/internal`, hợp nhất |
| (c) Platform ngừng hoạt động (D35) | trung bình | Quyền lợi dùng chính sách đã lưu tối đa 5 phút rồi thêm cửa sổ 15 phút của client; sau đó cổng hạn mức mở (`off`), trái với D35 đòi chặn hành động tính phí mới, vì client hiện có mở cho qua và việc đổi nó nằm ngoài D35 (5.5); lời mời và link chia sẻ bị chặn với 503; đọc và cưỡng chế trạng thái tài khoản, workspace không bị ảnh hưởng vì đọc từ DB chung |
| Xung đột khi nhập mã upstream vào các tệp ở 3.3 | trung bình | Điểm móc nhỏ, logic ở gói riêng; danh sách 3.3 là danh sách kiểm sau mỗi lần nhập |
| Thêm một lần đọc cache vào mọi request | trung bình | Một lần `GET` Redis; đo độ trễ p95 trước và sau ở G-R1 |
| Bật thương mại làm khách hiện hữu bị chặn | trung bình | Thuê bao chuyển tiếp (mục 12) |
| Guard trả 503 khi DB chậm | thấp | Cache che phần lớn; đây là hành vi đúng theo I8 |
| Hết dải số migration 651 đến 699 | thấp | Kế hoạch đã phân bổ đủ 46 tệp trong dải; dự phòng 953 đến 969 |
| (d) R5: `platform_usage_daily` trở thành nguồn tiền; mục 5.13 chưa định nghĩa "task chạy" (bắt đầu hay hoàn tất, theo múi giờ nào) và chưa có điều chỉnh từng dòng | cao | Q9 định nghĩa đồng hồ trước khi viết mã (task hoàn tất theo ngày UTC của `completed_at`), test đối chiếu với bảng nguồn; ngày thiếu số liệu không bị tính nên sai lệch nghiêng về phía khách; ngày sai chỉ sửa được bằng xóa nợ theo cả đơn (chấp nhận ở đợt này) |
| (e) R5: nợ gắn với workspace, owner bỏ nợ rồi mở workspace mới | thấp | `FREE_PLAN_OWNED_WORKSPACE_LIMIT` (5.7) giới hạn số workspace miễn phí; hạn mức công nợ giới hạn tổn thất mỗi workspace; không có chặn theo người dùng (chấp nhận) |
| (f) R5: tách `CreateWorkspace` để dùng chung có thể làm lệch tác dụng phụ sau commit (analytics, bật tri thức, báo daemon) khi chạy từ việc nền | trung bình | Q15 giữ hành vi của handler bằng test hồi quy; tác dụng phụ không có sẵn trong việc nền được ghi rõ là bỏ qua; chưa xác minh được rằng lượt chạy hằng giờ của tri thức tự bật cho workspace chưa từng gọi `AutoEnable` (chỉ thấy lời chú thích ở `workspace.go`), nên Q15 đọc mã đó trước |
| (g) R5: sáu khóa quyền là ánh xạ route viết tay; upstream thêm route mới vào nhóm owner và admin | thấp | Route mới mặc định vẫn chỉ owner và admin (đóng theo mặc định); bảng ánh xạ có test duyệt route; danh sách 3.3 là danh sách kiểm sau mỗi lần nhập mã |
| (h) R5: năm diễn giải A1 đến A5 của yêu cầu gốc có thể sai | trung bình | Mỗi diễn giải chỉ quyết định một quy tắc đã tách riêng (bảng 7b.0); chủ dự án xem ở 7b.7 trước Q1 |
| (i) R5: tài khoản do admin tạo mà khách không bao giờ đăng nhập; email gõ nhầm | thấp | Nhập email hai lần; hiển thị "chưa đăng nhập lần nào"; workspace trống nên thiệt hại tối đa là một gói miễn phí hay dùng thử; chưa có dọn tự động |
| (j) R5: migration của P21 có thể đặt `CHECK` trên `billing_order.kind` | thấp | Q10 kiểm migration của P21 và dùng số 969 nếu cần nới |

## 16. Việc pháp lý và thuế cần người có chuyên môn xác nhận

Tài liệu này không khẳng định yêu cầu pháp lý nào. Các điểm sau ảnh hưởng tới thiết kế và cần xác nhận trước G-LIC:

1. Giấy phép thương mại và miễn trừ thương hiệu của Multica (mục 0).
2. Nghĩa vụ theo quy định bảo vệ dữ liệu cá nhân của Việt Nam: nội dung đồng ý (4.4), thời hạn xử lý yêu cầu xóa và việc giữ lại nội dung đã viết trong workspace (D16), thời hạn lưu nhật ký có chứa dữ liệu cá nhân (4.3).
3. Yêu cầu về nơi lưu dữ liệu đối với từng nhóm khách hàng.
4. Nghĩa vụ xuất hóa đơn điện tử, thuế suất, và thông tin người mua bắt buộc trên đơn hàng (5.8).
5. Điều khoản dịch vụ về khóa tài khoản, chỉ đọc khi quá hạn, và xóa dữ liệu sau thời gian chờ (5.3, 5.9, 5.12).
6. Đợt R5: định dạng mã số thuế 10 hoặc 13 chữ số và thông tin người mua bắt buộc cho hồ sơ doanh nghiệp (7b.1); điều kiện để một khách được hưởng giá doanh nghiệp (D45).
7. Đợt R5: điều khoản của gói theo mức dùng và gói không giới hạn: cách tính, hóa đơn trả sau, lãi hay phạt chậm trả (hệ thống không tính), việc đưa workspace sang chỉ đọc khi còn nợ, và điều khoản dùng hợp lý cho gói không giới hạn (7b.3).
8. Đợt R5: nghĩa vụ báo trước khi tăng giá với khách đang có thuê bao (số ngày `PLATFORM_PRICE_NOTICE_DAYS` mặc định 30 là lựa chọn chưa được tư vấn xác nhận) (7b.4); cơ sở pháp lý để nhà vận hành tạo tài khoản và lưu email, tên của người thứ ba theo yêu cầu của khách, và nội dung thư chào (7b.5).

## Phụ lục A. Hợp đồng JSON của `/api/cloud-subscriptions/*`

Nguồn: schema zod ở `packages/core/api/schemas.ts` (từ `:3698`) và lời gọi ở `packages/core/api/client.ts` (từ `:2807`). Mọi đối tượng là "loose": trường lạ được bỏ qua, nên platform thêm trường tùy chọn được. Phản hồi không khớp schema bị client coi là "không khả dụng", không bao giờ coi là gói miễn phí; vì vậy mỗi ràng buộc dưới đây phải có test.

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

Ràng buộc: `limit` là số nguyên dương khi `mode = "limited"` (giới hạn 0 không biểu diễn được ở đây; workspace không `active` vẫn hiển thị giới hạn của gói). `seats`, `human_members`, `used`, `reserved`, `available`, `entitlement.version` không âm. `seat_capacity.purchased` và `seat_capacity.version` **dương**; workspace không giới hạn ghế trả `seat_capacity: null`. Vì `version` của thuê bao là 0 khi chưa có dòng, `seat_capacity.version` trả `version + 1`. `pending_quantity` là số dương hoặc `null`; khi có đơn `add_seats` đang chờ, trả số ghế đang chờ và `active_purchase` gồm `request_id` (mã đơn), `target_seats`, `status = "pending"`, `expires_at`. `payment_provider` là trường thêm của platform.

**`GET /prices`**: `{"month": {"currency": "vnd", "unit_amount": 149000, "interval": "month", "interval_count": 1}, "year": {…, "interval": "year", "interval_count": 1}}`. `unit_amount` dương; `interval_count` đúng bằng 1.

**`POST /checkout-sessions`**: yêu cầu `{"interval": "month|year", "idempotency_key": "…"}` cùng header `Idempotency-Key`; platform nhận thêm `plan_key`, `seats`. Phản hồi `{"request_id": "…", "session_id": "…", "url": "https://…"}`; cả `request_id` và `session_id` là mã đơn hàng.

**`POST /seats/purchase-preview`**: yêu cầu `{"additional_seats": n}`. Phản hồi `{"current_seats", "additional_seats", "resulting_seats", "purchase_version", "currency", "proration_amount", "next_invoice_amount", "quoted_at"}`. Ba số ghế và `purchase_version` dương; `currency` là ba chữ cái **thường**; hai số tiền không âm.

**`POST /seats/purchases`**: yêu cầu `{"additional_seats", "expected_current_seats", "expected_purchase_version", "accepted_proration_amount", "currency", "idempotency_key"}`. Phản hồi `{"request_id", "current_seats", "additional_seats", "resulting_seats", "currency", "proration_amount", "next_invoice_amount", "status": "pending|submitted|confirmed"}`.

**`POST /seats/reconcile`**: phản hồi `{"workspace_id", "action", "version"}`.

**`POST /portal-sessions`**: phản hồi `{"url": "https://…"}`; platform không phục vụ.

Lỗi ghế mà giao diện đã biết: `seat_capacity_full` (409), `seat_capacity_overcommitted` (409), `seat_capacity_rate_limited` (429), `seat_capacity_unavailable` (503), sinh ở `writeSeatCapacityError` (`server/internal/handler/seat_capacity.go:352`). Lỗi giới hạn issue đã có: 402 với `code = "issue_limit_reached"` (`server/internal/handler/issue_limit.go:37`); `plan_limit_reached` ở mục 10 theo đúng mẫu này.

## Phụ lục B. Giao thức ghế mà platform phải giữ

Nguồn: `server/internal/seatcapacity/client.go` và `server/internal/handler/seat_capacity.go`. Phía sản phẩm đã có sẵn và **giữ nguyên**: bảng intent bền, worker phục hồi, và khóa theo workspace (`WorkspaceLocker`, một khóa tư vấn gắn phiên). Platform đóng vai mà trước đây Multica Cloud đóng, và phục vụ giao thức này qua HTTP (mục 5.6).

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

**Quy tắc của platform.**

1. Mọi phương thức lũy đẳng theo `token`: gọi lại trả cùng kết quả, không giữ thêm chỗ. Worker và handler có thể gọi lại sau lỗi.
2. `Release` và `ReleaseMember` trên thao tác không tồn tại hoặc đã giải phóng trả `Allowed = false`, `Reason = "released"`, không trả lỗi.
3. Vượt chỗ được báo bằng phản hồi HTTP 409 với `code = "capacity_overcommitted"`, để client ghế dựng `*seatcapacity.HTTPError{StatusCode: 409, Code: "capacity_overcommitted"}`, vì handler nhận biết qua `IsCapacityOvercommitted` (`server/internal/seatcapacity/client.go:260`). Không bao giờ trả HTTP 429 (client coi là `IsRateLimited`).
4. Còn chỗ khi `PurchasedSeats - UsedSeats - ReservedSeats > 0`. `UsedSeats` đếm trực tiếp bảng `member`, nên thành viên có từ trước khi bật platform được tính đúng mà không cần chuyển dữ liệu. `Confirm` chuyển thao tác khỏi nhóm đang giữ vì thành viên vừa tạo đã nằm trong `UsedSeats`.
5. Chỗ giữ cho lời mời hết hiệu lực tại `expiresAt`; việc đếm bỏ qua thao tác đã quá hạn.
6. Platform tự tuần tự hóa theo workspace bằng `pg_advisory_xact_lock` với một không gian khóa **khác** của `WorkspaceLocker`. Handler đang giữ khóa của `WorkspaceLocker` trên một kết nối khác khi gọi platform qua HTTP; dùng chung khóa sẽ khiến handler chờ platform và platform chờ khóa của handler.
7. `Capacity.Version` là `version` của thuê bao.

**Worker phục hồi** (`server/internal/seatcapacity/worker.go`, đã đọc hết; xác nhận mục U3). Worker gọi `GetOperation` cho mục `reserve_invitation`, `consume_invitation` và `claim_share_join`; `Confirm` cho `confirm`; `Release` cho `release`; `ReleaseMember` cho `release_member`. Nó **giữ `WorkspaceLocker` suốt lúc gọi** (`settleWithWorkspaceLimit`, chờ khóa tối đa 30 giây), đúng như handler, nên quy tắc 6 áp dụng cho cả hai. Nó chỉ nhận bốn giá trị `Operation.State`: `reserved`, `consuming`, `used`, `released`; mọi giá trị khác là lỗi. `404` từ `GetOperation` được coi là thao tác không tồn tại; từ `Release` và `ReleaseMember` được coi là thành công.

**Trạng thái thao tác.** `reserved`: lời mời đang giữ chỗ đến `expiresAt`. `consuming`: việc chấp nhận lời mời, hoặc lượt vào bằng link, đang xử lý và đang giữ chỗ, không có hạn. `used`: thành viên đã tạo, chỗ nay do bảng `member` đếm. `released`: chỗ đã trả. Cả `reserved` và `consuming` nằm trong `ReservedSeats`. `ClaimShareJoin` tạo thao tác ở `consuming` ngay; `Consume` đổi `reserved` thành `consuming` và bỏ hạn; `Confirm` đổi hai trạng thái đó thành `used`; `Release` đổi bất kỳ trạng thái nào thành `released`. Một thao tác `reserved` quá hạn được báo là `released` (quy tắc 5).

**Chi tiết mà bảng chưa nói.**

- `Consume` trên token không có hoặc đã `released` trả `Allowed = false`, `Reason = "released"`. Trên `consuming` hoặc `used` trả `Allowed = true`.
- `Confirm` luôn thành công khi thành viên đã tạo, kể cả khi thao tác không còn: bảng `member` đã đếm chỗ đó.
- `ReleaseMember` không có dòng nào để đổi, vì `platform_seat_operation` không lưu mã thành viên và chỗ của thành viên do bảng `member` đếm. Nó luôn trả `Allowed = false`, `Reason = "released"`.
- `GetOperation` trên token không có trả 404 với `code = "operation_not_found"`. Lỗi đầu vào sai là 400 với `code = "invalid_request"`. Mọi lỗi khác là 500 với `code = "internal"`. Không có đường nào trả 429.
- Khóa của quy tắc 6 là `pg_advisory_xact_lock(int, int)` (hai số nguyên), còn `WorkspaceLocker` dùng `pg_try_advisory_lock(bigint)` (một số nguyên); PostgreSQL tách hai dạng này thành hai không gian khóa.

**Bộ test hợp đồng (kế hoạch P19)** mã hóa bảng và bảy quy tắc trên dưới dạng dữ liệu: `platform-management/contract/testdata/seat_scenarios.json` (kịch bản và kết quả mong đợi, viết tay). Test của platform (`platform-management/internal/seat`) chạy từng kịch bản trên handler HTTP với một kho giả, kiểm mọi kết quả mong đợi và ghi các phản hồi vào `seat_transcript.json`. Test của Multica (`server/internal/seatcapacity/contract_test.go`) chạy cùng các lời gọi bằng client ghế hiện có trên bản ghi đó, kiểm cùng kết quả mong đợi, kiểm yêu cầu client gửi đúng bản ghi, và kiểm các điều handler và worker dựa vào theo bảng. `stubSeatCapacity` trong `invitation_test.go` trả một quyết định cố định cho mỗi lời gọi và không giữ trạng thái, nên không phải đối tượng chạy bộ test các quy tắc có trạng thái (1, 2, 4, 5, 7); nó vẫn là bản giả của test handler. Các quy tắc đó được chứng minh trên handler của platform. Test handler `invitation` và `share_link` chạy lại với client thật trên các phản hồi đã ghi.
