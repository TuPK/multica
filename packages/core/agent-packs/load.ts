import { AgentPackSchema, type AgentPack } from "./schema";
import { AGENT_PACK_IDS, AGENT_PACK_LOADERS, type AgentPackId } from "../catalog/generated/agent-packs";

export { AGENT_PACK_IDS, type AgentPackId };

/** Each bundle is generated from catalog/packs by `pnpm catalog:build` and loaded on demand. */
export async function loadAgentPack(id: AgentPackId): Promise<AgentPack> {
  const bundle = await AGENT_PACK_LOADERS[id]();
  return AgentPackSchema.parse(bundle.default);
}
