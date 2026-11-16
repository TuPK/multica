# Marketing domain: kế hoạch triển khai

Đặc tả: [spec.md](spec.md) (quyết định D1 đến D16, thiết kế mục 5, kiểm tra mục 7). Đọc spec trước; plan này không nhắc lại lý do.
Bản sửa lần 2: thêm squad cửa trước `marketing` và cài cả bộ (E9, E10, T0.4, T1.7, T1.8, T3.6) để đạt G7: chỉ tạo một squad là dùng được toàn bộ.
Quy ước: đường dẫn kèm "(mới)" sẽ được tạo; còn lại đã tồn tại lúc viết. Mã `E1` đến `E10` là các thay đổi engine ở spec 5.7.
Nguồn upstream: `https://github.com/coreyhaines31/marketingskills` ở commit `1f837447b20bb6f0e72d87b80ece1c221c362c7d` (2026-11-11, v2.14.12). Pack hiện ở `820ad2c1c0ac4cc350158bd7912c753681a7d173`.

## 1. Nguyên tắc

1. Mỗi giai đoạn để lại cây làm việc xanh với các kiểm tra của giai đoạn đó, và là một commit nguyên tử (commit conventional, chỉ khi người dùng yêu cầu). Không mở PR vào `multica-ai/multica` (đẩy vào fork theo thói quen của repo này).
2. Không đổi id, tên squad, tên agent, tên skill đã có. Không có migration DB, không có thay đổi Go dự kiến.
3. Không sửa skill vendor ngoài hai ngoại lệ của spec 5.5 mục 3, mỗi chỗ ghi vào `SOURCES.md`.
4. Mọi chuỗi hiển thị mới có đủ 7 bản dịch (`en`, `zh-Hans`, `zh-Hant`, `ko`, `ja`, `fr`, `es`), không fallback.
5. Cây làm việc hiện có thay đổi chưa commit ở `platform-management/` và `server/`: không đụng, không `git add -A`. Chỉ thêm file đã nêu.
6. Chạy kiểm tra hẹp khi làm, rộng ở giai đoạn 6; báo đúng cái gì chạy, cái gì bỏ qua. `make test` dùng chung CSDL `.env`: kiểm rác còn lại trước khi coi một lỗi là hồi quy.
7. Tài nguyên tạm (clone upstream, script băm) để ở thư mục scratchpad, không để trong repo.

## 2. Phụ thuộc và cách chia việc

```
P0 nền ──► P1 engine ──┬──► P3 domain + squad ──► P4 pack ──► P5 tài liệu ──► P6 kiểm tra
               P2 skill ┘ (P2 chạy song song với P1; P3 cần cả hai, và T3.6 cần T0.4, T1.7, T1.8; P4 cần E5 và P2)
T0.4 (spike điều phối) là cổng của T1.7, T1.8, T3.6: nếu hỏng thì làm phương án thu hẹp (spec 5.11), không xây tiếp.
```

Mỗi giai đoạn là một lát tự đủ, có thể giao cho một phiên riêng (một phiên một lát; không để hai phiên ghi cùng file). P1 và P2 không chạm file chung. P2 chưa ghi `domain` vào skill (trường này cần E1): P3 thêm trường đó bằng codemod một lần.

## 3. P0: nền (kiểm chứng điều spec chưa kiểm chứng)

### T0.1 Ghi nền
Chạy và lưu kết quả vào scratchpad: `pnpm catalog:check`, `pnpm catalog:build`, `node --test scripts/catalog/*.test.mjs`, `pnpm --filter @multica/core exec vitest run catalog squads agent-packs`, `pnpm typecheck`. Mục nào đỏ ở HEAD thì ghi lại là đỏ sẵn, không sửa trong lát này.

### T0.2 Băm nền của đầu ra
Script tạm (scratchpad) sau `catalog:build`: sha256 của `SDLC_BUILTIN_SKILLS` (tên và nội dung), `rev` của từng flow (`CATALOG_FLOWS`), danh sách `{id, category}` của `SQUAD_TEMPLATES`, tên và băm của 19 skill trong bundle `market-strategist.json`, kích thước `packages/core/catalog/generated/skills-builtin.ts` (đo trước: 2.248.538 byte). Chạy hai lần cho cùng kết quả. Đây là chuẩn cho V5.

