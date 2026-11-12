# Template Registry: đưa nội dung template ra thư mục riêng và thêm template không cần sửa code

Trạng thái: bản nháp để duyệt (chưa triển khai). Kế hoạch đi kèm: [plan.md](plan.md).
Ngày khảo sát: 2026-11-12, trên cây làm việc có nhiều thay đổi chưa commit của người dùng (xem mục 4.6).

Ngôn ngữ: tiếng Việt. Định danh, đường dẫn và lệnh giữ nguyên tiếng Anh.
Quy ước đánh dấu: đường dẫn kèm chữ "(mới)" là đường dẫn đề xuất, chưa tồn tại. Mọi đường dẫn còn lại đã được kiểm tra tồn tại bằng `git ls-files` hoặc `ls`.

## 1. Tóm tắt

Nội dung "template" của Multica (skill, squad, agent, workflow, công ty mẫu, agent pack) hiện nằm rải rác ở `packages/core/squads/`, `packages/core/agents/`, `packages/views/locales/`, `server/internal/` và `examples/`. Thêm một squad hoặc một skill phải sửa tay từ 4 đến 8 chỗ trong code, trong đó có hai kiểu union TypeScript, một mảng đăng ký, và 7 file dịch.

Đề xuất (phương án B trong mục 5): đặt toàn bộ nội dung nguồn dưới một thư mục gốc `templates/` (mới). Mỗi template là một thư mục có `template.json` (manifest) cộng file nội dung. Một CLI trong `scripts/templates/` (mới) kiểm tra manifest, rồi sinh ra các file TypeScript và JSON đã commit sẵn cho `packages/core`, `packages/views` và `server/`. Thêm template = tạo thư mục, chạy `pnpm templates:build`, commit. Không sửa file code nào.

Giữ lại những gì repo đã làm tốt: nguồn là file, đầu ra sinh ra được commit và có test chặn lệch, tải lazy theo gói như `KITVIET_PACK_LOADERS`. Thay những gì đang gây đau: danh sách đăng ký viết tay, nội dung lặp ở ba nơi (TypeScript, Go), ba cơ chế kiểm tra độ tươi khác nhau, và bundle 2,1 MB nạp sớm.

## 2. Vấn đề

1. Nội dung hay đổi (SKILL.md, hướng dẫn của agent, charter, workflow) nằm lẫn với logic lõi trong `packages/core/squads/`: tám thư mục con chứa nội dung template có tổng 1.087 file được git theo dõi, cạnh các file `.ts` của engine. Người không phải lập trình viên khó tìm đúng chỗ sửa.
2. Thêm một template phải chạm nhiều file code (mục 4.5). Quên một chỗ thì typecheck hoặc test báo lỗi, đó là tín hiệu tốt, nhưng người thêm vẫn phải biết toàn bộ danh sách chỗ.
3. Cùng một nội dung được giữ ở nhiều nơi, chỉ ràng buộc bằng comment hoặc test: workflow "SDLC Auto" có ở TypeScript và hai nơi trong Go (mục 4.4, C5).
4. Dữ liệu được tác giả viết bằng TypeScript rồi sinh JSON cho Go (`scripts/generate-company-template.mjs` bundle bằng esbuild). Go phụ thuộc chuỗi công cụ Node, và người viết dữ liệu phải sửa code TypeScript.
5. Có ba cơ chế kiểm tra "file sinh ra còn mới không" (mục 4.4, C9), và `ci-paths.json`, Dockerfile, turbo đều giả định nội dung nằm trong `packages/` hoặc `server/`.

## 3. Mục tiêu và không phải mục tiêu

Mục tiêu:

- G1. Một thư mục gốc `templates/` chứa toàn bộ nội dung template nguồn. Phân loại rõ "lõi" và "template" bằng tiêu chí kiểm tra được (mục 4.1).
- G2. Thêm skill, skillset, squad, workflow, collection, agent pack mới không cần sửa file code. Có lệnh scaffold và lệnh kiểm tra.
- G3. Một nguồn sự thật cho nội dung dùng ở cả Go và TypeScript, sinh ra bằng một lệnh.
- G4. Không đổi hành vi người dùng thấy. Mỗi bước di chuyển chứng minh được bằng băm nội dung trước và sau (mục 7, lớp L6).
- G5. Manifest có phiên bản, id bất biến, phụ thuộc khai báo được, dịch tách khỏi nội dung, kiểm tra bằng schema.

Không phải mục tiêu (xem thêm "Ngoài phạm vi" trong plan):

- Kho template chạy lúc runtime, chợ, template riêng theo workspace, giao diện soạn template. Thiết kế chừa đường nối nhưng không làm (mục 5, phương án C).
- Đổi nội dung prompt, đổi hành vi quickstart, đổi schema DB. Không có migration.
- Skill nền tảng của máy chủ (`server/internal/service/builtin_skills/`) và agent hệ thống Mika: đây là hợp đồng với daemon, giữ nguyên là lõi.
- `examples/plugins/`: là fixture của 3 test Go, giữ nguyên chỗ.
- Dịch nội dung dành cho agent (SKILL.md, instructions). Chỉ dịch phần hiển thị cho người dùng (tên, mô tả thẻ).

## 4. Hiện trạng (kết quả rà soát)

### 4.1 Ranh giới "lõi" và "template"

Một mục là template khi trả lời "không" câu 1 và "có" câu 3 và 4:

| # | Câu hỏi | Nếu trả lời |
| --- | --- | --- |
| 1 | Thêm một thể hiện mới có cần thêm nhánh code (`switch`, `if` theo id, kiểu union) không? | Có: lõi hoặc template bị ghim (mục 6.3, trường `contracts`) |
| 2 | Code Go hoặc TS có tham chiếu thể hiện này theo tên hoặc id không (hợp đồng)? | Có: template bị ghim, phải khai báo `contracts` |
| 3 | Vòng đời thay đổi có độc lập với bản phát hành của engine không (đổi hàng tuần, tăng dần)? | Có: template |
| 4 | Người không viết code có sửa được bằng văn bản hoặc JSON không? | Có: template |

Hệ quả:

- Lõi: engine quickstart (`packages/core/squads/quickstart.ts`, `upgrade.ts`, `handoff.ts`), engine workflow (`server/internal/issueworkflow/`), skill nền tảng của server, agent Mika, schema zod, generator, nội dung giao diện (nhãn, nút).
- Template: SKILL.md và `references/`, hướng dẫn agent, charter, routine, workflow mẫu, công ty mẫu, agent pack, danh mục nguồn sales.
- Template bị ghim: những template mà Go tham chiếu theo tên, ví dụ squad SDLC (`SDLC Developer`...), workflow `BMAD Planning Auto` với lead `Blueprint Lead`, squad `executive_office`. Nội dung vẫn là template, nhưng hợp đồng phải khai báo và có test.

### 4.2 Bảng kiểm kê

Kích thước là tổng byte của file được theo dõi bởi git (`git ls-files` + `du -cb`), trừ khi ghi khác. "Cách nạp" mô tả đường từ nguồn đến chỗ dùng.

