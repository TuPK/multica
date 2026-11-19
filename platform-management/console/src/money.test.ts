import { describe, expect, it } from "vitest";
import { currencyExponent, formatMinorAmount } from "./money";

describe("formatMinorAmount", () => {
  it("uses 0 digits for vnd and 2 for usd and cad", () => {
    expect(currencyExponent("vnd")).toBe(0);
    expect(currencyExponent("usd")).toBe(2);
    expect(currencyExponent("cad")).toBe(2);
    expect(formatMinorAmount(1500, "vnd")).toMatch(/^1\D500 VND$/);
    expect(formatMinorAmount(1234, "usd")).toMatch(/^12\D34 USD$/);
  });

  it("uses 2 digits for an unknown code and prints the integer alone for an empty code", () => {
    expect(formatMinorAmount(1234, "xyz")).toMatch(/^12\D34 XYZ$/);
    expect(formatMinorAmount(5, "")).toBe("5");
  });
});