### T0.3 Clone upstream đã ghim
Clone `marketingskills` vào thư mục riêng ở scratchpad, `git checkout 1f837447b20bb6f0e72d87b80ece1c221c362c7d`, xác nhận `git rev-parse HEAD`. Chạy `grep -rniE "magister|conversionfactory|swipefiles|\?ref=marketingskills"` trên 33 skill đợt 1 và ghi danh sách khối cần gỡ (spec 11, mục 2).

### T0.4 Spike: điều phối giữa squad chạy được không (cổng của G7)
Mục đích: xác nhận các điểm spec 11 chưa kiểm chứng, trước khi xây E9, E10 và squad `marketing`.
1. Đọc và chạy test có sẵn quanh `child_done` và gán cho squad: `server/internal/service/issue_wakeup_squad_test.go`, và test của `issue_wakeup_system.go`; chạy bằng CSDL test theo quy tắc test DB của repo (không dùng CSDL `multica`; kiểm rác còn lại trước khi coi một lỗi là hồi quy).
2. Thử thật nhỏ trên máy chủ dev (cần người dùng đồng ý, theo quyền e2e đã cho; ghi mọi đối tượng tạo ra): hai squad thử có agent do quickstart tạo; lead squad A tạo issue con gán `--assignee-squad B`; B hoàn tất; xác nhận lead A được đánh thức và đọc được kết quả. Làm lại với B là squad có flow (một squad Kit Việt, như `kv_marketing`).
3. Ghi kết quả ba điểm (a), (b), (c) ở spec mục 11. Đạt cả ba: tiếp tục. Không đạt: ghi lý do, chuyển sang phương án thu hẹp của spec 5.11 và đổi G7 thành "chưa đạt" trong báo cáo cuối.

## 4. P1: engine (E1 đến E7, E9, E10)

Thứ tự bên trong: E7, E4, E1, E2, E3, E5, E6. E7 trước vì nó là điều kiện để di chuyển squad Kit Việt.

### T1.1 E7: phép thử Kit Việt theo adapter
- Test đỏ trước (`packages/core/squads/upgrade.test.ts`): với một squad Kit Việt đặt ở collection khác `kitviet`, `hasKitvietSquad` phải đúng và `upgradeKitvietWorkspace` phải xử lý nó; với squad `vertical` cũng vậy (D10); với squad không Kit Việt (ví dụ `content_studio`) phải sai.
- `packages/core/catalog/squads.ts`: mang `adapter` vào `SquadTemplate` (`packages/core/squads/templates.ts`, thêm `adapter?: { type: "kitviet" }`). Thêm `isKitvietTemplate(tpl)`.
- Thay phép thử ở `packages/core/squads/mutations.ts:54` và `packages/core/squads/upgrade.ts:398`.
- Kiểm tra: `pnpm --filter @multica/core exec vitest run squads`. Chưa di chuyển squad nào ở bước này.

### T1.2 E4: kiểm tra skill của squad
- `scripts/catalog/lib/validate.mjs`: với squad không Kit Việt, mỗi tên trong `skills.extra`, `skills.leader`, `skills.byRole.*` phải là skill có thật thuộc skillset `quickstart`; mỗi khóa của `byRole` là vai của squad. Test trong `scripts/catalog/catalog.test.mjs`: tên sai, vai sai.
- Chạy `pnpm catalog:check`. Squad có sẵn vi phạm thì sửa dữ liệu của nó trong cùng task (ghi vào báo cáo; đó là lỗi có thật).

