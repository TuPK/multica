import { describe, expect, it } from "vitest";
import { screen } from "@testing-library/react";
import { renderWithI18n } from "../../test/i18n";
import { WorkflowGraphSheet } from "./workflow-graph-sheet";
import type { IssueWorkflow } from "@multica/core/types";

describe("WorkflowGraphSheet", () => {
  const sampleWorkflow: IssueWorkflow = {
    id: "wf-trade",
    name: "Trade · a1b2c3",
    description: "Kit Việt Trade import & export dossier flow",
    template_ref: "kv_trade@a1b2c3",
    steps: [
      {
        status_key: "collect",
        tasks: [
          { key: "docs_reader", title: "trade_docs_reader", kind: "work", handler: { type: "agent", id: "agent-trade_docs_reader" }, instructions: "", requires: ["attachment"] },
          {
            key: "customs_research",
            title: "trade_customs_research",
            kind: "work",
            handler: { type: "agent", id: "agent-customs" },
            instructions: "",
            requires: [],
            when: { field: "switch.customs_lookup", op: "eq", value: true },
          },
        ],
        transitions: [{ id: "t1", label: "Proceed to Draft", to_status_key: "draft", when: null }],
      },
      {
        status_key: "draft",
        tasks: [
          { key: "crosscheck_drafter", title: "trade_crosscheck_drafter", kind: "work", handler: { type: "agent", id: "agent-trade_crosscheck_drafter" }, instructions: "", requires: ["attachment"] },
        ],
        transitions: [{ id: "t2", label: "Submit to Gate", to_status_key: "gate" }],
      },
      {
        status_key: "gate",
        tasks: [
          { key: "docs_gate", title: "trade_docs_gate", kind: "gate", handler: { type: "agent", id: "agent-trade_docs_gate" }, instructions: "", requires: ["outcome"] },
          { key: "customs_gate", title: "trade_customs_gate", kind: "gate", handler: { type: "agent", id: "agent-trade_customs_gate" }, instructions: "", requires: ["outcome"] },
        ],
        transitions: [
          {
            id: "t3_rework",
            label: "Rejected -> Return to Draft",
            to_status_key: "draft",
            rework: true,
            when: {
              all: [
                { field: "tasks_outcome", op: "eq", value: "any_rejected" },
                { field: "rework_count", op: "lt", value: 2 },
              ],
            },
          },
          { id: "t3_pass", label: "Approved -> Handover", to_status_key: "handover", when: null },
        ],
      },
      {
        status_key: "in_review",
        human_only: true,
        tasks: [],
        transitions: [{ id: "t4", label: "Done", to_status_key: "done" }],
      },
    ],
  } as unknown as IssueWorkflow;

  it("renders workflow header and description when open", () => {
    renderWithI18n(
      <WorkflowGraphSheet
        workflow={sampleWorkflow}
        open={true}
        onOpenChange={() => {}}
      />
    );

    expect(screen.getByText("Trade · a1b2c3")).toBeInTheDocument();
    expect(screen.getByText("Kit Việt Trade import & export dossier flow")).toBeInTheDocument();
  });

  it("renders steps in sequential order with status chips", () => {
    renderWithI18n(
      <WorkflowGraphSheet
        workflow={sampleWorkflow}
        open={true}
        onOpenChange={() => {}}
      />
    );

    expect(screen.getAllByText("collect").length).toBeGreaterThan(0);
    expect(screen.getAllByText("draft").length).toBeGreaterThan(0);
    expect(screen.getAllByText("gate").length).toBeGreaterThan(0);
    expect(screen.getAllByText("in_review").length).toBeGreaterThan(0);
  });

  it("renders human-only indicator when step is human_only", () => {
    renderWithI18n(
      <WorkflowGraphSheet
        workflow={sampleWorkflow}
        open={true}
        onOpenChange={() => {}}
      />
    );

    expect(screen.getByText(/chỉ người|human only/i)).toBeInTheDocument();
  });

  it("renders tasks with gate/work kind chips, requires outcome/attachment, and when condition in words", () => {
    renderWithI18n(
      <WorkflowGraphSheet
        workflow={sampleWorkflow}
        open={true}
        onOpenChange={() => {}}
      />
    );

    // Gate & Work badges
    expect(screen.getAllByText(/gate/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/work/i).length).toBeGreaterThan(0);

    // Tasks
    expect(screen.getByText("docs_reader")).toBeInTheDocument();
    expect(screen.getByText("customs_research")).toBeInTheDocument();
    expect(screen.getByText("docs_gate")).toBeInTheDocument();

    // Requires
    expect(screen.getAllByText(/đính kèm|attachment/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/verdict|outcome/i).length).toBeGreaterThan(0);

    // When in words
    expect(screen.getByText(/customs_lookup/i)).toBeInTheDocument();

    // Rework limit
    expect(screen.getByText(/return up to 2 times|tối đa 2 lần/i)).toBeInTheDocument();
  });
});
