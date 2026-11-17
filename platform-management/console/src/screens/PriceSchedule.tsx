import { useEffect, useState } from "react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Badge } from "@multica/ui/components/ui/badge";
import { Spinner } from "@multica/ui/components/ui/spinner";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@multica/ui/components/ui/card";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@multica/ui/components/ui/table";
import { AlertTriangle, ArrowLeft } from "@multica/ui/icons";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import {
  usePriceChanges,
  useSchedulePriceChange,
  useCancelScheduledPriceChange,
} from "../api/hooks";
import { PlatformApiError } from "../api/client";
import type {
  Plan,
  PriceChange,
  PriceChangeDryRunResponse,
  PriceChangeState,
  PriceSnapshot,
} from "../api/schemas";

const LARGE_CHANGE_CONFIRM_PCT = 50;

export interface PriceScheduleProps {
  plan?: Plan;
  planKey?: string;
  readOnly?: boolean;
  onBack?: () => void;
}

function toDateTimeLocalUTC(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso.slice(0, 19);
  return date.toISOString().slice(0, 19);
}

function localUtcInputToRfc3339(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return "";
  if (/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) return `${trimmed}T00:00:00Z`;
  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(trimmed)) return `${trimmed}:00Z`;
  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$/.test(trimmed)) return `${trimmed}Z`;
  const parsed = new Date(trimmed);
  return Number.isNaN(parsed.getTime()) ? trimmed : parsed.toISOString();
}

function parseSeatInput(raw: string): number | null {
  const trimmed = raw.trim();
  if (trimmed === "") return null;
  const parsed = Number(trimmed);
  return Number.isFinite(parsed) ? parsed : NaN;
}

function computeMaxChangePct(
  plan: Plan | undefined,
  nextMonth: number | null,
  nextYear: number | null,
): number {
  if (!plan) return 0;
  let biggest = 0;
  const pairs: [number | null, number | null][] = [
    [plan.seat_price_month, nextMonth],
    [plan.seat_price_year, nextYear],
  ];
  for (const [oldVal, newVal] of pairs) {
    if (oldVal === null || oldVal <= 0 || newVal === null || !Number.isFinite(newVal)) {
      continue;
    }
    const pct = (Math.abs(newVal - oldVal) * 100) / oldVal;
    if (pct > biggest) biggest = pct;
  }
  return Math.round(biggest * 100) / 100;
}

function formatSeatValue(val: unknown, currency: string): string | null {
  if (typeof val !== "number") return null;
  const code = currency ? ` ${currency.toUpperCase()}` : "";
  return `${val.toLocaleString()}${code}`;
}

function formatPriceSnapshot(snapshot: PriceSnapshot, currency: string): string {
  const parts: string[] = [];
  if ("seat_price_month" in snapshot) {
    const formatted = formatSeatValue(snapshot.seat_price_month, currency);
    parts.push(`Mo: ${formatted ?? "-"}`);
  }
  if ("seat_price_year" in snapshot) {
    const formatted = formatSeatValue(snapshot.seat_price_year, currency);
    parts.push(`Yr: ${formatted ?? "-"}`);
  }
  for (const key of Object.keys(snapshot)) {
    if (key === "seat_price_month" || key === "seat_price_year") continue;
    parts.push(`${key}: ${JSON.stringify(snapshot[key])}`);
  }
  return parts.length > 0 ? parts.join(" · ") : "-";
}

function stateBadgeVariant(
  state: PriceChangeState,
): "default" | "secondary" | "destructive" | "outline" {
  switch (state) {
    case "scheduled":
      return "default";
    case "applied":
      return "secondary";
    case "canceled":
      return "destructive";
    default:
      return "outline";
  }
}

