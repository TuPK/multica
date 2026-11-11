# Kit Việt squads: implementation plan

Kế hoạch triển khai cho [kitviet-squads.md](kitviet-squads.md). Trạng thái (2026-11-11): các task đã làm nằm trên nhánh `kitviet-pack`, nhiều task gom vào một commit (bảng "Trạng thái" dưới đây; sai lệch ghi ở spec 8.1); diễn tập S7 chưa chạy. Đọc spec trước; các mục "5.x", "6.x", "8.x", "10.x", Fxx và Qx dưới đây trỏ về spec.

Quy ước thực hiện (theo AGENTS.md):

- Commit nhỏ, mỗi task một commit, kiểu conventional (`feat(squads): ...`). Kết thúc commit bằng dòng `Co-Authored-By` theo hướng dẫn của phiên làm việc.
- PR chỉ mở vào `--repo TuPK/multica`, không bao giờ vào `multica-ai/multica`. Mô tả PR theo mẫu (Problem, Goal, Proposal) và nêu phạm vi nội dung đã đọc (mục 3.2 của spec).
- Sau mỗi task chạy kiểm tra hẹp nhất có ích; cuối mỗi giai đoạn chạy bộ rộng hơn (`pnpm typecheck`, `pnpm lint`, `pnpm test`). Báo cáo đúng những gì đã chạy và những gì bỏ qua.
- Dữ liệu sinh ra không sửa tay: sửa nguồn rồi chạy `pnpm generate:kitviet`; test `--check` fail khi tệp sinh cũ.
- Chuỗi UI qua i18n đủ bảy ngôn ngữ (en, zh-Hans, zh-Hant, fr, es, ko, ja); mô tả card theo mục "UI descriptions" của `conventions.mdx`: chỉ giữ điều không hiển nhiên.
- Test mặc định **không** gọi mạng, không gọi nhà cung cấp trả phí, không chạy agent CLI thật. Phần chạy thật (mục 8.2 của spec) nằm ở các bước "diễn tập" và chỉ chạy khi người dùng cho phép.
- Chỉ stage đường dẫn của task mình. Cây làm việc có thể có thay đổi của phiên khác; không `git add -A`.
- Không dùng khóa ngoại hay cascade; kế hoạch này không đụng schema hay server Go, trừ khi một cổng ở Giai đoạn G cho kết quả "không".

## Trạng thái

Ghi ngày 2026-11-11. Commit là hash trên nhánh `kitviet-pack` hiện tại. Mục "Lệch" nêu khác biệt với kế hoạch; danh sách đầy đủ ở spec 8.1.

