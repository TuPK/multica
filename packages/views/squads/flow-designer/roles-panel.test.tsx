// @vitest-environment jsdom

import React from "react";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlowDoc } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { RolesPanel } from "./roles-panel";

vi.mock("@multica/core/paths", () => ({ useWorkspacePaths: () => ({ squadDetail: (id: string) => `/acme/squads/${id}` }) }));
vi.mock("../../navigation", () => ({
  AppLink: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a>,
}));

const DOC = {
  version: 2,
  start: "a",
  roles: [
    { key: "worker", label: "Worker", kind: "agent" },
    { key: "spare", label: "Spare", kind: "agent" },
  ],
  steps: [{ key: "a", title: "A", kind: "tasks", tasks: [{ key: "t1", title: "T1", kind: "work", role: "worker" }], edges: [] }],
  terminals: [],
} as unknown as FlowDoc;

afterEach(cleanup);

describe("RolesPanel delete", () => {
  it("shows Delete only on a role that no task uses, and Delete takes it out", () => {
    const onChangeRoles = vi.fn();
    renderWithI18n(<RolesPanel doc={DOC} onChangeRoles={onChangeRoles} />);
    expect(screen.queryByRole("button", { name: "Delete worker" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Delete spare" }));
    expect(onChangeRoles).toHaveBeenCalledWith([DOC.roles![0]]);
  });
});

describe("RolesPanel choices", () => {
  it("lists only the agents it is given (the squad's members) for an agent role", () => {
    renderWithI18n(<RolesPanel doc={DOC} onChangeRoles={vi.fn()} agentChoices={[{ id: "ag1", name: "Writer agent" }]} />);
    expect(screen.getAllByRole("checkbox", { name: "Writer agent" }).length).toBe(2);
    expect(screen.queryByRole("checkbox", { name: "Outsider agent" })).toBeNull();
  });
});
