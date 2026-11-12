# Kit Việt P1 notes: foundation, recipe and per-trade data

Status (2026-11-11): the five P1 squads (spec `kitviet-squads.md` section 9.1), the section 7 extension attach and the Revenue
floor are built on `kitviet-pack`, not committed yet. Section 6 lists what is resolved and what is still open. The runbook
(`kitviet-runbook.md`, sections 8 and 8b) describes each squad and the extension. Commands run from the repository root.

## 1. What is in place

- **Vendored trades.** `catalog/_sources/kitviet/upstream/` now holds the five P1 trades (`nhan-su`,
  `bao-chi-truyen-thong`, `ban-hang-online`, `hoc-thuat-nghien-cuu`, `cong-nghe-thong-tin`) at the pinned commit
  `faee4f14cd21672325a7f442b19c7239a5d7f152`. The 30 paths identical in all ten trades stay in `common/`. Every vendored file
  equals the pinned file after LF normalisation (563 checks, see `NOTICE.md`). P2 trades are vendored too (`kitviet-p2-notes.md`, section 1).
- **One config file per squad.** `catalog/_sources/kitviet/squads/base.json` (shared skills and phrases) and one
  `kitviet-squads/<id>.json` per squad. The generator reads the folder; nothing in it names a trade or a squad.
- **Extension configs (spec 7).** `catalog/_sources/kitviet/extensions/<id>.json`: `support`, `people`, `it`.
- **Generator** `scripts/generate-kitviet-pack.mjs` writes one pack per config (`<id>.ts`), `packs.ts` (loader per pack id),
  `extensions.ts` (names only) and `squads.ts` (names only). `packages/core/squads/kitviet-packs.ts` imports only `packs.ts`.
- **Hand-written map** `catalog/_sources/kitviet/extensions.ts`: `KITVIET_EXTENSION_BUILTIN_SKILLS`, target -> bundle names.

## 2. Recipe: add a squad (P1 or any other trade)

You own exactly these files. Nothing else in the pack needs editing.

1. **`catalog/_sources/kitviet/squads/<id>.json`** (`<id>` is the kebab-case pack id, e.g. `talent`, `pr`, `ecommerce`, `office-ops`, `academic`).
   Fields (same shape as `admin.json`):
   - `order`: optional number; squads sort by `order`, then by id.
   - `templateId`: `kv_<x>`, the `SquadTemplate` id. `name`: the squad name. `trade`: a folder under `kitviet-upstream/`.
   - `flow`: `"pipeline"` (default; needs at least one upstream role besides the lead) or `"authored"` (no upstream pipeline; the lead flow is written in the template, and the squad may not attach a `day-chuyen-*` skill).
   - `agents`: map upstream agent name -> `{ "role": "<trade-prefixed role key>", "kind": "kv_<kind>" }`. The lead maps to `lap-ke-hoach-dieu-phoi` with `kv_lead`. A non-lead kind must equal the classifier (section 4). A gate or handover body must end up with exactly one `VERDICT:` line (the generator checks it; the upstream `## Kết luận` section gets the line).
   - `builtinSkills`: core skills by folder name (the pipeline `day-chuyen-*` skill goes here). `leaderSkills`: always `["kv-squad-lead", "kv-company-profile"]`.
   - `roleSkills`: role key -> up to 6 folder names. `promoted`: subset of `builtinSkills`.
   - `deferredSkills`: trade skills the squad ships but does not attach (import by name).
2. **Every skill of the trade must be owned.** A squad ships only what its config lists. A trade skill that no config lists fails the build as an orphan, unless another pack renders it (then it is shared, see section 5).
3. **Run `pnpm generate:kitviet`** until it passes. `node --test scripts/generate-kitviet-pack.test.mjs` runs the same checks as tests. A leftover platform phrase fails with its file and line. Fix it with an exact-match patch in `kitviet-patches/<folder>.json` (`{ "find", "replace" }`, the `find` must occur once) or, when it recurs, a rewrite rule in `kitviet-rewrites.json`. A rule without `"optional": true` must match something in the run.
4. **Template file `packages/core/squads/kitviet-templates/<id>.ts`**, copied from `admin.ts`: `DESCRIPTIONS`, `KV_<X>_ROLE_KEYS`, `KV_<X>_ROLES` built with `kvRole`, and `KV_<X>_TEMPLATE` (`id: "kv_<x>"`, a `floorName` from the default floors, `charter`, `instructions: kvLeadInstructions("<lead role>")`, `builtinSkills: pack.builtinSkills`, `leaderSkills`, `roleSkills: kvRoleSkills(...)`, with `pack = kitvietSquad("kv_<x>")`). For `flow: "authored"`, add the lead's flow to `KV_LEAD_NOTES["kv_<x>"]` in `kitviet-agent-instructions.ts`.
5. **Register** the template in `templates.ts` (`SquadTemplateId`, `SQUAD_TEMPLATES`, `SQUAD_ROLES`). Locales (`modals.json` keys `templates.kv_<x>`, seven languages) and the runbook row follow spec 8.1.
6. **Test** `kitviet-templates/<id>.test.ts`, copied from `admin.test.ts`. The pack-wide tests (`kitviet-skills.test.ts`, `kitviet-packs.test.ts`) cover the new squad with no edit.
7. **Checks:** `pnpm generate:kitviet --check`, `node --test scripts/generate-kitviet-pack.test.mjs`, `cd packages/core && npx vitest run squads`, `npx tsc --noEmit`.

