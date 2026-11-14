"use client";

import type { AgentRuntime, PackAgentAction, PackPreview } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Plus, Trash2 } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { extraAgentKey, oneAgentPerSkill, type AgentChoiceState, type ExtraAgentState, type PackChoices } from "./pack-choices";

export function runtimeLabel(runtime: AgentRuntime): string {
  return runtime.custom_name || runtime.name;
}

const SELECT = "h-8 max-w-full rounded-md border border-input bg-background px-2 text-caption";

export interface PackPreviewAgentsProps {
  preview: PackPreview;
  choices: PackChoices;
  runtimes: AgentRuntime[];
  defaultRuntimeId: string;
  onChange: (patch: Partial<PackChoices>) => void;
}

function RuntimeSelect({ value, label, runtimes, onChange }: { value: string; label: string; runtimes: AgentRuntime[]; onChange: (id: string) => void }) {
  const { t } = useT("squads");
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} aria-label={label} className={SELECT}>
      {runtimes.length === 0 && <option value="">{t(($) => $.packs.runtime_none)}</option>}
      {runtimes.map((r) => (
        <option key={r.id} value={r.id}>
          {runtimeLabel(r)}
        </option>
      ))}
    </select>
  );
}

function SkillChecks({ all, chosen, onChange }: { all: string[]; chosen: string[]; onChange: (next: string[]) => void }) {
  return (
    <div className="flex flex-wrap gap-x-3 gap-y-1">
      {all.map((name) => (
        <label key={name} className="flex cursor-pointer items-center gap-1 text-micro">
          <input
            type="checkbox"
            checked={chosen.includes(name)}
            onChange={(e) => onChange(e.target.checked ? [...chosen, name] : chosen.filter((n) => n !== name))}
            className="size-3.5"
          />
          <span>{name}</span>
        </label>
      ))}
    </div>
  );
}

/**
 * The agents of the pack and the ones the person adds. On a narrow screen each agent is a block, and the
 * columns of a table appear from the small breakpoint up.
 */
export function PackPreviewAgents({ preview, choices, runtimes, defaultRuntimeId, onChange }: PackPreviewAgentsProps) {
  const { t } = useT("squads");
  const skillNames = preview.skills.map((s) => s.name);

  const setAgent = (key: string, patch: Partial<AgentChoiceState>) =>
    onChange({ agents: { ...choices.agents, [key]: { ...choices.agents[key]!, ...patch } } });
  const setExtra = (key: string, patch: Partial<ExtraAgentState>) =>
    onChange({ extras: choices.extras.map((e) => (e.key === key ? { ...e, ...patch } : e)) });
  const removeExtra = (key: string) =>
    onChange({
      extras: choices.extras.filter((e) => e.key !== key),
      leader: choices.leader.kind === "agent" && choices.leader.key === key ? { kind: "none" } : choices.leader,
    });
  const addExtra = () => {
    const taken = new Set([...preview.agents.map((a) => a.key), ...choices.extras.map((e) => e.key)]);
    const key = extraAgentKey(`agent ${choices.extras.length + 1}`, taken);
    onChange({ extras: [...choices.extras, { key, name: "", runtimeId: defaultRuntimeId, skills: [] }] });
  };

  const empty = preview.agents.length === 0;

  return (
    <section className="space-y-2" aria-labelledby="pack-agents-title">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 id="pack-agents-title" className="text-caption font-semibold text-foreground">
          {t(($) => $.packs.agents)}
        </h3>
        {empty && (
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" size="xs" onClick={addExtra} className="gap-1">
              <Plus className="size-3" aria-hidden />
              {t(($) => $.packs.add_agent)}
            </Button>
            {preview.skills.length > 0 && (
              <Button
                type="button"
                variant="outline"
                size="xs"
                onClick={() => onChange({ extras: [...choices.extras, ...oneAgentPerSkill(preview, defaultRuntimeId, choices.extras)] })}
              >
                {t(($) => $.packs.one_agent_per_skill)}
              </Button>
            )}
          </div>
        )}
      </div>

      {empty && choices.extras.length === 0 && <p className="text-caption text-muted-foreground">{t(($) => $.packs.no_agents)}</p>}

      <ul className="divide-y divide-border rounded-lg border border-border">
        {preview.agents.map((agent) => {
          const c = choices.agents[agent.key];
          if (!c) return null;
          const actions: PackAgentAction[] = agent.existing_agent_id ? ["use_existing", "create", "skip"] : ["create", "skip"];
          return (
            <li key={agent.key} className="grid grid-cols-1 gap-2 p-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-start">
              <div className="min-w-0 space-y-1">
                <div className="truncate text-caption font-medium text-foreground">{agent.name}</div>
                {agent.skills.length > 0 && <div className="truncate text-micro text-muted-foreground">{agent.skills.join(", ")}</div>}
                {c.action === "create" && (
                  <Input
                    value={c.name}
                    onChange={(e) => setAgent(agent.key, { name: e.target.value })}
                    placeholder={agent.name}
                    aria-label={`${agent.name} ${t(($) => $.packs.agent_name)}`}
                    className="h-7 max-w-xs text-caption"
                  />
                )}
                {c.action === "create" && !agent.skills_declared && skillNames.length > 0 && (
                  <SkillChecks all={skillNames} chosen={c.skills ?? []} onChange={(next) => setAgent(agent.key, { skills: next })} />
                )}
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <select
                  value={c.action}
                  onChange={(e) => setAgent(agent.key, { action: e.target.value as PackAgentAction })}
                  aria-label={`${agent.name} ${t(($) => $.packs.action.label)}`}
                  className={SELECT}
                >
                  {actions.map((a) => (
                    <option key={a} value={a}>
                      {t(($) => $.packs.action[a])}
                    </option>
                  ))}
                </select>
                {c.action === "create" && (
                  <RuntimeSelect
                    value={c.runtimeId}
                    label={`${agent.name} ${t(($) => $.packs.runtime)}`}
                    runtimes={runtimes}
                    onChange={(runtimeId) => setAgent(agent.key, { runtimeId })}
                  />
                )}
              </div>
            </li>
          );
        })}
        {choices.extras.map((extra) => (
          <li key={extra.key} className="grid grid-cols-1 gap-2 p-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-start">
            <div className="min-w-0 space-y-1">
              <Input
                value={extra.name}
                onChange={(e) => setExtra(extra.key, { name: e.target.value })}
                placeholder={extra.key}
                aria-label={t(($) => $.packs.agent_name)}
                className="h-7 max-w-xs text-caption"
              />
              <SkillChecks all={skillNames} chosen={extra.skills} onChange={(next) => setExtra(extra.key, { skills: next })} />
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <RuntimeSelect value={extra.runtimeId} label={`${extra.name || extra.key} ${t(($) => $.packs.runtime)}`} runtimes={runtimes} onChange={(runtimeId) => setExtra(extra.key, { runtimeId })} />
              <Button type="button" variant="ghost" size="icon-xs" aria-label={t(($) => $.packs.remove)} onClick={() => removeExtra(extra.key)}>
                <Trash2 className="size-3.5" aria-hidden />
              </Button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
