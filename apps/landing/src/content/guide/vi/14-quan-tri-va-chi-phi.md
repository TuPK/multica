---
title: Kiểm soát, tự chủ và chi phí
order: 14
---

## Mức tự chủ

Mỗi đội có mức tự chủ từ 0 đến 3 (mặc định là 1). Chỉ owner và admin đổi được, và mỗi lần đổi được ghi vào nhật ký.

- **Gợi ý** (0): lượt chạy định kỳ bị giữ. Đội chỉ chạy khi có người giao việc hoặc nhắc đến. Vẫn có thể nộp đề xuất.
- **Làm việc** (1): lượt định kỳ chạy. Đội nghiên cứu, phân tích, viết bản nháp kiến thức và nộp đề xuất. Mọi quyết định là của bạn.
- **Tin cậy** (2): như mức 1, và bản nháp kiến thức có thể xuất bản sau rà soát. Duyệt sẵn có thể duyệt đề xuất của đội.
- **Tự chủ** (3): như mức 2, và duyệt sẵn có thể tạo initiative từ cơ hội của đội ở giai đoạn đầu, trong giới hạn số lượng và ngân sách.

Ở mọi mức, việc ra mắt và thay đổi cơ cấu luôn là quyết định của bạn, và chi tiêu luôn bị giới hạn bởi ngân sách.

Lưu ý quan trọng: Delegora không xem được agent làm gì bên trong một lượt chạy. Vì vậy mức tự chủ không thể ngăn một agent gọi API bên ngoài mà thông tin đăng nhập của nó cho phép.

### Duyệt sẵn

Duyệt sẵn là một chính sách phê duyệt tự động một nhóm hẹp các đề xuất đã được bạn chấp thuận trước: loại đề xuất, đội hoặc tầng áp dụng, chi phí tối đa, và số lần tối đa mỗi tuần. Bạn xem được các lần đã duyệt trong bản tin và có thể thu hồi bất cứ lúc nào. Nếu một lần duyệt bị đảo ngược khi rà soát, nó được đánh dấu. Agent không tự tạo hay sửa chính sách.

## Kiểm soát công ty

Công ty ở một trong ba trạng thái:

- **Đang chạy**: mọi việc chạy bình thường.
- **Tạm dừng**: lượt định kỳ và lượt được giao bị giữ; lượt đang chạy bị hủy khi chọn "dừng và hủy việc". Chat và Secretary vẫn hoạt động.
- **Giờ yên lặng**: lượt định kỳ chờ đến hết khung giờ (mặc định 22:00 đến 07:00). Lượt do người bắt đầu không bị giữ.

Chỉ owner và admin tạm dừng hoặc tiếp tục. Thành viên chỉ xem được trạng thái.

## Ngân sách

Ngân sách đặt giới hạn chi tiêu cho một phạm vi và một kỳ. Chi tiêu được tính từ các lượt chạy của phạm vi đó. Khi gần giới hạn, ngân sách cảnh báo. Ở chế độ **giữ**, khi đạt giới hạn, các lượt mới bị giữ trong hàng đợi và không bị đánh dấu thất bại. Chế độ **cảnh báo** chỉ báo mà không giữ lượt nào. Ngân sách mới nên bắt đầu ở chế độ cảnh báo để kiểm tra trước.

Bạn có thể tăng ngân sách cho kỳ hiện tại; mọi thay đổi được ghi lại. Một đội muốn thêm tiền có thể nộp đề xuất ngân sách.

## Nhật ký governance

Nhật ký là bản ghi chỉ thêm (append-only) của mọi thay đổi về cách công ty được quản trị: chính sách duyệt sẵn, ngân sách, mức tự chủ, thay đổi cơ cấu được áp dụng hoặc hoàn tác, xuất bản kiến thức tự động, initiative bị ép chuyển giai đoạn, tạm dừng và tiếp tục, mục tiêu được kích hoạt và đánh giá được xuất bản. Nhật ký không sửa hay xóa được và được giữ vô thời hạn. Lọc theo loại, tác nhân hoặc đối tượng, và xuất CSV tối đa 50.000 dòng.

## Phân tích

Trang **Analytics** tổng hợp mức dùng agent trên toàn workspace: chi phí, token, thời gian chạy, số lượt và lượt lỗi. Mọi thành viên xem được. Có hai tab:

- **Sử dụng**: xu hướng theo ngày hoặc tuần và bảng xếp hạng agent theo token, chi phí, thời gian hoặc số lượt.
- **Lỗi**: số lượt lỗi, tỉ lệ lỗi, các nguyên nhân gom theo loại, và các agent lỗi nhiều nhất.

Chi phí là **ước tính** theo bảng giá công bố của từng mô hình, không phải hóa đơn của nhà cung cấp. Với mô hình chưa có trong bảng, chi phí được tính bằng 0 cho đến khi bạn đặt giá tùy chỉnh.

## Bảo mật và ranh giới

Mỗi lượt chạy là một tiến trình con của daemon, chạy với đầy đủ quyền của người dùng hệ điều hành đang chạy daemon. Delegora không đóng khung hệ thống tệp cho bạn. Để giới hạn phạm vi, nên chạy daemon bằng một tài khoản hệ điều hành riêng, trong container hoặc máy ảo, chỉ gắn những thư mục và bí mật agent cần. Với mọi thông tin đăng nhập có thể truy cập từ môi trường đó, hãy coi là thứ agent có thể dùng: cấp token hẹp nhất và ưu tiên khóa triển khai riêng cho từng mục đích.

Token dành cho lượt chạy (`mat_`) được máy chủ tạo riêng cho từng lượt, gắn với người dùng, workspace, agent và lượt đó. Nó tự hết hạn và không phải thứ bạn cần quản lý.
