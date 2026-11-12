# Gold files for `multica research eval`

A gold file is the answer a person wrote for one research topic, so a finished run can be measured:
`multica research eval --dir research-run --gold docs/engineering/research-lab/gold/<topic>.json`.

**Only a person writes the labels.** An agent that labels its own run's papers measures nothing. No
real gold file ships yet: the spec (section 7.4) asks for three topics of 15 to 30 papers each, labelled
by the requester or a domain expert. `example.json` shows the format with invented entries; do not use
it as a result.

```json
{
  "topic": "Vietnamese legal GraphRAG",
  "relevant": [
    {"doi": "10.59232/dst-v5i1p101"},
    {"openalex_id": "W7161138986"},
    {"arxiv_id": "2501.01234"},
    {"title": "A paper known only by its title"}
  ],
  "merge_pairs": [
    {"a": {"title": "Same paper", "authors": ["Ann Lee"], "metadata_source": "OpenAlex"},
     "b": {"title": "same paper", "authors": ["ann lee"], "metadata_source": "Crossref"}, "same": true}
  ]
}
```

- `relevant`: papers that answer the question. A paper is matched by DOI, OpenAlex id, arXiv id, or an
  equal normalised title.
- `merge_pairs` (optional): record pairs labelled as one paper (`same: true`) or two (`same: false`), to
  count false and missed merges of the deduplication.

The report gives precision at the selected set, recall of the selected set and of everything retrieved,
dedup errors, provenance failures, unresolved DOIs, language compliance, and claim grounding. A measure with
no data is left out, not printed as zero. Set thresholds only after a baseline run (spec 7.3).
