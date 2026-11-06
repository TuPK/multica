# Secretary analysis: worked examples

## Reading an answer

Weak answer:

> Checkout is 80 percent done and on track. Risks are low.

What to do: open the cited issues, see the open items, and challenge in a comment:

> You say 80 percent. Against what plan: the acceptance list in the project, or the
> story count? Which items are open? Your risk list is empty. Name the one thing that
> would move the date.

Good answer, same squad:

> Completion 65 to 75 percent against the acceptance list (11 of 16 items done; see
> the linked list). Items open: tax calculation for two regions (blocked on the
> provider, owner Finance Lead), refund flow (not started). Schedule: at risk for 14
> November; on track for 28 November if the provider answers by 10 November. Risks:
> provider answer late (owner: Build lead, likelihood medium, impact: date slips a
> fortnight). I did not check the load test; the last run is from 30 October.

What you do with it: accept the range and the reasons, take the provider risk into
the assessment, and say the load test was not checked.

## Across answers

Two leaders disagree: the build squad says the release is green, the release squad's
queue shows three failed deploys since Monday. Name both, say which one the evidence
supports, and say what you could not confirm. Do not average them.

## Writing a decision

Weak:

> Some decisions for you: checkout, the supplier, and hiring.

Good:

> D1. Move the checkout launch: 14 November (on the current risk, 40 percent chance of
> a clean launch) or 28 November (on track, owner Release lead). Recommendation: 28
> November. The owner is the boss.
>
> D2. Supplier contract: sign the 12-month term now, or the 3-month term at 15 percent
> more. Recommendation: 3-month term; the 12-month term is not needed before the
> provider answer.

## A challenge

One thing per comment, with the reason:

> The release squad's queue shows three failed deploys since Monday, and your answer
> says green. Which deploys did you count, and is the failure rate in your confidence?

## Challenge limit

After eight challenges on one child issue with no change, close it as unresolved and
say in the assessment what is missing and who holds it. Do not keep asking.

## An unanswered child after the deadline

The deadline wakeup runs and the Build squad has not answered. The assessment says:

> Build squad: no answer by the deadline on 10 November (request asked 6 November).
> Its last activity on the mua_ngon project is from 4 November. The schedule claim for
> 14 November therefore rests on the Release squad's answer alone, which I could not
> confirm against the Build queue.
