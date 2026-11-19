import { useState, type ComponentType, type ReactNode } from "react";
import {
  Activity,
  BarChart3,
  Building2,
  CreditCard,
  FileText,
  Inbox,
  Layers,
  LogOut,
  Menu,
  Shield,
  UserPlus,
  Users,
  X,
} from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { cn } from "@multica/ui/lib/utils";
import { StatusBadge } from "./StatusBadge";
import type { MeResponse } from "./types";

interface LayoutProps {
  currentScreen: string;
  onNavigate: (screen: string) => void;
  user: MeResponse;
  onLogout: () => void;
  children: ReactNode;
}

interface NavItem {
  id: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
}

interface NavGroup {
  label: string;
  items: NavItem[];
}

export function Layout({
  currentScreen,
  onNavigate,
  user,
  onLogout,
  children,
}: LayoutProps) {
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  const isBillingOrAdmin = user.role === "billing" || user.role === "admin";
  const isAdmin = user.role === "admin";

  const commercialItems: NavItem[] = [
    ...(isBillingOrAdmin
      ? [
          { id: "orders", label: "Orders", icon: CreditCard },
          { id: "payment-notices", label: "Payment notices", icon: Inbox },
          { id: "einvoices", label: "E-invoices", icon: FileText },
        ]
      : []),
    ...(isAdmin
      ? [
          { id: "accounts", label: "Accounts", icon: UserPlus },
          { id: "plans", label: "Plans", icon: Layers },
        ]
      : []),
  ];

  const navGroups: NavGroup[] = [
    {
      label: "Operations",
      items: [
        { id: "overview", label: "Overview", icon: BarChart3 },
        { id: "tenants", label: "Tenants", icon: Building2 },
        { id: "users", label: "Users", icon: Users },
      ],
    },
    ...(commercialItems.length > 0
      ? [{ label: "Commercial", items: commercialItems }]
      : []),
    {
      label: "Administration",
      items: [
        ...(isAdmin ? [{ id: "staff", label: "Staff", icon: Shield }] : []),
        { id: "audit", label: "Audit Log", icon: Activity },
      ],
    },
  ];

  const handleSelectNav = (id: string) => {
    setMobileNavOpen(false);
    onNavigate(id);
  };

  return (
    <div className="flex min-h-screen flex-col bg-app-shell text-foreground md:flex-row">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-surface focus:px-3 focus:py-1.5 focus:text-body focus:font-medium focus:shadow-md"
      >
        Skip to content
      </a>

      {/* Compact mobile bar */}
      <div className="flex h-14 items-center justify-between border-b border-sidebar-border bg-sidebar px-4 md:hidden">
        <div className="flex items-center gap-2 font-heading text-title-sm font-semibold">
          <Shield className="size-4 text-primary" aria-hidden="true" />
          <span>Platform Console</span>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={() => setMobileNavOpen((prev) => !prev)}
          aria-label={mobileNavOpen ? "Close navigation" : "Open navigation"}
          aria-expanded={mobileNavOpen}
        >
          {mobileNavOpen ? (
            <X className="size-4" aria-hidden="true" />
          ) : (
            <Menu className="size-4" aria-hidden="true" />
          )}
        </Button>
      </div>

      {/* Left SideNav */}
      <aside
        aria-label="Sidebar"
        className={cn(
          "w-full shrink-0 flex-col border-b border-sidebar-border bg-sidebar text-sidebar-foreground md:sticky md:top-0 md:flex md:h-screen md:w-60 md:border-r md:border-b-0",
          mobileNavOpen ? "flex" : "hidden",
        )}
      >
        <div className="hidden h-14 shrink-0 items-center gap-2 border-b border-sidebar-border px-4 font-heading text-title-sm font-semibold md:flex">
          <Shield className="size-4 text-primary" aria-hidden="true" />
          <span>Platform Console</span>
        </div>

        <nav
          aria-label="Main Navigation"
          className="flex-1 space-y-5 overflow-y-auto px-3 py-4"
        >
          {navGroups.map((group) => (
            <div key={group.label} className="space-y-1">
              <div className="px-2.5 pb-1 text-micro font-semibold uppercase tracking-wider text-muted-foreground">
                {group.label}
              </div>
              <div className="space-y-0.5">
                {group.items.map((item) => {
                  const Icon = item.icon;
                  const isActive =
                    currentScreen === item.id ||
                    (item.id === "tenants" && currentScreen === "tenant-detail") ||
                    (item.id === "users" && currentScreen === "user-detail");
                  return (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => handleSelectNav(item.id)}
                      aria-current={isActive ? "page" : undefined}
                      className={cn(
                        "flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-left text-body font-medium transition-colors",
                        isActive
                          ? "bg-surface-selected text-surface-selected-foreground hover:bg-surface-selected"
                          : "text-muted-foreground hover:bg-surface-hover hover:text-foreground",
                      )}
                    >
                      <Icon className="size-4 shrink-0" aria-hidden="true" />
                      <span className="truncate">{item.label}</span>
                    </button>
                  );
                })}
              </div>
            </div>
          ))}
        </nav>

        <div className="shrink-0 space-y-2.5 border-t border-sidebar-border p-3">
          <div className="flex items-center justify-between gap-2 px-1">
            <span
              className="min-w-0 flex-1 truncate text-label text-muted-foreground"
              title={user.email}
            >
              {user.email}
            </span>
            <StatusBadge status={user.role} />
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onLogout}
            className="w-full justify-start gap-2 text-muted-foreground hover:text-foreground"
            aria-label="Sign out"
          >
            <LogOut className="size-4" aria-hidden="true" />
            <span>Sign out</span>
          </Button>
        </div>
      </aside>

      <main
        id="main-content"
        className="min-w-0 flex-1 overflow-y-auto bg-page-canvas"
      >
        <div className="mx-auto max-w-7xl p-6">{children}</div>
      </main>
    </div>
  );
}
