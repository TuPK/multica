import { useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import { pixel, proportional, Table } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Building2, Search, X } from "@multica/ui/icons";
import { PageHeader } from "../components/PageHeader";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { RowActions, useRowActionsWidth } from "../components/RowActions";
import { useTenants } from "../api/hooks";
import type { Tenant } from "../api/schemas";
import { t } from "../i18n";

const BYTE_UNITS = ["B", "KB", "MB", "GB", "TB"];

/** Formats a byte count with one decimal for units above bytes: 2048 -> "2.0 KB". */
export function formatBytes(bytes: number): string {
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < BYTE_UNITS.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return unit === 0 ? `${value} B` : `${value.toFixed(1)} ${BYTE_UNITS[unit]}`;
}

/** The limit a tenant's plans allow: a number, or "Unlimited" when there is none. */
export function maxWorkspacesLabel(max: number | null): string {
  return max === null ? t("Unlimited") : String(max);
}

/** "2 active, 1 suspended": the tenant's workspaces by status, largest group first. */
export function statusSummaryText(summary: Record<string, number>): string {
  const parts = Object.entries(summary)
    .filter(([, n]) => n > 0)
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([status, n]) => `${n} ${status.replace(/_/g, " ")}`);
  return parts.length ? parts.join(", ") : "-";
}

interface TenantsProps {
  onSelectTenant: (id: string) => void;
}

export function Tenants({ onSelectTenant }: TenantsProps) {
  const actionsWidth = useRowActionsWidth(96);
  const [search, setSearch] = useState("");
  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useTenants({
    q: search || undefined,
  });

  const tenants: Tenant[] = Array.isArray(data)
    ? data
    : (data as unknown as { tenants?: Tenant[] })?.tenants || [];

  return (
    <div className="space-y-6">
      <PageHeader title={t("Tenants")} />

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative w-full max-w-sm">
          <Search
            className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            aria-label={t("Search tenants")}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("Search by tenant, workspace or owner email...")}
            className="pr-8 pl-9"
          />
          {search && (
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label={t("Clear search")}
              onClick={() => setSearch("")}
              className="absolute top-1 right-1 text-muted-foreground hover:text-foreground"
            >
              <X className="size-3.5" aria-hidden="true" />
            </Button>
          )}
        </div>
      </div>

      {isLoading && <LoadingState message={t("Loading tenants...")} />}

      {isError && (
        <ErrorState
          title={t("Failed to load tenants")}
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && tenants.length === 0 && (
        <EmptyState
          icon={<Building2 className="size-5" aria-hidden="true" />}
          title={t("No tenants found")}
          description={
            search
              ? t("No tenants matched your search query.")
              : t("There are currently no registered tenants.")
          }
          actions={
            search ? (
              <Button variant="outline" size="sm" onClick={() => setSearch("")}>
                {t("Clear search")}
              </Button>
            ) : undefined
          }
        />
      )}

      {!isLoading && !isError && tenants.length > 0 && (
        <div className="space-y-4">
          <Card padding={0} className="overflow-x-auto">
            <Table
              data={tenants}
              idKey="id"
              density="balanced"
              dividers="rows"
              hasHover
              columns={[
                {
                  key: "name",
                  header: t("Name"),
                  width: proportional(2, { minWidth: 200 }),
                  renderCell: (tenant) => (
                    <>
                      <div className="truncate font-medium text-foreground" title={tenant.name}>
                        {tenant.name}
                      </div>
                      <div className="truncate font-mono text-caption text-muted-foreground" title={tenant.id}>
                        {tenant.id}
                      </div>
                    </>
                  ),
                },
                {
                  key: "workspace_count",
                  header: t("Workspaces"),
                  width: pixel(120),
                  align: "end",
                  renderCell: (tenant) => (
                    <div data-testid="tenant-workspace-count">
                      <Text type="body" hasTabularNumbers>
                        {tenant.workspace_count}
                      </Text>
                    </div>
                  ),
                },
                {
                  key: "max_workspaces",
                  header: t("Max workspaces"),
                  width: proportional(1, { minWidth: 120 }),
                  align: "end",
                  renderCell: (tenant) => (
                    <div data-testid="tenant-max-workspaces">
                      <Text type="body" hasTabularNumbers>
                        {maxWorkspacesLabel(tenant.max_workspaces)}
                      </Text>
                    </div>
                  ),
                },
                {
                  key: "best_plan_key",
                  header: t("Best plan"),
                  width: proportional(1, { minWidth: 110 }),
                  renderCell: (tenant) => <Text type="supporting">{tenant.best_plan_key || "-"}</Text>,
                },
                {
                  key: "status_summary",
                  header: t("Workspace status"),
                  width: proportional(1.5, { minWidth: 160 }),
                  renderCell: (tenant) => <Text type="supporting">{statusSummaryText(tenant.status_summary)}</Text>,
                },
                {
                  key: "last_activity_at",
                  header: t("Last Activity"),
                  width: proportional(1.3, { minWidth: 150 }),
                  renderCell: (tenant) => (
                    <Text type="supporting" hasTabularNumbers>
                      {tenant.last_activity_at ? new Date(tenant.last_activity_at).toLocaleString() : "-"}
                    </Text>
                  ),
                },
                {
                  key: "actions",
                  header: t("Action"),
                  width: pixel(actionsWidth),
                  align: "end",
                  renderCell: (tenant) => (
                    <RowActions
                      label={t("tenant {0}", tenant.name)}
                      actions={[{ label: t("View"), onSelect: () => onSelectTenant(tenant.id) }]}
                    />
                  ),
                },
              ]}
            />
          </Card>

          {hasNextPage && (
            <div className="flex justify-center pt-2">
              <Button
                variant="outline"
                onClick={() => fetchNextPage()}
                disabled={isFetchingNextPage}
                aria-busy={isFetchingNextPage}
              >
                {isFetchingNextPage && <Spinner className="size-3.5" />}
                {t("Load more")}
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
