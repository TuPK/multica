// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { RuntimeAccountMenu } from "./RuntimeAccountMenu";

const mockSetStatusMutateAsync = vi.fn();
const mockUnregisterMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useSetRuntimeStatus: () => ({ mutateAsync: mockSetStatusMutateAsync, isPending: false }),
  useUnregisterRuntime: () => ({ mutateAsync: mockUnregisterMutateAsync, isPending: false }),
}));

function renderMenu(disabled: boolean, canControlRuntime = true) {
  return render(
    <RuntimeAccountMenu
      runtimeId="rt-1"
      runtimeName="mac-studio-01"
      disabled={disabled}
      canControlRuntime={canControlRuntime}
    />,
  );
}

describe("RuntimeAccountMenu", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("disable asks for a reason and sends the status change", async () => {
    mockSetStatusMutateAsync.mockResolvedValueOnce({ changed: true, disabled: true });
    renderMenu(false);

    fireEvent.click(screen.getByRole("button", { name: /^Disable runtime/ }));

    const dialog = screen.getByRole("dialog");
    const submitBtn = within(dialog).getByRole("button", { name: /Disable runtime/i });
    expect(submitBtn).toBeDisabledControl();

    fireEvent.change(within(dialog).getByLabelText(/Reason/i), {
      target: { value: "Abusive daemon traffic" },
    });
    expect(submitBtn).not.toBeDisabledControl();
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(mockSetStatusMutateAsync).toHaveBeenCalledWith({
        runtimeId: "rt-1",
        status: "disabled",
        reason: "Abusive daemon traffic",
      });
    });
  });

  it("offers enable for a disabled runtime", () => {
    renderMenu(true);
    expect(screen.getByRole("button", { name: /^Enable runtime/ })).toBeInTheDocument();
  });

  it("unregister requires typing the runtime name", async () => {
    mockUnregisterMutateAsync.mockResolvedValueOnce({ request_id: "dr-9", state: "pending" });
    renderMenu(false);

    fireEvent.click(screen.getByRole("button", { name: /^Unregister runtime/ }));

    const dialog = screen.getByRole("dialog");
    const confirmBtn = within(dialog).getByRole("button", { name: /Unregister runtime/i });
    expect(confirmBtn).toBeDisabledControl();

    fireEvent.change(within(dialog).getByLabelText(/Reason/i), {
      target: { value: "Machine decommissioned" },
    });
    expect(confirmBtn).toBeDisabledControl();

    const confirmInput = within(dialog).getByLabelText(/Runtime name/i);
    fireEvent.change(confirmInput, { target: { value: "wrong-name" } });
    expect(confirmBtn).toBeDisabledControl();

    fireEvent.change(confirmInput, { target: { value: "mac-studio-01" } });
    expect(confirmBtn).not.toBeDisabledControl();
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(mockUnregisterMutateAsync).toHaveBeenCalledWith({
        runtimeId: "rt-1",
        reason: "Machine decommissioned",
        confirm: "mac-studio-01",
      });
    });
  });

  it("shows no actions when the viewer cannot control runtimes", () => {
    renderMenu(false, false);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
