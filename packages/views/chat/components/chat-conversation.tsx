"use client";

import type { ReactNode } from "react";
import { AnimatePresence, motion } from "motion/react";
import { UI_EASE_OUT, UI_MOTION_DURATION } from "@multica/ui/lib/motion";
import { cn } from "@multica/ui/lib/utils";
import { useReducedMotion } from "@multica/core/appearance";
import type { ChatQuickActionsPendingState } from "@multica/core/types";
import { useRegenerateChatQuickActions } from "@multica/core/chat/mutations";
import { ChatMessageList, ChatMessageSkeleton, type ChatMessageEnterTransition } from "./chat-message-list";
import { ChatInput } from "./chat-input";
import { ChatQueue } from "./chat-queue";
import { ChatWakeupStrip, useChatWakeups } from "./chat-wakeups";
import { EmptyState } from "./chat-empty-state";
import { OfflineBanner } from "./offline-banner";
import { NoAgentBanner } from "./no-agent-banner";
import { ArchivedAgentBanner } from "./archived-agent-banner";
import { AgentAccessRevokedBanner } from "./agent-access-revoked-banner";
import { RuntimeRequiredBanner } from "./runtime-required-banner";
import { useChatAnnotations } from "./use-chat-annotations";
import { ChatAppRequests } from "../../common/app-requests";
import type { useChatController } from "./use-chat-controller";

/**
 * Smoothly expands/collapses an optional status strip above the composer so
 * async or session-dependent banners do not snap the message viewport.
 * Keying on `slotKey` keeps height steady when two consecutive sessions both
 * show a banner in the same slot.
 */
function CollapsibleSlot({
  slotKey,
  visible,
  children,
}: {
  slotKey: string;
  visible: boolean;
  children: ReactNode;
}) {
  const reduceMotion = useReducedMotion();
  const duration = reduceMotion ? 0 : UI_MOTION_DURATION.fast;

  return (
    <AnimatePresence initial={false}>
      {visible && (
        <motion.div
          key={slotKey}
          data-slot={slotKey}
          initial={{ opacity: 0, height: 0, overflow: "hidden" }}
          animate={{
            opacity: 1,
            height: "auto",
            transitionEnd: { overflow: "visible" },
          }}
          exit={{ opacity: 0, height: 0, overflow: "hidden" }}
          transition={{ duration, ease: UI_EASE_OUT }}
          className="shrink-0"
        >
          {children}
        </motion.div>
      )}
    </AnimatePresence>
  );
}

export type ChatController = ReturnType<typeof useChatController>;

/**
 * The conversation body shared by the Chat tab and embedded surfaces: the
 * message list (or empty state) above a persistent banner, the queue and the
 * input. It only renders; the caller owns the controller and everything that
 * depends on where the conversation sits.
 */
