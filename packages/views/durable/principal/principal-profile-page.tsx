"use client";

import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import {
  PRINCIPAL_INTERVIEW_SESSION_TITLE,
  useAnswerPrincipalInterview,
  useDeletePrincipalProfile,
  useEnablePrincipalProfile,
  usePrincipalInterview,
  usePrincipalProfile,
  type PrincipalStatus,
} from "@multica/core/durable/principal";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@multica/ui/components/ui/dialog";
import { Label } from "@multica/ui/components/ui/label";
import { Switch } from "@multica/ui/components/ui/switch";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Loader2, ShieldCheck } from "@multica/ui/icons";
import { cn } from "@multica/ui/lib/utils";
import { useLocale, useT } from "../../i18n";
import { PAGE_GUTTER } from "../../layout/page-header";
import { ConsentDialog } from "./consent-dialog";
import { DelegationPanel } from "./delegation-panel";
import { buildConsentText, principalErrorMessage } from "./error-copy";
import { TraitList } from "./trait-list";

/**
 * The signed-in person's own principal profile (durable spec 6.2 to 6.6, F5).
 * When no profile exists (404), shows an opt-in consent flow; when enabled, shows status,
 * owner-sharing preference, delete confirmation, start-up interview, active traits, and
 * per-kind delegation controls.
 */
