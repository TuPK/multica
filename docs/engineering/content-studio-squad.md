# Content Studio squad: sản xuất nội dung, kiểm duyệt của người, đăng từ một chỗ

Spec (thiết kế, chưa triển khai). Cùng họ với [kitviet-squads.md](kitviet-squads.md), [sales-loop-squad.md](sales-loop-squad.md) và [digital-company.md](digital-company.md).

Ngôn ngữ: tiếng Việt để người dùng đọc; tên role key, kind, status, skill và lệnh giữ tiếng Anh.

Nguồn ý tưởng: [XuHo-IT/Content-Agent-Kit](https://github.com/XuHo-IT/Content-Agent-Kit), giấy phép MIT. Các skill được chọn ở mục 6 chưa được đọc từng dòng; chỉ đọc mô tả và tài liệu kiến trúc.

**Quyết định đã chốt**
- Đường đăng: **Ayrshare**.
- Duyệt: qua **hàng đợi quyết định (proposal)** của multica.
- Zalo OA: **để sau**, chưa nằm trong lộ trình vì chưa có OA.
- Thị trường: Việt Nam và quốc tế. Kênh quốc tế đề xuất ở mục 3.1.

## 1. Tóm tắt

Một squad sản xuất nội dung cho hai thị trường, Việt Nam và quốc tế, với ba đặc điểm:

1. **Người duyệt trước khi đăng.** Agent soạn, soát và đóng gói. Không có gì được đăng nếu chưa có quyết định duyệt của người.
2. **Đăng luôn từ multica.** Sau khi duyệt, nội dung đi thẳng tới kênh đã cấu hình qua Ayrshare. Người dùng không phải đăng thủ công trên từng site hay phần mềm.
3. **Kênh đăng cấu hình được trong multica.** Có tab cài đặt "Kênh đăng" để kết nối tài khoản, gắn kênh với thị trường và kiểm tra trước khi dùng. Người dùng cấu hình kênh trước, squad chạy sau.

## 2. Mục tiêu và không mục tiêu

**Mục tiêu**
- Từ một brief hoặc release note tới bài đã đăng, người duyệt chỉ cần bấm duyệt ở một nơi.
- Mỗi kênh đăng có trạng thái rõ: chưa kết nối, sẵn sàng, lỗi, đã hết hạn.
- Nội dung cho mỗi thị trường có ngôn ngữ, giọng và cổng kiểm tra riêng.
- Mỗi lần đăng để lại dấu vết: ai duyệt, lúc nào, đăng lên kênh nào, ra URL nào.

**Không mục tiêu (bản này)**
- Tự đăng mà không có người duyệt, kể cả khi agent tự tin.
- Tự quản lý OAuth cho từng nền tảng. Ayrshare làm việc này.
- Quảng cáo trả phí. Đó là việc của squad Marketing ([kitviet-squads.md](kitviet-squads.md)).
- Zalo OA trong giai đoạn đầu.
- Đăng tự động lên Reddit và Hacker News. Hai kênh này đăng tay có duyệt.

## 3. Thị trường và kênh

Mỗi kênh gắn với đúng một thị trường. Một nội dung có thể đăng lên nhiều kênh, nhưng mỗi phiên bản được viết cho một thị trường.

| Thị trường | Kênh | Ghi chú |
|---|---|---|
| Việt Nam | Facebook Pages, YouTube; TikTok sau khi qua audit | Zalo OA để sau. TikTok: xem mục 3.1 |
| Quốc tế | X, LinkedIn; sau đó YouTube, TikTok, Threads, Bluesky | Chi tiết ở mục 3.1 |

Mỗi thị trường có một bộ **quy tắc tuân thủ** riêng (quảng cáo, nhãn nội dung do AI tạo, giới hạn độ dài và thời lượng video). Các quy tắc này nằm trong kit ở `skills/freshness-check`, phải có ngày kiểm và nguồn chính thức; luật thay đổi thì chờ người duyệt trước khi áp dụng.

### 3.1 Kênh quốc tế: nghiên cứu và đề xuất

Nguồn về chiến lược kênh chủ yếu là blog của các hãng bán công cụ và agency, nên các con số chỉ là giả thuyết cần đo lại bằng dữ liệu của chính multica.

| Kênh | Vai trò cho multica | Ayrshare | Ràng buộc đã biết | Ưu tiên |
|---|---|---|---|---|
| **LinkedIn** | Kênh pipeline B2B; bài từ người sáng lập và đội dev | Hỗ trợ | Đăng với tư cách trang công ty cần quyền `w_organization_social` thuộc Community Management API, phải qua duyệt ứng dụng và có thể bị từ chối. Đăng với tư cách cá nhân (`w_member_social`) dễ hơn. **Chưa xác minh Ayrshare dùng quyền nào** | 1 |
| **X** | Tiếp cận cộng đồng dev; cập nhật sản phẩm | Hỗ trợ | 280 ký tự (25.000 với Premium); giới hạn lượt gọi API theo gói, một changelog năm 2025 ghi 100.000 (Business) và 5.000 (Premium), có thể đã lỗi thời; tiếp cận tự nhiên hạn chế | 1 |
| **YouTube** | Demo, hướng dẫn, Shorts | Hỗ trợ, chỉ video, cần tiêu đề | Cần video, nên để giai đoạn 4 | 2 |
| **TikTok** | Video ngắn, cả VN và quốc tế | Hỗ trợ đăng video | App chưa qua audit chỉ đăng được ở chế độ riêng tư; người dùng đăng phải để riêng tư tại thời điểm đăng. Audit ước 2–6 tuần (nguồn bên thứ ba). Nên nộp audit sớm | 2 |
| **Threads**, **Bluesky** | Mở rộng khi đã có nội dung | Hỗ trợ | Threads: bài chữ và carousel tối đa 20 ảnh/video | 3 |
| **Reddit** | Đánh giá sản phẩm trong cộng đồng | Ayrshare liệt kê không nhất quán giữa các trang; **chưa xác minh** | Quy tắc tự quảng bá do từng subreddit đặt và khác nhau nhiều; không có quy tắc chung đáng tin | 3, đăng tay |
| **Hacker News**, Discord, Slack cộng đồng | Ra mắt sản phẩm, thảo luận | Không có đường đăng tự động đáng tin | Đăng tay | Thủ công |

**Đề xuất:**
- Quốc tế, giai đoạn 1: **X và LinkedIn cá nhân**. Lý do: nguồn đều đồng ý LinkedIn là kênh pipeline B2B mạnh nhất, còn X phù hợp để tiếp cận cộng đồng dev. LinkedIn trang công ty để sau khi xác minh quyền với Ayrshare.
- Việt Nam, giai đoạn 1: **Facebook Pages**. TikTok nộp audit ngay từ bây giờ để không bị chặn lúc cần đăng.
- YouTube cho video, giai đoạn 4.

## 4. Squad và vai trò

| Vai | role key | kind (theo Kit Việt) | Việc | Skill chính |
|---|---|---|---|---|
| Content Lead | `cs_lead` | `kv_lead` | Nhận brief hoặc release note, lập lịch theo thị trường, giao sub-issue | `daily-run` (phần lập lịch) |
| Scout | `cs_scout` | `kv_collector` | Tìm chủ đề, theo dõi đối thủ | `topic-radar`, `competitor-analysis` |
| Researcher | `cs_researcher` | `kv_collector` | Thu nguồn, bằng chứng, ảnh chụp | `research-and-capture` |
| Drafter | `cs_drafter` | `kv_drafter` | Viết bài và kịch bản cho một thị trường | `story-video`, `repurpose`, `geo-optimize` |
| Localizer | `cs_localizer` | `kv_drafter` | Chuyển bản đã duyệt sang thị trường còn lại, không dịch máy từng câu | `localize-video` |
| Producer (giai đoạn 4) | `cs_producer` | `kv_drafter` | Dựng video 9:16 | `create-video`, `motion-craft` |
| Claim Gate | `cs_claim_gate` | `kv_gate` | Soát mọi con số, tính năng và so sánh với nguồn; không có số liệu thì ghi `[cần bổ sung]` | `review-gate` |
| Market Gate | `cs_market_gate` | `kv_gate` | Soát luật và chính sách của thị trường đích; nhãn AI; giới hạn kênh | `freshness-check` |
| Voice Gate | `cs_voice_gate` | `kv_gate` | Soát giọng thương hiệu, lỗi "mùi AI", metadata lọt vào thân bài | `validate-post.mjs` |
| Handover | `cs_handover` | `kv_handover` | Đóng gói bản đã qua cổng, đề xuất kênh và lịch đăng, gửi vào hàng đợi duyệt | — |
| Publisher | `cs_publisher` | `kv_handover` | Chỉ chạy **sau** khi đề xuất được duyệt; gọi adapter Ayrshare và ghi kết quả | thay `make-post` bằng adapter |

Ghi chú:
- Claim Gate, Market Gate và Voice Gate là ba agent độc lập với Drafter. Đây là điều kiện của cam kết "người soạn, người soát, người bàn giao là ba agent khác nhau" trong Kit Việt.
- Publisher không tự quyết. Nó chỉ nhận lệnh từ đề xuất đã được duyệt.
- Giai đoạn 1 không có Producer. Video đến giai đoạn 4.

## 5. Luồng và cổng người

```
brief / release note
  → Lead lập lịch theo thị trường
  → Scout + Researcher (bằng chứng, nguồn)
  → Drafter → Localizer (nếu có bản thị trường thứ hai)
  → Producer (giai đoạn 4, nếu là video)
  → Claim Gate · Market Gate · Voice Gate   (tối đa 2 vòng sửa)
  → Handover: gói nội dung + kênh đích + lịch đăng → đề xuất "publish"
  → NGƯỜI DUYỆT trong hàng đợi quyết định  ◄── cổng bắt buộc
  → Publisher → Ayrshare → kênh đăng
  → ghi kết quả: trạng thái, URL, thời điểm, người duyệt
```

### 5.1 Cổng duyệt: đề xuất "publish" trong hàng đợi quyết định

Dùng hàng đợi quyết định có sẵn ([packages/views/company/lib/decision-queue.ts](../../packages/views/company/lib/decision-queue.ts)) thay vì comment trong issue. Lý do: hàng đợi đã có đủ ba trạng thái mà việc duyệt cần:

- **Cần quyết định** (`pending`): bài đã qua cổng, đang chờ bạn.
- **Để sau** (snooze): bạn chưa muốn quyết định, bài trở lại hàng đợi đúng ngày đã chọn.
- **Đang chờ squad sửa** (`revising`): bạn yêu cầu sửa kèm ghi chú, squad làm lại rồi gửi lại.

Ngoài ra còn `approved`, `rejected` và `expired`.

**Việc phải làm để có loại đề xuất này:** thêm `publish` vào `ProposalKind` và `KNOWN_KINDS` trong [packages/core/company/proposal-schema.ts](../../packages/core/company/proposal-schema.ts). Nếu không thêm, một loại lạ sẽ hiện như `unknown` và chỉ đọc, không ai duyệt được. Không dùng `standing_approval` để tự động duyệt bài đăng; mỗi bài vẫn cần người duyệt.

Yêu cầu với đề xuất `publish`:
- Người duyệt thấy bản nội dung, các phiên bản theo thị trường, kênh đích, lịch đăng, và kết quả của từng cổng gác.
- Duyệt, yêu cầu sửa kèm ghi chú, hoặc từ chối. Sửa xong phải qua lại các cổng, không được bỏ qua cổng.
- Một lần duyệt chỉ áp dụng cho đúng nội dung và đúng kênh đã hiển thị. Nếu nội dung hoặc kênh đổi, phải duyệt lại.
- Mỗi lần đăng có khoá idempotent (`item_id` + `channel_id` + `version`) để một lần duyệt không đăng hai lần.

### 5.2 Lịch đăng

Người duyệt chọn giờ đăng, hoặc để mặc định theo khung giờ của thị trường đích. Múi giờ luôn ghi rõ (`Asia/Ho_Chi_Minh` cho VN, múi giờ của người dùng cho thị trường quốc tế).

### 5.3 Kênh đăng và cấu hình

**Tab mới trong Cài đặt: "Kênh đăng".** Theo mẫu của `channels-tab.tsx` và `composio-tab.tsx` trong `packages/views/settings/components/`. Mỗi kênh có:

- nền tảng, thị trường, tên tài khoản hiển thị;
- trạng thái kết nối (chưa kết nối / sẵn sàng / lỗi / hết hạn), kèm thời điểm kiểm tra gần nhất;
- nút "Kết nối" mở liên kết xác thực của Ayrshare; nút "Đăng thử" đăng một bài nháp đánh dấu thử, nếu nền tảng cho phép xoá sau đó;
- giới hạn của kênh (độ dài, thời lượng video, tỉ lệ khung hình) đọc từ cấu hình, có ngày nguồn.

Khoá API và token **chỉ lưu ở phía server**, không đưa vào client hay Zustand, theo quy tắc trong `AGENTS.md`.

**Đường đăng (adapter).** Mỗi adapter cùng một giao diện: `connect`, `status`, `publish`, `delete` (nếu có).

| Adapter | Phủ được | Ghi chú | Trạng thái |
|---|---|---|---|
| Ayrshare | Các kênh ở mục 3 và 3.1 | Liên kết tài khoản qua profile và `POST /api/profiles/link-sessions` với header `Profile-Key`. Video cần gói trả phí. Có giới hạn 10 kết nối cho mỗi người dùng (một nguồn FAQ). **Gói và giá cần xác minh trên trang giá** | Đã chọn |
| Zalo OA trực tiếp | Bài viết và tin broadcast Zalo | Có SDK bên thứ ba mô tả API, chưa xác minh với tài liệu chính thức | Để sau, khi có OA |
| Webhook (Make, n8n) | Tuỳ kịch bản | Kit đã có `make-post.mjs` | Phương án dự phòng |

Lưu ý khi đăng qua Ayrshare: mỗi workspace multica tương ứng với một profile Ayrshare. Cần xác minh cách mô hình này khớp với cách multica quản lý workspace trước khi triển khai.

### 5.4 Ghi kết quả và phản hồi

Sau khi đăng, Publisher ghi trạng thái (`posted`, `failed`, `retry`), URL, và thời điểm. Lỗi phải hiện trong issue, không chỉ nằm trong log. Giai đoạn sau: `performance-review` đọc số liệu và đưa bài học vào brief kế tiếp.

## 6. Skill lấy từ kit

**Giữ**: `topic-radar`, `competitor-analysis`, `research-and-capture`, `story-video`, `repurpose`, `geo-optimize`, `localize-video`, `create-video`, `motion-craft`, `review-gate`, `freshness-check`, `performance-review`, `validate-post.mjs` (`scripts/social/`).

**Bỏ**: `ads-*`, `catalog-feed-audit`, `ugc-koc-brief`, `promo-calendar`, `audience-plan`, `brand-lessons` (8 thương hiệu Việt, trừ khi cần học thương hiệu cụ thể), `video-and-post` (đăng qua Make, thay bằng Publisher), `talking-head-edit` (giai đoạn sau, cần quay).

**Chỉnh**: mọi bước "publish" trong skill phải trỏ về Publisher và đề xuất đã duyệt, không tự đăng.

## 7. Rủi ro và giấy phép

- **Trùng với Marketing Studio (`kv_marketing`).** Ranh giới: Marketing giữ quảng cáo và chiến dịch trả phí; Content Studio giữ sản xuất và đăng nội dung tự nhiên. Hai squad không cùng gác một loại việc.
- **TikTok chặn đăng công khai khi chưa qua audit.** Nên nộp audit từ sớm; trước khi có kết quả, chỉ đăng được ở chế độ riêng tư.
- **LinkedIn trang công ty có thể bị từ chối.** Nếu bị từ chối, app không nộp lại được và phải tạo app mới. Trong thời gian chờ, dùng LinkedIn cá nhân.
- **Reddit và Hacker News.** Đăng tự động dễ bị coi là spam; để người duyệt đăng tay.
- **Giấy phép.** Kit là MIT; phần video kế thừa từ dự án MIT khác, template Apache-2.0 yêu cầu giữ NOTICE và LICENSES. Lấy về theo commit đã ghim, không copy vào repo. Ghi commit vào spec khi triển khai.
- **Phụ thuộc runtime.** Script của kit cần Node, ffmpeg, Python (crawl4ai) và các khoá API. Chưa kiểm tra runtime của daemon multica.
- **Chi phí.** Ayrshare trả phí, video cần gói cao hơn; TTS và stock có thể trả phí. Cần ngưỡng chi phí theo tháng.
- **Chính sách nền tảng và luật.** Nhãn nội dung AI, quy định quảng cáo Việt Nam, giới hạn kênh đổi theo thời gian. Market Gate phải dùng ngày kiểm và nguồn chính thức, không dựa vào trí nhớ.
- **Đăng nhầm kênh hoặc nhầm thị trường.** Adapter phải kiểm tra thị trường của kênh khớp với thị trường của nội dung trước khi gửi.

## 8. Các điểm chưa xác minh

- Gói Ayrshare nào đủ cho X, LinkedIn, Facebook Pages, TikTok và YouTube, và giá mỗi gói.
- Ayrshare đăng lên trang công ty LinkedIn bằng quyền nào, và có cần Community Management API không.
- Quy tắc đăng Reddit hiện tại; các nguồn mâu thuẫn nhau.
- Thời gian audit TikTok (nguồn bên thứ ba ước 2–6 tuần).
- Cách liên kết profile của Ayrshare có khớp với mô hình workspace của multica không.
- Composio (đã có trong `composio-tab.tsx`) có toolkit mạng xã hội nào và có phù hợp làm adapter không. Chưa kiểm tra.

## 9. Giai đoạn triển khai

1. **Cấu hình kênh.** Tab "Kênh đăng", trạng thái kết nối, adapter Ayrshare. Kết nối X và LinkedIn cá nhân, Facebook Pages. Không có squad; chỉ có kết nối và đăng thử.
2. **Đề xuất "publish".** Thêm loại đề xuất, nối vào hàng đợi quyết định, ghi kết quả đăng.
3. **Squad giai đoạn 1.** Lead, Scout, Researcher, Drafter, Localizer, ba cổng, Handover, Publisher. Chưa có Producer.
4. **Video.** YouTube, Producer, `create-video`; kiểm tra runtime ffmpeg và Node trước.
5. **Mở rộng.** TikTok sau audit, Threads, Bluesky, LinkedIn trang công ty sau khi xác minh quyền. Zalo OA khi có OA.

Mỗi giai đoạn có kiểm thử riêng theo `AGENTS.md`: helper cho logic đăng và trạng thái, component cho tab kênh, và test cho khoá idempotent và cho loại đề xuất mới.

## 10. Câu hỏi còn mở

1. Bắt đầu với X và LinkedIn cá nhân ở quốc tế, Facebook Pages ở Việt Nam có đúng ý bạn không?
2. Nộp audit TikTok ngay bây giờ (cần thời gian chờ) hay để sau?
3. Gói Ayrshare: sẽ chọn gói sau khi bạn xem giá trên trang của họ.
