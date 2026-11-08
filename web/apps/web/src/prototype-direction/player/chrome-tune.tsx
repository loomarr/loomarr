// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variant H, "Loomarr TV · signature tune". G with the channel change rebuilt, a restrained remote,
// and the utility layer a modern player is expected to have.
//
// 1. THE TUNE NEVER ADDS WAITING TIME. It only dresses a wait that already exists.
//    - Warm flip (the target is already streaming; the default): about 340 ms and quiet. The picture
//      dips once, the seven bars run once along the band's top edge only, and the number updates in
//      the channel block and the readout. No static, no full-screen bars. Rapid flips just re-target:
//      the number and band cut at once and a sweep already running is left to finish.
//    - Cold start (first tune, or a channel that must spin up): the full signature, paced by the
//      real wait. If the picture is ready almost at once (inside 960 ms) it is the warm flip instead.
//        release   0-180    the held picture drains
//        static    100-740  brief grey snow, thinning as the programme's fanart comes up under it
//        sweep     340-770  the seven bars wipe the full frame, hard-edged, left to right
//        number    560-840  the channel number lands, very large, in ink on the bars
//        hold      then     long waits (1.5 s and up) fold the bars into a band across the middle of the
//                           frame, the fanart dimmed above and below, "CH 9 Movie Matinee / TUNING IN_"
//        hand-off  on ready the bars collapse together onto the lower third's top edge, the number
//                  lands in the amber block, the band unfolds from the bars, the fanart crossfades into
//                  the live picture.
//    Reduced motion: bars and number appear, then crossfade out; no sweep, no flight.
//    ?t=<ms> freezes the tune at that moment (Web Animations API), ?loop=1 replays it.
//
// 2. THE REMOTE AND BAND. Keys are neutral solid surfaces with crisp 6 px corners and mono legends
//    on an 8 px grid; amber play is the one colour fill, every other brand colour is a thin edge on
//    hover, focus, press. The lower third has the brand bars as its top edge, a bold amber block and
//    a real contrast step. Clock colon ticks, Live dot breathes, nothing else idles.
//
// 3. LOGOS AND FANART where they serve the TV idea: the Now row sets the programme's logo in place of
//    its typed title when there is one (text when there is not: CH 9 has none); the cold tune shows the
//    programme's fanart while it waits; the station-ID card sits on dimmed fanart. Real logos and
//    fanart come from #2567, which imports them as RoleLogo / RoleBackdrop with dominantHex. Here
//    they are code-drawn stand-ins (art.tsx, shared standInStill).
//
// 4. THE UTILITY LAYER (docs/frontend-design.md section 9.4, playback-clients.md Panels). Top right:
//    Subtitles, Audio, Picture, Playback details, Picture in picture, Fullscreen, a gap, then Leave
//    (Esc). Tooltips name shortcuts ("Leave  Esc"). Subtitles on is an amber glyph with a dot. Volume
//    is a mute key with a level beside the key row. Subtitles, Audio, Picture and Channels are flat
//    right-hand sheets; Playback details is the enthusiasts' panel (mono, live-ticking, Copy details)
//    behind its own door. Phones: portrait keeps Leave and one "Subtitles and audio" key, the rest in
//    More; landscape has Leave, Subtitles, Audio and More.
//
// Interactive: ArrowUp / ArrowDown, digits, L (last channel) tune; C cycles subtitles, A audio, I
// playback details, M mute, F fullscreen, Esc closes the top layer and then leaves.
import { brandChroma, icons, playerChrome, typography } from "@loomarr/design-system";
import {
  Activity,
  ArrowLeftRight,
  AudioLines,
  Captions,
  Check,
  ChevronRight,
  Copy,
  Ellipsis,
  Maximize,
  Minimize,
  PictureInPicture2,
  SlidersHorizontal,
  Volume2,
  VolumeX,
  X,
} from "lucide-react";
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
import { standInStill } from "../shared";
import { dinerFrameUri, heistFanartUri, WORDMARK_RATIO, wordmarkUri } from "./art";
import type { ChromeProps } from "./chrome-classic";
import { nowProgramme as current } from "./chrome-rich";
import { behindLabel, clock, guideOf, IdentCard, mono, ParentalBug, Slot, Strip } from "./chrome-tv";
import {
  b1,
  type ChannelRow,
  channel,
  channels,
  formatTime,
  lastChannel,
  MINUTE,
  segments,
  WINDOW,
} from "./data";
import { flat, scrimStyle } from "./flat";
import { Playbar } from "./playbar";
import type { Scenario } from "./scenario";
import { ChannelList, Cover, Row, Sheet } from "./sheets";

// ---------------------------------------------------------------------------------------------
// The timeline, in ms from the key press.

const T = {
  drain: 180,
  exit: 160,
  snowAt: 100,
  sweepAt: 340,
  sweepStagger: 32,
  sweepDur: 240,
  landAt: 560,
  landDur: 280,
  nameAt: 620,
  nameDur: 240,
  /** The earliest the hand-off can start: the number has landed and been read. */
  handoff: 960,
  handoffDur: 260,
  bodyAt: 40,
  bodyDur: 200,
  readoutAt: 320,
  readoutDur: 220,
  /** A wait this long folds the bars into the hold band while the picture is still coming. */
  compressAt: 1000,
  compressDur: 320,
} as const;

/** The warm flip: one dip, one sweep along the band edge, the number cut in. */
const Q = {
  dip: 330,
  number: 180,
  numberAt: 40,
  readout: 200,
  readoutAt: 80,
  strip: 170,
  stripAt: 30,
  stripStagger: 22,
} as const;
const QUICK_MS = 340;

const SETTLE_MS = 420;
/** A cold start's wait for the first segment in the "cold-tune" state, and a slow stream's. */
const COLD_MS = 2000;
const SLOW_MS = 3200;
/** From this wait up the hold folds into a band so the fanart shows. */
const COMPRESS_MIN = 1500;
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
// Device metrics, all on the 8 px grid.

interface Dims {
  bandH: number;
  big: number;
  blockW: number;
  bodyPad: string;
  chipW: number;
  clock: number;
  edge: string;
  gutter: number;
  hero: { h: number; w: number };
  holdName: number;
  holdNum: number;
  key: number;
  keyGap: number;
  legendGap: number;
  line: number;
  logo: number;
  name: number;
  nameBig: number;
  num: number;
  rocker: { half: number; w: number };
  /** Distance of the top cluster and the bug from the frame's top edge. */
  side: number;
  small: number;
  strip: number;
  title: number;
  top: number;
}

