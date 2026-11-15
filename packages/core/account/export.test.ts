import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { downloadAccountExport, AccountExportRateLimitedError } from "./export";

const api = vi.hoisted(() => ({ exportAccount: vi.fn() }));
vi.mock("../api", () => ({ api }));

describe("downloadAccountExport", () => {
  beforeEach(() => vi.clearAllMocks());

  it("returns the blob", async () => {
    const blob = new Blob(["{}"]);
    api.exportAccount.mockResolvedValue(blob);

    expect(await downloadAccountExport()).toBe(blob);
  });

  it("maps a 429 to an error with code export_rate_limited", async () => {
    api.exportAccount.mockRejectedValue(
      new ApiError("slow down", 429, "Too Many Requests", { code: "export_rate_limited" }),
    );

    const err = await downloadAccountExport().catch((e: unknown) => e);

    expect(err).toBeInstanceOf(AccountExportRateLimitedError);
    expect((err as AccountExportRateLimitedError).code).toBe("export_rate_limited");
  });

  it("passes other failures through", async () => {
    const failure = new ApiError("nope", 403, "Forbidden");
    api.exportAccount.mockRejectedValue(failure);

    await expect(downloadAccountExport()).rejects.toBe(failure);
  });
});
