"use client";

import { useState } from "react";
import { toast } from "sonner";
import { PRIORITY_DISPLAY_ORDER } from "@multica/core/issues/config";
import { useCreateIssue } from "@multica/core/issues/mutations";
import { findSquadProject } from "@multica/core/company";
import { projectListOptions } from "@multica/core/projects/queries";
import { useWorkspaceId } from "@multica/core/hooks";
import { useQuery } from "@tanstack/react-query";
import type { Agent, IssuePriority, IssueStatus } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@multica/ui/components/ui/popover";
import { Switch } from "@multica/ui/components/ui/switch";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { useT } from "../../i18n";
import { PillButton } from "../../common/pill-button";
import { PriorityIcon } from "../../issues/components/priority-icon";
import { ProjectPicker } from "../../projects/components/project-picker";
import { RunQueueWaitPicker } from "../../issues/components/run-queue-control";
import { useRunQueueApply } from "../../issues/components/use-run-queue-apply";
import { StatusPicker } from "../../issues/components/pickers/status-picker";
import { priorityLabel } from "../../issues/utils/priority-label";

/**
 * A quick issue form, handed to the agent by default. Handing it over starts the
 * issue as `todo` so the agent picks it up; without that it waits in the backlog.
 * The status picker overrides that default, offering the project's workflow
 * columns, or the workspace statuses when the issue has no project.
 */
export function CompanyAgentNewIssue({
  agent,
  projectId,
  onCreated,
  onCancel,
}: {
  agent: Agent;
  projectId?: string;
  /** Called with the new issue's id once it exists. */
  onCreated: (issueId: string) => void;
  onCancel: () => void;
}) {
  const { t } = useT("issues");
  const create = useCreateIssue();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState<IssuePriority>("none");
  const [assign, setAssign] = useState(true);
  // Unset until the user picks, so the status follows the assign switch by default.
  const [chosenStatus, setChosenStatus] = useState<IssueStatus | null>(null);
  // Starts from the project the panel was opened in; the picker can move the issue elsewhere.
  const [chosenProject, setChosenProject] = useState<string | null>(projectId ?? null);
  // An issue handed to an agent of a squad with a project belongs to that project.
  const wsId = useWorkspaceId();
  const { data: projects = [] } = useQuery(projectListOptions(wsId));
  const squadProject = assign ? findSquadProject(projects, agent.home_squad_id) : undefined;
  const effectiveProjectId = squadProject ? squadProject.id : chosenProject;
  const status: IssueStatus = chosenStatus ?? (assign ? "todo" : "backlog");
  // Only backlog issues join the run queue; the control is hidden for other statuses.
  const [queued, setQueued] = useState(false);
  const [queueOpen, setQueueOpen] = useState(false);
  const [waits, setWaits] = useState<string[]>([]);
  const { apply, dialog } = useRunQueueApply();

  const submit = () => {
    const trimmed = title.trim();
    if (!trimmed || create.isPending) return;
    const queueNew = queued && status === "backlog";
    create.mutate(
      {
        title: trimmed,
        ...(description.trim() ? { description: description.trim() } : {}),
        priority,
        status,
        ...(assign ? { assignee_type: "agent" as const, assignee_id: agent.id } : {}),
        ...(effectiveProjectId ? { project_id: effectiveProjectId } : {}),
      },
      {
        onSuccess: (issue) => {
          toast.success(t(($) => $.company.new_created, { identifier: issue.identifier }));
          if (!queueNew) {
            onCreated(issue.id);
            return;
          }
          // The form stays mounted until the queue is written, so a confirmation
          // about starting runs now can be answered here.
          const done = () => onCreated(issue.id);
          void apply([issue.id], { queued: true, afterIssueIds: waits }, { onDone: done, onCancel: done });
        },
        onError: () => toast.error(t(($) => $.company.new_failed)),
      },
    );
  };

  return (
    <form
      className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="company-new-issue-title">{t(($) => $.company.new_title)}</Label>
        <Input id="company-new-issue-title" autoFocus value={title} onChange={(event) => setTitle(event.target.value)} />
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="company-new-issue-description">{t(($) => $.company.new_description)}</Label>
        <Textarea
          id="company-new-issue-description"
          value={description}
          onChange={(event) => setDescription(event.target.value)}
          className="min-h-28"
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <span className="text-caption font-medium">{t(($) => $.detail.prop_status)}</span>
        <div>
          <StatusPicker
            status={status}
            projectId={effectiveProjectId}
            onUpdate={(updates) => {
              if (updates.status) setChosenStatus(updates.status);
            }}
            triggerRender={<PillButton />}
            align="start"
          />
        </div>
      </div>
      <div className="flex flex-col gap-1.5">
        <span className="text-caption font-medium">{t(($) => $.detail.prop_project)}</span>
        <div>
          <ProjectPicker
            projectId={effectiveProjectId}
            onUpdate={(updates) => {
              if ("project_id" in updates) setChosenProject(updates.project_id ?? null);
            }}
            disabled={!!squadProject}
            triggerRender={<PillButton disabled={!!squadProject} />}
            align="start"
          />
        </div>
      </div>
      <div className="flex flex-col gap-1.5">
        <span className="text-caption font-medium">{t(($) => $.company.new_priority)}</span>
        <div className="flex flex-wrap gap-1.5">
          {PRIORITY_DISPLAY_ORDER.map((value) => (
            <Button
              key={value}
              type="button"
              size="xs"
              variant={priority === value ? "secondary" : "outline"}
              aria-pressed={priority === value}
              onClick={() => setPriority(value)}
            >
              <PriorityIcon priority={value} />
              {priorityLabel(value, t)}
            </Button>
          ))}
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Switch id="company-new-issue-assign" checked={assign} onCheckedChange={setAssign} />
        <Label htmlFor="company-new-issue-assign">{t(($) => $.company.new_assign, { name: agent.name })}</Label>
      </div>
      {status === "backlog" ? (
        <div className="flex flex-col gap-1.5">
          <span className="text-caption font-medium">{t(($) => $.run_queue.label)}</span>
          <div>
            <Popover open={queueOpen} onOpenChange={setQueueOpen}>
              <PopoverTrigger render={<PillButton />}>
                {queued ? t(($) => $.run_queue.queued) : t(($) => $.run_queue.queue)}
              </PopoverTrigger>
              <PopoverContent align="start" className="w-80 gap-0 p-0">
                <div className="flex items-center justify-between gap-2 border-b px-2.5 py-2">
                  <Label htmlFor="company-new-issue-queue">{t(($) => $.company.new_queue)}</Label>
                  <Switch id="company-new-issue-queue" checked={queued} onCheckedChange={setQueued} />
                </div>
                {queued ? (
                  <>
                    <RunQueueWaitPicker excludeIds={[]} waits={waits} onChange={setWaits} />
                    <div className="border-t px-2.5 py-2 text-caption text-muted-foreground">
                      {waits.length === 0
                        ? t(($) => $.run_queue.none_selected)
                        : t(($) => $.run_queue.selected_count, { count: waits.length })}
                    </div>
                  </>
                ) : null}
              </PopoverContent>
            </Popover>
          </div>
        </div>
      ) : null}
      <div className="mt-auto flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onCancel}>
          {t(($) => $.company.new_cancel)}
        </Button>
        <Button type="submit" disabled={!title.trim() || create.isPending}>
          {t(($) => $.company.new_create)}
        </Button>
      </div>
      {dialog}
    </form>
  );
}
