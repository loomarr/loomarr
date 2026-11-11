// PROTOTYPE — throwaway (Direction mocks, 2026-11-09)
// Variant J: everything the player opens. The pieces are the real player's own (Subtitles, Audio,
// Picture, Playback details, Info, More, the volume level, the muted first play); nothing here is a
// feature the app does not have. What J decides is where each one goes.
//
//   Desktop and television: a key opens a MENU IN THE CONTROL CLUSTER, right over the keys. One
//   place for everything transient, next to the key that opened it, and the picture stays whole. A
//   television moves the amber focus ring through its rows; Back closes it.
//
//   iPhone held upright: a SHEET FROM THE BOTTOM, as tall as what it holds, so the picture above it
//   stays visible (you see subtitles change as you choose them). Landscape keeps the right-hand sheet.
//
//   Playback details is the exception: it is something you leave open while you watch, so it is a
//   panel in the upper left, clear of the band and the keys, and it stays when the controls hide.
//
//   Info is the on-air block's own card on the film strip, opened in full (desktop, television); a
//   phone has no strip, so it is a sheet there.
import { Text, typography } from "@loomarr/design-system";
import {
  Check,
  ChevronLeft,
  ChevronRight,
  Copy,
  Info as InfoIcon,
  List,
  type LucideIcon,
  MonitorCog,
  PictureInPicture2,
  Play,
  RotateCcw,
  SquareActivity,
  Volume2,
  VolumeX,
  X,
} from "lucide-react";
import type { CSSProperties, ReactNode } from "react";

import { nowProgramme as current, nextProgramme as next } from "./chrome-rich";
import { channel, formatTime, MINUTE, NOW } from "./data";
import { type Density, flat } from "./flat";
import { realChannel } from "./real-art";

const MONO = typography.family.data.web;
const real = realChannel(channel.number);

/** The amber ring a television's remote moves: drawn inside the row, so it never clips. */
const ring = (on: boolean): CSSProperties =>
  on ? { outline: `2px solid ${flat.amber}`, outlineOffset: -2 } : {};

// ---------------------------------------------------------------------------------------------
// The cluster menu (desktop, television).

const Menu = ({
  back = false,
  children,
  title,
}: {
  /** A second page of More: the heading is the way back. */
  back?: boolean;
  children: ReactNode;
  title?: string;
}) => (
  <div
    aria-label={title ?? "More"}
    role="menu"
    style={{
      background: flat.raised,
      border: `1px solid ${flat.hairline}`,
      borderRadius: 12,
      display: "flex",
      flexDirection: "column",
      padding: 6,
      pointerEvents: "auto",
      width: 280,
    }}
  >
    {title ? (
      <div style={{ alignItems: "center", display: "flex", gap: 4, height: 32, padding: "0 10px" }}>
        {back ? <ChevronLeft aria-hidden color={flat.textSecondary} size={16} strokeWidth={2} /> : null}
        <Text density="pointer" strong textRole="cardMeta" tone="secondary">
          {title}
        </Text>
      </div>
    ) : null}
    {children}
  </div>
);

interface Option {
  detail?: string;
  label: string;
}

/**
 * A choice: plain words, the one in use marked with a check. `readOnly` is a list the viewer cannot
 * change from here (the channel's audio language): the others are set quieter and take no hover.
 */
