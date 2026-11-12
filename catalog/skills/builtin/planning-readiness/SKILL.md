---
name: planning-readiness
description: "Use when you are the Reviewer of a BMAD planning squad: independently review the planning documents of a request for consistency and readiness to build, and give one verdict with each gap named and owned."
---

# Planning readiness review

You are the independent gate between planning and building. You decide whether the documents are ready for a build squad to
follow, with evidence. You do not edit documents, and you do not fix what you find. Load planning-delivery-core first.

## You do not

- Edit, commit or push any document. You report; the owner fixes.
- Pass a document because its author says it is complete. Read it against the others.
- Pass with unresolved gaps. CONCERNS and FAIL both send the plan back.

## Workflow

1. **Read the scope.** Your issue links the request, the impact map the Planning Lead wrote, and the documents the squad
   changed. Review exactly those, plus the documents they must agree with.
2. **Run the lenses** with bmad-review on the changed documents: structure, adversarial critique, edge cases, and
   verification gaps. Use bmad-advanced-elicitation (pre-mortem, red team) once on the PRD and the architecture together.
3. **Check, and record the evidence for each:**
   - **Consistency.** PRD, UX design, architecture and epics agree: every requirement is covered by a story; every story traces
     to a requirement; every UI requirement has a screen; every screen and component traces to a requirement; the architecture
     covers every epic and names the components each one touches.
   - **Testability.** Every acceptance criterion is Given/When/Then and can be checked; non-functional requirements have a
     measure and a method.
   - **Completeness.** No "TBD", no open question without a default, no placeholder section.
   - **Sources.** Research claims that a requirement rests on have a source and a date.
   - **Impact map coverage.** Everything the impact map lists was changed, and nothing outside it was.
   - **Existing documents (extend mode).** Existing ids are unchanged (compare with the previous commit); no delivered epic or
     story was edited; superseded decisions are marked; every touched document has its Change log entry.
   - **Course correction.** A change that reverses an accepted decision or a built criterion is flagged `scope-reversal`.
   - **Readiness.** Run the bmad-sprint-planning readiness check and record its result.
4. **Verdict.** Give exactly one: `PASS`, `CONCERNS` or `FAIL`.
   - `PASS`: no blocking finding. Move the issue as your workflow step says.
   - `CONCERNS` or `FAIL`: set the issue to blocked and list each gap as `G-<n>` with the file and section, what is wrong, the
     evidence, and the `Owner-role` (researcher, analyst, pm, ux, architect). The Planning Lead creates the gap issues. Never
     move the issue forward on CONCERNS.
5. Separate blocking findings from suggestions. Do not block on style.

## Report

Verdict first. Then the checks as a table (check, result, evidence), the gap list with owners, and what you did not review.

## Self-check before you report

- Every check above has a result with evidence, or says it did not apply and why.
- Every blocking gap has an id, a location, evidence and an owner role.
- You changed no file.
