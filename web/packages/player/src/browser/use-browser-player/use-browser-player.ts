import type { ClientObservation } from "@loomarr/core/client-diagnostics";
import { useCallback, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import {
  createPlayerController,
  type PlayerController,
  type PlayerSnapshot,
  type PlayerTuneReport,
} from "../../player-controller";
import type { PlayerChannel, PlayerSourcePort } from "../../player-source";
import { type BrowserTransport, type BrowserTunePhase, createBrowserTransport } from "../browser-transport";

// useBrowserPlayer — the browser's player: the shared player controller running over the browser
// transport, bound to the page's <video>. The controller owns tuning, surfing, warming and recovery;
// the transport owns hls.js and the live clock. This hook only owns their lifetime in React.

interface BrowserPlayerOptions {
  /** Every channel the server lists; the controller keeps the playable ones, in number order. */
  channels: readonly PlayerChannel[];
  /** A tune attempt started, once it is published. */
  onTune?: (report: PlayerTuneReport) => void;
  /** An attempt reached a phase of its tune, for timing. */
  onPhase?: (attemptId: number, phase: BrowserTunePhase) => void;
  recordDiagnostic: (observation: ClientObservation) => void;
  source: PlayerSourcePort;
}

/** What a surface can ask of the player. */
type BrowserPlayerControls = Pick<
  PlayerController,
  "goLive" | "pause" | "play" | "previous" | "retry" | "seek" | "step" | "tuneChannel" | "tuneNumber"
>;

interface BrowserPlayer {
  /**
   * Binds the player to a <video>. Pass to the video surface's `attach`; the returned cleanup
   * unbinds it. Stable, so the surface binds once per element.
   */
  attach: (video: HTMLVideoElement) => () => void;
  controls: BrowserPlayerControls;
  /** Names this player's session in client diagnostics; empty until the player has started. */
  playbackSessionId: string;
  snapshot: PlayerSnapshot;
}

interface Runtime {
  controller: PlayerController;
  transport: BrowserTransport;
}

const EMPTY_SNAPSHOT: PlayerSnapshot = { catalog: [], recentChannelIds: [], status: "empty" };
const noUnsubscribe = () => undefined;

const useBrowserPlayer = ({
  channels,
  onPhase,
  onTune,
  recordDiagnostic,
  source,
}: BrowserPlayerOptions): BrowserPlayer => {
  // Callbacks are read when they fire, so a new function never rebuilds the player.
  const callbacks = useRef({ onPhase, onTune, recordDiagnostic });
  callbacks.current = { onPhase, onTune, recordDiagnostic };
  const [runtime, setRuntime] = useState<Runtime>();
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);

  useLayoutEffect(() => {
    // The transport reports the channel of the attempt it is opening from the controller's snapshot.
    let controller: PlayerController | undefined;
    const transport = createBrowserTransport({
      channelId: () => controller?.getSnapshot().channel?.id,
      onPhase: (attemptId, phase) => callbacks.current.onPhase?.(attemptId, phase),
      recordDiagnostic: (observation) => callbacks.current.recordDiagnostic(observation),
    });
    const created = createPlayerController({
      // The viewer's surface names the channel to tune; the controller starts idle until it does.
      initialTune: "none",
      onTune: (report) => callbacks.current.onTune?.(report),
      source,
      transport,
    });
    controller = created;
    setRuntime({ controller: created, transport });
    return () => {
      created.dispose();
      setRuntime(undefined);
    };
  }, [source]);

  useLayoutEffect(() => {
    if (!runtime || !video) return;
    return runtime.transport.attach(video);
  }, [runtime, video]);

  // Reconcile when the playable set changes, not on every new array of the same channels.
  const channelsRef = useRef(channels);
  channelsRef.current = channels;
  const catalogKey = channels
    .map((channel) => `${channel.id}:${channel.number}:${channel.inAppPlayable}`)
    .join("|");
  // biome-ignore lint/correctness/useExhaustiveDependencies: the key is the channels' content
  useLayoutEffect(() => {
    void runtime?.controller.reconcile(channelsRef.current);
  }, [runtime, catalogKey]);

  const subscribe = useCallback(
    (listener: () => void) => runtime?.controller.subscribe(listener) ?? noUnsubscribe,
    [runtime],
  );
  const getSnapshot = useCallback(() => runtime?.controller.getSnapshot() ?? EMPTY_SNAPSHOT, [runtime]);
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  const attach = useCallback((element: HTMLVideoElement) => {
    setVideo(element);
    return () => setVideo((bound) => (bound === element ? null : bound));
  }, []);
  const controls = useMemo<BrowserPlayerControls>(
    () => ({
      goLive: async () => runtime?.controller.goLive(),
      pause: () => runtime?.controller.pause(),
      play: async () => runtime?.controller.play(),
      previous: async () => runtime?.controller.previous(),
      retry: async () => runtime?.controller.retry(),
      seek: async (wallClockMs) => runtime?.controller.seek(wallClockMs),
      step: async (direction) => runtime?.controller.step(direction),
      tuneChannel: async (channelId) => runtime?.controller.tuneChannel(channelId),
      tuneNumber: async (digits) => runtime?.controller.tuneNumber(digits),
    }),
    [runtime],
  );

  return { attach, controls, playbackSessionId: runtime?.transport.playbackSessionId ?? "", snapshot };
};

export type { BrowserPlayer, BrowserPlayerControls, BrowserPlayerOptions };
export { useBrowserPlayer };
