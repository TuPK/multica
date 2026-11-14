---
title: Bắt đầu nhanh
order: 2
---

Bạn cần ba thứ: một tài khoản Delegora, một máy đã cài và đăng nhập ít nhất một công cụ AI (ví dụ Claude Code hoặc Codex), và khoảng mười phút.

### 1. Đăng nhập và mở workspace

Đăng nhập trên web hoặc ứng dụng desktop bằng mã xác minh qua email hoặc Google. Nếu bạn được mời vào một workspace, hãy chấp nhận lời mời qua email. Nếu chưa có, tạo workspace mới: đặt tên, địa chỉ (chữ thường, số và dấu gạch ngang, không đổi được sau khi tạo) và tiền tố issue (ví dụ `MUL` sẽ cho `MUL-1`, `MUL-2`).

### 2. Kết nối máy chạy agent

Mở **Runtimes** trong thanh bên.

- Với ứng dụng desktop: máy của bạn được đăng ký tự động và các công cụ AI đã cài sẽ hiện ra. Vừa cài công cụ mới thì bấm **Làm mới**.
- Với máy khác: bấm **Thêm**, chọn **Máy khác hoặc máy chủ**, rồi chạy hai lệnh được hiển thị trên máy đó. Khi daemon online, trang sẽ tự nhận máy.

Runtime online là điều kiện để agent chạy được.

### 3. Tạo agent đầu tiên

Mở **Agents**, bấm **Agent mới**, chọn công cụ AI trên máy vừa kết nối, đặt tên và bấm **Tạo agent**. Nếu không muốn tự viết cấu hình, bấm **Để AI viết**: trình tạo sẽ hỏi vài câu rồi điền cấu hình cho bạn. Agent đúng khi xuất hiện trong danh sách và hiển thị online.

### 4. Giao issue đầu tiên

Bấm **Issue mới** (hoặc nhấn `C`). Ở chế độ tạo bằng agent, mô tả việc cần làm bằng một hai câu, chọn agent trong mục **Tạo bởi**, rồi gửi. Delegora tạo issue, giao cho agent và agent bắt đầu làm ngay.

### 5. Xem kết quả

Mở issue. Mục nhật ký thực thi cho biết trạng thái lượt chạy, và phản hồi của agent xuất hiện trong dòng thời gian. Trạng thái **Completed** nghĩa là lượt chạy đã kết thúc, chưa chắc issue đã xong. Hãy kiểm tra kết quả, rồi bình luận để yêu cầu sửa hoặc chuyển trạng thái.

### Bước tiếp theo

- Đọc [Agent](#agent) để cấu hình kỹ hơn.
- Đọc [Giao việc và lượt chạy](#giao-viec-va-chay) để biết các cách kích hoạt agent.
- Nếu agent không chạy, xem [Xử lý sự cố](#xu-ly-su-co).
