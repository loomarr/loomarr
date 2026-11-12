// @vitest-environment jsdom

import { LoomarrProvider, playerChrome, semanticColors } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  PlayerChrome,
  type PlayerChromeProps,
  PlayerCover,
  PlayerInfo,
  type PlayerKeysProps,
  PlayerMore,
  PlayerOptions,
  PlayerSheet,
  type PlayerUtilitiesProps,
  PlayerVolume,
} from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// The package compiles without the DOM lib (it is React Native first), so the test names the little
// of the DOM it touches.
type Node = { click(): void; getAttribute(n: string): string | null; textContent: string | null };
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
  return host;
};
afterEach(() => {
  for (const { host, unmount } of mounted.splice(0)) {
    unmount();
    host.remove();
  }
});

const button = (host: Host, name: string) =>
  Array.from(host.querySelectorAll('[role="button"]')).find(
    (el) => el.getAttribute("aria-label") === name || el.textContent === name,
  );

// Whether the band sits under a transparent ancestor (the hidden scrim). On the web the hidden layer
// is not aria-hidden: its keys stay focusable, and focusing one brings the controls back.
const bandHidden = (host: Host) =>
  Array.from(host.querySelectorAll("[style]")).some(
    (el) => /opacity:\s*0(;|$)/.test(el.getAttribute("style") ?? "") && el.textContent?.includes("Coast 90s"),
  );

// A More row with a detail is named by what it shows, so it is found by its leading words.
const row = (host: Host, name: string) =>
  Array.from(host.querySelectorAll('[role="button"]')).find(
    (el) => el.getAttribute("aria-label") === name || el.textContent?.startsWith(name),
  );

const keys = (overrides: Partial<PlayerKeysProps> = {}): PlayerKeysProps => ({
  density: "pointer",
  moreOpen: false,
  onChannelDown: vi.fn(),
  onChannelUp: vi.fn(),
  onMore: vi.fn(),
  onTogglePlay: vi.fn(),
  playing: true,
  pointer: true,
  volume: { muted: false, onPress: vi.fn(), open: false },
  ...overrides,
});

const utilities = (overrides: Partial<PlayerUtilitiesProps> = {}): PlayerUtilitiesProps => ({
  density: "pointer",
  pointer: true,
  ...overrides,
});

const band = {
  channelName: "Coast 90s",
  channelNumber: "7",
  clockMs: 1,
  line: "S2 E4 “Opening Night”",
  next: { name: "A detective film", startMs: 4 },
  progress: 0.4,
  startMs: 0,
  stopMs: 10,
  title: "An ensemble comedy",
};

const chrome = (overrides: Partial<PlayerChromeProps> = {}) => (
  <PlayerChrome
    band={band}
    density="pointer"
    formatTime={(ms) => `t${ms}`}
    keys={keys()}
    live={{ lagSeconds: 0, mode: "live" }}
    onGoLive={vi.fn()}
    onLeave={vi.fn()}
    utilities={utilities()}
    visible
    {...overrides}
  />
);

// A schedule for the film strip: the programme on air, a break, and the next programme.
const schedule = {
  playheadMs: 30 * 60_000,
  segments: [
    {
      chips: ["S2 E4", "TV-PG"],
      id: "a",
      kind: "programme" as const,
      name: "An ensemble comedy",
      startMs: 0,
      stopMs: 40 * 60_000,
    },
    { id: "b", kind: "break" as const, spots: [], startMs: 40 * 60_000, stopMs: 43 * 60_000 },
    {
      chips: [],
      id: "c",
      kind: "programme" as const,
      name: "A detective film",
      startMs: 43 * 60_000,
      stopMs: 150 * 60_000,
    },
  ],
  window: { endMs: 150 * 60_000, nowMs: 30 * 60_000, startMs: 15 * 60_000 },
};

