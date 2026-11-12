import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithI18n } from "../../test/i18n";
import { KitvietUpdateButton } from "./kitviet-update-button";

const mocks = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
}));

vi.mock("@multica/core/squads/kitviet-update", () => ({
  useUpgradeKitvietSquads: () => ({ mutateAsync: mocks.mutateAsync, isPending: false }),
}));

describe("KitvietUpdateButton", () => {
  beforeEach(() => {
    mocks.mutateAsync.mockReset();
  });

  it("renders nothing when the viewer cannot update or no Kit Viet squad exists", () => {
    renderWithI18n(<KitvietUpdateButton wsId="ws-1" visible={false} />);
    expect(screen.queryByRole("button", { name: "Update Kit Việt" })).toBeNull();
  });

  it("runs the update once when clicked and reports a success", async () => {
    mocks.mutateAsync.mockResolvedValue([]);
    const user = userEvent.setup();
    renderWithI18n(<KitvietUpdateButton wsId="ws-1" visible />);

    await user.click(screen.getByRole("button", { name: "Update Kit Việt" }));

    expect(mocks.mutateAsync).toHaveBeenCalledTimes(1);
    expect(await screen.findByText("Kit Việt squads updated")).toBeTruthy();
  });
});
