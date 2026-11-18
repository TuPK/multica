# SePay e-invoice fixtures

Read on 2026-11-18 from developer.sepay.vn (Vietnamese pages unless noted). Samples are
abbreviated and use placeholders. No API key, token, real buyer or real invoice data.

| Fixture | Source page |
| --- | --- |
| token_response.json, error_envelope.json | https://developer.sepay.vn/vi/einvoice-api/v1/tao-token ; .../xu-ly-loi |
| provider_account_detail.json | https://developer.sepay.vn/vi/einvoice-api/v1/chi-tiet-tai-khoan |
| create_request.json, create_response.json | https://developer.sepay.vn/vi/einvoice-api/v1/xuat-hoa-don-dien-tu |
| create_check_*.json | https://developer.sepay.vn/vi/einvoice-api/v1/theo-doi-trang-thai-xuat-hoa-don ; .../xu-ly-loi (retry block) |
| issue_request.json, issue_response.json | https://developer.sepay.vn/en/einvoice-api/v1/phat-hanh-hoa-don-dien-tu |
| issue_check_*.json | https://developer.sepay.vn/vi/einvoice-api/v1/theo-doi-trang-thai-phat-hanh-hoa-don |
| invoice_detail.json | https://developer.sepay.vn/vi/einvoice-api/v1/chi-tiet-hoa-don |
| download_response.json | https://developer.sepay.vn/vi/einvoice-api/v1/tai-hoa-don |

Hosts: production https://einvoice-api.sepay.vn ; sandbox https://einvoice-api-sandbox.sepay.vn
(overview and sandbox pages).

Not confirmed by these pages:
- Child fields of `buyer` and `items[]` are collapsed on the create page (read 2026-11-18). The
  required buyer list is therefore a provisional assumption (see spec 7.3).
- `create_check_failed.json` and `issue_check_failed.json` follow the documented `Failed` status;
  the exact `message` text is not in the samples.
- `issue_check_*` status responses carry a `retry` block only while `Pending` after a temporary error.
