"use client";

import { memo } from "react";
import { useDroppable } from "@dnd-kit/core";
import { Plus } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { cn } from "@multica/ui/lib/utils";
import type { Issue } from "@multica/core/types";
import { useT } from "../../i18n";
import { ListLoadMoreFooter } from "../../issues/components/list-load-more-footer";
import type { IssueStatusPageState } from "../../issues/surface/use-issue-status-branches";
import { DESK_WIDTH, roomDropId, type OfficeDropData } from "../lib/office-dnd";
import { roomDecor } from "../lib/room-decor";
import type { RoomLook } from "../lib/room-theme";
import { OfficeDesk } from "./office-desk";

const WALL = "#343e58";
const DESK_GAP = 8;
const FLOOR_PADDING = 12;
/** Slack for the floor's vertical scrollbar, so desks never touch it. */
const SCROLLBAR_SLACK = 8;

export function roomWidth(columns: number): number {
  return columns * DESK_WIDTH + (columns - 1) * DESK_GAP + FLOOR_PADDING * 2 + SCROLLBAR_SLACK;
}

export type RoomDropIntent =
  | { kind: "none" }
  | { kind: "move"; blockedReason: string | null }
  | { kind: "create" };

export const OfficeRoom = memo(function OfficeRoom({
  status,
  label,
  look,
  columns,
  issues,
  total,
  page,
  workingIds,
  walkingIds,
  intent,
  registerDesk,
  onCreate,
}: {
  status: string;
  label: string;
  look: RoomLook;
  columns: 1 | 2;
  issues: Issue[];
  total: number;
  page?: IssueStatusPageState;
  workingIds: ReadonlySet<string>;
  walkingIds: ReadonlySet<string>;
  /** What dropping the active drag here would do; `none` when nothing is dragged. */
  intent: RoomDropIntent;
  registerDesk: (issueId: string, element: HTMLElement | null) => void;
  onCreate?: (status: string) => void;
}) {
  const { t } = useT("issues");
  const data: OfficeDropData = { type: "room", status };
  const { setNodeRef, isOver } = useDroppable({ id: roomDropId(status), data });
  const decor = roomDecor(look.theme);
  const blocked = intent.kind === "move" && intent.blockedReason !== null;

  return (
    <section
      ref={setNodeRef}
      data-office-room={status}
      aria-label={label}
      className="relative flex shrink-0 flex-col overflow-hidden"
      style={{ width: roomWidth(columns) }}
    >
      <header
        className="relative flex h-16 shrink-0 items-center justify-between gap-2 px-2"
        style={{ background: WALL }}
      >
        <div
          className="flex min-w-0 items-center gap-1.5 rounded-[3px] py-1 pl-2 pr-1"
          style={{ background: look.plate }}
        >
          <h2 className="truncate text-label font-semibold text-white">{label}</h2>
          <span
            className="rounded-full bg-white px-1.5 text-micro font-semibold tabular-nums"
            style={{ color: look.plate }}
          >
            {total}
          </span>
          {onCreate && (
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              className="text-white hover:bg-white/20 hover:text-white"
              aria-label={t(($) => $.office.create_in_room, { room: label })}
              onClick={() => onCreate(status)}
            >
              <Plus />
            </Button>
          )}
        </div>
        <span
          aria-hidden
          className={cn("shrink-0", look.theme === "alert" && "office-siren")}
          style={{
            width: decor.width * 2,
            height: decor.height * 2,
            backgroundImage: decor.uri,
            backgroundSize: "100% 100%",
            imageRendering: "pixelated",
          }}
        />
      </header>

      <div className="relative min-h-0 flex-1">
        <div
          aria-hidden
          className="absolute inset-0 dark:brightness-75"
          style={{
            backgroundImage: `repeating-conic-gradient(${look.floorA} 0% 25%, ${look.floorB} 0% 50%)`,
            backgroundSize: "32px 32px",
          }}
        />
        <div data-office-floor="" className="absolute inset-0 overflow-y-auto" style={{ padding: FLOOR_PADDING }}>
          <div
            className="grid content-start justify-center"
            style={{
              gridTemplateColumns: `repeat(${columns}, ${DESK_WIDTH}px)`,
              gap: DESK_GAP,
            }}
          >
            {issues.map((issue) => (
              <OfficeDesk
                key={issue.id}
                issue={issue}
                working={workingIds.has(issue.id)}
                walking={walkingIds.has(issue.id)}
                registerDesk={registerDesk}
              />
            ))}
          </div>
          {page && (
            <ListLoadMoreFooter
              hasMore={page.hasMore}
              isLoading={page.isLoading || page.isFetching}
              total={page.total}
              onLoadMore={page.loadMore}
              isError={page.isError}
              onRetry={page.retry}
            />
          )}
        </div>
        {intent.kind !== "none" && isOver && (
          <div
            aria-hidden
            className={cn(
              "pointer-events-none absolute inset-0 z-30 flex items-start justify-center p-3 ring-4 ring-inset",
              blocked ? "bg-destructive/20 ring-destructive" : "bg-brand/15 ring-brand",
            )}
          >
            {(blocked || intent.kind === "create") && (
              <span className="rounded bg-background/95 px-2 py-1 text-caption font-medium text-foreground shadow">
                {blocked
                  ? intent.kind === "move" && intent.blockedReason
                  : t(($) => $.office.create_in_room, { room: label })}
              </span>
            )}
          </div>
        )}
      </div>

      {/* Doorway to the corridor. */}
      <div aria-hidden className="flex h-2 shrink-0 justify-center" style={{ background: WALL }}>
        <span className="h-2 w-10" style={{ background: "#c8b08c" }} />
      </div>
    </section>
  );
});