| ID | Đường dẫn | Loại | Quy mô | Cách nạp | Code đọc |
| --- | --- | --- | --- | --- | --- |
| T1 | `packages/core/squads/sdlc-skills/` | skill | 77 thư mục, 390 file, 2,0 MB (bmad 25, superpowers 15, sdlc 12, sales 7, planning 7, accessibility 5, supabase 2, còn lại 4) | Sinh bởi `scripts/generate-sdlc-skills.mjs` thành `packages/core/squads/sdlc-skills-data.ts` (2,1 MB, commit), import tĩnh | `packages/core/squads/templates.ts`, `quickstart.ts` (`importBuiltinSkills`), type trong `kitviet-packs.ts` |
| T2 | `packages/core/squads/skills/` | skill (công ty mẫu) | 10 skill, 20 file, 57 KB | `scripts/generate-company-template.mjs` nhúng vào `server/internal/service/company_template_data.json` (337 KB) | Go: `server/internal/service/company_starter_skills.go`, `company_template_data.go` |
| T3 | `packages/core/squads/kitviet-upstream/` | nguồn vendor (MIT) | 515 file, 2,8 MB: 361 SKILL.md, 138 agent, 20 nghề | Đầu vào của `scripts/generate-kitviet-pack.mjs`, không bao giờ nạp lúc chạy | Generator |
| T4 | `packages/core/squads/kitviet-patches/`, `kitviet-rewrites.json` | quy tắc biến đổi | 91 file 63 KB; 6 KB | Đầu vào generator | Generator, `scripts/generate-kitviet-pack.test.mjs` |
| T5 | `packages/core/squads/kitviet-squads/`, `kitviet-extensions/`, `kitviet-skills/` | cấu hình squad, mở rộng, skill viết tay | 22 JSON (21 squad + `base.json`) 37 KB; 3 JSON 1,5 KB; 3 skill 27 KB | Đầu vào generator | Generator |
| T6 | `packages/core/squads/kitviet-skills-data/` | đầu ra sinh | 29 file .ts, 2,85 MB | Tải lazy theo pack qua `KITVIET_PACK_LOADERS` (`kitviet-skills-data/packs.ts`, `kitviet-packs.ts`) | `quickstart.ts` |
| T7 | `packages/core/squads/kitviet-templates/` | squad (viết tay) | 21 file `.ts` + 21 test, 330 KB: charter, routine, mô tả, role key | Import tĩnh trong `templates.ts` (21 dòng import) | `templates.ts` |
| T8 | `packages/core/squads/templates.ts` | squad + workflow + danh sách skill | 53 KB, 1.043 dòng: 5 squad nền (`sdlc_greenfield`, `sdlc_maintenance`, `market_research`, `bmad_planning`, `sales_loop`), `SDLC_AUTO_WORKFLOW`, `PLANNING_AUTO_WORKFLOW`, `PLANNING_SKILL_NAMES`, `SALES_SKILL_NAMES` | Import tĩnh | `packages/views/modals/squad-quickstart.tsx`, `packages/views/company/components/company-setup-card.tsx`, `company-starter-wizard.tsx` |
| T9 | `packages/core/squads/sdlc-agent-instructions.ts`, `planning-agent-instructions.ts`, `sales-agent-instructions.ts`, `kitviet-agent-instructions.ts`, `kitviet-squad-roles.ts`, `kitviet-floors.ts` | prompt agent theo vai | 6,7 KB, 5,7 KB, 3,8 KB, 66,6 KB (bộ ghép + khối văn bản), 3,9 KB, 1 KB | Import tĩnh | `quickstart.ts` (`instructionsFor`) |
| T10 | `packages/core/squads/kitviet-previous-texts.ts`, `sdlc-previous-texts.ts` | lịch sử văn bản (chỉ thêm) | 2,6 MB; 35 KB | Import tĩnh | `packages/core/squads/upgrade.ts`, `scripts/upgrade-sdlc-squad.mjs` |
| T11 | `packages/core/squads/company-template.ts`, `packages/core/agents/instruction-templates.ts`, `instruction-blocks.ts` | công ty mẫu (14 thẻ squad, 7 tầng, chủ stage) + 39 mẫu hướng dẫn agent | 45 KB; 76 KB | TS là nguồn; `generate-company-template.mjs` sinh `server/internal/service/company_template_data.json`; Go nhúng bằng `go:embed` | Go: `server/internal/service/company_template.go` (1.174 dòng), `company_setup.go`; TS: wizard, `agents/components/instruction-template-picker.tsx` |
| T12 | `packages/core/squads/sales-starter-sources.ts`, `sales-recipe.ts`, `sales-intake.ts` | danh mục nguồn sales | 30 KB, 4 KB | Import tĩnh | `quickstart.ts` (`ensureStarterSources`); Go `server/internal/salescollect/` kiểm cùng hình dạng recipe |
| T13 | Workflow: `SDLC_AUTO_WORKFLOW` (`templates.ts:378`), `PLANNING_AUTO_WORKFLOW` (`:495`); Go `sdlcSteps` (`server/internal/service/initiative_build.go:325`); Go `server/internal/issueworkflow/templates.go` (226 dòng, 7 template, chưa theo dõi bởi git); phía trình thiết kế (chưa theo dõi bởi git): union `IssueWorkflowTemplateId` ở `packages/core/types/issue-workflow.ts:227`, tập `ISSUE_WORKFLOW_TEMPLATE_IDS` ở `packages/core/api/schemas.ts`, danh sách 7 id viết tay trong `packages/views/workflow/designer/template-picker-dialog.tsx`, bản mô phỏng `template-mock.ts`; khóa dịch `packages/views/locales/*/workflow.json` (`templates`) | workflow | SDLC Auto có ở 3 nơi; id workflow template có ở 5 nơi | TS và Go riêng biệt | Quickstart (`workflowSteps`), `InitiativeService`, trình thiết kế workflow |
| T14 | `examples/agent-packs/market-strategist/` | agent pack | 543 KB: `pack.json`, `instructions.md`, 19 skill, giấy phép | `scripts/build-agent-pack-bundles.mjs` sinh `packages/core/agent-packs/bundles/market-strategist.json` (553 KB); `AGENT_PACK_IDS` viết tay trong `packages/core/agent-packs/load.ts:3` | `packages/views/modals/agent-pack-install.tsx:32` (`AGENT_PACK_IDS[0]`), `scripts/install-agent-pack.sh`, `Makefile` (`agent-pack`, `agent-pack-bundles`) |
| T15 | `examples/plugins/` (5 plugin mẫu) | plugin mẫu / fixture | 80 KB | Không nạp lúc chạy. 3 test Go đọc bằng đường tương đối `../../../examples/plugins` | `server/internal/handler/plugin_example_test.go`, `server/cmd/server/plugin_schedule_pulse_example_test.go`, `server/pkg/plugincontract/examples_test.go` |
| T16 | `server/internal/service/builtin_skills/` (2 skill, 11 file, 145 KB), `builtin_skills_legacy/` (1 file), `builtin_agents/mika/INSTRUCTIONS.md`, `server/internal/quickstart/bundle.json` (907 B) | skill nền tảng, agent hệ thống, danh sách cài Claude Code | | `go:embed` | `server/internal/service/builtin_skills.go`, `builtin_agents.go`, `server/internal/quickstart/bundle.go` |
| T17 | `packages/views/locales/{en,es,fr,ja,ko,zh-Hans,zh-Hant}/modals.json` (`squad_quickstart.templates`: 26 mục mỗi ngôn ngữ; `groups`; mục `market-strategist`), `agents.json` (`creation_studio.instruction_templates`), `workflow.json` (`templates`) | chuỗi hiển thị theo id template | | i18next, khóa gõ kiểu: `$.squad_quickstart.templates[tpl.id].name` (`squad-quickstart.tsx:162`) | `packages/views/locales/parity.test.ts` bắt buộc đủ khóa ở 7 ngôn ngữ |
| T18 | `docs/engineering/kitviet-*.md`, `sdlc-*.md`, `skill-plugin-hub.md`... | tài liệu | 43 file trong `docs/engineering/` | | Trích đường dẫn của T1 đến T14 |

Ngoài kiểm kê (không phải template): `.agents/skills/` và `skills-lock.json` (skill cho người đóng góp repo), `server/internal/handler/testdata/quick_action_bodies.json` (fixture test), `server/internal/handler/qoder_catalog.go` (client API của runtime), `pixtuoid/` (dự án Rust riêng).

### 4.3 Phân loại tóm tắt

