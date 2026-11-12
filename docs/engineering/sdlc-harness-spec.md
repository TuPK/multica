# SDLC squad: Harness v2, hợp đồng trước

Spec thiết kế của phương án B trong [sdlc-harness-research.md](sdlc-harness-research.md). Thay thế phần thiết kế (mục 2 đến 6,
8, 9) của [sdlc-harness.md](sdlc-harness.md) khi triển khai xong; bổ sung cho [sdlc-autonomy.md](sdlc-autonomy.md) và
[sdlc-specialist-routing.md](sdlc-specialist-routing.md). Kế hoạch triển khai ở [sdlc-harness-plan.md](sdlc-harness-plan.md).

Trạng thái: **đã triển khai S0 đến S7** (xem mục 0 của plan); S8, S9 chưa làm. Mọi con số ngưỡng ghi "tạm thời" là giả thuyết cần hiệu chỉnh (mục 10).

Ngôn ngữ: viết bằng tiếng Việt để người dùng đọc; tên lệnh, status, file, tên kiểm tra, và đoạn văn bản dành cho agent giữ
nguyên tiếng Anh, vì skill và `AGENTS.md` của agent viết bằng tiếng Anh.

## 1. Tóm tắt

Harness v2 gồm bốn phần:

1. **Hợp đồng bằng mã.** Hai lệnh chạy được: `Ensure` (dựng và kiểm tra môi trường) và `Done` (phép kiểm chứng của dự án). Được
   chứng minh bằng cách **chạy thật**, không chỉ đọc văn bản.
2. **`AGENTS.md` tối thiểu.** Chỉ Rules (có `Enforced by`), Known pitfalls (có bằng chứng) và con trỏ có điều kiện. Không tổng quan,
   không trạng thái, không chép lệnh.
3. **Kiểm tra ba mức.** `gate` chỉ chặn khi harness vắng hoặc nói dối; `cert` là điều kiện chứng nhận stage Harness; `warning` còn lại.
4. **Đo được.** Chỉ số lấy từ dữ liệu sẵn có của nền tảng, bộ eval nhỏ lấy từ lỗi thật, và quy tắc quyết định đặt trước.

Giữ nguyên: stage `Harness` sau `Environment`, Architect sở hữu, `CLAUDE.md` = `@AGENTS.md`, H3/H4, dòng `Ensure command:`, dòng
`Done = ...`, trigger làm mới, hạ bậc có ghi chép, không đổi server, không đổi status workflow, không đổi brief của stage.

| # | Quyết định | Thay đổi so với v1 |
| --- | --- | --- |
| D1 | Bốn loại nội dung có nơi cư trú riêng (mục 4) | Mới. v1 trộn mọi thứ trong `AGENTS.md` |
| D2 | `AGENTS.md` v2: Verify, Rules, Known pitfalls, Read when; mục tiêu 80 dòng | Bỏ định hướng, bảng "Where things are", provenance trong file |
| D3 | Nội dung chỉ lấy từ thứ Architect đã chạy và đã vấp; mỗi dòng có bằng chứng | Mới (v1 để Architect viết từ mẫu) |
| D4 | Ba mức `gate` / `cert` / `warning`; Developer chỉ bị `gate` chặn | v1: tám kiểm tra chặn đều chặn Developer |
| D5 | Kiểm tra thực thi `--run`: E1 (Ensure), E2 (Done) | Mới |
| D6 | Chứng nhận `docs/sdlc/harness-cert.json` mang provenance | v1: dòng `Verified` trong AGENTS.md và `harness-cert.md` không ai đọc |
| D7 | Smoke có bước làm hỏng cố ý | Mới |
| D8 | Luật tăng trưởng, cắt tỉa, nâng luật lên mã | Mới |
| D9 | Chỉ số M1 đến M9, eval pack, quy tắc quyết định đặt trước | Mới |
| D10 | Squad `bmad_planning` không viết `AGENTS.md` | v1 bước 7 của `planning-architecture` bắt viết |

## 2. Mục tiêu và không mục tiêu

Mục tiêu:

- G1. Agent mới vào dự án dựng được môi trường và chạy được phép kiểm chứng bằng lệnh ghi trong repo, không phải đoán.
- G2. Không harness nào dẫn agent đi sai: đường dẫn hay lệnh được nêu phải có thật (`gate`).
- G3. Ngữ cảnh nạp trước nhỏ nhất mà vẫn chứa sự kiện vận hành agent không tự suy ra được.
- G4. Luật không có ngoại lệ có nơi cưỡng chế bằng mã, CI hoặc hook; luật chỉ còn trong văn xuôi bị đánh dấu `prose` và đếm.
- G5. Hiệu quả của harness đo được bằng dữ liệu sẵn có; có quy tắc quyết định đặt trước để giữ, rút gọn hay bỏ từng phần.
- G6. Chạy giống nhau ở mọi runtime của daemon: dựa vào `AGENTS.md`, lệnh trong repo, CI và git.
- G7. Không đổi server, không đổi workflow status, không đổi brief của stage.

Không mục tiêu:

- Chứng minh `AGENTS.md` làm tăng tỉ lệ thành công. Nghiên cứu cho thấy chưa chứng minh được (research mục 7.4); spec này chỉ
  chứa quy trình đo.
- Sinh tài liệu kiến trúc, đặc tả sản phẩm hay kế hoạch. Đó là artifact planning.
- Viết hạ tầng. Lệnh `Ensure` là việc của DevOps; Architect báo `tooling` khi thiếu.
- Cài hook mặc định (Claude Code `Stop`, git hook). Chỉ là bộ chuyển tùy chọn (mục 5.9), sau khi có số đo.
- Chặn ở tầng engine (`todo` sang `in_progress`). Vẫn chưa đề xuất.
- Áp dụng cho mua_ngon_ecommerce hay sửa dự án cụ thể. Việc đó đi qua chính stage Harness sau khi spec này được triển khai.
- Squad Kit Việt và các template khác (Sales, Planning) ngoài bước 7 của `planning-architecture`.

## 3. Đối tượng

| Đối tượng | Khi nào | Hành vi v2 |
| --- | --- | --- |
| **Greenfield** (squad SDLC tạo repo mới hoặc repo gần trống) | Chuỗi `Environment` → `Harness` → `Harness smoke` → Epic 1 | Lead chạy `check-harness`. Chưa có `AGENTS.md` thì `gate` fail, tạo issue Harness. Architect chạy Mode 1 (mục 8.2) cho tới khi `certified` và có `harness-cert.json`. Repo trống được miễn mục "Read when" nhưng vẫn phải có Ensure và Done. |
| **Maintenance** (`sdlc_maintenance`, Route 0) | Repo đã có mã, có thể có `AGENTS.md` do người viết | Chỉ kiểm tra, không viết lại. `gate` fail: tạo issue Harness chạy trước, chặn Developer. Chỉ `cert` hoặc `warning` fail: tạo issue Harness (Mode 3) xếp hàng, **không chặn** Developer. Áp "ledger rule": không xóa chỉ dẫn của người chủ repo trừ khi có đủ căn cứ; sửa lệnh/đường dẫn sai và thêm hai dòng Ensure/Done là việc được phép. |
| **Planning squad** (`bmad_planning`) | Bước 7 của `planning-architecture` | Không viết `AGENTS.md`. Để lại trong `architecture.md` hai mục: "Non-default conventions" và "Verification strategy". Stage Harness của SDLC đọc hai mục đó. |
| **Template** (`templates.ts`, `initiative_build.go`) | Tạo squad mới và nâng cấp squad cũ | Không đổi step, status hay brief. Nội dung đổi nằm trong skill `sdlc-harness` và các skill role; squad cũ nhận qua `node scripts/upgrade-sdlc-squad.mjs --apply` (`importBuiltinSkills` gọi `updateSkill` khi nội dung khác). |

