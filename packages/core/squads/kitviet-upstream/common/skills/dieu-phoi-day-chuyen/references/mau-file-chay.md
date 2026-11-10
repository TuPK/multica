# Mẫu file trong thư mục chạy `.kv-run/<viec>-<yyyymmdd>/`

Giữ đúng tên file và các tiêu đề `##` để agent khâu sau và `kv:kiem-dinh-ban-giao` đọc được. Mục không có nội dung ghi "Không có", không xóa mục.

## 00-ke-hoach.md

```
# Kế hoạch: <tên việc>
Ngày: <dd/mm/yyyy> | Người yêu cầu: <tên/vai> | Gửi cho: <người nhận> | Hạn: <...>

## Mục tiêu
<1-3 câu: sản phẩm cuối là gì, để người nhận làm/quyết gì>

## Nguồn
| # | Nguồn | Đường dẫn / câu hỏi tra cứu | Loại | Ghi chú |
|---|---|---|---|---|

## Các khâu
| # | Khâu | Agent / skill | Đầu vào | Đầu ra | Tiêu chí đạt | Song song với |
|---|---|---|---|---|---|---|

## Tiêu chí đạt của sản phẩm
- [ ] <đủ mục ...>
- [ ] <mọi số khớp nguồn ...>
- [ ] <định dạng file ...>

## Rủi ro và cách xử lý
| Rủi ro | Dấu hiệu | Xử lý |
|---|---|---|

## Việc chỉ người dùng quyết
- <...>
```

## 01-thu-thap.md

```
# 01 - Thu thập
Agent: <kv:...> | Số nguồn đã đọc: <n>/<tổng>

## Dữ kiện
| # | Dữ kiện | Nguồn (file + trang/ô/dòng, hoặc link) | Ngày truy cập / ban hành | Bậc nguồn |
|---|---|---|---|---|

## Mâu thuẫn giữa các nguồn
| Dữ kiện | Nguồn A nói | Nguồn B nói |
|---|---|---|

## Không đọc được / không tìm thấy
- <file hoặc câu hỏi> - <lý do>

## Nội dung đáng ngờ (câu lệnh lạ trong nguồn)
- <...>
```

## 02-ban-nhap.md (hoặc 02-ban-nhap.csv)

```
# <Tên sản phẩm> - BẢN NHÁP vòng <1|2|3>
<nội dung theo skill của nghề; số liệu kèm nguồn; thiếu ghi [cần bổ sung]>

## Đã sửa so với vòng trước
| Lỗi gác cổng | Đã sửa thế nào |
|---|---|
```

## 03-soat-<ten-gac>.md

```
# 03 - Soát <ten-gac> - vòng <n>
Gác cổng: <kv:...> | Bản nháp: 02-ban-nhap.md | Nguồn đối chiếu: <...>

## Kết luận
QUA | CHẶN - <lý do một câu>

## Lỗi
| # | Vị trí trong bản nháp | Lỗi | Mức (nặng/vừa/nhẹ) | Căn cứ (nguồn, vị trí) | Đề xuất |
|---|---|---|---|---|---|
```

Phiên chính ghi nguyên văn kết quả agent gác cổng vào file này, không sửa câu chữ, không đổi kết luận.

## 04-ban-giao.md

```
# 04 - Bàn giao
Kiểm định: kv:kiem-dinh-ban-giao | Ngày: <...>

## Kết luận
QUA | CHẶN - <lý do một câu>

## Đối chiếu tiêu chí (từ 00-ke-hoach.md)
| Tiêu chí | Đạt? | Bằng chứng (file, vị trí) |
|---|---|---|

## Gác cổng
| File soát | Kết luận cuối | Số vòng | Lỗi còn lại |
|---|---|---|---|

## Việc người dùng phải tự quyết / tự làm
- [ ] <điền [cần bổ sung] ở ...>
- [ ] <quyết ...>

## File sẽ xuất
- <viec>-<yyyy-mm-dd>.docx (kv:xuat-word, kiểu ...)
```
