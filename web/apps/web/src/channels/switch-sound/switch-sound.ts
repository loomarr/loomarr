// The channel-change sound (#1620): what an analog set makes between channels. A SHORT soft hiss of
// static, played once as a switch begins — over well before a slow channel has finished tuning.
// Synthesized with the Web Audio API, so there is no audio asset. It follows the player's own volume
// and mute, stays silent until the page has had a user gesture, and the viewer can turn it off
// (useSwitchSoundPreference, the Audio menu's "Channel-change sound").
//
// The maintainer picked C, the soft hiss, from a demo of three (2026-09-27). A (the full static
// burst) and B (a rotary-dial click, then the burst) stay as internal variants.

type SwitchSoundVariant = "A" | "B" | "C";

interface StaticBurst {
  durationS: number;
  attackS: number;
  /** Peak gain at full player volume. */
  peak: number;
  /** The peak holds until this fraction of the duration, then fades exponentially to silence. */
  holdFraction: number;
  /** A bandpass sweeping down across the burst: the tuner falling off a station. */
  sweepFromHz: number;
  sweepToHz: number;
  q: number;
}

interface DialClick {
  /** A square wave dropping in pitch: the dial's mechanical thunk. */
  oscFromHz: number;
  oscToHz: number;
  oscSweepS: number;
  oscPeak: number;
  oscDecayS: number;
  /** A few milliseconds of high-passed noise: the click's edge. */
  clickS: number;
  clickHighpassHz: number;
  clickPeak: number;
  clickDecayS: number;
}

interface SwitchSoundShape {
  burst: StaticBurst;
  dial?: DialClick;
}

const BURST: StaticBurst = {
  durationS: 0.34,
  attackS: 0.012,
  peak: 0.8,
  holdFraction: 0.45,
  sweepFromHz: 3200,
  sweepToHz: 1400,
  q: 0.9,
};

const SWITCH_SOUND_VARIANTS: Record<SwitchSoundVariant, SwitchSoundShape> = {
  // A: the static burst alone.
  A: { burst: BURST },
  // B: the rotary-dial click, then the burst.
  B: {
    burst: BURST,
    dial: {
      oscFromHz: 170,
      oscToHz: 55,
      oscSweepS: 0.035,
      oscPeak: 0.45,
      oscDecayS: 0.045,
      clickS: 0.03,
      clickHighpassHz: 900,
      clickPeak: 0.9,
      clickDecayS: 0.025,
    },
  },
  // C (default): a soft hiss — lower, longer and quieter, with no click.
  C: { burst: { ...BURST, durationS: 0.45, peak: 0.35, sweepFromHz: 1800, sweepToHz: 900, q: 0.6 } },
};

const DEFAULT_VARIANT: SwitchSoundVariant = "C";

// An exponential ramp cannot reach 0, so fades land on this.
const SILENCE = 0.0001;

interface EnvelopePoint {
  t: number;
  gain: number;
}

// The burst's gain over time, in seconds from its start: silent, a fast linear attack to the peak
// (scaled by the player's volume), held, then an exponential fade to silence at the end.
const switchSoundEnvelope = (
  burst: StaticBurst,
  volume: number,
): [start: EnvelopePoint, peak: EnvelopePoint, hold: EnvelopePoint, end: EnvelopePoint] => [
  { t: 0, gain: 0 },
  { t: burst.attackS, gain: burst.peak * volume },
  { t: burst.durationS * burst.holdFraction, gain: burst.peak * volume },
  { t: burst.durationS, gain: SILENCE },
];

interface SwitchSoundGate {
  /** The viewer's preference (default on). */
  enabled: boolean;
  /** The player's own mute and volume (0–1). */
  muted: boolean;
  volume: number;
  /** Whether the page has had a user gesture. A cold autoplay start has not, so it stays silent. */
  activated: boolean;
}

const shouldPlaySwitchSound = ({ enabled, muted, volume, activated }: SwitchSoundGate): boolean =>
  enabled && activated && !muted && volume > 0;

// A buffer of white noise for a filter to shape.
const noise = (ctx: BaseAudioContext, durationS: number) => {
  const length = Math.max(1, Math.ceil(ctx.sampleRate * durationS));
  const buffer = ctx.createBuffer(1, length, ctx.sampleRate);
  const data = buffer.getChannelData(0);
  for (let i = 0; i < length; i++) data[i] = Math.random() * 2 - 1;
  const source = ctx.createBufferSource();
  source.buffer = buffer;
  return source;
};

