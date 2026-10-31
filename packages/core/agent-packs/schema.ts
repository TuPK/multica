import { z } from "zod";

/** Same rule the skill importer and the install script apply to skill names. */
const SKILL_NAME = /^[a-z0-9][a-z0-9._-]*$/;

export const AgentPackSchema = z.object({
  id: z.string().min(1),
  agent: z.object({
    name: z.string().min(1),
    description: z.string(),
    instructions: z.string().min(1),
    max_concurrent_tasks: z.number().int().positive(),
  }),
  skills: z
    .array(
      z.object({
        name: z.string().regex(SKILL_NAME),
        files: z
          .array(z.object({ path: z.string().min(1), content: z.string() }))
          .refine((files) => files.some((f) => f.path === "SKILL.md"), {
            message: "skill has no SKILL.md",
          }),
      }),
    )
    .min(1),
});

export type AgentPack = z.infer<typeof AgentPackSchema>;
