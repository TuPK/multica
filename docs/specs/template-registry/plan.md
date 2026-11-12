# Template Registry: kế hoạch triển khai

Đặc tả: [spec.md](spec.md). Trạng thái: chưa bắt đầu.
Quy ước đánh dấu giống spec: đường dẫn kèm chữ "(mới)" là đường dẫn sẽ được tạo; các đường dẫn còn lại đã được kiểm tra tồn tại.

Kích thước ước lượng cho một lập trình viên quen repo: S ≤ 0,5 ngày, M 0,5 đến 1,5 ngày, L 1,5 đến 3 ngày. Tổng ước lượng khoảng 22 đến 25 ngày làm việc, trong đó giai đoạn 0 đến 3 (khoảng 14,5 ngày) đạt hai mục tiêu người dùng đã nêu.

## 1. Nguyên tắc thực hiện

1. Mỗi giai đoạn là một hoặc vài PR nhỏ, theo commit kiểu `refactor(templates): ...` (đúng yêu cầu "atomic conventional commits" trong `AGENTS.md`). PR theo mẫu `.github/PULL_REQUEST_TEMPLATE.md` (Problem, Goal, Proposal, Alternatives). Giai đoạn 3 có thay đổi giao diện danh sách nên đính kèm ảnh chụp bộ chọn squad trước và sau (kỳ vọng giống hệt).
2. Không đổi hành vi người dùng thấy. Cổng bắt buộc của mọi bước di chuyển: băm nội dung đầu ra bằng `templates/golden-hashes.json` (mới, tạm) phải bằng nhau trước và sau (lớp L6 của spec).
3. Dùng `git mv` và codemod một lần, không gõ lại nội dung. Mọi codemod được commit riêng trước khi xóa mã cũ, và kết quả được so băm.
4. Không thêm lớp tương thích nội bộ. Mỗi giai đoạn chuyển hẳn sang đường mới và xóa đường cũ trong cùng PR.
5. Không có migration DB. Id, tên skill, tên agent, tên workflow giữ nguyên.
6. Chạy kiểm tra hẹp khi làm, kiểm tra rộng ở cuối PR, và ghi đúng lệnh đã chạy vào mô tả PR.

## 2. Mốc giao hàng

| Mốc | Sau giai đoạn | Người dùng nhận được |
| --- | --- | --- |
| M1 | 1 | Mọi nội dung nguồn ở `templates/`. Chưa đổi cách thêm template. Đạt yêu cầu số 1 |
| M2 | 3 | Thêm skill, skillset, squad, collection không cần sửa file code (phía TypeScript). Đạt yêu cầu số 2 |
| M3 | 4 | Một nguồn cho Go và TypeScript (công ty mẫu, workflow), bỏ chuỗi esbuild cho Go |
| M4 | 7 | Pack, bundle, tải lazy, tài liệu và dọn dẹp xong |

Có thể dừng sau M1 hoặc M2 mà không để lại trạng thái dở dang: mỗi mốc cuối giai đoạn đều xanh toàn bộ kiểm tra.

## 3. Giai đoạn 0: điều kiện tiên quyết và đo nền (khoảng 1,5 ngày)

Không đổi mã nguồn sản phẩm.

### T0.1 Chốt câu hỏi mở (S)

- Việc: người dùng trả lời Q1 đến Q10 trong mục 10 của spec; cập nhật spec theo câu trả lời. Q1 (tên thư mục), Q2 (commit đầu ra), Q3 (dịch), Q4 (phạm vi giai đoạn 4) cần chốt trước giai đoạn 1.
- Kiểm tra: spec không còn câu hỏi mở chặn giai đoạn kế tiếp.

### T0.2 Đưa thay đổi đang làm vào commit (S, do người dùng)

- Vì sao: `git status` có khoảng 100 mục chưa commit trong chính các thư mục sẽ di chuyển (`packages/core/squads/sdlc-skills/**`, `sdlc-skills-data.ts`, `sdlc-harness-*.test.ts`, `packages/core/issue-workflows/**`, `packages/views/workflow/**`, `server/internal/issueworkflow/**`). Di chuyển lúc cây bẩn gây xung đột và dễ mất thay đổi.
- Việc: người dùng commit hoặc stash thay đổi của mình, rồi tạo nhánh làm việc từ commit đó.
- Kiểm tra: `git status --short -- packages/core/squads packages/core/issue-workflows packages/views/workflow server/internal/issueworkflow server/internal/service` không in gì (ngoại trừ file người dùng chủ ý để lại).
- Hoàn tác: không áp dụng.

### T0.3 Ghi nền các kiểm tra (S)

- Chạy và ghi kết quả vào mô tả PR đầu tiên:
  - `node scripts/generate-kitviet-pack.mjs --check` (đã xanh ở lần khảo sát: "up to date (29 files)")
  - `node --test scripts/generate-kitviet-pack.test.mjs scripts/ci-scope.test.mjs`
  - `pnpm --filter @multica/core exec vitest run squads`
  - `pnpm --filter @multica/views exec vitest run modals locales company`
  - `cd server && go test ./internal/issueworkflow/ ./internal/quickstart/`
  - `make test` (cần `make up`; tốn thời gian, chạy một lần để biết nền)
- Kiểm tra: ghi lại kết quả đỏ có sẵn (nếu có) để không bị tính vào thay đổi của kế hoạch này.

### T0.4 Băm vàng (golden) của đầu ra hiện tại (M)

