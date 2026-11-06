"use client";

import { useEffect, useRef, useState, type MouseEvent, type ReactNode } from "react";
import { cn } from "@multica/ui/lib/utils";
import {
  unitsFor,
  type FromWorker,
  type CompanyLabel,
  type CompanyPoint,
  type CompanyRoom,
  type CompanyUnits,
  type ToWorker,
  type WorkerSession,
} from "../lib/pixtuoid/office-protocol";

export type { CompanyUnits, WorkerSession } from "../lib/pixtuoid/office-protocol";

/** What the company tells the overlays pinned to its floor: its size, and where the aquarium stands, in layout units. */
export interface CompanyStage {
  units: CompanyUnits;
  tank: CompanyPoint | null;
  /** The desk rooms, in the page's room order. */
  rooms: readonly CompanyRoom[];
  /** The whiteboards, which open the kanban board. */
  boards: readonly CompanyRoom[];
}

/** A worker's figure, in layout units, from its tag's anchor (the top of its head) down. */
const FIGURE_HALF_WIDTH = 8;
const FIGURE_HEIGHT = 24;

/** The issue a worker's tooltip names: its title and its status. */
export interface CompanyTagDetail {
  title: string;
  status: string;
}

interface TagProps {
  tagName: (key: string) => string;
  tagDetail: (key: string) => CompanyTagDetail | null;
}

/**
 * The pixtuoid company. The engine runs in a worker that owns the canvas; this
 * component sends it the sessions and places the tags the worker reports above
 * each seated worker's head. A session is a worker at a desk, and its tag names
 * the worker. Hovering a tag shows the issue the worker is on. The company fills
 * the page edge to edge: the worker lays the floor out for the page's own shape.
 */
