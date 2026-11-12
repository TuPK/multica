# Kit Việt sample workspace (21 squads)

Status: seeded on the v1.4.0 pack. The environment was destroyed on 2026-11-12. The pack is now upstream v1.5.0 (`505a472685c85ecefd4eb127ff016b988226ad05`), and the sample was not seeded again on it. The 21 Kit Việt squads existed in the workspace below, instantiated through the quickstart code path. **No agent was run: no task was assigned and no issue exists in the workspace (0 issues).** The squads are inert until a request is assigned to one. Follow-up applied: the Operating review autopilots of Legal and Accounting are now paused, and the Office Admin squad and its seven agents carry the instructions of commit `1cae0e31e` (see Follow-up).

## Environment

| Item | Value |
| --- | --- |
| Env name | `multica_kitviet-332` (worktree `/home/av-server/Documents/Github/multica-kitviet`, branch `kitviet-pack`) |
| API | http://localhost:18412 (health: commit `1e2ba5a19`) |
| Web (app) | http://localhost:13332 (**stopped** at the time of writing; see How to open) |
| Database | `multica_multica_kitviet_332` |
| CLI profile | `dev-multica_kitviet-332` |
| Runtime used by all 166 agents | `Claude (av-server-H410M-DS2V)`, id `75c295e8-70ec-4e11-b3a0-de89cbefc780` |
| Daemon | running (`dev-multica_kitviet-332`); restarted once after the workspace was created so it registers the new workspace's runtimes |

## Workspace

| Item | Value |
| --- | --- |
| Name | `Kit Việt - bộ mẫu` |
| Slug | `kitviet-mau` |
| Workspace id | `e56512a8-bd7a-48be-b0a3-09db73e57d42` |
| Issue prefix | `KVM` |
| Created | 2026-11-11 15:46 (+07:00) with `multica workspace create` |
| Other workspaces in this env (untouched) | `Dev`, `kit-s7-drill-0511`, `kit-s7-drill-0511b`, `kit-s7-final` |

## How to open

The app routes are `/{slug}/...`. With the web app running on port 13332:

- Squads list: http://localhost:13332/kitviet-mau/squads
- Squad detail: http://localhost:13332/kitviet-mau/squads/{squad id from the table below}
- Company floor plan: http://localhost:13332/kitviet-mau/company
- Autopilots: http://localhost:13332/kitviet-mau/autopilots

Web is not running. Start it from the worktree with `make up C=web` (or `make up C=api,web`); `make status` should show it running first. Log in as the env's dev user (`dev@localhost`).

## Floors

15 floors, of a cap of 24 (`CompanyMaxFloors`). The Education floor is shared by `kv_education` and `kv_teaching`, as the runbook requires.

| Floor | Squads on it | Floor id |
| --- | --- | --- |
| Executive | 2: kv_admin, kv_officeops | `11674ac2-68c2-4efb-9ae2-63e0cbe36ac1` |
| Launch | 3: kv_b2b, kv_marketing, kv_pr | `110e5556-2b44-4935-ba43-85507a8a95ae` |
| Governance | 3: kv_legal, kv_accounting, kv_talent | `c9117c1e-8d36-4120-be79-0a7110d9e64f` |
| Revenue | 1: kv_ecom | `d79f69a3-8339-42ba-87f4-e1ebfeec3b24` |
| Research | 1: kv_academic | `0eebfdfd-5d05-4120-a2a9-5284fd57cce9` |
| Healthcare | 1: kv_health | `7499f5dc-70a2-4e01-ae3b-186bab6587f9` |
| Construction | 1: kv_construction | `2353afb5-22c5-4503-8816-f697ed8bd28d` |
| F&B | 1: kv_fnb | `7953b735-d885-4058-a4fc-a05cb2f71da7` |
| Insurance | 1: kv_insurance | `3d820481-e7ff-4da6-ab7d-92d24c044919` |
| Banking | 1: kv_banking | `7576da7e-fa14-4888-8bc0-f14946d0f520` |
| Real Estate | 1: kv_realestate | `5317d129-4c6f-4881-8e6b-3c35a89a0b0b` |
| Hospitality | 1: kv_hospitality | `df28d5a5-cde7-4354-b611-0ec0f7cbb385` |
| Import & Export | 1: kv_trade | `cc461521-64eb-494d-8219-c1b91bc4096e` |
| Manufacturing | 1: kv_factory | `b872a3e8-db11-4f1a-8f9c-4e09ec102522` |
| Education | 2: kv_education, kv_teaching | `78487235-f04d-49e1-a24b-ab641c353241` |

