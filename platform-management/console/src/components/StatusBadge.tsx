import { Badge, type BadgeVariant } from "@astryxdesign/core/Badge";
import { StatusDot as AstryxStatusDot, type StatusDotVariant } from "@astryxdesign/core/StatusDot";
import { Text } from "@astryxdesign/core/Text";
import { t } from "../i18n";

type StatusPresentation =
  | {
      kind: "quiet";
      label: string;
      dotVariant: StatusDotVariant;
      textColor: "primary" | "secondary";
    }
  | {
      kind: "badge";
      label: string;
      badgeVariant: BadgeVariant;
      dotVariant?: StatusDotVariant;
    };

/**
 * Astryx Neutral status-dot & badge discipline (Badge.doc.mjs / StatusDot.doc.mjs):
 * - Routine/healthy and inactive states render as a quiet Astryx StatusDot + Text
 *   so they never drown out rows that demand operator attention.
 * - Exception, warning, and active transitional states render as Astryx semantic
 *   Badges with a leading StatusDot.
 * - Roles render as Astryx categorical Badges without a StatusDot.
 */
const STATUS_CONFIG: Record<string, StatusPresentation> = {
  // Routine healthy operational states — quiet dot + primary text
  active: { kind: "quiet", label: "Active", dotVariant: "success", textColor: "primary" },
  paid: { kind: "quiet", label: "Paid", dotVariant: "success", textColor: "primary" },
  completed: { kind: "quiet", label: "Completed", dotVariant: "success", textColor: "primary" },
  issued: { kind: "quiet", label: "Issued", dotVariant: "success", textColor: "primary" },
  applied: { kind: "quiet", label: "Applied", dotVariant: "success", textColor: "primary" },
  resolved: { kind: "quiet", label: "Resolved", dotVariant: "success", textColor: "primary" },
  verified: { kind: "quiet", label: "Verified", dotVariant: "success", textColor: "primary" },
  approved: { kind: "quiet", label: "Approved", dotVariant: "success", textColor: "primary" },

  // Inactive / neutral operational states — quiet dot + secondary text
  read_only: { kind: "quiet", label: "Read-only", dotVariant: "neutral", textColor: "secondary" },
  deleted: { kind: "quiet", label: "Deleted", dotVariant: "neutral", textColor: "secondary" },
  archived: { kind: "quiet", label: "Archived", dotVariant: "neutral", textColor: "secondary" },
  inactive: { kind: "quiet", label: "Inactive", dotVariant: "neutral", textColor: "secondary" },
  expired: { kind: "quiet", label: "Expired", dotVariant: "neutral", textColor: "secondary" },
  canceled: { kind: "quiet", label: "Canceled", dotVariant: "neutral", textColor: "secondary" },
  cancelled: { kind: "quiet", label: "Cancelled", dotVariant: "neutral", textColor: "secondary" },
  dismissed: { kind: "quiet", label: "Dismissed", dotVariant: "neutral", textColor: "secondary" },
  unverified: { kind: "quiet", label: "Unverified", dotVariant: "neutral", textColor: "secondary" },
  received: { kind: "quiet", label: "Received", dotVariant: "neutral", textColor: "secondary" },
  released: { kind: "quiet", label: "Released", dotVariant: "neutral", textColor: "secondary" },

  // Attention / warning states — semantic warning badge + dot
  pending: { kind: "badge", label: "Pending", badgeVariant: "warning", dotVariant: "warning" },
  awaiting_approval: {
    kind: "badge",
    label: "Awaiting approval",
    badgeVariant: "warning",
    dotVariant: "warning",
  },
  past_due: { kind: "badge", label: "Past due", badgeVariant: "warning", dotVariant: "warning" },
  needs_review: {
    kind: "badge",
    label: "Needs review",
    badgeVariant: "warning",
    dotVariant: "warning",
  },

  // Critical / destructive states — semantic error badge + dot
  suspended: { kind: "badge", label: "Suspended", badgeVariant: "error", dotVariant: "error" },
  pending_deletion: {
    kind: "badge",
    label: "Pending Deletion",
    badgeVariant: "error",
    dotVariant: "error",
  },
  legal_hold: { kind: "badge", label: "Legal hold", badgeVariant: "error", dotVariant: "error" },
  export_timeout: {
    kind: "badge",
    label: "Export timeout",
    badgeVariant: "error",
    dotVariant: "error",
  },
  failed: { kind: "badge", label: "Failed", badgeVariant: "error", dotVariant: "error" },
  denied: { kind: "badge", label: "Denied", badgeVariant: "error", dotVariant: "error" },
  needs_info: { kind: "badge", label: "Needs info", badgeVariant: "error", dotVariant: "error" },

  // Transitional / informational states — semantic info badge + dot
  running: { kind: "badge", label: "Running", badgeVariant: "info", dotVariant: "accent" },
  trialing: { kind: "badge", label: "Trialing", badgeVariant: "info", dotVariant: "accent" },
  provisioned: { kind: "badge", label: "Provisioned", badgeVariant: "info", dotVariant: "accent" },
  scheduled: { kind: "badge", label: "Scheduled", badgeVariant: "info", dotVariant: "accent" },
  refunded: { kind: "badge", label: "Refunded", badgeVariant: "info", dotVariant: "accent" },

  // Staff & workspace roles — categorical badges, no dot
  system_admin: { kind: "badge", label: "System Admin", badgeVariant: "purple" },
  admin: { kind: "badge", label: "Admin", badgeVariant: "blue" },
  support: { kind: "badge", label: "Support", badgeVariant: "neutral" },
  billing: { kind: "badge", label: "Billing", badgeVariant: "teal" },
  owner: { kind: "badge", label: "Owner", badgeVariant: "purple" },
  member: { kind: "badge", label: "Member", badgeVariant: "neutral" },
  viewer: { kind: "badge", label: "Viewer", badgeVariant: "neutral" },

  unknown: { kind: "badge", label: "Unknown", badgeVariant: "neutral" },
};

export function StatusBadge({ status, label }: { status: string; label?: string }) {
  const config = STATUS_CONFIG[status];
  if (!config) {
    return <Badge variant="neutral" label={label ?? status} />;
  }

  const text = label ?? t(config.label);

  if (config.kind === "quiet") {
    return (
      <span
        data-status-presentation="quiet"
        className="inline-flex items-center gap-1.5 whitespace-nowrap"
      >
        <AstryxStatusDot variant={config.dotVariant} label={text} aria-hidden="true" />
        <Text type="body" color={config.textColor}>
          {text}
        </Text>
      </span>
    );
  }

  return (
    <Badge
      variant={config.badgeVariant}
      label={text}
      icon={
        config.dotVariant ? (
          <AstryxStatusDot variant={config.dotVariant} label={text} aria-hidden="true" />
        ) : undefined
      }
    />
  );
}
