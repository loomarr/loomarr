// @vitest-environment jsdom

import { coverWash, semanticThemes } from "@loomarr/design-system";
import { cleanup, renderUi } from "@loomarr/test-support";
import { afterEach, describe, expect, it, vi } from "vitest";

import { type LibraryCollectionPoster, LibraryCollections } from "../index";

// The window the cover is drawn in, in points: a desktop unless a test says a phone.
const screen = vi.hoisted(() => ({ width: 1440 }));
vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return { ...actual, useWindowDimensions: () => ({ ...actual.useWindowDimensions(), width: screen.width }) };
});

afterEach(() => {
  cleanup();
  screen.width = 1440;
});

const DECADES = ["1950", "1960", "1970", "1980", "1990", "2000", "2010", "2020"] as const;

// A member's collection card draws its pile of posters and nothing else about a title.
const cover = (poster: LibraryCollectionPoster, theme: "dark" | "light" = "dark") => {
  const { container } = renderUi(
    <LibraryCollections
      collections={[{ id: "c1", name: "Weekend", titles: 1, pile: [poster], usedBy: [] }]}
      sets={[]}
    />,
    { theme },
  );
  return container.querySelector<HTMLElement>('[data-testid="library-cover"]');
};

const luminance = (hex: string) => {
  const [r = 0, g = 0, b = 0] = [1, 3, 5]
    .map((at) => Number.parseInt(hex.slice(at, at + 2), 16) / 255)
    .map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};
const contrast = (a: string, b: string) => {
  const [low = 0, high = 0] = [luminance(a), luminance(b)].sort((x, y) => x - y);
  return (high + 0.05) / (low + 0.05);
};

describe("the wash a cover takes", () => {
  it.each(DECADES)("is the %ss' wash from its first year to its last", (decade) => {
    expect(coverWash(Number(decade))).toBe(`coverWash${decade}`);
    expect(coverWash(Number(decade) + 9)).toBe(`coverWash${decade}`);
  });

  it.each([1949, 1927, undefined])("is neutral for %s", (year) => {
    expect(coverWash(year)).toBe("coverWashNeutral");
  });
});

describe("a title with no artwork", () => {
  it("draws its title and its year", () => {
    const drawn = cover({ name: "Paper Kites", year: 1987, kind: "movie" });
    expect(drawn?.textContent).toBe("Paper Kites1987");
  });

  // The caption under the poster says the kind; the cover says only what the caption also says.
  it.each([{ kind: "series" }, { kind: "movie", homeVideo: true }] as const)(
    "says the year alone for %j",
    (facts) => {
      expect(cover({ name: "Upriver", year: 1987, ...facts })?.textContent).toBe("Upriver1987");
    },
  );

  it("has no foot when the year isn't known", () => {
    expect(cover({ name: "Upriver" })?.textContent).toBe("Upriver");
  });

  it("is hidden from assistive tech, which hears the title once from what holds it", () => {
    expect(cover({ name: "Upriver" })?.getAttribute("aria-hidden")).toBe("true");
  });

  it.each(["dark", "light"] as const)("takes the dark colours on a %s page", (theme) => {
    const drawn = cover({ name: "Upriver", year: 1987 }, theme);
    expect(drawn?.closest(".t_dark, .t_light")?.classList.contains("t_dark")).toBe(true);
  });

  it("is not drawn over artwork", () => {
    expect(cover({ name: "Upriver", poster: <span>art</span> })).toBeNull();
  });
});

describe("the words on a cover", () => {
  const { dark, light } = semanticThemes;

  it("sit on the same grounds in both themes", () => {
    expect(light.coverWash).toEqual(dark.coverWash);
  });

  // The title takes the dark theme's primary text; the foot takes the decade's dark tint.
  it.each([...DECADES, "neutral"] as const)("read at 4.5:1 or better on the %s wash", (key) => {
    expect(contrast(dark.content.primary, dark.coverWash[key])).toBeGreaterThanOrEqual(4.5);
    expect(contrast(dark.decadeTint[key], dark.coverWash[key])).toBeGreaterThanOrEqual(4.5);
  });
});
