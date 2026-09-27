// The channel-change sound (#1620): a dial set. Turning a mechanical tuner, the set CLUNKS, then
// plays raw static until the new station locks, and the static CUTS off — it never fades. Ported
// exactly from the maintainer's pick ("Dial set: clunk and raw static", the switch demo's
// switchSound('turret')). Synthesized with the Web Audio API, so there is no audio asset.
//
// Everything reaches the output through one master gain at the player's own volume; nothing plays
// when the player is muted, before the page has had a user gesture (a cold autoplay start), or when
// the viewer has turned the sound off (the Audio menu's "Channel-change sound").

/** The static's level (before the player's volume): near the programme's own loudness. */
const STATIC_PEAK = 0.13;
/** A tune slower than this ducks the static so a long wait doesn't roar. */
const DUCK_AFTER_S = 1.0;
const DUCK_BY_S = 1.25;
const DUCK_LEVEL = 0.04;
/** At lock the static is cut in this long: a click-free cut, not a fade. */
const CUT_S = 0.004;
// An exponential ramp cannot reach 0, so the clunk's decays land on this.
const SILENCE = 0.0001;

interface SwitchSoundGate {
  /** The viewer's preference (default on). */
  enabled: boolean;
  /** The player's own mute and volume (0–1). */
  muted: boolean;
  volume: number;
  /** Whether the page has had a user gesture. A cold autoplay start has not, so it stays silent. */
  activated: boolean;
}

interface SwitchSound {
  /** The new channel's first frame has decoded: cut the static. Once; later calls do nothing. */
  lock: (at?: number) => void;
}

const shouldPlaySwitchSound = ({ enabled, muted, volume, activated }: SwitchSoundGate): boolean =>
  enabled && activated && !muted && volume > 0;

const noiseBuffer = (ctx: BaseAudioContext, seconds: number) => {
  const buffer = ctx.createBuffer(1, Math.ceil(ctx.sampleRate * seconds), ctx.sampleRate);
  const data = buffer.getChannelData(0);
  for (let i = 0; i < data.length; i++) data[i] = Math.random() * 2 - 1;
  return buffer;
};

// A small TV speaker: no deep bass, no air, a boxy mid bump, and a little overdrive.
const speaker = (ctx: BaseAudioContext, out: AudioNode): AudioNode => {
  const highpass = ctx.createBiquadFilter();
  highpass.type = "highpass";
  highpass.frequency.value = 170;
  highpass.Q.value = 0.7;
  const peaking = ctx.createBiquadFilter();
  peaking.type = "peaking";
  peaking.frequency.value = 2400;
  peaking.gain.value = 5;
  peaking.Q.value = 1.1;
  const lowpass = ctx.createBiquadFilter();
  lowpass.type = "lowpass";
  lowpass.frequency.value = 6200;
  const drive = ctx.createWaveShaper();
  const curve = new Float32Array(1024);
  for (let i = 0; i < curve.length; i++) curve[i] = Math.tanh(1.6 * (i / 511.5 - 1)) / Math.tanh(1.6);
  drive.curve = curve;
  highpass.connect(peaking).connect(lowpass).connect(drive).connect(out);
  return highpass;
};

// A burst of high-passed noise, snapping on and decaying at once.
const tick = (
  ctx: BaseAudioContext,
  out: AudioNode,
  at: number,
  level: number,
  highpassHz: number,
  seconds: number,
) => {
  const source = ctx.createBufferSource();
  source.buffer = noiseBuffer(ctx, seconds);
  const highpass = ctx.createBiquadFilter();
  highpass.type = "highpass";
  highpass.frequency.value = highpassHz;
  const gain = ctx.createGain();
  gain.gain.setValueAtTime(level, at);
  gain.gain.exponentialRampToValueAtTime(SILENCE, at + seconds);
  source.connect(highpass).connect(gain).connect(out);
  source.start(at);
};

