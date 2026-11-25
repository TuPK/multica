"use client";

import type { Effect } from "@multica/core/durable/effects";
import { Button } from "@multica/ui/components/ui/button";
import { Undo2 } from "@multica/ui/icons";
import { useLocale, useT } from "../../i18n";

/**
 * One effect event: what was done or held, where, and by which source (durable spec 4.4).
 * The undo button appears only while the server says the undo window is open.
 */
export function EffectRow({
  effect,
  onUndo,
  undoing = false,
}: {
  effect: Effect;
  onUndo?: (id: string) => void;
  undoing?: boolean;
}) {
  const { t } = useT("durable");
  const locale = useLocale();
  const when = new Date(effect.created_at);
  const undoUntil = effect.undo_until ? new Date(effect.undo_until) : null;

  return (
    <li className="flex flex-wrap items-start justify-between gap-3 py-3">
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="break-words text-body text-foreground">{effect.summary || effect.target}</span>
        <span className="flex flex-wrap items-center gap-x-2 text-caption text-muted-foreground">
          <time dateTime={effect.created_at}>{when.toLocaleString(locale)}</time>
          <span aria-hidden>·</span>
          <span>{t(($) => $.intent.effect_class[effect.effect_class])}</span>
          <span aria-hidden>·</span>
          <span>{t(($) => $.intent.source[effect.source])}</span>
          <span aria-hidden>·</span>
          <span>{t(($) => $.effects.decision[effect.decision])}</span>
          <span aria-hidden>·</span>
          <span>{t(($) => $.effects.status[effect.status])}</span>
        </span>
      </div>
      {effect.undo_available && onUndo ? (
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={undoing}
          aria-busy={undoing}
          title={undoUntil ? t(($) => $.effects.undo_until, { time: undoUntil.toLocaleString(locale) }) : undefined}
          onClick={() => onUndo(effect.id)}
        >
          <Undo2 className="h-3.5 w-3.5" />
          {t(($) => $.effects.undo)}
        </Button>
      ) : null}
    </li>
  );
}
