# Catalog: kế hoạch triển khai

Đặc tả: [spec.md](spec.md) (bản duyệt lần 2: thư mục `catalog/`, mã sinh ra không commit, bản dịch đủ mọi ngôn ngữ, Q4 đã chọn).
Quy ước: đường dẫn kèm "(mới)" sẽ được tạo; đường dẫn còn lại đã tồn tại lúc viết. Cột "Trạng thái" ở mục 12 được cập nhật khi triển khai.

## 1. Nguyên tắc

1. Mỗi giai đoạn để lại cây làm việc xanh với các kiểm tra của giai đoạn đó. Không commit (người dùng commit sau).
2. Cổng bắt buộc của mọi bước di chuyển: băm vàng `scripts/catalog/golden-hashes.json` (mới, tạm) bằng nhau trước và sau (lớp L6). Băm nội dung đầu ra, không băm đường dẫn.
3. Dùng `mv` và codemod một lần, không gõ lại nội dung. Không dùng lệnh làm đổi trạng thái git (`git mv`, `git rm`, `git add`...). Các file đã commit mà ta xóa/di chuyển sẽ hiện là "deleted"/"untracked" cho đến khi người dùng commit.
4. Không thêm lớp tương thích nội bộ. Mỗi giai đoạn chuyển hẳn và xóa đường cũ trong cùng giai đoạn.
5. Không có migration DB; id, tên skill/agent/workflow giữ nguyên.
6. Chạy kiểm tra hẹp khi làm; cuối cùng chạy các lệnh bắt buộc của `AGENTS.md` (`pnpm typecheck`, `pnpm lint`, `pnpm test`, `make test`, `make check` nếu môi trường cho phép) và báo đúng kết quả.

## 2. Phạm vi lần triển khai này

Làm: giai đoạn 0 đến 6 dưới đây. Chưa làm (ghi rõ để khỏi hiểu nhầm là đã xong):

- Thay `*-previous-texts.ts` bằng danh sách băm (Q6).
- Tải lazy nội dung squad theo chunk và đo bundle (spec C4): chỉ đo (T0.3), không chia gói.
- Hợp nhất workflow của trình thiết kế (c) với SDLC Auto (không làm theo Q4).
- Đổi hành vi `todo` của workflow do Go dựng (Q4b).
- Chuyển `sales-starter-sources.ts`, `*-agent-instructions.ts` (vai SDLC, planning, sales) sang catalog.

## 3. Giai đoạn 0: nền và băm vàng

### T0.1 Băm vàng của đầu ra hiện tại
- `scripts/catalog/golden.mjs` (mới, tạm): dựng bằng esbuild (như `scripts/generate-company-template.mjs`) rồi băm sha256 từng mục: `SQUAD_TEMPLATES` (28) cùng `SQUAD_ROLES`, `SDLC_AUTO_WORKFLOW`, `PLANNING_AUTO_WORKFLOW`, ba danh sách tên skill, `SDLC_BUILTIN_SKILLS`, từng pack Kit Việt, dữ liệu công ty mẫu (`COMPANY_*`), `AGENT_INSTRUCTION_TEMPLATES`, bundle agent pack. Phía Go: dump `issueworkflow.Templates()`, `sdlcSteps` (với 9 agent giả), `quickstart.LoadBundle()` qua một test tạm có build tag, băm đầu ra.
- Kiểm tra: chạy hai lần cho cùng kết quả; sửa một ký tự ở một template thì đúng một băm đổi.

### T0.2 Ghi nền các kiểm tra
- `node scripts/generate-kitviet-pack.mjs --check`; `node --test scripts/generate-kitviet-pack.test.mjs scripts/kitviet-update.test.mjs scripts/ci-scope.test.mjs`; `pnpm --filter @multica/core exec vitest run squads`; `cd server && go test ./internal/issueworkflow/ ./internal/quickstart/`.

### T0.3 Đo kích thước bundle
- `esbuild --bundle --metafile` trên điểm vào chỉ import `@multica/core/squads`; ghi byte của `sdlc-skills-data.ts` trong bundle. Chỉ ghi số liệu vào báo cáo.

## 4. Giai đoạn 1: di chuyển nguồn vào `catalog/`

Chỉ đổi đường dẫn. Đầu ra sinh ra phải giống hệt trừ dòng comment ghi đường dẫn nguồn. Kiểm tra sau mỗi task: sinh lại + `git diff -U0` chỉ còn dòng comment (hoặc băm vàng bằng nhau), cộng test hẹp.

