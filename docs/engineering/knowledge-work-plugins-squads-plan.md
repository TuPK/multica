# Knowledge Work Plugins (KWP): kế hoạch triển khai

Kế hoạch cho [knowledge-work-plugins-squads.md](knowledge-work-plugins-squads.md) (đánh giá lại 2026-11-16, mô hình chọn lọc: 11 vendor nguyên văn, 13 dẫn xuất, 2 nguyên tắc chung). Bảng quyết định từng skill: [knowledge-work-plugins-disposition.md](knowledge-work-plugins-disposition.md). Mọi "mục x.y" trỏ về spec.

Quy ước thực hiện (theo AGENTS.md):

- Nội dung nguồn nằm trong `catalog/`. Mã sinh (`packages/core/catalog/generated/`, `server/internal/catalog/generated/`) không commit. Chạy `pnpm catalog:build` trước `vitest`, `tsc`, `go test` chạy trực tiếp; `pnpm catalog:check` kiểm nguồn.
- Thêm squad, agent, skillset: `pnpm catalog:new <kind> <id> …` rồi sửa thư mục vừa tạo. Không sửa tay mã sinh.
- Mỗi task một commit, kiểu conventional (`feat(catalog): …`, `feat(company): …`, `fix(squads): …`, `test(…)`). Kết thúc commit bằng dòng `Co-Authored-By` theo hướng dẫn của phiên.
- PR mở vào `--repo TuPK/multica`, không bao giờ vào `multica-ai/multica`. Mô tả theo mẫu (Problem, Goal, Proposal), ghi phạm vi đã đọc.
- Chuỗi UI qua i18n đủ bảy ngôn ngữ (`en, zh-Hans, zh-Hant, fr, es, ko, ja`).
- Test mặc định không gọi mạng, không chạy agent CLI thật. Chạy thật chỉ khi người dùng cho phép.
- Test Go dùng chung một cơ sở dữ liệu: lỗi trùng khóa thì kiểm dòng sót (`go-test-db-leftovers`) trước khi kết luận là hồi quy. Không chạy hai lần song song.
- Chỉ stage đường dẫn của task. Không `git add -A`.
- Không khóa ngoại, không cascade, không migration.
- Nhóm A không sửa tay ngoài hai dòng đã nêu (spec 4.1). Nhóm B và D là skill viết tay, ghi nguồn trong frontmatter.
- Quyết định đã chốt ở spec 9.2 (D6, D9, D10, D12). Nếu một điều kiện đảo ngược xảy ra, dừng, cập nhật spec 9.2, rồi mới đổi code.

## Trạng thái cổng (đã kiểm trên mã 2026-11-16)

| Cổng | Câu hỏi | Kết quả |
| --- | --- | --- |
| G1 | Cơ chế gắn skill theo vai có chưa? | **Có.** `skills.byRole` → `template.roleSkills` (`packages/core/catalog/squads.ts:89`); `attachAgentSkills` (`quickstart.ts` ~645–678); `POST /api/agents/{id}/skills/add` (`server/cmd/server/router.go`). |
| G2 | Có giới hạn số skill cho agent không? | **Không tìm thấy** (tìm `maxSkills`, `MaxAgentSkill` trong `server/internal` và `packages/core`). Kiểm lại trong Task 5.6. |
| G3 | Kích thước `company.json` | 332 536 byte hiện nay. Dự kiến **tăng khoảng 110 KB** do 14 skill `kwp_company` (spec 5.1). Ghi con số thực vào PR. |
| G4 | Rerun apply có thêm vai/skill vào squad đã có không? | **Chưa kiểm.** Kiểm bằng test DB trong Task 5.6. |
| G5 | Khung Kit Việt có chưa? | **Có** trên `main`. |
| G6 | Quy tắc chunk có chặn membership `starter` không? | **Có** (`validate.mjs:120`). Gỡ ở Task 5.1. |
| G7 | Tên skill KWP có trùng id nào trong catalog không? | **Không** (kiểm 11 tên nhóm A và 13 tên nhóm B). |
| G8 | Vendor nguyên văn có đi qua validator không? | **Có.** Validator yêu cầu `name:` trong SKILL.md bằng id (`validate.mjs:106`); nhóm A giữ tên gốc nên khớp. |
| G9 | Các role key trong `byRole` và thẻ có tồn tại không? | **Có.** Đã kiểm: `sdlc_greenfield` (`ux`), `sdlc_maintenance` (`lead`, `ux`), `bmad_planning` (`plan_ux`), `market_research` (`critic`), `sales_loop` (`sales_analyst`); thẻ công ty `support_lead`, `support_agent`, `release_engineer`, `librarian` (executive_office), `customer_lead`, `validation_analyst` (product_discovery), `critic` (market_research, opportunity_lab), `finance_lead`. |

