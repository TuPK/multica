import { cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { AttestationBadge, AttestationStatusBadge } from "./attestation-badge";

const flags = vi.hoisted(() => ({ open: true }));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/config", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/config")>()),
  useFeatureEnabled: (key: string, fallback: boolean) =>
    key === "durable_open" ? flags.open : fallback,
}));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));

const request = vi.mocked(api.request);

const sampleDoc = {
  format: "ocf-attestation/1",
  issuer: { host: "delegora.example.com", key_id: "ed25519-1234" },
  signature: "sig",
};

function renderBadge(document: unknown) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <AttestationBadge document={document} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  flags.open = true;
  vi.clearAllMocks();
});

afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("AttestationBadge", () => {
  it("hides the status badge when durable_open is off", () => {
    flags.open = false;
    const { container } = renderWithI18n(<AttestationStatusBadge status="verified" issuer="delegora.example.com" />);
    expect(container.innerHTML).toBe("");
  });

  it("renders nothing and skips verification when durable_open is off", () => {
    flags.open = false;
    const { container } = renderBadge(sampleDoc);
    expect(container.innerHTML).toBe("");
    expect(request).not.toHaveBeenCalled();
  });

  it("shows Verified with issuer host when verification returns valid: true", async () => {
    request.mockResolvedValueOnce({
      valid: true,
      issuer: "delegora.example.com",
      key_id: "ed25519-1234",
    });
    renderBadge(sampleDoc);

    expect(await screen.findByText("Verified by delegora.example.com")).toBeInTheDocument();
    expect(request).toHaveBeenCalledWith(
      "/api/attestations/verify",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("never shows Verified while the request is still pending", () => {
    request.mockReturnValueOnce(new Promise(() => {}));
    renderBadge(sampleDoc);

    expect(screen.queryByText(/Verified/i)).not.toBeInTheDocument();
    expect(screen.getByText("Verifying...")).toBeInTheDocument();
  });

  it("shows Unverified when valid is false or reason is unknown", async () => {
    request.mockResolvedValueOnce({
      valid: false,
      issuer: "delegora.example.com",
      reason: "brand_new_server_reason",
    });
    renderBadge(sampleDoc);

    expect(await screen.findByText("Unverified")).toBeInTheDocument();
  });

  it("shows Unverified on network errors and malformed payloads", async () => {
    request.mockRejectedValueOnce(new Error("network down"));
    const first = renderBadge(sampleDoc);
    expect(await screen.findByText("Unverified")).toBeInTheDocument();
    first.unmount();

    request.mockResolvedValueOnce("not-json-object");
    renderBadge(sampleDoc);
    expect(await screen.findByText("Unverified")).toBeInTheDocument();
  });

  it("does not call verify when document is missing or blank", async () => {
    const first = renderBadge(null);
    expect(screen.getByText("Unverified")).toBeInTheDocument();
    await waitFor(() => expect(request).not.toHaveBeenCalled());
    first.unmount();

    renderBadge("   ");
    expect(screen.getByText("Unverified")).toBeInTheDocument();
    expect(request).not.toHaveBeenCalled();
  });
});
