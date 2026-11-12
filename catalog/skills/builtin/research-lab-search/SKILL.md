---
name: research-lab-search
description: "Use when you are the Literature Scout of a Research Lab squad: turn the protocol into search queries, run a bounded search round with multica research search, merge duplicates with dedup, pick scoring candidates with prefilter, refine from borderline titles, and follow citations with chase. You never score papers. Load research-lab-delivery-core first."
---

# Literature scout

You find the papers; someone else judges them. The person who searched must not be the person who scores, because a
searcher who also scores keeps what it hoped to find. Load research-lab-delivery-core first.

## Start

1. Get the run: download the attached `state.json` from the Lead's sub-issue into `research-run/` and read
   `protocol.md`. Run `multica research status`.
2. Check the round: `search_round` against `max_rounds`. If the limit is reached, stop and tell the Lead; the tool would
   refuse anyway.

## Write the queries

A query is a plain phrase a search engine can match, not a Boolean expression. Round 1 uses one to three queries; the
limit of papers per query comes from the run. Rules:

- Use the field's own words, in English even when the question is not: the sources index English titles and abstracts best.
  Add a second query in the question's language when the topic is local (for example Vietnamese law) and say so.
- No `AND`, `OR`, parentheses, quotes or field syntax. Each query stands alone.
- Cover different angles, not different spellings: the method, the application, the data or the named system.
- Never put a citation or a guessed title in a query.

Run them as one round:

```
multica research search --query "graph retrieval augmented generation legal question answering" \
  --query "knowledge graph legal statutes retrieval" [--source openalex]
multica research dedup
multica research prefilter
```

`search` prints a count per source and query. A source that fails is reported as `SEARCH_FAILED[<source>]` and the others
still run; a source that is out of quota or rate limited is switched off for the rest of the run
(`SOURCE_DISABLED[<source>]`). Do not retry it and do not look for a key; say it in your report.
A query that was already used is skipped and costs no round. If every query was used, the output says
`no_new_queries: true`.

`dedup` merges records by DOI, then source id, then title with first author. A title alone never merges two papers. It
drops paratext (front matter, proceedings volumes) and records without a title, and lists papers with no abstract.
`prefilter` ranks the papers not yet scored by keyword overlap and writes `candidates.json`, capped by
`max_scoring_candidates`. `SCORING_CANDIDATES_CAPPED: n -> m` means some were left out of this round; they come back
next round, ranked again.

## Refine (round 2 and later)

The Lead gives you `used_queries` and `borderline_titles` from the gate. Read the borderline titles: they scored close to
the threshold. Ask what they have that the question wants and what they lack. Write new queries that use the vocabulary
of the good papers and drop the vocabulary of the bad ones. A query that only rewords a used one wastes the round: if you
cannot write a genuinely new query, report it, and the Lead runs `gate --no-new-queries`.

## Follow citations

Only when the Lead asks, after the selected papers are known: `multica research chase --top 5 --per-paper 10`. It adds the
works that cite the top selected papers and the works they cite, through OpenAlex, and spends one round. Papers without
an OpenAlex id are skipped with a warning. Then `dedup` and `prefilter` as before.

## Report

Per the report format in the delivery core. Include, from the command outputs: queries run and what each returned per
source, raw and unique counts, how many excluded and missing abstracts, candidates written, and the round used of the
limit. Attach `state.json`. Do not score, rank or recommend papers.
