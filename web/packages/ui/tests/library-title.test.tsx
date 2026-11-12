// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { LibraryTitle, type LibraryTitleDetail, type LibraryTitleProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type El = { click(): void; getAttribute(n: string): string | null; textContent: string | null };
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
      extra: false,
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

const render = (title: LibraryTitleDetail, over: Partial<LibraryTitleProps> = {}) => {
  const onBack = vi.fn();
  const props: LibraryTitleProps = {
    title,
    onBack,
    onUpdateDetails: vi.fn(() => Promise.resolve()),
    onCheckFiles: vi.fn(() => Promise.resolve()),
    onSearch: vi.fn(() => Promise.resolve([])),
    onLink: vi.fn(() => Promise.resolve()),
    onForget: vi.fn(() => Promise.resolve()),
    onRemoveExtra: vi.fn(() => Promise.resolve()),
    ...over,
  };
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() =>
    root.render(
      <LoomarrProvider theme="dark">
        <LibraryTitle {...props} />
      </LoomarrProvider>,
    ),
  );
  const button = (name: string) => {
    const hit = Array.from(host.querySelectorAll('[role="button"], button')).find((n) =>
      n.textContent?.trim().startsWith(name),
    );
    if (!hit) throw new Error(`missing button: ${name}`);
    return hit;
  };
  const cleanup = () => {
    act(() => root.unmount());
    host.remove();
  };
  return { host, onBack, props, button, cleanup };
};

describe("LibraryTitle", () => {
  it("shows a movie's facts in plain words and its file behind Technical details", () => {
    const { host, button, cleanup } = render(MOVIE);
    expect(host.textContent).toContain("1996 · 1 h 38 min");
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

  it("offers to remove an extra copy, and says when it couldn't", async () => {
    const extra = {
      ...MOVIE.files[0],
      path: "/media/movies/Paper Kites (1996)/Paper Kites DVD.mkv",
      size: 4_300_000_000,
      extra: true,
    } as LibraryTitleDetail["files"][number];
    const single = render(MOVIE);
    expect(single.host.textContent).not.toContain("An extra copy");
    single.cleanup();

    const onRemoveExtra = vi.fn(() => Promise.reject(new Error("read-only")));
    const { host, button, cleanup } = render({ ...MOVIE, files: [...MOVIE.files, extra] }, { onRemoveExtra });
    expect(host.textContent).toContain("An extra copy is taking 4.0 GB");
    await act(async () => button("Remove extra copy").click());
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

    const movie = render({ ...MOVIE, unplayable: 1 });
    expect(movie.host.textContent).toContain("This movie can't be played — the file may be damaged");
    await act(async () => movie.button("Check again").click());
    expect(movie.props.onCheckFiles).toHaveBeenCalled();
    expect(movie.host.textContent).toContain("Checked just now");
    movie.cleanup();

    const onCheckFiles = vi.fn(() => Promise.reject(new Error("offline")));
    const series = render({ ...SERIES, unplayable: 2 }, { onCheckFiles });
    expect(series.host.textContent).toContain("2 episodes can't be played — the files may be damaged");
    await act(async () => series.button("Check again").click());
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
    series.cleanup();
  });

  it("goes back to the Library", () => {
    const { onBack, button, cleanup } = render(MOVIE);
    act(() => button("Library").click());
    expect(onBack).toHaveBeenCalled();
    cleanup();
  });

  it("lists a series' seasons, specials last, each opening to its episodes", () => {
    const { host, button, cleanup } = render(SERIES);
    // Specials aren't a season, so the count leaves them out.
    expect(host.textContent).toContain("1999 · 1 season");
    act(() => button("Technical details").click());
    expect(host.textContent).toContain("Description from TMDB. Episode names from Sonarr.");
    expect(host.textContent).toContain("Not recognised yet");
    const text = host.textContent ?? "";
    expect(text.indexOf("Season 1")).toBeLessThan(text.indexOf("Specials"));
    act(() => button("Season 1").click());
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
    const { host, button, cleanup } = render(unnumbered);
    act(() => button("Specials").click());
    expect(host.textContent).toContain("Harbour - The Movie");
    expect(host.textContent).not.toContain("0Harbour - The Movie");
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
    const { host, button, cleanup } = render(waiting, { onChooseOrder });
    expect(host.textContent).toContain("We're not sure which order these episodes are in.");
    act(() => button("Season 1").click());
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
    const { host, props, button, cleanup } = render(MOVIE, { onUpdateDetails });
    await act(async () => button("Check files again").click());
    expect(props.onCheckFiles).toHaveBeenCalled();
    expect(host.textContent).toContain("Checked just now");
    await act(async () => button("Update details").click());
    expect(onUpdateDetails).toHaveBeenCalled();
    expect(host.textContent).toContain("Couldn't update the details just now. Try again in a moment.");
    cleanup();
  });

  it("sends a title its app decides to that app", () => {
    const onOpenApp = vi.fn();
    const { host, button, cleanup } = render(MOVIE, { onOpenApp });
    act(() => button("Wrong title?").click());
    expect(host.textContent).toContain("Radarr decides what this is. Change it in Radarr.");
    expect(host.textContent).not.toContain("Search for the title");
    act(() => button("Open Radarr").click());
    expect(onOpenApp).toHaveBeenCalled();
    cleanup();
  });

  it("says a title its file decides is fixed in the file", () => {
    const { host, button, cleanup } = render({ ...MOVIE, match: { by: "file" } });
    act(() => button("Wrong title?").click());
    expect(host.textContent).toContain(
      "This file's name says what it is. Rename it, or change its NFO, to fix it.",
    );
    expect(host.textContent).not.toContain("Search for the title");
    cleanup();
  });

  it("lets the household pick the title, or forget an answer it gave", async () => {
    const unknown = render(SERIES);
    act(() => unknown.button("Wrong title?").click());
    expect(unknown.host.textContent).toContain("Search for the title");
    expect(unknown.host.textContent).not.toContain("Forget my answer");
    unknown.cleanup();

    const onForget = vi.fn(() => Promise.resolve());
    const answered = render({ ...SERIES, match: { by: "you" } }, { onForget });
    act(() => answered.button("Wrong title?").click());
    await act(async () => answered.button("Forget my answer").click());
    expect(onForget).toHaveBeenCalled();
    answered.cleanup();
  });
});
