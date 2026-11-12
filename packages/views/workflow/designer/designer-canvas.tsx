"use client";

import { useEffect, useRef, useState, useMemo } from "react";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  horizontalListSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import type { IssueWorkflowStep, IssueWorkflowTransition } from "../types";
import { StatusNodeCard } from "./status-node-card";

export interface DesignerCanvasProps {
  steps: IssueWorkflowStep[];
  wsId: string;
  onUpdateSteps: (steps: IssueWorkflowStep[]) => void;
  onEditTransition: (sourceKey: string, transition: IssueWorkflowTransition) => void;
  onAddTransition: (sourceKey: string) => void;
  selectedStepKey?: string;
  onSelectStep: (stepKey: string) => void;
}

function SortableNodeWrapper({
  step,
  wsId,
  canMoveLeft,
  canMoveRight,
  canRemove,
  onMoveLeft,
  onMoveRight,
  onRemove,
  onUpdateStep,
  onAddTransition,
  onEditTransition,
  isSelected,
  onSelect,
}: {
  step: IssueWorkflowStep;
  wsId: string;
  canMoveLeft: boolean;
  canMoveRight: boolean;
  canRemove: boolean;
  onMoveLeft: () => void;
  onMoveRight: () => void;
  onRemove: () => void;
  onUpdateStep: (updated: IssueWorkflowStep) => void;
  onAddTransition: () => void;
  onEditTransition: (transition: IssueWorkflowTransition) => void;
  isSelected?: boolean;
  onSelect?: () => void;
}) {
  const { statuses } = useIssueStatuses(wsId);
  const statusDef = statuses.find((s) => s.key === step.status_key);

  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: step.status_key });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.6 : 1,
  };

  return (
    <div ref={setNodeRef} style={style} className="shrink-0">
      <StatusNodeCard
        step={step}
        wsId={wsId}
        statusDef={statusDef}
        canMoveLeft={canMoveLeft}
        canMoveRight={canMoveRight}
        canRemove={canRemove}
        onMoveLeft={onMoveLeft}
        onMoveRight={onMoveRight}
        onRemove={onRemove}
        onUpdateStep={onUpdateStep}
        onAddTransition={onAddTransition}
        onEditTransition={onEditTransition}
        isSelected={isSelected}
        onSelect={onSelect}
        dragHandleProps={{ ...attributes, ...listeners }}
      />
    </div>
  );
}

