# Research Lab theo LitScout: nâng squad nghiên cứu thành squad nghiên cứu chuyên sâu, công cụ viết bằng Go

Trạng thái: bản nháp để duyệt (chưa triển khai). Kế hoạch đi kèm: [litscout-research-plan.md](litscout-research-plan.md).
Ngày khảo sát: 2026-11-12, trên cây làm việc có nhiều thay đổi chưa commit của người dùng (43 file đang sửa; spec này không đụng tới chúng).

Ngôn ngữ: tiếng Việt. Định danh, đường dẫn, lệnh giữ nguyên tiếng Anh.

Quy ước đánh dấu:
- **[V]** đã xác minh trực tiếp từ nguồn (đọc mã LitScout, đọc PDF bài báo, đọc mã Multica, chạy lệnh).
- **[S]** suy luận hoặc đề xuất của tôi, chưa kiểm chứng bằng chạy thử.
- Đường dẫn kèm chữ "(mới)" là đường dẫn đề xuất, chưa tồn tại. Mọi đường dẫn còn lại đã kiểm tra tồn tại.

## Quyết định đã chốt và điều chỉnh khi triển khai (2026-11-12)

Người dùng đã chốt: **Q1** Research Lab là một domain riêng, không liên quan `market_research` (không sửa nó). **Q2** mục tiêu là văn liệu học thuật. **Q8** API key OpenAlex cấu hình từ giao diện web. **Q9** bật toàn bộ nguồn (Crossref, arXiv, Semantic Scholar, toàn văn mở). **Q10** không sửa `critic`/`market_scout` dùng chung; agent và skill của Research Lab làm riêng. Các câu Q3 đến Q7 và Q11 theo khuyến nghị trong spec.

Chỗ triển khai khác với văn bản bên dưới (quyết định ưu tiên, văn bản gốc giữ nguyên để so sánh):

| Mục | Spec gốc | Đã làm |
| --- | --- | --- |
| 4.2 Agent nền | `research_lead`, `market_scout`, `critic`, `product_analyst` | `planning_lead`, `opportunity_analyst`, `validation_analyst`, `product_analyst`, `feasibility_analyst`. Không role nào dùng agent template của Market Research; test khóa điều này. Bước T7 (sửa `critic`/`market_scout`) **không làm**, theo Q10. |
| 5.1 Bố cục | `cache.go` trong `litscout` | Gói con `litscout/cache` (tránh vòng import với `source`). Có thêm `ops.go` (search, chase, doi-check, fulltext), `verify.go`, `fulltext.go`, `eval.go`. |
| 5.3 X3 Semantic Scholar | search và citation | Chỉ search. Theo trích dẫn dùng OpenAlex (`cites:` và `referenced_works`), không thêm API thứ hai. |
| 5.3 X1 DOI | chỉ Crossref | Crossref rồi `doi.org/api/handles` khi Crossref 404. Phát hiện khi chạy thật: DOI của arXiv (10.48550) thuộc DataCite nên Crossref báo 404 dù DOI hợp lệ. Trạng thái `ok_other_registry`. |
| 5.3 X6 Đọc PDF | `ledongthuc/pdf` hoặc `pdftotext` | Phát hiện khi chạy thật: `ledongthuc/pdf` mất khoảng cách giữa các từ với PDF dàn bằng TeX (X=0, W=0 cho mọi run). Dùng `pdftotext` nếu có, nếu không dùng bộ đọc Go và từ chối kết quả mất khoảng cách (`ErrWordSpacingLost`). |
| 4.6, Q4 Chuyển state | chưa chốt | `state.json` đi giữa agent bằng tệp đính kèm trên comment sub-issue (`multica issue comment add --attachment`, `multica attachment download`), mô tả trong skill `research-lab-delivery-core`. |
| Q8 Key trên web | chưa thiết kế | Bảng `research_settings` (migration 982), key niêm phong bằng secretbox (`MULTICA_RESEARCH_SECRET_KEY`, hoặc khóa dẫn xuất từ `JWT_SECRET`), `GET/PUT /api/research/settings`, `GET /api/research/credentials` cho `multica research`, tab Settings > Research sources. Spec gốc ghi "không có migration ở P0"; Q8 buộc có bảng. Bảng không có foreign key, khóa chính là `workspace_id`, không cần index riêng, và bị xóa cùng workspace. |
| Q8 Ai đọc được key | chưa thiết kế | `GET /api/research/credentials` chỉ trả key cho token tác vụ của agent (`X-Actor-Source: task_token` kèm `X-Agent-ID`, do middleware gắn) và owner/admin; thành viên thường và Cloud node credential của thành viên thường nhận 403. `GET /api/research/settings` (chỉ trạng thái, không key) mở cho mọi thành viên; `PUT` chỉ owner/admin và không cho agent. Không có `MULTICA_RESEARCH_SECRET_KEY` thì dẫn xuất từ `JWT_SECRET` và ghi WARN lúc khởi động. |
| 7.2 Golden | chỉ export | Thêm golden đối chiếu mã Python gốc cho normalize, `fuzz.ratio`, `_same`, dedup, prefilter, heuristic, dựng abstract, chuẩn hóa DOI (`testdata/python_golden/`, sinh bằng `generate.py` từ LitScout e860b12 và rapidfuzz 3.14.6). Hai khác biệt có chủ đích đã khóa bằng test: dấu tiếng Việt (Python tách từ tại chỗ dấu, Go bỏ dấu) và hai bản ghi không tiêu đề cùng tác giả đầu (Python gộp vì `fuzz.ratio("", "") = 100`, Go không gộp). |
| 5.1 Lệnh | `init/search/.../eval` | Thêm `status`, `chase`, `fulltext`. `search` nhận `--source` để thu hẹp trong số nguồn của run. |

## 0. Phạm vi khảo sát và những gì chưa xác minh được

| Việc | Kết quả |
| --- | --- |
| Repo `Hoanghung0603/LitScout` | [V] Clone `--depth 1` vào `~/.cache/litscout-src` (commit `e860b12`), đọc toàn bộ 1.468 dòng Python (src + test), `README.md`, `docs/repository-guide.md`, `pyproject.toml`. Nhánh `examples/legal-rag-run` cũng được fetch để đọc kết quả chạy thật. |
| Bài báo (Google Drive) | [V] Công cụ `web_fetch` không đọc được trang xem Drive (trả "Could not extract readable content"). Tôi tải thẳng tệp bằng `drive.google.com/uc?export=download` (HTTP 200, PDF 304 KB) rồi trích bằng `pdftotext`: 34 trang, "#27 – Xây dựng Agent tổng hợp bài báo khoa học", tác giả Hoàng Duy Hưng (AI Guru x TiniX). Đã đọc toàn văn. Hình vẽ chỉ có nhãn chữ, không xem được ảnh. |
| Bản chất bài báo | [V] Đây là **bài hướng dẫn thực hành về kiến trúc Agent** (State, Node, Tool, Router, vòng lặp có giới hạn, guardrail, provenance), không phải bài báo khoa học có thực nghiệm đối chứng. Không có benchmark, không có độ đo precision/recall. Số liệu duy nhất là một lần chạy minh họa (mục 1.7). |
| Squad tên "Research Lab" | [V] **Không tồn tại** trong mã. `grep -ri "research.lab\|research_lab\|researchlab\|research-lab"` trên `packages/`, `server/`, `apps/docs`, `docs/` không có kết quả; không có nhánh git hay commit nào nhắc. Tôi chưa truy vấn CSDL của workspace đang chạy, nên nếu bạn đã tạo tay một squad tên "Research Lab", tôi không thấy nó. Hiện trạng tôi dùng làm nền ở mục 2: template `market_research` ("Market Research"), squad `opportunity_lab` ("Opportunity Lab") và các role nghiên cứu khác. Xem câu hỏi mở Q1. |
| Chạy thử | Không chạy test hay build nào (không đổi mã sản phẩm). Lệnh đã chạy: `git clone/fetch/ls-remote`, `grep`, `ls`, `sed`, `pdftotext`, `curl`, một lượt tìm web về OpenAlex. Không chạy `git commit/push/stash/reset/checkout`. |

## 1. Tóm tắt LitScout

