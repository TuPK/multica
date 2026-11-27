"use client";

import type { ReactNode } from "react";
import { TabsContent, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { useT } from "../i18n";
import { GovernanceEffectsTab } from "./effects/governance-effects-tab";
import { useEffectsEnabled } from "./effects/use-effects-enabled";
import { GovernanceOutcomesTab } from "./outcomes/governance-outcomes-tab";
import { useOutcomesEnabled } from "./outcomes/use-outcomes-enabled";
import { PrincipalProfilePage } from "./principal/principal-profile-page";
import { usePrincipalEnabled } from "./principal/use-principal-enabled";

/**
 * The durable tabs of Governance (effects, outcomes and principal profile). Each one shows only while its flag is on;
 * the governance page places the triggers in its tab list and the contents after its own tabs.
 */
export function useDurableGovernanceTabs(): { triggers: ReactNode; contents: ReactNode } {
  const { t } = useT("governance");
  const effectsOn = useEffectsEnabled();
  const outcomesOn = useOutcomesEnabled();
  const principalOn = usePrincipalEnabled();
  return {
    triggers: (
      <>
        {effectsOn ? <TabsTrigger value="effects">{t(($) => $.tabs.effects)}</TabsTrigger> : null}
        {outcomesOn ? <TabsTrigger value="outcomes">{t(($) => $.tabs.outcomes)}</TabsTrigger> : null}
        {principalOn ? <TabsTrigger value="principal">{t(($) => $.tabs.principal)}</TabsTrigger> : null}
      </>
    ),
    contents: (
      <>
        {effectsOn ? (
          <TabsContent value="effects" className="min-h-0 flex-1 overflow-y-auto">
            <GovernanceEffectsTab />
          </TabsContent>
        ) : null}
        {outcomesOn ? (
          <TabsContent value="outcomes" className="min-h-0 flex-1 overflow-y-auto">
            <GovernanceOutcomesTab />
          </TabsContent>
        ) : null}
        {principalOn ? (
          <TabsContent value="principal" className="min-h-0 flex-1 overflow-y-auto">
            <PrincipalProfilePage />
          </TabsContent>
        ) : null}
      </>
    ),
  };
}
