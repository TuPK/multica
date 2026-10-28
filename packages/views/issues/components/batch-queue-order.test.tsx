import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enIssues from "../../locales/en/issues.json";
import { BatchQueueOrder } from "./batch-queue-order";

const TEST_RESOURCES = { en: { common: enCommon, issues: enIssues } };
const batchMutate = vi.fn().mockResolvedValue({ updated: 2 });
const preview = vi.fn();

vi.mock("@multica/core/issues/mutations", () => ({
  useSetIssueQueueOrder: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useBatchSetIssueQueueOrder: () => ({ mutateAsync: batchMutate, isPending: false }),
}));
vi.mock("@multica/core/api", () => ({
  api: { previewQueueOrder: (...args: unknown[]) => preview(...args) },
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

function renderBatch(ids: string[], onApplied = vi.fn()) {
  render(
    <I18nProvider resources={TEST_RESOURCES} locale="en">
      <BatchQueueOrder issueIds={ids} onApplied={onApplied} />
    </I18nProvider>,
  );
  return onApplied;
}

describe("BatchQueueOrder", () => {
  beforeEach(() => {
    batchMutate.mockClear();
    preview.mockReset();
    preview.mockResolvedValue({ release_now: 0, run_count: 0 });
  });

  it("renders nothing without a backlog issue selected", () => {
    renderBatch([]);
    expect(screen.queryByRole("button", { name: "Run order" })).not.toBeInTheDocument();
  });

  it("numbers every selected backlog issue at once", async () => {
    const onApplied = renderBatch(["a", "b"]);
    await userEvent.click(screen.getByRole("button", { name: "Run order" }));
    const input = await screen.findByRole("textbox", { name: "Run order" });
    await userEvent.type(input, "x2");
    expect(input).toHaveValue("2");
    await userEvent.click(screen.getByRole("button", { name: "Apply" }));
    await waitFor(() => expect(batchMutate).toHaveBeenCalledWith({ ids: ["a", "b"], queueOrder: 2 }));
    expect(onApplied).toHaveBeenCalled();
  });

  it("clears the numbers without a preview", async () => {
    renderBatch(["a", "b"]);
    await userEvent.click(screen.getByRole("button", { name: "Run order" }));
    await userEvent.click(await screen.findByRole("button", { name: "Clear" }));
    await waitFor(() => expect(batchMutate).toHaveBeenCalledWith({ ids: ["a", "b"], queueOrder: null }));
    expect(preview).not.toHaveBeenCalled();
  });
});
