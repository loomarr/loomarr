type ChannelSuggestPanelProps = {
  // Called after an approved proposal creates a channel, with the new channel id, so the
  // Guide can navigate to it. Approval is admin-gated server-side (§7); the panel only shows
  // the approve control to admins.
  onCreated: (channelId: string) => void;
  // Seeds the describe form. The wizard's guided first channel hands off with `?intent=`
  // (§13's blank-page killer), which the Guide forwards here. It is a STARTING POINT, not a
  // controlled value — the operator edits it before anything is generated.
  initialIntent?: string;
  className?: string;
};

export type { ChannelSuggestPanelProps };
