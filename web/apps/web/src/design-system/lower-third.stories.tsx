import { Surface } from "@loomarr/design-system";
import {
  FirstRunHint,
  LowerThird,
  type LowerThirdProps,
  type LowerThirdState,
  QuietRow,
  type QuietRowDevice,
  WatchScrim,
} from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { expect, userEvent, within } from "storybook/test";

// The lower third, quiet row and first-run hint of The Cross (frontend-design §9.2 and §9.3, target).
// Genre words only: the repo is public, so no real titles. The pictures are CSS stand-ins.

const SIZE: Record<QuietRowDevice, { height: number; width: number }> = {
  full: { height: 520, width: 1100 },
  landscape: { height: 390, width: 844 },
  portrait: { height: 300, width: 390 },
  tv: { height: 540, width: 960 },
  window: { height: 300, width: 700 },
};

const DARK_PICTURE = "linear-gradient(120deg, #1d2b3a, #0b0c0e)";
const BRIGHT_PICTURE = "linear-gradient(120deg, #ffffff, #f2ead8)";

const overPicture = (device: QuietRowDevice) => device !== "portrait" && device !== "window";

// Over the picture the parts sit on a frame; beside it, on the page's own ground.
const Frame = ({
  bright,
  children,
  device,
}: {
  bright?: boolean;
  children: ReactNode;
  device: QuietRowDevice;
}) =>
  overPicture(device) ? (
    // Over the picture the parts stand on the watching screen's one foot scrim, as it draws them.
    <div
      style={{
        backgroundImage: bright ? BRIGHT_PICTURE : DARK_PICTURE,
        display: "flex",
        flexDirection: "column",
        gap: 16,
        justifyContent: "flex-end",
        padding: 16,
        position: "relative",
        ...SIZE[device],
      }}
    >
      <WatchScrim edge="foot" />
      {/* Positioned after the scrim, so the words stand on it, as the screen draws them. */}
      <div style={{ display: "flex", flexDirection: "column", gap: 16, position: "relative" }}>{children}</div>
    </div>
  ) : (
    <Surface backgroundColor="surfaceCanvas" borderWidth={0} gap={16} paddingVertical={16} {...SIZE[device]}>
      {children}
    </Surface>
  );

const on = {
  episode: "S2 E4",
  kind: "on",
  minutesLeft: 12,
  next: { startsAt: "10:30", title: "The night desk" },
  rating: "TV-PG",
  title: "The diner years",
} as const satisfies LowerThirdState;

type Args = LowerThirdProps & { bright?: boolean; hint?: boolean };

const meta = {
  title: "Loomarr Components/Lower Third",
  component: LowerThird,
  args: { channelName: "Coast 90s", channelNumber: 7, decade: 1990, device: "full", state: on },
  render: ({ bright, hint, ...args }: Args) => (
    <Frame bright={bright ?? false} device={args.device}>
      {hint ? <FirstRunHint device={args.device} /> : null}
      <LowerThird {...args} ownInk={false} />
      <QuietRow
        behindLive={args.state.kind === "on" && Boolean(args.state.behindSeconds)}
        device={args.device}
        onPress={() => undefined}
        ownInk={false}
      />
    </Frame>
  ),
  parameters: { layout: "fullscreen" },
} satisfies Meta<Args>;

type Story = StoryObj<typeof meta>;

const light = { theme: "light" } as const;

/** A touch control is at least 44 px and no text is under 12 px. */
const measures =
  (touch: boolean): NonNullable<Story["play"]> =>
  async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    if (touch) {
      for (const control of canvas.getAllByRole("button")) {
        const { height, width } = control.getBoundingClientRect();
        await expect(Math.min(height, width)).toBeGreaterThanOrEqual(44);
      }
    }
    for (const element of canvasElement.querySelectorAll("*")) {
      const words = [...element.childNodes].some((n) => n.nodeType === 3 && n.textContent?.trim());
      if (words)
        await expect(Number.parseFloat(getComputedStyle(element).fontSize)).toBeGreaterThanOrEqual(12);
    }
  };

