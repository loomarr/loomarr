import * as settingsApi from "@loomarr/api/endpoints/settings";
import * as setupApi from "@loomarr/api/endpoints/setup";
import * as systemApi from "@loomarr/api/endpoints/system";
import { SettingKey } from "@loomarr/api/models/settingKey";
import { unwrap } from "@loomarr/api/unwrap";
import { invalidateSettings } from "@loomarr/core/events";
import { Action, Icon, icons, LoadingStrip, Stack, Text } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { SettingsFields } from "@/components/loomarr/settings/settings-fields";
import { AiModelSettings } from "@/settings/ai-model-settings";
import { useSettingsDraft } from "@/settings/settings-draft";
import { useSettingsEntries } from "@/settings/use-settings-entries";

// The wizard's AI onboarding (config-design §6: "AI — skippable; includes the §8.1 model
// picker"). It mirrors Settings → AI: the `ai` group's PROVIDER form (Ollama vs an
// OpenAI-compatible service — the URL + key fields reveal themselves for the hosted path via
// the registry's ShowWhen) sits above the model picker, and the picker reacts to the LIVE
// provider edit — so switching to "OpenAI-compatible" flips the surface before Save, exactly
// like Settings. The heavy lifting (installed picker, download, select/pull, hot-swap) is the
// shared AiModelSettings component, reused so the two never drift.
//
// Two seams the wizard needs that plain Settings doesn't:
//   1. Selecting a model must flip the wizard's `llm` checklist dot green — AiModelSettings
//      only invalidates its OWN status, so we pass onModelChange to also refresh setup-status.
//   2. The provider form saves through the same PATCH path the other checklist blocks use.
//
// Optional by design: skipping AI is neutral, never a red X (the ConnectionBlock wrapper owns
// that). Suggestions simply stay off until a model is chosen.
const AI_GROUP = "ai";

const WizardAiBlock = () => {
  const queryClient = useQueryClient();
  const entries = useSettingsEntries();
  const llm = systemApi.useSystemLlmStatus({ query: { retry: false } });
  const status = unwrap(llm.data);

  // Local edits for the provider fields, saved as a group like every other checklist block.
  const { edits, setEdit, dropSaved } = useSettingsDraft();
  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async (response, { data }) => {
        await invalidateSettings(queryClient);
        // A refused provider field stays editable, beside the message the results give for it.
        dropSaved(response, data.edits);
      },
    },
  });
  const patchResults = unwrap(patch.data, (b) => b.results) ?? undefined;

  // Essentials only (§6): advanced keys live in Settings. This is the provider dropdown +
  // the URL/key fields whose ShowWhen reveals them when provider = openai.
  const aiEntries = entries.filter((e) => e.group === AI_GROUP && !e.advanced);

  // The LIVE provider: an unsaved dropdown edit wins over the resolved value, so the picker
  // surface flips the instant you switch to OpenAI-compatible — before Save.
  const liveProvider =
    edits[SettingKey.LLMProvider] ??
    aiEntries.find((e) => e.key === SettingKey.LLMProvider)?.value ??
    status?.provider;
  const liveBaseUrl =
    edits[SettingKey.LLMURL] ?? aiEntries.find((e) => e.key === SettingKey.LLMURL)?.value ?? "";

  // A model became active (or provider saved): flip the wizard's `llm` checklist dot green.
  const refreshSetupStatus = () =>
    queryClient.invalidateQueries({ queryKey: setupApi.getSetupStatusQueryKey() });

  const dirty = Object.keys(edits).length > 0;

  return (
    <Stack gap="4">
      {/* Provider choice + (for a hosted provider) its URL/key — the same fields Settings→AI
          shows, so the wizard offers the full Ollama-vs-external choice, not Ollama only. */}
      <SettingsFields entries={aiEntries} values={edits} onChange={setEdit} results={patchResults} />

      {patch.error != null && <ErrorState error={patch.error} />}

      {dirty && (
        <Action
          disabled={patch.isPending}
          onPress={() => patch.mutate({ data: { edits } })}
          style={{ alignSelf: "flex-start" }}
          variant="outline"
        >
          {patch.isPending ? "Saving…" : "Save provider"}
        </Action>
      )}

      {/* Auto-detect header — only meaningful on the local (Ollama) branch. */}
      {liveProvider !== "openai" &&
        (llm.isLoading ? (
          <Stack align="center" direction="row" gap="inline" render="p">
            <LoadingStrip decorative width={32} />
            <Text as="span" textRole="compact" tone="secondary">
              Looking for a local AI model…
            </Text>
          </Stack>
        ) : status?.reachable ? (
          <Stack align="center" direction="row" gap="inline" render="p">
            <Icon decorative glyph={icons.passed} size="compact" tone="success" />
            <Text as="span" textRole="compact" tone="success">
              {status.model
                ? `Ollama detected: using ${status.model}.`
                : "Ollama detected. Pick a model below to turn on suggestions."}
            </Text>
          </Stack>
        ) : (
          <Text as="p" textRole="compact" tone="secondary">
            No local Ollama found. Start Ollama on this machine, or switch the provider above to an
            OpenAI-compatible service. You can skip this and add it later. Suggestions stay off until you do.
          </Text>
        ))}

      {/* The real picker: local installed/download list, OR the hosted model picker — chosen
          by the LIVE provider. onModelChange flips the wizard's green dot when a model is set. */}
      <AiModelSettings provider={liveProvider} baseUrl={liveBaseUrl} onModelChange={refreshSetupStatus} />
    </Stack>
  );
};

export { WizardAiBlock };
