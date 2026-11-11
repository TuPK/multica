# Kit Việt P2 notes: vertical trades, floors and the per-trade proposals

Status (2026-11-11): the ten P2 squads and Teaching Materials (`kv_teaching`, the second Education squad, section 11) are built on
`kitviet-pack` (spec `kitviet-squads.md` section 9.2): config, generated pack, template, lead rules in `KV_LEAD_NOTES`, seven-language
locale, tests and runbook (sections 8, 8c, 8d, 8e). Section 10 records
what changed while integrating them, and where that differs from the proposals in sections 6 to 9. Not committed yet. Commands run from the repository root.
The recipe is `kitviet-p1-notes.md` section 2; this file adds only what differs for P2.

## 1. What is in place

- **Vendored.** `packages/core/squads/kitviet-upstream/<trade>/{agents,skills}` for `y-te`, `xay-dung`, `nha-hang-fnb`,
  `bao-hiem`, `tai-chinh-ngan-hang`, `bat-dong-san`, `du-lich-khach-san`, `xuat-nhap-khau`, `san-xuat-nha-may`, `giao-duc`, at the
  pinned commit `faee4f14cd21672325a7f442b19c7239a5d7f152`. Nothing else from upstream (no `codex/`, `claude-web/`, `brand/`).
  219 trade files copied and checked (sha256 after LF normalisation, 0 mismatches). The 30 `common/` paths are identical in all
  ten P2 trades (300 checks, 0 mismatches), so they stay in `common/`. `NOTICE.md` and `UPSTREAM.json` list the ten trades.
- **Generator.** No change to the pack discovery or the agent-kind classifier was needed (section 4). Two general changes to the
  rewrite table (section 3).
- **Floors.** `packages/core/squads/kitviet-floors.ts` (`KITVIET_P2_FLOOR_NAMES`, `KITVIET_P2_FLOOR_NAME_LIST`) and its test.
- **Checks run (2026-11-11, after Teaching Materials and the guard/flag work):** `node scripts/generate-kitviet-pack.mjs --check`
  up to date (29 files); `node --test scripts/generate-kitviet-pack.test.mjs` 118 of 118; `pnpm typecheck` clean; `pnpm lint` 0 errors
  (warnings are in files this work does not touch); `pnpm --filter @multica/core test` 317 files, 4688 passed, 1 skipped;
  `pnpm --filter @multica/views test` 673 files, 8231 passed (including `locales/parity.test.ts`); `git diff --check` clean;
  `cd server && go build ./...` ok; `go test ./internal/service -run 'Company|Template' -count=1` ok. The P2 floor test is 3 of 3.

## 2. Floors (decision)

| Trade | Pack id | Template id | Floor (English) | Note |
| --- | --- | --- | --- | --- |
| Y tế | `health` | `kv_health` | `Healthcare` | |
| Xây dựng | `construction` | `kv_construction` | `Construction` | |
| Nhà hàng, F&B | `fnb` | `kv_fnb` | `F&B` | separate from Hospitality |
| Bảo hiểm | `insurance` | `kv_insurance` | `Insurance` | |
| Tài chính, ngân hàng | `banking` | `kv_banking` | `Banking` | |
| Bất động sản | `realestate` | `kv_realestate` | `Real Estate` | |
| Du lịch, khách sạn | `hospitality` | `kv_hospitality` | `Hospitality` | |
| Xuất nhập khẩu | `trade` | `kv_trade` | `Import & Export` | |
| Sản xuất nhà máy | `factory` | `kv_factory` | `Manufacturing` | |
| Giáo dục | `edu` | `kv_education` | `Education` | shared with `kv_teaching` (section 11) |

Decisions:

1. **Not in `COMPANY_FLOOR_KEYS` / `COMPANY_FLOOR_NAMES`.** Ecom's `Revenue` went there (P1, Q4) because that list is what the
   starter wizard renders, with a translated label per key in seven locales (`company.company_floors[floor]`). A P2 floor would need
   those labels and a wizard group with no starter card on it (the wizard hides it). The quickstart does not need the list:
   `ensureCompanyFloor(client, floorName)` (`quickstart.ts`) finds the floor by name and creates it when missing, then seats the
   squad. So a P2 squad template states `floorName` from `KITVIET_P2_FLOOR_NAME_LIST`; its test checks membership there, not in
   `COMPANY_FLOOR_NAMES` (the P0/P1 tests check the default list; P2 cannot).
2. **English names, stable across reruns**, like the squad and agent names. Short, no Vietnamese, no trailing spaces, at most 64
   characters (the server check is `char_length(name) BETWEEN 1 AND 64`). `kitviet-floors.test.ts` enforces this.
3. **Blocking: the floor cap (resolved, spec Q11).** The server caps a workspace at 16 floors (`CompanyMaxFloors = 16` in
   `server/internal/service/company_setup.go`, `companyMaxFloors = 16` in `server/internal/handler/company.go`; the ensure path in
   `company_template.go` and the create path in the handler both check it). A full starter company already uses the seven default
   floors (Executive, Research, Product, Build, Launch, Governance, Revenue). Ten P2 floors make 17. The tenth P2 squad would be
   seated with the error "could not set up the … floor" and no seat. **Decision needed (not changed here, it is server code):**
   raise both constants to at least 17; 24 leaves room. The existing test in `company_test.go` (around line 1104) reads the constant,
   so it follows. Until that is decided, the P2 squads can be built but the last one or two cannot all get their own floor.

## 3. Shared pieces (implemented)

- `kitviet-floors.ts`: the ten names, one per trade (table above). Templates state `floorName` from it.
- `kitviet-rewrites.json`:
  - `file-01-collect` now also matches `01-nguon.md` (y-te, `gac-pham-vi-chuyen-mon` and `soan-tai-lieu-y-te` read it). The rule keeps
    the P0/P1 output unchanged (`--check` passes).
  - new optional rule `verdict-gui-duoc-quoted`: `"Gửi được"` → `QUA`. It is optional because only the common gates
    `soat-bao-mat` and `soat-chinh-ta-giong-van` carry the quoted phrase, and no squad maps them yet. A squad that maps either one
    gets the quoted form fixed; the rule must not be made required before then.
- No new `FORBIDDEN` entry (section 6, open item 3).

## 4. Pack discovery and the agent-kind classifier

Checked on every P2 agent (classifier in `scripts/generate-kitviet-pack.mjs`, `classifyAgent`):

- Every trade-specific agent gets a kind that the tools and the `Gác cổng` opener determine. `Write`+`Edit` is `kv_drafter`
  (16 "Thực thi" agents across the ten trades); `Write` only is `kv_handover` (none in P2; the handover is the shared
  `kiem-dinh-ban-giao`); `Gác cổng` with no `Write` is `kv_gate` (all `soat-*`, `gac-*` agents); anything else with no `Write` is
  `kv_collector` ("Thu thập", with or without `WebSearch`/`WebFetch`). No P2 agent has an unusual tool set (no `Bash`, no `Edit`
  without `Write`, no `Write` on a gate).
- Every P2 trade has one `day-chuyen-*` pipeline skill, so every squad is `flow: "pipeline"`. No trade needs `flow: "authored"`.
- Every P2 skill has `Dùng khi nào` and `Không được làm`. No `kv:pro` leftover reaches the output (the Pro block removal covers it).
- Common-skill reference closure holds: a shared skill names no trade skill. The throwaway run fails if one does.
- `kv:` references from a pipeline to an agent the squad does not map become "(vai không có trong squad: nhờ lead)". Each proposal
  below maps every agent its pipeline names, so none of them reads that phrase.
