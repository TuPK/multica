import { useEffect, useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import { Divider } from "@astryxdesign/core/Divider";
import { Heading } from "@astryxdesign/core/Heading";
import { Section } from "@astryxdesign/core/Section";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@astryxdesign/core/Button";
import { Banner } from "@astryxdesign/core/Banner";
import { CheckboxInput } from "@astryxdesign/core/CheckboxInput";
import { TextArea } from "@astryxdesign/core/TextArea";
import { NumberInput } from "@astryxdesign/core/NumberInput";
import { DateTimeInput, type ISODateTimeString } from "@astryxdesign/core/DateTimeInput";
import { CalendarClock } from "@multica/ui/icons";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { RowActions } from "../components/RowActions";
import { StatusBadge } from "../components/StatusBadge";
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
  PriceSnapshot,
} from "../api/schemas";
import { formatMinorAmount } from "../money";
import { t } from "../i18n";

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
  return formatMinorAmount(val, currency);
}

function formatPriceSnapshot(snapshot: PriceSnapshot, currency: string): string {
  const parts: string[] = [];
  if ("seat_price_month" in snapshot) {
    const formatted = formatSeatValue(snapshot.seat_price_month, currency);
    parts.push(`${t("Mo")}: ${formatted ?? "-"}`);
  }
  if ("seat_price_year" in snapshot) {
    const formatted = formatSeatValue(snapshot.seat_price_year, currency);
    parts.push(`${t("Yr")}: ${formatted ?? "-"}`);
  }
  for (const key of Object.keys(snapshot)) {
    if (key === "seat_price_month" || key === "seat_price_year") continue;
    parts.push(`${key}: ${JSON.stringify(snapshot[key])}`);
  }
  return parts.length > 0 ? parts.join(" · ") : "-";
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
      setFormError(t("Effective from date/time (UTC) is required."));
      return null;
    }

    const nextPrices: { seat_price_month?: number; seat_price_year?: number } = {};
    if (parsedMonth !== null) {
      if (!Number.isInteger(parsedMonth) || parsedMonth < 0) {
        setFormError(t("Monthly seat price must be a whole number of 0 or more."));
        return null;
      }
      nextPrices.seat_price_month = parsedMonth;
    }
    if (parsedYear !== null) {
      if (!Number.isInteger(parsedYear) || parsedYear < 0) {
        setFormError(t("Yearly seat price must be a whole number of 0 or more."));
        return null;
      }
      nextPrices.seat_price_year = parsedYear;
    }

    if (
      nextPrices.seat_price_month === undefined &&
      nextPrices.seat_price_year === undefined
    ) {
      setFormError(t("Enter at least one seat price."));
      return null;
    }

    const trimmedReason = reason.trim();
    if (!trimmedReason) {
      setFormError(t("Reason is required."));
      return null;
    }
    if (trimmedReason.length > 500) {
      setFormError(t("Reason must be at most 500 characters."));
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
              t("Notice period is too short. Earliest effective date (UTC): {0}.", minFrom),
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
            t("Price change of {0}% requires confirmation. Tick the confirmation checkbox to proceed.", pct),
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
    setFormError(err instanceof Error ? err.message : t("Failed to schedule price change"));
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
      setFormError(t("Confirm the large price change before scheduling."));
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
            ? t("Price change applied immediately.")
            : t("Price change scheduled for {0}.", res.price_change.effective_from),
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
      setFormNotice(t("Scheduled price change canceled."));
    } catch (err: unknown) {
      setCancelError(
        err instanceof Error ? err.message : t("Failed to cancel scheduled price change"),
      );
    }
  };

  const invalidatePreviewAndConfirm = () => {
    setPreview(null);
    setServerConfirmPct(null);
  };

  return (
    <div className="space-y-8">
      {!readOnly && (
        <>
          <PageHeader
            title={t("Price Schedule: {0}", plan?.name ?? resolvedPlanKey)}
            backAction={onBack ? { label: t("Back to plans"), onClick: onBack } : undefined}
            meta={
              <span className="tabular-nums">
                {t("Current prices — Month:")}{" "}
                {plan?.seat_price_month !== null && plan?.seat_price_month !== undefined
                  ? formatMinorAmount(plan.seat_price_month, currency)
                  : "-"}
                {" · "}
                {t("Year:")}{" "}
                {plan?.seat_price_year !== null && plan?.seat_price_year !== undefined
                  ? formatMinorAmount(plan.seat_price_year, currency)
                  : "-"}
              </span>
            }
          />

          <Section variant="transparent" padding={0} className="space-y-4">
            <div className="space-y-1">
              <Heading level={2}>{t("Schedule price change")}</Heading>
              <Text type="supporting" as="p">
                {t("Preview affected subscribers before scheduling. Price increases with paid subscribers require advance notice in UTC.")}
              </Text>
            </div>

            <Card padding={5}>
              <form onSubmit={handleSchedule} className="space-y-4">
              <div className="grid gap-4 sm:grid-cols-3">
                <div className="flex flex-col gap-2">
                  <DateTimeInput
                    id="price-effective-from"
                    label={t("Effective from (UTC)")}
                    isRequired
                    width="100%"
                    min={minEffectiveFrom ? (toDateTimeLocalUTC(minEffectiveFrom) as ISODateTimeString) : undefined}
                    value={(effectiveFrom || undefined) as ISODateTimeString | undefined}
                    onChange={(value) => {
                      setEffectiveFrom(value ?? "");
                      setPreview(null);
                    }}
                    isDisabled={schedulePriceChange.isPending}
                    description={
                      minEffectiveFrom
                        ? `${t("Minimum effective date (UTC):")} ${minEffectiveFrom}`
                        : undefined
                    }
                  />
                </div>

                <NumberInput
                  id="price-seat-month"
                  min={0}
                  step={1}
                  isIntegerOnly
                  label={`${t("Monthly seat price")}${currency ? ` (${currency.toUpperCase()})` : ""}`}
                  width="100%"
                  value={seatPriceMonth.trim() === "" ? null : Number(seatPriceMonth)}
                  onChange={(value) => {
                    setSeatPriceMonth(value === null ? "" : String(value));
                    setConfirmLargeChange(false);
                    invalidatePreviewAndConfirm();
                  }}
                  isDisabled={schedulePriceChange.isPending}
                />

                <NumberInput
                  id="price-seat-year"
                  min={0}
                  step={1}
                  isIntegerOnly
                  label={`${t("Yearly seat price")}${currency ? ` (${currency.toUpperCase()})` : ""}`}
                  width="100%"
                  value={seatPriceYear.trim() === "" ? null : Number(seatPriceYear)}
                  onChange={(value) => {
                    setSeatPriceYear(value === null ? "" : String(value));
                    setConfirmLargeChange(false);
                    invalidatePreviewAndConfirm();
                  }}
                  isDisabled={schedulePriceChange.isPending}
                />
              </div>

              <TextArea
                id="price-reason"
                label={t("Reason")}
                isRequired
                width="100%"
                value={reason}
                onChange={(value) => {
                  setReason(value.slice(0, 500));
                  setPreview(null);
                }}
                rows={2}
                placeholder={t("Reason for this price change...")}
                isDisabled={schedulePriceChange.isPending}
              />

              {needsConfirm && (
                <Banner
                  status="info"
                  role="status"
                  title={
                    <>
                      {t("Large price change (")}
                      {warningPct}
                      {t("% exceeds")} {LARGE_CHANGE_CONFIRM_PCT}
                      {t("% threshold).")}
                    </>
                  }
                  description={
                    <CheckboxInput
                      label={t("Confirm large price change")}
                      value={confirmLargeChange}
                      onChange={(checked) => setConfirmLargeChange(checked)}
                      isDisabled={schedulePriceChange.isPending}
                    />
                  }
                />
              )}

              {preview && (
                <div
                  role="status"
                  data-testid="price-dry-run-preview"
                  className="space-y-1 rounded-lg border border-surface-border bg-muted/40 p-3 text-body"
                >
                  <div className="font-medium text-foreground">{t("Dry-run impact preview")}</div>
                  <div className="text-caption text-muted-foreground">
                    {t("Affected subscribers:")}{" "}
                    <span className="font-medium text-foreground tabular-nums">
                      {preview.affected_subscribers}
                    </span>
                  </div>
                  <div className="text-caption text-muted-foreground">
                    {t("Notice date:")}{" "}
                    <span className="font-mono font-medium text-foreground">
                      {preview.notice_date ?? t("None (immediate effect)")}
                    </span>
                  </div>
                </div>
              )}

              {formError && (
                <Banner
                  status="error"
                  role="alert"
                  title={formError}
                />
              )}

              {formNotice && (
                <Banner
                  status="info"
                  role="status"
                  title={formNotice}
                />
              )}

              <div className="flex flex-wrap items-center gap-3">
                <Button
                  type="button"
                  label={t("Preview impact")}
                  variant="secondary"
                  isLoading={schedulePriceChange.isPending}
                  onClick={handlePreview}
                  aria-busy={schedulePriceChange.isPending}
                  isDisabled={schedulePriceChange.isPending}
                />
                <Button
                  type="submit"
                  label={t("Schedule")}
                  variant="primary"
                  isLoading={schedulePriceChange.isPending}
                  aria-busy={schedulePriceChange.isPending}
                  isDisabled={
                    schedulePriceChange.isPending || (needsConfirm && !confirmLargeChange)
                  }
                />
              </div>
              </form>
            </Card>
          </Section>

          <Divider />
        </>
      )}

      <Section variant="transparent" padding={0} className="space-y-4">
        <Heading level={2}>
          {readOnly ? t("Changes for {0}", resolvedPlanKey) : t("Price Change History")}
        </Heading>

        {cancelError && (
          <Banner
            status="error"
            role="alert"
            title={cancelError}
          />
        )}

        {historyLoading && <LoadingState message={t("Loading price change history...")} />}

        {historyError && (
          <ErrorState
            title={t("Failed to load price change history")}
            message={
              historyErrObj instanceof Error
                ? historyErrObj.message
                : t("An error occurred while loading price changes.")
            }
            onRetry={refetchHistory}
          />
        )}

        {!historyLoading && !historyError && (!history || history.length === 0) && (
          <EmptyState
            icon={<CalendarClock className="size-5" aria-hidden="true" />}
            title={t("No price changes")}
            description={t("No scheduled or past price changes recorded for this plan.")}
          />
        )}

        {!historyLoading && !historyError && history && history.length > 0 && (
          <Card padding={0} className="overflow-x-auto">
            <Table density="compact" dividers="rows" hasHover>
              <TableHeader>
                <TableRow isHeaderRow>
                  <TableHeaderCell>{t("State")}</TableHeaderCell>
                  <TableHeaderCell>{t("Effective from (UTC)")}</TableHeaderCell>
                  <TableHeaderCell>{t("Old prices")}</TableHeaderCell>
                  <TableHeaderCell>{t("New prices")}</TableHeaderCell>
                  <TableHeaderCell>{t("Reason")}</TableHeaderCell>
                  {!readOnly && <TableHeaderCell className="text-right">{t("Actions")}</TableHeaderCell>}
                </TableRow>
              </TableHeader>
              <TableBody>
                {history.map((item: PriceChange) => (
                  <TableRow key={item.id}>
                    <TableCell>
                      <StatusBadge status={item.state} />
                    </TableCell>
                    <TableCell>
                      <Text type="code" hasTabularNumbers>
                        {item.effective_from}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting" hasTabularNumbers>
                        {formatPriceSnapshot(item.old, currency)}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting" color="primary" weight="medium" hasTabularNumbers>
                        {formatPriceSnapshot(item.new, currency)}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <Text type="body">{item.reason}</Text>
                    </TableCell>
                    {!readOnly && (
                      <TableCell className="text-right">
                        <RowActions
                          label={t("price change {0}", item.effective_from)}
                          actions={
                            item.state === "scheduled"
                              ? [
                                  {
                                    label: t("Cancel scheduled change"),
                                    busy: cancelScheduledPriceChange.isPending,
                                    onSelect: () => handleCancelScheduled(),
                                  },
                                ]
                              : []
                          }
                        />
                      </TableCell>
                    )}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Card>
        )}
      </Section>
    </div>
  );
}
