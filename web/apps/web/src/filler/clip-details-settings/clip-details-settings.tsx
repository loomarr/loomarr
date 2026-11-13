import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import { SettingKey } from "@loomarr/api/models/settingKey";
import { unwrap } from "@loomarr/api/unwrap";
import { invalidateSettings } from "@loomarr/core/events";
import {
  Action,
  Card,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Field,
  Icon,
  icons,
  Stack,
  Text,
} from "@loomarr/design-system";
import { useStore } from "@tanstack/react-form";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { SettingField } from "@/components/loomarr/settings/setting-field";
import { useAppForm } from "@/lib/app-form";

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
  const [message, setMessage] = useState<string>();
  const form = useAppForm({
    defaultValues: { provider: "brave" as Provider, apiKey: "", endpoint: "" },
    onSubmit: ({ value }) => testAndSave(value),
  });
  const provider = useStore(form.store, (state) => state.values.provider);

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
    form.reset({
      provider: next,
      apiKey: "",
      endpoint: next === "searxng" ? liveValue(SettingKey.FillerResearchSearxngURL) : "",
    });
    setMessage(undefined);
    setOpen(true);
  };

  const testAndSave = async ({
    provider,
    apiKey,
    endpoint,
  }: {
    provider: Provider;
    apiKey: string;
    endpoint: string;
  }) => {
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
    <Stack borderColor="borderDecorative" borderTopWidth={1} gap="4" paddingTop={16}>
      {statusQuery.isError || !enabled || configured ? (
        <Stack className="bg-muted/40 rounded-lg" padding={16}>
          <Stack align="start" direction="row" gap="control">
            {configured && status?.state === "ready" ? (
              <Stack direction="row" marginTop={2} render="span">
                <Icon decorative glyph={icons.passed} size="default" tone="success" />
              </Stack>
            ) : status?.state === "degraded" || status?.state === "limit_reached" ? (
              <Stack direction="row" marginTop={2} render="span">
                <Icon decorative glyph={icons.alert} size="default" tone="warning" />
              </Stack>
            ) : (
              <Stack direction="row" marginTop={2} render="span">
                <Icon decorative glyph={icons.search} size="default" tone="muted" />
              </Stack>
            )}
            <Stack flexBasis="0%" flexGrow={1} gap="1" minWidth={0}>
              <Text as="p" textRole="cardLabel">
                {statusQuery.isError
                  ? "Web search status is unavailable"
                  : !enabled
                    ? "Automatic lookups are off"
                    : status?.state === "limit_reached"
                      ? "Web search is paused for this month"
                      : status?.state === "degraded"
                        ? "Web search needs a check"
                        : "Web search ready"}
              </Text>
              <Text as="p" textRole="compact" tone="secondary">
                {statusQuery.isError
                  ? "Loomarr couldn't check web search. Public-source lookups can still run."
                  : !enabled
                    ? "Existing clip details stay in place."
                    : "Web search is used only when the selected public sources don't find enough information."}
              </Text>
              {configured && status ? (
                <Stack marginTop={4}>
                  <Text as="p" textRole="hint" tone="secondary">
                    {providerLabel(status.provider)} · {status.requestCount} of {status.requestLimit} searches
                    this month
                    {checked ? ` · Last checked ${checked}` : ""}
                  </Text>
                </Stack>
              ) : null}
            </Stack>
          </Stack>
        </Stack>
      ) : null}

      <Card padding={16} render="details">
        <Text as="summary" className="cursor-pointer" textRole="cardLabel">
          <span>Where Loomarr looks</span>
          <Text as="span" className="ml-2" textRole="compact" tone="secondary">
            ·{" "}
            {enabled
              ? `${enabledSourceCount} of ${structuredSources.length} sources on`
              : `${enabledSourceCount} selected`}
          </Text>
        </Text>
        <Stack gap="6" marginTop={20}>
          <Stack aria-labelledby="clip-detail-sources-heading" gap="3" render="section">
            <Stack gap="1">
              <Text as="h3" id="clip-detail-sources-heading" textRole="cardLabel">
                Public sources
              </Text>
              <Text as="p" textRole="hint" tone="secondary">
                All are on by default. Turn off any source you don't want Loomarr to use.
              </Text>
            </Stack>
            <Card className="divide-border divide-y">
              {structuredSources.map((entry) => {
                const labelID = `clip-detail-source-${entry.key.replaceAll(".", "-")}`;
                return (
                  <Stack
                    align="center"
                    direction="row"
                    flexShrink={0}
                    gap="4"
                    justify="between"
                    key={entry.key}
                    minHeight={48}
                    paddingHorizontal={12}
                    paddingVertical={10}
                  >
                    <Text as="span" id={labelID} textRole="cardLabel">
                      {entry.label}
                    </Text>
                    <SettingField
                      compact
                      labelledBy={labelID}
                      entry={entry}
                      value={liveValue(entry.key)}
                      onChange={(value) => setEdit(entry.key, value)}
                      disabledReason={!enabled ? "Turn on automatic clip details first." : undefined}
                    />
                  </Stack>
                );
              })}
            </Card>
            {!enabled ? (
              <Text as="p" textRole="hint" tone="secondary">
                Turn on automatic lookups to use these sources.
              </Text>
            ) : null}
          </Stack>

          {configured ? (
            <Stack
              aria-label="Web search controls"
              borderColor="borderDecorative"
              borderTopWidth={1}
              gap="5"
              paddingTop={20}
              render="section"
            >
              {monthly ? (
                <SettingField
                  entry={monthly}
                  value={liveValue(monthly.key)}
                  onChange={(value) => setEdit(monthly.key, value)}
                />
              ) : null}
              <Stack direction="row" gap="inline" wrap>
                <Action disabled={busy} onPress={() => void runCurrentTest()} variant="outline">
                  {testProvider.isPending ? "Checking…" : "Test connection"}
                </Action>
                <Action disabled={busy} onPress={openSetup} variant="outline">
                  Replace provider
                </Action>
                <Action disabled={busy} onPress={() => void remove()} variant="ghost">
                  Remove web search
                </Action>
              </Stack>
            </Stack>
          ) : null}
        </Stack>
      </Card>

      {!configured ? (
        <div className="bg-muted/40 flex flex-col gap-3 rounded-lg p-4 sm:flex-row sm:items-center sm:justify-between">
          <Stack gap="1">
            <Text as="p" textRole="cardLabel">
              Web search is optional
            </Text>
            <Text as="p" textRole="compact" tone="secondary">
              Add it if you want Loomarr to search more broadly when these sources come up short.
            </Text>
          </Stack>
          <div className="shrink-0 self-start sm:self-auto">
            <Action disabled={!enabled || statusQuery.isLoading || statusQuery.isError} onPress={openSetup}>
              Add web search
            </Action>
          </div>
        </div>
      ) : null}

      {message ? (
        <Text as="p" aria-live="polite" textRole="compact" tone={success ? "success" : "warning"}>
          {message}
        </Text>
      ) : null}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent placement="side">
          <DialogHeader>
            <DialogTitle>Add web search</DialogTitle>
            <DialogDescription>
              Loomarr sends only the clip title and search terms. It never uploads the media file.
            </DialogDescription>
          </DialogHeader>
          <Stack gap="6" padding={24}>
            <Stack gap="3" render="fieldset">
              <Text as="legend" textRole="cardLabel">
                Choose a provider
              </Text>
              <Stack gap="3" marginTop={12}>
                <label className="border-border has-[:checked]:border-foreground has-[:checked]:bg-static-800 flex cursor-pointer items-start gap-3 rounded-lg border p-4">
                  <input
                    type="radio"
                    name="web-search-provider"
                    value="brave"
                    checked={provider === "brave"}
                    onChange={() => form.setFieldValue("provider", "brave")}
                  />
                  <Stack gap="1" render="span">
                    <Text as="span" textRole="cardLabel">
                      Brave Search
                    </Text>
                    <Text as="span" textRole="hint" tone="secondary">
                      Recommended hosted option.
                    </Text>
                  </Stack>
                </label>
                <Card padding={16} render="details">
                  <Text as="summary" className="cursor-pointer" textRole="cardLabel">
                    Advanced / self-hosted
                  </Text>
                  <label className="mt-4 flex cursor-pointer items-start gap-3">
                    <input
                      type="radio"
                      name="web-search-provider"
                      value="searxng"
                      checked={provider === "searxng"}
                      onChange={() => form.setFieldValue("provider", "searxng")}
                    />
                    <Stack gap="1" render="span">
                      <Text as="span" textRole="cardLabel">
                        SearXNG
                      </Text>
                      <Text as="span" textRole="hint" tone="secondary">
                        Use a server you operate.
                      </Text>
                    </Stack>
                  </label>
                </Card>
              </Stack>
            </Stack>

            {provider === "brave" ? (
              <form.Field name="apiKey">
                {(field) => (
                  <Field
                    autoCapitalize="none"
                    autoComplete="off"
                    autoCorrect={false}
                    id="filler-research-brave-key"
                    label="Brave Search API key"
                    onChangeText={field.handleChange}
                    placeholder={
                      configured && currentProvider === "brave"
                        ? "Leave blank to keep the saved key"
                        : "Paste API key"
                    }
                    secureTextEntry
                    value={field.state.value}
                  />
                )}
              </form.Field>
            ) : (
              <form.Field name="endpoint">
                {(field) => (
                  <Field
                    autoCapitalize="none"
                    autoCorrect={false}
                    id="filler-research-searxng-url"
                    keyboardType="url"
                    label="SearXNG address"
                    onChangeText={field.handleChange}
                    placeholder="https://search.example.com"
                    value={field.state.value}
                  />
                )}
              </form.Field>
            )}

            {message ? (
              <Text as="p" aria-live="polite" textRole="compact" tone="warning">
                {message}
              </Text>
            ) : null}
            <Stack
              borderColor="borderDecorative"
              borderTopWidth={1}
              direction="row"
              gap="inline"
              justify="end"
              paddingTop={16}
            >
              <Action disabled={busy} onPress={() => setOpen(false)} variant="ghost">
                Cancel
              </Action>
              <Action disabled={busy} onPress={() => void form.handleSubmit()}>
                {busy ? "Checking…" : "Test and add"}
              </Action>
            </Stack>
          </Stack>
        </DialogContent>
      </Dialog>
    </Stack>
  );
};

export { ClipDetailsSettings };