### 1.1 Mục tiêu và phạm vi [V]

LitScout là CLI Python 3.11 chạy **một agent, workflow có giới hạn**: tìm bài báo trên OpenAlex, loại trùng, chấm độ liên quan 0–10, chọn bài tốt nhất, tóm tắt từ abstract, rồi xuất `papers.csv` và `research_digest.md`. Không phải multi-agent, không phải ReAct tự do: LLM không chọn tool, luồng điều khiển là code.

Ngoài phạm vi (README và `docs/repository-guide.md` mục 2): tải/phân tích PDF, scrape toàn văn, vượt paywall, vector DB hoặc RAG, browser automation, multi-agent, để LLM sửa metadata hoặc tạo citation.

### 1.2 Pipeline [V]

```text
query -> planning (LLM, 1..N truy vấn)
      -> search OpenAlex (cache SQLite TTL, retry hữu hạn)
      -> normalize về PaperMetadata
      -> dedup (DOI > OpenAlex ID > tiêu đề+tác giả đầu > fuzzy>=96 CHỈ KHI cùng tác giả đầu)
      -> abstract handling (chỉ bổ sung từ bản ghi tương đương đã fetch; thiếu thì MISSING_ABSTRACT)
      -> prefilter xác định (trùng từ khóa) -> batch scoring (LLM, 1 request/vòng)
      -> quality gate (router xác định, không dùng LLM)
            selected >= min_papers           -> summarize
            selected <  min_papers, còn vòng -> refine_query (LLM) -> search lại
            selected <  min_papers, hết vòng -> summarize_available + cảnh báo
      -> summarize (LLM, mỗi bài được chọn CÓ abstract)
      -> provenance validation (dừng trước export nếu sai)
      -> papers.csv + research_digest.md
```

### 1.3 Input và output [V]

Input CLI: `--query` (bắt buộc), `--from-year`, `--min-papers` (10), `--max-papers` (20), `--max-search-rounds` (3), `--max-planned-queries` (1), `--results-per-query` (10), `--max-scoring-candidates` (= max_papers), `--threshold` (6.0), `--cache-ttl` (86400 s), `--llm-provider` (openrouter|openai|none), `--model`, `--sources` (chỉ `openalex`).

Output: `papers.csv` (13 cột: rank, title, authors, publication_date, doi, openalex_id, url, metadata_source, abstract_source, relevance_score, relevance_reason, summary, warning) và `research_digest.md` (query, thống kê, cảnh báo, từng bài với lý do chấm điểm, tóm tắt, provenance, tuyên bố giới hạn).

### 1.4 Thành phần [V]

| Thành phần (Python) | Dòng | Vai trò |
| --- | ---: | --- |
| `agent/state.py` `ResearchState` | 20 | Trạng thái một lần chạy: query, used_queries, raw/unique/scored/selected papers, search_round, warnings, quality_gate_decision |
| `agent/workflow.py` | 68 | Vòng lặp có giới hạn: dừng khi đủ bài, hết vòng, hoặc refine không ra truy vấn mới |
| `agent/nodes.py` | 139 | 9 node nhỏ; lỗi tool thành cảnh báo `SEARCH_FAILED[openalex]` thay vì crash |
| `agent/router.py` | 15 | `route_quality_gate`, thuần xác định |
| `tools/openalex.py` | 154 | HTTP + retry (429/5xx/mạng, `Retry-After`, backoff mũ, jitter, tối thiểu 5 s), cache, normalize, dựng lại abstract từ `abstract_inverted_index` |
| `models/paper.py` | 43 | `PaperMetadata` (từ API) tách khỏi `PaperAnalysis` (từ LLM), ghép bằng `ScoredPaper` |
| `services/dedup.py` | 58 | Dedup theo thứ tự bằng chứng; fuzzy chỉ là bằng chứng phụ |
| `services/relevance.py` | 279 | Ranh giới LLM: schema Pydantic, retry JSON-only, prefilter, batch scoring, fallback heuristic |
| `services/provenance.py` | 28 | Allowlist nguồn, tiêu đề/tác giả bắt buộc, phải có ít nhất một định danh truy được |
| `services/cache.py` | 47 | Cache SQLite TTL, khóa SHA-256; API key không vào khóa/giá trị |
| `exporters/*` | 130 | CSV và Markdown |
| `main.py` | 158 | CLI, nạp `.env` không ghi đè biến môi trường |

### 1.5 Nguyên tắc thiết kế cần giữ (`repository-guide.md` mục 11) [V]

1. LLM đề xuất nội dung; code điều khiển luồng.
2. Tool trả schema chung, không lộ định dạng thô của nguồn.
3. Không dùng DB làm agent state; state ≠ cache.
4. Không dùng fuzzy title làm bằng chứng duy nhất để gộp.
5. Không bịa abstract, DOI, URL, citation; không tóm tắt bài thiếu abstract.
6. Mọi vòng lặp và retry đều có giới hạn.
7. Mọi output cuối đi qua provenance validation.

### 1.6 Độ đo đánh giá [V]

LitScout **không có độ đo chất lượng nghiên cứu**. Chỉ có 21 unit test (mock API và LLM) kiểm hợp đồng: chuẩn hóa OpenAlex, dedup, abstract thiếu không bị bịa, provenance, routing, export, fallback khi LLM trả sai schema, prefilter, tái dùng điểm giữa các vòng, khóa cache không chứa API key. Chi phí được ước bằng công thức đếm request LLM: `1 planning + 1 batch scoring/vòng + 1 refinement/lần chưa đạt + 1 summary/bài`.

### 1.7 Lần chạy minh họa trong bài báo và nhánh `examples/legal-rag-run` [V]

Query "Vietnamese legal GraphRAG", `--from-year 2019 --min-papers 10 --max-papers 15 --results-per-query 30`: 3 vòng, 72 bản ghi (22+30+20), 52 sau dedup, **chỉ 45/52 được chấm** (mỗi vòng tối đa 15 ứng viên), **3 bài đạt ngưỡng** (< `min_papers` 10), kết thúc bằng `summarize_available`. Cảnh báo: 4 `MISSING_ABSTRACT` (trong đó có các mục như "Findings of the Association for Computational Linguistics: EMNLP 2023", thực chất là mục lục kỷ yếu, không phải bài), 1 `SCORING_CANDIDATES_CAPPED: 22 -> 15`, 1 `INSUFFICIENT_RESULTS_AFTER_MAX_ROUNDS`. Bài báo cũng nêu: `relevance_reason` của hai bài trả về bằng **tiếng Trung** trong khi `summary` bằng tiếng Anh, vì schema không ép ngôn ngữ.

### 1.8 Điểm lệch giữa bài báo, README và mã [V]