### T1.3 E1, E2: kind `domain`, trường `domain`, `markets`
- `scripts/catalog/lib/discover.mjs`: `domain: "domains"` trong `KIND_DIRS`. `validate.mjs`: id theo quy tắc mặc định; `README.md` bắt buộc; `markets` là mảng `{ id, role }` với `role` thuộc `primary` hoặc `expansion`, đúng một `primary`; `related` trỏ tới template có thật. Trên squad, skill, skillset, pack: `domain` trỏ tới domain có thật; `markets` ⊆ `markets` của domain; squad có `domain: X` phải có `collection: X` và collection `X` tồn tại. Không có kiểm tra i18n cho domain (không nằm trong `KINDS_WITH_UI_TEXT`).
- `scripts/catalog/lib/scaffold.mjs`: `catalog new domain <id>` tạo `template.json` và `README.md` mẫu. `scripts/catalog/cli.mjs`: thêm `domains: "domain"` vào `KIND_BY_DIR` của `baseIdProblems`.
- Test: domain thiếu README; `domain` trỏ vào chỗ không có; squad lệch collection; `markets` ngoài danh sách; hai `primary`.
- Kiểm tra: `node --test scripts/catalog/*.test.mjs`; `pnpm catalog:check` (chưa có domain nào, phải vẫn xanh).

### T1.4 E3: `catalog map`
- `scripts/catalog/lib/map.mjs` (mới): theo spec 5.7 E3. Đầu ra văn bản gọn (bảng) và `--json`. Với squad Kit Việt, đọc `catalog/_sources/kitviet/squads/*.json` theo `templateId`: gộp `builtinSkills`, `roleSkills`, `leaderSkills` và hiển thị là skill `kv:`.
- `scripts/catalog/cli.mjs`: lệnh `map`. `package.json`: script `catalog:map`. Cập nhật dòng `usage`.
- Test: dựng catalog thử có một domain với một squad thường, một squad Kit Việt, một skill, một pack; so đầu ra JSON.

### T1.5 E5: pack tham chiếu skill catalog
- `scripts/catalog/lib/build-packs.mjs`: `buildPackBundle` nhận danh sách mục catalog; phân giải tên: thư mục pack trước, skill catalog cùng id sau; cả hai có là lỗi, không có cả hai là lỗi. Thư mục của skill catalog được liệt kê như `build-skills.mjs` (`skillFiles` trừ `template.json`, `i18n/`).
- `scripts/install-agent-pack.sh` (dòng 60 đến 97): khi `$PACK_DIR/skills/$name` không có, tìm trong `catalog/skills` bằng `find ... -type d -name "<tên, ':' thành '--'>"` có `SKILL.md`; lỗi rõ nếu không thấy. Giữ nguyên mọi hành vi khác.
- Test: pack tham chiếu skill catalog tạo bundle có cùng hình dạng; mơ hồ và thiếu thì lỗi. Chạy `make agent-pack PACK=market-strategist ARGS=--dry-run` trước và sau (chưa đổi pack, chỉ đổi script): danh sách phải giống.

### T1.6 E6: bundle skill tải lười theo skillset
- `scripts/catalog/lib/build-skills.mjs`: skillset có `"chunk": true` bị loại khỏi `SDLC_BUILTIN_SKILLS` và sinh `skills-builtin-<set>.ts` (xuất `<SET>_BUILTIN_SKILLS`); `build.mjs` ghi file mới. `validateSkillset` chấp nhận `chunk`.
- `packages/core/squads/quickstart.ts:526` (`builtinSkillNamed`): tra skillset chứa tên qua `skillsetMembers`; nếu là skillset chunk thì `import()` chunk đó. `SKILLSET_MEMBERS` vẫn là file nhỏ riêng.
- Cập nhật test đang giả định bundle gồm mọi skill quickstart: `packages/core/squads/sdlc-skills.test.ts:61` (đã đúng vì chunk không vào bundle), `packages/core/catalog/squad-invariants.test.ts:13`, `packages/core/squads/templates.test.ts:276`, `packages/core/squads/kitviet-skills.test.ts:368` và `:538`: nơi nào kiểm "tên skill của squad có trong bundle" thì tính thêm tên thuộc skillset chunk.
- Test mới trong `packages/core/squads/quickstart.test.ts`: một tên thuộc skillset chunk được cài đúng nội dung từ chunk; không có skillset chunk nào thì hành vi cũ không đổi (băm T0.2 bằng nhau).
- Chưa có skillset chunk thật ở giai đoạn này; test dùng catalog thử ở `scripts/catalog/catalog.test.mjs` và dữ liệu giả ở vitest.