## 4. Bốn loại nội dung

Điểm yếu cốt lõi của v1: luật, sự kiện, kiến thức và trạng thái cùng nằm trong `AGENTS.md`, nên file phình và mục nát (research
mục 8.3 dòng 9). v2 tách thành các loại, mỗi loại một nơi cư trú.

| Loại | Là gì | Ví dụ | Nơi cư trú | Nạp | Kiểm bằng máy |
| --- | --- | --- | --- | --- | --- |
| **Contract** | Lệnh agent phải chạy | Ensure, Quick, Done | `AGENTS.md` mục Verify, một dòng mỗi lệnh; thân lệnh nằm ở manifest hoặc script | Luôn | H3, H4, H6, H10, E1, E2 |
| **Rule** | "Không được X, hãy làm Y"; vi phạm là lỗi | "Never call fetch from a component; use `src/lib/api.ts`" | `AGENTS.md` mục Rules, mỗi dòng có `Enforced by:` | Luôn | H8 (đích `Enforced by` có thật), đếm `proseRules` |
| **Gotcha** | Sự kiện vận hành agent không tự suy ra được từ code và đã vấp | "Bộ test đầy đủ mất hơn 20 phút; dùng Quick khi sửa" | `AGENTS.md` mục Known pitfalls, kèm id bằng chứng | Luôn | H12 |
| **Knowledge** | Kiến thức về repo: kiến trúc, miền, quy ước, lịch sử quyết định | `architecture.md`, `epics.md`, ADR | `docs/` | Theo nhu cầu, qua con trỏ "Read when" | H3 (đường dẫn có thật), H15 |
| **Procedure** | Cách làm một việc nhiều bước | Quy trình phát hành, runbook sự cố | Skill hoặc `docs/` runbook | Khi gọi | n/a |
| **State** | Tiến độ, ruling, assumption, điều kiện phát hành | `docs/sdlc/rulings.md`, `assumptions.md`, issue tracker | `docs/sdlc/`, nền tảng | Không bao giờ trong `AGENTS.md` | H7 |

Ba câu hỏi phân loại, hỏi theo thứ tự:

1. Agent có thể tự biết điều này bằng cách đọc code, manifest hoặc README không? **Có** thì không đưa vào `AGENTS.md`
   (nó là Knowledge, nếu cần thì trỏ tới tài liệu).
2. Vi phạm có phải là lỗi mà một công cụ hoặc người review sẽ từ chối không? **Có** thì là **Rule**, và phải nêu công cụ cưỡng chế
   (`Enforced by`), hoặc ghi `prose` để được đếm và đưa vào danh sách nâng thành mã.
3. Agent đã vấp (hoặc Architect đã vấp lúc dựng) và hậu quả là lãng phí lượt chạy? **Có** thì là **Gotcha** kèm bằng chứng.

Không thuộc ba loại trên thì không vào `AGENTS.md`.

## 5. Artifact của harness

| # | Artifact | Định nghĩa | Chủ | Ghi/đọc | Kiểm |
| --- | --- | --- | --- | --- | --- |
| 5.1 | `AGENTS.md` | File ngữ cảnh nạp trước của repo. Cấu trúc ở mục 6.2. | Architect | Mọi agent đọc | H1 và các kiểm tra nội dung |
| 5.2 | `CLAUDE.md` | Đúng `@AGENTS.md`, tối đa một câu thêm. Cần vì Claude Code chỉ đọc `CLAUDE.md` khi hai file cùng có (research A1). | Architect | Claude Code | H1 |
| 5.3 | `docs/sdlc/environment.md` | Mô tả dựng môi trường. Chứa đúng một dòng ``- Ensure command: `<lệnh>` ``. | DevOps | Architect, Developer khi lỗi môi trường | H10, E1 |
| 5.4 | Lệnh Done | Một lệnh có thật trong manifest hoặc script, thoát khác 0 khi bất kỳ phép kiểm tra nào của dự án hỏng. Tên khuyến nghị `verify` (`pnpm verify`, `make verify` hoặc `./scripts/verify`) và có biến thể nhanh. Gộp những gì repo đã có (typecheck, lint, test); không thêm công cụ mới. | Architect | Mọi vai đổi mã; Reviewer, QA | H6, E2 |
| 5.5 | `docs/sdlc/harness-cert.json` | Chứng nhận machine-readable (phụ lục B). Mang provenance, kết quả thực thi, footprint. | Architect (qua `--write-cert`) | Lead, `--refresh` | H2 |
| 5.6 | `docs/sdlc/harness.json` | Cấu hình tùy chọn: `ignorePaths`, `profile`, `timeoutSeconds`. | Architect | `check-harness` | n/a |
| 5.7 | `docs/sdlc/assumptions.md` | Giữ nguyên (đã có). Mỗi lần hạ bậc `cert` ghi tên kiểm tra và lý do. | Mọi vai | Lead | n/a |
| 5.8 | Issue Harness smoke | Issue Developer; mục 8.4. | Lead tạo | Developer | M9 |
| 5.9 | Bộ chuyển cưỡng chế (tùy chọn, **không cài mặc định**) | (a) job CI chạy lệnh Done; (b) git hook `pre-commit` chạy biến thể nhanh, cài qua lệnh Ensure; (c) hook `Stop` của Claude Code chạy biến thể nhanh. Chỉ (a) dùng cho mọi runtime. | Architect, sau S8 | Mọi agent | n/a |

**Không tạo** (đã cân nhắc và loại): file tiến độ kiểu `claude-progress.txt` và danh sách tính năng JSON (trùng issue, comment và git
của nền tảng; research mục 5.4); cây `docs/design-docs`, `exec-plans`, `QUALITY_SCORE` (quá nặng, chưa có đo đối chứng); file
`harness-cert.md` của v1 (không ai đọc).

Profile (`references/profile-web.md`, `profile-backend.md`, `profile-baas.md`, `profile-mobile.md`) đổi vai trò: từ "nội dung
thêm vào mẫu" thành **danh sách thứ cần thăm dò** (ví dụ ngữ cảnh an toàn của trình duyệt trên HTTP qua LAN, tách biến công khai
và riêng tư). Kết quả thăm dò chỉ vào `AGENTS.md` nếu agent thật sự vấp và có bằng chứng.

## 6. Luật nội dung `AGENTS.md`

