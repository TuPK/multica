---
title: Ứng dụng và công cụ dòng lệnh
order: 17
---

## Ứng dụng desktop

Delegora desktop có cho macOS, Windows và Linux. Ứng dụng dùng cùng tài khoản và dữ liệu với bản web, và tự quản lý daemon trên máy bạn. Bạn có thể đăng nhập cả hai cùng lúc.

Tab được lưu theo từng workspace. Đăng xuất xóa mọi tab đã lưu trên máy. Phím tắt riêng của ứng dụng (đóng tab, chuyển tab, thu phóng) chỉ hoạt động trong ứng dụng desktop.

Cập nhật tự động được bật mặc định. Trên Linux, cập nhật tự động chỉ dùng cho `.AppImage`; với `.deb` và `.rpm`, cài gói mới đè lên gói cũ.

## Ứng dụng iOS

Ứng dụng iPhone chưa phát hành trên App Store. Bạn có thể tự build bản Release từ mã nguồn trên máy Mac và cài lên iPhone của mình. Tài khoản Apple miễn phí chỉ cho cài trong 7 ngày; sau đó kết nối lại iPhone và chạy lại lệnh build để ký lại. Dữ liệu trên máy chủ không thay đổi khi bạn cài lại.

## Công cụ dòng lệnh

Công cụ dòng lệnh kết nối máy cục bộ và quản lý workspace, issue, agent, autopilot, kỹ năng và runtime từ terminal. Cài đặt bằng script chính thức hoặc Homebrew, rồi kiểm tra bằng `delegora version`.

Đăng nhập lần đầu bằng `delegora setup`: lệnh mở trình duyệt để đăng nhập và khởi động daemon. Kiểm tra bằng `delegora auth status` và `delegora daemon status`. Trên máy không có trình duyệt, tạo token cá nhân trên web rồi chạy `delegora login --token`.

Ví dụ thường dùng:

```bash
delegora workspace list
delegora issue list
delegora issue create --title "Sửa lỗi đăng nhập"
delegora issue status MUL-123 in_progress
delegora issue comment add MUL-123 --content "Kiểm tra bài test hồi quy trước"
delegora issue runs MUL-123
```

Đọc nội dung dài từ tệp hoặc stdin để tránh vấn đề về xuống dòng và dấu nháy:

```bash
delegora issue create --title "Ghi chú nâng cấp" --description-stdin < notes.md
```

Để tránh lộ token trong lịch sử shell, truyền cấu hình có bí mật qua tệp hoặc stdin thay vì tham số dòng lệnh. Tệp cấu hình của công cụ chứa token truy cập tài khoản của bạn, nên không đưa vào repository.

Chạy `delegora <lệnh> --help` để xem đầy đủ cờ của phiên bản bạn đang dùng. Đầu ra JSON (`--output json`) dành cho script.
