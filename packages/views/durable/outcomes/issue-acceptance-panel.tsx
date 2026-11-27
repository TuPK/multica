"use client";

import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { AnimatePresence, motion, useIsPresent } from "motion/react";
import {
  CHECK_KINDS,
  useAddCriterion,
  useArchiveCriterion,
  useIssueCriteria,
  useIssueOutcome,
  useOutcome,
  type CheckKind,
  type Criterion,
  type Outcome,
  type Verification,
  type VerificationResult,
} from "@multica/core/durable/outcomes";
import { IconButton } from "@multica/ui/components/common/icon-button";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { Archive, Plus } from "@multica/ui/icons";
import { useReducedMotion } from "@multica/core/appearance";
import { UI_EASE_OUT, UI_MOTION_DURATION } from "@multica/ui/lib/motion";
import { useT } from "../../i18n";
import { IssueSidebarSectionHeader } from "../../issues/components/issue-sidebar-section";
import { VerdictChip } from "./verdict-chip";
import { JurySplitCard } from "./jury-split-card";
import { useOutcomesEnabled } from "./use-outcomes-enabled";

const noop = () => {};

/**
 * The "Acceptance" block of the issue sidebar (durable spec 2.9): the criteria the work must
 * meet, the result of each one, and the verdict with the time left until it settles. Nothing
 * shows while the flag is off.
 */
export function IssueAcceptancePanel({ wsId, issueId }: { wsId: string; issueId: string }) {
  const enabled = useOutcomesEnabled();
  if (!enabled) return null;
  return <AcceptanceBlock wsId={wsId} issueId={issueId} />;
}

function AcceptanceBlock({ wsId, issueId }: { wsId: string; issueId: string }) {
  const { t } = useT("durable");
  const criteria = useIssueCriteria(wsId, issueId);
  const issueOutcome = useIssueOutcome(wsId, issueId);
  const outcome = issueOutcome.data ?? null;
  // The issue route answers without checks; the detail route carries them.
  const detail = useOutcome(wsId, outcome?.id ?? "");
  const results = latestResults(detail.data?.verifications ?? []);
  const [adding, setAdding] = useState(false);
  const archive = useArchiveCriterion(wsId, issueId);

  const active = (criteria.data ?? []).filter((criterion) => !criterion.archived_at);
  // The server refuses to change criteria once the outcome has settled or been checked; a pending
  // outcome with no check yet is the only state where archiving is offered.
  const canArchive = outcome === null || outcome.verdict === "pending";

  const onArchive = (criterionId: string) => {
    archive.mutateAsync(criterionId).then(
      () => toast.success(t(($) => $.outcomes.acceptance.archived)),
      () => toast.error(t(($) => $.outcomes.acceptance.archive_failed)),
    );
  };

  return (
    <section aria-label={t(($) => $.outcomes.acceptance.title)} className="flex flex-col gap-2">
      <IssueSidebarSectionHeader
        title={t(($) => $.outcomes.acceptance.title)}
        open
        onOpenChange={noop}
        collapsible={false}
        actions={
          <IconButton
            variant="ghost"
            size="icon-xs"
            label={t(($) => $.outcomes.acceptance.add)}
            onClick={() => setAdding(true)}
          >
            <Plus />
          </IconButton>
        }
      />
      <div className="flex flex-col gap-2 px-4">
        {outcome ? <OutcomeChip outcome={outcome} /> : null}
        {criteria.isError ? (
          <p role="alert" className="text-caption text-destructive">
            {t(($) => $.outcomes.acceptance.load_failed)}
          </p>
        ) : null}
        {!criteria.isPending && !criteria.isError && active.length === 0 ? (
          <p className="text-caption text-muted-foreground">{t(($) => $.outcomes.acceptance.empty)}</p>
        ) : null}
        {active.length > 0 ? (
          <ul className="flex flex-col">
            <AnimatePresence initial={false}>
              {active.map((criterion) => (
                <CriterionRow
                  key={criterion.id}
                  criterion={criterion}
                  result={results.get(criterion.id)}
                  archiveDisabled={!canArchive || archive.isPending}
                  onArchive={onArchive}
                />
              ))}
            </AnimatePresence>
          </ul>
        ) : null}
        {outcome ? <JurySplitCard wsId={wsId} outcomeId={outcome.id} /> : null}
      </div>
      <AddCriterionDialog open={adding} onOpenChange={setAdding} wsId={wsId} issueId={issueId} />
    </section>
  );
}

/** The verdict chip. While the outcome is pending it names the time left until it settles. */
function OutcomeChip({ outcome }: { outcome: Outcome }) {
  const { t } = useT("durable");
  if (outcome.verdict !== "pending") return <VerdictChip verdict={outcome.verdict} />;

  const left = timeUntil(outcome.settle_at, Date.now());
  const remaining = left
    ? left.days > 0
      ? t(($) => $.outcomes.remaining.days_hours, { days: left.days, hours: left.hours })
      : left.hours > 0
        ? t(($) => $.outcomes.remaining.hours, { hours: left.hours })
        : t(($) => $.outcomes.remaining.minutes, { minutes: left.minutes })
    : null;
  const label = remaining
    ? t(($) => $.outcomes.acceptance.pending, { time: remaining })
    : t(($) => $.outcomes.acceptance.pending_now);
  return <VerdictChip verdict="pending" label={label} />;
}

