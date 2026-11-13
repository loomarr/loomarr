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
  type LibraryOrderNeed,
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
  externalId: { namespace: "tmdb", value: "90000901" },
  kind: "movie" as const,
  name: "Paper Kites",
  year: 1996,
  nameMatches: true,
  yearMatches: false,
  lengthMatches: true,
  episodesMatches: false,
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

// A show TMDB lists in several orders, the DVD one fitting the disk; and one TMDB lists one way,
// a season on disk running past it.
const tideFiles = [6, 7, 8, 9, 10].map((episode) => ({
  path: `/media/tv/Tidewater/Tidewater.S02E${String(episode).padStart(2, "0")}.mkv`,
  season: 2,
  episode,
}));
const tidewater: LibraryOrderNeed = {
  id: "90000831",
  name: "Tidewater",
  disk: [
    { season: 1, episodes: 12 },
    { season: 2, episodes: 14 },
  ],
  files: tideFiles,
  choices: [
    {
      choice: "catalog",
      seasons: [
        { season: 0, episodes: 2 },
        { season: 1, episodes: 13 },
        { season: 2, episodes: 13 },
      ],
      episodes: 28,
      fits: false,
    },
    {
      choice: "order",
      orderId: "90000799a",
      kind: "dvd",
      name: "DVD Order",
      seasons: [
        { season: 1, episodes: 12 },
        { season: 2, episodes: 14 },
      ],
      episodes: 26,
      fits: true,
    },
    {
      choice: "order",
      orderId: "90000799b",
      kind: "absolute",
      name: "Absolute",
      seasons: [],
      episodes: 26,
      fits: false,
    },
  ],
};
const kestrel: LibraryOrderNeed = {
  id: "90000832",
  name: "Kestrel Bay",
  disk: [
    { season: 1, episodes: 10 },
    { season: 2, episodes: 14 },
  ],
  files: [{ path: "/media/tv/Kestrel Bay/Kestrel.Bay.S02E14.mkv", season: 2, episode: 14 }],
  choices: [
    {
      choice: "catalog",
      seasons: [
        { season: 1, episodes: 10 },
        { season: 2, episodes: 13 },
      ],
      episodes: 23,
      fits: false,
    },
  ],
};