- Tạo `scripts/templates/golden.mjs` (mới, tạm; xóa ở T7.3). Dùng cách bundle bằng esbuild như `scripts/generate-company-template.mjs` để nạp TS, rồi băm sha256 từng mục:
  - từng phần tử của `SQUAD_TEMPLATES` cùng `SQUAD_ROLES` mà nó tham chiếu (`packages/core/squads/templates.ts`);
  - `SDLC_AUTO_WORKFLOW`, `PLANNING_AUTO_WORKFLOW`;
  - `SDLC_SKILL_NAMES`, `PLANNING_SKILL_NAMES`, `SALES_SKILL_NAMES` (danh sách tên);
  - từng phần tử của `SDLC_BUILTIN_SKILLS`;
  - từng pack Kit Việt qua `KITVIET_PACK_LOADERS`;
  - `COMPANY_*` và `AGENT_INSTRUCTION_TEMPLATES`;
  - các bundle trong `packages/core/agent-packs/bundles/`.
- Phía Go: một test có build tag `templategolden` (mới, tạm, cạnh `server/internal/issueworkflow/templates_test.go`) in JSON của `issueworkflow.Templates()` và `quickstart.LoadBundle()`; script băm đầu ra này.
- Kết quả: `templates/golden-hashes.json` (mới).
- Kiểm tra: chạy hai lần cho cùng kết quả; sửa một ký tự trong một template thì đúng một băm đổi.
- Kích thước: M. Rủi ro: bộ nạp TS cần esbuild; `packages/core` đã có `esbuild` ở thư mục gốc (đang dùng trong `generate-company-template.mjs`).

### T0.5 Đo kích thước bundle hiện tại (S)

- Dùng `esbuild --bundle --metafile` trên một điểm vào chỉ import `@multica/core/squads` (giống `handoff-section.tsx`). Ghi số byte của `sdlc-skills-data.ts` trong bundle.
- Kiểm tra: số liệu nằm trong mô tả PR. Quyết định ngưỡng "meta nhẹ" cho T5.3 dựa vào đây.

## 4. Giai đoạn 1: di chuyển nguồn, giữ nguyên generator (khoảng 3 ngày)

Mục tiêu M1. Chỉ đổi đường dẫn. Mọi đầu ra sinh ra phải giống hệt, ngoại trừ dòng comment ghi đường dẫn nguồn.

Quy ước kiểm tra chung cho các task dưới đây, gọi là "kiểm tra sinh lại":

```
pnpm generate:<tên>          # sinh lại
git diff -U0 -- <đầu ra> | grep '^[+-]' | grep -v '^[+-][+-]' | grep -v '^[+-]//'    # phải rỗng
```

Dòng thứ hai bảo đảm chỉ có comment đầu file đổi. Cộng thêm băm vàng khớp (`node scripts/templates/golden.mjs --check`).

### T1.1 Dựng khung và hạ tầng (M)

- Tạo `templates/README.md` (mới), `templates/_sources/.gitkeep` (mới).
- `.github/ci-paths.json`: thêm `templates/**` và `scripts/templates/**` vào bộ lọc `frontend`, `backend`, `quality`.
- `scripts/ci-scope.test.mjs`: cập nhật nếu test mô tả các bộ lọc.
- `turbo.json` hoặc `packages/core/turbo.json` (mới, `extends: ["//"]`): `inputs: ["$TURBO_DEFAULT$", "$TURBO_ROOT$/templates/**"]` cho task `test` và `typecheck` của `@multica/core`. Lý do: test đọc script trong thư mục skill (`sdlc-harness-check.test.ts`, `sdlc-harness-metrics.test.ts`) có thể bị phát lại kết quả cũ từ cache.
- `knip.jsonc`: thêm `scripts/templates/*.mjs` vào `entry` và `project` của workspace gốc.
- `.gitattributes`: thêm `packages/core/templates/generated/** linguist-generated=true` và dòng tương ứng cho `server/internal/templatereg/data/**` và `packages/views/locales/*/templates.json` (các đường dẫn mới, thêm trước để khỏi quên).
- Kiểm tra:
  - `node --test scripts/ci-scope.test.mjs`
  - `pnpm knip`
  - Bằng chứng cache: chạy `pnpm exec turbo run test --filter=@multica/core --dry=json`, ghi `hash`; tạo file tạm `templates/probe.txt`; chạy lại, `hash` phải đổi; xóa file tạm. Nếu cú pháp `$TURBO_ROOT$` không được phiên bản turbo của repo chấp nhận, ghi lại và dùng phương án thay thế: chuyển hai test script thành bước `node --test` ngoài turbo (T1.2).
  - `git diff --check`
- Hoàn tác: `git revert` commit này; không có thay đổi hành vi.

### T1.2 Di chuyển skill SDLC (M)

- `git mv packages/core/squads/sdlc-skills templates/skills/builtin`
- Sửa `scripts/generate-sdlc-skills.mjs`: hằng `skillsDir` (đường dẫn nguồn) và chuỗi comment ở đầu đầu ra. Đầu ra vẫn là `packages/core/squads/sdlc-skills-data.ts` ở giai đoạn này.
- Sửa test tham chiếu đường dẫn tương đối: `packages/core/squads/sdlc-skills.test.ts` (dòng 11, `new URL("./sdlc-skills", ...)`), `sdlc-harness-check.test.ts` (dòng 9), `sdlc-harness-metrics.test.ts` (dòng 9). Tính đường dẫn từ gốc repo bằng `new URL("../../../templates/skills/builtin/...", import.meta.url)`.
- Kiểm tra: kiểm tra sinh lại với `pnpm generate:sdlc-skills`; `pnpm --filter @multica/core exec vitest run squads`; `pnpm typecheck`.
- Hoàn tác: `git revert` (git mv được đảo ngược sạch).

