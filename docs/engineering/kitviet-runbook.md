# Kit Việt squads: runbook

Vận hành bộ Kit Việt (spec [kitviet-squads.md](kitviet-squads.md), kế hoạch [kitviet-squads-plan.md](kitviet-squads-plan.md)). Tài liệu này dành cho người nâng cấp upstream, người đăng sổ quy định và người chạy diễn tập. Nội dung của từng squad nằm ở các mục riêng sau mục 8.

## 1. Nguồn và dữ liệu sinh ra

| Tệp | Vai trò | Sửa tay? |
| --- | --- | --- |
| `catalog/_sources/kitviet/upstream/` | Bản vendor đã ghim commit, kèm `LICENSE`, `NOTICE.md`, `UPSTREAM.json` | Không. Thay bằng `pnpm kitviet:update` (mục 2) |
| `catalog/_sources/kitviet/rewrites.json` | Bảng viết lại: khối Pro, mục nền tảng, tên tệp, `VERDICT:` | Có, rồi sinh lại |
| `catalog/_sources/kitviet/patches/*.json` | Bản vá khớp đúng một câu gốc (`find` → `replace`) | Có, rồi sinh lại |
| `catalog/_sources/kitviet/squads/` | Cấu hình squad: mỗi nghề một JSON (`<nghề>.json`) gồm vai và loại vai, tầng lõi, skill lead, skill theo vai; `base.json` chung | Có, rồi sinh lại |
| `catalog/squads/kv_<nghề>/` | Mỗi squad một thư mục: `template.json` (charter, routine, floor, mô tả vai, dòng roster, khối `adapter: kitviet`) và `i18n/` (đủ 7 ngôn ngữ). Tên vai, loại vai, skill và hướng dẫn của lead lấy từ pack | Có |
| `catalog/_sources/kitviet/skills/kv-*/` | Skill viết tay (`kv-delivery-core`, `kv-squad-lead`, `kv-company-profile`) | Có (đây là nguồn) |
| `packages/core/catalog/generated/kitviet/*.ts` | Dữ liệu sinh ra, không commit: một tệp mỗi pack (`common.ts` và từng nghề) và tệp dùng chung | Không. `pnpm catalog:build` sinh (turbo, `make`, Docker tự gọi) |
| `scripts/generate-kitviet-pack.mjs` | Generator | Có |

Lệnh:

- `pnpm catalog:build` (hoặc `pnpm generate:kitviet` chỉ cho gói Kit Việt): sinh lại dữ liệu. Lần chạy lặp cho cùng kết quả. Dữ liệu sinh ra không nằm trong git.
- `node scripts/generate-kitviet-pack.mjs --check`: fail khi bản đã sinh trên máy cũ so với nguồn (quên chạy lại `pnpm catalog:build`).
- `node --test scripts/generate-kitviet-pack.test.mjs`: test của generator (bản vá khớp upstream, tách khối, phân giải tham chiếu).
- `cd packages/core && npx vitest run squads/kitviet-skills.test.ts`: test bộ dữ liệu (rò rỉ, tham chiếu theo squad, `VERDICT:`, bản vá, tên, kích thước, độ tươi).

Quy tắc phân giải tham chiếu (F17) áp cho từng squad: `kv:x` nằm trong tập phân giải của squad (tầng lõi cộng skill theo vai) thành tên `kv--x`; `kv:x` ngoài tập thành "nhờ lead tạo yêu cầu"; tên agent được thay bằng role key của squad, agent không có vai thì "nhờ lead".

## 2. Nâng commit upstream

Một lệnh làm các bước máy làm được. Chạy từ gốc repo:

```
pnpm kitviet:update                  # bản phát hành mới nhất (tag vX.Y.Z), hoặc HEAD của main nếu chưa có tag
pnpm kitviet:update -- v1.5.1        # một tag, nhánh hoặc commit cụ thể
pnpm kitviet:update -- --dry-run     # chạy toàn bộ trong bản sao tạm, không ghi gì vào repo
```

Để chạy với một bản clone cục bộ thay vì GitHub: `KITVIET_REMOTE=/đường/dẫn/tới/clone pnpm kitviet:update`.

Lệnh làm theo thứ tự (`scripts/kitviet-update.mjs`):

1. Hỏi remote bằng `git ls-remote` (chỉ đọc). Nếu commit đích trùng `UPSTREAM.json`, lệnh in "nothing to update" và thoát 0.
2. Dừng nếu các đường dẫn sẽ bị thay (`kitviet-upstream/`, `kitviet-skills-data/`, `kitviet-previous-texts.ts`) hoặc đầu vào của text Kit Việt (cấu hình squad, bản vá, skill viết tay, template, `kitviet-agent-instructions.ts`, `instruction-blocks.ts`, generator) có thay đổi chưa commit. Lý do: text "cũ" được dựng từ working tree, nên một thay đổi chưa commit sẽ bị ghi vào registry như thể đã phát hành. Commit hoặc cất chúng trước; danh sách đầy đủ là `GUARDED_PATHS` trong `scripts/kitviet-update.mjs`.
3. Tải đúng commit đó vào thư mục tạm bằng `git fetch --depth 1`. Không chạy script của upstream. Chỉ chấp nhận giấy phép MIT.
4. Dựng bản sao tạm của `scripts/` và `packages/core/` (các `node_modules` được nối vào, không cài lại). Trong bản sao: vendor các nghề theo `UPSTREAM.json`; ghi text cũ vào registry nếu bản mới làm nó đổi; sinh lại dữ liệu; chạy `--check`, test node của generator và của script, `vitest run packages/core/squads packages/core/agents`, và `tsc`.
5. Chỉ khi mọi bước đạt mới thay ba đường dẫn trong repo. Bản cũ được sao lưu trước; nếu một lần thay lỗi, các đường dẫn đã thay được khôi phục.
6. In tóm tắt: ref cũ và mới, skill và agent thêm, đổi, bỏ; số bản vá đổi; số text đưa vào registry.

Lệnh không commit, không push, không ghi lên mạng. Sau khi chạy:

- Đọc `git status` và `git diff --stat`. Với bản đã kiểm (v1.5.0), diff của `kitviet-skills-data/*.ts` chỉ đổi trường `ref`. Nếu diff có nội dung khác, đọc theo từng nghề.
- Kiểm bằng mắt: khối mới có quảng cáo, đường dẫn cũ, mốc pháp luật mới không? Mốc pháp luật đi qua Sổ quy định, không nhúng vào skill (F8, F23).
- Chạy lại `node scripts/generate-kitviet-pack.mjs --check`, rồi commit bằng tay.

Khi một bước trong stage lỗi, lệnh dừng, repo không đổi và thư mục stage được giữ lại (đường dẫn in ra). Các lỗi gặp thường gặp:

- Bản vá không khớp (`patch matched nothing`): đọc câu mới, sửa `find` trong `kitviet-patches/<tên>.json`. Nếu upstream đã làm xong việc đó, xóa bản vá và ghi lý do vào mục 9.
- Quy tắc viết lại không khớp (`stale rule`): upstream đã bỏ thứ đó. Xóa quy tắc khỏi `kitviet-rewrites.json`.
- Skill mới không thuộc danh sách nào (`attached to nothing and not deferred`): thêm vào `deferredSkills` của squad (đóng gói, không gắn), hoặc vào `builtinSkills`/`roleSkills` nếu cần gắn, hoặc vào `droppedSkills` nếu giữ ngoài bản này. Ghi lý do ở mục 9.
- Agent đổi loại (`is a drafter by its tools`): sửa `kind` trong `kitviet-squads/<nghề>.json`.
- Tham chiếu lạ (`unknown reference`), hoặc tham chiếu tới skill đã bỏ: thêm skill vào squad, hoặc bỏ luôn skill phụ thuộc và ghi lý do.
- Trích dẫn luật có số hiệu hoặc ngày (test "legal citations"): không thêm vào danh sách ổn định, vì danh sách đó cố ý rỗng. Patch câu đó để trỏ tới dòng của Sổ quy định, hoặc giữ skill ngoài pack đến khi có sổ.
- Nghề mới của upstream: lệnh chỉ in tên nghề và bỏ qua. Để vendor, thêm nghề vào `UPSTREAM.json.trades` và tạo config squad.

Registry: khi một text agent hoặc squad bị thay, lệnh ghi text cũ vào `kitviet-previous-texts.ts` (theo role key, hoặc vào danh sách squad). Entry cũ không bị sửa. Text không đổi thì không ghi, vì test registry cấm một text cũ trùng text hiện hành. Nhờ vậy workspace tạo từ bản cũ vẫn nâng được.

Kiểm sau khi áp (chạy trong repo):

- `node scripts/generate-kitviet-pack.mjs --check`
- `node --test scripts/generate-kitviet-pack.test.mjs scripts/kitviet-update.test.mjs`
- `pnpm exec vitest run packages/core/squads packages/core/agents`
- `pnpm -C packages/core exec tsc --noEmit`

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
| Công cụ web (`WebSearch`, `WebFetch`) | Vai tra cứu: `legal_sources`, `legal_validity`, `acct_tax_research`, `b2b_company_lookup`, `mkt_scout`, `admin_format_lookup`, `talent_market`, `pr_source`, `ecom_rival_scout`, `acad_finder` | Vai tra web dừng loại 1 và xin người dán nguyên văn nguồn. Không trả lời từ trí nhớ (F25) |
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
- Gác cổng thiếu bản nháp đính kèm (chỉ có nội dung dán trong comment, hoặc không có tệp) hoặc thiếu nguồn không là điểm dừng loại 1. Gác cổng ghi `VERDICT: CHẶN - không có bản nháp đính kèm` hoặc `VERDICT: CHẶN - thiếu <nguồn>`, liệt kê mục không soát được, và đặt sub-issue `done`. Gác cổng thiếu nguồn vẫn soát các mục không cần nguồn (cam kết, lời hứa, thông tin cá nhân). Sub-issue gác cổng chỉ `blocked` khi gác cổng không chạy được (ví dụ runtime không đọc được tệp).
- Lệnh `multica issue status <sub-issue> done` là lệnh cuối cùng của lần chạy gác cổng. Server không đặt trạng thái sub-issue khi một lần chạy hoàn thành; đường xử lý lần chạy thất bại là nơi duy nhất server ghi trạng thái issue khi lần chạy kết thúc. Gác cổng nào kết thúc mà chưa đặt `done` sẽ giữ chặng ở `in_progress`, và lead phải kiểm tay.

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

## 6a. Routine tạm dừng khi chạy lại (spec 5.7, Q9)

- Routine có `startPaused` (Legal, Accounting) được tạo ở trạng thái tạm dừng, trước khi thêm lịch.
- Chạy lại quickstart chỉ tạm dừng lại autopilot mà chính quickstart tạo và lần tạm dừng trước thất bại: đang hoạt động, chưa có trigger, mô tả và tiêu đề issue còn đúng từng chữ của template. Tạm dừng xong mới thêm lịch; nếu tạm dừng thất bại thì không thêm lịch, và báo lỗi `pause:`.
- Autopilot đã có lịch, hoặc do người dùng sửa mô tả, giữ nguyên trạng thái. Không bao giờ bật lại.
- Kết quả `partial` trên quickstart: routine đã nối và đã có lịch, nhưng đang chạy trong khi template muốn tạm dừng (thường là autopilot của người dùng cùng tên). Mở autopilot và tạm dừng nếu muốn chờ. Lần chạy sau báo `existing` và không đổi gì.

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
- Chú ý: khi công tắc "Operating review" tắt lúc lưu charter lần đầu, server không tạo đối tượng "Operating review" nào (không autopilot, không routine); quickstart gửi `operating_review_enabled: false` nên squad này không có đối tượng đó. Bật công tắc về sau sẽ tạo đối tượng "Operating review" ở trạng thái đang chạy; tắt lại sẽ tạm dừng nó.
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

