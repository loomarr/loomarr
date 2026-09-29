// @vitest-environment jsdom

import { layoutGuide } from "@loomarr/core/guide";
import { BottomSheet, LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { GuideCompact, type GuideCompactProps } from "../index";

const NOW = Date.UTC(2026, 8, 28, 20, 26);
const HALF_HOUR = 1_800_000;
const FROM = Date.UTC(2026, 8, 28, 20, 0);

const layout = layoutGuide(
  {
    fromMs: FROM,
    toMs: FROM + 4 * HALF_HOUR,
    timezone: "UTC",
    channels: [1, 2, 3].map((n) => ({
      channelId: `ch-${n}`,
      name: `Channel ${n}`,
      number: n,
      pendingCount: 0,
      status: "live" as const,
      airings: [0, 1, 2, 3].map((j) => ({
        kind: "program" as const,
        scheduleBlockId: `ch-${n}-${j}`,
        startMs: FROM + j * HALF_HOUR,
        stopMs: FROM + (j + 1) * HALF_HOUR,
        series: `A series ${n}`,
        title: `Episode ${j + 1}`,
      })),
    })),
  },
  NOW,
);

const markup = (overrides: Partial<GuideCompactProps> = {}) =>
  renderToStaticMarkup(
    <LoomarrProvider>
      <GuideCompact
        filter="all"
        layout={layout}
        myChannels={{ favouriteIds: ["ch-2"], recentIds: [] }}
        nowMs={NOW}
        onFilterChange={vi.fn()}
        onSelect={vi.fn()}
        onWatch={vi.fn()}
        {...overrides}
      />
    </LoomarrProvider>,
  );

describe("GuideCompact", () => {
  it("counts the filters as the mocks write them, and an empty personal filter can't be chosen", () => {
    const html = markup();
    expect(html).toContain("All · 3");
    expect(html).toContain("★ Favorites · 1");
    expect(html).toMatch(
      /aria-disabled="true"[^>]*aria-label="Recent, 0 channels"|aria-label="Recent, 0 channels"[^>]*aria-disabled="true"/,
    );
  });

  it("keeps only the filter's rows, and falls back to All when the filter has emptied", () => {
    expect(markup({ filter: "favourites" })).not.toContain('aria-label="1 Channel 1,');
    expect(markup({ filter: "favourites" })).toContain('aria-label="2 Channel 2,');
    expect(markup({ filter: "recent" })).toContain('aria-label="1 Channel 1,');
  });

  it("docks the selected programme with Watch, and leaves the dock out with nothing selected", () => {
    expect(markup()).not.toContain("Selected programme");
    const html = markup({ selection: { anchorMs: NOW, channelId: "ch-2", scheduleBlockId: "ch-2-2" } });
    expect(html).toContain("Selected programme");
    expect(html).toContain("A series 2 “Episode 3”");
    expect(html).toContain('aria-label="Watch 2 Channel 2"');
  });

  it("docks the iPhone's sheet (5d): the channel, when it starts or ends, and Watch with the number", () => {
    const upcoming = markup({
      dock: BottomSheet,
      selection: { anchorMs: NOW, channelId: "ch-2", scheduleBlockId: "ch-2-2" },
    });
    expect(upcoming).toContain("Selected programme");
    expect(upcoming).toContain(" · Channel 2");
    expect(upcoming).toContain("9:00–9:30 PM · in 34m");
    expect(upcoming).toContain("Watch 2 now");
    const onNow = markup({
      dock: BottomSheet,
      selection: { anchorMs: NOW, channelId: "ch-2", scheduleBlockId: "ch-2-0" },
    });
    expect(onNow).toContain("8:00–8:30 PM · 4m left");
  });

  it("badges now on the ruler", () => {
    expect(markup()).toContain("8:26 PM");
  });
});
