// @vitest-environment jsdom

import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderWithI18n } from "../../test/i18n";
import { DiscardConfirmDialog } from "./discard-confirm-dialog";

describe("DiscardConfirmDialog", () => {
  it("renders when open and handles confirm / cancel", () => {
    const onOpenChange = vi.fn();
    const onConfirm = vi.fn();

    renderWithI18n(
      <DiscardConfirmDialog
        open={true}
        onOpenChange={onOpenChange}
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByText("Discard unsaved changes?")).toBeTruthy();
    expect(screen.getByText("Your unsaved changes will be lost.")).toBeTruthy();

    const cancelBtn = screen.getByRole("button", { name: "Cancel" });
    const confirmBtn = screen.getByRole("button", { name: "Discard and replace" });

    expect(cancelBtn).toBeTruthy();
    expect(confirmBtn).toBeTruthy();

    fireEvent.click(confirmBtn);
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
