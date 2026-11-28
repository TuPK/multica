# Rà soát độc lập chương trình Delegora bền vững (codex gpt-6.1-terra, xhigh)

Ngày rà soát: 2026-11-28. Nhánh `durable-platform`. Chỉ đọc: không sửa code, spec, plan; không chạy test có DB; không commit, không push.

Nguồn thô của codex: `/home/av-server/.cache/claude-delegate/20261128-140307-codex-review/review.md` (brief tại `brief.md`, log tại `codex.log`).

## 1. Tóm tắt

**Chương trình chưa hoàn tất và chưa sẵn sàng bật cờ `durable_*` cho người dùng.** Codex báo 5 phát hiện chặn, 12 quan trọng, 1 nhỏ. Sau khi tôi đối chiếu lại bằng code, phát hiện chặn thứ 5 (trigger ghi lịch sử khi cờ tắt) là **xung đột trong spec** cần quyết định, nên xếp lại là quan trọng. Kết quả sau đối chiếu: **4 chặn (C1–C4), 12 quan trọng (Q1–Q12), 1 nhỏ (N1)**. Lỗi tài liệu plan được ghi riêng ở mục 6, không tính vào số trên.

Các lỗi chặn đã xác nhận:
- Duyệt intent không kiểm `call_sha256` trước khi thực thi (spec 5.3, 14.6).
- Attestation hợp lệ của template khác vẫn được gắn nhãn "đã xác minh" (Hub).
- Câu hỏi dự đoán không tạo task; hai hàm tự áp quyết định không có nơi gọi.
- Đảo quyết định không đảo proposal/intent/outcome và đếm được hai lần cho cùng một quyết định.

Trạng thái plan không đáng tin: bảng "Trạng thái thực tế" trong `plan.md` ghi E4 và E5 là chưa xong, nhưng code và test có `TestHiredSeatStartsFull`, `TestCloseGapCreatesIssueWithMetricCriterion`, và Hub đã có kind agent (`hub_items.go:133`). Ngược lại, C6 và D7 thiếu thật (xem mục 4).

Đây là rà soát từng phần, không phải chứng nhận các phần không có phát hiện là đúng. Cây làm việc đã thay đổi trong lúc rà soát (`git status` ban đầu chỉ có `plan.md`, sau đó có thêm file test handler), nên kết luận cần chốt lại trên một snapshot ổn định.

## 2. Phạm vi đã đọc / đã bỏ qua

**Đã đọc đầy đủ:** `AGENTS.md`, `CLAUDE.md`, `spec.md` (1302 dòng), `plan.md` trên đĩa (613 dòng, khác `HEAD` ở dòng trạng thái E2), `p0-findings.md`.

**Đã đối chiếu trực tiếp (bởi codex và/hoặc tôi):**
- Cờ: `server/internal/featureflags/keys_fork.go`, `keys_fork_test.go` (năm cờ `durable_*` có test mặc định tắt), `packages/core/durable/flags.ts`.
- Migration 1041–1100 và các migration liên quan: `1044_issue_status_transition_trigger.up.sql`, các index `CONCURRENTLY`, danh sách cleanup trong `server/cmd/migrate/`. Codex kiểm 44 lệnh tạo index trong phần thay đổi so với `origin/main`: đều có `CONCURRENTLY`, mỗi file một statement, đều có tên trong danh sách cleanup. Không thấy `FOREIGN KEY`, `REFERENCES`, `CASCADE` trong SQL thay đổi.
- Service và handler: intent, effect, prediction, delegation, Reach, OCF, attestation, seat offer, Hub attestation.
- Test: tìm tên test trong plan bằng cách quét toàn bộ test Go và Vitest. Plan có `TestDeleteProfile`, code có biến thể `TestDeleteProfileRemovesItsRows`.
- Locale: chín thư mục `packages/views/locales/` đều có `durable.json`. Codex so khóa đệ quy với tiếng Anh; một số locale thiếu hậu tố số nhiều `_one`, nhưng chưa kết luận thiếu bản dịch vì quy tắc số nhiều khác nhau. Chưa đánh giá chất lượng bản dịch.
- Tôi kiểm lại: `packages/core/durable/*/schemas.ts` có `parseWithFallback` (từ 1 đến 8 lần mỗi file).
- `e2e/durable/`: chỉ có `effects.spec.ts` và `outcomes.spec.ts`.

