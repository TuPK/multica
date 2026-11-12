# Knowledge Work Plugins (KWP) squads: implementation plan

Kế hoạch triển khai cho [knowledge-work-plugins-squads.md](knowledge-work-plugins-squads.md). Đọc spec trước; các mục "10.x" dưới đây trỏ về spec. Kế hoạch này viết để giao cho agent làm từng task độc lập.

Quy ước thực hiện (theo AGENTS.md):

- Commit nhỏ, mỗi task một commit, kiểu conventional (`feat(squads): ...`). Kết thúc commit bằng dòng `Co-Authored-By` theo hướng dẫn của phiên làm việc.
- PR chỉ mở vào `--repo TuPK/multica`, không bao giờ vào `multica-ai/multica`. Mô tả PR theo mẫu (Problem, Goal, Proposal) và nêu phạm vi nội dung đã đọc (với skill đánh dấu ✱ ở spec 10.5).
- Sau mỗi task chạy kiểm tra hẹp nhất có ích; cuối mỗi giai đoạn chạy `pnpm typecheck`, `pnpm lint`, `pnpm test`, và test Go liên quan. Báo cáo đúng những gì đã chạy và những gì bỏ qua.
- Dữ liệu sinh không sửa tay: sửa nguồn rồi chạy `pnpm generate:kwp-skills` và `pnpm generate:company-template`.
- Chuỗi UI qua i18n đủ bảy ngôn ngữ (en, zh-Hans, zh-Hant, fr, es, ko, ja); mô tả card theo mục "UI descriptions" của `conventions.mdx`.
- Test mặc định **không** gọi mạng, không chạy agent CLI thật. Phần "chạy thật" chỉ chạy khi người dùng cho phép.
- Test Go dùng chung một cơ sở dữ liệu: nếu có lỗi trùng khóa, kiểm dòng sót của lần chạy bị ngắt trước khi coi là hồi quy. Không chạy hai lần song song.
- Chỉ stage đường dẫn của task mình; cây làm việc có thể có thay đổi của phiên khác. Không `git add -A`.
- Không khóa ngoại, không cascade, không migration: kế hoạch này không đụng schema.
- Sửa chữ trong skill KWP chỉ qua `kwp-rewrites.json` hoặc skill viết mới; không sửa tay `kwp-skills/` hay `kwp-skills-data.ts`.

## Nền đã có và ranh giới (đã kiểm trong mã, 2026-11-11)

- Thẻ công ty: `packages/core/squads/company-template.ts` (14 squad) → `pnpm generate:company-template` → `server/internal/service/company_template_data.json` (nhúng vào Go). `starterSkillsForRole` (`server/internal/service/company_starter_skills.go`) chỉ cho leader `squad-reporting` và `directive-execution`; thành viên không có skill. Skill starter đọc từ `catalog/skills/starter/`.
- Template quickstart: `SQUAD_TEMPLATES` trong `packages/core/squads/templates.ts`; `attachBuiltinSkills` (`quickstart.ts`, dòng ~396) gắn cấp squad bằng `setSquadSkills`. Chưa có `roleSkills` và chưa có `addAgentSkills` trong `SquadQuickstartClient`.
- Khung Kit Việt (`scripts/generate-kitviet-pack.mjs`, `scripts/lib/skill-bundle.mjs`, `roleSkills`) **chưa có trong repo** tại thời điểm viết. Cổng G1 kiểm lại.
- Mẫu agent instruction: `packages/core/agents/instruction-templates.ts` (`SHARED_BLOCKS`, `WORKING_RULES`, `SQUAD_LEADER_RULES`); tên và mô tả dịch ở `packages/views/locales/*/agents.json` theo `id`; tên và mô tả template ở `locales/*/modals.json`, khóa `templates.<id>`.
- Squad skill đến mọi thành viên (`ListEffectiveAgentSkills`); skill gắn riêng agent qua `AddAgentSkill` cộng dồn.

