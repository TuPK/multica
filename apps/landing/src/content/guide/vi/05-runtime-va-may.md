---
title: Runtime và máy
order: 5
---

Một runtime là một máy đã kết nối cùng một công cụ AI. Daemon là tiến trình chạy nền trên máy đó: nó đăng ký runtime, nhận lượt chạy từ hàng đợi, gọi công cụ AI cục bộ và gửi kết quả về.

### Công cụ AI được hỗ trợ

Delegora không cung cấp tài khoản, gói đăng ký hay hạn mức mô hình cho các công cụ này. Bạn cần cài công cụ, đăng nhập bằng tài khoản của mình, và chạy thử trong terminal. Đăng nhập lưu cục bộ trong công cụ; Delegora không nhận token đăng nhập của chúng. Chỉ cần cài công cụ bạn định dùng. Daemon phải phát hiện ít nhất một công cụ được hỗ trợ mới khởi động được.

### Thêm máy

- **Ứng dụng desktop**: tự đăng ký máy và phát hiện công cụ. Sau khi đăng nhập, ứng dụng dùng một hồ sơ CLI riêng để chạy daemon, không ghi đè cấu hình terminal của bạn.
- **Máy khác hoặc máy chủ**: trong **Runtimes**, bấm **Thêm**, chọn **Máy khác hoặc máy chủ** và chạy hai lệnh được hiển thị trên máy đó.
- **Cloud runtime**: hiện đang mở danh sách chờ.

Nếu terminal tìm thấy công cụ mà ứng dụng không tìm thấy, thường là do khác biến `PATH`. Khởi động lại ứng dụng, hoặc đặt đường dẫn tuyệt đối cho công cụ.

### Trạng thái

- **Online**: nhận tín hiệu trong 45 giây gần nhất. Sẵn sàng nhận việc.
- **Mất kết nối dưới 5 phút**: thường là chớp mạng, hiển thị "vừa mất kết nối".
- **Offline** (trên 5 phút): kiểm tra máy hoặc khởi động lại daemon.
- **Offline trên 6 ngày**: nên chuyển agent sang runtime khác hoặc xóa runtime.

Lượt chạy đã vào hàng đợi chờ runtime quay lại; không tự chuyển sang máy khác.

### Giới hạn đồng thời

Mỗi daemon chạy tối đa 20 lượt cùng lúc; mỗi agent tối đa 6. Giới hạn thực tế là số nhỏ hơn trong hai số này. Lượt vượt giới hạn sẽ xếp hàng.

### Chia sẻ runtime

Runtime cục bộ mặc định là riêng tư: chỉ chủ sở hữu tạo được agent trên đó. Chủ sở hữu có thể chia sẻ cho **tất cả thành viên** hoặc cho **những người được chọn** trong trang chi tiết runtime. Chia sẻ không cho người khác đổi tên, xóa hay quản trị runtime. Khi thu hồi quyền, các agent của người bị thu hồi bị gỡ khỏi runtime và lượt chạy của họ bị hủy.

### Nhật ký và dọn dẹp

Daemon dọn thư mục làm việc của các issue đã đóng sau 24 giờ không cập nhật, dọn phần tạm (như `node_modules`) sau 12 giờ. Dữ liệu chat được dọn sau khi phiên bị lưu trữ. Thư mục đang dùng bởi lượt chạy không bao giờ bị dọn. Xem nhật ký bằng `delegora daemon logs`.

### Hồ sơ runtime tùy chỉnh

Nếu nhóm dùng một lệnh wrapper nội bộ, tạo một **hồ sơ runtime tùy chỉnh**. Chọn họ giao thức tương thích, nhập tên, lệnh và tham số cố định. Lệnh là một chương trình và tham số, không phải script shell. Nếu cần ống nối hay chuyển hướng, hãy đặt chúng trong một file script rồi dùng file đó làm lệnh.
