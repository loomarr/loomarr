// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variant G, "Loomarr TV". E is a well-finished on-demand player (poster, year · rating · runtime ·
// genre, synopsis, scrub thumbnail, logo): it answers "which file am I watching". G answers what an
// analog set answered: what channel is this, what is on, what is next on this channel.
//
// So the channel is the protagonist. Tuning (and every channel change) puts a big channel number top
// right like a set's on-screen display, which fades by itself after three seconds. The information is
// Loomarr's own lower third, flat: an amber channel block (the one place an era tint may go), then
// Now with its episode, a thin line across the programme's time slot with the clock at both ends,
// and Next on the same band. Broadcast details are content, not decoration: the parental guideline
// bug for the first seconds of a programme, a cable-box clock, "Back in 2 min" in a break.
//
// Flipping channels has two states of its own. Surfing: a slim strip of the neighbouring channels
// (number + what is on now) with the target lit, and number entry like a remote ("1_"). Tuning:
// the shipped static-to-signal idea (snow, then the seven brand bars lock to colour, "Tuning in_"),
// drawn only between channels, never over playing video. Faint scanlines exist only behind the
// station-ID card. Nothing fakes old hardware.
//
// Interactive: ArrowUp / ArrowDown flip, digits type a channel number, and the strip settles into a
// tune after a beat. The static states (surfing, tuning) hold still so they can be looked at.
import { brandChroma, icons, playerChrome, semanticMotion, Text, typography } from "@loomarr/design-system";
import {
  Airplay,
  ArrowLeft,
  ArrowLeftRight,
  AudioLines,
  Captions,
  type LucideIcon,
  Maximize,
  PictureInPicture2,
} from "lucide-react";
import { type CSSProperties, type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { type BugCorner, ChannelBug, clearOfBug } from "./bug";
import type { ChromeProps } from "./chrome-classic";
import { PreviewTimeline } from "./chrome-line";
import { nowProgramme as current, nextProgramme as next } from "./chrome-rich";
import { b1, type ChannelRow, channel, channels, formatTime, lastChannel, MINUTE, NOW } from "./data";
import { type Density, flat, scrimStyle } from "./flat";
import {
  AudioBody,
  BottomSheet,
  Caption,
  DetailsBody,
  DetailsPanel,
  InfoBody,
  Menu,
  MoreRows,
  PanelStyles,
  PictureBody,
  SubtitlesBody,
  UnmutePill,
  VolumeBar,
} from "./panels-j";
import { realChannel } from "./real-art";
import type { Scenario } from "./scenario";
import { stationLine } from "./scenario";
import { ChannelList, Cover, MoreBody, Sheet } from "./sheets";
import { FlipEdge, Tune, type TuneKind } from "./tune-j";

const EASE = "cubic-bezier(0.2, 0.8, 0.2, 1)";
const { focus: FOCUS_MS, overlay: OVERLAY_MS, tune: TUNE_MS } = semanticMotion;

/** How long the channel readout stays after a tune, as on a set's on-screen display. */
const READOUT_MS = 3000;
/** The parental guideline bug is on screen for the first seconds of a programme. */
const BUG_MS = 6000;
/** After the last key press the strip waits this long, then the tuner moves. */
const SETTLE_MS = 1200;
const TUNING_MS = 800;

const HOME = channels.findIndex((c) => c.number === channel.number);
const MONO = typography.family.data.web;

// ---------------------------------------------------------------------------------------------
// What each channel has on. Channel 7 is the real evening in data.ts; the others are stand-ins
// so a flip lands on something plausible (invented titles only).

interface Guide {
  line: string;
  nextAt: string;
  nextName: string;
  title: string;
}

const NEXT_ON: Record<string, string> = {
  "3": "More cartoons",
  "5": "The late news",
  "9": "A spy film",
  "12": "Prize round",
  "14": "A river crossing",
  "21": "Highlights",
};

// Plain words: "11 min behind", or seconds when it is under a minute.
const behindLabel = (ms: number) =>
  ms < 60_000 ? `${Math.round(ms / 1000)} sec behind` : `${Math.round(ms / MINUTE)} min behind`;

const clock = (ms: number) => formatTime(ms).replace(/ [AP]M$/, "");

const guideOf = (row: ChannelRow): Guide =>
  row.number === channel.number
    ? { line: current.line, nextAt: clock(next.startMs), nextName: next.name, title: current.name }
    : { line: "", nextAt: "9:00", nextName: NEXT_ON[row.number] ?? "Back soon", title: row.nowPlaying };

// ---------------------------------------------------------------------------------------------
// Stylesheet: keyframes and hover states, which inline style cannot say.

const TvStyles = () => (
  <style>{`
    @keyframes tv-lock {
      0% { background-color: var(--tv-snow); height: 28%; }
      25% { background-color: var(--tv-snow); height: 92%; }
      50% { background-color: var(--tv-snow); height: 44%; }
      75% { background-color: var(--tv-snow); height: 80%; }
      100% { background-color: var(--c); height: 64%; }
    }
    @keyframes tv-blink { 0%, 49% { opacity: 1; } 50%, 100% { opacity: 0; } }
    @keyframes tv-rise { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: none; } }
    @keyframes tv-fade { from { opacity: 0; } to { opacity: 1; } }

    .tv-root { font-variant-numeric: tabular-nums; }
    .tv-chrome { transition: opacity ${OVERLAY_MS}ms ${EASE}, transform ${OVERLAY_MS}ms ${EASE}; }
    .tv-chrome[data-shown="true"] { animation: tv-rise ${OVERLAY_MS}ms ${EASE} backwards; }
    .tv-chrome[data-shown="false"] { opacity: 0; transform: translateY(12px); pointer-events: none; }
    .tv-dress { transition: opacity ${OVERLAY_MS}ms ${EASE}; }
    .tv-osd { transition: opacity ${OVERLAY_MS}ms ${EASE}; }
    .tv-osd[data-shown="false"] { opacity: 0; }

    .tv-bar { animation: tv-lock 560ms ${EASE} both; background-color: var(--c); height: 64%; }
    .tv-cursor { animation: tv-blink 1s steps(1, end) infinite; }
    .tv-tuning { animation: tv-fade ${FOCUS_MS}ms linear backwards; }

    .tv-key { transition: transform ${FOCUS_MS}ms ${EASE}, background-color ${FOCUS_MS}ms ${EASE}; }
    .tv-key:hover { background-color: ${flat.focus}; }
    .tv-key.tv-key-ok:hover { background-color: ${playerChrome.text}; opacity: 0.88; }
    .tv-key:active { transform: scale(0.96); }
    .tv-cell { transition: background-color ${FOCUS_MS}ms ${EASE}; }
    .tv-keywrap-quiet .tv-legend { opacity: 0; transition: opacity ${FOCUS_MS}ms ${EASE}; }
    .tv-keywrap-quiet:hover .tv-legend, .tv-keywrap-quiet:focus-within .tv-legend, .tv-keywrap-on .tv-legend { opacity: 1; }
    .tv-util { border-radius: 8px; color: ${flat.textSecondary}; transition: background-color ${FOCUS_MS}ms ${EASE}, color ${FOCUS_MS}ms ${EASE}; }
    .tv-util:hover { background-color: ${flat.focus}; color: ${flat.text}; }

    @media (prefers-reduced-motion: reduce) {
      .tv-chrome[data-shown="true"] { animation-name: tv-fade; }
      .tv-chrome[data-shown="false"] { transform: none; }
      .tv-bar { animation: none; }
      .tv-cursor { animation: none; }
      .tv-key:active { transform: none; }
    }
  `}</style>
);

// ---------------------------------------------------------------------------------------------
// Type helpers. Mono is for channel numbers and clocks only; everything else is Geist.

const mono = (size: number, weight = 600, extra: CSSProperties = {}): CSSProperties => ({
  fontFamily: MONO,
  fontSize: size,
  fontWeight: weight,
  lineHeight: 1,
  ...extra,
});

const Clock = ({ ms, density }: { density: Density; ms: number }) => {
  const [hm, meridiem] = formatTime(ms).split(" ");
  return (
    <span style={{ alignItems: "baseline", display: "inline-flex", gap: 4 }}>
      <span style={mono(density === "pointer" ? 16 : 14, 600, { color: flat.text })}>{hm}</span>
      <span style={mono(density === "pointer" ? 11 : 10, 500, { color: flat.textMuted })}>{meridiem}</span>
    </span>
  );
};

const Cursor = () => (
  <span aria-hidden className="tv-cursor">
    _
  </span>
);

// ---------------------------------------------------------------------------------------------
// The channel block: CH over the number, flat amber. The era tint, when on, goes here and nowhere else.

const Block = ({
  density,
  ident = false,
  number,
  squareFoot = false,
  tint,
}: {
  density: Density;
  ident?: boolean;
  number: string;
  /** Variant J: the block stops at the header rows and the film strip runs beneath it. */
  squareFoot?: boolean;
  tint?: string;
}) => {
  const pointer = density === "pointer";
  return (
    <div
      aria-hidden
      style={{
        alignItems: "center",
        background: tint ?? flat.amber,
        borderRadius: squareFoot ? "12px 0 0 0" : "12px 0 0 12px",
        display: "flex",
        flexDirection: "column",
        flexShrink: 0,
        gap: pointer ? 8 : 4,
        justifyContent: "center",
        width: ident ? (pointer ? 112 : 72) : pointer ? 96 : 56,
      }}
    >
      <span
        style={mono(pointer ? 12 : 11, 600, {
          color: flat.ink,
          letterSpacing: "0.2em",
          paddingLeft: "0.2em",
        })}
      >
        CH
      </span>
      <span
        style={mono(ident ? (pointer ? 56 : 36) : pointer ? 48 : 30, 650, {
          color: flat.ink,
          letterSpacing: "-0.02em",
        })}
      >
        {number}
      </span>
    </div>
  );
};

// ---------------------------------------------------------------------------------------------
// The lower third: Now, the slot's progress line with clock times at both ends, and Next.

const Slot = ({ scenario, white = false }: { scenario: Scenario; white?: boolean }) => {
  const length = current.stopMs - current.startMs;
  const at = (ms: number) => Math.min(Math.max((ms - current.startMs) / length, 0), 1);
  const played = at(scenario.playheadMs);
  const live = at(NOW);
  const behind = scenario.lagMs > 1000;
  return (
    <div
      style={{
        background: "rgba(255, 255, 255, 0.18)",
        borderRadius: 2,
        flex: 1,
        height: 4,
        minWidth: 0,
        position: "relative",
      }}
    >
      {behind ? (
        <div
          style={{
            background: "rgba(255, 255, 255, 0.32)",
            borderRadius: 2,
            height: 4,
            position: "absolute",
            width: `${live * 100}%`,
          }}
        />
      ) : null}
      <div
        style={{
          // Variant J: the line is white, so amber stays on the channel block alone.
          background: white ? flat.text : flat.amber,
          borderRadius: 2,
          height: 4,
          position: "absolute",
          transition: `width ${TUNE_MS}ms ${EASE}`,
          width: `${played * 100}%`,
        }}
      />
      {behind ? (
        <div
          style={{
            background: flat.live,
            height: 12,
            left: `${live * 100}%`,
            position: "absolute",
            top: -4,
            width: 2,
          }}
        />
      ) : null}
    </div>
  );
};

const GoLive = ({ density }: { density: Density }) => (
  <button
    className="pp-btn tv-key"
    style={{
      alignItems: "center",
      border: `1px solid ${flat.live}`,
      borderRadius: 8,
      display: "inline-flex",
      gap: 8,
      height: 32,
      padding: "0 12px",
    }}
    type="button"
  >
    <span style={{ background: flat.live, borderRadius: 4, height: 8, width: 8 }} />
    <Text density={density} strong textRole="label" tone="live">
      Go live
    </Text>
  </button>
);

const LiveMark = ({ density }: { density: Density }) => (
  <span style={{ alignItems: "center", display: "inline-flex", gap: 8 }}>
    <span style={{ background: flat.live, borderRadius: 4, height: 8, width: 8 }} />
    <Text density={density} strong textRole="label" tone="live">
      Live
    </Text>
  </span>
);

const Tag = ({ children, density }: { children: ReactNode; density: Density }) => (
  <Text density={density} textRole="cardMeta" tone="muted">
    {children}
  </Text>
);

const LowerThird = ({
  density,
  guide,
  landscape,
  refined = false,
  row,
  scenario,
  standIns = false,
  timelineCard = false,
  timelineFull = false,
  timelineOpen = false,
  timelineRing = false,
  topEdge,
  tint,
}: {
  /** Variant J: the open card is Info, so it shows the whole synopsis and what follows. */
  timelineFull?: boolean;
  density: Density;
  guide: Guide;
  landscape: boolean;
  refined?: boolean;
  row: ChannelRow;
  scenario: Scenario;
  /** Variant J: a library with no art, so the strip and its card use the drawn stand-ins. */
  standIns?: boolean;
  /** Variant J, television: the block with the card open has remote focus. */
  timelineRing?: "now" | boolean;
  /** Variant J: drawn along the band's top edge (the warm flip's bars). */
  topEdge?: ReactNode;
  /** Variant J: hold a preview card open, for a still: the programme on now, or the next one. */
  timelineCard?: "next" | "now" | false;
  /** Variant J: the film strip stands in for the progress line and the Next row (desktop). */
  timelineOpen?: boolean;
  tint?: string;
}) => {
  const pointer = density === "pointer";
  const behind = scenario.lagMs > 1000;
  const breakLeft = Math.max(1, Math.round((b1.stopMs - scenario.playheadMs) / MINUTE));
  const gutter = pointer ? 40 : 36;
  return (
    <div
      style={{
        display: "flex",
        flex: 1,
        flexDirection: "column",
        maxWidth: pointer ? 960 : undefined,
        minWidth: 0,
      }}
    >
      <div style={{ display: "flex", minWidth: 0, position: "relative" }}>
        {topEdge}
        <Block density={density} number={row.number} squareFoot={timelineOpen} tint={tint} />
        <div
          style={{
            background: flat.raised,
            border: `1px solid ${flat.hairline}`,
            borderBottom: timelineOpen ? 0 : `1px solid ${flat.hairline}`,
            borderLeft: 0,
            borderRadius: timelineOpen ? "0 12px 0 0" : "0 12px 12px 0",
            display: "flex",
            flex: 1,
            flexDirection: "column",
            gap: pointer ? 8 : 4,
            minWidth: 0,
            padding: pointer ? "16px 24px" : "12px 16px",
          }}
        >
          <div
            style={{
              alignItems: "center",
              display: "flex",
              gap: 16,
              justifyContent: "space-between",
              minHeight: 32,
            }}
          >
            <span style={{ alignItems: "baseline", display: "inline-flex", gap: 8, minWidth: 0 }}>
              {/* The block beside this already says CH and the number; the band names the channel. */}
              <Text density={density} numberOfLines={1} strong textRole="label" tone="primary">
                {row.name}
              </Text>
            </span>
            <span style={{ alignItems: "center", display: "inline-flex", flexShrink: 0, gap: 16 }}>
              {behind ? (
                <>
                  {pointer ? <Tag density={density}>{behindLabel(scenario.lagMs)}</Tag> : null}
                  <GoLive density={density} />
                </>
              ) : (
                <LiveMark density={density} />
              )}
              {pointer || !behind ? <Clock density={density} ms={scenario.playheadMs} /> : null}
            </span>
          </div>

          {/* Variant J: a channel with nothing scheduled says so in the app's own two words, and has
              no Now, no Next and no strip to draw. */}
          {scenario.noSchedule ? (
            <div style={{ alignItems: "center", display: "flex", minHeight: pointer ? 36 : 24 }}>
              <Text density={density} textRole={pointer ? "reading" : "cardMeta"} tone="secondary">
                Nothing scheduled
              </Text>
            </div>
          ) : null}
          <div
            style={{
              alignItems: "baseline",
              columnGap: 16,
              display: scenario.noSchedule ? "none" : "grid",
              // minmax(0, …): a bare 1fr grows to fit a long unbroken line and pushes it out of the
              // band (found with the long-name stress case on a portrait phone).
              gridTemplateColumns: `${gutter}px minmax(0, 1fr)`,
              rowGap: pointer ? 8 : 4,
            }}
          >
            <Tag density={density}>Now</Tag>
            <div
              style={{
                alignItems: pointer ? "baseline" : "flex-start",
                display: "flex",
                flexDirection: pointer ? "row" : "column",
                gap: pointer ? 16 : 0,
                minWidth: 0,
              }}
            >
              {/* The title is typed, in every variant built on G. A title's logo was tried here for J and
                taken out (maintainer, 2026-11-09): at this size its own colours and detail fight the
                band, and only some titles have one. Logos go where they have room (the preview card). */}
              {/* Variant J: when the row is tight the episode line gives way first, then the title.
                  Shrinking both evenly cut a seven-letter title to "Fra…" on a television. */}
              <div
                // A flex box, so the text inside is a flex item and its ellipsis applies; as an inline
                // box inside a plain div it ran straight out of the band.
                style={
                  refined && pointer
                    ? { display: "flex", flex: "0 1 auto", minWidth: 0, overflow: "hidden" }
                    : { display: "flex", maxWidth: "100%", minWidth: 0, overflow: "hidden" }
                }
              >
                {/* A portrait phone has the height: the title may take two lines there. */}
                <Text
                  density={density}
                  minWidth={0}
                  numberOfLines={refined && !pointer && !landscape ? 2 : 1}
                  strong
                  textRole={pointer ? "title" : "label"}
                  tone="primary"
                >
                  {guide.title}
                </Text>
              </div>
              {scenario.inBreak ? (
                <Text
                  density={density}
                  numberOfLines={1}
                  strong
                  textRole={pointer ? "reading" : "cardMeta"}
                  tone="primary"
                >
                  <span style={{ color: playerChrome.breakAccent }}>Back in {breakLeft} min</span>
                </Text>
              ) : guide.line ? (
                <div
                  style={
                    refined && pointer
                      ? { display: "flex", flex: "0 100000 auto", minWidth: 0, overflow: "hidden" }
                      : { display: "flex", maxWidth: "100%", minWidth: 0, overflow: "hidden" }
                  }
                >
                  <Text
                    density={density}
                    minWidth={0}
                    numberOfLines={1}
                    textRole={pointer ? "reading" : "cardMeta"}
                    tone="secondary"
                  >
                    {guide.line}
                  </Text>
                </div>
              ) : null}
              {/* Variant J: with the strip standing in for the line and its two clocks, the row says
                how long is left in words. It is the thing people look at the band for. */}
              {timelineOpen && !scenario.inBreak ? (
                <span style={{ flexShrink: 0, marginLeft: "auto" }}>
                  <Text density={density} textRole="reading" tone="secondary">
                    {`${Math.max(1, Math.round((current.stopMs - scenario.playheadMs) / MINUTE))} min left`}
                  </Text>
                </span>
              ) : null}
            </div>

            {timelineOpen ? null : (
              <>
                <span />
                <div style={{ alignItems: "center", display: "flex", gap: 12, minHeight: 16 }}>
                  <Text density={density} textRole="cardTime" tone="secondary">
                    {clock(current.startMs)}
                  </Text>
                  <Slot scenario={scenario} white={refined} />
                  <Text density={density} textRole="cardTime" tone="secondary">
                    {clock(current.stopMs)}
                  </Text>
                </div>

                <Tag density={density}>Next</Tag>
                <div style={{ alignItems: "baseline", display: "flex", gap: 12, minWidth: 0 }}>
                  <Text density={density} textRole="cardTime" tone="secondary">
                    {guide.nextAt}
                  </Text>
                  <Text
                    density={density}
                    numberOfLines={1}
                    textRole={pointer ? "body" : "cardMeta"}
                    tone="primary"
                  >
                    {guide.nextName}
                  </Text>
                  {landscape && behind ? (
                    <span style={{ flexShrink: 0, marginLeft: "auto", whiteSpace: "nowrap" }}>
                      <Tag density={density}>{behindLabel(scenario.lagMs)}</Tag>
                    </span>
                  ) : null}
                </div>
              </>
            )}
          </div>
        </div>
      </div>
      {timelineOpen ? (
        // Variant J: the film strip is the band's foot, the full width of the band, under the
        // channel block as well. It says what the thin line and the Next row said and more (what
        // follows, where the breaks fall, a card for each block). The block on air carries no name,
        // because the row above has just said it. The card rises clear of the header, so it covers
        // the picture and not the band; the playhead is white, so amber stays on the block.
        <div
          style={{
            background: flat.raised,
            border: `1px solid ${flat.hairline}`,
            borderRadius: "0 0 12px 12px",
            borderTop: 0,
            padding: "14px 16px 12px",
          }}
        >
          <PreviewTimeline
            card={timelineCard}
            cardFull={timelineFull}
            cardLift={118}
            density={density}
            headColor={flat.text}
            playheadMs={scenario.playheadMs}
            quietNow
            ring={timelineRing}
            standIns={standIns}
          />
        </div>
      ) : null}
    </div>
  );
};

// ---------------------------------------------------------------------------------------------
// The station-ID card: the same lower third, larger, with the seven brand bars and (only here)
// faint scanlines behind the text.

const Bars = ({ height }: { height: number }) => (
  <div aria-hidden style={{ display: "flex", height }}>
    {brandChroma.map((color) => (
      <span key={color} style={{ background: color, flex: 1 }} />
    ))}
  </div>
);

const IdentCard = ({ density, tint }: { density: Density; tint?: string }) => {
  const pointer = density === "pointer";
  return (
    <div style={{ display: "flex", flex: 1, maxWidth: pointer ? 960 : undefined, minWidth: 0 }}>
      <Block density={density} ident number={channel.number} tint={tint} />
      <div
        style={{
          background: flat.raised,
          border: `1px solid ${flat.hairline}`,
          borderLeft: 0,
          borderRadius: "0 12px 12px 0",
          display: "flex",
          flex: 1,
          flexDirection: "column",
          minWidth: 0,
          overflow: "hidden",
          position: "relative",
        }}
      >
        <div
          aria-hidden
          style={{
            // Direction move 1: the only place scanlines are allowed.
            backgroundImage:
              "repeating-linear-gradient(0deg, rgba(255, 255, 255, 0.04) 0 1px, transparent 1px 3px)",
            inset: 0,
            position: "absolute",
          }}
        />
        <div style={{ position: "relative" }}>
          <Bars height={pointer ? 8 : 6} />
        </div>
        <div
          style={{
            alignItems: "center",
            display: "flex",
            gap: 16,
            justifyContent: "space-between",
            padding: pointer ? "24px 24px 24px" : "16px",
            position: "relative",
          }}
        >
          <div style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 0 }}>
            <Text
              density={density}
              numberOfLines={1}
              strong
              textRole={pointer ? "display" : "title"}
              tone="primary"
            >
              {channel.name}
            </Text>
            <Text
              density={density}
              numberOfLines={1}
              textRole={pointer ? "reading" : "body"}
              tone="secondary"
            >
              {stationLine}
            </Text>
          </div>
          <Clock density={density} ms={NOW} />
        </div>
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------------------------
// The neighbours strip: channel number + what is on now. The target is lit; the current channel
// is marked. Wide screens show five channels, phones three.

const Strip = ({
  art = false,
  count,
  density,
  onAir,
  standIns = false,
  target,
  tintFor,
}: {
  /** A library with no art: every card's picture is its flat stand-in. */
  standIns?: boolean;
  /** Variant J: a channel whose programme has fanart shows it, dimmed, behind its card. */
  art?: boolean;
  count: number;
  density: Density;
  onAir: number;
  target: number;
  tintFor: (era: string) => string | undefined;
}) => {
  const pointer = density === "pointer";
  const start = Math.min(Math.max(target - Math.floor(count / 2), 0), channels.length - count);
  return (
    <fieldset
      aria-label="Channels"
      style={{
        border: 0,
        margin: 0,
        padding: 0,
        display: "grid",
        flex: 1,
        gap: 8,
        gridTemplateColumns: `repeat(${count}, minmax(0, 1fr))`,
        maxWidth: pointer ? 960 : undefined,
        minWidth: 0,
      }}
    >
      {channels.slice(start, start + count).map((row, i) => {
        const index = start + i;
        const lit = index === target;
        const here = index === onAir;
        const era = tintFor(row.era);
        const fanart = art && !standIns ? realChannel(row.number)?.fanartSrc : undefined;
        // No fanart: a flat tone from the channel's number, the way the film strip's stand-ins are.
        const standIn = `hsl(${(Number(row.number) * 47) % 360} 24% 17%)`;
        const block = (
          <span
            style={{
              alignItems: "center",
              alignSelf: "flex-start",
              background: lit ? (era ?? flat.amber) : (era ?? flat.elevated),
              borderRadius: 8,
              display: "inline-flex",
              flexShrink: 0,
              height: 32,
              justifyContent: "center",
              minWidth: 40,
              padding: "0 8px",
            }}
          >
            <span
              style={mono(18, 650, {
                color: lit || era ? flat.ink : flat.text,
                letterSpacing: "-0.02em",
              })}
            >
              {row.number}
            </span>
          </span>
        );
        return (
          <div
            aria-current={lit || undefined}
            className="tv-cell"
            key={row.number}
            style={{
              background: lit ? flat.focus : flat.raised,
              border: `1px solid ${flat.hairline}`,
              borderRadius: 12,
              boxShadow: lit ? `inset 0 0 0 2px ${flat.amber}` : undefined,
              display: "flex",
              flexDirection: "column",
              gap: art ? 0 : 8,
              minWidth: 0,
              overflow: art ? "hidden" : undefined,
              padding: art ? 0 : pointer ? 12 : 8,
            }}
          >
            {art ? (
              // Variant J: what is on is what you choose a channel by, so each card gets a picture
              // of its own with the words under it, never words on top of a washed-out picture.
              // Fanart when the library has it, otherwise a flat stand-in. The card being aimed at
              // is at full strength; the rest sit back.
              <div
                style={{
                  background: fanart ? `${flat.elevated} url("${fanart}") center/cover` : standIn,
                  height: pointer ? 76 : 48,
                  opacity: lit ? 1 : 0.6,
                  position: "relative",
                }}
              >
                <span style={{ left: 8, position: "absolute", top: 8 }}>{block}</span>
              </div>
            ) : (
              block
            )}
            <div
              style={{
                display: "flex",
                flexDirection: "column",
                minWidth: 0,
                padding: art ? (pointer ? "10px 12px 12px" : "6px 8px 8px") : undefined,
                width: "100%",
              }}
            >
              <Text
                density={density}
                numberOfLines={1}
                strong
                textRole={pointer ? "label" : "cardMeta"}
                tone="primary"
              >
                {row.name}
              </Text>
              <Text
                density={density}
                numberOfLines={1}
                textRole="cardMeta"
                tone={here ? "signal" : "secondary"}
              >
                {here ? "Watching now" : row.nowPlaying}
              </Text>
            </div>
          </div>
        );
      })}
    </fieldset>
  );
};

// ---------------------------------------------------------------------------------------------
// The remote's keys. Flat rounded squares with the legend printed under each; the centre key is
// the white OK. Last channel is a key only when there is a last channel.

const Key = ({
  children,
  focused = false,
  label,
  legend,
  ok = false,
  onPress,
  pressed = false,
  quiet = false,
  size,
  width,
}: {
  /** Variant J: this key's menu is open, so it is a tone lighter and says so to a screen reader. */
  pressed?: boolean;
  children: ReactNode;
  /** Television: this key has remote focus, so it wears the amber ring and shows its legend. */
  focused?: boolean;
  label: string;
  legend?: string;
  ok?: boolean;
  onPress?: () => void;
  /** Variant J: the legend shows only while the key is pointed at or focused. */
  quiet?: boolean;
  size: number;
  width?: number;
}) => (
  <div
    className={
      quiet
        ? focused
          ? "tv-keywrap tv-keywrap-quiet tv-keywrap-on"
          : "tv-keywrap tv-keywrap-quiet"
        : "tv-keywrap"
    }
    style={{
      alignItems: "center",
      display: "flex",
      flexDirection: "column",
      gap: 8,
      width: legend ? 88 : undefined,
    }}
  >
    <button
      aria-expanded={pressed || undefined}
      aria-label={label}
      className={ok ? "pp-btn tv-key tv-key-ok" : "pp-btn tv-key"}
      onClick={onPress}
      style={{
        alignItems: "center",
        background: ok ? playerChrome.disc : pressed ? flat.focus : flat.elevated,
        border: ok ? undefined : `1px solid ${pressed ? flat.textSecondary : flat.hairline}`,
        borderRadius: 12,
        display: "inline-flex",
        flexShrink: 0,
        height: size,
        justifyContent: "center",
        outline: focused ? `2px solid ${flat.amber}` : undefined,
        outlineOffset: 3,
        width: width ?? size,
      }}
      type="button"
    >
      {children}
    </button>
    {legend ? (
      <span aria-hidden className="tv-legend">
        <Text density="pointer" textRole="cardMeta" tone="secondary">
          {legend}
        </Text>
      </span>
    ) : null}
  </div>
);

const Glyph = ({ name, size }: { name: keyof typeof icons; size: number }) => {
  const Svg = icons[name];
  return <Svg aria-hidden color={playerChrome.text} size={size} strokeWidth={2} />;
};

const Keys = ({
  density,
  landscape,
  onFlip,
  playing,
  pressed,
  quiet = false,
  focusPlay = false,
  tv = false,
}: {
  /** Variant J: the key whose menu is open. */
  pressed?: "more" | "volume" | undefined;
  density: Density;
  landscape: boolean;
  onFlip: (dir: -1 | 1) => void;
  playing: boolean;
  quiet?: boolean;
  /** Television: Play has remote focus (it is where focus lands when the chrome comes up). */
  focusPlay?: boolean;
  /** Television: no Volume key; the remote has one. */
  tv?: boolean;
}) => {
  const pointer = density === "pointer";
  const size = pointer ? 56 : 48;
  const ok = pointer ? 80 : 64;
  const glyph = pointer ? 28 : 24;
  const PlayGlyph = playing ? icons.pause : icons.play;
  const legends = pointer;
  return (
    <div
      style={{
        alignItems: "flex-start",
        display: "flex",
        gap: pointer ? 0 : landscape ? 8 : 16,
        justifyContent: "center",
        flexShrink: 0,
      }}
    >
      {lastChannel ? (
        <>
          <Key
            label={`Last channel, ${lastChannel.number}`}
            legend={legends ? "Last channel" : undefined}
            quiet={quiet}
            size={size}
          >
            <span style={{ alignItems: "center", display: "inline-flex", flexDirection: "column", gap: 4 }}>
              <ArrowLeftRight
                aria-hidden
                color={playerChrome.text}
                size={pointer ? 18 : 16}
                strokeWidth={2}
              />
              <span style={mono(pointer ? 14 : 12, 600, { color: flat.textSecondary })}>
                {lastChannel.number}
              </span>
            </span>
          </Key>
          <span
            aria-hidden
            style={{ background: flat.hairline, height: size, margin: pointer ? "0 4px" : "0 4px", width: 1 }}
          />
        </>
      ) : null}
      <Key
        label="Channel down"
        legend={legends ? "Channel down" : undefined}
        onPress={() => onFlip(-1)}
        quiet={quiet}
        size={size}
      >
        <Glyph name="channelDown" size={glyph} />
      </Key>
      <Key
        label={playing ? "Pause" : "Play"}
        focused={focusPlay}
        legend={legends ? (playing ? "Pause" : "Play") : undefined}
        ok
        quiet={quiet}
        size={size}
        width={ok}
      >
        <PlayGlyph
          aria-hidden
          color={playerChrome.discInk}
          fill={playerChrome.discInk}
          size={glyph}
          strokeLinejoin="round"
          strokeWidth={2}
          style={playing ? undefined : { transform: "translateX(2px)" }}
        />
      </Key>
      <Key
        label="Channel up"
        legend={legends ? "Channel up" : undefined}
        onPress={() => onFlip(1)}
        quiet={quiet}
        size={size}
      >
        <Glyph name="channelUp" size={glyph} />
      </Key>
      {/* Variant J, desktop: Volume in one click, as H had it. Phones use the hardware keys. */}
      {quiet && pointer && !tv ? (
        <Key label="Volume" legend="Volume" pressed={pressed === "volume"} quiet size={size}>
          <Glyph name="volume" size={glyph} />
        </Key>
      ) : null}
      <Key
        label="More"
        legend={legends ? "More" : undefined}
        pressed={pressed === "more"}
        quiet={quiet}
        size={size}
      >
        <Glyph name="more" size={glyph} />
      </Key>
    </div>
  );
};

// ---------------------------------------------------------------------------------------------
// The on-screen display (top right) and the parental bug (top left).

const Readout = ({
  density,
  device,
  entry,
  missed = false,
  name,
  number,
  shown,
  top,
}: {
  /** Variant J: no channel has the number typed. The line under it says so, in the live red. */
  missed?: boolean;
  density: Density;
  device: string;
  entry: boolean;
  name: string;
  number: string;
  shown: boolean;
  /** Variant J: where it starts, when the channel's watermark is in this corner. */
  top?: number;
}) => {
  const pointer = density === "pointer";
  const edge = pointer ? 48 : device === "landscape" ? 24 : 16;
  return (
    <div
      aria-live="polite"
      className="tv-osd"
      data-shown={shown}
      role="status"
      style={{
        alignItems: "flex-end",
        display: "flex",
        flexDirection: "column",
        gap: 8,
        pointerEvents: "none",
        position: "absolute",
        right: edge,
        textShadow: "0 1px 3px rgba(0, 0, 0, 0.85)",
        top: top ?? (pointer ? 40 : device === "landscape" ? 16 : 24),
        // The miss is red words, and red words do not read on a bright picture by a shadow alone,
        // so this one moment gets a solid ink plate (as the real player's readout has).
        ...(missed
          ? {
              background: "rgba(11, 12, 14, 0.92)",
              borderRadius: 12,
              padding: pointer ? "12px 24px 16px" : "8px 16px 12px",
              textShadow: "none",
            }
          : {}),
      }}
    >
      <span
        style={mono(pointer ? 96 : device === "landscape" ? 56 : 64, 650, {
          color: flat.amber,
          letterSpacing: "-0.02em",
        })}
      >
        {number}
        {entry ? <Cursor /> : null}
      </span>
      <Text density={density} numberOfLines={1} strong textRole={pointer ? "title" : "label"} tone="primary">
        {missed ? <span style={{ color: flat.live }}>No such channel</span> : name}
      </Text>
    </div>
  );
};

const ParentalBug = ({
  device,
  left,
  shown,
  top,
}: {
  device: string;
  left?: number | undefined;
  shown: boolean;
  top?: number;
}) => {
  const pointer = device === "desktop";
  return (
    <div
      aria-hidden
      className="tv-osd"
      data-shown={shown}
      style={{
        background: "rgba(0, 0, 0, 0.5)",
        border: "1px solid rgba(255, 255, 255, 0.72)",
        borderRadius: 4,
        left: left ?? (pointer ? 48 : device === "landscape" ? 24 : 16),
        padding: pointer ? "8px 12px" : "4px 8px",
        pointerEvents: "none",
        position: "absolute",
        top: top ?? (pointer ? 40 : device === "landscape" ? 16 : 24),
      }}
    >
      <span style={mono(pointer ? 14 : 12, 600, { color: flat.text, letterSpacing: "0.08em" })}>TV-PG</span>
    </div>
  );
};

// ---------------------------------------------------------------------------------------------
// Tuning: snow, then the seven brand bars lock to colour. Over the picture only between channels.

// Grey snow on the canvas: turbulence re-seeded in steps, so it flickers rather than drifts.
const Snow = () => (
  <svg aria-hidden height="100%" style={{ inset: 0, position: "absolute" }} width="100%">
    <filter height="100%" id="tv-snow" width="100%" x="0" y="0">
      <feTurbulence baseFrequency="0.9" numOctaves="1" seed="3" type="fractalNoise">
        <animate
          attributeName="seed"
          calcMode="discrete"
          dur="0.5s"
          repeatCount="indefinite"
          values="3;7;11;5;9"
        />
      </feTurbulence>
      <feColorMatrix type="matrix" values="0 0 0 0 0.6  0 0 0 0 0.62  0 0 0 0 0.66  1.5 0 0 0 -0.64" />
    </filter>
    <rect fill={flat.canvas} height="100%" width="100%" />
    <rect filter="url(#tv-snow)" height="100%" width="100%" />
  </svg>
);

const Tuning = ({ density, row }: { density: Density; row: ChannelRow }) => {
  const pointer = density === "pointer";
  return (
    <div
      aria-label={`Tuning in to channel ${row.number}, ${row.name}`}
      className="tv-tuning"
      role="status"
      style={{ inset: 0, position: "absolute", zIndex: 30 }}
    >
      <Snow />
      <div
        style={
          {
            "--tv-snow": "#8b8f98",
            alignItems: "center",
            textShadow: "0 1px 2px rgba(0, 0, 0, 0.9), 0 0 16px rgba(0, 0, 0, 0.85)",
            display: "flex",
            flexDirection: "column",
            gap: 24,
            inset: 0,
            justifyContent: "center",
            position: "absolute",
          } as CSSProperties
        }
      >
        <div
          aria-hidden
          style={
            {
              "--tv-snow": "#8b8f98",
              alignItems: "flex-end",
              display: "flex",
              gap: pointer ? 8 : 6,
              height: pointer ? 64 : 48,
            } as CSSProperties
          }
        >
          {brandChroma.map((color, i) => (
            <span
              className="tv-bar"
              key={color}
              style={
                {
                  "--c": color,
                  animationDelay: `${i * 60}ms`,
                  borderRadius: 2,
                  width: pointer ? 12 : 8,
                } as CSSProperties
              }
            />
          ))}
        </div>
        <div style={{ alignItems: "baseline", display: "flex", gap: 16 }}>
          <span style={mono(pointer ? 32 : 24, 650, { color: flat.amber })}>CH {row.number}</span>
          <Text density={density} strong textRole={pointer ? "title" : "headline"} tone="primary">
            {row.name}
          </Text>
        </div>
        <span style={mono(pointer ? 14 : 13, 500, { color: flat.amber, letterSpacing: "0.08em" })}>
          Tuning in
          <Cursor />
        </span>
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------------------------

type Phase = "idle" | "surfing" | "tuning";

// Variant J's four utilities (top right): icons only, a tone step on hover, the amber ring on focus.
const Utility = ({
  icon: Icon,
  label,
  pressed = false,
  size,
}: {
  icon: LucideIcon;
  label: string;
  /** Its menu or sheet is open. */
  pressed?: boolean;
  size: number;
}) => (
  <button
    aria-expanded={pressed || undefined}
    aria-label={label}
    className="pp-btn tv-util"
    style={{
      alignItems: "center",
      // The same solid square as a main key, smaller: it reads over any picture and as one family.
      background: pressed ? flat.focus : flat.elevated,
      border: `1px solid ${pressed ? flat.textSecondary : flat.hairline}`,
      borderRadius: 10,
      color: pressed ? flat.text : undefined,
      display: "inline-flex",
      height: size,
      justifyContent: "center",
      width: size,
    }}
    title={label}
    type="button"
  >
    <Icon aria-hidden size={size >= 44 ? 22 : 20} strokeWidth={1.75} />
  </button>
);

interface TvProps extends ChromeProps {
  /** Variant J: the corner the channel's watermark is burned into (playout's default is top right). */
  bug?: BugCorner | undefined;
  /** Variant J: freeze the tune this many ms in (`?t=`), for a still. */
  freezeAt?: number | undefined;
  /** Variant J: replay the tune (`?loop=1`). */
  loop?: boolean | undefined;
  refined?: boolean;
  /** Variant J: `long` puts long names everywhere; `bare` is a library with no art at all. */
  stress?: "bare" | "long" | undefined;
}

/** Variant J: what each thing the player opens is called, in its heading. */
const TITLES = {
  audio: "Audio",
  channels: "Channels",
  details: "Playback details",
  info: "Info",
  more: "More",
  picture: "Picture",
  subtitles: "Subtitles",
} as const;

/** The upright iPhone's frame, and how far its 16:9 picture's foot is from the bottom of the screen. */
const PORTRAIT = { height: 844, pictureFoot: (844 - (390 * 9) / 16) / 2 } as const;

// Long names, for the stress case: a channel, a title, an episode and what follows.
const LONG = {
  channel: "Saturday Morning Cartoon Marathon Classics",
  line: "S12 E104 “The One Where Everything Takes Rather Longer Than Anybody Had Expected”",
  next: "The Extraordinary Voyages of the Lighthouse Keeper's Apprentice",
  title: "The Incredibly Long-Running Adventures of Captain Barnaby",
};

const TvInner = ({
  bug: corner = "top-right",
  device,
  freezeAt,
  loop,
  refined = false,
  scenario,
  stress,
  tint,
  tintFor,
}: TvProps) => {
  // Variant J, device "tv": the ten-foot layout the Apple TV and Android TV (Shield) apps share.
  // Same pieces as desktop at 960 × 540, but there is no pointer: the remote moves an amber focus
  // ring, the focused key shows its legend, and the focused block of the film strip opens its card.
  // No Fullscreen or Leave (Back leaves), no Volume key (the remote has one).
  const tv = refined && device === "tv";
  const density: Density = device === "desktop" || tv ? "pointer" : "touch";
  const pointer = density === "pointer";
  const landscape = device === "landscape";
  // Variant J on a phone is drawn as an iPhone: the chrome keeps out of the Dynamic Island, the home
  // indicator and, in landscape, both ends of the screen.
  const ios = refined && !pointer;
  const safe = ios
    ? landscape
      ? { bottom: 21, side: 59, top: 0 }
      : { bottom: 34, side: 0, top: 59 }
    : { bottom: 0, side: 0, top: 0 };
  // Variant J: the watermark is part of the video, so the chrome in its corner steps clear of it and
  // nothing else moves. Top right: the utilities (and the readout while flipping). Top left: the
  // rating. Bottom left: the band. Bottom right: the keys.
  const watermark = refined ? corner : "off";
  // A television keeps 5% clear at every edge (overscan): 48 × 27 at 960 × 540.
  const topBase = tv ? 28 : pointer ? 40 : landscape ? 16 : ios ? safe.top + 4 : 24;
  const bottomBase = tv ? 28 : pointer ? 40 : landscape ? 16 + safe.bottom / 2 : 24 + safe.bottom / 2;
  const topAt = (corner: BugCorner, reach: number) =>
    watermark === corner ? clearOfBug(device, topBase, reach) : topBase;
  const liftOf = (corner: BugCorner, reach: number) =>
    watermark === corner ? clearOfBug(device, bottomBase, reach) - bottomBase : 0;
  const bandLift = liftOf("bottom-left", pointer ? 220 : 150);
  // The cluster is the main keys with the utilities row over them.
  const keysLift = liftOf("bottom-right", pointer ? 150 : 120);
  const [phase, setPhase] = useState<Phase>(
    scenario.tuning ? "tuning" : scenario.surfing ? "surfing" : "idle",
  );
  const [onAir, setOnAir] = useState(HOME);
  // The prototype's static states hold a believable moment: surfing has "1" typed (12 and 14 are
  // the choices), tuning is mid-way to channel 9.
  const [target, setTarget] = useState(scenario.tuning ? 3 : scenario.surfing ? 4 : HOME);
  const [entry, setEntry] = useState(scenario.surfing ? "1" : "");
  const [osdN, setOsdN] = useState(0);
  // Variant J: the readout belongs to a channel change, so it is not up when the player first shows.
  const [osd, setOsd] = useState(!refined);
  const [bug, setBug] = useState(true);
  const timer = useRef(0);
  const targetRef = useRef(target);

  const settle = useCallback((ms: number) => {
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      setPhase("tuning");
      timer.current = window.setTimeout(() => {
        setOnAir(targetRef.current);
        setEntry("");
        setPhase("idle");
        setOsdN((n) => n + 1);
      }, TUNING_MS);
    }, ms);
  }, []);

  const flip = useCallback(
    (dir: -1 | 1) => {
      const to = (targetRef.current + dir + channels.length) % channels.length;
      targetRef.current = to;
      setTarget(to);
      setEntry("");
      setPhase("surfing");
      settle(SETTLE_MS);
    },
    [settle],
  );

  const type = useCallback(
    (digit: string) => {
      setEntry((typed) => {
        const wanted = `${typed}${digit}`.slice(-2);
        const matches = channels.filter((c) => c.number.startsWith(wanted));
        const first = channels.findIndex((c) => c.number.startsWith(wanted));
        if (first < 0) return "";
        targetRef.current = first;
        setTarget(first);
        setPhase("surfing");
        settle(matches.length === 1 ? 600 : SETTLE_MS + 400);
        return wanted;
      });
    },
    [settle],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === "ArrowUp") flip(1);
      else if (e.key === "ArrowDown") flip(-1);
      else if (/^[0-9]$/.test(e.key)) type(e.key);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [flip, type]);
  useEffect(() => () => window.clearTimeout(timer.current), []);

  // The readout is up after every tune and fades by itself; the bug only at a programme's start.
  // biome-ignore lint/correctness/useExhaustiveDependencies: osdN is the trigger, not a value read here
  useEffect(() => {
    if (refined && osdN === 0) return;
    setOsd(true);
    const t = window.setTimeout(() => setOsd(false), READOUT_MS);
    return () => window.clearTimeout(t);
  }, [osdN]);
  useEffect(() => {
    const t = window.setTimeout(() => setBug(false), BUG_MS);
    return () => window.clearTimeout(t);
  }, []);

  if (scenario.cover) {
    return <Cover density={density} kind={scenario.cover} primary={refined ? "white" : "amber"} />;
  }

  // Variant J changes channel with H's tune, not G's static-and-bars screen. A flip made with the
  // keys is the warm flip (the band stays up); the "tune" states play it once; "cold-tune",
  // "slow-tune" and "tuning" are the full signature, the last one frozen mid-way as H freezes it.
  const tuneKind: TuneKind | undefined = !refined
    ? undefined
    : scenario.tune
      ? scenario.cold
        ? "cold"
        : scenario.slow
          ? "slow"
          : "warm"
      : scenario.tuning
        ? "cold"
        : phase === "tuning"
          ? "warm"
          : undefined;
  const tuneFreeze = freezeAt ?? (refined && scenario.tuning ? 900 : undefined);
  // Variant J: what is open, and the form it takes on this device (panels-j.tsx has the reasoning).
  // Desktop and television: a menu in the control cluster. An upright iPhone: a sheet from the
  // bottom. A landscape iPhone: the right-hand sheet. Playback details and Info have places of their
  // own where there is room for them. A phone and a television have no Volume key to open a level.
  const open = refined ? (scenario.panel ?? scenario.extra ?? scenario.sheet) : undefined;
  const menu =
    pointer && (open === "more" || open === "subtitles" || open === "audio" || open === "picture")
      ? open
      : undefined;
  const volumeBar = pointer && !tv && open === "volume";
  const detailsPanel = pointer && open === "details";
  const infoCard = pointer && open === "info";
  const phoneSheet = !pointer && open !== "volume" ? open : undefined;
  const bottomSheet = ios && !landscape ? phoneSheet : undefined;
  const sheet = refined ? (pointer ? (open === "channels" ? open : undefined) : phoneSheet) : scenario.sheet;
  const tuning = phase === "tuning" && !refined;
  const surfing = phase === "surfing";
  const controls = (scenario.visible || surfing) && !sheet && !tuning;
  const card = scenario.ident && !scenario.visible && !sheet && !tuning;
  const shown = controls || card;
  const row = channels[onAir] ?? channels[HOME] ?? channels[0];
  const aim = channels[target] ?? row;
  if (!(row && aim)) return null;
  // The stills tune INTO the channel the rest of the mock is on, so the band the bars land on, the
  // picture and the film strip all agree. A flip made with the keys tunes to where it was aimed.
  const tuneTo = (scenario.tune || scenario.tuning ? row : aim) ?? row;
  const playing = scenario.mode !== "paused";
  // Variant J, "no-channel": 99 was typed and nothing has that number. The readout says so and goes.
  const missed = refined && Boolean(scenario.missed) && !surfing;
  const readout = surfing
    ? { entry: entry !== "", name: aim.name, number: entry || aim.number }
    : missed
      ? { entry: false, missed: true, name: "", number: "99" }
      : { entry: false, name: row.name, number: row.number };
  // Variant J: one identity at a time. The readout is up while flipping (the strip has replaced the
  // band) and for a moment after a tune when the band is hidden; never beside the band.
  const readoutOn =
    !sheet &&
    !tuning &&
    !card &&
    (surfing || missed || (osd && (refined ? !scenario.visible : scenario.visible)));
  const utilities = refined && controls && !surfing;
  // An upright iPhone's toolbar and back arrow are above the picture, so a sheet from the bottom
  // does not cover them and they stay.
  const topKeys = utilities || Boolean(bottomSheet);
  const bugOn =
    bug && !sheet && !tuning && !surfing && scenario.visible && !scenario.inBreak && scenario.mode === "live";
  const side = (tv || pointer ? 48 : landscape ? 24 : 16) + safe.side;
  const edge = `0 ${side}px ${bottomBase}px`;
  // Television focus, for the stills: Play when the chrome comes up; the next block of the film
  // strip in "chrome-top" (its card opens, as hovering it does on desktop).
  const focusStrip = tv && Boolean(scenario.topTip);
  const keys = tv ? (
    // Television: only what a remote does not have. Channel up and down are the remote's own up and
    // down, Play/Pause and Volume are its own buttons, Back leaves, and there is no fullscreen. Six
    // keys here squeezed the band until the title read "F."; four leave it room.
    <div style={{ alignItems: "flex-start", display: "flex", flexShrink: 0 }}>
      {lastChannel ? (
        <Key label={`Last channel, ${lastChannel.number}`} legend="Last channel" quiet size={56}>
          <span style={{ alignItems: "center", display: "inline-flex", flexDirection: "column", gap: 4 }}>
            <ArrowLeftRight aria-hidden color={playerChrome.text} size={18} strokeWidth={2} />
            <span style={mono(14, 600, { color: flat.textSecondary })}>{lastChannel.number}</span>
          </span>
        </Key>
      ) : null}
      <Key label="Subtitles" legend="Subtitles" pressed={open === "subtitles"} quiet size={56}>
        <Captions aria-hidden color={playerChrome.text} size={26} strokeWidth={2} />
      </Key>
      <Key label="Audio" legend="Audio" pressed={open === "audio"} quiet size={56}>
        <AudioLines aria-hidden color={playerChrome.text} size={26} strokeWidth={2} />
      </Key>
      <Key label="More" legend="More" pressed={open === "more" || open === "picture"} quiet size={56}>
        <Glyph name="more" size={28} />
      </Key>
    </div>
  ) : (
    <Keys
      density={density}
      landscape={landscape}
      onFlip={flip}
      playing={playing}
      pressed={open === "more" || open === "picture" ? "more" : volumeBar ? "volume" : undefined}
      quiet={refined}
    />
  );
  const utilSize = pointer ? 40 : 44;
  const utilityKeys = (
    <>
      <Utility icon={Captions} label="Subtitles" pressed={open === "subtitles"} size={utilSize} />
      <Utility icon={AudioLines} label="Audio" pressed={open === "audio"} size={utilSize} />
      {tv ? null : ios ? (
        // iPhone: there is no fullscreen to toggle (you turn the phone). AirPlay and
        // picture-in-picture are what an iPhone player is expected to offer.
        <>
          <Utility icon={Airplay} label="AirPlay" size={utilSize} />
          <Utility icon={PictureInPicture2} label="Picture in picture" size={utilSize} />
        </>
      ) : (
        <Utility icon={Maximize} label="Fullscreen" size={utilSize} />
      )}
    </>
  );
  // Leave is a back arrow in the top left corner of the screen, where people reach for it (the
  // maintainer, 2026-11-09: "I'm used to clicking the top left of the screen to exit watch"). A
  // corner is a place of its own, so it is not a control floating over the picture. It steps below
  // the watermark when the channel's bug is in that corner. A television has no such key: Back leaves.
  const leaveLeft = pointer ? 48 : landscape ? 24 + safe.side : 12;
  const leaveTop = ios && !landscape ? topBase : topAt("top-left", utilSize);
  const long = refined && stress === "long";
  const bare = refined && stress === "bare";
  const shownRow = long ? { ...row, name: LONG.channel } : row;
  const guide = long
    ? { ...guideOf(row), line: LONG.line, nextName: LONG.next, title: LONG.title }
    : guideOf(row);
  const body = card ? (
    <IdentCard density={density} tint={tint} />
  ) : surfing ? (
    <Strip
      art={refined}
      count={pointer ? 5 : 3}
      density={density}
      onAir={onAir}
      standIns={bare}
      target={target}
      tintFor={tintFor}
    />
  ) : (
    <LowerThird
      density={density}
      guide={guide}
      landscape={landscape}
      refined={refined}
      row={shownRow}
      scenario={scenario}
      standIns={bare}
      // The on-now block's card on desktop, because that programme is the one with real art on this
      // machine. On a television, the next block: it is the one the remote has moved to.
      // Info is the block on air, its card opened in full.
      timelineCard={infoCard ? "now" : scenario.topTip ? (tv ? "next" : "now") : false}
      timelineFull={infoCard}
      // Television: remote focus starts on the block on air (ring, no card yet) and, in "chrome-top",
      // has moved to the next block, whose card is open.
      timelineRing={tv ? (focusStrip ? true : "now") : false}
      topEdge={tuneKind === "warm" ? <FlipEdge freezeAt={tuneFreeze} loop={loop} /> : undefined}
      // Variant J: on desktop the film strip IS the band's progress row, there whenever the band is.
      // Nothing to discover, and nothing that only a mouse can reach (the first draft opened it on
      // hover over a thin line, which nobody would find). Phones keep the line and the Next row.
      timelineOpen={refined && pointer && !scenario.noSchedule}
      tint={tint}
    />
  );

  // What a sheet holds (phones, and Channels everywhere). A phone has no film strip in its band, so
  // its More keeps the schedule; its rows are J's (Picture and Playback details, no "Sound and
  // picture"), and it lists no picture in picture because the iPhone toolbar has that key.
  const inset = (child: ReactNode) => <div style={{ padding: "4px 12px 0" }}>{child}</div>;
  const sheetBody =
    sheet === "channels" ? (
      <ChannelList density={density} tintFor={tintFor} />
    ) : sheet === "more" ? (
      <MoreBody
        behind={scenario.mode !== "live"}
        density={density}
        headColor={refined ? flat.text : undefined}
        only={refined}
        plain
        rows={
          refined ? <MoreRows behind={scenario.mode !== "live"} density={density} pip={false} /> : undefined
        }
        schedule={!(refined && pointer)}
      />
    ) : sheet === "subtitles" ? (
      <SubtitlesBody density={density} />
    ) : sheet === "audio" ? (
      <AudioBody density={density} />
    ) : sheet === "picture" ? (
      <PictureBody density={density} />
    ) : sheet === "details" ? (
      inset(<DetailsBody density={density} />)
    ) : sheet === "info" ? (
      inset(<InfoBody bare={bare} density={density} />)
    ) : null;

  return (
    <div className="tv-root" style={{ inset: 0, pointerEvents: "none", position: "absolute" }}>
      <TvStyles />
      {refined ? <PanelStyles /> : null}
      {/* The warm flip's dip is on the picture only, so it is drawn under the chrome. */}
      {tuneKind === "warm" ? (
        <Tune
          compact={!pointer || tv}
          freezeAt={tuneFreeze}
          kind="warm"
          loop={loop}
          lowerThird={0}
          name={tuneTo.name}
          number={tuneTo.number}
        />
      ) : null}
      <div className="tv-dress" style={{ inset: 0, opacity: shown ? 1 : 0, position: "absolute" }}>
        <div style={scrimStyle(pointer ? 520 : landscape ? 340 : 460, true)} />
      </div>
      {watermark === "off" ? null : <ChannelBug corner={watermark} device={device} name={shownRow.name} />}
      {/* With subtitles on, a line of them: centred on the picture and lifted clear of the band while
          the controls are up. An upright iPhone's picture has nothing over it, so they sit at its foot. */}
      {open === "subtitles" ? (
        <Caption
          bottom={
            ios && !landscape
              ? PORTRAIT.pictureFoot + 8
              : landscape
                ? // The side sheet hides the controls, so the line is back at the picture's foot.
                  bottomBase + 12
                : bottomBase + bandLift + 226 + 16
          }
          lines={
            // A television's line is short (about 32 characters, as broadcast captions are), which
            // also keeps it clear of the menu beside it.
            tv
              ? ["I thought you said tonight", "would be quiet."]
              : ["I thought you said tonight would be quiet."]
          }
          maxWidth={tv ? 320 : pointer ? 720 : landscape ? 320 : 340}
          // A landscape phone's sheet covers the right of the picture; the line stays in what shows.
          right={landscape ? 384 : 0}
          size={tv ? 18 : pointer ? 24 : landscape ? 15 : 13}
        />
      ) : null}
      {/* iPhone furniture, so the safe areas are visible: the Dynamic Island and the home indicator. */}
      {ios ? (
        <>
          <span
            aria-hidden
            style={{
              background: "#000",
              borderRadius: 20,
              position: "absolute",
              ...(landscape
                ? { height: 126, left: 11, top: "50%", transform: "translateY(-50%)", width: 37 }
                : { height: 37, left: "50%", top: 11, transform: "translateX(-50%)", width: 126 }),
            }}
          />
          <span
            aria-hidden
            style={{
              background: "rgba(255, 255, 255, 0.85)",
              borderRadius: 3,
              bottom: 8,
              height: 5,
              left: "50%",
              position: "absolute",
              transform: "translateX(-50%)",
              width: landscape ? 220 : 140,
              zIndex: 70,
            }}
          />
        </>
      ) : null}
      {/* The rating sits beside the back arrow, in the same place whether or not the arrow is up. */}
      <ParentalBug
        device={device}
        left={refined && !tv ? leaveLeft + utilSize + 12 : undefined}
        shown={bugOn}
        top={refined && !tv ? leaveTop + (utilSize - (pointer ? 34 : 26)) / 2 : topAt("top-left", 40)}
      />
      {topKeys && !tv ? (
        <div style={{ left: leaveLeft, pointerEvents: "auto", position: "absolute", top: leaveTop }}>
          <Utility icon={ArrowLeft} label="Leave" size={utilSize} />
        </div>
      ) : null}
      {/* Playback details: upper left, under the back arrow, clear of the band and the keys. It is
          the one panel that stays while the controls hide. A television has less height, so it is
          two columns wide there, and has no Copy key (no clipboard) or close key (Back closes it). */}
      {detailsPanel ? (
        <DetailsPanel
          closable={!tv}
          left={48}
          top={tv ? topAt("top-left", 40) + 46 : leaveTop + utilSize + 12}
          wide={tv}
        />
      ) : null}
      {/* The muted first play (a browser's rule, so not on a television): top centre, clear of both
          corners; on an upright phone, just under the picture. */}
      {refined && scenario.unmute && !tv ? (
        <div
          style={{
            display: "flex",
            justifyContent: "center",
            left: 0,
            position: "absolute",
            right: 0,
            ...(ios && !landscape ? { bottom: PORTRAIT.pictureFoot - 12 - 44 } : { top: topBase }),
          }}
        >
          <UnmutePill touch={!pointer} />
        </div>
      ) : null}
      <Readout
        density={density}
        device={device}
        {...readout}
        shown={readoutOn}
        top={topAt("top-right", pointer ? 150 : 100)}
      />
      {/* A portrait iPhone has black above its picture, so the utilities are a toolbar there, under
          the Dynamic Island: grounded to the top of the screen, never over the picture. Everywhere
          else they sit in the control cluster (below). */}
      {topKeys && ios && !landscape ? (
        <div
          style={{
            display: "flex",
            gap: 6,
            pointerEvents: "auto",
            position: "absolute",
            right: 12,
            top: topBase,
          }}
        >
          {utilityKeys}
        </div>
      ) : null}
      <div
        aria-hidden={!shown || undefined}
        className="tv-chrome"
        data-shown={shown}
        style={{
          alignItems: pointer || landscape ? "flex-end" : "stretch",
          bottom: 0,
          display: "flex",
          flexDirection: pointer || landscape ? "row" : "column",
          gap: tv ? 28 : pointer ? 48 : landscape ? 24 : 16,
          left: 0,
          padding: edge,
          pointerEvents: shown ? "auto" : "none",
          position: "absolute",
          right: 0,
        }}
      >
        {bandLift > 0 ? (
          <div style={{ display: "flex", flex: 1, marginBottom: bandLift, minWidth: 0 }}>{body}</div>
        ) : (
          body
        )}
        {/* Television, while flipping channels: the keys are no use, so the strip takes the row. */}
        {card || (tv && surfing) ? null : pointer || landscape ? (
          // The control cluster: everything you can press, in one place. The four utilities are a
          // smaller row over the main keys, sharing their right edge. They used to sit alone in the
          // picture's top corner, under the watermark, grounded to nothing (maintainer, 2026-11-09).
          <div
            style={{
              alignItems: "flex-end",
              display: "flex",
              flexDirection: "column",
              gap: pointer ? 12 : 8,
              marginBottom: keysLift,
            }}
          >
            {/* The cluster's menu: whatever a key here opens, over the keys, and the rest of the
                chrome stays where it is. More holds what has no key of its own, and Picture is its
                second page. On a television the amber ring is on a row of it. */}
            {volumeBar ? <VolumeBar level={0.8} /> : null}
            {menu ? (
              <Menu back={menu === "picture"} title={menu === "more" ? undefined : TITLES[menu]}>
                {menu === "more" ? (
                  <MoreRows
                    behind={scenario.mode !== "live"}
                    density={density}
                    focus={tv ? 0 : undefined}
                    pip={!tv}
                  />
                ) : menu === "subtitles" ? (
                  <SubtitlesBody density={density} focus={tv ? 1 : undefined} />
                ) : menu === "audio" ? (
                  <AudioBody density={density} focus={tv ? 0 : undefined} />
                ) : (
                  <PictureBody density={density} focus={tv ? 0 : undefined} />
                )}
              </Menu>
            ) : null}
            {utilities && !tv ? <div style={{ display: "flex", gap: 6 }}>{utilityKeys}</div> : null}
            {keys}
          </div>
        ) : (
          <div style={{ alignSelf: "center" }}>{keys}</div>
        )}
      </div>
      {tuning ? <Tuning density={density} row={aim} /> : null}
      {tuneKind && tuneKind !== "warm" ? (
        <Tune
          compact={!pointer || tv}
          fanart={bare ? undefined : realChannel(tuneTo.number)?.fanartSrc}
          freezeAt={tuneFreeze}
          kind={tuneKind}
          loop={loop}
          // Where the band's top edge is, for the bars to land on.
          lowerThird={bottomBase + (pointer ? 226 : landscape ? 150 : 236)}
          name={tuneTo.name}
          number={tuneTo.number}
        />
      ) : null}
      {sheet && bottomSheet ? (
        // An upright iPhone: from the bottom, as tall as what it holds, never over the toolbar.
        <BottomSheet
          maxHeight={PORTRAIT.height - (topBase + utilSize + 12)}
          safeBottom={safe.bottom}
          title={TITLES[sheet]}
        >
          {sheetBody}
        </BottomSheet>
      ) : sheet ? (
        <Sheet
          closable={!tv}
          density={density}
          icon={sheet === "channels" ? "list" : sheet === "more" ? "more" : undefined}
          // Variant J on an iPhone: the sheet keeps out of the Dynamic Island, the home indicator
          // and, in landscape, the end of the screen it sits against.
          panelStyle={{
            paddingBottom: safe.bottom,
            paddingRight: safe.side,
            paddingTop: safe.top,
            pointerEvents: "auto",
          }}
          title={TITLES[sheet]}
        >
          {sheetBody}
        </Sheet>
      ) : null}
    </div>
  );
};

// Keyed by state, so switching states in the prototype starts that state's moment afresh.
// `refined` is variant J: G with the readout only on a channel change, a white progress line, a quiet
// row of utility icons, key legends on hover, and the preview timeline opening from the progress line.
const TvChrome = (props: TvProps) => <TvInner key={props.scenario.state} {...props} />;

// Variant H (chrome-tune.tsx) builds on G's pieces and replaces only the tune and the remote.
export { behindLabel, clock, guideOf, IdentCard, mono, ParentalBug, Slot, Strip, TvChrome };
