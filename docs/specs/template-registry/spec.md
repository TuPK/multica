# Catalog: đưa nội dung template ra thư mục `catalog/` và thêm template không cần sửa code

Trạng thái: đã triển khai phần lớn (mục 11 ghi rõ cái nào xong, cái nào chưa). Quyết định Q1, Q2, Q3 do người dùng chốt; Q4 do kỹ sư triển khai chọn, xem mục 5.4. Kế hoạch: [plan.md](plan.md).
Ngày rà soát lại: 2026-11-12, trên HEAD `b324e4241` (cây làm việc sạch lúc bắt đầu).

Ngôn ngữ: tiếng Việt. Định danh, đường dẫn và lệnh giữ nguyên tiếng Anh.
Quy ước: đường dẫn kèm chữ "(mới)" là đường dẫn đề xuất, chưa tồn tại. Mọi đường dẫn còn lại đã được kiểm tra tồn tại bằng `git ls-files` hoặc `ls`. Tên "catalog" chỉ thư mục `catalog/` ở gốc repo; `deploy/helm/multica/templates/` và `.github/ISSUE_TEMPLATE/` là thứ khác.

## 0. Quyết định đã chốt

| # | Quyết định | Ghi chú |
| --- | --- | --- |
| Q1 | Thư mục gốc tên `catalog/` | Thay cho `templates/` trong bản nháp đầu |
| Q2 | Mã sinh ra không commit; sinh lúc build | Hệ quả ở mục 6.7: `.gitignore`, turbo, Makefile, CI, `Dockerfile`, `Dockerfile.web`, `.vercelignore` |
| Q3 | Bản dịch đầy đủ cho mọi ngôn ngữ hỗ trợ, không fallback | `catalog check` và build thất bại khi thiếu bản dịch |
| Q4 | Hợp nhất dữ liệu SDLC Auto thành một manifest; giữ nguyên phép chiếu của từng nơi dùng; workflow của trình thiết kế là manifest riêng có test khớp | Lý do và số liệu ở mục 5.4 |
| Q5 | Nội dung đổi mà `version` không đổi: cảnh báo | Có thể nâng thành lỗi sau |
| Q6 | Chưa làm "băm thay lịch sử văn bản" | Giữ `*-previous-texts.ts` trong code; ghi là phần chưa làm |
| Q8 | `kitviet-upstream` ở lại repo, dưới `catalog/_sources/kitviet/` | |
| Q10 | `examples/plugins/` giữ nguyên chỗ | Là fixture của 3 test Go |

Q7 (template bên thứ ba qua Hub) và Q9 (nguồn gốc skill vendor) vẫn mở, xem mục 10.

## 1. Tóm tắt

Nội dung "template" của Multica (skill, squad, agent, workflow, công ty mẫu, agent pack) nằm rải rác ở `packages/core/squads/`, `packages/core/agents/`, `packages/views/locales/`, `server/internal/` và `examples/`. Thêm một squad hoặc skill phải sửa tay 4 đến 8 chỗ trong code, gồm hai kiểu union TypeScript, một mảng đăng ký và 7 file dịch.

Giải pháp (phương án B, mục 5): toàn bộ nội dung nguồn nằm dưới `catalog/`. Mỗi template là một thư mục có `template.json` và file nội dung. Một CLI không phụ thuộc thư viện ngoài, `scripts/catalog/cli.mjs` (mới), kiểm tra manifest rồi sinh TypeScript và JSON trong lúc build. Các file sinh ra bị `.gitignore`, không commit. Thêm template = tạo thư mục rồi build; không sửa file code.

## 2. Vấn đề

1. Nội dung hay đổi (SKILL.md, hướng dẫn agent, charter, workflow) nằm lẫn với logic lõi trong `packages/core/squads/`: tám thư mục con chứa nội dung có tổng 1.101 file được git theo dõi, cạnh các file `.ts` của engine.
2. Thêm một template phải chạm nhiều file code (mục 4.5).
3. Cùng một nội dung nằm ở nhiều nơi, ràng buộc bằng comment hoặc test: workflow "SDLC Auto" ở TypeScript và hai nơi trong Go; id workflow template ở 5 nơi.
4. Dữ liệu viết bằng TypeScript rồi sinh JSON cho Go bằng esbuild (`scripts/generate-company-template.mjs`).
5. Có nhiều cơ chế kiểm tra độ tươi của file sinh ra, và 5 MB mã sinh ra được commit vào repo (`sdlc-skills-data.ts` 2,1 MB, `kitviet-skills-data/` 2,9 MB, `company_template_data.json` 0,3 MB, bundle agent pack 0,5 MB).
6. Hạ tầng giả định nội dung nằm trong `packages/` hoặc `server/` (mục 4.4, C12).

## 3. Mục tiêu và không phải mục tiêu

Mục tiêu:

- G1. Một thư mục gốc `catalog/` chứa nội dung nguồn; ranh giới lõi/template kiểm tra được (mục 4.1).
- G2. Thêm skill, skillset, collection, squad, agent pack mới không cần sửa file code.
- G3. Một nguồn cho Go và TypeScript với các nội dung dùng chung (công ty mẫu, workflow, bundle quickstart).
- G4. Không đổi hành vi người dùng thấy. Mỗi bước chứng minh bằng băm nội dung trước và sau (lớp L6, mục 7).
- G5. Manifest có phiên bản, id bất biến, phụ thuộc khai báo được, bản dịch đủ mọi ngôn ngữ, kiểm tra bằng schema.
- G6. Không còn mã sinh ra trong git; build tự sinh.

Không phải mục tiêu: kho template lúc runtime (Hub, DB), template riêng theo workspace, giao diện soạn template, đổi nội dung prompt, đổi schema DB (không có migration), skill nền tảng của server (`server/internal/service/builtin_skills/`), agent Mika, `examples/plugins/`, mobile, dịch nội dung dành cho agent.

## 4. Hiện trạng (kết quả rà soát lại trên HEAD `b324e4241`)

### 4.1 Ranh giới "lõi" và "template"

Một mục là template khi trả lời "không" câu 1 và "có" câu 3, 4:

| # | Câu hỏi | Nếu trả lời |
| --- | --- | --- |
| 1 | Thêm thể hiện mới có cần thêm nhánh code (`switch`, `if` theo id, kiểu union) không? | Có: lõi, hoặc template bị ghim |
| 2 | Code Go hoặc TS có tham chiếu thể hiện này theo tên hoặc id không (hợp đồng)? | Có: template bị ghim, khai báo `contracts` |
| 3 | Vòng đời thay đổi có độc lập với bản phát hành engine không? | Có: template |
| 4 | Người không viết code có sửa được bằng văn bản hoặc JSON không? | Có: template |

Lõi: engine quickstart (`packages/core/squads/quickstart.ts`, `upgrade.ts`, `handoff.ts`), engine workflow (`server/internal/issueworkflow/`), skill nền tảng của server, agent Mika, schema zod, các generator, nội dung giao diện (nhãn, nút). Template bị ghim: squad SDLC (Go dựng workflow theo tên agent `SDLC Developer`...), workflow `BMAD Planning Auto` với lead `Blueprint Lead`, thẻ `executive_office`, các khóa chủ stage trong `server/internal/service/pipeline_install.go`.

