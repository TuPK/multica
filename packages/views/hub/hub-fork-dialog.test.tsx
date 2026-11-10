// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../test/i18n";

const mocks = vi.hoisted(() => ({
  fork: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: { success: mocks.toastSuccess, error: mocks.toastError },
}));

vi.mock("@multica/core/hub", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/hub")>();
  return {
    ...actual,
    hubItemMetaListOptions: () => ({ queryKey: ["hub-item-meta"] }),
    useForkHubItem: () => ({ mutateAsync: mocks.fork, isPending: false }),
  };
});

import { HubForkDialog } from "./hub-fork-dialog";

function renderFork(onClose = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  renderWithI18n(
    <QueryClientProvider client={client}>
      <HubForkDialog
        wsId="ws-1"
        kind="plugin"
        refId="p1"
        name="Sales kit"
        sourceName="GitHub"
        open
        onClose={onClose}
      />
    </QueryClientProvider>,
  );
  return { onClose };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.fork.mockResolvedValue({ kind: "plugin", id: "p1", tracking: "forked" });
});

describe("HubForkDialog", () => {
  it("states what forking stops, with the source named", () => {
    renderFork();
    const dialog = screen.getByRole("alertdialog");
    expect(dialog).toHaveTextContent("Fork Sales kit?");
    expect(dialog).toHaveTextContent("It stops receiving updates from GitHub.");
  });

  it("forks after confirmation, then reports and closes", async () => {
    const { onClose } = renderFork();
    await userEvent.click(screen.getByRole("button", { name: "Fork" }));
    await waitFor(() => expect(mocks.fork).toHaveBeenCalledWith({ wsId: "ws-1", kind: "plugin", refId: "p1" }));
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Forked Sales kit");
    expect(onClose).toHaveBeenCalled();
  });

  it("stays open and shows the server message when forking fails", async () => {
    mocks.fork.mockRejectedValueOnce(new Error("Only admins can fork this plugin."));
    const { onClose } = renderFork();
    await userEvent.click(screen.getByRole("button", { name: "Fork" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Only admins can fork this plugin.");
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  });

  it("cancels without calling the server", async () => {
    const { onClose } = renderFork();
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(mocks.fork).not.toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
  });
});