export function DesignerCanvas({
  steps,
  wsId,
  onUpdateSteps,
  onEditTransition,
  onAddTransition,
  selectedStepKey,
  onSelectStep,
}: DesignerCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [positions, setPositions] = useState<Record<string, { x: number; y: number }>>({});

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 5 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  // Track node DOM positions to draw transition SVG curves
  useEffect(() => {
    const updatePositions = () => {
      if (!containerRef.current) return;
      const containerRect = containerRef.current.getBoundingClientRect();
      const posMap: Record<string, { x: number; y: number }> = {};

      for (const step of steps) {
        const el = document.getElementById(`node-${step.status_key}`);
        if (el) {
          const rect = el.getBoundingClientRect();
          posMap[step.status_key] = {
            x: rect.left - containerRect.left + containerRef.current.scrollLeft,
            y: rect.top - containerRect.top + containerRef.current.scrollTop,
          };
        }
      }
      setPositions(posMap);
    };

    updatePositions();
    window.addEventListener("resize", updatePositions);
    const timer = setTimeout(updatePositions, 100);

    return () => {
      window.removeEventListener("resize", updatePositions);
      clearTimeout(timer);
    };
  }, [steps]);

  const hasBacklog = steps.some((s) => s.status_key === "backlog");
  const hasBlocked = steps.some((s) => s.status_key === "blocked");
  const lastIdx = steps.length - 1;

  // Rule verification for step movement:
  // backlog must be index 0
  // todo must be index 0 (if no backlog) or 1 (if backlog)
  // blocked must be lastIdx - 2
  // done must be lastIdx - 1
  // cancelled must be lastIdx
  const isMoveAllowed = (fromIdx: number, toIdx: number): boolean => {
    if (fromIdx === toIdx || toIdx < 0 || toIdx >= steps.length) return false;
    const movingStep = steps[fromIdx]?.status_key;

    if (movingStep === "backlog" || movingStep === "todo") return false;
    if (movingStep === "done" || movingStep === "cancelled" || movingStep === "blocked") return false;

    // Target index cannot usurp fixed head
    const minAllowedIdx = hasBacklog ? 2 : 1;
    if (toIdx < minAllowedIdx) return false;

    // Target index cannot usurp fixed tail
    const maxAllowedIdx = hasBlocked ? lastIdx - 3 : lastIdx - 2;
    if (toIdx > maxAllowedIdx) return false;

    return true;
  };

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const oldIndex = steps.findIndex((s) => s.status_key === active.id);
    const newIndex = steps.findIndex((s) => s.status_key === over.id);

    if (isMoveAllowed(oldIndex, newIndex)) {
      onUpdateSteps(arrayMove(steps, oldIndex, newIndex));
    }
  };

  const handleMove = (fromIdx: number, toIdx: number) => {
    if (isMoveAllowed(fromIdx, toIdx)) {
      onUpdateSteps(arrayMove(steps, fromIdx, toIdx));
    }
  };

  const handleRemoveStep = (indexToRemove: number) => {
    const step = steps[indexToRemove];
    if (!step) return;
    if (
      step.status_key === "todo" ||
      step.status_key === "done" ||
      step.status_key === "cancelled"
    ) {
      return;
    }
    const nextSteps = steps.filter((_, idx) => idx !== indexToRemove);
    onUpdateSteps(nextSteps);
  };

  const handleUpdateStep = (index: number, updated: IssueWorkflowStep) => {
    const nextSteps = [...steps];
    nextSteps[index] = updated;
    onUpdateSteps(nextSteps);
  };

  // Build edge list for SVG render
  const edges = useMemo(() => {
    const list: Array<{
      id: string;
      sourceKey: string;
      targetKey: string;
      label: string;
      isDefault: boolean;
      sourcePos: { x: number; y: number };
      targetPos: { x: number; y: number };
      transition: IssueWorkflowTransition;
    }> = [];

    for (const step of steps) {
      if (!step.transitions) continue;
      const sourcePos = positions[step.status_key];
      if (!sourcePos) continue;

      for (const trans of step.transitions) {
        const targetPos = positions[trans.to_status_key];
        if (!targetPos) continue;

        list.push({
          id: trans.id,
          sourceKey: step.status_key,
          targetKey: trans.to_status_key,
          label: trans.label || (trans.when === null ? "Default" : "Branch"),
          isDefault: trans.when === null,
          sourcePos,
          targetPos,
          transition: trans,
        });
      }
    }
    return list;
  }, [steps, positions]);

  return (
    <div
      ref={containerRef}
      className="relative flex-1 w-full h-full overflow-x-auto overflow-y-auto p-8 min-h-[500px] bg-muted/10 [scrollbar-width:thin]"
    >
      {/* SVG Canvas for Edge connections */}
      <svg
        className="absolute inset-0 pointer-events-none w-full h-full min-w-[2000px] min-h-[800px]"
        style={{ zIndex: 0 }}
      >
        <defs>
          <marker
            id="arrow"
            viewBox="0 0 10 10"
            refX="6"
            refY="5"
            markerWidth="6"
            markerHeight="6"
            orient="auto-start-reverse"
          >
            <path d="M 0 1 L 10 5 L 0 9 z" fill="currentColor" className="text-muted-foreground" />
          </marker>
        </defs>

        {edges.map((edge) => {
          // Output port offset from source node top-left
          const sx = edge.sourcePos.x + 300;
          const sy = edge.sourcePos.y + 40;

          // Input port offset from target node top-left
          const tx = edge.targetPos.x;
          const ty = edge.targetPos.y + 40;

          // Compute smooth bezier curve
          const dx = Math.abs(tx - sx);
          const curvature = Math.max(dx * 0.4, 40);
          const cx1 = sx + curvature;
          const cy1 = sy;
          const cx2 = tx - curvature;
          const cy2 = ty;
          const pathD = `M ${sx} ${sy} C ${cx1} ${cy1}, ${cx2} ${cy2}, ${tx} ${ty}`;

          return (
            <path
              key={edge.id}
              d={pathD}
              fill="none"
              stroke="currentColor"
              strokeWidth={edge.isDefault ? 1.5 : 2}
              strokeDasharray={edge.isDefault ? "4 3" : undefined}
              markerEnd="url(#arrow)"
              className="text-muted-foreground transition-colors"
            />
          );
        })}
      </svg>

      {/* Nodes list in horizontal flow */}
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragEnd={handleDragEnd}
      >
        <SortableContext
          items={steps.map((s) => s.status_key)}
          strategy={horizontalListSortingStrategy}
        >
          <div className="relative flex items-start gap-12 z-10 min-w-max pb-16">
            {steps.map((step, idx) => {
              const canMoveLeft = isMoveAllowed(idx, idx - 1);
              const canMoveRight = isMoveAllowed(idx, idx + 1);
              const canRemove =
                step.status_key !== "todo" &&
                step.status_key !== "done" &&
                step.status_key !== "cancelled";

              return (
                <SortableNodeWrapper
                  key={step.status_key}
                  step={step}
                  wsId={wsId}
                  canMoveLeft={canMoveLeft}
                  canMoveRight={canMoveRight}
                  canRemove={canRemove}
                  onMoveLeft={() => handleMove(idx, idx - 1)}
                  onMoveRight={() => handleMove(idx, idx + 1)}
                  onRemove={() => handleRemoveStep(idx)}
                  onUpdateStep={(updated) => handleUpdateStep(idx, updated)}
                  onAddTransition={() => onAddTransition(step.status_key)}
                  onEditTransition={(trans) => onEditTransition(step.status_key, trans)}
                  isSelected={selectedStepKey === step.status_key}
                  onSelect={() => onSelectStep(step.status_key)}
                />
              );
            })}
          </div>
        </SortableContext>
      </DndContext>
    </div>
  );
}
