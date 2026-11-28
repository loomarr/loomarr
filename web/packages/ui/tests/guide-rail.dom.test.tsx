// @vitest-environment jsdom

import type { GuideLayout } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { act, cleanup, renderUi } from "@loomarr/test-support";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

// The window the guide is drawn in, in points; each test sets it.
const window = vi.hoisted(() => ({ width: 1280 }));

vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return { ...actual, useWindowDimensions: () => ({ ...actual.useWindowDimensions(), width: window.width }) };
});

const { GuideGrid, GuideSurface } = await import("../index");

const layout = {
  channels: [
    {
      airings: [],
      source: {
        airings: [],
        channelId: "morning",
        name: "Saturday Morning Cartoons",
        number: 4,
        pendingCount: 0,
        status: "live",
      },
    },
  ],
  fromMs: 0,
  nowMs: 0,
  timezone: "UTC",
  toMs: 7_200_000,
} as unknown as GuideLayout;

// The rail's width is the one the channel rows and the ruler's spacer are given.
const railWidths = (width: number) => {
  window.width = width;
  const html = renderToStaticMarkup(
    <LoomarrProvider>
      <GuideSurface
        layout={layout}
        onSelectionChange={vi.fn()}
        selection={{ anchorMs: 0, channelId: "morning", scheduleBlockId: "none" }}
      />
    </LoomarrProvider>,
  );
  return [...html.matchAll(/width:(\d+)px/g)].map((match) => Number(match[1]));
};

// The web guide's grid: its ruler's spacer is as wide as the channel column.
const gridRailWidths = (width: number) => {
  window.width = width;
  const html = renderToStaticMarkup(
    <LoomarrProvider>
      <GuideGrid layout={layout} nowMs={0} />
    </LoomarrProvider>,
  );
  return [...html.matchAll(/width:(\d+)px/g)].map((match) => Number(match[1]));
};

describe("the rail's decade bar (#2767)", () => {
  afterEach(cleanup);

  // The rail's leading bar and the number's colour, as jsdom reports them (theme variables, lower-cased).
  const rail = async (decade?: number) => {
    window.width = 1280;
    const row = layout.channels[0] as GuideLayout["channels"][number];
    const source = decade === undefined ? row.source : { ...row.source, decade };
    let view!: ReturnType<typeof renderUi>;
    await act(async () => {
      view = renderUi(<GuideGrid layout={{ ...layout, channels: [{ ...row, source }] }} nowMs={0} />);
    });
    const host = view.container;
    const bars = Array.from(host.querySelectorAll('[aria-hidden="true"]')).filter(
      (el) => el.textContent === "",
    );
    const number = Array.from(host.querySelectorAll("span")).find((el) => el.textContent === "4") as Element;
    const out = {
      bars: bars.map((el) => getComputedStyle(el).backgroundColor.toLowerCase()),
      number: getComputedStyle(number).color.toLowerCase(),
    };
    await act(async () => view.unmount());
    return out;
  };

  it("draws the channel's decade colour as the rail's bar, leaving the number neutral", async () => {
    const { bars, number } = await rail(1980);
    expect(bars).toContain("var(--decade1980)");
    expect(number).toBe("var(--contentprimary)");
  });

  it("keeps the bar clear without a decade", async () => {
    const { bars } = await rail();
    expect(bars.some((bar) => bar.includes("decade"))).toBe(false);
  });
});

describe("the guide's channel rail (#2745)", () => {
  it("scales with the viewport between 200 and 300 points", () => {
    expect(railWidths(900)).toContain(200);
    expect(railWidths(1280)).toContain(256);
    expect(railWidths(2560)).toContain(300);
    expect(railWidths(1468)).not.toContain(196);
  });

  it("does the same in the web grid, which had a fixed 260", () => {
    expect(gridRailWidths(900)).toContain(200);
    expect(gridRailWidths(1280)).toContain(256);
    expect(gridRailWidths(1280)).not.toContain(260);
    expect(gridRailWidths(2560)).toContain(300);
  });
});