Worked check (done in this foundation, not committed): a sample Talent Acquisition config (nine agents, six skills, three deferred)
generated without error once the optional platform-bullet rules were in place.

## 3. Ownership, closure and cuts (how the packs stay disjoint)

- **One owner per skill.** Each (trade, skill) is listed by one config. A second listing fails the build, e.g.
  `soan-don-tu (hanh-chinh-van-phong) is listed by squad admin and by extension dup`.
- **Extensions** (`kitviet-extensions/<id>.json`): `skills` lists the trade skills the extension attaches to its `targets`.
  The generator adds their **closure**: every trade or shared skill their bodies name with `kv:<key>`, transitively. Agent names win over skill names, as in `resolveReferences`.
- **Cuts.** A closure member that another config renders is not copied. It is listed in the extension's `cut`, and its references
  read "(việc ngoài squad: nhờ lead tạo yêu cầu)". Today: `support` cuts `chuan-bi-dam-phan-b2b` (B2B).
- **Shared skills** (`common/`, the authored `kv-*` skills) are never copied into a pack. An extension attaches them by name.
- **Optional rewrite rules** (`"optional": true`): may match nothing in a run. `codex-bullet` and `claude-code-bullet` are optional.
  The forbidden-text guard still fails on any leftover "Codex" or "Claude Desktop".

## 4. Role-kind classification (the generator checks it)

A non-lead agent's kind is derived from its tools and description: `Write` and `Edit` -> drafter; `Write` only -> handover;
description starting "Gác cổng" -> gate; anything else -> collector. A mapping whose kind differs fails the build. Consequence:
`dieu-phoi-hau-can-su-kien` (Office Ops candidate) is a handover by its tools, so it can only be mapped as `kv_handover`.

## 5. Cross-trade overlaps (identical bytes, one owner)

| Skill | Trades holding identical bytes | Owner |
|---|---|---|
| `soan-don-tu` (+ `references/khung-don-theo-loai.md`) | `phap-ly` (renamed `soan-don-tu-phap-ly`), `hanh-chinh-van-phong`, `nhan-su` | `admin` lists it (`roleSkills.admin_drafter`). Talent Acquisition must not list it: it is shipped by admin already. |
| `viet-giong-gen-z` (+ `references/tu-dien-gen-z.md`) | `marketing`, `ban-hang-online` | `marketing` (`mkt_content`). E-commerce does not list it. |

## 6. Decisions, deviations and open items

Resolved since the foundation:

1. **Company squads attach (was open).** The extension skills attach to the company starter squads after each company apply:
   `kitviet-extension-attach.ts` (`attachKitvietExtensions`) runs from `useApplyCompanyTemplate`. The server is unchanged.
2. **Office Ops (was open).** The nine operations skills left `admin.json`, which now defers nothing. They are listed in
   `office-ops.json` (`officeops_drafter` takes six by role, three are core). Office Ops is `flow: "authored"`.
3. **Revenue floor (was open).** `revenue` is in `COMPANY_FLOOR_KEYS` and `COMPANY_FLOOR_NAMES`; `company_template_data.json` is
   regenerated; the wizard hides a floor no starter card sits on. E-commerce is the first squad on it (Q4).
4. **Section 7 names and the runbook/spec wording (was open).** The runbook has the extension paragraph (8b); the spec and this
   file state the mechanism.
5. **Generator and comments.** `pr` is written `PR` in role names (`PR Lead`). The hand-written `kitviet-extensions.ts` header
   describes the attach design; it used to say it was generated.

Still open:

- **People closure.** `people` pulls `xay-kpi-okr-vi-tri` and `tra-cuu-quy-dinh-lao-dong` (nhan-su skills nobody lists yet). If
  Talent Acquisition lists them, `people` cuts them silently (see `cut`); list them in one place on purpose.
- **Section 7 skill names.** All 14 of 14 exist upstream (checked): `kich-ban-tong-dai-cskh`, `phan-hoi-khieu-nai-cskh`, `phan-tich-nps-csat`,
  `soat-hop-dong-lao-dong`, `soat-bang-cham-cong`, `onboarding-30-60-90`, `quy-trinh-ky-luat-lao-dong`, `danh-gia-thu-viec`,
  `dac-ta-yeu-cau-srs`, `viet-user-story-tieu-chi`, `sinh-test-case-tu-yeu-cau`, `chuyen-yeu-cau-khach-thanh-task`, `release-note-song-ngu`,
  `soat-du-lieu-ca-nhan-phan-mem`. The support extension adds `account-plan-khach-lon` (closure of `phan-tich-nps-csat`).
- **Live runs (S7).** Not run. The three runs of spec 8.2 per squad need the user's go-ahead.
- **Lead notes.** All lead rules of the squads live in `KV_LEAD_NOTES` (`kitviet-agent-instructions.ts`). A squad without an entry
  gets only the generic lead text.
- **Moved out of b2b.** `kich-ban-tong-dai-cskh`, `phan-hoi-khieu-nai-cskh`, `phan-tich-nps-csat` and `account-plan-khach-lon` left
  `b2b`'s deferred list for `support`. Object check against the pre-change pack: every other P0 skill and agent is byte-identical;
  `admin`, `marketing`, `legal`, `accounting`, `common` change only the generated header line.
