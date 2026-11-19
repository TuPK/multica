import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
} from "@multica/ui/components/ui/card";
import { Separator } from "@multica/ui/components/ui/separator";
import {
  Building2,
  Users,
  Shield,
  Activity,
  AlertTriangle,
  CreditCard,
} from "@multica/ui/icons";
import { PageHeader } from "../components/PageHeader";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { useOverview } from "../api/hooks";
import { formatMinorAmount } from "../money";
import { formatBytes } from "./Tenants";

export function Overview() {
  const { data, isLoading, isError, error, refetch } = useOverview();

  if (isLoading) {
    return <LoadingState message="Loading overview..." />;
  }

  if (isError) {
    return (
      <ErrorState
        title="Failed to load overview"
        message={error instanceof Error ? error.message : undefined}
        onRetry={refetch}
      />
    );
  }

  if (!data) {
    return (
      <EmptyState
        title="No overview data"
        description="Unable to load workspace metrics."
      />
    );
  }

  const byStatus = data.workspaces_by_status || {};
  const activeCount = byStatus["active"] ?? 0;
  const readOnlyCount = byStatus["read_only"] ?? 0;
  const suspendedCount = byStatus["suspended"] ?? 0;
  const pendingDeletionCount = byStatus["pending_deletion"] ?? 0;
  const totalWorkspaces =
    activeCount + readOnlyCount + suspendedCount + pendingDeletionCount;

  const planEntries = data.by_plan ? Object.entries(data.by_plan) : [];
  const pendingCurrencyEntries = data.pending_by_currency
    ? Object.entries(data.pending_by_currency)
    : [];
  const hasUsageSnapshot =
    data.usage_day != null ||
    data.usage_workspaces !== undefined ||
    data.usage_members !== undefined ||
    data.usage_agents !== undefined ||
    data.usage_runtimes !== undefined ||
    data.usage_issues_total !== undefined ||
    data.usage_issues_created !== undefined ||
    data.usage_tasks_run !== undefined ||
    data.usage_storage_bytes !== undefined;

  return (
    <div className="space-y-6">
      <PageHeader title="Platform Overview" />

      <section aria-label="Workspace metrics" className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card className="border-surface-border shadow-none">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-body font-medium text-muted-foreground">
              Total Workspaces
            </CardTitle>
            <Building2 className="size-4 text-muted-foreground" aria-hidden="true" />
          </CardHeader>
          <CardContent>
            <div className="font-heading text-display-sm font-semibold tabular-nums">
              {totalWorkspaces}
            </div>
          </CardContent>
        </Card>

        <Card className="border-surface-border shadow-none">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-body font-medium text-muted-foreground">
              Active Workspaces
            </CardTitle>
            <Activity className="size-4 text-success" aria-hidden="true" />
          </CardHeader>
          <CardContent>
            <div className="font-heading text-display-sm font-semibold tabular-nums text-success">
              {activeCount}
            </div>
          </CardContent>
        </Card>

        <Card className="border-surface-border shadow-none">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-body font-medium text-muted-foreground">
              Sign-ups (7 Days)
            </CardTitle>
            <Users className="size-4 text-primary" aria-hidden="true" />
          </CardHeader>
          <CardContent>
            <div className="font-heading text-display-sm font-semibold tabular-nums">
              {data.signups_7d}
            </div>
          </CardContent>
        </Card>

        {data.pending_orders !== undefined && (
          <Card className="border-surface-border shadow-none">
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-body font-medium text-muted-foreground">
                Pending Orders
              </CardTitle>
              <CreditCard className="size-4 text-warning" aria-hidden="true" />
            </CardHeader>
            <CardContent className="space-y-1">
              <div className="font-heading text-display-sm font-semibold tabular-nums">
                {data.pending_orders}
              </div>
              {pendingCurrencyEntries.length > 0 && (
                <div className="space-y-0.5 text-caption text-muted-foreground tabular-nums">
                  {pendingCurrencyEntries.map(([currency, summary]) => (
                    <div key={currency}>
                      {summary.orders} ·{" "}
                      {formatMinorAmount(
                        summary.amount + summary.vat_amount,
                        currency,
                      )}
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        )}

        <Card className="border-surface-border shadow-none">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-body font-medium text-muted-foreground">
              Read-Only
            </CardTitle>
            <Shield className="size-4 text-muted-foreground" aria-hidden="true" />
          </CardHeader>
          <CardContent>
            <div className="font-heading text-display-sm font-semibold tabular-nums text-muted-foreground">
              {readOnlyCount}
            </div>
          </CardContent>
        </Card>

        <Card className="border-surface-border shadow-none">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-body font-medium text-muted-foreground">
              Suspended
            </CardTitle>
            <AlertTriangle className="size-4 text-destructive" aria-hidden="true" />
          </CardHeader>
          <CardContent>
            <div className="font-heading text-display-sm font-semibold tabular-nums text-destructive">
              {suspendedCount}
            </div>
          </CardContent>
        </Card>

        <Card className="border-surface-border shadow-none">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-body font-medium text-muted-foreground">
              Pending Deletion
            </CardTitle>
            <AlertTriangle className="size-4 text-destructive" aria-hidden="true" />
          </CardHeader>
          <CardContent>
            <div className="font-heading text-display-sm font-semibold tabular-nums text-destructive">
              {pendingDeletionCount}
            </div>
          </CardContent>
        </Card>
      </section>

      {planEntries.length > 0 && (
        <>
          <Separator />
          <section className="space-y-4">
            <h2 className="font-heading text-title-sm font-semibold text-foreground">
              Subscriptions by Plan
            </h2>
            <MetadataList columns={4}>
              {planEntries.map(([planKey, planCount]) => (
                <MetadataListItem key={planKey} label={planKey} tabular>
                  {planCount}
                </MetadataListItem>
              ))}
            </MetadataList>
          </section>
        </>
      )}

      {hasUsageSnapshot && (
        <>
          <Separator />
          <section className="space-y-4">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <h2 className="font-heading text-title-sm font-semibold text-foreground">
                Platform Usage Snapshot
              </h2>
              {data.usage_day && (
                <span className="font-mono text-caption text-muted-foreground">
                  {data.usage_day}
                </span>
              )}
            </div>
            <MetadataList columns={4}>
              <MetadataListItem label="Usage Workspaces" tabular>
                {data.usage_workspaces ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Members" tabular>
                {data.usage_members ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Agents" tabular>
                {data.usage_agents ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Runtimes" tabular>
                {data.usage_runtimes ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Total Issues" tabular>
                {data.usage_issues_total ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Issues Created" tabular>
                {data.usage_issues_created ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Tasks Run" tabular>
                {data.usage_tasks_run ?? 0}
              </MetadataListItem>
              <MetadataListItem label="Storage" tabular>
                {formatBytes(data.usage_storage_bytes ?? 0)}
              </MetadataListItem>
            </MetadataList>
          </section>
        </>
      )}
    </div>
  );
}
