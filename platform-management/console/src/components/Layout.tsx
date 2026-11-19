import type { ComponentType, ReactNode } from "react";
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
  PanelLeftClose,
  PanelLeftOpen,
  Shield,
  UserPlus,
  Users,
  X,
} from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  useSidebar,
} from "@multica/ui/components/ui/sidebar";
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

function ConsoleShell({
  currentScreen,
  onNavigate,
  user,
  onLogout,
  navGroups,
  children,
}: LayoutProps & { navGroups: NavGroup[] }) {
  const { isCompact, open, openMobile, setOpenMobile, toggleSidebar } = useSidebar();

  const handleSelectNav = (id: string) => {
    if (isCompact) {
      setOpenMobile(false);
    }
    onNavigate(id);
  };

  return (
    <>
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-surface focus:px-3 focus:py-1.5 focus:text-body focus:font-medium focus:shadow-[var(--menu-shadow)]"
      >
        Skip to content
      </a>

      <Sidebar side="left" variant="sidebar" collapsible="offcanvas">
        <SidebarHeader className="h-14 flex-row items-center justify-between gap-2 border-b border-sidebar-border px-4 py-0">
          <div className="flex min-w-0 items-center gap-2 font-heading text-title-sm font-semibold">
            <Shield className="size-4 shrink-0 text-primary" aria-hidden="true" />
            <span className="truncate">Platform Console</span>
          </div>
          {!isCompact && (
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={toggleSidebar}
              aria-label="Collapse navigation"
              aria-expanded={open}
              className="shrink-0 text-muted-foreground hover:text-foreground"
            >
              <PanelLeftClose className="size-4" aria-hidden="true" />
            </Button>
          )}
        </SidebarHeader>

        <SidebarContent>
          <nav aria-label="Main Navigation" className="flex flex-col gap-1 py-2">
            {navGroups.map((group) => (
              <SidebarGroup key={group.label} className="px-3 py-1.5">
                <SidebarGroupLabel className="h-6 px-2 text-micro font-semibold uppercase tracking-wider text-muted-foreground">
                  {group.label}
                </SidebarGroupLabel>
                <SidebarGroupContent>
                  <SidebarMenu className="gap-0.5">
                    {group.items.map((item) => {
                      const Icon = item.icon;
                      const isActive =
                        currentScreen === item.id ||
                        (item.id === "tenants" && currentScreen === "tenant-detail") ||
                        (item.id === "users" && currentScreen === "user-detail");
                      return (
                        <SidebarMenuItem key={item.id}>
                          <SidebarMenuButton
                            type="button"
                            isActive={isActive}
                            aria-current={isActive ? "page" : undefined}
                            onClick={() => handleSelectNav(item.id)}
                          >
                            <Icon className="size-4 shrink-0" aria-hidden="true" />
                            <span>{item.label}</span>
                          </SidebarMenuButton>
                        </SidebarMenuItem>
                      );
                    })}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            ))}
          </nav>
        </SidebarContent>

        <SidebarFooter className="gap-2.5 border-t border-sidebar-border p-3">
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
        </SidebarFooter>
      </Sidebar>

      <SidebarInset id="main-content" className="min-w-0">
        {isCompact ? (
          <div className="sticky top-0 z-20 flex h-14 items-center justify-between border-b border-surface-border bg-sidebar px-4">
            <div className="flex min-w-0 items-center gap-2 font-heading text-title-sm font-semibold text-sidebar-foreground">
              <Shield className="size-4 shrink-0 text-primary" aria-hidden="true" />
              <span className="truncate">Platform Console</span>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={toggleSidebar}
              aria-label={openMobile ? "Close navigation" : "Open navigation"}
              aria-expanded={openMobile}
            >
              {openMobile ? (
                <X className="size-4" aria-hidden="true" />
              ) : (
                <Menu className="size-4" aria-hidden="true" />
              )}
            </Button>
          </div>
        ) : !open ? (
          <div className="sticky top-0 z-20 flex h-12 items-center gap-2 border-b border-surface-border bg-surface px-4">
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={toggleSidebar}
              aria-label="Open navigation"
              aria-expanded={false}
            >
              <PanelLeftOpen className="size-4" aria-hidden="true" />
            </Button>
            <div className="flex min-w-0 items-center gap-2 font-heading text-body font-semibold text-foreground">
              <Shield className="size-4 shrink-0 text-primary" aria-hidden="true" />
              <span className="truncate">Platform Console</span>
            </div>
          </div>
        ) : null}

        <div className="mx-auto w-full max-w-7xl min-w-0 px-4 py-5 sm:p-6">
          {children}
        </div>
      </SidebarInset>
    </>
  );
}

export function Layout({
  currentScreen,
  onNavigate,
  user,
  onLogout,
  children,
}: LayoutProps) {
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

  return (
    <SidebarProvider
      defaultOpen
      className="bg-app-shell text-foreground [--sidebar-wrapper-fill:var(--app-shell)]"
    >
      <ConsoleShell
        currentScreen={currentScreen}
        onNavigate={onNavigate}
        user={user}
        onLogout={onLogout}
        navGroups={navGroups}
      >
        {children}
      </ConsoleShell>
    </SidebarProvider>
  );
}
