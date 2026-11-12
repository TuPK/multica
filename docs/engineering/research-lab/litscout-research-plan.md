# Kế hoạch triển khai Research Lab theo LitScout

Trạng thái: đã triển khai (2026-11-12); xem mục 8 cho kết quả từng bước và những gì chưa xác minh. Spec đi kèm: [litscout-research-spec.md](litscout-research-spec.md). Đọc spec trước; các mã **[V]/[S]**, "(mới)", Q1–Q11 và R1–R10 dùng cùng nghĩa như trong spec.

Ngôn ngữ: tiếng Việt. Định danh, đường dẫn, lệnh giữ nguyên tiếng Anh.

## 0. Nguyên tắc chung

- **Cổng chặn trước khi bắt đầu mã (bước P0).** Q1, Q2, Q4, Q8 phải có đáp án; Q1 quyết định cả hình dạng của nhóm (a). Các bước dưới đây giả định đáp án khuyến nghị trong spec.
- Mỗi bước là **một commit nguyên tử** theo conventional commit (ví dụ `feat(squads): ...`, `feat(cli): ...`, `test(research): ...`) và PR theo mẫu `.github/PULL_REQUEST_TEMPLATE.md` (Problem, Goal, Proposal, Alternatives, Screenshots). Bước chỉ đổi mã không đổi giao diện: ghi "không có thay đổi giao diện" thay cho ảnh chụp, và nếu quickstart hiển thị thẻ mới thì chụp thẻ đó vì đó là thay đổi giao diện nhìn thấy được.
- **Cây làm việc đang có 43 file sửa chưa commit** (kể cả `packages/core/squads/templates.ts`, `quickstart.ts`, `sdlc-skills-data.ts`). Không bắt đầu T3/T4 trước khi người dùng commit, stash hoặc xác nhận cho phép chồng lên các hunk đó. Các bước Go (nhóm b) không bị ảnh hưởng và có thể bắt đầu sớm. Không dùng lệnh git làm đổi trạng thái trong khuôn khổ tài liệu này; việc chọn nhánh do người thực hiện quyết định.
- Cỡ: **S** ≈ ≤ 0,5 ngày, **M** ≈ 1–2 ngày, **L** ≈ 3–5 ngày, tính cho một người quen repo. Đây là ước lượng thô [S].
- Kiểm thử theo `AGENTS.md`: chạy lệnh hẹp nhất khi lặp, mở rộng khi rủi ro cao, báo cáo đúng những gì đã chạy.
  - TS: `pnpm --filter @multica/core exec vitest run squads/<tệp>.test.ts`, rồi từ gốc `pnpm typecheck`, `pnpm lint`, `pnpm test`.
  - Go: `cd server && go test ./internal/litscout/... ./cmd/multica/...`, rồi từ gốc `make test` (cần Postgres của checkout; `make test` chạy `cmd/migrate up`).
  - Tổng hợp: `make check` khi xong cả hai nhóm.
  - Chỉ có thay đổi tài liệu: kiểm liên kết và `git diff --check`, ghi rõ không chạy test mã.
- Không thêm tính năng ngoài danh sách. Phần đánh dấu **[mở rộng]** là tùy chọn có điều kiện, không phải cam kết.

## 1. Tổng quan phụ thuộc

```text
P0 (chốt Q1,Q2,Q4,Q8)
 ├── Nhóm (b) Go:        G1 ─┬─ G2 ─┬─ G4 ─ G5 ─ G6
 │                           ├─ G3 ─┘          └─ G9 (tùy chọn)
 │                           └──────── G7 [mở rộng] ─ G8 [mở rộng]
 ├── Nhóm (a) squad TS:  T1 ─ T2 ─ T3 ─ T4 ─ T5 ─ T6        T7 (tùy chọn, sau cùng)
 └── Nhóm (c) đánh giá:  E1 (sau G2) ─ E2 ─ E3 (sau G5 + T3) ─ E4
```

Song song được: (G1, G3) với (T1, T2) ngay sau P0; G7 với E1/E2; T5 với mọi bước khác sau T3. Bắt buộc tuần tự: G1 → G2 → G4 → G5; T2 → T3 → T4; E3 sau khi cả G5 và T4 xong.

## 2. Nhóm (a): Squad, template, agent, skill (TypeScript, không có mã Go mới)

### P0. Chốt phạm vi (S)

- Mục tiêu: có đáp án Q1, Q2, Q4, Q8; đọc `apps/docs/content/docs/developers/conventions.mdx` và `conventions.zh.mdx` (spec mục 6 ghi **chưa đọc**) để chốt tên squad/role và quy ước dịch.
- Tệp: chỉ ghi lại quyết định vào đầu spec này.
- Phụ thuộc: không.
- Hoàn thành khi: mỗi câu hỏi có một dòng "Quyết định: ... (ngày)" trong spec.
- Kiểm thử: không.

### T1. Role và hướng dẫn role của Research Lab (M)

