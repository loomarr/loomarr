import type { LineupChannel } from "@loomarr/core/channel-lineup";
import { idleTune, type TuneView, tuneFlip, tuneView } from "@loomarr/core/tune";
import { type WatchRequest, watchKeyOf } from "@loomarr/core/watch-screen";
import { Surface, Text } from "@loomarr/design-system";
import { type PlayerWatermark, useWatchScreen, WatchScreen, type WatchScreenInput } from "@loomarr/ui";

type WatermarkCorner = PlayerWatermark["corner"];

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useEffect, useRef } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";

// The watching screen of The Cross (frontend-design §9.1, target), at full screen. Genre words only:
// the repo is public, so no real titles. The pictures are CSS stand-ins, and so are the menu and the
// cover in their slots: the screen places the shipped ones.

type Channel = LineupChannel & { title: string };

const NAMES = [
  "Morning Paper",
  "Coast 90s",
  "Game Night",
  "Westerns",
  "Late Desk",
  "Cartoon Hour",
  "Coast 80s",
  "Cop Shows",
  "Matinee",
  "Sitcom Street",
  "Space Age",
  "Quiz Club",
];
const GENRES = ["A sitcom", "A western", "A cop show", "A quiz", "A cartoon", "A matinee"];
const CHANNELS: Channel[] = NAMES.map((name, i) => ({
  decade: i % 2 ? 1990 : 1980,
  id: `ch-${i + 1}`,
  name,
  number: i + 1,
  onNow: GENRES[i % GENRES.length],
  playable: i !== 4,
  title: `${GENRES[i % GENRES.length]} at ${i + 7}`,
}));
const LINEUP = { channels: CHANNELS, favouriteIds: ["ch-2", "ch-7"], recentIds: ["ch-7", "ch-2"] };

const DARK_PICTURE = "linear-gradient(120deg, #2a3f55, #0b0c0e 70%)";
const BRIGHT_PICTURE = "linear-gradient(120deg, #ffffff, #f2ead8)";

const IDLE = tuneView(idleTune, 0);
const TUNING_IN: TuneView = { bars: "waiting", cover: "cold", flip: 1, nextAt: null, run: 0 };
const CHANGING = tuneView(tuneFlip(idleTune, { at: 0, streaming: true }), 10);

/** A clock that moves only when a story moves it, so a story's state holds still for its picture. */
type Clock = { t: number };
type Setup = (send: (event: WatchScreenInput) => void, clock: Clock) => void;

interface Args {
  bright?: boolean;
  cover?: boolean;
  onRequest: (request: WatchRequest) => void;
  /** What happened before the picture is taken. Unless a story rests, the pointer holds it awake. */
  setup?: Setup;
  rest?: boolean;
  tune?: TuneView;
  /** The channel's logo, burned into a corner of the picture, with the back control drawn. */
  logo?: WatermarkCorner;
  /** The back control is drawn, with no logo. */
  back?: boolean;
}

// The logo as playout burns it: 12% of the picture's height, inset 5%, half as wide again as tall.
const LOGO = { margin: 0.05, size: 0.12 } as const;
const Logo = ({ corner }: { corner: WatermarkCorner }) => {
  const inset = LOGO.margin * 720;
  const height = LOGO.size * 720;
  return (
    <div
      data-testid="logo"
      style={{
        background: "rgba(200, 40, 40, 0.85)",
        height,
        position: "absolute",
        width: height * 1.5,
        [corner.startsWith("top") ? "top" : "bottom"]: inset,
        [corner.endsWith("left") ? "left" : "right"]: inset,
      }}
    />
  );
};

const Menu = ({ name }: { name: string }) => (
  <Surface backgroundColor="surfaceRaised" borderRadius={12} borderWidth={0} gap={8} padding={16} width={260}>
    <Text textRole="cardLabel">{name === "subtitles" ? "Subtitles" : name}</Text>
    <Text textRole="compact" tone="secondary">
      The shipped menu stands here.
    </Text>
  </Surface>
);

const Cover = () => (
  <Surface
    alignItems="center"
    backgroundColor="surfaceCanvas"
    borderWidth={0}
    flex={1}
    justifyContent="center"
  >
    <Text textRole="cardLabel">Off air</Text>
  </Surface>
);

