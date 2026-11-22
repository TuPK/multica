"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ALL_STATUSES,
  BUILT_IN_STATUS_CATEGORY,
  BUILT_IN_STATUS_ORDER,
} from "@multica/core/issues/config";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import {
  defaultWorkflow,
  issueRoomWorkflow,
  useIssueWorkflows,
  useRoomRoster,
} from "@multica/core/issue-workflows";
import type { RoomIssueRef } from "@multica/core/issue-workflows/queries";
import {
  issueStatusColor,
  normalizeIssueStatusCategory,
} from "@multica/core/issue-statuses/queries";
import { projectListOptions } from "@multica/core/projects/queries";
import type { IssueStatus, IssueStatusCategory, IssueWorkflow, Project } from "@multica/core/types";
import { useStatusLabel } from "./status-label";

export interface StatusOption {
  key: IssueStatus;
  /** Lifecycle category; not the custom status's icon or automation behavior. */
  category: IssueStatusCategory;
  label: string;
  /** `#rrggbb` for a custom status; null for a built-in, which keeps its token color. */
  color: string | null;
  icon?: string | null;
}

const NO_ARCHIVED_STATUS_KEYS: readonly IssueStatus[] = [];

export function useStatusOptions(
  wsId: string,
  includeArchivedKeys: readonly IssueStatus[] = NO_ARCHIVED_STATUS_KEYS,
): StatusOption[] {
  const { statuses } = useIssueStatuses(wsId);
  const labelOf = useStatusLabel(wsId);

  return useMemo(
    () => {
      const includedArchived = new Set(includeArchivedKeys);
      return ALL_STATUSES.flatMap<StatusOption>((category) => {
        const entries = statuses.filter(
          (entry) =>
            normalizeIssueStatusCategory(entry.category) === category &&
            (!entry.archived_at || includedArchived.has(entry.key)),
        );
        if (entries.length === 0) {
          return BUILT_IN_STATUS_ORDER.filter(
            (key) => BUILT_IN_STATUS_CATEGORY[key] === category,
          ).map((key) => ({
            key,
            category,
            label: labelOf(key),
            color: null,
          }));
        }
        return entries.map((e) => ({
          key: e.key as IssueStatus,
          category,
          label: labelOf(e.key),
          color: issueStatusColor(e),
          icon: e.icon,
        }));
      });
    },
    [includeArchivedKeys, labelOf, statuses],
  );
}

const CATEGORY_RANK = new Map<IssueStatusCategory, number>(ALL_STATUSES.map((c, i) => [c, i]));

/**
 * The statuses an issue may take: its room workflow's steps in workflow order,
 * or Default outside any project (or when the issue has no room).
 *
 * `projectId` is three-valued:
 * - a project id: the room workflow resolved for `issue` (or Default if no room);
 * - `null`: an issue outside any project. Offers workspace Default's statuses
 *   without handoffs (`workflow` stays null, `defaultScope` names Default);
 * - `undefined`: a multi-issue / multi-project surface; the whole catalog
 *   (narrowed by `allowedStatuses` when provided).
 */
export function useIssueStatusOptions(
  wsId: string,
  projectId: string | null | undefined,
  currentStatus?: IssueStatus | null,
  issue?: Partial<RoomIssueRef> | null,
): {
  options: StatusOption[];
  workflow: IssueWorkflow | null;
  project: Project | null;
  defaultScope: IssueWorkflow | null;
} {
  const catalogOptions = useStatusOptions(wsId);
  const { categoryOf, colorOf, iconOf } = useIssueStatuses(wsId);
  const labelOf = useStatusLabel(wsId);
  const { workflows } = useIssueWorkflows(wsId);
  const roster = useRoomRoster(wsId);
  const { data: projects } = useQuery({
    ...projectListOptions(wsId),
    enabled: Boolean(wsId) && Boolean(projectId),
  });

  const project = useMemo(
    () => (projectId ? projects?.find((p) => p.id === projectId) ?? null : null),
    [projectId, projects],
  );
  const workspaceDefault = useMemo(() => defaultWorkflow(workflows), [workflows]);
  const workflow = useMemo(() => {
    if (!projectId) return null;
    return issueRoomWorkflow(workflows, roster, {
      project_id: projectId,
      assignee_type: issue?.assignee_type ?? null,
      assignee_id: issue?.assignee_id ?? null,
    });
  }, [issue?.assignee_id, issue?.assignee_type, projectId, roster, workflows]);

  const defaultScope = projectId === null && !workflow ? workspaceDefault : null;
  const options = useMemo<StatusOption[]>(() => {
    const listed = workflow ?? defaultScope;
    if (!listed) return catalogOptions;
    const byKey = new Map(catalogOptions.map((o) => [o.key, o]));
    const optionOf = (key: IssueStatus): StatusOption =>
      byKey.get(key) ?? {
        key,
        category: categoryOf(key),
        label: labelOf(key),
        color: colorOf(key),
        icon: iconOf(key),
      };
    const steps = listed.steps.map((step) => optionOf(step.status_key));
    if (!listed.is_default || !currentStatus || steps.some((o) => o.key === currentStatus)) return steps;
    // Keep a current status Default leaves out visible and checked, after the
    // last step of its category.
    const current = optionOf(currentStatus);
    const rank = CATEGORY_RANK.get(current.category) ?? ALL_STATUSES.length;
    let at = steps.length;
    while (at > 0 && (CATEGORY_RANK.get(steps[at - 1]!.category) ?? 0) > rank) at -= 1;
    return [...steps.slice(0, at), current, ...steps.slice(at)];
  }, [catalogOptions, categoryOf, colorOf, currentStatus, defaultScope, iconOf, labelOf, workflow]);
  return { options, workflow, project, defaultScope };
}
