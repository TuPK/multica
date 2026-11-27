import type { CheckKind } from "./schemas";

// The check_spec each check kind takes (durable spec 2.5). The server refuses a criterion whose
// spec does not match its kind (validateCriterionSpec), so the form applies the same rules and
// only sends a spec the server will keep.
export const CRITERION_RUBRIC_MAX = 2000;
const WITHIN_DAYS_MAX = 365;
const WHOLE_NUMBER = /^-?\d+$/;

/** The raw text of each check field, as typed. Only the fields of the chosen kind are read. */
export type CheckSpecFields = {
  rubric: string;
  run: string;
  expectExit: string;
  artifactKind: string;
  metricKey: string;
  metricValue: string;
  withinDays: string;
};

export function emptyCheckSpecFields(): CheckSpecFields {
  return { rubric: "", run: "", expectExit: "", artifactKind: "", metricKey: "", metricValue: "", withinDays: "" };
}

/** `spec` is undefined for a manual check, which has none; `ok: false` means the form must not submit. */
export type CheckSpecResult = { ok: true; spec: Record<string, unknown> | undefined } | { ok: false };

export function checkSpecFor(kind: CheckKind, input: CheckSpecFields): CheckSpecResult {
  switch (kind) {
    case "manual":
      return { ok: true, spec: undefined };
    case "agent_judgment": {
      const rubric = input.rubric.trim();
      if (rubric === "" || [...rubric].length > CRITERION_RUBRIC_MAX) return { ok: false };
      return { ok: true, spec: { rubric } };
    }
    case "command": {
      const run = input.run.trim();
      if (run === "") return { ok: false };
      const exit = input.expectExit.trim();
      if (exit === "") return { ok: true, spec: { run } };
      if (!WHOLE_NUMBER.test(exit)) return { ok: false };
      return { ok: true, spec: { run, expect_exit: Number(exit) } };
    }
    case "artifact": {
      const kindOfDocument = input.artifactKind.trim();
      if (kindOfDocument === "") return { ok: false };
      return { ok: true, spec: { kind: kindOfDocument } };
    }
    case "metric": {
      const key = input.metricKey.trim();
      const value = input.metricValue.trim();
      const days = input.withinDays.trim();
      if (key === "" || value === "" || !Number.isFinite(Number(value))) return { ok: false };
      if (!WHOLE_NUMBER.test(days) || Number(days) < 1 || Number(days) > WITHIN_DAYS_MAX) return { ok: false };
      return {
        ok: true,
        spec: { metric_key: key, scope: "company", op: ">=", value: Number(value), within_days: Number(days) },
      };
    }
  }
}
