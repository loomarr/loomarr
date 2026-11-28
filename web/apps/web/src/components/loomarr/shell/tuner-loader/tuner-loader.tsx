import { AnalogSnow, Stack, Text } from "@loomarr/design-system";
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
// and the design system's AnalogSnow, so the readout always lands on static, never on a bright or busy picture.
// On a switch the wash DRAINS the held frame (grey, flat, fading) over the first ~450 ms; on a cold
// start there is nothing to drain and it rests at its drained look. Over it, a centred stack: the
// bars, the channel line (CH n in amber mono, the name in white), and "TUNING IN_". Each element
// carries the same faint static-950 halo so it reads on the brightest speckle, and there is
// deliberately no card, band or oval behind the text (the maintainer rejected those).
//
// Decorative motion, so the whole thing is aria-hidden; the accessible "loading" news is carried
// by the status TEXT the caller already renders (channel-watch's "Tuning in…" and the tuner OSD).
// Each animated part carries a `data-motion` hook that styles.css animates only where motion is
// allowed. Under reduced motion the wash sits drained, the snow holds still and the bars sit at
// their locked amber height, which is also the frame the visual suite pins, so baselines are stable.

// Seven bars (the B4 mock). The lock STAGGERS across them (per-index delay) so the signal travels
// along the strip rather than snapping as one block: a live signal, not a spinner.
const BARS = 7;
const LOCK_STAGGER_S = 0.09;

// The halo: the static-950 chrome tint, tight under the glyph and wide around it.
const shade = (percent: number) => `color-mix(in srgb, var(--color-static-950) ${percent}%, transparent)`;
const HALO = `0 1px 2px ${shade(90)}, 0 0 18px ${shade(85)}`;
const READOUT_HALO = `0 1px 2px ${shade(90)}, 0 0 14px ${shade(85)}, 0 0 8px color-mix(in srgb, var(--color-signal) 35%, transparent)`;
const BARS_HALO = `drop-shadow(0 0 10px ${shade(90)})`;

const FILL = { inset: 0, position: "absolute" } as const;
// The resting wash IS the drained frame of `held-drain`: the picture greyed, flattened and left at
// 12% under static-950.
const DRAINED = {
  ...FILL,
  backdropFilter: "grayscale(1) contrast(0.6)",
  backgroundColor: shade(88),
} as const;
// A fixed-height rail so the jittering bar heights have room, growing from a common baseline.
const RAIL = { alignItems: "flex-end", display: "flex", filter: BARS_HALO, gap: 5, height: 44 } as const;
// Resting height and colour are the locked frame of `signal-lock`.
const BAR = { background: "var(--color-signal-400)", borderRadius: 1, height: "64%", width: 6 } as const;

const TunerLoader = ({ label = "TUNING IN", channel, heldFrame = false, backdrop }: TunerLoaderProps) => (
  <Stack aria-hidden bottom={0} left={0} pointerEvents="none" position="absolute" right={0} top={0}>
    {/* The wash over the picture beneath. */}
    <div
      data-wash={heldFrame ? "draining" : "rest"}
      data-motion={heldFrame ? "held-drain" : undefined}
      style={DRAINED}
    />

    {/* The hold (a wait of 1.5 s or more): the programme's fanart, dimmed, contained at 16:9 under the
        snow. */}
    {backdrop ? (
      <img
        alt=""
        src={backdrop}
        style={{
          height: "100%",
          inset: 0,
          objectFit: "contain",
          opacity: 0.5,
          position: "absolute",
          width: "100%",
        }}
      />
    ) : null}

    {/* The CRT ground: analog snow under a dark layer + scanlines; still (not absent) under reduced
        motion. On a switch it thickens in over the draining frame. */}
    <div data-snow="wash" data-motion={heldFrame ? "wash-thicken" : undefined} style={FILL}>
      <AnalogSnow />
    </div>

    <Stack
      align="center"
      gap="control"
      height="100%"
      justify="center"
      paddingHorizontal={16}
      position="relative"
      style={{ textAlign: "center" }}
      width="100%"
      zIndex={1}
    >
      {/* Phosphor bars, like a level meter. */}
      <div style={RAIL}>
        {Array.from({ length: BARS }, (_, i) => (
          <span
            // Static, ordered strip — index key is correct and stable here.
            // biome-ignore lint/suspicious/noArrayIndexKey: fixed-length decorative strip, order is identity
            key={i}
            data-motion="signal-lock"
            style={{ ...BAR, animationDelay: `${i * LOCK_STAGGER_S}s` }}
          />
        ))}
      </div>

      {channel && (
        <Text as="p" style={{ color: "var(--color-static-0)", textShadow: HALO }} textRole="stationName">
          <Text
            as="span"
            style={{ marginRight: "0.7em", verticalAlign: "0.18em" }}
            textRole="stationTag"
            tone="signal"
          >
            CH {channel.number}
          </Text>
          <span>{channel.name}</span>
        </Text>
      )}

      {/* The readout — mono, tracked-out amber with the blinking cursor as the one extra sign of life. */}
      <Text as="p" style={{ textShadow: READOUT_HALO }} textRole="metadata" tone="signal" tracking={0.24}>
        <span>{label}</span>
        <span data-motion="cursor">_</span>
      </Text>
    </Stack>
  </Stack>
);

export { TunerLoader };
