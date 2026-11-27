/**
 * @vitest-environment jsdom
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen } from "@testing-library/react";
import { Tabs, TabsList } from "@multica/ui/components/ui/tabs";
import { renderWithI18n } from "../test/i18n";
import { useDurableGovernanceTabs } from "./governance-durable-tabs";

const flags = vi.hoisted(() => ({ on: new Set<string>() }));
vi.mock("@multica/core/config", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/config")>()),
  useFeatureEnabled: (flag: string) => flags.on.has(flag),
}));
vi.mock("./effects/governance-effects-tab", () => ({
  GovernanceEffectsTab: () => <p>effects body</p>,
}));
vi.mock("./outcomes/governance-outcomes-tab", () => ({
  GovernanceOutcomesTab: () => <p>outcomes body</p>,
}));

// A stand-in for the governance page: the hook's triggers sit in the list and its contents below.
function Harness() {
  const durableTabs = useDurableGovernanceTabs();
  return (
    <Tabs defaultValue="goals">
      <TabsList>{durableTabs.triggers}</TabsList>
      {durableTabs.contents}
    </Tabs>
  );
}

afterEach(() => {
  cleanup();
  flags.on.clear();
});

describe("useDurableGovernanceTabs", () => {
  it("shows neither durable tab while both flags are off", () => {
    renderWithI18n(<Harness />);

    expect(screen.queryByRole("tab", { name: "Effects" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Outcomes" })).not.toBeInTheDocument();
  });

  it("shows the effects tab only while durable_effects is on", () => {
    flags.on.add("durable_effects");
    renderWithI18n(<Harness />);

    expect(screen.getByRole("tab", { name: "Effects" })).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Outcomes" })).not.toBeInTheDocument();
  });

  it("shows the outcomes tab only while durable_outcomes is on", () => {
    flags.on.add("durable_outcomes");
    renderWithI18n(<Harness />);

    expect(screen.getByRole("tab", { name: "Outcomes" })).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Effects" })).not.toBeInTheDocument();
  });
});
