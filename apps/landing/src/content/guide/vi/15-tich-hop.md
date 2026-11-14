---
title: Tích hợp
order: 15
---

Các tích hợp cho phép cả đội hỏi agent, nhắc nó trong nhóm chat, hoặc tạo issue ngay từ ứng dụng chat, không cần mở Delegora.

## Ứng dụng chat

Hỗ trợ Slack, Telegram, Lark (Feishu), WeCom và DingTalk. Mỗi bot gắn với một agent; muốn nhiều agent, tạo một bot cho mỗi agent. Người quản trị kết nối bot bằng các khóa do ứng dụng chat cấp, rồi mỗi thành viên liên kết tài khoản Delegora của mình bằng một đường dẫn một lần (hết hạn sau 15 phút). Chỉ thành viên của workspace mới dùng được bot.

- **Tin nhắn riêng**: nhắn trực tiếp cho bot, không cần nhắc tên.
- **Nhóm và kênh**: chỉ những tin nhắn có nhắc đến bot mới được xử lý. Mỗi luồng có phiên riêng, và tin tiếp theo cần nhắc lại bot.
- **Tệp đính kèm**: ảnh và tệp được chuyển cho agent (tối đa 20 MiB mỗi tệp, 10 tệp mỗi tin, tùy nền tảng).

Lệnh trong chat:

- `/issue <mô tả>`: tạo issue từ tin nhắn và giao cho agent của bot.
- `/new`: tạo một cuộc trò chuyện mới với agent. `/new <nội dung>` tạo và gửi luôn nội dung.
- `/clear`: giữ lịch sử đầy đủ nhưng làm agent quên ngữ cảnh trước đó.
- `/brief` và `/decisions`: xem bản tin mới nhất và các quyết định đang chờ (xem chương Chat và Secretary).

Ngắt kết nối trong **Cài đặt → Nhắn tin**. Việc này dừng bot nhận tin mới; hội thoại cũ vẫn được giữ.

## GitHub

Kết nối GitHub để tự liên kết pull request với issue, hiện trạng thái CI và khả năng merge, và chuyển issue khi PR được merge. Chi tiết nằm trong chương Dự án và repository.

## Dịch vụ tri thức

Kho tri thức công ty được kết nối và quản lý từ trang Knowledge. Chi tiết nằm trong chương Kiến thức, kỹ năng và Hub.

## MCP

Agent dùng được các máy chủ MCP bạn đã cấu hình, ở cấp agent hoặc từ thư viện dùng chung. Chi tiết nằm trong chương Agent.

## API công khai (v1)

API v1 cho phép tích hợp Delegora với hệ thống của bạn bằng token cá nhân hoặc token ứng dụng. Đặc tả OpenAPI có sẵn để sinh client. Đừng đưa token vào mã nguồn; lấy từ trình quản lý bí mật.

## Webhook

Autopilot có thể nhận sự kiện từ hệ thống khác và chạy việc tương ứng. Xem chương Workflow và Autopilot.