### 6.1 Các luật

| ID | Luật | Kiểm bằng máy |
| --- | --- | --- |
| R1 | Chỉ bốn lớp: Contract (mục Verify), Rules, Known pitfalls, Read when. Mọi thứ khác bị loại. | H16 (cảnh báo) |
| R2 | **Không trạng thái sản phẩm.** Không "implemented", "scaffold", "TODO", "WIP", dấu tick, ô bảng `done`. Mục có tiêu đề Greenfield được miễn. | H7 (cert) |
| R3 | **Không chép lệnh.** Chỉ tên lệnh gọi (`pnpm verify`), tối đa một lệnh mỗi mục đích. Không chép thân script, danh sách cờ, chuỗi lệnh nhiều dòng; nguồn sự thật là manifest hoặc script. Lệnh viết phải có thật. | H3, H4 (gate); E1, E2 |
| R4 | **Con trỏ có điều kiện.** Mỗi con trỏ tới tài liệu có dạng "<điều kiện> → đọc <đường dẫn>" (ví dụ ``Changing the API client: read `docs/planning-artifacts/architecture.md` `` ). Không con trỏ trần. Đường dẫn phải có thật. | H3 (gate), H15 |
| R5 | **Bài kiểm tra xóa.** Mỗi dòng phải trả lời được: nếu xóa dòng này, agent có mắc lỗi nào không? Không thì xóa. | Báo cáo của Architect |
| R6 | **Gotcha có bằng chứng.** Mỗi dòng Known pitfalls kèm id bằng chứng (ruling, issue, lệnh và mã thoát). Mô tả điều gì sai và làm gì thay thế. | H12 |
| R7 | **Không lặp lại thứ công cụ đã cưỡng chế.** Quy tắc định dạng, đặt tên, import mà linter/formatter/CI đã bắt thì không viết. Cần thì viết một dòng Rule trỏ tên công cụ. | Báo cáo của Architect |
| R8 | **Không tổng quan.** Không mô tả dự án, danh sách công nghệ, cây thư mục, bảng "Where things are". Ngoại lệ hẹp: repo không có README hay tài liệu kiến trúc nào thì tối đa ba dòng định hướng kèm con trỏ tới tài liệu sẽ có. | H16 |
| R9 | **Mỗi Rule có `Enforced by:`** nêu lint rule, test, job CI, hook, hoặc `prose`. Luật `prose` bị đếm và đưa vào danh sách ứng viên nâng thành mã (mục 9). | H8 |
| R10 | **Kích thước.** Mục tiêu tối đa 80 dòng. Cảnh báo ở hơn 120 dòng hoặc hơn 8 KiB. Chặn chứng nhận ở hơn 200 dòng hoặc hơn 24 KiB. Tạm thời; hiệu chỉnh ở S8. | H14 |
| R11 | **Không bí mật, không đường dẫn tuyệt đối cá nhân, không giá trị biến môi trường.** `.env.example` chỉ chứa tên. | H11 (cảnh báo) |
| R12 | **Không mâu thuẫn và không trùng.** Chỉ dẫn không được trái `CLAUDE.md`, `AGENTS.md` lồng nhau hay steering khác. Mỗi Rule xuất hiện một lần. | Báo cáo của Architect; ledger rule |
| R13 | **Câu cụ thể, kiểm chứng được.** "Run `pnpm verify` before committing" thay cho "test your changes" (research A1). Tiếng Anh, một ý mỗi dòng. | Báo cáo của Architect |

### 6.2 Cấu trúc

```markdown
# <project name>

## Verify
- Ensure: `<ensure command>`. Read `docs/sdlc/environment.md` when the app or a service will not start.
- Quick check: `<fast command>`
- Done = `<verify command>` exits 0 (evidence level L<1-3>)

## Rules
- Never <X>; do <Y>. Enforced by: <lint rule | test | CI job | hook | prose>

## Known pitfalls
- <What goes wrong, and what to do instead> (<ruling, issue id, or command and exit code>)

## Read when
- Changing <area>: read `<path>`
```

Quy ước: tên mục `Known pitfalls` giữ như v1 (H12 và các dự án đang có); chấp nhận `Gotchas`. Mục nào rỗng thì xóa, trừ `Verify`.
Hai dòng bắt buộc: dòng `Ensure` (tên cả file lẫn lệnh, để H10 khớp) và đúng một dòng `Done = ...` (H6).

Tham chiếu độ dài: mẫu ở trên điền đầy đủ khoảng 25 đến 60 dòng.

### 6.3 Ví dụ phân loại

| Dòng | Giữ? | Vì sao |
| --- | --- | --- |
| "Dự án là ứng dụng web dùng React, Vite và Supabase." | Bỏ | Tổng quan; có trong manifest và README (R8) |
| "`src/` chứa mã nguồn; `tests/` chứa test." | Bỏ | Agent tự thấy khi liệt kê thư mục |
| "Use 2-space indentation." | Bỏ | Formatter đã bắt (R7) |
| "Never read `.env`; use `.env.example` for names. Enforced by: deny rule in CI secret scan." | Giữ | Rule, có cưỡng chế |
| "The full test suite takes about 20 minutes; run `pnpm test:quick` while editing. (TUPK-182)" | Giữ | Gotcha: sự kiện vận hành, có bằng chứng, đúng loại đã giảm chạy mù (research C3) |
| "App served over plain HTTP on the LAN has no `crypto.subtle`; test auth flows on `localhost`. (ruling R-014)" | Giữ | Gotcha khó suy ra từ code |
| "Changing database migrations: read `docs/planning-artifacts/architecture.md` section 5." | Giữ | Con trỏ có điều kiện |
| "Read `docs/planning-artifacts/architecture.md`." | Sửa | Con trỏ trần, thiếu điều kiện (R4) |
| "Auth is implemented; payments are in progress." | Bỏ | Trạng thái (R2) |

## 7. Cơ chế kiểm tra tất định

Công cụ: `catalog/skills/builtin/sdlc-harness/scripts/check-harness.mjs`. Node thuần, không phụ thuộc, in một báo cáo JSON.
Giữ ID cũ H1 đến H13 khi nghĩa không đổi để tái dùng test; thêm H14 đến H16, E1, E2.

### 7.1 Chế độ chạy và mã thoát

| Lệnh | Chạy | `blocking` gồm | Dùng bởi |
| --- | --- | --- | --- |
| `check-harness` | Mọi kiểm tra tĩnh (H1 đến H16) | Kiểm tra `gate` fail | Lead (Readiness, Route 0) |
| `check-harness --quick` | Chỉ `gate` (H1, H3, H4); dưới 1 giây | Kiểm tra `gate` fail | Developer, Bug Fixer, mọi vai đổi mã |
| `check-harness --cert` | Mọi kiểm tra tĩnh | `gate` và `cert` fail | Architect (Mode 1, 3) |
| `check-harness --cert --run [--timeout <giây>]` | Thêm E1, E2 (thực thi) | `gate`, `cert`, E1, E2 fail | Architect, chỉ trên repo của squad |
| `... --write-cert` | Ghi `docs/sdlc/harness-cert.json` nếu `certified` và đã `--run` | như trên | Architect |
| `check-harness --refresh` | Thêm mục `refresh` (mục 9) | như chế độ đi kèm | Lead, Architect |