// A new criterion fades in, and an archived one collapses out (motion.md, list rows). The row reads
// useIsPresent so a leaving row takes no clicks or focus while it collapses.
function CriterionRow({
  criterion,
  result,
  archiveDisabled,
  onArchive,
}: {
  criterion: Criterion;
  result: VerificationResult | undefined;
  archiveDisabled: boolean;
  onArchive: (criterionId: string) => void;
}) {
  const { t } = useT("durable");
  const isPresent = useIsPresent();
  const reduceMotion = useReducedMotion();
  const duration = reduceMotion ? 0 : UI_MOTION_DURATION.fast;
  return (
    <motion.li
      inert={!isPresent}
      initial={{ opacity: 0, y: reduceMotion ? 0 : 4, overflow: "hidden" }}
      animate={{ opacity: 1, y: 0, transitionEnd: { overflow: "visible" } }}
      exit={{ opacity: 0, height: 0, paddingBottom: 0, overflow: "hidden" }}
      transition={{ duration, ease: UI_EASE_OUT }}
      className="flex min-w-0 items-start gap-2 pb-2"
    >
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="break-words text-body">{criterion.text}</span>
        <span className="flex min-w-0 flex-wrap gap-x-2 text-caption text-muted-foreground">
          <span>{t(($) => $.outcomes.check_kind[criterion.check_kind])}</span>
          <span>{t(($) => $.outcomes.result[result ?? "unable"])}</span>
        </span>
      </div>
      <IconButton
        variant="ghost"
        size="icon-xs"
        label={t(($) => $.outcomes.acceptance.archive)}
        disabled={archiveDisabled}
        onClick={() => onArchive(criterion.id)}
      >
        <Archive />
      </IconButton>
    </motion.li>
  );
}

function AddCriterionDialog({
  open,
  onOpenChange,
  wsId,
  issueId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  wsId: string;
  issueId: string;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <AddCriterionForm wsId={wsId} issueId={issueId} onDone={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  );
}

// The form mounts with the dialog, so a reopened dialog starts empty. The dialog closes only after
// the server keeps the criterion; a refused one keeps the text so it can be fixed and sent again.
function AddCriterionForm({ wsId, issueId, onDone }: { wsId: string; issueId: string; onDone: () => void }) {
  const { t } = useT("durable");
  const [text, setText] = useState("");
  const [checkKind, setCheckKind] = useState<CheckKind>("manual");
  const add = useAddCriterion(wsId, issueId);
  const canSubmit = text.trim() !== "" && !add.isPending;

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!canSubmit) return;
    add.mutateAsync({ text: text.trim(), check_kind: checkKind }).then(
      () => {
        toast.success(t(($) => $.outcomes.acceptance.added));
        onDone();
      },
      () => toast.error(t(($) => $.outcomes.acceptance.add_failed)),
    );
  };

  const kindItems = CHECK_KINDS.map((kind) => ({
    value: kind,
    label: t(($) => $.outcomes.check_kind[kind]),
  }));

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>{t(($) => $.outcomes.acceptance.add)}</DialogTitle>
      </DialogHeader>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="acceptance-criterion">{t(($) => $.outcomes.acceptance.criterion_label)}</Label>
        <Input
          id="acceptance-criterion"
          value={text}
          placeholder={t(($) => $.outcomes.acceptance.criterion_placeholder)}
          disabled={add.isPending}
          onChange={(event) => setText(event.target.value)}
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <Select
          items={kindItems}
          value={checkKind}
          disabled={add.isPending}
          onValueChange={(value) => {
            if (value) setCheckKind(value as CheckKind);
          }}
        >
          <SelectTrigger aria-label={t(($) => $.outcomes.acceptance.check_kind_label)} className="w-full">
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
      <DialogFooter>
        <DialogClose render={<Button variant="outline" />}>{t(($) => $.outcomes.cancel)}</DialogClose>
        <Button type="submit" disabled={!canSubmit} aria-busy={add.isPending}>
          {t(($) => $.outcomes.acceptance.submit)}
        </Button>
      </DialogFooter>
    </form>
  );
}

/** The result of each criterion's latest check. A criterion never checked has no entry. */
function latestResults(verifications: Verification[]): Map<string, VerificationResult> {
  const latest = new Map<string, Verification>();
  for (const check of verifications) {
    if (!check.criterion_id) continue;
    const previous = latest.get(check.criterion_id);
    if (!previous || check.created_at > previous.created_at) latest.set(check.criterion_id, check);
  }
  return new Map([...latest].map(([id, check]) => [id, check.result]));
}

/** Whole days, hours and minutes until `settleAt`, or null once it has passed. */
export function timeUntil(settleAt: string, now: number): { days: number; hours: number; minutes: number } | null {
  const minutes = Math.floor((Date.parse(settleAt) - now) / 60_000);
  if (!Number.isFinite(minutes) || minutes <= 0) return null;
  return { days: Math.floor(minutes / 1440), hours: Math.floor((minutes % 1440) / 60), minutes: minutes % 60 };
}
