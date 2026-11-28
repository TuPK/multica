// @vitest-environment jsdom
import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useQuestionOpenedAt } from "./use-question-opened-at";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useQuestionOpenedAt", () => {
  it("captures Date.now() on mount and keeps it stable across re-renders", () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(1_700_000_000_000);

    const { result, rerender } = renderHook(() => useQuestionOpenedAt());
    expect(result.current).toBe(1_700_000_000_000);

    vi.setSystemTime(1_700_000_045_000);
    rerender();
    expect(result.current).toBe(1_700_000_000_000);
  });
});