Mã thoát: 0 khi `blocking` rỗng; 1 khi không rỗng; 2 khi sai tham số. `--quick` kết hợp `--run` bị từ chối (mã 2).

### 7.2 Báo cáo

```json
{
  "root": "...", "mode": "full | quick | cert | run",
  "ok": true,
  "certified": false,
  "blocking": [], "cert": ["H6"], "warnings": ["H5"],
  "footprint": { "lines": 52, "bytes": 2900, "instructions": 23, "proseRules": 3 },
  "checks": [ { "id": "H1", "title": "...", "level": "gate | cert | warning", "status": "pass | fail | skipped", "details": [] } ],
  "executed": { "ensure": { "command": "...", "exit": 0, "seconds": 41, "tail": [] }, "verify": { "command": "...", "exit": 0, "seconds": 120, "treeClean": true, "tail": [] } },
  "refresh": { "certSha": "...", "manifestsChanged": [], "needsRefresh": false, "reasons": [] }
}
```

`ok` = không có `gate` fail. `certified` = không có `gate` và `cert` fail (và, nếu có `--run`, E1 và E2 qua). `footprint.instructions` là số
mục danh sách trong `AGENTS.md` ngoài khối mã: một xấp xỉ, chỉ để báo cáo, không có ngưỡng.

### 7.3 Danh sách kiểm tra

| ID | Kiểm tra | Mức v2 | Mức v1 | Đầu vào | Chi tiết lỗi trả về |
| --- | --- | --- | --- | --- | --- |
| H1 | `AGENTS.md` tồn tại; `CLAUDE.md` chứa `@AGENTS.md` và tối đa một câu | **gate** | chặn | `AGENTS.md`, `CLAUDE.md` | file thiếu, hoặc thiếu import |
| H2 | `docs/sdlc/harness-cert.json` hợp lệ và `sha` là commit có thật; nếu chưa có, chấp nhận dòng `Verified ... against <sha>` của v1 | warning | chặn | cert JSON, git | thiếu, hỏng, sha không có |
| H3 | Mọi đường dẫn trong code span hoặc link tồn tại (bỏ URL, glob, tên gói, thư mục sinh ra, đường dẫn git ignore) | **gate** | chặn | `AGENTS.md`, cây repo | danh sách đường dẫn không có |
| H4 | Mọi script `npm`/`pnpm`/`yarn` và target `make` có thật | **gate** | chặn | `AGENTS.md`, `package.json`, Makefile | lệnh và nguyên nhân |
| H5 | Có file pin phiên bản và CI dùng cùng major | warning | cảnh báo | `.nvmrc`, CI | lệch phiên bản |
| H6 | Đúng một dòng `Done = ...` nêu lệnh và mức L1 đến L3; lệnh có thật nếu là `npm`/`pnpm`/`yarn`/`make` | **cert** | chặn | `AGENTS.md` | thiếu, nhiều dòng, thiếu mức |
| H7 | Không từ ngữ trạng thái trong luật (mục Greenfield được miễn) | **cert** | chặn | `AGENTS.md` | dòng và mẫu khớp |
| H8 | Mỗi mục trong Rules kết thúc bằng `Enforced by: <đích>`; đích khác `prose` phải là đường dẫn hoặc lệnh có thật | warning | chặn (bảng ranh giới) | `AGENTS.md` | dòng thiếu, đích không có; đếm `proseRules` |
| H9 | Có CONTRIBUTING, PR template hoặc quy ước commit | warning | cảnh báo | repo | không có |
| H10 | `docs/sdlc/environment.md` có dòng `Ensure command:`, và `AGENTS.md` nêu cả file lẫn lệnh | **cert** | chặn | hai file | thiếu một trong ba |
| H11 | `.env` và `.multica/` bị ignore, có `.env.example`, không mẫu bí mật trong file tracked | warning | cảnh báo | git | file và mẫu |
| H12 | Mỗi dòng Known pitfalls (hoặc Gotchas) có tham chiếu bằng chứng | warning | cảnh báo | `AGENTS.md` | dòng thiếu |
| H13 | Các file "must read" tổng cộng tối đa **32 KiB**, hoặc có chỉ mục "Read when" | warning | cảnh báo (60 KB) | `AGENTS.md`, file được trỏ | tổng và danh sách |
| H14 | Kích thước `AGENTS.md`: hơn 120 dòng hoặc 8 KiB là cảnh báo; hơn 200 dòng hoặc 24 KiB là `cert` | **cert** (cứng) / warning (mềm) | chặn (200 dòng, 12 KB) | `AGENTS.md` | dòng, byte, ngưỡng vượt |
| H15 | Mỗi con trỏ tới tài liệu có điều kiện (từ như "when", "before", "if", "changing"); không con trỏ trần | warning | mới | `AGENTS.md` | dòng thiếu điều kiện |
| H16 | Không có dấu hiệu tổng quan: ký tự vẽ cây (`├──`, `└──`), tiêu đề kiểu Overview/Architecture/Project structure/Tech stack, đoạn văn xuôi ba dòng liên tiếp ngoài danh sách; mục Greenfield được miễn | warning | mới | `AGENTS.md` | dòng và loại dấu hiệu |
| E1 | Chạy lệnh Ensure thoát 0 | **cert** | mới | `environment.md` | mã thoát, giây, đuôi đầu ra |
| E2 | Chạy lệnh Done thoát 0 trên cây git sạch | **cert** | mới | `AGENTS.md`, git | mã thoát, giây, đuôi đầu ra, `treeClean` |

Số lượng: `gate` 3 (H1, H3, H4); `cert` 6 (H6, H7, H10, H14, E1, E2); `warning` 9 (H2, H5, H8, H9, H11, H12, H13, H15, H16). H14 là kiểm tra duy nhất có mức
đổi theo ngưỡng: `warning` khi vượt ngưỡng mềm, `cert` khi vượt ngưỡng cứng.

Vì sao `gate` chỉ có ba: đó là các trường hợp harness **vắng hoặc nói dối** (không có file; trỏ tới thứ không tồn tại), nơi chỉ dẫn
còn tệ hơn không có (research mục 8.2). Chất lượng thấp (quá dài, thiếu dòng Done) là lý do để làm mới harness, không phải lý
do để đình công toàn dự án (bài học của [sdlc-autonomy.md](sdlc-autonomy.md) mục 4.1).

### 7.4 Kiểm tra thực thi (E1, E2)

- **Chỉ Architect** chạy `--run`, chỉ trên repo mà squad sở hữu. Lead không chạy `--run` (không khởi động dịch vụ khi lập kế hoạch);
  Developer không chạy `--run`.
- Lệnh lấy từ chính H10 (Ensure) và H6 (Done). Thực thi bằng `sh -c` tại gốc repo, `CI=1`, timeout mặc định 900 giây (cấu hình bằng
  `--timeout` hoặc `timeoutSeconds`), kết thúc cả nhóm tiến trình khi quá hạn. In ra lệnh trước khi chạy.