**Đã bỏ qua hoặc chưa đủ:**
- `apps/mobile/AGENTS.md`: không đọc, vì spec loại mobile khỏi v1.
- Không kiểm hết mọi endpoint, mọi nhánh quyền, lock order, selector Zustand, package boundaries.
- Không kiểm apply/rollback migration, không đối chiếu `origin/main` mới nhất.
- Không đánh giá chất lượng từng bản dịch.

## 3. Phát hiện

Ký hiệu: **[đã xác nhận]** là tôi đã đọc lại đúng dòng code; **[codex]** là codex báo, tôi chưa kiểm lại từng dòng.

### Chặn

**C1. Intent được thực thi mà không kiểm `call_sha256` đã lưu. [đã xác nhận]**
- Vị trí: `server/internal/service/durable_intent_fork.go:244` (`origSHA := it.CallSha256`), `:262–277` (ghi đè hash bằng `sum` tính từ call hiện tại).
- Bằng chứng: hàm đọc `origSHA` chỉ để quyết định `wasEdited`. Không có bước so `CallSHA256(call)` (từ `it.Call` đọc được) với `it.CallSha256` đã lưu. Trong mã không test, `CallSha256` chỉ được ghi, không được đọc để kiểm (`rg CallSha256` chỉ thấy trong generated và trong hàm này).
- Spec: 5.3 "nền tảng tự gọi đúng `call` (so call_sha256) qua gateway"; 14.6 "Gateway thực thi ý định đã duyệt kiểm `call_sha256` và trạng thái trong cùng giao dịch khoá dòng ý định".
- Hệ quả: nếu `call` và hash bị lệch (do lỗi ghi, do sửa trực tiếp DB), gateway vẫn thực thi call không khớp bản đã ràng buộc.
- Kiểm lại: `sed -n '240,279p' server/internal/service/durable_intent_fork.go`; `rg -n 'CallSHA256|CallSha256' server/internal --glob '!**/*test*'`.
- Việc cần làm: thêm kiểm hash trước khi thực thi, trong cùng transaction khoá dòng, và test lệch hash bị từ chối.

**C2. Hub gắn nhãn "đã xác minh" cho attestation của template khác. [đã xác nhận]**
- Vị trí: `server/internal/handler/hub_attestation_fork.go:70–86` (`verifyHubBundleAttestation`). Chỉ kiểm chữ ký và `issuer.host`, không đối chiếu `subject.template_ref` với item.
- Bằng chứng: test `hub_attestation_fork_test.go:102` tạo attestation có `subject.template_ref = "hub:agent-reviewer@1.0.0"`, còn `:141` đặt `source_ref = "acme/code-reviewer@1.0.0"`. Test vẫn mong `Verified = true` (`:178`, `:188`). Codex báo cùng lỗi ở `durable_seat_offer_fork.go:365` (chưa kiểm lại dòng này).
- Hệ quả: một chứng nhận thật của template A có thể được hiển thị như bằng chứng cho template B.
- Kiểm lại: `sed -n '60,96p' server/internal/handler/hub_attestation_fork.go`; đọc `hub_attestation_fork_test.go:98–149`.
- Việc cần làm: so `subject.template_ref` với ref của item (cả Hub và seat offer); sửa test để mong `Verified = false` khi lệch; thêm test cho đường seat offer.

**C3. Câu hỏi không tạo task dự đoán; hai hàm tự áp quyết định không có nơi gọi. [đã xác nhận]**
- Vị trí: `server/internal/service/durable_prediction_fork.go:124` (`InsertPendingPrediction` chỉ chèn dòng pending, không enqueue task Secretary); `durable_delegation_fork.go:418` (`IntentService.ApproveAsPrincipal`), `:440` (`VerificationService.RecordAsPrincipal`) có định nghĩa; `rg` trong production không thấy nơi gọi.
- Spec: 6.4–6.5 yêu cầu tạo task dự đoán và tự áp quyết định đủ điều kiện.
- Chỉ `delegateInquiry` (`durable_prediction_fork.go:171`) được gọi, và chỉ trong nhánh inquiry.
- Kiểm lại: `rg -n 'ApproveAsPrincipal\(|RecordAsPrincipal\(|delegateInquiry|InsertPendingPrediction' server/internal --glob '!**/*test*'`.
- Việc cần làm: nối task dự đoán và đường gọi tự áp quyết định, kèm test end-to-end ở tầng service.

