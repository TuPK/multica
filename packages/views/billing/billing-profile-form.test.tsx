import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { WorkspaceBillingProfile } from "@multica/core/types";
import { renderWithI18n } from "../test/i18n";
import { BillingProfileForm } from "./billing-profile-form";

const individual: WorkspaceBillingProfile = {
  accountKind: "individual",
  legalName: "",
  taxCode: "",
  address: "",
  invoiceEmail: "",
  verified: false,
};

function renderForm(
  props: Partial<React.ComponentProps<typeof BillingProfileForm>> = {},
) {
  const onSave = vi.fn();
  renderWithI18n(
    <BillingProfileForm
      profile={individual}
      canEdit
      saving={false}
      error={null}
      onSave={onSave}
      {...props}
    />,
  );
  return { onSave };
}

describe("BillingProfileForm", () => {
  it("starts on individual and hides the business fields", () => {
    renderForm();
    const toggle = screen.getByRole("switch", { name: "Business account" });
    expect(toggle).toHaveAttribute("aria-checked", "false");
    expect(screen.queryByLabelText("Legal name")).not.toBeInTheDocument();
  });

  it("shows the business fields when the switch is turned on and saves them", async () => {
    const { onSave } = renderForm();
    await userEvent.click(
      screen.getByRole("switch", { name: "Business account" }),
    );
    await userEvent.type(screen.getByLabelText("Legal name"), "Acme Co");
    await userEvent.type(screen.getByLabelText("Tax code"), "0123456789");
    await userEvent.type(screen.getByLabelText("Address"), "1 Main St");
    await userEvent.type(
      screen.getByLabelText("Invoice email"),
      "billing@acme.test",
    );
    await userEvent.click(screen.getByRole("button", { name: "Save profile" }));
    expect(onSave).toHaveBeenCalledWith({
      accountKind: "business",
      legalName: "Acme Co",
      taxCode: "0123456789",
      address: "1 Main St",
      invoiceEmail: "billing@acme.test",
    });
  });

  it("opens on business when the stored profile is business", () => {
    renderForm({
      profile: {
        ...individual,
        accountKind: "business",
        legalName: "Acme Co",
        taxCode: "0123456789",
        address: "1 Main St",
        invoiceEmail: "billing@acme.test",
      },
    });
    expect(
      screen.getByRole("switch", { name: "Business account" }),
    ).toHaveAttribute("aria-checked", "true");
    expect(screen.getByLabelText("Legal name")).toHaveValue("Acme Co");
  });

  it("hides every write control when editing is not available", () => {
    renderForm({ canEdit: false });
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Save profile" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });
});
