// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variants E and F: Rich flat (D) with the finish of a shipped player. The structure and content are
// D's (four controls, title block, time-true scrubber, Up next, Last channel, More sheet); what
// changes is craft: the title is the show's logo (a drawn stand-in), the picture is a film-like frame,
// the chrome takes a tint from the artwork's dominant colour, type is three sizes with tight tracking
// and no mono caps, icons are weighted to the text, spacing sits on an 8px grid, keyboard hints only
// appear on hover or focus, and the scrubber previews a frame as you move over it. Motion is real:
// chrome rises in, controls scale and brighten, the thumb grows, a channel change slides the title.
//
// E is flat: solid surfaces, 1px hairlines, one tinted bottom scrim, nothing else soft. F is the same
// component with `material="glass"`: translucent blurred panels, soft top and bottom gradients and a
// quiet shadow. F exists only to show what the flat decision costs; E is the proposal.
import { icons, playerChrome, semanticMotion, Text } from "@loomarr/design-system";
import { ArrowLeftRight } from "lucide-react";
import { type CSSProperties, type ReactNode, useCallback, useEffect, useState } from "react";
import { standInStill } from "../shared";
import { detectivePosterUri, dinerFrameUri, type Tint, tintOf, WORDMARK_RATIO, wordmarkUri } from "./art";
import type { ChromeProps } from "./chrome-classic";
import {
  BreakBadge,
  nowProgramme as current,
  nextProgramme as next,
  RatingBadge,
  SPAN,
  timeline,
} from "./chrome-rich";
import { b1, channel, formatTime, lastChannel, MINUTE, NOW, nowMeta } from "./data";
import { type Density, flat } from "./flat";
import { useWidth } from "./playbar";
import type { Scenario } from "./scenario";
import { lagLabel, stationLine } from "./scenario";
import { ChannelList, Cover, MoreBody, Sheet } from "./sheets";

type Material = "flat" | "glass";

const EASE = "cubic-bezier(0.2, 0.8, 0.2, 1)";
const { focus: FOCUS_MS, overlay: OVERLAY_MS, tune: TUNE_MS } = semanticMotion;
const BREAK_STILL = standInStill(200);

// ---------------------------------------------------------------------------------------------
// Stylesheet: motion, hover and the two materials. Inline style cannot say :hover, @keyframes or a
// reduced-motion query, so they live here; every colour is a variable set from the artwork's tint.

