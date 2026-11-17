// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { Layout } from "./Layout";

describe("Layout navigation", () => {
  it("hides Orders, Plans, and Staff from support role", () => {
    render(
      <Layout
        currentScreen="overview"
        onNavigate={vi.fn()}
        user={{ user_id: "u-supp", email: "support@example.com", role: "support" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>
    );

    expect(screen.getByRole("button", { name: /Overview/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Tenants/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Users/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Audit Log/i })).toBeInTheDocument();

    expect(screen.queryByRole("button", { name: /Orders/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Plans/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Staff/i })).not.toBeInTheDocument();
  });

  it("shows Orders to billing role, but hides Plans and Staff", () => {
    render(
      <Layout
        currentScreen="overview"
        onNavigate={vi.fn()}
        user={{ user_id: "u-bill", email: "billing@example.com", role: "billing" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>
    );

    expect(screen.getByRole("button", { name: /Orders/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Plans/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Staff/i })).not.toBeInTheDocument();
  });

  it("shows Orders, Plans, and Staff to admin role", () => {
    render(
      <Layout
        currentScreen="overview"
        onNavigate={vi.fn()}
        user={{ user_id: "u-admin", email: "admin@example.com", role: "admin" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>
    );

    expect(screen.getByRole("button", { name: /Orders/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Plans/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Staff/i })).toBeInTheDocument();
  });

  it("shows Accounts only to admin role", () => {
    const { rerender } = render(
      <Layout
        currentScreen="overview"
        onNavigate={vi.fn()}
        user={{ user_id: "u-supp", email: "support@example.com", role: "support" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>
    );
    expect(screen.queryByRole("button", { name: /Accounts/i })).not.toBeInTheDocument();

    rerender(
      <Layout
        currentScreen="overview"
        onNavigate={vi.fn()}
        user={{ user_id: "u-bill", email: "billing@example.com", role: "billing" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>
    );
    expect(screen.queryByRole("button", { name: /Accounts/i })).not.toBeInTheDocument();

    rerender(
      <Layout
        currentScreen="overview"
        onNavigate={vi.fn()}
        user={{ user_id: "u-admin", email: "admin@example.com", role: "admin" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>
    );
    expect(screen.getByRole("button", { name: /Accounts/i })).toBeInTheDocument();
  });

  it("shows Payment notices to billing and admin, hides it from support", () => {
    const { rerender } = render(
      <Layout
        currentScreen="overview"
        onNavigate={vi.fn()}
        user={{ user_id: "u-supp", email: "support@example.com", role: "support" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>
    );
    expect(screen.queryByRole("button", { name: /Payment notices/i })).not.toBeInTheDocument();

    rerender(
      <Layout
        currentScreen="overview"
        onNavigate={vi.fn()}
        user={{ user_id: "u-bill", email: "billing@example.com", role: "billing" }}
        onLogout={vi.fn()}
      >
        <div>Content</div>
      </Layout>
    );
    expect(screen.getByRole("button", { name: /Payment notices/i })).toBeInTheDocument();
  });
});