### T1.7 E9: trường `includes`
- `scripts/catalog/lib/validate.mjs`: `includes` (mảng id squad) chỉ ở squad; mỗi id là squad có thật, cùng `domain`, khác chính nó, và squad đó không có `includes`. Test trong `scripts/catalog/catalog.test.mjs`: id lạ, khác domain, tự tham chiếu, hai tầng.
- `scripts/catalog/lib/build-squads.mjs`, `packages/core/catalog/types.ts` (`CatalogSquad.includes`), `packages/core/catalog/squads.ts`, `packages/core/squads/templates.ts` (`SquadTemplate.includes`): đưa `includes` vào dữ liệu sinh ra và vào template. `catalog map` (T1.4) hiện `includes`.
- Kiểm tra: `node --test scripts/catalog/*.test.mjs`; `pnpm --filter @multica/core exec vitest run catalog squads`. Cần T0.4 đạt.

### T1.8 E10: cài một bộ trong quickstart
- Test đỏ trước (`packages/core/squads/quickstart.test.ts`, client giả): template có `includes` tạo các squad kèm theo thứ tự rồi squad chính; bỏ một id thì không tạo nó; một squad lỗi không dừng squad sau và kết quả gộp báo đúng squad lỗi; chạy lại không nhân đôi agent, squad, skill.
- `packages/core/squads/quickstart.ts`: hàm `runSquadSetQuickstart(client, template, selected, options, onProgress)` gọi `runSquadQuickstart` cho từng squad, trả `{ items: [{ templateId, outcome }] }`. Không đổi chữ ký của `runSquadQuickstart`. `planQuickstartAgents` có biến thể cho một bộ (gộp agent theo tên, đánh dấu dùng lại). `packages/core/squads/mutations.ts`: `useSquadQuickstart` nhận bộ.
- `packages/views/modals/squad-quickstart.tsx`: khi template có `includes`, hiện danh sách "cài kèm" với hộp chọn (mặc định chọn hết, tên và mô tả lấy từ `catalogText("squad", ...)`), danh sách agent theo bộ đã chọn, kết quả theo từng squad. Template không có `includes` giữ nguyên giao diện hiện tại. Khóa dịch mới ở `packages/views/locales/*/modals.json` x7, tuân quy tắc UI Copy (không mô tả thừa). Test component: hiện và ẩn đúng; bỏ chọn cập nhật danh sách agent; kết quả theo squad.
- Kiểm tra: `pnpm --filter @multica/core exec vitest run squads`; `pnpm --filter @multica/views exec vitest run modals/squad-quickstart`; `pnpm typecheck`; `pnpm lint`. Cần T1.7.

Kiểm tra cuối P1: `node --test scripts/catalog/*.test.mjs`, `pnpm catalog:check`, `pnpm catalog:build`, `pnpm --filter @multica/core exec vitest run catalog squads agent-packs`, `pnpm typecheck`; băm T0.2 bằng nhau (không đổi hành vi).

## 5. P2: nhập skill (chạy song song với P1)

### T2.1 Khung
`catalog/skills/marketing/{foundation,strategy,research,search,conversion,paid,lifecycle,measurement}/` (mới), `catalog/skills/marketing/licenses/` (mới): chép `marketingskills.LICENSE`, `pm-skills.LICENSE`, `wondelai-skills.LICENSE`, `idea-validation-agents.LICENSE` từ `catalog/packs/market-strategist/licenses/`. `catalog/skillsets/marketing/template.json` (mới): `kind: skillset`, `id: marketing`, `version: 1.0.0`, `target: quickstart`, `chunk: true`. (`chunk` cần T1.6 để có hiệu lực; trước đó validator chưa biết trường này, nên thêm trường ở T3.1 cùng lúc kích hoạt, hoặc làm T2.1 sau T1.6 nếu chạy tuần tự.)

