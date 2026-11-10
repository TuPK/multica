# Kit Việt squads: runbook

Vận hành bộ Kit Việt (spec [kitviet-squads.md](kitviet-squads.md), kế hoạch [kitviet-squads-plan.md](kitviet-squads-plan.md)). Tài liệu này dành cho người nâng cấp upstream, người đăng sổ quy định và người chạy diễn tập. Nội dung của từng squad nằm ở các mục riêng sau mục 8.

## 1. Nguồn và dữ liệu sinh ra

| Tệp | Vai trò | Sửa tay? |
| --- | --- | --- |
| `packages/core/squads/kitviet-upstream/` | Bản vendor đã ghim commit, kèm `LICENSE`, `NOTICE.md`, `UPSTREAM.json` | Không. Chỉ thay bằng bản vendor mới |
| `packages/core/squads/kitviet-rewrites.json` | Bảng viết lại: khối Pro, mục nền tảng, tên tệp, `VERDICT:` | Có, rồi sinh lại |
| `packages/core/squads/kitviet-patches/*.json` | Bản vá khớp đúng một câu gốc (`find` → `replace`) | Có, rồi sinh lại |
| `packages/core/squads/kitviet-squads.json` | Mỗi squad: nghề, vai và loại vai, tầng lõi, skill lead, skill theo vai | Có, rồi sinh lại |
| `packages/core/squads/kitviet-skills/kv-*/` | Skill viết tay (`kv-delivery-core`, `kv-squad-lead`, `kv-company-profile`) | Có (đây là nguồn) |
| `packages/core/squads/kitviet-skills-data/*.ts` | Dữ liệu sinh ra, một tệp mỗi squad và `common.ts` | Không. Chạy `pnpm generate:kitviet` |
| `scripts/generate-kitviet-pack.mjs` | Generator | Có |

Lệnh:

- `pnpm generate:kitviet`: sinh lại dữ liệu. Lần chạy lặp cho cùng kết quả.
- `node scripts/generate-kitviet-pack.mjs --check`: fail khi tệp sinh ra cũ so với nguồn. Chạy trước khi commit.
- `node --test scripts/generate-kitviet-pack.test.mjs`: test của generator (bản vá khớp upstream, tách khối, phân giải tham chiếu).
- `cd packages/core && npx vitest run squads/kitviet-skills.test.ts`: test bộ dữ liệu (rò rỉ, tham chiếu theo squad, `VERDICT:`, bản vá, tên, kích thước, độ tươi).

Quy tắc phân giải tham chiếu (F17) áp cho từng squad: `kv:x` nằm trong tập phân giải của squad (tầng lõi cộng skill theo vai) thành tên `kv--x`; `kv:x` ngoài tập thành "nhờ lead tạo yêu cầu"; tên agent được thay bằng role key của squad, agent không có vai thì "nhờ lead".

## 2. Nâng commit upstream

Làm trên một nhánh riêng. Không sửa tay tệp trong `kitviet-upstream/`.

1. Đọc changelog của upstream giữa commit cũ và commit mới. Ghi lại các skill và agent được thêm, bỏ, đổi tên.
2. Cập nhật `UPSTREAM.json` (`ref`, `fetchedAt`). Chép lại đúng năm nghề và phần nền (`common/`) từ commit mới: chỉ `claude-code/kv/skills`, `claude-code/kv/agents` và bốn script xuất file. Không chép `codex/`, `claude-web/`, `brand/`, file nén.
3. Cập nhật `NOTICE.md` (commit, ngày lấy, danh sách thay đổi ở mức tệp).
4. Chạy `pnpm generate:kitviet`. Generator dừng với danh sách lỗi nếu:
   - một `find` của bản vá không còn khớp đúng một chỗ: đọc lại câu mới, sửa `find` trong `kitviet-patches/`;
   - một quy tắc viết lại không khớp nữa: hoặc upstream đã bỏ thứ đó (xóa quy tắc khỏi `kitviet-rewrites.json`), hoặc mẫu cần đổi;
   - một agent đổi loại (ví dụ thêm `Write`): sửa `kind` trong `kitviet-squads.json`;
   - một tham chiếu không biết (`unknown reference`): thêm skill vào squad hoặc viết lại.
