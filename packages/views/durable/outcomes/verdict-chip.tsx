"use client";

import type { OutcomeVerdict } from "@multica/core/durable/outcomes";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";

// The colour follows the verdict's meaning: a pass and a fail read apart at a glance, and an
// unverified or pending result stays neutral, because nothing was checked yet.
const TONE: Record<OutcomeVerdict, string> = {
  pass: "text-success",
  partial: "text-warning",
  fail: "text-destructive",
  unverified: "text-muted-foreground",
  pending: "text-muted-foreground",
};

/** A verdict as a small label. `label` replaces the verdict's own text, such as a countdown. */
export function VerdictChip({ verdict, label }: { verdict: OutcomeVerdict; label?: string }) {
  const { t } = useT("durable");
  const text = label ?? t(($) => $.outcomes.verdict[verdict]);
  return (
    <span className={cn("inline-flex w-fit items-center rounded-md bg-muted px-1.5 py-0.5 text-caption font-medium", TONE[verdict])}>
      {text}
    </span>
  );
}
