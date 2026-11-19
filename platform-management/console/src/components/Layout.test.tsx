// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { Layout } from "./Layout";

const originalInnerWidth = window.innerWidth;
const originalMatchMedia = window.matchMedia;

function setViewportWidth(width: number) {
  Object.defineProperty(window, "innerWidth", {
    configurable: true,
    writable: true,
    value: width,
  });
  window.matchMedia = (query: string) => {
    const maxMatch = /\(max-width:\s*(\d+)px\)/.exec(query);
    const minMatch = /\(min-width:\s*(\d+)px\)/.exec(query);
    const matchesMax = maxMatch ? width <= Number(maxMatch[1]) : true;
    const matchesMin = minMatch ? width >= Number(minMatch[1]) : true;
    return {
      matches: matchesMax && matchesMin,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    } as MediaQueryList;
  };
}

describe("Layout navigation", () => {
  afterEach(() => {
    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      writable: true,
      value: originalInnerWidth,
    });
    window.matchMedia = originalMatchMedia;
  });

  it("renders AppShell + SideNav from @multica/ui/sidebar with grouped items, aria-current, and footer info", () => {
    setViewportWidth(1280);
    const { container } = render(
      <Layout
        currentScreen="tenant-detail"
        onNavigate={vi.fn()}
        user={{ user_id: "u-admin", email: "very.long.admin.address@example.com", role: "admin" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>,
    );

    expect(container.querySelector('[data-slot="sidebar-wrapper"]')).not.toBeNull();
    expect(container.querySelector('[data-slot="sidebar"]')).not.toBeNull();
    const inset = container.querySelector('[data-slot="sidebar-inset"]');
    expect(inset).not.toBeNull();
    expect(inset).toHaveClass("min-w-0");

    const nav = screen.getByRole("navigation", { name: "Main Navigation" });
    expect(within(nav).getByText("Operations")).toBeInTheDocument();
    expect(within(nav).getByText("Commercial")).toBeInTheDocument();
    expect(within(nav).getByText("Administration")).toBeInTheDocument();

    const tenantsBtn = within(nav).getByRole("button", { name: /Tenants/i });
    expect(tenantsBtn).toHaveAttribute("aria-current", "page");
    expect(tenantsBtn).toHaveAttribute("data-active");
    expect(within(nav).getByRole("button", { name: /Overview/i })).not.toHaveAttribute("aria-current");

    const emailEl = screen.getByText("very.long.admin.address@example.com");
    expect(emailEl).toHaveClass("truncate");
    expect(emailEl).toHaveAttribute("title", "very.long.admin.address@example.com");
    expect(screen.getByText("Admin")).toBeInTheDocument();
  });

  it("collapses and reopens the desktop sidebar via the toggle buttons", () => {
    setViewportWidth(1280);
    const { container } = render(
      <Layout
        currentScreen="overview"
        onNavigate={vi.fn()}
        user={{ user_id: "u-admin", email: "admin@example.com", role: "admin" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>,
    );

    const sidebar = container.querySelector('[data-slot="sidebar"]');
    expect(sidebar).toHaveAttribute("data-state", "expanded");

    fireEvent.click(screen.getByRole("button", { name: "Collapse navigation" }));
    expect(sidebar).toHaveAttribute("data-state", "collapsed");

    const reopenBtn = screen.getByRole("button", { name: "Open navigation" });
    expect(reopenBtn).toHaveAttribute("aria-expanded", "false");

    fireEvent.click(reopenBtn);
    expect(sidebar).toHaveAttribute("data-state", "expanded");
  });

  it("collapses into a mobile drawer on 375px screens with a working menu toggle", () => {
    setViewportWidth(375);
    const onNavigate = vi.fn();
    render(
      <Layout
        currentScreen="overview"
        onNavigate={onNavigate}
        user={{ user_id: "u-admin", email: "admin@example.com", role: "admin" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>,
    );

    // Closed by default on narrow viewports so content is not pushed off-screen
    expect(screen.queryByRole("navigation", { name: "Main Navigation" })).not.toBeInTheDocument();

    const toggleBtn = screen.getByRole("button", { name: "Open navigation" });
    expect(toggleBtn).toHaveAttribute("aria-expanded", "false");

    fireEvent.click(toggleBtn);

    const nav = screen.getByRole("navigation", { name: "Main Navigation" });
    expect(nav).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: "Audit Log" })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: "Staff" })).toBeInTheDocument();

    fireEvent.click(within(nav).getByRole("button", { name: "Audit Log" }));
    expect(onNavigate).toHaveBeenCalledWith("audit");
    expect(screen.queryByRole("navigation", { name: "Main Navigation" })).not.toBeInTheDocument();
  });

  it("hides Commercial group, Plans, Accounts, and Staff from support role", () => {
    setViewportWidth(1280);
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
    setViewportWidth(1280);
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
    setViewportWidth(1280);
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
