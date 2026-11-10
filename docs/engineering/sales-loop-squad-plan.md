# Sales Loop squad: implementation plan

Kế hoạch triển khai cho [sales-loop-squad.md](sales-loop-squad.md). Đọc spec trước; các mục "4.x", "7.x", "8.x" dưới đây trỏ về spec.

## Trạng thái triển khai (2026-11-10)

Đã làm, kiểm bằng test: giai đoạn G (kết luận ghi ở spec §15), phần lớn giai đoạn 0, phần server và giao diện danh sách của giai đoạn 1. Các mục còn lại ghi rõ lý do dưới bảng.

| Mục | Trạng thái | Ghi chú |
| --- | --- | --- |
| G1–G8 | Xong | Kết luận ở spec §15; G8 là spike có số đo (phụ lục A) |
| G9 | Chưa làm | Luật sư xem spec mục 10; chặn việc bật bậc 6 cho dữ liệu có người Việt |
| T0.1 `routines[]` | Xong | Ba routine của Sales Loop |
| T0.2 kind, base, policy | Xong | 13 kind `sales_*` (có `sales_report`), base `sales`, policy seed phía server; test DB |
| T0.3 skill `sales-*` | Xong, lệch nhỏ | 7 skill. `reference.md` chỉ có ở `sales-delivery-core`, `sales-scouting`, `sales-writing`; các skill còn lại giữ nội dung trong `SKILL.md` |
| T0.4 role, instruction, template | Xong | 5 agent, `SALES_SKILL_NAMES`, bộ lọc SDLC loại skill sales |
| T0.5 thẻ quickstart, i18n | Xong | Bảy ngôn ngữ; parity test qua |
| T0.6 nâng cấp | Xong | Dùng `upgradeSdlcSquad` có sẵn với `--template sales_loop`; test |
| T0.7 schema recipe | Xong | Hai lớp: bản nháp (hình dạng) và bản chạy được (đủ trường) |
| T0.8 adapter, SSRF, robots | Xong | rss, json_api, html_jsonld, html_list, csv_drop; chặn địa chỉ nội bộ kể cả qua chuyển hướng và DNS; fixture là phản hồi thật đã cắt |
| T0.9 CLI probe và collect | Xong, lệch nhỏ | `collect` nhận `--recipe-file` (không phải `--source <id>`): Scout lấy recipe từ tài liệu rồi truyền file; `--source-id` chỉ để gắn nhãn |
| T0.10 nguồn starter | Xong | 38 nguồn; 15 dòng mạng xã hội (spec có 16, tính cả tìm kiếm web, mục này không dùng); quickstart ghi vào KB, khóa idempotent |
| T0.11 route trong skill Lead | Xong | Route A–G |
| T0.12 tài liệu và diễn tập | Một phần | Runbook Crawl4AI viết xong (REST chưa kiểm với container). **Diễn tập R1–R11 trên agent thật chưa chạy**: cần bạn cho phép (tốn quota) |
| T1.1 migration | Xong | 922–926, mỗi chỉ mục một file `CONCURRENTLY`, có down |
| T1.2 SQL và sqlc | Xong | Lọc theo thuộc tính, lọc đến hạn, lịch sử. Sắp xếp chỉ theo thời gian tạo |
| T1.3 service | Một phần | Kiểm theo `fields`, cập nhật thuộc tính, lịch sử, luật stage, điền thuộc tính từ mục Record khi viết. **Chưa có backfill** cho tài liệu P0 đã viết: cần đọc nội dung từ WeKnora; hiện chưa có tài liệu P0 nào được ghi ở môi trường thật |
| T1.4 API | Xong, lệch | `GET /api/knowledge/collections` thay vì lọc trên `GET /documents`. Intake squad tạo issue giao cho **squad** (Lead định tuyến route E/F), không giao thẳng cho Scout |
| T1.5 CLI | Xong | `add --field`, `update --field/--unset`, `list --kind --field --due` |
| T1.6 core TS | Xong | Schema zod có fallback; query và mutation có `wsId` |
| T1.7 Collections | Xong, thiếu test thành phần | Bảng, bảng kanban có kéo thả, khung chi tiết sửa thuộc tính, năm view lưu sẵn; có test hàm thuần |
| T1.8 Source Studio | Một phần | Có: danh sách nguồn với trạng thái, ô thêm nguồn, ô chia sẻ, bấm tên nguồn để xem và sửa thuộc tính (cùng hộp với Collections), test thành phần. **Chưa có**: nội dung recipe trong hộp chi tiết (đọc nội dung cần WeKnora, chưa có đường đọc một tài liệu), bước xem mẫu 10 tín hiệu, nút kích hoạt/thử bậc khác, mục connector, ngân sách |
| T1.9 ngân sách | Chưa làm | Trần trong recipe có. Chưa có trần theo squad và theo nguồn, chưa có số đã dùng. Hoãn đến khi có bộ chạy bậc 6: hiện không có adapter nhà cung cấp nào, và bậc 6 chờ G9 (luật sư), nên chưa có chi phí để đếm |
| T1.10 Web engine | Một phần | Spike và runbook xong. Có trong code: client `Crawl4AI` chỉ nghe loopback, token từ môi trường, `fetch.engine`/`render: browser` đi qua engine có kiểm địa chỉ trước, `probe` báo `web_engine.status`. **Hình dạng REST `/health` và `/html` chưa kiểm với container**: chỉ kiểm bằng máy chủ giả trong test. Ngân sách trang và xếp hàng nguồn dùng engine chưa có |
| T1.11 kiểm chứng | Một phần | Test Go và TS cho phần đã làm; **chưa có Playwright E2E** |
| T2.1 Gmail | Một phần | Chỉ có hướng dẫn trong skill: nháp Gmail khi đã kết nối và `approved`, không gửi. Chưa kiểm với Gmail thật |
| T2.2 Lịch | Chưa làm | Chưa có trigger cho sự kiện lịch |
| T2.3 chia sẻ nhanh | Chưa làm | Không có bookmarklet hay tiện ích. Chia sẻ từ di động: không (G7) |
| T3 handoff khi won | Một phần | Dùng `multica squad handoff <request-id>` có sẵn, theo route G trong skill Lead. Không có mã mới; chưa có test ca bán hàng |

