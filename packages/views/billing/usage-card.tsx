"use client";

import type { WorkspaceUsagePeriod } from "@multica/core/types";
import { AlertTriangle, RefreshCw } from "@multica/ui/icons";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@multica/ui/components/ui/alert";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from "@multica/ui/components/ui/progress";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { useLocale, useT } from "../i18n";
import { formatStripeMinorAmount } from "../settings/components/billing-format";
import {
  SettingsCard,
  SettingsRow,
  SettingsSection,
} from "../settings/components/settings-layout";

const BINARY_UNITS = ["B", "KiB", "MiB", "GiB", "TiB"] as const;

export function formatUsageMoney(
  amount: number,
  currency: string,
  locale: string,
): string {
  return (
    formatStripeMinorAmount(amount, currency, locale) ??
    new Intl.NumberFormat(locale).format(amount)
  );
}

export function formatUsageBytes(bytes: number, locale: string): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "0 B";
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < BINARY_UNITS.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  const formatted = new Intl.NumberFormat(locale, {
    maximumFractionDigits: unitIndex === 0 ? 0 : 2,
  }).format(value);
  return `${formatted} ${BINARY_UNITS[unitIndex]}`;
}

export function formatUsageQuantity(
  meterKey: string,
  quantity: number,
  locale: string,
): string {
  if (meterKey === "storage_bytes") {
    return formatUsageBytes(quantity, locale);
  }
  return new Intl.NumberFormat(locale).format(quantity);
}

export function formatUsageUnitPer(
  meterKey: string,
  per: number,
  locale: string,
): string {
  if (meterKey === "storage_bytes") {
    return formatUsageBytes(per, locale);
  }
  return new Intl.NumberFormat(locale).format(per);
}

export function formatUsageUtcDate(
  value: string | null | undefined,
  locale: string,
): string | null {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeZone: "UTC",
  }).format(date);
}

export function resolveMeterLabel(
  meterKey: string,
  t: ReturnType<typeof useT<"billing">>["t"],
): string {
  switch (meterKey) {
    case "tasks_run":
      return t(($) => $.workspace.usage.meters.tasks_run);
    case "storage_bytes":
      return t(($) => $.workspace.usage.meters.storage_bytes);
    case "members":
      return t(($) => $.workspace.usage.meters.members);
    default:
      return t(($) => $.workspace.usage.meters.unknown);
  }
}

export interface UsageCardProps {
  usage: WorkspaceUsagePeriod | null;
  loading?: boolean;
  error?: boolean;
  onRetry?: () => void;
}

