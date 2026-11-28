// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import {
  type LibraryScan,
  LibraryShelf,
  type LibraryShelfAdmin,
  type LibraryShelfProps,
  type LibraryShelfTitle,
} from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// The window the wall is drawn in, in points: the mock's desktop unless a test says a phone.
const screen = vi.hoisted(() => ({ width: 1440 }));
// The grid's virtualizer, with its scrolling recorded.
const scrolled = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-virtual", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-virtual")>();
  return {
    ...actual,
    useVirtualizer: (options: Parameters<typeof actual.useVirtualizer>[0]) =>
      Object.assign(actual.useVirtualizer(options), { scrollToIndex: scrolled }),
  };
});
// jsdom has no ResizeObserver; this one records what the grid watches.
const watched = vi.hoisted(() => {
  const nodes: { textContent: string | null }[] = [];
  Object.assign(globalThis, {
    ResizeObserver: class {
      observe(node: { textContent: string | null }) {
        nodes.push(node);
      }
      unobserve() {}
      disconnect() {}
    },
  });
  return nodes;
});
vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return { ...actual, useWindowDimensions: () => ({ ...actual.useWindowDimensions(), width: screen.width }) };
});

type El = {
  click(): void;
  dispatchEvent(e: unknown): boolean;
  getAttribute(n: string): string | null;
  hasAttribute(n: string): boolean;
  querySelector(s: string): El | null;
  textContent: string | null;
  value: string;
};
type Host = Parameters<typeof createRoot>[0] & {
  querySelector(s: string): El | null;
  querySelectorAll(s: string): ArrayLike<El>;
  remove(): void;
  textContent: string | null;
};
const win = globalThis as unknown as {
  document: {
    activeElement: unknown;
    body: { append(n: unknown): void };
    createElement(t: string): Host;
    querySelector(s: string): (El & { querySelectorAll(s: string): ArrayLike<El> }) | null;
  };
  Event: new (type: string, init: { bubbles: boolean }) => unknown;
  HTMLSelectElement: { prototype: object };
};
const doc = win.document;

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
const COUNTS = { all: 3412, movie: 2180, series: 1232 };

// An admin's wall, with the status given.
const admin = (status: Partial<LibraryShelfAdmin["status"]> = {}): LibraryShelfAdmin => ({
  status: { health: "ok", scan: IDLE, held: [], offline: [], needsYou: 0, ...status },
  checking: false,
  onCheckAgain: vi.fn(),
  onRescan: vi.fn(),
  onOpenSettings: vi.fn(),
  onOpenNeeds: vi.fn(),
});