### T1.3 Di chuyển skill công ty mẫu (S)

- `git mv packages/core/squads/skills templates/skills/starter`
- Sửa `scripts/generate-company-template.mjs`: hằng `skillsDir` và comment.
- Kiểm tra: `pnpm generate:company-template` rồi `git diff --exit-code server/internal/service/company_template_data.json`; `pnpm --filter @multica/core exec vitest run squads/company-template-data.test.ts`; `cd server && go test ./internal/service -run 'Company|Starter'` (nhóm test DB cần `make up`; ghi nếu bỏ qua).
- Hoàn tác: `git revert`.

### T1.4 Di chuyển nguồn Kit Việt (M)

- `git mv` sang `templates/_sources/kitviet/`:
  - `packages/core/squads/kitviet-upstream` → `templates/_sources/kitviet/upstream`
  - `packages/core/squads/kitviet-patches` → `.../patches`
  - `packages/core/squads/kitviet-rewrites.json` → `.../rewrites.json`
  - `packages/core/squads/kitviet-squads` → `.../squads`
  - `packages/core/squads/kitviet-extensions` → `.../extensions`
  - `packages/core/squads/kitviet-skills` → `.../skills`
- Sửa `scripts/generate-kitviet-pack.mjs` (các hằng `squadsDir`, `upstreamDir`, `patchesDir`, `authoredDir`, và các chuỗi trong thông báo lỗi) và `scripts/generate-kitviet-pack.test.mjs` (hằng `squadsDir` và các đường dẫn fixture). Hàm `loadConfig(root)` đã nhận `root`, nên tách `configRoot` khỏi thư mục đầu ra là thay đổi nhỏ.
- Đầu ra `packages/core/squads/kitviet-skills-data/` giữ nguyên chỗ.
- Sửa `packages/core/squads/kitviet-skills.test.ts` và `kitviet-packs.test.ts` nếu chúng đọc cấu hình bằng đường dẫn.
- Sửa `docs/engineering/kitviet-runbook.md` mục 1, 2 và `kitviet-squads.md` theo đường dẫn mới.
- Kiểm tra: `node scripts/generate-kitviet-pack.mjs --check` (không lệch), `node --test scripts/generate-kitviet-pack.test.mjs`, `pnpm --filter @multica/core exec vitest run squads/kitviet`.
- Hoàn tác: `git revert`.

### T1.5 Di chuyển agent pack (S)

- `git mv examples/agent-packs templates/packs`
- Sửa `scripts/build-agent-pack-bundles.mjs` (hằng `packsDir`), `scripts/install-agent-pack.sh` (chỉ chuỗi trợ giúp nếu có), `Makefile` (dòng 328-331, `examples/agent-packs` → `templates/packs`).
- `examples/plugins/` giữ nguyên (fixture test Go; Q10).
- Kiểm tra: `make agent-pack-bundles` rồi `git diff --exit-code packages/core/agent-packs/bundles`; `bash scripts/install-agent-pack.sh --help`; `pnpm --filter @multica/core exec vitest run agent-packs`.
- Hoàn tác: `git revert`.

### T1.6 Cập nhật đường dẫn trong tài liệu (S)

- Cập nhật mọi tham chiếu trong `docs/engineering/*.md` tới các đường dẫn đã di chuyển (tìm bằng `git grep -n "packages/core/squads/\(sdlc-skills\|skills\|kitviet-\)\|examples/agent-packs" -- docs`).
- Kiểm tra: kiểm tra tồn tại đường dẫn cho mọi đường dẫn có dấu backtick trong các file đã sửa; `git diff --check`.

Kết thúc giai đoạn 1 (M1): toàn bộ `pnpm typecheck`, `pnpm lint`, `pnpm test`, `pnpm knip`, `node --test scripts/*.test.mjs` xanh.

## 5. Giai đoạn 2: lõi registry và skill (khoảng 4 ngày)

Giá trị: thêm skill không cần sửa code; một cơ chế kiểm tra độ tươi.

### T2.1 Schema zod và phong bì (M)

- Tạo `packages/core/templates/schema/` (mới): `envelope.ts`, `skill.ts`, `skillset.ts`, `collection.ts`, kèm test. Theo spec mục 6.3, 6.4. Dùng zod đang có (`^4.1.5` trong `pnpm-workspace.yaml`); `z.toJSONSchema` để sinh `templates/schema/*.schema.json`.
- `packages/core/package.json`: thêm mục `exports` cho `./templates`.
- Kiểm tra: `pnpm --filter @multica/core exec vitest run templates`; `pnpm typecheck`.
- Rủi ro: `packages/core` không được thêm thư viện UI hay `localStorage` (AGENTS.md); chỉ zod và kiểu thuần.

### T2.2 Khung CLI (L)

