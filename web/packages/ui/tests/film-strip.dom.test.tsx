// @vitest-environment jsdom

import { act, cleanup, fireEvent, renderUi, screen, userEvent } from "@loomarr/test-support";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  FilmStrip,
  type FilmStripHandle,
  type FilmStripProps,
  ProgrammeFacts,
  type StripAiring,
  StripMessage,
} from "../index";

afterEach(cleanup);

const at = (minutes: number) => minutes * 60_000;
const formatTime = (ms: number) => `T${ms / 60_000}`;
const airing = (start: number, stop: number, fields: Partial<StripAiring> = {}): StripAiring => ({
  kind: "program",
  scheduleBlockId: `b${start}`,
  startMs: at(start),
  stopMs: at(stop),
  title: `Film ${start}`,
  ...fields,
});

const airings = [
  airing(0, 30),
  airing(30, 60, { series: "Diner", thumbUrl: "/now.webp", title: "One" }),
  airing(60, 62, { kind: "filler" }),
  airing(62, 90, { series: "Diner", title: "Two" }),
  airing(90, 120, { description: "A long night.", genres: ["Comedy"], rating: "PG" }),
];

const strip = (props: Partial<FilmStripProps> = {}) =>
  renderUi(
    <FilmStrip
      airings={airings}
      formatTime={formatTime}
      nowMs={at(45)}
      window={{ startMs: at(0), stopMs: at(120) }}
      {...props}
    />,
  );

const names = () => screen.getAllByRole("button").map((b) => b.getAttribute("aria-label"));

describe("FilmStrip", () => {
  it("names every block with its title and time, a run once with its count, and a break as a tick", () => {
    strip();
    expect(names()).toEqual(["Film 0, ended T30", "Now · until T90 · 2 episodes Diner", "T90 Film 90"]);
    expect(screen.getAllByRole("img", { name: "Break at T60" })).toHaveLength(1);
    // An ended programme shows no time; a later one shows its start.
    expect(screen.getByTestId("film-strip-ended").textContent).toBe("Film 0");
    expect(screen.getByTestId("film-strip-later").textContent).toContain("T90");
  });

  it("dims the aired part of what is on now and nothing else", () => {
    strip();
    const aired = screen.getAllByTestId("film-strip-aired");
    expect(aired).toHaveLength(1);
    expect(screen.getByTestId("film-strip-now").contains(aired[0] as HTMLElement)).toBe(true);
  });

  it("shows the live mark only behind live", () => {
    strip();
    expect(screen.queryByRole("img", { name: "Live" })).toBeNull();
    cleanup();
    strip({ playheadMs: at(40) });
    expect(screen.getByRole("img", { name: "Live" })).toBeTruthy();
    expect(screen.getByTestId("film-strip-playhead")).toBeTruthy();
  });

  it("reports what is focused or pointed at, and what is pressed", () => {
    const onLook = vi.fn();
    const onSelect = vi.fn();
    strip({ onLook, onSelect });
    const [, now, later] = screen.getAllByRole("button");

    act(() => (later as HTMLElement).focus());
    expect(onLook).toHaveBeenLastCalledWith(expect.objectContaining({ startMs: at(90) }));

    // A pointer passing over another block looks there, then returns to the focused one.
    fireEvent.pointerEnter(now as HTMLElement, { pointerType: "mouse" });
    fireEvent.mouseEnter(now as HTMLElement);
    expect(onLook).toHaveBeenLastCalledWith(expect.objectContaining({ episodes: 2 }));
    fireEvent.pointerLeave(now as HTMLElement, { pointerType: "mouse" });
    fireEvent.mouseLeave(now as HTMLElement);
    expect(onLook).toHaveBeenLastCalledWith(expect.objectContaining({ startMs: at(90) }));

    act(() => (later as HTMLElement).blur());
    expect(onLook).toHaveBeenLastCalledWith(undefined);

    fireEvent.click(later as HTMLElement);
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ startMs: at(90) }));
  });
});

const style = (element: Element) => getComputedStyle(element);
const block = (name: RegExp) => screen.getByRole("button", { name });
// The layer around a block's button places and sizes it.
const placed = (button: Element) => style(button.parentElement as Element);
const key = (name: string) =>
  act(() => void fireEvent.keyDown(document.activeElement as Element, { key: name }));

describe("FilmStrip's picture", () => {
  it("fills the block's still with its artwork", () => {
    const { container } = strip();
    // react-native-web draws an Image as a box, the caller's style on it, holding the img.
    const box = style(container.querySelector('img[src="/now.webp"]')?.parentElement as Element);
    expect([box.position, box.top, box.left]).toEqual(["absolute", "0px", "0px"]);
    expect([box.width, box.height]).toEqual(["100%", "100%"]);
  });
});

