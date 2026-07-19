import { createFileRoute } from "@tanstack/react-router";
import { SettingsChecklist, SettingsPage, useSettingsEntries } from "@/settings";

const ConnectionsSettings = () => (
  <SettingsPage
    title="Connections"
    description="Where Loomarr reaches your media server, requester, Tunarr and TMDB."
    entries={useSettingsEntries()}
    blocks={[
      { group: "connections.media_server", title: "Media server", check: "media_server" },
      { group: "connections.requester", title: "Requester", check: "requester" },
      { group: "connections.tunarr", title: "Tunarr", check: "tunarr" },
      { group: "connections.tmdb", title: "TMDB", check: "tmdb" },
    ]}
  >
    <SettingsChecklist />
  </SettingsPage>
);

const Route = createFileRoute("/_authed/settings/connections")({
  component: ConnectionsSettings,
});

export { Route };
