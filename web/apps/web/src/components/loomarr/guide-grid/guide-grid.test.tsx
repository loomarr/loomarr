import type { GuideAiring, GuideChannelTimeline } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { GuideGrid } from "./guide-grid";

// A fixed 2-hour window, so every assertion can reason in percentages: 30 minutes = 25%.
const FROM = Date.UTC(2026, 6, 25, 20, 0, 0);
const TO = Date.UTC(2026, 6, 25, 22, 0, 0);

const at = (minutes: number) => FROM + minutes * 60_000;

const airing = (
  over: Partial<GuideAiring> & Pick<GuideAiring, "kind" | "startMs" | "stopMs">,
): GuideAiring => ({ title: "Untitled", ...over });

const row = (channelId: string, airings: GuideAiring[]): GuideChannelTimeline => ({
  channelId,
  name: channelId,
  number: 1,
  airings,
});

const leftPct = (el: HTMLElement) => Number.parseFloat(el.style.left);
const widthPct = (el: HTMLElement) => Number.parseFloat(el.style.width);
const block = (name: RegExp) => screen.getByRole("button", { name });

describe("GuideGrid", () => {
  it("sizes a block by its share of the window", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        channels={[row("ch1", [airing({ kind: "program", title: "Heat", startMs: at(0), stopMs: at(30) })])]}
      />,
    );
    // 30 of 120 minutes = 25% of the row. The width IS the duration — the premise of a time
    // grid — so it is asserted numerically rather than by snapshot.
    expect(widthPct(block(/Heat/))).toBeCloseTo(25, 1);
  });

  // THE PROPERTY THAT MAKES IT A GRID: two channels airing at the same instant must line up.
  // Scaling rows independently would let 9pm sit at different places per row.
  it("aligns the same instant across channels", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
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
    expect(leftPct(block(/^AtNine/))).toBeCloseTo(leftPct(block(/^AlsoAtNine/)), 3);
  });

  // REGRESSION: the ruler and the blocks must share one coordinate space.
  //
  // They did not, in the absolute-pixel version: ticks omitted the rail width that blocks
  // added, so every tick sat one rail-width left of the programme it named — "9:00" pointed at
  // whatever was on at 8:12. Every other test compared BLOCKS TO BLOCKS and stayed perfectly
  // consistent while collectively being wrong. The flex+percentage layout now makes the two
  // structurally the same space; this asserts it stays that way.
  it("puts a tick at the same offset as the block starting at that time", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        channels={[
          row("ch1", [airing({ kind: "program", title: "OnTheHour", startMs: at(60), stopMs: at(90) })]),
        ]}
      />,
    );
    const ticks = screen.getAllByTestId("guide-tick");
    const at21 = ticks.find((t) => Number.parseFloat(t.dataset.tickPct ?? "-1") === 50);
    expect(at21, "no ruler tick at the 50% mark (21:00)").toBeDefined();
    expect(leftPct(block(/OnTheHour/))).toBeCloseTo(50, 3);
  });

  // THE GATE'S REQUIREMENT at the UI layer: a commercial pod and a pending acquisition must be
  // visually distinct. The API stopped collapsing them to `gap: true` so this surface could
  // tell them apart; rendering them identically would waste that.
  it("renders each kind distinctly", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        channels={[
          row("ch1", [
            airing({ kind: "program", title: "Heat", startMs: at(0), stopMs: at(30) }),
            airing({ kind: "filler", title: "Break", startMs: at(30), stopMs: at(34) }),
            airing({ kind: "pending", title: "Dune 2", nominal: true, startMs: at(34), stopMs: at(64) }),
            airing({ kind: "flex", title: "", startMs: at(64), stopMs: at(70) }),
          ]),
        ]}
      />,
    );
    const kinds = screen
      .getAllByRole("button")
      .map((b) => b.dataset.kind)
      .filter(Boolean);
    expect(new Set(kinds)).toEqual(new Set(["program", "filler", "pending", "flex"]));
  });

  // A nominal block's times are an ESTIMATE. Saying so only in the styling would inform a
  // sighted user and mislead a screen-reader user.
  it("tells assistive tech that a pending block's times are estimated", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        channels={[
          row("ch1", [
            airing({ kind: "pending", title: "Dune 2", nominal: true, startMs: at(0), stopMs: at(30) }),
          ]),
        ]}
      />,
    );
    expect(screen.getByRole("button", { name: /estimated/i })).toBeInTheDocument();
  });

  // What is on RIGHT NOW is the question a guide is opened to answer, so it is marked rather
  // than left for the viewer to work out from the now-line's position.
  it("marks the programme that is airing now", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        nowMs={at(15)}
        channels={[
          row("ch1", [
            airing({ kind: "program", title: "OnNow", startMs: at(0), stopMs: at(30) }),
            airing({ kind: "program", title: "Later", startMs: at(30), stopMs: at(60) }),
          ]),
        ]}
      />,
    );
    expect(block(/OnNow/).dataset.airing).toBe("true");
    expect(block(/Later/).dataset.airing).toBeUndefined();
  });

  // A break renders its CLIPS proportionally: bumper → ads → bumper is a sequence, and one
  // flat rectangle hides what is actually playing.
  it("renders a pod's clips inside the break", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        channels={[
          row("ch1", [
            airing({
              kind: "filler",
              title: "Break",
              startMs: at(0),
              stopMs: at(30),
              pod: {
                matchLevel: "exact",
                totalMs: 65000,
                entries: [
                  { name: "Channel bumper", kind: "bumper", durationMs: 5000, isFallbackCard: false },
                  { name: "Sunny D", kind: "commercial", durationMs: 30000, isFallbackCard: false },
                  { name: "Gushers", kind: "commercial", durationMs: 30000, isFallbackCard: false },
                ],
              },
            }),
          ]),
        ]}
      />,
    );
    // The two 30s clips are 6x the 5s bumper — proportional, not equal thirds.
    expect(screen.getByText("Sunny D")).toBeInTheDocument();
    expect(screen.getByText("Gushers")).toBeInTheDocument();
  });

  // A programme already in progress reports its REAL start, which may precede the window;
  // drawing from there would push it off the left edge and misalign its end.
  it("clips a programme that began before the window", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        channels={[
          row("ch1", [airing({ kind: "program", title: "InProgress", startMs: at(-40), stopMs: at(20) })]),
        ]}
      />,
    );
    expect(leftPct(block(/InProgress/))).toBe(0);
    // Only the visible remainder: 20 of 120 minutes.
    expect(widthPct(block(/InProgress/))).toBeCloseTo(16.67, 1);
  });

  it("marks the current instant only when it falls inside the window", () => {
    const channels = [row("ch1", [])];
    const { rerender } = render(<GuideGrid fromMs={FROM} toMs={TO} nowMs={at(30)} channels={channels} />);
    expect(screen.getByTestId("guide-now-line")).toBeInTheDocument();

    // Outside the window there is no "now" to draw; a line pinned to an edge would claim the
    // current time is on screen when it is not.
    rerender(<GuideGrid fromMs={FROM} toMs={TO} nowMs={at(600)} channels={channels} />);
    expect(screen.queryByTestId("guide-now-line")).not.toBeInTheDocument();
  });

  // Zoom scales the CHROME, not the time scale: the window still fits, rows just get taller
  // and more legible. A block's share of the window must therefore not change.
  it("keeps block proportions constant across zoom", () => {
    const channels = [
      row("ch1", [airing({ kind: "program", title: "Heat", startMs: at(0), stopMs: at(30) })]),
    ];
    const { rerender } = render(<GuideGrid fromMs={FROM} toMs={TO} zoom={1} channels={channels} />);
    const base = widthPct(block(/Heat/));
    rerender(<GuideGrid fromMs={FROM} toMs={TO} zoom={1.6} channels={channels} />);
    expect(widthPct(block(/Heat/))).toBeCloseTo(base, 3);
  });

  // Hovering a block is how the detail card gets its subject.
  it("reports the hovered airing to the caller", async () => {
    const seen: (GuideAiring | null)[] = [];
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        onInspect={(a) => seen.push(a)}
        channels={[row("ch1", [airing({ kind: "program", title: "Heat", startMs: at(0), stopMs: at(30) })])]}
      />,
    );
    block(/Heat/).focus();
    expect(seen.at(-1)?.title).toBe("Heat");
    block(/Heat/).blur();
    expect(seen.at(-1)).toBeNull();
  });

  // A block too narrow for its label shows NONE of it rather than a truncated fragment. A
  // 4-minute break across a 2-hour window rendered "Commercials" clipped to the single letter
  // "C", which reads as a rendering glitch and invites the viewer to wonder what it means.
  //
  // The block itself stays — dropping it would leave a hole every later block slides into.
  it("hides a label too narrow to read rather than truncating it to a fragment", () => {
    render(
      <GuideGrid
        fromMs={FROM}
        toMs={TO}
        channels={[
          row("ch1", [
            airing({ kind: "program", title: "Heat", startMs: at(0), stopMs: at(60) }),
            // 4 minutes of 120 = 3.3% of the row.
            airing({ kind: "filler", title: "Commercials", startMs: at(60), stopMs: at(64) }),
          ]),
        ]}
      />,
    );
    // Still on the timeline, and still nameable to assistive tech via its accessible label…
    const narrow = screen.getByRole("button", { name: /Commercials/ });
    expect(narrow).toBeInTheDocument();
    // …but showing no visible text, rather than a one-letter fragment of it.
    expect(narrow.textContent).toBe("");
  });

  // A channel with nothing scheduled still gets a row: dropping it would read as the channel
  // having been deleted rather than as an empty evening.
  it("keeps a row for a channel with no airings", () => {
    render(<GuideGrid fromMs={FROM} toMs={TO} channels={[row("Quiet", [])]} />);
    expect(screen.getByText("Quiet")).toBeInTheDocument();
  });
});
