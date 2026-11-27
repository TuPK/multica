"use client";

import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { ApiError, errorCode } from "@multica/core/api";
import {
  useSeatNecessityTests,
  useSeatSkills,
  useStartNecessityTest,
  type NecessityState,
  type SeatSkill,
} from "@multica/core/durable/necessity";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { useSeatsEnabled } from "./use-seats-enabled";

const STATE_TONE: Record<NecessityState, string> = {
  running: "text-info",
  not_needed: "text-warning",
  needed: "text-success",
  inconclusive: "text-muted-foreground",
  stopped: "text-muted-foreground",
};

/**
 * Lists recent scaffolding necessity tests on a seat (durable spec 10.2 and 10.3) and lets a
 * person start a test on one of the seat's skills. Hidden when `durable_seats` is off.
 */
export function NecessityTests({ wsId, agentId }: { wsId: string; agentId: string }) {
  const { t } = useT("durable");
  const seatsEnabled = useSeatsEnabled();
  const [startOpen, setStartOpen] = useState(false);

  const testsQuery = useSeatNecessityTests(wsId, agentId, seatsEnabled);
  const skillsQuery = useSeatSkills(wsId, agentId, seatsEnabled);

  if (!seatsEnabled) return null;

  const tests = testsQuery.data ?? [];
  const skills = skillsQuery.data ?? [];

  const skillNameById = new Map(skills.map((skill) => [skill.id, skill.name]));
  const runningSkillRefs = new Set(
    tests
      .filter((test) => test.state === "running" && test.subject_type === "skill")
      .map((test) => test.subject_ref),
  );

  return (
    <div className="flex flex-col gap-3 pt-2" aria-label={t(($) => $.seats.necessity.title)}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-title-sm font-medium">{t(($) => $.seats.necessity.title)}</h3>
        <Dialog open={startOpen} onOpenChange={setStartOpen}>
          <DialogTrigger render={<Button type="button" size="sm" variant="outline" />}>
            {t(($) => $.seats.necessity.start)}
          </DialogTrigger>
          <DialogContent className="max-h-[85vh] overflow-y-auto">
            {startOpen ? (
              <StartNecessityForm
                wsId={wsId}
                agentId={agentId}
                skills={skills}
                skillsPending={skillsQuery.isPending}
                skillsError={skillsQuery.isError}
                runningSkillRefs={runningSkillRefs}
                onDone={() => setStartOpen(false)}
              />
            ) : null}
          </DialogContent>
        </Dialog>
      </div>

      {testsQuery.isPending ? (
        <p className="text-caption text-muted-foreground">{t(($) => $.seats.necessity.loading)}</p>
      ) : null}

      {testsQuery.isError ? (
        <p role="alert" className="text-caption text-destructive">
          {t(($) => $.seats.necessity.load_failed)}
        </p>
      ) : null}

      {!testsQuery.isPending && !testsQuery.isError && tests.length === 0 ? (
        <p className="text-body text-muted-foreground">{t(($) => $.seats.necessity.empty)}</p>
      ) : null}

      {tests.length > 0 ? (
        <ul className="flex flex-col divide-y rounded-lg border bg-card">
          {tests.map((test) => {
            const label =
              test.subject_type === "skill"
                ? skillNameById.get(test.subject_ref) || test.subject_ref
                : test.subject_ref;
            const hasSamples =
              test.result.pairs > 0 || test.result.shadow > 0 || test.result.primary > 0;
            return (
              <li key={test.id} className="flex flex-col gap-1.5 px-4 py-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex min-w-0 flex-wrap items-center gap-2">
                    <span className="break-all text-body font-medium text-foreground">{label}</span>
                    <span
                      className={cn(
                        "inline-flex w-fit items-center rounded-md bg-muted px-1.5 py-0.5 text-caption font-medium",
                        STATE_TONE[test.state],
                      )}
                    >
                      {t(($) => $.seats.necessity.state[test.state])}
                    </span>
                  </div>
                </div>

                {hasSamples ? (
                  <div className="flex flex-wrap gap-x-4 gap-y-1 tabular-nums text-caption text-muted-foreground">
                    <span>{t(($) => $.seats.necessity.pairs_count, { count: test.result.pairs })}</span>
                    <span>{t(($) => $.seats.necessity.wins_count, { count: test.result.shadow })}</span>
                    <span>{t(($) => $.seats.necessity.losses_count, { count: test.result.primary })}</span>
                  </div>
                ) : null}

                {test.proposal_id ? (
                  <p className="text-caption text-muted-foreground">
                    {t(($) => $.seats.necessity.waiting_for_decision)}
                  </p>
                ) : null}
              </li>
            );
          })}
        </ul>
      ) : null}
    </div>
  );
}

