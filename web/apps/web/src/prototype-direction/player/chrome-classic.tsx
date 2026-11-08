// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variants A and B: today's five planes, flattened. A is the minimum change (band, preview playbar,
// capsule transport, sheets). B adds channel down / channel up to the transport, with the arrow keys
// drawn beside them; the strip stays.
import { Text } from "@loomarr/design-system";

import { channel, segments } from "./data";
import {
  Capsule,
  type Density,
  flat,
  GoLive,
  LastChannel,
  LiveMark,
  PlayDisc,
  StartOver,
  scrimStyle,
  Tool,
} from "./flat";
import { Playbar } from "./playbar";
import type { PlayerDevice, Scenario } from "./scenario";
import { lagLabel, stationLine } from "./scenario";
import { ChannelList, Cover, SettingsBody, Sheet } from "./sheets";

interface ChromeProps {
  device: PlayerDevice;
  scenario: Scenario;
  tint?: string;
  tintFor: (era: string) => string | undefined;
}

// The lower third: an amber (or era-tinted) channel block, flat, beside a solid panel.
const Band = ({
  density,
  ident,
  scenario,
  tint,
}: {
  density: Density;
  ident: boolean;
  scenario: Scenario;
  tint?: string;
}) => {
  const compact = density === "touch";
  const blockWidth = ident ? (compact ? 64 : 96) : compact ? 48 : 64;
  return (
    <div style={{ alignItems: "stretch", display: "flex", maxWidth: 960 }}>
      <div
        aria-hidden
        style={{
          alignItems: "center",
          background: tint ?? flat.amber,
          borderRadius: "8px 0 0 8px",
          display: "flex",
          flexDirection: "column",
          justifyContent: "center",
          width: blockWidth,
        }}
      >
        <Text density={density} textRole="cardChannelNumber" tone="inverse">
          CH
        </Text>
        <Text
          density={density}
          textRole={
            ident ? (compact ? "channelNumber" : "display") : compact ? "guideNumber" : "channelNumber"
          }
          tone="inverse"
        >
          {channel.number}
        </Text>
      </div>
      <div
        style={{
          background: flat.raised,
          border: `1px solid ${flat.hairline}`,
          borderLeft: 0,
          borderRadius: "0 8px 8px 0",
          display: "flex",
          flex: 1,
          flexDirection: "column",
          gap: 2,
          minWidth: 0,
          padding: "10px 16px",
        }}
      >
        <div style={{ alignItems: "baseline", display: "flex", gap: 8 }}>
          <Text density={density} textRole="cardChannelNumber" tone="secondary">
            {`CH ${channel.number}`}
          </Text>
          <Text density={density} numberOfLines={1} strong textRole="label" tone="secondary">
            {ident ? `${channel.name} · ${stationLine}` : channel.name}
          </Text>
        </div>
        <Text
          density={density}
          numberOfLines={1}
          strong
          textRole={compact ? "cardTitle" : "headline"}
          tone="primary"
        >
          {compact ? scenario.programmeShort : scenario.programmeLine}
        </Text>
        <div style={{ alignItems: "center", display: "flex", gap: 12, marginTop: 6 }}>
          <div style={{ background: flat.hairline, borderRadius: 2, flex: 1, height: 4, overflow: "hidden" }}>
            <div
              style={{
                background: scenario.inBreak ? flat.info : flat.text,
                height: "100%",
                width: `${Math.round(scenario.progress * 100)}%`,
              }}
            />
          </div>
          <Text density={density} textRole="compact" tone="secondary">
            {scenario.timeLabel}
          </Text>
        </div>
      </div>
    </div>
  );
};

