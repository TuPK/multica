import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "@multica/core/api";
import type { PolicyCell } from "@multica/core/durable/effects";
import { renderWithI18n } from "../../test/i18n";
import { EffectPolicyGrid } from "./effect-policy-grid";

const { role } = vi.hoisted(() => ({ role: { value: "owner" as "owner" | "member" } }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/permissions", () => ({ useCurrentMember: () => ({ role: role.value }) }));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));

const DECISIONS = ["allow", "confirm", "rehearse", "block"] as const;

/**
 * A cell as the server answers it: a decision looser than the default is locked below L3,
 * and allow is never offered for spend. The grid must show these locks, not compute its own.
 */
function cell(
  level: number,
  effect: PolicyCell["effect_class"],
  def: PolicyCell["default"],
  override: PolicyCell["override"] = null,
  rev: PolicyCell["reversibility"] = "irreversible",
): PolicyCell {
  return {
    autonomy_level: level,
    effect_class: effect,
    reversibility: rev,
    default: def,
    override,
    options: DECISIONS.map((decision) => {
      const looser = DECISIONS.indexOf(decision) < DECISIONS.indexOf(def);
      const neverAllow = decision === "allow" && (effect === "spend" || effect === "share");
      const locked = neverAllow || (looser && level !== 3);
      return { decision, allowed: !locked, reason: neverAllow ? "never_allow" : locked ? "loosen_needs_l3" : "" };
    }),
  };
}

const sendRow: PolicyCell[] = [
  cell(0, "send", "block"),
  cell(1, "send", "rehearse"),
  cell(2, "send", "rehearse"),
  cell(3, "send", "allow"),
];
const spendRow: PolicyCell[] = [
  cell(0, "spend", "block"),
  cell(1, "spend", "rehearse"),
  cell(2, "spend", "rehearse"),
  cell(3, "spend", "rehearse"),
];

const request = vi.mocked(api.request);

function renderGrid(cells: PolicyCell[] = [...sendRow, ...spendRow]) {
  request.mockResolvedValueOnce({ cells });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <EffectPolicyGrid />
    </QueryClientProvider>,
  );
}

/** The table cell that holds the combobox with this name. */
function cellOf(label: string): HTMLElement {
  return screen.getByRole("combobox", { name: label }).closest("td") as HTMLElement;
}

async function openCell(user: ReturnType<typeof userEvent.setup>, label: string) {
  await user.click(await screen.findByRole("combobox", { name: label }));
}

beforeEach(() => {
  role.value = "owner";
  vi.clearAllMocks();
});
afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("EffectPolicyGrid", { timeout: 30_000 }, () => {
  it("reads the grid from the server, not from a copy of the policy", async () => {
    renderGrid();
    await screen.findByRole("combobox", { name: "Send, L2" });
    expect(request).toHaveBeenCalledWith("/api/effects/policy");
  });

  it("offers only the decisions the server allows, and says why the rest are locked", async () => {
    const user = userEvent.setup();
    renderGrid();
    await openCell(user, "Send, L1");

    expect(await screen.findByRole("option", { name: "Allowed" })).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("option", { name: "Confirm" })).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("option", { name: "Held for approval" })).not.toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("option", { name: "Blocked" })).not.toHaveAttribute("aria-disabled", "true");
    expect(within(cellOf("Send, L1")).getByText("A looser decision needs L3.")).toBeInTheDocument();
  });

  it("never offers allow for spend, at any level", async () => {
    const user = userEvent.setup();
    renderGrid();
    await openCell(user, "Spend, L3");

    expect(await screen.findByRole("option", { name: "Allowed" })).toHaveAttribute("aria-disabled", "true");
    expect(within(cellOf("Spend, L3")).getByText("Spend and share are never allowed.")).toBeInTheDocument();
  });

  it("saves a decision the grid allows, for the cell it was chosen in", async () => {
    const user = userEvent.setup();
    request.mockResolvedValueOnce({ cells: [...sendRow, ...spendRow] });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    renderWithI18n(
      <QueryClientProvider client={qc}>
        <EffectPolicyGrid />
      </QueryClientProvider>,
    );
    await openCell(user, "Send, L2");
    request.mockResolvedValueOnce(cell(2, "send", "rehearse", "block"));
    await user.click(await screen.findByRole("option", { name: "Blocked" }));

    await waitFor(() => expect(request).toHaveBeenCalledWith("/api/effects/policy", expect.objectContaining({ method: "PUT" })));
    const put = request.mock.calls.find((call) => (call[1] as RequestInit | undefined)?.method === "PUT");
    expect(JSON.parse(String((put![1] as RequestInit).body))).toEqual({
      autonomy_level: 2,
      effect_class: "send",
      reversibility: "irreversible",
      decision: "block",
    });
  });

  it("shows a saved decision as changed", async () => {
    renderGrid([cell(1, "send", "rehearse", "block"), sendRow[0]!, sendRow[2]!, sendRow[3]!, ...spendRow]);
    await screen.findByRole("combobox", { name: "Send, L1" });
    expect(within(cellOf("Send, L1")).getByText("Changed")).toBeInTheDocument();
  });

  it("is read-only for a member who cannot govern", async () => {
    role.value = "member";
    renderGrid();
    expect(await screen.findByRole("combobox", { name: "Send, L2" })).toBeDisabled();
    expect(screen.getByText("Only owners and admins can change the policy.")).toBeInTheDocument();
  });

  it("shows only the cells the server sent", async () => {
    renderGrid(sendRow);
    await screen.findByRole("combobox", { name: "Send, L0" });
    expect(screen.queryByRole("combobox", { name: "Spend, L0" })).not.toBeInTheDocument();
  });
});
