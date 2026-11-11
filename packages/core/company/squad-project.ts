import type { Agent, Project } from "../types";

/**
 * The squads a project is assigned to. Several rooms can work one project; an
 * older server sends only squad_id, which then stands for the one squad.
 */
export function projectSquadIds(project: Pick<Project, "squad_id" | "squad_ids">): string[] {
  if (project.squad_ids) return project.squad_ids;
  return project.squad_id ? [project.squad_id] : [];
}

/** Whether the project is assigned to this squad. */
export function projectHasSquad(project: Pick<Project, "squad_id" | "squad_ids">, squadId: string | null | undefined): boolean {
  return !!squadId && projectSquadIds(project).includes(squadId);
}

/**
 * Projects that can still be assigned a squad: those with a squad and not yet
 * finished. Finished projects keep no squad in the room, so they never count.
 */
export function openSquadProjects(projects: Project[]): Project[] {
  return projects.filter((p) => projectSquadIds(p).length > 0 && p.status !== "completed" && p.status !== "cancelled");
}

/**
 * The project a squad is working on: the one assigned to it that is still open.
 */
export function findSquadProject(projects: Project[], squadId: string | null | undefined): Project | undefined {
  if (!squadId) return undefined;
  return openSquadProjects(projects).find((p) => projectHasSquad(p, squadId));
}

/**
 * The squad whose project an issue inherits. A squad assignee names its own squad;
 * an agent assignee uses the agent's home squad, the one that seats it in a room.
 */
export function assigneeSquadId(
  assigneeType: string | null | undefined,
  assigneeId: string | null | undefined,
  agents: Agent[],
): string | null {
  if (!assigneeId) return null;
  if (assigneeType === "squad") return assigneeId;
  if (assigneeType === "agent") return agents.find((agent) => agent.id === assigneeId)?.home_squad_id ?? null;
  return null;
}
