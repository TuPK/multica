// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { Layout } from "./Layout";
import { getLanguage, setLanguage } from "../i18n";
import { initTheme, setTheme } from "../theme";
import { PageHeader } from "./PageHeader";

function renderLayout() {
  return render(
    <Layout
      currentScreen="overview"
      onNavigate={vi.fn()}
      user={{ user_id: "u-admin", email: "admin@example.com", role: "system_admin" }}
      onLogout={vi.fn()}
    >
      <PageHeader title="Content" />
    </Layout>,
  );
}

describe("header language and theme switches", () => {
  afterEach(() => {
    setLanguage("en");
    setTheme("blue-white");
    window.localStorage.clear();
  });

  it("starts in English on the blue-white palette", () => {
    initTheme();
    renderLayout();

    expect(getLanguage()).toBe("en");
    expect(document.documentElement.dataset.theme).toBe("blue-white");
    expect(screen.getByRole("button", { name: "EN" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Sign out" })).toBeInTheDocument();
  });

  it("switches the interface to Vietnamese and remembers the choice", () => {
    renderLayout();

    fireEvent.click(screen.getByRole("button", { name: "VI" }));

    expect(getLanguage()).toBe("vi");
    expect(screen.getByRole("button", { name: "VI" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Đăng xuất" })).toBeInTheDocument();
    expect(window.localStorage.getItem("platform-console.language")).toBe("vi");
  });

  it("applies the selected theme to the root element and remembers it", () => {
    renderLayout();

    const theme = screen.getByRole("group", { name: "Theme" });
    fireEvent.click(within(theme).getByRole("button", { name: "Dark" }));

    expect(within(theme).getByRole("button", { name: "Dark" })).toHaveAttribute("aria-pressed", "true");
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(window.localStorage.getItem("platform-console.theme")).toBe("dark");
  });
});
