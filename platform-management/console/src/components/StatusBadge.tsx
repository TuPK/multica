import { Badge } from "@multica/ui/components/ui/badge";

export function StatusBadge({ status }: { status: string }) {
  switch (status) {
    case "active":
      return (
        <Badge
          variant="outline"
          className="border-emerald-600/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-400"
        >
          Active
        </Badge>
      );
    case "read_only":
      return <Badge variant="secondary">Read-only</Badge>;
    case "suspended":
      return <Badge variant="destructive">Suspended</Badge>;
    case "pending_deletion":
      return (
        <Badge
          variant="destructive"
          className="border-destructive/40 bg-destructive/15"
        >
          Pending Deletion
        </Badge>
      );
    case "deleted":
      return (
        <Badge variant="outline" className="text-muted-foreground">
          Deleted
        </Badge>
      );
    case "admin":
      return <Badge variant="default">Admin</Badge>;
    case "support":
      return <Badge variant="secondary">Support</Badge>;
    case "billing":
      return <Badge variant="outline">Billing</Badge>;
    default:
      return <Badge variant="outline">{status}</Badge>;
  }
}