- Two cross-trade skill duplicates were checked over all 20 trades (`overlaps.mjs`, byte comparison after LF normalisation):
  `viet-giong-gen-z` is identical in `marketing`, `ban-hang-online`, `giao-duc`; `soan-don-tu` in `phap-ly`, `hanh-chinh-van-phong`,
  `nhan-su`. No other name is shared. Only `giao-duc` needed a decision: `viet-giong-gen-z` is **not** listed by the Education squad
  (marketing owns it, section 5 of the P1 notes). Its bytes stay vendored, as `ban-hang-online` keeps them.

## 5. Throwaway runs (all ten trades)

Each trade was run alone against the real P0/P1 configs and a throwaway squad config (every agent mapped, every other skill
deferred). Then all ten together. The throwaway configs were deleted afterwards. Results:

| Trade | First run | Fix | Result |
| --- | --- | --- | --- |
| y-te | `01-nguon.md` is a numbered stage file (2 agents) | rule `file-01-collect` extended | OK |
| xay-dung, nha-hang-fnb, bao-hiem, bat-dong-san, du-lich-khach-san, xuat-nhap-khau, san-xuat-nha-may | OK | none | OK |
| tai-chinh-ngan-hang | two gates have no VERDICT line: their `## Kết luận: QUA \| CHẶN` heading is inside the example block of `## Trả về` | exact-match patch per gate (text in section 7) | OK with patch |
| giao-duc | `viet-giong-gen-z` rendered by marketing and by giao-duc (bundle name duplicate) | not listed by giao-duc | OK |

All ten together: OK with the patches in place (re-added for the check and removed again, so the repo holds no P2 patch yet).

Leftovers that the guard does not catch (per trade, for the squad agent, section 7 gives the patches to write):

- Platform text in skill bodies: the `guard` forbids only `Claude Desktop`, `Codex`, `.kv-run`, `CLAUDE.md`, `AGENTS.md`, `ket-qua/`, `phiên chính`,
  `pip install`, `apt install`, numbered stage files and versioned files. Inline `Claude Code` and `Claude web` survive in the
  bodies of nine P2 files (listed in section 7). The same survives in P1 (section 6, item 3).
- Old severity words in P2 (`Nặng/Vừa/Nhẹ`, `Nghiêm trọng`, `Phải sửa`, `Gửi được` in bold, `Gợi ý`, `Lưu ý`). They are per file,
  not per trade, so they stay for the squad agent. Each trade's own scale is in section 7.
- Dated law and fee text: none to fix. The P2 bodies avoid hard-coded legal numbers (`không tự đặt số`, `theo quy định hiện hành`).
  They name laws without numbers (`Luật Quảng cáo`, `Luật Đất đai`, `Luật Nhà ở`, `Luật Kinh doanh bất động sản`,
  `Luật An toàn thực phẩm`, `Bộ luật Dân sự`) and `Nghị định biểu thuế ưu đãi đặc biệt` (xuat-nhap-khau). Example dates such as
  `03/11/2026` are sample data.
- Install hints: none. The only `cài` in P2 is `cài ứng dụng` in a scam warning (banking) and `cài đặt` for a machine setting
  (factory). Both are content, not install instructions.
- `.kv-run`, `ket-qua/`, `CLAUDE.md`, `AGENTS.md`: none left after the rules.

## 6. Open items and deviations

1. **Floor cap (section 2).** Blocking for "one floor per trade". Needs a decision and a two-constant change in server code.
2. **Own floors but shared work.** Each P2 squad is on its own floor (spec 9.2). A squad is seated by quickstart; the user moves it
   with drag-and-drop or `setCompanySeats`. No P2 squad is on an existing floor.
3. **Platform strings: resolved (2026-11-11).** `FORBIDDEN` now bans `Claude Code`, `Claude web`, `Claude Desktop`, `Claude.ai`, `Codex`
   and `Cursor` (spec Q14, runbook 8e). The inline lines that remained in P1 and P2 are handled by rewrite rules and patches; `--check` passes.
4. **Severity scale in P2 is per file.** The unified scale is `Chặn / Cần giấy tờ / Nên sửa / Góp ý`, verdict `QUA / CHẶN`, criterion
   `Đạt / Không đạt / Không kiểm được` (P0 rule, F18). The P2 gates mix scales (`Nặng/Vừa/Nhẹ`, `Nghiêm trọng`, `Phải sửa / Nên bổ sung`).
   The squad agent maps them per skill with patches.
5. **Education's four unmapped agents: resolved.** giao-duc has 8 trade agents; Education Desk maps 4 of them plus the lead and the
   handover. The other four (`tim-tu-lieu-bai-giang`, `soan-van-ban-chu-nhiem`, `tong-hop-ho-so-lop-hoc`, `soat-nguon-tu-lieu-giang-day`)
   now form Teaching Materials (section 11).
6. **Risky gates (spec 9.2).** `gac-pham-vi-chuyen-mon` and `gac-du-lieu-nguoi-benh` (y-te), `soat-cam-ket-tu-van` (bao-hiem),
   `soat-tuan-thu-tu-van-tai-chinh` and `soat-to-trinh-tin-dung` (tai-chinh-ngan-hang) need a person in the trade to read the
   output before the squad is switched on. Their descriptions are in section 7. `soat-an-toan-hoc-sinh` (giao-duc) handles student
   data and is also a gate to review. `soat-phuong-an-vay-bds` and `soat-cam-ket-dich-vu-luu-tru` (bat-dong-san, du-lich) hold
   promises and money, so they are also listed as risky.

## 7. Per trade

Agent kinds are the classifier's output. Common agents (`doc-nhieu-tai-lieu`, `kiem-dinh-ban-giao`, `lap-ke-hoach-dieu-phoi`,
`nghien-cuu-nguon`, `soat-bao-mat`, `soat-chinh-ta-giong-van`, `soat-so-lieu`) are shared by every squad. The lead maps to
`lap-ke-hoach-dieu-phoi` with `kv_lead`; the handover maps to `kiem-dinh-ban-giao` with `kv_handover`. Skill sizes are the folder
size in KiB (SKILL.md and files). "Core" is the shared core (about 90 KiB of prose, measured from `common/skills` plus
`kv-delivery-core`) plus the pipeline (6 to 8 KiB), about 100 KiB in every proposal, under the 140 KiB ceiling.

### 7.1 y-te (Healthcare): `kv_health`, pack `health`

Trade agents: `doc-tai-lieu-chuyen-mon-y-te` collector (Read,Grep,Glob); `tra-cuu-y-van` collector (+WebSearch,WebFetch);
`tra-cuu-quy-dinh-y-te` collector (+WebSearch,WebFetch); `soan-tai-lieu-y-te` drafter; `lap-bang-bieu-y-te` drafter;
`gac-pham-vi-chuyen-mon` gate (no Write, no network: scope gate); `gac-du-lieu-nguoi-benh` gate (patient identifiers).

Skills (16, 89 KiB, folder sizes): `day-chuyen-tai-lieu-y-te` pipeline 7.8, `tong-quan-tai-lieu-y-khoa` 7.4, `soan-sop-phong-kham` 5.4,
`tai-lieu-giao-duc-suc-khoe` 5.8, `bang-kiem-an-toan-thu-thuat` 5.6, `bao-cao-su-co-y-khoa` 5.4, `dao-tao-noi-bo-y-te` 5.1,
`noi-dung-quang-cao-phong-kham` 5.3, `nhac-lich-tai-kham-zalo` 4.9, `theo-doi-han-dung-thuoc` 5.3, `xep-lich-truc-dieu-duong` 5.0,
`tom-tat-phac-do-dieu-tri` 5.0, `ban-giao-ca-sbar` 5.2, `chuan-bi-doan-kiem-tra-chat-luong` 5.3,
`khao-sat-hai-long-nguoi-benh` 5.4, `tra-loi-danh-gia-nguoi-benh` 5.3.

