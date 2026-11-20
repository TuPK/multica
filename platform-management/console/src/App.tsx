import { useEffect, useState } from "react";
import { Theme } from "@astryxdesign/core/theme";
import { neutralTheme } from "@astryxdesign/theme-neutral/built";
import { useMe, useLogout } from "./api/hooks";
import { useLanguage } from "./i18n";
import { LoadingState } from "./components/LoadingState";
import { Layout } from "./components/Layout";
import type { StaffRole } from "./components/types";
import { Login } from "./screens/Login";
import { Overview } from "./screens/Overview";
import { Tenants } from "./screens/Tenants";
import { TenantDetail } from "./screens/TenantDetail";
import { Workspaces } from "./screens/Workspaces";
import { WorkspaceDetail } from "./screens/WorkspaceDetail";
import { Users } from "./screens/Users";
import { UserDetail } from "./screens/UserDetail";
import { Staff } from "./screens/Staff";
import { Accounts } from "./screens/Accounts";
import { DeletionRequests } from "./screens/DeletionRequests";
import { PaymentNotices } from "./screens/PaymentNotices";
import { EinvoiceFailures } from "./screens/EinvoiceFailures";
import { Audit } from "./screens/Audit";
import { Orders } from "./screens/Orders";
import { Plans } from "./screens/Plans";
import { PlanHistory } from "./screens/PlanHistory";
import { Resources } from "./screens/Resources";
import { Runtimes } from "./screens/Runtimes";
import { RuntimeDetail } from "./screens/RuntimeDetail";
import { TenantPolicy } from "./screens/TenantPolicy";
import { QuotaRequests } from "./screens/QuotaRequests";
import { Approvals } from "./screens/Approvals";
import { LegalHolds } from "./screens/LegalHolds";
import { t } from "./i18n";

type Screen =
  | "overview"
  | "tenants"
  | "tenant-detail"
  | "tenant-policy"
  | "workspaces"
  | "workspace-detail"
  | "users"
  | "user-detail"
  | "resources"
  | "runtimes"
  | "runtime-detail"
  | "quota-requests"
  | "approvals"
  | "legal-holds"
  | "payment-notices"
  | "einvoices"
  | "orders"
  | "plans"
  | "plan-history"
  | "accounts"
  | "staff"
  | "deletion-requests"
  | "audit";

interface RouteState {
  screen: Screen;
  // The tenant on screen, or for a workspace the tenant it was opened from.
  tenantId: string | null;
  workspaceId: string | null;
  userId: string | null;
  runtimeId: string | null;
  planKey: string | null;
}

const SCREEN_PATHS: Record<
  Exclude<
    Screen,
    "tenant-detail" | "workspace-detail" | "user-detail" | "runtime-detail" | "plan-history"
  >,
  string
> = {
  overview: "/",
  tenants: "/tenants",
  "tenant-policy": "/policy",
  workspaces: "/workspaces",
  users: "/users",
  resources: "/resources",
  runtimes: "/runtimes",
  "quota-requests": "/quota-requests",
  approvals: "/approvals",
  "legal-holds": "/legal-holds",
  orders: "/orders",
  "payment-notices": "/payment-notices",
  einvoices: "/einvoices",
  accounts: "/accounts",
  plans: "/plans",
  staff: "/staff",
  "deletion-requests": "/deletion-requests",
  audit: "/audit",
};

function safeDecodeSegment(raw: string): string | null {
  try {
    return decodeURIComponent(raw);
  } catch {
    return null;
  }
}

