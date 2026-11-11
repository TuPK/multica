// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { Agent, Project } from "../types";
import { assigneeSquadId, findSquadProject, openSquadProjects, projectHasSquad, projectSquadIds } from "./squad-project";

function project(id: string, squadId: string | null, status = "in_progress"): Project {
  return { id, squad_id: squadId, status } as Project;
}

function agent(id: string, homeSquadId: string | null): Agent {
  return { id, home_squad_id: homeSquadId } as Agent;
}

function sharedProject(id: string, squadIds: string[] | null, status = "in_progress"): Project {
  return { id, squad_id: squadIds?.[0] ?? null, squad_ids: squadIds, status } as Project;
}

describe("projectSquadIds", () => {
  it("reads every squad a project is assigned to", () => {
    expect(projectSquadIds(sharedProject("p1", ["s1", "s2"]))).toEqual(["s1", "s2"]);
  });

  it("falls back to squad_id from a server that does not send squad_ids", () => {
    expect(projectSquadIds(project("p1", "s1"))).toEqual(["s1"]);
    expect(projectSquadIds(project("p1", null))).toEqual([]);
  });

  it("answers membership for either squad", () => {
    const shared = sharedProject("p1", ["s1", "s2"]);
    expect(projectHasSquad(shared, "s2")).toBe(true);
    expect(projectHasSquad(shared, "s3")).toBe(false);
    expect(projectHasSquad(shared, null)).toBe(false);
  });
});

describe("findSquadProject", () => {
  it("finds a project that another room shares", () => {
    const projects = [sharedProject("p1", ["s1", "s2"])];
    expect(findSquadProject(projects, "s2")?.id).toBe("p1");
    expect(findSquadProject(projects, "s1")?.id).toBe("p1");
  });

  it("returns the open project assigned to the squad", () => {
    const projects = [project("p1", "s2"), project("p2", "s1")];
    expect(findSquadProject(projects, "s1")?.id).toBe("p2");
  });

  it("ignores finished projects, which keep no squad in the room", () => {
    const projects = [project("done", "s1", "completed"), project("gone", "s1", "cancelled")];
    expect(findSquadProject(projects, "s1")).toBeUndefined();
  });

  it("returns nothing without a squad", () => {
    expect(findSquadProject([project("p1", "s1")], null)).toBeUndefined();
    expect(findSquadProject([project("p1", "s1")], undefined)).toBeUndefined();
  });
});

describe("openSquadProjects", () => {
  it("keeps a shared project that has more than one squad", () => {
    expect(openSquadProjects([sharedProject("p1", ["s1", "s2"])]).map((p) => p.id)).toEqual(["p1"]);
  });

  it("keeps open projects that have a squad to assign", () => {
    const projects = [
      project("p1", "s1"),
      project("unassigned", null),
      project("done", "s1", "completed"),
    ];
    expect(openSquadProjects(projects).map((p) => p.id)).toEqual(["p1"]);
  });
});

describe("assigneeSquadId", () => {
  const agents = [agent("a1", "s1"), agent("a2", null)];

  it("names the squad an agent assignee is seated in", () => {
    expect(assigneeSquadId("agent", "a1", agents)).toBe("s1");
  });

  it("uses the squad itself when the assignee is a squad", () => {
    expect(assigneeSquadId("squad", "s9", agents)).toBe("s9");
  });

  it("is null for an agent with no home squad, a member, or no assignee", () => {
    expect(assigneeSquadId("agent", "a2", agents)).toBeNull();
    expect(assigneeSquadId("member", "u1", agents)).toBeNull();
    expect(assigneeSquadId(null, null, agents)).toBeNull();
  });
});
