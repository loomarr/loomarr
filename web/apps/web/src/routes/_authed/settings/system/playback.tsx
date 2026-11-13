import * as dashboardApi from "@loomarr/api/endpoints/dashboard";
import { SettingKey } from "@loomarr/api/models/settingKey";
import { unwrap } from "@loomarr/api/unwrap";
import { createFileRoute } from "@tanstack/react-router";
import { viewerIsAdmin } from "@/auth/me-query";
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
          keys: [SettingKey.PlayoutBackend, SettingKey.ServerPublicURL],
        },
        {
          group: "playout",
          title: "Picture and sound",
          description:
            "Defaults for Loomarr's internal playback. Individual channels can still choose their own audio language.",
          keys: [
            SettingKey.PlayoutQualityTier,
            SettingKey.PlayoutAudioLanguage,
            SettingKey.PlayoutToneCurve,
            SettingKey.PlayoutWatermarkLook,
            SettingKey.PlayoutWatermarkOpacityPct,
          ],
        },
        {
          group: "playout",
          title: "Live capacity",
          description:
            "Limit simultaneous internal transcodes. Loomarr selects the best detected encoder automatically; a manual override is available under Advanced.",
          keys: [SettingKey.PlayoutMaxChannels, SettingKey.PlayoutEncoder],
        },
        {
          group: "playout",
          title: "TV guide",
          description: "How channel times are displayed and how far back the guide can browse.",
          keys: [SettingKey.GuideTimezone, SettingKey.GuideRetentionHours],
        },
        {
          group: "connections.media_server",
          title: "Direct-play paths",
          description:
            "Advanced path translation for media files mounted at different locations in Loomarr and your media server.",
          keys: [SettingKey.LibraryPathMap],
        },
        {
          group: "playout",
          title: "Internal playback storage",
          description:
            "Advanced executable and live-segment storage controls. The defaults are right for most installations.",
          keys: [SettingKey.PlayoutFFmpegPath, SettingKey.PlayoutHLSDir],
        },
      ]}
    >
      <PlayoutPanel title="Current playback" status={unwrap(status.data)} loading={status.isLoading} />
    </SettingsPage>
  );
};

const Route = createFileRoute("/_authed/settings/system/playback")({
  component: PlaybackSettings,
  // Who is streaming now, so the panel opens on it rather than on "Reading playout status…". The
  // settings list is the layout's. Admin-only like the page.
  loader: async ({ context: { queryClient } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await queryClient.ensureQueryData(dashboardApi.getGetPlayoutStatusQueryOptions());
  },
});

export { Route };
