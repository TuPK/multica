import { useId, useState, type FormEvent } from "react";
import { Field } from "@astryxdesign/core/Field";
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Spinner } from "@multica/ui/components/ui/spinner";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { AlertCircle, ArrowUpCircle, RefreshCw } from "@multica/ui/icons";
import { RowActions } from "../components/RowActions";
import { ConfirmReasonDialog } from "../components/ConfirmReasonDialog";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { StatusBadge } from "../components/StatusBadge";
import { getApiErrorMessage } from "../api/client";
import {
  useApproveQuotaRequest,
  useDenyQuotaRequest,
  useMe,
  useQuotaRequests,
} from "../api/hooks";
import type { QuotaRequest } from "../api/schemas";
import type { StaffRole } from "../components/types";
import { t } from "../i18n";
import { PageHeader } from "../components/PageHeader";

export interface QuotaRequestsProps {
  userRole?: StaffRole;
}

const STATE_OPTIONS = [
  { value: "all", label: "All states" },
  { value: "pending", label: "Pending" },
  { value: "approved", label: "Approved" },
  { value: "denied", label: "Denied" },
] as const;

export function QuotaRequests({ userRole }: QuotaRequestsProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;
  const canAccess =
    !effectiveRole ||
    effectiveRole === "system_admin" ||
    effectiveRole === "admin" ||
    effectiveRole === "billing";
  const canDecide = effectiveRole === "system_admin";

  const [stateFilter, setStateFilter] = useState("");
  const [approveTarget, setApproveTarget] = useState<QuotaRequest | null>(null);
  const [approvedValueInput, setApprovedValueInput] = useState("");
  const [approveReasonInput, setApproveReasonInput] = useState("");
  const [valueFieldError, setValueFieldError] = useState<string | null>(null);
  const [reasonFieldError, setReasonFieldError] = useState<string | null>(null);
  const [approveSubmitError, setApproveSubmitError] = useState<string | null>(null);

  const [denyTarget, setDenyTarget] = useState<QuotaRequest | null>(null);
  const [denySubmitError, setDenySubmitError] = useState<string | null>(null);

  const approvedValueId = useId();
  const approvedValueErrorId = useId();
  const approveReasonId = useId();
  const approveReasonErrorId = useId();

  const query = useQuotaRequests(
    { state: stateFilter || undefined },
    canAccess,
  );
  const approveMutation = useApproveQuotaRequest();
  const denyMutation = useDenyQuotaRequest();

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

  const requests = query.data ?? [];

  const openApproveDialog = (req: QuotaRequest) => {
    setApproveTarget(req);
    setApprovedValueInput(String(req.requested_value));
    setApproveReasonInput("");
    setValueFieldError(null);
    setReasonFieldError(null);
    setApproveSubmitError(null);
  };

  const handleApproveSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!approveTarget || !canDecide) return;

    setValueFieldError(null);
    setReasonFieldError(null);
    setApproveSubmitError(null);

    const currentVal = approveTarget.current_value ?? 0;
    const parsedValue = Number(approvedValueInput.trim());
    if (
      approvedValueInput.trim() === "" ||
      !Number.isInteger(parsedValue) ||
      parsedValue <= currentVal
    ) {
      setValueFieldError(
        t("Approved value must be a whole number above {0}.", currentVal),
      );
      return;
    }

    const trimmedReason = approveReasonInput.trim();
    if (trimmedReason === "") {
      setReasonFieldError(t("Reason is required."));
      return;
    }

    try {
      await approveMutation.mutateAsync({
        id: approveTarget.id,
        value: parsedValue,
        reason: trimmedReason,
      });
      setApproveTarget(null);
    } catch (err) {
      setApproveSubmitError(getApiErrorMessage(err, t("Failed to approve quota request")));
    }
  };

  const handleDenyConfirm = async (reason: string) => {
    if (!denyTarget || !canDecide) return;
    setDenySubmitError(null);
    try {
      await denyMutation.mutateAsync({ id: denyTarget.id, reason });
      setDenyTarget(null);
    } catch (err) {
      setDenySubmitError(getApiErrorMessage(err, t("Failed to deny quota request")));
    }
  };

  return (
    <Layout height="auto">
      <LayoutHeader padding={0}>
          <PageHeader
            title={t("Quota requests")}
            actions={
              <>
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
              </>
            }
          />
        </LayoutHeader>

      <LayoutContent padding={0}>
        <div className="space-y-6">
          <Section padding={4}>
            <div className="space-y-4">
              <Heading level={3}>{t("Requests")}</Heading>
              <HStack wrap="wrap" gap={3} align="center">
                <div className="w-56">
                  <Select
                    items={[...STATE_OPTIONS]}
                    value={stateFilter || "all"}
                    onValueChange={(val) => setStateFilter(!val || val === "all" ? "" : val)}
                  >
                    <SelectTrigger aria-label={t("State")} className="w-full">
                      <SelectValue>
                        {t(STATE_OPTIONS.find((item) => item.value === (stateFilter || "all"))
                          ?.label ?? t("All states"))}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      {STATE_OPTIONS.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {t(item.label)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </HStack>

              {query.isLoading && <LoadingState message={t("Loading quota requests...")} />}

              {query.isError && (
                <ErrorState
                  title={t("Failed to load quota requests")}
                  message={query.error instanceof Error ? query.error.message : undefined}
                  onRetry={query.refetch}
                />
              )}

              {!query.isLoading && !query.isError && requests.length === 0 && (
                <EmptyState
                  icon={<ArrowUpCircle className="size-5" aria-hidden="true" />}
                  title={t("No quota requests")}
                  description={t("No workspace quota requests match the selected state.")}
                />
              )}

              {!query.isLoading && !query.isError && requests.length > 0 && (
                <Table density="compact" dividers="rows" hasHover>
                  <TableHeader>
                    <TableRow isHeaderRow>
                      <TableHeaderCell>{t("Gate")}</TableHeaderCell>
                      <TableHeaderCell>{t("Workspace")}</TableHeaderCell>
                      <TableHeaderCell>{t("Quota")}</TableHeaderCell>
                      <TableHeaderCell>{t("Reason")}</TableHeaderCell>
                      <TableHeaderCell>{t("Requested by")}</TableHeaderCell>
                      <TableHeaderCell>{t("State")}</TableHeaderCell>
                      {canDecide && (
                        <TableHeaderCell className="text-right">{t("Actions")}</TableHeaderCell>
                      )}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {requests.map((req) => (
                      <TableRow key={req.id}>
                        <TableCell>
                          <span className="font-mono text-body text-foreground">{req.gate}</span>
                        </TableCell>
                        <TableCell>
                          <div className="min-w-0">
                            <div className="font-mono text-caption text-foreground">
                              {req.workspace_id}
                            </div>
                            {req.tenant_id && (
                              <div className="font-mono text-caption text-muted-foreground">
                                {req.tenant_id}
                              </div>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          <Text type="body" hasTabularNumbers>
                            {req.current_value ?? "—"} {"->"} {req.requested_value ?? "—"}
                            {req.approved_value !== null ? t(" (approved {0})", req.approved_value) : ""}
                          </Text>
                        </TableCell>
                        <TableCell>
                          <div className="space-y-0.5">
                            <Text type="supporting">{req.reason || "—"}</Text>
                            {req.decision_reason && (
                              <Text type="supporting" color="secondary" as="div">
                                {t("Decision:")} {req.decision_reason}
                              </Text>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          <Text type="supporting">
                            {req.requested_by_label || req.requested_by || "—"}
                          </Text>
                        </TableCell>
                        <TableCell>
                          <StatusBadge status={req.state} />
                        </TableCell>
                        {canDecide && (
                          <TableCell className="text-right">
                            {req.state === "pending" ? (
                              <RowActions
                                label={t("quota request for {0}", req.workspace_id)}
                                actions={[
                                  { label: t("Approve"), primary: true, onSelect: () => openApproveDialog(req) },
                                  {
                                    label: t("Deny"),
                                    destructive: true,
                                    onSelect: () => {
                                      setDenyTarget(req);
                                      setDenySubmitError(null);
                                    },
                                  },
                                ]}
                              />
                            ) : (
                              <Text type="supporting">—</Text>
                            )}
                          </TableCell>
                        )}
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}

              {query.hasNextPage && (
                <div className="flex justify-center pt-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => query.fetchNextPage()}
                    disabled={query.isFetchingNextPage}
                    aria-busy={query.isFetchingNextPage || undefined}
                  >
                    {query.isFetchingNextPage && <Spinner />}
                    {t("Load more")}
                  </Button>
                </div>
              )}
            </div>
          </Section>
        </div>
      </LayoutContent>

      <Dialog
        open={approveTarget !== null}
        onOpenChange={(open) => {
          if (!open && !approveMutation.isPending) {
            setApproveTarget(null);
          }
        }}
      >
        <DialogContent className="sm:max-w-lg max-h-[85vh] overflow-y-auto">
          <form onSubmit={handleApproveSubmit} className="space-y-4">
            <DialogHeader>
              <DialogTitle>{t("Approve quota request")}</DialogTitle>
              {approveTarget && (
                <DialogDescription>
                  {t("Gate")} {approveTarget.gate}{t(": current")} {approveTarget.current_value}{t(", requested")}{" "}
                  {approveTarget.requested_value}{t(". You may approve a lower increase.")}
                </DialogDescription>
              )}
            </DialogHeader>

            {approveSubmitError && (
              <Alert variant="destructive">
                <AlertCircle className="size-4" />
                <AlertDescription>{approveSubmitError}</AlertDescription>
              </Alert>
            )}

            <Field
              label={t("Approved value")}
              inputID={approvedValueId}
              isRequired
              status={
                valueFieldError
                  ? { type: "error", message: valueFieldError, messageID: approvedValueErrorId }
                  : undefined
              }
              width="100%"
            >
              <Input
                id={approvedValueId}
                type="number"
                step={1}
                min={approveTarget ? (approveTarget.current_value ?? 0) + 1 : 1}
                value={approvedValueInput}
                onChange={(e) => {
                  setApprovedValueInput(e.target.value);
                  setValueFieldError(null);
                }}
                aria-describedby={valueFieldError ? approvedValueErrorId : undefined}
                aria-invalid={valueFieldError ? true : undefined}
                disabled={approveMutation.isPending}
              />
            </Field>

            <Field
              label={t("Reason")}
              inputID={approveReasonId}
              isRequired
              status={
                reasonFieldError
                  ? { type: "error", message: reasonFieldError, messageID: approveReasonErrorId }
                  : undefined
              }
              width="100%"
            >
              <Textarea
                id={approveReasonId}
                value={approveReasonInput}
                onChange={(e) => {
                  setApproveReasonInput(e.target.value);
                  setReasonFieldError(null);
                }}
                placeholder={t("Explain the approved limit and duration...")}
                aria-describedby={reasonFieldError ? approveReasonErrorId : undefined}
                aria-invalid={reasonFieldError ? true : undefined}
                disabled={approveMutation.isPending}
                rows={3}
              />
            </Field>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setApproveTarget(null)}
                disabled={approveMutation.isPending}
              >
                {t("Cancel")}
              </Button>
              <Button
                type="submit"
                variant="default"
                disabled={approveMutation.isPending}
                aria-busy={approveMutation.isPending || undefined}
              >
                {approveMutation.isPending && <Spinner />}
                {t("Approve request")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmReasonDialog
        open={denyTarget !== null}
        onOpenChange={(open) => {
          if (!open && !denyMutation.isPending) {
            setDenyTarget(null);
            setDenySubmitError(null);
          }
        }}
        title={t("Deny quota request")}
        description={
          denyTarget
            ? t("Deny {0} request for {1} (current {2}).", denyTarget.gate, denyTarget.requested_value, denyTarget.current_value)
            : ""
        }
        confirmLabel={t("Deny request")}
        confirmVariant="destructive"
        error={denySubmitError}
        onConfirm={handleDenyConfirm}
      />
    </Layout>
  );
}
