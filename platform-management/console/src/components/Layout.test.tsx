// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { Layout } from "./Layout";

describe("Layout navigation", () => {
  it("renders a grouped desktop SideNav with w-60, aria-current on the active item, and truncated footer email", () => {
    render(
      <Layout
        currentScreen="tenant-detail"
        onNavigate={vi.fn()}
        user={{ user_id: "u-admin", email: "very.long.admin.address@example.com", role: "admin" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>,
    );

    const aside = screen.getByRole("complementary", { name: "Sidebar" });
    expect(aside).toHaveClass("md:w-60");

    const nav = screen.getByRole("navigation", { name: "Main Navigation" });
    expect(within(nav).getByText("Operations")).toBeInTheDocument();
    expect(within(nav).getByText("Commercial")).toBeInTheDocument();
    expect(within(nav).getByText("Administration")).toBeInTheDocument();

    const tenantsBtn = within(nav).getByRole("button", { name: /Tenants/i });
    expect(tenantsBtn).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("button", { name: /Overview/i })).not.toHaveAttribute("aria-current");

    const emailEl = screen.getByText("very.long.admin.address@example.com");
    expect(emailEl).toHaveClass("truncate");
    expect(emailEl).toHaveAttribute("title", "very.long.admin.address@example.com");
    expect(screen.getByText("Admin")).toBeInTheDocument();
  });

  it("hides Commercial group, Plans, Accounts, and Staff from support role", () => {
    render(
      <Layout
        currentScreen="overview"
        onNavigate={vi.fn()}
        user={{ user_id: "u-supp", email: "support@example.com", role: "support" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>,
    );

    const nav = screen.getByRole("navigation", { name: "Main Navigation" });
    expect(within(nav).getByRole("button", { name: /Overview/i })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: /Tenants/i })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: /Users/i })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: /Audit Log/i })).toBeInTheDocument();

    expect(within(nav).queryByText("Commercial")).not.toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: /Orders/i })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: /Payment notices/i })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: /E-invoices/i })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: /Accounts/i })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: /Plans/i })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: /Staff/i })).not.toBeInTheDocument();
  });

  it("shows Orders, Payment notices, and E-invoices to billing role, but hides Accounts, Plans, and Staff", () => {
    render(
      <Layout
        currentScreen="overview"
        onNavigate={vi.fn()}
        user={{ user_id: "u-bill", email: "billing@example.com", role: "billing" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>,
    );

    const nav = screen.getByRole("navigation", { name: "Main Navigation" });
    expect(within(nav).getByText("Commercial")).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: /Orders/i })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: /Payment notices/i })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: /E-invoices/i })).toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: /Accounts/i })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: /Plans/i })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: /Staff/i })).not.toBeInTheDocument();
  });

  it("shows all admin items and invokes onNavigate / onLogout", () => {
    const onNavigate = vi.fn();
    const onLogout = vi.fn();
    render(
      <Layout
        currentScreen="overview"
        onNavigate={onNavigate}
        user={{ user_id: "u-admin", email: "admin@example.com", role: "admin" }}
        onLogout={onLogout}
      >
        <div>Content</div>
      </Layout>,
    );

    const nav = screen.getByRole("navigation", { name: "Main Navigation" });
    for (const name of [
      "Overview",
      "Tenants",
      "Users",
      "Orders",
      "Payment notices",
      "E-invoices",
      "Accounts",
      "Plans",
      "Staff",
      "Audit Log",
    ]) {
      expect(within(nav).getByRole("button", { name })).toBeInTheDocument();
    }

    fireEvent.click(within(nav).getByRole("button", { name: "Orders" }));
    expect(onNavigate).toHaveBeenCalledWith("orders");

    fireEvent.click(screen.getByRole("button", { name: /Sign out/i }));
    expect(onLogout).toHaveBeenCalledTimes(1);
  });
});
