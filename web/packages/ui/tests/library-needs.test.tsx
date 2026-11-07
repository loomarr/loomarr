// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  type LibraryNeed,
  LibraryNeedsCards,
  LibraryNeedsStrip,
  type LibraryNeedsStripProps,
  type LibraryProblem,
} from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type El = {
  click(): void;
  getAttribute(n: string): string | null;
  textContent: string | null;
};
type Host = Parameters<typeof createRoot>[0] & {
  querySelectorAll(s: string): ArrayLike<El>;
  remove(): void;
  textContent: string | null;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Host };
  }
).document;

const guess = (over: Partial<NonNullable<LibraryNeed["best"]>> = {}) => ({
  tmdbId: "90000901",
  kind: "movie" as const,
  name: "Paper Kites",
  year: 1996,
  nameMatches: true,
  yearMatches: false,
  lengthMatches: true,
  ...over,
});

const need = (id: string, over: Partial<LibraryNeed> = {}): LibraryNeed => ({
  id,
  kind: "movie",
  files: [`/media/movies/${id}/${id}.mkv`],
  best: guess(),
  others: [],
  ...over,
});

const mount = (ui: ReactElement) => {
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  const show = (next: ReactElement) =>
    act(() => root.render(<LoomarrProvider theme="dark">{next}</LoomarrProvider>));
  show(ui);
  const button = (name: string) => {
    const hit = Array.from(host.querySelectorAll('[role="button"], button')).find(
      (n) => n.textContent?.trim() === name || n.getAttribute("aria-label") === name,
    );
    if (!hit) throw new Error(`missing button: ${name}`);
    return hit;
  };
  const cleanup = () => {
    act(() => root.unmount());
    host.remove();
  };
  return { host, show, button, cleanup };
};

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

const problem = (id: string, over: Partial<LibraryProblem> = {}): LibraryProblem => ({
  id,
  kind: "movie",
  name: "Quiet Orchard",
  unplayable: 0,
  extraBytes: 0,
  ...over,
});

const strip = (over: Partial<LibraryNeedsStripProps> = {}) => (
  <LibraryNeedsStrip
    needs={[]}
    onLink={vi.fn()}
    onOpenNeeds={vi.fn()}
    onOpenTitle={vi.fn()}
    problems={[]}
    {...over}
  />
);

describe("LibraryNeedsStrip", () => {
  it("is nothing when nothing needs you", () => {
    const { host, cleanup } = mount(strip());
    expect(host.textContent).toBe("");
    cleanup();
  });

  it("lists the titles that need a look after the questions, and opens one", () => {
    const onOpenTitle = vi.fn();
    const { host, button, cleanup } = mount(
      strip({
        needs: [need("90000811")],
        onOpenTitle,
        problems: [
          problem("90000821", { unplayable: 1 }),
          problem("90000822", { kind: "series", name: "Night Ferry", unplayable: 3 }),
        ],
      }),
    );
    expect(host.textContent).toContain("Needs you (3)");
    const text = host.textContent ?? "";
    expect(text.indexOf("is this the 1996 movie?")).toBeLessThan(
      text.indexOf("Quiet Orchard — can't be played"),
    );
    expect(text).toContain("Night Ferry — 3 episodes can't be played");
    act(() => button("See why").click());
    expect(onOpenTitle).toHaveBeenCalledWith("90000821");
    cleanup();
  });

  it("asks about the first four and offers the rest", () => {
    const needs = ["90000811", "90000812", "90000813", "90000814", "90000815", "90000816"].map((id) =>
      need(id),
    );
    const { host, button, cleanup } = mount(strip({ needs }));
    expect(host.textContent).toContain("Needs you (6)");
    expect(host.textContent).toContain("Paper Kites — is this the 1996 movie?");
    act(() => button("Show 2 more").click());
    expect(host.textContent).not.toContain("Show 2 more");
    cleanup();
  });

  it("links the guess, says it's sorted, and lets it go", async () => {
    const onLink = vi.fn(() => Promise.resolve());
    const needs = [need("90000811")];
    const { host, show, button, cleanup } = mount(strip({ needs, onLink }));
    act(() => button("Yes, that's it").click());
    expect(onLink).toHaveBeenCalledWith("90000811", "90000901");
    // The refetch no longer holds it; the answered row stays until it has been seen.
    show(strip({ onLink }));
    expect(host.textContent).toContain("Done — Paper Kites is sorted.");
    expect(host.textContent).toContain("Needs you (0)");
    await act(async () => {
      vi.advanceTimersByTime(1500);
    });
    expect(host.textContent).toBe("");
    cleanup();
  });

  it("puts the question back when the answer fails", async () => {
    const onLink = vi.fn(() => Promise.reject(new Error("no")));
    const { host, button, cleanup } = mount(strip({ needs: [need("90000811")], onLink }));
    await act(async () => button("Yes, that's it").click());
    expect(host.textContent).toContain("Paper Kites — is this the 1996 movie?");
    cleanup();
  });

  it("sends a title with no guess to Needs you", () => {
    const onOpenNeeds = vi.fn();
    const { host, button, cleanup } = mount(
      strip({
        needs: [need("90000812", { best: undefined, files: ["/media/movies/disc2/title_t01.mkv"] })],
        onOpenNeeds,
      }),
    );
    expect(host.textContent).toContain("title_t01.mkv — we're not sure what this is");
    act(() => button("Find it").click());
    expect(onOpenNeeds).toHaveBeenCalled();
    cleanup();
  });
});