| Task | Trạng thái | Commit | Lệch |
| --- | --- | --- | --- |
| G1–G4 | Đóng (spec 10.2) | — | Không cần thêm phương thức client: `addAgentSkills` và `updateAutopilot` đã có. |
| K1 chia hàm dùng chung | Xong | `5e9547703` | — |
| K2 vendor | Xong | `263803bce` | `kitviet-upstream/**` nằm trong lint ignore (`600536b21`, không có trong plan). Thư mục có cây của nhiều nghề hơn năm P0, còn `NOTICE.md` ghi năm nghề; cần đối chiếu trước khi nâng commit. |
| K3 viết lại và vá | Xong | `fb8076c3f`, `1dea9d3c7` | 26 tệp vá với 66 cặp, kế hoạch là 6 tệp. Thêm vá cho trích dẫn có ngày và chuẩn hóa từ kết luận gác cổng. 24 luật viết lại, áp cả cho script. |
| K4 generator | Xong | `9f72f664a`, `5ad3ebee3`, `4cc9bc641` | Phân giải tham chiếu theo squad tách thành commit riêng. Đọc tệp upstream theo LF (`46eaae782`, không có trong plan). `--check` xanh. |
| K5 skill và instructions viết mới | Xong | `aa996c9bb`, `e787f2217` | Compose lười và tách tên khỏi dữ liệu pack là commit riêng. |
| K6 kiểu template | Xong | `f1f749001` | Gộp với K7. Có `startPaused`, `leaderSkills`, `roleSkills`. |
| K7 nạp pack và gắn theo vai | Xong, có thêm | `f1f749001`, `7e9325df8` | Nạp theo pack (`kitvietPackOf`). Tạm dừng routine: chỉ tạm dừng lại autopilot của quickstart khi lần chạy trước thất bại; trạng thái `partial` (spec Q9). Phần này là thay đổi của phiên ngày 2026-11-11. |
| K8 test pack | Xong | `4224e0784` | Chưa có số kích thước bundle trong nhánh; plan yêu cầu hai số. |
| K9 runbook và cổng | Xong | `9ce28b243`, `a33cdaa96` | Runbook có mục 6a (routine tạm dừng khi chạy lại) và mục 9 (nâng cấp chưa có điểm gọi). |
| S1 template | Xong, gộp năm squad | `049b82605` | Một commit cho năm template, không tách từng squad. |
| S2 dữ liệu pack | Xong, gộp | trong K4 | Không có commit riêng cho từng squad. |
| S3 locale | Xong, gộp năm squad, bảy ngôn ngữ | `a4e7b0bf9` | Phiên ngày 2026-11-11: bỏ khóa `kv_*` không dùng khỏi `agents.json` (bảy ngôn ngữ) và thêm `status_partial` vào `modals.json`. Kiểu `StandaloneInstructionTemplate` khiến bộ chọn mẫu chỉ tra khóa dịch của mẫu độc lập. |
| S4 test template | Xong | `049b82605` | Mỗi squad một test trong `kitviet-templates/`. |
| S5 runbook từng squad | Xong, gộp | `a33cdaa96` | Mục từng squad ở runbook mục 8. |
| S6 cổng PR | Một phần | — | Phiên ngày 2026-11-11: `vitest` core 40 tệp, 723 test; `vitest` views 87 tệp, 1218 test; `tsc` core và views; eslint trên tệp đã đổi; `generate:kitviet --check`. Chưa tách PR từng squad; chưa chạy `pnpm lint` và `pnpm test` cho toàn repo. |
| S7 diễn tập | Chưa chạy | — | Chờ người dùng cho phép (spec 8.2). |
| Nâng cấp (ngoài plan) | Hàm có, chưa gọi | `7e9325df8` | `upgradeKitvietSquad` và `upgradeSdlcSquad` không có điểm gọi trong giao diện hay mutation (spec 10.3). |
| F18 · Sửa cổng severity | Xong | chưa commit | Ngoài plan. Ba bản vá `kitviet-patches/` (`soat-thien-kien-tuyen-dung`, `soat-phap-ly-dao-duc-bao-chi`, `soat-liem-chinh-hoc-thuat`) đưa mức về Chặn / Nên sửa / Góp ý và "Cảnh báo chưa chặn". Test `OLD_SEVERITY` bỏ nhầm "cân nặng" (tiêu chí ngoại hình), không phải mức. |
| P1 · năm squad (Talent, PR, E-commerce, Office Ops, Academic) | Xong: template, pack, locale 7 ngôn ngữ, test, runbook mục 8 | chưa commit | Lệch: Office Ops là luồng viết tay (`flow: authored`), không có dây chuyền upstream. Lời dặn lead của mọi squad P1 nằm trong `KV_LEAD_NOTES` (`kitviet-agent-instructions.ts`), không còn trong file template. Floor `Revenue` thêm vào `COMPANY_FLOOR_KEYS` (Q4). Không có cổng soát dự toán (quyết định của người làm Office Ops). Generator: `pr` → `PR` trong tên vai. Sổ quy định không có cho P1 (Q1). |
| Mục 7 · skill mở rộng | Xong | chưa commit | Lệch: không qua `setSquadSkills` mà `attachKitvietExtensions` (`kitviet-extension-attach.ts`) sau mỗi apply công ty (`useApplyCompanyTemplate`), dùng `attachBuiltinSkills` và chỉ thêm skill, không xóa. Runbook mục 8b. Cổng rà soát nội dung không áp dụng (Q1). |
| P2 · mười squad (Healthcare, Construction, F&B, Insurance, Banking, Real Estate, Hospitality, Import & Export, Manufacturing, Education) | Xong: template, pack, locale 7 ngôn ngữ, test, runbook mục 8, 8c và 8d | chưa commit | Lệch: trần floor 16 → 24 (spec Q11). Gác rủi ro chặn mức "Cần giấy tờ" (spec Q12); gác cam kết tư vấn của Insurance chặn mọi lỗi; "Vừa" của gác báo giá và gác cam kết lưu trú thành "Cần giấy tờ". `roleSkills` lấy từ cấu hình pack, không từ bản nháp của `kitviet-p2-notes.md`. Lời dặn lead trong `KV_LEAD_NOTES`; test lead đọc từ đó. Floor P2 không vào `COMPANY_FLOOR_NAMES`, quickstart tạo khi thiếu. Tên vai `F&B Lead`. Education giữ bốn agent ngoài squad. |
| Trần floor 16 → 24 (spec Q11) | Xong | chưa commit | `CompanyMaxFloors`, `companyMaxFloors`, `COMPANY_MAX_FLOORS` và các chuỗi thông báo, tài liệu đều là 24. Test Go `Company|Floor|Template` xanh trên cơ sở dữ liệu của worktree. |

