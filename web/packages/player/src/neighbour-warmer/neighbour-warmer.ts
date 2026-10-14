import type { PlayerChannel, PlayerSource } from "../player-source";
import type { NeighbourWarmer, NeighbourWarmerOptions } from "./neighbour-warmer.type";

// One each side: the server retains at most two idle warm sessions (warmIdleSessionLimit), so a wider
// default ring would evict its own inner ring.
const DEFAULT_RADIUS = 1;

/** Channels at exactly `distance` steps either side of `index`, wrapping like channel up/down does. */
const ring = (catalog: readonly PlayerChannel[], index: number, distance: number): PlayerChannel[] => {
  const size = catalog.length;
  const picked = [catalog[(index + distance) % size], catalog[(((index - distance) % size) + size) % size]];
  return picked.filter(
    (channel, position): channel is PlayerChannel =>
      Boolean(channel) &&
      channel !== catalog[index] &&
      picked.findIndex((other) => other?.id === channel?.id) === position,
  );
};

/** A warmed source is only worth reusing while its signature outlives the tune. */
const MIN_REMAINING_MS = 60_000;

/**
 * Keeps the channels adjacent to the one on screen warm so channel up/down lands on a live session
 * that already exists. Latest request wins: every retarget aborts the previous warms, so speculation
 * never outlives the viewer's position. Warming is nearest-ring first; the next ring starts only once
 * the inner one settles, so a far neighbour never competes with a near one for host capacity. A warm
 * that fails (busy host, network) is a harmless miss; the real tune mints and attaches as normal.
 * Each warm's exact signed source is kept for `take`, so the real tune requests the very URLs that
 * were warmed.
 */
const createNeighbourWarmer = ({
  prefetchStill,
  profile,
  radius = DEFAULT_RADIUS,
  source,
}: NeighbourWarmerOptions): NeighbourWarmer => {
  let active: AbortController | undefined;
  const warmed = new Map<string, { at: number; source: PlayerSource }>();

  const cancel = () => {
    active?.abort();
    active = undefined;
  };

  const warmOne = async (channel: PlayerChannel, signal: AbortSignal) => {
    const result = await source.warm?.(channel, profile, signal);
    if (!result || signal.aborted) return;
    const { warmed: _certified, ...reusable } = result;
    warmed.set(channel.id, { at: Date.now(), source: reusable });
    // Fetched now, while the viewer is idle, so the switch overlay paints it from cache.
    if (reusable.stillUri) prefetchStill?.(reusable.stillUri);
  };

  const warmRings = async (
    catalog: readonly PlayerChannel[],
    index: number,
    controller: AbortController,
  ): Promise<void> => {
    for (let distance = 1; distance <= radius; distance += 1) {
      if (controller.signal.aborted) return;
      const targets = ring(catalog, index, distance);
      await Promise.allSettled(targets.map((channel) => warmOne(channel, controller.signal)));
    }
  };

  return {
    cancel,
    retarget: (catalog, currentId) => {
      cancel();
      const index = catalog.findIndex((channel) => channel.id === currentId);
      const keep = new Set<string>();
      if (index >= 0) {
        for (let distance = 1; distance <= radius; distance += 1) {
          for (const channel of ring(catalog, index, distance)) keep.add(channel.id);
        }
      }
      for (const id of warmed.keys()) if (!keep.has(id)) warmed.delete(id);
      if (index < 0 || catalog.length < 2 || !source.warm) return;
      const controller = new AbortController();
      active = controller;
      void warmRings(catalog, index, controller);
    },
    stillFor: (channelId) => warmed.get(channelId)?.source.stillUri,
    take: (channelId) => {
      const entry = warmed.get(channelId);
      warmed.delete(channelId);
      if (!entry) return undefined;
      const { expiresAt, serverTimeMs } = entry.source;
      if (expiresAt !== undefined && expiresAt < Date.now() + MIN_REMAINING_MS) return undefined;
      return {
        ...entry.source,
        ...(serverTimeMs !== undefined ? { serverTimeMs: serverTimeMs + (Date.now() - entry.at) } : {}),
      };
    },
  };
};

export { createNeighbourWarmer };