---

## Giai đoạn 1: vendor nhóm A (PR 1)

D6 đã chốt (spec 9.2). Không phụ thuộc giai đoạn nào khác.

### 1.1 Chép 11 skill

Files: `catalog/skills/kwp/{analyze,explore-data,sql-queries,statistical-analysis,validate-data,create-viz,build-dashboard,tech-debt,risk-assessment,variance-analysis,user-research}/`.

- Clone `https://github.com/anthropics/knowledge-work-plugins.git`, checkout `e605d8ddbfb688da57dfaaceebd93f12a64ef1d3`.
- Chép nguyên văn từ: `data/skills/{analyze,explore-data,sql-queries,statistical-analysis,validate-data,create-viz,build-dashboard}`, `engineering/skills/tech-debt`, `operations/skills/risk-assessment`, `finance/skills/variance-analysis`, `design/skills/user-research`.
- Kiểm: `find catalog/skills/kwp -name SKILL.md | wc -l` = 11 (sau 1.3).

### 1.2 Hai thay đổi cục bộ

- `analyze/SKILL.md` dòng 44: "1. Ask the user to provide data in one of these ways:" → thay bằng "1. Use the data the request attaches or pastes; if none, state the assumption and continue:" (giữ phần còn lại của danh sách).
- `explore-data/SKILL.md` dòng 36: "1. Ask the user to provide a table name (with their warehouse connected) or upload a file" → "1. Use the table or file the request names; if none, record the gap as MISSING and continue".
- Không sửa gì khác. Kiểm sau khi sửa: so từng tệp với upstream, chỉ hai tệp này khác và chỉ ở hai dòng trên.
- Mỗi nhóm A chỉ có một tệp `SKILL.md`, không có `references/`. Chép cả thư mục là đủ.

### 1.3 `template.json` và skillset

Files: `catalog/skills/kwp/<id>/template.json` (11 tệp), `catalog/skillsets/kwp/template.json`.

- Mỗi skill: `{"schemaVersion": 1, "kind": "skill", "id": "<tên gốc>", "version": "1.0.0", "sets": ["kwp"], "source": {"type": "vendored", "repo": "https://github.com/anthropics/knowledge-work-plugins", "ref": "e605d8ddbfb688da57dfaaceebd93f12a64ef1d3", "license": "Apache-2.0"}}`.
- Skillset: `{"schemaVersion": 1, "kind": "skillset", "id": "kwp", "version": "1.0.0", "target": "quickstart", "chunk": true}`. Chunk theo D10 (spec 5.1).

### 1.4 Giấy phép và nguồn

Files: `catalog/skills/kwp/SOURCES.md`, `catalog/skills/kwp/licenses/kwp.LICENSE`.

- `kwp.LICENSE`: bản sao `LICENSE` gốc của repo (Apache-2.0). Các plugin `operations`, `engineering`, `design` không có `LICENSE` riêng, nên dùng bản gốc.
- `SOURCES.md` theo mẫu `catalog/skills/marketing/SOURCES.md`: nguồn, commit, giấy phép; bảng 11 skill; mục "Thay đổi cục bộ" (hai dòng 1.2); mục "Không lấy" kèm lý do (tóm tắt bảng disposition); mục "Nâng commit" mô tả quy trình đọc diff.

### 1.5 Kiểm tra

- `pnpm catalog:check` sạch (không có cảnh báo "unpinned" cho nhóm kwp).
- Script băm một lần (không commit): so 11 thư mục với commit ghim; chỉ hai tệp khác, đúng hai dòng.
- Commit: `feat(catalog): vendor eleven Knowledge Work Plugins skills at a pinned commit`.

---

## Giai đoạn 2: dẫn xuất nhóm B và nguyên tắc nhóm D (PR 2)

Phụ thuộc Giai đoạn 1 (skillset `kwp` tồn tại).

