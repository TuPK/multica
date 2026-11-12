"use client";

import { useState } from "react";
import { Loader2 } from "@multica/ui/icons";
import { toast } from "sonner";
import { useWorkspaceId } from "@multica/core/hooks";
import { ApiError, errorCode } from "@multica/core/api";
import { useCurrentMember } from "@multica/core/permissions";
import { useResearchSettings, useUpdateResearchSettings } from "@multica/core/research";
import type { UpdateResearchSettingsInput } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { useT } from "../../i18n";
import { SettingsCard, SettingsRow, SettingsSection, SettingsTab } from "./settings-layout";

type KeyField = "openalex_api_key" | "semantic_scholar_api_key";

/**
 * Source settings of the Research Lab squad: the OpenAlex and Semantic Scholar API keys and the
 * contact address for Crossref and Unpaywall. A key is write-only: the server says whether one is
 * stored and never sends it back, so the box is always empty and a typed value replaces the key.
 */
export function ResearchTab() {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const { member } = useCurrentMember(wsId);
  const canManage = member?.role === "owner" || member?.role === "admin";

  const settingsQuery = useResearchSettings(wsId);
  const update = useUpdateResearchSettings(wsId);
  const settings = settingsQuery.data;

  const [openalexKey, setOpenalexKey] = useState("");
  const [s2Key, setS2Key] = useState("");
  const [email, setEmail] = useState<string | null>(null);
  const emailValue = email ?? settings?.contact_email ?? "";

  const save = async (input: UpdateResearchSettingsInput, onDone: () => void, removed = false) => {
    try {
      await update.mutateAsync(input);
      toast.success(removed ? t(($) => $.research.toast_removed) : t(($) => $.research.toast_saved));
      onDone();
    } catch (err) {
      const code = errorCode(err);
      if (code === "research_secret_not_configured") {
        toast.error(t(($) => $.research.toast_secret_missing));
      } else if (err instanceof ApiError && err.status === 400) {
        toast.error(t(($) => $.research.toast_invalid));
      } else {
        toast.error(t(($) => $.research.toast_failed));
      }
    }
  };

  const keyRow = (
    field: KeyField,
    label: string,
    configured: boolean,
    value: string,
    setValue: (v: string) => void,
  ) => (
    <SettingsRow label={label} size="text" anchor={field}>
      <div className="flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <Badge variant={configured ? "outline" : "secondary"}>
            {configured ? t(($) => $.research.status_configured) : t(($) => $.research.status_not_set)}
          </Badge>
        </div>
        {canManage ? (
          <div className="flex items-center gap-2">
            <Input
              type="password"
              autoComplete="off"
              spellCheck={false}
              aria-label={label}
              value={value}
              disabled={!settings?.keys_can_be_stored}
              placeholder={
                configured
                  ? t(($) => $.research.key_placeholder_replace)
                  : t(($) => $.research.key_placeholder_new)
              }
              onChange={(e) => setValue(e.target.value)}
            />
            <Button
              size="sm"
              disabled={!value.trim() || update.isPending || !settings?.keys_can_be_stored}
              onClick={() => save({ [field]: value.trim() }, () => setValue(""))}
            >
              {update.isPending ? <Loader2 className="size-4 animate-spin" /> : t(($) => $.research.save)}
            </Button>
            {configured ? (
              <Button
                size="sm"
                variant="outline"
                disabled={update.isPending}
                onClick={() => save({ [field]: "" }, () => setValue(""), true)}
              >
                {t(($) => $.research.remove)}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
    </SettingsRow>
  );

  return (
    <SettingsTab title={t(($) => $.research.title)} scope="workspace">
      <SettingsSection title={t(($) => $.research.sources_title)}>
        <SettingsCard>
          {settingsQuery.isLoading || !settings ? (
            <div className="flex items-center justify-center p-8 text-muted-foreground">
              <Loader2 className="size-5 animate-spin" aria-label={t(($) => $.research.loading)} />
            </div>
          ) : (
            <>
              {!settings.keys_can_be_stored ? (
                <p role="status" className="px-4 pt-3.5 text-caption text-destructive">
                  {t(($) => $.research.keys_unavailable)}
                </p>
              ) : null}
              {keyRow("openalex_api_key", t(($) => $.research.openalex_key), settings.openalex_configured, openalexKey, setOpenalexKey)}
              {keyRow(
                "semantic_scholar_api_key",
                t(($) => $.research.semantic_scholar_key),
                settings.semantic_scholar_configured,
                s2Key,
                setS2Key,
              )}
              <SettingsRow
                label={t(($) => $.research.contact_email)}
                description={t(($) => $.research.contact_email_help)}
                size="text"
                anchor="contact_email"
              >
                <div className="flex items-center gap-2">
                  <Input
                    type="email"
                    autoComplete="off"
                    aria-label={t(($) => $.research.contact_email)}
                    value={emailValue}
                    disabled={!canManage}
                    onChange={(e) => setEmail(e.target.value)}
                  />
                  {canManage ? (
                    <Button
                      size="sm"
                      disabled={update.isPending || emailValue.trim() === settings.contact_email}
                      onClick={() => save({ contact_email: emailValue.trim() }, () => setEmail(null))}
                    >
                      {t(($) => $.research.save)}
                    </Button>
                  ) : null}
                </div>
              </SettingsRow>
            </>
          )}
        </SettingsCard>
      </SettingsSection>
    </SettingsTab>
  );
}
