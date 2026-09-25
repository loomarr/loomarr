import type { PlayerChannel } from "../player-source";
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

/**
 * Keeps the channels adjacent to the one on screen warm so channel up/down lands on a live session
 * that already exists. Latest request wins: every retarget aborts the previous warms, so speculation
 * never outlives the viewer's position. Warming is nearest-ring first; the next ring starts only once
 * the inner one settles, so a far neighbour never competes with a near one for host capacity. A warm
 * that fails (busy host, network) is a harmless miss; the real tune mints and attaches as normal.
 */
const createNeighbourWarmer = ({
  profile,
  radius = DEFAULT_RADIUS,
  source,
}: NeighbourWarmerOptions): NeighbourWarmer => {
  let active: AbortController | undefined;

  const cancel = () => {
    active?.abort();
    active = undefined;
  };

  const warmRings = async (
    catalog: readonly PlayerChannel[],
    index: number,
    controller: AbortController,
  ): Promise<void> => {
    const warm = source.warm;
    if (!warm) return;
    for (let distance = 1; distance <= radius; distance += 1) {
      if (controller.signal.aborted) return;
      const targets = ring(catalog, index, distance);
      await Promise.allSettled(targets.map((channel) => warm(channel, profile, controller.signal)));
    }
  };

  return {
    cancel,
    retarget: (catalog, currentId) => {
      cancel();
      const index = catalog.findIndex((channel) => channel.id === currentId);
      if (index < 0 || catalog.length < 2 || !source.warm) return;
      const controller = new AbortController();
      active = controller;
      void warmRings(catalog, index, controller);
    },
  };
};

export { createNeighbourWarmer };