- Tạo `scripts/templates/` (mới): `cli.mjs`, `lib/discover.mjs`, `lib/load.mjs`, `lib/validate.mjs`, `lib/hash.mjs`, `lib/lock.mjs`, `lib/scaffold.mjs`, kèm `*.test.mjs`. Lệnh: `list`, `check`, `build`, `new`.
- Dùng lại `scripts/lib/skill-bundle.mjs` (`skillFiles`, `frontmatterDescription`); `skillFiles` phải loại `template.json` và `i18n/` (spec 6.1).
- `package.json` (gốc): thêm `templates:check`, `templates:build`, `templates:new`, `templates:list`.
- `.github/workflows/ci.yml`: thêm `scripts/templates/*.test.mjs` vào dòng `node --test` ở khoảng dòng 44; thêm bước "Verify templates are up to date" chạy `pnpm templates:check` (cạnh bước reserved-slugs, khoảng dòng 919-926).
- Kiểm tra: `node --test scripts/templates/*.test.mjs`; test có fixture thư mục sai (id sai regex, thiếu `en`, vòng `requires`, tham chiếu thiếu) cho mỗi lớp L1 đến L5.
- Rủi ro: kích thước; giữ CLI tối thiểu, không thêm lệnh ngoài danh sách.

### T2.3 Codemod tạo `template.json` cho 87 skill (M)

- Một lần chạy `scripts/templates/codemods/skills.mjs` (mới, xóa ở T7.3). Với mỗi thư mục trong `templates/skills/builtin` và `templates/skills/starter`, ghi `template.json` với `sets` suy ra từ quy tắc hiện hành:
  - tập `planning`: đúng danh sách `PLANNING_SKILL_NAMES` (`packages/core/squads/templates.ts:656`);
  - tập `sales`: `SALES_SKILL_NAMES` (`:692`);
  - tập `sdlc`: các skill không thuộc `NON_SDLC_SKILLS` (quy tắc ở `templates.ts` ở `:703-713`);
  - skill của `starter`: tập `starter-company`; các skill bí thư (`secretary-*`) thêm tập `secretary` để thay cho danh sách `SecretarySkills` trong Go (dùng ở giai đoạn 4).
- `source` cho skill vendor: để `type: "vendored"` với `repo`, `ref`, `license` ghi `"TODO"`. Check L3 báo cảnh báo cho "TODO" trong giai đoạn 2 và chặn từ giai đoạn 7 (Q9).
- Kiểm tra: ba danh sách tên suy ra từ `sets` bằng đúng ba danh sách cũ (so băm với `golden-hashes.json`); mọi skill có ít nhất một tập; `node scripts/templates/cli.mjs check`.
- Hoàn tác: xóa `template.json`.

### T2.4 Skillset và collection (S)

- Tạo `templates/skillsets/{sdlc,planning,sales,starter-company,secretary}/` (mới, mỗi cái có `template.json` và `i18n/en.json`). Collection chưa dùng ở giai đoạn này.
- Kiểm tra: `pnpm templates:check`.

### T2.5 Builder `skills` và thay `sdlc-skills-data.ts` (L)

- Builder đọc `templates/skills/**`, ghi `packages/core/templates/generated/skills/builtin.ts` (mới) có cùng hình dạng `BuiltinSkill` và `BuiltinSkillFile` như `packages/core/squads/sdlc-skills-data.ts`, cộng với bảng tên theo tập (`SDLC_SKILL_NAMES`...).
- Sửa chỗ dùng: `packages/core/squads/templates.ts` (bỏ `NON_SDLC_SKILLS`, `SDLC_SKILL_NAMES`; `PLANNING_SKILL_NAMES` và `SALES_SKILL_NAMES` lấy từ đầu ra sinh), `quickstart.ts` (import `SDLC_BUILTIN_SKILLS`), `kitviet-packs.ts` (kiểu), `kitviet-skills-data/*.ts` (đầu ra sinh của generator Kit Việt import kiểu từ `../sdlc-skills-data`; sửa chuỗi `HEADER` và import trong `scripts/generate-kitviet-pack.mjs` ở khoảng dòng 953-955 để trỏ tới đầu ra mới, rồi sinh lại).
- Xóa `scripts/generate-sdlc-skills.mjs`, mục `generate:sdlc-skills` trong `package.json`, `packages/core/squads/sdlc-skills-data.ts`.
- Kiểm tra: băm vàng khớp (danh sách tên và từng skill); `node scripts/generate-kitviet-pack.mjs --check`; `pnpm typecheck`; `pnpm --filter @multica/core exec vitest run squads`.
- Rủi ro: giữ nguyên độ chi tiết (một tệp `builtin.ts`), chưa tách chunk, để băm bằng nhau tuyệt đối. Tách chunk thuộc T5.3.
- Hoàn tác: `git revert` khôi phục generator và tệp cũ.

### T2.6 Thay test liệt kê tên (M)

- `packages/core/squads/sdlc-skills.test.ts`: bỏ `ROLE_SKILLS`, `PLANNING_ROLE_SKILLS` viết tay; kiểm tra "bộ gói bằng đúng các thư mục" thành kiểm tra registry. Giữ các khẳng định về nội dung skill (đó là nội dung, không phải danh sách).
- `packages/core/squads/templates.test.ts:91` (`planning.length toBe(8)`) và các số đếm cứng liên quan: thay bằng so với registry hoặc băm vàng.
- Kiểm tra: `pnpm --filter @multica/core exec vitest run squads`.

Kết thúc giai đoạn 2: thêm một skill thử vào `templates/skills/` với `sets: ["sdlc"]`, chạy `pnpm templates:build`, `pnpm typecheck`, `pnpm --filter @multica/core test` đều xanh mà không sửa file nào khác; sau đó xóa skill thử.

## 6. Giai đoạn 3: squad, collection, dịch, tải lazy (khoảng 6 ngày)

Mục tiêu M2.

### T3.1 Schema squad và collection đầy đủ (M)

