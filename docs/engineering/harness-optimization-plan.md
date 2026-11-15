# Kế hoạch và đặc tả tối ưu harness của squad SDLC

Trạng thái: **bản cuối. Các lựa chọn đã chốt ở mục 5. Chưa có code nào được viết theo tài liệu này.**
Ngày: 2026-11-15. Người chịu trách nhiệm: TuPK.

Tài liệu này kế thừa và không thay thế `sdlc-harness-spec.md` (đặc tả harness v2) và `sdlc-harness-eval.md` (kế hoạch S9).
Khi có mâu thuẫn, đặc tả này ghi rõ phần nào được thay đổi.

---

## 0. Mục tiêu và phạm vi

**Mục tiêu.** Làm cho harness của squad:
1. Không cho agent đọc được đáp án, và không để kết quả đánh giá lẫn vào cấu hình.
2. Đo được chính xác harness nào đã tạo ra kết quả nào.
3. Báo cáo đóng request nói đúng trạng thái thật.
4. Ngăn các hành động không đảo ngược bằng cơ chế, không chỉ bằng lời dặn.
5. Giữ prompt luôn bật ở mức gọn, có bài học có bằng chứng và hạn hiệu lực.
6. Có đường đánh giá (S9) chạy được với grader rõ ràng.

**Trong phạm vi:** `catalog/skills/builtin/sdlc-harness/`, `catalog/skills/builtin/sdlc-squad-lead/`,
`catalog/skills/builtin/sdlc-delivery-core/`, `catalog/skills/builtin/sdlc-qa-verification/`,
`packages/core/squads/sdlc-harness-*.test.ts`, `scripts/upgrade-sdlc-squad.mjs`, tài liệu `docs/engineering/sdlc-harness-*.md`,
và một phần server Go trong W11.

**Ngoài phạm vi:**
- Tích hợp bộ `google-research/harness2` vào repo. Đã kết luận là không làm (xem trao đổi trước).
- Tìm kiếm harness theo từng issue (4k rollout mỗi task). Quá đắt.
- Đổi cấu trúc workflow SDLC Auto ngoài W11.
- Thêm benchmark mới.

---

## 1. Hiện trạng đã kiểm chứng

Các điểm dưới đây đã đối chiếu với code hoặc tài liệu tại thời điểm viết. Mỗi điểm có nguồn.

| # | Hiện trạng | Nguồn |
|---|---|---|
| S1 | Nguồn skill đã chuyển sang `catalog/`. Lệnh build hiện tại là `pnpm catalog:build` (`scripts/catalog/cli.mjs`), sinh mã vào `packages/core/catalog/generated/`. File `packages/core/squads/sdlc-skills-data.ts` và đường dẫn `packages/core/squads/sdlc-skills/` **không còn trong working tree hoặc HEAD**. Commit `9d3a9e081` (refactor catalog) là nguyên nhân. | `git ls-files`, `package.json`, commit `9d3a9e081` |
| S2 | Fix cho Lead (không giao việc cho chính request) đã có trong nguồn `catalog/skills/builtin/sdlc-squad-lead/SKILL.md` (đã grep, có dòng "The request is not work"). Commit `898f60350` ghi vào đường dẫn cũ, không còn trong cây hiện tại. | grep trên catalog, `git show --stat 898f60350` |
| S3 | `check-harness.mjs` (848 dòng) có H1–H16 và E1–E2. Cờ: `--quick`, `--cert`, `--run`, `--write-cert`, `--refresh`, `--accept`. Chứng nhận gắn với sha trong `docs/sdlc/harness-cert.json`. | `check-harness.mjs`, SKILL.md |
| S4 | `harness-metrics.mjs` (255 dòng) có M2–M7 và M5 "block events". **Không có M1, M8, M9 trong code**, dù `sdlc-harness-spec.md` dòng 380–388 đặc tả chúng. Cờ: `--issues`, `--parent`, `--from-dir`, `--order`, `--output`. | `harness-metrics.mjs`, `sdlc-harness-spec.md` |
| S5 | Số liệu không gắn fingerprint của harness (AGENTS.md, skill bundle) vào từng run. Round 4 và round 5 khác nhau ở skill Lead nhưng không có cách tách số liệu. | Đọc `harness-metrics.mjs`; kết luận cần xác minh trong W2 |
| S6 | Round 5 (DEV-22): báo cáo cuối ghi "every sub-issue has closed", nhưng DEV-25 và DEV-26 ở `stage_done`. Lead vẫn chuyển `plan_ready`. | Comment trên DEV-22, `multica issue list` |
| S7 | Round 4 và round 5: issue epic chuyển sang `in_review` nhưng không có lượt Reviewer. Round 4 không tự phục hồi; round 5 cần chạy lại thủ công. | Lịch sử run của DEV-21, DEV-23 |
| S8 | Quy tắc "không push main ngoài QA landing" và "không `git add -A`" chỉ là văn xuôi trong SKILL. | `sdlc-delivery-core/SKILL.md`, `sdlc-qa-verification/SKILL.md` |
| S9 | Remote pilot `s8-greenfield-todo.git` đã có commit đáp án `a831612` (sumOpen). Bất kỳ agent nào chạy trên repo này sau đó đều đọc được lịch sử. | `git log main` của remote |
| S10 | S9 chưa có grader. Trong 22 tác vụ tupk, chỉ #168 chấm được tất định; #186 chưa xác định. | `sdlc-harness-eval.md` mục 6 |
| S11 | Known pitfalls trong AGENTS.md có yêu cầu evidence (H12) nhưng không có confidence, hạn hiệu lực, hay phạm vi (workspace-fact hay procedure). | SKILL.md, `agents-md-template.md` |
| S12 | Lỗi thông báo của check hiện là chuỗi `details`, chưa có định dạng thống nhất what/why/fix. | `check-harness.mjs` |

---

## 2. Cấu trúc công việc

Mười một workstream, chia thành năm milestone. Mỗi workstream có: vấn đề, thiết kế, file, tiêu chí chấp nhận, phụ thuộc, rủi ro.

```
W0 (vệ sinh)
 ├─ W1 (chống rò) ──► W10 (đường grader S9)
 ├─ W2 (fingerprint) ──► W3 (số liệu)
 ├─ W4 (báo cáo đóng)
 ├─ W5 (cưỡng chế hành động)
 ├─ W6 (bài học)  ─┐
 ├─ W7 (chi phí)  ─┼─ tất cả dùng cùng tài liệu đầu vào của W8
 ├─ W8 (thông báo lỗi) ─┘
 ├─ W9 (hình dạng sản phẩm)
 └─ W11 (platform: in_review)  — độc lập, rủi ro cao nhất
```

| Milestone | Workstream | Mục tiêu |
|---|---|---|
| M0 | W0 | Đường đi đúng và tài liệu không trỏ vào đường cũ |
| M1 | W1, W2, W3 | Số liệu tin được và không rò đáp án |
| M2 | W4, W5 | Báo cáo trung thực, hành động không đảo ngược được cưỡng chế |
| M3 | W6, W7, W8, W9 | Chất lượng harness |
| M4 | W10 | Đường đánh giá S9 có grader |
| M5 | W11 | Lỗi `in_review` của platform |