## Tổng quan giai đoạn

| Giai đoạn | Nội dung | Phụ thuộc | Đụng server Go | Kích cỡ | Rủi ro |
| --- | --- | --- | --- | --- | --- |
| G | Bốn cổng đọc mã | không | không | nhỏ | thấp |
| 1 · Khung (PR 1) | Vendor, chuyển đổi, generator, ba skill mới, sửa spec Kit Việt | G | không | lớn | trung bình |
| 2 · Đường thẻ công ty (PR 2) | Trường `skills` của vai, Go, generator; Support và Release & Ops | 1 | có | vừa | trung bình |
| 3 · Growth & Finance (PR 3) | Hai agent mới và skill | 2 | không | vừa | trung bình |
| 4 · Research, Product, Librarian (PR 4) | Gắn skill vào thẻ có sẵn | 3 (cùng tệp thẻ) | không | nhỏ | thấp |
| 5 · Đường template (PR 5) | `roleSkills`, `addAgentSkills`; SDLC UX, Maintenance, BMAD, Market Research, Sales Loop | 1 | không | vừa | trung bình |
| 6 · Data & Insights (PR 6) | Template mới, năm agent | 5 | không | vừa | trung bình |
| 7 · Gắn vào squad Kit Việt (PR 7) | `kv_b2b`, `kv_legal`, `kv_marketing` | 5 và squad Kit Việt tương ứng đã vào repo | không | nhỏ | thấp |

Thứ tự: G → 1 → (2 → 3 → 4) song song với (5 → 6) → 7. PR 3 và PR 4 cùng sửa `company-template.ts` nên làm tuần tự. PR 2 và PR 5 không chung tệp (PR 2: `company-template.ts`, Go; PR 5: `templates.ts`, `quickstart.ts`) nên chạy song song được.

---

## Giai đoạn G: các cổng đọc mã

Mỗi cổng ra một đoạn kết luận ngắn ghi vào spec mục 10 (đánh dấu đã đóng). Kết quả "không" thì sửa thiết kế trước khi sang Giai đoạn 1.

| Cổng | Câu hỏi | Cách trả lời | Nếu kết quả khác |
| --- | --- | --- | --- |
| G1 | Khung Kit Việt (`scripts/lib/skill-bundle.mjs`, `generate-kitviet-pack.mjs`, `SquadTemplate.roleSkills`, `addAgentSkills` trong client) đã vào repo chưa? | `ls scripts scripts/lib`; `git log --oneline -- catalog/_sources/kitviet/upstream`; đọc `quickstart.ts` | Nếu **đã có**: Task 1.1 và 5.1 dùng lại, không viết lại. Nếu **chưa**: làm theo thiết kế ở spec Kit Việt 5.10, 8.1 (PR 1 "tiếp"); khi khung Kit Việt vào sau thì hợp nhất, không để hai bản |
| G2 | Giới hạn số hoặc kích thước skill gắn cho một agent hoặc một workspace? | Đọc `server/internal/handler/skill.go`, `agent_skill`, `AddAgentSkill`, các hằng giới hạn | Nếu có giới hạn thấp hơn 6 skill/agent hay thấp hơn kích thước `build-dashboard` (25 KB): chỉnh bảng gắn trước khi làm |
| G3 | Máy chủ Go nhúng `company_template_data.json` hiện nặng bao nhiêu và có giới hạn nào (test kích thước, giới hạn payload)? | `wc -c server/internal/service/company_template_data.json`; grep test liên quan | Nếu gần ngưỡng: đổi sang tải skill KWP lúc apply (ngoài phạm vi), báo người đặt |
| G4 | Apply rerun trên workspace đã có squad: thêm vai mới vào thẻ có tạo agent mới và gắn vào squad có sẵn không? | Đọc `company_template.go`, `company_setup.go`, `phase4_company_db_test.go` | Nếu apply bỏ qua squad đã tồn tại: thêm bước "ensure member" vào Task 3.3, có test |

