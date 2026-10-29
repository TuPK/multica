// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import type { RuntimeQuickstartRequest } from "@multica/core/types";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enRuntimes from "../../locales/en/runtimes.json";

const mockQuickstart = vi.hoisted(() => vi.fn());

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

vi.mock("@multica/core/runtimes", () => ({
  useRuntimeQuickstart: (...args: unknown[]) => mockQuickstart(...args),
}));

import { QuickstartSection } from "./quickstart-section";

const TEST_RESOURCES = {
  en: { common: enCommon, runtimes: enRuntimes },
};

function renderSection(runtimeId: string | null, isOnline: boolean) {
  return render(
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <QuickstartSection runtimeId={runtimeId} isOnline={isOnline} />
    </I18nProvider>,
  );
}

function request(overrides: Partial<RuntimeQuickstartRequest>): RuntimeQuickstartRequest {
  return {
    id: "req-1",
    runtime_id: "runtime-1",
    status: "completed",
    steps: [],
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function setQuickstart(
  overrides: Partial<{
    request: RuntimeQuickstartRequest | undefined;
    isLive: boolean;
    isStarting: boolean;
    startFailed: boolean;
  }> = {},
) {
  mockQuickstart.mockReturnValue({
    request: undefined,
    start: vi.fn(),
    isStarting: false,
    startFailed: false,
    isLive: false,
    ...overrides,
  });
}

describe("QuickstartSection", () => {
  beforeEach(() => {
    mockQuickstart.mockReset();
  });

  it("renders nothing when the viewer cannot command the machine", () => {
    setQuickstart();
    const { container } = renderSection(null, true);
    expect(container).toBeEmptyDOMElement();
  });

  it("disables the button while the machine is offline", () => {
    setQuickstart();
    renderSection("runtime-1", false);
    expect(screen.getByRole("button", { name: "Quickstart" })).toBeDisabled();
  });

  it("shows a spinner and stays disabled while the install runs", () => {
    setQuickstart({
      request: request({ status: "running" }),
      isLive: true,
    });
    renderSection("runtime-1", true);
    const button = screen.getByRole("button", { name: "Quickstart" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button.querySelector("svg.animate-spin")).not.toBeNull();
  });

  it("starts a quickstart when the button is pressed", () => {
    const start = vi.fn();
    setQuickstart({ request: undefined });
    mockQuickstart.mockReturnValue({
      request: undefined,
      start,
      isStarting: false,
      startFailed: false,
      isLive: false,
    });
    renderSection("runtime-1", true);
    fireEvent.click(screen.getByRole("button", { name: "Quickstart" }));
    expect(start).toHaveBeenCalledTimes(1);
  });

  it("says everything was already installed when no step changed anything", () => {
    setQuickstart({
      request: request({
        steps: [
          { name: "claude code", status: "already_installed" },
          { name: "skill react-doctor", status: "already_installed" },
        ],
      }),
    });
    renderSection("runtime-1", true);
    expect(screen.getByText("Everything was already installed.")).toBeInTheDocument();
    expect(screen.getByText("skill react-doctor")).toBeInTheDocument();
  });

  it("lists a failed step with its reason", () => {
    setQuickstart({
      request: request({
        steps: [
          { name: "claude code", status: "installed" },
          { name: "plugin x@m", status: "failed", detail: "marketplace missing" },
        ],
      }),
    });
    renderSection("runtime-1", true);
    expect(screen.getByText("Some items failed. See the details below.")).toBeInTheDocument();
    expect(screen.getByText("marketplace missing")).toBeInTheDocument();
  });

  it("shows the server error when the whole request failed", () => {
    setQuickstart({
      request: request({ status: "failed", error: "the machine did not pick up the request within 2 minutes; check that its daemon is running" }),
    });
    renderSection("runtime-1", true);
    expect(screen.getByText(/did not pick up the request/)).toBeInTheDocument();
  });
});
