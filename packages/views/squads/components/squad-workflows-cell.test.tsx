import { describe, expect, it } from "vitest";
import { screen } from "@testing-library/react";
import type { Squad } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { SquadWorkflowsCell } from "./squad-workflows-cell";

describe("SquadWorkflowsCell", () => {
  it("renders empty dash when squad has no workflows", () => {
    const squad = {
      id: "squad-1",
      name: "Trade Squad",
      workflow_count: 0,
      active_workflow_id: null,
      active_workflow_name: null,
      flow_enforced: false,
    } as unknown as Squad;

    renderWithI18n(<SquadWorkflowsCell squad={squad} />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("renders active workflow name and workflow count", () => {
    const squad = {
      id: "squad-2",
      name: "Trade Squad",
      workflow_count: 3,
      active_workflow_id: "wf-1",
      active_workflow_name: "Import & Export Desk · a1b2c3",
      flow_enforced: false,
    } as unknown as Squad;

    renderWithI18n(<SquadWorkflowsCell squad={squad} />);
    expect(screen.getByText("Import & Export Desk · a1b2c3")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.queryByText(/có thực thi/i)).not.toBeInTheDocument();
  });

  it("renders 'Có thực thi' badge when flow_enforced is true", () => {
    const squad = {
      id: "squad-3",
      name: "Trade Squad",
      workflow_count: 1,
      active_workflow_id: "wf-1",
      active_workflow_name: "Import & Export Desk · a1b2c3",
      flow_enforced: true,
    } as unknown as Squad;

    renderWithI18n(<SquadWorkflowsCell squad={squad} />);
    expect(screen.getByText("Import & Export Desk · a1b2c3")).toBeInTheDocument();
    expect(screen.getByText(/có thực thi|enforced/i)).toBeInTheDocument();
  });
});
