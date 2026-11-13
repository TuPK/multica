/**
 * Layered graph layout for flow designer (spec section 11.3).
 *
 * Computes deterministic layered positions for steps:
 * - Back edges (edges with `loop` or creating cycles) are removed to form a forward DAG.
 * - Layer of each step is the length of the longest path from `start` in the forward DAG.
 * - Back edges do not alter layer assignment.
 * - Coordinates are spaced so that no nodes overlap (width 240px, height 100px).
 * - Sorting within layers is deterministic, making the output completely stable
 *   regardless of step declaration order in the input document.
 */

export interface FlowLayoutPosition {
  x: number;
  y: number;
  layer: number;
}

export interface FlowLayoutNode {
  id: string;
  position: { x: number; y: number };
  data: {
    key: string;
    layer: number;
    title?: string;
  };
}

export interface FlowLayoutEdge {
  id: string;
  source: string;
  target: string;
  isBack: boolean;
  loop?: string;
  stop?: string;
  label?: string;
}

export interface FlowLayoutInputStep {
  key: string;
  title?: string;
  edges?: Array<{
    id: string;
    label?: string;
    to?: string;
    loop?: string;
    stop?: string;
    trigger?: string;
  }>;
}

export interface FlowLayoutInput {
  start: string;
  steps: FlowLayoutInputStep[];
  terminals?: Array<{ key: string; status?: string }>;
}

export interface FlowLayoutResult {
  positions: Record<string, FlowLayoutPosition>;
  nodes: FlowLayoutNode[];
  edges: FlowLayoutEdge[];
  maxLayer: number;
  layers: Record<number, string[]>;
}

export const NODE_WIDTH = 240;
export const NODE_HEIGHT = 100;
export const HORIZONTAL_SPACING = 340;
export const VERTICAL_SPACING = 150;

/**
 * Computes a pure layered layout for a flow graph.
 */
