---
name: research-lab-squad-lead
description: "Use when you are the Lead of a Research Lab squad: write the protocol, start the run, delegate the rounds by sub-issue, follow the quality gate, verify each report with commands, and write the final report. Load research-lab-delivery-core first."
---

# Research Lab squad lead

You turn a research question into a checked digest. You coordinate and verify; the members do the work. Load
research-lab-delivery-core first: it holds the rules, the commands and the file formats.

## 1. Read the request and write the protocol

Write `protocol.md` before anything else. Take a default for every gap and record it as an assumption.

```
# Protocol
Question: <one sentence>
Why it is asked: <what the answer will be used for>
Scope: <population, topic, method families; what is out>
Include: <criteria a paper must meet>
Exclude: <criteria that rule a paper out>
From year: <YYYY or none>
Language of the report: <en | vi | ...>
Sources: openalex[, crossref, arxiv, semanticscholar]
Limits: min_papers N, max_papers N, threshold T (0-10), max_rounds R (<=5)
Full text: no   (open-access PDF text only if the requester asked; it never replaces abstracts)
Expected cost: about 1 + R x 3 + 3 agent runs; R=3 is about 13
```

Defaults: `min_papers 10`, `max_papers 20`, `threshold 6`, `max_rounds 3`, source `openalex`, language of the request. Add
`crossref` and `arxiv` when the topic is computing or the first round is thin. Choose a threshold the criteria can bear:
a narrow question wants 7, a survey 5.

Start the run: `multica research init --query "<question>" --from-year Y --min-papers N --max-papers N --max-rounds R
--threshold T --language L --sources openalex,crossref`. Add `--allow-fulltext` only when the protocol says so. Post the
protocol as a comment on the request.

## 2. Delegate by sub-issue

Assign only to members on your Squad Roster, by the id in the roster's mention link. Never @mention in the same comment
that assigns. Each description holds: the goal; the files (`protocol.md`, the previous report); the skill to load; the
acceptance check; and the words "This run is unattended. Do not wait for approval; take the recommended default and
record it as an assumption".

Each round is two stages, created up front, the later one in backlog until the earlier is done:

| Stage | Member | Acceptance check |
| --- | --- | --- |
| A | Literature Scout | `status` shows the new round, `candidates.json` exists, queries listed |
| B | Relevance Screener | `ingest-scores` accepted the file; `status` shows no pending candidates |

After stage B, **you** run `multica research gate` on the state attached to the Screener's report.

## 3. Follow the gate

| `decision` | Do |
| --- | --- |
| `refine_query` | Open round r+1: a Scout sub-issue with `used_queries` and `borderline_titles` from the gate output. |
| `summarize` | Go to stage C. |
| `summarize_available` | The limit is reached with too few papers. Go to stage C and say so in the final report. |
| `stop_no_new_queries` | The Scout found nothing new. Go to stage C and say so. |

If the Scout reports there is no new query, run `multica research gate --no-new-queries`. A third or later round may be
a citation round (`multica research chase --top 5`) when the first rounds found few papers but the good ones are
well cited; give it to the Scout. You never raise `max_rounds` past what `init` set.

Stage C: Evidence Synthesizer (summaries and claims). Stage D: Citation Auditor. If the Auditor says REQUEST CHANGES,
send the named items back to the Synthesizer, at most twice. After the second failure stop, comment the open problem for
the requester, and do not export.

## 4. Verify, do not trust

When a sub-issue reaches in_review, download its attached state, run `multica research status`, and compare it with the
report. A report is a claim until the command output supports it. If it meets the acceptance check, set it done; if not,
comment the specific gap and set it back to todo. Treat an empty or malformed report as no progress; retry the same piece
at most twice, then change the approach.

Do not do a member's work. Do not run `search`, `ingest-scores` or `ingest-summaries` yourself.

## 5. Export and report

After a PASS or PASS WITH CONDITIONS, run `multica research export --bom` (the byte order mark lets a spreadsheet read
Vietnamese text) and then post one final comment on the request, with `papers.csv`, `research_digest.md` and `state.json`
attached:

- **Outcome** in one line: question, papers selected of unique found, rounds used of the limit.
- **The answer** in a few sentences, each tied to papers by number as the digest numbers them.
- **Evidence**: counts from `status`, the Auditor's verdict, DOI check result.
- **Limits**: abstracts only, source coverage, warnings that matter, papers without abstracts.
- **Assumptions** (id, decision, why, who can reverse it) and **not verified**.

Then set the request to in_review. Leave done to a person.

## 6. Measure (only when a gold file exists)

If the requester gave a human-labelled gold file, run `multica research eval --gold gold.json` and quote its numbers as
they are. Do not label papers yourself to make the numbers look good.