| Floor `Revenue` (Q4) | Xong | chưa commit | `company-template.ts` và `company_template_data.json` (đã sinh lại). Starter company không tạo floor này vì không card nào nằm trên đó; nút chọn squad không hiện nhóm rỗng. Test Go đếm floor theo squad đã chọn, không theo mọi key. |

## Nền đã có và ranh giới

- `SQUAD_TEMPLATES` (`packages/core/squads/templates.ts`) là mảng tĩnh; giao diện `packages/views/modals/squad-quickstart.tsx` và `company-setup-card.tsx` đọc từ đó. `bmad_planning` là mẫu gần nhất (id, role key, instructions riêng, charter).
- `attachBuiltinSkills` (`quickstart.ts`) hiện tra `SDLC_BUILTIN_SKILLS` theo tên, tạo hoặc cập nhật skill workspace, gắn ở mức squad qua `setSquadSkills`. Skill squad đến mọi member (`ListEffectiveAgentSkills`); `SetAgentSkills` xóa skill riêng, `AddAgentSkills` cộng dồn (spec 10.2).
- `ensureRoutine` tạo autopilot ở trạng thái hoạt động; không có `startPaused` (spec PR 1).
- `scripts/generate-sdlc-skills.mjs` ghi cứng một thư mục vào một tệp; hàm đọc frontmatter và duyệt tệp tách rời được.
- Daemon ghi skill vào `{workDir}/.claude/skills/<tên>/` (hoặc thư mục tương ứng của runtime), nên `scripts/` của skill không nằm ở thư mục làm việc (F26).

## Tổng quan giai đoạn

| Giai đoạn | Nội dung | Phụ thuộc | Đụng server | Kích cỡ | Rủi ro |
| --- | --- | --- | --- | --- | --- |
| G | Bốn cổng đọc mã trước khi viết | không | không | nhỏ | thấp |
| 1 · Khung (PR 1) | Rút lib, vendor, generator, vá, skill viết mới, loại vai, quickstart nạp pack, `leaderSkills`/`roleSkills`/`startPaused` | G | không | rất lớn | cao |
| 2 · Office Admin (PR 2) | Template, locale, runbook | 1 | không | vừa | thấp |
| 3 · B2B (PR 3) | như trên | 2 | không | vừa | thấp |
| 4 · Marketing (PR 4) | như trên, kèm Sổ quy định Marketing | 2 | không | vừa | trung bình |
| 5 · Legal (PR 5) | như trên, kèm routine | 2 | không | vừa | trung bình |
| 6 · Accounting (PR 6) | như trên, kèm routine, bài kiểm F19 | 2 | không | vừa | trung bình |

Thứ tự: G → 1 → 2 → (3, 4, 5, 6 song song được, nhưng ra theo thứ tự của Q1: Admin, B2B, Marketing, Legal, Accounting). Không có nhãn "bản thử" và không có cổng xác minh pháp lý (Q1).

---

## Giai đoạn G: các cổng đọc mã

Mỗi cổng ra một đoạn kết luận ngắn ghi vào spec mục 10.2 (đánh dấu đã đóng). Cổng cho kết quả "không" thì sửa thiết kế trước khi sang Giai đoạn 1.