---

### W0. Vệ sinh nền và đường đi đúng

**Vấn đề.** Tài liệu và các bước đã làm trỏ vào đường dẫn cũ (`packages/core/squads/sdlc-skills/`) và lệnh cũ (`pnpm generate:sdlc-skills`).

**Thiết kế.**
- Cập nhật `docs/engineering/sdlc-harness-calibration.md` và `sdlc-harness-eval.md`: thay đường dẫn cũ bằng `catalog/skills/builtin/...`.
- Ghi quy trình chuẩn trong `sdlc-harness-spec.md` (hoặc ghi chú đầu tài liệu): sửa nguồn trong `catalog/`, chạy `pnpm catalog:check`, chạy `pnpm catalog:build`, rồi chạy `node scripts/upgrade-sdlc-squad.mjs` (dry-run, sau đó `--apply`) với biến `MULTICA_SERVER_URL`, `MULTICA_WORKSPACE_ID`, `MULTICA_TOKEN` trỏ đúng workspace.
- Ghi chú rõ trong calibration rằng các thay đổi `898f60350` và `88c33932f` có đường dẫn đã lỗi thời, và nội dung fix đã có trong nguồn catalog.

**File.** `docs/engineering/sdlc-harness-calibration.md`, `docs/engineering/sdlc-harness-eval.md`, `docs/engineering/sdlc-harness-spec.md`.

**Tiêu chí chấp nhận.**
- `grep -rn "packages/core/squads/sdlc-skills" docs/engineering` không ra kết quả trong các tài liệu chính (trừ dòng ghi chú lịch sử).
- `pnpm catalog:check` pass.

**Phụ thuộc.** Không. Làm đầu tiên.

**Rủi ro.** Thấp. Chỉ sửa tài liệu.

---

### W1. Chống rò đáp án (information firewall)

**Vấn đề.** Đánh giá có thể lộ đáp án theo bốn đường: (a) lịch sử git chứa commit sửa; (b) verdict của QA hoặc grader lọt vào AGENTS.md hoặc skill; (c) harness của squad bị thay đổi dựa trên kết quả đánh giá; (d) mô tả task chứa kết quả. Hiện chỉ có quy ước, chưa có cơ chế (S9).

**Thiết kế.**

**W1.a Snapshot không mang lịch sử tương lai.** Script mới tạo snapshot của một task:
- Đầu vào: repo, commit gốc của task (commit cha của commit sửa), đường dẫn đầu ra.
- Làm: `git archive` tại commit gốc, `git init`, một commit gốc duy nhất với nội dung đó. Không copy `.git` gốc.
- Đầu ra: thư mục snapshot và manifest (`snapshot.json`: repo, commit gốc, sha đã loại, thời điểm).
- Kiểm tra sau khi tạo: `git rev-list --all` chỉ có một commit; sha đáp án không tồn tại trong snapshot.

**W1.b Tripwire trong `check-harness`.** Thêm H17 (level `cert`):
- Quét: AGENTS.md, CLAUDE.md, `docs/sdlc/*.md`, `catalog/skills/builtin/sdlc-*/` của repo đích (chỉ file có trong repo).
- Chuỗi marker mặc định: `QA GO`, `Security Reviewer verdict`, `n_passed`, `pass_rate`, `test_pass_rate`, tên file grader, và danh sách marker lấy từ `docs/sdlc/harness.json` (`markers` tuỳ chọn).
- Lỗi: đường dẫn, số dòng, đoạn trích 120 ký tự quanh marker, kèm câu "Đây là rò rỉ, không phải báo động giả. Tìm đường đọc đã đưa nó vào."

**W1.c Quy tắc trong `sdlc-delivery-core`.** Thêm mục "Không mang verdict vào harness": Architect và Developer không được đưa kết quả đánh giá của role khác (QA, Security, grader) vào AGENTS.md hay skill. `enforced_by: tripwire H17 + prose`.

**W1.d Giới hạn được ghi rõ.** H17 là chốt chặn cuối. Phòng thủ chính là W1.a và việc không đưa verdict vào đầu vào.

**File.**
- Mới: script snapshot `scripts/eval/make-snapshot.mjs` (Q1).
- `catalog/skills/builtin/sdlc-harness/scripts/check-harness.mjs` (thêm H17).
- `packages/core/squads/sdlc-harness-check.test.ts` (test H17).
- `catalog/skills/builtin/sdlc-delivery-core/SKILL.md` (quy tắc mới).

**Tiêu chí chấp nhận.**
- Test snapshot: tạo snapshot từ một repo fixture có commit đáp án ở sau; kết quả không chứa sha đáp án và `git rev-list --all` có một commit.
- Test H17: fixture AGENTS.md có chuỗi `QA GO` → H17 fail, exit 1; fixture sạch → pass.
- Test H17 không chặn file không thuộc danh sách quét.

**Phụ thuộc.** W0.

**Rủi ro.** Tripwire có thể báo động giả với các từ thông dụng như `pass_rate` trong tài liệu chung. Giảm bằng cách chỉ quét đúng vị trí và cho phép ghi ngoại lệ vào `harness.json` (có lý do và người duyệt).

---

### W2. Fingerprint cho mọi run

**Vấn đề.** Số liệu không biết đang đo cấu hình nào. Round 4 và round 5 khác nhau ở skill Lead nhưng đã gộp chung.

**Thiết kế.**
- Định nghĩa fingerprint của một run: `sha256(AGENTS.md ‖ CLAUDE.md ‖ danh sách (tên skill, content_hash) của bộ skill áp dụng trên workspace ‖ phiên bản template squad)`.
- Lấy `content_hash` của từng skill từ nền tảng (`multica skill get` đã trả trường này; đã thấy ở round 4).
- Không tạo manifest trong git (Q4). `upgrade-sdlc-squad.mjs` chỉ in bảng `content_hash` của các skill sẽ áp dụng, để đối chiếu trước `--apply`.
- Mỗi issue/run ghi `harness_fingerprint` vào comment `Harness:` đầu tiên của Lead.
- `harness-metrics.mjs` đọc fingerprint, nhóm số liệu theo fingerprint. Mặc định in một bảng cho mỗi fingerprint. Cờ `--allow-mix` gộp nhiều fingerprint và thêm dòng cảnh báo vào `approximate`.

**File.**
- `scripts/upgrade-sdlc-squad.mjs` (in bảng content_hash khi dry-run).
- `catalog/skills/builtin/sdlc-harness/scripts/harness-metrics.mjs` (nhóm theo fingerprint, `--allow-mix`).
- `catalog/skills/builtin/sdlc-squad-lead/SKILL.md` (Lead ghi `Harness:` ở comment đầu).
- `packages/core/squads/sdlc-harness-metrics.test.ts`.

**Tiêu chí chấp nhận.**
- Test: hai nhóm issue có fingerprint khác nhau → output có hai bảng; không có `--allow-mix` thì không gộp.
- Test: `--allow-mix` → có dòng cảnh báo trong `approximate`.
- Test: issue không có fingerprint được xếp vào nhóm `unknown` và được báo, không bị bỏ.

