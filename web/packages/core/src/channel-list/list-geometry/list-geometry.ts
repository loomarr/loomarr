/** Where the list sits at rest. */
interface ListRest {
  /** The pinned channel's top, below the top of the list's box. */
  offset: number;
  /** The height of the list's box. */
  height: number;
  /** The scroll position that rests the pinned channel there. */
  scroll: number;
}

interface ListRestArgs {
  /** The height the list may take. */
  viewport: number;
  pinnedHeight: number;
  restOffset: number;
  /** The pinned channel's top within the scrolled content. */
  anchorY: number;
  /** Set to rest on whole rows of this height, as a television does. */
  wholeRows?: number | undefined;
}

const listRest = ({ viewport, pinnedHeight, restOffset, anchorY, wholeRows }: ListRestArgs): ListRest => {
  if (!wholeRows) return { height: viewport, offset: restOffset, scroll: anchorY - restOffset };
  const whole = (space: number) => Math.max(0, Math.floor(space / wholeRows)) * wholeRows;
  const offset = whole(Math.min(restOffset, viewport - pinnedHeight));
  const height = offset + pinnedHeight + whole(viewport - offset - pinnedHeight);
  return { height, offset, scroll: anchorY - offset };
};

/** The positions scrolling stays within: the list's own ends, and always as far as its rest. */
const scrollBounds = ({
  room,
  contentHeight,
  height,
  rest,
}: {
  room: number;
  contentHeight: number;
  height: number;
  rest: number;
}) => ({ max: Math.max(rest, contentHeight - room - height), min: Math.min(rest, room) });

/** How far a view that cannot stick moves the pinned channel to keep it at the edge in view. */
const pinnedShift = ({
  anchorY,
  scroll,
  height,
  pinnedHeight,
}: {
  anchorY: number;
  scroll: number;
  height: number;
  pinnedHeight: number;
}) => {
  if (anchorY < scroll) return scroll - anchorY;
  return Math.min(0, scroll + height - pinnedHeight - anchorY);
};

/**
 * The scroll position that shows a row clear of the pinned channel, which covers the top of the box
 * once the list has passed it and the bottom while the list is short of it.
 */
const scrollToShow = ({
  rowY,
  rowHeight,
  anchorY,
  scroll,
  height,
  pinnedHeight,
}: {
  rowY: number;
  rowHeight: number;
  anchorY: number;
  scroll: number;
  height: number;
  pinnedHeight: number;
}) => {
  const above = rowY < anchorY ? 0 : pinnedHeight;
  const below = rowY > anchorY ? 0 : pinnedHeight;
  if (rowY - above < scroll) return rowY - above;
  if (rowY + rowHeight + below > scroll + height) return rowY + rowHeight + below - height;
  return scroll;
};

export type { ListRest };
export { listRest, pinnedShift, scrollBounds, scrollToShow };
