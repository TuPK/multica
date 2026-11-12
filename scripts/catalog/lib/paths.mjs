// Locations shared by the catalog CLI. See docs/specs/template-registry/spec.md.
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");
export const catalogDir = join(repoRoot, "catalog");
export const kitvietSourceDir = join(catalogDir, "_sources", "kitviet");

// Generated output. Nothing below these directories is committed (see .gitignore).
export const tsOutDir = join(repoRoot, "packages/core/catalog/generated");
export const goOutDir = join(repoRoot, "server/internal/catalog/generated");

export const supportedLocalesFile = join(repoRoot, "packages/core/i18n/types.ts");
