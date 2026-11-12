import type {
  IssueWorkflowTemplateStep,
  IssueWorkflowTransition,
  NewWorkflowStatus,
} from "@multica/core/types";
import type { IssueWorkflowStep } from "../types";

/** A new transition id, unique within the draft. */
export function newTransitionId(): string {
  return `trans-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

/** The longest status key the server accepts (issuestatus.go maxKeyLen). */
const MAX_STATUS_KEY_LENGTH = 32;

/** Keys the server reserves: built-in statuses and lifecycle categories. */
const RESERVED_STATUS_KEYS = new Set([
  "backlog", "todo", "in_progress", "in_review", "blocked", "done", "cancelled",
  "unstarted", "started", "closed",
]);

/**
 * Converts a status display name to the ASCII key the server derives from it
 * (issuestatus.go slugify): lowercase, each run of other characters becomes one
 * underscore, outer underscores trimmed, at most 32 characters.
 */
export function slugifyStatusName(name: string): string {
  const slug = name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "");
  return truncateKey(slug, MAX_STATUS_KEY_LENGTH);
}

function truncateKey(base: string, limit: number): string {
  return base.length <= limit ? base : base.slice(0, limit).replace(/_+$/, "");
}

/** A catalog status as the designer reads it. Archived statuses still own their key. */
export interface CatalogStatus {
  key: string;
  name: string;
  archived_at?: string | null;
}

/**
 * The key a workflow save gives a status it creates, mirroring the server's
 * create path (handler/issue_workflow.go createWorkflowStatus): a status whose
 * name matches a live status reuses that status; otherwise the slugified name
 * is the key, with the first free `_n` suffix when another status already has it.
 * `taken` holds every key in use, including keys this save already resolved.
 */
export function resolveCreatedStatusKey(
  status: NewWorkflowStatus,
  catalog: readonly CatalogStatus[],
  taken: ReadonlySet<string>,
): string {
  const name = status.name.trim().toLowerCase();
  const reused = catalog.find((entry) => !entry.archived_at && entry.name.trim().toLowerCase() === name);
  if (reused) return reused.key;

  const base = slugifyStatusName(status.name) || (status.category ?? "started");
  const occupied = (key: string) => RESERVED_STATUS_KEYS.has(key) || taken.has(key);
  if (!occupied(base)) return base;
  for (let n = 2; n <= taken.size + RESERVED_STATUS_KEYS.size + 2; n++) {
    const suffix = `_${n}`;
    const candidate = truncateKey(base, MAX_STATUS_KEY_LENGTH - suffix.length) + suffix;
    if (!occupied(candidate)) return candidate;
  }
  return base;
}

export type AnyWorkflowStep = IssueWorkflowStep | IssueWorkflowTemplateStep;

/**
 * Deep clones workflow steps and ensures all transition ids are newly generated and unique.
 * Each status a save creates gets the key the server will give it. Transitions that
 * name a created status by its plain slug are pointed at that key.
 * Preserves layout, role_hint, and new_status if present.
 */
export function cloneWorkflowSteps(
  steps: readonly AnyWorkflowStep[],
  catalog: readonly CatalogStatus[] = [],
): IssueWorkflowStep[] {
  const taken = new Set(catalog.map((entry) => entry.key));
  const keyForPlain = new Map<string, string>();
  const createdKeys = steps.map((step) => {
    if (!("new_status" in step) || !step.new_status) return undefined;
    const plain = slugifyStatusName(step.new_status.name);
    const resolved = resolveCreatedStatusKey(step.new_status, catalog, taken);
    taken.add(resolved);
    if (plain && !keyForPlain.has(plain)) keyForPlain.set(plain, resolved);
    return resolved;
  });
  const targetKey = (key: string) => keyForPlain.get(key) ?? key;

  return steps.map((step, index) => {
    const transitions: IssueWorkflowTransition[] = (step.transitions ?? []).map((t) => ({
      id: newTransitionId(),
      label: t.label,
      when: t.when ? (JSON.parse(JSON.stringify(t.when)) as typeof t.when) : null,
      to_status_key: targetKey(t.to_status_key),
      ...(t.handler
        ? {
            handler: {
              type: t.handler.type,
              ...(t.handler.id ? { id: t.handler.id } : {}),
            },
          }
        : {}),
    }));

    const status_key =
      createdKeys[index] ??
      ("status_key" in step && step.status_key ? step.status_key : "");

    const role_hint = "role_hint" in step ? step.role_hint : undefined;
    const new_status: NewWorkflowStatus | undefined =
      "new_status" in step ? step.new_status : undefined;

    return {
      status_key,
      handler: {
        type: step.handler.type,
        ...(step.handler.id ? { id: step.handler.id } : {}),
      },
      instructions: step.instructions ?? "",
      transitions,
      ...(role_hint ? { role_hint } : {}),
      ...(new_status ? { new_status } : {}),
      ...(step.layout ? { layout: { ...step.layout } } : {}),
      ...("next_status_key" in step && step.next_status_key ? { next_status_key: step.next_status_key } : {}),
      ...("back_status_key" in step && step.back_status_key ? { back_status_key: step.back_status_key } : {}),
    };
  });
}

/** Layout constants: rows of at most 5 nodes. */
export const AUTO_LAYOUT_X_GAP = 320;
export const AUTO_LAYOUT_Y_GAP = 220;
export const AUTO_LAYOUT_START_X = 60;
export const AUTO_LAYOUT_START_Y = 60;
export const NODES_PER_ROW = 5;

/** Generates canvas position for a node by step index. */
export function getAutoLayoutPosition(index: number): { x: number; y: number } {
  const row = Math.floor(index / NODES_PER_ROW);
  const col = index % NODES_PER_ROW;
  return {
    x: AUTO_LAYOUT_START_X + col * AUTO_LAYOUT_X_GAP,
    y: AUTO_LAYOUT_START_Y + row * AUTO_LAYOUT_Y_GAP,
  };
}

/** Assigns auto-layout positions to any steps missing layout. */
export function ensureStepLayouts<T extends { layout?: { x: number; y: number } }>(
  steps: readonly T[],
): T[] {
  return steps.map((step, idx) => {
    if (step.layout && Number.isFinite(step.layout.x) && Number.isFinite(step.layout.y)) {
      return step;
    }
    return {
      ...step,
      layout: getAutoLayoutPosition(idx),
    };
  });
}

/**
 * Derives board order from node canvas position: left to right, then top to bottom.
 * Nodes are grouped by row band (vertical tolerance), and within each row ordered by x.
 */
export function deriveStepOrder<T extends { layout?: { x: number; y: number } }>(
  steps: readonly T[],
  rowHeight = 160,
): T[] {
  return [...steps].sort((a, b) => {
    const posA = a.layout ?? { x: 0, y: 0 };
    const posB = b.layout ?? { x: 0, y: 0 };
    const rowA = Math.floor((posA.y + rowHeight / 2) / rowHeight);
    const rowB = Math.floor((posB.y + rowHeight / 2) / rowHeight);
    if (rowA !== rowB) {
      return rowA - rowB;
    }
    return posA.x - posB.x;
  });
}

/**
 * Determines whether the draft is still the untouched initial blank seed.
 */
export function isUntouchedBlankSeed(params: {
  name: string;
  description: string;
  steps: readonly IssueWorkflowStep[];
  defaultName: string;
  defaultBranchStart: string;
  defaultBranchDone: string;
}): boolean {
  const { name, description, steps, defaultName, defaultBranchStart, defaultBranchDone } = params;

  // Name must match default
  if (name.trim() !== defaultName.trim() && name.trim() !== "New Workflow") {
    return false;
  }

  // Description must be empty
  if (description.trim() !== "") {
    return false;
  }

  // Initial blank seed has exactly 4 steps: todo, in_progress, done, cancelled
  if (steps.length !== 4) {
    return false;
  }

  const [s0, s1, s2, s3] = steps;

  // Step 0: todo
  if (
    !s0 ||
    s0.status_key !== "todo" ||
    s0.handler.type !== "none" ||
    Boolean(s0.instructions) ||
    (s0.transitions?.length ?? 0) !== 1
  ) {
    return false;
  }
  const t0 = s0.transitions?.[0];
  if (
    !t0 ||
    t0.to_status_key !== "in_progress" ||
    t0.when !== null ||
    (t0.label !== defaultBranchStart && t0.label !== "Start")
  ) {
    return false;
  }

  // Step 1: in_progress
  if (
    !s1 ||
    s1.status_key !== "in_progress" ||
    s1.handler.type !== "none" ||
    Boolean(s1.instructions) ||
    (s1.transitions?.length ?? 0) !== 1
  ) {
    return false;
  }
  const t1 = s1.transitions?.[0];
  if (
    !t1 ||
    t1.to_status_key !== "done" ||
    t1.when !== null ||
    (t1.label !== defaultBranchDone && t1.label !== "Done")
  ) {
    return false;
  }

  // Step 2: done
  if (
    !s2 ||
    s2.status_key !== "done" ||
    s2.handler.type !== "none" ||
    Boolean(s2.instructions) ||
    (s2.transitions?.length ?? 0) > 0
  ) {
    return false;
  }

  // Step 3: cancelled
  if (
    !s3 ||
    s3.status_key !== "cancelled" ||
    s3.handler.type !== "none" ||
    Boolean(s3.instructions) ||
    (s3.transitions?.length ?? 0) > 0
  ) {
    return false;
  }

  return true;
}
