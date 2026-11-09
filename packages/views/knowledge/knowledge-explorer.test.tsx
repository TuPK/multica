// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import type { KnowledgeBase, KnowledgeDocument, KnowledgeWikiOverview } from "@multica/core/knowledge";
import { renderWithI18n } from "../test/i18n";
import { KnowledgeExplorer } from "./knowledge-explorer";

vi.mock("@multica/core/api", () => ({
  api: {
    listKnowledgeDocuments: vi.fn(),
    searchKnowledge: vi.fn(),
    listKnowledgeKinds: vi.fn(),
    getKnowledgeWiki: vi.fn(),
    listKnowledgeWikiPages: vi.fn(),
    getKnowledgeWikiPage: vi.fn(),
    generateKnowledgeWiki: vi.fn(),
    updateKnowledgeBaseSettings: vi.fn(),
  },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("../navigation", () => ({
  useNavigation: () => ({ push: vi.fn() }),
  AppLink: ({ href, children, ...rest }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));
vi.mock("@multica/core/paths", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/paths")>()),
  useWorkspacePaths: () => ({ projectDetail: (id: string) => `/ws/projects/${id}` }),
}));
vi.mock("../editor/readonly-content", () => ({
  ReadonlyContent: ({ content }: { content: string }) => <div data-testid="wiki-body">{content}</div>,
}));

const bases: KnowledgeBase[] = [
  { id: "b1", key: "company", name: "Company", visibility: "workspace", position: 0 },
  {
    id: "b2",
    key: "project-abc",
    name: "Mua Ngon",
    visibility: "workspace",
    position: 100,
    project_id: "p1",
    project_title: "Mua Ngon",
  },
];

function doc(id: string, kb: string, folder: string, title: string): KnowledgeDocument {
  return {
    id,
    document_id: id,
    knowledge_base: kb,
    title,
    state: "published",
    kind: "manual",
    url: "",
    source_type: "sync",
    source_id: null,
    created_by_type: "system",
    created_by_id: null,
    created_at: "2026-11-01T00:00:00Z",
    published_at: null,
    error: "",
    folder,
  };
}

const documents = [
  doc("1", "company", "", "Charter"),
  doc("2", "project-abc", "docs/planning", "PRD"),
  doc("3", "project-abc", "docs/engineering", "Runbook"),
];

function wikiOverview(overrides: Partial<KnowledgeWikiOverview> = {}): KnowledgeWikiOverview {
  return {
    key: "project-abc",
    enabled: false,
    pages: 0,
    pages_by_type: {},
    documents: 2,
    chat_configured: true,
    ...overrides,
  };
}

function mount(canManage = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={client}>
      <KnowledgeExplorer wsId="ws-1" knowledgeBases={bases} canManage={canManage} secretaryAgentId={null} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.mocked(api.listKnowledgeDocuments)
    .mockReset()
    .mockImplementation(async (params) => ({
      state: "ready",
      documents: documents.filter((d) => !params?.kb || d.knowledge_base === params.kb),
    }));
  vi.mocked(api.listKnowledgeKinds).mockReset().mockResolvedValue([] as never);
  vi.mocked(api.searchKnowledge).mockReset().mockResolvedValue({ state: "ready", hits: [] });
  vi.mocked(api.getKnowledgeWiki).mockReset().mockResolvedValue(wikiOverview());
  vi.mocked(api.listKnowledgeWikiPages).mockReset().mockResolvedValue({ pages: [], total: 0 });
  vi.mocked(api.getKnowledgeWikiPage).mockReset();
  vi.mocked(api.generateKnowledgeWiki).mockReset().mockResolvedValue({ enabled: true, queued: 2 });
});

describe("the library explorer", () => {
  it("shows the company bases and the projects, each with its document count", async () => {
    mount();
    const nav = await screen.findByRole("navigation", { name: "Library structure" });
    expect(within(nav).getByRole("button", { name: /^Company\s*1$/ })).toBeTruthy();
    expect(within(nav).getByRole("button", { name: /^Mua Ngon\s*2$/ })).toBeTruthy();
    expect(within(nav).getByRole("button", { name: /^All documents\s*3$/ })).toBeTruthy();
  });

  it("opens a project base with its folders, narrows to a folder and links the project", async () => {
    mount();
    const nav = await screen.findByRole("navigation", { name: "Library structure" });
    fireEvent.click(await within(nav).findByRole("button", { name: /^Mua Ngon\s*\d/ }));

    expect(await screen.findByRole("heading", { name: "Mua Ngon" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Open project" }).getAttribute("href")).toBe("/ws/projects/p1");

    fireEvent.click(await within(nav).findByRole("button", { name: /^docs\s*2$/ }));
    fireEvent.click(await within(nav).findByRole("button", { name: /^planning/ }));
    expect(await screen.findByText("PRD")).toBeTruthy();
    expect(screen.queryByText("Runbook")).toBeNull();
    expect(screen.getByRole("navigation", { name: "Folder path" })).toBeTruthy();
  });

  it("offers Settings to owners and admins only", async () => {
    const { unmount } = mount(true);
    const nav = await screen.findByRole("navigation", { name: "Library structure" });
    fireEvent.click(await within(nav).findByRole("button", { name: /^Mua Ngon\s*\d/ }));
    expect(await screen.findByRole("tab", { name: "Settings" })).toBeTruthy();
    unmount();

    mount(false);
    const nav2 = await screen.findByRole("navigation", { name: "Library structure" });
    fireEvent.click(await within(nav2).findByRole("button", { name: /^Mua Ngon\s*\d/ }));
    await screen.findByRole("tab", { name: "Wiki" });
    expect(screen.queryByRole("tab", { name: "Settings" })).toBeNull();
  });
});

describe("the Wiki tab", () => {
  async function openWiki() {
    mount();
    const nav = await screen.findByRole("navigation", { name: "Library structure" });
    fireEvent.click(await within(nav).findByRole("button", { name: /^Mua Ngon\s*\d/ }));
    fireEvent.click(await screen.findByRole("tab", { name: "Wiki" }));
  }

  it("offers to generate the Wiki while it is off, and queues the documents", async () => {
    await openWiki();
    expect(await screen.findByText("Wiki is off for this base")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Generate Wiki" }));
    await waitFor(() => expect(api.generateKnowledgeWiki).toHaveBeenCalledWith("project-abc"));
  });

  it("says why it cannot be generated without a chat model", async () => {
    vi.mocked(api.getKnowledgeWiki).mockResolvedValue(wikiOverview({ chat_configured: false }));
    await openWiki();
    expect(await screen.findByText(/no chat model/i)).toBeTruthy();
    expect((screen.getByRole("button", { name: "Generate Wiki" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("lists pages and reads one", async () => {
    vi.mocked(api.getKnowledgeWiki).mockResolvedValue(
      wikiOverview({ enabled: true, pages: 2, pages_by_type: { summary: 1, concept: 1 } }),
    );
    vi.mocked(api.listKnowledgeWikiPages).mockResolvedValue({
      total: 2,
      pages: [
        {
          slug: "summary/prd",
          title: "PRD summary",
          page_type: "summary",
          status: "published",
          summary: "",
          content: "",
          aliases: [],
          category_path: [],
          updated_at: "",
        },
        {
          slug: "concept/taste-mission",
          title: "Taste Mission",
          page_type: "concept",
          status: "published",
          summary: "",
          content: "",
          aliases: [],
          category_path: ["Buying"],
          updated_at: "",
        },
      ],
    });
    vi.mocked(api.getKnowledgeWikiPage).mockResolvedValue({
      slug: "concept/taste-mission",
      title: "Taste Mission",
      page_type: "concept",
      status: "published",
      summary: "",
      content: "# Taste Mission\n\nA buyer states what they want.",
      aliases: [],
      category_path: ["Buying"],
      updated_at: "",
    });
    await openWiki();

    fireEvent.click(await screen.findByRole("button", { name: /Taste Mission/ }));
    expect((await screen.findByTestId("wiki-body")).textContent).toContain("A buyer states what they want.");
    expect(api.getKnowledgeWikiPage).toHaveBeenCalledWith("project-abc", "concept/taste-mission");
    expect(screen.getByRole("button", { name: "Rewrite Wiki" })).toBeTruthy();
  });
});
