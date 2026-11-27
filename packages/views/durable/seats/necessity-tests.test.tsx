/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { api, ApiError } from "@multica/core/api";
import { configStore } from "@multica/core/config";
import type { SkillSummary } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { NecessityTests } from "./necessity-tests";

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: {
    request: vi.fn(),
    listAgentSkills: vi.fn(),
  },
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const request = vi.mocked(api.request);
const listAgentSkills = vi.mocked(api.listAgentSkills);

function makeSkill(id: string, name: string): SkillSummary {
  return {
    id,
    workspace_id: "ws-1",
    name,
    description: "",
    config: {},
    created_by: null,
    created_at: "2026-11-01T00:00:00Z",
    updated_at: "2026-11-01T00:00:00Z",
  };
}

function renderSection() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <NecessityTests wsId="ws-1" agentId="a-1" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  configStore.getState().setFeatureFlags({ durable_seats: true });
  request.mockReset();
  listAgentSkills.mockReset();
  listAgentSkills.mockResolvedValue([makeSkill("sk-1", "Deploy"), makeSkill("sk-2", "SQL review")]);
  vi.mocked(toast.success).mockReset();
  vi.mocked(toast.error).mockReset();
});

afterEach(() => {
  cleanup();
  configStore.getState().setFeatureFlags({});
});