describe("PlayerChrome", () => {
  it("names the channel once, and carries Now, the progress clocks and Next", () => {
    const host = mount(chrome());

    // The band names the channel once; the block beside it carries only the number.
    expect(host.textContent?.match(/Coast 90s/g)).toHaveLength(1);
    for (const text of ["Now", "An ensemble comedy", "S2 E4 “Opening Night”", "t0", "t10", "Next", "t4"])
      expect(host.textContent, text).toContain(text);
    // Now and Next are plain labels in a gutter, not shouted chips (§9.6).
    expect(host.textContent).not.toContain("NOW");
    expect(host.textContent).not.toContain("NEXT");
    expect(host.textContent).toContain("A detective film");
    // Live reads Live with the clock; there is no Go live to offer.
    expect(host.textContent).toContain("Live");
    expect(host.textContent).toContain("t1");
    expect(button(host, "Go live")).toBeUndefined();
  });

  it("draws the keys, the utilities over them and Leave, each named", () => {
    const host = mount(
      chrome({ utilities: utilities({ fullscreen: false, onAudio: vi.fn(), onFullscreen: vi.fn() }) }),
    );

    for (const name of [
      "Channel up",
      "Channel down",
      "Pause",
      "Volume",
      "More",
      "Audio",
      "Fullscreen",
      "Leave",
    ])
      expect(button(host, name), name).toBeDefined();
    // No channel to return to yet: Last is not drawn. Subtitles are not drawn until the player has any.
    expect(host.textContent).not.toContain("Last");
    expect(button(host, "Subtitles")).toBeUndefined();
    // The utilities sit in the control cluster with the keys, not in a row of their own up top.
    expect(
      Array.from(host.querySelectorAll('[data-testid="player-cluster"] [role="button"]')).map((el) =>
        el.getAttribute("aria-label"),
      ),
    ).toEqual(expect.arrayContaining(["Audio", "Fullscreen", "Pause", "More"]));
  });

  it("sets the programme's rating beside Leave, and leaves", () => {
    const onLeave = vi.fn();
    const host = mount(chrome({ band: { ...band, rating: "TV-PG" }, onLeave }));
    expect(host.querySelector('[data-testid="player-rating"]')?.textContent).toBe("TV-PG");
    act(() => button(host, "Leave")?.click());
    expect(onLeave).toHaveBeenCalledOnce();
  });

  it("runs the film strip as the band's foot on a pointer device, in place of the progress line and Next", () => {
    const host = mount(chrome({ band: { ...band, clockMs: 30 * 60_000, stopMs: 40 * 60_000 }, schedule }));
    expect(host.querySelector('[role="group"][aria-label="Schedule"]')).not.toBeNull();
    expect(host.querySelector('[data-testid="band-progress"]')).toBeNull();
    expect(host.textContent).not.toContain("Next");
    expect(host.textContent).toContain("10 min left");
    // jsdom lays nothing out, so the strip draws no blocks here; the white playhead is always drawn.
    expect(host.querySelector('[data-testid="strip-playhead"]')).not.toBeNull();
  });

  it("says Nothing scheduled, with no Now or Next, when the channel has nothing on", () => {
    const host = mount(chrome({ band: { channelName: "Coast 90s", channelNumber: "7", clockMs: 1 } }));
    expect(host.textContent).toContain("Nothing scheduled");
    expect(host.textContent).not.toContain("Now");
    expect(host.textContent).not.toContain("Next");
  });

  it("gives a television four keys, no utilities row and no Leave: Back leaves", () => {
    const host = mount(
      chrome({
        density: "tv",
        keys: keys({ density: "tv", pointer: false }),
        layout: "tv",
        utilities: utilities({ density: "tv", onAudio: vi.fn(), onSubtitles: vi.fn(), pointer: false }),
      }),
    );
    for (const name of ["Subtitles", "Audio", "More"]) expect(button(host, name), name).toBeDefined();
    for (const name of ["Leave", "Channel up", "Pause", "Mute", "Fullscreen"])
      expect(button(host, name), name).toBeUndefined();
  });

  it("offers Last with the channel's number once there is one to return to", () => {
    const onPress = vi.fn();
    const host = mount(chrome({ keys: keys({ last: { number: "3", onPress } }) }));
    act(() => button(host, "Last channel, 3")?.click());
    expect(onPress).toHaveBeenCalledOnce();
  });

  it("flips channels, plays and opens the volume level from the keys", () => {
    const k = keys();
    const host = mount(chrome({ keys: k }));
    act(() => button(host, "Channel up")?.click());
    act(() => button(host, "Channel down")?.click());
    act(() => button(host, "Pause")?.click());
    act(() => button(host, "Volume")?.click());
    expect(k.onChannelUp).toHaveBeenCalledOnce();
    expect(k.onChannelDown).toHaveBeenCalledOnce();
    expect(k.onTogglePlay).toHaveBeenCalledOnce();
    expect(k.volume?.onPress).toHaveBeenCalledOnce();
  });

  it("draws the volume level in the menu's place: Off when muted", () => {
    const host = mount(
      chrome({
        keys: keys({ volume: { muted: true, onPress: vi.fn(), open: true } }),
        menu: <PlayerVolume muted onVolume={vi.fn()} volume={0.4} />,
      }),
    );
    // What a key opens says so to a screen reader; a key that opens nothing says nothing about it.
    expect(button(host, "Volume")?.getAttribute("aria-expanded")).toBe("true");
    expect(button(host, "More")?.getAttribute("aria-expanded")).toBe("false");
    expect(button(host, "Pause")?.getAttribute("aria-expanded")).toBeNull();
    expect(host.querySelector('[data-testid="player-volume"]')?.textContent).toContain("Off");
    expect(
      host.querySelector('[data-testid="player-volume"] [role="slider"]')?.getAttribute("aria-label"),
    ).toBe("Volume");
  });

  it("leaves out the rocker where the player cannot tune, and Volume where the device has its own", () => {
    const host = mount(
      chrome({
        density: "touch",
        keys: keys({
          density: "touch",
          onChannelDown: undefined,
          onChannelUp: undefined,
          pointer: false,
          volume: undefined,
        }),
      }),
    );
    expect(button(host, "Channel up")).toBeUndefined();
    expect(button(host, "Mute")).toBeUndefined();
    expect(button(host, "Pause")).toBeDefined();
  });

  it("replaces Live with Go live when behind, which goes live", () => {
    const onGoLive = vi.fn();
    const host = mount(
      chrome({
        band: { ...band, behindLabel: "2m 14s behind", liveProgress: 0.9 },
        live: { lagSeconds: 134, mode: "paused" },
        onGoLive,
      }),
    );

    expect(host.textContent).not.toContain("Live");
    expect(host.textContent).toContain("2m 14s behind");
    act(() => button(host, "Go live")?.click());
    expect(onGoLive).toHaveBeenCalledOnce();
  });

  it("always types the title: a title's logo never stands in for it in the band (§9.2)", () => {
    const logo = { height: 100, src: "data:image/png;base64,AAAA", srcSetWebp: "", width: 600 };
    const host = mount(chrome({ band: { ...band, logo } }));
    expect(host.querySelector('[role="img"]')).toBeNull();
    expect(host.querySelector('[data-testid="band-title"]')?.textContent).toBe("An ensemble comedy");
  });

  it("reads a break as the programme's title with Back in n min, and no badge", () => {
    const host = mount(chrome({ band: { ...band, breakMinutesLeft: 3, inBreak: true, line: undefined } }));
    expect(host.textContent).toContain("An ensemble comedy");
    expect(host.textContent).toContain("Back in 3 min");
    expect(host.textContent).not.toContain("S2 E4");
    expect(host.textContent).not.toContain("Ad break");
  });

  it("stands the band alone as the station-ID card, with the station's era and place", () => {
    const host = mount(
      chrome({
        band: {
          channelName: "Coast 90s",
          channelNumber: "7",
          station: "1990s · Portland, ME",
          title: "A film",
        },
        ident: true,
        visible: false,
      }),
    );

    expect(bandHidden(host)).toBe(false);
    expect(host.textContent).toContain("Coast 90s · 1990s · Portland, ME");
    expect(button(host, "Pause")).toBeUndefined();

    // Shown controls take its place: the plain band and the keys.
    const shown = mount(
      chrome({ band: { channelName: "Coast 90s", channelNumber: "7", station: "1990s" }, ident: true }),
    );
    expect(shown.textContent).not.toContain("1990s");
    expect(button(shown, "Pause")).toBeDefined();
  });

  it("fills the channel block with the channel's decade colour, amber without one", () => {
    const g = globalThis as unknown as {
      getComputedStyle(n: Node): { backgroundColor: string; color: string };
    };
    const block = (host: Host) => {
      const number = Array.from(host.querySelectorAll("div")).find((el) => el.textContent === "CH7") as Node;
      return g.getComputedStyle(number).backgroundColor;
    };
    const rgb = (hex: string) =>
      `rgb(${[1, 3, 5].map((i) => Number.parseInt(hex.slice(i, i + 2), 16)).join(", ")})`;

    const era = mount(chrome({ band: { ...band, decade: 1990 } }));
    expect(block(era)).toBe(rgb(semanticColors.decade["1990"]));
    const plain = mount(chrome());
    expect(block(plain)).toBe(rgb(playerChrome.signal));
  });

  it("flips to the next channel quietly: the band stays up and names it, the sweep and dip add nothing to the tree", () => {
    const host = doc.createElement("div");
    doc.body.append(host);
    const root = createRoot(host);
    const draw = (channelNumber: string, channelName: string) =>
      act(() =>
        root.render(
          <LoomarrProvider theme="dark">
            {chrome({ band: { ...band, channelName, channelNumber } })}
          </LoomarrProvider>,
        ),
      );
    draw("7", "Coast 90s");
    draw("9", "Movie Matinee");
    expect(host.textContent).toContain("Movie Matinee");
    expect(host.textContent).toContain("CH9");
    expect(bandHidden(host)).toBe(false);
    act(() => root.unmount());
    host.remove();
  });

  it("leaves the picture bare when hidden, with its keys still reachable by focus", () => {
    expect(bandHidden(mount(chrome()))).toBe(false);
    const hidden = mount(chrome({ visible: false }));
    expect(bandHidden(hidden)).toBe(true);
    // Not aria-hidden over focusable keys (axe aria-hidden-focus): Tab reaches a key, which reveals.
    expect(hidden.querySelector('[aria-hidden="true"] [role="button"]')).toBeNull();
    expect(button(hidden, "Pause")).toBeDefined();
  });

  it("keeps the keys with a sheet open, over a dimmed picture", () => {
    const host = mount(
      chrome({
        sheet: (
          <div aria-label="Audio" role="dialog">
            sheet
          </div>
        ),
      }),
    );

    expect(host.querySelector('[role="dialog"]')?.textContent).toBe("sheet");
    expect(bandHidden(host)).toBe(false);
    expect(button(host, "Pause")).toBeDefined();
  });
});

