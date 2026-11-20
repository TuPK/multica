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
import { RefreshCw, Search, Server, X } from "@multica/ui/icons";
import { RowActions } from "../components/RowActions";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
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
  disabled,
}: {
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
    isFetchingNextPage,
  } = useRuntimes(
    {
      q: search.trim() || undefined,
      status: statusFilter || undefined,
      disabled: disabledFilter || undefined,
      tenant_id: !isTenantAdmin && tenantFilter ? tenantFilter : undefined,
    },
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
                variant="outline"
                size="sm"
                onClick={() => void refetch()}
                disabled={Boolean(isFetching)}
                aria-busy={Boolean(isFetching) || undefined}
              >
                {isFetching ? <Spinner className="size-3.5" /> : <RefreshCw className="size-3.5" aria-hidden="true" />}
                <span>{t("Refresh")}</span>
              </Button>
            }
          />
        </LayoutHeader>
      }
      content={
        <LayoutContent padding={0}>
          <div className="space-y-6">
            <Section className="space-y-4">
              <HStack wrap="wrap" gap={2} align="center">
                <div className="relative w-full max-w-sm">
                  <Search
                    aria-hidden="true"
                    className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
                  />
                  <Input
                    type="search"
                    aria-label={t("Search runtimes")}
                    placeholder={t("Search by runtime name...")}
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    className="pr-8 pl-8"
                  />
                  {search && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-xs"
                      aria-label={t("Clear search")}
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
                    setStatusFilter(!value || value === "all" ? "" : value)
                  }
                >
                  <SelectTrigger aria-label={t("Filter by status")} className="w-40">
                    <SelectValue placeholder={t("All statuses")} />
                  </SelectTrigger>
                  <SelectContent>
                    {STATUS_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {t(opt.label)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>

                <Select
                  items={DISABLED_OPTIONS}
                  value={disabledFilter ? "true" : "all"}
                  onValueChange={(value) => setDisabledFilter(value === "true")}
                >
                  <SelectTrigger aria-label={t("Filter by access state")} className="w-44">
                    <SelectValue placeholder={t("All access states")} />
                  </SelectTrigger>
                  <SelectContent>
                    {DISABLED_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {t(opt.label)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>

                {!isTenantAdmin && (
                  <Select
                    items={tenantOptions}
                    value={tenantFilter || "all"}
                    onValueChange={(value) =>
                      setTenantFilter(!value || value === "all" ? "" : value)
                    }
                  >
                    <SelectTrigger aria-label={t("Filter by tenant")} className="w-48">
                      <SelectValue placeholder={t("All tenants")} />
                    </SelectTrigger>
                    <SelectContent>
                      {tenantOptions.map((opt) => (
                        <SelectItem key={opt.value} value={opt.value}>
                          {t(opt.label)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}

                {hasActiveFilters && (
                  <Button type="button" variant="ghost" size="sm" onClick={clearFilters}>
                    {t("Clear filters")}
                  </Button>
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
                      <Button variant="outline" size="sm" onClick={clearFilters}>
                        {t("Clear filters")}
                      </Button>
                    ) : undefined
                  }
                />
              )}

              {!isLoading && !isError && runtimes.length > 0 && (
                <div className="space-y-4">
                  <div className="overflow-x-auto">
                    <Table density="balanced" dividers="rows" hasHover>
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
                          <TableRow key={rt.id}>
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
                            <TableCell className="text-right">
                              <RowActions label={t("runtime")} actions={[{ label: t("View"), onSelect: () => onSelectRuntime(rt.id) }]} />
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
                        {isFetchingNextPage ? t("Loading...") : t("Load more")}
                      </Button>
                    </div>
                  )}
                </div>
              )}
            </Section>
          </div>
        </LayoutContent>
      }
    />
  );
}