| Nhóm | Mục | Quyết định |
| --- | --- | --- |
| Template, chuyển ra `templates/` | T1, T2, T3, T4, T5, T7, T8 (phần dữ liệu), T9 (phần văn bản), T11, T12, T13, T14, T16 `bundle.json`, T17 (khóa theo id) | Di chuyển theo plan |
| Template bị ghim | squad SDLC, `BMAD Planning Auto`, `executive_office`, các khóa chủ stage trong `server/internal/service/pipeline_install.go:97-121` | Chuyển, kèm `contracts` và test Go |
| Lõi | T6 và đầu ra sinh khác (là kết quả, không phải nguồn), T10 (giữ ở bước đầu, xem mục 6.9 về băm), T15, T16 (skill nền tảng, Mika), engine, schema | Giữ trong code |

### 4.4 Các điểm ghép nối cứng

Mỗi điểm dưới đây là chỗ phải sửa tay hoặc có thể lệch khi thêm template.

- C1. `SquadTemplateId` là union 26 phần tử (`packages/core/squads/templates.ts:33`). `SquadRoleKey` là union ghép từ 21 kiểu role của từng squad (`templates.ts:91`). 21 dòng import tĩnh (`templates.ts:3-23`). Danh sách đăng ký và phân nhóm viết tay (`templates.ts:1016`, hàm `inCategory` ở `:1011`).
- C2. Mỗi template cần 26 khóa × 7 ngôn ngữ trong `modals.json`. Khóa gõ kiểu bắt buộc phải đủ, nên thêm id mà quên bản dịch thì typecheck đỏ. Hành vi này đúng, nhưng buộc sửa 7 file giao diện cho một nội dung thuộc về template.
- C3. Nhóm skill theo tiền tố tên: `NON_SDLC_SKILLS` lọc theo `planning-`, `sales-`, `bmad-toolbox:`, `frontend-design:frontend-design` (`templates.ts`, dòng 703-713). `PLANNING_SKILL_NAMES` (`:656`) và `SALES_SKILL_NAMES` (`:692`) là danh sách viết tay. `packages/core/squads/sdlc-skills.test.ts` liệt kê tay `ROLE_SKILLS` và `PLANNING_ROLE_SKILLS`, chỉ phần `TOOL_SKILLS` tự suy ra từ thư mục.
- C4. Gói 2,1 MB nạp sớm. `templates.ts` import tĩnh `SDLC_BUILTIN_SKILLS` và tính `SDLC_SKILL_NAMES` lúc nạp module, nên bundler khó loại bỏ. Barrel `packages/core/squads/index.ts` xuất lại `templates`, và ít nhất `packages/views/issues/components/handoff-section.tsx` import từ barrel. Chưa đo xem 2,1 MB có thật sự vào bundle trình duyệt không (xem T0.5 trong plan).
- C5. Lặp nội dung giữa TS và Go. SDLC Auto có ở `templates.ts:378`, `server/internal/service/initiative_build.go:325` (`sdlcSteps`), và `server/internal/issueworkflow/templates.go` (`sdlcAutoTemplate`). Tên vai ("SDLC Lead", "SDLC Developer"...) có ở `company-template.ts:705`, `BASE_SQUAD_ROLES` trong `templates.ts`, và `sdlcRoleAgents` (`initiative_build.go:42`). Tên workflow `BMAD Planning Auto` và lead `Blueprint Lead` có ở `server/internal/service/squad_planning_workflow.go:22-23`, trích từ `templates.ts`. Ràng buộc chỉ bằng comment và test. Chưa kiểm chứng ba bản này đang khớp nhau; bước T4.3a trong plan đo điều đó.
- C6. Hướng sinh ngược: TS là nguồn, JSON cho Go được sinh bằng esbuild từ `company-template.ts` và `instruction-templates.ts`. Go còn hằng số tên skill viết tay (`server/internal/service/company_starter_skills.go:24`, `SecretarySkills` ở `:47`) và khóa squad (`company_template.go:314`, `:813`).
- C7. Hai nơi đăng ký pack. `KITVIET_PACK_LOADERS` được sinh tự động (tốt). `AGENT_PACK_IDS` và bảng `bundles` trong `packages/core/agent-packs/load.ts:3-9` viết tay, và giao diện luôn lấy phần tử đầu (`agent-pack-install.tsx:32`).
- C8. Lịch sử văn bản chỉ tăng: `kitviet-previous-texts.ts` 2,6 MB, `upgrade.ts` so sánh toàn văn.
- C9. Ba cơ chế kiểm tra độ tươi khác nhau: Kit Việt có `--check` chạy trong vitest (`packages/core/squads/kitviet-skills.test.ts:573`); SDLC chỉ có test vitest (`sdlc-skills.test.ts`); công ty mẫu chỉ có test vitest (`company-template-data.test.ts`); reserved-slugs và text-preview có bước `git diff --exit-code` trong CI (`.github/workflows/ci.yml`). CI chạy riêng `scripts/generate-kitviet-pack.test.mjs` (`ci.yml:44`).
- C10. Test đếm cứng: `company-template.test.ts:34` (14 squad), `templates.test.ts:91` (8 skill planning), `templates.test.ts:265` (7 stage), `kitviet-packs.test.ts:37` (2 pack). Mỗi squad Kit Việt còn có một test ~130 dòng gần giống nhau (`packages/core/squads/kitviet-templates/admin.test.ts`).
- C11. Test gắn đường dẫn tương đối: `sdlc-skills.test.ts` (`new URL("./sdlc-skills", ...)`), `sdlc-harness-check.test.ts:9` và `sdlc-harness-metrics.test.ts:9` chạy script nằm trong thư mục skill; `scripts/generate-kitviet-pack.test.mjs`; ba test Go đọc `../../../examples/plugins`.
- C12. Hạ tầng giả định nội dung nằm trong `packages/` hoặc `server/`: `.github/ci-paths.json` (bộ lọc `frontend` là `packages/**`, `backend` là `server/**`), `Dockerfile:13` chỉ `COPY server/ ./server/`, `Dockerfile.web:46` chỉ `COPY packages/ packages/`, và `turbo.json` băm đầu vào theo từng package (một thay đổi ở thư mục gốc không làm mất cache của `@multica/core#test` nếu không khai báo `inputs`).
- C13. Skill vendor (bmad, superpowers, supabase, accessibility...) không có file nguồn gốc hoặc giấy phép riêng. Tìm theo tên trong `packages/core/squads/sdlc-skills/` chỉ thấy `frontend-design--frontend-design/LICENSE.txt`. Kit Việt thì có `kitviet-upstream/LICENSE`, `NOTICE.md`, `UPSTREAM.json`.
- C14. Id workflow template lặp ở 5 nơi: hằng Go (`server/internal/issueworkflow/templates.go`), union TS (`packages/core/types/issue-workflow.ts:227`), tập lọc id (`packages/core/api/schemas.ts`, id lạ bị bỏ qua), danh sách và hàm dịch trong `template-picker-dialog.tsx`, khóa `templates` ở 7 `workflow.json`. Comment trong `templates.go` nêu rõ "a new id needs a matching frontend entry". Hệ quả: một workflow template mới ở máy chủ không hiện trên client chưa cập nhật, vì id lạ bị lọc bỏ. Các file phía trình thiết kế đang là công việc chưa commit của người dùng và có thể đổi.

### 4.5 Quy trình thêm một squad Kit Việt hiện nay

Theo `docs/engineering/kitviet-runbook.md` (mục 1, 8e) và mã nguồn:

1. Tạo `packages/core/squads/kitviet-squads/<id>.json`.
2. Tạo `packages/core/squads/kitviet-templates/<id>.ts` (role key, charter, routine, mô tả) và test đi kèm.
3. Sửa `packages/core/squads/templates.ts` ở 4 chỗ: import, `SquadTemplateId`, `SquadRoleKey`, danh sách trong `inCategory`.
4. Thêm khóa dịch vào 7 file `modals.json`.
5. Chạy `pnpm generate:kitviet`.
6. Nếu squad nằm trên tầng riêng: sửa `kitviet-floors.ts`.
7. Khi đổi văn bản sau đó: thêm bản cũ vào `kitviet-previous-texts.ts`.

