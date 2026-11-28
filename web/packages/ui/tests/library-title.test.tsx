// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import {
  LibraryTitle,
  type LibraryTitleDetail,
  type LibraryTitleFix,
  type LibraryTitleProps,
} from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type El = { click(): void; getAttribute(n: string): string | null; textContent: string | null };
type Host = Parameters<typeof createRoot>[0] & {
  querySelectorAll(s: string): ArrayLike<El>;
  remove(): void;
  textContent: string | null;
};
const doc = (
  globalThis as unknown as {
    document: {
      body: { append(n: unknown): void; querySelectorAll(s: string): ArrayLike<El> };
      createElement(t: string): Host;
    };
  }
).document;

const MOVIE: LibraryTitleDetail = {
  kind: "movie",
  name: "Paper Kites",
  year: 1996,
  overview: "A small-town salesman is bursting at the seams.",
  genres: ["Crime", "Thriller"],
  rating: "R",
  runtimeMillis: 5_880_000,
  files: [
    {
      path: "/media/movies/Paper Kites (1996)/Paper Kites.mkv",
      size: 8_100_000_000,
      lengthMillis: 5_880_000,
      picture: { width: 3840, height: 1600, hdr: true },
      audioChannels: 6,
      subtitles: [{ language: "eng" }, { language: "eng" }, {}],
      state: "plays",
    },
  ],
  unplayable: 0,
  seasons: [],
  match: { by: "app", app: "Radarr" },
  detailsFrom: { description: "TMDB", rating: "TMDB" },
  channels: [
    { id: "ch-90000932", name: "Late Show", number: 7 },
    { id: "ch-90000931", name: "Quiet Hours", number: 21 },
  ],
};

const SERIES: LibraryTitleDetail = {
  kind: "series",
  name: "Harbour Lights",
  year: 1999,
  genres: [],
  files: [],
  unplayable: 0,
  seasons: [
    {
      number: 0,
      episodes: [{ id: "90000911", name: "Christmas at the Harbour", number: 1, ready: true }],
    },
    {
      number: 1,
      episodes: [
        { id: "90000901", name: "Pilot", number: 1, end: 2, ready: true },
        { id: "90000902", name: "", number: 3, ready: false },
      ],
    },
  ],
  match: { by: "none" },
  detailsFrom: { description: "TMDB", episodeNames: "Sonarr" },
  channels: [],
};

// An admin's page unless `member`, whose page has no fixes.
const render = (
  title: LibraryTitleDetail,
  over: Partial<LibraryTitleFix> = {},
  member = false,
  extra: Pick<LibraryTitleProps, "watchLink"> = {},
) => {
  const onBack = vi.fn();
  const fix: LibraryTitleFix = {
    onUpdateDetails: vi.fn(() => Promise.resolve()),
    onCheckFiles: vi.fn(() => Promise.resolve()),
    onSearch: vi.fn(() => Promise.resolve([])),
    onLink: vi.fn(() => Promise.resolve()),
    onForget: vi.fn(() => Promise.resolve()),
    onRemoveExtra: vi.fn(() => Promise.resolve()),
    ...over,
  };
  const props: LibraryTitleProps = { title, onBack, fix: member ? undefined : fix, ...extra };
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  const draw = (next: LibraryTitleProps) =>
    act(() =>
      root.render(
        <LoomarrProvider theme="dark">
          <LibraryTitle {...next} />
        </LoomarrProvider>,
      ),
    );
  draw(props);
  // The same page again, with some of its fixes changed.
  const rerender = (changed: Partial<LibraryTitleFix>) => draw({ ...props, fix: { ...fix, ...changed } });
  const button = (name: string) => {
    const hit = Array.from(host.querySelectorAll('[role="button"], button')).find((n) =>
      n.textContent?.trim().startsWith(name),
    );
    if (!hit) throw new Error(`missing button: ${name}`);
    return hit;
  };
  // The seasons' tabs, in the order drawn.
  const tabs = () => Array.from(host.querySelectorAll('[role="tab"]'));
  const tab = (name: string) => {
    const hit = tabs().find((n) => n.textContent?.trim().startsWith(name));
    if (!hit) throw new Error(`missing tab: ${name}`);
    return hit;
  };
  const cleanup = () => {
    act(() => root.unmount());
    host.remove();
  };
  // The Fix menu is portalled to the body, so its items are looked up there.
  const menuItems = () => Array.from(doc.body.querySelectorAll('[role="menuitem"]'));
  const openFix = () => act(async () => button("Fix").click());
  const fixItem = async (name: string) => {
    await openFix();
    const item = menuItems().find((n) => n.textContent?.trim() === name);
    if (!item) throw new Error(`missing menu item: ${name}`);
    await act(async () => item.click());
  };
  return { host, onBack, props, rerender, button, tab, tabs, cleanup, menuItems, openFix, fixItem };
};

