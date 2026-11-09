import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

vi.mock("../../editor/readonly-content", () => ({
  ReadonlyContent: ({ content }: { content: string }) => <div data-testid="markdown">{content}</div>,
}));

import { WakeupInstructionText } from "./wakeup-instruction-text";

describe("WakeupInstructionText", () => {
  afterEach(cleanup);

  it("keeps a plain instruction as plain text", () => {
    render(<WakeupInstructionText text="Check *the* build" className="x" />);
    expect(screen.getByText("Check *the* build").tagName).toBe("P");
    expect(screen.queryByTestId("markdown")).toBeNull();
  });

  it("renders an instruction with an attachment link as Markdown", () => {
    render(<WakeupInstructionText text="See ![a.png](https://cdn.example/a.png)" className="x" />);
    expect(screen.getByTestId("markdown")).toBeTruthy();
  });
});
