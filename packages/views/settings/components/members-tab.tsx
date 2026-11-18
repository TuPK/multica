"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  Check,
  Copy,
  Crown,
  Eye,
  Link,
  Loader2,
  Lock,
  Mail,
  MoreHorizontal,
  Plus,
  Search,
  Shield,
  ShieldCheck,
  Trash2,
  User,
  UserMinus,
  Users,
  X,
} from "@multica/ui/icons";
import { ActorAvatar } from "../../common/actor-avatar";
import { AppLink, useOptionalNavigation } from "../../navigation";
import type {
  Invitation,
  MemberRole,
  MemberWithUser,
  PurchaseWorkspaceSeatsRequest,
  ShareLink,
  WorkspaceSeatPurchasePreview,
} from "@multica/core/types";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Button } from "@multica/ui/components/ui/button";
import { IconButton } from "@multica/ui/components/common/icon-button";
import { Label } from "@multica/ui/components/ui/label";
import { Switch } from "@multica/ui/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Badge } from "@multica/ui/components/ui/badge";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@multica/ui/components/ui/alert-dialog";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@multica/ui/components/ui/select";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
} from "@multica/ui/components/ui/dropdown-menu";
import { TruncatedTooltip } from "@multica/ui/components/common/truncated-tooltip";
import { toast } from "sonner";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@multica/core/auth";
import {
  usePreviewWorkspaceSeatPurchase,
  usePurchaseWorkspaceSeats,
  workspaceSubscriptionSummaryOptions,
} from "@multica/core/billing";
import { useWorkspaceId } from "@multica/core/hooks";
import { useFeatureEnabled } from "@multica/core/config";
import { BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG } from "@multica/core/feature-flags";
import { useCurrentWorkspace } from "@multica/core/paths";
import {
  invitationListOptions,
  memberListOptions,
  shareLinkListOptions,
  workspaceKeys,
} from "@multica/core/workspace/queries";
import { projectListOptions } from "@multica/core/projects/queries";
import {
  memberGroupDetailOptions,
  memberGroupListOptions,
  myPermissionsOptions,
  useAddMemberGroupMembers,
  useRemoveMemberGroupMember,
  type MemberGroup,
} from "@multica/core/groups";
import { useCan } from "@multica/core/permissions";
import { api, errorCode } from "@multica/core/api";
import { useLocale, useT } from "../../i18n";
import { ObjectSectionEmpty } from "../../layout/object-page";
import {
  SettingsCard,
  SettingsReadOnlyNotice,
  SettingsTab,
  SettingsViewTabs,
} from "./settings-layout";
import { resolveSettingsLocation, settingsHref } from "./settings-navigation";
import { formatStripeMinorAmount } from "./billing-format";
import {
  isSingleSeatInvitePreview,
  purchasedSeatIsReadyForInvitation,
  seatInvitationCanRetryAfterPurchase,
  seatInvitationCapacityFailure,
  seatPurchaseCanRetryWithSameQuote,
  seatPurchaseMatchesPreview,
} from "./seat-invite-purchase";

const SEAT_PURCHASE_CONFIRM_TIMEOUT_MS = 2 * 60_000;

type MembersView = "members" | "invitations" | "links";

type InviteSeatPurchase = {
  workspaceId: string;
  email: string;
  role: MemberRole;
  isGuest?: boolean;
  guestProjectIds?: string[];
  preview: WorkspaceSeatPurchasePreview;
  idempotencyKey: string;
  phase: "review" | "purchasing" | "waiting" | "inviting" | "error";
  submittedAt?: number;
  error?: string;
  retryable?: boolean;
};

