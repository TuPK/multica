// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import { trackRecordListOptions } from "./queries";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api")>()),
  api: { request },
}));

describe("track record queries", () => {
  it("keys the list by workspace and filters", () => {
    const key = trackRecordListOptions("ws-1", { agent_id: "a-1", window_days: 90 }).queryKey;
    expect(key.slice(0, 2)).toEqual(["durable", "ws-1"]);
    expect(key).not.toEqual(trackRecordListOptions("ws-2", { agent_id: "a-1", window_days: 90 }).queryKey);
    expect(key).not.toEqual(trackRecordListOptions("ws-1", { agent_id: "a-1", window_days: 30 }).queryKey);
  });

  it("requests the list with its filters and returns the records", async () => {
    request.mockResolvedValueOnce({
      track_records: [{ agent_id: "a-1", work_class: "general", window_days: 90, n_pass: 3 }],
    });
    const rows = await trackRecordListOptions("ws-1", { agent_id: "a-1", work_class: "general", window_days: 90 }).queryFn!({} as never);
    expect(request).toHaveBeenCalledWith("/api/track-records?agent_id=a-1&work_class=general&window_days=90");
    expect(rows).toEqual([expect.objectContaining({ agent_id: "a-1", n_pass: 3 })]);
  });

  it("does not run without a workspace", () => {
    expect(trackRecordListOptions("", {}).enabled).toBe(false);
  });
});
