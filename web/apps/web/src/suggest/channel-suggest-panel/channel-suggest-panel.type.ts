import type { OriginationFlow } from "../use-origination-flow";

type ChannelSuggestPanelProps = {
  // The origination flow: the run, its search params and the steps between them.
  flow: OriginationFlow;
  // Called after an approved proposal creates a channel, with the new channel id, so the
  // page can navigate to it. Approval is admin-gated server-side (§7); the panel only shows
  // the approve control to admins.
  onCreated: (channelId: string) => void;
  className?: string;
};

export type { ChannelSuggestPanelProps };
