// @vitest-environment jsdom

import type { GuideLayout } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

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

describe("the guide's channel rail (#2745)", () => {
  it("scales with the viewport between 200 and 280 points", () => {
    expect(railWidths(1024)).toContain(200);
    expect(railWidths(1468)).toContain(264);
    expect(railWidths(2560)).toContain(280);
    expect(railWidths(1468)).not.toContain(196);
  });

  it("does the same in the web grid, which had a fixed 260", () => {
    expect(gridRailWidths(1024)).toContain(200);
    expect(gridRailWidths(1468)).toContain(264);
    expect(gridRailWidths(1468)).not.toContain(260);
    expect(gridRailWidths(2560)).toContain(280);
  });
});