| Cổng | Câu hỏi | Cách trả lời | Ảnh hưởng nếu "không" |
| --- | --- | --- | --- |
| G1 | `SquadQuickstartClient` có `addAgentSkills` và `updateAutopilot` chưa, và `useSquadQuickstart` nối client thật vào đó thế nào? | Đọc `quickstart.ts` (kiểu client), `packages/core/api/client.ts`, hook `useSquadQuickstart` | Thêm hai phương thức vào giao diện client và vào hook |
| G2 | Tên skill có dấu hai chấm (`kv:viet-email`) có được máy chủ và daemon chấp nhận như tên built-in hiện có (`superpowers:brainstorming`)? Thư mục skill trên đĩa dùng tên nào? | Đọc `execenv/context.go` (`writeSkillFiles`), test `reserved_skill_path_test.go`, cách `sdlc-skills` lưu `--` | Nếu thư mục không an toàn: đổi tên skill sang `kv-<tên>` toàn bộ (spec F11, mục 5.5) |
| G3 | Modal Squads quickstart và `company-setup-card` có giả định số template hoặc id cố định không? | Đọc `modals/squad-quickstart.tsx`, `company-setup-card.tsx` và test | Sửa giao diện để duyệt `SQUAD_TEMPLATES`; kể tên tệp trong PR |
| G4 | Tên squad có ngoặc và dấu (`Legal & Contracts (Pháp lý)`) qua được việc kiểm tên squad và tái dùng theo tên của quickstart? | Đọc `quickstart.ts` (tìm squad theo `template.name`), handler squad | Nếu có giới hạn ký tự: giữ tên Anh, đưa từ Việt vào charter và floor |

---

## Giai đoạn 1: khung (PR 1)

### K1 Rút hàm dùng chung

Files: `scripts/lib/skill-bundle.mjs` (mới, `skillFiles`, `frontmatterDescription`), `scripts/generate-sdlc-skills.mjs` (gọi lại lib).

- Bước: chuyển hai hàm sang lib, đổi script cũ import; chạy `pnpm generate:sdlc-skills` rồi `git diff --stat packages/core/squads/sdlc-skills-data.ts` phải rỗng.
- Kiểm: test stale của `sdlc-skills.test.ts` vẫn xanh.
- Commit: `refactor(scripts): share the skill bundle helpers between generators`.

### K2 Vendor upstream

Files: `packages/core/squads/kitviet-upstream/{LICENSE,NOTICE.md,UPSTREAM.json}` và `claude-code/kv/{skills,agents}` của năm nghề P0 (`phap-ly`, `ke-toan`, `kinh-doanh`, `marketing`, `hanh-chinh-van-phong`) cộng phần nền, đúng commit `faee4f14cd21672325a7f442b19c7239a5d7f152`.

- Chỉ chép `claude-code/kv/skills`, `claude-code/kv/agents` và bốn script xuất file (một bản, vì giống nhau giữa các nghề); không chép `codex/`, `claude-web/`, `brand/`, `*.zip`.
- `NOTICE.md`: nguồn, commit, ngày lấy (2026-11-10), danh sách thay đổi (chỉ là "không sửa tay; mọi thay đổi nằm ở generator và `kitviet-patches/`"). `UPSTREAM.json`: `{ repo, ref, license: "MIT", fetchedAt }`.
- Kiểm: băm từng tệp so với bản clone ở commit đã ghim (script một lần, không commit); `pnpm lint` không quét thư mục này (thêm vào ignore nếu cần).
- Commit: `chore(squads): vendor the Kit Viet skills for five trades at a pinned commit`.

### K3 Dữ liệu viết lại và bản vá

Files: `packages/core/squads/kitviet-rewrites.json` (bảng ở spec 5.5), `packages/core/squads/kitviet-patches/{lich-han-nop-thue,kiem-tra-hoa-don-dau-vao,goi-y-dinh-khoan,soat-luat-quang-cao,the-le-minigame-khuyen-mai,soan-cong-van-hanh-chinh}.json` cùng các bản vá cho trích dẫn luật nằm ngoài sáu skill đó (xem bảng ở spec 5.5). Không có `_all.json`: quy tắc áp mọi skill nằm trong `kitviet-rewrites.json`.

- Mỗi bản vá là `[{ "find": "...", "replace": "..." }]`, `find` là câu nguyên văn lấy từ upstream đã vendor (copy bằng máy, không gõ).
- `replace` luôn trỏ vào sổ quy định ("tra dòng <id> của Sổ quy định - <squad>"), không viết lại mốc. Nếu sổ chưa tồn tại: câu thay ghi "chưa có sổ quy định: tự tra nguồn chính thức và ghi `[chưa tìm thấy trên nguồn chính thức]` nếu không có" (Q1: sổ không phải điều kiện).
- Kiểm: unit test của generator (K4) chạy từng bản vá và fail nếu `find` không khớp đúng một chỗ.
- Commit: `feat(squads): add the rewrite table and patches for the Kit Viet pack`.

