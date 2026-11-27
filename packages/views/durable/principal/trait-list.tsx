"use client";

import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import {
  useEditPrincipalTrait,
  useRetirePrincipalTrait,
  type PrincipalOrigin,
  type PrincipalTrait,
} from "@multica/core/durable/principal";
import { Button } from "@multica/ui/components/ui/button";
import { Label } from "@multica/ui/components/ui/label";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Loader2, Pencil, Trash2 } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { decisionKindLabel, principalErrorMessage } from "./error-copy";

// Mirror server/internal/service/durable_principal_fork.go:principalStatementMax.
const MAX_STATEMENT_LENGTH = 500;

/**
 * Lists the person's active principal traits grouped by `decision_kind` (durable spec 6.2–6.3).
 * Manual trait creation is intentionally hidden: POST /api/principal/profile/traits requires valid
 * UUID evidence owned by the person (a proposal decision, comment, or chat message) and no picker
 * endpoint exists, so the UI does not invent evidence.
 */
export function TraitList({ wsId, traits }: { wsId: string; traits: PrincipalTrait[] }) {
  const { t } = useT("durable");
  const edit = useEditPrincipalTrait(wsId);
  const retire = useRetirePrincipalTrait(wsId);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [retiringId, setRetiringId] = useState<string | null>(null);

  const grouped = groupByKind(traits);

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
      <h2 id="principal-traits-heading" className="text-title-sm font-medium">
        {t(($) => $.principal.traits.title)}
      </h2>

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