Kiểm chứng đã chạy: `go test` cho `internal/salescollect`, `internal/service` (phần knowledge), `internal/handler` (phần knowledge và squad intake), `cmd/multica` (phần sales và field); `pnpm --filter @multica/core` (squads, knowledge) và `@multica/views` (knowledge, squads, modals, locales); `tsc --noEmit` cho core và views. `pnpm lint` còn lỗi ở các file script có sẵn trong skill `superpowers--*` (không do thay đổi này).

Quy ước thực hiện (theo AGENTS.md):

- Commit nhỏ, mỗi task một commit, kiểu conventional (`feat(squads): ...`). Kết thúc commit bằng dòng `Co-Authored-By` theo hướng dẫn của phiên làm việc.
- PR chỉ mở vào `--repo TuPK/multica`, không bao giờ vào `multica-ai/multica`.
- Sau mỗi task chạy kiểm tra hẹp nhất có ích; cuối mỗi giai đoạn chạy bộ rộng hơn (`pnpm typecheck`, `pnpm lint`, `pnpm test`, `make test` cho phần Go). Báo cáo đúng những gì đã chạy, kể cả cái bỏ qua.
- Skill sửa trong `packages/core/squads/sdlc-skills/`, rồi `pnpm generate:sdlc-skills`. Không sửa tay `sdlc-skills-data.ts`.
- Migration: không khóa ngoại, không cascade; mỗi chỉ mục một file migration `CREATE INDEX CONCURRENTLY`; sau khi sửa SQL chạy `make sqlc`.
- Phản hồi API mới đi qua zod và `parseWithFallback`, có test phản hồi lỗi; khóa query có `wsId`; chuỗi UI qua i18n đủ bảy ngôn ngữ (en, zh-Hans, zh-Hant, fr, es, ko, ja).
- Test mặc định **không** gọi mạng, không gọi nhà cung cấp trả phí, không chạy agent CLI thật. Phần cần chạy thật nằm ở các bước "diễn tập" và chỉ chạy khi người dùng cho phép.
- Chỉ stage đường dẫn của task mình. Cây làm việc có thể có thay đổi của phiên khác; không `git add -A`.

## Nền đã có và ranh giới

