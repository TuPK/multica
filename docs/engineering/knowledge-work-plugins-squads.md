# Knowledge Work Plugins (KWP): đánh giá lại và phương án đưa vào Multica

Trạng thái: thiết kế, chưa triển khai. Viết lại ngày 2026-11-16 trên `main` (HEAD `e84995480`), thay cho bản 2026-11-11 vì catalog đã thay đổi lớn (30 squad, 41 agent, 146 skill, domain marketing, collection Kit Việt và vertical).

Companion: [knowledge-work-plugins-disposition.md](knowledge-work-plugins-disposition.md) (bảng quyết định cho cả 252 file `SKILL.md`), [knowledge-work-plugins-squads-plan.md](knowledge-work-plugins-squads-plan.md) (kế hoạch thực hiện).

Nguồn: [anthropics/knowledge-work-plugins](https://github.com/anthropics/knowledge-work-plugins), Apache-2.0, commit `e605d8ddbfb688da57dfaaceebd93f12a64ef1d3` (2026-11-09). Đã kiểm 2026-11-16: đường dẫn và tên của mọi skill được chọn có trên commit này.

Ngôn ngữ: tiếng Việt. Tên skill, role key, kind, lệnh giữ tiếng Anh.

## 0. Kết luận

1. **Không dùng nguyên gói.** 16 plugin, 252 file `SKILL.md`, 174 khai báo MCP server (có trùng), 5 slash command theo cách của Cowork. Gói này giả định người ngồi cạnh, có connector CRM/mail/lịch, và nhiều bước "hỏi người dùng" hoặc "gửi". Catalog của Multica chạy không người, không gửi ra ngoài, và đã có nhiều squad trùng việc. Dùng nguyên gói sẽ trùng và vi phạm quy tắc tự động hóa.
2. **Chọn lọc theo ba nhóm, tổng 26 skill mới, không tạo agent mới:**
   - **A, dùng nguyên trạng (11 skill):** vendor nguyên văn, chỉ ghi thay đổi cục bộ. Gần như không có connector. Phần lớn là phương pháp: phân tích dữ liệu, kiểm số, ma trận rủi ro, nợ kỹ thuật, nghiên cứu người dùng.
   - **B, bổ trợ (13 skill):** viết lại thành skill dẫn xuất, gắn cho vai đã có. Đây là chỗ Multica còn mỏng: hỗ trợ khách hàng, sự cố, runbook, tri thức, thiết kế UX.
   - **D, nguyên tắc chung (2 skill):** `untrusted-content` và `absent-is-not-zero`, rút từ `small-business/shared`. Áp cho mọi squad đọc dữ liệu bên ngoài.
3. **Khoảng trống lớn nhất là phân tích dữ liệu.** Catalog không có skill phân tích dữ liệu nào. Đây là lý do duy nhất để tạo squad mới (`Data & Insights`).
4. **So với bản 2026-11-11:** bỏ `growth_analyst` và `risk_analyst` (đã có `growth_lab` và `finance_lead` đảm nhiệm, xem mục 2). Bỏ gắn cho `kv_*` (hoãn). Giảm từ 41 skill vendor xuống 11, cộng 15 skill dẫn xuất và nguyên tắc.

## 1. Hiện trạng catalog (2026-11-16)

| Thành phần | Số | Ghi chú |
| --- | ---: | --- |
| Squad catalog | 30 | `core` 6, `marketing` 5, `kitviet` 8, `vertical` 11 |
| Agent (`catalog/agents/`) | 41 | Trong đó 5 `kv_*` và 2 `flow_*` |
| Skill catalog | 146 | `builtin` 85, `marketing` 51 (vendored), `starter` 10 |
| Skill Kit Việt | sinh | `catalog/_sources/kitviet/`, không nằm trong `catalog/skills/` |
| Skillset | 7 | `marketing` (chunk), `sdlc`, `planning`, `sales`, `research_lab` (quickstart); `starter_company`, `secretary` (starter) |
| Collection `marketing` | 5 squad | `growth_lab`, `content_studio`, `kv_marketing`, `kv_pr`, `marketing`; domain `marketing` vendor 51 skill |
| Thẻ công ty | 14 squad, 35 vai | 7 floor (thêm `revenue`) |

Quy tắc đã đổi so với 2026-11-11 và ảnh hưởng đến thiết kế này:
- Gắn skill theo vai: `skills.byRole` → quickstart, `addAgentSkills` (`quickstart.ts` ~645–678). Không cần cơ chế mới.
- Quickstart chỉ nạp skill có skillset `target: quickstart`; Go chỉ cài skill có skillset `target: starter` (`starterSkillsForGo`).
- Quy tắc chunk (`scripts/catalog/lib/validate.mjs:120`): skill chunk không được có skillset khác.
- Validator yêu cầu `name:` trong SKILL.md bằng id (`validate.mjs:106`). Vendor nguyên văn thì id phải là tên gốc.
- Quy tắc vendor của domain marketing (`docs/specs/marketing-domain/spec.md`, mục 5.5): skill vendor không sửa để hợp Multica; thay đổi chỉ được liệt kê trong `SOURCES.md`.
- Kit Việt extension `support.json` đã gắn 4 skill CSKH vào squad `support` của thẻ công ty. KWP phải bổ trợ, không lặp lại.

## 2. Đánh giá theo vùng

| Vùng | Đã có trong catalog | Khoảng trống | Quyết định KWP |
| --- | --- | --- | --- |
| Kỹ thuật SDLC | `sdlc-*`, `superpowers:*`, `bmad-*` | Nợ kỹ thuật có công thức ưu tiên; xử lý sự cố | A `tech-debt`; B `incident-response` |
| Lập kế hoạch và PRD | `bmad-*`, `planning-*` | — | Không lấy (C) |
| Tăng trưởng và marketing | 51 skill vendored, `growth_lab`, `content_studio`, `kv_marketing`, `kv_pr` | — | Không lấy (C). Không cần `growth_analyst` |
| Bán hàng | `sales_loop` (7 skill `sales-*`), `kv_b2b` | Các skill KWP đều bám CRM | Không lấy (C) |
| Nghiên cứu | `research_lab`, `market_research` | — | Không lấy (C) |
| Tài chính | `kv_accounting`, `finance_lead` | Ma trận rủi ro; phân rã biến động | A `risk-assessment`, `variance-analysis` |
| Pháp lý | `kv_legal` | — | Không lấy (C) |
| Nhân sự | `kv_talent`, `people_performance` | Vận hành nhân sự (xem D9) | Không lấy (C) |
| Vận hành và hành chính | `kv_officeops`, `kv_admin` | Runbook; quản lý thay đổi | B `runbook`, `change-request` |
| Hỗ trợ khách hàng | Thẻ `support`, Kit Việt CSKH | Phân loại và leo thang yêu cầu | B `ticket-triage`, `customer-escalation`, `kb-article` |
| Tri thức | Thẻ `librarian` (`knowledge_curator`) | Tổng hợp nhiều nguồn; tìm kiếm theo truy vấn | B `knowledge-synthesis`, `search-strategy` |
| Thiết kế và UX | `sdlc-ux-design`, `planning-ux`, `frontend-design`, `accessibility-*` | Handoff cho dev; microcopy; phê bình | B `design-handoff`, `ux-copy`, `design-critique`; A `user-research` |
| Khám phá sản phẩm | `customer-research` (marketing), `customer_voice` | Tổng hợp phỏng vấn | B `synthesize-research` |
| **Phân tích dữ liệu** | **Không có** | Toàn bộ quy trình: khám phá, SQL, thống kê, kiểm số, biểu đồ | **A 7 skill**, B `data-context-extractor`; squad mới |
| Năng suất cá nhân, connector, phần cứng | `productivity`, `secretary-*` | — | Không lấy (C) |

## 3. Ba phương án đã xét

| Phương án | Nội dung | Đánh giá |
| --- | --- | --- |
| P1. Nguyên gói | Cài cả 16 plugin, 174 MCP server | Loại. Trùng ở nhiều vùng (mục 2); kéo connector CRM và mail; đòi hỏi người ngồi cạnh; vi phạm quy tắc không gửi ra ngoài. |
| P2. Chọn lọc (khuyến nghị) | 11 A + 13 B + 2 D | Lấp đúng khoảng trống; không tạo agent mới; kích thước khoảng 230 KB, và khoảng 110 KB cho nhóm thẻ công ty. |
| P3. Tối giản | Chỉ squad Data & Insights, không đụng thẻ công ty | Gọn nhất, nhưng bỏ sót hỗ trợ khách hàng, sự cố và tri thức, là các vai thẻ công ty đang mỏng. |

## 4. Danh mục chốt

Ký hiệu cột đích: "quickstart" là vai trong squad quickstart (byRole); "thẻ" là vai trong thẻ công ty (`skills` của vai).

### 4.1 Nhóm A: vendor nguyên văn

| Skill | Nguồn (plugin) | Đích | Thay đổi cục bộ |
| --- | --- | --- | --- |
| `analyze` | data | `data_lead` | Dòng 44: "Ask the user…" → lấy mặc định, ghi vào giả định |
| `explore-data` | data | `data_profiler` | Dòng 36: "Ask the user…" → như trên |
| `sql-queries` | data | `data_analyst` | Không |
| `statistical-analysis` | data | `data_analyst` | Không |
| `validate-data` | data | `data_validator`; thẻ `critic` (`market_research`, `opportunity_lab`) | Không |
| `create-viz` | data | `data_viz` | Không (cần Python trên runtime) |
| `build-dashboard` | data | `data_viz` | Không (925 dòng, HTML tự đủ) |
| `tech-debt` | engineering | quickstart `sdlc_maintenance`, `lead` | Không |
| `risk-assessment` | operations | thẻ `finance_lead` | Không |
| `variance-analysis` | finance | `data_analyst` | Không (ví dụ bằng USD giữ nguyên là ví dụ) |
| `user-research` | design | thẻ `validation_analyst` (`product_discovery`) | Không |

Tên giữ nguyên tên gốc (không tiền tố), vì validator yêu cầu `name:` khớp id. Đã kiểm: không trùng id nào trong catalog.

Thay đổi cục bộ chỉ có hai dòng "Ask the user" (`analyze` dòng 44, `explore-data` dòng 36). Dòng `argument-hint` và `user-invocable` trong frontmatter là metadata, giữ nguyên, không đọc như chỉ dẫn. Mỗi thay đổi ghi trong `SOURCES.md`.

### 4.2 Nhóm B: dẫn xuất (tên `kwp-<tên gốc>`)

| Skill mới | Từ | Đích | Việc chính khi viết lại |
| --- | --- | --- | --- |
| `kwp-ticket-triage` | customer-support/ticket-triage | thẻ `support_agent` | Bỏ tra cứu support platform; kiểm trùng bằng KB và issue đang mở |
| `kwp-customer-escalation` | customer-support/customer-escalation | thẻ `support_lead` | Người nhận là squad Build qua issue; bỏ gửi cho khách |
| `kwp-kb-article` | customer-support/kb-article | thẻ `support_agent` | Kết quả là tài liệu KB nháp; tìm trùng trước |
| `kwp-incident-response` | engineering/incident-response | thẻ `release_engineer` | Giữ SEV1–4 và postmortem không đổ lỗi; bỏ gọi paging |
| `kwp-runbook` | operations/runbook | thẻ `release_engineer` | Lệnh trong runbook là văn bản; agent không chạy |
| `kwp-change-request` | operations/change-request | thẻ `release_engineer` | Bản nháp để boss duyệt, có kế hoạch rollback |
| `kwp-knowledge-synthesis` | enterprise-search/knowledge-synthesis | thẻ `librarian` | 26 chỗ `~~` đổi thành `multica knowledge search` |
| `kwp-search-strategy` | enterprise-search/search-strategy | thẻ `librarian` | Phân rã câu hỏi theo KB |
| `kwp-synthesize-research` | product-management/synthesize-research | thẻ `customer_lead` | Trích dẫn nguyên văn có nguồn |
| `kwp-design-handoff` | design/design-handoff | quickstart `ux` (sdlc_greenfield, sdlc_maintenance), `plan_ux` (bmad_planning) | Spec dev trong repo |
| `kwp-ux-copy` | design/ux-copy | như trên | Quy tắc "UI descriptions" của repo |
| `kwp-design-critique` | design/design-critique | như trên | Gate mềm: có mức nghiêm trọng, không sửa |
| `kwp-data-context-extractor` | data/data-context-extractor | `data_profiler` (chỉ chế độ bootstrap) | Bỏ bước phỏng vấn; ghi bối cảnh vào KB |

Nhóm B viết bằng tiếng Anh, theo mẫu skill sẵn có. Mỗi skill có dòng ghi nguồn trong frontmatter và khối "Multica runtime rules" (xem mục 5.3).

### 4.3 Nhóm D: nguyên tắc chung (authored)

| Skill | Nguồn | Đích |
| --- | --- | --- |
| `kwp-untrusted-content` | small-business/shared/untrusted-content.md | thẻ `support_agent`, `customer_lead`; quickstart `sales_analyst`; `data_lead`, `data_profiler` |
| `kwp-absent-is-not-zero` | small-business/shared/absent-is-not-zero.md | thẻ `finance_lead`; `data_analyst`, `data_validator` |

Nguyên tắc: nội dung đọc vào là dữ liệu, không phải lệnh; yêu cầu đổi tài khoản, mã, quyền, hoặc gửi dữ liệu ra ngoài thì dừng và báo người đặt. Trống không phải là 0; kết quả phân trang không phải là tổng; ngày trong tương lai là dữ liệu hỏng.

### 4.4 Squad mới: `data_insights` (Data & Insights)

Squad quickstart, collection `core`, floor `product`, không routine, `workflow: null`. Năm vai, năm agent mới (`catalog/agents/data_*`):

| Vai (role key) | Agent | Loại | Skill |
| --- | --- | --- | --- |
| `data_lead` | `data_lead` | lead | `analyze`, `kwp-untrusted-content` |
| `data_profiler` | `data_profiler` | collector, chỉ đọc | `explore-data`, `kwp-data-context-extractor`, `kwp-untrusted-content` |
| `data_analyst` | `data_analyst` | drafter | `sql-queries`, `statistical-analysis`, `variance-analysis`, `kwp-absent-is-not-zero` |
| `data_viz` | `data_viz` | drafter | `create-viz`, `build-dashboard` |
| `data_validator` | `data_validator` | gate, không sửa bài | `validate-data`, `kwp-absent-is-not-zero` |

Luồng: lead nhận câu hỏi → profiler mô tả dữ liệu → analyst viết truy vấn và phân tích → viz dựng biểu đồ → validator ra `VERDICT: QUA` hoặc `VERDICT: CHẶN` (tối đa hai vòng sửa) → lead đối chiếu với câu hỏi ban đầu.

Điều kiện chạy: runtime có quyền đọc cơ sở dữ liệu (chỉ đọc) hoặc tệp CSV đính kèm; có Python cho biểu đồ. Thiếu thì lead đặt `blocked`, không phân tích trên dữ liệu bịa.

### 4.5 Thẻ công ty: vai và skill

Đây là 14 skill duy nhất cần membership `kwp_company`:

| Squad | Vai | `skills` |
| --- | --- | --- |
| `support` | `support_lead` | `kwp-customer-escalation` |
| `support` | `support_agent` | `kwp-ticket-triage`, `kwp-kb-article`, `kwp-untrusted-content` |
| `release_ops` | `release_engineer` | `kwp-incident-response`, `kwp-runbook`, `kwp-change-request` |
| `executive_office` | `librarian` | `kwp-knowledge-synthesis`, `kwp-search-strategy` |
| `customer_voice` | `customer_lead` | `kwp-synthesize-research`, `kwp-untrusted-content` |
| `product_discovery` | `validation_analyst` | `user-research` |
| `market_research` | `critic` | `validate-data` |
| `opportunity_lab` | `critic` | `validate-data` |
| `finance_risk` | `finance_lead` | `risk-assessment`, `kwp-absent-is-not-zero` |

Không thêm vai nào. `growth_gtm` không nhận skill từ KWP: `growth_lab` đã có vai đo lường (`gl_measure`).

Các thẻ có sẵn không đổi số vai. Thẻ `release_ops` không nhận `status-report`, vì `secretary-brief` đã làm việc báo cáo.

## 5. Cơ chế và quy tắc

### 5.1 Skillset và membership

| Skillset | Target | Chunk | Thành viên |
| --- | --- | --- | --- |
| `kwp` | `quickstart` | có | 26 skill (11 A, 13 B, 2 D) |
| `kwp_company` | `starter` | không | 14 skill trong bảng 4.5 |

Lý do chọn chunk (D10, khuyến nghị): tránh thêm khoảng 230 KB vào bundle quickstart chính. Quy tắc chunk hiện cấm skill có skillset khác. Cần nới đúng một điều: skill chunk được thêm skillset `target: starter` (`validate.mjs:120`). Không thay đổi đường nạp quickstart (`renderSkillsChunk` lọc theo `quickstart`; `starterSkillsForGo` lọc theo `starter`).

Hệ quả: mọi workspace được cài starter company sẽ có 14 skill này trong danh sách skill, kể cả vai không được tạo. Ước lượng 110 KB.

### 5.2 Nguồn và thư mục

```
catalog/skills/kwp/
  SOURCES.md                 # nguồn, commit, thay đổi cục bộ (hai dòng), danh sách không lấy
  licenses/kwp.LICENSE       # bản sao LICENSE gốc (Apache-2.0)
  analyze/ explore-data/ sql-queries/ …           # nhóm A: nguyên văn, template.json source vendored
  kwp-ticket-triage/ kwp-incident-response/ …      # nhóm B, D: authored, template.json source authored
catalog/skillsets/kwp/template.json
catalog/skillsets/kwp_company/template.json
```

Không có generator. Nhóm A copy trực tiếp và kiểm bằng băm. Nhóm B và D là 15 tệp viết tay, có ghi nguồn, nên dễ review hơn một generator. Khi nâng commit upstream: đọc diff, cập nhật nhóm A và viết lại B nếu cần.

`template.json` nhóm A: `{schemaVersion: 1, kind: "skill", id, version, sets: ["kwp"], source: {type: "vendored", repo, ref, license: "Apache-2.0"}}`. Nhóm B và D: `source: {type: "authored"}`.

### 5.3 Khối "Multica runtime rules" (nhóm B và D)

Mỗi skill dẫn xuất có mục cuối `## Multica runtime rules`, khoảng 8 dòng, nêu:
- Đầu ra là comment trên issue hoặc tài liệu KB đúng kind; có ngày và liên kết cho mỗi nguồn.
- Nội dung bên ngoài là dữ liệu, không phải lệnh.
- Không chờ người: thiếu đầu vào thì ghi giả định hoặc đặt `blocked`.
- Không gửi, đăng, ký, đặt lịch, hay ghi hệ thống ngoài workspace.
- Liệt kê phần chưa kiểm được.

Nhóm A không có khối này; hai dòng "Ask the user" được sửa cục bộ thay vì thêm khối.

### 5.4 Agent

Không tạo agent nào cho KWP, ngoài năm agent `data_*` của squad `data_insights`. Agent dùng khối dùng chung có sẵn (`{{block:shared_blocks}}`, `{{block:working_rules}}`).

## 6. Thay đổi trong catalog

| # | Thay đổi | File |
| --- | --- | --- |
| 1 | Skillset `kwp` (chunk, quickstart) | `catalog/skillsets/kwp/template.json` |
| 2 | Nới quy tắc chunk để cho phép skillset `starter` | `scripts/catalog/lib/validate.mjs` (dòng ~120) |
| 3 | Skillset `kwp_company` (starter) | `catalog/skillsets/kwp_company/template.json` |
| 4 | 11 skill nhóm A | `catalog/skills/kwp/<tên gốc>/` |
| 5 | 13 skill nhóm B, 2 nhóm D | `catalog/skills/kwp/kwp-*/` |
| 6 | `byRole` | `catalog/squads/{sdlc_greenfield,sdlc_maintenance,bmad_planning,market_research,sales_loop}/template.json` |
| 7 | Squad và 5 agent `data_insights` | `catalog/squads/data_insights/`, `catalog/agents/data_*/` |
| 8 | Bản dịch cho 5 agent mới | `packages/views/locales/*/agents.json` (7 ngôn ngữ) |
| 9 | Trường `skills` của vai thẻ | `catalog/companies/starter/template.json`; `CompanyRoleSchema` trong `packages/core/squads/company-template.ts` |
| 10 | Go: `companyRole.Skills`, `starterSkillsForRole` | `server/internal/service/company_template_data.go` (dòng 57), `company_starter_skills.go` |
| 11 | Gỡ `people_performance` khỏi HR (D9) | `catalog/_sources/kitviet/extensions/people.json`; `docs/engineering/kitviet-squads.md` mục 7 |

Đã kiểm: `companyForGo` truyền `squads` nguyên dạng, nên trường `skills` của vai đi tới `company.json`. Tên vai và mô tả squad quickstart nằm trong `template.json` (tiếng Anh); tên dịch của agent đọc từ `agents.json` theo `agentTemplate`.

## 7. Kiểm thử

| Kiểm | Lệnh hoặc vị trí | Ý nghĩa |
| --- | --- | --- |
| Catalog hợp lệ | `pnpm catalog:check` | id, `byRole` đúng vai và đúng skillset, nguồn vendored có ref và license, bản dịch đủ bảy ngôn ngữ cho squad mới |
| Nhóm A không lệch | băm từng thư mục so với commit ghim (script một lần khi vendor) | Chỉ hai dòng "Ask the user" khác upstream; `SOURCES.md` liệt kê đúng |
| Membership | `scripts/catalog/catalog.test.mjs` | Mọi skill KWP có `kwp`; 14 skill thẻ công ty có `kwp_company`; skill chunk có hai skillset quickstart là lỗi |
| Thẻ công ty | `packages/core/squads/company-template.test.ts`, `company-template-data.test.ts`, `packages/core/catalog/squad-invariants.test.ts` | 14 squad, 35 vai; `skills` đúng schema; không vai nào quá 6 skill KWP |
| Starter Go | `server/internal/service/company_starter_skills_test.go` | Skill thẻ nằm trong `starter_skills`; thứ tự ổn định; không trùng |
| Apply có DB | `server/internal/service/phase4_company_db_test.go` hoặc tệp mới | Apply hai lần không nhân đôi; skill cùng tên của boss không bị ghi đè; khẳng định trên hàng của fixture (`go-test-db-leftovers`) |
| Quickstart | `packages/core/squads/quickstart-flow.test.ts`, `quickstart.test.ts` | Vai nhận đúng skill; skill không có trong bundle được báo, squad vẫn tạo |
| Kit Việt | `scripts/generate-kitviet-pack.test.mjs` | Sau D9: `people_performance` không có skill HR; `kv_talent` có `onboarding-30-60-90` |

Thứ tự chạy: `pnpm catalog:build` trước `vitest`, `tsc`, `go test`. Chạy thật có người cho phép: một lần cho `data_insights` (dữ liệu đủ, thiếu quyền đọc → `blocked`, truy vấn có join nổ → `VERDICT: CHẶN`).

## 8. Rủi ro

| Rủi ro | Giảm thiểu |
| --- | --- |
| Hai dòng "Ask the user" trong nhóm A bị bỏ sót khi nâng upstream | Test băm và `SOURCES.md`; đọc diff khi nâng commit |
| Nhóm B viết lại làm mất ý phương pháp | Đọc trọn skill upstream trước khi viết; mỗi skill ghi phạm vi đã đọc trong mô tả PR |
| `kwp_company` cài 14 skill vào mọi starter company | Ghi trong PR; ước lượng 110 KB; có thể bỏ nếu thẻ không dùng |
| Nới quy tắc chunk ảnh hưởng skill marketing | Test hiện có của `marketing` chunk; quy tắc mới chỉ thêm skillset `starter` |
| Vai thẻ đã có nhận skill khi rerun | Rerun chỉ cộng thêm; test apply hai lần |
| Trùng với Kit Việt CSKH (cùng squad `support`) | Phân vai rõ: KWP lo phân loại và leo thang; KV lo kịch bản và phản hồi tiếng Việt |
| `data_insights` đọc dữ liệu nhạy cảm | Chỉ đọc; `kwp-untrusted-content`; không ghi ra ngoài workspace; thiếu quyền thì `blocked` |

## 9. Quyết định

### 9.1 Đã chốt (2026-11-11)

1. Thêm squad `Data & Insights`. (Được giữ, với năm vai, mục 4.4.)
2. Thêm agent `growth_analyst`, `risk_analyst` vào thẻ công ty. **Đảo ngược** (mục 2): không cần, do `growth_lab` và `finance_lead` đã đảm nhiệm.
3. Skill nhân sự cá nhân khỏi People & Performance. Chưa thực hiện (xem D9).
4. Support: skill tiếng Anh chỉ cho workspace tiếng Anh; tiếng Việt dùng skill Kit Việt. Giữ nguyên.
5. Chấp nhận Apache-2.0 kèm NOTICE. Giữ nguyên.

### 9.2 Quyết định (2026-11-16)

Chủ sản phẩm ủy quyền chọn phương án. Bốn điểm mở được chốt như sau. Mỗi điểm ghi điều kiện làm đảo ngược, để có thể đổi khi bằng chứng thay đổi.

| # | Quyết định | Lý do | Đảo ngược khi |
| --- | --- | --- | --- |
| D6 | Nhóm A vendor nguyên văn (chỉ sửa hai dòng); nhóm B và D là skill authored có ghi nguồn | A không sửa ngoài hai dòng, nên đúng quy tắc vendor của domain marketing. B không mang nhãn vendored nên không cần ngoại lệ. Không cần generator. | Phải nâng commit upstream thường xuyên: khi đó viết generator cho nhóm A |
| D9 | Gỡ `people_performance` khỏi `people.json`. `onboarding-30-60-90` sang `kv_talent`. Bốn skill vận hành không gắn vào squad nào. | Charter của People & Performance cấm đánh giá cá nhân; `kv_talent` có phạm vi sàng lọc ứng viên và onboarding là việc liền kề; bốn skill còn lại cần squad People Ops mới có ngữ cảnh | Kit Việt có squad People Ops: gắn lại bốn skill vào đó |
| D10 | Skillset `kwp` là chunk. Nới quy tắc chunk một dòng để cho phép skillset `starter` (`kwp_company`). | Không thêm khoảng 230 KB vào bundle quickstart. Thay đổi validator nhỏ và có test. `company.json` tăng khoảng 110 KB, chấp nhận được. | `company.json` vượt giới hạn của build: bỏ `kwp_company`, và thẻ công ty không nhận skill KWP |
| D12 | Tạo squad `data_insights` với năm agent | Đây là khoảng trống lớn nhất (mục 2). Năm vai khớp năm nhóm skill; mỗi vai có trách nhiệm kiểm riêng (validator độc lập với analyst). | Sau một quý không có việc phân tích nào chạy: gộp `data_profiler` và `data_analyst`, rồi bỏ `data_viz` nếu cần |

Đã đóng từ trước: D7 (không lấy `competitive-brief`), D8 (không tạo `growth_analyst` và `risk_analyst`), D11 (`kv_*` hoãn, ngoài phạm vi).

Không còn điểm nào chờ duyệt. Có thể bắt đầu từ Giai đoạn 1 và Giai đoạn 6 của plan ngay.