export function ChatConversation({
  controller: c,
  header,
  quickActionsPending,
  onProjectChange,
  enterTransition = null,
}: {
  controller: ChatController;
  /** Rendered above the messages, e.g. the open session's header. */
  header?: ReactNode;
  quickActionsPending: ChatQuickActionsPendingState | null;
  onProjectChange: (projectId: string | null) => void;
  /** Direction of the session switch that brought this conversation in. */
  enterTransition?: ChatMessageEnterTransition | null;
}) {
  const reduceMotion = useReducedMotion();
  const wakeups = useChatWakeups(c.activeSessionId);
  const regenerateQuickActions = useRegenerateChatQuickActions();
  const annotations = useChatAnnotations({
    sessionId: c.activeSessionId,
    agentName: c.activeAgent?.name,
    enabled: !c.isSessionArchived && !c.isAgentArchived && !c.isAgentAccessRevoked && c.isAgentRuntimeBound && !c.noAgent,
  });
  const queuedTasks = c.pendingTask?.queued_tasks ?? [];
  const statusBanner = c.isAgentAccessRevoked ? (
    <AgentAccessRevokedBanner agentName={c.activeAgent?.name} />
  ) : c.noAgent ? (
    <NoAgentBanner />
  ) : c.isAgentArchived ? (
    <ArchivedAgentBanner agentName={c.activeAgent?.name} />
  ) : !c.isAgentRuntimeBound && c.activeAgent ? (
    <RuntimeRequiredBanner agentId={c.activeAgent.id} agentName={c.activeAgent.name} />
  ) : c.availability === "offline" || c.availability === "unstable" ? (
    <OfflineBanner agentName={c.activeAgent?.name} availability={c.availability} />
  ) : null;
  const hasActiveWakeup = !!c.activeSessionId && wakeups.some((w) => w.status === "active");
  const emptyEnterClass = enterTransition
    ? reduceMotion
      ? "animate-chat-enter-fade"
      : enterTransition === "next"
        ? "animate-chat-enter-next"
        : enterTransition === "prev"
          ? "animate-chat-enter-prev"
          : "animate-chat-enter-fade"
    : undefined;
  return (
    <div className="flex flex-1 flex-col min-h-0 @container">
      {header}
      {c.showSkeleton ? (
        <ChatMessageSkeleton />
      ) : c.hasMessages ? (
        <ChatMessageList
          key={c.activeSessionId}
          revealKey={c.activeSessionId}
          enterTransition={enterTransition}
          messages={c.messages}
          pendingTask={c.pendingTask}
          availability={c.availability}
          firstItemIndex={c.firstItemIndex}
          hasOlderMessages={c.hasOlderMessages}
          isFetchingOlderMessages={c.isFetchingOlderMessages}
          onLoadOlderMessages={() => void c.fetchOlderMessages()}
          onQuickAction={(action) => c.handleSend(action.prompt)}
          quickActionsDisabled={
            !!c.pendingTaskId ||
            c.isSessionArchived ||
            c.isAgentArchived ||
            c.isAgentAccessRevoked ||
            !c.isAgentRuntimeBound ||
            c.noAgent
          }
          onRegenerateQuickActions={(message) =>
            c.activeSessionId
              ? regenerateQuickActions.mutateAsync({
                  sessionId: c.activeSessionId,
                  messageId: message.id,
                })
              : undefined
          }
          quickActionsPendingMessageId={quickActionsPending?.message_id ?? null}
          annotations={annotations}
          wakeups={wakeups}
        />
      ) : (
        <div
          key={c.activeSessionId ?? "new"}
          className={cn("flex min-h-0 flex-1 flex-col overflow-hidden", emptyEnterClass)}
        >
          <EmptyState
            agent={c.activeAgent}
            hasSessions={c.sessions.length > 0}
            onPickPrompt={c.sendConversationStarter}
          />
        </div>
      )}

      <CollapsibleSlot slotKey="chat-status-banner" visible={statusBanner !== null}>
        {statusBanner}
      </CollapsibleSlot>

      <div className="px-4">
        <ChatAppRequests sessionId={c.activeSessionId} />
      </div>

      <CollapsibleSlot slotKey="chat-wakeup-slot" visible={hasActiveWakeup}>
        <ChatWakeupStrip sessionId={c.activeSessionId} wakeups={wakeups} />
      </CollapsibleSlot>

      <ChatQueue
        tasks={queuedTasks}
        wakeups={wakeups}
        headStatus={c.pendingTask?.status}
        onSendNow={c.handleSendQueuedTaskNow}
        sendNowDisabled={c.isAgentAccessRevoked}
        onEdit={c.handleEditQueuedTask}
        onRemove={c.handleRemoveQueuedTask}
        onClear={c.handleClearQueuedTasks}
      />

      <ChatInput
        onSend={c.handleSend}
        restoreDraftRequest={c.restoreDraftRequest}
        conversationStarterRequest={c.conversationStarterRequest}
        onConversationStarterApplied={c.handleConversationStarterApplied}
        onRestoreDraftApplied={c.handleRestoreDraftApplied}
        uploadEnabled={c.uploadEnabled && !c.isAgentAccessRevoked}
        onStop={c.handleStop}
        isRunning={!!c.pendingTaskId}
        allowSubmitWhileRunning={c.pendingTask?.supports_queue === true}
        canSteerWhileRunning={c.pendingTask?.can_supplement}
        disabled={
          c.isSessionArchived ||
          c.isAgentArchived ||
          c.isAgentAccessRevoked ||
          !c.isAgentRuntimeBound
        }
        noAgent={c.noAgent}
        agentArchived={c.isAgentArchived}
        agentAccessRevoked={c.isAgentAccessRevoked}
        agentRuntimeRequired={!c.isAgentRuntimeBound}
        agentName={c.activeAgent?.name}
        projects={c.projects}
        projectId={c.activeProjectId}
        projectContextUnsupported={c.projectContextUnsupported}
        onProjectChange={onProjectChange}
        isProjectUpdating={c.isProjectUpdating}
        focusRequest={c.focusInputRequest}
        annotations={annotations}
      />
    </div>
  );
}
