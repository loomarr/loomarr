import {
  type ChannelListState,
  channelListReduce,
  createChannelListState,
} from "@loomarr/core/channel-list";
import { createChannelLineup, type LineupChannel } from "@loomarr/core/channel-lineup";
import { Surface, Text, ThemeScope } from "@loomarr/design-system";
import { ChannelList, type ChannelListLayout } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fireEvent, userEvent, waitFor, within } from "storybook/test";

// The channel list of The Cross (frontend-design §9.5, target). Genre words only: the repo is public,
// so no real titles or channels. The pinned channel's drawing is a plain stand-in for the lower third.

const KINDS = ["Comedy", "Drama", "Mystery", "Western", "Cartoon", "Nature", "Cooking", "News", "Quiz", "Film"];
const DECADES = [1950, 1960, 1970, 1980, 1990, 2000];

const channels: LineupChannel[] = Array.from({ length: 30 }, (_, index) => ({
  decade: DECADES[Math.floor(index / 5)],
  id: `ch-${index + 1}`,
  name: `${KINDS[index % KINDS.length]} ${["Hour", "Corner", "Night"][Math.floor(index / 10)]}`,
  number: index + 1,
  onNow: `${KINDS[(index + 3) % KINDS.length]} half hour`,
  playable: true,
}));
const lineup = createChannelLineup({
  channels,
  favouriteIds: ["ch-3", "ch-9", "ch-21"],
  recentIds: ["ch-9", "ch-14", "ch-2"],
});
const empty = createChannelLineup({ channels });

type Frame = "window" | "full" | "television" | "phone";

const SIZES: Record<Frame, { width: number; height: number; column: number; rest: number }> = {
  full: { column: 420, height: 900, rest: 520, width: 1440 },
  phone: { column: 390, height: 560, rest: 96, width: 390 },
  television: { column: 380, height: 540, rest: 288, width: 960 },
  window: { column: 340, height: 900, rest: 480, width: 340 },
};
const LAYOUTS: Record<Frame, ChannelListLayout> = {
  full: "pointer",
  phone: "touch",
  television: "television",
  window: "pointer",
};

interface Args {
  frame: Frame;
  start?: Partial<ChannelListState>;
  watched?: number;
  recent?: "some" | "none";
  digits?: string;
  autoFocus?: boolean;
}

const List = ({ frame, start, watched = 14, recent = "some", digits, autoFocus }: Args) => {
  const model = recent === "none" ? empty : lineup;
  const [state, setState] = useState<ChannelListState>(() => ({
    ...createChannelListState({
      open: true,
      television: frame === "television",
      watchedId: `ch-${watched}`,
    }),
    ...start,
  }));
  const [looking, setLooking] = useState<string | undefined>(undefined);
  const size = SIZES[frame];
  const overPicture = frame === "full" || frame === "television";
  const playing = model.channels.find((channel) => channel.id === state.watchedId);
  const lookedAt = model.channels.find((channel) => channel.id === looking);

  const list = (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        gap: 8,
        height: "100%",
        // A television keeps 5 % clear at every edge.
        padding: frame === "television" ? "27px 48px" : 12,
        width: size.column + (frame === "television" ? 96 : 24),
      }}
    >
      <ChannelList
        autoFocus={autoFocus}
        detail={(channel) =>
          channel.number % 7 === 0 ? { inBreak: true } : { minutesLeft: 8 + (channel.number % 40) }
        }
        layout={LAYOUTS[frame]}
        lineup={model}
        onEvent={(event) =>
          setState((now) => {
            const step = channelListReduce(now, event, model);
            return step.tune
              ? channelListReduce(step.state, { channelId: step.tune, type: "tuned" }, model).state
              : step.state;
          })
        }
        onLook={setLooking}
        renderPinned={(channel) => (
          <Surface backgroundColor="surfaceElevated" borderWidth={0} gap={4} padding={8}>
            <Text textRole="numeral">{digits ?? channel.number}</Text>
            <Text textRole="cardLabel">{channel.name}</Text>
            <Text textRole="metadata" tone="secondary">
              {digits ? "Tuning in 2" : frame === "phone" ? "watching" : channel.onNow}
            </Text>
          </Surface>
        )}
        restOffset={size.rest}
        state={state}
      />
    </div>
  );

  const body = (
    <Surface
      backgroundColor={overPicture ? "transparent" : "surfaceCanvas"}
      borderWidth={0}
      height={size.height}
      width={overPicture ? size.width : size.column + 24}
    >
      {list}
      {lookedAt && playing ? (
        <Text textRole="metadata">
          {`Looking at ${lookedAt.number} ${lookedAt.name}, still playing ${playing.number} ${playing.name}`}
        </Text>
      ) : null}
    </Surface>
  );

  return overPicture ? (
    <ThemeScope theme="dark">
      <div
        style={{
          background: "linear-gradient(90deg, #07080a 0%, #14181f 40%, #5b4a38 100%)",
          height: size.height,
          width: size.width,
        }}
      >
        {body}
      </div>
    </ThemeScope>
  ) : (
    body
  );
};