BMAD Planning đã vào `main` (`02fe41673` template, `3d829d150` workflow cho dự án của squad, `a954a7b6e` handoff giữa squad, `524a550a8` giao diện handoff). Sales Loop đi cùng đường:
template trong `packages/core/squads/templates.ts`, skill trong `sdlc-skills/` với tiền tố `sales-`, `SALES_SKILL_NAMES` tường minh, bộ lọc `SDLC_SKILL_NAMES` mở rộng, instruction ngắn trong file riêng.
`SquadTemplate.routine` vẫn là mục đơn nên `routines[]` là việc đầu tiên. Handoff khi `won` (P3) dùng lại cơ chế đã có.

## Tổng quan giai đoạn

| Giai đoạn | Nội dung | Phụ thuộc | Đụng server | Kích cỡ | Rủi ro |
| --- | --- | --- | --- | --- | --- |
| G | Các cổng kiểm trước khi viết mã: trả lời các câu hỏi mở của spec bằng đọc mã hoặc một phép thử nhỏ | không | không | nhỏ | thấp |
| 0 | Template, skill, seed kind, bộ thu thập CLI, nguồn starter, onboarding qua issue | G | có, nhỏ (seed kind, CLI) | lớn | trung bình |
| 1 | Thuộc tính tài liệu KB, Collections, Source Studio, `template_key` | 0 | có (migration, API, giao diện) | rất lớn | cao |
| 2 | Connector Gmail và Lịch, chia sẻ nhanh (bookmarklet, di động) | 1 | có, qua Composio | trung bình | trung bình |
| 3 | Handoff khi deal `won` sang squad giao hàng | 1 | tái dùng | nhỏ | thấp |

Thứ tự: G → 0 → 1 → (2, 3 song song được). Squad dùng được sau Giai đoạn 0 (xem qua tài liệu snapshot); "xem trực quan" đúng nghĩa là sau Giai đoạn 1.
**Không bật squad cho workspace đang chạy thật trước khi qua diễn tập R1 đến R5 của Giai đoạn 0.**

---

## Giai đoạn G: các cổng kiểm (không viết mã sản phẩm)

Mỗi cổng ra một đoạn kết luận ngắn, ghi vào spec mục 15 (đánh dấu đã đóng). Cổng nào cho kết quả "không" thì sửa thiết kế trước khi sang Giai đoạn 0.

| Cổng | Câu hỏi | Cách trả lời | Ảnh hưởng nếu "không" |
| --- | --- | --- | --- |
| G1 | Tạo base `sales` ở đâu, và seed kind mới cho workspace **đã có** thế nào? | Đọc `server/internal/service/knowledge_kind.go`, `knowledge_base*`, cách `company_template_test.go` đếm kind | Cần migration hoặc bước backfill; nếu không tạo được base mới thì dùng base `market` và đổi mục 6.1 |
| G2 | Kết nối Composio thuộc về người kết nối hay workspace; agent chạy theo lịch có dùng được không? | Đọc `server/internal/integrations/composio/`, `server/internal/runtimeapps/`, `packages/core/composio` | Bậc 1 và 6 không chạy được trong lịch tự động: chuyển sang "chạy khi người dùng kích hoạt" hoặc đổi cách giữ khóa |
| G3 | Có kind proposal nào dùng cho `sales_calibration` mà không cần code server mới? | Đọc `server/internal/service/proposal_kinds/` | Dùng kind quyết định chung ở P0; kind riêng đẩy sang P1 |
| G4 | Đường ghi KB của agent và của CLI: `multica knowledge add` đủ dùng cho `sales_account` (idempotency, `--supersedes`, `--tag`) không? | Đọc `cmd_knowledge.go`, `knowledge_write.go` | Bổ sung cờ ở P0 hay chờ `--field` ở P1 |
| G5 | Go đã có thư viện phân tích HTML/CSS selector trong `go.mod` chưa? | Đọc `server/go.mod` | Nếu chưa, **xin buy-in** trước khi thêm dependency (quy tắc "không mở rộng phạm vi âm thầm") |
| G6 | Quickstart (TS, chạy bởi người) tạo được tài liệu `sales_source` qua `POST /api/knowledge/documents` không, và policy `auto` có áp dụng? | Đọc `router.go` nhóm `/api/knowledge`, `knowledge_write.go` | Seed nguồn starter phải do server làm thay vì quickstart |
| G7 | App di động có nhận chia sẻ (share target) không? | Đọc `apps/mobile/AGENTS.md`, `app.config.ts` | Bỏ "chia sẻ từ di động" khỏi P2 |
| G8 | Spike Crawl4AI: tỷ lệ đọc được, bộ nhớ, thời gian, độ phủ trường của `generate_schema` trên ITviec, CareerViet, TopCV, VietnamWorks, voz, ba danh bạ hiệp hội, hai danh sách đơn vị trưng bày | Spec mục 13 "Spike Crawl4AI". **Cần người dùng cho phép** (cài nặng, gọi trang thật). Máy có mạng ổn hoặc Docker image | Quyết định Web engine là khuyến nghị mặc định hay chỉ tùy chọn; kết quả vào phụ lục A |
| G9 | Luật sư xem mục 10 của spec | Việc của người | Chặn **bật** bậc 6 cho dữ liệu có người Việt, không chặn viết mã |

