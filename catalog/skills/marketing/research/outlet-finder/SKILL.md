---
name: outlet-finder
description: "Find and rank creative outlets (sales channels, distribution routes, buyer communities, partnerships) for a product, then turn the best ones into cheap experiments with kill criteria. Use when the user asks where to sell, how to find first customers, which channel to try, how to get traction, 'đầu ra cho sản phẩm', 'tìm kênh bán', 'tìm khách đầu tiên', or wants unconventional distribution ideas for a defined segment."
---

# Outlet Finder

You find where a specific segment can be reached and sold to, generate more candidates than the user would think of alone, then narrow to a few testable bets. An outlet is any route to buyers: a channel, a platform, a partner, a community, a reseller, an event, a product feature that spreads.

Method: Bullseye (test a few channels cheaply, then focus on the one that works), widened with creative divergence and cut down with explicit scoring. The final output is experiments, not a channel wishlist.

## Before starting

1. Read the product context if present (`.agents/product-marketing.md`, `.claude/product-marketing.md`, or what the issue provides).
2. You need three inputs. If any is missing, ask the member before generating anything:
   - **Segment**: one named beachhead segment, not "everyone". If segmentation has not been done, run `beachhead-segment` and `ideal-customer-profile` first.
   - **Market**: country/language and B2B vs B2C. Channel effectiveness differs sharply by market.
   - **Constraints**: budget per month, hours per week, team skills, anything off-limits.
3. Apply `evidence-discipline` to every claim in the output.

## Steps

### 1. Map where the segment already is
List concrete places the segment spends attention or money today: platforms, communities, publications, marketplaces, events, tools they already use, people they already trust. Use real names. Mine for them with `customer-research` (Mode 2: online sources) when you can search. Mark each place `observed` (you saw evidence) or `assumed`.

### 2. Diverge: generate at least 30 candidate outlets
Do not filter yet. Use all of these lenses, and tag each candidate with its lens:

| Lens | Question |
|---|---|
| Catalog sweep | Walk all 19 channels in `references/channel-catalog.md`; propose at least one concrete tactic per channel, even if it looks weak |
| Where they already are | Which of step 1's places can we enter, and how? |
| Adjacent industries | Which other product sells to the same buyer on a different day? Partner, bundle, or borrow their route |
| Competitor channels | Where do the competitors from the positioning work get customers? Which channel are they ignoring? |
| Inversion | What would make us the only option in a place nobody sells to? What if we sold to the user's user, or to the person who pays instead of the person who uses? |
| Product as channel | Can the product itself spread: shareable output, embeds, integrations, free tools, templates? |
| SCAMPER | Substitute, combine, adapt, modify, put to another use, eliminate, reverse one element of the usual channel |
| Local reality | Use `references/local-channels.md` as a starting list for Vietnam, then verify each |

### 3. Converge: score and cut to a shortlist
Score each candidate 1-5 on the factors below and show the table. Do not hide weak scores.

| Factor | Meaning |
|---|---|
| Reach | Can it put us in front of the beachhead segment, not just any audience? |
| Intent | How close to a buying moment is the person we reach? |
| Cost to test | Money plus hours for the cheapest meaningful test |
| Speed to signal | Days until we learn something real |
| Control | Do we depend on one platform or person who can cut us off? |

Rank by (Reach + Intent + Speed) minus (Cost to test) plus Control. State the formula. Keep the scoring honest: a high score from guesswork is `assumed`, say so.

Place the shortlist on three rings:
- **Inner ring (3 outlets)**: highest scores, test first.
- **Promising (3-4)**: strong on one factor, unclear on another.
- **Long shots (2-3)**: low odds, high upside, or unconventional; keep one cheap test to avoid missing a surprise.

### 4. Estimate unit economics where possible
For the inner ring, estimate cost per lead and cost per customer with `cac-modeler` if available. If inputs are missing, give the formula and the inputs needed rather than inventing numbers.

### 5. Design a 30-day experiment per inner-ring outlet
Each experiment states:
- **Hypothesis**: "If we do X in outlet Y, then Z of segment will do W."
- **Cheapest version**: what we can run this week without building anything.
- **Metric and threshold**: the number that means keep going, and the number that means stop. Set both before the test.
- **Budget and time cap**.
- **Owner** (member or agent) and **check date**.
- **Kill criteria**: when we drop it regardless of how we feel.

### 6. Report
Deliver one report with: segment and constraints, the full candidate list (collapsed is fine), the scored table, the three rings, the three experiments, assumptions list, and what evidence would change the ranking. Put the next decision for the member at the top.

## Checkpoint

Stop and ask a member before: committing real budget, contacting real people on their behalf, posting publicly, or choosing the outlet to scale. Staging a draft is fine; sending is not.

## Rules

- Quantity first in step 2, judgment in step 3. Do not self-censor while generating.
- A channel is only "good" for a named segment. Never rank channels in the abstract.
- No numbers without a source or a stated formula. "Industry benchmark" is not a source.
- Prefer tests that finish in under 30 days and under the budget the member gave.
- If the same outlet wins for every segment, you have not segmented enough; go back to `user-segmentation`.