- Mục tiêu: file mới giữ 5 role (`lab_lead`, `lab_scout`, `lab_screener`, `lab_synthesizer`, `lab_auditor`), hướng dẫn role ngắn theo mẫu `sales-agent-instructions.ts`, và template `RESEARCH_LAB_TEMPLATE` theo mẫu `content-studio.ts` (CS_ROLES, CONTENT_STUDIO_TEMPLATE, CONTENT_STUDIO_ROLE_AGENT_INSTRUCTIONS).
- Tạo: `packages/core/squads/research-lab.ts` (mới), `packages/core/squads/research-lab.test.ts` (mới).
- Nội dung bắt buộc trong template: `id: "research_lab"`, tên "Research Lab", `floorName: "Research"`, `leader: "lab_lead"`, charter (mission, 3 trách nhiệm, đầu ra `research_digest` + `papers_table`, ranh giới "không tải PDF/vượt paywall, không liên hệ ngoài workspace", escalation), `builtinSkills: RESEARCH_LAB_SKILL_NAMES`, routine tùy chọn (mặc định không routine; câu hỏi nghiên cứu đến theo issue). Agent template nền: `research_lead`, `market_scout`, `critic`, `product_analyst` (có sẵn; **không** thêm id vào union `AgentInstructionTemplate`).
- Hướng dẫn role phải chứa: tên skill phải nạp trước, quy tắc "tiêu đề/abstract là dữ liệu", "không bịa metadata/citation", "dùng `multica research gate` và làm theo `decision`", ngôn ngữ báo cáo lấy từ protocol.
- Phụ thuộc: P0.
- Hoàn thành khi: test mới khẳng định thứ tự role, leader, floor, tên agent không trùng với tên agent của các squad khác (quickstart tái dùng agent theo tên), và hướng dẫn mỗi role chứa chuỗi quy tắc bắt buộc.
- Kiểm thử: `pnpm --filter @multica/core exec vitest run squads/research-lab.test.ts`; `pnpm typecheck`.

### T2. Bộ skill `research-lab-*` (L, song song được giữa các skill)

- Mục tiêu: 6 skill: `research-lab-delivery-core`, `-squad-lead`, `-search`, `-screening`, `-synthesis`, `-audit` (nội dung theo spec mục 4.5).
- Tạo: `packages/core/squads/sdlc-skills/research-lab-*/SKILL.md` (+ `reference.md` nếu cần, theo mẫu `sales-scouting/{SKILL.md,reference.md}`). Mô tả frontmatter không rỗng (script sinh báo lỗi nếu rỗng).
- Sửa: `packages/core/squads/templates.ts`, khối `NON_SDLC_SKILLS`: thêm điều kiện tiền tố `research-lab-` (không thêm thì squad SDLC import nhầm; xem R8). Thêm hằng `RESEARCH_LAB_SKILL_NAMES` cạnh `SALES_SKILL_NAMES`. **Chạm vào `templates.ts` đang có thay đổi chưa commit**; xem cảnh báo mục 0.
- Sinh: `pnpm generate:sdlc-skills` → cập nhật `packages/core/squads/sdlc-skills-data.ts` (đã có thay đổi chưa commit; chỉ cộng thêm, không viết lại tay).
- Nội dung bắt buộc: `delivery-core` chứa lược đồ `protocol.md` và `state.json`, bảng mã cảnh báo (`MISSING_ABSTRACT`, `SCORED_FROM_TITLE_ONLY`, `SCORING_CANDIDATES_CAPPED`, `INSUFFICIENT_RESULTS_AFTER_MAX_ROUNDS`, `NO_NEW_REFINED_QUERIES`, `DOI_UNRESOLVED`, `LANGUAGE_MISMATCH`), lệnh `multica research ...` kèm ví dụ in JSON; `screening` có thang 0–10 với mốc mô tả, không chấm "9–10" nếu thiếu abstract.
- Phụ thuộc: P0; lệnh trong skill phải khớp bề mặt CLI ở G2 (viết skill song song được, rà lại sau G2).
- Hoàn thành khi: test mới `research-lab-skills.test.ts` (theo `sales-skills.test.ts`) khẳng định: đúng 6 tên theo thứ tự, mô tả > 40 ký tự, nội dung > ngưỡng hợp lý, **SDLC squad không import skill nào có tiền tố `research-lab-`**, template `research_lab` import đúng bộ tên.
- Kiểm thử: `pnpm generate:sdlc-skills` rồi test lõi có sẵn kiểm "sinh ra còn mới"; `pnpm --filter @multica/core exec vitest run squads/sdlc-skills.test.ts squads/research-lab-skills.test.ts`.

### T3. Đăng ký template và nối quickstart (M)

- Mục tiêu: Research Lab xuất hiện trong quickstart và tạo được squad đầy đủ.
- Sửa: `packages/core/squads/templates.ts` (thêm `"research_lab"` vào `SquadTemplateId`; thêm 5 khóa vào `BaseSquadRoleKey`; trải `RESEARCH_LAB_ROLES` vào `SQUAD_ROLES` giống `...CS_ROLES` ở khoảng dòng 338; thêm `RESEARCH_LAB_TEMPLATE` vào danh sách `inCategory("core", [...])` ở khoảng dòng 1021), `packages/core/squads/quickstart.ts` (thêm nhánh `RESEARCH_LAB_ROLE_AGENT_INSTRUCTIONS` trong `instructionsFor`, khoảng dòng 200–223). Cả hai file đang có thay đổi chưa commit; xem mục 0.
- Phụ thuộc: T1, T2.
- Hoàn thành khi: (1) `SQUAD_TEMPLATES` có `research_lab` ở danh mục `core`; (2) `planQuickstartAgents` trả đúng 5 role; (3) `instructionsFor` trả hướng dẫn riêng cho 5 role; (4) test `market_research` và test công ty mẫu (`templates.test.ts`, `company-template.test.ts`) **vẫn pass không sửa** (không đổi `market_research`, không thêm `research_lab` vào `COMPANY_SQUAD_KEYS`).
- Kiểm thử: `pnpm --filter @multica/core exec vitest run squads/`; `pnpm typecheck`; `pnpm lint`; thêm một test `quickstart` cho `research_lab` theo mẫu các test hiện có.