### 4.6 Điều kiện tiên quyết về cây làm việc

Tại thời điểm khảo sát, `git status` có khoảng 100 mục chưa commit. Các thay đổi chạm trực tiếp vùng này: `packages/core/squads/sdlc-skills/**` (SKILL.md, `check-harness.mjs`, `harness-metrics.mjs`), `sdlc-skills-data.ts`, `sdlc-harness-*.test.ts`, `packages/core/issue-workflows/**`, `packages/views/workflow/**`, `packages/views/locales/*/workflow.json`, `server/internal/issueworkflow/**` (có `templates.go` chưa theo dõi). Bản kiểm tra chỉ đọc `node scripts/generate-kitviet-pack.mjs --check` báo "up to date (29 files)" trên cây hiện tại. Kế hoạch bắt đầu di chuyển phải chờ các thay đổi này được commit (T0.2).

## 5. Các phương án

### 5.1 Mô tả

- A. Quy ước thuần hệ thống file. Template là thư mục, tự phát hiện bằng glob lúc build hoặc lúc chạy, không có file chỉ mục sinh ra.
- B. Thư mục + manifest + sinh mã. Phát hiện bằng script Node, sinh chỉ mục TypeScript (kiểu, danh sách, bộ nạp lazy), JSON cho Go và khóa dịch, tất cả được commit và có kiểm tra lệch trong CI.
- C. Kho lúc chạy. Template nằm trong DB hoặc kho từ xa (Hub hiện có: `hub_source`, `hub_item`, migration 927-970; xem `docs/engineering/skill-plugin-hub.md`), có lệnh seed hoặc import.

### 5.2 So sánh

| Tiêu chí | A. Glob thuần | B. Manifest + sinh mã (đề xuất) | C. Kho lúc chạy |
| --- | --- | --- | --- |
| Công thêm template mới | Thấp nếu bundler hỗ trợ glob; nhưng id và kiểu TypeScript không tự có | Thấp: tạo thư mục, một lệnh build. Có scaffold | Thấp cho người dùng cuối, cao cho bản dựng sẵn: cần API, giao diện admin, lệnh seed |
| Rủi ro làm hỏng hành vi hiện có | Trung bình: mất kiểu id, mất lỗi biên dịch khi thiếu bản dịch | Thấp: có thể chứng minh bằng băm nội dung từng bước | Cao: đổi nơi lưu 5 MB+, đổi luồng quickstart sang bất đồng bộ, thêm schema DB |
| Phiên bản | Không có chỗ khai báo | `version` theo template + `registry.lock.json` | Tốt: phiên bản theo bản ghi, nhưng cần quy tắc nâng cấp theo workspace |
| Khả năng kiểm thử | Test phải tự duyệt thư mục | Kiểm tra tĩnh toàn bộ trước khi chạy (mục 7), test tạo theo registry | Phải có DB; test chậm hơn |
| Cách Go và TS cùng dùng | `import.meta.glob` chỉ có ở Vite; Next (webpack/turbopack) và Metro không có. `go:embed` không vượt ra ngoài thư mục `server/` | Một nguồn, hai đầu ra sinh ra: TS vào `packages/`, JSON vào `server/` | Go là nguồn, TS gọi API; client cũ gặp backend mới nên cần zod + fallback theo AGENTS.md |
| Offline, desktop, mobile | Như hiện nay | Như hiện nay: dữ liệu đóng gói sẵn trong bundle; mobile không dùng | Desktop cần mạng tới server cho danh sách; thêm trạng thái tải và lỗi |
| Chi phí bảo trì | Thấp lúc đầu, tăng khi cần thêm trường | Trung bình: có CLI + schema để giữ; giảm các test lặp (21 file test squad) | Cao: bảo mật chuỗi cung ứng prompt, nhập từ xa, di trú, giao diện |
| Phù hợp Docker/turbo/CI hiện tại | Không (nguồn ngoài `packages/`, không có chỉ mục) | Có, vì đầu ra sinh được commit trong `packages/` và `server/` | Có, nhưng nội dung đi qua DB |

### 5.3 Quyết định

Chọn B. Lý do:

- Docker chỉ sao chép `server/` (cho Go) và `packages/` (cho web). Nguồn ở thư mục gốc không có mặt lúc build image, nên đầu ra sinh ra phải được commit trong `packages/` và `server/`. Đây cũng là quy ước hiện có (`company_template_data.json`, `reserved-slugs.ts`, `sdlc-skills-data.ts`). B mở rộng quy ước đó thay vì thêm bước build mới.
- B cho phép chứng minh "không đổi hành vi" từng bước bằng băm đầu ra, điều A và C không làm được rẻ.
- B giữ kiểu id cho code TypeScript có đặt tên template (test, ánh xạ), vì union được sinh ra.

Bị loại:

- A: Next.js và Metro không có glob như Vite, và Go không nhúng được ngoài `server/`. Cuối cùng vẫn cần bước sinh, nên A thu hẹp thành B.
- C làm phương án duy nhất: đổi cả nơi lưu, luồng và mô hình bảo mật cho nội dung mà bản phát hành phải cập nhật cùng engine. Rủi ro lớn, lợi ích chính (template của bên thứ ba) chưa được yêu cầu.
- Sinh lúc build thay vì commit: cần thêm `COPY templates/` và cài Node vào image Go, và ba điểm build khác nhau (Go, Next, Electron).
- Manifest YAML: thêm phụ thuộc mới; repo đã dùng JSON cho cấu hình (`kitviet-squads/*.json`, có tiền lệ `$comment` ở `kitviet-extensions/it.json`).
- Workspace `@multica/templates` riêng: thêm workspace phải khai báo trong pnpm, knip, `ci-paths.json` và bản đồ exports mà không đem lại lợi ích. Xem lại nếu mobile cần dùng trực tiếp.
- Repo template tách riêng hoặc submodule: mất tính nguyên tử giữa engine và nội dung. Chỉ có thể cân nhắc cho `kitviet-upstream` (2,8 MB vendor) sau này.

Đường nối sang C (không làm bây giờ): giữ định dạng thư mục skill nguyên bản (`SKILL.md` + file kèm), để một bản xuất `pnpm templates:export <kind> <id>` thành zip nhập được vào Hub bằng luồng zip hiện có. Khi cần, đăng ký `templates/` như một nguồn Hub kiểu `builtin`.

## 6. Thiết kế đã chọn

### 6.1 Bố cục thư mục

```
templates/                                (mới)
├── README.md                             hướng dẫn 1 trang: thêm / sửa / xóa / đổi tên
├── registry.config.json                  danh sách locale, loại template, mục tiêu xuất (ts, go)
├── registry.lock.json                    SINH RA: id → version + sha256; danh sách id đã gỡ (tombstone)
├── schema/                               SINH RA từ zod: *.schema.json cho trình soạn thảo
├── collections/<id>/                     nhóm hiển thị trong bộ chọn (thay cho SquadTemplateCategory)
│   ├── template.json
│   └── i18n/en.json …
├── skillsets/<id>/                       tập skill đặt tên (sdlc, planning, sales, ...)
├── skills/<nhóm>/<skill-id>/             nhóm chỉ để sắp xếp (builtin, starter, ...)
│   ├── SKILL.md                          nội dung gốc, không sửa vì manifest
│   ├── template.json                     tập thuộc về, nguồn gốc, phiên bản
│   └── references/ scripts/ ...          file kèm, giữ nguyên
├── agents/<id>/                          mẫu hướng dẫn agent: template.json, instructions.md, i18n/
├── squads/<id>/                          template.json, instructions.md, roles/*.md (nếu có), i18n/
├── workflows/<id>/                       template.json, i18n/
├── companies/<id>/                       công ty mẫu: template.json (+ i18n/)
├── packs/<id>/                           agent pack: template.json, instructions.md, skills/<tên>/
├── bundles/quickstart/template.json      danh sách cài Claude Code (marketplace, plugin, skill)
└── _sources/                             đầu vào của bộ biến đổi, không phải template
    └── kitviet/{upstream,patches,squads,extensions,skills,rewrites.json}
```

