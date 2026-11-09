// @vitest-environment jsdom

import { layoutGuide } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { GuideNowNext, GuidePhoneHeader } from "../index";

const MINUTE = 60_000;
const FROM = Date.UTC(2026, 8, 28, 20, 0);
const NOW = FROM + 7 * MINUTE;

const at = (minutes: number) => FROM + minutes * MINUTE;
const program = (id: string, start: number, stop: number, title: string, series?: string) => ({
  kind: "program" as const,
  scheduleBlockId: id,
  series,
  startMs: at(start),
  stopMs: at(stop),
  title,
});

const layout = layoutGuide(
  {
    fromMs: FROM,
    toMs: at(240),
    timezone: "UTC",
    channels: [
      {
        channelId: "kids",
        name: "Cartoon Classics",
        number: 2,
        pendingCount: 0,
        status: "live" as const,
        airings: [
          program("k0", -15, 25, "The Upside-Down Picnic", "Kettle Kids"),
          program("k1", 25, 47, "Snow Day for Gulls", "Kettle Kids"),
        ],
      },
      {
        channelId: "ads",
        name: "Sitcom Corner",
        number: 5,
        pendingCount: 0,
        status: "live" as const,
        airings: [
          {
            kind: "filler" as const,
            scheduleBlockId: "a0",
            startMs: at(0),
            stopMs: at(10),
            title: "",
          },
          program("a1", 10, 32, "The Spare Key", "Two Doors Down"),
        ],
      },
      {
        channelId: "new",
        name: "Late Movies",
        number: 9,
        pendingCount: 0,
        status: "building" as const,
        airings: [
          {
            kind: "filler" as const,
            scheduleBlockId: "n0",
            startMs: at(-5),
            stopMs: at(60),
            title: "",
          },
        ],
      },
      {
        channelId: "last",
        name: "Matinee Westerns",
        number: 7,
        pendingCount: 0,
        status: "live" as const,
        airings: [program("w0", -30, 200, "A Long Film")],
      },
    ],
  },
  NOW,
);

const markup = (onWatch = vi.fn()) =>
  renderToStaticMarkup(
    <LoomarrProvider>
      <GuideNowNext layout={layout} nowMs={NOW} onWatch={onWatch} />
    </LoomarrProvider>,
  );

describe("GuideNowNext", () => {
  it("is one row per channel, in the guide's order", () => {
    const html = markup();
    const names = ["Cartoon Classics", "Sitcom Corner", "Late Movies", "Matinee Westerns"];
    const positions = names.map((name) => html.indexOf(`>${name}<`));
    expect(positions.every((position) => position >= 0)).toBe(true);
    expect(positions).toEqual([...positions].sort((a, b) => a - b));
  });

  it("says what is on, how long is left, and what comes next", () => {
    const html = markup();
    expect(html).toContain(">Kettle Kids · The Upside-Down Picnic<");
    expect(html).toContain(">18 min left<");
    expect(html).toContain(">8:25 PM<");
    expect(html).toContain(">Kettle Kids · Snow Day for Gulls<");
    // A row is named by what it draws, never by a written label that says something else: the
    // list's own "Channel schedule" is the only label.
    expect(html.match(/aria-label="[^"]*"/g)).toEqual(['aria-label="Channel schedule"']);
  });

  it("names a break and skips it when naming what is next", () => {
    const html = markup();
    expect(html).toContain(">Commercial break<");
    expect(html).toContain(">Two Doors Down · The Spare Key<");
    // Only the two programmes on now count down; a break and a downloading channel do not.
    expect(html.match(/ left</g)).toHaveLength(2);
  });

  it("says a channel still downloading is, and what plays meanwhile", () => {
    const html = markup();
    expect(html).toContain(">Getting its shows ready<");
    expect(html).toContain(">Commercials play until its shows arrive.<");
    expect(html).toContain(">When its first show arrives<");
  });

  it("says later tonight when nothing is listed next", () => {
    const html = markup();
    expect(html).toContain(">A Long Film<");
    expect(html).toContain(">Later tonight<");
    expect(html).toContain(">—<");
    expect(html).toContain(">3 hr 13 min left<");
  });
});

describe("GuideNowNext number block", () => {
  type El = { parentElement: El | null; textContent: string | null };
  const g = globalThis as unknown as {
    document: {
      body: { append(n: unknown): void; querySelectorAll(s: string): ArrayLike<El> };
      createElement(t: string): Parameters<typeof createRoot>[0] & { remove(): void };
    };
    getComputedStyle(n: El): { backgroundColor: string; color: string };
    IS_REACT_ACT_ENVIRONMENT: boolean;
  };

  it("fills a channel's number with its decade colour and leaves the rest clear", async () => {
    g.IS_REACT_ACT_ENVIRONMENT = true;
    const eraLayout = layoutGuide(
      {
        channels: [
          {
            airings: [],
            channelId: "eighties",
            decade: 1980,
            name: "Late Night Sci-Fi",
            number: 3,
            pendingCount: 0,
            status: "live" as const,
          },
          {
            airings: [],
            channelId: "plain",
            name: "Nature Hour",
            number: 12,
            pendingCount: 0,
            status: "live" as const,
          },
        ],
        fromMs: FROM,
        timezone: "UTC",
        toMs: at(240),
      },
      NOW,
    );
    const host = g.document.createElement("div");
    g.document.body.append(host);
    const root = createRoot(host);
    await act(async () =>
      root.render(
        <LoomarrProvider theme="dark">
          <GuideNowNext layout={eraLayout} nowMs={NOW} onWatch={vi.fn()} />
        </LoomarrProvider>,
      ),
    );
    const number = (text: string) => {
      const el = Array.from(g.document.body.querySelectorAll("span")).find(
        (n) => n.textContent === text,
      ) as El;
      return {
        fill: g.getComputedStyle(el.parentElement as El).backgroundColor.toLowerCase(),
        ink: g.getComputedStyle(el).color.toLowerCase(),
      };
    };
    expect(number("3")).toEqual({ fill: "var(--decade1980)", ink: "var(--decadeink)" });
    expect(number("12")).toEqual({ fill: "rgba(0, 0, 0, 0)", ink: "var(--contentsecondary)" });
    await act(async () => root.unmount());
    host.remove();
  });
});

describe("GuidePhoneHeader", () => {
  it("heads the page with a title, a line under it and the view switch", () => {
    const html = renderToStaticMarkup(
      <LoomarrProvider>
        <GuidePhoneHeader subtitle="8:07 PM · 4 channels" title="What's on">
          <button type="button">Switch</button>
        </GuidePhoneHeader>
      </LoomarrProvider>,
    );
    expect(html).toContain(">What&#x27;s on<");
    expect(html).toContain(">8:07 PM · 4 channels<");
    expect(html).toContain(">Switch<");
  });
});
