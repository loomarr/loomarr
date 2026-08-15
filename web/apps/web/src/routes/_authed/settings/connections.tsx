import { createFileRoute } from "@tanstack/react-router";
import { TmdbAttribution } from "@/components/loomarr/settings";
import { blockTitle, SettingsPage, useSettingsEntries } from "@/settings";

// Connections is just the four connection blocks — each self-reporting (status dot + inline
// Test + Fix), with the first failure open for focused triage. There is no wiring section or checklist:
// putting Tunarr into the TV guide and pointing it at the library are idempotent consequences
// of saving the connection, so the server does them automatically on save (config-design §5).
// If auto-wiring fails, it surfaces on the relevant connection's own status — where the fix is.
const ConnectionsSettings = () => {
  const entries = useSettingsEntries();
  const usesTunarrByDefault = entries.find((entry) => entry.key === "playout.backend")?.value === "tunarr";
  return (
    <SettingsPage
      title="Connections"
      description="Connect only the services you use. Saving a valid connection completes supported setup automatically."
      entries={entries}
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
          title: blockTitle(entries, "Requester", "requester.provider"),
          check: "requester",
          optional: true,
          description:
            "Optional. Connect Seerr, or Sonarr and Radarr directly, only if Loomarr should request missing titles.",
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
            "Optional. Add a TMDB key for grounded suggestions and remote artwork; channels still play without it.",
          // TMDB's licence requires this notice (§22, V52 phase 7). It sits INSIDE the TMDB block,
          // below the API key and the Test row, rather than at the foot of the page — so it reads
          // as belonging to the connection it is about instead of as a page-level colophon under
          // four unrelated blocks. Maintainer's call.
          //
          // ⚠ Consequence, recorded rather than discovered later: the block collapses when its
          // check passes, so on a healthy install the notice is behind one click.
          //
          // ⚠ The logo is HALF the obligation and shipped a release later than the notice, because
          // it is TMDB's trademark and had to come from their attribution page rather than be
          // drawn. `blue_short` is the compact mark; the component caps its height and mutes it,
          // which is the "less prominent than our own branding" half (§22).
          //
          // ⚠ An SVG is fine HERE and nowhere near the image service. §22 refuses SVG on upload
          // because that serve route is public and an operator-supplied SVG can carry <script> —
          // stored XSS in Loomarr's own origin. This file is ours, vendored, and served by Vite
          // from public/ exactly as favicon.svg already is. It was checked for <script>/on*= on
          // the way in.
          footer: <TmdbAttribution logo={<img src="/tmdb.svg" alt="TMDB" />} />,
        },
      ]}
    />
  );
};

const Route = createFileRoute("/_authed/settings/connections")({
  component: ConnectionsSettings,
});

export { Route };
