// @vitest-environment node
import { describe, expect, it } from "vitest";
import { RESOURCES } from "./index";

// The keys that spec 11.9 of the flow engine names for the pack, proposal, task, trial and request surfaces. The
// parity test proves that the seven bundles have the same keys; this one proves that these keys exist, say what the
// spec says in English, and are translated rather than copied.

type Json = Record<string, unknown>;

const SPEC_11_9_EN: Record<string, string> = {
  "packs.import": "Import a pack",
  "packs.agents": "Agents",
  "packs.skills": "Skills",
  "packs.leader": "Leader",
  "packs.leader_generic": "Use a general leader",
  "packs.one_agent_per_skill": "One agent per skill",
  "packs.ignored": "Ignored settings",
  "packs.design_workflow": "Design the workflow",
  "flow_designer.start.proposal": "Ask an agent to draft",
  "flow_designer.proposal.waiting": "Waiting for the agent's proposal",
  "flow_designer.proposal.import": "Import proposal",
  "flow_designer.proposal.by": "Proposed by {{agent}}",
  "flow_designer.ack.proposal_reviewed": "I have read this workflow",
  "flow_designer.task.inputs": "Inputs",
  "flow_designer.task.skills": "Skills",
  "flow_designer.task.sets": "Sets variables",
  "flow_designer.task.requires_vars": "Must set before closing",
  "flow_designer.trial.run": "Trial run",
  "flow_designer.trial.badge": "Trial",
  "flow_designer.insights.include_trial": "Include trial runs",
  "flow_designer.request.path": "Path",
  "flow_designer.request.feedback": "Feedback",
  "flow_designer.reference.title": "Reference",
};

const pick = (squads: unknown, path: string): unknown => path.split(".").reduce<unknown>((o, k) => (o as Json | undefined)?.[k], squads);
const locales = Object.keys(RESOURCES);
const squadsOf = (locale: string) => (RESOURCES as unknown as Record<string, Record<string, unknown>>)[locale]!.squads;

describe("spec 11.9 locale keys", () => {
  it("covers the eight locales", () => {
    expect(locales.sort()).toEqual(["en", "es", "fr", "ja", "ko", "ru", "zh-Hans", "zh-Hant"]);
  });

  for (const [key, en] of Object.entries(SPEC_11_9_EN)) {
    it(`${key}: English as the spec says, and a translation in each other locale`, () => {
      expect(pick(squadsOf("en"), key)).toBe(en);
      for (const locale of locales.filter((l) => l !== "en")) {
        const value = pick(squadsOf(locale), key);
        expect(typeof value, `${locale} has no string for ${key}`).toBe("string");
        expect((value as string).trim(), `${locale} is empty for ${key}`).not.toBe("");
        // A placeholder the English text has must be kept by the translation.
        for (const placeholder of en.match(/{{\w+}}/g) ?? []) expect(value, `${locale}: ${key} lost ${placeholder}`).toContain(placeholder);
      }
    });
  }
});

describe("no Vietnamese in the interface text of the flow designer and the packs", () => {
  // Letters only Vietnamese uses. The name "Kit Việt" is the name of a product, and stays.
  const VIETNAMESE = /[đĐăĂơƠưƯ\u1EA0-\u1EF9]/;
  const strings = (value: unknown): string[] =>
    typeof value === "string" ? [value] : value && typeof value === "object" ? Object.values(value as Json).flatMap(strings) : [];

  for (const locale of locales) {
    it(`${locale}`, () => {
      const squads = squadsOf(locale);
      const found = [...strings(pick(squads, "flow_designer")), ...strings(pick(squads, "packs"))]
        .map((s) => s.replaceAll("Kit Việt", "Kit"))
        .filter((s) => VIETNAMESE.test(s));
      expect(found).toEqual([]);
    });
  }
});
