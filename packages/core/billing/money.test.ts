// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  CURRENCY_EXPONENTS,
  currencyExponent,
  formatMinorAmount,
} from "./money";

function fractionDigits(text: string): number {
  return text.match(/\.(\d+)(?!.*\d)/)?.[1]?.length ?? 0;
}

describe("currency exponent table", () => {
  it("holds vnd 0, usd 2, cad 2", () => {
    expect(CURRENCY_EXPONENTS).toEqual({ vnd: 0, usd: 2, cad: 2 });
  });

  it("falls back to 2 digits for an unknown code", () => {
    expect(currencyExponent("xyz")).toBe(2);
  });
});

describe("formatMinorAmount", () => {
  it("prints vnd with 0 fraction digits", () => {
    const out = formatMinorAmount(149000, "vnd", "en-US");
    expect(out).toContain("149,000");
    expect(fractionDigits(out)).toBe(0);
    expect(formatMinorAmount(149000, "VND", "vi-VN")).toMatch(/149\.000/);
  });

  it("prints usd and cad with 2 fraction digits", () => {
    expect(formatMinorAmount(1234, "usd", "en-US")).toBe("$12.34");
    expect(formatMinorAmount(1000, "usd", "en-US")).toBe("$10.00");
    expect(formatMinorAmount(1234, "cad", "en-US")).toContain("12.34");
  });

  it("prints an unknown code with 2 digits and the code", () => {
    expect(formatMinorAmount(1234, "xyz", "en-US")).toBe("12.34 XYZ");
  });
});