---

## Giai đoạn 1: khung (PR 1)

### 1.1 Thư viện dùng chung (bỏ qua nếu G1 báo đã có)

Files: `scripts/lib/skill-bundle.mjs` (mới: `skillFiles`, `frontmatterDescription`), `scripts/generate-sdlc-skills.mjs` (import lại).

- Bước: chuyển hai hàm từ `generate-sdlc-skills.mjs` sang lib; script cũ import; chạy `pnpm catalog:build`; `git diff --stat packages/core/squads/sdlc-skills-data.ts` phải rỗng.
- Kiểm: test stale của `sdlc-skills.test.ts` xanh.
- Commit: `refactor(scripts): share the skill bundle helpers between generators`.

### 1.2 Vendor upstream

Files: `packages/core/squads/kwp-upstream/{LICENSE,NOTICE.md,UPSTREAM.json}` và 43 thư mục skill.

- Clone `https://github.com/anthropics/knowledge-work-plugins.git` vào thư mục tạm, `git checkout e605d8ddbfb688da57dfaaceebd93f12a64ef1d3`. Chép **nguyên văn**, giữ cấu trúc `<plugin>/skills/<skill>/` (kèm `references/` nếu có), đúng danh sách sau (đây là cột Nguồn của spec 10.5, bỏ ba skill viết mới):
  - customer-support: `ticket-triage kb-article draft-response customer-escalation customer-research`
  - marketing: `campaign-plan performance-report seo-audit`
  - product-management: `metrics-review competitive-brief synthesize-research write-spec roadmap-update`
  - data: `statistical-analysis validate-data analyze explore-data data-context-extractor sql-queries create-viz build-dashboard`
  - operations: `vendor-review risk-assessment compliance-tracking runbook change-request status-report`
  - finance: `variance-analysis`
  - engineering: `deploy-checklist incident-response tech-debt`
  - design: `user-research design-handoff ux-copy design-critique`
  - enterprise-search: `knowledge-synthesis search-strategy`
  - sales: `win-loss-review deal-review handle-objection close-plan stakeholder-map`
  - legal: `legal-risk-assessment`
  Tổng 43 (đếm lại bằng `find kwp-upstream -name SKILL.md | wc -l`; lệch thì dừng và báo).
- Chép thêm `LICENSE` gốc (Apache-2.0). `UPSTREAM.json` và `NOTICE.md` theo spec 10.2.
- Kiểm: băm từng tệp với bản clone (script một lần, không commit). `pnpm lint` không quét thư mục này (thêm vào ignore nếu cần).
- Commit: `chore(squads): vendor 43 Knowledge Work Plugins skills at a pinned commit`.

### 1.3 Quy tắc chạy, bảng chuyển đổi và generator

Files: `packages/core/squads/kwp-runtime-rules.md`, `packages/core/squads/kwp-rewrites.json`, `scripts/generate-kwp-skills.mjs`, `package.json` (script `generate:kwp-skills`), `packages/core/squads/kwp-skills/**` và `kwp-skills-data.ts` (sinh).

