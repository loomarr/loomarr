import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import { unwrap } from "@loomarr/api/unwrap";
import { AUTO_SENTINEL, audioOptions, withSaved } from "@loomarr/core/channel-audio";
import { ChannelAudioSection } from "@loomarr/ui";

// The channel's default audio language (#2088), on Channel info beside the On-screen logo: admin-only,
// autosaving `policy.playout.audioLanguage` through the page's one writer like the other fields. It
// is a channel setting, so it lives with the channel rather than in the player, whose Settings
// sheet shows it read-only.

interface ChannelDefaultAudioProps {
  channel: ChannelDTO;
  savePolicy: (policy: ChannelPolicy) => void;
}

const ChannelDefaultAudio = ({ channel, savePolicy }: ChannelDefaultAudioProps) => {
  // The choices are the tracks the airing programme actually carries, so only a channel on air has
  // any to offer beyond Auto and the saved value.
  const tracks = channelsApi.useChannelTracks(channel.id, {
    query: { enabled: channel.status === "live", retry: false },
  });
  const value = channel.policy?.playout?.audioLanguage || AUTO_SENTINEL;
  const options = withSaved(audioOptions(unwrap(tracks.data)?.audio ?? []), value);

  return (
    <ChannelAudioSection
      options={options}
      value={value}
      onValueChange={(v) =>
        savePolicy({
          ...channel.policy,
          playout: { ...channel.policy?.playout, audioLanguage: v === AUTO_SENTINEL ? "" : v },
        })
      }
    />
  );
};

export type { ChannelDefaultAudioProps };
export { ChannelDefaultAudio };