- E2 yêu cầu `git status --porcelain` rỗng (bỏ qua file bị ignore): Architect **commit đường dẫn của mình trước** rồi mới chạy `--run`.
  Nếu bẩn: fail với danh sách file.
- Đuôi đầu ra tối đa 20 dòng; dòng khớp mẫu bí mật bị che.
- Không đảm bảo lệnh Ensure idempotent; smoke (mục 8.4) chạy lại nó lần hai.
- Repo trống: E2 có thể qua vì chưa có gì để kiểm. "Có răng" chỉ được chứng minh ở bước làm hỏng cố ý của smoke (mục 8.4).

### 7.5 Hạ bậc có ghi chép

Chỉ các kiểm tra `cert` được hạ bậc: Architect ghi một assumption nêu tên kiểm tra và lý do trong `docs/sdlc/assumptions.md`, báo
trong báo cáo, và `--write-cert` ghi id đó vào `assumptions` của cert. `gate` không hạ bậc: sửa file. Sửa nhầm dương tính bằng cách
viết lại dòng hoặc khai `ignorePaths` trong `harness.json`, không nới kiểm tra.

### 7.6 Giới hạn đã biết

- H3 chỉ xét token trong code span hoặc link.
- H4 và H6 chỉ phân giải lệnh `npm`, `pnpm`, `yarn`, `make`; lệnh ngôn ngữ khác (`go test`, `pytest`, `cargo test`) chỉ được kiểm bằng E1, E2.
- H7 có thể báo nhầm một từ như "scaffold" dùng như động từ: sửa câu chữ.
- H11 quét mẫu khóa phổ biến; file `.env.*` công khai (ví dụ Expo) có thể báo nhầm (hiệu chỉnh ở S0, S2).
- H16 và H15 là heuristic; để mức `warning` cho tới khi biết tỉ lệ báo nhầm.
- Kiểm tra không đo được **chất lượng** của một Rule hay Gotcha; chỉ có đo hiệu quả (mục 10) làm việc đó.

## 8. Gating trong workflow

Không đổi server, không đổi status, không đổi brief. Mọi luật gating nằm trong skill (như v1).

### 8.1 Ai chặn ai, khi nào

| Vai | Khi nào | Chạy | Nếu fail | Ai xử lý |
| --- | --- | --- | --- | --- |
| **Lead** | Readiness, trước khi lập kế hoạch | `check-harness` | `gate` fail: tạo issue Harness (Architect, `--start-status harness`) và issue Harness smoke; **không** tạo `todo` cho Developer/Bug Fixer cho tới khi gate qua; Epic 1 chờ smoke (`--after`). Chỉ `cert`/`warning` fail trên repo đã chạy được: tạo issue Harness (Mode 3) xếp hàng, không chặn | Architect |
| **Lead** | Route 0 (`sdlc_maintenance`) | `check-harness` | như trên | Architect |
| **Lead** | Sau retrospective được chấp nhận; sau ruling lặp lần hai | `check-harness --refresh` | `needsRefresh`: tạo issue Harness (Mode 3) trước epic kế | Architect |
| **Architect** | Issue Harness | `check-harness --cert --run --write-cert` | Sửa tới khi `certified`; nếu không đạt vì lý do hợp lệ, hạ bậc `cert` có ghi chép (7.5) | Architect |
| **DevOps** | Environment | Viết ``- Ensure command: `<lệnh>` `` | Thiếu: Architect báo `tooling`, không tự viết hạ tầng | DevOps |
| **Developer, Bug Fixer** | Trước khi đổi mã trong mỗi issue | `check-harness --quick` | `gate` fail: dừng bằng `Blocker-class: tooling`, nêu id kiểm tra; Lead chuyển cho Architect. `cert`/`warning` fail: **không dừng** | Architect |
| **Developer** | Issue Harness smoke | Mục 8.4 | Báo kết quả; trượt thì Architect sửa và chạy lại một lần | Lead đánh giá |
| **Reviewer, QA** | Như hiện tại | Đọc bằng chứng Done | Không đổi | n/a |
| **Planning squad** | Bước 7 `planning-architecture` | Không chạy gì | n/a | n/a |

Không thêm lớp blocker mới: `Blocker-class: tooling` kèm id `H*` như v1.

### 8.2 Stage Harness, Mode 1 (greenfield), tóm tắt

Đầu vào: `Environment` đã xong, `docs/sdlc/environment.md` có dòng Ensure. Quy trình đầy đủ nằm trong skill `sdlc-harness`.

1. `check-harness --cert`. Nếu `certified` và `--refresh` không đòi làm mới: báo `pass`, dừng.
2. Chọn profile (web, backend, baas, mobile).
3. Khám phá lệnh từ manifest, CI, compose, `environment.md`. **Chạy** Ensure và từng lệnh dự định ghi; bỏ lệnh không chạy được.
4. Nếu chưa có lệnh Done: tạo một entrypoint (`verify`) gộp typecheck, lint, test sẵn có và biến thể nhanh. Không thêm công cụ.
5. Ghi lại mọi chỗ vấp ở bước 3 và 4 (lỗi môi trường, thứ tự lệnh, biến thiếu, thời gian chạy lâu), kèm lệnh và mã thoát: đây là ứng viên Known pitfalls.
6. Đọc hai mục "Non-default conventions" và "Verification strategy" của `architecture.md` (nếu có) và cấu hình công cụ sẵn có để lập Rules.
7. Viết `AGENTS.md` theo mục 6.2 (không tổng quan), đặt `CLAUDE.md` = `@AGENTS.md`. Chạy bài kiểm tra xóa (R5) từng dòng.
8. Commit đường dẫn của mình. Chạy `check-harness --cert --run --write-cert` tới khi `certified`. Commit `harness-cert.json`.
9. Báo cáo: kết quả kiểm tra, E1/E2 (lệnh, mã thoát, giây), footprint, số Rule `prose`, số assumption, điều chưa kiểm chứng.

Mode 2 (audit) và Mode 3 (refresh) giữ vai trò như v1 và bổ sung theo mục 9.

### 8.3 Điều không làm

- Lead không tự viết hay sửa `AGENTS.md`.
- Không vai nào thêm điều kiện chặn mà bảng 7.3 không nêu (sdlc-autonomy nguyên tắc 1).
- Reviewer và QA không phải chạy lại `check-harness`.

### 8.4 Harness smoke v2

Issue nhỏ do Lead tạo, giao Developer, chỉ dựa vào `AGENTS.md`, không hỏi ai:

1. Chạy Ensure theo `AGENTS.md`. Chạy lần hai (kiểm tra idempotent).
2. Thực hiện một thay đổi có test trong đúng một vùng của repo (tác vụ cố định cho từng profile, ví dụ thêm một test và một hàm nhỏ).
3. Chạy Quick rồi Done; ghi mã thoát.
4. **Bước âm tính:** làm hỏng cố ý một khẳng định hoặc một kiểu, chạy Done, xác nhận thoát khác 0, rồi hoàn tác.
5. Commit theo quy ước của repo.
6. Báo cáo các lệnh đã phải tự khám phá mà `AGENTS.md` lẽ ra nêu.