Với mỗi skill: đọc trọn SKILL.md và `references/` upstream trước khi viết (spec 4.2). Viết bằng tiếng Anh, theo mẫu skill sẵn có. Frontmatter: `name` bằng id, `description` có mở đầu "Use when you are …", dòng `Source: derived from <plugin>/<skill> at e605d8dd (Apache-2.0)`. Cuối file có khối `## Multica runtime rules` (8 dòng, spec 5.3). `template.json`: `sets: ["kwp"]`, `source: {"type": "authored"}`.

### 2.1 Hỗ trợ khách hàng

Files: `catalog/skills/kwp/kwp-{ticket-triage,customer-escalation,kb-article}/`.

- `kwp-ticket-triage`: phân loại, P1–P4, kiểm trùng bằng `multica knowledge search` và issue đang mở. Không tra cứu support platform.
- `kwp-customer-escalation`: brief cho squad Build qua issue, có bước tái hiện. Hoàn tiền, ngoại lệ, hứa ngày là việc của boss.
- `kwp-kb-article`: nháp KB, tìm trùng trước khi tạo.
- Commit: `feat(catalog): add the derived support skills`.

### 2.2 Sự cố, runbook, thay đổi

Files: `catalog/skills/kwp/kwp-{incident-response,runbook,change-request}/`.

- `kwp-incident-response`: giữ SEV1–4, nhịp cập nhật, postmortem không đổ lỗi, 5 whys. Bỏ paging và chat; thông báo khách là nháp.
- `kwp-runbook`: lệnh trong runbook là văn bản; không chạy.
- `kwp-change-request`: bản nháp, có rollback; boss duyệt.
- Commit: `feat(catalog): add the derived incident, runbook and change skills`.

### 2.3 Tri thức và khách hàng

Files: `catalog/skills/kwp/kwp-{knowledge-synthesis,search-strategy,synthesize-research}/`.

- `kwp-knowledge-synthesis`: 26 chỗ `~~` của upstream được thay bằng `multica knowledge search`; giữ gộp trùng và chấm độ tin theo độ mới và độ chính thống.
- `kwp-search-strategy`: phân rã câu hỏi theo KB.
- `kwp-synthesize-research`: chủ đề kèm trích dẫn nguyên văn có nguồn.
- Commit: `feat(catalog): add the derived knowledge and research skills`.

### 2.4 Thiết kế và dữ liệu

Files: `catalog/skills/kwp/kwp-{design-handoff,ux-copy,design-critique,data-context-extractor}/`.

- `kwp-design-handoff`: spec cho dev (token, state, breakpoint); dùng token của dự án khi có.
- `kwp-ux-copy`: microcopy; theo quy ước "UI descriptions" của repo.
- `kwp-design-critique`: phê bình có khung và mức nghiêm trọng; không sửa.
- `kwp-data-context-extractor`: chỉ chế độ bootstrap; bỏ bước phỏng vấn; ghi bối cảnh vào KB.
- Commit: `feat(catalog): add the derived design and data-context skills`.

### 2.5 Nguyên tắc chung (nhóm D)

Files: `catalog/skills/kwp/kwp-{untrusted-content,absent-is-not-zero}/`.

- Đọc trọn `small-business/shared/untrusted-content.md` và `absent-is-not-zero.md` từ bản clone. Chắt lọc theo spec 4.3, bỏ ví dụ connector và tên nhà cung cấp.
- Commit: `feat(catalog): add the untrusted-content and absent-is-not-zero skills`.

### 2.6 Test chuỗi cấm

Files: `scripts/catalog/catalog.test.mjs`.

- Với mọi `kwp-*` có `source.type: authored`: có khối `## Multica runtime rules`; không có `~~`, `$ARGUMENTS`, `@$1`, `argument-hint`, "Ask the user", "ask the user", "send for signature", "DocuSign", `AskUserQuestion`.
- Với nhóm A: không có chuỗi cấm, trừ hai dòng đã sửa (đã nêu trong SOURCES).
- Kiểm: `node --test scripts/catalog/catalog.test.mjs`; `pnpm catalog:check`.
- Commit: `test(catalog): check the derived KWP skills for forbidden steps`.

Cổng ra Giai đoạn 2: 26 skill trong `catalog/skills/kwp/`, `pnpm catalog:check` và test chuỗi cấm xanh.

---