const PolishedStyles = () => (
  <style>{`
    @keyframes pe-rise { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: none; } }
    @keyframes pe-pop { from { opacity: 0; transform: translateY(4px); } to { opacity: 1; transform: none; } }
    @keyframes pe-slide { from { opacity: 0; transform: translateX(24px); } to { opacity: 1; transform: none; } }
    @keyframes pe-tune-up { from { opacity: 0; transform: translateY(16px); } to { opacity: 1; transform: none; } }
    @keyframes pe-tune-down { from { opacity: 0; transform: translateY(-16px); } to { opacity: 1; transform: none; } }
    @keyframes pe-fade { from { opacity: 0; } to { opacity: 1; } }

    .pe-root { font-variant-numeric: tabular-nums; }
    .pe-root[data-material="flat"] {
      --pe-fill: var(--pe-raised-hi); --pe-fill-hi: var(--pe-raised-top); --pe-line: var(--pe-hairline);
      --pe-card: var(--pe-raised); --pe-sheet: var(--pe-raised);
      --pe-blur: none; --pe-shadow: none; --pe-sheet-shadow: none; --pe-logo-shadow: none;
    }
    .pe-root[data-material="glass"] {
      --pe-fill: rgba(255, 255, 255, 0.16); --pe-fill-hi: rgba(255, 255, 255, 0.28); --pe-line: rgba(255, 255, 255, 0.24);
      --pe-card: rgba(255, 255, 255, 0.14); --pe-sheet: rgba(30, 26, 24, 0.5);
      --pe-blur: blur(28px) saturate(1.8); --pe-shadow: 0 8px 24px rgba(0, 0, 0, 0.35), inset 0 1px 0 rgba(255, 255, 255, 0.28);
      --pe-sheet-shadow: -16px 0 48px rgba(0, 0, 0, 0.4); --pe-logo-shadow: drop-shadow(0 2px 14px rgba(0, 0, 0, 0.55));
    }
    .pe-surface {
      background: var(--pe-fill); border: 1px solid var(--pe-line);
      -webkit-backdrop-filter: var(--pe-blur); backdrop-filter: var(--pe-blur); box-shadow: var(--pe-shadow);
    }
    .pe-card { background: var(--pe-card); }
    .pe-logo { filter: var(--pe-logo-shadow); }

    .pe-chrome { transition: opacity ${OVERLAY_MS}ms ${EASE}, transform ${OVERLAY_MS}ms ${EASE}; }
    .pe-chrome[data-shown="true"] { animation: pe-rise ${OVERLAY_MS}ms ${EASE} backwards; }
    .pe-chrome[data-shown="false"] { opacity: 0; transform: translateY(12px); pointer-events: none; }
    .pe-dress { transition: opacity ${OVERLAY_MS}ms ${EASE}; }

    .pe-btn { transition: transform ${FOCUS_MS}ms ${EASE}, background-color ${FOCUS_MS}ms ${EASE}; }
    .pe-btn.pe-fill { background: var(--pe-fill); }
    .pe-ctl:hover .pe-btn, .pe-ctl:focus-within .pe-btn, .pe-btn:hover, .pe-btn:focus-visible { transform: scale(1.06); }
    .pe-ctl:hover .pe-btn.pe-fill, .pe-ctl:focus-within .pe-btn.pe-fill, .pe-btn.pe-fill:hover { background: var(--pe-fill-hi); }
    .pe-btn:active { transform: scale(0.97); }
    .pe-btn:focus-visible { outline: 2px solid var(--pe-accent); outline-offset: 3px; }
    .pe-disc { box-shadow: var(--pe-shadow); }

    .pe-key { display: inline-flex; max-width: 0; margin-left: 0; opacity: 0; overflow: hidden;
      transition: max-width ${FOCUS_MS}ms ${EASE}, margin-left ${FOCUS_MS}ms ${EASE}, opacity ${FOCUS_MS}ms ${EASE}; }
    .pe-ctl:hover .pe-key, .pe-ctl:focus-within .pe-key { max-width: 64px; margin-left: 8px; opacity: 1; }

    .pe-slider:focus-visible { outline: none; }
    .pe-slider:focus-visible .pe-thumb { outline: 2px solid var(--pe-accent); outline-offset: 3px; }
    .pe-root[data-material="glass"] .pe-dim { background: rgba(0, 0, 0, 0.3) !important; }
    .pe-pop { animation: pe-pop ${FOCUS_MS}ms ${EASE} backwards; }
    .pe-sheet { animation: pe-slide ${OVERLAY_MS}ms ${EASE} backwards; }
    .pe-dim { animation: pe-fade ${OVERLAY_MS}ms ${EASE} backwards; }
    .pe-tune-up { animation: pe-tune-up ${TUNE_MS}ms ${EASE} backwards; }
    .pe-tune-down { animation: pe-tune-down ${TUNE_MS}ms ${EASE} backwards; }

    @media (prefers-reduced-motion: reduce) {
      .pe-chrome[data-shown="true"], .pe-pop, .pe-sheet, .pe-dim, .pe-tune-up, .pe-tune-down { animation-name: pe-fade; }
      .pe-chrome[data-shown="false"] { transform: none; }
      .pe-btn, .pe-ctl:hover .pe-btn, .pe-ctl:focus-within .pe-btn, .pe-btn:hover, .pe-btn:focus-visible, .pe-btn:active { transform: none; }
    }
  `}</style>
);

// ---------------------------------------------------------------------------------------------
// Small parts. Three type sizes carry the chrome: reading (16), body/label (14), caption (12).

const Time = ({ children, density }: { children: ReactNode; density: Density }) => (
  <Text density={density} textRole="cardMeta" tone="secondary">
    {children}
  </Text>
);

const Dot = ({ density }: { density: Density }) => (
  <Text density={density} textRole="body" tone="muted">
    ·
  </Text>
);

const Meta = ({ density, items }: { density: Density; items: { key: string; node: ReactNode }[] }) => (
  <div style={{ alignItems: "center", display: "flex", flexWrap: "wrap", gap: "0 8px" }}>
    {items.map((item, i) => (
      <span key={item.key} style={{ alignItems: "center", display: "inline-flex", gap: 8 }}>
        {i > 0 ? <Dot density={density} /> : null}
        {typeof item.node === "string" ? (
          <Text density={density} textRole="body" tone="secondary">
            {item.node}
          </Text>
        ) : (
          item.node
        )}
      </span>
    ))}
  </div>
);

