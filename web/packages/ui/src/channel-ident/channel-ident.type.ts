interface ChannelIdentProps {
  name: string;
  number: number;
  // The channel's icon URL, when it has one. Absent falls back to the derived monogram.
  logo?: string;
  // Edge length in px. The guide row draws 30; the channel page's header 48.
  size?: number;
}

export type { ChannelIdentProps };
