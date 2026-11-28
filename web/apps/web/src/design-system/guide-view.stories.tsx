import { createChannelLineup, type LineupGroupKind } from "@loomarr/core/channel-lineup";
import { formatGuideTime } from "@loomarr/core/guide";
import {
  createGuideViewState,
  type GuideViewContext,
  type GuideViewState,
  guideViewReduce,
} from "@loomarr/core/guide-view";
import { Surface } from "@loomarr/design-system";
import { LowerThird, type StripAiring } from "@loomarr/ui";
import { GuideView, type GuideViewEntry, type GuideViewLayout, guideViewChannels } from "@loomarr/ui/guide";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";

// The Guide of The Cross (frontend-design §9.5, target). Genre words only: the repo is public, so no
// real titles. The clock is fixed, and stills are `data:` stand-ins, never remote images.

const day = Date.UTC(2026, 9, 29);
const HOUR = 3_600_000;
const at = (h: number, m = 0) => day + (h * 60 + m) * 60_000;
const NOW = at(20, 26);
const formatTime = (ms: number) => formatGuideTime(ms, "UTC");
const dayName = (ms: number) => (ms >= at(24) ? "Tomorrow" : undefined);

const still = (hue: number) =>
  `data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180"><rect width="320" height="180" fill="hsl(${hue} 45% 34%)"/></svg>`,
  )}`;

const TITLES = ["A police drama", "A space mystery film", "A cooking show", "A quiz", "A western film"];
const NAMES = [
  "Coast 90s",
  "Game Night",
  "Late Mysteries",
  "Saturday Cartoons",
  "Kitchen",
  "Westerns",
  "Science",
  "Quiet Hours",
];
const DECADES = [1990, 1980, 1970, 1990, 2000, 1960, 1980, 1990];

/** A channel's evening and the next day's morning, in lengths that differ from row to row. */
const schedule = (row: number, pictured: boolean): StripAiring[] => {
  const airings: StripAiring[] = [];
  let startMs = at(19, 30);
  for (let n = 0; startMs < at(27); n += 1) {
    const stopMs = startMs + (25 + ((n + row) % 4) * 15) * 60_000;
    const run = row === 0 && n >= 2 && n <= 3;
    airings.push({
      description: "The night shift takes a call nobody else will answer.",
      genres: ["Drama"],
      kind: "program",
      rating: "PG",
      scheduleBlockId: `c${row}-${n}`,
      startMs,
      stopMs,
      title: TITLES[(n + row) % TITLES.length] as string,
      ...(run ? { series: "A sitcom", episode: n, season: 1 } : {}),
      ...(pictured ? { thumbUrl: still(row * 45 + n * 20) } : {}),
    });
    const spots = n % 2 ? 5 * 60_000 : 0;
    if (spots) {
      airings.push({
        kind: "filler",
        scheduleBlockId: `f${row}-${n}`,
        startMs: stopMs,
        stopMs: stopMs + spots,
        title: "",
      });
    }
    startMs = stopMs + spots;
  }
  return airings;
};

const channelsOf = (pictured: boolean) =>
  guideViewChannels(
    NAMES.map((name, row) => ({
      // The last channel has nothing scheduled.
      airings: name === "Quiet Hours" ? [] : schedule(row, pictured),
      decade: DECADES[row],
      id: `c${row}`,
      name,
      number: 7 + row * 2,
      playable: true,
    })),
  );

interface Args {
  layout: GuideViewLayout;
  group?: LineupGroupKind;
  favouriteIds?: string[];
  pictured?: boolean;
  loading?: boolean;
}

const SIZES: Record<GuideViewLayout, { width: number; height: number }> = {
  desktop: { height: 800, width: 1280 },
  phone: { height: 844, width: 390 },
  television: { height: 540, width: 960 },
};

const Harness = ({ favouriteIds = ["c1", "c2"], group, layout, loading, pictured = true }: Args) => {
  const channels = channelsOf(pictured);
  const context: GuideViewContext<GuideViewEntry> = {
    formatTime,
    lineup: createChannelLineup<GuideViewEntry>({ channels, favouriteIds }),
    hourStartMs: at(20),
    nowMs: NOW,
    untilMs: at(27),
    widthMs: (layout === "desktop" ? 3 : 2) * HOUR,
  };
  const [state, setState] = useState<GuideViewState>(() =>
    createGuideViewState({ group, television: layout === "television", watchedId: "c0" }, context),
  );
  const [message, setMessage] = useState<string | undefined>(undefined);
  const picture = <Surface backgroundColor="surfaceFocus" borderRadius={0} borderWidth={0} flex={1} />;
  return (
    <Surface
      backgroundColor="surfaceCanvas"
      borderRadius={0}
      borderWidth={0}
      height={SIZES[layout].height}
      width={SIZES[layout].width}
    >
      {loading ? (
        <GuideView layout={layout} loading picture={picture} />
      ) : (
        <GuideView
          autoFocus
          context={context}
          dayName={dayName}
          layout={layout}
          lowerThird={
            <LowerThird
              channelName="Coast 90s"
              channelNumber={7}
              decade={1990}
              device={layout === "desktop" ? "window" : layout === "phone" ? "portrait" : "tv"}
              portraitTitleLines={2}
              state={{ kind: "on", minutesLeft: 30, title: "A space mystery film" }}
            />
          }
          message={message}
          onEvent={(event) => {
            const step = guideViewReduce(state, event, context);
            const tuned = step.tune
              ? guideViewReduce(step.state, { channelId: step.tune, type: "tuned" }, context).state
              : step.state;
            setState(tuned);
            setMessage(step.message);
          }}
          picture={picture}
          state={state}
        />
      )}
    </Surface>
  );
};

const meta = {
  component: Harness,
  parameters: { layout: "fullscreen" },
  // The gallery fails these stories on anything `play()` throws.
  tags: ["checked-play"],
  title: "Watching/Guide view",
} satisfies Meta<Args>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The Guide takes focus onto its cursor after it mounts; keys sent sooner go nowhere. */
const ready = (canvasElement: HTMLElement) =>
  waitFor(() => expect(canvasElement.querySelector('[data-testid*="-now"]:focus')).not.toBeNull());

const press =
  (keys: string): Story["play"] =>
  async ({ canvasElement }) => {
    await ready(canvasElement);
    await userEvent.keyboard(keys);
  };

/** What an element and everything it draws covers, so a box that reserves no height still counts. */
const covered = (element: Element) => {
  const boxes: DOMRect[] = [];
  const visit = (each: Element) => {
    const box = each.getBoundingClientRect();
    if (box.width > 1 && box.height > 1) boxes.push(box);
    // What a box clips is not drawn outside it: a scroller's rows, a screen reader's hidden words.
    if (getComputedStyle(each).overflow === "visible") for (const child of each.children) visit(child);
  };
  visit(element);
  return {
    bottom: Math.max(...boxes.map((box) => box.bottom)),
    left: Math.min(...boxes.map((box) => box.left)),
    right: Math.max(...boxes.map((box) => box.right)),
    top: Math.min(...boxes.map((box) => box.top)),
  };
};

/** Fails, naming each pair, when any of these named elements are drawn over each other. */
const expectNoOverlap = async (named: [string, Element | undefined][]) => {
  const drawn = named.flatMap(([name, element]) =>
    element?.textContent || element?.firstElementChild ? [{ name, box: covered(element) }] : [],
  );
  const clashes = drawn.flatMap((a, at) =>
    drawn
      .slice(at + 1)
      // A point of slack for borders that share an edge.
      .filter(
        (b) =>
          a.box.left < b.box.right - 1 &&
          b.box.left < a.box.right - 1 &&
          a.box.top < b.box.bottom - 1 &&
          b.box.top < a.box.bottom - 1,
      )
      .map((b) => `${a.name} overlaps ${b.name}`),
  );
  await expect(clashes).toEqual([]);
};

/** The Guide's parts, top to bottom, and each channel's label and row. */
const expectLaidOut = async (canvasElement: HTMLElement, layout: GuideViewLayout) => {
  const all = (id: string) => [...canvasElement.querySelectorAll(`[data-testid="guide-view-${id}"]`)];
  const one = (id: string) => [id, all(id)[0]] as [string, Element | undefined];
  const labels = all("label");
  const rows = all("row");
  await expect(rows.length).toBeGreaterThan(0);
  const channels = labels.flatMap((label, at): [string, Element | undefined][] => [
    [`label ${at}`, label],
    [`row ${at}`, rows[at]],
  ]);
  await expectNoOverlap([
    one("picture"),
    one("header"),
    one("tabs"),
    one("pager"),
    one("ruler"),
    ...channels,
  ]);
  if (all("facts").length) await expectNoOverlap([one("rows"), one("facts")]);
  if (layout !== "phone") return;
  // A phone's label stands on its own line: above its row, and below the row before it.
  for (const [at, label] of labels.entries()) {
    const row = rows[at] as Element;
    await expect(covered(label).bottom).toBeLessThanOrEqual(covered(row).top + 1);
    if (at)
      await expect(covered(label).top).toBeGreaterThanOrEqual(covered(rows[at - 1] as Element).bottom - 1);
  }
};

const laidOut =
  (layout: GuideViewLayout, keys?: string): Story["play"] =>
  async ({ canvasElement }) => {
    await ready(canvasElement);
    if (keys) await userEvent.keyboard(keys);
    await expectLaidOut(canvasElement, layout);
  };

const light = { theme: "light" } as const;
const desktop = { layout: "desktop" } as const;
const television = { layout: "television" } as const;
const phone = { layout: "phone" } as const;

const Desktop: Story = {
  args: desktop,
  play: async ({ canvasElement }) => {
    await ready(canvasElement);
    await expect(within(canvasElement).getByText("Playing 7 Coast 90s")).toBeVisible();
    await expectLaidOut(canvasElement, "desktop");
  },
};
const Television: Story = { args: television, play: laidOut("television") };
const Phone: Story = { args: phone, play: laidOut("phone") };

const LookingAtAnotherChannel: Story = {
  args: desktop,
  play: async ({ canvasElement }) => {
    await ready(canvasElement);
    await userEvent.keyboard("{ArrowDown}");
    await expect(within(canvasElement).getByText(/^Looking at Game Night/)).toBeVisible();
    await expect(within(canvasElement).getByText("Playing 7 Coast 90s")).toBeVisible();
  },
};
const LookingAtALaterProgramme: Story = {
  args: desktop,
  play: async ({ canvasElement }) => {
    await ready(canvasElement);
    await userEvent.keyboard("{ArrowDown}{ArrowRight}");
    await expect(within(canvasElement).getByText(/^Looking at Game Night/)).toBeVisible();
  },
};
const PhoneLater: Story = { args: phone, play: laidOut("phone", "{ArrowDown}{ArrowRight}") };
const TelevisionLooking: Story = {
  args: television,
  play: laidOut("television", "{ArrowDown}{ArrowRight}"),
};

const Message: Story = {
  args: desktop,
  play: async ({ canvasElement }) => {
    await ready(canvasElement);
    await userEvent.keyboard("{ArrowDown}{ArrowRight}{Enter}");
    await expect(within(canvasElement).getByRole("status")).toHaveTextContent("starts at");
  },
};
const PagedLater: Story = { args: desktop, play: press("{ArrowRight}{ArrowRight}{ArrowRight}{ArrowRight}") };
const PastMidnight: Story = {
  args: desktop,
  play: async ({ canvasElement }) => {
    const later = within(canvasElement).getByRole("button", { name: "Later" });
    for (let page = 0; page < 4; page += 1) await userEvent.click(later);
    await expect(within(canvasElement).getByText(/^Tomorrow/)).toBeVisible();
    await expect(within(canvasElement).getByText(/^The schedule ends at/)).toBeVisible();
  },
};
const ByDecade: Story = { args: { ...desktop, group: "decade" } };
const Favourites: Story = { args: { ...desktop, group: "favourites" } };
const EmptyGroup: Story = {
  args: { ...desktop, favouriteIds: [] },
  play: async ({ canvasElement }) => {
    // The cursor is on the watched channel, so Up reaches the tabs and Left the one before All.
    await ready(canvasElement);
    await userEvent.keyboard("{ArrowUp}{ArrowLeft}{Enter}");
    await expect(within(canvasElement).getByText("No favourites yet.")).toBeVisible();
  },
};
const Loading: Story = { args: { ...desktop, loading: true } };
const NothingScheduled: Story = {
  args: television,
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByText("Nothing scheduled")).toBeInTheDocument();
  },
};
const NoArtwork: Story = { args: { ...desktop, pictured: false } };
const DesktopLight: Story = { args: desktop, globals: light };
const ByDecadeLight: Story = { args: { ...desktop, group: "decade" }, globals: light };

export {
  ByDecade,
  ByDecadeLight,
  Desktop,
  DesktopLight,
  EmptyGroup,
  Favourites,
  Loading,
  LookingAtALaterProgramme,
  LookingAtAnotherChannel,
  Message,
  NoArtwork,
  NothingScheduled,
  PagedLater,
  PastMidnight,
  Phone,
  PhoneLater,
  Television,
  TelevisionLooking,
};
