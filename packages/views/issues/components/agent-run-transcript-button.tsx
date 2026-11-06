"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { issueTasksOptions } from "@multica/core/issues/queries";
import { TranscriptButton } from "../../common/task-transcript";
import { useT } from "../../i18n";
import { pickAgentRunTask } from "./agent-run-transcript";

interface AgentRunTranscriptButtonProps {
  issueId: string;
  agentId: string;
  agentName: string;
  className?: string;
}

/**
 * Transcript icon for one agent's run on an issue: the live run while one is
 * running, otherwise the most recent run. Renders nothing when there is no run
 * to open. Reads the issue's task list, which the execution log and header chip
 * share, so it adds no request of its own on an issue already open.
 */
export function AgentRunTranscriptButton({
  issueId,
  agentId,
  agentName,
  className,
}: AgentRunTranscriptButtonProps) {
  const { t } = useT("issues");
  const { data: tasks } = useQuery(issueTasksOptions(issueId));
  const task = useMemo(
    () => (tasks ? pickAgentRunTask(tasks, agentId) : null),
    [tasks, agentId],
  );
  if (!task) return null;
  return (
    <TranscriptButton
      // Keyed so a switch to another run never reuses this run's loaded transcript.
      key={task.id}
      task={task}
      agentName={agentName}
      isLive={task.status === "running"}
      title={t(($) => $.execution_log.transcript_tooltip)}
      className={className}
    />
  );
}