Quy tắc khám phá:

- Thư mục template = thư mục có `template.json`. Với `skills/`, một thư mục có `SKILL.md` là skill; quét dừng ở `SKILL.md` đầu tiên (một `references/` chứa `SKILL.md` mẫu không bị coi là skill).
- Tên thư mục = `id`, riêng skill đổi `:` thành `--` (giữ quy ước hiện tại vì dấu `:` không hợp lệ trong đường dẫn Windows). Ví dụ `superpowers--brainstorming` có id `superpowers:brainstorming`.
- `template.json` và `i18n/` không bao giờ nằm trong danh sách file cài đặt của skill. Hiện `skillFiles()` trong `scripts/lib/skill-bundle.mjs` lấy mọi file trừ `SKILL.md`; builder mới phải loại hai thứ này, nếu không nội dung gói đổi và băm lệch.
- Nhóm thư mục `skills/<nhóm>/` do người bảo trì tự đặt. Nhóm không có ý nghĩa ngữ nghĩa; tập skill do manifest quyết định.

Tên `templates/`: trùng chữ với `deploy/helm/multica/templates/` và `.github/ISSUE_TEMPLATE/`. README ở gốc `templates/` nói rõ. Phương án thay thế: `catalog/`. Xem câu hỏi mở Q1.

### 6.2 Luồng dữ liệu

```
templates/ (nguồn, người viết sửa)
   │   pnpm templates:check   → lớp L1–L5 (mục 7)
   │   pnpm templates:build
   ▼
┌───────────────────────────────┬─────────────────────────────────────────┬─────────────────────────────────┐
│ packages/core/templates/      │ packages/views/locales/<lang>/          │ server/internal/templatereg/    │
│   generated/ (mới)            │   templates.json (mới, sinh)            │   data/*.json (mới, sinh)       │
│  - index.ts: kiểu id, meta    │  - tên, mô tả thẻ theo locale           │  - company, workflows,          │
│  - chunks/: nội dung lazy     │                                         │    starter-skills, quickstart   │
└───────────────┬───────────────┴─────────────────────┬───────────────────┴───────────────┬─────────────────┘
                ▼                                     ▼                                   ▼
   @multica/core (quickstart, upgrade)     @multica/views (useT("templates"))     Go: go:embed + loader có kiểu
```

Mọi đầu ra sinh ra đều được commit. `.gitattributes` đánh dấu `linguist-generated=true` để GitHub gập diff.

### 6.3 Manifest: phong bì chung

Mọi `template.json` có cùng phong bì; `spec` hoặc khối riêng theo loại nằm ở mục 6.4.

| Trường | Kiểu | Bắt buộc | Quy tắc |
| --- | --- | --- | --- |
| `$schema` | chuỗi | không | Trỏ tới `templates/schema/<kind>.schema.json` để trình soạn thảo gợi ý |
| `schemaVersion` | số nguyên | có | Phiên bản định dạng của loại này. Hiện là `1`. Máy chủ và CLI từ chối giá trị lạ |
| `kind` | chuỗi | có | `skill`, `skillset`, `collection`, `agent`, `squad`, `workflow`, `company`, `pack`, `bundle` |
| `id` | chuỗi | có | Bất biến sau khi phát hành. Quy tắc bên dưới |
| `version` | semver | có | Phiên bản nội dung. Patch: sửa văn bản. Minor: thêm vai, skill, bước. Major: bỏ hoặc đổi tên vai, làm squad đã tạo không ánh xạ được |
| `status` | `stable` \| `beta` \| `deprecated` \| `hidden` | không, mặc định `stable` | `deprecated` và `hidden` không hiện trong bộ chọn nhưng vẫn nạp được (squad đã tạo còn tham chiếu `template_key`) |
| `order` | số | không | Thứ tự trong nhóm; mặc định theo `id` |
| `tags` | mảng chuỗi | không | Lọc và tìm kiếm |
| `requires` | mảng `{kind, id}` | không | Phụ thuộc (mục 6.3.3) |
| `contracts` | mảng chuỗi | không | Hợp đồng mà code phụ thuộc (mục 6.3.4) |
| `aliases` | mảng chuỗi | không | Id cũ của template này |
| `source` | `{type, repo?, ref?, license?, notice?}` | có với `vendored` | `type` là `authored` hoặc `vendored` |
| `i18n` | không phải trường; thư mục `i18n/<locale>.json` cạnh manifest | xem 6.3.5 | |

#### 6.3.1 Quy tắc id

- skill: `^[a-z0-9][a-z0-9._-]*(:[a-z0-9][a-z0-9._-]*)?$`, tối đa 64 ký tự. Cùng quy tắc với `SKILL_NAME` trong `packages/core/agent-packs/schema.ts`, thêm tiền tố plugin tùy chọn (`bmad-method:bmad-prd`).
- pack: `^[a-z][a-z0-9-]{1,47}$` (kebab, theo `market-strategist`).
- các loại còn lại: `^[a-z][a-z0-9_]{1,47}$` (snake, theo `sdlc_greenfield`, `kv_admin`, `executive_office`).
- Tiền tố `_` dành cho thư mục nội bộ (`_sources`).
- `id` duy nhất trong cùng loại, và phải bằng tên thư mục (có quy tắc `--`).
- Id được lưu vào dữ liệu người dùng (`squad.template_key`, tên skill trong workspace, tên agent), nên không được đổi. Đổi tên = tạo id mới, id cũ vào `aliases` của template mới và thẻ cũ thành `deprecated`.
- Gỡ hẳn một id: thêm vào mảng `removed` trong `registry.config.json`. Id đã gỡ không được tái sử dụng. Lệnh `check` so với bản trên nhánh chính và báo lỗi nếu một id trong `registry.lock.json` biến mất mà không nằm trong `removed`.

#### 6.3.2 Phiên bản

- `version` là của từng template, độc lập với phiên bản sản phẩm.
- `registry.lock.json` (sinh ra) ghi `{ "squad:kv_admin": { "version": "1.0.0", "sha256": "…" } }`. `sha256` băm manifest chuẩn hóa + mọi file nội dung (trừ `i18n/`).
- Quy tắc: nội dung đổi (sha256 khác bản trên nhánh chính) mà `version` không đổi thì `check` cảnh báo ở giai đoạn đầu (không chặn), và có thể nâng thành lỗi sau (câu hỏi mở Q5).
- `schemaVersion` đổi khi định dạng manifest đổi không tương thích. CLI và loader Go từ chối `schemaVersion` mà chúng không biết, thay vì đọc sai.

#### 6.3.3 Phụ thuộc

- `requires: [{ "kind": "skillset", "id": "sdlc" }, { "kind": "workflow", "id": "sdlc_auto" }]`.
- Chỉ id, chưa có khoảng phiên bản: toàn bộ registry được dựng từ một snapshot git, nên phiên bản luôn khớp. Khoảng phiên bản chỉ cần khi có nguồn ngoài (phương án C), nên chưa làm.
- Builder tính bao đóng bắc cầu, báo lỗi khi thiếu hoặc có vòng, và cảnh báo khi một template `stable` cần một template `beta` hoặc `deprecated`.
- Quan hệ tự suy ra (không cần khai báo lại): squad cần skill nó liệt kê, và workflow nó tham chiếu.

#### 6.3.4 Hợp đồng

- `contracts: ["squad.sdlc"]` khai báo rằng template này đáp ứng một hợp đồng mà code phụ thuộc, ví dụ `initiative_build.go` cần squad có vai `developer`, `reviewer`, `qa`.
- Danh sách hợp đồng code cần được ghi ở một nơi: `server/internal/templatereg/contracts.go` (mới) và `packages/core/templates/contracts.ts` (mới). Test kiểm tra mỗi hợp đồng có đúng một template `stable` đáp ứng. Mục đích: loại bỏ trường hợp xóa nhầm một template mà Go đang dựa vào.

