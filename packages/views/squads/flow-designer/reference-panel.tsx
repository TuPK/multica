"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspaceId } from "@multica/core/hooks";
import { companySquadMembersOptions } from "@multica/core/company";
import { agentListOptions, skillDetailOptions, squadSkillsOptions } from "@multica/core/workspace/queries";
import { Button } from "@multica/ui/components/ui/button";
import { BookOpen, ChevronDown, ChevronRight } from "@multica/ui/icons";
import { useT } from "../../i18n";

export interface ReferencePanelProps {
  squadId: string;
}

/** Text as it is: a `pre` that holds characters, never markup, so what a file says cannot act on the page. */
function Plain({ text }: { text: string }) {
  const { t } = useT("squads");
  return (
    <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted/40 p-2 text-micro text-foreground">
      {text.trim() === "" ? t(($) => $.flow_designer.reference.no_text) : text}
    </pre>
  );
}

function SkillText({ skillId }: { skillId: string }) {
  const wsId = useWorkspaceId();
  const query = useQuery(skillDetailOptions(wsId, skillId));
  if (!query.data) return null;
  return (
    <div className="space-y-2">
      <Plain text={query.data.content} />
      {query.data.files.map((file) => (
        <details key={file.id} className="text-micro">
          <summary className="cursor-pointer font-mono text-muted-foreground">{file.path}</summary>
          <Plain text={file.content} />
        </details>
      ))}
    </div>
  );
}

function Skill({ id, name }: { id: string; name: string }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="space-y-1">
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open} className="flex items-center gap-1 text-caption font-medium text-foreground">
        {open ? <ChevronDown className="size-3.5" aria-hidden /> : <ChevronRight className="size-3.5" aria-hidden />}
        <span>{name}</span>
      </button>
      {open && <SkillText skillId={id} />}
    </div>
  );
}

function Contents({ squadId }: { squadId: string }) {
  const { t } = useT("squads");
  const wsId = useWorkspaceId();
  const membersQuery = useQuery(companySquadMembersOptions(wsId, squadId));
  const agentsQuery = useQuery(agentListOptions(wsId));
  const skillsQuery = useQuery(squadSkillsOptions(wsId, squadId));

  const memberIds = new Set((membersQuery.data ?? []).filter((m) => m.member_type === "agent").map((m) => m.member_id));
  const agents = (agentsQuery.data ?? []).filter((a) => memberIds.has(a.id) && !a.archived_at);
  const skills = skillsQuery.data ?? [];

  if (agents.length === 0 && skills.length === 0 && !membersQuery.isLoading && !skillsQuery.isLoading) {
    return <p className="p-3 text-caption text-muted-foreground">{t(($) => $.flow_designer.reference.empty)}</p>;
  }
  return (
    <div className="space-y-4 p-3">
      {agents.length > 0 && (
        <section className="space-y-2" aria-labelledby="reference-agents">
          <h3 id="reference-agents" className="text-caption font-semibold text-foreground">
            {t(($) => $.flow_designer.reference.agents)}
          </h3>
          {agents.map((agent) => (
            <div key={agent.id} className="space-y-1">
              <div className="text-caption font-medium text-foreground">{agent.name}</div>
              <Plain text={agent.instructions} />
            </div>
          ))}
        </section>
      )}
      {skills.length > 0 && (
        <section className="space-y-2" aria-labelledby="reference-skills">
          <h3 id="reference-skills" className="text-caption font-semibold text-foreground">
            {t(($) => $.flow_designer.reference.skills)}
          </h3>
          {skills.map((skill) => (
            <Skill key={skill.id} id={skill.id} name={skill.name} />
          ))}
        </section>
      )}
    </div>
  );
}

/**
 * Where the person looks up the procedure the workflow comes from: the instructions of the agents of the
 * squad and the files of its skills. It folds away, reads nothing until it is opened, and cannot change anything.
 */
export function ReferencePanel({ squadId }: ReferencePanelProps) {
  const { t } = useT("squads");
  const [open, setOpen] = useState(false);
  return (
    <aside className={`flex min-h-0 shrink-0 flex-col border-l border-border bg-card ${open ? "w-72 lg:w-80" : ""}`} aria-label={t(($) => $.flow_designer.reference.title)}>
      <Button
        type="button"
        variant="ghost"
        size="xs"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className="m-1 justify-start gap-1.5 text-caption"
      >
        <BookOpen className="size-3.5" aria-hidden />
        <span>{open ? t(($) => $.flow_designer.reference.hide) : t(($) => $.flow_designer.reference.show)}</span>
      </Button>
      {open && (
        <div className="min-h-0 flex-1 overflow-y-auto">
          <Contents squadId={squadId} />
        </div>
      )}
    </aside>
  );
}
