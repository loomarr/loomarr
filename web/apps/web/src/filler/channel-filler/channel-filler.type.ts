import type { ChannelPolicy } from "@loomarr/api";

type ChannelFillerProps = {
  channelId: string;
  // The channel's whole saved policy — the draft seeds from `policy.filler`, and apply
  // merges the draft back onto this (PATCH replaces policy whole; see the hook).
  policy: ChannelPolicy;
  // Controlled open state for the section's collapsible (so the channel page's nav menu can
  // open it). Optional — omitted, it manages its own open state (starts closed).
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  className?: string;
};

export type { ChannelFillerProps };
