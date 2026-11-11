# SDLC squad: agents that keep working instead of parking at the first gate

Spec. Follows [sdlc-specialist-routing.md](sdlc-specialist-routing.md), which made the Lead route every blocker to a
specialist. This one is about what that routing does not cover: gates that only a person can open, gates the squad
invented for itself, and one blocked epic freezing the whole project.

Ngôn ngữ: viết bằng tiếng Việt để người dùng đọc; tên lệnh, status, file và đoạn văn bản dành cho agent giữ nguyên
tiếng Anh, vì skill của agent viết bằng tiếng Anh.

## 1. Tóm tắt

Sau khi có định tuyến chuyên gia và Owner waiver, Epic 9 của mua_ngon đã tự gỡ được phần lớn block trong khoảng 2 giờ
(đổi key, profile Hermes, độ trễ combo). Nhưng lúc 19:09 nó lại dừng, lần này ở một chỗ mà không agent nào gỡ được:

- Review độc lập PASS, QA local CONDITIONAL GO, mọi tiêu chí chức năng của Epic 9 đã có bằng chứng.
- Lead tạo thêm TUPK-140: "kiểm tra accessibility trên môi trường hosted, đã xác thực, không thay thế bằng local".
- Runtime không có tài khoản Vercel Preview hay Supabase riêng, nên QA block (TUPK-140), DevOps xác nhận `capability`
  (TUPK-141), và đề xuất gửi chủ dự án bị từ chối vì hết hạn mức 3 proposal mỗi tuần.
- Vì hàng đợi `in_place` chạy tuần tự, Epic 10, Final QA, Final Security và Release Prep (TUPK-103 trở đi) đều
  đứng sau TUPK-102 nên cả dự án dừng.

Đó không phải lỗi của một agent. Có bốn khuyết điểm của hệ thống, đã gặp lặp lại:

1. **Điều kiện tự đặt thêm.** Lead biến một điều kiện phát hành (release gate) thành cổng chặn của Epic, và tự cấm
   phương án thay thế mà tiêu chí chấp nhận không hề cấm.
2. **Không có thang bằng chứng.** Agent chỉ biết "có" hoặc "chặn". Không có quy tắc "nếu không có hosted thì chạy local
   và ghi rủi ro còn lại".
3. **Chặn đầu hàng đợi.** Một Epic bị block chờ con người chặn mọi Epic sau nó, kể cả Epic không phụ thuộc vào nó.
4. **Kênh hỏi chủ dự án nghẹt.** Việc chỉ chủ dự án làm được phải đi qua proposal có hạn mức; khi hết hạn mức, chủ dự án
   chỉ biết nếu tự vào xem.

## 2. Bằng chứng

Tám tiếng rưỡi của Epic 9 trong mua_ngon (07/11, theo các comment và issue):

| Giờ | Loại | Chuyện gì | Ai gỡ |
| --- | --- | --- | --- |
| 10:43 | environment | Reviewer chặn: thiếu bộ red-team 9router (AC-9.5-3) | Lead chia việc |
| 11:23 | rework-loop | Lần trả lại thứ ba | Architect |
| 11:35 | design | Phán quyết R-001 tự mâu thuẫn | Architect |
| 15:24 | capability | Cần `sudo` để cài service `mng-redteam` | Bỏ yêu cầu nhờ Owner waiver O-1 |
| 17:11 | tooling | API Multica quá tải, không ghi được verdict | Tự hết |
| 17:19 | defect | Runner đặt `--safe-mode` làm Hermes bỏ qua profile | Developer |
| 17:40 | environment | Combo chọn chậm hơn giới hạn 20 giây | DevOps/Architect |
| 19:07 | environment | Không có môi trường hosted non-production | **Chưa ai gỡ được** |

Hai điều rút ra:

- Bảy trong tám block đã được gỡ trong một đến hai giờ mà không cần con người, miễn là chủ dự án đã quyết. Hệ thống
  định tuyến chuyên gia hoạt động.
- Cả hai block dài nhất (15:24 và 19:07) có cùng dạng: một **điều kiện do squad tự đặt** (tài khoản gốc, môi trường
  hosted) mà runtime không đáp ứng được. Block 15:24 chỉ hết nhờ chủ dự án nói thẳng.

Điều kiện hosted không nằm trong tiêu chí của Story 9.1–9.6. Nó đến từ phần "còn lại" của verdict QA và được Lead
chuyển thành issue chặn Epic với câu "đây là điều kiện release/pilot duy nhất còn lại; không thay thế bằng kết quả
local". Chính điều kiện đó là phát hành (release), không phải xây dựng.