export function PrincipalProfilePage() {
  const { t } = useT("durable");
  const locale = useLocale();
  const wsId = useWorkspaceId();
  const profileQuery = usePrincipalProfile(wsId);
  const updateProfile = useEnablePrincipalProfile(wsId);
  const deleteProfile = useDeletePrincipalProfile(wsId);

  const [consentOpen, setConsentOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  if (profileQuery.isError) {
    return (
      <div className={cn(PAGE_GUTTER, "py-4")}>
        <p role="alert" className="text-caption text-destructive">
          {t(($) => $.principal.load_failed)}
        </p>
      </div>
    );
  }

  if (profileQuery.isPending) {
    return (
      <div className={cn(PAGE_GUTTER, "flex items-center py-8")}>
        <Loader2 className="size-4 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const profile = profileQuery.data;

  if (!profile) {
    return (
      <div className={cn(PAGE_GUTTER, "flex max-w-2xl flex-col gap-4 py-6")}>
        <div className="flex items-center gap-2">
          <ShieldCheck className="size-5 text-muted-foreground" aria-hidden="true" />
          <h1 className="text-title-sm font-medium">{t(($) => $.principal.off_title)}</h1>
        </div>
        <div>
          <Button type="button" onClick={() => setConsentOpen(true)}>
            {t(($) => $.principal.enable)}
          </Button>
        </div>
        <ConsentDialog open={consentOpen} onOpenChange={setConsentOpen} wsId={wsId} />
      </div>
    );
  }

  const handleToggleShare = (next: boolean) => {
    if (updateProfile.isPending) return;
    updateProfile
      .mutateAsync({
        consent_text: buildConsentText(t),
        share_with_owners: next,
      })
      .then(
        () => toast.success(t(($) => $.principal.share_saved)),
        (err: unknown) => {
          toast.error(principalErrorMessage(err, t, t(($) => $.principal.share_failed)));
        },
      );
  };

  const handleDelete = () => {
    if (deleteProfile.isPending) return;
    deleteProfile.mutateAsync().then(
      () => {
        toast.success(t(($) => $.principal.deleted));
        setDeleteOpen(false);
      },
      (err: unknown) => {
        toast.error(principalErrorMessage(err, t, t(($) => $.principal.delete_failed)));
      },
    );
  };

  const consentDate = formatDate(profile.consent_at, locale);
  const traitKinds = profile.traits.map((trait) => trait.decision_kind);

  return (
    <div className={cn(PAGE_GUTTER, "flex max-w-3xl flex-col gap-8 py-6")}>
      <section aria-labelledby="principal-profile-heading" className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap items-center gap-2">
            <h1 id="principal-profile-heading" className="text-title-sm font-medium">
              {t(($) => $.principal.title)}
            </h1>
            <span className="inline-flex items-center rounded-md border border-surface-border px-2 py-0.5 text-caption text-foreground">
              {statusLabel(profile.status, t)}
            </span>
            <span className="text-caption tabular-nums text-muted-foreground">
              {t(($) => $.principal.version, { version: profile.version })}
            </span>
            {consentDate ? (
              <span className="text-caption text-muted-foreground">
                {t(($) => $.principal.consented_at, { date: consentDate })}
              </span>
            ) : null}
          </div>

          <Dialog
            open={deleteOpen}
            onOpenChange={(open) => {
              if (!open && deleteProfile.isPending) return;
              setDeleteOpen(open);
            }}
          >
            <DialogTrigger render={<Button type="button" variant="destructive" size="sm" />}>
              {t(($) => $.principal.delete)}
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>{t(($) => $.principal.delete_dialog_title)}</DialogTitle>
                <DialogDescription>{t(($) => $.principal.delete_dialog_description)}</DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <DialogClose render={<Button type="button" variant="outline" disabled={deleteProfile.isPending} />}>
                  {t(($) => $.principal.cancel)}
                </DialogClose>
                <Button
                  type="button"
                  variant="destructive"
                  onClick={handleDelete}
                  disabled={deleteProfile.isPending}
                  aria-busy={deleteProfile.isPending || undefined}
                >
                  {deleteProfile.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
                  {t(($) => $.principal.delete_confirm)}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>

        <div className="flex items-center justify-between gap-3 rounded-md border border-surface-border px-3 py-2.5">
          <Label htmlFor="principal-share-owners" className="text-body font-normal">
            {t(($) => $.principal.share_with_owners)}
          </Label>
          <Switch
            id="principal-share-owners"
            checked={profile.share_with_owners}
            disabled={updateProfile.isPending}
            onCheckedChange={handleToggleShare}
          />
        </div>
      </section>

      <InterviewSection wsId={wsId} />

      <TraitList wsId={wsId} traits={profile.traits} />

      <DelegationPanel extraKinds={traitKinds} />
    </div>
  );
}

function InterviewSection({ wsId }: { wsId: string }) {
  const { t } = useT("durable");
  const interview = usePrincipalInterview(wsId);
  const answerMutation = useAnswerPrincipalInterview(wsId);
  const [index, setIndex] = useState(0);
  const [answer, setAnswer] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const questions = interview.data?.questions ?? [];

  if (interview.isError) {
    return (
      <section aria-labelledby="principal-interview-heading" className="flex flex-col gap-2">
        <h2 id="principal-interview-heading" className="text-title-sm font-medium">
          {t(($) => $.principal.interview.title)}
        </h2>
        <p role="alert" className="text-caption text-destructive">
          {t(($) => $.principal.interview.load_failed)}
        </p>
      </section>
    );
  }

  if (interview.isPending || questions.length === 0) {
    return null;
  }

  const finished = index >= questions.length;
  const currentQuestion = finished ? "" : questions[index]!;
  const busy = submitting || answerMutation.isPending;

  const submitAnswer = async (event: FormEvent) => {
    event.preventDefault();
    const trimmed = answer.trim();
    if (!trimmed || finished || busy) return;
    setSubmitting(true);
    try {
      // POST /api/principal/interview/answers requires a chat message UUID owned by the caller
      // (durable_principal_fork.go:329-353). When the Secretary interview chat exists, send the
      // answer there first to obtain its real message_id rather than inventing an id.
      const messageId = await recordInterviewChatMessage(trimmed);
      await answerMutation.mutateAsync({
        question: index,
        answer: trimmed,
        message_id: messageId,
      });
      toast.success(t(($) => $.principal.interview.saved));
      setAnswer("");
      setIndex((prev) => prev + 1);
    } catch (err: unknown) {
      toast.error(principalErrorMessage(err, t, t(($) => $.principal.interview.save_failed)));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <section aria-labelledby="principal-interview-heading" className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <h2 id="principal-interview-heading" className="text-title-sm font-medium">
          {t(($) => $.principal.interview.title)}
        </h2>
        {!finished ? (
          <span className="text-caption tabular-nums text-muted-foreground">
            {t(($) => $.principal.interview.progress, { current: index + 1, total: questions.length })}
          </span>
        ) : null}
      </div>

      {finished ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-surface-border px-3 py-3">
          <p className="text-body text-muted-foreground">{t(($) => $.principal.interview.completed)}</p>
          <Button type="button" size="sm" variant="outline" onClick={() => setIndex(0)}>
            {t(($) => $.principal.interview.restart)}
          </Button>
        </div>
      ) : (
        <form onSubmit={(e) => void submitAnswer(e)} className="flex flex-col gap-3 rounded-md border border-surface-border p-3">
          <p className="text-body font-medium text-foreground">{currentQuestion}</p>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="principal-interview-answer" className="text-caption">
              {t(($) => $.principal.interview.answer_label)}
            </Label>
            <Textarea
              id="principal-interview-answer"
              value={answer}
              rows={3}
              maxLength={500}
              placeholder={t(($) => $.principal.interview.answer_placeholder)}
              disabled={busy}
              onChange={(e) => setAnswer(e.target.value)}
            />
          </div>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={index === 0 || busy}
                onClick={() => {
                  setIndex((prev) => Math.max(0, prev - 1));
                  setAnswer("");
                }}
              >
                {t(($) => $.principal.interview.previous)}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => {
                  setIndex((prev) => prev + 1);
                  setAnswer("");
                }}
              >
                {t(($) => $.principal.interview.next)}
              </Button>
            </div>
            <Button type="submit" size="sm" disabled={!answer.trim() || busy} aria-busy={busy || undefined}>
              {busy ? <Loader2 className="size-4 animate-spin" /> : null}
              {t(($) => $.principal.interview.submit)}
            </Button>
          </div>
        </form>
      )}
    </section>
  );
}

async function recordInterviewChatMessage(answer: string): Promise<string> {
  const sessions = await api.listChatSessions({ status: "active" });
  const interviewSession = sessions.find((s) => s.title === PRINCIPAL_INTERVIEW_SESSION_TITLE);
  if (!interviewSession) return "";
  const sent = await api.sendChatMessage(interviewSession.id, answer);
  return sent.message_id;
}

function statusLabel(status: PrincipalStatus, t: ReturnType<typeof useT<"durable">>["t"]): string {
  switch (status) {
    case "paused":
      return t(($) => $.principal.status.paused);
    case "active":
    default:
      return t(($) => $.principal.status.active);
  }
}

function formatDate(value: string, locale: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(date);
}
