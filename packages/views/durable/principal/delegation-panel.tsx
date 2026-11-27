"use client";

import { useState } from "react";
import { toast } from "sonner";
import {
  DEFAULT_DELEGATION_MIN_CONFIDENCE,
  isForbiddenDelegationKind,
  useDisablePrincipalDelegation,
  useEnablePrincipalDelegation,
  usePrincipalDelegations,
  type PrincipalDelegation,
  type PrincipalKindAccuracy,
} from "@multica/core/durable/principal";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Loader2, Lock } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { decisionKindLabel, principalErrorMessage } from "./error-copy";

// Standard decision kinds from durable spec 6.2 & 6.5. Delegatable kinds show live delegation
// status and accuracy from GET /api/principal/delegations; forbidden kinds stay locked in the UI
// (the server enforces the same rule).
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
 * Per-kind delegation status, accuracy and enable/disable controls (durable spec 6.5).
 * Reads active/latest-disabled delegations and per-kind accuracy from GET /api/principal/delegations.
 */
export function DelegationPanel({ wsId, extraKinds = [] }: { wsId: string; extraKinds?: string[] }) {
  const { t } = useT("durable");
  const delegationsQuery = usePrincipalDelegations(wsId);
  const enable = useEnablePrincipalDelegation(wsId);
  const disable = useDisablePrincipalDelegation(wsId);
  const [minConfidence, setMinConfidence] = useState(String(DEFAULT_DELEGATION_MIN_CONFIDENCE));
  const [busyKind, setBusyKind] = useState<string | null>(null);

  const parsedConfidence = Number(minConfidence);
  const validConfidence =
    minConfidence.trim() !== "" &&
    Number.isFinite(parsedConfidence) &&
    parsedConfidence > 0 &&
    parsedConfidence <= 1;

  const apiDelegations = delegationsQuery.data?.delegations ?? [];
  const apiKinds = delegationsQuery.data?.kinds ?? [];

  const delegationByKind = new Map<string, PrincipalDelegation>(
    apiDelegations.map((entry) => [entry.decision_kind, entry]),
  );
  const accuracyByKind = new Map<string, PrincipalKindAccuracy>(
    apiKinds.map((entry) => [entry.decision_kind, entry]),
  );

  const discoveredKinds = [
    ...extraKinds,
    ...apiDelegations.map((d) => d.decision_kind),
    ...apiKinds.map((k) => k.decision_kind),
  ];
  const delegatable = uniqueKinds([
    ...DELEGATABLE_KINDS,
    ...discoveredKinds.filter((k) => !isForbiddenDelegationKind(k)),
  ]);
  const locked = uniqueKinds([
    ...LOCKED_KINDS,
    ...discoveredKinds.filter((k) => isForbiddenDelegationKind(k)),
  ]);

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

      {delegationsQuery.isError ? (
        <p role="alert" className="text-caption text-destructive">
          {t(($) => $.principal.delegation.load_failed)}
        </p>
      ) : delegationsQuery.isPending ? (
        <div className="flex items-center py-4">
          <Loader2 className="size-4 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <>
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
              const delegation = delegationByKind.get(kind);
              const accuracy = accuracyByKind.get(kind);
              const isOn = delegation?.enabled === true;
              const eligible = accuracy?.eligible === true;
              const isBusy = busyKind === kind;
              return (
                <li key={kind} className="flex flex-wrap items-center justify-between gap-3 px-3 py-2.5">
                  <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-body font-medium text-foreground">{label}</span>
                      <span className="inline-flex items-center rounded-md border border-surface-border px-1.5 py-0.5 text-caption text-foreground">
                        {isOn
                          ? t(($) => $.principal.delegation.status_on)
                          : t(($) => $.principal.delegation.status_off)}
                      </span>
                      {isOn && delegation ? (
                        <span className="text-caption tabular-nums text-muted-foreground">
                          {t(($) => $.principal.delegation.min_confidence_value, {
                            percent: Math.round(delegation.min_confidence * 100),
                          })}
                        </span>
                      ) : null}
                    </div>
                    <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-caption text-muted-foreground">
                      <code className="font-mono">{kind}</code>
                      <span className="tabular-nums">
                        {accuracy && accuracy.n > 0
                          ? t(($) => $.principal.delegation.accuracy, {
                              matched: accuracy.matched,
                              n: accuracy.n,
                              lower: Math.round(accuracy.lower * 100),
                            })
                          : t(($) => $.principal.delegation.accuracy_none)}
                      </span>
                    </div>
                    {!isOn && !eligible ? (
                      <p className="text-caption text-muted-foreground">
                        {t(($) => $.principal.delegation.not_eligible_reason)}
                      </p>
                    ) : null}
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    {isOn ? (
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
                    ) : (
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={!eligible || !validConfidence || busyKind !== null}
                        aria-busy={isBusy || undefined}
                        aria-label={`${t(($) => $.principal.delegation.enable)} ${label}`}
                        onClick={() => handleEnable(kind)}
                      >
                        {isBusy && enable.isPending ? <Loader2 className="size-3.5 animate-spin" /> : null}
                        {t(($) => $.principal.delegation.enable)}
                      </Button>
                    )}
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
        </>
      )}
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
