// @vitest-environment node
import { describe, expect, it } from "vitest";
import { SQUAD_TEMPLATES, SQUAD_WORKFLOWS } from "../squads/templates";
import { CATALOG_CONTRACTS } from "./generated/contracts";
import { REQUIRED_CONTRACTS, contractProviders } from "./contracts";

describe("catalog contracts", () => {
  it.each(REQUIRED_CONTRACTS)("%s has a stable provider", (id) => {
    expect(contractProviders(id).length).toBeGreaterThan(0);
  });

  it("every declared contract is one the code lists", () => {
    const known: readonly string[] = REQUIRED_CONTRACTS;
    expect(Object.keys(CATALOG_CONTRACTS).filter((id) => !known.includes(id))).toEqual([]);
  });

  it("a squad or workflow provider exists as a template under the same id", () => {
    for (const provider of Object.values(CATALOG_CONTRACTS).flat()) {
      if (provider.kind === "squad") expect(SQUAD_TEMPLATES.some((t) => t.id === provider.id)).toBe(true);
      if (provider.kind === "workflow") expect(SQUAD_WORKFLOWS[provider.id]).toBeDefined();
    }
  });
});
