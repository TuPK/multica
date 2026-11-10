# Kit Việt squads: runbook

Vận hành bộ Kit Việt (spec [kitviet-squads.md](kitviet-squads.md), kế hoạch [kitviet-squads-plan.md](kitviet-squads-plan.md)). Tài liệu này dành cho người nâng cấp upstream, người đăng sổ quy định và người chạy diễn tập. Nội dung của từng squad nằm ở mục riêng cuối tài liệu.

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
- Routine tạo issue ở trạng thái tạm dừng. Bật routine là việc của người đặt, sau khi đọc charter.

## 4. Phụ thuộc runtime

Một squad chạy được khi runtime của agent có các công cụ sau. Thiếu thì squad dừng ở điểm dừng loại 1 (không tự cài), không đoán.

| Phụ thuộc | Dùng cho | Nếu thiếu |
| --- | --- | --- |
| Bash | Chạy script tính toán và xuất file (`xuat-*`), đọc `.xlsx` | Mọi vai tính số bằng script phải dừng loại 1. **Accounting không bật** nếu thiếu Bash (F19) |
| Python 3 | Script `xuat-*` và script tính | Như trên; đính kèm Markdown hoặc CSV đã duyệt và ghi định dạng chưa xuất được (F9) |
| `python-docx`, `openpyxl`, `python-pptx`, `reportlab` | Xuất Word, Excel, PowerPoint, PDF | Thiếu thư viện nào: ghi tên thư viện thiếu, dừng loại 1, không tự cài |
| LibreOffice (tùy chọn) | Chuyển đổi một số định dạng | Bỏ qua, ghi rõ |
| Font tiếng Việt | PDF có dấu | Ghi rõ font thiếu trong biên bản |
| Công cụ web (`WebSearch`, `WebFetch`) | Vai tra cứu: `legal_sources`, `legal_validity`, `acct_tax_research`, `b2b_company_lookup`, `mkt_scout`, `admin_format_lookup` | Vai tra web dừng loại 1 và xin người dán nguyên văn nguồn. Không trả lời từ trí nhớ (F25) |
| Ghi comment và đính kèm (`multica issue comment add --attachment`) | Mọi khâu | Không chạy được squad |

Preflight tự động không có (F9). Người chạy diễn tập kiểm tay bằng lệnh `python3 -c "import docx, openpyxl, pptx, reportlab"` và `which bash` trong môi trường của runtime.

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

## 9. Việc còn mở

- Ngưỡng dung lượng của Accounting: tầng lõi đo được khoảng 146,6 KiB chữ; ngưỡng riêng của squad này là 160 KiB (test K8) vì ba skill được nâng lên là chính dây chuyền và chỉ được đọc khi cần. Các squad khác giữ 140 KiB.
- `soan-don-tu` ở Hành chính và Pháp lý là hai bản byte giống nhau (F11). Bản Pháp lý đổi tên thành `soan-don-tu-phap-ly` theo kế hoạch; nội dung không khác nên tên chỉ là nhãn.
- Tham chiếu `chuan-bi-dam-phan-hoa-giai` trong `soan-bien-ban-thanh-ly-hop-dong` (Pháp lý) không nằm trong tầng lõi hay vai; ở P0 nó thành "nhờ lead tạo yêu cầu" (spec mục 6 để ngoài P0).