Tác nhân chưa thử: chạy `/ops` trên bản preview local, đã nối với Supabase local, người dùng reviewer tổng hợp, rồi chạy
`accessibility-scan`, `accessibility-inspect` trên địa chỉ đó. Bộ kiểm tra bàn phím, tiêu điểm và screen reader chạy
được đủ trên đó; điều khác biệt so với hosted (HTTPS, CDN, CSP) là phần rủi ro còn lại và có thể ghi rõ.

## 3. Mục tiêu và ngoài phạm vi

Mục tiêu:

- Một Epic đã qua review và QA chức năng thì đóng được, kể cả khi còn điều kiện phát hành chưa chạy.
- Agent hạ bậc bằng chứng có ghi chép thay vì dừng, khi tiêu chí không cấm.
- Một Epic chờ con người không chặn Epic độc lập.
- Mọi việc chỉ chủ dự án làm được đến tay chủ dự án trong một câu hỏi gộp, kèm mặc định, không phụ thuộc hạn mức.
- Số lần block, thời gian gỡ và tỉ lệ tự gỡ được đo được.

Ngoài phạm vi:

- Thay đổi tiêu chí chấp nhận hay bỏ qua kiểm tra bảo mật vì tiện. Hạ bậc bằng chứng không bao giờ xoá một tiêu chí.
- Cho agent tự tạo tài khoản đám mây, tiêu tiền hay dùng credential của người khác.
- Sửa các lỗi test có sẵn của repo.

## 4. Nguyên tắc

1. **Không thêm cổng ngoài tiêu chí.** Lead không được tạo điều kiện chặn mà tiêu chí chấp nhận hay một phán quyết của
   chuyên gia không nêu. Điều kiện còn thiếu đi vào danh sách điều kiện phát hành, không vào hàng đợi chặn.
2. **Hạ bậc có ghi chép, không dừng.** Khi mức bằng chứng cao nhất không đạt được, thử mức kế tiếp, ghi mức đã đạt và
   rủi ro còn lại. Chỉ dừng khi tiêu chí tự nói rõ mức tối thiểu.
3. **Chờ con người không chặn người khác.** Việc cần người được đỗ (park) và hàng đợi đi tiếp qua các Epic độc lập.
4. **Hỏi một lần, gộp, có mặc định.** Câu hỏi cho chủ dự án gồm các việc đang chờ, mặc định đề xuất cho từng việc, và hạn
   dùng mặc định nếu im lặng.
5. **Phát hiện sớm.** Năng lực cần có (tài khoản, môi trường, runtime riêng) được tìm ở lúc lập kế hoạch cho cả backlog,
   không phải ở cuối Epic đầu tiên cần đến.

## 5. Thiết kế

### 5.1 Thang bằng chứng (Evidence ladder)

Mỗi kiểm tra có mức bằng chứng. Skill delivery-core định nghĩa:

| Mức | Tên | Ví dụ |
| --- | --- | --- |
| L1 | Hosted non-production | Preview Vercel + Supabase riêng + danh tính kiểm thử |
| L2 | Local giống production | Build production chạy local, Supabase local có migration và seed, người dùng tổng hợp |
| L3 | Hợp đồng/mô phỏng | Test hợp đồng, DOM kiểm thử, mock của dịch vụ ngoài |

Quy tắc:

- Tiêu chí hoặc phán quyết ghi **mức tối thiểu** của nó. Không ghi thì mức tối thiểu là L2.
- Agent chạy ở mức cao nhất đạt được và **ghi mức đã đạt** trong verdict: "đạt L2, L1 chưa chạy vì thiếu môi trường
  hosted; rủi ro còn lại: khác biệt HTTPS/CDN/CSP".
- Bằng chứng ở mức thấp hơn mức tối thiểu của tiêu chí thì mới là block, với `Blocker-class` và câu hỏi chính xác.
- Verdict QA: `GO` (đủ mức tối thiểu), `GO với điều kiện` (đủ mức tối thiểu nhưng còn mức cao hơn chưa chạy: điều kiện
  phát hành), `NO GO`. Mức cao hơn chưa chạy **không** làm `NO GO`.

### 5.2 Điều kiện phát hành (Release conditions)

- File `docs/sdlc/release-conditions.md` do QA và Lead cùng giữ. Mỗi dòng: id, điều kiện, mức cần, vì sao chưa chạy, ai
  có thể mở khoá, Epic/issue nguồn.