### 4.2 Bảng kiểm kê (số liệu đo lại)

Kích thước là tổng byte của file được git theo dõi.

| ID | Đường dẫn | Loại | Quy mô | Cách nạp | Code đọc |
| --- | --- | --- | --- | --- | --- |
| T1 | `packages/core/squads/sdlc-skills/` | skill | 83 thư mục, 397 file, 2,04 MB (thêm 6 skill `research-lab-*` so với lần khảo sát đầu) | `scripts/generate-sdlc-skills.mjs` sinh `packages/core/squads/sdlc-skills-data.ts` (2,1 MB, đang commit), import tĩnh | `templates.ts`, `quickstart.ts`, type trong `kitviet-packs.ts` |
| T2 | `packages/core/squads/skills/` | skill (công ty mẫu) | 10 skill, 20 file, 57 KB | `scripts/generate-company-template.mjs` nhúng vào `server/internal/service/company_template_data.json` (337 KB, đang commit) | Go: `company_starter_skills.go`, `company_template_data.go` |
| T3 | `packages/core/squads/kitviet-upstream/` | nguồn vendor (MIT) | 523 file, 2,9 MB (nâng lên v1.5.0 ở commit `05f54b3e0`) | Đầu vào `scripts/generate-kitviet-pack.mjs`; cũng được ghi bởi `scripts/kitviet-update.mjs` (mới từ commit đó) | Generator, `kitviet-update.mjs` |
| T4 | `packages/core/squads/kitviet-patches/`, `kitviet-rewrites.json` | quy tắc biến đổi | 91 file 63 KB; 6 KB | Đầu vào generator | Generator, `kitviet-update.mjs` |
| T5 | `packages/core/squads/kitviet-squads/`, `kitviet-extensions/`, `kitviet-skills/` | cấu hình squad, mở rộng, skill viết tay | 22 JSON (21 squad + `base.json`) 37 KB; 3 JSON; 3 skill 27 KB | Đầu vào generator | Generator |
| T6 | `packages/core/squads/kitviet-skills-data/` | đầu ra sinh (đang commit) | 29 file, 2,85 MB | Lazy theo pack qua `KITVIET_PACK_LOADERS` | `quickstart.ts`, `kitviet-packs.ts`, `kitviet-squad-roles.ts` |
| T7 | `packages/core/squads/kitviet-templates/` | squad (viết tay) | 21 `.ts` + 21 test, 330 KB | Import tĩnh trong `templates.ts` | `templates.ts` |
| T8 | `packages/core/squads/templates.ts` (53,5 KB), `content-studio.ts`, `research-lab.ts` (hai squad mới) | squad + workflow + danh sách skill | 7 squad lõi: `sdlc_greenfield`, `sdlc_maintenance`, `market_research`, `bmad_planning`, `sales_loop`, `content_studio`, `research_lab`; `SDLC_AUTO_WORKFLOW`, `PLANNING_AUTO_WORKFLOW`, `PLANNING_SKILL_NAMES`, `SALES_SKILL_NAMES` | Import tĩnh | `packages/views/modals/squad-quickstart.tsx`, `company/components/company-setup-card.tsx`, `company-starter-wizard.tsx` |
| T9 | `sdlc-agent-instructions.ts`, `planning-agent-instructions.ts`, `sales-agent-instructions.ts`, `kitviet-agent-instructions.ts`, `kitviet-squad-roles.ts`, `kitviet-floors.ts` | prompt agent theo vai (và bộ ghép) | 6,7 KB, 5,7 KB, 3,8 KB, 66,6 KB, 3,9 KB, 1 KB | Import tĩnh | `quickstart.ts` (`instructionsFor`) |
| T10 | `kitviet-previous-texts.ts`, `sdlc-previous-texts.ts` | lịch sử văn bản (chỉ thêm) | 2,6 MB; 35 KB | Import tĩnh | `upgrade.ts` |
| T11 | `company-template.ts` (45 KB), `packages/core/agents/instruction-templates.ts` (76 KB), `instruction-blocks.ts` | công ty mẫu (14 thẻ squad) và 39 mẫu hướng dẫn agent | | TS là nguồn, sinh `company_template_data.json` cho Go | Go: `company_template.go`, `company_setup.go`; TS: wizard, `instruction-template-picker.tsx` |
| T12 | `sales-starter-sources.ts`, `sales-recipe.ts`, `sales-intake.ts` | danh mục nguồn sales | 30 KB | Import tĩnh | `quickstart.ts` |
| T13 | Workflow SDLC Auto: `templates.ts` (`SDLC_AUTO_WORKFLOW`), `server/internal/service/initiative_build.go` (`sdlcSteps`), `server/internal/issueworkflow/templates.go` (7 template của trình thiết kế, nay đã commit); id lặp ở `packages/core/types/issue-workflow.ts`, `packages/core/api/schemas.ts`, `packages/views/workflow/designer/template-picker-dialog.tsx`, `workflow.json` ×7 | workflow | 3 nơi chứa SDLC Auto; 5 nơi chứa id | TS và Go riêng | Quickstart, `InitiativeService`, trình thiết kế |
| T14 | `examples/agent-packs/market-strategist/` | agent pack | 543 KB | `scripts/build-agent-pack-bundles.mjs` sinh `packages/core/agent-packs/bundles/market-strategist.json` (553 KB, đang commit); `AGENT_PACK_IDS` viết tay ở `packages/core/agent-packs/load.ts` | `packages/views/modals/agent-pack-install.tsx`, `Makefile` (`agent-pack`, `agent-pack-bundles`) |
| T15 | `examples/plugins/` | plugin mẫu / fixture | 5 plugin, 80 KB | Không nạp lúc chạy | 3 test Go (`../../../examples/plugins`) |
| T16 | `server/internal/service/builtin_skills/` (+ `_legacy`), `builtin_agents/mika/`, `server/internal/quickstart/bundle.json` | skill nền tảng, agent hệ thống, danh sách cài Claude Code | | `go:embed` | `builtin_skills.go`, `builtin_agents.go`, `quickstart/bundle.go` |
| T17 | `packages/views/locales/*/modals.json` (`squad_quickstart.templates`: 28 mục, `groups`: 3, mục agent pack), `agents.json` (34 mục `instruction_templates.items`), `workflow.json` | chuỗi hiển thị theo id | 7 ngôn ngữ | i18next, khóa gõ kiểu | `packages/views/locales/parity.test.ts` |

Ngoài kiểm kê: `.agents/skills/`, `skills-lock.json`, fixture test, `qoder_catalog.go`, `pixtuoid/`.

### 4.3 Phân loại

Template (chuyển vào `catalog/`): T1 đến T5, T7, T8 (phần dữ liệu), T11 (dữ liệu), T12 (để nguyên ở lần này, xem plan), T13, T14, T16 `bundle.json`, T17 (khóa theo id). Lõi (giữ trong code): T6 và các đầu ra sinh khác (là kết quả, không phải nguồn), T9 (bộ ghép, logic), T10, T15, T16 (skill nền tảng, Mika), engine, schema.

### 4.4 Các điểm ghép nối cứng (đã kiểm tra lại)

