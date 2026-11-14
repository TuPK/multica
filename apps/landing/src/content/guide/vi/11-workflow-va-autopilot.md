---
title: Workflow và Autopilot
order: 11
---

## Workflow

Workflow xác định các bước một issue đi qua trong dự án, ai xử lý từng bước, và việc đó cần làm gì. Ví dụ, một quy trình phát triển: bước **Phát triển** do agent kỹ thuật làm; bước **Review** do agent rà soát, rồi chuyển sang QA hoặc trả về sửa; bước **QA** do một thành viên kiểm tra và quyết định hoàn thành.

Khi issue được chuyển vào bước Review, nó được giao cho agent rà soát. Mô tả, thảo luận và kết quả trước đó vẫn ở trên issue, nên người hoặc agent tiếp theo làm tiếp từ chỗ đã dừng.

### Tạo workflow

Owner và admin mở **Cài đặt → Workflows**, bấm **Workflow mới**, bắt đầu trống hoặc từ mẫu **Delivery**, **Content** hoặc **Community**. Đặt tên, chọn các cột trạng thái, và sắp xếp chúng theo cách nhóm làm việc.

Với mỗi cột, chọn điều gì xảy ra khi issue vào cột đó: **Không chuyển giao**, **Giao cho agent**, hoặc **Giao cho thành viên** (có thể chọn người phụ trách dự án). Với bước giao cho agent, viết hướng dẫn cho nó, đặt **Khi xong, chuyển sang** (cột kế tiếp nếu để trống), và **Nếu bị từ chối, trả về** khi cần làm lại. Dùng **Bị chặn** thay cho trả về khi bước không thể tiếp tục vì lý do bên ngoài.

Delegora kiểm tra cấu trúc khi lưu: `backlog` đứng đầu, `todo` ngay sau backlog, `blocked` trước `done`, `done` ngay trước `cancelled`, `cancelled` đứng cuối, mọi bước đều đi tới được trạng thái kết thúc, và không có vòng lặp về chính nó. Lỗi được chỉ rõ để bạn sửa.

### Dùng trong dự án

Mở dự án và chọn **Workflow**. Việc đổi workflow không chuyển giao issue và không khởi động agent. Dự án chưa chọn dùng **Default**, nơi mọi trạng thái đều có và không có chuyển giao tự động.

Workflow có thể dùng chung cho nhiều dự án; thay đổi sẽ áp dụng cho tất cả. Để một dự án có quy trình riêng, nhân bản workflow và chọn bản sao cho dự án đó.

Workflow đang được dự án hoặc đội dùng phải chuyển sang workflow khác trước khi xóa.

## Autopilot

Công việc lặp lại, như bản tóm tắt tiến độ hằng ngày, kiểm tra phụ thuộc định kỳ, hay một agent được kích hoạt bởi sự kiện từ hệ thống khác, nên đặt vào autopilot. Mỗi autopilot có một **runbook** (hướng dẫn chi tiết agent đọc mỗi lần chạy), một người được giao, và một hoặc nhiều **trigger**.

Có sáu mẫu sẵn: tin tức hằng ngày, rà soát PR cũ, phân loại bug, báo cáo tiến độ tuần, kiểm tra phụ thuộc, kiểm tra tài liệu.

### Trigger

- **Lịch (cron)**: chạy theo biểu thức cron với múi giờ IANA (ví dụ `Asia/Ho_Chi_Minh`). Trang hiển thị vài lần chạy tiếp theo để bạn kiểm tra trước khi lưu.
- **Webhook**: sau khi lưu, Delegora tạo một URL. Hệ thống khác gửi POST với JSON tới URL để kích hoạt. Thêm khóa `Idempotency-Key` để tránh chạy trùng khi người gửi thử lại. Có thể lọc theo loại sự kiện. Nếu URL bị lộ, bấm **Đổi URL**; URL cũ dừng ngay.

### Chế độ chạy

- **Tạo issue**: mỗi lần chạy tạo một issue và chạy như issue bình thường. Nếu runtime offline, issue vẫn được tạo và lượt chạy chờ.
- **Chỉ chạy**: không tạo issue. Nếu runtime không sẵn sàng khi kích hoạt, lượt chạy bị bỏ qua, không để lại việc chờ.

### Quản lý

**Chạy ngay** thực hiện đầy đủ một lần, không chờ lịch, và nên dùng để kiểm tra trước khi bật tự động. Nhật ký giao nhận cho thấy từng webhook đã nhận; chỉ phát lại được những lần có chữ ký hợp lệ, không bị từ chối, và đã xử lý xong.

Chủ sở hữu và admin luôn có quyền. Có thể cấp quyền chỉnh sửa và chạy cho thành viên khác. Số lượt chạy tự động trong mỗi kỳ bị giới hạn; khi chạm giới hạn, các lượt sau chờ đến kỳ mới. Autopilot bị tạm dừng sau nhiều lỗi liên tiếp, và bạn nhận thông báo trong inbox.
