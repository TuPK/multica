// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import type { FlowDoc } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { SimulateDialog } from "./simulate-dialog";

const fakeApi = { simulateFlowDraft: vi.fn() };
const doc = { version: 2, start: "plan", steps: [], terminals: [] } as unknown as FlowDoc;

function renderDialog(onOpenChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  renderWithI18n(
    <QueryClientProvider client={client}>
      <SimulateDialog open onOpenChange={onOpenChange} squadId="sq-1" draftId="d-1" doc={doc} />
    </QueryClientProvider>,
  );
  return onOpenChange;
}

describe("SimulateDialog", () => {
  beforeEach(() => setApiInstance(fakeApi as unknown as ApiClient));
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("has one control that closes it: the X in the corner, not a second Close in the footer", () => {
    const onOpenChange = renderDialog();
    const closers = screen.getAllByRole("button", { name: "Close" });
    expect(closers).toHaveLength(1);
    expect(closers[0]).toHaveAttribute("data-slot", "dialog-close");

    fireEvent.click(closers[0]!);
    expect(onOpenChange).toHaveBeenCalledWith(false, expect.anything());
  });
});
