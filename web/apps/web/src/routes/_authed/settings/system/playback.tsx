import { dashboardApi, unwrap } from "@loomarr/api";
import { createFileRoute } from "@tanstack/react-router";
import { PlayoutPanel } from "@/components/loomarr";
import { SettingsPage, useSettingsEntries } from "@/settings";

const PlaybackSettings = () => {
  const status = dashboardApi.useGetPlayoutStatus();

  return (
    <SettingsPage
      title="Playback"
      description="How Loomarr delivers channels, chooses tracks and picture quality, and protects this machine from too many simultaneous transcodes."
      entries={useSettingsEntries()}
      blocks={[
        {
          group: "playout",
          title: "Delivery",
          keys: ["playout.backend", "server.public_url"],
        },
        {
          group: "playout",
          title: "Video and audio",
          keys: ["playout.quality_tier", "playout.audio_language"],
        },
        {
          group: "playout",
          title: "Hardware and capacity",
          keys: ["playout.max_channels", "playout.encoder"],
        },
        {
          group: "playout",
          title: "Guide",
          description: "How channel times are displayed and how far back the guide can browse.",
          keys: ["guide.timezone", "guide.retention_hours"],
        },
        {
          group: "connections.media_server",
          title: "Direct play",
          keys: ["library.path_map"],
        },
        {
          group: "playout",
          title: "Playback paths",
          keys: ["playout.ffmpeg_path", "playout.hls_dir", "playout.prepared_dir"],
        },
      ]}
      footer={<PlayoutPanel status={unwrap(status.data)} loading={status.isLoading} />}
    />
  );
};

const Route = createFileRoute("/_authed/settings/system/playback")({
  component: PlaybackSettings,
});

export { Route };