- T1.1 Khung: `catalog/README.md`, `catalog/registry.config.json` (mới). `.github/ci-paths.json`: thêm `catalog/**` và `scripts/catalog/**` vào `frontend`, `backend`, `quality`. `.vercelignore`, `.gitignore` (xem T2.6).
- T1.2 Skill SDLC: `packages/core/squads/sdlc-skills` → `catalog/skills/builtin`; sửa `scripts/generate-sdlc-skills.mjs`, `sdlc-skills.test.ts`, `sdlc-harness-check.test.ts`, `sdlc-harness-metrics.test.ts` (đường dẫn script trong skill), cùng mọi chỗ khác tham chiếu (`git grep`).
- T1.3 Skill khởi đầu: `packages/core/squads/skills` → `catalog/skills/starter`; sửa `scripts/generate-company-template.mjs`.
- T1.4 Nguồn Kit Việt: `kitviet-upstream`, `kitviet-patches`, `kitviet-rewrites.json`, `kitviet-squads`, `kitviet-extensions`, `kitviet-skills` → `catalog/_sources/kitviet/{upstream,patches,rewrites.json,squads,extensions,skills}`; sửa `scripts/generate-kitviet-pack.mjs` (+ test), `scripts/kitviet-update.mjs` (+ test), `kitviet-skills.test.ts`, `kitviet-packs.test.ts`...
- T1.5 Agent pack: `examples/agent-packs` → `catalog/packs`; sửa `scripts/build-agent-pack-bundles.mjs`, `scripts/install-agent-pack.sh`, `Makefile`.
- T1.6 Tài liệu: cập nhật đường dẫn trong `docs/engineering/*.md`.
- Kiểm tra giai đoạn: băm vàng bằng nhau; `node --test scripts/generate-kitviet-pack.test.mjs scripts/kitviet-update.test.mjs scripts/ci-scope.test.mjs`; `node scripts/generate-kitviet-pack.mjs --check`; `pnpm --filter @multica/core exec vitest run squads agent-packs`; `pnpm typecheck`.

## 5. Giai đoạn 2: lõi catalog, skill, và sinh lúc build

### T2.1 CLI không phụ thuộc thư viện ngoài
`scripts/catalog/` (mới): `cli.mjs` (lệnh `check`, `build`, `new`, `list`), `lib/discover.mjs`, `lib/schema.mjs` (kiểm tra thủ công theo spec 6.3, 6.4), `lib/emit-ts.mjs`, `lib/emit-go.mjs`, `lib/i18n.mjs`, test `*.test.mjs`. Dùng lại `scripts/lib/skill-bundle.mjs`; `skillFiles` loại `template.json` và `i18n/`.

### T2.2 Manifest cho skill và skillset
Codemod một lần `scripts/catalog/codemods/skills.mjs` (mới, xóa ở giai đoạn 6) ghi `template.json` cho 93 skill (83 `builtin` + 10 `starter`) với `sets` suy ra từ quy tắc hiện hành (`SDLC_SKILL_NAMES`, `PLANNING_SKILL_NAMES`, `SALES_SKILL_NAMES`, `RESEARCH_LAB_SKILL_NAMES`), và `catalog/skillsets/*/template.json`. Kiểm tra: ba danh sách tên suy ra từ `sets` bằng danh sách cũ (băm vàng).

### T2.3 Builder skill
Sinh `packages/core/catalog/generated/skills-builtin.ts` (cùng hình dạng `BuiltinSkill` như `sdlc-skills-data.ts`) và tên theo tập. Sửa `templates.ts`, `quickstart.ts`, `kitviet-packs.ts`, `scripts/generate-kitviet-pack.mjs` (import kiểu). Xóa `scripts/generate-sdlc-skills.mjs`, `packages/core/squads/sdlc-skills-data.ts`, mục `generate:sdlc-skills`.

### T2.4 Builder Kit Việt và agent pack trong build
`catalog build` gọi bộ biến đổi Kit Việt với đầu ra mới `packages/core/catalog/generated/kitviet/` và bộ biến đổi pack với đầu ra `packages/core/catalog/generated/agent-packs/`. Sửa các import của `kitviet-skills-data/*` và `agent-packs/bundles/*`. Xóa các file sinh ra đang commit.

### T2.5 Test thay thế
`sdlc-skills.test.ts` bỏ danh sách tên viết tay; `templates.test.ts` bỏ số đếm cứng liên quan tới skill; test đọc dữ liệu sinh ra.

