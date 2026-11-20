import { Card } from "@astryxdesign/core/Card";
import { useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
import { HStack } from "@astryxdesign/core/HStack";
import { Heading } from "@astryxdesign/core/Heading";
import { Layout, LayoutContent, LayoutHeader } from "@astryxdesign/core/Layout";
import { Section } from "@astryxdesign/core/Section";
import { StatusDot } from "@astryxdesign/core/StatusDot";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@astryxdesign/core/Button";
import { Selector } from "@astryxdesign/core/Selector";
import { TextInput } from "@astryxdesign/core/TextInput";
import { RefreshCw, Search, Server } from "@multica/ui/icons";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { RuntimeAccountMenu } from "./RuntimeAccountMenu";
import { useMe, useRuntimes, useTenants, type Runtime, type RuntimeStatus } from "../api/hooks";
import type { StaffRole } from "../components/types";
import { t } from "../i18n";

interface RuntimesProps {
  onSelectRuntime: (id: string) => void;
  userRole?: StaffRole;
}

const STATUS_OPTIONS: Array<{ value: string; label: string }> = [
  { value: "all", label: "All statuses" },
  { value: "online", label: "Online" },
  { value: "offline", label: "Offline" },
];

const DISABLED_OPTIONS: Array<{ value: string; label: string }> = [
  { value: "all", label: "All access states" },
  { value: "true", label: "Disabled only" },
];

export function RuntimeStatusCell({
  status,
  disabled }: {
  status: RuntimeStatus;
  disabled: boolean;
}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {status === "online" ? (
        <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
          <StatusDot variant="success" label={t("Online")} aria-hidden="true" />
          <Text type="body" color="primary">
            {t("Online")}
          </Text>
        </span>
      ) : status === "offline" ? (
        <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
          <StatusDot variant="neutral" label={t("Offline")} aria-hidden="true" />
          <Text type="body" color="secondary">
            {t("Offline")}
          </Text>
        </span>
      ) : (
        <Badge variant="neutral" label={t("Unknown")} />
      )}
      {disabled && (
        <Badge
          variant="error"
          label={t("Disabled")}
          icon={<StatusDot variant="error" label={t("Disabled")} aria-hidden="true" />}
        />
      )}
    </div>
  );
}