**C4. Đảo quyết định không đảo gì ngoài inquiry, và đếm được hai lần cho cùng một quyết định. [đã xác nhận]**
- Vị trí: `server/internal/service/durable_delegation_fork.go:297–330` (`OnReverted`); `server/pkg/db/queries/durable_delegation.sql:46–50` (`CountDelegationReverts`).
- Bằng chứng: `OnReverted` kiểm chủ sở hữu và cửa sổ 72 giờ, rồi ghi governance event mỗi lần gọi, không kiểm đã đảo chưa. Đếm đọc governance event theo `subject_id`, nên gọi hai lần cùng `predID` cho số đếm 2. Chỉ nhánh `inquiry` gọi `ReopenBriefInquiryAnswer`; proposal, intent và outcome không được đảo. `ReviewAccuracy` chỉ có định nghĩa, chưa thấy nơi gọi.
- Spec: yêu cầu đảo thực sự và đếm đúng (kiểm lại mục delegation trong spec khi sửa).
- Kiểm lại: `sed -n '294,356p' server/internal/service/durable_delegation_fork.go`; `rg -n 'ReviewAccuracy\(' server/internal --glob '!**/*test*'`.
- Việc cần làm: dedupe theo `predID` (đã đảo thì trả lỗi hoặc no-op), đảo proposal/intent/outcome theo đúng spec, và test gọi hai lần.

### Quan trọng

**Q1. Trigger ghi `issue_status_transition` khi cờ `durable_outcomes` tắt: xung đột trong spec. [đã xác nhận]** (codex xếp chặn; tôi xếp lại quan trọng.)
- Vị trí: `server/migrations/1044_issue_status_transition_trigger.up.sql:42–57`; test `durable_transition_fork_test.go:939–975`.
- Bằng chứng: trigger không kiểm cờ. D26 (spec dòng 61) cố ý dùng trigger "độc lập với cờ"; test ghi chú điều này. Nhưng spec 2.10 mục 7 (dòng 314) yêu cầu "Tắt `durable_outcomes`: không ghi gì vào bốn bảng mới". Tên `TestTransitionNotRecordedWhenFlagOff` chỉ kiểm consumer, không kiểm việc ghi.
- Tôi chưa xác định `issue_status_transition` có nằm trong "bốn bảng mới" hay không; đây là câu cần người viết spec trả lời.
- Việc cần làm: quyết định (a) sửa spec 2.10 mục 7 để loại bảng transition khỏi điều khoản, hoặc (b) đổi trigger để kiểm cờ. Đổi tên test cho đúng nội dung.

**Q2. Duyệt intent có thể kẹt ở `approved` khi tiến trình dừng giữa chừng. [codex]**
- Vị trí: `durable_intent_fork.go:148` (`claimForApproval` commit `approved` trước `ExecuteIntent`); lỗi ghi outcome/event sau thực thi chỉ được log.
- Hệ quả: nếu dừng sau commit và trước thực thi, lần duyệt lại bị từ chối vì không còn `pending`. Chưa thấy cơ chế phục hồi.
- Kiểm lại: `sed -n '140,187p;217,279p' server/internal/service/durable_intent_fork.go`.

**Q3. Tiêu chí Reach không khớp nguồn đo. [codex]**
- Vị trí: `durable_reach_fork.go:367` (tạo issue với `metric_key = reach.gap_asks_30d`); `durable_verification_fork.go:970`.
- Bằng chứng: chuỗi `reach.gap_asks_30d` chỉ xuất hiện ở chỗ tạo tiêu chí. Bộ kiểm đọc `metric_reading`, còn snapshot ghi `reach_snapshot`. Query chỉ chọn outcome chưa chốt; cửa sổ mặc định outcome là 7 ngày, tiêu chí Reach là 30 ngày.
- Kiểm lại: `rg -n 'reach\.gap_asks_30d' server --glob '!**/*test*'`; đọc `ListOpenSystemCriteria` trong `server/pkg/db/queries/durable_verification.sql`.