function createSeatPurchaseKey(workspaceId: string): string {
  const suffix =
    globalThis.crypto?.randomUUID?.() ??
    `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  return `invite-seat-${workspaceId}-${suffix}`.slice(0, 200);
}

const ROLE_ICONS: Record<MemberRole, typeof Crown> = {
  owner: Crown,
  admin: Shield,
  member: User,
  viewer: Eye,
};

// Builds the shareable URL for a share-link invite. Prefers the navigation
// adapter's getShareableUrl (works on desktop where window.location.origin is
// not the public web origin), falling back to the browser origin on web.
function buildShareLinkUrl(
  navigation: ReturnType<typeof useOptionalNavigation>,
  code: string,
): string {
  const joinPath = `/join?code=${code}`;
  if (navigation?.getShareableUrl) {
    return navigation.getShareableUrl(joinPath);
  }
  return `${typeof window !== "undefined" ? window.location.origin : ""}${joinPath}`;
}

function useRoleLabels() {
  const { t } = useT("settings");
  return {
    owner: {
      label: t(($) => $.members.roles.owner.label),
      description: t(($) => $.members.roles.owner.description),
      icon: ROLE_ICONS.owner,
    },
    admin: {
      label: t(($) => $.members.roles.admin.label),
      description: t(($) => $.members.roles.admin.description),
      icon: ROLE_ICONS.admin,
    },
    member: {
      label: t(($) => $.members.roles.member.label),
      description: t(($) => $.members.roles.member.description),
      icon: ROLE_ICONS.member,
    },
    viewer: {
      label: t(($) => $.members.roles.viewer.label),
      description: t(($) => $.members.roles.viewer.description),
      icon: ROLE_ICONS.viewer,
    },
  } as const;
}

function MemberRow({
  member,
  canManage,
  canManageOwners,
  ownerCount,
  isSelf,
  busy,
  onManageGroups,
  onGuestToggle,
  onRoleChange,
  onRemove,
}: {
  member: MemberWithUser;
  canManage: boolean;
  canManageOwners: boolean;
  /** Total number of owners in this workspace — needed to gate demoting the
   *  last owner per `workspace.go:497-507`. */
  ownerCount: number;
  isSelf: boolean;
  busy: boolean;
  onManageGroups: () => void;
  onGuestToggle: (isGuest: boolean) => void;
  onRoleChange: (role: MemberRole) => void;
  onRemove: () => void;
}) {
  const { t } = useT("settings");
  const locale = useLocale();
  const roleConfig = useRoleLabels();
  const rc = roleConfig[member.role];
  const RoleIcon = rc.icon;
  const memberIsGuest = "is_guest" in member && Boolean(member.is_guest);
  const canEditRole = canManage && !isSelf && (member.role !== "owner" || canManageOwners);
  const canRemove = canManage && !isSelf && (member.role !== "owner" || canManageOwners);
  const canAssignGroups = canManage && !isSelf;
  const canToggleGuest =
    canManage && !isSelf && member.role !== "owner" && member.role !== "admin";
  const isLastOwner = member.role === "owner" && ownerCount <= 1;
  const showMenu = canEditRole || canRemove;

  return (
    <div className="flex items-center gap-3 px-4 py-3">
      <ActorAvatar actorType="member" actorId={member.user_id} size="lg" />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-body font-medium">{member.name}</span>
          {isSelf ? (
            <Badge variant="secondary" className="shrink-0">
              {t(($) => $.members.you)}
            </Badge>
          ) : null}
          {memberIsGuest ? (
            <Badge variant="outline" className="shrink-0">
              {t(($) => $.members.guest_badge)}
            </Badge>
          ) : null}
        </div>
        <div className="truncate text-caption text-muted-foreground">{member.email}</div>
      </div>
      <span className="hidden w-32 shrink-0 text-caption tabular-nums text-muted-foreground md:block">
        {t(($) => $.members.joined, {
          date: new Date(member.created_at).toLocaleDateString(locale),
        })}
      </span>
      <span className="flex w-24 shrink-0 items-center gap-1.5 text-body">
        <RoleIcon className="size-3.5 text-muted-foreground" aria-hidden="true" />
        {rc.label}
      </span>
      <span className="flex shrink-0 items-center justify-end gap-1">
        {canToggleGuest ? (
          <div className="flex shrink-0 items-center gap-1.5 pr-1">
            <span className="text-caption text-muted-foreground">
              {t(($) => $.members.guest_toggle_label)}
            </span>
            <Switch
              size="sm"
              checked={memberIsGuest}
              disabled={busy}
              aria-label={t(($) => $.members.guest_toggle_aria, { name: member.name })}
              onCheckedChange={(checked) => onGuestToggle(checked)}
            />
          </div>
        ) : null}
        {canAssignGroups ? (
          <IconButton
            variant="ghost"
            size="icon-sm"
            disabled={busy}
            onClick={onManageGroups}
            label={t(($) => $.members.groups_for_aria, { name: member.name })}
          >
            <ShieldCheck className="h-4 w-4 text-muted-foreground" />
          </IconButton>
        ) : null}
        <span className="flex size-7 shrink-0 items-center justify-center">
        {showMenu && (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <IconButton
                variant="ghost"
                size="icon-sm"
                disabled={busy}
                label={t(($) => $.members.member_actions, { name: member.name })}
              >
                <MoreHorizontal className="h-4 w-4 text-muted-foreground" />
              </IconButton>
            }
          />
          <DropdownMenuContent align="end" className="w-auto">
            {canEditRole && (
              <DropdownMenuSub>
                <DropdownMenuSubTrigger>
                  <Shield className="h-3.5 w-3.5" />
                  {t(($) => $.members.change_role)}
                </DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="w-auto">
                  {(Object.entries(roleConfig) as [MemberRole, (typeof roleConfig)[MemberRole]][]).map(
                    ([role, config]) => {
                      if (role === "owner" && !canManageOwners) return null;
                      const Icon = config.icon;
                      const wouldDemoteLastOwner =
                        isLastOwner && role !== "owner";
                      return (
                        <DropdownMenuItem
                          key={role}
                          onClick={() =>
                            wouldDemoteLastOwner ? undefined : onRoleChange(role)
                          }
                          disabled={wouldDemoteLastOwner}
                          title={
                            wouldDemoteLastOwner
                              ? t(($) => $.members.cannot_demote_last_owner_title)
                              : undefined
                          }
                        >
                          <Icon className="h-3.5 w-3.5" />
                          <div className="flex flex-col">
                            <span>{config.label}</span>
                            <span className="text-caption text-muted-foreground font-normal">
                              {wouldDemoteLastOwner
                                ? t(($) => $.members.cannot_demote_last_owner)
                                : config.description}
                            </span>
                          </div>
                          {member.role === role && (
                            <Check className="ml-auto size-3.5 text-muted-foreground" />
                          )}
                        </DropdownMenuItem>
                      );
                    }
                  )}
                </DropdownMenuSubContent>
              </DropdownMenuSub>
            )}
            {canEditRole && canRemove && <DropdownMenuSeparator />}
            {canRemove && (
              <DropdownMenuItem variant="destructive" onClick={onRemove}>
                <UserMinus className="h-3.5 w-3.5" />
                {t(($) => $.members.remove_action)}
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
        </span>
      </span>
    </div>
  );
}

function InvitationRow({
  invitation,
  canManage,
  onRevoke,
  busy,
}: {
  invitation: Invitation;
  canManage: boolean;
  onRevoke: () => void;
  busy: boolean;
}) {
  const { t } = useT("settings");
  const locale = useLocale();
  const roleConfig = useRoleLabels();
  const rc = roleConfig[invitation.role];
  const invitationIsGuest =
    "is_guest" in invitation && Boolean(invitation.is_guest);

  return (
    <div className="flex items-center gap-3 px-4 py-3">
      <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted">
        <Mail className="size-4 text-muted-foreground" aria-hidden="true" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-body font-medium">{invitation.invitee_email}</span>
          {invitationIsGuest ? (
            <Badge variant="outline" className="shrink-0">
              {t(($) => $.members.guest_badge)}
            </Badge>
          ) : null}
        </div>
        <div className="truncate text-caption text-muted-foreground">
          {t(($) => $.members.invitation_meta, {
            sent: new Date(invitation.created_at).toLocaleDateString(locale),
            expires: new Date(invitation.expires_at).toLocaleDateString(locale),
          })}
        </div>
      </div>
      <span className="w-24 shrink-0 text-body">{rc.label}</span>
      <span className="flex size-7 shrink-0 items-center justify-center">
        {canManage && (
          <IconButton
            variant="ghost"
            size="icon-sm"
            disabled={busy}
            onClick={onRevoke}
            label={t(($) => $.members.revoke_invitation_aria, {
              email: invitation.invitee_email,
            })}
          >
            <X className="size-4 text-muted-foreground" />
          </IconButton>
        )}
      </span>
    </div>
  );
}

function ShareLinkRow({
  link,
  onRevoke,
  busy,
  onCopy,
}: {
  link: ShareLink;
  onRevoke: () => void;
  busy: boolean;
  onCopy: () => void;
}) {
  const { t } = useT("settings");
  const locale = useLocale();
  const roleConfig = useRoleLabels();
  const rc = roleConfig[link.role];
  const navigation = useOptionalNavigation();
  const joinUrl = buildShareLinkUrl(navigation, link.code);

  return (
    <div className="flex items-center gap-3 px-4 py-3">
      <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted">
        <Link className="size-4 text-muted-foreground" aria-hidden="true" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-1 text-body font-medium">
          <span>{t(($) => $.members.share_link_uses, { used: link.use_count, max: link.max_uses ?? "∞" })}</span>
          {link.expires_at && <span>· {t(($) => $.members.share_link_expires, { date: new Date(link.expires_at).toLocaleDateString(locale) })}</span>}
        </div>
        <TruncatedTooltip content={joinUrl}>
          <div
            className="truncate font-mono text-caption text-muted-foreground"
          >
            {joinUrl}
          </div>
        </TruncatedTooltip>
      </div>
      <span className="w-24 shrink-0 text-body">{rc.label}</span>
      <IconButton
        variant="ghost"
        size="icon-sm"
        onClick={onCopy}
        label={t(($) => $.members.share_link_copy_tooltip)}
      >
        <Copy className="size-4 text-muted-foreground" />
      </IconButton>
      <IconButton
        variant="ghost"
        size="icon-sm"
        disabled={busy}
        onClick={onRevoke}
        label={t(($) => $.members.share_link_revoke_tooltip)}
      >
        <Trash2 className="size-4 text-muted-foreground" />
      </IconButton>
    </div>
  );
}

export function MembersTab() {
  const { t } = useT("settings");
  const { t: billingT } = useT("billing");
  const locale = useLocale();
  const roleConfig = useRoleLabels();
  const user = useAuthStore((s) => s.user);
  const workspace = useCurrentWorkspace();
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  const navigation = useOptionalNavigation();
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const currentMember = members.find((m) => m.user_id === user?.id) ?? null;
  const { data: myPermissions } = useQuery({
    ...myPermissionsOptions(wsId),
    enabled: Boolean(wsId),
  });
  const canInviteKey = useCan(wsId, "members.invite", { write: true }).allowed;
  const isCurrentGuest =
    (currentMember !== null &&
      "is_guest" in currentMember &&
      Boolean(currentMember.is_guest)) ||
    Boolean(
      myPermissions && "is_guest" in myPermissions && myPermissions.is_guest,
    );
  const canInvite = !isCurrentGuest && canInviteKey;
  const effectiveRole = myPermissions?.role || currentMember?.role;
  const canManageWorkspace =
    !isCurrentGuest &&
    (effectiveRole === "owner" || effectiveRole === "admin");
  // Only owners/admins or holders of members.invite may list pending invitations
  // (avoids exposing invitee emails to plain members; server returns 403).
  const { data: invitations = [] } = useQuery(
    invitationListOptions(wsId, canManageWorkspace || canInvite),
  );

  const [inviteOpen, setInviteOpen] = useState(false);
  const [shareLinkOpen, setShareLinkOpen] = useState(false);
  const [groupsMember, setGroupsMember] = useState<MemberWithUser | null>(null);
  const requestedView = resolveSettingsLocation(
    navigation?.searchParams ?? new URLSearchParams(),
  ).section;
  const urlView: MembersView =
    requestedView === "invitations" || requestedView === "links"
      ? requestedView
      : "members";
  const [localView, setLocalView] = useState<MembersView>(urlView);
  // With a router the URL owns the open list, so search results that land on
  // this page and back/forward switch it too. Without one it is local state.
  const view = navigation ? urlView : localView;
  const setView = (next: MembersView) => {
    if (!navigation) {
      setLocalView(next);
      return;
    }
    navigation.replace(
      settingsHref(navigation.pathname, navigation.searchParams, "members", {
        section: next === "members" ? undefined : next,
      }),
    );
  };
  const [memberQuery, setMemberQuery] = useState("");
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState<MemberRole>("member");
  const [inviteIsGuest, setInviteIsGuest] = useState(false);
  const [inviteGuestProjectIds, setInviteGuestProjectIds] = useState<string[]>([]);
  const [inviteLoading, setInviteLoading] = useState(false);
  const [inviteSeatPurchase, setInviteSeatPurchase] =
    useState<InviteSeatPurchase | null>(null);
  const dispatchedInvitePurchaseKey = useRef<string | null>(null);
  const [memberActionId, setMemberActionId] = useState<string | null>(null);
  const [invitationActionId, setInvitationActionId] = useState<string | null>(null);
  const [shareLinkActionId, setShareLinkActionId] = useState<string | null>(null);
  const [shareLinkLoading, setShareLinkLoading] = useState(false);
  const [shareLinkRole, setShareLinkRole] = useState<MemberRole>("member");
  const [shareLinkExpiry, setShareLinkExpiry] = useState<string>("168"); // default 7 days
  const [confirmAction, setConfirmAction] = useState<{
    title: string;
    description: string;
    variant?: "destructive";
    onConfirm: () => Promise<void>;
  } | null>(null);
  const previewSeatPurchase = usePreviewWorkspaceSeatPurchase();
  const purchaseSeats = usePurchaseWorkspaceSeats(wsId);
  const seatPurchaseSummary = useQuery({
    ...workspaceSubscriptionSummaryOptions(wsId),
    enabled:
      inviteSeatPurchase?.phase === "waiting" &&
      inviteSeatPurchase.workspaceId === wsId,
    staleTime: 0,
    refetchInterval:
      inviteSeatPurchase?.phase === "waiting" ? 2_000 : false,
  });

  // The Team panel's invite button links here with `?invite=1`; it waits for
  // the caller permissions so only invite-authorized users get the dialog.
  const inviteRequested = navigation?.searchParams.has("invite") ?? false;
  useEffect(() => {
    if (!inviteRequested || !navigation || !currentMember) return;
    if (canInvite) setInviteOpen(true);
    const params = new URLSearchParams(navigation.searchParams);
    params.delete("invite");
    navigation.replace(`${navigation.pathname}?${params}`);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- consume the intent once permissions are known
  }, [inviteRequested, canInvite, currentMember?.role]);
  const isOwner = effectiveRole === "owner";
  const ownerCount = members.filter((m) => m.role === "owner").length;
  // Only owners/admins or holders of members.invite may list share links; skip
  // the request for plain members/viewers (server returns 403).
  const { data: shareLinks = [] } = useQuery(
    shareLinkListOptions(wsId, canManageWorkspace || canInvite),
  );
  const billingEnabled = useFeatureEnabled(BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG, false);
  const seats = useQuery({
    ...workspaceSubscriptionSummaryOptions(wsId),
    enabled: billingEnabled && canManageWorkspace,
  }).data?.seatCapacity;
  const needle = memberQuery.trim().toLocaleLowerCase();
  const visibleMembers = needle
    ? members.filter(
        (m) =>
          m.name.toLocaleLowerCase().includes(needle) ||
          m.email.toLocaleLowerCase().includes(needle),
      )
    : members;

  const { data: guestSelectableProjects = [] } = useQuery({
    ...projectListOptions(wsId),
    enabled: inviteOpen && inviteIsGuest && Boolean(wsId),
  });

  const resetInviteForm = useCallback(() => {
    setInviteEmail("");
    setInviteRole("member");
    setInviteIsGuest(false);
    setInviteGuestProjectIds([]);
  }, []);

  const memberErrorMessage = useCallback(
    (err: unknown, fallback: string) => {
      const code = errorCode(err);
      if (code === "guest_forbidden") return t(($) => $.members.error_guest_forbidden);
      if (code === "project_member_invalid") {
        return t(($) => $.members.error_project_member_invalid);
      }
      return err instanceof Error ? err.message : fallback;
    },
    [t],
  );

  const sendInvitation = useCallback(
    async (
      email: string,
      role: MemberRole,
      guestOpts?: { isGuest?: boolean; guestProjectIds?: string[] },
    ) => {
      if (!workspace) return;
      const payload = guestOpts?.isGuest
        ? {
            email,
            role,
            is_guest: true,
            guest_project_ids: guestOpts.guestProjectIds ?? [],
          }
        : { email, role };
      await api.createMember(workspace.id, payload);
      resetInviteForm();
      await qc.invalidateQueries({
        queryKey: workspaceKeys.invitations(wsId),
      });
      toast.success(t(($) => $.members.toast_invitation_sent));
    },
    [qc, resetInviteForm, t, workspace, wsId],
  );

  const effectiveInviteRole: MemberRole =
    (!canManageWorkspace || inviteIsGuest) && inviteRole === "admin"
      ? "member"
      : inviteRole;
  const effectiveShareLinkRole: MemberRole =
    !canManageWorkspace && shareLinkRole === "admin" ? "member" : shareLinkRole;

  const handleInviteMember = async () => {
    if (!workspace || !canInvite) return;
    const email = inviteEmail.trim();
    const role = effectiveInviteRole;
    const guestOpts = inviteIsGuest
      ? { isGuest: true, guestProjectIds: inviteGuestProjectIds }
      : undefined;
    setInviteLoading(true);
    try {
      await sendInvitation(email, role, guestOpts);
      setInviteOpen(false);
    } catch (e) {
      const code = errorCode(e);
      const capacityFailure = seatInvitationCapacityFailure(code);
      if (code === "seat_capacity_overcommitted") {
        try {
          const summary = await qc.fetchQuery({
            ...workspaceSubscriptionSummaryOptions(wsId),
            staleTime: 0,
          });
          const capacity = summary?.seatCapacity;
          toast.error(
            billingT(($) => $.workspace.seats.members_over_capacity_title),
            capacity
              ? {
                  description: billingT(
                    ($) => $.workspace.seats.occupancy_over_capacity_description,
                    {
                      // Cloud refuses on used + reserved, so pending
                      // invitations have to appear here: the most common
                      // trigger is a first ledger snapshot whose member count
                      // alone still fits inside the purchased seats.
                      occupied: capacity.used + capacity.reserved,
                      purchased: capacity.purchased,
                      members: capacity.used,
                      reserved: capacity.reserved,
                    },
                  ),
                }
              : undefined,
          );
        } catch {
          toast.error(
            billingT(($) => $.workspace.seats.members_over_capacity_title),
          );
        }
        return;
      }
      if (capacityFailure === "full") {
        try {
          const preview = await previewSeatPurchase.mutateAsync({
            additionalSeats: 1,
          });
          if (!isSingleSeatInvitePreview(preview)) {
            toast.error(
              billingT(($) => $.workspace.seat_purchase.preview_unreadable),
            );
            return;
          }
          // Hand over to the purchase confirmation rather than stacking a
          // second modal on the invite dialog.
          setInviteOpen(false);
          setInviteSeatPurchase({
            workspaceId: workspace.id,
            email,
            role,
            isGuest: inviteIsGuest || undefined,
            guestProjectIds: inviteIsGuest ? inviteGuestProjectIds : undefined,
            preview,
            idempotencyKey: createSeatPurchaseKey(workspace.id),
            phase: "review",
          });
          dispatchedInvitePurchaseKey.current = null;
        } catch (previewError) {
          toast.error(
            errorCode(previewError) === "seat_purchase_in_progress"
              ? billingT(($) => $.workspace.seat_purchase.in_progress)
              : billingT(($) => $.workspace.seat_purchase.preview_failed),
          );
        }
        return;
      }
      if (capacityFailure === "unavailable") {
        toast.error(t(($) => $.members.toast_seat_capacity_unavailable));
        return;
      }
      if (capacityFailure === "rate_limited") {
        toast.error(t(($) => $.members.toast_seat_capacity_rate_limited));
        return;
      }
      toast.error(
        memberErrorMessage(e, t(($) => $.members.toast_invitation_failed)),
      );
    } finally {
      setInviteLoading(false);
    }
  };

  const handlePurchaseSeatAndInvite = async () => {
    if (
      !inviteSeatPurchase ||
      (inviteSeatPurchase.phase !== "review" &&
        !(inviteSeatPurchase.phase === "error" && inviteSeatPurchase.retryable))
    ) {
      return;
    }
    const current = inviteSeatPurchase;
    if (!workspace || current.workspaceId !== workspace.id) {
      setInviteSeatPurchase(null);
      return;
    }
    // A transient failure may happen after the purchase succeeded and the
    // first invitation dispatch ran. Reusing the same idempotency key is safe,
    // but the dispatch guard must be reopened for the retried invitation.
    dispatchedInvitePurchaseKey.current = null;
    setInviteSeatPurchase({
      ...current,
      phase: "purchasing",
      error: undefined,
      retryable: false,
    });
    const request: PurchaseWorkspaceSeatsRequest = {
      additionalSeats: current.preview.additionalSeats,
      expectedCurrentSeats: current.preview.currentSeats,
      expectedPurchaseVersion: current.preview.purchaseVersion,
      acceptedProrationAmount: current.preview.prorationAmount,
      currency: current.preview.currency,
      idempotencyKey: current.idempotencyKey,
    };
    try {
      const response = await purchaseSeats.mutateAsync(request);
      if (!seatPurchaseMatchesPreview(response, current.preview)) {
        setInviteSeatPurchase({
          ...current,
          phase: "error",
          error: billingT(
            ($) => $.workspace.seat_purchase.purchase_unreadable,
          ),
          retryable: true,
        });
        return;
      }
      const submittedAt = Date.now();
      setInviteSeatPurchase({
        ...current,
        phase: "waiting",
        submittedAt,
        error: undefined,
        retryable: false,
      });
      await qc.invalidateQueries({
        queryKey: workspaceSubscriptionSummaryOptions(wsId).queryKey,
      });
    } catch (error) {
      const code = errorCode(error);
      const message =
        code === "seat_purchase_payment_failed"
          ? billingT(($) => $.workspace.seat_purchase.payment_failed)
          : code === "seat_purchase_in_progress"
            ? billingT(($) => $.workspace.seat_purchase.in_progress)
            : code === "seat_quote_changed" || code === "seat_capacity_changed"
              ? billingT(($) => $.workspace.seat_purchase.quote_changed)
              : billingT(($) => $.workspace.seat_purchase.purchase_failed);
      setInviteSeatPurchase({
        ...current,
        phase: "error",
        error: message,
        retryable: seatPurchaseCanRetryWithSameQuote(code),
      });
    }
  };

  useEffect(() => {
    setInviteSeatPurchase((current) =>
      current && current.workspaceId !== wsId ? null : current,
    );
  }, [wsId]);

  useEffect(() => {
    const purchase = inviteSeatPurchase;
    if (
      purchase?.phase !== "waiting" ||
      purchase.workspaceId !== wsId ||
      purchase.submittedAt == null ||
      dispatchedInvitePurchaseKey.current === purchase.idempotencyKey ||
      !purchasedSeatIsReadyForInvitation(
        seatPurchaseSummary.data,
        purchase.preview,
        purchase.submittedAt,
        seatPurchaseSummary.dataUpdatedAt,
      )
    ) {
      return;
    }

    dispatchedInvitePurchaseKey.current = purchase.idempotencyKey;
    setInviteSeatPurchase({ ...purchase, phase: "inviting" });
    void sendInvitation(
      purchase.email,
      purchase.role,
      purchase.isGuest
        ? { isGuest: true, guestProjectIds: purchase.guestProjectIds }
        : undefined,
    )
      .then(() => setInviteSeatPurchase(null))
      .catch((error) => {
        const code = errorCode(error);
        const capacityFailure = seatInvitationCapacityFailure(code);
        let message: string;
        switch (capacityFailure) {
          case "full":
            message = t(($) => $.members.seat_purchase_capacity_taken);
            break;
          case "unavailable":
            message = t(($) => $.members.toast_seat_capacity_unavailable);
            break;
          case "rate_limited":
            message = t(($) => $.members.toast_seat_capacity_rate_limited);
            break;
          default:
            message =
              error instanceof Error
                ? error.message
                : t(($) => $.members.toast_invitation_failed);
        }
        setInviteSeatPurchase({
          ...purchase,
          phase: "error",
          retryable: seatInvitationCanRetryAfterPurchase(code),
          error: message,
        });
      });
  }, [
    inviteSeatPurchase,
    seatPurchaseSummary.data,
    seatPurchaseSummary.dataUpdatedAt,
    sendInvitation,
    t,
    wsId,
  ]);

  useEffect(() => {
    if (
      inviteSeatPurchase?.phase !== "waiting" ||
      inviteSeatPurchase.submittedAt == null
    ) {
      return;
    }
    const elapsed = Date.now() - inviteSeatPurchase.submittedAt;
    const timeout = window.setTimeout(() => {
      setInviteSeatPurchase((current) =>
        current?.phase === "waiting"
          ? {
              ...current,
              phase: "error",
              error: t(($) => $.members.seat_purchase_timeout),
              retryable: false,
            }
          : current,
      );
    }, Math.max(0, SEAT_PURCHASE_CONFIRM_TIMEOUT_MS - elapsed));
    return () => window.clearTimeout(timeout);
  }, [inviteSeatPurchase?.phase, inviteSeatPurchase?.submittedAt, t]);

  const handleRevokeInvitation = (invitation: Invitation) => {
    if (!workspace) return;
    setConfirmAction({
      title: t(($) => $.members.revoke_invitation_title),
      description: t(($) => $.members.revoke_invitation_description, { email: invitation.invitee_email }),
      variant: "destructive",
      onConfirm: async () => {
        setInvitationActionId(invitation.id);
        try {
          await api.revokeInvitation(workspace.id, invitation.id);
          qc.invalidateQueries({ queryKey: workspaceKeys.invitations(wsId) });
          toast.success(t(($) => $.members.toast_invitation_revoked));
        } catch (e) {
          toast.error(e instanceof Error ? e.message : t(($) => $.members.toast_invitation_revoke_failed));
        } finally {
          setInvitationActionId(null);
        }
      },
    });
  };

  const handleRoleChange = async (memberId: string, role: MemberRole) => {
    if (!workspace) return;
    setMemberActionId(memberId);
    try {
      await api.updateMember(workspace.id, memberId, { role });
      qc.invalidateQueries({ queryKey: workspaceKeys.members(wsId) });
      toast.success(t(($) => $.members.toast_role_updated));
    } catch (e) {
      toast.error(memberErrorMessage(e, t(($) => $.members.toast_role_failed)));
    } finally {
      setMemberActionId(null);
    }
  };

  const handleGuestToggle = async (member: MemberWithUser, isGuest: boolean) => {
    if (!workspace) return;
    setMemberActionId(member.id);
    try {
      await api.updateMember(workspace.id, member.id, {
        role: member.role,
        is_guest: isGuest,
      });
      qc.invalidateQueries({ queryKey: workspaceKeys.members(wsId) });
      toast.success(t(($) => $.members.toast_guest_updated));
    } catch (e) {
      toast.error(memberErrorMessage(e, t(($) => $.members.toast_role_failed)));
    } finally {
      setMemberActionId(null);
    }
  };

  const handleRemoveMember = (member: MemberWithUser) => {
    if (!workspace) return;
    setConfirmAction({
      title: t(($) => $.members.remove_member_title, { name: member.name }),
      description: t(($) => $.members.remove_member_description, { name: member.name, workspace: workspace.name }),
      variant: "destructive",
      onConfirm: async () => {
        setMemberActionId(member.id);
        try {
          await api.deleteMember(workspace.id, member.id);
          qc.invalidateQueries({ queryKey: workspaceKeys.members(wsId) });
          toast.success(t(($) => $.members.toast_member_removed));
        } catch (e) {
          toast.error(e instanceof Error ? e.message : t(($) => $.members.toast_member_remove_failed));
        } finally {
          setMemberActionId(null);
        }
      },
    });
  };

  const handleCreateShareLink = async () => {
    if (!workspace || !canInvite) return;
    setShareLinkLoading(true);
    try {
      await api.createShareLink(workspace.id, { role: effectiveShareLinkRole, expires_in: parseInt(shareLinkExpiry) || undefined });
      qc.invalidateQueries({ queryKey: workspaceKeys.shareLinks(wsId) });
      setShareLinkOpen(false);
      toast.success(t(($) => $.members.toast_share_link_created));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.members.toast_share_link_failed));
    } finally {
      setShareLinkLoading(false);
    }
  };

  const handleRevokeShareLink = (link: ShareLink) => {
    if (!workspace) return;
    setShareLinkActionId(link.id);
    api.revokeShareLink(workspace.id, link.id)
      .then(() => {
        qc.invalidateQueries({ queryKey: workspaceKeys.shareLinks(wsId) });
        toast.success(t(($) => $.members.toast_share_link_revoked));
      })
      .catch((e) => {
        toast.error(e instanceof Error ? e.message : t(($) => $.members.toast_share_link_revoke_failed));
      })
      .finally(() => setShareLinkActionId(null));
  };

  const handleCopyShareLink = (link: ShareLink) => {
    const joinUrl = buildShareLinkUrl(navigation, link.code);
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(joinUrl).then(
        () => toast.success(t(($) => $.members.toast_share_link_copied)),
        () => toast.error(t(($) => $.members.toast_share_link_copy_failed)),
      );
    } else {
      const textArea = document.createElement("textarea");
      textArea.value = joinUrl;
      textArea.style.position = "fixed";
      textArea.style.left = "-9999px";
      document.body.appendChild(textArea);
      textArea.select();
      try {
        document.execCommand("copy");
        toast.success(t(($) => $.members.toast_share_link_copied));
      } catch {
        toast.error(t(($) => $.members.toast_share_link_copy_failed));
      }
      document.body.removeChild(textArea);
    }
  };

  if (!workspace) return null;

  const allowedShareLinkRoles = canManageWorkspace
    ? (["member", "admin", "viewer"] as const)
    : (["member", "viewer"] as const);
  const roleItems = allowedShareLinkRoles.map((value) => ({
    value,
    label: roleConfig[value].label,
  }));
  const allowedInviteRoles =
    canManageWorkspace && !inviteIsGuest
      ? (["member", "admin", "viewer"] as const)
      : (["member", "viewer"] as const);
  const inviteRoleItems = allowedInviteRoles.map((value) => ({
    value,
    label: roleConfig[value].label,
  }));
  const expiryOptions = [
    { value: "24", label: t(($) => $.members.expiry_24h) },
    { value: "168", label: t(($) => $.members.expiry_7d) },
    { value: "720", label: t(($) => $.members.expiry_30d) },
    { value: "0", label: t(($) => $.members.expiry_never) },
  ];
  const views: { value: MembersView; label: string; count: number }[] = [
    { value: "members", label: t(($) => $.members.members_label), count: members.length },
    { value: "invitations", label: t(($) => $.members.pending_label), count: invitations.length },
    ...(canInvite
      ? [{ value: "links" as const, label: t(($) => $.members.share_links_label), count: shareLinks.length }]
      : []),
  ];
  const activeView = views.some((item) => item.value === view) ? view : "members";

  return (
    <SettingsTab
      title={t(($) => $.page.tabs.members)}
      scope="workspace"
      actions={
        canInvite ? (
          <Button onClick={() => setInviteOpen(true)}>
            <Plus />
            {t(($) => $.members.invite_title)}
          </Button>
        ) : undefined
      }
    >
      {currentMember && !canManageWorkspace && !canInvite ? (
        <SettingsReadOnlyNotice wsId={wsId} />
      ) : null}
      <section className="space-y-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <SettingsViewTabs
            label={t(($) => $.page.tabs.members)}
            items={views}
            value={activeView}
            onChange={setView}
          />
          <div className="flex min-w-0 items-center gap-3">
            {seats && billingEnabled ? (
              <span className="shrink-0 text-caption text-muted-foreground">
                {t(($) => $.members.seats_used, {
                  used: seats.used + seats.reserved,
                  purchased: seats.purchased,
                })}
                {" · "}
                <AppLink
                  href={
                    navigation
                      ? settingsHref(navigation.pathname, navigation.searchParams, "billing")
                      : "?tab=billing"
                  }
                  className="text-foreground underline-offset-4 hover:underline"
                >
                  {t(($) => $.members.manage_seats)}
                </AppLink>
              </span>
            ) : null}
            {activeView === "members" ? (
              <div className="relative w-full sm:w-56">
                <Search
                  aria-hidden="true"
                  className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                />
                <Input
                  type="search"
                  value={memberQuery}
                  onChange={(event) => setMemberQuery(event.target.value)}
                  placeholder={t(($) => $.members.search_placeholder)}
                  aria-label={t(($) => $.members.search_placeholder)}
                  className="pl-8"
                />
              </div>
            ) : activeView === "links" && canInvite ? (
              <Button variant="outline" size="sm" onClick={() => setShareLinkOpen(true)}>
                <Plus />
                {t(($) => $.members.share_links_create_title)}
              </Button>
            ) : null}
          </div>
        </div>

        {activeView === "members" ? (
          visibleMembers.length > 0 ? (
            <SettingsCard>
              {visibleMembers.map((m) => (
                <div key={m.id}>
                  <MemberRow
                    member={m}
                    canManage={canManageWorkspace}
                    canManageOwners={isOwner}
                    ownerCount={ownerCount}
                    isSelf={m.user_id === user?.id}
                    busy={memberActionId === m.id}
                    onManageGroups={() => setGroupsMember(m)}
                    onGuestToggle={(isGuest) => void handleGuestToggle(m, isGuest)}
                    onRoleChange={(role) => handleRoleChange(m.id, role)}
                    onRemove={() => handleRemoveMember(m)}
                  />
                </div>
              ))}
            </SettingsCard>
          ) : (
            <ObjectSectionEmpty
              icon={needle ? Search : Users}
              description={needle ? t(($) => $.members.no_results) : t(($) => $.members.no_members)}
            />
          )
        ) : activeView === "invitations" ? (
          invitations.length > 0 ? (
            <SettingsCard>
              {invitations.map((inv) => (
                <div key={inv.id}>
                  <InvitationRow
                    invitation={inv}
                    canManage={canInvite}
                    onRevoke={() => handleRevokeInvitation(inv)}
                    busy={invitationActionId === inv.id}
                  />
                </div>
              ))}
            </SettingsCard>
          ) : (
            <ObjectSectionEmpty icon={Mail} description={t(($) => $.members.no_invitations)} />
          )
        ) : shareLinks.length > 0 ? (
          <SettingsCard>
            {shareLinks.map((link) => (
              <div key={link.id}>
                <ShareLinkRow
                  link={link}
                  onRevoke={() => handleRevokeShareLink(link)}
                  busy={shareLinkActionId === link.id}
                  onCopy={() => handleCopyShareLink(link)}
                />
              </div>
            ))}
          </SettingsCard>
        ) : (
          <ObjectSectionEmpty icon={Link} description={t(($) => $.members.no_share_links)} />
        )}
      </section>

      <Dialog
        open={inviteOpen}
        onOpenChange={(open) => {
          if (!open && !inviteLoading) {
            setInviteOpen(false);
            resetInviteForm();
          }
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t(($) => $.members.invite_title)}</DialogTitle>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(event) => {
              event.preventDefault();
              if (inviteEmail.trim()) void handleInviteMember();
            }}
          >
            <div className="space-y-1.5">
              <Label htmlFor="invite-email">{t(($) => $.members.email_label)}</Label>
              <Input
                id="invite-email"
                type="email"
                name="invite-email"
                autoComplete="email"
                spellCheck={false}
                value={inviteEmail}
                onChange={(e) => setInviteEmail(e.target.value)}
                placeholder={t(($) => $.members.invite_email_placeholder)}
                autoFocus
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="invite-role">{t(($) => $.members.role_label)}</Label>
              <Select
                items={inviteRoleItems}
                value={effectiveInviteRole}
                onValueChange={(value) => setInviteRole(value as MemberRole)}
              >
                <SelectTrigger id="invite-role" className="w-full">
                  <SelectValue>{() => roleConfig[effectiveInviteRole].label}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {inviteRoleItems.map((item) => (
                    <SelectItem
                      key={item.value}
                      value={item.value}
                      aria-label={item.label}
                    >
                      <span className="flex flex-col">
                        <span>{item.label}</span>
                        <span className="text-caption text-muted-foreground">
                          {roleConfig[item.value].description}
                        </span>
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center justify-between gap-3">
              <Label htmlFor="invite-guest">
                {t(($) => $.members.invite_guest_label)}
              </Label>
              <Switch
                id="invite-guest"
                aria-label={t(($) => $.members.invite_guest_label)}
                checked={inviteIsGuest}
                onCheckedChange={(checked) => {
                  setInviteIsGuest(checked);
                  if (!checked) setInviteGuestProjectIds([]);
                }}
              />
            </div>
            {inviteIsGuest && (
              <div className="space-y-1.5">
                <span className="text-caption font-medium">
                  {t(($) => $.members.invite_guest_projects_label)}
                </span>
                {guestSelectableProjects.length === 0 ? (
                  <p className="text-caption text-muted-foreground">
                    {t(($) => $.members.invite_guest_projects_empty)}
                  </p>
                ) : (
                  <div className="max-h-40 space-y-1 overflow-y-auto rounded-md border p-2">
                    {guestSelectableProjects.map((project) => {
                      const checked = inviteGuestProjectIds.includes(project.id);
                      const checkboxId = `invite-guest-project-${project.id}`;
                      return (
                        <div
                          key={project.id}
                          className="flex items-center gap-2 rounded px-1.5 py-1 text-caption hover:bg-accent/50"
                        >
                          <Checkbox
                            id={checkboxId}
                            aria-label={project.title}
                            checked={checked}
                            onCheckedChange={(next) => {
                              setInviteGuestProjectIds((prev) =>
                                next
                                  ? prev.includes(project.id)
                                    ? prev
                                    : [...prev, project.id]
                                  : prev.filter((id) => id !== project.id),
                              );
                            }}
                          />
                          <label
                            htmlFor={checkboxId}
                            className="flex min-w-0 flex-1 cursor-pointer items-center gap-1.5"
                          >
                            {"access" in project && project.access === "restricted" && (
                              <Lock
                                aria-hidden="true"
                                className="size-3.5 shrink-0 text-muted-foreground"
                              />
                            )}
                            <span className="min-w-0 truncate">{project.title}</span>
                          </label>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            )}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setInviteOpen(false);
                  resetInviteForm();
                }}
                disabled={inviteLoading}
              >
                {t(($) => $.members.confirm_cancel)}
              </Button>
              <Button
                type="submit"
                disabled={inviteLoading || !inviteEmail.trim()}
                aria-busy={inviteLoading || undefined}
              >
                {inviteLoading ? t(($) => $.members.inviting) : t(($) => $.members.invite_button)}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={shareLinkOpen}
        onOpenChange={(open) => {
          if (!open && !shareLinkLoading) setShareLinkOpen(false);
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t(($) => $.members.share_links_create_title)}</DialogTitle>
            <DialogDescription>{t(($) => $.members.share_links_description)}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="share-link-role">{t(($) => $.members.role_label)}</Label>
              <Select
                items={roleItems}
                value={effectiveShareLinkRole}
                onValueChange={(value) => setShareLinkRole(value as MemberRole)}
              >
                <SelectTrigger id="share-link-role" className="w-full">
                  <SelectValue>{() => roleConfig[effectiveShareLinkRole].label}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {roleItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="share-link-expiry">{t(($) => $.members.expiry_label)}</Label>
              <Select
                items={expiryOptions}
                value={shareLinkExpiry}
                onValueChange={(v) => v && setShareLinkExpiry(v)}
              >
                <SelectTrigger id="share-link-expiry" className="w-full">
                  <SelectValue>
                    {() =>
                      expiryOptions.find((option) => option.value === shareLinkExpiry)?.label ??
                      shareLinkExpiry
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {expiryOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShareLinkOpen(false)}
              disabled={shareLinkLoading}
            >
              {t(($) => $.members.confirm_cancel)}
            </Button>
            <Button
              onClick={() => void handleCreateShareLink()}
              disabled={shareLinkLoading}
              aria-busy={shareLinkLoading || undefined}
            >
              {shareLinkLoading
                ? t(($) => $.members.share_links_creating)
                : t(($) => $.members.share_links_create_button)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={inviteSeatPurchase !== null}
        onOpenChange={(open) => {
          if (
            !open &&
            (inviteSeatPurchase?.phase === "review" ||
              inviteSeatPurchase?.phase === "error")
          ) {
            setInviteSeatPurchase(null);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t(($) => $.members.seat_purchase_title)}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {inviteSeatPurchase?.phase === "review"
                ? t(($) => $.members.seat_purchase_description, {
                    email: inviteSeatPurchase.email,
                  })
                : inviteSeatPurchase?.phase === "error"
                  ? inviteSeatPurchase.error
                  : t(($) => $.members.seat_purchase_waiting)}
            </AlertDialogDescription>
          </AlertDialogHeader>

          {inviteSeatPurchase?.phase === "review" && (
            <div className="divide-y rounded-lg border text-body">
              <div className="flex items-center justify-between gap-4 px-4 py-3">
                <span className="text-muted-foreground">
                  {billingT(($) => $.workspace.seat_purchase.seats_after)}
                </span>
                <span className="font-medium">
                  {billingT(($) => $.workspace.seats.seat_count, {
                    count: inviteSeatPurchase.preview.resultingSeats,
                  })}
                </span>
              </div>
              <div className="flex items-center justify-between gap-4 px-4 py-3">
                <span className="text-muted-foreground">
                  {billingT(($) => $.workspace.seat_purchase.charge_today)}
                </span>
                <span className="font-medium">
                  {formatStripeMinorAmount(
                    inviteSeatPurchase.preview.prorationAmount,
                    inviteSeatPurchase.preview.currency,
                    locale,
                  ) ?? "—"}
                </span>
              </div>
              <div className="flex items-center justify-between gap-4 px-4 py-3">
                <span className="text-muted-foreground">
                  {billingT(($) => $.workspace.seat_purchase.next_invoice)}
                </span>
                <span className="font-medium">
                  {formatStripeMinorAmount(
                    inviteSeatPurchase.preview.nextInvoiceAmount,
                    inviteSeatPurchase.preview.currency,
                    locale,
                  ) ?? "—"}
                </span>
              </div>
              <p className="px-4 py-3 text-caption text-muted-foreground">
                {billingT(($) => $.workspace.seat_purchase.tax_notice)}
              </p>
            </div>
          )}

          {(inviteSeatPurchase?.phase === "purchasing" ||
            inviteSeatPurchase?.phase === "waiting" ||
            inviteSeatPurchase?.phase === "inviting") && (
            <div className="flex items-center gap-2 text-body text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              {inviteSeatPurchase.phase === "inviting"
                ? t(($) => $.members.inviting)
                : t(($) => $.members.seat_purchase_waiting)}
            </div>
          )}

          <AlertDialogFooter>
            {(inviteSeatPurchase?.phase === "review" ||
              inviteSeatPurchase?.phase === "error") && (
              <AlertDialogCancel>
                {t(($) => $.members.confirm_cancel)}
              </AlertDialogCancel>
            )}
            {inviteSeatPurchase?.phase === "review" && (
              <AlertDialogAction
                onClick={(event) => {
                  event.preventDefault();
                  void handlePurchaseSeatAndInvite();
                }}
              >
                {t(($) => $.members.purchase_seat_and_invite)}
              </AlertDialogAction>
            )}
            {inviteSeatPurchase?.phase === "error" &&
              inviteSeatPurchase.retryable && (
                <AlertDialogAction
                  onClick={(event) => {
                    event.preventDefault();
                    void handlePurchaseSeatAndInvite();
                  }}
                >
                  {t(($) => $.members.retry_seat_purchase)}
                </AlertDialogAction>
              )}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={!!confirmAction} onOpenChange={(v) => { if (!v) setConfirmAction(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{confirmAction?.title}</AlertDialogTitle>
            <AlertDialogDescription>{confirmAction?.description}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t(($) => $.members.confirm_cancel)}</AlertDialogCancel>
            <AlertDialogAction
              variant={confirmAction?.variant === "destructive" ? "destructive" : "default"}
              onClick={async () => {
                await confirmAction?.onConfirm();
                setConfirmAction(null);
              }}
            >
              {t(($) => $.members.confirm_action)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {groupsMember && canManageWorkspace ? (
        <MemberGroupsDialog
          wsId={wsId}
          member={groupsMember}
          onClose={() => setGroupsMember(null)}
        />
      ) : null}
    </SettingsTab>
  );
}

function MemberGroupsDialog({
  wsId,
  member,
  onClose,
}: {
  wsId: string;
  member: MemberWithUser;
  onClose: () => void;
}) {
  const { t } = useT("settings");
  const [error, setError] = useState<string | null>(null);
  const { data: groups = [], isLoading } = useQuery({
    ...memberGroupListOptions(wsId),
    enabled: Boolean(wsId),
  });

  return (
    <Dialog open onOpenChange={(nextOpen) => { if (!nextOpen) onClose(); }}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t(($) => $.members.member_groups_title, { name: member.name })}
          </DialogTitle>
        </DialogHeader>

        {isLoading ? (
          <div className="flex items-center gap-2 py-2 text-body text-muted-foreground">
            <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            <span>{t(($) => $.groups.loading)}</span>
          </div>
        ) : groups.length === 0 ? (
          <p className="py-2 text-body text-muted-foreground">
            {t(($) => $.members.member_groups_empty)}
          </p>
        ) : (
          <div className="max-h-72 divide-y divide-surface-border overflow-y-auto rounded-lg border">
            {groups.map((group) => (
              <MemberGroupToggleRow
                key={group.id}
                wsId={wsId}
                group={group}
                member={member}
                onError={setError}
              />
            ))}
          </div>
        )}

        {error ? (
          <p className="text-caption text-destructive" role="alert">
            {error}
          </p>
        ) : null}

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t(($) => $.members.member_groups_close)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function MemberGroupToggleRow({
  wsId,
  group,
  member,
  onError,
}: {
  wsId: string;
  group: MemberGroup;
  member: MemberWithUser;
  onError: (msg: string | null) => void;
}) {
  const { t } = useT("settings");
  const { data: detail, isLoading } = useQuery({
    ...memberGroupDetailOptions(wsId, group.id),
    enabled: Boolean(wsId && group.id),
  });
  const addMembers = useAddMemberGroupMembers(wsId, group.id);
  const removeMember = useRemoveMemberGroupMember(wsId, group.id);

  const detailReady = Boolean(detail && Array.isArray(detail.members));
  const isMember =
    detailReady && detail!.members.some((entry) => entry.user_id === member.user_id);
  const isPending = addMembers.isPending || removeMember.isPending;
  const cbId = `member-${member.user_id}-group-${group.id}`;

  const handleToggle = async (checked: boolean) => {
    onError(null);
    try {
      if (checked) {
        await addMembers.mutateAsync([member.user_id]);
        toast.success(
          t(($) => $.members.member_groups_added, { group: group.name }),
        );
      } else {
        await removeMember.mutateAsync(member.user_id);
        toast.success(
          t(($) => $.members.member_groups_removed, { group: group.name }),
        );
      }
    } catch {
      const msg = t(($) => $.members.member_groups_failed);
      onError(msg);
      toast.error(msg);
    }
  };

  return (
    <label
      htmlFor={cbId}
      className="flex cursor-pointer items-center justify-between gap-3 px-3 py-2.5 hover:bg-muted/40"
    >
      <div className="flex min-w-0 items-center gap-3">
        <Checkbox
          id={cbId}
          checked={isMember}
          disabled={isLoading || !detailReady || isPending}
          onCheckedChange={(val) => void handleToggle(val === true)}
        />
        <div className="min-w-0">
          <div className="truncate text-body font-medium">{group.name}</div>
          {group.description ? (
            <div className="truncate text-caption text-muted-foreground">
              {group.description}
            </div>
          ) : null}
        </div>
      </div>
      <span className="shrink-0 text-caption tabular-nums text-muted-foreground">
        {group.member_count}
      </span>
    </label>
  );
}