describe("FilmStrip's two sizing rules", () => {
  const long = [airing(0, 1), airing(1, 180)];
  const window = { startMs: at(0), stopMs: at(180) };

  it("draws a short airing at the minimum width under the picture, and a long one at the maximum", () => {
    strip({ airings: long, nowMs: at(0), window });
    expect(placed(block(/Film 0/)).width).toBe("150px");
    expect(placed(block(/Film 1/)).width).toBe("330px");
    cleanup();
    strip({ airings: long, maxBlockWidth: 200, minBlockWidth: 100, nowMs: at(0), window });
    expect(placed(block(/Film 0/)).width).toBe("100px");
    expect(placed(block(/Film 1/)).width).toBe("200px");
  });

  it("keeps a Guide row true to the clock, with a 44px ring on a focused or pointed sliver", async () => {
    strip({ airings: [airing(0, 179), airing(179, 180)], layout: "row", nowMs: at(0), window });
    const sliver = block(/Film 179/);
    expect(Number.parseFloat(placed(sliver).width)).toBeCloseTo(100 / 180, 2);
    expect(screen.queryByTestId("film-strip-target")).toBeNull();

    // jsdom does not model `:focus-visible`; the KeyboardFocus story holds the browser to it.
    for (const button of screen.getAllByRole("button")) {
      const matches = button.matches.bind(button);
      vi.spyOn(button, "matches").mockImplementation((selector: string) =>
        selector === ":focus-visible" ? button === document.activeElement : matches(selector),
      );
    }
    const user = userEvent.setup();
    await user.tab();
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(sliver);
    expect(style(screen.getByTestId("film-strip-target")).minWidth).toBe("44px");
    expect(Number.parseFloat(placed(sliver).width)).toBeCloseTo(100 / 180, 2);
    expect(sliver.getAttribute("aria-label")).toBe("T179 Film 179");

    act(() => sliver.blur());
    fireEvent.mouseEnter(sliver);
    fireEvent.pointerEnter(sliver, { pointerType: "mouse" });
    expect(style(screen.getByTestId("film-strip-target")).minWidth).toBe("44px");
    vi.restoreAllMocks();
  });
});

describe("FilmStrip by keyboard", () => {
  it("is one Tab stop, on what is on now, and arrows, Home and End move within it", () => {
    strip();
    const [first, now, last] = screen.getAllByRole("button") as [HTMLElement, HTMLElement, HTMLElement];
    const stops = () => screen.getAllByRole("button").filter((b) => b.tabIndex === 0);
    expect(stops()).toEqual([now]);

    act(() => now.focus());
    key("ArrowRight");
    expect(document.activeElement).toBe(last);
    key("ArrowRight");
    expect(document.activeElement).toBe(last);
    key("Home");
    expect(document.activeElement).toBe(first);
    key("ArrowLeft");
    expect(document.activeElement).toBe(first);
    key("End");
    expect(document.activeElement).toBe(last);
    expect(stops()).toEqual([last]);
  });

  it("takes focus from outside a step from what is on now, and stops at either end", () => {
    const handle: { current: FilmStripHandle | null } = { current: null };
    strip({ ref: handle });
    const [first, , last] = screen.getAllByRole("button") as [HTMLElement, HTMLElement, HTMLElement];
    act(() => handle.current?.focusFromNow(1));
    expect(document.activeElement).toBe(last);
    act(() => handle.current?.focusFromNow(-1));
    expect(document.activeElement).toBe(first);
    strip({ airings: [airing(0, 30), airing(30, 60)], nowMs: at(50), ref: handle });
    act(() => handle.current?.focusFromNow(1));
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Now · until T60 Film 30");
  });
});

describe("FilmStrip's leading edge", () => {
  it("fades only once the strip is scrolled along from its start", async () => {
    strip();
    const group = screen.getByRole("group", { name: "Schedule" });
    const masked = () => (group.getAttribute("style") ?? "").includes("transparent, black 90px");
    expect(masked()).toBe(false);
    const scroller = group.firstElementChild as HTMLElement;
    fireEvent.scroll(scroller, { target: { scrollLeft: 120 } });
    expect(masked()).toBe(true);
    // The scroll view reports a scroll at most every 16 ms.
    await act(() => new Promise((done) => setTimeout(done, 40)));
    fireEvent.scroll(scroller, { target: { scrollLeft: 0 } });
    expect(masked()).toBe(false);
  });
});

