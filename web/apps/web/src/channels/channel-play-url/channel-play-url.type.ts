type ChannelPlaySource = {
  url: string;
  expiresAt: number;
  /** The channel's current frame, signed like the stream. Absent when the server sent none. */
  stillURL?: string;
};

export type { ChannelPlaySource };
