"use client";

import { Button } from "@multica/ui/components/ui/button";
import { Copy, File, LayoutTemplate } from "@multica/ui/icons";
import { useT } from "../../i18n";

export type StartingPointKind = "blank" | "clone" | "template";

export interface StartPointBarProps {
  activeChoice: StartingPointKind;
  onSelectBlank: () => void;
  onOpenClone: () => void;
  onOpenTemplate: () => void;
}

export function StartPointBar({
  activeChoice,
  onSelectBlank,
  onOpenClone,
  onOpenTemplate,
}: StartPointBarProps) {
  const { t } = useT("workflow");

  return (
    <div
      role="toolbar"
      aria-label={t(($) => $.designer.start_points.label)}
      className="flex items-center gap-2 px-6 py-2 border-b border-border bg-muted/40 shrink-0"
    >
      <span className="text-caption font-medium text-muted-foreground mr-1">
        {t(($) => $.designer.start_points.label)}:
      </span>

      <Button
        type="button"
        variant={activeChoice === "blank" ? "secondary" : "ghost"}
        size="sm"
        onClick={onSelectBlank}
        className="gap-1.5 h-7 text-caption"
      >
        <File className="size-3.5" />
        <span>{t(($) => $.designer.start_points.blank)}</span>
      </Button>

      <Button
        type="button"
        variant={activeChoice === "clone" ? "secondary" : "ghost"}
        size="sm"
        onClick={onOpenClone}
        className="gap-1.5 h-7 text-caption"
      >
        <Copy className="size-3.5" />
        <span>{t(($) => $.designer.start_points.clone)}</span>
      </Button>

      <Button
        type="button"
        variant={activeChoice === "template" ? "secondary" : "ghost"}
        size="sm"
        onClick={onOpenTemplate}
        className="gap-1.5 h-7 text-caption"
      >
        <LayoutTemplate className="size-3.5" />
        <span>{t(($) => $.designer.start_points.template)}</span>
      </Button>
    </div>
  );
}