**Q4. Reach phân loại sai khai báo của agent và bỏ qua nhiều loại câu hỏi. [codex]**
- Vị trí: `durable_reach_fork.go:96`; `server/pkg/db/queries/durable_reach.sql:11`.
- Bằng chứng: snapshot cộng cả event `declared` vào `EffectsAgent30d`, trong khi `declared_only` đòi số đó bằng 0. Query attention chỉ nối `effect_intent`.
- Kiểm lại: `sed -n '94,107p;206,230p' server/internal/service/durable_reach_fork.go`.

**Q5. Tạo issue lấp khoảng trống không nguyên tử. [codex]**
- Vị trí: `durable_reach_fork.go:329–385`.
- Bằng chứng: đọc issue cũ, tạo issue, thêm criterion, rồi lưu `gap_issue_id` qua các lời gọi riêng. Hai request đồng thời có thể cùng tạo issue; lỗi sau khi tạo để lại issue mồ côi.
- Việc cần làm: gom vào một transaction, hoặc khoá theo gap.

**Q6. Flow external kẹt vô hạn sau từ chối hoặc hết hạn. [codex; plan cũng ghi nhận, chưa đọc lại code]**
- Vị trí: `durable_flow_effects_fork.go:21–43`.
- Bằng chứng: nếu đã có intent và trạng thái không phải `approved` thì hàm trả false, không tạo intent mới và không dừng flow. Plan ghi nhận "intent bị từ chối giữ bước chờ, chưa có đường dừng".
- Kiểm lại: `sed -n '21,43p' server/internal/service/durable_flow_effects_fork.go`.

**Q7. Test shadow đếm hàng toàn DB dùng chung. [codex]**
- Vị trí: `durable_shadow_fork_test.go:209–255` (đếm `agent_task_queue` theo `durable_kind = shadow` không lọc workspace, mong đúng một hàng).
- Hệ quả: package khác chạy song song có thể làm test đỏ. AGENTS.md cấm kiểu assertion này.
- Việc cần làm: lọc theo fixture của test.

**Q8. Gate thêm hai SELECT trên đường nóng. [codex]**
- Vị trí: `durable_effect_log_fork.go:81`; `durable_effect_log_fork_test.go:270` (`gateReadBudget = 2`).
- Spec 14.7 và plan B2 yêu cầu không thêm query đọc ngoài insert event. Test đã nới ngân sách để khớp code.
- Kiểm lại: `rg -n 'gateReadBudget|runClass|companyState' server/internal/service/durable_effect_log_fork*.go`.

**Q9. Event log cho phép sửa ngoài hợp đồng append-only. [codex]**
- Vị trí: `server/pkg/db/queries/durable_effect.sql:25–84`.
- Bằng chứng: query cập nhật `undo`, `undo_until`, `result`, chuyển `executed → undoing → executed|undone`, và cập nhật `intent_id`. Spec 4.4 chỉ cho `awaiting → executed|blocked|failed` và `executed → undone`. Test append-only (`server/internal/migrations/durable_effect_append_only_test.go`) chấp nhận phạm vi rộng hơn.
- Việc cần làm: thu hẹp query theo spec hoặc sửa spec, và cập nhật test.

**Q10. Job học bỏ quyết định chưa xử lý quá sáu giờ. [codex]**
- Vị trí: `durable_prediction_fork.go:370`; `server/pkg/db/queries/durable_prediction.sql:84–91` (`resolved_at >= now - 6h AND learning_task_id IS NULL`).
- Hệ quả: sau downtime hoặc job lỗi quá sáu giờ, các quyết định chưa học không được chọn lại.
- Việc cần làm: bỏ cửa sổ thời gian, hoặc dùng cờ "chưa học" không kèm giới hạn thời gian.

**Q11. Import OCF bỏ qua một phần dữ liệu đã xuất. [codex]**
- Vị trí: `durable_ocf_fork.go:832–876`, `918–945`; `250–258`.
- Bằng chứng: export đưa `principals` vào package nhưng import đánh dấu `principals` và `attestations` là `ImportSkipped, "not imported by this build"`. Dòng dossier thành governance event, không thành bản ghi dossier hoạt động.
- Hệ quả: khôi phục không đầy đủ.

