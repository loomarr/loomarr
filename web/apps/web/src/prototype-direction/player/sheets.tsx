// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// The transient layer, flat: one right-hand sheet over a dimmed picture (solid raised surface, a
// hairline on its left edge, no blur, no shadow), and the full-picture covers (error, off-air).
import { Action, Icon, type IconName, icons, playerChrome, Text } from "@loomarr/design-system";
import { ArrowLeftRight } from "lucide-react";
import type { CSSProperties, ReactNode } from "react";
import { type ChannelRow, channel, channels, formatTime, segments, WINDOW } from "./data";
import { type Density, flat, Tool } from "./flat";
import { Playbar } from "./playbar";

const Sheet = ({
  children,
  className,
  closable = true,
  density,
  dimClassName,
  icon,
  panelStyle,
  title,
}: {
  children: ReactNode;
  /** A television has no close key on the sheet: the remote's Back closes it (variant J). */
  closable?: boolean;
  /** Motion hooks for variants E and F; the earlier variants draw the sheet without them. */
  className?: string;
  density: Density;
  dimClassName?: string;
  icon?: IconName;
  /** Overrides on the panel: E tints its surface, F makes it a translucent material. */
  panelStyle?: CSSProperties;
  title: string;
}) => (
  <>
    <div
      className={dimClassName}
      style={{ background: flat.dim, inset: 0, pointerEvents: "none", position: "absolute", zIndex: 39 }}
    />
    <div
      aria-label={title}
      className={className}
      role="dialog"
      style={{
        background: flat.raised,
        borderLeft: `1px solid ${flat.hairline}`,
        bottom: 0,
        display: "flex",
        flexDirection: "column",
        maxWidth: "100%",
        position: "absolute",
        right: 0,
        top: 0,
        width: density === "touch" ? 384 : 416,
        zIndex: 40,
        ...panelStyle,
      }}
    >
      <div
        style={{
          alignItems: "center",
          borderBottom: `1px solid ${flat.hairline}`,
          display: "flex",
          gap: 10,
          padding: "6px 8px 6px 16px",
        }}
      >
        {icon ? <Icon color={flat.textSecondary} decorative glyph={icons[icon]} size="compact" /> : null}
        <div style={{ flex: 1 }}>
          <Text as="h2" density={density} textRole="label" tone="primary">
            {title}
          </Text>
        </div>
        <Text density={density} numberOfLines={1} textRole="cardMeta" tone="muted">
          {`CH ${channel.number} · ${channel.name}`}
        </Text>
        {closable ? (
          <Tool density={density} icon="close" label={`Close ${title}`} />
        ) : (
          <span style={{ height: density === "touch" ? 44 : 36 }} />
        )}
      </div>
      <div style={{ flex: 1, overflow: "auto", padding: 16 }}>{children}</div>
    </div>
  </>
);

// A channel number in its block: the tint only ever colours this block.
const NumberBlock = ({ density, tint, number }: { density: Density; number: string; tint?: string }) => (
  <span
    style={{
      alignItems: "center",
      background: tint ?? flat.elevated,
      border: tint ? undefined : `1px solid ${flat.hairline}`,
      borderRadius: 6,
      display: "inline-flex",
      flexShrink: 0,
      height: 32,
      justifyContent: "center",
      width: 44,
    }}
  >
    <Text density={density} textRole="cardChannelNumber" tone={tint ? "inverse" : "primary"}>
      {number}
    </Text>
  </span>
);

const ChannelList = ({
  density,
  tintFor,
}: {
  density: Density;
  tintFor: (era: string) => string | undefined;
}) => (
  <ul style={{ display: "flex", flexDirection: "column", gap: 2, listStyle: "none", margin: 0, padding: 0 }}>
    {channels.map((row: ChannelRow) => {
      const current = row.number === channel.number;
      return (
        <li key={row.number}>
          <button
            aria-current={current || undefined}
            className="pp-btn pp-row"
            style={{
              alignItems: "center",
              background: current ? flat.focus : undefined,
              borderRadius: 10,
              display: "flex",
              gap: 12,
              minHeight: density === "touch" ? 56 : 48,
              padding: "6px 10px",
              textAlign: "left",
              width: "100%",
            }}
            type="button"
          >
            <NumberBlock density={density} number={row.number} tint={tintFor(row.era)} />
            <span style={{ display: "flex", flex: 1, flexDirection: "column", minWidth: 0 }}>
              <Text density={density} numberOfLines={1} strong textRole="body" tone="primary">
                {row.name}
              </Text>
              <Text density={density} numberOfLines={1} textRole="cardMeta" tone="secondary">
                {current ? `Now watching · ${row.nowPlaying}` : row.nowPlaying}
              </Text>
            </span>
            <Text density={density} textRole="cardMeta" tone="muted">
              {row.era}
            </Text>
          </button>
        </li>
      );
    })}
  </ul>
);

