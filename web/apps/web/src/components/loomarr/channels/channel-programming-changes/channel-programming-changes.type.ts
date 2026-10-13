import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";

interface ChannelProgrammingChangesProps {
  channelId: string;
  // The unsaved policy to compare against the saved channel. Omitted means nothing is
  // unsaved, so there is nothing to compare and no request is sent.
  draftPolicy?: ChannelPolicy;
  // The household guide timezone, so slot times read as the Guide shows them.
  timeZone?: string;
  className?: string;
}

export type { ChannelProgrammingChangesProps };
