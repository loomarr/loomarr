import { createFileRoute } from "@tanstack/react-router";
import { AiModelSettings } from "@/settings/ai-model-settings";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";

const AiSettings = () => (
  <SettingsPage
    title="AI"
    description="Which models do each AI job, and the safety limits around requests and self-updating channels."
    entries={useSettingsEntries()}
    blocks={[
      {
        group: "ai",
        title: "Lineup model",
        check: "llm",
        checkLabel: "Check AI readiness",
        keys: ["llm.provider", "llm.url", "llm.api_key", "llm.keep_alive"],
      },
      {
        group: "filler",
        title: "Filler analysis models",
        keys: [
          "filler.vision.provider",
          "filler.vision.url",
          "filler.vision.api_key",
          "filler.language_provider",
          "filler.language_model",
        ],
      },
      {
        group: "ai",
        title: "Suggestion safety",
        keys: ["suggest.max_acquisitions"],
      },
      {
        group: "ai",
        title: "Self-updating channels",
        keys: ["recurate.min_score_pct", "recurate.max_titles"],
      },
    ]}
    // Render prop so the model picker reacts to the LIVE provider edit — it collapses to a
    // hosted hint the moment the dropdown flips to OpenAI, not only after Save.
    footer={({ liveValue, setEdit }) => (
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
    )}
  />
);

const Route = createFileRoute("/_authed/settings/ai")({
  component: AiSettings,
});

export { Route };
