// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// The flat vocabulary for player chrome (maintainer decision 2026-11-08): solid surfaces from
// `semanticThemes.dark.surface`, hairlines from `border.decorative`, one bottom-up scrim as the only
// soft edge. No gradients, inset highlights, drop shadows, backdrop blur or bevel anywhere else.
import { Icon, type IconName, icons, playerChrome, semanticThemes, Text } from "@loomarr/design-system";
import { ArrowLeftRight } from "lucide-react";
import type { ComponentProps, CSSProperties, ReactNode } from "react";

const dark = semanticThemes.dark;

const flat = {
  amber: playerChrome.signal,
  canvas: dark.surface.canvas,
  control: dark.border.control,
  dim: playerChrome.dim,
  elevated: dark.surface.elevated,
  focus: dark.surface.focus,
  hairline: dark.border.decorative,
  info: dark.state.info,
  infoSurface: dark.stateSurface.info,
  ink: dark.content.inverse,
  live: dark.state.live,
  liveSurface: dark.stateSurface.live,
  overlay: dark.surface.overlay,
  raised: dark.surface.raised,
  text: playerChrome.text,
  textMuted: playerChrome.textMuted,
  textSecondary: playerChrome.textSecondary,
} as const;

// The single scrim: bottom-up, black to clear. The only soft edge in the chrome.
const scrimStyle = (heightPx: number, held = false): CSSProperties => ({
  // `held` keeps the scrim dense higher up, for text that sits directly on it with no panel behind.
  background: held
    ? `linear-gradient(0deg, ${playerChrome.scrimFoot} 0%, rgba(0, 0, 0, 0.7) 55%, rgba(0, 0, 0, 0) 100%)`
    : `linear-gradient(0deg, ${playerChrome.scrimFoot} 0%, rgba(0, 0, 0, 0) 100%)`,
  bottom: 0,
  height: heightPx,
  left: 0,
  pointerEvents: "none",
  position: "absolute",
  right: 0,
});

type Density = "pointer" | "touch";
type TextRole = ComponentProps<typeof Text>["textRole"];

// Focus is amber and hover is a tone step; both live in one stylesheet because inline style cannot
// say :hover or :focus-visible.
const PlayerStyles = () => (
  <style>{`
    .pp-btn { appearance: none; background: transparent; border: 0; color: inherit; cursor: pointer; font: inherit; padding: 0; }
    .pp-btn:focus-visible { outline: 2px solid ${flat.amber}; outline-offset: 2px; }
    .pp-tool:hover { background: ${flat.focus}; }
    .pp-row:hover { background: ${flat.elevated}; }
    .pp-tool .pp-tip { display: none; }
    .pp-tool:hover .pp-tip, .pp-tool:focus-visible .pp-tip { display: flex; }
    .pp-ghost:hover { background: ${flat.focus}; }
  `}</style>
);

const Tip = ({
  align = "center",
  density,
  keyHint,
  label,
}: {
  align?: "center" | "end" | "start";
  density: Density;
  keyHint?: string;
  label: string;
}) => (
  <span
    className="pp-tip"
    style={{
      alignItems: "center",
      background: flat.raised,
      border: `1px solid ${flat.hairline}`,
      borderRadius: 6,
      bottom: "100%",
      gap: 6,
      marginBottom: 10,
      padding: "4px 8px",
      pointerEvents: "none",
      position: "absolute",
      whiteSpace: "nowrap",
      zIndex: 60,
      ...(align === "start"
        ? { left: 0 }
        : align === "end"
          ? { right: 0 }
          : { left: "50%", transform: "translateX(-50%)" }),
    }}
  >
    <Text density={density} textRole="cardMeta" tone="primary">
      {label}
    </Text>
    {keyHint ? (
      <span
        style={{
          background: flat.elevated,
          border: `1px solid ${flat.hairline}`,
          borderRadius: 3,
          padding: "0 4px",
        }}
      >
        <Text density={density} textRole="captionMono" tone="secondary">
          {keyHint}
        </Text>
      </span>
    ) : null}
  </span>
);

interface ToolProps {
  active?: boolean;
  density: Density;
  disabled?: boolean;
  icon?: IconName;
  keyHint?: string;
  label: string;
  size?: number;
  tipAlign?: "center" | "end" | "start";
  /** A keycap drawn beside the button, always visible (variant B's arrow-key hints). */
  keycap?: string;
  swap?: boolean;
  /** Glyph at full text colour: a control that is one of four on the screen. */
  bright?: boolean;
  /** A larger glyph and a hairline ring: a control that carries the screen (variant D). */
  ringed?: boolean;
}

