// @vitest-environment node
import { describe, expect, it } from "vitest";
import { plansForAccountKind } from "./plan-audience";

type Plan = { key: string; audience?: string | null };

const plans: Plan[] = [
  { key: "free", audience: "any" },
  { key: "personal", audience: "individual" },
  { key: "team", audience: "business" },
  { key: "legacy" },
];

describe("plansForAccountKind", () => {
  it("offers individual and any plans to an individual workspace", () => {
    expect(
      plansForAccountKind(plans, "individual").map((p) => p.key),
    ).toEqual(["free", "personal", "legacy"]);
  });

  it("offers business and any plans to a business workspace", () => {
    expect(
      plansForAccountKind(plans, "business").map((p) => p.key),
    ).toEqual(["free", "team", "legacy"]);
  });

  it("treats a plan without audience as any", () => {
    expect(
      plansForAccountKind<Plan>([{ key: "legacy" }], "individual").map((p) => p.key),
    ).toEqual(["legacy"]);
  });
});
