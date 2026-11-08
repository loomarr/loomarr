// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variant H, "Loomarr TV · signature tune". G with two things rebuilt, nothing else:
//
// 1. THE TUNE. Changing channel is Loomarr's one ownable moment, so it is one choreographed
//    sequence (~1.3 s) instead of a strip and a spinner. Static becoming signal, handed off to the UI
//    that stays:
//      release   0-180   the held picture drains (grey, dim); the lower third steps out
//      static    100-340 grey snow
//      sweep     340-770 the seven brand bars wipe across the FULL frame, hard-edged, left to right
//      number    560-840 the channel number lands, very large, set in ink on the bars, then the name
//      hold      840-960 one beat to read it
//      hand-off  960-1230 the bars collapse into the lower third's top edge, the number flies into the
//                amber channel block, the band unfolds from the bars, the picture comes up through
//                the static; the readout settles top right (and fades after 3 s, as in G)
//    Every beat is eased, none overshoots. A slow stream holds after "number" (bars breathing,
//    "Tuning in_") until the picture arrives, then runs the hand-off. Repeated presses re-target the
//    number in place; the hand-off runs once, after the viewer stops. Reduced motion: the bars and
//    number simply appear and crossfade out, no sweep, no flight.
//    ?t=<ms> freezes the tune at that moment (Web Animations API), ?loop=1 replays it.
//
// 2. THE REMOTE AND THE BAND (maintainer: "flat and boring, especially the buttons", then "don't make
//    it a toy"). Keys are neutral solid surfaces with crisp 6px corners and confident mono legends.
//    The one colour fill is the amber play key. Everything else shows its brand colour only as a
//    thin edge on hover, focus, press, or as a ring that flashes when the key tunes. Channel up and
//    down are one rocker. The lower third gets the brand bars as its top edge (the tune lands there),
//    a larger amber block, bigger type and a real contrast step; the clock's colon ticks and the Live
//    dot breathes, and nothing else idles.
//
// Interactive: ArrowUp / ArrowDown, digits, and L (last channel) start the tune. The static "surfing"
// state still shows G's neighbours strip until a key is pressed.
import { brandChroma, icons, playerChrome, typography } from "@loomarr/design-system";
import { ArrowLeftRight } from "lucide-react";
import {
  type CSSProperties,
  type ReactNode,
  type Ref,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { ChromeProps } from "./chrome-classic";
import { nowProgramme as current } from "./chrome-rich";
import { behindLabel, clock, guideOf, IdentCard, mono, ParentalBug, Slot, Strip } from "./chrome-tv";
import { b1, type ChannelRow, channel, channels, formatTime, lastChannel, MINUTE } from "./data";
import { flat, scrimStyle } from "./flat";
import type { Scenario } from "./scenario";
import { ChannelList, Cover, MoreBody, Sheet } from "./sheets";

// ---------------------------------------------------------------------------------------------
// The timeline, in ms from the key press. Part A runs on its own; part B (the hand-off) starts at
// T.handoff, or later when the viewer is still pressing or the picture has not arrived.

const T = {
  drain: 180,
  exit: 160,
  snowAt: 100,
  snowIn: 60,
  sweepAt: 340,
  sweepStagger: 32,
  sweepDur: 240,
  landAt: 560,
  landDur: 280,
  nameAt: 620,
  nameDur: 240,
  handoff: 960,
  handoffDur: 260,
  handoffStagger: 3,
  bodyAt: 40,
  bodyDur: 200,
  readoutAt: 320,
  readoutDur: 220,
} as const;

/** After the last key press the tune waits this long before handing off (number entry waits more). */
const SETTLE_MS = 420;
/** A slow stream: how long the picture takes to arrive in the "slow-tune" state. */
const SLOW_MS = 3200;
const READOUT_MS = 3000;
const BUG_MS = 6000;

const OUT = "cubic-bezier(0.16, 1, 0.3, 1)";
const SWEEP = "cubic-bezier(0.7, 0, 0.2, 1)";
const HAND = "cubic-bezier(0.76, 0, 0.24, 1)";
const EASE = "cubic-bezier(0.2, 0.8, 0.2, 1)";

const HOME = channels.findIndex((c) => c.number === channel.number);
const TO_NINE = channels.findIndex((c) => c.number === "9");
const LAST = channels.findIndex((c) => c.number === lastChannel?.number);
const MONO = typography.family.data.web;
const BODY = typography.family.body.web;

// ---------------------------------------------------------------------------------------------
// Device metrics.

interface Dims {
  big: number;
  blockW: number;
  bodyPad: string;
  chipW: number;
  clock: number;
  edge: string;
  hero: { h: number; w: number };
  key: number;
  keyGap: number;
  line: number;
  name: number;
  nameBig: number;
  num: number;
  rocker: { half: number; w: number };
  small: number;
  strip: number;
  title: number;
}

const dimsOf = (device: string): Dims => {
  if (device === "desktop") {
    return {
      big: 340,
      blockW: 144,
      bodyPad: "20px 28px",
      chipW: 64,
      clock: 20,
      edge: "0 48px 40px",
      hero: { h: 72, w: 104 },
      key: 60,
      keyGap: 20,
      line: 17,
      name: 14,
      nameBig: 64,
      num: 80,
      rocker: { half: 52, w: 68 },
      small: 14,
      strip: 10,
      title: 36,
    };
  }
  const landscape = device === "landscape";
  return {
    big: landscape ? 150 : 190,
    blockW: landscape ? 76 : 72,
    bodyPad: landscape ? "8px 14px" : "12px 14px",
    chipW: 44,
    clock: landscape ? 13 : 15,
    edge: landscape ? "0 24px 16px" : "0 16px 24px",
    hero: { h: landscape ? 56 : 64, w: landscape ? 76 : 84 },
    key: landscape ? 48 : 52,
    keyGap: landscape ? 14 : 16,
    line: 13,
    name: 12,
    nameBig: landscape ? 34 : 32,
    num: landscape ? 40 : 40,
    rocker: { half: landscape ? 40 : 44, w: 56 },
    small: 11,
    strip: 6,
    title: landscape ? 19 : 21,
  };
};

// ---------------------------------------------------------------------------------------------
// Stylesheet.

const HStyles = () => (
  <style>{`
    @keyframes tn-snow {
      0% { background-position: 0 0; } 20% { background-position: -47px 83px; }
      40% { background-position: 101px -29px; } 60% { background-position: -13px -97px; }
      80% { background-position: 67px 59px; } 100% { background-position: 0 0; }
    }
    @keyframes tn-breathe { from { filter: brightness(1); } to { filter: brightness(0.78); } }
    @keyframes tn-fade { from { opacity: 0; } to { opacity: 1; } }
    @keyframes tn-blink { 0%, 49% { opacity: 1; } 50%, 100% { opacity: 0; } }
    @keyframes tn-tick { 0%, 49% { opacity: 1; } 50%, 100% { opacity: 0.22; } }
    @keyframes tn-live { 0%, 100% { opacity: 1; } 50% { opacity: 0.3; } }
    @keyframes tn-ring-a { from { box-shadow: inset 0 0 0 2px var(--edge); } to { box-shadow: inset 0 0 0 2px transparent; } }
    @keyframes tn-ring-b { from { box-shadow: inset 0 0 0 2px var(--edge); } to { box-shadow: inset 0 0 0 2px transparent; } }
    @keyframes tn-flash-a { from { filter: brightness(1.3); } to { filter: brightness(1); } }
    @keyframes tn-flash-b { from { filter: brightness(1.3); } to { filter: brightness(1); } }

    .tn-root { font-variant-numeric: tabular-nums; }
    .tn-fade { transition: opacity 220ms ${EASE}; }
    .tn-fade[data-shown="false"] { opacity: 0; pointer-events: none; }
    .tv-osd { transition: opacity 220ms ${EASE}; }
    .tv-osd[data-shown="false"] { opacity: 0; }

    .tn-snow { animation: tn-snow 250ms steps(1, end) infinite; }
    .tn-ov[data-slow="true"] .tn-col { animation: tn-breathe 1800ms ease-in-out infinite alternate; animation-delay: calc(1000ms + var(--i) * 90ms); }
    .tn-label { animation: tn-fade 240ms linear 1000ms backwards; }
    .tn-cursor { animation: tn-blink 1s steps(1, end) infinite; }
    .tn-colon { animation: tn-tick 1s steps(1, end) infinite; }
    .tn-dot { animation: tn-live 2s ease-in-out infinite; }

    /* Keys: neutral solid surfaces. --edge is the key's brand colour, shown only as a thin edge. */
    .tn-key, .tn-half {
      --edge: ${flat.text};
      appearance: none; cursor: pointer; font: inherit; color: ${flat.text};
      align-items: center; display: inline-flex; justify-content: center;
      transition: transform 160ms ${EASE}, background-color 140ms ${EASE}, border-color 140ms ${EASE};
    }
    .tn-key { background: ${flat.elevated}; border: 1px solid ${flat.hairline}; border-radius: 6px; padding: 0; }
    .tn-key:hover { background: ${flat.focus}; border-color: var(--edge); }
    .tn-key:active { transform: scale(0.94); transition-duration: 50ms; background: ${flat.focus}; border-color: var(--edge); box-shadow: inset 0 0 0 1px var(--edge); }
    .tn-key:focus-visible, .tn-half:focus-visible { outline: 3px solid var(--edge); outline-offset: 3px; }
    .tn-half { background: transparent; border: 0; padding: 0; border-radius: 0; }
    .tn-half:hover { background: ${flat.focus}; }
    .tn-half:active { background: ${flat.focus}; box-shadow: inset 0 0 0 2px var(--edge); transition-duration: 50ms; }
    .tn-half:focus-visible { outline-offset: -4px; }
    .tn-half:active svg { transform: scale(0.88); }
    .tn-half svg { transition: transform 160ms ${EASE}; }
    .tn-key.tn-hero { background: ${flat.amber}; border-color: transparent; color: ${flat.ink}; }
    .tn-key.tn-hero:hover { background: ${flat.amber}; filter: brightness(1.08); border-color: transparent; }
    .tn-key.tn-hero:active { box-shadow: none; filter: brightness(0.92); }
    .tn-key.tn-hero:focus-visible { outline-color: ${flat.text}; }
    .tn-key[data-hit="0"], .tn-half[data-hit="0"] { animation: tn-ring-a 340ms ${EASE}; }
    .tn-key[data-hit="1"], .tn-half[data-hit="1"] { animation: tn-ring-b 340ms ${EASE}; }
    .tn-hero[data-hit="0"] { animation: tn-flash-a 300ms ${EASE}; }
    .tn-hero[data-hit="1"] { animation: tn-flash-b 300ms ${EASE}; }
    .tn-legend { transition: opacity 160ms linear; }
    [data-tune="true"] .tn-legend { opacity: 0; }

    @media (prefers-reduced-motion: reduce) {
      .tn-snow, .tn-colon, .tn-dot, .tn-cursor, .tn-ov .tn-col, .tn-label { animation: none !important; }
      .tn-key, .tn-half, .tn-half svg { transition: none; }
      .tn-key:active { transform: none; }
    }
  `}</style>
);

// ---------------------------------------------------------------------------------------------
// Snow: one grey noise tile, jittered in steps (cheap, and the same every time).

let noise: string | undefined;
const noiseUri = () => {
  if (noise) return noise;
  const size = 192;
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d");
  if (!ctx) return "";
  const image = ctx.createImageData(size, size);
  let s = 12345;
  const rand = () => {
    s = (s * 1664525 + 1013904223) % 4294967296;
    return s / 4294967296;
  };
  for (let i = 0; i < image.data.length; i += 4) {
    const v = 120 + rand() * 135;
    image.data[i] = v;
    image.data[i + 1] = v;
    image.data[i + 2] = v + 4;
    image.data[i + 3] = rand() * 200;
  }
  ctx.putImageData(image, 0, 0);
  noise = canvas.toDataURL();
  return noise;
};

// ---------------------------------------------------------------------------------------------
// The channel stack: CH over the number, ink, sized by one font-size. The same component is drawn
// huge on the bars and small in the amber block, so the number can fly from one to the other.

const ChannelStack = ({
  cursor = false,
  number,
  size,
  stackRef,
}: {
  cursor?: boolean;
  number: string;
  size: number;
  stackRef?: Ref<HTMLDivElement>;
}) => (
  <div
    aria-hidden
    ref={stackRef}
    style={{
      alignItems: "center",
      color: flat.ink,
      display: "flex",
      flexDirection: "column",
      fontFamily: MONO,
      fontSize: size,
      fontWeight: 700,
      gap: "0.05em",
      lineHeight: 1,
      transformOrigin: "center",
    }}
  >
    <span style={{ fontSize: "0.17em", fontWeight: 600, letterSpacing: "0.2em", paddingLeft: "0.2em" }}>
      CH
    </span>
    <span style={{ letterSpacing: "-0.03em" }}>
      {number}
      {cursor ? (
        <span className="tn-cursor" style={{ marginLeft: "0.04em" }}>
          _
        </span>
      ) : null}
    </span>
  </div>
);

// ---------------------------------------------------------------------------------------------
// The lower third: brand bars as the top edge, a bold amber block, then Now / progress / Next on ink.

const Chip = ({ children, dims, solid }: { children: ReactNode; dims: Dims; solid: boolean }) => (
  <span
    style={mono(dims.small - 2, 700, {
      background: solid ? flat.text : "transparent",
      border: `1px solid ${solid ? flat.text : "rgba(255, 255, 255, 0.4)"}`,
      borderRadius: 2,
      color: solid ? flat.ink : flat.textSecondary,
      letterSpacing: "0.16em",
      padding: "4px 7px 4px 9px",
      textAlign: "center",
      width: "fit-content",
    })}
  >
    {children}
  </span>
);

const TickClock = ({ dims, ms }: { dims: Dims; ms: number }) => {
  const [hm = "", meridiem = ""] = formatTime(ms).split(" ");
  const [h = "", m = ""] = hm.split(":");
  return (
    <span style={{ alignItems: "baseline", display: "inline-flex", gap: 4 }}>
      <span style={mono(dims.clock, 650, { color: flat.text })}>
        {h}
        <span className="tn-colon">:</span>
        {m}
      </span>
      <span style={mono(dims.small - 2, 600, { color: flat.textMuted })}>{meridiem}</span>
    </span>
  );
};

const LiveMark = ({ dims }: { dims: Dims }) => (
  <span style={{ alignItems: "center", display: "inline-flex", gap: 8 }}>
    <span className="tn-dot" style={{ background: flat.live, borderRadius: 4, height: 8, width: 8 }} />
    <span style={mono(dims.small, 700, { color: flat.live, letterSpacing: "0.16em" })}>LIVE</span>
  </span>
);

const GoLive = ({ dims }: { dims: Dims }) => (
  <button
    className="pp-btn"
    style={{
      ...mono(dims.small - 1, 700, { color: flat.text, letterSpacing: "0.14em" }),
      alignItems: "center",
      border: `1px solid ${flat.live}`,
      borderRadius: 4,
      display: "inline-flex",
      gap: 8,
      height: 30,
      padding: "0 12px",
    }}
    type="button"
  >
    <span style={{ background: flat.live, borderRadius: 4, height: 8, width: 8 }} />
    GO LIVE
  </button>
);

const ellipsis: CSSProperties = { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" };

const Band = ({
  blockPlateRef,
  blockRef,
  dims,
  landscape,
  plateRef,
  pointer,
  row,
  scenario,
  stripRef,
  tint,
}: {
  blockPlateRef: Ref<HTMLDivElement>;
  blockRef: Ref<HTMLDivElement>;
  dims: Dims;
  landscape: boolean;
  plateRef: Ref<HTMLDivElement>;
  pointer: boolean;
  row: ChannelRow;
  scenario: Scenario;
  stripRef: Ref<HTMLDivElement>;
  tint?: string;
}) => {
  const guide = guideOf(row);
  const behind = scenario.lagMs > 1000;
  const breakLeft = Math.max(1, Math.round((b1.stopMs - scenario.playheadMs) / MINUTE));
  return (
    <div
      style={{
        display: "flex",
        flex: 1,
        maxWidth: pointer ? 1000 : undefined,
        minWidth: 0,
        position: "relative",
        zIndex: 10,
      }}
    >
      {/* The block runs the full height beside the bars, so the amber bar never melts into it. */}
      <div
        ref={blockPlateRef}
        style={{
          alignItems: "center",
          background: tint ?? flat.amber,
          display: "flex",
          flexShrink: 0,
          justifyContent: "center",
          width: dims.blockW,
        }}
      >
        <ChannelStack number={row.number} size={dims.num} stackRef={blockRef} />
      </div>
      <div style={{ display: "flex", flex: 1, flexDirection: "column", minWidth: 0 }}>
        <div aria-hidden ref={stripRef} style={{ display: "flex", height: dims.strip }}>
          {brandChroma.map((color) => (
            <span key={color} style={{ background: color, flex: 1 }} />
          ))}
        </div>
        <div
          ref={plateRef}
          style={{
            background: flat.canvas,
            border: "1px solid rgba(255, 255, 255, 0.16)",
            borderLeft: 0,
            borderTop: 0,
            display: "flex",
            flex: 1,
            flexDirection: "column",
            gap: pointer ? 14 : 8,
            minWidth: 0,
            padding: dims.bodyPad,
          }}
        >
          <div style={{ alignItems: "center", display: "flex", gap: 16, justifyContent: "space-between" }}>
            <span
              style={{
                ...ellipsis,
                color: flat.textSecondary,
                fontFamily: BODY,
                fontSize: dims.name,
                fontWeight: 600,
                letterSpacing: "0.12em",
                textTransform: "uppercase",
              }}
            >
              {row.name}
            </span>
            <span style={{ alignItems: "center", display: "inline-flex", flexShrink: 0, gap: 16 }}>
              {behind ? <GoLive dims={dims} /> : <LiveMark dims={dims} />}
              {pointer || !behind ? <TickClock dims={dims} ms={scenario.playheadMs} /> : null}
            </span>
          </div>

          <div
            style={{
              alignItems: "center",
              columnGap: 16,
              display: "grid",
              gridTemplateColumns: `${dims.chipW}px minmax(0, 1fr)`,
              rowGap: pointer ? 12 : 8,
            }}
          >
            <Chip dims={dims} solid>
              NOW
            </Chip>
            <div
              style={{
                alignItems: pointer ? "baseline" : "flex-start",
                display: "flex",
                flexDirection: pointer ? "row" : "column",
                gap: pointer ? 16 : 2,
                minWidth: 0,
              }}
            >
              <span
                style={{
                  ...ellipsis,
                  color: flat.text,
                  fontFamily: BODY,
                  fontSize: dims.title,
                  fontWeight: 700,
                  letterSpacing: "-0.01em",
                  lineHeight: 1.1,
                }}
              >
                {guide.title}
              </span>
              {scenario.inBreak ? (
                <span
                  style={{
                    ...ellipsis,
                    color: playerChrome.breakAccent,
                    fontFamily: BODY,
                    fontSize: dims.line,
                    fontWeight: 600,
                  }}
                >
                  Back in {breakLeft} min
                </span>
              ) : guide.line ? (
                <span
                  style={{ ...ellipsis, color: flat.textSecondary, fontFamily: BODY, fontSize: dims.line }}
                >
                  {guide.line}
                </span>
              ) : null}
            </div>

            <span />
            <div style={{ alignItems: "center", display: "flex", gap: 12 }}>
              <span style={mono(dims.small, 500, { color: flat.textSecondary })}>
                {clock(current.startMs)}
              </span>
              <Slot scenario={scenario} />
              <span style={mono(dims.small, 500, { color: flat.textSecondary })}>
                {clock(current.stopMs)}
              </span>
              {landscape || !behind ? null : (
                <span style={mono(dims.small, 500, { color: flat.textMuted })}>
                  {behindLabel(scenario.lagMs)}
                </span>
              )}
            </div>

            <Chip dims={dims} solid={false}>
              NEXT
            </Chip>
            <div style={{ alignItems: "baseline", display: "flex", gap: 12, minWidth: 0 }}>
              <span style={mono(dims.small, 500, { color: flat.textSecondary })}>{guide.nextAt}</span>
              <span
                style={{
                  ...ellipsis,
                  color: flat.text,
                  fontFamily: BODY,
                  fontSize: pointer ? 19 : 14,
                  fontWeight: 500,
                }}
              >
                {guide.nextName}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------------------------
// The remote. Neutral solid keys, crisp corners, mono legends. Amber is the one colour fill (play);
// every other key's brand colour (--edge) only shows as a thin edge on hover, focus and press.

interface Hit {
  key: string;
  n: number;
}

const Legend = ({ children, pointer }: { children: string; pointer: boolean }) =>
  pointer ? (
    <span
      aria-hidden
      className="tn-legend"
      style={mono(11, 600, { color: flat.text, letterSpacing: "0.16em", textTransform: "uppercase" })}
    >
      {children}
    </span>
  ) : null;

const hitOf = (hit: Hit, key: string) => (hit.key === key ? String(hit.n % 2) : undefined);

const Remote = ({
  dims,
  hit,
  lastNumber,
  onFlip,
  onLast,
  playing,
  pointer,
}: {
  dims: Dims;
  hit: Hit;
  lastNumber?: string;
  onFlip: (dir: -1 | 1) => void;
  onLast: () => void;
  playing: boolean;
  pointer: boolean;
}) => {
  const glyph = pointer ? 26 : 22;
  const col = (children: ReactNode, key: string): ReactNode => (
    <div key={key} style={{ alignItems: "center", display: "flex", flexDirection: "column", gap: 10 }}>
      {children}
    </div>
  );
  const PlayGlyph = playing ? icons.pause : icons.play;
  const Up = icons.channelUp;
  const Down = icons.channelDown;
  const More = icons.more;
  const edge = (c: string) => ({ "--edge": c }) as CSSProperties;
  return (
    <div
      style={{
        alignItems: "flex-end",
        display: "flex",
        flexShrink: 0,
        gap: dims.keyGap,
        justifyContent: "center",
      }}
    >
      {lastNumber
        ? col(
            <>
              <button
                aria-label={`Last channel, ${lastNumber}`}
                className="tn-key"
                data-hit={hitOf(hit, "last")}
                onClick={onLast}
                style={{ ...edge(brandChroma[3]), height: dims.key, width: dims.key }}
                type="button"
              >
                <span
                  style={{ alignItems: "center", display: "inline-flex", flexDirection: "column", gap: 4 }}
                >
                  <ArrowLeftRight
                    aria-hidden
                    color={playerChrome.text}
                    size={pointer ? 18 : 16}
                    strokeWidth={2.25}
                  />
                  <span style={mono(pointer ? 14 : 12, 650, { color: flat.text })}>{lastNumber}</span>
                </span>
              </button>
              <Legend pointer={pointer}>Last</Legend>
            </>,
            "last",
          )
        : null}
      {col(
        <>
          <div
            style={{
              background: flat.elevated,
              border: `1px solid ${flat.hairline}`,
              borderRadius: 6,
              display: "flex",
              flexDirection: "column",
              overflow: "hidden",
              width: dims.rocker.w,
              ...edge(brandChroma[3]),
            }}
          >
            <button
              aria-label="Channel up"
              className="tn-half"
              data-hit={hitOf(hit, "up")}
              onClick={() => onFlip(1)}
              style={{
                ...edge(brandChroma[3]),
                alignItems: "center",
                display: "flex",
                height: dims.rocker.half,
                justifyContent: "center",
              }}
              type="button"
            >
              <Up aria-hidden color={playerChrome.text} size={glyph} strokeWidth={2.5} />
            </button>
            <span aria-hidden style={{ background: flat.hairline, height: 1 }} />
            <button
              aria-label="Channel down"
              className="tn-half"
              data-hit={hitOf(hit, "down")}
              onClick={() => onFlip(-1)}
              style={{
                ...edge(brandChroma[3]),
                alignItems: "center",
                display: "flex",
                height: dims.rocker.half,
                justifyContent: "center",
              }}
              type="button"
            >
              <Down aria-hidden color={playerChrome.text} size={glyph} strokeWidth={2.5} />
            </button>
          </div>
          <Legend pointer={pointer}>Channel</Legend>
        </>,
        "rocker",
      )}
      {col(
        <>
          <button
            aria-label={playing ? "Pause" : "Play"}
            className="tn-key tn-hero"
            data-hit={hitOf(hit, "play")}
            style={{ height: dims.hero.h, width: dims.hero.w }}
            type="button"
          >
            <PlayGlyph
              aria-hidden
              color={flat.ink}
              fill={flat.ink}
              size={glyph + 4}
              strokeLinejoin="round"
              strokeWidth={2}
              style={playing ? undefined : { transform: "translateX(2px)" }}
            />
          </button>
          <Legend pointer={pointer}>{playing ? "Pause" : "Play"}</Legend>
        </>,
        "play",
      )}
      {col(
        <>
          <button
            aria-label="More"
            className="tn-key"
            data-hit={hitOf(hit, "more")}
            style={{ ...edge(brandChroma[4]), height: dims.key, width: dims.key }}
            type="button"
          >
            <More aria-hidden color={playerChrome.text} size={glyph} strokeWidth={2.25} />
          </button>
          <Legend pointer={pointer}>More</Legend>
        </>,
        "more",
      )}
    </div>
  );
};

// ---------------------------------------------------------------------------------------------
// The tune.

type Phase = "idle" | "tune" | "handoff";

const reducedMotion = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

const play = (
  bucket: Animation[],
  el: Element | null | undefined,
  frames: Keyframe[],
  delay: number,
  duration: number,
  fill: FillMode,
  easing = "linear",
) => {
  if (el) bucket.push(el.animate(frames, { delay, duration, easing, fill }));
};

const stepAt = "steps(1, end)";

interface Layers {
  blockStack: HTMLElement | null;
  cols: HTMLElement[];
  label: HTMLElement | null;
  name: HTMLElement | null;
  overlay: HTMLElement | null;
  plates: HTMLElement[];
  readout: HTMLElement | null;
  snow: HTMLElement | null;
  stack: HTMLElement | null;
  strip: HTMLElement[];
  wash: HTMLElement | null;
}

/** Part A: release, static, sweep, number. Fill "both", so any moment of it can be frozen. */
const buildA = (bucket: Animation[], l: Layers) => {
  const reduced = reducedMotion();
  const a = (
    el: Element | null | undefined,
    frames: Keyframe[],
    delay: number,
    dur: number,
    ease = "linear",
  ) => play(bucket, el, frames, delay, dur, "both", ease);
  const away = [{ opacity: 1 }, { opacity: 0 }];
  for (const span of l.strip) a(span, away, 0, T.exit);
  for (const plate of l.plates) a(plate, away, 0, T.exit);
  // The old band's number goes with the band; the new one stays hidden until the number lands on it.
  a(l.blockStack, [{ opacity: 1 }, { opacity: 0 }], T.exit, 1, stepAt);
  if (reduced) {
    a(
      l.wash,
      [
        { backdropFilter: "grayscale(1) brightness(0.3)" },
        { backdropFilter: "grayscale(1) brightness(0.3)" },
      ],
      0,
      1,
    );
    for (const col of l.cols) a(col, [{ opacity: 0 }, { opacity: 1 }], 0, 120);
    for (const el of [l.stack, l.name]) a(el, [{ opacity: 0 }, { opacity: 1 }], 80, 140);
    return;
  }
  a(
    l.wash,
    [{ backdropFilter: "grayscale(0) brightness(1)" }, { backdropFilter: "grayscale(1) brightness(0.3)" }],
    0,
    T.drain,
    "cubic-bezier(0.3, 0, 0.2, 1)",
  );
  a(l.snow, [{ opacity: 0 }, { opacity: 1 }], T.snowAt, T.snowIn);
  l.cols.forEach((col, i) => {
    a(
      col,
      [{ transform: "scaleX(0)" }, { transform: "scaleX(1)" }],
      T.sweepAt + i * T.sweepStagger,
      T.sweepDur,
      SWEEP,
    );
  });
  a(
    l.stack,
    [
      { opacity: 0, transform: "translateY(56px) scale(1.04)" },
      { opacity: 1, transform: "translateY(0) scale(1)" },
    ],
    T.landAt,
    T.landDur,
    OUT,
  );
  a(
    l.name,
    [
      { opacity: 0, transform: "translateY(24px)" },
      { opacity: 1, transform: "translateY(0)" },
    ],
    T.nameAt,
    T.nameDur,
    OUT,
  );
};

/** Part B: the hand-off. Measured from the real band, so the bars end exactly on its top edge. */
const buildB = (bucket: Animation[], l: Layers) => {
  const reduced = reducedMotion();
  const b = (
    el: Element | null | undefined,
    frames: Keyframe[],
    delay: number,
    dur: number,
    ease = "linear",
  ) => play(bucket, el, frames, delay, dur, "forwards", ease);
  const root = l.overlay;
  if (!root) return;
  const R = root.getBoundingClientRect();
  const rel = (r: DOMRect) => ({ h: r.height, l: r.left - R.left, t: r.top - R.top, w: r.width });
  const end = T.handoffDur;
  const fade = [{ opacity: 1 }, { opacity: 0 }];
  const swapIn = [{ opacity: 0 }, { opacity: 1 }];
  const readoutFrames = [
    { opacity: 0, transform: "translateY(-10px)" },
    { opacity: 1, transform: "translateY(0)" },
  ];

  if (reduced) {
    for (const el of [...l.cols, l.stack, l.name, l.label, l.snow, l.wash]) b(el, fade, 0, 220);
    for (const span of l.strip) b(span, swapIn, 0, 220);
    for (const plate of l.plates) b(plate, swapIn, 60, 220);
    b(l.blockStack, swapIn, 60, 220);
    b(l.readout, [{ opacity: 0 }, { opacity: 1 }], 220, 200);
    return;
  }

  b(l.snow, fade, 0, 200);
  b(l.wash, fade, 80, 280, EASE);
  b(l.name, fade, 0, 120);
  b(l.label, fade, 0, 120);

  l.cols.forEach((col, i) => {
    const target = l.strip[i]?.getBoundingClientRect();
    if (!target) return;
    const to = rel(target);
    const from = { h: R.height, l: (R.width / 7) * i, t: 0, w: R.width / 7 + (i < 6 ? 1 : 0) };
    const delay = i * T.handoffStagger;
    b(
      col,
      [
        { height: `${from.h}px`, left: `${from.l}px`, top: `${from.t}px`, width: `${from.w}px` },
        { height: `${to.h}px`, left: `${to.l}px`, top: `${to.t}px`, width: `${to.w}px` },
      ],
      delay,
      end,
      HAND,
    );
    // The real strip takes over the instant the column lands on it.
    b(col, fade, delay + end, 1, stepAt);
    b(l.strip[i], swapIn, delay + end, 1, stepAt);
  });

  // The number flies into the amber block, then the block's own number takes over.
  if (l.stack && l.blockStack) {
    const s = rel(l.stack.getBoundingClientRect());
    const k = rel(l.blockStack.getBoundingClientRect());
    const dx = k.l + k.w / 2 - (s.l + s.w / 2);
    const dy = k.t + k.h / 2 - (s.t + s.h / 2);
    const scale = k.h / s.h;
    b(
      l.stack,
      [
        { opacity: 1, transform: "translate(0, 0) scale(1)" },
        { opacity: 1, transform: `translate(${dx}px, ${dy}px) scale(${scale})` },
      ],
      0,
      end,
      HAND,
    );
    b(l.stack, fade, end, 1, stepAt);
    b(l.blockStack, swapIn, end, 1, stepAt);
  }

  // The band unfolds downward from the bars.
  for (const plate of l.plates) {
    b(
      plate,
      [
        { clipPath: "inset(0 0 100% 0)", opacity: 1 },
        { clipPath: "inset(0 0 0% 0)", opacity: 1 },
      ],
      T.bodyAt,
      T.bodyDur,
      OUT,
    );
  }
  b(l.readout, readoutFrames, T.readoutAt, T.readoutDur, OUT);
};

interface TuneProps extends ChromeProps {
  freezeAt?: number | undefined;
  loop?: boolean | undefined;
}

const TuneInner = ({ device, freezeAt, loop, scenario, tint, tintFor }: TuneProps) => {
  const pointer = device === "desktop";
  const landscape = device === "landscape";
  const density = pointer ? "pointer" : "touch";
  const dims = dimsOf(device);
  const slow = scenario.slow === true || scenario.tuning === true;
  const frozenAt = freezeAt ?? (scenario.tuning ? 900 : undefined);
  const auto = scenario.tune === true || scenario.tuning === true;

  const [phase, setPhase] = useState<Phase>(auto ? "tune" : "idle");
  const [onAir, setOnAir] = useState(HOME);
  const [target, setTarget] = useState(auto ? TO_NINE : HOME);
  const [entry, setEntry] = useState("");
  const [session, setSession] = useState(1);
  const [presses, setPresses] = useState(0);
  const [lastIdx, setLastIdx] = useState(LAST);
  const [osdN, setOsdN] = useState(0);
  const [osd, setOsd] = useState(true);
  const [bug, setBug] = useState(true);
  const [hit, setHit] = useState<Hit>({ key: "", n: 0 });
  const [touched, setTouched] = useState(false);

  const phaseRef = useRef<Phase>(phase);
  const onAirRef = useRef(onAir);
  const targetRef = useRef(target);
  const lastRef = useRef(lastIdx);
  const entryRef = useRef("");
  const startRef = useRef(0);
  const pressRef = useRef(0);
  const retargetRef = useRef(false);
  const settleRef = useRef(SETTLE_MS);
  const pendingRef = useRef<number | null>(null);
  const sessionRef = useRef(session);
  sessionRef.current = session;
  const bucketA = useRef<Animation[]>([]);
  const bucketB = useRef<Animation[]>([]);

  const overlayRef = useRef<HTMLDivElement>(null);
  const washRef = useRef<HTMLDivElement>(null);
  const snowRef = useRef<HTMLDivElement>(null);
  const colRefs = useRef<(HTMLDivElement | null)[]>([]);
  const stackRef = useRef<HTMLDivElement>(null);
  const nameRef = useRef<HTMLSpanElement>(null);
  const labelRef = useRef<HTMLSpanElement>(null);
  const stripRef = useRef<HTMLDivElement>(null);
  const plateRef = useRef<HTMLDivElement>(null);
  const blockPlateRef = useRef<HTMLDivElement>(null);
  const blockRef = useRef<HTMLDivElement>(null);
  const readoutRef = useRef<HTMLDivElement>(null);
  const timer = useRef(0);

  const layers = (): Layers => ({
    blockStack: blockRef.current,
    cols: colRefs.current.filter((c): c is HTMLDivElement => c !== null),
    label: labelRef.current,
    name: nameRef.current,
    overlay: overlayRef.current,
    plates: [blockPlateRef.current, plateRef.current].filter((p): p is HTMLDivElement => p !== null),
    readout: readoutRef.current,
    snow: snowRef.current,
    stack: stackRef.current,
    strip: Array.from(stripRef.current?.children ?? []) as HTMLElement[],
    wash: washRef.current,
  });

  const cancelAll = () => {
    for (const a of [...bucketA.current, ...bucketB.current]) a.cancel();
    bucketA.current = [];
    bucketB.current = [];
  };

  const freeze = (t: number) => {
    for (const a of bucketA.current) {
      a.pause();
      a.currentTime = t;
    }
    for (const a of bucketB.current) {
      a.pause();
      a.currentTime = t - T.handoff;
    }
    for (const a of document.getAnimations()) {
      if (a instanceof CSSAnimation) {
        a.pause();
        a.currentTime = t;
      }
    }
  };

  const go = useCallback(() => {
    if (targetRef.current !== onAirRef.current) {
      lastRef.current = onAirRef.current;
      setLastIdx(onAirRef.current);
    }
    onAirRef.current = targetRef.current;
    setOnAir(targetRef.current);
    entryRef.current = "";
    setEntry("");
    phaseRef.current = "handoff";
    setPhase("handoff");
    // The readout's three seconds start with the hand-off, not with the page.
    setOsdN((n) => n + 1);
  }, []);

  const press = useCallback((to: number, key: string, settle = SETTLE_MS) => {
    setTouched(true);
    setHit((h) => ({ key, n: h.n + 1 }));
    if (phaseRef.current === "handoff") {
      pendingRef.current = to;
      return;
    }
    if (phaseRef.current === "tune") retargetRef.current = true;
    targetRef.current = to;
    setTarget(to);
    pressRef.current = performance.now();
    settleRef.current = settle;
    setPresses((n) => n + 1);
    if (phaseRef.current === "idle") {
      phaseRef.current = "tune";
      setPhase("tune");
      setSession((s) => s + 1);
    }
  }, []);

  const flip = useCallback(
    (dir: -1 | 1) => {
      entryRef.current = "";
      setEntry("");
      const from = phaseRef.current === "idle" ? onAirRef.current : targetRef.current;
      press((from + dir + channels.length) % channels.length, dir > 0 ? "up" : "down");
    },
    [press],
  );

  const type = useCallback(
    (digit: string) => {
      const wanted = `${entryRef.current}${digit}`.slice(-2);
      const first = channels.findIndex((c) => c.number.startsWith(wanted));
      if (first < 0) {
        entryRef.current = "";
        setEntry("");
        return;
      }
      const matches = channels.filter((c) => c.number.startsWith(wanted)).length;
      entryRef.current = wanted;
      setEntry(wanted);
      press(first, "digit", matches === 1 ? 600 : 1600);
    },
    [press],
  );

  const last = useCallback(() => {
    if (lastRef.current >= 0) press(lastRef.current, "last");
  }, [press]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === "ArrowUp") flip(1);
      else if (e.key === "ArrowDown") flip(-1);
      else if (e.key === "l" || e.key === "L") last();
      else if (/^[0-9]$/.test(e.key)) type(e.key);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [flip, last, type]);

  // Part A starts with every session (before paint, so there is no flash of the old band).
  // biome-ignore lint/correctness/useExhaustiveDependencies: a new session is the only trigger
  useLayoutEffect(() => {
    if (phaseRef.current !== "tune") return;
    cancelAll();
    startRef.current = performance.now();
    pressRef.current = startRef.current;
    buildA(bucketA.current, layers());
    if (frozenAt !== undefined) {
      // Before the hand-off the old channel's band is still the one on screen, so only part A exists.
      if (slow || frozenAt < T.handoff) freeze(frozenAt);
      else go();
    }
  }, [session]);

  // Part B starts when the band for the new channel is in the DOM, so it can be measured.
  // biome-ignore lint/correctness/useExhaustiveDependencies: phase and session are the triggers
  useLayoutEffect(() => {
    if (phase !== "handoff") return;
    const mine = session;
    bucketB.current = [];
    buildB(bucketB.current, layers());
    if (frozenAt !== undefined) {
      freeze(frozenAt);
      return;
    }
    Promise.all(bucketB.current.map((a) => a.finished))
      .then(() => {
        if (sessionRef.current !== mine || phaseRef.current !== "handoff") return;
        phaseRef.current = "idle";
        setPhase("idle");
        const pending = pendingRef.current;
        pendingRef.current = null;
        if (pending !== null) press(pending, "up");
      })
      .catch(() => undefined);
  }, [phase, session]);

  // Back at rest the persistent elements shed their (matching) end states.
  // biome-ignore lint/correctness/useExhaustiveDependencies: only the phase matters
  useLayoutEffect(() => {
    if (phase === "idle") cancelAll();
  }, [phase]);

  // Hand off once the viewer has stopped pressing, the beat has been read and the picture is here.
  // biome-ignore lint/correctness/useExhaustiveDependencies: presses re-arms the wait
  useEffect(() => {
    if (phase !== "tune" || frozenAt !== undefined) return;
    const due = Math.max(
      startRef.current + T.handoff,
      pressRef.current + settleRef.current,
      startRef.current + (slow ? SLOW_MS : 0),
    );
    timer.current = window.setTimeout(go, Math.max(0, due - performance.now()));
    return () => window.clearTimeout(timer.current);
  }, [phase, session, presses, slow, frozenAt, go]);

  // The viewing loop (?loop=1): back to CH 7, tune to CH 9 again.
  // biome-ignore lint/correctness/useExhaustiveDependencies: cancelAll only touches refs
  useEffect(() => {
    if (!(loop && auto) || frozenAt !== undefined) return;
    const id = window.setInterval(
      () => {
        cancelAll();
        phaseRef.current = "tune";
        onAirRef.current = HOME;
        targetRef.current = TO_NINE;
        setOnAir(HOME);
        setTarget(TO_NINE);
        setPhase("tune");
        setSession((s) => s + 1);
      },
      slow ? SLOW_MS + 2800 : 4200,
    );
    return () => window.clearInterval(id);
  }, [loop, auto, slow, frozenAt]);

  // Re-targeting during a tune nudges the number in place; it does not replay the sequence.
  const seen = useRef(target);
  useEffect(() => {
    if (seen.current === target) return;
    seen.current = target;
    // Only a press while a tune is already running re-targets; the first press starts the sequence.
    if (!retargetRef.current) return;
    retargetRef.current = false;
    stackRef.current?.animate(
      [
        { opacity: 0.35, transform: "translateY(-18px)" },
        { opacity: 1, transform: "translateY(0)" },
      ],
      { duration: 160, easing: OUT },
    );
    nameRef.current?.animate([{ opacity: 0.3 }, { opacity: 1 }], { duration: 160, easing: OUT });
  }, [target]);

  // The readout is up after every tune and fades by itself; the bug only at a programme's start.
  // biome-ignore lint/correctness/useExhaustiveDependencies: osdN is the trigger, not a value read here
  useEffect(() => {
    setOsd(true);
    if (frozenAt !== undefined) return;
    const t = window.setTimeout(() => setOsd(false), READOUT_MS);
    return () => window.clearTimeout(t);
  }, [osdN]);
  useEffect(() => {
    if (frozenAt !== undefined) return;
    const t = window.setTimeout(() => setBug(false), BUG_MS);
    return () => window.clearTimeout(t);
  }, [frozenAt]);
  useEffect(() => () => window.clearTimeout(timer.current), []);

  const noise = useMemo(() => (typeof document === "undefined" ? "" : noiseUri()), []);

  if (scenario.cover) return <Cover density={density} kind={scenario.cover} />;

  const sheet = scenario.sheet;
  const tuneOn = phase !== "idle";
  const row = channels[onAir] ?? channels[HOME] ?? channels[0];
  const aim = channels[target] ?? row;
  if (!(row && aim)) return null;
  const staticStrip = scenario.surfing === true && !touched && !tuneOn;
  const card = scenario.ident && !scenario.visible && !sheet && !tuneOn;
  const shown = (scenario.visible && !sheet) || card;
  const playing = scenario.mode !== "paused";
  const readoutOn = !sheet && !card && phase !== "tune" && osd && scenario.visible;
  const bugOn = bug && !sheet && !tuneOn && scenario.visible && !scenario.inBreak && scenario.mode === "live";
  const lastRow = channels[lastIdx];
  const legendsHidden =
    frozenAt !== undefined ? frozenAt > 60 && frozenAt < T.handoff + 200 : phase === "tune";
  const lastNumber = lastRow?.number;

  const remote = (
    <div data-tune={legendsHidden} style={{ position: "relative", zIndex: 40 }}>
      <Remote
        dims={dims}
        hit={hit}
        lastNumber={lastNumber}
        onFlip={flip}
        onLast={last}
        playing={playing}
        pointer={pointer}
      />
    </div>
  );
  const body = card ? (
    <IdentCard density={density} tint={tint} />
  ) : staticStrip ? (
    <Strip count={pointer ? 5 : 3} density={density} onAir={onAir} target={target} tintFor={tintFor} />
  ) : (
    <Band
      blockPlateRef={blockPlateRef}
      blockRef={blockRef}
      dims={dims}
      landscape={landscape}
      plateRef={plateRef}
      pointer={pointer}
      row={row}
      scenario={scenario}
      stripRef={stripRef}
      tint={tint}
    />
  );

  return (
    <div className="tn-root" style={{ inset: 0, pointerEvents: "none", position: "absolute" }}>
      <HStyles />
      <div className="tn-fade" data-shown={shown} style={{ inset: 0, position: "absolute" }}>
        <div style={scrimStyle(pointer ? 520 : landscape ? 340 : 460, true)} />
      </div>
      <ParentalBug device={device} shown={bugOn} />
      <div
        ref={readoutRef}
        style={{ inset: 0, opacity: tuneOn ? 0 : 1, pointerEvents: "none", position: "absolute" }}
      >
        <TuneReadout
          density={density}
          device={device}
          name={row.name}
          number={row.number}
          shown={readoutOn}
        />
      </div>

      {tuneOn ? (
        <div
          aria-label={`Tuning to channel ${aim.number}, ${aim.name}`}
          className="tn-ov"
          data-slow={slow}
          ref={overlayRef}
          role="status"
          style={{ inset: 0, overflow: "hidden", position: "absolute", zIndex: 30 }}
        >
          <div
            ref={washRef}
            style={{ backdropFilter: "grayscale(1) brightness(0.3)", inset: 0, position: "absolute" }}
          />
          <div
            className="tn-snow"
            ref={snowRef}
            style={{
              backgroundColor: "rgba(11, 12, 14, 0.5)",
              backgroundImage: `url("${noise}")`,
              inset: 0,
              opacity: 0,
              position: "absolute",
            }}
          />
          {brandChroma.map((color, i) => (
            <div
              className="tn-col"
              key={color}
              ref={(el) => {
                colRefs.current[i] = el;
              }}
              style={
                {
                  "--i": i,
                  background: color,
                  height: "100%",
                  left: `${(100 / 7) * i}%`,
                  position: "absolute",
                  top: 0,
                  transformOrigin: "left center",
                  width: `${100 / 7 + (i < 6 ? 0.2 : 0)}%`,
                } as CSSProperties
              }
            />
          ))}
          <div
            style={{
              alignItems: "center",
              display: "flex",
              flexDirection: "column",
              gap: pointer ? 28 : 16,
              inset: 0,
              justifyContent: "center",
              position: "absolute",
            }}
          >
            <ChannelStack
              cursor={entry !== ""}
              number={entry || aim.number}
              size={dims.big}
              stackRef={stackRef}
            />
            <span
              ref={nameRef}
              style={{
                color: flat.ink,
                fontFamily: BODY,
                fontSize: dims.nameBig,
                fontWeight: 700,
                letterSpacing: "-0.015em",
                lineHeight: 1,
                textAlign: "center",
              }}
            >
              {/* While a number is half typed the name waits, so "1" never wears "Game Night". */}
              {entry !== "" && aim.number !== entry ? " " : aim.name}
            </span>
            {slow ? (
              <span
                className="tn-label"
                ref={labelRef}
                style={mono(pointer ? 18 : 14, 650, {
                  color: flat.ink,
                  letterSpacing: "0.2em",
                  marginTop: -4,
                })}
              >
                TUNING IN
                <span className="tn-cursor">_</span>
              </span>
            ) : null}
          </div>
        </div>
      ) : null}

      <div
        style={{
          alignItems: pointer || landscape ? "flex-end" : "stretch",
          bottom: 0,
          display: "flex",
          flexDirection: pointer || landscape ? "row" : "column",
          gap: pointer ? 48 : landscape ? 24 : 16,
          left: 0,
          padding: dims.edge,
          pointerEvents: "none",
          position: "absolute",
          right: 0,
        }}
      >
        <div
          aria-hidden={!shown || undefined}
          className="tn-fade"
          data-shown={shown}
          style={{ display: "flex", flex: 1, minWidth: 0, pointerEvents: shown ? "auto" : "none" }}
        >
          {body}
        </div>
        {card ? null : (
          <div
            className="tn-fade"
            data-shown={shown}
            style={{
              alignSelf: pointer || landscape ? "flex-end" : "center",
              pointerEvents: shown ? "auto" : "none",
            }}
          >
            {remote}
          </div>
        )}
      </div>

      {sheet ? (
        <Sheet
          density={density}
          icon={sheet === "channels" ? "list" : "more"}
          panelStyle={{ pointerEvents: "auto" }}
          title={sheet === "channels" ? "Channels" : "More"}
        >
          {sheet === "channels" ? (
            <ChannelList density={density} tintFor={tintFor} />
          ) : (
            <MoreBody behind={scenario.mode !== "live"} density={density} plain />
          )}
        </Sheet>
      ) : null}
    </div>
  );
};

// The on-screen display (top right): the channel number in amber, then the name.
const TuneReadout = ({
  density,
  device,
  name,
  number,
  shown,
}: {
  density: "pointer" | "touch";
  device: string;
  name: string;
  number: string;
  shown: boolean;
}) => {
  const pointer = density === "pointer";
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
        position: "absolute",
        right: pointer ? 48 : device === "landscape" ? 24 : 16,
        textShadow: "0 1px 3px rgba(0, 0, 0, 0.85)",
        top: pointer ? 40 : device === "landscape" ? 16 : 24,
      }}
    >
      <span
        style={mono(pointer ? 96 : device === "landscape" ? 56 : 64, 700, {
          color: flat.amber,
          letterSpacing: "-0.02em",
        })}
      >
        {number}
      </span>
      <span
        style={{
          color: flat.text,
          fontFamily: BODY,
          fontSize: pointer ? 24 : 16,
          fontWeight: 650,
        }}
      >
        {name}
      </span>
    </div>
  );
};

// Keyed by state, so switching states starts that state's moment afresh.
const TuneChrome = (props: TuneProps) => <TuneInner key={props.scenario.state} {...props} />;

export { TuneChrome };
