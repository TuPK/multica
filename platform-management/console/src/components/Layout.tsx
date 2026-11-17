import React from "react";
import {
  BarChart3,
  Building2,
  Users,
  Shield,
  Activity,
  LogOut,
  CreditCard,
  Layers,
  UserPlus,
} from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { StatusBadge } from "./StatusBadge";
import type { MeResponse } from "./types";

interface LayoutProps {
  currentScreen: string;
  onNavigate: (screen: string) => void;
  user: MeResponse;
  onLogout: () => void;
  children: React.ReactNode;
}

export function Layout({
  currentScreen,
  onNavigate,
  user,
  onLogout,
  children,
}: LayoutProps) {
  const navItems = [
    { id: "overview", label: "Overview", icon: BarChart3 },
    { id: "tenants", label: "Tenants", icon: Building2 },
    { id: "users", label: "Users", icon: Users },
    ...(user.role === "billing" || user.role === "admin"
      ? [{ id: "orders", label: "Orders", icon: CreditCard }]
      : []),
    ...(user.role === "admin"
      ? [
          { id: "accounts", label: "Accounts", icon: UserPlus },
          { id: "plans", label: "Plans", icon: Layers },
          { id: "staff", label: "Staff", icon: Shield },
        ]
      : []),
    { id: "audit", label: "Audit Log", icon: Activity },
  ];

  return (
    <div className="min-h-screen bg-background text-foreground flex flex-col">
      <header className="sticky top-0 z-40 border-b border-surface-border bg-surface/95 backdrop-blur-sm">
        <div className="flex h-14 items-center justify-between px-6">
          <div className="flex items-center gap-6">
            <div className="flex items-center gap-2 font-heading font-semibold text-title-sm">
              <Shield className="size-5 text-primary" />
              <span>Platform Console</span>
            </div>
            <nav className="flex items-center gap-1" aria-label="Main Navigation">
              {navItems.map((item) => {
                const Icon = item.icon;
                const isActive =
                  currentScreen === item.id ||
                  (item.id === "tenants" && currentScreen === "tenant-detail") ||
                  (item.id === "users" && currentScreen === "user-detail");
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => onNavigate(item.id)}
                    className={`flex items-center gap-2 rounded-md px-3 py-1.5 text-body font-medium transition-colors ${
                      isActive
                        ? "bg-accent text-accent-foreground"
                        : "text-muted-foreground hover:bg-muted hover:text-foreground"
                    }`}
                    aria-current={isActive ? "page" : undefined}
                  >
                    <Icon className="size-4" />
                    <span>{item.label}</span>
                  </button>
                );
              })}
            </nav>
          </div>

          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2 text-body">
              <span className="text-muted-foreground">{user.email}</span>
              <StatusBadge status={user.role} />
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={onLogout}
              className="gap-1.5 text-muted-foreground hover:text-foreground"
              aria-label="Sign out"
            >
              <LogOut className="size-4" />
              <span>Sign out</span>
            </Button>
          </div>
        </div>
      </header>

      <main className="flex-1 p-6">
        <div className="mx-auto max-w-7xl">{children}</div>
      </main>
    </div>
  );
}