- C1. `SquadTemplateId` là union 28 phần tử (`templates.ts:33`), `SquadRoleKey` là union ghép 23 kiểu role (thêm `CsRoleKey`, `RlRoleKey`), 23 dòng import tĩnh (21 Kit Việt + `content-studio` + `research-lab`), danh sách đăng ký viết tay (`SQUAD_TEMPLATES`, `inCategory`).
- C2. 28 khóa × 7 ngôn ngữ trong `modals.json`; khóa gõ kiểu buộc đủ, nên thêm id mà thiếu bản dịch thì typecheck đỏ (đúng), nhưng phải sửa 7 file giao diện.
- C3. Nhóm skill theo tiền tố tên (`NON_SDLC_SKILLS` lọc `planning-`, `sales-`, `research-lab-`, `bmad-toolbox:`, `frontend-design:frontend-design`) và hai danh sách viết tay (`PLANNING_SKILL_NAMES`, `SALES_SKILL_NAMES`); `RESEARCH_LAB_SKILL_NAMES` nằm trong `research-lab.ts`. Mỗi nhóm skill mới lại thêm một tiền tố vào bộ lọc.
- C4. Gói 2,1 MB nạp sớm: `templates.ts` import tĩnh `SDLC_BUILTIN_SKILLS`, và barrel `packages/core/squads/index.ts` xuất lại nó. Chưa đo ảnh hưởng đến bundle trình duyệt (xem plan T0.5).
- C5. Lặp nội dung giữa TS và Go: đã đo ở mục 5.4.
- C6. Hướng sinh ngược: TS là nguồn, JSON cho Go được sinh bằng esbuild; Go còn hằng số tên skill và khóa squad viết tay (`company_starter_skills.go`, `company_template.go`).
- C7. `AGENT_PACK_IDS` và bảng `bundles` viết tay (`packages/core/agent-packs/load.ts`), giao diện lấy phần tử đầu (`agent-pack-install.tsx`).
- C8. Lịch sử văn bản chỉ tăng (2,6 MB).
- C9. Nhiều cơ chế kiểm tra độ tươi: `--check` của Kit Việt chạy trong vitest, test vitest riêng cho SDLC và công ty mẫu, `git diff --exit-code` trong CI cho reserved-slugs. Với Q2 (không commit mã sinh ra) khái niệm "lệch" biến mất: build luôn sinh lại.
- C10. Test đếm cứng (`company-template.test.ts` 14 squad, `templates.test.ts` 8 skill planning, `kitviet-packs.test.ts` 2 pack) và 21 test squad gần giống nhau (`kitviet-templates/*.test.ts`).
- C11. Test gắn đường dẫn tương đối tới thư mục skill (`sdlc-skills.test.ts`, `sdlc-harness-check.test.ts`, `sdlc-harness-metrics.test.ts`), `scripts/kitviet-update.mjs` và test của nó, `scripts/generate-kitviet-pack.test.mjs`, 3 test Go đọc `examples/plugins`.
- C12. Hạ tầng giả định nội dung nằm trong `packages/` hoặc `server/`: `.github/ci-paths.json`, `Dockerfile` (`COPY server/ ./server/`), `Dockerfile.web` (`COPY packages/ packages/`, build bằng `pnpm --filter @multica/web build` nên turbo không chạy), `turbo.json` (băm theo từng package), và `.vercelignore` loại `/scripts/` và `/server/` khỏi bản deploy web.
- C13. Skill vendor không có file nguồn gốc/giấy phép riêng (chỉ `frontend-design--frontend-design/LICENSE.txt`).
- C14. Id workflow template lặp ở 5 nơi (Go, union TS, tập lọc id trong schema, `template-picker-dialog.tsx`, `workflow.json` ×7); id lạ bị lọc bỏ nên template mới ở máy chủ không hiện trên client cũ.
- C15. Với mã sinh ra không commit (Q2): mọi lệnh cần dữ liệu sinh ra (`go build`, `go test`, `tsc`, `vitest`, `next build`) phải chạy bước sinh trước. Đây là chi phí mới của Q2, xử lý ở mục 6.7.

### 4.5 Quy trình thêm một squad hiện nay

1. `packages/core/squads/kitviet-squads/<id>.json` (nếu là Kit Việt). 2. `kitviet-templates/<id>.ts` và test. 3. `templates.ts`: import, `SquadTemplateId`, `SquadRoleKey`, `inCategory`. 4. Khóa dịch ở 7 `modals.json`. 5. `pnpm generate:kitviet`. 6. `kitviet-floors.ts` nếu có tầng riêng. 7. Khi đổi văn bản: thêm bản cũ vào `*-previous-texts.ts`. Squad lõi mới (như `research_lab`) cũng phải sửa `templates.ts` (import, hai union, mảng) và 7 `modals.json`.

## 5. Các phương án và quyết định

### 5.1 Mô tả

A. Quy ước thuần hệ thống file, phát hiện bằng glob, không có chỉ mục sinh ra. B. Thư mục + manifest + sinh mã. C. Kho lúc chạy (DB hoặc Hub: `hub_source`, `hub_item`, migration 927-970; `docs/engineering/skill-plugin-hub.md`).

### 5.2 So sánh

| Tiêu chí | A. Glob thuần | B. Manifest + sinh mã | C. Kho lúc chạy |
| --- | --- | --- | --- |
| Công thêm template | Thấp nếu bundler có glob; id và kiểu TS không tự có | Thấp: tạo thư mục, build; có scaffold | Thấp cho người dùng, cao cho bản dựng sẵn: API, giao diện admin, lệnh seed |
| Rủi ro hỏng hành vi | Trung bình: mất kiểu id | Thấp: chứng minh bằng băm từng bước | Cao: đổi nơi lưu 5 MB+, quickstart thành bất đồng bộ, schema DB |
| Phiên bản | Không có chỗ khai báo | `version` theo template | Tốt nhưng cần quy tắc nâng cấp theo workspace |
| Kiểm thử | Test tự duyệt thư mục | Kiểm tra tĩnh toàn bộ (mục 7) | Cần DB, chậm |
| Go + TS cùng dùng | `import.meta.glob` chỉ có ở Vite; Next và Metro không; `go:embed` không vượt ngoài `server/` | Một nguồn, đầu ra TS vào `packages/`, JSON vào `server/` | Go là nguồn, TS gọi API; client cũ gặp backend mới cần zod + fallback |
| Offline, desktop, mobile | Như nay | Như nay (đóng gói sẵn); mobile không dùng | Desktop cần mạng; thêm trạng thái tải |
| Bảo trì | Thấp lúc đầu | Trung bình: có CLI và schema | Cao: bảo mật chuỗi cung ứng prompt, nhập từ xa |

### 5.3 Quyết định: B, với mã sinh ra không commit

