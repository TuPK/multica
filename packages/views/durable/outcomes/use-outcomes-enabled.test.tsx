/**
 * @vitest-environment jsdom
 */
import { afterEach, describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { configStore } from "@multica/core/config";
import { useOutcomesEnabled } from "./use-outcomes-enabled";

// The server publishes its flags in the config store; a view reads them from there. This guards
// against reading them through a context provider that nothing mounts, which always reads off.
describe("useOutcomesEnabled", () => {
  afterEach(() => configStore.getState().setFeatureFlags({}));

  it("reads durable_outcomes from the flags the server published", () => {
    configStore.getState().setFeatureFlags({ durable_outcomes: true });
    expect(renderHook(() => useOutcomesEnabled()).result.current).toBe(true);
  });

  it("reads off when the server does not publish the flag", () => {
    configStore.getState().setFeatureFlags({});
    expect(renderHook(() => useOutcomesEnabled()).result.current).toBe(false);
  });
});