- Bài báo vẽ thư mục `storage/` trong cấu trúc mã; repo thực tế không có thư mục đó (cache nằm ở `services/cache.py`).
- Bài báo viết "OPENALEX_API_KEY là tùy chọn... không cần API key". Thông tin này đã **lỗi thời**: OpenAlex bắt đầu yêu cầu API key từ 2026-02-13 với hạn mức theo chi phí dùng; không có key thì vẫn gọi được nhưng hạn mức ngày rất nhỏ, có key miễn phí thì khoảng 1 USD/ngày (xem [help.openalex.org/access/pricing](https://help.openalex.org/access/pricing/) và [help.openalex.org/api/authentication](https://help.openalex.org/api/authentication/); con số chính xác nên kiểm lại lúc triển khai). Tool Go phải coi key là cấu hình gần như bắt buộc và xử lý lỗi hết hạn mức (HTTP 409/429) như một trạng thái rõ ràng, không retry vô ích.
- `Retry-After` trong mã Python chỉ parse dạng số giây (`float(...)`); header này còn có thể là ngày HTTP. Bản Go nên xử lý cả hai.
- Bài báo nói "mô hình dữ liệu dùng chung"; mã dùng `_paper_key` với `str(doi or openalex_id or (title, authors))` làm khóa tái dùng điểm. Khóa này không nhất quán với `_same` của dedup (mục 4, rủi ro R6).

## 2. Hiện trạng squad nghiên cứu trong Multica

### 2.1 Không có "Research Lab" [V]

Xem mục 0. Các thành phần nghiên cứu hiện có:

| Thành phần | Vị trí | Nội dung |
| --- | --- | --- |
| Template `market_research` | `packages/core/squads/templates.ts` (id ở dòng 37, template ở khoảng dòng 886) | Tên "Market Research", sàn "Research", 4 role: `research_lead` (leader), `scout`, `competitor`, `critic`. Danh mục `core`. Không có skill (`builtinSkills: []`), không workflow. |
| Role | `templates.ts` khoảng dòng 200–227 (`BASE_SQUAD_ROLES`) | `research_lead` → agent template `research_lead`; `scout` → `market_scout`; `competitor` → `competitor_analyst`; `critic` → `critic`. Tên agent: Research Lead, Market Scout, Competitor Analyst, Research Critic. |
| Hướng dẫn agent | `packages/core/agents/instruction-templates.ts` (id `research_lead`, `market_scout`, `competitor_analyst`, `critic` từ khoảng dòng 480–565) | Văn bản hướng dẫn ngắn, tập trung thị trường/đối thủ. Quy tắc cốt lõi: mỗi claim có link + ngày; critic chặn proposal còn phản đối blocking; "không có proposal tuần này" là kết quả hợp lệ. |
| Hướng dẫn leader squad | `templates.ts` `MARKET_RESEARCH_INSTRUCTIONS` + `RESEARCH_COMMON_INSTRUCTIONS` (khoảng dòng 746–795) | Ủy quyền bằng sub-issue, giai đoạn 1 (scout + competitor) rồi critic, tối đa 3 proposal/tuần qua `multica proposal create --squad "Market Research"`. |
| Routine | Trong template | "Market scan, week of {{date}}", cron `0 8 * * 1`. |
| Charter | Trong template | Mục tiêu, trách nhiệm, đầu ra (`weekly_report`, `market_proposals`), ranh giới (không viết mã, không liên hệ ngoài workspace). |
| Công ty mẫu | `packages/core/squads/company-template.ts` (`COMPANY_SQUAD_KEYS`: `market_research`, `opportunity_lab`, `competitor_intelligence`, ...) | Sàn "research" gồm Market Research (3 role: lead, scout, critic), Opportunity Lab (lead, analyst, critic; cron `0 8 * * 2`), Competitor Intelligence. Dữ liệu được sinh sang JSON cho Go bằng `scripts/generate-company-template.mjs`. |
| Role nghiên cứu ở squad khác | `plan_researcher` (Planning), `cs_researcher` (Content Studio, `content-studio.ts`), `sales_scout` | Cùng dùng agent template `market_scout` làm nền. |
| Test liên quan | `packages/core/squads/templates.test.ts` (khối "market research template"), `company-template.test.ts`, `quickstart.test.ts`, `packages/core/agents/instruction-templates.test.ts` | Khóa cứng: danh sách role, cron, chuỗi như "after the critic has reviewed them". |
| i18n | `packages/views/locales/{en,es,fr,ja,ko,zh-Hans,zh-Hant}/modals.json` (khóa `templates.market_research`), `issues.json` (`company_cards.market_research`) | Thẻ quickstart dịch theo `id`. |
| Cơ chế skill | `packages/core/squads/sdlc-skills/<tên>/SKILL.md` → sinh `sdlc-skills-data.ts` bằng `pnpm generate:sdlc-skills`; danh sách tên skill theo squad ở `templates.ts` (`PLANNING_SKILL_NAMES`, `SALES_SKILL_NAMES`) | Bộ lọc `NON_SDLC_SKILLS` loại skill theo **tiền tố** (`planning-`, `sales-`...) khỏi squad SDLC. Skill có tiền tố mới không được liệt kê sẽ bị squad SDLC import nhầm. |
| Mẫu squad nhiều role có KB và công cụ Go | `sales_loop` (`sales-agent-instructions.ts`, 7 skill `sales-*`), `server/internal/salescollect/` (3.295 dòng), `server/cmd/multica/cmd_sales.go` (`multica sales probe|collect`), `server/internal/service/knowledge_sales_kinds.go` (kind KB), `docs/engineering/sales-loop-squad.md` | Là tiền lệ trực tiếp cho thiết kế công cụ Go ở mục 4: collector chạy **không dùng model**, in JSON, agent ghi kết quả vào KB. |
| Mẫu squad tách role + gate | `content_studio` (`content-studio.ts`, `docs/engineering/content-studio-squad.md`) | Role có hướng dẫn riêng ghi đè agent template nền qua `instructionsFor` (`quickstart.ts` khoảng dòng 200–223). Mẫu tốt nhất để thêm role mà không thêm id vào union `AgentInstructionTemplate`. |

Luồng hiện tại của Market Research [V]: routine mở issue tuần → leader viết lại phạm vi → sub-issue cho scout + competitor (giai đoạn 1) → báo cáo → sub-issue cho critic → leader nộp 0–3 proposal. Một vòng, không lặp, không có công cụ thu thập có cấu trúc: scout "tìm và đọc nguồn" bằng khả năng web của runtime, ghi claim kèm link và ngày.

### 2.2 Spec khác có thể va chạm [V]

`docs/specs/template-registry/{spec,plan}.md` (bản nháp, chưa duyệt) đề xuất dời nội dung template ra thư mục `templates/` ở gốc repo. Nếu nó được duyệt trước khi Research Lab triển khai, vị trí file skill/template trong plan này phải đổi theo. Xem Q7.

## 3. Khoảng cách so với mục tiêu "nghiên cứu chuyên sâu theo LitScout"

"Chuyên sâu theo LitScout" cần hiểu đúng: LitScout là một **khung kỷ luật** (tách dữ liệu nguồn khỏi nội dung LLM, vòng lặp có giới hạn, router xác định, provenance gate), không phải một hệ nghiên cứu sâu. Squad mục tiêu = kỷ luật của LitScout + các mở rộng cần thiết để thực sự "sâu" (đánh dấu **[mở rộng]**, không có trong LitScout).

| # | Khoảng cách | Hiện trạng Market Research | Mục tiêu |
| --- | --- | --- | --- |
| G1 | Nguồn học thuật có cấu trúc | Chỉ web tự do, không có công cụ tìm paper | Công cụ tìm OpenAlex trả `Paper` chuẩn hóa (LitScout) + Crossref/arXiv/Semantic Scholar **[mở rộng]** |
| G2 | Tách metadata nguồn khỏi nội dung LLM | Không có: agent tự gõ claim, link, ngày | `PaperMetadata` chỉ do công cụ ghi; agent chỉ ghi score/reason/summary/claim kèm `paper_id` |
| G3 | Vòng lặp nhiều vòng có trần | Một vòng | Giai đoạn theo vòng, router xác định (`multica research gate`), trần `max_rounds` |
| G4 | Dedup có kỷ luật bằng chứng | Không | DOI > ID nguồn > tiêu đề+tác giả đầu > fuzzy có điều kiện |
| G5 | Sàng lọc độc lập | Scout tự đánh giá nguồn | Role Screener tách khỏi Scout; chấm theo tiêu chí viết sẵn trong protocol |
| G6 | Xử lý thiếu abstract | Không | Không tóm tắt khi thiếu abstract; cờ `SCORED_FROM_TITLE_ONLY` |
| G7 | Cổng provenance trước khi xuất | Critic kiểm bằng đọc | Cổng cơ học (Go) + kiểm ngữ nghĩa (Auditor) |
| G8 | Kiểm soát chi phí | Không có trần | Trần số vòng, số truy vấn, số ứng viên chấm, số bài tóm tắt, ghi vào protocol; ước lượng request trước khi chạy |
| G9 | Cache giữa các lần chạy | Không | Cache HTTP có TTL, khóa không chứa bí mật |
| G10 | Báo cáo có cấu trúc, đối chiếu được | Comment tự do | `papers.csv` + `research_digest.md` + `state.json`, cột cố định |
| G11 | Tổng hợp chéo bài (chủ đề, khoảng trống, mâu thuẫn) | Không; LitScout cũng không có **[mở rộng]** | Role Synthesizer, bảng bằng chứng, mọi mệnh đề kèm `paper_id` |
| G12 | Theo vết trích dẫn (citation chasing) | Không; LitScout không có **[mở rộng]** | Vòng 3 đi theo `referenced_works` / `cited_by` của các bài hạt giống |
| G13 | Đánh giá chất lượng | Không; LitScout không có **[mở rộng]** | Bộ chỉ số và tập vàng (mục 7) |
| G14 | Nhất quán ngôn ngữ báo cáo | Không | Trường `language` trong protocol, kiểm tra tự động |
| G15 | An toàn dữ liệu không tin cậy | Có quy tắc "tài liệu là dữ liệu" | Giữ; thêm: abstract/tiêu đề từ API luôn bọc trong khối dữ liệu khi đưa vào prompt |
| G16 | Lọc loại tài liệu | Không; LitScout cũng thiếu (mục kỷ yếu lọt vào) **[mở rộng]** | Lọc `type` (OpenAlex `type`/`is_paratext`) trước khi chấm |

## 4. Thiết kế nâng cấp squad

### 4.1 Quyết định chính [S]

1. **Giữ `market_research` nguyên id và hành vi** (nhiều test và công ty mẫu phụ thuộc; mục đích khác: quét thị trường theo tuần). **Thêm template mới `research_lab`** tên "Research Lab", danh mục `core`, sàn "Research". Đây là cách thực hiện "nâng cấp Research Lab" mà không phá squad đang chạy.
2. **Theo mẫu `content-studio.ts`**: file mới `packages/core/squads/research-lab.ts` giữ role, hướng dẫn role, template; role dùng agent template nền có sẵn, hướng dẫn riêng ghi đè qua `instructionsFor`. Không thêm id vào union `AgentInstructionTemplate`.
3. **Quy trình nằm trong skill** (`research-lab-*`), hướng dẫn role ngắn, theo mẫu `sales-agent-instructions.ts`.
4. **Công cụ Go không gọi LLM.** Mọi bước LLM (lập truy vấn, chấm điểm, refine, tóm tắt, tổng hợp) do agent của squad làm, vì runtime của agent đã là LLM. Go chỉ làm phần xác định: tìm kiếm, chuẩn hóa, dedup, prefilter, kiểm schema và provenance, router, export. Giống `multica sales collect` ("không dùng model"). Điều này cũng loại nhu cầu giữ khóa LLM trong CLI.
5. **State của một lần chạy = tệp `state.json`** do công cụ Go đọc/ghi (đối ứng `ResearchState`); không đưa vào Postgres ở P0 (đúng nguyên tắc state ≠ DB). Kết quả cuối lưu thành tài liệu KB (mục 4.6).

### 4.2 Roster đề xuất (5 role)

Giữ 4 role cũ để tái dùng mô hình quen thuộc, nhưng đổi chức năng cho đúng nghiên cứu văn liệu. Tên agent mới có tiền tố "Lab" để không đụng agent "Market Scout" đang dùng lại theo tên (quickstart tái dùng agent chưa archive theo tên).

| Role key | Tên agent | Agent template nền | Thêm/bớt/sửa | Vai trò |
| --- | --- | --- | --- | --- |
| `lab_lead` | Research Lab Lead | `research_lead` | **Thêm** (kế thừa nền) | Viết protocol nghiên cứu, ủy quyền theo sub-issue, đọc kết quả `gate`, xác minh báo cáo, báo cáo cuối. Không tìm, không chấm, không tóm tắt. |
| `lab_scout` | Literature Scout | `market_scout` | **Thêm** (sửa mục đích) | Lập truy vấn (LLM), chạy `multica research search/dedup/prefilter`, ghi `candidates.json`. Không chấm điểm. |
| `lab_screener` | Relevance Screener | `critic` | **Thêm** | Chấm 0–10 theo tiêu chí trong protocol, chỉ từ title + abstract, ghi lý do bằng ngôn ngữ báo cáo; gắn cờ title-only. Không thấy điểm tự động của heuristic để tránh neo. |
| `lab_synthesizer` | Evidence Synthesizer | `product_analyst` | **Thêm** | Tóm tắt từng bài được chọn (chỉ từ abstract), dựng bảng bằng chứng và tổng hợp chéo bài: chủ đề, mâu thuẫn, khoảng trống. Mọi mệnh đề kèm `paper_id`. |
| `lab_auditor` | Citation Auditor | `critic` | **Thêm** | Chạy cổng cơ học `multica research verify` (+ `doi-check`), rồi lấy mẫu kiểm từng claim với abstract, tìm bằng chứng phản bác, đánh giá thiên lệch phạm vi. Verdict PASS / PASS WITH CONDITIONS / REQUEST CHANGES. |

Bớt: không dùng lại `competitor` (không liên quan văn liệu). Gộp "planner" vào Scout và Lead (lập truy vấn đầu: Scout; refine: Scout theo yêu cầu của Lead) để không phình roster. Nếu muốn tách, xem Q3.

Agent có sẵn được điều chỉnh (đề xuất nhỏ, đánh dấu tùy chọn, làm sau cùng):
- `critic` (dùng chung với `market_research`, `opportunity_lab`): thêm một đoạn "Citation check": khi claim trích paper, chạy `multica research verify` nếu có; không đổi các câu mà test đang khóa. Việc này đụng agent dùng chung, nên tách thành bước riêng (plan P-T7) và cần duyệt.
- `market_scout`: thêm một dòng cho phép dùng `multica research search` khi câu hỏi cần nền học thuật. Cũng thuộc P-T7.

### 4.3 Luồng nhiều vòng [S]

Mỗi giai đoạn là một nhóm sub-issue theo `--stage N`, đúng cơ chế mà `RESEARCH_COMMON_INSTRUCTIONS` đã dùng.

```text
Stage 0  Lead: viết protocol.md (câu hỏi, phạm vi, tiêu chí gồm/loại, ngôn ngữ báo cáo,
         from_year, min/max_papers, threshold, max_rounds, trần chi phí) + `multica research init`
Vòng r (r = 1..max_rounds), mỗi vòng:
  Stage A  Scout   : lập truy vấn -> `search` -> `dedup` -> `prefilter` -> candidates.json
  Stage B  Screener: chấm candidates -> scores.json -> `ingest-scores`
  Stage C  Lead    : `gate` -> {summarize | refine_query | summarize_available}
                      refine_query: Scout nhận title cận ngưỡng + used_queries, quay lại Stage A (r+1)
                      r = 3 (tùy chọn, [mở rộng]): citation chasing từ các bài top-k
Stage D  Synthesizer: tóm tắt + bảng bằng chứng + tổng hợp -> summaries.json -> `ingest-summaries`
Stage E  Auditor : `verify` (cổng cơ học) + kiểm mẫu ngữ nghĩa -> verdict
         REQUEST CHANGES -> Stage D, tối đa 2 lượt, rồi dừng và báo cáo vấn đề mở
Stage F  Lead    : `export` -> papers.csv, research_digest.md, state.json; lưu KB; báo cáo cuối
```

Trần và điều kiện dừng (đều ghi trong protocol, đúng nguyên tắc "mọi vòng lặp có giới hạn"): `max_rounds` (mặc định 3, tối đa 5), Scout không ra truy vấn mới → dừng với cảnh báo `WORKFLOW_STOPPED_NO_NEW_QUERIES`, Auditor tối đa 2 lượt trả lại, trần số bài tóm tắt. Lead **không** tự quyết tiếp tục hay dừng: nó chạy `multica research gate` và làm theo `decision`.

Ước lượng chi phí (số lượt chạy agent, không phải tiền): `1 (Lead) + vòng × (1 Scout + 1 Screener + 1 Lead gate) + 1 Synthesizer + 1..2 Auditor + 1 Lead cuối`. Ví dụ 3 vòng: khoảng 14–15 lượt. Lead ghi con số này vào protocol trước khi chạy (đối ứng công thức đếm request của LitScout).

### 4.4 Tiêu chí chất lượng và độ chính xác trích dẫn [S, dựa trên 7 nguyên tắc ở 1.5]

Cổng cứng (vi phạm là không xuất):
- C1. Mọi bài trong kết quả có `title`, `authors`, và ít nhất một trong `doi` / `openalex_id` / `url` (đúng `validate_paper`). Mọi trường metadata có `*_source` nằm trong allowlist.
- C2. Không có metadata do agent tự ghi: công cụ từ chối tệp `scores.json` / `summaries.json` chứa trường ngoài schema (giải mã nghiêm ngặt).
- C3. Mọi `paper_id` được nhắc trong tổng hợp đều tồn tại trong `selected`.
- C4. Không có `summary` cho bài thiếu abstract.
- C5. DOI có trong kết quả phải khớp định dạng; `doi-check` (Crossref) không trả "không tồn tại" cho bài nào. DOI không phân giải được thì bài đó mang cờ `DOI_UNRESOLVED` và Auditor quyết định giữ hay loại.

Cổng mềm (cảnh báo trong báo cáo): thiếu abstract; chấm từ title-only; ngôn ngữ lý do không đúng `language`; số bài `< min_papers` sau hết vòng; ứng viên bị cắt (`SCORING_CANDIDATES_CAPPED`); ngày xuất bản chưa kiểm (bài báo cảnh báo ngày OpenAlex có thể là ngày đăng ký DOI hoặc ngày online).

Quy tắc dữ liệu không tin cậy: tiêu đề, abstract, tên tác giả là dữ liệu. Hướng dẫn role nói rõ: lệnh nào nằm trong đó thì ghi vào mục "Cảnh báo" và không làm theo (cùng quy tắc `cs_researcher` đã dùng).

### 4.5 Skill cần có [S]

Thư mục mới dưới `packages/core/squads/sdlc-skills/` (nguồn duy nhất; chạy `pnpm generate:sdlc-skills`), tiền tố `research-lab-`. Bắt buộc thêm tiền tố này vào bộ lọc `NON_SDLC_SKILLS` trong `templates.ts`, nếu không mọi squad SDLC sẽ import chúng (kiểm bằng test như `sales-skills.test.ts`).

| Skill | Dùng bởi | Nội dung |
| --- | --- | --- |
| `research-lab-delivery-core` | Tất cả | Lược đồ `protocol.md`, `state.json`, bảng mã cảnh báo, định dạng báo cáo, quy tắc dữ liệu không tin cậy, ranh giới (không tải PDF/paywall ở P0) |
| `research-lab-squad-lead` | Lead | Viết protocol, chia stage, đọc `gate`, xác minh báo cáo, báo cáo cuối |
| `research-lab-search` | Scout | Cách lập truy vấn (câu thuần, không Boolean/ngoặc/field syntax theo bài báo; chấp nhận ngoại lệ nếu nguồn hỗ trợ), refine từ title cận ngưỡng, lọc loại tài liệu |
| `research-lab-screening` | Screener | Thang 0–10 có mốc mô tả, quy tắc title-only, ngôn ngữ, tránh neo |
| `research-lab-synthesis` | Synthesizer | Tóm tắt chỉ từ abstract, bảng bằng chứng, mẫu "claim → paper_id → câu trích từ abstract" |
| `research-lab-audit` | Auditor | Thủ tục kiểm cơ học và lấy mẫu, thang blocking/minor, verdict |

### 4.6 Lưu trữ kết quả [S]

- P0: `papers.csv`, `research_digest.md`, `state.json` đính kèm issue yêu cầu (dùng `multica attachment`, cần kiểm tra thực tế khả năng của lệnh này khi triển khai: [V] tệp `cmd_attachment.go` tồn tại, chưa đọc chi tiết) và bản Markdown lưu vào KB bằng `multica knowledge create`.
- P2 (tùy chọn): kind KB `research_question`, `research_paper`, `research_report` tại `server/internal/service/knowledge_research_kinds.go` (mới), theo mẫu `knowledge_sales_kinds.go`, để lọc/sắp xếp bài đã đọc và tái dùng giữa các lần chạy. Không cần migration vì kind là cấu hình mã.
- Không tạo bảng Postgres cho run ở P0/P2. Nếu về sau muốn (Q5), áp dụng các ràng buộc ở mục 6.

## 5. Công cụ Python → Go

### 5.1 Bố cục triển khai [S]

```text
server/internal/litscout/                 (mới)  tên gói: litscout
  paper/paper.go                                 Paper, Metadata, Analysis, Scored (đối ứng models/paper.py)
  source/source.go                               interface Source { Name(); Search(ctx, Query) ([]paper.Metadata, error) }
  source/openalex.go                             OpenAlex (đối ứng tools/openalex.py)
  source/crossref.go, arxiv.go, s2.go            [mở rộng], thêm sau
  state.go                                       State (đối ứng agent/state.py), Load/Save atomic
  router.go                                      Route (đối ứng agent/router.py)
  dedup.go, abstract.go, prefilter.go            đối ứng services/dedup, abstract, phần xác định của relevance
  provenance.go                                  đối ứng services/provenance
  ingest.go                                      kiểm schema scores/summaries do agent ghi
  cache.go                                       cache tệp TTL (đối ứng services/cache)
  export_csv.go, export_md.go                    đối ứng exporters/
  *_test.go, testdata/                           bảng test + bản ghi OpenAlex thật đã ghi lại
server/cmd/multica/cmd_research.go        (mới)  nhóm lệnh `multica research`, đăng ký vào rootCmd như cmd_sales.go
```

Lý do chọn `server/internal/` (không phải `server/pkg/`): cùng vị trí với `salescollect`; `pkg/` dành cho thứ cần import từ ngoài module. [V] `server/internal/service/` chứa logic cần Postgres/handler, không cần cho công cụ CLI chạy không DB.

Lý do chọn CLI thay vì API handler: agent chạy trên máy daemon và gọi `multica ...` (như `multica sales collect`, `multica knowledge search`). Handler chỉ cần khi phải giữ bí mật phía máy chủ; khóa OpenAlex để ở biến môi trường của máy daemon (`OPENALEX_API_KEY`), giống LitScout. Không có handler, không có route mới, không đụng `reserved_slugs`.

Bề mặt lệnh (mỗi lệnh in JSON ra stdout, lỗi ra stderr, mã thoát khác 0 khi không đọc được hoặc vi phạm cổng):

```text
multica research init   --dir D --query Q [--from-year Y --min-papers N --max-papers N --max-rounds N --threshold T --language L]
multica research search --dir D --query Q [--source openalex --limit N]      # round++ , used_queries, raw_papers, SEARCH_FAILED[...] thành cảnh báo
multica research dedup  --dir D                                              # unique_papers + MISSING_ABSTRACT
multica research prefilter --dir D --limit N                                 # candidates.json, SCORING_CANDIDATES_CAPPED
multica research ingest-scores --dir D --file scores.json                    # kiểm schema nghiêm ngặt, chọn theo threshold
multica research gate   --dir D                                              # {"decision":"summarize|refine_query|summarize_available", ...}
multica research ingest-summaries --dir D --file summaries.json
multica research verify --dir D                                              # provenance; thoát != 0 nếu sai
multica research doi-check --dir D                                           # [mở rộng] Crossref
multica research export --dir D --out OUT                                    # papers.csv, research_digest.md
multica research eval   --dir D --gold gold.json                             # [mở rộng] chỉ số mục 7
```

### 5.2 Bảng ánh xạ từng công cụ

Cột "Khác biệt hành vi" là chỗ dễ sai nhất; mỗi dòng cần một golden test so với đầu ra Python đã ghi lại (mục 7, E1).

| # | Công cụ Python (LitScout) | Chức năng | Thư viện Python | Tương đương Go | Vị trí | Expose | Rủi ro / khác biệt hành vi |
| --- | --- | --- | --- | --- | --- | --- | --- |
| T1 | `tools/openalex.py` `search` + `_request` | Gọi `GET /works` với `search`, `per_page` ≤ 100, `select`, lọc năm; retry 429/5xx/mạng | `httpx` | `net/http` + `context` + `encoding/json`; client riêng theo mẫu `salescollect/fetch.go` (UA, interval theo host, trần body, timeout); [V] `pkg/httptransport` có sẵn | `litscout/source/openalex.go` | `multica research search` | (a) Key OpenAlex nay gần bắt buộc, cần lỗi riêng cho hết hạn mức (mục 1.8). (b) `Retry-After` dạng ngày HTTP: Python bỏ sót, Go phải parse `http.ParseTime`. (c) Jitter/backoff dùng `math/rand/v2`; test phải tiêm đồng hồ và nguồn ngẫu nhiên để không ngủ thật. (d) Python tạm khóa nguồn sau 429 liên tục (`_rate_limited`); giữ ở mức một lần chạy lệnh, lưu cờ vào `state.json` vì mỗi lệnh Go là một tiến trình mới. |
| T2 | `OpenAlexSearchTool.normalize` | Chuẩn hóa work → `PaperMetadata`; `openalex_id` lấy phần cuối URL; `url` ưu tiên `primary_location.landing_page_url` | `dict.get` | struct có tag JSON, con trỏ cho trường có thể null; `time.Parse("2006-01-02")` thay `datetime.fromisoformat` | `source/openalex.go` | — | JSON `null` ở mảng/đối tượng lồng nhau cần con trỏ hoặc `json.RawMessage`; lỗi một work không được làm hỏng cả trang (giữ hành vi "bỏ qua bản ghi hỏng + log"). `publication_date` có thể là chuỗi rỗng. |
| T3 | `restore_abstract` | Dựng abstract từ `abstract_inverted_index` | thuần Python | `map[string][]int`, gom cặp (vị trí, từ), `sort.Slice`, `strings.Join` | `source/openalex.go` | — | Python sắp theo tuple `(position, word)`, nên khi hai từ cùng vị trí thì theo thứ tự chữ; Go phải so sánh theo cùng cặp để kết quả giống. Khác với Python là Go không đảm bảo thứ tự duyệt map, nên bắt buộc sắp xếp tường minh. |
| T4 | `models/paper.py` (Pydantic) | Kiểu dữ liệu, chuẩn hóa DOI, ràng buộc `0 ≤ score ≤ 10` | `pydantic` v2 | struct + hàm `Validate()` thủ công; `json.Decoder.DisallowUnknownFields()`; chuẩn hóa DOI bằng `strings.ToLower` + `TrimPrefix` | `litscout/paper` | — | **Không có tương đương trực tiếp**: Go không có xác thực khai báo, không ép kiểu tự động, không sinh JSON Schema từ struct (không cần vì agent không gọi structured-output API). Thay thế: kiểm tra tường minh trong `ingest.go` + test bảng. Bản Go nên chuẩn hóa thêm tiền tố `dx.doi.org` (Python bỏ sót). |
| T5 | `services/dedup.py` | Gộp trùng theo bằng chứng có thứ tự | `rapidfuzz` (`fuzz.ratio`), `unicodedata`, `re` | Tự viết: tỉ lệ Indel dựa trên LCS theo `rune` (không dùng khoảng cách Levenshtein); chuẩn hóa bằng `golang.org/x/text/unicode/norm` (NFKD) và `x/text/cases` (`Fold`) | `litscout/dedup.go` | `multica research dedup` | (a) **`rapidfuzz.fuzz.ratio` là độ tương đồng Indel chuẩn hóa (chỉ chèn/xóa), không phải Levenshtein**; dùng thư viện Levenshtein sẽ lệch quanh ngưỡng 96 [S, cần xác nhận bằng golden test so với rapidfuzz thật]. (b) Python `\w` trong `re` nhận Unicode; **`\w` của Go (RE2) chỉ ASCII**, phải viết `[\p{L}\p{N}_]`. (c) `casefold()` ≠ `ToLower` (ví dụ "ß"); dùng `cases.Fold()`. (d) [S, suy từ ngữ nghĩa Unicode của `\w` trong Python, chưa chạy kiểm] Sau NFKD, dấu kết hợp (Mn) có thể bị `[^\w\s]` biến thành khoảng trắng, làm "café" thành "cafe " hoặc "caf e"; Go nên **loại dấu** thay vì tách từ. Hai phía chỉ cần nhất quán nội bộ, nhưng điều này làm đầu ra khác Python với tiếng Việt, nên golden test cần dùng bản ghi có dấu và chấp nhận khác biệt có chủ đích. |
| T6 | `services/abstract.py` | Bổ sung abstract từ bản ghi tương đương đã fetch | thuần Python | hàm dùng lại `same()` của dedup | `litscout/abstract.go` | trong `dedup` | Không gọi mạng, không gọi LLM. Giữ nguyên ràng buộc đó. |
| T7 | `nodes._paper_key` | Khóa để tái dùng điểm giữa các vòng | thuần Python | hàm `Key(p) string` | `litscout/state.go` | — | Python dùng `str(tuple)` làm khóa và bản ghi có DOI hay không thì khóa khác nhau, nên cùng một bài vào vòng sau có DOI có thể bị chấm lại (rủi ro R6). Go dùng một khóa chuẩn (ưu tiên DOI chuẩn hóa, rồi ID nguồn, rồi băm tiêu đề+tác giả đầu) và một bảng alias sau dedup. |
| T8 | `services/relevance.py` prefilter + `_heuristic` | Xếp hạng từ khóa xác định để giới hạn số ứng viên | `re`, `sorted` | `regexp` với `[\p{L}\p{N}_]+`; `sort.SliceStable` (giữ thứ tự bản ghi bằng nhau như `sorted` ổn định của Python) | `litscout/prefilter.go` | `multica research prefilter` | Trọng số `title_hits*3 + abstract_hits` và so khớp chuỗi con giữ nguyên để golden test khớp. Cân nhắc loại từ dừng sau, ngoài phạm vi sao chép. Tiếng Việt: từ có dấu, so khớp chuỗi con sẽ dễ khớp sai; ghi vào Q6. |
| T9 | `relevance.py` planning/scoring/refine/summary (LLM) | Lập truy vấn, chấm điểm, refine, tóm tắt | `openai` (OpenRouter), `pydantic` JSON-schema | **Không port vào Go.** Do agent của squad làm. Go chỉ kiểm tệp kết quả (`ingest.go`). Nếu sau này cần chạy trong tiến trình Go: [V] `server/pkg/llm` (dựa `openai-go`, có `GenerateJSON`) đã có, nhưng chạy phía máy chủ, không có trên máy daemon. | `litscout/ingest.go` | `multica research ingest-scores`, `ingest-summaries` | **Không có tương đương Go trực tiếp** cho "structured output theo Pydantic và retry JSON-only"; thay bằng hợp đồng tệp JSON + kiểm nghiêm ngặt + lỗi nói rõ trường sai để agent sửa. Hệ quả: chất lượng chấm điểm phụ thuộc skill `research-lab-screening`, không phải schema. |
| T10 | `agent/router.py` | Quyết định bước kế | thuần Python | hàm thuần `Route(state, minPapers) Decision` | `litscout/router.go` | `multica research gate` | Dễ port nhất. Bảng test 3 nhánh + biên. Bản Go thêm nhánh `stop_no_new_queries` rõ ràng thay vì chỉ thêm cảnh báo trong workflow. |
| T11 | `agent/workflow.py` + `nodes.py` | Điều phối vòng lặp | Python | **Không port thành vòng lặp trong một tiến trình.** Vòng lặp trở thành chuỗi sub-issue do Lead điều khiển; mỗi node thành một lệnh con. | `cmd_research.go` | các lệnh con | Mất tính nguyên tử của "một lần chạy" (Python chạy trọn vẹn trong một tiến trình). Giảm rủi ro bằng `state.json` ghi nguyên tử (ghi tệp tạm rồi `os.Rename`) và trường `schema_version`. |
| T12 | `services/provenance.py` | Cổng nguồn gốc dữ liệu | thuần Python | hàm `Validate(p) []string`, allowlist nguồn cấu hình được | `litscout/provenance.go` | `multica research verify` | Mở rộng allowlist khi thêm nguồn (Crossref, arXiv...). Mã lỗi giữ nguyên chuỗi (`INVALID_METADATA_SOURCE`...) để bộ test và skill dùng chung. |
| T13 | `services/cache.py` | Cache HTTP có TTL | `sqlite3` | **Khuyến nghị: tệp JSON theo khóa SHA-256 dưới `os.UserCacheDir()/multica/litscout/`, ghi nguyên tử, TTL theo `mtime`.** `crypto/sha256`, `encoding/json` | `litscout/cache.go` | cờ `--cache-ttl`, `--no-cache` | [V] `go.mod` không có thư viện SQLite; thêm `modernc.org/sqlite` là phụ thuộc mới mà `AGENTS.md` yêu cầu khai báo và cân nhắc. Tệp theo khóa tránh khóa DB khi nhiều agent chạy song song trên một máy. Giữ quy tắc: khóa và giá trị **không bao giờ chứa API key** (có test như `test_cache_key_does_not_contain_api_key`). `json.Marshal` của `map` sắp khóa theo thứ tự nên khóa ổn định; không cần trùng khóa với Python. |
| T14 | `exporters/csv_exporter.py` | Xuất CSV 13 cột | `csv.DictWriter` | `encoding/csv` | `litscout/export_csv.go` | `multica research export` | `DictWriter` mặc định dùng `\r\n`; `csv.Writer` của Go dùng `\n` (đặt `UseCRLF` nếu cần). Tùy chọn thêm BOM UTF-8 để Excel đọc đúng tiếng Việt. Đồng thời chống "CSV injection" (ô bắt đầu bằng `= + - @` do abstract/tên bài do bên ngoài cung cấp): LitScout không làm việc này. |
| T15 | `exporters/markdown_exporter.py` | Báo cáo Markdown | f-string | `text/template` | `litscout/export_md.go` | `multica research export` | Escape Markdown cho tiêu đề/tác giả từ nguồn; golden test so với `research_digest.md` của nhánh `examples/legal-rag-run`. |
| T16 | `main.py` | CLI và nạp `.env` | `argparse`, tự đọc `.env` | `spf13/cobra` ([V] đã có trong `go.mod`); cờ như `cmd_sales.go` | `cmd/multica/cmd_research.go` | — | Không nạp `.env` (CLI Multica dùng biến môi trường/cấu hình của nó); không thêm thư viện dotenv. |
| T17 | `tests/unit/test_research_agent.py` (21 test) | Hợp đồng hành vi | `pytest`, mock | `testing` + bảng test, `httptest.Server` cho OpenAlex, `testdata/` | `litscout/*_test.go` | `make test` | Test trong `AGENTS.md`: gần mã, không cần Postgres vì gói không dùng DB; không chạy CLI agent thật. |

### 5.3 Công cụ mở rộng [mở rộng, không có trong LitScout]

| # | Công cụ | Chức năng | Go | Ghi chú |
| --- | --- | --- | --- | --- |
| X1 | Crossref | Phân giải DOI, xác minh metadata (`doi-check`) | `net/http` + JSON; thêm `mailto` vào query để vào "polite pool" | Cần địa chỉ liên hệ cấu hình được; không gán cứng email. |
| X2 | arXiv | Tìm preprint | `encoding/xml` đọc Atom; tôn trọng giới hạn của arXiv (khoảng 1 request / 3 s) | Hợp ngành ML. |
| X3 | Semantic Scholar Graph API | Lấy citation / reference, tóm tắt | `net/http`; key tùy chọn | Có thể thay bằng `referenced_works` / `cited_by_api_url` của chính OpenAlex để tránh thêm nguồn. |
| X4 | Citation chasing | Vòng 3: đi theo tham chiếu của bài hạt giống | trên OpenAlex `filter=cites:<id>` và `referenced_works` | Cần kiểm tra bằng thử thực tế các tham số; chưa xác minh trong khảo sát này. |
| X5 | Unpaywall / toàn văn mở | Tìm bản PDF truy cập mở hợp pháp | `net/http` | **Tắt mặc định**; chỉ khi bật rõ ràng trong protocol; không vượt paywall. |
| X6 | Đọc PDF | Trích văn bản từ PDF truy cập mở | [V] `github.com/ledongthuc/pdf` đã có trong `go.mod` (dùng ở `service/knowledge_upload_headings.go`); hoặc gọi `pdftotext` nếu có | Chất lượng trích xuất bố cục hai cột và công thức thấp hơn công cụ Python (PyMuPDF, pdfplumber); đây là phần **không có tương đương Go ngang chất lượng**. Thay thế: ghi nhận giới hạn, chỉ dùng cho tóm tắt thô, cờ `FULLTEXT_LOW_CONFIDENCE`. Để pha cuối (P3). |
| X7 | Embedding / RAG | — | — | **LitScout không có** (ngoài phạm vi, README mục 2). Không port. Dùng `multica knowledge search` có sẵn để tránh lặp lại điều công ty đã biết; không thêm vector DB. |

## 6. Ràng buộc kiến trúc của repo (`AGENTS.md`) áp dụng vào thiết kế

| Ràng buộc | Cách tuân thủ |
| --- | --- |
| Go backend ở `server/` | Mọi mã Go mới ở `server/internal/litscout/` và `server/cmd/multica/cmd_research.go`. |
| `packages/core` không UI, không `localStorage`, `process.env` | `research-lab.ts` chỉ chứa dữ liệu, chuỗi hướng dẫn và template; không import UI. |
| `packages/views` không định nghĩa store, không `next/*` | Phần giao diện chỉ thêm chuỗi dịch (thẻ quickstart); không thêm component ở P0. |
| Không thêm foreign key, cascade | P0/P2 không có bảng mới. Nếu chọn lưu run trong Postgres (Q5): không FK, dọn phụ thuộc trong mã. |
| Index trong migration dùng `CREATE INDEX CONCURRENTLY`, mỗi index một tệp một câu lệnh | Chỉ áp dụng nếu Q5 chọn bảng; số migration kế tiếp sau `981` [V] (`server/migrations/981_publish_attempt_queue_idx.*`; kiểm lại số cuối cùng ngay trước khi tạo). |
| SQL qua sqlc, chạy `make sqlc` | Chỉ nếu có bảng. |
| JSON UI-consumed qua zod `parseWithFallback` | P0 không thêm endpoint hay JSON UI mới. Nếu UI sau này đọc `state.json`/báo cáo, thêm schema zod + test malformed. |
| API tương thích ngược | Không đổi API. Lệnh CLI mới là thêm mới, không đổi lệnh có sẵn. |
| Agent CLI mặc định test không được resolve/chạy CLI người dùng | Test Go của gói dùng `httptest`, không gọi OpenAlex thật, không gọi agent CLI. Kiểm tra thật (nếu có) phải gắn cờ riêng và xin phép. |
| Tên mới trong `scripts/agent-cli-command-names.txt` | Chỉ liên quan agent CLI mặc định; không áp dụng. |
| UI copy: bỏ mô tả không cần | Thẻ quickstart có `name` + `description` ngắn, như thẻ hiện có; dịch đủ 7 locale. |
| Thay đổi phải gọn, không thêm shim tương thích nội bộ | Không đổi `market_research`; không có lớp tương thích. |
| Conventional commit nguyên tử, PR có Problem/Goal/Proposal | Chia theo các bước trong plan; mỗi bước một commit. |
| Đọc `conventions.mdx` (EN/ZH) khi đặt tên/dịch | **Chưa đọc** trong khảo sát này; phải đọc trước khi chốt tên squad/role và dịch (plan T5). |
| Không thêm độ phức tạp cho race hoặc rủi ro bảo mật giả định (`AGENTS.md`, "Change and Delivery Rules") | Không thêm khóa phân tán cho `state.json`; ghi nguyên tử là đủ. Hai agent cùng ghi một `--dir` được tránh bằng cách Lead tuần tự hóa theo stage. Nếu muốn thêm cơ chế khóa, phải trình bày failure mode và xin đồng ý trước. |

## 7. Kiểm thử và đánh giá chất lượng nghiên cứu [S]

LitScout chỉ kiểm hợp đồng. Squad "chuyên sâu" cần thêm đo chất lượng.

### 7.1 Kiểm thử hợp đồng (Go)

Bảng test bám 21 test Python (mục 1.6), cộng: Retry-After dạng ngày, hết hạn mức OpenAlex, `state.json` ghi nguyên tử và `schema_version` lạ, schema scores/summaries sai (thiếu `paper_index`, điểm ngoài 0–10, trường thừa), CSV injection, ngôn ngữ lý do sai (ký tự Hán trong báo cáo `vi`/`en`).

### 7.2 Golden test đối chiếu với Python

Dùng nhánh `examples/legal-rag-run` [V] (có `papers.csv`, `research_digest.md`, `run.log`, `config.json`, `command.txt`) làm nguồn bản ghi: ghi lại phản hồi OpenAlex cho 3 truy vấn trong log, rồi kiểm tra Go cho cùng 22/30/20 bản ghi → 21/47/52 duy nhất (số liệu trong `run.log`). Đây là kiểm tra tương đương hành vi của dedup, abstract và prefilter. Lưu ý: tôi chưa kiểm tra được rằng phản hồi gốc có sẵn để ghi lại; nếu không, phải chạy LitScout Python một lần để thu (cần khóa OpenAlex, ngoài phạm vi khảo sát).

### 7.3 Chỉ số chất lượng cho Research Lab

| Chỉ số | Định nghĩa | Mục tiêu đề xuất |
| --- | --- | --- |
| Hallucinated citation | Số mục trong kết quả không truy được về bản ghi nguồn | 0 (cổng cứng) |
| Provenance pass | Tỉ lệ bài qua `verify` | 100% trước export |
| DOI resolve rate | Tỉ lệ DOI được Crossref xác nhận | ≥ 95%, phần còn lại có cờ |
| Claim grounding | Tỉ lệ mệnh đề tổng hợp có `paper_id` và câu trích khớp abstract (Auditor lấy mẫu ≥ 20% hoặc ≥ 10 mệnh đề) | ≥ 95%, mệnh đề không khớp bị loại |
| Precision@k | Phần bài chọn mà người đánh giá coi là liên quan, trên tập vàng | Đặt ngưỡng sau khi có mẫu cơ sở (chưa có số liệu để chọn con số có căn cứ) |
| Recall trên tập vàng | Phần bài vàng được tìm thấy | Như trên |
| Dedup precision | Số cặp bị gộp nhầm / số cặp gộp (tập cặp gắn nhãn tay) | 0 gộp nhầm khi chỉ dùng fuzzy |
| Coverage | Số nguồn, số vòng dùng, số bài bị cắt | Báo cáo, không có ngưỡng |
| Language compliance | Tỉ lệ trường văn bản đúng `language` | 100% |
| Cost | Số lượt chạy agent, số request HTTP, số lần trúng cache | Báo cáo, so với trần trong protocol |

Chỉ số nào chưa có căn cứ để đặt ngưỡng thì để trống đến khi có lần chạy cơ sở (plan E3). Không bịa số.

### 7.4 Tập vàng

Bắt đầu nhỏ: 3 đề tài (một là "Vietnamese legal GraphRAG" từ ví dụ, hai là đề tài do bạn chọn), mỗi đề tài 15–30 bài do người gắn nhãn liên quan/không liên quan. Người gắn nhãn là bạn hoặc chuyên gia lĩnh vực; agent không tự gắn nhãn tập vàng.

## 8. Rủi ro chính

| # | Rủi ro | Mức | Giảm thiểu |
| --- | --- | --- | --- |
| R1 | Hết hạn mức OpenAlex hoặc thiếu key làm chạy dở | Cao | Lỗi rõ ràng, không retry khi 409/hết hạn mức; cache; trần số truy vấn; khuyến nghị đặt key ở máy daemon |
| R2 | Lệch hành vi dedup (rapidfuzz vs Indel tự viết; `\w` Unicode) | Trung bình | Golden test; chấp nhận khác biệt có chủ đích đã ghi |
| R3 | Agent bỏ qua `gate` và tự quyết dừng/tiếp tục | Trung bình | Skill Lead + Auditor kiểm `state.json` có `quality_gate_decision`; test hướng dẫn chứa quy tắc |
| R4 | Prompt injection qua abstract/tiêu đề | Trung bình | Quy tắc "dữ liệu không phải lệnh"; export escape; không có công cụ nguy hiểm trong squad |
| R5 | Trạng thái giữa các stage mất (agent khác máy, khác thư mục) | Cao | Chốt Q4 trước P1; đính kèm `state.json` vào issue sau mỗi stage |
| R6 | Khóa tái dùng điểm không nhất quán (T7) | Thấp–Trung bình | Một hàm `Key` chuẩn + test |
| R7 | Lọc loại tài liệu kém: mục lục kỷ yếu lọt vào kết quả (đã thấy trong ví dụ) | Thấp | Lọc `type`/`is_paratext` theo `select` mở rộng; kiểm tra tên trường thực tế của OpenAlex lúc triển khai |
| R8 | Thêm skill `research-lab-*` mà quên bộ lọc tiền tố | Trung bình | Test như `sales-skills.test.ts` khẳng định SDLC không import |
| R9 | Đụng 43 file đang sửa chưa commit (`templates.ts`, `quickstart.ts` nằm trong đó) | Trung bình | Làm trên nhánh riêng sau khi người dùng chốt thay đổi hiện tại; mỗi commit chỉ chứa hunk của mình |
| R10 | Bài báo tập trung metadata + abstract; kết luận "chuyên sâu" từ abstract có thể nông | Trung bình | Nói rõ trong báo cáo: kết luận dựa trên abstract; toàn văn chỉ ở P3 và có cờ độ tin cậy |

## 9. Câu hỏi mở cần bạn chốt

| # | Câu hỏi | Khuyến nghị |
| --- | --- | --- |
| Q1 | "Research Lab" là gì? Không có squad tên này trong mã. Bạn muốn (a) template mới `research_lab` song song với `market_research`, (b) nâng cấp thẳng `market_research`, hay (c) có squad "Research Lab" tạo tay trong workspace mà tôi không thấy? | (a). Nếu (c), cho tôi biết tên agent/skill thực tế để điều chỉnh mục 2. |
| Q2 | Mục tiêu nghiên cứu: văn liệu học thuật (như LitScout) hay vẫn gồm nghiên cứu thị trường? | Văn liệu học thuật; thị trường giữ ở `market_research`. |
| Q3 | Roster 5 role có hợp lý không? Có muốn tách Planner riêng, hoặc thêm Librarian lưu KB? | 5 role như mục 4.2, chưa thêm role. |
| Q4 | `state.json` sống ở đâu giữa các stage: thư mục làm việc của issue, đính kèm issue, hay KB? | Đính kèm issue sau mỗi stage + bản cuối vào KB. Cần xác minh khả năng của `multica attachment` trước. |
| Q5 | Có cần lưu run trong Postgres (bảng `research_run`) cho thống kê/UI không? | Chưa. Chỉ làm khi có yêu cầu UI cụ thể. |
| Q6 | Ngôn ngữ chính của đề tài: tiếng Việt, tiếng Anh, hoặc song ngữ? OpenAlex ít phủ nguồn tiếng Việt; có cần nguồn riêng (ví dụ tạp chí trong nước) không? | Truy vấn tiếng Anh, báo cáo theo `language`. Nguồn tiếng Việt là việc riêng sau. |
| Q7 | Có làm trước hay sau `docs/specs/template-registry` (dời template ra `templates/`)? | Làm trước, theo cấu trúc hiện tại; di dời sau theo spec đó. |
| Q8 | Khóa OpenAlex: cấp ở đâu (máy daemon nào)? Có chấp nhận chi phí vượt hạn mức miễn phí không? | Biến môi trường `OPENALEX_API_KEY` trên máy daemon; hạn mức miễn phí đủ cho P1. |
| Q9 | Có bật nguồn mở rộng (Crossref, arXiv, Semantic Scholar) và toàn văn truy cập mở ngay không? | Crossref (xác minh DOI) trong P2; arXiv/S2 sau; toàn văn (X5, X6) để P3, mặc định tắt. |
| Q10 | Có cho sửa agent dùng chung `critic` và `market_scout` (ảnh hưởng 4 squad khác) không? | Không ở P0–P2; chỉ làm P-T7 nếu bạn đồng ý. |
| Q11 | Chấp nhận phụ thuộc mới nào? (đề xuất: không phụ thuộc mới; dùng `golang.org/x/text` đang gián tiếp trong `go.mod`, chuyển thành trực tiếp.) | Không thêm SQLite; `x/text` thành phụ thuộc trực tiếp. |
