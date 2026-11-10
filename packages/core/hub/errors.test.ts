// @vitest-environment node
import { describe, expect, it } from "vitest";
import { ApiError } from "../api/client";
import {
  HubImportBlockedError,
  HubImportWarningsError,
  HubPreviewExpiredError,
  HubSourceInUseError,
  mapHubImportCommitError,
  mapHubSourceDeleteError,
} from "./errors";

describe("mapHubSourceDeleteError", () => {
  it("turns a 409 with in_use into HubSourceInUseError carrying the list", () => {
    const error = new ApiError("in use", 409, "Conflict", {
      in_use: [{ kind: "agent", id: "a1", name: "Writer" }, { kind: "skill", id: "s1" }],
    });
    const mapped = mapHubSourceDeleteError(error);
    expect(mapped).toBeInstanceOf(HubSourceInUseError);
    expect((mapped as HubSourceInUseError).inUse).toEqual([
      { kind: "agent", id: "a1", name: "Writer" },
      { kind: "skill", id: "s1", name: "" },
    ]);
  });

  it("passes other errors through unchanged", () => {
    const notFound = new ApiError("missing", 404, "Not Found", {});
    expect(mapHubSourceDeleteError(notFound)).toBe(notFound);
    const conflictWithoutList = new ApiError("conflict", 409, "Conflict", { error: "x" });
    expect(mapHubSourceDeleteError(conflictWithoutList)).toBe(conflictWithoutList);
  });
});

describe("mapHubImportCommitError", () => {
  it("maps the blocked and unconfirmed-warning codes on 409", () => {
    const blocked = new ApiError("scan blocked", 409, "Conflict", { error: "scan blocked", code: "scan_blocked" });
    expect(mapHubImportCommitError(blocked)).toBeInstanceOf(HubImportBlockedError);
    const warned = new ApiError("confirm", 409, "Conflict", { error: "confirm", code: "warnings_unconfirmed" });
    expect(mapHubImportCommitError(warned)).toBeInstanceOf(HubImportWarningsError);
  });

  it("maps 410 to HubPreviewExpiredError", () => {
    const expired = new ApiError("gone", 410, "Gone", { error: "expired" });
    expect(mapHubImportCommitError(expired)).toBeInstanceOf(HubPreviewExpiredError);
  });

  it("leaves a 409 without a known code as the original ApiError", () => {
    const unknown = new ApiError("conflict", 409, "Conflict", { error: "other", code: "something_else" });
    expect(mapHubImportCommitError(unknown)).toBe(unknown);
  });

  it("leaves non-HTTP failures untouched", () => {
    const network = new TypeError("fetch failed");
    expect(mapHubImportCommitError(network)).toBe(network);
  });
});
