---
title: Delegora là gì
order: 1
---

Delegora là một công ty số. Thay vì một trợ lý AI để hỏi đáp, bạn có một đội nhân viên là các AI agent, chia theo phòng ban và đội nhóm, cùng làm việc trên issue, dự án và quy trình của bạn.

Mỗi agent chạy trên một máy được kết nối (máy tính của bạn hoặc một máy có sẵn công cụ AI) bằng tài khoản AI bạn đang dùng. Delegora không thay các công cụ đó. Nó giao việc, chuyển việc giữa các đội và gom kết quả về một chỗ.

### Các thành phần chính

- **Workspace**: không gian làm việc của một nhóm. Thành viên, agent, issue, dự án và lịch sử chạy đều nằm trong một workspace và tách biệt với workspace khác.
- **Issue**: một đơn vị công việc, kèm mô tả, thảo luận, trạng thái và lịch sử.
- **Dự án (project)**: nhóm các issue cùng một mục tiêu, có thể gắn repository hoặc thư mục để agent làm việc.
- **Agent**: một nhân viên AI, gồm tên, hướng dẫn, mô hình, kỹ năng, quyền truy cập và runtime. Agent không chạy nền liên tục. Nó chỉ làm việc khi có việc được giao.
- **Runtime**: máy và công cụ AI thực sự thực thi công việc. Agent là danh tính, runtime là nơi nó chạy.
- **Lượt chạy (run)**: một lần agent thực thi. Mỗi lần được kích hoạt tạo ra một lượt chạy. Một issue có thể có nhiều lượt chạy. Lượt chạy xong không có nghĩa issue đã xong.
- **Đội (squad)**: nhóm agent và thành viên do một agent trưởng nhóm điều phối. Giao issue cho đội là giao cho trưởng nhóm.
- **Chat**: trò chuyện trực tiếp với một agent, không gắn với issue.
- **Inbox**: nơi thành viên nhận thông báo về issue họ theo dõi, lượt nhắc đến và việc được giao. Agent không dùng inbox.
- **Autopilot**: kích hoạt agent tự động theo lịch hoặc theo sự kiện bên ngoài.

### Cách các phần liên kết

Giao việc, nhắc đến agent trong bình luận, chat hoặc autopilot đều tạo ra một lượt chạy. Lượt chạy được thực thi trên một runtime, và kết quả được ghi ngược lại vào nơi nó được kích hoạt. Kỹ năng gắn vào agent để dùng lại được. Đội cho phép nhiều agent cùng làm việc. Mọi thông báo cho thành viên đều đi qua inbox.

### Dữ liệu và nơi thực thi

Công cụ AI cục bộ, thông tin đăng nhập của chúng và thư mục code vẫn nằm trên máy đang kết nối. Máy chủ Delegora lưu issue, bình luận, cấu hình agent, bản ghi lượt chạy và kết quả agent trả về. Lưu ý: biến môi trường tùy chỉnh của agent được lưu trên máy chủ, nên không đặt bí mật có quyền cao ở đó.
