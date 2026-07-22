type ChannelSuggestPanelProps = {
  // Called after an approved proposal creates a channel, with the new channel id, so the
  // Channels list can navigate to it. Approval is admin-gated server-side (§7); the panel
  // only shows the approve control to admins.
  onCreated: (channelId: string) => void;
  className?: string;
};

export type { ChannelSuggestPanelProps };
