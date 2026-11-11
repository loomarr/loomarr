// PROTOTYPE — throwaway (Direction mocks, 2026-11-09)
// Variant J's channel change: H's tune (chrome-tune.tsx, section 1 of its header), drawn for J's
// band. The rule is H's and unchanged: THE TUNE NEVER ADDS WAITING TIME, it only dresses a wait that
// already exists.
//
//   Warm flip (the target is already streaming; the default): about 340 ms and quiet. The picture
//   dips once and the seven bars run once along the band's top edge (`FlipEdge`, drawn by the band).
//   No static, no full-screen bars.
//
//   Cold start (first tune, or a channel that has to spin up): the full signature, paced by the wait.
//     release   0-180     the held picture drains
//     static    100-740   brief grey snow, thinning as the programme's fanart comes up under it
//     sweep     340-770   the seven bars wipe the full frame, hard-edged, left to right
//     number    560-840   the channel number lands, very large, in ink on the bars
//     hold      1000 on   the bars fold into a band across the middle, the fanart dimmed above and
//                         below, the band saying the channel and "Tuning in"
//     hand-off  on ready  the band drops onto the lower third's top edge and thins to a line, the
//                         fanart gives way to the picture, and J's band is there underneath
//
// The whole thing is CSS keyframes on one clock, so `?t=<ms>` freezes it (a negative delay, paused)
// and `?loop=1` replays it. Reduced motion: the bars and number appear and cross-fade out.
import { brandChroma, typography } from "@loomarr/design-system";
import type { CSSProperties } from "react";

import { flat } from "./flat";

const MONO = typography.family.data.web;
const BODY = typography.family.body.web;
const INK = "rgba(11, 12, 14, 0.94)";

const QUICK_MS = 340;
const HANDOFF_MS = 420;
const WAIT = { cold: 2000, slow: 3200 } as const;

type TuneKind = "cold" | "slow" | "warm";

/** One clock for every layer: frozen at `freezeAt`, or looping. */
const clock = (total: number, freezeAt?: number, loop?: boolean): CSSProperties => ({
  animationDuration: `${total}ms`,
  animationFillMode: "both",
  animationIterationCount: loop ? "infinite" : 1,
  animationTimingFunction: "linear",
  ...(freezeAt === undefined ? {} : { animationDelay: `-${freezeAt}ms`, animationPlayState: "paused" }),
});

/** A moment of the cold tune as a percentage of its whole length. */
const at = (ms: number, total: number) => `${Math.min(100, (ms / total) * 100).toFixed(2)}%`;

const coldKeyframes = (wait: number, lowerThird: number) => {
  const total = wait + HANDOFF_MS;
  const p = (ms: number) => at(ms, total);
  return `
    @keyframes tj-ground { 0% { opacity: 0; } ${p(180)} { opacity: 1; } ${p(wait)} { opacity: 1; } 100% { opacity: 0; } }
    @keyframes tj-snow { 0%, ${p(100)} { opacity: 0; } ${p(220)} { opacity: 1; } ${p(740)} { opacity: 0; } 100% { opacity: 0; } }
    @keyframes tj-sweep {
      0%, ${p(340)} { clip-path: inset(0 100% 0 0); top: 0; height: 100%; }
      ${p(770)} { clip-path: inset(0 0 0 0); top: 0; height: 100%; }
      ${p(1000)} { clip-path: inset(0 0 0 0); top: 0; height: 100%; }
      ${p(1320)} { clip-path: inset(0 0 0 0); top: 38%; height: 24%; }
      ${p(wait)} { clip-path: inset(0 0 0 0); top: 38%; height: 24%; opacity: 1; }
      ${p(wait + HANDOFF_MS * 0.8)} { clip-path: inset(0 0 0 0); top: calc(100% - ${lowerThird}px); height: 4px; opacity: 1; }
      100% { clip-path: inset(0 0 0 0); top: calc(100% - ${lowerThird}px); height: 4px; opacity: 0; }
    }
    @keyframes tj-number {
      0%, ${p(560)} { opacity: 0; transform: translate(-50%, -50%) scale(1.12); }
      ${p(840)} { opacity: 1; transform: translate(-50%, -50%) scale(1); }
      ${p(1000)} { opacity: 1; transform: translate(-50%, -50%) scale(1); }
      ${p(1180)} { opacity: 0; transform: translate(-50%, -50%) scale(0.5); }
      100% { opacity: 0; transform: translate(-50%, -50%) scale(0.5); }
    }
    @keyframes tj-hold { 0%, ${p(1240)} { opacity: 0; } ${p(1400)} { opacity: 1; } ${p(wait)} { opacity: 1; } ${p(wait + 120)} { opacity: 0; } 100% { opacity: 0; } }
    @keyframes tj-dip { 0% { opacity: 0; } 40% { opacity: 0.38; } 100% { opacity: 0; } }
    @keyframes tj-edge { 0% { clip-path: inset(0 100% 0 0); opacity: 1; } 55% { clip-path: inset(0 0 0 0); opacity: 1; } 100% { clip-path: inset(0 0 0 0); opacity: 0; } }
    @media (prefers-reduced-motion: reduce) {
      .tj-bars { animation-name: tj-ground !important; clip-path: none !important; }
      .tj-number { transform: translate(-50%, -50%) !important; }
    }
  `;
};

/**
 * The seven bars along the band's top edge, once: the warm flip's only flourish. The band draws
 * this over its own top edge, so it lands exactly there on every device.
 */
