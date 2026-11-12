// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../../test/i18n";
import { ApiError } from "@multica/core/api";
import type { ResearchSettings } from "@multica/core/types";

const mockUpdate = vi.hoisted(() => vi.fn());

const testState = vi.hoisted(() => ({
  settings: undefined as ResearchSettings | undefined,
  isLoading: false,
  role: "admin" as "owner" | "admin" | "member",
}));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-test-1" }));
vi.mock("@multica/core/permissions", () => ({ useCurrentMember: () => ({ member: { role: testState.role } }) }));
vi.mock("@multica/core/research", () => ({
  useResearchSettings: () => ({ data: testState.settings, isLoading: testState.isLoading }),
  useUpdateResearchSettings: () => ({ mutateAsync: mockUpdate, isPending: false }),
}));

const toastMock = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("sonner", () => ({ toast: toastMock }));

import { ResearchTab } from "./research-tab";

const SETTINGS: ResearchSettings = {
  openalex_configured: false,
  semantic_scholar_configured: false,
  contact_email: "",
  keys_can_be_stored: true,
  updated_at: null,
};

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <ResearchTab />
    </QueryClientProvider>,
  );
}

describe("ResearchTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    testState.role = "admin";
    testState.settings = SETTINGS;
    testState.isLoading = false;
  });

  it("shows which keys are stored and never a key", () => {
    testState.settings = { ...SETTINGS, openalex_configured: true, contact_email: "lab@example.org" };
    renderTab();

    expect(screen.getByText("Configured")).toBeInTheDocument();
    expect(screen.getByText("Not set")).toBeInTheDocument();
    const key = screen.getByLabelText("OpenAlex API key") as HTMLInputElement;
    expect(key.type).toBe("password");
    expect(key.value).toBe("");
    expect(screen.getByLabelText("Contact email")).toHaveValue("lab@example.org");
  });

  it("saves only the key that was typed, then clears the box", async () => {
    mockUpdate.mockResolvedValue(SETTINGS);
    renderTab();
    const user = userEvent.setup();

    const input = screen.getByLabelText("OpenAlex API key");
    await user.type(input, "  OA-KEY-1  ");
    await user.click(screen.getAllByRole("button", { name: "Save" })[0]!);

    await waitFor(() => expect(mockUpdate).toHaveBeenCalledWith({ openalex_api_key: "OA-KEY-1" }));
    await waitFor(() => expect(input).toHaveValue(""));
    expect(toastMock.success).toHaveBeenCalledWith("Research settings saved");
  });

  it("keeps Save off until there is something to save", () => {
    renderTab();
    for (const button of screen.getAllByRole("button", { name: "Save" })) expect(button).toBeDisabled();
  });

  it("removes a stored key with an empty value", async () => {
    testState.settings = { ...SETTINGS, semantic_scholar_configured: true };
    mockUpdate.mockResolvedValue(SETTINGS);
    renderTab();

    await userEvent.setup().click(screen.getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(mockUpdate).toHaveBeenCalledWith({ semantic_scholar_api_key: "" }));
    expect(toastMock.success).toHaveBeenCalledWith("Key removed");
  });

  it("saves the contact email on its own", async () => {
    mockUpdate.mockResolvedValue(SETTINGS);
    renderTab();
    const user = userEvent.setup();

    await user.type(screen.getByLabelText("Contact email"), "lab@example.org");
    await user.click(screen.getAllByRole("button", { name: "Save" })[2]!);

    await waitFor(() => expect(mockUpdate).toHaveBeenCalledWith({ contact_email: "lab@example.org" }));
  });

  it("says so, and takes no key, when the server cannot store one", () => {
    testState.settings = { ...SETTINGS, keys_can_be_stored: false };
    renderTab();

    expect(screen.getByRole("status")).toHaveTextContent("This server cannot store API keys");
    expect(screen.getByLabelText("OpenAlex API key")).toBeDisabled();
    expect(screen.getByLabelText("Semantic Scholar API key")).toBeDisabled();
  });

  it("explains a server that has no sealing secret when a save is refused", async () => {
    mockUpdate.mockRejectedValue(new ApiError("no secret", 503, "Service Unavailable", { code: "research_secret_not_configured" }));
    renderTab();
    const user = userEvent.setup();

    await user.type(screen.getByLabelText("OpenAlex API key"), "k");
    await user.click(screen.getAllByRole("button", { name: "Save" })[0]!);

    await waitFor(() => expect(toastMock.error).toHaveBeenCalledWith(expect.stringContaining("MULTICA_RESEARCH_SECRET_KEY")));
  });

  it("shows a plain failure toast for any other error, and keeps what was typed", async () => {
    mockUpdate.mockRejectedValue(new Error("boom"));
    renderTab();
    const user = userEvent.setup();

    const input = screen.getByLabelText("OpenAlex API key");
    await user.type(input, "k");
    await user.click(screen.getAllByRole("button", { name: "Save" })[0]!);

    await waitFor(() => expect(toastMock.error).toHaveBeenCalledWith("Could not save the settings"));
    expect(input).toHaveValue("k");
  });

  it("says the value was not accepted when the server rejects it", async () => {
    mockUpdate.mockRejectedValue(new ApiError("bad key", 400, "Bad Request", { error: "bad key" }));
    renderTab();
    const user = userEvent.setup();

    await user.type(screen.getByLabelText("OpenAlex API key"), "k");
    await user.click(screen.getAllByRole("button", { name: "Save" })[0]!);

    await waitFor(() => expect(toastMock.error).toHaveBeenCalledWith("That value was not accepted"));
  });

  it("is read-only for a member", () => {
    testState.role = "member";
    testState.settings = { ...SETTINGS, openalex_configured: true };
    renderTab();

    expect(screen.getByText("Configured")).toBeInTheDocument();
    expect(screen.queryByLabelText("OpenAlex API key")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    expect(screen.getByLabelText("Contact email")).toBeDisabled();
  });
});