### T4. Thẻ quickstart và dịch (S)

- Mục tiêu: thẻ Research Lab hiển thị đúng ở mọi ngôn ngữ.
- Sửa: `packages/views/locales/{en,es,fr,ja,ko,zh-Hans,zh-Hant}/modals.json`, khóa `templates.research_lab.{name,description}`. Mô tả ngắn, không nhắc lại tên; theo mục "UI descriptions" trong `conventions.mdx`.
- Phụ thuộc: T3 (cần `id`), P0 (quy ước đặt tên).
- Hoàn thành khi: 7 locale có đủ khóa; test locale có sẵn (nếu có kiểm khớp khóa giữa ngôn ngữ) pass. Chụp ảnh thẻ trong quickstart cho PR (thay đổi giao diện).
- Kiểm thử: `pnpm --filter @multica/views test`; `pnpm typecheck`.

### T5. Tài liệu vận hành (S, song song với T4)

- Tạo: `docs/engineering/research-lab/research-lab-runbook.md` (mới): cách tạo squad từ quickstart, đặt `OPENALEX_API_KEY` trên máy daemon, chạy một nghiên cứu, đọc `state.json`, xử lý cảnh báo, giới hạn (không PDF ở P1–P2).
- Phụ thuộc: G5 để có lệnh thật; có thể viết khung trước.
- Hoàn thành khi: mọi lệnh trong tài liệu chạy được trên bản dựng hiện tại, hoặc được đánh dấu rõ là chưa có.
- Kiểm thử: `git diff --check`; kiểm tra liên kết nội bộ. Ghi rõ không chạy test mã.

### T6. Rà soát bất biến (S)

- Mục tiêu: khóa các bất biến bằng test: không `research_lab` trong `COMPANY_SQUAD_KEYS`; tên agent Research Lab không trùng tên agent hiện có; không role Research Lab nào dùng agent template ngoài danh sách đã chọn.
- Sửa: `packages/core/squads/research-lab.test.ts`.
- Phụ thuộc: T3.
- Hoàn thành khi: test pass và thất bại khi cố thêm `research_lab` vào công ty mẫu.

### T7. [Tùy chọn, cần Q10 = đồng ý] Điều chỉnh agent dùng chung (S)

- Mục tiêu: `critic` thêm đoạn "Citation check" (chạy `multica research verify` nếu claim trích paper); `market_scout` thêm một dòng cho phép `multica research search` khi cần nền học thuật.
- Sửa: `packages/core/agents/instruction-templates.ts` (đoạn `critic`, `market_scout`), giữ nguyên mọi chuỗi mà `instruction-templates.test.ts` và `templates.test.ts` đang khóa.
- Phụ thuộc: G5 (lệnh phải tồn tại), T3.
- Hoàn thành khi: tất cả test hiện có pass không sửa; thêm test khẳng định hai đoạn mới; ghi trong PR rằng thay đổi ảnh hưởng 4 squad dùng chung agent.
- Cảnh báo: thay đổi hướng dẫn agent dùng chung có hiệu lực với agent đã tạo hay chỉ agent mới? Chưa xác minh cơ chế `upgrade` (có `packages/core/squads/upgrade.ts`); đọc trước khi làm.

## 3. Nhóm (b): Công cụ chuyển sang Go

Toàn bộ nằm ở `server/internal/litscout/` (mới) và `server/cmd/multica/cmd_research.go` (mới). Không có migration, không có route, không sqlc (xem spec mục 6). Mọi bước dùng kiểm thử bảng + `httptest`, không gọi mạng thật, không chạy CLI agent thật.

### G1. Mô hình, state, router (M)

- Mục tiêu: lõi xác định: `paper` (Metadata/Analysis/Scored, chuẩn hóa DOI), `State` (đọc/ghi nguyên tử, `schema_version`), `Route`, `Key` chuẩn (T7 trong spec), cảnh báo dạng mã.
- Tạo: `server/internal/litscout/paper/paper.go`, `state.go`, `router.go` và `_test.go`.
- Phụ thuộc: P0.
- Hoàn thành khi: test bảng phủ đủ 3 nhánh router + biên (đúng `min_papers`, đúng `max_rounds`, 0 truy vấn mới); `Load` từ chối `schema_version` lạ; ghi bằng tệp tạm + rename; DOI chuẩn hóa khớp các ví dụ Python (`https://doi.org/10.59232/DST-V5I1P101` → `10.59232/dst-v5i1p101`).
- Kiểm thử: `cd server && go test ./internal/litscout/...`.

### G2. Dedup, abstract, prefilter, provenance (M)

