// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { publishPayload, type Proposal } from "@multica/core/company";
import { renderWithI18n } from "../../test/i18n";
import { PublishProposalSummary, PublishProposalView } from "./publish-proposal-view";
import { CompanyDecisionsDialog } from "./company-decisions-dialog";

const decide = vi.hoisted(() => ({ mutate: vi.fn() }));
const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock("sonner", () => ({ toast: toasts }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/company", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/company")>()),
  useDecideProposal: () => ({ mutate: decide.mutate, isPending: false }),
}));
vi.mock("@multica/core/issues/queries", () => ({
  issueDetailOptions: () => ({ queryKey: ["issue-detail"], queryFn: () => null }),
}));
vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({ issueDetail: (id: string) => `/acme/issues/${id}` }),
  useCurrentWorkspace: () => ({ id: "ws-1", name: "Acme", slug: "acme" }),
}));
vi.mock("@multica/core/workspace/hooks", () => ({
  useActorName: () => ({ getActorName: (type: string, id: string) => `${type} ${id}` }),
}));
vi.mock("../../editor/readonly-content", () => ({
  ReadonlyContent: ({ content }: { content: string }) => <div>{content}</div>,
}));

function makePublishProposal(extra: Partial<Proposal> = {}): Proposal {
  return {
    id: "prop-publish-1",
    workspace_id: "ws-1",
    squad_id: "squad-cs",
    issue_id: "issue-10",
    issue_identifier: "MUL-10",
    issue_status: "in_review",
    title: "Launch announcement post",
    handoff_squad_id: null,
    urgent: false,
    urgent_reason: "",
    state: "pending",
    kind: "publish" as unknown as Proposal["kind"],
    payload: {
      item_id: "cs-item-42",
      version: 1,
      market: "intl",
      scheduled_at: null,
      channels: [
        {
          channel_id: "ch-x",
          platform: "x",
          account_name: "@multica_hq",
          text: "Excited to introduce Multica Content Studio with autonomous agents!",
          media_url: "https://example.com/banner.png",
          hashtags: ["multica", "ai"],
        },
        {
          channel_id: "ch-li",
          platform: "linkedin",
          account_name: "Multica Official",
          text: "Today we are launching the Content Studio squad to automate publishing workflows.",
          media_url: null,
          hashtags: ["ContentMarketing", "Automation"],
        },
      ],
    },
    created_by_type: "agent",
    created_by_id: "cs_lead",
    decided_by: null,
    decided_at: null,
    decision_note: "",
    created_at: "2026-11-12T08:00:00Z",
    revision: 1,
    expires_at: null,
    snoozed_until: null,
    related: [],
    handoff: null,
    ...extra,
  };
}

