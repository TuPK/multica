export interface Point {
  x: number;
  y: number;
}

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** Where a walker's feet land inside a desk rect. */
export function deskAnchor(rect: Rect): Point {
  return { x: rect.x + 30, y: rect.y + rect.h - 26 };
}

/**
 * Out of the room, along the corridor lane, into the next room. Consecutive
 * duplicates are dropped so a walker never pauses on a zero-length leg.
 */
export function buildWalkPath(from: Point, to: Point, laneY: number): Point[] {
  const raw: Point[] = [from, { x: from.x, y: laneY }, { x: to.x, y: laneY }, to];
  return raw.filter(
    (point, index) =>
      index === 0 || point.x !== raw[index - 1]!.x || point.y !== raw[index - 1]!.y,
  );
}

export function pathLength(points: readonly Point[]): number {
  let total = 0;
  for (let i = 1; i < points.length; i++) {
    total += Math.hypot(points[i]!.x - points[i - 1]!.x, points[i]!.y - points[i - 1]!.y);
  }
  return total;
}

const WALK_SPEED_PX_PER_S = 300;
const MIN_WALK_MS = 700;
const MAX_WALK_MS = 2600;

export function walkDurationMs(points: readonly Point[]): number {
  const ms = (pathLength(points) / WALK_SPEED_PX_PER_S) * 1000;
  return Math.min(MAX_WALK_MS, Math.max(MIN_WALK_MS, ms));
}

export interface WalkKeyframe extends Point {
  offset: number;
  /** -1 faces left, 1 faces right. */
  facing: 1 | -1;
}

/** One keyframe per waypoint; offsets follow distance so the pace stays even. */
export function walkKeyframes(points: readonly Point[]): WalkKeyframe[] {
  const total = pathLength(points) || 1;
  let walked = 0;
  let facing: 1 | -1 = 1;
  // A walker faces the way it first moves sideways, whichever leg that is.
  for (let i = 1; i < points.length; i++) {
    const dx = points[i]!.x - points[i - 1]!.x;
    if (dx !== 0) {
      facing = dx < 0 ? -1 : 1;
      break;
    }
  }
  return points.map((point, index) => {
    if (index > 0) {
      walked += Math.hypot(point.x - points[index - 1]!.x, point.y - points[index - 1]!.y);
      const dx = point.x - points[index - 1]!.x;
      if (dx !== 0) facing = dx < 0 ? -1 : 1;
    }
    return { ...point, offset: index === points.length - 1 ? 1 : walked / total, facing };
  });
}
