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
] as const;
type PlayerState = (typeof STATES)[number];

const VARIANTS = [
  { key: "A", name: "Flat B2" },
  { key: "B", name: "Flat B2 + tuner" },
  { key: "C", name: "On-screen display" },
  { key: "D", name: "Rich flat" },
] as const;

const DEVICES = ["desktop", "portrait", "landscape"] as const;
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
  cover?: "error-not-started" | "error-server" | "off-air";
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
  timeLabel: string;
  /** The title as the on-screen display sets it. */
  title: string;
  /** The line under the title. */
  subtitle: string;
  visible: boolean;
}

const diner = segments.find((s) => s.id === "p2");
if (diner?.kind !== "programme") throw new Error("stub data: p2 is a programme");

const scenarioOf = (state: PlayerState): Scenario => {
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
    case "channels":
      return { ...base, sheet: "channels" };
    case "more":
      return { ...base, sheet: "more" };
    case "error-server":
    case "error-not-started":
    case "off-air":
      return { ...base, cover: state, visible: false };
    default:
      return base;
  }
};

const stationLine = `${channel.era} · ${channel.place}`;

export type { PlayerDevice, PlayerState, Scenario };
export { DEVICES, formatTime, lagLabel, STATES, scenarioOf, stationLine, VARIANTS };