// Mounted only while the dialog is open so reopening starts cleanly,
// while a failed submit preserves the person's selection and shows the error.
function StartNecessityForm({
  wsId,
  agentId,
  skills,
  skillsPending,
  skillsError,
  runningSkillRefs,
  onDone,
}: {
  wsId: string;
  agentId: string;
  skills: SeatSkill[];
  skillsPending: boolean;
  skillsError: boolean;
  runningSkillRefs: ReadonlySet<string>;
  onDone: () => void;
}) {
  const { t } = useT("durable");
  const [selectedSkillId, setSelectedSkillId] = useState("");
  const [serverError, setServerError] = useState<string | null>(null);

  const start = useStartNecessityTest(wsId, agentId);

  const skillItems = skills.map((skill) => ({
    value: skill.id,
    label: skill.name || skill.id,
  }));

  const selectedSkill = skills.find((skill) => skill.id === selectedSkillId);
  const selectedIsRunning = selectedSkill
    ? runningSkillRefs.has(selectedSkill.id) ||
      (selectedSkill.name !== "" && runningSkillRefs.has(selectedSkill.name))
    : false;

  const canSubmit = selectedSkillId !== "" && !selectedIsRunning && !start.isPending;

  const mapError = (err: unknown): string => {
    const code = errorCode(err);
    if (code === "necessity_test_running") return t(($) => $.seats.necessity.error_running);
    if (code === "shadow_already_running") return t(($) => $.seats.necessity.error_shadow_running);
    if (err instanceof ApiError && err.status === 422) {
      return t(($) => $.seats.necessity.error_invalid);
    }
    return t(($) => $.seats.necessity.error_failed);
  };

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!canSubmit) return;
    setServerError(null);
    start
      .mutateAsync({ skill_id: selectedSkillId })
      .then(
        (created) => {
          if (created) {
            toast.success(t(($) => $.seats.necessity.started));
            onDone();
          } else {
            const msg = t(($) => $.seats.necessity.error_failed);
            setServerError(msg);
            toast.error(msg);
          }
        },
        (err: unknown) => {
          const msg = mapError(err);
          setServerError(msg);
          toast.error(msg);
        },
      );
  };

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>{t(($) => $.seats.necessity.dialog_title)}</DialogTitle>
        <DialogDescription>{t(($) => $.seats.necessity.dialog_note)}</DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-1.5">
        <Label>{t(($) => $.seats.necessity.skill_label)}</Label>
        {skillsPending ? (
          <p className="text-caption text-muted-foreground">
            {t(($) => $.seats.necessity.skills_loading)}
          </p>
        ) : skillsError ? (
          <p role="alert" className="text-caption text-destructive">
            {t(($) => $.seats.necessity.skills_load_failed)}
          </p>
        ) : skillItems.length === 0 ? (
          <p className="text-caption text-muted-foreground">
            {t(($) => $.seats.necessity.skills_empty)}
          </p>
        ) : (
          <Select
            items={skillItems}
            value={selectedSkillId}
            disabled={start.isPending}
            onValueChange={(next) => {
              if (next) {
                setSelectedSkillId(next);
                setServerError(null);
              }
            }}
          >
            <SelectTrigger aria-label={t(($) => $.seats.necessity.skill_label)} className="w-full">
              <SelectValue placeholder={t(($) => $.seats.necessity.skill_placeholder)} />
            </SelectTrigger>
            <SelectContent>
              {skillItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {selectedIsRunning ? (
          <p className="text-caption text-muted-foreground">
            {t(($) => $.seats.necessity.error_running)}
          </p>
        ) : null}
      </div>

      {serverError ? (
        <p role="alert" className="text-caption text-destructive">
          {serverError}
        </p>
      ) : null}

      <DialogFooter>
        <DialogClose render={<Button type="button" variant="outline" />}>
          {t(($) => $.seats.necessity.cancel)}
        </DialogClose>
        <Button type="submit" disabled={!canSubmit} aria-busy={start.isPending}>
          {t(($) => $.seats.necessity.submit)}
        </Button>
      </DialogFooter>
    </form>
  );
}
