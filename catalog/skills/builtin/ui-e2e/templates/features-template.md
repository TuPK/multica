# <Product> feature inventory

Source: <spec or PRD path, if any>, <code roots scanned>, <date>. Status: `built` = found in code; `not built` = in the spec but absent from code; `disagreement` = spec and code differ.

| ID | Feature | Rule or behaviour | Route / screen / job | Code file | Status |
| --- | --- | --- | --- | --- | --- |
| AUTH-01 | Email code send | Sends a 6-digit code; per-email cooldown; unknown email gets the same 204 | POST /auth/send-code | server/handler/auth.go | built |

## Verification

- [ ] Every "Code file" exists.
- [ ] Every route, label or error code string in "Rule" or "Route" is found in that file (grep).
- [ ] Each `not built` row is confirmed absent (grep the router and the UI).
- [ ] Each `disagreement` row has the spec's text and the code's text.