const render = (overrides: Partial<LibraryShelfProps>) => {
  const props: LibraryShelfProps = {
    admin: admin(),
    filter: "all",
    onFilterChange: vi.fn(),
    counts: COUNTS,
    sort: "name",
    onSortChange: vi.fn(),
    onGenreChange: vi.fn(),
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
  const buttons = () => Array.from(host.querySelectorAll('[role="button"], [role="tab"], button, a'));
  const find = (name: string) =>
    buttons().find((n) => n.textContent?.trim() === name || n.getAttribute("aria-label") === name);
  const button = (name: string) => {
    const hit = find(name);
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
  return { host, props, button, find, tile, cleanup };
};

// Chooses an option in a native <select> the way a person does: its value, then a change event.
const choose = (select: El, value: string) => {
  const setter = Object.getOwnPropertyDescriptor(win.HTMLSelectElement.prototype, "value")?.set;
  act(() => {
    setter?.call(select, value);
    select.dispatchEvent(new win.Event("change", { bubbles: true }));
  });
};

describe("LibraryShelf", () => {
  it("lists the titles with their year and kind, and opens one", () => {
    const { host, props, tile, cleanup } = render({});
    expect(host.textContent).toContain("Up to date");
    expect(host.textContent).toContain("1999 · Show");
    expect(host.textContent).toContain("Undated");
    act(() => tile("Harbour Lights").click());
    expect(props.onOpenTitle).toHaveBeenCalledWith("90000801");
    cleanup();
  });

  it("says a title nothing matches isn't identified yet, with the needs-a-look mark", () => {
    const { tile, cleanup } = render({
      titles: [
        { id: "90000804", kind: "movie", name: "Clip 12", year: 2019, ready: true, unidentified: true },
      ],
    });
    const card = tile("Clip 12");
    expect(card.textContent).toContain("2019 · Not identified yet");
    expect(card.textContent).not.toContain("Movie");
    expect(card.querySelector('[aria-label="Needs a look"]')).not.toBeNull();
    cleanup();
  });

  it("calls a kept video a home video, with its year when it has one, not a Movie or Show", () => {
    const { tile, cleanup } = render({
      titles: [
        { id: "90000805", kind: "movie", name: "Lake day", year: 2019, ready: true, homeVideo: true },
        { id: "90000806", kind: "series", name: "Garden tapes", ready: true, homeVideo: true },
      ],
    });
    expect(tile("Lake day").textContent).toContain("2019 · Home video");
    expect(tile("Lake day").textContent).not.toContain("Movie");
    expect(tile("Garden tapes").textContent).toContain("Home video");
    expect(tile("Garden tapes").textContent).not.toContain("Show");
    expect(tile("Lake day").querySelector('[aria-label="Needs a look"]')).toBeNull();
    cleanup();
  });

  it("draws a card on the caller's link, ringed in amber on focus in place of the browser's ring", () => {
    const onClick = vi.fn((event: { preventDefault(): void }) => event.preventDefault());
    const { host, props, cleanup } = render({
      // The card replaces the link's own children.
      titleLink: (id) => (
        <a href={`/library/${id}`} onClick={onClick}>
          the card's own text
        </a>
      ),
    });
    const card = host.querySelector('a[href="/library/90000801"]') as El & {
      focus(): void;
      style: { outline: string };
      querySelectorAll(s: string): ArrayLike<{ classList: { contains(name: string): boolean } }>;
    };
    // The ring is drawn in the actionFocus colour. Tamagui hashes each atomic class, so find the
    // rules that carry that colour and count the card's elements wearing one of their classes.
    const sheets = globalThis as unknown as {
      document: { styleSheets: Iterable<{ cssRules: Iterable<{ cssText: string }> }> };
    };
    const ringed = () => {
      const css = [...sheets.document.styleSheets]
        .flatMap((sheet) => [...sheet.cssRules].map((rule) => rule.cssText))
        .join("\n");
      const names = [...css.matchAll(/\.(_[\w-]+)[^{}]*\{[^}]*var\(--actionFocus\)[^}]*\}/g)].map(
        (match) => match[1] ?? "",
      );
      return Array.from(card.querySelectorAll("[class]")).filter((el) =>
        names.some((name) => el.classList.contains(name)),
      ).length;
    };
    expect(card.textContent).toContain("Harbour Lights");
    expect(card.style.outline).toBe("none");
    expect(ringed()).toBe(0);
    act(() => card.focus());
    expect(ringed()).toBeGreaterThan(0);
    // Forced colours repaint a transparent outline, so focus stays visible there.
    expect(card.style.outline).toBe("2px solid transparent");
    // The link's own click is what navigates.
    act(() => card.click());
    expect(onClick).toHaveBeenCalledOnce();
    expect(props.onOpenTitle).not.toHaveBeenCalled();
    cleanup();
  });

  it("draws Review as the same button, not an underlined link, at every width", () => {
    for (const width of [1440, 390]) {
      screen.width = width;
      const { button, cleanup } = render({ admin: admin({ needsYou: 2 }) });
      const review = button("Review") as El & { style: { textDecorationLine: string } };
      expect(review.getAttribute("role") ?? "button").toBe("button");
      expect(review.style.textDecorationLine).toBe("");
      cleanup();
    }
    screen.width = 1440;
  });

  it("gives a phone's search, order, genre, All tab and Review touch targets", async () => {
    screen.width = 390;
    const { host, button, cleanup } = render({ admin: admin({ needsYou: 2 }), genres: [] });
    const style = (node: unknown) => (node as { style: { minHeight: string } }).style;
    expect(style(host.querySelector("input")).minHeight).toBe("48px");
    await act(async () => button("Filters").click());
    expect(style(doc.querySelector('[role="dialog"] select')).minHeight).toBe("48px");
    const all = doc.querySelector('[role="dialog"] [role="tab"]');
    expect(all?.textContent?.trim()).toBe("All");
    // The width is on the box inside the tab's padding, set by a stylesheet class.
    const box = (all as unknown as { firstElementChild: { firstElementChild: unknown } }).firstElementChild
      .firstElementChild;
    const computed = (globalThis as unknown as { getComputedStyle(n: unknown): { minWidth: string } })
      .getComputedStyle;
    expect(computed(box).minWidth).toBe("48px");
    expect(button("Review").getAttribute("href")).toBeNull();
    screen.width = 1440;
    cleanup();
  });

  it("says how many titles need you on the status line, and Review opens Needs you", () => {
    const status = admin({ needsYou: 6 });
    const { host, button, cleanup } = render({ admin: status });
    expect(host.textContent).toContain("6 need you");
    act(() => button("Review").click());
    expect(status.onOpenNeeds).toHaveBeenCalled();
    cleanup();

    const clear = render({});
    expect(clear.host.textContent).not.toContain("need you");
    expect(clear.find("Review")).toBeUndefined();
    clear.cleanup();
  });

  it("says one title needs you in the singular", () => {
    const { host, cleanup } = render({ admin: admin({ needsYou: 1 }) });
    expect(host.textContent).toContain("1 needs you");
    expect(host.textContent).not.toContain("1 need you");
    cleanup();
  });

  it("tells a member an empty Library has nothing in it yet, and offers an admin the folders", () => {
    const member = render({ admin: undefined, titles: [] });
    expect(member.host.textContent).toContain("Nothing in the Library yet");
    expect(member.find("Tell Loomarr where your media is")).toBeUndefined();
    member.cleanup();

    const idle = render({ titles: [] });
    expect(idle.host.textContent).toContain("Dead air");
    expect(idle.host.textContent).not.toContain("Nothing in the Library yet");
    idle.cleanup();

    const importing = render({ admin: admin({ scan: WALKING }), titles: [] });
    expect(importing.host.textContent).not.toContain("Dead air");
    importing.cleanup();
  });

  it("gives a member the wall with no status line, no Needs you and no settings", () => {
    const { host, find, tile, props, cleanup } = render({ admin: undefined });
    expect(host.textContent).not.toContain("Up to date");
    expect(host.textContent).not.toContain("need you");
    expect(find("Library settings")).toBeUndefined();
    expect(find("Review")).toBeUndefined();
    act(() => tile("Harbour Lights").click());
    expect(props.onOpenTitle).toHaveBeenCalledWith("90000801");
    cleanup();
  });

  it("says how many titles wait for details and when it tries again", () => {
    const { host, cleanup } = render({
      admin: admin({
        details: { waiting: 12, unreachable: true, retryAt: new Date(2026, 10, 2, 15, 40).toISOString() },
      }),
    });
    expect(host.textContent).toContain("12 titles are waiting on details. Trying again at 3:40 PM");
    expect(host.textContent).not.toContain("Up to date");
    cleanup();
  });

  it("tells the retry time on the household's clock, whatever the device's", () => {
    const { host, cleanup } = render({
      admin: admin({ details: { waiting: 12, unreachable: true, retryAt: "2026-11-02T02:40:00Z" } }),
      timeZone: "Pacific/Auckland",
    });
    expect(host.textContent).toContain("12 titles are waiting on details. Trying again at 3:40 PM");
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

  // A three-across phone grid has no room for a word on the poster, so the caption says it.
  it("says a title is getting ready in its caption, in place of its year and kind", () => {
    const { tile, cleanup } = render({});
    const getting = tile("Paper Kites");
    expect(getting.textContent).toContain("Getting ready");
    expect(getting.textContent).not.toContain("1996 · Movie");
    expect(twirls(getting as unknown as Host)).toBe(0);
    expect(tile("Harbour Lights").textContent).not.toContain("Getting ready");
    cleanup();
  });

  it("marks a title that needs a look, to a screen reader too", () => {
    const { tile, cleanup } = render({
      titles: [{ id: "90000804", kind: "movie", name: "Night Ferry", ready: true, attention: true }],
    });
    expect(tile("Night Ferry").querySelector('[aria-label="Needs a look"]')).toBeTruthy();
    cleanup();
  });

  // The monogram on an artless poster stands in for artwork, in the secondary tone over a neutral
  // surface; assistive tech hears the title once, from the line under the poster.
  it("keeps an artless poster's cover out of the tile's name", () => {
    const { tile, cleanup } = render({});
    const hidden = tile("Paper Kites").querySelector('[aria-hidden="true"]');
    expect(hidden?.textContent).toBe("Paper Kites1996");
    // Nor does the frame caption itself "No artwork" under the cover.
    expect(tile("Paper Kites").textContent).not.toContain("No artwork");
    cleanup();
  });

  it("counts each tab across the library, and filters by kind", () => {
    const { props, button, cleanup } = render({});
    expect(button("All, 3,412 titles").textContent).toContain("3,412");
    expect(button("Movies, 2,180 titles").textContent).toContain("2,180");
    expect(button("Shows, 1,232 titles").textContent).toContain("1,232");
    act(() => button("Shows, 1,232 titles").click());
    expect(props.onFilterChange).toHaveBeenCalledWith("series");
    act(() => button("Collections").click());
    expect(props.onFilterChange).toHaveBeenCalledWith("collections");
    cleanup();

    const counting = render({ counts: undefined });
    expect(counting.button("All").textContent?.trim()).toBe("All");
    counting.cleanup();
  });

  it("on a phone, says in the sheet how many titles the filters leave, and counts none on Collections", async () => {
    screen.width = 390;
    try {
      const filtering = {
        filters: { uhd: true },
        onChange: vi.fn(),
        onClear: vi.fn(),
        personSearch: { value: "", onChange: vi.fn(), options: [], loading: false, failed: false },
        total: 37,
      };
      const { button, cleanup } = render({ filter: "all", genres: [], filtering });
      await act(async () => button("Filters (1)").click());
      const sheet = doc.querySelector('[role="dialog"]');
      expect(sheet?.querySelector('[role="status"]')?.textContent).toBe("37 titles");
      expect(sheet?.textContent).toContain("Filters");
      cleanup();

      const collections = render({ filter: "collections", genres: [], filtering });
      expect(collections.button("Filters").textContent?.trim()).toBe("Filters");
      collections.cleanup();
    } finally {
      screen.width = 1440;
    }
  });

  it("on a phone, folds the tabs and filters into a sheet that applies and gives focus back", async () => {
    screen.width = 390;
    try {
      const { host, props, button, find, cleanup } = render({ filter: "movie", genres: [] });
      // One line on the page: the search, and Filters with how many are on.
      expect(host.querySelector('[role="tab"]')).toBeNull();
      expect(host.querySelector("select")).toBeNull();
      expect(host.querySelector("input")).not.toBeNull();
      const opener = button("Filters (1)") as El & { focus(): void; style: { minHeight: string } };
      expect(opener.style.minHeight).toBe("48px");
      // Settings is the gear alone, named for what it opens, and a finger's size each way.
      const settings = find("Library settings") as El & { style: { minHeight: string; width: string } };
      expect(settings.textContent?.trim()).toBe("");
      expect([settings.style.minHeight, settings.style.width]).toEqual(["48px", "48px"]);

      await act(async () => {
        opener.focus();
        opener.click();
      });
      const sheet = () => doc.querySelector('[role="dialog"]');
      const inSheet = (selector: string) => Array.from(sheet()?.querySelectorAll(selector) ?? []);
      // The sheet is outside the page, with the count of the tab and no counts in the tabs.
      expect(sheet()?.textContent).toContain("2,180 titles");
      expect(sheet()?.textContent).not.toContain("3,412");
      const shows = inSheet('[role="tab"]').find((n) => n.textContent?.trim() === "Shows");
      act(() => shows?.click());
      expect(props.onFilterChange).toHaveBeenCalledWith("series");
      const [order] = inSheet("select");
      choose(order as El, "recent");
      expect(props.onSortChange).toHaveBeenCalledWith("recent");

      await act(async () => {
        inSheet("button, [role='button']")
          .find((n) => n.textContent?.trim() === "Done")
          ?.click();
      });
      await vi.waitFor(() => expect(sheet()).toBeNull());
      await vi.waitFor(() => expect(doc.activeElement).toBe(opener));
      cleanup();
    } finally {
      screen.width = 1440;
    }
  });

  it("orders the wall by name or by the titles added last", () => {
    const { host, props, cleanup } = render({});
    const select = host.querySelector('select[aria-label="Order"]');
    if (!select) throw new Error("missing the order");
    expect(select.value).toBe("name");
    expect(select.textContent).toContain("A–Z");
    expect(select.textContent).toContain("Recently added");
    choose(select, "recent");
    expect(props.onSortChange).toHaveBeenCalledWith("recent");
    cleanup();
  });

  it("shows the collections in place of the search, the order and the grid on Collections", () => {
    const { host, cleanup } = render({ filter: "collections", collections: <p>the collections</p> });
    expect(host.textContent).toContain("the collections");
    expect(host.textContent).not.toContain("Harbour Lights");
    expect(host.querySelector('[aria-label="Search titles"]')).toBeNull();
    expect(host.querySelector('select[aria-label="Order"]')).toBeNull();
    cleanup();
  });

  it("says when a search matches nothing, what it looked at, and clears it", () => {
    const { host, props, button, cleanup } = render({ search: "zebra 1977 ", titles: [] });
    expect(host.textContent).toContain("Nothing here matches “zebra 1977”");
    expect(host.textContent).toContain("Search looks at titles, years and genres across all 3,412 titles.");
    act(() => button("Clear search").click());
    expect(props.onSearchChange).toHaveBeenCalledWith("");
    cleanup();

    const member = render({ admin: undefined, search: "zebra", titles: [], counts: undefined });
    expect(member.host.textContent).toContain("Nothing here matches “zebra”");
    expect(member.host.textContent).toContain("Search looks at titles, years and genres.");
    member.cleanup();
  });

  it("draws placeholders while the first page is on its way, and no empty words", () => {
    const { host, cleanup } = render({ titles: [], loading: true, search: "zebra", filter: "movie" });
    expect(host.querySelector('[aria-label="Loading your titles"]')).toBeTruthy();
    expect(host.textContent).not.toContain("Nothing here matches");
    expect(host.textContent).not.toContain("No movies yet");
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
  });

  it("shows the import's step, share and time left on one line, twirling over its bar", () => {
    const { host, cleanup } = render({ admin: admin({ scan: WALKING, needsYou: 3 }), titles: [] });
    expect(host.textContent).toContain("Adding your media · Step 1 of 3 · 21% · about 9 min left");
    expect(host.textContent).toContain("3 need you");
    expect(host.textContent).toContain("Library settings");
    expect(twirls(host)).toBe(1);
    cleanup();

    const early = render({ admin: admin({ scan: { ...WALKING, etaSeconds: undefined } }), titles: [] });
    expect(early.host.textContent).toContain("Adding your media · Step 1 of 3 · 21%");
    expect(early.host.textContent).not.toContain("left");
    early.cleanup();
  });

  it("keeps a still dot while paused, and says only that it's checking on a quiet pass", () => {
    const paused = render({
      admin: admin({ scan: { ...WALKING, step: "reading", stepNumber: 2, paused: true } }),
      titles: [],
    });
    expect(paused.host.textContent).toContain("Paused while someone is watching · Step 2 of 3 · 21%");
    expect(paused.host.textContent).not.toContain("left");
    expect(twirls(paused.host)).toBe(0);
    paused.cleanup();

    const quiet = render({ admin: admin({ scan: { ...WALKING, quiet: true } }), titles: [] });
    expect(quiet.host.textContent).toContain("Checking your folders…");
    expect(quiet.host.textContent).not.toContain("Step 1");
    expect(twirls(quiet.host)).toBe(1);
    quiet.cleanup();
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

  it("puts a held removal first, with Check files again", () => {
    const held = admin({
      held: [{ path: "/media/movies", movies: 2406, episodes: 0, other: 0, titles: [], titleCount: 0 }],
    });
    const { host, button, cleanup } = render({ admin: held });
    expect(host.textContent).toContain("2,406 movies seem to be gone from your Movies folder");
    act(() => button("Check files again").click());
    expect(held.onRescan).toHaveBeenCalled();
    cleanup();
  });

  it("names an offline folder, and Check again looks at the folders", () => {
    const offline = admin({ health: "folder_offline", offline: ["/media/movies"] });
    const { host, button, cleanup } = render({ admin: offline });
    expect(host.textContent).toContain("Can't reach your Movies folder");
    act(() => button("Check again").click());
    expect(offline.onCheckAgain).toHaveBeenCalled();
    cleanup();
  });

  it("keeps the import's step and share on the line after an offline folder", () => {
    const { host, button, cleanup } = render({
      admin: admin({ health: "folder_offline", scan: WALKING, offline: ["/media/movies"] }),
    });
    expect(host.textContent).toContain(
      "Can't reach your Movies folder · Adding your media · Step 1 of 3 · 21% · about 9 min left",
    );
    expect(button("Check again")).toBeTruthy();
    cleanup();

    const idle = render({ admin: admin({ health: "folder_offline", offline: ["/media/movies"] }) });
    expect(idle.host.textContent).not.toContain("Up to date");
    idle.cleanup();
  });

  it("is dead air until a folder is chosen, and points at Settings", () => {
    const fresh = admin({ health: "needs_folders" });
    const { host, button, cleanup } = render({ admin: fresh, titles: [] });
    expect(host.textContent).toContain("Dead air");
    expect(host.textContent).toContain("Point it at your movie and show folders");
    expect(host.querySelector('[aria-label="Search titles"]')).toBeNull();
    act(() => button("Tell Loomarr where your media is").click());
    expect(fresh.onOpenSettings).toHaveBeenCalled();
    cleanup();
  });

  it("lists each genre with its titles under All genres, and reports the choice", () => {
    const { host, props, cleanup } = render({
      genres: [
        { name: "Drama", titles: 1204 },
        { name: "Comedy", titles: 7 },
      ],
      genre: "drama",
    });
    const select = host.querySelector('select[aria-label="Genre"]');
    if (!select) throw new Error("missing the genre filter");
    // The URL's spelling matches the list's in any case.
    expect(select.value).toBe("Drama");
    expect(select.textContent).toContain("All genres");
    expect(select.textContent).toContain("Drama · 1,204");
    expect(select.textContent).toContain("Comedy · 7");
    choose(select, "Comedy");
    expect(props.onGenreChange).toHaveBeenCalledWith("Comedy");
    choose(select, "");
    expect(props.onGenreChange).toHaveBeenLastCalledWith(undefined);
    cleanup();
  });

  it("says a genre has nothing in the tab, keeps it chosen, and goes back to every genre", () => {
    const { host, props, button, cleanup } = render({
      filter: "series",
      genre: "Western",
      genres: [{ name: "Drama", titles: 3 }],
      titles: [],
    });
    expect(host.textContent).toContain("No shows in Western");
    expect(host.querySelector('select[aria-label="Genre"]')?.value).toBe("Western");
    act(() => button("Show all genres").click());
    expect(props.onGenreChange).toHaveBeenCalledWith(undefined);
    cleanup();
  });

  const LETTERS = [
    { letter: "H", offset: 0 },
    { letter: "P", offset: 40 },
  ];

  it("draws the rail on a desktop window, with letters that have no titles disabled", () => {
    const onJump = vi.fn();
    const { button, cleanup } = render({ letters: LETTERS, onJump });
    const z = button("Jump to titles starting with Z");
    expect(z.hasAttribute("disabled") || z.getAttribute("aria-disabled") === "true").toBe(true);
    act(() => z.click());
    expect(onJump).not.toHaveBeenCalled();
    act(() => button("Jump to titles starting with P").click());
    expect(onJump).toHaveBeenCalledWith({ letter: "P", offset: 40 });
    cleanup();
  });

  it("draws no rail below the desktop width, or for a list with no letters", () => {
    screen.width = 1023;
    try {
      const narrow = render({ letters: LETTERS, onJump: vi.fn() });
      expect(narrow.find("Jump to titles starting with P")).toBeUndefined();
      narrow.cleanup();
    } finally {
      screen.width = 1440;
    }
    const recent = render({ sort: "recent", onJump: vi.fn() });
    expect(recent.find("Jump to titles starting with P")).toBeUndefined();
    recent.cleanup();
  });

  // The grid is endless: a rail after it in the document is out of a keyboard's reach.
  it("puts the rail before the grid in the document, so Tab reaches it first", () => {
    const { host, cleanup } = render({ letters: LETTERS, onJump: vi.fn() });
    const order = Array.from(host.querySelectorAll('[role="navigation"], [role="list"]')).map((n) =>
      n.getAttribute("role"),
    );
    expect(order).toEqual(["navigation", "list"]);
    cleanup();
  });

  it("scrolls the grid to the row holding the jumped title", () => {
    scrolled.mockClear();
    // The grid scrolls whatever holds it, and jsdom has no such thing until one says it scrolls.
    const body = (globalThis as unknown as { document: { body: { style: { overflowY: string } } } }).document
      .body;
    body.style.overflowY = "auto";
    const titles = Array.from(
      { length: 120 },
      (_, i) => ({ ...TITLES[0], id: `t${i}` }) as LibraryShelfTitle,
    );
    const { cleanup } = render({
      titles,
      letters: LETTERS,
      onJump: vi.fn(),
      jump: { index: 62, letter: "P", request: 1 },
    });
    // Six across, so the 63rd title is in the eleventh row.
    expect(scrolled).toHaveBeenCalledWith(10, { align: "start" });
    cleanup();
    body.style.overflowY = "";
  });

  // The grid's own parent holds only the grid, beside the rail. A change in what sits above it
  // (the status line) moves the grid, and only an outer ancestor changes height with it.
  it("watches the heights of what holds the status line above the grid", () => {
    watched.length = 0;
    const { cleanup } = render({});
    expect(watched.some((node) => node.textContent?.includes("Up to date"))).toBe(true);
    cleanup();
  });

  it("opens Library settings for an admin", () => {
    const { button, props, cleanup } = render({});
    act(() => button("Library settings").click());
    expect(props.admin?.onOpenSettings).toHaveBeenCalled();
    cleanup();
  });
});
