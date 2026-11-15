// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { Login } from "./Login";

const mockSendCodeMutateAsync = vi.fn();
const mockVerifyCodeMutateAsync = vi.fn();

vi.mock("../api/hooks", () => ({
  useSendCode: () => ({
    mutateAsync: mockSendCodeMutateAsync,
    isPending: false,
  }),
  useVerifyCode: () => ({
    mutateAsync: mockVerifyCodeMutateAsync,
    isPending: false,
  }),
}));

describe("Login screen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders email input initially and executes send code flow", async () => {
    mockSendCodeMutateAsync.mockResolvedValue({});
    const onSuccess = vi.fn();

    render(<Login onSuccess={onSuccess} />);

    const emailInput = screen.getByLabelText(/Email address/i);
    expect(emailInput).toBeInTheDocument();

    const sendButton = screen.getByRole("button", { name: /Send sign-in code/i });
    expect(sendButton).toBeDisabled();

    fireEvent.change(emailInput, { target: { value: "staff@example.com" } });
    expect(sendButton).toBeEnabled();

    fireEvent.click(sendButton);

    await waitFor(() => {
      expect(mockSendCodeMutateAsync).toHaveBeenCalledWith({ email: "staff@example.com" });
    });

    expect(
      screen.getByText("If this address can sign in, a code has been sent")
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/6-digit code/i)).toBeInTheDocument();
  });

  it("always shows message after send code even if request rejected", async () => {
    mockSendCodeMutateAsync.mockRejectedValue(new Error("Network error"));
    const onSuccess = vi.fn();

    render(<Login onSuccess={onSuccess} />);

    const emailInput = screen.getByLabelText(/Email address/i);
    fireEvent.change(emailInput, { target: { value: "unknown@example.com" } });
    fireEvent.click(screen.getByRole("button", { name: /Send sign-in code/i }));

    await waitFor(() => {
      expect(mockSendCodeMutateAsync).toHaveBeenCalledWith({ email: "unknown@example.com" });
    });

    expect(
      screen.getByText("If this address can sign in, a code has been sent")
    ).toBeInTheDocument();
  });

  it("wrong code shows 'That code is not valid.' and nothing more", async () => {
    mockSendCodeMutateAsync.mockResolvedValue({});
    mockVerifyCodeMutateAsync.mockRejectedValue(new Error("Invalid code"));
    const onSuccess = vi.fn();

    render(<Login onSuccess={onSuccess} />);

    // Step 1: Send code
    fireEvent.change(screen.getByLabelText(/Email address/i), {
      target: { value: "staff@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Send sign-in code/i }));

    // Step 2: Code step
    await waitFor(() => {
      expect(screen.getByLabelText(/6-digit code/i)).toBeInTheDocument();
    });

    const codeInput = screen.getByLabelText(/6-digit code/i);
    fireEvent.change(codeInput, { target: { value: "123456" } });
    fireEvent.click(screen.getByRole("button", { name: /Sign in/i }));

    await waitFor(() => {
      expect(mockVerifyCodeMutateAsync).toHaveBeenCalledWith({
        email: "staff@example.com",
        code: "123456",
      });
    });

    expect(screen.getByText("That code is not valid.")).toBeInTheDocument();
    expect(onSuccess).not.toHaveBeenCalled();
  });

  it("valid code calls onSuccess to refetch useMe", async () => {
    mockSendCodeMutateAsync.mockResolvedValue({});
    mockVerifyCodeMutateAsync.mockResolvedValue({ user_id: "u-1", role: "admin" });
    const onSuccess = vi.fn();

    render(<Login onSuccess={onSuccess} />);

    fireEvent.change(screen.getByLabelText(/Email address/i), {
      target: { value: "admin@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Send sign-in code/i }));

    await waitFor(() => {
      expect(screen.getByLabelText(/6-digit code/i)).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText(/6-digit code/i), {
      target: { value: "654321" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Sign in/i }));

    await waitFor(() => {
      expect(mockVerifyCodeMutateAsync).toHaveBeenCalledWith({
        email: "admin@example.com",
        code: "654321",
      });
      expect(onSuccess).toHaveBeenCalledTimes(1);
    });
  });

  it("allows switching back to email form via 'Use a different email'", async () => {
    mockSendCodeMutateAsync.mockResolvedValue({});
    render(<Login onSuccess={vi.fn()} />);

    fireEvent.change(screen.getByLabelText(/Email address/i), {
      target: { value: "first@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Send sign-in code/i }));

    await waitFor(() => {
      expect(screen.getByLabelText(/6-digit code/i)).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /Use a different email/i }));
    expect(screen.getByLabelText(/Email address/i)).toBeInTheDocument();
  });
});