const dimsOf = (device: string): Dims => {
  if (device === "desktop") {
    return {
      bandH: 208,
      big: 340,
      blockW: 144,
      bodyPad: "24px 32px",
      chipW: 64,
      clock: 20,
      edge: "0 48px 48px",
      gutter: 64,
      hero: { h: 72, w: 112 },
      holdName: 56,
      holdNum: 72,
      key: 64,
      keyGap: 24,
      legendGap: 16,
      line: 17,
      logo: 40,
      name: 14,
      nameBig: 64,
      num: 80,
      rocker: { half: 56, w: 72 },
      side: 48,
      small: 14,
      strip: 10,
      title: 36,
      top: 40,
    };
  }
  const landscape = device === "landscape";
  return {
    bandH: landscape ? 112 : 168,
    big: landscape ? 150 : 190,
    blockW: landscape ? 80 : 72,
    bodyPad: landscape ? "8px 16px" : "16px",
    chipW: 48,
    clock: landscape ? 14 : 16,
    edge: landscape ? "0 32px 24px" : "0 24px 32px",
    gutter: landscape ? 32 : 24,
    hero: { h: landscape ? 56 : 64, w: landscape ? 80 : 88 },
    holdName: landscape ? 32 : 32,
    holdNum: landscape ? 40 : 48,
    key: landscape ? 48 : 56,
    keyGap: landscape ? 16 : 24,
    legendGap: 0,
    line: 13,
    logo: 24,
    name: 12,
    nameBig: 32,
    num: 40,
    rocker: { half: landscape ? 40 : 48, w: landscape ? 56 : 64 },
    side: landscape ? 32 : 24,
    small: 12,
    strip: 8,
    title: 24,
    top: landscape ? 16 : 24,
  };
};

// ---------------------------------------------------------------------------------------------
// Art. Logos and fanart come from #2567 (RoleLogo / RoleBackdrop, dominantHex); these are code-drawn
// stand-ins. CH 7 has a logo, CH 9 has fanart but no logo (the text fallback), the rest have neither.

const fanartOf = (row: ChannelRow): string => {
  if (row.number === channel.number) return dinerFrameUri;
  if (row.number === "9") return heistFanartUri;
  return standInStill(Number(row.number) * 23 + 190);
};

const logoOf = (row: ChannelRow): { alt: string; ratio: number; src: string } | undefined =>
  row.number === channel.number ? { alt: current.name, ratio: WORDMARK_RATIO, src: wordmarkUri } : undefined;

// ---------------------------------------------------------------------------------------------
// Stylesheet.