## Talent Acquisition (Tuyển dụng)

- Template: `kv_talent`, floor: Governance, routine: không
- Vai (role key · loại · agent gốc):
  - `talent_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `talent_reader` · collector · `doc-ho-so-ung-vien`
  - `talent_market` · collector · `tra-cuu-thi-truong-tuyen-dung` (web; chỉ nhận chức danh, không nhận dữ liệu ứng viên)
  - `talent_screener` · drafter · `lap-bang-sang-loc-ung-vien`
  - `talent_drafter` · drafter · `soan-tai-lieu-tuyen-dung`
  - `talent_bias_gate` · gate · `soat-thien-kien-tuyen-dung`
  - `talent_data_gate` · gate · `soat-du-lieu-nhan-su`
  - `talent_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu JD đã duyệt, tiêu chí đã duyệt có trọng số, hoặc thư mục CV. Nếu chỉ thiếu tiêu chí, vai đọc CV vẫn chạy; vai sàng lọc ghi tiêu chí đề xuất kèm `CHỜ NGƯỜI DÙNG DUYỆT` và không chấm điểm.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment việc người dùng phải làm: chốt danh sách vào vòng, gửi thư, xếp lịch phỏng vấn. Squad không liên hệ, không hẹn, không quyết định tuyển ai.
- Điểm dừng loại 3: vòng sửa tối đa 2. Hết vòng mà cổng còn CHẶN thì `blocked`, comment bản nháp và các lỗi còn mở, mention người yêu cầu.
- Tài liệu bền: không có sổ quy định hay KB riêng. Hồ sơ đơn vị: lead qua `kv-company-profile` (mục 5.8).
- Phụ thuộc runtime riêng: web cho `talent_market`; Bash và Python để tính điểm sàng lọc bằng script (mục 4).
- Routine: không.
- Giả định mặc định: điểm theo trọng số đã duyệt, mỗi điểm khác 0 có một câu trích từ CV.
- Chưa xác minh: mức lương và tin tuyển dụng công khai (`talent_market`) ghi kèm ngày truy cập. Không có cổng xác minh pháp lý (Q1); cổng thiên kiến soát văn bản, không thay cho đánh giá pháp lý về phân biệt đối xử.
- Diễn tập: chưa chạy.

## PR & Comms (Truyền thông báo chí)

- Template: `kv_pr`, floor: Launch, routine: không
- Vai (role key · loại · agent gốc):
  - `pr_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `pr_source` · collector · `tra-cuu-nguon-bao-chi` (web)
  - `pr_transcript` · collector · `trich-xuat-bang-ghi-phong-van`
  - `pr_writer` · drafter · `soan-bai-bao-chi`
  - `pr_release` · drafter · `soan-tai-lieu-pr`
  - `pr_fact_gate` · gate · `soat-su-that-bai-bao`
  - `pr_ethics_gate` · gate · `soat-phap-ly-dao-duc-bao-chi`
  - `pr_numbers_gate` · gate · `soat-so-lieu` (dùng chung)
  - `pr_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thông điệp chính hoặc người phát ngôn chưa duyệt (trừ khi request đã ghi là đã duyệt); khủng hoảng mà sự việc chưa xác minh hoặc thiếu lập trường của công ty; một nhận định không có nguồn.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment việc người dùng phải làm. Squad không gửi cho phóng viên, không đăng và không xuất bản.
- Điểm dừng loại 3: vòng sửa tối đa 2. Hết vòng thì mention người yêu cầu kèm các dòng bị chặn.
- Tài liệu bền: Hồ sơ đơn vị (lead qua `kv-company-profile`). Không có sổ quy định.
- Phụ thuộc runtime riêng: web cho `pr_source`; Bash và Python cho số liệu (mục 4).
- Routine: không.
- Chưa xác minh: luật báo chí và quy định đạo đức nghề báo hiện hành. Bản soát của `pr_ethics_gate` ghi rõ không thay ý kiến pháp chế, luật sư hay quyết định của tổng biên tập. Không có cổng xác minh pháp lý (Q1).
- Diễn tập: chưa chạy.

## E-commerce Shop Ops (Bán hàng online)

- Template: `kv_ecom`, floor: Revenue (floor mới của P1; squad này là nơi đầu tiên đặt ô trên floor này, Q4), routine: không
- Vai (role key · loại · agent gốc):
  - `ecom_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `ecom_intake` · collector · `doc-file-xuat-don-san` và `gom-du-lieu-shop` (hai agent chung một vai)
  - `ecom_rival_scout` · collector · `tra-cuu-doi-thu-san` (web)
  - `ecom_listing_drafter` · drafter · `soan-listing-san-pham`
  - `ecom_cskh_drafter` · drafter · `soan-tin-cskh-shop`
  - `ecom_policy_gate` · gate · `soat-tu-cam-quang-cao-san`
  - `ecom_margin_gate` · gate · `soat-bien-loi-nhuan-shop`
  - `ecom_numbers_gate` · gate · `soat-so-lieu` (dùng chung)
  - `ecom_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu thông số nhà cung cấp, giá vốn, sàn mục tiêu hoặc biểu phí đã xác nhận; hoặc các nguồn của chủ shop mâu thuẫn nhau.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment danh sách quyết định chủ shop phải làm trước khi đăng. Chủ shop đăng sản phẩm và trả lời khách.
- Điểm dừng loại 3: vòng sửa tối đa 2. Hết vòng thì mention người yêu cầu kèm bản nháp và lỗi còn lại.
- Tài liệu bền: Hồ sơ đơn vị (lead). Biểu phí sàn, phí thanh toán, trợ giá vận chuyển và thuế do chủ shop cung cấp kèm ngày xác nhận; thiếu thì `[cần xác nhận]`, và cổng biên lợi nhuận chặn giá phụ thuộc vào khoản đó.
- Phụ thuộc runtime riêng: web cho `ecom_rival_scout`; Bash và Python cho biên lợi nhuận và số liệu (mục 4).
- Routine: không.
- Chưa xác minh: chính sách sàn hiện hành và danh sách từ bị cấm. Cổng không chứng nhận tin đăng đúng chính sách của sàn; chủ shop kiểm chính sách trước khi đăng.
- Diễn tập: chưa chạy.

## Office Ops (Vận hành văn phòng)

- Template: `kv_officeops`, floor: Executive, routine: không. Luồng lead viết tay (không có dây chuyền upstream): `KV_LEAD_NOTES.kv_officeops`.
- Vai (role key · loại · agent gốc):
  - `officeops_lead` · lead · `lap-ke-hoach-dieu-phoi` (luồng viết tay)
  - `officeops_files` · collector · `gom-ho-so-van-thu`
  - `officeops_drafter` · drafter · `lap-bang-hanh-chinh` (chạy skill của việc được chọn)
  - `officeops_numbers_gate` · gate · `soat-so-lieu` (dùng chung)
  - `officeops_handover` · handover · `kiem-dinh-ban-giao`
- Việc vận hành (chín skill, mục 5.10): ba skill ở tầng lõi cho mọi vai (`to-chuc-su-kien-hoi-nghi`, `theo-doi-hop-dong-dich-vu-vp`, `tiep-khach-thu-cam-on`); `officeops_drafter` nhận thêm sáu skill theo vai (đặt phòng, đặt vé và dự toán công tác, tài sản, văn phòng phẩm, lưu trữ, báo cáo tháng).
- Không có cổng soát dự toán mua sắm. Việc chi tiêu chỉ dừng loại 1 khi thiếu trần ngân sách; soát số liệu do `officeops_numbers_gate` làm.
- Điểm dừng loại 1: việc chi tiêu thiếu trần ngân sách; đặt phòng hoặc sự kiện thiếu ngày, số người hoặc địa điểm; báo cáo tài sản thiếu danh sách hoặc file sổ; báo cáo tháng thiếu file nguồn. Yêu cầu không thuộc chín việc trên thì `blocked`, comment danh sách việc.
- Điểm dừng loại 2: không đặt, không cọc, không đặt hàng, không thanh toán, không ký, không gửi, không mời. Handover QUA thì `in_review`, comment việc người yêu cầu phải đặt, trả tiền, gửi hoặc ký.
- Điểm dừng loại 3: vòng sửa tối đa 2. Hết vòng thì mention người yêu cầu kèm bản nháp và lỗi còn lại.
- Tài liệu bền: Hồ sơ đơn vị (lead). Sổ tài sản, văn phòng phẩm và lưu trữ là bản nội bộ theo tháng; số lượng không có trong file ghi `[cần bổ sung: số lượng]`, không ước lượng.
- Phụ thuộc runtime riêng: Bash và Python cho số liệu (mục 4). Không có vai web.
- Routine: không.
- Chưa xác minh: điều khoản hợp đồng dịch vụ và thời hạn lưu trữ chưa đối chiếu với nguồn chính thức; thiếu điều khoản thì ghi `[cần bổ sung: điều khoản]`. Không có cổng xác minh (Q1).
- Diễn tập: chưa chạy.

## Academic Evidence Desk (Học thuật, nghiên cứu)

- Template: `kv_academic`, floor: Research, routine: không
- Vai (role key · loại · agent gốc):
  - `acad_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `acad_reader` · collector · `doc-pdf-bai-bao`
  - `acad_finder` · collector · `tim-tai-lieu-hoc-thuat` (web; mở DOI hoặc trang tạp chí trước khi báo cáo)
  - `acad_matrix` · drafter · `lap-ma-tran-tai-lieu` (ghi vào Sổ tài liệu)
  - `acad_drafter` · drafter · `soan-ban-thao-hoc-thuat`
  - `acad_citation_gate` · gate · `soat-trich-dan-hoc-thuat` (cổng chống trích dẫn bịa)
  - `acad_integrity_gate` · gate · `soat-liem-chinh-hoc-thuat`
  - `acad_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu thư mục tài liệu, dàn ý, hoặc quy định của trường về dùng AI. Vai tìm nguồn không có web thì dừng và xin người dùng dán nguyên văn từng nguồn.