- `packages/core/templates/schema/squad.ts` (mới) theo spec 6.4, khớp `SquadTemplate` (`templates.ts:599`), gồm `roles`, `routines`, `charter`, `skills`, `workflow`, `adapter`. Test: dựng lại một `SquadTemplate` từ manifest và so sâu.
- Kiểm tra: `pnpm --filter @multica/core exec vitest run templates`.

### T3.2 Codemod 5 squad nền (M)

- `scripts/templates/codemods/squads-base.mjs` (mới): đọc `BASE_SQUAD_TEMPLATES` (nạp TS bằng esbuild như T0.4), ghi `templates/squads/{sdlc_greenfield,sdlc_maintenance,market_research,bmad_planning,sales_loop}/` với `template.json`, `instructions.md` (văn bản `SDLC_SQUAD_INSTRUCTIONS`... tách ra file), `i18n/<locale>.json` (lấy từ `squad_quickstart.templates.<id>` đang có ở 7 `modals.json`).
- Hướng dẫn agent theo vai (`sdlc-agent-instructions.ts`, `planning-agent-instructions.ts`, `sales-agent-instructions.ts`) vào `templates/agents/<id>/instructions.md` hoặc `templates/squads/<id>/roles/<role>.md`.
- Kiểm tra: `SquadTemplate` dựng từ manifest bằng băm với bản cũ (băm vàng, mục squad).

### T3.3 Codemod 21 squad Kit Việt (L)

- Ghép `templates/_sources/kitviet/squads/<trade>.json` (cấu hình bộ biến đổi) với `packages/core/squads/kitviet-templates/<trade>.ts` (charter, routine, mô tả, role key) thành `templates/squads/kv_<trade>/template.json` với khối `adapter` (spec 6.4).
- Sửa `scripts/generate-kitviet-pack.mjs`: `readConfigFolder` đọc từ các manifest squad có `adapter.type === "kitviet"` thay vì thư mục `squads/`. Giữ nguyên toàn bộ logic biến đổi (1.061 dòng). Cập nhật `scripts/generate-kitviet-pack.test.mjs` cho cấu trúc đọc mới.
- Kiểm tra: `node scripts/generate-kitviet-pack.mjs --check` (29 tệp không lệch); băm vàng khớp cho 21 squad và các pack; `node --test scripts/generate-kitviet-pack.test.mjs`.
- Rủi ro cao nhất của giai đoạn: cùng một dữ liệu đi qua hai đường (cấu hình generator và `SquadTemplate`). Chạy băm sau mỗi squad, commit từng nhóm 5 squad.

### T3.4 Chỉ mục sinh ra, kiểu id, và thu gọn `templates.ts` (L)

- Builder `squads` ghi `packages/core/templates/generated/index.ts` (kiểu `SquadTemplateId`, `SquadRoleKey`, mảng meta, bộ nạp lazy) và `generated/chunks/squads/*.ts`.
- `packages/core/squads/templates.ts`: giữ kiểu giao diện (`SquadTemplate`, `SquadRole`, `SquadWorkflowStep`...) và hàm chạy; xóa 21 import, hai union, `BASE_SQUAD_TEMPLATES`, `SQUAD_TEMPLATES`, `inCategory`, `SquadTemplateCategory`, `SQUAD_TEMPLATE_CATEGORIES`.
- Xóa `packages/core/squads/kitviet-templates/` (21 `.ts`, 21 test). `kitviet-squad-roles.ts` và `kitviet-floors.ts` đọc dữ liệu từ registry.
- Thêm `packages/core/templates/squad-invariants.test.ts` (mới) mang các khẳng định lặp trong 21 `kitviet-templates/*.test.ts` và `templates.test.ts` thành test duyệt theo registry (spec mục 7).
- Thêm quy tắc ESLint `no-restricted-imports` cho `@multica/core/templates/generated/chunks/*`.
- Kiểm tra: `pnpm typecheck`, `pnpm lint`, `pnpm --filter @multica/core test`; băm vàng.

### T3.5 Dịch sinh ra (M)

- Builder ghi `packages/views/locales/<lang>/templates.json` (mới, 7 file) từ `i18n/` của template. Đăng ký namespace `templates` trong `packages/views/locales/index.ts`.
- Chuyển `squad_quickstart.templates` và `squad_quickstart.groups` ra khỏi `modals.json` ở 7 ngôn ngữ; `packages/views/modals/squad-quickstart.tsx` đổi sang `useT("templates")`; cập nhật `squad-quickstart.test.tsx`, `company-starter-wizard.test.tsx` nếu chúng tham chiếu khóa cũ.
- Kiểm tra: `pnpm --filter @multica/views exec vitest run locales modals company`; `parity.test.ts` xanh.
- Rủi ro: khóa gõ kiểu. Union id sinh ra giữ chúng được gõ kiểu; xác nhận bằng `pnpm typecheck`.

### T3.6 Tải nội dung lazy trong giao diện và quickstart (M)

- Danh sách trong `squad-quickstart.tsx`, `company-setup-card.tsx`, `company-starter-wizard.tsx` dùng meta đồng bộ; trước khi gọi `runSquadQuickstart` thì `await loadTemplate("squad", id)`.
- Kiểm tra: `pnpm --filter @multica/views exec vitest run modals company`; chạy Playwright bộ quickstart nếu có (`pnpm exec playwright test e2e/company-first-loop.spec.ts`; cần môi trường `make up`, ghi nếu bỏ qua).
- Hoàn tác: `git revert` PR T3.6 riêng.

