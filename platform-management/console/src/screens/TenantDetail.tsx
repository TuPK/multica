import { useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
import { Card } from "@astryxdesign/core/Card";
import { Heading } from "@astryxdesign/core/Heading";
import { ProgressBar } from "@astryxdesign/core/ProgressBar";
import { RadioList, RadioListItem } from "@astryxdesign/core/RadioList";
import { Section } from "@astryxdesign/core/Section";
import {
  pixel,
  proportional,
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
import { RowActions, useRowActionsWidth } from "../components/RowActions";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Field, FieldLabel } from "@multica/ui/components/ui/field";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  AlertCircle,
  Building2,
  Gauge,
  Info,
  Lock,
  SlidersHorizontal,
  Trash2,
} from "@multica/ui/icons";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { LoadingState } from "../components/LoadingState";
import { ErrorState } from "../components/ErrorState";
import { EmptyState } from "../components/EmptyState";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { TypeToConfirmDialog } from "../components/TypeToConfirmDialog";
import { getApiErrorMessage, getLimitOverrideErrorMessage } from "../api/client";
import {
  useDeleteTenantNow,
  useLegalHolds,
  useMe,
  useSetTenantLimitOverride,
  useSetTenantStatus,
  useTenant,
  useTenantResources,
} from "../api/hooks";
import { LIMIT_GATES, type ResourceUsageItem, type TenantWorkspace } from "../api/schemas";
import type { StaffRole } from "../components/types";
import {
  computeResourcePercent,
  formatResourceAmount,
  formatResourceLimit,
  resolveResourceState,
  ResourceStateBadge,
  resourceTypeLabel,
} from "./Resources";
import { formatBytes, maxWorkspacesLabel, statusSummaryText } from "./Tenants";
import { TenantPolicy } from "./TenantPolicy";
import { TenantUsers } from "./TenantUsers";
import { t } from "../i18n";

interface TenantDetailProps {
  tenantId: string;
  onBack: () => void;
  onSelectWorkspace: (id: string) => void;
  onSelectUser?: (userId: string) => void;
  userRole?: StaffRole;
}

/**
 * The usage cell text of a workspace. A workspace with no daily row has a null
 * (or missing) usage day and shows "-", not zeros.
 */
function usageSummary(workspace: TenantWorkspace): string {
  if (!workspace.usage_day) return "-";
  return [
    t("{0} members", workspace.usage_members ?? 0),
    t("{0} agents", workspace.usage_agents ?? 0),
    t("{0} tasks run", workspace.usage_tasks_run ?? 0),
    formatBytes(workspace.usage_storage_bytes ?? 0),
  ].join(" · ");
}

const LIMIT_GATE_OPTIONS: Array<{ value: string; label: string }> = LIMIT_GATES.map((gate) => ({
  value: gate,
  label: `${resourceTypeLabel(gate)} (${gate})`,
}));

