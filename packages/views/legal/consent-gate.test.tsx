// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { renderWithI18n } from "../test/i18n";

const mockMutateAsync = vi.hoisted(() => vi.fn());
const acceptState = vi.hoisted(() => ({
  isPending: false,
}));

vi.mock("@multica/core/account", () => ({
  useAcceptConsents: () => ({
    mutateAsync: mockMutateAsync,
    isPending: acceptState.isPending,
  }),
}));

const mockSetUser = vi.hoisted(() => vi.fn());
const currentUserState = vi.hoisted(() => ({
  user: null as {
    id: string;
    name: string;
    email: string;
    consent_required?: Array<{ document: string; version: string; url: string }>;
  } | null,
}));

vi.mock("@multica/core/auth", () => {
  const useAuthStore = Object.assign(
    (
      selector?: (state: {
        user: typeof currentUserState.user;
        setUser: typeof mockSetUser;
      }) => unknown,
    ) =>
      selector
        ? selector({ user: currentUserState.user, setUser: mockSetUser })
        : { user: currentUserState.user, setUser: mockSetUser },
    { getState: () => ({ user: currentUserState.user, setUser: mockSetUser }) },
  );
  return { useAuthStore };
});

import { ConsentGate } from "./consent-gate";

describe("ConsentGate", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    acceptState.isPending = false;
    currentUserState.user = null;
  });

  it("renders nothing when user is null", () => {
    currentUserState.user = null;
    const { container } = renderWithI18n(<ConsentGate />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when consent_required is absent", () => {
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
    };
    const { container } = renderWithI18n(<ConsentGate />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when consent_required is empty", () => {
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      consent_required: [],
    };
    const { container } = renderWithI18n(<ConsentGate />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders dialog with document names and links when consent_required has items", () => {
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      consent_required: [
        {
          document: "terms",
          version: "2026-11",
          url: "https://example.com/legal/terms",
        },
        {
          document: "privacy",
          version: "2026-11",
          url: "https://example.com/legal/privacy",
        },
      ],
    };

    renderWithI18n(<ConsentGate />);

    expect(screen.getByRole("heading", { name: "Required legal documents" })).toBeInTheDocument();

    const termsLink = screen.getByRole("link", { name: /Terms of Service/i });
    expect(termsLink).toHaveAttribute("href", "https://example.com/legal/terms");
    expect(termsLink).toHaveAttribute("target", "_blank");

    const privacyLink = screen.getByRole("link", { name: /Privacy Policy/i });
    expect(privacyLink).toHaveAttribute("href", "https://example.com/legal/privacy");
    expect(privacyLink).toHaveAttribute("target", "_blank");

    expect(screen.getByRole("button", { name: "Accept" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Close" })).not.toBeInTheDocument();
  });

  it("does not dismiss dialog when pressing Escape", () => {
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      consent_required: [
        {
          document: "terms",
          version: "2026-11",
          url: "https://example.com/legal/terms",
        },
      ],
    };

    renderWithI18n(<ConsentGate />);

    expect(screen.getByRole("heading", { name: "Required legal documents" })).toBeInTheDocument();

    fireEvent.keyDown(window, { key: "Escape" });

    expect(screen.getByRole("heading", { name: "Required legal documents" })).toBeInTheDocument();
  });

  it("disables button and shows busy indicator when isPending is true", () => {
    acceptState.isPending = true;
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      consent_required: [
        {
          document: "terms",
          version: "2026-11",
          url: "https://example.com/legal/terms",
        },
      ],
    };

    renderWithI18n(<ConsentGate />);

    const button = screen.getByRole("button", { name: "Accept" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "true");
  });

  it("calls mutateAsync on accept and closes dialog on success", async () => {
    mockMutateAsync.mockResolvedValueOnce({ consent_required: [] });
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      consent_required: [
        {
          document: "terms",
          version: "2026-11",
          url: "https://example.com/legal/terms",
        },
        {
          document: "privacy",
          version: "2026-11",
          url: "https://example.com/legal/privacy",
        },
      ],
    };

    renderWithI18n(<ConsentGate />);

    const acceptBtn = screen.getByRole("button", { name: "Accept" });
    fireEvent.click(acceptBtn);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith({
        documents: [
          { document: "terms", version: "2026-11" },
          { document: "privacy", version: "2026-11" },
        ],
      });
    });

    await waitFor(() => {
      expect(screen.queryByRole("heading", { name: "Required legal documents" })).not.toBeInTheDocument();
    });
    expect(mockSetUser).toHaveBeenCalledWith({
      ...currentUserState.user,
      consent_required: [],
    });
  });

  it("keeps dialog open when mutation rejects", async () => {
    mockMutateAsync.mockRejectedValueOnce(new Error("Version mismatch"));
    currentUserState.user = {
      id: "u-1",
      name: "Alice",
      email: "alice@example.com",
      consent_required: [
        {
          document: "terms",
          version: "2026-11",
          url: "https://example.com/legal/terms",
        },
      ],
    };

    renderWithI18n(<ConsentGate />);

    const acceptBtn = screen.getByRole("button", { name: "Accept" });
    fireEvent.click(acceptBtn);

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledTimes(1);
    });

    expect(screen.getByRole("heading", { name: "Required legal documents" })).toBeInTheDocument();
  });
});
