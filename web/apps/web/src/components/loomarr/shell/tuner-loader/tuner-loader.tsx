import { cn } from "@/lib";
import { TvStatic } from "../tv-static";
import type { TunerLoaderProps } from "./tuner-loader.type";

// TunerLoader — the "acquiring signal" state for a video surface (§9.1 Watch). A cold channel
// takes a beat to produce its first segment (the encoder spins up, more so when it TRANSCODES
// HEVC→h264 at ~realtime), and #187's fast live-edge sync means the player attaches before there
// is picture. Rather than show that beat as a dead black frame, we OWN it with a tuner warming up:
// a row of phosphor bars that jitter as grey snow and then LOCK to amber — static becoming signal,
// the whole palette's story (§1). Over the real CRT snow + scanlines of TvStatic, so it reads as
// continuous with the idle test-card surfaces elsewhere, not a generic spinner.
//
// Decorative motion, so the whole thing is aria-hidden; the accessible "loading" news is carried
// by the status TEXT the caller already renders (channel-watch's "Tuning in…") and by the label
// here. Every animation is `motion-safe:` — under reduced motion the bars sit at their locked
// amber height (no jitter), which is also the frame the visual suite pins, so baselines are stable.

// Nine bars — enough to read as a level meter, few enough to stay crisp at the small centered size.
// The lock STAGGERS across them (per-index delay) so the signal travels along the strip rather than
// snapping as one block — the same "live signal, not spinner" move ColorBars makes with its breathe.
const BARS = 9;
const LOCK_STAGGER_S = 0.09;

const TunerLoader = ({ label = "TUNING IN", className }: TunerLoaderProps) => (
  <div
    className={cn(
      "pointer-events-none absolute inset-0 flex flex-col items-center justify-center gap-4",
      className,
    )}
    aria-hidden
  >
    {/* The CRT ground: real fractal-noise snow + scanlines, gated motion-safe inside TvStatic. */}
    <TvStatic />

    {/* Phosphor bars. Fixed-height rail so the jittering bar heights have room; items-end so they
        grow from a common baseline like a level meter. `h-*` is animated by signal-lock, so the
        static (reduced-motion) height comes from the keyframe's locked frame. */}
    <div className="relative z-[1] flex h-14 items-end gap-1.5">
      {Array.from({ length: BARS }, (_, i) => (
        <span
          // Static, ordered strip — index key is correct and stable here.
          // biome-ignore lint/suspicious/noArrayIndexKey: fixed-length decorative strip, order is identity
          key={i}
          className="w-1.5 rounded-[1px] bg-signal-400 motion-safe:animate-signal-lock"
          // Per-index delay is a multiplication, not nine arbitrary classes — inline, like ColorBars.
          style={{ height: "64%", animationDelay: `${i * LOCK_STAGGER_S}s` }}
        />
      ))}
    </div>

    {/* The readout — mono, tracked-out, amber with a soft phosphor glow, matching the player's
        "CH n" chrome. The blinking cursor is the one extra sign of life. */}
    <p className="relative z-[1] flex items-center gap-0.5 font-mono text-[0.68rem] text-signal tracking-[0.22em]">
      <span style={{ textShadow: "0 0 8px var(--color-signal-tint-40)" }}>{label}</span>
      <span className="motion-safe:animate-[onair-pulse_1s_steps(2,end)_infinite]">_</span>
    </p>
  </div>
);

export { TunerLoader };
