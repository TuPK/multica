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
import { Search } from "@multica/ui/icons";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { useTenants } from "../api/hooks";
import type { Tenant } from "../api/schemas";

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
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="font-heading text-title-lg font-bold">Tenants</h1>
          <p className="text-body text-muted-foreground">
            View and manage workspace tenants.
          </p>
        </div>
        <div className="relative w-full max-w-xs">
          <Search className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
          <Input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name or slug..."
            className="pl-9"
          />
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
        />
      )}

      {!isLoading && !isError && tenants.length > 0 && (
        <div className="space-y-4">
          <div className="rounded-lg border border-surface-border bg-surface shadow-xs">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Slug</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Members</TableHead>
                  <TableHead>Last Activity</TableHead>
                  <TableHead className="text-right">Action</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tenants.map((tenant: Tenant) => (
                  <TableRow key={tenant.id}>
                    <TableCell className="font-medium">{tenant.name}</TableCell>
                    <TableCell className="font-mono text-caption text-muted-foreground">
                      {tenant.slug}
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={tenant.status} />
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {tenant.member_count}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
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