- Điểm dừng loại 2: squad không nộp bài và không biến bản nháp thành bài làm; tác giả tự viết lại. Yêu cầu nộp bản cuối là việc của tác giả. Handover QUA thì `in_review`.
- Điểm dừng loại 3: hai vòng CHẶN ở cổng trích dẫn hoặc cổng liêm chính, hoặc bất kỳ nguồn bịa nào. Đánh dấu `[CẦN NGƯỜI HƯỚNG DẪN]`, giao lại người yêu cầu với các dòng còn mở, không đổi nội dung.
- Tài liệu bền: Sổ tài liệu là issue cố định của squad; `acad_matrix` thêm mỗi bản ma trận thành một comment kèm CSV. Hồ sơ đơn vị (lead) nếu là đơn vị.
- Phụ thuộc runtime riêng: web cho `acad_finder`; công cụ đọc PDF trên runtime cho `acad_reader`; Bash và Python cho số liệu (mục 4).
- Routine: không.
- Chưa xác minh: một tài liệu chưa được mở thì ghi `[chưa xác minh]`, không báo là thật. Cổng liêm chính không thay cho kiểm tra đạo văn của trường.
- Diễn tập: chưa chạy.

## Healthcare (Y tế)

- Template: `kv_health`, floor: Healthcare (tạo khi quickstart chạy lần đầu, mục 8c), routine: không. Pipeline `day-chuyen-tai-lieu-y-te`.
- Vai (role key · loại · agent gốc):
  - `health_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `health_reader` · collector · `doc-tai-lieu-chuyen-mon-y-te`
  - `health_research` · collector · `tra-cuu-y-van` (web)
  - `health_regulation` · collector · `tra-cuu-quy-dinh-y-te` (web)
  - `health_drafter` · drafter · `soan-tai-lieu-y-te`
  - `health_table` · drafter · `lap-bang-bieu-y-te`
  - `health_scope_gate` · gate · `gac-pham-vi-chuyen-mon` (không ghi, không web)
  - `health_privacy_gate` · gate · `gac-du-lieu-nguoi-benh` (không ghi, không web)
  - `health_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: yêu cầu chẩn đoán, kê đơn, chỉnh liều, điều trị cho một người cụ thể, hoặc tài liệu bảo người bệnh uống, ngừng hay đổi thuốc; tài liệu có nội dung lâm sàng mà không có người duyệt chuyên môn được nêu; không có nguồn đã duyệt; yêu cầu web khi không được phép. Nguồn có định danh người bệnh: dừng, ghi tên tệp, không chép dữ liệu.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment việc bác sĩ hoặc đơn vị phải làm trước khi dùng (duyệt nội dung lâm sàng, điền dòng người duyệt, kiểm tra ngày, quyết định in). Không gửi, không in phát, không đăng, không kê đơn.
- Điểm dừng loại 3: vòng sửa tối đa 2 cho từng gác. Gác phạm vi hoặc gác dữ liệu còn CHẶN sau vòng 2 thì `blocked`, mention người yêu cầu kèm các dòng còn mở.
- Tài liệu bền: không có sổ. Hồ sơ đơn vị (lead) là nguồn duy nhất về quy trình của đơn vị.
- Phụ thuộc runtime riêng: web cho `health_research` và `health_regulation`, chỉ khi yêu cầu cho phép. Không có Bash.
- Routine: không.
- Giả định mặc định đã biết: đơn vị là phòng khám hoặc cơ sở y tế, không phải bệnh viện có hồ sơ bệnh án điện tử; tài liệu đầu vào là tài liệu của đơn vị.
- Chưa xác minh: quy định y tế hiện hành (`health_regulation` ghi số, ngày, trạng thái, liên kết, không kết luận được phép hay không); liều và ngưỡng trong nguồn được chép nguyên văn, không kiểm đối chiếu với dược thư.
- Trước khi bật: **rủi ro cao**. Một bác sĩ hoặc dược sĩ của đơn vị đọc đầu ra của `health_scope_gate` trong ba đến năm lần chạy đầu. Nếu gác để lọt một chỉ định hoặc một liều, không bật squad cho đến khi sửa xong gác. Gác dữ liệu người bệnh cũng được đọc một lần.
- Diễn tập: chưa chạy.

## Construction (Xây dựng)

- Template: `kv_construction`, floor: Construction, routine: không. Pipeline `day-chuyen-ho-so-cong-truong`.
- Vai:
  - `construction_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `construction_reader` · collector · `doc-ho-so-cong-truong`
  - `construction_standards` · collector · `tra-cuu-quy-chuan-xay-dung` (web)
  - `construction_quantity` · drafter · `lap-bang-khoi-luong`
  - `construction_site_drafter` · drafter · `soan-van-ban-cong-truong`
  - `construction_quantity_gate` · gate · `soat-khoi-luong-don-vi`
  - `construction_safety_gate` · gate · `soat-an-toan-cong-truong`
  - `construction_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu đơn giá hoặc căn cứ khối lượng đã duyệt; thiếu bản vẽ hay thiết kế mà khối lượng phụ thuộc vào; thiếu hạng mục hoặc công việc được nêu tên; bản ghi hiện trường mâu thuẫn với nhật ký. Yêu cầu không thuộc các việc trên thì `blocked`.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment danh sách kiểm tra và chữ ký người phải làm. Biên bản nghiệm thu, biên bản giao ban và hồ sơ hoàn công là bản nháp, không có dòng nào ghi "đã nghiệm thu" hay "đã ký"; người có trách nhiệm ký trên giấy hoặc trên hệ thống của họ.
- Điểm dừng loại 3: vòng sửa tối đa 2 cho gác khối lượng và gác an toàn. Hết vòng thì `blocked`, mention người yêu cầu.
- Tài liệu bền: không có sổ. Hồ sơ đơn vị (lead) cho tên công trình và đơn vị đo.
- Phụ thuộc runtime riêng: web cho `construction_standards` (chỉ khi cho phép). Không có Bash.
- Routine: không.
- Giả định mặc định đã biết: khối lượng là khối lượng của bên làm việc, chưa có đơn giá đã duyệt thì bảng không có tiền.
- Chưa xác minh: hiệu lực của TCVN, QCVN và nghị định; số điều khoản không có nguồn chính thức được ghi `[chưa kiểm tra hiệu lực]`. Squad không tính kết cấu, tải trọng hay độ ổn định.
- Trước khi bật: **rủi ro cao ở gác an toàn** (`soat-an-toan-cong-truong`). Cán bộ an toàn hoặc kỹ sư có chứng chỉ của công trình đọc đầu ra trong các lần chạy đầu. Gác này chặn mọi rủi ro trong nguồn mà bản nháp không nêu, và chặn "Cần giấy tờ" (điều khoản không có nguồn, thông tin cá nhân của công nhân trong văn bản gửi rộng).
- Diễn tập: chưa chạy.

## F&B Ops (Nhà hàng, quán ăn)

- Template: `kv_fnb`, floor: `F&B` (tách với Hospitality), routine: không. Pipeline `day-chuyen-ra-mat-mon-moi`.
- Vai:
  - `fnb_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `fnb_reader` · collector · `doc-so-lieu-quan-an`
  - `fnb_regulation` · collector · `tra-cuu-quy-dinh-fnb` (web)
  - `fnb_drafter` · drafter · `soan-tai-lieu-quan-an`
  - `fnb_allergen_gate` · gate · `soat-di-ung-attp-mon`
  - `fnb_margin_gate` · gate · `soat-gia-von-lai-quan`
  - `fnb_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: món không có công thức có định lượng, nguyên liệu không có giá mua, hoặc nước sốt, gia vị, bột tẩm đóng gói không có danh sách thành phần từ nhãn. Dừng, comment tên món, mention người yêu cầu. Không tính giá vốn trên công thức đoán.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment việc chủ quán và bếp trưởng phải xác nhận: từng nhãn nguyên liệu, từng giá, phí app giao hàng, thuế suất VAT, tên món. Không in, không đăng lên app giao hàng hay mạng xã hội, không đặt hàng nhà cung cấp, không gửi khách.
- Điểm dừng loại 3: vòng sửa tối đa 2. Gác dị ứng còn CHẶN sau vòng 2 thì `blocked`, mention người yêu cầu.
- Tài liệu bền: không có sổ. Hồ sơ đơn vị (lead) cho tên quán và các dữ liệu cố định của quán.
- Phụ thuộc runtime riêng: web cho `fnb_regulation`. Không có Bash hay Python bắt buộc.
- Routine: không.
- Chưa xác minh: quy định an toàn thực phẩm và ghi nhãn chay, thuần chay; phí app giao hàng; tỷ lệ hao hụt. Mọi giá trị này là `[cần xác nhận]` cho đến khi chủ quán xác nhận trong comment.
- Trước khi bật: **rủi ro dị ứng**. Bếp trưởng đọc đầu ra của `fnb_allergen_gate` trong các lần chạy đầu và đối chiếu với nhãn nguyên liệu của nhà cung cấp. Danh sách nhóm dị ứng trong gác là danh sách làm việc, không phải căn cứ pháp lý Việt Nam. Gác này chặn mọi dị ứng bị sót, nhiễm chéo đã có trong nguồn, nhãn chay sai; claim sức khỏe và quy định không nguồn đi vào "Cần giấy tờ" và chặn cho đến khi có xác nhận.
- Diễn tập: chưa chạy.

## Insurance Advisory (Bảo hiểm)

- Template: `kv_insurance`, floor: Insurance, routine: không. Pipeline `day-chuyen-de-xuat-bao-hiem`.
- Vai:
  - `insurance_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `insurance_gather` · collector · `gom-du-kien-tu-van-bao-hiem`
  - `insurance_claim_reader` · collector · `kiem-ke-ho-so-boi-thuong`
  - `insurance_proposal` · drafter · `soan-de-xuat-bao-hiem`
  - `insurance_claim_drafter` · drafter · `soan-thu-huong-dan-claim`
  - `insurance_advice_gate` · gate · `soat-cam-ket-tu-van`
  - `insurance_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: yêu cầu thiếu hồ sơ khách (ghi chú liên hệ, hợp đồng đang có), thiếu tài liệu sản phẩm của gói sẽ trình bày, hoặc thiếu phạm vi (nhu cầu, so sánh, loại trừ hay kịch bản). Yêu cầu không nêu đại lý có giấy phép đọc và trình bày bản nháp thì dừng loại 1.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment việc đại lý có giấy phép phải quyết và gửi (chọn gói, gửi báo giá, ký, nộp hồ sơ). Squad không ràng buộc bảo hiểm, không nộp hồ sơ, không thu phí, không ký, không gửi cho khách.
- Điểm dừng loại 3: vòng sửa tối đa 2. Gác cam kết còn CHẶN sau vòng 2 thì `blocked`, đính kèm bản nháp và các dòng còn mở, mention người yêu cầu.
- Tài liệu bền: không có sổ. Số CCCD, số tài khoản và bệnh án không được chép vào đầu ra (`[đã có, không chép]`).
- Phụ thuộc runtime riêng: không. Không có web (quy định bán hàng và điều kiện bảo hiểm hiện hành được ghi `[cần kiểm tra quy định hiện hành]` cho đại lý).
- Routine: không.
- Chưa xác minh: quy định hiện hành về bán hàng bảo hiểm và điều kiện của từng doanh nghiệp bảo hiểm; mọi số quyền lợi phải khớp tài liệu gốc của sản phẩm.
- Trước khi bật: **rủi ro cao**. Một đại lý có giấy phép đọc đầu ra của `insurance_advice_gate` trước khi bật squad. Gác này coi mọi lỗi là CHẶN (không còn mức nhẹ): lời hứa lợi nhuận hoặc bồi thường, số không có trong tài liệu, so sánh không có tài liệu, che loại trừ, áp lực giả, lộ dữ liệu.
- Diễn tập: chưa chạy.

