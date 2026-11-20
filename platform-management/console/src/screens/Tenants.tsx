import { useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@astryxdesign/core/Button";
import { TextInput } from "@astryxdesign/core/TextInput";
import { Building2, Search } from "@multica/ui/icons";
import { PageHeader } from "../components/PageHeader";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { TenantAccountMenu } from "./TenantAccountMenu";
import { useMe, useTenants } from "../api/hooks";
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
  const { data: me } = useMe();
  const canManageTenantStatus = me?.role === "system_admin";
  const canDeleteTenant = me?.role === "system_admin";
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
        <div className="w-full max-w-sm">
        <TextInput
          type="search"
          label={t("Search tenants")}
          isLabelHidden
          width="100%"
          startIcon={<Search className="size-4" aria-hidden="true" />}
          hasClear
          value={search}
          onChange={(value) => setSearch(value)}
          placeholder={t("Search by tenant, workspace or owner email...")}
        />
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
              <Button label={t("Clear search")} variant="secondary" size="sm" onClick={() => setSearch("")} />
            ) : undefined
          }
        />
      )}

      {!isLoading && !isError && tenants.length > 0 && (
        <div className="space-y-4">
          <Card padding={0} className="overflow-x-auto">
            <Table density="balanced" dividers="rows" hasHover>
              <TableHeader>
                <TableRow isHeaderRow>
                  <TableHeaderCell>{t("Name")}</TableHeaderCell>
                  <TableHeaderCell className="text-right">{t("Workspaces")}</TableHeaderCell>
                  <TableHeaderCell className="text-right">{t("Max workspaces")}</TableHeaderCell>
                  <TableHeaderCell>{t("Best plan")}</TableHeaderCell>
                  <TableHeaderCell>{t("Workspace status")}</TableHeaderCell>
                  <TableHeaderCell>{t("Last Activity")}</TableHeaderCell>
                  <TableHeaderCell className="text-right">{t("Action")}</TableHeaderCell>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tenants.map((tenant: Tenant) => (
                  <TableRow key={tenant.id} onClick={() => onSelectTenant(tenant.id)}>
                    <TableCell>
                      <div className="max-w-60 truncate font-medium text-foreground" title={tenant.name}>
                        {tenant.name}
                      </div>
                      <div
                        className="max-w-44 truncate font-mono text-caption text-muted-foreground"
                        title={tenant.id}
                      >
                        {tenant.id}
                      </div>
                    </TableCell>
                    <TableCell className="text-right">
                      <div data-testid="tenant-workspace-count">
                        <Text type="body" hasTabularNumbers>
                          {tenant.workspace_count}
                        </Text>
                      </div>
                    </TableCell>
                    <TableCell className="text-right">
                      <div data-testid="tenant-max-workspaces">
                        <Text type="body" hasTabularNumbers>
                          {maxWorkspacesLabel(tenant.max_workspaces)}
                        </Text>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting">{tenant.best_plan_key || "-"}</Text>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting">{statusSummaryText(tenant.status_summary)}</Text>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting" hasTabularNumbers>
                        {tenant.last_activity_at ? new Date(tenant.last_activity_at).toLocaleString() : "-"}
                      </Text>
                    </TableCell>
                    <TableCell className="text-right" onClick={(event) => event.stopPropagation()}>
                      <TenantAccountMenu
                        tenantId={tenant.id}
                        tenantName={tenant.name || tenant.id}
                        tenantStatus={tenant.status}
                        canManageTenantStatus={canManageTenantStatus}
                        canDeleteTenant={canDeleteTenant}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Card>

          {hasNextPage && (
            <div className="flex justify-center pt-2">
              <Button
                label={t("Load more")}
                variant="secondary"
                onClick={() => fetchNextPage()}
                isDisabled={isFetchingNextPage}
                isLoading={isFetchingNextPage}
                aria-busy={isFetchingNextPage}
              />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
