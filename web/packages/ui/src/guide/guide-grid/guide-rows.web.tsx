import { useVirtualizer } from "@tanstack/react-virtual";
import { useRef } from "react";

import type { GuideRowsProps } from "./guide-rows.type";

// react-native-web's FlatList failed the 100-row measurement on #1705. Its default window mounts
// every row in 10-row batches, and on a 4x-throttled CPU the late batches stalled scrolling for
// 217 ms (383 ms at 6x); a window small enough to virtualise stalled at 1x instead. react-virtual
// mounts only the rows entering the viewport each frame, so web uses it.
const OVERSCAN = 6;

const GuideRows = ({ channels, header, headerHeight, renderRow, rowHeight }: GuideRowsProps) => {
  const scroller = useRef<HTMLDivElement>(null);
  const rows = useVirtualizer({
    count: channels.length,
    estimateSize: () => rowHeight,
    getItemKey: (index) => channels[index]?.source.channelId ?? index,
    getScrollElement: () => scroller.current,
    overscan: OVERSCAN,
    // The ruler sits above the rows inside the same scroller.
    scrollMargin: headerHeight,
  });

  return (
    <div ref={scroller} style={{ flex: 1, minHeight: 0, minWidth: 900, overflowY: "auto" }}>
      <div style={{ position: "sticky", top: 0, zIndex: 1 }}>{header}</div>
      <div style={{ height: rows.getTotalSize(), position: "relative" }}>
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
