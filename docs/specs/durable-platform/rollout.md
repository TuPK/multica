# Rollout chương trình Delegora bền vững (F1)

Ngày: 2026-11-28. Nhánh `durable-platform`, HEAD gốc `33a0ce26ea`. File này ghi bằng chứng thật, không ghi kết quả dự kiến. Trạng thái **chưa sẵn sàng bật cờ** cho người dùng: review xhigh cuối và Playwright C6/D7 chưa chạy.

## 0. Cập nhật lần commit này

- Các mục review 1–16 đã được sửa trong code (kiểm từng mục bằng đọc code và test), gồm các lỗi bảo mật chặn bật cờ: route chỉ dành cho người (`RequireHumanActor`) cho effects, intents, principal, envelope; intent không tự chạy lại sau khi đã duyệt; undo và đảo quyết định có trạng thái rõ ràng (409).
- Test DB tuần tự trên `multica_test`: `cmd/server`, `internal/migrations`, `cmd/migrate` xanh; `internal/service` và `internal/handler` còn một số test flaky do timeout DB dưới tải máy (chạy riêng đều PASS): `TestSettlementTriggerDoesNotBlockSharedComments`, `TestRuntimeOfflineRetryWaitsForHealthyRuntime`, `TestClaimTaskConcurrentCapacityRespected`, `TestClaimTasksByRuntime_ResponsePrecedesRecoveryNotifications`, `TestRuntimeMemberSharingAddUnderEachAction`, `TestEnqueueTaskForIssueAutopilotPreservesTriggerOwnerAfterRunCloses` (lỗi email trùng do dữ liệu test còn sót).
- Chưa chạy: Playwright C6/D7, `make check`, `pnpm sync:*`, và review độc lập bằng codex (credit đang hết). Chưa bật cờ trên bất kỳ môi trường nào.
- Các sai lệch spec đã nêu ở mục 4 vẫn cần người viết spec quyết.

## 1. Trạng thái tổng

| Hạng mục | Trạng thái | Bằng chứng |
|---|---|---|
| Build và vet server | Xanh | `go build ./...`, `go vet ./internal/service ./internal/handler ./internal/migrations ./cmd/migrate` |
| Test service (DB `multica_test`) | Xanh | `go test ./internal/service -count=1` → ok (156s) |
| Test migrations và cmd/migrate | Xanh | `go test ./internal/migrations ./cmd/migrate -count=1` → ok |
| Test handler (DB `multica_test`) | Xanh, 1 test flaky | `go test ./internal/handler -count=1` → chỉ `TestInboxReadThreeInstancesBoundSlowSQL` đỏ do timing; chạy riêng PASS |
| Vitest core durable | Xanh | `pnpm --filter @multica/core exec vitest run durable` → 33 file, 207 test |
| Vitest views (badge, Hub) | Xanh | `pnpm --filter @multica/views exec vitest run durable/open/attestation-badge.test.tsx hub/hub-item-page.test.tsx` → 18 test |
| Playwright `e2e/durable` (C6, D7) | **Chưa chạy** | Cần backend và frontend cô lập trên `multica_test`. Backend ở `:8080` đang trả 200 là stack dev dùng DB `multica`, không được dùng để chạy e2e |
| `make check` | **Chưa chạy** | Chưa chạy cả frontend lẫn backend đầy đủ trong phiên này |
| `pnpm sync:contract`, `pnpm sync:coverage`, `pnpm test:scripts` | **Chưa chạy** | |
| Review bảo mật xhigh (codex) | **Chưa chạy** | Codex báo "Your workspace is out of credits" ở mọi lần thử |
| Bật cờ trên dev (F2) | **Không làm** | Cần người dùng đồng ý; không bật cờ trên bất kỳ môi trường nào |

## 2. Lỗi trong review và trạng thái sửa

Mã theo `review-codex-xhigh.md`.

