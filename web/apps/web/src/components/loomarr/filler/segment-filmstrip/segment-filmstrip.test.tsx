import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SegmentFilmstrip } from "./segment-filmstrip";
import type { FilmstripSegment } from "./segment-filmstrip.type";

// The reel's detected clips as one time-scaled bar (the v2 mock's `rl.strip`).
//
// ⚠ The assertions are about WIDTH and about what a screen reader hears, because those are the
// two things this component actually delivers. A strip that renders the right number of blocks
// at the wrong widths draws the same picture for a well-split reel and a badly-split one —
// which is exactly the judgement the operator opens it to make.

const seg = (key: string, startMs: number, endMs: number, over: Partial<FilmstripSegment> = {}) =>
  ({ key, startMs, endMs, ...over }) as FilmstripSegment;

const flexOf = (el: HTMLElement) => {
  const li = el.closest("li");
  if (!li) throw new Error("block is not inside a list item");
  return Number.parseFloat(li.style.flex);
};

describe("SegmentFilmstrip", () => {
  it("renders one block per segment, in order", () => {
    render(
      <SegmentFilmstrip
        segments={[seg("a", 0, 10_000, { name: "First" }), seg("b", 10_000, 20_000, { name: "Second" })]}
      />,
    );
    const list = screen.getByRole("list", { name: /detected clips/i });
    expect(within(list).getAllByRole("button")).toHaveLength(2);
  });

  // ⚠ THE property this component exists for. A 45s advert must be visibly wider than a 5s
  // sting; equal blocks would hide a bad split entirely.
  it("sizes each block in proportion to its duration", () => {
    render(
      <SegmentFilmstrip
        segments={[
          seg("short", 0, 10_000, { name: "Short" }), // 10s
          seg("long", 10_000, 40_000, { name: "Long" }), // 30s — 3x
        ]}
      />,
    );
    const short = flexOf(screen.getByRole("button", { name: /Short/ }));
    const long = flexOf(screen.getByRole("button", { name: /Long/ }));
    expect(long).toBeCloseTo(short * 3, 1);
    // And they describe the whole reel between them.
    expect(short + long).toBeCloseTo(100, 1);
  });

  // ⚠ Without a floor, a 0.5s sting inside a 20-minute reel computes to well under a pixel:
  // present in the DOM, impossible to click, invisible. The distortion is deliberate.
  it("keeps a very short segment clickable", () => {
    render(
      <SegmentFilmstrip
        segments={[seg("tiny", 0, 500, { name: "Tiny" }), seg("huge", 500, 1_200_000, { name: "Huge" })]}
      />,
    );
    expect(flexOf(screen.getByRole("button", { name: /Tiny/ }))).toBeGreaterThanOrEqual(0.6);
  });

  it("reports the reel's end as the last segment's end", () => {
    render(<SegmentFilmstrip segments={[seg("a", 0, 65_000, { name: "One" })]} />);
    expect(screen.getByText("01:05")).toBeInTheDocument();
    expect(screen.getByText("00:00")).toBeInTheDocument();
  });

  // ⚠ Blocks are told apart visually by width and position, neither of which reaches a screen
  // reader. Without the timecode in the accessible name every unnamed block reads "unnamed".
  it("names each block with its timecode, not just its name", () => {
    render(<SegmentFilmstrip segments={[seg("a", 65_000, 95_000, { name: "Toy ad" })]} />);
    expect(screen.getByRole("button", { name: "01:05 · Toy ad" })).toBeInTheDocument();
  });

  it("falls back to 'unnamed' rather than an empty accessible name", () => {
    render(<SegmentFilmstrip segments={[seg("a", 0, 5000)]} />);
    expect(screen.getByRole("button", { name: /unnamed/ })).toBeInTheDocument();
  });

  it("reports the clicked block to the caller", async () => {
    const onFocus = vi.fn();
    render(
      <SegmentFilmstrip
        segments={[seg("a", 0, 5000, { name: "One" }), seg("b", 5000, 9000, { name: "Two" })]}
        onFocus={onFocus}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /Two/ }));
    expect(onFocus).toHaveBeenCalledWith("b");
  });

  it("marks the active block with aria-current", () => {
    render(
      <SegmentFilmstrip
        segments={[seg("a", 0, 5000, { name: "One" }), seg("b", 5000, 9000, { name: "Two" })]}
        activeKey="b"
      />,
    );
    expect(screen.getByRole("button", { name: /Two/ })).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("button", { name: /One/ })).not.toHaveAttribute("aria-current");
  });

  // ⚠ An all-dropped reel is a REAL state the editor can reach, and dividing by its zero total
  // is how a timeline becomes a row of NaN-width artefacts.
  it("renders nothing rather than NaN widths when there is no duration", () => {
    const { container } = render(<SegmentFilmstrip segments={[]} />);
    expect(container).toBeEmptyDOMElement();

    const zero = render(<SegmentFilmstrip segments={[seg("a", 1000, 1000)]} />);
    expect(zero.container).toBeEmptyDOMElement();
  });

  // The caption is a PROMISE about what clicking does, and it was false for as long as the strip
  // has existed: it read "click to preview" while clicking has only ever scrolled that segment's
  // row into view (V54 A7).
  //
  // ⚠ **Written as a conditional so it relaxes on its own.** Phase C delivers the real preview,
  // and on that day this component renders a media element and the word "preview" becomes true —
  // at which point the guard stops applying rather than blocking the feature it exists to protect.
  // It is not vacuous today: the component renders no media element, so the caption assertion is
  // live. Sabotage-checked by putting "preview" back.
  it("does not promise a preview it cannot deliver", () => {
    const { container } = render(<SegmentFilmstrip segments={[seg("a", 0, 30_000, { name: "One" })]} />);

    const previews = container.querySelectorAll("video, canvas, img");
    if (previews.length > 0) return; // Phase C landed — the promise is now keepable

    expect(container.textContent ?? "").not.toMatch(/preview/i);
  });

  // The behaviour the caption now describes. Pinned separately, because a caption that matches a
  // behaviour nothing tests is only half a fix.
  it("jumps to a segment's row by reporting the click, and marks it current", async () => {
    const onFocus = vi.fn();
    render(
      <SegmentFilmstrip
        segments={[seg("a", 0, 30_000, { name: "One" }), seg("b", 30_000, 61_000, { name: "Two" })]}
        activeKey="b"
        onFocus={onFocus}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /One/ }));

    expect(onFocus).toHaveBeenCalledWith("a");
    // The strip does not own the scroll — it reports, and the editor scrolls the row. What the
    // strip itself must show is WHICH block the editor considers current.
    expect(screen.getByRole("button", { name: /Two/ })).toHaveAttribute("aria-current", "true");
  });
});
