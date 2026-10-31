import { describe, expect, it } from "vitest";
import { AGENT_PACK_IDS, loadAgentPack } from "./load";

describe("loadAgentPack", () => {
  it.each(AGENT_PACK_IDS)("loads the %s bundle with valid skills", async (id) => {
    const pack = await loadAgentPack(id);

    expect(pack.id).toBe(id);
    expect(pack.agent.instructions.length).toBeGreaterThan(0);
    const names = pack.skills.map((s) => s.name);
    expect(new Set(names).size).toBe(names.length);
  });

  it("ships the Market Strategist agent with its 19 skills", async () => {
    const pack = await loadAgentPack("market-strategist");

    expect(pack.agent.name).toBe("Market Strategist");
    expect(pack.skills).toHaveLength(19);
    expect(pack.skills.map((s) => s.name)).toContain("outlet-finder");
  });
});
