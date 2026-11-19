// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TypeToConfirmDialog } from "./TypeToConfirmDialog";

describe("TypeToConfirmDialog", () => {
  it("keeps submit disabled until reason is non-empty and typed text matches", async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined);
    render(
      <TypeToConfirmDialog
        open
        onOpenChange={vi.fn()}
        title="Delete workspace"
        description="This action starts immediate deletion."
        expectedText="acme-ws"
        confirmInputLabel="Workspace slug"
        confirmLabel="Delete workspace"
        confirmVariant="destructive"
        onConfirm={onConfirm}
      />,
    );

    const submitBtn = screen.getByRole("button", { name: "Delete workspace" });
    const reasonInput = screen.getByLabelText(/Reason/i);
    const confirmInput = screen.getByLabelText(/Workspace slug/i);

    expect(submitBtn).toBeDisabled();

    // Reason only -> still disabled
    fireEvent.change(reasonInput, { target: { value: "Customer requested" } });
    expect(submitBtn).toBeDisabled();

    // Whitespace-only reason + matching confirm -> still disabled
    fireEvent.change(reasonInput, { target: { value: "   " } });
    fireEvent.change(confirmInput, { target: { value: "acme-ws" } });
    expect(submitBtn).toBeDisabled();

    // Valid reason + wrong case (case-sensitive by default) -> still disabled
    fireEvent.change(reasonInput, { target: { value: "Customer requested" } });
    fireEvent.change(confirmInput, { target: { value: "ACME-WS" } });
    expect(submitBtn).toBeDisabled();

    // Valid reason + exact match (with outer spaces trimmed) -> enabled
    fireEvent.change(confirmInput, { target: { value: " acme-ws " } });
    expect(submitBtn).toBeEnabled();

    fireEvent.click(submitBtn);
    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith({
        reason: "Customer requested",
        confirm: "acme-ws",
      });
    });
  });

  it("supports case-insensitive matching when caseSensitive is false and shows errors without clearing input", async () => {
    const onConfirm = vi.fn().mockRejectedValue(new Error("confirmation_mismatch"));
    render(
      <TypeToConfirmDialog
        open
        onOpenChange={vi.fn()}
        title="Delete account"
        expectedText="User@Example.com"
        confirmInputLabel="Account email"
        caseSensitive={false}
        confirmLabel="Delete account"
        confirmVariant="destructive"
        error="Failed to submit"
        onConfirm={onConfirm}
      />,
    );

    const reasonInput = screen.getByLabelText(/Reason/i);
    const confirmInput = screen.getByLabelText(/Account email/i);
    const submitBtn = screen.getByRole("button", { name: "Delete account" });

    fireEvent.change(reasonInput, { target: { value: "Legal erasure" } });
    fireEvent.change(confirmInput, { target: { value: "user@example.com" } });
    expect(submitBtn).toBeEnabled();

    expect(screen.getByRole("alert")).toHaveTextContent("Failed to submit");
    expect(reasonInput).toHaveValue("Legal erasure");
    expect(confirmInput).toHaveValue("user@example.com");
  });
});
