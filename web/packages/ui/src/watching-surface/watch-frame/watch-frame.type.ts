import type { ReactNode } from "react";

interface WatchFrameProps {
  children: ReactNode;
  /** The page's heading, read by assistive tech only: the player's own top bar names the channel. */
  title: string;
}

export type { WatchFrameProps };