const playBurst = (ctx: BaseAudioContext, burst: StaticBurst, volume: number, at: number) => {
  const source = noise(ctx, burst.durationS);
  const filter = ctx.createBiquadFilter();
  filter.type = "bandpass";
  filter.Q.value = burst.q;
  filter.frequency.setValueAtTime(burst.sweepFromHz, at);
  filter.frequency.exponentialRampToValueAtTime(burst.sweepToHz, at + burst.durationS);
  const gain = ctx.createGain();
  const [start, peak, hold, end] = switchSoundEnvelope(burst, volume);
  gain.gain.setValueAtTime(start.gain, at + start.t);
  gain.gain.linearRampToValueAtTime(peak.gain, at + peak.t);
  gain.gain.setValueAtTime(hold.gain, at + hold.t);
  gain.gain.exponentialRampToValueAtTime(end.gain, at + end.t);
  source.connect(filter).connect(gain).connect(ctx.destination);
  source.start(at);
  source.stop(at + burst.durationS);
};

const playDial = (ctx: BaseAudioContext, dial: DialClick, volume: number, at: number) => {
  const osc = ctx.createOscillator();
  osc.type = "square";
  osc.frequency.setValueAtTime(dial.oscFromHz, at);
  osc.frequency.exponentialRampToValueAtTime(dial.oscToHz, at + dial.oscSweepS);
  const oscGain = ctx.createGain();
  oscGain.gain.setValueAtTime(dial.oscPeak * volume, at);
  oscGain.gain.exponentialRampToValueAtTime(SILENCE, at + dial.oscDecayS);
  osc.connect(oscGain).connect(ctx.destination);
  osc.start(at);
  osc.stop(at + dial.oscDecayS);

  const click = noise(ctx, dial.clickS);
  const highpass = ctx.createBiquadFilter();
  highpass.type = "highpass";
  highpass.frequency.setValueAtTime(dial.clickHighpassHz, at);
  const clickGain = ctx.createGain();
  clickGain.gain.setValueAtTime(dial.clickPeak * volume, at);
  clickGain.gain.exponentialRampToValueAtTime(SILENCE, at + dial.clickDecayS);
  click.connect(highpass).connect(clickGain).connect(ctx.destination);
  click.start(at);
  click.stop(at + dial.clickS);
};

// Schedules the sound on `ctx` now (or at `at`). Returns whether anything was scheduled. With the
// dial (B), the burst's own 12 ms attack lets the click's transient lead it, so both start together.
const playSwitchSound = (
  ctx: BaseAudioContext,
  gate: SwitchSoundGate & { variant?: SwitchSoundVariant; at?: number },
): boolean => {
  if (!shouldPlaySwitchSound(gate)) return false;
  const shape = SWITCH_SOUND_VARIANTS[gate.variant ?? DEFAULT_VARIANT];
  const at = gate.at ?? ctx.currentTime;
  if (shape.dial) playDial(ctx, shape.dial, gate.volume, at);
  playBurst(ctx, shape.burst, gate.volume, at);
  return true;
};

// One context for the page, made on first use: a browser caps how many a page may hold.
let pageContext: AudioContext | undefined;

// Plays the sound for a switch on this <video>, at its own volume and mute. A no-op where Web Audio
// is missing. `navigator.userActivation` is the browser's own record of a gesture; where it is not
// implemented, a context without one stays suspended, which is silent anyway.
const playChannelSwitchSound = (video: HTMLVideoElement, { enabled }: { enabled: boolean }): boolean => {
  const activated = navigator.userActivation?.hasBeenActive ?? true;
  const gate = { enabled, muted: video.muted, volume: video.volume, activated };
  if (!shouldPlaySwitchSound(gate) || typeof AudioContext === "undefined") return false;
  pageContext ??= new AudioContext();
  if (pageContext.state === "suspended") void pageContext.resume();
  return playSwitchSound(pageContext, gate);
};

export type { EnvelopePoint, SwitchSoundGate, SwitchSoundShape, SwitchSoundVariant };
export {
  playChannelSwitchSound,
  playSwitchSound,
  SWITCH_SOUND_VARIANTS,
  shouldPlaySwitchSound,
  switchSoundEnvelope,
};
