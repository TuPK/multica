"use client";

import type { AgentRuntime, PackPreview } from "@multica/core/types";
import { useT } from "../../i18n";
import { leaderIsValid, type PackChoices } from "./pack-choices";
import { runtimeLabel } from "./pack-preview-agents";

export interface PackLeaderPickerProps {
  preview: PackPreview;
  choices: PackChoices;
  runtimes: AgentRuntime[];
  onChange: (patch: Partial<PackChoices>) => void;
}

/**
 * Who leads the squad: one of the agents that will be in it, or a general leader the install makes. A pack
 * of the native format names its lead, and the server accepts no other of its agents.
 */
export function PackLeaderPicker({ preview, choices, runtimes, onChange }: PackLeaderPickerProps) {
  const { t } = useT("squads");
  const native = preview.format === "native_bundle";
  const options = [
    ...preview.agents
      .filter((a) => choices.agents[a.key] && choices.agents[a.key]!.action !== "skip")
      .filter((a) => !native || !preview.leader || a.key === preview.leader)
      .map((a) => ({ key: a.key, name: choices.agents[a.key]!.name.trim() || a.name })),
    ...choices.extras.map((e) => ({ key: e.key, name: e.name.trim() || e.key })),
  ];

  return (
    <section className="space-y-2" aria-labelledby="pack-leader-title">
      <h3 id="pack-leader-title" className="text-caption font-semibold text-foreground">
        {t(($) => $.packs.leader)}
      </h3>
      <div role="radiogroup" aria-labelledby="pack-leader-title" className="flex flex-wrap gap-x-4 gap-y-1.5">
        {options.map((o) => (
          <label key={o.key} className="flex cursor-pointer items-center gap-1.5 text-caption">
            <input
              type="radio"
              name="pack-leader"
              checked={choices.leader.kind === "agent" && choices.leader.key === o.key}
              onChange={() => onChange({ leader: { kind: "agent", key: o.key } })}
              className="size-3.5"
            />
            <span>{o.name}</span>
          </label>
        ))}
        <label className="flex cursor-pointer items-center gap-1.5 text-caption">
          <input
            type="radio"
            name="pack-leader"
            checked={choices.leader.kind === "generic"}
            onChange={() => onChange({ leader: { kind: "generic" } })}
            className="size-3.5"
          />
          <span>{t(($) => $.packs.leader_generic)}</span>
        </label>
      </div>
      {choices.leader.kind === "generic" && (
        <label className="flex flex-wrap items-center gap-2 text-caption">
          <span className="text-muted-foreground">{t(($) => $.packs.runtime)}</span>
          <select
            value={choices.genericRuntimeId}
            onChange={(e) => onChange({ genericRuntimeId: e.target.value })}
            aria-label={t(($) => $.packs.runtime)}
            className="h-8 max-w-full rounded-md border border-input bg-background px-2 text-caption"
          >
            {runtimes.length === 0 && <option value="">{t(($) => $.packs.runtime_none)}</option>}
            {runtimes.map((r) => (
              <option key={r.id} value={r.id}>
                {runtimeLabel(r)}
              </option>
            ))}
          </select>
        </label>
      )}
      {choices.leader.kind === "agent" && !leaderIsValid(preview, choices) && (
        <p className="text-micro text-muted-foreground">{t(($) => $.packs.disabled.leader)}</p>
      )}
    </section>
  );
}
