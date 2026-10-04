import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";

interface ChannelDefaultsSummaryProps {
  // The saved policy whose overrides are counted.
  policy: ChannelPolicy;
  className?: string;
}

export type { ChannelDefaultsSummaryProps };
