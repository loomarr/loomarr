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
const IDLE: LibraryScan = { enabled: true, stage: "idle", headersDone: 0, headersTotal: 0, paused: false };

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

  it("shows the import's progress, and pauses it in plain words", () => {
    const reading: LibraryScan = { ...IDLE, stage: "reading", headersDone: 25, headersTotal: 100 };
    const adding = render({ health: "ok", folders: [TV_IN_USE], scan: reading });
    expect(adding.host.textContent).toContain("Adding your media");
    expect(adding.host.textContent).toContain("Titles appear as they're ready.");
    expect(adding.host.querySelector('[role="progressbar"]')?.getAttribute("aria-valuenow")).toBe("25");
    adding.cleanup();

    const paused = render({ health: "ok", folders: [TV_IN_USE], scan: { ...reading, paused: true } });
    expect(paused.host.textContent).toContain("Paused while someone is watching");
    expect(paused.host.textContent).toContain("Picks up again when everyone's done watching.");
    paused.cleanup();
  });

  it("shows the first five titles ready so far while the import runs, and none while paused", () => {
    const reading: LibraryScan = { ...IDLE, stage: "reading", headersDone: 25, headersTotal: 100 };
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