| Mã | Nội dung | Trạng thái | Ghi chú |
|---|---|---|---|
| C1 | Kiểm `call_sha256` trước khi thực thi | Đã sửa | Lệch hash: không gọi gateway, `failed`, ghi chú "call hash mismatch" |
| C2 | Attestation khớp `template_ref` (Hub và seat offer) | Đã sửa | Ref rỗng là chưa xác minh |
| C3 | Task dự đoán và tự áp | Đã sửa | Task dự đoán qua đường enqueue chuẩn; tự áp chỉ khi đủ điều kiện spec 6.5 |
| C4 | Đảo quyết định có hiệu lực, idempotent | Đã sửa | Idempotent theo `predID`; đếm theo `DISTINCT subject_id` |
| C6 | Kịch bản F5/F6 trong e2e | Đã viết, **chưa chạy** | `e2e/durable/effects.spec.ts` |
| D7 | `e2e/durable/shadow.spec.ts` | Đã viết, **chưa chạy** | |
| B2 | Hai nguồn hệ quả thiếu hook | Một phần | Đã hook Action API (ghi và xoá storage). Không có nguồn webhook gửi ra khớp spec; xem mục 4 |
| B3 | Test khối khai báo trong briefing | Đã sửa | `TestEffectDeclarationBriefingFollowsFlag` |
| Q1 | Trigger transition ghi khi cờ tắt | **Giữ, cần người viết spec quyết** | Xem mục 4 |
| Q2 | Intent kẹt ở `approved` | Đã sửa theo quyết định D1 | Quét sau 10 phút → `failed`, không tự thực thi lại |
| Q3 | Metric Reach và cửa sổ 30 ngày | Đã sửa | Đọc `reach_snapshot`, không chốt sớm |
| Q4 | Phân loại khai báo Reach | Đã sửa | `declared` tách khỏi `EffectsAgent30d` |
| Q5 | Tạo issue lấp khoảng trống nguyên tử | Đã sửa | Một transaction, khoá hàng gap |
| Q6 | Flow kẹt khi intent bị từ chối hoặc hết hạn | Đã sửa | Dừng bước với lý do `rejected`/`expired` |
| Q7 | Test shadow đếm toàn DB | Đã sửa | Lọc theo fixture |
| Q8 | Gate đọc thêm hai SELECT | Đã sửa | Gộp vào một INSERT có điều kiện; ngân sách đọc 0 |
| Q9 | Append-only mở rộng hơn spec | Một phần | Giữ `undoing` để undo chỉ gọi provider một lần; sai lệch so với spec 4.4 |
| Q10 | Job học bỏ quyết định chưa xử lý quá 6 giờ | Đã sửa | |
| Q11 | Import OCF bỏ sót dữ liệu | Đã sửa | Xác minh bằng 10 test OCF |
| Q12 | Gate Hub theo `durable_open` | Đã sửa | Backend và frontend |
| N1 | JSON lỗi OCF không qua zod | Đã sửa | `packages/core/durable/ocf/mutations.ts` |

## 3. Quá trình sửa và lưu ý về công cụ

- Codex (`gpt-6.1-terra`, xhigh) chạy hai vòng và hết credit giữa chừng. Sandbox của codex không kết nối được PostgreSQL, nên mọi test DB được chạy lại bởi người điều phối.
- Agy (`gemini-4-neon-high`, effort high) làm nốt phần còn lại theo brief có quyết định thiết kế. Sau đó agy sửa ba test hồi quy được phát hiện khi chạy lại test DB.
- Tôi kiểm chứng lại từng lượt bằng test DB tuần tự, không chỉ dựa vào báo cáo của agy hay codex.

## 4. Giả định và xung đột spec (cần người viết spec quyết)

1. **Q1, trigger transition (spec 2.10 mục 7 và 2.2, D26):** trigger `1044` luôn ghi `issue_status_transition` dù cờ `durable_outcomes` tắt. Consumer chỉ xử lý khi cờ bật. Không đổi migration đã merge, không đổi spec. Cần quyết: có coi bảng này là bảng nền độc lập cờ hay không.
2. **Q9, effect log (spec 4.4):** claim trung gian `executed → undoing → executed|undone` để provider undo chỉ được gọi tối đa một lần. Spec chưa liệt kê bước này. Cần bổ sung vào spec 4.4.
3. **D1, intent bị gián đoạn (spec 5.3 và 5.6):** intent `approved` quá 10 phút chưa có kết quả chuyển sang `failed` với ghi chú "không rõ hành động đã chạy hay chưa". Hệ thống không tự thực thi lại vì gateway không có idempotency key. Cần bổ sung hành vi này vào spec 5.3.
4. **B2, nguồn `webhook`:** không có nguồn webhook gửi ra khớp spec (`webhook_delivery_worker.go` chỉ xử lý webhook đi vào). Không tạo nguồn mới. Cần người viết spec xác nhận bỏ hoặc hoãn khỏi phạm vi v1.
5. **Cấu trúc locale (spec 14.2):** khoá durable nằm trong `packages/views/locales/<locale>.ts`, không phải `durable.ts` riêng. Chưa quyết.

## 5. Việc còn lại trước khi bật cờ

1. Review xhigh (codex) trên toàn bộ thay đổi, chỉ đọc, lặp đến khi sạch. Đang bị chặn bởi hết credit.
2. Playwright `e2e/durable` (C6, D7) trên backend và frontend cô lập, dùng DB `multica_test`. Không dùng stack `:8080` của dev.
3. `make check`, `pnpm sync:contract`, `pnpm sync:coverage`, `pnpm test:scripts`.
4. `/security-review` và sửa trong commit riêng.
5. Giải quyết các mục ở mục 4.
6. F2: chỉ khi người dùng đồng ý. Không làm trong lượt này.

## 6. Test flaky ghi nhận

- `TestClaimTaskConcurrentCapacityRespected` (service): lỗi timeout khi xoá DB test isolated. Chạy riêng PASS.
- `TestInboxReadThreeInstancesBoundSlowSQL` (handler): "slow reads did not start". Chạy riêng PASS.
- `TestClaimTasksByRuntime_ResponsePrecedesRecoveryNotifications` (handler): flaky, chạy riêng PASS.

Các test này không thuộc phạm vi durable. Ghi lại để không nhầm với hồi quy.