export function PriceSchedule({
  plan,
  planKey,
  readOnly = false,
  onBack,
}: PriceScheduleProps) {
  const resolvedPlanKey = plan?.key ?? planKey ?? "";
  const currency = plan?.currency ?? "";

  const {
    data: history,
    isLoading: historyLoading,
    isError: historyError,
    error: historyErrObj,
    refetch: refetchHistory,
  } = usePriceChanges(resolvedPlanKey, Boolean(resolvedPlanKey));

  const schedulePriceChange = useSchedulePriceChange();
  const cancelScheduledPriceChange = useCancelScheduledPriceChange();

  const [effectiveFrom, setEffectiveFrom] = useState("");
  const [seatPriceMonth, setSeatPriceMonth] = useState(() =>
    plan?.seat_price_month !== null && plan?.seat_price_month !== undefined
      ? String(plan.seat_price_month)
      : "",
  );
  const [seatPriceYear, setSeatPriceYear] = useState(() =>
    plan?.seat_price_year !== null && plan?.seat_price_year !== undefined
      ? String(plan.seat_price_year)
      : "",
  );
  const [reason, setReason] = useState("");
  const [confirmLargeChange, setConfirmLargeChange] = useState(false);
  const [serverConfirmPct, setServerConfirmPct] = useState<number | null>(null);
  const [minEffectiveFrom, setMinEffectiveFrom] = useState<string | null>(null);
  const [preview, setPreview] = useState<PriceChangeDryRunResponse | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [formNotice, setFormNotice] = useState<string | null>(null);
  const [cancelError, setCancelError] = useState<string | null>(null);

  useEffect(() => {
    setEffectiveFrom("");
    setSeatPriceMonth(
      plan?.seat_price_month !== null && plan?.seat_price_month !== undefined
        ? String(plan.seat_price_month)
        : "",
    );
    setSeatPriceYear(
      plan?.seat_price_year !== null && plan?.seat_price_year !== undefined
        ? String(plan.seat_price_year)
        : "",
    );
    setReason("");
    setConfirmLargeChange(false);
    setServerConfirmPct(null);
    setMinEffectiveFrom(null);
    setPreview(null);
    setFormError(null);
    setFormNotice(null);
    setCancelError(null);
  }, [resolvedPlanKey, plan?.seat_price_month, plan?.seat_price_year]);

  const parsedMonth = parseSeatInput(seatPriceMonth);
  const parsedYear = parseSeatInput(seatPriceYear);
  const clientMaxPct = computeMaxChangePct(plan, parsedMonth, parsedYear);
  const exceedsClientThreshold = clientMaxPct > LARGE_CHANGE_CONFIRM_PCT;
  const needsConfirm = exceedsClientThreshold || serverConfirmPct !== null;
  const warningPct = exceedsClientThreshold ? clientMaxPct : serverConfirmPct;

  const buildPayload = () => {
    const effectiveIso = localUtcInputToRfc3339(effectiveFrom);
    if (!effectiveIso) {
      setFormError("Effective from date/time (UTC) is required.");
      return null;
    }

    const nextPrices: { seat_price_month?: number; seat_price_year?: number } = {};
    if (parsedMonth !== null) {
      if (!Number.isInteger(parsedMonth) || parsedMonth < 0) {
        setFormError("Monthly seat price must be a whole number of 0 or more.");
        return null;
      }
      nextPrices.seat_price_month = parsedMonth;
    }
    if (parsedYear !== null) {
      if (!Number.isInteger(parsedYear) || parsedYear < 0) {
        setFormError("Yearly seat price must be a whole number of 0 or more.");
        return null;
      }
      nextPrices.seat_price_year = parsedYear;
    }

    if (
      nextPrices.seat_price_month === undefined &&
      nextPrices.seat_price_year === undefined
    ) {
      setFormError("Enter at least one seat price.");
      return null;
    }

    const trimmedReason = reason.trim();
    if (!trimmedReason) {
      setFormError("Reason is required.");
      return null;
    }
    if (trimmedReason.length > 500) {
      setFormError("Reason must be at most 500 characters.");
      return null;
    }

    return {
      key: resolvedPlanKey,
      effective_from: effectiveIso,
      new: nextPrices,
      reason: trimmedReason,
      confirm_large_change: confirmLargeChange,
    };
  };

  const handleApiError = (err: unknown) => {
    if (err instanceof PlatformApiError) {
      switch (err.code) {
        case "price_notice_too_short": {
          const minFrom =
            typeof err.details.min_effective_from === "string"
              ? err.details.min_effective_from
              : null;
          if (minFrom) {
            setMinEffectiveFrom(minFrom);
            setFormError(
              `Notice period is too short. Earliest effective date (UTC): ${minFrom}.`,
            );
          } else {
            setFormError(err.message);
          }
          return;
        }
        case "price_change_confirm_required": {
          const pct =
            typeof err.details.change_pct === "number"
              ? err.details.change_pct
              : clientMaxPct;
          setServerConfirmPct(pct);
          setFormError(
            `Price change of ${pct}% requires confirmation. Tick the confirmation checkbox to proceed.`,
          );
          return;
        }
        case "price_invalid": {
          const field =
            typeof err.details.field === "string" ? err.details.field : null;
          setFormError(field ? `${field}: ${err.message}` : err.message);
          return;
        }
        default:
          setFormError(err.message);
          return;
      }
    }
    setFormError(err instanceof Error ? err.message : "Failed to schedule price change");
  };

  const handlePreview = async () => {
    if (schedulePriceChange.isPending) return;
    setFormError(null);
    setFormNotice(null);
    const payload = buildPayload();
    if (!payload) return;

    try {
      const res = await schedulePriceChange.mutateAsync({
        ...payload,
        dryRun: true,
      });
      if ("dry_run" in res) {
        setPreview(res);
      }
    } catch (err: unknown) {
      setPreview(null);
      handleApiError(err);
    }
  };

  const handleSchedule = async (e: React.FormEvent) => {
    e.preventDefault();
    if (schedulePriceChange.isPending) return;
    if (needsConfirm && !confirmLargeChange) {
      setFormError("Confirm the large price change before scheduling.");
      return;
    }

    setFormError(null);
    setFormNotice(null);
    const payload = buildPayload();
    if (!payload) return;

    try {
      const res = await schedulePriceChange.mutateAsync({
        ...payload,
        dryRun: false,
      });
      if ("price_change" in res) {
        setPreview(null);
        setMinEffectiveFrom(null);
        setServerConfirmPct(null);
        setConfirmLargeChange(false);
        setReason("");
        setFormNotice(
          res.applied
            ? "Price change applied immediately."
            : `Price change scheduled for ${res.price_change.effective_from}.`,
        );
      }
    } catch (err: unknown) {
      handleApiError(err);
    }
  };

  const handleCancelScheduled = async () => {
    if (!resolvedPlanKey || cancelScheduledPriceChange.isPending) return;
    setCancelError(null);
    setFormNotice(null);
    try {
      await cancelScheduledPriceChange.mutateAsync({ key: resolvedPlanKey });
      setFormNotice("Scheduled price change canceled.");
    } catch (err: unknown) {
      setCancelError(
        err instanceof Error ? err.message : "Failed to cancel scheduled price change",
      );
    }
  };

  const invalidatePreviewAndConfirm = () => {
    setPreview(null);
    setServerConfirmPct(null);
  };

  return (
    <div className="space-y-6">
      {!readOnly && (
        <>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div className="space-y-1">
              {onBack && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={onBack}
                  className="-ml-2 mb-1 gap-1.5"
                >
                  <ArrowLeft className="size-4" />
                  <span>Back to plans</span>
                </Button>
              )}
              <h1 className="font-heading text-title-lg font-bold">
                Price Schedule: {plan?.name ?? resolvedPlanKey}
              </h1>
              <p className="text-body text-muted-foreground">
                Current prices — Month:{" "}
                {plan?.seat_price_month !== null && plan?.seat_price_month !== undefined
                  ? `${plan.seat_price_month.toLocaleString()} ${currency.toUpperCase()}`
                  : "-"}
                {" · "}
                Year:{" "}
                {plan?.seat_price_year !== null && plan?.seat_price_year !== undefined
                  ? `${plan.seat_price_year.toLocaleString()} ${currency.toUpperCase()}`
                  : "-"}
              </p>
            </div>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Schedule price change</CardTitle>
              <CardDescription>
                Preview affected subscribers before scheduling. Price increases with paid subscribers require advance notice in UTC.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={handleSchedule} className="space-y-4">
                <div className="grid gap-4 sm:grid-cols-3">
                  <div className="flex flex-col gap-1.5">
                    <label
                      htmlFor="price-effective-from"
                      className="text-caption font-medium text-foreground"
                    >
                      Effective from (UTC) <span className="text-destructive">*</span>
                    </label>
                    <Input
                      id="price-effective-from"
                      type="datetime-local"
                      step={1}
                      min={minEffectiveFrom ? toDateTimeLocalUTC(minEffectiveFrom) : undefined}
                      value={effectiveFrom}
                      onChange={(e) => {
                        setEffectiveFrom(e.target.value);
                        setPreview(null);
                      }}
                      required
                      disabled={schedulePriceChange.isPending}
                    />
                    {minEffectiveFrom && (
                      <p className="text-caption text-muted-foreground">
                        Minimum effective date (UTC): {minEffectiveFrom}
                      </p>
                    )}
                  </div>

                  <div className="flex flex-col gap-1.5">
                    <label
                      htmlFor="price-seat-month"
                      className="text-caption font-medium text-foreground"
                    >
                      Monthly seat price {currency ? `(${currency.toUpperCase()})` : ""}
                    </label>
                    <Input
                      id="price-seat-month"
                      type="number"
                      min={0}
                      step={1}
                      value={seatPriceMonth}
                      onChange={(e) => {
                        setSeatPriceMonth(e.target.value);
                        setConfirmLargeChange(false);
                        invalidatePreviewAndConfirm();
                      }}
                      disabled={schedulePriceChange.isPending}
                    />
                  </div>

                  <div className="flex flex-col gap-1.5">
                    <label
                      htmlFor="price-seat-year"
                      className="text-caption font-medium text-foreground"
                    >
                      Yearly seat price {currency ? `(${currency.toUpperCase()})` : ""}
                    </label>
                    <Input
                      id="price-seat-year"
                      type="number"
                      min={0}
                      step={1}
                      value={seatPriceYear}
                      onChange={(e) => {
                        setSeatPriceYear(e.target.value);
                        setConfirmLargeChange(false);
                        invalidatePreviewAndConfirm();
                      }}
                      disabled={schedulePriceChange.isPending}
                    />
                  </div>
                </div>

                <div className="flex flex-col gap-1.5">
                  <label
                    htmlFor="price-reason"
                    className="text-caption font-medium text-foreground"
                  >
                    Reason <span className="text-destructive">*</span>
                  </label>
                  <Textarea
                    id="price-reason"
                    value={reason}
                    onChange={(e) => {
                      setReason(e.target.value);
                      setPreview(null);
                    }}
                    maxLength={500}
                    rows={2}
                    placeholder="Reason for this price change..."
                    required
                    disabled={schedulePriceChange.isPending}
                  />
                </div>

                {needsConfirm && (
                  <div
                    role="status"
                    className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 space-y-2"
                  >
                    <div className="flex items-center gap-2 text-body font-medium text-foreground">
                      <AlertTriangle className="size-4 text-amber-600" />
                      <span>
                        Large price change ({warningPct}% exceeds {LARGE_CHANGE_CONFIRM_PCT}% threshold).
                      </span>
                    </div>
                    <label className="flex items-center gap-2 text-body">
                      <input
                        id="confirm-large-change"
                        type="checkbox"
                        checked={confirmLargeChange}
                        onChange={(e) => setConfirmLargeChange(e.target.checked)}
                        disabled={schedulePriceChange.isPending}
                        className="size-4 rounded border-surface-border text-primary focus:ring-primary"
                      />
                      <span>Confirm large price change</span>
                    </label>
                  </div>
                )}

                {preview && (
                  <div
                    role="status"
                    data-testid="price-dry-run-preview"
                    className="rounded-lg border border-surface-border bg-muted/40 p-3 text-body space-y-1"
                  >
                    <div className="font-medium text-foreground">Dry-run impact preview</div>
                    <div className="text-caption text-muted-foreground">
                      Affected subscribers:{" "}
                      <span className="font-medium text-foreground tabular-nums">
                        {preview.affected_subscribers}
                      </span>
                    </div>
                    <div className="text-caption text-muted-foreground">
                      Notice date:{" "}
                      <span className="font-medium text-foreground font-mono">
                        {preview.notice_date ?? "None (immediate effect)"}
                      </span>
                    </div>
                  </div>
                )}

                {formError && (
                  <div role="alert" className="text-caption text-destructive">
                    {formError}
                  </div>
                )}

                {formNotice && (
                  <div role="status" className="text-caption text-emerald-700 dark:text-emerald-300">
                    {formNotice}
                  </div>
                )}

                <div className="flex flex-wrap items-center gap-3">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={handlePreview}
                    disabled={schedulePriceChange.isPending}
                  >
                    {schedulePriceChange.isPending && <Spinner className="size-3.5" />}
                    Preview impact
                  </Button>
                  <Button
                    type="submit"
                    disabled={
                      schedulePriceChange.isPending || (needsConfirm && !confirmLargeChange)
                    }
                  >
                    {schedulePriceChange.isPending && <Spinner className="size-3.5" />}
                    Schedule
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>
        </>
      )}

      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="font-heading text-title-sm font-semibold">
            {readOnly ? `Changes for ${resolvedPlanKey}` : "Price Change History"}
          </h2>
        </div>

        {cancelError && (
          <div role="alert" className="text-caption text-destructive">
            {cancelError}
          </div>
        )}

        {historyLoading && <LoadingState message="Loading price change history..." />}

        {historyError && (
          <ErrorState
            title="Failed to load price change history"
            message={
              historyErrObj instanceof Error
                ? historyErrObj.message
                : "An error occurred while loading price changes."
            }
            onRetry={refetchHistory}
          />
        )}

        {!historyLoading && !historyError && (!history || history.length === 0) && (
          <EmptyState
            title="No price changes"
            description="No scheduled or past price changes recorded for this plan."
          />
        )}

        {!historyLoading && !historyError && history && history.length > 0 && (
          <div className="rounded-lg border border-surface-border bg-surface overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>State</TableHead>
                  <TableHead>Effective from (UTC)</TableHead>
                  <TableHead>Old prices</TableHead>
                  <TableHead>New prices</TableHead>
                  <TableHead>Reason</TableHead>
                  {!readOnly && <TableHead className="text-right">Actions</TableHead>}
                </TableRow>
              </TableHeader>
              <TableBody>
                {history.map((item: PriceChange) => (
                  <TableRow key={item.id}>
                    <TableCell>
                      <Badge variant={stateBadgeVariant(item.state)}>{item.state}</Badge>
                    </TableCell>
                    <TableCell className="font-mono text-caption">
                      {item.effective_from}
                    </TableCell>
                    <TableCell className="text-caption tabular-nums">
                      {formatPriceSnapshot(item.old, currency)}
                    </TableCell>
                    <TableCell className="text-caption font-medium tabular-nums">
                      {formatPriceSnapshot(item.new, currency)}
                    </TableCell>
                    <TableCell className="text-body">{item.reason}</TableCell>
                    {!readOnly && (
                      <TableCell className="text-right">
                        {item.state === "scheduled" && (
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            onClick={handleCancelScheduled}
                            disabled={cancelScheduledPriceChange.isPending}
                          >
                            {cancelScheduledPriceChange.isPending && (
                              <Spinner className="size-3.5" />
                            )}
                            Cancel scheduled change
                          </Button>
                        )}
                      </TableCell>
                    )}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </div>
  );
}
