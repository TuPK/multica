import {
  kvLeadInstructions,
  kvRole,
  kvRoleSkills,
  kitvietSquad,
} from "../squads/kitviet-squad-roles";
import type { SquadRole, SquadRoleKey, SquadTemplate, SquadWorkflowTemplate } from "../squads/templates";
import { CATALOG_COLLECTIONS, type SquadTemplateCategory } from "./generated/collections";
import { CATALOG_SQUADS } from "./generated/squads";
import { skillsetMembers } from "./skillsets";
import type { CatalogSquad } from "./types";

export type { SquadTemplateId } from "./generated/squads";

/** The groups of the squad quickstart, in listed order (catalog/collections). */
export const SQUAD_TEMPLATE_CATEGORIES: readonly SquadTemplateCategory[] = CATALOG_COLLECTIONS.map((c) => c.id);

const isKitviet = (squad: CatalogSquad): boolean => squad.adapter?.type === "kitviet";

/**
 * Every role of every catalog squad. A Kit Viet role takes its name and agent kind from the generated
 * pack (kitviet-squad-roles.ts); any other role is written out in its squad's template.json. A role
 * several squads share (the two SDLC squads) is the same record in each, and is listed once.
 */
export function assembleSquadRoles(): Record<SquadRoleKey, SquadRole> {
  const roles: Record<string, SquadRole> = {};
  for (const catalogSquad of CATALOG_SQUADS) {
    for (const role of catalogSquad.squad.roles) {
      if (roles[role.key]) continue;
      roles[role.key] = isKitviet(catalogSquad)
        ? kvRole(role.key, { description: role.description, roster: role.role })
        : ({
            key: role.key,
            agentTemplate: role.agentTemplate,
            name: role.name,
            description: role.description,
            role: role.role,
          } as SquadRole);
    }
  }
  return roles as Record<SquadRoleKey, SquadRole>;
}

function unique(names: readonly string[]): string[] {
  return [...new Set(names)];
}

/**
 * The quickstart templates, in listed order. `workflows` resolves the workflow ids a manifest names;
 * they stay defined in templates.ts until the workflow kind moves into the catalog.
 */
export function assembleSquadTemplates(workflows: Readonly<Record<string, SquadWorkflowTemplate>>): SquadTemplate[] {
  return CATALOG_SQUADS.map((catalogSquad) => {
    const { squad } = catalogSquad;
    const kitviet = isKitviet(catalogSquad);
    const workflow = squad.workflow === null ? null : workflows[squad.workflow];
    if (workflow === undefined) throw new Error(`squad ${catalogSquad.id} names unknown workflow ${squad.workflow}`);

    const template: SquadTemplate = {
      category: catalogSquad.collection as SquadTemplateCategory,
      id: catalogSquad.id as SquadTemplate["id"],
      name: squad.name,
      description: squad.description,
      ...(catalogSquad.expertReview === undefined ? {} : { expertReview: catalogSquad.expertReview }),
      roles: squad.roles.map((role) => role.key as SquadRoleKey),
      leader: squad.leader as SquadRoleKey,
      floorName: squad.floorName,
      routines: squad.routines as SquadTemplate["routines"],
      charter: squad.charter as SquadTemplate["charter"],
      ...(squad.operatingReview === undefined ? {} : { operatingReview: squad.operatingReview }),
      instructions: kitviet ? kvLeadInstructions(squad.leader) : (squad.instructions ?? ""),
      workflow,
      builtinSkills: [],
    };

    if (kitviet) {
      const pack = kitvietSquad(catalogSquad.id);
      template.builtinSkills = pack.builtinSkills;
      template.leaderSkills = pack.leaderSkills;
      template.roleSkills = kvRoleSkills(catalogSquad.id, template.roles);
    } else {
      const skills = squad.skills ?? { sets: [] };
      template.builtinSkills = unique([...skills.sets.flatMap((set) => skillsetMembers(set)), ...(skills.extra ?? [])]);
      if (skills.leader) template.leaderSkills = skills.leader;
      if (skills.byRole) template.roleSkills = skills.byRole as SquadTemplate["roleSkills"];
    }
    return template;
  });
}
