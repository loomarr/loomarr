// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// What each state of the player means: where the playhead sits, what the band says, which sheet or
// cover is up. Shared by every variant, so the three differ in drawing, never in the story.
import { b1, channel, formatTime, MINUTE, NOW, segments } from "./data";

const STATES = [
  "playing",
  "idle",
  "station-id",
  "behind",
  "commercials",
  "channels",
  "more",
  "error-server",
  "error-not-started",
  "off-air",
  // G only: flipping channels (the neighbours strip) and the tuner's static. The other variants draw
  // both as "playing".
  "surfing",
  "tuning",
  // H only: the signature tune playing once (CH 7 to CH 9), and the same with a slow stream, which
  // holds on the bars until the picture arrives. Other variants draw both as "playing".
  "tune",
  "cold-tune",
  "slow-tune",
  // H only: the top-right utility cluster with a tooltip showing, and its panels. The other variants
  // draw all four as "playing".
  "chrome-top",
  "subtitles",
  "audio",
  "details",
  // J only: the rest of what the real player has. Picture quality (a page of the More menu), Info,
  // the volume level, the muted first play, a typed number no channel has, a channel with nothing
  // scheduled, and the stream stopping. The other variants draw the first six as "playing".
  "picture",
  "info",
  "volume",
  "unmute",
  "no-channel",
  "no-schedule",
  "error-stopped",
] as const;
type PlayerState = (typeof STATES)[number];

const VARIANTS = [
  { key: "A", name: "Flat B2" },
  { key: "B", name: "Flat B2 + tuner" },
  { key: "C", name: "On-screen display" },
  { key: "D", name: "Rich flat" },
  { key: "E", name: "Polished flat" },
  { key: "F", name: "Glass" },
  { key: "G", name: "Loomarr TV" },
  { key: "H", name: "Loomarr TV · signature tune" },
  { key: "I", name: "Loomarr TV · preview timeline" },
  { key: "J", name: "Loomarr TV · G refined" },
] as const;

// "tv" is the ten-foot layout the Apple TV and Android TV (Shield) apps share: both lay out at 960
// wide, so the frame is 960 × 540 drawn at 1.5×. Variant J draws it; the others treat it as desktop.
const DEVICES = ["desktop", "portrait", "landscape", "tv"] as const;
type PlayerDevice = (typeof DEVICES)[number];

const lagLabel = (ms: number) => {
  const total = Math.round(ms / 1000);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return m > 0 ? `${m}m ${s}s` : `${s}s`;
};

interface Scenario {
  /** The break airing at the playhead, with the spot on air. */
  breakSpot?: string;
  cover?: "error-not-started" | "error-server" | "error-stopped" | "off-air";
  /** Variant J: what else is open, beyond H's three panels. */
  extra?: "info" | "picture" | "volume";
  /** Variant J: a number was typed that no channel has. */
  missed?: boolean;
  /** Variant J: the channel has nothing scheduled, so the band has only its name. */
  noSchedule?: boolean;
  /** Variant J: the first play of a session starts muted; the pill that undoes it is up. */
  unmute?: boolean;
  ident: boolean;
  inBreak: boolean;
  lagMs: number;
  mode: "behind" | "live" | "paused";
  playheadMs: number;
  progress: number;
  /** `Series · S2 E4 "Episode"`, or `Commercials · <spot>`. */
  programmeLine: string;
  /** The same on a phone, where the episode's name has no room. */
  programmeShort: string;
  sheet?: "channels" | "more";
  /** Variant H: a cold start, so the tune is paced by a real wait (about two seconds). */
  cold?: boolean;
  /** Variant H: the panel open from the top cluster. */
  panel?: "audio" | "details" | "subtitles";
  /** Variant H: the stream is slow, so the tune holds on the bars until the picture arrives. */
  slow?: boolean;
  /** Variant H: keep the Leave tooltip showing, for the still. */
  topTip?: boolean;
  state: PlayerState;
  /** Variant G: the viewer is flipping channels (the strip is up). */
  surfing?: boolean;
  timeLabel: string;
  /** Variant H: play the signature tune on load. */
  tune?: boolean;
  /** Variant G: the tuner is between channels (static locking to the brand bars). */
  tuning?: boolean;
  /** The title as the on-screen display sets it. */
  title: string;
  /** The line under the title. */
  subtitle: string;
  visible: boolean;
}

