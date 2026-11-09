import type { ReactElement, ReactNode } from "react";

interface WallTileProps {
  /** The picture: fills the tile and crops to 16:9. */
  art: ReactNode;
  /** The channel's name. Shown on the tile at desktop width; the phone tile folds it into the second line. */
  channelName: string;
  /** The compact tile for a phone's two columns: smaller text, no channel name on the top row. */
  compact?: boolean;
  /** The channel has nothing to air yet (building, paused, waiting for titles): the picture is dimmed. */
  dim?: boolean;
  /** The channel the viewer tuned last: a ring and a pill ("You were here", "Last" when compact). */
  last?: boolean;
  /** The channel number, zero-padded as the guide shows it ("07"). */
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