## Giai đoạn 3: gắn vai trong squad quickstart (PR 3)

Phụ thuộc Giai đoạn 1 và 2. Không đụng thẻ công ty.

Files: `catalog/squads/{sdlc_greenfield,sdlc_maintenance,bmad_planning,market_research,sales_loop}/template.json`.

| Squad | Vai | `byRole` |
| --- | --- | --- |
| `sdlc_greenfield` | `ux` | `kwp-design-handoff`, `kwp-ux-copy`, `kwp-design-critique` |
| `sdlc_maintenance` | `lead` | `tech-debt` |
| `sdlc_maintenance` | `ux` | `kwp-design-handoff`, `kwp-ux-copy`, `kwp-design-critique` |
| `bmad_planning` | `plan_ux` | `kwp-design-handoff`, `kwp-ux-copy`, `kwp-design-critique` |
| `market_research` | `critic` | `validate-data` |
| `sales_loop` | `sales_analyst` | `kwp-untrusted-content` |

- Đừng thêm `risk-assessment`, `user-research` vào đây: chúng gắn ở thẻ công ty (Giai đoạn 5).
- Kiểm: `pnpm catalog:check`; `pnpm catalog:build`; `pnpm --filter @multica/core exec vitest run squads/quickstart.test.ts`.
- Commit: `feat(catalog): give the SDLC, planning, research and sales roles their KWP skills`.

---

## Giai đoạn 4: squad Data & Insights (PR 4)

D12 đã chốt (spec 9.2). Phụ thuộc Giai đoạn 1 và 2.

### 4.1 Năm agent

Files: `catalog/agents/{data_lead,data_profiler,data_analyst,data_viz,data_validator}/{template.json,instructions.md}`.

- `template.json`: `{"schemaVersion": 1, "kind": "agent", "id", "version": "1.0.0", "order"}`. `order` đặt sau các agent `kv_*`.
- `instructions.md` dùng `{{block:shared_blocks}}` và `{{block:working_rules}}`. `data_lead` thêm `{{block:squad_leader_rules}}`. Nội dung theo spec 4.4.
- `data_validator`: kết luận dòng đầu `VERDICT: QUA` hoặc `VERDICT: CHẶN`; không sửa bài; tối đa hai vòng sửa.
- Kiểm: `pnpm catalog:check`.
- Commit: `feat(catalog): add the data analysis agents`.

### 4.2 Bản dịch agent

Files: `packages/views/locales/{en,es,fr,ja,ko,zh-Hans,zh-Hant}/agents.json`.

- Khóa theo `agentTemplate` (`data_lead`, …), cạnh `growth_lead`. Tên và mô tả ≤200 ký tự, không nhắc lại tên.
- Kiểm trước khi commit, không để mở: `packages/views/modals/squad-quickstart.tsx` dòng ~320 hiển thị `a.name`. Đọc nơi `a` được tạo (thường là `template.roles` trong `packages/core/catalog/squads.ts`).
  - Nếu `a.name` lấy từ `template.json` (tiếng Anh): không cần khóa dịch cho vai, chỉ cần khóa `agentTemplate` nếu picker agent đọc `agents.json`. Ghi kết quả trong PR.
  - Mặc định nếu chưa rõ: thêm khóa theo `agentTemplate` vào `agents.json` (như `growth_lead`), vì picker agent đọc khóa này.
- Kiểm: `pnpm --filter @multica/views exec vitest run i18n` nếu có.
- Commit: `feat(views): translate the data analysis agents`.

### 4.3 Squad

Files: `catalog/squads/data_insights/{template.json,instructions.md,i18n/*.json}`.

- `pnpm catalog:new squad data_insights --collection core`, rồi điền:
  - `leader: "data_lead"`, `floorName: "Product"`, `workflow: null`, `routines: []`.
  - Năm vai theo spec 4.4, kèm `byRole`: `data_lead` (`analyze`, `kwp-untrusted-content`); `data_profiler` (`explore-data`, `kwp-data-context-extractor`, `kwp-untrusted-content`); `data_analyst` (`sql-queries`, `statistical-analysis`, `variance-analysis`, `kwp-absent-is-not-zero`); `data_viz` (`create-viz`, `build-dashboard`); `data_validator` (`validate-data`, `kwp-absent-is-not-zero`).
  - `instructions.md`: điều kiện chỉ đọc; `blocked` khi thiếu quyền hoặc dữ liệu; không phân tích trên số bịa.
  - `i18n/` đủ bảy ngôn ngữ (title, description).
