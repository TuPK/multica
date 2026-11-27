// @vitest-environment jsdom
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, ApiError } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { PrincipalProfilePage } from "./principal-profile-page";

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: {
    request: vi.fn(),
    listChatSessions: vi.fn(),
    sendChatMessage: vi.fn(),
  },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const request = vi.mocked(api.request);
const listChatSessions = vi.mocked(api.listChatSessions);
const sendChatMessage = vi.mocked(api.sendChatMessage);

const activeProfile = {
  member_id: "u-1",
  status: "active",
  consent_at: "2026-11-26T08:00:00Z",
  share_with_owners: false,
  version: 2,
  traits: [
    {
      id: "tr-1",
      decision_kind: "general",
      statement: "Prefer reversible steps first.",
      origin: "learned",
      evidence: [{ type: "decision", ref: "00000000-0000-0000-0000-000000000001" }],
      created_at: "2026-11-26T08:00:00Z",
    },
  ],
};

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <PrincipalProfilePage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  listChatSessions.mockResolvedValue([]);
});
afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("PrincipalProfilePage", () => {
  it("maps 404 to not-enabled and opens the consent dialog", async () => {
    request.mockRejectedValueOnce(new ApiError("the person has no principal profile", 404, "Not Found"));
    renderPage();

    expect(await screen.findByRole("heading", { name: "Principal profile is off" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Enable profile" }));
    expect(await screen.findByRole("heading", { name: "Enable principal profile" })).toBeInTheDocument();
  });

  it("shows an error alert for non-404 errors", async () => {
    request.mockRejectedValueOnce(new ApiError("boom", 500, "Internal Server Error"));
    renderPage();

    expect(await screen.findByRole("alert")).toHaveTextContent("Could not load your principal profile.");
  });

  it("shows enabled profile controls, toggles share_with_owners via PUT, and deletes after confirm", async () => {
    request.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === "/api/principal/profile" && !init?.method) return activeProfile;
      if (path === "/api/principal/interview") return { questions: [] };
      if (path === "/api/principal/profile" && init?.method === "PUT") {
        return { ...activeProfile, share_with_owners: true, version: 3, traits: [] };
      }
      if (path === "/api/principal/profile" && init?.method === "DELETE") {
        return undefined;
      }
      throw new Error(`unexpected ${init?.method ?? "GET"} ${path}`);
    });
    renderPage();

    expect(await screen.findByRole("heading", { name: "Principal profile" })).toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.getByText("v2")).toBeInTheDocument();
    expect(screen.getByText("Prefer reversible steps first.")).toBeInTheDocument();

    const shareSwitch = screen.getByRole("switch", { name: "Share with workspace owners" });
    fireEvent.click(shareSwitch);
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Sharing preference saved."));
    const putCall = request.mock.calls.find(([p, init]) => p === "/api/principal/profile" && init?.method === "PUT");
    expect(JSON.parse(String(putCall?.[1]?.body))).toMatchObject({ share_with_owners: true });

    fireEvent.click(screen.getByRole("button", { name: "Delete profile" }));
    expect(await screen.findByRole("heading", { name: "Delete principal profile?" })).toBeInTheDocument();

    const dialogButtons = screen.getAllByRole("button", { name: "Delete profile" });
    fireEvent.click(dialogButtons[dialogButtons.length - 1]!);
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Principal profile deleted."));
    expect(request).toHaveBeenCalledWith("/api/principal/profile", expect.objectContaining({ method: "DELETE" }));
  });

  it("walks interview questions one at a time, sends message_id from the interview chat, and refreshes the profile", async () => {
    let profileCalls = 0;
    request.mockImplementation(async (path: string, init?: RequestInit) => {
      if (path === "/api/principal/profile" && !init?.method) {
        profileCalls += 1;
        return activeProfile;
      }
      if (path === "/api/principal/interview") {
        return { questions: ["Speed or thoroughness?", "Cost threshold?"] };
      }
      if (path === "/api/principal/interview/answers" && init?.method === "POST") {
        return {
          id: "tr-interview-1",
          decision_kind: "general",
          statement: "Thoroughness for external releases",
          origin: "interview",
          evidence: [{ type: "interview", ref: "msg-uuid-1" }],
          created_at: "2026-11-26T10:00:00Z",
        };
      }
      throw new Error(`unexpected ${init?.method ?? "GET"} ${path}`);
    });
    listChatSessions.mockResolvedValueOnce([
      { id: "sess-1", title: "Phỏng vấn hồ sơ" } as never,
    ]);
    sendChatMessage.mockResolvedValueOnce({
      message_id: "msg-uuid-1",
      task_id: "task-1",
      created_at: "2026-11-26T10:00:00Z",
    });

    renderPage();

    expect(await screen.findByText("Speed or thoroughness?")).toBeInTheDocument();
    expect(screen.getByText("Question 1 of 2")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Your answer"), {
      target: { value: "Thoroughness for external releases" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save answer" }));

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Answer saved to your profile."));
    expect(sendChatMessage).toHaveBeenCalledWith("sess-1", "Thoroughness for external releases");
    expect(request).toHaveBeenCalledWith(
      "/api/principal/interview/answers",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          question: 0,
          answer: "Thoroughness for external releases",
          message_id: "msg-uuid-1",
        }),
      }),
    );
    expect(await screen.findByText("Cost threshold?")).toBeInTheDocument();
    expect(screen.getByText("Question 2 of 2")).toBeInTheDocument();
    await waitFor(() => expect(profileCalls).toBeGreaterThanOrEqual(2));
  });
});