## Banking Credit Desk (Tài chính, ngân hàng)

- Template: `kv_banking`, floor: Banking, routine: không. Pipeline `day-chuyen-to-trinh-tin-dung`.
- Vai:
  - `banking_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `banking_reader` · collector · `doc-ho-so-tin-dung`
  - `banking_industry` · collector · `tra-cuu-nganh-tai-chinh` (web)
  - `banking_analyst` · drafter · `lap-bang-phan-tich-tin-dung`
  - `banking_memo_drafter` · drafter · `soan-to-trinh-tin-dung`
  - `banking_credit_gate` · gate · `soat-to-trinh-tin-dung`
  - `banking_advice_gate` · gate · `soat-tuan-thu-tu-van-tai-chinh`
  - `banking_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu thư mục hồ sơ của khách vay, thiếu đề xuất (sản phẩm, số tiền, kỳ hạn, mục đích), hoặc thiếu báo cáo tài chính khi tờ trình cần. Dừng, comment việc thiếu, mention người yêu cầu. Yêu cầu đòi quyết định tín dụng, lãi suất hay hạn mức vượt thẩm quyền ghi trong yêu cầu thì dừng.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment các quyết định cán bộ tín dụng phải đưa ra: quyết định tín dụng, lãi suất và phí `[cần xác nhận]`, định giá tài sản bảo đảm. Squad không phê duyệt, không định giá, không gửi cho khách.
- Điểm dừng loại 3: vòng sửa tối đa 2 cho gác tờ trình và gác tư vấn. Gác còn CHẶN sau vòng 2 thì `blocked`, mention người yêu cầu.
- Tài liệu bền: không có sổ. Mẫu tờ trình và ngưỡng nội bộ là tài liệu của ngân hàng, do người dùng cung cấp hoặc để `[cần bổ sung]`.
- Phụ thuộc runtime riêng: web cho `banking_industry` (chỉ khi cho phép). Python cho script lịch trả nợ của `lich-tra-no-khoan-vay` (mục 4).
- Routine: không.
- Chưa xác minh: lãi suất, phí, hạn mức và ngưỡng tỷ lệ; mọi con số là `[cần bổ sung]` cho đến khi có nguồn của ngân hàng và ngày.
- Trước khi bật: **rủi ro cao**. Một cán bộ tín dụng đọc đầu ra của hai gác trong các lần chạy đầu, vì squad này soạn tài liệu cho quyết định tín dụng. Squad không đưa ra "khách được duyệt", không tư vấn đầu tư, không hứa lợi nhuận; yêu cầu loại này bị dừng.
- Diễn tập: chưa chạy.

## Real Estate Desk (Bất động sản)

- Template: `kv_realestate`, floor: Real Estate, routine: không. Pipeline `day-chuyen-ho-so-can-ban`.
- Vai:
  - `realestate_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `realestate_reader` · collector · `doc-ho-so-nha-dat`
  - `realestate_area_research` · collector · `tra-cuu-khu-vuc-bds` (web)
  - `realestate_loan_drafter` · drafter · `lap-phuong-an-vay-bds`
  - `realestate_listing_drafter` · drafter · `soan-tin-dang-bds`
  - `realestate_planning_gate` · gate · `soat-nguon-quy-hoach-bds`
  - `realestate_loan_gate` · gate · `soat-phuong-an-vay-bds`
  - `realestate_listing_gate` · gate · `soat-tin-dang-bds`
  - `realestate_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu thư mục hồ sơ; tin không có thông tin sổ, loại sổ hoặc giá; phương án vay thiếu số tiền, kỳ hạn, lãi ưu đãi và số tháng, hoặc mục đích; sổ mâu thuẫn với nguồn. Yêu cầu hứa giá, hứa lợi nhuận hay hứa duyệt vay thì dừng và nói rõ với người yêu cầu.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment việc môi giới phải làm: xác nhận thông tin sổ và loại sổ, xác nhận từng lãi suất và phí, đưa câu hỏi pháp lý về sổ cho công chứng viên hoặc luật sư, quyết định kênh đăng và số liên hệ. Không đăng, không gửi, không ký, không nộp vay.
- Điểm dừng loại 3: vòng sửa tối đa 2 cho từng gác. Gác còn CHẶN sau vòng 2 thì `blocked`, mention người yêu cầu.
- Tài liệu bền: không có sổ. Số sổ, số CCCD, số tài khoản và địa chỉ đầy đủ của chủ nhà không vào đầu ra.
- Phụ thuộc runtime riêng: web cho `realestate_area_research` (chỉ khi cho phép). Python cho script lịch trả nợ (`tinh-phuong-an-vay-mua-nha`) và script kiểm tin (`viet-tin-dang-nha-dat`), mục 4.
- Routine: không.
- Chưa xác minh: quy hoạch và hạ tầng (gác nguồn chỉ chấp nhận nguồn bậc 1-2 có số và ngày); lãi suất ưu đãi và lãi thả nổi; tình trạng pháp lý của sổ (squad không kết luận sổ sạch hay không tranh chấp).
- Trước khi bật: **rủi ro về tiền và lời hứa**. Môi giới có kinh nghiệm đọc đầu ra của `realestate_loan_gate` và `realestate_listing_gate` trong các lần chạy đầu. Gác tin đăng chặn trái nguồn, không có trong nguồn, tự suy, lời hứa lợi nhuận và tin quy hoạch không nguồn.
- Diễn tập: chưa chạy.

## Hospitality Ops (Du lịch, khách sạn)

- Template: `kv_hospitality`, floor: Hospitality, routine: không. Pipeline `day-chuyen-ho-so-tour-doan`.
- Vai:
  - `hospitality_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `hospitality_intake` · collector · `gom-yeu-cau-doan-khach`
  - `hospitality_destination` · collector · `tra-cuu-diem-den-du-lich` (web)
  - `hospitality_tour_drafter` · drafter · `soan-ho-so-tour`
  - `hospitality_hotel_drafter` · drafter · `soan-noi-dung-khach-san`
  - `hospitality_quote_gate` · gate · `soat-lich-trinh-bao-gia`
  - `hospitality_promise_gate` · gate · `soat-cam-ket-dich-vu-luu-tru`
  - `hospitality_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu số khách, ngày, hoặc bảng giá nhà cung cấp cho báo giá; chính sách hủy, đặt cọc hay nhận phòng không có trong nguồn đã duyệt. Dừng, comment việc thiếu, mention người yêu cầu. Yêu cầu bảo squad đặt phòng, cọc hay hoàn tiền thì không chạy.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment việc quản lý phải làm trước khi dùng: xác nhận từng đơn giá với nhà cung cấp, kiểm tra phòng trống, duyệt điều khoản cọc và hủy, gửi báo giá, đăng listing. Squad không đặt, không giữ chỗ, không thu tiền, không hoàn tiền, không gửi cho khách, đại lý, nhà cung cấp hay nền tảng.
- Điểm dừng loại 3: vòng sửa tối đa 2 cho từng gác. Gác báo giá hoặc gác cam kết còn CHẶN sau vòng 2 thì `blocked`, mention người yêu cầu.
- Tài liệu bền: không có sổ. Thông tin khách (họ tên kèm phòng, số điện thoại, email, giấy tờ, mã đặt chỗ) không vào kế hoạch, tiêu đề sub-issue hay báo cáo.
- Phụ thuộc runtime riêng: web cho `hospitality_destination` (chỉ khi cho phép). Không có Bash.
- Routine: không.
- Chưa xác minh: giờ mở cửa, giá vé, giờ bay và giá phòng theo mùa; mọi con số thiếu nguồn ở mức `Cần giấy tờ` và chặn báo giá.
- Trước khi bật: **rủi ro giá và lời hứa với khách**. Quản lý khách sạn đọc đầu ra của hai gác trong các lần chạy đầu. Gác báo giá chặn sai số tiền, lộ giá vốn, lịch trình không khả thi, và chặn đơn giá thiếu nguồn hay thiếu xác nhận. Gác cam kết chặn hoàn tiền, giảm giá, đền bù chưa duyệt, và chặn lộ dữ liệu khách.
- Diễn tập: chưa chạy.

## Import & Export Desk (Xuất nhập khẩu)

- Template: `kv_trade`, floor: `Import & Export`, routine: không. Pipeline `day-chuyen-ho-so-lo-hang`.
- Vai:
  - `trade_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `trade_docs_reader` · collector · `gom-chung-tu-lo-hang`
  - `trade_customs_research` · collector · `tra-cuu-chinh-sach-hai-quan` (web)
  - `trade_crosscheck_drafter` · drafter · `lap-bang-doi-chieu-xnk`
  - `trade_docs_gate` · gate · `soat-chung-tu-xnk`
  - `trade_customs_gate` · gate · `soat-khai-bao-hai-quan`
  - `trade_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu hóa đơn, packing list hoặc chứng từ vận tải (B/L, AWB); người khai chưa nói loại hình (loại hình xuất nhập). Dừng, comment việc thiếu kèm mã lô, mention người yêu cầu.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment việc người khai phải quyết: từng mã HS có độ tin cậy thấp, từng thuế suất, ngày tỷ giá, loại hình, khóa phân bổ cước và bảo hiểm, giấy phép hoặc kiểm tra chuyên ngành nếu hàng cần. Không khai báo hải quan, không nhập ECUS hay VNACCS, không ký, không thanh toán, không gửi nhà cung cấp, ngân hàng hay người mua.
- Điểm dừng loại 3: vòng sửa tối đa 2. Một trong hai gác còn CHẶN sau vòng 2 thì `blocked`, mention người yêu cầu.
- Tài liệu bền: không có sổ. Hồ sơ đơn vị (lead) cho thông tin doanh nghiệp.
- Phụ thuộc runtime riêng: web cho `trade_customs_research` (chỉ khi cho phép). Không có Bash hay Python bắt buộc.
- Routine: không.
- Chưa xác minh: mã HS, thuế suất, thuế ưu đãi FTA, tỷ giá. Squad không khẳng định con số nào; mọi giá trị là đề xuất kèm nguồn, hoặc `[cần tra biểu thuế]`, và gác hải quan chặn thuế không có nguồn.
- Trước khi bật: không có gác nào cần người trong ngành đọc trước, nhưng người khai hải quan vẫn phải xác nhận mọi mã và thuế trước khi khai (squad không khai).
- Diễn tập: chưa chạy.

## Factory Ops (Sản xuất nhà máy)

- Template: `kv_factory`, floor: Manufacturing, routine: không. Pipeline `day-chuyen-bao-cao-kpi-nha-may`.
- Vai:
  - `factory_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `factory_reader` · collector · `doc-du-lieu-san-xuat`
  - `factory_standards` · collector · `tra-cuu-tieu-chuan-nha-may` (web)
  - `factory_kpi_drafter` · drafter · `lap-bang-so-lieu-san-xuat`
  - `factory_doc_drafter` · drafter · `soan-ho-so-nha-may`
  - `factory_numbers_gate` · gate · `soat-so-lieu-chat-luong`
  - `factory_safety_gate` · gate · `soat-an-toan-nha-may`
  - `factory_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: yêu cầu chỉnh máy, đặt thông số quy trình, quyết định lô hàng, hoặc đóng NCR, CAPA; yêu cầu thiếu định nghĩa KPI và mục tiêu, thiếu kỳ báo cáo, hoặc thiếu thư mục nguồn; báo cáo tháng thiếu danh sách ca (ca thiếu được ghi là thiếu, không lấp bằng trung bình).