Proposed squad: `Healthcare (Y tế)`, floor `Healthcare`, 9 desks.

| Role key | Upstream agent | Kind |
| --- | --- | --- |
| `health_lead` | `lap-ke-hoach-dieu-phoi` | kv_lead |
| `health_reader` | `doc-tai-lieu-chuyen-mon-y-te` | kv_collector |
| `health_research` | `tra-cuu-y-van` | kv_collector |
| `health_regulation` | `tra-cuu-quy-dinh-y-te` | kv_collector |
| `health_drafter` | `soan-tai-lieu-y-te` | kv_drafter |
| `health_table` | `lap-bang-bieu-y-te` | kv_drafter |
| `health_scope_gate` | `gac-pham-vi-chuyen-mon` | kv_gate |
| `health_privacy_gate` | `gac-du-lieu-nguoi-benh` | kv_gate |
| `health_handover` | `kiem-dinh-ban-giao` | kv_handover |

`builtinSkills`: `day-chuyen-tai-lieu-y-te`. `roleSkills`: `health_drafter`: `soan-sop-phong-kham`, `tai-lieu-giao-duc-suc-khoe`,
`bao-cao-su-co-y-khoa`, `noi-dung-quang-cao-phong-kham`, `nhac-lich-tai-kham-zalo`; `health_table`: `theo-doi-han-dung-thuoc`,
`xep-lich-truc-dieu-duong`, `bang-kiem-an-toan-thu-thuat`; `health_research`: `tong-quan-tai-lieu-y-khoa`, `tom-tat-phac-do-dieu-tri`.
`deferredSkills`: `dao-tao-noi-bo-y-te`, `ban-giao-ca-sbar`, `chuan-bi-doan-kiem-tra-chat-luong`, `khao-sat-hai-long-nguoi-benh`,
`tra-loi-danh-gia-nguoi-benh`.

Risky: `gac-pham-vi-chuyen-mon` blocks diagnosis, prescription, dose change, promised cure, claims beyond the source; it is the
clinical scope gate and needs a clinician to check its output before it is on. `gac-du-lieu-nguoi-benh` blocks identifiers
(name, bed, record, phone, ID card, insurance card, photo). Both are read-only.

Leftovers: `tong-quan-tai-lieu-y-khoa` line 37 "(Claude web/desktop: tự tra, chỉ giữ bậc 1-2)" (patch).
`noi-dung-quang-cao-phong-kham` cites "Luật Quảng cáo" without a number (kept).

### 7.2 xay-dung (Construction): `kv_construction`, pack `construction`

Trade agents: `doc-ho-so-cong-truong` collector; `tra-cuu-quy-chuan-xay-dung` collector (+web); `lap-bang-khoi-luong` drafter;
`soan-van-ban-cong-truong` drafter; `soat-khoi-luong-don-vi` gate; `soat-an-toan-cong-truong` gate (safety and authority).

Skills (17, 108 KiB): `day-chuyen-ho-so-cong-truong` pipeline 8.8, `de-cuong-thuyet-minh-thi-cong` 7.9, `bien-ban-nghiem-thu-cong-viec` 6.6,
`checklist-an-toan-hang-muc` 6.6, `bao-cao-su-co-cong-truong` 6.5, `nhat-ky-thi-cong` 6.8, `bao-cao-tuan-ban-chi-huy` 6.1,
`bao-cao-anh-hien-truong` 6.3, `kiem-tra-khoi-luong-xay-dung` 6.0, `du-toan-so-bo-cong-trinh` 6.1, `de-nghi-phat-sinh-rfi` 6.2,
`dan-y-bien-phap-thi-cong` 5.8, `tien-do-thi-cong-duong-gang` 5.5, `soat-ho-so-hoan-cong` 5.9, `tong-hop-loi-hoan-thien` 5.7,
`yeu-cau-vat-tu-cong-truong` 5.7, `bien-ban-giao-ban-cong-truong` 5.5.

Proposed squad: `Construction (Xây dựng)`, floor `Construction`, 8 desks.

| Role key | Upstream agent | Kind |
| --- | --- | --- |
| `construction_lead` | lap-ke-hoach-dieu-phoi | kv_lead |
| `construction_reader` | doc-ho-so-cong-truong | kv_collector |
| `construction_standards` | tra-cuu-quy-chuan-xay-dung | kv_collector |
| `construction_quantity` | lap-bang-khoi-luong | kv_drafter |
| `construction_site_drafter` | soan-van-ban-cong-truong | kv_drafter |
| `construction_quantity_gate` | soat-khoi-luong-don-vi | kv_gate |
| `construction_safety_gate` | soat-an-toan-cong-truong | kv_gate |
| `construction_handover` | kiem-dinh-ban-giao | kv_handover |

`builtinSkills`: `day-chuyen-ho-so-cong-truong`. `roleSkills`: `construction_site_drafter`: `nhat-ky-thi-cong`,
`bao-cao-tuan-ban-chi-huy`, `bao-cao-su-co-cong-truong`, `bien-ban-giao-ban-cong-truong`, `bao-cao-anh-hien-truong`;
`construction_quantity`: `kiem-tra-khoi-luong-xay-dung`, `du-toan-so-bo-cong-trinh`; `construction_standards`:
`de-cuong-thuyet-minh-thi-cong`, `dan-y-bien-phap-thi-cong`; `construction_reader`: `tong-hop-loi-hoan-thien`.
`deferredSkills`: the other nine.

Leftovers: `de-cuong-thuyet-minh-thi-cong` line 40 "(Claude Code có thể gọi `kv:tra-cuu-quy-chuan-xay-dung` …)" (patch).
Severity: none on a fixed scale (the gates say "Chặn" in their own rules, already the new scale). "Gợi ý" in two skills.
Gate criterion wording "Đạt / Chưa đạt" in the `soat-khoi-luong-don-vi` verdict line is replaced by the template (the generator
checks one VERDICT line). The Chưa đạt criterion table in the same agent must be checked.

### 7.3 nha-hang-fnb (F&B): `kv_fnb`, pack `fnb`

Trade agents: `doc-so-lieu-quan-an` collector; `tra-cuu-quy-dinh-fnb` collector (+web); `soan-tai-lieu-quan-an` drafter;
`soat-di-ung-attp-mon` gate (allergen and food safety); `soat-gia-von-lai-quan` gate (food cost and margin).

Skills (13, 89 KiB): `day-chuyen-ra-mat-mon-moi` pipeline 7.6, `ho-so-giay-phep-fnb` 8.4, `sop-ve-sinh-an-toan-thuc-pham` 8.4,
`tinh-food-cost-mon` 7.1, `thiet-ke-menu-mo-ta-mon` 7.0, `phan-hoi-review-khieu-nai-quan` 6.9, `noi-dung-mxh-quan-an` 6.5,
`nhap-nguyen-lieu-ton-kho-bep` 6.3, `checklist-mo-dong-ca-quan` 6.3, `khuyen-mai-quan-tinh-lai` 6.4, `bao-cao-doanh-thu-quan` 6.0,
`dao-tao-nhan-vien-phuc-vu` 6.1, `xep-lich-ca-nhan-vien-quan` 6.3.

Proposed squad: `F&B Ops (Nhà hàng, quán ăn)`, floor `F&B`, 7 desks.

