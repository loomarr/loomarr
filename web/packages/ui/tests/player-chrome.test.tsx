// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  type PlaybarSegment,
  PlayerChrome,
  type PlayerChromeProps,
  PlayerCover,
  PlayerInfo,
  PlayerPlaybackInfo,
  PlayerPortrait,
  type PlayerPortraitProps,
  PlayerSettings,
  PlayerSheet,
  PlayerShortcuts,
  type PlayerTransportProps,
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

// Whether the lower third sits under an aria-hidden ancestor (the hidden scrim).
const bandHidden = (host: Host) =>
  Array.from(host.querySelectorAll('[aria-hidden="true"]')).some((el) =>
    el.textContent?.includes("CH 7 · Coast 90s"),
  );

const transport = (overrides: Partial<PlayerTransportProps> = {}): PlayerTransportProps => ({
  density: "pointer",
  live: { lagSeconds: 0, mode: "live" },
  muted: false,
  onGoLive: vi.fn(),
  onLeave: vi.fn(),
  onSettings: vi.fn(),
  onToggleMute: vi.fn(),
  onTogglePlay: vi.fn(),
  onVolume: vi.fn(),
  playing: true,
  pointer: true,
  settingsOpen: false,
  volume: 1,
  ...overrides,
});

const chrome = (overrides: Partial<PlayerChromeProps> = {}) => (
  <PlayerChrome
    band={{
      channelName: "Coast 90s",
      channelNumber: "7",
      programmeLine: "A sitcom · S2 E4",
      timeLabel: "9m in · 13m left",
    }}
    density="pointer"
    formatTime={(ms) => String(ms)}
    transport={transport()}
    visible
    {...overrides}
  />
);

