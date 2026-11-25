# Delegora bền vững: chiến lược để nền tảng không lỗi thời khi AI tiến bộ

Trạng thái: chiến lược, đã chốt hướng. Đặc tả: [spec.md](spec.md). Kế hoạch: [plan.md](plan.md).
Ngày viết: 2026-11-25, trên HEAD `21cbcdd90`.
Ngôn ngữ: tiếng Việt. Định danh, đường dẫn và lệnh giữ nguyên tiếng Anh. Tên hiển thị là **Delegora**, định danh giữ `multica` ([fork-branding](../../fork-branding.md)).
Nền: tài liệu này xây trên [digital-company](../../engineering/digital-company.md) (Phase 1 đến 4: Office, Secretary, knowledge, pipeline, goals, budget, autonomy, standing approval, review, governance log). Tài liệu đó mô tả *công ty số đang có*. Tài liệu này mô tả *vì sao và bằng cách nào công ty số đó không lỗi thời*.

## 1. Vấn đề

Ngành AI agent đang tạo ra rất nhiều kỹ thuật bù cho chỗ mô hình còn yếu: skill, harness, prompt template, loop engineering, workflow cứng nhiều bước, framework điều phối (LangGraph, CrewAI, AutoGen, Agno…). Mỗi thế hệ mô hình mới làm một phần các kỹ thuật này thừa. Một nền tảng dựa trên chúng sẽ lỗi thời cùng chúng.

Delegora hôm nay có cả hai loại tài sản:

| Loại | Ví dụ trong Delegora | Khi mô hình giỏi gấp 10 lần |
| --- | --- | --- |
| **Giàn giáo** (bù năng lực) | skill (`catalog/skills`), instruction template, flow nhiều cổng (`catalog/workflows`), harness SDLC, skillset, agent pack | Giá trị **giảm** |
| **Thể chế** (quan hệ giữa người, agent và thế giới) | issue, goal và key result, budget, autonomy, standing approval, governance log, proposal, knowledge, review, company export | Giá trị **tăng** |

Phần thể chế đã được xây khá đầy đủ ở Phase 4. Nhưng nó còn ba điểm yếu, và cả ba đều làm Delegora phụ thuộc vào giàn giáo nhiều hơn cần thiết:

1. **Bằng chứng yếu.** Scorecard dùng số liệu thay thế: thời điểm xong là `issue.updated_at`, không có lịch sử mở lại, và chỉ tính chi phí do nhà cung cấp báo ([Phase 4 deviations](../../engineering/digital-company.md#phase-4-deviations)). Nền tảng biết agent *chạy* bao nhiêu lần, nhưng không biết chắc agent *làm đúng* bao nhiêu việc.
2. **Tự chủ đứng yên.** `squad.autonomy_level` chỉ đổi khi người chủ đổi bằng tay. Mô hình giỏi lên, nhưng quyền của nó không lớn theo. Tiến bộ của AI không chuyển thành giá trị cho người dùng.
3. **Người là nút thắt.** Mỗi proposal, mỗi xác nhận ghi (`confirm_writes`), mỗi cổng đều chờ người. Giới hạn chú ý (3/3/10) chỉ đếm *số câu hỏi*, chưa đo *thời gian của người*, và chưa có cách nào giúp người trả lời nhanh hơn.

## 2. Luận điểm

> **Delegora là thể chế của công việc do agent làm, không phải công cụ để agent làm việc.**

Công ty, luật lao động, kế toán, kiểm toán không biến mất khi nhân viên giỏi lên. Nhân viên càng giỏi thì càng được giao việc lớn, và thể chế càng quan trọng. Một agent ngang chuyên gia giỏi nhất vào năm 2030 vẫn cần một công ty trao cho nó: ủy quyền, ngân sách, danh tính để ký, tài khoản để truy cập, biết người ta muốn gì, một cách để bị dừng lại, và một lý lịch để được tin.

### 2.1 Năm thứ trí thông minh không thay được

| # | Thứ khan hiếm | Vì sao không mô hình nào tự tạo ra | Delegora giữ nó ở đâu |
| --- | --- | --- | --- |
| S1 | **Tính chính danh**: ai cho phép | Quyền do người trao. Mô hình không tự cấp quyền cho mình | Autonomy, budget, approval policy, Effect Registry (F3) |
| S2 | **Tiếp xúc với thực tế**: việc có thành ngoài đời không | Người làm không tự chấm bài của mình được. Thực tế chỉ đến qua số đo bên ngoài | Outcome Record (F1), metric webhook, Jury (F7) |
| S3 | **Ý muốn của con người**: muốn gì, ghét gì, đánh đổi ra sao | Nằm trong đầu người cụ thể, không có trong dữ liệu huấn luyện | Goals, Principal Profile (F5) |
| S4 | **Thời gian**: uy tín tích qua thời gian | Không thông minh đến đâu cũng không giả được hai năm lịch sử đã kiểm chứng | Track Record (F2), Seat Dossier (F8) |
| S5 | **Sự chú ý của con người** | Agent gần như vô hạn, còn người chủ chỉ có vài phút mỗi ngày | Attention Budget (F6), brief |

S4 là con hào thật sự: đối thủ có thể sao chép tính năng, nhưng không sao chép được lịch sử đã kiểm chứng của từng công ty.

### 2.2 Phép thử 10x

Mọi tính năng mới, và mọi tính năng cũ khi xem lại, đều qua câu hỏi:

> **Nếu ngày mai mô hình thông minh gấp 10 lần, tính năng này có giá trị hơn hay kém đi?**

- **Hơn**: đầu tư, làm thành lõi, đưa vào chuẩn mở (F11).
- **Kém đi**: giữ mỏng, cắm-rút được, có bài kiểm tra cần thiết (F9), không để lõi phụ thuộc vào nó.
- **Không rõ**: tách phần bền (dữ liệu, hợp đồng, quyền) khỏi phần tạm (cách làm), rồi làm lại phép thử cho từng phần.

Ví dụ: một flow Kit Việt có hai phần. *Các cổng và hiện vật bắt buộc* (thế nào là xong một bước) là phần bền. *Lời nhắc từng vai phải làm gì* là phần tạm.

### 2.3 Vòng quay

Các tính năng không rời rạc. Chúng tạo thành một vòng tự tăng tốc:

```
   Outcome Record (F1)  ──►  Track Record (F2)  ──►  Autonomy Envelope (F2)
   kết quả đã kiểm chứng      uy tín theo ghế         quyền tự nới hoặc tự thu
          ▲                     và loại việc                    │
          │                                                     ▼
   Jury (F7) và Rehearse (F4)                     ít chờ người hơn, làm được nhiều hơn
   kiểm chứng rẻ hơn, an toàn hơn                               │
          ▲                                                     ▼
          └──── Seat Dossier + Shadow Run (F8): mô hình mới vào ghế cũ, mang theo uy tín
```

Mô hình càng giỏi, vòng quay càng nhanh. Đây chính là tính chất cần có: **AI càng tiến bộ, Delegora càng có giá trị**, vì nền tảng chuyển tiến bộ đó thành quyền được kiểm soát và thành thời gian rảnh của người chủ.

## 3. Bài học từ bên ngoài

Ba dự án nổi bật cuối 2026 được xem xét có chủ đích. Đây là những gì Delegora lấy và không lấy.

| Dự án | Nó làm gì | Phần sẽ lỗi thời | Phần bền Delegora lấy |
| --- | --- | --- | --- |
| [HKUDS/CLI-Anything](https://github.com/HKUDS/CLI-Anything) (64.7k sao) | Đọc mã nguồn một phần mềm (GIMP, Blender, FreeCAD…) rồi sinh CLI có JSON output, undo/redo và test. `refine` chạy gap analysis giữa năng lực thật của phần mềm và độ phủ của CLI. Có preview và trajectory loop | Bản thân lớp CLI: khi computer-use đủ tốt thì không cần bọc nữa | (a) Đo **độ phủ** → Reach Map (F10). (b) **Undo** là thuộc tính hạng nhất của hành động → Effect Registry (F3). (c) **Preview** trước khi commit → Rehearse (F4) |
| [titanwings/distilly](https://github.com/titanwings/distilly) (26.8k sao) | Chưng cất kinh nghiệm, kiểu ra quyết định và giọng văn của một người thành Person Profile có dẫn nguồn, có version, sửa dần qua hội thoại, mang sang nhiều agent host | Đóng gói thành `SKILL.md` | Thứ khan hiếm là **phán đoán và khẩu vị của người**, không phải kỹ năng → Principal Profile (F5). Kinh nghiệm phải **mang sang được** giữa các host → Seat Dossier (F8) |
| [ashishpatel26/500-AI-Agents-Projects](https://github.com/ashishpatel26/500-AI-Agents-Projects) (40.7k sao) | Danh mục hơn 500 use case theo framework và theo ngành | Lớp framework: danh mục này cho thấy framework thay đổi rất nhanh | **Công việc của từng ngành** là bền (thế nào là xong, kiểm thế nào), `catalog/domains` đã đi đúng hướng. Không chọn phe framework: làm **nơi mọi framework đến làm việc** → Open Company Format (F11), Labor Market (F12) |

Distilly còn để lại một bài học về giới hạn: dự án tự nói nó "không tuyên bố nhân bản con người". Principal Profile (F5) giữ nguyên ranh giới đó: chỉ dựa trên chứng cứ trong workspace, có sự đồng ý của người được mô hình hoá, xem và sửa được, xoá được.

## 4. Mười hai tính năng

Nhóm theo năm thứ khan hiếm. Chi tiết ở [spec.md](spec.md).

| # | Tính năng | Khan hiếm | Một câu | Xây trên |
| --- | --- | --- | --- | --- |
| F1 | **Outcome Record** | S2 | Mỗi việc xong có một bản ghi kết quả đã kiểm chứng: ai kiểm, bằng chứng gì, sau đó có bị mở lại không | issue, task, scorecard, initiative artifact |
| F2 | **Track Record + Autonomy Envelope** | S1, S4 | Uy tín theo cặp (agent, loại việc), và mức tự chủ tự lên xuống trong khoảng người chủ cho phép | scorecard, autonomy L0–L3, governance log |
| F3 | **Effect Registry** | S1 | Mọi hành động đi qua nền tảng đều có nhãn hệ quả (đọc, ghi, gửi ra ngoài, xoá, tốn tiền) và cách hoàn tác | `connectedapps` (classify, policy), plugin action, CLI |
| F4 | **Rehearse & Preview** | S1, S5 | Hành động không đảo ngược được đi qua một bản nháp có preview; người duyệt *kết quả*, không duyệt *kế hoạch* | flow rehearsal, `confirm_writes` |
| F5 | **Principal Profile** | S3 | Hồ sơ phán đoán của người chủ, học từ quyết định thật; agent dự đoán trước, người chỉ xác nhận | proposal, decision, knowledge |
| F6 | **Attention Budget** | S5 | Thời gian của người là một loại ngân sách có giá; câu hỏi phải gộp, có câu trả lời dự đoán, có lý do | squad_attention, brief, budget |
| F7 | **Disagreement Jury** | S2 | Việc quan trọng được hai, ba mô hình khác hãng kiểm độc lập; bất đồng là chỗ cần người | nhiều runtime, review |
| F8 | **Seat Dossier + Shadow Run** | S4 | Kinh nghiệm thuộc về ghế, không thuộc về mô hình; mô hình mới chạy bóng trên việc thật trước khi vào ghế | agent, instruction revision, probation |
| F9 | **Scaffolding Sunset** | phép thử 10x | Skill và flow có bài kiểm tra cần thiết; bỏ đi mà kết quả không kém thì đề xuất gỡ | skill, flow, F1 |
| F10 | **Reach Map** | S1, S5 | Bản đồ những gì agent đã chạm được trong công ty và những gì còn phải nhờ người | F3, F6, connected apps |
| F11 | **Open Company Format** | S4 | Chuẩn mở, có version, cho cấu trúc và lịch sử của một công ty agent; framework nào cũng vào được qua adapter | company export/import |
| F12 | **Agent Labor Market** | S1, S4 | Hub thành chợ lao động có bảo chứng: agent bên ngoài nộp đơn vào ghế, mang theo track record | Hub, F2, F11 |

## 5. Thứ tự và lý do

| Giai đoạn | Tính năng | Vì sao ở đây |
| --- | --- | --- |
| **A. Bằng chứng** | F1, F2 | Gốc dữ liệu của mọi thứ khác. Rẻ nhất, vì chủ yếu nối các bảng Phase 4 đã có |
| **B. Hệ quả** | F3, F4 | Biến Automation First từ chữ thành luật được máy cưỡng chế. Cần trước khi nới quyền nhiều (F2 mức L3) |
| **C. Con người** | F5, F6 | Giảm thời gian chờ người. Đây là chỗ khác biệt rõ nhất với mọi nền tảng khác. Cần dữ liệu quyết định của giai đoạn A |
| **D. Chống lỗi thời** | F7, F8, F9 | Mô hình mới vào ghế an toàn và giàn giáo tự dọn. Cần F1 để so sánh |
| **E. Mở ra ngoài** | F10, F11, F12 | Tầng mạng lưới, khó sao chép nhất. Cần cả bốn giai đoạn trước để có dữ liệu đáng tin |

Mỗi giai đoạn tự đứng được: dừng sau giai đoạn nào thì phần đã làm vẫn có giá trị riêng.

## 6. Nguyên tắc thiết kế (bắt buộc cho mọi tính năng)

1. **Không phụ thuộc mô hình.** Không bảng, không API, không quy tắc nào giả định một nhà cung cấp, một runtime hay một kỹ thuật prompt. Dữ liệu bền viết bằng văn bản trung lập và số đo.
2. **Đo kết quả, không đo cách làm.** Nền tảng không chấm *agent làm thế nào*. Nó chấm *việc có thành không*.
3. **Bằng chứng trước quyền.** Không tính năng nào nới quyền nếu không dựa trên bản ghi kết quả đã kiểm chứng.
4. **Thu quyền nhanh hơn trao quyền.** Hạ mức khi có sự cố là tự động và tức thì. Nâng mức thì cần thời gian và mẫu đủ lớn.
5. **Người chủ đặt khung, máy chạy trong khung.** Người quyết định trần và sàn một lần. Máy di chuyển trong đó và ghi lý do. Điều này đúng tinh thần [Automation First](../../../AGENTS.md#automation-first): chỉ dừng chờ người ở những chỗ spec yêu cầu.
6. **Thành thật về giới hạn.** Delegora không nhìn được bên trong một lần chạy của runtime (đã nói ở [digital-company 4.3](../../engineering/digital-company.md#43-autonomy-levels)). Tính năng nào chỉ kiểm soát được những gì đi qua nền tảng thì phải nói đúng như vậy trên giao diện.
7. **Mở trước, khoá sau.** Dữ liệu của công ty xuất ra được theo chuẩn mở (F11). Nền tảng giữ khách bằng giá trị của vòng quay, không bằng việc khoá dữ liệu.
8. **Code của fork nằm ở file của fork.** Tính năng mới là file mới (`*_fork.go`, `*.fork.ts`, `router_fork.go`); file upstream chỉ nhận hook một dòng ([upstream-sync](../upstream-sync/spec.md)).

## 7. Không làm

- **Không đua framework.** Không viết agent loop riêng, không cạnh tranh với LangGraph, CrewAI hay bất kỳ harness nào.
- **Không tự bọc mọi phần mềm.** CLI-Anything, MCP, computer-use đều được dùng như nguồn hành động. Delegora chỉ giữ Effect Registry ở trên chúng.
- **Không gỡ skill và flow hiện có.** Chúng vẫn tạo giá trị hôm nay. Chỉ đổi vị thế: chúng là plugin tạm thời, có bài kiểm tra cần thiết (F9).
- **Không xếp hạng con người.** Track record và review là công cụ làm việc cho agent và squad, không phải bảng xếp hạng nhân viên. Principal Profile không bao giờ được dùng để đánh giá người được mô hình hoá.
- **Không tuyên bố "không cần giám sát".** Mức tự chủ cao nhất vẫn bị chặn bởi budget, Effect Registry và các quyết định luôn thuộc về người (launch, thay đổi tổ chức).

## 8. Thước đo thành công

Đo ở mỗi workspace bật chương trình, so với 30 ngày trước khi bật:

| Thước đo | Định nghĩa | Mục tiêu sau giai đoạn C |
| --- | --- | --- |
| **Tỷ lệ việc có kết quả đã kiểm chứng** | issue xong có `outcome` với `verdict` khác `unverified` / issue xong | ≥ 80% |
| **Phút người trên mỗi việc xong** | tổng `attention_cost_minutes` đã tiêu / số issue xong | giảm ≥ 50% |
| **Tỷ lệ dự đoán đúng của Principal Profile** | dự đoán khớp quyết định thật / dự đoán có quyết định, theo từng loại | ≥ 85% ở ít nhất 3 loại |
| **Thời gian từ mô hình mới đến vào ghế** | từ lúc bật shadow run đến lúc đổi mô hình chính thức | ≤ 14 ngày, có bằng chứng |
| **Tỷ lệ hành động ra ngoài có nhãn hệ quả** | hành động qua nền tảng có `effect_class` / tổng hành động qua nền tảng | 100% (cưỡng chế bằng code) |
| **Sự cố do nới quyền** | outcome `verdict = failed` có `severity = high` sau một lần nâng mức tự chủ | 0 không được tự động hạ mức |

## 9. Rủi ro chiến lược

| Rủi ro | Hậu quả | Cách giảm |
| --- | --- | --- |
| Verifier sai hoặc dễ đánh lừa | Uy tín giả, nới quyền sai | Nhiều nguồn kiểm (F1 mục 2.3), verifier khác runtime với người làm (F7), lấy mẫu ngẫu nhiên cho người kiểm, và nguyên tắc "thu quyền nhanh hơn trao quyền" |
| Việc phi kỹ thuật khó kiểm bằng máy | F1 chỉ hữu ích với việc code | Bốn cấp kiểm chứng (máy, agent chéo, metric hạ nguồn, người theo mẫu) và cho phép `verdict = unverified` một cách trung thực |
| Principal Profile bị coi là giám sát người | Mất niềm tin, vấn đề pháp lý | Đồng ý rõ ràng, người được mô hình hoá xem và xoá được, không dùng cho đánh giá người, mặc định tắt |
| Chuẩn mở giúp đối thủ | Khách chuyển đi dễ hơn | Đúng là vậy, và đó là chủ đích. Con hào là vòng quay và lịch sử, không phải định dạng |
| Quá nhiều tính năng cùng lúc | Không cái nào xong | Năm giai đoạn, mỗi giai đoạn tự đứng được, sau cờ tính năng mặc định tắt |
| Xung đột với upstream khi sync | Tốn công merge | Nguyên tắc 8: mọi thứ ở file fork, hook một dòng, đăng ký ở `pnpm sync:contract` |