### T3.7 Thử nghiệm nghiệm thu "thêm không cần sửa code" (S)

- Tạo squad thử `zz_probe` (collection `core`, vai tối thiểu) qua `pnpm templates:new squad zz_probe --collection core`, điền nội dung, `pnpm templates:build`.
- Kiểm tra (tiêu chí nghiệm thu mục tiêu G2): `git status` chỉ có file dưới `templates/`, `packages/core/templates/generated/`, `packages/views/locales/*/templates.json`; `pnpm typecheck` và `pnpm --filter @multica/core test` xanh; bộ chọn quickstart hiển thị squad thử. Xóa squad thử và build lại, băm vàng về trạng thái cũ.
- Kết quả đưa vào mô tả PR như bằng chứng cho M2.

## 7. Giai đoạn 4: phía Go, công ty mẫu, workflow (khoảng 5 ngày)

Mục tiêu M3. Chạm vào vùng người dùng đang sửa (workflow, initiative). Chỉ bắt đầu khi T0.2 đã xong và các thay đổi workflow đã vào nhánh chính.

### T4.1 Công ty mẫu và mẫu hướng dẫn agent thành JSON (L)

- Codemod ghi `templates/companies/starter/template.json` từ `packages/core/squads/company-template.ts` (`COMPANY_FLOOR_KEYS`, `COMPANY_FLOOR_NAMES`, `COMPANY_SQUAD_CARDS`, `COMPANY_STAGE_OWNERS`, `COMPANY_START_OPTIONS`) và `templates/agents/<id>/` từ `packages/core/agents/instruction-templates.ts` (39 mẫu) cùng `instruction-blocks.ts`.
- `company-template.ts` giữ phần logic (`estimateCompanyRuns`, schema zod) và đọc dữ liệu từ registry. `instruction-templates.ts` còn phần ghép Kit Việt (`composeKvAgentInstructions`).
- Kiểm tra: băm vàng; `pnpm --filter @multica/core exec vitest run squads/company-template company`.

### T4.2 Gói Go `templatereg` (M)

- Tạo `server/internal/templatereg/` (mới): `templatereg.go` (`//go:embed data/*.json`, kiểu, loader trả lỗi), `contracts.go`, test `TestEmbeddedRegistryParses`.
- Builder `go` ghi `server/internal/templatereg/data/{company,workflows,starter-skills,quickstart-bundle}.json`.
- `server/internal/service/company_template_data.go` đọc từ `templatereg`; xóa `company_template_data.json` và `scripts/generate-company-template.mjs`, mục `generate:company-template`; `packages/core/squads/company-template-data.test.ts` thay bằng kiểm tra L5.
- Hằng số tên skill trong `server/internal/service/company_starter_skills.go` (dòng 24-47, `SecretarySkills`) đọc từ tập `secretary`/`starter-company` của registry.
- Kiểm tra: `cd server && go test ./internal/templatereg/ ./internal/service/...` (nhóm DB cần `make up`); băm vàng của JSON Go.
- Hoàn tác: `git revert`.

### T4.3a Đo độ lệch của ba bản SDLC Auto (M)

- Test tạm so sánh ba bản: `SDLC_AUTO_WORKFLOW` (`templates.ts:378`), `sdlcSteps` (`server/internal/service/initiative_build.go:325`), `sdlcAutoTemplate` (`server/internal/issueworkflow/templates.go`), đối chiếu theo trạng thái, vai, chuyển tiếp, và văn bản hướng dẫn.
- Kết quả: bản báo cáo các chỗ lệch (nếu có) trong mô tả PR. Người dùng quyết bản chuẩn (Q4) trước T4.3b. Task này không đổi hành vi.

### T4.3b Hợp nhất workflow thành một nguồn (L)

- `templates/workflows/{delivery,content,community,sdlc_auto,bmad_planning_auto,sales_loop,quick_fix}/template.json` theo hình dạng spec 6.4.
- Go: `issueworkflow.Templates()` đọc từ `templatereg`; `sdlcSteps` và `sdlcStageSteps` dựng bước từ manifest (`optionalRole`) thay vì văn bản viết tay; hằng `planningWorkflowName`, `planningLeadAgentName` (`squad_planning_workflow.go:22-23`) lấy từ hợp đồng.
- TS: `SDLC_AUTO_WORKFLOW`, `PLANNING_AUTO_WORKFLOW` đọc từ registry; `workflowSteps` (`quickstart.ts:229`) giữ nguyên.
- Kiểm tra: `cd server && go test ./internal/issueworkflow/ ./internal/service/ -run 'Workflow|Initiative|SDLC'`; `pnpm --filter @multica/core exec vitest run squads issue-workflows`; băm vàng.
- Rủi ro: bước chuyên gia tùy chọn (`sdlcStageSteps`) chỉ có khi squad có vai đó; test với squad thiếu vai.

### T4.4 API tên theo locale cho workflow template (M)