- **Support bodies** that named `b2b` roles now read "(vai không có trong squad: nhờ lead)": an extension has no roles.
- **Cuts.** `support` cuts `chuan-bi-dam-phan-b2b` (B2B), so that skill reads "(việc ngoài squad: nhờ lead tạo yêu cầu)".

## 7. Per-trade data

Agent kinds are the classifier's output; the squad mapping must match. Common agents are shared by every squad and map to the squad's roles.
Skill descriptions are truncated; read the upstream `SKILL.md` before writing a squad around one.

### nhan-su: Talent Acquisition (P1) and the People & Performance extension

| Upstream agent | Kind | Tools | From |
|---|---|---|---|
| `doc-ho-so-ung-vien` | collector | Read, Grep, Glob | nhan-su |
| `lap-bang-sang-loc-ung-vien` | drafter | Read, Write, Edit, Glob, Grep | nhan-su |
| `soan-tai-lieu-tuyen-dung` | drafter | Read, Write, Edit, Glob, Grep | nhan-su |
| `soat-du-lieu-nhan-su` | gate | Read, Glob, Grep | nhan-su |
| `soat-thien-kien-tuyen-dung` | gate | Read, Glob, Grep | nhan-su |
| `tra-cuu-thi-truong-tuyen-dung` | collector | Read, Grep, Glob, WebSearch, WebFetch | nhan-su |
| `doc-nhieu-tai-lieu` | collector | Read, Grep, Glob | common |
| `kiem-dinh-ban-giao` | handover | Read, Glob, Grep, Write | common |
| `lap-ke-hoach-dieu-phoi` | handover | Read, Glob, Grep, Write | common |
| `nghien-cuu-nguon` | collector | Read, Grep, Glob, WebSearch, WebFetch | common |
| `soat-bao-mat` | gate | Read, Glob, Grep | common |
| `soat-chinh-ta-giong-van` | gate | Read, Glob, Grep | common |
| `soat-so-lieu` | gate | Read, Glob, Grep | common |

| Skill | What it does |
|---|---|
| `bo-cau-hoi-phong-van-star` | Soạn bộ câu hỏi phỏng vấn theo năng lực (STAR) kèm phiếu chấm có thang hành vi cho từng vị trí. Dùng khi  |
| `dang-tin-tuyen-dung-da-kenh` | Chuyển JD thành tin tuyển dụng cho TopCV, VietnamWorks, Facebook, Zalo, LinkedIn - đúng giọng từng kênh.  |
| `danh-gia-thu-viec` | Soạn phiếu đánh giá thử việc/năng lực có bằng chứng, đề xuất ký chính thức hay không, báo kết quả. Dùng k |
| `day-chuyen-tuyen-dung` (pipeline) | Dây chuyền tuyển dụng một vị trí: đọc JD + thư mục CV -> chấm sàng lọc, soạn câu hỏi và thư -> gác cổng t |
| `ke-hoach-dao-tao-nam` | Lập kế hoạch đào tạo năm từ nhu cầu thật: khảo sát nhu cầu, ưu tiên, lịch, ngân sách, cách đo hiệu quả. D |
| `khao-sat-gan-ket-nhan-vien` | Thiết kế khảo sát gắn kết nhân viên ẩn danh và phân tích kết quả: điểm theo nhóm, ý kiến mở, việc nên làm |
| `onboarding-30-60-90` | Lập kế hoạch hội nhập nhân viên mới 30-60-90 ngày: việc trước ngày đầu, tuần đầu, mục tiêu từng mốc, ngườ |
| `phong-van-thoi-viec` | Chuẩn bị và tổng hợp phỏng vấn thôi việc (exit interview): bộ câu hỏi, ghi chép, phân tích lý do nghỉ, ch |
| `quy-trinh-ky-luat-lao-dong` | Lập kế hoạch xử lý kỷ luật lao động đúng trình tự: thu thập chứng cứ, thời hiệu, họp xử lý, biên bản - lu |
| `sang-loc-cv-co-trong-so` | Sàng lọc CV theo tiêu chí có trọng số, chấm có bằng chứng, che thông tin nhạy cảm và chống thiên kiến. Dù |
| `soan-don-tu` | Soạn đơn đúng thể thức: đơn đề nghị, khiếu nại, tố cáo, khởi kiện, xin xác nhận, xin nghỉ việc, trình bày |
| `soat-bang-cham-cong` | Soát bảng chấm công tháng tìm bất thường: thiếu giờ ra/vào, lệch ca, trùng dòng, làm thêm vượt trần, chấm |
| `soat-hop-dong-lao-dong` | Soát hợp đồng lao động, phụ lục, hợp đồng thử việc theo bảng kiểm: đủ nội dung bắt buộc, khớp offer, điều |
| `thu-gui-ung-vien` | Soạn thư mời phỏng vấn, thư mời nhận việc (offer), thư từ chối lịch sự - đúng điều kiện đã duyệt, không h |
| `tra-cuu-quy-dinh-lao-dong` | Tra quy định lao động, BHXH, lương tối thiểu vùng, làm thêm giờ, thử việc trên nguồn chính thống, có tríc |
| `viet-jd-tuyen-dung` | Viết mô tả công việc (JD) rõ việc, rõ tiêu chí, không phân biệt tuổi/giới/quê quán. Dùng khi nói "viết JD |
| `xay-kpi-okr-vi-tri` | Xây bộ KPI hoặc OKR cho vị trí/phòng ban: chỉ số đo được, công thức, nguồn dữ liệu, trọng số, ngưỡng. Dùn |

