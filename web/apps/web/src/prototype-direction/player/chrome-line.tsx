// PROTOTYPE — throwaway (Direction mocks, 2026-11-09)
// Variant I: H's channel identity with the preview timeline back. One lower third instead of three
// objects: a slim number tab, the title, and the time-true preview timeline (programme stills, breaks
// spot by spot, a card on hover) where H drew one progress line and a NEXT row. Keys are icon-only
// and sit on the same row as the title; amber is on the number tab and on focus, nowhere else.
import { Text } from "@loomarr/design-system";
import {
  AudioLines,
  Captions,
  ChevronDown,
  ChevronUp,
  Ellipsis,
  type LucideIcon,
  Maximize,
  Pause,
  Play,
  Volume2,
  X,
} from "lucide-react";

import type { ChromeProps } from "./chrome-classic";
import { channel, formatTime, NOW, type Programme, type Segment, segments } from "./data";
import { type Density, flat, scrimStyle } from "./flat";
import { layout, Playbar, useWidth } from "./playbar";
import { realChannel } from "./real-art";
import { ChannelList, Cover, SettingsBody, Sheet } from "./sheets";

// Real library art when it is on this machine (gitignored): the title's logo for the heading, and its
// fanart as the still on its own block of the timeline. Otherwise the drawn stand-ins.
const real = realChannel(channel.number);
const timeline: readonly Segment[] = segments.map((s) =>
  real && s.kind === "programme" && s.name === real.title ? { ...s, stillUri: real.fanartSrc } : s,
);

const CARD_WIDTH = 320;