### T2.2 Nhập 33 skill của marketingskills
Cho mỗi skill ở spec 5.5, từ clone đã ghim (T0.3):
1. Chép `SKILL.md` và các file `references/`, `assets/` dạng văn bản; bỏ `evals/`, mọi `*.py|*.mjs|*.js|*.sh|*.html|*.csv` không phải văn bản tham chiếu mà skill dựa vào (liệt kê từng file bị bỏ trong `SOURCES.md`), không chép `tools/`.
2. Ghi `template.json` với `sets: ["marketing"]`, `source: { type: "vendored", repo: "https://github.com/coreyhaines31/marketingskills", ref: "1f837447b20bb6f0e72d87b80ece1c221c362c7d", license: "MIT" }`, `version` bằng `metadata.version` của upstream (khớp `VERSIONS.md` upstream).
3. Áp hai ngoại lệ: gỡ khối quảng cáo theo danh sách T0.3 (đã biết `ad-creative/SKILL.md`, đoạn "Skip the plumbing" cùng ghi chú disclosure và chỉ dẫn `tools/SETUP.md`); gỡ liên kết trỏ vào `tools/`. Mỗi chỗ ghi tên file, dòng, và lý do vào "Local changes" của `SOURCES.md`.
4. Kiểm tra skill dùng được không cần file đã bỏ: `grep` các liên kết tương đối trong `SKILL.md` và `references/` trỏ vào file không còn; sửa bằng cách ghi vào "Local changes" (không im lặng).

Nhóm đặt skill: theo spec 5.2. Với `product-marketing`, `customer-research`, `competitor-profiling`, `marketing-ideas` (đã có trong pack ở commit cũ): bản mới thay bản trong pack ở T4.1.

### T2.3 Chuyển 15 skill của pack
Từ `catalog/packs/market-strategist/skills/`: `market-sizing`, `user-segmentation`, `ideal-customer-profile`, `beachhead-segment`, `growth-loops`, `gtm-motions`, `value-proposition`, `prioritize-assumptions` (phuryn/pm-skills, ref `2a12295a7717569baa7be181b4cfebe3ca38a6bb`), `mom-test`, `jobs-to-be-done`, `obviously-awesome` (wondelai/skills, ref `c172996495bed0fcd26896a9416b2093fd7073f0`), `distribution-analysis`, `cac-modeler` (MaxKmet/idea-validation-agents, ref `3a4c800a3022e73e9997533b603c79b60ee7597a`), `outlet-finder`, `evidence-discipline` (tự viết). Dùng `mv`, không gõ lại; thêm `template.json` (`source` lấy từ `packs/market-strategist/SOURCES.md`). Chưa xóa gì khỏi pack ở giai đoạn này nếu P1 chưa xong E5: nếu chạy P2 trước E5 thì **sao chép** rồi xóa bản trong pack ở T4.1; còn không thì `mv`.

### T2.4 Viết ba skill tự viết (`foundation/`)
`mkt-context`, `mkt-market-vn`, `mkt-market-intl`, theo dàn ý spec 5.6, `source: { type: "authored" }`, `sets: ["marketing"]`, `markets` lúc có E2 (thêm ở T3.1). Mô tả (`description` trong frontmatter) nói khi nào nạp, dưới 500 ký tự. Nội dung pháp lý chỉ gồm điều đọc được từ nguồn chính thức có ngày; điều chưa kiểm được ghi `[cần bổ sung: nguồn]`. Dùng `evidence-discipline` cho quy tắc nguồn. Đưa cho người phụ trách pháp chế xem trước khi coi `mkt-market-*` là ổn định (spec R11); trong lúc chờ, skill ở `status: beta` ở manifest.

### T2.5 `SOURCES.md`
`catalog/skills/marketing/SOURCES.md` (mới): bảng skill theo nguồn (như `packs/market-strategist/SOURCES.md`), "Local changes" chi tiết, các file bị bỏ, quy trình nâng cấp (spec 5.5 mục 5).

Kiểm tra: `pnpm catalog:check` (cảnh báo skill mồ côi không còn vì đã có skillset); `grep -rniE "magister|\?ref=marketingskills" catalog/skills/marketing` rỗng; đếm 51 thư mục skill.

## 6. P3: domain, squad, collection (sau P1 và P2)

### T3.1 Domain và gán trường
- `catalog/domains/marketing/template.json` và `README.md` (mới; README viết ở P5). Thêm `domain: "marketing"` vào `template.json` của 51 skill, skillset `marketing`, pack `market-strategist`, và `markets` theo spec (`mkt-market-vn`: `["vn"]`, `mkt-market-intl`: `["intl"]`, pack `["vn","intl"]`; skill phương pháp không khai `markets`). Codemod một lần trong scratchpad; không để lại trong repo.
- Kiểm tra: `pnpm catalog:check`.