export function Runtimes({ onSelectRuntime, userRole }: RuntimesProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;
  const canRead =
    effectiveRole === "system_admin" ||
    effectiveRole === "admin" ||
    effectiveRole === "support";
  const isTenantAdmin = effectiveRole === "admin";

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [disabledFilter, setDisabledFilter] = useState(false);
  const [tenantFilter, setTenantFilter] = useState("");

  const { data: tenantsData } = useTenants({});
  const tenants = Array.isArray(tenantsData) ? tenantsData : [];

  const {
    data: runtimes = [],
    isLoading,
    isFetching,
    isError,
    error,
    refetch,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage } = useRuntimes(
    {
      q: search.trim() || undefined,
      status: statusFilter || undefined,
      disabled: disabledFilter || undefined,
      tenant_id: !isTenantAdmin && tenantFilter ? tenantFilter : undefined },
    canRead,
  );

  if (effectiveRole && !canRead) {
    return (
      <div role="region" aria-label={t("Not found")}>
        <ErrorState
          title={t("Not found")}
          message={t("The page you requested was not found or you do not have permission to view it.")}
        />
      </div>
    );
  }

  const tenantOptions = [
    { value: "all", label: t("All tenants") },
    ...tenants.map((t) => ({ value: t.id, label: t.name || t.id })),
  ];

  const hasActiveFilters = Boolean(
    search.trim() || statusFilter || disabledFilter || (!isTenantAdmin && tenantFilter),
  );

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("");
    setDisabledFilter(false);
    setTenantFilter("");
  };

  return (
    <Layout
      header={
        <LayoutHeader padding={0}>
          <PageHeader
            title={t("Runtimes")}
            meta={
              !isLoading && !isError
                ? t("{0} shown", runtimes.length.toLocaleString())
                : undefined
            }
            actions={
              <Button
                type="button"
                label={t("Refresh")}
                variant="secondary"
                size="sm"
                icon={<RefreshCw className="size-3.5" aria-hidden="true" />}
                isLoading={isFetching}
                onClick={() => void refetch()}
                aria-busy={Boolean(isFetching) || undefined}
                isDisabled={Boolean(isFetching)}
              />
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
    label={t("Search runtimes")}
    isLabelHidden
    width="100%"
    startIcon={<Search className="size-4" aria-hidden="true" />}
    hasClear
    placeholder={t("Search by runtime name...")}
    value={search}
    onChange={(value) => setSearch(value)}
  />
</div>

                <Selector
                  label={t("Filter by status")}
                  isLabelHidden
                  width={160}
                  options={STATUS_OPTIONS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                  value={statusFilter || "all"}
                  onChange={(value) =>
                    setStatusFilter(!value || value === "all" ? "" : value)}
                />

                <Selector
                  label={t("Filter by access state")}
                  isLabelHidden
                  width={176}
                  options={DISABLED_OPTIONS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                  value={disabledFilter ? "true" : "all"}
                  onChange={(value) => setDisabledFilter(value === "true")}
                />

                {!isTenantAdmin && (
                  <Selector
                    label={t("Filter by tenant")}
                    isLabelHidden
                    width={192}
                    options={tenantOptions.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                    value={tenantFilter || "all"}
                    onChange={(value) =>
                      setTenantFilter(!value || value === "all" ? "" : value)}
                  />
                )}

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

              {isLoading && <LoadingState message={t("Loading runtimes...")} />}

              {isError && (
                <ErrorState
                  title={t("Failed to load runtimes")}
                  message={error instanceof Error ? error.message : undefined}
                  onRetry={refetch}
                />
              )}

              {!isLoading && !isError && runtimes.length === 0 && (
                <EmptyState
                  icon={<Server className="size-5" aria-hidden="true" />}
                  title={t("No runtimes found")}
                  description={
                    hasActiveFilters
                      ? t("No runtimes matched your current filters.")
                      : t("There are no registered runtimes available for your scope.")
                  }
                  actions={
                    hasActiveFilters ? (
                      <Button
                        label={t("Clear filters")}
                        variant="secondary"
                        size="sm"
                        onClick={clearFilters}
                      />
                    ) : undefined
                  }
                />
              )}

              {!isLoading && !isError && runtimes.length > 0 && (
                <div className="space-y-4">
                  <Card padding={0} className="overflow-x-auto">
                    <Table density="compact" dividers="rows" hasHover>
                      <TableHeader>
                        <TableRow isHeaderRow>
                          <TableHeaderCell>{t("Runtime")}</TableHeaderCell>
                          <TableHeaderCell>{t("Status")}</TableHeaderCell>
                          <TableHeaderCell>{t("OS")}</TableHeaderCell>
                          <TableHeaderCell>{t("Versions")}</TableHeaderCell>
                          <TableHeaderCell className="text-right">{t("Running tasks")}</TableHeaderCell>
                          <TableHeaderCell>{t("Last seen")}</TableHeaderCell>
                          <TableHeaderCell className="text-right">{t("Action")}</TableHeaderCell>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {runtimes.map((rt: Runtime) => (
                          <TableRow key={rt.id} onClick={() => onSelectRuntime(rt.id)}>
                            <TableCell>
                              <div
                                className="max-w-52 truncate font-medium text-foreground"
                                title={rt.name || rt.id}
                              >
                                {rt.name || rt.id}
                              </div>
                              <div
                                className="max-w-44 truncate font-mono text-caption text-muted-foreground"
                                title={rt.id}
                              >
                                {rt.id}
                              </div>
                            </TableCell>
                            <TableCell>
                              <RuntimeStatusCell status={rt.status} disabled={rt.disabled} />
                            </TableCell>
                            <TableCell>
                              <Text type="code">{rt.os || "-"}</Text>
                            </TableCell>
                            <TableCell>
                              <div className="space-y-0.5">
                                <div className="font-mono text-caption text-foreground">
                                  {t("Daemon:")} {rt.daemon_version || "-"}
                                </div>
                                <div className="font-mono text-caption text-muted-foreground">
                                  {t("Agent:")} {rt.agent_version || "-"}
                                </div>
                              </div>
                            </TableCell>
                            <TableCell className="text-right">
                              <Text type="body" hasTabularNumbers>
                                {rt.running_tasks.toLocaleString()}
                              </Text>
                            </TableCell>
                            <TableCell>
                              <Text type="supporting" hasTabularNumbers>
                                {rt.last_seen_at
                                  ? new Date(rt.last_seen_at).toLocaleString()
                                  : "-"}
                              </Text>
                            </TableCell>
                            <TableCell className="text-right" onClick={(event) => event.stopPropagation()}>
                              <RuntimeAccountMenu
                                runtimeId={rt.id}
                                runtimeName={rt.name || rt.id}
                                disabled={rt.disabled}
                                canControlRuntime={effectiveRole === "system_admin"}
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
                        isLoading={isFetchingNextPage}
                        aria-busy={isFetchingNextPage || undefined}
                        onClick={() => void fetchNextPage()}
                        isDisabled={isFetchingNextPage}
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
