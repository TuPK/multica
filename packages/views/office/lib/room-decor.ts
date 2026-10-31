import type { RoomTheme } from "./room-theme";

type Rect = readonly [x: number, y: number, w: number, h: number, fill: string];

export interface RoomDecor {
  uri: string;
  width: number;
  height: number;
}

const WOOD = "#b08054";
const SCREEN = "#262a3a";

const DECOR_RECTS: Record<RoomTheme, readonly Rect[]> = {
  archive: [
    [0, 9, 40, 2, WOOD],
    [2, 4, 6, 5, "#e6be78"],
    [10, 5, 6, 4, "#c89660"],
    [18, 3, 6, 6, "#78a0d0"],
    [26, 4, 5, 5, "#e6be78"],
    [33, 5, 5, 4, "#aac896"],
    [0, 18, 40, 2, WOOD],
    [3, 13, 6, 5, "#c89660"],
    [12, 12, 7, 6, "#e6be78"],
    [22, 14, 5, 4, "#78a0d0"],
    [30, 13, 6, 5, "#c89660"],
  ],
  intake: [
    [1, 1, 38, 18, WOOD],
    [2, 2, 36, 16, "#c4a06e"],
    [5, 5, 6, 4, "#eb5757"],
    [14, 4, 6, 4, "#f2c94c"],
    [23, 6, 6, 4, "#f2994a"],
    [31, 5, 5, 4, "#6aa6f2"],
    [8, 11, 6, 4, "#f2c94c"],
    [20, 12, 6, 4, "#6aa6f2"],
  ],
  work: [
    [1, 1, 38, 18, "#4a4f66"],
    [2, 2, 36, 16, SCREEN],
    [6, 12, 3, 4, "#52be7c"],
    [11, 9, 3, 7, "#52be7c"],
    [16, 11, 3, 5, "#52be7c"],
    [21, 6, 3, 10, "#52be7c"],
    [26, 8, 3, 8, "#52be7c"],
    [31, 4, 3, 12, "#52be7c"],
  ],
  review: [
    [1, 1, 38, 18, "#4a4f66"],
    [2, 2, 36, 16, "#e8f4ec"],
    [9, 10, 2, 2, "#46aa64"],
    [11, 12, 2, 2, "#46aa64"],
    [13, 10, 2, 2, "#46aa64"],
    [15, 8, 2, 2, "#46aa64"],
    [17, 6, 2, 2, "#46aa64"],
    [19, 4, 2, 2, "#46aa64"],
    [24, 8, 10, 2, "#aab4c4"],
    [24, 12, 7, 2, "#aab4c4"],
  ],
  alert: [
    [15, 1, 10, 2, "#f05050"],
    [13, 3, 14, 8, "#f05050"],
    [15, 11, 10, 2, "#f05050"],
    [16, 4, 4, 4, "#ff9696"],
    [12, 14, 16, 4, "#555a6e"],
  ],
  lounge: [
    [6, 2, 6, 5, "#facc48"],
    [8, 7, 2, 3, "#facc48"],
    [6, 10, 6, 2, "#facc48"],
    [17, 2, 6, 5, "#facc48"],
    [19, 7, 2, 3, "#facc48"],
    [17, 10, 6, 2, "#facc48"],
    [28, 2, 6, 5, "#facc48"],
    [30, 7, 2, 3, "#facc48"],
    [28, 10, 6, 2, "#facc48"],
    [2, 13, 36, 2, WOOD],
    [2, 16, 36, 2, "#7a5a3c"],
  ],
  closed: [
    [3, 3, 34, 14, "#8a8a96"],
    [4, 4, 32, 12, "#a4a4b0"],
    [8, 6, 2, 2, "#6e6e7d"],
    [10, 8, 2, 2, "#6e6e7d"],
    [12, 10, 2, 2, "#6e6e7d"],
    [12, 6, 2, 2, "#6e6e7d"],
    [8, 10, 2, 2, "#6e6e7d"],
  ],
};

const WIDTH = 40;
const HEIGHT = 20;
const cache = new Map<RoomTheme, RoomDecor>();

/** Wall art for a room's header, as a pixel-grid SVG data URI. */
export function roomDecor(theme: RoomTheme): RoomDecor {
  const cached = cache.get(theme);
  if (cached) return cached;
  const body = DECOR_RECTS[theme]
    .map(([x, y, w, h, fill]) => `<rect x="${x}" y="${y}" width="${w}" height="${h}" fill="${fill}"/>`)
    .join("");
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${WIDTH} ${HEIGHT}" shape-rendering="crispEdges">${body}</svg>`;
  const decor = {
    uri: `url("data:image/svg+xml,${encodeURIComponent(svg)}")`,
    width: WIDTH,
    height: HEIGHT,
  };
  cache.set(theme, decor);
  return decor;
}