Ngưỡng (giả thuyết, hiệu chỉnh ở S8): tối đa 2 run để tới Stage done; 0 lần `blocked`; 0 lần review/QA trả lại; 0 can thiệp của người;
Done thoát 0 trên cây sạch; bước 4 thoát khác 0. Lệnh tự khám phá thành một dòng trong `AGENTS.md` hoặc một kiểm tra. Trượt:
Architect sửa chỗ thiếu và chạy lại một lần; trượt lần hai: ghi `docs/sdlc/parked.md`, epic tiếp tục.

Smoke là kiểm tra **chức năng đạt/trượt**, không phải phép đo thống kê: một lượt chạy không thể phân biệt nhiễu (research C8).

### 8.5 Bộ chuyển cưỡng chế tùy chọn

Chỉ xem xét sau S8, và chỉ khi số đo (M7, M2) cho thấy agent còn báo xong sai:

| Bộ chuyển | Phạm vi runtime | Chi phí | Rủi ro |
| --- | --- | --- | --- |
| Job CI chạy lệnh Done | Mọi runtime | Thấp | CI chậm |
| Git hook `pre-commit` chạy biến thể nhanh, cài bởi lệnh Ensure (`core.hooksPath`) | Mọi runtime dùng git | Trung bình | Bị bỏ qua bằng `--no-verify`; làm chậm mỗi commit |
| Hook `Stop` của Claude Code chạy biến thể nhanh (exit 2 khi hỏng) | Chỉ Claude Code | Trung bình | Giới hạn tám lần chặn liên tiếp (research A2, A4); chưa kiểm trên runtime daemon |

## 9. Cơ chế làm mới

Trigger giữ như v1 và đọc từ `harness-cert.json` thay vì dòng trong `AGENTS.md`.

| Trigger | Điều kiện | Việc |
| --- | --- | --- |
| T1. Sau retrospective được chấp nhận | `check-harness --refresh` báo `needsRefresh` | Issue Harness Mode 3 trước epic kế |
| T2. Manifest đổi | Có commit từ `sha` của cert chạm `package.json`, `Makefile`, CI, `supabase/config.toml`, `.env.example`, compose, `AGENTS.md`, hoặc `environment.md` | Như T1 |
| T3. Ruling lặp | Lần thứ hai ruling `environment`, `tooling` hoặc `rework-loop` cùng nguyên nhân được chấp nhận | Như T1, kèm thêm một Gotcha nếu thỏa luật tăng trưởng |
| T4. Cert hỏng | `sha` không còn là commit tổ tiên của `HEAD`, hoặc `harness-cert.json` thiếu/hỏng | Issue Harness Mode 1 hoặc 3 |
| T5. Cổng `gate` fail ở Developer | Block `tooling` có id H1, H3, H4 | Issue Harness Mode 3 |

Không có đồng hồ định kỳ: tránh lượt chạy rỗng.

**Luật tăng trưởng.** Một dòng mới chỉ vào `AGENTS.md` khi thỏa một trong hai: (a) agent đã bị sửa hoặc bị chặn hai lần vì cùng một chuyện
(hai ruling, hai comment bị trả lại, hai block cùng nguyên nhân); (b) Architect vấp lúc dựng ở Mode 1. Dòng luôn kèm id bằng chứng.

**Luật cắt tỉa.** Mỗi lần làm mới: chạy bài kiểm tra xóa (R5) trên mọi dòng ngoài Contract; xóa Gotcha mà nguyên nhân đã được sửa trong mã
(kèm commit sửa), theo "ledger rule" (cần một trong bốn căn cứ gỡ của `bmad-project-context`). Ghi số dòng trước và sau.

**Luật ngân sách.** Nếu `AGENTS.md` vượt mục tiêu (80 dòng), muốn thêm dòng phải gỡ một dòng hoặc nâng một dòng lên mã.

**Luật nâng lên mã.** Rule `prose` bị vi phạm hai lần được chuyển thành lint, test, job CI hoặc hook; dòng văn xuôi đổi `Enforced by` hoặc bị xóa.
Thông báo của kiểm tra mới nên nêu cách sửa (research D1, C7). Việc này do Architect tạo issue riêng cho Developer, không nằm trong stage Harness.

**Đầu ra của làm mới.** Cert mới (`--write-cert`), cộng một dòng trong báo cáo: số dòng trước/sau, số Gotcha thêm/xóa, số Rule nâng lên mã.

## 10. Cách đo hiệu quả

### 10.1 Nguyên tắc

- Đo để **phát hiện hồi quy và hướng đi**, không để chứng minh nhân quả. Nghiên cứu cho thấy một lượt chạy dao động vài điểm và
  phát hiện chênh 10 điểm cần 120 đến 200 tác vụ (research C3, C8). Với vài dự án, mọi chênh lệch là mô tả.
- Đơn vị phân tích: dự án (nhóm các issue Developer). So với nhóm đối chứng là các dự án SDLC trước, do người dùng chỉ định, đọc bằng CLI,
  không sửa gì.
- Báo cáo luôn kèm `n` và trung vị/p90, không kèm "có ý nghĩa".
- Quyết định giữ, rút gọn hay bỏ từng phần theo quy tắc đặt trước (10.5), không theo cảm giác sau khi thấy số.

### 10.2 Chỉ số

| ID | Tên | Định nghĩa | Hướng tốt |
| --- | --- | --- | --- |
| M1 | Chi phí dựng harness | Tổng số run và token của issue ở status Harness và Harness smoke | thấp |
| M2 | Tỉ lệ qua ngay (first-pass) | Số issue Developer về `done` mà không đi lùi trong thứ tự workflow (từ Review/QA về trạng thái làm) chia số issue Developer về `done` | cao |
| M3 | Run mỗi issue giao xong | Số run (kể cả thất bại) của issue về `done`: trung vị và p90 | thấp |
| M4 | Token mỗi issue giao xong | Input, output, cache read/write của các run; trung vị và p90 | thấp |
| M5 | Block mỗi issue theo lớp | Số lần vào `blocked` mỗi issue, tách theo `Blocker-class` đọc từ comment block | thấp |
| M6 | Thời gian chu kỳ | Từ lần vào trạng thái làm đầu tiên đến `done`: trung vị và p90 | thấp |
| M7 | Block do harness | Số block `tooling` nêu id `H*`, cộng block `environment` nhắc Ensure, trên toàn dự án | 0 |
| M8 | Footprint và tuổi | `footprint` của `check-harness`; số ngày kể từ cert; số manifest đổi từ cert | ổn định |
| M9 | Smoke | Đạt/trượt, số run, số lệnh tự khám phá, bước âm tính đạt/trượt | đạt, ít |

### 10.3 Lấy dữ liệu từ nền tảng

Không cần đổi server. Nguồn đã có:

| Dữ liệu | Lệnh CLI | Bảng nền tảng |
| --- | --- | --- |
| Các run của issue: trạng thái, thời điểm bắt đầu và kết thúc, lần thử, lý do thất bại | `multica issue runs <id> --output json` | `agent_task_queue` (`status`, `started_at`, `completed_at`, `attempt`, `failure_reason`) |
| Token | `multica issue usage <id> --output json` | `task_usage` (`input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`) |
| Chuyển status, `task_completed`, `task_failed` | `multica issue timeline <id> --activity-only --output json` (lọc `--action status_changed,task_completed,task_failed`) | `activity_log` |
| Lớp block | `multica issue comment list <id>` (đọc dòng `Blocker-class:`) | comment |
| Danh sách issue của dự án | `multica issue list`, `multica issue children <epic>` | `issue` |

Tên trường JSON thật của từng lệnh được xác nhận ở bước S0 bằng cách đọc struct trả về trong `server/internal/handler/`, không chạy lên DB dùng chung.

Công cụ: `scripts/harness-metrics.mjs` trong skill (bước S6). Các hàm tính toán là hàm thuần nhận JSON đã tải; lớp ngoài gọi `multica`. Có chế độ
`--from-dir` để test bằng dữ liệu mẫu. Lead chạy cuối mỗi epic và dán bảng vào báo cáo; không ghi file mới.

### 10.4 Eval pack ngoại tuyến

Mục đích: trả lời "harness có giúp không" khi số liệu dự án quá thưa (research F9; Anthropic A10).

- **Task:** 20 đến 30 tác vụ lấy từ lỗi thật của các dự án SDLC (issue bị trả lại, `rework-loop`, `environment`, `tooling`), mỗi tác vụ: snapshot repo (sha), nội dung
  issue, và chấm tất định (lệnh Done cộng một test hoặc kiểm tra riêng của tác vụ). Không dùng mua_ngon_ecommerce trừ khi người dùng đồng ý.
- **Biến thể:** V0' (chỉ ba dòng Verify, tối đa 15 dòng), V1 (AGENTS.md theo mẫu v1), V2 (mẫu v2). V0 (không file) tùy chọn trên mẫu con.
- **Lặp:** ít nhất 3 lượt mỗi ô. Số lượt chạy ≈ 25 tác vụ × 3 biến thể × 3 lượt = 225.
- **Số đo:** pass@1 (trung bình các lượt), pass^3, token, số bước; khoảng tin cậy bootstrap theo cụm tác vụ. Đọc transcript của mẫu thất bại để biết
  grader có sai không (A10).
- **Cỡ mẫu thấp** (research C3): kết quả được đọc như hướng và chi phí, không như phép thử ý nghĩa. Khoảng tin cậy rộng hơn 15 điểm thì kết luận là "không phân biệt được".

### 10.5 Quy tắc quyết định đặt trước (mặc định, có thể đổi trước khi chạy S8, S9)

| # | Điều kiện | Quyết định |
| --- | --- | --- |
| DR1 | Sau ≥5 dự án có cert, M7 > 0 ở ≥2 dự án | Không mở rộng `AGENTS.md`. Kiểm Ensure/Done và H-check; sửa nguyên nhân gốc |
| DR2 | Eval pack: V2 không hơn V0' về pass@1 và tốn token mỗi lượt cao hơn hơn 10% | Rút gọn mặc định xuống V0' (chỉ Verify, Rules có cưỡng chế). Chỉ giữ Known pitfalls có bằng chứng ≥2 lần |
| DR3 | Eval pack: V2 so V1 có chi phí token không cao hơn và pass@1 không thấp hơn quá 5 điểm (khoảng rộng thì tính là không phân biệt) | Giữ V2 làm mặc định |
| DR4 | Smoke trượt ở ≥2 trong 5 dự án đầu vì cùng một lệnh tự khám phá | Thêm lệnh đó vào mẫu profile như điều cần thăm dò; không thêm vào mọi AGENTS.md |
| DR5 | Sau S8 | Đặt lại ngưỡng H14 (mềm, cứng) theo phân vị 90 của `AGENTS.md` đã qua smoke; không tối ưu ngưỡng theo M2/M3 |

### 10.6 Khi nào báo cáo

- Cuối mỗi epic: Lead dán bảng M2 đến M7 của epic và M8 vào báo cáo.
- Sau mỗi stage Harness: M1, M9, `footprint`.
- Mỗi 5 dự án có cert: một lần xem lại DR1 đến DR5 (do người dùng quyết).

## 11. Rủi ro và giảm thiểu

| # | Rủi ro | Giảm thiểu |
| --- | --- | --- |
| 1 | `AGENTS.md` v2 vẫn không tăng tỉ lệ thành công (khả năng cao theo C1) | Mục tiêu thật là chi phí thấp và chặn báo xong sai; DR2 rút gọn tự động; đo M1 đến M7 |
| 2 | Kiểm tra `gate` báo nhầm dương tính, chặn Developer | Chỉ ba kiểm tra gate, khách quan; hiệu chỉnh trên ≥3 repo tốt ở S0; `ignorePaths`; tỉ lệ chặn sai là tiêu chí chấp nhận |
| 3 | `--run` chạy lệnh có hại | Chỉ Architect, chỉ repo của squad, hiện lệnh trước khi chạy, timeout, che bí mật; Lead và Developer không dùng `--run` |
| 4 | Lệnh Ensure có tác dụng phụ trên môi trường dùng chung | DevOps viết Ensure idempotent và cô lập theo worktree; kiểm bằng smoke bước 1 |
| 5 | Done vô nghĩa (`echo ok`) qua kiểm tra tĩnh | E2 và bước âm tính của smoke |
| 6 | Architect LLM vẫn viết tổng quan hoặc chép lệnh | Mẫu không có chỗ cho tổng quan; H16, H15; R5; reviewer là Lead qua báo cáo; đo `footprint` |
| 7 | Ngưỡng kích thước sai | Gọi rõ là tạm thời; DR5 hiệu chỉnh; chỉ trần công cụ được chặn |
| 8 | Hook cưỡng chế chậm hoặc bị bỏ qua | Không cài mặc định; mục 8.5 chỉ sau số đo |
| 9 | Chỉ số bị đọc thành nhân quả | Mục 10.1; luôn báo `n`; quyết định theo DR đặt trước |
| 10 | Mất nội dung khi chuyển `AGENTS.md` cũ sang v2 | Ledger rule: không xóa chỉ dẫn của người; chuyển tổng quan sang `docs/` bằng con trỏ |
| 11 | Mất tương thích: squad cũ đang chạy skill v1 | Skill cập nhật qua `upgrade-sdlc-squad.mjs --apply`; script giữ đọc dòng `Verified` của v1 (H2); không đổi status/brief |
| 12 | `harness-cert.json` lệch khỏi sự thật sau khi ai đó sửa `AGENTS.md` bằng tay | T2 coi `AGENTS.md` là manifest; `--refresh` bắt |
| 13 | Dữ liệu CLI không có trường cần cho M2 hoặc M5 | S0 xác nhận; nếu thiếu, M2/M5 rút về xấp xỉ từ comment và ghi rõ |
| 14 | Chuẩn thay đổi: Claude Code, Codex đổi cách đọc AGENTS.md | Ghi ngày xác nhận trong research; H1 chỉ đòi import; xem lại mỗi lần nâng cấp runtime |

