import { describe, expect, it } from "vitest";
import { ApiError } from "@multica/core/api";
import { issueUpdateErrorMessage } from "./issue-update-error";
import type { IssuesT } from "./priority-label";

// A translator that answers with the dictionary path its selector reads, so a
// test can name the copy key that was chosen without loading the locale files.
const keyT = ((selector: (dict: unknown) => string) => {
  const path: string[] = [];
  const proxy: unknown = new Proxy({}, {
    get(_target, key) {
      path.push(String(key));
      return proxy;
    },
  });
  selector(proxy);
  return path.join(".");
}) as unknown as IssuesT;

const FALLBACK = "Could not update the issue";

// The API client attaches the stable error code as body.code on a failed request.
function failure(code: string, message = "server text"): ApiError {
  return new ApiError(message, 422, "Unprocessable Entity", { code });
}

describe("issueUpdateErrorMessage", () => {
  it.each([
    "flow_outcome_required",
    "flow_outcome_invalid",
    "flow_attachment_required",
    "flow_human_only",
    "flow_managed_request",
    "flow_children_managed",
    "flow_gate_same_agent",
    "flow_forbidden",
    "flow_not_managed",
  ])("names the localised copy for the guard code %s", (code) => {
    expect(issueUpdateErrorMessage(failure(code), keyT, FALLBACK)).toBe(`flow.errors.${code}`);
  });

  it("names the revision conflict copy", () => {
    expect(issueUpdateErrorMessage(failure("revision_conflict"), keyT, FALLBACK)).toBe(
      "revision.conflict",
    );
  });

  it("passes through the message of an error no copy covers", () => {
    expect(issueUpdateErrorMessage(new Error("boom"), keyT, FALLBACK)).toBe("boom");
  });

  it("uses the fallback when the failure carries no message", () => {
    expect(issueUpdateErrorMessage("not an error", keyT, FALLBACK)).toBe(FALLBACK);
  });
});
