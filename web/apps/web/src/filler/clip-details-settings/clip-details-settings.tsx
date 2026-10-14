import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Search, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { SettingField } from "@/components/loomarr/settings/setting-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";

type Provider = "brave" | "searxng";

interface ClipDetailsSettingsProps {
  entries: SettingEntry[];
  liveValue: (key: string) => string;
  setEdit: (key: string, value: string) => void;
}

const providerLabel = (provider: string) => (provider === "searxng" ? "SearXNG" : "Brave Search");

const structuredSourceKeys = [
  "filler.research.wikidata_enabled",
  "filler.research.wikipedia_enabled",
  "filler.research.archive_enabled",
  "filler.research.loc_enabled",
] as const;

const lastChecked = (value?: string): string | undefined => {
  if (!value) return undefined;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return undefined;
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
};

const ClipDetailsSettings = ({ entries, liveValue, setEdit }: ClipDetailsSettingsProps) => {
  const queryClient = useQueryClient();
  const statusQuery = fillerApi.useFillerResearchStatus();
  const testProvider = fillerApi.useFillerResearchTest();
  const patchSettings = settingsApi.useSettingsPatch();
  const status = unwrap(statusQuery.data, (body) => body);
  const [open, setOpen] = useState(false);
  const [provider, setProvider] = useState<Provider>("brave");
  const [apiKey, setAPIKey] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [message, setMessage] = useState<string>();

  const monthly = entries.find((entry) => entry.key === "filler.research.monthly_limit");
  const structuredSources = structuredSourceKeys
    .map((key) => entries.find((entry) => entry.key === key))
    .filter((entry): entry is SettingEntry => entry !== undefined);
  const enabled = liveValue("filler.research.enabled") === "true";
  const enabledSourceCount = structuredSources.filter((entry) => liveValue(entry.key) === "true").length;
  const configured = status?.configured === true;
  const currentProvider = status?.provider === "searxng" ? "searxng" : "brave";

  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() }),
      queryClient.invalidateQueries({ queryKey: fillerApi.getFillerResearchStatusQueryKey() }),
    ]);
  };

  const openSetup = () => {
    const next = configured ? currentProvider : "brave";
    setProvider(next);
    setAPIKey("");
    setEndpoint(next === "searxng" ? liveValue("filler.research.searxng_url") : "");
    setMessage(undefined);
    setOpen(true);
  };

  const testAndSave = async () => {
    setMessage(undefined);
    try {
      const tested = await testProvider.mutateAsync({
        data: {
          provider,
          ...(provider === "brave" && apiKey ? { apiKey } : {}),
          ...(provider === "searxng" && endpoint ? { endpoint } : {}),
        },
      });
      const verdict = unwrap(tested, (body) => body);
      if (!verdict?.ok) {
        setMessage(verdict?.message ?? "Loomarr couldn't verify this provider.");
        return;
      }
      const edits: Record<string, string> = { "filler.research.web_provider": provider };
      if (provider === "brave") {
        if (apiKey) edits["filler.research.brave_api_key"] = apiKey;
        edits["filler.research.searxng_url"] = "";
      } else {
        edits["filler.research.searxng_url"] = endpoint;
        edits["filler.research.brave_api_key"] = "";
      }
      const saved = await patchSettings.mutateAsync({ data: { edits } });
      const results = unwrap(saved, (body) => body.results) ?? [];
      const failed = Object.keys(edits).find(
        (key) => !results.some((result) => result.key === key && result.status === "saved"),
      );
      if (failed) {
        setMessage(
          results.find((result) => result.key === failed)?.problem ?? "Loomarr couldn't save this provider.",
        );
        return;
      }
      await refresh();
      setOpen(false);
    } catch {
      setMessage("Loomarr couldn't reach the search provider. Check the details and try again.");
    }
  };

  const runCurrentTest = async () => {
    setMessage(undefined);
    try {
      const tested = await testProvider.mutateAsync({ data: { provider: currentProvider } });
      const verdict = unwrap(tested, (body) => body);
      setMessage(verdict?.message ?? "Loomarr couldn't verify this provider.");
      await queryClient.invalidateQueries({ queryKey: fillerApi.getFillerResearchStatusQueryKey() });
    } catch {
      setMessage("Loomarr couldn't reach the search provider. Try again.");
    }
  };

  const remove = async () => {
    setMessage(undefined);
    const edits = {
      "filler.research.web_provider": "none",
      "filler.research.brave_api_key": "",
      "filler.research.searxng_url": "",
    };
    try {
      const saved = await patchSettings.mutateAsync({ data: { edits } });
      const results = unwrap(saved, (body) => body.results) ?? [];
      if (
        !Object.keys(edits).every((key) =>
          results.some((result) => result.key === key && result.status === "saved"),
        )
      ) {
        setMessage("Loomarr couldn't remove web search. Try again.");
        return;
      }
      await refresh();
    } catch {
      setMessage("Loomarr couldn't remove web search. Try again.");
    }
  };

  const busy = testProvider.isPending || patchSettings.isPending;
  const success = message === "Web search is ready.";
  const checked = lastChecked(status?.lastSuccessAt);

  return (
    <div className="space-y-4 border-border border-t pt-4">
      {statusQuery.isError || !enabled || configured ? (
        <div className="rounded-lg bg-muted/40 p-4">
          <div className="flex items-start gap-3">
            {configured && status?.state === "ready" ? (
              <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-lock" aria-hidden />
            ) : status?.state === "degraded" || status?.state === "limit_reached" ? (
              <TriangleAlert className="mt-0.5 size-5 shrink-0 text-caution" aria-hidden />
            ) : (
              <Search className="mt-0.5 size-5 shrink-0 text-muted-foreground" aria-hidden />
            )}
            <div className="min-w-0 flex-1">
              <p className="font-medium text-sm">
                {statusQuery.isError
                  ? "Web search status is unavailable"
                  : !enabled
                    ? "Automatic lookups are off"
                    : status?.state === "limit_reached"
                      ? "Web search is paused for this month"
                      : status?.state === "degraded"
                        ? "Web search needs a check"
                        : "Web search ready"}
              </p>
              <p className="mt-1 text-muted-foreground text-sm leading-relaxed">
                {statusQuery.isError
                  ? "Loomarr couldn't check web search. Public-source lookups can still run."
                  : !enabled
                    ? "Existing clip details stay in place."
                    : "Web search is used only when the selected public sources don't find enough information."}
              </p>
              {configured && status ? (
                <p className="mt-2 text-muted-foreground text-xs">
                  {providerLabel(status.provider)} · {status.requestCount} of {status.requestLimit} searches
                  this month
                  {checked ? ` · Last checked ${checked}` : ""}
                </p>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}

      <details className="rounded-lg border border-border p-4">
        <summary className="cursor-pointer font-medium text-sm">
          <span>Where Loomarr looks</span>
          <span className="ml-2 font-normal text-muted-foreground">
            ·{" "}
            {enabled
              ? `${enabledSourceCount} of ${structuredSources.length} sources on`
              : `${enabledSourceCount} selected`}
          </span>
        </summary>
        <div className="mt-5 space-y-6">
          <section className="space-y-3" aria-labelledby="clip-detail-sources-heading">
            <div>
              <h3 id="clip-detail-sources-heading" className="font-medium text-sm">
                Public sources
              </h3>
              <p className="mt-1 text-muted-foreground text-xs">
                All are on by default. Turn off any source you don't want Loomarr to use.
              </p>
            </div>
            <div className="divide-y divide-border rounded-lg border border-border">
              {structuredSources.map((entry) => {
                const labelID = `clip-detail-source-${entry.key.replaceAll(".", "-")}`;
                return (
                  <div
                    key={entry.key}
                    className="flex min-h-12 items-center justify-between gap-4 px-3 py-2.5"
                  >
                    <span id={labelID} className="font-medium text-sm">
                      {entry.label}
                    </span>
                    <SettingField
                      compact
                      labelledBy={labelID}
                      entry={entry}
                      value={liveValue(entry.key)}
                      onChange={(value) => setEdit(entry.key, value)}
                      disabledReason={!enabled ? "Turn on automatic clip details first." : undefined}
                    />
                  </div>
                );
              })}
            </div>
            {!enabled ? (
              <p className="text-muted-foreground text-xs">Turn on automatic lookups to use these sources.</p>
            ) : null}
          </section>

          {configured ? (
            <section className="space-y-5 border-border border-t pt-5" aria-label="Web search controls">
              {monthly ? (
                <SettingField
                  entry={monthly}
                  value={liveValue(monthly.key)}
                  onChange={(value) => setEdit(monthly.key, value)}
                />
              ) : null}
              <div className="flex flex-wrap gap-2">
                <Button type="button" variant="outline" onClick={() => void runCurrentTest()} disabled={busy}>
                  {testProvider.isPending ? "Checking…" : "Test connection"}
                </Button>
                <Button type="button" variant="outline" onClick={openSetup} disabled={busy}>
                  Replace provider
                </Button>
                <Button type="button" variant="ghost" onClick={() => void remove()} disabled={busy}>
                  Remove web search
                </Button>
              </div>
            </section>
          ) : null}
        </div>
      </details>

      {!configured ? (
        <div className="flex flex-col gap-3 rounded-lg bg-muted/40 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="font-medium text-sm">Web search is optional</p>
            <p className="mt-1 text-muted-foreground text-sm">
              Add it if you want Loomarr to search more broadly when these sources come up short.
            </p>
          </div>
          <Button
            type="button"
            className="shrink-0 self-start sm:self-auto"
            onClick={openSetup}
            disabled={!enabled || statusQuery.isLoading || statusQuery.isError}
          >
            Add web search
          </Button>
        </div>
      ) : null}

      {message ? (
        <p aria-live="polite" className={success ? "text-lock text-sm" : "text-caution-foreground text-sm"}>
          {message}
        </p>
      ) : null}

      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>Add web search</SheetTitle>
            <SheetDescription>
              Loomarr sends only the clip title and search terms. It never uploads the media file.
            </SheetDescription>
          </SheetHeader>
          <div className="space-y-6 p-6">
            <fieldset className="space-y-3">
              <legend className="font-medium text-sm">Choose a provider</legend>
              <label className="flex cursor-pointer items-start gap-3 rounded-lg border border-border p-4 has-[:checked]:border-signal/60 has-[:checked]:bg-signal/5">
                <input
                  type="radio"
                  name="web-search-provider"
                  value="brave"
                  checked={provider === "brave"}
                  onChange={() => setProvider("brave")}
                />
                <span>
                  <span className="block font-medium text-sm">Brave Search</span>
                  <span className="mt-1 block text-muted-foreground text-xs">Recommended hosted option.</span>
                </span>
              </label>
              <details className="rounded-lg border border-border p-4">
                <summary className="cursor-pointer font-medium text-sm">Advanced / self-hosted</summary>
                <label className="mt-4 flex cursor-pointer items-start gap-3">
                  <input
                    type="radio"
                    name="web-search-provider"
                    value="searxng"
                    checked={provider === "searxng"}
                    onChange={() => setProvider("searxng")}
                  />
                  <span>
                    <span className="block font-medium text-sm">SearXNG</span>
                    <span className="mt-1 block text-muted-foreground text-xs">
                      Use a server you operate.
                    </span>
                  </span>
                </label>
              </details>
            </fieldset>

            {provider === "brave" ? (
              <div className="space-y-2">
                <Label htmlFor="filler-research-brave-key">Brave Search API key</Label>
                <Input
                  id="filler-research-brave-key"
                  type="password"
                  autoComplete="off"
                  value={apiKey}
                  placeholder={
                    configured && currentProvider === "brave"
                      ? "Leave blank to keep the saved key"
                      : "Paste API key"
                  }
                  onChange={(event) => setAPIKey(event.target.value)}
                />
              </div>
            ) : (
              <div className="space-y-2">
                <Label htmlFor="filler-research-searxng-url">SearXNG address</Label>
                <Input
                  id="filler-research-searxng-url"
                  type="url"
                  value={endpoint}
                  placeholder="https://search.example.com"
                  onChange={(event) => setEndpoint(event.target.value)}
                />
              </div>
            )}

            {message ? (
              <p aria-live="polite" className="text-caution-foreground text-sm">
                {message}
              </p>
            ) : null}
            <div className="flex justify-end gap-2 border-border border-t pt-4">
              <Button type="button" variant="ghost" onClick={() => setOpen(false)} disabled={busy}>
                Cancel
              </Button>
              <Button type="button" onClick={() => void testAndSave()} disabled={busy}>
                {busy ? "Checking…" : "Test and add"}
              </Button>
            </div>
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
};

export { ClipDetailsSettings };