- Lead gắn mỗi điều kiện vào issue phát hành (Final QA, Final Security, Release Prep) bằng mention, không tạo issue chặn Epic.
- Epic được đóng với ghi chú "đóng với điều kiện RC-n". Issue Final QA và Release Prep đọc danh sách này và chạy
  điều kiện nào đã có môi trường; phần chưa chạy được thành một lời đề nghị gộp cho chủ dự án (5.4).

### 5.3 Đỗ và đi tiếp (Park and proceed)

Khi một issue bị block cần con người (`capability`, `irreversible` sau thăm dò DevOps, hoặc hạn đã hết):

1. Lead ghi issue vào danh sách `docs/sdlc/parked.md` với lý do, câu hỏi, mặc định, hạn.
2. Lead đọc `docs/planning-artifacts/epics.md` để xác định Epic kế tiếp không phụ thuộc vào Epic đang bị đỗ.
3. Với Epic độc lập, Lead xếp lại hàng bằng `multica issue queue <epic> --start-status <status>` không có `--after`,
   nên chuỗi `in_place` đi tiếp. Epic phụ thuộc thì để nguyên sau Epic bị đỗ.
4. Khi chủ dự án trả lời, Lead gỡ khỏi danh sách đỗ và đưa issue về trạng thái đã chặn.

Không cần thay đổi server: lệnh queue và mutex theo thư mục đã có.

### 5.4 Hỏi chủ dự án một lần

- **Một bản đề nghị gộp** thay cho từng proposal: comment một lần trên issue gốc của dự án, mention chủ dự án, gồm
  bảng "việc chờ – mặc định đề xuất – hạn mặc định – hậu quả nếu im lặng", cập nhật tại chỗ khi có thay đổi.
- **Mặc định khi im lặng.** Sau hạn (mặc định 24 giờ cho dev/test, không áp dụng cho `irreversible` thật như production
  và tiền), Lead thực hiện mặc định: thường là hạ bậc bằng chứng (5.1) và ghi vào `docs/sdlc/assumptions.md`.
- **Kênh dự phòng.** Khi proposal bị từ chối vì hạn mức, đề nghị gộp vẫn được đăng; không phải dừng để chờ.
- **Platform (tuỳ chọn, giai đoạn sau).** Các proposal loại `capability` và `security` không tính vào hạn mức mỗi tuần,
  hoặc có thông báo hộp thư cho chủ dự án khi một issue chờ con người quá N giờ.

### 5.5 Kiểm tra năng lực ngay từ đầu (Capability preflight)

Khi lập kế hoạch cho cả backlog, Lead đọc tất cả Epic và Story và lập bảng năng lực cần:

- môi trường hosted, tài khoản đám mây, runtime riêng, dịch vụ ngoài, danh tính kiểm thử;
- với mỗi mục: Epic cần, mức bằng chứng tối thiểu (5.1), DevOps có thể tự dựng không, hay cần chủ dự án.

Bảng ghi vào `docs/sdlc/capabilities.md`. Mọi mục cần chủ dự án được gộp thành **một** câu hỏi đầu dự án. Với mỗi mục
chủ dự án chưa trả lời, mặc định là L2 (5.1). DevOps chạy kiểm tra thăm dò ở Chế độ Environment cho các mục tự dựng
được trước khi Epic cần đến.

### 5.6 Giới hạn block và đo lường

- **Ngân sách block** mỗi Epic: sau block thứ ba cùng loại `environment` hoặc `capability`, Lead dừng định tuyến lẻ và
  tạo **một** issue DevOps "dựng đủ năng lực cho Epic" kèm bảng 5.5, thay vì thêm một vòng nữa.
- **Số liệu** trong báo cáo cuối Epic của Lead: số block, loại, thời gian từ block đến resume, số lần tự gỡ không cần
  con người, số điều kiện phát hành mở. Dữ liệu lấy từ comment, nên không cần thay đổi server.

## 6. Thay đổi theo thành phần