### bao-chi-truyen-thong: PR & Comms (P1)

| Upstream agent | Kind | Tools | From |
|---|---|---|---|
| `soan-bai-bao-chi` | drafter | Read, Write, Edit, Glob, Grep | bao-chi-truyen-thong |
| `soan-tai-lieu-pr` | drafter | Read, Write, Edit, Glob, Grep | bao-chi-truyen-thong |
| `soat-phap-ly-dao-duc-bao-chi` | gate | Read, Glob, Grep | bao-chi-truyen-thong |
| `soat-su-that-bai-bao` | gate | Read, Glob, Grep | bao-chi-truyen-thong |
| `tra-cuu-nguon-bao-chi` | collector | Read, Grep, Glob, WebSearch, WebFetch | bao-chi-truyen-thong |
| `trich-xuat-bang-ghi-phong-van` | collector | Read, Grep, Glob | bao-chi-truyen-thong |
| `doc-nhieu-tai-lieu` | collector | Read, Grep, Glob | common |
| `kiem-dinh-ban-giao` | handover | Read, Glob, Grep, Write | common |
| `lap-ke-hoach-dieu-phoi` | handover | Read, Glob, Grep, Write | common |
| `nghien-cuu-nguon` | collector | Read, Grep, Glob, WebSearch, WebFetch | common |
| `soat-bao-mat` | gate | Read, Glob, Grep | common |
| `soat-chinh-ta-giong-van` | gate | Read, Glob, Grep | common |
| `soat-so-lieu` | gate | Read, Glob, Grep | common |

| Skill | What it does |
|---|---|
| `bien-tap-tit-sapo` | Biên tập tít và sapo cho bài báo: nhiều phương án đúng sự thật, không giật tít, kèm tít SEO và tít mạng x |
| `bo-cau-hoi-phong-van-bao-chi` | Soạn bộ câu hỏi phỏng vấn báo chí cho chuyên gia, quan chức, nhân chứng, doanh nghiệp: câu chính, câu bám |
| `day-chuyen-bai-bao-kiem-chung` (pipeline) | Dây chuyền ra bài báo đã kiểm chứng: tra nguồn -> viết bài -> gác cổng sự thật và pháp lý-đạo đức -> bàn  |
| `de-cuong-bai-dieu-tra-phong-su` | Lập đề cương bài điều tra, phóng sự, loạt bài dài kỳ: giả thuyết, bằng chứng cần có, nhân vật, lộ trình t |
| `diem-bao-theo-doi-tin-tuc` | Theo dõi tin tức về tổ chức, đối thủ, ngành và lập báo cáo điểm báo: bảng bài đăng, phân loại sắc thái, t |
| `go-bang-phong-van-thanh-bai` | Biến bản gỡ băng phỏng vấn thành bài hỏi-đáp hoặc bài viết, giữ nguyên trích dẫn trực tiếp, đánh dấu mốc  |
| `hoi-dap-chuan-bi-hop-bao` | Chuẩn bị bộ hỏi đáp (Q&A) cho họp báo, phỏng vấn lãnh đạo: câu dễ đoán, câu khó, câu bẫy, câu trả lời dựa |
| `ke-hoach-truyen-thong-su-kien` | Lập kế hoạch truyền thông cho sự kiện, ra mắt, họp báo: mục tiêu đo được, thông điệp, kênh, lịch trước-tr |
| `kich-ban-xu-ly-khung-hoang-truyen-thong` | Lập kịch bản xử lý khủng hoảng truyền thông: phân cấp, giờ vàng, tuyên bố ban đầu, phát ngôn nhất quán, t |
| `kiem-chung-thong-tin-da-nguon` | Kiểm chứng (fact-check) một tin, phát ngôn, số liệu hay ảnh/clip lan truyền bằng nhiều nguồn độc lập, ra  |
| `nghien-cuu-de-tai-bao-chi` | Nghiên cứu đề tài bài báo từ nguồn uy tín theo bậc nguồn của nghề báo, ra hồ sơ tư liệu có trích nguồn và |
| `soat-dao-van-dao-duc-bai-bao` | Soát bài trước khi đăng về đạo văn, trích dẫn thiếu nguồn, nhân vật không xác minh, lộ nguồn tin, xâm phạ |
| `viet-thong-cao-bao-chi` | Soạn thông cáo báo chí doanh nghiệp/tổ chức theo chuẩn tòa soạn: tít tin, sapo 5W1H, trích dẫn lãnh đạo,  |
| `viet-tin-kim-tu-thap-nguoc` | Viết tin, bài báo theo kim tự tháp ngược từ dữ kiện đã kiểm chứng: 5W1H ở đầu, trích dẫn có nguồn, cắt từ |

### ban-hang-online: E-commerce Shop Ops (P1)

