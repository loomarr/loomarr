interface ChannelIdentProps {
  name: string;
  number: number;
  // The channel's icon URL, when it has one. Absent falls back to a derived monogram — every
  // channel gets an identity, because a rail of unlabelled rows is harder to scan than one
  // with even a crude mark.
  logo?: string;
  // Pixel size; the grid scales this with zoom.
  size?: number;
  className?: string;
}

export type { ChannelIdentProps };
