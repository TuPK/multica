"use client";

import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { agentTaskSnapshotOptions } from "@multica/core/agents";
import { useWorkspaceId } from "@multica/core/hooks";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import {
  resolveProjectWorkflow,
  stepHandsOff,
  useIssueWorkflows,
  workflowAllowsStatus,
  workflowStep,
} from "@multica/core/issue-workflows";
import { useViewStore } from "@multica/core/issues/stores/view-store-context";
import { projectListOptions } from "@multica/core/projects/queries";
import type { AgentTask, Issue, IssueStatus, UpdateIssueRequest } from "@multica/core/types";
import { statusCategoryOfKey } from "@multica/core/issues";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";
import { BoardView, type BoardViewProps } from "../../issues/components/board-view";
import { useIssueSurfaceActionsOptional } from "../../issues/surface/actions-context";
import { useIssuePeekActions } from "../../issues/surface/peek-context";
import type { DragMoveUpdates } from "../../issues/utils/drag-utils";
import { useStatusLabel } from "../../issues/utils/status-label";
import { WorkflowHandoffConfirmDialog } from "../../workflows/handoff-confirm-dialog";
import { officeCollision, type OfficeDragData } from "../lib/office-dnd";
import { lookForActor } from "../lib/pixel-sprites";
import { planRoomMove } from "../lib/plan-move";
import { roomColumns, roomLookFor } from "../lib/room-theme";
import { OfficeCorridor } from "./office-corridor";
import { OfficeRoom, type RoomDropIntent } from "./office-room";
import { OfficeWalkers, useOfficeWalkers } from "./office-walkers";
import { PixelSprite } from "./pixel-sprite";

const WALL = "#343e58";
const EMPTY_TASKS: AgentTask[] = [];
const EMPTY_ISSUES: Issue[] = [];

interface PendingHandoff {
  issue: Issue;
  status: IssueStatus;
  updates: DragMoveUpdates;
}

