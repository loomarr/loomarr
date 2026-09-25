import { createNeighbourWarmer } from "../neighbour-warmer";
import type { PlayerChannel } from "../player-source";
import type {
  PlayerController,
  PlayerControllerOptions,
  PlayerSnapshot,
  TuneDirection,
  TuneReason,
} from "./player-controller.type";

const RECENT_CHANNEL_LIMIT = 6;
const DEFAULT_BACKOFF_MS = [1_000, 3_000, 7_000] as const;
/** Playing this long without an error proves the stream recovered, so the retry budget refills. */
const STABLE_PLAYBACK_MS = 30_000;

const playableCatalog = (channels: readonly PlayerChannel[]): PlayerChannel[] =>
  [...channels]
    .filter((channel) => channel.inAppPlayable)
    .sort((left, right) => left.number - right.number || left.id.localeCompare(right.id));

const createPlayerController = ({
  initialTune = "first",
  onPlayerError,
  profile,
  recovery,
  source,
  transport,
  warmRadius,
}: PlayerControllerOptions): PlayerController => {
  const backoffMs = recovery?.backoffMs ?? DEFAULT_BACKOFF_MS;
  let disposed = false;
  let attempt = 0;
  let activeRequest: AbortController | undefined;
  let consecutiveFailures = 0;
  let tuneStartedAt = 0;
  let retryTimer: ReturnType<typeof setTimeout> | undefined;
  let stableTimer: ReturnType<typeof setTimeout> | undefined;
  let snapshot: PlayerSnapshot = {
    catalog: [],
    recentChannelIds: [],
    status: "empty",
  };
  const listeners = new Set<(next: PlayerSnapshot) => void>();
  const warmer = createNeighbourWarmer({ profile, radius: warmRadius, source });

  const publish = (next: PlayerSnapshot) => {
    snapshot = next;
    for (const listener of listeners) listener(snapshot);
  };

  const isCurrentAttempt = (attemptId: number, signal?: AbortSignal) =>
    !disposed && attemptId === attempt && !signal?.aborted;

  const cancelRecoveryTimers = () => {
    clearTimeout(retryTimer);
    clearTimeout(stableTimer);
    retryTimer = undefined;
    stableTimer = undefined;
  };

  /**
   * Counts one player error, reports it, and either schedules a silent re-tune of the same Channel
   * (fresh mint + replace) or, once the budget is spent, publishes the manual-Retry failure.
   */
  const recordFailure = (message: string) => {
    const channel = snapshot.channel;
    if (!channel) return;
    clearTimeout(stableTimer);
    stableTimer = undefined;
    consecutiveFailures += 1;
    const retryable = consecutiveFailures <= backoffMs.length;
    try {
      onPlayerError?.({
        attempt: consecutiveFailures,
        channelId: channel.id,
        elapsedMs: Date.now() - tuneStartedAt,
        error: message,
        fatal: !retryable,
      });
    } catch {
      // Diagnostics must never affect playback.
    }
    if (!retryable) {
      publish({ ...snapshot, error: message, reconnecting: undefined, status: "failed" });
      return;
    }
    publish({
      ...snapshot,
      error: undefined,
      reconnecting: { attempt: consecutiveFailures, maxAttempts: backoffMs.length },
      status: "tuning",
    });
    retryTimer = setTimeout(
      () => {
        retryTimer = undefined;
        void tune(channel, "retry", true, true);
      },
      backoffMs[consecutiveFailures - 1],
    );
  };

  const tune = async (channel: PlayerChannel, reason: TuneReason, force = false, recovering = false) => {
    if (disposed) return;
    if (!force && snapshot.channel?.id === channel.id && snapshot.status !== "failed") return;
    cancelRecoveryTimers();
    const tunedAt = Date.now();
    if (!recovering) {
      consecutiveFailures = 0;
      tuneStartedAt = tunedAt;
    }

    const previousId = snapshot.channel?.id;
    const recentChannelIds =
      previousId && previousId !== channel.id
        ? [previousId, ...snapshot.recentChannelIds]
            .filter((id, index, all) => id !== channel.id && all.indexOf(id) === index)
            .slice(0, RECENT_CHANNEL_LIMIT)
        : [...snapshot.recentChannelIds];

    activeRequest?.abort();
    // A stale prediction must not compete with the channel now being tuned.
    warmer.cancel();
    const request = new AbortController();
    activeRequest = request;
    attempt += 1;
    const attemptId = attempt;
    publish({
      attemptId,
      catalog: snapshot.catalog,
      channel,
      livePlayback: {
        lagSeconds: 0,
        mode: "live",
        noticeRevision: 0,
        viewerTimeMs: tunedAt,
      },
      previousChannelId: recentChannelIds[0],
      reconnecting: recovering ? { attempt: consecutiveFailures, maxAttempts: backoffMs.length } : undefined,
      recentChannelIds,
      status: "tuning",
      tuneReason: reason,
    });

    try {
      // A warmed neighbour's exact signed source skips the mint round trip and keeps the asset URLs
      // the warm already fetched. Recovery always mints fresh: the warmed one just failed.
      const nextSource =
        (!recovering && warmer.take(channel.id)) || (await source.mint(channel, profile, request.signal));
      if (!isCurrentAttempt(attemptId, request.signal)) return;
      await transport.replace(nextSource, { attemptId, signal: request.signal });
      if (!isCurrentAttempt(attemptId, request.signal)) return;
      // The viewer's own stream is on its way; only now may neighbours start their sessions.
      warmer.retarget(snapshot.catalog, channel.id);
      if (snapshot.status === "paused") return;
      await transport.play();
    } catch (error) {
      if (!isCurrentAttempt(attemptId, request.signal)) return;
      const message = error instanceof Error ? error.message : "Couldn't tune that channel.";
      if (recovering) {
        recordFailure(message);
        return;
      }
      publish({ ...snapshot, error: message, status: "failed" });
    }
  };

  const findAdjacent = (direction: TuneDirection) => {
    const catalog = snapshot.catalog;
    if (catalog.length === 0) return undefined;
    const index = catalog.findIndex((channel) => channel.id === snapshot.channel?.id);
    if (index < 0) return direction > 0 ? catalog[0] : catalog.at(-1);
    return catalog[(index + direction + catalog.length) % catalog.length];
  };

  const unsubscribeTransport = transport.subscribe((event) => {
    if (!isCurrentAttempt(event.attemptId) || event.attemptId !== snapshot.attemptId) return;
    if (event.type === "live-state") {
      publish({ ...snapshot, livePlayback: event.state });
      return;
    }
    if (event.type === "error") {
      // A retry is already scheduled for this failure; a second error from the same dead player
      // must not spend budget or report twice.
      if (!retryTimer) recordFailure(event.error);
      return;
    }
    if (event.type === "paused") {
      cancelRecoveryTimers();
      publish({ ...snapshot, error: undefined, reconnecting: undefined, status: "paused" });
      return;
    }
    if (event.type === "first-frame" && snapshot.status === "paused") return;
    if (event.type === "first-frame") consecutiveFailures = 0;
    else if (consecutiveFailures > 0 && !stableTimer) {
      stableTimer = setTimeout(() => {
        stableTimer = undefined;
        consecutiveFailures = 0;
      }, STABLE_PLAYBACK_MS);
    }
    publish({ ...snapshot, error: undefined, reconnecting: undefined, status: "playing" });
  });

  return {
    dispose: () => {
      if (disposed) return;
      disposed = true;
      cancelRecoveryTimers();
      activeRequest?.abort();
      warmer.cancel();
      unsubscribeTransport();
      transport.pause();
      transport.dispose();
      listeners.clear();
    },
    getSnapshot: () => snapshot,
    goLive: async () => {
      if (disposed || !snapshot.channel) return;
      await transport.goLive();
    },
    pause: () => {
      if (disposed || !snapshot.channel) return;
      cancelRecoveryTimers();
      transport.pause();
      publish({ ...snapshot, error: undefined, reconnecting: undefined, status: "paused" });
    },
    play: async () => {
      if (disposed || !snapshot.channel) return;
      await transport.play();
    },
    previous: async () => {
      const channel = snapshot.catalog.find(({ id }) => id === snapshot.previousChannelId);
      if (channel) await tune(channel, "previous");
    },
    reconcile: async (channels) => {
      if (disposed) return;
      const catalog = playableCatalog(channels);
      if (catalog.length === 0) {
        activeRequest?.abort();
        warmer.cancel();
        transport.pause();
        publish({
          catalog,
          recentChannelIds: snapshot.recentChannelIds,
          status: "empty",
        });
        return;
      }

      const current = catalog.find((channel) => channel.id === snapshot.channel?.id);
      if (current) {
        publish({ ...snapshot, catalog, channel: current });
        return;
      }
      if (!snapshot.channel && initialTune === "none") {
        publish({
          catalog,
          recentChannelIds: snapshot.recentChannelIds,
          status: "idle",
        });
        return;
      }

      publish({ ...snapshot, catalog });
      const first = catalog[0];
      if (first) await tune(first, "catalog");
    },
    retry: async () => {
      if (snapshot.channel) await tune(snapshot.channel, "retry", true);
    },
    step: async (direction) => {
      const channel = findAdjacent(direction);
      if (channel && (snapshot.catalog.length > 1 || channel.id !== snapshot.channel?.id)) {
        await tune(channel, "step");
      }
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    tuneChannel: async (channelId) => {
      const channel = snapshot.catalog.find(({ id }) => id === channelId);
      if (channel) await tune(channel, "channel");
    },
    tuneNumber: async (digits) => {
      if (!/^\d+$/.test(digits)) return;
      const number = Number.parseInt(digits, 10);
      const channel = snapshot.catalog.find((candidate) => candidate.number === number);
      if (channel) await tune(channel, "number");
    },
  };
};

export { createPlayerController, playableCatalog };
