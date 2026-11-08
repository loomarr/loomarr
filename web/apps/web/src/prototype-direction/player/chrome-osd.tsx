// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variant C: a television's on-screen display. A bottom-up scrim, a channel chip, the title as the
// largest thing on screen, one thin progress line with its LIVE mark, and four controls on every
// screen: channel down, play, channel up, more. The strip, Info, Settings, Start over and Last
// channel all sit behind More. Same layout on desktop, portrait and landscape; only the scale moves.
import { Text } from "@loomarr/design-system";
import type { ChromeProps } from "./chrome-classic";
import { channel } from "./data";
import { type Density, flat, GoLive, LiveMark, PlayDisc, scrimStyle, Tool } from "./flat";
import type { Scenario } from "./scenario";
import { lagLabel, stationLine } from "./scenario";
import { ChannelList, Cover, MoreBody, Sheet } from "./sheets";

const Chip = ({ density, tint }: { density: Density; tint?: string }) => (
  <div style={{ alignItems: "center", display: "flex", gap: 10 }}>
    <span
      style={{
        alignItems: "baseline",
        background: tint ?? flat.amber,
        borderRadius: 6,
        display: "inline-flex",
        gap: 6,
        padding: "3px 10px",
      }}
    >
      <Text density={density} textRole="cardChannelNumber" tone="inverse">
        CH
      </Text>
      <Text density={density} textRole="guideNumber" tone="inverse">
        {channel.number}
      </Text>
    </span>
    <Text density={density} strong textRole="label" tone="primary">
      {channel.name}
    </Text>
  </div>
);

const ProgressLine = ({ density, scenario }: { density: Density; scenario: Scenario }) => {
  const live = scenario.mode === "live";
  return (
    <div style={{ alignItems: "center", display: "flex", gap: 14 }}>
      {live ? <LiveMark density={density} /> : <GoLive density={density} />}
      <div
        style={{
          background: "rgba(255, 255, 255, 0.15)",
          borderRadius: 2,
          flex: 1,
          height: 3,
          overflow: "hidden",
        }}
      >
        <div
          style={{
            background: scenario.inBreak ? flat.info : flat.text,
            height: "100%",
            width: `${Math.round(scenario.progress * 100)}%`,
          }}
        />
      </div>
      <Text density={density} textRole="compact" tone="secondary">
        {live
          ? (scenario.timeLabel.split(" · ")[1] ?? scenario.timeLabel)
          : `${lagLabel(scenario.lagMs)} behind`}
      </Text>
    </div>
  );
};

const OsdChrome = ({ device, scenario, tint, tintFor }: ChromeProps) => {
  const density: Density = device === "desktop" ? "pointer" : "touch";
  const pointer = density === "pointer";
  const landscape = device === "landscape";
  if (scenario.cover) return <Cover density={density} kind={scenario.cover} />;
  const sheet = scenario.sheet;
  const controls = scenario.visible && !sheet;
  const card = scenario.ident && !scenario.visible && !sheet;
  const shown = controls || card;
  const small = !pointer;
  return (
    <>
      {shown ? <div style={scrimStyle(pointer ? 460 : landscape ? 390 : 400, true)} /> : null}
      <div
        aria-hidden={!shown || undefined}
        style={{
          bottom: 0,
          display: "flex",
          flexDirection: "column",
          gap: pointer ? 14 : landscape ? 6 : 10,
          left: 0,
          opacity: shown ? 1 : 0,
          padding: pointer ? "0 32px 36px" : landscape ? "0 16px 10px" : "0 16px 28px",
          pointerEvents: shown ? "auto" : "none",
          position: "absolute",
          right: 0,
        }}
      >
        <Chip density={density} tint={tint} />
        <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
          <Text
            density={density}
            numberOfLines={2}
            strong
            textRole={pointer ? "display" : "title"}
            tone="primary"
          >
            {scenario.title}
          </Text>
          <Text density={density} numberOfLines={1} textRole={pointer ? "reading" : "body"} tone="secondary">
            {card ? stationLine : scenario.subtitle}
          </Text>
        </div>
        {card ? null : (
          <>
            <ProgressLine density={density} scenario={scenario} />
            <div
              style={{
                alignItems: "center",
                display: "flex",
                gap: small ? 20 : 32,
                justifyContent: "center",
                marginTop: pointer ? 6 : 0,
              }}
            >
              <Tool
                density={density}
                bright
                icon="channelDown"
                keyHint="↓"
                label="Channel down"
                size={small ? 52 : 56}
              />
              <PlayDisc playing={scenario.mode !== "paused"} size={small ? 60 : 68} />
              <Tool
                density={density}
                bright
                icon="channelUp"
                keyHint="↑"
                label="Channel up"
                size={small ? 52 : 56}
              />
              <Tool
                density={density}
                bright
                icon="more"
                keyHint="M"
                label="More"
                size={small ? 52 : 56}
                tipAlign="end"
              />
            </div>
          </>
        )}
      </div>
      {sheet === "channels" ? (
        <Sheet density={density} icon="list" title="Channels">
          <ChannelList density={density} tintFor={tintFor} />
        </Sheet>
      ) : sheet === "more" ? (
        <Sheet density={density} icon="more" title="More">
          <MoreBody behind={scenario.mode !== "live"} density={density} />
        </Sheet>
      ) : null}
    </>
  );
};

export { OsdChrome };
