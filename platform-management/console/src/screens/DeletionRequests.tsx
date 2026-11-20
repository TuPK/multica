import { useState } from "react";
import { Card } from "@astryxdesign/core/Card";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { RefreshCw, Trash2 } from "@multica/ui/icons";
import { Button } from "@astryxdesign/core/Button";
import { Selector } from "@astryxdesign/core/Selector";
import { Banner } from "@astryxdesign/core/Banner";
import { getApiErrorMessage } from "../api/client";
import {
  useCancelDeletionRequest,
  useDeletionRequests,
  useMe,
  useRetryDeletionRequest,
  type DeletionRequest } from "../api/hooks";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { PageHeader } from "../components/PageHeader";
import { RowActions } from "../components/RowActions";
import { StatusBadge } from "../components/StatusBadge";
import type { StaffRole } from "../components/types";
import { t } from "../i18n";

interface DeletionRequestsProps {
  userRole?: StaffRole;
}

const KIND_OPTIONS = [
  { value: "all", label: "All kinds" },
  { value: "workspace", label: "Workspace" },
  { value: "tenant", label: "Tenant" },
  { value: "account", label: "Account" },
] as const;

const STATE_OPTIONS = [
  { value: "all", label: "All states" },
  { value: "awaiting_approval", label: "Awaiting approval" },
  { value: "pending", label: "Pending" },
  { value: "running", label: "Running" },
  { value: "failed", label: "Failed" },
  { value: "legal_hold", label: "Legal hold" },
  { value: "export_timeout", label: "Export timeout" },
  { value: "completed", label: "Completed" },
  { value: "canceled", label: "Canceled" },
] as const;

function kindLabel(kind: DeletionRequest["kind"]): string {
  switch (kind) {
    case "workspace":
      return t("Workspace");
    case "tenant":
      return t("Tenant");
    case "account":
      return t("Account");
    default:
      return t("Unknown");
  }
}

