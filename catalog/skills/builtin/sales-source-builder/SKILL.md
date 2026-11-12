---
name: sales-source-builder
description: "Use when you are the Scout of a Sales Loop squad and a source must be onboarded or repaired: identify the platform and kind of a link, handle, query, feed, file or description; walk the access ladder from the first-party connector down to the person's share; probe each rung with multica sales probe; check a sample of 20 items; write the recipe; show the boss ten signals before the source goes active; and end in a stated state (active, needs_connector, manual_only, blocked, or degraded), never silently."
---

# Sales source builder

You make a source the squad can read, or you say exactly why it cannot be read and what would unlock it. Every source
someone adds ends in one stated state with a next step. Load sales-delivery-core first. The access ladder and the hard
limits are the same for every source, so read them there before you start.

## The ladder: use the first rung that works and is legal

| Rung | Route | Examples |
| --- | --- | --- |
| 1 | The person's own account, through an enabled connector | the page's inbox or lead forms; Gmail; Calendar |
| 2 | An open format | RSS and Atom, JSON feeds, oEmbed, sitemaps, JSON-LD, a public Telegram channel (`t.me/s/...`), a forum's RSS (`/forums/-/index.rss`) |
| 3 | An official public API | YouTube Data API, Reddit API (after approval), paid X API |
| 4 | A public page, no login, plain HTTP | a company's public page, a directory |
| 5 | A public page that needs a browser to render | a page built with JavaScript; the Web engine, when it is running |
| 6 | A data provider, with a budget | Apify actors for public Facebook pages, posts, groups, reviews, ads library; Firecrawl |
| 7 | The person's share | a pasted link or text, a CSV, a post the person read in a group they belong to |

Try 2, 3, 4, 5 with the collector and the probe. Rung 1 needs the person's connection; rung 6 needs a budget and an owner
or admin's confirmation the first time. When a rung is not allowed, say so and move to the next one that is.

## Hard limits (they apply at every rung)

- No login on the person's behalf, no fake or pooled accounts, no session cookies, no CAPTCHA solving, no private groups
  or profiles, no purchased lists, no guessed addresses, no SMTP probing.
- Stealth, undetected browsers, saved logins and rotating proxies stay off. A source that needs one of them is
  `blocked` with that reason.
- A source whose terms are `restricted` or `unknown` is not made `active`. Its document says what the terms say, or
  that they are unknown, and the boss decides.
- Robots and terms are respected. The collector checks `robots.txt`; a disallowed path is not fetched.

## The onboarding pipeline

1. **Identify.** From the input, determine: the platform (from the host or the handle format), the kind of source
   (feed, listing, directory, post, file, connector), the language, and the region. Use the probe for URLs; use your own
   reading for a description in words. Name the platform and kind in the source's `## What it gives`.
2. **Walk the ladder.** For each rung that applies, run it and record the attempt:

   ```bash
   multica sales probe "<input>" --sample 20 --output json
   ```

   The probe tries feed discovery, JSON-LD, sitemap and a static fetch, and prints one candidate per method with its
   HTTP status, item count and field coverage. Record each attempt in the source's `## Probe log`: rung, method, HTTP
   status, items, fields found, time, and cost (0 for rungs 2 to 5).
3. **Check the sample.** A candidate passes when: at least 5 items are read; `entity`, `url`, `observed_at` and `text`
   are present for 80% or more of them; the items are recent for the family's window (`sales-scouting`, timing windows);
   the items are not all duplicates; and no item holds a private person's personal data beyond what the page shows.
   A failing candidate is recorded with the reason; the next rung is tried.
4. **Write the recipe.** The first passing rung becomes the recipe, with the next passing rung in `fallbacks`. A recipe
   is JSON with `version: 1`, saved in the source's `## Recipe` section in a fenced block. Example:

   ```json
   {
     "version": 1,
     "rung": 2,
     "method": "rss",
     "fetch": { "engine": "http", "url": "https://example.vn/feed", "render": "none" },
     "parse": { "items": "", "fields": { "entity": "title", "url": "link", "observed_at": "pubDate", "text": "description" } },
     "filter": { "keywords_any": ["khai trương", "mở rộng"], "keywords_none": [], "language": "vi", "since_days": 30 },
     "map": { "family": "expansion", "entity_from": "page_name" },
     "cadence": "daily",
     "rate_limit": "1/s",
     "max_items": 100,
     "verified_at": "2026-11-10",
     "sample_count": 20,
     "field_coverage": 0.95,
     "fallbacks": []
   }
   ```

   The recipe holds no secret. A token, cookie or key is never written into it; a connector keeps its own credentials.
   A provider recipe names the toolkit and actor and the cost cap; the cap is the lowest the boss confirmed.
