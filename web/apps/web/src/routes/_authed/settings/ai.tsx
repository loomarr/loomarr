import { createFileRoute } from "@tanstack/react-router";
import { AiModelSettings, SettingsPage, useSettingsEntries } from "@/settings";

const AiSettings = () => (
  <SettingsPage
    title="AI"
    description="The model that turns a sentence into a lineup, and the quota it works within."
    entries={useSettingsEntries()}
    blocks={[{ group: "ai", title: "Provider and model", check: "llm" }]}
    footer={<AiModelSettings />}
  />
);

const Route = createFileRoute("/_authed/settings/ai")({
  component: AiSettings,
});

export { Route };