### T2.6 Hạ tầng build (Q2)
`.gitignore` (hai đường dẫn sinh ra); `package.json` gốc (`catalog:build`, `catalog:check`, `catalog:new`, `catalog:list`; bỏ các script `generate:*` đã thay); `turbo.json` (task gốc `//#catalog:build`, các task phụ thuộc vào nó); `.vercelignore`; `Dockerfile.web`; `knip.jsonc`; `.github/workflows/ci.yml` (bước `catalog check`, `node --test scripts/catalog/*.test.mjs`).

Kiểm tra giai đoạn: băm vàng; `node scripts/catalog/cli.mjs check` và `build`; `node --test scripts/catalog/*.test.mjs`; `pnpm typecheck`; `pnpm --filter @multica/core exec vitest run squads agent-packs`; `pnpm exec turbo run build --dry=json` cho thấy `//#catalog:build` trong đồ thị phụ thuộc; `pnpm knip`.

## 6. Giai đoạn 3: squad, collection, bản dịch

- T3.1 Schema squad/collection trong CLI (spec 6.4).
- T3.2 Codemod 7 squad lõi (`templates.ts`, `content-studio.ts`, `research-lab.ts`) → `catalog/squads/<id>/{template.json,instructions.md}`; bản dịch từ `squad_quickstart.templates.<id>` của 7 `modals.json` → `i18n/<locale>.json`.
- T3.3 Codemod 21 squad Kit Việt: phần viết tay của `kitviet-templates/<trade>.ts` (mô tả vai, dòng roster, charter, routine, floor, thứ tự vai) + khối `adapter`; bộ biến đổi Kit Việt đọc cấu hình từ `catalog/_sources/kitviet/squads/` như cũ.
- T3.4 Collection: `core`, `kitviet`, `vertical` + bản dịch từ `squad_quickstart.groups` của 7 locale.
- T3.5 Sinh `packages/core/catalog/generated/{squads,collections,i18n}.ts`; viết tay `packages/core/catalog/` (lắp ráp `SquadTemplate` từ dữ liệu, `catalogText`, kiểu `SquadTemplateId`, `SquadRoleKey`). Thu gọn `templates.ts` (bỏ 23 import, hai union, mảng, `inCategory`); xóa `kitviet-templates/*.ts` (21 + 21 test), `content-studio.ts`, `research-lab.ts` (và test của chúng được thay).
- T3.6 Giao diện: `squad-quickstart.tsx`, `company-setup-card.tsx`, `company-starter-wizard.tsx` đọc tên/mô tả qua `catalogText` + `useLocale`; xóa `squad_quickstart.templates` và `groups` khỏi 7 `modals.json`; cập nhật test giao diện.
- T3.7 Nghiệm thu "thêm không sửa code": tạo squad thử `zz_probe` bằng `catalog new`, build, `pnpm typecheck` và test core xanh mà `git status` chỉ có file dưới `catalog/`; xóa squad thử.
- T3.8 `squad-invariants.test.ts` (mới) thay 21 test squad lặp.
- Kiểm tra giai đoạn: băm vàng (28 squad, vai, danh sách tên); `pnpm typecheck`; `pnpm --filter @multica/core test`; `pnpm --filter @multica/views exec vitest run modals company locales`; `node scripts/catalog/cli.mjs check` (đủ 7 ngôn ngữ).

## 7. Giai đoạn 4: Go, công ty mẫu, workflow