const strip = (over: Partial<LibraryNeedsStripProps> = {}) => (
  <LibraryNeedsStrip
    needs={[]}
    onLink={vi.fn()}
    onOpenNeeds={vi.fn()}
    onOpenTitle={vi.fn()}
    orders={[]}
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
    expect(onLink).toHaveBeenCalledWith("90000811", { namespace: "tmdb", value: "90000901" });
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

  it("puts the question back when the answer fails, and says it wasn't saved", async () => {
    const onLink = vi.fn(() => Promise.reject(new Error("no")));
    const { host, button, cleanup } = mount(strip({ needs: [need("90000811")], onLink }));
    await act(async () => button("Yes, that's it").click());
    expect(host.textContent).toContain("Paper Kites — is this the 1996 movie?");
    expect(host.textContent).toContain("Couldn't save your answer. Try again in a moment.");
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

  // The question has more than two answers, so the strip only opens Needs you.
  it("asks about a show's episode order first, and opens Needs you", () => {
    const onOpenNeeds = vi.fn();
    const { host, button, cleanup } = mount(
      strip({ needs: [need("90000811")], orders: [tidewater, kestrel], onOpenNeeds }),
    );
    const text = host.textContent ?? "";
    expect(text).toContain("Needs you (3)");
    expect(text).toContain("Tidewater — which order are the episodes in?");
    expect(text).toContain("Kestrel Bay — 1 episode keeps its file name");
    expect(text.indexOf("Tidewater")).toBeLessThan(text.indexOf("Paper Kites"));
    act(() => button("Choose").click());
    expect(onOpenNeeds).toHaveBeenCalledTimes(1);
    act(() => button("See why").click());
    expect(onOpenNeeds).toHaveBeenCalledTimes(2);
    cleanup();
  });
});

describe("LibraryNeedsCards", () => {
  const cards = (over: Partial<Parameters<typeof LibraryNeedsCards>[0]> = {}) => (
    <LibraryNeedsCards
      needs={[need("90000811")]}
      onChooseOrder={vi.fn(() => Promise.resolve())}
      onIgnore={vi.fn(() => Promise.resolve())}
      onLink={vi.fn(() => Promise.resolve())}
      onOpenTitle={vi.fn()}
      onSearch={vi.fn(() => Promise.resolve([]))}
      orders={[]}
      problems={[]}
      {...over}
    />
  );

  it("asks which order a show's episodes are in, the one that fits chosen first", () => {
    const onChooseOrder = vi.fn(() => Promise.resolve());
    const { host, button, cleanup } = mount(cards({ orders: [tidewater], onChooseOrder }));
    const text = host.textContent ?? "";
    expect(text).toContain("Needs you (2)");
    expect(text).toContain("Which order are your Tidewater episodes in?");
    expect(text).toContain("Until you choose, 5 episodes use the name and number in their file.");
    // Plain names by kind; specials don't count as a season.
    expect(text).toContain("As first aired2 seasons · 28 episodes");
    expect(text).toContain("DVD orderFits your files2 seasons · 26 episodes");
    expect(text).toContain("One long run26 episodes, no seasons");
    expect(text).toContain("Keep the file names");
    const radios = Array.from(host.querySelectorAll('[role="radio"]'));
    expect(radios.map((r) => r.getAttribute("aria-checked"))).toEqual(["false", "true", "false", "false"]);
    // The title card below isn't primary: only the first card is.
    act(() => button("Use this order").click());
    expect(onChooseOrder).toHaveBeenCalledWith("90000831", { choice: "order", orderId: "90000799a" });
    expect(host.textContent).toContain("Done — Tidewater uses DVD order.");
    expect(host.textContent).toContain("5 episodes now have their names and details from TMDB.");
    cleanup();
  });

  it("keeps the file names when the household says so", () => {
    const onChooseOrder = vi.fn(() => Promise.resolve());
    const { host, button, cleanup } = mount(cards({ needs: [], orders: [tidewater], onChooseOrder }));
    const keep = Array.from(host.querySelectorAll('[role="radio"]')).find(
      (r) => r.getAttribute("aria-label") === "Keep the file names",
    );
    act(() => keep?.click());
    act(() => button("Keep the file names").click());
    expect(onChooseOrder).toHaveBeenCalledWith("90000831", { choice: "files" });
    expect(host.textContent).toContain("Done — Tidewater keeps the names in its files.");
    expect(host.textContent).not.toContain("from TMDB");
    cleanup();
  });

  it("lists the files that wait, numbered as they say, the first three then a count", () => {
    const { host, cleanup } = mount(cards({ needs: [], orders: [tidewater] }));
    expect(host.textContent).toContain("5 episodes");
    act(() => host.querySelectorAll("[aria-expanded]")[0]?.click());
    expect(host.textContent).toContain("S02E06 — Tidewater.S02E06.mkv");
    expect(host.textContent).toContain("S02E08 — Tidewater.S02E08.mkv");
    expect(host.textContent).not.toContain("S02E09 —");
    expect(host.textContent).toContain("and 2 more");
    cleanup();
  });

  // One order: nothing to choose. That's fine keeps the files' names; See the show opens it.
  it("says where a show's one order doesn't line up", () => {
    const onChooseOrder = vi.fn(() => Promise.resolve());
    const onOpenTitle = vi.fn();
    const { host, button, cleanup } = mount(
      cards({ needs: [], orders: [kestrel], onChooseOrder, onOpenTitle }),
    );
    expect(host.textContent).toContain("Some Kestrel Bay episodes don't line up with TMDB");
    expect(host.textContent).toContain(
      "Season 2 on your disk has 14 episodes; TMDB lists 13. It still plays, named as it is in its file.",
    );
    expect(host.querySelectorAll('[role="radio"]')).toHaveLength(0);
    act(() => button("See the show").click());
    expect(onOpenTitle).toHaveBeenCalledWith("90000832");
    act(() => button("That's fine").click());
    expect(onChooseOrder).toHaveBeenCalledWith("90000832", { choice: "files" });
    cleanup();
  });

  it("puts the order question back when the answer fails", async () => {
    const onChooseOrder = vi.fn(() => Promise.reject(new Error("no")));
    const { host, button, cleanup } = mount(cards({ needs: [], orders: [tidewater], onChooseOrder }));
    await act(async () => button("Use this order").click());
    expect(host.textContent).toContain("Which order are your Tidewater episodes in?");
    expect(host.textContent).toContain("Couldn't save your answer. Try again in a moment.");
    cleanup();
  });

  it("says a title's answer wasn't saved, and clears it when answered again", async () => {
    let fail = true;
    const onIgnore = vi.fn(() => (fail ? Promise.reject(new Error("no")) : new Promise<void>(() => {})));
    const { host, button, cleanup } = mount(cards({ needs: [need("90000812")], onIgnore }));
    await act(async () => button("Ignore this").click());
    expect(host.textContent).toContain("Is this Paper Kites (1996)?");
    expect(host.textContent).toContain("Couldn't save your answer.");
    fail = false;
    await act(async () => button("Ignore this").click());
    expect(host.textContent).not.toContain("Couldn't save your answer.");
    cleanup();
  });

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

  // A show is checked by its episodes, never its length (decision 57).
  it("says a show's episodes fit", () => {
    const show = guess({
      kind: "series",
      name: "Harbour Lights",
      lengthMatches: false,
      episodesMatches: true,
    });
    const { host, cleanup } = mount(cards({ needs: [need("90000822", { best: show })] }));
    expect(host.textContent).toContain("The name and episodes match.");
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

  it("keeps TMDB's name for two orders of one kind, and a plain one for a kind that stands alone", () => {
    type Choice = LibraryOrderNeed["choices"][number];
    const [catalog, dvd, absolute] = tidewater.choices as [Choice, Choice, Choice];
    const twoDvd: LibraryOrderNeed = {
      ...tidewater,
      choices: [
        catalog,
        { ...dvd, name: "DVD Order (US)" },
        { ...dvd, name: "DVD Order (UK)", orderId: "90000799c" },
        { ...absolute, kind: "story", name: "Arc order" },
      ],
    };
    const { host, cleanup } = mount(cards({ needs: [], orders: [twoDvd] }));
    const text = host.textContent ?? "";
    expect(text).toContain("DVD Order (US)");
    expect(text).toContain("DVD Order (UK)");
    expect(text).toContain("Story order");
    expect(text).not.toContain("Arc order");
    cleanup();
  });

  it("says when a season on disk is one TMDB doesn't list", () => {
    const extra: LibraryOrderNeed = { ...kestrel, disk: [{ season: 3, episodes: 4 }] };
    const { host, cleanup } = mount(cards({ needs: [], orders: [extra] }));
    expect(host.textContent).toContain(
      "Season 3 on your disk isn't one TMDB lists. It still plays, named as it is in its file.",
    );
    cleanup();
  });

  it("words the other reasons a guess can give", () => {
    const nameAndYear = guess({ lengthMatches: false, yearMatches: true });
    const close = guess({ lengthMatches: false, nameMatches: false });
    const first = mount(cards({ needs: [need("90000811", { best: nameAndYear })] }));
    expect(first.host.textContent).toContain("The name and year match.");
    first.cleanup();
    const second = mount(cards({ needs: [need("90000812", { best: close })] }));
    expect(second.host.textContent).toContain("It looks close, but we can't be sure.");
    second.cleanup();
  });

  it("gives a title with both problems two entries", () => {
    const { host, cleanup } = mount(
      cards({ needs: [], problems: [problem("90000823", { extraBytes: 4_300_000_000, unplayable: 2 })] }),
    );
    expect(host.textContent).toContain("Needs you (2)");
    expect(host.textContent).toContain("Quiet Orchard can't be played");
    expect(host.textContent).toContain("Quiet Orchard has an extra copy");
    cleanup();
  });

  it("picks one of the other guesses", () => {
    const onLink = vi.fn(() => Promise.resolve());
    const other = guess({
      externalId: { namespace: "imdb", value: "tt90000902" },
      year: 2014,
      lengthMatches: false,
    });
    const { host, button, cleanup } = mount(
      cards({ onLink, needs: [need("90000811", { others: [other] })] }),
    );
    act(() => button("Pick a different title").click());
    expect(host.textContent).toContain("Paper Kites (2014)");
    act(() => button("Use this one").click());
    expect(onLink).toHaveBeenCalledWith("90000811", { namespace: "imdb", value: "tt90000902" });
    expect(host.textContent).toContain("Added to your library.");
    cleanup();
  });
});
