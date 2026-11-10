# Adapters, scoring and probe output

## Adapters

| Adapter | Runs in | Input | Notes |
| --- | --- | --- | --- |
| `rss` | `multica sales collect` | feed URL (RSS 2.0 or Atom); optional keyword filter | Items are `item` or `entry` elements |
| `json_api` | `multica sales collect` | URL; `parse.items` as a dot path to the array (empty for a top-level array); fields as dot paths | Public APIs without a key only |
| `html_jsonld` | `multica sales collect` | HTML page; `parse.items` is a schema.org `@type` such as `JobPosting` | Finds every object of that type, at any depth |
| `html_list` | `multica sales collect` | HTML page; `parse.items` is a selector for each item; fields are selectors relative to the item | Selector subset below |
| `csv_drop` | `multica sales collect --recipe-file <file> --csv <path>` | a CSV the person dropped; the path is passed on the command line, never read from a recipe | Header names map to fields |
| `web_search` | the agent, with the runtime's search tool | a query template | Only when the runtime has the tool |
| `kb_internal` | the agent, through `multica knowledge search --kb <base>` | a kind and a keyword | Other squads' documents |
| `social_connector` | the agent, with an enabled Composio toolkit | toolkit action names | Rung 1 only |
| `provider` | the agent, with a paid provider | actor name and input | Rung 6, budget capped |
| `human_share` | the agent, from `sales_share` | the shared content | Route F |

### Selector subset for `html_list`

- Tag: `div`, `a`. Id: `#main`. Class: `.item`. Attribute: `[href]`, `[data-role=card]`.
- Combinators: a space for descendant, `>` for child.
- A field selector may end with `@attr` to read an attribute (`a@href`); with no `@attr` it reads the element's text.
- Anything outside this subset is refused with the unsupported token named. Use the simplest selector that picks the
  item elements.

## Field mapping

Every recipe maps four fields: `entity` (the organisation or the item's title), `url`, `observed_at`, and `text` (the
summary). `website` is optional and gives the entity key its first choice.

## Signal normalisation

- `observed_at` is `YYYY-MM-DD`. Dates in other formats are parsed; an unparseable date is recorded as missing and the
  item is kept only when `since_days` is not set.
- `entity_key`: the website domain (lowercase, no `www.`), else `tax:<tax code>` when the item names one (10 digits, or
  10 digits and a dash and 3 digits), else `name:<normalised name>|<province>`.
- `summary` is the item's text trimmed to 280 characters.

## Field coverage

`field_coverage` is the share of sampled items that have all four of `entity`, `url`, `observed_at` and `text`. An
onboarding sample passes at 80% or more over at least 5 items.

## Scoring rubric

| Term | Weight | What it measures | 1.0 looks like | 0.0 looks like |
| --- | --- | --- | --- | --- |
| fit | 0.35 | match to the ICP segment, size, region, buyer role | the exact segment and region, the buyer role known | outside the segment |
| signal | 0.30 | strength and clarity of the signal | a clear primary signal, two independent sources | a passing mention |
| timing | 0.15 | how new the signal is for its family's window | inside the first third of the window | outside the window |
| reach | 0.10 | a public contact path the lane's channel allows | a published address or form on the organisation's own page | none |
| proof | 0.10 | a matching case or capability in `sales_proof` | a case in the same sector and size | none |

Round the score to a whole number. Write the five terms next to the score in `## Fit`.

## Probe output

`multica sales probe <input>` prints JSON:

```json
{
  "input": "https://example.vn/tin-tuc",
  "platform": "web",
  "verdict": "active",
  "reason": "RSS feed found in the page head",
  "candidates": [
    {
      "rung": 2,
      "method": "rss",
      "recipe": { "version": 1, "rung": 2, "method": "rss", "...": "..." },
      "http_status": 200,
      "items": 20,
      "field_coverage": 0.95,
      "sample": [ { "family": "expansion", "entity_name": "...", "url": "...", "observed_at": "2026-11-08" } ]
    }
  ]
}
```

`verdict` is one of `active`, `needs_connector` (with the connection named in `reason`), `manual_only`, `blocked`
(with the cause), or `needs_review` (a candidate exists but its sample is below the bar). Probe never uses a paid
provider and never logs in.
