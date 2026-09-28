import type { GuideLayout } from "@loomarr/core/guide";

interface GuideGridProps {
  layout: GuideLayout;
  /** The clock the layout was computed against; draws the now line and its label. */
  nowMs: number;
  /** Opens a channel from its row or one of its blocks. */
  onOpenChannel?: (channelId: string) => void;
}

export type { GuideGridProps };