const HStyles = () => (
  <style>{`
    @keyframes tn-snow {
      0% { background-position: 0 0; } 20% { background-position: -47px 83px; }
      40% { background-position: 101px -29px; } 60% { background-position: -13px -97px; }
      80% { background-position: 67px 59px; } 100% { background-position: 0 0; }
    }
    @keyframes tn-breathe { from { filter: brightness(1); } to { filter: brightness(0.8); } }
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
    .tn-ov[data-hold="true"] .tn-col { animation: tn-breathe 1800ms ease-in-out infinite alternate; animation-delay: calc(1500ms + var(--i) * 90ms); }
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
    .tn-key { background: ${flat.elevated}; border: 1px solid ${flat.hairline}; border-radius: 6px; padding: 0; position: relative; }
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

    /* Tooltips name the shortcut ("Leave  Esc"); keyboard focus shows them too. */
    .tn-tipwrap { display: inline-flex; position: relative; }
    .tn-tip {
      align-items: center; background: ${flat.canvas}; border: 1px solid ${flat.hairline}; border-radius: 6px;
      color: ${flat.text}; display: flex; font-family: ${BODY}; font-size: 13px; font-weight: 500; gap: 12px;
      left: 50%; opacity: 0; padding: 8px 12px; pointer-events: none; position: absolute; top: calc(100% + 8px);
      transform: translateX(-50%); transition: opacity 140ms linear; white-space: nowrap; z-index: 50;
    }
    .tn-tip[data-align="end"] { left: auto; right: 0; transform: none; }
    .tn-tipwrap:hover .tn-tip, .tn-tipwrap:has(:focus-visible) .tn-tip, .tn-tip[data-open="true"] { opacity: 1; }

    @media (prefers-reduced-motion: reduce) {
      .tn-snow, .tn-colon, .tn-dot, .tn-cursor, .tn-ov .tn-col { animation: none !important; }
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
// The channel stack: CH over the number, ink, sized by one font-size. Drawn huge on the bars and
// small in the amber block, so the number can fly from one to the other.

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
      padding: "4px 8px",
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
      height: 32,
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
  const logo = logoOf(row);
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
            <span key={color} style={{ background: color, flex: 1, transformOrigin: "left center" }} />
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
            gap: pointer ? 16 : 8,
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
              {pointer || (landscape && !behind) ? <TickClock dims={dims} ms={scenario.playheadMs} /> : null}
            </span>
          </div>

          <div
            style={{
              alignItems: "center",
              columnGap: 16,
              display: "grid",
              gridTemplateColumns: `${dims.chipW}px minmax(0, 1fr)`,
              rowGap: pointer ? 16 : 8,
            }}
          >
            <Chip dims={dims} solid>
              NOW
            </Chip>
            <div
              style={{
                alignItems: pointer ? "center" : "flex-start",
                display: "flex",
                flexDirection: pointer ? "row" : "column",
                gap: pointer ? 16 : 4,
                minHeight: dims.logo,
                minWidth: 0,
              }}
            >
              {/* A logo stands in for the typed title (same line, sized to its height); no logo, text. */}
              {logo ? (
                <img
                  alt={logo.alt}
                  height={dims.logo}
                  src={logo.src}
                  style={{
                    display: "block",
                    flexShrink: 0,
                    height: dims.logo,
                    width: dims.logo * logo.ratio,
                  }}
                  width={dims.logo * logo.ratio}
                />
              ) : (
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
              )}
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
            <div style={{ alignItems: "center", display: "flex", gap: 16 }}>
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

            {/* Next stays text: the band is not a carousel. */}
            <Chip dims={dims} solid={false}>
              NEXT
            </Chip>
            <div style={{ alignItems: "baseline", display: "flex", gap: 16, minWidth: 0 }}>
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
// The remote. Neutral solid keys, crisp corners, mono legends, air between them. Amber is the one
// colour fill (play); every other key's brand colour (--edge) is a thin edge on hover, focus, press.

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
const edge = (c: string) => ({ "--edge": c }) as CSSProperties;

interface Volume {
  level: number;
  muted: boolean;
  onMute: () => void;
}

const Remote = ({
  dims,
  hit,
  lastNumber,
  onFlip,
  onLast,
  onMore,
  playing,
  pointer,
  volume,
}: {
  dims: Dims;
  hit: Hit;
  lastNumber?: string | undefined;
  onFlip: (dir: -1 | 1) => void;
  onLast: () => void;
  onMore: () => void;
  playing: boolean;
  pointer: boolean;
  volume?: Volume | undefined;
}) => {
  const glyph = pointer ? 24 : 24;
  const col = (children: ReactNode, key: string): ReactNode => (
    <div
      key={key}
      style={{ alignItems: "center", display: "flex", flexDirection: "column", gap: dims.legendGap }}
    >
      {children}
    </div>
  );
  const PlayGlyph = playing ? icons.pause : icons.play;
  const Up = icons.channelUp;
  const Down = icons.channelDown;
  const More = icons.more;
  const VolGlyph = volume?.muted ? VolumeX : Volume2;
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
                  <ArrowLeftRight aria-hidden color={playerChrome.text} size={18} strokeWidth={2.25} />
                  <span style={mono(14, 650, { color: flat.text })}>{lastNumber}</span>
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
              boxSizing: "border-box",
              display: "flex",
              flexDirection: "column",
              height: dims.rocker.half * 2,
              overflow: "hidden",
              width: dims.rocker.w,
            }}
          >
            <button
              aria-label="Channel up"
              className="tn-half"
              data-hit={hitOf(hit, "up")}
              onClick={() => onFlip(1)}
              style={{ ...edge(brandChroma[3]), flex: 1 }}
              type="button"
            >
              <Up aria-hidden color={playerChrome.text} size={glyph} strokeWidth={2.5} />
            </button>
            <span aria-hidden style={{ background: flat.hairline, flexShrink: 0, height: 1 }} />
            <button
              aria-label="Channel down"
              className="tn-half"
              data-hit={hitOf(hit, "down")}
              onClick={() => onFlip(-1)}
              style={{ ...edge(brandChroma[3]), flex: 1 }}
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
      {volume
        ? col(
            <>
              <button
                aria-label={volume.muted ? "Unmute" : "Mute"}
                className="tn-key"
                onClick={volume.onMute}
                style={{ ...edge(brandChroma[3]), height: dims.key, width: dims.key }}
                type="button"
              >
                <VolGlyph
                  aria-hidden
                  color={volume.muted ? flat.textMuted : playerChrome.text}
                  size={glyph}
                  strokeWidth={2.25}
                  style={{ marginBottom: 8 }}
                />
                {/* The level: a thin amber line inside the key's foot. */}
                <span
                  aria-hidden
                  style={{
                    background: "rgba(255, 255, 255, 0.16)",
                    bottom: 8,
                    height: 4,
                    left: 16,
                    position: "absolute",
                    right: 16,
                  }}
                >
                  <span
                    style={{
                      background: volume.muted ? flat.textMuted : flat.amber,
                      display: "block",
                      height: 4,
                      width: `${volume.muted ? 0 : volume.level}%`,
                    }}
                  />
                </span>
              </button>
              <Legend pointer={pointer}>{volume.muted ? "Muted" : `Vol ${volume.level}`}</Legend>
            </>,
            "volume",
          )
        : null}
      {col(
        <>
          <button
            aria-label="More"
            className="tn-key"
            data-hit={hitOf(hit, "more")}
            onClick={onMore}
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
// The top-right cluster: view options, window, then Leave in the corner. Same key language.

type Panel = "audio" | "channels" | "details" | "more" | "picture" | "subtitles";

interface TopItem {
  Glyph: typeof Captions;
  active?: boolean;
  hint: string;
  id: string;
  label: string;
  pressed?: boolean;
}

const TipLabel = ({
  align,
  hint,
  label,
  open,
}: {
  align?: "end";
  hint: string;
  label: string;
  open?: boolean;
}) => (
  <span className="tn-tip" data-align={align} data-open={open} role="tooltip">
    {label}
    <span
      style={mono(11, 650, {
        border: `1px solid ${flat.hairline}`,
        borderRadius: 3,
        color: flat.textSecondary,
        padding: "3px 6px",
      })}
    >
      {hint}
    </span>
  </span>
);

const TopCluster = ({
  device,
  dims,
  fullscreen,
  onItem,
  onLeave,
  panel,
  subsOn,
  tip,
}: {
  device: string;
  dims: Dims;
  fullscreen: boolean;
  onItem: (id: string) => void;
  onLeave: () => void;
  panel: Panel | null;
  subsOn: boolean;
  tip: boolean;
}) => {
  const pointer = device === "desktop";
  const landscape = device === "landscape";
  const items: TopItem[] = pointer
    ? [
        {
          Glyph: Captions,
          active: subsOn,
          hint: "C",
          id: "subtitles",
          label: "Subtitles",
          pressed: panel === "subtitles",
        },
        { Glyph: AudioLines, hint: "A", id: "audio", label: "Audio", pressed: panel === "audio" },
        {
          Glyph: SlidersHorizontal,
          hint: "Q",
          id: "picture",
          label: "Picture",
          pressed: panel === "picture",
        },
        {
          Glyph: Activity,
          hint: "I",
          id: "details",
          label: "Playback details",
          pressed: panel === "details",
        },
        { Glyph: PictureInPicture2, hint: "P", id: "pip", label: "Picture in picture" },
        {
          Glyph: fullscreen ? Minimize : Maximize,
          hint: "F",
          id: "fullscreen",
          label: fullscreen ? "Exit fullscreen" : "Fullscreen",
        },
      ]
    : landscape
      ? [
          {
            Glyph: Captions,
            active: subsOn,
            hint: "C",
            id: "subtitles",
            label: "Subtitles",
            pressed: panel === "subtitles",
          },
          { Glyph: AudioLines, hint: "A", id: "audio", label: "Audio", pressed: panel === "audio" },
          { Glyph: Ellipsis, hint: "", id: "more", label: "More", pressed: panel === "more" },
        ]
      : [
          {
            Glyph: Captions,
            active: subsOn,
            hint: "C",
            id: "subtitles",
            label: "Subtitles and audio",
            pressed: panel === "subtitles" || panel === "audio",
          },
        ];
  const size = 48;
  return (
    <div
      style={{
        alignItems: "center",
        display: "flex",
        gap: pointer ? 16 : 16,
        position: "absolute",
        right: dims.side,
        top: dims.top,
      }}
    >
      {items.map(({ Glyph, active, hint, id, label, pressed }) => (
        <span className="tn-tipwrap" key={id}>
          <button
            aria-label={label}
            aria-pressed={pressed ?? undefined}
            className="tn-key"
            onClick={() => onItem(id)}
            style={{ ...edge(active ? flat.amber : brandChroma[3]), height: size, width: size }}
            type="button"
          >
            <Glyph aria-hidden color={active ? flat.amber : playerChrome.text} size={24} strokeWidth={2} />
            {active ? (
              <span
                aria-hidden
                style={{
                  background: flat.amber,
                  borderRadius: 4,
                  height: 8,
                  position: "absolute",
                  right: 8,
                  top: 8,
                  width: 8,
                }}
              />
            ) : null}
          </button>
          {pointer && hint ? <TipLabel hint={hint} label={label} /> : null}
        </span>
      ))}
      <span aria-hidden style={{ width: pointer ? 16 : 0 }} />
      <span className="tn-tipwrap">
        <button
          aria-label="Leave, back to Guide"
          className="tn-key"
          onClick={onLeave}
          style={{ ...edge(flat.live), gap: 8, height: size, padding: "0 16px 0 12px" }}
          type="button"
        >
          <X aria-hidden color={playerChrome.text} size={24} strokeWidth={2} />
          <span style={mono(13, 650, { color: flat.text, letterSpacing: "0.14em" })}>LEAVE</span>
        </button>
        {pointer ? <TipLabel align="end" hint="Esc" label="Leave" open={tip} /> : null}
      </span>
    </div>
  );
};

// ---------------------------------------------------------------------------------------------
// Sheets: flat right-hand panels (sheets.tsx Sheet). Plain words in Subtitles, Audio and Picture;
// the technical terms live behind Playback details only.

const SUBTITLE_OPTIONS = [
  { label: "Off" },
  { label: "English" },
  { detail: "Dialogue plus sounds", label: "English (for the deaf and hard of hearing)" },
  { label: "Spanish" },
];
const AUDIO_OPTIONS = [
  { detail: "Stereo", label: "English" },
  { detail: "Stereo", label: "Spanish" },
  { detail: "Describes what happens on screen", label: "Audio description" },
];
const PICTURE_OPTIONS = [
  { detail: "Best for your connection", label: "Auto · 1080p" },
  { label: "1080p" },
  { label: "720p" },
  { label: "480p" },
];

const OptionGroup = ({
  on,
  options,
  title,
}: {
  on: number;
  options: { detail?: string; label: string }[];
  title?: string;
}) => (
  <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
    {title ? (
      <span
        style={{
          color: flat.textSecondary,
          fontFamily: BODY,
          fontSize: 13,
          fontWeight: 600,
          letterSpacing: "0.08em",
          padding: "0 16px 8px",
          textTransform: "uppercase",
        }}
      >
        {title}
      </span>
    ) : null}
    {options.map((o, i) => (
      <button
        aria-pressed={i === on}
        className="pp-btn pp-row"
        key={o.label}
        style={{
          alignItems: "center",
          background: i === on ? flat.focus : "transparent",
          borderRadius: 6,
          display: "flex",
          gap: 16,
          minHeight: 56,
          padding: "8px 16px",
          textAlign: "left",
          width: "100%",
        }}
        type="button"
      >
        <span style={{ display: "flex", flex: 1, flexDirection: "column", gap: 4 }}>
          <span
            style={{ color: flat.text, fontFamily: BODY, fontSize: 17, fontWeight: i === on ? 600 : 400 }}
          >
            {o.label}
          </span>
          {o.detail ? (
            <span style={{ color: flat.textMuted, fontFamily: BODY, fontSize: 13 }}>{o.detail}</span>
          ) : null}
        </span>
        {i === on ? <Check aria-hidden color={flat.amber} size={20} strokeWidth={2.5} /> : null}
      </button>
    ))}
  </div>
);

const SubtitlesBody = ({ on }: { on: number }) => (
  <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
    <OptionGroup on={on} options={SUBTITLE_OPTIONS} />
    <div style={{ borderTop: `1px solid ${flat.hairline}`, paddingTop: 16 }}>
      <button
        className="pp-btn pp-row"
        style={{
          alignItems: "center",
          borderRadius: 6,
          display: "flex",
          gap: 16,
          minHeight: 56,
          padding: "0 16px",
          textAlign: "left",
          width: "100%",
        }}
        type="button"
      >
        <span style={{ color: flat.text, flex: 1, fontFamily: BODY, fontSize: 17 }}>Subtitle style</span>
        <span style={{ color: flat.textMuted, fontFamily: BODY, fontSize: 14 }}>Medium · White</span>
        <ChevronRight aria-hidden color={flat.textMuted} size={20} />
      </button>
    </div>
  </div>
);

const AudioBody = () => <OptionGroup on={0} options={AUDIO_OPTIONS} />;
const PictureBody = () => <OptionGroup on={0} options={PICTURE_OPTIONS} />;

/** The More sheet no longer repeats what is on screen: schedule, and on phones what the top bar leaves out. */
const MoreBodyH = ({ behind, pointer }: { behind: boolean; pointer: boolean }) => (
  <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
    <div>
      <span
        style={{
          color: flat.textSecondary,
          fontFamily: BODY,
          fontSize: 13,
          fontWeight: 600,
          letterSpacing: "0.08em",
          textTransform: "uppercase",
        }}
      >
        Schedule
      </span>
      <div style={{ marginTop: 16 }}>
        <Playbar
          density="touch"
          plain
          playheadMs={behind ? WINDOW.nowMs - 134_000 : WINDOW.nowMs}
          pointer={false}
          segments={segments}
        />
      </div>
    </div>
    <div>
      {behind ? <Row density="touch" glyph="startOver" label="Start over" detail="Opening Night" /> : null}
      {behind ? <Row density="touch" glyph="play" label="Go live" detail={formatTime(WINDOW.nowMs)} /> : null}
      {pointer ? null : <Row density="touch" label="Last channel" detail="CH 3" swap />}
      {pointer ? null : <Row density="touch" glyph="settings" label="Picture" detail="Auto · 1080p" />}
      {pointer ? null : <Row density="touch" glyph="info" label="Playback details" />}
      <Row density="touch" glyph="list" label="Channels" />
      <Row density="touch" glyph="info" label="Info" />
      <Row density="touch" glyph="help" label="Shortcuts" />
    </div>
  </div>
);

// ---------------------------------------------------------------------------------------------
// Playback details: the enthusiasts' panel. Mono values, numbers that tick, Copy details.

const detailRows = (tick: number, lagMs: number): [string, string][] => {
  const wobble = Math.sin(tick * 1.3);
  return [
    ["Stream", "Direct play"],
    ["Resolution", "1920×1080 · 29.97 fps"],
    ["Video", "H.264 · High@4.1"],
    ["HDR", "None (SDR)"],
    ["Audio", "AAC · 2.0 stereo"],
    ["Bitrate", `${(4.6 + wobble * 0.4).toFixed(1)} Mbps now · 4.2 avg`],
    ["Buffer ahead", `${(6.2 + Math.cos(tick * 0.9) * 0.6).toFixed(1)} s`],
    ["Dropped frames", `${Math.floor(tick / 7)} of ${(12_418 + tick * 30).toLocaleString("en-US")}`],
    ["Behind live", `${(lagMs / 1000).toFixed(1)} s`],
    ["Server", "loomarr.lan · session 4f2a"],
    ["HLS segment", `#${4127 + Math.floor(tick / 4)} · age ${(0.4 + (tick % 4) * 1.0).toFixed(1)} s`],
  ];
};

