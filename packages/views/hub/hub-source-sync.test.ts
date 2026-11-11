import { describe, expect, it } from "vitest";
import { autoSyncDuration, autoSyncKey } from "./hub-source-sync";

describe("autoSyncKey", () => {
  it("maps the offered intervals to their labels and keeps any other value as custom", () => {
    expect(autoSyncKey(0)).toBe("off");
    expect(autoSyncKey(900)).toBe("every_15m");
    expect(autoSyncKey(86_400)).toBe("every_24h");
    expect(autoSyncKey(600)).toBe("custom");
  });
});

describe("autoSyncDuration", () => {
  it("uses the largest unit that divides the interval exactly", () => {
    expect(autoSyncDuration(86_400, "en")).toBe("1 day");
    expect(autoSyncDuration(172_800, "en")).toBe("2 days");
    expect(autoSyncDuration(3_600, "en")).toBe("1 hour");
    expect(autoSyncDuration(21_600, "en")).toBe("6 hours");
    expect(autoSyncDuration(600, "en")).toBe("10 minutes");
  });

  it("falls back to seconds when no larger unit divides the interval", () => {
    expect(autoSyncDuration(90, "en")).toBe("90 seconds");
  });

  it("formats in the requested locale", () => {
    expect(autoSyncDuration(600, "es")).toBe("10 minutos");
  });
});
