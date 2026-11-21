// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FileName, LoomarrProvider } from "../index";

// A file name gives way in the middle: the tail (the end of the name and its extension) shows last,
// the head takes what room is left, and the two together are the name once. The cut prefers a word
// start; a single last word longer than the tail may be cut, at a character boundary. Names here
// are invented.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const parts = (name: string) => {
  const { container } = render(
    <LoomarrProvider>
      <FileName name={name} />
    </LoomarrProvider>,
  );
  return {
    container,
    head: container.querySelector('[data-part="head"]')?.textContent ?? null,
    tail: container.querySelector('[data-part="tail"]')?.textContent ?? null,
  };
};

const LONE_SURROGATE = /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/;

describe("FileName shortening", () => {
  it("keeps the last words and the extension of a long name, cutting at a word start", () => {
    const { head, tail } = parts("Ashgrove.Station.1974.Remastered.1080p.mkv");
    expect(tail).toBe("Remastered.1080p.mkv");
    expect(head).toBe("Ashgrove.Station.1974.");
  });

  it("prefers a word start after a space, underscore or hyphen too", () => {
    expect(parts("Beacon Moor - Directors Cut - 1080p.mp4").tail).toBe("Cut - 1080p.mp4");
    cleanup();
    expect(parts("cinder_and_vale_s01e04_extended.m2ts").tail).toBe("s01e04_extended.m2ts");
  });

  it("never shortens a short name", () => {
    const { head, tail } = parts("intro.mkv");
    expect(head).toBeNull();
    expect(tail).toBe("intro.mkv");
  });

  it("keeps the end of a name with no extension", () => {
    const { head, tail } = parts("Brass Orchard Behind The Scenes Reel");
    expect(head).toBe("Brass Orchard Behind ");
    expect(tail).toBe("The Scenes Reel");
  });

  it("may cut a single last word longer than the tail, at a character boundary", () => {
    const { head, tail } = parts("ashgrovestationtheextendedcut.mkv");
    expect(head).toBe("ashgrovestationtheext");
    expect(tail).toBe("endedcut.mkv");
  });

  it("does not read a leading dot or a long run after a dot as an extension", () => {
    expect(parts(".cartographer-daughter-notes").tail).toBe("daughter-notes");
    cleanup();
    expect(parts("Calder Pier Holiday.Footage").tail).toBe("Holiday.Footage");
  });
});

// Each case runs with Intl.Segmenter and with the fallback that joins code points back into
// characters where the runtime has none.
describe.each([
  ["with Intl.Segmenter", false],
  ["without Intl.Segmenter", true],
])("FileName characters, %s", (_, withoutSegmenter) => {
  const split = (name: string) => {
    if (withoutSegmenter) vi.stubGlobal("Intl", { ...Intl, Segmenter: undefined });
    return parts(name);
  };

  it("never splits an emoji's surrogate pair", () => {
    // Twelve emoji, then three letters: counted in UTF-16 units the cut would fall at offset 19,
    // inside the tenth emoji's pair.
    const name = `${"🎬".repeat(12)}abc.mkv`;
    const { head, tail } = split(name);
    expect(head).not.toMatch(LONE_SURROGATE);
    expect(tail).not.toMatch(LONE_SURROGATE);
    expect(`${head}${tail}`).toBe(name);
    // Eight characters, not eight UTF-16 units.
    expect(tail).toBe(`${"🎬".repeat(5)}abc.mkv`);
  });

  it("keeps a skin-toned emoji and a flag whole", () => {
    const thumb = `👍${String.fromCodePoint(0x1f3fd)}`;
    const flag = String.fromCodePoint(0x1f1e8, 0x1f1e6);
    const name = `holidayfootagereelparttwo${thumb}${flag}${thumb}${flag}${thumb}${flag}${thumb}${flag}.mkv`;
    const { head, tail } = split(name);
    expect(`${head}${tail}`).toBe(name);
    expect(tail).toBe(`${`${thumb}${flag}`.repeat(4)}.mkv`);
  });

  it("keeps a combining accent with its letter", () => {
    // The eighth character from the end of the stem, where the cut falls, is "e" plus a combining acute.
    const accented = `e${String.fromCodePoint(0x0301)}`;
    const name = `cartographersdau${accented}tudelon.mkv`;
    const { head, tail } = split(name);
    expect(tail?.startsWith(accented)).toBe(true);
    expect(head).toBe("cartographersdau");
    expect(`${head}${tail}`).toBe(name);
  });

  it("keeps a joined emoji whole", () => {
    const joiner = String.fromCodePoint(0x200d);
    const family = ["👩", "👩", "👧"].join(joiner);
    const name = `holidayfootagereel${family}${family}x.mkv`;
    const { head, tail } = split(name);
    expect(`${head}${tail}`).toBe(name);
    expect(tail?.startsWith(joiner)).toBe(false);
    expect(head?.endsWith(joiner)).toBe(false);
    expect(tail).toContain(family);
  });
});