describe("FilmStrip's words under the picture", () => {
  it("stand under every block, with artwork or without, so no tick or playhead crosses them", () => {
    strip({ airings: airings.map(({ thumbUrl: _, ...rest }) => rest) });
    expect(screen.queryByTestId("film-strip-words")).toBeNull();
    expect(screen.getByTestId("film-strip-now").textContent).toContain("Diner");
  });

  it("stay inside the block in a Guide row", () => {
    strip({ layout: "row" });
    expect(screen.getAllByTestId("film-strip-words").length).toBeGreaterThan(0);
  });
});

describe("FilmStrip's order and marks", () => {
  it("keeps blocks and ticks in time order", () => {
    strip({
      airings: [
        airing(0, 30),
        airing(30, 32, { kind: "filler" }),
        airing(32, 60),
        airing(60, 62, { kind: "filler" }),
        airing(62, 90),
      ],
    });
    const inOrder = [...screen.getByRole("group", { name: "Schedule" }).querySelectorAll("[aria-label]")];
    expect(inOrder.map((e) => e.getAttribute("aria-label")?.split(",")[0])).toEqual([
      "Film 0",
      "Break at T30",
      "Now · until T60 Film 32",
      "Break at T60",
      "T62 Film 62",
    ]);
  });

  it("gives an open run its title and no count", () => {
    strip({
      airings: [
        airing(0, 30),
        airing(30, 60, { series: "Diner" }),
        airing(60, 90, { series: "Diner", title: "Two" }),
      ],
    });
    expect(block(/Diner/).getAttribute("aria-label")).toBe("Now · until T90 Diner");
  });

  it("starts a Guide row's words after the playhead only when the programme has room left", () => {
    const wordsLeft = () => style(screen.getByTestId("film-strip-words")).left;
    const one = {
      airings: [airing(0, 60)],
      layout: "row" as const,
      window: { startMs: at(0), stopMs: at(60) },
    };
    strip({ ...one, nowMs: at(30) });
    expect(wordsLeft()).toBe("50%");
    cleanup();
    strip({ ...one, nowMs: at(45) });
    expect(wordsLeft()).toBe("0px");
  });

  it("sets the Live word before its mark near the trailing edge", () => {
    const row = { layout: "row" as const, playheadMs: at(40), window: { startMs: at(0), stopMs: at(120) } };
    strip({ ...row, nowMs: at(115) });
    expect(screen.getByTestId("film-strip-live-leading")).toBeTruthy();
    cleanup();
    strip({ ...row, nowMs: at(50) });
    expect(screen.getByTestId("film-strip-live-trailing")).toBeTruthy();
  });
});