const Options = ({
  density,
  focus,
  on,
  options,
  readOnly = false,
}: {
  density: Density;
  /** Television: the row the remote is on. */
  focus?: number;
  on: number;
  options: readonly Option[];
  readOnly?: boolean;
}) => {
  const touch = density === "touch";
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
      {options.map((o, i) => (
        <button
          aria-checked={i === on}
          className={readOnly ? "pp-btn" : "pp-btn pj-row"}
          disabled={readOnly}
          key={o.label}
          role="menuitemradio"
          style={{
            alignItems: "center",
            borderRadius: 8,
            cursor: readOnly ? "default" : undefined,
            display: "flex",
            gap: 12,
            minHeight: touch ? 48 : 44,
            padding: touch ? "6px 12px" : "6px 10px",
            textAlign: "left",
            width: "100%",
            ...ring(i === focus),
          }}
          type="button"
        >
          <span style={{ display: "flex", flex: 1, flexDirection: "column", gap: 2, minWidth: 0 }}>
            <Text
              density={density}
              strong={i === on}
              textRole={touch ? "body" : "label"}
              tone={readOnly && i !== on ? "secondary" : "primary"}
            >
              {o.label}
            </Text>
            {o.detail ? (
              <Text density={density} textRole="cardMeta" tone="muted">
                {o.detail}
              </Text>
            ) : null}
          </span>
          {i === on ? <Check aria-hidden color={flat.text} size={touch ? 20 : 18} strokeWidth={2.5} /> : null}
        </button>
      ))}
    </div>
  );
};

const SUBTITLES: readonly Option[] = [
  { label: "Off" },
  { label: "English" },
  { detail: "For the deaf and hard of hearing", label: "English, with sounds" },
  { label: "Spanish" },
];
const AUDIO: readonly Option[] = [{ label: "Auto" }, { label: "English" }, { label: "Spanish" }];
const PICTURE: readonly Option[] = [
  { detail: "Best for your connection", label: "Auto · 1080p" },
  { label: "1080p" },
  { label: "720p" },
];

const SubtitlesBody = ({ density, focus }: { density: Density; focus?: number }) => (
  <Options density={density} focus={focus} on={1} options={SUBTITLES} />
);

const PictureBody = ({ density, focus }: { density: Density; focus?: number }) => (
  <Options density={density} focus={focus} on={0} options={PICTURE} />
);

/** A switch, drawn: white when on. Amber stays on the channel block and on focus. */
const Switch = ({ on }: { on: boolean }) => (
  <span
    aria-hidden
    style={{
      background: on ? flat.text : flat.control,
      borderRadius: 10,
      display: "inline-block",
      flexShrink: 0,
      height: 20,
      position: "relative",
      width: 36,
    }}
  >
    <span
      style={{
        background: on ? flat.canvas : flat.text,
        borderRadius: 8,
        height: 16,
        left: on ? 18 : 2,
        position: "absolute",
        top: 2,
        width: 16,
      }}
    />
  </span>
);

/**
 * Audio, as the real sheet has it: the channel's languages with the one airing marked, the line
 * that says who sets it, then the channel-change sound. The list is read-only today (one rendition).
 */
const AudioBody = ({ density, focus }: { density: Density; focus?: number }) => {
  const touch = density === "touch";
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      <Options density={density} on={1} options={AUDIO} readOnly />
      <div style={{ padding: touch ? "0 12px 8px" : "0 10px 8px" }}>
        <Text density={density} textRole="cardMeta" tone="muted">
          The channel’s language. An admin sets it on the channel’s page.
        </Text>
      </div>
      <div style={{ borderTop: `1px solid ${flat.hairline}`, paddingTop: 4 }}>
        <button
          aria-checked
          className="pp-btn pj-row"
          role="switch"
          style={{
            alignItems: "center",
            borderRadius: 8,
            display: "flex",
            gap: 12,
            minHeight: touch ? 48 : 44,
            padding: touch ? "6px 12px" : "6px 10px",
            textAlign: "left",
            width: "100%",
            ...ring(focus === 0),
          }}
          type="button"
        >
          <span style={{ flex: 1 }}>
            <Text density={density} textRole={touch ? "body" : "label"} tone="primary">
              Channel-change sound
            </Text>
          </span>
          <Switch on />
        </button>
      </div>
    </div>
  );
};

