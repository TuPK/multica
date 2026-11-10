// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { toast } from "sonner";
import { renderWithI18n } from "../test/i18n";
import { HubDownloadBundleButton, HubExportAllButton } from "./hub-export-actions";

const mocks = vi.hoisted(() => ({
  fetchHubBundleDownload: vi.fn(),
  getHubExportBlob: vi.fn(),
}));

vi.mock("@multica/core/hub", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/hub")>();
  return {
    ...actual,
    fetchHubBundleDownload: mocks.fetchHubBundleDownload,
    getHubExportBlob: mocks.getHubExportBlob,
  };
});
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

function renderWithClient(ui: React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return renderWithI18n(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("HubDownloadBundleButton", () => {
  it("opens the bundle link for the item", async () => {
    mocks.fetchHubBundleDownload.mockResolvedValue({ kind: "url", url: "https://files.example.com/a.zip", version: "1" });
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    const user = userEvent.setup();
    renderWithClient(<HubDownloadBundleButton wsId="ws-1" kind="skill" refId="s1" />);

    await user.click(screen.getByRole("button", { name: "Download bundle" }));
    await waitFor(() =>
      expect(open).toHaveBeenCalledWith("https://files.example.com/a.zip", "_blank", "noopener,noreferrer"),
    );
    expect(mocks.fetchHubBundleDownload).toHaveBeenCalledWith("ws-1", "skill", "s1");
    open.mockRestore();
  });

  it("reports a failed download with a toast and opens nothing", async () => {
    mocks.fetchHubBundleDownload.mockRejectedValue(new Error("boom"));
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    const user = userEvent.setup();
    renderWithClient(<HubDownloadBundleButton wsId="ws-1" kind="plugin" refId="p1" />);

    await user.click(screen.getByRole("button", { name: "Download bundle" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Couldn't prepare the download."));
    expect(open).not.toHaveBeenCalled();
    open.mockRestore();
  });
});

describe("HubExportAllButton", () => {
  it("saves the export zip from memory under a dated name", async () => {
    const blob = new Blob(["zip"], { type: "application/zip" });
    mocks.getHubExportBlob.mockResolvedValue(blob);
    URL.createObjectURL = vi.fn(() => "blob:export");
    URL.revokeObjectURL = vi.fn();
    const downloads: string[] = [];
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(function (this: HTMLAnchorElement) {
        downloads.push(this.download);
      });
    const user = userEvent.setup();
    renderWithClient(<HubExportAllButton wsId="ws-1" />);

    await user.click(screen.getByRole("button", { name: "Export all" }));
    await waitFor(() => expect(downloads).toHaveLength(1));
    expect(mocks.getHubExportBlob).toHaveBeenCalledWith("ws-1");
    expect(downloads[0]).toMatch(/^multica-hub-\d{4}-\d{2}-\d{2}\.zip$/);
    expect(URL.createObjectURL).toHaveBeenCalledWith(blob);
    click.mockRestore();
  });

  it("reports a failed export with a toast", async () => {
    mocks.getHubExportBlob.mockRejectedValue(new Error("forbidden"));
    const user = userEvent.setup();
    renderWithClient(<HubExportAllButton wsId="ws-1" />);

    await user.click(screen.getByRole("button", { name: "Export all" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Couldn't export the Hub."));
  });
});