export function TenantDetail({
  tenantId,
  onBack,
  onSelectWorkspace,
  onSelectUser,
  userRole,
}: TenantDetailProps) {
  const actionsWidth = useRowActionsWidth(96);
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;
  const canDeleteTenant = effectiveRole === "system_admin";
  const canManageTenantStatus = effectiveRole === "system_admin";
  const canViewPolicy =
    !effectiveRole ||
    effectiveRole === "system_admin" ||
    effectiveRole === "admin" ||
    effectiveRole === "support";
  const canSetLimitOverride =
    effectiveRole === "system_admin" || effectiveRole === "billing";

  const [activeTab, setActiveTab] = useState<"workspaces" | "users" | "resources" | "policy">("workspaces");

  const { data: tenant, isLoading, isError, error, refetch } = useTenant(tenantId);
  const {
    data: resourcesData,
    isLoading: resourcesLoading,
    isError: resourcesError,
    error: resourcesErrorObj,
    refetch: refetchResources,
  } = useTenantResources(tenantId, activeTab === "resources");
  const legalHoldsQuery = useLegalHolds(effectiveRole === "system_admin");
  const setLimitOverride = useSetTenantLimitOverride();
  const deleteTenantNow = useDeleteTenantNow();
  const setTenantStatus = useSetTenantStatus();

  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [deleteNotice, setDeleteNotice] = useState<string | null>(null);

  const [statusDialogMode, setStatusDialogMode] = useState<"lock" | "unlock" | null>(null);
  const [lockTargetStatus, setLockTargetStatus] = useState<"suspended" | "read_only">("suspended");
  const [statusReason, setStatusReason] = useState("");
  const [statusDialogError, setStatusDialogError] = useState<string | null>(null);
  const [statusNotice, setStatusNotice] = useState<string | null>(null);

  const [overrideDialogOpen, setOverrideDialogOpen] = useState(false);
  const [overrideGate, setOverrideGate] = useState<string>("agents");
  const [overrideMode, setOverrideMode] = useState<"set" | "release">("set");
  const [overrideValue, setOverrideValue] = useState("");
  const [overrideWorkspaceId, setOverrideWorkspaceId] = useState<string>("all");
  const [overrideReason, setOverrideReason] = useState("");
  const [overrideError, setOverrideError] = useState<string | null>(null);
  const [overrideNotice, setOverrideNotice] = useState<string | null>(null);
  const back = { label: t("Back to tenants"), onClick: onBack };

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader title={t("Tenant")} backAction={back} />
        <LoadingState message={t("Loading tenant...")} />
      </div>
    );
  }

  if (isError || !tenant) {
    return (
      <div className="space-y-6">
        <PageHeader title={t("Tenant")} backAction={back} />
        <ErrorState
          title={t("Failed to load tenant")}
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      </div>
    );
  }

  const handleConfirmDeleteTenant = async ({
    reason,
    confirm,
    export_first,
  }: {
    reason: string;
    confirm: string;
    export_first?: boolean;
  }) => {
    setDeleteError(null);
    try {
      const res = await deleteTenantNow.mutateAsync({
        tenantId,
        reason,
        confirm,
        ...(export_first !== undefined ? { export_first } : {}),
      });
      setDeleteDialogOpen(false);
      setDeleteNotice(t("Immediate deletion requested ({0}).", res.request_id));
    } catch (err) {
      setDeleteError(getApiErrorMessage(err, t("Failed to start tenant deletion")));
    }
  };

  const openStatusDialog = (mode: "lock" | "unlock") => {
    setStatusDialogMode(mode);
    setLockTargetStatus("suspended");
    setStatusReason("");
    setStatusDialogError(null);
  };

  const handleSubmitTenantStatus = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!statusDialogMode || setTenantStatus.isPending) return;
    const trimmedReason = statusReason.trim();
    if (!trimmedReason) {
      setStatusDialogError(t("A reason is required."));
      return;
    }

    const nextStatus = statusDialogMode === "unlock" ? "active" : lockTargetStatus;
    setStatusDialogError(null);
    try {
      const res = await setTenantStatus.mutateAsync({
        tenantId,
        status: nextStatus,
        reason: trimmedReason,
      });
      setStatusDialogMode(null);
      setStatusNotice(
        t("Tenant status updated to {0} across {1} workspace(s).", res.status, res.workspace_ids.length),
      );
    } catch (err) {
      setStatusDialogError(getApiErrorMessage(err, t("Failed to update tenant status")));
    }
  };

  const hasLegalHold =
    effectiveRole === "system_admin" &&
    (Boolean(tenant.legal_hold) ||
      (legalHoldsQuery.data ?? []).some(
        (h) => !h.released && h.scope_kind === "tenant" && h.scope_id === tenantId,
      ));

  const handleOpenOverrideDialog = () => {
    setOverrideGate("agents");
    setOverrideMode("set");
    setOverrideValue("");
    setOverrideWorkspaceId("all");
    setOverrideReason("");
    setOverrideError(null);
    setOverrideDialogOpen(true);
  };

  const handleSubmitOverride = async (e: React.FormEvent) => {
    e.preventDefault();
    if (setLimitOverride.isPending) return;
    const trimmedReason = overrideReason.trim();
    if (!trimmedReason) {
      setOverrideError(t("A reason is required."));
      return;
    }

    let nextValue: number | null = null;
    if (overrideMode === "set") {
      if (overrideValue.trim() === "") {
        setOverrideError(t("Enter a non-negative integer value."));
        return;
      }
      const parsed = Number(overrideValue);
      if (!Number.isFinite(parsed) || !Number.isInteger(parsed) || parsed < 0) {
        setOverrideError(t("Value must be a non-negative integer."));
        return;
      }
      nextValue = parsed;
    }

    setOverrideError(null);
    try {
      const res = await setLimitOverride.mutateAsync({
        tenantId,
        gate: overrideGate,
        value: nextValue,
        workspace_ids: overrideWorkspaceId !== "all" ? [overrideWorkspaceId] : undefined,
        reason: trimmedReason,
      });
      setOverrideDialogOpen(false);
      setOverrideNotice(
        nextValue === null
          ? t("Released {0} limit override for {1} workspace(s).", res.gate, res.workspace_ids.length)
          : t("Set {0} limit override to {1} for {2} workspace(s).", res.gate, nextValue.toLocaleString(), res.workspace_ids.length),
      );
    } catch (err) {
      setOverrideError(getLimitOverrideErrorMessage(err, t("Failed to set limit override")));
    }
  };

  const workspaceScopeOptions = [
    { value: "all", label: t("All workspaces") },
    ...tenant.workspaces.map((ws) => ({
      value: ws.id,
      label: ws.name || ws.slug || ws.id,
    })),
  ];

  return (
    <div className="space-y-6">
      <PageHeader
        title={tenant.name || tenant.id}
        badge={
          <div className="inline-flex items-center gap-2">
            <StatusBadge status={tenant.status} />
            {hasLegalHold && <Badge variant="error" label={t("Legal hold")} />}
          </div>
        }
        backAction={back}
        description={<span className="font-mono text-caption">{tenant.id}</span>}
        actions={
          canManageTenantStatus || canDeleteTenant ? (
            <div className="flex flex-wrap items-center gap-2">
              {canManageTenantStatus && tenant.status !== "suspended" && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => openStatusDialog("lock")}
                >
                  <Lock className="size-3.5" aria-hidden="true" />
                  <span>{t("Lock tenant")}</span>
                </Button>
              )}
              {canManageTenantStatus &&
                (tenant.status === "suspended" || tenant.status === "read_only") && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => openStatusDialog("unlock")}
                  >
                    <span>{t("Unlock tenant")}</span>
                  </Button>
                )}
              {canDeleteTenant && (
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={() => {
                    setDeleteError(null);
                    setDeleteDialogOpen(true);
                  }}
                >
                  <Trash2 className="size-3.5" aria-hidden="true" />
                  <span>{t("Delete tenant")}</span>
                </Button>
              )}
            </div>
          ) : undefined
        }
      />

      {statusNotice && (
        <Alert role="status">
          <Info className="size-4" aria-hidden="true" />
          <AlertDescription>{statusNotice}</AlertDescription>
        </Alert>
      )}

      {deleteNotice && (
        <Alert role="status">
          <Info className="size-4" aria-hidden="true" />
          <AlertDescription>{deleteNotice}</AlertDescription>
        </Alert>
      )}

      <Card padding={4}>
        <MetadataList columns={3}>
          <MetadataListItem label={t("Workspaces")} tabular>
            <span data-testid="tenant-workspace-count">{tenant.workspace_count}</span>
          </MetadataListItem>
          <MetadataListItem label={t("Max workspaces")} tabular>
            <span data-testid="tenant-max-workspaces">{maxWorkspacesLabel(tenant.max_workspaces)}</span>
          </MetadataListItem>
          <MetadataListItem label={t("Best plan")}>{tenant.best_plan_key || "-"}</MetadataListItem>
          <MetadataListItem label={t("Workspace status")}>
            <span data-testid="tenant-status-summary">{statusSummaryText(tenant.status_summary)}</span>
          </MetadataListItem>
          <MetadataListItem label={t("Last activity")} tabular>
            {tenant.last_activity_at ? new Date(tenant.last_activity_at).toLocaleString() : "-"}
          </MetadataListItem>
        </MetadataList>
      </Card>

      <Tabs
        value={activeTab}
        onValueChange={(value) => {
          if (
            value === "workspaces" ||
            value === "users" ||
            value === "resources" ||
            value === "policy"
          ) {
            setActiveTab(value);
          }
        }}
        className="space-y-4"
      >
        <div className="border-b border-surface-border">
          <TabsList variant="line" aria-label={t("Tenant sections")}>
            <TabsTrigger value="workspaces">{t("Workspaces")}</TabsTrigger>
            <TabsTrigger value="users">{t("Users")}</TabsTrigger>
            <TabsTrigger value="resources">{t("Resources")}</TabsTrigger>
            {canViewPolicy && <TabsTrigger value="policy">{t("Policy")}</TabsTrigger>}
          </TabsList>
        </div>

        <TabsContent value="workspaces">
          <Section variant="transparent" padding={0} className="space-y-4">
            <Heading level={3}>{t("Workspaces")}</Heading>
            {tenant.workspaces.length === 0 ? (
              <EmptyState
                icon={<Building2 className="size-5" aria-hidden="true" />}
                title={t("This tenant has no workspaces")}
              />
            ) : (
              <div className="overflow-x-auto">
                <Table
                  data={tenant.workspaces}
                  idKey="id"
                  density="balanced"
                  dividers="rows"
                  hasHover
                  columns={[
                    {
                      key: "name",
                      header: t("Workspace"),
                      width: proportional(2, { minWidth: 200 }),
                      renderCell: (workspace) => (
                        <>
                          <div className="truncate font-medium text-foreground" title={workspace.name}>
                            {workspace.name}
                          </div>
                          <div className="truncate font-mono text-caption text-muted-foreground" title={workspace.slug}>
                            {workspace.slug}
                          </div>
                        </>
                      ),
                    },
                    {
                      key: "plan_key",
                      header: t("Plan"),
                      width: proportional(1, { minWidth: 100 }),
                      renderCell: (workspace) => <Text type="supporting">{workspace.plan_key || "-"}</Text>,
                    },
                    {
                      key: "status",
                      header: t("Status"),
                      width: proportional(1, { minWidth: 110 }),
                      renderCell: (workspace) => <StatusBadge status={workspace.status} />,
                    },
                    {
                      key: "member_count",
                      header: t("Members"),
                      width: pixel(96),
                      align: "end",
                      renderCell: (workspace) => (
                        <Text type="body" hasTabularNumbers>
                          {workspace.member_count}
                        </Text>
                      ),
                    },
                    {
                      key: "owner",
                      header: t("Owner"),
                      width: proportional(1.5, { minWidth: 160 }),
                      renderCell: (workspace) => (
                        <Text type="supporting">{workspace.owner?.email ?? "-"}</Text>
                      ),
                    },
                    {
                      key: "usage",
                      header: t("Usage"),
                      width: proportional(1.5, { minWidth: 160 }),
                      renderCell: (workspace) => (
                        <span
                          data-testid="workspace-usage"
                          title={workspace.usage_day ? t("Usage on {0} (UTC)", workspace.usage_day) : t("No usage recorded yet")}
                        >
                          <Text type="supporting" hasTabularNumbers>
                            {usageSummary(workspace)}
                          </Text>
                        </span>
                      ),
                    },
                    {
                      key: "actions",
                      header: t("Action"),
                      width: pixel(actionsWidth),
                      align: "end",
                      renderCell: (workspace) => (
                        <RowActions
                          label={t("workspace {0}", workspace.name)}
                          actions={[{ label: t("View"), onSelect: () => onSelectWorkspace(workspace.id) }]}
                        />
                      ),
                    },
                  ]}
                />
              </div>
            )}
          </Section>
        </TabsContent>

        <TabsContent value="users">
          {activeTab === "users" && (
            <TenantUsers
              tenantId={tenantId}
              workspaces={tenant.workspaces}
              onSelectUser={onSelectUser ?? (() => {})}
              userRole={effectiveRole}
            />
          )}
        </TabsContent>

        <TabsContent value="resources">
          {activeTab === "resources" && (
            <Section variant="transparent" padding={0} className="space-y-6">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <Heading level={3}>{t("Resources")}</Heading>
                {canSetLimitOverride && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={handleOpenOverrideDialog}
                  >
                    <SlidersHorizontal className="size-3.5" aria-hidden="true" />
                    <span>{t("Set extra limit")}</span>
                  </Button>
                )}
              </div>

              {overrideNotice && (
                <Alert role="status">
                  <Info className="size-4" aria-hidden="true" />
                  <AlertDescription>{overrideNotice}</AlertDescription>
                </Alert>
              )}

              {resourcesLoading && <LoadingState message={t("Loading tenant resources...")} />}

              {resourcesError && (
                <ErrorState
                  title={t("Failed to load tenant resources")}
                  message={
                    resourcesErrorObj instanceof Error
                      ? resourcesErrorObj.message
                      : undefined
                  }
                  onRetry={refetchResources}
                />
              )}

              {!resourcesLoading && !resourcesError && resourcesData && (
                <div className="space-y-6">
                  {!resourcesData.complete && (
                    <Alert role="status">
                      <Info className="size-4" aria-hidden="true" />
                      <AlertDescription>
                        {t("Resource counters are partial across the tenant's workspaces.")}
                      </AlertDescription>
                    </Alert>
                  )}

                  {resourcesData.resources.length === 0 ? (
                    <EmptyState
                      icon={<Gauge className="size-5" aria-hidden="true" />}
                      title={t("No resource usage reported")}
                    />
                  ) : (
                    <div className="space-y-4">
                      {resourcesData.resources.map((item: ResourceUsageItem) => {
                        const pct = computeResourcePercent(
                          item.used,
                          item.limit,
                          item.percent,
                        );
                        const resolvedState = resolveResourceState(
                          item.state,
                          pct,
                          item.limit,
                        );
                        const barVariant =
                          resolvedState === "over"
                            ? "error"
                            : resolvedState === "warning"
                              ? "warning"
                              : "success";
                        const label = resourceTypeLabel(item.type, item.label);
                        const usedLabel = formatResourceAmount(item.type, item.used);
                        const limitLabel = formatResourceLimit(item.type, item.limit);

                        return (
                          <div
                            key={item.type || label}
                            data-testid={`resource-row-${item.type}`}
                            data-resource-state={resolvedState}
                            className="space-y-2 border-b border-surface-border pb-4 last:border-b-0 last:pb-0"
                          >
                            <div className="flex flex-wrap items-center justify-between gap-2">
                              <div className="flex flex-wrap items-center gap-2">
                                <Text type="body" weight="medium">
                                  {label}
                                </Text>
                                <ResourceStateBadge state={resolvedState} />
                              </div>
                              <Text type="body" hasTabularNumbers>
                                {usedLabel} / {limitLabel}
                                {pct !== null ? ` (${pct}%)` : ""}
                              </Text>
                            </div>
                            {item.limit !== null && item.limit > 0 && (
                              <ProgressBar
                                value={item.used}
                                max={item.limit}
                                variant={barVariant}
                                label={t("{0} usage", label)}
                                isLabelHidden
                              />
                            )}
                          </div>
                        );
                      })}
                    </div>
                  )}

                  {resourcesData.workspaces.length > 0 && (
                    <Section variant="transparent" padding={0} className="space-y-3">
                      <Heading level={3}>{t("By Workspace")}</Heading>
                      <div className="overflow-x-auto">
                        <Table density="compact" dividers="rows" hasHover>
                          <TableHeader>
                            <TableRow isHeaderRow>
                              <TableHeaderCell>{t("Workspace")}</TableHeaderCell>
                              <TableHeaderCell>{t("Plan")}</TableHeaderCell>
                              <TableHeaderCell>{t("Usage")}</TableHeaderCell>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {resourcesData.workspaces.map((ws) => (
                              <TableRow key={ws.workspace_id || ws.workspace_name}>
                                <TableCell>
                                  <Text type="body" weight="medium">
                                    {ws.workspace_name || ws.workspace_id || "-"}
                                  </Text>
                                </TableCell>
                                <TableCell>
                                  <Text type="code">{ws.plan_key || "-"}</Text>
                                </TableCell>
                                <TableCell>
                                  {ws.resources.length === 0 ? (
                                    <Text type="supporting">-</Text>
                                  ) : (
                                    <div className="flex flex-wrap gap-3">
                                      {ws.resources.map((res) => {
                                        const pct = computeResourcePercent(
                                          res.used,
                                          res.limit,
                                          res.percent,
                                        );
                                        const st = resolveResourceState(
                                          res.state,
                                          pct,
                                          res.limit,
                                        );
                                        return (
                                          <div
                                            key={res.type}
                                            className="inline-flex items-center gap-1.5"
                                          >
                                            <Text type="supporting" hasTabularNumbers>
                                              {resourceTypeLabel(res.type, res.label)}:{" "}
                                              {formatResourceAmount(res.type, res.used)} /{" "}
                                              {formatResourceLimit(res.type, res.limit)}
                                              {pct !== null ? ` (${pct}%)` : ""}
                                            </Text>
                                            {(st === "warning" || st === "over") && (
                                              <ResourceStateBadge state={st} />
                                            )}
                                          </div>
                                        );
                                      })}
                                    </div>
                                  )}
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </div>
                    </Section>
                  )}
                </div>
              )}
            </Section>
          )}
        </TabsContent>

        {canViewPolicy && (
          <TabsContent value="policy">
            <TenantPolicy tenantId={tenantId} userRole={effectiveRole} embedded />
          </TabsContent>
        )}
      </Tabs>

      {canSetLimitOverride && (
        <Dialog
          open={overrideDialogOpen}
          onOpenChange={(open) => {
            if (!setLimitOverride.isPending) {
              setOverrideDialogOpen(open);
              if (!open) setOverrideError(null);
            }
          }}
        >
          <DialogContent>
            <form onSubmit={handleSubmitOverride} className="flex flex-col gap-4">
              <DialogHeader>
                <DialogTitle>{t("Set extra limit")}</DialogTitle>
                <DialogDescription>
                  {t("Override one plan limit gate across workspaces of this tenant, or release an existing override.")}
                </DialogDescription>
              </DialogHeader>

              {overrideError && (
                <Alert variant="destructive">
                  <AlertCircle className="size-4" aria-hidden="true" />
                  <AlertDescription>{overrideError}</AlertDescription>
                </Alert>
              )}

              <Field>
                <FieldLabel htmlFor="override-gate-select" className="text-label">
                  {t("Limit gate")} <span className="text-destructive">*</span>
                </FieldLabel>
                <Select
                  items={LIMIT_GATE_OPTIONS}
                  value={overrideGate}
                  onValueChange={(val) => {
                    if (val) setOverrideGate(val);
                  }}
                  disabled={setLimitOverride.isPending}
                >
                  <SelectTrigger id="override-gate-select" aria-label={t("Limit gate")} className="w-full">
                    <SelectValue>
                      {t(LIMIT_GATE_OPTIONS.find((item) => item.value === overrideGate)?.label ??
                        overrideGate)}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {LIMIT_GATE_OPTIONS.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {t(item.label)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>

              <RadioList
                label={t("Override action")}
                value={overrideMode}
                onChange={(val) => setOverrideMode(val as "set" | "release")}
                isDisabled={setLimitOverride.isPending}
              >
                <RadioListItem value="set" label={t("Set custom limit")} />
                <RadioListItem value="release" label={t("Release limit (use plan default)")} />
              </RadioList>

              {overrideMode === "set" && (
                <Field>
                  <FieldLabel htmlFor="override-value-input" className="text-label">
                    {t("Extra limit value")} <span className="text-destructive">*</span>
                  </FieldLabel>
                  <Input
                    id="override-value-input"
                    type="number"
                    min={0}
                    step={1}
                    value={overrideValue}
                    onChange={(e) => setOverrideValue(e.target.value)}
                    placeholder={t("e.g. 25")}
                    disabled={setLimitOverride.isPending}
                  />
                </Field>
              )}

              <Field>
                <FieldLabel htmlFor="override-workspace-select" className="text-label">
                  {t("Target workspaces")}
                </FieldLabel>
                <Select
                  items={workspaceScopeOptions}
                  value={overrideWorkspaceId}
                  onValueChange={(val) => {
                    if (val) setOverrideWorkspaceId(val);
                  }}
                  disabled={setLimitOverride.isPending}
                >
                  <SelectTrigger
                    id="override-workspace-select"
                    aria-label={t("Target workspaces")}
                    className="w-full"
                  >
                    <SelectValue>
                      {t(workspaceScopeOptions.find((item) => item.value === overrideWorkspaceId)
                        ?.label ?? t("All workspaces"))}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {workspaceScopeOptions.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {t(item.label)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>

              <Field>
                <FieldLabel htmlFor="override-reason-input" className="text-label">
                  {t("Reason")} <span className="text-destructive">*</span>
                </FieldLabel>
                <Textarea
                  id="override-reason-input"
                  value={overrideReason}
                  onChange={(e) => setOverrideReason(e.target.value)}
                  placeholder={t("Explain why this override is being changed...")}
                  rows={3}
                  disabled={setLimitOverride.isPending}
                />
              </Field>

              <DialogFooter>
                <DialogClose
                  render={
                    <Button
                      type="button"
                      variant="outline"
                      disabled={setLimitOverride.isPending}
                    />
                  }
                >
                  {t("Cancel")}
                </DialogClose>
                <Button
                  type="submit"
                  variant="default"
                  disabled={setLimitOverride.isPending}
                  aria-busy={setLimitOverride.isPending || undefined}
                >
                  {setLimitOverride.isPending && <Spinner className="size-3.5" />}
                  {t("Save limit")}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      )}

      {canManageTenantStatus && (
        <Dialog
          open={statusDialogMode !== null}
          onOpenChange={(open) => {
            if (!open && !setTenantStatus.isPending) {
              setStatusDialogMode(null);
              setStatusDialogError(null);
            }
          }}
        >
          <DialogContent className="sm:max-w-md">
            <form onSubmit={handleSubmitTenantStatus} className="space-y-4">
              <DialogHeader>
                <DialogTitle>
                  {statusDialogMode === "unlock" ? t("Unlock tenant") : t("Lock tenant")}
                </DialogTitle>
                <DialogDescription>
                  {statusDialogMode === "unlock"
                    ? t("Restore all locked workspaces in this tenant back to active status.")
                    : t("Cascade a status lock across every active workspace belonging to this tenant.")}
                </DialogDescription>
              </DialogHeader>

              {statusDialogError && (
                <Alert variant="destructive">
                  <AlertCircle className="size-4" aria-hidden="true" />
                  <AlertDescription>{statusDialogError}</AlertDescription>
                </Alert>
              )}

              {statusDialogMode === "lock" && (
                <Field>
                  <FieldLabel htmlFor="tenant-lock-status">{t("Target status")}</FieldLabel>
                  <Select
                    items={[
                      { value: "suspended", label: t("Suspended") },
                      { value: "read_only", label: t("Read-only") },
                    ]}
                    value={lockTargetStatus}
                    onValueChange={(val) => {
                      if (val === "suspended" || val === "read_only") {
                        setLockTargetStatus(val);
                      }
                    }}
                  >
                    <SelectTrigger
                      id="tenant-lock-status"
                      aria-label={t("Target status")}
                      className="w-full"
                    >
                      <SelectValue>
                        {lockTargetStatus === "read_only" ? t("Read-only") : t("Suspended")}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="suspended">{t("Suspended")}</SelectItem>
                      <SelectItem value="read_only">{t("Read-only")}</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
              )}

              <Field>
                <FieldLabel htmlFor="tenant-status-reason">{t("Reason")}</FieldLabel>
                <Textarea
                  id="tenant-status-reason"
                  value={statusReason}
                  onChange={(e) => setStatusReason(e.target.value)}
                  placeholder={t("Explain why this tenant is being updated...")}
                  disabled={setTenantStatus.isPending}
                  rows={3}
                />
              </Field>

              <DialogFooter>
                <DialogClose
                  render={
                    <Button
                      type="button"
                      variant="outline"
                      disabled={setTenantStatus.isPending}
                    />
                  }
                >
                  {t("Cancel")}
                </DialogClose>
                <Button
                  type="submit"
                  variant={statusDialogMode === "unlock" ? "default" : "destructive"}
                  disabled={!statusReason.trim() || setTenantStatus.isPending}
                  aria-busy={setTenantStatus.isPending || undefined}
                >
                  {setTenantStatus.isPending && <Spinner className="size-3.5" />}
                  {statusDialogMode === "unlock" ? t("Unlock tenant") : t("Apply lock")}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      )}

      {canDeleteTenant && (
        <TypeToConfirmDialog
          open={deleteDialogOpen}
          onOpenChange={(open) => {
            setDeleteDialogOpen(open);
            if (!open) setDeleteError(null);
          }}
          title={t("Delete tenant")}
          description={t("Immediate deletion permanently removes this tenant and all of its workspaces.")}
          expectedText={tenant.name || tenant.id}
          confirmInputLabel={t("Tenant name")}
          confirmLabel={t("Delete tenant")}
          confirmVariant="destructive"
          showExportFirst
          error={deleteError}
          onConfirm={handleConfirmDeleteTenant}
        />
      )}
    </div>
  );
}
