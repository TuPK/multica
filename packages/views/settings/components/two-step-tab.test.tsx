import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiError } from "@multica/core/api";
import { renderWithI18n } from "../../test/i18n";

const mockApi = vi.hoisted(() => ({
  enrollMFA: vi.fn(),
  confirmMFA: vi.fn(),
  disableMFA: vi.fn(),
}));

const mockCopyText = vi.hoisted(() => vi.fn().mockResolvedValue(true));

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: mockApi,
}));

vi.mock("@multica/ui/lib/clipboard", () => ({
  copyText: mockCopyText,
}));

vi.mock("react-qr-code", () => ({
  QRCode: ({ value }: { value: string }) => <div data-testid="qr" data-value={value} />,
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

import { TwoStepTab } from "./two-step-tab";

function renderTwoStepTab() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <TwoStepTab />
    </QueryClientProvider>,
  );
}

const TEN_CODES = [
  "aaaa-1111",
  "bbbb-2222",
  "cccc-3333",
  "dddd-4444",
  "eeee-5555",
  "ffff-6666",
  "gggg-7777",
  "hhhh-8888",
  "iiii-9999",
  "jjjj-0000",
];

describe("TwoStepTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.clear();
    mockApi.enrollMFA.mockResolvedValue({
      secret: "JBSWY3DPEHPK3PXP",
      otpauth_uri: "otpauth://totp/Multica:ada@acme.com?secret=JBSWY3DPEHPK3PXP",
    });
    mockApi.confirmMFA.mockResolvedValue({
      recovery_codes: TEN_CODES,
    });
    mockApi.disableMFA.mockResolvedValue(undefined);
  });

  it("starts from idle with no status request and does not touch localStorage", () => {
    renderTwoStepTab();

    expect(screen.getByRole("button", { name: "Set up two-step sign-in" })).toBeInTheDocument();
    expect(mockApi.enrollMFA).not.toHaveBeenCalled();
    expect(mockApi.confirmMFA).not.toHaveBeenCalled();
    expect(mockApi.disableMFA).not.toHaveBeenCalled();
    expect(window.localStorage.length).toBe(0);
  });

  it("enrolls, shows secret + otpauth URI, confirms with 6-digit code, and gates leaving recovery codes", async () => {
    const user = userEvent.setup();
    renderTwoStepTab();

    await user.click(screen.getByRole("button", { name: "Set up two-step sign-in" }));

    await waitFor(() => {
      expect(screen.getByText("JBSWY3DPEHPK3PXP")).toBeInTheDocument();
    });
    expect(
      screen.getByText("otpauth://totp/Multica:ada@acme.com?secret=JBSWY3DPEHPK3PXP"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("qr")).toHaveAttribute(
      "data-value",
      "otpauth://totp/Multica:ada@acme.com?secret=JBSWY3DPEHPK3PXP",
    );

    const codeInput = screen.getByLabelText(/6-digit verification code/i);
    await user.type(codeInput, "123456");
    await user.click(screen.getByRole("button", { name: "Confirm and enable" }));

    await waitFor(() => {
      expect(mockApi.confirmMFA).toHaveBeenCalledWith("123456");
    });

    for (const code of TEN_CODES) {
      expect(screen.getByText(code)).toBeInTheDocument();
    }

    await user.click(screen.getByRole("button", { name: "Copy codes" }));
    expect(mockCopyText).toHaveBeenCalledWith(TEN_CODES.join("\n"));

    const doneButton = screen.getByRole("button", { name: "Done" });
    expect(doneButton).toBeDisabled();

    await user.click(screen.getByRole("checkbox", { name: "I saved them" }));
    expect(doneButton).toBeEnabled();

    await user.click(doneButton);
    expect(screen.queryByText("aaaa-1111")).toBeNull();
    expect(screen.getByText(/Two-step sign-in is on/i)).toBeInTheDocument();
    expect(window.localStorage.length).toBe(0);
  });

  it("maps mfa_already_enabled (409) to the already on state with disable option", async () => {
    mockApi.enrollMFA.mockRejectedValue(
      new ApiError("already on", 409, "Conflict", { code: "mfa_already_enabled" }),
    );
    const user = userEvent.setup();
    renderTwoStepTab();

    await user.click(screen.getByRole("button", { name: "Set up two-step sign-in" }));

    await waitFor(() => {
      expect(screen.getByText(/Two-step sign-in is on/i)).toBeInTheDocument();
    });

    await user.type(screen.getByLabelText(/Authenticator or recovery code/i), "abcd-efgh");
    await user.click(screen.getByRole("button", { name: "Turn off" }));

    await waitFor(() => {
      expect(mockApi.disableMFA).toHaveBeenCalledWith("abcd-efgh");
    });
    expect(screen.getByRole("button", { name: "Set up two-step sign-in" })).toBeInTheDocument();
  });

  it("maps mfa_not_enrolling (409) during confirm to start again", async () => {
    mockApi.confirmMFA.mockRejectedValue(
      new ApiError("expired", 409, "Conflict", { code: "mfa_not_enrolling" }),
    );
    const user = userEvent.setup();
    renderTwoStepTab();

    await user.click(screen.getByRole("button", { name: "Set up two-step sign-in" }));
    await user.type(await screen.findByLabelText(/6-digit verification code/i), "123456");
    await user.click(screen.getByRole("button", { name: "Confirm and enable" }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(/Start again/i);
    });
    expect(screen.getByRole("button", { name: "Set up two-step sign-in" })).toBeInTheDocument();
  });

  it.each([
    ["invalid_code", 400, /invalid or expired/i],
    ["mfa_unavailable", 503, /not available on this server/i],
    ["viewer_read_only", 403, /human accounts/i],
    ["unknown_code", 500, /Could not update two-step sign-in/i],
  ])("maps %s error during confirm", async (code, status, pattern) => {
    mockApi.confirmMFA.mockRejectedValue(new ApiError("err", status, "Error", { code }));
    const user = userEvent.setup();
    renderTwoStepTab();

    await user.click(screen.getByRole("button", { name: "Set up two-step sign-in" }));
    await user.type(await screen.findByLabelText(/6-digit verification code/i), "654321");
    await user.click(screen.getByRole("button", { name: "Confirm and enable" }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(pattern);
    });
  });

  it("maps mfa_not_enabled (409) when turning off from idle", async () => {
    mockApi.disableMFA.mockRejectedValue(
      new ApiError("not enabled", 409, "Conflict", { code: "mfa_not_enabled" }),
    );
    const user = userEvent.setup();
    renderTwoStepTab();

    await user.type(screen.getByLabelText(/Authenticator or recovery code/i), "123456");
    await user.click(screen.getByRole("button", { name: "Turn off" }));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(/not turned on/i);
    });
  });
});