| Upstream agent | Kind | Tools | From |
|---|---|---|---|
| `doc-file-xuat-don-san` | collector | Read, Grep, Glob | ban-hang-online |
| `gom-du-lieu-shop` | collector | Read, Grep, Glob | ban-hang-online |
| `soan-listing-san-pham` | drafter | Read, Write, Edit, Glob, Grep | ban-hang-online |
| `soan-tin-cskh-shop` | drafter | Read, Write, Edit, Glob, Grep | ban-hang-online |
| `soat-bien-loi-nhuan-shop` | gate | Read, Glob, Grep | ban-hang-online |
| `soat-tu-cam-quang-cao-san` | gate | Read, Glob, Grep | ban-hang-online |
| `tra-cuu-doi-thu-san` | collector | Read, Grep, Glob, WebSearch, WebFetch | ban-hang-online |
| `doc-nhieu-tai-lieu` | collector | Read, Grep, Glob | common |
| `kiem-dinh-ban-giao` | handover | Read, Glob, Grep, Write | common |
| `lap-ke-hoach-dieu-phoi` | handover | Read, Glob, Grep, Write | common |
| `nghien-cuu-nguon` | collector | Read, Grep, Glob, WebSearch, WebFetch | common |
| `soat-bao-mat` | gate | Read, Glob, Grep | common |
| `soat-chinh-ta-giong-van` | gate | Read, Glob, Grep | common |
| `soat-so-lieu` | gate | Read, Glob, Grep | common |

| Skill | What it does |
|---|---|
| `bao-cao-doanh-so-sku` | Báo cáo doanh số tuần/tháng theo SKU và theo sàn từ file xuất đơn: doanh thu, số đơn, hoàn hủy, lãi gộp,  |
| `cham-soc-sau-ban-shop` | Soạn chuỗi tin chăm sóc sau bán: báo giao hàng, hướng dẫn dùng, hỏi thăm, mời đánh giá đúng luật sàn, nhắ |
| `chot-don-inbox-zalo` | Soạn kịch bản chốt đơn qua inbox Facebook, Zalo, chat sàn: chào, tư vấn, xử lý từ chối, xác nhận đơn và t |
| `day-chuyen-len-san-pham-moi` (pipeline) | Dây chuyền lên sản phẩm mới: gom thông số + giá vốn, xem đối thủ -> soạn tiêu đề, mô tả, giá -> soát từ c |
| `ke-hoach-nhap-hang-ton-kho` | Lập kế hoạch nhập hàng theo SKU: tốc độ bán, số ngày tồn, điểm đặt hàng lại, tồn an toàn, hàng chậm cần x |
| `ke-hoach-noi-dung-kenh-ban-hang` | Lập kế hoạch nội dung tuần cho kênh TikTok Shop, Facebook Page/nhóm: lịch video, live, bài đăng gắn sản p |
| `kich-ban-livestream-chot-don` | Soạn kịch bản livestream bán hàng TikTok, Shopee Live, Facebook: mở màn, giữ người xem, demo, chốt đơn, x |
| `loc-tu-cam-san-tmdt` | Soát tiêu đề, mô tả, caption, kịch bản live tìm từ bị sàn cấm/hạn chế, claim công dụng sai, hàng cần giấy |
| `mo-ta-san-pham-shop` | Viết mô tả sản phẩm cho sàn và fanpage: thông số đúng thật, lợi ích rõ, không claim cấm, có hướng dẫn chọ |
| `nghien-cuu-doi-thu-san` | Nghiên cứu đối thủ trên sàn: khoảng giá, tiêu đề, ưu đãi, phàn nàn trong đánh giá - để định vị sản phẩm.  |
| `phan-tich-huy-hoan-don-san` | Phân tích file xuất đơn từ sàn: tỷ lệ hủy, hoàn/trả theo SKU, sàn, lý do, đơn vị vận chuyển, COD - chỉ ra |
| `thiet-ke-khuyen-mai-san` | Thiết kế voucher, combo, flash sale, mua 2 giảm thêm có tính biên lợi nhuận và lịch mega sale 11.11, 12.1 |
| `tieu-de-seo-san-tmdt` | Viết tiêu đề sản phẩm chuẩn tìm kiếm cho Shopee, TikTok Shop, Lazada: đúng độ dài, từ khóa khách hay gõ,  |
| `tinh-gia-ban-san` | Tính giá bán từ giá vốn + phí sàn + ship + quảng cáo + thuế, ra giá hòa vốn và giá có lãi mục tiêu, công  |
| `tra-loi-danh-gia-shop` | Soạn phản hồi công khai cho đánh giá 1-2 sao trên Shopee, TikTok Shop, Lazada: nhận lỗi đúng mức, mời xử  |
| `viet-giong-gen-z` | Viết hoặc viết lại caption, bài đăng, kịch bản TikTok, trả lời bình luận theo giọng Gen Z có liều lượng,  |
| `xu-ly-doi-tra-khieu-nai-don` | Xử lý yêu cầu trả hàng, hoàn tiền, khiếu nại đơn trên sàn: dựng dòng thời gian, gom bằng chứng, soạn phản |
| `y-tuong-video-ngan-ban-hang` | Lên ý tưởng và kịch bản video ngắn 15-60 giây bán hàng cho TikTok, Shopee Video, Reels: câu móc 3 giây đầ |

### hoc-thuat-nghien-cuu: Academic Evidence Desk (P1)

