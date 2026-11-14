// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient } from "./client";

afterEach(() => vi.unstubAllGlobals());

function stubFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockImplementation(
    async () => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const client = () => new ApiClient("https://api.example.test");
const call = (fetchMock: ReturnType<typeof stubFetch>) => {
  const [url, init] = fetchMock.mock.calls[0] ?? [];
  const body = init?.body;
  return {
    url: String(url),
    method: init?.method ?? "GET",
    body: typeof body === "string" ? JSON.parse(body) : body,
  };
};

const digest = "sha256:" + "b".repeat(64);

// The routes of spec 16.2, 16.3, 16.4 and 16.6, with the bodies the server reads.
describe("pack routes", () => {
  it("previews a source by URL", async () => {
    const fetchMock = stubFetch({ digest, name: "Pack", agents: [], skills: [] });
    const preview = await client().previewPack({ kind: "github", url: "https://github.com/acme/pack" });
    expect(call(fetchMock)).toMatchObject({
      method: "POST",
      url: "https://api.example.test/api/squads/packs/preview",
      body: { source: { kind: "github", url: "https://github.com/acme/pack" } },
    });
    expect(preview).toMatchObject({ digest, name: "Pack" });
  });

  it("previews a zip as a multipart upload with the field the server reads", async () => {
    const fetchMock = stubFetch({ digest, name: "Pack" });
    await client().previewPackUpload(new File(["zip"], "pack.zip", { type: "application/zip" }));
    const sent = call(fetchMock);
    expect(sent).toMatchObject({ method: "POST", url: "https://api.example.test/api/squads/packs/preview" });
    expect(sent.body).toBeInstanceOf(FormData);
    expect((sent.body as FormData).get("file")).toBeInstanceOf(File);
  });

  it("refuses a preview that cannot be read instead of handing the screen an empty one", async () => {
    for (const body of ["nope", null, 7, [], { digest: 4, name: [] }]) {
      stubFetch(body);
      await expect(client().previewPack({ kind: "url_zip", url: "https://x.test/p.zip" })).rejects.toThrow(/preview/i);
    }
  });

  it("commits the choices of the person", async () => {
    const fetchMock = stubFetch({ install_id: "i-1", squad_id: "sq-1", agents: [], skills: [] });
    const done = await client().commitPack({ digest, squad: { name: "Writers" }, leader: { generic: true, runtime_id: "rt-1" }, create_draft_from_flow: true, ack_scan_warnings: true });
    expect(call(fetchMock)).toMatchObject({
      method: "POST",
      url: "https://api.example.test/api/squads/packs/commit",
      body: { digest, squad: { name: "Writers" }, leader: { generic: true, runtime_id: "rt-1" }, create_draft_from_flow: true, ack_scan_warnings: true },
    });
    expect(done).toMatchObject({ install_id: "i-1", squad_id: "sq-1" });
  });

  it("refuses a result that says nothing was installed", async () => {
    for (const body of ["nope", null, [], {}]) {
      stubFetch(body);
      await expect(client().commitPack({ digest })).rejects.toThrow(/install/i);
    }
  });

  it("surfaces a conflict as an error that keeps its code and the earlier install", async () => {
    stubFetch({ code: "pack_conflict", error: "installed already", previous: { install_id: "i-0", version: "1.0.0", squad_id: "sq-0" } }, 409);
    await expect(client().commitPack({ digest })).rejects.toMatchObject({ status: 409, body: { code: "pack_conflict", previous: { install_id: "i-0" } } });
  });
});

describe("proposal routes", () => {
  it("asks an agent to draft by creating a draft from a proposal", async () => {
    const fetchMock = stubFetch({ id: "d-1", revision: 1, origin: "proposal", source_ref: { issue_id: "iss-1", agent_id: "ag-1" } });
    const draft = await client().createFlowDraft("sq-1", { name: "Flow", from: { kind: "proposal", agent_id: "ag-1", references: ["att-1"], note: "Two checks" } });
    expect(call(fetchMock)).toMatchObject({
      method: "POST",
      url: "https://api.example.test/api/squads/sq-1/flow-drafts",
      body: { name: "Flow", from: { kind: "proposal", agent_id: "ag-1", references: ["att-1"], note: "Two checks" } },
    });
    expect(draft.origin).toBe("proposal");
  });

  it("imports the file of the proposal at the revision the caller holds", async () => {
    const fetchMock = stubFetch({ id: "d-1", revision: 2, origin: "proposal" });
    const draft = await client().importFlowProposal("sq-1", "d-1", { revision: 1 });
    expect(call(fetchMock)).toMatchObject({ method: "POST", url: "https://api.example.test/api/squads/sq-1/flow-drafts/d-1/import-proposal", body: { revision: 1 } });
    expect(draft.revision).toBe(2);
  });

  it("surfaces a missing file as an error with its code", async () => {
    stubFetch({ code: "flow_proposal_missing", error: "no file" }, 404);
    await expect(client().importFlowProposal("sq-1", "d-1", { revision: 1 })).rejects.toMatchObject({ status: 404, body: { code: "flow_proposal_missing" } });
  });
});

describe("flow insights route", () => {
  it("reads the numbers of a version, without trial runs by default", async () => {
    let fetchMock = stubFetch({ requests: { total: 2, open: 1, by_end: {} }, steps: [], edges: [], loops: [], stalls: [], manual_closes: 0, truncated: false });
    const report = await client().getFlowInsights("wf-1");
    expect(call(fetchMock)).toMatchObject({ method: "GET", url: "https://api.example.test/api/issue-workflows/wf-1/flow-insights?include_trial=false" });
    expect(report.requests.total).toBe(2);

    fetchMock = stubFetch({});
    await client().getFlowInsights("wf-1", { includeTrial: true });
    expect(call(fetchMock).url).toBe("https://api.example.test/api/issue-workflows/wf-1/flow-insights?include_trial=true");
  });

  it("reads a body it cannot read as a report with no requests", async () => {
    for (const body of ["nope", null, 7, [], { requests: "many", steps: 3 }]) {
      stubFetch(body);
      const report = await client().getFlowInsights("wf-1");
      expect(report.requests.total).toBe(0);
      expect(report.steps).toEqual([]);
    }
  });
});