describe("NecessityTests", () => {
  it("renders tests with their state words, sample counts, and proposal decision note", async () => {
    request.mockResolvedValueOnce({
      tests: [
        {
          id: "nt-1",
          subject_type: "skill",
          subject_ref: "sk-1",
          agent_id: "a-1",
          shadow_run_id: "sr-1",
          state: "not_needed",
          result: { pairs: 40, primary: 2, shadow: 6, tie: 32 },
          proposal_id: "prop-1",
          created_at: "2026-11-26T09:00:00Z",
          completed_at: "2026-11-27T01:00:00Z",
        },
        {
          id: "nt-2",
          subject_type: "skill",
          subject_ref: "legacy-skill",
          agent_id: "a-1",
          state: "needed",
          result: { pairs: 25, primary: 9, shadow: 1, tie: 15 },
          created_at: "2026-11-25T09:00:00Z",
          completed_at: "2026-11-25T15:00:00Z",
        },
        {
          id: "nt-3",
          subject_type: "skill",
          subject_ref: "sk-2",
          agent_id: "a-1",
          state: "running",
          result: {},
          created_at: "2026-11-27T02:00:00Z",
        },
        {
          id: "nt-4",
          subject_type: "skill",
          subject_ref: "lint-check",
          agent_id: "a-1",
          state: "inconclusive",
          result: { pairs: 40, primary: 4, shadow: 3, tie: 33 },
          created_at: "2026-11-24T09:00:00Z",
        },
        {
          id: "nt-5",
          subject_type: "skill",
          subject_ref: "old-helper",
          agent_id: "a-1",
          state: "stopped",
          result: {},
          created_at: "2026-11-23T09:00:00Z",
        },
      ],
    });

    renderSection();

    expect(await screen.findByText("Deploy")).toBeInTheDocument();
    expect(screen.getByText("Not needed")).toBeInTheDocument();
    expect(screen.getByText("Waiting for a person's decision")).toBeInTheDocument();
    expect(screen.getByText("Wins: 6")).toBeInTheDocument();
    expect(screen.getByText("Losses: 2")).toBeInTheDocument();

    expect(screen.getByText("legacy-skill")).toBeInTheDocument();
    expect(screen.getByText("Needed")).toBeInTheDocument();
    expect(screen.getByText("Pairs: 25")).toBeInTheDocument();
    expect(screen.getByText("Wins: 1")).toBeInTheDocument();
    expect(screen.getByText("Losses: 9")).toBeInTheDocument();

    expect(screen.getByText("SQL review")).toBeInTheDocument();
    expect(screen.getByText("Running")).toBeInTheDocument();
    expect(screen.getByText("Inconclusive")).toBeInTheDocument();
    expect(screen.getByText("Stopped")).toBeInTheDocument();
    expect(request).toHaveBeenCalledWith("/api/agents/a-1/necessity");
  });

  it("renders an empty state when the seat has no necessity tests", async () => {
    request.mockResolvedValueOnce({ tests: [] });
    renderSection();

    expect(await screen.findByText("No necessity tests yet.")).toBeInTheDocument();
  });

  it("renders an error alert when loading fails", async () => {
    request.mockRejectedValueOnce(new Error("boom"));
    renderSection();

    expect(await screen.findByRole("alert")).toHaveTextContent("Could not load necessity tests.");
  });

  it("lists the seat's skills in the start dialog, shows the spec note, and posts skill_id", async () => {
    const user = userEvent.setup({ delay: null });
    request.mockImplementation(async (_path, init) => {
      if (init?.method === "POST") {
        return {
          id: "nt-new",
          subject_type: "skill",
          subject_ref: "sk-1",
          agent_id: "a-1",
          state: "running",
          result: {},
          created_at: "2026-11-27T03:00:00Z",
        };
      }
      return { tests: [] };
    });

    renderSection();
    await screen.findByText("No necessity tests yet.");

    await user.click(screen.getByRole("button", { name: "Start necessity test" }));

    expect(
      await screen.findByText(
        "A test runs the seat's work without the skill in a shadow trial, and the skill is removed only after a person approves the proposal.",
      ),
    ).toBeInTheDocument();

    const submitButton = screen.getByRole("button", { name: "Start test" });
    expect(submitButton).toBeDisabled();

    await user.click(screen.getByRole("combobox", { name: "Skill" }));
    expect(await screen.findByRole("option", { name: "Deploy" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "SQL review" })).toBeInTheDocument();

    await user.click(screen.getByRole("option", { name: "Deploy" }));
    expect(submitButton).toBeEnabled();

    await user.click(submitButton);

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "/api/agents/a-1/necessity",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ skill_id: "sk-1" }),
        }),
      ),
    );
    expect(toast.success).toHaveBeenCalledWith("Necessity test started.");
  });

  it("disables submitting while a test on that skill is running", async () => {
    const user = userEvent.setup({ delay: null });
    request.mockResolvedValueOnce({
      tests: [
        {
          id: "nt-run",
          subject_type: "skill",
          subject_ref: "sk-1",
          agent_id: "a-1",
          state: "running",
          result: {},
          created_at: "2026-11-27T03:00:00Z",
        },
      ],
    });

    renderSection();
    await screen.findByText("Deploy");

    await user.click(screen.getByRole("button", { name: "Start necessity test" }));
    await user.click(screen.getByRole("combobox", { name: "Skill" }));
    await user.click(await screen.findByRole("option", { name: "Deploy" }));

    expect(screen.getByRole("button", { name: "Start test" })).toBeDisabled();
    expect(
      screen.getByText("A necessity test for this skill is already running."),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: "Skill" }));
    await user.click(await screen.findByRole("option", { name: "SQL review" }));
    expect(screen.getByRole("button", { name: "Start test" })).toBeEnabled();
  });

  it("shows mapped error sentences for necessity_test_running and shadow_already_running", async () => {
    const user = userEvent.setup({ delay: null });
    request.mockImplementation(async (_path, init) => {
      if (init?.method === "POST") {
        throw new ApiError("running", 409, "Conflict", { code: "necessity_test_running" });
      }
      return { tests: [] };
    });

    renderSection();
    await screen.findByText("No necessity tests yet.");

    await user.click(screen.getByRole("button", { name: "Start necessity test" }));
    await user.click(screen.getByRole("combobox", { name: "Skill" }));
    await user.click(await screen.findByRole("option", { name: "Deploy" }));
    await user.click(screen.getByRole("button", { name: "Start test" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "A necessity test for this skill is already running.",
    );

    request.mockImplementation(async (_path, init) => {
      if (init?.method === "POST") {
        throw new ApiError("shadow running", 409, "Conflict", { code: "shadow_already_running" });
      }
      return { tests: [] };
    });

    await user.click(screen.getByRole("button", { name: "Start test" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "A shadow trial is already running on this seat.",
    );
  });

  it("is hidden and sends no requests while the durable_seats flag is off", () => {
    configStore.getState().setFeatureFlags({ durable_seats: false });
    const { container } = renderSection();

    expect(container).toBeEmptyDOMElement();
    expect(request).not.toHaveBeenCalled();
    expect(listAgentSkills).not.toHaveBeenCalled();
  });
});
