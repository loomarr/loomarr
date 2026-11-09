import type { ReactElement } from "react";

interface NeedsLineItem {
  id: string;
  /** The number as shown: a count ("3"), or the filler quota as a share ("91%"). */
  value: string;
  /** What the number is, in a person's words, already matched to it: "requests to approve". */
  label: string;
  /** The short form a phone shows: "to approve". */
  short: string;
  /** A warning rather than a to-do (filler space running short): drawn with a dot. */
  warn?: boolean;
  /** The page that deals with it, as a router link. Its children are replaced by the item. */
  link: ReactElement<Record<string, unknown>>;
}

interface NeedsLineProps {
  /** Only the items that need a look. An empty list renders nothing. */
  items: NeedsLineItem[];
  /** The phone layout: short labels, wrapping onto a second line when needed. */
  compact?: boolean;
}

export type { NeedsLineItem, NeedsLineProps };
