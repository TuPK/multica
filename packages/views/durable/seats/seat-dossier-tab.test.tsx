/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { SeatDossierTab } from "./seat-dossier-tab";

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: {
    request: vi.fn(),
    listRuntimes: vi.fn(),
  },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const request = vi.mocked(api.request);
const listRuntimes = vi.mocked(api.listRuntimes);

const activeEntry = {
  id: "d-1",
  agent_id: "a-1",
  kind: "pitfall",
  text: "Always verify migration numbers before pushing",
  work_class: "flow:pr:review",
  evidence: [{ type: "outcome", ref: "o-1" }],
  origin: "outcome",
  status: "active",
  created_at: "2026-11-26T09:00:00Z",
  retired_at: null,
};

function renderTab() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <SeatDossierTab agentId="a-1" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  request.mockReset();
  listRuntimes.mockReset();
  listRuntimes.mockResolvedValue([]);
  vi.mocked(toast.success).mockReset();
  vi.mocked(toast.error).mockReset();
});

afterEach(cleanup);

describe("SeatDossierTab", () => {
  it("renders active entries with kind, text, work_class, origin, and date", async () => {
    request.mockResolvedValueOnce({ entries: [activeEntry] });
    renderTab();

    expect(await screen.findByText("Always verify migration numbers before pushing")).toBeInTheDocument();
    expect(screen.getByText("Pitfall")).toBeInTheDocument();
    expect(screen.getByText("flow:pr:review")).toBeInTheDocument();
    expect(screen.getByText("Outcome")).toBeInTheDocument();
    expect(request).toHaveBeenCalledWith("/api/agents/a-1/dossier?status=active");
  });

  it("shows an empty state when the seat has no entries", async () => {
    request.mockResolvedValueOnce({ entries: [] });
    renderTab();

    expect(await screen.findByText("No dossier entries yet.")).toBeInTheDocument();
  });

  it("filters entries by status", async () => {
    const user = userEvent.setup({ delay: null });
    request.mockImplementation(async (path) => {
      if (String(path).includes("status=retired")) {
        return {
          entries: [{ ...activeEntry, id: "d-2", text: "Retired note", status: "retired" }],
        };
      }
      return { entries: [activeEntry] };
    });

    renderTab();
    await screen.findByText("Always verify migration numbers before pushing");

    await user.click(screen.getByRole("combobox", { name: "Status" }));
    await user.click(await screen.findByRole("option", { name: "Retired" }));

    expect(await screen.findByText("Retired note")).toBeInTheDocument();
    expect(request).toHaveBeenCalledWith("/api/agents/a-1/dossier?status=retired");
  });

  it("adds an entry with kind, text, work_class, and required evidence", async () => {
    const user = userEvent.setup({ delay: null });
    request.mockImplementation(async (_path, init) => {
      if (init?.method === "POST") {
        return { entries: [{ ...activeEntry, id: "d-3", kind: "lesson", origin: "member" }] };
      }
      return { entries: [] };
    });

    renderTab();
    await screen.findByText("No dossier entries yet.");

    await user.click(screen.getByRole("button", { name: "Add entry" }));
    const saveButton = screen.getByRole("button", { name: "Save entry" });
    expect(saveButton).toBeDisabled();

    await user.type(screen.getByLabelText("Note"), "Keep SQL tests isolated");
    expect(saveButton).toBeDisabled();

    await user.type(screen.getByLabelText("Work class"), "general");
    await user.type(screen.getByLabelText("Evidence reference"), "doc-42");
    expect(saveButton).toBeEnabled();

    await user.click(saveButton);

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "/api/agents/a-1/dossier",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({
            entries: [
              {
                kind: "lesson",
                text: "Keep SQL tests isolated",
                work_class: "general",
                evidence: [{ type: "document", ref: "doc-42" }],
              },
            ],
          }),
        }),
      ),
    );
    expect(toast.success).toHaveBeenCalled();
  });

  it("requires a non-empty reason to retire an entry and awaits the server", async () => {
    const user = userEvent.setup({ delay: null });
    request.mockImplementation(async (path, init) => {
      if (init?.method === "POST" && String(path).endsWith("/retire")) {
        return { ...activeEntry, status: "retired", retired_at: "2026-11-27T01:00:00Z" };
      }
      return { entries: [activeEntry] };
    });

    renderTab();
    await screen.findByText("Always verify migration numbers before pushing");

    await user.click(screen.getByRole("button", { name: "Retire" }));
    const confirm = screen.getByRole("button", { name: "Confirm retire" });
    expect(confirm).toBeDisabled();

    await user.type(screen.getByLabelText("Reason"), "Automated by CI check");
    expect(confirm).toBeEnabled();

    await user.click(confirm);

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "/api/agents/a-1/dossier/d-1/retire",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ reason: "Automated by CI check" }),
        }),
      ),
    );
    expect(toast.success).toHaveBeenCalled();
  });

  it("opens the shadow trial dialog from the seat tab", async () => {
    const user = userEvent.setup({ delay: null });
    request.mockImplementation(async (path) => {
      if (String(path).includes("/shadow/estimate")) {
        return { estimate_micros: 500_000, remaining_micros: -1, warn: false };
      }
      return { entries: [] };
    });

    renderTab();
    await screen.findByText("No dossier entries yet.");

    await user.click(screen.getByRole("button", { name: "Start shadow trial" }));
    expect(
      await screen.findByText(
        "Only use a runtime with no access keys to real systems other than the model key.",
      ),
    ).toBeInTheDocument();
  });
});
