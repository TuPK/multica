import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, ApiError } from "@multica/core/api";
import { buildOCFExportPath } from "@multica/core/durable/ocf";
import { renderWithI18n } from "../../test/i18n";
import { ExportHistoryDialog } from "./export-history-dialog";

const flags = vi.hoisted(() => ({ open: true }));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/config", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/config")>()),
  useFeatureEnabled: (key: string, fallback: boolean) =>
    key === "durable_open" ? flags.open : fallback,
}));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: {
    request: vi.fn(),
    getBaseUrl: vi.fn(() => "https://api.example.com"),
    getToken: vi.fn(() => "tok-1"),
  },
}));
vi.mock("@multica/core/platform", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/platform")>()),
  getCurrentSlug: () => "acme",
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const request = vi.mocked(api.request);

const CONSENTED_PROFILE = {
  member_id: "m-1",
  status: "active",
  consent_at: "2026-11-20T00:00:00Z",
  share_with_owners: false,
  version: 1,
  traits: [],
};

function renderDialog(open = true, onOpenChange = vi.fn()) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return {
    onOpenChange,
    ...renderWithI18n(
      <QueryClientProvider client={qc}>
        <ExportHistoryDialog open={open} onOpenChange={onOpenChange} />
      </QueryClientProvider>,
    ),
  };
}

let createObjectURLMock: ReturnType<typeof vi.fn>;
let revokeObjectURLMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  flags.open = true;
  vi.clearAllMocks();
  createObjectURLMock = vi.fn(() => "blob:ocf-export");
  revokeObjectURLMock = vi.fn();
  vi.stubGlobal(
    "URL",
    Object.assign(URL, {
      createObjectURL: createObjectURLMock,
      revokeObjectURL: revokeObjectURLMock,
    }),
  );
});

afterEach(() => {
  cleanup();
  request.mockReset();
  vi.unstubAllGlobals();
});

describe("ExportHistoryDialog", () => {
  it("renders nothing when durable_open is off", () => {
    flags.open = false;
    const { container } = renderDialog(true);
    expect(container.innerHTML).toBe("");
  });

  it("hides Include principal profile when the user has not consented", async () => {
    request.mockRejectedValueOnce(new ApiError("no profile", 404, "Not Found"));
    renderDialog(true);

    const historyToggle = await screen.findByRole("checkbox", { name: "Export history" });
    expect(historyToggle).not.toBeChecked();
    expect(
      screen.queryByRole("checkbox", { name: "Include principal profile" }),
    ).not.toBeInTheDocument();

    fireEvent.click(historyToggle);
    await waitFor(() => expect(request).toHaveBeenCalledWith("/api/principal/profile"));
    expect(
      screen.queryByRole("checkbox", { name: "Include principal profile" }),
    ).not.toBeInTheDocument();
  });

  it("shows Include principal profile only when consented AND Export history is turned on", async () => {
    request.mockResolvedValueOnce(CONSENTED_PROFILE);
    renderDialog(true);

    const historyToggle = await screen.findByRole("checkbox", { name: "Export history" });
    await waitFor(() => expect(request).toHaveBeenCalledWith("/api/principal/profile"));

    expect(
      screen.queryByRole("checkbox", { name: "Include principal profile" }),
    ).not.toBeInTheDocument();

    fireEvent.click(historyToggle);
    expect(
      await screen.findByRole("checkbox", { name: "Include principal profile" }),
    ).toBeInTheDocument();
  });

  it("downloads company.ocf.zip using buildOCFExportPath with chosen options", async () => {
    request.mockResolvedValueOnce(CONSENTED_PROFILE);
    const zipBlob = new Blob(["zip"], { type: "application/zip" });
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      blob: () => Promise.resolve(zipBlob),
    });
    vi.stubGlobal("fetch", fetchMock);

    const { onOpenChange } = renderDialog(true);

    const historyToggle = await screen.findByRole("checkbox", { name: "Export history" });
    await waitFor(() => expect(request).toHaveBeenCalledWith("/api/principal/profile"));

    fireEvent.click(historyToggle);
    const principalToggle = await screen.findByRole("checkbox", {
      name: "Include principal profile",
    });
    fireEvent.click(principalToggle);
    fireEvent.click(screen.getByRole("button", { name: "Export" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      `https://api.example.com${buildOCFExportPath({ history: true, include_principal: true })}`,
      expect.objectContaining({ method: "GET" }),
    );
    await waitFor(() => expect(createObjectURLMock).toHaveBeenCalledWith(zipBlob));
    expect(toast.success).toHaveBeenCalledWith("Exported company.ocf.zip.");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("shows an error toast and keeps the dialog open when export fails", async () => {
    request.mockRejectedValueOnce(new ApiError("no profile", 404, "Not Found"));
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
      json: () => Promise.resolve({ error: "export failed" }),
    });
    vi.stubGlobal("fetch", fetchMock);

    const { onOpenChange } = renderDialog(true);
    fireEvent.click(await screen.findByRole("button", { name: "Export" }));

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Could not export the OCF package."),
    );
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });
});