- Phản hồi danh sách workflow template thêm `names`/`descriptions` theo locale, trường tùy chọn có mặc định. Cập nhật `packages/core/api/schemas.ts` (`ListIssueWorkflowTemplatesResponseSchema`, `EMPTY_LIST_ISSUE_WORKFLOW_TEMPLATES_RESPONSE`, bỏ tập lọc `ISSUE_WORKFLOW_TEMPLATE_IDS`), `packages/core/types/issue-workflow.ts` (kiểu id thành `string`), `packages/core/issue-workflows/schemas.test.ts` (test phản hồi sai định dạng và id lạ), `packages/views/workflow/designer/template-picker-dialog.tsx` (bỏ danh sách 7 id viết tay, hiển thị theo tên từ phản hồi), `packages/views/locales/*/workflow.json` (bỏ khóa `templates`). Kết quả: workflow template mới ở máy chủ hiện trên client cũ mà không cần cập nhật client (id lạ có tên đi kèm thay vì bị lọc bỏ). Các file trình thiết kế hiện là công việc chưa commit của người dùng nên task này chờ chúng vào nhánh chính.
- Kiểm tra: `pnpm --filter @multica/core exec vitest run issue-workflows`; `pnpm --filter @multica/views exec vitest run workflow`; `cd server && go test ./internal/handler -run IssueWorkflow` (DB).
- Hoàn tác: `git revert`.

### T4.5 Hợp đồng (S)

- Điền `contracts` cho squad SDLC, `bmad_planning`, workflow `bmad_planning_auto`, thẻ `executive_office` và các chủ stage dùng ở `server/internal/service/pipeline_install.go:97-121`. Test Go `TestRequiredContractsProvided` và bản TS ở `packages/core/templates/contracts.ts` (mới).
- Kiểm tra: xóa tạm một template có hợp đồng thì test đỏ; hoàn lại thì xanh.

## 8. Giai đoạn 5: pack, bundle, chia gói (khoảng 2 ngày)

### T5.1 Agent pack không còn danh sách viết tay (M)

- Builder `pack` sinh `packages/core/agent-packs/bundles/<id>.json` (thay `scripts/build-agent-pack-bundles.mjs`) và chỉ mục `AGENT_PACK_IDS` + bảng bộ nạp. `packages/core/agent-packs/load.ts:3-9` đọc từ chỉ mục sinh ra.
- `packages/views/modals/agent-pack-install.tsx:32` hiện lấy `AGENT_PACK_IDS[0]`. Giữ hành vi một pack (không thêm giao diện chọn) trừ khi có quyết định khác; ghi nhận trong PR.
- Khóa dịch `market-strategist` (`modals.json`) chuyển vào `i18n/` của pack.
- Kiểm tra: `pnpm --filter @multica/core exec vitest run agent-packs`; `pnpm --filter @multica/views exec vitest run modals/agent-pack-install`; bundle JSON bằng băm cũ; `make agent-pack-bundles` đổi thành `pnpm templates:build` (cập nhật `Makefile`).

### T5.2 Bundle quickstart của Go (S)

- `templates/bundles/quickstart/template.json` thay `server/internal/quickstart/bundle.json`; `quickstart.LoadBundle` đọc từ `templatereg`.
- Kiểm tra: `cd server && go test ./internal/quickstart/`; băm vàng của bundle.

### T5.3 Chia gói và đo (M)

- Dựa số đo ở T0.5: chia chunk theo tập skill (một chunk mỗi tập, skill thuộc nhiều tập được tham chiếu từ một chunk "gốc"). Mục tiêu: import `@multica/core/squads` chỉ kéo meta nhỏ, nội dung vào bundle khi cần.
- Kiểm tra: đo lại bằng `esbuild --metafile`; kích thước điểm vào giảm theo ngưỡng đã ghi ở T0.5; `pnpm test`, quickstart vẫn chạy (T3.6).
- Hoàn tác: `git revert`; không đổi nội dung.

## 9. Giai đoạn 6 (tùy chọn): băm thay lịch sử văn bản (khoảng 2 ngày)

Chỉ làm nếu Q6 chấp thuận. Thay `kitviet-previous-texts.ts` (2,6 MB) và `sdlc-previous-texts.ts` (35 KB) bằng danh sách sha256.

- T6.1 Cài sha256 thuần JS trong `packages/core/templates/hash.ts` (mới) vì `crypto.subtle` thiếu trên HTTP không an toàn.
- T6.2 Codemod tính băm của từng văn bản cũ, ghi `templates/squads/<id>/history.json` (mới). `upgrade.ts` so sánh băm của văn bản hiện có với danh sách.
- T6.3 Xóa hai tệp `*-previous-texts.ts` và test tương ứng (`kitviet-previous-texts.test.ts`) sau khi mọi quyết định của `upgrade.test.ts` giữ nguyên.
- Kiểm tra: `pnpm --filter @multica/core exec vitest run squads/upgrade squads/kitviet-previous-texts`; một test đối chiếu "quyết định nâng cấp cho mọi văn bản cũ" trước và sau giống nhau.
- Hoàn tác: `git revert`. Không đụng dữ liệu người dùng, vì chỉ đổi cách nhận biết văn bản chưa bị sửa.

## 10. Giai đoạn 7: tài liệu và dọn dẹp (khoảng 1 ngày)

### T7.1 `AGENTS.md` và tài liệu (S)

- `AGENTS.md`: thêm hàng `templates/` vào bảng "Sharing Rules and Package Boundaries" (nội dung nguồn, không code, sinh bằng `pnpm templates:build`), một câu trong mục Development and Verification về `pnpm templates:check`, và cập nhật mục Change and Delivery nếu cần.
- `templates/README.md`: cách thêm/sửa/gỡ/đổi tên (một trang, lệnh cụ thể).
- Cập nhật `docs/engineering/kitviet-runbook.md` (mục 1, 2, 8e), `sdlc-harness.md` (bảng đường dẫn), `skill-plugin-hub.md` (nếu có tham chiếu), và thêm liên kết tới spec này.

