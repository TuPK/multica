# Sales Loop: squad tìm khách và chăm sóc cơ hội đa ngành, dùng KB làm nơi lưu sự thật

Spec (nghiên cứu và thiết kế, chưa triển khai). Cùng họ với [bmad-planning-squad.md](bmad-planning-squad.md) và
[digital-company.md](digital-company.md); phần dữ liệu trong KB dựa trên [project-knowledge-ingest.md](project-knowledge-ingest.md).
Hình quy trình mà người dùng gửi chỉ là tham khảo: spec này không bám theo hình, theo SDLC hay theo BMAD, mà thiết kế lại từ mục tiêu.

Ngôn ngữ: tiếng Việt để người dùng đọc; tên lệnh, status, file, kind, skill và đoạn văn bản dành cho agent giữ tiếng Anh.

## 1. Tóm tắt

Một squad template/quickstart, gán vào một room sau, có việc là: **tìm cơ hội kinh doanh, tiếp cận, chăm sóc, chốt, rồi học lại từ kết quả**,
cho **bất kỳ ngành nào** và **nhiều mục tiêu kinh doanh cùng lúc**. Sáu quyết định:

1. **Tổng quát hóa bằng ba trục, không bằng danh sách nguồn.** Một *lane* (mục tiêu kinh doanh + phân khúc + khu vực + sản phẩm) chọn
   *signal family* (loại tín hiệu mua), và mỗi family được phục vụ bởi các *source adapter*. Tuyển dụng chỉ là một trong 17 family, và
   là tín hiệu của **mọi chức năng**, không riêng IT (mục 4).
2. **Nguồn = danh mục khởi đầu đã kiểm chứng + bộ adapter tổng quát + vòng khám phá theo ngành.** Squad chạy được ngay trên danh mục khởi
   đầu, rồi tự tìm thêm nguồn đúng ngành (hiệp hội, hội chợ, cổng đấu thầu, báo ngành) và đăng ký chúng kèm bằng chứng đã thử (mục 4.5, 4.6).
3. **KB là nơi lưu sự thật.** Mỗi khách, cơ hội, thư nháp, bài học là một tài liệu KB; trạng thái là *thuộc tính có kiểu* trên tài liệu. Cần thêm
   cơ chế nền này (properties, CLI, view bảng và kanban) và nó dùng chung được cho mọi squad (mục 7).
4. **Không cấu hình, chỉ xác nhận một lần.** Lần chạy hiệu chỉnh ngày 0 đọc KB và workspace, dựng lane, ICP và bộ nguồn, rồi nộp một proposal
   tối đa 3 câu hỏi. Trước khi bạn xác nhận squad chỉ khám phá, chưa soạn thư (mục 5).
5. **Agent không bao giờ gửi.** Đầu ra dừng ở thư/báo giá/bài nháp đã qua Checker; người duyệt và người gửi (mục 9, 10).
6. **Mạng xã hội là nguồn hạng nhất, đi qua một thang tiếp cận hợp pháp; người dùng tự thêm nguồn được và agent phải đào được.** Nguồn có sẵn gồm cả
   Facebook (Page, nhóm công khai, Ad Library), Instagram, Zalo OA, TikTok, YouTube, LinkedIn, Telegram, diễn đàn; mỗi nền tảng có đường tiếp cận
   thật đã kiểm, kèm điều kiện bật. Mọi nguồn thêm vào đi qua một **đường onboarding** (nhận diện, thử từng bậc, chấm mẫu, viết recipe, người xem mẫu rồi
   mới chạy) và kết thúc ở một trạng thái có lý do, không bao giờ im lặng. Phần còn lại, nơi thông tin nằm sau đăng nhập, do người chia sẻ và agent đào
   (mục 4.9 đến 4.13, 7.5).

Trang web công khai cần dựng bằng trình duyệt được đọc bằng một **Web engine tùy chọn** (Crawl4AI, mã nguồn mở Apache-2.0, chạy trên máy daemon; mục 4.13). Nó là động cơ cho
bậc 4 và 5 và cho bộ chạy recipe, không phải giải pháp cho mạng xã hội.

Giai đoạn P0 (template, skill, seed kind, bộ chạy recipe, onboarding nguồn qua issue; chưa đổi mô hình dữ liệu KB) đã dùng được; "xem trực quan" và
Source Studio đúng nghĩa là P1 (mục 12).

## 2. Mục tiêu và không mục tiêu

**Mục tiêu**
- Vận hành đa ngành, đa lĩnh vực: cùng một squad phục vụ bán phần mềm cho chuỗi F&B, bán máy cho nhà máy, bán dịch vụ tuân thủ cho SME,
  tìm nhà phân phối ở nước ngoài, săn gói thầu.
- Nhiều lane song song trong một workspace, mỗi lane có tín hiệu, nguồn, điểm số và mẫu thư riêng.
- Mọi dữ liệu nằm trong KB, xem và sửa được tại chỗ, tìm kiếm được, có nguồn và ngày.
- Chạy ngay sau quickstart; thêm ngành mới không đòi viết code.
- Nguồn có sẵn phủ cả mạng xã hội, người dùng chỉnh được; thêm một nguồn mới (link, handle, truy vấn, feed, tệp, connector, hoặc mô tả bằng lời) thì
  agent tự tìm cách đào và chứng minh bằng một mẫu thật trước khi chạy (mục 4.11).
- Squad không ngầm làm điều pháp luật hoặc điều khoản của nguồn cấm (mục 10).

**Không mục tiêu (bản này)**
- Tự gửi thư, tự báo giá cho khách, tự đăng bài.
- Bán cho người tiêu dùng đại trà (B2C hàng loạt): không có "một khách một thư".
- Đăng nhập thay người dùng hoặc dùng tài khoản ảo để đọc mạng xã hội, dùng cookie phiên của người dùng, vượt CAPTCHA, đọc nhóm hoặc hồ sơ riêng tư, mua danh
  sách, đoán email theo mẫu (mục 4.9). Thông tin nằm sau đăng nhập đi qua người chia sẻ (mục 4.12), không qua tự động hóa phiên.
- Thay CRM đầy đủ (hợp đồng, hóa đơn, hoa hồng).
- Phía cung ứng (tìm nhà cung cấp) và gọi vốn: cùng động cơ, nhưng để sau (mục 4.2).

## 3. Hiện trạng đã kiểm chứng

### 3.1 Tái dùng được

| Cái đã có | Ở đâu | Dùng cho |
| --- | --- | --- |
| Template squad: roles, charter, instructions, `builtinSkills`; quickstart idempotent | `packages/core/squads/templates.ts`, `quickstart.ts` | Khai báo `sales_loop` |
| Mẫu squad chạy theo lịch, không workflow, lead giao bằng sub-issue, critic kiểm, đầu ra là proposal | template `market_research` trong `templates.ts` | Khuôn gần nhất với squad này (gần hơn SDLC) |
| Skill của một squad template: thư mục trong `sdlc-skills/`, sinh bằng `pnpm generate:sdlc-skills`, gắn qua `builtinSkills` (`attachBuiltinSkills` chỉ tìm trong `SDLC_BUILTIN_SKILLS`); skill được mang cả `scripts/` (ví dụ `bmad-toolbox--bmad-deep-recon`); `SDLC_SKILL_NAMES` đã lọc skill chỉ dành cho planning | `packages/core/squads/sdlc-skills/`, `scripts/generate-sdlc-skills.mjs`, `templates.ts` | Chỗ đặt skill `sales-*` và `SALES_SKILL_NAMES`, cùng đường BMAD Planning đã đi. Thư mục `packages/core/squads/skills/` là cơ chế khác (skill của starter company do server cài), không gắn vào squad qua quickstart |
| KB: kind có mục bắt buộc và template, chính sách đăng `manual/auto_after_review/auto`, `--idempotency-key`, `--supersedes`, `--confidence`, `--valid-until`, `--tag`, draft/publish | `server/internal/service/knowledge_kind.go`, `knowledge_write.go`, `cmd_knowledge.go` | Lưu hồ sơ, bài học, tài liệu nền |
| 27 kind mặc định do server seed cho mỗi workspace | `defaultKnowledgeKinds` | Thêm kind `sales_*` cùng cơ chế |
| Tìm kiếm ngữ nghĩa (`multica knowledge search`), Wiki, cây thư mục | `packages/views/knowledge/` | Dossier tìm được, liên kết chéo |
| Proposal có kind, duyệt trên Office; `multica proposal create --squad` | `server/internal/service/proposal.go` | Cổng người ở P0 |
| Autonomy L0..L3, `company_state`, budget, governance log | `server/internal/service/autonomy.go` | Giới hạn chạy |
| Composio: Gmail đã có tên hiển thị; toolkit khám phá động | `server/internal/runtimeapps/connected_app.go`, `integrations/composio/service.go` | Kênh tùy chọn ở P2 |
| Danh mục kind do chính các squad khác ghi: `market_report`, `competitor_profile`, `customer_insight`, `gtm_plan` | `defaultKnowledgeKinds` | Đầu vào hiệu chỉnh và nguồn "khách của đối thủ" |

### 3.2 Chưa có (và là việc của spec này)

- Template chỉ có **một** `routine` (`SquadTemplate.routine`, `quickstart.ts` tạo một autopilot). Squad này cần ít nhất hai lịch.
- KB chỉ có tài liệu văn xuôi: không có trường có kiểu, không lọc, không bảng/kanban. Mỗi lần đổi trạng thái là một bản `supersedes`
  mới, tức đánh chỉ mục lại cả tài liệu.
- `PATCH /api/knowledge/kinds/{key}` và `PUT /api/knowledge/policies` chỉ dành cho người là owner/admin
  (`server/cmd/server/router.go`), nên quickstart không nên tự tạo kind hay policy. Server nên seed chúng (như 27 kind mặc định).
- Chưa kiểm: cách tạo một **base** mới (`sales`) ngoài các base seed; cách seed kind mới cho workspace **đã có**.
- `Squad` (`packages/core/types/squad.ts`) không có trường nào cho biết squad được tạo từ template nào, nên giao diện không biết khi nào hiện phần
  quản lý nguồn. Cần thêm `template_key` (mục 7.5).
- Agent đã có cấu hình MCP riêng (`packages/views/agents/components/tabs/mcp-config-tab.tsx`) và kết nối Composio có gắn công cụ vào tác vụ
  (`runtimeapps`; `packages/core/composio`), nhưng chưa có chỗ nào mô tả *một nguồn dữ liệu* (URL, cách đọc, trạng thái, chi phí) và chưa có bộ chạy
  cách đọc đó mà không cần LLM. Đó là phần mới của mục 4.9 và 7.5.
- Chưa kiểm: một kết nối Composio thuộc về người kết nối hay về workspace, và agent chạy tác vụ có dùng được kết nối đó không (quyết định có
  bậc 1 và bậc 6 chạy được trong lịch tự động hay không).

## 4. Mô hình tổng quát hóa đa ngành

### 4.1 Ba trục

```
Lane  (mục tiêu + phân khúc + khu vực + sản phẩm)
  └─ chọn  Signal families  (loại tín hiệu mua phù hợp lane)
              └─ được phục vụ bởi  Source adapters  (cách lấy dữ liệu, có kiểm chứng và sức khỏe)
```

Ngành nghề chỉ xuất hiện ở **dữ liệu** (lane, nguồn đã đăng ký), không xuất hiện ở **mã**. Thêm một ngành là thêm lane và nguồn.

### 4.2 Goal playbook (mục tiêu kinh doanh)

Mỗi lane có `goal`, quyết định *đối tượng*, *tín hiệu ưu tiên* và *cách tiếp cận*.

| goal | Tìm ai | Tín hiệu ưu tiên | Kênh tiếp cận thường gặp |
| --- | --- | --- | --- |
| `direct_sale` | Doanh nghiệp mua sản phẩm/dịch vụ của bạn | hiring, funding, expansion, launch, regulatory, pain, competitor-customer | email, điện thoại, biểu mẫu, giới thiệu |
| `local_business` | Cửa hàng, nhà hàng, phòng khám, trường, xưởng nhỏ trong một địa bàn | expansion/mở mới, directory, pain (đánh giá), events | điện thoại, chat/Zalo, ghé thăm, biểu mẫu |
| `channel` | Nhà phân phối, đại lý, reseller, đối tác tích hợp | directory (hiệp hội), events (exhibitor), competitor-customer, trade flow | email, giới thiệu, sự kiện |
| `tender` | Gói thầu công và tư mà bạn dự thầu được | tender | hồ sơ dự thầu (người làm), liên hệ bên mời thầu |
| `export_buyer` | Nhà nhập khẩu, người mua nước ngoài | trade flow, events, directory, news | email, sự kiện |

Mở rộng cùng động cơ nhưng chưa làm: `supplier` (tìm nhà cung cấp), `investor` (tìm nhà đầu tư), `talent`.

### 4.3 Signal family

Tín hiệu là *lý do một tổ chức có thể cần bạn lúc này*. Ví dụ đủ ngành để thấy không gắn với IT.

| # | Family | Nghĩa | Ví dụ ở nhiều ngành |
| --- | --- | --- | --- |
| 1 | `hiring` | Đang tuyển **bất kỳ chức năng nào**: nhu cầu và mở rộng | Tuyển quản lý cửa hàng (chuỗi bán lẻ mở rộng), kế toán trưởng (cần phần mềm kế toán), thợ vận hành CNC (nhà máy tăng ca), marketing (cần dịch vụ quảng cáo) |
| 2 | `funding` | Vừa huy động vốn, phát hành, vay | Startup gọi vốn, công ty niêm yết phát hành, nhà máy vay mở rộng |
| 3 | `expansion` | Mở chi nhánh, nhà máy, cửa hàng, thị trường mới | Khai trương cơ sở mới, đặt nền móng nhà máy, FDI mới đăng ký |
| 4 | `launch` | Ra mắt sản phẩm/thương hiệu/dự án | Thương hiệu mới, app mới, dự án bất động sản mở bán |
| 5 | `regulatory` | Quy định hoặc tiêu chuẩn mới có hạn thực thi | Hóa đơn điện tử, luật dữ liệu, tiêu chuẩn xuất khẩu, môi trường |
| 6 | `tender` | Gói thầu, yêu cầu báo giá | Mua sắm công, đấu thầu thiết bị y tế, RFP doanh nghiệp |
| 7 | `event` | Tham gia hội chợ, hội thảo, danh sách trưng bày/diễn giả | Danh sách đơn vị trưng bày của một triển lãm ngành |
| 8 | `pain` | Than phiền, đánh giá xấu, câu hỏi tìm giải pháp | Đánh giá ứng dụng của đối thủ, hỏi đáp diễn đàn ngành |
| 9 | `site_change` | Website hoặc công nghệ thay đổi | Trang mới, tên miền mới, chuyển nền tảng bán hàng |
| 10 | `registry` | Pháp nhân, nhãn hiệu, giấy phép mới | Doanh nghiệp mới thành lập, nhãn hiệu mới nộp đơn |
| 11 | `competitor_customer` | Khách của đối thủ | Logo và case study trên trang đối thủ |
| 12 | `trade_flow` | Nhập/xuất khẩu | Danh sách nhà nhập khẩu một mặt hàng |
| 13 | `leadership_change` | Lãnh đạo mới | Tin bổ nhiệm CEO/CMO/giám đốc nhà máy |
| 14 | `directory` | Tập tĩnh theo ngành | Danh bạ hội viên hiệp hội, khu công nghiệp, danh bạ phòng thương mại |
| 15 | `own_data` | Dữ liệu của chính bạn | Khách cũ, hộp thư, ghi chép họp, CSV bạn thả vào, tài liệu KB |
| 16 | `inbound` | Họ tự tìm đến | Biểu mẫu liên hệ, phản hồi thư, bình luận và tin nhắn vào Page/OA của bạn, form quảng cáo Lead Ads |
| 17 | `ad_activity` | Đang chạy quảng cáo | Thư viện quảng cáo của Meta (doanh nghiệp đang chi tiền để tăng trưởng, hoặc đối thủ đang đẩy sản phẩm nào) |

