import { useId, useMemo, useState, type FormEvent } from "react";
import { Heading } from "@astryxdesign/core/Heading";
import { HStack, Layout, LayoutContent, LayoutHeader } from "@astryxdesign/core/Layout";
import { Section } from "@astryxdesign/core/Section";
import {
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow } from "@astryxdesign/core/Table";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@astryxdesign/core/Button";
import { Banner } from "@astryxdesign/core/Banner";
import { TextInput } from "@astryxdesign/core/TextInput";
import { TextArea } from "@astryxdesign/core/TextArea";
import { Dialog, DialogHeader } from "@astryxdesign/core/Dialog";
import { Selector } from "@astryxdesign/core/Selector";
import { Lock, Plus, RefreshCw } from "@multica/ui/icons";
import { RowActions } from "../components/RowActions";
import { EmptyState } from "../components/EmptyState";
import { ErrorState } from "../components/ErrorState";
import { LoadingState } from "../components/LoadingState";
import { StatusBadge } from "../components/StatusBadge";
import { getLegalHoldErrorMessage } from "../api/client";
import {
  useLegalHolds,
  useMe,
  usePlaceLegalHold,
  useReleaseLegalHold } from "../api/hooks";
import type { LegalHold, LegalHoldScopeKind } from "../api/schemas";
import type { StaffRole } from "../components/types";
import { t } from "../i18n";
import { PageHeader } from "../components/PageHeader";

export interface LegalHoldsProps {
  userRole?: StaffRole;
}

const SCOPE_OPTIONS = [
  { value: "tenant", label: "Tenant" },
  { value: "workspace", label: "Workspace" },
  { value: "account", label: "Account" },
] as const;

const STATUS_FILTERS = [
  { value: "all", label: "All holds" },
  { value: "active", label: "Active holds" },
  { value: "released", label: "Released holds" },
] as const;

