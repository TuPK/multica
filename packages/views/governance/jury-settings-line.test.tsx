/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { WorkspaceSlugProvider } from "@multica/core/paths";
import { renderWithI18n } from "../test/i18n";
import { JurySettingsLine } from "./jury-settings-line";

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { listWorkspaces: vi.fn() },
}));

const listWorkspaces = vi.mocked(api.listWorkspaces);

function makeWorkspace(settings: Record<string, unknown>) {
  return {
    id: "ws-1",
    name: "Acme",
    slug: "acme",
    description: null,
    context: null,
    settings,
    repos: [],
    issue_prefix: "ACM",
    avatar_url: null,
    created_at: "2026-11-01T00:00:00Z",
    updated_at: "2026-11-01T00:00:00Z",
  };
}

function renderLine() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <WorkspaceSlugProvider slug="acme">
        <JurySettingsLine />
      </WorkspaceSlugProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  listWorkspaces.mockReset();
});

afterEach(cleanup);

describe("JurySettingsLine", () => {
  it("shows the pool size, work-class prefixes, and rule from workspace settings", async () => {
    listWorkspaces.mockResolvedValueOnce([
      makeWorkspace({
        jury_pool: ["a-1", "a-2", "a-3"],
        jury_work_classes: ["flow:pr:", "label:legal"],
      }),
    ]);

    renderLine();

    expect(await screen.findByText("3 agents in pool")).toBeInTheDocument();
    expect(screen.getByText("Work classes: flow:pr:, label:legal")).toBeInTheDocument();
    expect(screen.getByText("Used for important work only. The pool needs at least two agents.")).toBeInTheDocument();
  });

  it("shows the empty state when no jury pool is configured or values are malformed", async () => {
    listWorkspaces.mockResolvedValueOnce([
      makeWorkspace({
        jury_pool: "invalid",
        jury_work_classes: 123,
      }),
    ]);

    renderLine();

    expect(await screen.findByText("No jury pool set")).toBeInTheDocument();
    expect(screen.getByText("Used for important work only. The pool needs at least two agents.")).toBeInTheDocument();
  });
});