**Q12. Hub agent và badge attestation không kiểm `durable_open`. [codex, chưa kiểm lại]**
- Vị trí: `server/internal/handler/hub_items.go:133` (có `KindAgent`), `packages/views/hub/hub-item-page.tsx:157`.
- Bằng chứng: codex không thấy kiểm cờ trong các đoạn này. Tôi chỉ xác nhận `hub_items.go:133`.
- Kiểm lại: `rg -n 'DurableOpenEnabled|KindAgent|hubItemAttestation' server/internal/handler/hub_items.go server/internal/handler/hub_attestation_fork.go`.

### Nhỏ

**N1. JSON lỗi OCF đọc bằng ép kiểu. [codex, đã thấy dòng]**
- Vị trí: `packages/core/durable/ocf/mutations.ts:29`: `const data = (await res.json()) as { error?: string }`. Vi phạm quy tắc "UI-consumed JSON passes through zod" trong AGENTS.md. Hàm có `try/catch`, không có bằng chứng lỗi dữ liệu.
- Việc cần làm: đi qua schema zod và `parseWithFallback`.

## 4. Đánh giá từng task trong plan

Quy ước: **Hoàn thành** = có code và test theo tên plan, không có phát hiện chặn/quan trọng mở. **Một phần** = có code nhưng còn phát hiện hoặc thiếu nghiệm thu. **Thiếu** = chưa có. Không ai trong hai phiên này chạy test, nên không có task nào "đã xanh" theo kết quả chạy.

| Task | Trạng thái | Lý do / phát hiện liên quan |
|---|---|---|
| P0 | Hoàn thành | `p0-findings.md` có U1–U10. |
| A1 | Một phần | Trigger có, nhưng ghi khi cờ tắt (Q1). Chưa kiểm backfill. |
| A2 | Một phần | Outcome và settlement có; chưa kiểm vòng đời đầy đủ. |
| A3 | Một phần | Metric Reach chưa đo đúng nguồn (Q3). |
| A4 | Một phần | Briefing có; chưa đối chiếu hết đường gọi. |
| A5 | Một phần | Scorecard có; chưa kiểm hành vi cờ tắt đầy đủ. |
| A6 | Một phần | Track record có; chưa kiểm đồng thời và job. |
| A7 | Một phần | Envelope có; chưa kiểm đổi tay, high-fail, quyền. |
| A8 | Một phần | Core, view, schema có; chưa kiểm locale, desktop, GIF. |
| A9 | Một phần | `outcomes.spec.ts` có; chưa chạy. |
| B1 | Một phần | Bảng chính sách có; chưa kiểm mọi kết hợp. |
| B2 | Một phần | Gate và log có; hai SELECT (Q8), append-only (Q9). |
| B3 | Một phần | Declare/undo có; chưa kiểm quyền, expiry, provider thật. |
| B4 | Một phần | Lock và test đồng thời có; thiếu kiểm hash (C1), phục hồi (Q2). |
| B5 | Một phần | Trường flow và hold có; kẹt khi từ chối/hết hạn (Q6). |
| B6 | Một phần | Views và test có; chưa kiểm unmount, desktop, GIF. |
| B7 | Một phần | `effects.spec.ts` có; chưa chạy. |
| C1 | Một phần | Ledger, metric, header có; chưa kiểm mọi loại ask. |
| C2 | Một phần | Profile, consent, interview có; chưa kiểm xoá dữ liệu qua mọi đường. |
| C3 | Một phần | Prediction và job học có; thiếu task dự đoán (C3), kẹt sau downtime (Q10). |
| C4 | Một phần | Delegation có; tự áp chưa nối (C3), đảo sai (C4). |
| C5 | Một phần | Views principal và attention có; chưa kiểm locale, luồng thật. |
| C6 | **Thiếu** | Không có `e2e/durable/` cho prediction và attention ledger 0,5 phút. |
| D1 | Một phần | Jury có; chưa kiểm timeout, vote, chi phí runtime. |
| D2 | Một phần | Dossier có; chưa kiểm retrospective, quyền, dọn định kỳ. |
| D3 | Một phần | Shadow có; test đếm toàn DB (Q7). |
| D4 | Một phần | Runtime change có; chưa kiểm promotion và rollback thật. |
| D5 | Một phần | Necessity, Newcombe, job có; chưa kiểm luồng proposal đầy đủ. |
| D6 | Một phần | Views seats có; chưa kiểm cảnh báo, GIF, desktop. |
| D7 | **Thiếu** | Không có `e2e/durable/shadow.spec.ts`. |
| E1 | Một phần | Reach, route, test criterion có; metric, phân loại, tạo issue (Q3–Q5). |
| E2 | Một phần | OCF, schema, adapter có; import bỏ sót (Q11). |
| E3 | Một phần | Ký và xác minh có; gắn nhầm subject (C2). |
| E4 | Một phần | Route, budget, `TestHiredSeatStartsFull` có; xác minh subject sai (C2). |
| E5 | Một phần | Reach UI, export dialog, badge Hub có; badge chưa gate (Q12). Chưa kiểm trải nghiệm thật. |
| F1 | **Thiếu** | Chưa có bằng chứng chạy `make check`, e2e durable, `sync:contract`, security review; chưa có `rollout.md`. |
| F2 | **Thiếu** | Bật thử trên dev cần người dùng đồng ý. |