**Phụ thuộc.** W0.

**Rủi ro.** Lead phải ghi fingerprint đúng. Giảm bằng cách lấy fingerprint từ nền tảng chứ không để agent tự tính.

---

### W3. Số liệu đầy đủ và đúng

**Vấn đề.** M1, M8, M9 được đặc tả nhưng chưa có. Judge lỗi hoặc hạ tầng lỗi có thể bị tính là thất bại (cần kiểm tra trong code, chưa xác nhận).

**Thiết kế.**
- **M1 (chi phí dựng harness):** tổng run và token của issue ở status Harness và Harness smoke. Lấy từ `usage` và `timeline` như M3, M4.
- **M8 (footprint):** lấy `footprint` từ báo cáo `check-harness`; số ngày từ cert; số manifest đổi kể từ cert. Đọc `harness-cert.json`.
- **M9 (smoke):** đạt/trượt; số run; số lệnh tự khám phá (lấy từ comment của Developer, mục "Report every command you had to discover"); bước âm tính đạt/trượt.
- **Ungraded tách riêng:** issue bị lỗi hạ tầng (run `failed` với lỗi provider hoặc timeout nền tảng) đếm vào `ungraded`, không vào `failed`. Hiện harness-metrics có `M3_failedRuns`; thêm `M3_ungradedRuns`. Phải kiểm tra trước khi sửa xem `M3_failedRuns` đang gộp những gì.
- **Đếm run theo identity:** mỗi run có `run_id` duy nhất. Đếm trùng là lỗi, không được cộng hai lần. Dùng cho bộ đếm pilot (giới hạn 120 lượt).
- Cập nhật `sdlc-harness-spec.md` dòng 380–388 nếu tên hoặc định nghĩa thay đổi.

**File.** `catalog/skills/builtin/sdlc-harness/scripts/harness-metrics.mjs`, `packages/core/squads/sdlc-harness-metrics.test.ts`, `sdlc-harness-spec.md`.

**Tiêu chí chấp nhận.**
- Test M1, M8, M9 với fixture có số đã biết.
- Test: run lỗi hạ tầng không làm tăng `M3_failedRuns`, tăng `M3_ungradedRuns`.
- Test: hai run trùng `run_id` → cảnh báo, không cộng đôi.

**Phụ thuộc.** W2 (để nhóm theo fingerprint).

**Rủi ro.** Đổi định nghĩa `M3_failedRuns` làm số liệu cũ không so sánh được. Ghi phiên bản metric vào output (`metric_version`).

---

### W4. Báo cáo đóng request trung thực

**Vấn đề.** Round 5: Lead viết "every sub-issue has closed" khi hai issue ở `stage_done`.

**Thiết kế.**
- Thêm `--status-table` vào `harness-metrics.mjs` (hoặc lệnh riêng `squad-closeout.mjs`): đọc các issue con của request, in bảng `id | title | status | assignee | parent`. Đây là nguồn duy nhất để báo cáo cuối trích dẫn.
- Cập nhật SKILL của Lead, Bước 8 và "Self-check":
  - Báo cáo cuối phải có bảng sinh từ công cụ, không viết bằng tay.
  - Cấm cụm "tất cả", "every", "đã đóng hết" nếu bảng còn dòng chưa `done`.
  - `plan_ready` chỉ khi mọi dòng `done`, hoặc khi có danh sách `stage_done` được người quyết định chấp nhận ghi rõ trong báo cáo.
- Cập nhật `reference.md` (định dạng báo cáo).
- Kiểm tra tự động: script đọc báo cáo cuối và bảng, nếu có cụm cấm và có dòng chưa done thì báo lỗi. Đây là lint báo cáo, không phải kiểm tra ngữ nghĩa.

**File.** `catalog/skills/builtin/sdlc-squad-lead/SKILL.md`, `reference.md`, `harness-metrics.mjs` hoặc script mới.

**Tiêu chí chấp nhận.**
- Test: fixture có một issue `stage_done` và báo cáo chứa "every sub-issue has closed" → lint báo lỗi.
- Test: bảng sinh đúng số dòng và trạng thái của fixture.

**Phụ thuộc.** Không.

**Rủi ro.** Lint cụm từ có thể bỏ sót cách diễn đạt khác. Bảng sinh tự động là phòng thủ chính, lint chỉ là lớp phụ.

---

### W5. Cưỡng chế hành động không đảo ngược

**Vấn đề.** Các quy tắc "không push main ngoài QA", "không `git add -A`", "không stash/reset" là văn xuôi (S8).

**Thiết kế.** Ba cấp, theo độ mạnh tăng dần.

**W5.a Git pre-push hook trong workspace squad.** Cài bởi bước thiết lập workspace (xác định cơ chế cài cụ thể sau khi kiểm tra daemon):
- Chặn push tới `refs/heads/main` trừ khi biến `SDLC_ROLE=qa` được đặt bởi runtime cho lượt QA landing.
- Hạn chế: agent chạy `--no-verify` sẽ bỏ qua hook. Vì vậy W5.b là cần thiết.

**W5.b Chặn ở phía remote.**
- Pilot: remote là smart HTTP tự viết (`git-smart-http.py`). Thêm kiểm tra ở tầng này, từ chối push `main` nếu tiêu đề commit không có tiền tố `landing:` và `SDLC_ROLE` không được gửi qua header.
- Origin GitHub: không bật branch protection (Q2). Quy tắc "chỉ QA push" sẽ chặn luồng commit trực tiếp hiện tại của TuPK.

**W5.c Cơ chế hook của runtime (nếu có).** Kiểm tra xem runtime Claude/Kiro có hook kiểu PreToolUse để chặn lệnh `git add -A`, `git push origin main`, `git reset --hard`. Nếu có, cài trong template của squad. Nếu không, ghi rõ là không có và giữ W5.a/b.

**W5.d Kiểm tra việc add.** Pre-commit hook kiểm tra file staged nằm trong phạm vi của issue (danh sách path trong description). Không chặn `git add -A` ở bước add, nhưng chặn ở commit nếu có file ngoài phạm vi.

**File.** Script cài hook (mới, vị trí do W5.c quyết định), `scripts/` của pilot (`git-smart-http.py` ngoài repo multica), `sdlc-delivery-core/SKILL.md` (cập nhật mục đã có).

**Tiêu chí chấp nhận.**
- Test trên remote pilot: push `main` từ role dev → bị từ chối; từ QA landing → thành công.
- Test pre-push: `SDLC_ROLE` không đặt → exit khác 0.
- Test: `--no-verify` không vượt được W5.b.

**Phụ thuộc.** W0. W5.b chỉ áp dụng cho remote pilot khi dựng lại.

**Rủi ro.** Chặn quá tay làm QA không landing được. Có bước rollback: gỡ hook bằng một lệnh. Thử trên pilot trước.

---

### W6. Bài học có cấu trúc trong Known pitfalls

