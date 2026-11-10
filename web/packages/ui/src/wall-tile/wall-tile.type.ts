import type { ReactElement, ReactNode } from "react";

interface WallTileProps {
  /** The picture: fills the tile and crops to 16:9. */
  art: ReactNode;
  /** The channel's name: on the top row beside the number, one line that ends in an ellipsis before any pill. */
  channelName: string;
  /** The compact tile for a phone's two columns: smaller text, the title held to two lines, one detail line. */
  compact?: boolean;
  /** The channel's era decade from the API (1980 for the 1980s): it colours the number block. Absent is neutral. */
  decade?: number;
  /** The channel has nothing to air yet (building, paused, waiting for titles): the picture is dimmed. */
  dim?: boolean;
  /** The channel the viewer tuned last: a ring and a pill ("You were here", "Last" when compact). */
  last?: boolean;
  /** Who is watching this channel now, by name (an admin's view only, #2785): a pill in the top-right corner. The last-tuned tile shows its own pill instead. */
  watchers?: string[];
  /** The channel number as the guide and player print it, unpadded ("7"). */
  number: string;
  onClick?: () => void;
  /** 0 to 100: how far through its programme the channel is, as a thin line along the foot. */
  progressPercent?: number;
  /** Web only: the tile's one destination, as the router's `<Link to=… />`; the default is a button. */
  render?: ReactElement<Record<string, unknown>>;
  /** What is on: the programme, or the channel's own state when nothing is. */
  title: string;
  /** The quiet line under the title ("S2 E4 · 13 min left"). */
  detail?: string;
}

export type { WallTileProps };
