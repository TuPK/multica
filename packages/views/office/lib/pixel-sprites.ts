import type { IssueAssigneeType } from "@multica/core/types";

export type SpriteKind = "human" | "agent" | "squad";

export interface SpriteLook {
  kind: SpriteKind;
  shirt: string;
  hair: string;
  skin: string;
}

/** `sit` is the upper body behind a desk; `stand` is the full body. */
export type SpritePose = "sit" | "stand";

export const SPRITE_FRAME_WIDTH = 9;
const FRAME_HEIGHT: Record<SpritePose, number> = { sit: 10, stand: 14 };

export function spriteFrameHeight(pose: SpritePose): number {
  return FRAME_HEIGHT[pose];
}

const SHIRTS = [
  "#4280e8",
  "#ec5c5c",
  "#52be7c",
  "#fabe40",
  "#9868e6",
  "#e8793a",
  "#3bb3c4",
  "#d2639c",
];
const HAIRS = ["#3a261c", "#d68430", "#1c1c20", "#96428c", "#7a5238", "#c9b27c"];
const SKINS = ["#f4c8a0", "#e0a878", "#b87c54", "#7c5236"];
const METAL = "#c4cedc";
const CYAN = "#50e8ff";
const VIOLET = "#b69cff";
const PANTS = "#3a3e54";
const SHOE = "#1e1e24";
const EYE = "#1e1e1e";

/** Stable 32-bit string hash (FNV-1a), so one actor always gets one look. */
export function hashString(value: string): number {
  let hash = 0x811c9dc5;
  for (let i = 0; i < value.length; i++) {
    hash ^= value.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}

export function lookForActor(
  type: IssueAssigneeType,
  id: string,
): SpriteLook {
  const h = hashString(`${type}:${id}`);
  return {
    kind: type === "member" ? "human" : type === "agent" ? "agent" : "squad",
    shirt: SHIRTS[h % SHIRTS.length]!,
    hair: HAIRS[(h >>> 4) % HAIRS.length]!,
    skin: SKINS[(h >>> 8) % SKINS.length]!,
  };
}

function rect(x: number, y: number, w: number, h: number, fill: string, opacity?: number) {
  return `<rect x="${x}" y="${y}" width="${w}" height="${h}" fill="${fill}"${opacity === undefined ? "" : ` fill-opacity="${opacity}"`}/>`;
}

function head(look: SpriteLook, ox: number): string {
  if (look.kind === "human") {
    return [
      rect(ox + 2, 1, 5, 2, look.hair),
      rect(ox + 2, 3, 5, 3, look.skin),
      rect(ox + 3, 4, 1, 1, EYE),
      rect(ox + 5, 4, 1, 1, EYE),
    ].join("");
  }
  const accent = look.kind === "agent" ? CYAN : VIOLET;
  return [
    rect(ox + 4, 0, 1, 1, accent),
    rect(ox + 2, 1, 5, 5, METAL),
    rect(ox + 3, 3, 1, 1, accent),
    rect(ox + 5, 3, 1, 1, accent),
  ].join("");
}

function body(look: SpriteLook, pose: SpritePose, frame: 0 | 1, ox: number): string {
  const parts = [
    rect(ox + 2, 6, 5, 4, look.shirt),
    rect(ox + 1, 6, 1, 3, look.shirt),
    rect(ox + 7, 6, 1, 3, look.shirt),
  ];
  const skin = look.kind === "human" ? look.skin : METAL;
  if (pose === "sit") {
    // Typing: the hands alternate between two keys.
    parts.push(rect(ox + (frame === 0 ? 3 : 2), 9, 1, 1, skin));
    parts.push(rect(ox + (frame === 0 ? 5 : 6), 9, 1, 1, skin));
    return parts.join("");
  }
  // Walking: opposite arm and leg swing together.
  parts.push(rect(ox + 1, frame === 0 ? 9 : 8, 1, 1, skin));
  parts.push(rect(ox + 7, frame === 0 ? 8 : 9, 1, 1, skin));
  parts.push(rect(ox + 2, 10, 2, frame === 0 ? 3 : 2, PANTS));
  parts.push(rect(ox + 5, 10, 2, frame === 0 ? 2 : 3, PANTS));
  parts.push(rect(ox + 2, frame === 0 ? 13 : 12, 2, 1, SHOE));
  parts.push(rect(ox + 5, frame === 0 ? 12 : 13, 2, 1, SHOE));
  return parts.join("");
}

function frameSvg(look: SpriteLook, pose: SpritePose, frame: 0 | 1, ox: number): string {
  return head(look, ox) + body(look, pose, frame, ox);
}

const sheetCache = new Map<string, string>();

/**
 * A two-frame sprite sheet as a data URI (frames side by side, 200% wide), so
 * one element animates by stepping `background-position`.
 */
export function spriteSheetUri(look: SpriteLook, pose: SpritePose): string {
  const key = `${look.kind}|${look.shirt}|${look.hair}|${look.skin}|${pose}`;
  const cached = sheetCache.get(key);
  if (cached) return cached;
  const width = SPRITE_FRAME_WIDTH * 2;
  const height = FRAME_HEIGHT[pose];
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${width} ${height}" shape-rendering="crispEdges">` +
    frameSvg(look, pose, 0, 0) +
    frameSvg(look, pose, 1, SPRITE_FRAME_WIDTH) +
    `</svg>`;
  const uri = `url("data:image/svg+xml,${encodeURIComponent(svg)}")`;
  sheetCache.set(key, uri);
  return uri;
}

const THEME_PRIORITY_NOTE = {
  urgent: { paper: "#ffd9d6", strip: "#eb5757" },
  high: { paper: "#ffe6cf", strip: "#f2994a" },
  medium: { paper: "#fff3c4", strip: "#f2c94c" },
  low: { paper: "#dcecff", strip: "#6aa6f2" },
  none: { paper: "#f4f1e8", strip: "#b4b4c0" },
} as const;

export function noteColors(priority: keyof typeof THEME_PRIORITY_NOTE) {
  return THEME_PRIORITY_NOTE[priority] ?? THEME_PRIORITY_NOTE.none;
}