describe("PlayerSheet", () => {
  it("wears the band's identity: the channel's number and name over its title, with a close", () => {
    const onClose = vi.fn();
    const host = mount(
      <PlayerSheet
        channel={{ name: "Coast 90s", number: "7" }}
        density="pointer"
        onClose={onClose}
        title="Audio"
      >
        body
      </PlayerSheet>,
    );

    expect(host.textContent).toContain("CH7");
    expect(host.textContent).toContain("Coast 90s");
    expect(host.textContent).toContain("Audio");
    act(() => button(host, "Close Audio")?.click());
    expect(onClose).toHaveBeenCalledOnce();
  });
});

describe("PlayerOptions", () => {
  it("marks the chosen option and reports a choice", () => {
    const onSelect = vi.fn();
    const host = mount(
      <PlayerOptions
        density="pointer"
        label="Picture quality"
        onSelect={onSelect}
        options={[
          { detail: "Best for your connection", label: "Auto · 1080p", selected: true, value: "auto" },
          { label: "720p", selected: false, value: "720" },
        ]}
      />,
    );

    const radios = Array.from(host.querySelectorAll('[role="radio"]'));
    expect(radios.map((r) => r.getAttribute("aria-checked"))).toEqual(["true", "false"]);
    expect(host.textContent).toContain("Best for your connection");
    act(() => radios[1]?.click());
    expect(onSelect).toHaveBeenCalledWith("720");
  });

  it("is a plain list when the viewer cannot choose", () => {
    const host = mount(
      <PlayerOptions
        density="pointer"
        label="Audio language"
        options={[{ label: "English", selected: true, value: "eng" }]}
      />,
    );
    expect(host.querySelector('[role="radio"]')).toBeNull();
    expect(host.querySelector('[role="listitem"]')?.textContent).toBe("English");
  });
});