const Row = ({
  density,
  detail,
  glyph,
  label,
  onClick,
  swap = false,
}: {
  density: Density;
  detail?: string;
  glyph?: IconName;
  label: string;
  onClick?: () => void;
  swap?: boolean;
}) => (
  <button
    className="pp-btn pp-row"
    onClick={onClick}
    style={{
      alignItems: "center",
      borderBottom: `1px solid ${flat.hairline}`,
      display: "flex",
      gap: 14,
      minHeight: 52,
      padding: "0 8px",
      textAlign: "left",
      width: "100%",
    }}
    type="button"
  >
    {swap ? (
      <ArrowLeftRight aria-hidden color={flat.textSecondary} size={20} strokeWidth={1.75} />
    ) : glyph ? (
      <Icon color={flat.textSecondary} decorative glyph={icons[glyph]} size="default" />
    ) : null}
    <span style={{ flex: 1 }}>
      <Text density={density} textRole="body" tone="primary">
        {label}
      </Text>
    </span>
    {detail ? (
      <Text density={density} textRole="cardMeta" tone="muted">
        {detail}
      </Text>
    ) : null}
    <Icon color={flat.textMuted} decorative glyph={icons.disclosure} size="compact" />
  </button>
);

const SettingsBody = ({ density }: { density: Density }) => (
  <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
    {[
      { name: "Sound", options: ["Auto", "English", "Spanish"], on: 1 },
      { name: "Picture quality", options: ["Auto · 4K", "1080p", "4K"], on: 0 },
    ].map((group) => (
      <div key={group.name}>
        <Text density={density} shout textRole="cardMeta" tone="muted">
          {group.name}
        </Text>
        <div style={{ display: "flex", flexDirection: "column", gap: 2, marginTop: 8 }}>
          {group.options.map((option, i) => (
            <div
              key={option}
              style={{
                alignItems: "center",
                background: i === group.on ? flat.focus : undefined,
                borderRadius: 10,
                display: "flex",
                height: 44,
                justifyContent: "space-between",
                padding: "0 12px",
              }}
            >
              <Text density={density} textRole="body" tone="primary">
                {option}
              </Text>
              {i === group.on ? (
                <Icon color={flat.amber} decorative glyph={icons.success} size="compact" />
              ) : null}
            </div>
          ))}
        </div>
      </div>
    ))}
  </div>
);

// Variant C's More sheet: everything the four-control screen leaves out, the strip included.
const MoreBody = ({
  density,
  behind,
  headColor,
  only = false,
  plain = false,
  rows,
  schedule = true,
}: {
  /** Variant J: its own rows in place of the ones below (panels-j.tsx). */
  rows?: ReactNode;
  behind: boolean;
  density: Density;
  /** The schedule strip's playhead colour (variant J: white). */
  headColor?: string;
  /**
   * Variant J: More holds only what is not already on screen. Last channel, Fullscreen and Leave
   * have keys of their own there, so listing them again here is the same control twice.
   */
  only?: boolean;
  /** Sentence-case section labels (variants E and F). */
  plain?: boolean;
  /** Variant J on desktop and television: the film strip is in the band, so not here as well. */
  schedule?: boolean;
}) => (
  <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
    {schedule ? (
      // J's rows carry their own side padding, so the strip is inset to line up with their words.
      <div style={rows ? { padding: "0 12px" } : undefined}>
        <Text density={density} shout={!plain} strong={plain} textRole="cardMeta" tone="muted">
          Schedule
        </Text>
        <div style={{ marginTop: 14 }}>
          <Playbar
            density="touch"
            headColor={headColor}
            plain={plain}
            playheadMs={behind ? WINDOW.nowMs - 134_000 : WINDOW.nowMs}
            pointer={false}
            segments={segments}
          />
        </div>
      </div>
    ) : null}
    {rows}
    <div style={rows ? { display: "none" } : undefined}>
      {behind ? <Row density={density} glyph="startOver" label="Start over" detail="Opening Night" /> : null}
      {behind ? (
        <Row density={density} glyph="play" label="Go live" detail={formatTime(WINDOW.nowMs)} />
      ) : null}
      {only ? null : <Row density={density} label="Last channel" detail="CH 3" swap />}
      <Row density={density} glyph="list" label="Channels" />
      <Row density={density} glyph="info" label="Info" />
      <Row density={density} glyph="settings" label="Sound and picture" />
      {density === "pointer" && !only ? (
        <Row density={density} glyph="fullscreen" label="Fullscreen" />
      ) : null}
      {only ? null : <Row density={density} glyph="back" label="Leave Watch" />}
    </div>
  </div>
);