## 12. Tiêu chí chấp nhận

Kiểm chứng bằng test và lệnh cụ thể ở [plan](sdlc-harness-plan.md).

| # | Tiêu chí |
| --- | --- |
| AC1 | Báo cáo có `level` ∈ {gate, cert, warning}; `gate` đúng {H1, H3, H4}; `cert` đúng {H6, H7, H10, H14 (khi vượt ngưỡng cứng), E1, E2}; các kiểm tra còn lại `warning` |
| AC2 | `check-harness --quick --root <multica>` thoát 0 (v1: thoát 1); `check-harness --quick` trên thư mục trống thoát 1 |
| AC3 | Repo có `AGENTS.md` 130 dòng, không có dòng Done: `--quick` thoát 0, `--cert` thoát 1 và liệt kê H6 |
| AC4 | `--cert --run` trên fixture: Ensure thoát 1 thì E1 fail và exit 1; Done thoát 1 thì E2 fail; cây bẩn thì E2 fail; hết timeout thì fail và nhóm tiến trình bị dừng |
| AC5 | `--quick --run` thoát 2 |
| AC6 | H14: 121 dòng cho cảnh báo; 201 dòng hoặc 24 KiB + 1 byte cho `cert` fail |
| AC7 | H15, H16, H8: fixture có con trỏ trần, cây thư mục, tiêu đề Overview, Rule thiếu `Enforced by` đều bị bắt; fixture sạch không bị bắt; mục Greenfield được miễn |
| AC8 | `--write-cert` chỉ ghi khi `certified` và đã `--run`; cert qua schema ở phụ lục B; `--refresh` báo `needsRefresh` khi đổi `package.json` sau sha cert |
| AC9 | Mẫu v2 điền cho mỗi profile (web, backend, baas, mobile) vượt `--cert` trên fixture, ≤80 dòng |
| AC10 | Skill: Developer chỉ bị `gate` chặn; Lead không tạo `todo` khi `gate` fail; Route 0 chỉ chặn khi `gate` fail; `planning-architecture` bước 7 không bắt viết `AGENTS.md`; smoke có bước âm tính. Các câu này được test bằng `sdlc-skills.test.ts` |
| AC11 | `harness-metrics.mjs` tái tạo M2 đến M7 từ dữ liệu mẫu đã biết (test); không chạy lên DB dùng chung trong CI |
| AC12 | `git diff --stat` của triển khai chỉ chạm các file liệt kê trong plan; không đổi `templates.ts`, `initiative_build.go`, `server/`, `upgrade.ts`, `upgrade.test.ts`; `pnpm generate:sdlc-skills` không còn lệch |
| AC13 | `docs/engineering/sdlc-harness.md` được cập nhật khớp v2 hoặc đánh dấu thay thế |
| AC14 | Dự án thí điểm (S8): smoke đạt trong ≤2 run, M7 = 0, `footprint` ≤ 80 dòng; số liệu M1 đến M9 được ghi |

## 13. Câu hỏi mở và cần người quyết định

1. Chấp nhận mô hình ba mức `gate`/`cert`/`warning`, hay giữ nhị phân chặn/cảnh báo như v1?
2. Chọn các repo hiệu chỉnh ở S0 (ít nhất hai repo tốt, khác multica và khác mua_ngon_ecommerce)?
3. Có chấp thuận chạy thí điểm S8 (dự án thật hoặc hai bản sao trên môi trường cô lập, 30 đến 120 lượt chạy agent) không? Phải dùng môi trường riêng theo worktree (`make up`), không DB dùng chung.
4. Chấp thuận S9 (eval pack, khoảng 225 lượt chạy)? Có thể hoãn tới khi có số đo từ ≥5 dự án.
5. Runtime nào của daemon chạy agent SDLC? Cần để quyết định bộ chuyển hook `Stop` hay git hook (mục 8.5).
6. Có đồng ý bỏ yêu cầu viết `AGENTS.md` khỏi bước 7 của `planning-architecture` không? (D10; khác với v1.)
7. Cập nhật tại chỗ `sdlc-harness.md` ở S7, hay viết tài liệu thay thế và đánh dấu bản cũ?
8. Nhóm đối chứng cho M2 đến M6 là những dự án nào?

## Phụ lục A. Mẫu `AGENTS.md` v2 đầy đủ (ví dụ giả định)

```markdown
# acme-shop

## Verify
- Ensure: `pnpm env:ensure`. Read `docs/sdlc/environment.md` when the app or Supabase will not start.
- Quick check: `pnpm verify:quick`
- Done = `pnpm verify` exits 0 (evidence level L2)

## Rules
- Never call `fetch` from a component; use the client in `src/lib/api.ts`. Enforced by: `eslint.config.js` (no-restricted-imports)
- Never put a service-role key behind a `VITE_` prefix. Enforced by: `scripts/check-env-names.mjs` in `pnpm verify`
- Migrations are forward-only; never edit an applied one. Enforced by: prose

## Known pitfalls
- `pnpm verify` takes about 20 minutes; use `pnpm verify:quick` while editing. (TUPK-182)
- The app over plain HTTP on a LAN address has no `crypto.subtle`; test sign-in on `localhost`. (ruling R-014)

## Read when
- Changing database schema or policies: read `docs/planning-artifacts/architecture.md` section "Data".
- Changing sign-in or sessions: read `docs/sdlc/rulings.md` entries R-014 and R-021.
```

## Phụ lục B. Schema `docs/sdlc/harness-cert.json`

```json
{
  "schema": 1,
  "date": "2026-11-12",
  "sha": "<HEAD tại lúc chứng nhận>",
  "profile": ["web", "baas"],
  "checks": { "gate": ["H1", "H3", "H4"], "cert": ["H6", "H7", "H10", "H14", "E1", "E2"], "warnings": ["H5"] },
  "executed": {
    "ensure": { "command": "pnpm env:ensure", "exit": 0, "seconds": 41 },
    "verify": { "command": "pnpm verify", "exit": 0, "seconds": 120, "treeClean": true }
  },
  "footprint": { "lines": 22, "bytes": 1400, "instructions": 9, "proseRules": 1 },
  "assumptions": ["A-3"]
}
```

`assumptions` liệt kê id assumption đã hạ bậc một kiểm tra `cert`. Bản triển khai thêm trường `accepted` (id các kiểm tra `cert` bị hạ bậc, ví dụ `["H6"]`),
được ghi bằng `--accept <ID>=<assumption id>`; `executed.verify` hoặc `executed.ensure` là `null` khi kiểm tra tương ứng được chấp nhận mà không chạy. Báo cáo của
`check-harness` có thêm `accepted` và `certificate` (kết quả của `--write-cert`). JSON thay vì Markdown vì model ít sửa nhầm JSON hơn (research A9) và vì
máy đọc nó.