const FlipEdge = ({ freezeAt, loop }: { freezeAt?: number | undefined; loop?: boolean | undefined }) => (
  <div
    aria-hidden
    style={{
      animationName: "tj-edge",
      display: "flex",
      height: 4,
      left: 0,
      pointerEvents: "none",
      position: "absolute",
      right: 0,
      top: 0,
      zIndex: 2,
      ...clock(QUICK_MS, freezeAt, loop),
      animationTimingFunction: "cubic-bezier(0.7, 0, 0.2, 1)",
    }}
  >
    {brandChroma.map((color) => (
      <span key={color} style={{ background: color, flex: 1 }} />
    ))}
  </div>
);

/** How long a tune of this kind runs, for the route's ?t= and for the chrome's own timing. */
const tuneLength = (kind: TuneKind) => (kind === "warm" ? QUICK_MS : WAIT[kind] + HANDOFF_MS);

const Tune = ({
  compact,
  fanart,
  freezeAt,
  kind,
  loop,
  lowerThird,
  name,
  number,
}: {
  /** A phone: smaller type. */
  compact: boolean;
  /** The programme's fanart, shown while the picture is still coming; a flat ground without one. */
  fanart?: string | undefined;
  freezeAt?: number | undefined;
  kind: TuneKind;
  loop?: boolean | undefined;
  /** How far the band's top edge is from the bottom of the screen, where the bars land. */
  lowerThird: number;
  name: string;
  number: string;
}) => {
  if (kind === "warm") {
    return (
      <>
        <style>{coldKeyframes(WAIT.cold, lowerThird)}</style>
        <div
          aria-hidden
          style={{
            animationName: "tj-dip",
            background: "#000",
            inset: 0,
            pointerEvents: "none",
            position: "absolute",
            // No z-index: it is the first thing in the player, so the chrome paints over it and
            // only the picture dips.
            ...clock(QUICK_MS, freezeAt, loop),
          }}
        />
      </>
    );
  }
  const wait = WAIT[kind];
  const total = wait + HANDOFF_MS;
  const layer: CSSProperties = { inset: 0, pointerEvents: "none", position: "absolute" };
  return (
    <div aria-live="polite" role="status" style={{ ...layer, zIndex: 60 }}>
      <style>{coldKeyframes(wait, lowerThird)}</style>
      {/* The ground: the programme's fanart, dimmed, standing in for the picture that is coming. */}
      <div
        style={{
          ...layer,
          animationName: "tj-ground",
          background: fanart
            ? `linear-gradient(rgba(11, 12, 14, 0.72), rgba(11, 12, 14, 0.72)), ${flat.canvas} url("${fanart}") center/contain no-repeat`
            : flat.canvas,
          ...clock(total, freezeAt, loop),
        }}
      />
      {/* Static: brief, thinning as the fanart comes up. */}
      <svg
        aria-hidden
        height="100%"
        style={{ ...layer, animationName: "tj-snow", ...clock(total, freezeAt, loop) }}
        width="100%"
      >
        <filter height="100%" id="tj-snow-f" width="100%" x="0" y="0">
          <feTurbulence baseFrequency="0.9" numOctaves="1" seed="3" type="fractalNoise" />
          <feColorMatrix type="matrix" values="0 0 0 0 0.6  0 0 0 0 0.62  0 0 0 0 0.66  1.5 0 0 0 -0.64" />
        </filter>
        <rect filter="url(#tj-snow-f)" height="100%" width="100%" />
      </svg>
      {/* The seven bars: wipe the frame, fold to a band, then drop onto the lower third's edge. */}
      <div
        className="tj-bars"
        style={{
          animationName: "tj-sweep",
          display: "flex",
          left: 0,
          pointerEvents: "none",
          position: "absolute",
          right: 0,
          ...clock(total, freezeAt, loop),
        }}
      >
        {brandChroma.map((color) => (
          <span key={color} style={{ background: color, flex: 1 }} />
        ))}
      </div>
      {/* The number, very large, in ink on the bars. */}
      <span
        className="tj-number"
        style={{
          animationName: "tj-number",
          color: INK,
          fontFamily: MONO,
          fontSize: compact ? 150 : 300,
          fontWeight: 700,
          left: "50%",
          letterSpacing: "-0.04em",
          lineHeight: 1,
          position: "absolute",
          top: "50%",
          ...clock(total, freezeAt, loop),
        }}
      >
        {number}
      </span>
      {/* The hold: the band says who is coming. Ink on the bars, the one place type sits on them. */}
      <div
        style={{
          alignItems: "center",
          animationName: "tj-hold",
          display: "flex",
          gap: compact ? 12 : 24,
          height: "24%",
          justifyContent: "center",
          left: 0,
          pointerEvents: "none",
          position: "absolute",
          right: 0,
          top: "38%",
          ...clock(total, freezeAt, loop),
        }}
      >
        <span
          style={{
            color: INK,
            fontFamily: MONO,
            fontSize: compact ? 56 : 112,
            fontWeight: 700,
            letterSpacing: "-0.03em",
            lineHeight: 1,
          }}
        >
          {number}
        </span>
        <span style={{ display: "flex", flexDirection: "column", gap: 4 }}>
          <span
            style={{
              color: INK,
              fontFamily: BODY,
              fontSize: compact ? 20 : 36,
              fontWeight: 700,
              lineHeight: 1.1,
            }}
          >
            {name}
          </span>
          <span
            style={{
              color: INK,
              fontFamily: BODY,
              fontSize: compact ? 13 : 18,
              fontWeight: 500,
              opacity: 0.8,
            }}
          >
            Tuning in
          </span>
        </span>
      </div>
    </div>
  );
};

export type { TuneKind };
export { FlipEdge, Tune, tuneLength };