### K4 Generator

Files: `scripts/generate-kitviet-pack.mjs`, `package.json` (`generate:kitviet`), đầu ra `packages/core/squads/kitviet-skills-data/{common,admin,b2b,marketing,legal,accounting}.ts`, tệp kiểu dùng chung `kitviet-skills-data/types.ts` (tái dùng `BuiltinSkill`).

Thứ tự bước (spec 5.5): (1) đọc agent và skill của nghề và phần nền; (2) xóa khối Pro (F1), mục nền tảng (F4), `CLAUDE.md`/`.kv-run` (F3, F7), áp bảng viết lại tên tệp (F22, F26) rồi bản vá (F23); (3) viết lại tham chiếu `kv:x`, bỏ ba skill bị loại (F11, F12, F17, F27); (4) phân loại agent theo `tools` thành bốn loại vai, sinh `KV_ROLE_AGENT_INSTRUCTIONS` (thân đã làm sạch, đầu `You are the <role> in a <squad> squad`, quy tắc `VERDICT:` cho gate, mục 5.3 cho điểm dừng); (5) ghi `config: { source: "kitviet", upstream, ref, license }`; (6) chế độ `--check`.

- Chuẩn hóa dòng kết luận gate về `VERDICT: QUA|CHẶN` (F18): `Đạt`, `Gửi được` → QUA; `Chưa đạt`, `Chưa gửi được` → CHẶN.
- Tập phân giải tham chiếu của squad = tầng lõi ∪ `roleSkills` (F17, mục 5.10). Tham chiếu ngoài tập: đổi thành "nhờ lead tạo yêu cầu".
- Tên agent trong thân đổi theo bảng role key của từng squad (mục 6); agent không có vai thì "nhờ lead".
- Kiểm: `node scripts/generate-kitviet-pack.mjs --check` xanh sau khi sinh; chạy hai lần cho cùng kết quả.
- Commit: `feat(squads): generate the Kit Viet skill and agent data from the vendored pack`.

### K5 Skill và instructions viết mới

Files: `packages/core/squads/kitviet-skills/{kv-delivery-core,kv-squad-lead,kv-company-profile}/SKILL.md` (viết tay, tiếng Anh, mẫu đầu ra tiếng Việt), `packages/core/squads/kitviet-agent-instructions.ts`, `packages/core/agents/instruction-templates.ts` (năm id `kv_lead`, `kv_collector`, `kv_drafter`, `kv_gate`, `kv_handover`).

- `kv-delivery-core` gồm: giao thức issue (spec 5.2), `VERDICT:` và vòng sửa tối đa 2, ba loại cổng người và cơ chế thông báo (5.3, 10.2 dòng c), `[cần bổ sung]`, không đoán số, **tính bằng script không tính nhẩm** (F19), xử lý `.xlsx` (F19), chạy script theo thư mục chứa SKILL.md (F26), "hỏi" nghĩa là cổng loại 1 chỉ khi thiếu thứ không mặc định được (F27), ngày chạy ở dòng đầu của issue routine (F21), vai tra web không có công cụ web thì dừng loại 1 (F25), thiếu thư viện thì dừng loại 1 và không tự cài (F9).
- `kv-squad-lead`: triage, kế hoạch (thay `lap-ke-hoach-dieu-phoi`), giao sub-issue, kiểm hợp đồng bàn giao, đếm vòng sửa, `blocked`/`in_review` trên request gốc, tổng hợp.
- `kv-company-profile`: phỏng vấn hồ sơ đơn vị (5.4), tạo draft KB loại `reference`, trường riêng theo squad (chế độ kế toán có ba lựa chọn do người dùng xác nhận, F20; người ký và quyền hạn cho Admin).
- Kiểm: test skill cấu trúc của repo (nếu có cho `sdlc-skills` thì áp tương tự); đọc lại bằng mắt theo bảng kiểm mục 5.5.
- Commit: `feat(squads): add the Kit Viet delivery, lead and company-profile skills`.

