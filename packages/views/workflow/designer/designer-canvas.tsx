"use client";

import { useCallback, useEffect, useMemo } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  MarkerType,
  useNodesState,
  type Connection,
  type Edge,
  type EdgeMouseHandler,
  type Node,
  type OnNodeDrag,
  type NodeTypes,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import type { IssueWorkflowStep, IssueWorkflowTransition } from "../types";
import { StatusNodeCard, type StatusNodeData } from "./status-node-card";
import { ensureStepLayouts, getAutoLayoutPosition } from "./clone-workflow-utils";

export interface DesignerCanvasProps {
  steps: IssueWorkflowStep[];
  wsId: string;
  onUpdateSteps: (steps: IssueWorkflowStep[]) => void;
  onEditTransition: (sourceKey: string, transition: IssueWorkflowTransition) => void;
  onAddTransition: (sourceKey: string, targetKey?: string) => void;
  selectedStepKey?: string;
  onSelectStep: (stepKey: string) => void;
}

const nodeTypes: NodeTypes = {
  statusNode: StatusNodeCard,
};

export function DesignerCanvas({
  steps,
  wsId,
  onUpdateSteps,
  onEditTransition,
  onAddTransition,
  selectedStepKey,
  onSelectStep,
}: DesignerCanvasProps) {
  const { statuses } = useIssueStatuses(wsId);

  // Ensure every step has layout coordinates (auto-layout when layout is missing)
  const stepsWithLayout = useMemo(() => {
    return ensureStepLayouts(steps);
  }, [steps]);

  // Convert steps to React Flow nodes
  const stepNodes: Node<StatusNodeData>[] = useMemo(() => {
    return stepsWithLayout.map((step, idx) => {
      const statusDef = statuses.find((s) => s.key === step.status_key);
      const isSelected = step.status_key === selectedStepKey;
      const position = step.layout ?? getAutoLayoutPosition(idx);

      return {
        id: step.status_key,
        type: "statusNode",
        position,
        data: {
          step,
          wsId,
          statusDef,
          isSelected,
          onSelect: () => onSelectStep(step.status_key),
        },
        selected: isSelected,
      };
    });
  }, [stepsWithLayout, statuses, selectedStepKey, wsId, onSelectStep]);

  // React Flow owns the node positions while a node is dragged, so the canvas
  // keeps its own node state and resyncs it from the steps. Positions reach the
  // steps on drag stop.
  const [nodes, setNodes, onNodesChange] = useNodesState<Node<StatusNodeData>>(stepNodes);
  useEffect(() => {
    setNodes(stepNodes);
  }, [stepNodes, setNodes]);

  // Convert transitions to React Flow edges
  const edges: Edge[] = useMemo(() => {
    const result: Edge[] = [];
    const validTargetKeys = new Set(steps.map((s) => s.status_key));

    for (const step of steps) {
      const transitions = step.transitions ?? [];
      for (const trans of transitions) {
        if (!validTargetKeys.has(trans.to_status_key)) continue;

        const isDefault = trans.when === null;
        const edgeLabel = trans.label || (isDefault ? "Default" : "Branch");

        result.push({
          id: `${step.status_key}->${trans.to_status_key}-${trans.id}`,
          source: step.status_key,
          target: trans.to_status_key,
          label: edgeLabel,
          type: "smoothstep",
          markerEnd: {
            type: MarkerType.ArrowClosed,
            width: 16,
            height: 16,
          },
          style: {
            strokeWidth: 2,
            stroke: isDefault ? "var(--color-border)" : "var(--color-primary)",
          },
          labelStyle: {
            fill: "var(--color-foreground)",
            fontWeight: 500,
            fontSize: 11,
          },
          labelBgStyle: {
            fill: "var(--color-card)",
            stroke: "var(--color-border)",
            strokeWidth: 1,
            rx: 4,
            ry: 4,
          },
          labelBgPadding: [4, 6],
          data: {
            sourceKey: step.status_key,
            transition: trans,
          },
        });
      }
    }

    return result;
  }, [steps]);

  // Node drag handler: dragging one node never moves another
  const handleNodeDragStop: OnNodeDrag = useCallback(
    (_, node) => {
      const nextSteps = steps.map((s) => {
        if (s.status_key !== node.id) return s;
        return {
          ...s,
          layout: {
            x: Math.round(node.position.x),
            y: Math.round(node.position.y),
          },
        };
      });
      onUpdateSteps(nextSteps);
    },
    [steps, onUpdateSteps],
  );

  // Connect handles by dragging
  const handleConnect = useCallback(
    (connection: Connection) => {
      if (!connection.source || !connection.target) return;
      if (connection.source === connection.target) return; // Disallow self-target via drag
      onAddTransition(connection.source, connection.target);
    },
    [onAddTransition],
  );

  // Click edge to edit condition
  const handleEdgeClick: EdgeMouseHandler = useCallback(
    (_, edge) => {
      const edgeData = edge.data as {
        sourceKey?: string;
        transition?: IssueWorkflowTransition;
      } | undefined;

      if (edgeData?.sourceKey && edgeData?.transition) {
        onEditTransition(edgeData.sourceKey, edgeData.transition);
      }
    },
    [onEditTransition],
  );

  return (
    <div className="w-full h-full relative bg-muted/10">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodesChange={onNodesChange}
        onNodeDragStop={handleNodeDragStop}
        onConnect={handleConnect}
        onEdgeClick={handleEdgeClick}
        onPaneClick={() => onSelectStep("")}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        minZoom={0.2}
        maxZoom={1.8}
        proOptions={{ hideAttribution: true }}
      >
        <Background gap={16} size={1} />
        <Controls showInteractive={false} position="bottom-left" />
      </ReactFlow>
    </div>
  );
}
