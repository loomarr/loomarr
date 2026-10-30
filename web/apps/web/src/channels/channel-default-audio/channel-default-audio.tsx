import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import { unwrap } from "@loomarr/api/unwrap";
import { Text } from "@loomarr/design-system";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { AUTO_SENTINEL, audioOptions, withSaved } from "./audio-options";

// The channel's default audio language (#2088), on Channel info beside the On-screen logo: admin-only,
// autosaving `policy.playout.audioLanguage` through the page's one writer like the other fields. It
// moved here from the player's Audio menu when Watch became its own route; the player now shows it
// read-only. It is the DEFAULT: the viewer's choice between renditions comes with alternate audio.

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
    <section className="border-border bg-card flex flex-col gap-3 rounded-lg border p-5">
      {/* The same header roles as the On-screen logo section above it. */}
      <div className="flex flex-col gap-1">
        <Text as="h2" textRole="title">
          Audio
        </Text>
        <Text as="p" textRole="hint" tone="muted">
          The language everyone watching this channel hears, when the programme carries it.
        </Text>
      </div>
      <div className="flex flex-col gap-1.5 sm:max-w-xs">
        <Label htmlFor="channel-default-audio">Default audio language</Label>
        <Select
          value={value}
          onValueChange={(v) =>
            savePolicy({
              ...channel.policy,
              playout: { ...channel.policy?.playout, audioLanguage: v === AUTO_SENTINEL ? "" : v },
            })
          }
        >
          <SelectTrigger id="channel-default-audio">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {options.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </section>
  );
};

export type { ChannelDefaultAudioProps };
export { ChannelDefaultAudio };
