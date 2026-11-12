// @vitest-environment node
import { describe, expect, it } from "vitest";
import { ApiError } from "../api";
import { issueWorkflowMappingPlanFrom, issueWorkflowStatusCreateForbidden } from "./mapping";

const plan = {
  required: [{ status_key: "in_review", issue_count: 2, suggested_status_key: "in_progress" }],
  unchanged: [{ status_key: "todo", issue_count: 5 }],
  total_issues: 7,
  affected_issues: 2,
};

describe("issueWorkflowMappingPlanFrom", () => {
  it("reads the plan from a 409 status-mapping requirement", () => {
    const err = new ApiError("conflict", 409, "Conflict", {
      code: "workflow_status_mapping_required",
      plan,
    });

    expect(issueWorkflowMappingPlanFrom(err)).toEqual(plan);
  });

  it("defaults the counts a plan may omit", () => {
    const err = new ApiError("conflict", 409, "Conflict", {
      code: "workflow_status_mapping_required",
      plan: { required: [{ status_key: "in_review" }] },
    });

    expect(issueWorkflowMappingPlanFrom(err)).toEqual({
      required: [{ status_key: "in_review", issue_count: 0, suggested_status_key: "" }],
      unchanged: [],
      total_issues: 0,
      affected_issues: 0,
    });
  });

  it("is null for other errors, other statuses and a body without a plan", () => {
    expect(issueWorkflowMappingPlanFrom(new Error("network"))).toBeNull();
    expect(
      issueWorkflowMappingPlanFrom(new ApiError("bad", 400, "Bad Request", {
        code: "workflow_status_mapping_required",
        plan,
      })),
    ).toBeNull();
    expect(
      issueWorkflowMappingPlanFrom(new ApiError("conflict", 409, "Conflict", {
        code: "workflow_in_use",
        reasons: [],
      })),
    ).toBeNull();
    expect(
      issueWorkflowMappingPlanFrom(new ApiError("conflict", 409, "Conflict", {
        code: "workflow_status_mapping_required",
      })),
    ).toBeNull();
  });
});

describe("issueWorkflowStatusCreateForbidden", () => {
  it("recognizes the refusal to create a status", () => {
    const err = new ApiError("only workspace admins can create statuses", 403, "Forbidden", {
      error: "only workspace admins can create statuses",
      code: "status_create_forbidden",
    });
    expect(issueWorkflowStatusCreateForbidden(err)).toBe(true);
  });

  it("does not match another 403, a 409, or a malformed body", () => {
    expect(issueWorkflowStatusCreateForbidden(new ApiError("no", 403, "Forbidden", { code: "other" }))).toBe(false);
    expect(issueWorkflowStatusCreateForbidden(new ApiError("conflict", 409, "Conflict", { code: "status_create_forbidden" }))).toBe(false);
    expect(issueWorkflowStatusCreateForbidden(new ApiError("no", 403, "Forbidden", "not json"))).toBe(false);
    expect(issueWorkflowStatusCreateForbidden(new Error("network"))).toBe(false);
  });
});
