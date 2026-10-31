"use client";

import { memo, useMemo, type RefObject } from "react";
import { useDraggable } from "@dnd-kit/core";
import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import type { AgentTask } from "@multica/core/types";
import { useViewStore } from "@multica/core/issues/stores/view-store-context";
import { agentListOptions, memberListOptions } from "@multica/core/workspace/queries";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { canAssignAgent } from "../../issues/components/pickers";
import type { OfficeDragData } from "../lib/office-dnd";
import { lookForActor } from "../lib/pixel-sprites";
import { PixelSprite } from "./pixel-sprite";

const CORRIDOR_HEIGHT = 120;

interface StaffEntry {
  type: "member" | "agent";
  id: string;
  name: string;
  running: number;
  assignable: boolean;
}

function runningByAgent(tasks: readonly AgentTask[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const task of tasks) {
    if (task.status !== "running" || !task.issue_id) continue;
    counts.set(task.agent_id, (counts.get(task.agent_id) ?? 0) + 1);
  }
  return counts;
}

function StaffMember({
  entry,
  active,
  onToggle,
}: {
  entry: StaffEntry;
  active: boolean;
  onToggle: (entry: StaffEntry) => void;
}) {
  const { t } = useT("issues");
  const data: OfficeDragData = {
    type: "actor",
    actorType: entry.type,
    actorId: entry.id,
    name: entry.name,
  };
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: `actor:${entry.type}:${entry.id}`,
    data,
    disabled: !entry.assignable,
  });
  const look = useMemo(() => lookForActor(entry.type, entry.id), [entry.type, entry.id]);

  return (
    <button
      ref={setNodeRef}
      type="button"
      {...attributes}
      {...listeners}
      aria-pressed={active}
      aria-label={t(($) => $.office.staff_filter, { name: entry.name })}
      onClick={() => onToggle(entry)}
      className={cn(
        "relative flex w-[80px] shrink-0 flex-col items-center gap-0.5 rounded-sm px-1 pb-1 pt-2 outline-none",
        "hover:bg-black/10 focus-visible:ring-2 focus-visible:ring-brand",
        active && "bg-black/15 ring-2 ring-brand",
        isDragging && "opacity-30",
        !entry.assignable && "opacity-60",
      )}
    >
      <PixelSprite look={look} pose="stand" animated={entry.running > 0} scale={4} />
      <span className="max-w-full truncate text-micro font-medium" style={{ color: "#2c2418" }}>
        {entry.name}
      </span>
      {entry.running > 0 && (
        <span
          className="absolute right-1 top-0 rounded-full bg-brand px-1 text-micro font-semibold tabular-nums text-brand-foreground"
          title={t(($) => $.office.running, { count: entry.running })}
        >
          {entry.running}
        </span>
      )}
    </button>
  );
}

/**
 * The corridor under the rooms: where walkers cross, and where the staff
 * stand. Dragging a staff member onto a desk assigns the issue; onto a room
 * opens a new issue there. Clicking one filters the office to their issues.
 */
export const OfficeCorridor = memo(function OfficeCorridor({
  laneRef,
  tasks,
}: {
  laneRef: RefObject<HTMLDivElement | null>;
  tasks: readonly AgentTask[];
}) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const user = useAuthStore((s) => s.user);
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: agents = [] } = useQuery(agentListOptions(wsId));
  const assigneeFilters = useViewStore((s) => s.assigneeFilters);
  const toggleAssigneeFilter = useViewStore((s) => s.toggleAssigneeFilter);

  const staff = useMemo<StaffEntry[]>(() => {
    const counts = runningByAgent(tasks);
    const role = members.find((member) => member.user_id === user?.id)?.role;
    const agentEntries = agents
      .filter((agent) => !agent.archived_at)
      .map<StaffEntry>((agent) => ({
        type: "agent",
        id: agent.id,
        name: agent.name,
        running: counts.get(agent.id) ?? 0,
        assignable: canAssignAgent(agent, user?.id, role),
      }))
      .sort((a, b) => b.running - a.running || a.name.localeCompare(b.name));
    const memberEntries = members
      .map<StaffEntry>((member) => ({
        type: "member",
        id: member.user_id,
        name: member.name,
        running: 0,
        assignable: true,
      }))
      .sort((a, b) => a.name.localeCompare(b.name));
    return [...agentEntries, ...memberEntries];
  }, [agents, members, tasks, user?.id]);

  const activeKeys = useMemo(
    () => new Set(assigneeFilters.map((filter) => `${filter.type}:${filter.id}`)),
    [assigneeFilters],
  );

  return (
    <div
      ref={laneRef}
      data-office-corridor=""
      className="relative shrink-0 overflow-hidden"
      style={{ height: CORRIDOR_HEIGHT }}
    >
      <div
        aria-hidden
        className="absolute inset-0 dark:brightness-75"
        style={{
          backgroundImage: "repeating-linear-gradient(0deg, #c8b08c 0 8px, #bca47f 8px 16px)",
        }}
      />
      <div className="absolute inset-x-0 bottom-0 flex items-end gap-1 overflow-x-auto px-3">
        <span
          className="mb-2 mr-2 shrink-0 self-center rounded-[3px] px-2 py-0.5 text-label font-semibold text-white"
          style={{ background: "#78685a" }}
        >
          {t(($) => $.office.staff)}
        </span>
        {staff.length === 0 ? (
          <span className="mb-3 text-caption" style={{ color: "#2c2418" }}>
            {t(($) => $.office.staff_empty)}
          </span>
        ) : (
          staff.map((entry) => (
            <StaffMember
              key={`${entry.type}:${entry.id}`}
              entry={entry}
              active={activeKeys.has(`${entry.type}:${entry.id}`)}
              onToggle={(picked) => toggleAssigneeFilter({ type: picked.type, id: picked.id })}
            />
          ))
        )}
      </div>
    </div>
  );
});
