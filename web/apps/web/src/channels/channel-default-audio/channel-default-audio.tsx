import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import { unwrap } from "@loomarr/api/unwrap";
import { ChannelAudioSection } from "@loomarr/ui";
import { AUTO_SENTINEL, audioOptions, burnOptions, OFF_SENTINEL, withSaved } from "./audio-options";

// The channel's default audio language and burned-in subtitles (#2088), on Channel info beside the
// On-screen logo: admin-only, autosaving `policy.playout.audioLanguage` and
// `policy.playout.burnSubtitles` through the page's one writer like the other fields. The audio
// default moved here from the player's Audio menu when Watch became its own route; the player's
// Settings sheet now shows it read-only.

interface ChannelDefaultAudioProps {
  channel: ChannelDTO;
  savePolicy: (policy: ChannelPolicy) => void;
}

const ChannelDefaultAudio = ({ channel, savePolicy }: ChannelDefaultAudioProps) => {
  // The choices are the tracks the airing programme actually carries, so only a channel on air has
  // any to offer beyond Auto/Off and the saved values.
  const tracks = channelsApi.useChannelTracks(channel.id, undefined, {
    query: { enabled: channel.status === "live", retry: false },
  });
  const airing = unwrap(tracks.data);
  const value = channel.policy?.playout?.audioLanguage || AUTO_SENTINEL;
  const burn = channel.policy?.playout?.burnSubtitles || OFF_SENTINEL;
  const savePlayout = (playout: NonNullable<ChannelPolicy["playout"]>) =>
    savePolicy({ ...channel.policy, playout: { ...channel.policy?.playout, ...playout } });

  return (
    <ChannelAudioSection
      options={withSaved(audioOptions(airing?.audio ?? []), value)}
      value={value}
      onValueChange={(v) => savePlayout({ audioLanguage: v === AUTO_SENTINEL ? "" : v })}
      subtitles={{
        options: withSaved(burnOptions(airing?.subtitles ?? []), burn),
        value: burn,
        onValueChange: (v) => savePlayout({ burnSubtitles: v === OFF_SENTINEL ? "" : v }),
      }}
    />
  );
};

export type { ChannelDefaultAudioProps };
export { ChannelDefaultAudio };
