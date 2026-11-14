---
title: Giao việc và lượt chạy
order: 7
---

Có bốn cách kích hoạt agent. Chọn theo nhu cầu: giao quyền sở hữu, hỏi một việc, đánh thức theo điều kiện, hoặc chạy tự động theo lịch.

### Giao issue (sở hữu lâu dài)

Khi việc cần một chủ sở hữu rõ ràng, giao issue cho agent hoặc đội. Agent đọc mô tả và thảo luận, làm việc bằng cấu hình của nó, và cập nhật trạng thái. Trước khi xác nhận, hộp thoại cho bạn xem agent sắp chạy. **Chưa bắt đầu** lưu người được giao mà không tạo lượt chạy; chuyển khỏi `backlog` mới bắt đầu chạy.

Giao cho **người** thì không tạo lượt chạy nào, chỉ đổi người phụ trách. Giao cho **đội** thì trưởng nhóm được đánh thức trước, và nó quyết định ai làm từng bước.

### Nhắc đến agent trong bình luận (hỏi một việc)

Khi chỉ cần một câu hỏi hoặc một lần xem xét, nhắc đến agent bằng `@` trong bình luận mà không đổi người được giao. Trước khi gửi, khung xem trước cho biết mỗi agent được kích hoạt do nhắc đến trực tiếp, do người được giao, hay do trưởng đội. Bỏ chọn một agent thì chỉ bỏ lượt đó.

Bình luận không có `@` được định tuyến theo ngữ cảnh: trả lời trong một thảo luận đã có agent sẽ đi tới agent đó; bình luận cấp cao nhất đi tới người được giao. Bắt đầu bình luận bằng `/note` thì không kích hoạt agent nào. `@all` thông báo cho mọi thành viên và tắt kích hoạt tự động cho người được giao.

Agent luôn trả lời trong luồng của nó, kể cả khi runtime đang bận hay offline. Delegora không tự chuyển sang agent khác sau khi hết thời gian chờ.

### Điều chỉnh khi agent đang chạy (steer)

Khi agent đi sai hướng, không cần dừng và chạy lại. Gửi một bình luận chỉnh hướng; nó được đưa vào lượt đang chạy. Nhãn dưới ô nhập cho biết cách gửi và có thể đổi. Chỉ văn bản được đưa vào lượt đang chạy; tin nhắn có tệp đính kèm được xử lý sau khi lượt kết thúc. Steer cần công cụ AI hỗ trợ.

Nhiều bình luận liên tiếp cho cùng một agent sẽ gộp vào lượt đang chờ, và được xử lý cùng nhau.

### Đánh thức theo điều kiện (wakeup)

Một wakeup là quy tắc gắn trên issue, nói rằng "khi điều này xảy ra thì đánh thức agent này". Dùng khi việc phải chờ: chờ báo giá, chờ người khác trả lời, chờ issue con hoàn tất. Quy tắc được lưu trên issue, nên không có gì chạy trong lúc chờ.

Điều kiện có thể là: thời điểm, khi có người trả lời, khi trạng thái hoặc trường thay đổi, khi issue con hoàn tất, khi PR liên kết được merge, hoặc khi một issue khác đổi. Có thể lặp lại tối đa 50 lần. Khi đến hạn mà không có gì xảy ra, agent được đánh thức để báo lại, hoặc quy tắc kết thúc không chạy lượt nào. Agent có thể tự đặt lịch đánh thức của chính nó trong lượt chạy.

Lượt do wakeup tạo ra chạy thay mặt người tạo quy tắc. Delegora kiểm tra lại quyền của người đó trước khi chạy.

### Lượt chạy và nhật ký thực thi

Mỗi lần agent làm việc là một lượt chạy: nó ghi lại tác nhân kích hoạt, agent, tiến độ và kết quả. Lượt chạy xếp hàng, đang chạy, hoàn tất, thất bại hoặc bị hủy. Mục nhật ký thực thi trên issue liệt kê mọi lượt chạy, xem được nội dung từng lượt, dừng lượt đang chờ hoặc đang chạy, và thử lại.

Lỗi tạm thời (runtime mất kết nối ngắn, daemon khởi động lại, hết thời gian chờ, mất mạng khi gọi công cụ) có thể được thử lại tự động. Lỗi do agent trả về, như hết hạn mức hay thông tin đăng nhập sai, thường không tự thử lại. Thử lại thủ công luôn được.

Chạy lại (rerun) lặp lại một lượt với đúng agent và bối cảnh; có thể chọn chạy mới hoàn toàn.

### Các lý do thất bại thường gặp

- Hết thời gian chờ: chia yêu cầu thành các bước nhỏ.
- Mô hình hoặc tài khoản AI hết hạn mức: nạp thêm hoặc đổi tài khoản.
- Tài khoản AI chưa đăng nhập lại: đăng nhập trên máy chạy agent.
- Runtime offline: kiểm tra máy và daemon.
- Máy không thiết lập được lượt chạy (hết đĩa, thiếu quyền): xem lỗi chi tiết bên dưới tin nhắn.

Bảng đầy đủ các lý do và cách xử lý có trong Analytics, tab Lỗi.
