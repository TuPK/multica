import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RowActions } from "./RowActions";
import { useSidebarCollapsed } from "./sidebar-state";

const setCollapsed = (value: boolean) =>
  vi.mocked(useSidebarCollapsed).mockReturnValue(value);

describe("RowActions", () => {
  beforeEach(() => setCollapsed(true));

  it("renders each action as a button while the sidebar is collapsed", () => {
    const onEdit = vi.fn();
    render(<RowActions label="plan Pro" actions={[{ label: "Edit", onSelect: onEdit }]} />);

    expect(screen.queryByRole("button", { name: "Actions for plan Pro" })).toBeNull();
    screen.getByRole("button", { name: "Edit" }).click();
    expect(onEdit).toHaveBeenCalledTimes(1);
  });

  it("collapses actions into a menu trigger while the sidebar is expanded", async () => {
    setCollapsed(false);
    const user = userEvent.setup();
    const onEdit = vi.fn();
    const onRemove = vi.fn();
    render(
      <RowActions
        label="plan Pro"
        actions={[
          { label: "Edit", onSelect: onEdit },
          { label: "Remove", destructive: true, onSelect: onRemove },
        ]}
      />,
    );

    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Actions for plan Pro" }));
    await user.click(await screen.findByRole("menuitem", { name: "Remove" }));

    expect(onRemove).toHaveBeenCalledTimes(1);
    expect(onEdit).not.toHaveBeenCalled();
  });

  it("does not run a disabled menu item", async () => {
    setCollapsed(false);
    const user = userEvent.setup();
    const onEdit = vi.fn();
    render(
      <RowActions
        label="plan Pro"
        actions={[{ label: "Edit", disabled: true, onSelect: onEdit }]}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Actions for plan Pro" }));
    const item = await screen.findByRole("menuitem", { name: "Edit" });
    await user.click(item);

    expect(onEdit).not.toHaveBeenCalled();
  });

  it("renders nothing when there are no actions", () => {
    const { container } = render(<RowActions label="plan Pro" actions={[]} />);
    expect(container).toBeEmptyDOMElement();
  });
});
