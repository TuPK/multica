import { defineCollection } from "astro:content";
import { glob } from "astro/loaders";
import { z } from "astro/zod";

// User guide chapters. One Markdown file per chapter, one folder per language (vi, en).
const guide = defineCollection({
  loader: glob({ pattern: "**/*.md", base: "./src/content/guide" }),
  schema: z.object({
    title: z.string(),
    order: z.number(),
  }),
});

export const collections = { guide };