- T4.1 `catalog/companies/starter/template.json` từ `company-template.ts` (codemod); `company-template.ts` giữ logic và đọc dữ liệu sinh ra. Mẫu hướng dẫn agent (39) → `catalog/agents/<id>/` bằng codemod, khối dùng chung (`WORKING_RULES`, `SHARED_BLOCKS`) thành `{{block:...}}` do builder mở rộng.
- T4.2 Gói Go `server/internal/catalog/` (mới): `go:embed all:data`, loader có kiểu, `contracts.go`, test phân tích. Builder xuất `server/internal/catalog/generated/{company,workflows,starter-skills,quickstart-bundle}.json`. `service/company_template_data.go` và `quickstart/bundle.go` đọc từ đó; xóa `company_template_data.json`, `quickstart/bundle.json`, `scripts/generate-company-template.mjs`, mục `generate:company-template`.
- T4.3a Đo độ lệch ba bản SDLC Auto (đã làm ở bước rà soát, kết quả ở spec 5.4; giữ thành test khớp trong CLI: L2).
- T4.3b Manifest `catalog/workflows/sdlc_auto` (nguồn cho TS `SDLC_AUTO_WORKFLOW` và Go `sdlcSteps`, `implicitNext`), `planning_auto` (TS `PLANNING_AUTO_WORKFLOW`), và 7 workflow của trình thiết kế (Go `issueworkflow.Templates()`, bản dịch từ `workflow.json`).
- T4.4 Hạ tầng Go (Q2): `Makefile` (`catalog-build` là điều kiện của các target `go build`/`go run`/`test`), `scripts/dev.sh`, `scripts/dev-env.sh`, `scripts/check.sh`, `scripts/test-go.sh`, `Dockerfile` (stage `catalog`), `.goreleaser.yml` (`before.hooks`), `.github/workflows/ci.yml` (setup-node + build trước Go) và `release.yml`.
- T4.5 Hợp đồng: `contracts` cho squad SDLC, `bmad_planning`, workflow `bmad_planning_auto`, thẻ `executive_office`; test Go `TestRequiredContractsProvided`.
- Kiểm tra giai đoạn: băm vàng (Go và TS); `node scripts/catalog/cli.mjs build --target go`; `cd server && go build ./... && go vet ./internal/catalog/... && go test ./internal/catalog/ ./internal/issueworkflow/ ./internal/quickstart/`; test service/handler liên quan (cần `make up` cho nhóm DB); `docker build --target catalog` (nếu daemon và mạng cho phép).

## 8. Giai đoạn 5: agent pack không còn danh sách viết tay

`catalog/packs/<id>/template.json` (+ `i18n/`), `AGENT_PACK_IDS` và bảng bộ nạp sinh ra; xóa khóa dịch `market-strategist` khỏi 7 `modals.json`. Kiểm tra: băm vàng bundle; `pnpm --filter @multica/core exec vitest run agent-packs`; `pnpm --filter @multica/views exec vitest run modals/agent-pack-install`.

## 9. Giai đoạn 6: tài liệu và dọn

- `AGENTS.md`: hàng `catalog/` trong bảng ranh giới, quy tắc "mã sinh ra không commit; chạy `pnpm catalog:build`", lệnh kiểm tra.
- `catalog/README.md`; cập nhật `docs/engineering/kitviet-runbook.md`, `sdlc-harness.md` và các tài liệu có đường dẫn cũ; thêm tham chiếu tới spec.
- Xóa `scripts/catalog/golden.mjs`, `golden-hashes.json`, `scripts/catalog/codemods/` sau khi mọi băm đã khớp lần cuối.
- Chạy các lệnh bắt buộc của `AGENTS.md`: `pnpm typecheck`, `pnpm lint`, `pnpm test`, `make test`, `make check`; `git diff --check`.

## 10. Rủi ro và điểm hoàn tác

| Rủi ro | Giảm thiểu | Hoàn tác |
| --- | --- | --- |
| Lệch hành vi vì chữ đổi nhẹ | Băm vàng mỗi giai đoạn; codemod | Mỗi giai đoạn là một tập thay đổi riêng; người dùng có thể bỏ tập đó khi commit |
| Điểm vào build bỏ sót bước sinh (Q2) | Liệt kê ở spec 6.7, kiểm tra từng điểm vào | Thêm bước sinh vào điểm đó |
| Go không biên dịch khi chưa sinh dữ liệu | `go:embed all:data` + `.gitkeep` | |
| Mã sinh ra bị xóa khỏi cây làm việc nhưng chưa commit xóa | Báo rõ trong báo cáo | Người dùng commit xóa cùng thay đổi |

Vì không commit, điểm hoàn tác thực tế là cây git của người dùng: sau mỗi giai đoạn, trạng thái `git status` mô tả đủ thay đổi.

## 11. Ngoài phạm vi

Kho template lúc runtime (Hub/DB), chợ, template riêng theo workspace, giao diện soạn template, đổi nội dung prompt/skill/charter, migration DB, skill nền tảng của server, agent Mika, `.agents/skills/`, `examples/plugins/`, `apps/docs/`, dịch nội dung dành cho agent, mobile, nâng upstream Kit Việt.

## 12. Trạng thái triển khai

