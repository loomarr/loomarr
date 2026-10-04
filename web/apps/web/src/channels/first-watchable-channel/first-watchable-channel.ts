import type { ChannelDTO } from "@loomarr/api/models/channelDTO";

/**
 * Decision X2 (#1785, #1659 Shell): the phone bottom bar's Watch target. A per-user last-tuned
 * channel needs #1662, which isn't built — so until then this is the next rung of the map's own
 * fallback chain, "the last-tuned channel, else the first channel, else hide the entry": the
 * lowest-numbered channel a viewer can actually open, the same `inAppPlayable` + number-sort
 * convention `useChannelTuner`'s surfable catalog already uses. `undefined` when none exist,
 * which hides Watch's slot entirely rather than holding a gap.
 */
const firstWatchableChannelId = (channels: ChannelDTO[]): string | undefined =>
  channels
    .filter((channel) => channel.inAppPlayable)
    .sort((a, b) => a.number - b.number || a.id.localeCompare(b.id))[0]?.id;

export { firstWatchableChannelId };
