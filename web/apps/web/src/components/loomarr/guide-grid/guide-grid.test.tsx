import type { GuideAiring, GuideChannelTimeline } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { GuideGrid } from "./guide-grid";

// A fixed window so every assertion can reason in real pixels: 2 hours at 4px/min = 480px.
const FROM = Date.UTC(2026, 6, 25, 20, 0, 0);
const TO = Date.UTC(2026, 6, 25, 22, 0, 0);
const PX_PER_MIN = 4;

const at = (minutes: number) => FROM + minutes * 60_000;

const airing = (
  over: Partial<GuideAiring> & Pick<GuideAiring, "kind" | "startMs" | "stopMs">,
): GuideAiring => ({
  title: "Untitled",
  ...over,
});

const row = (channelId: string, airings: GuideAiring[]): GuideChannelTimeline => ({
  channelId,
  name: channelId,
  number: 1,
  airings,
});

const leftPx = (el: HTMLElement) => Number.parseFloat(el.style.left);
const widthPx = (el: HTMLElement) => Number.parseFloat(el.style.width);

describe("GuideGrid", () => {
  it("sizes a block by its duration, on a shared scale", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        pxPerMinute={PX_PER_MIN}
        channels={[row("ch1", [airing({ kind: "program", title: "Heat", startMs: at(0), stopMs: at(30) })])]}
      />,
    );
    // 30 minutes at 4px/min is 120px. The width IS the duration — that is the whole premise
    // of a time grid, so it is asserted numerically rather than by snapshot.
    expect(widthPx(screen.getByRole("button", { name: /Heat/ }))).toBe(30 * PX_PER_MIN);
  });

  // THE PROPERTY THAT MAKES IT A GRID: two channels airing at the same instant must line up.
  // If rows were scaled independently (percent-of-row, the way a self-contained pod is),
  // 9pm on one row would sit at a different x than 9pm on another and the columns would lie.
  it("aligns the same instant across channels", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        pxPerMinute={PX_PER_MIN}
        channels={[
          row("ch1", [
            airing({ kind: "program", title: "Short", startMs: at(0), stopMs: at(15) }),
            airing({ kind: "program", title: "AtNine", startMs: at(60), stopMs: at(90) }),
          ]),
          row("ch2", [
            airing({ kind: "program", title: "Long", startMs: at(0), stopMs: at(60) }),
            airing({ kind: "program", title: "AlsoAtNine", startMs: at(60), stopMs: at(75) }),
          ]),
        ]}
      />,
    );
    expect(leftPx(screen.getByRole("button", { name: /^AtNine/ }))).toBe(
      leftPx(screen.getByRole("button", { name: /^AlsoAtNine/ })),
    );
  });

  // THE GATE'S REQUIREMENT, at the UI layer: a commercial pod and a pending acquisition must
  // be visually distinct. The API stopped collapsing them to `gap: true` precisely so this
  // surface could tell them apart; rendering them identically would waste that.
  it("renders program, filler and pending distinctly", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        pxPerMinute={PX_PER_MIN}
        channels={[
          row("ch1", [
            airing({ kind: "program", title: "Heat", startMs: at(0), stopMs: at(30) }),
            airing({ kind: "filler", title: "Break", startMs: at(30), stopMs: at(34) }),
            airing({ kind: "pending", title: "Dune 2", nominal: true, startMs: at(34), stopMs: at(64) }),
          ]),
        ]}
      />,
    );
    const classes = ["Heat", "Break", "Dune 2"].map(
      (n) => screen.getByRole("button", { name: new RegExp(n) }).className,
    );
    expect(new Set(classes).size).toBe(3);
  });

  // A nominal block's times are an ESTIMATE. Saying so only in the styling would tell a
  // sighted user and mislead a screen-reader user, so the caveat rides in the accessible name.
  it("tells assistive tech that a pending block's times are estimated", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        pxPerMinute={PX_PER_MIN}
        channels={[
          row("ch1", [
            airing({ kind: "pending", title: "Dune 2", nominal: true, startMs: at(0), stopMs: at(30) }),
          ]),
        ]}
      />,
    );
    expect(screen.getByRole("button", { name: /estimated/i })).toBeInTheDocument();
  });

  // An episode needs BOTH names: the series alone repeats down the row saying nothing about
  // what is on, and the episode alone hides which show it belongs to.
  it("labels an episode with its series and its own title", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        pxPerMinute={PX_PER_MIN}
        channels={[
          row("ch1", [
            airing({
              kind: "program",
              title: "Bart the Mother",
              series: "The Simpsons",
              season: 10,
              episode: 3,
              startMs: at(0),
              stopMs: at(22),
            }),
          ]),
        ]}
      />,
    );
    expect(screen.getByRole("button", { name: /The Simpsons · Bart the Mother/ })).toBeInTheDocument();
    expect(screen.getByText("S10E03")).toBeInTheDocument();
  });

  // A programme already in progress reports its REAL start, which may precede the window.
  // Drawing from that start would push the block off the left edge and misalign its end, so
  // it is clipped to the window while keeping its true right edge.
  it("clips a programme that began before the window", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        pxPerMinute={PX_PER_MIN}
        channels={[
          row("ch1", [airing({ kind: "program", title: "InProgress", startMs: at(-40), stopMs: at(20) })]),
        ]}
      />,
    );
    const block = screen.getByRole("button", { name: /InProgress/ });
    expect(leftPx(block)).toBe(144); // flush to the axis origin, behind the label gutter
    expect(widthPx(block)).toBe(20 * PX_PER_MIN); // only the visible remainder
  });

  it("marks the current instant when it falls inside the window", () => {
    const { rerender } = render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        pxPerMinute={PX_PER_MIN}
        nowMs={at(30)}
        channels={[row("ch1", [])]}
      />,
    );
    expect(screen.getByTestId("guide-now-line")).toBeInTheDocument();

    // Outside the window there is no "now" to draw; a line pinned to an edge would claim
    // the current time is on screen when it is not.
    rerender(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        pxPerMinute={PX_PER_MIN}
        nowMs={at(600)}
        channels={[row("ch1", [])]}
      />,
    );
    expect(screen.queryByTestId("guide-now-line")).not.toBeInTheDocument();
  });

  // Zoom is one number. Doubling px-per-minute must double every block, or zooming would
  // distort the schedule rather than magnify it.
  it("scales the whole grid with zoom", () => {
    const channels = [
      row("ch1", [airing({ kind: "program", title: "Heat", startMs: at(0), stopMs: at(30) })]),
    ];
    const { rerender } = render(<GuideGrid fromMs={FROM} toMs={TO} pxPerMinute={2} channels={channels} />);
    const narrow = widthPx(screen.getByRole("button", { name: /Heat/ }));
    rerender(<GuideGrid fromMs={FROM} toMs={TO} pxPerMinute={4} channels={channels} />);
    expect(widthPx(screen.getByRole("button", { name: /Heat/ }))).toBe(narrow * 2);
  });

  // A channel with nothing scheduled still gets a row: dropping it would read as the channel
  // having been deleted rather than as an empty evening.
  it("keeps a row for a channel with no airings", () => {
    render(<GuideGrid fromMs={FROM} toMs={TO} pxPerMinute={PX_PER_MIN} channels={[row("Quiet", [])]} />);
    expect(screen.getByText("Quiet")).toBeInTheDocument();
  });

  // REGRESSION: the ruler and the blocks must share one origin.
  //
  // They did not. Ticks were positioned at xOf(t) while blocks added the label-gutter width
  // on top, so every tick sat exactly one gutter (144px) left of the programme it named —
  // the "9:00" label pointed at whatever was on at 8:12. Every other test here compared
  // BLOCKS TO BLOCKS, which stayed perfectly consistent with each other while collectively
  // being wrong, so the whole suite passed. This is the one relationship that was unasserted.
  it("puts a tick at the same x as the block starting at that time", () => {
    const { container } = render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        pxPerMinute={PX_PER_MIN}
        // Starts exactly on the 21:00 half-hour boundary, so a tick names precisely it.
        channels={[row("ch1", [airing({ kind: "program", title: "OnTheHour", startMs: at(60), stopMs: at(90) })])]}
      />,
    );
    const block = screen.getByRole("button", { name: /OnTheHour/ });
    // The ruler renders one span per tick; find the one labelled for at(60).
    const label = new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" }).format(
      new Date(at(60)),
    );
    const tick = Array.from(container.querySelectorAll("span")).find((s) => s.textContent === label);
    expect(tick, `no ruler tick labelled ${label}`).toBeDefined();
    expect(leftPx(tick as HTMLElement)).toBe(leftPx(block));
  });
});
