"use client";

import {
  memo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react";
import type { Issue } from "@multica/core/types";
import { lookForActor, noteColors, type SpriteLook } from "../lib/pixel-sprites";
import {
  buildWalkPath,
  deskAnchor,
  walkDurationMs,
  walkKeyframes,
  type Point,
  type Rect,
} from "../lib/walk-path";
import { PixelSprite } from "./pixel-sprite";

/** More than this many movers at once (a bulk move) just appear in place. */
const MAX_WALKERS = 8;
/** Feet of a walker in the corridor, below the lane element's top edge. */
const LANE_OFFSET = 34;

const SPRITE_SCALE = 3;
const WALKER_WIDTH = 9 * SPRITE_SCALE;
const WALKER_HEIGHT = 14 * SPRITE_SCALE + 16;

interface Measured extends Rect {
  /** Visible band of the desk's room, relative to the office. */
  clipTop: number;
  clipBottom: number;
}

export interface Walker {
  key: string;
  issueId: string;
  look: SpriteLook;
  paper: string;
  strip: string;
  path: Point[];
}

interface Placement {
  status: string;
  look: SpriteLook;
  paper: string;
  strip: string;
}

function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

/**
 * Turns a status change into a walk: the worker leaves the old desk, crosses
 * the corridor and sits at the new one. Desks are re-parented between rooms, so
 * the walk is an overlay driven by measured positions rather than a layout
 * animation; the destination desk hides its worker until the walker arrives.
 */
export function useOfficeWalkers({
  contentRef,
  laneRef,
  issues,
}: {
  contentRef: RefObject<HTMLElement | null>;
  laneRef: RefObject<HTMLElement | null>;
  issues: readonly Issue[];
}) {
  const elements = useRef(new Map<string, HTMLElement>());
  const measured = useRef(new Map<string, Measured>());
  const previous = useRef<Map<string, Placement> | null>(null);
  const [walkers, setWalkers] = useState<Walker[]>([]);

  const placements = useMemo(() => {
    const map = new Map<string, Placement>();
    for (const issue of issues) {
      const note = noteColors(issue.priority);
      map.set(issue.id, {
        status: issue.status,
        // An unassigned issue is carried by a courier, so it still walks.
        look:
          issue.assignee_type && issue.assignee_id
            ? lookForActor(issue.assignee_type, issue.assignee_id)
            : lookForActor("member", issue.id),
        paper: note.paper,
        strip: note.strip,
      });
    }
    return map;
  }, [issues]);

  const registerDesk = useCallback((issueId: string, element: HTMLElement | null) => {
    if (element) elements.current.set(issueId, element);
    else elements.current.delete(issueId);
  }, []);

  const measure = useCallback(() => {
    const next = new Map<string, Measured>();
    const root = contentRef.current;
    if (!root) return next;
    const base = root.getBoundingClientRect();
    elements.current.forEach((element, issueId) => {
      const box = element.getBoundingClientRect();
      const floor = element.closest("[data-office-floor]")?.getBoundingClientRect();
      next.set(issueId, {
        x: box.left - base.left,
        y: box.top - base.top,
        w: box.width,
        h: box.height,
        clipTop: (floor?.top ?? box.top) - base.top,
        clipBottom: (floor?.bottom ?? box.bottom) - base.top,
      });
    });
    return next;
  }, [contentRef]);

  const laneY = useCallback(() => {
    const root = contentRef.current;
    const lane = laneRef.current;
    if (!root || !lane) return null;
    return lane.getBoundingClientRect().top - root.getBoundingClientRect().top + LANE_OFFSET;
  }, [contentRef, laneRef]);

  useLayoutEffect(() => {
    const next = measure();
    const before = previous.current;
    const lane = laneY();
    if (before && lane !== null && !prefersReducedMotion()) {
      const fresh: Walker[] = [];
      for (const [issueId, placement] of placements) {
        const was = before.get(issueId);
        const from = measured.current.get(issueId);
        const to = next.get(issueId);
        if (!was || was.status === placement.status || !from || !to) continue;
        if (fresh.length >= MAX_WALKERS) break;
        const start = deskAnchor(from);
        const end = deskAnchor(to);
        end.y = Math.min(Math.max(end.y, to.clipTop + 24), to.clipBottom - 4);
        fresh.push({
          key: `${issueId}:${Date.now()}:${fresh.length}`,
          issueId,
          look: placement.look,
          paper: placement.paper,
          strip: placement.strip,
          path: buildWalkPath(start, end, lane),
        });
      }
      if (fresh.length > 0) setWalkers((current) => [...current, ...fresh]);
    }
    measured.current = next;
    previous.current = placements;
  }, [placements, measure, laneY]);

  // Positions go stale when a room's floor scrolls or the window resizes.
  useEffect(() => {
    const root = contentRef.current;
    if (!root) return;
    let frame = 0;
    const refresh = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        measured.current = measure();
      });
    };
    root.addEventListener("scroll", refresh, { capture: true, passive: true });
    const observer =
      typeof ResizeObserver === "function" ? new ResizeObserver(refresh) : null;
    observer?.observe(root);
    return () => {
      cancelAnimationFrame(frame);
      root.removeEventListener("scroll", refresh, { capture: true });
      observer?.disconnect();
    };
  }, [contentRef, measure]);

  const finish = useCallback((key: string) => {
    setWalkers((current) => current.filter((walker) => walker.key !== key));
  }, []);

  const walkingIds = useMemo(
    () => new Set(walkers.map((walker) => walker.issueId)),
    [walkers],
  );

  return { walkers, walkingIds, registerDesk, finish };
}

const WalkerSprite = memo(function WalkerSprite({
  walker,
  onDone,
}: {
  walker: Walker;
  onDone: (key: string) => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const first = walker.path[0]!;

  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    // Environments without the Web Animations API show no walk, only arrival.
    if (typeof element.animate !== "function") {
      onDone(walker.key);
      return;
    }
    const animation = element.animate(
      walkKeyframes(walker.path).map((frame) => ({
        offset: frame.offset,
        transform: `translate(${frame.x}px, ${frame.y}px) scaleX(${frame.facing})`,
      })),
      { duration: walkDurationMs(walker.path), easing: "linear", fill: "forwards" },
    );
    animation.finished.then(() => onDone(walker.key)).catch(() => {});
    return () => animation.cancel();
  }, [walker, onDone]);

  return (
    <div
      ref={ref}
      aria-hidden
      data-office-walker={walker.issueId}
      className="pointer-events-none absolute left-0 top-0 z-40 flex flex-col items-center"
      style={{
        width: WALKER_WIDTH,
        height: WALKER_HEIGHT,
        marginLeft: -WALKER_WIDTH / 2,
        marginTop: -WALKER_HEIGHT,
        transformOrigin: "50% 100%",
        transform: `translate(${first.x}px, ${first.y}px)`,
      }}
    >
      <span
        className="mb-0.5 block h-3 w-5 rounded-[1px]"
        style={{ background: walker.paper, borderTop: `3px solid ${walker.strip}` }}
      />
      <PixelSprite look={walker.look} pose="stand" animated scale={SPRITE_SCALE} />
    </div>
  );
});

export function OfficeWalkers({
  walkers,
  onDone,
}: {
  walkers: readonly Walker[];
  onDone: (key: string) => void;
}) {
  return (
    <>
      {walkers.map((walker) => (
        <WalkerSprite key={walker.key} walker={walker} onDone={onDone} />
      ))}
    </>
  );
}