**Vấn đề.** Pitfall chỉ có evidence. Không biết bài học còn đúng không, áp dụng cho bao nhiêu repo, có cần kiểm lại khi copy sang repo khác không.

**Thiết kế.** Mỗi pitfall có các trường (dùng dòng `- **field**: giá trị` như thư viện của harness2, để máy đọc được):
- `cue`: tình huống nhận biết (bắt buộc).
- `lesson`: việc phải làm (bắt buộc).
- `evidence`: lệnh và exit code, hoặc id issue/commit (bắt buộc, đã có ở H12).
- `confidence`: `high | medium | low` (bắt buộc từ bản mới).
- `scope`: `repo | stack | workspace-fact` (bắt buộc).
- `valid_until`: commit hoặc ngày (tuỳ chọn).

Quy tắc:
- Khi copy harness sang repo khác: pitfall `scope: workspace-fact` và `scope: stack` phải được đánh dấu `re-verify` cho đến khi chạy lại được.
- H12 mở rộng: thiếu trường bắt buộc → warning; `valid_until` đã qua → warning.
- Thư viện chỉ mục theo domain: **không làm bây giờ** (số repo còn ít), ghi vào mục "để sau".

**File.** `check-harness.mjs` (H12), `agents-md-template.md`, SKILL của harness (mục "Content rules" và Mode 3).

**Tiêu chí chấp nhận.**
- Test: pitfall thiếu `confidence` → H12 warning; có đủ → pass.
- Test: `valid_until` đã qua → warning có ngày.

**Phụ thuộc.** W8 (định dạng thông báo).

**Rủi ro.** Tăng độ dài AGENTS.md. Giảm bằng cách trường tuỳ chọn không bắt buộc ghi trên dòng đầu, và ngân sách footprint của W7 vẫn áp dụng.

---

### W7. Chi phí prompt luôn bật và trigger

**Vấn đề.** Prose luôn bật tốn ngân sách trên mọi run. Skill có thể không được nhắc tên, nên không bao giờ được gọi.

**Thiết kế.**
- **Ngân sách theo section:** `check-harness` đếm dòng và byte của từng section (Verify, Rules, Known pitfalls, Read when). Cảnh báo khi Rules + Pitfalls vượt 60% ngân sách footprint; gợi ý chuyển một quy tắc sang check (lint, test) hoặc sang skill có trigger.
- **H18 (warning) trigger check:** mọi skill nội bộ hoặc runbook trong `docs/` được AGENTS.md hoặc skill khác nhắc đến phải nằm trong `Read when` với điều kiện. Skill không được nhắc tên → warning "skill này sẽ không bao giờ được gọi".
- Đây là phiên bản đơn giản của quy tắc "trigger line" của harness2. Không cần cơ chế chèn tự động.

**File.** `check-harness.mjs` (H18, số liệu theo section), SKILL của harness.

**Tiêu chí chấp nhận.**
- Test: skill không được nhắc → H18 warning.
- Test: section vượt ngưỡng → cảnh báo có gợi ý.

**Phụ thuộc.** W8.

**Rủi ro.** Thêm warning làm nhiễu. Giữ H18 là warning, không phải cert.

---

### W8. Thông báo lỗi có hướng dẫn sửa

**Vấn đề.** `details` của check là chuỗi tự do. Agent khó tự sửa.

**Thiết kế.**
- Chuẩn hoá mỗi kết quả check thành `{what, why, fix}`:
  - `what`: cái gì sai, kèm đường dẫn và dòng.
  - `why`: vì sao quy tắc tồn tại.
  - `fix`: việc cụ thể để sửa (ví dụ "Tạo `docs/x.md` hoặc xoá dòng nêu nó ở dòng N").
- Áp dụng cho `check-harness --quick` (Developer đọc khi bị chặn) và cho lint rule do Developer tạo (Mode 3 mục 4): bắt buộc có ba trường.
- Với Architect khi chạy `check-harness --cert` và bị trả về: đưa nguyên văn `fix` vào lượt retry.

**File.** `check-harness.mjs` (định dạng đầu ra, giữ nguyên trường `checks` để tương thích), `harness-metrics.mjs` không đổi.

**Tiêu chí chấp nhận.**
- Snapshot test: mọi check fail có đủ ba trường.
- Test tương thích: đầu ra JSON cũ vẫn đọc được (có `details` vẫn tồn tại).

**Phụ thuộc.** Không.

**Rủi ro.** Đổi định dạng làm vỡ người đọc cũ. Giữ `details` và thêm trường mới.

---

### W9. Hình dạng sản phẩm không đổi

**Vấn đề.** Harness hoặc Developer có thể thêm cột, trường, đổi tên, hay đổi thứ tự mà request đã đặt tên. Đây là lỗi cho người nhận (harness2 gọi là "deliverable shape belongs to the request").

**Thiết kế.**
- Thêm vào `sdlc-delivery-core` và SKILL của Architect (Mode 1, Mode 3): "Harness không được yêu cầu thay đổi cấu trúc sản phẩm mà request đã đặt tên (cột, trường, mục, thứ tự, tên). Nếu cần, ghi rõ trong báo cáo là giả định và chờ QA."
- QA thêm một mục vào checklist: so sánh cấu trúc đầu ra với đặc tả. `enforced_by: prose + QA`.
- Không có check tự động ở giai đoạn này (không biết cấu trúc là gì nếu không có đặc tả máy đọc).

**File.** `sdlc-delivery-core/SKILL.md`, `sdlc-harness/SKILL.md`, `sdlc-qa-verification/SKILL.md`.

**Tiêu chí chấp nhận.** Kiểm tra bằng đọc: mục xuất hiện đúng trong ba file. Trong pilot, một request có ràng buộc cấu trúc không bị đổi cấu trúc (xác nhận khi chạy M2).

**Phụ thuộc.** Không.

**Rủi ro.** Chỉ là quy tắc văn xuôi. Ghi rõ là "prose", và đưa vào check nếu bị vi phạm hai lần.

---

### W10. Đường grader cho S9

**Vấn đề.** S10: chỉ 1/3 tác vụ chấm được tất định. Không có grader thì không chạy được 225 lượt.

**Thiết kế.** Làm nhánh A trước; nhánh B khi đủ điều kiện (Q3).

**Nhánh A: tất định.** Với mỗi tác vụ có test:
- Test chạy trên commit gốc phải FAIL, trên commit sửa phải PASS. Chỉ dùng tác vụ đạt điều kiện này.
- Đầu vào là snapshot của W1.a.
- Đếm được ít tác vụ. Cần kiểm tra thêm các commit khác trong repo tupk và pilot S8 để có đủ ba tác vụ.

**Nhánh B: rubric với judge LLM.**
- Judge cố định (model và phiên bản ghi trong `docs/engineering/`). Đổi judge là đổi phép đo.
- Firewall W1: judge chỉ nhìn đầu ra và trajectory, không nhìn đáp án.
- Hiệu chuẩn: lấy 10 kết quả đã biết (người chấm trước), đo tỉ lệ đồng thuận giữa judge và người. Ngưỡng 80% (Q3).
- Dùng khi tác vụ không có test tất định.

