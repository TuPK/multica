import { useEffect, useState } from "react";
import { useMe, useLogout } from "./api/hooks";
import { LoadingState } from "./components/LoadingState";
import { Layout } from "./components/Layout";
import type { StaffRole } from "./components/types";
import { Login } from "./screens/Login";
import { Overview } from "./screens/Overview";
import { Tenants } from "./screens/Tenants";
import { TenantDetail } from "./screens/TenantDetail";
import { Users } from "./screens/Users";
import { UserDetail } from "./screens/UserDetail";
import { Staff } from "./screens/Staff";
import { Accounts } from "./screens/Accounts";
import { PaymentNotices } from "./screens/PaymentNotices";
import { EinvoiceFailures } from "./screens/EinvoiceFailures";
import { Audit } from "./screens/Audit";
import { Orders } from "./screens/Orders";
import { Plans } from "./screens/Plans";

type Screen =
  | "overview"
  | "tenants"
  | "tenant-detail"
  | "users"
  | "user-detail"
  | "payment-notices"
  | "einvoices"
  | "orders"
  | "plans"
  | "accounts"
  | "staff"
  | "audit";

interface RouteState {
  screen: Screen;
  tenantId: string | null;
  userId: string | null;
}

const SCREEN_PATHS: Record<
  Exclude<Screen, "tenant-detail" | "user-detail">,
  string
> = {
  overview: "/",
  tenants: "/tenants",
  users: "/users",
  orders: "/orders",
  "payment-notices": "/payment-notices",
  einvoices: "/einvoices",
  accounts: "/accounts",
  plans: "/plans",
  staff: "/staff",
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
    return { screen: "overview", tenantId: null, userId: null };
  }
  if (clean === "/tenants") {
    return { screen: "tenants", tenantId: null, userId: null };
  }
  if (clean.startsWith("/tenants/")) {
    const id = safeDecodeSegment(clean.slice("/tenants/".length));
    if (id && !id.includes("/")) {
      return { screen: "tenant-detail", tenantId: id, userId: null };
    }
  }
  if (clean === "/users") {
    return { screen: "users", tenantId: null, userId: null };
  }
  if (clean.startsWith("/users/")) {
    const id = safeDecodeSegment(clean.slice("/users/".length));
    if (id && !id.includes("/")) {
      return { screen: "user-detail", tenantId: null, userId: id };
    }
  }
  for (const [screen, path] of Object.entries(SCREEN_PATHS)) {
    if (path !== "/" && clean === path) {
      return { screen: screen as Screen, tenantId: null, userId: null };
    }
  }
  return { screen: "overview", tenantId: null, userId: null };
}

function routeToPath(route: RouteState): string {
  if (route.screen === "tenant-detail" && route.tenantId) {
    return `/tenants/${encodeURIComponent(route.tenantId)}`;
  }
  if (route.screen === "user-detail" && route.userId) {
    return `/users/${encodeURIComponent(route.userId)}`;
  }
  if (route.screen === "tenant-detail") return "/tenants";
  if (route.screen === "user-detail") return "/users";
  return SCREEN_PATHS[route.screen];
}

function canAccessScreen(screen: Screen, role: StaffRole): boolean {
  if (screen === "staff" || screen === "accounts" || screen === "plans") {
    return role === "admin";
  }
  if (
    screen === "orders" ||
    screen === "payment-notices" ||
    screen === "einvoices"
  ) {
    return role === "billing" || role === "admin";
  }
  return true;
}

export function App() {
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
      <div className="flex min-h-screen items-center justify-center bg-app-shell">
        <LoadingState message="Loading platform console..." />
      </div>
    );
  }

  if (!me) {
    return <Login onSuccess={() => refetch()} />;
  }

  const effectiveRoute: RouteState = canAccessScreen(route.screen, me.role)
    ? route
    : { screen: "overview", tenantId: null, userId: null };

  const navigateTo = (next: RouteState) => {
    const resolved: RouteState = canAccessScreen(next.screen, me.role)
      ? next
      : { screen: "overview", tenantId: null, userId: null };
    setRoute(resolved);
    const nextPath = routeToPath(resolved);
    if (window.location.pathname !== nextPath) {
      window.history.pushState(null, "", nextPath);
    }
  };

  const handleNavigate = (screen: string) => {
    const targetScreen = (
      screen in SCREEN_PATHS ? screen : "overview"
    ) as Exclude<Screen, "tenant-detail" | "user-detail">;
    navigateTo({ screen: targetScreen, tenantId: null, userId: null });
  };

  const handleLogout = async () => {
    try {
      await logout.mutateAsync();
    } finally {
      refetch();
    }
  };

  const openTenant = (id: string) =>
    navigateTo({ screen: "tenant-detail", tenantId: id, userId: null });
  const openUser = (id: string) =>
    navigateTo({ screen: "user-detail", tenantId: null, userId: id });

  const renderScreen = () => {
    switch (effectiveRoute.screen) {
      case "overview":
        return <Overview />;
      case "tenants":
        return <Tenants onSelectTenant={openTenant} />;
      case "tenant-detail":
        if (!effectiveRoute.tenantId) {
          return <Tenants onSelectTenant={openTenant} />;
        }
        return (
          <TenantDetail
            tenantId={effectiveRoute.tenantId}
            onBack={() =>
              navigateTo({ screen: "tenants", tenantId: null, userId: null })
            }
            userRole={me.role}
          />
        );
      case "users":
        return <Users onSelectUser={openUser} />;
      case "user-detail":
        if (!effectiveRoute.userId) {
          return <Users onSelectUser={openUser} />;
        }
        return (
          <UserDetail
            userId={effectiveRoute.userId}
            onBack={() =>
              navigateTo({ screen: "users", tenantId: null, userId: null })
            }
          />
        );
      case "orders":
        return <Orders userRole={me.role} />;
      case "plans":
        return <Plans userRole={me.role} />;
      case "payment-notices":
        return <PaymentNotices userRole={me.role} />;
      case "einvoices":
        return <EinvoiceFailures userRole={me.role} />;
      case "accounts":
        return <Accounts userRole={me.role} />;
      case "staff":
        return <Staff />;
      case "audit":
        return <Audit />;
      default:
        return <Overview />;
    }
  };

  return (
    <Layout
      currentScreen={effectiveRoute.screen}
      onNavigate={handleNavigate}
      user={me}
      onLogout={handleLogout}
    >
      {renderScreen()}
    </Layout>
  );
}

export default App;
