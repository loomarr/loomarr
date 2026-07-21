import { createFileRoute } from "@tanstack/react-router";
import { SettingsPage, useSettingsEntries } from "@/settings";

const ChannelsSettings = () => (
  <SettingsPage
    title="Channels & playback"
    description="Defaults every channel inherits — ordering, filling in gaps, and the programming policy."
    entries={useSettingsEntries()}
    blocks={[{ group: "channels", title: "Scheduling defaults" }]}
  />
);

const Route = createFileRoute("/_authed/settings/channels")({
  component: ChannelsSettings,
});

export { Route };
