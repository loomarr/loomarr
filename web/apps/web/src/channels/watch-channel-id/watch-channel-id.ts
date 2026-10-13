import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ContinueWatchingDTO } from "@loomarr/api/models/continueWatchingDTO";
import { firstWatchableChannelId } from "@/channels/first-watchable-channel";

/**
 * Decision X2 (#1817 item 3, #1659 Shell, maintainer-clarified 2026-10-03): Watch's target, now
 * that #1662/#1709's continue-watching exists. The caller's own last-tuned channel
 * (`continueWatching`, from `GET /v1/household/viewing`) wins when it still names a channel this
 * viewer can open; otherwise this falls back to `firstWatchableChannelId`'s lowest-numbered
 * playable channel — the state before anyone has tuned at all. `undefined` with no playable
 * channel at all, which hides Watch's nav slot entirely rather than holding a gap.
 *
 * One function for both the desktop rail and the phone bottom bar (#1849): neither shell computes
 * its own fallback chain.
 */
const watchChannelId = (
  channels: ChannelDTO[],
  continueWatching: ContinueWatchingDTO | undefined,
): string | undefined => {
  const tuned = continueWatching
    ? channels.find((channel) => channel.id === continueWatching.channelId && channel.inAppPlayable)
    : undefined;
  return tuned?.id ?? firstWatchableChannelId(channels);
};

export { watchChannelId };
