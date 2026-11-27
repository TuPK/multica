// @vitest-environment jsdom
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, ApiError } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { ConsentDialog } from "./consent-dialog";

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const request = vi.mocked(api.request);

function renderDialog(onOpenChange = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  renderWithI18n(
    <QueryClientProvider client={qc}>
      <ConsentDialog open={true} onOpenChange={onOpenChange} wsId="ws-1" />
    </QueryClientProvider>,
  );
  return { onOpenChange };
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("ConsentDialog", () => {
  it("shows all four spec 6.6 points and keeps the confirm button disabled until ticked", () => {
    renderDialog();

    expect(
      screen.getByText("Data read: your decisions and notes in this workspace, and your interview answers."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Who can read it: you, the workspace Secretary, and other owners only if you allow it."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Never used to evaluate people, scorecards, reviews, or organization changes."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("You can delete your profile and all its traits at any time."),
    ).toBeInTheDocument();

    const confirmButton = screen.getByRole("button", { name: "Enable profile" });
    expect(confirmButton).toBeDisabled();

    const shareSwitch = screen.getByRole("switch", { name: "Allow workspace owners to read my profile" });
    expect(shareSwitch.getAttribute("aria-checked")).toBe("false");

    const consentCheckbox = screen.getByRole("checkbox", {
      name: "I understand and consent to Delegora keeping my principal profile in this workspace.",
    });
    fireEvent.click(consentCheckbox);
    expect(confirmButton).toBeEnabled();
  });

  it("sends PUT /api/principal/profile with the displayed consent text and sharing choice", async () => {
    request.mockResolvedValueOnce({
      member_id: "u-1",
      status: "active",
      consent_at: "2026-11-26T08:00:00Z",
      share_with_owners: true,
      version: 1,
      traits: [],
    });
    const { onOpenChange } = renderDialog();

    fireEvent.click(screen.getByRole("switch", { name: "Allow workspace owners to read my profile" }));
    fireEvent.click(
      screen.getByRole("checkbox", {
        name: "I understand and consent to Delegora keeping my principal profile in this workspace.",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Enable profile" }));

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(request).toHaveBeenCalledWith("/api/principal/profile", expect.objectContaining({ method: "PUT" }));
    const body = JSON.parse((request.mock.calls[0]?.[1] as { body: string }).body) as {
      consent_text: string;
      share_with_owners: boolean;
    };
    expect(body.share_with_owners).toBe(true);
    expect(body.consent_text).toContain("Data read: your decisions and notes in this workspace");
    expect(body.consent_text).toContain("You can delete your profile and all its traits at any time.");
    expect(toast.success).toHaveBeenCalledWith("Principal profile enabled.");
  });

  it("maps 422 profile_consent_required and stays open on failure", async () => {
    request.mockRejectedValueOnce(
      new ApiError("consent required", 422, "Unprocessable Entity", { code: "profile_consent_required" }),
    );
    const { onOpenChange } = renderDialog();

    fireEvent.click(
      screen.getByRole("checkbox", {
        name: "I understand and consent to Delegora keeping my principal profile in this workspace.",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Enable profile" }));

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Consent text is required before enabling your profile."),
    );
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });
});