Hai ghi chú:
- `hiring` được dùng như **tín hiệu nhu cầu theo chức năng**. Lane bán cho công nghệ thì quan tâm vị trí IT; lane bán máy móc thì quan tâm vị
  trí sản xuất; lane bán dịch vụ kế toán thì quan tâm vị trí tài chính. Bản "tuyển IT" trong phân tích trước chỉ là một lane.
- `own_data` là family có chất lượng cao nhất và luôn bật.
- Mạng xã hội không phải một family mà là một **kênh mang tín hiệu**: một bài trong nhóm Facebook có thể là `pain` (hỏi tìm giải pháp), `hiring`, `launch`
  hay `inbound` tùy nội dung. Nền tảng được ghi ở `platform` của nguồn; family do nội dung bài quyết định (mục 4.10).

### 4.4 Source adapter

Mỗi nguồn là một tài liệu KB kind `sales_source` (mục 6) và một adapter chạy được. Adapter là bộ thu thập **cố định, không phụ thuộc LLM**:
lấy dữ liệu, chuẩn hóa về một dạng chung, ghi số liệu sức khỏe. LLM chỉ chấm điểm, viết hồ sơ và soạn thư.

Dạng chuẩn hóa của một *signal*: `source_id`, `family`, `entity_name`, `entity_key` (tên miền, nếu không có thì mã số thuế, nếu không có thì
tên + tỉnh), `url`, `observed_at`, `summary`, `raw_ref`, `region`, `sector_hint`.

Các loại adapter:

| Adapter | Dùng cho | Cấu hình |
| --- | --- | --- |
| `rss` | Báo, blog ngành, feed tuyển dụng, tin pháp luật | URL, từ khóa lọc |
| `json_api` | API công khai không khóa (việc làm, đấu thầu, hỏi đáp, app store) | URL mẫu, ánh xạ trường |
| `html_jsonld` | Trang liệt kê công khai có JSON-LD (nhiều trang tuyển dụng) | URL, selector |
| `html_list` | Danh bạ hiệp hội, danh sách đơn vị trưng bày, bảng thành viên | URL, selector hoặc mô tả trường |
| `web_search` | Truy vấn công cụ tìm kiếm của runtime (khi runtime có) | Mẫu truy vấn |
| `csv_drop` | Danh sách bạn thả vào KB | Ánh xạ cột |
| `kb_internal` | Tài liệu KB của squad khác (`competitor_profile`, `customer_insight`) | kind, từ khóa |
| `social_connector` | Dữ liệu **của chính bạn** trên mạng xã hội: bình luận, tin nhắn, form Lead Ads của Page/OA/TikTok/YouTube | Kết nối OAuth (Composio hoặc API chính thức) |
| `provider` | Nội dung công khai trên nền tảng đóng, qua nhà cung cấp dữ liệu (Apify, Firecrawl, Exa) | Token của bạn (Composio hoặc MCP), loại actor, trần chi phí |
| `human_share` | Bài, link, tệp do người dùng chia sẻ (nhóm kín, nhóm Zalo, LinkedIn) | Không; mục 4.12 |
| `mailbox` (P2) | Hộp thư, lịch qua connector tùy chọn | connector |

Ba adapter đầu và `kb_internal` đủ cho gần hết danh mục khởi đầu. Mỗi adapter chỉ cần shell và HTTP, nên chạy ở mọi runtime; chỉ `web_search`
phụ thuộc runtime (kiểm lúc hiệu chỉnh và báo trong view "Sức khỏe nguồn"). `social_connector` và `provider` chạy **trong tác vụ của agent**
với công cụ Composio/MCP gắn sẵn, không chạy bằng bộ chạy mã.

Cách một adapter đọc một nguồn cụ thể là một **recipe** khai báo (mục 4.9). Agent viết và sửa recipe; mã thực thi recipe.

### 4.5 Danh mục khởi đầu (đã gọi thử, 2026-11-10)

Kết quả gọi thử từ máy phát triển; khác máy, khác IP có thể khác. Đầy đủ ở phụ lục A. "OK" là HTTP 200 và nhận được dữ liệu dạng mong đợi.

**Tin tức và tín hiệu doanh nghiệp (mọi ngành)**

| Nguồn | Family | Trạng thái | Ghi chú |
| --- | --- | --- | --- |
| Google News RSS (`news.google.com/rss/search?q=...&hl=vi&gl=VN`) | funding, expansion, launch, regulatory, leadership_change | OK | Truy vấn do lane quyết định, đổi `hl/gl` theo khu vực |
| VnExpress Kinh doanh, VietnamNet, Dân trí, Tuổi Trẻ, Thanh Niên, CafeF, Vietnambiz, Vietstock (RSS) | expansion, funding, launch, regulatory | OK | Báo tổng hợp; lọc từ khóa theo lane |
| Tech in Asia (RSS) | funding, launch | OK | |
| GDELT DOC API (tin toàn cầu theo chủ đề và vị trí) | expansion, launch, trade_flow | **429** khi gọi dồn | Dùng được nếu giới hạn tốc độ; kiểm lại |
| Google Trends RSS (VN) | pain, launch | OK | Chủ đề đang nổi, không phải tổ chức; dùng để gợi truy vấn |

**Tuyển dụng (nhu cầu theo chức năng)**

| Nguồn | Phạm vi | Trạng thái | Ghi chú |
| --- | --- | --- | --- |
| CareerViet | Việt Nam, mọi ngành | OK (HTML) | robots cho phép |
| ITviec | Việt Nam, IT | OK (HTML có JSON-LD) | Chỉ hợp lane bán cho công nghệ |
| HN "Who is hiring" (Algolia) | Toàn cầu, công nghệ | OK | Không khóa |
| RemoteOK, Remotive, Himalayas, WeWorkRemotely | Toàn cầu, làm từ xa | OK | RemoteOK bắt buộc ghi nguồn và link; Himalayas cấm phân phối lại cho job board khác (đọc từ tóm tắt bên thứ ba); dùng nội bộ |
| Arbeitnow | Châu Âu | OK | Dữ liệu thiên về Đức/Anh/Pháp/Thụy Sĩ; chỉ hợp lane châu Âu |
| TopCV, VietnamWorks, TopDev | Việt Nam | Chặn (403, 404, không tới được) | Chỉ qua trình duyệt, dễ vỡ; không dùng ở bản đầu |

**Đấu thầu và mua sắm**

| Nguồn | Phạm vi | Trạng thái | Ghi chú |
| --- | --- | --- | --- |
| Cổng mua sắm công Việt Nam (`muasamcong.mof.gov.vn`) | Việt Nam | Trang tải được | **Chưa kiểm** trích xuất thông báo mời thầu |
| World Bank procurement notices API | Toàn cầu | OK | Không khóa |
| UK Contracts Finder (OCDS) | Anh | OK | Không khóa |
| TED (EU) | Châu Âu | Trang chủ trả 429 | API chưa thử |

**Pháp nhân và giấy phép (làm giàu và xác minh)**

