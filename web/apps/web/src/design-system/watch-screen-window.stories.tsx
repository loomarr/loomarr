import type { LineupChannel } from "@loomarr/core/channel-lineup";
import { formatGuideTime } from "@loomarr/core/guide";
import { idleTune, type TuneView, tuneFlip, tuneView } from "@loomarr/core/tune";
import { type WatchRequest, watchKeyOf } from "@loomarr/core/watch-screen";
import { Surface, Text } from "@loomarr/design-system";
import {
  type StripAiring,
  useWatchScreen,
  WatchScreen,
  type WatchScreenInput,
  type WatchScreenLayout,
} from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { expect, fireEvent, fn, userEvent, waitFor, within } from "storybook/test";

// The watching screen of The Cross in a desktop window (frontend-design §9.7, target). Genre words
// only: the repo is public, so no real titles. The clock is fixed, the pictures are CSS stand-ins,
// and stills are `data:` stand-ins, never remote images.

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
  "Movie Matinee",
  "Sitcom Street",
  "Space Age",
  "Quiz Club",
  "Night Owls",
  "Road Movies",
  "Soap Box",
  "Weather Desk",
  "Music Hall",
  "Kids Corner",
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
const WATCHED = "ch-7";

const day = Date.UTC(2026, 9, 29);
const at = (h: number, m = 0) => day + (h * 60 + m) * 60_000;
const NOW = at(20, 26);
const formatTime = (ms: number) => formatGuideTime(ms, "UTC");