const DetailsBody = ({ lagMs, tick }: { lagMs: number; tick: number }) => {
  const [copied, setCopied] = useState(false);
  const rows = detailRows(tick, lagMs);
  const copy = () => {
    void navigator.clipboard?.writeText(rows.map(([k, v]) => `${k}: ${v}`).join("\n"));
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <dl style={{ display: "grid", gap: 8, gridTemplateColumns: "auto 1fr", margin: 0 }}>
        {rows.map(([k, v]) => (
          <div key={k} style={{ display: "contents" }}>
            <dt
              style={{ color: flat.textMuted, fontFamily: BODY, fontSize: 13, margin: 0, paddingRight: 24 }}
            >
              {k}
            </dt>
            <dd
              style={{
                ...mono(13, 500, { color: flat.text }),
                lineHeight: 1.4,
                margin: 0,
                textAlign: "right",
              }}
            >
              {v}
            </dd>
          </div>
        ))}
      </dl>
      <button
        className="tn-key"
        onClick={copy}
        style={{ ...edge(brandChroma[3]), gap: 8, height: 40, padding: "0 16px", width: "fit-content" }}
        type="button"
      >
        {copied ? <Check aria-hidden size={16} /> : <Copy aria-hidden size={16} />}
        <span style={{ fontFamily: BODY, fontSize: 14, fontWeight: 600 }}>
          {copied ? "Copied" : "Copy details"}
        </span>
      </button>
    </div>
  );
};