### T7.2 CI (S)

- Bước CI `pnpm templates:check` đã thêm ở T2.2; thêm kiểm tra bất biến id (L7) so với nhánh chính nếu hạ tầng cho phép lấy `origin/main:templates/registry.lock.json`.
- Nâng cảnh báo `source.license = "TODO"` thành lỗi.

### T7.3 Dọn (S)

- Xóa `templates/golden-hashes.json`, `scripts/templates/golden.mjs`, `scripts/templates/codemods/`, test Go có tag `templategolden`.
- Chạy kiểm tra cuối: `pnpm typecheck`, `pnpm lint`, `pnpm test`, `pnpm knip`, `pnpm templates:check`, `make check` (Go + web), `git diff --check`.

## 11. Bảng tóm tắt kích thước

| Giai đoạn | Nội dung | Ước lượng |
| --- | --- | --- |
| 0 | Tiên quyết, đo nền, băm vàng | 1,5 ngày |
| 1 | Di chuyển nguồn (M1) | 3 ngày |
| 2 | Lõi registry và skill | 4 ngày |
| 3 | Squad, dịch, lazy (M2) | 6 ngày |
| 4 | Go, công ty mẫu, workflow (M3) | 5 ngày |
| 5 | Pack, bundle, chia gói | 2 ngày |
| 6 | Băm thay lịch sử (tùy chọn) | 2 ngày |
| 7 | Tài liệu, CI, dọn | 1 ngày |
| Tổng | | khoảng 24 ngày với giai đoạn 6; khoảng 22 ngày không có |

Đây là ước lượng thô cho một người. Giai đoạn 3 và 4 lớn nhất và có độ bất định cao nhất.

## 12. Rủi ro và điểm hoàn tác

| Rủi ro | Giai đoạn | Giảm thiểu | Điểm hoàn tác |
| --- | --- | --- | --- |
| Đụng thay đổi chưa commit của người dùng | 1 đến 4 | T0.2; PR nhỏ; không di chuyển khi `git status` bẩn trong vùng | Dừng và đợi, không có gì để hoàn tác |
| Lệch hành vi vì đổi chữ | 1 đến 5 | Băm vàng mỗi bước; `git mv` và codemod | `git revert` PR gần nhất |
| `templates/` không kích hoạt CI hoặc cache turbo phát lại kết quả cũ | 1 | T1.1 kiểm chứng bằng `--dry=json` và `ci-scope` | `git revert` T1.1 |
| Docker thiếu dữ liệu | 2 đến 4 | Đầu ra sinh ra được commit trong `packages/` và `server/`; không đổi Dockerfile | Không cần |
| Hai đường dữ liệu cho Kit Việt trong lúc chuyển | 3 | Commit từng nhóm 5 squad, `--check` sau mỗi nhóm | `git revert` nhóm gần nhất |
| Ba bản SDLC Auto lệch nhau sẵn từ trước | 4 | T4.3a lập báo cáo, người dùng chọn bản chuẩn trước khi hợp nhất | Dừng sau T4.2 (đã có giá trị riêng) |
| Mất kiểm tra kiểu khóa dịch | 3 | Union id sinh ra, `useT("templates")` có kiểu | `git revert` T3.5 |
| `crypto.subtle` thiếu trên HTTP | 6 | sha256 thuần JS; hoặc bỏ giai đoạn 6 | Bỏ giai đoạn |
| CLI trở thành gánh nặng | tất cả | CLI tối thiểu (4 lệnh), test riêng, chia theo mốc | Dừng sau M1 hoặc M2 |

Quy ước hoàn tác: mỗi task là một commit hoặc một PR độc lập, nên hoàn tác là `git revert` của chính commit đó; sau mỗi giai đoạn, nhánh chính xanh toàn bộ kiểm tra nên mốc cuối giai đoạn là điểm quay lại an toàn.

## 13. Ngoài phạm vi

- Kho template lúc chạy (DB, Hub, từ xa), chợ template, template riêng theo workspace, giao diện soạn template. Spec mục 5.3 nêu đường nối, không làm.
- Đổi nội dung prompt, skill, charter, hay hành vi quickstart và nâng cấp.
- Migration DB, API mới ngoài T4.4.
- Skill nền tảng của máy chủ (`server/internal/service/builtin_skills/`, `builtin_skills_legacy/`), agent Mika, `.agents/skills/`, `skills-lock.json`.
- `examples/plugins/` và runtime plugin (Q10).
- Nội dung `apps/docs/`.
- Dịch nội dung dành cho agent.
- Thay đổi ứng dụng mobile.
- Nâng commit upstream của Kit Việt.
- Giao diện chọn nhiều agent pack.

## 14. Danh sách kiểm tra khi hoàn tất

- [ ] Thêm skill, skillset, squad, workflow, collection, pack chỉ tạo file dưới `templates/` và đầu ra sinh ra (T3.7 chứng minh với squad thử).
- [ ] `pnpm templates:check` xanh trong CI; không còn cơ chế kiểm tra độ tươi rời rạc.
- [ ] Băm vàng bằng nhau ở mọi bước; sau T7.3 `registry.lock.json` thay vai trò đó.
- [ ] Go không còn đọc dữ liệu sinh từ esbuild; `scripts/generate-company-template.mjs` đã xóa.
- [ ] `AGENTS.md` có hàng `templates/` và lệnh sinh.
- [ ] Mô tả PR từng giai đoạn ghi đúng lệnh đã chạy và lệnh bỏ qua.
