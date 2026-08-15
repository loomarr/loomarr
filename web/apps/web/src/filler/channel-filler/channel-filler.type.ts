import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";

type ChannelFillerProps = {
  channelId: string;
  // The channel's whole saved policy — the draft seeds from `policy.filler`, and apply
  // merges the draft back onto this (PATCH replaces policy whole; see the hook).
  policy: ChannelPolicy;
  className?: string;
};

export type { ChannelFillerProps };