const Screen = ({
  back: withBack = false,
  bright = false,
  cover = false,
  logo,
  onRequest,
  rest = false,
  setup,
  tune = IDLE,
}: Args) => {
  const clock = useRef<Clock>({ t: 0 }).current;
  const model = useWatchScreen({
    hintSeen: false,
    layout: "full",
    lineup: LINEUP,
    now: () => clock.t,
    onRequest,
    watchedId: "ch-7",
  });
  const { send } = model;
  useEffect(() => {
    if (!rest) send({ held: true, region: "quietRow", type: "pointerHold" });
    setup?.(send, clock);
    if (rest) {
      clock.t += 6000;
      send({ type: "tick" });
    }
  }, [clock, rest, send, setup]);
  // The web adapter's part: the document's keys reach the screen unless something took them.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const key = watchKeyOf({
        altKey: event.altKey,
        composing: event.isComposing,
        ctrlKey: event.ctrlKey,
        defaultPrevented: event.defaultPrevented,
        inField: target?.tagName === "INPUT",
        key: event.key,
        metaKey: event.metaKey,
      });
      if (key !== undefined) send({ key, type: "key" });
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [send]);
  const picture: ReactNode = (
    <div
      style={{
        backgroundImage: bright ? BRIGHT_PICTURE : DARK_PICTURE,
        height: "100%",
        position: "relative",
        width: "100%",
      }}
    >
      {logo ? <Logo corner={logo} /> : null}
    </div>
  );
  return (
    <div data-testid="frame" style={{ height: 720, overflow: "hidden", position: "relative", width: 1280 }}>
      <WatchScreen
        {...(logo || withBack ? { back: { label: "Guide", onPress: () => undefined } } : {})}
        {...(logo ? { watermark: { ...LOGO, corner: logo } } : {})}
        cover={cover ? <Cover /> : undefined}
        detail={(channel) => ({ minutesLeft: 8 + channel.number })}
        holdSnow
        layout="full"
        model={model}
        picture={picture}
        programme={(channel) => ({
          episode: "S2 E4",
          kind: "on",
          minutesLeft: 12,
          next: { startsAt: "10:30", title: "The night desk" },
          rating: "TV-PG",
          title: CHANNELS.find((each) => each.id === channel.id)?.title ?? channel.name,
        })}
        renderMenu={({ name }) => <Menu name={name} />}
        tune={tune}
      />
    </div>
  );
};

const meta = {
  title: "Loomarr Components/Watch Screen",
  component: Screen,
  args: { onRequest: fn() },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Screen>;

type Story = StoryObj<typeof meta>;

const tunes = (onRequest: Args["onRequest"]) =>
  (onRequest as ReturnType<typeof fn>).mock.calls.filter(([request]) => request.type === "tune");

const AtRest: Story = {
  args: { rest: true },
  play: async ({ canvasElement }) => {
    await waitFor(() => expect(within(canvasElement).queryByRole("note")).toBeNull());
  },
};

// The key that wakes the screen also does its job: Down flips at once and opens no list.
const Wake: Story = {
  args: { rest: true },
  play: async ({ args, canvasElement }) => {
    await userEvent.keyboard("{ArrowDown}");
    const canvas = within(canvasElement);
    await expect(canvas.getByTestId("lower-third-title")).toHaveTextContent(CHANNELS[7]?.title ?? "");
    await expect(canvas.queryByRole("listbox")).toBeNull();
    await expect(tunes(args.onRequest)).toEqual([[{ channelId: "ch-8", type: "tune" }]]);
  },
};

const Settled: Story = {};

// Channels opens the list; it rests the watched channel on the crossing and tunes nothing.
const ListOpen: Story = {
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Channels" }));
    const pinned = await canvas.findByTestId("channel-list-pinned");
    await waitFor(() => {
      const foot = canvas.getByTestId("frame").getBoundingClientRect().bottom;
      return expect(Math.abs(foot - pinned.getBoundingClientRect().bottom - 148)).toBeLessThan(6);
    });
    await expect(tunes(args.onRequest)).toEqual([]);
  },
};

const ListCursorOnRow: Story = {
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Channels" }));
    await userEvent.keyboard("{ArrowDown}{ArrowDown}");
    await expect(canvas.getAllByTestId("ring-actionFocus")).toHaveLength(1);
    await expect(tunes(args.onRequest)).toEqual([]);
  },
};

const ListCursorOnTab: Story = {
  args: {
    setup: (send) => {
      send({ control: "channels", type: "press" });
      send({ event: { type: "tabs" }, type: "list" });
    },
  },
};

