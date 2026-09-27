import { describe, expect, it } from "vitest";
import { STATIC_PEAK, shouldPlaySwitchSound, startSwitchSound } from "./switch-sound";

type Call = [string, number, number?];
interface RecordedNode {
  kind: string;
  calls: Call[];
  props: Record<string, unknown>;
  connects: RecordedNode[];
  started?: number;
  stopped?: number;
}

// A recording stand-in for the Web Audio graph: every node the sound builds, its settings, its
// connections and every automation call, so the test reads what the listener would hear.
const fakeContext = (now = 10) => {
  const nodes: RecordedNode[] = [];
  const destination: RecordedNode = { kind: "destination", calls: [], props: {}, connects: [] };
  const param = (record: RecordedNode, name: string) => ({
    set value(v: number) {
      record.props[name] = v;
    },
    get value() {
      return (record.props[name] as number) ?? 0;
    },
    setValueAtTime: (v: number, t: number) => record.calls.push([`${name}.set`, v, t]),
    linearRampToValueAtTime: (v: number, t: number) => record.calls.push([`${name}.linear`, v, t]),
    exponentialRampToValueAtTime: (v: number, t: number) => record.calls.push([`${name}.exp`, v, t]),
    cancelScheduledValues: (t: number) => record.calls.push([`${name}.cancel`, t]),
  });
  const node = (kind: string) => {
    const record: RecordedNode = { kind, calls: [], props: {}, connects: [] };
    nodes.push(record);
    const handle = {
      record,
      connect: (next: { record?: RecordedNode }) => {
        record.connects.push(next.record ?? destination);
        return next;
      },
      gain: param(record, "gain"),
      frequency: param(record, "frequency"),
      Q: param(record, "Q"),
      set type(v: string) {
        record.props.type = v;
      },
      set loop(v: boolean) {
        record.props.loop = v;
      },
      set curve(v: Float32Array) {
        record.props.curve = v;
      },
      buffer: null as unknown,
      start: (t: number) => {
        record.started = t;
      },
      stop: (t: number) => {
        record.stopped = t;
      },
    };
    return handle;
  };
  const ctx = {
    currentTime: now,
    sampleRate: 8000,
    destination: { record: destination },
    createBuffer: (_channels: number, length: number) => ({ getChannelData: () => new Float32Array(length) }),
    createBufferSource: () => node("source"),
    createOscillator: () => node("oscillator"),
    createBiquadFilter: () => node("filter"),
    createWaveShaper: () => node("shaper"),
    createGain: () => node("gain"),
  };
  return { ctx: ctx as unknown as BaseAudioContext, nodes, destination };
};

const audible = { enabled: true, muted: false, volume: 1, activated: true };
const T = 10 + 0.005; // the sound starts 5 ms after "now", as in the reference

const staticGain = (nodes: RecordedNode[]) =>
  nodes.find(
    (n) => n.kind === "gain" && n.calls.some(([name, v]) => name === "gain.linear" && v === STATIC_PEAK),
  );
const filter = (nodes: RecordedNode[], type: string) =>
  nodes.find((n) => n.kind === "filter" && n.props.type === type);

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

