"use client";

import { useId, useMemo, useState, type FormEvent } from "react";
import { Loader2 } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { useWorkspaceId } from "@multica/core/hooks";
import { usePreviewHubImport, type HubImportPreview } from "@multica/core/hub";
import { useT } from "../i18n";
import {
  errorMessage,
  parseWebAddress,
  previewVars,
  resolveRegistryItemUrl,
  type HubImportRequest,
} from "./hub-import-request";

type RegistryItem = NonNullable<NonNullable<HubImportPreview["registry"]>["items"]>[number];

/** Lists a hub-index.json registry; picking an item previews it through the wizard. */
export function HubImportRegistry({
  onPreview,
}: {
  onPreview: (request: HubImportRequest) => Promise<void>;
}) {
  const { t } = useT("hub");
  const wsId = useWorkspaceId();
  const listing = usePreviewHubImport();
  const inputId = useId();
  const searchId = useId();
  const [registryInput, setRegistryInput] = useState("");
  const [loaded, setLoaded] = useState<{ url: string; items: RegistryItem[] } | null>(null);
  const [query, setQuery] = useState("");
  const [pendingItem, setPendingItem] = useState<string | null>(null);
  const [error, setError] = useState("");

  const visible = useMemo(() => {
    const items = loaded?.items ?? [];
    const needle = query.trim().toLowerCase();
    if (!needle) return items;
    return items.filter((item) =>
      [item.name, item.description, ...item.tags].some((field) => field.toLowerCase().includes(needle)),
    );
  }, [loaded, query]);

  const load = async (event: FormEvent) => {
    event.preventDefault();
    const url = parseWebAddress(registryInput);
    if (!url) {
      setError(t(($) => $.add.registry.invalid_url));
      return;
    }
    setError("");
    try {
      const result = await listing.mutateAsync(
        previewVars(wsId, { kind: "source", source: { type: "registry", url } }),
      );
      setLoaded({ url, items: result.registry?.items ?? [] });
      setQuery("");
    } catch (err) {
      setError(errorMessage(err, t(($) => $.add.registry.load_failed)));
    }
  };

  const pick = async (item: RegistryItem) => {
    if (!loaded) return;
    setPendingItem(item.url);
    setError("");
    try {
      await onPreview({
        kind: "source",
        // A picked item is a plain zip download; only the index itself is previewed as "registry".
        source: {
          type: "url_zip",
          url: resolveRegistryItemUrl(loaded.url, item.url),
          sha256: item.sha256,
        },
      });
    } catch (err) {
      setError(errorMessage(err, t(($) => $.add.registry.preview_failed)));
    } finally {
      setPendingItem(null);
    }
  };

  return (
    <div className="flex min-h-0 flex-col gap-4">
      <form onSubmit={load} className="flex flex-col gap-1.5">
        <Label htmlFor={inputId} className="text-caption text-muted-foreground">
          {t(($) => $.add.registry.url_label)}
        </Label>
        <div className="flex gap-2">
          <Input
            id={inputId}
            type="url"
            inputMode="url"
            autoComplete="off"
            spellCheck={false}
            value={registryInput}
            onChange={(event) => setRegistryInput(event.target.value)}
            placeholder={t(($) => $.add.registry.url_placeholder)}
            aria-invalid={error ? true : undefined}
          />
          <Button type="submit" variant="outline" disabled={!registryInput.trim() || listing.isPending} aria-busy={listing.isPending}>
            {listing.isPending ? <Loader2 className="animate-spin" aria-hidden="true" /> : null}
            {t(($) => $.add.registry.load)}
          </Button>
        </div>
      </form>

      {error ? (
        <p role="alert" className="text-caption text-destructive">
          {error}
        </p>
      ) : null}

      {loaded ? (
        loaded.items.length === 0 ? (
          <p className="text-label text-muted-foreground">{t(($) => $.add.registry.empty)}</p>
        ) : (
          <div className="flex min-h-0 flex-col gap-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor={searchId} className="sr-only">
                {t(($) => $.add.registry.search_label)}
              </Label>
              <Input
                id={searchId}
                type="search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t(($) => $.add.registry.search_placeholder)}
              />
            </div>
            {visible.length === 0 ? (
              <p className="text-label text-muted-foreground">{t(($) => $.add.registry.no_matches)}</p>
            ) : (
              <ul className="flex max-h-72 flex-col divide-y overflow-y-auto rounded-md border">
                {visible.map((item) => (
                  <li key={item.url} className="flex min-w-0 items-center gap-3 px-3 py-2">
                    <div className="flex min-w-0 flex-1 flex-col">
                      <span className="truncate text-label font-medium">{item.name}</span>
                      {item.description ? (
                        <span className="truncate text-caption text-muted-foreground">{item.description}</span>
                      ) : null}
                      <span className="truncate text-caption text-muted-foreground">
                        {[
                          t(($) => $.kind_singular[item.kind === "plugin" ? "plugin" : "skill"]),
                          item.version ? t(($) => $.add.registry.version, { version: item.version }) : null,
                          item.tags.join(", ") || null,
                        ]
                          .filter(Boolean)
                          .join(" · ")}
                      </span>
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={pendingItem !== null}
                      aria-busy={pendingItem === item.url}
                      aria-label={t(($) => $.add.registry.preview_item, { name: item.name })}
                      onClick={() => void pick(item)}
                    >
                      {pendingItem === item.url ? <Loader2 className="animate-spin" aria-hidden="true" /> : null}
                      {t(($) => $.add.registry.preview)}
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )
      ) : null}
    </div>
  );
}
