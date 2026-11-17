import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderWithI18n } from "../test/i18n";
import { SubscriptionLifecycleSection } from "./subscription-lifecycle-section";

function renderSection(
  props: Partial<React.ComponentProps<typeof SubscriptionLifecycleSection>>,
) {
  const onCancel = vi.fn();
  const onResume = vi.fn();
  renderWithI18n(
    <SubscriptionLifecycleSection
      canCancel={false}
      canResume={false}
      periodEnd={null}
      cancelling={false}
      resuming={false}
      onCancel={onCancel}
      onResume={onResume}
      {...props}
    />,
  );
  return { onCancel, onResume };
}

describe("SubscriptionLifecycleSection", () => {
  it("shows only the cancel control when available_actions allows cancel", () => {
    renderSection({ canCancel: true });
    expect(
      screen.getByRole("button", { name: "Cancel subscription" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Resume subscription" }),
    ).not.toBeInTheDocument();
  });

  it("shows only the resume control when available_actions allows resume", async () => {
    const { onResume } = renderSection({ canResume: true });
    expect(
      screen.queryByRole("button", { name: "Cancel subscription" }),
    ).not.toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: "Resume subscription" }),
    );
    expect(onResume).toHaveBeenCalledTimes(1);
  });

  it("renders no lifecycle control when neither action is available", () => {
    renderSection({});
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("cancels only after the confirmation dialog is confirmed", async () => {
    const { onCancel } = renderSection({ canCancel: true });
    await userEvent.click(
      screen.getByRole("button", { name: "Cancel subscription" }),
    );
    expect(onCancel).not.toHaveBeenCalled();
    await userEvent.click(
      screen.getByRole("button", { name: "Cancel at period end" }),
    );
    expect(onCancel).toHaveBeenCalledTimes(1);
  });
});
