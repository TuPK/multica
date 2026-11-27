// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { setSchemaLogger } from "../../api/schema";
import { noopLogger, type Logger } from "../../logger";
import {
  DEFAULT_DELEGATION_MIN_CONFIDENCE,
  EMPTY_PRINCIPAL_DELEGATIONS,
  EMPTY_PRINCIPAL_EVIDENCE_CANDIDATES,
  isForbiddenDelegationKind,
  parseDelegationEnableResult,
  parsePrincipalDelegations,
  parsePrincipalEvidenceCandidates,
  parsePrincipalInterview,
  parsePrincipalPrediction,
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

  it("parses delegations and per-kind accuracy, defaulting missing fields and dropping bad rows", () => {
    const valid = {
      delegations: [
        {
          decision_kind: "proposal:general",
          enabled: true,
          min_confidence: 0.92,
          enabled_at: "2026-11-26T08:00:00Z",
          disabled_reason: "",
        },
      ],
      kinds: [
        {
          decision_kind: "proposal:general",
          matched: 22,
          n: 24,
          lower: 0.74,
          eligible: true,
        },
      ],
    };
    expect(parsePrincipalDelegations(valid, "GET /api/principal/delegations")).toEqual(valid);
    expect(warn).not.toHaveBeenCalled();

    const partial = parsePrincipalDelegations(
      {
        delegations: [null, { decision_kind: "" }, { decision_kind: "general", min_confidence: 99 }],
        kinds: [42, { decision_kind: "general", matched: -1, n: "bad", lower: 2, eligible: "yes" }],
      },
      "GET /api/principal/delegations",
    );
    expect(partial).toEqual({
      delegations: [
        {
          decision_kind: "general",
          enabled: false,
          min_confidence: DEFAULT_DELEGATION_MIN_CONFIDENCE,
          enabled_at: "",
          disabled_reason: "",
        },
      ],
      kinds: [
        {
          decision_kind: "general",
          matched: 0,
          n: 0,
          lower: 0,
          eligible: false,
        },
      ],
    });

    expect(parsePrincipalDelegations(null, "GET /api/principal/delegations")).toEqual(
      EMPTY_PRINCIPAL_DELEGATIONS,
    );
    expect(warn).toHaveBeenCalled();
  });

  it("parses predictions with nullable actual/matched and returns null on malformed payloads", () => {
    const pending = {
      predicted: "approve",
      confidence: 0.92,
      why: "Matches prior reversible rollout approvals.",
      actual: null,
      matched: null,
    };
    expect(parsePrincipalPrediction(pending, "GET /api/principal/predictions")).toEqual(pending);
    expect(warn).not.toHaveBeenCalled();

    expect(
      parsePrincipalPrediction({ predicted: "reject", confidence: 5 }, "GET /api/principal/predictions"),
    ).toEqual({
      predicted: "reject",
      confidence: 0,
      why: "",
      actual: null,
      matched: null,
    });

    expect(parsePrincipalPrediction({ predicted: "" }, "GET /api/principal/predictions")).toBeNull();
    expect(parsePrincipalPrediction(null, "GET /api/principal/predictions")).toBeNull();
    expect(warn).toHaveBeenCalled();
  });

  it("parses evidence candidates, drops rows without ids, and falls back to empty", () => {
    const payload = {
      decisions: [
        {
          id: "00000000-0000-0000-0000-000000000010",
          title: "Launch pricing pilot",
          decision: "approved",
          decided_at: "2026-11-25T10:00:00Z",
        },
      ],
    };
    expect(parsePrincipalEvidenceCandidates(payload, "GET /api/principal/evidence-candidates")).toEqual(payload);
    expect(warn).not.toHaveBeenCalled();

    expect(
      parsePrincipalEvidenceCandidates(
        { decisions: [null, { id: "" }, { id: "d-1" }] },
        "GET /api/principal/evidence-candidates",
      ),
    ).toEqual({
      decisions: [{ id: "d-1", title: "", decision: "", decided_at: "" }],
    });

    expect(parsePrincipalEvidenceCandidates("bad", "GET /api/principal/evidence-candidates")).toEqual(
      EMPTY_PRINCIPAL_EVIDENCE_CANDIDATES,
    );
    expect(warn).toHaveBeenCalled();
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
