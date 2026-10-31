"use client";

import { memo, useCallback } from "react";
import { useDraggable, useDroppable } from "@dnd-kit/core";
import type { Issue } from "@multica/core/types";
import { useWorkspacePaths } from "@multica/core/paths";
import { useActorName } from "@multica/core/workspace/hooks";
import { cn } from "@multica/ui/lib/utils";
import { AppLink } from "../../navigation";
import { useT } from "../../i18n";
import { IssueActionsContextMenu } from "../../issues/actions";
import { PriorityIcon } from "../../issues/components/priority-icon";
import { IssueAgentActivityIndicator } from "../../issues/components/issue-agent-activity-indicator";
import {
  PEEK_TARGET_ATTR,
  useIsIssuePeeked,
  useIssuePeekActions,
  useIssuePeekLinkProps,
} from "../../issues/surface/peek-context";
import { lookForActor, noteColors } from "../lib/pixel-sprites";
import {
  DESK_HEIGHT,
  DESK_WIDTH,
  deskDropId,
  type OfficeDragData,
  type OfficeDropData,
} from "../lib/office-dnd";
import { PixelSprite } from "./pixel-sprite";

const NOTE_TEXT = "#262a3c";

export const OfficeDesk = memo(function OfficeDesk({
  issue,
  working,
  walking,
  registerDesk,
}: {
  issue: Issue;
  /** An agent has a running task on this issue: the worker types. */
  working: boolean;
  /** The worker is still walking in from another room. */
  walking: boolean;
  registerDesk: (issueId: string, element: HTMLElement | null) => void;
}) {
  const { t } = useT("issues");
  const paths = useWorkspacePaths();
  const peek = useIssuePeekActions();
  const peeked = useIsIssuePeeked(issue.id);
  const peekLinkProps = useIssuePeekLinkProps(issue.id);
  const { getActorName } = useActorName();

  const dragData: OfficeDragData = { type: "issue", issueId: issue.id };
  const { attributes, listeners, setNodeRef: setDragRef, isDragging } = useDraggable({
    id: issue.id,
    data: dragData,
  });
  const dropData: OfficeDropData = { type: "desk", issueId: issue.id, status: issue.status };
  const { setNodeRef: setDropRef } = useDroppable({ id: deskDropId(issue.id), data: dropData });

  const setRef = useCallback(
    (element: HTMLElement | null) => {
      setDragRef(element);
      setDropRef(element);
      registerDesk(issue.id, element);
    },
    [issue.id, registerDesk, setDragRef, setDropRef],
  );

  const look =
    issue.assignee_type && issue.assignee_id
      ? lookForActor(issue.assignee_type, issue.assignee_id)
      : null;
  const assigneeName =
    issue.assignee_type && issue.assignee_id
      ? getActorName(issue.assignee_type, issue.assignee_id)
      : t(($) => $.pickers.assignee.trigger_unassigned);
  const note = noteColors(issue.priority);

  return (
    <IssueActionsContextMenu issue={issue}>
      <div
        ref={setRef}
        data-office-desk=""
        data-walking={walking ? "" : undefined}
        data-peeked={peeked ? "" : undefined}
        {...{ [PEEK_TARGET_ATTR]: issue.id }}
        {...attributes}
        {...listeners}
        className={cn("group/card relative shrink-0", isDragging && "opacity-30")}
        style={{ width: DESK_WIDTH, height: DESK_HEIGHT }}
        onKeyDown={
          peek
            ? (e) => {
                // Space peeks the focused desk, like a board card.
                if (e.key !== " " || e.repeat) return;
                if (e.target !== e.currentTarget && (e.target as HTMLElement).tagName !== "A") return;
                e.preventDefault();
                peek.toggle(issue.id);
              }
            : undefined
        }
      >
        <AppLink
          href={paths.issueDetail(issue.id)}
          newTabTitle={issue.identifier}
          className={cn("relative block size-full", isDragging && "pointer-events-none")}
          {...peekLinkProps}
        >
          {/* Chair, worker, then the desk on top so the hands rest on it. */}
          <span
            aria-hidden
            className="absolute left-[14px] top-[26px] h-4 w-8 rounded-[2px]"
            style={{ background: look ? "#5a6482" : "#a4acc4" }}
          />
          {look && (
            <span
              data-office-actor=""
              className={cn(
                "absolute left-[12px] top-[0px] group-data-[walking]/card:invisible",
                working && "office-bob",
              )}
            >
              <PixelSprite look={look} pose="sit" animated={working} scale={4} />
            </span>
          )}
          <span
            aria-hidden
            className="absolute left-[6px] top-[38px] h-[22px] w-20 border-b-4"
            style={{ background: "#b08054", borderColor: "#7e583a" }}
          />
          <span aria-hidden className="absolute left-[10px] top-[60px] h-2 w-1" style={{ background: "#5a3e28" }} />
          <span aria-hidden className="absolute left-[78px] top-[60px] h-2 w-1" style={{ background: "#5a3e28" }} />
          <span
            aria-hidden
            className="absolute left-[54px] top-[24px] h-4 w-6"
            style={{ background: "#262a3a" }}
          >
            <span
              className={cn("absolute inset-[2px] bottom-[3px]", working && "office-siren")}
              style={{ background: working ? "#7fe0ff" : "#4a5a78" }}
            />
          </span>
          <span aria-hidden className="absolute left-[64px] top-[40px] h-[3px] w-1" style={{ background: "#262a3a" }} />
          <span aria-hidden className="absolute left-[20px] top-[46px] h-[3px] w-5" style={{ background: "#e8e6e0" }} />

          <span
            className="absolute left-[92px] top-1 flex h-[84px] w-[118px] flex-col overflow-hidden rounded-[3px] shadow-sm group-data-[peeked]/card:ring-2 group-data-[peeked]/card:ring-brand/60"
            style={{ background: note.paper, color: NOTE_TEXT }}
          >
            <span aria-hidden className="h-1.5 shrink-0" style={{ background: note.strip }} />
            <span className="flex min-h-0 flex-1 flex-col gap-0.5 px-1.5 py-1">
              <span className="flex items-center justify-between gap-1">
                <span className="flex min-w-0 items-center gap-1">
                  {issue.priority !== "none" && <PriorityIcon priority={issue.priority} className="size-3 shrink-0" />}
                  <span className="truncate font-mono text-micro opacity-70">{issue.identifier}</span>
                </span>
                <IssueAgentActivityIndicator issueId={issue.id} hoverCard={false} />
              </span>
              <span className="line-clamp-3 text-caption font-medium leading-snug">{issue.title}</span>
              <span className="mt-auto truncate text-micro opacity-70">{assigneeName}</span>
            </span>
          </span>
        </AppLink>
      </div>
    </IssueActionsContextMenu>
  );
});
