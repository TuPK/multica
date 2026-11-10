// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import type { KindField } from "@multica/core/knowledge";
import type { KnowledgeDocument } from "@multica/core/knowledge";
import {
  SALES_SAVED_VIEWS,
  boardField,
  groupByValue,
  tableFields,
  todayISO,
} from "./knowledge-collections";

const stage: KindField = {
  key: "stage",
  label: "Stage",
  type: "select",
  options: ["new", "watch", "qualified"],
  required: false,
  ordered: true,
};
const fields: KindField[] = [
  { key: "owner", label: "Owner", type: "text", options: [], required: false, ordered: false },
  stage,
  { key: "url", label: "Link", type: "url", options: [], required: false, ordered: false },
  { key: "fit_score", label: "Score", type: "number", options: [], required: false, ordered: false },
  { key: "next", label: "Next", type: "date", options: [], required: false, ordered: false },
  { key: "dnc", label: "Do not contact", type: "bool", options: [], required: false, ordered: false },
];

const doc = (id: string, properties: Record<string, unknown>): KnowledgeDocument =>
  ({ id, title: `Doc ${id}`, properties }) as KnowledgeDocument;

describe("collection layout helpers", () => {
  it("groups a board by its select field, in the field's option order", () => {
    const columns = groupByValue(
      [doc("a", { stage: "qualified" }), doc("b", { stage: "new" }), doc("c", { stage: "unknown" }), doc("d", {})],
      stage,
    );
    expect(columns.map((c) => c.option)).toEqual(["new", "watch", "qualified"]);
    expect(columns.map((c) => c.docs.map((d) => d.id))).toEqual([["b"], [], ["a"]]);
  });

  it("picks the board field the view names, else the first ordered select", () => {
    expect(boardField(fields, "stage")?.key).toBe("stage");
    expect(boardField(fields)?.key).toBe("stage");
    expect(boardField(fields.filter((f) => f.key !== "stage"), "missing")).toBeUndefined();
  });

  it("shows the first few fields as columns and leaves out links", () => {
    const shown = tableFields(fields).map((f) => f.key);
    expect(shown).not.toContain("url");
    expect(shown.length).toBeLessThanOrEqual(5);
  });

  it("names the day in UTC as YYYY-MM-DD for the due filter", () => {
    expect(todayISO(new Date("2026-11-10T23:30:00Z"))).toBe("2026-11-10");
  });

  it("gives each saved view a kind of the sales base and a layout", () => {
    for (const view of SALES_SAVED_VIEWS) {
      expect(view.kind.startsWith("sales_"), view.id).toBe(true);
      expect(["board", "table"], view.id).toContain(view.layout);
    }
    expect(SALES_SAVED_VIEWS.find((v) => v.id === "pipeline")?.groupBy).toBe("stage");
  });
});
