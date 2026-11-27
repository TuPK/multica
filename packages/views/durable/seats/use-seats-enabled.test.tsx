/**
 * @vitest-environment jsdom
 */
import { afterEach, describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { configStore } from "@multica/core/config";
import { useSeatsEnabled } from "./use-seats-enabled";

describe("useSeatsEnabled", () => {
  afterEach(() => configStore.getState().setFeatureFlags({}));

  it("reads durable_seats from the flags the server published", () => {
    configStore.getState().setFeatureFlags({ durable_seats: true });
    expect(renderHook(() => useSeatsEnabled()).result.current).toBe(true);
  });

  it("reads off when the server does not publish the flag", () => {
    configStore.getState().setFeatureFlags({});
    expect(renderHook(() => useSeatsEnabled()).result.current).toBe(false);
  });
});