- Điểm dừng loại 2: handover QUA thì `in_review`, comment các quyết định người phụ trách phải làm trước khi phát hành: duyệt số liệu, quyết định gửi báo cáo, ký NCR hay CAPA, đóng sự cố. Squad không gửi báo cáo cho hội đồng, khách hàng hay cơ quan quản lý.
- Điểm dừng loại 3: vòng sửa tối đa 2 cho gác số liệu và gác an toàn. Gác còn CHẶN sau vòng 2 thì `blocked`, đính kèm bản nháp và các dòng còn mở.
- Tài liệu bền: không có sổ. Số NCR, CAPA và ngày ghi `[cần bổ sung]` cho đến khi người yêu cầu xác nhận trong comment.
- Phụ thuộc runtime riêng: web cho `factory_standards` (chỉ khi cho phép). Không có Bash hay Python bắt buộc.
- Routine: không.
- Chưa xác minh: thông số quy trình, dung sai và chu kỳ bảo trì phải có nguồn của nhà máy; squad không đặt giá trị từ trí nhớ.
- Trước khi bật: **rủi ro an toàn lao động**. Cán bộ an toàn hoặc người phụ trách an toàn của nhà máy đọc đầu ra của `factory_safety_gate` trong các lần chạy đầu. Gác này chặn mọi sự cố trong nguồn mà bản nháp không nêu, nguyên nhân gán khi chưa có điều tra, và thông số an toàn tự đặt. Gác số liệu không chặn số không có nguồn (giữ theo chuẩn P0).
- Diễn tập: chưa chạy.

## Education Desk (Giáo dục)

