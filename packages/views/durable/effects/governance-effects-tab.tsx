"use client";

import { toast } from "sonner";
import { errorCode } from "@multica/core/api";
import { useEffects, useUndoEffect, type Effect } from "@multica/core/durable/effects";
import { useWorkspaceId } from "@multica/core/hooks";
import { Empty, EmptyHeader, EmptyTitle } from "@multica/ui/components/ui/empty";
import { PAGE_GUTTER } from "../../layout/page-header";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { EffectRow } from "./effect-row";

/**
 * The effect log (durable spec 4.7): what the platform controlled, and apart from it what
 * agents only declared. A declared effect was recorded, not stopped, so it is set apart rather
 * than read as controlled. Undo is offered only inside the window the server opens.
 */
export function GovernanceEffectsTab() {
  const { t } = useT("durable");
  const wsId = useWorkspaceId();
  const log = useEffects(wsId, { limit: 200 });
  const undo = useUndoEffect(wsId);

  const effects: Effect[] = log.data ?? [];
  const controlled = effects.filter((e) => e.source !== "declared");
  const declared = effects.filter((e) => e.source === "declared");

  const onUndo = (id: string) => {
    undo.mutateAsync(id).then(
      (result) => {
        if (result) toast.success(t(($) => $.effects.undo_done));
        else toast.error(t(($) => $.effects.undo_failed));
      },
      (err: unknown) => {
        toast.error(
          errorCode(err) === "undo_window_closed"
            ? t(($) => $.effects.undo_closed)
            : t(($) => $.effects.undo_failed),
        );
      },
    );
  };

  if (log.isError) {
    return (
      <div className={cn(PAGE_GUTTER, "py-4")}>
        <p role="alert" className="text-caption text-destructive">
          {t(($) => $.effects.load_failed)}
        </p>
      </div>
    );
  }

  if (!log.isPending && effects.length === 0) {
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>{t(($) => $.effects.empty)}</EmptyTitle>
        </EmptyHeader>
      </Empty>
    );
  }

  return (
    <div className={cn(PAGE_GUTTER, "flex flex-col gap-8 py-4")}>
      <EffectSection
        id="effects-controlled"
        title={t(($) => $.effects.controlled)}
        effects={controlled}
        onUndo={onUndo}
        undoing={undo.isPending}
      />
      <EffectSection
        id="effects-declared"
        title={t(($) => $.effects.declared)}
        hint={t(($) => $.effects.declared_hint)}
        effects={declared}
      />
    </div>
  );
}

function EffectSection({
  id,
  title,
  hint,
  effects,
  onUndo,
  undoing,
}: {
  id: string;
  title: string;
  hint?: string;
  effects: Effect[];
  onUndo?: (id: string) => void;
  undoing?: boolean;
}) {
  const headingId = `${id}-heading`;
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-2">
      <div className="flex flex-col gap-0.5">
        <h2 id={headingId} className="text-title-sm font-medium">
          {title}
        </h2>
        {hint ? <p className="text-caption text-muted-foreground">{hint}</p> : null}
      </div>
      <ul className="flex flex-col divide-y">
        {effects.map((effect) => (
          <EffectRow key={effect.id} effect={effect} onUndo={onUndo} undoing={undoing} />
        ))}
      </ul>
    </section>
  );
}
