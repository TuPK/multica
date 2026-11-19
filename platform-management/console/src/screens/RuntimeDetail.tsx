import { useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
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
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import { Ban, CheckCircle2, Info, Server, Trash2 } from "@multica/ui/icons";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { TypeToConfirmDialog } from "../components/TypeToConfirmDialog";
import { getApiErrorMessage } from "../api/client";
import {
  useMe,
  useRuntime,
  useSetRuntimeStatus,
  useUnregisterRuntime,
  type RuntimeStatus,
  type RuntimeTask,
} from "../api/hooks";
import type { AuditEvent } from "../api/schemas";
import type { StaffRole } from "../components/types";

interface RuntimeDetailProps {
  runtimeId: string;
  onBack: () => void;
  onSelectWorkspace?: (workspaceId: string) => void;
  userRole?: StaffRole;
}

function ConnectionStatusBadge({ status }: { status: RuntimeStatus }) {
  if (status === "online") {
    return (
      <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
        <StatusDot variant="success" label="Online" aria-hidden="true" />
        <Text type="body" color="primary">
          Online
        </Text>
      </span>
    );
  }
  if (status === "offline") {
    return (
      <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
        <StatusDot variant="neutral" label="Offline" aria-hidden="true" />
        <Text type="body" color="secondary">
          Offline
        </Text>
      </span>
    );
  }
  return <Badge variant="neutral" label="Unknown" />;
}

function auditReason(detail: Record<string, unknown> | null): string {
  if (!detail) return "-";
  const raw = detail["reason"];
  return typeof raw === "string" && raw.trim() !== "" ? raw : "-";
}

export function RuntimeDetail({
  runtimeId,
  onBack,
  onSelectWorkspace,
  userRole,
}: RuntimeDetailProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;
  const canRead =
    effectiveRole === "system_admin" ||
    effectiveRole === "admin" ||
    effectiveRole === "support";
  const canControlRuntime = effectiveRole === "system_admin";

  const {
    data: runtime,
    isLoading,
    isError,
    error,
    refetch,
  } = useRuntime(runtimeId, canRead);

  const setRuntimeStatus = useSetRuntimeStatus();
  const unregisterRuntime = useUnregisterRuntime();

  const [statusDialogOpen, setStatusDialogOpen] = useState(false);
  const [statusError, setStatusError] = useState<string | null>(null);
  const [unregisterDialogOpen, setUnregisterDialogOpen] = useState(false);
  const [unregisterError, setUnregisterError] = useState<string | null>(null);
  const [actionNotice, setActionNotice] = useState<string | null>(null);

  if (effectiveRole && !canRead) {
    return (
      <div role="region" aria-label="Not found">
        <ErrorState
          title="Not found"
          message="The page you requested was not found or you do not have permission to view it."
        />
      </div>
    );
  }

  const back = { label: "Back to runtimes", onClick: onBack };

  if (isLoading) {
    return (
      <Layout
        header={
          <LayoutHeader hasDivider>
            <PageHeader title="Runtime" backAction={back} />
          </LayoutHeader>
        }
        content={
          <LayoutContent>
            <LoadingState message="Loading runtime..." />
          </LayoutContent>
        }
      />
    );
  }

  if (isError || !runtime) {
    return (
      <Layout
        header={
          <LayoutHeader hasDivider>
            <PageHeader title="Runtime" backAction={back} />
          </LayoutHeader>
        }
        content={
          <LayoutContent>
            <ErrorState
              title="Failed to load runtime"
              message={error instanceof Error ? error.message : undefined}
              onRetry={refetch}
            />
          </LayoutContent>
        }
      />
    );
  }

  const nextStatus: "disabled" | "enabled" = runtime.disabled ? "enabled" : "disabled";

  const handleConfirmStatusChange = async (reason: string) => {
    setStatusError(null);
    try {
      await setRuntimeStatus.mutateAsync({
        runtimeId: runtime.id,
        status: nextStatus,
        reason,
      });
      setStatusDialogOpen(false);
      setActionNotice(
        nextStatus === "disabled" ? "Runtime disabled." : "Runtime enabled.",
      );
    } catch (err) {
      setStatusError(getApiErrorMessage(err, "Failed to update runtime status"));
    }
  };

  const handleConfirmUnregister = async ({
    reason,
    confirm,
  }: {
    reason: string;
    confirm: string;
  }) => {
    setUnregisterError(null);
    try {
      const res = await unregisterRuntime.mutateAsync({
        runtimeId: runtime.id,
        reason,
        confirm,
      });
      setUnregisterDialogOpen(false);
      setActionNotice(`Runtime unregister requested (${res.request_id}).`);
    } catch (err) {
      setUnregisterError(getApiErrorMessage(err, "Failed to unregister runtime"));
    }
  };

  return (
    <Layout
      header={
        <LayoutHeader hasDivider>
          <PageHeader
            title={runtime.name || runtime.id}
            backAction={back}
            badge={
              <div className="flex flex-wrap items-center gap-2">
                <ConnectionStatusBadge status={runtime.status} />
                {runtime.disabled && (
                  <Badge
                    variant="error"
                    label="Disabled"
                    icon={<StatusDot variant="error" label="Disabled" aria-hidden="true" />}
                  />
                )}
              </div>
            }
            meta={<span className="font-mono">{runtime.id}</span>}
            actions={
              canControlRuntime ? (
                <>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      setStatusError(null);
                      setStatusDialogOpen(true);
                    }}
                  >
                    {runtime.disabled ? (
                      <CheckCircle2 className="size-3.5" aria-hidden="true" />
                    ) : (
                      <Ban className="size-3.5" aria-hidden="true" />
                    )}
                    <span>{runtime.disabled ? "Enable runtime" : "Disable runtime"}</span>
                  </Button>
                  <Button
                    type="button"
                    variant="destructive"
                    size="sm"
                    onClick={() => {
                      setUnregisterError(null);
                      setUnregisterDialogOpen(true);
                    }}
                  >
                    <Trash2 className="size-3.5" aria-hidden="true" />
                    <span>Unregister runtime</span>
                  </Button>
                </>
              ) : undefined
            }
          />
        </LayoutHeader>
      }
      content={
        <LayoutContent>
          <div className="space-y-6">
            {actionNotice && (
              <Alert role="status">
                <Info className="size-4" aria-hidden="true" />
                <AlertDescription>{actionNotice}</AlertDescription>
              </Alert>
            )}

            <Section className="space-y-4">
              <Heading level={3}>Runtime Details</Heading>
              <MetadataList columns={3}>
                <MetadataListItem label="Connection">
                  <ConnectionStatusBadge status={runtime.status} />
                </MetadataListItem>
                <MetadataListItem label="Access state">
                  {runtime.disabled ? (
                    <Badge
                      variant="error"
                      label="Disabled"
                      icon={<StatusDot variant="error" label="Disabled" aria-hidden="true" />}
                    />
                  ) : (
                    <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
                      <StatusDot variant="success" label="Enabled" aria-hidden="true" />
                      <Text type="body" color="primary">
                        Enabled
                      </Text>
                    </span>
                  )}
                </MetadataListItem>
                <MetadataListItem label="OS" mono>
                  {runtime.os || "-"}
                </MetadataListItem>
                <MetadataListItem label="Daemon version" mono>
                  {runtime.daemon_version || "-"}
                </MetadataListItem>
                <MetadataListItem label="Agent version" mono>
                  {runtime.agent_version || "-"}
                </MetadataListItem>
                <MetadataListItem label="Last seen" tabular>
                  {runtime.last_seen_at
                    ? new Date(runtime.last_seen_at).toLocaleString()
                    : "-"}
                </MetadataListItem>
                <MetadataListItem label="Running tasks" tabular>
                  {runtime.running_tasks.toLocaleString()}
                </MetadataListItem>
                <MetadataListItem label="Failed tasks (7d)" tabular>
                  <span data-testid="runtime-failed-tasks-7d">
                    {runtime.failed_tasks_7d.toLocaleString()}
                  </span>
                </MetadataListItem>
                <MetadataListItem label="Created" tabular>
                  {runtime.created_at
                    ? new Date(runtime.created_at).toLocaleString()
                    : "-"}
                </MetadataListItem>
                <MetadataListItem label="Workspace ID" mono>
                  {onSelectWorkspace && runtime.workspace_id ? (
                    <Button
                      type="button"
                      variant="link"
                      size="sm"
                      className="h-auto p-0 font-mono"
                      onClick={() => onSelectWorkspace(runtime.workspace_id)}
                    >
                      {runtime.workspace_id}
                    </Button>
                  ) : (
                    runtime.workspace_id || "-"
                  )}
                </MetadataListItem>
                <MetadataListItem label="Tenant ID" mono>
                  {runtime.tenant_id || "-"}
                </MetadataListItem>
                <MetadataListItem label="Owner ID" mono>
                  {runtime.owner_id || "-"}
                </MetadataListItem>
              </MetadataList>
            </Section>

            <Section className="space-y-4">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <Heading level={3}>Recent Tasks</Heading>
                <Text type="supporting" hasTabularNumbers>
                  {runtime.tasks.length.toLocaleString()} shown
                </Text>
              </div>
              {runtime.tasks.length === 0 ? (
                <EmptyState
                  icon={<Server className="size-5" aria-hidden="true" />}
                  title="No recent tasks"
                  isCompact
                />
              ) : (
                <div className="overflow-x-auto">
                  <Table density="compact" dividers="rows" hasHover>
                    <TableHeader>
                      <TableRow isHeaderRow>
                        <TableHeaderCell>Task ID</TableHeaderCell>
                        <TableHeaderCell>Status</TableHeaderCell>
                        <TableHeaderCell>Error</TableHeaderCell>
                        <TableHeaderCell>Created</TableHeaderCell>
                        <TableHeaderCell>Completed</TableHeaderCell>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {runtime.tasks.map((task: RuntimeTask) => (
                        <TableRow key={task.id}>
                          <TableCell>
                            <Text type="code">{task.id}</Text>
                          </TableCell>
                          <TableCell>
                            <StatusBadge status={task.status} />
                          </TableCell>
                          <TableCell>
                            {task.has_error ? (
                              <Badge variant="error" label="Error" />
                            ) : (
                              <Text type="supporting">-</Text>
                            )}
                          </TableCell>
                          <TableCell>
                            <Text type="supporting" hasTabularNumbers>
                              {task.created_at
                                ? new Date(task.created_at).toLocaleString()
                                : "-"}
                            </Text>
                          </TableCell>
                          <TableCell>
                            <Text type="supporting" hasTabularNumbers>
                              {task.completed_at
                                ? new Date(task.completed_at).toLocaleString()
                                : "-"}
                            </Text>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </Section>

            <Section className="space-y-4">
              <Heading level={3}>Staff Audit Events</Heading>
              {runtime.audit_events.length === 0 ? (
                <EmptyState
                  icon={<Server className="size-5" aria-hidden="true" />}
                  title="No staff audit events"
                  isCompact
                />
              ) : (
                <div className="overflow-x-auto">
                  <Table density="compact" dividers="rows" hasHover>
                    <TableHeader>
                      <TableRow isHeaderRow>
                        <TableHeaderCell>When</TableHeaderCell>
                        <TableHeaderCell>Actor</TableHeaderCell>
                        <TableHeaderCell>Action</TableHeaderCell>
                        <TableHeaderCell>Reason</TableHeaderCell>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {runtime.audit_events.map((ev: AuditEvent) => (
                        <TableRow key={ev.id}>
                          <TableCell>
                            <Text type="supporting" hasTabularNumbers>
                              {ev.created_at
                                ? new Date(ev.created_at).toLocaleString()
                                : "-"}
                            </Text>
                          </TableCell>
                          <TableCell>
                            <Text type="body">{ev.actor_label || ev.actor_id || "-"}</Text>
                          </TableCell>
                          <TableCell>
                            <Text type="code">{ev.action}</Text>
                          </TableCell>
                          <TableCell>
                            <Text type="supporting">{auditReason(ev.detail)}</Text>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </Section>
          </div>

          {canControlRuntime && (
            <>
              <ConfirmReasonDialog
                open={statusDialogOpen}
                onOpenChange={(open) => {
                  if (!setRuntimeStatus.isPending) {
                    setStatusDialogOpen(open);
                    if (!open) setStatusError(null);
                  }
                }}
                title={runtime.disabled ? "Enable runtime" : "Disable runtime"}
                description={
                  runtime.disabled
                    ? "Re-enabling allows this daemon to reconnect and claim queued tasks."
                    : "Disabling rejects daemon routes and blocks task claims until re-enabled."
                }
                confirmLabel={runtime.disabled ? "Enable runtime" : "Disable runtime"}
                confirmVariant={runtime.disabled ? "default" : "destructive"}
                isPending={setRuntimeStatus.isPending}
                error={statusError}
                onConfirm={handleConfirmStatusChange}
              />

              <TypeToConfirmDialog
                open={unregisterDialogOpen}
                onOpenChange={(open) => {
                  setUnregisterDialogOpen(open);
                  if (!open) setUnregisterError(null);
                }}
                title="Unregister runtime"
                description="Unregistering queues permanent removal of this runtime and cannot be undone."
                expectedText={runtime.name || runtime.id}
                confirmInputLabel="Runtime name"
                confirmLabel="Unregister runtime"
                confirmVariant="destructive"
                error={unregisterError}
                onConfirm={handleConfirmUnregister}
              />
            </>
          )}
        </LayoutContent>
      }
    />
  );
}
