import { createFileRoute } from "@tanstack/react-router";
import { SettingsPage, useSettingsEntries } from "@/settings";

// Connections is just the four connection blocks — each self-reporting (status dot + inline
// Test + Fix), broken open / healthy collapsed. There is no wiring section and no checklist:
// putting Tunarr into the TV guide and pointing it at the library are idempotent consequences
// of saving the connection, so the server does them automatically on save (config-design §5).
// If auto-wiring fails, it surfaces on the relevant connection's own status — where the fix is.
const ConnectionsSettings = () => (
  <SettingsPage
    title="Connections"
    description="Where Loomarr reaches your media server, requester, Tunarr and TMDB."
    entries={useSettingsEntries()}
    blocks={[
      { group: "connections.media_server", title: "Media server", check: "media_server" },
      { group: "connections.requester", title: "Requester (Seerr)", check: "requester" },
      { group: "connections.tunarr", title: "Tunarr", check: "tunarr" },
      { group: "connections.tmdb", title: "TMDB", check: "tmdb" },
    ]}
  />
);

const Route = createFileRoute("/_authed/settings/connections")({
  component: ConnectionsSettings,
});

export { Route };
