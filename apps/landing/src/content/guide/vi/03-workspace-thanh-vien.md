---
title: Workspace và thành viên
order: 3
---

Workspace là ranh giới cao nhất. Thành viên, issue, dự án, agent, kỹ năng và lịch sử chạy đều thuộc một workspace và tách biệt với các workspace khác. Một tài khoản có thể tham gia nhiều workspace. Chuyển workspace không di chuyển dữ liệu.

### Nội dung của workspace

- **Công việc**: issue, dự án, bình luận, tệp đính kèm, lịch sử lượt chạy.
- **Đội ngũ**: thành viên, vai trò, lời mời.
- **Agent**: cấu hình, kỹ năng, runtime, tự động hóa.
- **Cài đặt chung**: ngữ cảnh của workspace, nhãn, thuộc tính tùy chỉnh, tích hợp.

Ngữ cảnh của workspace được đưa vào mọi agent. Dùng nó cho nền tảng của nhóm, quy ước sản phẩm và yêu cầu lâu dài. Yêu cầu chỉ cho một việc thì ghi vào issue.

### Vai trò

- **Owner**: toàn quyền, gồm xóa workspace. Workspace luôn phải có ít nhất một owner.
- **Admin**: quản lý cài đặt, thành viên, trạng thái issue, MCP và các tùy chọn chung. Không dùng quyền admin để chạy agent mà mình không được cấp quyền.
- **Member**: làm việc với issue, chat và các agent được cấp quyền truy cập.

Vai trò không quyết định ai được chạy agent. Mỗi agent có phạm vi truy cập riêng.

### Mời thành viên

Owner và admin mở **Cài đặt → Thành viên**, bấm **Mời thành viên**, nhập email và chọn vai trò. Lời mời có hiệu lực 7 ngày. Người được mời chỉ cần đăng nhập bằng đúng email đó. Để chia sẻ nhanh, tạo **liên kết mời** với vai trò và thời hạn; ai có liên kết đều tham gia được.

### Đổi vai trò và gỡ thành viên

Mở menu ở dòng thành viên, chọn **Đổi vai trò**. Thay đổi có hiệu lực ngay. Admin có thể chuyển giữa admin và member. Mọi thay đổi liên quan đến owner chỉ owner mới làm được.

Khi gỡ một thành viên, họ mất quyền truy cập ngay. Issue và bình luận họ tạo vẫn còn. Runtime của họ bị vô hiệu, agent gắn với các runtime đó bị lưu trữ, và các lượt chạy chưa xong bị hủy.

### Rời workspace

Thành viên có thể rời workspace. Sau đó cần lời mời mới để quay lại. Owner cuối cùng không rời được; hãy nâng một thành viên khác lên owner trước.

### Xóa workspace

Chỉ owner xóa được workspace. Xóa là vĩnh viễn: mất toàn bộ công việc, thành viên, cấu hình agent và lịch sử chạy. Nếu chỉ muốn tạm nghỉ, hãy rời workspace thay vì xóa.

### Token truy cập cá nhân

Token cá nhân (bắt đầu bằng `mul_`) đại diện cho tài khoản của bạn và truy cập được mọi workspace bạn có quyền. Tạo token trong **Cài đặt → API Token**, chọn thời hạn 30 ngày, 90 ngày, 1 năm hoặc không hết hạn. Token chỉ hiện một lần. Nếu lộ, thu hồi ngay trong cùng trang. Không đặt token trong repository, issue, bình luận hay ảnh chụp màn hình. Khi chạy CLI, lệnh `delegora login` tự tạo token hợp lệ 90 ngày và lưu trong cấu hình cục bộ.
