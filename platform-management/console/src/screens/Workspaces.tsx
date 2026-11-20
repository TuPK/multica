import { useState } from "react";
import { HStack } from "@astryxdesign/core/HStack";
import { Heading } from "@astryxdesign/core/Heading";
import { Layout, LayoutContent, LayoutHeader } from "@astryxdesign/core/Layout";
import { Section } from "@astryxdesign/core/Section";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { LayoutGrid, Search, X } from "@multica/ui/icons";
import { RowActions } from "../components/RowActions";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import {
  useMe,
  usePlans,
  useTenants,
  useWorkspaces,
  type WorkspaceListItem,
} from "../api/hooks";
import { WORKSPACE_STATUSES, type StaffRole } from "../api/schemas";

interface WorkspacesProps {
  onSelectWorkspace: (id: string) => void;
  userRole?: StaffRole;
}

const STATUS_OPTIONS: Array<{ value: string; label: string }> = [
  { value: "all", label: "All statuses" },
  ...WORKSPACE_STATUSES.map((s) => ({
    value: s,
    label:
      s === "read_only"
        ? "Read-only"
        : s === "pending_deletion"
          ? "Pending deletion"
          : s.charAt(0).toUpperCase() + s.slice(1),
  })),
];

export function Workspaces({ onSelectWorkspace, userRole }: WorkspacesProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole || me?.role;
  const canReadPlans = effectiveRole === "system_admin" || effectiveRole === "admin";

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [planFilter, setPlanFilter] = useState("");
  const [tenantFilter, setTenantFilter] = useState("");

  const { data: plansData } = usePlans(canReadPlans);
  const plans = Array.isArray(plansData) ? plansData : [];
  const { data: tenantsData } = useTenants({});
  const tenants = Array.isArray(tenantsData)
    ? tenantsData
    : ((tenantsData as unknown as { tenants?: Array<{ id: string; name?: string }> })?.tenants ?? []);

  const {
    data: workspaces = [],
    isLoading,
    isError,
    error,
    refetch,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useWorkspaces({
    q: search.trim() || undefined,
    status: statusFilter || undefined,
    plan_key: planFilter || undefined,
    tenant_id: tenantFilter || undefined,
  });

  const planOptions: Array<{ value: string; label: string }> = [
    { value: "all", label: "All plans" },
    ...plans.map((p) => ({
      value: p.key,
      label: p.name ? `${p.name} (${p.key})` : p.key,
    })),
  ];

  const tenantOptions: Array<{ value: string; label: string }> = [
    { value: "all", label: "All tenants" },
    ...tenants.map((t) => ({
      value: t.id,
      label: t.name || t.id,
    })),
  ];

  const hasActiveFilters = Boolean(
    search.trim() || statusFilter || planFilter || tenantFilter,
  );

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("");
    setPlanFilter("");
    setTenantFilter("");
  };

  return (
    <Layout
      header={
        <LayoutHeader>
          <PageHeader
            title="Workspaces"
            meta={
              !isLoading && !isError
                ? `${workspaces.length.toLocaleString()} shown`
                : undefined
            }
          />
        </LayoutHeader>
      }
      content={
        <LayoutContent>
          <div className="space-y-6">
            <Section>
              <div className="space-y-4">
                <HStack wrap="wrap" gap={2} align="center">
                  <div className="relative w-full max-w-sm">
                    <Search
                      aria-hidden="true"
                      className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
                    />
                    <Input
                      type="search"
                      aria-label="Search workspaces"
                      placeholder="Search by workspace name or slug..."
                      value={search}
                      onChange={(e) => setSearch(e.target.value)}
                      className="pl-8 pr-8"
                    />
                    {search && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-xs"
                        aria-label="Clear search"
                        onClick={() => setSearch("")}
                        className="absolute top-1/2 right-1.5 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                      >
                        <X className="size-3.5" aria-hidden="true" />
                      </Button>
                    )}
                  </div>

                  <Select
                    items={STATUS_OPTIONS}
                    value={statusFilter || "all"}
                    onValueChange={(value) =>
                      setStatusFilter(value === "all" ? "" : (value ?? ""))
                    }
                  >
                    <SelectTrigger aria-label="Filter by status" className="w-44">
                      <SelectValue placeholder="All statuses" />
                    </SelectTrigger>
                    <SelectContent>
                      {STATUS_OPTIONS.map((opt) => (
                        <SelectItem key={opt.value} value={opt.value}>
                          {opt.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>

                  {canReadPlans && (
                    <Select
                      items={planOptions}
                      value={planFilter || "all"}
                      onValueChange={(value) =>
                        setPlanFilter(value === "all" ? "" : (value ?? ""))
                      }
                    >
                      <SelectTrigger aria-label="Filter by plan" className="w-44">
                        <SelectValue placeholder="All plans" />
                      </SelectTrigger>
                      <SelectContent>
                        {planOptions.map((opt) => (
                          <SelectItem key={opt.value} value={opt.value}>
                            {opt.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}

                  <Select
                    items={tenantOptions}
                    value={tenantFilter || "all"}
                    onValueChange={(value) =>
                      setTenantFilter(value === "all" ? "" : (value ?? ""))
                    }
                  >
                    <SelectTrigger aria-label="Filter by tenant" className="w-48">
                      <SelectValue placeholder="All tenants" />
                    </SelectTrigger>
                    <SelectContent>
                      {tenantOptions.map((opt) => (
                        <SelectItem key={opt.value} value={opt.value}>
                          {opt.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>

                  {hasActiveFilters && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={clearFilters}
                    >
                      Clear filters
                    </Button>
                  )}
                </HStack>

                <Heading level={3}>Directory</Heading>

                {isLoading && <LoadingState message="Loading workspaces..." />}

                {isError && (
                  <ErrorState
                    title="Failed to load workspaces"
                    message={error instanceof Error ? error.message : undefined}
                    onRetry={refetch}
                  />
                )}

                {!isLoading && !isError && workspaces.length === 0 && (
                  <EmptyState
                    icon={<LayoutGrid className="size-5" aria-hidden="true" />}
                    title="No workspaces found"
                    description={
                      hasActiveFilters
                        ? "No workspaces matched your current filters."
                        : "There are no workspaces available for your account."
                    }
                    actions={
                      hasActiveFilters ? (
                        <Button variant="outline" size="sm" onClick={clearFilters}>
                          Clear filters
                        </Button>
                      ) : undefined
                    }
                  />
                )}

                {!isLoading && !isError && workspaces.length > 0 && (
                  <div className="space-y-4">
                    <div className="overflow-x-auto">
                      <Table density="balanced" dividers="rows" hasHover>
                        <TableHeader>
                          <TableRow isHeaderRow>
                            <TableHeaderCell>Workspace</TableHeaderCell>
                            <TableHeaderCell>Tenant</TableHeaderCell>
                            <TableHeaderCell>Status</TableHeaderCell>
                            <TableHeaderCell>Plan</TableHeaderCell>
                            <TableHeaderCell>Owner</TableHeaderCell>
                            <TableHeaderCell className="text-right">Members</TableHeaderCell>
                            <TableHeaderCell>Created</TableHeaderCell>
                            <TableHeaderCell className="text-right">Action</TableHeaderCell>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {workspaces.map((ws: WorkspaceListItem) => (
                            <TableRow key={ws.id}>
                              <TableCell>
                                <div
                                  className="max-w-56 truncate font-medium text-foreground"
                                  title={ws.name}
                                >
                                  {ws.name}
                                </div>
                                <div
                                  className="max-w-44 truncate font-mono text-caption text-muted-foreground"
                                  title={ws.slug || ws.id}
                                >
                                  {ws.slug || ws.id}
                                </div>
                              </TableCell>
                              <TableCell>
                                <div
                                  className="max-w-44 truncate text-body"
                                  title={ws.tenant_name || ws.tenant_id}
                                >
                                  {ws.tenant_name || ws.tenant_id || "-"}
                                </div>
                              </TableCell>
                              <TableCell>
                                <StatusBadge status={ws.status} />
                              </TableCell>
                              <TableCell>
                                <span className="font-mono text-caption text-foreground">
                                  {ws.plan_key || "-"}
                                </span>
                              </TableCell>
                              <TableCell>
                                <Text type="supporting">
                                  {ws.owner_email || "-"}
                                </Text>
                              </TableCell>
                              <TableCell className="text-right">
                                <Text type="body" hasTabularNumbers>
                                  {ws.member_count.toLocaleString()}
                                </Text>
                              </TableCell>
                              <TableCell>
                                <Text type="supporting" hasTabularNumbers>
                                  {ws.created_at
                                    ? new Date(ws.created_at).toLocaleString()
                                    : "-"}
                                </Text>
                              </TableCell>
                              <TableCell className="text-right">
                                <RowActions label="workspace" actions={[{ label: "View", onSelect: () => onSelectWorkspace(ws.id) }]} />
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </div>

                    {hasNextPage && (
                      <div className="flex justify-center pt-2">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={isFetchingNextPage}
                          aria-busy={isFetchingNextPage || undefined}
                          onClick={() => void fetchNextPage()}
                        >
                          {isFetchingNextPage && (
                            <Spinner className="size-4" aria-hidden="true" />
                          )}
                          {isFetchingNextPage ? "Loading..." : "Load more"}
                        </Button>
                      </div>
                    )}
                  </div>
                )}
              </div>
            </Section>
          </div>
        </LayoutContent>
      }
    />
  );
}