- Kiểm: `pnpm catalog:check`; `pnpm catalog:build`; `pnpm --filter @multica/core exec vitest run catalog/squad-invariants.test.ts`.
- Commit: `feat(catalog): add the Data & Insights squad`.

### 4.4 Diễn tập (chỉ khi người dùng cho phép)

Ba lần chạy thật (spec 7): dữ liệu đủ; thiếu quyền đọc (kỳ vọng `blocked`); truy vấn có join nổ (kỳ vọng `VERDICT: CHẶN`). Dán biên bản vào PR, hoặc ghi "chưa chạy thật".

Cổng ra: `pnpm typecheck`, `pnpm lint`, `pnpm test`.

---

## Giai đoạn 5: thẻ công ty (PR 5)

Phụ thuộc Giai đoạn 2 (skill tồn tại). Độc lập với Giai đoạn 3 và 4.

### 5.1 Gỡ quy tắc chunk cho membership starter

Files: `scripts/catalog/lib/validate.mjs` (dòng ~120), `scripts/catalog/catalog.test.mjs`.

- Quy tắc mới: skill chunk không được có skillset `target: quickstart` khác. Skillset `target: starter` được phép.
- Test: (a) skill chunk có `kwp` và `kwp_company` không lỗi; (b) skill chunk có hai skillset quickstart là lỗi.
- Lý do đã kiểm: `renderSkillsBuiltin` và `renderSkillsChunk` lọc theo `quickstart`; `starterSkillsForGo` lọc theo `starter`. Hai đường không lẫn.
- Commit: `fix(catalog): let a chunk skill also belong to a starter skillset`.

### 5.2 Skillset `kwp_company` và membership

Files: `catalog/skillsets/kwp_company/template.json` (mới), `sets` của 14 skill.

- `{"schemaVersion": 1, "kind": "skillset", "id": "kwp_company", "version": "1.0.0", "target": "starter"}`.
- Thêm `kwp_company` vào `sets` của: `kwp-ticket-triage`, `kwp-customer-escalation`, `kwp-kb-article`, `kwp-incident-response`, `kwp-runbook`, `kwp-change-request`, `kwp-knowledge-synthesis`, `kwp-search-strategy`, `kwp-synthesize-research`, `kwp-untrusted-content`, `kwp-absent-is-not-zero`, và các skill vendor `user-research`, `validate-data`, `risk-assessment`.
- Kiểm: `pnpm catalog:check`; `node --test scripts/catalog/catalog.test.mjs`.
- Commit: `feat(catalog): add the kwp_company skillset for starter roles`.

### 5.3 Trường `skills` của vai thẻ

Files: `catalog/companies/starter/template.json`, `packages/core/squads/company-template.ts` (`CompanyRoleSchema`), `packages/core/squads/company-template.test.ts`.

- Vai và skill theo spec 4.5:

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

- `CompanyRoleSchema` thêm `skills: z.array(z.string().min(1)).default([])`. Schema không strict, nên trường phải khai báo, nếu không zod bỏ lặng lẽ.
- Test: mặc định `[]`; mọi tên trong bảng trên tồn tại trong `catalog/skills/` (kiểm trong `catalog.test.mjs`, không đọc tệp trong schema); 14 squad, 35 vai không đổi.
- Kiểm: `pnpm catalog:build`; `pnpm --filter @multica/core exec vitest run squads/company-template`.
- Commit: `feat(company): let the starter roles name their skills`.

### 5.4 Go

Files: `server/internal/service/company_template_data.go` (`companyRole`, dòng 57), `server/internal/service/company_starter_skills.go`, `server/internal/service/company_starter_skills_test.go`.

- `companyRole` thêm `Skills []string` (json `skills`).
- `starterSkillsForRole`: trả skill leader hiện có cộng `role.Skills`, bỏ trùng, giữ thứ tự. Secretary không đổi.
- Test bảng: thành viên có skill; leader có skill riêng; trùng tên; vai không có skill; mọi tên trong `skills` có trong `starter_skills` của `company.json`.
- Kiểm: `pnpm catalog:build`; `(cd server && go test ./internal/service -run 'StarterSkill' -count=1)`.
- Commit: `feat(company): attach the skills a starter role names to its agent`.

