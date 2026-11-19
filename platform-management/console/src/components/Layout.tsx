import { useState, type ComponentType, type ReactNode } from "react";
import { AppShell } from "@astryxdesign/core/AppShell";
import {
  SideNav,
  SideNavCollapseButton,
  SideNavHeading,
  SideNavItem,
  SideNavSection,
} from "@astryxdesign/core/SideNav";
import {
  Activity,
  BarChart3,
  Building2,
  CreditCard,
  FileText,
  Inbox,
  Layers,
  LogOut,
  PanelLeftClose,
  PanelLeftOpen,
  Shield,
  Trash2,
  UserPlus,
  Users,
} from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
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
  const [isCollapsed, setIsCollapsed] = useState(false);

  const isSystemAdmin = user.role === "system_admin";
  const canSeeOverview = user.role !== "admin";
  const canSeeDeletionRequests = isSystemAdmin || user.role === "admin";
  const canSeeOrders = isSystemAdmin || user.role === "admin" || user.role === "billing";
  const canSeePayments = isSystemAdmin || user.role === "billing";

  const commercialItems: NavItem[] = [
    ...(canSeeOrders ? [{ id: "orders", label: "Orders", icon: CreditCard }] : []),
    ...(canSeePayments
      ? [
          { id: "payment-notices", label: "Payment notices", icon: Inbox },
          { id: "einvoices", label: "E-invoices", icon: FileText },
        ]
      : []),
    ...(isSystemAdmin
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
        ...(canSeeOverview ? [{ id: "overview", label: "Overview", icon: BarChart3 }] : []),
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
        ...(isSystemAdmin ? [{ id: "staff", label: "Staff", icon: Shield }] : []),
        ...(canSeeDeletionRequests
          ? [{ id: "deletion-requests", label: "Deletion requests", icon: Trash2 }]
          : []),
        { id: "audit", label: "Audit Log", icon: Activity },
      ],
    },
  ];

  return (
    <AppShell
      variant="section"
      contentPadding={0}
      sideNav={
        <SideNav
          aria-label="Main Navigation"
          data-state={isCollapsed ? "collapsed" : "expanded"}
          collapsible={{
            isCollapsed,
            onCollapsedChange: setIsCollapsed,
            hasButton: false,
          }}
          header={
            <SideNavHeading
              heading="Platform Console"
              icon={<Shield className="size-4 shrink-0" aria-hidden="true" />}
            />
          }
          footer={
            isCollapsed ? undefined : (
              <div className="space-y-2.5 border-t border-surface-border pt-3">
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
            )
          }
          footerIcons={
            <>
              <SideNavCollapseButton
                label={isCollapsed ? "Open navigation" : "Collapse navigation"}
                aria-expanded={!isCollapsed}
              >
                {isCollapsed ? (
                  <PanelLeftOpen className="size-4" aria-hidden="true" />
                ) : (
                  <PanelLeftClose className="size-4" aria-hidden="true" />
                )}
              </SideNavCollapseButton>
              {isCollapsed && (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  onClick={onLogout}
                  aria-label="Sign out"
                  className="text-muted-foreground hover:text-foreground"
                >
                  <LogOut className="size-4" aria-hidden="true" />
                </Button>
              )}
            </>
          }
        >
          {navGroups.map((group) => (
            <SideNavSection key={group.label} title={group.label}>
              {group.items.map((item) => {
                const Icon = item.icon;
                const isActive =
                  currentScreen === item.id ||
                  (item.id === "tenants" && (currentScreen === "tenant-detail" || currentScreen === "workspace-detail")) ||
                  (item.id === "users" && currentScreen === "user-detail");
                return (
                  <SideNavItem
                    key={item.id}
                    label={item.label}
                    icon={<Icon className="size-4 shrink-0" aria-hidden="true" />}
                    isSelected={isActive}
                    onClick={() => onNavigate(item.id)}
                  />
                );
              })}
            </SideNavSection>
          ))}
        </SideNav>
      }
    >
      <div className="mx-auto w-full max-w-7xl min-w-0 px-4 py-5 sm:p-6">
        {children}
      </div>
    </AppShell>
  );
}
