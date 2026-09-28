import type {
  GuideLayout,
  GuideNavigationDirection,
  GuideNavigationResult,
  GuideSelection,
} from "@loomarr/core/guide";

interface GuideGridProps {
  layout: GuideLayout;
  /** The clock the layout was computed against; draws the now line and its label. */
  nowMs: number;
  /**
   * An arrow key asks to move the selection. Wire it to the guide controller's `move`; a result
   * without a boundary moves focus to the new block.
   */
  onMove?: (direction: GuideNavigationDirection) => GuideNavigationResult | undefined;
  /** Opens a channel from its row or one of its blocks. */
  onOpenChannel?: (channelId: string) => void;
  /** A block took focus, or type-to-jump chose a channel. Wire it to the controller's `select`. */
  onSelect?: (selection: GuideSelection) => void;
  /** The selected block: the grid's one Tab stop. Without one, the first block is. */
  selection?: GuideSelection;
}

export type { GuideGridProps };
