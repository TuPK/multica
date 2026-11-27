"use client";

import { useState } from "react";
import { toast } from "sonner";
import { errorCode } from "@multica/core/api";
import {
  EFFECT_CLASSES,
  EFFECT_SOURCES,
  useEffectsInfinite,
  useUndoEffect,
  type Effect,
  type EffectClass,
  type EffectSource,
} from "@multica/core/durable/effects";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
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
  const { t: tGov } = useT("governance");
  const wsId = useWorkspaceId();
  const [effectClass, setEffectClass] = useState("all");
  const [source, setSource] = useState("all");
  const log = useEffectsInfinite(wsId, {
    limit: 200,
    effect_class: effectClass === "all" ? undefined : effectClass as EffectClass,
    source: source === "all" ? undefined : source as EffectSource,
  });
  const undo = useUndoEffect(wsId);

  const effects: Effect[] = log.data?.pages.flat() ?? [];
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

  const classItems = [
    { value: "all", label: t(($) => $.outcomes.tab.all) },
    ...EFFECT_CLASSES.map((value) => ({ value, label: t(($) => $.intent.effect_class[value]) })),
  ];
  const sourceItems = [
    { value: "all", label: t(($) => $.outcomes.tab.all) },
    ...EFFECT_SOURCES.map((value) => ({ value, label: t(($) => $.intent.source[value]) })),
  ];

  return (
    <div className={cn(PAGE_GUTTER, "flex flex-col gap-8 py-4")}>
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-2 text-caption">
          {t(($) => $.effects.class_filter)}
          <Select
            items={classItems}
            value={effectClass}
            onValueChange={(next) => {
              if (next) setEffectClass(next);
            }}
          >
            <SelectTrigger aria-label={t(($) => $.effects.class_filter)} className="w-44">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {classItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </label>
        <label className="flex items-center gap-2 text-caption">
          {t(($) => $.effects.source_filter)}
          <Select
            items={sourceItems}
            value={source}
            onValueChange={(next) => {
              if (next) setSource(next);
            }}
          >
            <SelectTrigger aria-label={t(($) => $.effects.source_filter)} className="w-44">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {sourceItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </label>
      </div>
      {log.isError ? (
        <p role="alert" className="text-caption text-destructive">{t(($) => $.effects.load_failed)}</p>
      ) : !log.isPending && effects.length === 0 ? (
        <Empty><EmptyHeader><EmptyTitle>{t(($) => $.effects.empty)}</EmptyTitle></EmptyHeader></Empty>
      ) : (
        <>
          <EffectSection id="effects-controlled" title={t(($) => $.effects.controlled)} effects={controlled} onUndo={onUndo} undoing={undo.isPending} />
          <EffectSection id="effects-declared" title={t(($) => $.effects.declared)} hint={t(($) => $.effects.declared_hint)} effects={declared} />
          {log.hasNextPage ? (
            <Button variant="outline" className="self-start" disabled={log.isFetching} onClick={() => void log.fetchNextPage()}>
              {tGov(($) => $.log.load_more)}
            </Button>
          ) : null}
        </>
      )}
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