| Role key | Upstream agent | Kind |
| --- | --- | --- |
| `fnb_lead` | lap-ke-hoach-dieu-phoi | kv_lead |
| `fnb_reader` | doc-so-lieu-quan-an | kv_collector |
| `fnb_regulation` | tra-cuu-quy-dinh-fnb | kv_collector |
| `fnb_drafter` | soan-tai-lieu-quan-an | kv_drafter |
| `fnb_allergen_gate` | soat-di-ung-attp-mon | kv_gate |
| `fnb_margin_gate` | soat-gia-von-lai-quan | kv_gate |
| `fnb_handover` | kiem-dinh-ban-giao | kv_handover |

`builtinSkills`: `day-chuyen-ra-mat-mon-moi`. `roleSkills`: `fnb_drafter`: `thiet-ke-menu-mo-ta-mon`, `tinh-food-cost-mon`,
`noi-dung-mxh-quan-an`, `sop-ve-sinh-an-toan-thuc-pham`; `fnb_regulation`: `ho-so-giay-phep-fnb`; `fnb_reader`:
`bao-cao-doanh-thu-quan`, `nhap-nguyen-lieu-ton-kho-bep`. `deferredSkills`: the rest.
The pipeline's "Khâu" headings and its Codex/Claude web bullets are removed by the existing rules.

Leftovers: none inline after the rules (`ho-so-giay-phep-fnb` "Trên Claude Code có thể giao agent" is cut by the on-claude-prefix
rule; the scan of the generated text found no platform word left in this pack). "Lưu ý" in 3 skills, "Đạt" as a checklist column (legitimate).

### 7.4 bao-hiem (Insurance): `kv_insurance`, pack `insurance`

Trade agents: `gom-du-kien-tu-van-bao-hiem` collector; `kiem-ke-ho-so-boi-thuong` collector; `soan-de-xuat-bao-hiem` drafter;
`soan-thu-huong-dan-claim` drafter; `soat-cam-ket-tu-van` gate (advice commitments).

Skills (12, 70 KiB): `day-chuyen-de-xuat-bao-hiem` pipeline 6.8, `phan-tich-nhu-cau-bao-ve` 6.7, `so-sanh-quyen-loi-bao-hiem` 6.3,
`ho-so-boi-thuong-bao-hiem` 6.4, `kich-ban-tu-van-bao-hiem` 6.0, `ke-hoach-tuyen-dai-ly` 5.8, `giai-thich-dieu-khoan-loai-tru` 5.7,
`lich-cham-soc-nhac-phi` 5.6, `xu-ly-tu-choi-bao-hiem` 5.2, `soat-loi-tu-van-bao-hiem` 5.3, `bao-cao-tuan-kinh-doanh-bao-hiem` 5.0,
`noi-dung-mxh-bao-hiem` 5.1.

Proposed squad: `Insurance Advisory (Bảo hiểm)`, floor `Insurance`, 7 desks.

| Role key | Upstream agent | Kind |
| --- | --- | --- |
| `insurance_lead` | lap-ke-hoach-dieu-phoi | kv_lead |
| `insurance_gather` | gom-du-kien-tu-van-bao-hiem | kv_collector |
| `insurance_claim_reader` | kiem-ke-ho-so-boi-thuong | kv_collector |
| `insurance_proposal` | soan-de-xuat-bao-hiem | kv_drafter |
| `insurance_claim_drafter` | soan-thu-huong-dan-claim | kv_drafter |
| `insurance_advice_gate` | soat-cam-ket-tu-van | kv_gate |
| `insurance_handover` | kiem-dinh-ban-giao | kv_handover |

`builtinSkills`: `day-chuyen-de-xuat-bao-hiem`. `roleSkills`: `insurance_proposal`: `so-sanh-quyen-loi-bao-hiem`,
`phan-tich-nhu-cau-bao-ve`, `giai-thich-dieu-khoan-loai-tru`, `xu-ly-tu-choi-bao-hiem`; `insurance_claim_drafter`:
`ho-so-boi-thuong-bao-hiem`; `insurance_advice_gate`: `soat-loi-tu-van-bao-hiem`. `deferredSkills`: `kich-ban-tu-van-bao-hiem`, `ke-hoach-tuyen-dai-ly`,
`lich-cham-soc-nhac-phi`, `noi-dung-mxh-bao-hiem`, `bao-cao-tuan-kinh-doanh-bao-hiem`.

Risky: `soat-cam-ket-tu-van` gate (profit promises, benefit figures not in the document, unfair comparison, hidden exclusions,
pressure, false claim). It is consumer-facing advice on insurance; a person in insurance review must read its output before the
squad is on. The severity scale in the agent and in `soat-loi-tu-van-bao-hiem` is `Nặng / Vừa / Nhẹ`; the patch maps it to
`Chặn / Nên sửa / Góp ý` (section 7.4), keeping `Nặng → Chặn`, `Vừa → Nên sửa`, `Nhẹ → Góp ý`. "Vừa" in `lich-cham-soc-nhac-phi` is a
priority label, not a severity: leave it.

Leftovers: `kich-ban-tu-van-bao-hiem` and `xu-ly-tu-choi-bao-hiem` say "Claude đóng khách" (a role-play, not a platform; leave).

### 7.5 tai-chinh-ngan-hang (Banking): `kv_banking`, pack `banking`

Trade agents: `doc-ho-so-tin-dung` collector; `tra-cuu-nganh-tai-chinh` collector (+web); `lap-bang-phan-tich-tin-dung` drafter;
`soan-to-trinh-tin-dung` drafter; `soat-to-trinh-tin-dung` gate (credit memo); `soat-tuan-thu-tu-van-tai-chinh` gate (client
material and advice).

Skills (16, 96 KiB): `day-chuyen-to-trinh-tin-dung` pipeline 7.2, `phan-tich-bctc-doanh-nghiep-vay` 6.8, `nhan-dien-giao-dich-bat-thuong` 6.8,
`phan-tich-dong-tien-du-an-vay` 6.2, `to-trinh-tin-dung-khung` 6.4, `nghien-cuu-nganh-khach-hang-vay` 6.3, `so-sanh-san-pham-tien-gui-vay` 6.3,
`bang-kiem-ho-so-vay-von` 6.0, `danh-gia-tai-san-bao-dam` 6.0, `kich-ban-tu-van-khach-ngan-hang` 6.0, `lich-tra-no-khoan-vay` 6.0,
`bao-cao-danh-muc-khach-hang-rm` 5.1, `ke-hoach-tai-chinh-ca-nhan-khach` 5.7, `thu-nhac-no-khach-vay` 5.3,
`email-cham-soc-khach-vip-ngan-hang` 4.8, `tom-tat-quy-dinh-noi-bo-ngan-hang` 4.9.

Proposed squad: `Banking Credit Desk (Tài chính, ngân hàng)`, floor `Banking`, 8 desks.

| Role key | Upstream agent | Kind |
| --- | --- | --- |
| `banking_lead` | lap-ke-hoach-dieu-phoi | kv_lead |
| `banking_reader` | doc-ho-so-tin-dung | kv_collector |
| `banking_industry` | tra-cuu-nganh-tai-chinh | kv_collector |
| `banking_analyst` | lap-bang-phan-tich-tin-dung | kv_drafter |
| `banking_memo_drafter` | soan-to-trinh-tin-dung | kv_drafter |
| `banking_credit_gate` | soat-to-trinh-tin-dung | kv_gate |
| `banking_advice_gate` | soat-tuan-thu-tu-van-tai-chinh | kv_gate |
| `banking_handover` | kiem-dinh-ban-giao | kv_handover |

