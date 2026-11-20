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
  TableRow } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@astryxdesign/core/Button";
import { RowActions, useRowActionsWidth } from "../components/RowActions";
import { Banner } from "@astryxdesign/core/Banner";
import { TextArea } from "@astryxdesign/core/TextArea";
import { Selector } from "@astryxdesign/core/Selector";
import { Dialog, DialogHeader } from "@astryxdesign/core/Dialog";
import { NumberInput } from "@astryxdesign/core/NumberInput";
import { TabList, Tab } from "@astryxdesign/core/TabList";
import {
  Building2,
  Gauge,
  Lock,
  SlidersHorizontal,
  Trash2 } from "@multica/ui/icons";
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
  useTenantResources } from "../api/hooks";
import { LIMIT_GATES, type ResourceUsageItem, type TenantWorkspace } from "../api/schemas";
import type { StaffRole } from "../components/types";
import {
  computeResourcePercent,
  formatResourceAmount,
  formatResourceLimit,
  resolveResourceState,
  ResourceStateBadge,
  resourceTypeLabel } from "./Resources";
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
  label: `${resourceTypeLabel(gate)} (${gate})` }));

export function TenantDetail({
  tenantId,
  onBack,
  onSelectWorkspace,
  onSelectUser,
  userRole }: TenantDetailProps) {
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
    refetch: refetchResources } = useTenantResources(tenantId, activeTab === "resources");
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
    export_first }: {
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
        ...(export_first !== undefined ? { export_first } : {}) });
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
        reason: trimmedReason });
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
        reason: trimmedReason });
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
      label: ws.name || ws.slug || ws.id })),
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
                  label={t("Lock tenant")}
                  variant="secondary"
                  size="sm"
                  icon={<Lock className="size-3.5" aria-hidden="true" />}
                  onClick={() => openStatusDialog("lock")}
                />
              )}
              {canManageTenantStatus &&
                (tenant.status === "suspended" || tenant.status === "read_only") && (
                  <Button
                    type="button"
                    label={t("Unlock tenant")}
                    variant="secondary"
                    size="sm"
                    onClick={() => openStatusDialog("unlock")}
                  />
                )}
              {canDeleteTenant && (
                <Button
                  label={t("Delete tenant")}
                  variant="destructive"
                  size="sm"
                  icon={<Trash2 className="size-3.5" aria-hidden="true" />}
                  onClick={() => {
                    setDeleteError(null);
                    setDeleteDialogOpen(true);
                  }}
                />
              )}
            </div>
          ) : undefined
        }
      />

      {statusNotice && (
        <Banner
          status="info"
          role="status"
          title={statusNotice}
        />
      )}

      {deleteNotice && (
        <Banner
          status="info"
          role="status"
          title={deleteNotice}
        />
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

      <div className="space-y-4">
        <div className="border-b border-surface-border">
          <TabList
            role="tablist"
            value={activeTab}
            onChange={(value) => {
              if (
                value === "workspaces" ||
                value === "users" ||
                value === "resources" ||
                value === "policy"
              ) {
                setActiveTab(value);
              }
            }}
            aria-label={t("Tenant sections")}
          >
            <Tab value="workspaces" label={t("Workspaces")} />
            <Tab value="users" label={t("Users")} />
            <Tab value="resources" label={t("Resources")} />
            {canViewPolicy && <Tab value="policy" label={t("Policy")} />}
          </TabList>
        </div>

        {activeTab === "workspaces" && (
<>
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
                      ) },
                    {
                      key: "plan_key",
                      header: t("Plan"),
                      width: proportional(1, { minWidth: 100 }),
                      renderCell: (workspace) => <Text type="supporting">{workspace.plan_key || "-"}</Text> },
                    {
                      key: "status",
                      header: t("Status"),
                      width: proportional(1, { minWidth: 110 }),
                      renderCell: (workspace) => <StatusBadge status={workspace.status} /> },
                    {
                      key: "member_count",
                      header: t("Members"),
                      width: pixel(96),
                      align: "end",
                      renderCell: (workspace) => (
                        <Text type="body" hasTabularNumbers>
                          {workspace.member_count}
                        </Text>
                      ) },
                    {
                      key: "owner",
                      header: t("Owner"),
                      width: proportional(1.5, { minWidth: 160 }),
                      renderCell: (workspace) => (
                        <Text type="supporting">{workspace.owner?.email ?? "-"}</Text>
                      ) },
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
                      ) },
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
                      ) },
                  ]}
                />
              </div>
            )}
          </Section>
        </>
)}

        {activeTab === "users" && (
<>
          {activeTab === "users" && (
            <TenantUsers
              tenantId={tenantId}
              workspaces={tenant.workspaces}
              onSelectUser={onSelectUser ?? (() => {})}
              userRole={effectiveRole}
            />
          )}
        </>
)}

        {activeTab === "resources" && (
<>
          {activeTab === "resources" && (
            <Section variant="transparent" padding={0} className="space-y-6">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <Heading level={3}>{t("Resources")}</Heading>
                {canSetLimitOverride && (
                  <Button
                    type="button"
                    label={t("Set extra limit")}
                    variant="secondary"
                    size="sm"
                    icon={<SlidersHorizontal className="size-3.5" aria-hidden="true" />}
                    onClick={handleOpenOverrideDialog}
                  />
                )}
              </div>

              {overrideNotice && (
                <Banner
                  status="info"
                  role="status"
                  title={overrideNotice}
                />
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
                    <Banner
                      status="info"
                      role="status"
                      title={t("Resource counters are partial across the tenant's workspaces.")}
                    />
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
                      <Card padding={0} className="overflow-x-auto">
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
                      </Card>
                    </Section>
                  )}
                </div>
              )}
            </Section>
          )}
        </>
)}

        {canViewPolicy && activeTab === "policy" && (
          <TenantPolicy tenantId={tenantId} userRole={effectiveRole} embedded />
        )}
      </div>

      {canSetLimitOverride && (
        <Dialog
          isOpen={overrideDialogOpen}
          onOpenChange={(open) => {
            if (!setLimitOverride.isPending) {
              setOverrideDialogOpen(open);
              if (!open) setOverrideError(null);
            }
          }}
          purpose="form"
        >

            <form onSubmit={handleSubmitOverride} className="flex flex-col gap-4">
              <DialogHeader title={t("Set extra limit")} subtitle={t("Override one plan limit gate across workspaces of this tenant, or release an existing override.")} />

              {overrideError && (
                <Banner
                  status="error"
                  role="alert"
                  title={overrideError}
                />
              )}

              <Selector
                label={t("Limit gate")}
                isRequired
                width="100%"
                options={LIMIT_GATE_OPTIONS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                value={overrideGate}
                onChange={(val) => {
                    if (val) setOverrideGate(val);
                  }}
              />

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
                <NumberInput
                  label={t("Extra limit value")}
                  id={"override-value-input"}
                  isRequired
                  min={0}
                  step={1}
                  value={overrideValue.trim() === "" ? null : Number(overrideValue)}
                  onChange={(value) => setOverrideValue(value === null ? "" : String(value))}
                  placeholder={t("e.g. 25")}
                  isDisabled={setLimitOverride.isPending}
                  width="100%"
                />
              )}

              <Selector
                label={t("Target workspaces")}
                width="100%"
                options={workspaceScopeOptions.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                value={overrideWorkspaceId}
                onChange={(val) => {
                    if (val) setOverrideWorkspaceId(val);
                  }}
              />

              <TextArea
                label={t("Reason")}
                id={"override-reason-input"}
                isRequired
                value={overrideReason}
                onChange={(value) => setOverrideReason(value)}
                placeholder={t("Explain why this override is being changed...")}
                rows={3}
                isDisabled={setLimitOverride.isPending}
                width="100%"
              />

              <div className="flex justify-end gap-2">
                <Button
                  type="button"
                  variant="secondary"
                  isDisabled={setLimitOverride.isPending}
                  label={t("Cancel")}
                  onClick={() => ((open) => {
            if (!setLimitOverride.isPending) {
              setOverrideDialogOpen(open);
              if (!open) setOverrideError(null);
            }
          })(false)}
                />
                <Button
                  type="submit"
                  label={t("Save limit")}
                  variant="primary"
                  isLoading={setLimitOverride.isPending}
                  aria-busy={setLimitOverride.isPending || undefined}
                  isDisabled={setLimitOverride.isPending}
                />
              </div>
            </form>

        </Dialog>
      )}

      {canManageTenantStatus && (
        <Dialog
          isOpen={statusDialogMode !== null}
          onOpenChange={(open) => {
            if (!open && !setTenantStatus.isPending) {
              setStatusDialogMode(null);
              setStatusDialogError(null);
            }
          }}
          purpose="form"
          width={448}
        >

            <form onSubmit={handleSubmitTenantStatus} className="space-y-4">
              <DialogHeader title={statusDialogMode === "unlock" ? t("Unlock tenant") : t("Lock tenant")} subtitle={statusDialogMode === "unlock"
                    ? t("Restore all locked workspaces in this tenant back to active status.")
                    : t("Cascade a status lock across every active workspace belonging to this tenant.")} />

              {statusDialogError && (
                <Banner
                  status="error"
                  role="alert"
                  title={statusDialogError}
                />
              )}

              {statusDialogMode === "lock" && (
                <Selector
                  label={t("Target status")}
                  width="100%"
                  options={[
                    { value: "suspended", label: t("Suspended") },
                    { value: "read_only", label: t("Read-only") },
                  ]}
                  value={lockTargetStatus}
                  onChange={(val) => {
                    if (val === "suspended" || val === "read_only") {
                      setLockTargetStatus(val);
                    }
                  }}
                />
              )}

              <TextArea
                id="tenant-status-reason"
                label={t("Reason")}
                value={statusReason}
                onChange={(value) => setStatusReason(value)}
                placeholder={t("Explain why this tenant is being updated...")}
                isDisabled={setTenantStatus.isPending}
                rows={3}
                width="100%"
              />

              <div className="flex justify-end gap-2">
                <Button
                  type="button"
                  variant="secondary"
                  isDisabled={setTenantStatus.isPending}
                  label={t("Cancel")}
                  onClick={() => ((open) => {
            if (!open && !setTenantStatus.isPending) {
              setStatusDialogMode(null);
              setStatusDialogError(null);
            }
          })(false)}
                />
                <Button
                  type="submit"
                  label={statusDialogMode === "unlock" ? t("Unlock tenant") : t("Apply lock")}
                  variant={statusDialogMode === "unlock" ? "primary" : "destructive"}
                  isDisabled={!statusReason.trim() || setTenantStatus.isPending}
                  isLoading={setTenantStatus.isPending}
                  aria-busy={setTenantStatus.isPending || undefined}
                />
              </div>
            </form>

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