- Mục tiêu: các dịch vụ xác định, đối ứng `dedup.py`, `abstract.py`, phần prefilter của `relevance.py`, `provenance.py`.
- Tạo: `dedup.go`, `abstract.go`, `prefilter.go`, `provenance.go` và test. Tự viết tỉ lệ Indel theo `rune`; chuẩn hóa văn bản bằng `golang.org/x/text/unicode/norm` + `x/text/cases` (chuyển `x/text` từ gián tiếp sang trực tiếp trong `server/go.mod`, đúng quy tắc mỗi workspace khai báo phụ thuộc import trực tiếp).
- Phụ thuộc: G1.
- Hoàn thành khi: có test tương ứng 6 test Python (dedup theo thứ tự bằng chứng, fuzzy không đủ để gộp nếu thiếu tác giả trùng, abstract thiếu không bị bịa, provenance, prefilter cắt đúng số lượng, tái dùng điểm); thêm test tiếng Việt có dấu và "ß"; sắp xếp ổn định giữ thứ tự bản ghi bằng nhau; cờ khác biệt có chủ đích với Python được ghi trong chú thích Go (rationale, không kể lịch sử sự cố).
- Kiểm thử: `go test ./internal/litscout/...`.
- Rủi ro đã biết: R2. Xác nhận độ lệch giữa Indel tự viết và `rapidfuzz` ở E1.

### G3. Nguồn OpenAlex, retry, cache (M)

- Mục tiêu: `Source` interface và OpenAlex: `select` đủ trường, lọc năm, `per_page` ≤ 100, retry hữu hạn (429/5xx/mạng, `Retry-After` dạng giây và ngày HTTP, backoff mũ + jitter, tối thiểu 5 s), phát hiện hết hạn mức, dựng abstract từ inverted index, chuẩn hóa work → Metadata, cache tệp TTL.
- Tạo: `source/source.go`, `source/openalex.go`, `cache.go` và test; `testdata/openalex_*.json`.
- Phụ thuộc: G1.
- Hoàn thành khi: `httptest.Server` mô phỏng 200, 429 có/không `Retry-After`, 5xx, ngắt mạng, hết hạn mức; test đồng hồ và RNG được tiêm nên không ngủ thật; **khóa cache và nội dung cache không bao giờ chứa API key** (test); work hỏng bị bỏ qua có log, không hỏng cả trang; dựng abstract khớp Python với trường hợp trùng vị trí.
- Kiểm thử: `go test ./internal/litscout/...`.
- Lưu ý: khóa OpenAlex theo Q8; không bao giờ in khóa ra log (`pkg/redact` có sẵn).

### G4. Ingest schema và export (M)

- Mục tiêu: `ingest-scores`/`ingest-summaries` kiểm nghiêm ngặt (`DisallowUnknownFields`, điểm 0–10, `paper_index` đủ và không lặp, không `summary` cho bài thiếu abstract, kiểm ngôn ngữ), và export CSV/Markdown.
- Tạo: `ingest.go`, `export_csv.go`, `export_md.go` và test; `testdata/` chứa `papers.csv`, `research_digest.md` mẫu từ nhánh `examples/legal-rag-run`.
- Phụ thuộc: G1, G2.
- Hoàn thành khi: lỗi trả về nêu rõ trường và chỉ số sai; CSV chống ô bắt đầu bằng `= + - @`; escape Markdown; export khớp bản mẫu (cho phép khác biệt đã ghi: dấu xuống dòng, BOM tùy chọn).
- Kiểm thử: `go test ./internal/litscout/...`.

### G5. Nhóm lệnh `multica research` (M)

- Mục tiêu: nối G1–G4 thành CLI (`init`, `search`, `dedup`, `prefilter`, `ingest-scores`, `gate`, `ingest-summaries`, `verify`, `export`), mỗi lệnh in JSON, mã thoát ≠ 0 khi không đọc được hoặc vi phạm cổng.
- Tạo: `server/cmd/multica/cmd_research.go`, `cmd_research_test.go` (theo `cmd_sales.go`, `cmd_sales_test.go`).
- Phụ thuộc: G2, G3, G4.
- Hoàn thành khi: test chạy chuỗi đầy đủ trên `httptest` từ `init` đến `export` và so với bản mẫu; `verify` thất bại với bài không có định danh truy được; `gate` trả `refine_query`/`summarize_available` đúng; `multica research --help` liệt kê đủ lệnh; không đụng lệnh có sẵn.
- Kiểm thử: `cd server && go test ./cmd/multica/...`; `make test` (cần Postgres); `go vet ./...`.

### G6. Chạy thử thủ công có kiểm soát (S)

- Mục tiêu: chạy một truy vấn thật với khóa OpenAlex thật trên máy của người thực hiện để kiểm tra khác biệt thực tế (ví dụ trường `type`, hạn mức), ghi vào runbook.
- Phụ thuộc: G5.
- Hoàn thành khi: có log chạy thật đính kèm PR (đã xóa khóa). Đây là kiểm thử có truy cập mạng thật, phải được bạn đồng ý trước và **không** nằm trong bộ test mặc định.

### G7. [Mở rộng, Q9] Crossref `doi-check` (S)

- Mục tiêu: xác nhận DOI; cờ `DOI_UNRESOLVED`.
- Tạo: `source/crossref.go`, lệnh `multica research doi-check`. Địa chỉ liên hệ (`mailto`) lấy từ cấu hình, không gán cứng.
- Phụ thuộc: G3, G5.
- Hoàn thành khi: test `httptest` cho 200, 404, 429; `verify` có tùy chọn bắt buộc `doi-check` đã chạy.

### G8. [Mở rộng, Q9] arXiv, Semantic Scholar, citation chasing (M mỗi nguồn)