**Ngân sách.** Giữ nguyên nguyên tắc: chạy thử một ô trước (3 tác vụ, 1 biến thể, 1 lượt, tối đa 3 lượt squad). Chưa chạy 225 lượt cho đến khi grader đã hiệu chuẩn.

**File.** `docs/engineering/sdlc-harness-eval.md` (ghi kết quả, thêm mục mới, không sửa mục cũ), script grader (mới, vị trí theo Q1).

**Tiêu chí chấp nhận.**
- Với nhánh A: mỗi tác vụ có một bản ghi FAIL trên gốc và PASS trên sửa.
- Với nhánh B: bảng đồng thuận có ít nhất 10 mẫu và tỉ lệ được báo cáo.

**Phụ thuộc.** W1 (snapshot và firewall), W2 (fingerprint cho từng biến thể), W3 (ungraded tách riêng).

**Rủi ro.** Nhánh B có thiên lệch của LLM judge. Báo cáo tỉ lệ đồng thuận thay vì coi đó là sự thật.

---

### W11. Lỗi `in_review` không đánh thức Reviewer (platform)

**Vấn đề.** Issue giao cho squad chuyển sang `in_review` nhưng không có lượt Reviewer (S7). Cần điều tra trong server Go.

**Điểm bắt đầu đã biết.** `server/internal/handler/issue_workflow_entry.go`, hàm `applyIssueWorkflow`: handoff bị bỏ qua khi `assigneeTouched` hoặc có `triage_state` (dòng 54). Issue giao cho squad không được đổi người giao sang handler của bước. DEV-18 (giao cho agent) thì được. Chưa biết cơ chế enqueue khi vào `in_review` có phụ thuộc người giao không.

**Thiết kế.**
1. **Tái hiện:** test Go (có DB) với fixture: issue giao squad, chuyển `in_review` → kỳ vọng có task cho handler của `in_review`. Chạy đúng cách AGENTS.md quy định (`make test`, dùng `testutil`, không dùng DB `multica`).
2. **Phân tích:** xác định điểm quyết định ai được enqueue. Ghi kết quả vào tài liệu trước khi sửa.
3. **Sửa:** tối thiểu. Hai hướng: (a) đổi người giao sang handler của bước như DEV-18; (b) enqueue handler của bước ngay cả khi người giao là squad. Chọn (b) nếu tái hiện thất bại (Q5).
4. **Kiểm tra:** test tái hiện pass; `make test`; chạy lại một request nhỏ trên pilot.

**File.** `server/internal/handler/issue_workflow_entry.go`, `server/internal/issueworkflow` (kiểm tra), test Go mới.

**Tiêu chí chấp nhận.** Test tái hiện pass; pilot không còn cần rerun thủ công cho `in_review`.

**Phụ thuộc.** Không. Độc lập với phần skill.

**Rủi ro.** Cao. Đổi lõi workflow ảnh hưởng mọi squad. Bắt buộc có test trước khi sửa, và chạy trên pilot trước khi đưa vào môi trường khác. Đây là điểm dừng duy nhất cần bạn duyệt trước khi sửa (Q5).

---

## 3. Thứ tự thực hiện và cổng kiểm tra

| Bước | Việc | Cổng kiểm tra |
|---|---|---|
| 1 | W0 | `pnpm catalog:check`; grep đường cũ |
| 2 | W1, W2 song song | Test snapshot, H17, fingerprint |
| 3 | W3 | Test M1, M8, M9, ungraded, identity |
| 4 | W4, W5 song song | Lint báo cáo; test remote pilot |
| 5 | W6, W7, W8, W9 | Test H12, H18, định dạng; đọc tài liệu |
| 6 | W10 | Nhánh đã chọn; ô thử 3 lượt |
| 7 | W11 | Test Go tái hiện; `make test` |

**Cổng chung cho mọi workstream có thay đổi code:**
- `pnpm catalog:check` và `pnpm catalog:build`.
- Bộ test liên quan: `packages/core/squads/sdlc-harness-*.test.ts` (chạy vitest của `packages/core`).
- Với thay đổi Go: `make test` (không chạy trong DB `multica`).
- Với thay đổi skill: `node scripts/upgrade-sdlc-squad.mjs` dry-run trên pilot, sau đó `--apply`, rồi xác nhận bằng `multica skill get` rằng `content_hash` khớp file nguồn.
- Với thay đổi có ảnh hưởng hành vi: một request nhỏ trên pilot, ngân sách không quá 25 lượt (pilot đang ở 64 trên trần 120).

**Quy tắc commit và PR** (theo `AGENTS.md`):
- Mỗi workstream một hoặc nhiều commit conventional, không gộp.
- PR theo mẫu: Problem, Goal, Proposal, Alternatives (tuỳ chọn), Screenshots. Các thay đổi này không có UI, nên ghi rõ "không có giao diện".
- PR mở vào `TuPK/multica`, không mở vào upstream (theo ghi nhớ của người dùng).
- Tài liệu kế hoạch được commit lên main theo yêu cầu. Code commit theo từng workstream.

---

## 4. Rủi ro tổng thể và cách rollback

| Rủi ro | Mức | Giảm thiểu | Rollback |
|---|---|---|---|
| Hook chặn nhầm QA landing (W5) | Trung bình | Thử trên pilot trước | Gỡ hook bằng một lệnh; khôi phục quy tắc văn xuôi |
| H17 báo động giả (W1) | Thấp | Danh sách ngoại lệ có người duyệt | Tắt H17 bằng `--accept`, ghi lý do |
| Đổi định nghĩa M3 làm số liệu cũ không so sánh được (W3) | Trung bình | `metric_version` trong output | Chạy lại với phiên bản cũ |
| Sửa workflow làm vỡ squad khác (W11) | Cao | Test tái hiện, pilot trước | Revert commit; giữ phương án rerun thủ công |
| LLM judge thiên lệch (W10) | Cao | Đo đồng thuận với người | Không dùng kết quả nhánh B cho quyết định |
| AGENTS.md dài thêm (W6) | Thấp | Ngân sách footprint (W7) | Xoá trường tuỳ chọn |

---

## 5. Quyết định đã chọn

> Các lựa chọn dưới đây do tôi chọn dựa trên hiện trạng (2026-11-15). Căn cứ ở mục 7.

