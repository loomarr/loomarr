// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// /prototype/player — the video player's on-screen chrome, flat. A: today's structure, flattened.
// B: A plus a tuner (channel down / up). C: a television's on-screen display (the design review's
// structure). D: C's structure with the content and type of a shipped player. E: D polished (logo,
// artwork tint, motion, scrub preview), flat. F: E in Apple-style glass. G: Loomarr TV, live
// television as an object (channel readout, broadcast lower third, surfing, tuning). H: G with the
// channel change rebuilt as one choreographed tune (states "tune", "slow-tune"; ?t=ms freezes it,
// ?loop=1 replays it) and a restrained remote. I: H's channel identity with the preview timeline
// back as the lower third, icon-only keys, amber on the number tab alone (state "chrome-top" holds a
// preview card open). Switch with [ ] variant, , . state, D device, T tint.
import { ThemeScope } from "@loomarr/design-system";
import { createFileRoute } from "@tanstack/react-router";
import { BrightFrame, DinerFrame } from "@/prototype-direction/player/art";
import { ClassicChrome } from "@/prototype-direction/player/chrome-classic";
import { LineChrome } from "@/prototype-direction/player/chrome-line";
import { OsdChrome } from "@/prototype-direction/player/chrome-osd";
import { PolishedChrome } from "@/prototype-direction/player/chrome-polished";
import { RichChrome } from "@/prototype-direction/player/chrome-rich";
import { TuneChrome } from "@/prototype-direction/player/chrome-tune";
import { TvChrome } from "@/prototype-direction/player/chrome-tv";
import { channel } from "@/prototype-direction/player/data";
import { PlayerStyles } from "@/prototype-direction/player/flat";
import { pictureSrc, RealFrame } from "@/prototype-direction/player/real-art";
import {
  DEVICES,
  type PlayerDevice,
  STATES,
  scenarioOf,
  VARIANTS,
} from "@/prototype-direction/player/scenario";
import {
  eraTint,
  LivePicture,
  ProtoFrame,
  type ProtoSearch,
  ProtoSwitcher,
  parseSearch,
} from "@/prototype-direction/shared";

// H only: `t` freezes the tune at that many ms into the sequence (for stills), `loop=1` replays it.
// `frame=bright` swaps the diner for a bright daytime beach, to test legibility over light pictures;
// `frame=drawn` keeps the drawn diner even when the gitignored real art is present.
// `bug=` is the channel watermark's corner (top-right default, top-left, bottom-right, bottom-left, off).
const BUG_CORNERS = ["top-right", "top-left", "bottom-right", "bottom-left", "off"] as const;
type PlayerSearch = Omit<ProtoSearch, "device"> & {
  bug?: (typeof BUG_CORNERS)[number];
  device: PlayerDevice;
  frame?: "bright" | "drawn";
  loop?: boolean;
  /** J only: `long` puts long names everywhere; `bare` is a library with no art at all. */
  stress?: "bare" | "long";
  t?: number;
};

const validateSearch = (raw: Record<string, unknown>): PlayerSearch => {
  const t = Number(raw.t);
  return {
    ...parseSearch(VARIANTS, STATES)(raw),
    device: DEVICES.includes(raw.device as PlayerDevice) ? (raw.device as PlayerDevice) : "desktop",
    ...(raw.t !== undefined && Number.isFinite(t) ? { t } : {}),
    ...(String(raw.loop) === "1" || raw.loop === true ? { loop: true } : {}),
    ...(raw.frame === "bright" || raw.frame === "drawn" ? { frame: raw.frame } : {}),
    ...(raw.stress === "long" || raw.stress === "bare" ? { stress: raw.stress } : {}),
    ...(BUG_CORNERS.includes(raw.bug as PlayerSearch["bug"] & string)
      ? { bug: raw.bug as (typeof BUG_CORNERS)[number] }
      : {}),
  };
};

const PlayerPrototype = () => {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const scenario = scenarioOf(search.state as (typeof STATES)[number]);
  // J is G with the maintainer's four changes (2026-11-09): the readout only on a channel change, a
  // white progress line, a quiet row of utility icons, and the preview timeline opening from the
  // band's progress line on desktop (state "chrome-top" holds it open with a preview card).
  const refined = search.variant === "J";
  const tv = search.variant === "G" || refined;
  const tune = search.variant === "H";
  const line = search.variant === "I";
  const polished = search.variant === "E" || search.variant === "F" || tv || tune || line;
  const props = {
    device: search.device,
    scenario,
    tint: eraTint(channel.era, search.tint),
    tintFor: (era: string) => eraTint(era, search.tint),
  };
  return (
    <>
      <ProtoSwitcher
        devices={DEVICES}
        onChange={(next) =>
          navigate({ search: (prev) => ({ ...prev, ...next }) as typeof prev, replace: true })
        }
        search={search}
        states={STATES}
        variants={VARIANTS}
      />
      <ProtoFrame device={search.device}>
        <ThemeScope theme="dark">
          <PlayerStyles />
          {polished ? (
            search.frame === "bright" ? (
              <BrightFrame />
            ) : pictureSrc && search.frame !== "drawn" ? (
              <RealFrame />
            ) : (
              <DinerFrame />
            )
          ) : (
            <LivePicture />
          )}
          {line ? (
            <LineChrome {...props} />
          ) : tune ? (
            <TuneChrome {...props} bug={search.bug} freezeAt={search.t} loop={search.loop} />
          ) : tv ? (
            <TvChrome
              {...props}
              bug={search.bug}
              freezeAt={search.t}
              loop={search.loop}
              refined={refined}
              stress={search.stress}
            />
          ) : polished ? (
            <PolishedChrome {...props} material={search.variant === "F" ? "glass" : "flat"} />
          ) : search.variant === "D" ? (
            <RichChrome {...props} />
          ) : search.variant === "C" ? (
            <OsdChrome {...props} />
          ) : (
            <ClassicChrome {...props} tuner={search.variant === "B"} />
          )}
        </ThemeScope>
      </ProtoFrame>
    </>
  );
};

const Route = createFileRoute("/prototype/player")({
  component: PlayerPrototype,
  validateSearch,
});

export { Route };