/**
 * The board, as an office: every status is a room and every issue a worker at
 * a desk. Rooms stand for statuses, so any other grouping falls back to the
 * classic board instead of inventing rooms for assignees or projects.
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

function OfficeScene({
  issues,
  visibleStatuses,
  onMoveIssue,
  onCreateIssue,
  statusPagination,
}: BoardViewProps) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const catalog = useIssueStatuses(wsId);
  const labelOf = useStatusLabel(wsId);
  const sortBy = useViewStore((s) => s.sortBy);
  const surfaceActions = useIssueSurfaceActionsOptional();
  const peek = useIssuePeekActions();

  const issuesByStatus = useMemo(() => {
    const map = new Map<string, Issue[]>();
    for (const status of visibleStatuses) map.set(status, []);
    for (const issue of issues) map.get(issue.status)?.push(issue);
    return map;
  }, [issues, visibleStatuses]);
  const issueMap = useMemo(() => new Map(issues.map((issue) => [issue.id, issue])), [issues]);

  const { data: tasks = EMPTY_TASKS } = useQuery(agentTaskSnapshotOptions(wsId));
  const workingIds = useMemo(
    () => new Set(tasks.filter((task) => task.status === "running" && task.issue_id).map((task) => task.issue_id)),
    [tasks],
  );

  // A project's workflow limits which rooms an issue may enter.
  const { workflows } = useIssueWorkflows(wsId);
  const { data: projects } = useQuery(projectListOptions(wsId));
  const workflowOfIssue = useCallback(
    (issue: Issue | undefined) =>
      issue?.project_id
        ? resolveProjectWorkflow(workflows, projects?.find((p) => p.id === issue.project_id))
        : null,
    [workflows, projects],
  );

  const [active, setActive] = useState<OfficeDragData | null>(null);
  const [pendingHandoff, setPendingHandoff] = useState<PendingHandoff | null>(null);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));

  const contentRef = useRef<HTMLDivElement>(null);
  const laneRef = useRef<HTMLDivElement>(null);
  const { walkers, walkingIds, registerDesk, finish } = useOfficeWalkers({
    contentRef,
    laneRef,
    issues,
  });

  // The side peek steps through a room with J / K, in the order shown here.
  useEffect(() => {
    peek?.publishColumns(
      visibleStatuses.map((status) => (issuesByStatus.get(status) ?? []).map((issue) => issue.id)),
    );
  }, [peek, visibleStatuses, issuesByStatus]);
  useEffect(() => () => peek?.publishColumns(null), [peek]);

  const blockedReason = useCallback(
    (issue: Issue | undefined, status: string): string | null => {
      const workflow = workflowOfIssue(issue);
      if (!issue || !workflow || issue.status === status || workflowAllowsStatus(workflow, status)) {
        return null;
      }
      return t(($) => $.workflows.not_in_workflow, { name: workflow.name });
    },
    [t, workflowOfIssue],
  );

  const intentFor = useCallback(
    (status: string): RoomDropIntent => {
      if (!active) return { kind: "none" };
      if (active.type === "actor") return { kind: "create" };
      return { kind: "move", blockedReason: blockedReason(issueMap.get(active.issueId), status) };
    },
    [active, blockedReason, issueMap],
  );

  const moveToRoom = useCallback(
    (issueId: string, status: IssueStatus) => {
      const issue = issueMap.get(issueId);
      if (!issue || issue.status === status) return;
      if (catalog.entryOf(status)?.archived_at) return;
      const workflow = workflowOfIssue(issue);
      if (workflow && !workflowAllowsStatus(workflow, status)) {
        toast.info(t(($) => $.workflows.drop_blocked, { status: labelOf(status), name: workflow.name }));
        return;
      }
      const updates = planRoomMove({
        issue,
        status,
        targetIds: (issuesByStatus.get(status) ?? []).map((i) => i.id),
        manualOrder: sortBy === "position",
        issueMap,
      });
      // Entering a step that hands the issue off is confirmed first.
      if (workflow && stepHandsOff(workflowStep(workflow, status))) {
        setPendingHandoff({ issue, status, updates });
        return;
      }
      onMoveIssue(issueId, updates);
    },
    [catalog, issueMap, issuesByStatus, labelOf, onMoveIssue, sortBy, t, workflowOfIssue],
  );

  const assign = useCallback(
    (issueId: string, actor: Extract<OfficeDragData, { type: "actor" }>) => {
      const updates: Partial<UpdateIssueRequest> = {
        assignee_type: actor.actorType,
        assignee_id: actor.actorId,
      };
      surfaceActions?.updateIssue(issueId, updates, { errorMessage: t(($) => $.card.update_failed) });
    },
    [surfaceActions, t],
  );

  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActive((event.active.data.current as OfficeDragData | undefined) ?? null);
  }, []);

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      setActive(null);
      const dragged = event.active.data.current as OfficeDragData | undefined;
      const target = event.over?.data.current as
        | { type: "room"; status: string }
        | { type: "desk"; issueId: string; status: string }
        | undefined;
      if (!dragged || !target) return;
      if (dragged.type === "issue") {
        moveToRoom(dragged.issueId, target.status);
      } else if (target.type === "desk") {
        assign(target.issueId, dragged);
      } else {
        onCreateIssue?.({
          status: target.status,
          assignee_type: dragged.actorType,
          assignee_id: dragged.actorId,
        });
      }
    },
    [assign, moveToRoom, onCreateIssue],
  );

  const handleCreate = useCallback(
    (status: string) => onCreateIssue?.({ status }),
    [onCreateIssue],
  );

  const activeIssue = active?.type === "issue" ? issueMap.get(active.issueId) : undefined;

  return (
    <>
      <DndContext
        sensors={sensors}
        collisionDetection={officeCollision}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
        onDragCancel={() => setActive(null)}
      >
        <div
          data-office-scroller=""
          className="flex min-h-0 flex-1 overflow-x-auto overflow-y-hidden p-2 group-data-[peek-open]/peek:after:w-(--issue-peek-width) group-data-[peek-open]/peek:after:shrink-0 group-data-[peek-open]/peek:after:content-['']"
        >
          <div
            ref={contentRef}
            className="relative flex min-h-[520px] min-w-max flex-col overflow-hidden rounded-md shadow-sm"
            style={{ background: WALL }}
          >
            {visibleStatuses.length === 0 ? (
              <div className="flex flex-1 items-center justify-center p-6 text-body text-white/80">
                {t(($) => $.board.empty_grouping)}
              </div>
            ) : (
              <div className="flex min-h-0 flex-1 gap-1" style={{ background: WALL }}>
                {visibleStatuses.map((status) => {
                  const category = catalog.entryOf(status)?.category ?? statusCategoryOfKey(status);
                  const roomIssues = issuesByStatus.get(status) ?? EMPTY_ISSUES;
                  const page = statusPagination?.[status];
                  return (
                    <OfficeRoom
                      key={status}
                      status={status}
                      label={labelOf(status)}
                      look={roomLookFor(status, category)}
                      columns={roomColumns(status)}
                      issues={roomIssues}
                      total={page?.total ?? roomIssues.length}
                      page={page}
                      workingIds={workingIds}
                      walkingIds={walkingIds}
                      intent={intentFor(status)}
                      registerDesk={registerDesk}
                      onCreate={onCreateIssue ? handleCreate : undefined}
                    />
                  );
                })}
              </div>
            )}
            <OfficeCorridor laneRef={laneRef} tasks={tasks} />
            <OfficeWalkers walkers={walkers} onDone={finish} />
          </div>
        </div>

        <DragOverlay dropAnimation={null}>
          {active?.type === "actor" ? (
            <div className="flex cursor-grabbing flex-col items-center gap-0.5 rounded bg-background/90 px-2 py-1 shadow-lg">
              <PixelSprite look={lookForActor(active.actorType, active.actorId)} pose="stand" />
              <span className="text-micro font-medium">{active.name}</span>
            </div>
          ) : activeIssue ? (
            <div
              className="w-[160px] cursor-grabbing rounded-[3px] bg-background px-2 py-1.5 shadow-lg ring-1 ring-foreground/10"
            >
              <p className="font-mono text-micro text-muted-foreground">{activeIssue.identifier}</p>
              <p className="line-clamp-2 text-caption font-medium">{activeIssue.title}</p>
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>
      <WorkflowHandoffConfirmDialog
        issue={pendingHandoff?.issue ?? null}
        toStatus={pendingHandoff?.status ?? ""}
        onCancel={() => setPendingHandoff(null)}
        onConfirm={({ stopPreviousRuns }) => {
          const pending = pendingHandoff;
          setPendingHandoff(null);
          if (!pending) return;
          onMoveIssue(pending.issue.id, {
            ...pending.updates,
            ...(stopPreviousRuns ? { stop_previous_assignee_runs: true } : {}),
          });
        }}
      />
    </>
  );
}