| Thành phần | Thay đổi |
| --- | --- |
| `sdlc-delivery-core` | Thêm "Evidence ladder" và "Release conditions"; verdict GO với điều kiện; quy tắc không thêm cổng ngoài tiêu chí |
| `sdlc-squad-lead` | Readiness: Capability preflight (5.5); Step 6: điều kiện phát hành không chặn Epic; Step 7: đỗ và đi tiếp (5.3), đề nghị gộp (5.4), ngân sách block (5.6) |
| `team-playbook.md` | Mẫu `parked.md`, `release-conditions.md`, `capabilities.md`, bản đề nghị gộp |
| `sdlc-qa-verification` | Ghi mức bằng chứng đạt được; điều kiện cao hơn chưa chạy là điều kiện phát hành; thử L2 trước khi báo `environment` |
| `sdlc-devops-release` | Chế độ Environment tự dựng L2 (bản preview local, dữ liệu seed, danh tính tổng hợp); báo L1 là `capability` chỉ khi tiêu chí đòi |
| Agent instructions | Lead và QA: một dòng trỏ tới các quy tắc mới (giữ giới hạn 2300 ký tự của Lead) |
| Server | Không đổi ở giai đoạn đầu. Giai đoạn sau: hạn mức proposal hoặc hộp thư (5.4) |

## 7. Áp dụng ngay cho Epic 9 (đề xuất, chờ chủ dự án quyết)

Cần một Owner decision `O-2`:

1. TUPK-140 và TUPK-141: đóng, chuyển điều kiện "accessibility hosted" thành `RC-1` trong `release-conditions.md`, gắn vào
   TUPK-104 (Final QA) và TUPK-106 (Release Prep).
2. Chạy kiểm tra accessibility ở mức L2: build production của `/ops` chạy local, Supabase local, reviewer tổng hợp, rồi
   `accessibility-scan` và `accessibility-inspect`. Ghi rõ "đạt L2; L1 chưa chạy".
3. Đưa TUPK-102 qua retrospective để về `done` với điều kiện RC-1.
4. Giải phóng TUPK-103 (Epic 10) và các issue sau.

## 8. Kiểm chứng

- Test nội dung skill: thang bằng chứng, điều kiện phát hành, đỗ và đi tiếp, đề nghị gộp, ngân sách block.
- Diễn tập trên agent thật (dự án mua_ngon hoặc dự án thử):
  - A1: QA gặp tiêu chí cần hosted mà chỉ có local → ghi L2 và điều kiện phát hành, không block.
  - A2: một issue cần tài khoản đám mây → Lead đỗ nó, Epic độc lập tiếp theo chạy.
  - A3: proposal bị từ chối vì hạn mức → đề nghị gộp vẫn được đăng và Lead không dừng.
  - A4: chủ dự án im lặng quá hạn → Lead thực hiện mặc định L2 và ghi giả định.
- Số liệu trước/sau: số block mỗi Epic, thời gian gỡ trung bình, số Epic đứng yên chờ con người.

## 9. Rủi ro

| Rủi ro | Giảm thiểu |
| --- | --- |
| Hạ bậc bằng chứng che mất lỗi chỉ hiện ở hosted | Mức tối thiểu ghi trong tiêu chí; điều kiện phát hành bắt buộc nêu rủi ro còn lại và được chạy ở Final QA khi có môi trường |
| Epic độc lập thực ra phụ thuộc ngầm | Lead đọc phụ thuộc từ epics.md; nghi ngờ thì không bỏ qua hàng đợi và hỏi trong đề nghị gộp |
| Mặc định khi im lặng làm việc chủ dự án không muốn | Chỉ áp dụng cho dev/test; `irreversible` thật (production, tiền, dữ liệu người dùng thật) luôn chờ |
| Agent lạm dụng "hạ bậc" để né kiểm tra khó | Mức tối thiểu của tiêu chí không bị hạ; verdict luôn nêu mức đã đạt |

## 10. Quyết định đã chọn và phương án đã loại

- **Chọn:** thay đổi skill và quy trình, không đổi server ở giai đoạn đầu. Mọi cơ chế (queue, wakeup, comment, file
  docs) đã có.
- **Loại: tăng hạn mức proposal** như giải pháp chính. Nó không sửa nguyên nhân (cổng tự đặt, chặn đầu hàng đợi).
- **Loại: cho agent bỏ qua điều kiện khi bị block lâu.** Dễ che lỗi thật. Hạ bậc có ghi chép an toàn hơn.
- **Loại: song song hoá nhiều Epic ở thư mục `in_place`.** Mutex theo thư mục tồn tại để tránh xung đột; đỗ và đi tiếp
  giữ nguyên nó. (Riêng các issue của hai phòng khác nhau trong cùng dự án nay chạy song song: xem lane theo phòng
  trong `ListReleasableQueuedIssues` và `LocalPathLocker`. Epic trong cùng một phòng vẫn xếp tuần tự.)