const ChannelChip = ({ density, tint }: { density: Density; tint?: string }) => (
  <div style={{ alignItems: "center", display: "flex", gap: 8 }}>
    <span
      style={{
        alignItems: "center",
        background: tint ?? flat.amber,
        borderRadius: 4,
        display: "inline-flex",
        height: 24,
        padding: "0 8px",
      }}
    >
      <Text density={density} strong textRole="label" tone="inverse" tracking={0.02}>
        CH {channel.number}
      </Text>
    </span>
    <Text density={density} strong textRole="label" tone="primary">
      {channel.name}
    </Text>
  </div>
);

// The show's logo. See art.tsx: the real app would set a clear-logo image from the media server here.
const Logo = ({ height }: { height: number }) => (
  <img
    alt={current.name}
    className="pe-logo"
    height={height}
    src={wordmarkUri}
    style={{ display: "block", height, maxWidth: "100%", width: "auto" }}
    width={Math.round(height * WORDMARK_RATIO)}
  />
);

// A round control: solid fill and hairline in E, a blurred pane in F (both from `.pe-surface`).
const Round = ({
  children,
  label,
  onPress,
  size,
}: {
  children: ReactNode;
  label: string;
  onPress?: () => void;
  size: number;
}) => (
  <button
    aria-label={label}
    className="pp-btn pe-btn pe-surface pe-fill"
    onClick={onPress}
    style={{
      alignItems: "center",
      borderRadius: 999,
      display: "inline-flex",
      flexShrink: 0,
      height: size,
      justifyContent: "center",
      width: size,
    }}
    type="button"
  >
    {children}
  </button>
);

// Glyphs: stroke 2 at 28px reads like Geist 600 at 14px, so the icons and the type carry one weight.
const Glyph = ({ name, size = 28 }: { name: keyof typeof icons; size?: number }) => {
  const Svg = icons[name];
  return <Svg aria-hidden color={playerChrome.text} size={size} strokeWidth={2} />;
};

const PlayButton = ({ playing, size }: { playing: boolean; size: number }) => {
  const Svg = playing ? icons.pause : icons.play;
  return (
    <button
      aria-label={playing ? "Pause" : "Play"}
      className="pp-btn pe-btn pe-disc"
      style={{
        alignItems: "center",
        background: playerChrome.disc,
        borderRadius: 999,
        display: "inline-flex",
        flexShrink: 0,
        height: size,
        justifyContent: "center",
        width: size,
      }}
      type="button"
    >
      <Svg
        aria-hidden
        color={playerChrome.discInk}
        fill={playerChrome.discInk}
        size={size / 2}
        strokeLinejoin="round"
        strokeWidth={2}
        // A triangle's weight sits left of its box; nudge it to the disc's optical centre.
        style={playing ? undefined : { transform: "translateX(2px)" }}
      />
    </button>
  );
};

// A control with its name under it (desktop). The key appears beside the name on hover or focus only.
const Control = ({ children, hint, label }: { children: ReactNode; hint: string; label: string }) => (
  <div
    className="pe-ctl"
    style={{ alignItems: "center", display: "flex", flexDirection: "column", gap: 8, width: 120 }}
  >
    <div style={{ alignItems: "center", display: "flex", height: 72 }}>{children}</div>
    <span aria-hidden style={{ alignItems: "center", display: "inline-flex", height: 16 }}>
      <Text density="pointer" textRole="cardMeta" tone="secondary">
        {label}
      </Text>
      <span className="pe-key">
        <span style={{ border: "1px solid var(--pe-line)", borderRadius: 4, padding: "0 4px" }}>
          <Text density="pointer" strong textRole="cardMeta" tone="primary">
            {hint}
          </Text>
        </span>
      </span>
    </span>
  </div>
);

