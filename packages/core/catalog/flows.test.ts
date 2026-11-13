// @vitest-environment node
import { describe, expect, it } from "vitest";
import { CATALOG_FLOWS } from "./generated/flows";
import { CATALOG_SQUADS } from "./generated/squads";
import { flowForSquad, flowTemplateRef } from "./flows";
import type { CatalogFlowCondition } from "./types";

// Every Kit Viet squad runs on a flow. The checks here are the ones a flow shares across squads;
// what is specific to one squad is checked by its rehearsal on the server (flow_rehearsal_*_test.go).
const KITVIET = CATALOG_SQUADS.filter((s) => s.adapter?.type === "kitviet");

function conditions(c: CatalogFlowCondition | null | undefined): CatalogFlowCondition[] {
  if (!c) return [];
  return [c, ...(c.all ?? []).flatMap(conditions), ...(c.any ?? []).flatMap(conditions), ...(c.not ? conditions(c.not) : [])];
}

describe("the flows of the Kit Viet squads", () => {
  it("gives every Kit Viet squad exactly one flow", () => {
    expect(KITVIET).toHaveLength(21);
    for (const squad of KITVIET) {
      const flows = Object.values(CATALOG_FLOWS).filter((f) => f.squad === squad.id);
      expect(flows, squad.id).toHaveLength(1);
      expect(flowForSquad(squad.id)?.id, squad.id).toBe(`flow_${squad.id}`);
    }
    expect(Object.keys(CATALOG_FLOWS)).toHaveLength(KITVIET.length);
  });

  it("names the revision in the template_ref", () => {
    for (const flow of Object.values(CATALOG_FLOWS)) expect(flowTemplateRef(flow)).toMatch(/^flow_kv_[a-z0-9]+@[0-9a-f]{8}$/);
  });

  it("gives the route switch exactly the routes of the squad's table (spec 12.2)", () => {
    const routes: Record<string, string[]> = {
      kv_trade: [], kv_b2b: [], kv_admin: [], kv_marketing: [], kv_talent: [], kv_education: [], kv_officeops: [], kv_health: [],
      kv_construction: [], kv_fnb: [], kv_banking: [],
      kv_pr: ["article_check", "release_qa"],
      kv_ecom: ["listing", "aftersale", "order_report"],
      kv_academic: ["chapter_draft", "chapter_review"],
      kv_legal: ["contract_review", "validity_review"],
      kv_accounting: ["month_close", "tax_letter", "tax_calendar"],
      kv_realestate: ["listing_pack", "loan_plan"],
      kv_teaching: ["teaching_material", "class_records"],
      kv_hospitality: ["tour_quote", "guest_text"],
      kv_insurance: ["proposal", "claim_guidance"],
      kv_factory: ["kpi_report", "quality_record"],
    };
    for (const squad of KITVIET) {
      const route = flowForSquad(squad.id)!.config.switches?.find((s) => s.key === "route");
      const want = routes[squad.id]!;
      // Trade keeps its single route as a switch too (spec 3.4); the others have none.
      expect(route?.values ?? [], squad.id).toEqual(squad.id === "kv_trade" ? ["shipment_dossier"] : want);
    }
  });

  it("uses every role of the squad except the leader, and never the leader", () => {
    for (const squad of KITVIET) {
      const flow = flowForSquad(squad.id)!;
      const used = new Set(flow.steps.flatMap((s) => (s.tasks ?? []).map((t) => t.role)));
      const wanted = squad.squad.roles.map((r) => r.key).filter((k) => k !== squad.squad.leader);
      expect([...used].sort(), squad.id).toEqual([...wanted].sort());
    }
  });

  it("asks a gate for an outcome and a draft for an attachment", () => {
    for (const flow of Object.values(CATALOG_FLOWS)) {
      for (const step of flow.steps) {
        for (const task of step.tasks ?? []) {
          if (task.kind === "gate") expect(task.requires, `${flow.id}/${task.key}`).toContain("outcome");
          if (step.statusKey === "draft") expect(task.requires, `${flow.id}/${task.key}`).toContain("attachment");
        }
      }
    }
  });

  it("limits rework to two rounds, always going back to an earlier step", () => {
    for (const flow of Object.values(CATALOG_FLOWS)) {
      const order = flow.steps.map((s) => s.statusKey);
      for (const [index, step] of flow.steps.entries()) {
        for (const tr of step.transitions ?? []) {
          if (!tr.rework) continue;
          const limit = conditions(tr.when).find((c) => c.field === "rework_count");
          expect(limit, `${flow.id}/${tr.id}`).toMatchObject({ op: "lt", value: 2 });
          expect(order.indexOf(tr.to), `${flow.id}/${tr.id}`).toBeLessThan(index);
        }
      }
    }
  });

  it("has a human-only in_review step as the end of every flow", () => {
    for (const flow of Object.values(CATALOG_FLOWS)) {
      const review = flow.steps.find((s) => s.statusKey === "in_review");
      expect(review?.humanOnly, flow.id).toBe(true);
      for (const task of review?.tasks ?? []) expect(task.key, flow.id).toBe("export");
    }
  });
});
