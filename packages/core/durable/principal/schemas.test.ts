// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { setSchemaLogger } from "../../api/schema";
import { noopLogger, type Logger } from "../../logger";
import {
  isForbiddenDelegationKind,
  parseDelegationEnableResult,
  parsePrincipalInterview,
  parsePrincipalProfile,
  parsePrincipalTrait,
} from "./schemas";

const traitRow = {
  id: "tr-1",
  decision_kind: "general",
  statement: "Prefer thorough verification over speed for customer-facing work",
  origin: "interview",
  evidence: [{ type: "interview", ref: "00000000-0000-0000-0000-000000000001" }],
  created_at: "2026-11-26T09:00:00Z",
};

const profileRow = {
  member_id: "u-1",
  status: "active",
  consent_at: "2026-11-26T08:00:00Z",
  share_with_owners: false,
  version: 2,
  traits: [traitRow],
};

describe("principal schemas", () => {
  let warn: Mock<(msg: string, ...data: unknown[]) => void>;

  beforeEach(() => {
    warn = vi.fn<(msg: string, ...data: unknown[]) => void>();
    const logger: Logger = { debug: vi.fn(), info: vi.fn(), warn, error: vi.fn() };
    setSchemaLogger(logger);
  });

  afterEach(() => {
    setSchemaLogger(noopLogger);
  });

  it("parses a profile and its active traits as the server sends them", () => {
    const profile = parsePrincipalProfile(profileRow, "GET /api/principal/profile");
    expect(profile).toEqual(profileRow);
    expect(warn).not.toHaveBeenCalled();
  });

  it("degrades unknown status, origin and evidence type to safe defaults", () => {
    const profile = parsePrincipalProfile(
      {
        ...profileRow,
        status: "future_status",
        traits: [
          {
            ...traitRow,
            origin: "imported",
            evidence: [{ type: "unknown_source", ref: "ref-1" }],
          },
        ],
      },
      "GET /api/principal/profile",
    );
    expect(profile?.status).toBe("active");
    expect(profile?.traits[0]?.origin).toBe("learned");
    expect(profile?.traits[0]?.evidence[0]?.type).toBe("decision");
  });

  it("drops malformed traits while keeping valid ones", () => {
    const profile = parsePrincipalProfile(
      { ...profileRow, traits: [null, { id: "" }, traitRow] },
      "GET /api/principal/profile",
    );
    expect(profile?.traits.map((t) => t.id)).toEqual(["tr-1"]);
  });

  it("returns null and warns for a malformed profile payload", () => {
    expect(parsePrincipalProfile(null, "GET /api/principal/profile")).toBeNull();
    expect(parsePrincipalProfile({ status: "active" }, "GET /api/principal/profile")).toBeNull();
    expect(warn).toHaveBeenCalled();
  });

  it("parses a single trait and falls back to null when malformed", () => {
    expect(parsePrincipalTrait(traitRow, "PATCH /api/principal/profile/traits/:id")).toEqual(traitRow);
    expect(parsePrincipalTrait({ statement: "no id" }, "PATCH /api/principal/profile/traits/:id")).toBeNull();
  });

  it("parses interview questions and drops empty or non-string entries", () => {
    expect(
      parsePrincipalInterview({ questions: ["Q1?", "", 42, "Q2?"] }, "GET /api/principal/interview"),
    ).toEqual({ questions: ["Q1?", "Q2?"] });
    expect(parsePrincipalInterview(null, "GET /api/principal/interview")).toEqual({ questions: [] });
  });

  it("parses delegation enable responses and rejects non-enabled bodies", () => {
    expect(
      parseDelegationEnableResult(
        { decision_kind: "proposal:general", enabled: true },
        "PUT /api/principal/delegations",
      ),
    ).toEqual({ decision_kind: "proposal:general", enabled: true });
    expect(
      parseDelegationEnableResult(
        { decision_kind: "proposal:general", enabled: false },
        "PUT /api/principal/delegations",
      ),
    ).toBeNull();
  });
});

describe("isForbiddenDelegationKind", () => {
  it("locks every kind forbidden by durable spec 6.5", () => {
    const forbidden = [
      "proposal:org_change",
      "proposal:launch",
      "proposal:stage_gate",
      "effect:spend",
      "effect:share",
      "proposal:budget",
      "proposal:autonomy",
      "proposal:envelope",
      "proposal:policy",
      "proposal:profile",
      "PROPOSAL:ORG_CHANGE",
    ];
    for (const kind of forbidden) {
      expect(isForbiddenDelegationKind(kind), kind).toBe(true);
    }
  });

  it("allows delegatable kinds", () => {
    const allowed = [
      "general",
      "outcome",
      "inquiry",
      "proposal:general",
      "proposal:hire",
      "proposal:promote",
      "effect:write",
      "effect:send",
    ];
    for (const kind of allowed) {
      expect(isForbiddenDelegationKind(kind), kind).toBe(false);
    }
  });
});
