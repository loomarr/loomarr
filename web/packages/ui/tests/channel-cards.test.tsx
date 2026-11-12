// @vitest-environment jsdom

import { layoutGuide } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  PlayerChrome,
  type PlayerChromeProps,
  type PlayerSurf,
  type PlayerSurfChannel,
  SURF_LINGER_MS,
  surfCardsFromGuide,
  useSurfCards,
} from "../index";
import { cardsAround } from "../src/watching-surface/player-chrome/channel-cards";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// The package compiles without the DOM lib (it is React Native first), so the test names the little
// of the DOM it touches.
type Node = { getAttribute(n: string): string | null; textContent: string | null };
type Host = Parameters<typeof createRoot>[0] & {
  querySelector(s: string): Node | null;
  querySelectorAll(s: string): ArrayLike<Node>;
  remove(): void;
  textContent: string | null;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Host };
  }
).document;

const mounted: { host: Host; unmount: () => void }[] = [];
const mount = (node: ReactNode) => {
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() => root.render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>));
  mounted.push({ host, unmount: () => act(() => root.unmount()) });
  return {
    host,
    rerender: (next: ReactNode) =>
      act(() => root.render(<LoomarrProvider theme="dark">{next}</LoomarrProvider>)),
  };
};
afterEach(() => {
  vi.useRealTimers();
  for (const { host, unmount } of mounted.splice(0)) {
    unmount();
    host.remove();
  }
});

const labelled = (host: Host, name: string) =>
  Array.from(host.querySelectorAll("[aria-label]")).find((el) => el.getAttribute("aria-label") === name);

// Seven invented channels, 3 through 21, each with something on.
const lineup: PlayerSurfChannel[] = [
  { id: "c3", name: "Saturday Mornings", nowPlaying: "Cartoon block", number: "3" },
  { id: "c5", name: "Late Show Reruns", nowPlaying: "A talk show", number: "5" },
  { id: "c7", name: "Coast 90s", nowPlaying: "An ensemble comedy", number: "7" },
  { id: "c9", name: "Movie Matinee", nowPlaying: "A heist film", number: "9" },
  { decade: 1990, id: "c12", name: "Game Night", nowPlaying: "A quiz show", number: "12" },
  { id: "c14", name: "Sunday Westerns", nowPlaying: "A cattle drive", number: "14" },
  { id: "c21", name: "Sports Replay", nowPlaying: "A baseball classic", number: "21" },
];

const chrome = (overrides: Partial<PlayerChromeProps> = {}) => (
  <PlayerChrome
    band={{ channelName: "Movie Matinee", channelNumber: "9", title: "A heist film" }}
    density="pointer"
    formatTime={(ms) => `t${ms}`}
    keys={{ density: "pointer", onChannelDown: vi.fn(), onChannelUp: vi.fn(), pointer: true }}
    live={{ lagSeconds: 0, mode: "live" }}
    onGoLive={vi.fn()}
    onLeave={vi.fn()}
    utilities={{ density: "pointer", onAudio: vi.fn(), pointer: true }}
    visible={false}
    {...overrides}
  />
);

describe("the channel cards while flipping (§9.2)", () => {
  it("take the band's place: five cards, the tuned one watching now, the rest saying what is on", () => {
    const { host } = mount(chrome({ surf: { channels: lineup, tunedId: "c9" } }));

    // The band is down: its title is not drawn, and the cards are.
    expect(host.querySelector('[data-testid="band-channel-block"]')).toBeNull();
    expect(labelled(host, "Channels")).toBeDefined();
    const cards = Array.from(host.querySelectorAll('[role="listitem"]')).map((el) => el.textContent);
    expect(cards).toEqual([
      "5Late Show RerunsA talk show",
      "7Coast 90sAn ensemble comedy",
      "9Movie MatineeWatching now",
      "12Game NightA quiz show",
      "14Sunday WesternsA cattle drive",
    ]);
    expect(host.querySelector('[data-testid="channel-card-lit"]')?.textContent).toContain("Movie Matinee");
  });

  it("bring the keys up with them and put the utilities and Leave away", () => {
    const { host } = mount(chrome({ surf: { channels: lineup, tunedId: "c9" } }));
    expect(labelled(host, "Channel up")).toBeDefined();
    expect(labelled(host, "Audio")).toBeUndefined();
    expect(labelled(host, "Leave")).toBeUndefined();
  });

  it("have a television's whole width: its keys step away too, since the remote is what flips", () => {
    const { host } = mount(
      chrome({
        density: "tv",
        keys: { density: "tv", onChannelUp: vi.fn(), pointer: false },
        layout: "tv",
        surf: { channels: lineup, tunedId: "c9" },
        utilities: { density: "tv", onAudio: vi.fn(), pointer: false },
      }),
    );
    expect(host.querySelectorAll('[role="listitem"]')).toHaveLength(5);
    expect(host.querySelector('[data-testid="player-cluster"]')).toBeNull();
    expect(labelled(host, "Audio")).toBeUndefined();
  });

  it("show three on a phone held upright, with the readout in the toolbar's place", () => {
    const { host } = mount(
      chrome({
        density: "touch",
        keys: { density: "touch", onChannelUp: vi.fn(), pointer: false },
        layout: "portrait",
        readout: { digits: "12", name: "Game Night" },
        surf: { channels: lineup, tunedId: "c12" },
        utilities: { density: "touch", onAudio: vi.fn(), pointer: false },
      }),
    );
    expect(host.querySelectorAll('[role="listitem"]')).toHaveLength(3);
    expect(host.querySelector('[data-testid="player-toolbar"]')).toBeNull();
    expect(host.querySelector('[role="status"]')?.textContent).toBe("12Game Night");
  });

  it("give way to an open sheet, and the band comes back without them", () => {
    const { host } = mount(
      chrome({ sheet: <span>Audio sheet</span>, surf: { channels: lineup, tunedId: "c9" } }),
    );
    expect(labelled(host, "Channels")).toBeUndefined();
    const plain = mount(chrome({ visible: true }));
    expect(labelled(plain.host, "Channels")).toBeUndefined();
    expect(plain.host.querySelector('[data-testid="band-channel-block"]')).not.toBeNull();
  });
});