const Find: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.keyboard("/");
    const canvas = within(canvasElement);
    await userEvent.type(await canvas.findByRole("searchbox"), "Coast");
    await expect(canvasElement).toHaveTextContent("2 Coast 90s, A western");
  },
};

const TypingNumber: Story = {
  args: {
    setup: (send) => {
      send({ key: "1", type: "key" });
      send({ key: "2", type: "key" });
    },
  },
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByTestId("lower-third-numeral")).toHaveTextContent("12");
  },
};

const NoSuchNumber: Story = {
  args: {
    setup: (send) => {
      send({ key: "9", type: "key" });
      send({ key: "9", type: "key" });
    },
  },
};

const Paused: Story = {
  args: { setup: (send) => send({ key: " ", type: "key" }) },
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole("button", { name: "Play (Space)" })).toBeVisible();
  },
};

// Behind live by the time paused; End goes live, and Go live goes with it.
const BehindLive: Story = {
  args: {
    setup: (send, clock) => {
      send({ key: " ", type: "key" });
      clock.t += 40_000;
      send({ key: " ", type: "key" });
    },
  },
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "Go live" })).toBeVisible();
    await userEvent.keyboard("{End}");
    await expect(canvas.queryByRole("button", { name: "Go live" })).toBeNull();
    await expect(args.onRequest).toHaveBeenLastCalledWith({ type: "goLive" });
  },
};

const BehindLiveSettled: Story = {
  args: {
    setup: (send, clock) => {
      send({ key: " ", type: "key" });
      clock.t += 40_000;
      send({ key: " ", type: "key" });
    },
  },
};

// A menu is the top layer: Esc closes it before anything else.
const MenuOpen: Story = {
  args: { setup: (send) => send({ key: "c", type: "key" }) },
};

const MenuClosesOnEsc: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.keyboard("c");
    await expect(canvas.getByTestId("watch-menu")).toBeVisible();
    await userEvent.keyboard("{Escape}");
    await expect(canvas.queryByTestId("watch-menu")).toBeNull();
  },
};

const OffAirCover: Story = { args: { cover: true } };

const TuningIn: Story = { args: { tune: TUNING_IN } };

const ChangingChannel: Story = {
  args: { setup: (send) => send({ key: "ArrowDown", type: "key" }), tune: CHANGING },
};

// Over a washed-out frame the words stand on the one foot scrim, with no plate or edge of their own.
const BrightFrame: Story = {
  args: { bright: true },
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelector('[data-testid^="picture-ink"]')).toBeNull();
    await expect(within(canvasElement).getByTestId("watch-scrim-foot")).toBeVisible();
  },
};

const BrightFrameListOpen: Story = {
  args: { bright: true, setup: (send) => send({ control: "channels", type: "press" }) },
};

/** The channel's logo in each corner over the bright frame: only the chrome in its corner steps clear. */
const LogoTopLeft: Story = { args: { bright: true, logo: "top-left" } };

/** The back control over the bright frame, awake, on its own corner ink so its words read. */
const BackOverBrightFrame: Story = {
  args: { back: true, bright: true },
  tags: ["checked-play"],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    // The quiet row has a Guide of its own; the back control is the one at the top leading corner.
    await expect(
      within(canvas.getByTestId("watch-back")).getByRole("button", { name: "Guide" }),
    ).toBeVisible();
    await waitFor(() => expect(canvas.getByTestId("watch-scrim-corner")).toBeInTheDocument());
  },
};
const LogoTopRight: Story = { args: { bright: true, logo: "top-right" } };
const LogoBottomLeft: Story = { args: { bright: true, logo: "bottom-left" } };
const LogoBottomRight: Story = { args: { bright: true, logo: "bottom-right" } };

export {
  AtRest,
  BackOverBrightFrame,
  BehindLive,
  BehindLiveSettled,
  BrightFrame,
  BrightFrameListOpen,
  ChangingChannel,
  Find,
  ListCursorOnRow,
  ListCursorOnTab,
  ListOpen,
  LogoBottomLeft,
  LogoBottomRight,
  LogoTopLeft,
  LogoTopRight,
  MenuClosesOnEsc,
  MenuOpen,
  NoSuchNumber,
  OffAirCover,
  Paused,
  Settled,
  TuningIn,
  TypingNumber,
  Wake,
};
export default meta;
