/// <reference lib="webworker" />

// The office runs here, off the page's thread: the engine steps, the frame is
// painted to the canvas, and the tags' positions go back to the page. A busy
// page then never stalls the office, and a busy office never stalls the page.

import init, { Office } from "./wasm/pixtuoid_web.js";
import {
  FRAME_MS,
  LAYOUT_SEED,
  UNITS_H,
  UNITS_W,
  type FromWorker,
  type OfficeLabel,
  type OfficeRoom,
  type ToWorker,
  type WorkerSession,
} from "./office-protocol";

const scope = self as unknown as DedicatedWorkerGlobalScope;

let office: Office | null = null;
let memory: WebAssembly.Memory | null = null;
let canvas: OffscreenCanvas | null = null;
let context: OffscreenCanvasRenderingContext2D | null = null;
let paused = false;
let sessions: readonly WorkerSession[] = [];
// The sessions this office has seated, each with whether it is working.
const seated = new Map<string, boolean>();
let lastLabels = "";
let lastRooms = "";

function post(message: FromWorker): void {
  scope.postMessage(message);
}

/** Brings the office in line with the sessions: seats new workers, sends finished ones home. */
function reconcile(view: Office): void {
  const wanted = new Map(sessions.map((session) => [session.key, session.working]));
  for (const key of [...seated.keys()]) {
    if (wanted.has(key)) continue;
    view.issue_end(key);
    seated.delete(key);
  }
  for (const [key, working] of wanted) {
    if (!seated.has(key)) {
      // A full office has no desk left: the worker waits until one frees up.
      if (!view.issue_start(key)) continue;
      seated.set(key, false);
    }
    if (seated.get(key) === working) continue;
    view.issue_working(key, working);
    seated.set(key, working);
  }
}

/** Paints the engine's frame. The view wraps wasm memory directly, so nothing is copied per frame. */
function paint(view: Office, wasm: WebAssembly.Memory): void {
  if (!canvas || !context) return;
  const scale = view.scale();
  const width = UNITS_W * scale;
  const height = UNITS_H * scale;
  if (canvas.width !== width) canvas.width = width;
  if (canvas.height !== height) canvas.height = height;
  // Read the view after every step: a grown memory detaches the older one.
  const pixels = new Uint8ClampedArray(wasm.buffer, view.frame_ptr(), view.frame_len());
  context.putImageData(new ImageData(pixels, width, height), 0, 0);
}

function frame(): void {
  if (!office || !memory) return;
  if (!paused) {
    office.step(Date.now(), UNITS_W, UNITS_H);
    reconcile(office);
    paint(office, memory);
    const overlay = JSON.parse(office.overlay_json()) as { labels: OfficeLabel[]; rooms: OfficeRoom[] };
    const encoded = JSON.stringify(overlay.labels);
    if (encoded !== lastLabels) {
      lastLabels = encoded;
      post({ type: "labels", labels: overlay.labels });
    }
    const rooms = JSON.stringify(overlay.rooms);
    if (rooms !== lastRooms) {
      lastRooms = rooms;
      post({ type: "rooms", rooms: overlay.rooms });
    }
  }
  scope.setTimeout(frame, FRAME_MS);
}

scope.onmessage = (event: MessageEvent<ToWorker>) => {
  const message = event.data;
  if (message.type === "sessions") {
    sessions = message.sessions;
    return;
  }
  if (message.type === "paused") {
    paused = message.paused;
    return;
  }
  canvas = message.canvas;
  context = canvas.getContext("2d");
  init()
    .then((exports) => {
      memory = exports.memory;
      office = new Office(LAYOUT_SEED);
      office.clear_script();
      frame();
    })
    .catch(() => post({ type: "failed" }));
};