| Nguồn | Trạng thái | Ghi chú |
| --- | --- | --- |
| `dangkykinhdoanh.gov.vn` | Tải được | Tra cứu cơ bản miễn phí theo [Nghị định 168/2025](https://thuvienphapluat.vn/phap-luat/cung-cap-thong-tin-dang-ky-doanh-nghiep-theo-nghi-dinh-1682025ndcp-thong-tin-dang-ky-doanh-nghiep-c-265795.html); dữ liệu đầy đủ tính phí (150.000 đ/sản phẩm). Dùng để xác minh, không để khám phá hàng loạt |
| infodoanhnghiep.com | OK | Tổng hợp từ bên thứ ba, độ tin cậy thấp hơn |
| masothue.com, hosocongty.vn, doanhnghiepmoi.vn, thongtindoanhnghiep.co | Chặn hoặc không tới được | Không dùng |
| OpenCorporates | 401 | Cần khóa; không dùng ở bản đầu |
| Wikidata SPARQL | OK | Doanh nghiệp lớn có hồ sơ, không dùng cho SME |

**Danh bạ, hội chợ, hiệp hội (family `directory`, `event`): nơi ngành khác nhau tách ra**
Không có một nguồn chung; mỗi ngành có hiệp hội và triển lãm riêng. Các trang sau đọc được công khai (tìm qua web, **chưa có adapter**):
danh bạ hội viên HASI (công nghiệp hỗ trợ), HAMI (cơ khí và điện), VASI (hỗ trợ), VMAA, VAFIE, AmCham Vietnam; danh sách đơn vị trưng bày
của một triển lãm cơ khí (MTA Vietnam, 455 đơn vị năm 2026). Dữ liệu doanh nghiệp theo khu công nghiệp có bán (Vietdata, trả phí).
Đây là lý do cần **vòng khám phá** (4.6) thay vì danh mục cố định.

**Pain, sản phẩm, cộng đồng**

| Nguồn | Family | Trạng thái |
| --- | --- | --- |
| Apple iTunes Search và customer reviews RSS (theo quốc gia) | pain, competitor_customer, launch | OK |
| Product Hunt (atom), danh bạ YC | launch, funding | OK |
| StackExchange API | pain | OK (cộng đồng kỹ thuật) |
| GitHub API | site_change, launch | OK, 60 request/giờ khi không khóa; chỉ hợp lane công nghệ |
| Luma, Meetup (HTML) | event | Tải được; trang dựng bằng JS, cần xác minh trích xuất |
| crt.sh (nhật ký chứng chỉ, tên miền mới) | site_change, registry | OK sau thử lại, không ổn định |
| Nominatim (địa lý) | làm giàu địa chỉ | OK, giới hạn 1 request/giây, bắt buộc User-Agent |
| Telegram kênh công khai (`t.me/s/<kênh>`) | pain, launch, inbound | OK, đọc được 20 tin |
| Reddit RSS theo subreddit | pain | OK; không có điểm số và số bình luận |
| Diễn đàn XenForo (`/forums/-/index.rss`), ví dụ voz.vn | pain, inbound | voz.vn OK; tinhte.vn 403 |
| X oEmbed (một bài), Pinterest RSS | | OK nhưng ít giá trị bán hàng |
| Mạng xã hội còn lại (Facebook, Instagram, TikTok, Zalo, LinkedIn, YouTube, X) | | Xem 4.10; đa số cần một lần kết nối |
| Reddit JSON, Bluesky search, SEC EDGAR, Overpass, ImportYeti | | Chặn hoặc không tới được |

**Dữ liệu của bạn (`own_data`, `kb_internal`, `csv_drop`)**: khách cũ và ghi chép họp; `customer_insight`, `competitor_profile` của squad khác
(trang case study của đối thủ cho family `competitor_customer`); CSV hoặc link bạn thả vào.

**Loại hẳn:** đăng nhập thay người dùng hoặc dùng tài khoản ảo, cookie phiên của người dùng, vượt CAPTCHA, đọc nhóm hoặc hồ sơ riêng tư, mua danh sách,
đoán email, dò SMTP. Mạng xã hội đi theo thang tiếp cận ở 4.9 và 4.10, không bị loại cả nền tảng.

### 4.6 Vòng khám phá nguồn theo ngành

Danh mục khởi đầu phủ tin tức, tuyển dụng, đấu thầu. Phần còn lại (hiệp hội, hội chợ, báo ngành, cổng đấu thầu địa phương) khác nhau theo ngành
nên squad tự tìm:

1. Với mỗi lane, Scout đọc `sales_icp` và liệt kê *loại nguồn* cần (hiệp hội ngành, hội chợ chính của ngành, báo chuyên ngành, cổng thầu,
   danh bạ phòng thương mại, khu công nghiệp) rồi dùng `web_search` (nếu runtime có) để tìm ứng viên.
2. Với mỗi ứng viên, thử gọi: HTTP có 200, có dữ liệu cấu trúc hoặc danh sách đọc được không, robots và điều khoản có chặn không, có cần đăng nhập
   hay khóa không. Kết quả ghi vào một `sales_source` ở trạng thái `proposed`, kèm bằng chứng thử và loại adapter đề xuất.
3. **Tự bật** nếu nguồn công khai, không đăng nhập, không khóa, robots cho phép, và adapter tổng quát (`rss`, `html_jsonld`, `html_list`) đọc được.
   Các nguồn còn lại (cần khóa, bị chặn, điều khoản không rõ) vào proposal để bạn quyết, không tự bật.
4. Nguồn bật rồi chịu giám sát sức khỏe (mục 8.4); nguồn chết hai lần liên tiếp thì chuyển `degraded` rồi `blocked`.
5. Giới hạn: tối đa số nguồn mới mỗi lane mỗi tuần (mặc định 5) để không phình chi phí.

Vòng khám phá và việc người dùng tự thêm nguồn dùng **cùng một đường onboarding** (4.11): chỉ khác ai đưa đầu vào. "Tự bật" ở bước 3 chỉ áp dụng cho bậc
1 đến 5 của thang tiếp cận (4.9); nguồn cần nhà cung cấp, kết nối chính chủ hoặc chi phí luôn chờ người.

### 4.7 Chấm điểm phù hợp

Điểm 0 đến 100 cho một account trong một lane, giải thích được trong dossier:

```
score = 100 × ( 0.35·fit + 0.30·signal + 0.15·timing + 0.10·reach + 0.10·proof )
fit    : khớp ICP của lane (phân khúc, quy mô, khu vực, vai trò người mua), 0..1
signal : độ mạnh và độ rõ của tín hiệu (loại family, số tín hiệu độc lập, có nguồn), 0..1
timing : mới đến đâu, so với cửa sổ của family (tuyển dụng 30 ngày, gọi vốn 90 ngày, hạn quy định), 0..1
reach  : có đường liên hệ công khai hợp lệ không, 0..1
proof  : bạn có bằng chứng liên quan để nói (case, năng lực) trong `sales_proof` không, 0..1
```

Cổng cứng, đặt `stage = disqualified` hoặc chặn soạn thư bất kể điểm: đã là khách; `do_not_contact`; ngoài ICP loại trừ; không có đường liên hệ
hợp lệ cho kênh dự định; tín hiệu không có nguồn. Ngưỡng mặc định: `>= 60` qualified, `40 đến 59` watch, `< 40` bỏ. Trọng số và ngưỡng là mặc định,
nằm trong `sales_lane` và chỉ đổi theo bài học đã xác nhận (mục 8.5).

### 4.8 Lane

Lane là đơn vị vận hành. Một workspace có nhiều lane, ví dụ:

- Lane 1: `direct_sale`, bán phần mềm quản lý cho chuỗi F&B ở TP.HCM; family `expansion`, `hiring`, `pain`, `directory`.
- Lane 2: `channel`, tìm nhà phân phối thiết bị ở Thái Lan; family `event`, `directory`, `trade_flow`.
- Lane 3: `tender`, thầu dịch vụ tư vấn cho cơ quan công; family `tender`, `regulatory`.

Mỗi lane có `sales_icp`, `sales_offer`, bộ nguồn, mẫu thư, trần hằng tuần và (khi đủ dữ liệu) trọng số riêng. Hiệu chỉnh ngày 0 dựng lane đầu
tiên từ KB; thêm lane là tạo một `sales_lane` mới (người hoặc Lead qua proposal) và squad chạy nó từ lịch kế tiếp.

### 4.9 Thang tiếp cận và recipe

Với mỗi nguồn, squad chọn **đường hợp pháp, rẻ và ổn định nhất mà vẫn đọc được dữ liệu**. Có bảy bậc, thử từ trên xuống; bậc đã dùng và bằng chứng thử
được ghi vào `sales_source`.

| Bậc | Đường | Ví dụ | Cần gì | Rủi ro chính |
| --- | --- | --- | --- | --- |
| 1 | Kết nối chính chủ | Page, inbox, form Lead Ads của **chính bạn** (Facebook, Instagram, Zalo OA, TikTok Business, YouTube), Gmail, Lịch | OAuth qua Composio hoặc API chính thức; bạn cấp quyền | Thấp: dữ liệu của bạn |
| 2 | Định dạng mở | RSS/Atom, JSON, oEmbed, sitemap, JSON-LD, kênh Telegram công khai, RSS diễn đàn XenForo | Không | Thấp |
| 3 | API công khai chính thức | YouTube Data API, Reddit (cần duyệt), X (trả theo lượt), Meta Page Public Content Access | Khóa hoặc duyệt | Chi phí, thời gian duyệt |
| 4 | Trang công khai, chưa đăng nhập, tải tĩnh | Trang web, danh bạ, phần công khai của một trang mạng xã hội | Không | Điều khoản, robots |
| 5 | Trang công khai, chưa đăng nhập, dựng bằng trình duyệt | Trang dựng bằng JS | Web engine (Crawl4AI, mục 4.13) hoặc công cụ trình duyệt của runtime | Dễ vỡ, bị chặn |
| 6 | Nhà cung cấp dữ liệu | Apify (Facebook Pages, Posts, Comments, Groups công khai, Reviews, Ads Library, Marketplace; TikTok; Instagram; Google Maps), Firecrawl, Exa | Token của bạn (Composio hoặc MCP), trần chi phí | Chi phí; điều khoản nền tảng (nhà cung cấp tự xử lý việc vượt chặn, bạn chịu trách nhiệm cách dùng) |
| 7 | Người chia sẻ | Dán link hoặc nội dung, tải CSV, chia sẻ bài từ nhóm kín mà bạn là thành viên | Không | Không có: người là người đọc |

**Ranh giới cứng, áp dụng cho mọi bậc:** không đăng nhập thay người dùng hoặc bằng tài khoản ảo; không dùng cookie phiên; không vượt CAPTCHA; không đọc
nhóm hoặc hồ sơ riêng tư. Dữ liệu cần đăng nhập thì chỉ có hai đường: bậc 1 (của chính bạn) hoặc bậc 7 (người đọc rồi chia sẻ).

**Recipe** là cách đọc một nguồn, lưu trong thân tài liệu `sales_source` (mục "Recipe") dưới dạng JSON có phiên bản:

```
{ "version": 1, "rung": 2, "method": "rss",
  "fetch":  { "engine": "http|crawl4ai", "url": "...", "params": {}, "pagination": { "type": "cursor", "max_pages": 3 }, "render": "none|browser" },
  "parse":  { "items": "...", "fields": { "entity": "...", "url": "...", "observed_at": "...", "text": "..." },
              "schema": { "...": "định dạng schema CSS/XPath của Crawl4AI, khi engine là crawl4ai" } },
  "filter": { "keywords_any": [], "keywords_none": [], "language": "vi", "since_days": 30 },
  "map":    { "family": "pain", "entity_from": "author|link_domain|page_name" },
  "provider": { "toolkit": "APIFY", "actor": "...", "input": {}, "cost_cap_usd": 5 },
  "cadence": "weekly", "rate_limit": "...", "max_items": 100,
  "verified_at": "2026-11-10", "sample_count": 20, "field_coverage": 0.92, "fallbacks": [ { "rung": 7 } ] }
```

Recipe không chứa bí mật; token nằm trong kết nối Composio hoặc MCP. **Agent viết và sửa recipe; mã thực thi nó.** Bậc 2 đến 5 chạy bằng bộ chạy
không dùng LLM (`multica sales collect`, trên daemon); bậc 1 và 6 chạy trong tác vụ của agent với công cụ gắn sẵn; LLM chỉ dùng để viết hoặc sửa recipe
và để chấm điểm các tín hiệu đã lọc. Nhờ vậy lịch chạy rẻ, lặp lại được và không phụ thuộc agent "tự tìm".

`fetch.engine` chọn động cơ đọc trang: `http` (tải tĩnh, mặc định cho bậc 2 và 4) hoặc `crawl4ai` (Web engine tùy chọn cho trang cần dựng bằng trình duyệt, bậc 5; mục 4.13).
Khi engine là `crawl4ai`, `parse.schema` dùng thẳng định dạng schema CSS/XPath của Crawl4AI, nên không cần tự thiết kế một ngôn ngữ selector.

**Trạng thái nguồn** (mục 6.2): `proposed`, `active` (có recipe đã qua mẫu), `needs_connector` (có đường hợp pháp nhưng cần bạn cấp quyền, khóa hoặc ngân sách;
luôn ghi *kết nối gì, chi phí ước tính, mở được gì*), `manual_only` (chỉ nhận qua chia sẻ), `degraded`, `blocked` (kèm lý do), `disabled`.

### 4.10 Mạng xã hội theo từng nền tảng

Thông tin hữu ích cho bán hàng nằm nhiều ở mạng xã hội, nhưng các nền tảng đã đóng dần. Bảng này là **thực tế đã kiểm**, từ tài liệu của nền tảng, các bài tổng hợp
có ngày và gọi thử ngày 2026-11-10. "Bật bằng" là việc phải làm một lần. Với mỗi dòng, squad có sẵn **mẫu nguồn** (recipe khung, trạng thái `needs_connector`
cho đến khi bật).

| Nền tảng | Tín hiệu bán hàng | Đường tiếp cận | Bật bằng | Đã kiểm |
| --- | --- | --- | --- | --- |
| Facebook, Page của bạn | Bình luận, tin nhắn, form Lead Ads: `inbound`, `pain` | Bậc 1 | Kết nối Facebook (Composio `FACEBOOK`, OAuth quản lý sẵn) | Toolkit có 64 công cụ |
| Facebook, Page công khai khác (đối thủ, khách, hiệp hội) | Bài đăng, bình luận, đánh giá, số theo dõi: `expansion`, `launch`, `pain`, `competitor_customer` | Bậc 3: Page Public Content Access (App Review và xác minh doanh nghiệp, thường nhiều tuần). Bậc 6: Apify Pages/Posts/Comments/Reviews (khoảng 2 đến 12 USD mỗi 1.000 mục) | Kết nối Apify | Gọi HTTP thường chỉ trả trang vỏ: tên và số theo dõi trong `og:description`, không có bài |
| Facebook, Group công khai | Bài hỏi tìm nhà cung cấp, tìm giải pháp: `pain`, `inbound` | Groups API đã bị Meta gỡ từ 22/04/2024. Bậc 6: Apify Groups (chỉ nhóm công khai, không đăng nhập). Nhóm kín: bậc 7 | Kết nối Apify | Meta Content Library chỉ dành cho nhà nghiên cứu phi lợi nhuận, không dùng được cho bán hàng |
| Facebook Ad Library | Ai đang chạy quảng cáo, nội dung, ngày bắt đầu: `ad_activity`, `launch`, `competitor_customer` | Trang web xem được mọi quảng cáo đang chạy, không cần tài khoản, không xuất được. API chính thức chỉ có quảng cáo chính trị/xã hội và quảng cáo giao tới EU/UK. Bậc 6: Apify Ads Library (khoảng 0,3 đến 10 USD mỗi 1.000 quảng cáo tùy actor). Hoặc bậc 7 | Kết nối Apify | Gọi HTTP thẳng trang thư viện trả 403 |
| Facebook Marketplace, đánh giá | Người bán và cửa hàng địa phương: `directory`, `pain` | Bậc 6 | Kết nối Apify | |
| Instagram | Tài khoản doanh nghiệp của bạn; hồ sơ công khai | Bậc 1 cho tài khoản của bạn (Composio `INSTAGRAM`). Hồ sơ công khai: bậc 6, hoặc Business Discovery (cần App Review) | Kết nối Instagram hoặc Apify | Trang công khai chỉ là vỏ (khoảng 700 KB), phần dữ liệu từ chối khách chưa đăng nhập |
| Zalo | OA của bạn: tin nhắn, danh sách người quan tâm, nhãn, form Zalo Ads: `inbound` | Bậc 1: API OA chính thức. Nhóm và tài khoản cá nhân không có API: bậc 7 | Tạo ứng dụng Zalo và cấp quyền OA (nhiều tính năng cần OA đã xác thực hoặc gói dịch vụ) | Tin Broadcast chỉ gửi 6h đến 19h59, một số loại không gửi 22h đến 7h. Squad chỉ soạn, không gửi |
| TikTok | Form lead gen và mentions của tài khoản bạn; video, hashtag công khai | Bậc 1: TikTok Business API (lead, mentions). Công khai: bậc 6 hoặc bậc 7 | Kết nối tài khoản Business, hoặc Apify | Research API chỉ cho nhà nghiên cứu phi thương mại; Commercial Content API chỉ có quảng cáo châu Âu; Creative Center không có API chính thức |
| YouTube | Kênh, video, **bình luận** của đối thủ và ngành (câu hỏi, than phiền): `pain`, `launch` | Bậc 3: Data API chỉ cần khóa, 10.000 đơn vị mỗi ngày, `search.list` tối đa 100 lần mỗi ngày. Bậc 2: RSS kênh | Tạo khóa API Google (hoặc Composio `YOUTUBE`) | RSS thử với một mã kênh trả 404, chưa xác nhận |
| LinkedIn | Công ty, người ra quyết định, tin tuyển dụng, bài đăng | API chính thức chỉ cho đối tác. Bậc 1 cho tài khoản bạn (Composio `LINKEDIN`, 25 công cụ). Công khai: bậc 6 là vùng xám nặng, không mặc định. Bậc 7: bạn xuất danh sách (Sales Navigator) rồi thả CSV | Kết nối LinkedIn; CSV | Trang công ty gọi HTTP thường trả 403. Không tự động hóa phiên đăng nhập |
| Reddit | Chủ đề và câu hỏi theo ngành (chủ yếu lane toàn cầu) | Bậc 2: RSS subreddit không cần khóa. API cần duyệt (từ 11/2025) | Không (RSS) | RSS OK, không có điểm số hay số bình luận; JSON không đăng nhập trả 403 |
| X (Twitter) | Bài đăng theo từ khóa | Bậc 3: trả theo lượt (0,005 USD mỗi bài, không còn bậc miễn phí) | Ứng dụng OAuth riêng (Composio `TWITTER`); mặc định tắt | oEmbed một bài OK |
| Telegram | Kênh công khai theo ngành và cộng đồng | Bậc 2: `t.me/s/<kênh>`. Nhóm kín: bậc 7 | Không | Đọc được 20 tin |
| Diễn đàn, cộng đồng (voz.vn, diễn đàn ngành) | Hỏi đáp tìm nhà cung cấp: `pain`, `inbound` | Bậc 2: RSS XenForo `/forums/-/index.rss` | Không | voz.vn OK; tinhte.vn 403 |
| Google Maps, đánh giá địa điểm | Cơ sở địa phương, đánh giá xấu: `directory`, `pain` | Places API (khóa, tính phí theo mức) hoặc bậc 6 (Apify Google Maps) | Khóa hoặc Apify | Chưa gọi thử |
| Tìm kiếm web (`site:facebook.com/groups ...`) | Bài công khai đã được lập chỉ mục | `web_search` của runtime (công cụ tìm kiếm chính thức) | Runtime có công cụ này | Google và Bing trả 200 khi gọi HTML, nhưng **không dùng** việc lấy kết quả từ trang tìm kiếm vì điều khoản |

**Một kết nối mở nhiều mẫu.** Kết nối **Apify** (Composio `APIFY`, khóa API; có cả `APIFY_MCP` đăng nhập OAuth) mở cùng lúc các mẫu Facebook Pages, Posts,
Comments, Groups, Reviews, Ad Library, Marketplace, Instagram, TikTok, Google Maps. Bốn kết nối chính chủ (Facebook, Instagram, LinkedIn, YouTube) mở dữ liệu của bạn.
Các toolkit này có trong danh mục Composio công khai; việc workspace của bạn bật được cái nào còn tùy cấu hình Composio (chưa kiểm).

**Chạy ngay không cấu hình** chỉ có: Telegram kênh công khai, Reddit RSS, RSS diễn đàn XenForo, và các nguồn ở 4.5. Facebook, Instagram, TikTok, Zalo, LinkedIn
không thể làm được như vậy, vì nền tảng đóng hoặc vì dữ liệu là của riêng bạn; nói khác đi là không có đường hợp pháp mà không cần bạn làm gì. Squad bù bằng ba thứ:
mẫu nguồn dựng sẵn cho từng nền tảng; một lần kết nối mở nhiều mẫu; và chia sẻ thủ công (4.12) dùng được từ ngày đầu.

**Chi phí tham khảo** (công bố trên trang nhà cung cấp tháng 9/2026, có thể đổi): Apify Facebook Posts và Groups khoảng 2 đến 7 USD mỗi 1.000 mục, Pages 5,4 đến 12 USD
mỗi 1.000 trang, Ads Library 0,3 đến 10 USD mỗi 1.000 quảng cáo; X 0,005 USD mỗi bài. Squad bắt buộc đặt trần chi phí (mục 8.4).

### 4.11 Onboarding nguồn: bất kỳ nguồn nào bạn thêm

Yêu cầu: người dùng thêm một nguồn thì agent phải đào được thông tin từ đó. Không ai bảo đảm được "đào được mọi thứ" (đăng nhập, chống bot, điều khoản), nên cam kết là:
**mọi nguồn thêm vào kết thúc ở một trạng thái có lý do và có bước tiếp theo, không bao giờ im lặng.**

Một đường duy nhất cho nguồn do người dùng thêm, do vòng khám phá (4.6) tìm ra, hoặc do một lần chia sẻ mở ra:

1. **Nhận diện.** Từ link, handle, truy vấn, feed, tệp, connector hoặc mô tả bằng lời, xác định nền tảng, loại nguồn, ngôn ngữ, khu vực (mẫu URL, tự khám phá feed, sau đó LLM).
2. **Đi thang.** Với mỗi bậc áp dụng (4.9), thử lấy tối đa 20 mục. Ghi mỗi lần thử: bậc, HTTP, số mục, trường có được, thời gian, chi phí. Phần máy móc (khám phá RSS và JSON-LD,
   oEmbed, sitemap, nhận diện nền tảng, thử trang tĩnh) do `multica sales probe` làm không cần LLM; Scout chỉ viết schema trích dữ liệu và ánh xạ trường cho trang HTML (có Web engine thì bằng `generate_schema` của Crawl4AI, mục 4.13).
3. **Chấm mẫu.** Đạt khi: từ 5 mục trở lên; các trường bắt buộc (`entity`, `url`, `observed_at`, `summary`) phủ từ 80%; mới trong cửa sổ của family; không toàn trùng; không chứa nội dung riêng tư.
4. **Viết recipe** từ bậc đầu tiên đạt, kèm bậc dự phòng. Với trang HTML, schema do LLM sinh một lần từ HTML mẫu rồi chạy lại không cần LLM; LLM chỉ vào lại khi nguồn đổi cấu trúc.
5. **Xem mẫu.** Người dùng thấy 10 tín hiệu đã chuẩn hóa, family được gán, và "nếu là khách thì hồ sơ sẽ như thế này". Chọn: kích hoạt, sửa recipe, thử bậc khác, hoặc chỉ nhận thủ công.
6. **Chạy.** Theo lịch, sức khỏe hằng ngày. Nếu lỗi vì nguồn đổi cấu trúc, Scout dựng lại recipe (tối đa 2 lần mỗi tuần); bản mới phải qua lại bước 3 trước khi thay bản cũ;
   hết cách thì `degraded` và báo trong báo cáo tuần.

Kết quả luôn là một trong: `active`; `needs_connector` (nói rõ kết nối gì, chi phí ước tính, mở được gì); `manual_only` (kèm cách chia sẻ); `blocked` (kèm lý do: cần đăng nhập, chặn bot, điều khoản).

**Quyền:** thành viên thêm được nguồn ở bậc 1 đến 5 và 7. Bậc 6 và kết nối chính chủ cần owner hoặc admin xác nhận một lần (tốn phí và đụng điều khoản); xác nhận ghi vào log quản trị.

### 4.12 Chia sẻ thủ công và đào sâu

Nơi có thông tin tốt nhất nhưng đóng: nhóm Facebook kín, nhóm Zalo, LinkedIn. Người bán đã là thành viên và đang đọc; **người đọc, agent đào**.

- **Chia sẻ:** dán link hoặc nội dung (kèm ảnh chụp) vào ô "Chia sẻ" của Source Studio hoặc một quick action; tạo issue giao cho squad (có ngay ở P0); tải CSV.
  Về sau: bookmarklet hoặc tiện ích trình duyệt, và chia sẻ từ app di động (chưa kiểm app di động có hỗ trợ nhận chia sẻ; xem mục 15).
- **Đào (route F):** phân loại ý định (tìm giải pháp, than phiền, thông báo, tuyển dụng, không liên quan); trích nhu cầu; xác định doanh nghiệp (trang công khai, website, đăng ký kinh doanh);
  làm giàu; khử trùng theo `entity_key`; chấm điểm; tạo hoặc gắn vào `sales_account` hay `sales_opportunity`; soạn **gợi ý trả lời công khai** (kênh `public_reply`, người tự đăng) hoặc thư riêng,
  rồi qua Checker.
- **Cá nhân:** nếu tác giả là một cá nhân không nêu doanh nghiệp, chỉ lưu liên kết và tóm tắt nhu cầu, **không dựng hồ sơ cá nhân**, không soạn thư riêng.
- **Lưu trữ:** mỗi lần chia sẻ là một tài liệu `sales_share`; sau 90 ngày (mặc định) chỉ giữ tóm tắt không danh tính.

### 4.13 Web engine: Crawl4AI

**Vai trò.** Động cơ đọc **trang web công khai** cho bậc 4 và 5 và cho bộ chạy recipe. Nó không phải giải pháp cho mạng xã hội: Facebook, Instagram, TikTok, LinkedIn và Zalo vẫn đi bằng
bậc 1, 6 hoặc 7 (4.10), vì trang bị chặn hoặc bắt đăng nhập, không phải vì thiếu trình duyệt.

**Là gì** (theo tài liệu chính thức, bản 0.9.5 ngày 14/10/2026): thư viện Python Apache-2.0 điều khiển Playwright; ra Markdown sạch; trích dữ liệu có cấu trúc bằng schema CSS/XPath **không cần LLM**, hoặc
bằng LLM; `generate_schema` để LLM viết schema một lần từ HTML mẫu; đọc sitemap và khám phá URL; crawl sâu và crawl thích ứng; chạy nhiều URL song song với bộ điều phối theo bộ nhớ; cache; hook.
Chạy như thư viện hoặc như **Docker server** có REST (`/md`, `/html`, `/crawl`, `/execute_js`, `/screenshot`) và MCP (SSE `/mcp/sse`, WebSocket `/mcp/ws`; công cụ `md`, `html`, `crawl`, `execute_js`, `screenshot`, `pdf`).
Từ 0.9.0 server **bắt buộc token** (không có token thì chỉ nghe trong container).

**Dùng ở đâu trong squad.**
1. **Recipe.** `fetch.engine = "crawl4ai"` cho nguồn cần dựng bằng trình duyệt; `parse.schema` là schema CSS/XPath của Crawl4AI (4.9). Bộ chạy `multica sales collect` gọi engine, không dùng LLM.
2. **Onboarding (4.11).** Scout cho `generate_schema` đọc HTML mẫu, ra schema; bộ chạy chạy schema và chấm độ phủ trường ở bước chấm mẫu; chỉ viết lại schema khi nguồn đổi cấu trúc.
3. **Khám phá nguồn (4.6).** Đọc sitemap và khám phá URL cho danh bạ hiệp hội, danh sách đơn vị trưng bày hội chợ, báo chuyên ngành: loại nguồn khác nhau ở mỗi ngành.
4. **Công cụ của agent.** Gắn MCP của Crawl4AI (SSE) vào `mcp_config` của Sales Scout (và của Lead khi hiệu chỉnh) để đọc website của một doanh nghiệp khi đào bài chia sẻ (4.12) và khi dựng lane.
   Multica đã đọc `mcp_config` cho khoảng 24 loại runtime (`packages/core/agents/mcp-support.ts`) và hỗ trợ transport SSE và HTTP trong cấu hình MCP của agent.
5. **Không dùng cho** các nền tảng xã hội đóng (xem trên) và các trang chống bot mạnh (xem "Giới hạn").

**Triển khai: tùy chọn, không phải điều kiện để squad chạy.**
- Một container `unclecode/crawl4ai` (phiên bản ghim) trên máy chạy daemon, ở `localhost:11235`, `CRAWL4AI_API_TOKEN` ngẫu nhiên, `--shm-size=1g`. Token nằm trong cấu hình daemon, không nằm trong recipe.
- `multica sales probe` và `multica sales collect` phát hiện engine. Chưa có engine mà nguồn cần trình duyệt thì nguồn chuyển `needs_connector` với câu "cần Web engine: chạy một container (lệnh)", không chạy dở và không âm thầm hạ cấp.
- Source Studio hiện thẻ **Web engine** (đang chạy hay chưa, phiên bản, số trang đang xử lý, bộ nhớ), cạnh các connector (7.5).
- Lựa chọn khác khi người dùng không muốn tự chạy: **Crawl4AI Cloud** (hosted, 1.000 credit miễn phí mỗi tháng, gói trả phí từ 10 USD; URL đi qua bên thứ ba, nên xếp vào bậc 6 và cần owner/admin xác nhận);
  **Firecrawl** qua Composio `FIRECRAWL` (tính phí theo trang; bản tự chạy dùng AGPL-3.0 và không có lớp chống bot) khi workspace đã có sẵn. Cả hai là `provider`, không thay Web engine mặc định.

**Hardening bắt buộc.**
- **Tắt mặc định:** stealth và undetected-browser, hồ sơ trình duyệt lưu đăng nhập, cookie của người dùng, proxy xoay vòng, giải CAPTCHA. Các chế độ này đụng ranh giới cứng của 4.9. Bật stealth chỉ theo từng nguồn, owner/admin xác nhận,
  và không bao giờ cho nền tảng có điều khoản cấm (mục 10).
- **Chống lợi dụng:** engine nhận URL từ nguồn do người dùng hoặc vòng khám phá thêm, nên chỉ nghe loopback, có token, không mở ra mạng, và chặn đích tới dải IP nội bộ (loopback, mạng riêng, link-local, địa chỉ metadata)
  để không bị dùng đọc dịch vụ trong mạng nội bộ. Crawl sâu qua API tắt theo mặc định của Crawl4AI; chỉ bật cho recipe khám phá với giới hạn số trang.
- **Lịch sự:** tôn trọng robots.txt (cần kiểm cờ trong bản đang dùng), giới hạn tốc độ theo tên miền, số trang tối đa mỗi lần chạy, User-Agent nhận diện được.
- **Tài nguyên:** giới hạn số trang đồng thời, timeout mỗi trang, đóng trình duyệt sau mỗi lần chạy.

**Giới hạn, nói rõ.**
- Chống bot mạnh (Cloudflare, DataDome và tương tự): Crawl4AI không lo proxy hay CAPTCHA; các bài so sánh độc lập 2026 đều xếp khả năng chống bot của nó ở mức hạn chế. Trang như TopCV (gọi HTTP thường trả 403)
  nhiều khả năng vẫn bị chặn, nên nhóm đó vẫn nằm ngoài bản đầu (4.5).
- Bảo trì: dự án có một người đóng góp chính và hàng trăm issue mở; vì vậy ghim phiên bản, có test hợp đồng với fixture, và không để squad phụ thuộc engine (4.9 và "Triển khai").
- **Chưa có số đo thành công trên nguồn của bạn.** Lần cài thử trong phiên nghiên cứu bị kẹt khi tải phụ thuộc nặng (Playwright, scipy) trên máy phát triển, nên chưa đo được. Spike ở mục 13 làm việc đó.
- Pháp lý không đổi: công cụ không biến việc đọc nội dung cá nhân thành hợp pháp; mục 10 áp dụng nguyên.

## 5. Không cấu hình: hiệu chỉnh ngày 0

Squad không thể biết bạn bán gì cho ai, nên đầu vào tối thiểu là **một lần xác nhận**, không phải một biểu mẫu cấu hình.

1. **Đọc.** Lead tìm trong KB (`multica knowledge search`): `product_brief`, `gtm_plan`, `market_report`, `customer_insight`, `competitor_profile`,
   `playbook`, `policy`; và liệt kê dự án, sản phẩm trong workspace. Ngôn ngữ giao diện và múi giờ cho khu vực mặc định.
2. **Dựng nháp.** Lead viết `sales_offer` và `sales_icp` kind `confidence: low`, tạo `sales_lane` (`status: draft`), chọn family phù hợp với `goal`,
   chọn adapter từ danh mục khởi đầu, sinh truy vấn mặc định. Các mẫu mạng xã hội hiện ở `needs_connector`, mỗi mẫu kèm một câu "kết nối gì thì mở được gì".
3. **Hỏi một lần.** Một proposal kind `sales_calibration`, tối đa ba câu, mỗi câu có đáp án đề xuất bấm một lần: *bạn bán gì (xác nhận offer)*;
   *bán cho ai và ở đâu (xác nhận lane)*; *có thông tin nào không được nói (xác nhận giới hạn)*.
4. **Chế độ khám phá trước khi xác nhận.** Squad quét và ghi hồ sơ `watch`, **chưa soạn thư**, chưa báo giá. Sau khi xác nhận, lane chuyển `active`.
5. **KB trống.** Nếu thiếu cả offer lẫn ICP và không suy ra được, Lead không chạy bừa mà nộp một proposal xin thông tin tối thiểu. Chạy khi
   thiếu nền là nguồn của thư rác và báo giá bịa.

**Mở khóa nguồn xã hội.** Ngoài ba câu hỏi, proposal có một danh sách *thông tin* (không phải câu hỏi): những nguồn đang chờ kết nối, kèm kết nối cần có, chi phí ước tính và việc nó
mở được cho lane nào. Bạn kết nối khi muốn, ở Source Studio (mục 7.5); không kết nối thì squad vẫn chạy trên các nguồn không cần cấu hình và nhận chia sẻ thủ công.

Hiệu chỉnh chạy lại khi bạn thêm lane, đổi offer, hoặc khi `sales_icp` quá `valid_until`.

## 6. Mô hình dữ liệu trong KB

### 6.1 Nguyên tắc

- Một thực thể là một **tài liệu KB**: phần văn xuôi (dossier, thư, bản phân tích) tìm kiếm và liên kết được; phần **thuộc tính có kiểu** để lọc, sắp xếp,
  kéo thả. Đổi thuộc tính không đánh chỉ mục lại tài liệu.
- Khóa chống trùng: `idempotency key = sales:<kind>:<lane>:<entity_key>` (hoặc theo id sự kiện với outreach).
- Chính sách đăng của các kind `sales_account`, `sales_opportunity`, `sales_outreach`, `sales_source`, `sales_lane`: `auto` (agent ghi, có thể đọc ngay).
  `sales_icp`, `sales_offer`, `sales_proof`, `sales_voice`, `sales_templates`: `manual` (người duyệt trước khi thành "sự thật" của squad).
  `sales_lesson`: `auto_after_review`.
- Base riêng `sales` để pipeline không làm nhiễu tìm kiếm `market`. (Chưa kiểm cách tạo base mới; xem 3.2.)
- Dữ liệu cá nhân tối thiểu: chỉ ghi liên hệ công khai do chính tổ chức công bố, kèm `contact_source`.

### 6.2 Kind và thuộc tính

Mọi kind có các mục bắt buộc (heading) như `knowledge_kind.required_sections`. Kiểu: `text`, `select`, `number`, `date`, `bool`, `url`.

**`sales_lane`** (mục bắt buộc: Goal, Segment, Offer, Signals, Sources)

| Thuộc tính | Kiểu | Ghi chú |
| --- | --- | --- |
| `goal` | select | `direct_sale`, `local_business`, `channel`, `tender`, `export_buyer` |
| `status` | select | `draft`, `active`, `paused` |
| `regions` | text | Ví dụ `VN-HCM`, `TH` |
| `offer` | text | Tiêu đề `sales_offer` |
| `families` | text | Danh sách family bật |
| `weekly_cap` | number | Số account qualify tối đa mỗi tuần |
| `min_score` | number | Ngưỡng qualified |

**`sales_source`** (mục: What it gives, How to read it, Recipe, Terms and limits, Probe log)

| Thuộc tính | Kiểu | Ghi chú |
| --- | --- | --- |
| `platform` | select | `web`, `facebook`, `instagram`, `zalo`, `tiktok`, `youtube`, `linkedin`, `reddit`, `x`, `telegram`, `forum`, `maps`, `registry`, `tender`, `file`, `other` |
| `family` | select | 17 family; nguồn xã hội có thể để trống vì family do nội dung bài quyết định |
| `adapter` | select | `rss`, `json_api`, `html_jsonld`, `html_list`, `web_search`, `csv_drop`, `kb_internal`, `social_connector`, `provider`, `human_share`, `mailbox` |
| `rung` | number | Bậc của thang tiếp cận đang dùng (1 đến 7) |
| `url` | url | Link, handle hoặc truy vấn gốc người dùng nhập |
| `regions`, `sectors` | text | |
| `auth` | select | `none`, `key`, `oauth`, `provider_token` |
| `terms` | select | `ok`, `attribution`, `restricted`, `unknown` |
| `status` | select | `proposed`, `active`, `needs_connector`, `manual_only`, `degraded`, `blocked`, `disabled` |
| `added_by` | select | `starter`, `user`, `discovery`, `share` |
| `last_run`, `last_http`, `hits_7d`, `hits_30d` | date, number, number, number | Cập nhật mỗi lần chạy |
| `qualified_30d` | number | Số account `qualified` sinh ra từ nguồn này (đo hiệu quả) |
| `cost_30d` | number | Chi phí nhà cung cấp 30 ngày (USD) |
| `recipe_version` | number | |

**`sales_account`** (mục: Record, Why now, Evidence, Contact path, Fit)

| Thuộc tính | Kiểu | Ghi chú |
| --- | --- | --- |
| `entity_key` | text | Khóa chống trùng |
| `lane` | text | |
| `country`, `province`, `sector` | text | |
| `size_band` | select | `micro`, `small`, `medium`, `large`, `unknown` |
| `signal_family`, `signal_url`, `signal_date` | select, url, date | Tín hiệu chính |
| `fit_score` | number | 0..100 |
| `stage` | select | `new`, `watch`, `qualified`, `contacted`, `replied`, `meeting`, `proposal`, `won`, `lost`, `disqualified` |
| `do_not_contact` | bool | Chặn cứng |
| `contact_channel` | select | `email`, `phone`, `form`, `chat`, `visit`, `introduction` |
| `contact_value`, `contact_source` | text, url | Mỗi liên hệ phải có nguồn |
| `owner` | text | Thành viên phụ trách |
| `next_action_date`, `last_touch` | date | |

**`sales_opportunity`** (mục: Need, Decision, Next step): `account`, `lane`, `stage` (`discovery`, `proposal`, `negotiation`, `won`, `lost`), `value`,
`currency`, `timeline`, `budget_signal` (`none`, `hinted`, `confirmed`), `close_reason`, `next_action_date`.

**`sales_outreach`** (nội dung thư/kịch bản là phần thân tài liệu): `account`, `opportunity`, `channel`, `state` (`draft`, `checked`, `approved`,
`sent`, `replied`, `rejected`), `verdict` (`pass`, `concerns`, `fail`), `compliance` (`ok`, `missing`), `sent_date`.

**`sales_lesson`** (mục: Pattern, Evidence, Applies to; dùng `--confidence` và `--valid-until` sẵn có): `lane`, `evidence_count`, `family`, `effect`
(`raise_weight`, `lower_weight`, `new_rule`, `template_change`).

**`sales_share`** (mục: Shared content, What it says, Outcome; thân tài liệu là nội dung người dùng chia sẻ): `platform`, `url`, `state` (`new`, `processed`, `rejected`, `expired`),
`shared_by`, `shared_at`, `intent` (`seeking`, `complaint`, `announcement`, `hiring`, `irrelevant`), `account` (kết quả), `expires_at`.

**Tài liệu nền** (một bộ cho mỗi lane, hoặc dùng chung): `sales_icp` (Segments, Buyers, Triggers, Disqualifiers, Regions), `sales_offer`
(Offerings, Price book, Terms), `sales_proof` (Cases, Capabilities, Numbers), `sales_voice` (Tone, Say, Avoid), `sales_templates` (Approved templates).
Giữ đúng tinh thần sáu tài liệu của hình, nhưng bỏ "câu hỏi hay gặp" thành một mục trong `sales_proof`.

### 6.3 Vòng đời

- `stage` của account chỉ tiến theo luật (agent không lùi trạng thái do người đặt; người đặt được mọi chiều). Mọi thay đổi ghi lịch sử (ai, khi nào, từ gì sang gì).
- Account `qualified` quá 90 ngày không có hoạt động thì `review_days` của kind đưa vào hàng xem lại.
- `sales_account` chưa qualified không phải tài liệu riêng: các ứng viên bị loại nằm trong **báo cáo quét** của lần chạy (một tài liệu kind `report`)
  với lý do loại, và `entity_key` của chúng được tra khi quét lại để khỏi lặp.

## 7. Cơ chế nền trong KB: tài liệu có thuộc tính

Đây là phần cho phép bạn **xem trực quan ngay khi cần**, và dùng chung cho mọi squad (theo dõi đối thủ, hỗ trợ khách, rủi ro...).

### 7.1 Server

- Migration: cột `properties JSONB NOT NULL DEFAULT '{}'` trên `knowledge_link`; cột `fields JSONB NOT NULL DEFAULT '[]'` trên `knowledge_kind`
  (danh sách `{key, label, type, options?, required?}`). Chỉ mục (nếu cần cho lọc) tạo `CONCURRENTLY`, mỗi chỉ mục một migration riêng. Không khóa ngoại.
- Kiểm tra khi ghi: thuộc tính phải khớp `fields` của kind (kiểu, `select` nằm trong `options`); khóa lạ bị từ chối. Mục bắt buộc của kind giữ nguyên.
- `POST /api/knowledge/documents` nhận `properties`; mới: `PATCH /api/knowledge/documents/{id}/properties` (chỉ sửa thuộc tính, không đụng WeKnora) cho
  member và agent, có `updated_by_type/id` và dòng lịch sử.
- `GET /api/knowledge/documents` nhận bộ lọc thuộc tính (`kind`, `field.<key>=<value>`, so sánh ngày), sắp xếp, phân trang.
- Seed kind `sales_*` (kèm `fields` và mục bắt buộc) và chính sách đăng vào `defaultKnowledgeKinds` và cách seed hiện có; seed base `sales`.

### 7.2 CLI

```
multica knowledge add --kb sales --kind sales_account --title "<name>" --file dossier.md \
    --idempotency-key sales:account:<lane>:<entity_key> --field stage=qualified --field fit_score=72 ...
multica knowledge update <id> --field stage=contacted --field next_action_date=2026-11-20
multica knowledge list --kind sales_account --field stage=qualified --field lane=<lane> --output json
```

`update` chỉ đổi thuộc tính. Đổi nội dung vẫn là `--supersedes`.

### 7.3 Giao diện (web và desktop, trong `packages/views/knowledge/`)

Tab **Collections** trong trang KB, hiện cho mọi kind có `fields`:

- **Bảng:** cột theo `fields`, lọc nhiều giá trị, sắp xếp, ẩn/hiện cột (dùng `collections/view-store.ts` sẵn có), tìm kiếm.
- **Kanban:** nhóm theo một trường `select` (mặc định `stage`); kéo thả để đổi, đó là thao tác của người.
- **Khung chi tiết:** thuộc tính sửa tại chỗ, dossier (`ReadonlyContent`), lịch sử thay đổi, liên kết tới account/opportunity/outreach liên quan.
- **View lưu sẵn cho Sales:** *Pipeline* (kanban `stage`), *Cần bạn* (`sales_outreach.state = checked` và báo giá chờ duyệt), *Đến hạn* (`next_action_date <= hôm nay`),
  *Bài học* (theo `confidence`), *Sức khỏe nguồn* (`sales_source`).
- **Phòng Sales trong tab Company:** thẻ số liệu (account mới tuần này, đang chờ bạn, đến hạn, thắng/thua tháng này) và đường vào các view trên.

Tuân thủ quy tắc UI: mọi chuỗi qua i18n đủ bảy ngôn ngữ (en, zh-Hans, zh-Hant, fr, es, ko, ja), mô tả ngắn, không lặp tiêu đề; zod schema và
`parseWithFallback` cho phản hồi mới (trường lạ, enum lạ có fallback); khóa query có `wsId`; Zustand chỉ giữ bộ lọc/ẩn cột.

### 7.4 P0: xem được trước khi có cơ chế này

Trước P1, thuộc tính nằm trong mục `Record` của tài liệu (các dòng `key: value`), và Lead ghi lại một **tài liệu snapshot** kind `report` sau mỗi lần chạy:
bảng markdown theo giai đoạn, có link tới từng hồ sơ, danh sách "cần bạn" và "đến hạn". Trang KB hiện có đọc được. Khi P1 vào, server đọc mục `Record` để điền
`properties` cho tài liệu cũ, nên không cần di chuyển dữ liệu tay. Giới hạn của P0: đổi trạng thái là một bản `--supersedes`, chấp nhận được ở quy mô
vài chục account mỗi tuần.

### 7.5 Source Studio: quản lý nguồn cho loại squad này

Tính năng riêng cho squad Sales Loop: nơi bạn thấy, chỉnh và thêm nguồn, kết nối công cụ, xem mẫu, chia sẻ bài và giữ chi phí trong tầm tay.

**Nhận ra loại squad.** Thêm cột `squad.template_key TEXT` (null được, không khóa ngoại), quickstart đặt `sales_loop`. `Squad.template_key?: string | null` trong `packages/core/types/squad.ts`
(backend cũ không gửi trường này thì giao diện không hiện Studio). Dùng lại được cho các template sau.

**Vị trí.** Phần **Sources** trong trang chi tiết squad (`packages/views/squads/components/`, thêm `squad-sources-section.tsx`, hiện khi `template_key = sales_loop`) và một chip "Nguồn: 14 chạy, 2 cần bạn"
trên phòng Sales ở tab Company. Cả hai chia sẻ một component trong `packages/views`, dùng cho web và desktop.

**Danh sách nguồn** (là một Collection của kind `sales_source`, mục 7.3, nên cột, lọc, sắp xếp có sẵn): tên, nền tảng, family, bậc, trạng thái, lần chạy gần nhất, số mục 7 ngày,
**số account qualified 30 ngày** (hiệu quả thật của nguồn), chi phí 30 ngày, người thêm. Nguồn có sẵn (`starter`) sửa được như nguồn của người dùng; "đặt lại về mặc định" có cho nguồn starter.

**Thêm nguồn.** Một ô nhập: "Dán link, @handle, RSS, truy vấn, hoặc mô tả bằng lời", cùng các lối vào: Tệp CSV, Connector, Chia sẻ bài. Sau khi gửi, giao diện hiện từng bước của onboarding (4.11):
nhận diện, từng bậc đã thử (đạt hay không và vì sao), mẫu 10 tín hiệu, rồi các nút **Kích hoạt**, **Sửa**, **Thử bậc khác**, **Chỉ nhận thủ công**. Nếu cần kết nối hoặc chi phí, nút dẫn tới
đúng connector và hiện chi phí ước tính.

**Chi tiết một nguồn.** Form sửa các trường thường dùng (link, từ khóa gồm/loại, khu vực, ngôn ngữ, lịch, số mục tối đa, trần chi phí, ánh xạ family); phần nâng cao hiện recipe JSON có kiểm
schema; nút **Chạy thử** (cùng bước chấm mẫu), **Nhờ agent sửa**, lịch sử chạy (HTTP, số mục, chi phí, lỗi), nhật ký thử các bậc.

**Connector.** Danh sách toolkit liên quan (Facebook, Instagram, LinkedIn, YouTube, X, Apify, Firecrawl, Exa, Gmail, Lịch): đã kết nối hay chưa, nút kết nối dùng lại luồng của `packages/core/composio`,
và "mở được những mẫu nguồn nào". Kết nối bậc 1 và 6 là việc của owner/admin.

**Web engine.** Một thẻ riêng trạng thái Crawl4AI (4.13): chưa cài, đang chạy, phiên bản, trang đang xử lý, bộ nhớ, lệnh chạy container khi chưa có. Nguồn đang chờ engine hiện rõ "cần Web engine".

**Ngân sách.** Trần chi phí nhà cung cấp mỗi tháng cho cả squad và cho từng nguồn; hiện mức đã dùng; chạm trần thì nguồn dừng và báo, không tự tăng.

**Hộp chia sẻ.** Danh sách `sales_share` với trạng thái, nút **Chia sẻ** (dán link hoặc nội dung, đính ảnh), kết quả mỗi lần (account nào được tạo hoặc vì sao bị bỏ).

**Máy chủ và CLI.**
- `POST /api/squads/{id}/sources` (đầu vào gốc, lane tùy chọn): tạo tài liệu `sales_source` trạng thái `proposed`, tạo issue giao cho Sales Scout theo route E, trả id. Giao diện theo dõi tiến độ qua issue
  và qua tài liệu. Danh sách và sửa dùng API KB (mục 7.1). `POST /api/squads/{id}/shares` làm tương tự cho chia sẻ (route F).
- `multica sales probe <đầu vào>`: nhận diện và đi các bậc 2 đến 5 bằng mã, in các recipe ứng viên và mẫu (JSON). `multica sales collect --source <id> [--dry-run] [--limit N]`: chạy recipe, ghi tín hiệu chuẩn hóa,
  cập nhật sức khỏe nguồn và chi phí; từ chối khi vượt trần chi phí.
- Mọi nguồn thêm qua giao diện hay qua issue đều đi cùng đường onboarding; ở P0 chưa có giao diện thì bạn thêm nguồn bằng một issue "Thêm nguồn: <link>" giao cho squad.

Quy tắc UI của repo áp dụng: chuỗi qua i18n đủ bảy ngôn ngữ, mô tả ngắn không lặp tiêu đề, zod và `parseWithFallback` cho phản hồi mới, khóa query có `wsId`, Zustand chỉ giữ bộ lọc.

## 8. Squad

### 8.1 Template và đội ngũ

`id: "sales_loop"`, tên squad `Sales Loop`, `floorName: null` (bạn gán vào room sau), `leader: "sales_lead"`. Năm agent, tiền tố `Sales` để không trùng
tên với agent của starter company (như đã làm ở spec BMAD). Tách agent chỉ khi cần **tách quyền**; chuyên môn nằm trong skill.

| Role key | Tên agent | Việc | Skill |
| --- | --- | --- | --- |
| `sales_lead` | Sales Lead | Hiệu chỉnh, chọn route, giao sub-issue, kiểm bằng chứng, ghi snapshot, báo cáo | `sales-delivery-core`, `sales-squad-lead` |
| `sales_scout` | Sales Scout | Chạy adapter, khử trùng, chấm điểm, ghi hồ sơ, onboarding và sửa recipe nguồn, đào bài được chia sẻ, ghi sức khỏe nguồn | `sales-delivery-core`, `sales-scouting`, `sales-source-builder` |
| `sales_writer` | Sales Writer | Thư mở lời, follow-up, kịch bản điện thoại/chat, bài nội dung | `sales-delivery-core`, `sales-writing` |
| `sales_analyst` | Sales Deal Analyst | Ghi chép họp thành opportunity, báo giá theo `sales_offer`, báo cáo thắng/thua, ghi bài học | `sales-delivery-core`, `sales-deal-analysis` |
| `sales_checker` | Sales Checker | Kiểm độc lập thư, báo giá, bài, tuân thủ; không viết gì | `sales-delivery-core`, `sales-review` |

Tên và vai trò trùng ý hình ảnh đầu vào ở chỗ nào là trùng, không phải mục tiêu. Số agent giảm từ chín (một node một lời nhắc) xuống năm vì ở Multica
một skill mang được chuyên môn; Scout và Writer vẫn tách để người tìm khách không phải người nhắn khách.

### 8.2 Skill (thư mục `packages/core/squads/sdlc-skills/sales-*`, sinh bằng `pnpm generate:sdlc-skills`)

| Skill | Nội dung |
| --- | --- |
| `sales-delivery-core` | Quy tắc chung: chế độ không giám sát, ID ổn định, khóa chống trùng, thang bằng chứng (mỗi khẳng định một nguồn có ngày), cách ghi giả định, bảng sở hữu tài liệu, định dạng báo cáo, luật tuân thủ, lệnh `multica knowledge` cho các kind `sales_*` |
| `sales-squad-lead` | Hiệu chỉnh ngày 0, chọn route (mục 9), trần theo lane, kiểm chứng, snapshot, báo cáo |
| `sales-scouting` | Danh mục adapter và cách chạy từng cái, chấm điểm (4.7), đào bài được chia sẻ (4.12), ghi sức khỏe nguồn, cách dùng công cụ Composio/MCP cho bậc 1 và 6 |
| `sales-source-builder` | Đường onboarding (4.11): nhận diện, đi thang tiếp cận, chấm mẫu, viết và sửa recipe (kể cả sinh schema bằng `generate_schema` khi có Web engine), khi nào dừng và báo `needs_connector`/`manual_only`/`blocked`; ranh giới cứng của 4.9; cấu hình hardening của Web engine (4.13) |
| `sales-writing` | Mẫu theo `goal` và kênh, giọng từ `sales_voice`, nội dung bắt buộc cho từng kênh, không bao giờ nêu giá ngoài `sales_offer` |
| `sales-deal-analysis` | Ghi chép họp thành opportunity, báo giá từ price book, phân tích thắng/thua, viết `sales_lesson` với độ tin cậy |
| `sales-review` | Danh sách kiểm theo kênh và theo luật (mục 10), verdict `pass/concerns/fail`, không sửa hộ |

Bộ thu thập là một lệnh CLI (`multica sales probe`, `multica sales collect`, chạy trên daemon) và skill chỉ gọi nó. Skill mang được `scripts/` (đã có ở `bmad-toolbox--bmad-deep-recon`), nhưng script trong skill phụ thuộc runtime của từng agent có Python hay không,
còn CLI chạy trên daemon nên đồng nhất, có kiểm soát chi phí và chặn IP nội bộ ở một chỗ.

### 8.3 Lịch và sự kiện

Cần đổi `SquadTemplate.routine` thành `routines: SquadRoutineTemplate[]` (`quickstart.ts` đã tạo từng autopilot theo tên; `company-template.ts` đã dùng dạng mảng).
Kết quả báo cáo `QuickstartOutcome.routine` thành danh sách.

| Routine | Cron | Việc |
| --- | --- | --- |
| Weekly scan | `0 8 * * 1` | Với mỗi lane `active`: chạy adapter, chấm điểm, ghi hồ sơ, soạn thư, nhờ Checker, ghi snapshot |
| Weekly review | `0 20 * * 0` | Báo cáo tuần, thắng/thua, bài học, nguồn chết; nội dung đề xuất |
| Source health | `0 7 * * *` | `multica sales collect --dry-run` từng nguồn bậc 2 đến 5, cập nhật `sales_source`, dựng lại recipe khi nguồn đổi cấu trúc (4.11), báo nguồn hỏng; rẻ vì chủ yếu là gọi HTTP |

Sự kiện sau họp: bạn tạo một issue giao cho squad ("Họp với X", kèm ghi chép; về sau là một quick action "Ghi chép họp" trong `packages/views/quick-actions`);
squad leader trigger sẵn có đánh thức Lead. Không cần server mới. Routine tạo ở trạng thái active cho Weekly scan sau khi hiệu chỉnh được xác nhận; trước đó
chỉ chạy chế độ khám phá (mục 5). Cron theo múi giờ mà quickstart truyền (sẵn có).

Sự kiện chia sẻ và thêm nguồn: một issue "Chia sẻ: <link>" hoặc "Thêm nguồn: <link>" (hoặc nút tương ứng trong Source Studio) đánh thức Lead, Lead chạy route F hoặc E.

### 8.4 Giới hạn chi phí và sức khỏe

- Trần mỗi lane mỗi tuần: số tín hiệu đọc, số account qualify (`weekly_cap`), số thư nháp (mặc định 20), số nguồn mới (mặc định 5).
- Adapter lấy dữ liệu bằng mã, LLM chỉ chạm các tín hiệu đã qua lọc từ khóa và khử trùng.
- Mỗi lần chạy ghi, cho từng nguồn: HTTP, số bản ghi, thời gian. Nguồn lỗi hai lần liên tiếp thành `degraded`; báo trong báo cáo tuần. Không có nguồn nào hỏng mà im lặng.
- Nhà cung cấp dữ liệu (bậc 6) có **ngân sách tháng** cho squad và cho từng nguồn, mặc định 0 USD cho đến khi owner hoặc admin đặt; `multica sales collect` từ chối chạy khi vượt trần và ghi lý do.
  Mỗi lần chạy ghi chi phí thật vào `cost_30d`; view "Sức khỏe nguồn" hiện chi phí trên mỗi account qualified để bạn biết nguồn nào đáng tiền.
- Nguồn ở bậc 6 chạy với số mục tối đa nhỏ khi mới kích hoạt (mặc định 50), tăng khi bạn xác nhận chất lượng.
- Web engine (4.13) tốn bộ nhớ nên có trần: số trang đồng thời, số trang mỗi nguồn mỗi lần chạy, timeout mỗi trang. Lịch Weekly scan và Source health xếp hàng các nguồn dùng engine, không chạy đồng loạt.
- Autonomy: squad cài ở L1 như các squad khác; `company_state` paused hoặc quiet thì không chạy.

### 8.5 Vòng học có phanh

`sales_lesson` ghi `confidence: low` và `evidence_count`. Chỉ nâng lên `medium` hoặc `high` khi bạn xác nhận hoặc đủ số mẫu tối thiểu (mặc định 5 kết quả
độc lập cùng chiều). Chỉ bài đã xác nhận mới đổi trọng số điểm, quy tắc lọc hoặc mẫu thư. Lý do: ở công ty nhỏ mỗi tuần chỉ vài deal, học từ giai thoại
làm hệ thống tự củng cố sai.

## 9. Quy trình theo route

Lead chọn route theo loại request (giống cách `sdlc-squad-lead` chọn route), giao bằng sub-issue với id thành viên lấy từ roster, xếp hàng, kiểm báo cáo.

| Route | Khi nào | Chuỗi |
| --- | --- | --- |
| **A. Hiệu chỉnh** | Lần đầu, thêm lane, đổi offer | Lead đọc KB → dựng offer/ICP/lane → Scout chọn adapter và thử gọi → proposal `sales_calibration` |
| **B. Quét theo lane** | Weekly scan | Scout: adapter → khử trùng → chấm điểm → hồ sơ → Writer: nháp (chỉ với `qualified`) → Checker → snapshot → "cần bạn" |
| **C. Sau họp** | Issue ghi chép họp | Analyst: opportunity → (Writer: follow-up ∥ Analyst: báo giá) → Checker → "cần bạn" |
| **D. Học tuần** | Weekly review | Analyst: thắng/thua, `sales_lesson` → Writer: nội dung đề xuất → Checker → báo cáo |
| **E. Onboard nguồn** | Người dùng thêm nguồn, lane mới (khám phá 4.6), nguồn chết | Scout: nhận diện → đi thang → chấm mẫu → recipe (4.11) → `active` (bậc 1 đến 5 tự bật) hoặc proposal kèm mẫu |
| **F. Chia sẻ và đào** | Người dùng chia sẻ bài, link, CSV | Scout: phân loại ý định → xác định doanh nghiệp → làm giàu → khử trùng → chấm điểm → account/opportunity → Writer: gợi ý trả lời hoặc thư → Checker (4.12) |

Bốn **cổng người** (đúng bốn chỗ): xác nhận hiệu chỉnh; duyệt thư và báo giá (`checked → approved`, rồi bạn tự gửi); duyệt bài; xác nhận bài học; kích hoạt nguồn cần quyền hoặc tốn phí (sau khi xem mẫu).
`sent` chỉ do người đặt (hoặc, ở P2, connector gửi đúng thư đã `approved`).

**Máy trạng thái outreach:** `draft` (Writer) → `checked` (Checker `pass`) → `approved` (người) → `sent` (người hoặc connector) → `replied`. Checker `concerns/fail`
trả về `draft` kèm lý do; thư không qua Checker không bao giờ tới "cần bạn".

**Kênh.** Với SME ở nhiều ngành, điện thoại và chat thường hiệu quả hơn email, nên `contact_channel` là thuộc tính hạng nhất. Writer soạn *kịch bản gọi* và
*tin nhắn chat* và *gợi ý trả lời công khai* (`public_reply`, dành cho bình luận trong nhóm hoặc trên Page; người tự đăng) cho người dùng, không gửi hộ. Mỗi kênh có
danh sách kiểm riêng ở mục 10. `contact_channel` thêm giá trị `public_reply`.

## 10. Tuân thủ

Đã đọc văn bản gốc, không phải bài tóm tắt, cho phần sau; việc áp dụng cho thư lạnh B2B là vùng xám và cần luật sư xem.

- **[Nghị định 91/2020/NĐ-CP, Điều 17 đến 20](https://congbaocdn.chinhphu.vn/CongBaoCP/VanBan/2020/8/31908/32282-1-2020837-83891-2020-nd-cp.pdf):** thư điện tử quảng cáo phải gắn nhãn `[QC]` hoặc `[AD]` ở
  vị trí đầu tiên của tiêu đề; có thông tin người quảng cáo (tên, điện thoại, email, địa chỉ, website, mạng xã hội nếu có) đặt liền trước phần từ chối; có phần từ
  chối ở cuối thư (qua trang web, bằng thư điện tử, qua điện thoại); gửi xác nhận một lần và ngừng ngay khi bị từ chối.
- **Điều 13:** tối đa 3 thư quảng cáo tới một địa chỉ trong 24 giờ; tin nhắn trong khung 07 giờ đến 22 giờ, gọi điện quảng cáo 08 giờ đến 17 giờ; không quảng cáo tới số trong
  danh sách không quảng cáo.
- **[Nghị định 13/2023/NĐ-CP, Điều 21](https://xaydungchinhsach.chinhphu.vn/toan-van-nghi-dinh-13-2023-nd-cp-bao-ve-du-lieu-ca-nhan-119230516104357809.htm):** dùng dữ liệu cá nhân cho tiếp thị cần sự đồng ý
  của chủ thể. Một nguồn thứ cấp cho biết [Luật Bảo vệ dữ liệu cá nhân 91/2025/QH15](https://baovedlcn.vn/bao-ve-du-lieu-ca-nhan-trong-kinh-doanh-dich-vu-tiep-thi-va-quang-cao/) kế thừa quy định này
  (chưa kiểm hiệu lực).

**Mạng xã hội: những điều đã kiểm và giới hạn của chúng.**
- **[Meta Platforms v. Bright Data](https://www.govinfo.gov/content/pkg/USCOURTS-cand-3_23-cv-00077/pdf/USCOURTS-cand-3_23-cv-00077-7.pdf)** (tòa liên bang Bắc California, 23/01/2024):
  điều khoản Facebook và Instagram không cấm việc thu thập dữ liệu **công khai khi không đăng nhập**. Đây là phán quyết về hợp đồng, của tòa Mỹ, về một bên cụ thể; nó **không** nói về
  dữ liệu cá nhân, về việc thu thập khi đã đăng nhập, hay về luật Việt Nam, và không phải giấy phép. Nó là lý do thang tiếp cận chỉ cho phép đọc khi chưa đăng nhập.
- **Nghị định 13/2023, Điều 17.2** miễn đồng ý khi việc công khai dữ liệu cá nhân là *theo quy định của luật*. Một bài đăng hiển thị công khai trên Facebook **không** tự động thuộc trường hợp này.
  **Điều 22.2** coi việc tổ chức thu thập, chuyển giao, mua bán dữ liệu cá nhân không có sự đồng ý của chủ thể là vi phạm. Quyền phản đối xử lý cho mục đích quảng cáo, tiếp thị phải được thực hiện trong
  72 giờ kể từ khi nhận yêu cầu (theo bản văn đã đọc; chưa đối chiếu số điều).
- Hệ quả thiết kế: (1) hồ sơ dựng cho **tổ chức**, không dựng hồ sơ cá nhân; (2) tác giả cá nhân chỉ được giữ liên kết và tóm tắt nhu cầu (4.12); (3) không lưu ảnh, danh sách bạn bè, hồ sơ cá nhân;
  (4) yêu cầu phản đối hoặc xóa thực hiện trong 72 giờ (`do_not_contact`, xóa nội dung hồ sơ); (5) `sales_share` hết hạn sau 90 ngày; (6) bậc 6 chỉ bật khi owner hoặc admin xác nhận, kèm cảnh báo này;
  (7) nhóm cấm quảng cáo: Checker đối chiếu quy định nhóm nếu người dùng cung cấp, và `public_reply` mặc định ở dạng trả lời hữu ích, không rao hàng.
- Mọi điều trên cần luật sư xem trước khi bật bậc 6 cho dữ liệu có người Việt Nam.

Thiết kế buộc chặt, không phụ thuộc cách hiểu pháp lý:
1. Checker `fail` nếu thư thiếu nhãn, khối người gửi, dòng từ chối, hoặc có địa chỉ không có `contact_source`. Cấu hình mặc định bật; nới là quyết định có ghi nhận của người.
2. `do_not_contact` là chặn cứng, quét trước mọi lần soạn; yêu cầu từ chối ghi ngay vào account.
3. Đếm số lần chạm mỗi địa chỉ, tôn trọng khung giờ theo kênh.
4. Không đoán email, không dò SMTP, không dùng nguồn đã `blocked`; tôn trọng robots, điều khoản (RemoteOK: ghi nguồn và link; Himalayas: không phân phối lại).
5. Liên hệ chỉ lấy từ trang do chính tổ chức công bố; mỗi liên hệ có `contact_source`.

## 11. Giá trị cho công ty và cách đo

- Pipeline đều mỗi tuần mà không ai phải nhớ đi tìm; áp dụng được cho từng ngành bạn làm.
- Họp xong có follow-up và báo giá nháp trong vài phút; báo giá chỉ lấy từ `sales_offer`.
- Hồ sơ khách, lý do thắng/thua, nguồn nào hiệu quả thành tài sản KB, tìm kiếm được.
- Một chỗ nhìn: pipeline, việc cần duyệt, việc đến hạn, nguồn nào chết.

Số theo dõi (mục tiêu đo, chưa có số thật): thời gian từ họp đến follow-up nháp; tỷ lệ phản hồi theo lane và family; tỷ lệ bạn phải sửa nháp (giảm
là vòng học hiệu quả); tỷ lệ account qualify được bạn giữ lại; báo giá lệch price book (phải bằng 0); số nguồn degraded/blocked.

## 12. Thay đổi theo thành phần và giai đoạn

| Thành phần | Thay đổi | Giai đoạn |
| --- | --- | --- |
| `packages/core/squads/templates.ts` | Thêm `sales_loop` vào `SquadTemplateId`, 5 role vào `SquadRoleKey`/`SQUAD_ROLES`, charter, instructions, `routines[]` thay `routine` | P0 |
| `packages/core/squads/quickstart.ts` | Tạo nhiều routine; `QuickstartOutcome.routine` thành danh sách; `instructionsFor` cho role sales | P0 |
| `packages/core/squads/sdlc-skills/sales-*`, `scripts/generate-sdlc-skills.mjs`, `templates.ts` | 7 skill; sinh lại `sdlc-skills-data.ts`; `SALES_SKILL_NAMES` tường minh; mở rộng bộ lọc của `SDLC_SKILL_NAMES` để SDLC squad không nhận skill sales; test dạng `planning-skills.test.ts` | P0 |
| `packages/core/squads/upgrade.ts`, `scripts/upgrade-sdlc-squad.mjs` | Cho phép nâng cấp squad sales (skill, instruction) | P0 |
| `server/internal/service/knowledge_kind.go` | Thêm kind `sales_*`, chính sách đăng, base `sales` vào seed (kiểm cách seed cho workspace đã có) | P0 |
| `packages/views/modals/squad-quickstart.tsx`, `locales/*/modals.json` | Thẻ chọn template, bảy ngôn ngữ | P0 |
| Danh mục adapter và bộ thu thập | Các adapter bậc 2 đến 5 của 4.4 và danh mục khởi đầu 4.5, chạy bằng `multica sales collect` (xem dòng dưới) | P0 |
| `server/migrations/` | `properties`, `fields` (P1); chỉ mục `CONCURRENTLY` mỗi cái một migration | P1 |
| `server/internal/service/knowledge_*.go`, `handler/knowledge_documents.go` | Kiểm tra theo `fields`, PATCH properties, lọc theo thuộc tính, lịch sử | P1 |
| `server/cmd/multica/cmd_knowledge.go` | `--field`, `update`, `list --field` | P1 |
| `packages/core/knowledge/` | Schema zod, query/mutation có `wsId`, kiểu `fields` | P1 |
| `packages/views/knowledge/` | Tab Collections (bảng, kanban, chi tiết), view lưu sẵn, phòng Sales ở Company | P1 |
| `packages/core/squads/sdlc-skills/sales-source-builder`, bộ chạy recipe | Skill onboarding; `multica sales probe` và `multica sales collect` (Go, `server/cmd/multica/`), recipe có schema và test với phản hồi ghi sẵn | P0 |
| Web engine Crawl4AI | Phát hiện engine trong `multica sales probe/collect`, `fetch.engine`, schema CSS/XPath, trần tài nguyên, chặn IP nội bộ, hardening 4.13; tài liệu chạy container (phiên bản ghim, token); MCP của Crawl4AI vào `mcp_config` của Sales Scout | P0 |
| Mẫu nguồn starter (mọi nguồn có adapter ở 4.5 và 16 dòng mẫu mạng xã hội ở 4.10) | Dữ liệu seed: tài liệu `sales_source` do quickstart tạo, có recipe khung | P0 |
| `server/migrations/`, `packages/core/types/squad.ts` | `squad.template_key`; quickstart đặt giá trị | P1 |
| `server/internal/handler/` | `POST /api/squads/{id}/sources`, `POST /api/squads/{id}/shares` (tạo tài liệu và issue cho Scout) | P1 |
| Thẻ Web engine trong Source Studio | Trạng thái engine (7.5), kiểm tra qua daemon | P1 |
| `packages/views/squads/components/` | Source Studio: danh sách nguồn, thêm nguồn, chi tiết và recipe, connector, ngân sách, hộp chia sẻ; chip ở phòng Sales | P1 |
| Ngân sách nhà cung cấp | Trần theo squad và theo nguồn, kiểm trong `multica sales collect`, hiển thị ở Studio | P1 (P0: trần ghi trong `sales_source`) |
| Connector Gmail/Calendar | Đẩy thư `approved` vào Gmail nháp, đọc phản hồi cập nhật `stage`, Lịch tạo issue họp | P2 |
| Chia sẻ nhanh | Bookmarklet hoặc tiện ích trình duyệt, chia sẻ từ app di động (chưa kiểm hỗ trợ) | P2 |
| Handoff khi `won` | Tạo request cho squad giao hàng (dùng handoff rule của spec BMAD) | P3 |

**Nền đã có (BMAD Planning đã vào `main`).** Squad BMAD Planning, workflow planning cho dự án của squad và handoff giữa squad đã được commit (`02fe41673`, `3d829d150`, `a954a7b6e`, `524a550a8`).
Sales Loop đi theo cùng đường: thêm template vào `templates.ts`, skill vào `sdlc-skills/` với tiền tố `sales-`, `SALES_SKILL_NAMES` tường minh và bộ lọc `SDLC_SKILL_NAMES` mở rộng
(hiện lọc `PLANNING_ONLY_SKILLS`). `SquadTemplate.routine` vẫn là một mục đơn (`routine: SquadRoutineTemplate | null`), nên `routines[]` còn là việc của P0. Handoff giữa squad ở P3 dùng lại
cơ chế đã có (`cmd_squad_handoff.go`, `squad_handoff`), không viết mới.

Quy tắc repo áp dụng: không khóa ngoại và không cascade; chỉ mục dùng `CONCURRENTLY` mỗi chỉ mục một file; response UI đi qua zod và `parseWithFallback`;
`make sqlc` sau khi sửa SQL; test cạnh mã; test Go dùng `server/internal/testutil` và khẳng định trên dòng fixture của mình, không trên tổng.

## 13. Kiểm chứng

- **Template và skill:** `templates.test.ts` (roster, tên không trùng starter company, `routines` hợp lệ), `quickstart.test.ts` (chạy lại không tạo trùng, nhiều routine),
  test nội dung skill (mỗi role nạp đúng skill; bảng sở hữu; luật tuân thủ; không báo giá ngoài `sales_offer`).
- **Adapter:** mỗi adapter có fixture ghi sẵn (phản hồi thật đã lưu) và test chuẩn hóa về dạng signal; test mặc định **không gọi mạng**.
  Một lệnh kiểm riêng (không nằm trong test mặc định) gọi thử từng nguồn và ghi bảng phụ lục A; chạy khi bạn cho phép.
- **Chống trùng và chặn cứng:** test với hai lần chạy cùng dữ liệu (không tạo trùng), `do_not_contact`, thiếu `contact_source`.
- **P1 (Go):** kiểm tra `fields` (kiểu, `select`, khóa lạ), PATCH, lọc, lịch sử, tương thích với tài liệu P0 có mục `Record`; response lỗi cho schema zod.
- **Diễn tập trên agent thật** (tốn quota, cần bạn cho phép), **ba ngành khác hẳn để chứng minh không gắn với IT**:
  - R1: bán phần mềm cho chuỗi F&B địa phương; kỳ vọng nguồn `expansion` và `directory`, kênh điện thoại/chat, không có kết quả tuyển dụng IT.
  - R2: bán máy cho nhà máy cơ khí; kỳ vọng khám phá ra danh bạ hiệp hội và danh sách đơn vị trưng bày của triển lãm ngành.
  - R3: dịch vụ tư vấn thầu; kỳ vọng nguồn `tender` và `regulatory`, hồ sơ không có kênh email lạnh.
  - R4: hiệu chỉnh với KB trống; kỳ vọng proposal xin thông tin, không có thư nháp.
  - R5: nguồn bị chặn giữa chừng; kỳ vọng `degraded`, báo trong báo cáo tuần, không hỏng im lặng.
  - R6: thêm một nguồn chưa từng thấy (một RSS ngành, một danh bạ hiệp hội HTML, một kênh Telegram); kỳ vọng recipe đạt bước chấm mẫu và người dùng thấy 10 tín hiệu trước khi kích hoạt.
  - R7: thêm link một Page Facebook công khai khi chưa kết nối gì; kỳ vọng `needs_connector` kèm đúng một câu "kết nối Apify, khoảng X USD, mở được Y", không có dữ liệu bịa.
  - R8: chia sẻ một bài từ nhóm kín (dán nội dung); kỳ vọng phân loại ý định, xác định doanh nghiệp từ nguồn công khai, tạo account hoặc chỉ lưu tóm tắt nếu tác giả là cá nhân, và
    không có hồ sơ cá nhân.
  - R9: chạm trần chi phí nhà cung cấp; kỳ vọng nguồn dừng, ghi lý do, không tự tăng trần.
  - R10: thêm một danh bạ hiệp hội dựng bằng JS khi chưa có Web engine; kỳ vọng `needs_connector` với câu "cần Web engine"; sau khi chạy container, cùng nguồn đạt bước chấm mẫu.
  - R11: engine được yêu cầu đọc một URL tới địa chỉ nội bộ (ví dụ `http://127.0.0.1` hoặc địa chỉ metadata); kỳ vọng bị chặn và ghi lý do.
- **Spike Crawl4AI (đầu P0, nửa ngày, cần bạn cho phép vì cài đặt nặng và gọi trang thật):** trên máy có mạng ổn hoặc bằng Docker image, đo trên ITviec, CareerViet, TopCV, VietnamWorks, voz, ba
  danh bạ hiệp hội, hai danh sách đơn vị trưng bày hội chợ: tỷ lệ đọc được so với `curl`, bộ nhớ, thời gian mỗi trang, độ phủ trường của schema do `generate_schema` sinh. Ghi kết quả vào phụ lục A và
  chốt việc Web engine có là khuyến nghị mặc định hay chỉ là tùy chọn.
- **Recipe và adapter:** schema của recipe có test (hợp lệ, thiếu trường, phiên bản lạ); `multica sales probe` và `multica sales collect` có fixture ghi sẵn cho từng bậc 2 đến 5; test mặc định không gọi mạng và
  không gọi nhà cung cấp thật. Kiểm các mẫu Apify và Composio chỉ khi bạn cho phép, vì tốn tiền.
  - Số đo: thời gian từ họp đến follow-up nháp; số lần bạn phải can thiệp ngoài bốn cổng.

## 14. Rủi ro

| Rủi ro | Giảm thiểu |
| --- | --- |
| Gửi thư sai luật hoặc làm phiền | Draft-only; Checker `fail` khi thiếu nhãn/khối người gửi/từ chối; `do_not_contact`; luật sư xem |
| Hồ sơ bịa hoặc báo giá bịa | Mỗi khẳng định có nguồn và ngày; báo giá chỉ từ `sales_offer`; Checker đối chiếu |
| Lệ thuộc nguồn dễ chết | Sức khỏe nguồn hằng ngày; `degraded` và `blocked`; không im lặng; nhiều family cho một lane |
| Điều khoản nguồn cấm dùng | Cột `terms` trên `sales_source`; nguồn `restricted` hoặc `unknown` không tự bật; ghi nguồn khi bắt buộc |
| Bài học sai làm hệ thống tự củng cố | `confidence` và mẫu tối thiểu; chỉ bài đã xác nhận đổi hành vi |
| Chi phí chạy phình | Trần theo lane; bộ thu thập bằng mã; LLM chỉ chạm tín hiệu đã lọc |
| Tập khách rác làm nhiễu tìm kiếm KB | Chỉ account `qualified` thành tài liệu riêng; base `sales` tách khỏi `market`; ứng viên bị loại ở báo cáo quét |
| P1 lớn hơn dự kiến | P0 độc lập và dùng được; thuộc tính tương thích ngược qua mục `Record` |
| Hiệu chỉnh đoán sai ngành | Chế độ khám phá trước khi xác nhận; proposal ba câu; chạy lại khi đổi offer |
| Dữ liệu cá nhân | Chỉ liên hệ công khai do tổ chức công bố, kèm nguồn; không ghi thông tin không cần |
| Nền tảng đóng thêm hoặc đổi API, mẫu nguồn xã hội chết | Nguồn xã hội luôn có bậc dự phòng 7 (người chia sẻ); sức khỏe hằng ngày; recipe có phiên bản và tự dựng lại (4.11); mẫu chỉ là khung, không phải cam kết |
| Nhà cung cấp tính phí vượt dự tính | Ngân sách tháng mặc định 0, trần theo nguồn, số mục nhỏ khi mới bật, chi phí trên mỗi account qualified hiển thị, dừng khi chạm trần |
| Điều khoản nền tảng và luật dữ liệu cá nhân (bậc 6, `human_share`) | Chỉ đọc khi chưa đăng nhập, không hồ sơ cá nhân, xóa trong 72 giờ, hết hạn chia sẻ, owner/admin xác nhận, luật sư xem (mục 10) |
| Tài khoản của người dùng bị khóa vì tự động hóa | Không bao giờ dùng tài khoản hoặc cookie của người dùng để đọc; chỉ bậc 1 qua OAuth chính thức |
| Recipe do LLM viết sai (selector, ánh xạ) mà không ai biết | Bước chấm mẫu bắt buộc, người xem mẫu trước khi kích hoạt, `field_coverage` ghi nhận, nguồn tụt chất lượng chuyển `degraded` |
| Web engine bị lợi dụng để đọc dịch vụ nội bộ (SSRF) vì nhận URL từ nguồn người dùng thêm | Chỉ nghe loopback, token, chặn dải IP nội bộ và địa chỉ metadata, crawl sâu tắt, test R11 |
| Web engine tốn bộ nhớ, làm chậm máy chạy daemon | Trần số trang đồng thời và mỗi nguồn, timeout, xếp hàng nguồn dùng engine, thẻ trạng thái ở Studio |
| Phụ thuộc một dự án chủ yếu một người bảo trì | Engine tùy chọn; ghim phiên bản; test hợp đồng với fixture; nguồn rơi về `needs_connector` thay vì chạy dở; recipe không khóa vào engine nhờ `fetch.engine` |
| Bật stealth hoặc hồ sơ đăng nhập của Crawl4AI làm vượt ranh giới cứng | Tắt mặc định, chỉ bật theo từng nguồn với xác nhận owner/admin, không cho nền tảng có điều khoản cấm, log quản trị |
| Người dùng kỳ vọng "đào được mọi nguồn" | Cam kết rõ ở 4.11: mọi nguồn có một trạng thái có lý do và có bước tiếp; không hứa vượt đăng nhập hay chống bot |

## 15. Quyết định

**Đã chốt (người dùng đồng ý trong trao đổi):** dùng KB thay Google Sheets; squad 5 agent; đầu tư cơ chế thuộc tính và view trong KB (P1) nhưng ship P0 trước;
bỏ nhóm nguồn bị chặn (TopCV, VietnamWorks, TopDev, LinkedIn) ở bản đầu; mặc định tuân thủ cứng; khu vực mặc định Việt Nam cộng nguồn toàn cầu.

**Yêu cầu mới của người dùng (đã đưa vào thiết kế):** nguồn có sẵn và chỉnh được; nguồn có sẵn gồm cả mạng xã hội; tính năng riêng cho loại squad này để thêm nguồn và agent phải đào được
(mục 4.9 đến 4.13, 7.5). Phạm vi: Source Studio ở P1, onboarding qua issue ở P0. Người dùng cũng yêu cầu đưa Crawl4AI vào spec: nó là Web engine **tùy chọn** (4.13), không thay Apify hay kết nối chính chủ cho mạng xã hội.

**Mở, xác nhận trước khi viết mã:**
1. Đã chọn: bộ thu thập là lệnh CLI, không phải script trong skill (8.2). Còn mở: hình dạng chính xác của `multica sales probe` và `multica sales collect` (cờ, định dạng ra).
2. Cách tạo base `sales` và seed kind mới cho workspace đã có.
3. Điều khoản gốc của Remotive và Himalayas; hiệu lực Luật 91/2025/QH15.
4. Trích xuất dữ liệu từ cổng mua sắm công Việt Nam; adapter cho danh bạ hiệp hội và danh sách trưng bày (chỉ mới đọc được bằng tay).
5. Runtime nào có `web_search`; nếu không, vòng khám phá chỉ chạy được một phần.
6. GDELT và TED: giới hạn tốc độ thật khi gọi có kiểm soát.
7. Kết nối Composio thuộc về người kết nối hay về workspace, và agent chạy theo lịch có dùng được không; toolkit nào của Composio workspace này bật được (Apify, Facebook, Instagram, LinkedIn, YouTube, X).
8. Bộ chạy recipe nằm ở đâu: lệnh CLI `multica sales collect` trên daemon (đề xuất), hay dịch vụ phía server. Chọn CLI vì mạng và IP của daemon là của người dùng, và vì bộ chạy có thể
   dùng cùng cơ chế xác thực của agent.
9. Các mẫu Apify cụ thể (actor nào, giá hiện tại, điều khoản) và việc workspace có token Apify chưa.
10. App di động có nhận chia sẻ (share target) không; có đáng làm tiện ích trình duyệt ở P2 không.
11. Facebook Page công khai: có xin Page Public Content Access (App Review, xác minh doanh nghiệp, nhiều tuần) để có đường hợp pháp hơn bậc 6 không.
12. Zalo: cần OA đã xác thực và gói dịch vụ cho tính năng nào; chưa đọc hết bảng gói.
13. Luật sư xem mục 10 trước khi bật bậc 6 cho dữ liệu có người Việt Nam.
14. YouTube RSS và Google Maps chưa xác nhận bằng gọi thử.
15. Crawl4AI (4.13): kết quả spike đo trên nguồn thật; cờ tôn trọng robots.txt trong bản đang dùng; cách chặn IP nội bộ (cấu hình của engine hay lớp chặn ở `multica sales collect`);
    cài đặt trên máy daemon của người dùng có Docker hay không, và nếu không thì dùng Crawl4AI Cloud hay bỏ bậc 5.
16. Có bật Crawl4AI MCP cho Sales Scout theo mặc định hay chỉ khi người dùng chạy engine (đề xuất: chỉ khi engine đang chạy).
17. Phiên bản Crawl4AI cần ghim (hiện 0.9.5, ngày 14/10/2026) và chính sách nâng cấp.

## Phụ lục A. Nhật ký gọi thử 2026-11-10

Gọi từ máy phát triển, User-Agent trình duyệt chung; "000" là không kết nối được. Phản hồi đã theo chuyển hướng ở các dòng ghi "sau chuyển hướng".

| Nguồn | HTTP |
| --- | --- |
| hn.algolia.com (who is hiring) | 200 |
| arbeitnow.com/api/job-board-api | 200 |
| remoteok.com/api | 200 |
| remotive.com/api/remote-jobs | 200 |
| himalayas.app/jobs/api | 200 |
| weworkremotely.com RSS | 200 |
| news.google.com/rss/search (vi, VN) | 200 |
| vnexpress.net RSS kinh doanh; startup (546 B) | 200 |
| cafef.vn, vietnambiz.vn, vietnamnet.vn, dantri.com.vn, tuoitre.vn, thanhnien.vn, vietstock.vn RSS | 200 |
| techinasia.com/feed | 200 |
| e27.co/feed | 403 |
| api.gdeltproject.org | 429 |
| trends.google.com/trending/rss?geo=VN | 200 |
| api.github.com/search/users | 200 |
| dangkykinhdoanh.gov.vn | 302, 200 sau chuyển hướng |
| muasamcong.mpi.gov.vn | 302, 200 sau chuyển hướng (`muasamcong.mof.gov.vn`) |
| masothue.com | 403 |
| infodoanhnghiep.com | 200 |
| hosocongty.vn, doanhnghiepmoi.vn, thongtindoanhnghiep.co, fia.mpi.gov.vn | 000 |
| topcv.vn | 403 |
| itviec.com/it-jobs | 200 |
| vietnamworks.com/viec-lam-it | 301, 404 sau chuyển hướng |
| topdev.vn, linkedin.com/jobs | 000 |
| careerviet.vn | 200 |
| producthunt.com/feed | 200 |
| ycombinator.com/companies | 200 |
| search.worldbank.org/api/v2/procnotices | 200 |
| contractsfinder.service.gov.uk (OCDS) | 200 |
| ted.europa.eu | 429 |
| sec.gov (EDGAR) | 403 |
| itunes.apple.com search; customer reviews RSS | 200; 200 |
| query.wikidata.org (câu truy vấn hợp lệ) | 200 |
| nominatim.openstreetmap.org | 200 |
| overpass-api.de; overpass.kumi.systems; overpass.openstreetmap.fr | 000; 000; 403 |
| reddit.com JSON | 403 |
| api.stackexchange.com | 200 |
| crt.sh | 000 lần đầu, 200 khi thử lại |
| lu.ma/discover; meetup.com/find | 200; 200 (HTML dựng bằng JS) |
| congthuong.vn RSS | 404 |
| thuvienphapluat.vn/page/tintuc.aspx | 403 |
| importyeti.com | 403 |
| api.opencorporates.com | 401 |
| find-and-update.company-information.service.gov.uk | 200 |

**Mạng xã hội và cộng đồng (gọi thử cùng ngày, không đăng nhập):**

| Nguồn | HTTP | Ghi chú |
| --- | --- | --- |
| t.me/s/<kênh công khai> | 200 | Đọc được 20 tin |
| Reddit `/r/<sub>/new/.rss` | 200 | Không có điểm số, số bình luận |
| voz.vn `/forums/-/index.rss` | 200 | |
| tinhte.vn `/forums/-/index.rss` | 403 | |
| webtretho.com | 000 | |
| facebook.com/<Page công khai> | 200 | Trang vỏ 506 KB; chỉ có tên và số theo dõi ở `og:description` |
| facebook.com/ads/library | 403 | Gọi HTTP thẳng |
| instagram.com/<hồ sơ> | 200 | Trang vỏ 700 KB |
| threads.net/<hồ sơ> | 301 | |
| linkedin.com/company/<tên> | 403 | |
| zalo.me | 200 | Trang giới thiệu, không phải dữ liệu |
| publish.twitter.com/oembed | 200 | Một bài |
| pinterest.com/<tên>/feed.rss | 200 | |
| public.api.bsky.app searchPosts | 403 | |
| youtube.com/feeds/videos.xml?channel_id=... | 404 | Mã kênh thử có thể sai; chưa xác nhận |
| tiktok.com/oembed | 000 | |
| google.com/search, bing.com/search?format=rss | 200 | Đọc được nhưng không dùng (điều khoản) |

Tài liệu nền tảng đã đọc: [Meta Page Public Content Access](https://developers.facebook.com/docs/features-reference/page-public-content-access/) (cần App Review và xác minh doanh nghiệp),
[Meta Content Library](https://transparency.meta.com/researchtools/meta-content-library) (dành cho tổ chức nghiên cứu phi lợi nhuận), bài tổng hợp về việc Meta gỡ Groups API ngày 22/04/2024,
[Zalo OA API](https://docs.zaloplatforms.com/docs/OA/bat-dau/gioi-thieu-official-account-api-va-cac-nhom-quyen) (dữ liệu của chính OA: tin nhắn, người quan tâm, nhãn, form Zalo Ads),
[YouTube Data API quota](https://developers.google.com/youtube/v3/determine_quota_cost), [TikTok Research API](https://developers.tiktok.com/products/research-api) (chỉ nhà nghiên cứu),
[Apify Facebook scrapers](https://apify.com/apify/facebook-posts-scraper) (chỉ nội dung công khai, không đăng nhập), [danh mục toolkit Composio](https://docs.composio.dev/toolkits).
Giá và hạn mức lấy từ các trang này và từ bài tổng hợp 2026; có thể đổi.

Crawl4AI: [kho mã](https://github.com/unclecode/crawl4ai) (Apache-2.0, bản 0.9.5 ngày 14/10/2026), [hướng dẫn tự chạy](https://docs.crawl4ai.com/core/self-hosting/) (token bắt buộc từ 0.9.0, MCP qua SSE/WebSocket,
crawl sâu tắt mặc định), cùng các so sánh độc lập 2026 với Firecrawl và Apify ([PkgPulse](https://www.pkgpulse.com/guides/crawl4ai-vs-firecrawl-vs-apify-ai-web-scraping-2026),
[String](https://usestring.ai/comparisons/firecrawl-alternatives), [serp.fast](https://serp.fast/guides/best-web-scraping-api-for-ai-agents)). Lần cài thử `crawl4ai` trong phiên nghiên cứu (pip rồi uv, môi trường cô lập) bị kẹt khi tải
Playwright, patchright và scipy trên máy phát triển, đã dừng và dọn; **không có số đo thành công trực tiếp**.

Dữ liệu mẫu đã xem: Arbeitnow 300 job (Anh, Thụy Sĩ, Pháp, Đức, Hà Lan là các địa điểm nhiều nhất); RemoteOK 100 job và điều khoản ghi nguồn trong chính phản hồi;
Himalayas 119.590 job với phân trang cursor; HN thread "Who is hiring" tháng 11/2026 có 323 comment; ITviec có JSON-LD; robots.txt của ITviec và CareerViet cho phép.

Nguồn tìm bằng web, chưa có adapter: danh bạ HASI, HAMI, VASI, VMAA, VAFIE, AmCham Vietnam; danh sách đơn vị trưng bày MTA Vietnam; dữ liệu khu công nghiệp của Vietdata (trả phí).

## Phụ lục B. Ví dụ ba lane khác ngành trong cùng một workspace

| | Lane 1: phần mềm cho chuỗi F&B | Lane 2: máy cho nhà máy cơ khí | Lane 3: tư vấn thầu |
| --- | --- | --- | --- |
| `goal` | `direct_sale` | `direct_sale` và `channel` | `tender` |
| Family chính | expansion, hiring (quản lý cửa hàng), pain, directory | event, directory, expansion, trade_flow | tender, regulatory |
| Nguồn khởi đầu | Google News RSS (khai trương, mở chi nhánh), CareerViet, đánh giá ứng dụng giao đồ ăn | Google News RSS, danh bạ hiệp hội, đơn vị trưng bày triển lãm (qua khám phá) | Cổng mua sắm công, World Bank, Google News RSS (quy định mới) |
| Kênh chính | điện thoại, chat | email, sự kiện | liên hệ bên mời thầu |
| Trọng số đặc thù | `timing` cao (khai trương mới) | `proof` cao (case nhà máy tương tự) | `signal` cao (gói thầu đúng chuyên môn) |

Đây là minh họa; hiệu chỉnh ngày 0 sinh ra lane thật từ KB và xác nhận của bạn.