const DetailsPanel = ({ dims, lagMs, tick }: { dims: Dims; lagMs: number; tick: number }) => (
  <section
    aria-label="Playback details"
    style={{
      background: flat.raised,
      border: `1px solid ${flat.hairline}`,
      borderRadius: 6,
      left: dims.side,
      padding: 24,
      pointerEvents: "auto",
      position: "absolute",
      top: dims.top + 72,
      width: 424,
      zIndex: 35,
    }}
  >
    <div style={{ alignItems: "center", display: "flex", gap: 8, marginBottom: 16 }}>
      <span className="tn-dot" style={{ background: flat.amber, borderRadius: 4, height: 8, width: 8 }} />
      <span style={mono(12, 700, { color: flat.text, letterSpacing: "0.16em" })}>PLAYBACK DETAILS</span>
    </div>
    <DetailsBody lagMs={lagMs} tick={tick} />
  </section>
);

// ---------------------------------------------------------------------------------------------
// The tune: Web Animations, built from real layout so the bars end exactly on the band's edge.

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
  art: HTMLElement | null;
  blockStack: HTMLElement | null;
  body: HTMLElement | null;
  cols: HTMLElement[];
  dip: HTMLElement | null;
  hold: HTMLElement | null;
  name: HTMLElement | null;
  overlay: HTMLElement | null;
  plates: HTMLElement[];
  readout: HTMLElement | null;
  snow: HTMLElement | null;
  stack: HTMLElement | null;
  strip: HTMLElement[];
  wash: HTMLElement | null;
}

/** The warm flip: the picture dips, the bars run once along the band edge, the number is cut in. */
const buildQuick = (bucket: Animation[], l: Layers, fresh: boolean) => {
  const reduced = reducedMotion();
  const a = (
    el: Element | null | undefined,
    frames: Keyframe[],
    delay: number,
    dur: number,
    ease = "linear",
  ) => play(bucket, el, frames, delay, dur, "both", ease);
  if (reduced) {
    for (const el of [l.blockStack, l.body, l.readout]) a(el, [{ opacity: 0.4 }, { opacity: 1 }], 0, 140);
    return;
  }
  if (fresh) {
    a(
      l.dip,
      [
        { opacity: 0, offset: 0 },
        { opacity: 0.65, offset: 0.2 },
        { opacity: 0, offset: 1 },
      ],
      0,
      Q.dip,
      EASE,
    );
    l.strip.forEach((span, i) => {
      a(
        span,
        [{ transform: "scaleX(0)" }, { transform: "scaleX(1)" }],
        Q.stripAt + i * Q.stripStagger,
        Q.strip,
        OUT,
      );
    });
    a(
      l.blockStack,
      [
        { opacity: 0, transform: "translateY(12px)" },
        { opacity: 1, transform: "translateY(0)" },
      ],
      Q.numberAt,
      Q.number,
      OUT,
    );
    a(l.body, [{ opacity: 0.35 }, { opacity: 1 }], 20, 160);
    a(
      l.readout,
      [
        { opacity: 0, transform: "translateY(-8px)" },
        { opacity: 1, transform: "translateY(0)" },
      ],
      Q.readoutAt,
      Q.readout,
      OUT,
    );
    return;
  }
  // A re-target while a flip is still settling is a plain cut: only the readout nudges, so ten
  // quick flips never make the band's number blink.
  a(
    l.readout,
    [
      { opacity: 0.6, transform: "translateY(-4px)" },
      { opacity: 1, transform: "translateY(0)" },
    ],
    0,
    140,
    OUT,
  );
};

/** Part A of the cold tune: release, static, sweep, number, and for long waits the hold band. */
const buildA = (bucket: Animation[], l: Layers, compress: boolean, bandH: number) => {
  const reduced = reducedMotion();
  const a = (
    el: Element | null | undefined,
    frames: Keyframe[],
    delay: number,
    dur: number,
    ease = "linear",
  ) => play(bucket, el, frames, delay, dur, "both", ease);
  const late = (
    el: Element | null | undefined,
    frames: Keyframe[],
    delay: number,
    dur: number,
    ease = "linear",
  ) => play(bucket, el, frames, delay, dur, "forwards", ease);
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
  // Static is only a brief texture: it comes in, then thins out as the programme's fanart comes up.
  a(l.art, [{ opacity: 0 }, { opacity: 1 }], 160, 240);
  a(
    l.snow,
    [
      { opacity: 0, offset: 0 },
      { opacity: 1, offset: 0.1 },
      { opacity: 1, offset: 0.4 },
      { opacity: 0, offset: 1 },
    ],
    T.snowAt,
    640,
  );
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
  const R = l.overlay?.getBoundingClientRect();
  if (compress && R) {
    // A long wait: the bars fold into a band across the middle so the fanart shows above and below.
    const top = (R.height - bandH) / 2;
    for (const col of l.cols) {
      late(
        col,
        [
          { height: `${R.height}px`, top: "0px" },
          { height: `${bandH}px`, top: `${top}px` },
        ],
        T.compressAt,
        T.compressDur,
        HAND,
      );
    }
    late(l.stack, away, T.compressAt, 140);
    late(l.name, away, T.compressAt, 140);
    late(l.hold, [{ opacity: 0 }, { opacity: 1 }], T.compressAt + 140, 180, EASE);
  }
};

