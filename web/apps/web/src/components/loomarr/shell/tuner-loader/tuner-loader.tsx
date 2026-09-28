import { cn } from "@/lib/utils";
import { TvStatic } from "../tv-static";
import type { TunerLoaderProps } from "./tuner-loader.type";

// TunerLoader — the "acquiring signal" state for a video surface (§9.1 Watch). A cold channel
// takes a beat to produce its first segment (the encoder spins up, more so when it TRANSCODES
// HEVC→h264 at ~realtime), and #187's fast live-edge sync means the player attaches before there
// is picture. Rather than show that beat as a dead black frame, we OWN it with a tuner warming up:
// a row of phosphor bars that jitter as grey snow and then LOCK to amber — static becoming signal,
// the whole palette's story (§1).
//
// The layout is the static wash (#1620, the maintainer's "B4"). Whatever picture sits under the
// loader, the previous channel's held frame or the tuned channel's still, is covered by a dark wash
// and the tuner's snow, so the readout always lands on static, never on a bright or busy picture.
// On a switch the wash DRAINS the held frame (grey, flat, fading) over the first ~450 ms; on a cold
// start there is nothing to drain and it rests at its drained look. Over it, a centred stack: the
// bars, the channel line (CH n in amber mono, the name in white), and "TUNING IN_". Each element
// carries the same faint static-950 halo so it reads on the brightest speckle, and there is
// deliberately no card, band or oval behind the text (the maintainer rejected those).
//
// Decorative motion, so the whole thing is aria-hidden; the accessible "loading" news is carried
// by the status TEXT the caller already renders (channel-watch's "Tuning in…" and the tuner OSD).
// Every animation is `motion-safe:` — under reduced motion the wash sits drained, the snow holds
// still and the bars sit at their locked amber height, which is also the frame the visual suite
// pins, so baselines are stable.

// Seven bars (the B4 mock). The lock STAGGERS across them (per-index delay) so the signal travels
// along the strip rather than snapping as one block — the same "live signal, not spinner" move
// ColorBars makes with its breathe.
const BARS = 7;
const LOCK_STAGGER_S = 0.09;

// The halo: the static-950 chrome tint, tight under the glyph and wide around it.
const shade = (percent: number) => `color-mix(in srgb, var(--color-static-950) ${percent}%, transparent)`;
const HALO = `0 1px 2px ${shade(90)}, 0 0 18px ${shade(85)}`;
const READOUT_HALO = `0 1px 2px ${shade(90)}, 0 0 14px ${shade(85)}, 0 0 8px color-mix(in srgb, var(--color-signal) 35%, transparent)`;
const BARS_HALO = `drop-shadow(0 0 10px ${shade(90)})`;

const TunerLoader = ({ label = "TUNING IN", channel, heldFrame = false, className }: TunerLoaderProps) => (
  <div className={cn("pointer-events-none absolute inset-0", className)} aria-hidden>
    {/* The wash over the picture beneath. Its resting style IS the drained frame of `held-drain`:
        the picture greyed, flattened and left at 12% under static-950. */}
    <div
      data-wash={heldFrame ? "draining" : "rest"}
      className={cn(
        "absolute inset-0 bg-static-950/88 backdrop-contrast-60 backdrop-grayscale",
        heldFrame && "motion-safe:animate-held-drain",
      )}
    />

    {/* The CRT ground: analog snow under a dark layer + scanlines; still (not absent) under reduced
        motion. On a switch it thickens in over the draining frame. */}
    <TvStatic variant="wash" className={cn(heldFrame && "motion-safe:animate-wash-thicken")} />

    <div className="relative z-[1] flex size-full flex-col items-center justify-center gap-3 px-4 text-center">
      {/* Phosphor bars. Fixed-height rail so the jittering bar heights have room; items-end so they
          grow from a common baseline like a level meter. `h-*` is animated by signal-lock, so the
          static (reduced-motion) height comes from the keyframe's locked frame. */}
      <div className="flex h-11 items-end gap-[5px]" style={{ filter: BARS_HALO }}>
        {Array.from({ length: BARS }, (_, i) => (
          <span
            // Static, ordered strip — index key is correct and stable here.
            // biome-ignore lint/suspicious/noArrayIndexKey: fixed-length decorative strip, order is identity
            key={i}
            className="w-1.5 rounded-[1px] bg-signal-400 motion-safe:animate-signal-lock"
            // Per-index delay is a multiplication, not seven arbitrary classes — inline, like ColorBars.
            style={{ height: "64%", animationDelay: `${i * LOCK_STAGGER_S}s` }}
          />
        ))}
      </div>

      {channel && (
        <p
          className="font-sans font-semibold text-[clamp(18px,2.8vw,28px)] text-static-0 tracking-[0.01em]"
          style={{ textShadow: HALO }}
        >
          <span className="mr-[0.7em] align-[0.18em] font-medium font-mono text-[0.62em] text-signal tracking-[0.16em]">
            CH {channel.number}
          </span>
          <span>{channel.name}</span>
        </p>
      )}

      {/* The readout — mono, tracked-out amber with the blinking cursor as the one extra sign of life. */}
      <p
        className="font-medium font-mono text-[clamp(11px,1.3vw,13px)] text-signal tracking-[0.24em]"
        style={{ textShadow: READOUT_HALO }}
      >
        <span>{label}</span>
        <span className="motion-safe:animate-[onair-pulse_1s_steps(2,end)_infinite]">_</span>
      </p>
    </div>
  </div>
);

export { TunerLoader };
