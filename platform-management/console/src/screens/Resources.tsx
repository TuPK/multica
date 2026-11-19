import { useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
import { Card } from "@astryxdesign/core/Card";
import { HStack } from "@astryxdesign/core/HStack";
import { Heading } from "@astryxdesign/core/Heading";
import { Layout, LayoutContent, LayoutHeader } from "@astryxdesign/core/Layout";
import { ProgressBar } from "@astryxdesign/core/ProgressBar";
import { Section } from "@astryxdesign/core/Section";
import { StatusDot } from "@astryxdesign/core/StatusDot";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
} from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { AlertCircle, Gauge, RefreshCw } from "@multica/ui/icons";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import {
  useMe,
  useSystemResources,
  type OverLimitResource,
  type ResourceState,
  type SystemResourcePlanSummary,
  type SystemResourceTypeSummary,
  type SystemTopTenant,
} from "../api/hooks";
import type { StaffRole } from "../components/types";
import { formatBytes } from "./Tenants";

interface ResourcesProps {
  onSelectTenant?: (tenantId: string) => void;
  onSelectWorkspace?: (workspaceId: string) => void;
  userRole?: StaffRole;
}

const SORT_OPTIONS: Array<{ value: string; label: string }> = [
  { value: "default", label: "Default order" },
  { value: "storage_bytes", label: "Storage" },
  { value: "members", label: "Members" },
  { value: "agents", label: "Agents" },
  { value: "runtimes", label: "Runtimes" },
  { value: "issues_total", label: "Issues" },
  { value: "autopilot_runs", label: "Autopilot runs" },
  { value: "over_limit", label: "Over-limit count" },
];

export const RESOURCE_TYPE_LABELS: Record<string, string> = {
  members: "Members",
  seats_held: "Held seats",
  agents: "Agents",
  runtimes: "Runtimes",
  issues_total: "Issues",
  issue_count: "Issues",
  tasks_run: "Tasks run",
  storage_bytes: "Storage",
  autopilot_runs: "Autopilot runs",
  member_groups: "Member groups",
  max_workspaces: "Max workspaces",
  api_rate_per_minute: "API rate (/min)",
  audit_retention_days: "Audit retention (days)",
};

export function resourceTypeLabel(type: string, label?: string): string {
  if (label && label.trim() !== "") return label;
  return RESOURCE_TYPE_LABELS[type] ?? (type || "Unknown");
}

export function formatResourceAmount(type: string, value: number): string {
  if (type === "storage_bytes") {
    return formatBytes(value);
  }
  return value.toLocaleString();
}

export function formatResourceLimit(type: string, limit: number | null): string {
  if (limit === null) return "Unlimited";
  return formatResourceAmount(type, limit);
}

export function computeResourcePercent(
  used: number,
  limit: number | null,
  serverPercent: number | null,
): number | null {
  if (serverPercent !== null && Number.isFinite(serverPercent)) {
    return Math.round(serverPercent);
  }
  if (limit === null || limit <= 0) return null;
  return Math.round((used / limit) * 100);
}

export function resolveResourceState(
  state: ResourceState,
  percent: number | null,
  limit: number | null,
): ResourceState {
  if (state !== "unknown") return state;
  if (limit === null || percent === null) return "unknown";
  if (percent >= 100) return "over";
  if (percent >= 80) return "warning";
  return "ok";
}

export function ResourceStateBadge({ state }: { state: ResourceState }) {
  switch (state) {
    case "ok":
      return (
        <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
          <StatusDot variant="success" label="OK" aria-hidden="true" />
          <Text type="body" color="primary">
            OK
          </Text>
        </span>
      );
    case "warning":
      return (
        <Badge
          variant="warning"
          label="Warning"
          icon={<StatusDot variant="warning" label="Warning" aria-hidden="true" />}
        />
      );
    case "over":
      return (
        <Badge
          variant="error"
          label="Over limit"
          icon={<StatusDot variant="error" label="Over limit" aria-hidden="true" />}
        />
      );
    default:
      return <Badge variant="neutral" label="Unknown" />;
  }
}

