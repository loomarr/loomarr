import * as dashboardApi from "@loomarr/api/endpoints/dashboard";
import { unwrap } from "@loomarr/api/unwrap";
import { createFileRoute } from "@tanstack/react-router";
import { PlayoutPanel } from "@/components/loomarr/dashboard/playout-panel";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";

const PlaybackSettings = () => {
  const status = dashboardApi.useGetPlayoutStatus();

  return (
    <SettingsPage
      title="Playback"
      description="See who is streaming your channels, then set the everyday playback preferences for this machine."
      entries={useSettingsEntries()}
      blocks={[
        {
          group: "playout",
          title: "Playback engine",
          description:
            "Choose who streams channels by default. Loomarr manages internal playback here; Tunarr-backed channels use the transcode profile in Settings → Connections.",
          keys: ["playout.backend", "server.public_url"],
        },
        {
          group: "playout",
          title: "Picture and sound",
          description:
            "Defaults for Loomarr's internal playback. Individual channels can still choose their own audio language.",
          keys: ["playout.quality_tier", "playout.audio_language"],
        },
        {
          group: "playout",
          title: "Live capacity",
          description:
            "Limit simultaneous internal transcodes. Loomarr selects the best detected encoder automatically; a manual override is available under Advanced.",
          keys: ["playout.max_channels", "playout.encoder"],
        },
        {
          group: "playout",
          title: "TV guide",
          description: "How channel times are displayed and how far back the guide can browse.",
          keys: ["guide.timezone", "guide.retention_hours"],
        },
        {
          group: "connections.media_server",
          title: "Direct-play paths",
          description:
            "Advanced path translation for media files mounted at different locations in Loomarr and your media server.",
          keys: ["library.path_map"],
        },
        {
          group: "playout",
          title: "Internal playback storage",
          description:
            "Advanced executable, cache, and prepared-media storage controls. The defaults are right for most installations.",
          keys: [
            "playout.ffmpeg_path",
            "playout.hls_dir",
            "playout.prepared_dir",
            "playout.prepared_budget_gb",
          ],
        },
      ]}
    >
      <PlayoutPanel title="Current playback" status={unwrap(status.data)} loading={status.isLoading} />
    </SettingsPage>
  );
};

const Route = createFileRoute("/_authed/settings/system/playback")({
  component: PlaybackSettings,
});

export { Route };
