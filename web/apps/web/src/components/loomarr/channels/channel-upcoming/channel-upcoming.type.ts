interface ChannelUpcomingProps {
  channelId: string;
  // Whether the channel is live right now — drives only the "on now" highlight on the first
  // entry. An empty backend response cannot prove broadcast state, so its copy stays neutral.
  live?: boolean;
  className?: string;
}

export type { ChannelUpcomingProps };