- B cho phép chứng minh "không đổi hành vi" từng bước bằng băm đầu ra.
- Q2 (không commit mã sinh ra) thay đổi cách chọn hạ tầng so với bản nháp đầu: thay vì commit đầu ra để Docker khỏi phải sinh, bản này chạy bộ sinh ngay trong build. Bộ sinh không phụ thuộc thư viện ngoài (chỉ Node ≥ 22), nên stage Docker chỉ cần image Node, không cần `pnpm install`.
- Bị loại: A (Next, Metro không có glob; Go không nhúng ngoài `server/`); C làm phương án duy nhất (rủi ro lớn, lợi ích chính chưa được yêu cầu); manifest YAML (thêm phụ thuộc, repo dùng JSON); workspace `@multica/templates` riêng (thêm workspace phải khai báo ở pnpm, knip, `ci-paths.json`); repo template tách riêng (mất tính nguyên tử giữa engine và nội dung).
- Đường nối sang C: giữ định dạng thư mục skill nguyên bản, để sau này xuất zip nhập được vào Hub.

### 5.4 Q4: hợp nhất ba bản SDLC Auto

Đã đo trên HEAD, bằng cách dựng cả ba bản với cùng 9 agent giả rồi so từng bước (script tạm; kết quả tái tạo được bằng plan T4.3a):

- Bản (a) `SDLC_AUTO_WORKFLOW` (TS, `templates.ts`) và bản (b) `sdlcSteps` + `sdlcStageSteps` (Go, `server/internal/service/initiative_build.go`) là cùng một workflow thực thi viết hai lần. Cả 16 trạng thái trùng tên; vai (handler), văn bản hướng dẫn của cả 16 bước và đích `back` đều giống hệt. 11 bước giống hoàn toàn; 5 bước còn lại chỉ khác ở đích `next` (xem bên dưới).
- Khác biệt thật duy nhất giữa (a) và (b) là cách biểu diễn đích "tiếp theo" của năm bước `backlog`, `todo`, `in_progress`, `in_review`, `qa`: (a) ghi `next` tường minh (`todo`→`in_progress`, `in_progress`→`in_review`, `in_review`→`qa`, `qa`→`done`); (b) không ghi chuyển tiếp mặc định nên engine tự lấy "trạng thái kế tiếp trên bảng, bỏ qua blocked" (`Definition.NextStatus` trong `server/internal/issueworkflow/issueworkflow.go`). Với `in_progress`, `in_review`, `qa` hai cách cho cùng đích. Với `todo`, thứ tự bảng của (b) là `backlog, todo, analysis, ...` nên đích ngầm là bước chuyên gia đầu tiên (thường là `analysis`) chứ không phải `in_progress` như câu hướng dẫn của chính bước đó. Đích "tiếp theo" được dùng khi giao việc (`issue_workflow_entry.go:103`), nên đây là khác biệt hành vi có thể gặp, chưa biết có chủ ý hay không.
- Bản (c) `sdlcAutoTemplate` (`server/internal/issueworkflow/templates.go`) là điểm xuất phát cho trình thiết kế: handler luôn `none`, hướng dẫn rút gọn, thêm cột `done` và `cancelled`, chuyển tiếp biểu diễn bằng điều kiện. Khác (a)/(b) có chủ đích, không phải lệch.

Quyết định (an toàn nhất):

1. Một manifest `catalog/workflows/sdlc_auto/` (mới) làm nguồn cho (a) và (b). TypeScript đọc dữ liệu sinh ra; Go đọc JSON sinh ra. Mỗi bước có `role`, `optionalRole`, `instructions`, `next`, `back`.
2. Giữ nguyên phép chiếu của từng bên để đầu ra không đổi (băm Go và băm TS không đổi): bước nào hiện không có chuyển tiếp mặc định ở Go mang cờ `implicitNext: true` trong manifest (`backlog`, `todo`, `in_progress`, `in_review`, `qa`). Cờ này làm sự khác biệt hiển thị và xóa được bằng một thay đổi dữ liệu nhỏ sau khi người sở hữu quyết định `todo` phải đi đâu. Tôi không tự đổi hành vi Go trong lần này.
3. Workflow của trình thiết kế (c) là manifest riêng (`catalog/workflows/designer_*`), không hợp nhất vào (1). Test khớp (mục 7, L2): các trạng thái và vai của (c) phải là tập con đã khai báo của (1) cộng `done`, `cancelled`; lệch thì check đỏ.
Kiểm chứng sau khi triển khai: băm bước Go dựng từ manifest (sau `Arrange`, với đủ 9 agent và với chỉ 4 agent bắt buộc) bằng đúng băm của mã Go cũ; test Go `TestDesignerSDLCAutoMatchesTheCatalogWorkflow` (`server/internal/service/catalog_workflows_test.go`) xác nhận bản của trình thiết kế có cùng tập trạng thái và cùng vai gợi ý với workflow build, cộng `done` và `cancelled`, nên (c) không lệch ngoài chủ ý. Phát hiện thêm: mô tả (`description`) của SDLC Auto khác nhau giữa TS ("Planning and build by agents: ...") và Go (`sdlcWorkflowDescription`: "Build, independent review, then QA, all by agents. ..."); lần này chỉ `steps` dùng chung, `description` của Go giữ nguyên.

4. Lý do không hợp nhất cả ba thành một: (c) khác về thiết kế nên hợp nhất sẽ phải thêm cờ cho mọi khác biệt, và sửa hành vi của trình thiết kế đang được người dùng phát triển.

## 6. Thiết kế đã chọn

### 6.1 Bố cục thư mục

```
catalog/
├── README.md                             hướng dẫn: thêm / sửa / xóa / đổi tên
├── registry.config.json                  locale, id đã gỡ (removed)
├── collections/<id>/                     nhóm hiển thị trong bộ chọn squad (core, kitviet, vertical)
│   ├── template.json
│   └── i18n/<locale>.json                chỉ có "title"
├── skillsets/<id>/template.json          tập skill đặt tên (sdlc, planning, sales, research_lab, starter_company, secretary)
├── skills/<nhóm>/<skill-id>/             nhóm (builtin, starter, custom) chỉ để sắp xếp
│   ├── SKILL.md                          nội dung gốc, không đổi vì manifest
│   ├── template.json                     tập thuộc về (sets), nguồn gốc (source), phiên bản
│   └── references/ scripts/ ...
├── squads/<id>/                          template.json, instructions.md (squad không phải Kit Việt), i18n/ (7 file)
├── workflows/<id>/template.json          workflow thực thi (sdlc_auto, planning_auto), dùng chung cho TS và Go
├── agents/<id>/                          template.json + instructions.md; agents/_blocks/*.md là khối chữ dùng chung ({{block:tên}})
├── companies/starter/template.json       công ty mẫu
├── packs/<id>/                           template.json, pack.json, instructions.md, skills/<tên>/, i18n/ (7 file)
├── bundles/quickstart/template.json      danh sách cài Claude Code
└── _sources/kitviet/                     đầu vào của bộ biến đổi Kit Việt (không phải template)
    └── upstream/ patches/ squads/ extensions/ skills/ rewrites.json
```

- Thư mục template = thư mục có `template.json`. Trong `skills/`, quét dừng ở `SKILL.md` đầu tiên.
- Tên thư mục = `id`; skill đổi `:` thành `--` (giữ quy ước có sẵn, vì `:` không hợp lệ trong đường dẫn Windows).
- `template.json` và `i18n/` không bao giờ nằm trong danh sách file cài đặt của skill (`skillFiles()` của `scripts/lib/skill-bundle.mjs` lấy mọi file trừ `SKILL.md`, nên bộ dựng mới phải loại hai thứ này).

