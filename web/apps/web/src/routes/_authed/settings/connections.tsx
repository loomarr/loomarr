import { SettingKey } from "@loomarr/api/models/settingKey";
import { createFileRoute } from "@tanstack/react-router";
import { blockTitle } from "@/settings/provider-title";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";
import { SuggestionDraftReturn } from "@/suggest/suggestion-draft-return";

interface ConnectionsSearch {
  focus?: "tmdb";
}

// Connections is just the four connection blocks — each self-reporting (status dot + inline
// Test + Fix), with the first failure open for focused triage. There is no wiring section or checklist:
// putting Tunarr into the TV guide and pointing it at the library are idempotent consequences
// of saving the connection, so the server does them automatically on save (config-design §5).
// If auto-wiring fails, it surfaces on the relevant connection's own status — where the fix is.
const ConnectionsSettings = () => {
  const { focus } = Route.useSearch();
  const entries = useSettingsEntries();
  const usesTunarrByDefault =
    entries.find((entry) => entry.key === SettingKey.PlayoutBackend)?.value === "tunarr";
  return (
    <SettingsPage
      title="Connections"
      description="Connect only the services you use. Saving a valid connection completes supported setup automatically."
      entries={entries}
      initialOpenGroup={focus === "tmdb" ? "connections.tmdb" : undefined}
      blocks={[
        {
          group: "connections.media_server",
          title: "Media server",
          check: "media_server",
          description:
            "Required for your library and guide. Saving lets Loomarr read the library and update supported guide wiring.",
        },
        // The requester title reflects the SAVED provider — "Requester (Seerr)" or
        // "Requester (Sonarr + Radarr)" — updating only after Save (§6).
        {
          group: "connections.requester",
          title: blockTitle(entries, "Requester", SettingKey.RequesterProvider),
          check: "requester",
          optional: true,
          description:
            "Optional. Connect Seerr, or Sonarr and Radarr directly, only if Loomarr should request missing titles.",
        },
        {
          group: "connections.sonarr",
          title: "Sonarr",
          check: "sonarr",
          optional: true,
          description:
            "Optional. Loomarr learns your shows from Sonarr and hears about new episodes as they arrive.",
        },
        {
          group: "connections.radarr",
          title: "Radarr",
          check: "radarr",
          optional: true,
          description:
            "Optional. Loomarr learns your movies from Radarr and hears about new ones as they arrive.",
        },
        {
          group: "connections.tunarr",
          title: "Tunarr",
          check: "tunarr",
          optional: !usesTunarrByDefault,
          description: usesTunarrByDefault
            ? "Required for the selected Tunarr playback engine. Loomarr chooses Tunarr's default transcode profile and updates channel programming automatically."
            : "Optional while Loomarr is the default playback engine. Connect Tunarr only for channels that use it; Loomarr chooses its default transcode profile automatically.",
        },
        {
          group: "connections.tmdb",
          title: "TMDB",
          check: "tmdb",
          optional: true,
          description:
            "Required for AI channel suggestions; otherwise optional. Add a TMDB key for grounded title matching and remote artwork.",
          // TMDB's notice and logo live on Settings → System → About, with Loomarr's credits
          // (#2151 decision 51), whatever key is in use. Maintainer's call.
        },
      ]}
    >
      <SuggestionDraftReturn />
    </SettingsPage>
  );
};

const Route = createFileRoute("/_authed/settings/connections")({
  component: ConnectionsSettings,
  validateSearch: (search: Record<string, unknown>): ConnectionsSearch => ({
    focus: search.focus === "tmdb" ? "tmdb" : undefined,
  }),
});

export { Route };