export function CompanyCanvas({
  sessions,
  tagName,
  tagDetail,
  onPick,
  onMove,
  roomNames,
  onRoomPick,
  roomPickable,
  children,
}: {
  sessions: readonly WorkerSession[];
  /** The name over a worker's head. */
  tagName: (key: string) => string;
  /** The issue a worker's tooltip names, or null when it has none. */
  tagDetail: (key: string) => CompanyTagDetail | null;
  /** A click on a tag: the worker's issue. */
  onPick: (key: string) => void;
  /** A click on a worker's figure: open its panel. */
  onMove: (key: string) => void;
  /** The names of the desk rooms, in slot order. */
  roomNames?: readonly string[];
  /** A click on a room's name. With `roomPickable`, only the rooms it accepts are clickable. */
  onRoomPick?: (index: number) => void;
  roomPickable?: (index: number) => boolean;
  /** Overlays pinned to the company's floor, placed in the stage's layout units. */
  children?: (stage: CompanyStage) => ReactNode;
}) {
  const pageRef = useRef<HTMLDivElement>(null);
  const stageRef = useRef<HTMLDivElement>(null);
  const tagsRef = useRef<HTMLDivElement>(null);
  const workerRef = useRef<Worker | null>(null);
  // The latest tags, so a click on a worker's figure can find which one it is.
  const labelsRef = useRef<readonly CompanyLabel[]>([]);
  const [rooms, setRooms] = useState<readonly CompanyRoom[]>([]);
  const [tank, setTank] = useState<CompanyPoint | null>(null);
  const [boards, setBoards] = useState<readonly CompanyRoom[]>([]);
  const [units, setUnits] = useState<CompanyUnits>(() => unitsFor(0, 0));
  const unitsRef = useRef(units);
  unitsRef.current = units;
  // The worker's callbacks and the tags read the latest props, so they never restart the company.
  const latest = useRef({ tagName, tagDetail, onPick, onMove });
  latest.current = { tagName, tagDetail, onPick, onMove };

  // The worker starts once per mount. Its canvas is created here because a canvas can move to a worker only once.
  useEffect(() => {
    const stage = stageRef.current;
    const tags = tagsRef.current;
    if (!stage || !tags) return;

    const canvas = document.createElement("canvas");
    canvas.setAttribute("aria-hidden", "true");
    canvas.className = "absolute inset-0 size-full";
    canvas.style.imageRendering = "pixelated";
    stage.insertBefore(canvas, tags);

    const worker = new Worker(new URL("../lib/pixtuoid/office.worker.ts", import.meta.url), { type: "module" });
    workerRef.current = worker;
    const offscreen = canvas.transferControlToOffscreen();
    worker.postMessage({ type: "init", canvas: offscreen } satisfies ToWorker, [offscreen]);

    // A hidden page needs no frames: the company holds still until the page is seen again.
    const sendPaused = () => worker.postMessage({ type: "paused", paused: document.hidden } satisfies ToWorker);
    sendPaused();
    document.addEventListener("visibilitychange", sendPaused);

    worker.onmessage = (event: MessageEvent<FromWorker>) => {
      if (event.data.type === "rooms") {
        setRooms(event.data.rooms);
        setTank(event.data.tank);
        setBoards(event.data.boards);
        return;
      }
      if (event.data.type !== "labels") return;
      labelsRef.current = event.data.labels;
      placeTags(tags, event.data.labels, unitsRef.current, latest.current);
    };

    return () => {
      document.removeEventListener("visibilitychange", sendPaused);
      worker.terminate();
      workerRef.current = null;
      canvas.remove();
      tags.replaceChildren();
      labelsRef.current = [];
    };
  }, []);

  // The company is laid out for the page's own shape, so a resized page re-measures it.
  useEffect(() => {
    const page = pageRef.current;
    if (!page) return;
    const measure = () => {
      const next = unitsFor(page.clientWidth, page.clientHeight);
      setUnits((current) => (current.w === next.w && current.h === next.h ? current : next));
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(page);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    workerRef.current?.postMessage({ type: "size", units } satisfies ToWorker);
    const tags = tagsRef.current;
    if (tags) placeTags(tags, labelsRef.current, units, latest.current);
  }, [units]);

  // The sessions go to the worker whenever they change; it seats and sends them home.
  useEffect(() => {
    workerRef.current?.postMessage({ type: "sessions", sessions: [...sessions] } satisfies ToWorker);
  }, [sessions]);

  // A change of issue re-words the tags at once, without waiting for the worker to report a move.
  useEffect(() => {
    const tags = tagsRef.current;
    if (!tags) return;
    for (const el of tags.querySelectorAll<HTMLButtonElement>("button")) fillTag(el, { tagName, tagDetail });
  }, [tagName, tagDetail]);

  // A click on a figure opens its worker's issue. The figure hangs below its tag, so the nearest tag above the click wins.
  const pickFigure = (event: MouseEvent<HTMLDivElement>) => {
    const target = event.target as HTMLElement;
    if (target.tagName !== "CANVAS") return;
    const box = target.getBoundingClientRect();
    const x = ((event.clientX - box.left) / box.width) * units.w;
    const y = ((event.clientY - box.top) / box.height) * units.h;
    let best: CompanyLabel | null = null;
    for (const label of labelsRef.current) {
      const dx = Math.abs(x - label.x);
      const dy = y - label.y;
      if (dx > FIGURE_HALF_WIDTH || dy < 0 || dy > FIGURE_HEIGHT) continue;
      if (!best || dx < Math.abs(x - best.x)) best = label;
    }
    if (best) latest.current.onMove(best.key);
  };

  return (
    <div
      ref={pageRef}
      className="min-h-0 min-w-0 flex-1 overflow-hidden bg-[#343e58]"
      style={{ containerType: "size" }}
      onClick={pickFigure}
    >
      <div ref={stageRef} className="relative size-full">
        <div ref={tagsRef} className="pointer-events-none absolute inset-0" />
        {rooms.map((room, index) => {
          // A worker's tag hangs over its head, which is at the room's north wall. The name of a top-row room
          // clears the tag row above it; the name of a lower room goes under the room, where nothing hangs.
          const topRow = room.y === Math.min(...rooms.map((other) => other.y));
          const className = cn(
            "absolute -translate-x-1/2 whitespace-nowrap rounded-[3px] bg-[#2c2418]/85 px-2 py-0.5 text-micro font-semibold text-[#fff8ea]",
            topRow ? "-translate-y-full -mt-7" : "mt-1",
          );
          const style = {
            left: `${((room.x + room.w / 2) / units.w) * 100}%`,
            top: `${((topRow ? room.y : room.y + room.h) / units.h) * 100}%`,
          };
          const position = topRow ? "above" : "below";
          if (onRoomPick && (roomPickable?.(index) ?? true)) {
            return (
              <button
                key={index}
                type="button"
                data-room-name={position}
                onClick={() => onRoomPick(index)}
                className={cn(className, "pointer-events-auto cursor-pointer hover:bg-[#1d5fd6]")}
                style={style}
              >
                {roomNames?.[index]}
              </button>
            );
          }
          return (
            <span key={index} data-room-name={position} className={cn(className, "pointer-events-none")} style={style}>
              {roomNames?.[index]}
            </span>
          );
        })}
        <div className="pointer-events-none absolute inset-0">{children?.({ units, tank, rooms, boards })}</div>
      </div>
    </div>
  );
}

/** Writes a tag's name and its tooltip. The tooltip is hidden unless there is an issue to show. */
function fillTag(el: HTMLButtonElement, props: TagProps): void {
  const key = el.dataset.key ?? "";
  const name = el.querySelector<HTMLElement>("[data-tag-name]");
  const text = props.tagName(key);
  if (name && name.textContent !== text) name.textContent = text;
  const tooltip = el.querySelector<HTMLElement>("[data-tag-tooltip]");
  if (!tooltip) return;
  const detail = props.tagDetail(key);
  tooltip.style.display = detail ? "" : "none";
  const title = document.createElement("span");
  title.className = "block font-semibold";
  title.textContent = detail?.title ?? "";
  const status = document.createElement("span");
  status.className = "block opacity-75";
  status.textContent = detail?.status ?? "";
  tooltip.replaceChildren(title, status);
}

/** Places one tag per labelled worker, making a button the first time a worker is seen. */
function placeTags(
  tags: HTMLDivElement,
  labels: readonly CompanyLabel[],
  units: CompanyUnits,
  props: TagProps & { onPick: (key: string) => void; onMove: (key: string) => void },
): void {
  const buttons = new Map<string, HTMLButtonElement>();
  for (const el of tags.querySelectorAll<HTMLButtonElement>("button")) buttons.set(el.dataset.key ?? "", el);
  const shown = new Set<string>();
  for (const label of labels) {
    shown.add(label.key);
    let el = buttons.get(label.key);
    if (!el) {
      el = document.createElement("button");
      el.type = "button";
      el.dataset.key = label.key;
      el.addEventListener("click", () => props.onPick(label.key));
      // The tooltip is a child of the tag, so the tag's hover shows it. Screen readers get the name alone.
      el.className =
        "group pointer-events-auto absolute left-0 top-0 -translate-x-1/2 -translate-y-full cursor-pointer whitespace-nowrap rounded-[3px] bg-white/90 px-2 py-0.5 text-micro font-semibold text-[#2c2418] shadow-sm";
      const name = document.createElement("span");
      name.dataset.tagName = "";
      el.appendChild(name);
      const tooltip = document.createElement("span");
      tooltip.dataset.tagTooltip = "";
      tooltip.setAttribute("aria-hidden", "true");
      tooltip.className =
        "pointer-events-none absolute bottom-full left-1/2 mb-1 hidden w-max max-w-[240px] -translate-x-1/2 whitespace-normal rounded-[3px] bg-[#2c2418] px-2 py-1 text-micro font-normal text-[#fff8ea] shadow-md group-hover:block";
      el.appendChild(tooltip);
      fillTag(el, props);
      tags.appendChild(el);
      buttons.set(label.key, el);
    }
    el.style.left = `${(label.x / units.w) * 100}%`;
    el.style.top = `${(label.y / units.h) * 100}%`;
    el.style.display = "";
  }
  for (const [key, el] of buttons) {
    if (!shown.has(key)) el.style.display = "none";
  }
}
