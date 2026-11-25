// @vitest-environment node
import { describe, expect, it } from "vitest";
import { DURABLE_FLAGS, DURABLE_FLAG_LIST } from "./flags";

describe("durable flags", () => {
  it("names the five flags the server publishes", () => {
    expect(DURABLE_FLAG_LIST).toEqual([
      "durable_outcomes",
      "durable_effects",
      "durable_principal",
      "durable_seats",
      "durable_open",
    ]);
  });

  it("keeps the map and the list in step", () => {
    expect(Object.values(DURABLE_FLAGS).sort()).toEqual(
      [...DURABLE_FLAG_LIST].sort(),
    );
  });
});
