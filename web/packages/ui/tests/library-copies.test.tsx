// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { LibraryTitle, type LibraryTitleDetail, type LibraryTitleFile } from "../index";

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

const file = (
  name: string,
  state: LibraryTitleFile["state"],
  extra: Partial<LibraryTitleFile> = {},
): LibraryTitleFile => ({
  path: `/media/movies/Paper Kites (1996)/${name}.mkv`,
  size: 4_000_000_000,
  ready: true,
  subtitles: [],
  state,
  ...extra,
});

const UHD = { width: 3840, height: 1600, hdr: true };
const HD = { width: 1920, height: 800, hdr: false };

const movie = (files: LibraryTitleFile[], unplayable = 0): LibraryTitleDetail => ({
  kind: "movie",
  name: "Paper Kites",
  genres: [],
  files,
  unplayable,
  seasons: [],
  match: { by: "none" },
  detailsFrom: {},
  channels: [],
});

const render = (title: LibraryTitleDetail, member = false) => {
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  const onRemoveExtra = vi.fn(() => Promise.resolve());
  const fix = {
    onUpdateDetails: () => Promise.resolve(),
    onCheckFiles: () => Promise.resolve(),
    onSearch: () => Promise.resolve([]),
    onLink: () => Promise.resolve(),
    onForget: () => Promise.resolve(),
    onRemoveExtra,
  };
  act(() =>
    root.render(
      <LoomarrProvider theme="dark">
        <LibraryTitle fix={member ? undefined : fix} onBack={() => {}} title={title} />
      </LoomarrProvider>,
    ),
  );
  const rows = () => Array.from(host.querySelectorAll('[role="listitem"]'), (n) => n.textContent ?? "");
  const removes = () =>
    Array.from(host.querySelectorAll('button, [role="button"]')).filter((n) =>
      n.textContent?.startsWith("Remove"),
    );
  const cleanup = () => {
    act(() => root.unmount());
    host.remove();
  };
  return { cleanup, host, onRemoveExtra, removes, rows };
};

describe("a movie's Copies card", () => {
  it("lists each file by quality, size and what it is", () => {
    const { rows, cleanup } = render(
      movie([
        file("a", "plays", { picture: UHD }),
        file("b", "extra", { picture: HD }),
        file("c", "waiting", { picture: { width: 1280, height: 720, hdr: false } }),
        file("d", "reading", { ready: false }),
        file("e", "unplayable"),
      ]),
    );
    expect(rows()).toEqual([
      expect.stringMatching(/^4K copy.*GB.*Plays$/),
      expect.stringMatching(/^1080p copy.*Extra.*Remove the 1080p copy$/),
      expect.stringMatching(/^720p copy.*Waiting$/),
      expect.stringMatching(/^A copy[^']*Reading$/),
      expect.stringMatching(/^A copy that can't be played.*Can't be played$/),
    ]);
    cleanup();
  });

  it("offers Remove only on an extra copy, and removes that file", async () => {
    const extra = file("b", "extra", { picture: { width: 720, height: 480, hdr: false } });
    const { removes, onRemoveExtra, cleanup } = render(movie([file("a", "plays", { picture: UHD }), extra]));
    expect(removes().map((n) => n.textContent)).toEqual(["Remove the SD copy"]);
    await act(async () => removes()[0]?.click());
    expect(onRemoveExtra).toHaveBeenCalledWith(extra.path);
    cleanup();
  });

  it("tells two copies of one quality apart by size", () => {
    const { removes, cleanup } = render(
      movie([
        file("a", "plays", { picture: UHD }),
        file("b", "extra", { picture: HD }),
        file("c", "extra", { picture: HD, size: 2_000_000_000 }),
      ]),
    );
    const labels = removes().map((n) => n.getAttribute("aria-label"));
    expect(new Set(labels).size).toBe(2);
    cleanup();
  });

  it("calls a broken extra beside a playing copy broken, not the movie", () => {
    const { host, rows, removes, cleanup } = render(
      movie([file("a", "plays", { picture: HD }), file("b", "extra")], 1),
    );
    expect(rows()[1]).toContain("Can't be played");
    expect(removes().map((n) => n.textContent)).toEqual(["Remove the broken copy"]);
    expect(host.textContent).not.toContain("This movie can't be played");
    expect(host.textContent).not.toContain("Check files again");
    cleanup();
  });

  it("says the movie can't be played only when no copy plays", () => {
    const { host, rows, removes, cleanup } = render(movie([file("a", "unplayable")], 1));
    expect(host.textContent).toContain("This movie can't be played");
    expect(host.textContent).toContain("Check files again");
    expect(rows()).toHaveLength(1);
    expect(removes()).toHaveLength(0);
    cleanup();
  });

  it("has no card for one good file, or for a member", () => {
    const one = render(movie([file("a", "plays", { picture: HD })]));
    expect(one.rows()).toHaveLength(0);
    one.cleanup();
    const member = render(movie([file("a", "plays"), file("b", "extra")]), true);
    expect(member.rows()).toHaveLength(0);
    expect(member.removes()).toHaveLength(0);
    member.cleanup();
  });
});
