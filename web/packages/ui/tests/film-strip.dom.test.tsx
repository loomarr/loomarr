// @vitest-environment jsdom

import { act, cleanup, fireEvent, renderUi, screen } from "@loomarr/test-support";
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

  it("is in the tab order, and reports what is focused or pointed at", () => {
    const onLook = vi.fn();
    const onSelect = vi.fn();
    strip({ onLook, onSelect });
    const [, now, later] = screen.getAllByRole("button");
    expect(screen.getAllByRole("button").every((b) => b.tabIndex === 0)).toBe(true);

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