describe("FilmStrip's edges", () => {
  const view = (list: StripAiring[], nowMs = at(15)) => (
    <FilmStrip
      airings={list}
      formatTime={formatTime}
      nowMs={nowMs}
      window={{ startMs: at(0), stopMs: at(120) }}
    />
  );
  const labels = () => screen.queryAllByRole("button").map((b) => b.getAttribute("aria-label"));
  const stops = () =>
    screen
      .queryAllByRole("button")
      .filter((b) => b.tabIndex === 0)
      .map((b) => b.getAttribute("aria-label"));

  it("is an empty Schedule group with no blocks when there are no airings", () => {
    renderUi(view([]));
    expect(screen.getByRole("group", { name: "Schedule" })).toBeTruthy();
    expect(labels()).toEqual([]);
    expect(screen.queryAllByRole("img")).toEqual([]);
  });

  it("is a Schedule group of one Tab-stop block for one airing", () => {
    renderUi(view([airing(0, 30)]));
    expect(screen.getByRole("group", { name: "Schedule" })).toBeTruthy();
    expect(labels()).toEqual(["Now · until T30 Film 0"]);
    expect(stops()).toEqual(labels());
  });

  it("is a Schedule group of named break ticks and no blocks when there are only breaks", () => {
    renderUi(view([airing(0, 30, { kind: "filler" }), airing(30, 60, { kind: "flex" })]));
    expect(screen.getByRole("group", { name: "Schedule" })).toBeTruthy();
    expect(labels()).toEqual([]);
    expect(screen.getAllByRole("img").map((i) => i.getAttribute("aria-label"))).toEqual([
      "Break at T0",
      "Break at T30",
    ]);
  });

  it("gives a strip that is one single run its title and no count", () => {
    renderUi(view([airing(0, 30, { series: "Diner" }), airing(30, 60, { series: "Diner", title: "Two" })]));
    expect(labels()).toEqual(["Now · until T60 Diner"]);
    // The drawn words carry the same: the time line, the title, and no episode count.
    expect(screen.getByRole("button").textContent?.replace(/\s+/g, " ").trim()).toBe("Now · until T60 Diner");
  });

  it("scrolls a block into view when the keyboard focuses it, never when a pointer does", () => {
    // jsdom has no layout and no scrolling: the view's width is handed to react-native-web's layout
    // handler, and `scroll` is what its ScrollView calls.
    const scroll = vi.fn();
    Object.assign(HTMLElement.prototype, { scroll });
    renderUi(view([airing(0, 30), airing(30, 60), airing(60, 90)]));
    const group = screen.getByRole("group", { name: "Schedule" }) as HTMLElement & {
      __reactLayoutHandler?: (event: unknown) => void;
    };
    act(() => group.__reactLayoutHandler?.({ nativeEvent: { layout: { width: 200 } } }));
    const [, middle, last] = screen.getAllByRole("button") as [HTMLElement, HTMLElement, HTMLElement];
    const focusFrom = (button: HTMLElement, keys: boolean) => {
      const matches = button.matches.bind(button);
      vi.spyOn(button, "matches").mockImplementation((selector: string) =>
        selector === ":focus-visible" ? keys : matches(selector),
      );
      act(() => button.focus());
    };

    focusFrom(last, false);
    expect(scroll).not.toHaveBeenCalled();
    focusFrom(middle, true);
    expect(scroll).toHaveBeenCalledTimes(1);

    vi.restoreAllMocks();
    delete (HTMLElement.prototype as { scroll?: unknown }).scroll;
  });

  it("moves the Tab stop to what is on now, or else the first, when the focused airing leaves", () => {
    const [first, now, later] = [airing(0, 30), airing(30, 60), airing(60, 90)];
    const { rerender } = renderUi(view([first, now, later], at(45)));
    act(() => screen.getByRole("button", { name: /Film 60/ }).focus());
    expect(stops()).toEqual(["T60 Film 60"]);

    rerender(view([first, now], at(45)));
    expect(stops()).toEqual(["Now · until T60 Film 30"]);

    // Nothing on now: the first block holds the stop.
    rerender(view([first, airing(90, 120)], at(45)));
    expect(stops()).toEqual(["Film 0, ended T30"]);
  });

  it("puts the playhead halfway along a block drawn at the minimum width, under the picture", () => {
    renderUi(view([airing(0, 10)], at(5)));
    expect(placed(screen.getByRole("button")).width).toBe("150px");
    expect(style(screen.getByTestId("film-strip-playhead")).left).toBe("75px");
  });

  it("starts a Guide row's words after the Live word when behind live", () => {
    const row = {
      airings: [airing(0, 120)],
      layout: "row" as const,
      nowMs: at(60),
      window: { startMs: at(0), stopMs: at(120) },
    };
    const words = () => style(screen.getByTestId("film-strip-words"));
    // The room for the Live word is padding on the box inside the words' layer.
    const inset = () =>
      style(screen.getByTestId("film-strip-words").firstElementChild as Element).paddingLeft;
    strip(row);
    expect(inset()).toBe("0px");
    cleanup();
    strip({ ...row, playheadMs: at(50) });
    expect(words().left).toBe("50%");
    expect(inset()).toBe("48px");
    cleanup();
    // Too little left to move the words at all.
    strip({ ...row, nowMs: at(110), playheadMs: at(100) });
    expect(inset()).toBe("0px");
  });
});

describe("ProgrammeFacts", () => {
  const later = airing(90, 120, {
    description: "A long night.",
    episode: 4,
    genres: ["Comedy", "Drama"],
    rating: "PG",
    season: 2,
    series: "Diner",
    title: "Closing time",
  });

  it("reads title, episode, rating, genres, synopsis and when it starts", () => {
    renderUi(<ProgrammeFacts airing={later} formatTime={formatTime} nowMs={at(45)} />);
    const facts = screen.getByRole("group", { name: "Programme facts" });
    for (const words of [
      "Diner",
      "S02E04 · Closing time",
      "PG · Comedy, Drama · Starts T90",
      "A long night.",
    ]) {
      expect(facts.textContent).toContain(words);
    }
    expect(screen.queryByTestId("programme-facts-plate")).toBeNull();
  });

  it("says until when on now, and stands on a plate over the picture", () => {
    renderUi(<ProgrammeFacts airing={later} formatTime={formatTime} ground="picture" nowMs={at(100)} />);
    expect(screen.getByTestId("programme-facts-plate").textContent).toContain("Until T120");
  });
});

describe("StripMessage", () => {
  it("says a raised message in a status region", () => {
    renderUi(<StripMessage message="Diner starts at T90" />);
    expect(screen.getByRole("status").textContent).toBe("Diner starts at T90");
  });
});