describe("LibraryNeedsCards", () => {
  const cards = (over: Partial<Parameters<typeof LibraryNeedsCards>[0]> = {}) => (
    <LibraryNeedsCards
      needs={[need("90000811")]}
      onIgnore={vi.fn(() => Promise.resolve())}
      onLink={vi.fn(() => Promise.resolve())}
      onOpenTitle={vi.fn()}
      onSearch={vi.fn(() => Promise.resolve([]))}
      problems={[]}
      {...over}
    />
  );

  it("words what's wrong with a title that needs a look, and opens it", () => {
    const onOpenTitle = vi.fn();
    const { host, button, cleanup } = mount(
      cards({
        needs: [],
        onOpenTitle,
        problems: [
          problem("90000821", { kind: "series", name: "Night Ferry", unplayable: 1 }),
          problem("90000822", { extraBytes: 4_300_000_000 }),
        ],
      }),
    );
    expect(host.textContent).toContain("Needs you (2)");
    expect(host.textContent).toContain("Night Ferry has an episode that can't be played");
    expect(host.textContent).toContain("The file may be damaged. Loomarr skips it until it's fixed.");
    expect(host.textContent).toContain("Quiet Orchard has an extra copy");
    expect(host.textContent).toMatch(/The extra copy is taking .+ and never plays\./);
    act(() => button("See why").click());
    expect(onOpenTitle).toHaveBeenCalledWith("90000821");
    cleanup();
  });

  it("says why it guessed", () => {
    const { host, cleanup } = mount(cards());
    expect(host.textContent).toContain("Is this Paper Kites (1996)?");
    expect(host.textContent).toContain("The name and length match; the year doesn't.");
    expect(host.textContent).toContain("1 file");
    cleanup();
  });

  it("says when nothing needs you", () => {
    const { host, cleanup } = mount(cards({ needs: [] }));
    expect(host.textContent).toContain("Nothing needs you.");
    cleanup();
  });

  it("ignores a title", () => {
    const onIgnore = vi.fn(() => Promise.resolve());
    const { host, button, cleanup } = mount(cards({ onIgnore }));
    act(() => button("Ignore this").click());
    expect(onIgnore).toHaveBeenCalledWith("90000811");
    expect(host.textContent).toContain("Okay, we'll leave it out.");
    cleanup();
  });

  it("picks one of the other guesses", () => {
    const onLink = vi.fn(() => Promise.resolve());
    const other = guess({ tmdbId: "90000902", year: 2014, lengthMatches: false });
    const { host, button, cleanup } = mount(
      cards({ onLink, needs: [need("90000811", { others: [other] })] }),
    );
    act(() => button("Pick a different title").click());
    expect(host.textContent).toContain("Paper Kites (2014)");
    act(() => button("Use this one").click());
    expect(onLink).toHaveBeenCalledWith("90000811", "90000902");
    expect(host.textContent).toContain("Added to your library.");
    cleanup();
  });
});
