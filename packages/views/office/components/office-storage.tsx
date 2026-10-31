"use client";

import { useMemo, useState } from "react";
import type { Issue } from "@multica/core/types";
import { useWorkspaceId } from "@multica/core/hooks";
import { useT } from "../../i18n";
import { useStatusLabel } from "../../issues/utils/status-label";
import { UNITS_H, UNITS_W } from "../lib/pixtuoid/office-protocol";

/**
 * The lounge's storage: two filing cabinets set on the floor either side of the
 * rug in the reading room, one for the project's backlog and one for its done
 * issues. A click opens the drawer's list. Their feet stand at the points below.
 */
const CABINETS = [
  { status: "backlog", at: { x: 8, y: 98 }, tint: "#e4e8ee" },
  { status: "done", at: { x: 43, y: 98 }, tint: "#dcebd9" },
] as const;

type ChestStatus = (typeof CABINETS)[number]["status"];

export function OfficeStorage({ issues, onOpen }: { issues: readonly Issue[]; onOpen: (issueId: string) => void }) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const labelOf = useStatusLabel(wsId);
  const [open, setOpen] = useState<ChestStatus | null>(null);

  const byStatus = useMemo(() => {
    const map = new Map<ChestStatus, Issue[]>([
      ["backlog", []],
      ["done", []],
    ]);
    for (const issue of issues) map.get(issue.status as ChestStatus)?.push(issue);
    return map;
  }, [issues]);

  return (
    <>
      {CABINETS.map((cabinet) => {
        const listed = byStatus.get(cabinet.status) ?? [];
        const name = labelOf(cabinet.status);
        return (
          <div
            key={cabinet.status}
            className="pointer-events-none absolute h-0 w-0"
            style={{ left: `${(cabinet.at.x / UNITS_W) * 100}%`, top: `${(cabinet.at.y / UNITS_H) * 100}%` }}
          >
            <button
              type="button"
              aria-expanded={open === cabinet.status}
              aria-label={t(($) => $.office.storage_count, { name, count: listed.length })}
              onClick={() => setOpen(open === cabinet.status ? null : cabinet.status)}
              className="pointer-events-auto group absolute bottom-0 left-0 flex -translate-x-1/2 flex-col rounded-[2px] p-[3px] shadow-[2px_3px_0_rgba(20,22,28,0.35)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-black"
              style={{ width: "calc(min(100cqw, calc(100cqh * 3 / 2)) * 0.034)", aspectRatio: "2 / 3", background: "#9ea4ad", border: "1px solid #4f545c" }}
            >
              <span
                className="mb-[3px] truncate whitespace-nowrap rounded-[1px] px-0.5 text-center font-semibold leading-tight text-[#2c2418]"
                style={{ background: cabinet.tint, fontSize: "max(8px, 0.62cqw)" }}
              >
                {name}
              </span>
              {[0, 1, 2].map((drawer) => (
                <span
                  key={drawer}
                  aria-hidden
                  className="mb-[3px] flex flex-1 items-center justify-center rounded-[1px] last:mb-0 group-hover:brightness-110"
                  style={{ background: "linear-gradient(#d9dde2, #aeb4bc)", border: "1px solid #6b7079" }}
                >
                  <span className="h-[2px] w-1/2 rounded-full bg-[#5b6068]" />
                </span>
              ))}
              <span
                className="absolute -right-2 -top-2 rounded-full bg-[#2c2418] px-1.5 font-semibold text-[#fff8ea]"
                style={{ fontSize: "max(9px, 0.7cqw)" }}
              >
                {listed.length}
              </span>
            </button>
            {open === cabinet.status ? (
              <StoragePanel
                issues={listed}
                onOpen={(issueId) => {
                  setOpen(null);
                  onOpen(issueId);
                }}
              />
            ) : null}
          </div>
        );
      })}
    </>
  );
}

function StoragePanel({ issues, onOpen }: { issues: readonly Issue[]; onOpen: (issueId: string) => void }) {
  const { t } = useT("issues");
  return (
    <div className="pointer-events-auto absolute left-1/2 top-4 max-h-60 w-56 -translate-x-1/2 overflow-y-auto rounded-[3px] border border-[#8a6a3c] bg-[#fff8ea] p-1 text-caption text-[#2c2418] shadow-lg">
      {issues.length === 0 ? (
        <p className="px-2 py-1 text-muted-foreground">{t(($) => $.office.storage_empty)}</p>
      ) : (
        <ul>
          {issues.map((issue) => (
            <li key={issue.id}>
              <button
                type="button"
                onClick={() => onOpen(issue.id)}
                className="flex w-full min-w-0 flex-col rounded-[2px] px-2 py-1 text-left hover:bg-[#f3e6cc]"
              >
                <span className="font-mono text-micro text-muted-foreground">{issue.identifier}</span>
                <span className="line-clamp-2 font-medium">{issue.title}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
