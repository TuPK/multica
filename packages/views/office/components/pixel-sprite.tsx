"use client";

import { memo } from "react";
import { cn } from "@multica/ui/lib/utils";
import {
  SPRITE_FRAME_WIDTH,
  spriteFrameHeight,
  spriteSheetUri,
  type SpriteLook,
  type SpritePose,
} from "../lib/pixel-sprites";

export const PixelSprite = memo(function PixelSprite({
  look,
  pose,
  animated = false,
  scale = 3,
  className,
}: {
  look: SpriteLook;
  pose: SpritePose;
  /** Steps through both frames (typing when seated, walking when standing). */
  animated?: boolean;
  scale?: number;
  className?: string;
}) {
  return (
    <span
      aria-hidden
      className={cn("block shrink-0", animated && "office-sprite-anim", className)}
      style={{
        width: SPRITE_FRAME_WIDTH * scale,
        height: spriteFrameHeight(pose) * scale,
        backgroundImage: spriteSheetUri(look, pose),
        backgroundSize: "200% 100%",
        backgroundRepeat: "no-repeat",
        imageRendering: "pixelated",
      }}
    />
  );
});