#### 6.3.5 Dịch

- Chỉ chuỗi hiển thị cho người dùng (tên và mô tả của thẻ) được dịch. Nội dung dành cho agent (SKILL.md, instructions, charter, hướng dẫn bước workflow) giữ một ngôn ngữ như đang có (tiếng Anh; Kit Việt viết tiếng Việt theo thiết kế).
- `i18n/<locale>.json` là đối tượng phẳng, ví dụ `{ "title": "Office Admin (Hành chính)", "description": "…" }`. `en` bắt buộc. Locale hợp lệ lấy từ `registry.config.json`; test kiểm tra danh sách này bằng đúng các thư mục dưới `packages/views/locales/`.
- Locale còn thiếu: builder điền bằng `en`, ghi vào báo cáo, và `check` cảnh báo. Chính sách chặn hay không do câu hỏi mở Q3 quyết định.
- Builder viết `packages/views/locales/<lang>/templates.json` (namespace mới `templates`). Namespace phải được đăng ký một lần trong `packages/views/locales/index.ts`; `parity.test.ts` tự bao phủ nhờ nó duyệt thư mục.
- Tên entity được tạo trong workspace (tên squad, tên agent) không dịch, vì quickstart chạy lại tìm theo tên (`templates.ts`: "A non-archived agent with this name is reused"). Tên đó nằm trong `spec`, không nằm trong `i18n`.

### 6.4 Khối riêng theo loại

Các trường dưới đây chuyển dữ liệu hiện có sang JSON, đúng tên với kiểu hiện tại để so sánh được (`SquadTemplate` ở `templates.ts:599`).

skill (`template.json` cạnh `SKILL.md`):

```json
{
  "schemaVersion": 1, "kind": "skill", "id": "sdlc-harness", "version": "2.0.0",
  "sets": ["sdlc"],
  "source": { "type": "authored" }
}
```

- `name` và `description` lấy từ phần đầu `SKILL.md` (dùng lại `frontmatterDescription` của `scripts/lib/skill-bundle.mjs`); manifest không lặp lại. Quy tắc L3: `name` trong frontmatter phải bằng `id` hoặc bằng phần sau dấu `:` của `id` (ví dụ `superpowers--brainstorming/SKILL.md` có `name: brainstorming`). Đã kiểm tra trên 87 skill của T1 và T2: tất cả khớp. Ba skill viết tay `kv-*` trong `kitviet-skills/` có `name: kv--<tên>` (tên trong gói), nên chúng nằm ở `_sources/kitviet/skills/` do bộ biến đổi Kit Việt xử lý và được miễn quy tắc này.
- `sets`: các skillset skill này thuộc về. Một skill có thể thuộc nhiều tập (hiện `bmad-method:bmad-prd` thuộc cả SDLC lẫn planning). Khai báo ở phía skill để thêm skill mới chỉ cần một thư mục.
- Skill vendor: `source.type = "vendored"` bắt buộc có `repo`, `ref`, `license`.

skillset:

```json
{ "schemaVersion": 1, "kind": "skillset", "id": "planning", "version": "1.0.0", "order": 20 }
```

Không liệt kê skill; thành viên suy ra từ `sets` của skill. Lý do: thêm skill vào tập chỉ sửa thư mục skill. Báo cáo cảnh báo skill mồ côi (không thuộc tập nào) và tập rỗng.

collection:

```json
{ "schemaVersion": 1, "kind": "collection", "id": "kitviet", "version": "1.0.0", "order": 20, "for": "squad" }
```

Thay `SquadTemplateCategory` (`templates.ts:589`). Squad khai báo `collection`. Tiêu đề nhóm lấy từ `i18n/`. Một "bộ template" mới = một thư mục collection + các squad của nó.

squad:

```json
{
  "schemaVersion": 1, "kind": "squad", "id": "kv_admin", "version": "1.0.0",
  "collection": "kitviet", "order": 10, "expertReview": false,
  "requires": [{ "kind": "skillset", "id": "kv-common" }],
  "squad": {
    "name": "Office Admin (Hành chính)",
    "floorName": "Executive",
    "leader": "admin_lead",
    "operatingReview": true,
    "instructionsFile": "instructions.md",
    "workflow": null,
    "roles": [
      { "key": "admin_lead", "agent": "kv_lead", "name": "Admin Lead",
        "description": "…", "roster": "Squad leader" }
    ],
    "routines": [],
    "charter": { "mission": "…", "responsibilities": [], "outputs": [],
                 "boundaries": "…", "escalation": "…", "intake": "…" },
    "skills": { "sets": ["kv-common"], "extra": [], "leader": [], "byRole": {} }
  },
  "adapter": { "type": "kitviet", "trade": "hanh-chinh-van-phong",
               "agents": { "lap-ke-hoach-dieu-phoi": { "role": "admin_lead", "kind": "kv_lead" } } }
}
```

- `squad.workflow` là `null` hoặc `{ "ref": "<workflow id>" }`.
- `adapter` chỉ có ở squad dựng từ nguồn vendor. Bộ biến đổi Kit Việt giữ nguyên (1.061 dòng) và đọc cấu hình từ khối này thay vì `kitviet-squads/<id>.json`. Xem plan giai đoạn 3.
- Tên `role.key` là duy nhất toàn registry, vì hiện `SquadRoleKey` là một union phẳng (`templates.ts:91`). Builder kiểm tra.

workflow:

```json
{
  "schemaVersion": 1, "kind": "workflow", "id": "sdlc_auto", "version": "1.0.0",
  "workflow": {
    "name": "SDLC Auto", "description": "…",
    "steps": [
      { "status": { "key": "todo" }, "role": "developer", "optionalRole": false,
        "instructions": "…",
        "transitions": [{ "id": "default", "to": "in_progress" }] }
    ]
  }
}
```

- `role` là vai của squad, không phải agent. TS ánh xạ vai sang agent id (`workflowSteps` ở `quickstart.ts:229`), Go ánh xạ trong `sdlcSteps`, trình thiết kế dùng làm gợi ý (`RoleHint` trong `server/internal/issueworkflow/templates.go`). `optionalRole: true` mô tả các bước chuyên gia chỉ có khi squad có vai đó (`sdlcStageSteps`).
- Hình dạng này là tập hợp của ba bản hiện có. Bước hợp nhất (plan T4.3) bắt đầu bằng test so sánh ba bản để biết chúng có đang lệch không.

company:

```json
{ "schemaVersion": 1, "kind": "company", "id": "starter", "version": "1.0.0",
  "company": { "floors": [], "squads": [], "stageOwners": [], "startOptions": ["all","per_floor","leave_paused"] } }
```

Dữ liệu y hệt `COMPANY_FLOOR_KEYS`, `COMPANY_SQUAD_CARDS`, `COMPANY_STAGE_OWNERS`, `COMPANY_START_OPTIONS` hiện có trong `company-template.ts`. Mẫu hướng dẫn agent (`AGENT_INSTRUCTION_TEMPLATES`, 39 mục) thành loại `agent`.

pack: giữ `pack.json` hiện tại (`agent`, `skills`) đổi tên thành `template.json` kèm phong bì; `skills/<tên>/` nằm trong thư mục pack và không tham gia tập skill toàn cục (tên có thể trùng skill toàn cục).

bundle: nội dung của `server/internal/quickstart/bundle.json` (marketplace, plugin, skill), một thể hiện duy nhất `quickstart`.

### 6.5 Ví dụ "thêm một bộ template mới"

Thêm bộ "Customer Success" gồm một collection, một skillset, hai skill, một squad:

```
templates/collections/customer_success/{template.json, i18n/en.json}
templates/skillsets/cs_core/{template.json, i18n/en.json}
templates/skills/customer-success/cs-triage/{SKILL.md, template.json}
templates/skills/customer-success/cs-escalation/{SKILL.md, template.json}
templates/squads/cs_desk/{template.json, instructions.md, i18n/en.json}
```

