import { useState } from "react";
import { Badge } from "@astryxdesign/core/Badge";
import { Card } from "@astryxdesign/core/Card";
import { Divider } from "@astryxdesign/core/Divider";
import { Heading } from "@astryxdesign/core/Heading";
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
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Inbox, RefreshCw, X } from "@multica/ui/icons";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { MetadataList, MetadataListItem } from "../components/MetadataList";
import { PageHeader } from "../components/PageHeader";
import { StatusBadge } from "../components/StatusBadge";
import { PlatformApiError } from "../api/client";
import {
  useDismissPaymentNotice,
  useMe,
  usePaymentNotice,
  usePaymentNotices,
  useResolvePaymentNotice,
} from "../api/hooks";
import type { Notice, NoticeCandidate } from "../api/schemas";
import type { StaffRole } from "../components/types";
import { formatMinorAmount } from "../money";

interface PaymentNoticesProps {
  userRole?: StaffRole;
}

const REVIEW_REASON_LABELS: Record<string, string> = {
  unknown_code: "Unknown transfer code",
  ambiguous_code: "Ambiguous transfer code",
  amount_mismatch: "Amount mismatch",
  currency_mismatch: "Currency mismatch",
  order_not_pending: "Order not pending",
  unparsable: "Unparsable",
  apply_failed: "Apply failed",
};

function formatReviewReason(reason: string): string {
  if (!reason) return "-";
  return REVIEW_REASON_LABELS[reason] ?? reason;
}

function formatDateTime(value: string): string {
  return value ? new Date(value).toLocaleString() : "-";
}

function getErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof PlatformApiError || err instanceof Error) {
    return err.message;
  }
  return fallback;
}

/**
 * Staff review queue for bank transfer notices that could not be applied
 * automatically. Follows the Astryx incident-console master-detail queue +
 * docked inspector pattern.
 */