| Upstream agent | Kind | Tools | From |
|---|---|---|---|
| `dieu-phoi-luan-van` | handover | Read, Glob, Grep, Write | hoc-thuat-nghien-cuu |
| `doc-pdf-bai-bao` | collector | Read, Grep, Glob | hoc-thuat-nghien-cuu |
| `lap-ma-tran-tai-lieu` | drafter | Read, Write, Edit, Glob, Grep | hoc-thuat-nghien-cuu |
| `soan-ban-thao-hoc-thuat` | drafter | Read, Write, Edit, Glob, Grep | hoc-thuat-nghien-cuu |
| `soat-liem-chinh-hoc-thuat` | gate | Read, Glob, Grep | hoc-thuat-nghien-cuu |
| `soat-trich-dan-hoc-thuat` | gate | Read, Glob, Grep | hoc-thuat-nghien-cuu |
| `tim-tai-lieu-hoc-thuat` | collector | Read, Grep, Glob, WebSearch, WebFetch | hoc-thuat-nghien-cuu |
| `doc-nhieu-tai-lieu` | collector | Read, Grep, Glob | common |
| `kiem-dinh-ban-giao` | handover | Read, Glob, Grep, Write | common |
| `lap-ke-hoach-dieu-phoi` | handover | Read, Glob, Grep, Write | common |
| `nghien-cuu-nguon` | collector | Read, Grep, Glob, WebSearch, WebFetch | common |
| `soat-bao-mat` | gate | Read, Glob, Grep | common |
| `soat-chinh-ta-giong-van` | gate | Read, Glob, Grep | common |
| `soat-so-lieu` | gate | Read, Glob, Grep | common |