### K6 Mở rộng kiểu template

Files: `packages/core/squads/templates.ts` (`SquadTemplateId` thêm năm id, `SquadRoleKey` thêm role có tiền tố squad, `SquadTemplate.leaderSkills?`, `roleSkills?`, `SquadRoutineTemplate.startPaused?`), `packages/core/squads/index.ts` (export).

- Chưa thêm template nào; chỉ kiểu và bản trống để PR sau chỉ thêm dữ liệu.
- Kiểm: `pnpm --filter @multica/core typecheck`.
- Commit: `feat(squads): extend the squad template type for per-role skills and paused routines`.

### K7 Quickstart nạp pack và gắn theo vai

Files: `packages/core/squads/quickstart.ts` (`attachBuiltinSkills`, `isBuiltinSkill`, `ensureRoutine`, giao diện `SquadQuickstartClient`), `packages/core/api/client.ts` hoặc hook nếu G1 đòi, `quickstart.test.ts`.

- `attachBuiltinSkills`: tên `kv--*`/`kv:*` nạp bằng `import()` từ `kitviet-skills-data/<pack>`; tên khác tra `SDLC_BUILTIN_SKILLS` như cũ. `config.source = "kitviet"` và `isBuiltinSkill` nhận cả hai nguồn.
- Sau khi tạo agent: `addAgentSkills(leaderAgentId, leaderSkills)` và `addAgentSkills(agentId, roleSkills[role])`; **không** dùng `setAgentSkills`. Chạy lại quickstart không nhân đôi (đã có thì bỏ qua).
- `ensureRoutine`: khi `startPaused` và autopilot vừa tạo mới, gọi `updateAutopilot(id, { status: "paused" })`; không bao giờ bỏ tạm dừng routine đã có.
- Test (đặt cạnh `quickstart.test.ts`): bundle `kv--` nạp lười, không đụng `SDLC_BUILTIN_SKILLS`; `roleSkills` gắn đúng vai; chạy lại không trùng; `startPaused` chỉ khi tạo mới; skill squad đã có không bị mất.
- Commit: `feat(squads): load Kit Viet skills on demand and attach them per role`.

### K8 Test của pack và kiểm bundle

Files: `packages/core/squads/kitviet-skills.test.ts`.

- Gồm: không còn `kv:pro`, `mochistudio`, `Bản Pro`, `.kv-run`, `CLAUDE.md`, `AGENTS.md`, `ket-qua/`, "phiên chính", `Claude Desktop`, `Codex`, `\d\d-[a-z-]+\.md` trong thân skill và agent sinh ra; mọi `kv:x` phân giải được **theo từng squad** (F17); mọi gate có đúng một dòng `VERDICT:` (F18); mỗi bản vá áp đúng một lần; `name` bằng thư mục; không trùng tên với skill khác; `LICENSE` và `NOTICE.md` có mặt; dung lượng mỗi pack dưới ngưỡng (tầng lõi dưới 140 KiB, Accounting dưới 160 KiB theo ngưỡng riêng ở spec 5.5, dưới 30 skill); `description` dưới 400 ký tự; dữ liệu sinh khớp nguồn (`--check`).
- Kiểm kích thước bundle: dựng web trước và sau (`pnpm --filter @multica/web build` hoặc lệnh đang dùng), so chunk của trang không dùng kitviet; PR ghi hai số.
- Commit: `test(squads): guard the Kit Viet pack against stale data, leftovers and dangling references`.

### K9 Runbook và cổng giai đoạn

Files: `docs/engineering/kitviet-runbook.md` (mới): cách nâng commit upstream (sửa `UPSTREAM.json`, vendor lại, `pnpm generate:kitviet`, đọc diff và bản vá), cách đăng Sổ quy định và rút chu kỳ xem lại (`multica knowledge kind set`), phụ thuộc runtime (Bash, Python, `python-docx`, `openpyxl`, `python-pptx`, `reportlab`, LibreOffice, font), cách đọc `VERDICT:` thiếu. Không có preflight mã: việc kiểm thư viện là lời dặn trong `kv-delivery-core` (dừng loại 1), runbook liệt kê lệnh kiểm tay.