// The preview card (pointer only): what you get for pointing at a block of the timeline. Its still,
// when it starts, its name and line, chips and two lines of synopsis, joined to its block.
const PreviewCard = ({
  density,
  full,
  heroHeight = 148,
  left,
  lift = 0,
  bare = false,
  onNow = false,
  programme,
}: {
  /**
   * Variant J's Info: the same card, held open, with the whole synopsis. It does not repeat what
   * is next or how long is left: the film strip under it and the band beside it say both.
   */
  full?: boolean;
  /** The fanart's height. A television's full card has less room above the band, so it is shorter. */
  heroHeight?: number;
  density: Density;
  left: number;
  /** Extra room under the card, so it clears whatever sits above the timeline. */
  lift?: number;
  /** A library with no art: no logo on the hero. */
  bare?: boolean;
  onNow?: boolean;
  programme: Programme;
}) => {
  // Sized by area, so a nearly square logo and a wide one carry about the same weight.
  const logo = !bare && real && programme.name === real.title ? real.logoOf : undefined;
  const logoHeight = logo ? Math.min(64, Math.sqrt(7200 / logo.ratio)) : 0;
  return (
    <div
      style={{
        background: flat.raised,
        border: `1px solid ${flat.hairline}`,
        borderRadius: 8,
        bottom: `calc(100% + ${10 + lift}px)`,
        left,
        overflow: "hidden",
        position: "absolute",
        width: CARD_WIDTH,
        zIndex: 5,
      }}
    >
      {/* The hero. A title's own logo sits here, on its own fanart, where it has room and its colours
        belong; a flat ink wash keeps it readable. Not in the band, where it fights the type. */}
      <div
        style={{
          alignItems: "flex-end",
          background: logo
            ? `linear-gradient(rgba(11, 12, 14, 0.42), rgba(11, 12, 14, 0.42)), ${flat.elevated} url("${programme.stillUri}") center/cover`
            : `${flat.elevated} url("${programme.stillUri}") center/cover`,
          display: "flex",
          height: heroHeight,
          padding: 14,
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
      <div style={{ display: "flex", flexDirection: "column", gap: 6, padding: "12px 14px 14px" }}>
        <div style={{ alignItems: "baseline", display: "flex", gap: 8, justifyContent: "space-between" }}>
          <Text density={density} numberOfLines={1} strong textRole="cardTitle" tone="primary">
            {programme.name}
          </Text>
          <Text density={density} textRole="compact" tone="secondary">
            {onNow ? `Until ${formatTime(programme.stopMs)}` : `Starts ${formatTime(programme.startMs)}`}
          </Text>
        </div>
        <Text density={density} numberOfLines={1} textRole="label" tone="secondary">
          {programme.chips.join(" · ")}
        </Text>
        <Text density={density} numberOfLines={full ? undefined : 2} textRole="label" tone="secondary">
          {programme.synopsis}
        </Text>
      </div>
    </div>
  );
};

// One key: an icon, no box, no caption. Hover is a tone step and focus is the amber ring, both from
// the shared player stylesheet.
const Key = ({
  bright = false,
  density,
  icon: Glyph,
  label,
}: {
  bright?: boolean;
  density: Density;
  icon: LucideIcon;
  label: string;
}) => {
  const box = density === "touch" ? 48 : 40;
  return (
    <button
      aria-label={label}
      className="pp-btn pp-ghost"
      style={{
        alignItems: "center",
        borderRadius: 8,
        color: bright ? flat.text : flat.textSecondary,
        display: "inline-flex",
        flexShrink: 0,
        height: box,
        justifyContent: "center",
        width: box,
      }}
      title={label}
      type="button"
    >
      <Glyph aria-hidden size={density === "touch" ? 24 : 20} strokeWidth={bright ? 2.25 : 1.75} />
    </button>
  );
};

// Channel down and up as one rocker, play, volume, more. The same five on every device.
const Keys = ({ density, playing }: { density: Density; playing: boolean }) => (
  <div style={{ alignItems: "center", display: "flex", flexShrink: 0, gap: density === "touch" ? 8 : 2 }}>
    <Key density={density} icon={ChevronDown} label="Channel down" />
    <Key density={density} icon={ChevronUp} label="Channel up" />
    <Key bright density={density} icon={playing ? Pause : Play} label={playing ? "Pause" : "Play"} />
    <Key density={density} icon={Volume2} label="Volume" />
    <Key density={density} icon={Ellipsis} label="More" />
  </div>
);

// The slim number tab: the channel's colour (amber without one) and its number, nothing else.
const NumberTab = ({ compact, density, tint }: { compact: boolean; density: Density; tint?: string }) => (
  <div
    aria-hidden
    style={{
      alignItems: "center",
      alignSelf: "stretch",
      background: tint ?? flat.amber,
      borderRadius: 6,
      display: "flex",
      flexShrink: 0,
      justifyContent: "center",
      minWidth: compact ? 44 : 52,
      padding: "0 10px",
    }}
  >
    <Text density={density} textRole={compact ? "guideNumber" : "channelNumber"} tone="inverse">
      {channel.number}
    </Text>
  </div>
);

const LineChrome = ({ device, scenario, tint, tintFor }: ChromeProps) => {
  const density: Density = device === "desktop" ? "pointer" : "touch";
  const pointer = density === "pointer";
  const portrait = device === "portrait";
  if (scenario.cover) return <Cover density={density} kind={scenario.cover} />;
  const sheet = scenario.sheet;
  const shown = scenario.visible && !sheet;
  const playing = scenario.mode !== "paused";
  // The programme a break leads back to (a break never titles itself).
  const next = timeline.find(
    (s): s is Programme => s.kind === "programme" && s.startMs > scenario.playheadMs,
  );
  const logoHeight = pointer ? 34 : 26;
  const gutter = pointer ? 32 : 16;
  return (
    <>
      {shown ? <div style={scrimStyle(pointer ? 420 : portrait ? 380 : 300, true)} /> : null}
      {/* Top right: the four utilities as icons, pinned to the corner. */}
      {shown && !portrait ? (
        <div
          style={{ display: "flex", gap: 2, position: "absolute", right: gutter - 8, top: pointer ? 20 : 8 }}
        >
          <Key density={density} icon={Captions} label="Subtitles" />
          <Key density={density} icon={AudioLines} label="Audio" />
          <Key density={density} icon={Maximize} label="Fullscreen" />
          <Key density={density} icon={X} label="Leave" />
        </div>
      ) : null}
      <div
        aria-hidden={!shown || undefined}
        style={{
          bottom: 0,
          display: "flex",
          flexDirection: "column",
          gap: pointer ? 14 : 10,
          left: 0,
          opacity: shown ? 1 : 0,
          padding: `0 ${gutter}px ${pointer ? 28 : 14}px`,
          pointerEvents: shown ? "auto" : "none",
          position: "absolute",
          right: 0,
        }}
      >
        {/* Identity and keys on one row: who you are watching on the left, what you can do on the right. */}
        <div
          style={{ alignItems: "flex-end", display: "flex", flexWrap: portrait ? "wrap" : "nowrap", gap: 14 }}
        >
          <div style={{ alignItems: "stretch", display: "flex", flex: 1, gap: 12, minWidth: 0 }}>
            <NumberTab compact={!pointer} density={density} tint={tint} />
            <div style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
              <div style={{ alignItems: "center", display: "flex", gap: 10, minWidth: 0 }}>
                <Text density={density} numberOfLines={1} strong textRole="label" tone="secondary">
                  {channel.name}
                </Text>
                {scenario.mode === "live" ? (
                  <span style={{ alignItems: "center", display: "inline-flex", flexShrink: 0, gap: 6 }}>
                    <span style={{ background: flat.live, borderRadius: 4, height: 7, width: 7 }} />
                    <Text density={density} strong textRole="label" tone="live">
                      Live
                    </Text>
                  </span>
                ) : null}
                <Text density={density} textRole="compact" tone="secondary">
                  {scenario.mode === "live" ? formatTime(NOW) : scenario.timeLabel}
                </Text>
              </div>
              <div style={{ alignItems: "flex-end", display: "flex", gap: 12, minWidth: 0 }}>
                {/* The title's own logo stands in for the typed title when the library has one. */}
                {/* A break keeps the name of the programme it leads back to; it never titles itself. */}
                {real && (!scenario.inBreak || next?.name === real.title) ? (
                  <img
                    alt={real.logoOf.alt}
                    height={logoHeight}
                    src={real.logoOf.src}
                    style={{
                      display: "block",
                      flexShrink: 0,
                      height: logoHeight,
                      width: logoHeight * real.logoOf.ratio,
                    }}
                    width={logoHeight * real.logoOf.ratio}
                  />
                ) : (
                  <Text
                    density={density}
                    numberOfLines={1}
                    strong
                    textRole={pointer ? "headline" : "cardTitle"}
                    tone="primary"
                  >
                    {scenario.inBreak ? (next?.name ?? scenario.title) : scenario.title}
                  </Text>
                )}
                {portrait ? null : (
                  <Text density={density} numberOfLines={1} textRole="label" tone="secondary">
                    {scenario.inBreak && next
                      ? `Ad break · back at ${formatTime(next.startMs)}`
                      : scenario.subtitle}
                  </Text>
                )}
              </div>
            </div>
          </div>
          <div
            style={portrait ? { display: "flex", flexBasis: "100%", justifyContent: "center" } : undefined}
          >
            <Keys density={density} playing={playing} />
          </div>
        </div>
        {/* The preview timeline is the lower third's spine: what is on, what is next, where the
            breaks fall. Pointing at a block opens its card. */}
        {/* "chrome-top" holds the next programme's preview card open over its block, for the still. */}
        <PreviewTimeline card={Boolean(scenario.topTip)} density={density} playheadMs={scenario.playheadMs} />
      </div>
      {sheet === "channels" ? (
        <Sheet density={density} icon="list" title="Channels">
          <ChannelList density={density} tintFor={tintFor} />
        </Sheet>
      ) : sheet === "more" ? (
        <Sheet density={density} icon="settings" title="Settings">
          <SettingsBody density={density} />
        </Sheet>
      ) : null}
    </>
  );
};

// The preview timeline as one piece, for a variant that opens it from somewhere else (J opens it
// from the band's progress line). `card` holds the next programme's preview card open, for a still.
const PreviewTimeline = ({
  card,
  cardFull = false,
  cardLift,
  density,
  headColor,
  playheadMs,
  quietNow,
  ring = false,
  standIns = false,
}: {
  /** Hold a preview card open for a still: the next programme (true or "next"), or the one on now. */
  card: "next" | "now" | boolean;
  /** Variant J's Info: the open card shows the whole synopsis and what follows. */
  cardFull?: boolean;
  cardLift?: number;
  density: Density;
  headColor?: string;
  playheadMs: number;
  quietNow?: boolean;
  /** Television: the focus ring. `true`: on the block whose card is open. "now": on the block on air. */
  ring?: "now" | boolean;
  /** A library with no art: every block is its drawn stand-in. */
  standIns?: boolean;
}) => {
  const pointer = density === "pointer";
  const onNow = card === "now";
  const blocks = standIns ? segments : timeline;
  const subject = blocks.find(
    (s): s is Programme =>
      s.kind === "programme" &&
      (onNow ? s.startMs <= playheadMs && playheadMs < s.stopMs : s.startMs > playheadMs),
  );
  const hovered = card && pointer ? subject : undefined;
  const { ref, width } = useWidth();
  const box = hovered ? layout(blocks, width).boxes.find((b) => b.segment.id === hovered.id) : undefined;
  const left = box ? Math.min(Math.max(box.x + box.width / 2 - CARD_WIDTH / 2, 0), width - CARD_WIDTH) : 0;
  return (
    <div ref={ref} style={{ position: "relative" }}>
      {hovered && box && width > 0 ? (
        <PreviewCard
          bare={standIns}
          density={density}
          full={cardFull}
          // Only a television has the remote's ring, and only there is the full card too tall.
          heroHeight={cardFull && ring ? 112 : undefined}
          left={left}
          lift={cardLift}
          onNow={onNow}
          programme={hovered}
        />
      ) : null}
      <Playbar
        density={density}
        headColor={headColor}
        hovered={hovered?.id}
        plain
        playheadMs={playheadMs}
        pointer={pointer}
        quietNow={quietNow}
        ringId={
          ring === "now"
            ? blocks.find((s) => s.kind === "programme" && s.startMs <= playheadMs && playheadMs < s.stopMs)
                ?.id
            : ring
              ? hovered?.id
              : undefined
        }
        segments={blocks}
      />
    </div>
  );
};

export { LineChrome, PreviewTimeline };
