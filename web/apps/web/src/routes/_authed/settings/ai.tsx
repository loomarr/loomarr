import { createFileRoute } from "@tanstack/react-router";
import { AiModelSettings } from "@/settings/ai-model-settings";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";

const AiSettings = () => {
  const entries = useSettingsEntries();

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
          keys: ["llm.provider", "llm.url", "llm.api_key", "llm.keep_alive"],
          // Provider and model are one setup decision. Keeping the picker inside this
          // connection block avoids a second, disconnected-looking policy panel below it.
          footer: ({ liveValue, setEdit }) => (
            <AiModelSettings
              provider={liveValue("llm.provider")}
              baseUrl={liveValue("llm.url")}
              onBaseUrlChange={(value) => setEdit("llm.url", value)}
              visionProvider={liveValue("filler.vision.provider")}
              visionModel={liveValue("filler.vision.model")}
              transcriptionProvider={liveValue("filler.transcribe.provider")}
              transcriptionModel={liveValue("filler.transcribe.model")}
              onRoleSettingChange={setEdit}
            />
          ),
        },
        {
          group: "ai",
          title: "AI behavior",
          description: "Limit suggestion work and control how aggressively self-updating channels change.",
          keys: ["suggest.max_acquisitions", "recurate.min_score_pct", "recurate.max_titles"],
        },
      ]}
    />
  );
};

const Route = createFileRoute("/_authed/settings/ai")({
  component: AiSettings,
});

export { Route };
