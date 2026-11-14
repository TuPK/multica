---
title: Xử lý sự cố
order: 16
---

Trước hết xác định lỗi nằm ở lớp nào: dịch vụ Delegora, daemon, runtime, hay công cụ AI. Phần lớn trường hợp chỉ cần xem trạng thái lượt chạy và runtime.

## Agent không chạy

Mở **nhật ký thực thi** của issue và xem trạng thái lượt chạy.

- **Đang xếp hàng**: kiểm tra runtime của agent có online không, công cụ AI có được phát hiện không, agent còn chỗ trong giới hạn đồng thời không, và daemon còn dung lượng không. Nếu công cụ không xuất hiện trong danh sách runtime, kiểm tra nó chạy được và đã đăng nhập trong cùng tài khoản hệ điều hành và `PATH`, rồi khởi động lại daemon.
- **Đang chờ thư mục cục bộ**: một lượt khác đang dùng cùng thư mục. Đổi tài nguyên sang chế độ song song (worktree) để loại bỏ hàng đợi.
- **Runtime offline**: kiểm tra máy và daemon. Các lượt chờ sẽ tự chạy khi runtime quay lại.

## Lỗi thường gặp

- **Không chạy được trên runtime riêng tư**: bạn không có quyền dùng runtime đó. Nhờ chủ sở hữu chia sẻ, hoặc gắn agent với runtime bạn dùng được.
- **Tài khoản AI hết hạn mức**: nạp thêm hoặc đổi tài khoản, rồi thử lại.
- **Tài khoản AI chưa đăng nhập**: đăng nhập lại trên máy chạy agent.
- **Hết thời gian chờ**: chia yêu cầu thành các bước nhỏ hơn.
- **Máy không thiết lập được lượt chạy**: thường do hết dung lượng đĩa, thiếu quyền, hoặc thư mục bị khóa. Đọc lỗi chi tiết bên dưới tin nhắn.
- **Công ty đang tạm dừng hoặc ngân sách đã chạm giới hạn**: lượt chạy bị giữ. Tiếp tục công ty hoặc tăng ngân sách.
- **Đã đạt giới hạn lượt autopilot trong kỳ**: các lượt sau chạy ở kỳ mới.
- **Autopilot tạm dừng vì mất runtime**: gắn lại runtime rồi kích hoạt lại.

## Không đăng nhập được hoặc không kết nối được

- Đăng nhập lại trong ứng dụng hoặc chạy lại lệnh đăng nhập của công cụ dòng lệnh.
- Nếu mạng có proxy hoặc tường lửa, kiểm tra máy có truy cập được dịch vụ Delegora không.
- Nếu đăng nhập trên web hoàn tất nhưng công cụ dòng lệnh báo lỗi bắt tay TLS, thử lại với tùy chọn tương thích TLS được ghi trong tài liệu kỹ thuật.

## Email không đến

Kiểm tra thư mục spam. Lời mời và mã xác minh hết hạn sau một khoảng thời gian ngắn; yêu cầu gửi lại. Nếu vẫn không thấy, liên hệ bộ phận hỗ trợ kèm email tài khoản.

## Khi báo lỗi

Gửi kèm phiên bản công cụ dòng lệnh, hệ điều hành, thông báo lỗi và nhật ký daemon (`delegora daemon logs`). Xóa token, địa chỉ email và dữ liệu khách hàng trước khi gửi.
