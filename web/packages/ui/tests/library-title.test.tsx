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
  files: [{ path: "/media/movies/Paper Kites (1996)/Paper Kites.mkv", size: 8_100_000_000 }],
  seasons: [],
  match: { by: "app", app: "Radarr" },
};

const SERIES: LibraryTitleDetail = {
  kind: "series",
  name: "Harbour Lights",
  year: 1999,
  genres: [],
  files: [],
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
    const { host, onBack, button, cleanup } = render(MOVIE);
    expect(host.textContent).toContain("1996 · 1 h 38 min");
    expect(host.textContent).toContain("R");
    expect(host.textContent).toContain("Crime, Thriller");
    expect(host.textContent).toContain("bursting at the seams");
    expect(host.textContent).not.toContain("Paper Kites.mkv");
    act(() => button("Technical details").click());
    expect(host.textContent).toContain("/media/movies/Paper Kites (1996)/Paper Kites.mkv");
    expect(host.textContent).toContain("7.5 GB");
    act(() => button("Library").click());
    expect(onBack).toHaveBeenCalled();
    cleanup();
  });

  it("lists a series' seasons, specials last, each opening to its episodes", () => {
    const { host, button, cleanup } = render(SERIES);
    // Specials aren't a season, so the count leaves them out.
    expect(host.textContent).toContain("1999 · 1 season");
    expect(host.textContent).not.toContain("Technical details");
    const text = host.textContent ?? "";
    expect(text.indexOf("Season 1")).toBeLessThan(text.indexOf("Specials"));
    act(() => button("Season 1").click());
    expect(host.textContent).toContain("1–2");
    expect(host.textContent).toContain("Pilot");
    // An episode with no name yet reads by its number, and one not read yet says so.
    expect(host.textContent).toContain("Episode 3");
    expect(host.textContent).toContain("Getting ready");
    cleanup();
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
