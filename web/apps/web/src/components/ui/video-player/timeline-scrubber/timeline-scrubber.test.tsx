import type { GuideAiring } from "@loomarr/api";
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TimelineScrubber } from "./timeline-scrubber";

const NOW = 1_700_000_000_000;
const min = (n: number) => n * 60_000;

const airings: GuideAiring[] = [
  {
    kind: "program",
    series: "The Simpsons",
    title: "Ep A",
    season: 1,
    episode: 1,
    startMs: NOW - min(5),
    stopMs: NOW + min(15),
  },
  { kind: "filler", title: "", startMs: NOW + min(15), stopMs: NOW + min(17) },
  {
    kind: "program",
    series: "The Simpsons",
    title: "Ep B",
    season: 1,
    episode: 2,
    startMs: NOW + min(17),
    stopMs: NOW + min(37),
  },
];

describe("TimelineScrubber", () => {
  it("renders one segment per airing block", () => {
    const { container } = render(<TimelineScrubber airings={airings} nowMs={NOW} />);
    // The track is the group; its direct child segments are one per block.
    const track = container.querySelector(".group");
    expect(track).not.toBeNull();
    // Each block is a positioned div with an inline width; the playhead is the extra absolute tick.
    const segments = track?.querySelectorAll(":scope > div[style*='width']");
    expect(segments?.length).toBe(airings.length);
  });

  it("renders nothing when there are no airings", () => {
    const { container } = render(<TimelineScrubber airings={[]} nowMs={NOW} />);
    expect(container.firstChild).toBeNull();
  });

  // ⚠ This is the case the test above CANNOT cover, and the distinction is the whole point.
  // Mounting with `[]` renders the component once, one way — so it never sees a change in hook
  // count. The empty-airings guard is an early `return null`, and the hover card's anchor is a
  // `useMemo`: put that hook below the guard and the component runs one fewer hook on an empty
  // render than on a populated one. React matches hooks by CALL ORDER, so it throws
  // "Rendered fewer hooks than expected" — but only when one instance renders both ways, which
  // is exactly what a channel does when its guide data empties out.
  it("survives an airings list that empties out (hook order is stable across the guard)", () => {
    const { container, rerender } = render(<TimelineScrubber airings={airings} nowMs={NOW} />);
    expect(container.querySelector(".group")).not.toBeNull();
    rerender(<TimelineScrubber airings={[]} nowMs={NOW} />);
    expect(container.firstChild).toBeNull();
    // And back again — the populated render must still mount cleanly afterwards.
    rerender(<TimelineScrubber airings={airings} nowMs={NOW} />);
    expect(container.querySelector(".group")).not.toBeNull();
  });

  it("positions the live playhead within the strip", () => {
    const { container } = render(<TimelineScrubber airings={airings} nowMs={NOW} />);
    // The playhead is the absolutely-positioned tick with a left percentage.
    const playhead = container.querySelector("[aria-hidden][style*='left']");
    expect(playhead).not.toBeNull();
  });
});
