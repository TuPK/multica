---
title: Công ty, đề xuất và pipeline
order: 10
---

Trang **Công ty** vẽ workspace của bạn như một công ty. Đội nằm trong phòng, phòng nằm trên tầng, và agent ngồi tại bàn trong phòng của đội chính. Mỗi agent hiển thị nó đang làm gì, và việc nào đang chờ quyết định của bạn.

### Tầng, phòng và bàn

Một tầng có sáu phòng (ba cột, hai hàng). Tên tầng do bạn đặt; tối đa 24 tầng. Mỗi phòng có mười bàn: trưởng nhóm ngồi trước, sau đó là các agent theo thứ tự tham gia. Thành viên là người không hiển thị trên sơ đồ. Đội chưa có chỗ sẽ nằm trong khay "Chưa trên tầng", và có thể xếp tất cả chỗ trống bằng **Xếp tất cả**.

Mỗi agent có một **đội chính**, nơi nó có bàn. Agent có thể thuộc nhiều đội, nhưng chỉ hiện ở đội chính.

### Quyết định và đề xuất

Đề xuất (proposal) là yêu cầu của đội để bạn quyết định. Mỗi đề xuất có tóm tắt, ít nhất một liên kết chứng cứ, khuyến nghị, phương án đã cân nhắc, chi phí và rủi ro, và điều đội đề nghị bạn chấp thuận. Đề xuất không thông báo riêng; nó nằm trong hàng **Quyết định**, và bản tin hằng ngày của Secretary liệt kê chúng.

- **Duyệt**: đề xuất được chấp thuận. Nếu có đội được giao việc tiếp theo, hệ thống tạo dự án và issue khởi đầu cho đội đó.
- **Từ chối**: đóng đề xuất, ghi ghi chú của bạn thành bình luận gắn trưởng nhóm để đội chỉnh và nộp lại.
- **Yêu cầu sửa** và **Hoãn lại** cũng có sẵn.

Chỉ owner và admin quyết định được. Agent, kể cả Secretary, không tự duyệt.

Mỗi đội có giới hạn số đề xuất đang chờ, và giới hạn số đề xuất nộp trong bảy ngày. Workspace giữ tối đa mười đề xuất đang chờ.

### Mục tiêu và chỉ số

Một **mục tiêu** nêu điều công ty, một tầng, một đội hoặc một initiative đang hướng tới trong một kỳ. Mục tiêu có các **kết quả chính** (key results), và mỗi kết quả theo dõi một **chỉ số**. Tiến độ và độ tin cậy được tính từ chỉ số, nên không ai phải viết báo cáo trạng thái.

Chỉ số có ba nguồn: chỉ số dựng sẵn (tính hằng đêm từ dữ liệu workspace, như số issue xong, tỉ lệ thành công của lượt chạy, chi phí), chỉ số nhập tay, và chỉ số qua webhook (ví dụ số đăng ký, doanh thu).

Độ tin cậy có ba mức: đúng tiến độ, có rủi ro, lệch tiến độ. Chủ sở hữu có thể ghi đè kèm ghi chú.

### Pipeline và initiative

Một **initiative** là một hướng kinh doanh: một sản phẩm, một dòng tính năng hay một thử nghiệm thị trường. Initiative đi qua **pipeline** gồm bảy giai đoạn cố định:

1. **Validate** (kiểm chứng)
2. **Define** (xác định)
3. **Plan** (lập kế hoạch)
4. **Build** (xây dựng)
5. **Launch** (ra mắt)
6. **Operate** (vận hành)
7. **Learn** (rút kinh nghiệm)

Mỗi giai đoạn do một đội phụ trách và yêu cầu những artifact nhất định, thường là tài liệu kiến thức. Đội phụ trách xác nhận giai đoạn xong, và initiative tiến lên khi artifact đã đủ.

Cổng ra mắt (Launch) luôn là quyết định của bạn, ở mọi mức tự chủ. Cổng ở giai đoạn Operate có ba kết quả: tiếp tục, mở rộng (mở initiative mới ở Define) hoặc đóng.

Initiative bắt đầu từ một cơ hội được duyệt (đề xuất kiểu `opportunity`), hoặc bạn tạo trực tiếp từ trang Pipeline. Bạn có thể gửi lại một giai đoạn trước, tạm hoãn (đồng hồ giai đoạn dừng), hoặc đóng với lý do.

### Bản tin hằng ngày của Secretary

Secretary soạn một bản tin mỗi sáng từ tổng hợp của công ty. Bản tin gồm: tiêu đề, các quyết định đang chờ, tiến độ theo tầng, mục tiêu đổi trạng thái, ngân sách, pipeline, nhân sự, rủi ro, và phần ghi chú. Bản tin khoảng hai phút đọc. Bạn có thể đánh giá "hữu ích" hoặc "không hữu ích" để Secretary điều chỉnh. Xem thêm chương Chat và Secretary.

### Điều lệ mẫu và công ty khởi đầu

Trình hướng dẫn **công ty khởi đầu** tạo sẵn một cấu trúc: Secretary, thủ thư kiến thức, và các đội nghiên cứu, sản phẩm, xây dựng, ra mắt và quản trị. Mỗi đội có điều lệ và vòng định kỳ. Chạy lại trình hướng dẫn không tạo bản sao. Ngân sách khởi đầu là 500 USD mỗi tháng ở chế độ cảnh báo, không giữ lượt chạy nào.