### 6.2 Luồng dữ liệu

```
catalog/ (nguồn, người viết sửa; committed)
   │   node scripts/catalog/cli.mjs check    → L1..L5, L7
   │   node scripts/catalog/cli.mjs build    (turbo task //#catalog:build, make catalog-build, bước Docker)
   ▼
packages/core/catalog/generated/    (ignored)  skills, kitviet packs, squads, collections, i18n, agent-packs
server/internal/catalog/generated/*.json (ignored)  công ty mẫu, workflow, skill khởi đầu, bundle quickstart
   │                                  │
   ▼                                  ▼
@multica/core, @multica/views        Go: go:embed + loader có kiểu
```

### 6.3 Manifest: phong bì chung

| Trường | Kiểu | Bắt buộc | Quy tắc |
| --- | --- | --- | --- |
| `schemaVersion` | số nguyên | có | `1`. CLI và loader Go từ chối giá trị lạ |
| `kind` | chuỗi | có | `skill`, `skillset`, `collection`, `squad`, `workflow`, `company`, `pack`, `bundle` |
| `id` | chuỗi | có | Bất biến sau khi phát hành; quy tắc ở 6.3.1 |
| `version` | semver | có | Patch: sửa văn bản. Minor: thêm vai, skill, bước. Major: bỏ/đổi tên vai |
| `status` | `stable` \| `beta` \| `deprecated` \| `hidden` | không (`stable`) | `deprecated`, `hidden` không hiện trong bộ chọn nhưng vẫn nạp được |
| `order` | số | không | Thứ tự trong nhóm; mặc định theo `id` |
| `tags` | mảng chuỗi | không | |
| `requires` | mảng `{kind, id}` | không | Phụ thuộc (6.3.3) |
| `contracts` | mảng chuỗi | không | Hợp đồng code phụ thuộc (6.3.4) |
| `aliases` | mảng chuỗi | không | Id cũ |
| `source` | `{type, repo?, ref?, license?}` | có với `vendored` | `type`: `authored` hoặc `vendored` |

#### 6.3.1 Quy tắc id

- skill: `^[a-z0-9][a-z0-9._-]*(:[a-z0-9][a-z0-9._-]*)?$`, tối đa 64 ký tự (cùng quy tắc `SKILL_NAME` trong `packages/core/agent-packs/schema.ts`, thêm tiền tố plugin tùy chọn).
- pack: `^[a-z][a-z0-9-]{1,47}$`. Các loại còn lại: `^[a-z][a-z0-9_]{1,47}$`.
- Id duy nhất trong loại và bằng tên thư mục. Id được lưu trong dữ liệu người dùng (`squad.template_key`, tên skill/agent/workflow trong workspace), nên không đổi. Đổi tên = id mới + `aliases`, thẻ cũ `deprecated`. Gỡ hẳn: thêm vào `removed` trong `registry.config.json`; không tái sử dụng id đã gỡ.
- `check --base <ref>` (L7) so tập id với nhánh gốc bằng `git ls-tree` (chỉ đọc) và báo lỗi khi một id biến mất mà không nằm trong `removed`.

#### 6.3.2 Phiên bản

`version` của từng template độc lập với phiên bản sản phẩm. Nội dung đổi so với `--base` mà `version` không đổi: cảnh báo (Q5). Không có file khóa được commit (mã sinh ra không commit, Q2).

#### 6.3.3 Phụ thuộc

`requires: [{ "kind": "skillset", "id": "sdlc" }]`. Chỉ id, chưa có khoảng phiên bản (cả registry dựng từ một snapshot git). Bộ dựng tính bao đóng, lỗi khi thiếu hoặc có vòng, cảnh báo khi `stable` cần `beta` hoặc `deprecated`. Quan hệ tự suy ra (skill squad liệt kê, workflow squad tham chiếu) không cần khai báo lại.

#### 6.3.4 Hợp đồng

`contracts: ["squad.sdlc"]` khai báo template đáp ứng một hợp đồng mà code phụ thuộc. Id hợp đồng có dạng `<loại>.<tên>` (`squad.sdlc`, `workflow.planning_auto`, `company.executive_office`) và chỉ template cùng loại mới được khai báo; bộ kiểm tra bắt lỗi dạng và loại. Go liệt kê hợp đồng nó cần trong `server/internal/catalog/contracts.go` (`RequiredContracts`), TypeScript trong `packages/core/catalog/contracts.ts` (`REQUIRED_CONTRACTS`). Mỗi hợp đồng cần ít nhất một template `stable` đáp ứng, không phải đúng một: hai squad SDLC (`sdlc_greenfield`, `sdlc_maintenance`) cùng đáp ứng `squad.sdlc`. Test `TestRequiredContractsProvided` (Go) và `contracts.test.ts` (TS) kiểm tra điều này, và các test trong `server/internal/service/catalog_contracts_test.go` kiểm tra tên mà mã Go tìm theo (tên agent SDLC, `Blueprint Lead`, `BMAD Planning Auto`, `SDLC Auto`, thẻ `executive_office` với vai `librarian`) vẫn khớp catalog, để không đổi tên hay xóa nhầm một template mà Go đang dựa vào.

#### 6.3.5 Dịch (Q3: đủ mọi ngôn ngữ, không fallback)

- Chỉ chuỗi hiển thị cho người dùng (tên, mô tả thẻ, tiêu đề collection) được dịch. Loại có hiển thị: `collection`, `squad`, `pack`, `workflow` (loại xuất hiện trên giao diện). Nội dung dành cho agent giữ một ngôn ngữ như hiện nay.
- `i18n/<locale>.json` đối tượng phẳng `{ "title", "description" }`; riêng `collection` chỉ có `{ "title" }` (tiêu đề nhóm, không có mô tả). Danh sách locale lấy từ `registry.config.json` và phải bằng `SUPPORTED_LOCALES` trong `packages/core/i18n/types.ts` (hiện `en`, `zh-Hans`, `zh-Hant`, `ko`, `ja`, `fr`, `es`); check đọc file này và so sánh.
- Thiếu một locale, `title`/`description` rỗng, hoặc có khóa thừa: lỗi, check và build đều thất bại. Không điền thay bằng `en`.
- Bản dịch hiện có (28 squad, 3 collection, 1 pack, 7 workflow ở `modals.json`, `workflow.json`) được chuyển nguyên văn bằng codemod; build không đọc `modals.json` để dịch nữa.
- Dữ liệu dịch sinh ra là TypeScript thuần (`packages/core/catalog/generated/i18n.ts`) cộng hàm `catalogText(kind, id, locale)` ở `packages/core/catalog/` (mới). Giao diện lấy locale hiện tại từ hook `useLocale` đã có ở `packages/views/i18n`. Cách này không cần đăng ký namespace i18next mới hay file locale bị ignore.
- Tên entity tạo trong workspace (tên squad, tên agent) không dịch, vì chạy lại quickstart tìm theo tên.

### 6.4 Khối riêng theo loại

skill (`template.json` cạnh `SKILL.md`):

```json
{ "schemaVersion": 1, "kind": "skill", "id": "sdlc-harness", "version": "1.0.0",
  "sets": ["sdlc"], "source": { "type": "authored" } }
```

