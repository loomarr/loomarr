import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import { SettingKey } from "@loomarr/api/models/settingKey";
import { unwrap } from "@loomarr/api/unwrap";
import { invalidateSettings } from "@loomarr/core/events";
import {
  Action,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Field,
} from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Search, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { SettingField } from "@/components/loomarr/settings/setting-field";

type Provider = "brave" | "searxng";

interface ClipDetailsSettingsProps {
  entries: SettingEntry[];
  liveValue: (key: SettingKey) => string;
  setEdit: (key: SettingKey, value: string) => void;
}

const providerLabel = (provider: string) => (provider === "searxng" ? "SearXNG" : "Brave Search");

const structuredSourceKeys = [
  SettingKey.FillerResearchWikidataEnabled,
  SettingKey.FillerResearchWikipediaEnabled,
  SettingKey.FillerResearchArchiveEnabled,
  SettingKey.FillerResearchLocEnabled,
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

  const monthly = entries.find((entry) => entry.key === SettingKey.FillerResearchMonthlyLimit);
  const structuredSources = structuredSourceKeys
    .map((key) => entries.find((entry) => entry.key === key))
    .filter((entry): entry is SettingEntry => entry !== undefined);
  const enabled = liveValue(SettingKey.FillerResearchEnabled) === "true";
  const enabledSourceCount = structuredSources.filter((entry) => liveValue(entry.key) === "true").length;
  const configured = status?.configured === true;
  const currentProvider = status?.provider === "searxng" ? "searxng" : "brave";

  const refresh = async () => {
    await Promise.all([
      invalidateSettings(queryClient),
      queryClient.invalidateQueries({ queryKey: fillerApi.getFillerResearchStatusQueryKey() }),
    ]);
  };

  const openSetup = () => {
    const next = configured ? currentProvider : "brave";
    setProvider(next);
    setAPIKey("");
    setEndpoint(next === "searxng" ? liveValue(SettingKey.FillerResearchSearxngURL) : "");
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
      const edits: Partial<Record<SettingKey, string>> = { [SettingKey.FillerResearchWebProvider]: provider };
      if (provider === "brave") {
        if (apiKey) edits[SettingKey.FillerResearchBraveAPIKey] = apiKey;
        edits[SettingKey.FillerResearchSearxngURL] = "";
      } else {
        edits[SettingKey.FillerResearchSearxngURL] = endpoint;
        edits[SettingKey.FillerResearchBraveAPIKey] = "";
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
      [SettingKey.FillerResearchWebProvider]: "none",
      [SettingKey.FillerResearchBraveAPIKey]: "",
      [SettingKey.FillerResearchSearxngURL]: "",
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
    <div className="border-border space-y-4 border-t pt-4">
      {statusQuery.isError || !enabled || configured ? (
        <div className="bg-muted/40 rounded-lg p-4">
          <div className="flex items-start gap-3">
            {configured && status?.state === "ready" ? (
              <CheckCircle2 className="text-lock mt-0.5 size-5 shrink-0" aria-hidden />
            ) : status?.state === "degraded" || status?.state === "limit_reached" ? (
              <TriangleAlert className="text-caution mt-0.5 size-5 shrink-0" aria-hidden />
            ) : (
              <Search className="text-muted-foreground mt-0.5 size-5 shrink-0" aria-hidden />
            )}
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">
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
              <p className="text-muted-foreground mt-1 text-sm leading-relaxed">
                {statusQuery.isError
                  ? "Loomarr couldn't check web search. Public-source lookups can still run."
                  : !enabled
                    ? "Existing clip details stay in place."
                    : "Web search is used only when the selected public sources don't find enough information."}
              </p>
              {configured && status ? (
                <p className="text-muted-foreground mt-2 text-xs">
                  {providerLabel(status.provider)} · {status.requestCount} of {status.requestLimit} searches
                  this month
                  {checked ? ` · Last checked ${checked}` : ""}
                </p>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}

      <details className="border-border rounded-lg border p-4">
        <summary className="cursor-pointer text-sm font-medium">
          <span>Where Loomarr looks</span>
          <span className="text-muted-foreground ml-2 font-normal">
            ·{" "}
            {enabled
              ? `${enabledSourceCount} of ${structuredSources.length} sources on`
              : `${enabledSourceCount} selected`}
          </span>
        </summary>
        <div className="mt-5 space-y-6">
          <section className="space-y-3" aria-labelledby="clip-detail-sources-heading">
            <div>
              <h3 id="clip-detail-sources-heading" className="text-sm font-medium">
                Public sources
              </h3>
              <p className="text-muted-foreground mt-1 text-xs">
                All are on by default. Turn off any source you don't want Loomarr to use.
              </p>
            </div>
            <div className="divide-border border-border divide-y rounded-lg border">
              {structuredSources.map((entry) => {
                const labelID = `clip-detail-source-${entry.key.replaceAll(".", "-")}`;
                return (
                  <div
                    key={entry.key}
                    className="flex min-h-12 items-center justify-between gap-4 px-3 py-2.5"
                  >
                    <span id={labelID} className="text-sm font-medium">
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
            <section className="border-border space-y-5 border-t pt-5" aria-label="Web search controls">
              {monthly ? (
                <SettingField
                  entry={monthly}
                  value={liveValue(monthly.key)}
                  onChange={(value) => setEdit(monthly.key, value)}
                />
              ) : null}
              <div className="flex flex-wrap gap-2">
                <Action disabled={busy} onPress={() => void runCurrentTest()} variant="outline">
                  {testProvider.isPending ? "Checking…" : "Test connection"}
                </Action>
                <Action disabled={busy} onPress={openSetup} variant="outline">
                  Replace provider
                </Action>
                <Action disabled={busy} onPress={() => void remove()} variant="ghost">
                  Remove web search
                </Action>
              </div>
            </section>
          ) : null}
        </div>
      </details>

      {!configured ? (
        <div className="bg-muted/40 flex flex-col gap-3 rounded-lg p-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="text-sm font-medium">Web search is optional</p>
            <p className="text-muted-foreground mt-1 text-sm">
              Add it if you want Loomarr to search more broadly when these sources come up short.
            </p>
          </div>
          <div className="shrink-0 self-start sm:self-auto">
            <Action disabled={!enabled || statusQuery.isLoading || statusQuery.isError} onPress={openSetup}>
              Add web search
            </Action>
          </div>
        </div>
      ) : null}

      {message ? (
        <p aria-live="polite" className={success ? "text-lock text-sm" : "text-caution-foreground text-sm"}>
          {message}
        </p>
      ) : null}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent placement="side">
          <DialogHeader>
            <DialogTitle>Add web search</DialogTitle>
            <DialogDescription>
              Loomarr sends only the clip title and search terms. It never uploads the media file.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-6 p-6">
            <fieldset className="space-y-3">
              <legend className="text-sm font-medium">Choose a provider</legend>
              <label className="border-border has-[:checked]:border-foreground has-[:checked]:bg-static-800 flex cursor-pointer items-start gap-3 rounded-lg border p-4">
                <input
                  type="radio"
                  name="web-search-provider"
                  value="brave"
                  checked={provider === "brave"}
                  onChange={() => setProvider("brave")}
                />
                <span>
                  <span className="block text-sm font-medium">Brave Search</span>
                  <span className="text-muted-foreground mt-1 block text-xs">Recommended hosted option.</span>
                </span>
              </label>
              <details className="border-border rounded-lg border p-4">
                <summary className="cursor-pointer text-sm font-medium">Advanced / self-hosted</summary>
                <label className="mt-4 flex cursor-pointer items-start gap-3">
                  <input
                    type="radio"
                    name="web-search-provider"
                    value="searxng"
                    checked={provider === "searxng"}
                    onChange={() => setProvider("searxng")}
                  />
                  <span>
                    <span className="block text-sm font-medium">SearXNG</span>
                    <span className="text-muted-foreground mt-1 block text-xs">
                      Use a server you operate.
                    </span>
                  </span>
                </label>
              </details>
            </fieldset>

            {provider === "brave" ? (
              <Field
                autoCapitalize="none"
                autoComplete="off"
                autoCorrect={false}
                id="filler-research-brave-key"
                label="Brave Search API key"
                onChangeText={setAPIKey}
                placeholder={
                  configured && currentProvider === "brave"
                    ? "Leave blank to keep the saved key"
                    : "Paste API key"
                }
                secureTextEntry
                value={apiKey}
              />
            ) : (
              <Field
                autoCapitalize="none"
                autoCorrect={false}
                id="filler-research-searxng-url"
                keyboardType="url"
                label="SearXNG address"
                onChangeText={setEndpoint}
                placeholder="https://search.example.com"
                value={endpoint}
              />
            )}

            {message ? (
              <p aria-live="polite" className="text-caution-foreground text-sm">
                {message}
              </p>
            ) : null}
            <div className="border-border flex justify-end gap-2 border-t pt-4">
              <Action disabled={busy} onPress={() => setOpen(false)} variant="ghost">
                Cancel
              </Action>
              <Action disabled={busy} onPress={() => void testAndSave()}>
                {busy ? "Checking…" : "Test and add"}
              </Action>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export { ClipDetailsSettings };
