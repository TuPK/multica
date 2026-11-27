// @vitest-environment jsdom
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, ApiError } from "@multica/core/api";
import type { PrincipalTrait } from "@multica/core/durable/principal";
import { renderWithI18n } from "../../test/i18n";
import { TraitList } from "./trait-list";

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const request = vi.mocked(api.request);

const sampleTraits: PrincipalTrait[] = [
  {
    id: "tr-1",
    decision_kind: "general",
    statement: "Always verify customer-facing copy before shipping.",
    origin: "interview",
    evidence: [{ type: "interview", ref: "00000000-0000-0000-0000-000000000001" }],
    created_at: "2026-11-26T08:00:00Z",
  },
  {
    id: "tr-2",
    decision_kind: "proposal:general",
    statement: "Prefer small reversible rollouts.",
    origin: "learned",
    evidence: [{ type: "decision", ref: "00000000-0000-0000-0000-000000000002" }],
    created_at: "2026-11-26T09:00:00Z",
  },
  {
    id: "tr-3",
    decision_kind: "proposal:general",
    statement: "Ask before changing external SLAs.",
    origin: "edited",
    evidence: [{ type: "comment", ref: "00000000-0000-0000-0000-000000000003" }],
    created_at: "2026-11-26T10:00:00Z",
  },
];

function renderList(traits: PrincipalTrait[] = sampleTraits) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <TraitList wsId="ws-1" traits={traits} />
    </QueryClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("TraitList", () => {
  it("groups traits by decision_kind, shows origin labels, and hides manual add", () => {
    renderList();

    expect(screen.getByText("general")).toBeInTheDocument();
    expect(screen.getByText("proposal:general")).toBeInTheDocument();
    expect(screen.getByText("Always verify customer-facing copy before shipping.")).toBeInTheDocument();
    expect(screen.getByText("Interview")).toBeInTheDocument();
    expect(screen.getByText("Learned")).toBeInTheDocument();
    expect(screen.getByText("Edited")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /add/i })).not.toBeInTheDocument();
  });

  it("edits a trait statement via PATCH /api/principal/profile/traits/:id", async () => {
    request.mockResolvedValueOnce({ ...sampleTraits[0]!, statement: "Updated rule", origin: "edited" });
    renderList([sampleTraits[0]!]);

    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    const textarea = screen.getByLabelText("Trait statement");
    fireEvent.change(textarea, { target: { value: "Updated rule" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Trait updated."));
    expect(request).toHaveBeenCalledWith(
      "/api/principal/profile/traits/tr-1",
      expect.objectContaining({ method: "PATCH", body: JSON.stringify({ statement: "Updated rule" }) }),
    );
  });

  it("retires a trait via DELETE /api/principal/profile/traits/:id", async () => {
    request.mockResolvedValueOnce(undefined);
    renderList([sampleTraits[0]!]);

    fireEvent.click(screen.getByRole("button", { name: "Retire" }));

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Trait retired."));
    expect(request).toHaveBeenCalledWith(
      "/api/principal/profile/traits/tr-1",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it.each([
    ["principal_trait_limit", 409, "You have reached the 200 active trait limit. Retire a trait first."],
    [
      "principal_evidence_invalid",
      422,
      "This trait requires valid evidence from your own decisions, comments, or interview chat.",
    ],
    ["principal_trait_invalid", 422, "Check the trait statement and decision kind (1 to 500 characters)."],
    ["principal_trait_edited", 403, "Only you can retire a trait you wrote or edited."],
  ])("maps server error code %s (%i) to localized copy", async (code, status, expected) => {
    request.mockRejectedValueOnce(new ApiError("err", status, "Error", { code }));
    renderList([sampleTraits[0]!]);

    fireEvent.click(screen.getByRole("button", { name: "Retire" }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(expected));
  });
});
