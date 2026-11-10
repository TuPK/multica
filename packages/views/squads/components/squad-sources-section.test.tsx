// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { SquadSourcesSection } from "./squad-sources-section";

vi.mock("@multica/core/api", () => ({
  api: {
    getKnowledgeStatus: vi.fn(),
    listKnowledgeCollection: vi.fn(),
    createSquadSource: vi.fn(),
    createSquadShare: vi.fn(),
    updateKnowledgeProperties: vi.fn(),
  },
}));

const sourceDoc = (id: string, status: string, title: string) => ({
  id,
  document_id: `doc-${id}`,
  knowledge_base: "sales",
  title,
  state: "published",
  kind: "manual",
  url: "",
  source_type: "upload",
  source_id: null,
  created_by_type: "member",
  created_by_id: null,
  created_at: "2026-11-10T00:00:00Z",
  published_at: null,
  error: "",
  folder: "",
  knowledge_kind: "sales_source",
  properties: { status, platform: "web", rung: 2, hits_7d: 4 },
});

function renderSection(canManage = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={client}>
      <SquadSourcesSection wsId="ws-1" squadId="squad-1" canManage={canManage} />
    </QueryClientProvider>,
  );
}

describe("SquadSourcesSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listKnowledgeCollection).mockResolvedValue({
      kind: "sales_source",
      fields: [
        { key: "status", label: "Status", type: "select", options: ["active", "proposed", "needs_connector"], required: false, ordered: false },
      ],
      documents: [
        sourceDoc("s1", "active", "Google News expansion"),
        sourceDoc("s2", "needs_connector", "Facebook own page"),
      ],
    } as never);
  });

  it("lists the squad's sources with their status, read from the knowledge base", async () => {
    renderSection();
    expect(await screen.findByText("Google News expansion")).toBeTruthy();
    expect(screen.getByText("Facebook own page")).toBeTruthy();
    expect(screen.getByText("Needs a connection")).toBeTruthy();
  });

  it("sends a typed source to the squad and clears the box", async () => {
    vi.mocked(api.createSquadSource).mockResolvedValue({ document: sourceDoc("s3", "proposed", "Source: x"), issue_id: "i-1" } as never);
    renderSection();
    const box = await screen.findByLabelText(/Paste a link/);
    fireEvent.change(box, { target: { value: "https://example.vn/tin" } });
    fireEvent.click(screen.getByRole("button", { name: "Add source" }));
    await waitFor(() => expect(api.createSquadSource).toHaveBeenCalledWith("squad-1", { input: "https://example.vn/tin" }));
    await waitFor(() => expect((box as HTMLInputElement).value).toBe(""));
  });

  it("shows the intake form only to people who can manage the squad", async () => {
    renderSection(false);
    await screen.findByText("Google News expansion");
    expect(screen.queryByRole("button", { name: "Add source" })).toBeNull();
  });

  it("opens a source's typed properties from its name, read from the kind's fields", async () => {
    renderSection();
    fireEvent.click(await screen.findByRole("button", { name: "Google News expansion" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Status")).toBeTruthy();
  });
});
