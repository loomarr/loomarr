// @vitest-environment jsdom

import {
  createPlayerController,
  type PlayerChannel,
  type PlayerController,
  type PlayerTransport,
  type PlayerTransportEvent,
} from "@loomarr/player";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

vi.mock("expo-crypto", () => ({ randomUUID: vi.fn() }));
vi.mock("expo-video", () => ({ createVideoPlayer: vi.fn(), VideoView: vi.fn() }));

const { createShellPause, useShellPause } = await import("@loomarr/player/native");

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const channels: PlayerChannel[] = [
  { id: "a", inAppPlayable: true, name: "A", number: 1 },
  { id: "b", inAppPlayable: true, name: "B", number: 2 },
];

/** A real controller over a transport that logs its calls in order, with nothing tuned yet. */
const idleController = () => {
  const listeners = new Set<(event: PlayerTransportEvent) => void>();
  const calls: string[] = [];
  const transport: PlayerTransport = {
    dispose: vi.fn(),
    goLive: vi.fn(),
    pause: vi.fn(() => void calls.push("pause")),
    play: vi.fn(() => void calls.push("play")),
    replace: vi.fn(async (source) => void calls.push(`replace:${source.uri}`)),
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
  const controller = createPlayerController({
    profile: {},
    source: { mint: async (channel) => ({ uri: `${channel.id}.m3u8` }) },
    transport,
  });
  return { calls, controller, listeners, transport };
};

/** The same, already playing channel A. */
const playingOnA = async () => {
  const idle = idleController();
  await idle.controller.reconcile(channels);
  await act(async () => {
    const attemptId = idle.controller.getSnapshot().attemptId as number;
    for (const listener of idle.listeners) listener({ attemptId, type: "first-frame" });
  });
  expect(idle.controller.getSnapshot()).toMatchObject({ channel: { id: "a" }, status: "playing" });
  // Tuning drives the transport itself; only what the shell does from here on is under test.
  idle.calls.length = 0;
  vi.mocked(idle.transport.play).mockClear();
  vi.mocked(idle.transport.pause).mockClear();
  return idle;
};

/** Renders the hook the way the phone shell does, so a test can move between Guide and Watching. */
const mountShell = async (initial: PlayerController) => {
  let forget!: (targetChannelId: string) => void;
  const Probe = ({ controller, watching }: { controller: PlayerController; watching: boolean }) => {
    forget = useShellPause(controller, watching);
    return null;
  };
  const container = (
    globalThis as unknown as { document: { createElement: (tag: string) => never } }
  ).document.createElement("div");
  const root = createRoot(container);
  const render = (controller: PlayerController, watching: boolean) =>
    act(async () => root.render(createElement(Probe, { controller, watching })));
  await render(initial, true);
  return {
    forget: (targetChannelId: string) => forget(targetChannelId),
    render,
    unmount: () => act(async () => root.unmount()),
  };
};

describe("shell pause", () => {
  it("pauses the picture on leaving Watching and resumes the same channel on return", async () => {
    const { controller, transport } = await playingOnA();
    const shell = await mountShell(controller);
    expect(transport.pause).not.toHaveBeenCalled();

    await shell.render(controller, false);
    expect(transport.pause).toHaveBeenCalledTimes(1);
    expect(controller.getSnapshot()).toMatchObject({ channel: { id: "a" }, status: "paused" });
    expect(transport.play).not.toHaveBeenCalled();

    await shell.render(controller, true);
    expect(transport.play).toHaveBeenCalledTimes(1);
    await shell.unmount();
  });

  it("never resumes the paused channel when the viewer tunes another from the Guide", async () => {
    const { calls, controller } = await playingOnA();
    const shell = await mountShell(controller);
    await shell.render(controller, false);

    // The Guide's onTune: forget the pause, tune, then show Watching.
    shell.forget("b");
    await act(async () => controller.tuneChannel("b"));
    await shell.render(controller, true);

    // The only play is the tune's own, after B's source replaced A's; none resumes A first.
    expect(calls).toEqual(["pause", "replace:b.m3u8", "play"]);
    expect(controller.getSnapshot()).toMatchObject({ channel: { id: "b" } });
    await shell.unmount();
  });

  it("forgetting for another channel leaves nothing to resume, even if the snapshot still reads A paused", () => {
    // A controller whose snapshot has not yet moved to B: only forget stops A from being resumed.
    const play = vi.fn();
    let status: "paused" | "playing" = "playing";
    const shellPause = createShellPause({
      getSnapshot: () => ({ channel: channels[0], status }) as ReturnType<PlayerController["getSnapshot"]>,
      pause: () => {
        status = "paused";
      },
      play,
    });
    shellPause.leave();
    shellPause.forget("b");
    shellPause.enter();
    expect(play).not.toHaveBeenCalled();
  });

  it("resumes the paused channel when the Guide tunes it again", async () => {
    const { calls, controller, transport } = await playingOnA();
    const shell = await mountShell(controller);
    await shell.render(controller, false);

    // Re-tuning A is a no-op for the controller, so Watching's return is the only thing that resumes it.
    shell.forget("a");
    await act(async () => controller.tuneChannel("a"));
    await shell.render(controller, true);

    expect(calls).toEqual(["pause", "play"]);
    expect(transport.play).toHaveBeenCalledTimes(1);
    await shell.unmount();
  });

  it("resumes only a stream still paused on the channel it paused, even without forget", async () => {
    const { calls, controller } = await playingOnA();
    const shell = await mountShell(controller);
    await shell.render(controller, false);

    await act(async () => controller.tuneChannel("b"));
    await shell.render(controller, true);

    expect(calls).toEqual(["pause", "replace:b.m3u8", "play"]);
    await shell.unmount();
  });

  it("does not resume a rebuilt controller from the old controller's pause", async () => {
    const first = await playingOnA();
    const rebuilt = idleController();
    const shell = await mountShell(first.controller);
    await shell.render(first.controller, false);

    // The runtime rebuilds the controller while the viewer is still on the Guide.
    await shell.render(rebuilt.controller, false);
    await shell.render(rebuilt.controller, true);

    expect(first.calls).toEqual(["pause"]);
    expect(rebuilt.calls).toEqual([]);
    await shell.unmount();
  });

  it("keeps the resume when the picture is left twice before returning", async () => {
    const { controller, transport } = await playingOnA();
    const shellPause = createShellPause(controller);

    shellPause.leave();
    shellPause.leave();
    shellPause.enter();

    expect(transport.play).toHaveBeenCalledTimes(1);
    shellPause.enter();
    expect(transport.play).toHaveBeenCalledTimes(1);
  });

  it("leaves a stream the viewer paused themselves alone", async () => {
    const { controller, transport } = await playingOnA();
    controller.pause();
    const shellPause = createShellPause(controller);

    shellPause.leave();
    shellPause.enter();

    expect(transport.play).not.toHaveBeenCalled();
  });
});
