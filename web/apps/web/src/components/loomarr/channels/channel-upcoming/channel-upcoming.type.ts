interface ChannelUpcomingProps {
  channelId: string;
  // Whether the channel is live right now — drives the "on now" highlight on the first entry
  // and the empty copy ("nothing scheduled right now" vs a plain dash for an off channel).
  live?: boolean;
  className?: string;
}

export type { ChannelUpcomingProps };