const Settled: Story = { args: { hint: true }, play: measures(false) };
const SettledLight: Story = { ...Settled, globals: light };
const NoDecade: Story = { args: { decade: undefined } };
// Over a washed-out frame the words stand on the foot scrim: no plate, box or hard edge around them.
const Bright: Story = {
  args: { bright: true },
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelector('[data-testid^="picture-ink"]')).toBeNull();
    await expect(within(canvasElement).getByTestId("watch-scrim-foot")).toBeVisible();
  },
};

const TuningIn: Story = { args: { state: { kind: "tuning" } } };
const InABreak: Story = {
  args: { state: { backInMinutes: 3, kind: "break", next: on.next, title: on.title } },
};
const Paused: Story = { args: { state: { ...on, paused: true } } };
const BehindLive: Story = { args: { state: { ...on, behindSeconds: 40 } } };
const NothingScheduled: Story = { args: { state: { kind: "nothing" } } };
const Typing: Story = {
  args: { state: { digits: "12", kind: "typing", target: { name: "Movie matinee", onNow: "A western" } } },
};
const NoSuchChannel: Story = { args: { state: { digits: "99", kind: "typing" } } };

const Window: Story = { args: { device: "window", hint: true } };
const WindowLight: Story = { ...Window, globals: light };
const WindowBehindLive: Story = { args: { device: "window", state: { ...on, behindSeconds: 150 } } };
const WindowTypingLight: Story = { args: { ...Typing.args, device: "window" }, globals: light };
const WindowBreakLight: Story = { args: { ...InABreak.args, device: "window" }, globals: light };

const Television: Story = { args: { device: "tv", hint: true }, play: measures(false) };
const TelevisionBright: Story = { args: { bright: true, device: "tv" } };
const TelevisionBehindLive: Story = { args: { device: "tv", state: { ...on, behindSeconds: 40 } } };
const TelevisionLight: Story = { args: { device: "tv" }, globals: light };

const Landscape: Story = { args: { device: "landscape" }, play: measures(true) };
const LandscapeBright: Story = { args: { bright: true, device: "landscape" } };
const LandscapeBehindLive: Story = {
  args: { device: "landscape", state: { ...on, behindSeconds: 40 } },
  play: measures(true),
};

const Portrait: Story = { args: { device: "portrait", hint: true }, play: measures(true) };
const PortraitLight: Story = { ...Portrait, globals: light };
const PortraitBehindLive: Story = {
  args: { device: "portrait", state: { ...on, behindSeconds: 40 } },
  play: measures(true),
};
const PortraitNoDecadeLight: Story = { args: { decade: undefined, device: "portrait" }, globals: light };

// The keyboard draws the one amber ring; a pointer draws the plain one.
const KeyboardFocus: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await userEvent.keyboard("{ArrowRight}{ArrowRight}");
    await expect(within(canvasElement).getAllByTestId("ring-actionFocus")).toHaveLength(1);
  },
};
const TelevisionFocus: Story = { ...KeyboardFocus, args: { device: "tv" } };
const Pointed: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.hover(canvas.getByRole("button", { name: "Guide" }));
    await expect(canvas.getByTestId("ring-contentPrimary")).toBeVisible();
    await expect(canvas.queryByTestId("ring-actionFocus")).toBeNull();
  },
};

export {
  BehindLive,
  Bright,
  InABreak,
  KeyboardFocus,
  Landscape,
  LandscapeBehindLive,
  LandscapeBright,
  NoDecade,
  NoSuchChannel,
  NothingScheduled,
  Paused,
  Pointed,
  Portrait,
  PortraitBehindLive,
  PortraitLight,
  PortraitNoDecadeLight,
  Settled,
  SettledLight,
  Television,
  TelevisionBehindLive,
  TelevisionBright,
  TelevisionFocus,
  TelevisionLight,
  TuningIn,
  Typing,
  Window,
  WindowBehindLive,
  WindowBreakLight,
  WindowLight,
  WindowTypingLight,
};
export default meta;