- **Q1. Vị trí script và bộ tác vụ S9.** Đặt `make-snapshot.mjs` và `grade-task.mjs` trong `scripts/eval/`. Bộ tác vụ S9 mới trên repo greenfield nhỏ do ta kiểm soát, mỗi tác vụ có test tất định. Không dùng 22 tác vụ tupk, vì đã mất (H-12).
- **Q2. Branch protection trên origin GitHub (W5.b).** Không bật. Thay bằng pre-push hook trong workspace squad (W5.a) và kiểm tra ở remote pilot khi dựng lại. Cài đặt GitHub nằm ngoài repo, nên không tự làm.
- **Q3. Đường grader S9 (W10).** Nhánh A trước: tác vụ tự viết, test FAIL trên base và PASS trên bản sửa. Nhánh B (judge rubric) chỉ dùng khi có ít nhất 10 kết quả do người chấm. Ngưỡng đồng thuận 80%, áp dụng cho nhánh B.
- **Q4. Nơi lưu fingerprint (W2).** Không ghi vào git. Fingerprint = `content_hash` của từng skill từ nền tảng + sha của AGENTS.md, ghi trong comment `Harness: <fingerprint>` đầu tiên của Lead.
- **Q5. Hướng sửa W11.** Tái hiện trên HEAD trước. Nếu thất bại, chọn (b) enqueue handler của bước. Đây là điểm dừng duy nhất cần bạn duyệt trước khi sửa lõi workflow.
- **Q6. Mức H17, H18.** H17 là warning trong chu kỳ đầu, lên cert sau một chu kỳ. H18 giữ warning. Khi nâng H17, cập nhật cùng lúc số check và tập cert trong test (H-7, H-8).

---

## 6. Phụ lục: những gì đã làm và trạng thái

| Mục | Trạng thái | Ghi chú |
|---|---|---|
| Fix Lead không giao việc cho request | Đã có trong nguồn catalog | Đã grep xác nhận; commit `898f60350` ghi vào đường cũ |
| Round 5 (DEV-22) đạt `plan_ready` | Đã xác nhận | Có một lần rerun thủ công DEV-23 do S7 |
| Bảng số liệu của pilot | Đã có M2–M7 | M1, M8, M9 chưa có (S4) |
| Grader S9 | Chưa có | S10 |
| Đường đi build skill | Đổi sang `pnpm catalog:build` | S1 |

Tài liệu này được commit lên main. Chưa có code nào được viết. Thứ tự thực hiện ở mục 3, bắt đầu bằng W0.

---

## 7. Rà soát với hiện trạng (2026-11-15) và trả lời Q1–Q6

### 7.1 Hiện trạng đã kiểm chứng (chỉ đọc)

| # | Phát hiện | Căn cứ | Tác động lên kế hoạch |
|---|---|---|---|
| H-1 | Repo đang có nhiều thay đổi chưa commit của người khác (server auth, packages/core/api, locales, docs platform). | `git status` | Không chạm các file đó. Mọi commit sau này stage theo đường dẫn cụ thể. |
| H-2 | Dev server đang có phiên kiro chạy (thư mục `20261115-*` trong `~/.cache/claude-delegate`: `fix-round`, `second-pass`, `pass3-kiro`). Các phiên này sửa `server/internal/handler/flow_*` và `catalog/workflows/flow_kv_*`. | Brief của các phiên | Không sửa flow engine, `flow_sweep`, `flow_kv_*`. Test DB chạy trên DB riêng. |
| H-3 | Môi trường pilot S8 không còn dùng được. API 18800 và web 13720 không phản hồi. Không có systemd user bus nên không quản lý được `s8-daemon` và `s8-web`. Các thư mục `s8-remotes`, `s8-driver`, `multica-s8`, `s9-work` đã mất. Chỉ còn tiến trình `git-smart-http.py` (pid 3092556) và profile `dev-multica_s8-720`. | `curl`, `systemctl --user`, `ls ~/.cache/claude-delegate` | W1, W5, W10 không kiểm chứng được trên pilot cũ. Phải dựng lại môi trường test (7.3). |
| H-4 | Server của bạn (8080) còn sống, trả 404 ở `/`. | `curl` | Không thao tác lên server này. |
| H-5 | Engine workflow đã đổi lớn ngày 12–13/11: transitions có điều kiện, squad có active workflow, enforce flow trên request (`2e4fafc3d`, `a2e80b629`, `98ff66641`). Round 4 chạy trên snapshot cũ hơn các commit này. | `git log`, message commit | Gap `in_review` (S7) có thể đã được sửa. Phải tái hiện trên HEAD trước khi sửa (W11). |
| H-6 | Trong HEAD, SDLC Auto có bước `in_review` với `role: reviewer`. | `catalog/workflows/sdlc_auto/template.json` | Hành vi kỳ vọng đã định nghĩa, dùng làm oracle cho test. |
| H-7 | `sdlc-harness-check.test.ts` khẳng định đúng 16 check (dòng 132) và cert gồm H6, H7, H10, H14 (dòng 138). | Đọc file | Thêm H17/H18 làm đỏ test. Phải cập nhật kỳ vọng trong cùng thay đổi. |
| H-8 | `mua_ngon_ecommerce` đã có `AGENTS.md`. Không tìm thấy `docs/sdlc/harness-cert.json` trong độ sâu 4. | `find` | Nâng H17/H18 lên cert ngay sẽ đổi trạng thái repo đang dùng. |
| H-9 | `pnpm generate:sdlc-skills` không còn trong `package.json`. Nguồn skill là `catalog/`, build bằng `node scripts/catalog/cli.mjs build`. | `grep package.json` | Sửa W0 và các bước trước đó. |
| H-10 | Có `scripts/require-test-database.sh`. AGENTS.md yêu cầu test DB-backed không dùng DB `multica`. | `ls scripts` | Mọi test Go của W11 đi qua script này. |
| H-11 | Kiểm tra catalog hiện tại: `node scripts/catalog/cli.mjs check` exit 0, 206 template, có cảnh báo về giấy phép vendored skill (không chặn). | Chạy chỉ đọc | Điểm xuất phát của W0 đã xác nhận. |
| H-12 | Bộ 22 tác vụ tupk của S9 không còn trên đĩa (`s9-work` mất). | `ls` | Q1 và W10 phải chọn lại tác vụ. |

### 7.2 Căn cứ cho các lựa chọn

**Q1. Vị trí script snapshot và grader; có chuyển S9 sang bộ tác vụ khác không?**

- Vị trí: `scripts/eval/` (chưa có, không trùng tên). Đặt `make-snapshot.mjs` và `grade-task.mjs` ở đây.
- Test dùng repo fixture tạo trong thư mục tạm, không phụ thuộc pilot (H-3).
- S9: không dùng 22 tác vụ tupk (H-12). Đề xuất bộ tác vụ mới trên repo greenfield nhỏ do ta kiểm soát, mỗi tác vụ có test tất định. Đây là thay đổi phạm vi S9 (đã chốt).

**Q2. Có bật branch protection trên `TuPK/multica` để chỉ QA push `main` không?**

- Đề xuất: không bật. Lịch sử cho thấy `main` nhận commit trực tiếp từ TuPK, đó là luồng hiện tại. Quy tắc "chỉ QA" sẽ chặn chính bạn. Hiện không có danh tính QA riêng để GitHub phân biệt nếu squad dùng chung token.
- Thay thế: pre-push hook trong workspace squad, và kiểm tra ở remote pilot khi dựng lại (W5.a, W5.b).
- Đây là cài đặt trên GitHub, nằm ngoài repo. Không làm nếu bạn chưa yêu cầu rõ.

**Q3. Đường grader cho S9 và ngưỡng đồng thuận.**