const GoLive = ({ density, size }: { density: Density; size: number }) => (
  <button
    className="pp-btn pe-btn pe-surface"
    style={{
      alignItems: "center",
      background: "transparent",
      border: `1px solid ${flat.live}`,
      borderRadius: 999,
      display: "inline-flex",
      gap: 8,
      height: size,
      padding: "0 16px",
    }}
    type="button"
  >
    <span style={{ background: flat.live, borderRadius: 4, height: 8, width: 8 }} />
    <Text density={density} strong textRole="label" tone="live">
      Go live
    </Text>
  </button>
);

const LastChannelPill = () => (
  <button
    aria-label={`Last channel, ${lastChannel?.number}`}
    className="pp-btn pe-btn pe-surface pe-fill"
    style={{
      alignItems: "center",
      borderRadius: 999,
      display: "inline-flex",
      gap: 8,
      height: 40,
      padding: "0 16px",
    }}
    type="button"
  >
    <ArrowLeftRight aria-hidden color={playerChrome.text} size={18} strokeWidth={2} />
    <Text density="pointer" strong textRole="label" tone="primary">
      Last channel
    </Text>
    <Text density="pointer" strong textRole="label" tone="muted">
      {lastChannel?.number}
    </Text>
  </button>
);

const UpNext = ({ clock, inBreak }: { clock: string; inBreak: boolean }) => {
  const item = inBreak ? current : next;
  return (
    <div
      className="pe-surface pe-card"
      style={{
        alignItems: "center",
        borderRadius: 12,
        display: "flex",
        flexShrink: 0,
        gap: 16,
        padding: "8px 24px 8px 8px",
        width: 360,
      }}
    >
      <img
        alt=""
        height={72}
        src={inBreak ? dinerFrameUri : detectivePosterUri}
        style={{ borderRadius: 6, flexShrink: 0, objectFit: "cover" }}
        width={48}
      />
      <div style={{ display: "flex", flexDirection: "column", minWidth: 0 }}>
        <Text density="pointer" textRole="cardMeta" tone="muted">
          {inBreak ? "Back to" : "Up next"}
        </Text>
        <Text density="pointer" numberOfLines={1} strong textRole="label" tone="primary">
          {item.name}
        </Text>
        <Text density="pointer" numberOfLines={1} textRole="cardMeta" tone="secondary">
          {inBreak
            ? `Ends ${formatTime(current.stopMs)}`
            : `${clock} · ${item.chips.filter((c) => c !== "TV-PG").join(" · ")}`}
        </Text>
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------------------------
// The scrubber: the evening as one time-true track (programmes white-ish, breaks blue), played part in
// the artwork's accent. Hover or focus grows the track and thumb; hovering shows a frame and the time
// at that point; dragging moves the thumb.

const Scrubber = ({
  density,
  pointer,
  scenario,
  tint,
}: {
  density: Density;
  pointer: boolean;
  scenario: Scenario;
  tint: Tint;
}) => {
  const { ref, width } = useWidth();
  const { boxes, xAt } = timeline(width);
  const [hover, setHover] = useState<number | null>(null);
  const [drag, setDrag] = useState(false);
  const [focused, setFocused] = useState(false);
  const liveX = xAt(NOW);
  const restX = xAt(Math.min(scenario.playheadMs, NOW));
  const head = drag && hover !== null ? hover : restX;
  const active = pointer && (hover !== null || focused || drag);
  const track = pointer ? (active ? 8 : 4) : 6;
  const holder = pointer ? 24 : 32;
  const thumb = 16;
  const behind = scenario.lagMs > 1000;
  const msAt = (x: number) =>
    Math.min(SPAN.startMs + (x / Math.max(width, 1)) * (SPAN.endMs - SPAN.startMs), NOW);
  const hoverMs = hover === null ? null : msAt(hover);
  const hoverBox =
    hover === null ? undefined : (boxes.find((b) => hover >= b.x && hover <= b.x + b.width + 3) ?? boxes[1]);
  const hoverBreak = hoverBox?.segment.kind === "break";
  const place = (clientX: number, el: HTMLElement) => {
    const rect = el.getBoundingClientRect();
    setHover(Math.min(Math.max(clientX - rect.left, 0), liveX));
  };
  const edge = (ms: number) => (
    <div style={{ alignItems: "center", display: "flex", height: holder }}>
      <Time density={density}>{formatTime(ms)}</Time>
    </div>
  );
  const bubbleW = 192;
  return (
    <div style={{ alignItems: "flex-start", display: "flex", gap: 16 }}>
      {pointer ? edge(SPAN.startMs) : null}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div
          aria-label="Position in the evening"
          aria-valuemax={Math.round(SPAN.endMs / MINUTE)}
          aria-valuemin={Math.round(SPAN.startMs / MINUTE)}
          aria-valuenow={Math.round(Math.min(scenario.playheadMs, NOW) / MINUTE)}
          aria-valuetext={formatTime(scenario.playheadMs)}
          className="pe-slider"
          onBlur={() => setFocused(false)}
          onFocus={() => setFocused(true)}
          onPointerCancel={() => setDrag(false)}
          onPointerDown={(e) => {
            e.currentTarget.setPointerCapture(e.pointerId);
            setDrag(true);
            place(e.clientX, e.currentTarget);
          }}
          onPointerLeave={() => (drag ? undefined : setHover(null))}
          onPointerMove={(e) => place(e.clientX, e.currentTarget)}
          onPointerUp={() => setDrag(false)}
          ref={ref}
          role="slider"
          style={{ cursor: "pointer", height: holder, position: "relative", touchAction: "none" }}
          tabIndex={0}
        >
          {width > 0 ? (
            <>
              {boxes.map((box) => {
                const isBreak = box.segment.kind === "break";
                const played = Math.min(Math.max(head - box.x, 0), box.width);
                return (
                  <div
                    key={box.segment.id}
                    style={{
                      background: isBreak ? flat.infoSurface : "rgba(255, 255, 255, 0.24)",
                      borderRadius: track / 2,
                      height: track,
                      left: box.x,
                      overflow: "hidden",
                      position: "absolute",
                      top: "50%",
                      transform: "translateY(-50%)",
                      transition: `height ${FOCUS_MS}ms ${EASE}, border-radius ${FOCUS_MS}ms ${EASE}`,
                      width: box.width,
                    }}
                  >
                    {isBreak && played === 0 ? (
                      <div style={{ background: flat.info, height: "100%", opacity: 0.55, width: "100%" }} />
                    ) : null}
                    <div
                      style={{
                        background: isBreak ? flat.info : tint.accent,
                        height: "100%",
                        transition: drag ? "none" : `width ${TUNE_MS}ms ${EASE}`,
                        width: played,
                      }}
                    />
                  </div>
                );
              })}
              {behind ? (
                <div
                  style={{
                    background: flat.live,
                    height: thumb + 8,
                    left: liveX - 1,
                    position: "absolute",
                    top: (holder - thumb - 8) / 2,
                    width: 2,
                  }}
                />
              ) : null}
              {hover !== null && !drag ? (
                <div
                  style={{
                    background: "rgba(255, 255, 255, 0.5)",
                    height: track,
                    left: hover - 1,
                    position: "absolute",
                    top: (holder - track) / 2,
                    width: 2,
                  }}
                />
              ) : null}
              <div
                className="pe-thumb"
                style={{
                  background: playerChrome.text,
                  borderRadius: thumb,
                  height: thumb,
                  left: head - thumb / 2,
                  position: "absolute",
                  top: (holder - thumb) / 2,
                  transform: `scale(${pointer && !active ? 0.75 : 1})`,
                  transition: `${drag ? "" : `left ${TUNE_MS}ms ${EASE}, `}transform ${FOCUS_MS}ms ${EASE}`,
                  width: thumb,
                }}
              />
              {hover !== null && hoverMs !== null ? (
                <div
                  aria-hidden
                  className="pe-pop pe-surface pe-card"
                  style={{
                    borderRadius: 12,
                    bottom: holder + 8,
                    display: "flex",
                    flexDirection: "column",
                    gap: 8,
                    left: Math.min(Math.max(hover - bubbleW / 2, 0), Math.max(width - bubbleW, 0)),
                    padding: 8,
                    pointerEvents: "none",
                    position: "absolute",
                    width: bubbleW,
                    zIndex: 5,
                  }}
                >
                  <div
                    style={{
                      background: `${tint.raised} url("${hoverBreak ? BREAK_STILL : dinerFrameUri}") center/cover`,
                      borderRadius: 6,
                      height: 96,
                      width: "100%",
                    }}
                  />
                  <div
                    style={{
                      alignItems: "baseline",
                      display: "flex",
                      justifyContent: "space-between",
                      padding: "0 4px 4px",
                    }}
                  >
                    <Text density={density} strong textRole="label" tone="primary">
                      {formatTime(hoverMs)}
                    </Text>
                    <Text density={density} textRole="cardMeta" tone="secondary">
                      {hoverBreak ? "Ad break" : current.name}
                    </Text>
                  </div>
                </div>
              ) : null}
            </>
          ) : null}
        </div>
        {pointer ? (
          <div aria-hidden style={{ height: 16, position: "relative" }}>
            {width > 0
              ? boxes
                  .filter((b) => b.segment.kind === "break")
                  .map((b) => (
                    <span
                      key={b.segment.id}
                      style={{
                        // A break too thin to centre its name under: end-align the last, start-align the first.
                        left: b.width < 64 ? (b.x > width / 2 ? b.x + b.width : b.x) : b.x + b.width / 2,
                        position: "absolute",
                        top: 0,
                        transform:
                          b.width < 64
                            ? b.x > width / 2
                              ? "translateX(-100%)"
                              : "none"
                            : "translateX(-50%)",
                        whiteSpace: "nowrap",
                        width: "max-content",
                      }}
                    >
                      <Text density={density} textRole="cardMeta" tone="muted">
                        Ad break
                      </Text>
                    </span>
                  ))
              : null}
            {width > 0 ? (
              <span style={{ left: liveX, position: "absolute", top: 0, transform: "translateX(-50%)" }}>
                <Text density={density} strong textRole="cardMeta" tone="live">
                  Live
                </Text>
              </span>
            ) : null}
          </div>
        ) : null}
      </div>
      {pointer ? edge(SPAN.endMs) : null}
    </div>
  );
};

// ---------------------------------------------------------------------------------------------

const PolishedChrome = ({
  device,
  material,
  scenario,
  tint: era,
  tintFor,
}: ChromeProps & { material: Material }) => {
  const density: Density = device === "desktop" ? "pointer" : "touch";
  const pointer = density === "pointer";
  const landscape = device === "landscape";
  const glass = material === "glass";
  const art = tintOf(current.dominantHex);
  const [tune, setTune] = useState<{ dir: "down" | "up"; n: number }>({ dir: "up", n: 0 });
  const retune = useCallback((dir: "down" | "up") => setTune((t) => ({ dir, n: t.n + 1 })), []);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "ArrowUp") retune("up");
      else if (e.key === "ArrowDown") retune("down");
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [retune]);
  if (scenario.cover) return <Cover density={density} kind={scenario.cover} />;

  const sheet = scenario.sheet;
  const controls = scenario.visible && !sheet;
  const card = scenario.ident && !scenario.visible && !sheet;
  const shown = controls || card;
  const behind = scenario.lagMs > 1000;
  const lag = scenario.lagMs;
  const endsAt = formatTime((scenario.inBreak ? b1.stopMs : current.stopMs) + lag);
  const breakLeft = Math.max(1, Math.round((b1.stopMs - scenario.playheadMs) / MINUTE));
  const runtime = `${Math.round((current.stopMs - current.startMs) / MINUTE)} min`;
  const playing = scenario.mode !== "paused";

  const meta: { key: string; node: ReactNode }[] = scenario.inBreak
    ? [
        { key: "b", node: <BreakBadge density={density} /> },
        { key: "back", node: `Back in ${breakLeft} min` },
      ]
    : landscape
      ? [
          { key: "ep", node: "S2 E4" },
          { key: "r", node: <RatingBadge density={density} rating={nowMeta.rating} /> },
          { key: "run", node: runtime },
        ]
      : !pointer
        ? [
            { key: "ep", node: current.line },
            { key: "r", node: <RatingBadge density={density} rating={nowMeta.rating} /> },
            { key: "run", node: runtime },
          ]
        : [
            { key: "y", node: nowMeta.year },
            { key: "r", node: <RatingBadge density={density} rating={nowMeta.rating} /> },
            { key: "run", node: runtime },
            { key: "g", node: nowMeta.genre },
          ];

  const titleBlock = (
    <div
      className={tune.n === 0 ? undefined : tune.dir === "up" ? "pe-tune-up" : "pe-tune-down"}
      key={tune.n}
      style={{ display: "flex", flexDirection: "column", gap: pointer ? 8 : 4, minWidth: 0 }}
    >
      <div style={{ alignItems: "center", display: "flex", justifyContent: "space-between" }}>
        <ChannelChip density={density} tint={era} />
        {!pointer && !landscape && !card && lastChannel ? (
          // Landscape has no room in this row; its Last channel sits beside the controls.
          <Round label={`Last channel, ${lastChannel.number}`} size={40}>
            <ArrowLeftRight aria-hidden color={playerChrome.text} size={18} strokeWidth={2} />
          </Round>
        ) : null}
      </div>
      <div style={{ paddingTop: pointer ? 8 : 4 }}>
        <Logo height={pointer ? 72 : landscape ? 40 : 56} />
      </div>
      {card ? (
        <Text density={density} numberOfLines={1} textRole={pointer ? "reading" : "body"} tone="secondary">
          {stationLine}
        </Text>
      ) : (
        <>
          {pointer ? (
            <Text density={density} numberOfLines={1} strong textRole="reading" tone="secondary">
              {current.line}
            </Text>
          ) : null}
          <Meta density={density} items={meta} />
          {pointer ? (
            <div style={{ maxWidth: 600 }}>
              <Text density={density} numberOfLines={1} textRole="reading" tone="secondary">
                {current.synopsis}
              </Text>
            </div>
          ) : null}
        </>
      )}
    </div>
  );

  const side = pointer ? 56 : landscape ? 48 : 48;
  const discSize = pointer ? 72 : landscape ? 56 : 64;
  const controlRow = (
    <div
      style={{
        alignItems: pointer ? "flex-start" : "center",
        display: "flex",
        gap: pointer ? 0 : landscape ? 24 : 24,
        justifyContent: "center",
      }}
    >
      {pointer ? (
        <>
          <Control hint="↓" label="Channel down">
            <Round label="Channel down" onPress={() => retune("down")} size={side}>
              <Glyph name="channelDown" />
            </Round>
          </Control>
          <Control hint="Space" label={playing ? "Pause" : "Play"}>
            <PlayButton playing={playing} size={discSize} />
          </Control>
          <Control hint="↑" label="Channel up">
            <Round label="Channel up" onPress={() => retune("up")} size={side}>
              <Glyph name="channelUp" />
            </Round>
          </Control>
          <Control hint="M" label="More">
            <Round label="More" size={side}>
              <Glyph name="more" />
            </Round>
          </Control>
        </>
      ) : (
        <>
          {landscape && lastChannel && !card ? (
            <>
              <Round label={`Last channel, ${lastChannel.number}`} size={side}>
                <ArrowLeftRight aria-hidden color={playerChrome.text} size={20} strokeWidth={2} />
              </Round>
              <span aria-hidden style={{ background: "var(--pe-line)", height: 24, width: 1 }} />
            </>
          ) : null}
          <Round label="Channel down" onPress={() => retune("down")} size={side}>
            <Glyph name="channelDown" size={24} />
          </Round>
          <PlayButton playing={playing} size={discSize} />
          <Round label="Channel up" onPress={() => retune("up")} size={side}>
            <Glyph name="channelUp" size={24} />
          </Round>
          <Round label="More" size={side}>
            <Glyph name="more" size={24} />
          </Round>
        </>
      )}
    </div>
  );

  const clockBlock = (
    <div style={{ display: "flex", flexDirection: "column" }}>
      <Text density={density} strong={!behind} textRole="label" tone="primary">
        {behind ? `${lagLabel(lag)} behind live` : formatTime(NOW)}
      </Text>
      <Time density={density}>{scenario.inBreak ? `Back at ${endsAt}` : `Ends ${endsAt}`}</Time>
    </div>
  );

  const scrubber = <Scrubber density={density} pointer={pointer} scenario={scenario} tint={art} />;
  // Phones and landscape: the live state and the end time sit under the track, one line.
  const trackFoot = pointer ? null : (
    <div style={{ alignItems: "center", display: "flex", height: 32, justifyContent: "space-between" }}>
      {behind ? (
        <GoLive density={density} size={32} />
      ) : (
        <Text density={density} strong textRole="cardMeta" tone="live">
          ● Live
        </Text>
      )}
      <Time density={density}>
        {behind ? `${lagLabel(lag)} behind · ` : ""}
        {scenario.inBreak ? "Back at" : "Ends"} {endsAt}
      </Time>
    </div>
  );

  const surfaceVars = {
    "--pe-accent": art.accent,
    "--pe-hairline": art.hairline,
    "--pe-raised": art.raised,
    "--pe-raised-hi": art.raisedHi,
    "--pe-raised-top": art.raisedTop,
  } as CSSProperties;

  const pad = pointer ? "0 48px 40px" : landscape ? "0 24px 16px" : "0 16px 24px";
  const scrimH = pointer ? 600 : landscape ? 320 : 460;

  return (
    <div
      className="pe-root"
      data-material={material}
      style={{ inset: 0, pointerEvents: "none", position: "absolute", ...surfaceVars }}
    >
      <PolishedStyles />
      <div
        className="pe-dress"
        style={{
          // E: dense and tinted, so text sits on a quiet field. F: lighter, so the panels have a
          // picture to blur.
          background: glass
            ? "linear-gradient(0deg, rgba(0, 0, 0, 0.74) 0%, rgba(0, 0, 0, 0.36) 50%, rgba(0, 0, 0, 0) 100%)"
            : `linear-gradient(0deg, ${art.scrimFoot} 0%, ${art.scrimMid} 55%, rgba(0, 0, 0, 0) 100%)`,
          bottom: 0,
          height: scrimH,
          left: 0,
          opacity: shown ? 1 : 0,
          position: "absolute",
          right: 0,
        }}
      />
      {glass ? (
        <div
          className="pe-dress"
          style={{
            background: "linear-gradient(180deg, rgba(0, 0, 0, 0.5) 0%, rgba(0, 0, 0, 0) 100%)",
            height: pointer ? 200 : 140,
            left: 0,
            opacity: shown ? 1 : 0,
            position: "absolute",
            right: 0,
            top: 0,
          }}
        />
      ) : null}
      <div
        aria-hidden={!shown || undefined}
        className="pe-chrome"
        data-shown={shown}
        style={{
          bottom: 0,
          display: "flex",
          flexDirection: "column",
          gap: pointer ? 24 : 8,
          left: 0,
          padding: pad,
          pointerEvents: shown ? "auto" : "none",
          position: "absolute",
          right: 0,
          textShadow: glass ? "0 1px 12px rgba(0, 0, 0, 0.5)" : undefined,
        }}
      >
        {pointer ? (
          <>
            <div
              style={{ alignItems: "flex-end", display: "flex", gap: 48, justifyContent: "space-between" }}
            >
              {titleBlock}
              {card ? null : <UpNext clock={formatTime(next.startMs + lag)} inBreak={scenario.inBreak} />}
            </div>
            {card ? null : (
              <>
                {scrubber}
                <div style={{ alignItems: "start", display: "grid", gridTemplateColumns: "1fr auto 1fr" }}>
                  <div style={{ alignItems: "center", display: "flex", gap: 16, height: 72 }}>
                    {behind ? <GoLive density={density} size={32} /> : null}
                    {clockBlock}
                  </div>
                  {controlRow}
                  <div
                    style={{ alignItems: "center", display: "flex", height: 72, justifyContent: "flex-end" }}
                  >
                    {lastChannel ? <LastChannelPill /> : null}
                  </div>
                </div>
              </>
            )}
          </>
        ) : landscape ? (
          <>
            <div
              style={{ alignItems: "flex-end", display: "flex", gap: 24, justifyContent: "space-between" }}
            >
              {titleBlock}
              {card ? null : controlRow}
            </div>
            {card ? null : (
              <div>
                {scrubber}
                {trackFoot}
              </div>
            )}
          </>
        ) : (
          <>
            {titleBlock}
            {card ? null : (
              <>
                <div>
                  {scrubber}
                  {trackFoot}
                </div>
                {controlRow}
              </>
            )}
          </>
        )}
      </div>
      {sheet ? (
        <Sheet
          className="pe-sheet"
          density={density}
          dimClassName="pe-dim"
          icon={sheet === "channels" ? "list" : "more"}
          panelStyle={{
            WebkitBackdropFilter: "var(--pe-blur)",
            background: "var(--pe-sheet)",
            backdropFilter: "var(--pe-blur)",
            borderLeft: "1px solid var(--pe-line)",
            boxShadow: "var(--pe-sheet-shadow)",
            pointerEvents: "auto",
          }}
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

export { PolishedChrome };