- Mục tiêu: nguồn thứ hai/ba và vòng 3 theo tham chiếu.
- Tạo: `source/arxiv.go` (Atom qua `encoding/xml`, tôn trọng giới hạn tốc độ), `source/s2.go`, lệnh `multica research chase`.
- Trước khi viết: xác minh tham số thật của OpenAlex (`referenced_works`, `filter=cites:`) và điều khoản của từng API; chưa xác minh trong khảo sát (spec X3, X4).
- Phụ thuộc: G3, G5.
- Hoàn thành khi: mỗi nguồn có test `httptest` + vào allowlist provenance + được dedup khớp chéo nguồn.

### G9. [Mở rộng, Q9, mặc định tắt] Toàn văn truy cập mở (L)

- Mục tiêu: Unpaywall + trích văn bản PDF cho bài truy cập mở, cờ `FULLTEXT_LOW_CONFIDENCE`.
- Tạo: `source/unpaywall.go`, `fulltext.go` dùng `github.com/ledongthuc/pdf` (đã có trong `go.mod`) hoặc `pdftotext` nếu có.
- Điều kiện làm: chỉ khi bạn bật trong protocol; không vượt paywall; giới hạn kích thước tệp; chạy trích PDF có timeout.
- Hoàn thành khi: có bộ PDF mẫu đa dạng (một cột, hai cột, scan) với chất lượng ghi rõ; nếu chất lượng không đạt thì không bật mặc định và ghi trong runbook.

## 4. Nhóm (c): Kiểm thử và đánh giá chất lượng nghiên cứu

### E1. Golden test đối chiếu LitScout Python (M)

- Mục tiêu: chứng minh các phần xác định của Go tương đương Python (hoặc ghi rõ khác biệt có chủ đích).
- Cách làm: lấy `examples/legal_rag_run/{papers.csv,research_digest.md,run.log,config.json,command.txt}` từ nhánh `examples/legal-rag-run` [V] làm kết quả mong đợi; thu phản hồi OpenAlex cho 3 truy vấn của lần chạy (phản hồi gốc chưa chắc còn lưu; nếu không, một lần chạy Python thật bằng khóa của bạn để thu), lưu vào `server/internal/litscout/testdata/`. Kiểm tra: 22 → 21, +30 → 47, +20 → 52 bản ghi duy nhất, 4 `MISSING_ABSTRACT`, `SCORING_CANDIDATES_CAPPED: 22 -> 15`.
- Tạo: `server/internal/litscout/golden_test.go`, `testdata/`.
- Phụ thuộc: G2, G3.
- Hoàn thành khi: golden test pass hoặc từng chỗ lệch có giải thích (ví dụ xử lý dấu tiếng Việt) và được bạn chấp nhận. Sai lệch dedup quanh ngưỡng 96 (R2) được kiểm trực tiếp trên cặp tiêu đề gần giống.
- Kiểm thử: `go test ./internal/litscout/...`.

### E2. Tập vàng và công cụ `eval` (M)

- Mục tiêu: tập vàng 3 đề tài × 15–30 bài do **người** gắn nhãn (spec mục 7.4) và lệnh `multica research eval` tính precision@k, recall, dedup precision, language compliance, grounding.
- Tạo: `docs/engineering/research-lab/gold/` (mới; JSON), `server/internal/litscout/eval.go`, `eval_test.go`, thêm lệnh `eval` vào `cmd_research.go`.
- Phụ thuộc: G5; người gắn nhãn do bạn chỉ định.
- Hoàn thành khi: `eval` chạy trên tập vàng và in bảng chỉ số; thống nhất ngưỡng **sau** lần chạy cơ sở (E3), không đặt số trước.
- Kiểm thử: `go test ./internal/litscout/...`.

### E3. Chạy thử đầu cuối bằng squad thật (M)

- Mục tiêu: tạo squad từ quickstart, chạy 3 đề tài, đo chi phí và chất lượng. Kiểm hai điều Lead phải làm: dùng `gate` thay vì tự quyết, và dừng đúng sau `max_rounds`.
- Phụ thuộc: G5 + T4 (+ T2 skill đã rà theo CLI thật).
- Hoàn thành khi: báo cáo `docs/engineering/research-lab/research-lab-pilot-report.md` (mới) với số liệu thật: số lượt chạy agent, số request, số lần trúng cache, các chỉ số E2, danh sách lỗi hành vi agent và sửa skill tương ứng. Chạy thật tốn tài nguyên model và truy cập mạng: cần bạn đồng ý trước.
- Kiểm thử: đây là kiểm thử thực địa; không đưa vào CI.

### E4. Chốt ngưỡng và khóa hồi quy (S)

- Mục tiêu: từ số liệu E3, đặt ngưỡng cho các chỉ số mềm và thêm test hồi quy cho hành vi sai đã gặp.
- Phụ thuộc: E3.
- Hoàn thành khi: ngưỡng ghi trong spec mục 7.3 thay cho ô "đặt sau"; mỗi lỗi hành vi của E3 có một test hoặc một dòng trong skill.

## 5. Bảng tổng hợp