- Việc 1: viết `kwp-runtime-rules.md` (spec 10.3 mục 8), ≤ 25 dòng, tiếng Anh (văn bản dành cho agent).
- Việc 2: viết generator theo spec 10.2 và 10.3: áp chuyển đổi chung (bước 1 đến 8), rồi mục riêng trong `kwp-rewrites.json`; báo lỗi rõ khi mỏ neo không tìm thấy, khi `~~` còn sót, khi hai skill trùng tên; hỗ trợ `--check` (so với tệp đã sinh, thoát mã khác 0 nếu cũ).
- Việc 3: **mỗi skill một mục `kwp-rewrites.json`**, theo cột "Chỉnh sửa riêng" của spec 10.5. Với skill ✱: đọc trọn `SKILL.md` và `references/` trước, rồi viết mục; ghi vào mô tả PR "đã đọc trọn: …". Làm theo từng nhóm, commit riêng cho mỗi nhóm (bên dưới).
- Việc 4: `kwp-skills-data.ts` xuất `KWP_BUILTIN_SKILLS` cùng dạng `BuiltinSkill` như `SDLC_BUILTIN_SKILLS`. Không import nó ở đâu ngoài chỗ nạp động (Task 5.1 và 2.2).
- Kiểm: `pnpm generate:kwp-skills --check`; test chuỗi cấm (Task 1.5).
- Commit (một cho khung, rồi sáu cho nhóm): 
  1. `feat(squads): add the generator that turns KWP skills into Multica skills`
  2. `feat(squads): convert the KWP customer-support and operations skills` (Support, Release & Ops, Finance & Risk)
  3. `feat(squads): convert the KWP marketing and product-management skills`
  4. `feat(squads): convert the KWP data skills`
  5. `feat(squads): convert the KWP design, engineering and enterprise-search skills`
  6. `feat(squads): convert the KWP sales and legal skills`

### 1.4 Ba skill viết mới

Files: `packages/core/squads/kwp-skills/{kwp-untrusted-content,kwp-absent-is-not-zero,kwp-pre-signature-checklist}/SKILL.md`.

- Nguồn: `small-business/shared/untrusted-content.md`, `absent-is-not-zero.md` ở upstream (đọc từ bản clone; không vendor cả thư mục `small-business`), và phần checklist của `legal/skills/signature-request`. Nội dung theo spec 10.4. Mỗi skill: frontmatter (`name`, `description` mở đầu "Use when you are …"), ≤ 90 dòng, tiếng Anh, có dòng ghi nguồn và giấy phép.
- Generator đọc thư mục `kwp-skills/` có sẵn hai loại: thư mục sinh ra và thư mục viết tay. Phân biệt bằng danh sách `handwritten` trong `kwp-rewrites.json`; thư mục không thuộc cả hai là lỗi.
- Commit: `feat(squads): add the untrusted-content, absent-is-not-zero and pre-signature skills`.

### 1.5 Test khung

Files: `packages/core/squads/kwp-skills.test.ts` (đầu tệp `// @vitest-environment node`).

- Kiểm: (a) dữ liệu sinh không cũ; (b) 46 skill, mỗi tên `kwp-…`, tên bằng thư mục, mô tả ≤ 300 ký tự; (c) không chuỗi cấm (spec 10.3, danh sách đầy đủ); (d) mọi skill có khối `## Multica runtime rules`; (e) không skill nào còn dòng bắt đầu bằng `/` trong khối mã dạng lệnh; (f) kích thước tổng ≤ 600 KB.
- Chạy: `pnpm --filter @multica/core exec vitest run squads/kwp-skills.test.ts`.
- Commit: `test(squads): check the converted KWP skills for forbidden steps`.

### 1.6 Sửa spec Kit Việt

Files: `docs/engineering/kitviet-squads.md`.

- Làm đúng spec 10.8 (ba ý). Không đổi nội dung khác.
- Kiểm: `git diff --check`; đọc lại mục 7 và 5.10.
- Commit: `docs(squads): move HR skills out of People & Performance and reference KWP`.

Cổng ra Giai đoạn 1: test xanh; `pnpm generate:kwp-skills --check` sạch; chưa có squad nào dùng skill.

---

## Giai đoạn 2: đường thẻ công ty (PR 2)

### 2.1 Trường `skills` của vai

Files: `packages/core/squads/company-template.ts`, `company-template.test.ts`.

