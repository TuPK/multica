// Groups a conflicted or changed path by what it is, so a sync report shows whether
// the pain is locale/doc/generated churn (machine-resolvable) or real code.
// Same buckets as docs/specs/upstream-sync/spec.md section 2.2.
export function classify(path) {
  if (/(^|\/)locales\/.+\.json$/.test(path)) return "locales";
  if (/\/generated\/|(^|\/)pnpm-lock\.yaml$|\.sum$|shard-timings\.json$|reserved-slugs\.ts$/.test(path)) return "generated/lock";
  if (/^apps\/docs\/|\.mdx?$/.test(path)) return "docs/md";
  if (/_test\.go$|\.test\.(tsx?|mjs)$|(^|\/)e2e\//.test(path)) return "tests";
  if (/^server\/migrations\//.test(path)) return "migrations";
  if (/(^|\/)package\.json$|knip|turbo\.json|^\.github\/|Makefile|docker|\.env|^scripts\/|pnpm-workspace/.test(path)) return "config/build";
  if (/\.go$/.test(path)) return "go-code";
  if (/\.tsx?$/.test(path)) return "ts-code";
  return "other";
}

export function countByClass(paths) {
  const out = {};
  for (const p of paths) out[classify(p)] = (out[classify(p)] ?? 0) + 1;
  return out;
}
