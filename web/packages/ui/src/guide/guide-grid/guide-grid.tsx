import {
  type GuideNavigationDirection,
  guideChannelForTypeahead,
  guideSelectionForChannel,
} from "@loomarr/core/guide";
import { PreviewGroup, useWindowWidth } from "@loomarr/design-system";
import { useEffect, useRef, useState } from "react";

import { guideRailWidth } from "../guide-rail";
import { selectionOf } from "./guide-blocks";
import type { GuideGridProps } from "./guide-grid.type";
import { GuideRow, ROW } from "./guide-row";
import { GuideRows } from "./guide-rows";
import { GuideRuler, RULER } from "./guide-ruler";

// The web mock's time grid (#1659, decision N7): a 200–280 px channel column beside a timeline of
// blocks, one 56 px row per channel. GuideRows virtualises the rows per platform. The keyboard
// model is a roving tabindex: the selected block is the grid's one Tab stop, arrow keys move it
// through the guide controller, and typing a channel number or name jumps to that channel.

const arrowDirection: Record<string, GuideNavigationDirection> = {
  ArrowDown: "down",
  ArrowLeft: "left",
  ArrowRight: "right",
  ArrowUp: "up",
};
// Keys typed within this gap build one query ("1", "12"); a longer pause starts a new one.
const TYPEAHEAD_MS = 800;

const GuideGrid = ({
  focusRequest,
  layout,
  nowMs,
  onMove,
  onOpenChannel,
  onOpenChannelInfo,
  onSelect,
  renderRowMenu,
  renderPreview,
  selection,
}: GuideGridProps) => {
  const windowWidth = useWindowWidth();
  const railWidth = guideRailWidth(windowWidth);
  const [timelineWidth, setTimelineWidth] = useState(0);
  const focusPending = useRef(false);
  // A host's request for focus to come home (a retry took the control focus was on): the Tab stop
  // takes it once the blocks have seen the tick.
  const [focusTick, setFocusTick] = useState(0);
  useEffect(() => {
    if (!focusRequest) return;
    focusPending.current = true;
    setFocusTick((tick) => tick + 1);
  }, [focusRequest]);
  const typed = useRef({ at: 0, query: "" });
  const span = Math.max(1, layout.toMs - layout.fromMs);
  const nowRatio = nowMs >= layout.fromMs && nowMs < layout.toMs ? (nowMs - layout.fromMs) / span : undefined;

  const ruler = (
    <GuideRuler
      layout={layout}
      nowMs={nowMs}
      nowRatio={nowRatio}
      onTimelineWidth={setTimelineWidth}
      railWidth={railWidth}
      span={span}
      timelineWidth={timelineWidth}
      windowWidth={windowWidth}
    />
  );

  // One Tab stop: the selected block, else the first block of the first row.
  const tabStop = selection
    ? selection
    : layout.channels[0]?.airings[0]
      ? selectionOf(layout.channels[0].airings[0])
      : undefined;
  const tabStopRow = tabStop
    ? layout.channels.findIndex((channel) => channel.source.channelId === tabStop.channelId)
    : -1;

  const onKey = (key: string, modified: boolean): boolean => {
    if (modified || !tabStop) return false;
    const direction = arrowDirection[key];
    if (direction) {
      const moved = onMove?.(direction);
      if (moved && !moved.boundary) focusPending.current = true;
      return true;
    }
    if (key.length !== 1 || !/\S/.test(key)) return false;
    const at = Date.now();
    const query = at - typed.current.at < TYPEAHEAD_MS ? typed.current.query + key : key;
    typed.current = { at, query };
    const channelId = guideChannelForTypeahead(layout, query, tabStop.channelId);
    const next = channelId ? guideSelectionForChannel(layout, channelId, tabStop.anchorMs) : undefined;
    if (next && next.scheduleBlockId !== tabStop.scheduleBlockId) {
      focusPending.current = true;
      onSelect?.(next);
    }
    return true;
  };

  return (
    <PreviewGroup resetKey={layout.source}>
      <GuideRows
        channels={layout.channels}
        focusIndex={tabStopRow < 0 ? undefined : tabStopRow}
        header={ruler}
        headerHeight={RULER + 1}
        onKey={onKey}
        renderRow={(channel) => (
          <GuideRow
            channel={channel}
            focusPending={focusPending}
            focusTick={focusTick}
            nowRatio={nowRatio}
            onOpenChannel={onOpenChannel}
            onOpenChannelInfo={onOpenChannelInfo ?? onOpenChannel}
            onSelect={onSelect}
            railWidth={railWidth}
            renderRowMenu={renderRowMenu}
            renderPreview={renderPreview}
            tabStop={channel.source.channelId === tabStop?.channelId ? tabStop.scheduleBlockId : undefined}
            timelineWidth={timelineWidth}
            timezone={layout.timezone}
          />
        )}
        rowHeight={ROW + 1}
      />
    </PreviewGroup>
  );
};

export { GuideGrid };
