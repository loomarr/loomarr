import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { ChannelGrowthSection } from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { ignoreFailure, useChannelSave } from "@/channels/use-channel-save";

// What the channel takes in by itself (#4333), on Channel info for admins. Every write is the channel's
// one PATCH: taking a joined title off is the normal lineup removal (the server records it as taken
// off), adding one back is the normal lineup add, and the switch is `policy.growth.keep`.
interface ChannelGrowthProps {
  channel: ChannelDTO;
}

const ChannelGrowth = ({ channel }: ChannelGrowthProps) => {
  const queryClient = useQueryClient();
  const { isSaving, pending, save } = useChannelSave(channel.id);
  // retry off: a 501 means growth isn't wired on this install, and the section simply isn't shown.
  const growth = unwrap(channelsApi.useChannelGrowth(channel.id, { query: { retry: false } }).data);
  if (!growth) return null;

  const lineup = pending?.lineup ?? channel.lineup;
  const policy = pending?.policy ?? channel.policy;
  // The growth read follows every saved change; the writer refreshes the channel itself.
  const commit = (edit: Parameters<typeof save>[0]) =>
    save(edit)
      .then(() =>
        queryClient.invalidateQueries({ queryKey: channelsApi.getChannelGrowthQueryKey(channel.id) }),
      )
      .catch(ignoreFailure);

  return (
    <ChannelGrowthSection
      busy={isSaving}
      joined={growth.joined}
      keep={policy.growth?.keep ?? false}
      lineWords={growth.line.words}
      off={growth.off}
      onAddBack={(key) => {
        const title = growth.off.find((t) => t.key === key);
        if (!title || lineup.some((e) => e.key === key)) return;
        void commit({ lineup: [...lineup, { key: title.key, name: title.name }] });
      }}
      // The whole growth object goes back with the switch, so the taken-off list it carries survives.
      onKeepChange={(keep) => void commit({ policy: { ...policy, growth: { ...policy.growth, keep } } })}
      onTakeOff={(key) => void commit({ lineup: lineup.filter((e) => e.key !== key) })}
      short={growth.short}
      still={growth.still}
      target={growth.target}
      waiting={growth.waiting}
    />
  );
};

export type { ChannelGrowthProps };
export { ChannelGrowth };