export function LegalHolds({ userRole }: LegalHoldsProps) {
  const { data: me } = useMe();
  const effectiveRole = userRole ?? me?.role;
  const canAccess = !effectiveRole || effectiveRole === "system_admin";

  const query = useLegalHolds(canAccess);
  const placeHold = usePlaceLegalHold();
  const releaseHold = useReleaseLegalHold();

  const [statusFilter, setStatusFilter] = useState<"all" | "active" | "released">("all");
  const [placeDialogOpen, setPlaceDialogOpen] = useState(false);
  const [scopeKind, setScopeKind] = useState<Exclude<LegalHoldScopeKind, "unknown">>("tenant");
  const [scopeId, setScopeId] = useState("");
  const [caseRef, setCaseRef] = useState("");
  const [reason, setReason] = useState("");
  const [placeError, setPlaceError] = useState<string | null>(null);

  const [releaseTarget, setReleaseTarget] = useState<LegalHold | null>(null);
  const [releaseError, setReleaseError] = useState<string | null>(null);

  const scopeIdInputId = useId();
  const caseRefInputId = useId();
  const reasonInputId = useId();

  const holds = useMemo(() => {
    const items = query.data ?? [];
    if (statusFilter === "active") return items.filter((item) => !item.released);
    if (statusFilter === "released") return items.filter((item) => item.released);
    return items;
  }, [query.data, statusFilter]);

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

  const openPlaceDialog = () => {
    setScopeKind("tenant");
    setScopeId("");
    setCaseRef("");
    setReason("");
    setPlaceError(null);
    setPlaceDialogOpen(true);
  };

  const canSubmitPlace =
    scopeId.trim() !== "" &&
    caseRef.trim() !== "" &&
    reason.trim() !== "" &&
    !placeHold.isPending;

  const handlePlaceSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!canSubmitPlace) return;
    setPlaceError(null);
    try {
      await placeHold.mutateAsync({
        scope_kind: scopeKind,
        scope_id: scopeId.trim(),
        case_ref: caseRef.trim(),
        reason: reason.trim() });
      setPlaceDialogOpen(false);
    } catch (err) {
      setPlaceError(getLegalHoldErrorMessage(err, t("Failed to place legal hold")));
    }
  };

  const handleReleaseConfirm = async () => {
    if (!releaseTarget) return;
    setReleaseError(null);
    try {
      await releaseHold.mutateAsync({ id: releaseTarget.id });
      setReleaseTarget(null);
    } catch (err) {
      setReleaseError(getLegalHoldErrorMessage(err, t("Failed to release legal hold")));
    }
  };

  return (
    <Layout height="auto">
      <LayoutHeader padding={0}>
          <PageHeader
            title={t("Legal holds")}
            actions={
              <>
                <HStack gap={2} align="center">
                            <Button
                              type="button"
                              label={t("Refresh")}
                              variant="secondary"
                              size="sm"
                              icon={<RefreshCw className="size-4" aria-hidden="true" />}
                              isLoading={query.isFetching}
                              onClick={() => query.refetch()}
                              aria-busy={query.isFetching || undefined}
                              isDisabled={query.isFetching}
                            />
                            <Button
                              type="button"
                              label={t("Place legal hold")}
                              variant="primary"
                              size="sm"
                              icon={<Plus className="size-4" aria-hidden="true" />}
                              onClick={openPlaceDialog}
                            />
                          </HStack>
              </>
            }
          />
        </LayoutHeader>

      <LayoutContent padding={0}>
        <div className="space-y-6">
          <Section padding={4}>
            <div className="space-y-4">
              <Heading level={3}>{t("Holds")}</Heading>
              <HStack wrap="wrap" gap={3} align="center">
                <div className="w-56">
                  <Selector
                    label={t("Hold status")}
                    isLabelHidden
                    width="100%"
                    options={STATUS_FILTERS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
                    value={statusFilter}
                    onChange={(val) => {
                      if (val === "all" || val === "active" || val === "released") {
                        setStatusFilter(val);
                      }
                    }}
                  />
                </div>
              </HStack>

              {query.isLoading && <LoadingState message={t("Loading legal holds...")} />}

              {query.isError && (
                <ErrorState
                  title={t("Failed to load legal holds")}
                  message={query.error instanceof Error ? query.error.message : undefined}
                  onRetry={query.refetch}
                />
              )}

              {!query.isLoading && !query.isError && holds.length === 0 && (
                <EmptyState
                  icon={<Lock className="size-5" aria-hidden="true" />}
                  title={t("No legal holds")}
                  description={t("Active preservation holds block immediate deletion of the covered entity.")}
                />
              )}

              {!query.isLoading && !query.isError && holds.length > 0 && (
                <Table density="compact" dividers="rows" hasHover>
                  <TableHeader>
                    <TableRow isHeaderRow>
                      <TableHeaderCell>{t("Case reference")}</TableHeaderCell>
                      <TableHeaderCell>{t("Scope")}</TableHeaderCell>
                      <TableHeaderCell>{t("Reason")}</TableHeaderCell>
                      <TableHeaderCell>{t("Placed by")}</TableHeaderCell>
                      <TableHeaderCell>{t("Status")}</TableHeaderCell>
                      <TableHeaderCell className="text-right">{t("Actions")}</TableHeaderCell>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {holds.map((hold) => (
                      <TableRow key={hold.id}>
                        <TableCell>
                          <span className="font-mono text-body text-foreground">
                            {hold.case_ref || "—"}
                          </span>
                        </TableCell>
                        <TableCell>
                          <div className="min-w-0">
                            <div className="font-medium text-foreground capitalize">
                              {hold.scope_kind}
                            </div>
                            <div className="font-mono text-caption text-muted-foreground">
                              {hold.scope_id}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Text type="supporting">{hold.reason || "—"}</Text>
                        </TableCell>
                        <TableCell>
                          <Text type="supporting">{hold.placed_by || "—"}</Text>
                        </TableCell>
                        <TableCell>
                          <StatusBadge
                            status={hold.released ? "released" : "legal_hold"}
                            label={hold.released ? t("Released") : t("Active")}
                          />
                        </TableCell>
                        <TableCell className="text-right">
                          {hold.released ? (
                            <Text type="supporting">—</Text>
                          ) : (
                            <RowActions
                              label={t("legal hold {0}", hold.id)}
                              actions={[
                                {
                                  label: t("Release"),
                                  onSelect: () => {
                                    setReleaseTarget(hold);
                                    setReleaseError(null);
                                  } },
                              ]}
                            />
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </div>
          </Section>
        </div>
      </LayoutContent>

      <Dialog
        isOpen={placeDialogOpen}
        onOpenChange={(open) => {
          if (!open && !placeHold.isPending) {
            setPlaceDialogOpen(false);
          }
        }}
        purpose="form"
        width={512}
        maxHeight="85vh"
      >

          <form onSubmit={handlePlaceSubmit} className="space-y-4">
            <DialogHeader title={t("Place legal hold")} subtitle={t("Prevents permanent deletion of the target tenant, workspace, or account until released.")} />

            {placeError && (
              <Banner
                status="error"
                role="alert"
                title={placeError}
              />
            )}

            <Selector
              label={t("Scope kind")}
              width="100%"
              options={SCOPE_OPTIONS.map((opt) => ({ value: opt.value, label: t(opt.label) }))}
              value={scopeKind}
              onChange={(val) => {
                  if (val === "tenant" || val === "workspace" || val === "account") {
                    setScopeKind(val);
                  }
                }}
            />

            <TextInput
              label={t("Scope ID")}
              id={scopeIdInputId}
              isRequired
              width="100%"
              value={scopeId}
              onChange={(value) => setScopeId(value)}
              placeholder={t("UUID of the tenant, workspace, or account")}
              isDisabled={placeHold.isPending}
            />

            <TextInput
              label={t("Case reference")}
              id={caseRefInputId}
              isRequired
              width="100%"
              value={caseRef}
              onChange={(value) => setCaseRef(value)}
              placeholder={t("CASE-2026-001")}
              isDisabled={placeHold.isPending}
            />

            <TextArea
              label={t("Reason")}
              id={reasonInputId}
              isRequired
              width="100%"
              value={reason}
              onChange={(value) => setReason(value)}
              placeholder={t("Explain the litigation or compliance preservation basis...")}
              isDisabled={placeHold.isPending}
              rows={3}
            />

            <div className="flex justify-end gap-2">
              <Button
                type="button"
                label={t("Cancel")}
                variant="secondary"
                onClick={() => setPlaceDialogOpen(false)}
                isDisabled={placeHold.isPending}
              />
              <Button
                type="submit"
                label={t("Place hold")}
                variant="primary"
                isLoading={placeHold.isPending}
                aria-busy={placeHold.isPending || undefined}
                isDisabled={!canSubmitPlace}
              />
            </div>
          </form>

      </Dialog>

      <Dialog
        isOpen={releaseTarget !== null}
        onOpenChange={(open) => {
          if (!open && !releaseHold.isPending) {
            setReleaseTarget(null);
            setReleaseError(null);
          }
        }}
        purpose="info"
        width={448}
      >

          <DialogHeader
            title={t("Release legal hold")}
            subtitle={
              releaseTarget ? (
                <>
                  {t("Release hold")} {releaseTarget.case_ref} {t("covering")} {releaseTarget.scope_kind}{" "}
                  {releaseTarget.scope_id}?
                </>
              ) : undefined
            }
          />

          {releaseError && (
            <Banner
              status="error"
              role="alert"
              title={releaseError}
            />
          )}

          <div className="flex justify-end gap-2">
            <Button
              type="button"
              label={t("Cancel")}
              variant="secondary"
              onClick={() => setReleaseTarget(null)}
              isDisabled={releaseHold.isPending}
            />
            <Button
              type="button"
              label={t("Release hold")}
              variant="primary"
              isLoading={releaseHold.isPending}
              aria-busy={releaseHold.isPending || undefined}
              onClick={handleReleaseConfirm}
              isDisabled={releaseHold.isPending}
            />
          </div>

      </Dialog>
    </Layout>
  );
}
