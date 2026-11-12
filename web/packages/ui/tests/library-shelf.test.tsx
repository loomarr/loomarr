// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { type LibraryScan, LibraryShelf, type LibraryShelfProps, type LibraryShelfTitle } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type El = {
  click(): void;
  getAttribute(n: string): string | null;
  hasAttribute(n: string): boolean;
  querySelector(s: string): El | null;
  textContent: string | null;
};
type Host = Parameters<typeof createRoot>[0] & {
  querySelector(s: string): El | null;
  querySelectorAll(s: string): ArrayLike<El>;
  remove(): void;
  textContent: string | null;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Host };
  }
).document;

const IDLE: LibraryScan = {
  enabled: true,
  step: "idle",
  stepNumber: 0,
  done: 0,
  total: 0,
  paused: false,
  quiet: false,
};
const WALKING: LibraryScan = {
  ...IDLE,
  step: "walking",
  stepNumber: 1,
  done: 112,
  total: 533,
  etaSeconds: 540,
};
const twirls = (node: { querySelectorAll(s: string): ArrayLike<unknown> }) =>
  node.querySelectorAll('[data-testid="library-twirl"]').length;
const TITLES: LibraryShelfTitle[] = [
  { id: "90000801", kind: "series", name: "Harbour Lights", year: 1999, ready: true },
  { id: "90000802", kind: "movie", name: "Paper Kites", year: 1996, ready: false },
  { id: "90000803", kind: "movie", name: "Undated", ready: true },
];

const render = (overrides: Partial<LibraryShelfProps>) => {
  const props: LibraryShelfProps = {
    status: { health: "ok", scan: IDLE, held: [], offline: [] },
    checking: false,
    onCheckAgain: vi.fn(),
    onRescan: vi.fn(),
    onOpenSettings: vi.fn(),
    filter: "all",
    onFilterChange: vi.fn(),
    search: "",
    onSearchChange: vi.fn(),
    titles: TITLES,
    loading: false,
    onOpenTitle: vi.fn(),
    onEndReached: vi.fn(),
    ...overrides,
  };
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() =>
    root.render(
      <LoomarrProvider theme="dark">
        <LibraryShelf {...props} />
      </LoomarrProvider>,
    ),
  );
  const buttons = () => Array.from(host.querySelectorAll('[role="button"], [role="tab"], button'));
  const button = (name: string) => {
    const hit = buttons().find(
      (n) => n.textContent?.trim() === name || n.getAttribute("aria-label") === name,
    );
    if (!hit) throw new Error(`missing button: ${name}`);
    return hit;
  };
  // A title's tile is named by what it shows, so it is found by its title.
  const tile = (title: string) => {
    const hit = buttons().find((n) => n.textContent?.includes(title));
    if (!hit) throw new Error(`missing tile: ${title}`);
    return hit;
  };
  const cleanup = () => {
    act(() => root.unmount());
    host.remove();
  };
  return { host, props, button, tile, cleanup };
};