---

## Giai đoạn 0: template, skill, bộ thu thập, nguồn starter

### T0.1 `routines[]` thay `routine`

Files: `packages/core/squads/templates.ts`, `quickstart.ts`, `quickstart.test.ts`, `templates.test.ts`, `packages/views/modals/squad-quickstart.tsx` (và test), nơi dùng `QuickstartOutcome.routine`.

- `SquadTemplate.routine: SquadRoutineTemplate | null` → `routines: readonly SquadRoutineTemplate[]`; `market_research` thành mảng một phần tử, các template khác mảng rỗng.
- `quickstart.ts`: vòng lặp tạo autopilot theo tên (đã có logic "reuse theo tên, link nếu chưa là routine"); `QuickstartOutcome.routine` thành `routines: { title, status, error? }[]`.
- Cập nhật chỗ hiển thị kết quả trong `squad-quickstart.tsx`.

Kiểm: `pnpm --filter @multica/core test -- quickstart templates`, `pnpm --filter @multica/views test -- squad-quickstart`. Chạy quickstart hai lần không tạo trùng routine.

### T0.2 Seed kind `sales_*` và policy (server)

Files: `server/internal/service/knowledge_kind.go` (`defaultKnowledgeKinds`), `knowledge_kind_test.go`, `company_template_test.go`, test seed liên quan; theo kết luận G1 có thể thêm migration/backfill.

- Thêm các kind ở spec 6.2 với mục bắt buộc (heading) và `default_base_key = sales`: `sales_lane`, `sales_source`, `sales_account`, `sales_opportunity`, `sales_outreach`, `sales_lesson`, `sales_share`, `sales_icp`, `sales_offer`, `sales_proof`, `sales_voice`, `sales_templates`. `review_days` theo spec 6.3 (account 90).
- Policy đăng: `auto` cho `sales_lane/source/account/opportunity/outreach/share`, `manual` cho `sales_icp/offer/proof/voice/templates`, `auto_after_review` cho `sales_lesson`.
- Base `sales` theo G1. P0 chưa có `fields` (P1); dữ liệu có cấu trúc nằm trong mục `Record` (`key: value`).

Kiểm: `cd server && go test ./internal/service -run 'Knowledge|CompanyTemplate'` (DB-backed, dùng fixture; khẳng định trên dòng của fixture, không trên tổng). Cập nhật các test đếm `len(defaultKnowledgeKinds)`.

### T0.3 Skill `sales-*` (7 thư mục) và danh sách tên

Files mới: `sdlc-skills/sales-delivery-core`, `sales-squad-lead`, `sales-scouting`, `sales-source-builder`, `sales-writing`, `sales-deal-analysis`, `sales-review` (spec 8.2). Mỗi thư mục `SKILL.md` + `reference.md`. Sửa `templates.ts` (`SALES_SKILL_NAMES`, mở rộng bộ lọc `SDLC_SKILL_NAMES`), `scripts/generate-sdlc-skills.mjs` nếu cần; chạy `pnpm generate:sdlc-skills`; thêm `sales-skills.test.ts` theo mẫu `planning-skills.test.ts`.

Nội dung theo spec:

- `sales-delivery-core`: chế độ không giám sát; ID ổn định; khóa chống trùng `sales:<kind>:<lane>:<entity_key>`; thang bằng chứng; mục `Record` và bảng sở hữu tài liệu; luật tuân thủ (mục 10); lệnh `multica knowledge` và `multica sales` cho kind `sales_*`; ranh giới cứng 4.9; định dạng báo cáo và snapshot.
- `sales-squad-lead`: hiệu chỉnh ngày 0 (mục 5), chọn route A đến F (mục 9), trần theo lane, kiểm chứng, snapshot, báo cáo, cổng người.
- `sales-scouting`: adapter và cách chạy, chấm điểm 4.7, đào bài chia sẻ 4.12, sức khỏe nguồn, dùng công cụ Composio/MCP cho bậc 1 và 6.
- `sales-source-builder`: onboarding 4.11, recipe 4.9, `generate_schema` khi có Web engine, khi nào dừng và báo `needs_connector`/`manual_only`/`blocked`, hardening 4.13.
- `sales-writing`: mẫu theo `goal` và kênh (email, điện thoại, chat, `public_reply`), giọng từ `sales_voice`, không nêu giá ngoài `sales_offer`.
- `sales-deal-analysis`: ghi chép họp thành opportunity, báo giá từ price book, thắng/thua, `sales_lesson` với `confidence`.
- `sales-review`: danh sách kiểm theo kênh và theo luật, verdict `pass/concerns/fail`, không sửa hộ.

Kiểm: `pnpm --filter @multica/core test -- sales-skills sdlc-skills`. Khẳng định: mỗi role nạp đúng skill; SDLC squad **không** nhận skill sales; bảng sở hữu và luật tuân thủ có mặt; không skill nào nêu giá hay hứa ngoài `sales_offer`.

### T0.4 Role, instruction và template `sales_loop`

Files: `packages/core/squads/templates.ts` (thêm `sales_loop` vào `SquadTemplateId`, năm role vào `SquadRoleKey`/`SQUAD_ROLES`, charter, instructions của Lead, `routines`, `builtinSkills: SALES_SKILL_NAMES`, `floorName: null`, `workflow: null`), `sales-agent-instructions.ts` (mới, theo `planning-agent-instructions.ts`), `quickstart.ts` (`instructionsFor`), `templates.test.ts`, `quickstart.test.ts`.

- Năm agent tiền tố `Sales` (Lead, Scout, Writer, Deal Analyst, Checker); test chống trùng tên với `company-template.ts`.
- Ba routine: Weekly scan `0 8 * * 1`, Weekly review `0 20 * * 0`, Source health `0 7 * * *` (spec 8.3).
- Charter: mission, trách nhiệm, đầu ra (`report`, `kb_document`, `proposal`), ranh giới ("không bao giờ gửi, đăng hay báo giá ngoài `sales_offer`"), escalation, intake.

Kiểm: `pnpm --filter @multica/core test -- templates quickstart`. Chạy quickstart hai lần không tạo trùng.

### T0.5 Thẻ quickstart và i18n

Files: `packages/views/modals/squad-quickstart.tsx`, `packages/views/locales/{en,zh-Hans,zh-Hant,fr,es,ko,ja}/modals.json`, `squad-quickstart.test.tsx`, `locales/parity.test.ts`.

Tên và mô tả ngắn theo "UI Copy" của AGENTS.md (không lặp tiêu đề, không viết đoạn dài). Kiểm: `pnpm --filter @multica/views test -- squad-quickstart parity`.

### T0.6 Nâng cấp squad đã tạo

Files: `packages/core/squads/upgrade.ts`, `upgrade.test.ts`, `scripts/upgrade-sdlc-squad.mjs`.

Cho phép nâng cấp skill và instruction của squad sales đã tạo. Kiểm: `pnpm --filter @multica/core test -- upgrade`.

### T0.7 Schema recipe và tín hiệu chuẩn hóa (Go)

Files mới: `server/internal/salescollect/` (đặt tên theo quy ước package hiện có sau khi xem `server/internal/`), `recipe.go`, `recipe_test.go`, `signal.go`.

- Struct và validation cho recipe version 1 (spec 4.9): `rung`, `method`, `fetch` (`engine`, `url`, `params`, `pagination`, `render`), `parse` (`items`, `fields`, `schema`), `filter`, `map`, `provider`, `cadence`, `rate_limit`, `max_items`, `verified_at`, `sample_count`, `field_coverage`, `fallbacks`.
- Dạng tín hiệu chuẩn hóa (spec 4.4): `source_id`, `family`, `entity_name`, `entity_key`, `url`, `observed_at`, `summary`, `raw_ref`, `region`, `sector_hint`; hàm `entity_key` (tên miền → mã số thuế → tên + tỉnh).
- Recipe không được chứa bí mật (test từ chối khóa dạng token).

Kiểm: `cd server && go test ./internal/salescollect`.

### T0.8 Adapter bậc 2 đến 4 và bộ chống lợi dụng