const still = (hue: number) =>
  `data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="hsl(${hue} 55% 46%)"/><stop offset="1" stop-color="hsl(${hue + 40} 45% 16%)"/></linearGradient></defs><rect width="320" height="180" fill="url(#g)"/></svg>`,
  )}`;

// A channel's evening: what is on now, a break, a run of one series, and a film.
const evening = (channel: Channel, art: boolean, inBreak: boolean): StripAiring[] => {
  const hue = channel.number * 37;
  const block = (start: number, stop: number, fields: Partial<StripAiring>): StripAiring => ({
    kind: "program",
    scheduleBlockId: `${channel.id}-${start}`,
    startMs: start,
    stopMs: stop,
    title: "",
    ...(art ? { thumbUrl: still(hue + start / 600_000) } : {}),
    ...fields,
  });
  const spots = (start: number, stop: number): StripAiring => ({
    kind: "filler",
    scheduleBlockId: `${channel.id}-${start}`,
    startMs: start,
    stopMs: stop,
    title: "",
  });
  return [
    block(at(19, 30), at(20), { title: "A cooking show" }),
    ...(inBreak
      ? [block(at(20), at(20, 24), { title: channel.title }), spots(at(20, 24), at(20, 30))]
      : [
          block(at(20), at(20, 56), {
            description: `${channel.onNow} that keeps the lights on late.`,
            genres: ["Drama"],
            rating: "TV-PG",
            title: channel.title,
          }),
          spots(at(20, 56), at(21)),
        ]),
    block(inBreak ? at(20, 30) : at(21), at(21, 25), {
      episode: 1,
      season: 1,
      series: "A sitcom",
      title: "Moving day",
    }),
    spots(at(21, 25), at(21, 29)),
    block(at(21, 29), at(21, 55), { episode: 2, season: 1, series: "A sitcom", title: "The sofa" }),
    block(at(21, 55), at(22, 45), {
      description: "A small town wakes to find every clock an hour fast, and one person remembers why.",
      genres: ["Science fiction"],
      rating: "PG",
      title: "A space mystery film",
    }),
    block(at(22, 45), at(23, 30), { title: "A late quiz" }),
  ];
};

const DARK_PICTURE = "linear-gradient(120deg, #2a3f55, #0b0c0e 70%)";
const IDLE = tuneView(idleTune, 0);
const TUNING_IN: TuneView = { bars: "waiting", cover: "cold", flip: 1, nextAt: null, run: 0 };
const CHANGING = tuneView(tuneFlip(idleTune, { at: 0, streaming: true }), 10);

type Clock = { t: number };
type Setup = (send: (event: WatchScreenInput) => void, clock: Clock) => void;

interface Args {
  art?: boolean;
  cover?: boolean;
  height?: number;
  hintSeen?: boolean;
  inBreak?: boolean;
  muted?: boolean;
  onRequest: (request: WatchRequest) => void;
  setup?: Setup;
  tune?: TuneView;
  width?: number;
}

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
  art = true,
  cover = false,
  height = 900,
  hintSeen = true,
  inBreak = false,
  muted = false,
  onRequest,
  setup,
  tune = IDLE,
  width = 1440,
}: Args) => {
  const clock = useRef<Clock>({ t: 0 }).current;
  // The caller's part of full screen: the screen asks, and the caller swaps the layout.
  const [layout, setLayout] = useState<WatchScreenLayout>("window");
  const model = useWatchScreen({
    hintSeen,
    layout,
    lineup: LINEUP,
    now: () => clock.t,
    onRequest: (request) => {
      onRequest(request);
      if (request.type === "fullScreen") setLayout(request.on ? "full" : "window");
    },
    watchedId: WATCHED,
  });
  const { send } = model;
  useEffect(() => {
    setup?.(send, clock);
  }, [clock, send, setup]);
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
      data-testid="story-picture"
      style={{ backgroundImage: DARK_PICTURE, height: "100%", width: "100%" }}
    />
  );
  const byId = (id: string) => CHANNELS.find((each) => each.id === id);
  return (
    <div data-testid="frame" style={{ height, overflow: "hidden", position: "relative", width }}>
      <WatchScreen
        back={{ label: "Home", onPress: () => undefined }}
        cover={cover ? <Cover /> : undefined}
        detail={(channel) =>
          inBreak && channel.id === WATCHED ? { inBreak: true } : { minutesLeft: 8 + channel.number }
        }
        holdSnow
        layout={layout}
        model={model}
        muted={muted}
        picture={picture}
        programme={(channel) =>
          inBreak && channel.id === WATCHED
            ? { backInMinutes: 4, kind: "break", title: byId(channel.id)?.title ?? channel.name }
            : {
                episode: "S2 E4",
                kind: "on",
                minutesLeft: 30,
                next: { startsAt: "21:00", title: "A sitcom" },
                rating: "TV-PG",
                title: byId(channel.id)?.title ?? channel.name,
              }
        }
        renderMenu={({ name }) => <Menu name={name} />}
        schedule={{ airings: (channel) => evening(channel, art, inBreak), formatTime, nowMs: NOW }}
        tune={tune}
      />
    </div>
  );
};

const meta = {
  title: "Loomarr Components/Watch Screen in a Window",
  component: Screen,
  args: { onRequest: fn() },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Screen>;

type Story = StoryObj<typeof meta>;

const tunes = (onRequest: Args["onRequest"]) =>
  (onRequest as ReturnType<typeof fn>).mock.calls.filter(([request]) => request.type === "tune");
const row = (canvas: HTMLElement, number: number) =>
  within(canvas).getByRole("option", { name: new RegExp(`^${number} `) });
const label = (canvas: HTMLElement) => within(canvas).getByTestId("watch-window-label");
const scroller = (canvas: HTMLElement) =>
  canvas.querySelector<HTMLElement>('[data-testid="channel-list-scroller"]') as HTMLElement;
const settled = (canvas: HTMLElement) => waitFor(() => expect(scroller(canvas).scrollTop).toBeGreaterThan(0));
const lookAt = async (canvas: HTMLElement, number: number) => {
  await settled(canvas);
  await userEvent.hover(row(canvas, number));
  await waitFor(() => expect(label(canvas)).toHaveTextContent(/^Looking at/));
};

// Nothing covers the picture, and the watched channel rests level with the top of the strip.
const Playing: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByTestId("watch-scrim-foot")).toBeNull();
    await expect(label(canvasElement)).toHaveTextContent("Today on Coast 80s");
    const strip = canvas.getByRole("group", { name: "Schedule" });
    const pinned = canvas.getByTestId("channel-list-pinned");
    await waitFor(() =>
      expect(Math.abs(pinned.getBoundingClientRect().top - strip.getBoundingClientRect().top)).toBeLessThan(
        6,
      ),
    );
  },
};

// Looking is not playing: the label names both channels, and nothing tunes.
const LookingAtAChannel: Story = {
  play: async ({ args, canvasElement }) => {
    await lookAt(canvasElement, 9);
    await expect(label(canvasElement)).toHaveTextContent(
      "Looking at 9 Movie Matinee · still playing 7 Coast 80s · click to tune",
    );
    await expect(within(canvasElement).getByRole("group", { name: "Programme facts" })).toHaveTextContent(
      "A matinee at 15",
    );
    await expect(tunes(args.onRequest)).toEqual([]);
  },
};

const PointingAtALaterProgramme: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.hover(canvas.getByRole("button", { name: /A space mystery film/ }));
    await expect(canvas.getByRole("group", { name: "Programme facts" })).toHaveTextContent("every clock");
    // Pointing is not focus: the ring is plain, never amber.
    await expect(canvas.queryByTestId("film-strip-ring-actionFocus")).toBeNull();
    await expect(canvas.getByTestId("film-strip-ring-contentPrimary")).toBeVisible();
  },
};

// Up and Down walk the focused rows and tune nothing; one amber ring.
const KeyboardFocusOnARow: Story = {
  play: async ({ args, canvasElement }) => {
    await settled(canvasElement);
    row(canvasElement, 9).focus();
    await userEvent.keyboard("{ArrowDown}{ArrowDown}");
    await expect(row(canvasElement, 11)).toHaveFocus();
    await expect(within(canvasElement).getAllByTestId("ring-actionFocus")).toHaveLength(1);
    await expect(tunes(args.onRequest)).toEqual([]);
  },
};

const ClickTunes: Story = {
  play: async ({ args, canvasElement }) => {
    await lookAt(canvasElement, 9);
    await userEvent.click(row(canvasElement, 9));
    await expect(tunes(args.onRequest)).toEqual([[{ channelId: "ch-9", type: "tune" }]]);
    await expect(label(canvasElement)).toHaveTextContent("Today on Movie Matinee");
  },
};

// After a tune the list re-centres under a still pointer: a second click on the same spot lands on
// another row, and does nothing until the pointer moves.
const SecondClickWaitsForThePointer: Story = {
  play: async ({ args, canvasElement }) => {
    await settled(canvasElement);
    const box = row(canvasElement, 10).getBoundingClientRect();
    const coords = { clientX: box.left + 40, clientY: box.top + box.height / 2 };
    const pressAt = async () =>
      userEvent.pointer([
        { coords, target: document.elementFromPoint(coords.clientX, coords.clientY) as Element },
        { keys: "[MouseLeft]", target: document.elementFromPoint(coords.clientX, coords.clientY) as Element },
      ]);
    await pressAt();
    await waitFor(() => expect(tunes(args.onRequest)).toHaveLength(1));
    await pressAt();
    await expect(tunes(args.onRequest)).toHaveLength(1);
    await expect(label(canvasElement)).not.toHaveTextContent(/^Looking at/);
  },
};

const WheelNeverTunes: Story = {
  play: async ({ args, canvasElement }) => {
    await settled(canvasElement);
    await fireEvent.wheel(scroller(canvasElement), { deltaY: 300 });
    await fireEvent.wheel(within(canvasElement).getByTestId("watch-picture"), { deltaY: 300 });
    await expect(tunes(args.onRequest)).toEqual([]);
  },
};

// F swaps to full screen and back with the channel, the group and the list's scroll kept.
const FullScreenAndBack: Story = {
  play: async ({ args, canvasElement }) => {
    await settled(canvasElement);
    // The picture is the player on a real route: it must be the same element throughout.
    const picture = within(canvasElement).getByTestId("story-picture");
    const top = scroller(canvasElement).scrollTop + 150;
    scroller(canvasElement).scrollTop = top;
    await fireEvent.scroll(scroller(canvasElement));
    await userEvent.keyboard("f");
    await waitFor(() => expect(within(canvasElement).queryByTestId("watch-window")).toBeNull());
    await userEvent.keyboard("f");
    await waitFor(() => expect(within(canvasElement).getByTestId("watch-window")).toBeVisible());
    await waitFor(() => expect(Math.abs(scroller(canvasElement).scrollTop - top)).toBeLessThan(2));
    await expect(label(canvasElement)).toHaveTextContent("Today on Coast 80s");
    await expect(within(canvasElement).getByTestId("story-picture")).toBe(picture);
    await expect(tunes(args.onRequest)).toEqual([]);
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
};

const TuningIn: Story = { args: { tune: TUNING_IN } };

const ChangingChannel: Story = {
  args: { setup: (send) => send({ key: "ArrowDown", type: "key" }), tune: CHANGING },
};

const Paused: Story = {
  args: { setup: (send) => send({ key: " ", type: "key" }) },
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole("button", { name: "Play (Space)" })).toBeVisible();
  },
};

// Behind live, Go live leads the quiet row: the pinned lower third takes no focus.
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
    await userEvent.click(canvas.getByRole("button", { name: "Go live" }));
    await expect(args.onRequest).toHaveBeenLastCalledWith({ type: "goLive" });
  },
};

const InABreak: Story = { args: { inBreak: true } };

const OffAirCover: Story = { args: { cover: true } };

const FirstPlayMuted: Story = {
  args: { hintSeen: false, muted: true },
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole("button", { name: "Unmute (M)" })).toBeVisible();
  },
};

// With no artwork the words still stand under the stills, clear of the tick and the playhead.
const NoArtwork: Story = { args: { art: false } };

const MenuOpen: Story = { args: { setup: (send) => send({ key: "c", type: "key" }) } };

// Narrower than 900 px the window stacks as an upright phone does.
const NarrowAtPhoneWidth: Story = {
  args: { height: 844, width: 390 },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() => expect(canvas.getByTestId("watch-window-narrow")).toBeVisible());
    await expect(canvas.queryByRole("group", { name: "Schedule" })).toBeNull();
  },
};

const JustBelowTheBreakpoint: Story = {
  args: { width: 899 },
  play: async ({ canvasElement }) => {
    await waitFor(() => expect(within(canvasElement).getByTestId("watch-window-narrow")).toBeVisible());
  },
};

const JustAboveTheBreakpoint: Story = {
  args: { width: 900 },
  play: async ({ canvasElement }) => {
    await waitFor(() => expect(within(canvasElement).getByTestId("watch-window")).toBeVisible());
  },
};

export {
  BehindLive,
  ChangingChannel,
  ClickTunes,
  Find,
  FirstPlayMuted,
  FullScreenAndBack,
  InABreak,
  JustAboveTheBreakpoint,
  JustBelowTheBreakpoint,
  KeyboardFocusOnARow,
  LookingAtAChannel,
  MenuOpen,
  NarrowAtPhoneWidth,
  NoArtwork,
  OffAirCover,
  Paused,
  Playing,
  PointingAtALaterProgramme,
  SecondClickWaitsForThePointer,
  TuningIn,
  TypingNumber,
  WheelNeverTunes,
};
export default meta;