- Thêm `skills: z.array(z.string().min(1)).default([])` vào `CompanyRoleSchema`. `superRefine` của thẻ kiểm mỗi tên có thư mục trong `kwp-skills/` hoặc `skills/` (đọc bằng `fs` trong test; schema chỉ kiểm dạng tên để không kéo `fs` vào gói trình duyệt). Test mới: tên lạ thì fail; vai không `skills` thì `[]`.
- Chưa gắn skill nào ở task này.
- Kiểm: `pnpm --filter @multica/core exec vitest run squads/company-template.test.ts`.
- Commit: `feat(squads): let a company role name the skills it gets`.

### 2.2 Generator thẻ công ty

Files: `scripts/generate-company-template.mjs`, `company-template-data.test.ts`.

- Sau khi dựng `starter_skills` từ `squads/skills`, thêm skill `kwp-*` mà vai nhắc đến (đọc từ `squads/kwp-skills/<tên>` giống cách đọc `SKILL.md` và tệp phụ hiện có). Tên thiếu thì generator lỗi.
- Test: mỗi tên trong `skills` của mọi vai có mặt trong `starter_skills` của JSON sinh; JSON khớp hằng số.
- Kiểm: `pnpm generate:company-template` rồi `pnpm --filter @multica/core exec vitest run squads/company-template-data.test.ts`.
- Commit: `feat(squads): ship the KWP skills a company role names in the starter data`.

### 2.3 Máy chủ Go

Files: `server/internal/service/company_template_data.go` (thêm `Skills []string` với thẻ `json:"skills"` vào `companyRole`), `company_starter_skills.go`, `company_starter_skills_test.go`.

- `starterSkillsForRole`: leader nhận hai skill hiện có cộng `role.Skills`; vai khác nhận `role.Skills`; loại trùng, giữ thứ tự (leader skills trước); Secretary không đổi.
- Test bảng: thêm ca "thành viên có skill", "leader có skill riêng", "trùng tên", "role không có skill".
- Kiểm: `(cd server && go test ./internal/service -run 'StarterSkill' -count=1)`.
- Commit: `feat(company): attach the skills a role names to its agent`.

### 2.4 Support và Release & Ops

Files: `packages/core/squads/company-template.ts` (vai `support_lead`, `support_agent`, `release_lead`, `release_engineer`), JSON sinh, `company-template.test.ts`.

- Điền `skills` theo bảng spec 10.6 cho bốn vai này. Chạy `pnpm generate:company-template`.
- Test: số skill mỗi vai ≤ 6; `support_agent` có `kwp-untrusted-content`; không vai nào có hai skill cùng nhóm trùng (xem spec mục 7: không có nhóm trùng nào ở bốn vai này, test chỉ ghi nhận).
- Test có DB: trong `phase4_company_db_test.go` (hoặc tệp mới `company_kwp_skills_db_test.go`), chạy apply hai lần cho thẻ `support` và `release_ops`; khẳng định agent `Support Agent` có `kwp-ticket-triage`, không nhân đôi agent hay skill ở lần hai, và một skill `kwp-ticket-triage` do boss tạo trước đó không bị ghi đè (dùng `testutil` và `dbfx`; khẳng định trên hàng của fixture, không trên tổng).
- Kiểm: `pnpm --filter @multica/core exec vitest run squads/company-template`; `(cd server && go test ./internal/service -run 'Company|StarterSkill' -count=1)`.
- Commit: `feat(squads): give Support and Release & Ops their KWP skills`.

Cổng ra Giai đoạn 2: `pnpm typecheck`, `pnpm lint`, `pnpm test`; `pnpm generate:company-template` không đổi tệp; kích thước `company_template_data.json` tăng bao nhiêu ghi vào mô tả PR.

---

## Giai đoạn 3: Growth & GTM và Finance & Risk (PR 3)

### 3.1 Hai mẫu agent

Files: `packages/core/agents/instruction-templates.ts`, `instruction-templates.test.ts`, `packages/views/locales/{en,es,fr,ja,ko,zh-Hans,zh-Hant}/agents.json`, `packages/views/i18n/resources-types.ts` (nếu kiểu khóa tạo từ tệp en thì chỉ cần cập nhật en).

