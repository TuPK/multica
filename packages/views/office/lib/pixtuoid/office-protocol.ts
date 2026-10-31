/**
 * The office's size in layout units. A smaller floor has fewer desks, and each one is drawn larger
 * on the page; 256 by 171 gives about two dozen desks.
 */
export const UNITS_W = 256;
export const UNITS_H = 171;
/** Picks the office's floor layout: the closest of the pack's variants to the reference. */
export const LAYOUT_SEED = 2;
/** The engine steps and paints this often; pixel art does not need more, and it keeps the worker light. */
export const FRAME_MS = 33;

/** A worker on the floor: one per issue, seated while its issue is being worked on. */
export interface WorkerSession {
  key: string;
  working: boolean;
}

/** A tag over a worker's head, in layout units: its anchor is the top of the head. */
export interface OfficeLabel {
  key: string;
  x: number;
  y: number;
}

/** Page to worker. The canvas moves to the worker once, at `init`. */
export type ToWorker =
  | { type: "init"; canvas: OffscreenCanvas }
  | { type: "sessions"; sessions: WorkerSession[] }
  | { type: "paused"; paused: boolean };

/** A walled desk room, in layout units. */
export interface OfficeRoom {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** Worker to page: where the tags go, and where the rooms are. Each is sent only when it changes. */
export type FromWorker =
  | { type: "labels"; labels: OfficeLabel[] }
  | { type: "rooms"; rooms: OfficeRoom[] }
  | { type: "failed" };
