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

const IDLE: LibraryScan = { enabled: true, stage: "idle", headersDone: 0, headersTotal: 0, paused: false };
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

  it("shows the import's progress while new files are read", () => {
    const { host, cleanup } = render({
      status: {
        health: "ok",
        scan: { enabled: true, stage: "reading", headersDone: 25, headersTotal: 100, paused: false },
        held: [],
        offline: [],
      },
    });
    expect(host.textContent).toContain("Adding your media");
    expect(host.querySelector('[role="progressbar"]')?.getAttribute("aria-valuenow")).toBe("25");
    cleanup();
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

  it("is dead air until a folder is chosen, and points at Settings", () => {
    const { host, props, button, cleanup } = render({
      status: { health: "needs_folders", scan: IDLE, held: [], offline: [] },
      titles: [],
    });
    expect(host.textContent).toContain("Dead air — tell Loomarr where your media is.");
    expect(host.textContent).not.toContain("Search");
    act(() => button("Tell Loomarr where your media is").click());
    expect(props.onOpenSettings).toHaveBeenCalled();
    cleanup();
  });
});