const meta = {
  title: "Loomarr Components/Channel List",
  component: List,
  parameters: { layout: "fullscreen" },
  args: { frame: "window" },
} satisfies Meta<typeof List>;

export default meta;
type Story = StoryObj<typeof meta>;

const row = (id: number) => `[aria-label^="${id} "]`;
const scroller = (canvas: HTMLElement) =>
  canvas.querySelector<HTMLElement>('[data-testid="channel-list-scroller"]') as HTMLElement;
const settled = (canvas: HTMLElement) =>
  waitFor(() => expect(scroller(canvas).scrollTop).toBeGreaterThan(0));

export const ThirtyChannelsInAWindow: Story = {};
export const FullScreenOverThePicture: Story = { args: { frame: "full" } };
export const Television: Story = { args: { autoFocus: true, frame: "television" } };
/** The television rests on whole rows: neither the tabs nor the screen's edge cuts one. */
export const TelevisionRestsOnWholeRows: Story = {
  args: { autoFocus: true, frame: "television", watched: 2 },
};
export const UprightPhone: Story = { args: { frame: "phone" } };

export const Favourites: Story = { args: { frame: "full", start: { group: "favourites" } } };
export const Recent: Story = { args: { start: { group: "recent" } } };
export const RecentWithNothingInIt: Story = { args: { recent: "none", start: { group: "recent" } } };
export const ByDecade: Story = { args: { start: { group: "decade" } } };
export const ByDecadeOverThePicture: Story = { args: { frame: "full", start: { group: "decade" } } };
export const ByDecadeOnATelevision: Story = {
  args: { autoFocus: true, frame: "television", start: { group: "decade" } },
};
export const ByDecadeOnAPhone: Story = { args: { frame: "phone", start: { group: "decade" } } };

export const FindWithResults: Story = { args: { start: { query: "cartoon" } } };
export const FindWithNoMatch: Story = { args: { start: { query: "opera" } } };
export const FindOnAPhone: Story = { args: { frame: "phone", start: { query: "quiz" } } };
export const TypedDigits: Story = { args: { digits: "2_", frame: "full" } };

export const CursorOnThePinnedChannel: Story = {
  args: { autoFocus: true, start: { cursor: { channelId: "ch-14", kind: "row" } } },
};
export const CursorOnAnotherRow: Story = {
  args: { autoFocus: true, frame: "full", start: { cursor: { channelId: "ch-12", kind: "row" } } },
};
export const CursorOnATab: Story = {
  args: {
    autoFocus: true,
    frame: "television",
    start: { cursor: { group: "all", kind: "tab", returnTo: "ch-1" }, group: "all" },
    watched: 1,
  },
};

export const PointerLooking: Story = {
  play: async ({ canvasElement }) => {
    await settled(canvasElement);
    await userEvent.hover(canvasElement.querySelector(row(12)) as HTMLElement);
    await expect(await within(canvasElement).findByText(/^Looking at 12 /)).toBeVisible();
  },
};

const scrolledTo = (top: number): Story => ({
  play: async ({ canvasElement }) => {
    await settled(canvasElement);
    scroller(canvasElement).scrollTop = top;
    await fireEvent.scroll(scroller(canvasElement));
    const pinned = within(canvasElement).getByTestId("channel-list-pinned");
    const view = scroller(canvasElement).getBoundingClientRect();
    await waitFor(() => {
      const box = pinned.getBoundingClientRect();
      expect(box.top).toBeGreaterThanOrEqual(view.top - 1);
      expect(box.bottom).toBeLessThanOrEqual(view.bottom + 1);
    });
  },
});

export const ScrolledFarAboveThePinnedChannel: Story = scrolledTo(0);
export const ScrolledFarBelowThePinnedChannel: Story = scrolledTo(100_000);

export const KeyboardOnly: Story = {
  args: { autoFocus: true, start: { cursor: { channelId: "ch-1", kind: "row" } }, watched: 1 },
  play: async ({ canvasElement }) => {
    await settled(canvasElement);
    await userEvent.keyboard("{ArrowUp}");
    await expect(within(canvasElement).getByRole("tab", { name: "All" })).toHaveFocus();
  },
};
