import { useState } from "react";
import { Input } from "@multica/ui/components/ui/input";
import { Button } from "@multica/ui/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@multica/ui/components/ui/table";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Search, X } from "@multica/ui/icons";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { useTenants } from "../api/hooks";
import type { Tenant } from "../api/schemas";

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

/**
 * The usage cell text of a tenant. A tenant with no daily row has a null (or
 * missing) usage day and shows "-", not zeros.
 */
function usageSummary(tenant: Tenant): string {
  if (!tenant.usage_day) return "-";
  return [
    `${tenant.usage_members ?? 0} members`,
    `${tenant.usage_agents ?? 0} agents`,
    `${tenant.usage_tasks_run ?? 0} tasks run`,
    formatBytes(tenant.usage_storage_bytes ?? 0),
  ].join(" · ");
}

interface TenantsProps {
  onSelectTenant: (id: string) => void;
}

export function Tenants({ onSelectTenant }: TenantsProps) {
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
      <PageHeader title="Tenants" />

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative w-full max-w-sm">
          <Search
            className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            aria-label="Search tenants"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name or slug..."
            className="pr-8 pl-9"
          />
          {search && (
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label="Clear search"
              onClick={() => setSearch("")}
              className="absolute top-1 right-1 text-muted-foreground hover:text-foreground"
            >
              <X className="size-3.5" aria-hidden="true" />
            </Button>
          )}
        </div>
      </div>

      {isLoading && <LoadingState message="Loading tenants..." />}

      {isError && (
        <ErrorState
          title="Failed to load tenants"
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && tenants.length === 0 && (
        <EmptyState
          title="No tenants found"
          description={
            search
              ? "No tenants matched your search query."
              : "There are currently no registered tenants."
          }
          actions={
            search ? (
              <Button variant="outline" size="sm" onClick={() => setSearch("")}>
                Clear search
              </Button>
            ) : undefined
          }
        />
      )}

      {!isLoading && !isError && tenants.length > 0 && (
        <div className="space-y-4">
          <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Slug</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Members</TableHead>
                  <TableHead>Usage</TableHead>
                  <TableHead>Last Activity</TableHead>
                  <TableHead className="text-right">Action</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tenants.map((tenant: Tenant) => (
                  <TableRow key={tenant.id}>
                    <TableCell>
                      <div className="max-w-56 truncate font-medium text-foreground" title={tenant.name}>
                        {tenant.name}
                      </div>
                      <div
                        className="max-w-44 truncate font-mono text-caption text-muted-foreground"
                        title={tenant.id}
                      >
                        {tenant.id}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div
                        className="max-w-40 truncate font-mono text-caption text-muted-foreground"
                        title={tenant.slug}
                      >
                        {tenant.slug}
                      </div>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={tenant.status} />
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {tenant.member_count}
                    </TableCell>
                    <TableCell
                      data-testid="tenant-usage"
                      className="text-muted-foreground tabular-nums"
                      title={
                        tenant.usage_day
                          ? `Usage on ${tenant.usage_day} (UTC)`
                          : "No usage recorded yet"
                      }
                    >
                      {usageSummary(tenant)}
                    </TableCell>
                    <TableCell className="text-muted-foreground tabular-nums">
                      {tenant.last_activity_at
                        ? new Date(tenant.last_activity_at).toLocaleString()
                        : "-"}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => onSelectTenant(tenant.id)}
                      >
                        View
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          {hasNextPage && (
            <div className="flex justify-center pt-2">
              <Button
                variant="outline"
                onClick={() => fetchNextPage()}
                disabled={isFetchingNextPage}
                aria-busy={isFetchingNextPage}
              >
                {isFetchingNextPage && <Spinner className="size-3.5" />}
                Load more
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
