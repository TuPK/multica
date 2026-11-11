// @vitest-environment node
import { describe, expect, it } from "vitest";
import { COMPANY_FLOOR_NAMES } from "./company-template";
import { KITVIET_P2_FLOOR_NAME_LIST, KITVIET_P2_FLOOR_NAMES } from "./kitviet-floors";

describe("Kit Viet P2 floors", () => {
  it("gives each of the ten P2 trades its own floor", () => {
    expect(Object.keys(KITVIET_P2_FLOOR_NAMES)).toHaveLength(10);
    expect(new Set(KITVIET_P2_FLOOR_NAME_LIST).size).toBe(10);
  });

  it("keeps names short, trimmed and free of the default floor names", () => {
    const defaults = Object.values(COMPANY_FLOOR_NAMES);
    for (const name of KITVIET_P2_FLOOR_NAME_LIST) {
      expect(name).toBe(name.trim());
      expect(name.length, name).toBeGreaterThan(0);
      expect(name.length, name).toBeLessThanOrEqual(64);
      expect(defaults, name).not.toContain(name);
    }
  });

  it("uses the floor name a P2 squad template states", () => {
    expect(KITVIET_P2_FLOOR_NAMES.health).toBe("Healthcare");
    expect(KITVIET_P2_FLOOR_NAMES.fnb).toBe("F&B");
  });
});