/** Part B: the hand-off. Measured from the real band, so the bars end exactly on its top edge. */
const buildB = (bucket: Animation[], l: Layers, compressed: boolean, bandH: number) => {
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
    for (const el of [...l.cols, l.stack, l.name, l.hold, l.snow, l.wash, l.art]) b(el, fade, 0, 220);
    for (const span of l.strip) b(span, swapIn, 0, 220);
    for (const plate of l.plates) b(plate, swapIn, 60, 220);
    b(l.blockStack, swapIn, 60, 220);
    b(l.readout, [{ opacity: 0 }, { opacity: 1 }], 220, 200);
    return;
  }

  b(l.wash, fade, 80, 280, EASE);
  b(l.art, fade, 60, 300, EASE);
  b(l.name, fade, 0, 120);
  b(l.hold, fade, 0, 140);

  const fromTop = compressed ? (R.height - bandH) / 2 : 0;
  const fromH = compressed ? bandH : R.height;
  // All seven bars collapse together as one band.
  l.cols.forEach((col, i) => {
    const target = l.strip[i]?.getBoundingClientRect();
    if (!target) return;
    const to = rel(target);
    const from = { h: fromH, l: (R.width / 7) * i, t: fromTop, w: R.width / 7 + (i < 6 ? 1 : 0) };
    b(
      col,
      [
        { height: `${from.h}px`, left: `${from.l}px`, top: `${from.t}px`, width: `${from.w}px` },
        { height: `${to.h}px`, left: `${to.l}px`, top: `${to.t}px`, width: `${to.w}px` },
      ],
      0,
      end,
      HAND,
    );
    // The real strip takes over the instant the column lands on it.
    b(col, fade, end, 1, stepAt);
    b(l.strip[i], swapIn, end, 1, stepAt);
  });

  if (l.stack && l.blockStack && !compressed) {
    // The number flies into the amber block, then the block's own number takes over.
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
  } else {
    // After a long hold the number is already off the frame: it rises into the block instead.
    b(
      l.blockStack,
      [
        { opacity: 0, transform: "translateY(12px)" },
        { opacity: 1, transform: "translateY(0)" },
      ],
      end - 80,
      200,
      OUT,
    );
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

const SUBS = SUBTITLE_OPTIONS.length;

const TuneInner = ({ device, freezeAt, loop, scenario, tint, tintFor }: TuneProps) => {
  const pointer = device === "desktop";
  const landscape = device === "landscape";
  const density = pointer ? "pointer" : "touch";
  const dims = dimsOf(device);
  const waitMs = scenario.cold ? COLD_MS : scenario.slow || scenario.tuning ? SLOW_MS : 0;
  // A wait this short is not worth dressing: it is the warm flip.
  const fullTune = waitMs > T.handoff;
  const compress = waitMs >= COMPRESS_MIN;
  const handoffAt = Math.max(T.handoff, waitMs);
  const frozenAt = freezeAt ?? (scenario.tuning ? 900 : undefined);
  const auto = scenario.tune === true || scenario.tuning === true;
  const autoQuick = auto && !fullTune;

  const [phase, setPhase] = useState<Phase>(auto && fullTune ? "tune" : "idle");
  const [onAir, setOnAir] = useState(autoQuick ? TO_NINE : HOME);
  const [target, setTarget] = useState(auto ? TO_NINE : HOME);
  const [entry, setEntry] = useState("");
  const [session, setSession] = useState(1);
  const [presses, setPresses] = useState(0);
  const [flipN, setFlipN] = useState(0);
  const [lastIdx, setLastIdx] = useState(LAST);
  const [osdN, setOsdN] = useState(0);
  const [osd, setOsd] = useState(true);
  const [bug, setBug] = useState(true);
  const [hit, setHit] = useState<Hit>({ key: "", n: 0 });
  const [touched, setTouched] = useState(false);
  const [panel, setPanel] = useState<Panel | null>(scenario.sheet ?? scenario.panel ?? null);
  const [subs, setSubs] = useState(scenario.panel === "subtitles" ? 1 : 0);
  const [muted, setMuted] = useState(false);
  const [level] = useState(72);
  const [fullscreen, setFullscreen] = useState(false);
  const [tick, setTick] = useState(3);

  const phaseRef = useRef<Phase>(phase);
  const onAirRef = useRef(onAir);
  const targetRef = useRef(target);
  const lastRef = useRef(lastIdx);
  const entryRef = useRef("");
  const startRef = useRef(0);
  const pressRef = useRef(0);
  const settleRef = useRef(SETTLE_MS);
  const pendingRef = useRef<number | null>(null);
  const retargetRef = useRef(false);
  const sessionRef = useRef(session);
  sessionRef.current = session;
  const quickPending = useRef(autoQuick);
  const shedRef = useRef(false);
  const quickFresh = useRef(true);
  const quickAt = useRef(0);
  const panelRef = useRef(panel);
  panelRef.current = panel;
  const bucketA = useRef<Animation[]>([]);
  const bucketB = useRef<Animation[]>([]);

  const overlayRef = useRef<HTMLDivElement>(null);
  const washRef = useRef<HTMLDivElement>(null);
  const artRef = useRef<HTMLDivElement>(null);
  const snowRef = useRef<HTMLDivElement>(null);
  const colRefs = useRef<(HTMLDivElement | null)[]>([]);
  const stackRef = useRef<HTMLDivElement>(null);
  const nameRef = useRef<HTMLSpanElement>(null);
  const holdRef = useRef<HTMLDivElement>(null);
  const stripRef = useRef<HTMLDivElement>(null);
  const plateRef = useRef<HTMLDivElement>(null);
  const blockPlateRef = useRef<HTMLDivElement>(null);
  const blockRef = useRef<HTMLDivElement>(null);
  const readoutRef = useRef<HTMLDivElement>(null);
  const dipRef = useRef<HTMLDivElement>(null);
  const timer = useRef(0);
  const entryTimer = useRef(0);

  const layers = (): Layers => ({
    art: artRef.current,
    blockStack: blockRef.current,
    body: plateRef.current,
    cols: colRefs.current.filter((c): c is HTMLDivElement => c !== null),
    dip: dipRef.current,
    hold: holdRef.current,
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
      a.currentTime = t - handoffAt;
    }
    for (const a of document.getAnimations()) {
      if (a instanceof CSSAnimation) {
        a.pause();
        a.currentTime = t;
      }
    }
  };

  // The warm flip: cut to the new channel at once; the sweep only restarts when none is running.
  const quickTo = useCallback((to: number) => {
    const now = performance.now();
    const fresh = now - quickAt.current > QUICK_MS;
    if (fresh) quickAt.current = now;
    quickFresh.current = fresh;
    if (to !== onAirRef.current) {
      lastRef.current = onAirRef.current;
      setLastIdx(onAirRef.current);
    }
    onAirRef.current = to;
    targetRef.current = to;
    setOnAir(to);
    setTarget(to);
    quickPending.current = true;
    setFlipN((n) => n + 1);
    setOsdN((n) => n + 1);
  }, []);

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

  const press = useCallback(
    (to: number, key: string, settle = SETTLE_MS) => {
      setTouched(true);
      setHit((h) => ({ key, n: h.n + 1 }));
      if (phaseRef.current === "handoff") {
        pendingRef.current = to;
        return;
      }
      if (phaseRef.current === "idle" && !fullTune) {
        quickTo(to);
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
    },
    [fullTune, quickTo],
  );

  const flip = useCallback(
    (dir: -1 | 1) => {
      entryRef.current = "";
      setEntry("");
      window.clearTimeout(entryTimer.current);
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
      if (!fullTune && phaseRef.current === "idle") {
        // Warm: the readout shows the digits typed so far, and the flip lands once typing settles.
        setTouched(true);
        setOsd(true);
        setOsdN((n) => n + 1);
        window.clearTimeout(entryTimer.current);
        entryTimer.current = window.setTimeout(
          () => {
            entryRef.current = "";
            setEntry("");
            quickTo(first);
          },
          matches === 1 ? 600 : 1600,
        );
        return;
      }
      press(first, "digit", matches === 1 ? 600 : 1600);
    },
    [fullTune, press, quickTo],
  );

  const last = useCallback(() => {
    if (lastRef.current >= 0) press(lastRef.current, "last");
  }, [press]);

  const togglePanel = useCallback((p: Panel) => setPanel((cur) => (cur === p ? null : p)), []);
  const onLeave = useCallback(() => {
    // Leave goes back to wherever the viewer came from (Guide or Home).
    if (window.history.length > 1) window.history.back();
  }, []);

  const onItem = useCallback(
    (id: string) => {
      if (id === "fullscreen") setFullscreen((f) => !f);
      else if (id === "pip") return;
      else togglePanel(id as Panel);
    },
    [togglePanel],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === "ArrowUp") flip(1);
      else if (e.key === "ArrowDown") flip(-1);
      else if (e.key === "l" || e.key === "L") last();
      else if (e.key === "c" || e.key === "C") setSubs((s) => (s + 1) % SUBS);
      else if (e.key === "a" || e.key === "A") togglePanel("audio");
      else if (e.key === "i" || e.key === "I") togglePanel("details");
      else if (e.key === "m" || e.key === "M") setMuted((m) => !m);
      else if (e.key === "f" || e.key === "F") setFullscreen((f) => !f);
      else if (e.key === "Escape") {
        // Esc closes the top layer, then leaves.
        if (panelRef.current) setPanel(null);
        else onLeave();
      } else if (/^[0-9]$/.test(e.key)) type(e.key);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [flip, last, onLeave, togglePanel, type]);

  // The warm flip's animations, built before paint so there is no flash of the old number.
  // biome-ignore lint/correctness/useExhaustiveDependencies: a new flip is the only trigger
  useLayoutEffect(() => {
    if (!quickPending.current) return;
    quickPending.current = false;
    if (quickFresh.current) cancelAll();
    buildQuick(bucketA.current, layers(), quickFresh.current);
    if (frozenAt !== undefined) freeze(frozenAt);
  }, [flipN]);

  // Part A of the cold tune starts with every session.
  // biome-ignore lint/correctness/useExhaustiveDependencies: a new session is the only trigger
  useLayoutEffect(() => {
    if (phaseRef.current !== "tune") return;
    cancelAll();
    startRef.current = performance.now();
    pressRef.current = startRef.current;
    buildA(bucketA.current, layers(), compress, dims.bandH);
    if (frozenAt !== undefined) {
      // Before the hand-off the old channel's band is still the one on screen, so only part A exists.
      if (frozenAt < handoffAt) freeze(frozenAt);
      else go();
    }
  }, [session]);

  // Part B starts when the band for the new channel is in the DOM, so it can be measured.
  // biome-ignore lint/correctness/useExhaustiveDependencies: phase and session are the triggers
  useLayoutEffect(() => {
    if (phase !== "handoff") return;
    const mine = session;
    bucketB.current = [];
    buildB(bucketB.current, layers(), compress, dims.bandH);
    if (frozenAt !== undefined) {
      freeze(frozenAt);
      return;
    }
    Promise.all(bucketB.current.map((a) => a.finished))
      .then(() => {
        if (sessionRef.current !== mine || phaseRef.current !== "handoff") return;
        phaseRef.current = "idle";
        shedRef.current = true;
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
    if (phase === "idle" && shedRef.current) {
      shedRef.current = false;
      cancelAll();
    }
  }, [phase]);

  // Hand off once the viewer has stopped pressing, the beat has been read and the picture is here.
  // The wait is the real wait: this never adds to it.
  // biome-ignore lint/correctness/useExhaustiveDependencies: presses re-arms the wait
  useEffect(() => {
    if (phase !== "tune" || frozenAt !== undefined) return;
    const due = Math.max(
      startRef.current + T.handoff,
      pressRef.current + settleRef.current,
      startRef.current + waitMs,
    );
    timer.current = window.setTimeout(go, Math.max(0, due - performance.now()));
    return () => window.clearTimeout(timer.current);
  }, [phase, session, presses, waitMs, frozenAt, go]);

  // The viewing loop (?loop=1).
  // biome-ignore lint/correctness/useExhaustiveDependencies: cancelAll only touches refs
  useEffect(() => {
    if (!(loop && auto) || frozenAt !== undefined) return;
    const id = window.setInterval(
      () => {
        if (!fullTune) {
          quickTo(onAirRef.current === HOME ? TO_NINE : HOME);
          return;
        }
        cancelAll();
        phaseRef.current = "tune";
        onAirRef.current = HOME;
        targetRef.current = TO_NINE;
        setOnAir(HOME);
        setTarget(TO_NINE);
        setPhase("tune");
        setSession((s) => s + 1);
      },
      fullTune ? handoffAt + 3200 : 1800,
    );
    return () => window.clearInterval(id);
  }, [loop, auto, fullTune, handoffAt, frozenAt, quickTo]);

  // Re-targeting during a tune nudges the number in place; it does not replay the sequence.
  const seen = useRef(target);
  useEffect(() => {
    if (seen.current === target) return;
    seen.current = target;
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
  // Playback details' numbers tick once a second while it is open.
  useEffect(() => {
    if (panel !== "details" || frozenAt !== undefined || scenario.panel === "details") return;
    const id = window.setInterval(() => setTick((n) => n + 1), 1000);
    return () => window.clearInterval(id);
  }, [panel, frozenAt, scenario.panel]);
  useEffect(() => {
    if (scenario.panel !== "details") return;
    const id = window.setInterval(() => setTick((n) => n + 1), 1000);
    return () => window.clearInterval(id);
  }, [scenario.panel]);
  useEffect(
    () => () => {
      window.clearTimeout(timer.current);
      window.clearTimeout(entryTimer.current);
    },
    [],
  );

  const noise = useMemo(() => (typeof document === "undefined" ? "" : noiseUri()), []);

  if (scenario.cover) return <Cover density={density} kind={scenario.cover} />;

  const tuneOn = phase !== "idle";
  const row = channels[onAir] ?? channels[HOME] ?? channels[0];
  const aim = channels[target] ?? row;
  if (!(row && aim)) return null;
  const detailsPanel = panel === "details" && pointer;
  const sheet = panel !== null && !detailsPanel ? panel : null;
  const staticStrip = scenario.surfing === true && !touched && !tuneOn;
  const card = scenario.ident && !scenario.visible && !sheet && !tuneOn;
  const shown = (scenario.visible && !sheet) || card;
  const playing = scenario.mode !== "paused";
  const typing = entry !== "" && !tuneOn;
  const controlsOn = scenario.visible && !sheet;
  // The chrome-top still keeps the readout away, so the Leave tooltip sits on clean picture.
  const readoutOn =
    !sheet && !card && !scenario.topTip && phase !== "tune" && (typing || (osd && scenario.visible));
  const bugOn = bug && !sheet && !tuneOn && scenario.visible && !scenario.inBreak && scenario.mode === "live";
  const lastRow = channels[lastIdx];
  const lastNumber = pointer ? lastRow?.number : undefined;
  const legendsHidden = fullTune
    ? frozenAt !== undefined
      ? frozenAt > 60 && frozenAt < handoffAt + 200
      : phase === "tune"
    : false;

  const remote = (
    <div data-tune={legendsHidden} style={{ position: "relative", zIndex: 40 }}>
      <Remote
        dims={dims}
        hit={hit}
        lastNumber={lastNumber}
        onFlip={flip}
        onLast={last}
        onMore={() => togglePanel("more")}
        playing={playing}
        pointer={pointer}
        volume={pointer ? { level, muted, onMute: () => setMuted((m) => !m) } : undefined}
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
  const touchSubs = !pointer && (sheet === "subtitles" || sheet === "audio");
  const sheetTitle =
    sheet === "subtitles"
      ? pointer
        ? "Subtitles"
        : "Subtitles and audio"
      : sheet === "audio"
        ? pointer
          ? "Audio"
          : "Subtitles and audio"
        : sheet === "picture"
          ? "Picture"
          : sheet === "details"
            ? "Playback details"
            : sheet === "channels"
              ? "Channels"
              : "More";

  return (
    <div className="tn-root" style={{ inset: 0, pointerEvents: "none", position: "absolute" }}>
      <HStyles />
      {/* The ident card sits on its programme's fanart, heavily dimmed. */}
      <div className="tn-fade" data-shown={card} style={{ inset: 0, position: "absolute" }}>
        <div
          aria-hidden
          style={{
            backgroundImage: `url("${fanartOf(row)}")`,
            backgroundPosition: "center",
            backgroundSize: "cover",
            filter: "grayscale(0.7) brightness(0.28)",
            inset: 0,
            position: "absolute",
          }}
        />
      </div>
      <div className="tn-fade" data-shown={shown} style={{ inset: 0, position: "absolute" }}>
        <div style={scrimStyle(pointer ? 520 : landscape ? 340 : 460, true)} />
      </div>
      <div ref={dipRef} style={{ background: "#000", inset: 0, opacity: 0, position: "absolute" }} />
      <ParentalBug device={device} shown={bugOn} />
      {detailsPanel ? <DetailsPanel dims={dims} lagMs={scenario.lagMs} tick={tick} /> : null}
      <div
        ref={readoutRef}
        style={{ inset: 0, opacity: phase === "tune" ? 0 : 1, pointerEvents: "none", position: "absolute" }}
      >
        <TuneReadout
          cursor={typing}
          dims={dims}
          name={typing ? " " : row.name}
          number={typing ? entry : row.number}
          pointer={pointer}
          shown={readoutOn}
        />
      </div>

      {tuneOn ? (
        <div
          aria-label={`Tuning to channel ${aim.number}, ${aim.name}`}
          className="tn-ov"
          data-hold={compress}
          ref={overlayRef}
          role="status"
          style={{ inset: 0, overflow: "hidden", position: "absolute", zIndex: 30 }}
        >
          <div
            ref={washRef}
            style={{ backdropFilter: "grayscale(1) brightness(0.3)", inset: 0, position: "absolute" }}
          />
          {/* The programme's fanart, dimmed and desaturated: the wait shows what is on. */}
          <div
            ref={artRef}
            style={{
              backgroundImage: `url("${fanartOf(aim)}")`,
              backgroundPosition: "center",
              backgroundSize: "cover",
              filter: "grayscale(0.8) brightness(0.8)",
              inset: 0,
              opacity: 0,
              position: "absolute",
            }}
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
              gap: pointer ? 32 : 16,
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
          </div>
          {/* The hold: a long wait folds the bars into a band and says what is happening. */}
          <div
            aria-hidden
            ref={holdRef}
            style={{
              alignItems: "center",
              color: flat.ink,
              display: "flex",
              flexDirection: "column",
              gap: 16,
              inset: 0,
              justifyContent: "center",
              opacity: 0,
              position: "absolute",
            }}
          >
            <div
              style={{
                alignItems: "baseline",
                display: "flex",
                flexWrap: "wrap",
                gap: "0 24px",
                justifyContent: "center",
              }}
            >
              <span style={mono(dims.holdNum, 700, { color: flat.ink, letterSpacing: "-0.03em" })}>
                <span style={{ fontSize: "0.34em", letterSpacing: "0.2em", marginRight: "0.5em" }}>CH</span>
                {aim.number}
              </span>
              <span
                style={{
                  fontFamily: BODY,
                  fontSize: dims.holdName,
                  fontWeight: 700,
                  letterSpacing: "-0.015em",
                }}
              >
                {aim.name}
              </span>
            </div>
            <span style={mono(pointer ? 18 : 14, 650, { color: flat.ink, letterSpacing: "0.2em" })}>
              TUNING IN
              <span className="tn-cursor">_</span>
            </span>
          </div>
        </div>
      ) : null}

      {/* The top cluster shows and hides with the rest of the chrome; it needs no scrim of its own. */}
      <div
        aria-hidden={!controlsOn || undefined}
        className="tn-fade"
        data-shown={controlsOn}
        style={{ inset: 0, pointerEvents: "none", position: "absolute", zIndex: 40 }}
      >
        <div style={{ pointerEvents: controlsOn ? "auto" : "none" }}>
          <TopCluster
            device={device}
            dims={dims}
            fullscreen={fullscreen}
            onItem={onItem}
            onLeave={onLeave}
            panel={panel}
            subsOn={subs > 0}
            tip={scenario.topTip === true}
          />
        </div>
      </div>

      <div
        style={{
          alignItems: pointer || landscape ? "flex-end" : "stretch",
          bottom: 0,
          display: "flex",
          flexDirection: pointer || landscape ? "row" : "column",
          gap: dims.gutter,
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
          icon={
            sheet === "details"
              ? "info"
              : sheet === "channels"
                ? "list"
                : sheet === "more"
                  ? "more"
                  : sheet === "audio"
                    ? "volume"
                    : sheet === "subtitles"
                      ? undefined // the design system has no subtitles glyph; a wrong one is worse than none
                      : "settings"
          }
          panelStyle={{ pointerEvents: "auto" }}
          title={sheetTitle}
        >
          {sheet === "channels" ? (
            <ChannelList density={density} tintFor={tintFor} />
          ) : sheet === "more" ? (
            <MoreBodyH behind={scenario.mode !== "live"} pointer={pointer} />
          ) : sheet === "details" ? (
            <DetailsBody lagMs={scenario.lagMs} tick={tick} />
          ) : sheet === "picture" ? (
            <PictureBody />
          ) : touchSubs ? (
            <div
              style={{
                display: "flex",
                flexDirection: sheet === "audio" ? "column-reverse" : "column",
                gap: 32,
              }}
            >
              <div>
                <span
                  style={{
                    color: flat.textSecondary,
                    fontFamily: BODY,
                    fontSize: 13,
                    fontWeight: 600,
                    letterSpacing: "0.08em",
                    textTransform: "uppercase",
                  }}
                >
                  Subtitles
                </span>
                <div style={{ marginTop: 16 }}>
                  <SubtitlesBody on={subs} />
                </div>
              </div>
              <div>
                <span
                  style={{
                    color: flat.textSecondary,
                    fontFamily: BODY,
                    fontSize: 13,
                    fontWeight: 600,
                    letterSpacing: "0.08em",
                    textTransform: "uppercase",
                  }}
                >
                  Audio
                </span>
                <div style={{ marginTop: 16 }}>
                  <AudioBody />
                </div>
              </div>
            </div>
          ) : sheet === "subtitles" ? (
            <SubtitlesBody on={subs} />
          ) : (
            <AudioBody />
          )}
        </Sheet>
      ) : null}
    </div>
  );
};

// The on-screen display: the channel number in amber, then the name. It sits under the top cluster,
// on the same right edge, so the two never collide.
const TuneReadout = ({
  cursor,
  dims,
  name,
  number,
  pointer,
  shown,
}: {
  cursor: boolean;
  dims: Dims;
  name: string;
  number: string;
  pointer: boolean;
  shown: boolean;
}) => (
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
      right: dims.side,
      textShadow: "0 1px 3px rgba(0, 0, 0, 0.85)",
      top: dims.top + 48 + (pointer ? 24 : 16),
    }}
  >
    <span style={mono(pointer ? 96 : dims.num + 16, 700, { color: flat.amber, letterSpacing: "-0.02em" })}>
      {number}
      {cursor ? <span className="tn-cursor">_</span> : null}
    </span>
    <span style={{ color: flat.text, fontFamily: BODY, fontSize: pointer ? 24 : 16, fontWeight: 650 }}>
      {name}
    </span>
  </div>
);

// Keyed by state, so switching states starts that state's moment afresh.
const TuneChrome = (props: TuneProps) => <TuneInner key={props.scenario.state} {...props} />;

export { TuneChrome };
