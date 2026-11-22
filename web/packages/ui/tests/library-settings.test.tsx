// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import {
  type LibraryBrowseListing,
  type LibraryFolder,
  type LibraryFolderBrowser,
  type LibraryScan,
  LibrarySettings,
  type LibrarySettingsProps,
} from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type El = {
  click(): void;
  dispatchEvent(e: unknown): boolean;
  getAttribute(n: string): string | null;
  textContent: string | null;
  value: string;
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

const NOW = Date.parse("2026-11-02T12:00:00Z");
const TV: LibraryFolder = {
  path: "/media/tv",
  inUse: false,
  readable: true,
  checkedAt: "2026-11-02T11:48:00Z",
  addedByHand: false,
  foundIn: [
    { authority: "emby", path: "/data/tv" },
    { authority: "sonarr", path: "/tv" },
  ],
};
const MOVIES: LibraryFolder = { ...TV, path: "/media/movies", inUse: true, readable: false, foundIn: [] };
const TV_IN_USE: LibraryFolder = { ...TV, inUse: true };
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
  current: "TV / Night Ferry / Season 2",
  etaSeconds: 540,
};
const READING: LibraryScan = { ...IDLE, step: "reading", stepNumber: 2, done: 25, total: 100 };
const twirls = (host: Host) => host.querySelectorAll('[data-testid="library-twirl"]').length;

