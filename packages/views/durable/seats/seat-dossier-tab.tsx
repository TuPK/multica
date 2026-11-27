"use client";

import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { ApiError } from "@multica/core/api";
import {
  DOSSIER_EVIDENCE_REF_MAX,
  DOSSIER_EVIDENCE_TYPES,
  DOSSIER_KINDS,
  DOSSIER_STATUS_FILTERS,
  DOSSIER_TEXT_MAX,
  DOSSIER_WORK_CLASS_MAX,
  useAddSeatDossierEntries,
  useRetireSeatDossierEntry,
  useSeatDossier,
  type DossierEvidenceType,
  type DossierKind,
  type DossierStatusFilter,
  type SeatDossierEntry,
} from "@multica/core/durable/dossier";
import { useWorkspaceId } from "@multica/core/hooks";
import { Plus } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { useT } from "../../i18n";
import { ShadowTrialDialog } from "./shadow-trial-dialog";

function formatCreatedDate(iso: string): string {
  if (!iso) return "";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleDateString();
}

/**
 * The seat dossier section on the agent page (durable spec 9.2): lists entries for the seat,
 * filters by status, adds new entries with required evidence, retires active entries with a
 * reason, and launches the shadow trial dialog.
 */
export function SeatDossierTab({ agentId }: { agentId: string }) {
  const { t } = useT("durable");
  const wsId = useWorkspaceId();

  const [status, setStatus] = useState<DossierStatusFilter>("active");
  const [adding, setAdding] = useState(false);
  const [kind, setKind] = useState<DossierKind>("lesson");
  const [text, setText] = useState("");
  const [workClass, setWorkClass] = useState("");
  const [evidenceType, setEvidenceType] = useState<DossierEvidenceType>("document");
  const [evidenceRef, setEvidenceRef] = useState("");
  const [addError, setAddError] = useState<string | null>(null);

  const [retiringId, setRetiringId] = useState<string | null>(null);
  const [retireReason, setRetireReason] = useState("");
  const [retireError, setRetireError] = useState<string | null>(null);

  const [shadowOpen, setShadowOpen] = useState(false);

  const dossier = useSeatDossier(wsId, agentId, { status });
  const addEntries = useAddSeatDossierEntries(wsId, agentId);
  const retireEntry = useRetireSeatDossierEntry(wsId, agentId);

  const entries = dossier.data ?? [];

  const statusItems = DOSSIER_STATUS_FILTERS.map((value) => ({
    value,
    label: t(($) => $.seats.dossier.status[value]),
  }));
  const kindItems = DOSSIER_KINDS.map((value) => ({
    value,
    label: t(($) => $.seats.dossier.kind[value]),
  }));
  const evidenceTypeItems = DOSSIER_EVIDENCE_TYPES.map((value) => ({
    value,
    label: t(($) => $.seats.dossier.evidence_type[value]),
  }));

  const canAdd = text.trim() !== "" && evidenceRef.trim() !== "" && !addEntries.isPending;

  const resetAddForm = () => {
    setKind("lesson");
    setText("");
    setWorkClass("");
    setEvidenceType("document");
    setEvidenceRef("");
    setAddError(null);
    setAdding(false);
  };

  const handleAddSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!canAdd) return;
    setAddError(null);
    addEntries
      .mutateAsync({
        entries: [
          {
            kind,
            text: text.trim(),
            work_class: workClass.trim(),
            evidence: [{ type: evidenceType, ref: evidenceRef.trim() }],
          },
        ],
      })
      .then(
        () => {
          toast.success(t(($) => $.seats.dossier.added));
          resetAddForm();
        },
        (err: unknown) => {
          const msg =
            err instanceof ApiError && err.status === 409
              ? t(($) => $.seats.dossier.full_error)
              : t(($) => $.seats.dossier.add_failed);
          setAddError(msg);
          toast.error(msg);
        },
      );
  };

  const openRetire = (entry: SeatDossierEntry) => {
    setRetiringId(entry.id);
    setRetireReason("");
    setRetireError(null);
  };

  const cancelRetire = () => {
    setRetiringId(null);
    setRetireReason("");
    setRetireError(null);
  };

  const handleRetireSubmit = (event: FormEvent, entryId: string) => {
    event.preventDefault();
    const trimmed = retireReason.trim();
    if (!trimmed || retireEntry.isPending) return;
    setRetireError(null);
    retireEntry
      .mutateAsync({ entryId, reason: trimmed })
      .then(
        () => {
          toast.success(t(($) => $.seats.dossier.retired));
          cancelRetire();
        },
        () => {
          const msg = t(($) => $.seats.dossier.retire_failed);
          setRetireError(msg);
          toast.error(msg);
        },
      );
  };

  return (
    <section aria-label={t(($) => $.seats.dossier.title)} className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-heading">{t(($) => $.seats.dossier.title)}</h2>
        <div className="flex flex-wrap items-center gap-2">
          <Select
            items={statusItems}
            value={status}
            onValueChange={(next) => {
              if (next) setStatus(next as DossierStatusFilter);
            }}
          >
            <SelectTrigger aria-label={t(($) => $.seats.dossier.status_label)} size="sm" className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {statusItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              if (adding) resetAddForm();
              else setAdding(true);
            }}
          >
            <Plus />
            {t(($) => $.seats.dossier.add)}
          </Button>
          <Button type="button" size="sm" variant="outline" onClick={() => setShadowOpen(true)}>
            {t(($) => $.seats.dossier.start_shadow)}
          </Button>
        </div>
      </div>

      {adding ? (
        <form onSubmit={handleAddSubmit} className="flex flex-col gap-3 rounded-lg border bg-card p-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label>{t(($) => $.seats.dossier.kind_label)}</Label>
              <Select
                items={kindItems}
                value={kind}
                disabled={addEntries.isPending}
                onValueChange={(next) => {
                  if (next) setKind(next as DossierKind);
                }}
              >
                <SelectTrigger aria-label={t(($) => $.seats.dossier.kind_label)} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {kindItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label htmlFor="dossier-work-class">{t(($) => $.seats.dossier.work_class_label)}</Label>
              <Input
                id="dossier-work-class"
                value={workClass}
                maxLength={DOSSIER_WORK_CLASS_MAX}
                placeholder={t(($) => $.seats.dossier.work_class_placeholder)}
                disabled={addEntries.isPending}
                onChange={(event) => setWorkClass(event.target.value)}
              />
            </div>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="dossier-text">{t(($) => $.seats.dossier.text_label)}</Label>
            <Textarea
              id="dossier-text"
              value={text}
              rows={3}
              maxLength={DOSSIER_TEXT_MAX}
              placeholder={t(($) => $.seats.dossier.text_placeholder)}
              disabled={addEntries.isPending}
              onChange={(event) => setText(event.target.value)}
            />
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label>{t(($) => $.seats.dossier.evidence_type_label)}</Label>
              <Select
                items={evidenceTypeItems}
                value={evidenceType}
                disabled={addEntries.isPending}
                onValueChange={(next) => {
                  if (next) setEvidenceType(next as DossierEvidenceType);
                }}
              >
                <SelectTrigger aria-label={t(($) => $.seats.dossier.evidence_type_label)} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {evidenceTypeItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label htmlFor="dossier-evidence-ref">{t(($) => $.seats.dossier.evidence_ref_label)}</Label>
              <Input
                id="dossier-evidence-ref"
                value={evidenceRef}
                maxLength={DOSSIER_EVIDENCE_REF_MAX}
                placeholder={t(($) => $.seats.dossier.evidence_ref_placeholder)}
                disabled={addEntries.isPending}
                onChange={(event) => setEvidenceRef(event.target.value)}
              />
            </div>
          </div>

          {addError ? (
            <p role="alert" className="text-caption text-destructive">
              {addError}
            </p>
          ) : null}

          <div className="flex flex-wrap justify-end gap-2">
            <Button type="button" size="sm" variant="outline" disabled={addEntries.isPending} onClick={resetAddForm}>
              {t(($) => $.seats.dossier.cancel)}
            </Button>
            <Button type="submit" size="sm" disabled={!canAdd} aria-busy={addEntries.isPending}>
              {t(($) => $.seats.dossier.add_submit)}
            </Button>
          </div>
        </form>
      ) : null}

      {dossier.isError ? (
        <p role="alert" className="text-caption text-destructive">
          {t(($) => $.seats.dossier.load_failed)}
        </p>
      ) : null}

      {!dossier.isPending && !dossier.isError && entries.length === 0 ? (
        <p className="text-body text-muted-foreground">
          {status === "retired" ? t(($) => $.seats.dossier.empty_retired) : t(($) => $.seats.dossier.empty)}
        </p>
      ) : null}

      {entries.length > 0 ? (
        <ul className="flex flex-col divide-y rounded-lg border bg-card">
          {entries.map((entry) => {
            const isRetiring = retiringId === entry.id;
            const created = formatCreatedDate(entry.created_at);
            return (
              <li key={entry.id} className="flex flex-col gap-2 px-4 py-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-caption text-muted-foreground">
                    <span className="rounded-md bg-muted px-1.5 py-0.5 font-medium text-foreground">
                      {t(($) => $.seats.dossier.kind[entry.kind])}
                    </span>
                    <span className="break-all">
                      {entry.work_class || t(($) => $.seats.dossier.all_work)}
                    </span>
                    <span aria-hidden="true">·</span>
                    <span>{t(($) => $.seats.dossier.origin[entry.origin])}</span>
                    {entry.status === "retired" ? (
                      <>
                        <span aria-hidden="true">·</span>
                        <span>{t(($) => $.seats.dossier.status.retired)}</span>
                      </>
                    ) : null}
                    {created ? (
                      <>
                        <span aria-hidden="true">·</span>
                        <span className="tabular-nums">{created}</span>
                      </>
                    ) : null}
                  </div>
                  {entry.status === "active" && !isRetiring ? (
                    <Button type="button" size="xs" variant="ghost" onClick={() => openRetire(entry)}>
                      {t(($) => $.seats.dossier.retire)}
                    </Button>
                  ) : null}
                </div>

                <p className="whitespace-pre-wrap break-words text-body text-foreground">{entry.text}</p>

                {isRetiring ? (
                  <form
                    onSubmit={(event) => handleRetireSubmit(event, entry.id)}
                    className="flex flex-col gap-2 rounded-md bg-muted/40 p-3"
                  >
                    <Label htmlFor={`dossier-retire-reason-${entry.id}`}>
                      {t(($) => $.seats.dossier.retire_reason_label)}
                    </Label>
                    <Input
                      id={`dossier-retire-reason-${entry.id}`}
                      value={retireReason}
                      placeholder={t(($) => $.seats.dossier.retire_reason_placeholder)}
                      disabled={retireEntry.isPending}
                      onChange={(event) => setRetireReason(event.target.value)}
                    />
                    {retireError ? (
                      <p role="alert" className="text-caption text-destructive">
                        {retireError}
                      </p>
                    ) : null}
                    <div className="flex flex-wrap justify-end gap-2">
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={retireEntry.isPending}
                        onClick={cancelRetire}
                      >
                        {t(($) => $.seats.dossier.cancel)}
                      </Button>
                      <Button
                        type="submit"
                        size="sm"
                        variant="destructive"
                        disabled={retireReason.trim() === "" || retireEntry.isPending}
                        aria-busy={retireEntry.isPending}
                      >
                        {t(($) => $.seats.dossier.retire_confirm)}
                      </Button>
                    </div>
                  </form>
                ) : null}
              </li>
            );
          })}
        </ul>
      ) : null}

      <ShadowTrialDialog open={shadowOpen} onOpenChange={setShadowOpen} wsId={wsId} agentId={agentId} />
    </section>
  );
}
