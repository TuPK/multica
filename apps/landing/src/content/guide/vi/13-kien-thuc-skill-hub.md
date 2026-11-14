---
title: Kiến thức, kỹ năng và Hub
order: 13
---

## Knowledge base

Knowledge base giữ bản ghi lâu dài của công ty: chính sách, nghiên cứu, tài liệu sản phẩm và quyết định. Agent tìm trong đó trước khi làm việc không tầm thường và trích dẫn nguồn đã dùng. Chỉ owner và admin bật được knowledge. Khi dịch vụ tri thức ngoại tuyến, agent vẫn làm việc nhưng không có tri thức công ty cho đến khi dịch vụ trở lại.

### Bản nháp và xuất bản

Tài liệu được tách thành hai bước. Thành viên và agent tạo **bản nháp**: bản nháp được lưu nhưng chưa tìm được. Owner hoặc admin bấm **Xuất bản** để tài liệu có thể tìm kiếm, hoặc **Hủy** để bỏ. Các quyết định và bản tin của Secretary được tự động ghi vào kho quyết định để lịch sử luôn tìm được.

Mỗi kho kiến thức và mỗi loại tài liệu có thể có **chính sách xuất bản**: để người xuất bản (mặc định), hoặc tự xuất bản sau khi có người rà soát, hoặc tự xuất bản ngay. Đội dưới mức tự chủ 2 không tự xuất bản.

Tài liệu có loại (ví dụ báo cáo xác thực, phân tích, kế hoạch ra mắt, bài học rút kinh nghiệm, báo cáo số liệu). Mỗi loại có các mục bắt buộc; bản nháp thiếu mục sẽ bị từ chối.

### Tải lên và nhập

Bạn tải tệp lên hoặc nhập trang từ trang Knowledge, hoặc bấm **Lưu vào kiến thức** trên một issue để lấy nội dung làm bản nháp. Tài liệu mới được xuất bản ngay khi thêm vào nếu chính sách cho phép.

Mỗi kho có quyền truy cập riêng: giới hạn cho những đội, tầng hoặc agent cụ thể. Kho Wiki được giới hạn ba kho.

### Đồng bộ repository

Một repository có thể đồng bộ thư mục `docs` mỗi ngày vào kho kỹ thuật. Mỗi tệp Markdown trở thành một tài liệu tham chiếu; mỗi tệp cần có mục **Summary**.

### Kiểm thử truy hồi

Thêm các câu hỏi mà tìm kiếm phải trả về đúng tài liệu. Hệ thống đo tỉ lệ tìm đúng, và tỉ lệ này xuất hiện trong bản tin. Kết quả tìm kiếm luôn có thứ hạng, kể cả khi không có gì phù hợp, nên hãy kiểm tra độ liên quan trước khi tin.

## Kỹ năng

Kỹ năng là một bộ phương pháp làm việc dùng lại được trong workspace: một tệp `SKILL.md` chính, kèm script, mẫu và tài liệu tham khảo. Gắn kỹ năng vào agent để nó áp dụng hướng dẫn khi làm việc liên quan, thay vì chép cùng một phương pháp vào hướng dẫn của từng agent.

### Tạo hoặc nhập

Trên trang **Skills**, bấm **Kỹ năng mới**. Có thể viết trực tiếp, nhập từ GitHub, từ tệp `.zip` hoặc `.skill`, từ skills.sh hoặc ClawHub, hoặc sao chép từ runtime (một ảnh chụp tại thời điểm đó). Nếu tên đã tồn tại, bạn chọn ghi đè, lưu bản mới, hoặc bỏ qua.

Kỹ năng nhập từ nguồn có thể **cập nhật từ nguồn**: giữ danh tính và các gán cho agent, nhưng thay tên, mô tả, `SKILL.md` và các tệp hỗ trợ bằng bản mới. Chỉnh sửa cục bộ sẽ bị ghi đè khi cập nhật.

Gắn kỹ năng vào agent từ trang kỹ năng (**Thêm vào agent**) hoặc từ tab Skills của agent. Khi chưa cần, hãy tắt gán thay vì xóa kỹ năng. Thay đổi có hiệu lực từ lượt chạy sau.

Mọi thành viên tạo hoặc nhập kỹ năng được. Người tạo và owner, admin sửa hoặc xóa. Xóa là vĩnh viễn và gỡ kỹ năng khỏi mọi agent.

## Hub

Hub liệt kê mọi kỹ năng và plugin của workspace, kèm nguồn gốc, phiên bản, và cho biết nguồn có bản mới hơn hay không. Owner và admin quản lý **nguồn** (GitHub, URL zip, chỉ mục registry); có thể nhập token cho repository riêng, và token được mã hóa, không hiện lại.

Mọi lần nhập đều được quét an toàn trước khi cài. Kết quả là **Không có vấn đề**, **Cảnh báo** hoặc **Bị chặn**. Nguồn bị chặn không cài được. Cảnh báo phải được xác nhận trước khi nhập.

Bản đã xuất bản của plugin không đổi. Chỉnh sửa tạo bản nháp cho đến khi owner hoặc admin xuất bản phiên bản mới. Khi nguồn có bản mới, chọn **Ghi đè** (mất chỉnh sửa cục bộ) hoặc **Tách nhánh** (giữ chỉnh sửa, ngừng nhận cập nhật).

Hạn mức mặc định là 500 mục và 1 GiB mỗi workspace. Tệp nén tải lên tối đa 16 MiB.
