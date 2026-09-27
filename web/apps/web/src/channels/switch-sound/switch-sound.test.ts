import { describe, expect, it } from "vitest";
import {
  playSwitchSound,
  SWITCH_SOUND_VARIANTS,
  shouldPlaySwitchSound,
  switchSoundEnvelope,
} from "./switch-sound";

type Call = [string, number | string, number];
interface RecordedNode {
  kind: string;
  calls: Call[];
  started?: number;
  stopped?: number;
}

// A recording stand-in for the Web Audio graph: every node the sound builds, and every automation
// call on its params, so the test reads the envelope and the sweeps the listener would hear.
const fakeContext = () => {
  const nodes: RecordedNode[] = [];
  const param = (calls: Call[], name: string) => ({
    setValueAtTime: (v: number, t: number) => calls.push([`${name}.set`, v, t]),
    linearRampToValueAtTime: (v: number, t: number) => calls.push([`${name}.linear`, v, t]),
    exponentialRampToValueAtTime: (v: number, t: number) => calls.push([`${name}.exp`, v, t]),
  });
  const node = (kind: string) => {
    const record: RecordedNode = { kind, calls: [] };
    nodes.push(record);
    return {
      connect: (next: unknown) => next,
      gain: param(record.calls, "gain"),
      frequency: param(record.calls, "frequency"),
      Q: { value: 0 },
      set type(value: string) {
        record.calls.push(["type", value, 0]);
      },
      buffer: null as unknown,
      start: (t: number) => {
        record.started = t;
      },
      stop: (t: number) => {
        record.stopped = t;
      },
    };
  };
  const ctx = {
    currentTime: 10,
    sampleRate: 8000,
    destination: {},
    createBuffer: (_channels: number, length: number) => ({ getChannelData: () => new Float32Array(length) }),
    createBufferSource: () => node("source"),
    createOscillator: () => node("oscillator"),
    createBiquadFilter: () => node("filter"),
    createGain: () => node("gain"),
  };
  return { ctx: ctx as unknown as BaseAudioContext, nodes };
};

const filterOf = (nodes: RecordedNode[], type: string) =>
  nodes.find(
    (n) => n.kind === "filter" && n.calls.some(([name, value]) => name === "type" && value === type),
  );

const audible = { enabled: true, muted: false, volume: 1, activated: true };

describe("switchSoundEnvelope", () => {
  // The soft hiss (C), the maintainer's pick.
  const burst = SWITCH_SOUND_VARIANTS.C.burst;

  it("attacks in 12 ms to 0.35, holds to 45% of the hiss, and is silent by 450 ms", () => {
    const [start, peak, hold, end] = switchSoundEnvelope(burst, 1);
    expect(start).toEqual({ t: 0, gain: 0 });
    expect(peak.t).toBeCloseTo(0.012);
    expect(peak.gain).toBeCloseTo(0.35);
    expect(hold.t).toBeCloseTo(0.45 * 0.45);
    expect(hold.gain).toBeCloseTo(0.35);
    expect(end.t).toBeCloseTo(0.45);
    expect(end.gain).toBeLessThan(0.001);
  });

  it("scales with the player's volume", () => {
    expect(switchSoundEnvelope(burst, 0.25)[1].gain).toBeCloseTo(0.35 * 0.25);
  });
});

describe("shouldPlaySwitchSound", () => {
  it("plays for an audible, activated player with the preference on", () => {
    expect(shouldPlaySwitchSound({ ...audible, volume: 0.8 })).toBe(true);
  });

  it("is silent when muted, at zero volume, turned off, or before any user gesture", () => {
    expect(shouldPlaySwitchSound({ ...audible, muted: true })).toBe(false);
    expect(shouldPlaySwitchSound({ ...audible, volume: 0 })).toBe(false);
    expect(shouldPlaySwitchSound({ ...audible, enabled: false })).toBe(false);
    expect(shouldPlaySwitchSound({ ...audible, activated: false })).toBe(false);
  });
});

describe("playSwitchSound", () => {
  it("builds nothing when the gate says silent", () => {
    const { ctx, nodes } = fakeContext();
    expect(playSwitchSound(ctx, { ...audible, muted: true })).toBe(false);
    expect(nodes).toEqual([]);
  });

  it("defaults to the soft hiss: one bandpass (Q 0.6) noise sweeping 1.8 kHz→900 Hz, silent by 450 ms", () => {
    const { ctx, nodes } = fakeContext();
    expect(playSwitchSound(ctx, { ...audible, volume: 0.5 })).toBe(true);
    // No click, no oscillator: one noise source through one bandpass.
    expect(nodes.some((n) => n.kind === "oscillator")).toBe(false);
    expect(nodes.filter((n) => n.kind === "source")).toHaveLength(1);
    expect(filterOf(nodes, "highpass")).toBeUndefined();
    const bandpass = filterOf(nodes, "bandpass");
    expect(bandpass?.calls).toContainEqual(["frequency.set", 1800, 10]);
    expect(bandpass?.calls).toContainEqual(["frequency.exp", 900, 10.45]);
    const gain = nodes.find((n) => n.kind === "gain");
    expect(gain?.calls).toContainEqual(["gain.linear", 0.35 * 0.5, 10.012]);
    const stops = nodes.map((n) => n.stopped).filter((t): t is number => t !== undefined);
    expect(Math.max(...stops)).toBeCloseTo(10.45);
  });

  it("B: a square thunk 170→55 Hz at 0.45 and a 900 Hz high-passed click at 0.9, with the burst", () => {
    const { ctx, nodes } = fakeContext();
    playSwitchSound(ctx, { ...audible, volume: 0.5, variant: "B" });
    const osc = nodes.find((n) => n.kind === "oscillator");
    expect(osc?.calls).toContainEqual(["type", "square", 0]);
    expect(osc?.calls).toContainEqual(["frequency.set", 170, 10]);
    expect(osc?.calls).toContainEqual(["frequency.exp", 55, 10.035]);
    expect(osc?.started).toBe(10);
    const gains = nodes.filter((n) => n.kind === "gain").map((n) => n.calls[0]);
    expect(gains).toContainEqual(["gain.set", 0.45 * 0.5, 10]);
    expect(gains).toContainEqual(["gain.set", 0.9 * 0.5, 10]);
    expect(filterOf(nodes, "highpass")?.calls).toContainEqual(["frequency.set", 900, 10]);
  });

  it("A/B's burst: bandpass (Q 0.9) sweeping 3.2→1.4 kHz, stopped at 340 ms — not the whole tune", () => {
    const { ctx, nodes } = fakeContext();
    playSwitchSound(ctx, { ...audible, variant: "B" });
    const bandpass = filterOf(nodes, "bandpass");
    expect(bandpass?.calls).toContainEqual(["frequency.set", 3200, 10]);
    expect(bandpass?.calls).toContainEqual(["frequency.exp", 1400, 10.34]);
    const stops = nodes.map((n) => n.stopped).filter((t): t is number => t !== undefined);
    expect(Math.max(...stops)).toBeCloseTo(10.34);
  });

  it("A is the burst without the dial", () => {
    const { ctx, nodes } = fakeContext();
    playSwitchSound(ctx, { ...audible, variant: "A" });
    expect(nodes.some((n) => n.kind === "oscillator")).toBe(false);
    expect(filterOf(nodes, "bandpass")).toBeDefined();
  });
});