5. Đọc diff của `kitviet-skills-data/*.ts` theo từng squad, không chỉ số dòng. Kiểm bằng mắt: khối mới có chứa quảng cáo, đường dẫn cũ, mốc pháp luật mới không? Mốc pháp luật phải đi qua Sổ quy định, không nhúng vào skill (F8, F23).
6. Chạy test: `node --test scripts/generate-kitviet-pack.test.mjs` và vitest của `squads`. Chạy `--check` hai lần.
7. Mỗi squad có mốc luật trong charter hoặc sổ quy định: đối chiếu với nguồn chính thức trước khi đăng lại sổ.

Không nâng commit khi đang có diễn tập chạy trên bản cũ. Ghi số commit trong PR.

## 3. Sổ quy định, hồ sơ đơn vị và cẩm nang

Tài liệu bền nằm trong KB (xem spec mục 5.8). Chu kỳ xem lại theo loại mặc định của KB:

| Tài liệu | Loại KB | Xem lại | Ai ghi | Ai đăng |
| --- | --- | --- | --- | --- |
| Hồ sơ đơn vị | `reference` | 365 ngày (đầu đề `Summary`) | lead qua `kv-company-profile` | chủ hoặc admin |
| Sổ quy định - <squad> | `reference` | 365 ngày | agent `legal_validity` và routine rà hiệu lực đề xuất bản draft mới | chủ hoặc admin |
| Cẩm nang giọng thương hiệu | `playbook` (180 ngày, `Purpose`, `Steps`) | 180 ngày | `mkt_lead` | chủ hoặc admin |

Quy tắc:

- Agent chỉ ghi bản **draft** (`multica knowledge add`). Không sửa bản đã đăng. Việc đăng (`multica knowledge publish`) là bước xác minh của người duyệt; không đăng thì không phải điều kiện bật squad (Q1).
- Muốn rút chu kỳ xem lại của `reference` xuống 30 ngày cho một tài liệu thì admin dùng `multica knowledge kind set`. Đây là việc của admin, không thuộc code.
- Mỗi dòng của sổ quy định có: văn bản, điều khoản dùng, hiệu lực, nguồn và ngày truy cập, người xác minh, ngày xác minh. Dòng không có nguồn chính thức ghi `[chưa tìm thấy trên nguồn chính thức]`.
- Sổ hợp đồng (Legal) và sổ công văn (Admin) là issue cố định, không phải KB: tạo ở lần chạy đầu, tìm lại bằng tiêu đề chuẩn hóa (`multica issue list`), không tạo bản trùng.
- Đầu đề bắt buộc (server từ chối khi thiếu): `reference` (Hồ sơ đơn vị, Sổ quy định) cần `## Summary`; `playbook` (Cẩm nang giọng thương hiệu) cần `## Purpose` và `## Steps`. Mẫu của từng loại nằm trong lead notes của squad và được test `kitviet-knowledge-format.test.ts` kiểm.
- Routine tạo issue ở trạng thái tạm dừng. Bật routine là việc của người đặt, sau khi đọc charter.

## 4. Phụ thuộc runtime

Một squad chạy được khi runtime của agent có các công cụ sau. Thiếu thì squad dừng ở điểm dừng loại 1 (không tự cài), không đoán.

| Phụ thuộc | Dùng cho | Nếu thiếu |
| --- | --- | --- |
| Bash | Chạy script tính toán và xuất file (`xuat-*`), đọc `.xlsx` | Mọi vai tính số bằng script phải dừng loại 1. Accounting không có kiểm tự động (spec 10.1, Q8); thiếu Bash thì vai tính số dừng loại 1 (F19) |
| Python 3 | Script `xuat-*` và script tính | Như trên; đính kèm Markdown hoặc CSV đã duyệt và ghi định dạng chưa xuất được (F9) |
| `python-docx`, `openpyxl`, `python-pptx`, `reportlab` | Xuất Word, Excel, PowerPoint, PDF | Thiếu thư viện nào: ghi tên thư viện thiếu, dừng loại 1, không tự cài |
| LibreOffice (tùy chọn) | Chuyển đổi một số định dạng | Bỏ qua, ghi rõ |
| Font tiếng Việt | PDF có dấu | Ghi rõ font thiếu trong biên bản |
| Công cụ web (`WebSearch`, `WebFetch`) | Vai tra cứu: `legal_sources`, `legal_validity`, `acct_tax_research`, `b2b_company_lookup`, `mkt_scout`, `admin_format_lookup` | Vai tra web dừng loại 1 và xin người dán nguyên văn nguồn. Không trả lời từ trí nhớ (F25) |
| Ghi comment và đính kèm (`multica issue comment add --attachment`) | Mọi khâu | Không chạy được squad |

