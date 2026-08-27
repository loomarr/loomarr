import type { GuideLayout } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { GuideExperience, GuideSurface } from "../index";

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
  it("matches the dense Compose TV grid with inline filters and a bottom detail band", () => {
    const output = renderToStaticMarkup(
      <LoomarrProvider>
        <GuideSurface density="tv" layout={layout} onSelectionChange={vi.fn()} selection={selection} />
      </LoomarrProvider>,
    );
    expect(output).toContain(">Guide<");
    expect(output).toContain("All · 1");
    expect(output).toContain("★ Favourites · 0");
    expect(output).toContain("Recent · 0");
    expect(output).toContain("▲ Filters");
    expect(output).toContain("CHANNEL");
    expect(output).toContain("Bart the Mother");
    expect(output).toContain("S10E03 · 1998 · TV-PG");
    expect(output).not.toContain("1 channels ·");
  });

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

  it("renders only the platform-provided channel window while retaining total position", () => {
    const secondChannel = {
      ...layout.channels[0]!,
      airings: layout.channels[0]!.airings.map((airing) => ({
        ...airing,
        channelId: "shelbyville",
        scheduleBlockId: "news",
        source: { ...airing.source, scheduleBlockId: "news", title: "Shelbyville News" },
      })),
      source: {
        ...layout.channels[0]!.source,
        channelId: "shelbyville",
        name: "Shelbyville News",
        number: 2,
      },
    };
    const windowedLayout = { ...layout, channels: [...layout.channels, secondChannel] };
    const output = renderToStaticMarkup(
      <LoomarrProvider>
        <GuideSurface
          channelWindow={{ end: 2, positionLabel: "2 of 2", start: 1 }}
          layout={windowedLayout}
          onSelectionChange={vi.fn()}
          selection={{ anchorMs: 900_000, channelId: "shelbyville", scheduleBlockId: "news" }}
        />
      </LoomarrProvider>,
    );

    expect(output).toContain("Shelbyville News");
    expect(output).not.toContain("Springfield Classics");
    expect(output).toContain("2 channels · 2 of 2");
  });

  it("selects one airing when schedule block identities repeat across channels", () => {
    const duplicate = {
      ...layout.channels[0]!,
      airings: layout.channels[0]!.airings.map((airing) => ({
        ...airing,
        channelId: "shelbyville",
      })),
      source: {
        ...layout.channels[0]!.source,
        channelId: "shelbyville",
        name: "Shelbyville Classics",
        number: 2,
      },
    };
    const output = renderToStaticMarkup(
      <LoomarrProvider>
        <GuideSurface
          density="tv"
          layout={{ ...layout, channels: [...layout.channels, duplicate] }}
          onSelectionChange={vi.fn()}
          selection={selection}
        />
      </LoomarrProvider>,
    );

    // The selected All filter and exactly one airing advertise selected state.
    expect(output.match(/aria-pressed="true"/g)).toHaveLength(2);
  });

  it.each([
    ["loading", "Loading channels"],
    ["empty", "No channels on air"],
    ["error", "Guide unavailable"],
    ["offline", "You&#x27;re offline"],
  ] as const)("owns the %s guide state", (state, title) => {
    const output = renderToStaticMarkup(
      <LoomarrProvider>
        <GuideExperience onRetry={vi.fn()} state={state} />
      </LoomarrProvider>,
    );
    expect(output).toContain(title);
    expect(output.includes("Try again")).toBe(state === "error" || state === "offline");
  });
});
