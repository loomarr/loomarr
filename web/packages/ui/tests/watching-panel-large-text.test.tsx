import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

const window = vi.hoisted(() => ({ fontScale: 1 }));

vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return { ...actual, useWindowDimensions: () => ({ ...actual.useWindowDimensions(), ...window }) };
});

import { WatchingPanel, type WatchingPanelProps } from "../index";
import { declarationsOf, tagBefore } from "./support/atomic-css";

const CONTROLS = ["Previous", "Channel −", "Pause", "Channel +"];

const props = (overrides: Partial<WatchingPanelProps> = {}): WatchingPanelProps => ({
  canPrevious: true,
  canSurf: true,
  channel: { name: "Late Night Science Fiction", number: "7" },
  density: "touch",
  favourites: [],
  live: { lagSeconds: 95, mode: "behind" },
  onChannelDown: vi.fn(),
  onChannelUp: vi.fn(),
  onGoLive: vi.fn(),
  onPause: vi.fn(),
  onPlay: vi.fn(),
  onPrevious: vi.fn(),
  onTune: vi.fn(),
  ...overrides,
});

const markupAt = (fontScale: number, overrides: Partial<WatchingPanelProps> = {}) => {
  window.fontScale = fontScale;
  return renderToStaticMarkup(
    <LoomarrProvider>
      <WatchingPanel {...props(overrides)} />
    </LoomarrProvider>,
  );
};

// The class list of the element whose text is exactly `text`. A one-line (`numberOfLines={1}`) text
// carries the ellipsis class; a wrapping one doesn't.
const stylesOf = (html: string, text: string) => {
  const tag = tagBefore(html, `${text}</`);
  if (!tag) throw new Error(`no element reads "${text}"`);
  return declarationsOf(html, tag);
};
const clips = (html: string, text: string) => stylesOf(html, text).includes("text-overflow:ellipsis");

describe("WatchingPanel at an accessibility text size", () => {
  beforeEach(() => {
    window.fontScale = 1;
  });

  it("keeps the four one-line tiles in a single row at an ordinary size, even the largest", () => {
    const html = markupAt(1.24);
    for (const label of CONTROLS) expect(clips(html, label)).toBe(true);
    expect(html).not.toContain("flex-wrap:wrap");
    expect(html.match(/style="flex-direction:row;gap:8px;margin-top:8px"/g)).toHaveLength(1);
  });

  // A tile label stays one line and shrinks to fit on native (`adjustsFontSizeToFit`), so a word is
  // never broken ("Previou/s"); the static markup can only show the full label is rendered.
  it("lays the controls out as a two-by-two grid with every full label", () => {
    const html = markupAt(1.65);
    for (const label of CONTROLS) expect(html).toContain(`>${label}</`);
    // Two rows of two under one column, and no row of four.
    expect(html.match(/style="flex-direction:row;gap:8px"/g)).toHaveLength(2);
    expect(html).not.toContain("margin-top:8px;flex-direction:row");
  });

  it("offers Play in the grid while paused", () => {
    const html = markupAt(1.65, { live: { lagSeconds: 20, mode: "paused" } });
    expect(html).toContain(">Play</");
    expect(html).not.toContain(">Pause</");
  });

  it("lets the channel name wrap and keeps Go Live in view", () => {
    const large = markupAt(1.65);
    const ordinary = markupAt(1);
    expect(clips(ordinary, "Late Night Science Fiction")).toBe(true);
    expect(clips(large, "Late Night Science Fiction")).toBe(false);
    expect(large).toContain(">Go Live</");
    expect(large).toContain("flex-wrap:wrap");
    expect(ordinary).not.toContain("flex-wrap:wrap");
  });
});
