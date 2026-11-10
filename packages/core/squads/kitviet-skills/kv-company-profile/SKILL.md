---
name: kv--company-profile
description: "Use when a Kit Viet squad needs the company profile (Hồ sơ đơn vị): gather the facts that every document depends on, fill what the request gives, mark the rest, and file a draft of the profile in the knowledge base for a person to publish. Never stores identity or account numbers."
---

# Kit Viet company profile

The profile replaces a `CLAUDE.md` file. It is one knowledge document with the title `Hồ sơ đơn vị`. Every squad's lead reads it at the start of a request, and every drafter follows its wording, numbering and signatory rules. You write the profile once and the person who owns the company publishes it. Load kv--delivery-core first.

## What the profile holds

Under 60 lines. Plain facts and rules, no story.

- **Đơn vị**: legal name, short name, type of entity, address of the head office (the place name you use in documents).
- **Vai trò và người nhận**: who the person who works with the squads is, and who reads the output.
- **Xưng hô và giọng văn**: how documents address customers, partners and staff; the tone to keep.
- **Từ không dùng**: words and claims the company never uses, one per line.
- **Quy tắc số và ngày**: the date format, the number format for documents, and the rule for issuing document numbers.
- **Phần mềm hằng ngày**: the accounting and sales tools the company uses, with the export format of each.
- **Điều AI hay sai với đơn vị này**: the mistakes a reviewer has already caught, each as one line.
- **Phần riêng của squad** (one section per squad that uses it; the sections are below).

Never record an ID card number, a bank account number, a password, a token or any secret. If the request contains one, leave it out of the profile and name the field in your report as `[không lưu: nhạy cảm]`.

## Per-squad sections

Put each section under its own heading so a squad reads only its own part.

- **Office Admin**: the agency's name and its abbreviation as used on documents, the place name, the document numbering rule, the signatory with the position and the authority the signatory holds for each kind of document. Do not issue a document number that the signatory has not confirmed.
- **Accounting & Tax**: the accounting regime the company applies. Offer exactly three choices and let the person confirm one: the general regime, the regime for small and medium enterprises, and the regime for micro enterprises. Never pick one by default. The register of the squad holds the rules for each choice, so the profile only names the choice. Also record the fiscal-year end and the tax filing frequency the person states.
- **Legal & Contracts**: the company's signatory for contracts and its authority limit for each kind of contract. Do not record a contract value.
- **B2B Proposal Desk**: the discount frame the person has approved, with the maximum for each product line and who approved it. A frame with no approver is recorded as `[cần bổ sung: người duyệt khung]`.
- **Marketing Studio**: the brand voice in short form. The longer brand playbook is a separate document (kind `playbook`), not part of this profile.

## How to gather the facts

1. Read the request and its attachments first. Many facts are already there (a company registration extract, an existing brand document, an accounting export header). Use them, and cite the attachment name next to each fact.
2. Search the knowledge base for a current profile or a draft: `multica knowledge search "Hồ sơ đơn vị"`. If one exists, you update it as a new draft: you do not edit the published document.
3. For each field still empty, decide by the rule of kv--delivery-core: a field that cannot be defaulted (the signatory, the accounting regime, the discount approver) is `[cần bổ sung: <field>]` and a stop 1 only if the squad cannot run without it. Every other empty field is written as `[cần bổ sung: <field>]` and does not stop the run; the squad works from the skill's defaults.
4. Never invent a fact to fill a field: no company name, address, number, signatory or date that is not in the request, the attachments or a source you opened.

## Filing the draft

File the draft with `multica knowledge add --kind reference --title "Hồ sơ đơn vị" --file <file> --confidence <low|medium|high> --valid-until <YYYY-MM-DD>`. Use `low` when any field is still `[cần bổ sung]`, `medium` when only minor fields are missing, and `high` when every field is sourced. The `--valid-until` date is one year after today's date from the request. You never publish. The company owner or an admin publishes it with `multica knowledge publish <id>`.

## Document template (Vietnamese)

The filed profile is a `reference` document. The server refuses a `reference` filing without a `## Summary` heading, so the file starts with it. The same headings are your report to the lead.

```
## Summary
Hồ sơ đơn vị · bản nháp <dd/mm/yyyy>
Độ tin cậy: <low | medium | high>. Đã điền: <count> trường; còn [cần bổ sung]: <count>.

## Cần bạn xác nhận trước khi đăng
- <each field marked [cần bổ sung] or chosen from a set of options, one line each>

## Không lưu
<fields left out as sensitive, or "không có">

## Nguồn
<attachment names and knowledge ids used>

## Chưa xác minh
<facts you could not check against a source>
```

The profile's own sections (Đơn vị, Vai trò và người nhận, and the rest) follow the Summary, in the order of "What the profile holds".

## Limits

- The profile holds no legal rate, threshold or date from the law: the squad's register holds those, and the profile only refers to them.
- The profile never decides the accounting regime, the tax rate or the signatory's authority. It records what the person confirms.
- A profile that contains an ID card number, a bank account number, a password or a token is not filed as it is. Remove that field, file the rest, and report the removal.