// ---------------------------------------------------------------------------------------------
// Covers: nothing to play, so the message takes the picture's place. Solid, no blur.

const copy = {
  "error-not-started": {
    message:
      "Loomarr is running, but it couldn't start the video for this channel. Trying again usually fixes it.",
    title: "This channel couldn't start",
  },
  "error-server": {
    message:
      "The picture stopped because this device lost touch with Loomarr. Check that the server is running and that you're on the same network, then try again.",
    title: "Loomarr's server isn't answering",
  },
  // The real player's words, and all of them: it does not know why, so it does not guess.
  "error-stopped": {
    message: "",
    title: "The stream stopped.",
  },
  "off-air": {
    message: "This channel is paused, so there's nothing to watch.",
    title: `${channel.name} is off air`,
  },
} as const;

const Cover = ({
  density,
  kind,
  primary = "amber",
}: {
  density: Density;
  kind: keyof typeof copy;
  /**
   * Variant J: Retry is white, like the player's play key. Inside the player amber is for the
   * channel block and focus, so the app's amber primary does not come in with the cover.
   */
  primary?: "amber" | "white";
}) => {
  const { message, title } = copy[kind];
  const off = kind === "off-air";
  return (
    <div
      role={off ? "status" : "alert"}
      style={{
        alignItems: "center",
        background: off ? flat.canvas : flat.overlay,
        display: "flex",
        flexDirection: "column",
        gap: 8,
        inset: 0,
        justifyContent: "center",
        padding: "0 24px",
        position: "absolute",
        textAlign: "center",
        zIndex: 35,
      }}
    >
      <Icon
        color={off ? flat.textMuted : flat.live}
        decorative
        glyph={off ? icons.channels : icons.warning}
        size="tv"
      />
      <Text
        density={density}
        strong
        textAlign="center"
        textRole={density === "touch" ? "headline" : "title"}
        tone="primary"
      >
        {title}
      </Text>
      {message ? (
        <div style={{ maxWidth: 440 }}>
          <Text density={density} textAlign="center" textRole="body" tone="secondary">
            {message}
          </Text>
        </div>
      ) : null}
      <Text density={density} textRole="cardMeta" tone="muted">
        {`CH ${channel.number} · ${channel.name}`}
      </Text>
      <div style={{ display: "flex", gap: 8, marginTop: 12 }}>
        {off ? null : primary === "white" ? (
          <button
            className="pp-btn"
            style={{
              alignItems: "center",
              background: playerChrome.disc,
              borderRadius: 8,
              color: playerChrome.discInk,
              display: "inline-flex",
              fontWeight: 600,
              minHeight: density === "touch" ? 44 : 36,
              padding: "0 16px",
            }}
            type="button"
          >
            Retry
          </button>
        ) : (
          <Action density={density} onPress={() => undefined}>
            Retry
          </Action>
        )}
        <Action density={density} onPress={() => undefined} variant="outline">
          Leave
        </Action>
      </div>
    </div>
  );
};

export type { ChannelRow };
export { ChannelList, Cover, MoreBody, NumberBlock, Row, SettingsBody, Sheet };
