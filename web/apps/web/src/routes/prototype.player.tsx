// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// /prototype/player — the video player's on-screen chrome, flat. A: today's structure, flattened.
// B: A plus a tuner (channel down / up). C: a television's on-screen display (the design review's
// structure). D: C's structure with the content and type of a shipped player. E: D polished (logo,
// artwork tint, motion, scrub preview), flat. F: E in Apple-style glass. G: Loomarr TV, live
// television as an object (channel readout, broadcast lower third, surfing, tuning). Switch with [ ] variant, , . state, D device, T tint.
import { ThemeScope } from "@loomarr/design-system";
import { createFileRoute } from "@tanstack/react-router";
import { DinerFrame } from "@/prototype-direction/player/art";
import { ClassicChrome } from "@/prototype-direction/player/chrome-classic";
import { OsdChrome } from "@/prototype-direction/player/chrome-osd";
import { PolishedChrome } from "@/prototype-direction/player/chrome-polished";
import { RichChrome } from "@/prototype-direction/player/chrome-rich";
import { TvChrome } from "@/prototype-direction/player/chrome-tv";
import { channel } from "@/prototype-direction/player/data";
import { PlayerStyles } from "@/prototype-direction/player/flat";
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

type PlayerSearch = Omit<ProtoSearch, "device"> & { device: PlayerDevice };

const validateSearch = (raw: Record<string, unknown>): PlayerSearch => ({
  ...parseSearch(VARIANTS, STATES)(raw),
  device: DEVICES.includes(raw.device as PlayerDevice) ? (raw.device as PlayerDevice) : "desktop",
});

const PlayerPrototype = () => {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const scenario = scenarioOf(search.state as (typeof STATES)[number]);
  const tv = search.variant === "G";
  const polished = search.variant === "E" || search.variant === "F" || tv;
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
          {polished ? <DinerFrame /> : <LivePicture />}
          {tv ? (
            <TvChrome {...props} />
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