Files: `server/internal/salescollect/adapters_*.go`, fixture ghi sẵn trong `testdata/`.

- Adapter `rss` (RSS/Atom), `json_api` (URL mẫu, ánh xạ trường), `html_jsonld` (JSON-LD), `html_list` (CSS/XPath, theo kết luận G5), `csv_drop`.
- Bộ lọc từ khóa, cửa sổ ngày, khử trùng theo `entity_key`, giới hạn tốc độ theo tên miền, tôn trọng robots.txt.
- **Chặn đích nội bộ** (loopback, mạng riêng, link-local, địa chỉ metadata) cho mọi lần tải; test với URL nội bộ và với chuyển hướng tới nội bộ (R11).
- Giới hạn kích thước phản hồi và timeout.

Kiểm: `go test ./internal/salescollect` bằng fixture, không gọi mạng.

### T0.9 `multica sales probe` và `multica sales collect`

Files: `server/cmd/multica/cmd_sales.go` và test; đăng ký lệnh trong cây `multica`.

- `probe <đầu vào>`: nhận diện nền tảng (mẫu URL), tự khám phá feed (`<link rel=alternate>`), JSON-LD, oEmbed, sitemap, thử trang tĩnh; in các recipe ứng viên kèm mẫu tối đa 20 mục (JSON). Không dùng LLM, không dùng bậc 6.
- `collect --source <id> [--dry-run] [--limit N]`: đọc recipe từ tài liệu `sales_source`, chạy, ghi tín hiệu chuẩn hóa ra stdout/KB, cập nhật sức khỏe (`last_run`, `last_http`, `hits_7d`), từ chối khi vượt trần chi phí hay trần nguồn.
- Cách ghi kết quả vào KB theo G4 (P0: qua `multica knowledge add` với mục `Record`).

Kiểm: `cd server && go test ./cmd/multica -run Sales`. Test mặc định không resolve hay chạy agent CLI thật.

### T0.10 Nguồn starter và mẫu mạng xã hội (dữ liệu seed)

Files: `packages/core/squads/sales-starter-sources.ts` (dữ liệu), `sales-starter-sources.test.ts`; theo G6 phần tạo tài liệu nằm ở quickstart hoặc server.

- Mọi nguồn có adapter ở spec 4.5 (tin tức, tuyển dụng, đấu thầu, pháp nhân, cộng đồng, danh bạ khởi đầu) và 16 dòng mẫu mạng xã hội ở 4.10, mỗi dòng là một `sales_source` với recipe khung, `added_by = starter`, `terms`, `rung`, trạng thái `active` (không cần cấu hình) hoặc `needs_connector` kèm câu "kết nối gì thì mở được gì".
- Test: mọi recipe qua schema T0.7; không recipe nào chứa bí mật; mẫu bậc 6 không có trần chi phí mặc định lớn hơn 0; nguồn `restricted`/`unknown` không ở `active`.
- Idempotent: chạy lại không tạo trùng (khóa `sales:source:<id>`).

### T0.11 Route trong skill Lead và proposal hiệu chỉnh

Files: `sdlc-skills/sales-squad-lead`, `sales-source-builder`, `sales-scouting`.

- Route A (hiệu chỉnh, proposal tối đa ba câu và danh sách mở khóa nguồn), E (onboard nguồn bằng issue "Thêm nguồn: <link>"), F (chia sẻ và đào bằng issue "Chia sẻ: <link>"), B, C, D.
- Proposal hiệu chỉnh dùng kind theo G3.
- Tài liệu snapshot kind `report` sau mỗi lần chạy: bảng markdown theo giai đoạn, link hồ sơ, "cần bạn", "đến hạn", "sức khỏe nguồn".

Kiểm: test nội dung skill (route, tối đa ba câu, không soạn thư trước khi xác nhận).

### T0.12 Tài liệu và diễn tập

- Docs: thêm một đoạn trỏ tới spec trong `docs/engineering/digital-company.md`; hướng dẫn chạy container Crawl4AI (nếu G8 đạt) trong `docs/engineering/runbooks/`.
- Kiểm rộng: `pnpm typecheck`, `pnpm lint`, `pnpm test`, `cd server && go test ./internal/service ./internal/salescollect ./cmd/multica`. Ghi rõ lỗi sẵn có không thuộc thay đổi này.
- **Diễn tập trên agent thật** (cần người dùng cho phép): R1 đến R5 của spec mục 13 trên ba ngành khác nhau (F&B địa phương, nhà máy cơ khí, tư vấn thầu), thêm R10 (chưa có Web engine) và R4 (KB trống). Số đo: thời gian từ họp đến follow-up nháp; số lần phải can thiệp ngoài bốn cổng.