const Tool = ({
  active = false,
  bright = false,
  density,
  disabled = false,
  icon,
  keyHint,
  keycap,
  label,
  ringed = false,
  size,
  swap,
  tipAlign,
}: ToolProps) => {
  const box = size ?? (density === "touch" ? 44 : 36);
  const glyph = active ? flat.amber : bright ? flat.text : flat.textSecondary;
  return (
    <span style={{ alignItems: "center", display: "inline-flex", gap: 4 }}>
      <button
        aria-label={label}
        className="pp-btn pp-tool"
        disabled={disabled}
        style={{
          alignItems: "center",
          background: active ? flat.focus : ringed ? flat.elevated : undefined,
          border: ringed ? `1px solid ${flat.hairline}` : undefined,
          borderRadius: 999,
          display: "inline-flex",
          flexShrink: 0,
          height: box,
          justifyContent: "center",
          opacity: disabled ? 0.3 : 1,
          position: "relative",
          width: box,
        }}
        type="button"
      >
        {swap ? (
          <ArrowLeftRight aria-hidden color={glyph} size={20} strokeWidth={1.75} />
        ) : icon ? (
          <Icon color={glyph} decorative glyph={icons[icon]} size={ringed ? "control" : "player"} />
        ) : null}
        {active ? (
          <span
            style={{
              background: flat.amber,
              borderRadius: 2,
              bottom: 4,
              height: 4,
              position: "absolute",
              width: 4,
            }}
          />
        ) : null}
        {density === "pointer" ? (
          <Tip align={tipAlign} density={density} keyHint={keyHint} label={label} />
        ) : null}
      </button>
      {keycap ? (
        <span style={{ border: `1px solid ${flat.hairline}`, borderRadius: 4, padding: "0 5px" }}>
          <Text density={density} textRole="captionMono" tone="muted">
            {keycap}
          </Text>
        </span>
      ) : null}
    </span>
  );
};

const Capsule = ({ children, end = false }: { children: ReactNode; end?: boolean }) => (
  <div
    style={{
      alignItems: "center",
      background: flat.elevated,
      border: `1px solid ${flat.hairline}`,
      borderRadius: 999,
      display: "flex",
      flexShrink: 0,
      gap: 2,
      padding: 3,
      paddingRight: end ? 12 : 3,
    }}
  >
    {children}
  </div>
);

// Play and pause: the one primary control, a flat white disc.
const PlayDisc = ({
  large = false,
  playing,
  size = 44,
}: {
  large?: boolean;
  playing: boolean;
  size?: number;
}) => (
  <button
    aria-label={playing ? "Pause" : "Play"}
    className="pp-btn"
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
    <Icon
      color={playerChrome.discInk}
      decorative
      glyph={playing ? icons.pause : icons.play}
      size={large ? "touch" : "default"}
    />
  </button>
);

const LiveMark = ({ density }: { density: Density }) => (
  <span style={{ alignItems: "center", display: "inline-flex", gap: 6 }}>
    <span style={{ background: flat.live, borderRadius: 4, height: 7, width: 7 }} />
    <Text density={density} shout strong textRole="guideIdent" tone="live">
      Live
    </Text>
  </span>
);

const GoLive = ({ density }: { density: Density }) => (
  <button
    className="pp-btn pp-ghost"
    style={{
      alignItems: "center",
      border: `1px solid ${flat.live}`,
      borderRadius: 999,
      display: "inline-flex",
      gap: 6,
      height: density === "touch" ? 40 : 30,
      padding: "0 12px",
    }}
    type="button"
  >
    <span style={{ background: flat.live, borderRadius: 4, height: 7, width: 7 }} />
    <Text density={density} strong textRole="label" tone="live">
      Go live
    </Text>
  </button>
);

const StartOver = ({ density }: { density: Density }) => (
  <button
    className="pp-btn pp-ghost"
    style={{
      alignItems: "center",
      borderRadius: 999,
      display: "inline-flex",
      gap: 6,
      height: density === "touch" ? 40 : 32,
      padding: "0 12px",
    }}
    type="button"
  >
    <Icon color={flat.text} decorative glyph={icons.startOver} size="compact" />
    <Text density={density} textRole="label" tone="primary">
      Start over
    </Text>
  </button>
);

// Last channel (phones): a swap glyph and its name, drawn only when there is a channel to return to.
const LastChannel = ({ density, lastNumber }: { density: Density; lastNumber: string }) => (
  <button
    aria-label={`Last channel, ${lastNumber}`}
    className="pp-btn pp-ghost"
    style={{
      alignItems: "center",
      background: flat.elevated,
      border: `1px solid ${flat.hairline}`,
      borderRadius: 999,
      display: "inline-flex",
      gap: 8,
      height: 44,
      padding: "0 14px",
    }}
    type="button"
  >
    <ArrowLeftRight aria-hidden color={flat.textSecondary} size={20} strokeWidth={1.75} />
    <Text density={density} textRole="label" tone="primary">
      Last channel
    </Text>
    <Text density={density} textRole="cardChannelNumber" tone="secondary">
      {lastNumber}
    </Text>
  </button>
);

export type { Density, TextRole };
export { Capsule, flat, GoLive, LastChannel, LiveMark, PlayDisc, PlayerStyles, StartOver, scrimStyle, Tool };