Lệnh: `pnpm templates:new squad cs_desk --collection customer_success` (tạo khung), sửa nội dung, `pnpm templates:build`, commit cả thư mục lẫn đầu ra sinh ra. Không sửa file nào trong `packages/`, `server/` bằng tay. Tiêu chí nghiệm thu của mục tiêu G2 là đúng quy trình này chạy xanh trên một template thử rồi hoàn tác (plan T3.7).

### 6.6 Đầu ra sinh ra

| Đầu ra | Nội dung | Ai dùng |
| --- | --- | --- |
| `packages/core/templates/generated/index.ts` (mới) | Kiểu `SquadTemplateId`, `SquadRoleKey`...; mảng meta nhỏ đồng bộ (id, collection, order, status, expertReview, số vai, cờ); bảng bộ nạp lazy theo id | `@multica/core`, `@multica/views` |
| `packages/core/templates/generated/chunks/*.ts` (mới) | Nội dung nặng (hướng dẫn, charter, SKILL.md và file kèm). Ban đầu giữ độ chi tiết hiện có: một gói SDLC và một pack Kit Việt mỗi nghề; tinh chỉnh ở plan T5.3 sau khi đo | Nạp qua `import()` |
| `packages/views/locales/<lang>/templates.json` (mới) | Tên, mô tả, tiêu đề collection theo locale | `useT("templates")` |
| `server/internal/templatereg/data/*.json` (mới) | Chỉ các loại có `targets: ["go"]` trong `registry.config.json`: `company`, `workflow`, skill của công ty mẫu, `bundle` | Go qua `go:embed` |
| `templates/registry.lock.json` (mới) | Id, phiên bản, băm, id đã gỡ | CLI, CI |

### 6.7 Quy tắc tiêu thụ

TypeScript (`packages/core/templates/`, mới):

- API đồng bộ chỉ trả meta nhỏ: `listTemplates(kind, filter?)`, `getTemplateMeta(kind, id)`. API bất đồng bộ trả nội dung: `loadTemplate(kind, id)`. Giao diện danh sách (`squad-quickstart.tsx`) chỉ cần meta; `runSquadQuickstart` nhận nội dung đã nạp.
- Không dùng `import.meta.glob`, `require.context` hay `fs` trong mã chạy trên trình duyệt. Chỉ `generated/index.ts` được import bởi mã bên ngoài thư mục `packages/core/templates/`. Thêm quy tắc ESLint `no-restricted-imports` cho `templates/generated/chunks/*` (plan T3.4).
- `packages/core` không thêm thư viện UI; mọi thứ là dữ liệu thuần, đúng ràng buộc trong `AGENTS.md`. Mobile có thể dùng meta về sau, hiện không dùng.
- Dữ liệu từ API đi qua zod + `parseWithFallback` như `AGENTS.md` yêu cầu. Trường mới ở API (ví dụ tên theo locale của workflow template) là tùy chọn có mặc định, id lạ có fallback.

Go (`server/internal/templatereg/`, mới):

- `//go:embed data/*.json`, kiểu Go cho từng loại, tải một lần, trả lỗi (không panic lúc khởi động). Kiểm tra `schemaVersion`.
- Không đọc `templates/` ở thư mục gốc: `Dockerfile:13` chỉ có `server/`. Không ghi DB lúc khởi động (không có seed).
- Logic áp dụng giữ nguyên ở `server/internal/service/company_template.go` và `initiative_build.go`; chỉ nguồn dữ liệu đổi.
- Thay `company_template_data.json` và các hằng số tên skill bằng dữ liệu đọc từ registry, có test hợp đồng (6.3.4).

### 6.8 Phân phối và hạ tầng

- Web, desktop: dữ liệu được đóng gói trong bundle như hiện nay; không thêm gọi mạng. Mobile không dùng.
- Docker: không đổi `Dockerfile` hay `Dockerfile.web`, vì đầu ra đã commit nằm trong `server/` và `packages/`.
- `.github/ci-paths.json`: thêm `templates/**` và `scripts/templates/**` vào các bộ lọc `frontend`, `backend`, `quality` (và `scripts` nếu cần). Nếu không, PR chỉ sửa `templates/` có thể không chạy job nào.
- turbo: nếu test trong `packages/core` còn đọc `templates/` trực tiếp (hiện có 2 test chạy script trong thư mục skill), thêm `packages/core/turbo.json` với `inputs` gồm `$TURBO_ROOT$/templates/**`, nếu không cache có thể phát lại kết quả cũ. Cách tránh triệt để: test của core chỉ đọc đầu ra sinh ra; việc kiểm tra nguồn thuộc `pnpm templates:check` chạy ngoài turbo. Plan T1.1 kiểm chứng bằng `turbo --dry=json`.
- knip (`knip.jsonc`): thêm `scripts/templates/*.mjs` vào `entry` của workspace gốc; đầu ra sinh ra có thể cần `ignore`.
- `packages/core/package.json`: thêm mục `exports` cho `./templates`.
- `AGENTS.md`: thêm một hàng cho `templates/` trong bảng ranh giới và một câu về lệnh sinh (plan giai đoạn 7).

### 6.9 Vận hành

| Việc | Cách làm | Sửa code? |
| --- | --- | --- |
| Sửa văn bản skill hay hướng dẫn | Sửa file, nâng `version`, `pnpm templates:build`, commit | Không |
| Thêm skill | `pnpm templates:new skill <id> --set sdlc`, sửa `SKILL.md`, build | Không |
| Thêm squad, workflow, collection, pack | Như mục 6.5 | Không |
| Ẩn khỏi bộ chọn | Đặt `status: "deprecated"` hoặc `hidden` | Không |
| Đổi tên | Id mới + `aliases`, thẻ cũ `deprecated` | Không |
| Gỡ hẳn | Thêm id vào `removed` trong `registry.config.json` sau ít nhất một bản phát hành `deprecated` | Không |
| Nâng upstream Kit Việt | Giữ quy trình ở `docs/engineering/kitviet-runbook.md` mục 2, đường dẫn đổi sang `templates/_sources/kitviet/` | Không |
| Thêm loại template mới | Thêm schema zod, builder, mục trong `registry.config.json` | Có (hiếm, có chủ đích) |
| Thêm collection mới | Thư mục `collections/<id>/` | Không |
| Thêm locale mới | Thêm vào `registry.config.json` và `packages/views/locales/` | Có (hiếm) |

Văn bản cũ để nâng cấp không phá chỉnh sửa của người dùng (T10): giai đoạn đầu giữ nguyên cơ chế "bản cũ toàn văn" nhưng chuyển vào thư mục template. Giai đoạn 6 (tùy chọn) thay bằng danh sách băm sha256, giảm 2,6 MB xuống kích thước nhỏ. Rủi ro đã biết: `crypto.subtle` không có trên trang HTTP không an toàn (self-host qua IP LAN), nên cần cài sha256 thuần JS; xem Q6.

## 7. Kiểm tra (validation)

Một CLI: `node scripts/templates/cli.mjs <lệnh>`, có alias `pnpm templates:check|build|new|list`. Test của CLI chạy bằng `node --test scripts/templates/*.test.mjs`, thêm cạnh dòng `ci.yml:44`.