describe("LibraryTitle", () => {
  it("shows a cast member's photo, and initials for one without", () => {
    const { host } = render({
      ...MOVIE,
      credits: [
        { name: "Imelda Varge", role: "actor", photo: <img alt="" data-testid="cast-photo" /> },
        { name: "Tobin Ashcroft", role: "actor" },
      ],
    });
    const cast = host.querySelectorAll('[aria-label="Cast"]')[0];
    expect(host.querySelectorAll('[aria-label="Cast"] [data-testid="cast-photo"]')).toHaveLength(1);
    expect(cast?.textContent).not.toContain("IV");
    expect(cast?.textContent).toContain("TA");
  });

  it("shows a movie's facts in plain words and its file behind Technical details", () => {
    const { host, button, cleanup } = render(MOVIE);
    expect(host.textContent).toContain("1996·1 h 38 min");
    expect(host.textContent).toContain("R");
    expect(host.textContent).toContain("Crime, Thriller");
    expect(host.textContent).toContain("bursting at the seams");
    // Closed, the file is mounted but hidden until found (find-in-page can still reach it).
    const hiddenFile = () =>
      Array.from(host.querySelectorAll('[hidden="until-found"]')).some((n) =>
        n.textContent?.includes("Paper Kites.mkv"),
      );
    expect(hiddenFile()).toBe(true);
    act(() => button("Technical details").click());
    expect(hiddenFile()).toBe(false);
    expect(host.textContent).toContain("/media/movies/Paper Kites (1996)/Paper Kites.mkv");
    expect(host.textContent).toContain("7.5 GB");
    expect(host.textContent).toContain("Description and rating from TMDB.");
    expect(host.textContent).toContain("Recognised from Radarr");
    // A widescreen 4K film is cropped below 2160 lines, and is still 4K.
    expect(host.textContent).toContain("4K, extra-rich colour");
    expect(host.textContent).toContain("Surround 5.1");
    expect(host.textContent).toContain("English, Unknown");
    cleanup();
  });

  it("says a clip shorter than a minute is, rather than 0 min", () => {
    const short = {
      ...MOVIE.files[0],
      lengthMillis: 20_000,
      subtitles: [],
    } as LibraryTitleDetail["files"][number];
    const { host, button, cleanup } = render({ ...MOVIE, runtimeMillis: undefined, files: [short] });
    act(() => button("Technical details").click());
    expect(host.textContent).toContain("Under a minute");
    expect(host.textContent).not.toContain("0 min");
    cleanup();
  });

  it("names each part of a movie split over several files", () => {
    const part = (n: number) =>
      ({
        ...MOVIE.files[0],
        path: `/media/movies/Paper Kites (1996)/Paper Kites - cd${n}.mkv`,
        part: n,
      }) as LibraryTitleDetail["files"][number];
    const { host, button, cleanup } = render({ ...MOVIE, files: [part(1), part(2)] });
    act(() => button("Technical details").click());
    expect(host.textContent).toContain("Part 1 of 2");
    expect(host.textContent).toContain("Part 2 of 2");
    expect(host.textContent).not.toContain("Extra");
    cleanup();
  });

  it("offers to remove an extra copy, and says when it couldn't", async () => {
    const extra = {
      ...MOVIE.files[0],
      path: "/media/movies/Paper Kites (1996)/Paper Kites DVD.mkv",
      size: 4_300_000_000,
      state: "extra",
    } as LibraryTitleDetail["files"][number];
    const single = render(MOVIE);
    expect(single.host.textContent).not.toContain("Copies");
    single.cleanup();

    const onRemoveExtra = vi.fn(() => Promise.reject(new Error("read-only")));
    const { host, button, cleanup } = render({ ...MOVIE, files: [...MOVIE.files, extra] }, { onRemoveExtra });
    // The action says which copy it moves, by quality.
    await act(async () => button("Remove the 4K copy").click());
    expect(onRemoveExtra).toHaveBeenCalledWith(extra.path);
    expect(host.textContent).toContain(
      "Couldn't remove it. Loomarr may not be allowed to change this folder.",
    );
    act(() => button("Technical details").click());
    expect(host.textContent).toContain("Extra copy");
    cleanup();
  });

  it("says when files can't be played, checks them again, and says when it couldn't", async () => {
    const fine = render(MOVIE);
    expect(fine.host.textContent).not.toContain("can't be played");
    fine.cleanup();

    const broken = { ...MOVIE.files[0], picture: undefined, state: "unplayable" };
    const movie = render({ ...MOVIE, files: [broken as LibraryTitleDetail["files"][number]], unplayable: 1 });
    expect(movie.host.textContent).toContain("This movie can't be played. The file may be damaged");
    await act(async () => movie.button("Check files again").click());
    expect(movie.props.fix?.onCheckFiles).toHaveBeenCalled();
    expect(movie.host.textContent).toContain("Checked just now");
    movie.cleanup();

    const onCheckFiles = vi.fn(() => Promise.reject(new Error("offline")));
    const series = render({ ...SERIES, unplayable: 2 }, { onCheckFiles });
    expect(series.host.textContent).toContain("2 episodes can't be played. The files may be damaged");
    await act(async () => series.button("Check files again").click());
    expect(series.host.textContent).toContain("Couldn't check the files just now. Try again in a moment.");
    series.cleanup();
  });

  it("names the channels it's on, by number, and has no such section when it's on none", () => {
    const movie = render(MOVIE);
    const section = movie.host.querySelectorAll('[aria-label="On these channels"]');
    expect(section.length).toBe(1);
    const text = section[0]?.textContent ?? "";
    expect(text).toContain("On these channels");
    expect(text.indexOf("Late Show")).toBeGreaterThan(-1);
    expect(text.indexOf("Late Show")).toBeLessThan(text.indexOf("Quiet Hours"));
    movie.cleanup();

    const series = render(SERIES);
    expect(series.host.querySelectorAll('[aria-label="On these channels"]').length).toBe(0);
    expect(series.host.textContent).toContain("Not on a channel yet");
    series.cleanup();
  });

  it("makes a channel's row the link to watching it, with its logo, where the app can name its player", () => {
    const withLogo: LibraryTitleDetail = {
      ...MOVIE,
      channels: [{ ...MOVIE.channels[0]!, logo: <span data-logo="late-show" /> }, MOVIE.channels[1]!],
    };
    const watchLink = (id: string) =>
      // The row is drawn into the link in place of its children.
      id === "ch-90000932" ? <a href={`/watch/${id}`}>Watch</a> : undefined;
    const page = render(withLogo, {}, true, { watchLink });
    const links = page.host.querySelectorAll('a[href="/watch/ch-90000932"]');
    expect(links.length).toBe(1);
    expect(links[0]?.textContent).toContain("Late Show");
    expect(page.host.querySelectorAll('a[href="/watch/ch-90000932"] [data-logo="late-show"]').length).toBe(1);
    // A channel the app gives no player stays a plain row.
    expect(page.host.querySelectorAll("a").length).toBe(1);
    expect(page.host.textContent).toContain("Quiet Hours");
    page.cleanup();
  });

  it("offers an admin Collections for a title on no channel, and a member only the sentence", () => {
    const collectionsLink = <a href="/library/collections">Collections</a>;
    const admin = render(SERIES, { collectionsLink });
    const link = admin.host.querySelectorAll('a[href="/library/collections"]');
    expect(link.length).toBe(1);
    expect(link[0]?.textContent).toContain("Add it to a collection");
    admin.cleanup();

    const member = render(SERIES, { collectionsLink }, true);
    expect(member.host.querySelectorAll('a[href="/library/collections"]').length).toBe(0);
    expect(member.host.textContent).toContain("Not on a channel yet");
    member.cleanup();
  });

  it("goes back to the Library", () => {
    const { onBack, button, cleanup } = render(MOVIE);
    act(() => button("Library").click());
    expect(onBack).toHaveBeenCalled();
    cleanup();
  });

  it("lists a series' seasons, specials last, each opening to its episodes", () => {
    const { host, button, tab, tabs, cleanup } = render(SERIES);
    // Specials aren't a season, so the count leaves them out.
    expect(host.textContent).toContain("1999·1 season");
    act(() => button("Technical details").click());
    expect(host.textContent).toContain("Description from TMDB. Episode names from Sonarr.");
    expect(host.textContent).toContain("Not recognised yet");
    // The data lists the specials first.
    expect(tabs().map((n) => n.textContent?.replace(/\d+$/, "").trim())).toEqual(["Season 1", "Specials"]);
    act(() => tab("Specials").click());
    expect(host.textContent).toContain("Christmas at the Harbour");
    expect(host.textContent).not.toContain("Pilot");
    act(() => tab("Season 1").click());
    expect(host.textContent).not.toContain("Christmas at the Harbour");
    expect(host.textContent).toContain("1–2");
    expect(host.textContent).toContain("Pilot");
    // An episode with no name yet reads by its number, and one not read yet says so.
    expect(host.textContent).toContain("Episode 3");
    expect(host.textContent).toContain("Getting ready");
    // Only the episode getting ready twirls; the ready one beside it doesn't.
    expect(host.querySelectorAll('[data-testid="library-twirl"]').length).toBe(1);
    cleanup();
  });

  it("shows no number for a special that has none", () => {
    const unnumbered: LibraryTitleDetail = {
      ...SERIES,
      seasons: [
        { number: 0, episodes: [{ id: "90000921", name: "Harbour - The Movie", number: 0, ready: true }] },
      ],
    };
    const { host, tab, cleanup } = render(unnumbered);
    act(() => tab("Specials").click());
    expect(host.textContent).toContain("Harbour - The Movie");
    expect(host.textContent).not.toContain("0. Harbour - The Movie");
    cleanup();
  });

  // #2151 decision 53: episodes that wait on the show's order say so, and Choose opens Needs you.
  it("says which episodes wait on the show's episode order, and opens Needs you", () => {
    const onChooseOrder = vi.fn();
    const waiting: LibraryTitleDetail = {
      ...SERIES,
      seasons: [
        {
          number: 1,
          episodes: [
            { id: "90000901", name: "Pilot", number: 1, ready: true },
            { id: "90000902", name: "Harbour Lights S01E02", number: 2, ready: true, fromFile: true },
          ],
        },
      ],
    };
    const { host, button, tab, cleanup } = render(waiting, { onChooseOrder });
    expect(host.textContent).toContain("We're not sure which order these episodes are in.");
    act(() => tab("Season 1").click());
    expect(host.textContent?.match(/Name from the file/g)).toHaveLength(1);
    act(() => button("Choose").click());
    expect(onChooseOrder).toHaveBeenCalled();
    cleanup();
    // Nothing waits: no hint.
    const plain = render(SERIES, { onChooseOrder });
    expect(plain.host.textContent).not.toContain("We're not sure which order");
    plain.cleanup();
  });

  it("checks the files again and says so, and says when updating the details failed", async () => {
    const onUpdateDetails = vi.fn(() => Promise.reject(new Error("tmdb down")));
    const { host, props, fixItem, cleanup } = render(MOVIE, { onUpdateDetails });
    await fixItem("Check files again");
    expect(props.fix?.onCheckFiles).toHaveBeenCalled();
    expect(host.textContent).toContain("Checked just now");
    await fixItem("Update details");
    expect(onUpdateDetails).toHaveBeenCalled();
    expect(host.textContent).toContain("Couldn't update the details just now. Try again in a moment.");
    cleanup();
  });

  it("sets a dot between each of the facts", () => {
    const { host, cleanup } = render({ ...MOVIE, score: 7.4 });
    // The first dot is outside the clipped row (the story measures that); none trails the last fact.
    expect(host.textContent).toContain("·1996·1 h 38 min·R·Crime, Thriller·Score 7.4/10A small-town");
    cleanup();
  });

  const READ_FILE = {
    ...MOVIE.files[0],
    path: "/media/tv/Harbour Lights/S01E01.mkv",
    container: "matroska,webm",
    streams: [
      {
        index: 0,
        kind: "video",
        codec: "h264",
        profile: "High",
        frameRate: "24000/1001",
        bitrate: 9_800_000,
      },
      { index: 1, kind: "audio", codec: "ac3", channels: 6, languageName: "English", default: true },
      { index: 2, kind: "audio", codec: "aac", channels: 2, languageName: "French" },
      { index: 3, kind: "subtitle", codec: "subrip", languageName: "Spanish", forced: true },
    ],
    chapters: [{ startMillis: 0, name: "Cold open" }, { startMillis: 3_725_000 }],
  } as unknown as LibraryTitleDetail["files"][number];

  it("opens an episode's own file from its row, and closes it", async () => {
    const onEpisodeFiles = vi.fn(() => Promise.resolve([READ_FILE]));
    const { host, button, cleanup } = render(SERIES, { onEpisodeFiles });
    expect(onEpisodeFiles).not.toHaveBeenCalled();
    await act(async () => button("1–2. Pilot").click());
    expect(onEpisodeFiles).toHaveBeenCalledWith("90000901");
    expect(host.textContent).toContain("This episode's file");
    expect(host.textContent).toContain(READ_FILE.path);
    await act(async () => button("1–2. Pilot").click());
    expect(host.textContent).not.toContain("This episode's file");
    cleanup();
  });

  it("says when an episode's file couldn't be loaded", async () => {
    const onEpisodeFiles = vi.fn(() => Promise.reject(new Error("offline")));
    const { host, button, cleanup } = render(SERIES, { onEpisodeFiles });
    await act(async () => button("1–2. Pilot").click());
    expect(host.textContent).toContain("Couldn't load this file's details. Try again in a moment.");
    cleanup();
  });

  it("shows a file's tracks and chapters behind More, to an admin only", () => {
    const { host, button, cleanup } = render({ ...MOVIE, files: [READ_FILE] });
    act(() => button("Technical details").click());
    expect(host.textContent).not.toContain("Cold open");
    act(() => button("More about this file").click());
    for (const words of [
      "Matroska (MKV)",
      "H.264, High profile",
      "9.8 Mb/s",
      "23.976 frames a second",
      "None (standard range)",
      "EnglishSurround 5.1 · AC-3 · default",
      "FrenchStereo · AAC",
      "SpanishForced · SRT",
      "0:00 · Cold open",
      "1:02:05 · Chapter 2",
    ]) {
      expect(host.textContent).toContain(words);
    }
    cleanup();

    const member = render({ ...MOVIE, files: [READ_FILE] }, {}, true);
    expect(member.host.textContent).not.toContain("More about this file");
    expect(member.host.textContent).not.toContain("Cold open");
    member.cleanup();
  });

  it("names each More by its file, and holds it to the touch target on a phone", () => {
    const view = globalThis as unknown as { innerWidth: number };
    const wide = view.innerWidth;
    view.innerWidth = 390;
    const second = { ...READ_FILE, path: "/media/tv/Harbour Lights/copy.mkv" };
    const { host, button, cleanup } = render({ ...MOVIE, files: [READ_FILE, second] });
    act(() => button("Technical details").click());
    const mores = Array.from(host.querySelectorAll('[aria-label^="More about this file"]')) as (El & {
      style: { minHeight: string };
    })[];
    expect(mores.map((n) => n.getAttribute("aria-label"))).toEqual([
      "More about this file, S01E01.mkv",
      "More about this file, copy.mkv",
    ]);
    expect(mores.map((n) => n.style.minHeight)).toEqual(["48px", "48px"]);
    cleanup();
    view.innerWidth = wide;
  });

  it("adds the folder to a file's name only when two files share it", async () => {
    const at = (path: string, extra: boolean) => ({
      ...READ_FILE,
      path,
      state: extra ? ("extra" as const) : ("plays" as const),
    });
    const { host, button, cleanup } = render({
      ...MOVIE,
      files: [
        at("/media/Kites/Kites.mkv", false),
        at("/media/Kites/Extras/Kites.mkv", true),
        at("/media/Kites/Kites DVD.mkv", true),
      ],
    });
    act(() => button("Technical details").click());
    expect(
      Array.from(host.querySelectorAll('[aria-label^="More about this file"]')).map((n) =>
        n.getAttribute("aria-label"),
      ),
    ).toEqual([
      "More about this file, Kites.mkv in Kites",
      "More about this file, Kites.mkv in Extras",
      "More about this file, Kites DVD.mkv",
    ]);
    // The card says the way back once the toast's Undo has gone.
    expect(host.textContent).toContain(
      "You can undo right after. After that, the file is in the .loomarr-trash folder inside its media folder for 30 days.",
    );
    cleanup();
  });

  it("announces an episode's file loading and failing in a region already there", async () => {
    let fail: (reason: Error) => void = () => {};
    const onEpisodeFiles = () =>
      new Promise<never>((_, reject) => {
        fail = reject;
      });
    const { host, button, cleanup } = render(SERIES, { onEpisodeFiles });
    const regions = () => Array.from(host.querySelectorAll('[aria-live="polite"]'));
    const before = regions();
    await act(async () => button("1–2. Pilot").click());
    const said = () => before.map((n) => n.textContent).join("");
    expect(said()).toBe("Loading the file's details…");
    await act(async () => fail(new Error("offline")));
    expect(said()).toBe("Couldn't load this file's details. Try again in a moment.");
    cleanup();
  });

  it("reads an open episode's file again when the title is read again", async () => {
    const onEpisodeFiles = vi
      .fn<(id: string) => Promise<LibraryTitleDetail["files"]>>()
      .mockResolvedValueOnce([READ_FILE])
      .mockResolvedValue([{ ...READ_FILE, path: "/media/tv/Harbour Lights/replaced.mkv" }]);
    const { host, button, rerender, cleanup } = render(SERIES, { onEpisodeFiles, episodeFilesAt: 1 });
    await act(async () => button("1–2. Pilot").click());
    expect(host.textContent).toContain(READ_FILE.path);
    await act(async () => rerender({ episodeFilesAt: 2 }));
    expect(host.textContent).toContain("replaced.mkv");
    cleanup();
  });

  it("gives a member episode rows that don't open", () => {
    const { host, button, cleanup } = render(SERIES, {}, true);
    expect(host.textContent).toContain("Pilot");
    expect(() => button("1–2. Pilot")).toThrow();
    cleanup();
  });

  it("puts focus back on Fix when Wrong title? is closed", async () => {
    const { button, fixItem, cleanup } = render(MOVIE);
    await fixItem("Wrong title?");
    const focused = () =>
      (globalThis as unknown as { document: { activeElement: unknown } }).document.activeElement;
    const close = button("Close") as El & { focus(): void };
    close.focus();
    await act(async () => close.click());
    expect(focused()).toBe(button("Fix"));
    cleanup();
  });

  it("has its live region in place before a fix says anything", async () => {
    const { host, fixItem, cleanup } = render(MOVIE, { onCheckFiles: () => new Promise<void>(() => {}) });
    const live = host.querySelectorAll('[aria-live="polite"]');
    expect(live.length).toBe(1);
    const region = live[0];
    expect(region?.textContent).toBe("");
    await fixItem("Check files again");
    expect(region?.textContent).toBe("Checking…");
    cleanup();
  });

  it("shows Wrong title? as open in the menu, and closes it", async () => {
    const { host, button, fixItem, openFix, menuItems, cleanup } = render(MOVIE);
    await fixItem("Wrong title?");
    await openFix();
    expect(menuItems().map((n) => n.textContent?.trim())).toContain("Wrong title? (open)");
    await act(async () => button("Close").click());
    expect(host.textContent).not.toContain("Radarr decides what this is");
    cleanup();
  });

  it("holds the Fix menu's items to the touch target on a phone", async () => {
    const view = globalThis as unknown as { innerWidth: number };
    const wide = view.innerWidth;
    view.innerWidth = 390;
    const { menuItems, openFix, cleanup } = render(MOVIE);
    await openFix();
    expect(menuItems().map((n) => (n as El & { style: { minHeight: string } }).style.minHeight)).toEqual([
      "48px",
      "48px",
      "48px",
    ]);
    cleanup();
    view.innerWidth = wide;
  });

  it("sends a title its app decides to that app", async () => {
    const onOpenApp = vi.fn();
    const { host, button, fixItem, cleanup } = render(MOVIE, { onOpenApp });
    await fixItem("Wrong title?");
    expect(host.textContent).toContain("Radarr decides what this is. Change it in Radarr.");
    expect(host.textContent).not.toContain("Search for the title");
    act(() => button("Open Radarr").click());
    expect(onOpenApp).toHaveBeenCalled();
    cleanup();
  });

  it("says a title its file decides is fixed in the file", async () => {
    const { host, fixItem, cleanup } = render({ ...MOVIE, match: { by: "file" } });
    await fixItem("Wrong title?");
    expect(host.textContent).toContain(
      "This file's name says what it is. Rename it, or change its NFO, to fix it.",
    );
    expect(host.textContent).not.toContain("Search for the title");
    cleanup();
  });

  it("lets the household pick the title, or forget an answer it gave", async () => {
    const unknown = render(SERIES);
    await unknown.fixItem("Wrong title?");
    expect(unknown.host.textContent).toContain("Search for the title");
    expect(unknown.host.textContent).not.toContain("Forget my answer");
    unknown.cleanup();

    const onForget = vi.fn(() => Promise.resolve());
    const answered = render({ ...SERIES, match: { by: "you" } }, { onForget });
    await answered.fixItem("Wrong title?");
    await act(async () => answered.button("Forget my answer").click());
    expect(onForget).toHaveBeenCalled();
    answered.cleanup();
  });

  // #4476: a member reads the page with nothing to fix, so nothing on it calls an admin route.
  it("gives a member the story and the channels, with no fixes and no file cards", () => {
    const extra = {
      ...MOVIE.files[0],
      path: "/media/movies/Paper Kites (1996)/copy.mkv",
      size: 4_300_000_000,
      state: "extra",
    } as LibraryTitleDetail["files"][number];
    const { host, props, cleanup } = render(
      { ...MOVIE, files: [...MOVIE.files, extra], unplayable: 1 },
      {},
      true,
    );
    expect(host.textContent).toContain(MOVIE.name);
    // No Fix menu, so none of what it holds can be reached.
    expect(
      Array.from(host.querySelectorAll('[role="button"], button')).map((n) => n.textContent?.trim()),
    ).not.toContain("Fix");
    for (const word of [
      "Update details",
      "Check files again",
      "Wrong title?",
      "Technical details",
      "can't be played",
      "Remove the",
      "Copies",
    ]) {
      expect(host.textContent).not.toContain(word);
    }
    act(() => {
      const back = Array.from(host.querySelectorAll('[role="button"], button')).find((n) =>
        n.textContent?.includes("Library"),
      );
      back?.click();
    });
    expect(props.onBack).toHaveBeenCalled();
    cleanup();
  });
});
