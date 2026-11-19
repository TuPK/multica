# Checklist cho kế toán: trường người mua SePay (mục 16, điểm 4)

Mục đích: kế toán xác nhận bộ trường người mua mà SePay eInvoice cần, và cách xử lý các trường hợp còn mở, trước lần phát hành hóa đơn thật đầu tiên. Mã và test không chờ bước này; việc xác nhận chỉ cần trước khi bật `PLATFORM_EINVOICE_PROVIDER` ở production (xem `platform-management-spec.md` mục 7.3 và 16).

Nguồn: `platform-management-spec.md` mục 7.3 (phần "Trường người mua", "Chưa xác minh") và mục 16, điểm 4. Các mục dưới đây là câu hỏi, chưa có đáp án.

## A. Trường người mua đã biết (tài liệu SePay đánh dấu `required=false`)

Kế toán đánh dấu từng trường: **bắt buộc / tùy chọn / không dùng**, và ghi điều kiện nếu có.

- [ ] `type`: `personal` hoặc `company`. Xác nhận các loại khác có được không.
- [ ] `name`: tên người mua. Với cá nhân, là họ tên đầy đủ?
- [ ] `legal_name`: tên pháp lý của doanh nghiệp (dùng khi `type = company`). Có bắt buộc khi `company`?
- [ ] `tax_code`: mã số thuế. Cá nhân không có MST thì xử lý thế nào? Định dạng 10 hay 13 chữ số (xem mục 16, điểm 6)?
- [ ] `address`: địa chỉ đăng ký. Bắt buộc với loại nào?
- [ ] `email`: email nhận hóa đơn. Bắt buộc không?
- [ ] `phone`: số điện thoại. Bắt buộc không?
- [ ] `buyer_code`: mã khách hàng nội bộ. Có dùng không?
- [ ] `national_id`: số CCCD/hộ chiếu cho cá nhân. Có bắt buộc khi `personal` không hay chỉ khi không có MST?

## B. Bộ trường bắt buộc theo loại người mua (CHƯA XÁC NHẬN)

Tài liệu SePay không cho biết bộ trường bắt buộc cho từng loại. Kế toán xác nhận:

- [ ] Cá nhân không có MST: những trường nào bắt buộc?
- [ ] Cá nhân có MST: những trường nào bắt buộc?
- [ ] Doanh nghiệp: `legal_name`, `tax_code`, `address` có đủ không, hay cần thêm trường nào?
- [ ] Đơn có thể phát hành với `name` trống không? Nếu không, trường nào thay thế?

Khi thiếu trường bắt buộc, hệ thống đặt bản ghi ở trạng thái `needs_info`, ghi tên trường thiếu (không ghi giá trị) và không gọi nhà cung cấp. Kế toán xác nhận danh sách trường để hệ thống kiểm đúng.

## C. Ngoại tệ và tỷ giá (CHƯA XÁC NHẬN)

- [ ] `currency` nhận VND, USD, CAD. Platform mặc định là VND (`PLATFORM_CURRENCIES`). Có phát hành hóa đơn bằng USD hoặc CAD cho khách hàng Việt Nam không?
- [ ] Nếu có: tài liệu không có trường tỷ giá trong yêu cầu create. Kế toán xác nhận cách tỷ giá được ghi trên hóa đơn (theo ngân hàng nhà nước, hay theo tỷ giá khác), và SePay có tự tính không.
- [ ] Nếu chưa phát hành ngoại tệ, ghi rõ: "chỉ VND ở giai đoạn này".

## D. Xử lý hóa đơn đã phát hành sai (CHƯA XÁC NHẬN)

R4 không có endpoint hủy hay thay thế đã xác minh. Kế toán quyết định:

- [ ] Hóa đơn phát hành sai thông tin người mua: quy trình xử lý là gì (hủy, thay thế, hay điều chỉnh)?
- [ ] Ai có quyền yêu cầu xử lý và phê duyệt?
- [ ] Cần lưu lại bằng chứng gì (biên bản, email khách hàng, lý do)?
- [ ] Chấp nhận quy trình thủ công ngoài hệ thống trong giai đoạn đầu không?

Lưu ý từ tài liệu: không gửi lại khi thông báo "Hệ thống chưa xác nhận được kết quả phát hành". Gửi lại có thể tạo hóa đơn trùng.

## E. Căn cứ pháp lý (CHƯA KIỂM CHỨNG)

- [ ] SePay vẫn dẫn Nghị định 123/2020 trong mô tả. Một số nguồn bên thứ ba nói Nghị định 254/2026/NĐ-CP và Thông tư 91/2026/TT-BTC thay thế từ 2026-07-01. Kế toán hoặc tư vấn thuế xác nhận văn bản nào đang áp dụng.
- [ ] Tài liệu SePay đã theo quy định mới chưa?
- [ ] Thuế suất VAT áp dụng cho từng gói (`BILLING_VAT_RATE_VND`). Xác nhận giá đã gồm hay chưa gồm VAT.

## F. Thông tin cần kế toán cung cấp để kiểm sandbox

- [ ] Tài khoản SePay eInvoice (sandbox) đã đăng ký chưa? Cần `client_id`, `client_secret`, `provider_account_id`. Thông tin này không đưa vào tài liệu này; chuyển qua kênh bảo mật.
- [ ] Mẫu hóa đơn (`template_code`, `invoice_series`) đã được cơ quan thuế duyệt chưa?

## G. Ghi nhận

| Mục | Người xác nhận | Ngày | Kết luận |
|---|---|---|---|
| A. Trường người mua | | | |
| B. Bộ trường bắt buộc | | | |
| C. Ngoại tệ | | | |
| D. Hóa đơn sai | | | |
| E. Pháp lý | | | |
| F. Sandbox | | | |

Sau khi xác nhận, cập nhật `platform-management-spec.md` mục 7.3 (bỏ các chỗ "CHƯA XÁC NHẬN" tương ứng) trong cùng một commit với thay đổi mã, nếu có.