describe("PlayerMore", () => {
  it("lists only the rows it has somewhere to go", () => {
    const onStartOver = vi.fn();
    const bare = mount(<PlayerMore density="pointer" formatTime={String} />);
    expect(row(bare, "Start over")).toBeUndefined();
    expect(row(bare, "Channels")).toBeUndefined();

    const host = mount(
      <PlayerMore
        channels={{ onPress: vi.fn() }}
        density="pointer"
        formatTime={String}
        goLive={{ detail: "8:44 PM", onPress: vi.fn() }}
        info={{ onPress: vi.fn() }}
        picture={{ detail: "Auto · 1080p", onPress: vi.fn() }}
        startOver={{ detail: "Opening Night", onPress: onStartOver }}
      />,
    );
    // Last channel is a key on every device, so More has no row for it (§9.7).
    expect(row(host, "Last channel")).toBeUndefined();
    for (const name of ["Start over", "Go live", "Picture", "Channels", "Info"])
      expect(row(host, name), name).toBeDefined();
    // A detail joins the name; a row without one keeps its label.
    expect(row(host, "Start over")?.getAttribute("aria-label")).toBeNull();
    expect(row(host, "Info")?.getAttribute("aria-label")).toBe("Info");
    act(() => row(host, "Start over")?.click());
    expect(onStartOver).toHaveBeenCalledOnce();
  });
});

