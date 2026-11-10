// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithI18n } from "../test/i18n";
import { HubFileTree } from "./hub-file-tree";

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 32,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({ index, key: index, start: index * 32 })),
    scrollToIndex: () => {},
  }),
}));

const PATHS = ["SKILL.md", "docs/guide.md", "docs/deep/notes.md"];

function renderTree(selectedPath = "SKILL.md") {
  const onSelect = vi.fn();
  renderWithI18n(
    <HubFileTree paths={PATHS} selectedPath={selectedPath} dirtyPaths={new Set()} onSelect={onSelect} />,
  );
  return { onSelect, tree: screen.getByRole("tree", { name: "Files" }) };
}

function row(name: string): HTMLElement {
  return screen.getByRole("treeitem", { name });
}

describe("HubFileTree", () => {
  it("exposes folders and files as tree items with their level and expanded state", () => {
    const { tree } = renderTree();

    expect(within(tree).getAllByRole("treeitem")).toHaveLength(5);
    expect(row("docs")).toHaveAttribute("aria-expanded", "true");
    expect(row("docs")).toHaveAttribute("aria-level", "1");
    expect(row("guide.md")).toHaveAttribute("aria-level", "2");
    expect(row("guide.md")).not.toHaveAttribute("aria-expanded");
    expect(row("SKILL.md")).toHaveAttribute("aria-selected", "true");
  });

  it("keeps one row in the tab order and moves it with the arrow keys", async () => {
    const user = userEvent.setup();
    renderTree();

    await user.tab();
    expect(row("SKILL.md")).toHaveFocus();

    await user.keyboard("{ArrowDown}");
    const next = screen.getAllByRole("treeitem")[1]!;
    expect(next).toHaveFocus();
    expect(next).toHaveAttribute("tabindex", "0");
    expect(row("SKILL.md")).toHaveAttribute("tabindex", "-1");
  });

  it("jumps to the first and last rows with Home and End", async () => {
    const user = userEvent.setup();
    renderTree();
    await user.tab();

    await user.keyboard("{End}");
    expect(row("guide.md")).toHaveFocus();

    await user.keyboard("{Home}");
    expect(row("SKILL.md")).toHaveFocus();
  });

  it("selects a file on Enter and does not move focus on other keys", async () => {
    const user = userEvent.setup();
    const { onSelect } = renderTree();
    await user.tab();
    await user.keyboard("{End}");
    expect(row("guide.md")).toHaveFocus();

    await user.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledWith("docs/guide.md");
  });

  it("collapses and expands a folder with Enter and the arrow keys", async () => {
    const user = userEvent.setup();
    renderTree();
    await user.tab();
    await user.keyboard("{ArrowDown}");
    expect(row("docs")).toHaveFocus();

    await user.keyboard("{Enter}");
    expect(row("docs")).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("treeitem", { name: "guide.md" })).toBeNull();

    await user.keyboard("{ArrowRight}");
    expect(row("docs")).toHaveAttribute("aria-expanded", "true");
    expect(row("docs")).toHaveFocus();
  });

  it("moves from a child to its parent folder with ArrowLeft", async () => {
    const user = userEvent.setup();
    renderTree();
    await user.tab();
    await user.keyboard("{End}");
    expect(row("guide.md")).toHaveFocus();

    await user.keyboard("{ArrowLeft}");
    expect(row("docs")).toHaveFocus();
  });

  it("moves into the first child with ArrowRight on an open folder", async () => {
    const user = userEvent.setup();
    renderTree();
    await user.tab();
    await user.keyboard("{ArrowDown}");
    expect(row("docs")).toHaveFocus();

    await user.keyboard("{ArrowRight}");
    expect(row("deep")).toHaveFocus();
  });

  it("keeps focus on the row a pointer activated", async () => {
    const user = userEvent.setup();
    const { onSelect } = renderTree();

    await user.click(row("guide.md"));
    expect(onSelect).toHaveBeenCalledWith("docs/guide.md");
    expect(row("guide.md")).toHaveFocus();
    expect(row("guide.md")).toHaveAttribute("tabindex", "0");
  });

  it("announces unsaved changes in the row name", () => {
    renderWithI18n(
      <HubFileTree
        paths={PATHS}
        selectedPath="SKILL.md"
        dirtyPaths={new Set(["SKILL.md"])}
        onSelect={vi.fn()}
      />,
    );
    expect(screen.getByRole("treeitem", { name: "SKILL.md, Unsaved changes" })).toBeInTheDocument();
  });
});
