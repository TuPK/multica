"use client";

import { useState } from "react";
import { toast } from "sonner";
import {
  DEFAULT_DELEGATION_MIN_CONFIDENCE,
  isForbiddenDelegationKind,
  useDisablePrincipalDelegation,
  useEnablePrincipalDelegation,
} from "@multica/core/durable/principal";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Loader2, Lock } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { decisionKindLabel, principalErrorMessage } from "./error-copy";

// Standard decision kinds from durable spec 6.2 & 6.5. Delegatable kinds expose enable/disable
// actions; forbidden kinds are locked in the UI for clarity (the server enforces the same list).
// No read endpoint exists for active delegations or prediction accuracy, so neither is displayed.
const DELEGATABLE_KINDS = [
  "general",
  "inquiry",
  "outcome",
  "proposal:general",
  "proposal:hire",
  "proposal:promote",
  "effect:write",
  "effect:send",
] as const;

const LOCKED_KINDS = [
  "proposal:org_change",
  "proposal:launch",
  "proposal:stage_gate",
  "effect:spend",
  "effect:share",
  "proposal:budget",
  "proposal:autonomy",
  "proposal:envelope",
  "proposal:policy",
  "proposal:profile",
] as const;

/**
 * Per-kind delegation actions (durable spec 6.5). Because the server has no read endpoint for
 * active delegations or prediction accuracy, this panel shows enable/disable actions without
 * inventing current state or accuracy figures.
 */
export function DelegationPanel({ extraKinds = [] }: { extraKinds?: string[] }) {
  const { t } = useT("durable");
  const enable = useEnablePrincipalDelegation();
  const disable = useDisablePrincipalDelegation();
  const [minConfidence, setMinConfidence] = useState(String(DEFAULT_DELEGATION_MIN_CONFIDENCE));
  const [busyKind, setBusyKind] = useState<string | null>(null);

  const parsedConfidence = Number(minConfidence);
  const validConfidence =
    minConfidence.trim() !== "" &&
    Number.isFinite(parsedConfidence) &&
    parsedConfidence > 0 &&
    parsedConfidence <= 1;

  const delegatable = uniqueKinds([...DELEGATABLE_KINDS, ...extraKinds.filter((k) => !isForbiddenDelegationKind(k))]);
  const locked = uniqueKinds([...LOCKED_KINDS, ...extraKinds.filter((k) => isForbiddenDelegationKind(k))]);

  const handleEnable = (kind: string) => {
    if (isForbiddenDelegationKind(kind) || !validConfidence || busyKind !== null) return;
    setBusyKind(kind);
    enable.mutateAsync({ decision_kind: kind, min_confidence: parsedConfidence }).then(
      () => {
        toast.success(t(($) => $.principal.delegation.enabled, { kind: decisionKindLabel(kind, t) }));
        setBusyKind((prev) => (prev === kind ? null : prev));
      },
      (err: unknown) => {
        toast.error(principalErrorMessage(err, t, t(($) => $.principal.delegation.enable_failed)));
        setBusyKind((prev) => (prev === kind ? null : prev));
      },
    );
  };

  const handleDisable = (kind: string) => {
    if (busyKind !== null) return;
    setBusyKind(kind);
    disable.mutateAsync(kind).then(
      () => {
        toast.success(t(($) => $.principal.delegation.disabled, { kind: decisionKindLabel(kind, t) }));
        setBusyKind((prev) => (prev === kind ? null : prev));
      },
      (err: unknown) => {
        toast.error(principalErrorMessage(err, t, t(($) => $.principal.delegation.disable_failed)));
        setBusyKind((prev) => (prev === kind ? null : prev));
      },
    );
  };

  return (
    <section aria-labelledby="principal-delegation-heading" className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h2 id="principal-delegation-heading" className="text-title-sm font-medium">
          {t(($) => $.principal.delegation.title)}
        </h2>
        <p className="text-caption text-muted-foreground">{t(($) => $.principal.delegation.rules_note)}</p>
      </div>

      <div className="flex items-end gap-3">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="principal-delegation-confidence" className="text-caption font-medium">
            {t(($) => $.principal.delegation.min_confidence)}
          </Label>
          <Input
            id="principal-delegation-confidence"
            type="number"
            inputMode="decimal"
            min={0.01}
            max={1}
            step={0.05}
            value={minConfidence}
            aria-invalid={!validConfidence}
            onChange={(e) => setMinConfidence(e.target.value)}
            className="w-28"
          />
        </div>
      </div>

      <ul className="flex flex-col divide-y divide-surface-border rounded-md border border-surface-border">
        {delegatable.map((kind) => {
          const label = decisionKindLabel(kind, t);
          const isBusy = busyKind === kind;
          return (
            <li key={kind} className="flex flex-wrap items-center justify-between gap-3 px-3 py-2.5">
              <div className="flex min-w-0 flex-col">
                <span className="text-body font-medium text-foreground">{label}</span>
                <code className="font-mono text-caption text-muted-foreground">{kind}</code>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={!validConfidence || busyKind !== null}
                  aria-busy={isBusy || undefined}
                  aria-label={`${t(($) => $.principal.delegation.enable)} ${label}`}
                  onClick={() => handleEnable(kind)}
                >
                  {isBusy && enable.isPending ? <Loader2 className="size-3.5 animate-spin" /> : null}
                  {t(($) => $.principal.delegation.enable)}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  disabled={busyKind !== null}
                  aria-busy={isBusy || undefined}
                  aria-label={`${t(($) => $.principal.delegation.disable)} ${label}`}
                  onClick={() => handleDisable(kind)}
                >
                  {isBusy && disable.isPending ? <Loader2 className="size-3.5 animate-spin" /> : null}
                  {t(($) => $.principal.delegation.disable)}
                </Button>
              </div>
            </li>
          );
        })}

        {locked.map((kind) => {
          const label = decisionKindLabel(kind, t);
          return (
            <li key={kind} className="flex flex-wrap items-center justify-between gap-3 px-3 py-2.5 opacity-75">
              <div className="flex min-w-0 flex-col">
                <span className="flex items-center gap-1.5 text-body font-medium text-foreground">
                  <Lock className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
                  {label}
                </span>
                <div className="flex flex-wrap items-center gap-2 text-caption text-muted-foreground">
                  <code className="font-mono">{kind}</code>
                  <span>{t(($) => $.principal.delegation.locked_reason)}</span>
                </div>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled
                aria-label={`${t(($) => $.principal.delegation.enable)} ${label}`}
              >
                {t(($) => $.principal.delegation.locked)}
              </Button>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function uniqueKinds(kinds: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const kind of kinds) {
    const trimmed = kind.trim();
    if (!trimmed || seen.has(trimmed)) continue;
    seen.add(trimmed);
    out.push(trimmed);
  }
  return out;
}
