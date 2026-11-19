import { Card } from "@astryxdesign/core/Card";
import { pixel, proportional, Table } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@multica/ui/components/ui/button";
import { Building2 } from "@multica/ui/icons";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { useTenant } from "../api/hooks";
import type { TenantWorkspace } from "../api/schemas";
import { formatBytes, maxWorkspacesLabel, statusSummaryText } from "./Tenants";

interface TenantDetailProps {
  tenantId: string;
  onBack: () => void;
  onSelectWorkspace: (id: string) => void;
}

/**
 * The usage cell text of a workspace. A workspace with no daily row has a null
 * (or missing) usage day and shows "-", not zeros.
 */
function usageSummary(workspace: TenantWorkspace): string {
  if (!workspace.usage_day) return "-";
  return [
    `${workspace.usage_members ?? 0} members`,
    `${workspace.usage_agents ?? 0} agents`,
    `${workspace.usage_tasks_run ?? 0} tasks run`,
    formatBytes(workspace.usage_storage_bytes ?? 0),
  ].join(" · ");
}

export function TenantDetail({ tenantId, onBack, onSelectWorkspace }: TenantDetailProps) {
  const { data: tenant, isLoading, isError, error, refetch } = useTenant(tenantId);
  const back = { label: "Back to tenants", onClick: onBack };

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Tenant" backAction={back} />
        <LoadingState message="Loading tenant..." />
      </div>
    );
  }

  if (isError || !tenant) {
    return (
      <div className="space-y-6">
        <PageHeader title="Tenant" backAction={back} />
        <ErrorState
          title="Failed to load tenant"
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={tenant.name || tenant.id}
        backAction={back}
        description={<span className="font-mono text-caption">{tenant.id}</span>}
      />

      <Card padding={4}>
        <MetadataList columns={3}>
          <MetadataListItem label="Workspaces" tabular>
            <span data-testid="tenant-workspace-count">{tenant.workspace_count}</span>
          </MetadataListItem>
          <MetadataListItem label="Max workspaces" tabular>
            <span data-testid="tenant-max-workspaces">{maxWorkspacesLabel(tenant.max_workspaces)}</span>
          </MetadataListItem>
          <MetadataListItem label="Best plan">{tenant.best_plan_key || "-"}</MetadataListItem>
          <MetadataListItem label="Workspace status">
            <span data-testid="tenant-status-summary">{statusSummaryText(tenant.status_summary)}</span>
          </MetadataListItem>
          <MetadataListItem label="Last activity" tabular>
            {tenant.last_activity_at ? new Date(tenant.last_activity_at).toLocaleString() : "-"}
          </MetadataListItem>
        </MetadataList>
      </Card>

      {tenant.workspaces.length === 0 ? (
        <EmptyState
          icon={<Building2 className="size-5" aria-hidden="true" />}
          title="This tenant has no workspaces"
        />
      ) : (
        <Card padding={0} className="overflow-x-auto">
          <Table
            data={tenant.workspaces}
            idKey="id"
            density="balanced"
            dividers="rows"
            hasHover
            columns={[
              {
                key: "name",
                header: "Workspace",
                width: proportional(2, { minWidth: 200 }),
                renderCell: (workspace) => (
                  <>
                    <div className="truncate font-medium text-foreground" title={workspace.name}>
                      {workspace.name}
                    </div>
                    <div className="truncate font-mono text-caption text-muted-foreground" title={workspace.slug}>
                      {workspace.slug}
                    </div>
                  </>
                ),
              },
              {
                key: "plan_key",
                header: "Plan",
                width: proportional(1, { minWidth: 100 }),
                renderCell: (workspace) => <Text type="supporting">{workspace.plan_key || "-"}</Text>,
              },
              {
                key: "status",
                header: "Status",
                width: proportional(1, { minWidth: 110 }),
                renderCell: (workspace) => <StatusBadge status={workspace.status} />,
              },
              {
                key: "member_count",
                header: "Members",
                width: pixel(96),
                align: "end",
                renderCell: (workspace) => (
                  <Text type="body" hasTabularNumbers>
                    {workspace.member_count}
                  </Text>
                ),
              },
              {
                key: "owner",
                header: "Owner",
                width: proportional(1.5, { minWidth: 160 }),
                renderCell: (workspace) => (
                  <Text type="supporting">{workspace.owner?.email ?? "-"}</Text>
                ),
              },
              {
                key: "usage",
                header: "Usage",
                width: proportional(1.5, { minWidth: 160 }),
                renderCell: (workspace) => (
                  <span
                    data-testid="workspace-usage"
                    title={workspace.usage_day ? `Usage on ${workspace.usage_day} (UTC)` : "No usage recorded yet"}
                  >
                    <Text type="supporting" hasTabularNumbers>
                      {usageSummary(workspace)}
                    </Text>
                  </span>
                ),
              },
              {
                key: "actions",
                header: "Action",
                width: pixel(96),
                align: "end",
                renderCell: (workspace) => (
                  <Button variant="outline" size="sm" onClick={() => onSelectWorkspace(workspace.id)}>
                    View
                  </Button>
                ),
              },
            ]}
          />
        </Card>
      )}
    </div>
  );
}
