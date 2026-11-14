---
title: Agent
order: 4
---

Agent là một cộng tác viên trong workspace: bạn giao issue cho nó, nhắc đến nó trong bình luận, hoặc chat trực tiếp. Agent không chạy nền. Nó lưu danh tính và cấu hình, và chỉ tạo lượt chạy khi có việc đến.

### Tạo agent

Cần có runtime online trước. Runtime quyết định máy và công cụ AI nào sẽ thực thi. Có hai cách tạo:

- **Để AI viết**: trình tạo hỏi các thông tin chính và điền cấu hình, có thể sửa bản nháp song song.
- **Điền thủ công**: chỉ bắt buộc tên (duy nhất trong workspace) và runtime.

Mô tả là phần giới thiệu ngắn cho đội, không đưa vào prompt. **Hướng dẫn** mới là phần agent đọc mỗi lần chạy. Nên ghi rõ agent làm gì và không làm gì, bước đầu tiên khi có việc, được sửa những gì, cách giao kết quả, và khi nào phải hỏi người.

### Cấu hình

- **Mô hình và mức suy luận**: để trống thì dùng mặc định của công cụ. Mỗi công cụ có danh sách mô hình riêng; đổi công cụ thì phải chọn lại mô hình.
- **Kỹ năng**: gắn các phương pháp dùng lại được. Quy trình riêng của một agent thì để trong hướng dẫn.
- **Giới hạn đồng thời**: số lượt chạy tối đa cùng lúc, mặc định 6. Lượt vượt quá sẽ chờ trong hàng đợi.
- **Biến môi trường**: đặt khi công cụ khởi động. Chỉ dùng thông tin đăng nhập có quyền hạn thấp, ví dụ khóa API chỉ đọc. Không dùng mật khẩu quản trị cơ sở dữ liệu.
- **Tham số dòng lệnh tùy chỉnh**: thêm vào lệnh khởi động công cụ. Không đặt bí mật ở đây vì có thể lộ qua danh sách tiến trình.
- **Máy chủ MCP**: cấu hình cho agent riêng, hoặc chọn từ thư viện MCP dùng chung của workspace.
- **Tích hợp**: kết nối dịch vụ bên ngoài mà agent có thể dùng.

Thay đổi cấu hình không ảnh hưởng các lượt đang chạy. Lượt sau dùng cấu hình mới.

### Quyền truy cập

Mỗi agent có một chủ sở hữu và một phạm vi truy cập:

- **Chỉ mình tôi** (mặc định): chỉ chủ sở hữu chạy được.
- **Toàn workspace**: mọi thành viên chạy được.
- **Người cụ thể**: chủ sở hữu và những thành viên được chọn.

Chỉ chủ sở hữu mới đổi được phạm vi này. Owner và admin quản lý được phần còn lại của cấu hình nhưng không vượt qua phạm vi truy cập để chạy agent.

### Máy chủ MCP và biến môi trường

Thông tin cấu hình có thể chứa token được lưu trên máy chủ và không hiển thị lại sau khi lưu. Để đổi, bấm **Thay cấu hình** và nhập đầy đủ cấu hình mới. Xóa một máy chủ MCP trong thư viện sẽ gỡ nó khỏi mọi agent được gán. Chỉ tắt cho một agent thì tắt ở tab MCP của agent đó.

### Sao chép

Sao chép giữ hầu hết cấu hình (hướng dẫn, kỹ năng, quyền truy cập, biến môi trường, máy chủ MCP), trừ tên. Phần gán máy chủ MCP từ thư viện không được sao chép và phải gán lại.

### Lưu trữ và khôi phục

Lưu trữ agent sẽ ngừng giao việc, không còn nhắc đến được, và hủy mọi lượt chạy chưa xong. Lịch sử được giữ nguyên và có thể khôi phục sau.

### Trạng thái

Danh sách agent hiển thị hai thứ: **khả năng** (online, offline, không ổn định, lấy từ runtime) và **khối lượng việc** (đang làm, đang chờ, rảnh, lấy từ các lượt chạy). Agent offline không bị xóa; các lượt đã vào hàng đợi chờ runtime quay lại.

### Gợi ý hội thoại

Thêm tối đa ba gợi ý hội thoại (nhãn ngắn và prompt đầy đủ) để người dùng biết agent giỏi việc gì. Chọn gợi ý chỉ điền vào ô nhập, không gửi tự động.

### Tạo bằng CLI

```bash
delegora agent create --name "Người rà soát frontend" --runtime-id <runtime-id> \
  --description "Rà soát pull request frontend" \
  --instructions "Đọc diff và test trước; chỉ trả kết luận bằng bình luận trên issue."
```

Dùng `delegora agent copy <agent-id>` để nhân bản một agent có sẵn.