/** One row of More: an icon, the words, what it is set to, and a chevron when it opens something. */
const MoreRow = ({
  density,
  detail,
  focused = false,
  icon: Icon,
  label,
  opens = true,
}: {
  density: Density;
  detail?: string;
  focused?: boolean;
  icon: LucideIcon;
  label: string;
  /** It opens another page or panel (a chevron); otherwise it just does the thing. */
  opens?: boolean;
}) => {
  const touch = density === "touch";
  return (
    <button
      className="pp-btn pj-row"
      role="menuitem"
      style={{
        alignItems: "center",
        borderRadius: 8,
        display: "flex",
        gap: 12,
        minHeight: touch ? 48 : 44,
        padding: touch ? "6px 12px" : "6px 10px",
        textAlign: "left",
        width: "100%",
        ...ring(focused),
      }}
      type="button"
    >
      <Icon aria-hidden color={flat.textSecondary} size={touch ? 20 : 18} strokeWidth={1.75} />
      <span style={{ flex: 1, minWidth: 0 }}>
        <Text density={density} numberOfLines={1} textRole={touch ? "body" : "label"} tone="primary">
          {label}
        </Text>
      </span>
      {detail ? (
        <Text density={density} textRole="cardMeta" tone="muted">
          {detail}
        </Text>
      ) : null}
      {opens ? <ChevronRight aria-hidden color={flat.textMuted} size={16} strokeWidth={2} /> : null}
    </button>
  );
};

/**
 * More: only what has no key of its own. Subtitles and Audio have keys, so "Sound and picture" is
 * gone from here; what is left of it is Picture. Picture in picture is listed where the platform has
 * it and no key shows it (desktop): an iPhone has the key, a television has neither.
 */
const MoreRows = ({
  behind,
  density,
  focus,
  pip,
}: {
  behind: boolean;
  density: Density;
  /** Television: the row the remote is on. */
  focus?: number;
  pip: boolean;
}) => (
  <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
    {behind ? (
      <>
        <MoreRow density={density} detail={current.name} icon={RotateCcw} label="Start over" opens={false} />
        <MoreRow density={density} detail={formatTime(NOW)} icon={Play} label="Go live" opens={false} />
      </>
    ) : null}
    <MoreRow density={density} focused={focus === 0} icon={List} label="Channels" />
    <MoreRow density={density} focused={focus === 1} icon={InfoIcon} label="Info" />
    <MoreRow
      density={density}
      detail="Auto · 1080p"
      focused={focus === 2}
      icon={MonitorCog}
      label="Picture"
    />
    <MoreRow density={density} focused={focus === 3} icon={SquareActivity} label="Playback details" />
    {pip ? (
      <MoreRow density={density} icon={PictureInPicture2} label="Picture in picture" opens={false} />
    ) : null}
  </div>
);

/**
 * The volume level (desktop): it opens in the same place as the menus, from the Volume key, and
 * shows for a moment whenever the level changes (the scroll wheel, the arrow keys, M to mute).
 */
const VolumeBar = ({ level, muted = false }: { level: number; muted?: boolean }) => {
  const Glyph = muted ? VolumeX : Volume2;
  const shown = muted ? 0 : level;
  return (
    <div
      aria-label="Volume"
      aria-valuemax={100}
      aria-valuemin={0}
      aria-valuenow={Math.round(shown * 100)}
      role="slider"
      style={{
        alignItems: "center",
        background: flat.raised,
        border: `1px solid ${flat.hairline}`,
        borderRadius: 12,
        display: "flex",
        gap: 12,
        height: 48,
        padding: "0 16px",
        pointerEvents: "auto",
        width: 280,
      }}
      tabIndex={0}
    >
      <Glyph aria-hidden color={flat.text} size={20} strokeWidth={1.75} />
      <span style={{ background: flat.control, borderRadius: 2, flex: 1, height: 4, position: "relative" }}>
        <span
          style={{
            background: flat.text,
            borderRadius: 2,
            bottom: 0,
            left: 0,
            position: "absolute",
            top: 0,
            width: `${shown * 100}%`,
          }}
        />
        <span
          style={{
            background: flat.text,
            borderRadius: 7,
            height: 14,
            left: `calc(${shown * 100}% - 7px)`,
            position: "absolute",
            top: -5,
            width: 14,
          }}
        />
      </span>
      <span
        style={{
          color: flat.textSecondary,
          fontFamily: MONO,
          fontSize: 13,
          fontWeight: 600,
          minWidth: 24,
          textAlign: "right",
        }}
      >
        {muted ? "Off" : Math.round(shown * 100)}
      </span>
    </div>
  );
};