`name`, `description` lấy từ frontmatter `SKILL.md`. Quy tắc L3: `name` bằng `id` hoặc phần sau dấu `:`. Skill vendor: `source.type = "vendored"` cần `repo`, `ref`, `license`. Đã đo: 90 skill hiện có đều khớp quy tắc tên (ba skill `kv-*` nằm trong `_sources/` nên miễn).

skillset: `{ "kind": "skillset", "id": "planning" }`, thành viên suy ra từ `sets` của skill (thêm skill vào tập không sửa file nào khác). Một skill thuộc nhiều tập (hiện `bmad-method:bmad-prd` thuộc cả SDLC và planning).

collection: `{ "kind": "collection", "id": "kitviet", "for": "squad", "order": 20 }` + `i18n/`. Thay `SquadTemplateCategory`. Một "bộ template" mới = một collection + các squad của nó.

squad: các trường của `SquadTemplate` (`templates.ts`): `collection`, `expertReview`, `squad.{name, floorName, leader, operatingReview, instructionsFile, workflow, roles[], routines[], charter, skills{sets, extra, leader, byRole}}`. Squad Kit Việt có thêm khối `adapter` (`type: "kitviet"`, `pack`): tên vai, loại vai, `builtinSkills`, `leaderSkills`, `roleSkills` và hướng dẫn của lead được dẫn ra từ đầu ra của bộ biến đổi Kit Việt thay vì ghi tay trong manifest, đúng như `kitviet-squad-roles.ts` làm hôm nay. `role.key` duy nhất toàn registry vì `SquadRoleKey` là union phẳng.

workflow: `workflow.{name, description, steps[{status, role, optionalRole, instructions, next, back, implicitNext}]}`; xem 5.4. Workflow của trình thiết kế là manifest `kind: workflow` có khối `designer` thay cho khối `workflow` (id `designer_<templateId>`, `ui: false`, `order` là thứ tự trong bộ chọn): `designer.{templateId, steps[{statusKey | new{name, category}, roleHint, instructions, transitions[{on?, to}]}]}`; `on` là `rejected` hoặc `blocked`, bỏ `on` là nhánh mặc định. Chúng không chạy được từ squad (bộ kiểm tra từ chối `squad.workflow` trỏ tới chúng).

company: `COMPANY_FLOOR_KEYS`, `COMPANY_FLOOR_NAMES`, `COMPANY_SQUAD_CARDS`, `COMPANY_STAGE_OWNERS`, `COMPANY_START_OPTIONS` như hiện có. pack: `pack.json` hiện tại thêm phong bì. bundle: nội dung `server/internal/quickstart/bundle.json`.

### 6.5 Ví dụ "thêm một bộ template mới"

```
catalog/collections/customer_success/{template.json, i18n/<7 locale>.json}
catalog/skillsets/cs_core/template.json
catalog/skills/customer-success/cs-triage/{SKILL.md, template.json}
catalog/squads/cs_desk/{template.json, instructions.md, i18n/<7 locale>.json}
```

`node scripts/catalog/cli.mjs new squad cs_desk --collection customer_success` tạo khung (kể cả 7 file dịch để điền), sau đó `pnpm catalog:build`. Không sửa file nào trong `packages/` hay `server/`. Tiêu chí nghiệm thu G2: chạy đúng quy trình này với một squad thử rồi xóa (plan T3.7).

### 6.6 Đầu ra sinh ra (không commit)

| Đầu ra | Nội dung | Ai dùng |
| --- | --- | --- |
| `packages/core/catalog/generated/skills-builtin.ts` | `SDLC_BUILTIN_SKILLS` (skill của các skillset có `target: "quickstart"`, khoảng 2 MB) | `quickstart.ts`, nạp lazy khi cài skill |
| `packages/core/catalog/generated/skillsets.ts` | `SKILLSET_MEMBERS`, nhỏ, tách riêng để mã chỉ cần tên skill không kéo gói trên | `catalog/skillsets.ts` |
| `packages/core/catalog/generated/role-instructions.ts` | Hướng dẫn theo vai (`catalog/squads/<id>/roles/<vai>.md`) | `catalog/role-instructions.ts`, bọc bởi `*-agent-instructions.ts`, `content-studio.ts`, `research-lab.ts` |
| `packages/core/catalog/generated/contracts.ts` | Template đáp ứng từng hợp đồng | `catalog/contracts.ts` |
| `packages/core/catalog/generated/kitviet/*` | Pack Kit Việt do bộ biến đổi `scripts/generate-kitviet-pack.mjs` sinh | `kitviet-packs.ts`, `kitviet-squad-roles.ts` |
| `packages/core/catalog/generated/squads.ts`, `collections.ts` | Dữ liệu squad; kiểu `SquadTemplateId`, `SquadRoleKey`, `SquadTemplateCategory` | `catalog/squads.ts` (lắp ráp `SquadTemplate`) |
| `packages/core/catalog/generated/i18n.ts` | Bản dịch squad, collection, pack theo locale | `catalogText()` |
| `packages/core/catalog/generated/agents.ts`, `company.ts`, `workflows.ts` | Mẫu hướng dẫn agent (đã mở khối chung), công ty mẫu, workflow | `agents/instruction-templates.ts`, `squads/company-template.ts`, `squads/templates.ts` |
| `packages/core/catalog/generated/agent-packs/` | Bundle JSON của từng pack và `index.ts` (id + bộ nạp) | `agent-packs/load.ts` |
| `server/internal/catalog/generated/{company,workflows,quickstart-bundle}.json` | Dữ liệu cho Go (`go:embed all:data`) | `server/internal/catalog` |

Không có `registry.lock.json`: khi không commit mã sinh ra thì không còn gì để khóa. Kiểm tra id không biến mất dùng `catalog check --base <ref>` (đọc `git ls-tree`, chỉ đọc).

### 6.7 Hệ quả của việc không commit mã sinh ra (Q2)