- Đề xuất: làm nhánh A trước. Tự viết tác vụ có test; test phải FAIL trên base và PASS trên bản sửa. Đây là grader đáng tin nhất và rẻ nhất.
- Nhánh B (judge rubric) để sau, chỉ dùng khi đã có ít nhất 10 kết quả do người chấm.
- Ngưỡng 80% đã chốt, áp dụng cho nhánh B. Trước khi có ít nhất 10 mẫu do người chấm, không báo cáo tỉ lệ đồng thuận.
- Ngân sách 3 lượt thử giữ nguyên.

**Q4. Nơi lưu manifest fingerprint.**

- Đề xuất: không tạo file manifest trong git. Workspace squad commit đường dẫn riêng, một file chung sẽ xung đột.
- Thay thế: lấy `content_hash` của từng skill từ nền tảng (`multica skill get` đã trả trường này) cộng sha commit của AGENTS.md. Lead ghi comment `Harness: <fingerprint>` đầu tiên. `harness-metrics` đọc comment đó.

**Q5. Hướng sửa W11 và có duyệt việc chạm lõi workflow không.**

- Đề xuất: chưa sửa code. Trước hết tái hiện trên HEAD (H-5, H-6). Gap được quan sát trên snapshot cũ; engine đã đổi sau đó; pilot đã mất; và code này đang có thay đổi chưa commit của người khác.
- Bước tái hiện: test Go dùng `scripts/require-test-database.sh`, fixture là issue giao cho squad có active workflow, chuyển sang `in_review`, kỳ vọng có task cho reviewer. Điểm kiểm: `applyIssueWorkflow` ở `server/internal/handler/issue_workflow_entry.go` dòng 54 và 110 (`assigneeTouched || prev.TriageState.Valid`).
- Nếu tái hiện thất bại, chọn (b) enqueue handler của bước. Lý do: (a) đổi người giao mà người dùng đang thấy trên issue; (b) giữ nguyên người giao và chỉ thêm đường enqueue, có guard chống enqueue trùng. Đây là điểm dừng duy nhất trước khi sửa lõi workflow.

**Q6. Mức của H17 và H18.**

- Đề xuất: warning trước, cert sau một chu kỳ. Nâng lên cert ngay sẽ làm đỏ test (H-7) và đổi trạng thái repo đang dùng (H-8).
- H17 lên cert sau một chu kỳ; H18 giữ warning. Khi nâng H17, cập nhật cùng lúc số check và tập cert trong test.

### 7.3 Ràng buộc từ hiện trạng và thứ tự làm lại

1. **Dựng lại môi trường test trước khi làm W1, W5, W10.** (a) Dựng pilot mới trên DB test riêng, không dùng DB `multica`. (b) Chỉ dùng repo fixture trong test. Đề xuất (b) cho W1 và W2. Chỉ dùng (a) khi cần end-to-end, và khi bạn cho phép.
2. **Không khởi động lại** `s8-daemon`, `s8-web` hay tiến trình nào của bạn. Khôi phục pilot phải hỏi trước.
3. **Không chạm:** `server/internal/handler/flow_*`, `catalog/workflows/flow_kv_*`, `server/internal/auth/*`, `packages/views/locales/*`, `docs/engineering/platform-*`.
4. **File sẽ đổi** (hiện không có thay đổi chưa commit): `catalog/skills/builtin/sdlc-*`, `scripts/upgrade-sdlc-squad.mjs`, `packages/core/squads/sdlc-harness-*.test.ts`, `docs/engineering/sdlc-harness-*.md`.
5. **Cổng trước khi commit:** `node scripts/catalog/cli.mjs check` (đã pass, H-11); vitest cho `sdlc-harness-*.test.ts`; với Go là `scripts/require-test-database.sh` và `make test`.

### 7.4 Cập nhật các workstream theo rà soát

- **W0:** thay `pnpm generate:sdlc-skills` bằng `node scripts/catalog/cli.mjs build` (H-9).
- **W1:** thay bộ tác vụ tupk bằng bộ mới (Q1). Test dùng fixture.
- **W2:** bỏ manifest trong git. Dùng `content_hash` của nền tảng và sha AGENTS.md (Q4).
- **W5:** giữ W5.a. W5.b ở remote pilot chỉ khi dựng lại. Không bật GitHub (Q2).
- **W10:** nhánh A trước, với fixture tự viết (Q3).
- **W11:** thay phần thiết kế bằng bước tái hiện trên HEAD. Chỉ sửa khi tái hiện thất bại (Q5).
- **Mục 3:** W11 trong M5 bắt đầu bằng tái hiện, không sửa.

---

## 8. Ghi chú thực hiện (2026-11-15)

Phần này ghi kết quả khi làm theo mục 3 và mục 7. Chưa commit gì. Các thay đổi nằm trong cây làm việc để người dùng xem trước.

### 8.1 Trạng thái theo workstream

| WS | Trạng thái | Ghi chú |
|---|---|---|
| W0 | Xong | Ba tài liệu được sửa. Các tài liệu khác (`sdlc-harness.md`, `sdlc-harness-plan.md`, tài liệu sales, research-lab) vẫn có đường dẫn cũ. Chúng không nằm trong danh sách file của W0, nên không sửa. |
| W1 | Xong (a, b, c). d là ghi chú | W1.a: `scripts/eval/make-snapshot.mjs`. W1.b: H17. W1.c: quy tắc trong `sdlc-delivery-core`. |
| W2 | Xong trong code, dry-run chưa chạy | `upgrade-sdlc-squad.mjs` in bảng hash và digest khi dry-run. Không gọi máy chủ (xem 8.3). |
| W3 | Xong | M1, M3 ungraded, M3 identity, M8 (`--harness`), M9. `metric_version` 2. |
| W4 | Xong | `--status-table`, `--lint-report`, SKILL của Lead, `reference.md`. |
| W5 | Một phần | W5.a xong (guard và installer). W5.c: không có hook chặn lệnh trong runtime (xem 8.4). W5.b và W5.d chưa làm. |
| W6 | Xong | H12 kiểm trường bắt buộc và `valid_until`. Mẫu và SKILL đã cập nhật. Chỉ mục theo domain để sau, như kế hoạch. |
| W7 | Xong | Ngân sách section trong H14. H18. |
| W8 | Xong | Mỗi check lỗi có `findings` gồm `what`, `why`, `fix`. `details` giữ nguyên. |
| W9 | Xong bằng prose | Ba file theo danh sách của W9, cộng mục trong `sdlc-harness/SKILL.md`. Architect được phủ bởi SKILL harness vì mục đó là Architect. |
| W10 | Nhánh A xong | Ba tác vụ, `grade-task.mjs`. Nhánh B và các lượt chạy chưa làm. |
| W11 | Dừng sau bước tái hiện, chưa tái hiện được | Xem 8.2. Không sửa `issue_workflow_entry.go` hay lõi workflow. |

### 8.2 W11: kết quả tái hiện trên HEAD

**Kết quả: chưa tái hiện được, vì không chạy được test có DB.** Đây không phải là kết quả "không lỗi".