Cổng ra Giai đoạn 1: `pnpm typecheck`, `pnpm lint`, `pnpm test` xanh; `pnpm generate:kitviet --check` không đổi; bundle trang không dùng kitviet không tăng. Chưa squad nào hiện trong giao diện.

Commit: `docs(squads): add the Kit Viet runbook`.

---

## Giai đoạn 2 đến 6: một PR cho mỗi squad

Mỗi PR squad có cùng khuôn task; chỉ khác dữ liệu ở bảng dưới. Mỗi task một commit.

**Khuôn.**

- **S1 Template.** Thêm template vào `packages/core/squads/kitviet-templates.ts`, nối vào `SQUAD_TEMPLATES` (`templates.ts`). Điền: `name` (kèm từ Việt, Q6), `description` (một dòng, điều không hiển nhiên), `roles` và `leader` theo bảng vai ở mục 6, `floorName`, `charter` và `routine` theo Phụ lục B, `instructions` (lead, từ `kv_lead`), `workflow: null`, `builtinSkills` (tầng lõi: skill nghề + 15 nền + `kv-delivery-core`, cộng skill nâng lên theo F17), `leaderSkills`, `roleSkills` (mục 5.10). Commit `feat(squads): add the <squad> Kit Viet squad template`.
- **S2 Dữ liệu pack.** Chạy `pnpm generate:kitviet` cho pack của squad; commit riêng tệp sinh. Commit `chore(squads): generate the <squad> Kit Viet pack`.
- **S3 Locale.** Thêm tên và mô tả card vào `packages/views/locales/{en,es,fr,ja,ko,zh-Hans,zh-Hant}/modals.json`, cùng chỗ với `bmad_planning`. Chạy bộ kiểm khóa i18n của repo (`pnpm test` trong `packages/views`). Commit `feat(views): name the <squad> squad template in every locale`.
- **S4 Test template.** Cập nhật `templates.test.ts`: đủ role, một lead, số member ≤ 8, `floorName` thuộc tập floor mặc định, mọi `builtinSkills` và `roleSkills` có trong pack, charter hợp lệ, routine đúng cron và `startPaused`. Commit cùng S1 nếu nhỏ.
- **S5 Runbook.** Thêm đoạn của squad vào `kitviet-runbook.md`: điểm dừng của squad, Sổ quy định cần tạo (nếu có), phụ thuộc riêng. Commit `docs(squads): document the <squad> Kit Viet squad`.
- **S6 Cổng PR.** `pnpm typecheck`, `pnpm lint`, `pnpm test`; `pnpm generate:kitviet --check`. Mô tả PR ghi phạm vi nội dung đã đọc (spec 3.2).
- **S7 Diễn tập (chỉ khi người dùng cho phép).** Ba lần chạy thật theo mục 8.2: đủ đầu vào; thiếu đầu vào (kỳ vọng `blocked` trên request gốc, không soạn); bản nháp cố tình sai (kỳ vọng gate CHẶN và không ai đổi thành QUA). Dán biên bản vào PR. Xác nhận thêm: người đặt nhận thông báo khi `blocked` và `in_review`; agent nhận hợp skill squad và skill riêng (spec 10.2, chỉ để xác nhận lại).

**Dữ liệu từng squad.**

