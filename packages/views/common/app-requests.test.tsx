import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiClient } from "@multica/core/api";
import { I18nProvider } from "@multica/core/i18n/react";
import { workspaceFeatureKeys } from "@multica/core/billing";
import type { AppRequest } from "@multica/core/types";
import { toast } from "sonner";
import enCommon from "../locales/en/common.json";
import enSettings from "../locales/en/settings.json";
import enBilling from "../locales/en/billing.json";
import { IssueAppRequests } from "./app-requests";

const nav = vi.hoisted(() => ({
  searchParams: new URLSearchParams(),
  pathname: "/acme/issues/MUL-1",
  hash: "",
  replace: vi.fn(),
}));
const flags = vi.hoisted(() => ({ appsEnabled: false }));
const apiMocks = vi.hoisted(() => ({
  listAppRequests: vi.fn(),
  listAppCatalog: vi.fn(),
  getAppsOverview: vi.fn(),
  approveAppRequest: vi.fn(),
  rejectAppRequest: vi.fn(),
}));

vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: apiMocks,
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/workspace/hooks", () => ({
  useActorName: () => ({ getActorName: (type: string, id: string) => `${type} ${id}` }),
}));
vi.mock("@multica/core/config", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useFeatureEnabled: () => flags.appsEnabled,
}));
vi.mock("@multica/core/auth", () => ({
  useAuthStore: Object.assign(
    (select: (state: { user: { id: string } }) => unknown) => select({ user: { id: "user-1" } }),
    { getState: () => ({ user: { id: "user-1" } }) },
  ),
}));
vi.mock("../navigation", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useNavigation: () => nav,
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

afterEach(() => {
  cleanup();
  flags.appsEnabled = false;
  nav.searchParams = new URLSearchParams();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function renderCard() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidate = vi.spyOn(client, "invalidateQueries");
  render(
    <QueryClientProvider client={client}>
      <I18nProvider locale="en" resources={{ en: { common: enCommon, settings: enSettings, billing: enBilling } }}>
        <IssueAppRequests issueId="issue-1" />
      </I18nProvider>
    </QueryClientProvider>,
  );
  return { invalidate };
}

it("explains a connect the plan refused and refetches the plan", async () => {
  nav.searchParams = new URLSearchParams(
    "app_connect_error=gmail&app_connect_error_reason=feature_locked&keep=1",
  );
  const { invalidate } = renderCard();
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith(enBilling.paid_feature.locked_error));
  expect(toast.error).not.toHaveBeenCalledWith(enSettings.connected_apps.toast_connect_failed);
  expect(invalidate).toHaveBeenCalledWith({ queryKey: workspaceFeatureKeys.all("ws-1") });
  expect(nav.replace).toHaveBeenCalledWith("/acme/issues/MUL-1?keep=1");
});

it("keeps the generic retry message when an older server sends no reason", async () => {
  nav.searchParams = new URLSearchParams("app_connect_error=notion");
  renderCard();
  await waitFor(() =>
    expect(toast.error).toHaveBeenCalledWith(enSettings.connected_apps.toast_connect_failed),
  );
  expect(toast.error).not.toHaveBeenCalledWith(enBilling.paid_feature.locked_error);
  expect(nav.replace).toHaveBeenCalledWith("/acme/issues/MUL-1");
});

function makeActionRequest(id: string): AppRequest {
  return {
    id,
    kind: "action",
    status: "pending",
    agent_id: "a-1",
    issue_id: "issue-1",
    chat_session_id: null,
    approver_user_id: "user-1",
    account_scope: "personal",
    toolkit_slug: "gmail",
    tool_slug: "GMAIL_SEND_EMAIL",
    action_class: "send",
    summary: `Send email ${id}`,
    arguments: { to: "c@example.com" },
    error: null,
    reason: null,
    redacted: false,
    expires_at: "2026-11-28T12:00:00Z",
    decided_by: null,
    decided_at: null,
    created_at: "2026-11-28T11:00:00Z",
  };
}

function wireRealAppRequestFetch(requests: AppRequest[]) {
  flags.appsEnabled = true;
  apiMocks.listAppRequests.mockResolvedValue({ requests, next_before: null });
  const fetchMock = vi.fn().mockImplementation(() =>
    Promise.resolve(
      new Response(JSON.stringify({ ...requests[0], status: "approved" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
  vi.stubGlobal("fetch", fetchMock);
  const realClient = new ApiClient("https://api.example.test");
  apiMocks.approveAppRequest.mockImplementation((id, args, openedAt) =>
    realClient.approveAppRequest(id, args, openedAt),
  );
  apiMocks.rejectAppRequest.mockImplementation((id, reason, openedAt) =>
    realClient.rejectAppRequest(id, reason, openedAt),
  );
  return fetchMock;
}

describe("AppRequest attention opened headers", () => {
  it("sends X-Attention-Opened-At with elapsed seconds on single approve", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(1_700_000_000_000);
    const fetchMock = wireRealAppRequestFetch([makeActionRequest("req-1")]);

    renderCard();
    const approveBtn = await screen.findByRole("button", { name: enSettings.connected_apps.request.approve });

    vi.setSystemTime(1_700_000_016_700);
    fireEvent.click(approveBtn);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect((init.headers as Record<string, string>)["X-Attention-Opened-At"]).toBe("16");
    const body = JSON.parse(String(init.body)) as Record<string, unknown>;
    expect(body).toEqual({});
    expect("openedAt" in body).toBe(false);
  });

  it("sends X-Attention-Opened-At with elapsed seconds on reject", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(1_700_000_000_000);
    const fetchMock = wireRealAppRequestFetch([makeActionRequest("req-1")]);

    renderCard();
    const rejectBtn = await screen.findByRole("button", { name: enSettings.connected_apps.request.reject });

    vi.setSystemTime(1_700_000_008_000);
    fireEvent.click(rejectBtn);
    fireEvent.change(screen.getByPlaceholderText(enSettings.connected_apps.request.reject_reason), {
      target: { value: "Wrong recipient" },
    });

    vi.setSystemTime(1_700_000_029_300);
    fireEvent.click(screen.getByRole("button", { name: enSettings.connected_apps.request.reject }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect((init.headers as Record<string, string>)["X-Attention-Opened-At"]).toBe("29");
    const body = JSON.parse(String(init.body)) as Record<string, unknown>;
    expect(body).toEqual({ reason: "Wrong recipient" });
    expect("openedAt" in body).toBe(false);
  });

  it("sends X-Attention-Opened-At with elapsed seconds on Approve all", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(1_700_000_000_000);
    const fetchMock = wireRealAppRequestFetch([makeActionRequest("req-1"), makeActionRequest("req-2")]);

    renderCard();
    const approveAllBtn = await screen.findByRole("button", {
      name: enSettings.connected_apps.request.approve_all,
    });

    vi.setSystemTime(1_700_000_021_800);
    fireEvent.click(approveAllBtn);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    for (const call of fetchMock.mock.calls) {
      const init = call[1] as RequestInit;
      expect((init.headers as Record<string, string>)["X-Attention-Opened-At"]).toBe("21");
      const body = JSON.parse(String(init.body)) as Record<string, unknown>;
      expect(body).toEqual({});
      expect("openedAt" in body).toBe(false);
    }
  });
});
