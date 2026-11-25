"use client";

import { useState } from "react";
import { toast } from "sonner";
import { errorCode } from "@multica/core/api";
import {
  buildEditedCall,
  canEditIntent,
  editableArgKey,
  useApproveIntent,
  useRejectIntent,
  type CanonicalCall,
  type Intent,
} from "@multica/core/durable/intents";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import { Collapsible, CollapsibleContent } from "@multica/ui/components/ui/collapsible";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { useT } from "../../i18n";

type Panel = "none" | "edit" | "reject";

/**
 * One action an agent wants to take, waiting for a person (durable spec 5.5). The preview is
 * what the platform built from the call; the agent's reason is shown apart from it. The
 * decision is recorded on the server first, and its result is reported with a toast that
 * still shows if this card has been unmounted by then.
 */
export function IntentCard({ intent, onDecided }: { intent: Intent; onDecided?: () => void }) {
  const { t } = useT("durable");
  const wsId = useWorkspaceId();
  const approve = useApproveIntent(wsId);
  const reject = useRejectIntent(wsId);
  const editable = canEditIntent(intent);
  const editKey = editableArgKey(intent.call);
  const originalText = editKey && intent.call ? String(intent.call.args[editKey] ?? "") : "";
  const [panel, setPanel] = useState<Panel>("none");
  const [text, setText] = useState(originalText);
  const [note, setNote] = useState("");
  const busy = approve.isPending || reject.isPending;

  const reportApproval = (editedCall?: CanonicalCall) => {
    approve
      .mutateAsync({ id: intent.id, editedCall })
      .then(
        (result) => {
          if (result?.state === "failed") toast.error(t(($) => $.intent.approve_failed));
          else if (result) toast.success(t(($) => $.intent.approved));
          else toast.error(t(($) => $.intent.approve_failed));
          onDecided?.();
        },
        (err: unknown) => {
          toast.error(
            errorCode(err) === "intent_not_pending"
              ? t(($) => $.intent.already_decided)
              : t(($) => $.intent.approve_failed),
          );
        },
      );
  };

  const approveEdited = () => {
    const edited = intent.call && text !== originalText ? buildEditedCall(intent.call, text) : null;
    reportApproval(edited ?? undefined);
  };

  const sendRejection = () => {
    reject
      .mutateAsync({ id: intent.id, note: note.trim() })
      .then(
        (result) => {
          if (result) toast.success(t(($) => $.intent.rejected));
          else toast.error(t(($) => $.intent.reject_failed));
          onDecided?.();
        },
        (err: unknown) => {
          toast.error(
            errorCode(err) === "intent_not_pending"
              ? t(($) => $.intent.already_decided)
              : t(($) => $.intent.reject_failed),
          );
        },
      );
  };

  const preview = intent.preview;
  const title = preview.title || intent.target;

  return (
    <article
      aria-labelledby={`intent-${intent.id}-title`}
      className="flex flex-col gap-3 rounded-lg border bg-card p-4"
    >
      <header className="flex flex-wrap items-center gap-x-2 gap-y-1 text-caption text-muted-foreground">
        <span>{t(($) => $.intent.kind[preview.kind])}</span>
        <span aria-hidden>·</span>
        <span>{t(($) => $.intent.effect_class[intent.effect_class])}</span>
        <span aria-hidden>·</span>
        <span>{t(($) => $.intent.source[intent.source])}</span>
      </header>

      <h3 id={`intent-${intent.id}-title`} className="text-title-sm font-medium break-words">
        {title}
      </h3>

      {preview.recipients.length > 0 ? (
        <p className="text-caption text-muted-foreground">
          <span>{t(($) => $.intent.recipients)}: </span>
          <span className="break-all text-foreground">{preview.recipients.join(", ")}</span>
        </p>
      ) : null}

      {preview.body_markdown ? (
        <p className="whitespace-pre-wrap break-words text-body text-foreground">{preview.body_markdown}</p>
      ) : null}

      {preview.fields.length > 0 ? (
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-caption">
          {preview.fields.map((field, index) => (
            <div key={`${field.label}-${index}`} className="contents">
              <dt className="text-muted-foreground">{field.label}</dt>
              <dd className="flex flex-wrap items-center gap-x-1.5 break-all text-foreground">
                {field.before ? (
                  <>
                    <span className="text-muted-foreground line-through">{field.before}</span>
                    <span aria-hidden className="text-muted-foreground">→</span>
                  </>
                ) : null}
                <span>{field.after}</span>
              </dd>
            </div>
          ))}
        </dl>
      ) : null}

      {intent.rationale ? (
        <div className="rounded-md bg-muted/50 px-3 py-2 text-caption">
          <p className="text-muted-foreground">{t(($) => $.intent.rationale)}</p>
          <p className="mt-0.5 whitespace-pre-wrap break-words text-foreground">{intent.rationale}</p>
        </div>
      ) : null}

      <Collapsible open={panel === "edit"}>
        <CollapsibleContent className="flex flex-col gap-2 pt-1">
          <label htmlFor={`intent-${intent.id}-text`} className="text-caption text-muted-foreground">
            {t(($) => $.intent.edit_label)}
          </label>
          <Textarea
            id={`intent-${intent.id}-text`}
            aria-label={t(($) => $.intent.edit_label)}
            value={text}
            onChange={(event) => setText(event.target.value)}
            rows={5}
          />
          <div className="flex flex-wrap justify-end gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => setPanel("none")}>
              {t(($) => $.intent.cancel)}
            </Button>
            <Button type="button" size="sm" disabled={busy} aria-busy={busy} onClick={approveEdited}>
              {t(($) => $.intent.approve_edited)}
            </Button>
          </div>
        </CollapsibleContent>
      </Collapsible>

      <Collapsible open={panel === "reject"}>
        <CollapsibleContent className="flex flex-col gap-2 pt-1">
          <Textarea
            aria-label={t(($) => $.intent.reject_label)}
            placeholder={t(($) => $.intent.reject_label)}
            value={note}
            onChange={(event) => setNote(event.target.value)}
            rows={3}
          />
          <div className="flex flex-wrap justify-end gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => setPanel("none")}>
              {t(($) => $.intent.cancel)}
            </Button>
            <Button
              type="button"
              variant="destructive"
              size="sm"
              disabled={busy || note.trim() === ""}
              aria-busy={busy}
              onClick={sendRejection}
            >
              {t(($) => $.intent.reject_confirm)}
            </Button>
          </div>
        </CollapsibleContent>
      </Collapsible>

      {panel === "none" ? (
        <div className="flex flex-wrap justify-end gap-2">
          {editable ? (
            <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => setPanel("edit")}>
              {t(($) => $.intent.edit)}
            </Button>
          ) : null}
          <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => setPanel("reject")}>
            {t(($) => $.intent.reject)}
          </Button>
          <Button type="button" size="sm" disabled={busy} aria-busy={busy} onClick={() => reportApproval()}>
            {t(($) => $.intent.approve)}
          </Button>
        </div>
      ) : null}
    </article>
  );
}
