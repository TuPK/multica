# SDLC squad: eval pack ngoại tuyến (S9)

Kết quả bước S9 của [sdlc-harness-plan.md](sdlc-harness-plan.md), theo mục 10.4 và 10.5 của [sdlc-harness-spec.md](sdlc-harness-spec.md).

Trạng thái: **chưa chạy eval.** Đã làm: chọn 22 tác vụ từ workspace tupk và kiểm tra che thông tin. Chưa có grader, chưa có biến thể
`AGENTS.md`, và chưa có lượt chạy nào (0 trên 225 lượt của plan). Vì vậy DR2 và DR3 chưa quyết được. Mục 4 liệt kê các việc cần người quyết trước khi chạy.

## 1. Nguồn và cách chọn

- Nguồn: workspace tupk (id `50cd7d4d-…`, DB `multica`). Chỉ đọc bằng `SELECT` trong giao dịch `default_transaction_read_only=on`. Không ghi vào tupk, không gọi API ghi.
- Tiêu chí "bị trả lại", lấy từ `activity_log` và comment:
  - chuyển trạng thái `in_review → in_progress` hoặc `qa → in_progress` (rework): 16 issue;
  - có comment `Blocker-class:` thuộc `environment`, `tooling` hoặc `rework-loop`: 9 issue.
- Hợp của hai nhóm: **22 issue**, trong đó 3 issue thuộc cả hai nhóm. Plan yêu cầu 20 đến 30.
- Không dùng mua_ngon_ecommerce.

## 2. Che thông tin nhạy cảm

Kiểm tra theo pattern trên tiêu đề, mô tả và tiêu chí chấp nhận của 22 issue. Chỉ đếm số issue khớp, không in nội dung khớp.

| Loại pattern | Số issue khớp | Xử lý |
| --- | --- | --- |
| Email | 0 | Không có |
| Chuỗi giống token (`sk-`, `eyJ`, `ghp_`, `AKIA`) | 0 | Không có |
| Từ khóa bí mật (`password`, `secret`, `api key`, `service role`, `token`) | 10 | Chỉ là từ chủ đề, không phải giá trị. Không đưa mô tả vào tài liệu này |
| Host nội bộ hoặc IP | 1 | Mô tả không được đưa vào tài liệu |
| URL | 2 | Mô tả không được đưa vào tài liệu |

Tài liệu này không chứa mô tả, comment, tiêu chí chấp nhận, tên người hay URL của issue. Tiêu đề được kiểm tra riêng: 0 trên 22 khớp pattern.
Nếu một tác vụ được dùng làm đầu vào eval, phải đọc lại nội dung đầy đủ trước khi đưa vào pack, để kiểm tra bí mật mà pattern có thể bỏ sót.

## 3. Danh sách 22 tác vụ

Cột "rework" là số lần chuyển `in_review` hoặc `qa` về `in_progress`. Cột "block" là các lớp đọc từ comment `Blocker-class:`.

| # | Trạng thái | Rework | Block | Tiêu đề |
| --- | --- | --- | --- | --- |
| 80 | done | 6 | | Epic 1: Secure v2 platform foundation (Stories 1.1–1.7) |
| 95 | done | 2 | | Epic 2: Taste Mission & Buyer Journey (Stories 2.1–2.7) |
| 96 | done | 2 | | Epic 3: Supplier Discovery, Verification & Trust (Stories 3.1–3.7) |
| 97 | done | 2 | | Epic 4: Verified Matching & Explainable Offers (Stories 4.1–4.6) |
| 98 | done | 1 | | Epic 5: Verified Referral & Post-purchase Evidence (Stories 5.1–5.5) |
| 99 | done | 1 | | Epic 6: Hermes/9router AI Sourcing & RFQ (Stories 6.1–6.8) |
| 100 | done | 1 | | Epic 7: Taste Passport & Feedback Learning (Stories 7.1–7.4) |
| 101 | done | 1 | | Epic 8: Transparent Notifications & Frequency Controls (Stories 8.1–8.4) |
| 102 | done | 0 | environment | Epic 9: Operator Console & Safety Governance (Stories 9.1–9.6) |
| 103 | done | 4 | design, requirement, rework-loop | Epic 10: Metrics, Unit Economics & Pilot Readiness (Stories 10.1–10.5) |
| 117 | cancelled | 0 | environment | [drill S1] database health check |
| 123 | done | 3 | design, rework-loop | [Unblock TUPK-102] defect: close Epic 9 reviewer gaps |
| 132 | cancelled | 0 | tooling | Review protected Hermes boundary before private red-team rerun |
| 136 | done | 0 | environment | [Unblock TUPK-134] defect: allow protected profile without weakening isolation |
| 139 | done | 2 | | Review Epic 9 after accepted AC-9.5-3 evidence |
| 140 | done | 0 | environment, verification | QA: hosted authenticated operator accessibility verification |
| 144 | done | 2 | | [Unblock TUPK-140] verification: make R-010 browser fixture runnable |
| 145 | done | 1 | | Retrospective: Epic 9 Operator Console & Safety Governance |
| 156 | done | 1 | | [Unblock TUPK-105] defect: bound signed seller callback parsing |
| 168 | done | 0 | review-capacity, tooling | Fix crypto.randomUUID failure on local LAN HTTP |
| 172 | done | 1 | | Fix lỗi và trang đăng nhập |
| 186 | done | 1 | environment | Điều chỉnh trang đăng nhập |