describe("PlayerInfo", () => {
  it("reads the programme, its time and what is up next", () => {
    const host = mount(
      <PlayerInfo
        density="pointer"
        formatTime={(ms) => `t${ms}`}
        info={{
          facts: "1994 · TV-PG · Mystery",
          progress: 0.4,
          startMs: 1,
          stopMs: 2,
          synopsis: "A storm cuts the island off.",
          timeLabel: "9m in · 13m left",
          title: "Coastline Mysteries · S2 E4",
          upNext: { line: "S1 E1 “Pilot”", name: "Night Shift Diner", startMs: 3 },
        }}
      />,
    );

    for (const text of [
      "Coastline Mysteries · S2 E4",
      "1994 · TV-PG · Mystery",
      "A storm cuts the island off.",
      "t1",
      "9m in · 13m left",
      "t2",
      "Up next · t3 · Night Shift Diner",
      "S1 E1 “Pilot”",
    ])
      expect(host.textContent).toContain(text);
  });
});

describe("PlayerCover", () => {
  it("is an alert for a stream that stopped, with Retry and Leave", () => {
    const onRetry = vi.fn();
    const onLeave = vi.fn();
    const host = mount(
      <PlayerCover
        channelLabel="CH 7 · Coast 90s"
        density="pointer"
        kind="error"
        onLeave={onLeave}
        onRetry={onRetry}
        title="The stream stopped."
      />,
    );

    expect(host.querySelector('[role="alert"]')?.textContent).toContain("The stream stopped.");
    act(() => button(host, "Retry channel")?.click());
    act(() => button(host, "Leave")?.click());
    expect(onRetry).toHaveBeenCalledOnce();
    expect(onLeave).toHaveBeenCalledOnce();
  });

  it("is a status for an off-air channel, with only the way out", () => {
    const host = mount(
      <PlayerCover
        channelLabel="CH 7 · Coast 90s"
        density="pointer"
        kind="off-air"
        onLeave={vi.fn()}
        title="Off air"
      />,
    );

    expect(host.querySelector('[role="status"]')?.textContent).toContain("Off air");
    expect(button(host, "Retry channel")).toBeUndefined();
    expect(button(host, "Leave")).toBeDefined();
  });
});

describe("player chrome tokens", () => {
  it("are flat: no glass, shadow, blur or glow token exists (#2769)", () => {
    expect(Object.keys(playerChrome).filter((key) => /glass|shadow|blur|glow|highlight/i.test(key))).toEqual(
      [],
    );
  });
});
