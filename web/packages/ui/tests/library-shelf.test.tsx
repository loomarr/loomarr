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
  const cleanup = () => {
    act(() => root.unmount());
    host.remove();
  };
  return { host, props, button, cleanup };
};

describe("LibraryShelf", () => {
  it("lists the titles with their year and kind, and opens one", () => {
    const { host, props, button, cleanup } = render({});
    expect(host.textContent).toContain("All caught up — every title's ready to air");
    expect(host.textContent).toContain("1999 · Show");
    expect(host.textContent).toContain("1996 · Movie");
    expect(host.textContent).toContain("Undated");
    act(() => button("Harbour Lights").click());
    expect(props.onOpenTitle).toHaveBeenCalledWith("90000801");
    cleanup();
  });

  it("marks a title that isn't ready yet, in words and to a screen reader", () => {
    const { host, button, cleanup } = render({});
    expect(host.textContent).toContain("Getting ready");
    expect(button("Paper Kites, getting ready")).toBeTruthy();
    cleanup();
  });

  it("filters by kind", () => {
    const { props, button, cleanup } = render({});
    act(() => button("Shows").click());
    expect(props.onFilterChange).toHaveBeenCalledWith("series");
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