**Tiêu chí xong Giai đoạn 0:** quickstart tạo squad và ba routine, chạy lại an toàn; SDLC squad không nhận skill sales; hiệu chỉnh ngày 0 ra proposal ba câu; nguồn starter chạy được bằng `multica sales collect` trên fixture; nguồn xã hội ở `needs_connector` có câu mở khóa; snapshot doc xem được trên trang KB hiện có; mọi diễn tập đạt hoặc có ghi chú.

---

## Giai đoạn 1: thuộc tính tài liệu, Collections, Source Studio

### T1.1 Migration

Files: `server/migrations/` (số tiếp theo sau số cao nhất hiện có).

- `knowledge_link.properties JSONB NOT NULL DEFAULT '{}'`; `knowledge_kind.fields JSONB NOT NULL DEFAULT '[]'`; `squad.template_key TEXT`.
- Chỉ mục lọc thuộc tính (nếu cần theo đo thật) tạo `CONCURRENTLY`, mỗi chỉ mục một migration; có down migration.

### T1.2 SQL và sqlc

Query mới: lấy/ghi `properties`, lọc theo thuộc tính (`kind`, `field.<key>=<value>`, so sánh ngày), sắp xếp, phân trang; ghi lịch sử thay đổi. `make sqlc`.

### T1.3 Service

Files: `server/internal/service/knowledge_*.go`.

- Kiểm tra theo `fields` của kind: kiểu, `select` nằm trong `options`, khóa lạ bị từ chối.
- `UpdateProperties` chỉ đổi thuộc tính, không đụng WeKnora; ghi `updated_by` và lịch sử.
- Đọc mục `Record` của tài liệu P0 để điền `properties` (không di chuyển dữ liệu tay).
- Luật `stage`: agent không lùi trạng thái do người đặt.

Kiểm: `go test ./internal/service -run Knowledge` với fixture của mình, không khẳng định trên tổng.

### T1.4 API

Files: `server/internal/handler/knowledge_documents.go`, `router.go`, handler mới cho squad.

- `POST /api/knowledge/documents` nhận `properties`; `PATCH /api/knowledge/documents/{id}/properties`; `GET /api/knowledge/documents` có bộ lọc thuộc tính.
- `POST /api/squads/{id}/sources` (tạo `sales_source` `proposed` và issue cho Scout, route E), `POST /api/squads/{id}/shares` (route F).
- Quyền: member và agent sửa thuộc tính; publish vẫn chỉ owner/admin; tạo nguồn bậc 6 và kết nối cần owner/admin.

Kiểm: `go test ./internal/handler -run 'Knowledge|SquadSource'` dùng `testutil.Call(h, req).Want(status).JSON(&out)`.

### T1.5 CLI

Files: `server/cmd/multica/cmd_knowledge.go`. Thêm `--field k=v` cho `add`, lệnh `update <id> --field ...`, `list --field ... --output json`. Test lệnh.

### T1.6 Core TS

Files: `packages/core/knowledge/` (types, schemas có `parseWithFallback`, queries, mutations có `wsId`), `packages/core/types/squad.ts` (`template_key?`). Test phản hồi lỗi (trường lạ, enum lạ, thiếu trường).

### T1.7 Collections (bảng, kanban, chi tiết)

Files: `packages/views/knowledge/` (tab Collections), tái dùng `packages/core/collections/view-store.ts`; view lưu sẵn của Sales: Pipeline, Cần bạn, Đến hạn, Bài học, Sức khỏe nguồn; i18n bảy ngôn ngữ; test component cho nối dây, trợ năng và đường vui.

### T1.8 Source Studio

Files: `packages/views/squads/components/squad-sources-section.tsx` (+ test), chip ở phòng Sales trong tab Company.

- Danh sách nguồn (Collection `sales_source`), thêm nguồn một ô nhập, từng bước onboarding, mẫu 10 tín hiệu, các nút Kích hoạt/Sửa/Thử bậc khác/Chỉ nhận thủ công.
- Chi tiết nguồn: form + recipe JSON có kiểm schema, Chạy thử, Nhờ agent sửa, lịch sử chạy.
- Connector (dùng `packages/core/composio`), thẻ Web engine, ngân sách, hộp chia sẻ.
- Chỉ hiện khi `template_key = sales_loop`; backend cũ thì ẩn.