export function layoutFlowGraph(input: FlowLayoutInput): FlowLayoutResult {
  const { start, steps } = input;
  const stepMap = new Map<string, FlowLayoutInputStep>();
  for (const step of steps) {
    stepMap.set(step.key, step);
  }

  // Sorted list of all step keys for deterministic processing
  const allStepKeys = Array.from(stepMap.keys()).sort();

  // Extract edges and identify back edges
  // An edge is a back edge if:
  // 1. edge.loop is non-empty
  // 2. Or it forms a cycle in DFS traversal from start
  const forwardAdj = new Map<string, string[]>();
  for (const key of allStepKeys) {
    forwardAdj.set(key, []);
  }

  const allEdges: FlowLayoutEdge[] = [];
  const explicitBackEdges = new Set<string>(); // edge id

  for (const stepKey of allStepKeys) {
    const step = stepMap.get(stepKey);
    if (!step || !step.edges) continue;

    for (const e of step.edges) {
      if (e.to && stepMap.has(e.to)) {
        const isBack = Boolean(e.loop);
        if (isBack) {
          explicitBackEdges.add(e.id);
        }
        allEdges.push({
          id: e.id,
          source: stepKey,
          target: e.to,
          isBack,
          loop: e.loop,
          stop: e.stop,
          label: e.label,
        });
      } else if (e.stop) {
        allEdges.push({
          id: e.id,
          source: stepKey,
          target: `stop:${e.stop}`,
          isBack: false,
          stop: e.stop,
          label: e.label,
        });
      }
    }
  }

  // Add initial non-back edges to forward adjacency
  for (const edge of allEdges) {
    if (edge.source && edge.target && !edge.isBack && stepMap.has(edge.target)) {
      forwardAdj.get(edge.source)?.push(edge.target);
    }
  }

  // Sort neighbors for deterministic DFS
  for (const neighbors of forwardAdj.values()) {
    neighbors.sort();
  }

  // Detect and remove any remaining cycle-forming back edges via DFS
  const visited = new Set<string>();
  const inStack = new Set<string>();
  const cycleBackEdges = new Set<string>();

  function dfsDetectCycles(u: string) {
    visited.add(u);
    inStack.add(u);

    const neighbors = forwardAdj.get(u) || [];
    for (const v of neighbors) {
      if (!visited.has(v)) {
        dfsDetectCycles(v);
      } else if (inStack.has(v)) {
        // Back edge u -> v
        cycleBackEdges.add(`${u}->${v}`);
      }
    }

    inStack.delete(u);
  }

  if (stepMap.has(start)) {
    dfsDetectCycles(start);
  }
  for (const key of allStepKeys) {
    if (!visited.has(key)) {
      dfsDetectCycles(key);
    }
  }

  // Rebuild pure DAG forward adjacency without back edges
  const dagForward = new Map<string, string[]>();
  const inDegree = new Map<string, number>();
  for (const key of allStepKeys) {
    dagForward.set(key, []);
    inDegree.set(key, 0);
  }

  for (const edge of allEdges) {
    if (edge.source && edge.target && stepMap.has(edge.target)) {
      const isCycleEdge = cycleBackEdges.has(`${edge.source}->${edge.target}`);
      if (isCycleEdge) {
        edge.isBack = true;
      }
      if (!edge.isBack) {
        dagForward.get(edge.source)?.push(edge.target);
        inDegree.set(edge.target, (inDegree.get(edge.target) || 0) + 1);
      }
    }
  }

  // Compute longest path from start in the forward DAG
  // Layer of start = 0
  // Layer of v = max(layer(u) + 1) for all forward edges u -> v
  const layerMap = new Map<string, number>();
  for (const key of allStepKeys) {
    layerMap.set(key, 0);
  }

  // Topological sort of the forward DAG
  const queue: string[] = [];
  // For nodes with inDegree 0: start with `start` if present, then others sorted
  const zeroInDegree = allStepKeys.filter((k) => inDegree.get(k) === 0);
  // Put start first if in zeroInDegree
  if (zeroInDegree.includes(start)) {
    queue.push(start);
    for (const k of zeroInDegree) {
      if (k !== start) queue.push(k);
    }
  } else {
    queue.push(...zeroInDegree);
  }

  // In case start has inDegree > 0 in some strange DAG, ensure it is processed from layer 0
  if (stepMap.has(start) && !queue.includes(start)) {
    queue.unshift(start);
  }

  const topoOrder: string[] = [];
  const tempInDegree = new Map(inDegree);

  while (queue.length > 0) {
    // Pick the next node deterministically
    queue.sort();
    const u = queue.shift()!;
    topoOrder.push(u);

    const neighbors = dagForward.get(u) || [];
    for (const v of neighbors) {
      // Longest path: layer[v] = max(layer[v], layer[u] + 1)
      const uLayer = layerMap.get(u) || 0;
      const currentVLayer = layerMap.get(v) || 0;
      if (uLayer + 1 > currentVLayer) {
        layerMap.set(v, uLayer + 1);
      }

      const deg = (tempInDegree.get(v) || 1) - 1;
      tempInDegree.set(v, deg);
      if (deg === 0 && !queue.includes(v)) {
        queue.push(v);
      }
    }
  }

  // Any remaining nodes not reached by topological sort (e.g. disconnected components)
  for (const k of allStepKeys) {
    if (!topoOrder.includes(k)) {
      topoOrder.push(k);
    }
  }

  // Group steps by layer
  const layers: Record<number, string[]> = {};
  let maxLayer = 0;

  for (const key of allStepKeys) {
    const l = layerMap.get(key) || 0;
    if (l > maxLayer) maxLayer = l;
    if (!layers[l]) layers[l] = [];
    layers[l].push(key);
  }

  // Within each layer, sort keys alphabetically for deterministic positioning
  for (const l of Object.keys(layers)) {
    layers[Number(l)]?.sort();
  }

  // Compute positions
  const positions: Record<string, FlowLayoutPosition> = {};
  const nodes: FlowLayoutNode[] = [];

  for (let l = 0; l <= maxLayer; l++) {
    const keysInLayer = layers[l] || [];
    keysInLayer.forEach((key, index) => {
      const x = l * HORIZONTAL_SPACING;
      const y = index * VERTICAL_SPACING;
      positions[key] = { x, y, layer: l };

      const step = stepMap.get(key);
      nodes.push({
        id: key,
        position: { x, y },
        data: {
          key,
          layer: l,
          title: step?.title || key,
        },
      });
    });
  }

  return {
    positions,
    nodes,
    edges: allEdges,
    maxLayer,
    layers,
  };
}