## The 21 squads

Columns: squad skills = the squad's `builtinSkills` count; lead skills = skills on the lead agent (`leaderSkills` plus any `roleSkills` of the leader); all agent skills = sum over the squad's agents. Agent counts are the squad roster, all on the Claude runtime above.

| # | Template | Squad id | Squad name | Floor (seat) | Leader (agent id) | Agents | Squad skills | Lead skills | All agent skills | Routine | Operating review |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | `kv_admin` | `eb636e74-97a1-487e-9fce-5bc54dbd80c7` | Office Admin (Hành chính) | Executive (0) | Admin Lead (`7be0b673-1553-4c0a-960b-5db579da426d`) | 7 | 23 | 3 | 6 | none | none |
| 2 | `kv_b2b` | `76c4a115-e960-41f9-81bb-fe277ac785d2` | B2B Proposal Desk (Chào giá B2B) | Launch (0) | B2B Lead (`300c13ce-38d1-43ff-94cf-6166b1f74e68`) | 9 | 21 | 2 | 4 | none | none |
| 3 | `kv_marketing` | `472a832a-920d-40c9-9f21-d191f644de12` | Marketing Studio (Marketing) | Launch (1) | Mkt Lead (`dbadf4b2-b7d2-48e9-a663-2606a0ff4af6`) | 9 | 24 | 3 | 19 | none | none |
| 4 | `kv_legal` | `f442482a-8bd1-4fd5-af9f-6edad260b2b5` | Legal & Contracts (Pháp lý) | Governance (0) | Legal Lead (`038b2cb5-97e6-4a97-8e19-876273adeb66`) | 8 | 24 | 2 | 10 | Rà hiệu lực sổ quy định (paused) | paused |
| 5 | `kv_accounting` | `0ea58d65-962c-4079-bb80-2080b7f19891` | Accounting & Tax (Kế toán, thuế) | Governance (1) | Acct Lead (`59a78e22-d1da-412d-af3e-e408751a92b0`) | 9 | 28 | 2 | 9 | Lịch hạn nộp thuế (paused) | paused |
| 6 | `kv_officeops` | `5b6d5262-586c-4312-856d-95b247488e70` | Office Ops (Vận hành văn phòng) | Executive (1) | Officeops Lead (`2a90df7c-545c-46b7-b814-e80d62c22e6d`) | 5 | 19 | 2 | 8 | none | none |
| 7 | `kv_talent` | `b80260dc-d15c-4083-947b-cba8e7008d1e` | Talent Acquisition (Tuyển dụng) | Governance (2) | Talent Lead (`718a659c-e093-4ec5-90a8-634387546f9d`) | 8 | 17 | 2 | 7 | none | none |
| 8 | `kv_pr` | `01aab254-5cc9-47c4-b89f-871c1757ddff` | PR & Comms (Truyền thông báo chí) | Launch (2) | PR Lead (`5e6c51f5-7d55-4c28-b001-f2e5808a0f49`) | 9 | 21 | 3 | 9 | none | none |
| 9 | `kv_ecom` | `aed86027-5b59-435d-9295-947b9547817f` | E-commerce Shop Ops (Bán hàng online) | Revenue (0) | Ecom Lead (`dd30fa0c-9e7d-4e55-b3a2-5b9a4520b804`) | 9 | 17 | 2 | 14 | none | none |
| 10 | `kv_academic` | `444fa9e9-d79a-4ef7-8c91-94506b8afc61` | Academic Evidence Desk (Học thuật, nghiên cứu) | Research (0) | Acad Lead (`6e6321a7-3019-41f8-b1d6-404d595ea1d6`) | 8 | 18 | 2 | 10 | none | none |
| 11 | `kv_health` | `92ecfe7e-b3ff-42a0-b326-2a28567067b4` | Healthcare (Y tế) | Healthcare (0) | Health Lead (`6278c62a-bc3b-4b13-89ad-3f8be530823e`) | 9 | 17 | 2 | 14 | none | none |
| 12 | `kv_construction` | `749bf7f4-c646-4e6e-8371-733434d44f38` | Construction (Xây dựng) | Construction (0) | Construction Lead (`484c33eb-81c8-4e8a-a601-6a885c1b0115`) | 8 | 17 | 2 | 12 | none | none |
| 13 | `kv_fnb` | `38fc9e2f-f710-44d6-9f3a-36ec2004a738` | F&B Ops (Nhà hàng, quán ăn) | F&B (0) | F&B Lead (`4edae329-cc65-43fa-ad7d-0cdbcfee4614`) | 7 | 17 | 2 | 11 | none | none |
| 14 | `kv_insurance` | `ec4465ef-838e-4dca-8f34-9bdfee557a75` | Insurance Advisory (Bảo hiểm) | Insurance (0) | Insurance Lead (`66be4388-c562-47e1-be83-49720d85c8e1`) | 7 | 17 | 2 | 8 | none | none |
| 15 | `kv_banking` | `d219c8b1-22b3-4a1c-8621-f9b067e9ac45` | Banking Credit Desk (Tài chính, ngân hàng) | Banking (0) | Banking Lead (`9977dd80-6187-406b-9ab2-3943853386ad`) | 8 | 17 | 2 | 10 | none | none |
| 16 | `kv_realestate` | `9e8da0fb-0cc4-47b5-9294-a7b704190f5d` | Real Estate Desk (Bất động sản) | Real Estate (0) | Realestate Lead (`f318b9d8-48f4-4de2-8cd1-e3412ca92b10`) | 9 | 17 | 2 | 9 | none | none |
| 17 | `kv_hospitality` | `80f02aac-8af2-4234-a04f-74d80480406e` | Hospitality Ops (Du lịch, khách sạn) | Hospitality (0) | Hospitality Lead (`57171eb8-7934-48df-a3b4-c143b12a4e16`) | 8 | 17 | 2 | 11 | none | none |
| 18 | `kv_trade` | `89b53a9e-56ce-46d9-a79d-020b8ad006ec` | Import & Export Desk (Xuất nhập khẩu) | Import & Export (0) | Trade Lead (`989bf4b3-f0f7-49c1-a2d4-9ef9ae060a22`) | 7 | 17 | 2 | 9 | none | none |
| 19 | `kv_factory` | `b2f9edfb-7f19-493c-9011-15e83c8320be` | Factory Ops (Sản xuất nhà máy) | Manufacturing (0) | Factory Lead (`fed1518b-c42f-40c1-819b-d0efd0726b5d`) | 8 | 17 | 2 | 9 | none | none |
| 20 | `kv_education` | `522bc126-2f01-4db5-8f7f-6310149f1403` | Education Desk (Giáo dục) | Education (0) | Edu Lead (`869edc4d-774b-4dbf-9bf8-b51c665e0963`) | 6 | 17 | 2 | 5 | none | none |
| 21 | `kv_teaching` | `dfc790e9-13ac-4bd4-8c1f-036b418e53fe` | Teaching Materials (Tư liệu giảng dạy) | Education (1) | TM Lead (`be354338-feb8-404d-a23b-d73454ffd872`) | 8 | 16 | 2 | 7 | none | none |

