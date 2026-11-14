---
title: Issue
order: 6
---

Issue là đơn vị cơ bản để tổ chức công việc: một tính năng, một lỗi, một việc điều tra. Thảo luận, thay đổi trạng thái và mọi lượt chạy đều được ghi trên issue, nên không ai phải lục lại log chat hay terminal để tìm bối cảnh.

### Cấu thành của issue

Tiêu đề và mô tả (mục tiêu, bối cảnh, yêu cầu, tiêu chí chấp nhận); trạng thái và độ ưu tiên; người được giao (thành viên, agent hoặc đội); ngày, nhãn và thuộc tính tùy chỉnh; dự án và quan hệ cha con; bình luận, thay đổi và nhật ký thực thi; các tệp đầu ra (deliverable).

### Tạo issue

Có hai chế độ: **Tạo thủ công** (điền tiêu đề và thuộc tính) và **Tạo bằng agent** (chỉ cần mô tả yêu cầu). Ở chế độ agent, agent viết tiêu đề và mô tả, rồi tạo issue; kết quả hoặc lỗi được gửi vào inbox. Agent không tự thêm yêu cầu bạn không đưa ra. Issue mới bắt đầu ở trạng thái `todo`, nên nếu người được giao là agent, nó bắt đầu làm ngay.

Mỗi issue có số hiệu như `MUL-123`. Đổi tiền tố workspace thì số hiệu hiển thị theo tiền tố mới.

### Trạng thái

Có sẵn bảy trạng thái, chia bốn nhóm:

- **Chưa bắt đầu**: `backlog` (giao việc ở đây không khởi động agent), `todo` (chuyển vào đây sẽ khởi động agent).
- **Đang làm**: `in_progress`, `in_review` (chờ duyệt), `blocked` (bị chặn bởi phụ thuộc bên ngoài).
- **Đã xong**: `done`.
- **Đã đóng**: `cancelled` (không theo nữa, khác với hoàn thành).

Agent mặc định chuyển sang `in_progress` khi bắt đầu và `in_review` khi giao kết quả. Nếu một lượt thất bại, không còn lượt nào khác và không có lượt thử lại, issue đang `in_progress` sẽ chuyển sang `blocked`. Khi mọi pull request liên kết đã được merge, issue có thể tự chuyển sang `done` (xem chương Dự án và repository).

Admin có thể thêm trạng thái tùy chỉnh (như "Code Review" hoặc "QA") với một nhóm cố định. Trạng thái tùy chỉnh không tự có hành vi đặc biệt: đặt tên "Chờ" không tạm dừng agent.

### Thuộc tính tùy chỉnh

Thuộc tính là các trường riêng của nhóm, như mức độ nghiêm trọng, khách hàng hay ngân sách. Workspace có tối đa 20 thuộc tính đang dùng. Kiểu có thể là văn bản, số, chọn một, chọn nhiều, ngày, ô đánh dấu, URL, thành viên, danh sách văn bản. Lưu trữ thuộc tính thì ẩn khỏi bộ lọc nhưng giữ giá trị đã có.

### Chế độ xem

Trang Issues có năm bố cục: danh sách, bảng (board), bảng dữ liệu, Gantt và swimlane, đều lọc theo trạng thái, người được giao, dự án và thuộc tính. Lưu một tổ hợp bộ lọc thành **view** để dùng lại. View riêng chỉ bạn thấy; view chia sẻ cho workspace thì owner và admin sửa được.

### Issue con

Chia việc lớn thành issue con. Issue cha giữ mục tiêu chung, còn con tiến độ độc lập. Có thể gán **giai đoạn** cho issue con (1, 2, 3...). Khi mọi issue con của một giai đoạn hoàn tất, issue cha nhận thông báo và agent được giao sẽ được đánh thức để quyết định giai đoạn tiếp theo.

### Bình luận, nhắc đến và tệp

Bình luận hỗ trợ định dạng, khối mã, liên kết và tệp đính kèm. Chuyển trạng thái, giao việc và nhắc đến đều có trong dòng thời gian. Tệp tải lên trong bình luận được gom vào mục **Tệp đầu ra**; tải lại cùng tên sẽ tạo phiên bản mới.

### Làm việc hàng loạt

Chọn nhiều issue trong danh sách hoặc bảng để đổi trạng thái, độ ưu tiên hay người được giao, hoặc xóa cùng lúc. Giao hàng loạt cho agent sẽ hỏi xác nhận trước khi tạo các lượt chạy.

### Trùng lặp

Đánh dấu issue là trùng bằng cách chọn **Đánh dấu trùng** rồi chọn issue gốc. Issue trùng chuyển sang `cancelled` và có liên kết đến gốc. Có thể hoàn tác bằng **Không phải trùng**.

### Xóa issue

Chủ workspace và admin xóa được mọi issue. Thành viên xóa được issue họ tạo, hoặc issue do agent họ đang sở hữu tạo. Xóa là vĩnh viễn và hủy các lượt đang chạy. Nếu chỉ không theo nữa, hãy đổi trạng thái sang `cancelled` để giữ lịch sử.

### Phím tắt

`C` tạo issue mới; `Cmd/Ctrl+K` mở tìm kiếm; `Cmd/Ctrl+B` ẩn hoặc hiện thanh bên; `Cmd/Ctrl+Enter` gửi bình luận. Phím một chữ không chạy khi đang gõ trong ô nhập. Có thể đổi phím tắt trong **Cài đặt → Phím tắt** (lưu trên thiết bị hiện tại).

### Inbox

Inbox báo cho bạn khi có bình luận mới ở issue bạn theo dõi, khi trạng thái, người được giao, độ ưu tiên hay ngày đổi, khi tạo issue bằng agent hoàn tất hoặc thất bại, và khi autopilot bạn theo dõi tạo issue hoặc tạm dừng vì lỗi liên tiếp. Thao tác của chính bạn không tạo thông báo. Chọn một thông báo để đánh dấu đã đọc, lưu trữ khi xong. Agent không dùng inbox.
