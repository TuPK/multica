# Open Company Format (ocf/1)

Open Company Format là cách mô tả một công ty agent bằng dữ liệu mở: cấu trúc (tầng, ghế, squad, lộ trình, chính sách, mục tiêu, ngân sách, mức tự chủ) và, khi người xuất chọn, lịch sử (kết quả đã chốt, uy tín, nhật ký quản trị, hồ sơ ghế). Mục tiêu là dữ liệu của khách luôn mang đi được, và một framework khác chỉ cần một adapter để đọc hoặc ghi định dạng này.

Đây là đặc tả của F11 trong chương trình durable (`docs/specs/durable-platform/spec.md`, mục 12). Trạng thái: bản nháp cho E2, chưa có trên nhánh chính. Các phần còn lại của thư mục này:

| File | Nội dung |
| --- | --- |
| `v1.schema.json` | JSON Schema draft 2020-12 của gói `ocf/1`. Nguồn chuẩn cho máy. |
| `README.md` | File này: đọc cho người. |
| `worker-protocol.md` | Giao thức của một worker bên ngoài và adapter CLI tham chiếu. |

## Giấy phép

- Tài liệu trong thư mục này (README và worker-protocol.md): [CC-BY-4.0](https://creativecommons.org/licenses/by/4.0/). Bạn có thể dùng, chia sẻ và sửa, kể cả cho mục đích thương mại, nếu ghi nguồn.
- Schema `v1.schema.json`: [MIT](https://opensource.org/licenses/MIT).
- Giấy phép của repository không đổi. Mã nguồn của Delegora vẫn theo giấy phép của repo.

## Gói xuất

Một gói là một file `.ocf.zip` có các tệp sau:

```
manifest.json                       bắt buộc
structure.json                      bắt buộc
history/outcomes.ndjson             tuỳ chọn: kết quả đã chốt
history/track_records.ndjson        tuỳ chọn: uy tín
history/governance.ndjson           tuỳ chọn: nhật ký quản trị
history/dossiers.ndjson             tuỳ chọn: hồ sơ ghế đang dùng
history/principals.ndjson           tuỳ chọn: chỉ có khi mọi người trong gói đã đồng ý
attestations/*.json                 tuỳ chọn: chứng nhận ký số (ocf-attestation/1)
```

`manifest.json` liệt kê phần nào có trong gói ở trường `parts`. Một gói chỉ xuất lịch sử khi người xuất yêu cầu (`multica company export --history`).

### manifest.json

Trường: `format` (phải là `"ocf/1"`), `generator` (ví dụ `delegora/0.9.0`), `exported_at`, `workspace` (`name` và `slug`), `parts`. Đối tượng này đóng hoàn toàn: trường lạ bị từ chối.

### structure.json

Cùng tên trường với export hiện có (`GET /api/company/export`), thêm hai mảng tuỳ chọn `envelopes` và `effect_policy_overrides` sẽ có khi các tính năng F2 và F3 được bật.

| Khoá | Nội dung |
| --- | --- |
| `floors`, `squads`, `agents`, `floor_chiefs` | Tầng, squad (kèm `charter`, `routines`, `autonomy_level`, vị trí `company_slot`), agent, trưởng tầng |
| `pipelines`, `initiatives`, `knowledge_kinds` | Lộ trình, sáng kiến chưa đóng, định nghĩa loại tri thức |
| `goals`, `budgets`, `approval_policies` | Mục tiêu và kết quả chính, ngân sách, chính sách phê duyệt đứng |
| `envelopes`, `effect_policy_overrides` | (F2, F3) giới hạn tự chủ theo squad; ô bị ghi đè trong bảng chính sách hệ quả |
| `governance` | Mục dành cho phần quản trị sẽ được thêm sau. v1 ghi `{}` |

Các tham chiếu đều dùng tên, không dùng id: squad là tên squad, agent là tên agent, sáng kiến là bộ ba `pipeline`, `stage`, `title`. Id chỉ có nghĩa trong một workspace, nên không đi theo gói.

### history/*.ndjson

Mỗi dòng là một JSON object. Không có nội dung issue, bình luận, tiêu chí, bằng chứng hay tài liệu. Nhật ký quản trị không mang tên người hành động, chỉ có `actor_type`.

## Ví dụ manifest tối thiểu

Đây là một gói hợp lệ chỉ có cấu trúc:

```json
{
  "format": "ocf/1",
  "generator": "delegora/0.9.0",
  "exported_at": "2026-11-25T00:00:00Z",
  "workspace": { "name": "Example Co", "slug": "example-co" },
  "parts": ["structure"]
}
```

`parts` phải chứa `"structure"`. Gói có `"history/outcomes"` trong `parts` thì phải có file tương ứng.

## Quy tắc ngoài schema

JSON Schema không diễn đạt được các quy tắc sau. Trình đọc (server khi import, và CI khi kiểm mẫu) phải kiểm riêng:

1. `parts` khớp các file có trong gói: mỗi phần được liệt kê phải có file, và mỗi file có mặt phải được liệt kê.
2. `envelopes[].floor_level` không vượt `ceiling_level`.
3. `key_results[].target_value` khác `start_value`.
4. `goals[].parent` là tiêu đề của một mục tiêu khác trong cùng `goals`. Tiêu đề không duy nhất, nên khi trùng thì đọc theo thứ tự xuất hiện và báo lỗi.
5. Một attestation chỉ hợp lệ khi chữ ký Ed25519 khớp với khoá công khai của `issuer` (spec 13.2). Schema không kiểm chữ ký.
6. Một agent được chứng nhận phải có tổng `n` từ 30 kết quả đã chốt trở lên trong kỳ. Đây là luật của bên phát hành, không phải của schema.
7. Gói có `history/principals.ndjson` chỉ hợp lệ khi người xuất có quyền và mỗi người trong file đã đồng ý xuất (spec 6.6).

## Nhập gói

Trình nhập đọc:

- `ocf/1`: có `manifest.json`.
- Định dạng export cũ: một file JSON duy nhất, không zip, không có `manifest.json`, có `schema_version: 1`. Trình nhập coi nó như `structure.json` và bỏ qua phần lịch sử. Vẫn nhập được.

Lịch sử nhập vào workspace mới được đánh dấu `imported = true` và **không** tạo uy tín: bản ghi `track_records` chỉ để xem, không dùng cho chế độ kiểm chứng hay autonomy envelope. Uy tín chỉ mang sang được qua attestation có chữ ký (F12). Lý do: một gói tự khai có thể ghi bất kỳ con số nào.

Gói sai theo schema bị từ chối trước khi nhập. Lỗi chỉ ra đường dẫn của trường sai, ví dụ `/workspace` cho trường lạ bên trong `workspace`.

## Phiên bản

`format` là phiên bản lớn. Thêm trường tuỳ chọn hoặc thêm giá trị ghi chú vào mô tả là thay đổi tương thích và giữ `ocf/1`. Đổi kiểu, xoá trường, hoặc đổi nghĩa một enum là thay đổi không tương thích và tạo `ocf/2`. Trình đọc phải từ chối một `format` mình không biết.

Giá trị enum trong schema là giá trị đã có trong mã và spec tại thời điểm viết. Một giá trị mới cần một bản schema mới.

## Kiểm tra gói

Dùng một trình kiểm JSON Schema draft 2020-12 có hỗ trợ format `date`, `date-time`. Kiểm `manifest.json` với gốc schema, rồi kiểm từng file khác với `$defs` tương ứng: `structure` cho `structure.json`, từng entry cho từng dòng `.ndjson`, `attestation` cho `attestations/*.json`.

Server kiểm gói theo schema trước khi nhập. CI kiểm mọi export mẫu trong test.