| Bước | Nhóm | Cỡ | Phụ thuộc | Song song với |
| --- | --- | :-: | --- | --- |
| P0 | — | S | — | — |
| T1 | a | M | P0 | G1, G3 |
| T2 | a | L | P0 (rà lại sau G5) | G1–G4 |
| T3 | a | M | T1, T2; **cây làm việc đã được người dùng xử lý** | G1–G5 |
| T4 | a | S | T3, P0 | T5 |
| T5 | a | S | G5 | T4 |
| T6 | a | S | T3 | T4, T5 |
| T7 | a (tùy chọn) | S | G5, T3, Q10 | — |
| G1 | b | M | P0 | T1, T2 |
| G2 | b | M | G1 | G3, T1, T2 |
| G3 | b | M | G1 | G2 |
| G4 | b | M | G1, G2 | G3 |
| G5 | b | M | G2, G3, G4 | T2 (rà), T3 |
| G6 | b | S | G5 | T4 |
| G7 | b (mở rộng) | S | G3, G5 | E1, E2 |
| G8 | b (mở rộng) | M+ | G3, G5 | G7 |
| G9 | b (mở rộng) | L | G5, Q9 | — |
| E1 | c | M | G2, G3 | G4, T1, T2 |
| E2 | c | M | G5, người gắn nhãn (do bạn chỉ định) | G7 |
| E3 | c | M | G5, T4 | — |
| E4 | c | S | E3 | — |

Tổng cỡ các bước bắt buộc (không tính G7–G9, T7): khoảng 3 S + 8 M + 1 L ≈ 15–22 ngày công nếu một người làm tuần tự; còn khoảng 8–11 ngày nếu chia hai luồng (nhóm a và nhóm b) rồi hợp ở G5/E3 [S].

## 6. Đóng gói theo mốc

| Mốc | Gồm | Giá trị giao được |
| --- | --- | --- |
| M1 "Công cụ chạy được" | P0, G1–G6, E1 | `multica research ...` chạy xác định, đối chiếu Python, chưa có squad |
| M2 "Squad dùng được" | T1–T6, E2, E3 (một đề tài) | Research Lab trong quickstart, 5 role, 6 skill, quy trình nhiều vòng |
| M3 "Chuyên sâu" | G7, G8, E3 đủ 3 đề tài, E4 | Xác minh DOI, nhiều nguồn, theo vết trích dẫn, ngưỡng chất lượng có căn cứ |
| M4 "Toàn văn" (tùy chọn) | G9, T7 | Đọc bài truy cập mở có cờ độ tin cậy; điều chỉnh agent dùng chung |

## 7. Điều chưa làm trong kế hoạch này

- Không đổi `market_research`, `opportunity_lab`, `competitor_intelligence` và công ty mẫu.
- Không thêm bảng Postgres, không thêm API/route, không thêm UI. Nếu cần (Q5), mở spec riêng và áp dụng quy tắc migration (không FK, `CREATE INDEX CONCURRENTLY`, mỗi index một tệp, `make sqlc`).
- Không port bước LLM của LitScout vào Go (spec T9). `server/pkg/llm` có sẵn nhưng không dùng cho CLI.
- Không port RAG/embedding: LitScout không có.
- Không thay đổi thư mục `templates/` theo `docs/specs/template-registry`; nếu spec đó được duyệt trước, đường dẫn của T1–T2 phải cập nhật.

## 8. Báo cáo triển khai (2026-11-12)

Quyết định người dùng chốt: Q1 (domain riêng, không đụng `market_research`), Q2 (văn liệu học thuật), Q8 (key OpenAlex cấu hình trên web), Q9 (bật toàn bộ nguồn), Q10 (không sửa `critic`/`market_scout`). Các điều chỉnh so với spec nằm ở đầu [litscout-research-spec.md](litscout-research-spec.md).

### 8.1 Trạng thái từng bước

| Bước | Trạng thái | Ghi chú |
| --- | --- | --- |
| P0 | Xong | Q1, Q2, Q8, Q9, Q10 đã chốt; `conventions.mdx` và `conventions.zh.mdx` đã đọc (thuật ngữ: không dịch tên nguồn và "API"; "API 密钥" ở zh-Hans, "API 金鑰" ở zh-Hant). |
| T1 | Xong | `packages/core/squads/research-lab.ts`, `research-lab.test.ts`. |
| T2 | Xong | 6 skill `research-lab-*`; tiền tố được thêm vào bộ lọc `NON_SDLC_SKILLS`; `sdlc-skills-data.ts` sinh lại. |
| T3 | Xong | `templates.ts`, `quickstart.ts` (chỉ thêm đúng các hunk của Research Lab). |
| T4 | Xong | Thẻ quickstart trong `modals.json` và tab Research sources trong `settings.json`, đủ 7 locale. |
| T5 | Xong | [research-lab-runbook.md](research-lab-runbook.md). |
| T6 | Xong | Các bất biến nằm trong `research-lab.test.ts` (không có `research_lab` trong công ty mẫu, tên agent không trùng, không dùng chung agent template của Market Research). |
| T7 | Không làm | Theo Q10. |
| G1 đến G5 | Xong | `server/internal/litscout/**`, `server/cmd/multica/cmd_research.go`. |
| G6 | Xong, thủ công | Chạy thật một lần với OpenAlex, Crossref, arXiv, Semantic Scholar (không dùng key): xem 8.3. Không nằm trong bộ test. |
| G7 | Xong | Crossref `doi-check`, kèm dự phòng doi.org (phát hiện khi chạy thật). |
| G8 | Xong một phần | arXiv, Semantic Scholar (search), `chase` qua OpenAlex. Semantic Scholar citation API không làm. |
| G9 | Xong | Unpaywall, `pdftotext` hoặc bộ đọc PDF thuần Go. |
| Q8 (mới) | Xong | Migration 982, `internal/research`, 3 endpoint, tab web. |
| E1 | Xong (phần làm được) | Golden export (CSV và digest) khớp byte từ kết quả LitScout Python; golden dedup, prefilter, `fuzz.ratio`, heuristic, dựng abstract, DOI sinh từ chính mã Python (mục 9). **Không làm được:** tái hiện con số 22/52/72 của lần chạy tháng 9, vì cần đúng phản hồi OpenAlex của hôm đó, không còn. |
| E2 | Xong công cụ | `multica research eval` và định dạng gold; **chưa có tập vàng thật** (cần người gắn nhãn). |
| E3 | Bị chặn | Cần squad chạy thật: agent runtime, key, chi phí model, và bạn đồng ý. |
| E4 | Bị chặn | Phụ thuộc E3. |

