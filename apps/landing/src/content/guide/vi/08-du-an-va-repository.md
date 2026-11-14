---
title: Dự án và repository
order: 8
---

Dự án gom các issue cùng một mục tiêu, theo dõi tiến độ chung, và gắn tài nguyên (repository, thư mục) để agent biết code nào cần làm việc. Việc chỉ một issue mô tả được không cần dự án.

### Tiến độ và trạng thái

Tiến độ của dự án là tỉ lệ issue ở nhóm đã xong hoặc đã đóng. Trạng thái dự án và trạng thái issue độc lập: hoàn thành mọi issue không tự đặt dự án thành hoàn thành.

Mô tả dự án được đưa vào ngữ cảnh của mọi lượt chạy trong dự án. Đặt ở đây những gì mọi lượt đều cần biết.

Người phụ trách (lead) có thể là thành viên hoặc agent. Đặt người phụ trách không tự giao việc. Thành viên làm phụ trách còn đổi được quy trình (workflow) của dự án.

### Tài nguyên

- **Repository GitHub**: một repo runtime có thể checkout, với nhánh hoặc commit tùy chọn. Một dự án có thể có nhiều repo.
- **Thư mục cục bộ**: cho một runtime trên một máy cụ thể làm việc trong thư mục có sẵn. Chỉ dùng khi không có lựa chọn nào khác, vì agent sửa trực tiếp các tệp của bạn.

Có thể dùng repo bất kỳ mà runtime truy cập được qua Git, không chỉ GitHub.

Với repository, mỗi lượt chạy có một **worktree** riêng, nên nhiều lượt có thể chạy song song mà không đụng vào nhau. Mỗi lượt trả về một nhánh `agent/…` để bạn xem và gộp.

Với thư mục cục bộ có hai chế độ: **Trực tiếp** (một lượt tại một thời điểm, thay đổi xuất hiện ngay trong thư mục của bạn) và **Song song** (mỗi lượt có worktree riêng; cần thư mục là repo git có ít nhất một commit).

**Tự động gộp** (tùy chọn): khi lượt chạy xong, nhánh của nó được gộp vào nhánh đang checkout rồi xóa nhánh đó. Bị bỏ qua nếu repo còn thay đổi chưa commit hoặc đang ở trạng thái detached.

### Kết nối GitHub

Chủ workspace hoặc admin mở **Cài đặt → Code**, bấm **Kết nối GitHub**, chọn tài khoản hoặc tổ chức, và cấp quyền cho tất cả hoặc một số repository. Ứng dụng GitHub chỉ đọc: nó không đẩy commit, không bình luận và không đặt trạng thái kiểm tra.

Công tắc:

- **Khung PR**: hiện các PR liên kết trong chi tiết issue.
- **Co-authored-by**: thêm dòng đồng tác giả vào commit do agent tạo.
- **Tự liên kết PR**: liên kết PR với issue có mã trong nhánh, tiêu đề, hoặc theo sau từ khóa như `Closes MUL-123`.
- **Sau khi PR được merge, chuyển issue sang**: `Done` (mặc định), một trạng thái khác, hoặc không đổi.
- **Thẻ PR → CI và khả năng merge**: hiện trạng thái kiểm tra và xung đột.

### Liên kết PR với issue

Đặt mã issue trong tên nhánh hoặc tiêu đề PR, ví dụ `mul-123-sua-loi-dang-nhap` hoặc `MUL-123 Sửa lỗi chuyển hướng sau đăng nhập`. Nếu mã chỉ có trong phần mô tả, đặt nó ngay sau từ khóa `Closes`, `Fixes` hoặc `Resolves`. Nhắc đến thông thường như `Liên quan đến MUL-123` không liên kết. Có thể liên kết thủ công bằng nút **+** trong mục Pull requests.

### Khi PR được merge

Issue chỉ chuyển trạng thái khi **mọi** PR liên kết đã được merge, và issue không ở `done` hoặc `cancelled`, và không bị đặt giữ trạng thái. PR đóng mà không merge sẽ giữ issue lại đến khi bạn gỡ nó khỏi issue. Có thể giữ trạng thái của một issue cụ thể từ menu Pull requests, khi PR chỉ giao một phần việc hoặc còn bước phát hành.

### Nhiều workspace

Cùng một cài đặt ứng dụng GitHub có thể nối với nhiều workspace. Mỗi workspace chỉ khớp với tiền tố issue của chính nó. Nếu một mã khớp issue ở nhiều workspace, Delegora không tự liên kết để tránh đóng nhầm việc.

### Gỡ kết nối

Chọn **Ngắt kết nối** trong menu GitHub trên **Cài đặt → Code** chỉ gỡ quan hệ giữa workspace và cài đặt. Để thu hồi quyền phía GitHub, gỡ ứng dụng trong trang cài đặt GitHub của bạn.
