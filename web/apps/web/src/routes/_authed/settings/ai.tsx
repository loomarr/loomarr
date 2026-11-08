import { SettingKey } from "@loomarr/api/models/settingKey";
import { createFileRoute } from "@tanstack/react-router";
import { AiModelSettings } from "@/settings/ai-model-settings";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";
import { SuggestionDraftReturn } from "@/suggest/suggestion-draft-return";

const AiSettings = () => {
  const entries = useSettingsEntries();
  // The key Loomarr ships with grounds suggestions as well as a household's own.
  const tmdbEntry = entries.find((entry) => entry.key === SettingKey.TMDBAPIKey);
  const tmdbConfigured = Boolean(tmdbEntry?.set || tmdbEntry?.builtIn);

  return (
    <SettingsPage
      title="AI"
      description="Connect one model for channel suggestions. Fine-tune request limits or specialist models only when you need them."
      entries={entries}
      blocks={[
        {
          group: "ai",
          title: "AI setup",
          description: "Choose a provider, add its credentials, then pick a lineup model.",
          check: "llm",
          checkLabel: "Check AI setup",
          dirtyCheckLabel: "Save & check AI setup",
          keys: [SettingKey.LLMProvider, SettingKey.LLMURL, SettingKey.LLMAPIKey, SettingKey.LLMKeepAlive],
          // Provider and model are one setup decision. Keeping the picker inside this
          // connection block avoids a second, disconnected-looking policy panel below it.
          footer: ({ liveValue, setEdit }) => (
            <AiModelSettings
              provider={liveValue(SettingKey.LLMProvider)}
              baseUrl={liveValue(SettingKey.LLMURL)}
              visionProvider={liveValue(SettingKey.FillerVisionProvider)}
              visionModel={liveValue(SettingKey.FillerVisionModel)}
              onRoleSettingChange={setEdit}
              tmdbConfigured={tmdbConfigured}
            />
          ),
        },
        {
          group: "ai",
          title: "Speech recognition",
          description:
            "Choose how Loomarr understands spoken language and fills in missing clip details. The built-in option needs no setup.",
          keys: [SettingKey.ASRProvider, SettingKey.ASRURL, SettingKey.ASRAPIKey, SettingKey.ASRModel],
        },
        {
          group: "ai",
          title: "AI behavior",
          description: "Limit suggestion work and control how aggressively self-updating channels change.",
          keys: [
            SettingKey.SuggestMaxAcquisitions,
            SettingKey.RecurateMinScorePct,
            SettingKey.RecurateMaxTitles,
          ],
        },
      ]}
    >
      <SuggestionDraftReturn />
    </SettingsPage>
  );
};

const Route = createFileRoute("/_authed/settings/ai")({
  component: AiSettings,
});

export { Route };