Expert review: six squads have `expertReview` in their template (`kv_health`, `kv_construction`, `kv_fnb`, `kv_insurance`, `kv_banking`, `kv_factory`). The modal's acknowledgement checkbox is UI-only; this run called `runSquadQuickstart` directly, so the acknowledgement was skipped on purpose by the operator.

## Creation

- Driver: `runSquadQuickstart` from `packages/core/squads/quickstart.ts`, one template at a time, through `ApiClient` (`packages/core/api/client.ts`) with the CLI profile token. Options sent: `runtimeId` and `createWorkflow: !!template.workflow`, the same as `squad-quickstart.tsx`. No `timezone` was sent, so the browser default applied (the routines show `Asia/Saigon`).
- The first run (`kv_admin`) was a probe; the other 20 ran after it in one sequential pass. Every call returned squad `created`, all agents `created`, skills `ready`, routines as listed. Errors: none. Failed agents: none. Partial skills: none.
- Workflow: no Kit Việt template has a workflow, so `workflow` is `skipped` for all 21.
- Scripts (scratchpad, not in the repo): `/tmp/claude-1000/-home-av-server-Documents-Github-multica/445e7c1c-9a4b-4b5a-ac6b-14e714f00c7c/scratchpad/kv/run-all.ts` (creation), `verify.ts` and `auto.ts` (checks). Raw per-template outcomes are in `.../scratchpad/kv/outcomes/`.

## Verification (real counts)