- `.gitignore`: thêm `packages/core/catalog/generated/` và `server/internal/catalog/generated/*.json`. Các file sinh ra đang commit (`sdlc-skills-data.ts`, `kitviet-skills-data/`, `company_template_data.json`, `agent-packs/bundles/`) bị xóa khỏi cây làm việc; người dùng ghi nhận việc xóa trong commit của mình.
- Bộ sinh chạy ở mọi điểm vào build. Một nguồn duy nhất: `node scripts/catalog/cli.mjs build [--target ts|go|all]`, thêm vào `package.json` gốc thành `catalog:build`.
  - turbo: task gốc `//#catalog:build` (`inputs`: `catalog/**`, `scripts/catalog/**`, `scripts/lib/**`; `outputs`: các đường dẫn sinh ra); `build`, `typecheck`, `test`, `lint`, `dev`, `dev:staging` phụ thuộc vào nó. Nhờ `inputs` của task gốc, một thay đổi trong `catalog/` làm mất cache của mọi task sau nó (không còn rủi ro cache cũ như bản nháp đầu).
  - `Dockerfile.web`: build bằng `pnpm --filter @multica/web build` (không qua turbo) nên thêm `COPY catalog/`, `COPY scripts/catalog`, `COPY scripts/lib` và `RUN node scripts/catalog/cli.mjs build --target ts` trước bước build.
  - `Dockerfile` (Go): thêm stage `FROM node:22.23.4-alpine AS catalog` chạy `build --target go`, rồi `COPY --from=catalog` vào `server/internal/catalog/generated/` của stage builder trước `go build`.
  - `.vercelignore`: hiện loại `/scripts/`; thêm ngoại lệ `!/scripts/catalog`, `!/scripts/lib` (và nội dung) để bản deploy web có bộ sinh. `catalog/` không bị loại.
  - Makefile: target `catalog-build`; các target gọi `go build` hoặc `go run` (`build`, `server`, `daemon`, `dev`...) và `test` phụ thuộc vào nó. Script `scripts/dev.sh`, `scripts/dev-env.sh`, `scripts/check.sh`, `scripts/test-go.sh` gọi trước.
  - CI: job Go cần setup-node và bước `node scripts/catalog/cli.mjs build --target go` trước `go build/test/vet`; job frontend đã qua turbo nên chạy cùng task gốc; thêm bước `node scripts/catalog/cli.mjs check`. `.goreleaser.yml`: `before.hooks`.
  - `go:embed all:data` với một file giữ chỗ `.gitkeep` được commit: nếu chưa sinh dữ liệu thì package vẫn biên dịch được, loader trả lỗi rõ ràng ("chạy catalog:build"), nên các job Windows chỉ chạy vài test cụ thể không bị vỡ khi biên dịch.
  - Sau khi clone: chạy `pnpm catalog:build` một lần (các lệnh `pnpm typecheck/test/lint/build/dev` đã tự gọi qua turbo; lệnh gọi trực tiếp `vitest` thì chưa).
- Mobile không nhập mã sinh ra (đã kiểm: chỉ `apps/mobile/data/queries/squads.ts` nhắc "squads", qua API).

## 7. Kiểm tra (validation)

CLI: `node scripts/catalog/cli.mjs <check|build|new|list>`; alias `pnpm catalog:check|build|new|list`. Test CLI: `node --test scripts/catalog/*.test.mjs`.

| Lớp | Nội dung | Mức |
| --- | --- | --- |
| L1 Schema | Phong bì và khối riêng theo loại; id đúng regex và bằng tên thư mục; semver; `schemaVersion`; id duy nhất | Lỗi |
| L2 Tham chiếu | `requires`, `sets`, `collection`, `workflow.ref`, skill squad liệt kê, `leader` thuộc `roles`, `role` của bước workflow thuộc squad dùng nó, `role.key` duy nhất toàn registry, không vòng; workflow trình thiết kế khớp SDLC Auto (5.4) | Lỗi |
| L3 Nội dung | `SKILL.md` có frontmatter `name` khớp và `description` không rỗng; chỉ văn bản; trần kích thước; không có mẫu bí mật (`sk-`, `AKIA`, khóa riêng); skill vendor có `source.license`; "chuỗi cấm" của Kit Việt giữ ở test hiện có | Lỗi (mồ côi, `license: "TODO"`: cảnh báo) |
| L4 Dịch | Mọi loại có hiển thị: đủ mọi locale, khóa bằng nhau, không rỗng; danh sách locale bằng `SUPPORTED_LOCALES` | Lỗi (Q3) |
| L5 Sinh | Build chạy trọn vẹn và đầu ra qua `tsc` (typecheck) và `go build`; thay cho kiểm tra "độ tươi" | Lỗi |
| L6 Tương đương | Trong lúc di chuyển: băm từng mục đầu ra (205 mục) so với `scripts/catalog/golden-hashes.json` bằng `scripts/catalog/golden.mjs`; công cụ tạm này đã chạy xanh (205/205) ở lần kiểm cuối rồi bị xóa cùng các codemod một lần; không còn trong repo | Lỗi |
| L7 Bất biến | `check --base <ref>`: id không biến mất nếu không nằm trong `removed`; nội dung đổi mà `version` không đổi | Lỗi (id), cảnh báo (version) |

Test vitest sinh theo registry thay test liệt kê tên: `packages/core/catalog/squad-invariants.test.ts` (mới) duyệt mọi squad với các khẳng định đang lặp ở `kitviet-templates/*.test.ts`. Go: `server/internal/catalog` có test phân tích dữ liệu nhúng và test hợp đồng.

## 8. Bảo mật và giấy phép

Hướng dẫn agent là chuỗi cung ứng prompt: coi thay đổi `catalog/**` như thay đổi code khi duyệt (đề xuất `CODEOWNERS`; `.github/` hiện chưa có). Skill vendor bắt buộc có `source.license`; trong lần này các skill vendor được đánh dấu `TODO` (Q9) và check cảnh báo. Không có bí mật trong `catalog/`; L3 quét mẫu cơ bản. Go không đọc đường dẫn lấy từ manifest.

## 9. Tương thích và rủi ro

- Không có migration DB. Id template, tên skill, tên agent, tên workflow giữ nguyên.
- API: trường mới là tùy chọn có mặc định; cập nhật zod schema và test phản hồi sai định dạng nếu đổi.
- Không thêm lớp tương thích nội bộ: mỗi giai đoạn chuyển hẳn và xóa đường cũ.

| # | Rủi ro | Giảm thiểu |
| --- | --- | --- |
| R1 | Lệch hành vi vì chữ đổi nhẹ | L6 băm từng mục; codemod, không gõ lại |
| R2 | Đụng thay đổi chưa commit của người dùng | Chỉ làm khi `git status` sạch trong vùng; hiện sạch; mọi bước dùng `mv`, không đụng index |
| R3 | Mã sinh ra không commit làm vỡ một điểm vào build không qua turbo | Liệt kê ở 6.7; kiểm tra từng điểm vào |
| R4 | `go build` thất bại khi chưa sinh dữ liệu Go | `go:embed all:data` + `.gitkeep`, loader báo lỗi rõ |
| R5 | Mất kiểm tra kiểu khóa dịch | Union id sinh ra; hàm `catalogText` nhận id kiểu union |
| R6 | CLI trở thành gánh nặng | Tối thiểu 4 lệnh, không thư viện ngoài, test riêng |

## 10. Câu hỏi còn mở

- Q7. Có cần bước tiếp theo cho template của bên thứ ba qua Hub không? Nếu có, nên chốt định dạng xuất zip.
- Q9. Quyết định về `react-doctor` và hai repo không có giấy phép (xem mục 11).
- Q4b. Đích "tiếp theo" của bước `todo` trong workflow do Go dựng nên là `in_progress` (như bản TS và câu hướng dẫn) hay bước chuyên gia đầu tiên (hành vi hiện tại)? Hiện giữ nguyên hành vi (cờ `implicitNext`).
- Q11. Có nên bỏ cờ `implicitNext` và để Go dùng chuyển tiếp tường minh như TS, sau khi trả lời Q4b?

## 11. Hiện trạng triển khai (as-built)

Đã làm (kèm cách kiểm chứng, chi tiết lệnh ở báo cáo cuối phiên):