- Thêm id `growth_analyst`, `risk_analyst` vào `AgentInstructionTemplate["id"]` và hai khối `instructions` theo spec 10.6 mục 2 (mỗi khối kết bằng `${SHARED_BLOCKS}` và `${WORKING_RULES}`; không `PIPELINE_OWNER_RULES`, đây là thành viên). Văn bản dành cho agent bằng tiếng Anh, theo mẫu `support_agent`.
- Bản dịch tên và mô tả: đặt cạnh `growth_lead` (khóa tương tự); mô tả ≤ 200 ký tự, không nhắc lại tên.
- Test: test hiện có của `instruction-templates.test.ts` (mỗi id có bản dịch ở cả bảy ngôn ngữ, mỗi mẫu kết bằng quy tắc chung) phải xanh; thêm ca cho hai id mới nếu test hiện chưa duyệt toàn bộ id.
- Kiểm: `pnpm --filter @multica/core exec vitest run agents/instruction-templates.test.ts`; `pnpm --filter @multica/views exec vitest run i18n` (nếu có test kiểm khóa dịch).
- Commit: `feat(agents): add the growth analyst and risk analyst instruction templates`.

### 3.2 Vai mới trong hai thẻ

Files: `packages/core/squads/company-template.ts`, test, JSON sinh.

- Thẻ `growth_gtm` thêm vai `growth_analyst` (runtime `worker`, `agentTemplate: "growth_analyst"`, `role`: "Metrics reports"); thẻ `finance_risk` thêm `risk_analyst` ("Risk register"). Charter hai squad thêm một dòng `responsibilities` (spec 10.6 mục 3). Điền `skills` cho **bốn** vai `growth_lead`, `growth_analyst`, `finance_lead`, `risk_analyst` theo bảng spec 10.6.
- `pnpm generate:company-template`; test `company-template.test.ts` vẫn `toHaveLength(14)`; test mới: hai vai mới có mặt, leader không đổi.
- Commit: `feat(squads): add the growth analyst and risk analyst to the starter company`.

### 3.3 Rerun trên workspace có sẵn

Files: test Go như Task 2.4; sửa `company_template.go`/`company_setup.go` chỉ khi G4 cho biết cần.

- Test có DB: dựng workspace đã apply bản cũ (squad `growth_gtm` có `Growth Lead`); apply bản mới; khẳng định `Growth Analyst` được tạo, là thành viên squad, có `kwp-metrics-review`, và `Growth Lead` giữ nguyên.
- Kiểm: `(cd server && go test ./internal/service ./internal/handler -run 'Company' -count=1)`.
- Commit: `test(company): rerun the starter company over a workspace that predates the analysts`.

Cổng ra: bộ test đầy đủ xanh; ghi vào ghi chú phát hành: "rerun thêm hai agent vào Growth & GTM và Finance & Risk".

---

## Giai đoạn 4: Research, Product, Executive Office (PR 4)

### 4.1 Gắn skill vào các thẻ còn lại

Files: `packages/core/squads/company-template.ts`, JSON sinh, `company-template.test.ts`.

- Điền `skills` cho: `librarian`, hai `critic` (Market Research, Opportunity Lab), `competitor_analyst`, `customer_lead`, `validation_analyst`, `prd_writer`, theo bảng spec 10.6. Chạy `pnpm generate:company-template`.
- Test: `kwp-write-spec` chỉ xuất hiện ở `prd_writer`; `kwp-validate-data` chỉ ở các vai gate và `validation_analyst`; không vai nào quá 6 skill.
- Kiểm: như Task 2.4.
- Commit: `feat(squads): give the research, product and librarian roles their KWP skills`.

Cổng ra: `pnpm typecheck`, `pnpm lint`, `pnpm test`, test Go `Company|StarterSkill`.

---

## Giai đoạn 5: đường template (PR 5)