| Check | Result |
| --- | --- |
| Squads in the workspace | 21 (one per template, each name matched exactly once) |
| Squad leader matches the template leader role | 21 of 21 |
| Roster: every template role exists as a named agent, and is a squad member | 21 of 21 (166 agents = 166 squad members; 166 agents in the workspace in total) |
| Agents on the Claude runtime | 166 of 166 |
| Squad skills equal `builtinSkills` | 21 of 21 |
| Lead and role agent skills equal `leaderSkills` and `roleSkills` | 166 of 166 agents (no mismatch) |
| Floors | 15 (Executive, Launch, Governance, Revenue, Research and the 10 P2 floors); all squads sit on their template's floor; no two squads share a seat |
| Education floor shared by `kv_education` (seat 0) and `kv_teaching` (seat 1) | yes |
| Routines (`startPaused`) | `kv_legal`: `Rà hiệu lực sổ quy định` paused, schedule `0 8 * * 1` Asia/Saigon; `kv_accounting`: `Lịch hạn nộp thuế` paused, same schedule. Exactly one paused routine each |
| 'Operating review' autopilot on any other squad | none (19 squads) |
| Operating review on Legal and Accounting | one each, **paused** (follow-up; was active, created by the server default at first save, schedule `0 9 * * 1` UTC) |
| Autopilots in the workspace | 4, all paused (2 routines, 2 Operating reviews); active: 0 |
| Issues in the workspace | 0 |

## Follow-up (coordinator request)

Done through the API only; no squad was re-created. Workspace HEAD for the composed text: `1cae0e31e`.

1. **Operating review paused.** `Operating review` autopilots of `kv_legal` (`1996efab-0a30-41ac-8202-ad37700817ef`) and `kv_accounting` (`fdb926ae-a260-429d-9075-63e2f1dde20d`) set from active to `paused` with `PATCH /api/autopilots/{id}`. Nothing deleted. Read-back: 2 of 2 paused, 0 active reviews in any squad; the 2 routines stay paused.
2. **Office Admin instructions updated** to the composed text from the worktree code (`kvLeadInstructions("admin_lead")` for the lead, `instructionsFor(role)` for the others, which is `kvRoleInstructionsWithBody` with `KV_ROLE_NOTES`):
   - Squad `Office Admin (Hành chính)` `de1b45c7-8225-47bc-9191-4e2ca8ac764d`: squad instructions set to the lead text (6992 characters, equal to `kv_admin` `template.instructions`). Changed.
   - `Admin Lead` `7be0b673-1553-4c0a-960b-5db579da426d`: changed (6992 characters). Carries "Internal notes never go into an outgoing document" and the format gate Chặn sentence.
   - `Admin Drafter` `90c83c85-fe9c-40b3-bcf2-84d647e83b8e`: changed (7676 characters). Carries the squad rule "the NỘI BỘ bullet above does not apply to this squad".
   - `Admin Format Gate` `fd17e3a3-98e1-4828-9860-25371b310b61`: changed (7606 characters).
   - `Admin Files`, `Admin Format Lookup`, `Admin Numbers Gate`, `Admin Handover`: text unchanged by the commit, re-set to the same composed text (no change in content).
3. **Read-back:** all 7 admin agents equal the freshly composed text (7 of 7). The drafter contains the NỘI BỘ override sentence (1 of 1). Across the workspace, 3 agent instructions changed in value (lead, drafter, format gate), and 166 agents are still on the workspace.

The API still reports commit `1e2ba5a19`: commit `1cae0e31e` changes only `packages/core` files (no server code), so the running API serves the same logic. The text was produced by the worktree code and written through `PUT /api/agents/{id}` and `PUT /api/squads/{id}`.

## Open items

1. Resolved: see Follow-up. Both Operating reviews and both routines are paused, so nothing is scheduled to run until someone resumes them.
2. The env's `make status` reports `api mismatch`: `api.pid` holds `3828461`, but the listener is `3829708`, and the health response matches the checkout's commit. The API is answering correctly; the pid file is stale.
3. Web is stopped, so the UI was not opened. Nothing in the UI was checked.
4. Not verified: the content of the instructions and skill bodies; the squads' charters beyond their presence; the squads' behavior (agents were not run); the re-run path (a second quickstart run was not tried).
5. The environment was destroyed on 2026-11-12, so no live squad exists. The pack moved to upstream v1.5.0 after this seed; the seed was not repeated. Seed again in a new environment before any run, and check the squads' instructions against the v1.5.0 pack (the update kept every role's text, so the instructions should not change).