- Thư mục `catalog/` với 174 template: 93 skill, 6 skillset, 28 squad, 3 collection, 2 workflow thực thi, 39 mẫu hướng dẫn agent (+ 5 khối dùng chung), công ty mẫu `starter`, agent pack `market-strategist`, bundle `quickstart`. Nguồn Kit Việt ở `catalog/_sources/kitviet/`.
- CLI `scripts/catalog/cli.mjs` (`check`, `build`, `new`, `list`), không phụ thuộc thư viện ngoài; 25 test Node (`catalog.test.mjs`); `catalog check --base <ref>` kiểm id không biến mất.
- Mã sinh ra không commit: `.gitignore`, task turbo gốc `//#catalog:build`, `Makefile`, `scripts/{test-go,check,dev,dev-env}.sh`, `Dockerfile` (stage `catalog`), `Dockerfile.web`, `.dockerignore`, `.vercelignore`, `.goreleaser.yml`, `.github/workflows/ci.yml`, `.github/actions/frontend-quality`, `.github/ci-paths.json`, `knip.jsonc`.
- Bản dịch đủ 7 ngôn ngữ cho squad, collection và pack; chuyển nguyên văn từ `modals.json` rồi xóa khóa cũ ở đó. Giao diện đọc qua `catalogText()`.
- Thêm một squad không sửa file code: kiểm chứng bằng `catalog new squad zz_probe` (typecheck, test bất biến và test `templates` xanh, `git status` chỉ có file dưới `catalog/` và thư mục sinh ra), rồi xóa.
- Q4: `sdlc_auto` và `planning_auto` là manifest duy nhất cho TS và Go; băm đầu ra giữ nguyên.

Khác với bản thiết kế ban đầu:

- Skillset đặt id snake_case (`research_lab`, `starter_company`) theo quy tắc id, không phải kebab.
- Collection chỉ cần `title` ở `i18n/`; loại `agent` chưa có bản dịch (tên và mô tả mẫu trong bộ chọn agent vẫn ở `agents.json`).
- 21 test riêng của squad Kit Việt không bị thay bằng test chung: chúng được giữ nguyên nội dung, chỉ đổi cách lấy dữ liệu, ở `packages/core/catalog/kitviet/`; test chung `packages/core/catalog/squad-invariants.test.ts` bổ sung cho mọi squad.
- Thứ tự tên skill trong `PLANNING_SKILL_NAMES`, `SALES_SKILL_NAMES`, `RESEARCH_LAB_SKILL_NAMES` (và `builtinSkills` của squad tương ứng) nay theo tên thư mục thay cho thứ tự viết tay; cùng tập, đã đối chiếu với `git show HEAD`.
- Năm khối chữ dùng chung của mẫu hướng dẫn agent nay là `catalog/agents/_blocks/*.md` (trước là hằng số trong `instruction-templates.ts`, `instruction-blocks.ts`); năm mẫu `kv_*` được ghi sẵn ở `catalog/agents/kv_*` để Go đọc, và test `packages/core/catalog/agents.test.ts` bảo đảm chúng bằng kết quả của bộ ghép Kit Việt.

Làm thêm ở đợt hoàn thiện:

- Hợp đồng (6.3.4): trường `contracts` trên 5 template và công ty mẫu, bộ kiểm tra dạng/loại, `server/internal/catalog/contracts.go` và test Go/TS (xem 6.3.4).
- Hướng dẫn theo vai: 37 file `catalog/squads/<squad>/roles/<vai>.md` thay các bảng chữ trong `{sdlc,planning,sales}-agent-instructions.ts`, `content-studio.ts`, `research-lab.ts`; các file này nay chỉ bọc dữ liệu sinh ra (giữ tên export và kiểu). Đối chiếu với bản cũ: bằng nhau từng khóa.
- 7 workflow của trình thiết kế: `catalog/workflows/designer_*`; `issueworkflow.Templates()` đọc `designer-workflows.json`. JSON của `Templates()` bằng bản ở HEAD.
- Tải lazy (C4): `SDLC_BUILTIN_SKILLS` (2,1 MB) và `KITVIET_PREVIOUS_*` (2,6 MB) nạp bằng `import()`; `SKILLSET_MEMBERS` tách file riêng. Đo bằng esbuild có `splitting`: phần nạp tĩnh của `squads/upgrade.ts` từ khoảng 4,0 MB xuống khoảng 1,2 MB; `squads/quickstart.ts` còn khoảng 1,2 MB (không còn gói skill).
- Nguồn và giấy phép của 51 skill vendor (Q9): đã điền; `catalog check` lỗi nếu còn `TODO` và cảnh báo khi ref `unpinned`, repo không có giấy phép, hoặc giấy phép có giới hạn (xem cảnh báo bên dưới).

Chưa làm:

- API trả tên workflow của trình thiết kế theo locale (T4.4): `packages/core/api/schemas.ts`, `types/issue-workflow.ts`, `template-picker-dialog.tsx` và `workflow.json` ×7 là mã trình thiết kế do người dùng đang phát triển; bản dịch của 7 template vẫn nằm ở `workflow.json`. Id workflow vẫn lặp ở 5 nơi (C14); manifest chỉ gom dữ liệu bước.
- Thay `*-previous-texts.ts` bằng băm (Q6): không làm. `upgrade.test.ts` và `kitviet-previous-texts.test.ts` cần văn bản gốc (dựng agent "tạo bằng bản cũ", kiểm tra bản cũ còn câu luật VERDICT cũ); băm làm mất các kiểm tra này, và `scripts/kitviet-update.mjs` ghi thêm văn bản vào file đó. Đã giải quyết phần kích thước bằng tải lazy.
- Hợp nhất mô tả (`description`) của SDLC Auto giữa TS và Go; bỏ cờ `implicitNext` (Q4b, Q11).
- Cảnh báo "nội dung đổi mà `version` không đổi" (Q5), `CODEOWNERS`, quét bí mật ngoài bốn mẫu cơ bản.
- `sales-starter-sources.ts` vẫn là mã.

Rủi ro giấy phép của skill vendor cần người sở hữu quyết định: `react-doctor` (`LicenseRef-Modified-MIT-Million`) cấm dùng làm dữ liệu huấn luyện/đánh giá AI và bán lại như dịch vụ lưu trữ; `AccessLint/skills` và `bmad-code-org/bmad-plugins` không có tệp giấy phép; 20 skill không có commit hoặc phiên bản để ghim.

## 12. Chưa kiểm chứng

- `make check` và `pnpm exec playwright test` không chạy: một frontend của người dùng đang chạy ở cổng 30000, nên E2E sẽ dùng dịch vụ và dữ liệu đó.
- Không build trọn `Dockerfile.web` (chỉ chạy bước sinh trong container Node); không chạy workflow CI/release thật.
- `make test` trên cơ sở dữ liệu sạch (tạo bằng `scripts/ensure-postgres.sh`, xóa sau khi chạy) chỉ còn đỏ ở `TestMigrationNumericPrefixesAreUnique`, cũng đỏ ở HEAD (16 cặp migration trùng số, từ `600` trở lên). Các test khác đỏ lúc máy quá tải đều xanh khi chạy lại riêng. Trên cơ sở dữ liệu dùng chung có sẵn rác từ các lần test bị ngắt thì thêm vài test đỏ vì rác.
