import { createFileRoute } from "@tanstack/react-router";
import { AiModelSettings, SettingsPage, useSettingsEntries } from "@/settings";

const AiSettings = () => (
  <SettingsPage
    title="AI"
    description="The model that turns a sentence into a lineup, and the quota it works within."
    entries={useSettingsEntries()}
    blocks={[{ group: "ai", title: "Provider and model", check: "llm" }]}
    // Render prop so the model picker reacts to the LIVE provider edit — it collapses to a
    // hosted hint the moment the dropdown flips to OpenAI, not only after Save.
    footer={({ liveValue }) => <AiModelSettings provider={liveValue("llm.provider")} />}
  />
);

const Route = createFileRoute("/_authed/settings/ai")({
  component: AiSettings,
});

export { Route };
