// @vitest-environment jsdom

import { act, cleanup, fireEvent, renderUi, screen, userEvent } from "@loomarr/test-support";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FilmStrip, type FilmStripProps, ProgrammeFacts, type StripAiring, StripMessage } from "../index";

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
const key = (name: string) =>
  act(() => void fireEvent.keyDown(document.activeElement as Element, { key: name }));

describe("FilmStrip's two sizing rules", () => {
  const long = [airing(0, 1), airing(1, 180)];
  const window = { startMs: at(0), stopMs: at(180) };

  it("draws a short airing at the minimum width under the picture, and a long one at the maximum", () => {
    strip({ airings: long, nowMs: at(0), window });
    expect(style(block(/Film 0/)).width).toBe("150px");
    expect(style(block(/Film 1/)).width).toBe("330px");
    cleanup();
    strip({ airings: long, maxBlockWidth: 200, minBlockWidth: 100, nowMs: at(0), window });
    expect(style(block(/Film 0/)).width).toBe("100px");
    expect(style(block(/Film 1/)).width).toBe("200px");
  });

  it("keeps a Guide row true to the clock, with a 44px ring on a focused or pointed sliver", async () => {
    strip({ airings: [airing(0, 179), airing(179, 180)], layout: "row", nowMs: at(0), window });
    const sliver = block(/Film 179/);
    expect(Number.parseFloat(style(sliver).width)).toBeCloseTo(100 / 180, 2);
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
    expect(Number.parseFloat(style(sliver).width)).toBeCloseTo(100 / 180, 2);
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