### 8.2 Lệnh kiểm tra đã chạy và kết quả

| Lệnh | Kết quả |
| --- | --- |
| `go test -race ./internal/litscout/...` | pass (cache, paper, source, litscout) |
| `go test ./internal/research/` | pass |
| `go test ./cmd/multica/ -run TestResearch` và cả gói | `TestResearch*` pass. Cả gói pass lần đầu (13 s); lần chạy song song với các gói khác, `TestDaemonStopForceHandlesUnresponsiveHealthWithVerifiedProfilePID` fail do tải máy và pass khi chạy riêng. |
| `go test ./internal/handler/ -run 'TestResearchSettings\|TestWorkspaceDeletionManifest'` | 5 test `TestResearchSettings*` pass. Manifest pass ở lần chạy đầu; về sau fail vì `missing=[publish_settings]`, một bảng mới trong công việc đang làm dở về publishing của người dùng, không phải của Research Lab. |
| `go test ./cmd/server/ -run 'TestRouterWiresTheResearchSettings\|TestRouterCORSContract'` | pass |
| `go vet` các gói đã đụng | pass |
| `go run ./cmd/migrate up` (DB của checkout) | áp dụng 982 (và các migration 978 đến 981 của người dùng chưa được áp dụng trước đó) |
| `make sqlc` | chạy; chỉ thêm `research.sql.go` và struct `ResearchSetting` trong `models.go` |
| `pnpm typecheck` | pass (9/9) |
| `pnpm lint` | pass, EXIT=0 (chỉ có cảnh báo `react-hooks/exhaustive-deps` đã có sẵn ở web, desktop, ui) |
| `vitest` toàn bộ `@multica/core` (`--testTimeout=120000`) | 332 file, 5001 test pass, 1 skipped |
| `vitest` `@multica/views` (`settings`, `locales`) | 853 pass; 2 fail trong `wecom-scan-install-dialog.test.tsx` khi máy quá tải và pass khi chạy riêng (14/14) |

### 8.3 Lần chạy thủ công với API thật (không phải test)

`multica research init/search/dedup/prefilter/ingest-scores --heuristic/gate/doi-check/verify/export` với `--results-per-query 3`, không key, `--no-cache`:
- OpenAlex: 3 kết quả; các trường `select` (gồm `type`, `is_paratext`, `referenced_works`, `open_access`) được API chấp nhận. Crossref: 3. arXiv: 3. Semantic Scholar: HTTP 429 khi không có key, bị ghi thành `SEARCH_FAILED`/`SOURCE_DISABLED` và các nguồn khác vẫn chạy.
- 9 bản ghi gộp còn 6 bài duy nhất (bản arXiv và bản OpenAlex của cùng một preprint được gộp theo DOI).
- Hai lỗi thật lộ ra và đã sửa: DOI của arXiv báo `unresolved` ở Crossref vì thuộc DataCite (nay hỏi tiếp doi.org); bộ đọc PDF thuần Go làm mất khoảng cách giữa các từ với PDF dàn bằng TeX (nay dùng `pdftotext` nếu có, nếu không thì từ chối kết quả).
- `fulltext` trên một PDF arXiv thật cho văn bản sạch qua `pdftotext`.

### 8.4 Chưa xác minh

- Chưa chạy squad Research Lab đầu cuối (agent thật, skill thật): chưa biết agent có tuân thủ skill, chuyển `state.json` bằng tệp đính kèm trót lọt, và chi phí thực tế.
- Chưa chạy Playwright/E2E. Tab web chỉ có test đơn vị (jsdom) và typecheck; chưa xem bằng mắt trên trình duyệt, chưa chụp ảnh cho PR.
- Chưa kiểm tra OpenAlex với key thật, hạn mức hết (HTTP 409) và Unpaywall thật (cần địa chỉ email liên hệ thật). Hành vi 409 chỉ có test với server giả.
- Chưa chạy `make test` / `make check` cả bộ: các gói DB chạy song song trên DB dev dùng chung, nên có lỗi không liên quan (xem 8.2); `internal/handler` toàn bộ chạy xong một lần (khoảng 11 phút, song song với gói khác trên cùng DB dev) với nhiều lỗi thuộc nhóm claim-task, `TestCompanyDay`, `TestCreateComment_ThreadReply...`; tôi chưa chạy lại từng test một nên chưa xác định tất cả đều do tải máy hoặc DB dùng chung (chạy lại riêng 3 test claim-deadline: 2 pass, 1 vẫn fail vì `context deadline exceeded` khi máy tải cao).
- Cây làm việc bị sửa đồng thời bởi người khác: khối `h.Research = ...` trong `cmd/server/router.go` từng bị mất một lần khi phần publishing bên cạnh được viết lại và đã được thêm lại; test `TestRouterWiresTheResearchSettings` giữ cho nó không mất âm thầm. Gói `cmd/server` tại một thời điểm không build được do `GetPublishSettings` của công việc publishing chưa sinh lại bằng `make sqlc`.
- Chưa có tập vàng thật nên các chỉ số precision/recall chưa có số.