Nhận xét về độ phù hợp làm eval:

- Nhiều tác vụ là epic, retrospective hoặc review. "Done" của chúng là phán đoán của agent, không phải kết quả của test trong repo. Chúng khó chấm tất định.
- Chỉ một phần nhỏ (ví dụ `#156`, `#172`, `#186`) là sửa lỗi có tiêu chí kiểm tra được trong code. Chưa xác định đủ số tác vụ có thể chấm tất định.
- Chưa tra cứu repo gắn với project tupk để lấy snapshot (sha) của từng tác vụ. Workspace có 2 project resource gắn repo; chưa kiểm tra chúng.

## 4. Trạng thái và việc cần người quyết

| Thành phần của plan | Trạng thái |
| --- | --- |
| Bộ tác vụ 20 đến 30, có che thông tin | Xong: 22 tác vụ (mục 1 và 2) |
| Snapshot repo (sha) và nội dung issue của từng tác vụ | Chưa làm |
| Grader tất định (lệnh Done và một test riêng cho từng tác vụ) | Chưa làm, và chưa xác định được cho đa số tác vụ |
| Ba biến thể `AGENTS.md`: V0', V1, V2 | Chưa dựng |
| Lượt chạy: 25 tác vụ × 3 biến thể × 3 lượt ≈ 225 | 0 lượt |
| Script chấm có test với tác vụ giả | Chưa làm |

Chi phí đã dùng cho S9: 0 lượt squad. Chi phí của S8 (48 lượt) được ghi ở [sdlc-harness-calibration.md](sdlc-harness-calibration.md), mục 5.

Trước khi chạy, cần người quyết:

1. **Bộ tác vụ.** Dùng 22 tác vụ tupk (đa số không chấm được tất định), hay thay bằng bộ tác vụ khác có grader rõ, ví dụ lỗi có chủ đích trên repo greenfield của S8.
2. **Người viết grader và snapshot.** Nếu dùng tupk, cần snapshot repo cho từng tác vụ.
3. **Thử trước một ô nhỏ.** Chạy một biến thể, ba tác vụ, một lượt để kiểm tra grader, rồi mới chạy đủ 225 lượt.
4. **Chính sách push.** Đã chốt: squad chỉ commit, không push lên `origin` (xem calibration 5.7). Hệ quả với eval: commit không vào `main`, nên bước kiểm tra phải chấm trên nhánh làm việc. Cách chấm chưa thiết kế.

**Quyết định của phiên này (người dùng cho phép chọn):** giữ 22 tác vụ tupk làm nguồn, nhưng chưa chạy lượt nào. Lý do: chưa có grader và chưa có snapshot repo, nên 225 lượt sẽ không cho kết quả đáng tin. Bước tiếp theo là thiết kế grader cho ít nhất một tác vụ có kiểm tra tất định, rồi chạy thử một ô (3 tác vụ, 1 biến thể, 1 lượt) trước khi tính đến chi phí đủ.

## 5. Quyết định theo DR2 và DR3

Chưa quyết. Không có pass@1, pass^3 hay token theo biến thể, nên không áp DR2 và DR3. Khi có số liệu, áp quy tắc đã đặt trước ở spec mục 10.5, không đổi quy tắc sau khi xem kết quả.