- Template: `kv_education`, floor: Education, routine: không. Pipeline `day-chuyen-de-kiem-tra-dinh-ky`.
- Vai (sáu vai, gồm lead và handover; bốn vai nghề còn lại không được ghép ở squad này, squad Teaching Materials dưới đây nhận chúng):
  - `edu_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `edu_curriculum` · collector · `tra-yeu-cau-can-dat` (web)
  - `edu_test_drafter` · drafter · `soan-de-va-dap-an`
  - `edu_answer_gate` · gate · `soat-dap-an-de-thi`
  - `edu_student_gate` · gate · `soat-an-toan-hoc-sinh`
  - `edu_handover` · handover · `kiem-dinh-ban-giao`
- Điểm dừng loại 1: thiếu môn, khối, thời lượng, hình thức, tỷ lệ mức (Biết, Hiểu, Vận dụng), thang điểm hoặc mẫu ma trận của tổ; tài liệu đính kèm có dữ liệu học sinh (tên kèm điểm, số định danh, số điện thoại, phụ huynh, nhận xét hạnh kiểm). Dừng, comment việc thiếu hoặc cột cần bỏ, mention người yêu cầu.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment việc giáo viên phải làm: kiểm đáp án, duyệt ma trận, in hoặc chia sẻ đề. Đề và đáp án không đưa vào bản học sinh; squad không gửi học sinh, phụ huynh hay nền tảng học tập nào.
- Điểm dừng loại 3: vòng sửa tối đa 2 cho gác đáp án và gác học sinh. Gác còn CHẶN sau vòng 2 thì `blocked`, giao các dòng còn mở cho giáo viên.
- Dấu hiệu an toàn (bạo hành, xâm hại, tự hại liên quan đến học sinh) trong tài liệu: dừng ngay, `blocked`, nêu dấu hiệu trong comment và mention người yêu cầu; không chạy vai nào sau đó. Ban giám hiệu xử lý theo quy trình riêng.
- Tài liệu bền: không có sổ. Đề kiểm tra và đáp án là hai tệp riêng, gắn nhãn bản học sinh và bản giáo viên.
- Phụ thuộc runtime riêng: web cho `edu_curriculum` (chỉ khi cho phép). Không có Bash.
- Routine: không.
- Chưa xác minh: yêu cầu cần đạt theo chương trình hiện hành (`edu_curriculum` ghi `[không tìm thấy nguồn]` khi không có nguồn chính thức).
- Trước khi bật: **rủi ro về trẻ em và dữ liệu học sinh**. Một cán bộ nhà trường đọc đầu ra của `edu_student_gate` trước khi bật. Gác này chặn dữ liệu cá nhân, sai kênh, cam kết vượt thẩm quyền, nhãn tính cách và dấu hiệu an toàn. Đáp án đề chặn mọi câu sai, câu hai đáp án đúng, câu không có đáp án đúng.
- Diễn tập: chưa chạy.

## Teaching Materials (Tư liệu giảng dạy)

- Template: `kv_teaching`, floor: Education (dùng chung với Education Desk, mục 8c), routine: không, dây chuyền: không (squad viết tay, `flow: authored`, cấu hình `kitviet-squads/teaching.json`). Cờ `expertReview`: không. Squad này và Education Desk không làm cùng một việc: Education Desk soạn đề và đáp án, Teaching Materials làm tư liệu bài giảng, nhận xét, thông báo phụ huynh và văn bản chủ nhiệm.
- Vai (tám vai, gồm lead và handover):
  - `tm_lead` · lead · `lap-ke-hoach-dieu-phoi`
  - `tm_sources` · collector · `tim-tu-lieu-bai-giang` (web)
  - `tm_records` · collector · `tong-hop-ho-so-lop-hoc`
  - `tm_drafter` · drafter · `soan-van-ban-chu-nhiem`
  - `tm_source_gate` · gate · `soat-nguon-tu-lieu-giang-day`
  - `tm_student_gate` · gate · `soat-an-toan-hoc-sinh`
  - `tm_numbers_gate` · gate · `soat-so-lieu` (gác dùng chung)
  - `tm_handover` · handover · `kiem-dinh-ban-giao`
- Skill theo vai: `tm_drafter`: `nhan-xet-hoc-ba`, `tin-nhan-phu-huynh-zalo`, `hop-phu-huynh-lop`, `ke-hoach-chu-nhiem-lop`; `tm_sources`: `nghien-cuu-tu-lieu-bai-giang`. Mười một skill còn lại của giáo dục nằm trong `deferredSkills`, chưa gắn cho squad nào.
- Điểm dừng loại 1 (khi nhận yêu cầu): yêu cầu thiếu lớp, môn hoặc mục tiêu bài; hoặc hồ sơ đính kèm có cột học sinh (tên kèm điểm, chuyên cần, ghi chú hạnh kiểm hay gia đình, sức khỏe, khuyết tật, số định danh, số điện thoại, phụ huynh) mà yêu cầu không nói rõ là đã ẩn danh (chỉ mã, không tên, không liên hệ) hoặc đã được duyệt (kèm người duyệt). Lead đặt `blocked`, comment danh sách cột cần bỏ hoặc xác nhận, mention người yêu cầu; không chạy `tm_records` trên đầu vào đoán.
- Mã hóa: `tm_records` chỉ trả mã (HS01, HS02...) và tổng; bảng tên-mã ở lại với giáo viên, không vào đầu ra, tài liệu, tài liệu KB hay comment. Ghi chú nhạy cảm được báo "có ghi chú nhạy cảm - GV tự xử lý" và không chuyển cho `tm_drafter`.
- Nguồn: mỗi ý ghi tên nguồn, đơn vị, năm, liên kết hoặc trang, ngày truy cập và mức (1 đến 4). Nguồn chưa mở ghi `[chưa mở nguồn]`; mức 4 không làm căn cứ; ý không có nguồn ghi `[không tìm thấy nguồn]`.
- Điểm dừng loại 2: handover QUA thì `in_review`, comment việc giáo viên phải làm: kiểm nguồn, duyệt từng văn bản, quyết từng số. Squad không gửi phụ huynh hay học sinh, không đăng nhóm chat, không tải lên hệ thống nhà trường.
- Điểm dừng loại 3: vòng sửa tối đa 2 cho gác nguồn, gác học sinh và gác số liệu. Gác còn CHẶN sau vòng 2 thì `blocked`, giao các dòng còn mở cho giáo viên.
- Dấu hiệu an toàn (bạo hành, xâm hại, tự hại liên quan đến học sinh) trong tài liệu hoặc hồ sơ: dừng ngay, `blocked`, nêu dấu hiệu trong comment và mention người yêu cầu; không chạy vai nào sau đó. Ban giám hiệu xử lý theo quy trình riêng.
- Phụ thuộc runtime riêng: web cho `tm_sources` (chỉ khi cho phép). Không có Bash, không cần thư viện xuất file.
- Routine: không.
- Chưa xác minh: chương trình và tài liệu hiện hành (`tm_sources` ghi `[không tìm thấy nguồn]` khi không có nguồn chính thức).
- Trước khi bật: **rủi ro về trẻ em và dữ liệu học sinh**, giống Education Desk. Gác `soat-an-toan-hoc-sinh` đọc mọi văn bản gửi học sinh và phụ huynh; nên có một cán bộ nhà trường đọc đầu ra của gác này trong các lần chạy đầu. Gác này không nằm trong bảng mục 8d vì không có cờ chuyên gia.
- Diễn tập: chưa chạy.

## 8b. Skill mở rộng trên squad có sẵn (spec 7)

Extension skills on existing squads (spec 7). The Kit Viet extension skills attach to the company starter squads Support, People & Performance, Product Analysis, Planning and Build (kitvietExtensionTargetOf in packages/core/squads/kitviet-extension-attach.ts). Company squads carry no template_key, so they match by card name. After a company apply, the client runs attachKitvietExtensions (useApplyCompanyTemplate). It imports each target's bundles with the extension pack only, and adds them to the squad's skills without removing any. Running the company apply again on an earlier workspace attaches the skills to its existing squads. A renamed company squad is not matched; a hand-made squad named like a card with no template is matched too. A failed squad is logged; the apply is not affected.

## 8c. Floor của squad P2

Mười nghề P2 mỗi nghề một floor, tên floor là `floorName` của template (`packages/core/squads/kitviet-floors.ts`): Healthcare, Construction, F&B, Insurance, Banking, Real Estate, Hospitality, Import & Export, Manufacturing, Education. Teaching Materials dùng chung floor Education với Education Desk, là ngoại lệ duy nhất. Floor không nằm trong `COMPANY_FLOOR_NAMES`, nên bộ chọn công ty không hiện chúng; quickstart tự tạo floor khi chưa có (`ensureCompanyFloor` tìm theo tên, không thấy thì tạo) rồi xếp squad vào ô trống đầu tiên. Floor đầy thì squad vẫn được tạo, báo `no free seat on the <tên> floor`.

Trần số floor của workspace là 24 (`CompanyMaxFloors` ở `server/internal/service/company_setup.go`, `companyMaxFloors` ở `server/internal/handler/company.go`, `COMPANY_MAX_FLOORS` ở `packages/core/company/constants.ts`). Một công ty khởi đầu đủ dùng bảy floor mặc định; mười floor P2 đưa tổng lên 17, còn chỗ cho các floor khác.

## 8d. Trước khi bật các squad rủi ro

Sáu squad có gác phải có người trong ngành đọc đầu ra trước khi bật. Người đọc không thay gác, và gác không thay người đọc.

| Squad | Gác cần người đọc | Lý do |
| --- | --- | --- |
| Healthcare | `health_scope_gate` (và `health_privacy_gate`) | Phạm vi lâm sàng: chẩn đoán, liều, cam kết khỏi bệnh |
| Insurance Advisory | `insurance_advice_gate` | Cam kết tư vấn cho khách; mọi lỗi đều chặn |
| Banking Credit Desk | `banking_credit_gate`, `banking_advice_gate` | Tờ trình tín dụng và tài liệu gửi khách |
| Factory Ops | `factory_safety_gate` | An toàn lao động; không đặt thông số tự do |
| F&B Ops | `fnb_allergen_gate` | Dị ứng; chỉ tin nhãn nguyên liệu của nhà cung cấp |
| Construction | `construction_safety_gate` | An toàn công trường và thẩm quyền ký |

Sáu squad trên là các squad có cờ `expertReview` (mục 8e): bộ chọn yêu cầu xác nhận trước khi tạo.

Ngoài sáu squad trên, Education có `edu_student_gate` (dữ liệu học sinh), Real Estate có `realestate_loan_gate` và `realestate_listing_gate` (tiền và lời hứa), Hospitality có `hospitality_quote_gate` và `hospitality_promise_gate` (giá và cam kết với khách). Các gác này cũng chặn cho đến khi có xác nhận, nhưng không yêu cầu người trong ngành đọc trước.

Gác nào trả CHẶN sau hai vòng thì squad dừng loại 3. Gác nào QUA mà người đọc thấy sai thì sửa gác (bản vá trong `catalog/_sources/kitviet/patches/`) trước khi chạy tiếp, không sửa đầu ra bằng tay.

## 8e. Bộ chọn squad, cờ xem xét chuyên môn và chuỗi cấm

- **Bộ chọn.** Các template chia nhóm theo `SQUAD_TEMPLATE_CATEGORIES` trong `packages/core/squads/templates.ts`: `core` (squad SDLC, nghiên cứu, lập kế hoạch, Sales Loop), `kitviet` (P0 và P1), `vertical` (P2 và Teaching Materials). Tiêu đề nhóm là `squad_quickstart.groups.<nhóm>` ở bảy ngôn ngữ. Template không có `category` rơi vào nhóm `vertical`. Thêm một squad Kit Việt: đưa template vào `inCategory(...)` đúng nhóm, rồi thêm khóa dịch.
- **Cờ xem xét chuyên môn (`expertReview`, Q13).** Cờ nằm ở file template (`SquadTemplate.expertReview`). Hiện có sáu: Healthcare (`health.ts`), Construction, F&B Ops, Insurance Advisory, Banking Credit Desk, Factory Ops, đúng bằng bảng mục 8d. Bộ chọn hiện huy hiệu "Expert review" (`expert_review_badge`) và, khi được chọn, thông báo (`expert_review_notice`) cùng ô xác nhận (`expert_review_ack`) đứng trước nút tạo; nút tạo bị khóa đến khi tích. Chọn squad khác sẽ xóa xác nhận. Máy chủ không kiểm cờ. Đổi danh sách: sửa cờ trong file template, cập nhật bảng mục 8d và test "marks only the squads that need expert review" (`packages/views/modals/squad-quickstart.test.tsx`).
- **Chuỗi nền tảng (guard, Q14).** Generator (`FORBIDDEN` trong `scripts/generate-kitviet-pack.mjs`) và test `kitviet-skills.test.ts` cùng cấm trong mọi thân skill và agent: `kv:pro`, `mochistudio`, `Bản Pro`, `.kv-run`, `CLAUDE.md`, `AGENTS.md`, `ket-qua/`, `phiên chính`, `Claude Code`, `Claude web`, `Claude Desktop`, `Claude.ai` (không phân biệt hoa thường), `Codex`, `Cursor`, tệp đánh số (`01-…md`), tệp có phiên bản (`-v2.md`), lệnh cài (`pip install`, `apt install`, "hoặc cài LibreOffice", "Cài bằng lệnh"), tham chiếu `kv:` chưa phân giải, và tên thư mục tác giả (`kv-delivery-core` thay vì `kv--delivery-core`). Từ "Claude" đứng một mình được giữ: một ví dụ nhật ký dùng AI (bảng ghi "Claude" là tên công cụ) nằm trong một skill và là ví dụ hợp lệ. Thêm chuỗi cấm: sửa cả generator và test, rồi chạy `pnpm generate:kitviet`.
- **Tên cơ quan (quy tắc viết lại).** Tên cơ quan không gắn cứng vào một mốc đổi tên: `Cục Hải quan` thành `cơ quan hải quan` (`customs-agency-name`); `Tổng cục Thống kê (nay là Cục Thống kê, Bộ Tài chính)` thành `cơ quan thống kê nhà nước (tên cơ quan hiện hành: xem nguồn chính thức)` (`statistics-agency-history`). Tên hiện hành phải đến từ nguồn chính thức, không từ skill.
- **Mục nền tảng và dòng nhắc nền tảng trong bản vá.** Các câu như "(Claude Code: …)" và "lên Claude web" được viết lại qua `kitviet-rewrites.json` (`claude-code-colon-note`, `paste-into-chat-web`), không cần bản vá riêng.

## 8f. Flow: workflow bắt buộc của squad (spec `kitviet-workflow-spec.md`)

> Flow do người dùng **soạn, kiểm, mô phỏng và xuất bản** (bản nháp, phiên bản khóa, nhân bản hàng Kit Việt, cờ `FF_FLOW_AUTHORING` và `FF_FLOW_GRAPH_V2`, quyền, ack, quay lui, đọc `issues[]`, mã lỗi) nằm ở `flow-authoring-runbook.md`; mục này chỉ nói về flow do catalog tạo. Hàng Kit Việt vẫn khóa (`workflow_managed_by_template`); muốn sửa thì nhân bản thành bản nháp, bản sao không còn `template_ref` nên nâng cấp Kit Việt không động tới nó.

Mỗi squad Kit Việt có một workflow flow (manifest `catalog/workflows/flow_<id squad>/template.json`, khối `flow`). Máy chủ mở sub-issue cho từng task của bước, chờ mọi task đóng, mở lại **mọi** gate sau mỗi bản sửa, đếm vòng (tối đa 2) và đăng comment ba điểm dừng. Lead chỉ lập kế hoạch (`multica issue flow plan`) và gọi `multica issue flow advance`. Đọc kế hoạch chi tiết ở `kitviet-workflow-plan.md`.

| Squad | Template (`template_ref`) | Route | Công tắc khác |
| --- | --- | --- | --- |
| `kv_academic` | `flow_kv_academic@17c8b737` | `chapter_draft`, `chapter_review` | `web` |
| `kv_accounting` | `flow_kv_accounting@ebf3a4d9` | `month_close`, `tax_letter`, `tax_calendar` | `tax_lookup`, `export` |
| `kv_admin` | `flow_kv_admin@5d2f1037` | (một route) | `format_lookup`, `has_figures`, `export` |
| `kv_b2b` | `flow_kv_b2b@613bcc0e` | (một route) | `company_lookup`, `export` |
| `kv_banking` | `flow_kv_banking@19291576` | (một route) | `customer_facing`, `export` |
| `kv_construction` | `flow_kv_construction@80caefb4` | (một route) | `has_money_table`, `export` |
| `kv_ecom` | `flow_kv_ecom@4718f565` | `listing`, `aftersale`, `order_report` | `has_figures` |
| `kv_education` | `flow_kv_education@2166f2c3` | (một route) | `export` |
| `kv_factory` | `flow_kv_factory@4f00994d` | `kpi_report`, `quality_record` | `export` |
| `kv_fnb` | `flow_kv_fnb@16965907` | (một route) | `regulation_check`, `export` |
| `kv_health` | `flow_kv_health@f70324d5` | (một route) | `web_regulation`, `web_research`, `needs_table` |
| `kv_hospitality` | `flow_kv_hospitality@3895701d` | `tour_quote`, `guest_text` | `web` |
| `kv_insurance` | `flow_kv_insurance@82b656ff` | `proposal`, `claim_guidance` | - |
| `kv_legal` | `flow_kv_legal@6b0343ec` | `contract_review`, `validity_review` | `legal_sources`, `has_citations`, `export` |
| `kv_marketing` | `flow_kv_marketing@c75ebcd1` | (một route) | `message_approved`, `export` |
| `kv_officeops` | `flow_kv_officeops@10d72de3` | (một route) | `has_source_files`, `has_costs`, `export` |
| `kv_pr` | `flow_kv_pr@c1cedbca` | `article_check`, `release_qa` | `message_approved`, `has_figures`, `export` |
| `kv_realestate` | `flow_kv_realestate@5cfbaee7` | `listing_pack`, `loan_plan` | `area_research`, `mentions_planning` |
| `kv_talent` | `flow_kv_talent@c52a70d6` | (một route) | `export` |
| `kv_teaching` | `flow_kv_teaching@9d083bd5` | `teaching_material`, `class_records` | `student_facing`, `records_anonymised` |
| `kv_trade` | `flow_kv_trade@b10d7319` | `shipment_dossier` | `customs_lookup`, `export` |

### Công tắc và lệnh

| Việc | Cách |
| --- | --- |
| Xem trạng thái request | `multica issue flow show <id>`. Request không do flow chạy: in `{"managed": false}` và lead theo quy trình cũ |
| Lập kế hoạch | lead: `multica issue flow plan <id> --switch route=<route> --switch customs_lookup=true`; thiếu căn cứ: `--blocked --missing "<mục>" --reason "<vì sao>"` (dừng 1) |
| Tiếp tục request đã dừng | thành viên quản lý squad: `multica issue flow resume <id>` (dừng 3 đặt lại vòng sửa; dừng 1 mở lại task bị chặn; `stalled` đưa task treo về `todo`) |
| Gỡ request khỏi flow | `multica issue flow detach <id> [--cancel-tasks]`; task thành sub-issue thường |
| Bật runner nền (tầng 2) | `FF_FLOW_RUNNER=on` cho máy chủ, rồi `PUT /api/issue-workflows/<id>/flow-auto {"auto": true}` (quản trị workspace). Mặc định tắt. Tắt một trong hai thì `child_done` bắn lại ngay và lead tự gọi `advance` |
| Tắt cả engine | `FF_FLOW_ENGINE=off`: guard và API flow nhường chỗ; trạng thái lưu giữ nguyên |
| Cập nhật bản flow mới | nút cập nhật Kit Việt tạo hàng workflow mới theo `rev` và đặt làm workflow đang dùng; request đang chạy ở lại bản cũ. Squad đang chạy workflow không do template tạo thì giữ nguyên |

### Điểm dừng

| Dừng | Tiêu đề comment | Ai làm gì |
| --- | --- | --- |
| 1 | `BLOCKED · stop 1 · thiếu căn cứ` | Người yêu cầu bổ sung đầu vào; thành viên quản lý `resume` |
| 2 | `IN REVIEW · stop 2 · trước khi ra khỏi workspace` | Người nhận đọc sản phẩm, tự gửi/nộp/ký/công bố ngoài workspace, chuyển request sang `done` (agent không làm được) |
| 3 | `BLOCKED · stop 3 · hết 2 vòng sửa` | Người yêu cầu quyết định đổi bản nháp; thành viên quản lý `resume` |
| treo | `BLOCKED · stalled · request không tiến thêm được` | Runner thấy task hết số lần thử, lead không lập kế hoạch, hoặc agent bị lưu trữ; `resume` sau khi xử lý |
| chờ duyệt | `WAITING · chờ duyệt thông điệp` | Marketing và PR: người duyệt thông điệp rồi chuyển request sang `Draft` |

### Dựng chỉ mục và kiểm thử trên DB riêng

Migration `985` thêm cột, `986` và `987` tạo chỉ mục bằng `CREATE INDEX CONCURRENTLY` (xem `server/cmd/migrate/README.md`); trên bảng `issue` lớn, chạy chúng ngoài giờ cao điểm. Test DB theo D19: dùng một database có tên kết thúc `_test`, đi qua `scripts/require-test-database.sh`, đặt `DATABASE_URL` tường minh trong chính lệnh. Không dùng `make test`, `make check`, `make migrate-up`.

### Hoàn tác

Thứ tự: tắt runner → tắt engine → `detach` từng request → chạy down `987`, `986`, `985` (do người dùng tự chạy với DSN của họ). `985` down xóa tám cột và mất `flow`, `outcome`. Bảng đầy đủ ở `kitviet-workflow-plan.md` mục 4.1; test: `flow_rollback_test.go`.

## 9. Việc còn mở

### Nâng cấp lên v1.5.0 (2026-11-12)

- **Không đổi nội dung.** Từ v1.4.0 đến v1.5.0, mọi thay đổi trong upstream là dòng quảng cáo Pro, nằm trong khối bị gỡ. Không có bản vá nào đổi hay bị bỏ. Không có text agent hay squad nào đổi. Diff của dữ liệu sinh ra chỉ là trường `ref` của 348 cấu hình skill.
- **Tám skill BIM của nghề xây dựng giữ ngoài pack.** Upstream 1.5.0 thêm `xay-dung/skills/`: `bao-cao-va-cham-bim`, `boc-khoi-luong-tu-ban-ve-cad`, `checklist-nop-ho-so-bim-tham-dinh`, `doi-chieu-khoi-luong-mo-hinh-du-toan`, `lap-bep-ke-hoach-thuc-hien-bim`, `lap-eir-yeu-cau-thong-tin-bim`, `quy-tac-dat-ten-cde-bim`, `tong-hop-vat-tu-theo-hang-muc`. Ba skill (`boc-khoi-luong-tu-ban-ve-cad`, `checklist-nop-ho-so-bim-tham-dinh`, `lap-eir-yeu-cau-thong-tin-bim`) trích Nghị định 206/2026/NĐ-CP, 217/2026/NĐ-CP, 10/2021 và 175/2024. Test "legal citations" chặn chúng, và nghề Xây dựng chưa có Sổ quy định để trỏ tới. Ba skill khác (`doi-chieu-khoi-luong-mo-hinh-du-toan`, `quy-tac-dat-ten-cde-bim`, `tong-hop-vat-tu-theo-hang-muc`) tham chiếu tới ba skill trên, `lap-bep-ke-hoach-thuc-hien-bim` tham chiếu tới `quy-tac-dat-ten-cde-bim`, và `bao-cao-va-cham-bim` giữ cùng nhóm vì cùng chủ đề BIM. Cả tám nằm trong `droppedSkills` của `kitviet-rewrites.json`; mã nguồn vẫn được vendor. **Cần chốt:** (a) đối chiếu nguồn chính thức cho các mốc trên; (b) nếu đưa vào, tạo Sổ quy định Xây dựng rồi patch câu trích dẫn; (c) gắn vào vai `construction_quantity` hay chỉ đóng gói (`deferredSkills`).
- **Registry không ghi thêm text ở bản 1.5.0.** Vì đầu ra agent và squad không đổi, không có text cũ nào bị thay, nên lệnh không ghi gì. Các workspace tạo từ v1.4.0 đang trùng text hiện hành, nên vẫn nâng được. Ghi thêm bản v1.4.0 dù trùng text hiện hành sẽ vi phạm test "previous text ≠ current text". **Cần chốt:** có muốn ghi bản đối chứng này (và đổi test) không. Mặc định: không.
- **Một bước vẫn cần tay.** `pnpm kitviet:update` là lệnh terminal, cập nhật bộ nguồn trong repo. Nút trên trang Squads (mục 9) chỉ cập nhật squad trong workspace theo bộ đã có trong repo. Lệnh không commit. Mốc pháp luật mới trong upstream vẫn phải đối chiếu bằng tay. Nghề mới của upstream chỉ được in tên, chưa được vendor cho đến khi có config.

- Registry nâng cấp (`kitviet-previous-texts.ts`) đã bổ sung 138 text, từ 142 lên 280 text agent và từ 70 lên 92 role key. Phần thêm: text gate của `f27661e8a` (48 role, bị thay bởi quy tắc không lặp dòng VERDICT), text gate của `1c604d8ad` (48 role, bị thay ở `2375d7ba1`), và text của `e787f2217` (42 role: 14 gate, 5 lead, 23 collector, drafter và handover, bị thay ở `1c604d8ad`). Nguồn là hàm `loadKitvietRoleInstructions` dựng lại từ `git archive` tại từng commit, so khớp theo role; không sửa entry cũ. Bằng chứng text đã được dùng: 53 agent trong DB diễn tập (`multica_multica_kitviet_332`, chỉ đọc) có text trùng phần thêm.
  - Không có tag release nào chứa commit Kit Việt, nên "đã phát hành" trong registry nghĩa là "đã có trên main". Commit Kit Việt đầu tiên trên lịch sử của HEAD là `aa996c9bb` (2026-11-10). `1e2ba5a19` không nằm trong lịch sử của HEAD.
  - Chưa có trong registry: (a) text của giai đoạn trước `e787f2217`, khi chưa có `composeKvAgentInstructions` (từ `aa996c9bb` đến `7a93d1136`); chưa dựng lại từ git; (b) 4 agent trong DB có text không khớp text hiện tại lẫn registry; chưa rõ nguồn.
  - Quyết định cần chốt: file tăng từ 1,49 MB lên 2,62 MB. Hiện `upgrade.ts` chỉ có test gọi, chưa vào bundle ứng dụng. Khi nó vào bundle, nên lưu hash SHA-256 thay vì toàn văn. Đổi cấu trúc là quyết định riêng, chưa làm.
- Knowledge giữ một kết nối DB trong suốt `Enable`, gồm các lệnh HTTP tới WeKnora (`lock()` trong `knowledge.go`). Đo được: khi `EnsureTenant` được gọi, pool có 1 kết nối đang giữ (nền 0). Chưa sửa vì khóa phải kéo dài qua các bước HTTP, còn khóa phiên Postgres buộc giữ kết nối đó. Loại: mutex trong tiến trình (không bảo vệ khi chạy nhiều instance); khóa bằng `state` (cần quy tắc hết hạn, mà `workspace_knowledge` không có `updated_at`, nên cần migration). **Quyết định cần chốt:** lease trong DB kèm migration, hay giữ hiện trạng. Knowledge đang tắt trên stack này.
- Pool kết nối giữ 25. Trong cửa sổ diễn tập 05:47 đến 05:48 pool đầy (`acquired_conns=25`). Chưa đo bằng `pg_stat_activity`, nên việc đo tải vẫn còn mở.
- Kiểm trên trình duyệt cho giao diện duyệt chuyên môn chưa làm. Cần ghi một squad vào workspace thử, việc chưa được chấp thuận.

- Ngưỡng dung lượng của Accounting: tầng lõi đo được 146,9 KiB (ngày 2026-11-11); ngưỡng riêng của squad này là 160 KiB (test K8) vì ba skill được nâng lên là chính dây chuyền và chỉ được đọc khi cần. Các squad khác đo được 107,8–127,6 KiB, dưới ngưỡng 140 KiB. **Quyết định: giữ 160 KiB.** Lý do: ba skill nâng lên được đọc khi cần, không nạp ở mọi lần chạy, và test K8 khóa ngưỡng này.
- Nâng cấp squad đã tạo đã có nút trong giao diện: trang Squads, nút **Cập nhật Kit Việt**, chỉ hiện cho quản trị viên khi workspace có squad Kit Việt. Nút gọi `upgradeKitvietWorkspace` (`upgrade.ts`) với mọi mẫu Kit Việt, rồi hiện kết quả từng squad: agent, chỉ dẫn squad và kỹ năng. Văn bản người dùng đã sửa được giữ và được báo tên. Skill Kit Việt trong workspace (`source: kitviet`) được cập nhật về nội dung hiện hành khi nội dung khác; skill do người dùng tự tạo cùng tên thì không bị ghi đè. Nút không có bước xem trước `dryRun`; hàm vẫn hỗ trợ `dryRun` cho test và công cụ.
- `soan-don-tu` ở Hành chính và Pháp lý là hai bản byte giống nhau (F11). **Đã làm.** Bản Pháp lý được đổi tên thành `soan-don-tu-phap-ly` qua `renamedSkills` trong `kitviet-rewrites.json`. Đầu ra có `kv--soan-don-tu-phap-ly` trong `legal.ts` và `squads.ts`; Hành chính giữ `kv--soan-don-tu`. Test `scripts/generate-kitviet-pack.test.mjs` khóa tên này. Nội dung không khác nên tên chỉ là nhãn.
- Tham chiếu `chuan-bi-dam-phan-hoa-giai` trong `soan-bien-ban-thanh-ly-hop-dong` (Pháp lý) không nằm trong tầng lõi hay vai. **Quyết định: giữ cách ghi P0.** Tham chiếu được viết thành "(việc ngoài squad: nhờ lead tạo yêu cầu)" trong `legal.ts`. Skill đích nằm trong `deferredSkills` của Pháp lý và không được đóng gói, nên không có liên kết hỏng. Spec mục 6 để ngoài P0.
- P2 và Teaching Materials (mười một squad, mục 8 đến 8e): chưa chạy thật (S7). Bốn agent nghề giáo dục không được Education Desk ghép nay thuộc Teaching Materials (mục 8, sau Education Desk).
- Gác số liệu của Factory (`soat-so-lieu-chat-luong`) không chặn số không có nguồn. **Quyết định: giữ theo gác số liệu chung P0**, không thêm chặn riêng cho Factory. Lý do: đây là quyết định P0 đã chốt; đổi nó là thay đổi spec, không phải việc của nhánh này.
- Gác khối lượng của Construction chặn "Cần giấy tờ" (quyết định ở spec 10.1, Q12). **Quyết định: giữ.** Gác khối lượng (`construction.ts`) không kết luận QUA khi còn lỗi "Cần giấy tờ". Hệ quả: một bảng thiếu giấy tờ dừng ở QUA, và người làm phải bổ sung giấy tờ rồi chạy lại gác.

### Trạng thái D1–D5 của diễn tập 2026-11-11

Chi tiết và số liệu diễn tập nằm trong [kitviet-drill-report.md](kitviet-drill-report.md). Các thay đổi dưới đây chưa chạy lại trên môi trường thật. Chúng chỉ được kiểm bằng test văn bản (`kitviet-stop-header.test.ts`, `kitviet-verdict-line.test.ts`, `kitviet-lead-self-mention.test.ts`, `kitviet-stage-status.test.ts`) và test Go (đơn vị và DB cô lập, xem ghi chú bên dưới).

| Mục | Trạng thái | Đã đổi | Chưa xác minh trên stack thật |
| --- | --- | --- | --- |
| D1: lead chạy lại sau stop comment của chính nó | Đã sửa cho đường lead được gán theo squad (`89ca0c07e`, trên main). Nguyên nhân thật của 5 ca: comment dừng của lead có mention `mention://agent/<chính lead>`, do văn bản `kv--delivery-core` hướng dẫn cách viết; mention đó enqueue một lần chạy mới của lead. | Đường squad: `CreateComment` không đánh thức leader khi comment là stop của leader lúc request `blocked` hoặc `in_review` (`89ca0c07e`). Văn bản: `kv--delivery-core` không còn cho lead cách viết mention của chính nó; test `kitviet-lead-self-mention.test.ts` (đã thấy đỏ trước khi sửa). Nhánh `X-Task-ID` không khớp đã gỡ khỏi code, vì không tới được qua HTTP. | Chưa diễn tập lại: chưa biết lead có còn tự mention không. Server vẫn cho phép self-mention có chủ đích; xem quyết định dưới. |
| D2: gate để sub-issue ở `in_progress` sau CHẶN | Sửa văn bản (`4ddd14f7f`): gate kết thúc `done` sau VERDICT, kể cả CHẶN. Lệnh đổi trạng thái là lệnh cuối của lần chạy. | Không đổi code server. | Chưa diễn tập. Lệnh cuối chỉ là văn bản. Lần chạy kết thúc trước lệnh đó vẫn để chặng ở `in_progress`. |
| D3: gate không có VERDICT khi bản nháp là văn bản dán | Sửa văn bản: quy tắc nêu rõ văn bản dán trong comment hoặc description không là bản nháp; không có bản nháp đính kèm thì gate ghi `VERDICT: CHẶN - không có bản nháp đính kèm` và kết thúc `done`. | Quyết định: không nhận bản nháp dán (xem dưới). | Test `kitviet-verdict-line.test.ts` chỉ kiểm chữ. Chưa diễn tập. |
| D4: gate bất động sản đặt `blocked` và bỏ sót lời hứa | Sửa văn bản và patch (`4ddd14f7f`): gate kết thúc `done`; mỗi cam kết hoặc lời hứa là một phát hiện riêng. | Patch `soat-tin-dang-bds.json` và các patch gate liên quan. | Chưa diễn tập. |
| D5: stop comment của lead mở bằng kế hoạch | Sửa văn bản: stop là comment riêng, dòng đầu là header. Áp dụng trong `kv-squad-lead/SKILL.md`, kind text của lead, và ghi chú của Marketing và PR. | `kitviet-stop-header.test.ts`. | Chưa diễn tập. |
| Gate ghi đúng một dòng VERDICT | Sửa văn bản: kind text của gate ghi rõ dòng `VERDICT: QUA` hoặc `VERDICT: CHẶN` là dòng đầu, và `kv-delivery-core` dùng cùng một câu. | `kitviet-verdict-line.test.ts`. | Test chỉ kiểm chữ. Không có test nào kiểm gate thật có xuất đúng một dòng. |

