// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// The time grid of variant B (and C's "full grid"): blocks carry colour, not text, below a width
// threshold; the title shows on the current block and on hover; a break is a neutral hatch; a
// downloading channel is one long block with one sentence.
import { formatGuideTime, formatGuideTimeRange, type GuideLayout } from "@loomarr/core/guide";
import { type CSSProperties, useLayoutEffect, useRef, useState } from "react";
import { eraTint, type Tint } from "../shared";
import { channelNow, DOWNLOADING_SENTENCE, minutesText, NOW_MS } from "./data";

interface Sel {
  blockId?: string;
  channelId: string;
}

const useWidth = () => {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const observer = new ResizeObserver(() => setWidth(el.clientWidth));
    observer.observe(el);
    setWidth(el.clientWidth);
    return () => observer.disconnect();
  }, []);
  return [ref, width] as const;
};

const HATCH = {
  backgroundImage: "repeating-linear-gradient(135deg, var(--color-static-700) 0 2px, transparent 2px 7px)",
} as const;

// The words a block says when it is chosen: one source for the footer, hover and screen readers.
const blockWords = (
  layout: GuideLayout,
  sel: Sel,
): { meta: string; title: string; watch: boolean } | undefined => {
  const channel = layout.channels.find((c) => c.source.channelId === sel.channelId)?.source;
  if (!channel) return undefined;
  const now = channelNow(channel, NOW_MS);
  if (channel.status === "building")
    return {
      meta: `${channel.name} · ${DOWNLOADING_SENTENCE}`,
      title: "Getting its shows ready",
      watch: false,
    };
  const airing = channel.airings.find((a) => a.scheduleBlockId === sel.blockId) ?? now.airing;
  if (!airing) return undefined;
  const range = formatGuideTimeRange(airing.startMs, airing.stopMs, layout.timezone);
  const live = airing.startMs <= NOW_MS && NOW_MS < airing.stopMs;
  const when = live
    ? `${minutesText(Math.ceil((airing.stopMs - NOW_MS) / 60_000))} left`
    : airing.startMs > NOW_MS
      ? `in ${minutesText(Math.ceil((airing.startMs - NOW_MS) / 60_000))}`
      : "finished";
  const title =
    airing.kind === "program"
      ? airing.series
        ? `${airing.series} · ${airing.title}`
        : airing.title
      : "Commercial break";
  return { meta: `${channel.name} · ${range} · ${when}`, title, watch: live };
};

interface GridProps {
  device: "desktop" | "phone";
  eras: Record<string, "1970s" | "1980s" | "1990s" | undefined>;
  layout: GuideLayout;
  onHover: (sel: Sel | undefined) => void;
  onSelect: (sel: Sel) => void;
  selected: Sel | undefined;
  tint: Tint;
}

