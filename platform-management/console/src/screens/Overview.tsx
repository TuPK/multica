import { Card } from "@astryxdesign/core/Card";
import { Divider } from "@astryxdesign/core/Divider";
import { Heading } from "@astryxdesign/core/Heading";
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
import {
  Chart,
  ChartAxis,
  ChartGrid,
  ChartSwatch,
  bar,
} from "@astryxdesign/charts";
import { Button } from "@multica/ui/components/ui/button";
import { Spinner } from "@multica/ui/components/ui/spinner";
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Building2,
  CreditCard,
  RefreshCw,
  Shield,
  Users,
} from "@multica/ui/icons";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { useOverview } from "../api/hooks";
import { formatMinorAmount } from "../money";
import { formatBytes } from "./Tenants";
import { t } from "../i18n";

const PLAN_SERIES_COLORS = [
  "var(--data-color-1, #0171e3)",
  "var(--data-color-2, #eb6e00)",
  "var(--data-color-3, #0b991f)",
  "var(--data-color-4, #6b1efd)",
  "var(--data-color-5, #08a3a3)",
] as const;

interface StatusDatum {
  key: string;
  status: string;
  count: number;
  color: string;
}

interface PlanDatum {
  plan: string;
  count: number;
  color: string;
}

function formatShare(value: number, total: number): string {
  if (total <= 0) return "0%";
  const pct = (value / total) * 100;
  return pct >= 10 || Number.isInteger(pct) ? `${pct.toFixed(0)}%` : `${pct.toFixed(1)}%`;
}

// Counts are whole numbers; hide the fractional ticks the chart picks for small maxima.
function integerTick(value: unknown): string {
  const n = Number(value);
  return Number.isInteger(n) ? String(n) : "";
}

