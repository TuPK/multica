import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
} from "@multica/ui/components/ui/card";
import {
  Building2,
  Users,
  Shield,
  Activity,
  AlertTriangle,
} from "@multica/ui/icons";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { useOverview } from "../api/hooks";

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

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-heading text-title-lg font-bold">Platform Overview</h1>
        <p className="text-body text-muted-foreground">
          System-wide workspaces status and activity metrics.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-body font-medium text-muted-foreground">
              Total Workspaces
            </CardTitle>
            <Building2 className="size-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="font-heading text-title-xl font-bold">
              {totalWorkspaces}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-body font-medium text-muted-foreground">
              Active Workspaces
            </CardTitle>
            <Activity className="size-4 text-emerald-600 dark:text-emerald-400" />
          </CardHeader>
          <CardContent>
            <div className="font-heading text-title-xl font-bold text-emerald-600 dark:text-emerald-400">
              {activeCount}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-body font-medium text-muted-foreground">
              Sign-ups (7 Days)
            </CardTitle>
            <Users className="size-4 text-primary" />
          </CardHeader>
          <CardContent>
            <div className="font-heading text-title-xl font-bold">
              {data.signups_7d}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-body font-medium text-muted-foreground">
              Read-Only
            </CardTitle>
            <Shield className="size-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="font-heading text-title-xl font-bold text-muted-foreground">
              {readOnlyCount}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-body font-medium text-muted-foreground">
              Suspended
            </CardTitle>
            <AlertTriangle className="size-4 text-destructive" />
          </CardHeader>
          <CardContent>
            <div className="font-heading text-title-xl font-bold text-destructive">
              {suspendedCount}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-body font-medium text-muted-foreground">
              Pending Deletion
            </CardTitle>
            <AlertTriangle className="size-4 text-destructive" />
          </CardHeader>
          <CardContent>
            <div className="font-heading text-title-xl font-bold text-destructive">
              {pendingDeletionCount}
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