function parseRoute(pathname: string): RouteState {
  const clean = pathname.replace(/\/+$/, "") || "/";
  if (clean === "/") {
    return { screen: "overview", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null };
  }
  if (clean === "/tenants") {
    return { screen: "tenants", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null };
  }
  if (clean === "/workspaces") {
    return { screen: "workspaces", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null };
  }
  if (clean.startsWith("/tenants/") && clean.endsWith("/policy")) {
    const rawId = clean.slice("/tenants/".length, -"/policy".length);
    const id = safeDecodeSegment(rawId);
    if (id && !id.includes("/")) {
      return { screen: "tenant-policy", tenantId: id, workspaceId: null, userId: null, runtimeId: null, planKey: null };
    }
  }
  if (clean.startsWith("/tenants/")) {
    const id = safeDecodeSegment(clean.slice("/tenants/".length));
    if (id && !id.includes("/")) {
      return { screen: "tenant-detail", tenantId: id, workspaceId: null, userId: null, runtimeId: null, planKey: null };
    }
  }
  if (clean.startsWith("/workspaces/")) {
    const id = safeDecodeSegment(clean.slice("/workspaces/".length));
    if (id && !id.includes("/")) {
      return { screen: "workspace-detail", tenantId: null, workspaceId: id, userId: null, runtimeId: null, planKey: null };
    }
  }
  if (clean.startsWith("/plans/") && clean.endsWith("/history")) {
    const rawKey = clean.slice("/plans/".length, -"/history".length);
    const key = safeDecodeSegment(rawKey);
    if (key && !key.includes("/")) {
      return { screen: "plan-history", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: key };
    }
  }
  if (clean === "/users") {
    return { screen: "users", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null };
  }
  if (clean.startsWith("/users/")) {
    const id = safeDecodeSegment(clean.slice("/users/".length));
    if (id && !id.includes("/")) {
      return { screen: "user-detail", tenantId: null, workspaceId: null, userId: id, runtimeId: null, planKey: null };
    }
  }
  if (clean === "/resources") {
    return { screen: "resources", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null };
  }
  if (clean === "/runtimes") {
    return { screen: "runtimes", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null };
  }
  if (clean.startsWith("/runtimes/")) {
    const id = safeDecodeSegment(clean.slice("/runtimes/".length));
    if (id && !id.includes("/")) {
      return { screen: "runtime-detail", tenantId: null, workspaceId: null, userId: null, runtimeId: id, planKey: null };
    }
  }
  for (const [screen, path] of Object.entries(SCREEN_PATHS)) {
    if (path !== "/" && clean === path) {
      return { screen: screen as Screen, tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null };
    }
  }
  return { screen: "overview", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null };
}

function routeToPath(route: RouteState): string {
  if (route.screen === "tenant-detail" && route.tenantId) {
    return `/tenants/${encodeURIComponent(route.tenantId)}`;
  }
  if (route.screen === "tenant-policy" && route.tenantId) {
    return `/tenants/${encodeURIComponent(route.tenantId)}/policy`;
  }
  if (route.screen === "workspace-detail" && route.workspaceId) {
    return `/workspaces/${encodeURIComponent(route.workspaceId)}`;
  }
  if (route.screen === "user-detail" && route.userId) {
    return `/users/${encodeURIComponent(route.userId)}`;
  }
  if (route.screen === "runtime-detail" && route.runtimeId) {
    return `/runtimes/${encodeURIComponent(route.runtimeId)}`;
  }
  if (route.screen === "plan-history" && route.planKey) {
    return `/plans/${encodeURIComponent(route.planKey)}/history`;
  }
  if (route.screen === "tenant-detail") return "/tenants";
  if (route.screen === "workspace-detail") return "/workspaces";
  if (route.screen === "user-detail") return "/users";
  if (route.screen === "runtime-detail") return "/runtimes";
  if (route.screen === "plan-history") return "/plans";
  return SCREEN_PATHS[route.screen];
}

function canAccessScreen(screen: string, role: StaffRole): boolean {
  if (screen === "overview") {
    return role !== "admin";
  }
  if (
    screen === "staff" ||
    screen === "accounts" ||
    screen === "resources" ||
    screen === "approvals" ||
    screen === "legal-holds"
  ) {
    return role === "system_admin";
  }
  if (screen === "runtimes" || screen === "runtime-detail") {
    return role === "system_admin" || role === "admin" || role === "support";
  }
  if (screen === "tenant-policy") {
    return role === "system_admin" || role === "admin" || role === "support";
  }
  if (screen === "quota-requests") {
    return role === "system_admin" || role === "admin" || role === "billing";
  }
  if (screen === "plans") {
    return role === "system_admin" || role === "admin";
  }
  if (screen === "plan-history") {
    return role === "system_admin" || role === "billing";
  }
  if (screen === "deletion-requests") {
    return role === "system_admin" || role === "admin";
  }
  if (screen === "orders") {
    return role === "billing" || role === "admin" || role === "system_admin";
  }
  if (screen === "payment-notices" || screen === "einvoices") {
    return role === "billing" || role === "system_admin";
  }
  return true;
}

