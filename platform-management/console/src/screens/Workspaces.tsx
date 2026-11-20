import { useState } from "react";
import { HStack } from "@astryxdesign/core/HStack";
import { Heading } from "@astryxdesign/core/Heading";
import { Layout, LayoutContent, LayoutHeader } from "@astryxdesign/core/Layout";
import { Card } from "@astryxdesign/core/Card";
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
import { Button } from "@astryxdesign/core/Button";
import { Selector } from "@astryxdesign/core/Selector";
import { TextInput } from "@astryxdesign/core/TextInput";
import { LayoutGrid, Search } from "@multica/ui/icons";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { WorkspaceAccountMenu } from "./WorkspaceAccountMenu";
import { StatusBadge } from "../components/StatusBadge";
import {
  useMe,
  usePlans,
  useTenants,
  useWorkspaces,
  type WorkspaceListItem,
} from "../api/hooks";
import { WORKSPACE_STATUSES, type StaffRole } from "../api/schemas";
import { t } from "../i18n";

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
        ? t("Read-only")
        : s === "pending_deletion"
          ? t("Pending deletion")
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
    { value: "all", label: t("All plans") },
    ...plans.map((p) => ({
      value: p.key,
      label: p.name ? `${p.name} (${p.key})` : p.key,
    })),
  ];

  const tenantOptions: Array<{ value: string; label: string }> = [
    { value: "all", label: t("All tenants") },
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
        <LayoutHeader padding={0}>
          <PageHeader
            title={t("Workspaces")}
            meta={
              !isLoading && !isError
                ? t("{0} shown", workspaces.length.toLocaleString())
                : undefined
            }
          />
        </LayoutHeader>
      }
      content={
        <LayoutContent padding={0}>
          <div className="space-y-6">
            <Section>
              <div className="space-y-4">
                <HStack wrap="wrap" gap={2} align="center">
                  <div className="w-full max-w-sm">
                    <TextInput
                      type="search"
                      label={t("Search workspaces")}
                      isLabelHidden
                      width="100%"
                      startIcon={<Search className="size-4" aria-hidden="true" />}
                      hasClear
                      placeholder={t("Search by workspace name or slug...")}
                      value={search}
                      onChange={(value) => setSearch(value)}
                    />
                  </div>

                  <Selector
                    label={t("Filter by status")}
                    isLabelHidden
                    width={176}
                    options={STATUS_OPTIONS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                    value={statusFilter || "all"}
                    onChange={(value) => setStatusFilter(value === "all" ? "" : value)}
                  />

                  {canReadPlans && (
                    <Selector
                      label={t("Filter by plan")}
                      isLabelHidden
                      width={176}
                      options={planOptions}
                      value={planFilter || "all"}
                      onChange={(value) => setPlanFilter(value === "all" ? "" : value)}
                    />
                  )}

                  <Selector
                    label={t("Filter by tenant")}
                    isLabelHidden
                    width={192}
                    options={tenantOptions}
                    value={tenantFilter || "all"}
                    onChange={(value) => setTenantFilter(value === "all" ? "" : value)}
                  />

                  {hasActiveFilters && (
                    <Button
                      type="button"
                      label={t("Clear filters")}
                      variant="ghost"
                      size="sm"
                      onClick={clearFilters}
                    />
                  )}
                </HStack>

                <Heading level={3}>{t("Directory")}</Heading>

                {isLoading && <LoadingState message={t("Loading workspaces...")} />}

                {isError && (
                  <ErrorState
                    title={t("Failed to load workspaces")}
                    message={error instanceof Error ? error.message : undefined}
                    onRetry={refetch}
                  />
                )}

                {!isLoading && !isError && workspaces.length === 0 && (
                  <EmptyState
                    icon={<LayoutGrid className="size-5" aria-hidden="true" />}
                    title={t("No workspaces found")}
                    description={
                      hasActiveFilters
                        ? t("No workspaces matched your current filters.")
                        : t("There are no workspaces available for your account.")
                    }
                    actions={
                      hasActiveFilters ? (
                        <Button label={t("Clear filters")} variant="secondary" size="sm" onClick={clearFilters} />
                      ) : undefined
                    }
                  />
                )}

                {!isLoading && !isError && workspaces.length > 0 && (
                  <div className="space-y-4">
                    <Card padding={0} className="overflow-x-auto">
                      <Table density="compact" dividers="rows" hasHover>
                        <TableHeader>
                          <TableRow isHeaderRow>
                            <TableHeaderCell>{t("Workspace")}</TableHeaderCell>
                            <TableHeaderCell>{t("Tenant")}</TableHeaderCell>
                            <TableHeaderCell>{t("Status")}</TableHeaderCell>
                            <TableHeaderCell>{t("Plan")}</TableHeaderCell>
                            <TableHeaderCell>{t("Owner")}</TableHeaderCell>
                            <TableHeaderCell className="text-right">{t("Members")}</TableHeaderCell>
                            <TableHeaderCell>{t("Created")}</TableHeaderCell>
                            <TableHeaderCell className="text-right">{t("Action")}</TableHeaderCell>
                        </TableRow>
                        </TableHeader>
                        <TableBody>
                          {workspaces.map((ws: WorkspaceListItem) => (
                            <TableRow key={ws.id} onClick={() => onSelectWorkspace(ws.id)}>
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
                              <TableCell className="text-right" onClick={(event) => event.stopPropagation()}>
                                <WorkspaceAccountMenu
                                  workspaceId={ws.id}
                                  workspaceName={ws.name}
                                  workspaceSlug={ws.slug || ws.id}
                                  workspaceStatus={ws.status}
                                  role={effectiveRole}
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
                          type="button"
                          label={isFetchingNextPage ? t("Loading...") : t("Load more")}
                          variant="secondary"
                          size="sm"
                          isDisabled={isFetchingNextPage}
                          isLoading={isFetchingNextPage}
                          aria-busy={isFetchingNextPage || undefined}
                          onClick={() => void fetchNextPage()}
                        />
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