const ColourGrid = ({ device, eras, layout, onHover, onSelect, selected, tint }: GridProps) => {
  const phone = device === "phone";
  const railW = phone ? 112 : 240;
  const rowH = phone ? 48 : 56;
  const [ref, timelineW] = useWidth();
  const span = layout.toMs - layout.fromMs;
  const nowRatio = (NOW_MS - layout.fromMs) / span;

  // Ticks: every half hour when there is room for them, else every hour, aligned to the clock.
  const step = (timelineW >= 640 ? 30 : 60) * 60_000;
  const ticks: number[] = [];
  for (let t = Math.ceil(layout.fromMs / step) * step; t < layout.toMs; t += step) ticks.push(t);
  const ratioOf = (t: number) => (t - layout.fromMs) / span;

  const nowLabel = formatGuideTime(NOW_MS, layout.timezone);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex" style={{ paddingLeft: railW }}>
        <div ref={ref} className="relative h-7 flex-1">
          {ticks.map((t) => {
            const r = ratioOf(t);
            const tooCloseToNow = Math.abs(r - nowRatio) * timelineW < 56;
            return tooCloseToNow ? null : (
              <span
                key={t}
                className="text-static-400 absolute top-1 font-mono text-xs font-medium"
                style={{ left: `${r * 100}%`, transform: "translateX(-4px)" }}
              >
                {formatGuideTime(t, layout.timezone)}
              </span>
            );
          })}
          <span
            className="bg-static-100 text-static-950 absolute top-0.5 z-20 rounded-full px-2 py-0.5 font-mono text-xs font-semibold whitespace-nowrap"
            style={{ left: `${nowRatio * 100}%`, transform: "translateX(-50%)" }}
          >
            {nowLabel}
          </span>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="relative flex flex-col gap-1">
          {/* faint hour lines and the now line sit over the whole body, under the blocks' focus rings */}
          <div className="pointer-events-none absolute inset-y-0 right-0" style={{ left: railW }}>
            {ticks.map((t) => (
              <span
                key={t}
                className="bg-static-800 absolute inset-y-0 w-px"
                style={{ left: `${ratioOf(t) * 100}%` }}
              />
            ))}
          </div>
          <div className="pointer-events-none absolute inset-y-0 right-0 z-30" style={{ left: railW }}>
            <span
              className="bg-static-100/50 absolute inset-y-0 w-px"
              style={{ left: `${nowRatio * 100}%` }}
            />
          </div>
          {layout.channels.map((channel) => {
            const id = channel.source.channelId;
            const downloading = channel.source.status === "building";
            const tintColour = eraTint(eras[id], tint);
            const rowSelected = selected?.channelId === id;
            return (
              <div key={id} className="flex" style={{ height: rowH }}>
                <div className="flex shrink-0 items-center gap-2 pr-2" style={{ width: railW }}>
                  <span
                    aria-hidden
                    className="h-6 w-1 shrink-0 rounded-full"
                    style={{ background: tintColour ?? "transparent" }}
                  />
                  {phone ? (
                    <span className="text-static-100 line-clamp-2 min-w-0 text-xs leading-4 font-semibold">
                      <span className="text-static-400 mr-1.5 font-mono">{channel.source.number}</span>
                      {channel.source.name}
                    </span>
                  ) : (
                    <>
                      <span className="text-static-400 w-6 shrink-0 text-right font-mono text-xs font-semibold">
                        {channel.source.number}
                      </span>
                      <span className="text-static-100 min-w-0 text-sm leading-tight font-semibold">
                        {channel.source.name}
                      </span>
                    </>
                  )}
                </div>
                <div className="relative flex-1">
                  {downloading ? (
                    <button
                      type="button"
                      aria-label={`${channel.source.name}. ${DOWNLOADING_SENTENCE}`}
                      onClick={() => onSelect({ channelId: id })}
                      onMouseEnter={() => onHover({ channelId: id })}
                      onMouseLeave={() => onHover(undefined)}
                      className={`bg-static-900 border-static-500 text-static-400 absolute inset-x-0 inset-y-0.5 z-40 flex items-center rounded-md border border-dashed px-3 text-left text-xs leading-tight ${
                        rowSelected ? "ring-signal ring-2" : ""
                      }`}
                    >
                      {DOWNLOADING_SENTENCE}
                    </button>
                  ) : (
                    channel.airings.map((airing) => {
                      const widthPx = airing.widthRatio * timelineW;
                      const program = airing.source.kind === "program";
                      const isSelected = rowSelected && selected?.blockId === airing.scheduleBlockId;
                      const label = program ? airing.source.title : "";
                      const showLabel = program && (widthPx >= 120 || (airing.isOnNow && widthPx >= 96));
                      const nowBlock = airing.isOnNow && program;
                      let cls: string;
                      let style: CSSProperties = {};
                      if (!program) {
                        cls = "bg-static-900";
                        style = HATCH;
                      } else if (tintColour) {
                        cls = nowBlock ? "border border-static-100/70" : "";
                        style = {
                          background: `color-mix(in srgb, ${tintColour} ${nowBlock ? 55 : 40}%, transparent)`,
                        };
                      } else {
                        cls = nowBlock ? "bg-static-700 border border-static-500" : "bg-static-800";
                      }
                      const range = formatGuideTimeRange(
                        airing.source.startMs,
                        airing.source.stopMs,
                        layout.timezone,
                      );
                      return (
                        <button
                          key={airing.scheduleBlockId}
                          type="button"
                          aria-label={`${channel.source.name}, ${program ? airing.source.title : "Commercial break"}, ${range}`}
                          onClick={() => onSelect({ blockId: airing.scheduleBlockId, channelId: id })}
                          onMouseEnter={() => onHover({ blockId: airing.scheduleBlockId, channelId: id })}
                          onMouseLeave={() => onHover(undefined)}
                          onFocus={() => onHover({ blockId: airing.scheduleBlockId, channelId: id })}
                          onBlur={() => onHover(undefined)}
                          className={`focus-visible:ring-signal absolute inset-y-0.5 overflow-hidden rounded-sm text-left focus-visible:ring-2 focus-visible:outline-none ${cls} ${
                            isSelected ? "ring-signal z-20 ring-2" : ""
                          }`}
                          style={{
                            ...style,
                            left: `calc(${airing.startRatio * 100}% + 1px)`,
                            width: `max(3px, calc(${airing.widthRatio * 100}% - 2px))`,
                          }}
                        >
                          {showLabel ? (
                            <span className="text-static-100 line-clamp-2 px-2 text-xs leading-4 font-semibold">
                              {label}
                            </span>
                          ) : null}
                          {nowBlock ? (
                            <span
                              aria-hidden
                              className="bg-static-100/25 absolute inset-x-0 bottom-0 h-[3px]"
                            >
                              <span
                                className="bg-static-100 block h-full"
                                style={{
                                  width: `${Math.min(100, Math.max(0, ((NOW_MS - airing.source.startMs) / (airing.source.stopMs - airing.source.startMs)) * 100))}%`,
                                }}
                              />
                            </span>
                          ) : null}
                        </button>
                      );
                    })
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
};

export type { Sel };
export { blockWords, ColourGrid };