`builtinSkills`: `day-chuyen-to-trinh-tin-dung`. `roleSkills`: `banking_memo_drafter`: `to-trinh-tin-dung-khung`,
`bang-kiem-ho-so-vay-von`, `danh-gia-tai-san-bao-dam`; `banking_analyst`: `phan-tich-bctc-doanh-nghiep-vay`,
`phan-tich-dong-tien-du-an-vay`, `lich-tra-no-khoan-vay`; `banking_industry`: `nghien-cuu-nganh-khach-hang-vay`;
`banking_reader`: `bao-cao-danh-muc-khach-hang-rm`. `deferredSkills`: the rest (the customer-facing ones are for a sales squad).

Risky: `soat-to-trinh-tin-dung` (credit memo: source of each figure, wrong formula, missed risk, conclusion beyond authority,
data exposure) and `soat-tuan-thu-tu-van-tai-chinh` (unconfirmed rates, promised profit, investment advice, threats, exposure).
Both are read-only. A banking person reviews the gate output on the first runs.

Patches (exact text, validated on the throwaway run, not in the repo yet):

- `kitviet-patches/soat-to-trinh-tin-dung.json` and `kitviet-patches/soat-tuan-thu-tu-van-tai-chinh.json`, each:
  `[{ "find": "## Kết luận: QUA | CHẶN\n<lý do một câu>", "replace": "VERDICT: QUA|CHẶN - <lý do một câu>" }]`.

Leftovers: `day-chuyen-to-trinh-tin-dung` "Codex" and "Claude web / Desktop" bullets are cut by the rules; "Gợi ý" 3, "Lưu ý" 2.
The `phan-tich-dong-tien-du-an-vay` and `to-trinh-tin-dung-khung` skills name rates and ratios as fields to fill, with no value (keep).

### 7.6 bat-dong-san (Real Estate): `kv_realestate`, pack `realestate`

Trade agents: `doc-ho-so-nha-dat` collector; `tra-cuu-khu-vuc-bds` collector (+web); `lap-phuong-an-vay-bds` drafter;
`soan-tin-dang-bds` drafter; `soat-nguon-quy-hoach-bds` gate; `soat-phuong-an-vay-bds` gate; `soat-tin-dang-bds` gate.

Skills (18, 112 KiB): `day-chuyen-ho-so-can-ban` pipeline 8.7, `nghien-cuu-quy-hoach-khu-vuc` 8.1, `bang-kiem-phap-ly-nha-dat` 7.2,
`viet-tin-dang-nha-dat` 7.0, `tinh-phuong-an-vay-mua-nha` 6.8, `ra-soat-hop-dong-dat-coc` 6.6, `kich-ban-cho-thue-nha` 6.5,
`dinh-gia-tham-khao-nha-dat` 6.4, `ho-so-gioi-thieu-can-nha` 6.0, `kich-ban-video-tour-nha` 5.3, `so-sanh-can-ho-du-an` 5.7,
`phan-tich-khu-vuc-bds` 5.8, `xu-ly-tu-choi-khach-bds` 5.8, `cham-soc-khach-cu-bds` 5.6, `ghi-chu-gap-khach-bds` 5.2,
`ho-so-nhu-cau-khach-bds` 5.3, `cham-soc-sau-xem-nha` 5.0, `bao-cao-tuan-san-bds` 5.1.

Proposed squad: `Real Estate Desk (Bất động sản)`, floor `Real Estate`, 9 desks.

| Role key | Upstream agent | Kind |
| --- | --- | --- |
| `realestate_lead` | lap-ke-hoach-dieu-phoi | kv_lead |
| `realestate_reader` | doc-ho-so-nha-dat | kv_collector |
| `realestate_area_research` | tra-cuu-khu-vuc-bds | kv_collector |
| `realestate_loan_drafter` | lap-phuong-an-vay-bds | kv_drafter |
| `realestate_listing_drafter` | soan-tin-dang-bds | kv_drafter |
| `realestate_planning_gate` | soat-nguon-quy-hoach-bds | kv_gate |
| `realestate_loan_gate` | soat-phuong-an-vay-bds | kv_gate |
| `realestate_listing_gate` | soat-tin-dang-bds | kv_gate |
| `realestate_handover` | kiem-dinh-ban-giao | kv_handover |

`builtinSkills`: `day-chuyen-ho-so-can-ban`. `roleSkills`: `realestate_listing_drafter`: `viet-tin-dang-nha-dat`,
`ho-so-gioi-thieu-can-nha`, `kich-ban-video-tour-nha`; `realestate_loan_drafter`: `tinh-phuong-an-vay-mua-nha`;
`realestate_area_research`: `phan-tich-khu-vuc-bds`, `nghien-cuu-quy-hoach-khu-vuc`; `realestate_reader`: `ra-soat-hop-dong-dat-coc`.
`deferredSkills`: `bang-kiem-phap-ly-nha-dat`, `dinh-gia-tham-khao-nha-dat`, `kich-ban-cho-thue-nha`, `so-sanh-can-ho-du-an`, and the
sales-side skills (`xu-ly-tu-choi-khach-bds`, `cham-soc-khach-cu-bds`, `ghi-chu-gap-khach-bds`, `ho-so-nhu-cau-khach-bds`,
`cham-soc-sau-xem-nha`, `bao-cao-tuan-san-bds`).

Risky: `soat-phuong-an-vay-bds` (loan plan: rate, term, payment figures) and `soat-tin-dang-bds` (listing: legal status, price
claims). Both are gates on money claims and on the seller's documents.

Leftovers (patches): `nghien-cuu-quy-hoach-khu-vuc` line 38 "Claude Code có thể giao agent `kv:tra-cuu-khu-vuc-bds` tra web …";
`phan-tich-khu-vuc-bds` line 20 "(Claude Code)"; `ra-soat-hop-dong-dat-coc` "Phải sửa trước khi ký / Nên bổ sung / Ghi nhận" (map
to `Chặn / Nên sửa / Góp ý`, keep the section name "Chặn trước khi ký"). The description of `day-chuyen-ho-so-can-ban` reads
"theo giao thức .kv-run" and becomes "theo giao thức tệp đính kèm trên issue" by the rule: rewrite the description with a patch.
`bang-kiem-phap-ly-nha-dat` names "Luật Đất đai", "Luật Nhà ở", "Luật Kinh doanh bất động sản" with no number (kept).

### 7.7 du-lich-khach-san (Hospitality): `kv_hospitality`, pack `hospitality`

Trade agents: `gom-yeu-cau-doan-khach` collector; `tra-cuu-diem-den-du-lich` collector (+web); `soan-ho-so-tour` drafter;
`soan-noi-dung-khach-san` drafter; `soat-lich-trinh-bao-gia` gate (itinerary and quote); `soat-cam-ket-dich-vu-luu-tru` gate (promises
to guests).

Skills (15, 87 KiB): `day-chuyen-ho-so-tour-doan` pipeline 7.4, `thiet-ke-lich-trinh-tour` 6.6, `checklist-an-toan-tour` 6.3,
`bao-gia-tour-chi-phi` 6.4, `sop-buong-phong-khach-san` 5.5, `mo-ta-phong-listing-ota` 5.3, `email-xac-nhan-dat-phong` 5.9,
`xu-ly-overbooking-khieu-nai` 5.9, `de-xuat-tour-mice-doan-cong-ty` 5.7, `kich-ban-le-tan-check-in-out` 5.7, `gia-phong-theo-mua` 5.4,
`bao-cao-cong-suat-phong` 5.1, `thuyet-minh-diem-tham-quan` 5.1, `tra-loi-review-khach-san` 5.1, `nghien-cuu-diem-den-du-lich` 5.8.