function defaultRouteForRole(role: StaffRole): RouteState {
  return {
    screen: role === "admin" ? "tenants" : "overview",
    tenantId: null,
    workspaceId: null,
    userId: null,
    runtimeId: null,
    planKey: null,
  };
}

export function App() {
  // Screens read strings with the plain t() lookup, so subscribing here
  // re-renders the tree (and every screen under it) when the language changes.
  const language = useLanguage();
  useEffect(() => {
    document.documentElement.lang = language;
  }, [language]);

  const { data: me, isLoading, refetch } = useMe();
  const logout = useLogout();

  const [route, setRoute] = useState<RouteState>(() =>
    parseRoute(typeof window !== "undefined" ? window.location.pathname : "/"),
  );

  useEffect(() => {
    const handlePopState = () => {
      setRoute(parseRoute(window.location.pathname));
    };
    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  const isUnauthorizedRoute = Boolean(me && !canAccessScreen(route.screen, me.role));

  useEffect(() => {
    if (!isUnauthorizedRoute) return;
    if (window.location.pathname !== "/") {
      window.history.replaceState(null, "", "/");
    }
  }, [isUnauthorizedRoute]);

  if (isLoading) {
    return (
      <Theme theme={neutralTheme}>
        <div className="flex min-h-screen items-center justify-center bg-app-shell">
          <LoadingState message={t("Loading platform console...")} />
        </div>
      </Theme>
    );
  }

  if (!me) {
    return (
      <Theme theme={neutralTheme}>
        <Login onSuccess={() => refetch()} />
      </Theme>
    );
  }

  const effectiveRoute: RouteState = canAccessScreen(route.screen, me.role)
    ? route
    : defaultRouteForRole(me.role);

  const navigateTo = (next: RouteState) => {
    const resolved: RouteState = canAccessScreen(next.screen, me.role)
      ? next
      : defaultRouteForRole(me.role);
    setRoute(resolved);
    const nextPath = routeToPath(resolved);
    if (window.location.pathname !== nextPath) {
      window.history.pushState(null, "", nextPath);
    }
  };

  const handleNavigate = (screen: string) => {
    const targetScreen = (
      screen in SCREEN_PATHS ? screen : me.role === "admin" ? "tenants" : "overview"
    ) as Exclude<
      Screen,
      "tenant-detail" | "workspace-detail" | "user-detail" | "runtime-detail" | "plan-history"
    >;
    navigateTo({
      screen: targetScreen,
      tenantId: null,
      workspaceId: null,
      userId: null,
      runtimeId: null,
      planKey: null,
    });
  };

  const handleLogout = async () => {
    try {
      await logout.mutateAsync();
    } finally {
      refetch();
    }
  };

  const openTenant = (id: string) =>
    navigateTo({ screen: "tenant-detail", tenantId: id, workspaceId: null, userId: null, runtimeId: null, planKey: null });
  const openWorkspace = (id: string, fromTenantId: string | null) =>
    navigateTo({ screen: "workspace-detail", tenantId: fromTenantId, workspaceId: id, userId: null, runtimeId: null, planKey: null });
  const openUser = (id: string, fromTenantId: string | null = null) =>
    navigateTo({ screen: "user-detail", tenantId: fromTenantId, workspaceId: null, userId: id, runtimeId: null, planKey: null });
  const openRuntime = (id: string) =>
    navigateTo({ screen: "runtime-detail", tenantId: null, workspaceId: null, userId: null, runtimeId: id, planKey: null });
  const openPlanHistory = (planKey: string) =>
    navigateTo({ screen: "plan-history", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey });

  const renderScreen = () => {
    switch (effectiveRoute.screen) {
      case "overview":
        return <Overview onNavigate={handleNavigate} canNavigate={(s) => canAccessScreen(s, me.role)} />;
      case "tenants":
        return <Tenants onSelectTenant={openTenant} />;
      case "workspaces":
        return <Workspaces onSelectWorkspace={(id) => openWorkspace(id, null)} userRole={me.role} />;
      case "tenant-detail":
        if (!effectiveRoute.tenantId) {
          return <Tenants onSelectTenant={openTenant} />;
        }
        return (
          <TenantDetail
            tenantId={effectiveRoute.tenantId}
            onBack={() =>
              navigateTo({ screen: "tenants", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null })
            }
            onSelectWorkspace={(id) => openWorkspace(id, effectiveRoute.tenantId)}
            onSelectUser={(id) => openUser(id, effectiveRoute.tenantId)}
            userRole={me.role}
          />
        );
      case "workspace-detail": {
        if (!effectiveRoute.workspaceId) {
          return <Workspaces onSelectWorkspace={(id) => openWorkspace(id, null)} userRole={me.role} />;
        }
        const fromTenant = effectiveRoute.tenantId;
        return (
          <WorkspaceDetail
            workspaceId={effectiveRoute.workspaceId}
            backLabel={fromTenant ? t("Back to tenant") : t("Back to workspaces")}
            onBack={() =>
              fromTenant
                ? openTenant(fromTenant)
                : navigateTo({ screen: "workspaces", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null })
            }
            userRole={me.role}
          />
        );
      }
      case "users":
        return <Users onSelectUser={openUser} />;
      case "user-detail": {
        if (!effectiveRoute.userId) {
          return <Users onSelectUser={openUser} />;
        }
        const fromTenant = effectiveRoute.tenantId;
        return (
          <UserDetail
            userId={effectiveRoute.userId}
            tenantId={fromTenant ?? undefined}
            backLabel={fromTenant ? t("Back to tenant") : t("Back to users")}
            onBack={() =>
              fromTenant
                ? openTenant(fromTenant)
                : navigateTo({ screen: "users", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null })
            }
            onSelectWorkspace={(id) => openWorkspace(id, fromTenant)}
            userRole={me.role}
          />
        );
      }
      case "resources":
        return (
          <Resources
            onSelectTenant={openTenant}
            onSelectWorkspace={(id) => openWorkspace(id, null)}
            userRole={me.role}
          />
        );
      case "runtimes":
        return <Runtimes onSelectRuntime={openRuntime} userRole={me.role} />;
      case "runtime-detail":
        if (!effectiveRoute.runtimeId) {
          return <Runtimes onSelectRuntime={openRuntime} userRole={me.role} />;
        }
        return (
          <RuntimeDetail
            runtimeId={effectiveRoute.runtimeId}
            onBack={() =>
              navigateTo({ screen: "runtimes", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null })
            }
            onSelectWorkspace={(id) => openWorkspace(id, null)}
            userRole={me.role}
          />
        );
      case "orders":
        return <Orders userRole={me.role} />;
      case "plans":
        return <Plans userRole={me.role} onOpenHistory={openPlanHistory} />;
      case "plan-history":
        if (!effectiveRoute.planKey) {
          return <Plans userRole={me.role} onOpenHistory={openPlanHistory} />;
        }
        return (
          <PlanHistory
            planKey={effectiveRoute.planKey}
            onBack={() =>
              navigateTo({ screen: "plans", tenantId: null, workspaceId: null, userId: null, runtimeId: null, planKey: null })
            }
            userRole={me.role}
          />
        );
      case "payment-notices":
        return <PaymentNotices userRole={me.role} />;
      case "einvoices":
        return <EinvoiceFailures userRole={me.role} />;
      case "accounts":
        return <Accounts userRole={me.role} />;
      case "staff":
        return <Staff />;
      case "tenant-policy":
        return (
          <TenantPolicy
            tenantId={me.role === "admin" ? (me.tenant_id ?? undefined) : (effectiveRoute.tenantId ?? undefined)}
            userRole={me.role}
          />
        );
      case "quota-requests":
        return <QuotaRequests userRole={me.role} />;
      case "approvals":
        return <Approvals userRole={me.role} />;
      case "legal-holds":
        return <LegalHolds userRole={me.role} />;
      case "deletion-requests":
        return <DeletionRequests userRole={me.role} />;
      case "audit":
        return <Audit />;
      default:
        return me.role === "admin" ? (
          <Tenants onSelectTenant={openTenant} />
        ) : (
          <Overview onNavigate={handleNavigate} canNavigate={(s) => canAccessScreen(s, me.role)} />
        );
    }
  };

  return (
    <Theme theme={neutralTheme}>
      <Layout
        currentScreen={effectiveRoute.screen}
        onNavigate={handleNavigate}
        user={me}
        onLogout={handleLogout}
        workspaceFromTenant={Boolean(effectiveRoute.tenantId)}
      >
        {renderScreen()}
      </Layout>
    </Theme>
  );
}

export default App;