describe("FileName on the page", () => {
  it("carries the name exactly once, for copy, find and assistive tech", () => {
    const name = "Ashgrove.Station.1974.Remastered.1080p.mkv";
    const { container } = parts(name);
    expect(container.textContent).toBe(name);
    // Tamagui's own config marker is the only hidden element.
    expect(container.querySelector("[aria-hidden]:not([data-tamagui-config-revision])")).toBeNull();
  });

  it("shows the whole name on hover", () => {
    const name = "Ashgrove.Station.1974.Remastered.1080p.mkv";
    const { container } = parts(name);
    expect(container.querySelectorAll(`[title="${name}"]`)).toHaveLength(1);
  });

  it("follows the name's own direction, so a right-to-left name keeps its ellipsis by its tail", () => {
    const { container } = parts("فيلم.العائلة.في.الحديقة.الطويل.mkv");
    expect(container.querySelector('[dir="auto"]')).toBeTruthy();
  });

  it("draws a name that fits as plain inline text, inline in a line and clipped to its box", () => {
    const { container } = parts("Ashgrove.Station.1974.Remastered.1080p.mkv");
    const box = container.querySelector<HTMLElement>('[dir="auto"]');
    expect(box?.style.display).toBe("inline-block");
    expect(box?.style.overflow).toBe("hidden");
    expect(box?.style.maxWidth).toBe("100%");
    expect(box?.style.whiteSpace).toBe("pre");
    // Plain inline parts: find-in-page searches across them.
    for (const drawn of container.querySelectorAll<HTMLElement>("[data-part]")) {
      expect(drawn.getAttribute("style")).toBeNull();
    }
  });

  it("shortens only once the name overflows, leaving the tail's length for the tail", () => {
    // jsdom lays nothing out: the box reports a name twice as wide as itself.
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(private readonly callback: () => void) {}
        observe() {
          this.callback();
        }
        disconnect() {}
      },
    );
    vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(400);
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(200);
    const { container } = parts("Ashgrove.Station.1974.Remastered.1080p.mkv");
    const head = container.querySelector<HTMLElement>('[data-part="head"]');
    const tail = container.querySelector<HTMLElement>('[data-part="tail"]');
    expect(container.querySelector("[data-shortened]")).toBeTruthy();
    expect(head?.style.maxWidth).toBe("calc(100% - 20ch)");
    // A head never narrower than a pixel: a part with no width drops out of a copied selection.
    expect(head?.style.minWidth).toBe("1px");
    expect(head?.style.textOverflow).toBe("ellipsis");
    expect(tail?.style.textOverflow).toBe("ellipsis");
    expect(tail?.style.maxWidth).toBe("calc(100% - 1px)");
    // Still the name once.
    expect(container.textContent).toBe("Ashgrove.Station.1974.Remastered.1080p.mkv");
    vi.restoreAllMocks();
  });

  it("fills a table cell without holding its column open", () => {
    const { container } = render(
      <LoomarrProvider>
        <FileName layout="fill" name="Ashgrove.Station.1974.Remastered.1080p.mkv" />
      </LoomarrProvider>,
    );
    const box = container.querySelector<HTMLElement>('[dir="auto"]');
    expect(box?.style.display).toBe("block");
    expect(box?.style.width).toBe("0px");
    expect(box?.style.minWidth).toBe("100%");
  });

  it("draws a control character as its visible picture, so the name stays on one line", () => {
    const { container } = parts("Beacon Moor\nreel two.mkv");
    const shown = `Beacon Moor${String.fromCodePoint(0x240a)}reel two.mkv`;
    expect(container.textContent).toBe(shown);
    expect(container.querySelector(`[title="${shown}"]`)).toBeTruthy();
  });

  it("sets the name in the data face", () => {
    const { container } = parts("Ashgrove.Station.1974.Remastered.1080p.mkv");
    expect(container.innerHTML).toMatch(/Geist Mono/);
  });
});
