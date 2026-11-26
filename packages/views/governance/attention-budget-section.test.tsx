/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { AttentionBudget } from "@multica/core/governance";
import { renderWithI18n } from "../test/i18n";
import { AttentionBudgetSection } from "./attention-budget-section";

const api = vi.hoisted(() => ({ getAttentionBudget: vi.fn(), setAttentionBudget: vi.fn() }));

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api,
}));

const ON: AttentionBudget = {
  enabled: true,
  minutes_per_week: 60,
  split: "equal",
  week_start: "2026-11-23",
  spent_minutes: 23,
};

function renderSection(editable = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={client}>
      <AttentionBudgetSection wsId="ws-1" editable={editable} />
    </QueryClientProvider>,
  );
}

describe("AttentionBudgetSection", () => {
  beforeEach(() => {
    api.getAttentionBudget.mockReset();
    api.setAttentionBudget.mockReset();
  });
  afterEach(cleanup);

  it("stays hidden while durable_principal is off", async () => {
    api.getAttentionBudget.mockResolvedValue({ ...ON, enabled: false });
    renderSection();

    await waitFor(() => expect(api.getAttentionBudget).toHaveBeenCalled());
    expect(screen.queryByText("Attention budget")).toBeNull();
  });

  it("shows the budget and what this week has used", async () => {
    api.getAttentionBudget.mockResolvedValue(ON);
    renderSection();

    expect(await screen.findByText("23 of 60 minutes used this week.")).toBeTruthy();
    expect((screen.getByLabelText("Minutes per week") as HTMLInputElement).value).toBe("60");
  });

  it("saves the minutes and split the server then keeps", async () => {
    api.getAttentionBudget.mockResolvedValue(ON);
    api.setAttentionBudget.mockResolvedValue({ ...ON, minutes_per_week: 90, split: "goals" });
    renderSection();

    const minutes = await screen.findByLabelText("Minutes per week");
    fireEvent.change(minutes, { target: { value: "90" } });
    fireEvent.click(screen.getByRole("button", { name: "Save budget" }));

    await waitFor(() =>
      expect(api.setAttentionBudget).toHaveBeenCalledWith({ minutes_per_week: 90, split: "equal" }),
    );
  });

  it("refuses a budget outside a week and saves nothing", async () => {
    api.getAttentionBudget.mockResolvedValue(ON);
    renderSection();

    fireEvent.change(await screen.findByLabelText("Minutes per week"), { target: { value: "99999" } });
    expect(screen.getByRole("alert").textContent).toContain("0 to 10080");
    expect((screen.getByRole("button", { name: "Save budget" }) as HTMLButtonElement).disabled).toBe(true);
    expect(api.setAttentionBudget).not.toHaveBeenCalled();
  });
});
