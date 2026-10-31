"use client";

import { memo, useCallback, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { agentTaskSnapshotOptions } from "@multica/core/agents";
import { useWorkspaceId } from "@multica/core/hooks";
import type { AgentTask, Issue } from "@multica/core/types";
import { useViewStore } from "@multica/core/issues/stores/view-store-context";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";
import { BoardView, type BoardViewProps } from "../../issues/components/board-view";
import { useIssuePeekActions } from "../../issues/surface/peek-context";
import { useStatusLabel } from "../../issues/utils/status-label";
import { OfficeCanvas, type WorkerSession } from "./office-canvas";
import { OfficeStorage } from "./office-storage";

const EMPTY_TASKS: AgentTask[] = [];

/**
 * The board, drawn as the pixtuoid office. Each issue that has an assignee is a
 * worker at a desk: the tag over the worker names the issue's status, and a click
 * opens the issue in peek. Any other grouping falls back to the classic board.
 */
export const OfficeView = memo(function OfficeView(props: BoardViewProps) {
  const grouping = useViewStore((s) => s.grouping);
  const setGrouping = useViewStore((s) => s.setGrouping);
  const { t } = useT("issues");

  if (grouping !== "status") {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <div className="mx-4 mb-2 flex shrink-0 items-center gap-2 rounded-lg bg-muted/50 px-3 py-1.5 text-caption text-muted-foreground">
          <span className="min-w-0 flex-1">{t(($) => $.office.grouping_notice)}</span>
          <Button type="button" variant="outline" size="xs" onClick={() => setGrouping("status")}>
            {t(($) => $.office.grouping_reset)}
          </Button>
        </div>
        <BoardView {...props} />
      </div>
    );
  }
  return <OfficeScene {...props} />;
});

function OfficeScene({ issues, visibleStatuses }: BoardViewProps) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const labelOf = useStatusLabel(wsId);
  const peek = useIssuePeekActions();
  const { data: tasks = EMPTY_TASKS } = useQuery(agentTaskSnapshotOptions(wsId));

  // The project's agents: every agent assigned to one of its issues, each seated once.
  const agentIds = useMemo(() => {
    const ids = new Set<string>();
    for (const issue of issues) {
      if (issue.assignee_type === "agent" && issue.assignee_id) ids.add(issue.assignee_id);
    }
    return [...ids].sort();
  }, [issues]);

  const workingAgents = useMemo(
    () => new Set(tasks.filter((task) => task.status === "running").map((task) => task.agent_id)),
    [tasks],
  );

  // What each agent shows: its issue in progress, else its first open issue. Done issues are not shown.
  const focusOf = useMemo(() => {
    const map = new Map<string, Issue>();
    const shown = new Set(visibleStatuses);
    for (const issue of issues) {
      if (!issue.assignee_id || issue.assignee_type !== "agent") continue;
      if (issue.status === "done" || !shown.has(issue.status)) continue;
      const current = map.get(issue.assignee_id);
      if (!current || (issue.status === "in_progress" && current.status !== "in_progress")) {
        map.set(issue.assignee_id, issue);
      }
    }
    return map;
  }, [issues, visibleStatuses]);

  const sessions = useMemo<WorkerSession[]>(
    () => agentIds.map((id) => ({ key: id, working: workingAgents.has(id) })),
    [agentIds, workingAgents],
  );

  const tagText = useCallback(
    (key: string) => {
      const issue = focusOf.get(key);
      return issue ? labelOf(issue.status) : t(($) => $.office.idle);
    },
    [focusOf, labelOf, t],
  );

  const onPick = useCallback(
    (agentId: string) => {
      const issue = focusOf.get(agentId);
      if (issue) peek?.open(issue.id);
    },
    [focusOf, peek],
  );

  const roomNames = useMemo(
    () => [t(($) => $.office.room_product), t(($) => $.office.room_research)],
    [t],
  );

  const onOpenIssue = useCallback((issueId: string) => peek?.open(issueId), [peek]);

  if (visibleStatuses.length === 0) return <OfficeEmpty />;
  return (
    <OfficeCanvas sessions={sessions} tagText={tagText} onPick={onPick} roomNames={roomNames}>
      <OfficeStorage issues={issues} onOpen={onOpenIssue} />
    </OfficeCanvas>
  );
}

/** Shown when no status is visible, so the office has no floor to draw. */
function OfficeEmpty() {
  const { t } = useT("issues");
  return (
    <div className="flex flex-1 items-center justify-center p-6 text-body text-white/80">
      {t(($) => $.board.empty_grouping)}
    </div>
  );
}