### T3.2 Collection và chuyển squad
- `catalog/collections/marketing/{template.json,i18n/*.json}` (mới): `for: squad`, `order: 15`, title "Marketing" dịch đủ 7 ngôn ngữ. Thứ tự squad trong nhóm: `marketing` 5, `kv_marketing` 10, `content_studio` 20, `growth_lab` 30, `kv_pr` 40.
- `catalog/squads/{kv_marketing,kv_pr,content_studio}/template.json`: `collection: "marketing"`, `domain: "marketing"`, `markets` (`["vn"]`, `["vn"]`, `["vn","intl"]`), `order` (10, 40, 20), `version` 1.0.1; mô tả (`i18n/*.json`) nói thêm rằng squad này cũng được cài và điều phối bởi squad Marketing. Cập nhật `i18n/*.json` của ba squad nếu câu mô tả hiện có sai so với bảng spec 5.3 (đọc từng câu trước, chỉ sửa khi sai).
- Kiểm tra: `pnpm catalog:check`; `pnpm catalog:build`; `pnpm --filter @multica/core exec vitest run catalog squads`; băm T0.2: `rev` của flow và `SDLC_BUILTIN_SKILLS` bằng nhau; `SQUAD_TEMPLATES` chỉ khác `category` ở ba squad.

### T3.3 Squad `growth_lab`
- `pnpm catalog:new squad growth_lab --collection marketing`, rồi điền theo spec 5.4: 7 vai, `agentTemplate`, tên agent có tiền tố "Growth Lab", `skills: { sets: [], extra: [...5 chung], leader: [...], byRole: {...} }`, `routines` (một mục "Weekly growth review", `0 9 * * 1`, `issueTitleTemplate`), `charter`, `domain`, `markets: ["vn","intl"]`, `floorName: "Launch"`, `order: 30`, `workflow: null`.
- `instructions.md` (lead) và `roles/<vai>.md` x7: mỗi file nói việc của vai, nhắc nạp `mkt-context` và skill thị trường theo thị trường của brief, quy tắc `MISSING`, và ranh giới "không gửi, đăng, chi tiền, sửa hệ thống đang chạy". `roles/gl_checker.md` theo khuôn `catalog/squads/content_studio/roles/cs_market_gate.md` (dòng `VERDICT: PASS|FAIL`) và `catalog/squads/sales_loop/roles/sales_checker.md`. Mỗi file ngắn (khoảng 1 KB như các vai của `content_studio`, hoặc dài hơn nếu cần chỉ dẫn).
- `i18n/*.json` x7 (title "Growth Lab", description một đến hai câu).
- Kiểm tra: `pnpm catalog:check` (E4 kiểm mọi tên skill); `pnpm catalog:build`; test `squad-invariants` (đã duyệt mọi squad).

### T3.6 Squad cửa trước `marketing` (cần T0.4, T1.7, T1.8 và T3.3)
- `pnpm catalog:new squad marketing --collection marketing`, rồi điền theo spec 5.11: một vai `mktg_chief` (agent "Marketing Chief", mẫu `growth_lead`), `includes: ["kv_marketing","content_studio","growth_lab","kv_pr"]`, `skills: { sets: [], extra: [10 skill của spec 5.11] }`, `routines` (một mục "Weekly marketing report", `0 8 * * 1`), `charter`, `domain`, `markets: ["vn","intl"]`, `floorName: "Launch"`, `workflow: null`, `order: 5`.
- `instructions.md` (lead) viết theo spec 5.11: sáu bước làm việc, bảng điều phối năm dòng, lệnh `multica issue create --parent <yêu cầu> --assignee-squad "<tên squad>"`, ranh giới (không viết sản phẩm cuối, không thay cổng, không gửi, đăng, chi tiền), tối đa hai vòng giao lại, cách xử lý issue con đang chờ người (R15), và việc nói với người yêu cầu khi squad cần không có trong workspace. Tên squad trong bảng lấy đúng `squad.name` của catalog (V10 kiểm). `roles/mktg_chief.md` ngắn, trỏ về `instructions.md` và nhắc nạp skill thị trường.
- `i18n/*.json` x7: title "Marketing", description một đến hai câu nói rõ đây là squad duy nhất cần tạo.
- Kiểm tra: `pnpm catalog:check`; `pnpm catalog:build`; test V9, V10 (T3.4).

