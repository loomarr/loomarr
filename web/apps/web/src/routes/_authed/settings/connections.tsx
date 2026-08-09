import { createFileRoute } from "@tanstack/react-router";
import { TmdbAttribution } from "@/components/loomarr/settings";
import { blockTitle, SettingsPage, useSettingsEntries } from "@/settings";

// Connections is just the four connection blocks — each self-reporting (status dot + inline
// Test + Fix), broken open / healthy collapsed. There is no wiring section and no checklist:
// putting Tunarr into the TV guide and pointing it at the library are idempotent consequences
// of saving the connection, so the server does them automatically on save (config-design §5).
// If auto-wiring fails, it surfaces on the relevant connection's own status — where the fix is.
const ConnectionsSettings = () => {
  const entries = useSettingsEntries();
  return (
    <SettingsPage
      title="Connections"
      description="Where Loomarr reaches your media server, requester, Tunarr and TMDB."
      entries={entries}
      blocks={[
        { group: "connections.media_server", title: "Media server", check: "media_server" },
        // The requester title reflects the SAVED provider — "Requester (Seerr)" or
        // "Requester (Sonarr + Radarr)" — updating only after Save (§6).
        {
          group: "connections.requester",
          title: blockTitle(entries, "Requester", "requester.provider"),
          check: "requester",
        },
        { group: "connections.tunarr", title: "Tunarr", check: "tunarr" },
        { group: "connections.tmdb", title: "TMDB", check: "tmdb" },
      ]}
      // TMDB's licence requires this notice, and TMDB is the last block above it (§22, V52 phase
      // 7). The footer slot is the right home rather than a block-level one: the obligation is a
      // statement about the product, not a property of the connection's settings.
      footer={<TmdbAttribution />}
    />
  );
};

const Route = createFileRoute("/_authed/settings/connections")({
  component: ConnectionsSettings,
});

export { Route };
