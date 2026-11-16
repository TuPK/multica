// @vitest-environment jsdom

import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlowDoc } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { VarsPanel } from "./vars-panel";

const DOC = {
  version: 2,
  start: "a",
  roles: [
    { key: "writer", kind: "agent" },
    { key: "editor", kind: "agent" },
    { key: "signer", kind: "member" },
  ],
  vars: [{ key: "topics", type: "list_string", default: [], set_by: ["lead", "writer"], set_at: ["*"] }],
  steps: [],
  terminals: [],
} as unknown as FlowDoc;

afterEach(cleanup);

describe("VarsPanel Set by", () => {
  it("lets the person pick the declared roles that set the variable, and keeps the built-in ones", () => {
    const onChangeVars = vi.fn();
    renderWithI18n(<VarsPanel doc={DOC} onChangeVars={onChangeVars} />);
    const row = screen.getByText("topics").closest("tr") as HTMLElement;
    const writer = within(row).getByRole("button", { name: "writer" });
    const editor = within(row).getByRole("button", { name: "editor" });
    expect(writer.getAttribute("aria-pressed")).toBe("true");
    expect(editor.getAttribute("aria-pressed")).toBe("false");
    // The role of a person can set a variable too, as a declared role.
    expect(within(row).getByRole("button", { name: "signer" })).toBeTruthy();

    fireEvent.click(editor);
    expect(onChangeVars).toHaveBeenLastCalledWith([{ ...DOC.vars![0], set_by: ["lead", "writer", "editor"] }]);
    fireEvent.click(writer);
    expect(onChangeVars).toHaveBeenLastCalledWith([{ ...DOC.vars![0], set_by: ["lead"] }]);
  });
});
