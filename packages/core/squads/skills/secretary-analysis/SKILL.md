---
name: secretary-analysis
description: "Use when the answers to a Secretary request have come in, or its deadline has passed. Reads each answer against its cited sources, challenges weak claims, writes the assessment for the boss, and closes the request."
---

# Secretary analysis

The leaders answered; your job is to read critically, say what you could not confirm,
and give the boss one assessment with the decisions it needs. You do not gather new
facts from squads' issues to replace an answer. You challenge the leader on its child
issue, and the leader answers again.

This skill runs when a child issue of your request closes (the children-done wakeup),
or when the request's deadline passes. Start from the request, not from the boss's
chat: the question is already on the request. Worked examples are in
[reference.md](reference.md).

## Steps

### 1. Load the request and its answers

```bash
multica issue get <request-id> --output json
multica issue children <request-id> --output json
```

For each child, read its root comments first, then open only the threads that carry the
answer:

```bash
multica issue comment list <child-id> --roots-only --summary --compact --output json
multica issue comment list <child-id> --thread <thread-id> --tail 30 --compact --output json
```

### 2. Note what is missing

- A child with no answer and a deadline that has not passed: challenge it once (step 4).
- A child with no answer after the deadline: it is unanswered. Name it in the
  assessment, with the leader and what you asked.

### 3. Read each claim critically

- Open the issue or comment each claim cites before you believe the claim.
- Flag what sounds too tidy, a "green" beside a stale queue, a risk with no owner, a
  number with no source, two answers that contradict each other.
- Mark each claim you confirmed, each you could not confirm, and each you disputed.

### 4. Challenge in a comment on the child

One specific thing per challenge, posted as a comment on the child issue. The leader is
woken to answer again.

```bash
multica issue comment add <child-id> --content-file <file>
```

- At most eight challenges per child. After eight with no change, close the child as unresolved, and say in the assessment what is missing and who holds it.
- Never challenge a point you have already confirmed.

### 5. Compare across the answers

- Where the risk is concentrated, and which squad blocks the others.
- Whether the progress claims add up to the date the company is promising.
- Where two leaders disagree: name both, say which the evidence supports, and say what
  you could not confirm. Do not average them.
- What changed since your last brief: `multica brief list`.

### 6. Write the assessment

In this order, for this boss and this question:

1. A headline the boss reads in ten seconds.
2. The answer to the question, with the range or the verdict and its reason.
3. Progress and risk, each rated, with the reason for the rating.
4. Decisions, numbered D1 to Dn. Each has the situation, the options, your
   recommendation, and the owner. You recommend; you do not decide.
5. Risks, ranked, with cause, likelihood, impact, owner, and proposed response.
6. Disagreements between leaders, named.
7. What was not checked, and which leader did not answer in time.
8. The cost of the request, when the usage is available.

Every claim names its source: a link to the issue or comment, and the date you read it.
Mark each inference as an inference. A quiet question gets a short answer; the list is
a menu, not a form.

### 7. Publish, reply, close

```bash
multica brief publish --kind ad_hoc --title "<short title>" --file <file>
multica issue status <request-id> done
```

Reply in the boss's chat with the headline and the brief's link. The reply is
`--kind assessment`, and the server checks it:

```bash
multica chat reply --session <session-id> --file <file> --kind assessment
```

The server refuses an assessment that comes before every leader has closed its child
(or the deadline has passed), and it refuses one with a claim no leader answer supports.
When it refuses, the message names the claim: revise the claim, or challenge the leader.
The boss sees only what you write with `multica chat reply`; your final message is not shown.

Publish and close only when the assessment is written. A request past its due date
without an assessment shows in the digest.

## Rules

- Never present an unconfirmed claim as a fact.
- Never decide a proposal or a stage gate for the boss.
- Never hand decisions to leaders yourself. Once the boss decides, `secretary-directives`
  does that.
- The assessment is yours, but it cites the leaders' answers. Say so when a conclusion
  is your own inference.
