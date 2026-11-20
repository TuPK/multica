import { useState, type ComponentType, type ReactNode } from "react";
import { AppShell } from "@astryxdesign/core/AppShell";
import {
  SideNav,
  SideNavCollapseButton,
  SideNavItem,
  SideNavSection,
} from "@astryxdesign/core/SideNav";
import {
  Activity,
  ArrowUpCircle,
  BarChart3,
  Building2,
  CreditCard,
  FileText,
  Gauge,
  Inbox,
  Layers,
  LayoutGrid,
  Lock,
  LogOut,
  PanelLeftClose,
  PanelLeftOpen,
  Server,
  Shield,
  ShieldCheck,
  SlidersHorizontal,
  Trash2,
  UserPlus,
  Users,
} from "@multica/ui/icons";
import { Button } from "@astryxdesign/core/Button";
import { StatusBadge } from "./StatusBadge";
import type { MeResponse } from "./types";
import { SidebarCollapsedProvider } from "./sidebar-state";
import { HeaderControlsProvider } from "./header-controls";
import { useTheme } from "../theme";
import { LanguageSwitch, ThemeSwitch } from "./PreferenceControls";
import { useLanguage } from "../i18n";
import { t } from "../i18n";

interface LayoutProps {
  currentScreen: string;
  onNavigate: (screen: string) => void;
  user: MeResponse;
  onLogout: () => void;
  workspaceFromTenant?: boolean;
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
  workspaceFromTenant = false,
  children,
}: LayoutProps) {
  const [isCollapsed, setIsCollapsed] = useState(false);
  const theme = useTheme();
  // Re-render this shell when the language changes so its own labels follow.
  useLanguage();

  const isSystemAdmin = user.role === "system_admin";
  const canSeeOverview = user.role !== "admin";
  const canSeeResources = isSystemAdmin;
  const canSeeRuntimes =
    isSystemAdmin || user.role === "admin" || user.role === "support";
  const canSeePlans = isSystemAdmin || user.role === "admin";
  const canSeeQuotaRequests =
    isSystemAdmin || user.role === "admin" || user.role === "billing";
  const canSeePolicy =
    isSystemAdmin || user.role === "admin" || user.role === "support";
  const canSeeApprovals = isSystemAdmin;
  const canSeeLegalHolds = isSystemAdmin;
  const canSeeDeletionRequests = isSystemAdmin || user.role === "admin";
  const canSeeOrders = isSystemAdmin || user.role === "admin" || user.role === "billing";
  const canSeePayments = isSystemAdmin || user.role === "billing";

  const commercialItems: NavItem[] = [
    ...(canSeeOrders ? [{ id: "orders", label: t("Orders"), icon: CreditCard }] : []),
    ...(canSeePayments
      ? [
          { id: "payment-notices", label: t("Payment notices"), icon: Inbox },
          { id: "einvoices", label: t("E-invoices"), icon: FileText },
        ]
      : []),
    ...(isSystemAdmin ? [{ id: "accounts", label: t("Accounts"), icon: UserPlus }] : []),
    ...(canSeePlans ? [{ id: "plans", label: t("Plans"), icon: Layers }] : []),
    ...(canSeeQuotaRequests
      ? [{ id: "quota-requests", label: t("Quota requests"), icon: ArrowUpCircle }]
      : []),
  ];

  const navGroups: NavGroup[] = [
    {
      label: t("Operations"),
      items: [
        ...(canSeeOverview ? [{ id: "overview", label: t("Overview"), icon: BarChart3 }] : []),
        { id: "tenants", label: t("Tenants"), icon: Building2 },
        { id: "workspaces", label: t("Workspaces"), icon: LayoutGrid },
        { id: "users", label: t("Users"), icon: Users },
        ...(canSeeResources ? [{ id: "resources", label: t("Resources"), icon: Gauge }] : []),
        ...(canSeeRuntimes ? [{ id: "runtimes", label: t("Runtimes"), icon: Server }] : []),
      ],
    },
    ...(commercialItems.length > 0
      ? [{ label: t("Commercial"), items: commercialItems }]
      : []),
    {
      label: t("Administration"),
      items: [
        ...(isSystemAdmin ? [{ id: "staff", label: t("Staff"), icon: Shield }] : []),
        ...(canSeePolicy
          ? [{ id: "tenant-policy", label: t("Policy"), icon: SlidersHorizontal }]
          : []),
        ...(canSeeApprovals ? [{ id: "approvals", label: t("Approvals"), icon: ShieldCheck }] : []),
        ...(canSeeLegalHolds ? [{ id: "legal-holds", label: t("Legal holds"), icon: Lock }] : []),
        ...(canSeeDeletionRequests
          ? [{ id: "deletion-requests", label: t("Deletion requests"), icon: Trash2 }]
          : []),
        { id: "audit", label: t("Audit Log"), icon: Activity },
      ],
    },
  ];

  const headerControls = (
    <>
      <LanguageSwitch />
      <ThemeSwitch />
      <span className="min-w-0 truncate text-label text-muted-foreground" title={user.email}>
        {user.email}
      </span>
      <StatusBadge status={user.role} />
      <Button
        type="button"
        label={t("Sign out")}
        icon={<LogOut className="size-4" aria-hidden="true" />}
        isIconOnly
        tooltip={t("Sign out")}
        variant="ghost"
        size="sm"
        onClick={onLogout}
        className="shrink-0"
      />
    </>
  );

  return (
    <SidebarCollapsedProvider value={isCollapsed}>
    <AppShell
      variant="section"
      contentPadding={0}
      sideNav={
        <SideNav
          aria-label={t("Main Navigation")}
          data-state={isCollapsed ? "collapsed" : "expanded"}
          collapsible={{
            isCollapsed,
            onCollapsedChange: setIsCollapsed,
            hasButton: false,
          }}
          header={
            isCollapsed ? (
              // The collapsed rail is too narrow for the wordmark, so show the symbol only.
              <div className="flex h-8 w-full items-center justify-center overflow-visible">
                <img
                  src={theme === "light" ? "/brand/delegora-symbol-on-light.svg" : "/brand/delegora-symbol-on-dark.svg"}
                  alt={t("Delegora")}
                  className="size-6 shrink-0"
                />
              </div>
            ) : (
              <div className="flex min-w-0 items-center gap-2 px-1">
                <img
                  src={theme === "light" ? "/brand/delegora-logo-on-light.svg" : "/brand/delegora-logo-on-dark.svg"}
                  alt={t("Delegora")}
                  className="h-6 w-auto shrink-0"
                />
                <span className="truncate whitespace-nowrap text-caption text-muted-foreground">
                  {t("Platform Console")}
                </span>
              </div>
            )
          }
          footerIcons={
            <>
              <SideNavCollapseButton
                label={isCollapsed ? t("Open navigation") : t("Collapse navigation")}
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
                  label={t("Sign out")}
                  icon={<LogOut className="size-4" aria-hidden="true" />}
                  isIconOnly
                  variant="ghost"
                  size="sm"
                  onClick={onLogout}
                />
              )}
            </>
          }
        >
          {navGroups.map((group) => (
            <SideNavSection key={t(group.label)} title={t(group.label)}>
              {group.items.map((item) => {
                const Icon = item.icon;
                const isActive =
                  currentScreen === item.id ||
                  (item.id === "tenants" &&
                    (currentScreen === "tenant-detail" ||
                      (currentScreen === "workspace-detail" && workspaceFromTenant))) ||
                  (item.id === "workspaces" &&
                    currentScreen === "workspace-detail" &&
                    !workspaceFromTenant) ||
                  (item.id === "plans" && currentScreen === "plan-history") ||
                  (item.id === "users" && currentScreen === "user-detail") ||
                  (item.id === "runtimes" && currentScreen === "runtime-detail");
                return (
                  <SideNavItem
                    key={item.id}
                    label={t(item.label)}
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
        <HeaderControlsProvider value={headerControls}>{children}</HeaderControlsProvider>
      </div>
    </AppShell>
    </SidebarCollapsedProvider>
  );
}