describe("PlayerChrome", () => {
  it("carries the lower third and the transport, each control named", () => {
    const host = mount(chrome());

    expect(host.textContent).toContain("CH 7 · Coast 90s");
    expect(host.textContent).toContain("A sitcom · S2 E4");
    expect(host.textContent).toContain("9m in · 13m left");
    for (const name of ["Pause", "Mute", "Settings", "Leave Watch"])
      expect(button(host, name), name).toBeDefined();
    // Live: the state reads Live, with no Go live to offer.
    expect(host.textContent).toContain("Live");
    expect(button(host, "Go live")).toBeUndefined();
  });

  it("offers Go live when behind, reading how far", () => {
    const onGoLive = vi.fn();
    const host = mount(
      chrome({ transport: transport({ live: { lagSeconds: 134, mode: "paused" }, onGoLive }) }),
    );

    expect(host.textContent).toContain("Paused · 2m 14s behind");
    act(() => button(host, "Go live")?.click());
    expect(onGoLive).toHaveBeenCalledOnce();
  });

  it("steps through the window when the player can seek, with no forward from live", () => {
    const onSeekBack = vi.fn();
    const onSeekForward = vi.fn();
    const onStartOver = vi.fn();
    const atLive = mount(chrome({ transport: transport({ onSeekBack, onSeekForward, onStartOver }) }));
    act(() => button(atLive, "Back 10 seconds")?.click());
    act(() => button(atLive, "Forward 10 seconds")?.click());
    act(() => button(atLive, "Start over")?.click());
    expect(onSeekBack).toHaveBeenCalledOnce();
    expect(onSeekForward).not.toHaveBeenCalled();
    expect(button(atLive, "Forward 10 seconds")?.getAttribute("aria-disabled")).toBe("true");
    expect(onStartOver).toHaveBeenCalledOnce();

    const behind = mount(
      chrome({
        transport: transport({ live: { lagSeconds: 40, mode: "behind" }, onSeekBack, onSeekForward }),
      }),
    );
    act(() => button(behind, "Forward 10 seconds")?.click());
    expect(onSeekForward).toHaveBeenCalledOnce();
    expect(button(behind, "Start over")).toBeUndefined();
  });

  it("draws only the tools the platform has", () => {
    const bare = mount(chrome());
    expect(button(bare, "Channels")).toBeUndefined();
    expect(button(bare, "Fullscreen")).toBeUndefined();
    expect(button(bare, "Back 10 seconds")).toBeUndefined();

    const full = mount(
      chrome({ transport: transport({ fullscreen: false, onChannels: vi.fn(), onFullscreen: vi.fn() }) }),
    );
    expect(button(full, "Channels")).toBeDefined();
    expect(button(full, "Fullscreen")).toBeDefined();
  });

  it("stands the band alone as the station-ID card, with the station's era and place", () => {
    const host = mount(
      chrome({
        band: { channelName: "Coast 90s", channelNumber: "7", station: "1990s · Portland, ME" },
        ident: true,
        visible: false,
      }),
    );

    expect(bandHidden(host)).toBe(false);
    expect(host.textContent).toContain("CH 7 · Coast 90s · 1990s · Portland, ME");
    expect(button(host, "Pause")).toBeUndefined();

    // Shown controls take its place: the plain band and the transport.
    const shown = mount(
      chrome({ band: { channelName: "Coast 90s", channelNumber: "7", station: "1990s" }, ident: true }),
    );
    expect(shown.textContent).not.toContain("1990s");
    expect(button(shown, "Pause")).toBeDefined();
  });

  it("offers picture-in-picture where the browser has it, renamed while it is open", () => {
    const onPictureInPicture = vi.fn();
    expect(button(mount(chrome()), "Picture-in-picture")).toBeUndefined();
    const host = mount(chrome({ transport: transport({ onPictureInPicture, pictureInPicture: false }) }));
    act(() => button(host, "Picture-in-picture")?.click());
    expect(onPictureInPicture).toHaveBeenCalledOnce();

    const open = mount(chrome({ transport: transport({ onPictureInPicture, pictureInPicture: true }) }));
    expect(button(open, "Exit picture-in-picture")).toBeDefined();
  });

  it("offers Subtitles only when the programme carries a track, naming the next step while on", () => {
    const onSubtitles = vi.fn();
    expect(button(mount(chrome()), "Subtitles")).toBeUndefined();
    const off = mount(chrome({ transport: transport({ onSubtitles, subtitlesOn: false }) }));
    act(() => button(off, "Subtitles")?.click());
    expect(onSubtitles).toHaveBeenCalledOnce();

    const on = mount(chrome({ transport: transport({ onSubtitles, subtitlesOn: true }) }));
    expect(button(on, "Subtitles")).toBeUndefined();
    expect(button(on, "Next subtitles")).toBeDefined();
  });

  it("offers Info when there is a programme to describe", () => {
    const onInfo = vi.fn();
    expect(button(mount(chrome()), "Info")).toBeUndefined();
    const host = mount(chrome({ transport: transport({ onInfo }) }));
    act(() => button(host, "Info")?.click());
    expect(onInfo).toHaveBeenCalledOnce();
  });

  it("leaves the picture bare when hidden, out of reach of the pointer and assistive tech", () => {
    expect(bandHidden(mount(chrome()))).toBe(false);
    expect(bandHidden(mount(chrome({ visible: false })))).toBe(true);
  });

  it("puts an open sheet over a dimmed picture, standing the controls back", () => {
    const host = mount(
      chrome({
        sheet: (
          <div aria-label="Settings" role="dialog">
            sheet
          </div>
        ),
      }),
    );

    expect(host.querySelector('[role="dialog"]')?.textContent).toBe("sheet");
    expect(bandHidden(host)).toBe(true);
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

describe("PlayerPlaybackInfo", () => {
  const stream = [
    ["Quality", "Auto · 1080p (rung 1 of 2)"],
    ["Buffer", "12.4 s"],
  ] as const;

  it("reads a member the stream alone", () => {
    const host = mount(<PlayerPlaybackInfo density="pointer" stream={stream} />);
    expect(host.textContent).toContain("Auto · 1080p (rung 1 of 2)");
    expect(host.textContent).not.toContain("Admin");
    expect(button(host, "Copy stream URL")).toBeUndefined();
  });

  it("adds the server's side for an admin, with Copy stream URL", () => {
    const onCopyStreamUrl = vi.fn();
    const host = mount(
      <PlayerPlaybackInfo
        admin={{ copied: false, onCopyStreamUrl, rows: [["Session", "playback_1"]] }}
        density="pointer"
        stream={stream}
      />,
    );
    expect(host.textContent).toContain("Admin only");
    expect(host.textContent).toContain("playback_1");
    act(() => button(host, "Copy stream URL")?.click());
    expect(onCopyStreamUrl).toHaveBeenCalledOnce();
  });
});

describe("PlayerSettings", () => {
  it("ends with the way on to Playback info", () => {
    const onPlaybackInfo = vi.fn();
    const host = mount(
      <PlayerSettings
        audio={[]}
        density="pointer"
        onPlaybackInfo={onPlaybackInfo}
        onSwitchSound={vi.fn()}
        switchSound
      />,
    );
    act(() => button(host, "Playback info")?.click());
    expect(onPlaybackInfo).toHaveBeenCalledOnce();
    expect(
      button(
        mount(<PlayerSettings audio={[]} density="pointer" onSwitchSound={vi.fn()} switchSound />),
        "Playback info",
      ),
    ).toBeUndefined();
  });

  it("lists Off and each subtitle track as a choice, only when the programme carries any", () => {
    const onSelect = vi.fn();
    const host = mount(
      <PlayerSettings
        audio={[]}
        density="pointer"
        onSwitchSound={vi.fn()}
        subtitles={{
          onSelect,
          options: [
            { label: "Off", selected: false, value: "off" },
            { label: "English", selected: true, value: "0" },
          ],
        }}
        switchSound
      />,
    );
    const radios = Array.from(
      host.querySelectorAll('[role="radiogroup"][aria-label="Subtitles"] [role="radio"]'),
    );
    expect(radios.map((r) => [r.getAttribute("aria-label"), r.getAttribute("aria-checked")])).toEqual([
      ["Off", "false"],
      ["English", "true"],
    ]);
    act(() => radios[0]?.click());
    expect(onSelect).toHaveBeenCalledWith("off");

    const none = mount(<PlayerSettings audio={[]} density="pointer" onSwitchSound={vi.fn()} switchSound />);
    expect(none.querySelector('[aria-label="Subtitles"]')).toBeNull();
  });
});

describe("PlayerShortcuts", () => {
  it("lists each key beside what it does", () => {
    const host = mount(
      <PlayerShortcuts
        density="pointer"
        rows={[
          ["M", "Mute"],
          ["?", "This sheet"],
        ]}
      />,
    );

    const rows = Array.from(host.querySelectorAll('[role="listitem"]')).map((row) => row.textContent);
    expect(rows).toEqual(["MMute", "?This sheet"]);
  });
});

describe("PlayerPortrait", () => {
  const MIN = 60_000;
  // A programme on air since 0, a break still to come, then the next programme.
  const segments: PlaybarSegment[] = [
    { chips: [], id: "p1", kind: "programme", name: "Sitcom", startMs: 0, stopMs: 22 * MIN },
    {
      id: "b1",
      kind: "break",
      spots: [{ detail: "", durationLabel: "30s", durationMs: 30_000, id: "s1", name: "Station ID" }],
      startMs: 22 * MIN,
      stopMs: 25 * MIN,
    },
    { chips: [], id: "p2", kind: "programme", name: "Late Movie", startMs: 25 * MIN, stopMs: 120 * MIN },
  ];
  const portrait = (overrides: Partial<PlayerPortraitProps> = {}) => (
    <PlayerPortrait
      band={{ channelName: "Coast 90s", channelNumber: "7", programmeLine: "Sitcom · S2 E4" }}
      density="touch"
      formatTime={(ms) => `${ms / MIN}m`}
      transport={transport({ density: "touch", pointer: false })}
      {...overrides}
    />
  );

  it("stacks the band, the transport and surfing, without the overlay's volume or window tools", () => {
    const host = mount(
      portrait({
        surf: {
          canPrevious: true,
          canSurf: true,
          favourite: false,
          onChannelDown: vi.fn(),
          onChannelUp: vi.fn(),
          onPrevious: vi.fn(),
        },
      }),
    );

    expect(host.textContent).toContain("CH 7 · Coast 90s");
    expect(host.textContent).toContain("Sitcom · S2 E4");
    for (const name of ["Leave Watch", "Pause", "Channel up", "Channel down", "Previous", "Settings"])
      expect(button(host, name), name).toBeDefined();
    for (const name of ["Mute", "Fullscreen", "Picture-in-picture"])
      expect(button(host, name), name).toBeUndefined();
  });

  it("seeks a tile that has begun and the rewind tile, while what's to come only previews", () => {
    const onSeek = vi.fn();
    const host = mount(
      portrait({
        filmstrip: {
          onSeek,
          playheadMs: 10 * MIN,
          segments,
          window: { endMs: 120 * MIN, nowMs: 10 * MIN, startMs: -5 * MIN },
        },
      }),
    );

    expect(host.textContent).toContain("Coming up");
    expect(host.textContent).toContain("NOW");
    act(() => button(host, "Seek to NOW, 22m, Sitcom")?.click());
    act(() => button(host, "−15m, back to the start of the last 15 minutes")?.click());
    expect(onSeek.mock.calls).toEqual([[0], [-5 * MIN]]);
    // What's to come is named, but not as a button.
    for (const name of ["22m, Break · 3 min, 1 spots · Station ID", "25m, 95m, Late Movie"]) {
      expect(host.querySelector(`[aria-label="${name}"]`), name).not.toBeNull();
      expect(button(host, `Seek to ${name}`), name).toBeUndefined();
    }
  });

  it("names the star by what pressing it does", () => {
    const onFavourite = vi.fn();
    const host = mount(
      portrait({
        surf: {
          canPrevious: false,
          canSurf: true,
          favourite: true,
          onChannelDown: vi.fn(),
          onChannelUp: vi.fn(),
          onFavourite,
          onPrevious: vi.fn(),
        },
      }),
    );

    act(() => button(host, "Remove from Favorites")?.click());
    expect(onFavourite).toHaveBeenCalledOnce();
  });

  it("gives its place to an open sheet", () => {
    const host = mount(
      portrait({
        sheet: (
          <PlayerSheet
            channelLabel="CH 7 · Coast 90s"
            density="touch"
            icon="settings"
            inline
            onClose={vi.fn()}
            title="Settings"
          >
            {null}
          </PlayerSheet>
        ),
      }),
    );

    expect(host.querySelector('[role="dialog"]')?.getAttribute("aria-label")).toBe("Settings");
    expect(button(host, "Leave Watch")).toBeUndefined();
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
