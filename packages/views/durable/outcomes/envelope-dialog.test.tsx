/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";
import { EnvelopeDialog } from "./envelope-dialog";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { request: vi.fn() },
}));

const request = vi.mocked(api.request);

async function pick(user: ReturnType<typeof userEvent.setup>, name: string, option: string) {
  await user.click(screen.getByRole("combobox", { name }));
  await user.click(await screen.findByRole("option", { name: option }));
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
}

function renderDialog(onOpenChange = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  renderWithI18n(
    <QueryClientProvider client={qc}>
      <EnvelopeDialog open onOpenChange={onOpenChange} wsId="ws-1" squadId="s-1" />
    </QueryClientProvider>,
  );
  return { onOpenChange };
}

beforeEach(() => {
  request.mockReset();
  vi.mocked(toast.success).mockReset();
  vi.mocked(toast.error).mockReset();
});
afterEach(cleanup);

describe("EnvelopeDialog", () => {
  it("blocks a floor above the ceiling and sends nothing", async () => {
    const user = userEvent.setup({ delay: null });
    renderDialog();

    await pick(user, "Floor", "L3 Autonomous");
    await pick(user, "Ceiling", "L1 Work");
    await user.type(screen.getByLabelText("Reason"), "Trial");

    expect(screen.getByText("The floor cannot be above the ceiling.")).toBeInTheDocument();
    const save = screen.getByRole("button", { name: "Save range" });
    expect(save).toBeDisabled();
    await user.click(save);
    expect(request).not.toHaveBeenCalled();
  });

  it("saves the range with its reason and closes once the server keeps it", async () => {
    const user = userEvent.setup({ delay: null });
    request.mockResolvedValueOnce({ envelope: { squad_id: "s-1", floor_level: 0, ceiling_level: 2 }, reputation: {} });
    const { onOpenChange } = renderDialog();

    await pick(user, "Floor", "L0 Suggest");
    await pick(user, "Ceiling", "L2 Trusted");
    await user.type(screen.getByLabelText("Reason"), "Trial");
    await user.click(screen.getByRole("button", { name: "Save range" }));

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "/api/squads/s-1/autonomy/envelope",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ floor_level: 0, ceiling_level: 2, reason: "Trial" }),
        }),
      ),
    );
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(toast.success).toHaveBeenCalled();
  });
});