describe("startSwitchSound", () => {
  it("builds nothing when the gate says silent", () => {
    const { ctx, nodes } = fakeContext();
    expect(startSwitchSound(ctx, { ...audible, muted: true })).toBeUndefined();
    expect(nodes).toEqual([]);
  });

  it("opens with the clunk: a 95→42 Hz sine thump and two high-passed ticks", () => {
    const { ctx, nodes } = fakeContext();
    startSwitchSound(ctx, audible);
    const thump = nodes.find((n) => n.kind === "oscillator" && n.props.type === "sine");
    expect(thump?.calls).toContainEqual(["frequency.set", 95, T]);
    expect(thump?.calls).toContainEqual(["frequency.exp", 42, T + 0.07]);
    const thumpGain = thump?.connects[0];
    expect(thumpGain?.calls[0]).toEqual(["gain.set", 0.32, T]);
    const ticks = nodes
      .filter((n) => n.kind === "filter" && n.props.type === "highpass" && n.props.frequency !== 170)
      .map((n) => ({ hz: n.props.frequency, gain: n.connects[0]?.calls[0] }));
    expect(ticks).toEqual([
      { hz: 1500, gain: ["gain.set", 0.32, T] },
      { hz: 800, gain: ["gain.set", 0.14, T + 0.012] },
    ]);
  });

  it("then raw static with a 59.94 Hz buzz through a small speaker, up to 0.13 in 5 ms", () => {
    const { ctx, nodes } = fakeContext();
    startSwitchSound(ctx, audible);
    const noise = nodes.find((n) => n.kind === "source" && n.props.loop === true);
    expect(noise?.started).toBeCloseTo(T + 0.02);
    const buzz = nodes.find((n) => n.kind === "oscillator" && n.props.type === "square");
    expect(buzz?.props.frequency).toBeCloseTo(59.94);
    expect(buzz?.connects[0]?.props.gain).toBe(0.045);
    const g = staticGain(nodes);
    expect(g?.calls.slice(0, 2)).toEqual([
      ["gain.set", 0, T + 0.02],
      ["gain.linear", STATIC_PEAK, T + 0.025],
    ]);
    // The small speaker: 170 Hz high-pass, a +5 dB bump at 2.4 kHz, a 6.2 kHz low-pass, tanh drive.
    const speakerIn = nodes.find((n) => n.props.type === "highpass" && n.props.frequency === 170);
    expect(speakerIn?.props).toMatchObject({ Q: 0.7 });
    expect(speakerIn?.connects[0]).toBe(filter(nodes, "peaking"));
    expect(filter(nodes, "peaking")?.props).toMatchObject({ frequency: 2400, gain: 5, Q: 1.1 });
    expect(filter(nodes, "lowpass")?.props).toMatchObject({ frequency: 6200 });
    const curve = nodes.find((n) => n.kind === "shaper")?.props.curve as Float32Array;
    expect(curve[1023]).toBeCloseTo(1);
    expect(curve[768]).toBeCloseTo(Math.tanh(1.6 * (768 / 511.5 - 1)) / Math.tanh(1.6));
  });

  it("ducks a slow tune: 0.13 held to 1.0 s, down to 0.04 by 1.25 s", () => {
    const { ctx, nodes } = fakeContext();
    startSwitchSound(ctx, audible);
    expect(staticGain(nodes)?.calls.slice(2)).toEqual([
      ["gain.set", STATIC_PEAK, T + 1.0],
      ["gain.linear", 0.04, T + 1.25],
    ]);
  });

  it("never lets the static exceed 0.13 times the player's volume", () => {
    const { ctx, nodes } = fakeContext();
    startSwitchSound(ctx, { ...audible, volume: 0.5 });
    const levels = (staticGain(nodes)?.calls ?? [])
      .filter(([name]) => name !== "gain.cancel")
      .map(([, v]) => v);
    expect(Math.max(...levels)).toBe(STATIC_PEAK);
    // Every path reaches the output through one master at the player's volume.
    const master = nodes.find((n) => n.kind === "gain" && n.connects.some((c) => c.kind === "destination"));
    expect(master?.props.gain).toBe(0.5);
    expect(nodes.filter((n) => n.connects.some((c) => c.kind === "destination"))).toEqual([master]);
  });

  it("cuts the static at lock within 4 ms, a cut not a fade, and stops it", () => {
    const { ctx, nodes } = fakeContext();
    const sound = startSwitchSound(ctx, audible);
    sound?.lock(12);
    const g = staticGain(nodes);
    const after = g?.calls.slice(g.calls.findIndex(([name]) => name === "gain.cancel")) ?? [];
    expect(after[0]).toEqual(["gain.cancel", 12]);
    expect(after[1]?.[0]).toBe("gain.set");
    expect(after[2]?.[0]).toBe("gain.linear");
    expect(after[2]?.[1]).toBe(0);
    expect((after[2]?.[2] ?? 0) - 12).toBeLessThanOrEqual(0.004);
    const loops = nodes.filter((n) => n.props.loop === true || n.props.type === "square");
    expect(loops.map((n) => n.stopped)).toEqual([12.01, 12.01]);
  });

  it("locks once; a second lock does nothing", () => {
    const { ctx, nodes } = fakeContext();
    const sound = startSwitchSound(ctx, audible);
    sound?.lock(12);
    const count = staticGain(nodes)?.calls.length;
    sound?.lock(13);
    expect(staticGain(nodes)?.calls.length).toBe(count);
  });
});