export function DeletionRequests({ userRole }: DeletionRequestsProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;

  const [kindFilter, setKindFilter] = useState("");
  const [stateFilter, setStateFilter] = useState("");
  const [busyId, setBusyId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const canAccess =
    !effectiveRole || effectiveRole === "system_admin" || effectiveRole === "admin";

  const { data: requests = [], isLoading, isError, error, refetch, isFetching } =
    useDeletionRequests({
      kind: kindFilter || undefined,
      state: stateFilter || undefined });

  const cancelMutation = useCancelDeletionRequest();
  const retryMutation = useRetryDeletionRequest();

  if (!canAccess) {
    return (
      <section aria-label={t("Not found")} className="py-12 text-center">
        <Text type="supporting">{t("Not found")}</Text>
      </section>
    );
  }

  const canActOnRequest = (req: DeletionRequest): boolean => {
    if (effectiveRole === "system_admin") return true;
    if (effectiveRole === "admin") {
      return (
        req.kind === "workspace" &&
        Boolean(me?.user_id) &&
        req.requested_by === me?.user_id
      );
    }
    return false;
  };

  const handleCancel = async (id: string) => {
    setBusyId(id);
    setActionError(null);
    try {
      await cancelMutation.mutateAsync({ id });
    } catch (err) {
      setActionError(getApiErrorMessage(err, t("Failed to cancel deletion request.")));
    } finally {
      setBusyId(null);
    }
  };

  const handleRetry = async (id: string) => {
    setBusyId(id);
    setActionError(null);
    try {
      await retryMutation.mutateAsync({ id });
    } catch (err) {
      setActionError(getApiErrorMessage(err, t("Failed to retry deletion request.")));
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Deletion requests")}
        actions={
          <Button
            label={t("Refresh")}
            variant="secondary"
            size="sm"
            icon={<RefreshCw className="size-3.5" aria-hidden="true" />}
            isLoading={isFetching}
            onClick={() => refetch()}
            aria-busy={isFetching || undefined}
            isDisabled={isFetching}
          />
        }
      />

      <div className="flex flex-wrap items-center gap-3">
        <Selector
          label={t("Filter by kind")}
          isLabelHidden
          width={176}
          options={KIND_OPTIONS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
          value={kindFilter || "all"}
          onChange={(value) => setKindFilter(value === "all" ? "" : (value ?? ""))}
        />

        <Selector
          label={t("Filter by state")}
          isLabelHidden
          width={176}
          options={STATE_OPTIONS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
          value={stateFilter || "all"}
          onChange={(value) => setStateFilter(value === "all" ? "" : (value ?? ""))}
        />
      </div>

      {actionError && (
        <Banner
          status="error"
          role="alert"
          title={actionError}
        />
      )}

      {isLoading && <LoadingState message={t("Loading deletion requests...")} />}

      {isError && (
        <ErrorState
          title={t("Failed to load deletion requests")}
          message={getApiErrorMessage(error, t("An error occurred while loading deletion requests."))}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && requests.length === 0 && (
        <EmptyState
          icon={<Trash2 className="size-4" />}
          title={t("No deletion requests found")}
        />
      )}

      {!isLoading && !isError && requests.length > 0 && (
        <Card padding={0} className="overflow-x-auto">
          <Table density="compact" dividers="rows" hasHover>
            <TableHeader>
              <TableRow isHeaderRow>
                <TableHeaderCell>{t("Target")}</TableHeaderCell>
                <TableHeaderCell>{t("Kind")}</TableHeaderCell>
                <TableHeaderCell>{t("Reason")}</TableHeaderCell>
                <TableHeaderCell>{t("State")}</TableHeaderCell>
                <TableHeaderCell>{t("Error")}</TableHeaderCell>
                <TableHeaderCell className="text-right">{t("Attempts")}</TableHeaderCell>
                <TableHeaderCell>{t("Requested by")}</TableHeaderCell>
                <TableHeaderCell>{t("Updated")}</TableHeaderCell>
                <TableHeaderCell className="text-right">{t("Actions")}</TableHeaderCell>
              </TableRow>
            </TableHeader>
            <TableBody>
              {requests.map((req) => {
                const rowBusy = busyId === req.id;
                const allowed = canActOnRequest(req);
                const canRetry = allowed && req.state === "failed";
                const canCancel = allowed && (req.state === "pending" || req.state === "failed");

                return (
                  <TableRow key={req.id}>
                    <TableCell>
                      <div className="min-w-0">
                        <div className="font-medium text-foreground">
                          {req.target_label || req.target_id}
                        </div>
                        {req.target_label && req.target_id && (
                          <div className="font-mono text-caption text-muted-foreground">
                            {req.target_id}
                          </div>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Text type="body">{kindLabel(req.kind)}</Text>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting">{req.reason || "—"}</Text>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={req.state} />
                    </TableCell>
                    <TableCell>
                      {req.error_code === "legal_hold" || req.error_code === "export_timeout" ? (
                        <StatusBadge status={req.error_code} />
                      ) : req.error_code ? (
                        <span className="font-mono text-caption text-destructive">
                          {req.error_code}
                        </span>
                      ) : (
                        <Text type="supporting">—</Text>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <Text type="body" hasTabularNumbers>
                        {req.attempts}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting">
                        {req.requested_by_label || req.requested_by || "—"}
                      </Text>
                    </TableCell>
                    <TableCell>
                      <Text type="supporting" hasTabularNumbers>
                        {req.updated_at ? new Date(req.updated_at).toLocaleString() : "—"}
                      </Text>
                    </TableCell>
                    <TableCell className="text-right">
                      <RowActions
                        label={t("deletion request for {0}", req.target_label || req.target_id)}
                        actions={[
                          ...(canRetry
                            ? [{ label: t("Retry"), busy: rowBusy && retryMutation.isPending, disabled: rowBusy, onSelect: () => handleRetry(req.id) }]
                            : []),
                          ...(canCancel
                            ? [{ label: t("Cancel"), destructive: true, busy: rowBusy && cancelMutation.isPending, disabled: rowBusy, onSelect: () => handleCancel(req.id) }]
                            : []),
                        ]}
                      />
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Card>
      )}
    </div>
  );
}