// ---------------------------------------------------------------------------------------------
// Playback details: the one place technical terms are allowed. Only what the player really knows.

const DETAILS_WEB: readonly (readonly [string, string])[] = [
  ["Resolution", "1920×1080 · 29.97 fps"],
  // The codec by its name, not the raw string the stream carries (avc1.640029, mp4a.40.2).
  ["Video", "H.264 High 4.1"],
  ["Audio", "AAC stereo"],
  ["HDR", "None (SDR)"],
  ["Bitrate", "4.6 Mbps"],
  ["Buffer ahead", "6.2 s"],
  ["Dropped frames", "3 of 12,418"],
  ["Behind live", "2.3 s"],
  ["Server", "loomarr.lan"],
  ["Session", "4f2a9c1d"],
];

const DetailsBody = ({
  copy = true,
  density,
  wide = false,
}: {
  /** A television has no clipboard, so no Copy key. */
  copy?: boolean;
  density: Density;
  /** Two columns of rows, where there is more width than height (a television). */
  wide?: boolean;
}) => {
  const size = density === "touch" ? 14 : 13;
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <dl
        style={{
          display: "grid",
          gap: wide ? "8px 16px" : "8px 24px",
          gridTemplateColumns: wide ? "auto 1fr 24px auto 1fr" : "auto 1fr",
          margin: 0,
        }}
      >
        {DETAILS_WEB.map(([label, value], i) => (
          <div key={label} style={{ display: "contents" }}>
            <dt style={{ color: flat.textMuted, fontSize: size, lineHeight: 1.4, margin: 0 }}>{label}</dt>
            <dd
              style={{
                color: flat.text,
                fontFamily: MONO,
                fontSize: size,
                fontWeight: 500,
                lineHeight: 1.4,
                margin: 0,
                textAlign: "right",
              }}
            >
              {value}
            </dd>
            {/* The gutter between the two columns. */}
            {wide && i % 2 === 0 ? <span aria-hidden /> : null}
          </div>
        ))}
      </dl>
      {copy ? (
        <button
          className="pp-btn pj-row"
          style={{
            alignItems: "center",
            alignSelf: "flex-start",
            background: flat.elevated,
            border: `1px solid ${flat.hairline}`,
            borderRadius: 8,
            display: "inline-flex",
            gap: 8,
            height: density === "touch" ? 44 : 36,
            padding: "0 12px",
          }}
          type="button"
        >
          <Copy aria-hidden color={flat.text} size={16} strokeWidth={1.75} />
          <Text density={density} strong textRole="cardMeta" tone="primary">
            Copy details
          </Text>
        </button>
      ) : null}
    </div>
  );
};

const DetailsPanel = ({
  closable,
  left,
  top,
  wide,
}: {
  /** Desktop has a close key; a television's Back closes it. */
  closable: boolean;
  left: number;
  top: number;
  wide: boolean;
}) => (
  <section
    aria-label="Playback details"
    style={{
      background: flat.raised,
      border: `1px solid ${flat.hairline}`,
      borderRadius: 12,
      left,
      padding: "10px 16px 16px",
      pointerEvents: "auto",
      position: "absolute",
      top,
      width: wide ? 640 : 344,
      zIndex: 35,
    }}
  >
    <div style={{ alignItems: "center", display: "flex", height: 36, marginBottom: 6 }}>
      <span style={{ flex: 1 }}>
        <Text density="pointer" strong textRole="label" tone="primary">
          Playback details
        </Text>
      </span>
      {closable ? (
        <button
          aria-label="Close Playback details"
          className="pp-btn pj-row"
          style={{
            alignItems: "center",
            borderRadius: 8,
            display: "inline-flex",
            height: 32,
            justifyContent: "center",
            marginRight: -8,
            width: 32,
          }}
          type="button"
        >
          <X aria-hidden color={flat.textSecondary} size={18} strokeWidth={1.75} />
        </button>
      ) : null}
    </div>
    <DetailsBody copy={closable} density="pointer" wide={wide} />
  </section>
);