export function PaymentNotices({ userRole }: PaymentNoticesProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole || me?.role;

  const { data: notices, isLoading, isFetching, isError, error, refetch } = usePaymentNotices();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const {
    data: detail,
    isLoading: detailLoading,
    isError: detailIsError,
    error: detailError,
    refetch: refetchDetail,
  } = usePaymentNotice(selectedId);

  const resolveNotice = useResolvePaymentNotice();
  const dismissNotice = useDismissPaymentNotice();

  // Match dialog state
  const [matchDialogOpen, setMatchDialogOpen] = useState(false);
  const [selectedCandidate, setSelectedCandidate] = useState<NoticeCandidate | null>(null);
  const [matchError, setMatchError] = useState<string | null>(null);

  // Dismiss dialog state
  const [dismissDialogOpen, setDismissDialogOpen] = useState(false);
  const [dismissError, setDismissError] = useState<string | null>(null);

  // Role gating: Only billing and system_admin roles can view the payment notices queue
  if (effectiveRole && effectiveRole !== "billing" && effectiveRole !== "system_admin") {
    return (
      <div role="region" aria-label="Not found">
        <ErrorState
          title="Not found"
          message="The page you requested was not found or you do not have permission to view it."
        />
      </div>
    );
  }

  const handleOpenMatchDialog = (candidate: NoticeCandidate) => {
    setSelectedCandidate(candidate);
    setMatchError(null);
    setMatchDialogOpen(true);
  };

  const handleConfirmMatch = async (reason: string) => {
    const noticeId = detail?.id ?? selectedId;
    if (!noticeId || !selectedCandidate) return;
    setMatchError(null);
    try {
      await resolveNotice.mutateAsync({
        noticeId,
        orderId: selectedCandidate.order_id,
        reason,
      });
      setMatchDialogOpen(false);
      setSelectedCandidate(null);
      setSelectedId(null);
    } catch (err: unknown) {
      setMatchError(getErrorMessage(err, "Failed to match payment notice"));
    }
  };

  const handleOpenDismissDialog = () => {
    setDismissError(null);
    setDismissDialogOpen(true);
  };

  const handleConfirmDismiss = async (reason: string) => {
    const noticeId = detail?.id ?? selectedId;
    if (!noticeId) return;
    setDismissError(null);
    try {
      await dismissNotice.mutateAsync({
        noticeId,
        reason,
      });
      setDismissDialogOpen(false);
      setSelectedId(null);
    } catch (err: unknown) {
      setDismissError(getErrorMessage(err, "Failed to dismiss payment notice"));
    }
  };

  const queueCount = notices?.length ?? 0;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Payment notices"
        meta={
          !isLoading && !isError ? (
            <span className="tabular-nums">
              {queueCount} {queueCount === 1 ? "notice" : "notices"} awaiting review
            </span>
          ) : undefined
        }
        description="Match incoming bank transfers to pending orders or dismiss unmatched notices."
        actions={
          queueCount > 0 ? (
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
              <span>Refresh</span>
            </Button>
          ) : undefined
        }
      />

      {isLoading && <LoadingState message="Loading payment notices..." />}

      {isError && (
        <ErrorState
          title="Failed to load payment notices"
          message={getErrorMessage(error, "An error occurred while loading payment notices.")}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && (!notices || notices.length === 0) && (
        <EmptyState
          icon={<Inbox className="size-4" />}
          title="No payment notices need review"
          description="Incoming transfers that cannot be matched automatically will appear in this queue."
          actions={
            <Button variant="outline" size="sm" onClick={() => refetch()}>
              Refresh
            </Button>
          }
        />
      )}

      {!isLoading && !isError && notices && notices.length > 0 && (
        <div className="grid grid-cols-1 items-start gap-6 xl:grid-cols-[minmax(0,1fr)_27rem]">
          {/* Queue list */}
          <Card padding={0} className="overflow-x-auto">
            <Table density="compact" dividers="rows" hasHover>
              <TableHeader>
                <TableRow isHeaderRow>
                  <TableHeaderCell>Received</TableHeaderCell>
                  <TableHeaderCell>Transfer code</TableHeaderCell>
                  <TableHeaderCell className="text-right">Amount</TableHeaderCell>
                  <TableHeaderCell>State</TableHeaderCell>
                  <TableHeaderCell>Reason</TableHeaderCell>
                  <TableHeaderCell className="text-right">Action</TableHeaderCell>
                </TableRow>
              </TableHeader>
              <TableBody>
                {notices.map((notice: Notice) => {
                  const isSelected = selectedId === notice.id;
                  return (
                    <TableRow
                      key={notice.id}
                      data-state={isSelected ? "selected" : undefined}
                    >
                      <TableCell>
                        <Text type="supporting" hasTabularNumbers>
                          {formatDateTime(notice.received_at)}
                        </Text>
                      </TableCell>
                      <TableCell>
                        <Text type="code" weight="semibold">
                          {notice.transfer_code || "-"}
                        </Text>
                      </TableCell>
                      <TableCell className="text-right">
                        <Text type="body" weight="medium" hasTabularNumbers>
                          {formatMinorAmount(notice.amount, notice.currency)}
                        </Text>
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={notice.state} />
                      </TableCell>
                      <TableCell>
                        <Text type="body" color="secondary">
                          {formatReviewReason(notice.review_reason)}
                        </Text>
                      </TableCell>
                      <TableCell className="text-right">
                        <Button
                          variant={isSelected ? "secondary" : "outline"}
                          size="sm"
                          onClick={() => setSelectedId(notice.id)}
                        >
                          View
                        </Button>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </Card>

          {/* Docked Inspector Rail */}
          <aside aria-label="Notice details">
            <Card padding={5}>
              {selectedId === null ? (
                <EmptyState
                  isCompact
                  icon={<Inbox className="size-5" aria-hidden="true" />}
                  title="No notice selected"
                  description="Select a payment notice from the queue to inspect candidate orders."
                />
              ) : detailLoading ? (
                <LoadingState message="Loading payment notice..." />
              ) : detailIsError || !detail ? (
                <ErrorState
                  title="Failed to load payment notice"
                  message={getErrorMessage(detailError, "An error occurred while loading the payment notice.")}
                  onRetry={refetchDetail}
                />
              ) : (
                <div className="space-y-5">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 space-y-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <Heading level={2}>Notice details</Heading>
                        <StatusBadge status={detail.state} />
                      </div>
                      <Text type="code" color="secondary" as="p">
                        {detail.transfer_code || "-"}
                      </Text>
                    </div>
                    <div className="flex shrink-0 items-center gap-1.5">
                      {detail.state === "needs_review" && (
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={handleOpenDismissDialog}
                        >
                          Dismiss
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label="Close"
                        onClick={() => setSelectedId(null)}
                      >
                        <X className="size-4" aria-hidden="true" />
                      </Button>
                    </div>
                  </div>

                  <Divider />

                  <MetadataList columns={2}>
                    <MetadataListItem label="Amount" tabular>
                      {formatMinorAmount(detail.amount, detail.currency)}
                    </MetadataListItem>
                    <MetadataListItem label="Provider">
                      {detail.provider || "-"}
                    </MetadataListItem>
                    <MetadataListItem label="Reason">
                      {formatReviewReason(detail.review_reason)}
                    </MetadataListItem>
                    <MetadataListItem label="Received" tabular>
                      <Text type="supporting">{formatDateTime(detail.received_at)}</Text>
                    </MetadataListItem>
                    <MetadataListItem
                      label="Provider event ID"
                      mono
                      className="break-all sm:col-span-2"
                    >
                      {detail.provider_event_id || "-"}
                    </MetadataListItem>
                    {detail.order_id && (
                      <MetadataListItem
                        label="Order ID"
                        mono
                        className="break-all sm:col-span-2"
                      >
                        {detail.order_id}
                      </MetadataListItem>
                    )}
                  </MetadataList>

                  <Divider />

                  <div className="space-y-3">
                    <div className="flex items-baseline justify-between gap-2">
                      <Heading level={3}>Candidate orders</Heading>
                      <Text type="supporting" hasTabularNumbers>
                        {detail.candidates.length}
                      </Text>
                    </div>

                    {detail.candidates.length === 0 ? (
                      <EmptyState isCompact title="No matching orders" />
                    ) : (
                      <ul className="divide-y divide-surface-border rounded-lg border border-surface-border bg-page-canvas/50">
                        {detail.candidates.map((candidate: NoticeCandidate) => (
                          <li key={candidate.order_id} className="space-y-2.5 p-3">
                            <div className="flex items-start justify-between gap-2">
                              <div className="min-w-0 space-y-0.5">
                                <div className="flex flex-wrap items-center gap-2">
                                  <Text type="code" weight="semibold">
                                    {candidate.transfer_code || "-"}
                                  </Text>
                                  <Badge variant="neutral" label={candidate.match} />
                                </div>
                                <Text type="body" weight="medium" hasTabularNumbers as="div">
                                  {formatMinorAmount(
                                    candidate.amount + candidate.vat_amount,
                                    candidate.currency,
                                  )}
                                </Text>
                              </div>
                              {detail.state === "needs_review" && (
                                <Button
                                  size="sm"
                                  onClick={() => handleOpenMatchDialog(candidate)}
                                >
                                  Match
                                </Button>
                              )}
                            </div>

                            <dl className="grid grid-cols-1 gap-1.5 text-caption">
                              <div className="flex items-baseline justify-between gap-2">
                                <dt className="shrink-0 text-muted-foreground">Order</dt>
                                <dd
                                  className="min-w-0 truncate font-mono text-foreground"
                                  title={candidate.order_id}
                                >
                                  {candidate.order_id}
                                </dd>
                              </div>
                              <div className="flex items-baseline justify-between gap-2">
                                <dt className="shrink-0 text-muted-foreground">Workspace</dt>
                                <dd
                                  className="min-w-0 truncate font-mono text-muted-foreground"
                                  title={candidate.workspace_id}
                                >
                                  {candidate.workspace_id}
                                </dd>
                              </div>
                              <div className="flex items-baseline justify-between gap-2">
                                <dt className="shrink-0 text-muted-foreground">Created</dt>
                                <dd className="tabular-nums text-muted-foreground">
                                  {formatDateTime(candidate.created_at)}
                                </dd>
                              </div>
                            </dl>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                </div>
              )}
            </Card>
          </aside>
        </div>
      )}

      {/* Match Reason Dialog */}
      <ConfirmReasonDialog
        open={matchDialogOpen}
        onOpenChange={(open) => {
          setMatchDialogOpen(open);
          if (!open) {
            setSelectedCandidate(null);
            setMatchError(null);
          }
        }}
        title="Match payment to order"
        description={
          selectedCandidate
            ? `Match this payment notice to order ${selectedCandidate.order_id} (${formatMinorAmount(selectedCandidate.amount + selectedCandidate.vat_amount, selectedCandidate.currency)}).`
            : ""
        }
        confirmLabel="Match payment"
        isPending={resolveNotice.isPending}
        error={matchError}
        onConfirm={handleConfirmMatch}
      />

      {/* Dismiss Reason Dialog */}
      <ConfirmReasonDialog
        open={dismissDialogOpen}
        onOpenChange={(open) => {
          setDismissDialogOpen(open);
          if (!open) {
            setDismissError(null);
          }
        }}
        title="Dismiss payment notice"
        description="Dismiss this payment notice from the review queue without matching it to an order."
        confirmLabel="Dismiss notice"
        confirmVariant="destructive"
        isPending={dismissNotice.isPending}
        error={dismissError}
        onConfirm={handleConfirmDismiss}
      />
    </div>
  );
}
