// @vitest-environment node

import { describe, expect, it } from "vitest";
import { hubQuotaLevel } from "./hub-quota-state";

describe("hubQuotaLevel", () => {
  it("treats a limit of zero or less as unlimited", () => {
    expect(hubQuotaLevel(500, 0)).toBe("unlimited");
    expect(hubQuotaLevel(500, -1)).toBe("unlimited");
  });

  it("is ok below 80 percent of the limit", () => {
    expect(hubQuotaLevel(0, 10)).toBe("ok");
    expect(hubQuotaLevel(7, 10)).toBe("ok");
  });

  it("warns from 80 percent of the limit up to the limit", () => {
    expect(hubQuotaLevel(8, 10)).toBe("warning");
    expect(hubQuotaLevel(9, 10)).toBe("warning");
  });

  it("is full at and above the limit", () => {
    expect(hubQuotaLevel(10, 10)).toBe("full");
    expect(hubQuotaLevel(12, 10)).toBe("full");
  });
});