Proposed squad: `Hospitality Ops (Du lịch, khách sạn)`, floor `Hospitality`, 8 desks.

| Role key | Upstream agent | Kind |
| --- | --- | --- |
| `hospitality_lead` | lap-ke-hoach-dieu-phoi | kv_lead |
| `hospitality_intake` | gom-yeu-cau-doan-khach | kv_collector |
| `hospitality_destination` | tra-cuu-diem-den-du-lich | kv_collector |
| `hospitality_tour_drafter` | soan-ho-so-tour | kv_drafter |
| `hospitality_hotel_drafter` | soan-noi-dung-khach-san | kv_drafter |
| `hospitality_quote_gate` | soat-lich-trinh-bao-gia | kv_gate |
| `hospitality_promise_gate` | soat-cam-ket-dich-vu-luu-tru | kv_gate |
| `hospitality_handover` | kiem-dinh-ban-giao | kv_handover |

`builtinSkills`: `day-chuyen-ho-so-tour-doan`. `roleSkills`: `hospitality_tour_drafter`: `bao-gia-tour-chi-phi`,
`thiet-ke-lich-trinh-tour`, `checklist-an-toan-tour`; `hospitality_hotel_drafter`: `mo-ta-phong-listing-ota`,
`email-xac-nhan-dat-phong`, `tra-loi-review-khach-san`; `hospitality_intake`: `de-xuat-tour-mice-doan-cong-ty`;
`hospitality_destination`: `nghien-cuu-diem-den-du-lich`, `thuyet-minh-diem-tham-quan`. `deferredSkills`: the rest.

Risky: `soat-cam-ket-dich-vu-luu-tru` (promised refunds and deposits not approved, policy errors, guest data). Its severity scale
is `Nặng / Vừa / Nhẹ` in the agent (patch to `Chặn / Nên sửa / Góp ý`). `soat-lich-trinh-bao-gia` uses the same scale (patch).

Leftovers: "Gợi ý" 3; `day-chuyen-ho-so-tour-doan` line 51 "Claude web / desktop" bullet removed by the rule.

### 7.8 xuat-nhap-khau (Import & Export): `kv_trade`, pack `trade`

Trade agents: `gom-chung-tu-lo-hang` collector; `tra-cuu-chinh-sach-hai-quan` collector (+web); `lap-bang-doi-chieu-xnk` drafter;
`soat-chung-tu-xnk` gate (documents); `soat-khai-bao-hai-quan` gate (customs declaration).

Skills (14, 88 KiB): `day-chuyen-ho-so-lo-hang` pipeline 8.1, `tra-cuu-ma-hs-chinh-sach` 7.3, `soat-bo-chung-tu-xnk` 7.2,
`chuan-bi-to-khai-hai-quan` 7.2, `soat-dieu-khoan-lc` 6.6, `xu-ly-claim-hang-hu-hong` 6.7, `giai-thich-incoterms` 6.2,
`chon-uu-dai-thue-fta` 6.1, `ra-soat-hop-dong-ngoai-thuong` 5.9, `email-dam-phan-nha-cung-cap` 5.7, `tinh-landed-cost` 5.5,
`so-sanh-bao-gia-cuoc` 5.3, `bao-cao-lo-hang-cho-khach` 5.0, `theo-doi-booking-lich-tau` 4.9.

Proposed squad: `Import & Export Desk (Xuất nhập khẩu)`, floor `Import & Export`, 7 desks.

| Role key | Upstream agent | Kind |
| --- | --- | --- |
| `trade_lead` | lap-ke-hoach-dieu-phoi | kv_lead |
| `trade_docs_reader` | gom-chung-tu-lo-hang | kv_collector |
| `trade_customs_research` | tra-cuu-chinh-sach-hai-quan | kv_collector |
| `trade_crosscheck_drafter` | lap-bang-doi-chieu-xnk | kv_drafter |
| `trade_docs_gate` | soat-chung-tu-xnk | kv_gate |
| `trade_customs_gate` | soat-khai-bao-hai-quan | kv_gate |
| `trade_handover` | kiem-dinh-ban-giao | kv_handover |

Seven desks including the lead. `builtinSkills`: `day-chuyen-ho-so-lo-hang`. `roleSkills`: `trade_crosscheck_drafter`:
`chuan-bi-to-khai-hai-quan`, `tinh-landed-cost`, `bao-cao-lo-hang-cho-khach`; `trade_customs_research`: `tra-cuu-ma-hs-chinh-sach`,
`chon-uu-dai-thue-fta`; `trade_docs_gate`: `soat-bo-chung-tu-xnk`, `soat-dieu-khoan-lc`. `deferredSkills`: the rest.

Risky: none flagged as advice; the two gates are on documents and declarations (HS code, value, tax: the agents do not fill a rate,
the text says so). Tax rates and exchange rates come from the user's own tables only.

Leftovers: "Gợi ý" 3, "Lưu ý" in several skills. "CLAUDE.md" in 3 places is rewritten to the KB name by the rule.

### 7.9 san-xuat-nha-may (Manufacturing): `kv_factory`, pack `factory`

Trade agents: `doc-du-lieu-san-xuat` collector; `tra-cuu-tieu-chuan-nha-may` collector (+web); `lap-bang-so-lieu-san-xuat` drafter;
`soan-ho-so-nha-may` drafter; `soat-so-lieu-chat-luong` gate (production numbers); `soat-an-toan-nha-may` gate (safety, authority).

Skills (15, 92 KiB): `day-chuyen-bao-cao-kpi-nha-may` pipeline 7.9, `chuan-bi-danh-gia-iso` 7.6, `danh-gia-rui-ro-an-toan-lao-dong` 6.5,
`bao-cao-ncr-capa` 6.3, `bao-cao-su-co-tai-nan-nha-may` 6.8, `phan-tich-nguyen-nhan-goc-loi` 6.1, `ke-hoach-kiem-tra-dau-vao-iqc` 5.7,
`ke-hoach-bao-tri-phong-ngua` 6.2, `ke-hoach-san-xuat-tuan-theo-may` 5.8, `tinh-oee-tu-so-lieu` 5.8, `soan-wi-sop-tai-may` 5.8,
`bao-cao-ca-san-xuat` 5.4, `danh-gia-nha-cung-cap-vat-tu` 5.4, `checklist-5s-nha-xuong` 5.2, `dao-tao-cong-nhan-moi` 6.0.

Proposed squad: `Factory Ops (Sản xuất nhà máy)`, floor `Manufacturing`, 8 desks.

| Role key | Upstream agent | Kind |
| --- | --- | --- |
| `factory_lead` | lap-ke-hoach-dieu-phoi | kv_lead |
| `factory_reader` | doc-du-lieu-san-xuat | kv_collector |
| `factory_standards` | tra-cuu-tieu-chuan-nha-may | kv_collector |
| `factory_kpi_drafter` | lap-bang-so-lieu-san-xuat | kv_drafter |
| `factory_doc_drafter` | soan-ho-so-nha-may | kv_drafter |
| `factory_numbers_gate` | soat-so-lieu-chat-luong | kv_gate |
| `factory_safety_gate` | soat-an-toan-nha-may | kv_gate |
| `factory_handover` | kiem-dinh-ban-giao | kv_handover |

`builtinSkills`: `day-chuyen-bao-cao-kpi-nha-may`. `roleSkills`: `factory_kpi_drafter`: `tinh-oee-tu-so-lieu`, `bao-cao-ca-san-xuat`;
`factory_doc_drafter`: `bao-cao-ncr-capa`, `bao-cao-su-co-tai-nan-nha-may`, `phan-tich-nguyen-nhan-goc-loi`; `factory_standards`:
`chuan-bi-danh-gia-iso`; `factory_reader`: `ke-hoach-kiem-tra-dau-vao-iqc`. `deferredSkills`: the rest.

