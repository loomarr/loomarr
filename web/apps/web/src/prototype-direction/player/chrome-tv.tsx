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
import { ArrowLeftRight } from "lucide-react";
import { type CSSProperties, type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import type { ChromeProps } from "./chrome-classic";
import { nowProgramme as current, nextProgramme as next } from "./chrome-rich";
import { b1, type ChannelRow, channel, channels, formatTime, lastChannel, MINUTE, NOW } from "./data";
import { type Density, flat, scrimStyle } from "./flat";
import type { Scenario } from "./scenario";
import { stationLine } from "./scenario";
import { ChannelList, Cover, MoreBody, Sheet } from "./sheets";

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
  tint,
}: {
  density: Density;
  ident?: boolean;
  number: string;
  tint?: string;
}) => {
  const pointer = density === "pointer";
  return (
    <div
      aria-hidden
      style={{
        alignItems: "center",
        background: tint ?? flat.amber,
        borderRadius: "12px 0 0 12px",
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

const Slot = ({ scenario }: { scenario: Scenario }) => {
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
          background: flat.amber,
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
  row,
  scenario,
  tint,
}: {
  density: Density;
  guide: Guide;
  landscape: boolean;
  row: ChannelRow;
  scenario: Scenario;
  tint?: string;
}) => {
  const pointer = density === "pointer";
  const behind = scenario.lagMs > 1000;
  const breakLeft = Math.max(1, Math.round((b1.stopMs - scenario.playheadMs) / MINUTE));
  const gutter = pointer ? 40 : 36;
  return (
    <div style={{ display: "flex", flex: 1, maxWidth: pointer ? 960 : undefined, minWidth: 0 }}>
      <Block density={density} number={row.number} tint={tint} />
      <div
        style={{
          background: flat.raised,
          border: `1px solid ${flat.hairline}`,
          borderLeft: 0,
          borderRadius: "0 12px 12px 0",
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

        <div
          style={{
            alignItems: "baseline",
            columnGap: 16,
            display: "grid",
            gridTemplateColumns: `${gutter}px 1fr`,
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
            <Text
              density={density}
              numberOfLines={1}
              strong
              textRole={pointer ? "title" : "label"}
              tone="primary"
            >
              {guide.title}
            </Text>
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
              <Text
                density={density}
                numberOfLines={1}
                textRole={pointer ? "reading" : "cardMeta"}
                tone="secondary"
              >
                {guide.line}
              </Text>
            ) : null}
          </div>

          <span />
          <div style={{ alignItems: "center", display: "flex", gap: 12, minHeight: 16 }}>
            <Text density={density} textRole="cardTime" tone="secondary">
              {clock(current.startMs)}
            </Text>
            <Slot scenario={scenario} />
            <Text density={density} textRole="cardTime" tone="secondary">
              {clock(current.stopMs)}
            </Text>
          </div>

          <Tag density={density}>Next</Tag>
          <div style={{ alignItems: "baseline", display: "flex", gap: 12, minWidth: 0 }}>
            <Text density={density} textRole="cardTime" tone="secondary">
              {guide.nextAt}
            </Text>
            <Text density={density} numberOfLines={1} textRole={pointer ? "body" : "cardMeta"} tone="primary">
              {guide.nextName}
            </Text>
            {landscape && behind ? (
              <span style={{ marginLeft: "auto" }}>
                <Tag density={density}>{behindLabel(scenario.lagMs)}</Tag>
              </span>
            ) : null}
          </div>
        </div>
      </div>
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
  count,
  density,
  onAir,
  target,
  tintFor,
}: {
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
              gap: 8,
              minWidth: 0,
              padding: pointer ? 12 : 8,
            }}
          >
            {block}
            <div style={{ display: "flex", flexDirection: "column", minWidth: 0, width: "100%" }}>
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
  label,
  legend,
  ok = false,
  onPress,
  size,
  width,
}: {
  children: ReactNode;
  label: string;
  legend?: string;
  ok?: boolean;
  onPress?: () => void;
  size: number;
  width?: number;
}) => (
  <div
    style={{
      alignItems: "center",
      display: "flex",
      flexDirection: "column",
      gap: 8,
      width: legend ? 88 : undefined,
    }}
  >
    <button
      aria-label={label}
      className={ok ? "pp-btn tv-key tv-key-ok" : "pp-btn tv-key"}
      onClick={onPress}
      style={{
        alignItems: "center",
        background: ok ? playerChrome.disc : flat.elevated,
        border: ok ? undefined : `1px solid ${flat.hairline}`,
        borderRadius: 12,
        display: "inline-flex",
        flexShrink: 0,
        height: size,
        justifyContent: "center",
        width: width ?? size,
      }}
      type="button"
    >
      {children}
    </button>
    {legend ? (
      <span aria-hidden>
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
}: {
  density: Density;
  landscape: boolean;
  onFlip: (dir: -1 | 1) => void;
  playing: boolean;
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
        size={size}
      >
        <Glyph name="channelDown" size={glyph} />
      </Key>
      <Key
        label={playing ? "Pause" : "Play"}
        legend={legends ? (playing ? "Pause" : "Play") : undefined}
        ok
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
        size={size}
      >
        <Glyph name="channelUp" size={glyph} />
      </Key>
      <Key label="More" legend={legends ? "More" : undefined} size={size}>
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
  name,
  number,
  shown,
}: {
  density: Density;
  device: string;
  entry: boolean;
  name: string;
  number: string;
  shown: boolean;
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
        top: pointer ? 40 : device === "landscape" ? 16 : 24,
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
        {name}
      </Text>
    </div>
  );
};

const ParentalBug = ({ device, shown }: { device: string; shown: boolean }) => {
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
        left: pointer ? 48 : device === "landscape" ? 24 : 16,
        padding: pointer ? "8px 12px" : "4px 8px",
        pointerEvents: "none",
        position: "absolute",
        top: pointer ? 40 : device === "landscape" ? 16 : 24,
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

const TvInner = ({ device, scenario, tint, tintFor }: ChromeProps) => {
  const density: Density = device === "desktop" ? "pointer" : "touch";
  const pointer = density === "pointer";
  const landscape = device === "landscape";
  const [phase, setPhase] = useState<Phase>(
    scenario.tuning ? "tuning" : scenario.surfing ? "surfing" : "idle",
  );
  const [onAir, setOnAir] = useState(HOME);
  // The prototype's static states hold a believable moment: surfing has "1" typed (12 and 14 are
  // the choices), tuning is mid-way to channel 9.
  const [target, setTarget] = useState(scenario.tuning ? 3 : scenario.surfing ? 4 : HOME);
  const [entry, setEntry] = useState(scenario.surfing ? "1" : "");
  const [osdN, setOsdN] = useState(0);
  const [osd, setOsd] = useState(true);
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
    setOsd(true);
    const t = window.setTimeout(() => setOsd(false), READOUT_MS);
    return () => window.clearTimeout(t);
  }, [osdN]);
  useEffect(() => {
    const t = window.setTimeout(() => setBug(false), BUG_MS);
    return () => window.clearTimeout(t);
  }, []);

  if (scenario.cover) return <Cover density={density} kind={scenario.cover} />;

  const sheet = scenario.sheet;
  const tuning = phase === "tuning";
  const surfing = phase === "surfing";
  const controls = (scenario.visible || surfing) && !sheet && !tuning;
  const card = scenario.ident && !scenario.visible && !sheet && !tuning;
  const shown = controls || card;
  const row = channels[onAir] ?? channels[HOME] ?? channels[0];
  const aim = channels[target] ?? row;
  if (!(row && aim)) return null;
  const playing = scenario.mode !== "paused";
  const readout = surfing
    ? { entry: entry !== "", name: aim.name, number: entry || aim.number }
    : { entry: false, name: row.name, number: row.number };
  const readoutOn = !sheet && !tuning && !card && (surfing || (osd && scenario.visible));
  const bugOn =
    bug && !sheet && !tuning && !surfing && scenario.visible && !scenario.inBreak && scenario.mode === "live";
  const edge = pointer ? "0 48px 40px" : landscape ? "0 24px 16px" : "0 16px 24px";
  const keys = <Keys density={density} landscape={landscape} onFlip={flip} playing={playing} />;
  const body = card ? (
    <IdentCard density={density} tint={tint} />
  ) : surfing ? (
    <Strip count={pointer ? 5 : 3} density={density} onAir={onAir} target={target} tintFor={tintFor} />
  ) : (
    <LowerThird
      density={density}
      guide={guideOf(row)}
      landscape={landscape}
      row={row}
      scenario={scenario}
      tint={tint}
    />
  );

  return (
    <div className="tv-root" style={{ inset: 0, pointerEvents: "none", position: "absolute" }}>
      <TvStyles />
      <div className="tv-dress" style={{ inset: 0, opacity: shown ? 1 : 0, position: "absolute" }}>
        <div style={scrimStyle(pointer ? 520 : landscape ? 340 : 460, true)} />
      </div>
      <ParentalBug device={device} shown={bugOn} />
      <Readout density={density} device={device} {...readout} shown={readoutOn} />
      <div
        aria-hidden={!shown || undefined}
        className="tv-chrome"
        data-shown={shown}
        style={{
          alignItems: pointer || landscape ? "flex-end" : "stretch",
          bottom: 0,
          display: "flex",
          flexDirection: pointer || landscape ? "row" : "column",
          gap: pointer ? 48 : landscape ? 24 : 16,
          left: 0,
          padding: edge,
          pointerEvents: shown ? "auto" : "none",
          position: "absolute",
          right: 0,
        }}
      >
        {body}
        {card ? null : pointer || landscape ? keys : <div style={{ alignSelf: "center" }}>{keys}</div>}
      </div>
      {tuning ? <Tuning density={density} row={aim} /> : null}
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

// Keyed by state, so switching states in the prototype starts that state's moment afresh.
const TvChrome = (props: ChromeProps) => <TvInner key={props.scenario.state} {...props} />;

// Variant H (chrome-tune.tsx) builds on G's pieces and replaces only the tune and the remote.
export { behindLabel, clock, guideOf, IdentCard, mono, ParentalBug, Slot, Strip, TvChrome };
