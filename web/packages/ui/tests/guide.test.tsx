import type { GuideLayout } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { GuideSurface } from "../index";

const layout = {
  channels: [
    {
      airings: [
        {
          channelId: "springfield",
          clippedStartMs: 0,
          clippedStopMs: 1_800_000,
          endsAfterWindow: false,
          isOnNow: true,
          progressRatio: 0.5,
          scheduleBlockId: "bart",
          source: {
            description: "Bart cares for an injured bird.",
            episode: 3,
            genres: ["Animation", "Comedy"],
            kind: "program",
            rating: "TV-PG",
            scheduleBlockId: "bart",
            season: 10,
            series: "The Simpsons",
            startMs: 0,
            stopMs: 1_800_000,
            title: "Bart the Mother",
            year: 1998,
          },
          startRatio: 0,
          startsBeforeWindow: false,
          widthRatio: 0.5,
        },
      ],
      source: {
        airings: [],
        channelId: "springfield",
        name: "Springfield Classics",
        number: 1,
        pendingCount: 0,
        status: "live",
      },
    },
  ],
  fromMs: 0,
  source: { channels: [], fromMs: 0, timezone: "UTC", toMs: 3_600_000 },
  timezone: "UTC",
  toMs: 3_600_000,
} satisfies GuideLayout;

const selection = { anchorMs: 900_000, channelId: "springfield", scheduleBlockId: "bart" };

const markup = () =>
  renderToStaticMarkup(
    <LoomarrProvider>
      <GuideSurface layout={layout} onSelectionChange={vi.fn()} selection={selection} />
    </LoomarrProvider>,
  );

describe("GuideSurface", () => {
  it("renders authoritative channel, programme, episode, and detail facts", () => {
    const output = markup();
    expect(output).toContain("Springfield Classics");
    expect(output).toContain("Bart the Mother");
    expect(output).toContain("S10E03");
    expect(output).toContain("1998 · TV-PG · Animation · Comedy");
    expect(output).toContain("12:00 AM");
  });

  it("publishes one labelled tuning action and disables empty optional filters", () => {
    const output = markup();
    expect(output).toContain("Springfield Classics, The Simpsons · Bart the Mother, 12:00 AM–12:30 AM");
    expect(output).toContain('aria-label="Favourites channels"');
    expect(output).toContain('aria-label="Recent channels"');
    expect(output.match(/aria-disabled="true"/g)).toHaveLength(2);
  });
});