### T3.4 Test của domain
`packages/core/catalog/marketing-domain.test.ts` (mới): V4, V8, V9 (phần dữ liệu) và V10 của spec 7: `ref` 40 ký tự; không `evals/`, script, `tools/`; không chuỗi quảng cáo; số skill 51 trong 8 nhóm; mọi skill của `growth_lab` tồn tại và mỗi trong 33 skill đợt 1 gắn cho ít nhất một vai; tên agent của mọi squad duy nhất trong catalog (R6); lead nêu thị trường mặc định `vn`; mỗi vai có file hướng dẫn nhắc skill thị trường; không file nào khuyến khích gửi, đăng hay chi tiền.

### T3.5 Quickstart thử
Trong `packages/core/squads/quickstart.test.ts` (hoặc file test mới cạnh nó): với client giả, cài `growth_lab`: 7 agent, một squad, năm skill chung gắn cho squad, skill theo vai gắn đúng agent, chạy lần hai không nhân đôi (V6). Dùng đúng mẫu test quickstart có sẵn.

## 7. P4: pack

### T4.1 Pack tham chiếu catalog
- `catalog/packs/market-strategist/pack.json`: giữ danh sách 19 tên. Xóa `skills/` và `licenses/` khỏi thư mục pack (đã ở `catalog/skills/marketing/`); xóa 4 skill marketingskills bản cũ (bản mới đã ở catalog). `SOURCES.md` của pack: thay bằng một dòng trỏ tới `catalog/skills/marketing/SOURCES.md`. `README.md` của pack: sửa mô tả (không còn trỏ `packages/core/agent-packs/bundles/...`, đã stale) và trỏ về domain.
- `domain` và `markets` đã gán ở T3.1.
- Kiểm tra: `pnpm catalog:build`; bundle `market-strategist.json` có đúng 19 tên skill; 15 skill chuyển có nội dung giống hệt byte với bản cũ (so sánh băm T0.2); 4 skill marketingskills mang nội dung của `1f83744`; `pnpm --filter @multica/core exec vitest run agent-packs`; `pnpm --filter @multica/views exec vitest run modals/agent-pack-install`; `make agent-pack PACK=market-strategist ARGS=--dry-run` liệt kê 19 skill.

## 8. P5: tài liệu và hướng dẫn

### T5.1 README của domain
`catalog/domains/marketing/README.md`: mục đích; "Bắt đầu" (chọn squad Marketing trong bộ chọn squad, một lần là đủ; gửi yêu cầu cho nó); bảng squad (spec 5.3) kèm "khi nào dùng cái nào" và quy tắc phân vai Marketing Studio, Content Studio, Growth Lab; bảng skill theo nhóm và thị trường; lệnh `pnpm catalog:map marketing`; cách mở rộng (spec 5.10); cách nâng cấp skill vendor; việc chưa làm (spec 10). Tiếng Việt như các tài liệu `docs/engineering/`. Không chép dữ liệu có thể lệch: nói "chạy `catalog:map`" cho danh sách đầy đủ.

### T5.2 Cập nhật quy tắc
- `catalog/README.md`: thêm dòng `domains/<id>/` vào bảng "What is here"; mục "Everyday work" thêm `pnpm catalog:map`; mục Rules thêm: thành viên khai `domain`; squad của domain cùng collection; skill vendor của domain ghim commit.
- `AGENTS.md`, hàng `catalog/` của bảng Sharing Rules: nhắc "domains" cùng các loại khác (một mệnh đề), vì quy ước của `AGENTS.md` là sửa trong cùng thay đổi.
- `docs/specs/template-registry/spec.md`: thêm một dòng ở mục 6.1 hoặc 11 trỏ tới spec này (kind `domain` mở rộng bố cục); không viết lại.

## 9. P6: kiểm tra toàn bộ và diễn tập