export function Overview() {
  const { data, isLoading, isFetching, isError, error, refetch } = useOverview();

  if (isLoading) {
    return <LoadingState message={t("Loading overview...")} />;
  }

  if (isError) {
    return (
      <ErrorState
        title={t("Failed to load overview")}
        message={error instanceof Error ? error.message : undefined}
        onRetry={refetch}
      />
    );
  }

  if (!data) {
    return (
      <EmptyState
        icon={<BarChart3 className="size-5" aria-hidden="true" />}
        title={t("No overview data")}
        description={t("Unable to load workspace metrics.")}
        actions={
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            {t("Refresh")}
          </Button>
        }
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

  const statusData: StatusDatum[] = [
    {
      key: "active",
      status: "Active",
      count: activeCount,
      color: "var(--color-success)",
    },
    {
      key: "read_only",
      status: "Read-only",
      count: readOnlyCount,
      color: "var(--data-color-5, #08a3a3)",
    },
    {
      key: "suspended",
      status: "Suspended",
      count: suspendedCount,
      color: "var(--color-warning)",
    },
    {
      key: "pending_deletion",
      status: "Pending deletion",
      count: pendingDeletionCount,
      color: "var(--color-error)",
    },
  ];

  const planEntries = data.by_plan
    ? Object.entries(data.by_plan).sort(([, a], [, b]) => b - a)
    : [];
  const totalPlanSubscriptions = planEntries.reduce((sum, [, count]) => sum + count, 0);
  const planData: PlanDatum[] = planEntries.map(([planKey, count], index) => ({
    plan: planKey,
    count,
    color: PLAN_SERIES_COLORS[index % PLAN_SERIES_COLORS.length]!,
  }));

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

  const usageRows = hasUsageSnapshot
    ? [
        { label: t("Usage Workspaces"), raw: data.usage_workspaces ?? 0, formatted: String(data.usage_workspaces ?? 0) },
        { label: t("Members"), raw: data.usage_members ?? 0, formatted: String(data.usage_members ?? 0) },
        { label: t("Agents"), raw: data.usage_agents ?? 0, formatted: String(data.usage_agents ?? 0) },
        { label: t("Runtimes"), raw: data.usage_runtimes ?? 0, formatted: String(data.usage_runtimes ?? 0) },
        { label: t("Total Issues"), raw: data.usage_issues_total ?? 0, formatted: String(data.usage_issues_total ?? 0) },
        { label: t("Issues Created"), raw: data.usage_issues_created ?? 0, formatted: String(data.usage_issues_created ?? 0) },
        { label: t("Tasks Run"), raw: data.usage_tasks_run ?? 0, formatted: String(data.usage_tasks_run ?? 0) },
        { label: t("Storage"), raw: null as number | null, formatted: formatBytes(data.usage_storage_bytes ?? 0) },
      ]
    : [];
  const maxUsageCount = Math.max(
    ...usageRows.map((row) => (row.raw !== null ? row.raw : 0)),
    1,
  );

  const hasSupportingTables = hasUsageSnapshot || pendingCurrencyEntries.length > 0;

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Platform Overview")}
        actions={
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            disabled={Boolean(isFetching)}
            aria-busy={Boolean(isFetching)}
            className="gap-1.5"
          >
            {isFetching ? (
              <Spinner className="size-3.5" />
            ) : (
              <RefreshCw className="size-3.5" aria-hidden="true" />
            )}
            <span>{t("Refresh")}</span>
          </Button>
        }
      />

      {/* Band 1: Headline KPI tiles */}
      <Section variant="transparent" padding={0} aria-label={t("Workspace metrics")}>
        <div className="flex flex-col gap-4">
        <div
          className={
            data.pending_orders !== undefined
              ? "grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4"
              : "grid grid-cols-1 gap-4 sm:grid-cols-3"
          }
        >
          <Card padding={4}>
            <div className="flex items-center justify-between gap-2">
              <Text type="label" color="secondary">
                {t("Total Workspaces")}
              </Text>
              <Building2 className="size-4 text-faint-foreground" aria-hidden="true" />
            </div>
            <div className="mt-2 space-y-1">
              <Text type="display-3" weight="semibold" hasTabularNumbers as="div">
                {totalWorkspaces}
              </Text>
              <Text type="supporting" hasTabularNumbers as="p">
                {formatShare(activeCount, totalWorkspaces)} {t("active across platform")}
              </Text>
            </div>
          </Card>

          <Card padding={4}>
            <div className="flex items-center justify-between gap-2">
              <Text type="label" color="secondary">
                {t("Active Workspaces")}
              </Text>
              <Activity className="size-4 text-success" aria-hidden="true" />
            </div>
            <div className="mt-2 space-y-1">
              <Text
                type="display-3"
                weight="semibold"
                color="inherit"
                hasTabularNumbers
                as="div"
                className="text-success"
              >
                {activeCount}
              </Text>
              <div className="inline-flex items-center gap-1.5">
                <StatusDot variant="success" label={t("Active")} aria-hidden="true" />
                <Text type="supporting">{t("Operating normally")}</Text>
              </div>
            </div>
          </Card>

          <Card padding={4}>
            <div className="flex items-center justify-between gap-2">
              <Text type="label" color="secondary">
                {t("Sign-ups (7 Days)")}
              </Text>
              <Users className="size-4 text-brand" aria-hidden="true" />
            </div>
            <div className="mt-2 space-y-1">
              <Text type="display-3" weight="semibold" hasTabularNumbers as="div">
                {data.signups_7d}
              </Text>
              <Text type="supporting" as="p">
                {t("New registrations")}
              </Text>
            </div>
          </Card>

          {data.pending_orders !== undefined && (
            <Card padding={4}>
              <div className="flex items-center justify-between gap-2">
                <Text type="label" color="secondary">
                  {t("Pending Orders")}
                </Text>
                <CreditCard className="size-4 text-warning" aria-hidden="true" />
              </div>
              <div className="mt-2 space-y-1">
                <Text type="display-3" weight="semibold" hasTabularNumbers as="div">
                  {data.pending_orders}
                </Text>
                <Text type="supporting" hasTabularNumbers as="p">
                  {pendingCurrencyEntries.length > 0
                    ? t("{0} {1} awaiting settlement", pendingCurrencyEntries.length, pendingCurrencyEntries.length === 1 ? "currency" : "currencies")
                    : t("Awaiting settlement")}
                </Text>
              </div>
            </Card>
          )}
        </div>

        {/* Operational exception counters */}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <Card padding={4}>
            <div className="flex items-center justify-between gap-2">
              <div className="inline-flex items-center gap-1.5">
                <StatusDot variant="neutral" label={t("Read-Only")} aria-hidden="true" />
                <Text type="label" color="secondary">
                  {t("Read-Only")}
                </Text>
              </div>
              <Shield className="size-4 text-faint-foreground" aria-hidden="true" />
            </div>
            <div className="mt-2">
              <Text type="large" size="xl" weight="semibold" hasTabularNumbers as="div">
                {readOnlyCount}
              </Text>
            </div>
          </Card>

          <Card padding={4}>
            <div className="flex items-center justify-between gap-2">
              <div className="inline-flex items-center gap-1.5">
                <StatusDot
                  variant={suspendedCount > 0 ? "error" : "neutral"}
                  label={t("Suspended")}
                  aria-hidden="true"
                />
                <Text type="label" color="secondary">
                  {t("Suspended")}
                </Text>
              </div>
              <AlertTriangle
                className={suspendedCount > 0 ? "size-4 text-destructive" : "size-4 text-faint-foreground"}
                aria-hidden="true"
              />
            </div>
            <div className="mt-2">
              <Text
                type="large"
                size="xl"
                weight="semibold"
                color={suspendedCount > 0 ? "inherit" : "primary"}
                hasTabularNumbers
                as="div"
                className={suspendedCount > 0 ? "text-destructive" : undefined}
              >
                {suspendedCount}
              </Text>
            </div>
          </Card>

          <Card padding={4}>
            <div className="flex items-center justify-between gap-2">
              <div className="inline-flex items-center gap-1.5">
                <StatusDot
                  variant={pendingDeletionCount > 0 ? "error" : "neutral"}
                  label={t("Pending Deletion")}
                  aria-hidden="true"
                />
                <Text type="label" color="secondary">
                  {t("Pending Deletion")}
                </Text>
              </div>
              <AlertTriangle
                className={pendingDeletionCount > 0 ? "size-4 text-destructive" : "size-4 text-faint-foreground"}
                aria-hidden="true"
              />
            </div>
            <div className="mt-2">
              <Text
                type="large"
                size="xl"
                weight="semibold"
                color={pendingDeletionCount > 0 ? "inherit" : "primary"}
                hasTabularNumbers
                as="div"
                className={pendingDeletionCount > 0 ? "text-destructive" : undefined}
              >
                {pendingDeletionCount}
              </Text>
            </div>
          </Card>
        </div>
        </div>
      </Section>

      <Divider />

      {/* Band 2: Charts band using @astryxdesign/charts */}
      <Section variant="transparent" padding={0} aria-label={t("Distribution charts")} className="space-y-4">
        <div
          className={
            planEntries.length > 0
              ? "grid grid-cols-1 gap-4 lg:grid-cols-2"
              : "grid grid-cols-1 gap-4"
          }
        >
          <Card padding={4} className="space-y-4">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <Heading level={2}>{t("Workspace Status Distribution")}</Heading>
              <Text type="supporting" hasTabularNumbers>
                {totalWorkspaces.toLocaleString()} {t("workspaces")}
              </Text>
            </div>
            <Chart
              data={statusData as unknown as Record<string, unknown>[]}
              xKey="status"
              series={[
                bar("count", {
                  label: t("Workspaces"),
                  radius: 4,
                  color: (datum) => String(datum.color ?? "var(--data-color-1)"),
                }),
              ]}
              height={200}
              grid={<ChartGrid horizontal />}
              axes={
                <>
                  <ChartAxis position="bottom" />
                  <ChartAxis position="left" tickCount={4} tickFormat={integerTick} />
                </>
              }
              tooltip
              title={t("Workspace status distribution ({0} workspaces)", totalWorkspaces.toLocaleString())}
            />
            <div className="flex flex-wrap gap-x-5 gap-y-2">
              {statusData.map((segment) => (
                <div key={segment.key} className="inline-flex items-center gap-1.5">
                  <ChartSwatch variant="square" color={segment.color} />
                  <Text type="supporting" color="primary" hasTabularNumbers>
                    {segment.status} · {segment.count.toLocaleString()} (
                    {formatShare(segment.count, totalWorkspaces)})
                  </Text>
                </div>
              ))}
            </div>
          </Card>

          {planData.length > 0 && (
            <Card padding={4} className="space-y-4">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <Heading level={2}>{t("Subscriptions by Plan")}</Heading>
                <Text type="supporting" hasTabularNumbers>
                  {totalPlanSubscriptions.toLocaleString()} {t("total")}
                </Text>
              </div>
              <Chart
                data={planData as unknown as Record<string, unknown>[]}
                xKey="plan"
                series={[
                  bar("count", {
                    label: t("Subscriptions"),
                    radius: 4,
                    color: (datum) => String(datum.color ?? "var(--data-color-1)"),
                  }),
                ]}
                height={200}
                grid={<ChartGrid horizontal />}
                axes={
                  <>
                    <ChartAxis position="bottom" />
                    <ChartAxis position="left" tickCount={4} tickFormat={integerTick} />
                  </>
                }
                tooltip
                title={t("Subscriptions by plan ({0} total)", totalPlanSubscriptions.toLocaleString())}
              />
              <div className="flex flex-wrap gap-x-5 gap-y-2">
                {planData.map((segment) => (
                  <div key={segment.plan} className="inline-flex items-center gap-1.5">
                    <ChartSwatch variant="square" color={segment.color} />
                    <Text type="code" color="primary">
                      {segment.plan}
                    </Text>
                    <Text type="supporting" hasTabularNumbers>
                      {segment.count.toLocaleString()} ·{" "}
                      {formatShare(segment.count, totalPlanSubscriptions)}
                    </Text>
                  </div>
                ))}
              </div>
            </Card>
          )}
        </div>
      </Section>

      {/* Band 3: Supporting tables band */}
      {hasSupportingTables && (
        <>
          <Divider />
          <Section variant="transparent" padding={0} aria-label={t("Supporting tables")} className="space-y-6">
            <div
              className={
                hasUsageSnapshot && pendingCurrencyEntries.length > 0
                  ? "grid grid-cols-1 gap-6 lg:grid-cols-2"
                  : "grid grid-cols-1 gap-6"
              }
            >
              {hasUsageSnapshot && (
                <div className="space-y-3">
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <Heading level={2}>{t("Platform Usage Snapshot")}</Heading>
                    {data.usage_day && (
                      <Text type="code" color="secondary">
                        {data.usage_day}
                      </Text>
                    )}
                  </div>
                  <Card padding={0} className="overflow-x-auto">
                    <Table density="compact" dividers="rows" hasHover>
                      <TableHeader>
                        <TableRow isHeaderRow>
                          <TableHeaderCell>{t("Metric")}</TableHeaderCell>
                          <TableHeaderCell className="w-40">{t("Scale")}</TableHeaderCell>
                          <TableHeaderCell className="text-right">{t("Value")}</TableHeaderCell>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {usageRows.map((row) => (
                          <TableRow key={t(row.label)}>
                            <TableCell>
                              <Text type="body" weight="medium">
                                {t(row.label)}
                              </Text>
                            </TableCell>
                            <TableCell>
                              {row.raw !== null ? (
                                <ProgressBar
                                  value={row.raw}
                                  max={maxUsageCount}
                                  label={t("{0} relative volume", t(row.label))}
                                  isLabelHidden
                                />
                              ) : (
                                <Text type="supporting">—</Text>
                              )}
                            </TableCell>
                            <TableCell className="text-right">
                              <Text type="body" weight="medium" hasTabularNumbers>
                                {row.formatted}
                              </Text>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </Card>
                </div>
              )}

              {pendingCurrencyEntries.length > 0 && (
                <div className="space-y-3">
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <Heading level={2}>{t("Pending Orders by Currency")}</Heading>
                    <Text type="supporting" hasTabularNumbers>
                      {pendingCurrencyEntries.length}{" "}
                      {pendingCurrencyEntries.length === 1 ? "currency" : "currencies"}
                    </Text>
                  </div>
                  <Card padding={0} className="overflow-x-auto">
                    <Table density="compact" dividers="rows" hasHover>
                      <TableHeader>
                        <TableRow isHeaderRow>
                          <TableHeaderCell>{t("Currency")}</TableHeaderCell>
                          <TableHeaderCell className="text-right">{t("Orders")}</TableHeaderCell>
                          <TableHeaderCell className="text-right">{t("Net")}</TableHeaderCell>
                          <TableHeaderCell className="text-right">{t("VAT")}</TableHeaderCell>
                          <TableHeaderCell className="text-right">{t("Total (incl. VAT)")}</TableHeaderCell>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {pendingCurrencyEntries.map(([currency, summary]) => (
                          <TableRow key={currency}>
                            <TableCell>
                              <Text type="code" weight="medium" className="uppercase">
                                {currency}
                              </Text>
                            </TableCell>
                            <TableCell className="text-right">
                              <Text type="body" hasTabularNumbers>
                                {summary.orders}
                              </Text>
                            </TableCell>
                            <TableCell className="text-right">
                              <Text type="body" color="secondary" hasTabularNumbers>
                                {formatMinorAmount(summary.amount, currency)}
                              </Text>
                            </TableCell>
                            <TableCell className="text-right">
                              <Text type="body" color="secondary" hasTabularNumbers>
                                {formatMinorAmount(summary.vat_amount, currency)}
                              </Text>
                            </TableCell>
                            <TableCell className="text-right">
                              <Text type="body" weight="medium" hasTabularNumbers>
                                {formatMinorAmount(
                                  summary.amount + summary.vat_amount,
                                  currency,
                                )}
                              </Text>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </Card>
                </div>
              )}
            </div>
          </Section>
        </>
      )}
    </div>
  );
}
