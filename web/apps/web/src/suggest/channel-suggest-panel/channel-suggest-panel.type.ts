import type { Intent } from "@loomarr/api/models/intent";

type ChannelSuggestPanelProps = {
  // Called after an approved proposal creates a channel, with the new channel id, so the
  // Guide can navigate to it. Approval is admin-gated server-side (§7); the panel only shows
  // the approve control to admins.
  onCreated: (channelId: string) => void;
  // Seeds the describe form with either a resolved preset or a legacy free-text Intent. It is
  // a STARTING POINT, not a controlled value — the operator edits it before generation.
  initialIntent?: Intent;
  // Controlled by Guide search so a reload restores this exact execution.
  jobId?: string;
  onJobIdChange?: (jobId: string | undefined) => void;
  className?: string;
};

export type { ChannelSuggestPanelProps };