const diner = segments.find((s) => s.id === "p2");
if (diner?.kind !== "programme") throw new Error("stub data: p2 is a programme");

const scenarioOf = (state: PlayerState): Scenario => ({ ...scenarioFor(state), state });

const scenarioFor = (state: PlayerState): Omit<Scenario, "state"> => {
  const programme = (playheadMs: number) => ({
    inBreak: false,
    progress: (playheadMs - diner.startMs) / (diner.stopMs - diner.startMs),
    programmeLine: `${diner.name} · ${diner.line}`,
    programmeShort: `${diner.name} · ${diner.line.split(" ").slice(0, 2).join(" ")}`,
    subtitle: diner.line,
    timeLabel: `${Math.round((playheadMs - diner.startMs) / MINUTE)}m in · ${Math.round((diner.stopMs - playheadMs) / MINUTE)}m left`,
    title: diner.name,
  });
  const base = {
    ...programme(NOW),
    ident: false,
    lagMs: 0,
    mode: "live" as const,
    playheadMs: NOW,
    visible: true,
  };
  switch (state) {
    case "idle":
      return { ...base, visible: false };
    case "station-id":
      return { ...base, ident: true, visible: false };
    case "behind": {
      const playheadMs = NOW - 134_000;
      return {
        ...base,
        ...programme(playheadMs),
        lagMs: 134_000,
        mode: "paused",
        playheadMs,
        timeLabel: `${lagLabel(134_000)} behind`,
      };
    }
    case "commercials": {
      const playheadMs = b1.startMs + 60_000;
      const spot = b1.spots[1]?.name ?? "";
      return {
        ...base,
        breakSpot: spot,
        inBreak: true,
        lagMs: NOW - playheadMs,
        mode: "behind",
        playheadMs,
        progress: 60_000 / (b1.stopMs - b1.startMs),
        programmeLine: `Commercials · ${spot}`,
        programmeShort: `Commercials · ${spot}`,
        subtitle: `Break · ${spot}`,
        timeLabel: `Break · ${Math.round((b1.stopMs - b1.startMs) / MINUTE)} min`,
        title: "Commercials",
      };
    }
    case "surfing":
      return { ...base, surfing: true };
    case "tuning":
      return { ...base, tuning: true };
    case "tune":
      return { ...base, tune: true };
    case "cold-tune":
      return { ...base, cold: true, tune: true };
    case "slow-tune":
      return { ...base, slow: true, tune: true };
    case "chrome-top":
      return { ...base, topTip: true };
    case "subtitles":
    case "audio":
    case "details":
      return { ...base, panel: state };
    case "channels":
      return { ...base, sheet: "channels" };
    case "more":
      return { ...base, sheet: "more" };
    case "picture":
    case "info":
    case "volume":
      return { ...base, extra: state };
    case "unmute":
      return { ...base, unmute: true, visible: false };
    case "no-channel":
      return { ...base, missed: true, visible: false };
    case "no-schedule":
      return { ...base, noSchedule: true };
    case "error-server":
    case "error-not-started":
    case "error-stopped":
    case "off-air":
      return { ...base, cover: state, visible: false };
    default:
      return base;
  }
};

const stationLine = `${channel.era} · ${channel.place}`;

export type { PlayerDevice, PlayerState, Scenario };
export { DEVICES, formatTime, lagLabel, STATES, scenarioOf, stationLine, VARIANTS };
