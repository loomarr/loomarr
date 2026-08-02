import type { DiscoveredClip } from "@loomarr/api";
import { discoveredClips } from "@loomarr/fixtures";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SourceSearch } from "./source-search";

const noop = () => {};
const base = {
  results: discoveredClips,
  query: "cereal",
  onQueryChange: noop,
  onSearch: noop,
  onQueue: noop,
};

describe("SourceSearch", () => {
  // The mock's row: `date · duration · quality`. Only `date` is free — the other two cost a
  // per-item metadata call — so this asserts they actually arrive rather than that a line renders.
  it("renders date, duration and quality for a fully-described result", () => {
    render(<SourceSearch {...base} />);

    // 1989 · 10.8s · 720p — the year alone, not the full RFC3339 timestamp.
    expect(screen.getByText("1989 · 11s · 720p")).toBeInTheDocument();
  });

  // ⚠ The load-bearing one. Archive.org has not probed every item, so absent duration/height
  // must render as NOTHING — "0:00" or "0p" would claim the clip is empty or has no picture.
  //
  // Scoped to the ROW that knows only its date. A page-wide regex is what a first cut used, and
  // it matched "720p" and "11s" on the fully-described rows — passing or failing for reasons
  // having nothing to do with the property.
  it("omits duration and quality it does not know, rather than showing zero", () => {
    render(<SourceSearch {...base} />);

    const row = screen.getByText("1980s cereal commercial compilation").closest("li");
    expect(row).not.toBeNull();
    // The meta line is the year and nothing else — no "0s", no "0p", no stray separator.
    expect(row?.textContent).toContain("2015");
    expect(row?.textContent).not.toMatch(/\b0s\b|\b0p\b|0:00/);
    expect(row?.textContent).not.toContain("2015 ·");
  });

  // A compilation runs to hours, where the clip formatter would read "183m 52s".
  it("reads long compilations in hours, not hundreds of minutes", () => {
    render(<SourceSearch {...base} />);

    expect(screen.getByText(/3h 4m/)).toBeInTheDocument();
    expect(screen.queryByText(/183m/)).not.toBeInTheDocument();
  });

  // A row with no date at all still renders its other facts — every part is independently optional.
  it("renders a row that has no date", () => {
    render(<SourceSearch {...base} />);

    const row = screen.getByText("Saturday Morning Cartoons Vol. 110 (full block)").closest("li");
    expect(row).not.toBeNull();
    expect(row?.textContent).toContain("360p");
  });

  // ⚠ Searching downloads nothing; queueing is the only path that fetches. The footnote is a
  // behaviour claim, so it is asserted rather than left as decoration.
  it("says that nothing downloads until you queue it", () => {
    render(<SourceSearch {...base} />);

    expect(screen.getByText(/nothing downloads until you queue it/i)).toBeInTheDocument();
  });

  it("queues the clip that was clicked", async () => {
    const onQueue = vi.fn();
    render(<SourceSearch {...base} onQueue={onQueue} />);

    await userEvent.click(screen.getByRole("button", { name: /Queue download: Kellogg's Bran Flakes/ }));

    expect(onQueue).toHaveBeenCalledTimes(1);
    expect(onQueue.mock.calls[0]?.[0].id).toBe("kelloggs-bran-flakes");
  });

  // ⚠ Every row's button would otherwise be named "Queue download", so a screen-reader user
  // hears an undifferentiated list and cannot tell which one they are on.
  it("names what each queue button acts on", () => {
    render(<SourceSearch {...base} />);

    const names = screen
      .getAllByRole("button", { name: /Queue download:/ })
      .map((b) => b.getAttribute("aria-label"));
    expect(new Set(names).size).toBe(names.length);
  });

  // A queued row reports an OUTCOME. A disabled "Queue download" would invite hunting for why
  // the button stopped working.
  it("reports a queued row rather than disabling its button", () => {
    render(<SourceSearch {...base} queued={["kelloggs-bran-flakes"]} />);

    expect(screen.getByText("queued")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Queue download: Kellogg's Bran Flakes/ }),
    ).not.toBeInTheDocument();
  });

  // Enter must run the search: needing the mouse for a search box is what makes a surface feel
  // broken, and it is a form precisely so this works.
  it("searches on Enter", async () => {
    const onSearch = vi.fn();
    render(<SourceSearch {...base} onSearch={onSearch} />);

    await userEvent.type(screen.getByRole("textbox", { name: "Search this source" }), "{Enter}");

    expect(onSearch).toHaveBeenCalledTimes(1);
  });

  // ⚠ `total` is the FULL match count, not the page. Reporting results.length would tell an
  // operator a 3000-match search found 25 — and that number is what they judge it by.
  it("reports the full match count when the page is capped", () => {
    const results = Array.from(
      { length: 25 },
      (_, i): DiscoveredClip => ({
        ...(discoveredClips[0] as DiscoveredClip),
        id: `clip-${i}`,
        title: `Clip ${i}`,
      }),
    );
    render(<SourceSearch {...base} results={results} total={3000} />);

    expect(screen.getByText("Showing 25 of 3000 matches")).toBeInTheDocument();
  });

  it("does not search on a query too short to be useful", () => {
    render(<SourceSearch {...base} query="a" />);

    expect(screen.getByRole("button", { name: /Search/ })).toBeDisabled();
  });

  // --- stats arriving late (V35) ---

  // ⚠ The reason this component reports visibility at all: a search omits duration and quality
  // because each one is a ~1.8s upstream call, so the caller fetches them for rows on screen.
  // jsdom has no IntersectionObserver, so this also covers the documented fallback.
  it("reports its rows so the caller can fetch their stats", () => {
    const onVisible = vi.fn();
    render(<SourceSearch {...base} onVisible={onVisible} />);

    expect(onVisible).toHaveBeenCalled();
    const reported = onVisible.mock.calls.at(-1)?.[0];
    expect(reported).toEqual(expect.arrayContaining(discoveredClips.map((c) => c.id)));
  });

  // The whole point of the split: a row paints from the search, then gains its runtime.
  it("renders a runtime that arrived after the search", () => {
    const clip = discoveredClips[2] as DiscoveredClip; // the row that knows only its date
    render(
      <SourceSearch {...base} results={[clip]} stats={{ [clip.id]: { durationMs: 45_000, height: 480 } }} />,
    );

    expect(screen.getByText("2015 · 45s · 480p")).toBeInTheDocument();
  });

  // ⚠ "checking…" is for a request IN FLIGHT. Once it answers, an unknown duration renders as
  // nothing — a permanent spinner promises a value that is never coming.
  it("says it is checking only while the request is in flight", () => {
    const clip = discoveredClips[2] as DiscoveredClip;
    const { rerender } = render(<SourceSearch {...base} results={[clip]} loadingStats={[clip.id]} />);
    expect(screen.getByText(/checking…/)).toBeInTheDocument();

    // It answered, and knew nothing. The row must NOT keep saying "checking…".
    rerender(<SourceSearch {...base} results={[clip]} stats={{}} loadingStats={[]} />);
    expect(screen.queryByText(/checking…/)).not.toBeInTheDocument();
    // ⚠ Scoped to the row, and word-bounded: the TITLE contains "1980s", so a loose /0s/
    // matches it and the assertion fails for a reason having nothing to do with durations.
    // (Second time this exact regex bit — the fixture titles are full of years.)
    const row = screen.getByText("1980s cereal commercial compilation").closest("li");
    expect(row?.textContent).not.toMatch(/\b0s\b|0:00/);
  });

  // ⚠ A late 0 is still UNKNOWN. The server omits ids it could not answer for, but a client
  // must not render a zero it somehow receives as "0:00" either.
  it("does not render a zero runtime that arrives in stats", () => {
    const clip = discoveredClips[2] as DiscoveredClip;
    render(<SourceSearch {...base} results={[clip]} stats={{ [clip.id]: { durationMs: 0, height: 0 } }} />);

    const row = screen.getByText("1980s cereal commercial compilation").closest("li");
    expect(row?.textContent).not.toMatch(/\b0s\b|\b0p\b|0:00/);
  });
});
