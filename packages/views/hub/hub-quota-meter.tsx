"use client";

import { useQuery } from "@tanstack/react-query";
import { hubQuotaOptions, type HubQuotaUsage } from "@multica/core/hub";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../i18n";
import { formatHubSize } from "./hub-files";
import { hubQuotaLevel, type HubQuotaLevel } from "./hub-quota-state";

/**
 * Item count and storage against the workspace limits. Renders nothing when the
 * quota cannot be read, so the Hub still works without it.
 */
export function HubQuotaMeter({ wsId }: { wsId: string }) {
  const { t } = useT("hub");
  const quota = useQuery({ ...hubQuotaOptions(wsId), retry: false });
  if (!quota.data) return null;

  return (
    <div
      role="group"
      aria-label={t(($) => $.quota.label)}
      className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-caption"
    >
      <QuotaUsage
        label={t(($) => $.quota.items)}
        usage={quota.data.items}
        format={formatCount}
      />
      <QuotaUsage
        label={t(($) => $.quota.storage)}
        usage={quota.data.storage_bytes}
        format={formatHubSize}
      />
    </div>
  );
}

function formatCount(value: number): string {
  return new Intl.NumberFormat().format(value);
}

function QuotaUsage({
  label,
  usage,
  format,
}: {
  label: string;
  usage: HubQuotaUsage;
  format: (value: number) => string;
}) {
  const { t } = useT("hub");
  const level = hubQuotaLevel(usage.used, usage.limit);
  const used = format(usage.used);

  if (level === "unlimited") {
    return (
      <span className="flex min-w-0 items-center gap-1.5 text-muted-foreground">
        <span>{label}</span>
        <span className="text-foreground">{t(($) => $.quota.usage_unlimited, { used })}</span>
      </span>
    );
  }

  const text = t(($) => $.quota.usage, { used, limit: format(usage.limit) });
  const ratio = Math.min(1, usage.used / usage.limit);

  return (
    <div className="flex min-w-0 items-center gap-2">
      <span className="text-muted-foreground">{label}</span>
      <div
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={usage.limit}
        aria-valuenow={usage.used}
        aria-valuetext={text}
        className="h-1.5 w-20 shrink-0 overflow-hidden rounded-full bg-muted"
      >
        <div className={cn("h-full rounded-full transition-all duration-300 ease-out", barClass(level))} style={{ width: `${ratio * 100}%` }} />
      </div>
      <span className={cn("truncate", textClass(level))}>{text}</span>
      {level === "warning" ? (
        <span className="shrink-0 rounded-xs bg-warning/15 px-1.5 py-0.5 text-micro font-medium text-warning">
          {t(($) => $.quota.warning)}
        </span>
      ) : null}
      {level === "full" ? (
        <span className="shrink-0 rounded-xs bg-destructive/15 px-1.5 py-0.5 text-micro font-medium text-destructive">
          {t(($) => $.quota.full)}
        </span>
      ) : null}
    </div>
  );
}

function barClass(level: HubQuotaLevel): string {
  if (level === "full") return "bg-destructive";
  if (level === "warning") return "bg-warning";
  return "bg-brand";
}

function textClass(level: HubQuotaLevel): string {
  if (level === "full") return "text-destructive";
  if (level === "warning") return "text-warning";
  return "text-muted-foreground";
}
