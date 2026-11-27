// @vitest-environment jsdom
import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { usePrincipalEnabled } from "./use-principal-enabled";

const feature = vi.hoisted(() => ({ enabled: false }));
vi.mock("@multica/core/config", () => ({
  useFeatureEnabled: (_key: string, fallback: boolean) => (feature.enabled ? true : fallback),
}));

beforeEach(() => {
  feature.enabled = false;
});

describe("usePrincipalEnabled", () => {
  it("reads as off when durable_principal is disabled or absent", () => {
    expect(renderHook(() => usePrincipalEnabled()).result.current).toBe(false);
  });

  it("reads as on when durable_principal is enabled", () => {
    feature.enabled = true;
    expect(renderHook(() => usePrincipalEnabled()).result.current).toBe(true);
  });
});
