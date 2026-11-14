"use client";

import type { PackPreview, PackSkillConflictChoice } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { useT } from "../../i18n";
import type { PackChoices } from "./pack-choices";

export interface PackPreviewSkillsProps {
  preview: PackPreview;
  choices: PackChoices;
  onChange: (patch: Partial<PackChoices>) => void;
}

/** The skills of the pack. A name the workspace has already asks whether to keep that skill or import this one as new. */
export function PackPreviewSkills({ preview, choices, onChange }: PackPreviewSkillsProps) {
  const { t } = useT("squads");
  if (preview.skills.length === 0) return null;
  return (
    <section className="space-y-2" aria-labelledby="pack-skills-title">
      <h3 id="pack-skills-title" className="text-caption font-semibold text-foreground">
        {t(($) => $.packs.skills)}
      </h3>
      <ul className="divide-y divide-border rounded-lg border border-border">
        {preview.skills.map((skill) => (
          <li key={skill.name} className="flex flex-wrap items-center justify-between gap-2 p-3">
            <div className="flex min-w-0 items-center gap-2">
              <span className="truncate text-caption font-medium text-foreground">{skill.name}</span>
              {skill.existing_skill_id && (
                <Badge variant="outline" className="text-micro font-normal">
                  {skill.same_content ? t(($) => $.packs.skill.same) : t(($) => $.packs.skill.differs)}
                </Badge>
              )}
            </div>
            {skill.existing_skill_id && (
              <select
                value={choices.skills[skill.name] ?? "keep_existing"}
                onChange={(e) => onChange({ skills: { ...choices.skills, [skill.name]: e.target.value as PackSkillConflictChoice } })}
                aria-label={skill.name}
                className="h-8 max-w-full rounded-md border border-input bg-background px-2 text-caption"
              >
                <option value="keep_existing">{t(($) => $.packs.skill.keep)}</option>
                <option value="import_as_new">{t(($) => $.packs.skill.new)}</option>
              </select>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