### 5.5 Dung lượng

- Đo `company.json` trước và sau. Ghi vào PR. Dự kiến tăng khoảng 110 KB (G3). Nếu tăng nhiều hơn, dừng và báo.

### 5.6 Test DB: apply, rerun, G2 và G4

Files: `server/internal/service/company_kwp_skills_db_test.go` (mới), dùng `server/internal/testutil`.

- Apply hai lần: `support_agent` có `kwp-ticket-triage`; lần hai không nhân đôi agent hay skill.
- Skill cùng tên do boss tạo trước đó không bị ghi đè.
- G4: dựng workspace bằng bản apply cũ (có `growth_gtm`, `support` với vai đã có). Apply bản mới. Kiểm vai đã có vẫn nguyên, skill mới được gắn cho đúng vai. Nếu apply bỏ qua vai thiếu trong squad đã có, sửa để tạo, có test, và cập nhật G4.
- G2: tìm lại giới hạn số skill mỗi agent; ghi kết quả vào plan.
- Khẳng định trên hàng của fixture, không trên tổng.
- Kiểm: `(cd server && go test ./internal/service -run 'Company|StarterSkill' -count=1)`.
- Commit: `test(company): check the KWP skills over the database, rerun included`.

Cổng ra Giai đoạn 5: `pnpm catalog:check`, `pnpm catalog:build`, `pnpm typecheck`, `pnpm lint`, `pnpm test`; test Go `Company|StarterSkill` xanh; ghi kích thước `company.json` và kết quả G2, G4.

---

## Giai đoạn 6: HR trong Kit Việt (PR 6, D9)

Độc lập với các giai đoạn khác. Có thể làm trước.

Files: `catalog/_sources/kitviet/extensions/people.json`, `docs/engineering/kitviet-squads.md` (mục 7, dòng ~491).

- `people.json`: `targets` đổi từ `["people_performance"]` thành `["kv_talent"]`; `skills` chỉ còn `onboarding-30-60-90`. Bốn skill `soat-hop-dong-lao-dong`, `soat-bang-cham-cong`, `quy-trinh-ky-luat-lao-dong`, `danh-gia-thu-viec` rời khỏi extension cho đến khi có squad People Ops.
- Sinh lại: `pnpm generate:kitviet`. Không sửa tay pack sinh ra.
- `kitviet-squads.md` mục 7: bỏ dòng "People & Performance ← HR vận hành"; ghi `onboarding-30-60-90` cho `kv_talent` và phần còn lại chờ squad People Ops.
- Test: `node --test scripts/generate-kitviet-pack.test.mjs` với hai ca mới: `people_performance` không có skill HR; `kv_talent` có `onboarding-30-60-90`.
- Nếu D9 chọn khác khuyến nghị, sửa đích tương ứng và ghi lại.
- Commit: `fix(squads): move onboarding to Talent Acquisition and drop the other HR skills from People & Performance`.

---

## Không làm trong kế hoạch này

- Gắn KWP cho `kv_*` (D11): hoãn. `close-plan`, `stakeholder-map`, `legal-risk-assessment` và `kwp-pre-signature-checklist` không lấy.
- `growth_analyst`, `risk_analyst`: bỏ (D8). `growth_lab` và `finance_lead` đảm nhiệm.
- Các plugin nguyên gói, MCP server, slash command, `partner-built`, `small-business` (ngoài hai nguyên tắc): không lấy.

## Phụ lục: kiểm tra trước khi gộp mỗi PR

1. `git diff --stat` chỉ chứa đường dẫn của PR. Không có tệp sinh nào (`packages/core/catalog/generated/`, `server/internal/catalog/generated/`) trong diff.
2. `pnpm catalog:check` sạch; cảnh báo còn lại (nếu có) ghi trong PR.
3. `pnpm catalog:build` chạy được; sau đó `pnpm typecheck`, `pnpm lint`, `pnpm test`; test Go liên quan xanh.
4. Nhóm A: băm vẫn khớp commit ghim, trừ hai dòng đã nêu.
5. Nhóm B và D: không còn chuỗi cấm (Giai đoạn 2.6).
6. Mô tả PR nêu: phạm vi đã đọc, kích thước `company.json` trước và sau, việc chưa chạy thật.
7. Mở PR: `gh pr create --repo TuPK/multica`.
