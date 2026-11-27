// @vitest-environment jsdom
import { cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "@multica/core/api";
import type { PrincipalPredictionSubjectType } from "@multica/core/durable/principal";
import { renderWithI18n } from "../../test/i18n";
import { PrincipalPredictionLine } from "./prediction-line";

const flags = vi.hoisted(() => ({ principal: false }));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/config", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/config")>()),
  useFeatureEnabled: (key: string, fallback: boolean) => (key === "durable_principal" ? flags.principal : fallback),
}));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));

const request = vi.mocked(api.request);

function renderLine(subjectType: PrincipalPredictionSubjectType, subjectId: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <PrincipalPredictionLine subjectType={subjectType} subjectId={subjectId} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  flags.principal = false;
  vi.clearAllMocks();
});
afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("PrincipalPredictionLine", () => {
  it("renders nothing and does not fetch while durable_principal is off", () => {
    const { container } = renderLine("proposal", "p-1");
    expect(container).toBeEmptyDOMElement();
    expect(request).not.toHaveBeenCalled();
  });

  it("shows the predicted decision and confidence when present", async () => {
    flags.principal = true;
    request.mockResolvedValueOnce({
      predicted: "approve",
      confidence: 0.92,
      why: "Matches your reversible rollout preference.",
      actual: null,
      matched: null,
    });
    renderLine("proposal", "p-1");

    const line = await screen.findByText("Predicted: approve, 92%");
    expect(line).toBeInTheDocument();
    expect(line).toHaveAttribute("title", "Matches your reversible rollout preference.");
    expect(request).toHaveBeenCalledWith(
      "/api/principal/predictions?subject_type=proposal&subject_id=p-1",
    );
  });

  it("shows nothing on 404", async () => {
    flags.principal = true;
    request.mockRejectedValueOnce(new ApiError("no prediction", 404, "Not Found"));
    const { container } = renderLine("effect_intent", "i-1");

    await waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    expect(container).toBeEmptyDOMElement();
  });

  it("shows nothing on server errors", async () => {
    flags.principal = true;
    request.mockRejectedValueOnce(new ApiError("boom", 500, "Internal Server Error"));
    const { container } = renderLine("proposal", "p-2");

    await waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    expect(container).toBeEmptyDOMElement();
  });
});
