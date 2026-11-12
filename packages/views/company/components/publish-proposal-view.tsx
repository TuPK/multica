"use client";

import { useMemo } from "react";
import { publishPayload, type Proposal } from "@multica/core/company";
import { Badge } from "@multica/ui/components/ui/badge";
import {
  Calendar,
  Clock,
  ExternalLink,
  Globe,
  Megaphone,
  Tag,
} from "@multica/ui/icons";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { CompanyProposalDecision } from "./company-proposal-decision";
import { CompanyProposalStatus } from "./company-proposal-details";

export function formatSchedule(scheduledAt: string | null, immediateLabel: string): string {
  if (!scheduledAt) return immediateLabel;
  const date = new Date(scheduledAt);
  if (Number.isNaN(date.getTime())) return scheduledAt;
  return date.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * Summary of a publish proposal payload: item ID, version, market, schedule, and per-channel content.
 */
export function PublishProposalSummary({
  proposal,
  className,
}: {
  proposal: Proposal;
  className?: string;
}) {
  const { t } = useT("issues");
  const payload = useMemo(() => publishPayload(proposal), [proposal]);

  if (!payload) return null;

  const marketLabel = payload.market === "vn" ? "Vietnam (vn)" : "International (intl)";
  const scheduleText = formatSchedule(
    payload.scheduled_at,
    t(($) => $.company.decision_publish_immediate),
  );

  return (
    <div className={cn("space-y-4", className)}>
      <div className="flex flex-wrap items-center gap-2">
        {payload.item_id ? (
          <Badge variant="outline" className="text-micro font-mono">
            {t(($) => $.company.decision_publish_item, { item: payload.item_id })}
          </Badge>
        ) : null}
        <Badge variant="outline" className="text-micro">
          {t(($) => $.company.decision_publish_version, { version: payload.version })}
        </Badge>
        <Badge variant="outline" className="text-micro">
          <Globe className="mr-1 size-3" />
          {t(($) => $.company.decision_publish_market, { market: marketLabel })}
        </Badge>
        <Badge variant="outline" className="text-micro">
          {payload.scheduled_at ? (
            <Calendar className="mr-1 size-3" />
          ) : (
            <Clock className="mr-1 size-3" />
          )}
          {t(($) => $.company.decision_publish_schedule, { schedule: scheduleText })}
        </Badge>
      </div>

      <div className="space-y-3">
        <h4 className="flex items-center gap-1.5 text-caption font-medium text-muted-foreground">
          <Megaphone className="size-3.5" />
          {t(($) => $.company.decision_publish_channels, {
            count: payload.channels.length,
          })}
        </h4>

        {payload.channels.length === 0 ? (
          <p className="text-caption text-muted-foreground">
            {t(($) => $.company.decision_publish_no_channels)}
          </p>
        ) : (
          <ul className="space-y-3">
            {payload.channels.map((channel, idx) => (
              <li
                key={channel.channel_id || `channel-${idx}`}
                data-testid={`publish-channel-card-${channel.channel_id || idx}`}
                className="rounded-lg border border-surface-border bg-surface p-3.5 space-y-2.5"
              >
                <div className="flex flex-wrap items-center justify-between gap-2 border-b border-surface-border/60 pb-2">
                  <div className="flex items-center gap-2">
                    <span className="font-semibold text-body uppercase tracking-wide">
                      {channel.platform}
                    </span>
                    <span className="text-body text-muted-foreground">
                      {channel.account_name}
                    </span>
                  </div>
                  {channel.media_url ? (
                    <a
                      href={channel.media_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 text-caption text-primary hover:underline"
                    >
                      <span>{t(($) => $.company.decision_publish_media)}</span>
                      <ExternalLink className="size-3" />
                    </a>
                  ) : null}
                </div>

                <p className="whitespace-pre-wrap text-body text-foreground leading-relaxed">
                  {channel.text}
                </p>

                {channel.hashtags.length > 0 ? (
                  <div className="flex flex-wrap items-center gap-1.5 pt-1">
                    {channel.hashtags.map((tag) => (
                      <span
                        key={tag}
                        className="inline-flex items-center gap-0.5 rounded bg-muted px-1.5 py-0.5 text-micro text-muted-foreground"
                      >
                        <Tag className="size-2.5" />
                        {tag.startsWith("#") ? tag : `#${tag}`}
                      </span>
                    ))}
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

/**
 * Full publish proposal view including item, channels, market, schedule, and decision controls.
 */
export function PublishProposalView({
  proposal,
  canDecide = true,
  squadName = null,
  showDecisions = true,
  className,
}: {
  proposal: Proposal;
  canDecide?: boolean;
  squadName?: string | null;
  showDecisions?: boolean;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col gap-4", className)}>
      <CompanyProposalStatus proposal={proposal} squadName={squadName} />
      <PublishProposalSummary proposal={proposal} />
      {showDecisions ? (
        <CompanyProposalDecision proposal={proposal} canDecide={canDecide} />
      ) : null}
    </div>
  );
}
