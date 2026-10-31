import { isBuiltInIssueStatus } from "@multica/core/issue-statuses";
import type { IssueStatusCategory } from "@multica/core/types";

export type RoomTheme = "archive" | "intake" | "work" | "review" | "alert" | "lounge" | "closed";

export interface RoomLook {
  theme: RoomTheme;
  floorA: string;
  floorB: string;
  /** Header plate; dark enough for white text. */
  plate: string;
}

const LOOKS: Record<RoomTheme, RoomLook> = {
  archive: { theme: "archive", floorA: "#e2dcce", floorB: "#d4cdbe", plate: "#78685a" },
  intake: { theme: "intake", floorA: "#d8e8f6", floorB: "#cce0ee", plate: "#4c78aa" },
  work: { theme: "work", floorA: "#e6f0e2", floorB: "#dae7d6", plate: "#347d4e" },
  review: { theme: "review", floorA: "#f3e8d6", floorB: "#e8dbc4", plate: "#a06928" },
  alert: { theme: "alert", floorA: "#f8d8d8", floorB: "#eec8c8", plate: "#be3c3c" },
  lounge: { theme: "lounge", floorA: "#d6f2e2", floorB: "#caead6", plate: "#2d785c" },
  closed: { theme: "closed", floorA: "#e2e2e6", floorB: "#d6d6dc", plate: "#6e6e7d" },
};

const BUILT_IN_THEME: Record<string, RoomTheme> = {
  backlog: "archive",
  todo: "intake",
  in_progress: "work",
  in_review: "review",
  blocked: "alert",
  done: "lounge",
  cancelled: "closed",
};

const CATEGORY_THEME: Record<IssueStatusCategory, RoomTheme> = {
  unstarted: "intake",
  started: "work",
  done: "lounge",
  closed: "closed",
};

/** Built-in statuses keep their own room; a custom status takes its category's. */
export function roomLookFor(statusKey: string, category: IssueStatusCategory): RoomLook {
  const theme = isBuiltInIssueStatus(statusKey)
    ? (BUILT_IN_THEME[statusKey] ?? CATEGORY_THEME[category])
    : CATEGORY_THEME[category];
  return LOOKS[theme];
}

/** The busiest room spans two desk columns; every other room is one. */
export function roomColumns(statusKey: string): 1 | 2 {
  return statusKey === "in_progress" ? 2 : 1;
}