Ghi chú về test Go: `TestSquadLeaderSelfWakeSuppressed` (package `dispatch`) pass. Trên DB cô lập của checkout (`multica_multica_51`, lấy từ `.env.worktree`), `TestCreateComment_SquadLeaderStopCommentDoesNotSelfWake` pass ở HEAD và đỏ trên cây trước `89ca0c07e` (`queued 1 leader tasks, want 0`). `TestNotification_TaskFailed*` (package `cmd/server`) pass trên cùng DB. Không chạy test DB trên `multica`: `DATABASE_URL` mặc định của package handler trỏ tới DB đó, và test ghi fixture vào DB đích.

Quyết định (đã chốt, không đổi trong nhánh này):

- **Trạng thái nền tảng khi lần chạy cấp cao nhất của lead thất bại không retry.** Giữ mô hình trạng thái hiện tại: request không đổi trạng thái và không có loại chặn mới. Người yêu cầu được báo qua đường có sẵn. Creator là subscriber từ `issue:created` (`cmd/server/subscriber_listeners.go`). Khi `task:failed` không còn retry, `notifySubscribers` ghi inbox `task_failed` loại `action_required` cho subscriber (`cmd/server/notification_listeners.go`). Test: `TestNotification_TaskFailed_FinalFailureNotifiesCreator`, pass. Lý do: đường thông báo đã đủ; đổi trạng thái request sẽ đổi cách dừng và tiếp tục run, nên cần một quyết định riêng. Hệ quả: request vẫn `todo` cho đến khi người yêu cầu xem inbox. Lý do: đường thông báo đã đủ. Đổi trạng thái request sẽ đổi cách dừng và tiếp tục run, nên cần một quyết định riêng. Hệ quả: request vẫn `todo` cho đến khi người yêu cầu xem inbox.
- **D3: gate không nhận bản nháp dán.** Chỉ bản nháp đính kèm được tính. Gate không có bản nháp đính kèm ghi đúng một dòng `VERDICT: CHẶN - không có bản nháp đính kèm` kèm lý do, rồi kết thúc `done`. Lý do: bản nháp dán vào comment hoặc description không có tệp để kiểm lại. Test: `kitviet-verdict-line.test.ts`.
- **D1: đường thật và cách sửa.** Trên DB diễn tập (`multica_multica_kitviet_332`, chỉ đọc), cả 5 ca D1 có task được enqueue bởi chính comment dừng của lead (`trigger_evidence_kind = 'comment'`), và comment đó có mention `mention://agent/<lead>`. Đã sửa văn bản của `kv--delivery-core`: bỏ cách viết mention lead trong comment dừng, và ghi rõ không mention chính mình. Nguồn văn bản được suy ra từ cách viết trùng với đoạn này trong comment; chưa đối chiếu với prompt của lần chạy. Không sửa server, vì server cố ý cho phép self-mention (ghi trong `resolveMentionedAgentCommentTriggers`). Chặn self-mention cùng issue ở server là phương án khác, bị loại vì đổi hành vi chung cho mọi agent. Rủi ro còn lại: lead vẫn có thể tự mention nếu không làm theo văn bản. Cần diễn tập lại để xác nhận. Nếu tái diễn, chặn ở server là bước tiếp theo, với quyết định riêng.
- **Đường `X-Task-ID` không khớp: đã gỡ.** Không tới được qua HTTP. `Auth` và `DaemonAuth` xóa `X-Agent-ID` và `X-Task-ID` của client. Nhánh `mat_` đặt lại chúng cùng `X-Actor-Source=task_token`, và `resolveActor` trả về `agent` trước khi tới nhánh fallback.