Không có preflight mã: quickstart không phát hiện được công cụ của runtime (spec 10.1, Q8). Trước khi bật squad, hoặc khi đổi runtime, người bật kiểm tay trong môi trường của runtime bằng các lệnh sau; thiếu lệnh nào thì squad dừng loại 1 khi chạy, không tự cài:

- `which bash` (Accounting và mọi vai tính số)
- `python3 --version` (Python 3)
- `python3 -c "import docx, openpyxl, pptx, reportlab"` (thư viện xuất file; thiếu thì báo tên thư viện)
- `command -v soffice` (tùy chọn; thiếu thì ghi định dạng PDF chưa xuất)

## 5. Đọc `VERDICT:` thiếu hoặc sai

Mỗi gác cổng và bàn giao phải có đúng một dòng bắt đầu bằng `VERDICT:` ở dòng đầu của comment: `VERDICT: QUA` hoặc `VERDICT: CHẶN`. Quy tắc:

- Thiếu dòng `VERDICT:` (hoặc có hai dòng) được tính là **CHẶN** (quy tắc trong `kv_gate`, agent tự áp dụng).
- Dòng viết kiểu cũ (`Đạt`, `Chưa đạt`, `Gửi được`, `Chưa gửi được`) không phải kết luận hợp lệ. Generator đã thay mọi dòng mẫu của gác cổng bằng `VERDICT:`, nên dòng kiểu cũ chỉ xuất hiện khi agent tự viết sai; lead coi là thiếu và chạy lại.

Khi lead thấy `VERDICT:` thiếu:

1. Mở sub-issue gác cổng và đọc comment cuối. Nếu có bảng lỗi nhưng thiếu dòng kết luận, đó là lỗi của agent, không phải kết quả QUA.
2. Ghi "Round n/2" trên request và chạy lại gác cổng (chỉ gác cổng đó) với cùng link bản nháp.
3. Hết vòng 2 vẫn không có dòng hợp lệ: điểm dừng loại 3. Đặt `blocked` trên request gốc, comment kèm bản nháp và lỗi còn lại, mention người yêu cầu.

Không bao giờ đổi CHẶN hay thiếu dòng thành QUA bằng tay để đi tiếp.

## 6. Điểm dừng của người (spec 5.3)

- Loại 1 · thiếu căn cứ: `blocked` trên request gốc, comment liệt kê đúng thứ còn thiếu, mention người yêu cầu.
- Loại 2 · trước khi ra khỏi workspace: handover QUA thì `in_review` trên request gốc, comment "việc người dùng phải làm". `done` do người đặt. Squad không gửi, không nộp, không ký.
- Loại 3 · hết vòng sửa: như loại 1, kèm bản nháp và lỗi còn lại.

Ngoài ba loại này, agent lấy mặc định, ghi giả định và đi tiếp.

## 7. Diễn tập (spec 8.2)

Diễn tập chạy thật chỉ khi người dùng cho phép. Mỗi squad có ba lần chạy:

1. Đủ đầu vào: kỳ vọng ra sản phẩm và handover QUA.
2. Thiếu đầu vào: kỳ vọng `blocked` trên request gốc, không soạn.
3. Bản nháp cố tình sai: kỳ vọng gác cổng CHẶN và không ai đổi thành QUA.

Accounting thêm bài F19: hai bảng 200 dòng có một dòng lệch cố ý; agent phải tìm đúng dòng đó bằng script. Biên bản dán vào PR.

