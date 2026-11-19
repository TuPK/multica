import { useState } from "react";
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
import { AlertCircle, Building2, Gauge, Info, SlidersHorizontal, Trash2 } from "@multica/ui/icons";
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
  useMe,
  useSetTenantLimitOverride,
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
import { TenantUsers } from "./TenantUsers";

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
    `${workspace.usage_members ?? 0} members`,
    `${workspace.usage_agents ?? 0} agents`,
    `${workspace.usage_tasks_run ?? 0} tasks run`,
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
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;
  const canDeleteTenant = effectiveRole === "system_admin";
  const canSetLimitOverride =
    effectiveRole === "system_admin" || effectiveRole === "billing";

  const [activeTab, setActiveTab] = useState<"workspaces" | "users" | "resources">("workspaces");

  const { data: tenant, isLoading, isError, error, refetch } = useTenant(tenantId);
  const {
    data: resourcesData,
    isLoading: resourcesLoading,
    isError: resourcesError,
    error: resourcesErrorObj,
    refetch: refetchResources,
  } = useTenantResources(tenantId, activeTab === "resources");
  const setLimitOverride = useSetTenantLimitOverride();
  const deleteTenantNow = useDeleteTenantNow();

  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [deleteNotice, setDeleteNotice] = useState<string | null>(null);

  const [overrideDialogOpen, setOverrideDialogOpen] = useState(false);
  const [overrideGate, setOverrideGate] = useState<string>("agents");
  const [overrideMode, setOverrideMode] = useState<"set" | "release">("set");
  const [overrideValue, setOverrideValue] = useState("");
  const [overrideWorkspaceId, setOverrideWorkspaceId] = useState<string>("all");
  const [overrideReason, setOverrideReason] = useState("");
  const [overrideError, setOverrideError] = useState<string | null>(null);
  const [overrideNotice, setOverrideNotice] = useState<string | null>(null);
  const back = { label: "Back to tenants", onClick: onBack };

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Tenant" backAction={back} />
        <LoadingState message="Loading tenant..." />
      </div>
    );
  }

  if (isError || !tenant) {
    return (
      <div className="space-y-6">
        <PageHeader title="Tenant" backAction={back} />
        <ErrorState
          title="Failed to load tenant"
          message={error instanceof Error ? error.message : undefined}
          onRetry={refetch}
        />
      </div>
    );
  }

  const handleConfirmDeleteTenant = async ({ reason, confirm }: { reason: string; confirm: string }) => {
    setDeleteError(null);
    try {
      const res = await deleteTenantNow.mutateAsync({ tenantId, reason, confirm });
      setDeleteDialogOpen(false);
      setDeleteNotice(`Immediate deletion requested (${res.request_id}).`);
    } catch (err) {
      setDeleteError(getApiErrorMessage(err, "Failed to start tenant deletion"));
    }
  };

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
      setOverrideError("A reason is required.");
      return;
    }

    let nextValue: number | null = null;
    if (overrideMode === "set") {
      if (overrideValue.trim() === "") {
        setOverrideError("Enter a non-negative integer value.");
        return;
      }
      const parsed = Number(overrideValue);
      if (!Number.isFinite(parsed) || !Number.isInteger(parsed) || parsed < 0) {
        setOverrideError("Value must be a non-negative integer.");
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
          ? `Released ${res.gate} limit override for ${res.workspace_ids.length} workspace(s).`
          : `Set ${res.gate} limit override to ${nextValue.toLocaleString()} for ${res.workspace_ids.length} workspace(s).`,
      );
    } catch (err) {
      setOverrideError(getLimitOverrideErrorMessage(err, "Failed to set limit override"));
    }
  };

  const workspaceScopeOptions = [
    { value: "all", label: "All workspaces" },
    ...tenant.workspaces.map((ws) => ({
      value: ws.id,
      label: ws.name || ws.slug || ws.id,
    })),
  ];

  return (
    <div className="space-y-6">
      <PageHeader
        title={tenant.name || tenant.id}
        badge={<StatusBadge status={tenant.status} />}
        backAction={back}
        description={<span className="font-mono text-caption">{tenant.id}</span>}
        actions={
          canDeleteTenant ? (
            <Button
              variant="destructive"
              size="sm"
              onClick={() => {
                setDeleteError(null);
                setDeleteDialogOpen(true);
              }}
            >
              <Trash2 className="size-3.5" aria-hidden="true" />
              <span>Delete tenant</span>
            </Button>
          ) : undefined
        }
      />

      {deleteNotice && (
        <Alert role="status">
          <Info className="size-4" aria-hidden="true" />
          <AlertDescription>{deleteNotice}</AlertDescription>
        </Alert>
      )}

      <Card padding={4}>
        <MetadataList columns={3}>
          <MetadataListItem label="Workspaces" tabular>
            <span data-testid="tenant-workspace-count">{tenant.workspace_count}</span>
          </MetadataListItem>
          <MetadataListItem label="Max workspaces" tabular>
            <span data-testid="tenant-max-workspaces">{maxWorkspacesLabel(tenant.max_workspaces)}</span>
          </MetadataListItem>
          <MetadataListItem label="Best plan">{tenant.best_plan_key || "-"}</MetadataListItem>
          <MetadataListItem label="Workspace status">
            <span data-testid="tenant-status-summary">{statusSummaryText(tenant.status_summary)}</span>
          </MetadataListItem>
          <MetadataListItem label="Last activity" tabular>
            {tenant.last_activity_at ? new Date(tenant.last_activity_at).toLocaleString() : "-"}
          </MetadataListItem>
        </MetadataList>
      </Card>

      <Tabs
        value={activeTab}
        onValueChange={(value) => {
          if (value === "workspaces" || value === "users" || value === "resources") {
            setActiveTab(value);
          }
        }}
        className="space-y-4"
      >
        <div className="border-b border-surface-border">
          <TabsList variant="line" aria-label="Tenant sections">
            <TabsTrigger value="workspaces">Workspaces</TabsTrigger>
            <TabsTrigger value="users">Users</TabsTrigger>
            <TabsTrigger value="resources">Resources</TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value="workspaces">
          <Section variant="transparent" padding={0} className="space-y-4">
            <Heading level={3}>Workspaces</Heading>
            {tenant.workspaces.length === 0 ? (
              <EmptyState
                icon={<Building2 className="size-5" aria-hidden="true" />}
                title="This tenant has no workspaces"
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
                      header: "Workspace",
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
                      header: "Plan",
                      width: proportional(1, { minWidth: 100 }),
                      renderCell: (workspace) => <Text type="supporting">{workspace.plan_key || "-"}</Text>,
                    },
                    {
                      key: "status",
                      header: "Status",
                      width: proportional(1, { minWidth: 110 }),
                      renderCell: (workspace) => <StatusBadge status={workspace.status} />,
                    },
                    {
                      key: "member_count",
                      header: "Members",
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
                      header: "Owner",
                      width: proportional(1.5, { minWidth: 160 }),
                      renderCell: (workspace) => (
                        <Text type="supporting">{workspace.owner?.email ?? "-"}</Text>
                      ),
                    },
                    {
                      key: "usage",
                      header: "Usage",
                      width: proportional(1.5, { minWidth: 160 }),
                      renderCell: (workspace) => (
                        <span
                          data-testid="workspace-usage"
                          title={workspace.usage_day ? `Usage on ${workspace.usage_day} (UTC)` : "No usage recorded yet"}
                        >
                          <Text type="supporting" hasTabularNumbers>
                            {usageSummary(workspace)}
                          </Text>
                        </span>
                      ),
                    },
                    {
                      key: "actions",
                      header: "Action",
                      width: pixel(96),
                      align: "end",
                      renderCell: (workspace) => (
                        <Button variant="outline" size="sm" onClick={() => onSelectWorkspace(workspace.id)}>
                          View
                        </Button>
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
                <Heading level={3}>Resources</Heading>
                {canSetLimitOverride && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={handleOpenOverrideDialog}
                  >
                    <SlidersHorizontal className="size-3.5" aria-hidden="true" />
                    <span>Set extra limit</span>
                  </Button>
                )}
              </div>

              {overrideNotice && (
                <Alert role="status">
                  <Info className="size-4" aria-hidden="true" />
                  <AlertDescription>{overrideNotice}</AlertDescription>
                </Alert>
              )}

              {resourcesLoading && <LoadingState message="Loading tenant resources..." />}

              {resourcesError && (
                <ErrorState
                  title="Failed to load tenant resources"
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
                        Resource counters are partial across the tenant&apos;s workspaces.
                      </AlertDescription>
                    </Alert>
                  )}

                  {resourcesData.resources.length === 0 ? (
                    <EmptyState
                      icon={<Gauge className="size-5" aria-hidden="true" />}
                      title="No resource usage reported"
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
                                label={`${label} usage`}
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
                      <Heading level={3}>By Workspace</Heading>
                      <div className="overflow-x-auto">
                        <Table density="compact" dividers="rows" hasHover>
                          <TableHeader>
                            <TableRow isHeaderRow>
                              <TableHeaderCell>Workspace</TableHeaderCell>
                              <TableHeaderCell>Plan</TableHeaderCell>
                              <TableHeaderCell>Usage</TableHeaderCell>
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
                <DialogTitle>Set extra limit</DialogTitle>
                <DialogDescription>
                  Override one plan limit gate across workspaces of this tenant, or release an existing override.
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
                  Limit gate <span className="text-destructive">*</span>
                </FieldLabel>
                <Select
                  items={LIMIT_GATE_OPTIONS}
                  value={overrideGate}
                  onValueChange={(val) => {
                    if (val) setOverrideGate(val);
                  }}
                  disabled={setLimitOverride.isPending}
                >
                  <SelectTrigger id="override-gate-select" aria-label="Limit gate" className="w-full">
                    <SelectValue>
                      {LIMIT_GATE_OPTIONS.find((item) => item.value === overrideGate)?.label ??
                        overrideGate}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {LIMIT_GATE_OPTIONS.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>

              <RadioList
                label="Override action"
                value={overrideMode}
                onChange={(val) => setOverrideMode(val as "set" | "release")}
                isDisabled={setLimitOverride.isPending}
              >
                <RadioListItem value="set" label="Set custom limit" />
                <RadioListItem value="release" label="Release limit (use plan default)" />
              </RadioList>

              {overrideMode === "set" && (
                <Field>
                  <FieldLabel htmlFor="override-value-input" className="text-label">
                    Extra limit value <span className="text-destructive">*</span>
                  </FieldLabel>
                  <Input
                    id="override-value-input"
                    type="number"
                    min={0}
                    step={1}
                    value={overrideValue}
                    onChange={(e) => setOverrideValue(e.target.value)}
                    placeholder="e.g. 25"
                    disabled={setLimitOverride.isPending}
                  />
                </Field>
              )}

              <Field>
                <FieldLabel htmlFor="override-workspace-select" className="text-label">
                  Target workspaces
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
                    aria-label="Target workspaces"
                    className="w-full"
                  >
                    <SelectValue>
                      {workspaceScopeOptions.find((item) => item.value === overrideWorkspaceId)
                        ?.label ?? "All workspaces"}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {workspaceScopeOptions.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>

              <Field>
                <FieldLabel htmlFor="override-reason-input" className="text-label">
                  Reason <span className="text-destructive">*</span>
                </FieldLabel>
                <Textarea
                  id="override-reason-input"
                  value={overrideReason}
                  onChange={(e) => setOverrideReason(e.target.value)}
                  placeholder="Explain why this override is being changed..."
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
                  Cancel
                </DialogClose>
                <Button
                  type="submit"
                  variant="default"
                  disabled={setLimitOverride.isPending}
                  aria-busy={setLimitOverride.isPending || undefined}
                >
                  {setLimitOverride.isPending && <Spinner className="size-3.5" />}
                  Save limit
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
          title="Delete tenant"
          description="Immediate deletion permanently removes this tenant and all of its workspaces."
          expectedText={tenant.name || tenant.id}
          confirmInputLabel="Tenant name"
          confirmLabel="Delete tenant"
          confirmVariant="destructive"
          error={deleteError}
          onConfirm={handleConfirmDeleteTenant}
        />
      )}
    </div>
  );
}
