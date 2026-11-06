// @vitest-environment node
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// The Secretary's procedures are built-in skills, not instruction text. These
// tests pin the contract each skill must keep: the router reaches every other
// skill, and each procedure carries the step the platform depends on.
const skillsDir = fileURLToPath(new URL("./skills", import.meta.url));
const read = (name: string) => readFileSync(resolve(skillsDir, name, "SKILL.md"), "utf8");

const SECRETARY_SKILLS = [
  "secretary-router",
  "secretary-facts",
  "secretary-ask",
  "secretary-analysis",
  "secretary-brief",
  "secretary-directives",
  "secretary-leader-questions",
];

describe("secretary router", () => {
  it("reaches every other Secretary skill by name", () => {
    const router = read("secretary-router");
    for (const name of SECRETARY_SKILLS.filter((n) => n !== "secretary-router")) {
      expect(router, name).toContain(name);
    }
  });

  it("classifies before it acts, and checks for an open request first", () => {
    const router = read("secretary-router");
    expect(router).toContain("Check for an open request on the same question");
    expect(router).toContain("Judgment never comes from the digest.");
  });
});

describe("secretary facts", () => {
  it("answers from the digest at once, with its period", () => {
    expect(read("secretary-facts")).toContain("multica brief digest --since 24h --json");
  });
});

describe("secretary ask", () => {
  const ask = read("secretary-ask");

  it("chooses addressees from the org map and never guesses an id", () => {
    expect(ask).toContain("multica company map --output json");
    expect(ask).toContain("never guess an id");
  });

  it("files the whole request in one call, with the chat session it answers", () => {
    expect(ask).toContain("multica secretary ask --chat-session <session-id>");
    expect(ask).toContain("--addressees-file <addressees.json>");
    expect(ask).toContain("The `chat-session` is the session of the boss's message you are answering.");
  });

  it("sets the deadline inside that call, and tells the boss with a typed reply", () => {
    expect(ask).toContain("the deadline wakeup");
    expect(ask).toContain("multica chat reply --session <session-id> --file <reply.md> --kind ack");
    expect(ask).toContain("Do not wait for the answers.");
  });

  it("states the platform caps", () => {
    expect(ask).toContain("at most 20 addressees per request and 8 open requests");
  });
});

describe("secretary analysis", () => {
  const analysis = read("secretary-analysis");

  it("reads the answers and their threads before judging them", () => {
    expect(analysis).toContain("multica issue children <request-id>");
    expect(analysis).toContain("--roots-only");
  });

  it("caps challenges per child and closes an unresolved child as such", () => {
    expect(analysis).toContain("At most eight challenges per child");
    expect(analysis).toContain("close the child as unresolved");
  });

  it("publishes the assessment as an ad hoc brief and closes the request", () => {
    expect(analysis).toContain("multica brief publish --kind ad_hoc");
    expect(analysis).toContain("multica issue status <request-id> done");
  });
});

describe("secretary brief", () => {
  const brief = read("secretary-brief");

  it("reads the boss's feedback before the digest", () => {
    const feedback = brief.indexOf("multica brief feedback --recent 14d --json");
    const digest = brief.indexOf("multica brief digest --since 24h --json");
    expect(feedback).toBeGreaterThan(-1);
    expect(feedback).toBeLessThan(digest);
  });

  it("writes the ten sections in the agreed order", () => {
    const headings = [
      "1. **Headline**",
      "2. **Needs your decision**",
      "3. **Progress**",
      "4. **Pipeline**",
      "5. **Goals**",
      "6. **Budget**",
      "7. **Blockers and risks**",
      "8. **Watch**",
      "9. **People**",
      "10. **FYI**",
    ];
    const at = headings.map((h) => brief.indexOf(h));
    expect(at.every((i) => i >= 0)).toBe(true);
    expect([...at].sort((a, b) => a - b)).toEqual(at);
  });

  it("keeps the quiet-day line and the pending drafts count", () => {
    expect(brief).toContain("Nothing needs you today.");
    expect(brief).toContain("`pending_drafts`");
  });

  it("asks leaders only with multica brief ask, under the daily cap", () => {
    expect(brief).toContain("multica brief ask --squad <squad-id> --issue <issue-id>");
    expect(brief).toContain("At most three inquiries per workspace per UTC day");
    expect(brief).toContain("`inquiry_limit`");
  });

  it("keeps the pipeline vocabulary in its reference, and the initiative commands read-only", () => {
    expect(brief).toContain("You do not run `multica initiative attest`");
    const reference = readFileSync(resolve(skillsDir, "secretary-brief", "reference.md"), "utf8");
    expect(reference).toContain("## Pipeline vocabulary");
    expect(reference).toContain("A stalled stage is one past its SLA days.");
  });
});

describe("secretary directives", () => {
  const directives = read("secretary-directives");

  it("files a directive only from the boss's confirmed decision, quoted", () => {
    expect(directives).toContain("Wait for confirmation in the boss's chat.");
    expect(directives).toContain("The boss's own words, quoted");
    expect(directives).toContain("Never file a directive from a wakeup");
  });

  it("caps directives per boss message", () => {
    expect(directives).toContain("At most five directives per boss message.");
  });
});

describe("secretary leader questions", () => {
  it("never answers another squad's judgment from its own reading of its issues", () => {
    expect(read("secretary-leader-questions")).toContain(
      "Never answer a judgment question about another squad's work",
    );
  });
});
