import { Badge } from "@multica/ui/components/ui/badge";
import { cn } from "@multica/ui/lib/utils";

type BadgeVariant = "default" | "secondary" | "destructive" | "outline";

interface StatusConfig {
  label: string;
  variant: BadgeVariant;
  className?: string;
  dot?: boolean;
}

const STATUS_CONFIG: Record<string, StatusConfig> = {
  // Operational workspace & account statuses
  active: {
    label: "Active",
    variant: "outline",
    className: "border-success/30 bg-success/10 text-success",
    dot: true,
  },
  read_only: {
    label: "Read-only",
    variant: "secondary",
    dot: true,
  },
  suspended: {
    label: "Suspended",
    variant: "destructive",
    dot: true,
  },
  pending_deletion: {
    label: "Pending Deletion",
    variant: "destructive",
    className: "border-destructive/40 bg-destructive/15",
    dot: true,
  },
  deleted: {
    label: "Deleted",
    variant: "outline",
    className: "text-muted-foreground",
    dot: true,
  },
  archived: {
    label: "Archived",
    variant: "secondary",
    className: "text-muted-foreground",
    dot: true,
  },

  // Subscription statuses
  trialing: {
    label: "Trialing",
    variant: "outline",
    className: "border-info/30 bg-info/10 text-info",
    dot: true,
  },
  past_due: {
    label: "Past due",
    variant: "outline",
    className: "border-warning/40 bg-warning/10 text-warning",
    dot: true,
  },

  // Order statuses
  pending: {
    label: "Pending",
    variant: "outline",
    className: "border-warning/40 bg-warning/10 text-warning",
    dot: true,
  },
  paid: {
    label: "Paid",
    variant: "outline",
    className: "border-success/30 bg-success/10 text-success",
    dot: true,
  },
  expired: {
    label: "Expired",
    variant: "secondary",
    className: "text-muted-foreground",
    dot: true,
  },
  canceled: {
    label: "Canceled",
    variant: "outline",
    className: "text-muted-foreground",
    dot: true,
  },
  cancelled: {
    label: "Cancelled",
    variant: "outline",
    className: "text-muted-foreground",
    dot: true,
  },
  refunded: {
    label: "Refunded",
    variant: "outline",
    className: "border-info/30 bg-info/10 text-info",
    dot: true,
  },

  // Account requests, payment notices, e-invoices, and price changes
  provisioned: {
    label: "Provisioned",
    variant: "outline",
    className: "border-info/30 bg-info/10 text-info",
    dot: true,
  },
  completed: {
    label: "Completed",
    variant: "outline",
    className: "border-success/30 bg-success/10 text-success",
    dot: true,
  },
  failed: {
    label: "Failed",
    variant: "destructive",
    dot: true,
  },
  needs_info: {
    label: "Needs info",
    variant: "destructive",
    dot: true,
  },
  issued: {
    label: "Issued",
    variant: "outline",
    className: "border-success/30 bg-success/10 text-success",
    dot: true,
  },
  received: {
    label: "Received",
    variant: "secondary",
    dot: true,
  },
  needs_review: {
    label: "Needs review",
    variant: "outline",
    className: "border-warning/40 bg-warning/10 text-warning",
    dot: true,
  },
  applied: {
    label: "Applied",
    variant: "outline",
    className: "border-success/30 bg-success/10 text-success",
    dot: true,
  },
  resolved: {
    label: "Resolved",
    variant: "outline",
    className: "border-success/30 bg-success/10 text-success",
    dot: true,
  },
  dismissed: {
    label: "Dismissed",
    variant: "outline",
    className: "text-muted-foreground",
    dot: true,
  },
  scheduled: {
    label: "Scheduled",
    variant: "outline",
    className: "border-info/30 bg-info/10 text-info",
    dot: true,
  },
  verified: {
    label: "Verified",
    variant: "outline",
    className: "border-success/30 bg-success/10 text-success",
    dot: true,
  },
  unverified: {
    label: "Unverified",
    variant: "outline",
    className: "text-muted-foreground",
    dot: true,
  },

  // Staff & workspace roles
  admin: {
    label: "Admin",
    variant: "default",
  },
  support: {
    label: "Support",
    variant: "secondary",
  },
  billing: {
    label: "Billing",
    variant: "outline",
  },
  owner: {
    label: "Owner",
    variant: "default",
  },
  member: {
    label: "Member",
    variant: "secondary",
  },

  unknown: {
    label: "Unknown",
    variant: "outline",
    className: "text-muted-foreground",
  },
};

export function StatusDot({ className }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn("size-1.5 shrink-0 rounded-full bg-current", className)}
    />
  );
}

export function StatusBadge({ status, label }: { status: string; label?: string }) {
  const config = STATUS_CONFIG[status];
  if (!config) {
    return <Badge variant="outline">{label ?? status}</Badge>;
  }

  return (
    <Badge variant={config.variant} className={config.className}>
      {config.dot && <StatusDot />}
      <span>{label ?? config.label}</span>
    </Badge>
  );
}