### T6.1 Kiểm tra rộng
`pnpm catalog:check --base HEAD` (id không biến mất), `pnpm catalog:build`, `node --test scripts/catalog/*.test.mjs`, `pnpm typecheck`, `pnpm lint`, `pnpm test`, và `make test` (cần CSDL; theo quy tắc test DB của repo: chỉ dùng CSDL test, không dùng `multica`). Đối chiếu băm T0.2 lần cuối. Ghi kích thước `skills-builtin.ts` (phải bằng nền) và kích thước chunk marketing.

### T6.2 Diễn tập cả bộ (cần người dùng đồng ý trước khi chạy trên máy chủ thật; kiểm tiêu chí A7)
Theo quyền đã cho cho phép e2e trên máy chủ dev (workspace `tupk`): chọn **chỉ** squad `marketing` trong quickstart, xác nhận 5 squad và 36 agent được tạo, rồi giao cho squad `marketing` các yêu cầu thật: một chuỗi nhiều squad (soạn chiến dịch tiếng Việt ở Marketing Studio rồi đưa sang Content Studio, kết thúc ở proposal đăng; không đăng thật), một yêu cầu Growth Lab. Kiểm: lead nêu thị trường, giao đúng squad, được đánh thức khi issue con đóng, đọc kết quả, nối chuỗi, không tự viết sản phẩm cuối, flow của Marketing Studio vẫn chạy và dừng đúng chỗ (cổng thông điệp); sau đó giao riêng cho Growth Lab ba yêu cầu thật (kiểm toán SEO một URL công khai, rà soát một trang landing, kế hoạch thử nghiệm email), một bằng tiếng Việt và một quốc tế, và đọc kết quả: có `MISSING` đúng chỗ không có dữ liệu, thị trường nêu ở dòng đầu, checker ra `VERDICT`, không có hành động ra ngoài workspace. Ghi mọi đối tượng đã tạo (agent, squad, skill, issue) vào báo cáo. Nếu không được chạy, nói rõ phần này chưa kiểm chứng; không coi là đã xong.

### T6.3 Thử mở rộng (A6)
Thêm thị trường thử `zz` (một skill `mkt-market-zz`, khai báo trong domain, thêm vào `markets` của một squad), chạy `catalog:check` và `catalog:map`, xác nhận `git status` chỉ có file dưới `catalog/`, rồi xóa.

### T6.4 Báo cáo
Ghi vào `spec.md` mục "Hiện trạng triển khai" (thêm theo khuôn của template-registry): cái đã làm, cái khác thiết kế, cái chưa làm, lệnh đã chạy và kết quả, cùng D10 (hành vi làm mới squad `vertical`) để người sở hữu quyết định giữ hay quay lại.

## 10. Việc để sau (không thuộc plan này)

Đợt 2 skill cho Content Studio quốc tế và PR; flow cho `growth_lab` và cho chính squad `marketing`; một squad thật duy nhất thay cửa trước và bốn squad chuyên môn (cần bộ sinh Kit Việt và flow cho squad mới); hội tụ `kv_marketing` và `content_studio`; domain Sales; `conversion-tracking`; kết nối dữ liệu. Chi tiết và lý do ở spec mục 10.

## 11. Bảng trạng thái

| Giai đoạn | Nội dung | Trạng thái |
| --- | --- | --- |
| P0 | Nền, băm, clone ghim, spike điều phối (T0.4) | xong; phần máy chủ dev của T0.4 và T6.2 chờ người dùng đồng ý |
| P1 | Engine E1 đến E7, E9, E10 | xong (E10 có test khung và test giao diện; cài kèm thật chưa chạy trên máy chủ) |
| P2 | Nhập 33 + 15 skill, 3 skill tự viết, `SOURCES.md` | xong (51 skill; mkt-market-* ở beta chờ pháp chế) |
| P3 | Domain, collection, `growth_lab`, squad `marketing`, test domain | xong |
| P4 | Pack tham chiếu catalog | xong (19 tên; 15 file giống hệt byte) |
| P5 | README domain, quy tắc, tài liệu | xong |
| P6 | Kiểm tra rộng, diễn tập, báo cáo | kiểm tra xong; thử mở rộng xong; diễn tập trên máy chủ chưa chạy (cần đồng ý) |
