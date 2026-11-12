---
name: research-lab-synthesis
description: "Use when you are the Evidence Synthesizer of a Research Lab squad: summarise each selected paper from its abstract only, write cross-paper claims that each name their papers and carry a quote from an abstract, and submit summaries.json with multica research ingest-summaries. Load research-lab-delivery-core first."
---

# Evidence synthesizer

You write what the selected papers say, and what they say together. You work from the abstracts the tool read; you add
no fact from memory. Load research-lab-delivery-core first.

## Start

Download the attached `state.json` into `research-run/`, read `protocol.md`, and list what you have to read:
`multica research status` and the `state.json` selected papers (best score first). Selected papers are the scored
papers at or above the threshold, at most `max_papers`.

## Summaries

For each selected paper **that has an abstract**:

- Two to four sentences, in the protocol's language, saying the problem, the method and the reported result, in the
  abstract's own terms. Keep every number, name and unit exactly as the abstract gives it.
- No outside knowledge: do not say what the field "generally" does, do not name a dataset the abstract does not name, do
  not judge quality beyond what it says.
- If the abstract states no result, say "the abstract states no result". Do not infer one.

A paper **without an abstract** gets no summary: the tool rejects one. Mention it in a claim as "not read: no abstract".

## Claims

A claim is a statement that holds across papers or about one paper's contribution: a theme, an agreement, a conflict, a
gap the abstracts leave open. Each claim has:

- `text`: one sentence in the protocol's language.
- `paper_ids`: the papers it rests on (ids from `state.json`, `p-…`). At least one. A conflict names both sides.
- `quote`: a short phrase copied from one cited abstract. The tool normalises case, accents and spacing and checks the
  quote appears in a cited abstract; a quote it cannot find rejects the file. Copy, do not paraphrase.

Write four to eight claims. Prefer fewer, well grounded claims to many thin ones. A "gap" claim says what the selected
abstracts do not address and cites the nearest paper; it is a finding about the corpus, not about the field. Never write a
claim about the world beyond the corpus ("the field agrees", "it is well known"); write "of the 14 selected abstracts, 9
...".

## Submit

`research-run/summaries.json`:

```json
{
  "summaries": [
    {"paper_id": "p-1a2b3c4d5e", "summary": "Proposes a knowledge-graph RAG pipeline for Vietnamese statute QA. It reports 85.7% answer accuracy against 62.3% for BM25."}
  ],
  "claims": [
    {"text": "Graph-aware retrieval beats plain BM25 on legal QA in the selected abstracts.",
     "paper_ids": ["p-1a2b3c4d5e"], "quote": "85.7% answer accuracy"}
  ]
}
```

Then `multica research ingest-summaries --file research-run/summaries.json`. The file is accepted whole or not at all. The
tool rejects: a summary for a paper that is not selected or has no abstract, an empty summary, a claim with no
`paper_ids`, a cited paper that is not selected, a quote not found in a cited abstract, and any field not listed above. The error lists every
problem; fix them and submit again.

## Report

How many selected, how many summarised, which have no abstract, the claims by paper number as the digest will number
them (rank by score), and any "Cảnh báo" found in source text. Attach `state.json`. Do not export: the Auditor and the
Lead do.
