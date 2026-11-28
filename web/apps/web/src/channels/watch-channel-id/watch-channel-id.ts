import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ContinueWatchingDTO } from "@loomarr/api/models/continueWatchingDTO";
import { createChannelLineup } from "@loomarr/core/channel-lineup";

/**
 * Decision X2 (#1817 item 3, #1659 Shell, maintainer-clarified 2026-10-03): Watch's target, using
 * #1662/#1709's continue-watching. It is the lineup's landing channel: the caller's own last-tuned
 * channel (`continueWatching`, from `GET /v1/household/viewing`), else the lowest-numbered playable
 * one. `undefined` with no playable channel at all, which hides Watch's nav slot entirely rather
 * than holding a gap.
 *
 * One function for both the desktop rail and the phone bottom bar (#1849).
 */
const watchChannelId = (
  channels: ChannelDTO[],
  continueWatching: ContinueWatchingDTO | undefined,
): string | undefined =>
  createChannelLineup({
    channels: channels.map((channel) => ({ ...channel, playable: channel.inAppPlayable })),
    lastTunedId: continueWatching?.channelId,
  }).landing?.id;

export { watchChannelId };