- Đã viết `server/internal/handler/issue_workflow_in_review_repro_test.go` với ba test: thành viên chuyển request giao squad sang `in_review`; run của leader chuyển request; thành viên chuyển issue con giao squad. Mỗi test kỳ vọng người duyệt của bước `in_review` được giao việc và có một run.
- `go vet ./internal/handler/` pass (không có đầu ra), nên file biên dịch được.
- `env -u DATABASE_URL bash scripts/require-test-database.sh` thoát 1: `DATABASE_URL is empty or unset`.
- `env -u DATABASE_URL go test -run TestRepro ./internal/handler/` thoát 1: `DATABASE_URL is empty; set it to a database whose name ends in _test`.
- Postgres lắng nghe ở `127.0.0.1:5432` và đòi mật khẩu. Mật khẩu chỉ có trong `.env` chính, và không được đọc cho việc test. Không có DB `_test` nào để dùng, và không tạo DB bằng tay. Cần người dùng cấp `DATABASE_URL` tới một DB `*_test` đã migrate.

**Phân tích từ mã (chưa kiểm chứng bằng test).** Nhánh quyết định nằm ở `issue_workflow_entry.go`:
- dòng 144: `if h.isSquadRequest(ctx, prev) && !h.isRunOfRequestLeader(ctx, prev, run) { return nil, nil }`. Một request cấp cao giao cho squad mà người khác chuyển trạng thái thì không có handoff, và không có run cho người duyệt.
- Hành vi này có chủ ý. `TestWorkflowDoesNotHandOffASquadRequest` (`issue_queue_start_status_test.go:82`) khẳng định nó với `in_progress`, và chú thích ghi "a request a squad leader is working is not a step of the workflow when someone else moves it".
- Vì vậy kỳ vọng của test thứ nhất (thành viên chuyển request giao squad sang `in_review`) mâu thuẫn với thiết kế đã có test. Nếu test này đỏ trên HEAD, đó là đúng hành vi thiết kế, không phải lỗi cần sửa ở lõi.
- Run của leader (dòng 144, `isRunOfRequestLeader` đúng) đi tiếp tới handoff. Issue con có `parent_issue_id` không bị chặn ở dòng 144. Hai trường hợp này có vẻ đúng trên HEAD, nhưng cần test chạy để xác nhận.
- Giả thuyết cho S7: request giao squad được chuyển sang `in_review` bởi run của một Developer (không phải leader). Dòng 144 chặn handoff. Giả thuyết này chưa kiểm chứng.

**Việc cần làm trước khi sửa (Q5):** chạy ba test với một DB `*_test`. Nếu test thứ nhất đỏ, cần quyết định: đổi thiết kế (cho phép handoff khi run không phải leader chuyển request sang `in_review`), hay giữ thiết kế và sửa quy trình của Lead để nó chuyển request bằng run của leader. Quyết định này thuộc về người dùng, nên dừng ở đây.

### 8.3 Những việc không chạy

- Dry-run `upgrade-sdlc-squad.mjs` cần máy chủ. Máy chủ `8080` là của người dùng, và mục 7.3 cấm thao tác trên đó. Đã kiểm `node --check` cho script.
- Kiểm `multica skill get` khớp `content_hash` với file nguồn: cần máy chủ.
- W5.b (kiểm tra ở remote pilot): pilot không còn dùng được (H-3). Phần kiểm tra remote là một file ngoài repo (`git-smart-http.py`).
- Ô thử W10 trên pilot, kiểm "≤25 lượt" của mục 3, và kiểm W11 trên pilot: cần pilot.

### 8.4 Kiểm tra runtime (W5.c)

Daemon ghi hook `PreToolUse` cho Claude qua giao thức SDK, chỉ để thêm ngữ cảnh (`claude_supplement.go`). Không có hook nào trong mã của repo chặn lệnh shell. Không có chỗ nào daemon cài `core.hooksPath` hay `.git/hooks`. Vì vậy W5.a dùng `install-guard.sh`, chạy một lần trên mỗi clone. Hook chặn lệnh của Claude qua `.claude/settings.json` không được thêm, vì chưa biết daemon khởi chạy Claude bằng cách nào.

### 8.5 Thay đổi so với kế hoạch và lý do

1. **H17 là warning, exit code 0.** Mục 3 W1.b ghi "fail, exit 1". Q6 đã chốt H17 là warning trong chu kỳ này. Kết quả: H17 hiện trong `warnings`, và `ok`/exit vẫn là 0.
2. **Số check 16 → 18** (H17, H18). Tập cert không đổi. Test `sdlc-harness-check.test.ts` và `sdlc-skills.test.ts` được cập nhật danh sách, vì các check mới làm danh sách cũ sai (Q6).
3. **Thông báo của checker bằng tiếng Anh.** Đoạn "Đây là rò rỉ, không phải báo động giả..." của W1.b được viết bằng tiếng Anh. Lý do: spec (mục ngôn ngữ) giữ văn bản dành cho agent bằng tiếng Anh, và toàn bộ thông báo của checker là tiếng Anh.
4. **Fingerprint (W2)** là `<agents blob>.<skills digest>`. Blob là `git hash-object` của AGENTS.md (checker in `agentsBlob`). Digest là sha256 của các dòng `name sha` đã sắp xếp của file nguồn skill (`upgrade-sdlc-squad` in `SKILLS DIGEST`). Dòng `Harness:` đầu tiên của Lead được đọc là giá trị. Không có manifest trong git (Q4).
5. **H12 đổi định dạng pitfall.** Mỗi pitfall có năm trường bắt buộc. Các fixture và mẫu AGENTS.md cũ được cập nhật theo.
6. **`M3_failedRuns` đổi nghĩa** (không tính run lỗi hạ tầng). Đây là lý do của `metric_version` 2 (W3).
7. **M1 và M9 dùng định nghĩa thực tế của workflow.** Không có status `harness_smoke` trong workflow SDLC Auto. M1 đếm issue đã vào status `harness`. M9 nhận diện issue smoke theo tiêu đề "Harness smoke".
8. **W5.d không làm.** Không có cơ chế nào đưa danh sách path của issue vào hook commit. Thiếu cơ chế này thì hook không biết phạm vi.
9. **W5.b không làm** (xem 8.3).
10. **Trường "tests FAIL trên base" của W10** đã đạt. Nhưng tác vụ `sum-open` đầu tiên có seed thiếu `sumOpen`. Đã thêm một stub để base fail bằng assertion, không fail do lỗi link.
11. **Test M8 và các test khác dùng repo tạm** (không dùng pilot), theo Q1 và H-3.

### 8.6 Hai file test được sửa vì thay đổi có chủ ý

- `packages/core/squads/sdlc-skills.test.ts`: danh sách file của `sdlc-harness` (thêm hai script guard) và danh sách check `warns` (thêm H17, H18).
- `packages/core/squads/sdlc-harness-check.test.ts`: danh sách ID (16 → 18), fixture pitfall theo định dạng mới, và kỳ vọng `H12` của test "flags a pitfall without an evidence reference" (dùng `toContain` vì giờ có thêm lỗi trường bắt buộc).
