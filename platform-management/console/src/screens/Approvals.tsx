import { useState } from "react";
import { Heading } from "@astryxdesign/core/Heading";
import { HStack, Layout, LayoutContent, LayoutHeader } from "@astryxdesign/core/Layout";
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
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Button } from "@multica/ui/components/ui/button";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { AlertCircle, RefreshCw, ShieldCheck } from "@multica/ui/icons";
import { RowActions } from "../components/RowActions";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { StatusBadge } from "../components/StatusBadge";
import { getApprovalErrorMessage } from "../api/client";
import {
  useApprovals,
  useApproveApproval,
  useMe,
  useRejectApproval,
} from "../api/hooks";
import type { StaffRole } from "../components/types";
import { t } from "../i18n";

export interface ApprovalsProps {
  userRole?: StaffRole;
}

export function Approvals({ userRole }: ApprovalsProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;
  const canAccess = !effectiveRole || effectiveRole === "system_admin";

  const query = useApprovals(canAccess);
  const approveMutation = useApproveApproval();
  const rejectMutation = useRejectApproval();

  const [busyId, setBusyId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  if (!canAccess) {
    return (
      <div role="region" aria-label={t("Not found")}>
        <ErrorState
          title={t("Not found")}
          message={t("The page you requested was not found or you do not have permission to view it.")}
        />
      </div>
    );
  }

  const approvals = query.data ?? [];

  const handleApprove = async (id: string) => {
    setBusyId(id);
    setActionError(null);
    try {
      await approveMutation.mutateAsync({ id });
    } catch (err) {
      setActionError(getApprovalErrorMessage(err, t("Failed to approve request")));
    } finally {
      setBusyId(null);
    }
  };

  const handleReject = async (id: string) => {
    setBusyId(id);
    setActionError(null);
    try {
      await rejectMutation.mutateAsync({ id });
    } catch (err) {
      setActionError(getApprovalErrorMessage(err, t("Failed to reject request")));
    } finally {
      setBusyId(null);
    }
  };

  return (
    <Layout height="auto">
      <LayoutHeader hasDivider>
        <HStack justify="between" align="center" wrap="wrap" gap={3}>
          <Heading level={1}>{t("Approvals")}</Heading>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => query.refetch()}
            disabled={query.isFetching}
            aria-busy={query.isFetching || undefined}
          >
            {query.isFetching ? (
              <Spinner />
            ) : (
              <RefreshCw className="size-4" aria-hidden="true" />
            )}
            {t("Refresh")}
          </Button>
        </HStack>
      </LayoutHeader>

      <LayoutContent>
        <div className="space-y-6">
          {actionError && (
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertDescription>{actionError}</AlertDescription>
            </Alert>
          )}

          <Section padding={4}>
            <div className="space-y-4">
              <Heading level={3}>{t("Pending approvals")}</Heading>
              {query.isLoading && <LoadingState message={t("Loading pending approvals...")} />}

              {query.isError && (
                <ErrorState
                  title={t("Failed to load approvals")}
                  message={query.error instanceof Error ? query.error.message : undefined}
                  onRetry={query.refetch}
                />
              )}

              {!query.isLoading && !query.isError && approvals.length === 0 && (
                <EmptyState
                  icon={<ShieldCheck className="size-5" aria-hidden="true" />}
                  title={t("No approvals awaiting review")}
                  description={t("Two-person confirmation requests will appear here when queued.")}
                />
              )}

              {!query.isLoading && !query.isError && approvals.length > 0 && (
                <Table density="compact" dividers="rows" hasHover>
                  <TableHeader>
                    <TableRow isHeaderRow>
                      <TableHeaderCell>{t("Target")}</TableHeaderCell>
                      <TableHeaderCell>{t("Kind")}</TableHeaderCell>
                      <TableHeaderCell>{t("Reason")}</TableHeaderCell>
                      <TableHeaderCell>{t("Requested by")}</TableHeaderCell>
                      <TableHeaderCell>{t("State")}</TableHeaderCell>
                      <TableHeaderCell className="text-right">{t("Actions")}</TableHeaderCell>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {approvals.map((req) => {
                      const isSelfRequest = Boolean(me?.user_id && req.requested_by === me.user_id);
                      const rowBusy = busyId === req.id;

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
                            <Text type="body">{req.kind}</Text>
                          </TableCell>
                          <TableCell>
                            <Text type="supporting">{req.reason || "—"}</Text>
                          </TableCell>
                          <TableCell>
                            <div className="space-y-0.5">
                              <Text type="supporting">
                                {req.requested_by_label || req.requested_by || "—"}
                              </Text>
                              {isSelfRequest && (
                                <Text type="supporting" color="secondary" as="div">
                                  {t("Approver must differ from requester")}
                                </Text>
                              )}
                            </div>
                          </TableCell>
                          <TableCell>
                            <StatusBadge status={req.state} />
                          </TableCell>
                          <TableCell className="text-right">
                            <RowActions
                              label={t("approval request for {0}", req.target_label || req.target_id)}
                              actions={[
                                {
                                  label: t("Approve"),
                                  primary: true,
                                  busy: rowBusy && approveMutation.isPending,
                                  disabled: isSelfRequest || rowBusy,
                                  onSelect: () => handleApprove(req.id),
                                },
                                {
                                  label: t("Reject"),
                                  destructive: true,
                                  busy: rowBusy && rejectMutation.isPending,
                                  disabled: isSelfRequest || rowBusy,
                                  onSelect: () => handleReject(req.id),
                                },
                              ]}
                            />
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              )}
            </div>
          </Section>
        </div>
      </LayoutContent>
    </Layout>
  );
}
