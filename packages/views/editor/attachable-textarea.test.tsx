import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const upload = vi.hoisted(() => vi.fn());
vi.mock("./use-editor-upload", () => ({ useEditorUpload: () => ({ uploadWithToast: upload }) }));

import { AttachableTextarea } from "./attachable-textarea";

function Harness({ initial = "" }: { initial?: string }) {
  const [value, setValue] = useState(initial);
  return (
    <AttachableTextarea aria-label="Instruction" issueId="issue-1" value={value} onValueChange={setValue} />
  );
}

const uploaded = { id: "a1", markdownLink: "https://cdn.example/a1.png" };

describe("AttachableTextarea", () => {
  afterEach(() => {
    cleanup();
    upload.mockReset();
  });

  it("uploads a pasted image against the issue and inserts its link", async () => {
    upload.mockResolvedValue(uploaded);
    render(<Harness initial="Check the screen" />);
    const file = new File(["x"], "shot.png", { type: "image/png" });

    fireEvent.paste(screen.getByLabelText("Instruction"), { clipboardData: { files: [file] } });

    await waitFor(() =>
      expect((screen.getByLabelText("Instruction") as HTMLTextAreaElement).value).toBe(
        "Check the screen\n![shot.png](https://cdn.example/a1.png)\n",
      ),
    );
    expect(upload).toHaveBeenCalledWith(file, { issueId: "issue-1" });
  });

  it("links a dropped non-image file as a plain link", async () => {
    upload.mockResolvedValue({ ...uploaded, markdownLink: "https://cdn.example/a1.pdf" });
    render(<Harness />);
    const file = new File(["x"], "spec.pdf", { type: "application/pdf" });

    fireEvent.drop(screen.getByLabelText("Instruction"), { dataTransfer: { files: [file], types: ["Files"] } });

    await waitFor(() =>
      expect((screen.getByLabelText("Instruction") as HTMLTextAreaElement).value).toBe(
        "[spec.pdf](https://cdn.example/a1.pdf)\n",
      ),
    );
  });

  it("leaves a text paste alone and keeps the text when the upload fails", async () => {
    upload.mockResolvedValue(null);
    render(<Harness initial="keep" />);
    const box = screen.getByLabelText("Instruction") as HTMLTextAreaElement;

    fireEvent.paste(box, { clipboardData: { files: [] } });
    expect(upload).not.toHaveBeenCalled();

    fireEvent.paste(box, { clipboardData: { files: [new File(["x"], "a.png", { type: "image/png" })] } });
    await waitFor(() => expect(upload).toHaveBeenCalled());
    expect(box.value).toBe("keep");
  });
});
