import { describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";

const upload = vi.hoisted(() => vi.fn());
vi.mock("./use-editor-upload", () => ({ useEditorUpload: () => ({ uploadWithToast: upload }) }));

import { useLocalEditorUploads } from "./use-local-editor-uploads";

const result = (id: string) => ({
  id,
  url: `https://cdn.example/${id}.png`,
  link: `https://cdn.example/${id}.png`,
  markdownLink: `https://cdn.example/${id}.png`,
});

describe("useLocalEditorUploads", () => {
  it("binds only the uploads the body still references", async () => {
    upload.mockResolvedValueOnce(result("a")).mockResolvedValueOnce(result("b"));
    const { result: hook } = renderHook(() => useLocalEditorUploads({ current: null }));

    await act(async () => {
      await hook.current.onUploadFile(new File(["x"], "a.png"));
      await hook.current.onUploadFile(new File(["y"], "b.png"));
    });

    expect(hook.current.attachmentIdsIn("![](https://cdn.example/b.png)")).toEqual(["b"]);

    act(() => hook.current.reset());
    expect(hook.current.attachmentIdsIn("![](https://cdn.example/b.png)")).toEqual([]);
  });

  it("ignores a failed upload", async () => {
    upload.mockResolvedValueOnce(null);
    const { result: hook } = renderHook(() => useLocalEditorUploads({ current: null }));
    await act(async () => {
      await hook.current.onUploadFile(new File(["x"], "a.png"));
    });
    expect(hook.current.attachments).toEqual([]);
  });
});
