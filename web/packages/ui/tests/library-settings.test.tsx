// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { type LibraryFolder, type LibraryScan, LibrarySettings, type LibrarySettingsProps } from "../index";

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

  it("says all is caught up and what keeps it so, leaving out an arr never connected", () => {
    const { host, cleanup } = render({ health: "ok", folders: [TV_IN_USE] });
    expect(host.textContent).toContain("All caught up — every title's ready to air");
    expect(host.textContent).toContain("Keeping up to date");
    expect(host.textContent).toContain("Loomarr notices new and deleted files on its own");
    expect(host.textContent).toContain("Sonarr tells Loomarr about new downloads");
    expect(host.textContent).not.toContain("— waiting");
    expect(host.textContent).not.toContain("Radarr");
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
    expect(host.textContent).not.toContain("Titles appear as they're ready.");
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
    expect(host.textContent).toContain("Titles appear as they're ready.");
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

  it("shows the first five titles ready so far while the import runs, and none while paused", () => {
    const reading = READING;
    const readySoFar = ["Quiet Harbor", "Paper Kites", "Tidewater", "Harbour Lights", "Alpha", "Zulu"].map(
      (name, i) => ({ id: `9000095${i}`, name }),
    );
    const adding = render({ health: "ok", folders: [TV_IN_USE], scan: reading, readySoFar });
    expect(adding.host.textContent).toContain("Ready so far");
    expect(adding.host.textContent).toContain("Alpha");
    expect(adding.host.textContent).not.toContain("Zulu");
    adding.cleanup();

    const none = render({ health: "ok", folders: [TV_IN_USE], scan: reading, readySoFar: [] });
    expect(none.host.textContent).not.toContain("Ready so far");
    none.cleanup();

    const paused = render({
      health: "ok",
      folders: [TV_IN_USE],
      scan: { ...reading, paused: true },
      readySoFar,
    });
    expect(paused.host.textContent).not.toContain("Ready so far");
    paused.cleanup();
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
      held: [{ path: "/media/movies", movies: 2406, episodes: 0, other: 0 }],
    });
    expect(host.textContent).toContain("2,406 movies seem to be gone from your Movies folder");
    expect(host.textContent).not.toContain("All caught up");
    act(() => button("Check again").click());
    expect(props.onRescan).toHaveBeenCalled();
    act(() => button("They're gone, remove them").click());
    expect(props.onConfirmRemoval).toHaveBeenCalledWith("/media/movies");
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