const Transport = ({
  density,
  scenario,
  tuner,
}: {
  density: Density;
  scenario: Scenario;
  tuner: boolean;
}) => {
  const pointer = density === "pointer";
  const live = scenario.mode === "live";
  const playing = scenario.mode !== "paused";
  const liveState = live ? (
    <div style={{ alignItems: "center", display: "flex", height: 36, padding: "0 12px" }}>
      <LiveMark density={density} />
    </div>
  ) : (
    <div style={{ alignItems: "center", display: "flex", gap: 10, paddingLeft: 12 }}>
      {pointer ? (
        <Text density={density} textRole="compact" tone="secondary">
          {`${scenario.mode === "paused" ? "Paused · " : ""}${lagLabel(scenario.lagMs)} behind`}
        </Text>
      ) : null}
      <GoLive density={density} />
    </div>
  );
  const startOver = scenario.inBreak ? null : live ? null : <StartOver density={density} />;
  return (
    <div style={{ alignItems: "center", display: "flex", flexWrap: "wrap", gap: pointer ? 10 : 8 }}>
      <Capsule>
        <Tool density={density} icon="seekBack" keyHint="←" label="Back 10 seconds" tipAlign="start" />
        <PlayDisc playing={playing} size={density === "touch" ? 48 : 44} />
        <Tool density={density} disabled={live} icon="seekForward" keyHint="→" label="Forward 10 seconds" />
      </Capsule>
      {tuner ? (
        <Capsule>
          <Tool
            density={density}
            icon="channelDown"
            keyHint="↓"
            keycap={pointer ? "↓" : undefined}
            label="Channel down"
          />
          <Tool
            density={density}
            icon="channelUp"
            keyHint="↑"
            keycap={pointer ? "↑" : undefined}
            label="Channel up"
          />
        </Capsule>
      ) : null}
      <Capsule>
        {liveState}
        {startOver}
      </Capsule>
      {pointer ? (
        <Capsule end>
          <Tool density={density} icon="volume" keyHint="M" label="Mute" />
          <div style={{ alignItems: "center", display: "flex", height: 36, width: 80 }}>
            <div
              style={{
                background: "rgba(255, 255, 255, 0.15)",
                borderRadius: 2,
                height: 4,
                position: "relative",
                width: "100%",
              }}
            >
              <div style={{ background: flat.text, borderRadius: 2, height: "100%", width: "80%" }} />
              <div
                style={{
                  background: "#FFFFFF",
                  borderRadius: 999,
                  height: 10,
                  left: "80%",
                  marginLeft: -5,
                  position: "absolute",
                  top: -3,
                  width: 10,
                }}
              />
            </div>
          </div>
        </Capsule>
      ) : (
        <LastChannel density={density} lastNumber="3" />
      )}
      <div style={{ display: "flex", gap: 10, marginLeft: "auto" }}>
        <Capsule>
          <Tool density={density} icon="settings" label="Settings" />
          <Tool density={density} icon="info" keyHint="I" label="Info" />
          <Tool density={density} icon="list" keyHint="G" label="Channels" />
        </Capsule>
        {pointer ? (
          <Capsule>
            <Tool density={density} icon="fullscreen" label="Fullscreen" />
            <Tool density={density} icon="close" label="Leave Watch" tipAlign="end" />
          </Capsule>
        ) : null}
      </div>
    </div>
  );
};

const ClassicChrome = ({ device, scenario, tint, tintFor, tuner }: ChromeProps & { tuner: boolean }) => {
  const density: Density = device === "desktop" ? "pointer" : "touch";
  const pointer = density === "pointer";
  if (scenario.cover) return <Cover density={density} kind={scenario.cover} />;
  const sheet = scenario.sheet;
  const controls = scenario.visible && !sheet;
  const card = scenario.ident && !scenario.visible && !sheet;
  const shown = controls || card;
  return (
    <>
      {shown ? <div style={scrimStyle(pointer ? 380 : device === "landscape" ? 300 : 340)} /> : null}
      <div
        aria-hidden={!shown || undefined}
        style={{
          bottom: 0,
          display: "flex",
          flexDirection: "column",
          gap: pointer ? 12 : 8,
          left: 0,
          opacity: shown ? 1 : 0,
          padding: pointer ? "96px 32px 24px" : device === "landscape" ? "24px 16px 10px" : "40px 16px 14px",
          pointerEvents: shown ? "auto" : "none",
          position: "absolute",
          right: 0,
        }}
      >
        <Band density={density} ident={card} scenario={scenario} tint={tint} />
        {card ? null : (
          <>
            <Playbar
              density={density}
              playheadMs={scenario.playheadMs}
              pointer={pointer}
              segments={segments}
            />
            <Transport density={density} scenario={scenario} tuner={tuner} />
          </>
        )}
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

export type { ChromeProps };
export { ClassicChrome };
