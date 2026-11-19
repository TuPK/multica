import { useState } from "react";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@multica/ui/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@multica/ui/components/ui/table";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
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

/** Staff review queue for bank transfer notices that could not be applied automatically. */
export function PaymentNotices({ userRole }: PaymentNoticesProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole || me?.role;

  const { data: notices, isLoading, isError, error, refetch } = usePaymentNotices();
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

  // Role gating: Only billing and admin roles can view the payment notices queue
  if (effectiveRole && effectiveRole !== "billing" && effectiveRole !== "admin") {
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

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-heading text-title-lg font-bold">Payment notices</h1>
        <p className="text-body text-muted-foreground">
          Match incoming bank transfers to pending orders or dismiss unmatched notices.
        </p>
      </div>

      {isLoading && <LoadingState message="Loading payment notices..." />}

      {isError && (
        <ErrorState
          title="Failed to load payment notices"
          message={getErrorMessage(error, "An error occurred while loading payment notices.")}
          onRetry={refetch}
        />
      )}

      {!isLoading && !isError && (!notices || notices.length === 0) && (
        <EmptyState title="No payment notices need review" />
      )}

      {!isLoading && !isError && notices && notices.length > 0 && (
        <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Received</TableHead>
                <TableHead>Transfer code</TableHead>
                <TableHead>Amount</TableHead>
                <TableHead>Reason</TableHead>
                <TableHead className="text-right">Action</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {notices.map((notice: Notice) => {
                const isSelected = selectedId === notice.id;
                return (
                  <TableRow key={notice.id} className={isSelected ? "bg-muted/50" : undefined}>
                    <TableCell className="text-caption text-muted-foreground">
                      {formatDateTime(notice.received_at)}
                    </TableCell>
                    <TableCell className="font-mono text-body font-semibold">
                      {notice.transfer_code || "-"}
                    </TableCell>
                    <TableCell className="font-medium tabular-nums">
                      {formatMinorAmount(notice.amount, notice.currency)}
                    </TableCell>
                    <TableCell>{formatReviewReason(notice.review_reason)}</TableCell>
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
        </div>
      )}

      {selectedId !== null && (
        <Card>
          {detailLoading && (
            <CardContent className="pt-6">
              <LoadingState message="Loading payment notice..." />
            </CardContent>
          )}

          {!detailLoading && (detailIsError || !detail) && (
            <CardContent className="pt-6">
              <ErrorState
                title="Failed to load payment notice"
                message={getErrorMessage(detailError, "An error occurred while loading the payment notice.")}
                onRetry={refetchDetail}
              />
            </CardContent>
          )}

          {!detailLoading && !detailIsError && detail && (
            <>
              <CardHeader className="flex flex-row items-start justify-between gap-4">
                <CardTitle>Notice details</CardTitle>
                <div className="flex items-center gap-2">
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
                    size="sm"
                    onClick={() => setSelectedId(null)}
                  >
                    Close
                  </Button>
                </div>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="grid grid-cols-2 gap-4 text-body sm:grid-cols-4">
                  <div>
                    <span className="block text-caption text-muted-foreground">State</span>
                    <div className="mt-1">
                      <StatusBadge status={detail.state} />
                    </div>
                  </div>
                  <div>
                    <span className="block text-caption text-muted-foreground">Provider</span>
                    <span className="mt-1 block font-medium">{detail.provider || "-"}</span>
                  </div>
                  <div>
                    <span className="block text-caption text-muted-foreground">Provider event ID</span>
                    <span className="mt-1 block font-mono text-caption break-all">
                      {detail.provider_event_id || "-"}
                    </span>
                  </div>
                  <div>
                    <span className="block text-caption text-muted-foreground">Transfer code</span>
                    <span className="mt-1 block font-mono font-medium">
                      {detail.transfer_code || "-"}
                    </span>
                  </div>
                  <div>
                    <span className="block text-caption text-muted-foreground">Amount</span>
                    <span className="mt-1 block font-medium tabular-nums">
                      {formatMinorAmount(detail.amount, detail.currency)}
                    </span>
                  </div>
                  <div>
                    <span className="block text-caption text-muted-foreground">Received</span>
                    <span className="mt-1 block text-muted-foreground">
                      {formatDateTime(detail.received_at)}
                    </span>
                  </div>
                  <div>
                    <span className="block text-caption text-muted-foreground">Reason</span>
                    <span className="mt-1 block font-medium">
                      {formatReviewReason(detail.review_reason)}
                    </span>
                  </div>
                  {detail.order_id && (
                    <div>
                      <span className="block text-caption text-muted-foreground">Order ID</span>
                      <span className="mt-1 block font-mono text-caption break-all">
                        {detail.order_id}
                      </span>
                    </div>
                  )}
                </div>

                <div className="space-y-3">
                  <h3 className="font-heading text-title-sm font-semibold">Candidate orders</h3>
                  {detail.candidates.length === 0 ? (
                    <EmptyState title="No matching orders" />
                  ) : (
                    <div className="overflow-x-auto rounded-lg border border-surface-border bg-surface">
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>Order ID</TableHead>
                            <TableHead>Workspace ID</TableHead>
                            <TableHead>Transfer code</TableHead>
                            <TableHead>Total incl. VAT</TableHead>
                            <TableHead>Created</TableHead>
                            <TableHead>Match</TableHead>
                            {detail.state === "needs_review" && (
                              <TableHead className="text-right">Action</TableHead>
                            )}
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {detail.candidates.map((candidate: NoticeCandidate) => (
                            <TableRow key={candidate.order_id}>
                              <TableCell className="font-mono text-caption">
                                {candidate.order_id}
                              </TableCell>
                              <TableCell className="font-mono text-caption">
                                {candidate.workspace_id}
                              </TableCell>
                              <TableCell className="font-mono text-body font-medium">
                                {candidate.transfer_code || "-"}
                              </TableCell>
                              <TableCell className="font-medium tabular-nums">
                                {formatMinorAmount(candidate.amount + candidate.vat_amount, candidate.currency)}
                              </TableCell>
                              <TableCell className="text-caption text-muted-foreground">
                                {formatDateTime(candidate.created_at)}
                              </TableCell>
                              <TableCell>
                                <Badge variant="outline">{candidate.match}</Badge>
                              </TableCell>
                              {detail.state === "needs_review" && (
                                <TableCell className="text-right">
                                  <Button
                                    size="sm"
                                    onClick={() => handleOpenMatchDialog(candidate)}
                                  >
                                    Match
                                  </Button>
                                </TableCell>
                              )}
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </div>
                  )}
                </div>
              </CardContent>
            </>
          )}
        </Card>
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
