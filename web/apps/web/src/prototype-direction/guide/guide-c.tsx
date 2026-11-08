// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variant C — Now / Next. No time grid on first view: one row per channel saying what is on, how
// far through it is, and what comes next. The full grid is one tap away.
import { formatGuideTime, formatGuideTimeRange } from "@loomarr/core/guide";
import { useState } from "react";
import { eraTint } from "../shared";
import { ColourGrid, type Sel } from "./colour-grid";
import { Segmented, type VariantProps, ZOOM_OPTIONS } from "./controls";
import { channelNow, DOWNLOADING_SENTENCE, minutesText, NOW_MS } from "./data";
import { DetailBar } from "./guide-b";

const VIEWS = [
  { label: "Now & next", value: "list" },
  { label: "Full grid", value: "grid" },
] as const;

const GuideC = ({ device, eras, layout, onZoom, tint, zoom }: VariantProps) => {
  const [view, setView] = useState<"list" | "grid">("list");
  const [selected, setSelected] = useState<Sel | undefined>();
  const [hover, setHover] = useState<Sel | undefined>();
  const phone = device === "phone";
  const tz = layout.timezone;

  return (
    <div className={`flex h-full flex-col gap-3 ${phone ? "px-3 pt-4 pb-3" : "px-6 pt-5 pb-4"}`}>
      <header className="flex items-end justify-between gap-3">
        <div>
          <h1 className="text-static-100 text-xl font-semibold">{view === "list" ? "What's on" : "Guide"}</h1>
          <p className="text-static-400 text-xs">
            {view === "list"
              ? `${formatGuideTime(NOW_MS, tz)} · ${layout.channels.length} channels`
              : `Tonight, ${formatGuideTimeRange(layout.fromMs, layout.toMs, tz)}`}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {view === "grid" && !phone ? (
            <Segmented label="Time shown" options={ZOOM_OPTIONS} value={zoom} onChange={onZoom} />
          ) : null}
          <Segmented label="View" options={VIEWS} value={view} onChange={setView} />
        </div>
      </header>

      {view === "grid" ? (
        <>
          {phone ? (
            <div className="flex justify-end">
              <Segmented label="Time shown" options={ZOOM_OPTIONS} value={zoom} onChange={onZoom} />
            </div>
          ) : null}
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
        </>
      ) : (
        <ul className="flex min-h-0 flex-1 flex-col gap-1.5 overflow-y-auto">
          {phone ? null : (
            <li
              aria-hidden
              className="text-static-400 grid grid-cols-[240px_minmax(0,1.4fr)_minmax(0,1fr)] gap-8 pr-6 pl-4 text-xs font-medium"
            >
              <span>Channel</span>
              <span>On now</span>
              <span className="pl-24">Next</span>
            </li>
          )}
          {layout.channels.map((channel) => {
            const c = channel.source;
            const now = channelNow(c, NOW_MS);
            const colour = eraTint(eras[c.channelId], tint);
            const range =
              now.airing && now.kind === "show"
                ? formatGuideTimeRange(now.airing.startMs, now.airing.stopMs, tz)
                : "";
            const nextText = now.next ? `${formatGuideTime(now.next.startMs, tz)}` : "";
            const nextTitle = now.next ? now.next.title : "";
            const nextLine =
              now.kind === "downloading"
                ? "When its first show arrives"
                : now.next
                  ? nextTitle
                  : "Later tonight";
            const nowTitle = now.series && now.kind === "show" ? `${now.series} · ${now.title}` : now.title;
            return (
              <li key={c.channelId}>
                <button
                  type="button"
                  aria-label={`${c.name}. On now: ${nowTitle}. ${now.next ? `Next at ${nextText}: ${nextTitle}` : ""}`}
                  className="bg-card border-static-700 focus-visible:ring-signal hover:bg-static-800 relative flex w-full gap-3 overflow-hidden rounded-lg border text-left focus-visible:ring-2 focus-visible:outline-none"
                >
                  <span
                    aria-hidden
                    className="w-1 shrink-0"
                    style={{ background: colour ?? "transparent" }}
                  />
                  {phone ? (
                    <span className="flex min-w-0 flex-1 flex-col gap-1 py-2.5 pr-3">
                      <span className="text-static-400 flex items-baseline gap-1.5 text-xs font-medium">
                        <span className="font-mono font-semibold">{c.number}</span>
                        <span className="truncate">{c.name}</span>
                        {now.kind === "show" ? (
                          <span className="ml-auto shrink-0">{minutesText(now.minutesLeft)} left</span>
                        ) : null}
                      </span>
                      <span
                        className={`line-clamp-2 text-sm leading-snug font-semibold ${now.kind === "show" ? "text-static-100" : "text-static-400"}`}
                      >
                        {nowTitle}
                      </span>
                      {now.kind === "downloading" ? (
                        <span className="text-static-400 text-xs">
                          Commercials play until its shows arrive.
                        </span>
                      ) : (
                        <Progress value={now.progress} />
                      )}
                      <span className="text-static-400 flex gap-2 truncate text-xs">
                        <span className="shrink-0 font-mono">{now.next ? nextText : "—"}</span>
                        <span className="truncate">{nextLine}</span>
                      </span>
                    </span>
                  ) : (
                    <span className="grid min-w-0 flex-1 grid-cols-[240px_minmax(0,1.4fr)_minmax(0,1fr)] items-center gap-8 py-4 pr-6">
                      <span className="flex min-w-0 items-baseline gap-3">
                        <span className="text-static-400 w-7 shrink-0 text-right font-mono text-sm font-semibold">
                          {c.number}
                        </span>
                        <span className="text-static-100 min-w-0 text-sm font-semibold">{c.name}</span>
                      </span>
                      <span className="flex min-w-0 flex-col gap-2">
                        <span
                          className={`truncate text-lg leading-tight font-semibold ${now.kind === "show" ? "text-static-100" : "text-static-400"}`}
                        >
                          {nowTitle}
                        </span>
                        {now.kind === "downloading" ? (
                          <span className="text-static-400 truncate text-sm">{DOWNLOADING_SENTENCE}</span>
                        ) : (
                          <span className="flex items-center gap-4">
                            <span className="min-w-0 flex-1">
                              <Progress value={now.progress} />
                            </span>
                            <span className="text-static-400 w-44 shrink-0 text-xs">
                              {now.kind === "show"
                                ? `${range.replace(/^.*–/, "until ")} · ${minutesText(now.minutesLeft)} left`
                                : `Back in ${minutesText(now.minutesLeft)}`}
                            </span>
                          </span>
                        )}
                      </span>
                      <span className="flex min-w-0 items-baseline gap-4">
                        <span className="text-static-400 w-20 shrink-0 text-right font-mono text-sm font-medium">
                          {now.next ? nextText : ""}
                        </span>
                        <span className="text-static-100 truncate text-base">{nextLine}</span>
                      </span>
                    </span>
                  )}
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
};

const Progress = ({ value }: { value: number | undefined }) => (
  <span aria-hidden className="bg-static-700 block h-[3px] w-full rounded-full">
    {value === undefined ? null : (
      <span className="bg-static-100 block h-full rounded-full" style={{ width: `${value * 100}%` }} />
    )}
  </span>
);

export { GuideC };