const render = (overrides: Partial<LibrarySettingsProps>) => {
  const props: LibrarySettingsProps = {
    health: "needs_folders",
    folders: [TV],
    pathMap: [{ authority: "sonarr", authorityPath: "/tv", localPath: "/media/tv", byHand: false }],
    scan: IDLE,
    held: [],
    hooks: [
      { app: "sonarr", state: "registered" },
      { app: "radarr", state: "not_configured" },
    ],
    checking: false,
    onCheckAgain: vi.fn(),
    onRescan: vi.fn(),
    onRefreshEverything: vi.fn(),
    onConfirmRemoval: vi.fn(),
    onUseFolder: vi.fn(),
    onAddFolder: vi.fn(async () => "Loomarr can't open that folder"),
    onMapFolder: vi.fn(),
    now: NOW,
    ...overrides,
  };
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() =>
    root.render(
      <LoomarrProvider theme="dark">
        <LibrarySettings {...props} />
      </LoomarrProvider>,
    ),
  );
  const button = (name: string) => {
    const hit = Array.from(host.querySelectorAll('[role="button"], button')).find(
      (n) => n.textContent?.trim() === name,
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

const type = (input: El, text: string) => {
  const setter = Object.getOwnPropertyDescriptor(
    (globalThis as unknown as { HTMLInputElement: { prototype: object } }).HTMLInputElement.prototype,
    "value",
  )?.set;
  act(() => {
    setter?.call(input, text);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
};

describe("LibrarySettings", () => {
  it("asks for folders in plain words and confirms one", () => {
    const { host, props, button, cleanup } = render({});
    expect(host.textContent).toContain("Tell Loomarr where your media is");
    expect(host.textContent).toContain("Found through Emby and Sonarr · Checked 12 minutes ago");
    expect(host.textContent).toContain("Loomarr can open this folder");
    act(() => button("Use this folder").click());
    expect(props.onUseFolder).toHaveBeenCalledWith("/media/tv", true);
    cleanup();
  });

  it("names an offline folder with one way out", () => {
    const { host, props, button, cleanup } = render({ health: "folder_offline", folders: [MOVIES] });
    expect(host.textContent).toContain("Can't reach your Movies folder");
    expect(host.textContent).toContain("Loomarr can't open this folder right now");
    expect(host.textContent).toContain("No app names this folder any more");
    act(() => button("Stop using").click());
    expect(props.onUseFolder).toHaveBeenCalledWith("/media/movies", false);
    act(() =>
      Array.from(host.querySelectorAll("button, [role=button]"))
        .find((n) => n.textContent === "Check again")
        ?.click(),
    );
    expect(props.onCheckAgain).toHaveBeenCalled();
    cleanup();
  });

  it("says all is caught up, and nothing about what keeps it so while all of it works", () => {
    const { host, cleanup } = render({ health: "ok", folders: [TV_IN_USE] });
    expect(host.textContent).toContain("All caught up — every title's ready to air");
    // The folder watch and Sonarr's webhook both work: the section is left out.
    expect(host.textContent).not.toContain("Keeping up to date");
    expect(host.textContent).not.toContain("notices new and deleted files");
    expect(host.textContent).not.toContain("tells Loomarr about new downloads");
    cleanup();
  });

  it("keeps the import's bar, step and time left on screen beside an offline folder", () => {
    const { host, cleanup } = render({
      health: "folder_offline",
      folders: [MOVIES, TV_IN_USE],
      scan: WALKING,
    });
    expect(host.textContent).toContain("Can't reach your Movies folder");
    expect(host.textContent).toContain("Step 1 of 3 · Looking through your folders · 112 of 533");
    expect(host.textContent).toContain("About 9 minutes left in this step");
    expect(host.textContent).toContain("TV / Night Ferry / Season 2");
    expect(host.querySelector('[role="progressbar"]')?.getAttribute("aria-valuenow")).toBe("21");
    expect(host.textContent).not.toContain("All caught up");
    cleanup();
  });

  // An unmounted share leaves its mount point an empty folder: it opens, so it isn't offline, but
  // nothing in it is the library's.
  it("asks whether the drive is connected for an empty folder, and never says all is caught up", () => {
    const { host, props, cleanup } = render({
      health: "folder_empty",
      folders: [{ ...TV_IN_USE, empty: true }],
    });
    expect(host.textContent).toContain("This folder is empty — is the drive connected?");
    expect(host.textContent).not.toContain("Loomarr can open this folder");
    expect(host.textContent).toContain("Up to date");
    expect(host.textContent).not.toContain("All caught up");
    act(() =>
      Array.from(host.querySelectorAll("button, [role=button]"))
        .find((n) => n.textContent === "Check again")
        ?.click(),
    );
    expect(props.onCheckAgain).toHaveBeenCalled();
    cleanup();
  });

  // A lookup that's stuck is never silent: how many wait and when it tries again. 3:40 PM here is
  // local time, as the line shows it.
  const RETRY = new Date(2026, 10, 2, 15, 40).toISOString();
  it.each([
    [true, "Can't reach TMDB — trying again at 3:40 PM"],
    [false, "Trying again at 3:40 PM"],
  ] as const)(
    "says when titles waiting for details are tried again (unreachable: %s)",
    (unreachable, reason) => {
      const { host, cleanup } = render({
        health: "ok",
        folders: [TV_IN_USE],
        details: { waiting: 12, unreachable, retryAt: RETRY },
      });
      expect(host.textContent).toContain("12 titles are waiting on details");
      expect(host.textContent).toContain(reason);
      expect(host.textContent).not.toContain("All caught up");
      expect(host.textContent).toContain("Up to date");
      cleanup();
    },
  );

  it("tells the retry time on the household's clock, whatever the device's", () => {
    const { host, cleanup } = render({
      health: "ok",
      folders: [TV_IN_USE],
      details: { waiting: 12, unreachable: false, retryAt: "2026-11-02T02:40:00Z" },
      timeZone: "Pacific/Auckland",
    });
    expect(host.textContent).toContain("Trying again at 3:40 PM");
    cleanup();
  });

  // A key problem is never the household's to fix: nothing is tried again, and nothing is said.
  it("says nothing of waiting titles when no retry is due", () => {
    const { host, cleanup } = render({
      health: "ok",
      folders: [TV_IN_USE],
      details: { waiting: 12, unreachable: false },
    });
    expect(host.textContent).not.toContain("waiting on details");
    expect(host.textContent).not.toMatch(/key/i);
    cleanup();
  });

  it("leaves waiting titles to the progress while step 3 is looking them up", () => {
    const { host, cleanup } = render({
      health: "ok",
      folders: [TV_IN_USE],
      scan: { ...IDLE, step: "lookingUp", stepNumber: 3, done: 3, total: 15 },
      details: { waiting: 12, unreachable: false, retryAt: RETRY },
    });
    expect(host.textContent).not.toContain("waiting on details");
    cleanup();
  });

  it("says nothing of being caught up beside an offline folder while no import runs", () => {
    const { host, cleanup } = render({ health: "folder_offline", folders: [MOVIES, TV_IN_USE] });
    expect(host.textContent).toContain("Can't reach your Movies folder");
    expect(host.textContent).not.toContain("All caught up");
    expect(host.querySelector('[role="progressbar"]')).toBeNull();
    cleanup();
  });

  it("shows step 1: the folders looked through, the time left and the folder it's in", () => {
    const { host, cleanup } = render({ health: "ok", folders: [TV_IN_USE], scan: WALKING });
    expect(host.textContent).toContain("Adding your media");
    expect(host.textContent).toContain("Step 1 of 3 · Looking through your folders · 112 of 533");
    expect(host.textContent).toContain("About 9 minutes left in this step");
    expect(host.textContent).toContain("TV / Night Ferry / Season 2");
    // The titles' row is shown, not said: no words about it, its place kept from the start.
    expect(host.textContent).not.toContain("Titles appear");
    expect(host.textContent).not.toContain("so far");
    expect(host.querySelector('[role="progressbar"]')?.getAttribute("aria-valuenow")).toBe("21");
    expect(twirls(host)).toBe(1);
    cleanup();
  });

  it("shows step 2 with the file it reads, and says it is still working out the time", () => {
    const { host, cleanup } = render({
      health: "ok",
      folders: [TV_IN_USE],
      scan: { ...READING, current: "Night Ferry S02E03.mkv" },
    });
    expect(host.textContent).toContain("Step 2 of 3 · Reading each file · 25 of 100");
    expect(host.textContent).toContain("Working out how long this takes…");
    expect(host.textContent).toContain("Night Ferry S02E03.mkv");
    expect(host.querySelector('[role="progressbar"]')?.getAttribute("aria-valuenow")).toBe("25");
    cleanup();
  });

  it("shows step 3, the last, with its time left and the title it looks up", () => {
    const { host, cleanup } = render({
      health: "ok",
      folders: [TV_IN_USE],
      scan: {
        ...IDLE,
        step: "lookingUp",
        stepNumber: 3,
        done: 40,
        total: 80,
        etaSeconds: 150,
        current: "Paper Kites (2019)",
      },
    });
    expect(host.textContent).toContain("Step 3 of 3 · Looking up details · 40 of 80");
    expect(host.textContent).toContain("About 3 minutes left");
    expect(host.textContent).not.toContain("in this step");
    expect(host.textContent).toContain("Paper Kites (2019)");
    // Looking up details never holds up a scan.
    expect(host.textContent).toContain("Scan now");
    cleanup();
  });

  it("pauses in plain words, with a still dot and no time or current line", () => {
    const paused = render({
      health: "ok",
      folders: [TV_IN_USE],
      scan: { ...READING, paused: true, current: "Night Ferry S02E03.mkv", etaSeconds: 60 },
    });
    const text = paused.host.textContent;
    expect(text).toContain("Paused while someone is watching");
    expect(text).toContain("Step 2 of 3 · Reading each file · 25 of 100");
    expect(text).toContain("Picks up again when everyone's done watching.");
    expect(text).not.toContain("left");
    expect(text).not.toContain("Working out");
    expect(text).not.toContain("Night Ferry S02E03.mkv");
    expect(twirls(paused.host)).toBe(0);
    paused.cleanup();
  });

  it("says only that it's checking the folders on a quiet pass", () => {
    const { host, cleanup } = render({
      health: "ok",
      folders: [TV_IN_USE],
      scan: { ...WALKING, quiet: true },
    });
    expect(host.textContent).toContain("Checking your folders…");
    expect(host.textContent).not.toContain("Adding your media");
    expect(host.textContent).not.toContain("Step 1 of 3");
    expect(host.querySelector('[role="progressbar"]')).toBeNull();
    expect(twirls(host)).toBe(1);
    cleanup();
  });

  it("keeps the still dot when nothing runs", () => {
    const { host, cleanup } = render({ health: "ok", folders: [TV_IN_USE] });
    expect(twirls(host)).toBe(0);
    cleanup();
  });

  it("shows the first five titles found so far while the import runs, paused or not", () => {
    const reading = READING;
    const readySoFar = ["Quiet Harbor", "Paper Kites", "Tidewater", "Harbour Lights", "Alpha", "Zulu"].map(
      (name, i) => ({ id: `9000095${i}`, name, ready: true }),
    );
    const adding = render({ health: "ok", folders: [TV_IN_USE], scan: reading, readySoFar });
    expect(adding.host.textContent).toContain("Alpha");
    expect(adding.host.textContent).not.toContain("Zulu");
    adding.cleanup();

    const none = render({ health: "ok", folders: [TV_IN_USE], scan: reading, readySoFar: [] });
    expect(none.host.textContent).not.toContain("Alpha");
    none.cleanup();

    // A title still being prepared shows faded; once ready, in full.
    const poster = (id: string) => <span data-poster={id} />;
    const mixed = render({
      health: "ok",
      folders: [TV_IN_USE],
      scan: WALKING,
      readySoFar: [
        { id: "90000960", name: "Quiet Harbor", ready: false, poster: poster("90000960") },
        { id: "90000961", name: "Paper Kites", ready: true, poster: poster("90000961") },
      ],
    });
    expect(mixed.host.textContent).toContain("Quiet Harbor");
    // The frame each poster sits in: faded below full opacity while the title is prepared.
    // The package compiles without the DOM lib, so the jsdom side is typed structurally.
    type Node = { parentElement: Node | null };
    const { getComputedStyle } = globalThis as unknown as {
      getComputedStyle(node: Node): { opacity: string };
    };
    const opacity = (id: string) => {
      let at = (mixed.host.querySelector(`[data-poster="${id}"]`) as Node | null)?.parentElement ?? null;
      while (at && at !== (mixed.host as unknown as Node)) {
        const value = getComputedStyle(at).opacity;
        if (value !== "" && value !== "1") return Number(value);
        at = at.parentElement;
      }
      return 1;
    };
    expect(opacity("90000960")).toBeLessThan(1);
    expect(opacity("90000961")).toBe(1);
    mixed.cleanup();

    const paused = render({
      health: "ok",
      folders: [TV_IN_USE],
      scan: { ...reading, paused: true },
      readySoFar,
    });
    // Titles found stay while the scan waits, and a walk starting again keeps them too.
    expect(paused.host.textContent).toContain("Alpha");
    expect(paused.host.textContent).toContain("Picks up again when everyone's done watching.");
    paused.cleanup();
    const walking = render({
      health: "ok",
      folders: [TV_IN_USE],
      scan: { ...reading, step: "walking" },
      readySoFar,
    });
    expect(walking.host.textContent).toContain("Alpha");
    walking.cleanup();
  });

  it("scans now, and refreshes everything only once asked", () => {
    const { host, props, button, cleanup } = render({ health: "ok", folders: [TV_IN_USE] });
    act(() => button("Scan now").click());
    expect(props.onRescan).toHaveBeenCalled();

    // The confirm may draw outside the host, so look through the whole page.
    const page = (globalThis as unknown as { document: { body: Host } }).document.body;
    const buttons = (name: string) =>
      Array.from(page.querySelectorAll('[role="button"], button')).filter(
        (n) => n.textContent?.trim() === name,
      );
    expect(host.textContent).not.toContain("Refresh every title?");
    act(() => button("Refresh everything").click());
    expect(page.textContent).toContain("Refresh every title?");
    act(() => buttons("Not now")[0]?.click());
    expect(props.onRefreshEverything).not.toHaveBeenCalled();

    act(() => button("Refresh everything").click());
    const confirm = buttons("Refresh everything");
    act(() => confirm[confirm.length - 1]?.click());
    expect(props.onRefreshEverything).toHaveBeenCalledTimes(1);
    cleanup();
  });

  it("says a scan is running and offers no second one", () => {
    const { host, cleanup } = render({ health: "ok", folders: [TV_IN_USE], scan: WALKING });
    expect(host.textContent).toContain("Scanning…");
    expect(host.textContent).not.toContain("Scan now");
    cleanup();
  });

  it("holds a removal until the maintainer answers", () => {
    const { host, props, button, cleanup } = render({
      health: "ok",
      folders: [TV_IN_USE],
      held: [
        {
          path: "/media/movies",
          movies: 2406,
          episodes: 0,
          other: 0,
          titles: ["Amber Hollow", "Brightwater", "Calder Point"],
          titleCount: 2406,
        },
      ],
    });
    expect(host.textContent).toContain("2,406 movies seem to be gone from your Movies folder");
    // What letting them go removes, by name.
    expect(host.textContent).toContain("That's Amber Hollow, Brightwater, Calder Point and 2,403 more.");
    expect(host.textContent).not.toContain("All caught up");
    act(() => button("Check again").click());
    expect(props.onRescan).toHaveBeenCalled();
    act(() => button("They're gone, remove them").click());
    expect(props.onConfirmRemoval).toHaveBeenCalledWith("/media/movies");
    cleanup();
  });

  it("asks for files whose names it can't read to be renamed", () => {
    const { host, props, button, cleanup } = render({
      health: "ok",
      folders: [TV_IN_USE],
      unreadable: [
        "/media/movies/Ferrow �t� (1999).mkv",
        "/media/tv/Kettle Bay/Kettle� 01.mkv",
        "/media/tv/a�.mkv",
        "/media/tv/b�.mkv",
      ],
    });
    expect(host.textContent).toContain("4 files have names Loomarr can't read");
    expect(host.textContent).toContain(
      "Rename Ferrow �t� (1999).mkv, Kettle� 01.mkv, a�.mkv and 1 more using ordinary letters, then check again.",
    );
    expect(host.textContent).not.toContain("All caught up");
    act(() => button("Check again").click());
    expect(props.onRescan).toHaveBeenCalled();
    cleanup();
  });

  it("says which files it skips and what fixes each kind", () => {
    const { host, props, button, cleanup } = render({
      health: "ok",
      folders: [TV_IN_USE],
      skipped: [
        {
          reason: "broken_link",
          total: 5,
          paths: [
            "/media/movies/Gone (2003).mkv",
            "/media/movies/Old Reel.mkv",
            "/media/tv/a.mkv",
            "/media/tv/b.mkv",
          ],
        },
        { reason: "link_outside", total: 1, paths: ["/media/movies/Away (2002).mkv"] },
        { reason: "disc_image", total: 2, paths: ["/media/movies/Tarn.iso", "/media/movies/Wick.iso"] },
        { reason: "empty_file", total: 1, paths: ["/media/movies/Fallow (1971)/Fallow (1971).mkv"] },
        { reason: "identity_conflict", total: 1, paths: ["/media/tv/x.mkv"] },
        { reason: "invalid_facts", total: 1, paths: ["/media/tv/y.mkv"] },
      ],
    });
    expect(host.textContent).toContain("1 video file is empty");
    expect(host.textContent).toContain(
      "Fallow (1971).mkv has nothing in it — a copy that never finished. Copy it again or delete it, then check again.",
    );
    expect(host.textContent).toContain("5 shortcuts point to files that are gone");
    expect(host.textContent).toContain(
      "Gone (2003).mkv, Old Reel.mkv, a.mkv and 2 more. Point them at the files again or delete them, then check again.",
    );
    expect(host.textContent).toContain("1 shortcut leads outside your media folders");
    expect(host.textContent).toContain(
      "Loomarr only reads inside your media folders. Move Away (2002).mkv into one, or add its folder.",
    );
    expect(host.textContent).toContain("2 disc images Loomarr can't play");
    expect(host.textContent).toContain("Rip Tarn.iso and Wick.iso to video files, then check again.");
    // Two reasons Loomarr refuses a file are said as one: they're Loomarr's to fix.
    expect(host.textContent).toContain("2 files Loomarr couldn't add");
    expect(host.textContent).toContain("Loomarr couldn't make sense of x.mkv and y.mkv.");
    expect(host.textContent).not.toContain("All caught up");
    act(() => button("Check again").click());
    expect(props.onRescan).toHaveBeenCalled();
    cleanup();
  });

  it("says what the folder watch and an arr are waiting for", () => {
    const { host, cleanup } = render({
      health: "folder_offline",
      folders: [MOVIES],
      hooks: [
        { app: "sonarr", state: "failed" },
        { app: "radarr", state: "no_address" },
      ],
    });
    expect(host.textContent).toContain(
      "Loomarr notices new and deleted files on its own — waiting for Movies",
    );
    expect(host.textContent).toContain("Sonarr tells Loomarr about new downloads — waiting for Sonarr");
    expect(host.textContent).toContain(
      "Radarr tells Loomarr about new downloads — waiting for Loomarr's address",
    );
    cleanup();
  });

  const SHARE = "/mnt/share";
  const browser = (
    listing: LibraryBrowseListing,
    more: Partial<LibraryFolderBrowser> = {},
  ): LibraryFolderBrowser => ({
    listing,
    onOpen: vi.fn(),
    ...more,
  });
  const TOP: LibraryBrowseListing = {
    path: "",
    parent: "",
    inUse: false,
    holds: [],
    folders: [
      { path: "/mnt/share/TV", name: "/mnt/share/TV", named: true, inUse: false, holds: [] },
      {
        path: SHARE,
        name: SHARE,
        named: false,
        inUse: false,
        holds: [],
        summary: { folders: 12, videos: 340, state: "counted" },
      },
    ],
  };
  const rowFor = (host: Host, name: string) => {
    // A row reads its name, then what it holds: "/mnt/share12 folders · 340 videos", never "/mnt/share/TV…".
    const hit = Array.from(host.querySelectorAll('[role="button"], button')).find(
      (n) => n.textContent?.startsWith(name) && !n.textContent.startsWith(`${name}/`),
    );
    if (!hit) throw new Error(`missing row: ${name}`);
    return hit;
  };
  it("picks a folder by opening folders, the apps' first, each saying what it holds once counted", async () => {
    const folderBrowser = browser(TOP);
    const { host, props, button, cleanup } = render({ folderBrowser });
    act(() => button("Add a folder").click());
    expect(folderBrowser.onOpen).toHaveBeenCalledWith("");
    expect(host.textContent).toContain("From your apps · …");
    expect(host.textContent).toContain("12 folders · 340 videos");
    await act(async () => button("Use this folder").click());
    expect(props.onAddFolder).not.toHaveBeenCalled();
    act(() => rowFor(host, SHARE).click());
    expect(folderBrowser.onOpen).toHaveBeenCalledWith(SHARE);
    cleanup();
  });

  it("uses a folder holding folders in use in their place, and refuses one inside a folder in use", async () => {
    const holding = browser({
      path: SHARE,
      parent: "",
      inUse: false,
      holds: ["/mnt/share/Movies"],
      folders: [
        { path: "/mnt/share/Movies", name: "Movies", named: false, inUse: true, holds: [] },
        {
          path: "/mnt/share/Empty",
          name: "Empty",
          named: false,
          inUse: false,
          holds: [],
          summary: { folders: 0, videos: 0, state: "counted" },
        },
      ],
    });
    const added = vi.fn(async () => undefined);
    const first = render({ folderBrowser: holding, onAddFolder: added });
    act(() => first.button("Add a folder").click());
    expect(first.host.textContent).toContain("In use");
    expect(first.host.textContent).toContain("Empty");
    await act(async () => first.button("Use this folder instead").click());
    expect(added).toHaveBeenCalledWith(SHARE, { replace: true });
    first.cleanup();

    const inside = browser({
      path: "/mnt/share/Movies/Gale Spit (2004)",
      parent: "/mnt/share/Movies",
      inUse: false,
      within: "/mnt/share/Movies",
      holds: [],
      folders: [],
    });
    const second = render({ folderBrowser: inside });
    act(() => second.button("Add a folder").click());
    expect(second.host.textContent).toContain("Loomarr already reads this as part of Movies.");
    await act(async () => second.button("Use this folder").click());
    expect(second.props.onAddFolder).not.toHaveBeenCalled();
    act(() => second.button("Up").click());
    expect(inside.onOpen).toHaveBeenCalledWith("/mnt/share/Movies");
    act(() => second.button("Type a path instead").click());
    expect(second.host.querySelector("input")).not.toBeNull();
    second.cleanup();
  });

  it("names the folder opening on a slow share, holding Up and Use this folder until it is listed", async () => {
    const SLOW = "/mnt/share/Ferrow (1999)";
    const opening = browser(TOP, { opening: SLOW });
    const first = render({ folderBrowser: opening });
    act(() => first.button("Add a folder").click());
    expect(first.host.textContent).toContain(SLOW);
    expect(first.host.textContent).toContain("Opening…");
    expect(first.host.textContent).not.toContain("drives and shares");
    const opened = vi.mocked(opening.onOpen).mock.calls.length;
    act(() => first.button("Up").click());
    expect(opening.onOpen).toHaveBeenCalledTimes(opened);
    await act(async () => first.button("Use this folder").click());
    expect(first.props.onAddFolder).not.toHaveBeenCalled();
    first.cleanup();

    // One that can't be opened leaves Up to go back to the folder it was picked from.
    const listed = { ...TOP, path: SHARE, parent: "" };
    const refused = browser(listed, { opening: SLOW, error: "Loomarr can't open this folder." });
    const second = render({ folderBrowser: refused });
    act(() => second.button("Add a folder").click());
    expect(second.host.textContent).toContain("Loomarr can't open this folder.");
    act(() => second.button("Up").click());
    expect(refused.onOpen).toHaveBeenLastCalledWith(SHARE);
    second.cleanup();
  });

  it("keeps a refused folder in the field with the reason", async () => {
    const { host, props, button, cleanup } = render({});
    act(() => button("Add a folder").click());
    const input = host.querySelector('input[aria-label="Where is the folder?"], input') as El;
    type(input, "/mnt/missing");
    await act(async () => button("Add folder").click());
    expect(props.onAddFolder).toHaveBeenCalledWith("/mnt/missing");
    expect(host.textContent).toContain("Loomarr can't open that folder");
    expect(input.value).toBe("/mnt/missing");
    cleanup();
  });

  const openAddFolder = (overrides: Partial<LibrarySettingsProps> = {}) => {
    const rendered = render(overrides);
    act(() => rendered.button("Add a folder").click());
    const input = rendered.host.querySelector('input[aria-label="Where is the folder?"], input') as El;
    return { ...rendered, input };
  };
  const pressEnter = (input: El) =>
    act(async () => {
      const dom = globalThis as unknown as {
        KeyboardEvent: new (type: string, init: { key: string; bubbles: boolean }) => Event;
      };
      input.dispatchEvent(new dom.KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    });

  it("adds the trimmed path and closes the box once it is accepted", async () => {
    const { host, props, button, input, cleanup } = openAddFolder({
      onAddFolder: vi.fn(async () => undefined),
    });
    type(input, "  /mnt/new  ");
    await act(async () => button("Add folder").click());
    expect(props.onAddFolder).toHaveBeenCalledTimes(1);
    expect(props.onAddFolder).toHaveBeenCalledWith("/mnt/new");
    expect(host.textContent).not.toContain("Where is the folder?");
    expect(host.textContent).toContain("Add a folder");
    cleanup();
  });

  it("adds the path when Enter is pressed in the box", async () => {
    const { props, input, cleanup } = openAddFolder({ onAddFolder: vi.fn(async () => undefined) });
    type(input, "/mnt/enter");
    await pressEnter(input);
    expect(props.onAddFolder).toHaveBeenCalledWith("/mnt/enter");
    cleanup();
  });

  it("has nothing to add until a path is typed", async () => {
    const { props, button, input, cleanup } = openAddFolder({ onAddFolder: vi.fn(async () => undefined) });
    await act(async () => button("Add folder").click());
    type(input, "   ");
    await act(async () => button("Add folder").click());
    expect(props.onAddFolder).not.toHaveBeenCalled();
    cleanup();
  });

  it("clears a refusal as soon as the path is edited", async () => {
    const { host, button, input, cleanup } = openAddFolder();
    type(input, "/mnt/missing");
    await act(async () => button("Add folder").click());
    expect(host.textContent).toContain("Loomarr can't open that folder");
    type(input, "/mnt/missing2");
    expect(host.textContent).not.toContain("Loomarr can't open that folder");
    cleanup();
  });

  it("forgets the path and the refusal when cancelled", async () => {
    const { host, props, button, input, cleanup } = openAddFolder();
    type(input, "/mnt/missing");
    await act(async () => button("Add folder").click());
    act(() => button("Cancel").click());
    expect(host.textContent).not.toContain("Where is the folder?");
    act(() => button("Add a folder").click());
    const reopened = host.querySelector('input[aria-label="Where is the folder?"], input') as El;
    expect(reopened.value).toBe("");
    expect(host.textContent).not.toContain("Loomarr can't open that folder");
    expect(props.onAddFolder).toHaveBeenCalledTimes(1);
    cleanup();
  });

  it("saves a path-map change once when Enter is followed by leaving the field", () => {
    const { host, props, cleanup } = render({ health: "ok", folders: [TV_IN_USE] });
    const input = host.querySelector('input[aria-label="Sonarr /tv maps to"]') as El;
    const dom = globalThis as unknown as {
      KeyboardEvent: new (type: string, init: { key: string; bubbles: boolean }) => Event;
      FocusEvent: new (type: string, init: { bubbles: boolean }) => Event;
    };
    const enter = () =>
      act(() => {
        input.dispatchEvent(new dom.KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
        input.dispatchEvent(new dom.FocusEvent("focusout", { bubbles: true }));
      });
    type(input, "/srv/tv");
    enter();
    expect(props.onMapFolder).toHaveBeenCalledTimes(1);
    expect(props.onMapFolder).toHaveBeenCalledWith({
      authority: "sonarr",
      authorityPath: "/tv",
      localPath: "/srv/tv",
    });
    // A save that failed leaves the map as it was; typing the folder again sends it again.
    type(input, "/srv/t");
    type(input, "/srv/tv");
    enter();
    expect(props.onMapFolder).toHaveBeenCalledTimes(2);
    cleanup();
  });
});