## 8. Mẫu cho từng squad

Mỗi squad thêm một mục theo khung này. Các mục chưa điền để trống, không xóa.

```
## <Tên squad đầy đủ>

- Template: <templateId>, floor: <floorName>, routine: <có/không, cron, startPaused>
- Vai (role key · loại · agent gốc):
  - <role> · <lead|collector|drafter|gate|handover> · <agent gốc>
- Điểm dừng loại 1: <việc>
- Điểm dừng loại 2: <việc>
- Điểm dừng loại 3: vòng sửa tối đa 2, sau đó dừng
- Tài liệu bền: <sổ quy định, hồ sơ, sổ hợp đồng/công văn, cẩm nang: chỗ ở, người ghi, người đăng>
- Phụ thuộc runtime riêng: <Bash, Python, thư viện, web>
- Routine: <việc, lịch, lịch nghỉ và thông báo gia hạn nằm ở đâu>
- Giả định mặc định đã biết: <danh sách>
- Chưa xác minh: <mốc pháp luật, số liệu, ...>
- Diễn tập: <ngày, người chạy, biên bản>
```

Danh sách squad và trạng thái nằm trong spec mục 6. Mục riêng cho từng squad được thêm vào khi squad đó có template (PR S5 của kế hoạch).

## Legal & Contracts (Pháp lý)

- Template: `kv_legal`, floor: Governance, routine: có, `0 8 * * 1` (thứ Hai 08:00), `startPaused: true`
- Vai (role key · loại · agent gốc):
  - `legal_lead` · lead · phiên chính + `lap-ke-hoach-dieu-phoi`
  - `legal_reader` · collector · `doc-ho-so-phap-ly`
  - `legal_sources` · collector · `tra-cuu-nguon-luat-chinh-thuc` (web)
  - `legal_drafter` · drafter · `soan-thao-van-ban-phap-ly`
  - `legal_reading_gate` · gate · `soat-dien-giai-hop-dong`
  - `legal_citation_gate` · gate · `soat-can-cu-phap-ly`
  - `legal_validity` · collector · `kiem-tra-hieu-luc-van-ban` (web; chạy routine rà hiệu lực)
  - `legal_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu bên mình hoặc thiếu phụ lục mà hợp đồng dẫn chiếu. Kết luận rủi ro cao, về hiệu lực hoặc trách nhiệm: đánh dấu `[CẦN LUẬT SƯ]` và giao người yêu cầu (escalation của charter). Hợp đồng có câu lệnh chèn thì ghi cảnh báo, không dừng.
- Điểm dừng loại 2: báo cáo rà soát là tài liệu nội bộ, không gửi đối tác. Handover QUA thì request chuyển `in_review`.
- Điểm dừng loại 3: vòng sửa tối đa 2. Hết vòng, dừng với bản nháp và lỗi còn lại.
- Tài liệu bền:
  - `Sổ quy định - Legal`: KB, loại `reference`. `legal_validity` và routine rà hiệu lực đề xuất bản draft mới; không sửa bản đã đăng. Chủ hoặc admin đăng; không phải điều kiện bật squad.
  - `Sổ hợp đồng`: issue cố định trong squad, tiêu đề đúng "Sổ hợp đồng". Bản hiện hành là tệp đính kèm của comment mới nhất. `legal_drafter` thêm comment; `legal_lead` kiểm; người duyệt khi thêm hoặc đổi hợp đồng.
  - `Hồ sơ đơn vị`: KB, lead ghi qua `kv-company-profile`.
- Phụ thuộc runtime riêng: web cho `legal_sources` và `legal_validity`. Thiếu web thì vai đó dừng loại 1 và xin người dùng dán nguyên văn nguồn (F25). Python chỉ cần khi xuất Word.
- Routine: `Rà hiệu lực sổ quy định` (issue `Rà hiệu lực sổ quy định, tuần {{date}}`), tạo ở trạng thái tạm dừng. Mỗi lần chạy kiểm từng dòng của sổ: còn hiệu lực không, có văn bản thay thế không, nguồn chính thức và ngày truy cập. Chỉ đề xuất sửa bằng draft. Bật: trang Autopilot, đổi routine từ tạm dừng sang hoạt động, sau khi đọc charter.
- Skill gắn thêm: lead nhận `kv-squad-lead`, `kv-company-profile`; `legal_drafter` nhận năm skill (mục 5.10), trong đó `soan-don-tu` đổi thành `soan-don-tu-phap-ly`.
- Giả định mặc định: không đưa ý kiến pháp lý thay luật sư; mọi trích dẫn phải có nguồn chính thức, không có thì ghi `[chưa tìm thấy trên nguồn chính thức]`.
- Chưa xác minh: mọi mốc pháp luật trong skill và sổ quy định (F8). Theo Q1 không có cổng xác minh pháp lý, nên không có bước nào xác nhận trước khi ra.
- Diễn tập: chưa chạy.

## Accounting & Tax (Kế toán, thuế)

- Template: `kv_accounting`, floor: Governance, routine: có, `0 8 * * 1` (thứ Hai 08:00), `startPaused: true`
- Vai (role key · loại · agent gốc):
  - `acct_lead` · lead · phiên chính + `lap-ke-hoach-dieu-phoi`
  - `acct_collector` · collector · `gom-chung-tu-ke-toan`
  - `acct_tax_research` · collector · `tra-cuu-van-ban-thue` (web)
  - `acct_reconciler` · drafter · `lap-so-lieu-doi-chieu`
  - `acct_tax_drafter` · drafter · `soan-ho-so-thue`
  - `acct_voucher_gate` · gate · `soat-chung-tu-hoa-don`
  - `acct_numbers_gate` · gate · `soat-so-lieu` (dùng chung)
  - `acct_tax_gate` · gate · `soat-tuan-thu-thue`
  - `acct_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu sổ chi tiết hoặc sao kê (không soạn trên dữ liệu thiếu). Chưa biết chế độ kế toán đang áp dụng: người dùng chọn một trong ba lựa chọn (doanh nghiệp chung theo Thông tư 99/2025; doanh nghiệp nhỏ và vừa theo Thông tư 133/2016; doanh nghiệp siêu nhỏ), câu hỏi nằm trong Hồ sơ đơn vị (F20). Thiếu CSV, hoặc không chạy được script tính, cũng dừng loại 1 (F19).