| Lớp | Nội dung | Lỗi hay cảnh báo |
| --- | --- | --- |
| L1 Schema | Mỗi `template.json` qua zod theo `kind`; id đúng regex và bằng tên thư mục; `version` hợp lệ; `schemaVersion` biết; id duy nhất trong loại | Lỗi |
| L2 Tham chiếu | `requires`, `sets`, `collection`, `workflow.ref`, kỹ năng trong `skills`, `leader` thuộc `roles`, `role` của bước workflow thuộc squad dùng nó, `role.key` duy nhất toàn registry; không vòng | Lỗi |
| L3 Nội dung | `SKILL.md` có frontmatter `name` khớp và `description` không rỗng; file chỉ là văn bản (không nhị phân, theo `build-agent-pack-bundles.mjs`); trần kích thước theo loại (mang từ `kitviet-skills.test.ts`); không có chuỗi bí mật (mẫu `sk-`, `AKIA`, khóa riêng); skill vendor có `source.license`; quy tắc "chuỗi cấm" hiện có của Kit Việt | Lỗi (kích thước và chuỗi cấm), cảnh báo (mồ côi) |
| L4 Dịch | `en` có; các locale có cùng tập khóa; danh sách locale khớp thư mục `packages/views/locales/` | Lỗi (`en`), cảnh báo (locale khác; Q3) |
| L5 Độ tươi | Chạy lại builder trong bộ nhớ và so với file đã commit; thay cho ba cơ chế ở C9. CI chạy `pnpm templates:check`, thất bại nếu có chênh | Lỗi |
| L6 Tương đương | Trong giai đoạn di chuyển: băm từng mục đầu ra (squad, skill, workflow, công ty mẫu, pack, bundle) trước và sau, so với `templates/golden-hashes.json` (mới, tạm; xóa ở giai đoạn 7) | Lỗi |
| L7 Bất biến id và hợp đồng | `registry.lock.json` so với nhánh chính: id không biến mất nếu không có trong `removed`; mỗi hợp đồng ở Go và TS có đúng một template `stable` đáp ứng; nội dung đổi mà `version` không đổi | Lỗi (id, hợp đồng), cảnh báo (version) |

Test vitest sinh theo registry thay cho test liệt kê tên: một `squad-invariants.test.ts` (mới) duyệt mọi squad và áp các khẳng định hiện nằm lặp ở `kitviet-templates/*.test.ts` (đúng một lead là leader, tối đa 8 thành viên ngoài lead, mô tả một dòng, tầng hợp lệ, cron 5 trường, vai có tên duy nhất). Khẳng định riêng của từng squad (ví dụ tên chính xác) được thay bằng so sánh băm L6 trong lúc di chuyển, và sau đó bằng `registry.lock.json`.

Go: `server/internal/templatereg` có test `TestEmbeddedRegistryParses`, `TestRequiredContractsProvided`, và test hiện có của `company_template_test.go`, `issueworkflow/templates_test.go` chạy trên dữ liệu mới.

## 8. Bảo mật và giấy phép

- Hướng dẫn agent là chuỗi cung ứng prompt: mọi thay đổi `templates/**` được xem như thay đổi code khi duyệt PR. Đề xuất thêm `CODEOWNERS` cho `templates/` (hiện `.github/` không có).
- Skill vendor bắt buộc có `source.license`; khối `kitviet-upstream/` giữ `LICENSE`, `NOTICE.md`, `UPSTREAM.json`. Việc bổ sung nguồn gốc cho skill vendor hiện chưa có (C13) và là điều kiện của L3.
- Không có bí mật trong `templates/`; L3 có quét mẫu cơ bản. Agent pack không chứa nhị phân (đã là quy tắc của `build-agent-pack-bundles.mjs`).
- Go không đọc đường dẫn lấy từ manifest; mọi đường dẫn là tên file cố định lúc build.

## 9. Tương thích và rủi ro

- Không có migration DB. Id template, tên skill, tên agent, tên workflow không đổi, nên squad và agent đã tạo tiếp tục khớp khi chạy lại quickstart hoặc nâng cấp.
- API: khi thêm trường vào phản hồi (ví dụ danh sách workflow template), cập nhật zod schema, giá trị mặc định và test phản hồi sai định dạng, theo `AGENTS.md`. Desktop cũ gặp backend mới vẫn hiển thị nhờ fallback id lạ.
- Không thêm lớp tương thích nội bộ: mỗi giai đoạn chuyển hẳn và xóa đường cũ trong cùng PR (đúng quy tắc "no compatibility shims").

Rủi ro chính:

| # | Rủi ro | Giảm thiểu |
| --- | --- | --- |
| R1 | Lệch hành vi vì chữ đổi nhẹ (id, tên agent, văn bản dùng để nhận biết "chưa bị sửa") | L6: băm từng mục trước và sau mỗi giai đoạn; di chuyển bằng `git mv` và codemod, không gõ lại |
| R2 | Đụng thay đổi chưa commit của người dùng (SDLC harness, issue-workflows, designer) | T0.2: chờ commit; mỗi giai đoạn là PR riêng; giai đoạn 4 bắt đầu sau khi vùng workflow ổn định |
| R3 | Hạ tầng bỏ sót `templates/` (CI không chạy, cache turbo phát lại kết quả cũ, Docker thiếu dữ liệu) | Thêm `ci-paths.json`, `inputs` của turbo, đầu ra commit trong `packages/` và `server/`; T1.1 kiểm chứng từng thứ |
| R4 | Diff lớn và nhiễu (đầu ra sinh vài MB) | `linguist-generated`, chia PR, `registry.lock.json` để review theo băm |
| R5 | Mất kiểm tra kiểu khóa dịch | Union id và `templates.json` được sinh, nên `$.templates.items[id]` vẫn gõ kiểu |
| R6 | Làm quá tay: CLI và registry thành gánh nặng | Giao hàng theo mốc (mục 2 của plan); dừng được sau giai đoạn 1 hoặc 3 mà vẫn có giá trị |

## 10. Câu hỏi mở (cần người dùng quyết trước khi triển khai)

- Q1. Tên thư mục gốc: `templates/` (đề xuất, trùng chữ với Helm) hay `catalog/`?
- Q2. Đầu ra sinh ra được commit (đề xuất, lý do ở mục 5.3) hay sinh lúc build (cần sửa hai Dockerfile)?
- Q3. Dịch ngoài `en`: bắt buộc đủ 7 ngôn ngữ cho mọi template (nghiêm, giống `parity.test.ts`) hay chỉ `en` bắt buộc và còn lại rơi về `en` kèm cảnh báo (đề xuất)?
- Q4. Phạm vi giai đoạn 4: hợp nhất SDLC Auto ở TS và hai nơi trong Go ngay, hay chỉ thêm test so sánh trước? Bản nào là chuẩn nếu hiện đang lệch?
- Q5. Khi nội dung đổi mà `version` không đổi: cảnh báo (đề xuất ban đầu) hay lỗi?
- Q6. Có làm giai đoạn 6 (thay `*-previous-texts.ts` bằng danh sách băm)? Cần chấp nhận cài sha256 thuần JS vì `crypto.subtle` thiếu trên HTTP không an toàn.
- Q7. Có cần bước tiếp theo cho template của bên thứ ba qua Hub không? Nếu có, nên chốt định dạng xuất zip (`templates:export`) từ giờ.
- Q8. `kitviet-upstream/` (2,8 MB vendor) ở lại trong repo dưới `templates/_sources/kitviet/` (đề xuất) hay tách repo/submodule?
- Q9. Ai điền nguồn gốc và giấy phép cho skill vendor (bmad, superpowers, supabase, accessibility...) vì C13?
- Q10. `examples/plugins/` giữ nguyên (đề xuất, vì là fixture test Go) hay cũng đưa vào `templates/` làm loại `plugin`?

## 11. Chưa kiểm chứng trong lần khảo sát này

- Kích thước thật của 2,1 MB `sdlc-skills-data.ts` trong bundle trình duyệt (cần đo, T0.5).
- Ba bản SDLC Auto (TS, `sdlcSteps`, `sdlcAutoTemplate`) có đang khớp nhau không.
- Cú pháp và hành vi của `inputs: ["$TURBO_ROOT$/templates/**"]` với phiên bản turbo của repo (T1.1).
- Kết quả bộ test đầy đủ (`pnpm test`, `make test`) trên cây làm việc hiện tại: chỉ chạy `node scripts/generate-kitviet-pack.mjs --check` (xanh).
- Số lượng mục trong các `*-previous-texts.ts` (ảnh hưởng đến kích thước thực của giai đoạn 6).
