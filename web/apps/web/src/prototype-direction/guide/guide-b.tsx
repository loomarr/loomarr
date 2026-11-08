// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variant B — colour blocks plus a "now" strip. The strip above the grid names what is on every
// channel, so nobody has to read inside the grid.
import { formatGuideTimeRange, type GuideLayout } from "@loomarr/core/guide";
import { useState } from "react";
import { eraTint, type Tint } from "../shared";
import { blockWords, ColourGrid, type Sel } from "./colour-grid";
import { Segmented, type VariantProps, ZOOM_OPTIONS } from "./controls";
import { channelNow, minutesText, NOW_MS } from "./data";

interface StripProps {
  device: "desktop" | "phone";
  eras: VariantProps["eras"];
  layout: GuideLayout;
  onPick: (sel: Sel) => void;
  selected: Sel | undefined;
  tint: Tint;
}

const NowStrip = ({ device, eras, layout, onPick, selected, tint }: StripProps) => (
  <section aria-label="On now" className="flex flex-col gap-2">
    <h2 className="text-static-400 text-xs font-medium">On now</h2>
    <div
      className={`flex gap-2 pb-1 ${device === "phone" ? "-mx-3 overflow-x-auto px-3" : ""}`}
      // a phone strip scrolls sideways; the desktop one shares the width between the channels
    >
      {layout.channels.map((channel) => {
        const id = channel.source.channelId;
        const now = channelNow(channel.source, NOW_MS);
        const colour = eraTint(eras[id], tint);
        const on = selected?.channelId === id;
        return (
          <button
            key={id}
            type="button"
            onClick={() => onPick({ blockId: now.airing?.scheduleBlockId, channelId: id })}
            className={`bg-card border-static-700 relative flex min-w-0 snap-start flex-col gap-1 overflow-hidden rounded-lg border p-2.5 text-left focus-visible:outline-none ${
              device === "phone" ? "w-36 shrink-0" : "flex-1"
            } ${on ? "ring-signal ring-2" : "focus-visible:ring-signal focus-visible:ring-2"}`}
          >
            <span
              aria-hidden
              className="absolute inset-x-0 top-0 h-[3px]"
              style={{ background: colour ?? "transparent" }}
            />
            <span className="text-static-400 flex min-w-0 items-baseline gap-1.5 text-xs font-medium">
              <span className="font-mono font-semibold">{channel.source.number}</span>
              <span className="truncate">{channel.source.name}</span>
            </span>
            <span
              className={`line-clamp-2 text-sm leading-snug font-semibold ${
                now.kind === "show" ? "text-static-100" : "text-static-400"
              }`}
              style={{ minHeight: "2.5em" }}
            >
              {now.title}
            </span>
            <span className="text-static-400 text-xs">
              {now.kind === "show"
                ? `${minutesText(now.minutesLeft)} left`
                : now.kind === "break"
                  ? `Back in ${minutesText(now.minutesLeft)}`
                  : "Commercials meanwhile"}
            </span>
            {now.kind === "show" ? (
              <span aria-hidden className="bg-static-700 h-[3px] rounded-full">
                <span
                  className="bg-static-100 block h-full rounded-full"
                  style={{ width: `${now.progress * 100}%` }}
                />
              </span>
            ) : (
              <span aria-hidden className="h-[3px]" />
            )}
          </button>
        );
      })}
    </div>
  </section>
);

// The chosen (or hovered) block, said in full. Watch is the one amber thing.
const DetailBar = ({ layout, sel }: { layout: GuideLayout; sel: Sel | undefined }) => {
  const words = sel ? blockWords(layout, sel) : undefined;
  return (
    <div className="bg-card border-static-700 flex min-h-16 shrink-0 items-center gap-3 rounded-lg border px-3 py-2">
      {words ? (
        <>
          <div className="min-w-0 flex-1">
            <p className="text-static-100 truncate text-sm font-semibold">{words.title}</p>
            <p className="text-static-400 line-clamp-2 text-xs">{words.meta}</p>
          </div>
          {words.watch ? (
            <button
              type="button"
              className="bg-signal text-static-950 h-9 shrink-0 rounded-md px-4 text-sm font-semibold"
            >
              Watch
            </button>
          ) : null}
        </>
      ) : (
        <p className="text-static-400 text-sm">Tap or hover any block to see what it is.</p>
      )}
    </div>
  );
};

const GuideB = ({ device, eras, layout, onZoom, tint, zoom }: VariantProps) => {
  const [selected, setSelected] = useState<Sel | undefined>();
  const [hover, setHover] = useState<Sel | undefined>();
  const phone = device === "phone";
  return (
    <div className={`flex h-full flex-col gap-3 ${phone ? "px-3 pt-4 pb-3" : "px-6 pt-5 pb-4"}`}>
      <header className="flex items-end justify-between gap-3">
        <div>
          <h1 className="text-static-100 text-xl font-semibold">Guide</h1>
          <p className="text-static-400 text-xs">
            Tonight, {formatGuideTimeRange(layout.fromMs, layout.toMs, layout.timezone)}
          </p>
        </div>
        <Segmented label="Time shown" options={ZOOM_OPTIONS} value={zoom} onChange={onZoom} />
      </header>
      <NowStrip
        device={device}
        eras={eras}
        layout={layout}
        onPick={setSelected}
        selected={selected}
        tint={tint}
      />
      <ColourGrid
        device={device}
        eras={eras}
        layout={layout}
        onHover={setHover}
        onSelect={setSelected}
        selected={hover ?? selected}
        tint={tint}
      />
      <DetailBar layout={layout} sel={hover ?? selected} />
    </div>
  );
};

export { DetailBar, GuideB };