Risky: `soat-an-toan-nha-may` (safety and authority: a claim of a rule without a source). Not on the spec 9.2 list; flagged here.

Leftovers: `ke-hoach-kiem-tra-dau-vao-iqc` uses `Nặng` / `Nhẹ` as a column of severity in its table (patch to `Chặn` / `Góp ý`).
"Gợi ý" 1, "Đạt / Chưa đạt" in `dao-tao-cong-nhan-moi` (a training table, legitimate).

### 7.10 giao-duc (Education): `kv_education`, pack `edu`

Trade agents: `soan-de-va-dap-an` drafter; `soan-van-ban-chu-nhiem` drafter; `soat-an-toan-hoc-sinh` gate (school text, student
data); `soat-dap-an-de-thi` gate (answers); `soat-nguon-tu-lieu-giang-day` gate (source of teaching material); `tim-tu-lieu-bai-giang`
collector (+web); `tong-hop-ho-so-lop-hoc` collector; `tra-yeu-cau-can-dat` collector (+web).

Skills (21, 142 KiB): `day-chuyen-de-kiem-tra-dinh-ky` pipeline 8.6, `ke-hoach-bai-day-gdpt` 7.8, `ma-tran-dac-ta-de-kiem-tra` 6.8,
`sang-kien-kinh-nghiem` 7.1, `ngan-hang-cau-hoi-trac-nghiem` 6.1, `rubric-cham-bai-hoc-sinh` 5.4, `ke-hoach-on-tap-cuoi-ky` 5.3,
`nghien-cuu-tu-lieu-bai-giang` 6.2, `hop-phu-huynh-lop` 6.0, `tin-nhan-phu-huynh-zalo` 5.9, `nhan-xet-hoc-ba` 6.0,
`phan-hoi-bai-viet-hoc-sinh` 6.4, `bao-cao-to-chuyen-mon` 6.3, `ke-hoach-chu-nhiem-lop` 5.8, `tro-choi-khoi-dong-on-tap` 5.2,
`bai-trinh-chieu-tu-giao-an` 5.0, `de-cuong-chuyen-de-bai-giang` 5.5, `de-cuong-nckh-su-pham-ung-dung` 6.4, `phieu-hoc-tap-nhom` 5.4,
`day-hoc-phan-hoa-trinh-do` 5.6, `viet-giong-gen-z` (owned by marketing; not listed here).

Proposed squad: `Education Desk (Giáo dục)`, floor `Education`, 6 desks (lead, four trade roles, handover).

| Role key | Upstream agent | Kind |
| --- | --- | --- |
| `edu_lead` | lap-ke-hoach-dieu-phoi | kv_lead |
| `edu_curriculum` | tra-yeu-cau-can-dat | kv_collector |
| `edu_test_drafter` | soan-de-va-dap-an | kv_drafter |
| `edu_answer_gate` | soat-dap-an-de-thi | kv_gate |
| `edu_student_gate` | soat-an-toan-hoc-sinh | kv_gate |
| `edu_handover` | kiem-dinh-ban-giao | kv_handover |

Five roles, plus the lead: 6 desks. `builtinSkills`: `day-chuyen-de-kiem-tra-dinh-ky`. `roleSkills`: `edu_test_drafter`:
`ma-tran-dac-ta-de-kiem-tra`, `ngan-hang-cau-hoi-trac-nghiem`, `ke-hoach-on-tap-cuoi-ky`, `rubric-cham-bai-hoc-sinh`;
`edu_curriculum`: `ke-hoach-bai-day-gdpt`, `day-hoc-phan-hoa-trinh-do`. `deferredSkills`: `phieu-hoc-tap-nhom`,
`de-cuong-chuyen-de-bai-giang`, `nghien-cuu-tu-lieu-bai-giang`, `nhan-xet-hoc-ba`, `hop-phu-huynh-lop`, `tin-nhan-phu-huynh-zalo`,
`bao-cao-to-chuyen-mon`, `sang-kien-kinh-nghiem`, `ke-hoach-chu-nhiem-lop`, `tro-choi-khoi-dong-on-tap`, `bai-trinh-chieu-tu-giao-an`,
`de-cuong-nckh-su-pham-ung-dung`, `phan-hoi-bai-viet-hoc-sinh`. Second squad for the four agents not mapped: section 6.5.

Risky: `soat-an-toan-hoc-sinh` (child safety, student data, a gate that must be read by a school person before use).

Leftovers (patches): `soat-an-toan-hoc-sinh` "Gửi được / Chưa gửi được" and "Nghiêm trọng" (map to `Chặn / Góp ý`; patch the verdict
line through the template, and the "Kết luận: **Gửi được** chỉ khi …" line by a patch); `nghien-cuu-tu-lieu-bai-giang` line 40 "(Claude Code/Codex)"
and "Claude web/desktop tự tra …" (patch); "Nghiêm trọng" in `soat-dap-an-de-thi` and `soat-nguon-tu-lieu-giang-day`; "Đạt" in grade rubrics
(legitimate: school grade "Tốt/Đạt/Chưa đạt"). `tro-choi-khoi-dong-on-tap` "Vừa thời lượng" is plain text (keep).

## 8. Recipe for one P2 squad (what the squad agent creates)

Same as `kitviet-p1-notes.md` section 2, with these P2 specifics. Pack id = config file name: `health`, `construction`, `fnb`,
`insurance`, `banking`, `realestate`, `hospitality`, `trade`, `factory`, `edu`.

1. `packages/core/squads/kitviet-squads/<pack>.json`: `order`, `templateId` (`kv_<x>`), `trade`, `name` (English (Vietnamese)), `agents` (the
   table of the squad), `builtinSkills`, `leaderSkills: ["kv-squad-lead", "kv-company-profile"]`, `roleSkills`, `promoted: []`, `deferredSkills`.
   `flow` stays `pipeline`.
2. `packages/core/squads/kitviet-patches/<skill or agent>.json` for the leftovers of its skills (exact `find`, must occur once). Start
   from section 7; the rewrite rules cover the file names, `.kv-run`, `CLAUDE.md`, `pip install`, and the pipeline platform bullets.
   The banking verdict patch (section 7.5) goes with the banking squad.
3. Run `pnpm generate:kitviet` until it passes, then `pnpm generate:kitviet --check`.
4. `packages/core/squads/kitviet-templates/<x>.ts` copied from `admin.ts`, with `floorName: KITVIET_P2_FLOOR_NAMES.<key>` and the squad's
   `KV_<X>_ROLE_KEYS` / `KV_<X>_ROLES` (role keys from section 7, prefix `health_`, `construction_`, `fnb_`, `insurance_`, `banking_`,
   `realestate_`, `hospitality_`, `trade_`, `factory_`, `edu_`).
5. Register in `templates.ts` (`SquadTemplateId`, `SQUAD_TEMPLATES`, `SQUAD_ROLES`); locales (`modals.json` key `templates.kv_<x>`, seven
   languages) and the runbook row, as P1.
6. `kitviet-templates/<x>.test.ts` from `admin.test.ts`, with the floor check `expect(KITVIET_P2_FLOOR_NAME_LIST).toContain(t.floorName)`
   in place of the default-floor check.
7. Checks: `pnpm generate:kitviet --check`, `node --test scripts/generate-kitviet-pack.test.mjs`, `cd packages/core && npx vitest run squads`,
   `npx tsc --noEmit`.