| Skill | What it does |
|---|---|
| `abstract-song-ngu-hoc-thuat` | Viết tóm tắt (abstract) song ngữ Việt - Anh cho bài báo, luận văn, hội thảo theo cấu trúc chuẩn, đúng số  |
| `chuan-bi-bao-ve-luan-van` | Chuẩn bị buổi bảo vệ khóa luận, luận văn, luận án: dàn slide theo thời gian, lời thuyết trình, câu hỏi hộ |
| `day-chuyen-viet-chuong-luan-van` (pipeline) | Dây chuyền viết một chương luận văn / bài báo: đọc PDF -> soạn bản nháp -> gác cổng trích dẫn và liêm chí |
| `de-cuong-nghien-cuu-khoa-hoc` | Lập đề cương khóa luận, luận văn, đề tài nghiên cứu: lý do, mục tiêu, câu hỏi, giả thuyết, phương pháp, k |
| `de-xuat-xin-tai-tro-nghien-cuu` | Soạn hồ sơ đề xuất xin tài trợ, đề tài nghiên cứu các cấp hoặc quỹ: vấn đề, mục tiêu, nội dung, sản phẩm, |
| `dien-dat-lai-hoc-thuat` | Diễn đạt lại (paraphrase) và tổng hợp ý từ nguồn bằng văn phong học thuật, luôn giữ trích nguồn, không lá |
| `ke-hoach-phan-tich-du-lieu` | Lập kế hoạch phân tích dữ liệu định lượng (Cronbach, EFA, hồi quy, SEM) hoặc định tính (mã hóa chủ đề), đ |
| `khai-bao-dung-ai-hoc-thuat` | Rà việc dùng AI trong luận văn, bài báo theo quy định của trường / tạp chí, ghi nhật ký và soạn đoạn khai |
| `kiem-tra-trich-dan-co-that` | Xác minh từng tài liệu tham khảo có thật: DOI, tạp chí, tác giả, năm, nội dung trích có đúng bài gốc nói  |
| `quan-ly-tai-lieu-tham-khao` | Tổ chức thư viện tài liệu tham khảo: đặt tên file PDF, thư mục, thẻ, ma trận Excel, nhập Zotero/Mendeley, |
| `thiet-ke-bang-hoi-khao-sat` | Thiết kế bảng hỏi khảo sát nghiên cứu: thang đo Likert, mã hóa biến, tránh câu dẫn dắt và câu kép, lời mờ |
| `tom-tat-bai-bao-khoa-hoc` | Đọc và tóm tắt bài báo khoa học theo khung: câu hỏi, phương pháp, mẫu, kết quả, hạn chế, giá trị cho đề t |
| `tong-quan-tai-lieu-hoc-thuat` | Tổng quan tài liệu có hệ thống cho khóa luận, luận văn, bài báo: chọn từ khóa, lọc nguồn theo bậc uy tín, |
| `tra-loi-phan-bien-bai-bao` | Soạn thư trả lời phản biện (response to reviewers) và bảng giải trình chỉnh sửa luận văn sau hội đồng, từ |
| `trich-dan-apa-ieee-harvard` | Định dạng trích dẫn trong bài và danh mục tài liệu tham khảo theo APA 7, IEEE, Harvard hoặc mẫu của trườn |
| `viet-chuong-luan-van` | Dựng dàn ý và hỗ trợ viết từng chương khóa luận, luận văn (mở đầu, tổng quan, phương pháp, kết quả, thảo  |
| `xay-khung-ly-thuyet-nghien-cuu` | Xây khung lý thuyết, mô hình nghiên cứu và giả thuyết từ lý thuyết nền và nghiên cứu trước, mỗi quan hệ c |

### cong-nghe-thong-tin: no squad of its own; the Product Analysis, Planning and Build extension

| Upstream agent | Kind | Tools | From |
|---|---|---|---|
| `doc-log-he-thong` | collector | Read, Grep, Glob | cong-nghe-thong-tin |
| `doc-ma-nguon` | collector | Read, Grep, Glob | cong-nghe-thong-tin |
| `soan-tai-lieu-ky-thuat` | drafter | Read, Write, Edit, Glob, Grep | cong-nghe-thong-tin |
| `soat-bao-mat-ma-nguon` | gate | Read, Glob, Grep | cong-nghe-thong-tin |
| `soat-do-phu-yeu-cau-test` | gate | Read, Glob, Grep | cong-nghe-thong-tin |
| `tra-cuu-tai-lieu-ky-thuat` | collector | Read, Grep, Glob, WebSearch, WebFetch | cong-nghe-thong-tin |
| `viet-test-tu-dong` | drafter | Read, Write, Edit, Glob, Grep | cong-nghe-thong-tin |
| `doc-nhieu-tai-lieu` | collector | Read, Grep, Glob | common |
| `kiem-dinh-ban-giao` | handover | Read, Glob, Grep, Write | common |
| `lap-ke-hoach-dieu-phoi` | handover | Read, Glob, Grep, Write | common |
| `nghien-cuu-nguon` | collector | Read, Grep, Glob, WebSearch, WebFetch | common |
| `soat-bao-mat` | gate | Read, Glob, Grep | common |
| `soat-chinh-ta-giong-van` | gate | Read, Glob, Grep | common |
| `soat-so-lieu` | gate | Read, Glob, Grep | common |

| Skill | What it does |
|---|---|
| `bao-cao-tien-do-du-an-phan-mem` | Báo cáo tiến độ dự án phần mềm cho khách: đã xong, đang làm, trễ và lý do, rủi ro, việc khách cần làm, tr |
| `chuyen-yeu-cau-khach-thanh-task` | Chuyển yêu cầu lộn xộn của khách (Zalo, email, biên bản) thành task rõ: loại, tiêu chí xong, câu hỏi lại, |
| `dac-ta-yeu-cau-srs` | Lập tài liệu đặc tả yêu cầu phần mềm (SRS) khung: phạm vi, chức năng, phi chức năng, tích hợp, ma trận tr |
| `day-chuyen-soat-ma-truoc-phat-hanh` (pipeline) | Dây chuyền soát mã trước phát hành theo Giao thức Kit Việt: đọc mã nguồn -> viết test và báo cáo review - |
| `de-xuat-du-an-cntt` | Lập đề cương tài liệu kỹ thuật hoặc đề xuất dự án CNTT: bối cảnh, mục tiêu đo được, giải pháp, phạm vi, l |
| `helpdesk-ho-tro-nhan-vien` | Soạn hướng dẫn xử lý sự cố IT từng bước cho nhân viên không chuyên (máy in, Wi-Fi, email, VPN, mật khẩu)  |
| `phan-tich-log-nguyen-nhan-goc` | Đọc log lỗi, dựng dòng thời gian, khoanh vùng và tìm nguyên nhân gốc bằng 5 Why, phân biệt bằng chứng với |
| `postmortem-su-co-khong-do-loi` | Viết báo cáo sau sự cố (postmortem) không đổ lỗi: tác động, dòng thời gian, nguyên nhân hệ thống, việc cầ |
| `release-note-song-ngu` | Viết release note song ngữ Việt - Anh cho người dùng từ commit, ticket: tính năng mới, cải tiến, sửa lỗi, |
| `review-code-theo-checklist` | Review code theo checklist: đúng logic, bảo mật (OWASP), hiệu năng, xử lý lỗi, dễ bảo trì; mỗi nhận xét c |
| `runbook-van-hanh-he-thong` | Viết runbook vận hành cho sysadmin: kiểm tra, sao lưu - khôi phục, xử lý cảnh báo, quay lui, có lệnh kiểm |
| `sinh-test-case-tu-yeu-cau` | Sinh bộ test case từ yêu cầu, user story, tiêu chí chấp nhận: phân vùng tương đương, giá trị biên, ngoại  |
| `so-sanh-giai-phap-ky-thuat` | So sánh giải pháp kỹ thuật (thư viện, CSDL, cloud, kiến trúc) từ docs chính thức, RFC, CVE/NVD, có trích  |
| `soat-du-lieu-ca-nhan-phan-mem` | Checklist bảo vệ dữ liệu cá nhân trong phần mềm: kiểm kê dữ liệu, đồng ý, phân quyền, mã hóa, lưu - xóa,  |
| `uoc-luong-ke-hoach-sprint` | Ước lượng công việc phần mềm theo 3 điểm hoặc story point, xếp kế hoạch sprint theo năng lực đội, có rủi  |
| `viet-bug-report-chuan` | Viết báo cáo lỗi (bug report) đủ để dev tái hiện: bước, mong đợi, thực tế, môi trường, mức nghiêm trọng,  |
| `viet-readme-tai-lieu-api` | Viết README dự án và tài liệu API (endpoint, tham số, mã lỗi, ví dụ request/response) từ mã nguồn thật, k |
| `viet-user-story-tieu-chi` | Viết user story chuẩn INVEST kèm tiêu chí chấp nhận Given/When/Then đo được. Dùng khi nói "viết user stor |

### hanh-chinh-van-phong: Office Admin (P0) and Office Ops (P1)

| Upstream agent | Kind | Tools | From |
|---|---|---|---|
| `dieu-phoi-hau-can-su-kien` | handover | Read, Glob, Grep, Write | hanh-chinh-van-phong |
| `gom-ho-so-van-thu` | collector | Read, Grep, Glob | hanh-chinh-van-phong |
| `lap-bang-hanh-chinh` | drafter | Read, Write, Edit, Glob, Grep | hanh-chinh-van-phong |
| `soan-van-ban-hanh-chinh` | drafter | Read, Write, Edit, Glob, Grep | hanh-chinh-van-phong |
| `soat-du-toan-mua-sam` | gate | Read, Glob, Grep | hanh-chinh-van-phong |
| `soat-the-thuc-van-ban` | gate | Read, Glob, Grep | hanh-chinh-van-phong |
| `tra-cuu-the-thuc-van-ban` | collector | Read, Grep, Glob, WebSearch, WebFetch | hanh-chinh-van-phong |
| `doc-nhieu-tai-lieu` | collector | Read, Grep, Glob | common |
| `kiem-dinh-ban-giao` | handover | Read, Glob, Grep, Write | common |
| `lap-ke-hoach-dieu-phoi` | handover | Read, Glob, Grep, Write | common |
| `nghien-cuu-nguon` | collector | Read, Grep, Glob, WebSearch, WebFetch | common |
| `soat-bao-mat` | gate | Read, Glob, Grep | common |
| `soat-chinh-ta-giong-van` | gate | Read, Glob, Grep | common |
| `soat-so-lieu` | gate | Read, Glob, Grep | common |

| Skill | What it does |
|---|---|
| `bao-cao-hanh-chinh-thang` | Tổng hợp báo cáo hành chính tháng: văn bản đến - đi, chi phí hành chính so dự toán, tài sản, hợp đồng dịc |
| `chuan-bi-hop-hdqt` | Chuẩn bị bộ tài liệu họp HĐQT, Hội đồng thành viên: thư mời, chương trình, danh mục tài liệu, phiếu biểu  |
| `dat-phong-hop-hau-can` | Đặt phòng họp và lo hậu cần cuộc họp: chọn phòng theo số người, thiết bị, trực tuyến, nước uống, tài liệu |
| `dat-ve-du-toan-cong-tac` | Lên lịch trình công tác, so phương án vé máy bay/tàu/xe, khách sạn và dự toán chi phí theo quy chế công t |
| `day-chuyen-van-ban-hanh-chinh` (pipeline) | Dây chuyền ra văn bản hành chính 3 khâu: gom căn cứ -> soạn công văn, quyết định, tờ trình -> gác cổng th |
| `luu-tru-ho-so-van-phong` | Dựng quy trình lưu trữ hồ sơ văn phòng: danh mục hồ sơ, mã hồ sơ, cách đặt tên file, thời hạn bảo quản, m |
| `quan-ly-lich-giam-doc` | Sắp lịch tuần cho giám đốc: gom yêu cầu gặp, xếp ưu tiên, phát hiện trùng lịch, chừa thời gian di chuyển, |
| `quan-ly-van-phong-pham` | Quản lý văn phòng phẩm và đồ dùng tiêu hao: định mức theo phòng, sổ nhập - xuất - tồn, đề xuất mua bổ sun |
| `so-cong-van-den-di` | Lập và cập nhật sổ đăng ký công văn đến, công văn đi: số đến, trích yếu, người xử lý, hạn trả lời, việc q |
| `so-sanh-bao-gia-mua-sam` | Lập bảng so sánh báo giá nhà cung cấp (giá, VAT, bảo hành, giao hàng, thanh toán) và đề xuất mua sắm có l |
| `so-tai-san-van-phong` | Lập sổ tài sản, công cụ dụng cụ văn phòng: mã tài sản, người sử dụng, bàn giao, sửa chữa, kiểm kê, đề xuấ |
| `soan-cong-van-hanh-chinh` | Soạn công văn đi đúng thể thức hành chính: số ký hiệu, trích yếu, kính gửi, nơi nhận, người ký. Dùng khi  |
| `soan-don-tu` | Soạn đơn đúng thể thức: đơn đề nghị, khiếu nại, tố cáo, khởi kiện, xin xác nhận, xin nghỉ việc, trình bày |
| `soan-quy-che-noi-bo` | Soạn quy chế, nội quy văn phòng theo chương - điều: phạm vi, trách nhiệm, quy trình, chế tài. Dùng khi nó |
| `soan-quyet-dinh-noi-bo` | Soạn quyết định nội bộ đúng thể thức: căn cứ, các Điều, hiệu lực, người thi hành. Dùng khi nói "soạn quyế |
| `soan-thong-bao-hanh-chinh` | Soạn thông báo hành chính toàn công ty: nghỉ lễ, lịch trực, cắt điện nước, đổi giờ làm, sự kiện. Kèm bản  |
| `soan-to-trinh-de-xuat` | Soạn tờ trình xin phê duyệt gửi giám đốc: lý do, phương án, chi phí, đề nghị cụ thể, ô phê duyệt. Dùng kh |
| `theo-doi-hop-dong-dich-vu-vp` | Theo dõi hợp đồng dịch vụ văn phòng (vệ sinh, bảo vệ, internet, thuê nhà, máy photo): hạn, gia hạn tự độn |
| `tiep-khach-thu-cam-on` | Lên kịch bản tiếp khách tại văn phòng (đón, dẫn, phòng, nước, quà, tiễn) và soạn thư cảm ơn sau buổi gặp. |
| `to-chuc-su-kien-hoi-nghi` | Lập kế hoạch tổ chức sự kiện, hội nghị, year-end party, team building: lịch ngược, checklist, ngân sách d |