describe("LibraryShelf", () => {
  it("lists the titles with their year and kind, and opens one", () => {
    const { host, props, tile, cleanup } = render({});
    expect(host.textContent).toContain("All caught up — every title's ready to air");
    expect(host.textContent).toContain("1999 · Show");
    expect(host.textContent).toContain("1996 · Movie");
    expect(host.textContent).toContain("Undated");
    act(() => tile("Harbour Lights").click());
    expect(props.onOpenTitle).toHaveBeenCalledWith("90000801");
    cleanup();
  });

  // #2779: "All caught up — every title's ready to air" sat above "Needs you (4)". With anything
  // waiting the line says only what the folders' state is.
  it("doesn't claim every title is ready while Needs you is waiting", () => {
    const { host, cleanup } = render({
      status: { health: "ok", scan: IDLE, held: [], offline: [], needsYou: true },
    });
    expect(host.textContent).not.toContain("All caught up");
    expect(host.textContent).toContain("Up to date");
    cleanup();
  });

  it("says how many titles wait for details and when it tries again, not that all is caught up", () => {
    const { host, cleanup } = render({
      status: {
        health: "ok",
        scan: IDLE,
        held: [],
        offline: [],
        details: { waiting: 12, unreachable: true, retryAt: new Date(2026, 10, 2, 15, 40).toISOString() },
      },
    });
    expect(host.textContent).toContain("12 titles are waiting on details — trying again at 3:40 PM");
    expect(host.textContent).not.toContain("All caught up");
    cleanup();
  });

  it("tells the retry time on the household's clock, whatever the device's", () => {
    const { host, cleanup } = render({
      status: {
        health: "ok",
        scan: IDLE,
        held: [],
        offline: [],
        details: { waiting: 12, unreachable: true, retryAt: "2026-11-02T02:40:00Z" },
      },
      timeZone: "Pacific/Auckland",
    });
    expect(host.textContent).toContain("12 titles are waiting on details — trying again at 3:40 PM");
    cleanup();
  });

  it("doesn't claim every title is ready while a folder in use is empty", () => {
    const { host, cleanup } = render({
      status: { health: "folder_empty", scan: IDLE, held: [], offline: [] },
    });
    expect(host.textContent).not.toContain("All caught up");
    expect(host.textContent).toContain("Up to date");
    cleanup();
  });

  // WCAG 2.5.3: a tile's name is what it shows. A label of its own ("Harbour Lights") left out the
  // year and kind beside it, which axe flags and a voice-control user cannot say.
  it("names a tile by what it shows, with no label of its own", () => {
    const { tile, cleanup } = render({});
    expect(tile("Harbour Lights").hasAttribute("aria-label")).toBe(false);
    expect(tile("Harbour Lights").textContent).toContain("1999 · Show");
    cleanup();
  });

  it("marks a title that isn't ready yet, in words and to a screen reader", () => {
    const { tile, cleanup } = render({});
    expect(tile("Paper Kites").textContent).toContain("Getting ready");
    cleanup();
  });

  it("marks a title that needs a look, to a screen reader too", () => {
    const { tile, cleanup } = render({
      titles: [{ id: "90000804", kind: "movie", name: "Night Ferry", ready: true, attention: true }],
    });
    expect(tile("Night Ferry").querySelector('[aria-label="Needs a look"]')).toBeTruthy();
    cleanup();
  });

  // The name set on an artless poster stands in for artwork and sits over the dimmed frame, at
  // full contrast; assistive tech hears the title once, from the line under the poster.
  it("keeps an artless poster's name out of the tile's name", () => {
    const { tile, cleanup } = render({});
    const hidden = tile("Paper Kites").querySelector('[aria-hidden="true"]');
    expect(hidden?.textContent).toBe("Paper Kites");
    // Nor does the frame caption itself "No artwork" under the name it already shows.
    expect(tile("Paper Kites").textContent).not.toContain("No artwork");
    cleanup();
  });

  it("filters by kind", () => {
    const { props, button, cleanup } = render({});
    act(() => button("Shows").click());
    expect(props.onFilterChange).toHaveBeenCalledWith("series");
    cleanup();
  });

  it("shows the collections in place of the search and the grid on Collections", () => {
    const { host, cleanup } = render({ filter: "collections", collections: <p>the collections</p> });
    expect(host.textContent).toContain("the collections");
    expect(host.textContent).not.toContain("Harbour Lights");
    expect(host.querySelector('[aria-label="Search your library"]')).toBeNull();
    cleanup();
  });

  it("says when a search matches nothing", () => {
    const { host, cleanup } = render({ search: "zebra", titles: [] });
    expect(host.textContent).toContain('Nothing here matches "zebra".');
    cleanup();
  });

  it("says a tab is empty and offers everything, rather than a blank grid", () => {
    const { host, props, button, cleanup } = render({ filter: "movie", titles: [] });
    expect(host.textContent).toContain("No movies yet");
    act(() => button("Show everything").click());
    expect(props.onFilterChange).toHaveBeenCalledWith("all");
    cleanup();
    const shows = render({ filter: "series", titles: [] });
    expect(shows.host.textContent).toContain("No shows yet");
    shows.cleanup();
    const loading = render({ filter: "movie", titles: [], loading: true });
    expect(loading.host.textContent).not.toContain("No movies yet");
    loading.cleanup();
  });

  it("shows the import's step, share and time left on one line, twirling", () => {
    const { host, cleanup } = render({
      status: { health: "ok", scan: WALKING, held: [], offline: [] },
      titles: [],
    });
    expect(host.textContent).toContain("Adding your media · Step 1 of 3 · 21% · about 9 min left");
    expect(host.textContent).toContain("Library settings");
    expect(twirls(host)).toBe(1);
    cleanup();

    const early = render({
      status: { health: "ok", scan: { ...WALKING, etaSeconds: undefined }, held: [], offline: [] },
      titles: [],
    });
    expect(early.host.textContent).toContain("Adding your media · Step 1 of 3 · 21%");
    expect(early.host.textContent).not.toContain("left");
    early.cleanup();
  });

  it("keeps a still dot while paused, and says only that it's checking on a quiet pass", () => {
    const paused = render({
      status: {
        health: "ok",
        scan: { ...WALKING, step: "reading", stepNumber: 2, paused: true },
        held: [],
        offline: [],
      },
      titles: [],
    });
    expect(paused.host.textContent).toContain("Paused while someone is watching · Step 2 of 3 · 21%");
    expect(paused.host.textContent).not.toContain("left");
    expect(twirls(paused.host)).toBe(0);
    paused.cleanup();

    const quiet = render({
      status: { health: "ok", scan: { ...WALKING, quiet: true }, held: [], offline: [] },
      titles: [],
    });
    expect(quiet.host.textContent).toContain("Checking your folders…");
    expect(quiet.host.textContent).not.toContain("Step 1");
    expect(twirls(quiet.host)).toBe(1);
    quiet.cleanup();
  });

  it("twirls on a title still getting ready, and not on a ready one", () => {
    const { host, cleanup } = render({});
    const tiles = Array.from(host.querySelectorAll('[role="listitem"]')) as unknown as Host[];
    const tile = (name: string) => tiles.find((t) => t.textContent?.includes(name));
    const getting = tile("Paper Kites");
    const ready = tile("Harbour Lights");
    if (!getting || !ready) throw new Error("missing tiles");
    expect(getting.textContent).toContain("Getting ready");
    expect(twirls(getting)).toBe(1);
    expect(twirls(ready)).toBe(0);
    cleanup();
  });

  // A household's library is thousands of titles: only the rows near the screen are mounted, and
  // the next page is asked for once the last rows are.
  it("mounts only a window of a large library's rows, and asks for more only near the end", () => {
    const many: LibraryShelfTitle[] = Array.from({ length: 2000 }, (_, i) => ({
      id: String(90100000 + i),
      kind: "movie",
      name: `Title ${i}`,
      ready: true,
    }));
    const big = render({ titles: many });
    const mounted = big.host.querySelectorAll('[role="listitem"]').length;
    expect(mounted).toBeGreaterThan(0);
    expect(mounted).toBeLessThan(120);
    expect(big.host.textContent).toContain("Title 0");
    expect(big.host.textContent).not.toContain("Title 1999");
    expect(big.props.onEndReached).not.toHaveBeenCalled();
    big.cleanup();

    const short = render({ titles: many.slice(0, 30) });
    expect(short.host.querySelectorAll('[role="listitem"]').length).toBe(30);
    expect(short.props.onEndReached).toHaveBeenCalled();
    short.cleanup();
  });

  it("puts a held removal first, with Check again", () => {
    const { host, props, button, cleanup } = render({
      status: {
        health: "ok",
        scan: IDLE,
        held: [{ path: "/media/movies", movies: 2406, episodes: 0, other: 0 }],
        offline: [],
      },
    });
    expect(host.textContent).toContain("2,406 movies seem to be gone from your Movies folder");
    act(() => button("Check again").click());
    expect(props.onRescan).toHaveBeenCalled();
    cleanup();
  });

  it("names an offline folder, and Check again looks at the folders", () => {
    const { host, props, button, cleanup } = render({
      status: { health: "folder_offline", scan: IDLE, held: [], offline: ["/media/movies"] },
    });
    expect(host.textContent).toContain("Can't reach your Movies folder");
    act(() => button("Check again").click());
    expect(props.onCheckAgain).toHaveBeenCalled();
    cleanup();
  });

  it("keeps the import's step and share on the line after an offline folder", () => {
    const { host, button, cleanup } = render({
      status: { health: "folder_offline", scan: WALKING, held: [], offline: ["/media/movies"] },
    });
    expect(host.textContent).toContain(
      "Can't reach your Movies folder · Adding your media · Step 1 of 3 · 21% · about 9 min left",
    );
    expect(button("Check again")).toBeTruthy();
    cleanup();

    const idle = render({
      status: { health: "folder_offline", scan: IDLE, held: [], offline: ["/media/movies"] },
    });
    expect(idle.host.textContent).not.toContain("caught up");
    idle.cleanup();
  });

  it("is dead air until a folder is chosen, and points at Settings", () => {
    const { host, props, button, cleanup } = render({
      status: { health: "needs_folders", scan: IDLE, held: [], offline: [] },
      titles: [],
    });
    expect(host.textContent).toContain("Dead air");
    expect(host.textContent).toContain("Point it at your movie and show folders");
    expect(host.textContent).not.toContain("Search");
    act(() => button("Tell Loomarr where your media is").click());
    expect(props.onOpenSettings).toHaveBeenCalled();
    cleanup();
  });
});
