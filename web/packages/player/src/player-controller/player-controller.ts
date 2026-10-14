import { createNeighbourWarmer } from "../neighbour-warmer";
import type { PlayerChannel } from "../player-source";
import { StartFailure } from "../start-failure";
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
/** A stall this long is more than a hiccup: the viewer sees "Reconnecting…" instead of a frozen frame. */
const STALL_NOTICE_MS = 3_000;
/** A stall this long without progress fails with the manual Retry (the server is unreachable). */
const STALL_FAIL_MS = 20_000;
const STALL_FAILURE = "Lost the connection to the server.";

const playableCatalog = (channels: readonly PlayerChannel[]): PlayerChannel[] =>
  [...channels]
    .filter((channel) => channel.inAppPlayable)
    .sort((left, right) => left.number - right.number || left.id.localeCompare(right.id));

const createPlayerController = ({
  initialTune = "first",
  onPlayerError,
  onTune,
  prefetchStill,
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
  let stallNoticeTimer: ReturnType<typeof setTimeout> | undefined;
  let stallFailTimer: ReturnType<typeof setTimeout> | undefined;
  let snapshot: PlayerSnapshot = {
    catalog: [],
    recentChannelIds: [],
    status: "empty",
  };
  const listeners = new Set<(next: PlayerSnapshot) => void>();
  const warmer = createNeighbourWarmer({ prefetchStill, profile, radius: warmRadius, source });

  const publish = (next: PlayerSnapshot) => {
    snapshot = next;
    for (const listener of listeners) listener(snapshot);
  };

  const isCurrentAttempt = (attemptId: number, signal?: AbortSignal) =>
    !disposed && attemptId === attempt && !signal?.aborted;

  const cancelStall = () => {
    clearTimeout(stallNoticeTimer);
    clearTimeout(stallFailTimer);
    stallNoticeTimer = undefined;
    stallFailTimer = undefined;
  };

  const cancelRecoveryTimers = () => {
    clearTimeout(retryTimer);
    clearTimeout(stableTimer);
    retryTimer = undefined;
    stableTimer = undefined;
    cancelStall();
  };

  /**
   * A stall the viewer didn't cause shows "Reconnecting…" once it outlasts a hiccup, and fails with
   * the manual Retry once it shows no progress for long enough. The player keeps waiting meanwhile,
   * so a stream that comes back on its own still clears both.
   */
  const startStall = (attemptId: number) => {
    if (stallNoticeTimer || stallFailTimer) return;
    stallNoticeTimer = setTimeout(() => {
      stallNoticeTimer = undefined;
      if (!isCurrentAttempt(attemptId) || snapshot.status !== "playing") return;
      publish({ ...snapshot, reconnecting: { attempt: 0, maxAttempts: backoffMs.length }, status: "tuning" });
    }, STALL_NOTICE_MS);
    stallFailTimer = setTimeout(() => {
      stallFailTimer = undefined;
      const channel = snapshot.channel;
      if (
        !isCurrentAttempt(attemptId) ||
        !channel ||
        snapshot.status === "paused" ||
        snapshot.status === "failed"
      ) {
        return;
      }
      try {
        onPlayerError?.({
          attempt: consecutiveFailures + 1,
          channelId: channel.id,
          elapsedMs: Date.now() - tuneStartedAt,
          error: STALL_FAILURE,
          fatal: true,
        });
      } catch {
        // Diagnostics must never affect playback.
      }
      publish({ ...snapshot, error: STALL_FAILURE, reconnecting: undefined, status: "failed" });
    }, STALL_FAIL_MS);
  };

  /**
   * Counts one player error, reports it, and either schedules a silent re-tune of the same Channel
   * (fresh mint + replace) or, once the budget is spent, publishes the manual-Retry failure. A
   * start the server refused (`StartFailure`) is final at once: retrying it waits out the server's
   * start deadline again for the same answer.
   */
  const recordFailure = (message: string, startFailure?: StartFailure) => {
    const channel = snapshot.channel;
    if (!channel) return;
    clearTimeout(stableTimer);
    stableTimer = undefined;
    consecutiveFailures += 1;
    const retryable = !startFailure && consecutiveFailures <= backoffMs.length;
    try {
      onPlayerError?.({
        attempt: consecutiveFailures,
        channelId: channel.id,
        elapsedMs: Date.now() - tuneStartedAt,
        error: message,
        fatal: !retryable,
        ...(startFailure ? { status: startFailure.status } : {}),
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
    // Read before anything cancels or takes the warmed entry: this is what lets the switch overlay
    // paint in the same snapshot that starts the tune, with no request.
    const stillUri = recovering ? undefined : warmer.stillFor(channel.id);
    // A warmed neighbour's exact signed source skips the mint round trip and keeps the asset URLs
    // the warm already fetched. Recovery always mints fresh: the warmed one just failed.
    const warmedSource = recovering ? undefined : warmer.take(channel.id);
    // A stale prediction must not compete with the channel now being tuned.
    warmer.cancel();
    const request = new AbortController();
    activeRequest = request;
    attempt += 1;
    const attemptId = attempt;
    // A warm that read its master names the only rendition, ready to open. It goes to the player
    // before the tuning snapshot renders the switch overlay, so the native start is not queued
    // behind the overlay's mount on the main thread (#1037). A warm that didn't (refused, or a
    // master offering a choice) still saves the mint, but its master is read through `start`, like
    // a cold tune's, so a refusal arrives with the server's reason.
    const ready = warmedSource?.mediaUri !== undefined || !source.start ? warmedSource : undefined;
    const replacing = ready ? transport.replace(ready, { attemptId, signal: request.signal }) : undefined;
    replacing?.catch(() => undefined); // awaited below; never an unhandled rejection meanwhile
    let starting = false;
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
      stillUri,
      tuneReason: reason,
    });
    try {
      onTune?.({ attemptId, channel, reason, warm: Boolean(warmedSource) });
    } catch {
      // Instrumentation must never affect playback.
    }

    try {
      if (replacing) {
        await replacing;
      } else {
        const nextSource = warmedSource ?? (await source.mint(channel, profile, request.signal));
        if (!isCurrentAttempt(attemptId, request.signal)) return;
        // A channel nobody warmed gets its picture from its own mint: the server decodes a cold
        // channel's still on demand, so the overlay can show it while the stream starts.
        if (!recovering && !snapshot.stillUri && nextSource.stillUri && snapshot.status === "tuning") {
          publish({ ...snapshot, stillUri: nextSource.stillUri });
        }
        starting = true;
        // The outgoing channel holds its last frame, silent, while the server starts this one, and
        // stays held if it refuses; otherwise it would keep playing under the new channel's name.
        if (source.start) transport.pause();
        const started = source.start ? await source.start(nextSource, request.signal) : nextSource;
        starting = false;
        if (!isCurrentAttempt(attemptId, request.signal)) return;
        await transport.replace(started, { attemptId, signal: request.signal });
      }
      if (!isCurrentAttempt(attemptId, request.signal)) return;
      // The viewer's own stream is on its way; only now may neighbours start their sessions.
      warmer.retarget(snapshot.catalog, channel.id);
      if (snapshot.status === "paused") return;
      await transport.play();
    } catch (error) {
      if (!isCurrentAttempt(attemptId, request.signal)) return;
      const message = error instanceof Error ? error.message : "Couldn't tune that channel.";
      if (error instanceof StartFailure) {
        recordFailure(message, error);
        return;
      }
      // A start that failed without the server's reason (a dropped connection, a warm-up 404) gets
      // the automatic retries it got when the player made that request itself.
      if (recovering || starting) {
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
    if (event.type === "stalled") {
      if (snapshot.status === "playing") startStall(event.attemptId);
      return;
    }
    cancelStall();
    if (event.type === "error") {
      // A retry is already scheduled for this failure; a second error from the same dead player
      // must not spend budget or report twice.
      if (!retryTimer) recordFailure(event.error);
      return;
    }
    if (event.type === "paused") {
      cancelRecoveryTimers();
      // The native transport echoes the pause of a failed player; the failure stands (see pause).
      if (snapshot.status === "failed") return;
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
      // A failure has nothing to pause: it keeps its Retry, or the viewer comes back to a black
      // picture labelled Paused over a dead player.
      if (snapshot.status === "failed") return;
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