Kiểm trên stack thật còn lại (chưa chạy; cần người dùng cho phép chạy agent thật):

1. D1, chỉ đọc, trên DB của môi trường diễn tập. Kỳ vọng: 0 dòng cho các issue diễn tập.

   ```
   psql "$DRILL_DATABASE_URL" -c "SELECT i.number, t.created_at FROM agent_task_queue t JOIN comment c ON c.id = t.trigger_comment_id JOIN issue i ON i.id = t.issue_id WHERE c.author_type = 'agent' AND c.author_id = t.agent_id;"
   ```

2. D2 và D4: với mỗi sub-issue gate, `multica issue get <id> --output json` phải có `status` là `done`, và dòng đầu của comment cuối là `VERDICT:`.
3. D3: chạy lại Run B của diễn tập với bản nháp đính kèm và với bản nháp dán. Với bản nháp dán, kỳ vọng dòng đầu là `VERDICT: CHẶN - không có bản nháp đính kèm`, sub-issue `done`. Với bản nháp đính kèm, kỳ vọng dòng đầu là `VERDICT: QUA` hoặc `VERDICT: CHẶN` theo nội dung.
4. D5: dòng đầu của mỗi stop comment là header.
5. Test DB cô lập: `cd server && DATABASE_URL='<DATABASE_URL trong .env.worktree>' go test -run 'TestCreateComment_SquadLeaderStopCommentDoesNotSelfWake$' ./internal/handler/ -count=1`.

### Diễn tập lại lần ba (2026-11-12)

Bảng trên ghi "chưa diễn tập" cho D2, D4 và D5. Các mục đó đã được diễn tập lại ngày 2026-11-12 trên stack thử; chi tiết và id nằm trong [kitviet-drill-report.md](kitviet-drill-report.md), mục "Third follow-up".

- **D2 và D4 (gác với bản nháp đính kèm).** Gác đọc bản nháp đính kèm, không đọc bản nháp dán. Kết quả: gác thương mại (KVM-74) và gác bất động sản (KVM-75) đều kết thúc `done`, dòng đầu là `VERDICT: CHẶN`, và nêu đúng các lời hứa đã cài (thuế FTA 0% và "không cần giấy phép"; lợi nhuận 20%/năm và "duyệt vay trong 24 giờ").
- **D5 (plan trước stop).** Quyết định: lead không đăng `## Plan` thành comment riêng trước stop. Nguyên nhân là văn bản cũ cho phép điều đó (`kv-squad-lead/SKILL.md`, mục Templates, và kind text của lead). Văn bản mới: khi stop áp dụng, comment stop là comment duy nhất của bước đó. Test: `kitviet-stop-header.test.ts`, bốn khẳng định đỏ trên văn bản cũ. Kết quả: Office Ops (KVM-76) và Marketing (KVM-77) đều mở bằng header `BLOCKED · stop 1`, không có plan riêng.
- **Registry nâng cấp (`kitviet-previous-texts.ts`).** Điền các text đã phát hành ở `1e2ba5a19`, `1cae0e31e`, `ac8c06003`, cộng text của `f27661e8a` cho các lead và squad mà D5 vừa đổi. Test trong `upgrade.test.ts`: squad có text đã ghi nhận được nâng cấp; text người dùng sửa tay được giữ. Giới hạn: text phát hành trước `1e2ba5a19` chưa có trong registry; phần bổ sung và giới hạn còn lại nằm ở mục 9.
- **Lệnh tạo issue bị huỷ.** Enqueue sau khi tạo issue chạy trên context tách khỏi request, có timeout 30 giây (`IssueService.Create`). Test: `TestCreateAgentIssueQueuesRunWhenRequestCancelsAfterCommit`. Endpoint quick-create cùng lỗi này đã sửa ở `EnqueueQuickCreateTask`; xem báo cáo drill, follow-up thứ tư.
- **KVM-41** (bản mồ côi của lệnh tạo hết giờ) đã được comment và huỷ trong kitviet-mau. Không xoá.
- **Pool kết nối.** Chưa sửa; chưa tìm được nguyên nhân rõ trên đường diễn tập. Bước tiếp theo nằm trong báo cáo drill, mục 5 của lần ba.