- Điểm dừng loại 2: không ghi sổ phần mềm, không nộp tờ khai, không gửi email. Kết quả là hồ sơ nháp chờ kế toán trưởng duyệt. Số liệu nhạy cảm (lương, tài khoản) được che trước khi đính kèm sang sub-issue khác (`an-thong-tin-nhay-cam`).
- Điểm dừng loại 3: vòng sửa tối đa 2. Hết vòng, bàn giao kèm các dòng còn mở.
- Tài liệu bền:
  - `Hồ sơ đơn vị`: KB, lead ghi qua `kv-company-profile`. Chứa câu trả lời về chế độ kế toán.
  - `Sổ quy định - Accounting`: KB, loại `reference`, đầu đề `## Summary`. Người ghi: `acct_tax_research`, vai tra cứu nguồn chính thức (quyết định của runbook này, chủ squad có thể đổi); routine chỉ đọc. Chủ hoặc admin đăng. Các dòng KT-01 đến KT-04 bên dưới là bản đầu tiên, đều chưa xác minh.
  - Lịch hạn: không lưu riêng; mỗi lần routine chạy là một comment trên issue của routine.
Bảng sổ quy định Accounting, dòng theo F20 (ngày kiểm 10/11/2026; mọi dòng chưa xác minh vì Q1 không có cổng xác minh; cột "Nguồn và ngày truy cập" là ngày kiểm). Cùng nội dung nằm trong lead notes của `kv_accounting`:

| id | Văn bản | Điều khoản dùng | Hiệu lực / valid_until | Nguồn và ngày truy cập | Người xác minh | Ngày xác minh |
| --- | --- | --- | --- | --- | --- | --- |
| KT-01 | Luật Quản lý thuế 108/2025/QH15; Nghị định 252/2026/NĐ-CP; Thông tư 89/2026/TT-BTC | Hạn nộp tờ khai và kỳ khai bổ sung | Từ 01/07/2026 (nguồn tin, chưa đối chiếu) | nguồn tin và tư vấn, truy cập 10/11/2026 | [chưa xác minh] | [chưa xác minh] |
| KT-02 | Thông tư 99/2025/TT-BTC (thay 200/2014 và 75/2015); Thông tư 58/2026/TT-BTC (thay 132/2018, doanh nghiệp siêu nhỏ) | Chế độ kế toán đang áp dụng | 99/2025 từ 01/01/2026; 58/2026 từ 01/07/2026 | nguồn tin và tư vấn, truy cập 10/11/2026 | [chưa xác minh] | [chưa xác minh] |
| KT-03 | Thời hạn khai bổ sung hồ sơ thuế, 10 năm đổi thành 5 năm (Luật 108/2025) | Khai bổ sung và lưu hồ sơ thuế | Theo Luật 108/2025 (chưa đối chiếu) | nguồn tin, truy cập 10/11/2026 | [chưa xác minh] | [chưa xác minh] |
| KT-04 | Nghị quyết 204/2025/QH15: giảm 8% thuế GTGT | Kê khai GTGT | Đến hết 31/12/2026 | nguồn tin, truy cập 10/11/2026 | [chưa xác minh] | [chưa xác minh] |

- Phụ thuộc runtime riêng: Bash và Python là bắt buộc để tính bằng script (F19). Thiếu một trong hai thì vai tính số dừng loại 1; không có preflight nên đây là chốt duy nhất (mục 4). Web cho `acct_tax_research`, thiếu thì dừng loại 1 (F25).
- Routine: `Lịch hạn nộp thuế` (issue `Lịch hạn nộp thuế, tuần {{date}}`), tạo ở trạng thái tạm dừng. Mỗi lần chạy liệt kê hạn trong 14 ngày tới theo đơn vị, đầu vào còn thiếu, và dòng sổ quy định có hiệu lực kết thúc trong 60 ngày. Thiếu lịch nghỉ thì ghi `[chưa có lịch nghỉ]`, không tự lùi hạn. Bật: trang Autopilot, đổi routine từ tạm dừng sang hoạt động, sau khi đọc charter.
- Skill gắn thêm: lead nhận `kv-squad-lead`, `kv-company-profile`; `acct_reconciler`, `acct_tax_drafter`, `acct_tax_research` nhận skill theo mục 5.10.
- Giả định mặc định: không tự chọn chế độ kế toán; không tính nhẩm số liệu; không chọn hạn nộp khi lịch nghỉ thiếu.
- Chưa xác minh: ba mốc F20 (chế độ kế toán, dòng GTGT 8%) chưa đối chiếu với nguồn chính thức; Q1 không có cổng xác minh. Ngưỡng dung lượng riêng 160 KiB (mục 9).
- Diễn tập: chưa chạy. Bắt buộc có bài F19 (mục 7): hai bảng 200 dòng, có một dòng lệch cố ý, agent phải tìm bằng script.

## B2B Proposal Desk (Chào giá B2B)

