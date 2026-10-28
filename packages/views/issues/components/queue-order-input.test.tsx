import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enIssues from "../../locales/en/issues.json";
import { queueWaitingOn } from "@multica/core/issues/queries";
import { parseQueueOrder, QueueOrderInput, sanitizeQueueOrder } from "./queue-order-input";

const TEST_RESOURCES = { en: { common: enCommon, issues: enIssues } };
const mutate = vi.fn().mockResolvedValue({});
const batchMutate = vi.fn().mockResolvedValue({ updated: 0 });
const preview = vi.fn();

vi.mock("@multica/core/issues/mutations", () => ({
  useSetIssueQueueOrder: () => ({ mutateAsync: mutate, isPending: false }),
  useBatchSetIssueQueueOrder: () => ({ mutateAsync: batchMutate, isPending: false }),
}));

vi.mock("@multica/core/api", () => ({
  api: { previewQueueOrder: (...args: unknown[]) => preview(...args) },
}));

vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

function renderInput(queueOrder: number | null) {
  return render(
    <I18nProvider resources={TEST_RESOURCES} locale="en">
      <QueueOrderInput issue={{ id: "issue-1", queue_order: queueOrder }} />
    </I18nProvider>,
  );
}

describe("queue order parsing", () => {
  it("keeps digits only", () => {
    expect(sanitizeQueueOrder("a1b-2.3 e")).toBe("123");
    expect(sanitizeQueueOrder("12345678901")).toBe("1234567");
  });

  it("maps empty to clearing and zero to invalid", () => {
    expect(parseQueueOrder("")).toBeNull();
    expect(parseQueueOrder("0")).toBeUndefined();
    expect(parseQueueOrder("007")).toBe(7);
  });
});

describe("QueueOrderInput", () => {
  beforeEach(() => {
    mutate.mockClear();
    batchMutate.mockClear();
    preview.mockReset();
    preview.mockResolvedValue({ release_now: 0, run_count: 0 });
  });

  it("rejects letters as they are typed", async () => {
    renderInput(null);
    const input = screen.getByRole("textbox", { name: "Run order" });
    await userEvent.type(input, "ab4c2");
    expect(input).toHaveValue("42");
  });

  it("saves the number when the field loses focus", async () => {
    renderInput(null);
    const input = screen.getByRole("textbox", { name: "Run order" });
    await userEvent.type(input, "3");
    fireEvent.blur(input);
    await waitFor(() => expect(mutate).toHaveBeenCalledWith({ id: "issue-1", queueOrder: 3 }));
  });

  it("clears the order when the field is emptied", async () => {
    renderInput(2);
    const input = screen.getByRole("textbox", { name: "Run order" });
    await userEvent.clear(input);
    fireEvent.blur(input);
    await waitFor(() => expect(mutate).toHaveBeenCalledWith({ id: "issue-1", queueOrder: null }));
  });

  it("does not save an unchanged or zero value", async () => {
    renderInput(2);
    const input = screen.getByRole("textbox", { name: "Run order" });
    fireEvent.blur(input);
    await userEvent.clear(input);
    await userEvent.type(input, "0");
    fireEvent.blur(input);
    expect(mutate).not.toHaveBeenCalled();
    expect(input).toHaveValue("2");
  });

  it("asks before a number that starts agent runs, and applies on confirm", async () => {
    preview.mockResolvedValue({ release_now: 3, run_count: 2 });
    renderInput(null);
    const input = screen.getByRole("textbox", { name: "Run order" });
    await userEvent.type(input, "1");
    fireEvent.blur(input);
    expect(await screen.findByText("Start queued tasks now?")).toBeInTheDocument();
    expect(screen.getByText("3 will move to Todo now and 2 will start an agent run.")).toBeInTheDocument();
    expect(mutate).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Start" }));
    await waitFor(() => expect(mutate).toHaveBeenCalledWith({ id: "issue-1", queueOrder: 1 }));
  });

  it("puts the stored number back when the confirmation is declined", async () => {
    preview.mockResolvedValue({ release_now: 1, run_count: 1 });
    renderInput(null);
    const input = screen.getByRole("textbox", { name: "Run order" });
    await userEvent.type(input, "1");
    fireEvent.blur(input);
    await userEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(input).toHaveValue(""));
    expect(mutate).not.toHaveBeenCalled();
  });
});

describe("queueWaitingOn", () => {
  const frontiers = [
    { project_id: null, frontier: 2 },
    { project_id: "p1", frontier: 5 },
  ];
  it("names the lowest unfinished number below the issue's own", () => {
    expect(queueWaitingOn({ status: "backlog", queue_order: 4, project_id: null }, frontiers)).toBe(2);
    expect(queueWaitingOn({ status: "backlog", queue_order: 6, project_id: "p1" }, frontiers)).toBe(5);
  });
  it("is null when the issue is next, not queued, or not in backlog", () => {
    expect(queueWaitingOn({ status: "backlog", queue_order: 2, project_id: null }, frontiers)).toBeNull();
    expect(queueWaitingOn({ status: "backlog", queue_order: null, project_id: null }, frontiers)).toBeNull();
    expect(queueWaitingOn({ status: "todo", queue_order: 9, project_id: null }, frontiers)).toBeNull();
    expect(queueWaitingOn({ status: "backlog", queue_order: 9, project_id: "other" }, frontiers)).toBeNull();
    expect(queueWaitingOn({ status: "backlog", queue_order: 9, project_id: null }, undefined)).toBeNull();
  });
});