export function Resources({ onSelectTenant, onSelectWorkspace, userRole }: ResourcesProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;
  const canView = effectiveRole === "system_admin";

  const [sort, setSort] = useState("");

  const {
    data,
    isLoading,
    isFetching,
    isError,
    error,
    refetch,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useSystemResources({ sort: sort || undefined }, canView);

  if (effectiveRole && !canView) {
    return (
      <div role="region" aria-label="Not found">
        <ErrorState
          title="Not found"
          message="The page you requested was not found or you do not have permission to view it."
        />
      </div>
    );
  }

  const byType = data?.by_type ?? [];
  const byPlan = data?.by_plan ?? [];
  const overLimit = data?.over_limit ?? [];
  const tenantRows =
    (data?.top_tenants.length ?? 0) > 0 ? data!.top_tenants : (data?.tenants ?? []);

  return (
    <Layout
      header={
        <LayoutHeader hasDivider>
          <PageHeader
            title="Resources"
            actions={
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void refetch()}
                disabled={Boolean(isFetching)}
                aria-busy={Boolean(isFetching) || undefined}
              >
                {isFetching ? <Spinner className="size-3.5" /> : <RefreshCw className="size-3.5" aria-hidden="true" />}
                <span>Refresh</span>
              </Button>
            }
          />
        </LayoutHeader>
      }
      content={
        <LayoutContent>
          <div className="space-y-6">
            {data && !data.complete && (
              <Alert role="status">
                <AlertCircle className="size-4" aria-hidden="true" />
                <AlertDescription>
                  Resource totals are partial across the loaded pages; load remaining pages for complete totals.
                </AlertDescription>
              </Alert>
            )}

            <HStack wrap="wrap" gap={2} align="center">
              <Select
                items={SORT_OPTIONS}
                value={sort || "default"}
                onValueChange={(value) => setSort(!value || value === "default" ? "" : value)}
              >
                <SelectTrigger aria-label="Sort by" className="w-52">
                  <SelectValue placeholder="Default order" />
                </SelectTrigger>
                <SelectContent>
                  {SORT_OPTIONS.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </HStack>

            {isLoading && <LoadingState message="Loading resources..." />}

            {isError && (
              <ErrorState
                title="Failed to load resources"
                message={error instanceof Error ? error.message : undefined}
                onRetry={refetch}
              />
            )}

            {!isLoading && !isError && data && (
              <div className="space-y-6">
                {byType.length > 0 && (
                  <Section variant="transparent" padding={0} className="space-y-3">
                    <Heading level={3}>Platform Totals</Heading>
                    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
                      {byType.map((item: SystemResourceTypeSummary) => (
                        <Card key={item.type} padding={4}>
                          <Text type="label" color="secondary">
                            {resourceTypeLabel(item.type, item.label)}
                          </Text>
                          <div className="mt-2 space-y-1">
                            <Text type="display-3" weight="semibold" hasTabularNumbers as="div">
                              {formatResourceAmount(item.type, item.used)}
                            </Text>
                            {item.limit !== null && (
                              <Text type="supporting" hasTabularNumbers as="p">
                                Limit {formatResourceLimit(item.type, item.limit)}
                                {item.percent !== null ? ` (${Math.round(item.percent)}%)` : ""}
                              </Text>
                            )}
                          </div>
                        </Card>
                      ))}
                    </div>
                  </Section>
                )}

                <Section className="space-y-4">
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <Heading level={3}>Over-limit &amp; Warning Workspaces</Heading>
                    <Text type="supporting" hasTabularNumbers>
                      {overLimit.length.toLocaleString()} flagged
                    </Text>
                  </div>

                  {overLimit.length === 0 ? (
                    <EmptyState
                      icon={<Gauge className="size-5" aria-hidden="true" />}
                      title="No workspaces near or over limit"
                      isCompact
                    />
                  ) : (
                    <div className="overflow-x-auto">
                      <Table density="compact" dividers="rows" hasHover>
                        <TableHeader>
                          <TableRow isHeaderRow>
                            <TableHeaderCell>Tenant</TableHeaderCell>
                            <TableHeaderCell>Workspace</TableHeaderCell>
                            <TableHeaderCell>Resource</TableHeaderCell>
                            <TableHeaderCell className="text-right">Used / Limit</TableHeaderCell>
                            <TableHeaderCell className="w-40">Usage</TableHeaderCell>
                            <TableHeaderCell>Status</TableHeaderCell>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {overLimit.map((row: OverLimitResource, idx: number) => {
                            const pct = computeResourcePercent(row.used, row.limit, row.percent);
                            const resolvedState = resolveResourceState(row.state, pct, row.limit);
                            const barVariant =
                              resolvedState === "over"
                                ? "error"
                                : resolvedState === "warning"
                                  ? "warning"
                                  : "success";
                            return (
                              <TableRow
                                key={`${row.tenant_id}-${row.workspace_id}-${row.type}-${idx}`}
                              >
                                <TableCell>
                                  {onSelectTenant && row.tenant_id ? (
                                    <Button
                                      type="button"
                                      variant="link"
                                      size="sm"
                                      className="h-auto p-0 font-medium"
                                      onClick={() => onSelectTenant(row.tenant_id)}
                                    >
                                      {row.tenant_name || row.tenant_id}
                                    </Button>
                                  ) : (
                                    <Text type="body" weight="medium">
                                      {row.tenant_name || row.tenant_id || "-"}
                                    </Text>
                                  )}
                                </TableCell>
                                <TableCell>
                                  {onSelectWorkspace && row.workspace_id ? (
                                    <Button
                                      type="button"
                                      variant="link"
                                      size="sm"
                                      className="h-auto p-0"
                                      onClick={() => onSelectWorkspace(row.workspace_id)}
                                    >
                                      {row.workspace_name || row.workspace_id}
                                    </Button>
                                  ) : (
                                    <Text type="body">
                                      {row.workspace_name || row.workspace_id || "-"}
                                    </Text>
                                  )}
                                </TableCell>
                                <TableCell>
                                  <Text type="body">{resourceTypeLabel(row.type)}</Text>
                                </TableCell>
                                <TableCell className="text-right">
                                  <Text type="body" hasTabularNumbers>
                                    {formatResourceAmount(row.type, row.used)} /{" "}
                                    {formatResourceLimit(row.type, row.limit)}
                                    {pct !== null ? ` (${pct}%)` : ""}
                                  </Text>
                                </TableCell>
                                <TableCell>
                                  {row.limit !== null && row.limit > 0 ? (
                                    <ProgressBar
                                      value={row.used}
                                      max={row.limit}
                                      variant={barVariant}
                                      label={`${resourceTypeLabel(row.type)} usage`}
                                      isLabelHidden
                                    />
                                  ) : (
                                    <Text type="supporting">-</Text>
                                  )}
                                </TableCell>
                                <TableCell>
                                  <ResourceStateBadge state={resolvedState} />
                                </TableCell>
                              </TableRow>
                            );
                          })}
                        </TableBody>
                      </Table>
                    </div>
                  )}
                </Section>

                <Section className="space-y-4">
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <Heading level={3}>Top Tenants</Heading>
                    <Text type="supporting" hasTabularNumbers>
                      {tenantRows.length.toLocaleString()} shown
                    </Text>
                  </div>

                  {tenantRows.length === 0 ? (
                    <EmptyState
                      icon={<Gauge className="size-5" aria-hidden="true" />}
                      title="No tenant resource data"
                      isCompact
                    />
                  ) : (
                    <div className="space-y-4">
                      <div className="overflow-x-auto">
                        <Table density="balanced" dividers="rows" hasHover>
                          <TableHeader>
                            <TableRow isHeaderRow>
                              <TableHeaderCell>Tenant</TableHeaderCell>
                              <TableHeaderCell>Plan</TableHeaderCell>
                              <TableHeaderCell className="text-right">Workspaces</TableHeaderCell>
                              <TableHeaderCell className="text-right">Members</TableHeaderCell>
                              <TableHeaderCell className="text-right">Agents</TableHeaderCell>
                              <TableHeaderCell className="text-right">Runtimes</TableHeaderCell>
                              <TableHeaderCell className="text-right">Issues</TableHeaderCell>
                              <TableHeaderCell className="text-right">Storage</TableHeaderCell>
                              <TableHeaderCell>Status</TableHeaderCell>
                              {onSelectTenant && (
                                <TableHeaderCell className="text-right">Action</TableHeaderCell>
                              )}
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {tenantRows.map((row: SystemTopTenant) => (
                              <TableRow key={row.tenant_id || row.tenant_name}>
                                <TableCell>
                                  <div
                                    className="max-w-52 truncate font-medium text-foreground"
                                    title={row.tenant_name || row.tenant_id}
                                  >
                                    {row.tenant_name || row.tenant_id || "-"}
                                  </div>
                                  {row.tenant_name && row.tenant_id && (
                                    <div className="max-w-52 truncate font-mono text-caption text-muted-foreground">
                                      {row.tenant_id}
                                    </div>
                                  )}
                                </TableCell>
                                <TableCell>
                                  <Text type="code">{row.best_plan_key || "-"}</Text>
                                </TableCell>
                                <TableCell className="text-right">
                                  <Text type="body" hasTabularNumbers>
                                    {row.workspace_count.toLocaleString()}
                                  </Text>
                                </TableCell>
                                <TableCell className="text-right">
                                  <Text type="body" hasTabularNumbers>
                                    {row.members.toLocaleString()}
                                  </Text>
                                </TableCell>
                                <TableCell className="text-right">
                                  <Text type="body" hasTabularNumbers>
                                    {row.agents.toLocaleString()}
                                  </Text>
                                </TableCell>
                                <TableCell className="text-right">
                                  <Text type="body" hasTabularNumbers>
                                    {row.runtimes.toLocaleString()}
                                  </Text>
                                </TableCell>
                                <TableCell className="text-right">
                                  <Text type="body" hasTabularNumbers>
                                    {row.issues_total.toLocaleString()}
                                  </Text>
                                </TableCell>
                                <TableCell className="text-right">
                                  <Text type="body" hasTabularNumbers>
                                    {formatBytes(row.storage_bytes)}
                                  </Text>
                                </TableCell>
                                <TableCell>
                                  <ResourceStateBadge state={row.state} />
                                </TableCell>
                                {onSelectTenant && (
                                  <TableCell className="text-right">
                                    <Button
                                      type="button"
                                      variant="outline"
                                      size="xs"
                                      disabled={!row.tenant_id}
                                      onClick={() => onSelectTenant(row.tenant_id)}
                                    >
                                      View
                                    </Button>
                                  </TableCell>
                                )}
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
                </Section>

                {byPlan.length > 0 && (
                  <Section className="space-y-4">
                    <Heading level={3}>Usage by Plan</Heading>
                    <div className="overflow-x-auto">
                      <Table density="compact" dividers="rows" hasHover>
                        <TableHeader>
                          <TableRow isHeaderRow>
                            <TableHeaderCell>Plan</TableHeaderCell>
                            <TableHeaderCell className="text-right">Tenants</TableHeaderCell>
                            <TableHeaderCell className="text-right">Workspaces</TableHeaderCell>
                            <TableHeaderCell>Totals</TableHeaderCell>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {byPlan.map((planRow: SystemResourcePlanSummary) => {
                            const summary = Object.entries(planRow.totals)
                              .map(
                                ([key, value]) =>
                                  `${resourceTypeLabel(key)}: ${formatResourceAmount(key, value)}`,
                              )
                              .join(" · ");
                            return (
                              <TableRow key={planRow.plan_key}>
                                <TableCell>
                                  <Text type="code" weight="medium">
                                    {planRow.plan_key || "-"}
                                  </Text>
                                </TableCell>
                                <TableCell className="text-right">
                                  <Text type="body" hasTabularNumbers>
                                    {planRow.tenant_count.toLocaleString()}
                                  </Text>
                                </TableCell>
                                <TableCell className="text-right">
                                  <Text type="body" hasTabularNumbers>
                                    {planRow.workspace_count.toLocaleString()}
                                  </Text>
                                </TableCell>
                                <TableCell>
                                  <Text type="supporting" hasTabularNumbers>
                                    {summary || "-"}
                                  </Text>
                                </TableCell>
                              </TableRow>
                            );
                          })}
                        </TableBody>
                      </Table>
                    </div>
                  </Section>
                )}
              </div>
            )}
          </div>
        </LayoutContent>
      }
    />
  );
}
