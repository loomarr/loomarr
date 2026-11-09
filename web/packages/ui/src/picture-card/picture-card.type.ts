import type { ReactNode } from "react";

/** The fixed media ratios of the picture card: a programme still, a poster, a square clip frame. */
type PictureRatio = "1/1" | "2/3" | "16/9";

interface PictureCardProps {
  /** The one action at the trailing edge: a `CardAction`, or a link composed onto one. */
  action?: ReactNode;
  /** The ident chip above the title: a `ChannelChip`, or nothing when the item has no channel. */
  chip?: ReactNode;
  /** A readout in the picture's corner (a clip's length). Mono. */
  corner?: string;
  /** The picture. It fills a box of the card's ratio and crops to it. */
  media: ReactNode;
  /** The one metadata line: parts joined by a middle dot. A string is quiet text; a node is shown as is. */
  meta: ReactNode[];
  /** `top` is a bordered card in a grid; `left` is a row inside a hairline-divided `ListGroup`. */
  orientation: "left" | "top";
  ratio: PictureRatio;
  /** A string, or a button when the name opens the details because the one action is something else. */
  title: ReactNode;
}

export type { PictureCardProps, PictureRatio };