- Template: `kv_b2b`, floor: Launch, routine: không (`routine: null`)
- Vai (role key · loại · agent gốc):
  - `b2b_lead` · lead · phiên chính + `lap-ke-hoach-dieu-phoi`
  - `b2b_history` · collector · `doc-lich-su-giao-dich-khach`
  - `b2b_company_lookup` · collector · `tra-cuu-doanh-nghiep-cong-khai` (web)
  - `b2b_quote_drafter` · drafter · `soan-ho-so-chao-gia`
  - `b2b_commit_gate` · gate · `soat-cam-ket-thuong-mai`
  - `b2b_data_gate` · gate · `soat-quyen-dung-du-lieu-khach`
  - `b2b_tone_gate` · gate · `soat-giong-van-gui-khach`
  - `b2b_numbers_gate` · gate · `soat-so-lieu` (dùng chung)
  - `b2b_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu bảng giá hiện hành hoặc khung chiết khấu đã duyệt. Khách đòi vượt khung: không tự duyệt, chuyển `proposal` cho người có thẩm quyền.
- Điểm dừng loại 2: báo giá không rời workspace; không gửi email. Phần "NỘI BỘ - KHÔNG GỬI KHÁCH" (giá vốn, khung chiết khấu) tách khỏi bản gửi và bị `b2b_data_gate` soát. Handover QUA thì request chuyển `in_review`.
- Điểm dừng loại 3: vòng sửa tối đa 2. Hết vòng, bàn giao kèm bản nháp và lỗi còn lại.
- Tài liệu bền: không có sổ riêng (mục 5.8). Dùng `Hồ sơ đơn vị` chung. Bảng giá và khung chiết khấu do người dùng cung cấp làm đầu vào.
- Phụ thuộc runtime riêng: web cho `b2b_company_lookup`. Thiếu web thì dừng loại 1 (F25).
- Routine: không.
- Quan hệ: nhận deal đã qua sàng lọc từ Sales Loop; không tạo squad thứ hai soạn báo giá (mục 5.6).
- Giả định mặc định: không giảm giá hay kéo dài điều khoản ngoài khung; không đưa giá vốn hay biên lợi nhuận vào bản gửi khách.
- Chưa xác minh: thông tin doanh nghiệp tra cứu công khai chỉ được ghi kèm nguồn và ngày; không có mốc pháp luật riêng.
- Diễn tập: chưa chạy.

## Marketing Studio (Marketing)

- Template: `kv_marketing`, floor: Launch, routine: không (`routine: null`), không có chế độ `unattended` (Q3)
- Vai (role key · loại · agent gốc):
  - `mkt_lead` · lead · phiên chính + `lap-ke-hoach-dieu-phoi`
  - `mkt_scout` · collector · `do-tham-doi-thu-web` + `nghien-cuu-thi-truong-web` (web, gộp một vai)
  - `mkt_insight` · collector · `gom-insight-khach-hang`
  - `mkt_content` · drafter · `soan-noi-dung-da-kenh`
  - `mkt_ads` · drafter · `soan-ad-copy-landing`
  - `mkt_claim_gate` · gate · `soat-claim-quang-cao`
  - `mkt_voice_gate` · gate · `soat-giong-thuong-hieu`
  - `mkt_numbers_gate` · gate · `soat-so-lieu-thi-truong`
  - `mkt_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: "chốt thông điệp" (một thông điệp chính và ba ý hỗ trợ) là điểm dừng trước khi soạn. Lead trình thông điệp bằng `proposal` hoặc comment mention người yêu cầu, và chờ duyệt. Nếu request đã nêu thông điệp đã duyệt thì lead ghi "duyệt trong request" và bỏ qua điểm này. Ngành nhạy cảm (thực phẩm, mỹ phẩm, y tế, giáo dục) luôn đi qua `mkt_claim_gate`.
- Điểm dừng loại 2: không đăng, không chạy quảng cáo, không gửi brief cho KOL/KOC. Handover QUA thì request chuyển `in_review`.
- Điểm dừng loại 3: vòng sửa tối đa 2. Claim vẫn bị chặn sau vòng 2 thì bàn giao cho người yêu cầu kèm các dòng bị chặn.
- Tài liệu bền:
  - `Sổ quy định - Marketing`: KB, loại `reference`, gồm luật quảng cáo, khuyến mại, thuế trúng thưởng. Agent ghi bản draft; chủ hoặc admin đăng.
  - `Cẩm nang giọng thương hiệu`: KB, loại `playbook` (180 ngày), đầu đề bắt buộc `## Purpose` và `## Steps`. `mkt_lead` ghi qua `cam-nang-giong-thuong-hieu`; chủ hoặc admin đăng.
- Phụ thuộc runtime riêng: web cho `mkt_scout`. Thiếu web thì dừng loại 1 (F25).
- Routine: không.
- Quan hệ: Growth & GTM lập chiến lược và giao brief; Studio sản xuất và soát nội dung (mục 5.6).
- Giả định mặc định: mọi nội dung là nháp; không dùng số liệu thị trường không có nguồn và ngày.
- Chưa xác minh: luật quảng cáo, khuyến mại, thuế trúng thưởng và các mốc gắn ngày (F8) chưa đối chiếu với nguồn chính thức; mốc thời hạn trong `soat-luat-quang-cao` và `the-le-minigame-khuyen-mai` phải đi qua Sổ quy định.
- Diễn tập: chưa chạy.