/** Displays accrued metered usage and credit-limit progress for the current UTC period. */
export function UsageCard({
  usage,
  loading = false,
  error = false,
  onRetry,
}: UsageCardProps) {
  const { t } = useT("billing");
  const locale = useLocale();

  if (loading) {
    return (
      <SettingsSection title={t(($) => $.workspace.usage.title)}>
        <SettingsCard>
          <div
            className="space-y-3 p-4 motion-reduce:[&_[data-slot=skeleton]]:animate-none"
            role="status"
            aria-label={t(($) => $.workspace.usage.loading)}
          >
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-2/3" />
          </div>
        </SettingsCard>
      </SettingsSection>
    );
  }

  if (error) {
    return (
      <SettingsSection title={t(($) => $.workspace.usage.title)}>
        <Alert variant="destructive">
          <AlertTriangle />
          <AlertTitle>{t(($) => $.workspace.usage.unavailable)}</AlertTitle>
          {onRetry ? (
            <AlertDescription>
              <Button
                className="mt-2"
                type="button"
                variant="outline"
                size="sm"
                onClick={onRetry}
              >
                <RefreshCw />
                {t(($) => $.workspace.actions.retry)}
              </Button>
            </AlertDescription>
          ) : null}
        </Alert>
      </SettingsSection>
    );
  }

  if (!usage || usage.meters.length === 0) {
    return (
      <SettingsSection title={t(($) => $.workspace.usage.title)}>
        <SettingsCard>
          <div className="px-4 py-4 text-body text-muted-foreground">
            {t(($) => $.workspace.usage.empty)}
          </div>
        </SettingsCard>
      </SettingsSection>
    );
  }

  const periodStart = formatUsageUtcDate(usage.periodStart, locale);
  const periodEnd = formatUsageUtcDate(usage.periodEnd, locale);
  const periodRange =
    periodStart && periodEnd
      ? t(($) => $.workspace.usage.period, {
          start: periodStart,
          end: periodEnd,
        })
      : null;

  const hasCreditLimit = usage.creditLimit > 0;
  const isAtWarningThreshold =
    hasCreditLimit && usage.accruedAmount * 100 >= usage.creditLimit * 80;
  const rawPercent = hasCreditLimit
    ? Math.round((usage.accruedAmount / usage.creditLimit) * 100)
    : 0;
  const progressValue = hasCreditLimit
    ? Math.min(100, Math.max(0, (usage.accruedAmount / usage.creditLimit) * 100))
    : 0;

  return (
    <SettingsSection title={t(($) => $.workspace.usage.title)}>
      <SettingsCard>
        {usage.meters.map((meter) => {
          const quantityText = formatUsageQuantity(
            meter.key,
            meter.quantity,
            locale,
          );
          const rateText =
            meter.unitPrice > 0
              ? t(($) => $.workspace.usage.rate, {
                  price: formatUsageMoney(
                    meter.unitPrice,
                    usage.currency,
                    locale,
                  ),
                  per: formatUsageUnitPer(meter.key, meter.per, locale),
                })
              : null;

          return (
            <SettingsRow
              key={meter.key}
              label={
                <div className="flex flex-wrap items-center gap-2">
                  <span>{resolveMeterLabel(meter.key, t)}</span>
                  {meter.discountPct > 0 ? (
                    <Badge variant="secondary" className="tabular-nums">
                      {t(($) => $.workspace.usage.discount_badge, {
                        pct: meter.discountPct,
                      })}
                    </Badge>
                  ) : null}
                </div>
              }
              description={
                <span className="tabular-nums">
                  {rateText ? `${quantityText} · ${rateText}` : quantityText}
                </span>
              }
            >
              <span className="font-medium tabular-nums">
                {formatUsageMoney(meter.amount, usage.currency, locale)}
              </span>
            </SettingsRow>
          );
        })}

        <SettingsRow
          label={t(($) => $.workspace.usage.accrued)}
          description={
            periodRange ? (
              <span className="tabular-nums">{periodRange}</span>
            ) : undefined
          }
        >
          <span className="font-semibold tabular-nums">
            {formatUsageMoney(usage.accruedAmount, usage.currency, locale)}
          </span>
        </SettingsRow>

        {hasCreditLimit ? (
          <div className="space-y-3 px-4 py-3.5">
            <Progress
              value={progressValue}
              aria-label={t(($) => $.workspace.usage.credit_progress_label)}
            >
              <ProgressLabel>
                {t(($) => $.workspace.usage.credit_limit)}
              </ProgressLabel>
              <ProgressValue>
                {() =>
                  t(($) => $.workspace.usage.credit_progress, {
                    accrued: formatUsageMoney(
                      usage.accruedAmount,
                      usage.currency,
                      locale,
                    ),
                    limit: formatUsageMoney(
                      usage.creditLimit,
                      usage.currency,
                      locale,
                    ),
                  })
                }
              </ProgressValue>
            </Progress>

            {isAtWarningThreshold ? (
              <Alert className="border-warning/50 bg-warning/10">
                <AlertTriangle className="text-warning" />
                <AlertTitle>
                  {t(($) => $.workspace.usage.warning_title)}
                </AlertTitle>
                <AlertDescription>
                  {t(($) => $.workspace.usage.warning_body, {
                    percent: rawPercent,
                  })}
                </AlertDescription>
              </Alert>
            ) : null}
          </div>
        ) : null}
      </SettingsCard>
    </SettingsSection>
  );
}