### T1.9 Ngân sách đầu cuối

Trần theo squad và theo nguồn: lưu ở `sales_source`/squad, `multica sales collect` từ chối khi vượt, Studio hiện mức dùng. Test: chạm trần thì dừng, không tự tăng (R9).

### T1.10 Web engine

Theo kết quả G8: `fetch.engine = crawl4ai` trong bộ chạy, phát hiện engine ở `localhost:11235` bằng token từ cấu hình daemon, MCP của Crawl4AI vào `mcp_config` của Sales Scout khi engine đang chạy, nguồn cần engine mà chưa có thì `needs_connector`. Test bằng engine giả (không cài Crawl4AI thật trong test mặc định).

### T1.11 Kiểm chứng và diễn tập

`pnpm typecheck`, `pnpm lint`, `pnpm test`, `make test`, `pnpm exec playwright test` cho luồng thêm nguồn (tạo bằng `TestApiClient`). Diễn tập R6 đến R11 (spec mục 13) khi được phép.

**Tiêu chí xong Giai đoạn 1:** thuộc tính trên tài liệu có kiểm tra và lịch sử; bảng, kanban, chi tiết hoạt động trên web và desktop; thêm một nguồn lạ qua Studio đi hết đường onboarding và kết thúc ở một trạng thái có lý do; ngân sách chặn thật; tài liệu P0 hiện đúng trong Collections không cần di chuyển.

---

## Giai đoạn 2: connector và chia sẻ nhanh

- **T2.1 Gmail:** đẩy thư `approved` vào Gmail nháp qua Composio (cần kết quả G2); không bao giờ gửi. Đọc phản hồi để cập nhật `stage`.
- **T2.2 Lịch:** cuộc họp tạo issue "Họp với X" cho squad.
- **T2.3 Chia sẻ nhanh:** bookmarklet hoặc tiện ích trình duyệt gọi `POST /api/squads/{id}/shares`; chia sẻ từ di động nếu G7 cho phép.
- Kiểm: test bằng connector giả; thư chỉ vào Gmail khi `state = approved` và Checker `pass`.

## Giai đoạn 3: handoff khi `won`

Dùng cơ chế handoff đã có (`cmd_squad_handoff.go`, `squad_handoff`): quy tắc từ Sales Loop sang squad giao hàng, gói bàn giao gồm opportunity, báo giá đã duyệt và liên hệ. Chế độ `off | ask | auto` giữ nguyên, mặc định `off`. Kiểm: test handoff hiện có cộng một ca cho nguồn là `sales_opportunity`.

---

## Phụ thuộc và song song

- T0.1 độc lập; T0.2, T0.3, T0.7 độc lập với nhau. T0.4 cần T0.1 và T0.3. T0.5 cần T0.4. T0.8 cần T0.7. T0.9 cần T0.7 và T0.8. T0.10 cần T0.7. T0.11 cần T0.3 và T0.9.
- Task động vào `templates.ts` và `quickstart.ts` (T0.1, T0.4) làm lần lượt để tránh xung đột; các task Go (T0.2, T0.7 đến T0.9) và skill (T0.3) chạy song song được vì không chung file.
- Giai đoạn 1: T1.1 đến T1.4 lần lượt (cùng chuỗi dữ liệu); T1.6 sau T1.4; T1.7, T1.8 sau T1.6 và song song được nếu không chung file.

## Quyết định cần người dùng trước khi làm

1. Cho phép chạy spike Crawl4AI (G8): cài đặt nặng, gọi trang thật.
2. Cho phép thêm dependency Go phân tích HTML nếu `go.mod` chưa có (G5).
3. Cho phép diễn tập trên agent thật (tốn quota) cuối Giai đoạn 0 và 1.
4. Luật sư xem mục 10 trước khi bật bậc 6 (G9).
5. Workspace nào được bật squad đầu tiên, và có token Apify cho nguồn xã hội hay chưa.

## Không làm trong kế hoạch này

Tự gửi thư hoặc đăng bài; tự báo giá cho khách; đăng nhập thay người dùng hay tài khoản ảo; bậc nhà cung cấp chạy mặc định; CRM đầy đủ; phía cung ứng và gọi vốn (spec mục 2).