| Task | Trạng thái | Ghi chú |
| --- | --- | --- |
| T0.1 Băm vàng | Xong, rồi xóa | `scripts/catalog/golden.mjs`, 205 băm TS và Go; xanh ở lần kiểm cuối; công cụ và các codemod một lần đã bị xóa khỏi cây làm việc |
| T0.2 Ghi nền | Xong | Ghi nhận: `sdlc-skills-data.ts` đang commit ở HEAD đã cũ so với thư mục skill (thiếu 6 skill `research-lab-*`), 7 test vitest đỏ ở HEAD vì thế; sinh lại trước khi lấy băm |
| T0.3 Đo bundle | Xong (đo, không chia gói) | Cổng `@multica/core/squads` kéo tĩnh khoảng 2,1 MB `sdlc-skills-data` và 2,9 MB `kitviet-previous-texts` (esbuild minify, chưa tree-shake) |
| T1.1 đến T1.6 Di chuyển nguồn | Xong | `mv` (không dùng `git mv`); đầu ra giữ nguyên băm |
| T2.1 CLI | Xong | Không dùng zod; kiểm tra thủ công để chạy trong image Node trần |
| T2.2 Manifest skill | Xong | 93 skill + 6 skillset |
| T2.3 Builder skill | Xong | Thay `sdlc-skills-data.ts` |
| T2.4 Kit Việt, pack trong build | Xong | |
| T2.5 Test thay thế | Xong | |
| T2.6 Hạ tầng build (Q2) | Xong, trừ ghi chú ở báo cáo | Docker: build stage `catalog` và `builder` của `Dockerfile`; chỉ chạy bước sinh của `Dockerfile.web` trong container Node, chưa build trọn image |
| T3.1 đến T3.6, T3.8 | Xong | 28 squad, 3 collection, 7 ngôn ngữ; 21 test Kit Việt được giữ lại ở `packages/core/catalog/kitviet/` |
| T3.7 Nghiệm thu thêm không sửa code | Xong | squad thử `zz_probe`, đã xóa |
| T4.1 Công ty mẫu và agent | Xong | 39 mẫu + 5 khối; `company.json` cho Go bằng dữ liệu cũ (trừ `generated_by`) |
| T4.2 Gói Go `catalog` | Xong | `server/internal/catalog`; `company_template_data.json` và `quickstart/bundle.json` đã xóa |
| T4.3a Đo độ lệch SDLC Auto | Xong | Kết quả ở spec 5.4 |
| T4.3b Manifest workflow | Xong | `sdlc_auto`, `planning_auto`; 7 workflow của trình thiết kế ở `catalog/workflows/designer_*`, `issueworkflow.Templates()` đọc từ catalog, JSON bằng bản ở HEAD |
| T4.4 API tên theo locale cho workflow template | Không làm | Chạm mã trình thiết kế do người dùng đang phát triển (`schemas.ts`, `issue-workflow.ts`, `template-picker-dialog.tsx`, `workflow.json` ×7); bản dịch giữ ở `workflow.json` |
| T4.4 (hạ tầng Go) | Xong | Makefile, scripts, Dockerfile, goreleaser, ci.yml |
| T4.5 Hợp đồng | Xong | `contracts` trên 5 template + công ty mẫu; `server/internal/catalog/contracts.go`, `service/catalog_contracts_test.go`, `packages/core/catalog/contracts.ts`; yêu cầu "ít nhất một" provider `stable` (spec sửa lại, không phải "đúng một") |
| Hướng dẫn theo vai | Xong | 37 file `catalog/squads/*/roles/*.md`; các file `*-agent-instructions.ts`, `content-studio.ts`, `research-lab.ts` bọc dữ liệu sinh ra; bằng bản cũ từng khóa |
| C4 Tải lazy | Xong (hai gói lớn nhất) | `SDLC_BUILTIN_SKILLS` và `KITVIET_PREVIOUS_*` nạp bằng `import()`; `SKILLSET_MEMBERS` tách file; phần nạp tĩnh của `upgrade.ts` từ khoảng 4,0 MB còn khoảng 1,2 MB |
| Q9 Nguồn và giấy phép skill vendor | Xong | 51 skill; còn 3 mục cần người sở hữu quyết định (spec mục 11) |
| Giai đoạn 5 Agent pack | Xong | `AGENT_PACK_IDS` sinh ra; UI vẫn dùng pack đầu tiên |
| Giai đoạn 6 (tùy chọn, băm lịch sử) | Không làm | Q6: băm làm mất các kiểm tra cần văn bản gốc (`upgrade.test.ts`, `kitviet-previous-texts.test.ts`) và phá luồng ghi thêm của `scripts/kitviet-update.mjs`; phần kích thước đã giải quyết bằng tải lazy |
| T7 Tài liệu, dọn | Xong | `AGENTS.md`, `catalog/README.md`, runbook |
