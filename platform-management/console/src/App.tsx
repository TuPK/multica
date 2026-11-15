import { useState } from "react";
import { useMe, useLogout } from "./api/hooks";
import { LoadingState } from "./components/LoadingState";
import { Layout } from "./components/Layout";
import { Login } from "./screens/Login";
import { Overview } from "./screens/Overview";
import { Tenants } from "./screens/Tenants";
import { TenantDetail } from "./screens/TenantDetail";
import { Users } from "./screens/Users";
import { UserDetail } from "./screens/UserDetail";
import { Staff } from "./screens/Staff";
import { Audit } from "./screens/Audit";

type Screen =
  | "overview"
  | "tenants"
  | "tenant-detail"
  | "users"
  | "user-detail"
  | "staff"
  | "audit";

export function App() {
  const { data: me, isLoading, refetch } = useMe();
  const logout = useLogout();

  const [currentScreen, setCurrentScreen] = useState<Screen>("overview");
  const [selectedTenantId, setSelectedTenantId] = useState<string | null>(null);
  const [selectedUserId, setSelectedUserId] = useState<string | null>(null);

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <LoadingState message="Loading platform console..." />
      </div>
    );
  }

  if (!me) {
    return <Login onSuccess={() => refetch()} />;
  }

  const handleNavigate = (screen: string) => {
    // Role gating: billing and support do not see the staff screen
    if (screen === "staff" && me.role !== "admin") {
      setCurrentScreen("overview");
      return;
    }
    setCurrentScreen(screen as Screen);
  };

  const handleLogout = async () => {
    try {
      await logout.mutateAsync();
    } finally {
      refetch();
    }
  };

  const renderScreen = () => {
    switch (currentScreen) {
      case "overview":
        return <Overview />;
      case "tenants":
        return (
          <Tenants
            onSelectTenant={(id) => {
              setSelectedTenantId(id);
              setCurrentScreen("tenant-detail");
            }}
          />
        );
      case "tenant-detail":
        if (!selectedTenantId) {
          return (
            <Tenants
              onSelectTenant={(id) => {
                setSelectedTenantId(id);
                setCurrentScreen("tenant-detail");
              }}
            />
          );
        }
        return (
          <TenantDetail
            tenantId={selectedTenantId}
            onBack={() => {
              setSelectedTenantId(null);
              setCurrentScreen("tenants");
            }}
            userRole={me.role}
          />
        );
      case "users":
        return (
          <Users
            onSelectUser={(id) => {
              setSelectedUserId(id);
              setCurrentScreen("user-detail");
            }}
          />
        );
      case "user-detail":
        if (!selectedUserId) {
          return (
            <Users
              onSelectUser={(id) => {
                setSelectedUserId(id);
                setCurrentScreen("user-detail");
              }}
            />
          );
        }
        return (
          <UserDetail
            userId={selectedUserId}
            onBack={() => {
              setSelectedUserId(null);
              setCurrentScreen("users");
            }}
          />
        );
      case "staff":
        if (me.role !== "admin") {
          return <Overview />;
        }
        return <Staff />;
      case "audit":
        return <Audit />;
      default:
        return <Overview />;
    }
  };

  return (
    <Layout
      currentScreen={currentScreen}
      onNavigate={handleNavigate}
      user={me}
      onLogout={handleLogout}
    >
      {renderScreen()}
    </Layout>
  );
}

export default App;
