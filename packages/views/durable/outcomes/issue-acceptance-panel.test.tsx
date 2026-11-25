/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { api } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { IssueAcceptancePanel } from "./issue-acceptance-panel";

const flag = vi.hoisted(() => ({ on: true }));
vi.mock("@multica/core/config", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/config")>()),
  useFeatureEnabled: () => flag.on,
}));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));

const request = vi.mocked(api.request);

// What the server holds: an archived criterion drops out of the list on the next read.
let criterionArchived = false;
let outcomeVerdict = "pending";

const criterion = {
  id: "c-1",
  issue_id: "i-1",
  position: 0,
  text: "Unit tests pass",
  check_kind: "command",
  check_spec: {},
  source: "agent",
  created_by_type: "agent",
  created_by_id: "a-1",
  archived_at: null,
  created_at: "2026-11-25T08:00:00Z",
};

// Settles in two days, three hours and ten minutes: the chip reads "2d 3h".
const settleAt = new Date(Date.now() + (2 * 24 * 60 + 3 * 60 + 10) * 60_000).toISOString();

const outcome = {
  id: "o-1",
  issue_id: "i-1",
  agent_id: "a-1",
  member_id: null,
  squad_id: "s-1",
  work_class: "general",
  verification_mode: "full",
  sampled: false,
  verdict: "pending",
  severity: "none",
  fail_cause: "",
  settle_at: settleAt,
  settled_at: null,
  superseded_by: null,
  created_at: "2026-11-25T08:00:00Z",
};

function route(path: string) {
  if (path === "/api/issues/i-1/criteria") {
    return Promise.resolve({ criteria: [{ ...criterion, archived_at: criterionArchived ? "2026-11-25T10:00:00Z" : null }] });
  }
  if (path === "/api/issues/i-1/outcome") return Promise.resolve({ ...outcome, verdict: outcomeVerdict });
  if (path === "/api/outcomes/o-1") {
    return Promise.resolve({
      ...outcome,
      verdict: outcomeVerdict,
      verifications: [
        {
          id: "v-1",
          outcome_id: "o-1",
          criterion_id: "c-1",
          verifier_type: "agent",
          verifier_id: "a-2",
          task_id: null,
          result: "pass",
          severity: "none",
          note: "",
          evidence: [],
          created_at: "2026-11-25T09:00:00Z",
        },
      ],
    });
  }
  return Promise.reject(new Error(`unexpected ${path}`));
}

function renderPanel() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <IssueAcceptancePanel wsId="ws-1" issueId="i-1" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  flag.on = true;
  criterionArchived = false;
  outcomeVerdict = "pending";
  request.mockReset();
  request.mockImplementation((path) => route(String(path)));
});
afterEach(cleanup);

describe("IssueAcceptancePanel", () => {
  it("shows the criteria with their result, and a pending chip with the time left", async () => {
    renderPanel();

    const panel = await screen.findByRole("region", { name: "Acceptance" });
    expect(await within(panel).findByText("Unit tests pass")).toBeInTheDocument();
    // The per-criterion result comes from the detail route, which loads after the criteria.
    expect(await within(panel).findByText("Passed")).toBeInTheDocument();
    expect(within(panel).getByText("Pending · settles in 2d 3h")).toBeInTheDocument();
  });

  it("renders nothing while the flag is off", () => {
    flag.on = false;
    const { container } = renderPanel();

    expect(container).toBeEmptyDOMElement();
    expect(request).not.toHaveBeenCalled();
  });

  it("adds a criterion through the dialog and closes it once the server keeps it", async () => {
    const user = userEvent.setup({ delay: null });
    renderPanel();
    await screen.findByText("Unit tests pass");

    request.mockImplementation((path, init) => {
      if (init?.method === "POST") return Promise.resolve({ ...criterion, id: "c-2", text: "Docs updated" });
      return route(String(path));
    });

    await user.click(screen.getByRole("button", { name: "Add criterion" }));
    const dialog = await screen.findByRole("dialog", { name: "Add criterion" });
    await user.type(within(dialog).getByLabelText("Criterion"), "Docs updated");
    await user.click(within(dialog).getByRole("button", { name: "Add" }));

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "/api/issues/i-1/criteria",
        expect.objectContaining({ method: "POST", body: JSON.stringify({ text: "Docs updated", check_kind: "manual" }) }),
      ),
    );
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Add criterion" })).not.toBeInTheDocument());
  });

  it("archives a criterion and removes its row", async () => {
    const user = userEvent.setup({ delay: null });
    renderPanel();
    await screen.findByText("Unit tests pass");

    request.mockImplementation((path, init) => {
      if (init?.method === "PATCH") {
        criterionArchived = true;
        return Promise.resolve({ ...criterion, archived_at: "2026-11-25T10:00:00Z" });
      }
      return route(String(path));
    });

    await user.click(screen.getByRole("button", { name: "Archive criterion" }));

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "/api/issues/i-1/criteria/c-1",
        expect.objectContaining({ method: "PATCH", body: JSON.stringify({ archived: true }) }),
      ),
    );
    await waitFor(() => expect(screen.queryByText("Unit tests pass")).not.toBeInTheDocument());
  });

  it("does not offer to archive once the outcome has settled", async () => {
    outcomeVerdict = "pass";
    renderPanel();

    expect(await screen.findByText("Unit tests pass")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: "Archive criterion" })).toBeDisabled());
  });
});