## Office Admin (Hành chính)

- Template: `kv_admin`, floor: Executive, routine: không (`routine: null`)
- Vai (role key · loại · agent gốc):
  - `admin_lead` · lead · phiên chính + `lap-ke-hoach-dieu-phoi`
  - `admin_files` · collector · `gom-ho-so-van-thu`
  - `admin_format_lookup` · collector · `tra-cuu-the-thuc-van-ban` (web)
  - `admin_drafter` · drafter · `soan-van-ban-hanh-chinh`
  - `admin_format_gate` · gate · `soat-the-thuc-van-ban`
  - `admin_numbers_gate` · gate · `soat-so-lieu` (dùng chung)
  - `admin_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu người ký hoặc mục đích của văn bản. Hồ sơ đơn vị thiếu tên, ký hiệu, địa danh, quy tắc đánh số, danh sách người ký hoặc quyền hạn cũng dừng loại 1.
- Điểm dừng loại 2: không cấp số, không ký, không phát hành văn bản. Số văn bản để `[cần bổ sung]`. Handover QUA thì request chuyển `in_review`. Trước khi cấp số đi, người xác nhận văn bản đã ký.
- Điểm dừng loại 3: vòng sửa tối đa 2. Hết vòng, bàn giao kèm bản nháp và lỗi còn lại.
- Tài liệu bền:
  - `Hồ sơ đơn vị`: KB, lead ghi qua `kv-company-profile`. Phải có các mục ở điểm dừng loại 1.
  - `Sổ quy định - Admin`: KB, loại `reference` (mục 5.8). Agent ghi bản draft; chủ hoặc admin đăng.
  - `Sổ công văn <năm>`: issue cố định trong squad, một issue mỗi năm (spec 5.8), tạo ở lần chạy đầu của năm; tìm lại bằng tiêu đề chuẩn hóa. `admin_drafter` ghi một comment mỗi văn bản; số văn bản chỉ ghi khi người xác nhận đã ký.
- Phụ thuộc runtime riêng: web cho `admin_format_lookup`. Thiếu web thì dừng loại 1 (F25).
- Routine: không.
- Skill gắn thêm: lead nhận `quan-ly-lich-giam-doc`; `admin_drafter` nhận `soan-don-tu`, `so-sanh-bao-gia-mua-sam`, `chuan-bi-hop-hdqt` (mục 5.10).
- Quan hệ: Secretary ở Executive Office giữ việc định tuyến; Office Admin không nhận việc của Secretary và báo cáo cho boss.
- Giả định mặc định: thể thức văn bản theo tra cứu của `admin_format_lookup` tại thời điểm chạy; có ghi ngày truy cập.
- Chưa xác minh: thể thức, quy tắc đánh số và các mốc pháp luật chưa đối chiếu với nguồn chính thức; không có cổng xác minh (Q1).
- Diễn tập: chưa chạy.

## 9. Việc còn mở

- Ngưỡng dung lượng của Accounting: tầng lõi đo được 146,9 KiB (ngày 2026-11-11); ngưỡng riêng của squad này là 160 KiB (test K8) vì ba skill được nâng lên là chính dây chuyền và chỉ được đọc khi cần. Các squad khác đo được 107,8–127,6 KiB, dưới ngưỡng 140 KiB.
- `soan-don-tu` ở Hành chính và Pháp lý là hai bản byte giống nhau (F11). Bản Pháp lý đổi tên thành `soan-don-tu-phap-ly` theo kế hoạch; nội dung không khác nên tên chỉ là nhãn.
- Tham chiếu `chuan-bi-dam-phan-hoa-giai` trong `soan-bien-ban-thanh-ly-hop-dong` (Pháp lý) không nằm trong tầng lõi hay vai; ở P0 nó thành "nhờ lead tạo yêu cầu" (spec mục 6 để ngoài P0).