5. **Show the sample.** Write the ten signals the recipe produced (family, entity, date, URL) into the source's `## How
   to read it`, with one line: "if this organisation were a customer, its account would read like this" for the first
   of them. The person sees this before the source goes active.
6. **Decide the state**, and write it in the Record with the reason:
   - `active`: the recipe passed, the terms are `ok` or `attribution` (attribution is written into the source's `## Terms
     and limits`), and the source is not a paid rung. Activation of a new source is the squad's call within the weekly cap
     of new sources (default 5).
   - `active` after the boss confirms, for rung 6 (budget) and for any rung 1 connection. Write the confirmation's date
     and who confirmed it.
   - `needs_connector`: the source has a legal route but needs the person's connection, a key, or a budget. Write one
     sentence in the Record's reason: **which connection, its estimated cost, and what it opens**. Example: "Connect Apify
     (a key, about 5 USD per 1,000 posts, and public posts of the page you name; opens Facebook pages in lane fnb-hcm)."
   - `manual_only`: the only legal route is the person's share (rung 7). Write how to share: "paste the post or its link
     in Source Studio's Share box".
   - `blocked`: the source needs login, is blocked by bot protection, or its terms forbid use. Write the reason.
   - `degraded`: it worked before and fails now; see repair below.
   - `disabled`: the person turned it off. Never re-enable on your own.

## Social networks

The social sources in the starter catalogue each have a row with the unlock sentence. Use the same rules for any other
network:

- Facebook and Instagram pages: the person's own page is rung 1 (a connection); another public page is rung 6 only, with
  the same boundaries; a public group is rung 6 only, and a private group is rung 7 only.
- LinkedIn: the person's own account is rung 1; a company page on the public web is at most rung 4 when the page answers
  without a login (usually it does not). Lists exported by the person are rung 7.
- Zalo, TikTok business accounts: rung 1 through the platform's business API, once the connection exists.
- YouTube, Reddit, Telegram, forums: the open formats (rung 2) and the official API (rung 3) cover them.
- Search engine results are not used as a source; the search engine's terms forbid it.

## Repairs

- A source that returns no usable items, or whose field coverage falls below 80%, is `degraded`. Repair it once: re-run
  the probe on the same input and rebuild the recipe. A repaired recipe is a new version, and it must pass the sample
  check again before it replaces the old one.
- Two repairs a week at most per source. A third failure in the same week is `blocked` with the last error, and a note in
  the weekly snapshot.
- Web-engine schemas (below) are repaired the same way, with the same limit.

## The Web engine (optional)

- If the probe reports that the Web engine is running (`web_engine.status` is `running` in its output), a rung 5 source may use
  `"fetch": { "engine": "crawl4ai", ... }`. The `parse.schema` is a Crawl4AI CSS or XPath extraction schema. You may
  generate it once from a sample of the page's HTML; the collector then runs it without a model.
- If the source needs a browser and the Web engine is not running, the state is `needs_connector` with this sentence:
  "Needs the Web engine: run the container (see docs/engineering/runbooks), then probe again." Never fall back silently
  to plain HTTP for a page that needs a browser.
- The Web engine never renders a social network that requires a login; those stay on rungs 1, 6 or 7.

## Sources the squad finds itself (route E from discovery)

When the lane has no source for a family, look for one that is public and in the same industry (an association's
member directory, a trade fair's exhibitor list, a trade press feed, a regional tender portal). Probe the candidate. A
candidate that is public, needs no login, has no key and no terms restriction, and passes the sample with rungs 2 to 4,
is activated without the boss. Anything else goes to the snapshot's "Needs you" list. At most five new sources a lane a
week.

## Self-check

- The source's Record has one state, with its reason, and the Probe log shows every rung tried.
- The recipe is valid JSON, has `version: 1`, holds no secret, and its `sample_count` and `field_coverage` are the values
  the probe measured.
- A source that is not `active` says what would unlock it, in one sentence, and never silently.
- No login, cookie, CAPTCHA, guessed address or purchased list was used.
