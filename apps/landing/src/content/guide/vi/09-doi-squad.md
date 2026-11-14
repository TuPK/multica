---
title: Đội (squad)
order: 9
---

Đội gồm một agent trưởng nhóm (leader) và các thành viên. Giao issue cho đội không chạy mọi agent cùng lúc: Delegora đánh thức trưởng nhóm trước, nó đọc issue, giao từng bước cho thành viên phù hợp, rồi theo dõi cho đến khi xong. Trưởng nhóm không tự làm phần việc đã giao.

Đội chỉ quyết định việc được chuyển cho ai. Nó không gộp nhiều agent thành một, và không tự tăng giới hạn đồng thời.

### Khi nào dùng đội

Dùng đội khi công việc cần nhiều năng lực mà người phụ trách cụ thể chưa rõ lúc tạo issue. Ví dụ, một đội phát triển sản phẩm có thể có agent viết thiết kế, agent viết code và agent kiểm thử. Khi phạm vi đã rõ, giao thẳng cho agent phù hợp.

### Thành phần

Tạo đội cần tên và trưởng nhóm. Trưởng nhóm tự động là thành viên. Thêm thành viên (agent hoặc người), mỗi người có mô tả vai trò để trưởng nhóm biết ai giỏi việc gì.

- **Hướng dẫn của đội**: trưởng nhóm đọc mỗi khi làm issue của đội. Dùng cho quy tắc định tuyến ("việc về giao diện đi cho người X").
- **Kỹ năng**: kỹ năng gắn ở cấp đội áp dụng cho mọi thành viên.
- **Plugin bắt buộc**: trưởng nhóm tự cài trên runtime của dự án nếu còn thiếu.
- **Runtime chung**: đặt một runtime cho cả đội; có thể đổi hàng loạt cho nhiều agent.

### Luồng xử lý

1. Issue được giao cho đội (không ở `backlog`), trưởng nhóm bắt đầu làm.
2. Trưởng nhóm đăng một bình luận giao việc, nhắc đến thành viên được chọn.
3. Thành viên nhận lượt chạy riêng và làm việc.
4. Khi thành viên trả kết quả hoặc một giai đoạn kết thúc, trưởng nhóm được đánh thức lại để kiểm tra và giao bước tiếp theo.

Trưởng nhóm ghi đánh giá của mình vào dòng thời gian của issue, để con người xem được quyết định của nó.

Nếu bạn nhắc đến một thành viên trực tiếp trong bình luận, trưởng nhóm không bị đánh thức thêm cho bình luận đó, vì đó là một lần giao việc có chủ đích.

### Dự án của đội

Một đội chỉ làm một dự án đang mở tại một thời điểm. Gán dự án cho đội sẽ gắn cho đội quy trình phát triển của dự án, và issue sẽ đi qua các bước phát triển, review, QA. Issue đang ở trạng thái mà quy trình mới không có sẽ được hỏi để chuyển sang trạng thái tương ứng.

### Lưu trữ đội

Lưu trữ một đội sẽ chuyển các issue đang mở của đội sang trưởng nhóm. Thao tác này không hoàn tác được, nên hãy tạo đội mới nếu cần.

### Điều lệ và các vòng định kỳ

Một đội có thể có **điều lệ** gồm sứ mệnh, đầu ra phải có, những việc không được làm, khi nào phải hỏi bạn, và thời gian phản hồi. Điều lệ thay thế mô tả của đội cho mục đích này. Mỗi lần lưu tăng số phiên bản; nếu có người khác đã lưu bản mới hơn, lần lưu của bạn bị từ chối để bạn xem thay đổi đó trước.

**Vòng định kỳ** (routine) là một autopilot được gắn với đội, kèm mục đích và đầu ra mong đợi. Mỗi vòng có trạng thái: đúng hạn, không có đầu ra, trễ, lỗi, tạm dừng, bị giữ, hoặc chưa chạy. Đội luôn có một vòng **rà soát vận hành** hằng tuần, tự đánh giá đầu ra so với điều lệ và đề xuất việc đáng làm.