// ---------------------------------------------------------------------------------------------
// Info, on a phone: the programme on now, in full, and what follows it.

const left = (ms: number) => `${Math.max(1, Math.round(ms / MINUTE))} min left`;

const InfoBody = ({ bare, density }: { bare: boolean; density: Density }) => {
  const logo = !bare && real && current.name === real.title ? real.logoOf : undefined;
  const still = !bare && real && current.name === real.title ? real.fanartSrc : current.stillUri;
  const logoHeight = logo ? Math.min(56, Math.sqrt(6400 / logo.ratio)) : 0;
  const progress = (NOW - current.startMs) / (current.stopMs - current.startMs);
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <div
        style={{
          alignItems: "flex-end",
          aspectRatio: "16 / 9",
          background: logo
            ? `linear-gradient(rgba(11, 12, 14, 0.42), rgba(11, 12, 14, 0.42)), ${flat.elevated} url("${still}") center/cover`
            : `${flat.elevated} url("${still}") center/cover`,
          borderRadius: 8,
          display: "flex",
          maxHeight: 168,
          padding: 12,
        }}
      >
        {logo ? (
          <img
            alt=""
            height={logoHeight}
            src={logo.src}
            style={{ display: "block", height: logoHeight, width: logoHeight * logo.ratio }}
            width={logoHeight * logo.ratio}
          />
        ) : null}
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
        <Text density={density} strong textRole="title" tone="primary">
          {current.name}
        </Text>
        <Text density={density} textRole="label" tone="secondary">
          {[current.line, ...current.chips.filter((c) => !current.line.includes(c))].join(" · ")}
        </Text>
      </div>
      <div style={{ alignItems: "center", display: "flex", gap: 12 }}>
        <span style={{ background: flat.control, borderRadius: 2, flex: 1, height: 3, position: "relative" }}>
          <span
            style={{
              background: flat.text,
              borderRadius: 2,
              bottom: 0,
              left: 0,
              position: "absolute",
              top: 0,
              width: `${progress * 100}%`,
            }}
          />
        </span>
        <Text density={density} textRole="cardMeta" tone="secondary">
          {left(current.stopMs - NOW)}
        </Text>
      </div>
      <Text density={density} textRole="body" tone="secondary">
        {current.synopsis}
      </Text>
      <div
        style={{
          alignItems: "baseline",
          borderTop: `1px solid ${flat.hairline}`,
          display: "flex",
          gap: 12,
          paddingTop: 12,
        }}
      >
        <Text density={density} textRole="cardMeta" tone="muted">
          Next
        </Text>
        <span style={{ color: flat.textSecondary, fontFamily: MONO, fontSize: 13, fontWeight: 600 }}>
          {formatTime(next.startMs)}
        </span>
        <span style={{ flex: 1, minWidth: 0 }}>
          <Text density={density} numberOfLines={1} strong textRole="label" tone="primary">
            {next.name}
          </Text>
        </span>
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------------------------
// The bottom sheet (iPhone, upright): as tall as what it holds, so the picture stays in view.

const BottomSheet = ({
  children,
  maxHeight,
  safeBottom,
  title,
}: {
  children: ReactNode;
  /** It never covers the toolbar at the top of the screen. */
  maxHeight: number;
  safeBottom: number;
  title: string;
}) => (
  <div
    aria-label={title}
    role="dialog"
    style={{
      background: flat.raised,
      borderRadius: "16px 16px 0 0",
      borderTop: `1px solid ${flat.hairline}`,
      bottom: 0,
      display: "flex",
      flexDirection: "column",
      left: 0,
      maxHeight,
      paddingBottom: safeBottom,
      pointerEvents: "auto",
      position: "absolute",
      right: 0,
      zIndex: 40,
    }}
  >
    <span
      aria-hidden
      style={{
        alignSelf: "center",
        background: flat.control,
        borderRadius: 2,
        flexShrink: 0,
        height: 4,
        marginTop: 6,
        width: 36,
      }}
    />
    <div style={{ alignItems: "center", display: "flex", flexShrink: 0, padding: "0 8px 0 20px" }}>
      <span style={{ flex: 1 }}>
        <Text as="h2" density="touch" strong textRole="label" tone="primary">
          {title}
        </Text>
      </span>
      <button
        aria-label={`Close ${title}`}
        className="pp-btn pj-row"
        style={{
          alignItems: "center",
          borderRadius: 10,
          display: "inline-flex",
          height: 44,
          justifyContent: "center",
          width: 44,
        }}
        type="button"
      >
        <X aria-hidden color={flat.textSecondary} size={20} strokeWidth={1.75} />
      </button>
    </div>
    <div style={{ overflow: "auto", padding: "0 8px 12px" }}>{children}</div>
  </div>
);

// ---------------------------------------------------------------------------------------------
// Two things that sit on the picture by themselves.

/**
 * The first play of a session starts muted (a browser's rule), so the player says so. It is the only
 * chrome that stays up while the controls are hidden, and it goes when the sound is on.
 */
const UnmutePill = ({ touch }: { touch: boolean }) => (
  <button
    className="pp-btn pj-row"
    style={{
      alignItems: "center",
      background: flat.elevated,
      border: `1px solid ${flat.hairline}`,
      borderRadius: 10,
      display: "inline-flex",
      gap: 8,
      height: touch ? 44 : 40,
      padding: "0 14px",
      pointerEvents: "auto",
    }}
    type="button"
  >
    <VolumeX aria-hidden color={flat.text} size={18} strokeWidth={1.75} />
    <Text density={touch ? "touch" : "pointer"} strong textRole="label" tone="primary">
      {touch ? "Tap to unmute" : "Click to unmute"}
    </Text>
  </button>
);

/**
 * A subtitle, where the player puts it: centred on the picture, and lifted clear of the band while
 * the controls are up, so the two never sit on each other.
 */
const Caption = ({
  bottom,
  lines,
  maxWidth,
  right = 0,
  size,
}: {
  bottom: number;
  lines: readonly string[];
  maxWidth: number;
  /** Room kept clear on the right (a sheet), so the line is centred in what shows of the picture. */
  right?: number;
  size: number;
}) => (
  <div
    aria-hidden
    style={{
      alignItems: "center",
      bottom,
      display: "flex",
      flexDirection: "column",
      gap: 2,
      left: 0,
      pointerEvents: "none",
      position: "absolute",
      right,
    }}
  >
    {lines.map((line) => (
      <span
        key={line}
        style={{
          background: "rgba(0, 0, 0, 0.78)",
          borderRadius: 2,
          color: "#fff",
          fontSize: size,
          fontWeight: 500,
          lineHeight: 1.3,
          maxWidth,
          padding: "2px 10px",
          textAlign: "center",
        }}
      >
        {line}
      </span>
    ))}
  </div>
);

/** Hover for every row and key in this file; the focus ring comes from the shared player stylesheet. */
const PanelStyles = () => (
  <style>{`
    .pj-row { transition: background-color 120ms ease; }
    .pj-row:hover { background-color: ${flat.focus}; }
  `}</style>
);

export {
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
};
