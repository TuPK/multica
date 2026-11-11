/**
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import type { CompanyFloor } from "@multica/core/company";
import { renderWithI18n } from "../../test/i18n";
import { FloorDeleteDialog } from "./company-floor-dialogs";

const mutations = vi.hoisted(() => ({ remove: vi.fn(), pending: { value: false } }));

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

vi.mock("@multica/core/company", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/company")>()),
  useDeleteCompanyFloor: () => ({ mutate: mutations.remove, isPending: mutations.pending.value }),
}));

const FLOOR: CompanyFloor = { id: "f2", name: "Build", position: 1, workspace_id: "ws-1", created_at: "", updated_at: "" };

function setup(seatedCount: number) {
  const onClose = vi.fn();
  const onDeleted = vi.fn();
  renderWithI18n(
    <FloorDeleteDialog floor={FLOOR} seatedCount={seatedCount} onClose={onClose} onDeleted={onDeleted} />,
  );
  return { onClose, onDeleted };
}

describe("FloorDeleteDialog", () => {
  beforeEach(() => {
    mutations.remove.mockReset();
    mutations.pending.value = false;
    vi.mocked(toast.error).mockReset();
  });
  afterEach(cleanup);

  it("says how many rooms go and that the seated squads are kept", () => {
    setup(2);

    const dialog = screen.getByRole("alertdialog");
    expect(within(dialog).getByText("Delete Build?")).toBeTruthy();
    expect(within(dialog).getByText(/Its 6 rooms will be removed\./)).toBeTruthy();
    expect(within(dialog).getByText(/2 squads will lose their rooms\. They are kept\./)).toBeTruthy();
  });

  it("uses the singular squad sentence for one seated squad", () => {
    setup(1);

    expect(screen.getByText(/1 squad will lose its room\. It is kept\./)).toBeTruthy();
  });

  it("leaves out the squad sentence when no squad is seated", () => {
    setup(0);

    expect(screen.queryByText(/will lose/)).toBeNull();
    expect(screen.getByText(/Its 6 rooms will be removed\./)).toBeTruthy();
  });

  it("deletes on confirm and reports the floor only after the server confirms", async () => {
    mutations.remove.mockImplementation((_id: string, options: { onSuccess: () => void }) => options.onSuccess());
    const { onDeleted } = setup(0);

    await userEvent.click(screen.getByRole("button", { name: "Delete floor" }));

    expect(mutations.remove).toHaveBeenCalledWith("f2", expect.any(Object));
    expect(onDeleted).toHaveBeenCalledWith(FLOOR);
  });

  it("closes without deleting on cancel", async () => {
    const { onClose } = setup(0);

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onClose).toHaveBeenCalled();
    expect(mutations.remove).not.toHaveBeenCalled();
  });

  it("stays open and reports a failed delete", async () => {
    mutations.remove.mockImplementation((_id: string, options: { onError: (error: unknown) => void }) =>
      options.onError(new Error("network")),
    );
    const { onDeleted } = setup(1);

    await userEvent.click(screen.getByRole("button", { name: "Delete floor" }));

    expect(toast.error).toHaveBeenCalledWith("Couldn't delete the floor");
    expect(onDeleted).not.toHaveBeenCalled();
    expect(screen.getByRole("alertdialog")).toBeTruthy();
  });

  it("disables both buttons while the delete is pending", () => {
    mutations.pending.value = true;
    setup(0);

    expect((screen.getByRole("button", { name: "Delete floor" }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Cancel" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