// A falling sine: the tuner's mechanical thump.
const thump = (
  ctx: BaseAudioContext,
  out: AudioNode,
  at: number,
  fromHz: number,
  toHz: number,
  level: number,
  seconds: number,
) => {
  const osc = ctx.createOscillator();
  osc.type = "sine";
  osc.frequency.setValueAtTime(fromHz, at);
  osc.frequency.exponentialRampToValueAtTime(toHz, at + seconds);
  const gain = ctx.createGain();
  gain.gain.setValueAtTime(level, at);
  gain.gain.exponentialRampToValueAtTime(SILENCE, at + seconds);
  osc.connect(gain).connect(out);
  osc.start(at);
  osc.stop(at + seconds + 0.02);
};

// Starts the sound: the clunk, then static that runs until `lock()`. Undefined when the gate says
// silent (nothing is built).
const startSwitchSound = (ctx: BaseAudioContext, gate: SwitchSoundGate): SwitchSound | undefined => {
  if (!shouldPlaySwitchSound(gate)) return undefined;
  const t = ctx.currentTime + 0.005;
  const master = ctx.createGain();
  master.gain.value = gate.volume;
  master.connect(ctx.destination);

  // The clunk: felt more than heard.
  thump(ctx, master, t, 95, 42, 0.32, 0.07);
  tick(ctx, master, t, 0.32, 1500, 0.018);
  tick(ctx, master, t + 0.012, 0.14, 800, 0.03);

  // Raw static until lock, with the mains buzz a set's audio stage picks up.
  const noise = ctx.createBufferSource();
  noise.buffer = noiseBuffer(ctx, 2);
  noise.loop = true;
  const level = ctx.createGain();
  level.gain.setValueAtTime(0, t + 0.02);
  level.gain.linearRampToValueAtTime(STATIC_PEAK, t + 0.025);
  level.gain.setValueAtTime(STATIC_PEAK, t + DUCK_AFTER_S);
  level.gain.linearRampToValueAtTime(DUCK_LEVEL, t + DUCK_BY_S);
  const buzz = ctx.createOscillator();
  buzz.type = "square";
  buzz.frequency.value = 59.94;
  const buzzLevel = ctx.createGain();
  buzzLevel.gain.value = 0.045;
  buzz.connect(buzzLevel).connect(level);
  noise.connect(level).connect(speaker(ctx, master));
  noise.start(t + 0.02);
  buzz.start(t + 0.02);

  let locked = false;
  return {
    lock: (at = ctx.currentTime) => {
      if (locked) return;
      locked = true;
      level.gain.cancelScheduledValues(at);
      level.gain.setValueAtTime(level.gain.value, at);
      level.gain.linearRampToValueAtTime(0, at + CUT_S);
      noise.stop(at + 0.01);
      buzz.stop(at + 0.01);
    },
  };
};

// One context for the page, made on first use: a browser caps how many a page may hold.
let pageContext: AudioContext | undefined;

// Starts the sound for a switch on this <video>, at its own volume and mute. Undefined (silent) where
// Web Audio is missing or the gate says so. `navigator.userActivation` is the browser's own record of
// a gesture; where it is not implemented, a context without one stays suspended, which is silent.
const startChannelSwitchSound = (
  video: HTMLVideoElement,
  { enabled }: { enabled: boolean },
): SwitchSound | undefined => {
  const activated = navigator.userActivation?.hasBeenActive ?? true;
  const gate = { enabled, muted: video.muted, volume: video.volume, activated };
  if (!shouldPlaySwitchSound(gate) || typeof AudioContext === "undefined") return undefined;
  pageContext ??= new AudioContext();
  if (pageContext.state === "suspended") void pageContext.resume();
  return startSwitchSound(pageContext, gate);
};

export type { SwitchSound, SwitchSoundGate };
export { STATIC_PEAK, shouldPlaySwitchSound, startChannelSwitchSound, startSwitchSound };