### 5.1 `roleSkills` và `addAgentSkills` (bỏ qua nếu G1 báo đã có)

Files: `packages/core/squads/templates.ts`, `quickstart.ts`, `quickstart.test.ts`, `packages/core/api/client.ts`, hook `useSquadQuickstart` ở `packages/views/` (đường dẫn cụ thể: grep `useSquadQuickstart`).

- `SquadTemplate.roleSkills?` (spec 10.7). `SquadQuickstartClient` thêm `addAgentSkills(agentId, skillIds)`; nối client thật tới `POST /agents/{id}/skills/add` (`AddAgentSkills` trong `server/cmd/server/router.go`, dòng ~2678); kiểm `packages/core/api/client.ts` đã có phương thức tương ứng chưa.
- `attachBuiltinSkills` đổi chữ ký để nhận tên từ hai nguồn: `SDLC_BUILTIN_SKILLS` (đồng bộ) và `kwp-*` qua `await import("./kwp-skills-data")`. Giữ nguyên hành vi hiện tại cho tên khác.
- Trong luồng tạo squad (dòng ~542 và ~593 của `quickstart.ts`): sau khi có agent theo role key, gắn `roleSkills` bằng skill id đã tạo hoặc tìm theo tên; lỗi một skill không hủy việc tạo squad (như `.catch` hiện có), ghi vào `outcome`.
- Test đơn vị (client giả): vai nhận đúng skill; skill thiếu trong bundle thì báo trong outcome, squad vẫn tạo; mô-đun `kwp-skills-data` chỉ nạp khi template có `roleSkills`.
- Kiểm: `pnpm --filter @multica/core exec vitest run squads/quickstart.test.ts`.
- Commit: `feat(squads): attach skills to single roles when a template starts a squad`.

### 5.2 Gắn skill vào template có sẵn

Files: `packages/core/squads/templates.ts`, `templates.test.ts`.

- Điền `roleSkills` theo bảng spec 10.7 cho `sdlc_greenfield` (`ux`), `sdlc_maintenance` (`ux`, `lead`), `bmad_planning` (`plan_ux`), `market_research` (`competitor`, `critic`), `sales_loop` (`sales_analyst`).
- Test: `roleSkills` chỉ nêu vai thuộc template; mỗi vai ≤ 6; `kwp-write-spec` không xuất hiện ở bất kỳ template nào; skill `accessibility-*` vẫn trong `builtinSkills` của SDLC (không bị thay).
- Kiểm: `pnpm --filter @multica/core exec vitest run squads/templates.test.ts`.
- Commit: `feat(squads): give the SDLC, planning, research and sales templates their KWP skills`.

Cổng ra: `pnpm typecheck`, `pnpm lint`, `pnpm test`; chạy thử quickstart `sdlc_maintenance` trên workspace thử và xác nhận `SDLC UX Designer` có ba skill và `SDLC Lead` có `kwp-tech-debt` (cần người cho phép nếu chạy thật; không thì ghi "chưa chạy thật").

---

## Giai đoạn 6: Data & Insights (PR 6)

### 6.1 Năm mẫu agent

Files: `packages/core/agents/instruction-templates.ts`, test, `locales/*/agents.json` (7 ngôn ngữ).

- Thêm id `data_lead`, `data_profiler`, `data_analyst`, `data_viz`, `data_validator`. Nội dung theo spec 10.7 (luồng, chỉ đọc, `VERDICT:`, tối đa 2 vòng sửa); `data_lead` kết bằng `${SQUAD_LEADER_RULES}`; tất cả kết bằng `${SHARED_BLOCKS}` và `${WORKING_RULES}`. Lead tự đối chiếu kết quả với câu hỏi ban đầu (không vai bàn giao ở bản đầu).
- Commit: `feat(agents): add the data analysis instruction templates`.

### 6.2 Template `data_insights`

