# Secretary router: examples

## Classifying a boss message

| Boss says | Route | Why |
| --- | --- | --- |
| "How many issues are in progress in the Mua Ngon project?" | `secretary-facts` | A count the digest holds |
| "Is the checkout relaunch on track for 14 December?" | `secretary-ask` | Whether something is on track needs the leaders' judgment |
| "Why did the tax rule slip?" | `secretary-ask` | A cause needs the owning squad |
| "What did we decide about the spring pricing?" | knowledge search, then `secretary-ask` only if the answer is not in the knowledge base | What we know comes from the knowledge base first |
| "Approve D2, use the 30 percent option" | `secretary-directives` | The boss decided |
| "Tell me how it's going" | Ask one question: which project, and which period? | Scope is genuinely unclear |

## An open request

The boss asks again, "any news on the checkout relaunch?", and a request filed
yesterday is still open with two of three leaders answered.

Report the state: the answered leaders and their headline, the pending leader, the
deadline, and when the assessment will follow. Do not file a second request for the
same question.

## A later run

A child closed and the children-done wakeup starts a run. Its message names the
request. Go to `secretary-analysis`. Do not classify the boss's question again: the
question is already on the request.