Squad-level rules: at most 9 roles including the lead (the generator fails above that); at most 6 role skills per role; a gate has
exactly one VERDICT line (the template gives it; a gate with its own heading needs the patch of section 7.5).

## 9. Proposal summary

| Trade | Squad id (`templateId`) | Name | Floor | Desks | Gates (risk) | Pipeline |
| --- | --- | --- | --- | --- | --- | --- |
| y-te | `kv_health` | Healthcare (Y tế) | Healthcare | 9 | scope, patient data (clinician review) | day-chuyen-tai-lieu-y-te |
| xay-dung | `kv_construction` | Construction (Xây dựng) | Construction | 8 | quantity, site safety | day-chuyen-ho-so-cong-truong |
| nha-hang-fnb | `kv_fnb` | F&B Ops (Nhà hàng, quán ăn) | F&B | 7 | allergen, food cost | day-chuyen-ra-mat-mon-moi |
| bao-hiem | `kv_insurance` | Insurance Advisory (Bảo hiểm) | Insurance | 7 | advice commitments (review) | day-chuyen-de-xuat-bao-hiem |
| tai-chinh-ngan-hang | `kv_banking` | Banking Credit Desk (Tài chính, ngân hàng) | Banking | 8 | credit memo, advice (review) | day-chuyen-to-trinh-tin-dung |
| bat-dong-san | `kv_realestate` | Real Estate Desk (Bất động sản) | Real Estate | 9 | planning source, loan, listing | day-chuyen-ho-so-can-ban |
| du-lich-khach-san | `kv_hospitality` | Hospitality Ops (Du lịch, khách sạn) | Hospitality | 8 | quote, guest promises | day-chuyen-ho-so-tour-doan |
| xuat-nhap-khau | `kv_trade` | Import & Export Desk (Xuất nhập khẩu) | Import & Export | 7 | documents, customs declaration | day-chuyen-ho-so-lo-hang |
| san-xuat-nha-may | `kv_factory` | Factory Ops (Sản xuất nhà máy) | Manufacturing | 8 | production numbers, safety | day-chuyen-bao-cao-kpi-nha-may |
| giao-duc | `kv_education` | Education Desk (Giáo dục) | Education | 6 | answer key, student data | day-chuyen-de-kiem-tra-dinh-ky |

The "Desks" column counts the lead. Every squad has the shared handover (`kiem-dinh-ban-giao`) and no shared gate beyond it.

## 10. Integration (2026-11-11): what changed from the proposals

- **Squads registered.** `templates.ts` lists the ten squads after P1 (`SquadTemplateId`, `SquadRoleKey`, `SQUAD_ROLES`, `SQUAD_TEMPLATES`). The quickstart modal, the company card and the tests read the list generically; no other consumer names a squad.
- **Lead rules.** The `*_LEAD_NOTES` of the ten templates moved verbatim into `KV_LEAD_NOTES` (`kitviet-agent-instructions.ts`), keyed by template id. Every P2 template uses `kvLeadInstructions`. The tests read the notes from `KV_LEAD_NOTES`.
- **Floor cap.** 16 → 24 (`CompanyMaxFloors`, `companyMaxFloors`, `COMPANY_MAX_FLOORS`, the error text, the docs). Decision Q11 in `kitviet-squads.md`.
- **Gate rules (Q12).** A gate that blocks a claim without a source is the rule for the safety, commitment and money gates. The changes by gate, in the patch files of `kitviet-patches/`:
  - Healthcare scope gate: unsourced claims and comparisons, a missing safety line and a claim outside the source are Chặn or Cần giấy tờ. Healthcare privacy gate: an indirect identifier is Cần giấy tờ.
  - Construction quantity gate: Cần giấy tờ blocks (its verdict was only Chặn). Construction safety gate: Cần giấy tờ blocks; the verdict line no longer says "Đạt".
  - F&B allergen gate: cross-contamination in the source is Chặn; an unsourced health claim or regulation is Cần giấy tờ.
  - Insurance advice gate: Vừa became Chặn (no tier below Chặn; "hứa kết quả bồi thường" and pressure are not Nên sửa).
  - Hospitality quote gate: Vừa became Cần giấy tờ (a missing price source or confirmation blocks). Hospitality promise gate: Vừa became Cần giấy tờ (a claim without a basis).
  - Realestate loan gate: a rate with no bank source (GIẢ ĐỊNH) and an unconditional "chắc chắn duyệt" block. Realestate listing gate: "Không có trong nguồn" and "Tự suy" block.
  - Customs declaration gate: an overstated claim with no level-1 source blocks. Factory safety gate: Cần giấy tờ blocks.
  - Trade docs gate and Education student gate: the old "Nghiêm trọng" and "Gửi được" scale is replaced by Chặn and QUA.
  - Factory numbers gate: unchanged (P0 rule: a figure with no source does not block; that gate is shared with P0).
- **Frontmatter.** The gate description of `soat-dap-an-de-thi` said "Kết luận Đạt/Chưa đạt"; the rewrite table (`gate-description-verdict`) rewrites it to QUA/CHẶN. The "history" wording of the Customs Department ("Cục Hải quan (trước đây là Tổng cục Hải quan)") is in the body of `tra-cuu-chinh-sach-hai-quan` and `tra-cuu-ma-hs-chinh-sach`, not in a frontmatter description, so the rewrite does not touch it; left as the upstream wrote it.
- **Display names.** `fnb` is an acronym (`F&B`): `F&B Lead`, `F&B Allergen Gate`. `&` is safe: role names are display text, the role key is the id. `edu` stays `Edu`.
- **Role skills.** The per-role skills follow the pack configs, which differ from the lists in sections 7.1 to 7.10 (for example Education's test drafter takes three skills, Healthcare's drafter takes five). The runbook lists the configured skills.
- **Copy.** Ten squads in seven languages: names keep "English (Vietnamese)"; descriptions follow each template's English proposal, checked against the UI description rules.
- **Floors.** Quickstart creates a P2 floor when missing, reuses it when present, and seats the squad at the first free slot. Tested for all ten squads (`quickstart.test.ts`).
- **Expert review (spec Q13).** `expertReview: true` on the six templates whose gates need a trade reader (runbook 8d). The quickstart
  picker shows a badge and a confirmation checkbox that gates Create; the server does not read the flag.
- **Picker groups.** `SQUAD_TEMPLATE_CATEGORIES` (`core`, `kitviet`, `vertical`) with `squad_quickstart.groups` labels in seven languages.
- **Open.** Live runs (S7) of all eleven, to be done last and only with the user's go-ahead for real agents.

## 11. Teaching Materials (`kv_teaching`, 2026-11-11)

- **Why a squad.** Education Desk maps four of giao-duc's eight trade agents. The other four form a second squad with its own
  purpose: sources with levels, coded class records, class documents for a teacher, and three gates (sources, student text,
  figures), plus the lead and the handover. Eight desks in all (the cap is nine).
- **Floor.** Shares the Education floor with Education Desk (`KITVIET_P2_FLOOR_NAMES.edu`). This is the only shared floor; the
  quickstart seats the second squad on the next free slot. The floor-uniqueness test allows exactly this pair (`quickstart.test.ts`).
- **Expert review: no.** The gates check sources and student text, but no licensed professional's signature is needed. The student-data
  gate needs a school reader before the first runs (runbook section "Teaching Materials").
- **Records.** Class records are read only when the request says they are anonymised or approved (stop type 1 at intake). The lead's
  rules are `KV_LEAD_NOTES.kv_teaching`; `TEACHING_LEAD_NOTES` is gone.
- **Tests.** `kitviet-templates/teaching.test.ts`, the quickstart P2 test (eleven squads, one shared floor), and the generator tests.
