import { observeElementRect, useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useRef } from "react";

import type { GuideRowsProps } from "./guide-rows.type";

// react-native-web's FlatList failed the 100-row measurement on #1705. Its default window mounts
// every row in 10-row batches, and on a 4x-throttled CPU the late batches stalled scrolling for
// 217 ms (383 ms at 6x); a window small enough to virtualise stalled at 1x instead. react-virtual
// mounts only the rows entering the viewport each frame, so web uses it.
const OVERSCAN = 6;
const FALLBACK_RECT = { height: 800, width: 1280 };

const GuideRows = ({
  channels,
  focusIndex,
  header,
  headerHeight,
  onKey,
  renderRow,
  rowHeight,
}: GuideRowsProps) => {
  const scroller = useRef<HTMLDivElement>(null);
  const rows = useVirtualizer({
    count: channels.length,
    estimateSize: () => rowHeight,
    getItemKey: (index) => channels[index]?.source.channelId ?? index,
    getScrollElement: () => scroller.current,
    initialRect: FALLBACK_RECT,
    // jsdom measures every element as 0×0, and a zero-height viewport mounts no rows, so unit
    // tests would assert against an empty grid. A zero measurement falls back to a usable
    // rect; a real browser never reports one, so the app takes the same path as the tests.
    observeElementRect: (instance, report) =>
      observeElementRect(instance, (rect) =>
        report({
          height: rect.height || FALLBACK_RECT.height,
          width: rect.width || FALLBACK_RECT.width,
        }),
      ),
    overscan: OVERSCAN,
    // The ruler sits above the rows inside the same scroller, and stays pinned over them.
    scrollMargin: headerHeight,
    scrollPaddingStart: headerHeight,
  });

  useEffect(() => {
    if (focusIndex !== undefined && focusIndex < channels.length) rows.scrollToIndex(focusIndex);
  }, [channels.length, focusIndex, rows]);

  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: keys bubble up from the focused block.
    <div
      onKeyDown={(event) => {
        if (onKey?.(event.key, event.altKey || event.ctrlKey || event.metaKey)) event.preventDefault();
      }}
      ref={scroller}
      // Keep a readable timeline at narrow desktop widths; scroll inside the Guide.
      style={{ flex: 1, minHeight: 0, overflow: "auto" }}
    >
      <div style={{ minWidth: 900, position: "sticky", top: 0, zIndex: 1 }}>{header}</div>
      <div style={{ height: rows.getTotalSize(), minWidth: 900, position: "relative" }}>
        {rows.getVirtualItems().map((row) => {
          const channel = channels[row.index];
          return channel ? (
            <div
              key={row.key}
              style={{
                height: rowHeight,
                left: 0,
                position: "absolute",
                right: 0,
                top: row.start - headerHeight,
              }}
            >
              {renderRow(channel)}
            </div>
          ) : null;
        })}
      </div>
    </div>
  );
};

export { GuideRows };
