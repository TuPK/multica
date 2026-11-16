"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "@multica/ui/icons";
import { useAuthStore } from "@multica/core/auth";
import { catalogText } from "@multica/core/catalog";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { isRuntimeUsableForUser, runtimeListOptions } from "@multica/core/runtimes";
import {
  planSquadSetAgents,
  SQUAD_TEMPLATE_CATEGORIES,
  SQUAD_TEMPLATES,
  squadTemplateCategory,
  useSquadSetQuickstart,
  type QuickstartOutcome,
  type QuickstartSetOutcome,
  type SquadTemplate,
} from "@multica/core/squads";
import { agentListOptions, memberListOptions } from "@multica/core/workspace/queries";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Label } from "@multica/ui/components/ui/label";
import { toast } from "sonner";

import { RuntimePicker } from "../agents/components/runtime-picker";
import { useLocale, useT } from "../i18n";
import { useNavigation } from "../navigation";

function hasProblems(outcome: QuickstartOutcome): boolean {
  return (
    outcome.agents.some((a) => a.status === "failed") ||
    outcome.squad.status === "failed" ||
    !!outcome.squad.error ||
    outcome.workflow.status === "failed" ||
    outcome.routines.some((r) => r.status === "failed" || r.status === "partial") ||
    outcome.sources.status === "failed"
  );
}

function hasSetProblems(outcome: QuickstartSetOutcome): boolean {
  return outcome.items.some((item) => hasProblems(item.outcome));
}

/** The template groups in list order, leaving out an empty group. */
const TEMPLATE_GROUPS = SQUAD_TEMPLATE_CATEGORIES.map((category) => ({
  category,
  templates: SQUAD_TEMPLATES.filter((tpl) => squadTemplateCategory(tpl) === category),
})).filter((group) => group.templates.length > 0);