describe("cardsAround", () => {
  const ids = (cards: readonly PlayerSurfChannel[]) => cards.map((card) => card.number);

  it("centres the tuned channel, and holds against the ends of the lineup rather than wrapping", () => {
    expect(ids(cardsAround(lineup, "c9", 5))).toEqual(["5", "7", "9", "12", "14"]);
    expect(ids(cardsAround(lineup, "c3", 5))).toEqual(["3", "5", "7", "9", "12"]);
    expect(ids(cardsAround(lineup, "c21", 3))).toEqual(["12", "14", "21"]);
  });

  it("shows every channel when there are fewer than the row holds", () => {
    expect(ids(cardsAround(lineup.slice(0, 2), "c5", 5))).toEqual(["3", "5"]);
  });
});

describe("useSurfCards", () => {
  const Probe = ({
    flipKey,
    onSurf,
  }: {
    flipKey: string | undefined;
    onSurf: (s: PlayerSurf | undefined) => void;
  }) => {
    onSurf(useSurfCards(lineup, "c9", flipKey));
    return null;
  };

  it("is up from a flip until a beat after the last one, and a flip before it mounted is not one", () => {
    vi.useFakeTimers();
    let surf: PlayerSurf | undefined;
    const seen = (s: PlayerSurf | undefined) => {
      surf = s;
    };
    const { rerender } = mount(<Probe flipKey="c7" onSurf={seen} />);
    expect(surf).toBeUndefined();

    rerender(<Probe flipKey="c9" onSurf={seen} />);
    expect(surf).toEqual({ channels: lineup, tunedId: "c9" });
    act(() => vi.advanceTimersByTime(SURF_LINGER_MS - 100));
    // A second flip restarts the beat.
    rerender(<Probe flipKey="c12" onSurf={seen} />);
    act(() => vi.advanceTimersByTime(SURF_LINGER_MS - 1));
    expect(surf).toBeDefined();
    act(() => vi.advanceTimersByTime(1));
    expect(surf).toBeUndefined();
  });

  it("goes at once when a tune that is not a flip follows", () => {
    vi.useFakeTimers();
    let surf: PlayerSurf | undefined;
    const seen = (s: PlayerSurf | undefined) => {
      surf = s;
    };
    const { rerender } = mount(<Probe flipKey={undefined} onSurf={seen} />);
    rerender(<Probe flipKey="c12" onSurf={seen} />);
    expect(surf).toBeDefined();
    rerender(<Probe flipKey={undefined} onSurf={seen} />);
    expect(surf).toBeUndefined();
  });
});

describe("surfCardsFromGuide", () => {
  const layout = layoutGuide(
    {
      channels: [
        {
          airings: [
            {
              kind: "program",
              scheduleBlockId: "now",
              startMs: 1_000,
              stopMs: 3_000,
              thumbUrl: "/v1/images/now.jpg",
              title: "A heist film",
            },
          ],
          channelId: "c9",
          decade: 1980,
          name: "Movie Matinee",
          number: 9,
          pendingCount: 0,
          status: "live",
        },
      ],
      fromMs: 1_000,
      timezone: "America/New_York",
      toMs: 5_000,
    },
    2_000,
  );

  it("keeps the catalog's order and names, and takes what is on, its picture and the decade from the guide", () => {
    const cards = surfCardsFromGuide(
      [
        { id: "c9", name: "Movie Matinee", number: 9 },
        { id: "c12", name: "Game Night", number: 12 },
      ],
      layout,
      2_000,
    );
    expect(cards).toEqual([
      {
        artUri: "/v1/images/now.jpg",
        decade: 1980,
        id: "c9",
        name: "Movie Matinee",
        nowPlaying: "A heist film",
        number: "9",
      },
      // A channel the guide has not read yet is still a card: its number and name.
      { id: "c12", name: "Game Night", number: "12" },
    ]);
  });
});
