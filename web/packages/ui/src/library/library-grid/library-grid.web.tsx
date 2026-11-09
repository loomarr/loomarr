import { defaultRangeExtractor, measureElement, useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useLayoutEffect, useRef, useState } from "react";

import type { LibraryGridProps } from "./library-grid.type";

// A household's library is thousands of titles, and a tile per loaded title slows the page with
// every page loaded: only the rows near the screen are mounted, and each row is measured once
// drawn, so a row is as tall as its posters and captions really are.
const OVERSCAN = 3;
// The next page is asked for once a row this close to the end is mounted.
const END_ROWS = 2;
// Before the page lays out (and always in jsdom, which lays nothing out), the window is this big.
const FALLBACK_RECT = { height: 800, width: 1280 };

// The slice of the DOM the grid reads. This package has no DOM lib (it targets React Native too),
// so the shape is spelled out here.
type DomNode = {
  parentElement: DomNode | null;
  scrollTop: number;
  getBoundingClientRect(): { top: number };
  closest(selector: string): { getAttribute(name: string): string | null } | null;
};
type Observer = { observe(target: unknown): void; disconnect(): void };
const { getComputedStyle, ResizeObserver: Resize } = globalThis as unknown as {
  getComputedStyle(node: DomNode): { overflowY: string };
  ResizeObserver?: new (callback: () => void) => Observer;
};

// The element the grid scrolls in: the nearest ancestor that scrolls (the app shell's main).
const scrollerOf = (node: DomNode | null): DomNode | null => {
  for (let el = node?.parentElement; el; el = el.parentElement) {
    const { overflowY } = getComputedStyle(el);
    if (overflowY === "auto" || overflowY === "scroll") return el;
  }
  return null;
};

// How far down the scroller's content the grid starts: the heading, status line, needs and tabs
// sit above it in the same scroller.
const offsetIn = (list: DomNode, scroller: DomNode) =>
  list.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop;

const LibraryGrid = <Row,>({
  rows,
  rowKey,
  renderRow,
  estimateRowHeight,
  onEndReached,
  label,
}: LibraryGridProps<Row>) => {
  const list = useRef<HTMLDivElement>(null);
  const [scroller, setScroller] = useState<DomNode | null>(null);
  const [margin, setMargin] = useState(0);
  // The row holding keyboard focus, by key, stays mounted while it scrolls away, so focus isn't
  // lost; a key still names it when new titles or a new column count reshape the rows.
  const [focusedKey, setFocusedKey] = useState<string>();
  // Bumped when what sits above the grid changes height on its own (Needs you opening or an
  // answered row leaving), so the offset below is read again.
  const [, setResized] = useState(0);

  // Read after every render: the scroller, and how far down it the grid starts.
  useLayoutEffect(() => {
    const node = list.current as DomNode | null;
    const found = scrollerOf(node);
    if (found !== scroller) setScroller(found);
    const offset = node && found ? Math.round(offsetIn(node, found)) : 0;
    if (offset !== margin) setMargin(offset);
  });

  // The grid's parent holds what sits above it: any change in its height is re-read.
  useEffect(() => {
    const parent = (list.current as DomNode | null)?.parentElement;
    if (!parent || !Resize) return;
    const observer = new Resize(() => setResized((n) => n + 1));
    observer.observe(parent);
    return () => observer.disconnect();
  }, []);

  const focusedRow =
    focusedKey === undefined ? -1 : rows.findIndex((row, index) => rowKey(row, index) === focusedKey);

  const virtual = useVirtualizer({
    count: rows.length,
    estimateSize: () => estimateRowHeight,
    getItemKey: (index) => {
      const row = rows[index];
      return row === undefined ? index : rowKey(row, index);
    },
    getScrollElement: () => scroller as HTMLElement | null,
    initialRect: FALLBACK_RECT,
    // A drawn row is never 0 px tall in a browser; jsdom reports 0 for everything, which would
    // mount every row, so a 0 keeps the estimate and tests window the rows as a browser does.
    measureElement: (element, entry, instance) =>
      measureElement(element, entry, instance) || estimateRowHeight,
    overscan: OVERSCAN,
    rangeExtractor: (range) => {
      const indexes = defaultRangeExtractor(range);
      return focusedRow < 0 || indexes.includes(focusedRow)
        ? indexes
        : [...indexes, focusedRow].sort((a, b) => a - b);
    },
    scrollMargin: margin,
  });
  const items = virtual.getVirtualItems();

  const last = items.at(-1)?.index;
  useEffect(() => {
    if (last !== undefined && last >= rows.length - 1 - END_ROWS) onEndReached?.();
  }, [last, rows.length, onEndReached]);

  return (
    // biome-ignore lint/a11y/useSemanticElements: a <ul> may hold only <li>; the measured row boxes between it and the titles (role="none") are divs.
    <div
      aria-label={label}
      onBlur={() => setFocusedKey(undefined)}
      onFocus={(event) => {
        const key = (event.target as unknown as DomNode).closest("[data-key]")?.getAttribute("data-key");
        setFocusedKey(key ?? undefined);
      }}
      ref={list}
      role="list"
      style={{ height: virtual.getTotalSize(), position: "relative", width: "100%" }}
    >
      {items.map((item) => {
        const row = rows[item.index];
        return row === undefined ? null : (
          <div
            data-index={item.index}
            data-key={String(item.key)}
            key={item.key}
            ref={virtual.measureElement}
            role="none"
            style={{
              left: 0,
              position: "absolute",
              right: 0,
              top: 0,
              transform: `translateY(${item.start - margin}px)`,
            }}
          >
            {renderRow(row)}
          </div>
        );
      })}
    </div>
  );
};

export { LibraryGrid };