## 5. Chưa kiểm được bằng đọc code

- **GIF, ảnh chụp, trải nghiệm trình duyệt/desktop:** chưa kiểm. Cần chạy app và kiểm overflow, reduced motion, frame sampling theo `scripts/motion/capture.mjs`.
- **Bật cờ trên dev, E2E trên workspace thật:** chưa kiểm. Cần người dùng đồng ý (plan F2).
- **Lệnh cần DB** (go test DB-backed, migration apply/rollback, e2e): chưa kiểm. Cần DB test riêng; không dùng DB `multica`.
- **Real-agent smoke:** không được uỷ quyền, không chạy.
- **Lock order và đường quyền đầy đủ:** chưa kiểm hết. Cần rà mọi đường enqueue và test ép interleaving trên DB test.
- **Test không DB:** không chạy (chỉ đọc). `go vet`/`tsc` chưa chạy.
- **Migration:** chưa kiểm apply/rollback; khoảng trống số `1067–1069`, `1079`, `1082–1089`, `1135–1139` chưa được giải thích.

## 6. Kết luận và việc còn lại theo ưu tiên

**Chặn (làm trước):**
1. Kiểm `call_sha256` trước khi thực thi intent, trong cùng transaction (C1), kèm test lệch hash.
2. Ràng buộc attestation với đúng template ở cả Hub và seat offer (C2), sửa test `hub_attestation_fork_test.go` đang mong `Verified = true` cho template lệch.
3. Nối task dự đoán và đường tự áp quyết định (C3), có test end-to-end.
4. Làm đảo quyết định có hiệu lực, idempotent, và đếm đúng (C4).

**Quyết định spec (cần người viết spec):**
5. Trigger transition có ghi khi `durable_outcomes` tắt không (Q1). Sửa spec 2.10 mục 7 hoặc đổi trigger.

**Quan trọng:**
6. Phục hồi intent sau lỗi (Q2).
7. Metric Reach, phân loại, tạo issue nguyên tử, cửa sổ 30 ngày (Q3–Q5).
8. Flow bị từ chối/hết hạn phải dừng (Q6).
9. Test đếm toàn DB (Q7) và ngân sách gate (Q8).
10. Thu hẹp query append-only (Q9) và bỏ cửa sổ 6 giờ của job học (Q10).
11. Hoàn thiện import OCF (Q11). Gate badge Hub theo `durable_open` (Q12).
12. Viết E2E C6 và D7 (mục 4).
13. Cập nhật bảng trạng thái trong `plan.md`: E4 và E5 đã có code/test mà bảng ghi là thiếu (`TestHiredSeatStartsFull`, `TestCloseGapCreatesIssueWithMetricCriterion`, Hub kind agent tại `hub_items.go:133`). Làm việc này trước khi tick task.

**Nhỏ:**
14. Đưa JSON lỗi OCF qua zod (N1).

**Trước khi nói "hoàn tất":** chốt snapshot ổn định (cây đã đổi trong lúc rà soát), rồi chạy F1 trên DB test được phép: `make check`, e2e durable, `pnpm sync:contract`, rà security review, viết `rollout.md`. F2 chỉ sau khi người dùng đồng ý.

Ghi chú: file này chưa được commit. `plan.md` có thay đổi chưa commit từ trước khi rà soát; không đụng đến.
