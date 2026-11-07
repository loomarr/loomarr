// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { LibraryTitle, type LibraryTitleDetail } from "../index";

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
};

const render = (title: LibraryTitleDetail) => {
  const onBack = vi.fn();
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() =>
    root.render(
      <LoomarrProvider theme="dark">
        <LibraryTitle onBack={onBack} title={title} />
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
  return { host, onBack, button, cleanup };
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
});