function renderWithClient(ui: React.ReactElement) {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return renderWithI18n(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe("PublishProposalView and publishPayload", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("publishPayload parser", () => {
    it("parses valid proposal payload correctly", () => {
      const prop = makePublishProposal();
      const payload = publishPayload(prop);
      expect(payload).not.toBeNull();
      expect(payload?.item_id).toBe("cs-item-42");
      expect(payload?.version).toBe(1);
      expect(payload?.market).toBe("intl");
      expect(payload?.scheduled_at).toBeNull();
      expect(payload?.channels).toHaveLength(2);
      expect(payload?.channels[0]?.platform).toBe("x");
      expect(payload?.channels[0]?.account_name).toBe("@multica_hq");
      expect(payload?.channels[0]?.hashtags).toEqual(["multica", "ai"]);
    });

    it("returns null for non-object or empty payload without item_id or channels", () => {
      const prop = makePublishProposal({ payload: {} });
      expect(publishPayload(prop)).toBeNull();
    });

    it("defaults optional channel fields when the rest of the payload is valid", () => {
      const prop = makePublishProposal({
        payload: {
          item_id: "item-partial",
          version: 1,
          market: "intl",
          channels: [{ channel_id: "ch-fb", text: "hello", platform: "facebook" }],
        },
      });
      const payload = publishPayload(prop);
      expect(payload?.item_id).toBe("item-partial");
      expect(payload?.scheduled_at).toBeNull();
      expect(payload?.channels[0]?.platform).toBe("facebook");
      expect(payload?.channels[0]?.account_name).toBe("");
      expect(payload?.channels[0]?.media_url).toBeNull();
      expect(payload?.channels[0]?.hashtags).toEqual([]);
    });

    it("reads a payload that misses a required field as null", () => {
      const prop = makePublishProposal({
        payload: { item_id: "item-no-version", market: "intl", channels: [] },
      });
      expect(publishPayload(prop)).toBeNull();
    });
  });

  describe("PublishProposalSummary component", () => {
    it("renders item ID, version, market, schedule, and all channel cards", () => {
      const prop = makePublishProposal();
      renderWithClient(<PublishProposalSummary proposal={prop} />);

      // Item & version
      expect(screen.getByText("Item: cs-item-42")).toBeInTheDocument();
      expect(screen.getByText("Version 1")).toBeInTheDocument();

      // Market & schedule
      expect(screen.getByText("Market: International (intl)")).toBeInTheDocument();
      expect(screen.getByText("Scheduled: Immediate")).toBeInTheDocument();

      // Channels count header
      expect(screen.getByText("Channels (2)")).toBeInTheDocument();

      // Channel texts & accounts
      expect(screen.getByText("@multica_hq")).toBeInTheDocument();
      expect(screen.getByText("Multica Official")).toBeInTheDocument();
      expect(
        screen.getByText("Excited to introduce Multica Content Studio with autonomous agents!"),
      ).toBeInTheDocument();
      expect(
        screen.getByText("Today we are launching the Content Studio squad to automate publishing workflows."),
      ).toBeInTheDocument();

      // Media link
      expect(screen.getByRole("link", { name: /Media/i })).toHaveAttribute(
        "href",
        "https://example.com/banner.png",
      );

      // Hashtags
      expect(screen.getByText("#multica")).toBeInTheDocument();
      expect(screen.getByText("#ai")).toBeInTheDocument();
      expect(screen.getByText("#ContentMarketing")).toBeInTheDocument();
    });

    it("renders scheduled timestamp when scheduled_at is set", () => {
      const prop = makePublishProposal({
        payload: {
          item_id: "cs-item-scheduled",
          version: 2,
          market: "vn",
          scheduled_at: "2026-11-20T15:30:00Z",
          channels: [
            {
              channel_id: "ch-fb",
              platform: "facebook",
              account_name: "Multica Vietnam",
              text: "Ngày mai ra mắt.",
              media_url: null,
              hashtags: [],
            },
          ],
        },
      });
      renderWithClient(<PublishProposalSummary proposal={prop} />);

      expect(screen.getByText("Market: Vietnam (vn)")).toBeInTheDocument();
      expect(screen.getByText(/Scheduled: /)).toBeInTheDocument();
      expect(screen.getByText("Channels (1)")).toBeInTheDocument();
    });

    it("renders nothing for a publish payload with no channels, which the server refuses to file", () => {
      const prop = makePublishProposal({
        payload: { item_id: "cs-item-empty", version: 1, market: "intl", scheduled_at: null, channels: [] },
      });
      const { container } = renderWithClient(<PublishProposalSummary proposal={prop} />);
      expect(container).toBeEmptyDOMElement();
    });
  });

  describe("PublishProposalView with decision actions", () => {
    it("executes Approve action", () => {
      const prop = makePublishProposal();
      renderWithClient(<PublishProposalView proposal={prop} canDecide={true} />);

      const approveBtn = screen.getByRole("button", { name: "Approve" });
      fireEvent.click(approveBtn);

      expect(decide.mutate).toHaveBeenCalledWith(
        expect.objectContaining({
          id: prop.id,
          decision: "approve",
        }),
        expect.any(Object),
      );
    });

    it("executes Reject action", () => {
      const prop = makePublishProposal();
      renderWithClient(<PublishProposalView proposal={prop} canDecide={true} />);

      const rejectBtn = screen.getByRole("button", { name: "Reject" });
      fireEvent.click(rejectBtn);

      expect(decide.mutate).toHaveBeenCalledWith(
        expect.objectContaining({
          id: prop.id,
          decision: "reject",
        }),
        expect.any(Object),
      );
    });

    it("executes Request changes action with required note validation", async () => {
      const prop = makePublishProposal();
      renderWithClient(<PublishProposalView proposal={prop} canDecide={true} />);

      const askChangesBtn = screen.getByRole("button", { name: "Ask for changes" });

      // Click without note -> validation alert shown
      fireEvent.click(askChangesBtn);
      expect(decide.mutate).not.toHaveBeenCalled();
      expect(screen.getByRole("alert")).toHaveTextContent(
        "Write what should change first.",
      );

      // Enter note and click again
      const noteInput = screen.getByRole("textbox", { name: "Note (optional)" });
      await userEvent.type(noteInput, "Please add more details about performance benchmarks.");
      fireEvent.click(askChangesBtn);

      expect(decide.mutate).toHaveBeenCalledWith(
        expect.objectContaining({
          id: prop.id,
          decision: "revise",
          note: "Please add more details about performance benchmarks.",
        }),
        expect.any(Object),
      );
    });

    it("executes Snooze (Decide later) action with date picker", () => {
      const prop = makePublishProposal();
      renderWithClient(<PublishProposalView proposal={prop} canDecide={true} />);

      const snoozeBtn = screen.getByRole("button", { name: "Decide later" });
      fireEvent.click(snoozeBtn);

      // Date input appears with label "Until"
      const dateInput = screen.getByLabelText("Until");
      expect(dateInput).toBeInTheDocument();

      // Enter a valid future date
      const futureDate = "2026-11-25";
      fireEvent.change(dateInput, { target: { value: futureDate } });

      // Confirm snooze
      const confirmBtn = screen.getByRole("button", { name: "Set date" });
      fireEvent.click(confirmBtn);

      expect(decide.mutate).toHaveBeenCalledWith(
        expect.objectContaining({
          id: prop.id,
          decision: "snooze",
          until: futureDate,
        }),
        expect.any(Object),
      );
    });
  });

  describe("CompanyDecisionsDialog integration", () => {
    it("renders the publish proposal details and decision queue when opened", () => {
      const prop = makePublishProposal();
      const squads = [
        { id: "squad-cs", name: "Content Studio", archived_at: null },
      ];

      renderWithClient(
        <CompanyDecisionsDialog
          open={true}
          proposals={[prop]}
          squads={squads as any}
          canDecide={true}
          focusId={prop.id}
          onClose={vi.fn()}
        />,
      );

      expect(screen.getByRole("heading", { name: "Decisions" })).toBeInTheDocument();
      expect(screen.getAllByText("Launch announcement post").length).toBeGreaterThanOrEqual(1);
      expect(screen.getByText("Item: cs-item-42")).toBeInTheDocument();
      expect(screen.getByText("Market: International (intl)")).toBeInTheDocument();
      expect(screen.getByText("@multica_hq")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Approve" })).toBeInTheDocument();
    });
  });
});