export function SquadQuickstartModal({ onClose }: { onClose: () => void }) {
  const { t } = useT("modals");
  const locale = useLocale();
  const router = useNavigation();
  const wsPaths = useWorkspacePaths();
  const wsId = useWorkspaceId();
  const currentUserId = useAuthStore((s) => s.user?.id ?? null);

  const { data: runtimes = [], isLoading: runtimesLoading } = useQuery(
    runtimeListOptions(wsId),
  );
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: agents = [] } = useQuery(agentListOptions(wsId));

  const [templateId, setTemplateId] = useState<SquadTemplate["id"]>(
    SQUAD_TEMPLATES[0]!.id,
  );
  const [runtimeId, setRuntimeId] = useState("");
  const [createWorkflow, setCreateWorkflow] = useState(true);
  const [reviewAcknowledged, setReviewAcknowledged] = useState(false);
  const [outcome, setOutcome] = useState<QuickstartSetOutcome | null>(null);
  // The included squads the boss unchecked. Every included squad is kept by default.
  const [droppedIncluded, setDroppedIncluded] = useState<ReadonlySet<SquadTemplate["id"]>>(new Set());
  const quickstart = useSquadSetQuickstart(wsId);

  const template = SQUAD_TEMPLATES.find((tpl) => tpl.id === templateId)!;
  const includedTemplates = useMemo(
    () =>
      (template.includes ?? []).flatMap((id) => {
        const included = SQUAD_TEMPLATES.find((tpl) => tpl.id === id);
        return included && !droppedIncluded.has(id) ? [included] : [];
      }),
    [template, droppedIncluded],
  );
  const plan = useMemo(
    () => planSquadSetAgents([...includedTemplates, template], agents),
    [includedTemplates, template, agents],
  );
  const runtime = runtimes.find((r) => r.id === runtimeId);
  const needsRuntime = plan.some((p) => !p.reuse);
  const runtimeOk = !needsRuntime || (!!runtime && isRuntimeUsableForUser(runtime, currentUserId));
  const running = quickstart.isPending;
  // The acknowledgement belongs to one template: choosing another one clears it.
  const reviewOk = !template.expertReview || reviewAcknowledged;
  const canSubmit = runtimeOk && reviewOk && !running;

  const selectTemplate = (id: SquadTemplate["id"]) => {
    if (id === templateId) return;
    setTemplateId(id);
    setReviewAcknowledged(false);
    setDroppedIncluded(new Set());
  };

  const toggleIncluded = (id: SquadTemplate["id"], keep: boolean) => {
    setDroppedIncluded((current) => {
      const next = new Set(current);
      if (keep) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const run = async () => {
    if (!canSubmit) return;
    try {
      const done = await quickstart.mutateAsync({
        template,
        included: includedTemplates,
        options: {
          runtimeId,
          createWorkflow: createWorkflow && !!template.workflow,
        },
        onProgress: setOutcome,
      });
      setOutcome(done);
      if (!hasSetProblems(done)) toast.success(t(($) => $.squad_quickstart.toast_done));
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t(($) => $.squad_quickstart.toast_failed),
      );
    }
  };

  const finished = outcome !== null && !running;
  // The set's own squad is the last item; it is the one the boss opens.
  const openSquadId = outcome?.items.at(-1)?.outcome.squad.id ?? null;
  const statusText = {
    created: t(($) => $.squad_quickstart.status_created),
    reused: t(($) => $.squad_quickstart.status_reused),
    failed: t(($) => $.squad_quickstart.status_failed),
    existing: t(($) => $.squad_quickstart.status_existing),
    partial: t(($) => $.squad_quickstart.status_partial),
    skipped: "",
  };

  return (
    <Dialog open onOpenChange={(v) => { if (!v && !running) onClose(); }}>
      <DialogContent className="p-0 gap-0 flex flex-col overflow-hidden !top-1/2 !left-1/2 !-translate-x-1/2 !-translate-y-1/2 !w-full !max-w-2xl !max-h-[85vh]">
        <DialogHeader className="border-b px-5 py-3 space-y-0">
          <DialogTitle className="text-title-sm font-semibold">
            {t(($) => $.squad_quickstart.title)}
          </DialogTitle>
        </DialogHeader>

        <div className="flex-1 overflow-y-auto p-5 space-y-5">
          {!outcome && (
            <>
              <div className="space-y-2">
                <Label className="text-caption text-muted-foreground">
                  {t(($) => $.squad_quickstart.template_label)}
                </Label>
                {TEMPLATE_GROUPS.map((group) => {
                  const headingId = `squad-quickstart-group-${group.category}`;
                  return (
                    <div key={group.category} className="space-y-2">
                      <h3
                        id={headingId}
                        className="text-caption font-medium text-muted-foreground"
                      >
                        {catalogText("collection", group.category, locale).title}
                      </h3>
                      <div role="radiogroup" aria-labelledby={headingId} className="grid gap-2">
                        {group.templates.map((tpl) => {
                          const selected = tpl.id === templateId;
                          return (
                            <button
                              key={tpl.id}
                              type="button"
                              role="radio"
                              aria-checked={selected}
                              onClick={() => selectTemplate(tpl.id)}
                              className={`flex w-full min-w-0 flex-col items-start gap-0.5 rounded-lg border px-3 py-2 text-left transition-colors hover:bg-accent/50 ${
                                selected ? "border-primary bg-accent/50" : ""
                              }`}
                            >
                              <span className="flex w-full min-w-0 items-center justify-between gap-2">
                                <span className="min-w-0 break-words text-body font-medium">
                                  {catalogText("squad", tpl.id, locale).title}
                                </span>
                                {tpl.expertReview && (
                                  <span className="shrink-0 text-caption text-warning">
                                    {t(($) => $.squad_quickstart.expert_review_badge)}
                                  </span>
                                )}
                              </span>
                              <span className="min-w-0 break-words text-caption text-muted-foreground">
                                {catalogText("squad", tpl.id, locale).description}
                              </span>
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  );
                })}
              </div>

              {template.includes && template.includes.length > 0 && (
                <div className="space-y-2">
                  <Label className="text-caption text-muted-foreground">
                    {t(($) => $.squad_quickstart.included_label)}
                  </Label>
                  <ul className="space-y-2">
                    {template.includes.map((id) => {
                      const included = SQUAD_TEMPLATES.find((tpl) => tpl.id === id);
                      if (!included) return null;
                      const text = catalogText("squad", included.id, locale);
                      const checkboxId = `squad-quickstart-included-${included.id}`;
                      return (
                        <li key={included.id} className="flex items-start gap-2">
                          <Checkbox
                            id={checkboxId}
                            checked={!droppedIncluded.has(included.id)}
                            onCheckedChange={(v) => toggleIncluded(included.id, v === true)}
                            className="mt-0.5"
                          />
                          <div className="min-w-0 space-y-0.5">
                            <Label htmlFor={checkboxId} className="text-body">
                              {text.title}
                            </Label>
                            <p className="text-caption text-muted-foreground">{text.description}</p>
                          </div>
                        </li>
                      );
                    })}
                  </ul>
                </div>
              )}

              <div className="space-y-2">
                <Label className="text-caption text-muted-foreground">
                  {t(($) => $.squad_quickstart.agents_label)}
                </Label>
                <ul className="grid grid-cols-1 gap-x-4 gap-y-1 sm:grid-cols-2">
                  {plan.map((p) => (
                    <li key={p.name} className="flex items-center justify-between gap-2 text-body">
                      <span className="truncate">{p.name}</span>
                      <span className="shrink-0 text-caption text-muted-foreground">
                        {p.reuse
                          ? t(($) => $.squad_quickstart.agent_reused)
                          : t(($) => $.squad_quickstart.agent_new)}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>

              {needsRuntime && (
                <RuntimePicker
                  runtimes={runtimes}
                  runtimesLoading={runtimesLoading}
                  members={members}
                  currentUserId={currentUserId}
                  selectedRuntimeId={runtimeId}
                  onSelect={setRuntimeId}
                />
              )}

              {template.workflow && (
                <div className="flex items-start gap-2">
                  <Checkbox
                    id="squad-quickstart-workflow"
                    checked={createWorkflow}
                    onCheckedChange={(v) => setCreateWorkflow(v === true)}
                    className="mt-0.5"
                  />
                  <div className="space-y-0.5">
                    <Label htmlFor="squad-quickstart-workflow" className="text-body">
                      {t(($) => $.squad_quickstart.workflow_label, {
                        name: template.workflow.name,
                      })}
                    </Label>
                    <p className="text-caption text-muted-foreground">
                      {t(($) => $.squad_quickstart.workflow_hint)}
                    </p>
                  </div>
                </div>
              )}
            </>
          )}

          {outcome && (
            <div className="space-y-4" aria-live="polite">
              <p className="text-body font-medium">
                {running
                  ? t(($) => $.squad_quickstart.creating)
                  : hasSetProblems(outcome)
                    ? t(($) => $.squad_quickstart.result_problems)
                    : t(($) => $.squad_quickstart.result_ready)}
              </p>
              {outcome.items.map((item) => {
                const current = item.outcome;
                return (
                  <div key={item.templateId} className="space-y-2">
                    {outcome.items.length > 1 && (
                      <h3 className="text-caption font-medium text-muted-foreground">
                        {catalogText("squad", item.templateId, locale).title}
                      </h3>
                    )}
                    <ul className="space-y-1">
                      {current.agents.map((a) => (
                        <li key={a.role} className="flex items-start justify-between gap-3 text-body">
                          <span className="truncate">{a.name}</span>
                          <span
                            className={`shrink-0 text-caption ${
                              a.status === "failed" ? "text-destructive" : "text-muted-foreground"
                            }`}
                          >
                            {a.status === "failed" && a.error
                              ? `${t(($) => $.squad_quickstart.status_failed)}: ${a.error}`
                              : statusText[a.status]}
                          </span>
                        </li>
                      ))}
                      {current.squad.id !== null || current.squad.status === "failed" ? (
                        <li className="flex items-start justify-between gap-3 text-body font-medium">
                          <span>{t(($) => $.squad_quickstart.squad_row)}</span>
                          <span
                            className={`shrink-0 text-caption ${
                              current.squad.status === "failed" || current.squad.error
                                ? "text-destructive"
                                : "text-muted-foreground"
                            }`}
                          >
                            {current.squad.error
                              ? `${t(($) => $.squad_quickstart.status_problem)}: ${current.squad.error}`
                              : statusText[current.squad.status]}
                          </span>
                        </li>
                      ) : null}
                      {current.workflow.status !== "skipped" && (
                        <li className="flex items-start justify-between gap-3 text-body font-medium">
                          <span>{t(($) => $.squad_quickstart.workflow_row)}</span>
                          <span
                            className={`shrink-0 text-caption ${
                              current.workflow.status === "failed"
                                ? "text-destructive"
                                : "text-muted-foreground"
                            }`}
                          >
                            {current.workflow.status === "failed"
                              ? `${t(($) => $.squad_quickstart.status_failed)}: ${current.workflow.error}`
                              : current.workflow.note === "qa_step_left_out"
                                ? t(($) => $.squad_quickstart.workflow_without_qa)
                                : statusText[current.workflow.status]}
                          </span>
                        </li>
                      )}
                      {current.routines.map((r) => (
                        <li key={r.title} className="flex items-start justify-between gap-3 text-body font-medium">
                          <span>{r.title}</span>
                          <span
                            className={`shrink-0 text-caption ${
                              r.status === "failed" || r.status === "partial"
                                ? "text-destructive"
                                : "text-muted-foreground"
                            }`}
                          >
                            {r.status === "failed"
                              ? `${t(($) => $.squad_quickstart.status_failed)}: ${r.error}`
                              : r.status === "partial"
                                ? `${statusText.partial}: ${r.error}`
                                : statusText[r.status]}
                          </span>
                        </li>
                      ))}
                      {current.sources.status !== "skipped" || current.sources.error ? (
                        <li className="flex items-start justify-between gap-3 text-body font-medium">
                          <span>{t(($) => $.squad_quickstart.sources_row)}</span>
                          <span
                            className={`shrink-0 text-caption ${
                              current.sources.status === "failed" || current.sources.status === "skipped"
                                ? "text-destructive"
                                : "text-muted-foreground"
                            }`}
                          >
                            {current.sources.status === "skipped"
                              ? t(($) => $.squad_quickstart.sources_need_knowledge)
                              : current.sources.status === "failed"
                                ? `${t(($) => $.squad_quickstart.status_failed)}: ${current.sources.failed}`
                                : t(($) => $.squad_quickstart.sources_written, { count: current.sources.written })}
                          </span>
                        </li>
                      ) : null}
                    </ul>
                  </div>
                );
              })}
              {finished && !hasSetProblems(outcome) && (
                <p className="text-caption text-muted-foreground">
                  {t(($) => $.squad_quickstart.next_step)}
                </p>
              )}
            </div>
          )}
        </div>

        {!outcome && template.expertReview && (
          <div className="space-y-2 border-t px-5 py-3">
            <p id="squad-quickstart-review-notice" className="text-body">
              {t(($) => $.squad_quickstart.expert_review_notice)}
            </p>
            <div className="flex items-start gap-2">
              <Checkbox
                id="squad-quickstart-review"
                checked={reviewAcknowledged}
                onCheckedChange={(v) => setReviewAcknowledged(v === true)}
                aria-describedby="squad-quickstart-review-notice"
                className="mt-0.5"
              />
              <Label htmlFor="squad-quickstart-review" className="text-body">
                {t(($) => $.squad_quickstart.expert_review_ack)}
              </Label>
            </div>
          </div>
        )}

        <div className="flex items-center justify-end gap-2 border-t px-5 py-3">
          {finished ? (
            <>
              {hasSetProblems(outcome) && (
                <Button variant="outline" onClick={() => void run()}>
                  {t(($) => $.squad_quickstart.retry)}
                </Button>
              )}
              <Button variant="outline" onClick={onClose}>
                {t(($) => $.squad_quickstart.close)}
              </Button>
              {openSquadId && (
                <Button
                  onClick={() => {
                    onClose();
                    router.push(wsPaths.squadDetail(openSquadId));
                  }}
                >
                  {t(($) => $.squad_quickstart.open_squad)}
                </Button>
              )}
            </>
          ) : (
            <>
              <Button variant="outline" onClick={onClose} disabled={running}>
                {t(($) => $.squad_quickstart.cancel)}
              </Button>
              <Button onClick={() => void run()} disabled={!canSubmit}>
                {running && <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />}
                {running
                  ? t(($) => $.squad_quickstart.creating)
                  : t(($) => $.squad_quickstart.create)}
              </Button>
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
