// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { Attachment } from "../types";
import { latestProposalFile, proposalSource } from "./proposal";

const att = (over: Partial<Attachment>): Attachment => ({
  id: "att-1",
  filename: "flow.json",
  uploader_type: "agent",
  uploader_id: "ag-1",
  created_at: "2026-11-14T10:00:00Z",
  ...over,
}) as Attachment;

describe("proposalSource", () => {
  it("reads the issue and the agent a proposal was made from", () => {
    expect(proposalSource({ issue_id: "iss-1", agent_id: "ag-1" })).toEqual({ issue_id: "iss-1", agent_id: "ag-1" });
  });

  it("reads anything else as no source", () => {
    for (const ref of [null, undefined, "x", 4, [], {}, { issue_id: 3 }, { agent_id: "ag-1" }]) expect(proposalSource(ref)).toBeNull();
  });

  it("keeps the issue when the agent is not named", () => {
    expect(proposalSource({ issue_id: "iss-1" })).toEqual({ issue_id: "iss-1", agent_id: "" });
  });
});

describe("latestProposalFile", () => {
  it("is the newest file ending in flow.json that the agent attached", () => {
    const files = [
      att({ id: "old", created_at: "2026-11-14T09:00:00Z" }),
      att({ id: "new", filename: "intake.flow.json", created_at: "2026-11-14T11:00:00Z" }),
      att({ id: "note", filename: "notes.md", created_at: "2026-11-14T12:00:00Z" }),
    ];
    expect(latestProposalFile(files, "ag-1")?.id).toBe("new");
  });

  it("ignores a file of a person, of another agent, and a name that only contains the ending", () => {
    const files = [
      att({ id: "person", uploader_type: "member" }),
      att({ id: "other", uploader_id: "ag-2" }),
      att({ id: "bak", filename: "flow.json.bak" }),
    ];
    expect(latestProposalFile(files, "ag-1")).toBeNull();
  });

  it("is none for no files, and for a source with no agent", () => {
    expect(latestProposalFile([], "ag-1")).toBeNull();
    expect(latestProposalFile([att({})], "")).toBeNull();
  });
});
