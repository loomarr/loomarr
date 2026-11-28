import {
  idleTune,
  type TuneView,
  tuneAbandon,
  tuneFlip,
  tunePictureReady,
  tuneView,
} from "@loomarr/core/tune";
import { Action } from "@loomarr/design-system";
import { TuneBars, TuneStatic } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";

// The Cross's tune (frontend-design §9.4): dim static over the picture, seven bars along the
// crossing. Every state sits over a white frame, where the snow is at its brightest, and a dark one.

const FRAMES = [
  { name: "Bright frame", picture: "#FFFFFF" },
  { name: "Dark frame", picture: "linear-gradient(135deg, #10141C, #2A1B2E)" },
] as const;

// The snow holds still in the single states, as a screenshot baseline needs.
type PlateProps = Pick<TuneView, "bars" | "cover" | "run"> & { holdSnow?: boolean; reducedMotion?: boolean };

const Plate = ({ bars, cover, holdSnow = true, reducedMotion = false, run }: PlateProps) => (
  <div style={{ display: "flex", flexWrap: "wrap", gap: 16 }}>
    {FRAMES.map((frame) => (
      <figure key={frame.name} style={{ margin: 0, width: 360 }}>
        <div style={{ background: frame.picture, height: 202, position: "relative" }}>
          <TuneStatic cover={cover} holdSnow={holdSnow} reducedMotion={reducedMotion} />
        </div>
        <div style={{ height: 3, marginTop: 8 }}>
          <TuneBars bars={bars} reducedMotion={reducedMotion} run={run} />
        </div>
        <figcaption style={{ fontSize: 13, marginTop: 8 }}>{frame.name}</figcaption>
      </figure>
    ))}
  </div>
);

const meta = {
  title: "Watching/Tune",
  component: Plate,
  parameters: { layout: "padded" },
  args: { bars: "none", cover: "none", run: 1 },
} satisfies Meta<typeof Plate>;

type Story = StoryObj<typeof meta>;

/** The burst of a lone flip. The bars run once and are gone. */
const ChangingChannel: Story = { args: { bars: "run", cover: "burst" } };

/** The first tune or a spinning-up channel, for as long as the wait lasts. */
const TuningIn: Story = { args: { bars: "waiting", cover: "cold" } };

/** The picture has arrived: it is clear and the bars have locked to colour. */
const PictureArrived: Story = { args: { bars: "locked", cover: "none" } };

/** Flips still coming: one low wash. */
const FastFlipping: Story = { args: { bars: "none", cover: "wash" } };

const ReducedMotionChanging: Story = { args: { bars: "run", cover: "burst", reducedMotion: true } };

const ReducedMotionTuningIn: Story = { args: { bars: "waiting", cover: "cold", reducedMotion: true } };

// The part driven by the shared timing model, the way a player drives it.
const Driven = () => {
  const [state, setState] = useState(idleTune);
  const [now, setNow] = useState(0);
  const view = tuneView(state, now);

  const { nextAt } = view;
  useEffect(() => {
    if (nextAt === null) return undefined;
    // A timer may fire a millisecond early; reading before `nextAt` would name it again and stop here.
    const timer = window.setTimeout(
      () => setNow(Math.max(performance.now(), nextAt)),
      nextAt - performance.now(),
    );
    return () => window.clearTimeout(timer);
  }, [nextAt]);

  const flip = (streaming: boolean) => {
    const at = performance.now();
    setState((current) => tuneFlip(current, { at, streaming }));
    setNow(at);
  };
  const ready = () => {
    const at = performance.now();
    setState((current) => tunePictureReady(current, view.flip, at));
    setNow(at);
  };
  const fail = () => setState((current) => tuneAbandon(current, view.flip));

  return (
    <div style={{ display: "grid", gap: 16 }}>
      <div style={{ display: "flex", gap: 8 }}>
        <Action onPress={() => flip(true)}>Change channel</Action>
        <Action onPress={() => flip(false)}>Tune in</Action>
        <Action onPress={ready}>Picture arrives</Action>
        <Action onPress={fail}>Playback fails</Action>
      </div>
      <Plate bars={view.bars} cover={view.cover} holdSnow={false} run={view.run} />
    </div>
  );
};

/** Press quickly to see a run of flips hold as one wash. */
const TryIt: Story = { render: () => <Driven /> };

/**
 * A flip as the viewer's own motion setting draws it, with the snow moving: static and running bars,
 * or under reduced motion a crossfade with no static and no running bars.
 */
const FollowsTheViewer: Story = {
  render: () => (
    <div style={{ background: "#FFFFFF", height: 202, position: "relative", width: 360 }}>
      <TuneStatic cover="burst" />
      <div style={{ bottom: 0, height: 3, left: 0, position: "absolute", right: 0 }}>
        <TuneBars bars="run" run={1} />
      </div>
    </div>
  ),
};

export default meta;
export {
  ChangingChannel,
  FastFlipping,
  FollowsTheViewer,
  PictureArrived,
  ReducedMotionChanging,
  ReducedMotionTuningIn,
  TryIt,
  TuningIn,
};