### 8.5 Quyết định nhỏ tự chọn (để xem lại)

- Khóa niêm phong key: `MULTICA_RESEARCH_SECRET_KEY`, nếu thiếu thì dẫn xuất từ `JWT_SECRET` (cho phép dùng trang ngay không cần cấu hình thêm, đổi lại đổi `JWT_SECRET` thì mất key đã lưu). Có thể đổi thành bắt buộc đặt khóa riêng.
- `GET /api/research/credentials` trả key dạng rõ cho mọi thành viên và agent của workspace (agent chạy trên máy daemon cần nó). Nếu muốn hạn chế, đổi thành chỉ agent hoặc chỉ owner/admin.
- Tab Research sources chỉ hiện cho owner/admin (như tab Publishing); thành viên thường không thấy trạng thái key.
- Nhiều lệnh con có `--dir` mặc định `research-run`, `--cache-ttl` 24 giờ, `--results-per-query` 10, `--threshold` 6, như mặc định của LitScout.
- Role Research Lab dùng agent template nền riêng (`planning_lead`, `opportunity_analyst`, `validation_analyst`, `product_analyst`, `feasibility_analyst`); hướng dẫn thật nằm trong `research-lab.ts` và ghi đè qua `instructionsFor`.
- Lọc loại tài liệu mặc định bỏ `paratext` và `erratum`.
- Thêm `x/text` làm phụ thuộc trực tiếp trong `server/go.mod`; không thêm phụ thuộc mới nào khác.

## 9. Rà soát lần hai (2026-11-12)

### 9.1 Thay đổi

- **Credentials thu hẹp.** `GET /api/research/credentials` chỉ cho agent task (`task_token` kèm agent id) và owner/admin; thành viên thường nhận 403. `multica research` in cảnh báo ra stderr khi gặp 403 rồi dùng biến môi trường. Khi thiếu `MULTICA_RESEARCH_SECRET_KEY`, server vẫn dẫn xuất khóa từ `JWT_SECRET` nhưng ghi WARN rõ lúc khởi động; runbook ghi hệ quả.
- **Golden đối chiếu Python** (`server/internal/litscout/golden_test.go`, `testdata/python_golden/{generate.py,golden.json}`): `IndelRatio` khớp rapidfuzz trên 54 cặp; dedup, `Same`, prefilter, điểm heuristic, dựng abstract, DOI khớp. Hai khác biệt có chủ đích được khóa bằng test (xem spec).
- **Test thêm:** `runSquadQuickstart` cho `research_lab` (6 test trong `quickstart.test.ts`); `TestResearchCommandSet` và test TS đọc danh sách lệnh trực tiếp từ `cmd_research.go` (skill không thể trôi khỏi CLI); thành viên thường đọc status nhưng không ghi; `Save` không để lại tệp tạm; test CLI cho việc lấy key từ workspace.
- **E2E:** `e2e/research-settings.spec.ts` (mock API ở biên mạng). **Chưa chạy**: `loginAsDefault` tạo user và workspace qua API của môi trường e2e, và máy này chỉ có server đang chạy trên DB chính.

### 9.2 Squad trên workspace `tupk`

Workspace `tupk` (id `50cd7d4d-7e86-4fb4-ac2d-486ef9c3bba1`, DB `multica`, server `localhost:8080`) chưa có squad Research Lab. Đã tạo bằng chính `runSquadQuickstart` của template. Squad id `a2d01887-7855-46cd-855b-9779c3e2af0e`, tên "Research Lab", 5 agent, 6 skill gắn ở cấp squad, runtime Claude. Chi tiết trong báo cáo cuối của phiên.

### 9.3 Còn thiếu hoặc bị chặn

| Mục | Trạng thái | Lý do |
| --- | --- | --- |
| E3 pilot, E4 ngưỡng chỉ số | Bị chặn | Cần chạy squad với agent thật (chi phí model, nhiều giờ) và có số liệu cơ sở. |
| E2 tập vàng thật | Bị chặn | Chỉ người gắn nhãn được viết; công cụ `eval` và định dạng đã sẵn. |
| G6 với key OpenAlex thật, HTTP 409 thật, Unpaywall thật | Bị chặn | Cần key OpenAlex, email liên hệ thật. Đã chạy thủ công không key; hạn mức hết chỉ có test với server giả. |
| E2E Playwright | Chưa chạy | Xem 9.1. |
| Ảnh chụp tab cho PR | Chưa có | Chưa mở được giao diện trên bản dựng có endpoint mới (server đang chạy là bản cũ). |
| Kind KB `research_question/paper/report` (spec 4.6, P2) | Không làm | Tùy chọn theo spec; cần quyết định sản phẩm về KB base riêng và seed khi tạo squad. |
| Semantic Scholar citation API | Không làm | Thay bằng `chase` qua OpenAlex. |
| Squad chạy đầu cuối để kiểm tra agent tuân thủ skill và chuyển `state.json` bằng tệp đính kèm | Chưa kiểm | Cần chạy thật. |