Files: `templates.ts` (`SquadTemplateId`, `SquadRoleKey`, `SQUAD_ROLES`, `SQUAD_TEMPLATES`, hằng `DATA_INSIGHTS_INSTRUCTIONS`), `templates.test.ts`, `packages/views/locales/*/modals.json` (khóa `templates.data_insights`), `squad-quickstart` modal và `company-setup-card.tsx` chỉ sửa nếu cổng G (của Kit Việt G3) chưa xác nhận chúng duyệt `SQUAD_TEMPLATES`.

- Theo bảng spec 10.7: năm vai, leader `data_lead`, `floorName: "Product"`, charter, `routines: []`, `workflow: null`, `builtinSkills: []`, `roleSkills` như bảng. `DATA_INSIGHTS_INSTRUCTIONS` là văn bản lead đọc (theo mẫu `MARKET_RESEARCH_INSTRUCTIONS`, nêu điều kiện chỉ đọc và `blocked` khi thiếu quyền).
- Test: đủ vai và leader; `roleSkills` hợp lệ; `floorName` là "Product"; số template tăng 1; bản dịch bảy ngôn ngữ có khóa mới.
- Kiểm: `pnpm --filter @multica/core exec vitest run squads/templates.test.ts`; `pnpm --filter @multica/views exec vitest run` cho các test modal liên quan.
- Commit: `feat(squads): add the Data & Insights squad template`.

### 6.3 Diễn tập (chỉ khi người dùng cho phép)

Ba lần chạy thật theo spec 10.9: đủ dữ liệu, thiếu quyền đọc (kỳ vọng `blocked`), truy vấn có join nổ (kỳ vọng `VERDICT: CHẶN`). Dán biên bản vào PR. Không chạy được thì ghi "chưa chạy thật".

Cổng ra: `pnpm typecheck`, `pnpm lint`, `pnpm test`; `make check` nếu thay đổi chạm cả web và server.

---

## Giai đoạn 7: gắn vào squad Kit Việt (PR 7)

Chỉ làm khi `kv_b2b`, `kv_legal`, `kv_marketing` đã có trong `SQUAD_TEMPLATES` (spec Kit Việt PR 3, 4, 5). Nếu chưa, giữ giai đoạn này và không đoán tên vai.

### 7.1 Gắn skill

Files: `packages/core/squads/kitviet-templates.ts` (hoặc tệp template Kit Việt hiện hành), test của tệp đó.

- Thêm vào `roleSkills` hiện có (không ghi đè): `b2b_quote_drafter` ← `kwp-close-plan`; `b2b_company_lookup` ← `kwp-stakeholder-map`; `legal_lead` ← `kwp-legal-risk-assessment`; `legal_handover` ← `kwp-pre-signature-checklist`; `mkt_scout` ← `kwp-seo-audit`.
- Test: sau khi thêm, mỗi vai vẫn ≤ 6 skill tính cả skill Kit Việt; `kwp-close-plan` có `b2b_commit_gate` trong cùng squad; không skill KWP nào nằm ở vai gate (trừ `kwp-validate-data` ở Data).
- Commit: `feat(squads): add the KWP skills to the Kit Viet B2B, Legal and Marketing squads`.

---

## Phụ lục: kiểm tra chéo trước khi gộp từng PR

1. `git diff --stat` chỉ chứa đường dẫn của PR; không có `kwp-skills-data.ts` sửa tay (so với đầu ra generator).
2. `pnpm generate:kwp-skills --check` và `pnpm generate:company-template` không đổi tệp.
3. Không còn chuỗi cấm trong `kwp-skills/` (spec 10.3); không `~~`.
4. Mô tả PR có: phạm vi đã đọc, kích thước thay đổi (`company_template_data.json`, chunk trình duyệt), việc chưa chạy thật.
5. PR mở bằng `gh pr create --repo TuPK/multica`. Sau khi mở PR: `get_status` và `bind_pr` nếu chưa gắn.
