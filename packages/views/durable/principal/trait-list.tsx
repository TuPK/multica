"use client";

import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import {
  useAddPrincipalTrait,
  useEditPrincipalTrait,
  usePrincipalEvidenceCandidates,
  useRetirePrincipalTrait,
  type PrincipalEvidenceCandidate,
  type PrincipalOrigin,
  type PrincipalTrait,
} from "@multica/core/durable/principal";
import { Button } from "@multica/ui/components/ui/button";
import { Label } from "@multica/ui/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Loader2, Pencil, Plus, Trash2 } from "@multica/ui/icons";
import { useLocale, useT } from "../../i18n";
import { KNOWN_DECISION_KINDS, decisionKindLabel, principalErrorMessage } from "./error-copy";

// Mirror server/internal/service/durable_principal_fork.go:principalStatementMax.
const MAX_STATEMENT_LENGTH = 500;

/**
 * Lists the person's active principal traits grouped by `decision_kind` (durable spec 6.2–6.3),
 * and lets the person add a manual trait backed by one of their own recent proposal decisions.
 */
export function TraitList({ wsId, traits }: { wsId: string; traits: PrincipalTrait[] }) {
  const { t } = useT("durable");
  const locale = useLocale();
  const add = useAddPrincipalTrait(wsId);
  const edit = useEditPrincipalTrait(wsId);
  const retire = useRetirePrincipalTrait(wsId);
  const [adding, setAdding] = useState(false);
  const [evidenceRef, setEvidenceRef] = useState<string | null>(null);
  const [decisionKind, setDecisionKind] = useState<string>("general");
  const [newStatement, setNewStatement] = useState("");
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [retiringId, setRetiringId] = useState<string | null>(null);

  const candidatesQuery = usePrincipalEvidenceCandidates(wsId, adding);
  const candidates = candidatesQuery.data?.decisions ?? [];

  const grouped = groupByKind(traits);

  const kindItems = KNOWN_DECISION_KINDS.map((kind) => ({
    value: kind,
    label: decisionKindLabel(kind, t),
  }));

  const candidateItems = candidates.map((candidate) => ({
    value: candidate.id,
    label: formatCandidateLabel(candidate, locale, t),
  }));

  const resetAddForm = () => {
    setAdding(false);
    setEvidenceRef(null);
    setDecisionKind("general");
    setNewStatement("");
  };

  const canSubmitAdd = Boolean(evidenceRef) && newStatement.trim().length > 0 && !add.isPending;

  const submitAdd = (event: FormEvent) => {
    event.preventDefault();
    const statement = newStatement.trim();
    if (!evidenceRef || !statement || add.isPending) return;
    add
      .mutateAsync({
        decision_kind: decisionKind,
        statement,
        origin: "edited",
        evidence: [{ type: "decision", ref: evidenceRef }],
      })
      .then(
        () => {
          toast.success(t(($) => $.principal.traits.added));
          resetAddForm();
        },
        (err: unknown) => {
          toast.error(principalErrorMessage(err, t, t(($) => $.principal.traits.add_failed)));
        },
      );
  };

  const startEdit = (trait: PrincipalTrait) => {
    setEditingId(trait.id);
    setDraft(trait.statement);
  };

  const cancelEdit = () => {
    setEditingId(null);
    setDraft("");
  };

  const submitEdit = (event: FormEvent, trait: PrincipalTrait) => {
    event.preventDefault();
    const statement = draft.trim();
    if (!statement || statement === trait.statement || edit.isPending) return;
    edit.mutateAsync({ id: trait.id, statement }).then(
      () => {
        toast.success(t(($) => $.principal.traits.edited));
        setEditingId(null);
        setDraft("");
      },
      (err: unknown) => {
        toast.error(principalErrorMessage(err, t, t(($) => $.principal.traits.edit_failed)));
      },
    );
  };

  const handleRetire = (id: string) => {
    if (retire.isPending) return;
    setRetiringId(id);
    retire.mutateAsync(id).then(
      () => {
        toast.success(t(($) => $.principal.traits.retired));
        setRetiringId((prev) => (prev === id ? null : prev));
        if (editingId === id) cancelEdit();
      },
      (err: unknown) => {
        toast.error(principalErrorMessage(err, t, t(($) => $.principal.traits.retire_failed)));
        setRetiringId((prev) => (prev === id ? null : prev));
      },
    );
  };

  return (
    <section aria-labelledby="principal-traits-heading" className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="principal-traits-heading" className="text-title-sm font-medium">
          {t(($) => $.principal.traits.title)}
        </h2>
        {!adding ? (
          <Button type="button" size="sm" variant="outline" onClick={() => setAdding(true)}>
            <Plus className="size-3.5" />
            {t(($) => $.principal.traits.add)}
          </Button>
        ) : null}
      </div>

      {adding ? (
        <form
          onSubmit={submitAdd}
          className="flex flex-col gap-3 rounded-md border border-surface-border p-3"
        >
          <div className="flex flex-col gap-1.5">
            <span className="text-caption font-medium">
              {t(($) => $.principal.traits.evidence_label)}
            </span>
            {candidatesQuery.isError ? (
              <p role="alert" className="text-caption text-destructive">
                {t(($) => $.principal.traits.evidence_load_failed)}
              </p>
            ) : candidatesQuery.isPending ? (
              <div className="flex items-center py-1">
                <Loader2 className="size-4 animate-spin text-muted-foreground" />
              </div>
            ) : candidateItems.length === 0 ? (
              <p className="text-caption text-muted-foreground">
                {t(($) => $.principal.traits.evidence_empty)}
              </p>
            ) : (
              <Select
                items={candidateItems}
                value={evidenceRef}
                disabled={add.isPending}
                onValueChange={(next) => setEvidenceRef(next ?? null)}
              >
                <SelectTrigger
                  aria-label={t(($) => $.principal.traits.evidence_label)}
                  className="w-full"
                >
                  <SelectValue placeholder={t(($) => $.principal.traits.evidence_placeholder)} />
                </SelectTrigger>
                <SelectContent>
                  {candidateItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <span className="text-caption font-medium">
              {t(($) => $.principal.traits.decision_kind_label)}
            </span>
            <Select
              items={kindItems}
              value={decisionKind}
              disabled={add.isPending}
              onValueChange={(next) => {
                if (next) setDecisionKind(next);
              }}
            >
              <SelectTrigger
                aria-label={t(($) => $.principal.traits.decision_kind_label)}
                className="w-full"
              >
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

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="principal-trait-new-statement" className="text-caption font-medium">
              {t(($) => $.principal.traits.statement_label)}
            </Label>
            <Textarea
              id="principal-trait-new-statement"
              value={newStatement}
              rows={2}
              maxLength={MAX_STATEMENT_LENGTH}
              placeholder={t(($) => $.principal.traits.statement_placeholder)}
              disabled={add.isPending}
              onChange={(e) => setNewStatement(e.target.value)}
            />
          </div>

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={!canSubmitAdd} aria-busy={add.isPending || undefined}>
              {add.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
              {t(($) => $.principal.traits.add)}
            </Button>
            <Button type="button" size="sm" variant="outline" disabled={add.isPending} onClick={resetAddForm}>
              {t(($) => $.principal.cancel)}
            </Button>
          </div>
        </form>
      ) : null}

      {traits.length === 0 ? (
        <p className="text-body text-muted-foreground">{t(($) => $.principal.traits.empty)}</p>
      ) : (
        <div className="flex flex-col gap-5">
          {grouped.map(([kind, items]) => (
            <div key={kind} className="flex flex-col gap-2">
              <div className="flex items-center gap-2">
                <h3 className="text-caption font-medium text-muted-foreground">
                  {decisionKindLabel(kind, t)}
                </h3>
                <code className="font-mono text-caption text-muted-foreground">{kind}</code>
              </div>
              <ul className="flex flex-col divide-y divide-surface-border rounded-md border border-surface-border">
                {items.map((trait) => {
                  const isEditing = editingId === trait.id;
                  const isRetiring = retiringId === trait.id;
                  return (
                    <li key={trait.id} className="flex flex-col gap-2 px-3 py-3">
                      {isEditing ? (
                        <form onSubmit={(e) => submitEdit(e, trait)} className="flex flex-col gap-2">
                          <Label htmlFor={`trait-edit-${trait.id}`} className="text-caption">
                            {t(($) => $.principal.traits.statement_label)}
                          </Label>
                          <Textarea
                            id={`trait-edit-${trait.id}`}
                            value={draft}
                            rows={2}
                            maxLength={MAX_STATEMENT_LENGTH}
                            disabled={edit.isPending}
                            onChange={(e) => setDraft(e.target.value)}
                          />
                          <div className="flex items-center gap-2">
                            <Button
                              type="submit"
                              size="sm"
                              disabled={!draft.trim() || draft.trim() === trait.statement || edit.isPending}
                              aria-busy={edit.isPending || undefined}
                            >
                              {edit.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
                              {t(($) => $.principal.traits.save)}
                            </Button>
                            <Button type="button" size="sm" variant="outline" disabled={edit.isPending} onClick={cancelEdit}>
                              {t(($) => $.principal.cancel)}
                            </Button>
                          </div>
                        </form>
                      ) : (
                        <div className="flex items-start justify-between gap-3">
                          <div className="flex min-w-0 flex-1 flex-col gap-1">
                            <p className="text-body text-foreground wrap-anywhere">{trait.statement}</p>
                            <span className="inline-flex w-fit items-center rounded-md border border-surface-border px-1.5 py-0.5 text-caption text-muted-foreground">
                              {originLabel(trait.origin, t)}
                            </span>
                          </div>
                          <div className="flex shrink-0 items-center gap-1">
                            <Button
                              type="button"
                              variant="ghost"
                              size="sm"
                              onClick={() => startEdit(trait)}
                              disabled={isRetiring}
                            >
                              <Pencil className="size-3.5" />
                              {t(($) => $.principal.traits.edit)}
                            </Button>
                            <Button
                              type="button"
                              variant="ghost"
                              size="sm"
                              onClick={() => handleRetire(trait.id)}
                              disabled={retire.isPending}
                              aria-busy={isRetiring || undefined}
                            >
                              {isRetiring ? <Loader2 className="size-3.5 animate-spin" /> : <Trash2 className="size-3.5" />}
                              {t(($) => $.principal.traits.retire)}
                            </Button>
                          </div>
                        </div>
                      )}
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function formatCandidateLabel(
  candidate: PrincipalEvidenceCandidate,
  locale: string,
  t: ReturnType<typeof useT<"durable">>["t"],
): string {
  const title = candidate.title || candidate.id;
  const state = decisionStateLabel(candidate.decision, t);
  const date = formatShortDate(candidate.decided_at, locale);
  const meta = [state, date].filter(Boolean).join(" · ");
  return meta ? `${title} (${meta})` : title;
}

function decisionStateLabel(decision: string, t: ReturnType<typeof useT<"durable">>["t"]): string {
  switch (decision.trim().toLowerCase()) {
    case "approved":
      return t(($) => $.principal.traits.decision_state.approved);
    case "rejected":
      return t(($) => $.principal.traits.decision_state.rejected);
    case "revising":
      return t(($) => $.principal.traits.decision_state.revising);
    case "expired":
      return t(($) => $.principal.traits.decision_state.expired);
    default:
      return decision;
  }
}

function formatShortDate(value: string, locale: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(date);
}

function originLabel(origin: PrincipalOrigin, t: ReturnType<typeof useT<"durable">>["t"]): string {
  switch (origin) {
    case "interview":
      return t(($) => $.principal.traits.origin.interview);
    case "edited":
      return t(($) => $.principal.traits.origin.edited);
    case "learned":
    default:
      return t(($) => $.principal.traits.origin.learned);
  }
}

function groupByKind(traits: PrincipalTrait[]): Array<[string, PrincipalTrait[]]> {
  const map = new Map<string, PrincipalTrait[]>();
  for (const trait of traits) {
    const kind = trait.decision_kind || "general";
    const list = map.get(kind) ?? [];
    list.push(trait);
    map.set(kind, list);
  }
  return [...map.entries()];
}
