"use client";

import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { NAV_COLUMN_MOTION_CLASS } from "../../layout/rail-styles";

/**
 * The floor plan's left column, which holds the company's floors. It starts
 * collapsed, and the toolbar's toggle opens it. The floor list portals into
 * `slotRef`, so it stays mounted while the column is collapsed.
 */
export function CompanyRoomsSidebar({
  open,
  slotRef,
}: {
  open: boolean;
  slotRef: (el: HTMLDivElement | null) => void;
}) {
  const { t } = useT("issues");
  return (
    <aside
      data-slot="company-rooms"
      data-state={open ? "expanded" : "collapsed"}
      inert={!open}
      className={cn(
        "flex shrink-0 overflow-hidden",
        NAV_COLUMN_MOTION_CLASS,
        open ? "w-56 border-r" : "w-0",
      )}
    >
      <div className="flex h-full w-56 shrink-0 flex-col gap-1 px-2 py-2">
        <div className="px-2 text-caption text-muted-foreground">{t(($) => $.company.floors)}</div>
        <div ref={slotRef} className="flex min-h-0 flex-1 flex-col overflow-y-auto" />
      </div>
    </aside>
  );
}
