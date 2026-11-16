// @vitest-environment node
import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api-error";
import { shouldRetryQuery } from "./query-retry";

describe("shouldRetryQuery", () => {
  it("retries a network error once and stops at failureCount 1", () => {
    const networkErr = new Error("Network request failed");
    expect(shouldRetryQuery(0, networkErr)).toBe(true);
    expect(shouldRetryQuery(1, networkErr)).toBe(false);
    expect(shouldRetryQuery(2, networkErr)).toBe(false);
  });

  it("retries transient 5xx ApiError once and stops at failureCount 1", () => {
    const serverErr = new ApiError("500 Internal Server Error", 500);
    expect(shouldRetryQuery(0, serverErr)).toBe(true);
    expect(shouldRetryQuery(1, serverErr)).toBe(false);
  });

  it.each([401, 403])("never retries ApiError %i", (status) => {
    const err = new ApiError("refused", status, {
      code: status === 401 ? "session_revoked" : "account_suspended",
    });
    expect(shouldRetryQuery(0, err)).toBe(false);
    expect(shouldRetryQuery(1, err)).toBe(false);
  });
});
