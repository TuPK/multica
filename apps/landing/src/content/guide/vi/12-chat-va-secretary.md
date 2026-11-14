---
title: Chat và Secretary
order: 12
---

## Chat với agent

Chat dành cho việc chưa thành issue rõ ràng: sắp xếp ý tưởng, hỏi một câu không thuộc issue nào, hoặc thảo luận cách làm. Chat là cuộc trò chuyện riêng giữa bạn và một agent, nằm ngoài bảng issue. Người khác trong workspace không xem được, trừ khi quản trị viên quản lý hệ thống. Kết luận cần cả đội biết thì đưa vào issue, mô tả dự án, hoặc tài liệu kiến thức.

Chat không gắn với issue, nên agent không tự nhận mô tả và bình luận của issue nào. Bạn có thể đính kèm một dự án vào cuộc trò chuyện để agent có ngữ cảnh.

Mỗi tin nhắn tạo một lượt chạy và câu trả lời trở lại trong cuộc trò chuyện. Lượt sau tiếp tục cùng phiên của công cụ AI, nên không cần nhắc lại nội dung trước đó.

**Chat nổi**: từ mọi trang, nút chat ở góc dưới phải mở một cửa sổ nhỏ. Nhấn `Cmd/Ctrl+J` để mở hoặc đóng; cửa sổ đưa con trỏ vào ô nhập.

**Hàng đợi**: khi agent đang trả lời, tin nhắn mới xếp vào hàng đợi. Với tin đang chờ, **Steer** dừng câu trả lời hiện tại trước, rồi xử lý tin này. Nội dung đã trả về được giữ lại. Nếu agent chưa trả lời gì, tin của bạn được lấy lại vào ô nhập, kèm tệp đính kèm, để bạn sửa.

Để dừng giữa chừng, bấm nút dừng cạnh ô nhập.

## Secretary

Secretary là một agent cho cả workspace. Nó đọc những gì các đội đã làm, rồi viết một bản tin mỗi ngày. Bản tin nêu việc cần quyết định, việc đang tiến triển, và việc đang bị kẹt. Bạn không nhận báo cáo riêng từ từng đội. Các đội làm việc qua issue, dự án và đề xuất; Secretary tự lấy thông tin nó cần, nên đội không tốn lượt chạy để báo cáo trạng thái.

### Secretary làm gì

- Hỏi các trưởng đội về phần việc họ phụ trách, đọc câu trả lời, và thách thức những gì còn sơ sài. Mỗi câu hỏi có giới hạn số lần thách thức.
- Tổng hợp một đánh giá duy nhất cho bạn, nêu rõ câu trả lời, các quyết định cần đưa ra, rủi ro được xếp hạng kèm người phụ trách, và những gì nó chưa kiểm tra.
- Chuyển quyết định của bạn cho trưởng đội thực hiện, dưới dạng một chỉ thị (directive). Chỉ thị chỉ được tạo từ một tin nhắn chat của owner hoặc admin, và lời bạn được lưu nguyên văn.

Secretary không tự duyệt đề xuất hay cổng quyết định. Các việc đó luôn có nút **Duyệt** và **Từ chối** cho bạn. Secretary cũng không sửa trạng thái hay người được giao của issue, và không xóa gì.

### Bản tin hằng ngày

Mỗi sáng Secretary xuất bản bản tin từ bản tóm tắt (digest) của công ty. Bản tin có tiêu đề ngắn, các quyết định đang chờ, tiến độ theo tầng, mục tiêu đổi trạng thái, ngân sách, pipeline, nhân sự, rủi ro và ghi chú. Bản tin khoảng hai phút đọc. Nếu máy chạy Secretary đang ngủ lúc 8 giờ, bản tin được bù sau trong ngày.

Thông báo "Bản tin đã sẵn sàng" là thông báo định kỳ duy nhất từ Secretary. Bạn có thể đánh giá từng bản tin là hữu ích hoặc không, kèm thẻ lý do (dài quá, thiếu nội dung, sai, mơ hồ, không phù hợp). Secretary đọc đánh giá gần đây để điều chỉnh.

### Gửi bản tin qua ứng dụng chat

Bạn có thể nhận bản tin ngay trong Slack, Telegram hoặc Lark sau khi liên kết tài khoản. Tin nhắn ngắn, gồm tiêu đề, số quyết định đang chờ và ba quyết định đầu, kèm nút **Duyệt**, **Từ chối**, **Hoãn 7 ngày** cho từng quyết định. Chỉ owner và admin quyết định được qua ứng dụng chat. Có thể đặt khoảng giờ yên lặng để không nhận tin.

Hai lệnh có sẵn trong chat với Secretary: `/brief` và `/decisions`.

### Giới hạn

Secretary chỉ đọc workspace qua digest và có thể hỏi bất kỳ đội nào, nhưng không quyết định đề xuất, không sửa issue, không xóa, và không xuất bản kiến thức trừ khi chính sách xuất bản cho phép. Khi công ty tạm dừng hoặc đang trong giờ yên lặng, câu trả lời của nó ghi rõ là một phần và lý do.