| PR | Squad (`name`) | `floorName` | Vai (mục 6) | Điều riêng |
| --- | --- | --- | --- | --- |
| 2 | `Office Admin (Hành chính)` | Executive | `admin_lead`, `admin_files`, `admin_format_lookup`, `admin_drafter`, `admin_format_gate`, `admin_numbers_gate`, `admin_handover` | Không routine. Hồ sơ đơn vị phải có tên, ký hiệu, địa danh, quy tắc đánh số, người ký và quyền hạn. Không cấp số văn bản chưa ký. Sổ công văn đến/đi là issue cố định (mục 5.8). Bản vá `soan-cong-van-hanh-chinh`. Gắn `quan-ly-lich-giam-doc` cho lead, `soan-don-tu`, `so-sanh-bao-gia-mua-sam`, `chuan-bi-hop-hdqt` cho `admin_drafter` |
| 3 | `B2B Proposal Desk (Chào giá B2B)` | Launch | `b2b_lead`, `b2b_history`, `b2b_company_lookup`, `b2b_quote_drafter`, `b2b_commit_gate`, `b2b_data_gate`, `b2b_tone_gate`, `b2b_numbers_gate`, `b2b_handover` | Không routine. Phần "NỘI BỘ - KHÔNG GỬI KHÁCH" tách khỏi bản gửi và bị `b2b_data_gate` soát. Nâng `bien-ban-gap-khach-b2b` lên tầng lõi. Không tạo squad thứ hai soạn báo giá (Sales Loop giao deal, mục 5.6) |
| 4 | `Marketing Studio (Marketing)` | Launch | `mkt_lead`, `mkt_scout`, `mkt_insight`, `mkt_content`, `mkt_ads`, `mkt_claim_gate`, `mkt_voice_gate`, `mkt_numbers_gate`, `mkt_handover` | Không routine, không `unattended` (Q3). "Chốt thông điệp" là điểm dừng loại 1 trừ khi request đã nêu thông điệp. Sổ quy định Marketing (luật quảng cáo, khuyến mại, thuế trúng thưởng) và Cẩm nang giọng thương hiệu (KB `playbook`). Bản vá `soat-luat-quang-cao`, `the-le-minigame-khuyen-mai`. Nâng `persona-tu-du-lieu-that`. Launch đạt 5/6 sau PR 3 và 4 (mục 5.6) |
| 5 | `Legal & Contracts (Pháp lý)` | Governance | `legal_lead`, `legal_reader`, `legal_sources`, `legal_validity`, `legal_drafter`, `legal_reading_gate`, `legal_citation_gate`, `legal_handover` | Routine `Rà hiệu lực sổ quy định` (`0 8 * * 1`, `startPaused`). Sổ hợp đồng là issue cố định. Gắn skill theo mục 5.10; bản `soan-don-tu-phap-ly` đổi tên. Nâng `dong-thoi-gian-vu-viec` |
| 6 | `Accounting & Tax (Kế toán, thuế)` | Governance | `acct_lead`, `acct_collector`, `acct_tax_research`, `acct_reconciler`, `acct_tax_drafter`, `acct_voucher_gate`, `acct_numbers_gate`, `acct_tax_gate`, `acct_handover` | Routine `Lịch hạn nộp thuế` (`0 8 * * 1`, `startPaused`). Tầng lõi lớn nhất (28 skill). Nâng `goi-y-dinh-khoan`, `soat-chi-phi-duoc-tru`, `tinh-luong-thue-tncn-bhxh`. Bản vá `lich-han-nop-thue`, `kiem-tra-hoa-don-dau-vao`, `goi-y-dinh-khoan`. Không có preflight (Q8): thiếu Bash hoặc Python thì các vai tính số dừng loại 1 (Q5, F19); thêm bài kiểm F19 (đối chiếu hai bảng 200 dòng có một dòng lệch cố ý) vào diễn tập. Thêm dòng GTGT 8% hết hạn 31/12/2026 vào hướng dẫn Sổ quy định (F20) |

---

## Rủi ro và cách chặn

| Rủi ro | Cách chặn |
| --- | --- |
| Upstream đổi câu bị vá | `find` phải khớp đúng một chỗ, generator fail; nâng commit theo runbook |
| Gate thiếu `VERDICT:` khiến lead không đếm được vòng sửa | Test K8 + dòng mẫu do generator chèn; `kv-delivery-core` nhắc lại |
| Tên skill có dấu hai chấm không an toàn trên đĩa | Cổng G2 trước khi viết; lùi về `kv-<tên>` nếu cần |
| Tầng lõi phình làm briefing của leader dài | Trần 30 skill và `description` dưới 400 ký tự trong test K8 |
| Daemon thiếu Bash hoặc Python | Accounting dừng loại 1, runbook liệt kê; các squad khác giảm chất lượng, không chặn |
| `startPaused` làm routine mới không chạy khiến người dùng tưởng hỏng | Charter và runbook ghi rõ routine tạo ở trạng thái tạm dừng và cách bật |
| Bundle trang chính phình do pack | `import()` theo pack (K7) và so kích thước chunk (K8) |

## Việc không thuộc kế hoạch này

Chưa làm: diễn tập P2 (S7, chờ người dùng cho phép), xác minh pháp lý (Q1), locale `vi` (Q6), công tắc `unattended` (Q3), diễn tập thật của P1 (S7, chờ người dùng cho phép), và điểm gọi cho `upgradeKitvietSquad`.
