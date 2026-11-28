// @vitest-environment jsdom

import { act, renderUi } from "@loomarr/test-support";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { LibraryTitle, type LibraryTitleDetail, type LibraryTitleFile } from "../index";

const UHD = { width: 3840, height: 1600, hdr: true };
const HD = { width: 1920, height: 800, hdr: false };

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

const CHANNEL = { id: "ch-104", number: 104, name: "Premium Reels" };

const movie = (over: Partial<LibraryTitleDetail> = {}): LibraryTitleDetail => ({
  kind: "movie",
  name: "Paper Kites",
  genres: [],
  files: [],
  unplayable: 0,
  seasons: [],
  match: { by: "none" },
  detailsFrom: {},
  channels: [],
  ...over,
});

const fix = (onRemoveExtra: (path: string) => Promise<void>) => ({
  onUpdateDetails: () => Promise.resolve(),
  onCheckFiles: () => Promise.resolve(),
  onSearch: () => Promise.resolve([]),
  onLink: () => Promise.resolve(),
  onForget: () => Promise.resolve(),
  onRemoveExtra,
});

const watchLink = (id: string) => <a href={`/watch/${id}`}>Watch</a>;

const channelPage = (airing: { when: string; episode?: string }, linked = true) =>
  renderUi(
    <LibraryTitle
      onBack={() => {}}
      title={movie({
        channels: [CHANNEL],
        airings: [{ channel: CHANNEL, forecast: false, ...airing }],
      })}
      watchLink={linked ? watchLink : undefined}
    />,
  );

const region = (host: HTMLElement) => host.querySelector('[aria-label="On these channels"]');

describe("a title's channel row", () => {
  it("says Watch, is named for watching the channel, and draws the channel's initials", () => {
    const { container, unmount } = channelPage({ when: "Next today at 4:02 AM" });
    const link = container.querySelector("a");
    expect(link?.getAttribute("aria-label")).toBe("Watch 104 Premium Reels, next today at 4:02 AM");
    expect(link?.textContent).toContain("Next today at 4:02 AM");
    expect(link?.textContent).toContain("Watch");
    // The placeholder tile holds the initials; the number is written once.
    expect(link?.querySelector('[aria-hidden="true"]')?.textContent).toBe("PR");
    expect(link?.textContent?.split("104")).toHaveLength(2);
    unmount();
  });

  it("reads On now · Watch while on air, and keeps the episode in what it says", () => {
    const { container, unmount } = channelPage({ when: "On now", episode: "S1 E2 Low Tide" });
    const link = container.querySelector("a");
    expect(link?.textContent).toContain("On now · Watch");
    expect(link?.textContent?.split("On now")).toHaveLength(2);
    expect(link?.getAttribute("aria-label")).toBe("Watch 104 Premium Reels, on now");
    const described = link?.getAttribute("aria-describedby") ?? "";
    expect(document.getElementById(described)?.textContent).toBe("S1 E2 Low Tide");
    unmount();
  });

  it("leaves an unlinked row without a verb", () => {
    const { container, unmount } = channelPage({ when: "On now" }, false);
    expect(container.querySelector("a")).toBeNull();
    expect(region(container)?.textContent).toContain("On now");
    expect(region(container)?.textContent).not.toContain("Watch");
    unmount();
  });
});

// The page as the app holds it: a removal that goes through takes the file out of the title.
const Page = ({ files, fails = false }: { files: LibraryTitleFile[]; fails?: boolean }) => {
  const [held, setHeld] = useState(files);
  const remove = (path: string) => {
    if (fails) return Promise.reject(new Error("read only"));
    setHeld((now) => now.filter((f) => f.path !== path));
    return Promise.resolve();
  };
  return <LibraryTitle fix={fix(remove)} onBack={() => {}} title={movie({ files: held })} />;
};

const removes = (host: HTMLElement) =>
  Array.from(host.querySelectorAll<HTMLElement>('button, [role="button"]')).filter((n) =>
    n.textContent?.startsWith("Remove"),
  );

const press = async (button: HTMLElement | undefined) => {
  await act(async () => {
    button?.focus();
    button?.click();
  });
};

const KEPT = file("kept", "plays", { picture: UHD });
const FIRST = file("first", "extra", { picture: HD });
const SECOND = file("second", "extra", { picture: HD, size: 2_000_000_000 });

describe("focus after removing a copy", () => {
  it("moves to the next copy's Remove", async () => {
    const { container, unmount } = renderUi(<Page files={[KEPT, FIRST, SECOND]} />);
    const [first, second] = removes(container);
    await press(first);
    expect(document.activeElement).toBe(second);
    unmount();
  });

  it("moves to the Copies heading when no copy follows", async () => {
    const { container, unmount } = renderUi(<Page files={[KEPT, FIRST, SECOND]} />);
    await press(removes(container)[1]);
    expect(document.activeElement?.textContent).toBe("Copies");
    unmount();
  });

  it("moves to the page heading when the card goes too", async () => {
    const { container, unmount } = renderUi(<Page files={[KEPT, FIRST]} />);
    await press(removes(container)[0]);
    expect(document.activeElement?.textContent).toBe("Paper Kites");
    unmount();
  });

  it("stays put when the removal fails", async () => {
    const { container, unmount } = renderUi(<Page fails files={[KEPT, FIRST, SECOND]} />);
    const [first] = removes(container);
    await press(first);
    expect(document.activeElement).toBe(first);
    unmount();
  });
});

describe("technical details", () => {
  it("labels each file in the Copies card's words", async () => {
    const files = [KEPT, file("broken", "extra"), file("bad", "unplayable"), file("new", "reading")];
    const { container, unmount } = renderUi(<Page files={files} />);
    const toggle = Array.from(container.querySelectorAll<HTMLElement>('[role="button"], button')).find((n) =>
      n.textContent?.startsWith("Technical details"),
    );
    await press(toggle);
    const details = document.getElementById(toggle?.getAttribute("aria-controls") ?? "");
    for (const words of ["Plays", "Extra · can't be played", "Can't be played", "Reading"]) {
      expect(details?.textContent).toContain(words);
    }
    expect(details?.textContent).not.toContain("Extra copy");
    unmount();
  });
});

describe("a title page on a phone", () => {
  const view = globalThis as unknown as { innerWidth: number };
  const wide = view.innerWidth;
  afterEach(() => {
    view.innerWidth = wide;
  });

  it("holds Technical details and Check files again to the touch target", () => {
    view.innerWidth = 390;
    const { container, unmount } = renderUi(
      <LibraryTitle
        fix={fix(() => Promise.resolve())}
        onBack={() => {}}
        title={movie({ files: [file("bad", "unplayable")], unplayable: 1 })}
      />,
    );
    for (const label of ["Technical details", "Check files again"]) {
      const button = Array.from(container.querySelectorAll<HTMLElement>('[role="button"], button')).find(
        (n) => n.textContent?.startsWith(label),
      );
      expect(Number.parseInt(button ? getComputedStyle(button).minHeight : "", 10)).toBeGreaterThanOrEqual(
        44,
      );
    }
    unmount();
  });
});
