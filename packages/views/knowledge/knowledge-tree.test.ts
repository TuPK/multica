// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { KnowledgeBase, KnowledgeDocument } from "@multica/core/knowledge";
import { buildKnowledgeTree, inFolder, isProjectBase } from "./knowledge-tree";

const base = (key: string, extra: Partial<KnowledgeBase> = {}): KnowledgeBase => ({
  id: key,
  key,
  name: key,
  visibility: "workspace",
  position: 0,
  ...extra,
});

const doc = (kb: string, folder: string, state: KnowledgeDocument["state"] = "published"): KnowledgeDocument => ({
  id: `${kb}-${folder}-${Math.random()}`,
  document_id: "d",
  knowledge_base: kb,
  title: "t",
  state,
  kind: "manual",
  url: "",
  source_type: "sync",
  source_id: null,
  created_by_type: "system",
  created_by_id: null,
  created_at: "",
  published_at: null,
  error: "",
  folder,
});

describe("buildKnowledgeTree", () => {
  const bases = [
    base("company", { position: 0 }),
    base("project-abc", { position: 100, project_id: "p1", name: "Mua Ngon" }),
    base("project-def", { position: 101, project_id: "p2", name: "Alpha" }),
    base("market", { position: 1 }),
  ];

  it("separates company bases from project bases and orders both", () => {
    const tree = buildKnowledgeTree(bases, []);
    expect(tree.company.map((n) => n.base.key)).toEqual(["company", "market"]);
    expect(tree.projects.map((n) => n.base.name)).toEqual(["Alpha", "Mua Ngon"]);
  });

  it("keeps an empty project base listed", () => {
    const tree = buildKnowledgeTree(bases, []);
    expect(tree.projects).toHaveLength(2);
    expect(tree.projects[0]!.count).toBe(0);
  });

  it("nests folders by path and counts every level", () => {
    const docs = [
      doc("project-abc", "docs/planning"),
      doc("project-abc", "docs/planning"),
      doc("project-abc", "docs/engineering"),
      doc("project-abc", ""),
    ];
    const node = buildKnowledgeTree(bases, docs).projects.find((n) => n.base.key === "project-abc")!;
    expect(node.count).toBe(4);
    expect(node.folders).toHaveLength(1);
    const docsFolder = node.folders[0]!;
    expect(docsFolder).toMatchObject({ path: "docs", count: 3 });
    expect(docsFolder.children.map((c) => [c.path, c.count])).toEqual([
      ["docs/engineering", 1],
      ["docs/planning", 2],
    ]);
  });

  it("counts drafts apart", () => {
    const node = buildKnowledgeTree(bases, [doc("company", "", "draft"), doc("company", "")]).company[0]!;
    expect(node.drafts).toBe(1);
    expect(node.count).toBe(2);
  });
});

describe("folders", () => {
  it("matches a folder and everything below it", () => {
    expect(inFolder(doc("k", "docs/a"), "docs")).toBe(true);
    expect(inFolder(doc("k", "docs"), "docs")).toBe(true);
    expect(inFolder(doc("k", "docsx"), "docs")).toBe(false);
    expect(inFolder(doc("k", ""), "")).toBe(true);
  });

  it("knows a project base", () => {
    expect(isProjectBase(base("a", { project_id: "p" }))).toBe(true);
    expect(isProjectBase(base("a"))).toBe(false);
  });
});
